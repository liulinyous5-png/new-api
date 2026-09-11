package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// ChannelCredential keeps an account's endpoint and secret together in the
// existing key field, so key removal and reordering cannot separate the pair.
type ChannelCredential struct {
	Key     string `json:"key"`
	BaseURL string `json:"base_url,omitempty"`
}

func (channel *Channel) UsesAccountCredentials() bool {
	return channel.ChannelInfo.AccountCredentials && (channel.Type == constant.ChannelTypeOpenAI || channel.Type == constant.ChannelTypeAzure)
}

// ResolveCredential accepts legacy keys as well as account objects. Errors must
// never include the input: it contains an administrator's upstream secret.
func (channel *Channel) ResolveCredential(entry string) (ChannelCredential, error) {
	credential := ChannelCredential{Key: entry}
	if channel.UsesAccountCredentials() && strings.HasPrefix(strings.TrimSpace(entry), "{") {
		credential = ChannelCredential{}
		if err := common.Unmarshal([]byte(entry), &credential); err != nil {
			return ChannelCredential{}, errors.New("invalid account JSON")
		}
		credential.Key = strings.TrimSpace(credential.Key)
		credential.BaseURL = strings.TrimRight(strings.TrimSpace(credential.BaseURL), "/")
		if credential.Key == "" || strings.ContainsAny(credential.Key, "\r\n") {
			return ChannelCredential{}, errors.New("account key must be non-empty and contain no line breaks")
		}
		if credential.BaseURL != "" {
			parsed, err := url.Parse(credential.BaseURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(credential.BaseURL, "?#") {
				return ChannelCredential{}, errors.New("account base_url must be an HTTP(S) URL without credentials, query or fragment")
			}
		}
	}
	return credential, nil
}

// NormalizeAccountCredentials canonicalizes JSON arrays and JSON lines. Plain
// key lines keep their existing representation and use the channel's base URL.
func (channel *Channel) NormalizeAccountCredentials() error {
	if channel.ChannelInfo.AccountCredentials && !channel.UsesAccountCredentials() {
		return errors.New("account credentials require an OpenAI or Azure channel")
	}
	if !channel.UsesAccountCredentials() || strings.TrimSpace(channel.Key) == "" {
		return nil
	}
	input := strings.TrimSpace(channel.Key)
	entries := strings.Split(input, "\n")
	if strings.HasPrefix(input, "[") {
		var array []json.RawMessage
		if err := common.Unmarshal([]byte(input), &array); err != nil || len(array) == 0 {
			return errors.New("account list must be a non-empty JSON array")
		}
		entries = make([]string, len(array))
		for i, entry := range array {
			if !strings.HasPrefix(strings.TrimSpace(string(entry)), "{") {
				return fmt.Errorf("account %d must be a JSON object", i+1)
			}
			entries[i] = string(entry)
		}
	} else if strings.HasPrefix(input, "{") {
		var object map[string]json.RawMessage
		if common.Unmarshal([]byte(input), &object) == nil {
			entries = []string{input}
		}
	}
	normalized := make([]string, 0, len(entries))
	for i, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		credential, err := channel.ResolveCredential(entry)
		if err != nil {
			return fmt.Errorf("account %d: %w", i+1, err)
		}
		if strings.HasPrefix(entry, "{") {
			encoded, err := common.Marshal(credential)
			if err != nil {
				return errors.New("failed to encode account")
			}
			entry = string(encoded)
		}
		normalized = append(normalized, entry)
	}
	channel.Key = strings.Join(normalized, "\n")
	channel.Keys = nil
	return nil
}

// ValidateAccountEndpoints ensures Azure accounts without an override have a
// shared endpoint to fall back to. Other providers retain their default URL.
func (channel *Channel) ValidateAccountEndpoints() error {
	if !channel.UsesAccountCredentials() || channel.Type != constant.ChannelTypeAzure || channel.GetBaseURL() != "" {
		return nil
	}
	for i, entry := range channel.GetKeys() {
		credential, err := channel.ResolveCredential(entry)
		if err != nil {
			return fmt.Errorf("account %d: %w", i+1, err)
		}
		if credential.BaseURL == "" {
			return fmt.Errorf("account %d requires base_url or a channel default", i+1)
		}
	}
	return nil
}
