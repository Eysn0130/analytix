import { useEffect, useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { DesktopEnvironment } from '@shared/desktop-environment'

export function DevelopmentInstanceDetails({ environment }: { environment: DesktopEnvironment }): ReactElement | null {
  const { t } = useTranslation('common')
  if (environment.mode !== 'development') return null
  return (
    <div className="ds-no-drag relative z-[60] flex h-6 shrink-0 items-center justify-end border-b border-ds-border-muted px-3 text-[11px] text-ds-muted">
      <details className="group" onKeyDown={(event) => {
        if (event.key !== 'Escape') return
        event.currentTarget.open = false
        event.currentTarget.querySelector('summary')?.focus()
      }}>
        <summary className="cursor-pointer rounded px-1 focus-visible:outline-accent">
          {t(environment.mock ? 'developmentInstanceMock' : 'developmentInstance')} · {environment.profileId.slice(0, 6)}
        </summary>
        <div className="absolute right-3 top-full mt-1 grid max-w-sm gap-1.5 rounded-xl border border-ds-border bg-ds-card p-3 text-[12px] shadow-lg">
          <p>{t(environment.isolated ? 'developmentInstanceIsolated' : 'developmentInstanceStandard')}</p>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
            <dt>{t('developmentInstanceProfile')}</dt><dd>{environment.profileId}</dd>
            <dt>{t('developmentInstanceVersion')}</dt><dd>{environment.version}</dd>
            <dt>{t('developmentInstanceMainBuild')}</dt><dd>{environment.mainBuildId ?? t('developmentInstanceUnknown')}</dd>
          </dl>
          {environment.mock ? <p className="text-ds-faint">{t('developmentInstanceMockHint')}</p> : null}
        </div>
      </details>
    </div>
  )
}

export function DevelopmentInstanceBar(): ReactElement | null {
  const [environment, setEnvironment] = useState<DesktopEnvironment | null>(null)
  useEffect(() => {
    let active = true
    const read = window.analytix?.app?.getEnvironment
    if (read) void read().then((result) => { if (active) setEnvironment(result) }).catch(() => undefined)
    return () => { active = false }
  }, [])
  return environment ? <DevelopmentInstanceDetails environment={environment} /> : null
}
