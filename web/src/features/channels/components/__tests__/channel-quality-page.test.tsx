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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelQualityProbe, ChannelQualityRun } from '../../api'
import { ChannelQualityPage } from '../../channel-quality-page'
import { CHANNEL_TYPE_ANTHROPIC, CHANNEL_TYPE_AWS } from '../../constants'
import { channelSchema, type Channel } from '../../types'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient

const builtinProbe: ChannelQualityProbe = {
  id: 3,
  name: 'clear_tool_uses',
  builtin: true,
  headers: { 'anthropic-beta': 'context-management-2025-06-27' },
  body: '{"model":"{{model}}"}',
  statuses: [200],
  assertions: [
    {
      name: 'clear tool uses applied',
      expr: 'any(context_management?.applied_edits ?? [], { int(.cleared_tool_uses) == 3 })',
    },
  ],
  created_at: 1,
  updated_at: 1,
}

function channel(type: number): Channel {
  return channelSchema.parse({
    id: 8,
    type,
    key: 'sk-test',
    name: 'Relay',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'claude-opus-4-7',
  })
}

function passedRun(): ChannelQualityRun {
  return {
    id: 11,
    channel_id: 8,
    key_index: 0,
    requested_model: 'claude-opus-4-7',
    upstream_model: 'claude-haiku-4-5',
    probe_id: 3,
    probe_name: 'clear_tool_uses',
    verdict: 'passed',
    assertion_results: [
      {
        name: 'clear tool uses applied',
        expr: 'any([])',
        passed: true,
      },
    ],
    actual_edits: [{ type: 'clear_tool_uses_20250919', cleared_tool_uses: 3 }],
    usage: { input_tokens: 20 },
    error: '',
    created_at: 10,
  }
}

function renderPage(row: Channel, run: ChannelQualityRun | null) {
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/channel/quality/probes') {
      return { data: { success: true, data: [builtinProbe] } }
    }
    if (url === '/api/channel/quality/suites') {
      return { data: { success: true, data: [] } }
    }
    if (url === `/api/channel/${row.id}/quality/suite-run`) {
      return { data: { success: true, data: null } }
    }
    if (url === `/api/channel/${row.id}/quality`) {
      return { data: { success: true, data: run } }
    }
    if (url === `/api/channel/${row.id}/quality/runs`) {
      return {
        data: {
          success: true,
          data: {
            items: run ? [run] : [],
            total: run ? 1 : 0,
            page: 1,
            page_size: 10,
          },
        },
      }
    }
    if (url === '/api/channel' || url === '/api/channel/search') {
      const type = (config?.params as { type?: number } | undefined)?.type
      const matches = type === row.type
      return {
        data: {
          success: true,
          data: {
            items: matches ? [row] : [],
            total: matches ? 1 : 0,
            page: 1,
            page_size: 50,
          },
        },
      }
    }
    return { data: { success: true, data: [] } }
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelQualityPage channelId={row.id} onSelectChannel={vi.fn()} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState({ auth: originalAuth })
})

it('hides channels that are not Anthropic or AWS', async () => {
  renderPage(channel(1), null)
  await waitFor(() => {
    const channelCalls = vi
      .mocked(api.get)
      .mock.calls.filter((call) => call[0] === '/api/channel')
    expect(channelCalls).toHaveLength(2)
  })
  expect(screen.getByText('No channels found')).toBeInTheDocument()
  expect(screen.getByText('Select a channel')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /Relay/ })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Run audit' })
  ).not.toBeInTheDocument()
})

it('lets an AWS channel run a Messages audit', async () => {
  renderPage(channel(CHANNEL_TYPE_AWS), null)
  expect(await screen.findByRole('button', { name: 'Run audit' })).toBeEnabled()
  expect(
    screen.queryByText(
      'This audit sends an Anthropic Messages request, so AWS channels cannot run it yet.'
    )
  ).not.toBeInTheDocument()
})

