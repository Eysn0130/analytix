import type { CSSProperties, ReactElement } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'

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
  const anchorRef = useRef<HTMLSpanElement>(null)
  const bubbleRef = useRef<HTMLSpanElement>(null)
  const [shift, setShift] = useState(0)
  const positionTooltip = useCallback(() => {
    const anchor = anchorRef.current
    const bubble = bubbleRef.current
    if (!anchor || !bubble) return
    const rect = anchor.getBoundingClientRect()
    const width = bubble.getBoundingClientRect().width
    if (!width || !rect.width) return
    const centeredLeft = rect.left + rect.width / 2 - width / 2
    const left = Math.max(12, Math.min(centeredLeft, window.innerWidth - 12 - width))
    // Convert viewport pixels back to the existing UI zoom coordinate space.
    const scale = anchor.offsetWidth ? rect.width / anchor.offsetWidth : 1
    setShift((left - centeredLeft) / scale)
  }, [])
  useEffect(() => {
    positionTooltip()
    window.addEventListener('resize', positionTooltip)
    return () => window.removeEventListener('resize', positionTooltip)
  }, [label, positionTooltip, tooltipHidden])

  return (
    <span
      ref={anchorRef}
      style={{ '--ds-tooltip-shift': `${shift}px` } as CSSProperties}
      onFocusCapture={positionTooltip}
      onPointerEnter={positionTooltip}
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
      <span ref={bubbleRef} className="ds-toolbar-tooltip-bubble" role="tooltip">
        {label}
      </span>
    </span>
  )
}

export function shellToolbarIconButtonClass(active = false, extraClass = ''): string {
  return `ds-toolbar-icon-button${active ? ' is-active' : ''}${extraClass ? ` ${extraClass}` : ''}`
}
