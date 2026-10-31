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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { ChannelQualityProbe, ChannelQualitySuite } from '../../api'

export function ChannelQualitySuiteList(props: {
  suites: ChannelQualitySuite[]
  probes: ChannelQualityProbe[]
  canWrite: boolean
  onEdit: (suite: ChannelQualitySuite) => void
}) {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const probeNames = useMemo(
    () => new Map(props.probes.map((probe) => [probe.id, probe.name])),
    [props.probes]
  )
  const keyword = search.trim().toLowerCase()
  const suites = props.suites.filter(
    (suite) =>
      !keyword ||
      suite.name.toLowerCase().includes(keyword) ||
      suite.items.some(
        (item) =>
          item.model.toLowerCase().includes(keyword) ||
          probeNames.get(item.probe_id)?.toLowerCase().includes(keyword)
      )
  )

  return (
    <div className='flex min-w-0 flex-col gap-3'>
      <Input
        aria-label={t('Search suites')}
        placeholder={t('Search suites')}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        className='max-w-sm'
      />
      {suites.length === 0 ? (
        <EmptyState title={t('No results found')} />
      ) : (
        <div className='grid min-w-0 gap-3 xl:grid-cols-2'>
          {suites.map((suite) => (
            <Card key={suite.id} size='sm' className='min-w-0'>
              <CardHeader className='gap-3'>
                <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
                  <CardTitle className='min-w-0 break-words'>
                    <h3>{suite.name}</h3>
                  </CardTitle>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    aria-label={
                      props.canWrite
                        ? t('Edit {{title}}', { title: suite.name })
                        : `${t('View details')}: ${suite.name}`
                    }
                    onClick={() => props.onEdit(suite)}
                  >
                    {props.canWrite ? t('Edit') : t('View details')}
                  </Button>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Badge variant='secondary'>
                    {t('Probes')}:{' '}
                    {formatNumber(
                      new Set(suite.items.map((item) => item.probe_id)).size,
                      locale
                    )}
                  </Badge>
                  <Badge variant='outline'>
                    {t('Models')}:{' '}
                    {formatNumber(
                      new Set(suite.items.map((item) => item.model)).size,
                      locale
                    )}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent>
                <ul
                  tabIndex={0}
                  aria-label={t('Probes')}
                  className='max-h-48 space-y-2 overflow-y-auto text-sm'
                >
                  {suite.items.map((item) => (
                    <li
                      key={item.id}
                      className='grid min-w-0 gap-1 sm:grid-cols-2'
                    >
                      <span className='break-words'>
                        {probeNames.get(item.probe_id) ?? item.probe_id}
                      </span>
                      <span className='text-muted-foreground font-mono text-xs break-all'>
                        {item.model}
                      </span>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
