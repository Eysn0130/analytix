/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { AppSettingsPatch, PerformanceUsageMode } from '@shared/app-settings'
import { usePerformanceUsageMode } from './chat/presentation/chat/TurnUsagePanel'
import { SettingRow } from './settings-controls'

type Translate = (key: string) => string
const options = ['compact', 'detailed'] as const

export function SettingsPerformanceUsage({ update, t, value, selectClassName = '' }: {
  update: (patch: AppSettingsPatch) => void
  t: Translate
  value?: PerformanceUsageMode
  selectClassName?: string
}) {
  const savedMode = usePerformanceUsageMode()
  const mode = value ?? savedMode
  return (
    <SettingRow
      title={t('performanceUsage')}
      description={t('performanceUsageDesc')}
      control={
        <select
          className={selectClassName}
          aria-label={t('performanceUsage')}
          value={mode}
          onChange={(event) => update({ performanceUsageMode: event.target.value as PerformanceUsageMode })}
        >
          {options.map((option) => (
            <option key={option} value={option}>
              {t(option === 'compact' ? 'performanceUsageCompact' : 'performanceUsageDetailed')}
            </option>
          ))}
        </select>
      }
    />
  )
}
