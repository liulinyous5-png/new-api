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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { getSuccessRateTextClass } from '@/features/performance-metrics/lib/format'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { isTagAggregateRow } from '../lib'
import { combineChannelMetrics } from '../lib/channel-metrics'
import type { Channel } from '../types'
import { useChannelMetrics } from './channel-metrics-context'
import { useChannels } from './channels-provider'

export function ChannelMetricsWindow() {
  const { t } = useTranslation()
  const context = useChannelMetrics()
  if (!context) return null
  const windows = [
    { value: '5', label: t('Last 5 minutes') },
    { value: '30', label: t('Last 30 minutes') },
    { value: '60', label: t('Last 1 hour') },
  ]
  return (
    <Select
      items={windows}
      value={String(context.minutes)}
      onValueChange={(value) => {
        if (value) context.setMinutes(Number(value))
      }}
    >
      <SelectTrigger size='sm' aria-label={t('Success rate window')}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {windows.map((window) => (
          <SelectItem key={window.value} value={window.value}>
            {window.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function ChannelMetricsCell(props: { channel: Channel }) {
  const { t, i18n } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const context = useChannelMetrics()
  if (!context) return <span className='text-muted-foreground'>—</span>
  if (context.loading) return <Skeleton className='h-9 w-28' />
  if (context.error) {
    return (
      <Button variant='ghost' size='sm' onClick={context.refresh}>
        {t('Retry')}
      </Button>
    )
  }
  if (context.data?.enabled === false) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Metrics disabled')}
      </span>
    )
  }
  const ids = isTagAggregateRow(props.channel)
    ? props.channel.children.map((channel) => channel.id)
    : [props.channel.id]
  const metric = combineChannelMetrics(
    ids.flatMap((id) => context.metrics.get(id) ?? [])
  )
  const total = metric.success + metric.failure
  const rate =
    metric.success_rate === null
      ? '—'
      : `${formatNumber(metric.success_rate, locale)}%`
  return (
    <Button
      variant='ghost'
      size='sm'
      className='h-auto max-w-full flex-col items-start gap-0.5 px-0 py-1 text-xs'
      aria-label={t('View channel success rates')}
      onClick={() =>
        context.select(ids, sensitiveVisible ? props.channel.name : '••••')
      }
    >
      <span
        className={getSuccessRateTextClass(metric.success_rate ?? Number.NaN)}
      >
        {rate} ·{' '}
        {t('{{samples}} attempts', { samples: formatNumber(total, locale) })}
      </span>
      {total > 0 && total < 20 && (
        <span className='text-muted-foreground font-normal'>
          {t('Insufficient samples')}
        </span>
      )}
    </Button>
  )
}
