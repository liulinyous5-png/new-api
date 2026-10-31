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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { ChannelQualityRun } from '../../api'
import { ChannelQualityResponse } from '../dialogs/channel-quality-response'

let client: QueryClient
const run: ChannelQualityRun = {
  id: 11,
  channel_id: 8,
  key_index: 0,
  requested_model: 'claude',
  upstream_model: 'claude',
  probe_id: 3,
  probe_name: 'probe',
  verdict: 'passed',
  assertion_results: null,
  actual_edits: null,
  usage: null,
  error: '',
  created_at: 1,
}
const raw = ' {"content":[{"text":"你好"}]}\n'
function response(body = raw, extra = {}) {
  let formattedBody: string | null = null
  try {
    formattedBody = JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    /* Non-JSON bodies have no formatted view. */
  }
  return {
    data: {
      success: true,
      data: {
        run_id: 11,
        status_code: 200,
        formatted_body: formattedBody,
        body_base64: Buffer.from(body).toString('base64'),
        truncated: false,
        incomplete: false,
        ...extra,
      },
    },
  }
}
function renderResponse(value = run) {
  return render(
    <QueryClientProvider client={client}>
      <ChannelQualityResponse run={value} />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})
afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

it('fetches only after expansion and preserves the raw response for copy and download', async () => {
  const user = userEvent.setup()
  const get = vi.spyOn(api, 'get').mockResolvedValue(response())
  renderResponse()
  const expand = screen.getByRole('button', { name: 'Full response' })
  expect(expand).toHaveAttribute('aria-expanded', 'false')
  expect(get).not.toHaveBeenCalled()
  await user.click(expand)
  expect((await screen.findByLabelText('Response body')).textContent).toBe(raw)
  expect(expand).toHaveAttribute('aria-expanded', 'true')
  expect(get).toHaveBeenCalledWith('/api/channel/8/quality/runs/11/response')
  await user.click(screen.getByRole('button', { name: 'Format JSON' }))
  expect(screen.getByLabelText('Response body').textContent).toBe(
    JSON.stringify(JSON.parse(raw), null, 2)
  )
  await user.click(screen.getByRole('button', { name: 'Copy to clipboard' }))
  expect(await navigator.clipboard.readText()).toBe(raw)
  expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute(
    'href',
    `data:application/octet-stream;base64,${Buffer.from(raw).toString('base64')}`
  )
  expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute(
    'download',
    'quality-audit-11.txt'
  )
})

it('shows non-JSON error responses as text with the HTTP status', async () => {
  const user = userEvent.setup()
  const html = '<html><h1>Unavailable</h1></html>'
  vi.spyOn(api, 'get').mockResolvedValue(response(html, { status_code: 503 }))
  renderResponse()
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  expect((await screen.findByLabelText('Response body')).textContent).toBe(html)
  expect(screen.getByText('HTTP 503')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Format JSON' })).toBeDisabled()
  expect(
    screen.queryByRole('heading', { name: 'Unavailable' })
  ).not.toBeInTheDocument()
})

it('marks partial responses and bounds the preview without truncating downloads', async () => {
  const user = userEvent.setup()
  const text = 'a'.repeat(65537)
  vi.spyOn(api, 'get').mockResolvedValue(
    response(text, { truncated: true, incomplete: true })
  )
  renderResponse()
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  expect(
    (await screen.findByLabelText('Response body')).textContent
  ).toHaveLength(65536)
  expect(
    screen.getByText('Response exceeded 1 MiB; only the first 1 MiB was saved')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Response reading was interrupted; saved content is incomplete'
    )
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Preview is limited; copy or download to view all saved content'
    )
  ).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute(
    'download',
    'quality-audit-11-partial.txt'
  )
  expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute(
    'href',
    `data:application/octet-stream;base64,${btoa(text)}`
  )
})

it('distinguishes an empty saved body from a historical record without a response', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue(response(''))
  renderResponse()
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  expect((await screen.findByLabelText('Response body')).textContent).toBe('')
  expect(
    screen.queryByText('No saved response for this audit')
  ).not.toBeInTheDocument()
})

it('shows a historical empty state when no response was saved', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: null },
  })
  renderResponse()
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  expect(
    await screen.findByText('No saved response for this audit')
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('link', { name: 'Download' })
  ).not.toBeInTheDocument()
})

it('offers retry after a failed request', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue(response())
  renderResponse()
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  await user.click(await screen.findByRole('button', { name: 'Retry' }))
  expect((await screen.findByLabelText('Response body')).textContent).toBe(raw)
})

it('waits for a running audit to finish before fetching its response', async () => {
  const user = userEvent.setup()
  const get = vi.spyOn(api, 'get').mockResolvedValue(response())
  const view = renderResponse({ ...run, verdict: 'running' })
  await user.click(screen.getByRole('button', { name: 'Full response' }))
  expect(screen.getByText('Response is not available yet')).toBeInTheDocument()
  expect(get).not.toHaveBeenCalled()
  view.rerender(
    <QueryClientProvider client={client}>
      <ChannelQualityResponse run={run} />
    </QueryClientProvider>
  )
  await waitFor(() =>
    expect(screen.getByLabelText('Response body')).toBeInTheDocument()
  )
})
