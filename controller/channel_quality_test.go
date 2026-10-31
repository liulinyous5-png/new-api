package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/qualityexpr"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAuditAnthropicChannelChecksRawResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var seenModel string
	var seenBeta string
	var seenContext bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		payload := map[string]any{}
		require.NoError(t, common.Unmarshal(body, &payload))
		seenModel, _ = payload["model"].(string)
		_, seenContext = payload["context_management"]
		seenBeta = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context_management":{"applied_edits":[{"type":"clear_tool_uses_20250919","cleared_tool_uses":3,"cleared_input_tokens":12}]},"usage":{"input_tokens":20,"output_tokens":2}}`))
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL
	mapping := `{"claude-opus-4-7":"claude-haiku-4-5"}`
	override := `{"anthropic-beta":"stripped"}`
	channel := &model.Channel{
		Type:           constant.ChannelTypeAnthropic,
		Key:            "sk-test",
		Name:           "audit",
		BaseURL:        &baseURL,
		ModelMapping:   &mapping,
		HeaderOverride: &override,
	}
	probe := &model.ChannelQualityProbe{
		Name:    "clear_tool_uses",
		Headers: `{"anthropic-beta":"context-management-2025-06-27","anthropic-version":"2023-06-01"}`,
		Body: `{
			"model":"{{model}}",
			"max_tokens":10,
			"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}
		}`,
		Assertions: `[{"name":"clear tool uses applied","expr":"any(context_management?.applied_edits ?? [], { .type == \"clear_tool_uses_20250919\" && int(.cleared_tool_uses) == 3 && float(.cleared_input_tokens) >= 1 })"}]`,
	}

	outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictPassed, outcome.Verdict)
	assert.Equal(t, "claude-opus-4-7", outcome.RequestedModel)
	assert.Equal(t, "claude-haiku-4-5", outcome.UpstreamModel)
	assert.Equal(t, "claude-haiku-4-5", seenModel)
	assert.True(t, seenContext)
	assert.Equal(t, "context-management-2025-06-27", seenBeta)
	assert.Contains(t, outcome.AssertionResults, `"passed":true`)
	assert.NotEmpty(t, outcome.UsageJSON)
}

func TestAuditAnthropicChannelFailsWhenEditDoesNotMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context_management":{"applied_edits":[{"type":"clear_tool_uses_20250919","cleared_tool_uses":1,"cleared_input_tokens":12}]}}`))
	}))
	t.Cleanup(server.Close)
	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeAnthropic,
		Key:     "sk-test",
		Name:    "audit",
		BaseURL: &baseURL,
	}
	probe := &model.ChannelQualityProbe{
		Headers:    `{"anthropic-beta":"context-management-2025-06-27"}`,
		Body:       `{"model":"{{model}}","max_tokens":10}`,
		Assertions: `[{"name":"count","expr":"any(context_management?.applied_edits ?? [], { int(.cleared_tool_uses) == 3 })"}]`,
	}
	outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-haiku-4-5", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictFailed, outcome.Verdict)
}

func TestQualityAuditUnsupportedChannelDoesNotCallUpstream(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQualityProbe{}, &model.ChannelQualityRun{}, &model.ChannelQualitySuiteRun{}))
	require.NoError(t, model.EnsureBuiltinChannelQualityProbe())
	require.NoError(t, model.EnsureBuiltinChannelQualityProbe())

	var probes []*model.ChannelQualityProbe
	require.NoError(t, db.Find(&probes).Error)
	require.Len(t, probes, 1)
	assert.True(t, probes[0].Builtin)
	assert.Less(t, len(probes[0].Body), 60*1024)
	_, err = qualityexpr.Compile(model.ChannelQualityClearToolUsesExpr)
	require.NoError(t, err)

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-test",
		Name:    "openai",
		BaseURL: &baseURL,
	}
	require.NoError(t, db.Create(channel).Error)
	run := &model.ChannelQualityRun{
		ChannelId: channel.Id,
		ProbeId:   probes[0].Id,
		ProbeName: probes[0].Name,
		Verdict:   model.ChannelQualityVerdictRunning,
	}
	require.NoError(t, model.CreateChannelQualityRun(run))

	older := &model.ChannelQualityRun{
		ChannelId: channel.Id,
		ProbeId:   probes[0].Id,
		ProbeName: probes[0].Name,
		Verdict:   model.ChannelQualityVerdictFailed,
		CreatedAt: 1,
	}
	newer := &model.ChannelQualityRun{
		ChannelId: channel.Id,
		ProbeId:   probes[0].Id,
		ProbeName: probes[0].Name,
		Verdict:   model.ChannelQualityVerdictPassed,
		CreatedAt: 2,
	}
	other := &model.ChannelQualityRun{
		ChannelId: channel.Id + 100,
		ProbeId:   probes[0].Id,
		ProbeName: probes[0].Name,
		Verdict:   model.ChannelQualityVerdictError,
		CreatedAt: 3,
	}
	require.NoError(t, model.CreateChannelQualityRun(older))
	require.NoError(t, model.CreateChannelQualityRun(newer))
	require.NoError(t, model.CreateChannelQualityRun(other))
	latest, err := model.ListLatestChannelQualityRuns()
	require.NoError(t, err)
	byChannel := map[int]string{}
	for _, item := range latest {
		byChannel[item.ChannelId] = item.Verdict
	}
	assert.Equal(t, model.ChannelQualityVerdictPassed, byChannel[channel.Id])
	assert.Equal(t, model.ChannelQualityVerdictError, byChannel[channel.Id+100])

	view, err := executeChannelQualityTask(context.Background(), channelQualityTaskPayload{
		ChannelID: channel.Id,
		ProbeID:   probes[0].Id,
		Model:     "gpt-4o-mini",
		RunID:     run.Id,
	})
	require.NoError(t, err)
	assert.False(t, called)
	assert.Equal(t, model.ChannelQualityVerdictUnsupported, view["verdict"])
}

func TestAuditAcceptsConfiguredResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	status := http.StatusBadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"rejected"}`))
	}))
	t.Cleanup(server.Close)
	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeAnthropic,
		Key:     "sk-test",
		Name:    "audit",
		BaseURL: &baseURL,
	}
	probe := &model.ChannelQualityProbe{
		Name:       "status",
		Headers:    `{}`,
		Body:       `{"model":"{{model}}","max_tokens":1}`,
		Statuses:   `[200,400]`,
		Assertions: `[{"name":"error present","expr":"notEmpty(error)"}]`,
	}

	outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictPassed, outcome.Verdict)

	probe.Assertions = `[]`
	outcome, err = auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictPassed, outcome.Verdict)

	status = http.StatusInternalServerError
	outcome, err = auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictFailed, outcome.Verdict)
	assert.Contains(t, outcome.Error, "upstream status 500")

	status = http.StatusServiceUnavailable
	outcome, err = auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictUnknown, outcome.Verdict)
	assert.Contains(t, outcome.Error, "upstream status 503")

	status = http.StatusTooManyRequests
	outcome, err = auditAnthropicChannel(context.Background(), channel, probe, "claude-opus-4-7", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictUnknown, outcome.Verdict)
}

func TestQualitySuiteVerdict(t *testing.T) {
	passed := model.ChannelQualityVerdictPassed
	failed := model.ChannelQualityVerdictFailed
	errored := model.ChannelQualityVerdictError
	unknown := model.ChannelQualityVerdictUnknown
	unsupported := model.ChannelQualityVerdictUnsupported
	skipped := model.ChannelQualityVerdictSkipped
	tests := []struct {
		name     string
		verdicts []string
		want     string
	}{
		{name: "empty", verdicts: nil, want: skipped},
		{name: "passed", verdicts: []string{passed, skipped}, want: passed},
		{name: "failed wins", verdicts: []string{passed, failed, errored}, want: failed},
		{name: "unknown is not a failure", verdicts: []string{passed, unknown}, want: unknown},
		{name: "error before pass", verdicts: []string{passed, errored}, want: errored},
		{name: "error before unsupported", verdicts: []string{unsupported, errored}, want: errored},
		{name: "unsupported", verdicts: []string{unsupported, skipped}, want: unsupported},
		{name: "all skipped", verdicts: []string{skipped, skipped}, want: skipped},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, qualitySuiteVerdict(test.verdicts))
		})
	}
}

func TestQualityScanSkipsModelTheChannelDoesNotServe(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)
	db := openQualityDB(t)

	probe := &model.ChannelQualityProbe{
		Name: "ping", Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1}`, Assertions: `[]`,
	}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "nightly"}
	require.NoError(t, model.SaveChannelQualitySuite(suite, []*model.ChannelQualitySuiteItem{{
		ProbeId: probe.Id, Model: "claude-sonnet-4-5",
	}}))
	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeAnthropic, Key: "sk-test", Name: "audit", Status: common.ChannelStatusEnabled,
		BaseURL: &baseURL, Models: "claude-haiku-4-5", QualityAutoEnabled: true, QualitySuiteId: suite.Id,
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQualityScan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary["skipped"])
	assert.False(t, called)

	suiteRun, items, err := model.GetLatestChannelQualitySuiteRun(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, suiteRun)
	assert.Equal(t, model.ChannelQualityVerdictSkipped, suiteRun.Verdict)
	require.Len(t, items, 1)
	assert.Equal(t, "channel does not serve this model", items[0].Error)
}

