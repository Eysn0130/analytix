/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { memo, useId, type ReactNode } from 'react'
import { DisclosureRow } from '../DisclosureRow'
import { DshThinkOutlineRegular } from '../DshToolIcons'
import { TextShimmer } from '../TextShimmer'
import type { ChatPresentationPolicy } from './chat-view-model'
import { presentationPolicyFor } from './chat-view-model'
import css from './ReasoningRow.module.css'

export function reasoningPreview(text: string, running: boolean): string {
  if (!running) return text.slice(0, text.indexOf('\n') < 0 ? text.length : text.indexOf('\n')).replaceAll('**', '')
  let summary = '', start = 0
  const separator = /\r?\n(?:[\t ]*\r?\n)+/g
  while (true) {
    const next = separator.exec(text)
    const end = next === null ? text.length : next.index + next[0].indexOf('\n')
    const newline = text.indexOf('\n', start)
    if (newline !== -1 && newline <= end) {
      const candidate = text.slice(start, newline).trim()
      if (candidate) summary = candidate
    }
    if (next === null) return summary.replaceAll('**', '')
    start = next.index + next[0].length
  }
}
type ReasoningRowProps = {
  id: string; running?: boolean; policy?: ChatPresentationPolicy; visible?: boolean
  protectedBody: ReactNode; title: string; preview: string; expanded: boolean; expandable: boolean; onToggle: () => void
}
// Only the mounted protected leaf supplies a genuine native Think. Ordinary
// assistant text uses MessageBubble; this view never parses public text as Think.
export const ReasoningRow = memo(function ReasoningRow({ id, running = false, policy = presentationPolicyFor('detailed'), visible = true,
  protectedBody, title, preview, expanded, expandable, onToggle }: ReasoningRowProps) {
  const bodyId = useId()
  const open = visible && expanded
  // This leaf receives the Host's already projected first-line preview.
  // Parsing it again as a raw running paragraph would erase a valid single line.
  const summary = preview.replaceAll('**', '')
  const showPreview = !open && summary !== '' && (running || policy.settledReasoningPreview)
  return <div className={css.root} data-variant="think" data-state={running ? 'running' : 'ok'} data-expanded={open || undefined}
    data-preview={showPreview || undefined} data-reasoning-id={id}>
    <DisclosureRow rowClassName={css.row} leadingClassName={css.leading} titleClassName={css.title}
      icon={<DshThinkOutlineRegular size={14} />} title={title} running={running} open={open} expandable={expandable} expandOnRowClick
      bodyId={bodyId} onToggle={onToggle} collapsedContent={<>
        <span className={css.separator} data-shimmer-decoration aria-hidden />
        <span className={css.summary} data-streaming={running || undefined}><span className={css.summaryText}><TextShimmer active={running}>{summary}</TextShimmer></span></span>
      </>}>
      {open ? <div id={bodyId} className={css.thinkBody}>{protectedBody}</div> : null}
    </DisclosureRow>
  </div>
})
