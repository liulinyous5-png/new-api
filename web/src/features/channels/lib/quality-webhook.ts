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
import z from 'zod'

import { api } from '@/lib/api'

export type QualityWebhookConfig = {
  enabled: boolean
  url: string
  cooldown_seconds: number
  secret_configured: boolean
}
export const qualityWebhookSchema = z
  .object({
    enabled: z.boolean(),
    url: z.string().max(2048, 'Webhook configuration is too long'),
    secret: z.string().max(4096, 'Webhook configuration is too long'),
    clear_secret: z.boolean(),
    cooldown_seconds: z
      .number({
        error: 'Webhook cooldown must be between 60 and 86400 seconds',
      })
      .int('Webhook cooldown must be between 60 and 86400 seconds')
      .min(60, 'Webhook cooldown must be between 60 and 86400 seconds')
      .max(86400, 'Webhook cooldown must be between 60 and 86400 seconds'),
  })
  .superRefine((value, ctx) => {
    if (!value.url.trim()) {
      if (value.enabled) {
        ctx.addIssue({
          code: 'custom',
          path: ['url'],
          message: 'Webhook URL is required',
        })
      }
      return
    }
    try {
      const parsed = new URL(value.url)
      if (
        !['http:', 'https:'].includes(parsed.protocol) ||
        parsed.username ||
        parsed.password ||
        parsed.hash
      ) {
        throw new Error('invalid URL')
      }
    } catch {
      ctx.addIssue({
        code: 'custom',
        path: ['url'],
        message: 'Webhook URL must be a valid HTTP or HTTPS URL',
      })
    }
  })
export type QualityWebhookForm = z.infer<typeof qualityWebhookSchema>

type Response<T> = { success: boolean; message?: string; data?: T }
export async function getQualityWebhookConfig(): Promise<
  Response<QualityWebhookConfig>
> {
  const result = await api.get('/api/monitor/quality_webhook/config')
  return result.data
}
export async function updateQualityWebhookConfig(
  config: Omit<QualityWebhookConfig, 'secret_configured'> & { secret?: string }
): Promise<Response<null>> {
  const result = await api.put('/api/monitor/quality_webhook/config', config)
  return result.data
}