func TestQualityScanFailureDoesNotChangeChannelStatus(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	fetch := system_setting.GetFetchSetting()
	previousProtection := fetch.EnableSSRFProtection
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { fetch.EnableSSRFProtection = previousProtection })
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	webhookCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notify" {
			webhookCalls++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	db := openQualityDB(t)
	probe := &model.ChannelQualityProbe{
		Name: "missing", Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1}`,
		Assertions: `[{"name":"present","expr":"notEmpty(missing)"}]`,
	}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "nightly"}
	require.NoError(t, model.SaveChannelQualitySuite(suite, []*model.ChannelQualitySuiteItem{{
		ProbeId: probe.Id, Model: "claude-sonnet-4-5",
	}}))
	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeAnthropic, Key: "sk-test", Name: "audit", Status: common.ChannelStatusEnabled,
		BaseURL: &baseURL, Models: "claude-sonnet-4-5", QualityAutoEnabled: true, QualitySuiteId: suite.Id,
	}
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, service.SaveQualityWebhookConfig(service.QualityWebhookConfig{Enabled: true, URL: server.URL + "/notify", CooldownSeconds: 600}))
	service.ClearQualityWebhookCooldown(channel.Id, suite.Id)
	t.Cleanup(func() { service.ClearQualityWebhookCooldown(channel.Id, suite.Id) })
	summary, err := runChannelQualityScan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary["failed"])
	assert.Equal(t, 1, summary["webhook_delivered"])
	assert.Equal(t, 1, webhookCalls)

	// A status mismatch fails before assertion evaluation and must also notify.
	require.NoError(t, db.Model(probe).Updates(map[string]any{"statuses": "[429]", "assertions": "[]"}).Error)
	service.ClearQualityWebhookCooldown(channel.Id, suite.Id)
	summary, err = runChannelQualityScan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary["failed"])
	assert.Equal(t, 1, summary["webhook_delivered"])
	assert.Equal(t, 2, webhookCalls)

	saved := &model.Channel{}
	require.NoError(t, db.First(saved, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, saved.Status)
	assert.True(t, saved.QualityAutoEnabled)
}

func TestQualitySchedulePersistsDisabledSwitch(t *testing.T) {
	db := openQualityDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeAnthropic, Key: "sk-test", Name: "audit", Status: common.ChannelStatusEnabled,
		QualityAutoEnabled: true, QualitySuiteId: 7,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, model.SaveChannelQualitySchedule(channel.Id, false, 0))

	saved := &model.Channel{}
	require.NoError(t, db.First(saved, channel.Id).Error)
	assert.False(t, saved.QualityAutoEnabled)
	assert.Equal(t, 0, saved.QualitySuiteId)
}

func TestQualitySuiteDeleteGuards(t *testing.T) {
	db := openQualityDB(t)
	probe := &model.ChannelQualityProbe{Name: "ping", Headers: `{}`, Body: `{"model":"m"}`, Assertions: `[]`}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "nightly"}
	require.NoError(t, model.SaveChannelQualitySuite(suite, []*model.ChannelQualitySuiteItem{{
		ProbeId: probe.Id, Model: "claude-sonnet-4-5",
	}}))
	require.ErrorContains(t, model.DeleteChannelQualityProbe(probe.Id), "probe is used by a quality suite")

	channel := &model.Channel{
		Type: constant.ChannelTypeAnthropic, Key: "sk-test", Name: "audit", QualitySuiteId: suite.Id,
	}
	require.NoError(t, db.Create(channel).Error)
	require.ErrorContains(t, model.DeleteChannelQualitySuite(suite.Id), "suite is used by a channel")
}

func TestQualityScanAWSAPIKeyIsUnsupported(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)
	db := openQualityDB(t)
	probe := &model.ChannelQualityProbe{Name: "ping", Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1}`, Assertions: `[]`}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "aws"}
	require.NoError(t, model.SaveChannelQualitySuite(suite, []*model.ChannelQualitySuiteItem{{
		ProbeId: probe.Id, Model: "claude-sonnet-4-5",
	}}))
	baseURL := server.URL
	channel := &model.Channel{
		Type: constant.ChannelTypeAws, Key: "test-key|us-east-1", Name: "aws", Status: common.ChannelStatusEnabled,
		BaseURL: &baseURL, Models: "claude-sonnet-4-5", QualityAutoEnabled: true, QualitySuiteId: suite.Id,
		OtherSettings: `{"aws_key_type":"api_key","aws_api_type":"invoke_model"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	summary, err := runChannelQualityScan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary["unsupported"])
	assert.False(t, called)
	suiteRun, items, err := model.GetLatestChannelQualitySuiteRun(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, suiteRun)
	assert.Equal(t, model.ChannelQualityVerdictUnsupported, suiteRun.Verdict)
	assert.Equal(t, "this AWS channel mode cannot run a Messages probe", suiteRun.Error)
	assert.Empty(t, items)
}

func TestAuditAWSMantleSendsMessagesProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var seenModel string
	var seenBeta string
	var seenKey string
	var seenPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		payload := map[string]any{}
		require.NoError(t, common.Unmarshal(body, &payload))
		seenModel, _ = payload["model"].(string)
		seenBeta = r.Header.Get("anthropic-beta")
		seenKey = r.Header.Get("x-api-key")
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL
	setting := `{"tls_insecure_skip_verify":true}`
	override := `{"anthropic-beta":"stripped"}`
	channel := &model.Channel{
		Type:           constant.ChannelTypeAws,
		Key:            "test-key|us-east-1",
		Name:           "mantle",
		BaseURL:        &baseURL,
		Setting:        &setting,
		HeaderOverride: &override,
		OtherSettings:  `{"aws_api_type":"mantle"}`,
	}
	probe := &model.ChannelQualityProbe{
		Name:       "mantle",
		Headers:    `{"anthropic-beta":"context-management-2025-06-27"}`,
		Body:       `{"model":"{{model}}","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`,
		Assertions: `[{"name":"id","expr":"notEmpty(id)"}]`,
	}
	outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-sonnet-4-5", nil)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictPassed, outcome.Verdict, outcome.Error)
	assert.Equal(t, "anthropic.claude-sonnet-4-5", seenModel)
	assert.Equal(t, "context-management-2025-06-27", seenBeta)
	assert.Equal(t, "test-key", seenKey)
	assert.Contains(t, seenPath, "/anthropic/v1/messages")
}

