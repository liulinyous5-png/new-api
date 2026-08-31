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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2, Plus, Trash2 } from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { CHANNEL_STATUS, CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'
import { getChannel, getChannels } from '@/features/channels/api'
import { getGroups } from '@/features/users/api'
import { SettingsSection } from '../components/settings-section'
import {
  deleteGroupChannelFallback,
  getGroupChannelFallbacks,
  upsertGroupChannelFallback,
} from '../api'
import type { GroupChannelFallbackItem } from '../types'

type ChannelItem = {
  id: number
  name: string
  type: number
  group: string
  status: number
}

const DEFAULT_FORM = {
  id: 0,
  group_name: '',
  channel_type: '',
  fallback_channel_id: '',
  enabled: true,
  remark: '',
}

function formatChannelLabel(channel: {
  name: string
  group?: string | null
}) {
  return `${channel.name} (${String(channel.group || '').trim() || '-'})`
}

export function GroupChannelFallbackSection() {
  const { t } = useTranslation()
  const [form, setForm] = useState(DEFAULT_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [deletingKey, setDeletingKey] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState(0)

  const groupsQuery = useQuery({
    queryKey: ['group-fallback-groups'],
    queryFn: async () => {
      const res = await getGroups()
      if (!res.success) {
        throw new Error(res.message || t('Failed to load groups'))
      }
      return Array.isArray(res.data) ? res.data : []
    },
    staleTime: 3 * 60 * 1000,
  })

  const channelsQuery = useQuery({
    queryKey: ['group-fallback-channels', 'enabled'],
    queryFn: async () => {
      const res = await getChannels({
        p: 0,
        page_size: 1000,
        status: 'enabled',
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to load channels'))
      }
      const items = Array.isArray(res?.data?.items) ? res.data.items : []
      return items.filter(
        (channel) => channel.status === CHANNEL_STATUS.ENABLED
      ) as ChannelItem[]
    },
    staleTime: 60 * 1000,
  })

  const fallbackListQuery = useQuery({
    queryKey: ['group-channel-fallback-list'],
    queryFn: getGroupChannelFallbacks,
  })

  const groups = useMemo(() => {
    return groupsQuery.data || []
  }, [groupsQuery.data])

  const channels = useMemo(() => channelsQuery.data || [], [channelsQuery.data])
  const configs = useMemo<GroupChannelFallbackItem[]>(
    () => fallbackListQuery.data?.data || [],
    [fallbackListQuery.data]
  )

  const selectedFallbackChannelId = Number(form.fallback_channel_id)
  const needsSelectedChannelLookup =
    dialogOpen &&
    Number.isInteger(selectedFallbackChannelId) &&
    selectedFallbackChannelId > 0 &&
    !channels.some((channel) => channel.id === selectedFallbackChannelId)

  const selectedChannelQuery = useQuery({
    queryKey: ['group-fallback-selected-channel', selectedFallbackChannelId],
    queryFn: async () => {
      const res = await getChannel(selectedFallbackChannelId)
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to load channels'))
      }
      return res.data
    },
    enabled: needsSelectedChannelLookup,
    staleTime: 60 * 1000,
  })

  const groupItems = useMemo(
    () => groups.map((group) => ({ value: group, label: group })),
    [groups]
  )

  const channelTypeItems = useMemo(
    () =>
      CHANNEL_TYPE_OPTIONS.map((item) => ({
        value: String(item.value),
        label: t(item.label),
      })),
    [t]
  )

  const fallbackChannelOptions = useMemo(() => {
    const channelType = Number(form.channel_type)
    if (!Number.isInteger(channelType)) {
      return [] as Array<{ value: string; label: string }>
    }
    const options = channels
      .filter((channel) => {
        return (
          channel.type === channelType &&
          channel.status === CHANNEL_STATUS.ENABLED
        )
      })
      .map((channel) => ({
        value: String(channel.id),
        label: formatChannelLabel(channel),
      }))

    // Keep the current selection visible while editing, even if it is no longer enabled.
    if (
      form.fallback_channel_id &&
      !options.some((item) => item.value === form.fallback_channel_id)
    ) {
      const selected = selectedChannelQuery.data
      const label =
        selected && selected.id === selectedFallbackChannelId
          ? formatChannelLabel(selected)
          : `#${form.fallback_channel_id}`
      options.unshift({
        value: form.fallback_channel_id,
        label,
      })
    }

    return options
  }, [
    channels,
    form.channel_type,
    form.fallback_channel_id,
    selectedChannelQuery.data,
    selectedFallbackChannelId,
  ])

  const channelDisplayMap = useMemo(() => {
    const map = new Map<number, string>()
    for (const channel of channels) {
      map.set(channel.id, formatChannelLabel(channel))
    }
    if (
      selectedChannelQuery.data &&
      selectedChannelQuery.data.id === selectedFallbackChannelId
    ) {
      map.set(
        selectedChannelQuery.data.id,
        formatChannelLabel(selectedChannelQuery.data)
      )
    }
    return map
  }, [channels, selectedChannelQuery.data, selectedFallbackChannelId])

  const isLoadingAny =
    groupsQuery.isLoading || channelsQuery.isLoading || fallbackListQuery.isLoading

  const isEditing = editingId > 0

  const onReset = () => {
    setForm(DEFAULT_FORM)
    setEditingId(0)
  }

  const openCreateDialog = () => {
    onReset()
    setDialogOpen(true)
  }

  const closeDialog = () => {
    setDialogOpen(false)
    onReset()
  }

  const onDialogOpenChange = (open: boolean) => {
    if (!open) {
      closeDialog()
      return
    }
    setDialogOpen(true)
  }

  const onEdit = (item: GroupChannelFallbackItem) => {
    setForm({
      id: item.id,
      group_name: item.group_name,
      channel_type: String(item.channel_type),
      fallback_channel_id: String(item.fallback_channel_id),
      enabled: item.enabled,
      remark: item.remark || '',
    })
    setEditingId(item.id)
    setDialogOpen(true)
  }

  const onSubmit = async () => {
    const payload = {
      id: editingId || 0,
      group_name: form.group_name.trim(),
      channel_type: Number(form.channel_type),
      fallback_channel_id: Number(form.fallback_channel_id),
      enabled: form.enabled,
      remark: form.remark.trim(),
    }
    if (!payload.group_name) {
      toast.warning(t('Please select group'))
      return
    }
    if (!Number.isInteger(payload.channel_type) || payload.channel_type < 0) {
      toast.warning(t('Please select channel type'))
      return
    }
    if (
      !Number.isInteger(payload.fallback_channel_id) ||
      payload.fallback_channel_id <= 0
    ) {
      toast.warning(t('Please select fallback channel'))
      return
    }

    setSubmitting(true)
    try {
      const res = await upsertGroupChannelFallback(payload)
      if (!res.success) {
        throw new Error(res.message || t('Failed to save setting'))
      }
      toast.success(t('Saved successfully'))
      closeDialog()
      await fallbackListQuery.refetch()
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to save setting')
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  const onDelete = async (item: GroupChannelFallbackItem) => {
    const key = `${item.group_name}#${item.channel_type}`
    setDeletingKey(key)
    try {
      const res = await deleteGroupChannelFallback({
        group_name: item.group_name,
        channel_type: item.channel_type,
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to delete setting'))
      }
      toast.success(t('Deleted successfully'))
      await fallbackListQuery.refetch()
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to delete setting')
      toast.error(message)
    } finally {
      setDeletingKey('')
    }
  }

  const onToggle = async (item: GroupChannelFallbackItem, enabled: boolean) => {
    setSubmitting(true)
    try {
      const res = await upsertGroupChannelFallback({
        id: item.id,
        group_name: item.group_name,
        channel_type: item.channel_type,
        fallback_channel_id: item.fallback_channel_id,
        enabled,
        remark: item.remark || '',
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to update status'))
      }
      toast.success(t('Updated successfully'))
      await fallbackListQuery.refetch()
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to update status')
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <SettingsSection title={t('Group channel fallback')}>
      <p className='text-muted-foreground mb-4 text-sm'>
        {t(
          'This setting only takes effect after normal retries are exhausted. The system matches by group + channel type, then sends one additional request to the configured fallback channel.'
        )}
      </p>

      {isLoadingAny ? (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Loading...')}
        </div>
      ) : (
        <>
          <div className='mb-4'>
            <Button onClick={openCreateDialog} disabled={submitting}>
              <Plus className='mr-1 h-4 w-4' />
              {t('Add group fallback rule')}
            </Button>
          </div>

          <div className='overflow-x-auto rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead>{t('Channel type')}</TableHead>
                  <TableHead>{t('Fallback channel')}</TableHead>
                  <TableHead>{t('Enabled')}</TableHead>
                  <TableHead>{t('Remark')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {configs.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground text-center'
                    >
                      {t('No fallback rules')}
                    </TableCell>
                  </TableRow>
                ) : (
                  configs.map((item) => {
                    const key = `${item.group_name}#${item.channel_type}`
                    const typeLabel =
                      CHANNEL_TYPE_OPTIONS.find(
                        (opt) => opt.value === item.channel_type
                      )?.label || String(item.channel_type)
                    return (
                      <TableRow key={key}>
                        <TableCell>{item.group_name}</TableCell>
                        <TableCell>{t(typeLabel)}</TableCell>
                        <TableCell>
                          {channelDisplayMap.get(item.fallback_channel_id) ||
                            `#${item.fallback_channel_id}`}
                        </TableCell>
                        <TableCell>
                          <Switch
                            checked={item.enabled}
                            onCheckedChange={(checked) =>
                              onToggle(item, checked)
                            }
                            disabled={submitting}
                          />
                        </TableCell>
                        <TableCell>{item.remark || '-'}</TableCell>
                        <TableCell className='text-right'>
                          <div className='flex justify-end gap-2'>
                            <Button
                              variant='outline'
                              size='sm'
                              onClick={() => onEdit(item)}
                              disabled={submitting}
                            >
                              {t('Edit')}
                            </Button>
                            <Button
                              variant='destructive'
                              size='sm'
                              onClick={() => onDelete(item)}
                              disabled={deletingKey === key}
                            >
                              <Trash2 className='mr-1 h-3 w-3' />
                              {t('Delete')}
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      <Dialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        title={
          isEditing
            ? t('Edit group fallback rule')
            : t('Add group fallback rule')
        }
        description={t(
          'After selecting a group and channel type, you can assign any channel of the same type as the fallback. It triggers once only after main retries are exhausted.'
        )}
        contentClassName='max-w-2xl'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={closeDialog}
              disabled={submitting}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={onSubmit} disabled={submitting}>
              {submitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              {t('Save')}
            </Button>
          </>
        }
      >
        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label>{t('Group')}</Label>
            <Select
              items={groupItems}
              value={form.group_name || null}
              onValueChange={(value) =>
                setForm((prev) => ({
                  ...prev,
                  group_name: value ?? '',
                  fallback_channel_id: '',
                }))
              }
            >
              <SelectTrigger disabled={isEditing}>
                <SelectValue placeholder={t('Select group')} />
              </SelectTrigger>
              <SelectContent>
                {groupItems.map((group) => (
                  <SelectItem key={group.value} value={group.value}>
                    {group.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-1.5'>
            <Label>{t('Channel type')}</Label>
            <Select
              items={channelTypeItems}
              value={form.channel_type || null}
              onValueChange={(value) =>
                setForm((prev) => ({
                  ...prev,
                  channel_type: value ?? '',
                  fallback_channel_id: '',
                }))
              }
            >
              <SelectTrigger disabled={isEditing}>
                <SelectValue placeholder={t('Select channel type')} />
              </SelectTrigger>
              <SelectContent>
                {channelTypeItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-1.5 sm:col-span-2'>
            <Label>{t('Fallback channel')}</Label>
            <Select
              items={fallbackChannelOptions}
              value={form.fallback_channel_id || null}
              onValueChange={(value) =>
                setForm((prev) => ({
                  ...prev,
                  fallback_channel_id: value ?? '',
                }))
              }
            >
              <SelectTrigger>
                <SelectValue placeholder={t('Select fallback channel')} />
              </SelectTrigger>
              <SelectContent>
                {fallbackChannelOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-1.5 sm:col-span-2'>
            <Label>{t('Remark')}</Label>
            <Input
              value={form.remark}
              maxLength={255}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  remark: event.target.value,
                }))
              }
              placeholder={t('Optional')}
            />
          </div>

          <div className='flex items-center gap-2'>
            <Label>{t('Enabled')}</Label>
            <Switch
              checked={form.enabled}
              onCheckedChange={(checked) =>
                setForm((prev) => ({ ...prev, enabled: checked }))
              }
            />
          </div>
        </div>
      </Dialog>
    </SettingsSection>
  )
}
