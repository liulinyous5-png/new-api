package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixClaudeRequestOnFirstRetryFillsEmptyText(t *testing.T) {
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []dto.ClaudeMediaMessage{
					{Type: "text"},
					{Type: "text", Text: ptrString("keep me")},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "messages: text content blocks must be non-empty", false)
	require.Equal(t, 1, changed)

	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 2)
	assert.Equal(t, ".", blocks[0].GetText())
	assert.Equal(t, "keep me", blocks[1].GetText())
}

func TestFixClaudeRequestOnFirstRetryIgnoresUnrelatedEmptyTextError(t *testing.T) {
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []dto.ClaudeMediaMessage{
					{Type: "text"},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "rate limit exceeded", false)
	assert.Equal(t, 0, changed)
	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	assert.Equal(t, "", blocks[0].GetText())
}

func TestFixClaudeRequestOnFirstRetryStripsInvalidThinkingSignature(t *testing.T) {
	thinking := "internal reasoning"
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "thinking", Thinking: &thinking, Signature: "bad-sig"},
					{Type: "text", Text: ptrString("visible")},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "invalid `signature` in `thinking` block", true)
	require.Equal(t, 1, changed)

	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].Type)
	assert.Equal(t, "visible", blocks[0].GetText())
}

func TestFixClaudeRequestOnFirstRetryKeepsThinkingWhenSelfHealDisabled(t *testing.T) {
	thinking := "internal reasoning"
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "thinking", Thinking: &thinking, Signature: "bad-sig"},
					{Type: "text"},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "invalid `signature` in `thinking` block", false)
	assert.Equal(t, 0, changed)
	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 2)
	assert.Equal(t, "thinking", blocks[0].Type)
	assert.Equal(t, "bad-sig", blocks[0].Signature)
	assert.Equal(t, "", blocks[1].GetText())
}

func TestFixClaudeRequestOnFirstRetryFillsEmptyTextWhileSignatureSelfHealDisabled(t *testing.T) {
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "   "},
					map[string]any{"type": "thinking", "thinking": "secret", "signature": "bad-sig"},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "messages: text content blocks must be non-empty; invalid `signature` in `thinking` block", false)
	require.Equal(t, 1, changed)
	blocks, ok := request.Messages[0].Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 2)
	textBlock, ok := blocks[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, ".", textBlock["text"])
	thinkingBlock, ok := blocks[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "thinking", thinkingBlock["type"])
	assert.Equal(t, "bad-sig", thinkingBlock["signature"])
}

func TestFixClaudeRequestOnFirstRetryIgnoresUnrelatedThinkingError(t *testing.T) {
	thinking := "internal reasoning"
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "thinking", Thinking: &thinking, Signature: "bad-sig"},
				},
			},
		},
	}

	changed := FixClaudeRequestOnFirstRetry(request, 400, "overloaded_error", true)
	assert.Equal(t, 0, changed)
	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	assert.Equal(t, "thinking", blocks[0].Type)
}

func ptrString(value string) *string {
	return &value
}
