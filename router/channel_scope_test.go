package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The same HTTP and persistence contracts run against each real engine. DSNs
// must point at disposable databases; startup migrations run twice on purpose.
func TestChannelScopeDatabase(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := "local"
			if dialect != "sqlite" {
				dsn = os.Getenv("TEST_SCOPE_" + strings.ToUpper(dialect) + "_DSN")
				if dsn == "" {
					t.Skip("dedicated database DSN not configured")
				}
			}
			previousDB, previousLog := model.DB, model.LOG_DB
			previousMaster, previousRedis, previousCache, previousSQLite := common.IsMasterNode, common.RedisEnabled, common.MemoryCacheEnabled, common.SQLitePath
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			common.IsMasterNode, common.RedisEnabled, common.MemoryCacheEnabled = true, false, false
			common.SQLitePath = filepath.Join(t.TempDir(), "scope.db")
			if dialect == "sqlite" && os.Getenv("TEST_SCOPE_SQLITE_PATH") != "" {
				common.SQLitePath = os.Getenv("TEST_SCOPE_SQLITE_PATH")
			}
			t.Setenv("SQL_DSN", dsn)
			t.Setenv("LOG_SQL_DSN", os.Getenv("TEST_SCOPE_LOG_DSN"))
			var legacyID int
			if os.Getenv("TEST_SCOPE_UPGRADE") == "1" {
				// The database must first be initialized by a released server binary.
				var driver gorm.Dialector = sqlite.Open(common.SQLitePath)
				if dialect == "mysql" {
					driver = mysql.Open(dsn)
				} else if dialect == "postgres" {
					driver = postgres.Open(dsn)
				}
				legacy, err := gorm.Open(driver)
				require.NoError(t, err)
				require.True(t, legacy.Migrator().HasTable("users"))
				require.False(t, legacy.Migrator().HasColumn("users", "admin_channel_scope"))
				require.True(t, legacy.Migrator().HasIndex("users", "idx_users_username"))
				require.NoError(t, legacy.Table("users").Create(map[string]any{
					"username": "legacy-scope-admin", "password": "", "display_name": "Existing administrator", "role": common.RoleAdminUser,
					"status": common.UserStatusEnabled, "aff_code": "legscope", "auth_version": 7,
				}).Error)
				require.NoError(t, legacy.Table("users").Where("username = ?", "legacy-scope-admin").Pluck("id", &legacyID).Error)
				connection, err := legacy.DB()
				require.NoError(t, err)
				require.NoError(t, connection.Close())
			}
			require.NoError(t, model.InitDB())
			require.NoError(t, model.InitLogDB())
			t.Cleanup(func() {
				db, err := model.DB.DB()
				require.NoError(t, err)
				require.NoError(t, db.Close())
				if model.LOG_DB != model.DB {
					logDB, err := model.LOG_DB.DB()
					require.NoError(t, err)
					require.NoError(t, logDB.Close())
				}
				model.DB, model.LOG_DB = previousDB, previousLog
				common.IsMasterNode, common.RedisEnabled, common.MemoryCacheEnabled, common.SQLitePath = previousMaster, previousRedis, previousCache, previousSQLite
				common.SetDatabaseTypes(previousMainType, previousLogType)
			})
			if legacyID != 0 {
				t.Cleanup(func() { require.NoError(t, model.DB.Unscoped().Delete(&model.User{}, legacyID).Error) })
			}
			require.NoError(t, authz.Init(model.DB))
			admin := model.User{Username: "scope-admin", DisplayName: "Scope administrator", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "scopeadm", AuthVersion: 1}
			require.NoError(t, model.DB.Create(&admin).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Unscoped().Delete(&admin).Error) })
			require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": {"read": true, "write": true, "operate": true, "sensitive_write": true}}))
			t.Cleanup(func() { require.NoError(t, authz.ClearUserPermissions(admin.Id)) })
			tag := "scope-shared-tag"
			channels := []model.Channel{
				{Name: "scope-openai", Type: 1, Key: "fixture-openai", Status: common.ChannelStatusEnabled, Models: "scope-model-a", Group: "default", Tag: &tag},
				{Name: "scope-anthropic", Type: 14, Key: "fixture-anthropic", Status: common.ChannelStatusEnabled, Models: "scope-model-b", Group: "default", Tag: &tag},
				{Name: "scope-gemini-hidden", Type: 24, Key: "fixture-gemini", Status: common.ChannelStatusEnabled, Models: "scope-model-hidden", Group: "default", Tag: &tag},
			}
			for i := range channels {
				require.NoError(t, channels[i].Insert())
			}
			ids := []int{channels[0].Id, channels[1].Id, channels[2].Id}
			t.Cleanup(func() { _, err := model.BatchDeleteChannels(ids); require.NoError(t, err) })
			gin.SetMode(gin.TestMode)
			role := common.RoleAdminUser
			engine := gin.New()
			group := engine.Group("/api/channel", func(c *gin.Context) { c.Set("id", admin.Id); c.Set("role", role); c.Next() })
			for _, route := range channelPermissionRoutes {
				handlePermissionRoute(group, route.method, route.path, route.permission, middleware.RequireChannelScope(), route.handler)
			}
			engine.GET("/api/log/", func(c *gin.Context) { c.Set("id", admin.Id); c.Set("role", role); c.Next() }, middleware.LoadChannelScope(), controller.GetAllLogs)
			engine.GET("/api/log/stat", func(c *gin.Context) { c.Set("id", admin.Id); c.Set("role", role); c.Next() }, middleware.LoadChannelScope(), controller.GetLogsStat)
			engine.GET("/api/models/", func(c *gin.Context) { c.Set("id", admin.Id); c.Set("role", role); c.Next() }, middleware.LoadChannelScope(), controller.GetAllModelsMeta)
			request := func(method, path, body string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, r)
				return w
			}
			list := func(path string, expected int) {
				w := request(http.MethodGet, path, "")
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				var response struct {
					Success bool `json:"success"`
					Data    struct {
						Total  int             `json:"total"`
						Items  []model.Channel `json:"items"`
						Counts map[int]int     `json:"type_counts"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				require.True(t, response.Success, w.Body.String())
				assert.Equal(t, expected, response.Data.Total)
				assert.NotContains(t, w.Body.String(), "fixture-openai")
				if role != common.RoleRootUser && expected < 3 {
					assert.NotContains(t, w.Body.String(), channels[2].Name)
					assert.NotContains(t, response.Data.Counts, 24)
				}
			}
			list("/api/channel/", 3) // old/missing configuration remains unrestricted
			scope := model.ChannelScope{Mode: "selected", Types: []int{14, 1, 1}}
			require.NoError(t, model.SetAdminChannelScopeInTx(model.DB, admin.Id, scope))
			loaded, err := model.GetAdminChannelScope(admin.Id)
			require.NoError(t, err)
			assert.Equal(t, []int{1, 14}, loaded.Types)
			list("/api/channel/", 2)
			list("/api/channel/?page_size=1&p=2", 2)
			list("/api/channel/?type=24", 0)
			list("/api/channel/?tag_mode=true", 1)
			list("/api/channel/search?keyword=scope-", 2)
			list("/api/channel/search?keyword=scope-&tag_mode=true", 2)
			list("/api/channel/search?keyword=scope-gemini-hidden&tag_mode=true", 0)
			metrics := []model.ChannelMetric{
				{ID: "scope-visible-metric", ChannelID: ids[0], BucketTs: time.Now().Unix(), ModelName: "scope-visible-metric-model", Kind: "relay", Result: "success", Count: 1},
				{ID: "scope-hidden-metric", ChannelID: ids[2], BucketTs: time.Now().Unix(), ModelName: "scope-hidden-metric-model", Kind: "relay", Result: "success", Count: 1},
			}
			require.NoError(t, model.DB.Create(&metrics).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(&metrics).Error) })
			// Client-facing status mappings must not change the observed upstream
			// failure or remove it from the channel's success-rate denominator.
			mappedModel := "mapped-status-" + dialect
			for _, status := range []int{401, 429, 503} {
				upstreamErr := types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error"}, status)
				service.ResetStatusCode(upstreamErr, fmt.Sprintf(`{"%d":502}`, status))
				service.ResetStatusCode(upstreamErr, `{"502":400}`)
				assert.Equal(t, 400, upstreamErr.StatusCode)
				perfmetrics.RecordChannelAttempt(context.Background(), ids[0], &relaycommon.RelayInfo{OriginModelName: mappedModel}, true, "relay", upstreamErr)
			}
			require.NoError(t, perfmetrics.FlushChannelMetrics())
			t.Cleanup(func() {
				require.NoError(t, model.DB.Where("model_name = ?", mappedModel).Delete(&model.ChannelMetric{}).Error)
			})
			for _, minutes := range []int{5, 30, 60} {
				metricResponse := request(http.MethodGet, fmt.Sprintf("/api/channel/metrics?channel_ids=%d,%d&minutes=%d", ids[0], ids[2], minutes), "")
				require.Equal(t, http.StatusOK, metricResponse.Code, metricResponse.Body.String())
				assert.Contains(t, metricResponse.Body.String(), "scope-visible-metric-model")
				assert.NotContains(t, metricResponse.Body.String(), "scope-hidden-metric-model")
				var response struct {
					Data perfmetrics.ChannelMetricsResult
				}
				require.NoError(t, common.Unmarshal(metricResponse.Body.Bytes(), &response))
				require.Len(t, response.Data.Channels, 1)
				assert.Equal(t, int64(3), response.Data.Channels[0].Failure)
				assert.Equal(t, map[string]int64{"success": 1, "upstream_auth": 1, "rate_limit": 1, "upstream_server": 1}, response.Data.Channels[0].Results)
			}
			for _, query := range []string{"minutes=0", "minutes=24", "minutes=9999", "minutes=abc", "hours=24"} {
				assert.Equal(t, http.StatusBadRequest, request(http.MethodGet, "/api/channel/metrics?channel_ids=1&"+query, "").Code)
			}
			metadata := request(http.MethodGet, "/api/models/?include_channel_models=true", "")
			require.Equal(t, http.StatusOK, metadata.Code)
			assert.Contains(t, metadata.Body.String(), "scope-model-a")
			assert.NotContains(t, metadata.Body.String(), "scope-model-hidden")
			assert.NotContains(t, metadata.Body.String(), channels[2].Name)
			fallbacks := []model.GroupChannelFallback{
				{GroupName: "scope-hidden", ChannelType: 1, FallbackChannelId: ids[2], Enabled: true},
				{GroupName: "scope-visible", ChannelType: 14, FallbackChannelId: ids[0], Enabled: true},
			}
			require.NoError(t, model.DB.Create(&fallbacks).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(&fallbacks).Error) })
			fallbackList := request(http.MethodGet, "/api/channel/group_fallback", "")
			assert.Contains(t, fallbackList.Body.String(), "scope-visible")
			assert.NotContains(t, fallbackList.Body.String(), "scope-hidden")
			for _, test := range []struct{ method, path, body string }{
				{http.MethodGet, fmt.Sprintf("/api/channel/%d", ids[2]), ""},
				{http.MethodGet, fmt.Sprintf("/api/channel/test/%d", ids[2]), ""},
				{http.MethodPost, "/api/channel/status/batch", fmt.Sprintf(`{"ids":[%d,%d],"status":2}`, ids[0], ids[2])},
				{http.MethodPost, "/api/channel/batch", fmt.Sprintf(`{"ids":[%d,%d]}`, ids[0], ids[2])},
				{http.MethodPost, "/api/channel/", `{"channel":{"type":24,"name":"blocked","key":"not-used"}}`},
				{http.MethodPut, "/api/channel/", fmt.Sprintf(`{"id":%d,"type":24}`, ids[0])},
				{http.MethodPost, "/api/channel/fetch_models", fmt.Sprintf(`{"type":1,"channel_id":%d}`, ids[2])},
				{http.MethodPost, "/api/channel/multi_key/manage", fmt.Sprintf(`{"channel_id":%d,"action":"disable"}`, ids[2])},
				{http.MethodPost, "/api/channel/priority/reorder", `{}`},
				{http.MethodPost, "/api/channel/fix", `{}`},
				{http.MethodDelete, "/api/channel/group_fallback?group_name=scope-hidden&channel_type=1", ""},
				{http.MethodPut, "/api/channel/group_fallback", fmt.Sprintf(`{"id":%d,"group_name":"scope-hidden","channel_type":1,"fallback_channel_id":%d}`, fallbacks[0].Id, ids[0])},
			} {
				w := request(test.method, test.path, test.body)
				assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", test.method, test.path, w.Body.String())
				assert.Contains(t, w.Body.String(), "CHANNEL_SCOPE_DENIED")
			}
			deleted := request(http.MethodDelete, "/api/channel/group_fallback?group_name=scope-visible&channel_type=14", "")
			require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
			remaining, err := model.GetGroupChannelFallback("scope-visible", 14)
			require.NoError(t, err)
			assert.Nil(t, remaining)
			visible, err := model.GetChannelById(ids[0], false)
			require.NoError(t, err)
			assert.Equal(t, common.ChannelStatusEnabled, visible.Status)
			assert.Equal(t, 1, visible.Type)
			w := request(http.MethodPost, "/api/channel/tag/disabled", `{"tag":"scope-shared-tag"}`)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			for i, id := range ids {
				channel, err := model.GetChannelById(id, false)
				require.NoError(t, err)
				expected := common.ChannelStatusManuallyDisabled
				if i == 2 {
					expected = common.ChannelStatusEnabled
				}
				assert.Equal(t, expected, channel.Status)
				var ability model.Ability
				require.NoError(t, model.DB.Where("channel_id = ?", id).First(&ability).Error)
				assert.Equal(t, i == 2, ability.Enabled)
			}
			w = request(http.MethodPost, "/api/channel/tag/enabled", `{"tag":"scope-shared-tag"}`)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			w = request(http.MethodPut, "/api/channel/tag", `{"tag":"scope-shared-tag","new_tag":"scope-renamed","models":"scope-updated"}`)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			hidden, err := model.GetChannelById(ids[2], false)
			require.NoError(t, err)
			assert.Equal(t, tag, hidden.GetTag())
			assert.Equal(t, "scope-model-hidden", hidden.Models)
			// Logs use a separate connection and must not join against its channels table.
			logs := []model.Log{
				{UserId: admin.Id, Username: admin.Username, CreatedAt: common.GetTimestamp(), Type: model.LogTypeConsume, ChannelId: ids[0], Content: "visible fixture"},
				{UserId: admin.Id, Username: admin.Username, CreatedAt: common.GetTimestamp(), Type: model.LogTypeConsume, ChannelId: ids[2], Content: "hidden fixture"},
			}
			if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
				// ClickHouse log IDs are generated by the host, not auto-incremented.
				logs[0].Id, logs[1].Id = 10001, 10002
			}
			require.NoError(t, model.LOG_DB.Create(&logs).Error)
			t.Cleanup(func() {
				require.NoError(t, model.LOG_DB.Where("id IN ?", []int{logs[0].Id, logs[1].Id}).Delete(&model.Log{}).Error)
			})
			w = request(http.MethodGet, "/api/log/?username=scope-admin", "")
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "visible fixture")
			assert.NotContains(t, w.Body.String(), "hidden fixture")
			assert.NotContains(t, w.Body.String(), channels[2].Name)
			w = request(http.MethodGet, fmt.Sprintf("/api/log/stat?channel=%d", ids[2]), "")
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var stat struct {
				Success bool
				Data    model.Stat
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &stat))
			require.True(t, stat.Success, w.Body.String())
			assert.Equal(t, 0, stat.Data.Rpm)
			w = request(http.MethodGet, fmt.Sprintf("/api/log/stat?channel=%d", ids[0]), "")
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &stat))
			require.True(t, stat.Success, w.Body.String())
			assert.Equal(t, 1, stat.Data.Rpm)
			// A queued manual task follows the actor's current scope, not its snapshot.
			ctx, err := authz.ChannelTaskContext(context.Background(), admin.Id, authz.ChannelOperate)
			require.NoError(t, err)
			assert.True(t, authz.ChannelTaskAllows(ctx, &channels[0]))
			require.NoError(t, model.SetAdminChannelScopeInTx(model.DB, admin.Id, model.ChannelScope{Mode: "selected", Types: []int{14}}))
			assert.False(t, authz.ChannelTaskAllows(ctx, &channels[0]))
			assert.True(t, authz.ChannelTaskAllows(ctx, &channels[1]))
			role = common.RoleRootUser
			list("/api/channel/", 3)
			role = common.RoleAdminUser
			for _, corrupt := range []string{`null`, `{}`, `{"mode":"selected","types":[]}`, `broken`} {
				require.NoError(t, model.DB.Model(&admin).Update("admin_channel_scope", corrupt).Error)
				assert.Equal(t, http.StatusForbidden, request(http.MethodGet, "/api/channel/", "").Code)
			}
			require.NoError(t, model.SetAdminChannelScopeInTx(model.DB, admin.Id, model.ChannelScope{Mode: "selected", Types: []int{1}}))
			// Full startup twice preserves data and the selected scope.
			for range 2 {
				if model.LOG_DB != model.DB {
					logDB, err := model.LOG_DB.DB()
					require.NoError(t, err)
					require.NoError(t, logDB.Close())
				}
				db, err := model.DB.DB()
				require.NoError(t, err)
				require.NoError(t, db.Close())
				require.NoError(t, model.InitDB())
				require.NoError(t, model.InitLogDB())
			}
			require.NoError(t, authz.Init(model.DB))
			loaded, err = model.GetAdminChannelScope(admin.Id)
			require.NoError(t, err)
			assert.Equal(t, []int{1}, loaded.Types)
			var stored model.User
			require.NoError(t, model.DB.First(&stored, admin.Id).Error)
			assert.Equal(t, admin.DisplayName, stored.DisplayName)
			assert.True(t, model.DB.Migrator().HasIndex(&model.User{}, "idx_users_username"))
			if legacyID != 0 {
				var legacy model.User
				require.NoError(t, model.DB.First(&legacy, legacyID).Error)
				assert.Equal(t, "Existing administrator", legacy.DisplayName)
				assert.EqualValues(t, 7, legacy.AuthVersion)
				scope, err := legacy.ChannelScope()
				require.NoError(t, err)
				assert.False(t, scope.Restricted())
			}
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, model.DB.Raw(query).Scan(&version).Error)
			t.Logf("verified %s %s", dialect, version)
		})
	}
}
