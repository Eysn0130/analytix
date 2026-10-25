import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement, type RefObject } from 'react'
import { ChevronLeft, ChevronRight, Loader2, Search } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import { createPortal } from 'react-dom'
import { Tooltip } from '../chat/presentation/Tooltip'
import styles from './write-pdf-viewer.module.css'
import {
  GlobalWorkerOptions,
  TextLayer,
  getDocument,
  type PDFDocumentProxy,
  type PDFPageProxy,
  type TextContentItem
} from 'pdfjs-dist/build/pdf.mjs'
import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.mjs?url'
import type {
  WriteEditorSelectionState,
  WriteSelectionAnchorRect,
  WriteSelectionPageRect
} from './WriteMarkdownEditor'

GlobalWorkerOptions.workerSrc = pdfWorkerUrl

type Props = {
  filePath: string
  dataBase64: string
  size: number
  mtimeMs: number
  workspaceRoot: string
  viewerRef?: RefObject<HTMLDivElement | null>
  /** Only zoom moves into the caller chrome; missing/null keeps the inline control. */
  zoomControlTarget?: HTMLElement | null
  onSelectionChange: (selection: WriteEditorSelectionState) => void
}

type PageText = {
  page: number
  text: string
}

type PdfSelectionSnapshot = WriteEditorSelectionState & {
  rects?: WriteSelectionPageRect[]
}

type ViewportRect = {
  left: number
  top: number
  right: number
  bottom: number
}

function bytesFromBase64(base64: string): Uint8Array {
  const binary = window.atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return bytes
}

function unionRects(rects: DOMRect[]): WriteSelectionAnchorRect | undefined {
  if (rects.length === 0) return undefined
  let left = Number.POSITIVE_INFINITY
  let right = Number.NEGATIVE_INFINITY
  let top = Number.POSITIVE_INFINITY
  let bottom = Number.NEGATIVE_INFINITY
  for (const rect of rects) {
    left = Math.min(left, rect.left)
    right = Math.max(right, rect.right)
    top = Math.min(top, rect.top)
    bottom = Math.max(bottom, rect.bottom)
  }
  if (!Number.isFinite(left) || !Number.isFinite(right) || !Number.isFinite(top) || !Number.isFinite(bottom)) {
    return undefined
  }
  return {
    left,
    right,
    top,
    bottom,
    width: right - left,
    height: bottom - top
  }
}

function anchorRectFromDomRect(rect: DOMRect): WriteSelectionAnchorRect | undefined {
  if (!Number.isFinite(rect.left) || !Number.isFinite(rect.top) || rect.width <= 0 || rect.height <= 0) {
    return undefined
  }
  return {
    left: rect.left,
    right: rect.right,
    top: rect.top,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height
  }
}

function isSelectionBackward(selection: Selection): boolean {
  const anchor = selection.anchorNode
  const focus = selection.focusNode
  if (!anchor || !focus) return false
  if (anchor === focus) return selection.anchorOffset > selection.focusOffset
  return Boolean(anchor.compareDocumentPosition(focus) & Node.DOCUMENT_POSITION_PRECEDING)
}

function intersects(a: ViewportRect, b: DOMRect): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
}

// pdf.js text layers are made of dozens of absolutely positioned spans per
// line, plus stretched whitespace-only spans and container boxes. Painting
// `range.getClientRects()` verbatim therefore produces overlapping blotches
// and full-width trailing bands. Collect rects from real text fragments only.
const MAX_SELECTION_FRAGMENT_RECTS = 6000

function collectRangeTextRects(range: Range): DOMRect[] {
  const doc = range.startContainer.ownerDocument ?? window.document
  const walker = doc.createTreeWalker(range.commonAncestorContainer, NodeFilter.SHOW_TEXT)
  const probe = doc.createRange()
  const rects: DOMRect[] = []

  let node: Node | null
  if (range.startContainer.nodeType === Node.TEXT_NODE) {
    walker.currentNode = range.startContainer
    node = range.startContainer
  } else {
    walker.currentNode = range.startContainer
    node = walker.nextNode()
  }

  while (node && rects.length < MAX_SELECTION_FRAGMENT_RECTS) {
    if (range.comparePoint(node, 0) > 0) break
    const text = node as Text
    if (text.data.trim() && range.intersectsNode(text)) {
      probe.selectNodeContents(text)
      if (text === range.startContainer) probe.setStart(text, range.startOffset)
      if (text === range.endContainer) probe.setEnd(text, range.endOffset)
      for (const rect of probe.getClientRects()) {
        if (rect.width > 0 && rect.height > 0) rects.push(rect)
      }
    }
    if (text === range.endContainer) break
    node = walker.nextNode()
  }
  return rects
}

