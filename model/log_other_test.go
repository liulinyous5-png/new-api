package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogOtherScopesAndMerges(t *testing.T) {
	var other LogOther

	assert.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	other.MergePublic(map[string]any{
		"zero": 0,
	})
	assert.True(t, other.SetAdmin("use_channel", []string{"channel-a"}))
	other.MergeAdmin(map[string]any{
		"rejected": false,
	})
	assert.True(t, other.SetRoot("upstream_request_id", "upstream-private"))
	other.MergeRoot(map[string]any{
		"generation": 0,
	})
	assert.True(t, other.SetAudit("method", "POST"))
	other.MergeAudit(map[string]any{
		"success": false,
	})

	require.JSONEq(t, `{
		"request_path": "/v1/chat/completions",
		"zero": 0,
		"admin_info": {
			"use_channel": ["channel-a"],
			"rejected": false
		},
		"root_info": {
			"upstream_request_id": "upstream-private",
			"generation": 0
		},
		"audit_info": {
			"method": "POST",
			"success": false
		}
	}`, other.JSONString())
}

func TestLogOtherRejectsSensitivePublicFields(t *testing.T) {
	other := NewLogOther()

	for _, key := range []string{
		"admin_info",
		"root_info",
		"audit_info",
		"channel_id",
		"channel_name",
		"channel_type",
		"reject_reason",
	} {
		assert.False(t, other.SetPublic(key, "must-not-leak"), key)
	}
	other.MergePublic(map[string]any{
		"request_path": "/v1/responses",
		"channel_name": "still-must-not-leak",
		"admin_info":   map[string]any{"secret": true},
	})

	require.JSONEq(t, `{"request_path":"/v1/responses"}`, other.JSONString())
	require.JSONEq(t, `{}`, NewLogOther().JSONString())
}

func TestLogOtherJSONStringDoesNotMutateReceiver(t *testing.T) {
	other := NewLogOther()
	require.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	require.True(t, other.SetAdmin("rejected", false))

	before := other.Snapshot()
	first := other.JSONString()
	after := other.Snapshot()
	second := other.JSONString()

	require.Equal(t, before, after)
	require.Equal(t, first, second)
}

func TestApplyClientTraceIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	longASCII := strings.Repeat("a", 130)
	splitRune := strings.Repeat("b", 127) + "你"

	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name: "records each present header",
			headers: map[string]string{
				"X-Trace-Id":        " trace-1 ",
				"x-request-id":      "client-req",
				"X-Conversation-Id": "conv-1",
			},
			want: `{"model_ratio":1,"trace_id":"trace-1","client_request_id":"client-req","session_id":"conv-1"}`,
		},
		{
			name: "omits blank broken and missing headers",
			headers: map[string]string{
				"X-Trace-Id":   "   ",
				"X-Request-ID": "bad\nid",
			},
			want: `{"model_ratio":1}`,
		},
		{
			name: "drops a header that contains a nul",
			headers: map[string]string{
				"x-conversation-id": "bad\x00id",
			},
			want: `{"model_ratio":1}`,
		},
		{
			name: "truncates to 128 bytes without splitting a rune",
			headers: map[string]string{
				"X-Trace-Id":   longASCII,
				"X-Request-ID": splitRune,
			},
			want: fmt.Sprintf(
				`{"model_ratio":1,"trace_id":%q,"client_request_id":%q}`,
				strings.Repeat("a", 128),
				strings.Repeat("b", 127),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			for key, value := range tt.headers {
				context.Request.Header.Set(key, value)
			}

			other := NewLogOther()
			require.True(t, other.SetPublic("model_ratio", 1))
			require.JSONEq(t, tt.want, applyClientTraceIDs(context, other).JSONString())
		})
	}

	t.Run("creates metadata when only headers are present", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		context.Request.Header.Set("X-Trace-Id", "trace-only")

		require.JSONEq(t, `{"trace_id":"trace-only"}`, applyClientTraceIDs(context, nil).JSONString())
	})

	t.Run("leaves metadata unchanged without a request", func(t *testing.T) {
		other := NewLogOther()
		require.True(t, other.SetPublic("model_ratio", 1))
		require.Same(t, other, applyClientTraceIDs(nil, other))

		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		require.Nil(t, applyClientTraceIDs(context, nil))
		require.JSONEq(t, `{"model_ratio":1}`, applyClientTraceIDs(context, other).JSONString())
	})
}
