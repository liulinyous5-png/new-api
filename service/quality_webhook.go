package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// The Secret suffix keeps the entire credential-bearing option out of the
// generic option list. The root-only configuration API returns a masked view.
const QualityWebhookOptionKey = "QualityAuditWebhookSecret"

type QualityWebhookConfig struct {
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url"`
	Secret          string `json:"secret"`
	CooldownSeconds int    `json:"cooldown_seconds"`
}

func GetQualityWebhookConfig() (QualityWebhookConfig, error) {
	cfg := QualityWebhookConfig{CooldownSeconds: 600}
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[QualityWebhookOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if raw == "" {
		return cfg, nil
	}
	if err := common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return QualityWebhookConfig{}, errors.New("invalid quality webhook configuration")
	}
	if err := ValidateQualityWebhookConfig(&cfg); err != nil {
		return QualityWebhookConfig{}, err
	}
	return cfg, nil
}

func ValidateQualityWebhookConfig(cfg *QualityWebhookConfig) error {
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.CooldownSeconds < 60 || cfg.CooldownSeconds > 86400 {
		return errors.New("Webhook cooldown must be between 60 and 86400 seconds")
	}
	if len(cfg.Secret) > 4096 || len(cfg.URL) > 2048 {
		return errors.New("Webhook configuration is too long")
	}
	if cfg.URL == "" {
		if cfg.Enabled {
			return errors.New("Webhook URL is required")
		}
		return nil
	}
	parsed, err := url.ParseRequestURI(cfg.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("Webhook URL must be a valid HTTP or HTTPS URL")
	}
	return nil
}

func SaveQualityWebhookConfig(cfg QualityWebhookConfig) error {
	if err := ValidateQualityWebhookConfig(&cfg); err != nil {
		return err
	}
	encoded, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	return model.UpdateOptionsBulk(map[string]string{QualityWebhookOptionKey: string(encoded)})
}

type QualityWebhookFailure struct {
	RunID             int      `json:"run_id"`
	ProbeID           int      `json:"probe_id"`
	ProbeName         string   `json:"probe_name"`
	Model             string   `json:"model"`
	Assertions        []string `json:"assertions"`
	ActualStatus      int      `json:"actual_status,omitempty"`
	ExpectedStatuses  []int    `json:"expected_statuses,omitempty"`
	UpstreamModel     string   `json:"upstream_model,omitempty"`
	OmittedAssertions int      `json:"omitted_assertions,omitempty"`
}

type QualityWebhookAlert struct {
	EventID         string                  `json:"event_id"`
	ChannelID       int                     `json:"channel_id"`
	ChannelName     string                  `json:"channel_name"`
	SuiteID         int                     `json:"suite_id"`
	SuiteRunID      int                     `json:"suite_run_id"`
	SuiteName       string                  `json:"suite_name"`
	Failures        []QualityWebhookFailure `json:"failures"`
	OmittedFailures int                     `json:"omitted_failures,omitempty"`
}

func BuildQualityWebhookAlert(channel *model.Channel, suite *model.ChannelQualitySuiteRun, runs []*model.ChannelQualityRun) *QualityWebhookAlert {
	alert := &QualityWebhookAlert{EventID: fmt.Sprintf("quality-suite-%d", suite.Id), ChannelID: channel.Id, ChannelName: channel.Name, SuiteID: suite.SuiteId, SuiteRunID: suite.Id, SuiteName: suite.SuiteName}
	for _, run := range runs {
		if run.Verdict != model.ChannelQualityVerdictFailed {
			continue
		}
		var results []struct {
			Name   string `json:"name"`
			Passed *bool  `json:"passed"`
			Error  string `json:"error"`
		}
		// Assertion details are optional: status mismatches fail before assertions run.
		if err := common.UnmarshalJsonStr(run.AssertionResults, &results); err != nil {
			results = nil
		}
		failure := QualityWebhookFailure{RunID: run.Id, ProbeID: run.ProbeId, ProbeName: run.ProbeName, Model: run.RequestedModel, UpstreamModel: run.UpstreamModel}
		// Only copy the numeric status, never the appended upstream response body.
		if suffix, ok := strings.CutPrefix(run.Error, "upstream status "); ok {
			code, _, found := strings.Cut(suffix, ":")
			if status, err := strconv.Atoi(code); found && err == nil && status >= 100 && status <= 599 {
				failure.ActualStatus = status
				var snapshot struct {
					Statuses []int `json:"statuses"`
				}
				if common.UnmarshalJsonStr(run.ProbeSnapshot, &snapshot) == nil {
					failure.ExpectedStatuses = snapshot.Statuses
				}
			}
		}
		for _, result := range results {
			if result.Passed == nil || *result.Passed || result.Error != "" {
				continue
			}
			if len(failure.Assertions) >= 5 {
				failure.OmittedAssertions++
				continue
			}
			name := []rune(result.Name)
			failure.Assertions = append(failure.Assertions, string(name[:min(len(name), 128)]))
		}
		if len(alert.Failures) >= 10 {
			alert.OmittedFailures++
			continue
		}
		alert.Failures = append(alert.Failures, failure)
	}
	if len(alert.Failures) == 0 {
		return nil
	}
	return alert
}

