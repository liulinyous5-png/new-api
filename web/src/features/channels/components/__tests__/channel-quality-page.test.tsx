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
  fireEvent,
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

import type {
  ChannelQualityProbe,
  ChannelQualityRun,
  ChannelQualitySuite,
} from '../../api'
import { ChannelQualityPage } from '../../channel-quality-page'
import { CHANNEL_TYPE_ANTHROPIC, CHANNEL_TYPE_AWS } from '../../constants'
import { channelSchema, type Channel } from '../../types'
import { ChannelQualitySuites } from '../dialogs/channel-quality-suites'

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

function renderPage(
  row: Channel,
  run: ChannelQualityRun | null,
  probes: ChannelQualityProbe[] = [builtinProbe]
) {
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/channel/quality/probes') {
      return { data: { success: true, data: probes } }
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

function renderSuites() {
  return render(
    <QueryClientProvider client={client}>
      <ChannelQualitySuites canWrite />
    </QueryClientProvider>
  )
}

const suite: ChannelQualitySuite = {
  id: 5,
  name: 'Nightly checks',
  created_at: 1,
  updated_at: 1,
  items: [
    { id: 1, probe_id: 3, model: 'claude-sonnet', sort: 0 },
    { id: 2, probe_id: 3, model: 'claude-opus', sort: 1 },
  ],
}

it('updates an open result when the running record finishes', async () => {
  const user = userEvent.setup()
  const running = {
    ...passedRun(),
    verdict: 'running',
    assertion_results: null,
  }
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), running)
  await user.click(await screen.findByRole('tab', { name: 'Audit records' }))
  await user.click(
    await screen.findByRole('button', { name: /clear_tool_uses/ })
  )
  const dialog = await screen.findByRole('dialog', { name: 'clear_tool_uses' })
  expect(within(dialog).getByText('Running')).toBeInTheDocument()
  vi.mocked(api.get).mockImplementation(async (url) => ({
    data: {
      success: true,
      data: String(url).endsWith('/runs')
        ? { items: [passedRun()], total: 1, page: 1, page_size: 10 }
        : passedRun(),
    },
  }))
  await client.refetchQueries({ queryKey: ['channels', 'quality', 'runs', 8] })
  expect(await within(dialog).findAllByText('Passed')).toHaveLength(2)
  expect(within(dialog).queryByText('Running')).not.toBeInTheDocument()
})

it('shows loading instead of an empty suite list while fetching', async () => {
  let resolve!: (value: {
    data: { success: boolean; data: ChannelQualitySuite[] }
  }) => void
  vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/channel/quality/suites') {
      return new Promise((done) => {
        resolve = done
      })
    }
    return Promise.resolve({ data: { success: true, data: [builtinProbe] } })
  })
  renderSuites()
  expect(screen.getByText('Loading...')).toBeInTheDocument()
  expect(screen.queryByText('No quality suites yet')).not.toBeInTheDocument()
  resolve({ data: { success: true, data: [] } })
  expect(await screen.findByText('No quality suites yet')).toBeInTheDocument()
})

it('offers retry after a suite request fails without showing an empty list', async () => {
  const user = userEvent.setup()
  let failed = true
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/quality/suites' && failed) {
      throw new Error('offline')
    }
    return {
      data: {
        success: true,
        data: url === '/api/channel/quality/suites' ? [suite] : [builtinProbe],
      },
    }
  })
  renderSuites()
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(screen.queryByText('No quality suites yet')).not.toBeInTheDocument()
  failed = false
  await user.click(retry)
  expect(await screen.findByText('Nightly checks')).toBeInTheDocument()
})

it('filters suites by model and opens an explicit edit action', async () => {
  vi.stubGlobal('crypto', undefined)
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url === '/api/channel/quality/suites' ? [suite] : [builtinProbe],
    },
  }))
  renderSuites()
  expect(await screen.findByText('Nightly checks')).toBeInTheDocument()
  const search = screen.getByRole('textbox', { name: 'Search suites' })
  await user.type(search, 'not-found')
  expect(screen.getByText('No results found')).toBeInTheDocument()
  await user.clear(search)
  await user.type(search, 'claude-opus')
  await user.click(screen.getByRole('button', { name: 'Edit Nightly checks' }))
  expect(
    await screen.findByRole('dialog', { name: 'Quality suite' })
  ).toBeInTheDocument()
})

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
  vi.unstubAllGlobals()
  useAuthStore.setState({ auth: originalAuth })
})

it('hides channels that are not Anthropic or AWS', async () => {
  const user = userEvent.setup()
  renderPage(channel(1), null)
  expect(
    screen.queryByRole('button', { name: 'Quality Webhook alerts' })
  ).not.toBeInTheDocument()
  expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
    'Manage suites',
    'Manage probes',
    'Audit records',
    'Endpoint audit',
  ])
  await user.click(await screen.findByRole('tab', { name: 'Audit records' }))
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

