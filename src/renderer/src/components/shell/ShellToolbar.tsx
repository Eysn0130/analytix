import type { ReactElement } from 'react'
import { useState } from 'react'

export function ToolbarTooltip({
  label,
  hidden = false,
  children
}: {
  label: string
  hidden?: boolean
  children?: ReactElement
}): ReactElement {
  const [suppressed, setSuppressed] = useState(false)
  const tooltipHidden = hidden || suppressed || !label.trim()

  return (
    <span
      className="ds-toolbar-tooltip-anchor ds-no-drag"
      data-tooltip-hidden={tooltipHidden ? 'true' : undefined}
      onClickCapture={() => setSuppressed(true)}
      onKeyDownCapture={(event) => {
        if (event.key === 'Enter' || event.key === ' ') setSuppressed(true)
      }}
      onBlurCapture={(event) => {
        const nextTarget = event.relatedTarget
        if (!(nextTarget instanceof Node) || !event.currentTarget.contains(nextTarget)) {
          setSuppressed(false)
        }
      }}
      onPointerLeave={() => setSuppressed(false)}
    >
      {children}
      <span className="ds-toolbar-tooltip-bubble" role="tooltip">
        {label}
      </span>
    </span>
  )
}

export function shellToolbarIconButtonClass(active = false, extraClass = ''): string {
  return `ds-toolbar-icon-button${active ? ' is-active' : ''}${extraClass ? ` ${extraClass}` : ''}`
}
