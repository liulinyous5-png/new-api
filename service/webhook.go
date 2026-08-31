package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// WebhookPayload webhook 通知的负载数据
type WebhookPayload struct {
	Type      string        `json:"type"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Values    []interface{} `json:"values,omitempty"`
	Timestamp int64         `json:"timestamp"`
}

type feishuWebhookPayload struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Post struct {
			ZhCN struct {
				Title   string                  `json:"title"`
				Content [][]feishuPostTextBlock `json:"content"`
			} `json:"zh_cn"`
		} `json:"post"`
	} `json:"content"`
}

type feishuInteractivePayload struct {
	MsgType string     `json:"msg_type"`
	Card    feishuCard `json:"card"`
}

type feishuCard struct {
	Config struct {
		WideScreenMode bool `json:"wide_screen_mode"`
	} `json:"config"`
	Header struct {
		Template string `json:"template"`
		Title    struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		} `json:"title"`
	} `json:"header"`
	Elements []feishuCardElement `json:"elements"`
}

type feishuCardElement struct {
	Tag  string `json:"tag"`
	Text *struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
	} `json:"text,omitempty"`
}

type feishuPostTextBlock struct {
	Tag  string `json:"tag"`
	Text string `json:"text"`
}

type feishuWebhookResponse struct {
	Code          *int   `json:"code"`
	Msg           string `json:"msg"`
	StatusCode    *int   `json:"StatusCode"`
	StatusMessage string `json:"StatusMessage"`
}

func generateSignature(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func isFeishuWebhookURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "open.feishu.cn" || host == "open.larksuite.com"
}

func buildFeishuPostPayload(title, content string) ([]byte, error) {
	payload := feishuWebhookPayload{
		MsgType: "post",
	}
	if strings.TrimSpace(title) == "" {
		title = "告警通知"
	}
	payload.Content.Post.ZhCN.Title = title
	content = strings.TrimSpace(content)
	if content == "" {
		content = "（空消息）"
	}
	lines := strings.Split(content, "\n")
	rows := make([][]feishuPostTextBlock, 0, len(lines)+2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		rows = append(rows, []feishuPostTextBlock{{Tag: "text", Text: trimmed}})
	}
	if len(rows) == 0 {
		rows = append(rows, []feishuPostTextBlock{{Tag: "text", Text: content}})
	}
	rows = append(rows, []feishuPostTextBlock{
		{Tag: "text", Text: fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05"))},
	})
	payload.Content.Post.ZhCN.Content = rows
	return common.Marshal(payload)
}

func buildFeishuAlertCardPayload(title, content, emptyFallback string) ([]byte, error) {
	if strings.TrimSpace(title) == "" {
		title = "告警通知"
	}
	if strings.TrimSpace(emptyFallback) == "" {
		emptyFallback = "告警触发，但消息正文为空。"
	}
	card := feishuInteractivePayload{MsgType: "interactive"}
	card.Card.Config.WideScreenMode = true
	card.Card.Header.Template = "red"
	card.Card.Header.Title.Tag = "plain_text"
	card.Card.Header.Title.Content = title

	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		md := trimmed
		if idx := strings.Index(trimmed, "："); idx > 0 && idx < len(trimmed)-1 {
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+len("："):])
			if strings.Contains(strings.ToLower(key), "requestid") {
				md = fmt.Sprintf("**%s**：`%s`", key, val)
			} else {
				md = fmt.Sprintf("**%s**：%s", key, val)
			}
		}
		textNode := struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		}{Tag: "lark_md", Content: md}
		card.Card.Elements = append(card.Card.Elements, feishuCardElement{
			Tag:  "div",
			Text: &textNode,
		})
	}

	if len(card.Card.Elements) == 0 {
		card.Card.Elements = append(card.Card.Elements, feishuCardElement{
			Tag: "div",
			Text: &struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			}{Tag: "lark_md", Content: emptyFallback},
		})
	}
	return common.Marshal(card)
}

func buildWebhookPayload(webhookURL string, data dto.Notify) ([]byte, error) {
	content := data.Content
	for _, value := range data.Values {
		content = fmt.Sprintf(content, value)
	}

	if isFeishuWebhookURL(webhookURL) {
		if data.Type == dto.NotifyTypeChannelAutoDisableAlert || strings.Contains(data.Title, "自动禁用") {
			return buildFeishuAlertCardPayload(data.Title, content, "通道自动禁用告警触发，但消息正文为空。")
		}
		if data.Type == dto.NotifyTypeChannelErrorAlert || strings.Contains(data.Title, "渠道错误") {
			return buildFeishuAlertCardPayload(data.Title, content, "渠道错误告警触发，但消息正文为空。")
		}
		if data.Type == dto.NotifyTypeTTFTAlert || strings.Contains(data.Title, "TTFT") {
			return buildFeishuAlertCardPayload(data.Title, content, "TTFT 告警触发，但消息正文为空。")
		}
		return buildFeishuPostPayload(data.Title, content)
	}

	payload := WebhookPayload{
		Type:      data.Type,
		Title:     data.Title,
		Content:   content,
		Values:    data.Values,
		Timestamp: time.Now().Unix(),
	}
	return common.Marshal(payload)
}

func validateWebhookResponse(webhookURL string, resp *http.Response, body []byte) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(body) == 0 {
			return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		}
		return fmt.Errorf("webhook request failed with status code: %d, body: %s", resp.StatusCode, string(body))
	}
	if isFeishuWebhookURL(webhookURL) {
		var r feishuWebhookResponse
		if len(body) > 0 {
			if err := common.Unmarshal(body, &r); err != nil {
				return fmt.Errorf("failed to parse feishu response: %v, body: %s", err, string(body))
			}
		}
		if r.Code != nil && *r.Code != 0 {
			msg := r.Msg
			if msg == "" {
				msg = "unknown error"
			}
			return fmt.Errorf("feishu webhook business error: code=%d, msg=%s", *r.Code, msg)
		}
		if r.StatusCode != nil && *r.StatusCode != 0 {
			msg := r.StatusMessage
			if msg == "" {
				msg = "unknown error"
			}
			return fmt.Errorf("feishu webhook business error: StatusCode=%d, StatusMessage=%s", *r.StatusCode, msg)
		}
	}
	return nil
}

func SendWebhookNotify(webhookURL string, secret string, data dto.Notify) error {
	payloadBytes, err := buildWebhookPayload(webhookURL, data)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	if system_setting.EnableWorker() {
		workerReq := &WorkerRequest{
			URL:    webhookURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: payloadBytes,
		}
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			workerReq.Headers["X-Webhook-Signature"] = signature
			workerReq.Headers["Authorization"] = "Bearer " + secret
		}
		resp, err := DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send webhook request through worker: %v", err)
		}
		defer resp.Body.Close()
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("failed to read webhook response body: %v", readErr)
		}
		return validateWebhookResponse(webhookURL, resp, bodyBytes)
	}

	if err := ValidateSSRFProtectedFetchURL(webhookURL); err != nil {
		return fmt.Errorf("request reject: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		signature := generateSignature(secret, payloadBytes)
		req.Header.Set("X-Webhook-Signature", signature)
	}
	client := GetSSRFProtectedHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %v", err)
	}
	defer resp.Body.Close()
	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read webhook response body: %v", readErr)
	}
	return validateWebhookResponse(webhookURL, resp, bodyBytes)
}
