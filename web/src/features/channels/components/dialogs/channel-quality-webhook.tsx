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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SettingsSwitchField } from '@/features/system-settings/components/settings-form-layout'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getQualityWebhookConfig,
  qualityWebhookSchema,
  updateQualityWebhookConfig,
  type QualityWebhookForm,
} from '../../lib/quality-webhook'

export function ChannelQualityWebhook() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['quality-webhook-config'],
    queryFn: async () => {
      const response = await getQualityWebhookConfig()
      requireServerSuccess(response)
      if (!response.data) throw new Error('Missing webhook configuration')
      return response.data
    },
    refetchOnWindowFocus: false,
  })
  const form = useForm<QualityWebhookForm>({
    resolver: zodResolver(qualityWebhookSchema),
    defaultValues: {
      enabled: false,
      url: '',
      secret: '',
      clear_secret: false,
      cooldown_seconds: 600,
    },
  })
  useEffect(() => {
    if (query.data) {
      form.reset({
        enabled: query.data.enabled,
        url: query.data.url,
        cooldown_seconds: query.data.cooldown_seconds,
        secret: '',
        clear_secret: false,
      })
    }
  }, [query.data, form])
  const save = useMutation({
    mutationFn: async (values: QualityWebhookForm) => {
      const response = await updateQualityWebhookConfig({
        enabled: values.enabled,
        url: values.url.trim(),
        cooldown_seconds: values.cooldown_seconds,
        ...(values.clear_secret ? { secret: '' } : {}),
        ...(!values.clear_secret && values.secret !== ''
          ? { secret: values.secret }
          : {}),
      })
      requireServerSuccess(response)
    },
    onSuccess: async () => {
      toast.success(t('Saved successfully'))
      await client.invalidateQueries({ queryKey: ['quality-webhook-config'] })
    },
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }

  return (
    <form
      className='flex flex-col gap-4'
      onSubmit={form.handleSubmit((values) => save.mutate(values))}
    >
      <p className='text-muted-foreground text-sm'>
        {t(
          'Notify when a scheduled audit probe fails, including unexpected HTTP status codes and failed assertions. Errors, unknown results, unsupported checks and skipped checks do not trigger alerts.'
        )}
      </p>
      <Controller
        control={form.control}
        name='enabled'
        render={({ field }) => (
          <SettingsSwitchField
            controlId='quality-webhook-enabled'
            label={t('Enable quality Webhook alerts')}
            checked={field.value}
            onCheckedChange={field.onChange}
            disabled={save.isPending}
          />
        )}
      />
      <div className='space-y-1.5'>
        <Label htmlFor='quality-webhook-url'>{t('Webhook URL')}</Label>
        <Input
          id='quality-webhook-url'
          {...form.register('url')}
          disabled={save.isPending}
          aria-invalid={!!form.formState.errors.url}
        />
        {form.formState.errors.url ? (
          <p role='alert' className='text-destructive text-sm'>
            {t(form.formState.errors.url.message ?? 'Invalid parameters')}
          </p>
        ) : null}
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor='quality-webhook-secret'>{t('Webhook Secret')}</Label>
        <Input
          id='quality-webhook-secret'
          type='password'
          autoComplete='new-password'
          {...form.register('secret')}
          disabled={save.isPending}
          placeholder={
            query.data?.secret_configured
              ? t('Leave blank to keep the saved secret')
              : ''
          }
          aria-invalid={!!form.formState.errors.secret}
        />
        {form.formState.errors.secret ? (
          <p role='alert' className='text-destructive text-sm'>
            {t(form.formState.errors.secret.message ?? 'Invalid parameters')}
          </p>
        ) : null}
      </div>
      {query.data?.secret_configured ? (
        <Controller
          control={form.control}
          name='clear_secret'
          render={({ field }) => (
            <SettingsSwitchField
              controlId='quality-webhook-clear-secret'
              label={t('Clear saved Webhook secret')}
              checked={field.value}
              onCheckedChange={field.onChange}
              disabled={save.isPending}
            />
          )}
        />
      ) : null}
      <div className='space-y-1.5'>
        <Label htmlFor='quality-webhook-cooldown'>
          {t('Webhook cooldown (seconds)')}
        </Label>
        <Input
          id='quality-webhook-cooldown'
          type='number'
          min={60}
          max={86400}
          step={1}
          {...form.register('cooldown_seconds', { valueAsNumber: true })}
          disabled={save.isPending}
          aria-invalid={!!form.formState.errors.cooldown_seconds}
        />
        {form.formState.errors.cooldown_seconds ? (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              form.formState.errors.cooldown_seconds.message ??
                'Invalid parameters'
            )}
          </p>
        ) : null}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Duplicate channel alerts are suppressed during the cooldown. No response bodies or credentials are sent. Failed deliveries are retried once and can retry on the next failed scan.'
        )}
      </p>
      <Button
        type='submit'
        className='self-end'
        disabled={save.isPending || !form.formState.isDirty}
      >
        {t('Save')}
      </Button>
    </form>
  )
}