// Merge fragment rects into one bar per visual line (split only across large
// horizontal gaps such as column gutters) so the committed highlight reads
// like a continuous text selection instead of stacked translucent chunks.
const LINE_MERGE_WINDOW = 6

function mergeRectsIntoLineBars(rects: DOMRect[]): ViewportRect[] {
  if (rects.length === 0) return []
  const sorted = [...rects].sort((a, b) => a.top - b.top || a.left - b.left)
  type LineBucket = { top: number; bottom: number; segments: Array<{ left: number; right: number }> }
  const lines: LineBucket[] = []

  for (const rect of sorted) {
    let target: LineBucket | null = null
    for (let index = lines.length - 1; index >= 0 && index >= lines.length - LINE_MERGE_WINDOW; index -= 1) {
      const line = lines[index]
      const overlap = Math.min(line.bottom, rect.bottom) - Math.max(line.top, rect.top)
      if (overlap > 0 && overlap >= Math.min(line.bottom - line.top, rect.height) * 0.45) {
        target = line
        break
      }
    }
    if (target) {
      target.top = Math.min(target.top, rect.top)
      target.bottom = Math.max(target.bottom, rect.bottom)
      target.segments.push({ left: rect.left, right: rect.right })
    } else {
      lines.push({ top: rect.top, bottom: rect.bottom, segments: [{ left: rect.left, right: rect.right }] })
    }
  }

  const bars: ViewportRect[] = []
  for (const line of lines) {
    const gapLimit = Math.max(10, (line.bottom - line.top) * 0.85)
    const segments = [...line.segments].sort((a, b) => a.left - b.left)
    let current = { ...segments[0] }
    for (let index = 1; index < segments.length; index += 1) {
      const segment = segments[index]
      if (segment.left - current.right <= gapLimit) {
        current.right = Math.max(current.right, segment.right)
      } else {
        bars.push({ left: current.left, right: current.right, top: line.top, bottom: line.bottom })
        current = { ...segment }
      }
    }
    bars.push({ left: current.left, right: current.right, top: line.top, bottom: line.bottom })
  }
  return bars
}

function pageRectsFromViewportRects(root: HTMLElement, rects: ViewportRect[]): WriteSelectionPageRect[] {
  const pages = Array.from(root.querySelectorAll<HTMLElement>('[data-write-pdf-page]')).map((element) => ({
    element,
    page: Number(element.dataset.writePdfPage ?? ''),
    rect: element.getBoundingClientRect()
  })).filter((page) => Number.isFinite(page.page) && page.page > 0)
  const out: WriteSelectionPageRect[] = []

  for (const rect of rects) {
    const page = pages.find((item) => intersects(rect, item.rect))
    if (!page) continue
    const left = Math.max(rect.left, page.rect.left)
    const right = Math.min(rect.right, page.rect.right)
    const top = Math.max(rect.top, page.rect.top)
    const bottom = Math.min(rect.bottom, page.rect.bottom)
    if (right <= left || bottom <= top) continue
    out.push({
      page: page.page,
      x: left - page.rect.left,
      y: top - page.rect.top,
      width: right - left,
      height: bottom - top
    })
  }
  return out
}

function pageFromNode(node: Node | null): number | null {
  const element = node instanceof Element ? node : node?.parentElement
  const pageElement = element?.closest<HTMLElement>('[data-write-pdf-page]')
  const page = Number(pageElement?.dataset.writePdfPage ?? '')
  return Number.isFinite(page) && page > 0 ? page : null
}

function emptyPdfSelection(): WriteEditorSelectionState {
  return {
    text: '',
    ranges: [],
    charCount: 0,
    sourceKind: 'pdf'
  }
}

