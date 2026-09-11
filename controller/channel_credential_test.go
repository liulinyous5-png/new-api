package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCredentialCreateAppendReplaceAndDelete(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
	input := map[string]any{"account_credentials": true, "mode": "multi_to_single", "multi_key_mode": "polling", "channel": map[string]any{"name": "accounts", "type": 3, "models": "gpt-4o", "group": "default", "other": "2024-10-21", "key": `[{"key":"same","base_url":"https://a.example"},{"key":"same","base_url":"https://b.example"}]`}}
	body, err := common.Marshal(input)
	require.NoError(t, err)
	response := postAddChannel(t, 1, common.RoleRootUser, string(body))
	require.Contains(t, response.Body.String(), `"success":true`)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel).Error)
	assert.Equal(t, 2, channel.ChannelInfo.MultiKeySize)
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusAutoDisabled}
	require.NoError(t, channel.SaveChannelInfo())
	for _, mode := range []string{"append", "replace"} {
		body, err := common.Marshal(map[string]any{"id": channel.Id, "name": channel.Name, "type": 3, "models": "gpt-4o", "group": "default", "other": "2024-10-21", "key_mode": mode, "key": `[{"key":"same","base_url":"https://c.example"}]`})
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("id", 1)
		c.Set("role", common.RoleRootUser)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/channel", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		UpdateChannel(c)
		require.Contains(t, rec.Body.String(), `"success":true`)
		loaded, err := model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		if mode == "append" {
			assert.Equal(t, 3, loaded.ChannelInfo.MultiKeySize)
			assert.Equal(t, map[int]int{0: common.ChannelStatusAutoDisabled}, loaded.ChannelInfo.MultiKeyStatusList)
			// Deleting a preceding account keeps each remaining endpoint with its key.
			deleteBody, err := common.Marshal(map[string]any{"channel_id": channel.Id, "action": "delete_key", "key_index": 0})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Set("id", 1)
			c.Set("role", common.RoleRootUser)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key", strings.NewReader(string(deleteBody)))
			c.Request.Header.Set("Content-Type", "application/json")
			ManageMultiKeys(c)
			require.Contains(t, rec.Body.String(), `"success":true`)
			loaded, err = model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.Len(t, loaded.GetKeys(), 2)
			credential, err := loaded.ResolveCredential(loaded.GetKeys()[0])
			require.NoError(t, err)
			assert.Equal(t, "https://b.example", credential.BaseURL)
		} else {
			assert.Equal(t, 1, loaded.ChannelInfo.MultiKeySize)
			assert.Empty(t, loaded.ChannelInfo.MultiKeyStatusList)
			assert.Zero(t, loaded.ChannelInfo.MultiKeyPollingIndex)
		}
	}
}

func TestChannelCredentialModelDiscoveryUsesAccountEndpointAndSecret(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer server.Close()
	entry, err := common.Marshal(model.ChannelCredential{Key: "secret", BaseURL: server.URL})
	require.NoError(t, err)
	fallback := "http://127.0.0.1:1"
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: string(entry), BaseURL: &fallback, ChannelInfo: model.ChannelInfo{AccountCredentials: true}}
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-4o"}, models)
	assert.Equal(t, "Bearer secret", <-received)
	assert.Equal(t, string(entry), channel.Key)
	assert.Equal(t, fallback, *channel.BaseURL)
}

func TestChannelCredentialLegacyReplacementPreservesExistingStatusContract(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
	channel := model.Channel{Name: "legacy", Type: constant.ChannelTypeOpenAI, Key: "old-a\nold-b", Models: "gpt-4o", Group: "default", ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModeRandom, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}, MultiKeyDisabledReason: map[int]string{0: "existing reason"}}}
	require.NoError(t, model.DB.Create(&channel).Error)
	body, err := common.Marshal(map[string]any{"id": channel.Id, "type": 1, "name": channel.Name, "models": channel.Models, "group": channel.Group, "key": "new-a\nnew-b", "key_mode": "replace"})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/channel", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(c)
	require.Contains(t, rec.Body.String(), `"success":true`)
	loaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "new-a\nnew-b", loaded.Key)
	assert.False(t, loaded.ChannelInfo.AccountCredentials)
	assert.Equal(t, channel.ChannelInfo.MultiKeyStatusList, loaded.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, channel.ChannelInfo.MultiKeyDisabledReason, loaded.ChannelInfo.MultiKeyDisabledReason)
	assert.Equal(t, constant.MultiKeyModeRandom, loaded.ChannelInfo.MultiKeyMode)
}

func TestChannelCredentialOptInPermissionOnlyAppliesToActualModeChanges(t *testing.T) {
	disabled, enabled := false, true
	origin := &model.Channel{}
	assert.False(t, channelHasSensitiveChanges(&PatchChannel{AccountCredentials: &disabled}, origin, map[string]any{"account_credentials": false}))
	assert.True(t, channelHasSensitiveChanges(&PatchChannel{AccountCredentials: &enabled}, origin, map[string]any{"account_credentials": true}))
	origin.ChannelInfo.AccountCredentials = true
	assert.True(t, channelHasSensitiveChanges(&PatchChannel{AccountCredentials: &disabled}, origin, map[string]any{"account_credentials": false}))
}
