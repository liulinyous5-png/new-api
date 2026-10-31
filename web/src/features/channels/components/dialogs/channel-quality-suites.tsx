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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  createChannelQualitySuite,
  deleteChannelQualitySuite,
  listChannelQualityProbes,
  listChannelQualitySuites,
  updateChannelQualitySuite,
  type ChannelQualitySuite,
} from '../../api'
import { channelsQueryKeys } from '../../lib'
import { ChannelQualitySuiteList } from './channel-quality-suite-list'

type SuiteItemDraft = {
  key: string
  probeId: string
  model: string
}

type SuiteDraft = {
  id: number | null
  name: string
  items: SuiteItemDraft[]
}

export function ChannelQualitySuites(props: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<SuiteDraft | null>(null)
  const [deleting, setDeleting] = useState<ChannelQualitySuite | null>(null)

  const suitesQuery = useQuery({
    queryKey: channelsQueryKeys.qualitySuites(),
    queryFn: async () => {
      const response = await listChannelQualitySuites()
      requireServerSuccess(response)
      return response.data ?? []
    },
  })
  const probesQuery = useQuery({
    queryKey: channelsQueryKeys.qualityProbes(),
    queryFn: async () => {
      const response = await listChannelQualityProbes()
      requireServerSuccess(response)
      return response.data ?? []
    },
  })

  const refresh = async () => {
    await queryClient.invalidateQueries({
      queryKey: channelsQueryKeys.qualitySuites(),
    })
  }

  const save = useMutation({
    mutationFn: async (next: SuiteDraft) => {
      const payload = {
        name: next.name.trim(),
        items: next.items.map((item) => ({
          probe_id: Number(item.probeId),
          model: item.model.trim(),
        })),
      }
      const response =
        next.id == null
          ? await createChannelQualitySuite(payload)
          : await updateChannelQualitySuite(next.id, payload)
      requireServerSuccess(response)
      return response.data
    },
    onSuccess: async () => {
      setDraft(null)
      await refresh()
    },
  })

  const remove = useMutation({
    mutationFn: async (id: number) => {
      const response = await deleteChannelQualitySuite(id)
      requireServerSuccess(response)
    },
    onSuccess: async () => {
      setDeleting(null)
      await refresh()
    },
  })

  const suites = suitesQuery.data ?? []
  const probes = probesQuery.data ?? []

  let listContent: ReactNode
  if (suitesQuery.isPending || probesQuery.isPending) {
    listContent = <LoadingState />
  } else if (suitesQuery.isError || probesQuery.isError) {
    listContent = (
      <ErrorState
        onRetry={() => {
          void Promise.all([suitesQuery.refetch(), probesQuery.refetch()])
        }}
      />
    )
  } else if (suites.length === 0) {
    listContent = <EmptyState title={t('No quality suites yet')} />
  } else {
    listContent = (
      <ChannelQualitySuiteList
        suites={suites}
        probes={probes}
        canWrite={props.canWrite}
        onEdit={(suite) => setDraft(draftFromSuite(suite))}
      />
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      {props.canWrite ? (
        <Button
          type='button'
          className='self-start'
          disabled={
            suitesQuery.isPending ||
            suitesQuery.isError ||
            probesQuery.isPending ||
            probesQuery.isError
          }
          onClick={() => setDraft(emptySuite())}
        >
          {t('New suite')}
        </Button>
      ) : null}
      {listContent}
      <Dialog
        open={draft != null}
        onOpenChange={(open) => {
          if (!open) setDraft(null)
        }}
        title={draft?.id == null ? t('New suite') : t('Quality suite')}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 640px)'
        footer={
          draft ? (
            <div className='flex w-full items-center justify-between gap-2'>
              {props.canWrite && draft.id != null ? (
                <Button
                  type='button'
                  variant='destructive'
                  onClick={() => {
                    const suite = suites.find((item) => item.id === draft.id)
                    if (suite) setDeleting(suite)
                  }}
                >
                  {t('Delete')}
                </Button>
              ) : (
                <span />
              )}
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => setDraft(null)}
                >
                  {t('Cancel')}
                </Button>
                {props.canWrite ? (
                  <Button
                    type='button'
                    disabled={
                      save.isPending ||
                      probesQuery.isPending ||
                      probesQuery.isError
                    }
                    onClick={() => {
                      const message = suiteDraftError(draft)
                      if (message) {
                        toast.error(t(message))
                        return
                      }
                      save.mutate(draft)
                    }}
                  >
                    {t('Save suite')}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null
        }
      >
        {draft ? (
          <SuiteFields
            draft={draft}
            probes={probes.map((probe) => ({ id: probe.id, name: probe.name }))}
            disabled={!props.canWrite || save.isPending}
            onChange={setDraft}
          />
        ) : null}
      </Dialog>
      <ConfirmDialog
        open={deleting != null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t('Delete suite')}
        desc={t('Delete this suite?')}
        destructive
        handleConfirm={() => {
          if (deleting) remove.mutate(deleting.id)
        }}
        isLoading={remove.isPending}
      />
    </div>
  )
}

function SuiteFields(props: {
  draft: SuiteDraft
  probes: { id: number; name: string }[]
  disabled: boolean
  onChange: (draft: SuiteDraft) => void
}) {
  const { t } = useTranslation()
  const draft = props.draft
  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='quality-suite-name'>{t('Suite name')}</Label>
        <Input
          id='quality-suite-name'
          value={draft.name}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange({ ...draft, name: event.target.value })
          }
        />
      </div>
      <div className='flex flex-col gap-2'>
        {draft.items.map((item) => (
          <div
            key={item.key}
            className='grid gap-2 sm:grid-cols-[1fr_1fr_auto]'
          >
            <Select
              items={props.probes.map((probe) => ({
                value: String(probe.id),
                label: probe.name,
              }))}
              value={item.probeId || undefined}
              disabled={props.disabled}
              onValueChange={(value) =>
                props.onChange({
                  ...draft,
                  items: draft.items.map((entry) =>
                    entry.key === item.key
                      ? { ...entry, probeId: value ?? '' }
                      : entry
                  ),
                })
              }
            >
              <SelectTrigger aria-label={t('Probe')} className='w-full min-w-0'>
                <SelectValue placeholder={t('Select a probe')} />
              </SelectTrigger>
              <SelectContent>
                {props.probes.map((probe) => (
                  <SelectItem key={probe.id} value={String(probe.id)}>
                    {probe.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              aria-label={t('Model')}
              value={item.model}
              disabled={props.disabled}
              placeholder={t('Enter a model')}
              onChange={(event) =>
                props.onChange({
                  ...draft,
                  items: draft.items.map((entry) =>
                    entry.key === item.key
                      ? { ...entry, model: event.target.value }
                      : entry
                  ),
                })
              }
            />
            {props.disabled ? null : (
              <Button
                type='button'
                variant='outline'
                onClick={() =>
                  props.onChange({
                    ...draft,
                    items: draft.items.filter(
                      (entry) => entry.key !== item.key
                    ),
                  })
                }
              >
                {t('Remove')}
              </Button>
            )}
          </div>
        ))}
        {props.disabled ? null : (
          <Button
            type='button'
            variant='outline'
            className='self-start'
            onClick={() =>
              props.onChange({
                ...draft,
                items: [...draft.items, emptySuiteItem()],
              })
            }
          >
            {t('Add probe')}
          </Button>
        )}
      </div>
    </div>
  )
}

function emptySuite(): SuiteDraft {
  return { id: null, name: '', items: [emptySuiteItem()] }
}

function emptySuiteItem(): SuiteItemDraft {
  return { key: crypto.randomUUID(), probeId: '', model: '' }
}

function draftFromSuite(suite: ChannelQualitySuite): SuiteDraft {
  return {
    id: suite.id,
    name: suite.name,
    items: suite.items.map((item) => ({
      key: crypto.randomUUID(),
      probeId: String(item.probe_id),
      model: item.model,
    })),
  }
}

function suiteDraftError(draft: SuiteDraft): string | null {
  if (!draft.name.trim()) return 'quality suite name is required'
  if (draft.items.length < 1 || draft.items.length > 50) {
    return 'a quality suite needs 1 to 50 probes'
  }
  const seen = new Set<string>()
  for (const item of draft.items) {
    if (!item.probeId) return 'Select a probe'
    const model = item.model.trim()
    if (!model) return 'quality suite model is required'
    const key = `${item.probeId}\u0000${model}`
    if (seen.has(key)) {
      return 'a quality suite lists this probe and model more than once'
    }
    seen.add(key)
  }
  return null
}
