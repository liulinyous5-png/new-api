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
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type channelQualityScanHandler struct{}

func (channelQualityScanHandler) Type() string {
	return model.SystemTaskTypeChannelQualityScan
}

func (channelQualityScanHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoQualityEnabled
}

func (channelQualityScanHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoQualityMinutes
	if minutes < operation_setting.MinAutoQualityMinutes || minutes > operation_setting.MaxAutoQualityMinutes {
		minutes = 60
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelQualityScanHandler) NewPayload() any { return nil }

func (channelQualityScanHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runChannelQualityScan(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type qualitySuiteItemPayload struct {
	ProbeID int    `json:"probe_id"`
	Model   string `json:"model"`
}

type qualitySuitePayload struct {
	Name  string                    `json:"name"`
	Items []qualitySuiteItemPayload `json:"items"`
}

func ListChannelQualitySuites(c *gin.Context) {
	suites, err := model.ListChannelQualitySuites()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]gin.H, 0, len(suites))
	for _, suite := range suites {
		view, err := qualitySuiteView(suite)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		views = append(views, view)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": views})
}

func CreateChannelQualitySuite(c *gin.Context) {
	suite, items, err := bindQualitySuite(c, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.SaveChannelQualitySuite(suite, items); err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := qualitySuiteView(suite)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func UpdateChannelQualitySuite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("suiteId"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid suite id"})
		return
	}
	existing, err := model.GetChannelQualitySuite(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "quality suite not found"})
		return
	}
	suite, items, err := bindQualitySuite(c, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	suite.CreatedAt = existing.CreatedAt
	if err := model.SaveChannelQualitySuite(suite, items); err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := qualitySuiteView(suite)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func DeleteChannelQualitySuite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("suiteId"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid suite id"})
		return
	}
	if err := model.DeleteChannelQualitySuite(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
}

func GetChannelQualitySuiteRun(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	run, items, err := model.GetLatestChannelQualitySuiteRun(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if run == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
		return
	}
	itemViews := make([]gin.H, 0, len(items))
	for _, item := range items {
		itemViews = append(itemViews, qualityRunView(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":          run.Id,
			"channel_id":  run.ChannelId,
			"suite_id":    run.SuiteId,
			"suite_name":  run.SuiteName,
			"verdict":     run.Verdict,
			"error":       run.Error,
			"created_at":  run.CreatedAt,
			"finished_at": run.FinishedAt,
			"items":       itemViews,
		},
	})
}

func qualitySuiteView(suite *model.ChannelQualitySuite) (gin.H, error) {
	items, err := model.ListChannelQualitySuiteItems(suite.Id)
	if err != nil {
		return nil, err
	}
	viewItems := make([]gin.H, 0, len(items))
	for _, item := range items {
		viewItems = append(viewItems, gin.H{
			"id":       item.Id,
			"probe_id": item.ProbeId,
			"model":    item.Model,
			"sort":     item.Sort,
		})
	}
	return gin.H{
		"id":         suite.Id,
		"name":       suite.Name,
		"items":      viewItems,
		"created_at": suite.CreatedAt,
		"updated_at": suite.UpdatedAt,
	}, nil
}

func bindQualitySuite(c *gin.Context, id int) (*model.ChannelQualitySuite, []*model.ChannelQualitySuiteItem, error) {
	payload := qualitySuitePayload{}
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		return nil, nil, errors.New("invalid request body")
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return nil, nil, errors.New("quality suite name is required")
	}
	taken, err := model.ChannelQualitySuiteNameTaken(name, id)
	if err != nil {
		return nil, nil, err
	}
	if taken {
		return nil, nil, errors.New("quality suite name is already used")
	}
	if len(payload.Items) < 1 || len(payload.Items) > 50 {
		return nil, nil, errors.New("a quality suite needs 1 to 50 probes")
	}
	seen := make(map[string]struct{}, len(payload.Items))
	items := make([]*model.ChannelQualitySuiteItem, 0, len(payload.Items))
	for _, raw := range payload.Items {
		modelName := strings.TrimSpace(raw.Model)
		if modelName == "" || len(modelName) > 255 {
			return nil, nil, errors.New("quality suite model is required")
		}
		if raw.ProbeID <= 0 {
			return nil, nil, errors.New("probe not found")
		}
		probe, err := model.GetChannelQualityProbe(raw.ProbeID)
		if err != nil {
			return nil, nil, err
		}
		if probe == nil {
			return nil, nil, errors.New("probe not found")
		}
		var body map[string]any
		if err := common.Unmarshal([]byte(probe.Body), &body); err != nil || body == nil {
			return nil, nil, errors.New("probe body must be a JSON object")
		}
		if _, err := applyQualityProbeModel(body, modelName); err != nil {
			return nil, nil, err
		}
		key := strconv.Itoa(raw.ProbeID) + "\x00" + modelName
		if _, ok := seen[key]; ok {
			return nil, nil, errors.New("a quality suite lists this probe and model more than once")
		}
		seen[key] = struct{}{}
		items = append(items, &model.ChannelQualitySuiteItem{
			ProbeId: raw.ProbeID,
			Model:   modelName,
		})
	}
	return &model.ChannelQualitySuite{Id: id, Name: name}, items, nil
}

func qualitySuiteVerdict(verdicts []string) string {
	if len(verdicts) == 0 {
		return model.ChannelQualityVerdictSkipped
	}
	passed := false
	failed := false
	unknown := false
	errored := false
	unsupported := false
	for _, verdict := range verdicts {
		switch verdict {
		case model.ChannelQualityVerdictFailed:
			failed = true
		case model.ChannelQualityVerdictUnknown:
			unknown = true
		case model.ChannelQualityVerdictError:
			errored = true
		case model.ChannelQualityVerdictPassed:
			passed = true
		case model.ChannelQualityVerdictUnsupported:
			unsupported = true
		}
	}
	if failed {
		return model.ChannelQualityVerdictFailed
	}
	if errored {
		return model.ChannelQualityVerdictError
	}
	if unknown {
		return model.ChannelQualityVerdictUnknown
	}
	if passed {
		return model.ChannelQualityVerdictPassed
	}
	if unsupported {
		return model.ChannelQualityVerdictUnsupported
	}
	return model.ChannelQualityVerdictSkipped
}

func runChannelQualityScan(ctx context.Context) (map[string]int, error) {
	if err := model.ExpireInterruptedQualitySuites(); err != nil {
		return nil, err
	}
	channels, err := model.ListQualityAutoChannels()
	if err != nil {
		return nil, err
	}
	summary := map[string]int{"channels": len(channels)}
	webhookConfig, configErr := service.GetQualityWebhookConfig()
	var alerts []service.QualityWebhookAlert
	for _, channel := range channels {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		verdict, err := runChannelQualitySuite(ctx, channel)
		if err != nil {
			return summary, err
		}
		summary[verdict]++
		if configErr == nil && webhookConfig.Enabled {
			if verdict == model.ChannelQualityVerdictPassed {
				service.ClearQualityWebhookCooldown(channel.Id, channel.QualitySuiteId)
			}
			run, items, readErr := model.GetLatestChannelQualitySuiteRun(channel.Id)
			if readErr != nil {
				summary["webhook_errors"]++
			}
			if readErr == nil && run != nil {
				if alert := service.BuildQualityWebhookAlert(channel, run, items); alert != nil {
					alerts = append(alerts, *alert)
				}
			}
		}
	}
	if configErr != nil {
		summary["webhook_errors"]++
	} else if len(alerts) > 0 {
		delivered, err := service.DeliverQualityWebhooks(ctx, alerts)
		summary["webhook_delivered"] = delivered
		if err != nil {
			summary["webhook_errors"]++
			common.SysError(common.LogText("quality audit webhook delivery failed") + ": " + err.Error())
		}
	}
	return summary, nil
}

func runChannelQualitySuite(ctx context.Context, channel *model.Channel) (string, error) {
	inProgress, err := model.ChannelQualityInProgress(channel.Id)
	if err != nil {
		return "", err
	}
	suite, err := model.GetChannelQualitySuite(channel.QualitySuiteId)
	if err != nil {
		return "", err
	}
	run := &model.ChannelQualitySuiteRun{
		ChannelId: channel.Id,
		SuiteId:   channel.QualitySuiteId,
		Verdict:   model.ChannelQualityVerdictRunning,
	}
	if suite != nil {
		run.SuiteName = suite.Name
	}
	if inProgress {
		return finishQualitySuiteRun(run, model.ChannelQualityVerdictError, "a quality audit is already running")
	}
	if suite == nil {
		return finishQualitySuiteRun(run, model.ChannelQualityVerdictError, "quality suite not found")
	}
	if channel.Type != constant.ChannelTypeAnthropic && channel.Type != constant.ChannelTypeAws {
		return finishQualitySuiteRun(run, model.ChannelQualityVerdictUnsupported, "only Anthropic and AWS channels can be audited")
	}
	if channel.Type == constant.ChannelTypeAws {
		settings := channel.GetOtherSettings()
		if err := aws.MessagesProbeSupported(string(settings.AwsApiType), string(settings.AwsKeyType)); err != nil {
			return finishQualitySuiteRun(run, model.ChannelQualityVerdictUnsupported, err.Error())
		}
	}
	items, err := model.ListChannelQualitySuiteItems(suite.Id)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return finishQualitySuiteRun(run, model.ChannelQualityVerdictSkipped, "quality suite has no probes")
	}
	if err := model.CreateChannelQualitySuiteRun(run); err != nil {
		return "", err
	}
	verdicts := make([]string, 0, len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			return finishSavedQualitySuiteRun(run, model.ChannelQualityVerdictError, ctx.Err().Error())
		}
		verdict, err := executeQualitySuiteItem(ctx, channel, item, run.Id)
		if err != nil {
			_, finishErr := finishSavedQualitySuiteRun(run, model.ChannelQualityVerdictError, "failed to save quality audit response")
			return "", errors.Join(err, finishErr)
		}
		verdicts = append(verdicts, verdict)
		if ctx.Err() != nil {
			return finishSavedQualitySuiteRun(run, model.ChannelQualityVerdictError, ctx.Err().Error())
		}
	}
	return finishSavedQualitySuiteRun(run, qualitySuiteVerdict(verdicts), "")
}

