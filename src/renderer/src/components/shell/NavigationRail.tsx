import type { ReactElement } from 'react'
import { Clock3, LayoutGrid, MessageSquare, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ToolbarTooltip } from './ShellToolbar'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'
import './navigation-rail.css'

type NavigationRailProps = {
  active: 'chat' | 'plugins' | 'schedule' | 'settings' | 'other'
  onChat: () => void
  onPlugins: () => void
  onSchedule: () => void
  onSettings: () => void
}

/** Existing product destinations stay available when the project sidebar closes. */
export function NavigationRail({ active, onChat, onPlugins, onSchedule, onSettings }: NavigationRailProps): ReactElement {
  const { t } = useTranslation('common')
  const destinations = [
    { id: 'chat', label: t('chatNavigationLabel'), icon: MessageSquare, onClick: onChat },
    { id: 'plugins', label: t('plugins'), icon: LayoutGrid, onClick: onPlugins },
    { id: 'schedule', label: t('schedule'), icon: Clock3, onClick: onSchedule }
  ] as const

  return (
    <nav className="ds-navigation-rail ds-no-drag" aria-label={t('appName')}>
      <div className="ds-navigation-rail-brand" aria-hidden>
        <img className="ds-navigation-rail-brand-light" src={AnalytixIconRegistry.brand.symbolMonoBlack} alt="" />
        <img className="ds-navigation-rail-brand-dark" src={AnalytixIconRegistry.brand.symbolMonoWhite} alt="" />
      </div>
      <div className="ds-navigation-rail-destinations">
        {destinations.map(({ id, label, icon: Icon, onClick }) => (
          <ToolbarTooltip key={id} label={label}>
            <button type="button" className="ds-navigation-rail-button" aria-label={label}
              aria-current={active === id ? 'page' : undefined} onClick={onClick}>
              <Icon size={20} strokeWidth={1.75} aria-hidden />
            </button>
          </ToolbarTooltip>
        ))}
      </div>
      <ToolbarTooltip label={t('settings')}>
        <button type="button" className="ds-navigation-rail-button" aria-label={t('settings')}
          aria-current={active === 'settings' ? 'page' : undefined} onClick={onSettings}>
          <Settings size={20} strokeWidth={1.75} aria-hidden />
        </button>
      </ToolbarTooltip>
    </nav>
  )
}
