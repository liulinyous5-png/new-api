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

	changed := FixClaudeRequestOnFirstRetry(request, 400, "messages: text content blocks must be non-empty")
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

	changed := FixClaudeRequestOnFirstRetry(request, 400, "rate limit exceeded")
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

	changed := FixClaudeRequestOnFirstRetry(request, 400, "invalid `signature` in `thinking` block")
	require.Equal(t, 1, changed)

	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].Type)
	assert.Equal(t, "visible", blocks[0].GetText())
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

	changed := FixClaudeRequestOnFirstRetry(request, 400, "overloaded_error")
	assert.Equal(t, 0, changed)
	blocks, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	assert.Equal(t, "thinking", blocks[0].Type)
}

func ptrString(value string) *string {
	return &value
}
