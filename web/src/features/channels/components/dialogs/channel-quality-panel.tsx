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
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import {
  requireServerSuccess,
  translateServerText,
} from '@/lib/server-error-message'
import { formatDateTimeObject } from '@/lib/time'
import { useAuthStore } from '@/stores/auth-store'

import {
  getChannelQualityRun,
  getChannelQualitySuiteRun,
  listChannelQualityProbes,
  listChannelQualityRuns,
  startChannelQuality,
  type ChannelQualityRun,
  type ChannelQualitySuiteRun,
} from '../../api'
import { QUALITY_AUDIT_CHANNEL_TYPES } from '../../constants'
import {
  channelsQueryKeys,
  isMultiKeyChannel,
  parseModelsList,
} from '../../lib'
import type { Channel } from '../../types'
import { ChannelQualityResponse } from './channel-quality-response'
import { ChannelQualityVerdictBadge } from './channel-quality-verdict'

type ChannelQualityPanelProps = {
  channel: Channel | null
}

export function ChannelQualityPanel(props: ChannelQualityPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canOperate = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.OPERATE
  )
  const channel = props.channel
  const channelId = channel?.id ?? 0
  const [probeId, setProbeId] = useState('')
  const [model, setModel] = useState('')
  const [keyIndex, setKeyIndex] = useState('')
  const [page, setPage] = useState(1)
  const [selectedRunSnapshot, setSelectedRun] =
    useState<ChannelQualityRun | null>(null)
  const [suiteOpen, setSuiteOpen] = useState(false)
  const pageSize = 10

  const probesQuery = useQuery({
    queryKey: channelsQueryKeys.qualityProbes(),
    queryFn: async () => {
      const response = await listChannelQualityProbes()
      requireServerSuccess(response)
      return response.data ?? []
    },
  })
  const runQuery = useQuery({
    queryKey: channelsQueryKeys.qualityRun(channelId),
    queryFn: async () => {
      const response = await getChannelQualityRun(channelId)
      requireServerSuccess(response)
      return response.data ?? null
    },
    enabled: channelId > 0,
    refetchInterval: (query) =>
      query.state.data?.verdict === 'running' ? 1500 : false,
  })
  const runsQuery = useQuery({
    queryKey: channelsQueryKeys.qualityRuns(channelId, page),
    queryFn: async () => {
      const response = await listChannelQualityRuns(channelId, page, pageSize)
      requireServerSuccess(response)
      return response.data ?? { page, page_size: pageSize, total: 0, items: [] }
    },
    enabled: channelId > 0,
    refetchInterval: (query) =>
      query.state.data?.items.some((item) => item.verdict === 'running')
        ? 1500
        : false,
  })
  const suiteRunQuery = useQuery({
    queryKey: channelsQueryKeys.qualitySuiteRun(channelId),
    queryFn: async () => {
      const response = await getChannelQualitySuiteRun(channelId)
      requireServerSuccess(response)
      return suiteRunData(response.data)
    },
    enabled: channelId > 0,
    refetchInterval: (query) =>
      query.state.data?.verdict === 'running' ? 1500 : false,
  })

  const selectedRun = selectedRunSnapshot
    ? (runsQuery.data?.items.find(
        (item) => item.id === selectedRunSnapshot.id
      ) ??
      suiteRunQuery.data?.items.find(
        (item) => item.id === selectedRunSnapshot.id
      ) ??
      (runQuery.data?.id === selectedRunSnapshot.id
        ? runQuery.data
        : selectedRunSnapshot))
    : null

  useEffect(() => {
    setProbeId('')
    setKeyIndex('')
    setPage(1)
    setSelectedRun(null)
    setSuiteOpen(false)
    setModel(parseModelsList(channel?.models ?? '')[0] ?? '')
  }, [channel?.id, channel?.models])

  useEffect(() => {
    if (probeId || !probesQuery.data?.length) return
    setProbeId(String(probesQuery.data[0].id))
  }, [probeId, probesQuery.data])

  const startAudit = useMutation({
    mutationFn: async () => {
      const payload: { probe_id: number; model: string; key_index?: number } = {
        probe_id: Number(probeId),
        model: model.trim(),
      }
      if (keyIndex.trim() !== '') {
        payload.key_index = Number(keyIndex)
      }
      const response = await startChannelQuality(channelId, payload)
      requireServerSuccess(response)
      return response.data
    },
    onSuccess: async () => {
      toast.success(t('Quality audit started'))
      setPage(1)
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.qualityRun(channelId),
        }),
        queryClient.invalidateQueries({
          queryKey: [...channelsQueryKeys.all, 'quality', 'runs', channelId],
        }),
        queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.qualityLatest(),
        }),
        queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.qualitySuiteRun(channelId),
        }),
      ])
    },
  })

  if (!channel) {
    return (
      <p className='text-muted-foreground text-sm'>{t('Select a channel')}</p>
    )
  }

  const models = parseModelsList(channel.models)
  const run = runQuery.data ?? null
  const suiteRun = suiteRunQuery.data ?? null
  const history = runsQuery.data?.items ?? []
  const total = runsQuery.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const canAudit = (QUALITY_AUDIT_CHANNEL_TYPES as readonly number[]).includes(
    channel.type
  )

  let historyContent: ReactNode = null
  if (runsQuery.isPending || suiteRunQuery.isPending) {
    historyContent = <LoadingState />
  } else if (runsQuery.isError || suiteRunQuery.isError) {
    historyContent = (
      <ErrorState
        onRetry={() => {
          void Promise.all([runsQuery.refetch(), suiteRunQuery.refetch()])
        }}
      />
    )
  } else if (history.length === 0 && !suiteRun) {
    historyContent = (
      <p className='text-muted-foreground text-sm'>
        {t('No quality audit yet')}
      </p>
    )
  } else if (history.length > 0) {
    historyContent = (
      <div className='overflow-hidden rounded-lg border'>
        {history.map((item) => (
          <button
            key={item.id}
            type='button'
            className='hover:bg-muted flex w-full items-center justify-between gap-3 border-b px-3 py-2 text-left last:border-b-0'
            onClick={() => setSelectedRun(item)}
          >
            <span className='min-w-0'>
              <span className='block truncate text-sm'>
                {item.probe_name || t('Probe')}
              </span>
              <span className='text-muted-foreground block truncate text-xs'>
                {item.requested_model || '-'}
                {' · '}
                {formatQualityTime(item.created_at)}
              </span>
            </span>
            <ChannelQualityVerdictBadge verdict={item.verdict} />
          </button>
        ))}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      {canAudit ? (
        <div className='flex flex-col gap-3'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='quality-probe'>{t('Probe')}</Label>
            <Combobox
              id='quality-probe'
              options={(probesQuery.data ?? []).map((probe) => ({
                value: String(probe.id),
                label: probe.name,
              }))}
              value={probeId || null}
              onValueChange={(value) => setProbeId(value ?? '')}
              placeholder={t('Select a probe')}
              aria-label={t('Probe')}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='quality-model'>{t('Model')}</Label>
            <Combobox
              id='quality-model'
              options={models.map((item) => ({ value: item, label: item }))}
              value={model || null}
              allowCustomValue
              onValueChange={(value) => setModel(value ?? '')}
              placeholder={t('Select a model')}
              aria-label={t('Model')}
            />
          </div>
          {isMultiKeyChannel(channel) ? (
            <div className='flex flex-col gap-1.5'>
              <Label htmlFor='quality-key'>{t('Key')}</Label>
              <Input
                id='quality-key'
                type='number'
                min={0}
                value={keyIndex}
                placeholder={t('Default key')}
                onChange={(event) => setKeyIndex(event.target.value)}
              />
            </div>
          ) : null}
          {canOperate ? (
            <Button
              type='button'
              className='self-start'
              disabled={
                probesQuery.isPending ||
                probesQuery.isError ||
                runQuery.isPending ||
                runQuery.isError ||
                suiteRunQuery.isPending ||
                suiteRunQuery.isError ||
                startAudit.isPending ||
                run?.verdict === 'running' ||
                suiteRun?.verdict === 'running'
              }
              onClick={() => {
                if (!probeId) {
                  toast.error(t('Select a probe'))
                  return
                }
                if (!model.trim()) {
                  toast.error(t('Select a model'))
                  return
                }
                if (keyIndex.trim() !== '') {
                  const index = Number(keyIndex)
                  if (!Number.isInteger(index) || index < 0) {
                    toast.error(t('account index out of range'))
                    return
                  }
                }
                startAudit.mutate()
              }}
            >
              {t('Run audit')}
            </Button>
          ) : null}
        </div>
      ) : (
        <p className='text-sm'>
          {t('only Anthropic and AWS channels can be audited')}
        </p>
      )}

      <div className='flex flex-col gap-2'>
        {suiteRun ? (
          <button
            type='button'
            className='hover:bg-muted flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left'
            onClick={() => setSuiteOpen(true)}
          >
            <span className='min-w-0'>
              <span className='block truncate text-sm'>
                {suiteRun.suite_name || t('Quality suite')}
              </span>
              <span className='text-muted-foreground block truncate text-xs'>
                {formatQualityTime(suiteRun.created_at)}
              </span>
            </span>
            <ChannelQualityVerdictBadge verdict={suiteRun.verdict} />
          </button>
        ) : null}
        {historyContent}
        {total > pageSize ? (
          <div className='flex items-center justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={() => setPage((current) => Math.max(1, current - 1))}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {page} / {pageCount}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page >= pageCount}
              onClick={() => setPage((current) => current + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        ) : null}
      </div>
      <Dialog
        open={selectedRun != null}
        onOpenChange={(open) => {
          if (!open) setSelectedRun(null)
        }}
        title={selectedRun?.probe_name || t('Quality result')}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 640px)'
      >
        {selectedRun ? (
          <SingleResult key={selectedRun.id} run={selectedRun} />
        ) : null}
      </Dialog>
      <Dialog
        open={suiteOpen && suiteRun != null}
        onOpenChange={setSuiteOpen}
        title={suiteRun?.suite_name || t('Quality suite')}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 640px)'
      >
        {suiteRun ? (
          <SuiteResult
            run={suiteRun}
            onSelect={(item) => {
              setSuiteOpen(false)
              setSelectedRun(item)
            }}
          />
        ) : null}
      </Dialog>
    </div>
  )
}

