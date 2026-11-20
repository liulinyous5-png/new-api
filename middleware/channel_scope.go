package middleware

import (
	"bytes"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// RequireChannelScope complements action permissions with object authorization.
// New collection routes fail closed for restricted administrators until their
// query/mutation semantics have explicitly been scoped below.
func RequireChannelScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loadChannelScope(c) {
			return
		}
		scope := model.ChannelScopeFromContext(c.Request.Context())
		if !scope.Restricted() {
			c.Next()
			return
		}
		deny := func() {
			abortWithWebMessage(c, http.StatusForbidden, "CHANNEL_SCOPE_DENIED", common.NewMessage("Channel is unavailable or outside your visibility scope"))
		}
		if rawID := c.Param("id"); rawID != "" {
			id, err := strconv.Atoi(rawID)
			if err != nil || !channelIDsAllowed(scope, []int{id}) {
				deny()
				return
			}
			c.Next()
			return
		}
		path := c.FullPath()
		if path == "/api/channel/priority/reorder" || path == "/api/channel/fix" {
			abortWithWebMessage(c, http.StatusForbidden, "CHANNEL_SCOPE_DENIED", common.NewMessage("This operation requires access to all channel types"))
			return
		}
		if c.Request.Method == http.MethodGet {
			switch path {
			case "/api/channel/", "/api/channel/search", "/api/channel/models", "/api/channel/default_base_urls", "/api/channel/models_enabled", "/api/channel/ops", "/api/channel/priority/order", "/api/channel/group_fallback", "/api/channel/tag/models", "/api/channel/test", "/api/channel/update_balance", "/api/channel/metrics":
				c.Next()
				return
			default:
				deny()
				return
			}
		}
		if path == "/api/channel/disabled" || path == "/api/channel/upstream_updates/apply_all" || path == "/api/channel/upstream_updates/detect_all" {
			c.Next()
			return
		}
		var req struct {
			ID        int   `json:"id"`
			ChannelID int   `json:"channel_id"`
			IDs       []int `json:"ids"`
			Type      *int  `json:"type"`
			Channel   *struct {
				Type int `json:"type"`
			} `json:"channel"`
			GroupName         string `json:"group_name"`
			ChannelType       int    `json:"channel_type"`
			FallbackChannelID int    `json:"fallback_channel_id"`
		}
		body, err := c.GetRawData()
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		if err != nil || common.Unmarshal(body, &req) != nil {
			if path == "/api/channel/group_fallback" && c.Request.Method == http.MethodDelete {
				req.ChannelType, _ = strconv.Atoi(c.Query("channel_type"))
				req.GroupName = c.Query("group_name")
				if req.GroupName == "" {
					req.GroupName = c.Query("group")
				}
			} else {
				deny()
				return
			}
		}
		var ids []int
		switch path {
		case "/api/channel/":
			if c.Request.Method == http.MethodPost {
				if req.Channel == nil || !scope.Allows(req.Channel.Type) {
					deny()
					return
				}
			} else {
				ids = []int{req.ID}
				if req.Type != nil && !scope.Allows(*req.Type) {
					deny()
					return
				}
			}
		case "/api/channel/status/batch", "/api/channel/batch", "/api/channel/batch/tag":
			if len(req.IDs) == 0 {
				deny()
				return
			}
			ids = req.IDs
		case "/api/channel/upstream_updates/apply", "/api/channel/upstream_updates/detect":
			ids = []int{req.ID}
		case "/api/channel/multi_key/manage", "/api/channel/ollama/pull", "/api/channel/ollama/pull/stream", "/api/channel/ollama/delete":
			ids = []int{req.ChannelID}
		case "/api/channel/fetch_models":
			if req.Type == nil || !scope.Allows(*req.Type) {
				deny()
				return
			}
			if req.ChannelID != 0 {
				ids = []int{req.ChannelID}
			}
		case "/api/channel/group_fallback":
			if !scope.Allows(req.ChannelType) {
				deny()
				return
			}
			var existing *model.GroupChannelFallback
			if req.ID > 0 && c.Request.Method != http.MethodDelete {
				existing, err = model.GetGroupChannelFallbackByID(req.ID)
			} else {
				existing, err = model.GetGroupChannelFallback(strings.TrimSpace(req.GroupName), req.ChannelType)
			}
			if err != nil || (existing != nil && (!scope.Allows(existing.ChannelType) || !channelIDsAllowed(scope, []int{existing.FallbackChannelId}))) {
				deny()
				return
			}
			if c.Request.Method != http.MethodDelete {
				ids = []int{req.FallbackChannelID}
			}
		case "/api/channel/tag", "/api/channel/tag/disabled", "/api/channel/tag/enabled", "/api/channel/upstream_updates/apply_all", "/api/channel/upstream_updates/detect_all":
			// Collection handlers apply the scope to every affected channel.
		default:
			deny()
			return
		}
		if len(ids) > 0 && !channelIDsAllowed(scope, ids) {
			deny()
			return
		}
		c.Next()
	}
}

func channelIDsAllowed(scope model.ChannelScope, ids []int) bool {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 || ids[0] <= 0 {
		return false
	}
	var count int64
	err := scope.Apply(model.DB.Model(&model.Channel{})).Where("id IN ?", ids).Count(&count).Error
	return err == nil && count == int64(len(ids))
}

// LoadChannelScope scopes channel references exposed by other management APIs.
func LoadChannelScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		if loadChannelScope(c) {
			c.Next()
		}
	}
}

func loadChannelScope(c *gin.Context) bool {
	scope := model.ChannelScope{Mode: "all", Types: []int{}}
	if c.GetInt("role") != common.RoleRootUser {
		var err error
		scope, err = model.GetAdminChannelScope(c.GetInt("id"))
		if err != nil {
			abortWithWebMessage(c, http.StatusForbidden, "CHANNEL_SCOPE_DENIED", common.NewMessage("Channel visibility could not be verified"))
			return false
		}
	}
	c.Request = c.Request.WithContext(model.WithChannelScope(c.Request.Context(), scope))
	return true
}
