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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState, type ReactNode } from 'react'

import { getChannelMetrics } from '../api'
import { channelsQueryKeys } from '../lib'
import { combineChannelMetrics } from '../lib/channel-metrics'
import type { Channel } from '../types'
import { ChannelMetricsContext } from './channel-metrics-context'
import { ChannelMetricsDialog } from './dialogs/channel-metrics-dialog'

export function ChannelMetricsProvider(props: {
  channels: Channel[]
  children: ReactNode
}) {
  const [minutes, setMinutes] = useState(5)
  const [selected, select] = useState<{ ids: number[]; name: string } | null>(
    null
  )
  const ids = useMemo(
    () => props.channels.map((channel) => channel.id).sort((a, b) => a - b),
    [props.channels]
  )
  const query = useQuery({
    queryKey: [...channelsQueryKeys.all, 'metrics', ids, minutes],
    queryFn: async ({ signal }) => {
      const batches = []
      for (let index = 0; index < ids.length; index += 200) {
        batches.push(
          getChannelMetrics(ids.slice(index, index + 200), minutes, signal)
        )
      }
      const results = await Promise.all(batches)
      return {
        ...results[0],
        channels: results.flatMap((result) => result.channels ?? []),
      }
    },
    enabled: ids.length > 0,
    refetchInterval: 15_000,
    staleTime: 10_000,
    meta: { errorToast: false },
  })
  const metrics = useMemo(
    () =>
      new Map(
        (query.data?.channels ?? []).map((item) => [item.channel_id, item])
      ),
    [query.data]
  )
  const selectedMetrics = selected
    ? combineChannelMetrics(selected.ids.flatMap((id) => metrics.get(id) ?? []))
    : null

  return (
    <ChannelMetricsContext.Provider
      value={{
        minutes,
        setMinutes,
        data: query.data,
        metrics,
        loading: ids.length > 0 && query.isPending,
        error: query.isError,
        refresh: () => {
          void query.refetch()
        },
        select: (channelIds, name) => select({ ids: channelIds, name }),
      }}
    >
      {props.children}
      {selected && selectedMetrics && (
        <ChannelMetricsDialog
          name={selected.name}
          metric={selectedMetrics}
          onClose={() => select(null)}
        />
      )}
    </ChannelMetricsContext.Provider>
  )
}