function formatQualityTime(createdAt: number): string {
  if (!createdAt) return '-'
  return formatDateTimeObject(new Date(createdAt * 1000))
}

function suiteRunData(
  value: ChannelQualitySuiteRun | null | undefined
): ChannelQualitySuiteRun | null {
  if (!value || typeof value !== 'object' || !('verdict' in value)) {
    return null
  }
  return value
}

function SingleResult(props: { run: ChannelQualityRun | null }) {
  const { t } = useTranslation()
  const run = props.run
  if (!run) return null
  return (
    <div className='flex flex-col gap-2'>
      <ChannelQualityVerdictBadge verdict={run.verdict} />
      <div className='overflow-hidden rounded-lg border'>
        <dl className='grid gap-3 p-3 text-sm sm:grid-cols-3'>
          <ResultRow label={t('Probe')} value={run.probe_name} />
          <ResultRow
            label={t('Requested model')}
            value={run.requested_model || '-'}
          />
          <ResultRow
            label={t('Upstream model')}
            value={run.upstream_model || '-'}
          />
        </dl>
        {run.error ? (
          <p className='text-destructive border-t px-3 py-2 text-sm'>
            {translateServerText(t, run.error)}
          </p>
        ) : null}
        {run.assertion_results?.length ? (
          <ul className='divide-y border-t'>
            {run.assertion_results.map((assertion) => (
              <li
                key={`${assertion.name}-${assertion.expr}`}
                className='flex items-center justify-between gap-3 px-3 py-2'
              >
                <span className='min-w-0'>
                  <span className='block truncate text-sm'>
                    {assertion.name || t('Assertion name')}
                  </span>
                  {assertion.error ? (
                    <span className='text-muted-foreground block truncate text-xs'>
                      {translateServerText(t, assertion.error)}
                    </span>
                  ) : null}
                </span>
                <ChannelQualityVerdictBadge
                  verdict={assertion.passed ? 'passed' : 'failed'}
                />
              </li>
            ))}
          </ul>
        ) : null}
        {run.actual_edits != null ? (
          <JsonRow label={t('Applied edits')} value={run.actual_edits} />
        ) : null}
        {run.usage != null ? (
          <JsonRow label={t('Usage')} value={run.usage} />
        ) : null}
      </div>
      <ChannelQualityResponse run={run} />
    </div>
  )
}

