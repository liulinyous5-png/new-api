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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { listChannelQualitySuites } from '@/features/channels/api'
import { channelsQueryKeys } from '@/features/channels/lib/channel-actions'
import {
  getServerErrorMessage,
  requireServerSuccess,
  translateServerText,
} from '@/lib/server-error-message'
import {
  endpointAuditKeys,
  getEndpointRun,
  startEndpointAudit,
  type EndpointRun,
  type EndpointRunItem,
} from './api'
import { EndpointVerdict } from './verdict'

export function EndpointTestPanel() {
  const { t } = useTranslation()
  const [baseURL, setBaseURL] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [suiteId, setSuiteId] = useState('')
  const [activeId, setActiveId] = useState<number | null>(null)
  const [selected, setSelected] = useState<EndpointRun | null>(null)
  const [item, setItem] = useState<EndpointRunItem | null>(null)

  const suitesQuery = useQuery({
    queryKey: channelsQueryKeys.qualitySuites(),
    queryFn: async () => {
      const response = await listChannelQualitySuites()
      requireServerSuccess(response)
      return response.data ?? []
    },
  })
  const activeQuery = useQuery({
    queryKey: endpointAuditKeys.run(activeId ?? 0),
    queryFn: () => getEndpointRun(activeId ?? 0),
    enabled: activeId != null,
    refetchInterval: (query) =>
      query.state.data?.verdict === 'running' ? 1500 : false,
  })

  const start = useMutation({
    mutationFn: () =>
      startEndpointAudit({
        base_url: baseURL.trim(),
        api_key: apiKey,
        suite_id: Number(suiteId),
      }),
    onSuccess: (run) => {
      setApiKey('')
      setActiveId(run.id)
      setSelected(null)
      setItem(null)
    },
    onError: (error) =>
      toast.error(translateServerText(t, getServerErrorMessage(error))),
  })

  const suites = suitesQuery.data ?? []
  const active = activeQuery.data
  const running = start.isPending || active?.verdict === 'running'

  return (
    <div className='flex max-w-3xl flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Send a saved suite to an HTTP or HTTPS endpoint. The API key is used once and is not stored.'
        )}
      </p>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='endpoint-audit-url'>{t('Base URL')}</Label>
        <Input
          id='endpoint-audit-url'
          value={baseURL}
          autoComplete='off'
          spellCheck={false}
          placeholder={t('https://example.com')}
          onChange={(event) => setBaseURL(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='endpoint-audit-key'>{t('API key')}</Label>
        <Input
          id='endpoint-audit-key'
          type='password'
          value={apiKey}
          autoComplete='new-password'
          spellCheck={false}
          onChange={(event) => setApiKey(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1.5'>
        <Label htmlFor='endpoint-audit-suite'>{t('Quality suite')}</Label>
        <Select
          items={suites.map((suite) => ({
            value: String(suite.id),
            label: suite.name,
          }))}
          value={suiteId || undefined}
          onValueChange={(value) => setSuiteId(value ?? '')}
        >
          <SelectTrigger id='endpoint-audit-suite' className='w-full'>
            <SelectValue placeholder={t('Select a quality suite')} />
          </SelectTrigger>
          <SelectContent>
            {suites.map((suite) => (
              <SelectItem key={suite.id} value={String(suite.id)}>
                {suite.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {suitesQuery.isSuccess && suites.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No quality suites yet')}
          </p>
        ) : null}
      </div>
      <Button
        type='button'
        className='self-start'
        disabled={start.isPending || running || !suiteId}
        onClick={() => {
          if (!baseURL.trim()) {
            toast.error(t('only http and https endpoints can be audited'))
            return
          }
          if (!apiKey.trim()) {
            toast.error(t('api key is required'))
            return
          }
          start.mutate()
        }}
      >
        {t('Start endpoint audit')}
      </Button>
      {active ? (
        <button
          type='button'
          className='hover:bg-muted flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left'
          onClick={() => {
            setSelected(active)
            setItem(null)
          }}
        >
          <span className='min-w-0'>
            <span className='block truncate text-sm'>{active.suite_name}</span>
            <span className='text-muted-foreground block truncate text-xs'>
              {active.base_host}
            </span>
          </span>
          <EndpointVerdict verdict={active.verdict} />
        </button>
      ) : null}
      <Dialog
        open={selected != null}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null)
            setItem(null)
          }
        }}
        title={selected?.suite_name || t('Endpoint audit')}
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 640px)'
      >
        {selected && active ? (
          <RunDetails run={active} item={item} onSelect={setItem} />
        ) : null}
      </Dialog>
    </div>
  )
}

