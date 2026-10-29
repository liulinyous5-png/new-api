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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { EndpointTestPanel } from './test-panel'

const suites = [
  {
    id: 7,
    name: 'check',
    items: [{ id: 1, probe_id: 1, model: 'claude-opus-4-6', sort: 0 }],
    created_at: 1,
    updated_at: 1,
  },
]
const runs = [
  {
    id: 3,
    suite_id: 7,
    suite_name: 'check',
    base_host: 'example.com',
    verdict: 'passed',
    error: '',
    created_at: 1_700_000_000,
    finished_at: 1_700_000_001,
    items: [
      {
        probe_name: 'clear_tool_uses',
        model: 'claude-opus-4-6',
        verdict: 'passed',
        status: 200,
        assertions: [
          {
            name: 'clear tool uses applied',
            expr: 'int(usage.input_tokens) == 8',
            passed: true,
          },
        ],
        response:
          '{"id":"msg_test","content":[{"text":"full upstream body"}]}',
      },
    ],
  },
]

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('EndpointAuditPage', () => {
  it('uses a quality suite and keeps assertion details in a dialog', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/channel/quality/suites') {
        return { data: { success: true, message: '', data: suites } }
      }
      if (url === '/api/endpoint-audit/tests/3') {
        return { data: { success: true, message: '', data: runs[0] } }
      }
      throw new Error(`unexpected ${url}`)
    })
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: { ...runs[0], verdict: 'running', items: [] },
      },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <EndpointTestPanel />
      </QueryClientProvider>
    )

    expect(
      await screen.findByRole('combobox', { name: 'Quality suite' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /example.com/ })
    ).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Base URL'), 'https://example.com')
    await user.type(screen.getByLabelText('API key'), 'sk-test')
    await user.click(screen.getByRole('combobox', { name: 'Quality suite' }))
    await user.click(await screen.findByRole('option', { name: 'check' }))
    await user.click(
      screen.getByRole('button', { name: 'Start endpoint audit' })
    )
    expect(
      await screen.findByRole('button', { name: /example.com/ })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('clear tool uses applied')
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /example.com/ }))
    expect(
      screen.queryByText('clear tool uses applied')
    ).not.toBeInTheDocument()
    await user.click(
      await screen.findByRole('button', { name: /claude-opus-4-6/ })
    )
    expect(
      await screen.findByText('clear tool uses applied')
    ).toBeInTheDocument()
    expect(screen.getByText('Response body')).toBeInTheDocument()
    expect(screen.getByText(/full upstream body/)).toBeInTheDocument()
  })
})
