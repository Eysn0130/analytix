import type { ReactNode } from 'react'
import { X } from '../../design/AnalytixUiIcons'

/** Composer-only shell. Each source retains its own label, actions and authority adapters. */
export function ComposerReferenceChip({ children, title, remove }: {
  children: ReactNode
  title?: string
  remove?: { label: string; onClick: () => void }
}) {
  return <span
    className="ds-no-drag inline-flex h-7 min-w-0 max-w-full items-center gap-1.5 rounded-lg border border-ds-border-muted bg-ds-card/80 px-2 text-[12px] font-medium text-ds-muted"
    title={title}
  >
    {children}
    {remove ? <button type="button" onClick={remove.onClick}
      className="shrink-0 rounded-full p-0.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
      aria-label={remove.label} title={remove.label}>
      <X className="h-3 w-3" strokeWidth={2} />
    </button> : null}
  </span>
}
