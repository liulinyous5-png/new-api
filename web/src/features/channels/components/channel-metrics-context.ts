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
import { createContext, useContext } from 'react'

import type {
  ChannelMetric,
  ChannelMetricsResponse,
} from '../lib/channel-metrics'

export const ChannelMetricsContext = createContext<{
  minutes: number
  setMinutes: (minutes: number) => void
  data?: ChannelMetricsResponse
  metrics: Map<number, ChannelMetric>
  loading: boolean
  error: boolean
  refresh: () => void
  select: (ids: number[], name: string) => void
} | null>(null)

export function useChannelMetrics() {
  return useContext(ChannelMetricsContext)
}
