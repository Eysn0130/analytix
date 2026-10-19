import type { WorkspaceFileReadResult, WorkspaceFileTarget, WorkspaceImageReadResult, WorkspacePdfReadResult, WorkspaceExcelReadResult } from '@shared/workspace-file'
import { ImagePreview } from './chat/presentation/ImagePreview'
import { WritePdfViewer } from './write/WritePdfViewer'
import { WorkspaceViewerHeader, WorkspaceViewerIconButton, WorkspaceViewerMenu, type WorkspaceViewerControls } from './workbench/WorkspaceViewerHeader'
import { LazyExcelBody } from './file-preview/excel/LazyExcelBody'
import previewStyles from './file-preview/excel/ExcelBody.module.css'
import {
  Check,
  Code2,
  Copy,
  Download,
  Eye,
  ExternalLink,
  FileCode2,
  Loader2,
  PanelRightClose,
  X
} from '../design/AnalytixUiIcons'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { harden } from 'rehype-harden'
import type { PluggableList } from 'unified'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type CSSProperties,
  type ReactElement,
  type ReactNode
} from 'react'
import { toolResultLocalEffectResultSchemaV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
import { useTranslation } from 'react-i18next'
import { formatFilePathForDisplay } from '../lib/diff-stats'
import { openWorkspacePathInEditor } from '../lib/open-workspace-path'
import {
  highlightCodeHtml,
  languageFromFilePath,
  renderFallbackCodeHtml
} from '../lib/code-highlighting'
import { createHtmlEmbedHandle, type HtmlEmbedHandle } from '../write/html-embed-dom'
import { isWorkspaceTextPreviewPath } from '../lib/workspace-text-preview'
import {
  initialWriteMarkdownImageSrc,
  loadWriteMarkdownImage
} from '../write/markdown-image'

type Props = {
  target: WorkspaceFileTarget | null
  openTargets?: WorkspaceFileTarget[]
  tabbedWorkspace?: boolean
  workspaceRoot: string
  className?: string
  onSelectTarget?: (target: WorkspaceFileTarget) => void
  onCloseTarget?: (target: WorkspaceFileTarget) => void
  onClose: () => void
  viewerControls?: WorkspaceViewerControls
  visible?: boolean
}

function WorkspaceHtmlBody({ path, workspaceRoot, control, protectedPresent }: {
  path: string; workspaceRoot: string; control: { current: HtmlEmbedHandle | null }; protectedPresent?: WorkspaceFileTarget['protectedPresent']
}): ReactElement {
  const host = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const handle = createHtmlEmbedHandle({ rawSrc: path, alt: '', filePath: path, workspaceRoot, mode: 'workspace-html', protectedPresent })
    control.current = handle
    handle.element.style.height = '100%'
    handle.element.style.display = 'flex'
    handle.element.style.flexDirection = 'column'
    host.current?.replaceChildren(handle.element)
    handle.run()
    return () => {
      handle.dispose()
      if (control.current === handle) control.current = null
    }
  }, [path, workspaceRoot, control, protectedPresent])
  return <div ref={host} data-workspace-html-preview="true" className="min-h-0 flex-1 overflow-hidden" />
}

type PresentMediaResult = { owner: WorkspaceFileTarget; attempt: number } & (
  | { kind: 'image'; value: WorkspaceImageReadResult }
  | { kind: 'pdf'; value: WorkspacePdfReadResult }
  | { kind: 'excel'; value: WorkspaceExcelReadResult }
)
// Protected PDF selection remains local to the passive viewer.
const ignoreProtectedPdfSelection = (): void => undefined
const COPY_RESET_MS = 1400
const MARKDOWN_DEFAULT_ORIGIN = 'https://analytix.local'
const markdownRehypePlugins = [
  [
    harden,
    {
      defaultOrigin: MARKDOWN_DEFAULT_ORIGIN,
      allowedLinkPrefixes: ['*'],
      allowedImagePrefixes: ['*']
    }
  ]
] as unknown as PluggableList

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function fileNameFromPath(path: string): string {
  return path.split(/[/\\]/).filter(Boolean).pop() ?? path
}

function extensionBadge(path: string, language: string): string {
  const fileName = fileNameFromPath(path)
  const ext = fileName.includes('.') ? fileName.split('.').pop() ?? '' : ''
  const value = ext || language || 'txt'
  return value.slice(0, 3).toUpperCase()
}

