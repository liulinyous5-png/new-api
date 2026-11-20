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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  render,
  screen,
  within,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type {
  ChannelMetric,
  ChannelMetricsResponse,
} from '../../lib/channel-metrics'
import { channelSchema } from '../../types'
import { ChannelMetricsCell } from '../channel-metrics-cell'
import { ChannelMetricsProvider } from '../channel-metrics-provider'
import { ChannelsProvider } from '../channels-provider'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})
const clients: QueryClient[] = []
const channel = channelSchema.parse({
  id: 1,
  name: 'Channel A',
  type: 1,
  key: '',
  status: 1,
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
})
const metric: ChannelMetric = {
  channel_id: 1,
  updated_at: 100,
  success: 90,
  failure: 10,
  excluded: 5,
  first_success: 80,
  first_failure: 10,
  success_rate: 90,
  first_success_rate: 88.89,
  results: { success: 90, timeout: 10, cancelled: 5 },
  models: [
    {
      model: 'model-a',
      kind: 'relay',
      success: 90,
      failure: 10,
      excluded: 5,
      first_success: 80,
      first_failure: 10,
      success_rate: 90,
      first_success_rate: 88.89,
      results: { success: 90, timeout: 10, cancelled: 5 },
    },
  ],
}

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  vi.restoreAllMocks()
})

function renderMetrics(data: ChannelMetricsResponse | Error, tag = false) {
  const channels = tag ? [channel, { ...channel, id: 2 }] : [channel]
  const row = tag ? { ...channel, children: channels } : channel
  const get = vi.spyOn(api, 'get')
  if (data instanceof Error) get.mockRejectedValue(data)
  else get.mockResolvedValue({ data: { success: true, data } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <ChannelMetricsProvider channels={channels}>
            <ChannelMetricsCell channel={row} />
          </ChannelMetricsProvider>
        </ChannelsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  return get
}

test('channel attempts open model and error details and changing the time window fetches matching data', async () => {
  const get = renderMetrics({
    enabled: true,
    window_start: 1,
    window_end: 100,
    channels: [metric],
  })
  const user = userEvent.setup()
  const button = await screen.findByRole('button', {
    name: 'View channel success rates',
  })
  expect(button).toHaveTextContent('90% · 100 attempts')
  expect(button).not.toHaveTextContent('First attempt')
  expect(get.mock.calls[0][1]?.params.minutes).toBe(5)
  await user.click(button)
  const dialog = screen.getByRole('dialog')
  expect(
    within(dialog).getByRole('heading', { name: 'Channel success rates' })
  ).toBeVisible()
  expect(
    within(dialog).getByRole('columnheader', {
      name: 'First-attempt success rate',
    })
  ).toBeVisible()
  expect(within(dialog).getByRole('cell', { name: '88.89%' })).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'model-a' })).toBeVisible()
  expect(within(dialog).getByRole('cell', { name: 'Timeout' })).toBeVisible()
  expect(
    within(dialog).getByRole('cell', { name: 'Client cancelled' })
  ).toBeVisible()
  await user.click(within(dialog).getByRole('button', { name: 'model-a' }))
  expect(
    within(dialog).getByRole('button', { name: 'All Models' })
  ).toBeVisible()
  await user.click(within(dialog).getByRole('button', { name: 'All Models' }))
  expect(
    within(dialog).queryByRole('button', { name: 'All Models' })
  ).not.toBeInTheDocument()
  for (const [label, minutes] of [
    ['Last 30 minutes', 30],
    ['Last 1 hour', 60],
    ['Last 5 minutes', 5],
  ] as const) {
    await user.click(
      within(dialog).getByRole('combobox', { name: 'Success rate window' })
    )
    expect(
      (await screen.findAllByRole('option')).map((option) => option.textContent)
    ).toEqual(['Last 5 minutes', 'Last 30 minutes', 'Last 1 hour'])
    await user.click(screen.getByRole('option', { name: label }))
    await waitFor(() =>
      expect(
        get.mock.calls.some(
          ([url, config]) =>
            url === '/api/channel/metrics' && config?.params.minutes === minutes
        )
      ).toBe(true)
    )
    expect(
      within(dialog).getByRole('combobox', { name: 'Success rate window' })
    ).toHaveTextContent(label)
  }
  expect(
    get.mock.calls.every(
      ([, config]) =>
        config?.params.channel_ids === '1' && config?.params.hours === undefined
    )
  ).toBe(true)
})

