package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadPreservesUsePersonCharacter(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "test prompt",
		Metadata: map[string]interface{}{
			"use_person_character": true,
		},
	}

	body, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convertToRequestPayload() error = %v", err)
	}
	if body.UsePersonCharacter == nil || !bool(*body.UsePersonCharacter) {
		t.Fatalf("UsePersonCharacter = %v, want true", body.UsePersonCharacter)
	}
}

func TestDoResponseReturnsUpstreamTaskIDForSeedance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{"id":"upstream_original"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-260628",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}

	taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_original", taskID)
	assert.True(t, common.GetContextKeyBool(context, constant.ContextKeyExposeUpstreamTaskID))
	assert.Equal(t, "upstream_original", common.GetContextKeyString(context, constant.ContextKeyUpstreamTaskID))

	var body struct {
		ID     string `json:"id"`
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "upstream_original", body.ID)
	assert.Equal(t, "upstream_original", body.TaskID)
}

func TestConvertToRequestPayloadPreservesDreaminaParameters(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "dreamina-seedance-2-5-260628",
		Prompt: "extend the reference video",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "video_url",
					"video_url": map[string]interface{}{
						"url": "https://example.com/reference.mov",
					},
					"role": "reference_video",
				},
			},
			"omni_reference_task_type": "extend",
			"output_format":            "mov",
			"ratio":                    "adaptive",
			"duration":                 -1,
		},
	}

	body, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, "extend", body.OmniReferenceTaskType)
	assert.Equal(t, "mov", body.OutputFormat)
	require.Len(t, body.Content, 2)
	assert.Equal(t, "reference_video", body.Content[0].Role)
	assert.Equal(t, "https://example.com/reference.mov", body.Content[0].VideoURL.URL)
	assert.Equal(t, dto.IntValue(-1), *body.Duration)
}