function selectionFromPdf(root: HTMLElement): PdfSelectionSnapshot {
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return emptyPdfSelection()
  const anchorInside = selection.anchorNode ? root.contains(selection.anchorNode) : false
  const focusInside = selection.focusNode ? root.contains(selection.focusNode) : false
  if (!anchorInside || !focusInside) return emptyPdfSelection()

  const text = selection.toString().trim()
  if (!text) return emptyPdfSelection()
  const range = selection.getRangeAt(0)
  const pageA = pageFromNode(selection.anchorNode)
  const pageB = pageFromNode(selection.focusNode)
  const pageStart = Math.min(pageA ?? pageB ?? 1, pageB ?? pageA ?? 1)
  const pageEnd = Math.max(pageA ?? pageB ?? pageStart, pageB ?? pageA ?? pageStart)
  const textRects = collectRangeTextRects(range)
  const backward = isSelectionBackward(selection)
  const focusRect = textRects.length > 0
    ? textRects[backward ? 0 : textRects.length - 1]
    : null
  const rects = pageRectsFromViewportRects(root, mergeRectsIntoLineBars(textRects))
  const anchorRect = (focusRect ? anchorRectFromDomRect(focusRect) : undefined)
    ?? unionRects(textRects)
    ?? anchorRectFromDomRect(range.getBoundingClientRect())
  return {
    text,
    ranges: [{
      from: 0,
      to: text.length,
      startLine: pageStart,
      startColumn: 1,
      endLine: pageEnd,
      endColumn: text.length + 1,
      text,
      charCount: text.length,
      page: pageStart
    }],
    charCount: text.length,
    sourceKind: 'pdf',
    pageStart,
    pageEnd,
    anchorRect,
    rects
  }
}

