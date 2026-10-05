import type { ReactElement, Ref } from 'react'
import { ChevronRight } from '../../design/AnalytixUiIcons'

export function ActionMenuSeparator(): ReactElement {
  return <div className="ds-session-actions-menu-separator" role="separator" />
}

export function ActionMenuItem({
  icon,
  label,
  shortcut,
  disabled = false,
  active = false,
  danger = false,
  hasSubmenu = false,
  expanded,
  controls,
  tabIndex,
  ref,
  title,
  onClick,
  onPointerEnter
}: {
  icon: ReactElement
  label: string
  shortcut?: string
  disabled?: boolean
  active?: boolean
  danger?: boolean
  hasSubmenu?: boolean
  expanded?: boolean
  controls?: string
  tabIndex?: number
  ref?: Ref<HTMLButtonElement>
  title?: string
  onClick?: () => void
  onPointerEnter?: () => void
}): ReactElement {
  return (
    <button
      type="button"
      role="menuitem"
      ref={ref}
      tabIndex={tabIndex}
      aria-haspopup={hasSubmenu ? 'menu' : undefined}
      aria-expanded={hasSubmenu ? expanded : undefined}
      aria-controls={controls}
      disabled={disabled}
      title={title}
      className={`ds-session-actions-menu-item${active ? ' is-active' : ''}${danger ? ' is-danger' : ''}`}
      onClick={onClick}
      onPointerEnter={onPointerEnter}
    >
      <span className="ds-session-actions-menu-icon">{icon}</span>
      <span className="ds-session-actions-menu-label">{label}</span>
      {shortcut ? <span className="ds-session-actions-menu-shortcut">{shortcut}</span> : null}
      {hasSubmenu ? <ChevronRight className="ds-session-actions-menu-chevron" strokeWidth={1.9} /> : null}
    </button>
  )
}
