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
import { useTranslation } from 'react-i18next'

const example = `[
  {"key": "key-001", "base_url": "https://resource-001.openai.azure.com"},
  {"key": "key-002", "base_url": "https://resource-002.openai.azure.com"}
]`

export function AccountCredentialsHelp() {
  const { t } = useTranslation()
  return (
    <details className='text-muted-foreground text-sm'>
      <summary className='cursor-pointer'>
        {t('Use a different Base URL for each key')}
      </summary>
      <p className='mt-2'>
        {t(
          'Paste a JSON array of accounts into the key field. Each account has a key and an optional base_url; omitted URLs use the channel default. Choose key aggregation and Polling to cycle through accounts in order.'
        )}
      </p>
      <pre className='bg-muted mt-2 overflow-x-auto rounded-md p-3 text-xs'>
        <code>{example}</code>
      </pre>
    </details>
  )
}
