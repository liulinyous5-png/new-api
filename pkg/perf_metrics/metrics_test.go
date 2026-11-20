package perfmetrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClassifyRelayOutcome(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  *types.NewAPIError
		want Outcome
	}{
		{"success", context.Background(), nil, OutcomeSuccess},
		{"upstream bad request", context.Background(), types.InitOpenAIError("unknown", 400), OutcomeIgnored},
		{"upstream credentials returned as 400", context.Background(), types.InitOpenAIError("invalid_api_key", 400), OutcomeFailure},
		{"wrapped credentials keep their cause", context.Background(), types.NewErrorWithStatusCode(types.InitOpenAIError("invalid_api_key", 400), types.ErrorCodeInvalidRequest, 400), OutcomeFailure},
		{"upstream context limit returned as 500", context.Background(), types.InitOpenAIError("context_length_exceeded", 500), OutcomeIgnored},
		{"local rate limit", context.Background(), types.NewErrorWithStatusCode(errors.New("limited"), types.ErrorCodeInvalidRequest, 429), OutcomeIgnored},
		{"upstream rate limit", context.Background(), types.InitOpenAIError("rate_limit_exceeded", 429), OutcomeFailure},
		{"local quota", context.Background(), types.NewError(errors.New("quota"), types.ErrorCodeInsufficientUserQuota), OutcomeIgnored},
		{"local violation fee", context.Background(), types.NewError(errors.New("csam"), types.ErrorCodeViolationFeeGrokCSAM), OutcomeIgnored},
		{"upstream quota", context.Background(), types.InitOpenAIError("insufficient_quota", 429), OutcomeFailure},
		{"upstream gateway quota", context.Background(), types.InitOpenAIError(types.ErrorCodeInsufficientUserQuota, 403), OutcomeFailure},
		{"unavailable channel", context.Background(), types.NewErrorWithStatusCode(errors.New("disabled"), types.ErrorCodeGetChannelFailed, 403), OutcomeFailure},
		{"empty upstream response", context.Background(), types.NewError(errors.New("empty"), types.ErrorCodeEmptyResponse), OutcomeFailure},
		{"network failure", context.Background(), types.NewOpenAIError(errors.New("connection refused"), types.ErrorCodeDoRequestFailed, 500), OutcomeFailure},
		{"client cancellation", canceled, types.NewOpenAIError(errors.New("context canceled"), types.ErrorCodeDoRequestFailed, 500), OutcomeIgnored},
		{"upstream deadline", context.Background(), types.NewOpenAIError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, 504), OutcomeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ClassifyRelayOutcome(tc.ctx, &relaycommon.RelayInfo{}, tc.err))
		})
	}
	assert.Equal(t, OutcomeIgnored, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{PerformanceBusinessRejection: true}, nil))
}

