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

export type QualityMatchMode = 'field' | 'any' | 'all'

export type QualityCompareOp =
  | 'eq'
  | 'neq'
  | 'contains'
  | 'notEmpty'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'

export type QualityValueKind = 'text' | 'number'

export type QualityCondition = {
  field: string
  op: QualityCompareOp
  kind: QualityValueKind
  value: string
}

export type QualityAssertionModel = {
  mode: QualityMatchMode
  listPath: string
  conditions: QualityCondition[]
}

const fieldPattern = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/
const numberPattern = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/

const opSource: Record<
  Exclude<QualityCompareOp, 'notEmpty' | 'contains'>,
  string
> = {
  eq: '==',
  neq: '!=',
  gt: '>',
  gte: '>=',
  lt: '<',
  lte: '<=',
}

const sourceOp: Record<string, QualityCompareOp> = {
  '==': 'eq',
  '!=': 'neq',
  '>': 'gt',
  '>=': 'gte',
  '<': 'lt',
  '<=': 'lte',
}

export function emptyQualityAssertion(): QualityAssertionModel {
  return {
    mode: 'any',
    listPath: '',
    conditions: [emptyQualityCondition()],
  }
}

export function emptyQualityCondition(): QualityCondition {
  return { field: '', op: 'eq', kind: 'text', value: '' }
}

export function compileQualityAssertion(model: QualityAssertionModel): string {
  if (model.conditions.length === 0) {
    throw new Error('condition is incomplete')
  }
  const relative = model.mode !== 'field'
  const clauses = model.conditions.map((condition) =>
    compileCondition(condition, relative)
  )
  const body = clauses.join(' && ')
  if (!relative) return body
  if (!fieldPattern.test(model.listPath.trim())) {
    throw new Error('condition is incomplete')
  }
  const fn = model.mode === 'all' ? 'all' : 'any'
  return `${fn}(${optionalPath(model.listPath.trim())} ?? [], { ${body} })`
}

export function parseQualityAssertion(
  expr: string
): QualityAssertionModel | null {
  const source = expr.trim()
  if (!source) return null
  const wrapped =
    /^(any|all)\(\s*([A-Za-z_][A-Za-z0-9_?.]*)\s*\?\?\s*\[\],\s*\{([\s\S]*)\}\s*\)$/.exec(
      source
    )
  let mode: QualityMatchMode = 'field'
  let listPath = ''
  let body = source
  if (wrapped) {
    mode = wrapped[1] === 'all' ? 'all' : 'any'
    const parsedList = normalizeField(wrapped[2])
    if (!parsedList) return null
    listPath = parsedList
    body = wrapped[3].trim()
  }
  const parts = splitConditions(body)
  if (parts.length === 0) return null
  const conditions: QualityCondition[] = []
  for (const part of parts) {
    const condition = parseCondition(part)
    if (!condition) return null
    conditions.push(condition)
  }
  return { mode, listPath, conditions }
}

function compileCondition(
  condition: QualityCondition,
  relative: boolean
): string {
  const field = condition.field.trim()
  if (!fieldPattern.test(field)) {
    throw new Error('condition is incomplete')
  }
  const path = relative ? `.${optionalPath(field)}` : optionalPath(field)
  if (condition.op === 'notEmpty') {
    return `notEmpty(${path})`
  }
  if (condition.op === 'contains') {
    if (condition.value === '') {
      throw new Error('condition is incomplete')
    }
    return `${path} contains ${JSON.stringify(condition.value)}`
  }
  const op = opSource[condition.op]
  if (condition.kind === 'number') {
    const value = condition.value.trim()
    if (!numberPattern.test(value)) {
      throw new Error('condition is incomplete')
    }
    return `float(${path}) ${op} ${value}`
  }
  if (condition.op !== 'eq' && condition.op !== 'neq') {
    throw new Error('condition is incomplete')
  }
  return `${path} ${op} ${JSON.stringify(condition.value)}`
}

function parseCondition(source: string): QualityCondition | null {
  const present = /^notEmpty\(\s*(\.?[A-Za-z_][A-Za-z0-9_?.]*)\s*\)$/.exec(
    source.trim()
  )
  if (present) {
    const field = normalizeField(present[1])
    if (!field) return null
    return { field, op: 'notEmpty', kind: 'text', value: '' }
  }
  const contained =
    /^(\.?[A-Za-z_][A-Za-z0-9_?.]*)\s+contains\s+"((?:\\.|[^"\\])*)"$/.exec(
      source.trim()
    )
  if (contained) {
    const field = normalizeField(contained[1])
    if (!field) return null
    let value = ''
    try {
      value = JSON.parse(`"${contained[2]}"`) as string
    } catch {
      return null
    }
    if (value === '') return null
    return { field, op: 'contains', kind: 'text', value }
  }
  const numeric =
    /^(?:int|float)\(\s*([^)]+?)\s*\)\s*(==|!=|>=|<=|>|<)\s*(-?\d+(?:\.\d+)?)$/.exec(
      source.trim()
    )
  if (numeric) {
    return conditionFrom(numeric[1], numeric[2], 'number', numeric[3])
  }
  const text =
    /^(\.?[A-Za-z_][A-Za-z0-9_?.]*)\s*(==|!=)\s*"((?:\\.|[^"\\])*)"$/.exec(
      source.trim()
    )
  if (text) {
    let value = ''
    try {
      value = JSON.parse(`"${text[3]}"`) as string
    } catch {
      return null
    }
    return conditionFrom(text[1], text[2], 'text', value)
  }
  const bareNumber =
    /^(\.?[A-Za-z_][A-Za-z0-9_?.]*)\s*(==|!=|>=|<=|>|<)\s*(-?\d+(?:\.\d+)?)$/.exec(
      source.trim()
    )
  if (bareNumber) {
    return conditionFrom(bareNumber[1], bareNumber[2], 'number', bareNumber[3])
  }
  return null
}

function conditionFrom(
  path: string,
  op: string,
  kind: QualityValueKind,
  value: string
): QualityCondition | null {
  const field = normalizeField(path)
  const compare = sourceOp[op]
  if (!field || !compare) return null
  if (kind === 'text' && compare !== 'eq' && compare !== 'neq') return null
  return { field, op: compare, kind, value }
}

function normalizeField(path: string): string | null {
  const field = path.trim().replace(/^\./, '').replaceAll('?.', '.')
  if (!fieldPattern.test(field)) return null
  return field
}

function optionalPath(path: string): string {
  return path.split('.').join('?.')
}

function splitConditions(body: string): string[] {
  const parts: string[] = []
  let start = 0
  let quoted = false
  let escaped = false
  for (let index = 0; index < body.length; index += 1) {
    const char = body[index]
    if (quoted) {
      if (escaped) {
        escaped = false
        continue
      }
      if (char === '\\') {
        escaped = true
        continue
      }
      if (char === '"') quoted = false
      continue
    }
    if (char === '"') {
      quoted = true
      continue
    }
    if (char === '&' && body[index + 1] === '&') {
      parts.push(body.slice(start, index).trim())
      index += 1
      start = index + 1
    }
  }
  parts.push(body.slice(start).trim())
  return parts.filter((part) => part !== '')
}