it('does not fetch channel results until the records tab is opened', async () => {
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null)
  expect(await screen.findByText('No quality suites yet')).toBeInTheDocument()
  expect(
    vi
      .mocked(api.get)
      .mock.calls.some(
        ([url]) =>
          url === '/api/channel' || url === '/api/channel/quality/latest'
      )
  ).toBe(false)
})

it('lets an AWS channel run a Messages audit', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_AWS), null)
  await user.click(await screen.findByRole('tab', { name: 'Audit records' }))
  expect(await screen.findByRole('button', { name: 'Run audit' })).toBeEnabled()
  expect(
    screen.queryByText(
      'This audit sends an Anthropic Messages request, so AWS channels cannot run it yet.'
    )
  ).not.toBeInTheDocument()
})

it('opens the suite dialog from the quality page', async () => {
  vi.stubGlobal('crypto', undefined)
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
  await user.click(screen.getByRole('button', { name: 'Add probe' }))
  expect(screen.getAllByRole('combobox', { name: 'Probe' })).toHaveLength(2)
})

it('shows the passed verdict and opens the result details', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), passedRun())
  await user.click(await screen.findByRole('tab', { name: 'Audit records' }))
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

it.each(['available', 'missing randomUUID', 'missing crypto'])(
  'keeps a builtin probe read-only with browser crypto: %s',
  async (availability) => {
    if (availability === 'missing randomUUID') {
      vi.stubGlobal('crypto', {
        getRandomValues: globalThis.crypto.getRandomValues.bind(
          globalThis.crypto
        ),
      })
    } else if (availability === 'missing crypto') {
      vi.stubGlobal('crypto', undefined)
    }
    const user = userEvent.setup()
    renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null)
    await user.click(await screen.findByRole('tab', { name: 'Manage probes' }))
    const section = await screen.findByRole('tabpanel', {
      name: 'Manage probes',
    })
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
  }
)

it('adds a custom status, a header row, and an assertion from the new probe form', async () => {
  vi.stubGlobal('crypto', undefined)
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
  await user.click(screen.getByRole('button', { name: 'Add condition' }))
  expect(screen.getAllByLabelText('Field path')).toHaveLength(2)
})

it('opens and saves a legacy probe with a null assertion list', async () => {
  const user = userEvent.setup()
  const probe = {
    ...builtinProbe,
    id: 4,
    name: 'Status only',
    builtin: false,
    assertions: null,
  }
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: probe } })
  // Older records can return null for the optional assertion list.
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null, [probe])
  await user.click(screen.getByRole('tab', { name: 'Manage probes' }))
  await user.click(await screen.findByRole('button', { name: 'Status only' }))
  expect(
    screen.getByText('No assertions yet. A matching response status is enough.')
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save probe' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/channel/quality/probes/4', {
      name: 'Status only',
      headers: probe.headers,
      body: probe.body,
      statuses: [200],
      assertions: [],
    })
  )
  expect(screen.getByLabelText('Probe name')).toHaveValue('Status only')
})

it('duplicates and saves a probe without crypto while preserving its custom assertion', async () => {
  vi.stubGlobal('crypto', undefined)
  const user = userEvent.setup()
  const original = {
    ...builtinProbe,
    assertions: [{ name: 'Has content', expr: 'len(content) > 0' }],
  }
  const copied = {
    ...original,
    id: 4,
    name: 'clear_tool_uses_copy',
    builtin: false,
  }
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: copied } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: { ...copied, name: 'Saved copy' } },
  })
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null, [original])
  await user.click(screen.getByRole('tab', { name: 'Manage probes' }))
  await user.click(await screen.findByRole('button', { name: original.name }))
  await user.click(screen.getByRole('button', { name: 'Duplicate' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Probe name')).toHaveValue(copied.name)
  )
  expect(post).toHaveBeenCalledWith('/api/channel/quality/probes/3/copy')
  expect(screen.getByLabelText('Probe name')).toBeEnabled()
  expect(screen.getByLabelText('Header value')).toHaveValue(
    original.headers['anthropic-beta']
  )
  await user.click(screen.getByRole('button', { name: 'Edit assertion' }))
  const editor = await screen.findByRole('dialog', { name: 'Edit assertion' })
  expect(
    within(editor).getByRole('button', { name: 'Replace condition' })
  ).toBeInTheDocument()
  await user.click(within(editor).getByRole('button', { name: 'Cancel' }))
  await user.clear(screen.getByLabelText('Probe name'))
  await user.type(screen.getByLabelText('Probe name'), 'Saved copy')
  await user.click(screen.getByRole('button', { name: 'Save probe' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/channel/quality/probes/4', {
      name: 'Saved copy',
      headers: original.headers,
      body: original.body,
      statuses: [200],
      assertions: original.assertions,
    })
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save probe' })).toBeEnabled()
  )
  expect(screen.getByText('len(content) > 0')).toBeInTheDocument()
})