func finishQualitySuiteRun(run *model.ChannelQualitySuiteRun, verdict string, message string) (string, error) {
	run.Verdict = verdict
	run.Error = message
	run.FinishedAt = common.GetTimestamp()
	if err := model.CreateChannelQualitySuiteRun(run); err != nil {
		return "", err
	}
	return verdict, nil
}

func finishSavedQualitySuiteRun(run *model.ChannelQualitySuiteRun, verdict string, message string) (string, error) {
	run.Verdict = verdict
	run.Error = message
	run.FinishedAt = common.GetTimestamp()
	if err := model.SaveChannelQualitySuiteRun(run); err != nil {
		return "", err
	}
	return verdict, nil
}

func executeQualitySuiteItem(ctx context.Context, channel *model.Channel, item *model.ChannelQualitySuiteItem, suiteRunID int) (string, error) {
	run := &model.ChannelQualityRun{
		ChannelId:      channel.Id,
		ProbeId:        item.ProbeId,
		RequestedModel: item.Model,
		Verdict:        model.ChannelQualityVerdictRunning,
		SuiteRunId:     suiteRunID,
		SuiteItemId:    item.Id,
	}
	if err := model.CreateChannelQualityRun(run); err != nil {
		return "", err
	}
	probe, err := model.GetChannelQualityProbe(item.ProbeId)
	if err != nil {
		run.Verdict = model.ChannelQualityVerdictError
		run.Error = err.Error()
		return run.Verdict, model.SaveChannelQualityRun(run)
	}
	if probe == nil {
		run.Verdict = model.ChannelQualityVerdictError
		run.Error = "probe not found"
		return run.Verdict, model.SaveChannelQualityRun(run)
	}
	run.ProbeName = probe.Name
	if !channelServesModel(channel, item.Model) {
		run.Verdict = model.ChannelQualityVerdictSkipped
		run.Error = "channel does not serve this model"
		return run.Verdict, model.SaveChannelQualityRun(run)
	}
	outcome, err := auditAnthropicChannel(ctx, channel, probe, item.Model, nil)
	applyQualityOutcome(run, outcome, err)
	if saveErr := model.SaveChannelQualityResult(run, outcome.Response); saveErr != nil {
		failQualityRun(run, model.ChannelQualityVerdictError, "failed to save quality audit response")
		return run.Verdict, saveErr
	}
	return run.Verdict, nil
}

func channelServesModel(channel *model.Channel, name string) bool {
	name = strings.TrimSpace(name)
	if channel == nil || name == "" {
		return false
	}
	for _, modelName := range channel.GetModels() {
		if strings.TrimSpace(modelName) == name {
			return true
		}
	}
	return false
}

func applyQualityOutcome(run *model.ChannelQualityRun, outcome qualityAuditOutcome, err error) {
	run.RequestedModel = outcome.RequestedModel
	run.UpstreamModel = outcome.UpstreamModel
	run.ProbeSnapshot = outcome.ProbeSnapshot
	run.Verdict = outcome.Verdict
	run.AssertionResults = outcome.AssertionResults
	run.ActualEdits = outcome.ActualEdits
	run.UsageJSON = outcome.UsageJSON
	run.Error = outcome.Error
	if err != nil {
		if run.Error == "" {
			run.Error = err.Error()
		}
		run.Verdict = model.ChannelQualityVerdictError
	}
	if run.Verdict == "" {
		run.Verdict = model.ChannelQualityVerdictError
	}
}

// Keep the scan handler referenced by the scheduler import check.
var _ service.ScheduledSystemTaskHandler = channelQualityScanHandler{}
