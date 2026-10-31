/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { expect, it } from 'vitest'

import {
  compileQualityAssertion,
  parseQualityAssertion,
} from '../quality-assertion'

const clearToolUses = `any(context_management?.applied_edits ?? [], {
  .type == "clear_tool_uses_20250919" &&
  int(.cleared_tool_uses) == 3 &&
  float(.cleared_input_tokens) >= 1
})`

it('parses the builtin clear_tool_uses condition into fields', () => {
  expect(parseQualityAssertion(clearToolUses)).toEqual({
    mode: 'any',
    listPath: 'context_management.applied_edits',
    conditions: [
      {
        field: 'type',
        op: 'eq',
        kind: 'text',
        value: 'clear_tool_uses_20250919',
      },
      { field: 'cleared_tool_uses', op: 'eq', kind: 'number', value: '3' },
      {
        field: 'cleared_input_tokens',
        op: 'gte',
        kind: 'number',
        value: '1',
      },
    ],
  })
})

it('compiles field comparisons and list matches', () => {
  const parsed = parseQualityAssertion(clearToolUses)
  if (!parsed) throw new Error('expected fields')
  expect(compileQualityAssertion(parsed)).toBe(
    'any(context_management?.applied_edits ?? [], { .type == "clear_tool_uses_20250919" && float(.cleared_tool_uses) == 3 && float(.cleared_input_tokens) >= 1 })'
  )
  expect(
    compileQualityAssertion({
      mode: 'field',
      listPath: '',
      conditions: [
        {
          field: 'usage.input_tokens',
          op: 'gt',
          kind: 'number',
          value: '0',
        },
      ],
    })
  ).toBe('float(usage?.input_tokens) > 0')
})

it('compiles a field that only needs to be present', () => {
  expect(
    compileQualityAssertion({
      mode: 'any',
      listPath: 'context_management.applied_edits',
      conditions: [{ field: 'type', op: 'notEmpty', kind: 'text', value: '' }],
    })
  ).toBe('any(context_management?.applied_edits ?? [], { notEmpty(.type) })')
  expect(
    parseQualityAssertion(
      'any(context_management?.applied_edits ?? [], { notEmpty(.type) })'
    )
  ).toEqual({
    mode: 'any',
    listPath: 'context_management.applied_edits',
    conditions: [{ field: 'type', op: 'notEmpty', kind: 'text', value: '' }],
  })
})

it('compiles and reads a text contains condition', () => {
  expect(
    compileQualityAssertion({
      mode: 'field',
      listPath: '',
      conditions: [
        { field: 'content.text', op: 'contains', kind: 'text', value: 'opus' },
      ],
    })
  ).toBe('content?.text contains "opus"')
  expect(parseQualityAssertion('content?.text contains "opus"')).toEqual({
    mode: 'field',
    listPath: '',
    conditions: [
      { field: 'content.text', op: 'contains', kind: 'text', value: 'opus' },
    ],
  })
})

it('rejects an incomplete field path', () => {
  expect(() =>
    compileQualityAssertion({
      mode: 'field',
      listPath: '',
      conditions: [{ field: 'a b', op: 'eq', kind: 'text', value: 'x' }],
    })
  ).toThrow('condition is incomplete')
  expect(parseQualityAssertion('now()')).toBeNull()
})
