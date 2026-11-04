package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// callLogHandler runs a log handler as the given user and returns the JSON
// lines of the response.
func callLogHandler(t *testing.T, handler gin.HandlerFunc, userId int, role int, query string) []map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log?"+query, nil)
	ctx.Set("id", userId)
	ctx.Set("role", role)
	handler(ctx)
	var lines []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(recorder.Body.String()), "\n") {
		var value map[string]any
		require.NoError(t, common.UnmarshalJsonStr(line, &value))
		lines = append(lines, value)
	}
	return lines
}

func exportedRequestIds(lines []map[string]any) []any {
	ids := make([]any, 0, len(lines))
	for _, line := range lines[:len(lines)-1] {
		ids = append(ids, line["request_id"])
	}
	return ids
}

// seedExportLogs stores logs of two users: three in the last hour and one
// from 400 days ago.
func seedExportLogs(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB, now int64) {
	t.Helper()
	require.NoError(t, mainDB.Create(&model.Channel{Id: 9, Name: "primary"}).Error)
	logs := []model.Log{
		{UserId: 1, Username: "alice", CreatedAt: now - 3000, Type: model.LogTypeConsume, ModelName: "gpt-a", ChannelId: 9, Quota: 10, RequestId: "a-1", Other: `{"admin_info":{"use_channel":["9"]}}`},
		{UserId: 2, Username: "bob", CreatedAt: now - 1800, Type: model.LogTypeConsume, ModelName: "gpt-b", ChannelId: 9, Quota: 20, RequestId: "b-1"},
		{UserId: 1, Username: "alice", CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "gpt-a", RequestId: "a-2"},
		{UserId: 1, Username: "alice", CreatedAt: now - 400*24*3600, Type: model.LogTypeConsume, ModelName: "gpt-a", RequestId: "a-old"},
	}
	for i := range logs {
		require.NoError(t, logDB.Create(&logs[i]).Error)
	}
}

func useLogExportDatabases(t *testing.T, mainDB *gorm.DB, logDB *gorm.DB, mainType common.DatabaseType, logType common.DatabaseType) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	model.DB, model.LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(mainType, logType)
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
	})
}

func TestLogExportDatabaseMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	for _, tc := range []struct {
		kind, env string
		typ       common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"mysql", "AUDIT_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "AUDIT_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
		{"clickhouse", "AUDIT_CLICKHOUSE_DSN", common.DatabaseTypeClickHouse},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if tc.env != "" && dsn == "" {
				t.Skip(tc.env + " is not configured")
			}
			db, isolatedDSN := newAuditTestDatabase(t, tc.kind, dsn)
			mainDB, mainType := db, tc.typ
			if tc.kind == "clickhouse" {
				mainDB, _ = newAuditTestDatabase(t, "sqlite", "")
				mainType = common.DatabaseTypeSQLite
			}
			useLogExportDatabases(t, mainDB, db, mainType, tc.typ)
			require.NoError(t, mainDB.AutoMigrate(&model.Channel{}))
			if tc.kind == "clickhouse" {
				t.Setenv("LOG_SQL_DSN", isolatedDSN)
				require.NoError(t, model.InitLogDB())
			} else {
				require.NoError(t, db.AutoMigrate(&model.Log{}))
				t.Setenv("LOG_SQL_DSN", "")
				require.NoError(t, model.InitLogDB())
				require.True(t, model.LOG_DB.Migrator().HasTable(&model.ErrorLog{}))
			}
			versionSQL := "SELECT version()"
			if tc.kind == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, model.LOG_DB.Raw(versionSQL).Scan(&version).Error)
			t.Logf("log database: %s %s", tc.kind, version)
			now := time.Now().Unix()
			seedExportLogs(t, mainDB, model.LOG_DB, now)
			query := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-2*24*3600, now+3600)

			listed := callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, query)[0]
			require.Equal(t, true, listed["success"])
			assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])
			listed = callLogHandler(t, GetAllLogs, 3, common.RoleAdminUser, query+"&username=alice")[0]
			require.Equal(t, true, listed["success"])
			assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])

			own := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, query)
			require.Len(t, own, 3)
			assert.Equal(t, []any{"a-2", "a-1"}, exportedRequestIds(own))
			assert.Equal(t, map[string]any{"success": true, "message": "", "data": map[string]any{"count": float64(2)}}, own[2])
			assert.Equal(t, float64(1), own[0]["id"], "user exports carry list positions, not row ids")
			assert.Empty(t, own[1]["channel_name"])
			assert.NotContains(t, own[1]["other"], "admin_info")

			all := callLogHandler(t, ExportAllLogs, 3, common.RoleAdminUser, query+"&model_name=gpt-%25")
			require.Len(t, all, 4)
			assert.Equal(t, []any{"a-2", "b-1", "a-1"}, exportedRequestIds(all))
			assert.Equal(t, "primary", all[2]["channel_name"])
			assert.Contains(t, all[2]["other"], "admin_info")
			assert.Equal(t, true, all[3]["success"])
			t.Run("trace request types", func(t *testing.T) {
				verifyTraceRequestLogTypes(t, mainDB)
			})
		})
	}
}