function SuiteResult(props: {
  run: ChannelQualitySuiteRun
  onSelect?: (item: ChannelQualityRun) => void
}) {
  const { t } = useTranslation()
  const run = props.run
  return (
    <div className='flex flex-col gap-2'>
      <ChannelQualityVerdictBadge verdict={run.verdict} />
      <div className='overflow-hidden rounded-lg border'>
        <dl className='grid gap-3 p-3 text-sm'>
          <ResultRow label={t('Suite')} value={run.suite_name || '-'} />
        </dl>
        {run.error ? (
          <p className='text-destructive border-t px-3 py-2 text-sm'>
            {translateServerText(t, run.error)}
          </p>
        ) : null}
        {run.items?.length ? (
          <ul className='divide-y border-t'>
            {run.items.map((item) => (
              <li key={item.id}>
                <button
                  type='button'
                  className='hover:bg-muted flex w-full items-center justify-between gap-3 px-3 py-2 text-left'
                  onClick={() => props.onSelect?.(item)}
                >
                  <span className='min-w-0'>
                    <span className='block truncate text-sm'>
                      {item.probe_name || t('Probe')}
                    </span>
                    <span className='text-muted-foreground block truncate text-xs'>
                      {item.requested_model || '-'}
                    </span>
                    {item.error ? (
                      <span className='text-muted-foreground block truncate text-xs'>
                        {translateServerText(t, item.error)}
                      </span>
                    ) : null}
                  </span>
                  <ChannelQualityVerdictBadge verdict={item.verdict} />
                </button>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </div>
  )
}

function ResultRow(props: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='truncate font-medium'>{props.value}</dd>
    </div>
  )
}

function JsonRow(props: { label: string; value: unknown }) {
  return (
    <div className='grid gap-1 border-t p-3'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd>
        <pre className='bg-muted overflow-x-auto rounded-md p-2 font-mono text-xs'>
          {JSON.stringify(props.value, null, 2)}
        </pre>
      </dd>
    </div>
  )
}
