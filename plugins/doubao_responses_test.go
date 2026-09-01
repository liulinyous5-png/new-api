package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubaoResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "doubao",
		model:     "doubao-seedance-2-0-260128",
		requestBody: map[string]any{
			"model": "doubao-seedance-2-0-260128",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "a running fox"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"},
			}}},
			"seconds": 6,
			"size":    "1920x1080",
		},
		wantAction: "image_to_video",
		wantRequest: map[string]any{
			"model":   "doubao-seedance-2-0-260128",
			"prompt":  "a running fox",
			"images":  []any{"https://cdn.example/frame.png"},
			"seconds": float64(6),
			"metadata": map[string]any{
				"resolution": "1080p",
			},
		},
		wantUsageKeys:  []string{"resolution", "tokens", "video_input"},
		wantVendorName: "doubao",
	})
}

func TestDoubaoOfficialSeedanceModelsAndBillingRatios(t *testing.T) {
	source, err := builtinplugins.Source("doubao")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "doubao"})
	require.NoError(t, err)

	for _, model := range []string{
		"doubao-seedance-2-5-260628",
		"dreamina-seedance-2-0-260128",
		"dreamina-seedance-2-0-fast-260128",
		"dreamina-seedance-2-0-mini-260615",
		"dreamina-seedance-2-5-260628",
	} {
		assert.Contains(t, plugin.Meta.Models, model)
	}

	binding, found := registry.Generation().LookupDeclaredRoute("POST", "/doubao/api/v3/contents/generations/tasks")
	require.True(t, found)
	assert.Equal(t, "doubao", binding.Plugin.Meta.Key)

	value, callErr := plugin.Engine.CallMember(t.Context(), "native", "createTask", map[string]any{
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "dreamina-seedance-2-5-260628",
			"content": []any{
				map[string]any{"type": "text", "text": "first line"},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}},
				map[string]any{"type": "text", "text": "second line"},
			},
			"duration":   6,
			"resolution": "1080p",
		}},
	})
	require.NoError(t, callErr)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.Unmarshal(encoded, &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, "dreamina-seedance-2-5-260628", intent["model"])
	assert.Equal(t, "image_to_video", intent["action"])
	requestBody, ok := intent["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "first line\nsecond line", requestBody["prompt"])
	assert.Equal(t, float64(6), requestBody["seconds"])
	metadata, ok := requestBody["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1080p", metadata["resolution"])

	tests := []struct {
		name       string
		model      string
		resolution string
		video      bool
		want       float64
	}{
		{name: "doubao 2.5 1080p video", model: "doubao-seedance-2-5-260628", resolution: "1080p", video: true, want: 46.0 / 70.0},
		{name: "dreamina 2.0 720p video", model: "dreamina-seedance-2-0-260128", resolution: "720p", video: true, want: 4.3 / 7.0},
		{name: "dreamina 2.0 1080p text", model: "dreamina-seedance-2-0-260128", resolution: "1080p", want: 7.7 / 7.0},
		{name: "dreamina 2.0 4k video", model: "dreamina-seedance-2-0-260128", resolution: "4k", video: true, want: 2.4 / 7.0},
		{name: "dreamina fast video", model: "dreamina-seedance-2-0-fast-260128", resolution: "720p", video: true, want: 3.3 / 5.6},
		{name: "dreamina mini video", model: "dreamina-seedance-2-0-mini-260615", resolution: "480p", video: true, want: 2.1 / 3.5},
		{name: "dreamina 2.5 1080p video", model: "dreamina-seedance-2-5-260628", resolution: "1080p", video: true, want: 7.0 / 10.7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := []any{map[string]any{"type": "text", "text": "test"}}
			if test.video {
				content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}})
			}
			value, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
				"model":         test.model,
				"upstreamModel": "ep-mapped-model",
				"usagePurpose":  "billing_ratios",
				"requestBody": map[string]any{
					"metadata": map[string]any{"resolution": test.resolution, "content": content},
				},
			})
			require.NoError(t, callErr)
			ratios, ok := value.(map[string]any)
			require.True(t, ok)
			assert.InDelta(t, test.want, ratios["video_input_ratio"], 1e-9)
		})
	}
}
