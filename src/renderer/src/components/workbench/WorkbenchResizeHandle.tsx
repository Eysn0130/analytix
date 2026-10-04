import type { PointerEventHandler, ReactElement } from 'react'

export type WorkbenchResizeEdge = 'left' | 'right' | 'top' | 'bottom'

function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}

type WorkbenchResizeHandleProps = {
  edge?: WorkbenchResizeEdge
  disabled?: boolean
  isResizing?: boolean
  onPointerDown: PointerEventHandler<HTMLDivElement>
  onReset?: () => void
  className?: string
  ariaLabel?: string
  value?: number
  min?: number
  max?: number
  onResizeDelta?: (delta: number) => void
}

export function WorkbenchResizeHandle({
  edge = 'right',
  disabled = false,
  isResizing = false,
  onPointerDown,
  onReset,
  className,
  ariaLabel, value, min, max, onResizeDelta
}: WorkbenchResizeHandleProps): ReactElement {
  const vertical = edge === 'left' || edge === 'right'

  return (
    <div
      role="separator"
      tabIndex={onResizeDelta && !disabled ? 0 : undefined}
      aria-label={ariaLabel}
      aria-valuenow={value}
      aria-valuemin={min}
      aria-valuemax={max}
      onKeyDown={(event) => {
        if (disabled || !onResizeDelta || event.altKey || event.metaKey || event.ctrlKey) return
        if (event.key === 'Enter' && onReset) { event.preventDefault(); onReset(); return }
        const increase = vertical ? edge === 'left' ? 'ArrowLeft' : 'ArrowRight' : edge === 'top' ? 'ArrowUp' : 'ArrowDown'
        const decrease = vertical ? edge === 'left' ? 'ArrowRight' : 'ArrowLeft' : edge === 'top' ? 'ArrowDown' : 'ArrowUp'
        if (event.key !== increase && event.key !== decrease) return
        event.preventDefault()
        onResizeDelta((event.key === increase ? 1 : -1) * (event.shiftKey ? 64 : 24))
      }}
      aria-disabled={disabled || undefined}
      aria-orientation={vertical ? 'vertical' : 'horizontal'}
      className={cx(
        'ds-workbench-resize-handle ds-no-drag',
        `ds-workbench-resize-handle-${edge}`,
        isResizing && 'is-resizing',
        disabled && 'is-disabled',
        className
      )}
      onPointerDown={disabled ? undefined : onPointerDown}
      onClick={(event) => {
        if (disabled || event.detail !== 2) return
        event.preventDefault()
        onReset?.()
      }}
    >
      <div className="ds-workbench-resize-handle-line" />
    </div>
  )
}