it('opens the suite dialog from the quality page', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null)
  await user.click(await screen.findByRole('tab', { name: 'Manage suites' }))
  const section = await screen.findByRole('tabpanel', { name: 'Manage suites' })
  await user.click(within(section).getByRole('button', { name: 'New suite' }))
  expect(
    await screen.findByRole('dialog', { name: 'New suite' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Suite name')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Add probe' })).toBeInTheDocument()
  await user.click(screen.getByRole('combobox', { name: 'Probe' }))
  await user.click(
    await screen.findByRole('option', { name: 'clear_tool_uses' })
  )
  expect(screen.getByRole('combobox', { name: 'Probe' })).toHaveTextContent(
    'clear_tool_uses'
  )
})

it('shows the passed verdict and opens the result details', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), passedRun())
  const record = await screen.findByRole('button', { name: /clear_tool_uses/ })
  expect(record).toHaveTextContent('Passed')
  expect(record).toHaveTextContent('claude-opus-4-7')
  expect(screen.queryByText('clear tool uses applied')).not.toBeInTheDocument()
  expect(screen.queryByText('Applied edits')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Run audit' })).toBeEnabled()
  await user.click(record)
  const dialog = await screen.findByRole('dialog', { name: 'clear_tool_uses' })
  expect(within(dialog).getByText('claude-haiku-4-5')).toBeInTheDocument()
  expect(
    within(dialog).getByText('clear tool uses applied')
  ).toBeInTheDocument()
  expect(within(dialog).getByText('Applied edits')).toBeInTheDocument()
})

it('keeps a builtin probe read-only on the quality page', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null)
  await user.click(await screen.findByRole('tab', { name: 'Manage probes' }))
  const section = await screen.findByRole('tabpanel', { name: 'Manage probes' })
  await user.click(
    await within(section as HTMLElement).findByRole('button', {
      name: 'clear_tool_uses',
    })
  )
  expect(
    screen.getByText('Builtin probes cannot be edited')
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Probe name')).toBeDisabled()
  expect(screen.getByText('200')).toBeInTheDocument()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(
    screen.getByText('Any item in a list · context_management.applied_edits')
  ).toBeInTheDocument()
  expect(screen.getByText('cleared_tool_uses Equals 3')).toBeInTheDocument()
  expect(screen.queryByLabelText('List path')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Save probe' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Duplicate' })).toBeEnabled()
})

it('adds a custom status, a header row, and an assertion from the new probe form', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null)
  await user.click(await screen.findByRole('tab', { name: 'Manage probes' }))
  const section = await screen.findByRole('tabpanel', {
    name: 'Manage probes',
  })
  await user.click(
    await within(section).findByRole('button', { name: 'New probe' })
  )
  expect(
    await screen.findByRole('dialog', { name: 'New probe' })
  ).toBeInTheDocument()
  expect(within(section).queryByLabelText('Probe name')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Add header' }))
  expect(screen.getByLabelText('Header name')).toBeInTheDocument()
  const statusCode = screen.getByLabelText('Status code')
  expect(statusCode).toHaveValue('')
  await user.click(screen.getByRole('button', { name: 'Add status' }))
  expect(screen.getAllByText('200')).toHaveLength(1)
  await user.type(statusCode, '200')
  await user.click(screen.getByRole('button', { name: 'Add status' }))
  expect(statusCode).toHaveValue('')
  expect(screen.getAllByText('200')).toHaveLength(1)
  await user.type(statusCode, '400')
  await user.click(screen.getByRole('button', { name: 'Add status' }))
  expect(screen.getByText('400')).toBeInTheDocument()
  expect(statusCode).toHaveValue('')
  await user.click(screen.getByRole('button', { name: 'Add assertion' }))
  expect(
    await screen.findByRole('dialog', { name: 'Add assertion' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Assertion name')).toBeInTheDocument()
})
