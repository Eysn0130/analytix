/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useState, type ReactNode } from 'react'
import type { ToolResultLocalSearchDataV1 } from '../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { headTailCap } from './head-tail-cap'
import css from './SearchBlock.module.css'

export interface SearchBlockLabels {
  pathsSummary: (shown: number, total: number | null, truncated: boolean) => string
  matchesSummary: (shown: number, total: number | null, files: number, truncated: boolean) => string
  copy: string; copied: string; noResults: string; collapseAria: string
  expandAria: (hidden: number) => string; collapse: string; expand: (hidden: number) => string
}
export type SearchBlockProps = { search: ToolResultLocalSearchDataV1; labels: SearchBlockLabels; onCopy: () => void; copied: boolean; pending?: boolean; maxLines?: number; className?: string; captureTruncated?: boolean }
type Row = { type: 'file'; index: number; path: string; count: number; collapsed: boolean } |
  { type: 'line'; fileIndex: number; number: number; line: string; context: boolean; column?: number; key: string } |
  { type: 'path'; path: string }

/** DSH disclosure with Host-owned copying and explicit source context rows. */
export function SearchBlock({ search, labels, onCopy, copied, pending = false, maxLines = 16, className, captureTruncated = false }: SearchBlockProps) {
  const [expanded, setExpanded] = useState(false), [collapsed, setCollapsed] = useState<ReadonlySet<number>>(() => new Set())
  const rows: Row[] = []
  if (search.shape === 'paths') search.paths.forEach(path => rows.push({ type: 'path', path }))
  else search.files.forEach((file, index) => {
    rows.push({ type: 'file', index, path: file.path, count: file.matches.length, collapsed: collapsed.has(index) })
    if (collapsed.has(index)) return
    file.matches.forEach(match => {
      match.contextBefore.forEach((line, offset) => rows.push({ type: 'line', fileIndex: index, number: match.lineNumber - match.contextBefore.length + offset,
        line, context: true, key: `${index}:${match.lineNumber}:before:${offset}` }))
      rows.push({ type: 'line', fileIndex: index, number: match.lineNumber, line: match.line, context: false, column: match.column, key: `${index}:${match.lineNumber}` })
      match.contextAfter.forEach((line, offset) => rows.push({ type: 'line', fileIndex: index, number: match.lineNumber + offset + 1,
        line, context: true, key: `${index}:${match.lineNumber}:after:${offset}` }))
    })
  })
  const shown = search.shape === 'paths' ? search.paths.length : search.files.reduce((sum, file) => sum + file.matches.length, 0)
  const { hidden, capped, headLines, tailLines } = headTailCap(rows.length, maxLines, expanded)
  const head = capped ? rows.slice(0, headLines) : rows, naturalTail = capped ? rows.slice(rows.length - tailLines) : []
  const lead = naturalTail[0]
  const tailHeader = lead?.type === 'line' && !head.some(row => row.type === 'file' && row.index === lead.fileIndex)
    ? rows.find((row): row is Extract<Row, { type: 'file' }> => row.type === 'file' && row.index === lead.fileIndex) : undefined
  const tail = tailHeader ? naturalTail.slice(1) : naturalTail
  const keyOf = (row: Row) => row.type === 'file' ? `file:${row.index}` : row.type === 'line' ? `line:${row.key}` : `path:${row.path}`
  const render = (row: Row): ReactNode => row.type === 'path' ? <div className={css.line}>{row.path}</div>
    : row.type === 'line' ? <div className={css.line} data-search-context={row.context || undefined} data-search-column={row.column}>
      <span className={css.lineNumber}>{row.number}{row.context ? '- ' : ': '}</span>{row.line}</div>
      : <button type="button" className={css.fileHeader} aria-expanded={!row.collapsed} onClick={() => setCollapsed(previous => {
        const next = new Set(previous); if (next.has(row.index)) next.delete(row.index); else next.add(row.index); return next
      })}><span className={css.filePath}>{row.path}</span><span className={css.fileCount}>{row.count}</span></button>
  return <div className={[css.block, className].filter(Boolean).join(' ')} data-search={search.shape}>
    <div className={css.header}><span className={css.summary}>{search.shape === 'paths'
      ? labels.pathsSummary(shown, search.total, search.truncated || captureTruncated) : labels.matchesSummary(shown, search.total, search.files.length, search.truncated || captureTruncated)}</span>
      {shown > 0 ? <button type="button" className={css.copyButton} disabled={pending} onClick={onCopy}>{copied ? labels.copied : labels.copy}</button> : null}</div>
    {rows.length === 0 ? <div className={css.empty}>{labels.noResults}</div> : <div className={css.body}>
      {head.map(row => <div key={keyOf(row)} data-search-row={row.type}>{render(row)}</div>)}
      {hidden > 0 ? <button type="button" className={css.expand} aria-expanded={expanded}
        aria-label={expanded ? labels.collapseAria : labels.expandAria(hidden)} onClick={() => setExpanded(value => !value)}>
        {expanded ? labels.collapse : labels.expand(hidden)}</button> : null}
      {tailHeader ? <div data-search-row="file" key={'tail:' + keyOf(tailHeader)}>{render(tailHeader)}</div> : null}
      {tail.map(row => <div key={keyOf(row)} data-search-row={row.type}>{render(row)}</div>)}
    </div>}
  </div>
}
