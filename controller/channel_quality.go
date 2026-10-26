package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	upstream "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/qualityexpr"

	"github.com/gin-gonic/gin"
)

const maxQualityResponseBytes = 1 << 20

type channelQualityTaskPayload struct {
	ChannelID int    `json:"channel_id"`
	ProbeID   int    `json:"probe_id"`
	Model     string `json:"model"`
	KeyIndex  *int   `json:"key_index,omitempty"`
	RunID     int    `json:"run_id"`
}

type channelQualityHandler struct{}

func (channelQualityHandler) Type() string { return model.SystemTaskTypeChannelQuality }

func (channelQualityHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelQualityTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	result, err := executeChannelQualityTask(ctx, payload)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

func init() {
	service.RegisterSystemTaskHandler(channelQualityHandler{})
}

type qualityProbePayload struct {
	Name       string                          `json:"name"`
	Headers    map[string]string               `json:"headers"`
	Body       string                          `json:"body"`
	Statuses   []int                           `json:"statuses"`
	Assertions []model.ChannelQualityAssertion `json:"assertions"`
}

func ListChannelQualityProbes(c *gin.Context) {
	probes, err := model.ListChannelQualityProbes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]gin.H, 0, len(probes))
	for _, probe := range probes {
		view, err := qualityProbeView(probe)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		views = append(views, view)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": views})
}