func TestQualityColumnsMigrateOntoExistingSQLiteChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE channels (
		id integer PRIMARY KEY,
		type integer DEFAULT 0,
		`+"`key`"+` text NOT NULL,
		status integer DEFAULT 1,
		name text
	)`).Error)
	require.NoError(t, db.Exec("INSERT INTO channels (type, `key`, status, name) VALUES (14, 'sk-test', 1, 'old')").Error)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	saved := &model.Channel{}
	require.NoError(t, db.First(saved, 1).Error)
	assert.False(t, saved.QualityAutoEnabled)
	assert.Equal(t, 0, saved.QualitySuiteId)
	assert.Equal(t, "old", saved.Name)
}

func TestListChannelQualityRunsPagesNewestFirst(t *testing.T) {
	db := openQualityDB(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeAnthropic, Key: "sk-test", Name: "audit", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)
	for _, name := range []string{"older", "middle", "newest"} {
		require.NoError(t, model.CreateChannelQualityRun(&model.ChannelQualityRun{
			ChannelId: channel.Id, ProbeName: name, Verdict: model.ChannelQualityVerdictPassed,
		}))
	}
	page, total, err := model.ListChannelQualityRuns(channel.Id, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, page, 2)
	assert.Equal(t, "newest", page[0].ProbeName)
	assert.Equal(t, "middle", page[1].ProbeName)
	rest, total, err := model.ListChannelQualityRuns(channel.Id, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, rest, 1)
	assert.Equal(t, "older", rest[0].ProbeName)
}

func TestQualityModelConflictDoesNotSendAnotherModel(t *testing.T) {
	body := map[string]any{"model": "claude-fixed"}
	_, err := applyQualityProbeModel(body, "claude-selected")
	require.Error(t, err)
	assert.Equal(t, "claude-fixed", body["model"])
	actual, err := applyQualityProbeModel(body, "claude-fixed")
	require.NoError(t, err)
	assert.Equal(t, "claude-fixed", actual)
	actual, err = applyQualityProbeModel(body, "")
	require.NoError(t, err)
	assert.Equal(t, "claude-fixed", actual)
}

func TestQualityResponseCapture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       []byte
		assertions []model.ChannelQualityAssertion
		verdict    string
		truncated  bool
	}{
		{name: "JSON success", status: 200, body: []byte(" {\"content\":[{\"text\":\"你好\"}]}\n"), verdict: model.ChannelQualityVerdictPassed},
		{name: "non JSON error", status: 503, body: []byte("<html>unavailable</html>"), verdict: model.ChannelQualityVerdictUnknown},
		{name: "invalid JSON", status: 200, body: []byte("not JSON"), assertions: []model.ChannelQualityAssertion{{Name: "check", Expr: "true"}}, verdict: model.ChannelQualityVerdictError},
		{name: "binary bytes", status: 200, body: []byte{0, 255, 254, 'a'}, verdict: model.ChannelQualityVerdictPassed},
		{name: "empty response", status: 200, body: nil, verdict: model.ChannelQualityVerdictPassed},
		{name: "exact size limit", status: 200, body: bytes.Repeat([]byte("a"), maxQualityResponseBytes), verdict: model.ChannelQualityVerdictPassed},
		{name: "over size limit", status: 200, body: bytes.Repeat([]byte("a"), maxQualityResponseBytes+1), verdict: model.ChannelQualityVerdictError, truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := finishQualityResponse(qualityAuditOutcome{Verdict: model.ChannelQualityVerdictError}, []int{200}, tc.assertions, tc.status, tc.body)
			require.NotNil(t, outcome.Response)
			assert.Equal(t, tc.verdict, outcome.Verdict)
			assert.Equal(t, tc.status, outcome.Response.StatusCode)
			assert.Equal(t, tc.body[:min(len(tc.body), maxQualityResponseBytes)], outcome.Response.Body)
			assert.Equal(t, tc.truncated, outcome.Response.Truncated)
		})
	}
}

func TestQualityResponseFormattingPreservesNumbersAndBoundsPreview(t *testing.T) {
	formatted := formatQualityResponsePreview([]byte(`{"value":9007199254740993,"escaped":"[\\\""}`))
	require.NotNil(t, formatted)
	assert.Contains(t, *formatted, "9007199254740993")
	assert.Nil(t, formatQualityResponsePreview([]byte(strings.Repeat("[", 17)+"0"+strings.Repeat("]", 17))))
	assert.Nil(t, formatQualityResponsePreview(bytes.Repeat([]byte("x"), 65537)))
	assert.Nil(t, formatQualityResponsePreview([]byte("not JSON")))
}

type qualityHTTPTransport func(*http.Request) (*http.Response, error)

func (transport qualityHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestQualityAWSArchivesRawSDKResponses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		truncated bool
	}{
		{name: "success", status: 200, body: " {\"content\":[]}\n"},
		{name: "error before SDK deserialization", status: 400, body: " {\"message\":\"invalid model\",\"extra\":\"original response\"}\n"},
		{name: "oversized", status: 200, body: strings.Repeat("a", maxQualityResponseBytes+1), truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := service.GetHttpClientWithProxy("")
			require.NoError(t, err)
			previous := client.Transport
			t.Cleanup(func() { client.Transport = previous })
			client.Transport = qualityHTTPTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{"application/json"}, "X-Amzn-Errortype": []string{"ValidationException"}}, Body: io.NopCloser(strings.NewReader(tc.body)), ContentLength: int64(len(tc.body)), Request: request}, nil
			})
			channel := &model.Channel{Type: constant.ChannelTypeAws, Key: "test-access|test-secret|us-east-1", Name: "aws"}
			probe := &model.ChannelQualityProbe{Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`}
			outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-sonnet-4-5", nil)
			require.NoError(t, err)
			require.NotNil(t, outcome.Response, outcome.Error)
			assert.Equal(t, tc.status, outcome.Response.StatusCode)
			assert.Equal(t, tc.body[:min(len(tc.body), maxQualityResponseBytes)], string(outcome.Response.Body))
			assert.Equal(t, tc.truncated, outcome.Response.Truncated)
		})
	}
}

func TestQualityArchivesInterruptedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("partial response"))
	}))
	t.Cleanup(server.Close)
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeAnthropic, Key: "test", BaseURL: &baseURL}
	probe := &model.ChannelQualityProbe{Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1}`}
	outcome, err := auditAnthropicChannel(context.Background(), channel, probe, "claude-sonnet-4-5", nil)
	require.NoError(t, err)
	require.NotNil(t, outcome.Response)
	assert.Equal(t, "partial response", string(outcome.Response.Body))
	assert.True(t, outcome.Response.Incomplete)
	assert.Equal(t, model.ChannelQualityVerdictError, outcome.Verdict)
}

func TestQualityResponseSaveFailureFinishesSuiteAndProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	t.Cleanup(server.Close)
	db := openQualityDB(t)
	probe := &model.ChannelQualityProbe{Name: "archive", Headers: `{}`, Body: `{"model":"{{model}}","max_tokens":1}`}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "archive"}
	require.NoError(t, model.SaveChannelQualitySuite(suite, []*model.ChannelQualitySuiteItem{{ProbeId: probe.Id, Model: "claude-sonnet-4-5"}}))
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeAnthropic, Key: "test", Name: "archive", BaseURL: &baseURL, Models: "claude-sonnet-4-5", QualitySuiteId: suite.Id}
	require.NoError(t, db.Create(channel).Error)
	manual := &model.ChannelQualityRun{ChannelId: channel.Id, ProbeId: probe.Id, ProbeName: probe.Name, Verdict: model.ChannelQualityVerdictRunning}
	require.NoError(t, model.CreateChannelQualityRun(manual))
	_, err := executeChannelQualityTask(context.Background(), channelQualityTaskPayload{ChannelID: channel.Id, ProbeID: probe.Id, RunID: manual.Id, Model: "claude-sonnet-4-5"})
	require.NoError(t, err)
	evidence, err := model.GetChannelQualityResponse(channel.Id, manual.Id)
	require.NoError(t, err)
	require.NotNil(t, evidence)
	assert.Equal(t, `{"content":[]}`, string(evidence.Body))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("quality_archive_failure", func(write *gorm.DB) {
		if _, ok := write.Statement.Dest.(*model.ChannelQualityResponse); ok {
			write.AddError(errors.New("storage unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("quality_archive_failure") })
	_, err = runChannelQualitySuite(context.Background(), channel)
	require.ErrorContains(t, err, "storage unavailable")
	suiteRun, items, err := model.GetLatestChannelQualitySuiteRun(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelQualityVerdictError, suiteRun.Verdict)
	require.Len(t, items, 1)
	assert.Equal(t, model.ChannelQualityVerdictError, items[0].Verdict)
}

func TestQualityDatabaseRegression(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db := openQualityDB(t, driver)

			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
			})
			cfg := service.QualityWebhookConfig{Enabled: true, URL: "https://example.com/quality", Secret: "test-signing-secret", CooldownSeconds: 600}
			require.NoError(t, service.SaveQualityWebhookConfig(cfg))
			storedConfig, err := service.GetQualityWebhookConfig()
			require.NoError(t, err)
			assert.Equal(t, cfg, storedConfig)
			var persistedOption model.Option
			require.NoError(t, db.Where(&model.Option{Key: service.QualityWebhookOptionKey}).First(&persistedOption).Error)
			var persistedConfig service.QualityWebhookConfig
			require.NoError(t, common.UnmarshalJsonStr(persistedOption.Value, &persistedConfig))
			assert.Equal(t, cfg, persistedConfig)
			configRecorder := httptest.NewRecorder()
			configContext, _ := gin.CreateTestContext(configRecorder)
			GetQualityWebhookConfig(configContext)
			assert.NotContains(t, configRecorder.Body.String(), cfg.Secret)
			assert.Contains(t, configRecorder.Body.String(), `"secret_configured":true`)
			updateContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			updateContext.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"enabled":false,"url":"https://example.com/quality","cooldown_seconds":60}`))
			UpdateQualityWebhookConfig(updateContext)
			storedConfig, err = service.GetQualityWebhookConfig()
			require.NoError(t, err)
			assert.Equal(t, cfg.Secret, storedConfig.Secret)
			assert.False(t, storedConfig.Enabled)
			require.NoError(t, db.Migrator().DropTable(&model.ChannelQualityResponse{}))
			// Upgrade the pre-response schema with existing history, then repeat
			// migration to verify idempotency and preservation of that history.
			legacy := &model.ChannelQualityRun{ChannelId: 9000, ProbeName: "legacy audit", Verdict: model.ChannelQualityVerdictPassed}
			require.NoError(t, db.Create(legacy).Error)
			t.Cleanup(func() { require.NoError(t, db.Delete(legacy).Error) })
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.ChannelQualityResponse{}))
			}
			var preserved model.ChannelQualityRun
			require.NoError(t, db.First(&preserved, legacy.Id).Error)
			assert.Equal(t, legacy.ProbeName, preserved.ProbeName)
			assert.Equal(t, legacy.Verdict, preserved.Verdict)
			historical, err := model.GetChannelQualityResponse(legacy.ChannelId, legacy.Id)
			require.NoError(t, err)
			assert.Nil(t, historical)
			require.NoError(t, db.Delete(legacy).Error)
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("verified %s %s", dialect, version)
			tx := db.Begin()
			require.NoError(t, tx.Error)
			model.DB = tx
			t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
			latest, err := model.ListLatestChannelQualityRuns()
			require.NoError(t, err)
			assert.Empty(t, latest)
			probe := &model.ChannelQualityProbe{Name: "quality-regression", Body: `{"model":"{{model}}"}`}
			require.NoError(t, tx.Create(probe).Error)
			require.NoError(t, tx.Model(probe).Update("body", `{"model":"claude-fixed"}`).Error)
			payload, err := common.Marshal(qualitySuitePayload{Name: "model-conflict", Items: []qualitySuiteItemPayload{{ProbeID: probe.Id, Model: "claude-selected"}}})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(payload)))
			_, _, err = bindQualitySuite(c, 0)
			require.EqualError(t, err, "probe model does not match the selected model")
			suite := &model.ChannelQualitySuiteRun{ChannelId: 9001, Verdict: model.ChannelQualityVerdictFailed, CreatedAt: 100}
			require.NoError(t, model.CreateChannelQualitySuiteRun(suite))
			for _, verdict := range []string{model.ChannelQualityVerdictFailed, model.ChannelQualityVerdictPassed} {
				require.NoError(t, model.CreateChannelQualityRun(&model.ChannelQualityRun{ChannelId: 9001, SuiteRunId: suite.Id, Verdict: verdict, CreatedAt: 100}))
			}
			latest, err = model.ListLatestChannelQualityRuns()
			require.NoError(t, err)
			require.Len(t, latest, 1)
			assert.Equal(t, model.ChannelQualityVerdictFailed, latest[0].Verdict, "a passing final probe must not hide a failed suite")
			require.NoError(t, model.CreateChannelQualityRun(&model.ChannelQualityRun{ChannelId: 9001, Verdict: model.ChannelQualityVerdictPassed, CreatedAt: 101}))
			latest, err = model.ListLatestChannelQualityRuns()
			require.NoError(t, err)
			require.Len(t, latest, 1)
			assert.Equal(t, model.ChannelQualityVerdictPassed, latest[0].Verdict, "a later standalone audit supersedes the suite")
			newSuite := &model.ChannelQualitySuiteRun{ChannelId: 9001, Verdict: model.ChannelQualityVerdictUnsupported, CreatedAt: 102}
			require.NoError(t, model.CreateChannelQualitySuiteRun(newSuite))
			latest, err = model.ListLatestChannelQualityRuns()
			require.NoError(t, err)
			require.Len(t, latest, 1)
			assert.Equal(t, model.ChannelQualityVerdictUnsupported, latest[0].Verdict, "a suite without child requests still has a result")
			verdict, err := executeQualitySuiteItem(context.Background(), &model.Channel{Id: 9002, Models: "another-model"}, &model.ChannelQualitySuiteItem{ProbeId: probe.Id, Model: "not-served"}, suite.Id)
			require.NoError(t, err)
			assert.Equal(t, model.ChannelQualityVerdictSkipped, verdict)
			run, err := model.GetLatestChannelQualityRun(9002)
			require.NoError(t, err)
			require.NotNil(t, run)
			assert.Equal(t, probe.Name, run.ProbeName)
			// The full binary response is archived atomically with its result,
			// including payloads larger than MySQL's ordinary 64 KiB BLOB.
			run.Verdict = model.ChannelQualityVerdictPassed
			body := bytes.Repeat([]byte{0, 255, 254, 'x'}, maxQualityResponseBytes/4)
			evidence := &model.ChannelQualityResponse{StatusCode: 200, Body: body}
			require.NoError(t, model.SaveChannelQualityResult(run, evidence))
			stored, err := model.GetChannelQualityResponse(run.ChannelId, run.Id)
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, body, stored.Body)
			assert.Equal(t, 200, stored.StatusCode)
			evidence.Truncated, evidence.Incomplete = true, true
			require.NoError(t, model.SaveChannelQualityResult(run, evidence))
			stored, err = model.GetChannelQualityResponse(run.ChannelId, run.Id)
			require.NoError(t, err)
			assert.True(t, stored.Truncated)
			assert.True(t, stored.Incomplete)
			_, err = model.GetChannelQualityResponse(run.ChannelId+1, run.Id)
			require.Error(t, err)
			// Exercise the public detail contract; summaries must not include it.
			engine := gin.New()
			engine.GET("/:id/quality/runs/:runId/response", GetChannelQualityResponse)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d/quality/runs/%d/response", run.ChannelId, run.Id), nil))
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			var detail struct {
				Data struct {
					Body string `json:"body_base64"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &detail))
			decoded, err := base64.StdEncoding.DecodeString(detail.Data.Body)
			require.NoError(t, err)
			assert.Equal(t, body, decoded)
			assert.NotContains(t, qualityRunView(run), "body_base64")
			for _, path := range []string{fmt.Sprintf("/%d/quality/runs/%d/response", run.ChannelId+1, run.Id), fmt.Sprintf("/%d/quality/runs/999999/response", run.ChannelId)} {
				recorder = httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				assert.Equal(t, http.StatusNotFound, recorder.Code)
			}
			for _, path := range []string{"/0/quality/runs/1/response", "/1/quality/runs/invalid/response"} {
				recorder = httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				assert.Equal(t, http.StatusBadRequest, recorder.Code)
			}
			require.NoError(t, tx.Callback().Create().Before("gorm:create").Register("quality_response_failure", func(write *gorm.DB) {
				if _, ok := write.Statement.Dest.(*model.ChannelQualityResponse); ok {
					write.AddError(errors.New("response storage unavailable"))
				}
			}))
			t.Cleanup(func() { _ = tx.Callback().Create().Remove("quality_response_failure") })
			run.Verdict = model.ChannelQualityVerdictFailed
			require.ErrorContains(t, model.SaveChannelQualityResult(run, evidence), "response storage unavailable")
			require.NoError(t, tx.Callback().Create().Remove("quality_response_failure"))
			persisted, err := model.GetLatestChannelQualityRun(run.ChannelId)
			require.NoError(t, err)
			assert.Equal(t, model.ChannelQualityVerdictPassed, persisted.Verdict, "evidence failure must roll back the verdict")
		})
	}
}

func openQualityDB(t *testing.T, drivers ...gorm.Dialector) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector = sqlite.Open(":memory:")
	if len(drivers) > 0 {
		driver = drivers[0]
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Option{},
		&model.ChannelQualityProbe{},
		&model.ChannelQualityRun{},
		&model.ChannelQualityResponse{},
		&model.ChannelQualitySuite{},
		&model.ChannelQualitySuiteItem{},
		&model.ChannelQualitySuiteRun{},
	))
	return db
}

func TestQualityWebhookNotifiesFailedProbes(t *testing.T) {
	channel := &model.Channel{Id: 44, Name: "quality"}
	suite := &model.ChannelQualitySuiteRun{Id: 45, SuiteId: 46, SuiteName: "suite"}
	for _, test := range []struct {
		name, verdict, assertions string
		alert                     bool
	}{
		{name: "passed", verdict: "passed", assertions: `[{"name":"ok","passed":true}]`},
		{name: "status mismatch", verdict: "failed", assertions: "", alert: true},
		{name: "runtime error", verdict: "failed", assertions: `[{"name":"broken","passed":false,"error":"missing field"}]`, alert: true},
		{name: "malformed result", verdict: "failed", assertions: `[{"name":"missing result"}]`, alert: true},
		{name: "execution error", verdict: "error"},
		{name: "unsupported", verdict: "unsupported"},
		{name: "skipped", verdict: "skipped"},
		{name: "unknown", verdict: "unknown", assertions: ""},
		{name: "assertion failure", verdict: "failed", assertions: `[{"name":"expected tools","passed":false}]`, alert: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			alert := service.BuildQualityWebhookAlert(channel, suite, []*model.ChannelQualityRun{{Id: 47, Verdict: test.verdict, AssertionResults: test.assertions, Error: "private response body", ProbeSnapshot: "secret header"}})
			if !test.alert {
				assert.Nil(t, alert)
				return
			}
			require.NotNil(t, alert)
			require.Len(t, alert.Failures, 1)
			if test.name == "assertion failure" {
				assert.Equal(t, []string{"expected tools"}, alert.Failures[0].Assertions)
			} else {
				assert.Empty(t, alert.Failures[0].Assertions)
			}
			encoded, err := common.Marshal(alert)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "private response body")
			assert.NotContains(t, string(encoded), "secret header")
		})
	}
}

func TestQualityWebhookDeliverySignsRetriesAndAppliesCooldown(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	fetch := system_setting.GetFetchSetting()
	previousProtection := fetch.EnableSSRFProtection
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { fetch.EnableSSRFProtection = previousProtection })
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	calls := 0
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		signature := hmac.New(sha256.New, []byte("test-secret"))
		_, err = signature.Write(body)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(signature.Sum(nil)), r.Header.Get("X-Webhook-Signature"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, string(body), `"type":"quality_assertion_failed"`)
		calls++
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	cfg := service.QualityWebhookConfig{Enabled: true, URL: server.URL, Secret: "test-secret", CooldownSeconds: 600}
	encoded, err := common.Marshal(cfg)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[service.QualityWebhookOptionKey] = string(encoded)
	common.OptionMapRWMutex.Unlock()
	alerts := []service.QualityWebhookAlert{{EventID: "quality-suite-1", ChannelID: 8401, SuiteID: 8402, Failures: []service.QualityWebhookFailure{{Assertions: []string{"expected output"}}}}}
	service.ClearQualityWebhookCooldown(8401, 8402)
	t.Cleanup(func() { service.ClearQualityWebhookCooldown(8401, 8402) })
	count, err := service.DeliverQualityWebhooks(context.Background(), alerts)
	require.Error(t, err)
	assert.Zero(t, count)
	assert.Equal(t, 2, calls)
	fail = false
	count, err = service.DeliverQualityWebhooks(context.Background(), alerts)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 3, calls)
	count, err = service.DeliverQualityWebhooks(context.Background(), alerts)
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Equal(t, 3, calls)
	service.ClearQualityWebhookCooldown(8401, 8402)
	count, err = service.DeliverQualityWebhooks(context.Background(), alerts)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 4, calls)
}

func TestQualityWebhookFeishuSignature(t *testing.T) {
	for _, host := range []string{"open.feishu.cn", "open.larksuite.com"} {
		t.Run(host, func(t *testing.T) {
			cfg := service.QualityWebhookConfig{URL: "https://" + host + "/open-apis/bot/v2/hook/test", Secret: "test-secret"}
			notify := dto.NewNotify("quality_assertion_failed", "Probe alert", "Probe failed", nil)
			body, err := service.BuildQualityWebhookPayload(cfg, notify, 1599360473)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, "1599360473", payload["timestamp"])
			assert.Equal(t, "wSds2BzzFIIGf/WrhUO+NI1q/9j+FRJd3JNHKAq0NZY=", payload["sign"])
			assert.Equal(t, "interactive", payload["msg_type"])
			assert.NotContains(t, string(body), cfg.Secret)
			later, err := service.BuildQualityWebhookPayload(cfg, notify, 1599360474)
			require.NoError(t, err)
			assert.NotEqual(t, body, later)
			cfg.Secret = ""
			body, err = service.BuildQualityWebhookPayload(cfg, notify, 1599360473)
			require.NoError(t, err)
			payload = nil
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.NotContains(t, payload, "sign")
			assert.NotContains(t, payload, "timestamp")
		})
	}
}

type qualityWebhookRoundTripper func(*http.Request) (*http.Response, error)

func (transport qualityWebhookRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestQualityWebhookFeishuRejectionsAreSafeAndRetryable(t *testing.T) {
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	fetch := system_setting.GetFetchSetting()
	previousProtection := fetch.EnableSSRFProtection
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { fetch.EnableSSRFProtection = previousProtection })
	client := service.GetSSRFProtectedHTTPClient()
	originalTransport := client.Transport
	t.Cleanup(func() { client.Transport = originalTransport })
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	cfg := service.QualityWebhookConfig{Enabled: true, URL: "https://open.feishu.cn/open-apis/bot/v2/hook/private-hook", Secret: "private-secret", CooldownSeconds: 60}
	encoded, err := common.Marshal(cfg)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[service.QualityWebhookOptionKey] = string(encoded)
	common.OptionMapRWMutex.Unlock()
	alerts := []service.QualityWebhookAlert{{ChannelID: 8501, SuiteID: 8502}}
	t.Cleanup(func() { service.ClearQualityWebhookCooldown(8501, 8502) })
	for _, test := range []struct{ body, wantError string }{
		{`{"code":19021,"msg":"private-secret private-hook"}`, "code=19021"},
		{`{"StatusCode":19024,"StatusMessage":"private-secret"}`, "code=19024"},
		{`{"msg":"private-hook"}`, "invalid Feishu webhook response"},
		{`{"code":0,"msg":"success"}`, ""},
	} {
		t.Run(test.wantError, func(t *testing.T) {
			calls := 0
			client.Transport = qualityWebhookRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				assert.Empty(t, request.Header.Get("X-Webhook-Signature"))
				var payload map[string]any
				require.NoError(t, common.DecodeJson(request.Body, &payload))
				assert.NotEmpty(t, payload["sign"])
				assert.NotEmpty(t, payload["timestamp"])
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})
			count, err := service.DeliverQualityWebhooks(context.Background(), alerts)
			if test.wantError == "" {
				require.NoError(t, err)
				assert.Equal(t, 1, count)
				assert.Equal(t, 1, calls)
				return
			}
			require.ErrorContains(t, err, test.wantError)
			assert.NotContains(t, err.Error(), "private-secret")
			assert.NotContains(t, err.Error(), "private-hook")
			assert.Equal(t, 0, count)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestQualityWebhookFormatsChannelSuiteProbeAndSafeFailureReason(t *testing.T) {
	require.NoError(t, i18n.Init())
	alert := service.BuildQualityWebhookAlert(
		&model.Channel{Id: 22, Name: "aws-test_copy"},
		&model.ChannelQualitySuiteRun{Id: 1018, SuiteId: 7, SuiteName: "aws质量测试"},
		[]*model.ChannelQualityRun{{Id: 2001, ProbeId: 9, ProbeName: "structured-outputs", RequestedModel: "claude-opus-4-8", UpstreamModel: "upstream-model", Verdict: "failed", Error: "upstream status 200: private upstream body", ProbeSnapshot: `{"statuses":[429],"headers":{"Authorization":"private credential"}}`}},
	)
	require.NotNil(t, alert)
	assert.Equal(t, 200, alert.Failures[0].ActualStatus)
	assert.Equal(t, []int{429}, alert.Failures[0].ExpectedStatuses)
	notify := service.FormatQualityWebhookNotification([]service.QualityWebhookAlert{*alert}, "zh-CN")
	assert.Equal(t, "质量审计失败告警", notify.Title)
	for _, detail := range []string{"渠道 ID：22", "渠道名称：aws-test_copy", "失败方案：aws质量测试（ID：7）", "失败探针：structured-outputs（ID：9）", "测试模型：claude-opus-4-8", "上游模型：upstream-model", "预期 [429]，实际 200", "#1018", "#2001"} {
		assert.Contains(t, notify.Content, detail)
	}
	assert.NotContains(t, notify.Content, notify.Title, "title must not repeat in the body")
	assert.NotContains(t, notify.Content, "private")
	body, err := service.BuildQualityWebhookPayload(service.QualityWebhookConfig{URL: "https://open.feishu.cn/open-apis/bot/v2/hook/test"}, notify, 1599360473)
	require.NoError(t, err)
	var card struct {
		MsgType string `json:"msg_type"`
		Card    struct {
			Header struct {
				Template string `json:"template"`
			} `json:"header"`
			Elements []struct {
				Tag  string `json:"tag"`
				Text *struct {
					Tag     string `json:"tag"`
					Content string `json:"content"`
				} `json:"text"`
			} `json:"elements"`
		} `json:"card"`
	}
	require.NoError(t, common.Unmarshal(body, &card))
	assert.Equal(t, "interactive", card.MsgType)
	assert.Equal(t, "red", card.Card.Header.Template)
	for _, element := range card.Card.Elements {
		if element.Text != nil {
			assert.Equal(t, "plain_text", element.Text.Tag)
		}
	}
	alert.Failures[0].ActualStatus = 0
	alert.Failures[0].Assertions = []string{"tools applied"}
	alert.Failures[0].OmittedAssertions = 2
	alert.OmittedFailures = 3
	notify = service.FormatQualityWebhookNotification([]service.QualityWebhookAlert{*alert}, "zh-CN")
	assert.Contains(t, notify.Content, "断言未通过：tools applied")
	assert.Contains(t, notify.Content, "另有 2 条失败断言")
	assert.Contains(t, notify.Content, "另有 3 个失败探针")
}

func TestQualityWebhookDefaultsToChineseAndHonorsExplicitLanguage(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousLanguage := common.DefaultLanguage
	t.Cleanup(func() { common.DefaultLanguage = previousLanguage })
	for _, test := range []struct {
		name, configured, requested, title, summary string
	}{
		{"default Chinese", "", "", "质量审计失败告警", "本批次有 0 个渠道方案检测失败"},
		{"configured English", "en", "", "Quality audit failure alert", "Failed channel/suite results: 0"},
		{"explicit traditional Chinese", "en", "zh-TW", "品質稽核失敗告警", "本批次有 0 個渠道方案檢測失敗"},
	} {
		t.Run(test.name, func(t *testing.T) {
			common.DefaultLanguage = test.configured
			notify := service.FormatQualityWebhookNotification(nil, test.requested)
			assert.Equal(t, test.title, notify.Title)
			assert.Equal(t, test.summary, notify.Content)
		})
	}
}

func TestQualityWebhookCardKeepsEachChannelAndItsProbesTogether(t *testing.T) {
	require.NoError(t, i18n.Init())
	alerts := []service.QualityWebhookAlert{
		{ChannelID: 22, ChannelName: "aws-copy\n\n<at id=all>**[link](https://example.com)</at>", SuiteID: 1, SuiteName: "suite A", Failures: []service.QualityWebhookFailure{{ProbeName: "first probe", Model: "model A"}, {ProbeName: "second probe", Model: "model B"}}},
		{ChannelID: 23, ChannelName: "another channel", SuiteID: 2, SuiteName: "suite B", Failures: []service.QualityWebhookFailure{{ProbeName: "third probe", Model: "model C"}}},
	}
	notify := service.FormatQualityWebhookNotification(alerts, "zh-CN")
	body, err := service.BuildQualityWebhookPayload(service.QualityWebhookConfig{URL: "https://open.feishu.cn/open-apis/bot/v2/hook/test"}, notify, 1599360473)
	require.NoError(t, err)
	var payload struct {
		Card struct {
			Elements []struct {
				Tag             string `json:"tag"`
				BackgroundStyle string `json:"background_style"`
				Columns         []struct {
					Elements []struct {
						Text struct {
							Tag     string `json:"tag"`
							Content string `json:"content"`
						} `json:"text"`
					} `json:"elements"`
				} `json:"columns"`
			} `json:"elements"`
		} `json:"card"`
	}
	require.NoError(t, common.Unmarshal(body, &payload))
	groups := 0
	for index, element := range payload.Card.Elements {
		if element.Tag != "column_set" {
			continue
		}
		require.Greater(t, index, 0)
		assert.Equal(t, "hr", payload.Card.Elements[index-1].Tag)
		assert.Equal(t, "grey", element.BackgroundStyle)
		require.Len(t, element.Columns, 1)
		sections := element.Columns[0].Elements
		require.Len(t, sections, 2)
		assert.Equal(t, "lark_md", sections[0].Text.Tag)
		assert.Contains(t, sections[0].Text.Content, "**渠道 ID：")
		assert.NotContains(t, sections[0].Text.Content, "<at")
		assert.NotContains(t, sections[0].Text.Content, "[link]")
		assert.Equal(t, "plain_text", sections[1].Text.Tag)
		if groups == 0 {
			assert.Contains(t, sections[0].Text.Content, "22")
			assert.Contains(t, sections[1].Text.Content, "first probe")
			assert.Contains(t, sections[1].Text.Content, "second probe")
			assert.NotContains(t, sections[1].Text.Content, "third probe")
		} else {
			assert.Contains(t, sections[0].Text.Content, "23")
			assert.Contains(t, sections[1].Text.Content, "third probe")
			assert.NotContains(t, sections[1].Text.Content, "first probe")
		}
		groups++
	}
	assert.Equal(t, 2, groups, "blank lines in names must not split channel containers")
	assert.Equal(t, "hr", payload.Card.Elements[len(payload.Card.Elements)-2].Tag, "timestamp must be outside channel groups")

	// The generic Webhook retains the original structured values contract.
	body, err = service.BuildQualityWebhookPayload(service.QualityWebhookConfig{URL: "https://example.com/webhook"}, notify, 1599360473)
	require.NoError(t, err)
	var generic struct {
		Values []service.QualityWebhookAlert `json:"values"`
	}
	require.NoError(t, common.Unmarshal(body, &generic))
	assert.Equal(t, alerts, generic.Values)
}
