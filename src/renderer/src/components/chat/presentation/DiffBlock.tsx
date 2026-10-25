/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useState } from 'react'
import clsx from 'clsx'
import type { DiffCardModel } from './chat/models/diff-card-model'
import { CodeToolbar, type CodeToolbarLabels } from './CodeToolbar'
import { languageForPath } from './chat/models/read-card-model'
import { headTailCap } from './head-tail-cap'
import cardCss from './CodeCard.module.css'
import css from './DiffBlock.module.css'
export interface DiffBlockLabels extends CodeToolbarLabels {
  copy: string; copied: string; collapse: string; collapseAria: string
  expand: (hidden: number) => string; expandAria: (hidden: number) => string
}
/** Host already produced the applied patch. Render it without a second diff engine or default effects. */
export function DiffBlock({ model, labels, maxLines = 9, onCopy, copied = false }: {
  model: DiffCardModel; labels: DiffBlockLabels; maxLines?: number; onCopy?: () => void; copied?: boolean
}) {
  const [expanded, setExpanded] = useState(false), [wrapped, setWrapped] = useState(false)
  const { rows } = model
  const { hidden, capped, headLines, tailLines } = headTailCap(rows.length, maxLines, expanded)
  const render = (slice: typeof rows, offset: number) => slice.map((row, index) =>
    <div key={offset + index} className={clsx(css.line, css[row.kind])}>{row.text}</div>)
  return <div className={clsx(cardCss.card, css.block)} data-diff data-code-wrap={wrapped}>
    <CodeToolbar lang={model.path ? languageForPath(model.path) : undefined} labels={labels}
      copyLabel={labels.copy} copiedLabel={labels.copied} copied={copied} onCopy={onCopy}
      wrapped={wrapped} onWrap={() => setWrapped(value => !value)} />
    <div className={css.body}>{render(capped ? rows.slice(0, headLines) : rows, 0)}
      {hidden > 0 ? <button type="button" className={css.expand} aria-expanded={expanded}
        aria-label={expanded ? labels.collapseAria : labels.expandAria(hidden)} onClick={() => setExpanded(value => !value)}>
        {expanded ? labels.collapse : labels.expand(hidden)}</button> : null}
      {capped ? render(rows.slice(rows.length - tailLines), rows.length - tailLines) : null}
    </div>
  </div>
}
