package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCredentialNormalize(t *testing.T) {
	tests := []struct {
		name, input, want string
		invalid           bool
	}{
		{name: "legacy keys", input: " one\n two \n", want: "one\ntwo"},
		{name: "array binds endpoints", input: `[{"base_url":"https://a.example/","key":"same"},{"key":"same","base_url":"https://b.example"}]`, want: "{\"key\":\"same\",\"base_url\":\"https://a.example\"}\n{\"key\":\"same\",\"base_url\":\"https://b.example\"}"},
		{name: "pretty object with fallback", input: "{\n\"key\":\"one\"\n}", want: `{"key":"one"}`},
		{name: "empty list", input: `[]`, invalid: true},
		{name: "missing key", input: `[{"base_url":"https://a.example"}]`, invalid: true},
		{name: "empty key", input: `[{"key":" "}]`, invalid: true},
		{name: "broken JSON", input: `[{"key":"secret"}`, invalid: true},
		{name: "non object", input: `["secret"]`, invalid: true},
		{name: "bad scheme", input: `{"key":"secret","base_url":"file:///tmp/a"}`, invalid: true},
		{name: "URL credentials", input: `{"key":"secret","base_url":"https://user:password@a.example"}`, invalid: true},
		{name: "URL query", input: `{"key":"secret","base_url":"https://a.example?key=secret"}`, invalid: true},
		{name: "header injection", input: `{"key":"secret\r\nx: y"}`, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := Channel{Type: constant.ChannelTypeAzure, Key: tt.input, ChannelInfo: ChannelInfo{AccountCredentials: true}}
			err := channel.NormalizeAccountCredentials()
			if tt.invalid {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "secret")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, channel.Key)
			require.NoError(t, channel.NormalizeAccountCredentials())
			assert.Equal(t, tt.want, channel.Key)
		})
	}
}

func TestChannelCredentialPollingWrapsAndSkipsDisabledAccounts(t *testing.T) {
	oldCache, oldEnabled := channelsIDM, common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { channelsIDM = oldCache; common.MemoryCacheEnabled = oldEnabled })
	channel := &Channel{Id: 92001, Type: constant.ChannelTypeAzure, ChannelInfo: ChannelInfo{AccountCredentials: true, IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling}}
	entries := make([]ChannelCredential, 200)
	for i := range entries {
		entries[i] = ChannelCredential{Key: fmt.Sprintf("key-%d", i+1), BaseURL: fmt.Sprintf("https://account-%d.example", i+1)}
	}
	raw, err := common.Marshal(entries)
	require.NoError(t, err)
	channel.Key = string(raw)
	require.NoError(t, channel.NormalizeAccountCredentials())
	channelsIDM = map[int]*Channel{channel.Id: channel}
	// Database-loaded copies must still advance the shared cursor.
	for i := 0; i < 201; i++ {
		copy := *channel
		entry, index, apiErr := copy.GetNextEnabledKey()
		require.Nil(t, apiErr)
		assert.Equal(t, i%200, index)
		credential, err := copy.ResolveCredential(entry)
		require.NoError(t, err)
		assert.Equal(t, entries[i%200], credential)
	}
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusAutoDisabled}
	entry, index, apiErr := channel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, 2, index)
	credential, err := channel.ResolveCredential(entry)
	require.NoError(t, err)
	assert.Equal(t, entries[2], credential)
}

func TestChannelCredentialStatusDistinguishesSameSecretAtDifferentEndpoints(t *testing.T) {
	channel := Channel{Type: constant.ChannelTypeAzure, Key: `[{"key":"same","base_url":"https://a.example"},{"key":"same","base_url":"https://b.example"}]`, ChannelInfo: ChannelInfo{AccountCredentials: true, IsMultiKey: true}}
	require.NoError(t, channel.NormalizeAccountCredentials())
	handlerMultiKeyUpdate(&channel, channel.GetKeys()[1], common.ChannelStatusAutoDisabled, "rejected")
	assert.Equal(t, map[int]int{1: common.ChannelStatusAutoDisabled}, channel.ChannelInfo.MultiKeyStatusList)
	handlerMultiKeyUpdate(&channel, channel.GetKeys()[1], common.ChannelStatusEnabled, "")
	assert.Empty(t, channel.ChannelInfo.MultiKeyStatusList)
}

func TestChannelCredentialLegacyKeysAreNeverReinterpretedWithoutOptIn(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeVertexAi} {
		for _, key := range []string{"  original-key  ", "key-one\nkey-two\n", `{"key":"legacy-json","base_url":"https://must-not-be-used.example"}`, `[{"private_key":"legacy-service-account"}]`} {
			channel := Channel{Type: channelType, Key: key}
			require.NoError(t, channel.NormalizeAccountCredentials())
			assert.Equal(t, key, channel.Key)
			credential, err := channel.ResolveCredential(key)
			require.NoError(t, err)
			assert.Equal(t, key, credential.Key)
			assert.Empty(t, credential.BaseURL)
			require.NoError(t, channel.ValidateAccountEndpoints())
		}
	}
	encoded, err := common.Marshal(ChannelInfo{})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "account_credentials")
}
