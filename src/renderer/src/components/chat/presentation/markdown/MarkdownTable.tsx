import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactElement } from 'react'
import { createPortal } from 'react-dom'
import { Check, Copy, Download, Expand, X } from '../../../../design/AnalytixUiIcons'
import { modalFocusableElements, useModalFocus } from '../../../../hooks/use-modal-focus'
import { ActionMenuItem } from '../../../common/ActionMenu'
import { downloadTextFile } from '../CodeSourceActions'
import { Tooltip } from '../Tooltip'
import { writeClipboard } from '../clipboard'
import { tableDataToCsv, tableDataToHtml, tableDataToMarkdown, tableDataToTsv, type MarkdownTableData } from './table-data'
import css from './MarkdownText.module.css'
import card from '../CodeCard.module.css'

export interface MarkdownTableLabels {
  title: string; copy: string; copied: string; copyFailed: string; download: string; downloadFailed: string
  fullscreen: string; close: string; csv: string; tsv: string; markdown: string; exportHint: string
}
type Format = 'csv' | 'tsv' | 'markdown'
type ControlsProps = {
  labels: MarkdownTableLabels; busy: boolean; copied: boolean; pending: boolean; modal?: boolean
  onCopy: (format: Format) => void; onDownload: (format: Exclude<Format, 'tsv'>) => void; onFullscreen?: (opener: HTMLButtonElement) => void
}

function TableControls({ labels, busy, copied, pending, modal = false, onCopy, onDownload, onFullscreen }: ControlsProps) {
  const [menu, setMenu] = useState<'copy' | 'download' | null>(null)
  const [position, setPosition] = useState<{ left: number; top: number; maxHeight: number } | null>(null)
  const entry = useRef<'first' | 'last'>('first')
  const rootRef = useRef<HTMLDivElement>(null), menuRef = useRef<HTMLDivElement>(null)
  const copyRef = useRef<HTMLButtonElement>(null), downloadRef = useRef<HTMLButtonElement>(null)
  const id = useId()
  const trigger = (): HTMLButtonElement | null => menu === 'copy' ? copyRef.current : downloadRef.current
  const close = (restore = false): void => {
    if (restore) trigger()?.focus({ preventScroll: true })
    setMenu(null); setPosition(null)
  }
  useLayoutEffect(() => {
    if (!menu) return
    const update = (): void => {
      const button = menu === 'copy' ? copyRef.current : downloadRef.current
      if (!button || !menuRef.current) return
      const scale = parseFloat(getComputedStyle(document.body).zoom) || 1
      const anchor = button.getBoundingClientRect(), box = menuRef.current.getBoundingClientRect()
      const left = Math.max(8, Math.min(anchor.right - box.width, window.innerWidth - box.width - 8))
      const top = anchor.bottom + 6 + box.height <= window.innerHeight - 8 ? anchor.bottom + 6
        : Math.max(8, anchor.top - box.height - 6)
      setPosition({ left: left / scale, top: top / scale, maxHeight: (window.innerHeight - 16) / scale })
    }
    update()
    const frame = window.requestAnimationFrame(() => {
      const items = menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
      ;(entry.current === 'last' ? items?.[items.length - 1] : items?.[0])?.focus({ preventScroll: true })
    })
    window.addEventListener('resize', update); window.addEventListener('scroll', update, true)
    return () => { window.cancelAnimationFrame(frame); window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true) }
  }, [menu, labels])
  useEffect(() => {
    if (!menu) return
    const outside = (event: Event): void => {
      if (event.target instanceof Node && (rootRef.current?.contains(event.target) || menuRef.current?.contains(event.target))) return
      setMenu(null); setPosition(null)
    }
    window.addEventListener('pointerdown', outside); window.addEventListener('focusin', outside)
    return () => { window.removeEventListener('pointerdown', outside); window.removeEventListener('focusin', outside) }
  }, [menu])
  useEffect(() => { if (busy) { setMenu(null); setPosition(null) } }, [busy])
  const open = (kind: 'copy' | 'download', last = false): void => {
    if (busy || (kind === 'copy' && pending)) return
    if (menu === kind) {
      const items = menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
      ;(last ? items?.[items.length - 1] : items?.[0])?.focus({ preventScroll: true })
      return
    }
    entry.current = last ? 'last' : 'first'
    setPosition(null); setMenu(kind)
  }
  const onTriggerKey = (event: KeyboardEvent<HTMLButtonElement>, kind: 'copy' | 'download'): void => {
    if (event.nativeEvent.isComposing || event.defaultPrevented) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault(); event.stopPropagation(); open(kind, event.key === 'ArrowUp')
    }
  }
  const onMenuKey = (event: KeyboardEvent<HTMLDivElement>): void => {
    if (event.nativeEvent.isComposing || event.defaultPrevented) return
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); return }
    if (event.key === 'Tab') {
      const button = trigger()
      close(true)
      if (modal && button) {
        event.preventDefault()
        const dialog = rootRef.current?.closest<HTMLElement>('[role="dialog"]')
        const items = (dialog ? modalFocusableElements(dialog) : [])
          .filter(item => !item.closest('[data-markdown-table-menu]'))
        const index = items.indexOf(button)
        items[(index + (event.shiftKey ? -1 : 1) + items.length) % items.length]?.focus({ preventScroll: true })
      }
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault(); event.stopPropagation()
    const items = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')]
    const index = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
      : (index + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
    items[next]?.focus({ preventScroll: true })
  }
  const formats: Format[] = menu === 'download' ? ['csv', 'markdown'] : ['csv', 'tsv', 'markdown']
  const popup = menu ? <div ref={menuRef} id={id} role="menu" data-markdown-table-menu
    aria-label={menu === 'copy' ? labels.copy : labels.download} className={css.tableMenu}
    style={position ? position : { visibility: 'hidden' }} onKeyDown={onMenuKey}>
    {formats.map(format => <ActionMenuItem key={format} icon={menu === 'copy' ? <Copy size={14} /> : <Download size={14} />}
      label={labels[format]} title={format === 'markdown' ? undefined : labels.exportHint} onClick={() => {
        const kind = menu; close(true)
        if (kind === 'copy') onCopy(format)
        else if (format !== 'tsv') onDownload(format)
      }} />)}
  </div> : null
  return <div ref={rootRef} className={css.tableActions}>
    <Tooltip label={copied ? labels.copied : labels.copy} portal side="top">
      <button ref={copyRef} type="button" className={card.action} disabled={busy} aria-disabled={pending || undefined} aria-busy={pending}
        aria-label={copied ? labels.copied : labels.copy} aria-haspopup="menu" aria-expanded={menu === 'copy'} aria-controls={menu === 'copy' ? id : undefined}
        onKeyDown={event => onTriggerKey(event, 'copy')} onClick={() => menu === 'copy' ? close() : open('copy')}>
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </button>
    </Tooltip>
    <Tooltip label={labels.download} portal side="top">
      <button ref={downloadRef} type="button" className={card.action} disabled={busy} aria-label={labels.download}
        aria-haspopup="menu" aria-expanded={menu === 'download'} aria-controls={menu === 'download' ? id : undefined}
        onKeyDown={event => onTriggerKey(event, 'download')} onClick={() => menu === 'download' ? close() : open('download')}><Download size={14} /></button>
    </Tooltip>
    {onFullscreen && <Tooltip label={labels.fullscreen} portal side="top">
      <button type="button" className={card.action} disabled={busy} aria-label={labels.fullscreen} onClick={event => { close(); onFullscreen(event.currentTarget) }}><Expand size={14} /></button>
    </Tooltip>}
    {modal ? popup : popup && createPortal(popup, document.body)}
  </div>
}

