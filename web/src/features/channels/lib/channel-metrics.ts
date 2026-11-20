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
export type ChannelMetricCounts = {
  success: number
  failure: number
  excluded: number
  first_success: number
  first_failure: number
  success_rate: number | null
  first_success_rate: number | null
  results: Record<string, number>
}

export type ChannelModelMetric = ChannelMetricCounts & {
  model: string
  kind: string
}

export type ChannelMetric = ChannelMetricCounts & {
  channel_id: number
  updated_at: number
  models: ChannelModelMetric[]
}

export type ChannelMetricsResponse = {
  enabled: boolean
  window_start: number
  window_end: number
  channels: ChannelMetric[]
}

/** Aggregate counts, including tag rows; never average percentages. */
export function combineChannelMetrics(metrics: ChannelMetric[]): ChannelMetric {
  const combined: ChannelMetric = {
    channel_id: 0,
    updated_at: 0,
    success: 0,
    failure: 0,
    excluded: 0,
    first_success: 0,
    first_failure: 0,
    success_rate: null,
    first_success_rate: null,
    results: {},
    models: [],
  }
  const models = new Map<string, ChannelModelMetric>()
  for (const metric of metrics) {
    combined.updated_at = Math.max(combined.updated_at, metric.updated_at)
    addMetricCounts(combined, metric)
    for (const model of metric.models) {
      const key = JSON.stringify([model.model, model.kind])
      const existing = models.get(key)
      if (existing) addMetricCounts(existing, model)
      else models.set(key, { ...model, results: { ...model.results } })
    }
  }
  combined.models = [...models.values()].sort(
    (a, b) => b.failure - a.failure || b.success - a.success
  )
  return combined
}

function addMetricCounts(
  target: ChannelMetricCounts,
  source: ChannelMetricCounts
): void {
  target.success += source.success
  target.failure += source.failure
  target.excluded += source.excluded
  target.first_success += source.first_success
  target.first_failure += source.first_failure
  for (const [reason, count] of Object.entries(source.results)) {
    target.results[reason] = (target.results[reason] ?? 0) + count
  }
  const total = target.success + target.failure
  const firstTotal = target.first_success + target.first_failure
  target.success_rate = total ? (target.success / total) * 100 : null
  target.first_success_rate = firstTotal
    ? (target.first_success / firstTotal) * 100
    : null
}
