import type { ReactElement } from 'react'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'

type PanelCollapseDirection = 'left' | 'right' | 'top' | 'bottom'

type PanelCollapseButtonProps = {
  title: string
  ariaLabel?: string
  onClick: () => void
  direction?: PanelCollapseDirection
  className?: string
  disabled?: boolean
}

function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}

export function PanelCollapseButton({
  title,
  ariaLabel,
  onClick,
  direction = 'right',
  className,
  disabled = false
}: PanelCollapseButtonProps): ReactElement {
  const CollapseIcon = AnalytixIconRegistry.icons.sidebarHide

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      aria-label={ariaLabel ?? title}
      className={cx(
        'ds-titlebar-sidebar-toggle ds-panel-collapse-button ds-no-drag',
        `ds-panel-collapse-button-${direction}`,
        className
      )}
    >
      <CollapseIcon className="ds-sidebar-toggle-icon" />
    </button>
  )
}
