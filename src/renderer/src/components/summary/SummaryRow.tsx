import { forwardRef } from 'react'
import type { KeyboardEvent, PointerEvent, ReactElement, ReactNode } from 'react'

export interface SummaryRowProps {
  icon?: ReactNode
  label: ReactNode
  labelClassName?: string
  detail?: ReactNode
  trailing?: ReactNode
  trailingVisible?: boolean
  actions?: ReactNode
  actionsAlwaysFocusable?: boolean
  actionsVisible?: boolean
  interactive?: boolean
  disabled?: boolean
  onClick?: () => void
  onPointerDown?: (event: PointerEvent<HTMLDivElement>) => void
  title?: string
  className?: string
  ariaExpanded?: boolean
  'aria-expanded'?: boolean
}

export const SummaryRow = forwardRef<HTMLDivElement, SummaryRowProps>(function SummaryRow({
  icon,
  label,
  labelClassName,
  detail,
  trailing,
  trailingVisible = false,
  actions,
  actionsAlwaysFocusable = false,
  actionsVisible = false,
  interactive = false,
  disabled = false,
  onClick,
  onPointerDown,
  title,
  className,
  ariaExpanded,
  'aria-expanded': ariaExpandedProp
}, ref): ReactElement {
  const expanded = ariaExpanded ?? ariaExpandedProp
  const canClick = interactive && !disabled && Boolean(onClick)
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
    if (!canClick) return
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onClick?.()
  }
  const handleClick = (): void => {
    if (!canClick) return
    onClick?.()
  }
  const handleActionsKeyDown = (event: KeyboardEvent<HTMLSpanElement>): void => {
    event.stopPropagation()
  }
  const labelClassNames = [
    'block truncate font-medium leading-5',
    labelClassName ? '' : interactive && !disabled ? 'text-ds-ink' : 'text-ds-faint',
    labelClassName
  ].filter(Boolean).join(' ')
  const rootClassNames = [
    'group/summary-row group/summary-panel-row relative isolate flex h-7 w-full min-w-0 items-center gap-2 rounded-sm border-0 bg-transparent px-0 py-1 text-left text-base outline-none transition',
    disabled
      ? 'cursor-not-allowed text-ds-muted opacity-60'
      : interactive
        ? "cursor-pointer text-ds-ink before:absolute before:inset-y-0 before:-inset-x-2 before:-z-10 before:rounded-sm before:content-[''] hover:before:bg-ds-hover focus-visible:before:bg-ds-hover"
        : 'text-ds-faint',
    className
  ].filter(Boolean).join(' ')
  const actionsClassNames = [
    'shrink-0 items-center gap-1 transition-opacity',
    actionsVisible
      ? 'flex opacity-100'
      : actionsAlwaysFocusable
        ? 'pointer-events-none absolute inset-y-0 right-0 flex opacity-0 group-hover/summary-row:pointer-events-auto group-hover/summary-row:opacity-100 group-focus-within/summary-row:pointer-events-auto group-focus-within/summary-row:opacity-100'
        : 'hidden group-hover/summary-row:flex group-focus-within/summary-row:flex',
    !trailing || actionsVisible ? 'ms-auto' : ''
  ].filter(Boolean).join(' ')
  return (
    <div
      ref={ref}
      role={interactive ? 'button' : undefined}
      tabIndex={canClick ? 0 : undefined}
      aria-disabled={disabled || undefined}
      aria-expanded={expanded}
      title={title}
      className={rootClassNames}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      onPointerDown={disabled ? undefined : onPointerDown}
    >
      {icon ? (
        <span className={`flex h-5 w-5 shrink-0 items-center justify-center ${interactive && !disabled ? 'text-ds-ink' : 'text-ds-muted'}`}>
          {icon}
        </span>
      ) : null}
      <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
        <span className="min-w-0 flex-1 overflow-hidden">
          <span className={labelClassNames}>{label}</span>
          {detail ? <span className="sr-only">{detail}</span> : null}
        </span>
        {actions ? (
          <span
            className={actionsClassNames}
            onClick={(event) => event.stopPropagation()}
            onKeyDown={handleActionsKeyDown}
          >
            {actions}
          </span>
        ) : null}
        {trailing ? (
          <span
            className={`shrink-0 text-base leading-none text-ds-faint transition-opacity ${
              trailingVisible ? 'opacity-100' : 'opacity-0 group-hover/summary-row:opacity-100 group-focus-within/summary-row:opacity-100'
            }`}
          >
            {trailing}
          </span>
        ) : null}
      </span>
    </div>
  )
})

SummaryRow.displayName = 'SummaryRow'
