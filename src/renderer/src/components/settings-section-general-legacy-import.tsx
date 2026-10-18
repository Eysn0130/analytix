import { useEffect, useState, type ReactElement } from 'react'
import { Loader2 } from '../design/AnalytixUiIcons'
import type { LegacySessionDetectResult } from '@shared/analytix-api'
import { SettingsCard, SettingRow } from './settings-controls'

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

function sum(detection: LegacySessionDetectResult | null, key: 'threadCount' | 'newCount'): number {
  return detection?.sources.reduce((total, source) => total + source[key], 0) ?? 0
}

export function LegacySessionImportCard({
  t
}: {
  t: TranslateFn
  tCommon: TranslateFn
}): ReactElement {
  const [detection, setDetection] = useState<LegacySessionDetectResult | null>(null)
  const [detecting, setDetecting] = useState(true)
  const [scanUnavailable, setScanUnavailable] = useState(false)

  useEffect(() => {
    let active = true
    const detect = window.analytix?.diagnostics?.detectLegacySessions
    const pending = typeof detect === 'function'
      ? Promise.resolve().then(() => detect())
      : Promise.reject(new Error('Legacy source detection unavailable.'))
    void pending.then((result) => {
      if (!active) return
      setDetection(result)
      setDetecting(false)
    }, () => {
      if (!active) return
      setScanUnavailable(true)
      setDetecting(false)
    })
    return () => { active = false }
  }, [])

  const totalNew = sum(detection, 'newCount')
  const totalFound = sum(detection, 'threadCount')

  const statusText = detecting
    ? t('legacyImportScanning')
    : scanUnavailable
      ? t('legacyImportScanUnavailable')
      : totalNew > 0
      ? t('legacyImportFound', { count: totalNew })
      : totalFound > 0
        ? t('legacyImportAllPresent')
        : t('legacyImportNoneFound')

  return (
    <SettingsCard title={t('legacyImportTitle')} className="mt-6">
      <SettingRow
        title={t('legacyImportTitle')}
        description={t('legacyImportDesc')}
        wideControl
        control={
          <div className="flex w-full min-w-0 flex-col items-start gap-2.5">
            <div className="flex items-center gap-1.5 text-[13px] text-ds-muted">
              {detecting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              <span>{statusText}</span>
            </div>

            {detection && detection.sources.length > 0 ? (
              <ul className="w-full space-y-1">
                {detection.sources.map((source) => (
                  <li
                    key={source.id}
                    className="flex flex-wrap items-baseline gap-x-2 text-[12px] text-ds-faint"
                  >
                    <span className="font-medium text-ds-muted">
                      {t('legacyImportSourceCount', {
                        newCount: source.newCount,
                        total: source.threadCount
                      })}
                    </span>
                    <code className="break-all font-mono">{source.path}</code>
                  </li>
                ))}
              </ul>
            ) : null}

            <p className="text-[13px] text-ds-muted">{t('legacyImportUnavailable')}</p>

          </div>
        }
      />
    </SettingsCard>
  )
}
