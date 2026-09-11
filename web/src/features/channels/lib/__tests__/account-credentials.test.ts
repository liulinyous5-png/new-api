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
import { describe, expect, test } from 'vitest'

import { parseAccountCredentials } from '../account-credentials'
import { CHANNEL_FORM_DEFAULT_VALUES, channelFormSchema } from '../channel-form'

describe('account credentials', () => {
  test('keeps repeated secrets bound to their different endpoints', () => {
    expect(
      parseAccountCredentials(
        '[{"key":"same","base_url":"https://a.example/"},{"base_url":"https://b.example","key":"same"}]'
      )
    ).toEqual([
      { key: 'same', base_url: 'https://a.example' },
      { key: 'same', base_url: 'https://b.example' },
    ])
  })
  test('supports legacy keys mixed with JSON account lines', () => {
    expect(
      parseAccountCredentials(
        'legacy\n{"key":"account","base_url":"https://a.example"}'
      )
    ).toEqual([
      { key: 'legacy' },
      { key: 'account', base_url: 'https://a.example' },
    ])
  })
  test.each([
    '[]',
    '[{"base_url":"https://a.example"}]',
    '[{"key":" "}]',
    '[{"key":"secret","base_url":"file:///tmp/a"}]',
    '[{"key":"secret","base_url":"https://user:password@a.example"}]',
    '[{"key":"secret","base_url":"https://a.example?secret"}]',
    '[{"key":"secret\\nheader"}]',
    '[{"key":"secret"}',
  ])('rejects invalid accounts without exposing secrets: %s', (input) => {
    expect(() => parseAccountCredentials(input)).toThrow('Invalid account list')
  })
  test('allows Azure accounts with individual URLs and no shared URL', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Azure accounts',
      account_credentials: true,
      type: 3,
      models: 'gpt-4o',
      other: '2024-10-21',
      base_url: '',
      multi_key_mode: 'multi_to_single',
      key: '[{"key":"one","base_url":"https://a.example"}]',
    })
    expect(result.success).toBe(true)
  })
})

describe('legacy channel forms', () => {
  test('does not parse legacy key content unless independent URLs are enabled', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Legacy OpenAI',
      type: 1,
      key: '{legacy-provider-credential}',
      models: 'gpt-4o',
    }
    expect(channelFormSchema.safeParse(values).success).toBe(true)
    expect(
      channelFormSchema.safeParse({ ...values, account_credentials: true })
        .success
    ).toBe(false)
  })
  test('retains the existing Azure default URL requirement when the option is off', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Legacy Azure',
      type: 3,
      key: '',
      models: 'gpt-4o',
      other: '2024-10-21',
      is_editing: true,
      base_url: '',
    }
    expect(channelFormSchema.safeParse(values).success).toBe(false)
    expect(
      channelFormSchema.safeParse({
        ...values,
        base_url: 'https://legacy.example',
      }).success
    ).toBe(true)
  })
})
