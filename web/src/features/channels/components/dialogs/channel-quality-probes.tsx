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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  copyChannelQualityProbe,
  createChannelQualityProbe,
  deleteChannelQualityProbe,
  listChannelQualityProbes,
  updateChannelQualityProbe,
  type ChannelQualityProbe,
} from '../../api'
import { channelsQueryKeys } from '../../lib'
import {
  compileQualityAssertion,
  parseQualityAssertion,
  type QualityAssertionModel,
} from '../../lib/quality-assertion'
import type { QualityAssertionDraftModel } from './channel-quality-assertion-fields'
import {
  ChannelQualityAssertions,
  type QualityDraftAssertion,
} from './channel-quality-assertions'
import { ChannelQualityHeaders } from './channel-quality-headers'

type ChannelQualityProbesProps = {
  canWrite: boolean
}

type DraftAssertion = QualityDraftAssertion

type ProbeDraft = {
  id: number | null
  name: string
  headers: string
  body: string
  statuses: number[]
  assertions: DraftAssertion[]
  builtin: boolean
}

export function ChannelQualityProbes(props: ChannelQualityProbesProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ProbeDraft | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [statusInput, setStatusInput] = useState('')
  const creatingRef = useRef(false)
  const probesQuery = useQuery({
    queryKey: channelsQueryKeys.qualityProbes(),
    queryFn: async () => {
      const response = await listChannelQualityProbes()
      requireServerSuccess(response)
      return response.data ?? []
    },
  })

  const refreshProbes = async () => {
    await queryClient.invalidateQueries({
      queryKey: channelsQueryKeys.qualityProbes(),
    })
  }

  const saveProbe = useMutation({
    mutationFn: async (next: ProbeDraft) => {
      const headers = parseHeaderObject(next.headers)
      if (!headers) {
        throw new Error(t('Header JSON must be an object'))
      }
      if (next.statuses.length === 0) {
        throw new Error(t('Add at least one response status'))
      }
      const payload = {
        name: next.name.trim(),
        headers,
        body: next.body,
        statuses: next.statuses,
        assertions: next.assertions
          .filter((assertion) => !blankAssertion(assertion))
          .map((assertion) => {
            let expr = ''
            try {
              expr = assertionExpr(assertion)
            } catch {
              expr = ''
            }
            if (!assertion.name.trim() || !expr.trim()) {
              throw new Error(t('Each condition needs a field and a value'))
            }
            return { name: assertion.name.trim(), expr }
          }),
      }
      const response = next.id
        ? await updateChannelQualityProbe(next.id, payload)
        : await createChannelQualityProbe(payload)
      requireServerSuccess(response)
      return response.data
    },
    onSuccess: async (probe) => {
      toast.success(t('Probe saved'))
      creatingRef.current = false
      await refreshProbes()
      if (probe) setDraft(draftFromProbe(probe))
    },
  })

  const copyProbe = useMutation({
    mutationFn: async (id: number) => {
      const response = await copyChannelQualityProbe(id)
      requireServerSuccess(response)
      return response.data
    },
    onSuccess: async (probe) => {
      toast.success(t('Probe duplicated'))
      await refreshProbes()
      if (probe) setDraft(draftFromProbe(probe))
    },
  })

  const removeProbe = useMutation({
    mutationFn: async (id: number) => {
      const response = await deleteChannelQualityProbe(id)
      requireServerSuccess(response)
    },
    onSuccess: async () => {
      toast.success(t('Probe deleted'))
      setDraft(null)
      setDeleteOpen(false)
      await refreshProbes()
    },
  })

  const probes = probesQuery.data ?? []
  const creating = draft?.id === null
  const locked = draft?.builtin === true
  const fieldsDisabled = locked || !props.canWrite

  const closeCreate = () => {
    if (!creatingRef.current) return
    creatingRef.current = false
    setStatusInput('')
    setDraft(null)
  }

  const openCreate = () => {
    creatingRef.current = true
    setStatusInput('')
    setDraft(emptyDraft())
  }

  const addStatus = () => {
    if (!draft) return
    const raw = statusInput.trim()
    if (!raw) {
      toast.error(t('Enter a status code'))
      return
    }
    const status = Number(raw)
    if (!Number.isInteger(status) || status < 100 || status > 599) {
      toast.error(t('Response status must be between 100 and 599'))
      return
    }
    if (draft.statuses.includes(status)) {
      toast.error(t('This status is already added'))
      setStatusInput('')
      return
    }
    if (draft.statuses.length >= 20) {
      toast.error(t('a probe can have at most 20 response statuses'))
      return
    }
    setDraft({
      ...draft,
      statuses: [...draft.statuses, status].sort((left, right) => left - right),
    })
    setStatusInput('')
  }

  return (
    <div className='flex flex-col gap-3 border-t pt-4'>
      <div className='flex flex-wrap gap-2'>
        {probes.map((probe) => (
          <Button
            key={probe.id}
            type='button'
            size='sm'
            variant={draft?.id === probe.id ? 'default' : 'outline'}
            onClick={() => {
              creatingRef.current = false
              setStatusInput('')
              setDraft(draftFromProbe(probe))
            }}
          >
            {probe.name}
          </Button>
        ))}
        {props.canWrite ? (
          <Button
            type='button'
            size='sm'
            variant={creating ? 'default' : 'outline'}
            onClick={openCreate}
          >
            {t('New probe')}
          </Button>
        ) : null}
      </div>

      {draft && !creating ? (
        <div className='flex flex-col gap-3'>
          {locked ? (
            <p className='text-muted-foreground text-sm'>
              {t('Builtin probes cannot be edited')}
            </p>
          ) : null}
          <ProbeFields
            draft={draft}
            disabled={fieldsDisabled}
            statusInput={statusInput}
            onDraft={setDraft}
            onStatusInput={setStatusInput}
            onAddStatus={addStatus}
          />
          {props.canWrite ? (
            <div className='flex flex-wrap gap-2'>
              {locked ? null : (
                <Button
                  type='button'
                  disabled={saveProbe.isPending}
                  onClick={() => saveProbe.mutate(draft)}
                >
                  {t('Save probe')}
                </Button>
              )}
              {draft.id ? (
                <Button
                  type='button'
                  variant='outline'
                  disabled={copyProbe.isPending}
                  onClick={() => {
                    if (draft.id) copyProbe.mutate(draft.id)
                  }}
                >
                  {t('Duplicate')}
                </Button>
              ) : null}
              {!locked && draft.id ? (
                <Button
                  type='button'
                  variant='destructive'
                  onClick={() => setDeleteOpen(true)}
                >
                  {t('Delete probe')}
                </Button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}

      <Dialog
        open={creating}
        onOpenChange={(open) => {
          if (!open) closeCreate()
        }}
        title={t('New probe')}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 640px)'
        footer={
          <>
            <Button type='button' variant='outline' onClick={closeCreate}>
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={!draft || saveProbe.isPending}
              onClick={() => {
                if (draft) saveProbe.mutate(draft)
              }}
            >
              {t('Save probe')}
            </Button>
          </>
        }
      >
        {creating && draft ? (
          <ProbeFields
            draft={draft}
            disabled={fieldsDisabled}
            statusInput={statusInput}
            onDraft={setDraft}
            onStatusInput={setStatusInput}
            onAddStatus={addStatus}
          />
        ) : null}
      </Dialog>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete probe')}
        desc={t('Delete this probe?')}
        destructive
        isLoading={removeProbe.isPending}
        handleConfirm={() => {
          if (draft?.id) removeProbe.mutate(draft.id)
        }}
      />
    </div>
  )
}

function ProbeFields(props: {
  draft: ProbeDraft
  disabled: boolean
  statusInput: string
  onDraft: (draft: ProbeDraft) => void
  onStatusInput: (value: string) => void
  onAddStatus: () => void
}) {
  const { t } = useTranslation()
  const { draft } = props
  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='quality-probe-name'>{t('Probe name')}</Label>
        <Input
          id='quality-probe-name'
          value={draft.name}
          disabled={props.disabled}
          onChange={(event) =>
            props.onDraft({ ...draft, name: event.target.value })
          }
        />
      </div>
      <div className='flex flex-col gap-1.5'>
        <Label>{t('Headers')}</Label>
        <ChannelQualityHeaders
          value={draft.headers}
          disabled={props.disabled}
          onChange={(headers) => props.onDraft({ ...draft, headers })}
        />
      </div>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='quality-probe-body'>{t('Request body')}</Label>
        <Textarea
          id='quality-probe-body'
          value={draft.body}
          disabled={props.disabled}
          className='max-h-64 font-mono text-xs'
          onChange={(event) =>
            props.onDraft({ ...draft, body: event.target.value })
          }
        />
      </div>
      <div className='flex flex-col gap-2'>
        <Label htmlFor='quality-status-code'>{t('Response status')}</Label>
        <div className='flex flex-wrap gap-2'>
          {draft.statuses.map((status) => (
            <span
              key={status}
              className='bg-muted inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs'
            >
              {status}
              {props.disabled ? null : (
                <button
                  type='button'
                  className='text-muted-foreground hover:text-foreground'
                  aria-label={`${t('Remove status')} ${status}`}
                  onClick={() =>
                    props.onDraft({
                      ...draft,
                      statuses: draft.statuses.filter(
                        (item) => item !== status
                      ),
                    })
                  }
                >
                  ×
                </button>
              )}
            </span>
          ))}
        </div>
        {props.disabled ? null : (
          <div className='flex gap-2'>
            <Input
              id='quality-status-code'
              inputMode='numeric'
              value={props.statusInput}
              placeholder={t('Enter a status code')}
              aria-label={t('Status code')}
              onChange={(event) => props.onStatusInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== 'Enter') return
                event.preventDefault()
                props.onAddStatus()
              }}
            />
            <Button type='button' variant='outline' onClick={props.onAddStatus}>
              {t('Add status')}
            </Button>
          </div>
        )}
      </div>
      <ChannelQualityAssertions
        assertions={draft.assertions}
        disabled={props.disabled}
        onChange={(assertions) => props.onDraft({ ...draft, assertions })}
      />
    </div>
  )
}

function emptyDraft(): ProbeDraft {
  return {
    id: null,
    name: '',
    headers: '{}',
    body: '{\n  "model": "{{model}}"\n}',
    statuses: [200],
    assertions: [],
    builtin: false,
  }
}

function draftFromProbe(probe: ChannelQualityProbe): ProbeDraft {
  return {
    id: probe.id,
    name: probe.name,
    headers: JSON.stringify(probe.headers ?? {}, null, 2),
    body: probe.body,
    statuses: probe.statuses?.length ? probe.statuses : [200],
    assertions: probe.assertions.map((assertion) => draftAssertion(assertion)),
    builtin: probe.builtin,
  }
}

function draftAssertion(assertion: {
  name: string
  expr: string
}): DraftAssertion {
  const parsed = parseQualityAssertion(assertion.expr)
  return {
    key: newAssertionKey(),
    name: assertion.name,
    model: parsed ? draftModel(parsed) : null,
    customExpr: parsed ? null : assertion.expr,
  }
}

function draftModel(model: QualityAssertionModel): QualityAssertionDraftModel {
  return {
    mode: model.mode,
    listPath: model.listPath,
    conditions: model.conditions.map((condition) => ({
      ...condition,
      key: newAssertionKey(),
    })),
  }
}

function assertionExpr(assertion: DraftAssertion): string {
  if (!assertion.model) return assertion.customExpr?.trim() ?? ''
  return compileQualityAssertion({
    mode: assertion.model.mode,
    listPath: assertion.model.listPath,
    conditions: assertion.model.conditions.map((condition) => ({
      field: condition.field,
      op: condition.op,
      kind: condition.kind,
      value: condition.value,
    })),
  })
}

function blankAssertion(assertion: DraftAssertion): boolean {
  if (assertion.name.trim() || assertion.customExpr || !assertion.model) {
    return false
  }
  return assertion.model.conditions.every(
    (condition) => !condition.field.trim() && !condition.value.trim()
  )
}

function newAssertionKey(): string {
  return globalThis.crypto.randomUUID()
}

function parseHeaderObject(raw: string): Record<string, string> | null {
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    return null
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const headers: Record<string, string> = {}
  for (const [key, item] of Object.entries(value)) {
    if (typeof item !== 'string') return null
    headers[key] = item
  }
  return headers
}
