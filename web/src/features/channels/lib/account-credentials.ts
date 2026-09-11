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
export interface AccountCredential {
  key: string
  base_url?: string
}

/** Parse account JSON or legacy key lines without exposing secrets in errors. */
export function parseAccountCredentials(input: string): AccountCredential[] {
  try {
    const trimmed = input.trim()
    if (!trimmed) return []
    let entries: unknown[]
    if (trimmed.startsWith('[')) {
      const parsed: unknown = JSON.parse(trimmed)
      if (!Array.isArray(parsed) || !parsed.length) throw new Error()
      entries = parsed
    } else {
      try {
        entries = [JSON.parse(trimmed) as unknown]
      } catch {
        entries = trimmed
          .split('\n')
          .filter((line) => line.trim())
          .map((line) => {
            if (line.trim().startsWith('{')) return JSON.parse(line) as unknown
            return { key: line.trim() }
          })
      }
    }
    return entries.map((entry) => {
      if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
        throw new Error()
      }
      const account = entry as Record<string, unknown>
      if (
        typeof account.key !== 'string' ||
        !account.key.trim() ||
        /[\r\n]/.test(account.key)
      ) {
        throw new Error()
      }
      const result: AccountCredential = { key: account.key.trim() }
      if (
        account.base_url !== undefined &&
        typeof account.base_url !== 'string'
      ) {
        throw new Error()
      }
      const baseURL = String(account.base_url || '')
        .trim()
        .replace(/\/+$/, '')
      if (baseURL) {
        const url = new URL(baseURL)
        if (
          !/^https?:\/\//.test(baseURL) ||
          !url.hostname ||
          url.username ||
          url.password ||
          url.search ||
          url.hash ||
          baseURL.includes('?') ||
          baseURL.includes('#')
        ) {
          throw new Error()
        }
        result.base_url = baseURL
      }
      return result
    })
  } catch {
    throw new Error('Invalid account list')
  }
}
