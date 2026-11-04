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
  within,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ChannelHealthSection } from '../channel-health-section'
import { defaultRequestPolicySettings } from '../defaults'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState({ auth: originalAuth })
})

function show() {
  render(
    <QueryClientProvider client={client}>
      <ChannelHealthSection defaultValues={defaultRequestPolicySettings} />
    </QueryClientProvider>
  )
}

function formItemOf(control: HTMLElement) {
  return control.closest('[data-slot=form-item]')
}

describe('channel health layout', () => {
  it('nests only the test mode and interval under the scheduled channel tests switch', () => {
    show()
    const options = screen.getByRole('group', {
      name: 'Scheduled test options',
    })
    expect(within(options).getByRole('combobox')).toBeVisible()
    expect(
      within(options).getByRole('spinbutton', {
        name: 'Test interval (minutes)',
      })
    ).toBeVisible()
    expect(
      within(options).queryByRole('spinbutton', {
        name: 'Channel test concurrency',
      })
    ).not.toBeInTheDocument()
    expect(within(options).queryByRole('switch')).not.toBeInTheDocument()
  })

  it('marks every switch row as full width so it never shares a grid row with an input', () => {
    show()
    for (const name of [
      'Scheduled channel tests',
      'Re-enable on success',
      'Disable on failure',
    ]) {
      expect(formItemOf(screen.getByRole('switch', { name }))).toHaveAttribute(
        'data-settings-form-span',
        'full'
      )
    }
  })

  it('lays out the auto-disable switch, inputs and keyword list in one shared grid', () => {
    show()
    const grid = formItemOf(
      screen.getByRole('switch', { name: 'Disable on failure' })
    )?.parentElement
    expect(grid).toHaveAttribute('data-settings-form-span', 'full')
    for (const control of [
      screen.getByRole('spinbutton', {
        name: 'Health check timeout threshold (seconds)',
      }),
      screen.getByRole('textbox', { name: 'Auto-disable status codes' }),
      screen.getByRole('textbox', { name: 'Failure keywords' }),
    ]) {
      expect(formItemOf(control)?.parentElement).toBe(grid)
    }
  })
})

it('opens Webhook settings under scheduled quality scan and saves independently', async () => {
  const user = userEvent.setup()
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        enabled: true,
        url: 'https://example.com/quality',
        cooldown_seconds: 600,
        secret_configured: false,
      },
    },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const patch = vi.spyOn(api, 'patch')
  show()
  expect(get).not.toHaveBeenCalled()
  const scan = screen.getByRole('group', { name: 'Scheduled quality scan' })
  await user.click(
    within(scan).getByRole('button', { name: 'Quality Webhook alerts' })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Quality Webhook alerts',
  })
  const cooldown = await within(dialog).findByLabelText(
    'Webhook cooldown (seconds)'
  )
  await user.clear(cooldown)
  await user.type(cooldown, '120')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/monitor/quality_webhook/config', {
      enabled: true,
      url: 'https://example.com/quality',
      cooldown_seconds: 120,
    })
  )
  expect(patch).not.toHaveBeenCalled()
})

it('keeps quality auto-disable off by default and saves it independently of alerts and general disable rules', async () => {
  const user = userEvent.setup()
  const actionsContainer = document.createElement('div')
  render(<div />).container.appendChild(actionsContainer)
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true, data: { options: {} } } })
  const put = vi.spyOn(api, 'put')
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <ChannelHealthSection defaultValues={defaultRequestPolicySettings} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const scan = screen.getByRole('group', { name: 'Scheduled quality scan' })
  const toggle = within(scan).getByRole('switch', {
    name: 'Disable on quality failure',
  })
  expect(toggle).not.toBeChecked()
  await user.click(toggle)
  expect(toggle).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Disable on failure' })
  ).not.toBeChecked()
  expect(
    screen.getByText(
      'Scheduled quality scans can disable channels independently of the general auto-disable rules.'
    )
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith('/api/option/request_policy', {
      options: { 'monitor_setting.quality_disable_on_failure': 'true' },
    })
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  )
  await user.click(toggle)
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(patch).toHaveBeenLastCalledWith('/api/option/request_policy', {
      options: { 'monitor_setting.quality_disable_on_failure': 'false' },
    })
  )
  expect(put).not.toHaveBeenCalled()
})

it('defaults 429 retries to zero, rejects invalid counts and saves the allowed maximum', async () => {
  const user = userEvent.setup()
  const actionsContainer = document.createElement('div')
  render(<div />).container.appendChild(actionsContainer)
  const patch = vi.spyOn(api, 'patch').mockResolvedValue({
    data: { success: true, data: { options: {} } },
  })
  render(
    <QueryClientProvider client={client}>
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <ChannelHealthSection defaultValues={defaultRequestPolicySettings} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const scan = screen.getByRole('group', { name: 'Scheduled quality scan' })
  const count = within(scan).getByRole('spinbutton', {
    name: '429 retry count',
  })
  expect(count).toHaveValue(0)
  expect(count).toHaveAttribute('min', '0')
  expect(count).toHaveAttribute('max', '10')
  const form = count.closest('form')
  if (!form) throw new Error('Settings form not found')
  for (const value of ['-1', '11', '1.5']) {
    fireEvent.change(count, { target: { value } })
    fireEvent.submit(form)
    expect(
      await screen.findByText(
        'quality 429 retries must be an integer between 0 and 10'
      )
    ).toBeVisible()
    expect(patch).not.toHaveBeenCalled()
  }
  fireEvent.change(count, { target: { value: '10' } })
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(patch).toHaveBeenCalledWith('/api/option/request_policy', {
      options: { 'monitor_setting.quality_429_retries': '10' },
    })
  )
  expect(
    within(scan).getByRole('switch', { name: 'Disable on quality failure' })
  ).not.toBeChecked()
})