function formatSize(size: number): string {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${Math.round(size / 1024)} KB`
  return `${size} B`
}

function WritePdfPage({
  document,
  pageNumber,
  scale,
  selectionRects,
  onPageText
}: {
  document: PDFDocumentProxy
  pageNumber: number
  scale: number
  selectionRects: WriteSelectionPageRect[]
  onPageText: (page: PageText) => void
}): ReactElement {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const textLayerRef = useRef<HTMLDivElement | null>(null)
  const [pageSize, setPageSize] = useState<{ width: number; height: number } | null>(null)

  useEffect(() => {
    let cancelled = false
    let renderTask: { cancel: () => void; promise: Promise<unknown> } | null = null
    let textLayerRenderer: TextLayer | null = null

    const renderPage = async (): Promise<void> => {
      const canvas = canvasRef.current
      const textLayer = textLayerRef.current
      if (!canvas || !textLayer) return
      const page: PDFPageProxy = await document.getPage(pageNumber)
      if (cancelled) return
      const viewport = page.getViewport({ scale })
      const outputScale = Math.max(1, window.devicePixelRatio || 1)
      canvas.width = Math.floor(viewport.width * outputScale)
      canvas.height = Math.floor(viewport.height * outputScale)
      canvas.style.width = `${viewport.width}px`
      canvas.style.height = `${viewport.height}px`
      setPageSize({ width: viewport.width, height: viewport.height })

      const context = canvas.getContext('2d')
      if (!context) return
      context.setTransform(outputScale, 0, 0, outputScale, 0, 0)
      const task = page.render({ canvasContext: context, viewport })
      renderTask = task
      await task.promise
      if (cancelled) return

      textLayer.replaceChildren()
      const textContent = await page.getTextContent()
      if (cancelled) return
      // pdf.js uses this CSS factor for text sizes and marked-content bounds.
      // It follows the page viewport, independently of the canvas pixel ratio.
      textLayer.style.setProperty('--total-scale-factor', String(viewport.scale * viewport.userUnit))
      textLayerRenderer = new TextLayer({
        textContentSource: textContent,
        container: textLayer,
        viewport
      })
      await textLayerRenderer.render()
      if (!cancelled) {
        // Whitespace-only spans are often stretched to absurd widths by the
        // text layer; flag them so native selection skips painting them.
        for (const span of Array.from(textLayer.querySelectorAll<HTMLElement>('span'))) {
          if (!(span.textContent ?? '').trim()) span.classList.add(styles.whitespace)
        }
        const text = textContent.items
          .map((item: TextContentItem) => (typeof item.str === 'string' ? item.str : ''))
          .filter(Boolean)
          .join(' ')
          .trim()
        onPageText({ page: pageNumber, text })
      }
      page.cleanup()
    }

    void renderPage().catch(() => undefined)
    return () => {
      cancelled = true
      renderTask?.cancel()
      textLayerRenderer?.cancel()
    }
  }, [document, onPageText, pageNumber, scale])

  return (
    <div
      className={styles.page}
      data-write-pdf-page={pageNumber}
      ref={(node) => {
        if (node && pageSize) {
          node.style.width = `${pageSize.width}px`
          node.style.height = `${pageSize.height}px`
        }
      }}
    >
      <canvas ref={canvasRef} className={styles.canvas} />
      <div ref={textLayerRef} className={`${styles.textLayer} textLayer`} />
      <div className={styles.overlayLayer} aria-hidden="true">
        {selectionRects.map((rect, index) => (
          <span
            key={`${pageNumber}-${index}-${rect.x}-${rect.y}`}
            className={styles.selectionRect}
            style={{
              left: rect.x,
              top: rect.y,
              width: rect.width,
              height: rect.height
            }}
          />
        ))}
      </div>
    </div>
  )
}

export function WritePdfViewer({
  filePath,
  dataBase64,
  size,
  mtimeMs,
  workspaceRoot,
  viewerRef,
  zoomControlTarget,
  onSelectionChange
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const localViewerRef = useRef<HTMLDivElement | null>(null)
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const pageRefs = useRef<Map<number, HTMLDivElement>>(new Map())
  const selectionSyncTimerRef = useRef<number | null>(null)
  const scrollRafRef = useRef<number | null>(null)
  const [pdfDocument, setPdfDocument] = useState<PDFDocumentProxy | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [zoomMode, setZoomMode] = useState<number | 'fit'>('fit')
  const [fitScale, setFitScale] = useState(1)
  const scale = zoomMode === 'fit' ? fitScale : zoomMode
  const [pageInput, setPageInput] = useState('1')
  const [currentPage, setCurrentPage] = useState(1)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const searchTriggerRef = useRef<HTMLButtonElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const searchPanelRef = useRef<HTMLDivElement>(null)
  const [searchIndex, setSearchIndex] = useState(0)
  const [pageTexts, setPageTexts] = useState<PageText[]>([])
  const [committedSelectionRects, setCommittedSelectionRects] = useState<WriteSelectionPageRect[]>([])
  // While the native selection is visible we keep the committed overlay
  // hidden; it only takes over once focus moves away (e.g. into the assist
  // popup) and the browser drops the native highlight.
  const [liveSelection, setLiveSelection] = useState(false)
  const pageCount = pdfDocument?.numPages ?? 0
  const rootRef = viewerRef ?? localViewerRef
  const currentPageRef = useRef(currentPage); currentPageRef.current = currentPage

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    setPdfDocument(null)
    setPageTexts([])
    setCommittedSelectionRects([])
    onSelectionChange(emptyPdfSelection())
    const task = getDocument({
      data: bytesFromBase64(dataBase64),
      isEvalSupported: false
    })
    void task.promise.then((pdf) => {
      if (cancelled) {
        void pdf.destroy()
        return
      }
      setPdfDocument(pdf)
      setPageInput('1')
      setCurrentPage(1)
      setLoading(false)
    }).catch((reason: unknown) => {
      if (!cancelled) {
        setError(reason instanceof Error ? reason.message : String(reason))
        setLoading(false)
      }
    })
    return () => {
      cancelled = true
      task.destroy()
    }
  }, [dataBase64, filePath, mtimeMs, onSelectionChange])

  useEffect(() => {
    return () => {
      if (pdfDocument) void pdfDocument.destroy()
    }
  }, [pdfDocument])

  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller || !pdfDocument) return
    let cancelled = false
    let pageWidth = 0
    const updateFit = (): void => {
      if (cancelled || !pageWidth) return
      const style = window.getComputedStyle(scroller)
      const padding = (Number.parseFloat(style.paddingLeft) || 0) + (Number.parseFloat(style.paddingRight) || 0)
      const available = (scroller.clientWidth || scroller.getBoundingClientRect().width) - padding
      if (available > 0) setFitScale(available / pageWidth)
    }
    void pdfDocument.getPage(1).then(page => {
      if (cancelled) return
      pageWidth = page.getViewport({ scale: 1 }).width
      if (Number.isFinite(pageWidth) && pageWidth > 0) updateFit()
    }).catch(() => undefined)
    const observer = new ResizeObserver(updateFit)
    observer.observe(scroller)
    window.addEventListener('resize', updateFit)
    return () => { cancelled = true; observer.disconnect(); window.removeEventListener('resize', updateFit) }
  }, [pdfDocument])

  useEffect(() => {
    setCommittedSelectionRects([])
    setLiveSelection(false)
    onSelectionChange(emptyPdfSelection())
  }, [onSelectionChange, scale])

  const searchMatches = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return []
    return pageTexts
      .filter((page) => page.text.toLowerCase().includes(query))
      .map((page) => page.page)
      .sort((a, b) => a - b)
  }, [pageTexts, searchQuery])
  const allPageTextLoaded = pageCount > 0 && pageTexts.length >= pageCount
  const pdfHasText = pageTexts.some((page) => page.text.trim().length > 0)
  const committedRectsByPage = useMemo(() => {
    const byPage = new Map<number, WriteSelectionPageRect[]>()
    for (const rect of committedSelectionRects) {
      const pageRects = byPage.get(rect.page)
      if (pageRects) pageRects.push(rect)
      else byPage.set(rect.page, [rect])
    }
    return byPage
  }, [committedSelectionRects])

  const updatePageText = useCallback((page: PageText): void => {
    setPageTexts((current) => {
      const existing = current.find((item) => item.page === page.page)
      if (existing?.text === page.text) return current
      const next = current.filter((item) => item.page !== page.page)
      next.push(page)
      return next.sort((a, b) => a.page - b.page)
    })
  }, [])

  const scrollToPage = useCallback((page: number): void => {
    if (!Number.isFinite(page)) {
      setPageInput(String(currentPageRef.current))
      return
    }
    const clamped = Math.max(1, Math.min(pageCount || 1, Math.round(page)))
    setCurrentPage(clamped)
    setPageInput(String(clamped))
    pageRefs.current.get(clamped)?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  }, [pageCount])

  const updateCurrentPageFromScroll = useCallback((): void => {
    const scroller = scrollerRef.current
    if (!scroller || pageRefs.current.size === 0) return
    const scrollerRect = scroller.getBoundingClientRect()
    const targetY = scrollerRect.top + scrollerRect.height * 0.42
    let bestPage = 1
    let bestDistance = Number.POSITIVE_INFINITY

    pageRefs.current.forEach((node, page) => {
      const rect = node.getBoundingClientRect()
      const distance = targetY >= rect.top && targetY <= rect.bottom
        ? 0
        : Math.min(Math.abs(targetY - rect.top), Math.abs(targetY - rect.bottom))
      if (distance < bestDistance) {
        bestDistance = distance
        bestPage = page
      }
    })

    setCurrentPage((value) => value === bestPage ? value : bestPage)
    setPageInput((value) => value === String(bestPage) ? value : String(bestPage))
  }, [])

  const schedulePageSync = useCallback((): void => {
    if (scrollRafRef.current != null) return
    scrollRafRef.current = window.requestAnimationFrame(() => {
      scrollRafRef.current = null
      updateCurrentPageFromScroll()
    })
  }, [updateCurrentPageFromScroll])

  const jumpSearch = (direction: 1 | -1): void => {
    if (searchMatches.length === 0) return
    const nextIndex = (searchIndex + direction + searchMatches.length) % searchMatches.length
    setSearchIndex(nextIndex)
    scrollToPage(searchMatches[nextIndex])
  }

  const searchMatchKey = useMemo(() => searchMatches.join(','), [searchMatches])
  const firstSearchMatch = searchMatches[0] ?? null
  useEffect(() => {
    setSearchIndex(0)
    if (firstSearchMatch != null) scrollToPage(firstSearchMatch)
  }, [firstSearchMatch, scrollToPage, searchMatchKey])

  const syncSelection = useCallback((): void => {
    const root = rootRef.current
    if (!root) return
    const next = selectionFromPdf(root)
    onSelectionChange(next)
    if (next.text.trim()) {
      setCommittedSelectionRects(next.rects ?? [])
      setLiveSelection(true)
    } else {
      setCommittedSelectionRects([])
      setLiveSelection(false)
    }
  }, [onSelectionChange, rootRef])

  const syncSelectionSoon = useCallback((): void => {
    if (selectionSyncTimerRef.current != null) {
      window.clearTimeout(selectionSyncTimerRef.current)
    }
    selectionSyncTimerRef.current = window.setTimeout(() => {
      selectionSyncTimerRef.current = null
      syncSelection()
    }, 0)
  }, [syncSelection])

  useEffect(() => {
    const handleSelectionChange = (): void => {
      const root = rootRef.current
      const selection = window.getSelection()
      if (!root) return
      if (!selection || selection.rangeCount === 0) {
        setLiveSelection(false)
        return
      }
      const anchorInside = selection.anchorNode ? root.contains(selection.anchorNode) : false
      const focusInside = selection.focusNode ? root.contains(selection.focusNode) : false
      if (anchorInside || focusInside) {
        syncSelectionSoon()
      } else {
        // Selection moved elsewhere (e.g. into the assist popup input): keep
        // the committed snapshot and let the overlay take over the highlight.
        setLiveSelection(false)
      }
    }
    window.document.addEventListener('selectionchange', handleSelectionChange)
    return () => {
      window.document.removeEventListener('selectionchange', handleSelectionChange)
      if (selectionSyncTimerRef.current != null) {
        window.clearTimeout(selectionSyncTimerRef.current)
        selectionSyncTimerRef.current = null
      }
    }
  }, [rootRef, syncSelectionSoon])

  useEffect(() => {
    return () => {
      if (scrollRafRef.current != null) {
        window.cancelAnimationFrame(scrollRafRef.current)
        scrollRafRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    if (!searchOpen) return
    searchInputRef.current?.focus()
    const dismiss = (event: PointerEvent): void => {
      if (!(event.target instanceof Node)) return
      if (!searchPanelRef.current?.contains(event.target) && !searchTriggerRef.current?.contains(event.target)) setSearchOpen(false)
    }
    window.document.addEventListener('pointerdown', dismiss)
    return () => window.document.removeEventListener('pointerdown', dismiss)
  }, [searchOpen])

  const beginPdfSelection = useCallback((): void => {
    setCommittedSelectionRects([])
    setLiveSelection(false)
    onSelectionChange(emptyPdfSelection())
  }, [onSelectionChange])

  const zoomControl = <select
    className={styles.zoomSelect}
    data-write-pdf-zoom
    aria-label={`${t('writePdfZoomOut')} / ${t('writePdfZoomIn')}`}
    value={String(zoomMode)}
    disabled={!pdfDocument}
    onChange={event => {
      const choice = event.target.value
      if (choice === 'fit') { setZoomMode('fit'); return }
      setZoomMode(choice === 'out' ? Math.max(.25, Math.min(4, Number((scale - .1).toFixed(2))))
        : choice === 'in' ? Math.max(.25, Math.min(4, Number((scale + .1).toFixed(2))))
        : Number.isFinite(Number(choice)) ? Math.max(.25, Math.min(4, Number(choice))) : zoomMode)
    }}
  >
    <option value="fit">{t('writeImageFitShort')}</option>
    <option value="out" disabled={scale <= .25}>{t('writePdfZoomOut')}</option>
    {[...new Set([.25, .5, 1, 2, ...(zoomMode === 'fit' ? [] : [zoomMode])])].sort((a, b) => a - b).map(value => <option key={value} value={String(value)}>{Math.round(value * 100)}%</option>)}
    <option value="in" disabled={scale >= 4}>{t('writePdfZoomIn')}</option>
  </select>

  return (
    <div
      ref={rootRef}
      className={styles.viewer}
      aria-label={`${workspaceRoot ? filePath.replace(`${workspaceRoot}/`, '') : filePath} · ${formatSize(size)}`}
      data-live-selection={liveSelection ? '' : undefined}
    >
      <div className={styles.toolbar} role="toolbar" aria-label={t('writePdfPreview')}>
        <span className={styles.metadata}>{formatSize(size)}</span>
        <div className={styles.pageControls}>
          <Tooltip bubbleClassName={styles.tooltipBubble} label={t('writePdfPrevPage')} side="bottom" portal><button
            type="button" className={styles.iconButton} aria-label={t('writePdfPrevPage')}
            onClick={() => scrollToPage(currentPage - 1)} disabled={!pageCount || currentPage <= 1}
          ><ChevronLeft /></button></Tooltip>
          <form className={styles.pageForm} onSubmit={event => { event.preventDefault(); scrollToPage(Number(pageInput)) }}>
            <input className={styles.pageInput} value={pageInput} aria-label={t('writePdfPageInput')}
              inputMode="numeric" disabled={!pageCount} onChange={event => setPageInput(event.target.value)} />
            <span>/ {pageCount || '-'}</span>
          </form>
          <Tooltip bubbleClassName={styles.tooltipBubble} label={t('writePdfNextPage')} side="bottom" portal><button
            type="button" className={styles.iconButton} aria-label={t('writePdfNextPage')}
            onClick={() => scrollToPage(currentPage + 1)} disabled={!pageCount || currentPage >= pageCount}
          ><ChevronRight /></button></Tooltip>
        </div>
        {zoomControlTarget ? createPortal(zoomControl, zoomControlTarget) : zoomControl}
        <Tooltip bubbleClassName={styles.tooltipBubble} label={t('writePdfSearchPlaceholder')} side="bottom" portal><button
          ref={searchTriggerRef} type="button" className={styles.iconButton}
          aria-label={t('writePdfSearchPlaceholder')} aria-expanded={searchOpen}
          disabled={!pdfDocument} onClick={() => setSearchOpen(value => !value)}
        ><Search /></button></Tooltip>
        {searchOpen ? <div ref={searchPanelRef} className={styles.searchPanel} role="search"
          aria-label={t('writePdfSearchPlaceholder')} onKeyDown={event => {
            if (event.key === 'Escape' && !event.nativeEvent.isComposing) {
              event.preventDefault(); event.stopPropagation(); setSearchOpen(false); searchTriggerRef.current?.focus()
            }
          }}>
          <form className={styles.searchForm} onSubmit={event => { event.preventDefault(); jumpSearch(1) }}>
            <Search aria-hidden="true" />
            <input ref={searchInputRef} className={styles.searchInput} value={searchQuery}
              aria-label={t('writePdfSearchPlaceholder')} placeholder={t('writePdfSearchPlaceholder')}
              onChange={event => setSearchQuery(event.target.value)} />
            <span className={styles.matchCount} role="status">{searchQuery.trim() ? `${searchMatches.length ? searchIndex + 1 : 0}/${searchMatches.length}` : ''}</span>
            <Tooltip bubbleClassName={styles.tooltipBubble} label={t('writePdfPrevMatch')} side="bottom" portal><button type="button"
              className={styles.iconButton} aria-label={t('writePdfPrevMatch')}
              disabled={!searchMatches.length} onClick={() => jumpSearch(-1)}><ChevronLeft /></button></Tooltip>
            <Tooltip bubbleClassName={styles.tooltipBubble} label={t('writePdfNextMatch')} side="bottom" portal><button type="button"
              className={styles.iconButton} aria-label={t('writePdfNextMatch')}
              disabled={!searchMatches.length} onClick={() => jumpSearch(1)}><ChevronRight /></button></Tooltip>
          </form>
        </div> : null}
      </div>
      <div ref={scrollerRef} className={`${styles.scroller} ds-page-scroll-edge`} data-write-pdf-scroller
        onPointerDown={beginPdfSelection} onPointerUp={syncSelectionSoon} onMouseUp={syncSelectionSoon}
        onKeyUp={syncSelectionSoon} onScroll={schedulePageSync}>
        {loading ? <div className={styles.state} role="status"><Loader2 className="animate-spin" />{t('writePdfLoading')}</div>
          : error ? <div className={styles.state} role="alert">{t('writePdfLoadFailed', { message: error })}</div>
          : pdfDocument ? <div className={styles.pages}>
            {allPageTextLoaded && !pdfHasText ? <div className={styles.noText} role="status">{t('writePdfNoTextLayer')}</div> : null}
            {Array.from({ length: pdfDocument.numPages }, (_, index) => index + 1).map(pageNumber => <div
              key={pageNumber} ref={node => { if (node) pageRefs.current.set(pageNumber, node); else pageRefs.current.delete(pageNumber) }}>
              <WritePdfPage document={pdfDocument} pageNumber={pageNumber} scale={scale}
                selectionRects={committedRectsByPage.get(pageNumber) ?? []} onPageText={updatePageText} />
              <div className={styles.pageLabel}>{t('writePdfPageLabel', { page: pageNumber })}</div>
            </div>)}
          </div> : null}
      </div>
    </div>
  )
}
