package qualityexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const clearToolUsesExpr = `any(context_management?.applied_edits ?? [], {
  .type == "clear_tool_uses_20250919" &&
  int(.cleared_tool_uses) == 3 &&
  float(.cleared_input_tokens) >= 1
})`

func appliedEditsEnv(clearedUses float64, clearedTokens float64) map[string]any {
	return map[string]any{
		"context_management": map[string]any{
			"applied_edits": []any{
				map[string]any{
					"type":                 "clear_tool_uses_20250919",
					"cleared_tool_uses":    clearedUses,
					"cleared_input_tokens": clearedTokens,
				},
			},
		},
	}
}

func TestClearToolUsesExpression(t *testing.T) {
	prog, err := Compile(clearToolUsesExpr)
	require.NoError(t, err)

	passed, err := prog.Run(appliedEditsEnv(3, 10))
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = prog.Run(appliedEditsEnv(1, 10))
	require.NoError(t, err)
	assert.False(t, passed)

	passed, err = prog.Run(appliedEditsEnv(3, 0))
	require.NoError(t, err)
	assert.False(t, passed)

	passed, err = prog.Run(map[string]any{})
	require.NoError(t, err)
	assert.False(t, passed)
}

func TestJSONNumberEqualsIntegerLiteral(t *testing.T) {
	prog, err := Compile(`any(context_management.applied_edits, { .cleared_tool_uses == 3 })`)
	require.NoError(t, err)
	passed, err := prog.Run(appliedEditsEnv(3, 10))
	require.NoError(t, err)
	assert.True(t, passed)
	passed, err = prog.Run(appliedEditsEnv(1, 10))
	require.NoError(t, err)
	assert.False(t, passed)
}

func TestNotEmpty(t *testing.T) {
	prog, err := Compile(`notEmpty(usage?.output)`)
	require.NoError(t, err)

	passed, err := prog.Run(map[string]any{"usage": map[string]any{"output": "ok"}})
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = prog.Run(map[string]any{"usage": map[string]any{"output": "  "}})
	require.NoError(t, err)
	assert.False(t, passed)

	passed, err = prog.Run(map[string]any{"usage": map[string]any{"output": 0}})
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = prog.Run(map[string]any{})
	require.NoError(t, err)
	assert.False(t, passed)

	passed, err = prog.Run(map[string]any{"usage": map[string]any{"output": []any{}}})
	require.NoError(t, err)
	assert.False(t, passed)

	passed, err = prog.Run(map[string]any{"usage": map[string]any{"output": map[string]any{}}})
	require.NoError(t, err)
	assert.False(t, passed)

	listProg, err := Compile(`any(items ?? [], { notEmpty(.name) })`)
	require.NoError(t, err)
	passed, err = listProg.Run(map[string]any{
		"items": []any{map[string]any{"name": "claude"}},
	})
	require.NoError(t, err)
	assert.True(t, passed)
}

func TestCompileRejectsEmptyAndNonBoolean(t *testing.T) {
	_, err := Compile("   ")
	require.Error(t, err)

	_, err = Compile("1 + 2")
	require.Error(t, err)

	_, err = Compile("now()")
	require.Error(t, err)
}
