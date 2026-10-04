/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
/** Shared shortcut keycaps; callers supply the effective platform presentation. */
import css from './ShortcutKeys.module.css'
import clsx from 'clsx'

/**
 * Render one command's keycaps without owning binding defaults or localized copy.
 * @param props - effective key labels, presentation variant and optional interaction styling.
 * @returns unboxed keys by default, or tooltip keycaps with plus-separated combinations grouped together.
 */
export function ShortcutKeys({ keys, variant = 'plain', className }: { keys: readonly string[]; variant?: 'plain' | 'tooltip'; className?: string | undefined }) {
  return <span className={clsx(css.keys, variant === 'tooltip' && css.tooltip, variant === 'tooltip' && keys.includes('+') && css.joined, className)}>{keys.map((key, index) => <kbd key={index} className={key === '+' ? css.separator : css.key}>{key}</kbd>)}</span>
}