function targetKey(target: WorkspaceFileTarget | null | undefined): string {
  if (!target?.path) return ''
  return `${target.workspaceRoot ?? ''}\n${target.path}`.replaceAll('\\', '/').toLowerCase()
}

function isMarkdownPreviewPath(path: string): boolean {
  return /\.(md|markdown|mdx)$/i.test(path)
}

function normalizePreviewImageSrc(src: string | undefined): string | undefined {
  if (!src?.startsWith(`${MARKDOWN_DEFAULT_ORIGIN}/`)) return src

  try {
    const url = new URL(src)
    if (url.origin !== MARKDOWN_DEFAULT_ORIGIN) return src
    return decodeURIComponent(url.pathname.replace(/^\/+/, ''))
  } catch {
    return src
  }
}

type ResolvedPreviewImageProps = {
  src?: string
  alt?: string | null
  filePath?: string | null
} & Omit<ComponentPropsWithoutRef<'img'>, 'src' | 'alt'>

function ResolvedPreviewImage({
  src,
  alt,
  filePath,
  ...props
}: ResolvedPreviewImageProps): ReactElement {
  const normalizedSrc = normalizePreviewImageSrc(src)
  const [resolvedSrc, setResolvedSrc] = useState(() => initialWriteMarkdownImageSrc(normalizedSrc, filePath))
  const [loadError, setLoadError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoadError(null)
    setResolvedSrc(initialWriteMarkdownImageSrc(normalizedSrc, filePath))

    void loadWriteMarkdownImage(normalizedSrc, filePath).then((next) => {
      if (cancelled) return
      if (next.ok) setResolvedSrc(next.src)
      else setLoadError(next.message)
    })

    return () => {
      cancelled = true
    }
  }, [normalizedSrc, filePath])

  if (loadError) {
    return (
      <span
        className="inline-flex max-w-full items-center rounded-lg border border-red-200/70 bg-red-50/80 px-2 py-1 text-[12px] text-red-700 dark:border-red-900/60 dark:bg-red-950/40 dark:text-red-200"
        title={loadError}
      >
        {alt || src || 'Image could not be loaded'}
      </span>
    )
  }

  if (!resolvedSrc) {
    return (
      <span
        className="inline-flex max-w-full items-center rounded-lg border border-ds-border px-2 py-1 text-[12px] text-ds-muted"
        title={src}
      >
        {alt || src || 'Image'}
      </span>
    )
  }

  return <img {...props} src={resolvedSrc} alt={alt ?? ''} />
}

