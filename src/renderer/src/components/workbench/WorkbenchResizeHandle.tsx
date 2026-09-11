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
}

export function WorkbenchResizeHandle({
  edge = 'right',
  disabled = false,
  isResizing = false,
  onPointerDown,
  onReset,
  className
}: WorkbenchResizeHandleProps): ReactElement {
  const vertical = edge === 'left' || edge === 'right'

  return (
    <div
      role="separator"
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
