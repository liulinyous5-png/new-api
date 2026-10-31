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
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EndpointTestPanel } from '@/features/endpoint-audit/test-panel'
import { useDebounce } from '@/hooks/use-debounce'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  getChannel,
  getChannels,
  listLatestChannelQuality,
  searchChannels,
} from './api'
import { ChannelQualityPanel } from './components/dialogs/channel-quality-panel'
import { ChannelQualityProbes } from './components/dialogs/channel-quality-probes'
import { ChannelQualitySuites } from './components/dialogs/channel-quality-suites'
import { ChannelQualityVerdictBadge } from './components/dialogs/channel-quality-verdict'
import { QUALITY_AUDIT_CHANNEL_TYPES } from './constants'
import { channelsQueryKeys, getChannelTypeLabel } from './lib'
import type { Channel } from './types'

type ChannelQualityPageProps = {
  channelId?: number
  initialTab?: 'suites' | 'probes' | 'records' | 'endpoint'
  onSelectChannel: (channelId: number) => void
}

export function ChannelQualityPage(props: ChannelQualityPageProps) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const [keyword, setKeyword] = useState('')
  const [activeTab, setActiveTab] = useState<string>(
    props.initialTab ?? 'suites'
  )
  const trimmedKeyword = useDebounce(keyword.trim(), 300)
  const channelsQuery = useQuery({
    enabled: activeTab === 'records',
    queryKey: [...channelsQueryKeys.lists(), 'quality-page', trimmedKeyword],
    queryFn: async () => {
      const pages = await Promise.all(
        QUALITY_AUDIT_CHANNEL_TYPES.map(async (type) => {
          const response = trimmedKeyword
            ? await searchChannels({
                keyword: trimmedKeyword,
                p: 1,
                page_size: 50,
                type,
              })
            : await getChannels({ p: 1, page_size: 50, type })
          requireServerSuccess(response)
          const items = (response.data?.items ?? []).filter(
            (item) => item.type === type
          )
          return {
            items,
            total: response.data?.total ?? items.length,
          }
        })
      )
      return {
        items: pages.flatMap((page) => page.items),
        total: pages.reduce((sum, page) => sum + page.total, 0),
      }
    },
  })
  const latestQuery = useQuery({
    enabled: activeTab === 'records',
    queryKey: channelsQueryKeys.qualityLatest(),
    queryFn: async () => {
      const response = await listLatestChannelQuality()
      requireServerSuccess(response)
      return response.data ?? []
    },
    refetchInterval: (query) =>
      query.state.data?.some((item) => item.verdict === 'running')
        ? 1500
        : false,
  })
  const items = channelsQuery.data?.items ?? []
  const listed = items.find((item) => item.id === props.channelId)
  const detailQuery = useQuery({
    queryKey: channelsQueryKeys.detail(props.channelId ?? 0),
    queryFn: async () => {
      const response = await getChannel(props.channelId ?? 0)
      requireServerSuccess(response)
      return response.data ?? null
    },
    enabled:
      activeTab === 'records' &&
      Boolean(props.channelId) &&
      !listed &&
      !channelsQuery.isLoading,
  })
  const fetched = detailQuery.data
  const selected =
    listed ??
    (fetched &&
    (QUALITY_AUDIT_CHANNEL_TYPES as readonly number[]).includes(fetched.type)
      ? fetched
      : null)
  const total = channelsQuery.data?.total ?? items.length

  const verdicts = useMemo(
    () =>
      new Map(
        (latestQuery.data ?? []).map((item) => [item.channel_id, item.verdict])
      ),
    [latestQuery.data]
  )
  let channelList: ReactNode
  if (channelsQuery.isPending) {
    channelList = <LoadingState />
  } else if (channelsQuery.isError) {
    channelList = (
      <ErrorState
        onRetry={() => {
          void channelsQuery.refetch()
        }}
      />
    )
  } else if (items.length === 0) {
    channelList = (
      <p className='text-muted-foreground p-3 text-sm'>
        {t('No channels found')}
      </p>
    )
  } else {
    channelList = items.map((item) => (
      <ChannelQualityListItem
        key={item.id}
        channel={item}
        verdict={verdicts.get(item.id)}
        selected={item.id === props.channelId}
        onSelect={props.onSelectChannel}
      />
    ))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Quality audit')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs
          value={activeTab}
          onValueChange={(value) => setActiveTab(String(value))}
          className='gap-4'
        >
          <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='suites'>{t('Manage suites')}</TabsTrigger>
            <TabsTrigger value='probes'>{t('Manage probes')}</TabsTrigger>
            <TabsTrigger value='records'>{t('Audit records')}</TabsTrigger>
            <TabsTrigger value='endpoint'>{t('Endpoint audit')}</TabsTrigger>
          </TabsList>
          <TabsContent value='records' className='flex flex-col gap-4'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Run a saved request against this channel and check the response.'
              )}
            </p>
            <div className='grid gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]'>
              <div className='flex min-w-0 flex-col gap-2'>
                <Input
                  value={keyword}
                  placeholder={t('Search channels')}
                  aria-label={t('Search channels')}
                  onChange={(event) => setKeyword(event.target.value)}
                />
                <div className='max-h-[32rem] overflow-y-auto rounded-lg border'>
                  {channelList}
                </div>
                {total > items.length ? (
                  <p className='text-muted-foreground text-xs'>
                    {t('Search to find more channels')}
                  </p>
                ) : null}
              </div>
              <ChannelQualityPanel channel={selected} />
            </div>
          </TabsContent>
          <TabsContent value='probes'>
            <ChannelQualityProbes canWrite={canWrite} />
          </TabsContent>
          <TabsContent value='suites' className='flex flex-col gap-3'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'A suite runs each probe against one model. Models the channel does not serve are skipped.'
              )}
            </p>
            <ChannelQualitySuites canWrite={canWrite} />
          </TabsContent>
          <TabsContent value='endpoint'>
            <EndpointTestPanel />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function ChannelQualityListItem(props: {
  channel: Channel
  verdict?: string
  selected: boolean
  onSelect: (channelId: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      variant='ghost'
      aria-pressed={props.selected}
      className={cn(
        'h-auto w-full justify-between rounded-none px-3 py-2',
        props.selected && 'bg-muted'
      )}
      onClick={() => props.onSelect(props.channel.id)}
    >
      <span className='flex min-w-0 flex-col items-start'>
        <span className='truncate'>{props.channel.name}</span>
        <span className='text-muted-foreground text-xs font-normal'>
          {t(getChannelTypeLabel(props.channel.type))}
        </span>
      </span>
      {props.verdict ? (
        <ChannelQualityVerdictBadge verdict={props.verdict} />
      ) : null}
    </Button>
  )
}