export function WorkspaceFilePreviewPanel({
  target,
  openTargets = target ? [target] : [],
  tabbedWorkspace = false,
  workspaceRoot,
  className,
  onSelectTarget,
  onCloseTarget,
  onClose,
  viewerControls,
  visible = true
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const [readAttempt, setReadAttempt] = useState(0)
  const currentReadAttempt = useRef(readAttempt); currentReadAttempt.current = readAttempt
  const retryPending = useRef(false)
  const [resultState, setResultState] = useState<{ owner: WorkspaceFileTarget; attempt: number; value: WorkspaceFileReadResult } | null>(null)
  // A retiring or replaced target masks its body in the same render, including
  // the dock's closing animation; effect cleanup alone is too late.
  const result = resultState?.owner === target && resultState.attempt === readAttempt ? resultState.value : null
  const [mediaState, setMediaState] = useState<PresentMediaResult | null>(null)
  const media = mediaState?.owner === target && mediaState.attempt === readAttempt ? mediaState : null
  const previewResult = media?.value ?? result
  const excelData = useMemo(() => {
    if (media?.kind !== 'excel' || !media.value.ok) return null
    try {
      const decoded = atob(media.value.dataBase64)
      if (decoded.length !== media.value.size || decoded.length > media.value.limits.maxBytes) return null
      return Uint8Array.from(decoded, char => char.charCodeAt(0))
    } catch { return null }
  }, [media])
  const sourceOnly = target?.preview === 'source' || target?.protectedRead !== undefined || target?.protectedPresent !== undefined
  // A suffix requests a typed reader only. Main chooses the real file format.
  const excelPreviewTarget = Boolean(target && /\.(xlsx|xls|csv|tsv)$/i.test(target.path)
    && (target.protectedPresent || !sourceOnly))
  const canReadTarget = Boolean(target && (sourceOnly || excelPreviewTarget || isWorkspaceTextPreviewPath(target.path)))
  // Go counts CRLF, CR and LF as line breaks. Normalize only source display;
  // the protected read buffer and ordinary preview bytes remain unchanged.
  const displayContent = result?.ok ? (sourceOnly ? result.content.replace(/\r\n?/g, '\n') : result.content) : ''
  const setResult = useCallback((value: WorkspaceFileReadResult | null) => {
    setResultState(value && target ? { owner: target, attempt: readAttempt, value } : null)
  }, [target, readAttempt])
  const [presentOpenState, setPresentOpenState] = useState<{ owner: WorkspaceFileTarget; phase: 'opening' | 'opened' | 'unavailable' } | null>(null)
  const presentOpen = presentOpenState?.owner === target ? presentOpenState.phase : null
  const currentTarget = useRef(target); currentTarget.current = target
  const currentWorkspaceRoot = useRef(workspaceRoot); currentWorkspaceRoot.current = workspaceRoot
  const downloadSequence = useRef(0)
  const downloadPending = useRef<{ owner: WorkspaceFileTarget; workspaceRoot: string; sequence: number } | null>(null)
  const [downloadState, setDownloadState] = useState<{
    owner: WorkspaceFileTarget; workspaceRoot: string; sequence: number; phase: 'saving' | 'saved' | 'canceled' | 'unavailable'
  } | null>(null)
  const download = downloadState?.owner === target && downloadState.workspaceRoot === workspaceRoot
    && downloadState.sequence === downloadSequence.current ? downloadState.phase : null
  useEffect(() => () => { downloadSequence.current++; downloadPending.current = null }, [target, workspaceRoot])
  const excelActive = useRef(true)
  useEffect(() => { excelActive.current = true; return () => { excelActive.current = false } }, [])
  const currentMedia = useRef(media); currentMedia.current = media
  const excelCopySequence = useRef(0)
  const [excelCopyState, setExcelCopyState] = useState<{ owner: WorkspaceFileTarget; media: PresentMediaResult; sequence: number; phase: 'copied' | 'unavailable' } | null>(null)
  const excelCopy = excelCopyState?.owner === target && excelCopyState.media === media
    && excelCopyState.sequence === excelCopySequence.current ? excelCopyState.phase : null
  const [zoomControlTarget, setZoomControlTarget] = useState<HTMLDivElement | null>(null)
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const [markdownRendered, setMarkdownRendered] = useState(true)
  const [htmlOwner, setHtmlOwner] = useState<{ target: WorkspaceFileTarget; root: string; identity: string } | null>(null)
  const htmlControl = useRef<HtmlEmbedHandle | null>(null)
  const htmlRoot = target?.workspaceRoot ?? workspaceRoot
  const htmlIdentity = JSON.stringify([htmlRoot, target?.path, target?.protectedPresent ?? null])
  const currentHtmlIdentity = useRef(htmlIdentity); currentHtmlIdentity.current = htmlIdentity
  const canRunHtml = Boolean(target && /\.html?$/i.test(target.path) && !target.protectedRead && (!sourceOnly || target.protectedPresent))
  const htmlRunning = canRunHtml && htmlOwner?.target === target && htmlOwner?.root === htmlRoot && htmlOwner.identity === htmlIdentity
  const stopHtml = useCallback((): void => { htmlControl.current?.dispose(); htmlControl.current = null; setHtmlOwner(null) }, [])
  const retryRead = (): void => {
    if (!target || !canReadTarget || loading || retryPending.current) return
    retryPending.current = true
    stopHtml()
    setLoading(true)
    setReadAttempt(attempt => attempt + 1)
  }
  useEffect(() => {
    const reference = target?.protectedPresent
    if (!reference) return
    const originalViewId = reference.viewId
    return window.analytix.runtime.onToolResultLocalDisplayInvalidated?.(viewId => {
      if (currentTarget.current === target && currentHtmlIdentity.current === htmlIdentity &&
        (viewId === null || viewId === originalViewId)) stopHtml()
    })
  }, [target, stopHtml, htmlIdentity])
  const [highlightState, setHighlightState] = useState<{ owner: WorkspaceFileReadResult; html: string } | null>(null)
  const highlightHtml = useMemo(() => highlightState?.owner === result ? highlightState.html
    : renderFallbackCodeHtml(displayContent), [highlightState, result, displayContent])
  const scrollRef = useRef<HTMLDivElement>(null)
  const rootRef = useRef<HTMLElement>(null)
  // Protected navigation exposes source locally. Transfers use explicit fresh
  // protected effects; the browser preview never copies or drags raw content.
  useEffect(() => {
    if (!sourceOnly) return
    const blockTransfer = (event: Event) => {
      const root = rootRef.current
      if (!root) return
      for (const element of event.type === 'dragstart' ? [event.target] : [event.target, document.activeElement]) {
        if (element instanceof Element && element.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') && !root.contains(element)) return
      }
      let intersects = event.target instanceof Node && root.contains(event.target)
      const selection = document.getSelection()
      for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
        try { if (selection.getRangeAt(index).intersectsNode(root)) intersects = true } catch { /* Retired range. */ }
      }
      if (intersects) event.preventDefault()
    }
    for (const event of ['copy', 'cut', 'dragstart']) document.addEventListener(event, blockTransfer, true)
    return () => { for (const event of ['copy', 'cut', 'dragstart']) document.removeEventListener(event, blockTransfer, true) }
  }, [sourceOnly])
  const copyResetRef = useRef<number | null>(null)

  useEffect(() => {
    setMediaState(null)
    if (!target) {
      retryPending.current = false
      setResult(null)
      setLoading(false)
      return
    }

    let cancelled = false
    const isCurrent = (): boolean => !cancelled && currentReadAttempt.current === readAttempt
    retryPending.current = true
    setLoading(true)
    setResult(null)

    if (!canReadTarget) {
      setResult({
        ok: false,
        message: t('filePreviewUnsupported')
      })
      setLoading(false)
      retryPending.current = false
      return
    }

    const request = { ...target, workspaceRoot: target.workspaceRoot ?? workspaceRoot }
    void (async () => {
      try {
        // Display suffix chooses a typed request, never path authority. Main
        // resolves the original selector/index and checks the real file MIME.
        if (excelPreviewTarget) {
          const value = await window.analytix.files.readExcel(request)
          if (isCurrent()) setMediaState({ owner: target, attempt: readAttempt, kind: 'excel', value })
        } else if (target.protectedPresent && /\.(png|jpe?g|gif|webp|bmp|ico|svg)$/i.test(target.path)) {
          const value = await window.analytix.files.readImage(request)
          if (isCurrent()) setMediaState({ owner: target, attempt: readAttempt, kind: 'image', value })
        } else if (target.protectedPresent && /\.pdf$/i.test(target.path)) {
          const value = await window.analytix.files.readPdf(request)
          if (isCurrent()) setMediaState({ owner: target, attempt: readAttempt, kind: 'pdf', value })
        } else {
          const next = await window.analytix.files.read(request)
          if (isCurrent()) setResult(next)
        }
      } catch (error) {
        if (isCurrent()) setResult({ ok: false, message: error instanceof Error ? error.message : String(error) })
      } finally {
        if (isCurrent()) { setLoading(false); retryPending.current = false }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [t, target, workspaceRoot, canReadTarget, excelPreviewTarget, setResult, readAttempt])

  useEffect(() => {
    if (!result?.ok || !result.line) return
    const id = window.requestAnimationFrame(() => {
      const row = scrollRef.current?.querySelector(`[data-line="${result.line}"]`)
      row?.scrollIntoView({ block: 'center' })
    })
    return () => window.cancelAnimationFrame(id)
  }, [result])

  useEffect(
    () => () => {
      if (copyResetRef.current !== null) window.clearTimeout(copyResetRef.current)
    },
    []
  )

  const displayPath = useMemo(() => {
    const root = target?.workspaceRoot ?? workspaceRoot
    if (previewResult?.ok) return formatFilePathForDisplay(previewResult.path, root) ?? fileNameFromPath(previewResult.path)
    return target?.path ? formatFilePathForDisplay(target.path, root) ?? fileNameFromPath(target.path) : ''
  }, [previewResult, target, workspaceRoot])
  const language = useMemo(() => {
    if (previewResult?.ok) return languageFromFilePath(previewResult.path)
    return target?.path ? languageFromFilePath(target.path) : ''
  }, [previewResult, target])
  const activeTargetKey = targetKey(target)
  const isHtmlFile = /\.html?$/i.test(result?.ok ? result.path : target?.path ?? '')
  const isMarkdownFile = isMarkdownPreviewPath(result?.ok ? result.path : target?.path ?? '')
  const lines = useMemo(() => (result?.ok ? displayContent.split('\n') : []), [result, displayContent])
  const currentFileName = displayPath ? fileNameFromPath(displayPath) : t('filePreviewTitle')
  const activeLine = result?.ok && result.line && result.line >= 1 && result.line <= lines.length
    ? result.line
    : null
  const codeSurfaceStyle = activeLine
    ? ({
        '--ds-file-preview-active-line': activeLine - 1
      } as CSSProperties)
    : undefined

  useEffect(() => {
    if (!result?.ok) {
      setHighlightState(null)
      return
    }

    let cancelled = false
    const fallback = renderFallbackCodeHtml(displayContent)
    setHighlightState({ owner: result, html: fallback })

    void highlightCodeHtml(displayContent, language).then((html) => {
      if (!cancelled) setHighlightState({ owner: result, html })
    })

    return () => {
      cancelled = true
    }
  }, [result, language, displayContent])

  const openInEditor = (): void => {
    if (target?.protectedPresent) {
      // Main resolves the current file and actual default application. The
      // displayed path is not passed to either the native query or opener.
      const owner = target
      if (presentOpen === 'opening') return
      setPresentOpenState({ owner, phase: 'opening' })
      void window.analytix.runtime.openToolResultLocalPresentFile(target.protectedPresent).then(value => {
        const result = toolResultLocalEffectResultSchemaV1.safeParse(value)
        if (currentTarget.current === owner) setPresentOpenState({ owner, phase: result.success && result.data.ok ? 'opened' : 'unavailable' })
      }).catch(() => { if (currentTarget.current === owner) setPresentOpenState({ owner, phase: 'unavailable' }) })
      return
    }
    if (sourceOnly) return
    const path = media?.kind === 'excel' && media.value.ok ? media.value.path : result?.ok ? result.path : target?.path
    if (!path) return
    void openWorkspacePathInEditor(
      {
        path,
        line: result?.ok ? result.line : target?.line,
        column: result?.ok ? result.column : target?.column
      },
      target?.workspaceRoot ?? workspaceRoot
    ).then((next) => {
      if (!next.ok && !sourceOnly) {
        void window.analytix?.logs?.error?.('editor-open', 'Failed to open previewed file', {
          message: next.message,
          target
        })?.catch(() => undefined)
      }
    })
  }

  // Fortune takes over synchronously; the existing protected IPC owns the async effect.
  const copyPresentExcel = (sheetIndex: number, sheetId: string, ranges: Array<{ row: [number, number]; column: [number, number] }>): boolean => {
    const owner = target
    if (!excelActive.current || !owner?.protectedPresent || currentTarget.current !== owner
      || currentMedia.current !== media || media?.owner !== owner || media.kind !== 'excel' || !media.value.ok || !excelData) return false
    const copyMedia = media
    const sequence = ++excelCopySequence.current
    setExcelCopyState(null)
    const currentCopy = (): boolean => excelActive.current && currentTarget.current === owner && currentMedia.current === copyMedia
      && excelCopySequence.current === sequence
    const request = {
      ...owner.protectedPresent,
      contentDigest: media.value.contentDigest,
      selection: { sheetIndex, sheetId, ranges: ranges.map(range => ({
        row: [...range.row] as [number, number], column: [...range.column] as [number, number]
      })) }
    }
    try {
      void window.analytix.runtime.copyToolResultLocalDisplay(request).then(value => {
        const parsed = toolResultLocalEffectResultSchemaV1.safeParse(value)
        if (currentCopy()) setExcelCopyState({ owner, media: copyMedia, sequence, phase: parsed.success && parsed.data.ok ? 'copied' : 'unavailable' })
      }).catch(() => { if (currentCopy()) setExcelCopyState({ owner, media: copyMedia, sequence, phase: 'unavailable' }) })
      return true
    } catch {
      if (currentCopy()) setExcelCopyState({ owner, media: copyMedia, sequence, phase: 'unavailable' })
      return false
    }
  }

  const copyContent = async (): Promise<void> => {
    if (sourceOnly || !result?.ok || !navigator?.clipboard?.writeText) return
    try {
      await navigator.clipboard.writeText(result.content)
      setCopied(true)
      if (copyResetRef.current !== null) window.clearTimeout(copyResetRef.current)
      copyResetRef.current = window.setTimeout(() => setCopied(false), COPY_RESET_MS)
    } catch {
      setCopied(false)
    }
  }

  const downloadFile = async (): Promise<void> => {
    const owner = target
    if (!excelActive.current || !owner || currentTarget.current !== owner || currentWorkspaceRoot.current !== workspaceRoot
      || sourceOnly && !owner.protectedPresent || downloadPending.current) return
    const pending = { owner, workspaceRoot, sequence: ++downloadSequence.current }
    downloadPending.current = pending
    setDownloadState({ ...pending, phase: 'saving' })
    const currentDownload = (): boolean => excelActive.current && currentTarget.current === owner
      && currentWorkspaceRoot.current === workspaceRoot && downloadSequence.current === pending.sequence
      && downloadPending.current === pending
    try {
      let phase: 'saved' | 'canceled' | 'unavailable'
      if (owner.protectedPresent) {
        const { threadId, turnId, callId, resultItemId, viewId, snapshotDigest, index } = owner.protectedPresent
        // Only the original Present identity reaches Main. Preview bytes, paths
        // and selection never become authority or an ordinary-save fallback.
        const value = await window.analytix.runtime.saveToolResultLocalDisplay({
          threadId, turnId, callId, resultItemId, viewId, snapshotDigest, index
        })
        const parsed = toolResultLocalEffectResultSchemaV1.safeParse(value)
        phase = !parsed.success ? 'unavailable' : parsed.data.ok ? 'saved'
          : parsed.data.code === 'canceled' ? 'canceled' : 'unavailable'
      } else {
        const value = await window.analytix.files.saveAs({ sourcePath: owner.path, workspaceRoot: owner.workspaceRoot ?? workspaceRoot })
        phase = value?.ok === true && typeof value.path === 'string' && value.path.trim() ? 'saved'
          : value?.ok === false && value.canceled === true ? 'canceled' : 'unavailable'
      }
      if (currentDownload()) setDownloadState({ ...pending, phase })
    } catch {
      if (currentDownload()) setDownloadState({ ...pending, phase: 'unavailable' })
    } finally {
      if (downloadPending.current === pending) downloadPending.current = null
    }
  }
  const downloadStatus = download ? t(download === 'saving' ? 'generatedFileSaving'
    : download === 'saved' ? 'generatedFileSaved' : download === 'canceled' ? 'userInputCancelled' : 'generatedFileSaveFailed') : null
  const excelStatus = excelCopy ? t(excelCopy === 'copied' ? 'copySuccess' : 'protectedToolUnavailable')
    : presentOpen ? t('dshPresent.' + presentOpen) : null
  const viewerStatus = [downloadStatus, excelStatus].filter((value, index, all) => value && all.indexOf(value) === index).join(' · ') || undefined
  const fileMetadata = [displayPath || currentFileName,
    previewResult?.ok ? formatBytes(previewResult.size) : '', language].filter(Boolean).join(' · ')
  const sharedViewerControls = visible && viewerControls ? { ...viewerControls, onCollapse: () => { stopHtml(); viewerControls.onCollapse() } } : undefined

  return (
    <aside ref={rootRef} data-protected-source-preview={sourceOnly || undefined}
      className={`ds-no-drag ds-code-sidebar flex min-h-0 flex-col border-l border-ds-border-muted ${className ?? ''}`}
    >
      <WorkspaceViewerHeader title={currentFileName} status={viewerStatus} viewerControls={sharedViewerControls}
        titleControl={<WorkspaceViewerMenu label={fileMetadata} title={currentFileName}
          ownerKey={JSON.stringify([target, workspaceRoot])} enabled={visible && Boolean(target)}>
          {!tabbedWorkspace && openTargets.length > 1 ? <div className="ds-code-sidebar-tabs !max-w-full !flex-col !items-stretch" role="tablist" aria-label={t('filePreviewOpenFiles')}>
            {openTargets.map(item => {
              const active = targetKey(item) === activeTargetKey
              const itemLabel = fileNameFromPath(item.path)
              const itemTitle = formatFilePathForDisplay(item.path, item.workspaceRoot ?? workspaceRoot) ?? item.path
              return <div key={targetKey(item)} role="tab" tabIndex={0} aria-selected={active}
                className={`ds-code-sidebar-tab ${active ? 'is-active' : ''}`} title={itemTitle}
                onDoubleClick={openInEditor} onClick={() => { if (item !== target) stopHtml(); onSelectTarget?.(item) }}
                onKeyDown={event => {
                  if (event.key !== 'Enter' && event.key !== ' ') return
                  event.preventDefault(); if (item !== target) stopHtml(); onSelectTarget?.(item)
                }}>
                <span className="ds-code-sidebar-file-badge">{extensionBadge(item.path, languageFromFilePath(item.path))}</span>
                <span className="min-w-0 truncate">{itemLabel}</span>
                {onCloseTarget ? <WorkspaceViewerIconButton compact label={t('filePreviewCloseTab', { file: itemLabel })}
                  className="ds-code-sidebar-tab-close" onClick={event => {
                    event.stopPropagation(); if (item === target) stopHtml(); onCloseTarget(item)
                  }} onKeyDown={event => {
                    if (event.key !== 'Enter' && event.key !== ' ') return
                    event.preventDefault(); event.stopPropagation(); if (item === target) stopHtml(); onCloseTarget(item)
                  }}><X aria-hidden="true" /></WorkspaceViewerIconButton> : null}
              </div>
            })}
          </div> : null}
          <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" label={t(target?.protectedPresent ? 'dshPresent.open' : 'filePreviewOpenEditor')}
            onClick={openInEditor} disabled={sourceOnly && !target?.protectedPresent || !target || presentOpen === 'opening'}>
            <ExternalLink aria-hidden="true" /><span>{t(target?.protectedPresent ? 'dshPresent.open' : 'filePreviewOpenEditor')}</span>
          </WorkspaceViewerIconButton>
          {!excelPreviewTarget ? <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start"
            label={copied ? t('copySuccess') : t('filePreviewCopyContent')} onClick={() => void copyContent()} disabled={sourceOnly || !result?.ok}>
            {copied ? <Check className="text-emerald-600" aria-hidden="true" /> : <Copy aria-hidden="true" />}<span>{copied ? t('copySuccess') : t('filePreviewCopyContent')}</span>
          </WorkspaceViewerIconButton> : null}
        </WorkspaceViewerMenu>}
        actions={<>
          {excelPreviewTarget || media?.kind === 'pdf' && media.value.ok ? <div key={activeTargetKey} ref={setZoomControlTarget}
            data-excel-preview-controls={excelPreviewTarget || undefined} data-pdf-preview-controls={media?.kind === 'pdf' || undefined}
            className={`ds-file-viewer-controls ${excelPreviewTarget ? previewStyles.zoomTarget : ''}`} /> : null}
          {isHtmlFile && canRunHtml ? <WorkspaceViewerIconButton disabled={!result?.ok}
            label={htmlRunning ? t('filePreviewShowSource') : t('writeHtmlEmbedRun')} aria-pressed={htmlRunning}
            onClick={() => { if (htmlRunning) stopHtml(); else if (target) setHtmlOwner({ target, root: htmlRoot, identity: htmlIdentity }) }}>
            {htmlRunning ? <Code2 aria-hidden="true" /> : <Eye aria-hidden="true" />}
          </WorkspaceViewerIconButton> : null}
          {isMarkdownFile && !sourceOnly ? <WorkspaceViewerIconButton onClick={() => setMarkdownRendered(value => !value)} disabled={!result?.ok}
            label={markdownRendered ? t('filePreviewShowSource') : t('filePreviewRenderMarkdown')} aria-pressed={markdownRendered}>
            {markdownRendered ? <Code2 aria-hidden="true" /> : <Eye aria-hidden="true" />}
          </WorkspaceViewerIconButton> : null}
          {target && (target.protectedPresent || !sourceOnly) ? <WorkspaceViewerIconButton shape="pill" onClick={() => void downloadFile()}
            disabled={download === 'saving'} aria-busy={download === 'saving'} label={t('generatedFileDownload')}>
            {download === 'saving' ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Download aria-hidden="true" />}
          </WorkspaceViewerIconButton> : null}
          {!tabbedWorkspace && !viewerControls ? <WorkspaceViewerIconButton onClick={() => { stopHtml(); onClose() }}
            className="ds-file-viewer-close" label={t('rightPanelCollapse')}><PanelRightClose aria-hidden="true" /></WorkspaceViewerIconButton> : null}
        </>} />

      <div className="flex min-h-0 flex-1 flex-col">
        {!target ? (
          <div className="flex flex-1 items-center justify-center px-6 text-center text-[12px] leading-6 text-ds-muted">
            <div className="flex flex-col items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-ds-border-muted text-ds-faint">
                <FileCode2 className="h-5 w-5" strokeWidth={1.7} />
              </div>
              {t('filePreviewEmpty')}
            </div>
          </div>
        ) : loading ? (
          <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-ds-muted">
            <Loader2 className="h-4 w-4 animate-spin" strokeWidth={1.8} />
            {t('filePreviewLoading')}
          </div>
        ) : media?.kind === 'image' && media.value.ok ? (
          <div data-present-media-preview="image" className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4"
            onContextMenu={event => event.preventDefault()}>
            <ImagePreview src={media.value.dataUrl} alt={currentFileName}
              loadingLabel={t('filePreviewLoading')} failedLabel={t('filePreviewFailed')} />
          </div>
        ) : media?.kind === 'pdf' && media.value.ok ? (
          <div data-present-media-preview="pdf" className="flex min-h-0 flex-1 flex-col" onContextMenu={event => event.preventDefault()}>
            <WritePdfViewer filePath={media.value.path} dataBase64={media.value.dataBase64} size={media.value.size}
              mtimeMs={media.value.mtimeMs} workspaceRoot="" zoomControlTarget={zoomControlTarget} onSelectionChange={ignoreProtectedPdfSelection} />
          </div>
        ) : media?.kind === 'excel' && media.value.ok && excelData ? (
          <div data-present-media-preview="excel" className="flex min-h-0 flex-1 flex-col">
            <LazyExcelBody owner={target} data={excelData} format={media.value.format} limits={media.value.limits}
              onProtectedCopy={target.protectedPresent ? copyPresentExcel : undefined} zoomControlTarget={zoomControlTarget} />
          </div>
        ) : result?.ok ? (
          <div className="relative flex min-h-0 flex-1 flex-col">
            {result.truncated ? (
              <div className="shrink-0 border-b border-ds-border-muted/70 px-4 py-1.5 text-[11px] text-amber-700 dark:text-amber-300">
                {t('filePreviewTruncated')}
              </div>
            ) : null}
            {isHtmlFile && htmlRunning ? <WorkspaceHtmlBody key={htmlIdentity}
              path={result.path} workspaceRoot={htmlRoot} control={htmlControl} protectedPresent={target?.protectedPresent} />
            : isMarkdownFile && markdownRendered && !sourceOnly ? (
              <div className="ds-file-preview-markdown min-h-0 flex-1 overflow-auto px-5 py-4">
                <div className="ds-markdown min-h-full text-ds-ink">
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    rehypePlugins={markdownRehypePlugins}
                    components={{
                      a: ({ href, children, ...props }): ReactNode => (
                        <a
                          {...props}
                          href={href}
                          onClick={(event) => {
                            if (!href) return
                            event.preventDefault()
                            void window.analytix?.app?.openExternal?.(href)?.catch(() => undefined)
                          }}
                        >
                          {children}
                        </a>
                      ),
                      img: ({ src, alt, ...props }): ReactNode => (
                        <ResolvedPreviewImage
                          {...props}
                          src={src}
                          alt={alt}
                          filePath={result.path}
                        />
                      )
                    }}
                  >
                    {result.content}
                  </ReactMarkdown>
                </div>
              </div>
            ) : (
              <div
                ref={scrollRef}
                className="ds-file-preview-scroll min-h-0 flex-1 overflow-auto font-mono text-[12px] leading-[22px] text-ds-ink"
              >
                <div
                  className="ds-file-preview-code-surface"
                  style={codeSurfaceStyle}
                >
                  {activeLine ? (
                    <div className="ds-file-preview-active-line" aria-hidden="true" />
                  ) : null}
                  <div className="ds-file-preview-gutter">
                    {lines.map((_, index) => {
                      const lineNo = index + 1
                      return (
                        <div
                          key={lineNo}
                          data-line={lineNo}
                          className={`ds-file-preview-line-number ${activeLine === lineNo ? 'is-active' : ''}`}
                        >
                          {lineNo}
                        </div>
                      )
                    })}
                  </div>
                  <div
                    className="ds-file-preview-code-html"
                    dangerouslySetInnerHTML={{ __html: highlightHtml }}
                  />
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center text-[12px] leading-6">
            <span role="alert" className="text-red-700 dark:text-red-300">{previewResult && !previewResult.ok ? previewResult.message : t('filePreviewFailed')}</span>
            {canReadTarget ? <button type="button" onClick={retryRead} disabled={loading}
              className="rounded-lg border border-ds-border px-3 py-1.5 text-ds-ink hover:bg-ds-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-ds-accent">
              {t('protectedToolRetry')}
            </button> : null}
          </div>
        )}
      </div>
    </aside>
  )
}
