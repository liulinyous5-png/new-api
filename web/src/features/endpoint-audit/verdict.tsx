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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

const labels: Record<string, string> = {
  passed: 'Passed',
  failed: 'Failed',
  error: 'Request error',
  running: 'Running',
}

export function EndpointVerdict(props: { verdict: string }) {
  const { t } = useTranslation()
  const label = labels[props.verdict]
  return (
    <span
      className={cn(
        'shrink-0 rounded-md px-1.5 py-0.5 text-xs',
        props.verdict === 'passed' &&
          'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
        props.verdict === 'failed' && 'bg-destructive/15 text-destructive',
        props.verdict === 'error' &&
          'bg-amber-500/15 text-amber-700 dark:text-amber-300',
        props.verdict === 'running' && 'bg-muted text-muted-foreground'
      )}
    >
      {label ? t(label) : props.verdict}
    </span>
  )
}