func TestChannelAttemptCategories(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  *types.NewAPIError
		want string
	}{
		{"success", context.Background(), nil, "success"},
		{"cancelled read", cancelled, types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed), "cancelled"},
		{"confirmed failure before cancellation", cancelled, types.InitOpenAIError("server_error", 503), "upstream_server"},
		{"stream server error without HTTP status", context.Background(), types.InitOpenAIError("server_error", 0), "upstream_server"},
		{"gateway internal error", context.Background(), types.NewError(errors.New("internal failure"), "internal_error"), "gateway"},
		{"credentials with misleading status", context.Background(), types.InitOpenAIError("invalid_api_key", 400), "upstream_auth"},
		{"upstream quota", context.Background(), types.InitOpenAIError("insufficient_quota", 429), "upstream_quota"},
		{"upstream limit", context.Background(), types.InitOpenAIError("rate_limit_exceeded", 429), "rate_limit"},
		{"HTTP authentication error with generic type", context.Background(), types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error"}, 401), "upstream_auth"},
		{"HTTP rate limit with generic type", context.Background(), types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error"}, 429), "rate_limit"},
		{"HTTP server failure with generic type", context.Background(), types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error"}, 503), "upstream_server"},
		{"HTTP timeout with generic server code", context.Background(), types.InitOpenAIError("server_error", 504), "timeout"},
		{"business rejection", context.Background(), types.InitOpenAIError("context_length_exceeded", 500), "business_rejected"},
		{"deadline", context.Background(), types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed), "timeout"},
		{"network", context.Background(), types.NewError(errors.New("connection refused"), types.ErrorCodeDoRequestFailed), "network"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyChannelAttempt(tc.ctx, &relaycommon.RelayInfo{}, tc.err))
		})
	}
	stream := relaycommon.NewStreamStatus()
	stream.RequireTerminal()
	stream.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	info := &relaycommon.RelayInfo{IsStream: true, StreamStatus: stream}
	assert.Equal(t, "protocol", classifyChannelAttempt(context.Background(), info, nil), "200 plus truncated stream must fail")
	stream.MarkIncomplete("content_filter")
	assert.Equal(t, "business_rejected", classifyChannelAttempt(context.Background(), info, nil))
	stream.MarkFailed("server_error", "", 503)
	assert.Equal(t, "upstream_server", classifyChannelAttempt(cancelled, info, nil))
	assert.Equal(t, "unknown", classifyChannelAttempt(context.Background(), &relaycommon.RelayInfo{IsStream: true}, nil))
}

