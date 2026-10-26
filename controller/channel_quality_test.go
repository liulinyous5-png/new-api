package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/qualityexpr"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQualityProbe{}, &model.ChannelQualityRun{}))
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
}

func TestQualitySuiteVerdict(t *testing.T) {
	passed := model.ChannelQualityVerdictPassed
	failed := model.ChannelQualityVerdictFailed
	errored := model.ChannelQualityVerdictError
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	summary, err := runChannelQualityScan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary["failed"])

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

func openQualityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.ChannelQualityProbe{},
		&model.ChannelQualityRun{},
		&model.ChannelQualitySuite{},
		&model.ChannelQualitySuiteItem{},
		&model.ChannelQualitySuiteRun{},
	))
	return db
}