func CreateChannelQualityProbe(c *gin.Context) {
	probe, err := bindQualityProbe(c, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.CreateChannelQualityProbe(probe); err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := qualityProbeView(probe)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func UpdateChannelQualityProbe(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("probeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid probe id"})
		return
	}
	existing, err := model.GetChannelQualityProbe(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing == nil || existing.Builtin {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe not found"})
		return
	}
	probe, err := bindQualityProbe(c, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	probe.Id = id
	if err := model.UpdateChannelQualityProbe(probe); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetChannelQualityProbe(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := qualityProbeView(updated)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func DeleteChannelQualityProbe(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("probeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid probe id"})
		return
	}
	existing, err := model.GetChannelQualityProbe(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing == nil || existing.Builtin {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe not found"})
		return
	}
	if err := model.DeleteChannelQualityProbe(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func CopyChannelQualityProbe(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("probeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid probe id"})
		return
	}
	existing, err := model.GetChannelQualityProbe(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe not found"})
		return
	}
	name := copyQualityProbeName(existing.Name)
	taken, err := model.ChannelQualityProbeNameTaken(name, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if taken {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "a probe with this name already exists"})
		return
	}
	probe := &model.ChannelQualityProbe{
		Name:       name,
		Headers:    existing.Headers,
		Body:       existing.Body,
		Statuses:   existing.Statuses,
		Assertions: existing.Assertions,
	}
	if err := model.CreateChannelQualityProbe(probe); err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := qualityProbeView(probe)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func ListChannelQualityRuns(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	page := common.GetPageQuery(c)
	if page.PageSize > 20 {
		page.PageSize = 20
	}
	runs, total, err := model.ListChannelQualityRuns(channelID, page.GetStartIdx(), page.PageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]gin.H, 0, len(runs))
	for _, run := range runs {
		views = append(views, qualityRunView(run))
	}
	page.SetTotal(total)
	page.SetItems(views)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": page})
}

func GetChannelQualityRun(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	run, err := model.GetLatestChannelQualityRun(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if run == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": qualityRunView(run)})
}

func ListLatestChannelQuality(c *gin.Context) {
	latest, err := model.ListLatestChannelQualityRuns()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": latest})
}

func StartChannelQuality(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	var request struct {
		ProbeID  int    `json:"probe_id"`
		Model    string `json:"model"`
		KeyIndex *int   `json:"key_index"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if request.ProbeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe is required"})
		return
	}
	inProgress, err := model.ChannelQualityInProgress(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if inProgress {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "a quality audit is already running",
		})
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	probe, err := model.GetChannelQualityProbe(request.ProbeID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if probe == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "probe not found"})
		return
	}
	if request.KeyIndex != nil {
		if !channel.ChannelInfo.IsMultiKey || *request.KeyIndex < 0 || *request.KeyIndex >= len(channel.GetKeys()) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "account index out of range"})
			return
		}
	}
	run := &model.ChannelQualityRun{
		ChannelId: channel.Id,
		ProbeId:   probe.Id,
		ProbeName: probe.Name,
		Verdict:   model.ChannelQualityVerdictRunning,
	}
	if request.KeyIndex != nil {
		run.KeyIndex = *request.KeyIndex
	}
	if err := model.CreateChannelQualityRun(run); err != nil {
		common.ApiError(c, err)
		return
	}
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelQuality, channelQualityTaskPayload{
		ChannelID: channel.Id,
		ProbeID:   probe.Id,
		Model:     strings.TrimSpace(request.Model),
		KeyIndex:  request.KeyIndex,
		RunID:     run.Id,
	})
	if err != nil {
		run.Verdict = model.ChannelQualityVerdictError
		run.Error = err.Error()
		_ = model.SaveChannelQualityRun(run)
		common.ApiError(c, err)
		return
	}
	if !created {
		run.Verdict = model.ChannelQualityVerdictError
		run.Error = "a quality audit is already running"
		_ = model.SaveChannelQualityRun(run)
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "a quality audit is already running",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id":    task.TaskID,
			"run_id":     run.Id,
			"started_at": run.CreatedAt,
		},
	})
}

func copyQualityProbeName(name string) string {
	const suffix = " copy"
	const maxRunes = 128
	if utf8.RuneCountInString(name)+len(suffix) <= maxRunes {
		return name + suffix
	}
	runes := []rune(name)
	keep := maxRunes - len(suffix)
	if keep < 1 {
		keep = 1
	}
	if keep > len(runes) {
		keep = len(runes)
	}
	return string(runes[:keep]) + suffix
}

func bindQualityProbe(c *gin.Context, excludeID int) (*model.ChannelQualityProbe, error) {
	var payload qualityProbePayload
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		return nil, errors.New("invalid request body")
	}
	return normalizeQualityProbe(payload, excludeID)
}

func normalizeQualityProbe(payload qualityProbePayload, excludeID int) (*model.ChannelQualityProbe, error) {
	name := strings.TrimSpace(payload.Name)
	if name == "" || len(name) > 128 {
		return nil, errors.New("probe name is required")
	}
	if len(payload.Body) == 0 || len(payload.Body) > model.MaxChannelQualityProbeBodyBytes {
		return nil, errors.New("probe body is empty or too large")
	}
	var body map[string]any
	if err := common.Unmarshal([]byte(payload.Body), &body); err != nil || body == nil {
		return nil, errors.New("probe body must be a JSON object")
	}
	if len(payload.Assertions) > 20 {
		return nil, errors.New("a probe can have at most 20 assertions")
	}
	statuses, err := normalizeQualityStatuses(payload.Statuses)
	if err != nil {
		return nil, err
	}
	assertions := make([]model.ChannelQualityAssertion, 0, len(payload.Assertions))
	for _, assertion := range payload.Assertions {
		assertion.Name = strings.TrimSpace(assertion.Name)
		assertion.Expr = strings.TrimSpace(assertion.Expr)
		if assertion.Name == "" || assertion.Expr == "" {
			return nil, errors.New("each assertion needs a name and an expression")
		}
		if _, err := qualityexpr.Compile(assertion.Expr); err != nil {
			return nil, fmt.Errorf("assertion %s: %w", assertion.Name, err)
		}
		assertions = append(assertions, assertion)
	}
	headers := payload.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return nil, errors.New("header names and values must be non-empty")
		}
	}
	taken, err := model.ChannelQualityProbeNameTaken(name, excludeID)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, errors.New("a probe with this name already exists")
	}
	headerJSON, err := common.Marshal(headers)
	if err != nil {
		return nil, err
	}
	statusJSON, err := common.Marshal(statuses)
	if err != nil {
		return nil, err
	}
	assertionJSON, err := common.Marshal(assertions)
	if err != nil {
		return nil, err
	}
	return &model.ChannelQualityProbe{
		Name:       name,
		Headers:    string(headerJSON),
		Body:       payload.Body,
		Statuses:   string(statusJSON),
		Assertions: string(assertionJSON),
	}, nil
}

func normalizeQualityStatuses(statuses []int) ([]int, error) {
	if len(statuses) == 0 {
		return nil, errors.New("a probe needs at least one response status")
	}
	if len(statuses) > 20 {
		return nil, errors.New("a probe can have at most 20 response statuses")
	}
	cleaned := make([]int, 0, len(statuses))
	for _, status := range statuses {
		if status < 100 || status > 599 {
			return nil, errors.New("response status must be between 100 and 599")
		}
		if slices.Contains(cleaned, status) {
			continue
		}
		cleaned = append(cleaned, status)
	}
	slices.Sort(cleaned)
	return cleaned, nil
}

func qualityProbeView(probe *model.ChannelQualityProbe) (gin.H, error) {
	headers, err := probe.HeaderMap()
	if err != nil {
		return nil, err
	}
	assertions, err := probe.AssertionList()
	if err != nil {
		return nil, err
	}
	statuses, err := probe.StatusList()
	if err != nil {
		return nil, err
	}
	return gin.H{
		"id":         probe.Id,
		"name":       probe.Name,
		"builtin":    probe.Builtin,
		"headers":    headers,
		"body":       probe.Body,
		"statuses":   statuses,
		"assertions": assertions,
		"created_at": probe.CreatedAt,
		"updated_at": probe.UpdatedAt,
	}, nil
}

func qualityRunView(run *model.ChannelQualityRun) gin.H {
	return gin.H{
		"id":                run.Id,
		"channel_id":        run.ChannelId,
		"key_index":         run.KeyIndex,
		"requested_model":   run.RequestedModel,
		"upstream_model":    run.UpstreamModel,
		"probe_id":          run.ProbeId,
		"probe_name":        run.ProbeName,
		"verdict":           run.Verdict,
		"assertion_results": decodeQualityJSON(run.AssertionResults),
		"actual_edits":      decodeQualityJSON(run.ActualEdits),
		"usage":             decodeQualityJSON(run.UsageJSON),
		"error":             run.Error,
		"suite_run_id":      run.SuiteRunId,
		"suite_item_id":     run.SuiteItemId,
		"created_at":        run.CreatedAt,
	}
}

func decodeQualityJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var value any
	if err := common.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}

type qualityAssertionResult struct {
	Name   string `json:"name"`
	Expr   string `json:"expr"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

func executeChannelQualityTask(ctx context.Context, payload channelQualityTaskPayload) (gin.H, error) {
	run, err := loadQualityRun(payload.RunID)
	if err != nil {
		return nil, err
	}
	resultView := func() gin.H {
		if run == nil {
			return nil
		}
		return qualityRunView(run)
	}
	channel, err := model.GetChannelById(payload.ChannelID, true)
	if err != nil {
		return failQualityRun(run, model.ChannelQualityVerdictError, err.Error()), err
	}
	probe, err := model.GetChannelQualityProbe(payload.ProbeID)
	if err != nil {
		return failQualityRun(run, model.ChannelQualityVerdictError, err.Error()), err
	}
	if probe == nil {
		return failQualityRun(run, model.ChannelQualityVerdictError, "probe not found"), errors.New("probe not found")
	}
	if channel.Type != constant.ChannelTypeAnthropic && channel.Type != constant.ChannelTypeAws {
		return failQualityRun(run, model.ChannelQualityVerdictUnsupported, "only Anthropic and AWS channels can be audited"), nil
	}
	if ctx.Err() != nil {
		return failQualityRun(run, model.ChannelQualityVerdictError, ctx.Err().Error()), ctx.Err()
	}
	outcome, err := auditAnthropicChannel(ctx, channel, probe, payload.Model, payload.KeyIndex)
	if run != nil {
		run.RequestedModel = outcome.RequestedModel
		run.UpstreamModel = outcome.UpstreamModel
		run.ProbeSnapshot = outcome.ProbeSnapshot
		run.Verdict = outcome.Verdict
		run.AssertionResults = outcome.AssertionResults
		run.ActualEdits = outcome.ActualEdits
		run.UsageJSON = outcome.UsageJSON
		run.Error = outcome.Error
		if err != nil && run.Error == "" {
			run.Error = err.Error()
			run.Verdict = model.ChannelQualityVerdictError
		}
		if saveErr := model.SaveChannelQualityRun(run); saveErr != nil {
			return resultView(), saveErr
		}
	}
	if err != nil {
		return resultView(), err
	}
	return resultView(), nil
}

func loadQualityRun(id int) (*model.ChannelQualityRun, error) {
	if id <= 0 {
		return nil, nil
	}
	run := &model.ChannelQualityRun{}
	err := model.DB.First(run, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return run, nil
}

func failQualityRun(run *model.ChannelQualityRun, verdict string, message string) gin.H {
	if run == nil {
		return gin.H{"verdict": verdict, "error": message}
	}
	run.Verdict = verdict
	run.Error = message
	_ = model.SaveChannelQualityRun(run)
	return qualityRunView(run)
}

type qualityAuditOutcome struct {
	RequestedModel   string
	UpstreamModel    string
	Verdict          string
	AssertionResults string
	ActualEdits      string
	UsageJSON        string
	Error            string
	ProbeSnapshot    string
}

func auditAnthropicChannel(ctx context.Context, channel *model.Channel, probe *model.ChannelQualityProbe, selectedModel string, keyIndex *int) (qualityAuditOutcome, error) {
	outcome := qualityAuditOutcome{Verdict: model.ChannelQualityVerdictError}
	headers, err := probe.HeaderMap()
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	assertions, err := probe.AssertionList()
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	statuses, err := probe.StatusList()
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	snapshot, err := common.Marshal(map[string]any{
		"headers":    headers,
		"statuses":   statuses,
		"assertions": assertions,
	})
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	outcome.ProbeSnapshot = string(snapshot)

	bodyMap := map[string]any{}
	if err := common.Unmarshal([]byte(probe.Body), &bodyMap); err != nil {
		outcome.Error = "probe body must be a JSON object"
		return outcome, nil
	}
	requestedModel, err := applyQualityProbeModel(bodyMap, selectedModel)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	outcome.RequestedModel = requestedModel
	delete(bodyMap, "stream")
	encoded, err := common.Marshal(bodyMap)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	request := &dto.ClaudeRequest{}
	if err := common.Unmarshal(encoded, request); err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	httpRequest := httptest.NewRequestWithContext(callCtx, http.MethodPost, "/v1/messages", nil)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	for key, value := range headers {
		httpRequest.Header.Set(key, value)
	}
	ginCtx.Request = httpRequest
	ginCtx.Set("channel", channel.Type)
	ginCtx.Set("base_url", channel.GetBaseURL())
	var keyIndexes []int
	if keyIndex != nil {
		keyIndexes = []int{*keyIndex}
	}
	if apiErr := middleware.SetupContextForSelectedChannel(ginCtx, channel, requestedModel, keyIndexes...); apiErr != nil {
		outcome.Error = apiErr.Error()
		return outcome, nil
	}
	info, err := relaycommon.GenRelayInfo(ginCtx, types.RelayFormatClaude, request, nil)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	info.IsChannelTest = true
	info.InitChannelMeta(ginCtx)
	if err := helper.ModelMappedHelper(ginCtx, info, request); err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	outcome.UpstreamModel = info.UpstreamModelName
	adaptor := relay.GetAdaptor(info.ApiType)
	if adaptor == nil {
		outcome.Error = "anthropic adaptor is unavailable"
		return outcome, nil
	}
	converted, err := adaptor.ConvertClaudeRequest(ginCtx, info, request)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	payload, err := common.Marshal(converted)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	if channel.Type == constant.ChannelTypeAws {
		awsAdaptor, ok := adaptor.(*aws.Adaptor)
		if !ok {
			outcome.Verdict = model.ChannelQualityVerdictUnsupported
			outcome.Error = "this AWS channel mode cannot run a Messages probe"
			return outcome, nil
		}
		status, respBody, err := awsAdaptor.ExecuteQuality(ginCtx, info, payload, headers)
		if err != nil {
			var unsupported *aws.QualityUnsupportedError
			if errors.As(err, &unsupported) {
				outcome.Verdict = model.ChannelQualityVerdictUnsupported
			}
			outcome.Error = err.Error()
			return outcome, nil
		}
		return finishQualityResponse(outcome, statuses, assertions, status, respBody), nil
	}
	resp, err := upstream.DoQualityRequest(adaptor, ginCtx, info, bytes.NewReader(payload), headers)
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxQualityResponseBytes+1))
	if err != nil {
		outcome.Error = err.Error()
		return outcome, nil
	}
	return finishQualityResponse(outcome, statuses, assertions, resp.StatusCode, respBody), nil
}

func finishQualityResponse(outcome qualityAuditOutcome, statuses []int, assertions []model.ChannelQualityAssertion, status int, respBody []byte) qualityAuditOutcome {
	if len(respBody) > maxQualityResponseBytes {
		outcome.Error = "upstream response is too large"
		return outcome
	}
	if !slices.Contains(statuses, status) {
		outcome.Verdict = model.ChannelQualityVerdictFailed
		outcome.Error = fmt.Sprintf("upstream status %d: %s", status, trimQualitySnippet(respBody))
		return outcome
	}
	if len(assertions) == 0 {
		outcome.Verdict = model.ChannelQualityVerdictPassed
		return outcome
	}
	env := map[string]any{}
	if err := common.Unmarshal(respBody, &env); err != nil {
		outcome.Error = "response is not JSON"
		return outcome
	}
	outcome.UsageJSON = qualityJSONField(env, "usage")
	outcome.ActualEdits = qualityAppliedEdits(env)
	results, failed := evaluateQualityAssertions(assertions, env)
	encodedResults, err := common.Marshal(results)
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	outcome.AssertionResults = string(encodedResults)
	if failed {
		outcome.Verdict = model.ChannelQualityVerdictFailed
		return outcome
	}
	outcome.Verdict = model.ChannelQualityVerdictPassed
	return outcome
}

func applyQualityProbeModel(body map[string]any, selectedModel string) (string, error) {
	raw, _ := body["model"].(string)
	raw = strings.TrimSpace(raw)
	selectedModel = strings.TrimSpace(selectedModel)
	if raw == model.ChannelQualityModelPlaceholder {
		if selectedModel == "" {
			return "", errors.New("model is required")
		}
		body["model"] = selectedModel
		return selectedModel, nil
	}
	if raw == "" {
		return "", errors.New("probe body is missing model")
	}
	return raw, nil
}

func evaluateQualityAssertions(assertions []model.ChannelQualityAssertion, env map[string]any) ([]qualityAssertionResult, bool) {
	results := make([]qualityAssertionResult, 0, len(assertions))
	failed := false
	for _, assertion := range assertions {
		result := qualityAssertionResult{Name: assertion.Name, Expr: assertion.Expr}
		prog, err := qualityexpr.Compile(assertion.Expr)
		if err != nil {
			result.Error = err.Error()
			failed = true
			results = append(results, result)
			continue
		}
		passed, err := prog.Run(env)
		if err != nil {
			result.Error = err.Error()
			failed = true
			results = append(results, result)
			continue
		}
		result.Passed = passed
		if !passed {
			failed = true
		}
		results = append(results, result)
	}
	return results, failed
}

func qualityJSONField(env map[string]any, key string) string {
	value, ok := env[key]
	if !ok || value == nil {
		return ""
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func qualityAppliedEdits(env map[string]any) string {
	management, ok := env["context_management"].(map[string]any)
	if !ok {
		return ""
	}
	edits, ok := management["applied_edits"]
	if !ok {
		return ""
	}
	encoded, err := common.Marshal(edits)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func trimQualitySnippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}
