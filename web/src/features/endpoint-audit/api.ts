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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type EndpointAssertionResult = {
  name: string
  expr?: string
  passed: boolean
  error?: string
}

export type EndpointRunItem = {
  probe_name: string
  model: string
  verdict: string
  status?: number
  error?: string
  response?: string
  assertions?: EndpointAssertionResult[]
  applied_edits?: unknown
  usage?: unknown
}

export type EndpointRun = {
  id: number
  suite_id: number
  suite_name: string
  base_host: string
  verdict: string
  error: string
  items: EndpointRunItem[]
  created_at: number
  finished_at: number
}

type Response<T> = { success: boolean; message: string; data: T }

async function read<T>(response: { data: Response<T> }): Promise<T> {
  requireServerSuccess(response.data)
  return response.data.data
}

export const endpointAuditKeys = {
  run: (id: number) => ['endpoint-audit', 'run', id] as const,
}

export async function getEndpointRun(id: number) {
  return read<EndpointRun>(await api.get(`/api/endpoint-audit/tests/${id}`))
}

export async function startEndpointAudit(body: {
  base_url: string
  api_key: string
  suite_id: number
}) {
  return read<EndpointRun>(await api.post('/api/endpoint-audit/tests', body))
}