test('a late response from an earlier window cannot replace the selected window', async () => {
  const get = renderMetrics({
    enabled: true,
    window_start: 1,
    window_end: 100,
    channels: [metric],
  })
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'View channel success rates' })
  )
  const pending = new Map<
    number,
    (value: {
      data: { success: boolean; data: ChannelMetricsResponse }
    }) => void
  >()
  get.mockImplementation(
    (_url, config) =>
      new Promise((resolve) => {
        pending.set(config?.params.minutes, resolve)
      })
  )
  for (const label of ['Last 30 minutes', 'Last 1 hour']) {
    await user.click(
      screen.getByRole('combobox', { name: 'Success rate window' })
    )
    await user.click(await screen.findByRole('option', { name: label }))
  }
  await waitFor(() => expect([...pending.keys()]).toEqual([30, 60]))
  await act(async () => {
    pending.get(60)?.({
      data: {
        success: true,
        data: {
          enabled: true,
          window_start: 1,
          window_end: 100,
          channels: [{ ...metric, success: 60, failure: 40, success_rate: 60 }],
        },
      },
    })
  })
  const dialog = screen.getByRole('dialog')
  expect(await within(dialog).findByText('60%')).toBeVisible()
  await act(async () => {
    pending.get(30)?.({
      data: {
        success: true,
        data: {
          enabled: true,
          window_start: 1,
          window_end: 100,
          channels: [{ ...metric, success: 30, failure: 70, success_rate: 30 }],
        },
      },
    })
  })
  expect(within(dialog).getByText('60%')).toBeVisible()
  expect(within(dialog).queryByText('30%')).not.toBeInTheDocument()
  expect(
    screen.getByRole('combobox', { name: 'Success rate window' })
  ).toHaveTextContent('Last 1 hour')
})

test.each([
  {
    name: 'a failed attempt',
    success: 0,
    failure: 1,
    rate: '0%',
    colors: ['text-red-600', 'dark:text-red-400'],
  },
  {
    name: 'a successful attempt',
    success: 1,
    failure: 0,
    rate: '100%',
    colors: ['text-emerald-600', 'dark:text-emerald-400'],
  },
  {
    name: '70% success',
    success: 7,
    failure: 3,
    rate: '70%',
    colors: ['text-amber-600', 'dark:text-amber-400'],
  },
  {
    name: '90% success',
    success: 9,
    failure: 1,
    rate: '90%',
    colors: ['text-emerald-500', 'dark:text-emerald-300'],
  },
])(
  'colors $name with fewer than 20 attempts while retaining the sample notice',
  async ({ success, failure, rate, colors }) => {
    renderMetrics({
      enabled: true,
      window_start: 1,
      window_end: 100,
      channels: [
        {
          ...metric,
          success,
          failure,
          excluded: 0,
          first_success: success,
          first_failure: failure,
          success_rate: Number.parseFloat(rate),
          first_success_rate: Number.parseFloat(rate),
          results: { success, timeout: failure },
          models: [],
        },
      ],
    })
    const button = await screen.findByRole('button', {
      name: 'View channel success rates',
    })
    expect(
      within(button).getByText(`${rate} · ${success + failure} attempts`)
    ).toHaveClass(...colors)
    expect(within(button).getByText('Insufficient samples')).toBeVisible()
  }
)

test('channels without eligible samples display a dash rather than a perfect success rate', async () => {
  renderMetrics({
    enabled: true,
    window_start: 1,
    window_end: 100,
    channels: [],
  })
  const button = await screen.findByRole('button', {
    name: 'View channel success rates',
  })
  expect(button).toHaveTextContent('— · 0 attempts')
  expect(within(button).getByText('— · 0 attempts')).toHaveClass(
    'text-muted-foreground'
  )
  expect(button).not.toHaveTextContent('100%')
  await userEvent.setup().click(button)
  expect(await screen.findByText('No metrics yet')).toBeVisible()
})

test('disabled collection is visible instead of being presented as no traffic', async () => {
  renderMetrics({
    enabled: false,
    window_start: 1,
    window_end: 100,
    channels: [],
  })
  expect(await screen.findByText('Metrics disabled')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'View channel success rates' })
  ).not.toBeInTheDocument()
})

test('a failed metrics request offers retry and restores the rates after recovery', async () => {
  const get = renderMetrics(new Error('metrics unavailable'))
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(
    screen.queryByRole('button', { name: 'View channel success rates' })
  ).not.toBeInTheDocument()
  get.mockResolvedValue({
    data: {
      success: true,
      data: {
        enabled: true,
        window_start: 1,
        window_end: 100,
        channels: [metric],
      },
    },
  })
  await userEvent.setup().click(retry)
  expect(
    await screen.findByRole('button', { name: 'View channel success rates' })
  ).toHaveTextContent('90% · 100 attempts')
})

test('tag success rates weight request counts instead of averaging channel percentages', async () => {
  const failed: ChannelMetric = {
    ...metric,
    channel_id: 2,
    success: 0,
    failure: 1,
    excluded: 0,
    first_success: 0,
    first_failure: 1,
    success_rate: 0,
    first_success_rate: 0,
    results: { timeout: 1 },
    models: [],
  }
  renderMetrics(
    {
      enabled: true,
      window_start: 1,
      window_end: 100,
      channels: [metric, failed],
    },
    true
  )
  const button = await screen.findByRole('button', {
    name: 'View channel success rates',
  })
  expect(button).toHaveTextContent('89.11% · 101 attempts')
  expect(button).not.toHaveTextContent('45%')
})
