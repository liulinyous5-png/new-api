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
		})
	}
}

// Exercise the real startup path with shared and separate log databases. The
// pre-merge Log schema is unchanged; removing only error_logs recreates the
// legacy layout without changing or copying its existing log rows.
func TestErrorLogDatabaseMatrix(t *testing.T) {
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
		for _, separate := range []bool{false, true} {
			if tc.kind == "clickhouse" && !separate {
				continue
			}
			t.Run(fmt.Sprintf("%s/separate=%v", tc.kind, separate), func(t *testing.T) {
				dsn := os.Getenv(tc.env)
				if tc.env != "" && dsn == "" {
					t.Skip(tc.env + " is not configured")
				}
				db, isolatedDSN := newAuditTestDatabase(t, tc.kind, dsn)
				mainDB, mainType := db, tc.typ
				if separate {
					mainDB, _ = newAuditTestDatabase(t, "sqlite", "")
					mainType = common.DatabaseTypeSQLite
				}
				useLogExportDatabases(t, mainDB, db, mainType, tc.typ)
				require.NoError(t, mainDB.AutoMigrate(&model.Channel{}))
				if separate {
					t.Setenv("LOG_SQL_DSN", isolatedDSN)
					if tc.kind == "sqlite" {
						previousPath := common.SQLitePath
						common.SQLitePath = isolatedDSN
						t.Cleanup(func() { common.SQLitePath = previousPath })
						t.Setenv("LOG_SQL_DSN", "local")
					}
				} else {
					t.Setenv("LOG_SQL_DSN", "")
					// InitDB normally creates the existing logs table first.
					require.NoError(t, db.AutoMigrate(&model.Log{}))
				}
				for range 2 {
					require.NoError(t, model.InitLogDB())
				}
				require.True(t, model.LOG_DB.Migrator().HasTable(&model.ErrorLog{}))
				var version string
				versionSQL := "SELECT version()"
				if tc.kind == "sqlite" {
					versionSQL = "SELECT sqlite_version()"
				}
				require.NoError(t, model.LOG_DB.Raw(versionSQL).Scan(&version).Error)
				t.Logf("verified fresh and upgrade migrations: %s %s (separate=%v)", tc.kind, version, separate)

				require.NoError(t, model.LOG_DB.Migrator().DropTable(&model.ErrorLog{}))
				now := time.Now().Unix()
				seedExportLogs(t, mainDB, model.LOG_DB, now)
				for range 2 {
					require.NoError(t, model.InitLogDB())
				}
				query := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-2*24*3600, now+3600)
				legacy := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, query)
				assert.Equal(t, []any{"a-2", "a-1"}, exportedRequestIds(legacy), "legacy errors remain accessible in usage logs")
				for _, row := range []model.ErrorLog{
					{UserId: 1, Username: "alice", CreatedAt: now - 40, Type: model.LogTypeError, ModelName: "gpt-a", ChannelId: 9, TokenId: 11, RequestId: "error-a", Group: "default", Other: `{"admin_info":{"use_channel":["9"]}}`},
					{UserId: 2, Username: "bob", CreatedAt: now - 30, Type: model.LogTypeError, ModelName: "gpt-b", ChannelId: 9, TokenId: 22, RequestId: "error-b"},
					{UserId: 1, CreatedAt: now - 400*24*3600, Type: model.LogTypeError, RequestId: "error-old"},
				} {
					require.NoError(t, model.LOG_DB.Create(&row).Error)
				}
				for range 2 {
					require.NoError(t, model.InitLogDB())
				}
				if tc.kind != "clickhouse" {
					assert.True(t, model.LOG_DB.Migrator().HasIndex(&model.Log{}, "idx_created_at_id"))
					assert.True(t, model.LOG_DB.Migrator().HasIndex(&model.ErrorLog{}, "idx_errlog_created_at_id"))
				}
				listed := callLogHandler(t, GetUserErrorLogs, 1, common.RoleCommonUser, query)[0]
				require.Equal(t, true, listed["success"])
				assert.EqualValues(t, 1, listed["data"].(map[string]any)["total"])
				own := callLogHandler(t, ExportUserErrorLogs, 1, common.RoleCommonUser, query)
				require.Len(t, own, 2)
				assert.Equal(t, []any{"error-a"}, exportedRequestIds(own))
				assert.Empty(t, own[0]["channel_name"])
				assert.NotContains(t, own[0]["other"], "admin_info")
				all := callLogHandler(t, ExportAllErrorLogs, 3, common.RoleAdminUser, query+"&channel=9")
				require.Len(t, all, 3)
				assert.Equal(t, []any{"error-b", "error-a"}, exportedRequestIds(all))
				assert.Equal(t, "primary", all[1]["channel_name"])
				assert.Contains(t, all[1]["other"], "admin_info")
				filtered := callLogHandler(t, GetAllErrorLogs, 3, common.RoleAdminUser, query+"&group=default&model_name=gpt-a")[0]
				require.Equal(t, true, filtered["success"])
				assert.EqualValues(t, 1, filtered["data"].(map[string]any)["total"])
				tokenLogs, err := model.GetLogByTokenId(11)
				require.NoError(t, err)
				require.Len(t, tokenLogs, 1)
				assert.Equal(t, "error-a", tokenLogs[0].RequestId)
				assert.NotContains(t, tokenLogs[0].Other, "admin_info")
				count, err := model.CountOldLog(context.Background(), now-365*24*3600)
				require.NoError(t, err)
				assert.EqualValues(t, 2, count)
				deleted, err := model.DeleteOldLogBatch(context.Background(), now-365*24*3600, 100)
				require.NoError(t, err)
				assert.EqualValues(t, 2, deleted)
				count, err = model.CountOldLog(context.Background(), now-365*24*3600)
				require.NoError(t, err)
				assert.Zero(t, count)
				retained := callLogHandler(t, ExportUserLogs, 1, common.RoleCommonUser, query)
				assert.Equal(t, exportedRequestIds(legacy), exportedRequestIds(retained), "separate error writes and cleanup preserve usage logs")
			})
		}
	}
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
