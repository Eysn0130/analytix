import type { ReactElement } from 'react'
import { SettingsCard, SettingRow } from './settings-controls'

export function MediaGenerationSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const { t } = ctx
  const unavailable = (
    <span className="rounded-lg border border-ds-border-muted bg-ds-main/40 px-2.5 py-1 text-[12px] font-medium text-ds-faint">
      {t('mediaGenerationUnavailable')}
    </span>
  )

  return (
    <SettingsCard title={t('mediaGeneration')}>
      <div className="px-5 py-4 text-[13px] leading-6 text-ds-muted">
        {t('mediaGenerationDesc')}
      </div>
      <SettingRow
        title={t('textToSpeech')}
        description={t('mediaGenerationUnavailableDesc')}
        control={unavailable}
      />
      <SettingRow
        title={t('musicGeneration')}
        description={t('mediaGenerationUnavailableDesc')}
        control={unavailable}
      />
      <SettingRow
        title={t('videoGeneration')}
        description={t('mediaGenerationUnavailableDesc')}
        control={unavailable}
      />
    </SettingsCard>
  )
}
