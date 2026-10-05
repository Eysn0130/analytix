import type { CSSProperties, ReactElement } from 'react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

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
    const bubbleRect = bubble.getBoundingClientRect()
    if (!bubbleRect.width || !rect.width) return
    // Measure in viewport pixels, then return to the existing UI zoom space.
    const scale = anchor.offsetWidth ? rect.width / anchor.offsetWidth : 1
    const currentShift = Number.parseFloat(anchor.style.getPropertyValue('--ds-tooltip-shift')) || 0
    const unshiftedLeft = bubbleRect.left - currentShift * scale
    const left = Math.max(12, Math.min(unshiftedLeft, window.innerWidth - 12 - bubbleRect.width))
    const nextShift = (left - unshiftedLeft) / scale
    setShift((previous) => Math.abs(previous - nextShift) < 0.1 ? previous : nextShift)
  }, [])
  // Panel animation moves the anchor without changing the tooltip label or size.
  useLayoutEffect(() => {
    positionTooltip()
  })
  useEffect(() => {
    positionTooltip()
    window.addEventListener('resize', positionTooltip)
    return () => window.removeEventListener('resize', positionTooltip)
  }, [label, positionTooltip, tooltipHidden])
  useEffect(() => {
    const dismissOnEscape = (event: KeyboardEvent) => {
      const anchor = anchorRef.current
      if (event.key !== 'Escape' || event.isComposing || !anchor) return
      if (anchor.contains(document.activeElement) || anchor.matches(':hover')) setSuppressed(true)
    }
    document.addEventListener('keydown', dismissOnEscape)
    return () => document.removeEventListener('keydown', dismissOnEscape)
  }, [])

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
        if (!event.nativeEvent.isComposing && (event.key === 'Enter' || event.key === ' ')) setSuppressed(true)
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