it('clears unsaved header rows when switching to a different probe with the same headers', async () => {
  const user = userEvent.setup()
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null, [
    {
      ...builtinProbe,
      id: 4,
      name: 'First probe',
      builtin: false,
      headers: {},
    },
    {
      ...builtinProbe,
      id: 5,
      name: 'Second probe',
      builtin: false,
      headers: {},
    },
  ])
  await user.click(screen.getByRole('tab', { name: 'Manage probes' }))
  await user.click(await screen.findByRole('button', { name: 'First probe' }))
  await user.click(screen.getByRole('button', { name: 'Add header' }))
  expect(screen.getByLabelText('Header name')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Second probe' }))
  expect(screen.queryByLabelText('Header name')).not.toBeInTheDocument()
  expect(screen.getByText('No headers yet')).toBeInTheDocument()
})

it('explains non-string header values and preserves JSON edits until they are corrected', async () => {
  const user = userEvent.setup()
  const probe = { ...builtinProbe, id: 4, builtin: false, headers: {} }
  renderPage(channel(CHANNEL_TYPE_ANTHROPIC), null, [probe])
  await user.click(screen.getByRole('tab', { name: 'Manage probes' }))
  await user.click(
    await screen.findByRole('button', { name: 'clear_tool_uses' })
  )
  await user.click(screen.getByRole('tab', { name: 'JSON' }))
  const invalid = '{"user_group_ratio":-1}'
  fireEvent.input(screen.getByRole('textbox', { name: 'Headers' }), {
    target: { value: invalid },
  })
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Request headers must be a JSON object with string values.'
  )
  await user.click(screen.getByRole('tab', { name: 'Visual' }))
  expect(screen.getByRole('tab', { name: 'JSON' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(screen.getByRole('textbox', { name: 'Headers' })).toHaveValue(invalid)
  fireEvent.input(screen.getByRole('textbox', { name: 'Headers' }), {
    target: { value: '{"user_group_ratio":"-1"}' },
  })
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Visual' }))
  expect(screen.getByLabelText('Header name')).toHaveValue('user_group_ratio')
  expect(screen.getByLabelText('Header value')).toHaveValue('-1')
})

it('wraps long suite names and model identifiers without hiding their text', async () => {
  const longName = 'Quality'.repeat(40)
  const longModel = 'claude-'.repeat(40)
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/channel/quality/suites'
          ? [
              {
                ...suite,
                name: longName,
                items: [{ ...suite.items[0], model: longModel }],
              },
            ]
          : [builtinProbe],
    },
  }))
  renderSuites()
  const heading = await screen.findByRole('heading', { name: longName })
  expect(heading.parentElement).toHaveClass('break-words')
  expect(screen.getByText(longModel)).toHaveClass('break-all')
  expect(screen.getAllByRole('listitem')).toHaveLength(1)
})

it('opens suite details with the keyboard for a read-only user', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url === '/api/channel/quality/suites' ? [suite] : [builtinProbe],
    },
  }))
  render(
    <QueryClientProvider client={client}>
      <ChannelQualitySuites canWrite={false} />
    </QueryClientProvider>
  )
  const view = await screen.findByRole('button', {
    name: 'View details: Nightly checks',
  })
  view.focus()
  await user.keyboard('{Enter}')
  expect(
    await screen.findByRole('dialog', { name: 'Quality suite' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Suite name')).toBeDisabled()
  expect(
    screen.queryByRole('button', { name: 'Save suite' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Delete' })
  ).not.toBeInTheDocument()
})

it.each([
  [
    'retry_limit',
    'Rate limited; retry limit reached. No failure action was triggered by this result.',
  ],
  [
    'time_budget',
    'Rate limited; the next retry would exceed the time budget. No failure action was triggered by this result.',
  ],
])(
  'shows recorded retry counts and %s in audit details',
  async (reason, message) => {
    const user = userEvent.setup()
    renderPage(channel(CHANNEL_TYPE_ANTHROPIC), {
      ...passedRun(),
      verdict: 'unknown',
      execution: {
        attempts: 3,
        retry_limit: 2,
        elapsed_ms: 10000,
        stop_reason: reason,
      },
    })
    await user.click(await screen.findByRole('tab', { name: 'Audit records' }))
    await user.click(
      await screen.findByRole('button', { name: /clear_tool_uses/ })
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'clear_tool_uses',
    })
    const retries = within(dialog).getByText('Actual retries').parentElement
    expect(retries?.querySelector('dd')).toHaveTextContent('2')
    const limit = within(dialog).getByText('429 retry count').parentElement
    expect(limit?.querySelector('dd')).toHaveTextContent('2')
    expect(within(dialog).getByText('Elapsed time (ms)')).toBeVisible()
    expect(within(dialog).getByText(message)).toBeVisible()
  }
)
