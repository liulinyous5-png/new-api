package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestChannelCredentialSelectionAndRetryKeepEndpointAndKeyTogether(t *testing.T) {
	fallback := "https://fallback.example"
	channel := &model.Channel{Type: constant.ChannelTypeAzure, BaseURL: &fallback, Key: `[{"key":"same","base_url":"https://a.example"},{"key":"same","base_url":"https://b.example"},{"key":"legacy"}]`, ChannelInfo: model.ChannelInfo{AccountCredentials: true, IsMultiKey: true}}
	require.NoError(t, channel.NormalizeAccountCredentials())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	for i, want := range []struct{ key, url string }{{"same", "https://a.example"}, {"same", "https://b.example"}, {"legacy", fallback}} {
		require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o", i))
		assert.Equal(t, want.key, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		assert.Equal(t, want.url, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
		assert.Equal(t, channel.GetKeys()[i], common.GetChannelCredentialIdentity(c))
		assert.Equal(t, i, common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex))
	}
	require.NotNil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o", 3))
	require.NotNil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o", -1))
	single := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "plain", BaseURL: &fallback}
	require.Nil(t, SetupContextForSelectedChannel(c, single, "gpt-4o"))
	assert.Equal(t, "plain", common.GetChannelCredentialIdentity(c))
	assert.Equal(t, fallback, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey))
}

func TestChannelCredentialLegacyJSONAndRandomDisabledSelectionRemainUnchanged(t *testing.T) {
	baseURL := "https://legacy.example"
	for _, key := range []string{"legacy-key", `{"key":"embedded","base_url":"https://ignored.example"}`} {
		channel := &model.Channel{Type: constant.ChannelTypeOpenAI, BaseURL: &baseURL, Key: key}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o"))
		assert.Equal(t, key, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		assert.Equal(t, baseURL, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
	}
	channel := &model.Channel{Type: constant.ChannelTypeAzure, BaseURL: &baseURL, Key: "disabled\nenabled", ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModeRandom, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-4o"))
	assert.Equal(t, "enabled", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	assert.Equal(t, baseURL, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
}