func verifyTraceRequestLogTypes(t *testing.T, mainDB *gorm.DB) {
	t.Helper()
	previousConsume, previousExport, previousCallback := common.LogConsumeEnabled, common.DataExportEnabled, model.OnConsumeLogRecorded
	common.LogConsumeEnabled, common.DataExportEnabled, model.OnConsumeLogRecorded = true, false, nil
	t.Cleanup(func() {
		common.LogConsumeEnabled, common.DataExportEnabled, model.OnConsumeLogRecorded = previousConsume, previousExport, previousCallback
	})
	require.NoError(t, mainDB.AutoMigrate(&model.User{}))
	require.NoError(t, mainDB.Create(&model.User{Id: 1, Username: "trace-user"}).Error)
	now := time.Now().Unix()
	for _, tc := range []struct {
		name, header, value string
		wantType            int
	}{
		{"ordinary", "", "", model.LogTypeConsume},
		{"trace", "X-Trace-Id", "trace-1", model.LogTypeTrace},
		{"request", "X-Request-ID", "client-1", model.LogTypeTrace},
		{"blank", "X-Trace-Id", "   ", model.LogTypeConsume},
		{"unsafe", "X-Request-ID", "bad\nvalue", model.LogTypeConsume},
		{"session", "X-Conversation-Id", "session-1", model.LogTypeConsume},
	} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Set("username", "trace-user")
		ctx.Set(common.RequestIdKey, tc.name)
		if tc.header != "" {
			ctx.Request.Header.Set(tc.header, tc.value)
		}
		model.RecordConsumeLog(ctx, 1, model.RecordConsumeLogParams{
			ModelName: "trace-test", ChannelId: 9, TokenName: "trace-token", Group: "trace-group",
			Quota: 10, PromptTokens: 2, CompletionTokens: 3,
		})
		model.RecordErrorLog(ctx, 1, 9, "trace-test", "trace-token", "failed", 0, 1, false, "trace-group", nil)
		var rows []model.Log
		require.NoError(t, model.LOG_DB.Where("request_id = ?", tc.name).Find(&rows).Error)
		require.Len(t, rows, 1, "each request writes exactly one consume log")
		assert.Equal(t, tc.wantType, rows[0].Type, tc.name)
		assert.Equal(t, 10, rows[0].Quota)
		if tc.wantType == model.LogTypeTrace {
			assert.Contains(t, rows[0].Other, tc.value)
		}
	}
	// A different user's matching type must not leak into user lists or totals.
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: 2, Username: "another-user", CreatedAt: now, Type: model.LogTypeTrace,
		ModelName: "trace-test", Quota: 99, RequestId: "other-user",
	}).Error)
	query := fmt.Sprintf("model_name=trace-test&start_timestamp=%d&end_timestamp=%d", now-3600, now+3600)
	for _, tc := range []struct{ typ, count int }{{0, 6}, {model.LogTypeConsume, 4}, {model.LogTypeTrace, 2}} {
		filtered := fmt.Sprintf("%s&type=%d", query, tc.typ)
		for _, endpoint := range []struct {
			list, export gin.HandlerFunc
			role         int
			query        string
		}{
			{GetUserLogs, ExportUserLogs, common.RoleCommonUser, filtered},
			{GetAllLogs, ExportAllLogs, common.RoleAdminUser, filtered + "&username=trace-user"},
		} {
			listed := callLogHandler(t, endpoint.list, 1, endpoint.role, endpoint.query)[0]
			require.Equal(t, true, listed["success"])
			assert.EqualValues(t, tc.count, listed["data"].(map[string]any)["total"])
			exported := callLogHandler(t, endpoint.export, 1, endpoint.role, endpoint.query)
			require.Len(t, exported, tc.count+1)
			assert.Equal(t, true, exported[tc.count]["success"])
			for _, row := range exported[:tc.count] {
				if tc.typ != 0 {
					assert.EqualValues(t, tc.typ, row["type"])
				}
				assert.NotContains(t, row, "request_type")
			}
		}
		stat, err := model.SumUsedQuota(tc.typ, now-3600, now+3600, "trace-test", "trace-user", "trace-token", 9, "trace-group")
		require.NoError(t, err)
		assert.Equal(t, model.Stat{Quota: tc.count * 10, Rpm: tc.count, Tpm: tc.count * 5}, stat)
		assert.Equal(t, tc.count*5, model.SumUsedToken(tc.typ, now-3600, now+3600, "trace-test", "trace-user", "trace-token"))
		for _, handler := range []gin.HandlerFunc{GetLogsStat, func(c *gin.Context) {
			c.Set("username", "trace-user")
			GetLogsSelfStat(c)
		}} {
			response := callLogHandler(t, handler, 1, common.RoleCommonUser, filtered+"&username=trace-user")[0]
			require.Equal(t, true, response["success"])
			assert.Equal(t, map[string]any{"quota": float64(tc.count * 10), "rpm": float64(tc.count), "tpm": float64(tc.count * 5)}, response["data"])
		}
	}
	errors := callLogHandler(t, GetUserErrorLogs, 1, common.RoleCommonUser, query)[0]
	require.Equal(t, true, errors["success"])
	assert.EqualValues(t, 6, errors["data"].(map[string]any)["total"])
	var errorRows []model.ErrorLog
	require.NoError(t, model.LOG_DB.Where("model_name = ?", "trace-test").Find(&errorRows).Error)
	for _, row := range errorRows {
		assert.Equal(t, model.LogTypeError, row.Type)
	}
	common.LogConsumeEnabled = false
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx.Request.Header.Set("X-Trace-Id", "disabled")
	model.RecordConsumeLog(ctx, 1, model.RecordConsumeLogParams{ModelName: "disabled"})
	model.RecordLog(1, model.LogTypeTrace, common.NewMessage("disabled"))
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("model_name = ? OR content = ?", "disabled", "disabled").Count(&count).Error)
	assert.Zero(t, count, "trace requests respect the consume log switch")
}

func TestLogExportRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := newAuditTestDatabase(t, "sqlite", "")
	useLogExportDatabases(t, db, db, common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Log{}, &model.QuotaData{}))
	now := time.Now().Unix()
	seedExportLogs(t, db, db, now)
	logSetting := operation_setting.GetLogSetting()
	previousSetting := *logSetting
	t.Cleanup(func() { *logSetting = previousSetting })
	recent := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-2*24*3600, now)
	tooLong := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-366*24*3600, now)

	t.Run("time range", func(t *testing.T) {
		rejected := callLogHandler(t, ExportAllLogs, 3, common.RoleAdminUser, tooLong)
		require.Len(t, rejected, 1)
		assert.Equal(t, false, rejected[0]["success"])
		assert.Equal(t, "The time range cannot exceed {{days}} days", rejected[0]["message_key"])
		assert.Equal(t, map[string]any{"days": float64(365)}, rejected[0]["message_params"])

		assert.Equal(t, false, callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, tooLong)[0]["success"])
		assert.Equal(t, false, callLogHandler(t, GetLogsSelfStat, 1, common.RoleCommonUser, tooLong)[0]["success"])
		assert.Equal(t, true, callLogHandler(t, GetUserLogs, 3, common.RoleAdminUser, tooLong)[0]["success"], "administrators search their own logs without a limit")
		assert.Equal(t, true, callLogHandler(t, GetAllLogs, 3, common.RoleAdminUser, tooLong)[0]["success"])

		// The dashboard follows the same range instead of its old one-month limit.
		fortyDays := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-40*24*3600, now)
		for _, handler := range []gin.HandlerFunc{GetUserQuotaDates, GetUserFlowQuotaDates} {
			assert.Equal(t, "The time range cannot exceed {{days}} days", callLogHandler(t, handler, 1, common.RoleCommonUser, tooLong)[0]["message_key"])
			assert.Equal(t, true, callLogHandler(t, handler, 1, common.RoleCommonUser, fortyDays)[0]["success"])
		}

		// Without a start, a search covers the hour before and after now.
		listed := callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, "")[0]
		assert.Equal(t, true, listed["success"])
		assert.EqualValues(t, 2, listed["data"].(map[string]any)["total"])

		logSetting.MaxRangeDays = 400
		assert.Equal(t, true, callLogHandler(t, GetUserLogs, 1, common.RoleCommonUser, tooLong)[0]["success"])
		logSetting.MaxRangeDays = previousSetting.MaxRangeDays
	})

	t.Run("user export switch", func(t *testing.T) {
		logSetting.UserExportEnabled = false
		t.Cleanup(func() { logSetting.UserExportEnabled = previousSetting.UserExportEnabled })
		disabled := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent)
		require.Len(t, disabled, 1)
		assert.Equal(t, "Log export is disabled", disabled[0]["message_key"])
		assert.Equal(t, true, callLogHandler(t, ExportUserLogs, 3, common.RoleAdminUser, recent)[0]["success"], "administrators export their own logs even when users cannot")
	})

	t.Run("one export per user", func(t *testing.T) {
		release, ok, err := service.AcquireLogExportLock(1)
		require.NoError(t, err)
		require.True(t, ok)
		busy := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent)
		require.Len(t, busy, 1)
		assert.Equal(t, "Another log export is running. Try again after it finishes.", busy[0]["message_key"])
		other := callLogHandler(t, ExportUserLogs, 2, common.RoleCommonUser, recent)
		assert.Equal(t, true, other[len(other)-1]["success"], "other users are not blocked")
		release()
		assert.Len(t, callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, recent), 3)
	})
}

func TestAcquireLogExportLock(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis=%v", useRedis), func(t *testing.T) {
			previousRedis, previousClient := common.RedisEnabled, common.RDB
			common.RedisEnabled = useRedis
			if useRedis {
				server := miniredis.RunT(t)
				common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
				require.NoError(t, common.RDB.Ping(context.Background()).Err())
			}
			t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedis, previousClient })

			release, ok, err := service.AcquireLogExportLock(1)
			require.NoError(t, err)
			require.True(t, ok)
			_, ok, err = service.AcquireLogExportLock(1)
			require.NoError(t, err)
			assert.False(t, ok, "a second export of the same user must wait")
			releaseOther, ok, err := service.AcquireLogExportLock(2)
			require.NoError(t, err)
			assert.True(t, ok)
			releaseOther()
			release()
			releaseAgain, ok, err := service.AcquireLogExportLock(1)
			require.NoError(t, err)
			assert.True(t, ok, "a finished export frees the user")
			releaseAgain()
		})
	}
}
