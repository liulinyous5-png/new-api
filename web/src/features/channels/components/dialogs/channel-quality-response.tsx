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
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Download } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getChannelQualityResponse, type ChannelQualityRun } from '../../api'
import { channelsQueryKeys } from '../../lib'

export function ChannelQualityResponse(props: { run: ChannelQualityRun }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [formatted, setFormatted] = useState(false)
  const responseQuery = useQuery({
    queryKey: [
      ...channelsQueryKeys.all,
      'quality',
      'response',
      props.run.channel_id,
      props.run.id,
    ],
    queryFn: async () => {
      const response = await getChannelQualityResponse(
        props.run.channel_id,
        props.run.id
      )
      requireServerSuccess(response)
      return response.data ?? null
    },
    enabled: open && props.run.verdict !== 'running',
    gcTime: 60_000,
    staleTime: Infinity,
  })
  const response = responseQuery.data
  const body = useMemo(() => {
    if (!response) return null
    const bytes = Uint8Array.from(atob(response.body_base64), (char) =>
      char.charCodeAt(0)
    )
    const text = new TextDecoder('utf-8', { ignoreBOM: true }).decode(bytes)
    // Bound rendering and pretty-printing work; copy and download retain all bytes.
    const previewLimited = text.length > 65536
    return { text, preview: text.slice(0, 65536), previewLimited }
  }, [response])

  let content
  if (props.run.verdict === 'running') {
    content = (
      <p className='text-muted-foreground text-sm'>
        {t('Response is not available yet')}
      </p>
    )
  } else if (responseQuery.isPending) {
    content = <LoadingState />
  } else if (responseQuery.isError) {
    content = (
      <ErrorState
        onRetry={() => {
          void responseQuery.refetch()
        }}
      />
    )
  } else if (!response || !body) {
    content = (
      <p className='text-muted-foreground text-sm'>
        {t('No saved response for this audit')}
      </p>
    )
  } else {
    content = (
      <div className='flex min-w-0 flex-col gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-sm'>HTTP {response.status_code}</span>
          <CopyButton value={body.text} size='sm' variant='outline'>
            {t('Copy')}
          </CopyButton>
          <Button
            role='link'
            size='sm'
            variant='outline'
            render={
              <a
                href={`data:application/octet-stream;base64,${response.body_base64}`}
                download={`quality-audit-${props.run.id}${response.truncated || response.incomplete ? '-partial' : ''}.txt`}
              />
            }
          >
            <Download aria-hidden='true' />
            {t('Download')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            aria-pressed={formatted}
            disabled={response.formatted_body == null}
            onClick={() => setFormatted((value) => !value)}
          >
            {t('Format JSON')}
          </Button>
        </div>
        {response.truncated ? (
          <p role='status' className='text-destructive text-sm'>
            {t('Response exceeded 1 MiB; only the first 1 MiB was saved')}
          </p>
        ) : null}
        {response.incomplete ? (
          <p role='status' className='text-destructive text-sm'>
            {t('Response reading was interrupted; saved content is incomplete')}
          </p>
        ) : null}
        {body.previewLimited ? (
          <p className='text-muted-foreground text-sm'>
            {t(
              'Preview is limited; copy or download to view all saved content'
            )}
          </p>
        ) : null}
        <pre
          tabIndex={0}
          aria-label={t('Response body')}
          className='bg-muted max-h-96 overflow-auto rounded-md p-3 font-mono text-xs whitespace-pre'
        >
          {formatted && response.formatted_body != null
            ? response.formatted_body
            : body.preview}
        </pre>
      </div>
    )
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger
        render={
          <Button
            type='button'
            variant='ghost'
            className='w-full justify-between'
          />
        }
      >
        {t('Full response')}
        <ChevronDown aria-hidden='true' className={open ? 'rotate-180' : ''} />
      </CollapsibleTrigger>
      <CollapsibleContent className='min-w-0 p-3 pt-0'>
        {open ? content : null}
      </CollapsibleContent>
    </Collapsible>
  )
}
