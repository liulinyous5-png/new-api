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

import { ChannelQualityWebhook } from '../dialogs/channel-quality-webhook'

let client: QueryClient
const config = {
  enabled: true,
  url: 'https://example.com/quality',
  cooldown_seconds: 600,
  secret_configured: true,
}
function renderSettings() {
  render(
    <QueryClientProvider client={client}>
      <ChannelQualityWebhook />
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

it('preserves a saved secret when saving other settings', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: config },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  renderSettings()
  const cooldown = await screen.findByLabelText('Webhook cooldown (seconds)')
  const secret = screen.getByLabelText('Webhook Secret')
  expect(secret).toHaveValue('')
  expect(secret).toHaveAttribute('type', 'password')
  await user.clear(cooldown)
  await user.type(cooldown, '120')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/monitor/quality_webhook/config', {
      enabled: true,
      url: config.url,
      cooldown_seconds: 120,
    })
  )
})

it('clears the stored secret only after an explicit clear action', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: config },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  renderSettings()
  await user.click(
    await screen.findByRole('switch', { name: 'Clear saved Webhook secret' })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/monitor/quality_webhook/config',
      expect.objectContaining({ secret: '' })
    )
  )
})

it('rejects a non-HTTP destination before submitting', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: config },
  })
  const put = vi.spyOn(api, 'put')
  renderSettings()
  const url = await screen.findByLabelText('Webhook URL')
  await user.clear(url)
  await user.type(url, 'file:///tmp/receiver')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Webhook URL must be a valid HTTP or HTTPS URL'
  )
  expect(put).not.toHaveBeenCalled()
})

it('offers retry when configuration fails to load', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue({ data: { success: true, data: config } })
  renderSettings()
  await user.click(await screen.findByRole('button', { name: 'Retry' }))
  expect(await screen.findByLabelText('Webhook URL')).toHaveValue(config.url)
})
