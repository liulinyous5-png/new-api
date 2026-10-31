package service

import (
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Keep presentation alongside the typed alert without changing the generic
// Webhook values JSON. Never recover channel boundaries by splitting user text.
type formattedQualityWebhookAlert struct {
	QualityWebhookAlert
	heading string
	details string
}

// The shared Feishu alert card only renders a flat list. Quality audits need a
// container per channel so headings and multiple failed probes stay together.
func buildQualityWebhookCard(notify dto.Notify, timestamp int64) ([]byte, error) {
	elements := make([]any, 0, 2*len(notify.Values)+3)
	summary, _, _ := strings.Cut(notify.Content, "\n\n")
	elements = append(elements, qualityWebhookCardText(summary, false))
	for _, value := range notify.Values {
		channel, ok := value.(formattedQualityWebhookAlert)
		if !ok {
			continue
		}
		// One shaded, full-width container per channel; internal probe text has
		// paragraph spacing, while separators only divide complete channels.
		elements = append(elements, map[string]any{"tag": "hr"}, map[string]any{
			"tag": "column_set", "flex_mode": "none", "background_style": "grey",
			"columns": []any{map[string]any{
				"tag": "column", "width": "weighted", "weight": 1, "vertical_align": "top",
				"elements": []any{
					qualityWebhookCardText(channel.heading, true),
					qualityWebhookCardText(strings.TrimSpace(channel.details), false),
				},
			}},
		})
	}
	if len(elements) == 1 {
		// Preserve caller-supplied text when no typed channel sections exist.
		elements[0] = qualityWebhookCardText(notify.Content, false)
	}
	elements = append(elements, map[string]any{"tag": "hr"}, qualityWebhookCardText(time.Unix(timestamp, 0).Format(time.RFC3339), false))
	return common.Marshal(map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"config":   map[string]any{"wide_screen_mode": true},
			"header":   map[string]any{"template": "red", "title": map[string]any{"tag": "plain_text", "content": "🔴 " + notify.Title}},
			"elements": elements,
		},
	})
}

func qualityWebhookCardText(content string, highlighted bool) map[string]any {
	tag := "plain_text"
	if highlighted {
		// Escape both Feishu tags and Markdown before adding our own emphasis.
		// Channel names remain text even when they contain links or @ mentions.
		content = html.EscapeString(content)
		content = strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "`", "\\`", "~", "\\~").Replace(content)
		content = "**" + strings.ReplaceAll(content, "\n", "**\n**") + "**"
		tag = "lark_md"
	}
	return map[string]any{"tag": "div", "text": map[string]any{"tag": tag, "content": content}}
}