type qualityWebhookCooldownKey struct {
	ChannelID   int
	SuiteID     int
	Destination [32]byte
}

var qualityWebhookCooldowns = struct {
	sync.Mutex
	Until map[qualityWebhookCooldownKey]time.Time
}{Until: make(map[qualityWebhookCooldownKey]time.Time)}

func ClearQualityWebhookCooldown(channelID, suiteID int) {
	qualityWebhookCooldowns.Lock()
	defer qualityWebhookCooldowns.Unlock()
	for key := range qualityWebhookCooldowns.Until {
		if key.ChannelID == channelID && key.SuiteID == suiteID {
			delete(qualityWebhookCooldowns.Until, key)
		}
	}
}

// DeliverQualityWebhooks runs after all channel results have been saved. A
// bounded total deadline prevents a slow receiver from stalling the scheduler.
// Delivery is best effort; a failed attempt does not start the cooldown and
// the next failed scan can try again. Receivers can deduplicate by event_id.
func DeliverQualityWebhooks(parent context.Context, alerts []QualityWebhookAlert) (int, error) {
	cfg, err := GetQualityWebhookConfig()
	if err != nil || !cfg.Enabled || len(alerts) == 0 {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	destination := sha256.Sum256([]byte(cfg.URL + "\x00" + cfg.Secret))
	delivered := 0
	for start := 0; start < len(alerts); start += 20 {
		if ctx.Err() != nil {
			return delivered, ctx.Err()
		}
		var batch []QualityWebhookAlert
		var keys []qualityWebhookCooldownKey
		now := time.Now()
		qualityWebhookCooldowns.Lock()
		for key, until := range qualityWebhookCooldowns.Until {
			if !until.After(now) {
				delete(qualityWebhookCooldowns.Until, key)
			}
		}
		for _, alert := range alerts[start:min(start+20, len(alerts))] {
			key := qualityWebhookCooldownKey{ChannelID: alert.ChannelID, SuiteID: alert.SuiteID, Destination: destination}
			if qualityWebhookCooldowns.Until[key].After(now) {
				continue
			}
			// Bound process-local state even when channels/configurations churn.
			if len(qualityWebhookCooldowns.Until) >= 65536 {
				continue
			}
			qualityWebhookCooldowns.Until[key] = now.Add(30 * time.Second)
			batch = append(batch, alert)
			keys = append(keys, key)
		}
		qualityWebhookCooldowns.Unlock()
		if len(batch) == 0 {
			continue
		}
		notify := FormatQualityWebhookNotification(batch, "")
		var sendErr error
		for range 2 {
			sendErr = sendQualityWebhook(ctx, cfg, notify)
			if sendErr == nil || ctx.Err() != nil {
				break
			}
		}
		qualityWebhookCooldowns.Lock()
		for _, key := range keys {
			if sendErr != nil {
				delete(qualityWebhookCooldowns.Until, key)
			} else {
				qualityWebhookCooldowns.Until[key] = time.Now().Add(time.Duration(cfg.CooldownSeconds) * time.Second)
			}
		}
		qualityWebhookCooldowns.Unlock()
		if sendErr != nil {
			return delivered, fmt.Errorf("quality webhook delivery failed: %w", sendErr)
		}
		delivered += len(batch)
	}
	return delivered, nil
}

// FormatQualityWebhookNotification keeps generic JSON and Feishu alerts readable
// with the same explicit field labels. Values retain machine-readable details.
func FormatQualityWebhookNotification(alerts []QualityWebhookAlert, language string) dto.Notify {
	if language == "" {
		language = common.DefaultLanguage
		if language == "" {
			language = "zh-CN"
		}
	}
	values := make([]any, 0, len(alerts))
	var content strings.Builder
	content.WriteString(i18n.Translate(language, "quality.summary", map[string]any{"Count": len(alerts)}))
	for _, alert := range alerts {
		channelDetails := i18n.Translate(language, "quality.channel_details", map[string]any{"ChannelID": alert.ChannelID, "ChannelName": alert.ChannelName, "SuiteID": alert.SuiteID, "SuiteName": alert.SuiteName, "RunID": alert.SuiteRunID})
		var details strings.Builder
		for _, failure := range alert.Failures {
			details.WriteString("\n\n")
			details.WriteString(i18n.Translate(language, "quality.probe_details", map[string]any{"ProbeID": failure.ProbeID, "ProbeName": failure.ProbeName, "Model": failure.Model, "RunID": failure.RunID}))
			if failure.UpstreamModel != "" && failure.UpstreamModel != failure.Model {
				details.WriteString("\n" + i18n.Translate(language, "quality.upstream_model", map[string]any{"Model": failure.UpstreamModel}))
			}
			reason := i18n.Translate(language, "quality.probe_failed")
			if failure.ActualStatus != 0 {
				reason = i18n.Translate(language, "quality.status_mismatch", map[string]any{"Actual": failure.ActualStatus, "Expected": fmt.Sprint(failure.ExpectedStatuses)})
			} else if len(failure.Assertions) > 0 {
				reason = i18n.Translate(language, "quality.assertions_failed", map[string]any{"Assertions": strings.Join(failure.Assertions, "; ")})
			}
			details.WriteString("\n" + i18n.Translate(language, "quality.failure_reason", map[string]any{"Reason": reason}))
			if failure.OmittedAssertions > 0 {
				details.WriteString("\n" + i18n.Translate(language, "quality.omitted_assertions", map[string]any{"Count": failure.OmittedAssertions}))
			}
		}
		if alert.OmittedFailures > 0 {
			details.WriteString("\n" + i18n.Translate(language, "quality.omitted_probes", map[string]any{"Count": alert.OmittedFailures}))
		}
		values = append(values, formattedQualityWebhookAlert{QualityWebhookAlert: alert, heading: channelDetails, details: details.String()})
		content.WriteString("\n\n" + channelDetails + details.String())
	}
	// Preserve the existing event type for receivers that filter notifications.
	return dto.NewNotify("quality_assertion_failed", i18n.Translate(language, "quality.probe_alert"), content.String(), values)
}

// BuildQualityWebhookPayload applies the destination's signing protocol.
// Feishu authenticates timestamp + newline + secret as the HMAC key, with an
// empty message, and requires the timestamp and Base64 signature in the JSON.
func BuildQualityWebhookPayload(cfg QualityWebhookConfig, notify dto.Notify, timestamp int64) ([]byte, error) {
	body, err := buildWebhookPayload(cfg.URL, notify)
	if isFeishuWebhookURL(cfg.URL) && notify.Type == "quality_assertion_failed" {
		body, err = buildQualityWebhookCard(notify, timestamp)
	}
	if err != nil || cfg.Secret == "" || !isFeishuWebhookURL(cfg.URL) {
		return body, err
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	stamp := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(stamp+"\n"+cfg.Secret))
	payload["timestamp"] = stamp
	payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return common.Marshal(payload)
}

// Errors returned by this transport must exclude URLs, credentials and raw
// response bodies: they are safe to include in the delivery failure log.
func sendQualityWebhook(parent context.Context, cfg QualityWebhookConfig, notify dto.Notify) error {
	if err := ValidateSSRFProtectedFetchURL(cfg.URL); err != nil {
		return errors.New("webhook destination rejected by fetch policy")
	}
	body, err := BuildQualityWebhookPayload(cfg, notify, time.Now().Unix())
	if err != nil {
		return errors.New("webhook payload encoding failed")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return errors.New("webhook request creation failed")
	}
	request.Header.Set("Content-Type", "application/json")
	if cfg.Secret != "" && !isFeishuWebhookURL(cfg.URL) {
		request.Header.Set("X-Webhook-Signature", generateSignature(cfg.Secret, body))
	}
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		return errors.New("webhook HTTP client is unavailable")
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("webhook request interrupted: %w", ctx.Err())
		}
		return errors.New("webhook connection failed")
	}
	defer response.Body.Close()
	result, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil {
		return errors.New("webhook response read failed")
	}
	if len(result) > 65536 {
		return errors.New("webhook response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP status %d", response.StatusCode)
	}
	if isFeishuWebhookURL(cfg.URL) {
		var reply feishuWebhookResponse
		if err := common.Unmarshal(result, &reply); err != nil || (reply.Code == nil && reply.StatusCode == nil) {
			return errors.New("invalid Feishu webhook response")
		}
		if reply.Code != nil && *reply.Code != 0 {
			return fmt.Errorf("Feishu webhook rejected notification: code=%d", *reply.Code)
		}
		if reply.StatusCode != nil && *reply.StatusCode != 0 {
			return fmt.Errorf("Feishu webhook rejected notification: code=%d", *reply.StatusCode)
		}
	}
	return nil
}
