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

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  emptyQualityCondition,
  type QualityAssertionModel,
  type QualityCompareOp,
  type QualityCondition,
  type QualityMatchMode,
  type QualityValueKind,
} from '../../lib/quality-assertion'

const maxConditions = 20

const modes: QualityMatchMode[] = ['field', 'any', 'all']
const operators: QualityCompareOp[] = [
  'eq',
  'neq',
  'notEmpty',
  'gt',
  'gte',
  'lt',
  'lte',
]
const kinds: QualityValueKind[] = ['text', 'number']

const modeLabel: Record<QualityMatchMode, string> = {
  field: 'A response field',
  any: 'Any item in a list',
  all: 'Every item in a list',
}

const operatorLabel: Record<QualityCompareOp, string> = {
  eq: 'Equals',
  neq: 'Does not equal',
  notEmpty: 'Is not empty',
  gt: 'Greater than',
  gte: 'Greater than or equal',
  lt: 'Less than',
  lte: 'Less than or equal',
}

const kindLabel: Record<QualityValueKind, string> = {
  text: 'Text',
  number: 'Number',
}

export type QualityConditionDraft = QualityCondition & {
  key: string
}

export type QualityAssertionDraftModel = {
  mode: QualityAssertionModel['mode']
  listPath: string
  conditions: QualityConditionDraft[]
}

type ChannelQualityAssertionFieldsProps = {
  model: QualityAssertionDraftModel
  disabled?: boolean
  onChange: (model: QualityAssertionDraftModel) => void
}

export function ChannelQualityAssertionFields(
  props: ChannelQualityAssertionFieldsProps
) {
  const { t } = useTranslation()
  const model = props.model
  const textOperators = operators.filter(
    (op) => op === 'eq' || op === 'neq' || op === 'notEmpty'
  )

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex flex-col gap-2 sm:flex-row'>
        <Select
          value={model.mode}
          disabled={props.disabled}
          onValueChange={(value) => {
            if (value !== 'field' && value !== 'any' && value !== 'all') return
            props.onChange({ ...model, mode: value })
          }}
        >
          <SelectTrigger
            className='w-full sm:w-56'
            aria-label={t('Where to match')}
          >
            <SelectValue>{t(modeLabel[model.mode])}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {modes.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {t(modeLabel[mode])}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {model.mode === 'field' ? null : (
          <Input
            value={model.listPath}
            disabled={props.disabled}
            aria-label={t('List path')}
            placeholder={t('List path')}
            onChange={(event) =>
              props.onChange({ ...model, listPath: event.target.value })
            }
          />
        )}
      </div>
      {model.conditions.map((condition, index) => (
        <div
          key={condition.key}
          className='flex flex-col gap-2 sm:flex-row sm:items-center'
        >
          <Input
            value={condition.field}
            disabled={props.disabled}
            aria-label={t('Field path')}
            placeholder={t('Field path')}
            onChange={(event) =>
              props.onChange({
                ...model,
                conditions: replaceCondition(model.conditions, index, {
                  ...condition,
                  field: event.target.value,
                }),
              })
            }
          />
          <Select
            value={condition.op}
            disabled={props.disabled}
            onValueChange={(value) => {
              if (!isOperator(value)) return
              props.onChange({
                ...model,
                conditions: replaceCondition(model.conditions, index, {
                  ...condition,
                  op: value,
                }),
              })
            }}
          >
            <SelectTrigger
              className='w-full sm:w-44'
              aria-label={t('Comparison')}
            >
              <SelectValue>{t(operatorLabel[condition.op])}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {(condition.kind === 'text' ? textOperators : operators).map(
                (op) => (
                  <SelectItem key={op} value={op}>
                    {t(operatorLabel[op])}
                  </SelectItem>
                )
              )}
            </SelectContent>
          </Select>
          {condition.op === 'notEmpty' ? null : (
            <>
              <Select
                value={condition.kind}
                disabled={props.disabled}
                onValueChange={(value) => {
                  if (value !== 'text' && value !== 'number') return
                  const op =
                    value === 'text' &&
                    condition.op !== 'eq' &&
                    condition.op !== 'neq' &&
                    condition.op !== 'notEmpty'
                      ? 'eq'
                      : condition.op
                  props.onChange({
                    ...model,
                    conditions: replaceCondition(model.conditions, index, {
                      ...condition,
                      kind: value,
                      op,
                    }),
                  })
                }}
              >
                <SelectTrigger
                  className='w-full sm:w-28'
                  aria-label={t('Value type')}
                >
                  <SelectValue>{t(kindLabel[condition.kind])}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {kinds.map((kind) => (
                    <SelectItem key={kind} value={kind}>
                      {t(kindLabel[kind])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                value={condition.value}
                disabled={props.disabled}
                aria-label={t('Expected value')}
                placeholder={t('Expected value')}
                onChange={(event) =>
                  props.onChange({
                    ...model,
                    conditions: replaceCondition(model.conditions, index, {
                      ...condition,
                      value: event.target.value,
                    }),
                  })
                }
              />
            </>
          )}
          {props.disabled || model.conditions.length <= 1 ? null : (
            <Button
              type='button'
              size='sm'
              variant='ghost'
              onClick={() =>
                props.onChange({
                  ...model,
                  conditions: model.conditions.filter(
                    (_, itemIndex) => itemIndex !== index
                  ),
                })
              }
            >
              {t('Remove condition')}
            </Button>
          )}
        </div>
      ))}
      {props.disabled ? null : (
        <Button
          type='button'
          size='sm'
          variant='outline'
          className='self-start'
          disabled={model.conditions.length >= maxConditions}
          onClick={() =>
            props.onChange({
              ...model,
              conditions: [
                ...model.conditions,
                { ...emptyQualityCondition(), key: crypto.randomUUID() },
              ],
            })
          }
        >
          {t('Add condition')}
        </Button>
      )}
    </div>
  )
}

function replaceCondition(
  conditions: QualityConditionDraft[],
  index: number,
  next: QualityConditionDraft
): QualityConditionDraft[] {
  return conditions.map((condition, itemIndex) =>
    itemIndex === index ? next : condition
  )
}

function isOperator(value: unknown): value is QualityCompareOp {
  return (
    value === 'eq' ||
    value === 'neq' ||
    value === 'notEmpty' ||
    value === 'gt' ||
    value === 'gte' ||
    value === 'lt' ||
    value === 'lte'
  )
}
