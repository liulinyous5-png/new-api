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
package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/endpointaudit"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

var (
	endpointAuditGate sync.Mutex
	endpointLive      *model.EndpointAuditRun
	endpointNextID    int
)

type endpointTestBody struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	SuiteID int    `json:"suite_id"`
}

func ListEndpointAuditRuns(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []gin.H{}})
}

func GetEndpointAuditRun(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		endpointAuditFail(c, http.StatusBadRequest, "invalid endpoint audit id")
		return
	}
	run, ok := endpointLiveCopy(id)
	if !ok {
		endpointAuditFail(c, http.StatusNotFound, "invalid endpoint audit id")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": endpointRunView(&run)})
}

func StartEndpointAudit(c *gin.Context) {
	var input endpointTestBody
	if err := decodeEndpointJSON(c, &input); err != nil {
		endpointAuditFail(c, http.StatusBadRequest, "only http and https endpoints can be audited")
		return
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" || len(apiKey) > 4096 || strings.ContainsAny(apiKey, "\r\n\\\"") {
		endpointAuditFail(c, http.StatusBadRequest, "api key is required")
		return
	}
	target, err := endpointaudit.NormalizeMessagesURL(c.Request.Context(), input.BaseURL)
	if err != nil {
		endpointAuditFail(c, http.StatusBadRequest, err.Error())
		return
	}
	suite, calls, err := endpointCallsForQualitySuite(input.SuiteID)
	if err != nil {
		if endpointSuiteMessage(err) {
			endpointAuditFail(c, http.StatusBadRequest, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	endpointAuditGate.Lock()
	if endpointLive != nil && endpointLive.Verdict == "running" {
		endpointAuditGate.Unlock()
		endpointAuditFail(c, http.StatusConflict, "an endpoint audit is already running")
		return
	}
	endpointNextID++
	run := model.EndpointAuditRun{
		Id: endpointNextID, SuiteId: suite.Id, SuiteName: suite.Name, BaseHost: target.Hostname(),
		Verdict: "running", Items: "[]", CreatedAt: common.GetTimestamp(),
	}
	endpointLive = &run
	started := run
	endpointAuditGate.Unlock()
	baseURL := input.BaseURL
	go runEndpointAudit(started.Id, baseURL, apiKey, calls)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": endpointRunView(&started)})
}

func runEndpointAudit(runID int, baseURL string, apiKey string, calls []endpointaudit.Call) {
	defer func() {
		if recover() != nil {
			finishEndpointRun(runID, apiKey, "error", "endpoint audit was interrupted", nil)
		}
	}()
	collected := make([]endpointaudit.ItemResult, 0, len(calls))
	_, verdict, err := endpointaudit.Execute(context.Background(), baseURL, apiKey, calls, func(item endpointaudit.ItemResult) {
		collected = append(collected, item)
		saveEndpointRunItems(runID, apiKey, collected)
	})
	if err != nil {
		finishEndpointRun(runID, apiKey, "error", err.Error(), collected)
		return
	}
	finishEndpointRun(runID, apiKey, verdict, "", collected)
}

func saveEndpointRunItems(runID int, apiKey string, items []endpointaudit.ItemResult) {
	encoded, err := common.Marshal(items)
	if err != nil {
		return
	}
	updateEndpointLive(runID, func(run *model.EndpointAuditRun) {
		run.Items = redactEndpointSecret(string(encoded), apiKey)
	})
}

func finishEndpointRun(runID int, apiKey string, verdict string, message string, items []endpointaudit.ItemResult) {
	if items == nil {
		items = []endpointaudit.ItemResult{}
	}
	encoded, err := common.Marshal(items)
	if err != nil {
		encoded = []byte("[]")
	}
	updateEndpointLive(runID, func(run *model.EndpointAuditRun) {
		run.Verdict = verdict
		run.Error = redactEndpointSecret(message, apiKey)
		run.Items = redactEndpointSecret(string(encoded), apiKey)
		run.FinishedAt = common.GetTimestamp()
	})
}

func updateEndpointLive(id int, update func(*model.EndpointAuditRun)) {
	endpointAuditGate.Lock()
	defer endpointAuditGate.Unlock()
	if endpointLive == nil || endpointLive.Id != id {
		return
	}
	update(endpointLive)
}

func endpointLiveCopy(id int) (model.EndpointAuditRun, bool) {
	endpointAuditGate.Lock()
	defer endpointAuditGate.Unlock()
	if endpointLive == nil || endpointLive.Id != id {
		return model.EndpointAuditRun{}, false
	}
	return *endpointLive, true
}

func redactEndpointSecret(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[redacted]")
}

func decodeEndpointJSON(c *gin.Context, dest any) error {
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20+65_536))
	if err != nil {
		return err
	}
	return common.Unmarshal(payload, dest)
}

func endpointRunView(run *model.EndpointAuditRun) gin.H {
	var items []endpointaudit.ItemResult
	_ = common.Unmarshal([]byte(run.Items), &items)
	if items == nil {
		items = []endpointaudit.ItemResult{}
	}
	return gin.H{
		"id": run.Id, "suite_id": run.SuiteId, "suite_name": run.SuiteName,
		"base_host": run.BaseHost, "verdict": run.Verdict, "error": run.Error,
		"items": items, "created_at": run.CreatedAt, "finished_at": run.FinishedAt,
	}
}

func endpointCallsForQualitySuite(suiteID int) (*model.ChannelQualitySuite, []endpointaudit.Call, error) {
	if suiteID <= 0 {
		return nil, nil, errors.New("invalid suite id")
	}
	suite, err := model.GetChannelQualitySuite(suiteID)
	if err != nil {
		return nil, nil, err
	}
	if suite == nil {
		return nil, nil, errors.New("quality suite not found")
	}
	items, err := model.ListChannelQualitySuiteItems(suite.Id)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, errors.New("quality suite has no probes")
	}
	calls := make([]endpointaudit.Call, 0, len(items))
	for _, item := range items {
		probe, err := model.GetChannelQualityProbe(item.ProbeId)
		if err != nil {
			return nil, nil, err
		}
		if probe == nil {
			return nil, nil, errors.New("probe not found")
		}
		headers, err := probe.HeaderMap()
		if err != nil {
			return nil, nil, errors.New("header name is invalid")
		}
		statuses, err := probe.StatusList()
		if err != nil {
			return nil, nil, errors.New("status code is invalid")
		}
		assertions, err := probe.AssertionList()
		if err != nil {
			return nil, nil, errors.New("each assertion needs a name and an expression")
		}
		exprAssertions := make([]endpointaudit.Assertion, 0, len(assertions))
		for _, assertion := range assertions {
			if strings.TrimSpace(assertion.Name) == "" || strings.TrimSpace(assertion.Expr) == "" {
				return nil, nil, errors.New("each assertion needs a name and an expression")
			}
			exprAssertions = append(exprAssertions, endpointaudit.Assertion{Name: assertion.Name, Expr: assertion.Expr})
		}
		calls = append(calls, endpointaudit.Call{
			ProbeName: probe.Name, Headers: headers, Body: probe.Body,
			Statuses: statuses, Assertions: exprAssertions, Model: item.Model,
		})
	}
	return suite, calls, nil
}

func endpointSuiteMessage(err error) bool {
	if err == nil {
		return false
	}
	return slices.Contains([]string{
		"invalid suite id",
		"quality suite not found",
		"quality suite has no probes",
		"probe not found",
		"header name is invalid",
		"status code is invalid",
		"each assertion needs a name and an expression",
	}, err.Error())
}

func endpointAuditFail(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}
