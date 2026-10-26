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
import { Code, Plus, Table, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type HeaderRow = {
  key: string
  name: string
  value: string
}

type ChannelQualityHeadersProps = {
  value: string
  disabled?: boolean
  onChange: (value: string) => void
}

export function ChannelQualityHeaders(props: ChannelQualityHeadersProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<HeaderRow[]>(
    () => headerRows(props.value) ?? []
  )
  const [jsonValue, setJsonValue] = useState(props.value)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const emitted = useRef(props.value)

  useEffect(() => {
    if (props.value === emitted.current) return
    emitted.current = props.value
    setJsonValue(props.value)
    const parsed = headerRows(props.value)
    if (parsed) {
      setRows(parsed)
      setJsonError(null)
    }
  }, [props.value])

  const emitRows = (next: HeaderRow[]) => {
    setRows(next)
    const encoded = JSON.stringify(rowsToHeaders(next), null, 2)
    emitted.current = encoded
    setJsonValue(encoded)
    setJsonError(null)
    props.onChange(encoded)
  }

  const switchMode = (next: string) => {
    if (next !== 'visual' && next !== 'json') return
    if (next === 'json') {
      const encoded = JSON.stringify(rowsToHeaders(rows), null, 2)
      setJsonValue(encoded)
      setJsonError(null)
      setMode('json')
      return
    }
    const parsed = headerRows(jsonValue)
    if (!parsed) {
      setJsonError(t('Header JSON must be an object'))
      return
    }
    setRows(parsed)
    setJsonError(null)
    setMode('visual')
  }

  return (
    <Tabs value={mode} onValueChange={switchMode}>
      <TabsList>
        <TabsTrigger value='visual'>
          <Table className='size-4' aria-hidden='true' />
          {t('Visual')}
        </TabsTrigger>
        <TabsTrigger value='json'>
          <Code className='size-4' aria-hidden='true' />
          {t('JSON')}
        </TabsTrigger>
      </TabsList>
      {jsonError ? (
        <Alert variant='destructive'>
          <AlertDescription>{jsonError}</AlertDescription>
        </Alert>
      ) : null}
      <TabsContent value='visual' className='flex flex-col gap-2'>
        {rows.length === 0 ? (
          <p className='text-muted-foreground rounded-lg border border-dashed px-3 py-6 text-center text-sm'>
            {t('No headers yet')}
          </p>
        ) : (
          rows.map((row) => (
            <div key={row.key} className='grid grid-cols-[1fr_1fr_auto] gap-2'>
              <Input
                value={row.name}
                disabled={props.disabled}
                aria-label={t('Header name')}
                placeholder={t('Header name')}
                onChange={(event) =>
                  emitRows(
                    rows.map((item) =>
                      item.key === row.key
                        ? { ...item, name: event.target.value }
                        : item
                    )
                  )
                }
              />
              <Input
                value={row.value}
                disabled={props.disabled}
                aria-label={t('Header value')}
                placeholder={t('Header value')}
                onChange={(event) =>
                  emitRows(
                    rows.map((item) =>
                      item.key === row.key
                        ? { ...item, value: event.target.value }
                        : item
                    )
                  )
                }
              />
              {props.disabled ? null : (
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  aria-label={t('Remove header')}
                  onClick={() =>
                    emitRows(rows.filter((item) => item.key !== row.key))
                  }
                >
                  <Trash2 className='size-4' aria-hidden='true' />
                </Button>
              )}
            </div>
          ))
        )}
        {props.disabled ? null : (
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() =>
              emitRows([...rows, { key: newHeaderKey(), name: '', value: '' }])
            }
          >
            <Plus className='size-4' aria-hidden='true' />
            {t('Add header')}
          </Button>
        )}
      </TabsContent>
      <TabsContent value='json'>
        <JsonCodeEditor
          value={jsonValue}
          disabled={props.disabled}
          ariaLabel={t('Headers')}
          heightClassName='h-36 max-h-36 min-h-36'
          className={jsonError ? 'border-destructive' : undefined}
          aria-invalid={Boolean(jsonError)}
          onChange={(value) => {
            setJsonValue(value)
            emitted.current = value
            props.onChange(value)
            const parsed = headerRows(value)
            if (!parsed) {
              setJsonError(t('Header JSON must be an object'))
              return
            }
            setRows(parsed)
            setJsonError(null)
          }}
        />
      </TabsContent>
    </Tabs>
  )
}

function headerRows(raw: string): HeaderRow[] | null {
  const trimmed = raw.trim()
  if (!trimmed) return []
  let value: unknown
  try {
    value = JSON.parse(trimmed)
  } catch {
    return null
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const rows: HeaderRow[] = []
  for (const [name, item] of Object.entries(value)) {
    if (typeof item !== 'string') return null
    rows.push({ key: newHeaderKey(), name, value: item })
  }
  return rows
}

function rowsToHeaders(rows: HeaderRow[]): Record<string, string> {
  const headers: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim()
    if (!name) continue
    headers[name] = row.value
  }
  return headers
}

function newHeaderKey(): string {
  return globalThis.crypto.randomUUID()
}