function RunDetails(props: {
  run: EndpointRun
  item: EndpointRunItem | null
  onSelect: (item: EndpointRunItem | null) => void
}) {
  const { t } = useTranslation()
  const run = props.run
  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center gap-2'>
        <EndpointVerdict verdict={run.verdict} />
        <span className='text-muted-foreground truncate text-xs'>
          {run.base_host}
        </span>
      </div>
      {run.error ? (
        <p className='text-destructive text-sm'>
          {translateServerText(t, run.error)}
        </p>
      ) : null}
      <div className='overflow-hidden rounded-lg border'>
        {(run.items ?? []).map((entry) => (
          <button
            key={`${entry.probe_name}-${entry.model}`}
            type='button'
            className='hover:bg-muted flex w-full items-center justify-between gap-3 border-b px-3 py-2 text-left last:border-b-0'
            onClick={() => props.onSelect(entry)}
          >
            <span className='min-w-0'>
              <span className='block truncate text-sm'>{entry.probe_name}</span>
              <span className='text-muted-foreground block truncate text-xs'>
                {entry.model}
              </span>
            </span>
            <EndpointVerdict verdict={entry.verdict} />
          </button>
        ))}
      </div>
      {props.item ? <ItemDetails item={props.item} /> : null}
    </div>
  )
}

function ItemDetails(props: { item: EndpointRunItem }) {
  const { t } = useTranslation()
  const item = props.item
  return (
    <div className='flex flex-col gap-2 rounded-lg border p-3'>
      {item.status ? (
        <p className='text-sm'>
          {t('Status code')}: {item.status}
        </p>
      ) : null}
      {item.error ? (
        <p className='text-destructive text-sm'>
          {translateServerText(t, item.error)}
        </p>
      ) : null}
      {item.assertions?.map((assertion) => (
        <div
          key={`${assertion.name}-${assertion.expr ?? ''}`}
          className='flex items-center justify-between gap-3'
        >
          <span className='min-w-0'>
            <span className='block truncate text-sm'>{assertion.name}</span>
            {assertion.error ? (
              <span className='text-muted-foreground block truncate text-xs'>
                {translateServerText(t, assertion.error)}
              </span>
            ) : null}
          </span>
          <EndpointVerdict verdict={assertion.passed ? 'passed' : 'failed'} />
        </div>
      ))}
      {item.response ? (
        <ResponseBody label={t('Response body')} value={item.response} />
      ) : null}
      {item.applied_edits != null ? (
        <JsonBlock label={t('Applied edits')} value={item.applied_edits} />
      ) : null}
      {item.usage != null ? (
        <JsonBlock label={t('Usage')} value={item.usage} />
      ) : null}
    </div>
  )
}

function ResponseBody(props: { label: string; value: string }) {
  let text = props.value
  try {
    text = JSON.stringify(JSON.parse(props.value), null, 2)
  } catch {
    text = props.value
  }
  return (
    <div className='flex flex-col gap-1'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <pre className='bg-muted max-h-96 overflow-auto rounded-md p-2 text-xs break-all whitespace-pre-wrap'>
        {text}
      </pre>
    </div>
  )
}

function JsonBlock(props: { label: string; value: unknown }) {
  return (
    <div className='flex flex-col gap-1'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <pre className='bg-muted max-h-48 overflow-auto rounded-md p-2 text-xs'>
        {JSON.stringify(props.value, null, 2)}
      </pre>
    </div>
  )
}