func TestChannelMetricsDatabase(t *testing.T) {
	for _, dialect := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "TEST_CHANNEL_MYSQL_DSN"}, {"postgres", "TEST_CHANNEL_POSTGRES_DSN"}} {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := ""
			if dialect.env != "" {
				dsn = os.Getenv(dialect.env)
				if dsn == "" {
					t.Skip("isolated database DSN not configured")
				}
			}
			t.Setenv("SQL_DSN", dsn)
			oldDB, oldPath, oldMaster := model.DB, common.SQLitePath, common.IsMasterNode
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			common.SQLitePath, common.IsMasterNode = filepath.Join(t.TempDir(), "channel-metrics.db"), true
			if dialect.name == "sqlite" && os.Getenv("TEST_CHANNEL_SQLITE_PATH") != "" {
				common.SQLitePath = os.Getenv("TEST_CHANNEL_SQLITE_PATH")
			}
			channelCollector.buckets = make(map[channelBucketKey]*channelBucket)
			t.Cleanup(func() {
				model.DB, common.SQLitePath, common.IsMasterNode = oldDB, oldPath, oldMaster
				common.SetDatabaseTypes(oldMain, oldLog)
				channelCollector.buckets = make(map[channelBucketKey]*channelBucket)
			})
			require.NoError(t, model.InitDB())
			db := model.DB
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			// Run the actual startup migration a second time, not just the new
			// table's migration, before exercising persisted counters.
			require.NoError(t, model.InitDB())
			secondDB, err := model.DB.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, secondDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.PerfMetric{}, &model.ChannelMetric{}))
			channels := []model.Channel{{Name: "metric-first", Type: 1}, {Name: "metric-fallback", Type: 14}}
			require.NoError(t, db.Create(&channels).Error)
			t.Cleanup(func() {
				db.Where("channel_id IN ?", []int{channels[0].Id, channels[1].Id}).Delete(&model.ChannelMetric{})
				db.Delete(&channels)
			})
			ctx := context.Background()
			info := &relaycommon.RelayInfo{OriginModelName: "model-a"}
			// A fails then B succeeds; a second request succeeds on A after retry.
			RecordChannelAttempt(ctx, channels[0].Id, info, true, "relay", types.InitOpenAIError("server_error", 503))
			RecordChannelAttempt(ctx, channels[1].Id, info, false, "relay", nil)
			RecordChannelAttempt(ctx, channels[0].Id, info, true, "relay", types.InitOpenAIError("rate_limit_exceeded", 429))
			RecordChannelAttempt(ctx, channels[0].Id, info, false, "relay", nil)
			RecordChannelAttempt(ctx, channels[0].Id, info, true, "relay", types.InitOpenAIError("context_length_exceeded", 400))
			RecordChannelAttempt(ctx, channels[0].Id, &relaycommon.RelayInfo{OriginModelName: "model-b"}, true, "relay", nil)
			require.NoError(t, FlushChannelMetrics())
			// Replaying identical committed snapshots cannot inflate counters.
			var saved []model.ChannelMetric
			require.NoError(t, db.Where("channel_id IN ?", []int{channels[0].Id, channels[1].Id}).Find(&saved).Error)
			require.NoError(t, model.SaveChannelMetrics(ctx, saved))
			require.NoError(t, db.AutoMigrate(&model.ChannelMetric{}))
			require.NoError(t, db.AutoMigrate(&model.ChannelMetric{}))
			result, err := QueryChannelMetrics(ctx, []int{channels[0].Id, channels[1].Id}, 5)
			require.NoError(t, err)
			require.Len(t, result.Channels, 2)
			a, b := result.Channels[0], result.Channels[1]
			assert.Equal(t, int64(2), a.Success)
			assert.Equal(t, int64(2), a.Failure)
			assert.Equal(t, int64(1), a.Excluded)
			require.NotNil(t, a.SuccessRate)
			assert.Equal(t, 50.0, *a.SuccessRate)
			require.NotNil(t, a.FirstSuccessRate)
			assert.Equal(t, 33.33, *a.FirstSuccessRate)
			assert.Nil(t, b.FirstSuccessRate, "fallback-only channels have no first-attempt sample")
			require.Len(t, a.Models, 2)
			assert.Equal(t, int64(1), a.Results["rate_limit"])
			scoped := model.WithChannelScope(ctx, model.ChannelScope{Mode: "selected", Types: []int{1}})
			filtered, err := QueryChannelMetrics(scoped, []int{channels[0].Id, channels[1].Id}, 5)
			require.NoError(t, err)
			require.Len(t, filtered.Channels, 1)
			assert.Equal(t, channels[0].Id, filtered.Channels[0].ChannelID)
			// A second collector snapshot must add to, rather than replace, A.
			extra := saved[0]
			extra.ID, extra.ChannelID, extra.ModelName, extra.Result, extra.FirstAttempt, extra.Count = "other-instance", channels[0].Id, "model-a", "success", true, 2
			require.NoError(t, model.SaveChannelMetrics(ctx, []model.ChannelMetric{extra}))
			channelCollector.buckets = make(map[channelBucketKey]*channelBucket)
			restarted, err := QueryChannelMetrics(ctx, []int{channels[0].Id}, 5)
			require.NoError(t, err)
			require.Len(t, restarted.Channels, 1)
			assert.Equal(t, int64(4), restarted.Channels[0].Success)
			// A queued Midjourney response is a successful submission; a later
			// transport timeout must remain distinguishable from task failure.
			RecordMidjourneyAttempt(ctx, channels[1].Id, "mj_imagine", true, &dto.MidjourneyResponseWithStatusCode{StatusCode: 200, Response: dto.MidjourneyResponse{Code: 21}}, nil)
			RecordMidjourneyAttempt(ctx, channels[1].Id, "mj_imagine", false, nil, context.DeadlineExceeded)
			require.NoError(t, FlushChannelMetrics())
			tasks, err := QueryChannelMetrics(ctx, []int{channels[1].Id}, 5)
			require.NoError(t, err)
			require.Len(t, tasks.Channels, 1)
			require.Len(t, tasks.Channels[0].Models, 2)
			assert.Equal(t, "task_submit", tasks.Channels[0].Models[0].Kind)
			assert.Equal(t, map[string]int64{"success": 1, "timeout": 1}, tasks.Channels[0].Models[0].Results)
			// Short windows must query their own history, not reuse an hour's
			// totals. Samples outside the window (including future rows) stay out.
			now := time.Now().Unix()
			history := []model.ChannelMetric{
				{ID: "window-recent", BucketTs: now - 2*60, Count: 1},
				{ID: "window-ten-minutes", BucketTs: now - 10*60, Count: 10},
				{ID: "window-forty-minutes", BucketTs: now - 40*60, Count: 100},
				{ID: "window-expired", BucketTs: now - 70*60, Count: 1000},
				{ID: "window-future", BucketTs: now + 2*60, Count: 10000},
			}
			for i := range history {
				history[i].ChannelID, history[i].ModelName, history[i].Kind, history[i].Result = channels[0].Id, "window-model", "relay", "success"
			}
			require.NoError(t, model.SaveChannelMetrics(ctx, history))
			for _, tc := range []struct {
				minutes   int
				successes int64
			}{{5, 1}, {30, 11}, {60, 111}} {
				window, err := QueryChannelMetrics(ctx, []int{channels[0].Id}, tc.minutes)
				require.NoError(t, err)
				require.Len(t, window.Channels, 1)
				index := slices.IndexFunc(window.Channels[0].Models, func(item ChannelModelMetric) bool { return item.Model == "window-model" })
				require.GreaterOrEqual(t, index, 0)
				assert.Equal(t, tc.successes, window.Channels[0].Models[index].Success)
				assert.GreaterOrEqual(t, window.WindowEnd-window.WindowStart, int64((tc.minutes-1)*60))
				assert.Less(t, window.WindowEnd-window.WindowStart, int64(tc.minutes*60))
			}
			require.NoError(t, model.SaveChannelMetrics(ctx, []model.ChannelMetric{
				{ID: "model-case-upper", ChannelID: channels[0].Id, BucketTs: now, ModelName: "Case-Sensitive-Model", Kind: "relay", Result: "success", Count: 3},
				{ID: "model-case-lower", ChannelID: channels[0].Id, BucketTs: now, ModelName: "case-sensitive-model", Kind: "relay", Result: "success", Count: 2},
			}))
			caseMetrics, err := QueryChannelMetrics(ctx, []int{channels[0].Id}, 5)
			require.NoError(t, err)
			require.Len(t, caseMetrics.Channels, 1)
			modelCounts := map[string]int64{}
			for _, metric := range caseMetrics.Channels[0].Models {
				modelCounts[metric.Model] = metric.Success
			}
			assert.Equal(t, int64(3), modelCounts["Case-Sensitive-Model"])
			assert.Equal(t, int64(2), modelCounts["case-sensitive-model"])
			// Failed writes must remain pending. A request finishing while a
			// snapshot is being written must survive the following flush as well.
			recoveryInfo := &relaycommon.RelayInfo{OriginModelName: "flush-recovery-model"}
			RecordChannelAttempt(ctx, channels[0].Id, recoveryInfo, true, "relay", nil)
			writeErr := errors.New("injected metrics write failure")
			require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("test:channel-metric-failure", func(tx *gorm.DB) { tx.AddError(writeErr) }))
			flushErr := FlushChannelMetrics()
			require.NoError(t, model.DB.Callback().Create().Remove("test:channel-metric-failure"))
			require.ErrorIs(t, flushErr, writeErr)
			entered, release := make(chan struct{}), make(chan struct{})
			var enteredOnce, releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("test:channel-metric-pause", func(*gorm.DB) {
				enteredOnce.Do(func() { close(entered); <-release })
			}))
			flushed := make(chan error, 1)
			go func() { flushed <- FlushChannelMetrics() }()
			<-entered
			RecordChannelAttempt(ctx, channels[0].Id, recoveryInfo, true, "relay", nil)
			releaseOnce.Do(func() { close(release) })
			flushErr = <-flushed
			require.NoError(t, model.DB.Callback().Create().Remove("test:channel-metric-pause"))
			require.NoError(t, flushErr)
			require.NoError(t, FlushChannelMetrics())
			recovered, err := QueryChannelMetrics(ctx, []int{channels[0].Id}, 5)
			require.NoError(t, err)
			require.Len(t, recovered.Channels, 1)
			index := slices.IndexFunc(recovered.Channels[0].Models, func(item ChannelModelMetric) bool { return item.Model == recoveryInfo.OriginModelName })
			require.GreaterOrEqual(t, index, 0)
			assert.Equal(t, int64(2), recovered.Channels[0].Models[index].Success)
			var version string
			query := "SELECT version()"
			if dialect.name == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("verified %s %s", dialect.name, version)
		})
	}
}

func TestStreamOutcomeClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		mark func(*relaycommon.StreamStatus)
		end  relaycommon.StreamEndReason
		want Outcome
	}{
		{"completed", (*relaycommon.StreamStatus).MarkCompleted, relaycommon.StreamEndReasonEOF, OutcomeSuccess},
		{"business rejection", func(s *relaycommon.StreamStatus) { s.MarkFailed("context_length_exceeded", "", 0) }, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"service error", func(s *relaycommon.StreamStatus) { s.MarkFailed("server_error", "", 0) }, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"error after completion", func(s *relaycommon.StreamStatus) { s.MarkCompleted(); s.MarkFailed("", "server_error", 0) }, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"output limit", func(s *relaycommon.StreamStatus) { s.MarkIncomplete("max_output_tokens") }, relaycommon.StreamEndReasonEOF, OutcomeSuccess},
		{"content filter", func(s *relaycommon.StreamStatus) { s.MarkIncomplete("content_filter") }, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"client cancel", (*relaycommon.StreamStatus).MarkCancelled, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"client gone", nil, relaycommon.StreamEndReasonClientGone, OutcomeIgnored},
		{"timeout", nil, relaycommon.StreamEndReasonTimeout, OutcomeFailure},
		{"missing terminal", nil, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"done marker without terminal", nil, relaycommon.StreamEndReasonDone, OutcomeSuccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := relaycommon.NewStreamStatus()
			stream.RequireTerminal()
			if tc.mark != nil {
				tc.mark(stream)
			}
			stream.SetEndReason(tc.end, nil)
			assert.Equal(t, tc.want, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{StreamStatus: stream}, nil))
		})
	}
}

func TestClientCancellationDuringUpstreamRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := relaycommon.NewStreamStatus()
	stream.RequireTerminal()
	stream.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("reader closed"))
	assert.Equal(t, OutcomeIgnored, ClassifyRelayOutcome(ctx, &relaycommon.RelayInfo{StreamStatus: stream}, nil))

	deadline := relaycommon.NewStreamStatus()
	deadline.SetEndReason(relaycommon.StreamEndReasonClientGone, context.DeadlineExceeded)
	assert.Equal(t, OutcomeFailure, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{StreamStatus: deadline}, nil))
}

func TestPerformanceWindowIncludesCurrentHour(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 37, 0, 0, time.UTC)
	start, end := queryWindow(now, 24)
	assert.Equal(t, time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC).Unix(), start)
	assert.Equal(t, now.Unix(), end)
}

func TestHourlySuccessSeriesWeightsSmallerBuckets(t *testing.T) {
	points := recentSuccessSeries(map[int64]counters{
		3600: {requestCount: 100, successCount: 100},
		3900: {requestCount: 1},
		7200: {requestCount: 1, successCount: 1},
	})
	assert.Equal(t, []SuccessRatePoint{{Ts: 3600, SuccessRate: 99.01}, {Ts: 7200, SuccessRate: 100}}, points)
}

func TestRecentHistoryRejectsOversizedModelLists(t *testing.T) {
	_, err := QuerySummaryAll(24, nil, make([]string, 51)...)
	require.EqualError(t, err, "Invalid parameters")
}

