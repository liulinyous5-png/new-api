package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const ChannelQualityClearToolUsesExpr = `any(context_management?.applied_edits ?? [], {
  .type == "clear_tool_uses_20250919" &&
  int(.cleared_tool_uses) == 3 &&
  float(.cleared_input_tokens) >= 1
})`

func EnsureBuiltinChannelQualityProbe() error {
	headers, err := common.Marshal(map[string]string{
		"anthropic-beta":    "context-management-2025-06-27",
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return err
	}
	body, err := clearToolUsesProbeBody()
	if err != nil {
		return err
	}
	statuses, err := common.Marshal([]int{200})
	if err != nil {
		return err
	}
	assertions, err := common.Marshal([]ChannelQualityAssertion{{
		Name: "clear tool uses applied",
		Expr: ChannelQualityClearToolUsesExpr,
	}})
	if err != nil {
		return err
	}

	existing := &ChannelQualityProbe{}
	err = DB.Where("name = ?", ChannelQualityProbeNameClearToolUses).First(existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if !existing.Builtin {
			return nil
		}
		if existing.Headers == string(headers) && existing.Body == string(body) && existing.Statuses == string(statuses) && existing.Assertions == string(assertions) {
			return nil
		}
		return DB.Model(&ChannelQualityProbe{}).Where("id = ?", existing.Id).Updates(map[string]any{
			"headers":    string(headers),
			"body":       string(body),
			"statuses":   string(statuses),
			"assertions": string(assertions),
			"updated_at": common.GetTimestamp(),
		}).Error
	}

	probe := &ChannelQualityProbe{
		Name:       ChannelQualityProbeNameClearToolUses,
		Builtin:    true,
		Headers:    string(headers),
		Body:       string(body),
		Statuses:   string(statuses),
		Assertions: string(assertions),
		CreatedAt:  common.GetTimestamp(),
		UpdatedAt:  common.GetTimestamp(),
	}
	return DB.Create(probe).Error
}

func clearToolUsesProbeBody() ([]byte, error) {
	toolText := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 80)
	toolResult := func(id string) map[string]any {
		return map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": id,
					"content":     toolText,
				},
			},
		}
	}
	toolUse := func(id string, label string, input string) map[string]any {
		return map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": label},
				map[string]any{
					"type":  "tool_use",
					"id":    id,
					"name":  "echo",
					"input": map[string]any{"text": input},
				},
			},
		}
	}
	body := map[string]any{
		"model":      ChannelQualityModelPlaceholder,
		"max_tokens": 10,
		"context_management": map[string]any{
			"edits": []any{
				map[string]any{
					"type": "clear_tool_uses_20250919",
					"trigger": map[string]any{
						"type":  "input_tokens",
						"value": 2000,
					},
					"keep": map[string]any{
						"type":  "tool_uses",
						"value": 1,
					},
					"clear_at_least": map[string]any{
						"type":  "input_tokens",
						"value": 500,
					},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"name":        "echo",
				"description": "Echo back its input verbatim",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
					},
					"required": []any{"text"},
				},
			},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "Use the echo tool four times with different inputs."},
			toolUse("tool_1", "Echo 1.", "a"),
			toolResult("tool_1"),
			toolUse("tool_2", "Echo 2.", "b"),
			toolResult("tool_2"),
			toolUse("tool_3", "Echo 3.", "c"),
			toolResult("tool_3"),
			toolUse("tool_4", "Echo 4.", "d"),
			toolResult("tool_4"),
			map[string]any{"role": "user", "content": "Summarize all four echoes briefly."},
		},
	}
	return common.Marshal(body)
}
