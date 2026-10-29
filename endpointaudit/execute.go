/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package endpointaudit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/qualityexpr"
)

const maxResponseBytes = 1<<20 + 1

type Call struct {
	ProbeName  string
	Headers    map[string]string
	Body       string
	Statuses   []int
	Assertions []Assertion
	Model      string
}

type Assertion struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type AssertionResult struct {
	Name   string `json:"name"`
	Expr   string `json:"expr,omitempty"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

type ItemResult struct {
	ProbeName    string            `json:"probe_name"`
	Model        string            `json:"model"`
	Verdict      string            `json:"verdict"`
	Status       int               `json:"status,omitempty"`
	Error        string            `json:"error,omitempty"`
	Response     string            `json:"response,omitempty"`
	Assertions   []AssertionResult `json:"assertions,omitempty"`
	AppliedEdits any               `json:"applied_edits,omitempty"`
	Usage        any               `json:"usage,omitempty"`
}

func Execute(ctx context.Context, baseURL string, apiKey string, calls []Call, onItem func(ItemResult)) ([]ItemResult, string, error) {
	target, err := NormalizeMessagesURL(ctx, baseURL)
	if err != nil {
		return nil, "", err
	}
	results := make([]ItemResult, 0, len(calls))
	for _, call := range calls {
		var result ItemResult
		if ctx.Err() != nil {
			result = ItemResult{ProbeName: call.ProbeName, Model: call.Model, Verdict: "error", Error: "endpoint audit was interrupted"}
		} else {
			result = executeCall(ctx, Client(), target.String(), apiKey, call)
		}
		results = append(results, result)
		if onItem != nil {
			onItem(result)
		}
	}
	return results, aggregate(results), nil
}

func executeCall(ctx context.Context, client *http.Client, target string, apiKey string, call Call) ItemResult {
	result := ItemResult{ProbeName: call.ProbeName, Model: call.Model, Verdict: "error"}
	body, err := prepareBody(call.Body, call.Model)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	requestCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		result.Error = "endpoint address is not allowed"
		return result
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("anthropic-version", "2023-06-01")
	for key, value := range call.Headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("x-api-key", apiKey)
	response, err := client.Do(request)
	if err != nil {
		result.Error = redact(publicError(err), apiKey)
		return result
	}
	defer response.Body.Close()
	payload, tooLarge, readErr := readLimited(response.Body)
	result.Status = response.StatusCode
	if readErr != nil {
		result.Error = "response is not JSON"
		return result
	}
	result.Response = redact(string(payload), apiKey)
	if tooLarge {
		result.Error = "upstream response is too large"
		return result
	}
	env := map[string]any{}
	jsonErr := common.Unmarshal(payload, &env)
	if jsonErr == nil {
		if edits, ok := env["context_management"].(map[string]any); ok {
			if applied, exists := edits["applied_edits"]; exists {
				result.AppliedEdits = applied
			}
		}
		if usage, ok := env["usage"]; ok {
			result.Usage = usage
		}
	}
	if !slices.Contains(call.Statuses, response.StatusCode) {
		result.Verdict = "failed"
		result.Error = "unexpected status code"
		return result
	}
	if len(call.Assertions) == 0 {
		result.Verdict = "passed"
		return result
	}
	if jsonErr != nil {
		result.Error = "response is not JSON"
		return result
	}
	passed := true
	for _, assertion := range call.Assertions {
		item := AssertionResult{Name: assertion.Name, Expr: assertion.Expr}
		program, err := qualityexpr.Compile(assertion.Expr)
		if err != nil {
			item.Error = err.Error()
			passed = false
			result.Assertions = append(result.Assertions, item)
			continue
		}
		ok, err := program.Run(env)
		if err != nil {
			item.Error = err.Error()
			passed = false
		} else {
			item.Passed = ok
			if !ok {
				passed = false
			}
		}
		result.Assertions = append(result.Assertions, item)
	}
	if passed {
		result.Verdict = "passed"
		return result
	}
	result.Verdict = "failed"
	return result
}

func prepareBody(body string, modelName string) ([]byte, error) {
	var payload map[string]any
	if err := common.Unmarshal([]byte(body), &payload); err != nil || payload == nil {
		return nil, errors.New("probe body is not JSON")
	}
	raw, _ := payload["model"].(string)
	raw = strings.TrimSpace(raw)
	modelName = strings.TrimSpace(modelName)
	if raw == "{{model}}" {
		if modelName == "" {
			return nil, errors.New("model is required")
		}
		payload["model"] = modelName
	} else if raw == "" {
		return nil, errors.New("probe body is missing model")
	}
	delete(payload, "stream")
	return common.Marshal(payload)
}

func readLimited(body io.Reader) ([]byte, bool, error) {
	payload, err := io.ReadAll(io.LimitReader(body, maxResponseBytes))
	if err != nil {
		return nil, false, err
	}
	return payload, len(payload) >= maxResponseBytes, nil
}

func publicError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream request timed out"
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "endpoint address is not allowed"):
		return "endpoint address is not allowed"
	case strings.Contains(text, "endpoint address could not be resolved"):
		return "endpoint address could not be resolved"
	case strings.Contains(text, "only http and https endpoints can be audited"):
		return "only http and https endpoints can be audited"
	default:
		return "endpoint address is not allowed"
	}
}

func redact(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[redacted]")
}

func aggregate(results []ItemResult) string {
	if len(results) == 0 {
		return "error"
	}
	failed := false
	errored := false
	passed := false
	for _, result := range results {
		switch result.Verdict {
		case "failed":
			failed = true
		case "error":
			errored = true
		case "passed":
			passed = true
		}
	}
	switch {
	case failed:
		return "failed"
	case errored:
		return "error"
	case passed:
		return "passed"
	default:
		return "error"
	}
}