function MountedTableModal({ labels, table, controls, feedback, opener, onClose }: {
  labels: MarkdownTableLabels; table: ReactElement; controls: ControlsProps; feedback: string; opener: HTMLButtonElement | null; onClose: () => void
}) {
  const ref = useModalFocus(onClose, true, true, opener), titleId = useId()
  return createPortal(<div ref={ref} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}
    className={`${css.tableModal} ${css.markdown} ds-assistant-markdown`} onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
    <section className={css.tableModalPanel}>
      <header className={css.tableModalHeader}><h2 id={titleId}>{labels.title}</h2>
        <TableControls {...controls} modal />
        <button type="button" className={card.action} data-modal-autofocus aria-label={labels.close} onClick={onClose}><X size={16} /></button>
        <span role="status" className="sr-only">{feedback}</span>
      </header>
      <div className={`${css.tableScroll} ${css.tableModalScroll}`} tabIndex={0} aria-label={labels.title}>{table}</div>
    </section>
  </div>, document.body)
}

export function MarkdownTable({ data, table, labels, wide, busy }: {
  data: MarkdownTableData; table: ReactElement; labels: MarkdownTableLabels; wide: boolean; busy: boolean
}) {
  const [fullscreen, setFullscreen] = useState(false), [copied, setCopied] = useState(false)
  const [pending, setPending] = useState(false), [feedback, setFeedback] = useState('')
  const opener = useRef<HTMLButtonElement | null>(null)
  const revision = JSON.stringify(data), generation = useRef(0), timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    generation.current += 1; setCopied(false); setPending(false); setFeedback(''); setFullscreen(false)
    if (timer.current !== null) clearTimeout(timer.current)
    return () => { generation.current += 1; if (timer.current !== null) clearTimeout(timer.current) }
  }, [revision, busy])
  const copy = async (format: Format): Promise<void> => {
    if (busy || pending) return
    const token = generation.current
    setPending(true)
    const text = format === 'csv' ? tableDataToCsv(data) : format === 'tsv' ? tableDataToTsv(data) : tableDataToMarkdown(data)
    const ok = await writeClipboard(text, tableDataToHtml(data))
    if (token !== generation.current) return
    setPending(false); setCopied(ok); setFeedback(ok ? labels.copied : labels.copyFailed)
    if (timer.current !== null) clearTimeout(timer.current)
    timer.current = setTimeout(() => { setCopied(false); setFeedback('') }, 2000)
  }
  const download = (format: 'csv' | 'markdown'): void => {
    try {
      if (format === 'csv') downloadTextFile(`\uFEFF${tableDataToCsv(data)}`, 'table.csv', 'text/csv;charset=utf-8')
      else downloadTextFile(tableDataToMarkdown(data), 'table.md', 'text/markdown;charset=utf-8')
    } catch { setFeedback(labels.downloadFailed) }
  }
  const controls: ControlsProps = { labels, busy: busy || data.headers.length === 0, copied, pending,
    onCopy: format => { void copy(format) }, onDownload: download }
  return <div className={css.tableBlock} data-markdown-table>
    <div inert={fullscreen}>
      <TableControls {...controls} onFullscreen={button => { opener.current = button; setFullscreen(true) }} />
      <div className={`${css.tableScroll} ${wide ? 'md-table-wide' : css.tableFill}`} tabIndex={0} aria-label={labels.title}>{table}</div>
      <span role="status" className="sr-only">{feedback}</span>
    </div>
    {fullscreen && <MountedTableModal labels={labels} table={table} controls={controls} feedback={feedback} opener={opener.current} onClose={() => setFullscreen(false)} />}
  </div>
}
