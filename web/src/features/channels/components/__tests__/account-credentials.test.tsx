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
import { fireEvent, render, screen } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { AccountCredentialsField } from '../account-credentials-field'
import { AccountCredentialsHelp } from '../account-credentials-help'
import { MultiKeyTableRowActions } from '../dialogs/multi-key-table-row-actions'

describe('account credentials UI', () => {
  test('reveals a valid example with a URL for each key', () => {
    render(<AccountCredentialsHelp />)
    fireEvent.click(screen.getByText('Use a different Base URL for each key'))
    const example = screen.getByText(/"key-001"/)
    expect(example).toBeVisible()
    expect(JSON.parse(example.textContent || '')).toEqual([
      { key: 'key-001', base_url: 'https://resource-001.openai.azure.com' },
      { key: 'key-002', base_url: 'https://resource-002.openai.azure.com' },
    ])
  })
  test('can test a disabled account without enabling it and prevents repeated tests while running', () => {
    const onTest = vi.fn()
    const onAction = vi.fn()
    const view = render(
      <MultiKeyTableRowActions
        keyIndex={1}
        status={2}
        canDelete
        onTest={onTest}
        onAction={onAction}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: /^Test$/ }))
    expect(onTest).toHaveBeenCalledOnce()
    expect(onAction).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Enable' })).toBeEnabled()
    view.rerender(
      <MultiKeyTableRowActions
        keyIndex={1}
        status={2}
        canDelete
        testing
        onTest={onTest}
        onAction={onAction}
      />
    )
    expect(screen.getByRole('button', { name: 'Testing...' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Testing...' }))
    expect(onTest).toHaveBeenCalledOnce()
  })
})

function AccountCredentialsFormFixture(props: { disabled?: boolean }) {
  const form = useForm<ChannelFormValues>({
    defaultValues: CHANNEL_FORM_DEFAULT_VALUES,
  })
  return (
    <Form {...form}>
      <AccountCredentialsField
        control={form.control}
        disabled={props.disabled ?? false}
      />
    </Form>
  )
}

describe('account mode opt-in', () => {
  test('is off by default and changes only when explicitly enabled', () => {
    render(<AccountCredentialsFormFixture />)
    const toggle = screen.getByRole('switch', {
      name: 'Use a different Base URL for each key',
    })
    expect(toggle).not.toBeChecked()
    fireEvent.click(toggle)
    expect(toggle).toBeChecked()
    fireEvent.click(toggle)
    expect(toggle).not.toBeChecked()
  })
  test('cannot be enabled without sensitive configuration permission', () => {
    render(<AccountCredentialsFormFixture disabled />)
    const toggle = screen.getByRole('switch', {
      name: 'Use a different Base URL for each key',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(toggle)
    expect(toggle).not.toBeChecked()
  })
})
