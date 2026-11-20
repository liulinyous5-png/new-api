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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'

import type {
  ChannelMetric,
  ChannelModelMetric,
} from '../../lib/channel-metrics'
import { ChannelMetricsWindow } from '../channel-metrics-cell'
import { useChannelMetrics } from '../channel-metrics-context'
import { useChannels } from '../channels-provider'

export function ChannelMetricsDialog(props: {
  name: string
  metric: ChannelMetric
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const context = useChannelMetrics()
  const [model, setModel] = useState<ChannelModelMetric | null>(null)
  const activeModel = props.metric.models.find(
    (item) => item.model === model?.model && item.kind === model?.kind
  )
  const counts = activeModel ?? props.metric
  const labels: Record<string, string> = {
    success: t('Success'),
    timeout: t('Timeout'),
    network: t('Network error'),
    upstream_auth: t('Upstream authentication'),
    upstream_quota: t('Upstream quota'),
    rate_limit: t('Rate limited'),
    upstream_server: t('Upstream server error'),
    model_unavailable: t('Model unavailable'),
    protocol: t('Invalid or interrupted response'),
    gateway: t('Gateway error'),
    upstream_other: t('Other upstream errors'),
    business_rejected: t('Business rejection'),
    cancelled: t('Client cancelled'),
    unknown: t('Unknown outcome'),
  }
  const reasons = Object.entries(counts.results)
    .filter(([reason]) => reason !== 'success')
    .sort((a, b) => b[1] - a[1])

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Channel success rates')}
      description={sensitiveVisible ? props.name : '••••'}
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-4'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <ChannelMetricsWindow />
        <Button variant='outline' size='sm' onClick={context?.refresh}>
          {t('Refresh')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Each retry counts separately. First attempts only include requests initially routed to this channel. Cancellations and business rejections are excluded.'
        )}
      </p>
      {context?.error && <ErrorState onRetry={context.refresh} />}
      {!context?.error && context?.loading && (
        <Skeleton className='h-40 w-full' />
      )}
      {!context?.error && !context?.loading && (
        <>
          {context?.data?.enabled === false && <p>{t('Metrics disabled')}</p>}
          <div className='grid grid-cols-2 gap-3 text-sm sm:grid-cols-4'>
            <div>
              {t('Attempt success rate')}
              <div className='mt-1 font-medium tabular-nums'>
                {counts.success_rate === null
                  ? '—'
                  : `${formatNumber(counts.success_rate, locale)}%`}
              </div>
            </div>
            <div>
              {t('First-attempt success rate')}
              <div className='mt-1 font-medium tabular-nums'>
                {counts.first_success_rate === null
                  ? '—'
                  : `${formatNumber(counts.first_success_rate, locale)}%`}
              </div>
            </div>
            <div>
              {t('Valid attempts')}
              <div className='mt-1 font-medium tabular-nums'>
                {formatNumber(counts.success + counts.failure, locale)}
              </div>
            </div>
            <div>
              {t('Excluded attempts')}
              <div className='mt-1 font-medium tabular-nums'>
                {formatNumber(counts.excluded, locale)}
              </div>
            </div>
          </div>
          {counts.success + counts.failure > 0 &&
            counts.success + counts.failure < 20 && (
              <p className='text-muted-foreground text-xs'>
                {t('Insufficient samples')}
              </p>
            )}
          <StaticDataTable<ChannelModelMetric>
            data={props.metric.models}
            getRowKey={(row) => JSON.stringify([row.model, row.kind])}
            emptyContent={<EmptyState title={t('No metrics yet')} />}
            columns={[
              {
                id: 'model',
                header: t('Model'),
                cell: (row) => (
                  <Button
                    variant='link'
                    className='h-auto max-w-64 truncate p-0'
                    onClick={() => setModel(row)}
                  >
                    {sensitiveVisible ? row.model : '••••'}
                  </Button>
                ),
              },
              {
                id: 'kind',
                header: t('Type'),
                cell: (row) =>
                  row.kind === 'task_submit'
                    ? t('Task submission')
                    : t('API request'),
              },
              {
                id: 'rate',
                header: t('Attempt success rate'),
                cell: (row) =>
                  row.success_rate === null
                    ? '—'
                    : `${formatNumber(row.success_rate, locale)}%`,
              },
              {
                id: 'first',
                header: t('First-attempt success rate'),
                cell: (row) =>
                  row.first_success_rate === null
                    ? '—'
                    : `${formatNumber(row.first_success_rate, locale)}%`,
              },
              {
                id: 'total',
                header: t('Valid attempts'),
                cell: (row) => formatNumber(row.success + row.failure, locale),
              },
              {
                id: 'failed',
                header: t('Failed'),
                cell: (row) => formatNumber(row.failure, locale),
              },
            ]}
          />
          <div className='flex flex-wrap items-center justify-between gap-2 text-sm'>
            <span>
              {t('Outcome breakdown')}
              {activeModel && sensitiveVisible ? ` · ${activeModel.model}` : ''}
            </span>
            {activeModel && (
              <Button variant='ghost' size='sm' onClick={() => setModel(null)}>
                {t('All Models')}
              </Button>
            )}
          </div>
          <StaticDataTable
            data={reasons}
            getRowKey={(row) => row[0]}
            emptyContent={t('No failures or exclusions')}
            columns={[
              {
                id: 'reason',
                header: t('Reason'),
                cell: (row) => labels[row[0]] ?? t('Unknown outcome'),
              },
              {
                id: 'count',
                header: t('Count'),
                cell: (row) => formatNumber(row[1], locale),
              },
            ]}
          />
        </>
      )}
      {context?.data && (
        <p className='text-muted-foreground text-xs tabular-nums'>
          {formatTimestampToDate(context.data.window_start)} –{' '}
          {formatTimestampToDate(context.data.window_end)}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Statistics refresh every 15 seconds. The latest few seconds may still be pending.'
        )}
      </p>
    </Dialog>
  )
}