// Terminal task sampling: success/failure counts, end-to-end latency, and
// token throughput only for successful tasks that report tokens.
func TestRecordTaskResultSamplesTerminalTasks(t *testing.T) {
	hotBuckets.Clear()
	t.Cleanup(func() { hotBuckets.Clear() })
	now := time.Now().Unix()
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusSuccess,
		Group:      "a",
		SubmitTime: now - 120,
		StartTime:  now - 100,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, &relaycommon.TaskInfo{TotalTokens: 5000})
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusFailure,
		Group:      "a",
		SubmitTime: now - 60,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, relaycommon.FailTaskInfo("boom"))
	RecordTaskResult(&model.Task{Status: model.TaskStatusSuccess, FinishTime: now}, nil)

	merged := map[bucketKey]counters{}
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		require.Equal(t, "video-model", k.model)
		require.Equal(t, "a", k.group)
		k.bucketTs = 0
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})
	assert.Equal(t, counters{
		requestCount:   2,
		successCount:   1,
		totalLatencyMs: 180000,
		outputTokens:   5000,
		generationMs:   100000,
	}, merged[bucketKey{model: "video-model", group: "a"}])
}

// TEST_PERF_MYSQL_DSN / TEST_PERF_POSTGRES_DSN optionally run the aggregation
// against isolated real MySQL/PostgreSQL databases.
func TestPerformanceAggregationAndFlush(t *testing.T) {
	for _, dialect := range []struct{ name, env string }{
		{"sqlite", ""}, {"mysql", "TEST_PERF_MYSQL_DSN"}, {"postgres", "TEST_PERF_POSTGRES_DSN"},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := ""
			if dialect.env != "" {
				dsn = os.Getenv(dialect.env)
				if dsn == "" {
					t.Skip("isolated test database DSN is not configured")
				}
			}
			t.Setenv("SQL_DSN", dsn)
			oldDB, oldPath, oldMaster, oldRedis := model.DB, common.SQLitePath, common.IsMasterNode, common.RedisEnabled
			oldType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			common.SQLitePath, common.IsMasterNode, common.RedisEnabled = filepath.Join(t.TempDir(), "perf.db"), false, false
			hotBuckets.Clear()
			t.Cleanup(func() {
				model.DB, common.SQLitePath, common.IsMasterNode, common.RedisEnabled = oldDB, oldPath, oldMaster, oldRedis
				common.SetDatabaseTypes(oldType, oldLogType)
				hotBuckets.Clear()
			})
			require.NoError(t, model.InitDB())
			db := model.DB
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))
			require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
			versionQuery := "SELECT version()"
			if dialect.name == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)

			now := time.Now()
			start, _ := queryWindow(now, 24)
			hour := now.Unix() - now.Unix()%3600 - 3600
			// Historical counters remain usable without reclassification or migration.
			for _, row := range []model.PerfMetric{
				{ModelName: "test-model", Group: "a", BucketTs: hour, RequestCount: 100, SuccessCount: 100, TotalLatencyMs: 100000, TtftCount: 100, TtftSumMs: 10000, OutputTokens: 200, GenerationMs: 40000},
				{ModelName: "test-model", Group: "inactive", BucketTs: hour, RequestCount: 100},
				{ModelName: "test-model", Group: "a", BucketTs: start - 3600, RequestCount: 100},
			} {
				require.NoError(t, model.UpsertPerfMetric(&row))
			}
			groups := []string{"a", "b"}

			RecordRelayResult(context.Background(), &relaycommon.RelayInfo{OriginModelName: "test-model", UsingGroup: "b", StartTime: now}, types.InitOpenAIError("unknown", 400))
			businessRejected, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			require.NotNil(t, businessRejected.Summary)
			assert.Equal(t, 100.0, businessRejected.Summary.SuccessRate)

			failure := &atomicBucket{}
			failure.add(Sample{LatencyMs: 2000})
			hotBuckets.Store(bucketKey{model: "test-model", group: "b", bucketTs: hour}, failure)
			before, err := Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			require.NotNil(t, before.Summary)
			assert.Equal(t, Summary{SuccessRate: 99.01, AvgLatencyMs: 1009, AvgTps: 5}, *before.Summary)
			assert.Equal(t, start, before.WindowStart)
			require.Len(t, before.Series, 1)
			assert.Equal(t, hour, before.Series[0].Ts)
			assert.InDelta(t, 99.01, before.Series[0].SuccessRate, 0.01)
			require.Len(t, before.Groups, 2)

			summary, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			assert.Equal(t, before.Summary, summary.Summary)
			require.Len(t, summary.Models, 1)
			assert.Equal(t, 99.01, summary.Models[0].SuccessRate)
			assert.Equal(t, 99.01, summary.Models[0].RecentSuccessSeries[0].SuccessRate)
			encoded, err := common.Marshal(summary)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "request_count")
			assert.NotContains(t, string(encoded), "success_count")

			flushCompletedBuckets()
			flushCompletedBuckets()
			after, err := Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, before.Summary, after.Summary)
			assert.Equal(t, before.Series, after.Series)

			RecordRelayResult(context.Background(), &relaycommon.RelayInfo{OriginModelName: "test-model", UsingGroup: "a", StartTime: now}, types.InitOpenAIError("context_length_exceeded", 400))
			after, err = Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, before.Summary, after.Summary)
			onlyA, err := Query(QueryParams{Model: "test-model", Group: "a", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, 100.0, onlyA.Summary.SuccessRate)
			empty, err := Query(QueryParams{Model: "missing", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Nil(t, empty.Summary)
			assert.Empty(t, empty.Series)

			require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{ModelName: "second-model", Group: "a", BucketTs: hour, RequestCount: 1}))
			combined, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			assert.Equal(t, 98.04, combined.Summary.SuccessRate)
			assert.Equal(t, 99.01, combined.Models[0].SuccessRate)

			t.Run("recent models are summarized over their status sample hours", func(t *testing.T) {
				for i := range 26 {
					require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{
						ModelName: "history-model", Group: "a", BucketTs: hour - int64(i+1)*6*3600,
						RequestCount: 1, SuccessCount: 1,
					}))
				}
				for _, row := range []model.PerfMetric{
					{ModelName: "history-model", Group: "a", BucketTs: hour + 60, RequestCount: 100, SuccessCount: 100},
					{ModelName: "history-model", Group: "inactive", BucketTs: hour + 120, RequestCount: 100},
					{ModelName: "history-model", Group: "a", BucketTs: hour + 2*3600, RequestCount: 1},
					{ModelName: "history-idle", Group: "a", BucketTs: hour - 3*24*3600, RequestCount: 1},
					{ModelName: "history-hidden", Group: "inactive", BucketTs: hour - 3*24*3600, RequestCount: 1},
				} {
					require.NoError(t, model.UpsertPerfMetric(&row))
				}
				require.NoError(t, db.Create(&model.PerfMetric{ModelName: "history-model", Group: "a", BucketTs: hour - 3600}).Error)
				hotFailure := &atomicBucket{}
				hotFailure.add(Sample{})
				hotBuckets.Store(bucketKey{model: "history-model", group: "b", bucketTs: hour + 120}, hotFailure)
				hotSuccess := &atomicBucket{}
				hotSuccess.add(Sample{Success: true})
				hotBuckets.Store(bucketKey{model: "history-hot", group: "a", bucketTs: hour}, hotSuccess)
				hiddenFailure := &atomicBucket{}
				hiddenFailure.add(Sample{})
				hotBuckets.Store(bucketKey{model: "history-hidden", group: "inactive", bucketTs: hour}, hiddenFailure)

				window, err := QuerySummaryAll(24, groups)
				require.NoError(t, err)
				history, err := QuerySummaryAll(24, groups, "history-model", "history-idle", "history-hidden", "history-hot", "missing")
				require.NoError(t, err)
				assert.Equal(t, window.WindowStart, history.WindowStart)
				summaries := lo.KeyBy(history.Models, func(m ModelSummary) string { return m.ModelName })
				assert.Equal(t, int64(124), summaries["history-model"].RequestCount)
				assert.Equal(t, 99.19, summaries["history-model"].SuccessRate)
				assert.Equal(t, int64(1), summaries["history-idle"].RequestCount)
				assert.Equal(t, 0.0, summaries["history-idle"].SuccessRate)
				assert.NotContains(t, summaries, "history-hidden")
				points := history.RecentSuccessSeries["history-model"]
				require.Len(t, points, 24)
				expected := make([]SuccessRatePoint, 0, 24)
				for i := 23; i > 0; i-- {
					expected = append(expected, SuccessRatePoint{Ts: hour - int64(i)*6*3600, SuccessRate: 100})
				}
				expected = append(expected, SuccessRatePoint{Ts: hour, SuccessRate: 99.01})
				assert.Equal(t, expected, points)
				assert.Equal(t, []SuccessRatePoint{{Ts: hour - 3*24*3600, SuccessRate: 0}}, history.RecentSuccessSeries["history-idle"])
				assert.Equal(t, []SuccessRatePoint{{Ts: hour, SuccessRate: 100}}, history.RecentSuccessSeries["history-hot"])
				assert.Empty(t, history.RecentSuccessSeries["history-hidden"])
				assert.Empty(t, history.RecentSuccessSeries["missing"])

				denied, err := QuerySummaryAll(24, []string{}, "history-model", "history-hidden")
				require.NoError(t, err)
				assert.Empty(t, denied.RecentSuccessSeries)
				flushCompletedBuckets()
				persisted, err := QuerySummaryAll(24, groups, "history-model", "history-hot")
				require.NoError(t, err)
				assert.Equal(t, points, persisted.RecentSuccessSeries["history-model"])
				assert.Equal(t, history.RecentSuccessSeries["history-hot"], persisted.RecentSuccessSeries["history-hot"])
			})

			t.Run("recent history stops at the 7 day boundary even with fewer than 24 samples", func(t *testing.T) {
				historyStart, _ := queryWindow(now, 7*24)
				for _, row := range []model.PerfMetric{
					{ModelName: "history-boundary", Group: "a", BucketTs: historyStart, RequestCount: 1, SuccessCount: 1},
					{ModelName: "history-boundary", Group: "a", BucketTs: historyStart - 3600, RequestCount: 1},
					{ModelName: "history-expired", Group: "a", BucketTs: historyStart - 3600, RequestCount: 1},
				} {
					require.NoError(t, model.UpsertPerfMetric(&row))
				}
				hotExpired := &atomicBucket{}
				hotExpired.add(Sample{Success: true})
				hotBuckets.Store(bucketKey{model: "history-hot-expired", group: "a", bucketTs: historyStart - 3600}, hotExpired)
				hotBoundary := &atomicBucket{}
				hotBoundary.add(Sample{Success: true})
				hotBuckets.Store(bucketKey{model: "history-hot-boundary", group: "a", bucketTs: historyStart}, hotBoundary)

				history, err := QuerySummaryAll(24, groups, "history-boundary", "history-expired", "history-hot-expired", "history-hot-boundary")
				require.NoError(t, err)
				assert.Equal(t, []SuccessRatePoint{{Ts: historyStart, SuccessRate: 100}}, history.RecentSuccessSeries["history-boundary"])
				assert.Equal(t, []SuccessRatePoint{{Ts: historyStart, SuccessRate: 100}}, history.RecentSuccessSeries["history-hot-boundary"])
				assert.Empty(t, history.RecentSuccessSeries["history-expired"])
				assert.Empty(t, history.RecentSuccessSeries["history-hot-expired"])
			})
		})
	}
}
