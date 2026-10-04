/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { memo, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react'
import clsx from 'clsx'
import { ChevronDown as IconChevronDownOutlineRegular, ChevronUp as IconChevronUpOutlineRegular } from '../../../design/AnalytixUiIcons'
import { TextShimmer } from './TextShimmer'
import css from './DisclosureRow.module.css'

/** Shared 24px process row: tertiary text and icons, secondary on hover. */
export interface DisclosureRowProps {
  summary?: ReactNode
  actions?: ReactNode
  error?: boolean
  showDisclosureIcon?: boolean
  icon: ReactNode
  title: string
  open: boolean
  expandable: boolean
  onToggle: () => void
  /** Animate the complete header while its owning operation is running. */
  running?: boolean | undefined
  /** Makes the complete title row the disclosure target. */
  expandOnRowClick?: boolean | undefined
  /** Replaces the collapsed icon with a chevron while the row is hovered. */
  previewChevron?: boolean | undefined
  /** Keeps `collapsedContent` inline while open. */
  keepContentWhenOpen?: boolean | undefined
  collapsedContent?: ReactNode
  children?: ReactNode
  className?: string | undefined
  rowClassName?: string | undefined
  /** Sizing class for the header text area, beside the leading icon. */
  contentClassName?: string | undefined
  /** Layout class shared by the header text and its decorative copy. */
  contentLayoutClassName?: string | undefined
  leadingClassName?: string | undefined
  chevronClassName?: string | undefined
  titleClassName?: string | undefined
}

/**
 * Render one disclosure header and its controlled expanded content.
 * Shallow prop comparison requires stable callbacks and React nodes to skip unchanged renders.
 * @param props - Visual content, controlled state, and interaction policy.
 * @returns the disclosure row.
 */
export const DisclosureRow = memo(function DisclosureRow({
  icon,
  summary,
  actions,
  error = false,
  showDisclosureIcon = true,
  title,
  open,
  expandable,
  onToggle,
  running = false,
  expandOnRowClick = false,
  previewChevron = expandable,
  keepContentWhenOpen = false,
  collapsedContent,
  children,
  className,
  rowClassName,
  contentClassName,
  contentLayoutClassName,
  leadingClassName,
  chevronClassName,
  titleClassName,
}: DisclosureRowProps) {
  const rowExpands = expandable && expandOnRowClick
  const toggleFromLeading = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    onToggle()
  }
  const toggleFromKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!rowExpands || event.nativeEvent.isComposing || event.target !== event.currentTarget || (event.key !== 'Enter' && event.key !== ' ')) return
    event.preventDefault()
    onToggle()
  }
  const collapsedLeading = previewChevron
    ? (
      <>
        <span className={css.iconIdle}>{icon}</span>
        <IconChevronDownOutlineRegular className={clsx(chevronClassName, css.chevronHover)} />
      </>
    )
    : icon
  const leading = !showDisclosureIcon ? icon : open
    ? <IconChevronUpOutlineRegular className={chevronClassName} />
    : collapsedLeading

  return (
    <div className={clsx(css.root, className)} data-open={open || undefined}>
      <div
        className={clsx(css.row, rowClassName)}
        data-error={error || undefined}
        data-disclosure-row
        data-expandable={rowExpands || undefined}
        role={rowExpands ? 'button' : undefined}
        tabIndex={rowExpands ? 0 : undefined}
        aria-expanded={rowExpands ? open : undefined}
        onClick={rowExpands ? onToggle : undefined}
        onKeyDown={rowExpands ? toggleFromKeyboard : undefined}
      >
        {expandable && !rowExpands ? (
          <button
            type="button"
            className={clsx(css.leading, leadingClassName)}
            aria-label={title}
            aria-expanded={open}
            onClick={toggleFromLeading}
          >
            {leading}
          </button>
        ) : (
          <span className={clsx(css.leading, leadingClassName)}>
            {leading}
          </span>
        )}
        <div className={clsx(css.content, contentClassName)}>
          {title && <TextShimmer active={running} className={clsx(css.title, titleClassName)}>{title}</TextShimmer>}
          {summary}
          {(keepContentWhenOpen || !open) && collapsedContent}
        </div>
        {actions && <div className={css.actions} onClick={event => event.stopPropagation()}>{actions}</div>}

      </div>
      {open && children}
    </div>
  )
})
