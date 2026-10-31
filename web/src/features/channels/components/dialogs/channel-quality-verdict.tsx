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

import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'

type ChannelQualityVerdictBadgeProps = {
  verdict: string
  className?: string
}

export function ChannelQualityVerdictBadge(
  props: ChannelQualityVerdictBadgeProps
) {
  const { t } = useTranslation()
  return (
    <StatusBadge
      label={qualityVerdictText(props.verdict, t)}
      variant={qualityVerdictVariant(props.verdict)}
      size='sm'
      copyable={false}
      className={props.className}
    />
  )
}

function qualityVerdictText(
  verdict: string,
  t: (key: string) => string
): string {
  if (verdict === 'passed') return t('Passed')
  if (verdict === 'failed') return t('Failed')
  if (verdict === 'unknown') return t('Unknown')
  if (verdict === 'error') return t('Error')
  if (verdict === 'unsupported') return t('Unsupported')
  if (verdict === 'running') return t('Running')
  if (verdict === 'skipped') return t('Skipped')
  return verdict
}

function qualityVerdictVariant(verdict: string): StatusBadgeProps['variant'] {
  if (verdict === 'passed') return 'success'
  if (verdict === 'failed') return 'danger'
  if (verdict === 'unknown') return 'warning'
  if (verdict === 'error') return 'warning'
  return 'neutral'
}
