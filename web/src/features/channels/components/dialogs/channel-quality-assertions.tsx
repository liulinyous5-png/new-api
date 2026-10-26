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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { compileQualityAssertion } from '../../lib/quality-assertion'
import {
  ChannelQualityAssertionFields,
  type QualityAssertionDraftModel,
} from './channel-quality-assertion-fields'

export type QualityDraftAssertion = {
  key: string
  name: string
  model: QualityAssertionDraftModel | null
  customExpr: string | null
}

const maxAssertions = 20

const modeLabel = {
  field: 'A response field',
  any: 'Any item in a list',
  all: 'Every item in a list',
} as const

const operatorLabel = {
  eq: 'Equals',
  neq: 'Does not equal',
  notEmpty: 'Is not empty',
  gt: 'Greater than',
  gte: 'Greater than or equal',
  lt: 'Less than',
  lte: 'Less than or equal',
} as const

type ChannelQualityAssertionsProps = {
  assertions: QualityDraftAssertion[]
  disabled?: boolean
  onChange: (assertions: QualityDraftAssertion[]) => void
}

type AssertionEditor = {
  index: number | null
  draft: QualityDraftAssertion
}

export function ChannelQualityAssertions(props: ChannelQualityAssertionsProps) {
  const { t } = useTranslation()
  const [editor, setEditor] = useState<AssertionEditor | null>(null)

  const closeEditor = () => setEditor(null)

  const commitEditor = () => {
    if (!editor) return
    const name = editor.draft.name.trim()
    if (!name) {
      toast.error(t('Assertion name is required'))
      return
    }
    if (editor.draft.model) {
      try {
        compileQualityAssertion({
          mode: editor.draft.model.mode,
          listPath: editor.draft.model.listPath,
          conditions: editor.draft.model.conditions,
        })
      } catch {
        toast.error(t('Each condition needs a field and a value'))
        return
      }
    } else if (!editor.draft.customExpr?.trim()) {
      toast.error(t('Each condition needs a field and a value'))
      return
    }
    const next = { ...editor.draft, name }
    if (editor.index === null) {
      props.onChange([...props.assertions, next])
    } else {
      props.onChange(
        props.assertions.map((assertion, index) =>
          index === editor.index ? next : assertion
        )
      )
    }
    closeEditor()
  }

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex items-center justify-between gap-2'>
        <Label>{t('Assertions')}</Label>
        {props.disabled ? null : (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={props.assertions.length >= maxAssertions}
            onClick={() =>
              setEditor({
                index: null,
                draft: emptyAssertion(),
              })
            }
          >
            <Plus className='size-4' aria-hidden='true' />
            {t('Add assertion')}
          </Button>
        )}
      </div>
      {props.assertions.length === 0 ? (
        <p className='text-muted-foreground rounded-lg border border-dashed px-3 py-6 text-center text-sm'>
          {t('No assertions yet. A matching response status is enough.')}
        </p>
      ) : (
        <ul className='flex flex-col gap-2'>
          {props.assertions.map((assertion, index) => (
            <li
              key={assertion.key}
              className='flex items-start justify-between gap-3 rounded-lg border px-3 py-2.5'
            >
              <div className='min-w-0'>
                <p className='truncate text-sm font-medium'>
                  {assertion.name || t('Assertion name')}
                </p>
                <ul className='text-muted-foreground mt-1 space-y-0.5 text-xs'>
                  {summaryLines(assertion, t).map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </div>
              {props.disabled ? null : (
                <div className='flex shrink-0 gap-1'>
                  <Button
                    type='button'
                    size='icon'
                    variant='ghost'
                    aria-label={t('Edit assertion')}
                    onClick={() =>
                      setEditor({
                        index,
                        draft: copyAssertion(assertion),
                      })
                    }
                  >
                    <Pencil className='size-4' aria-hidden='true' />
                  </Button>
                  <Button
                    type='button'
                    size='icon'
                    variant='ghost'
                    aria-label={t('Remove assertion')}
                    onClick={() =>
                      props.onChange(
                        props.assertions.filter(
                          (_, itemIndex) => itemIndex !== index
                        )
                      )
                    }
                  >
                    <Trash2 className='size-4' aria-hidden='true' />
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      <Dialog
        open={editor !== null}
        onOpenChange={(open) => {
          if (!open) closeEditor()
        }}
        title={
          editor?.index === null ? t('Add assertion') : t('Edit assertion')
        }
        description={t(
          'Choose where to look in the response, then add the comparisons.'
        )}
        contentClassName='sm:max-w-3xl'
        footer={
          <>
            <Button type='button' variant='outline' onClick={closeEditor}>
              {t('Cancel')}
            </Button>
            <Button type='button' onClick={commitEditor}>
              {t('Save assertion')}
            </Button>
          </>
        }
      >
        {editor ? (
          <div className='flex flex-col gap-3'>
            <div className='flex flex-col gap-1.5'>
              <Label htmlFor='quality-assertion-name'>
                {t('Assertion name')}
              </Label>
              <Input
                id='quality-assertion-name'
                value={editor.draft.name}
                aria-label={t('Assertion name')}
                onChange={(event) =>
                  setEditor({
                    ...editor,
                    draft: { ...editor.draft, name: event.target.value },
                  })
                }
              />
            </div>
            {editor.draft.model ? (
              <ChannelQualityAssertionFields
                model={editor.draft.model}
                onChange={(model) =>
                  setEditor({
                    ...editor,
                    draft: { ...editor.draft, model, customExpr: null },
                  })
                }
              />
            ) : (
              <div className='flex flex-col items-start gap-2'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'This saved condition cannot be edited as fields. Replace it to use the form.'
                  )}
                </p>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    setEditor({
                      ...editor,
                      draft: {
                        ...editor.draft,
                        model: emptyAssertion().model,
                        customExpr: null,
                      },
                    })
                  }
                >
                  {t('Replace condition')}
                </Button>
              </div>
            )}
          </div>
        ) : null}
      </Dialog>
    </div>
  )
}

function summaryLines(
  assertion: QualityDraftAssertion,
  t: (key: string) => string
): string[] {
  if (!assertion.model) return [assertion.customExpr ?? '']
  const lines = assertion.model.conditions.map((condition) => {
    if (condition.op === 'notEmpty') {
      return `${condition.field} ${t(operatorLabel[condition.op])}`
    }
    return `${condition.field} ${t(operatorLabel[condition.op])} ${condition.value}`
  })
  if (assertion.model.mode === 'field') return lines
  return [
    `${t(modeLabel[assertion.model.mode])} · ${assertion.model.listPath}`,
    ...lines,
  ]
}

function emptyAssertion(): QualityDraftAssertion {
  return {
    key: newAssertionKey(),
    name: '',
    model: {
      mode: 'any',
      listPath: '',
      conditions: [
        {
          key: newAssertionKey(),
          field: '',
          op: 'eq',
          kind: 'text',
          value: '',
        },
      ],
    },
    customExpr: null,
  }
}

function copyAssertion(
  assertion: QualityDraftAssertion
): QualityDraftAssertion {
  return {
    ...assertion,
    model: assertion.model
      ? {
          ...assertion.model,
          conditions: assertion.model.conditions.map((condition) => ({
            ...condition,
          })),
        }
      : null,
  }
}

function newAssertionKey(): string {
  return globalThis.crypto.randomUUID()
}
