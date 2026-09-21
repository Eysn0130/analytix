import { useEffect, useRef, useState, useSyncExternalStore, type ReactElement } from 'react'
import { ExternalLink, Image as ImageIcon, ZoomIn, ZoomOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ImageRegion } from '../../../../../packages/runtime/src/contracts/object-editing'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { imagePoint, imageRectangle } from '../../write/image-region-session'
import { registerWriteShutdownInput } from '../../write/write-shutdown'
import { isImageThreadNavigationFrozen, subscribeImageThreadNavigation } from '../../write/image-thread-navigation'
import { useChatStore } from '../../store/chat-store'
import { useNativeReferenceStore } from '../../office/native-reference-store'
import {
  writeBasenameFromPath,
  writeRelativeToWorkspace
} from '../../write/write-workspace-store'
import {
  clamp,
  toolbarIconButtonClass,
  toolbarMenuButtonClass
} from './write-workspace-view-utils'

const IMAGE_MIN_ZOOM = 25
const IMAGE_MAX_ZOOM = 300
const IMAGE_ZOOM_STEP = 25

type WriteImagePreviewProps = {
  src: string
  filePath: string
  mimeType: string
  size: number
  workspaceRoot: string
}

type WriteImageFitMode = 'fit' | 'actual'

function clampImageZoom(value: number): number {
  return clamp(Math.round(value), IMAGE_MIN_ZOOM, IMAGE_MAX_ZOOM)
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function WriteImagePreview({
  src,
  filePath,
  mimeType,
  size,
  workspaceRoot
}: WriteImagePreviewProps): ReactElement {
  const { t } = useTranslation('common')
  const threadId = useSyncExternalStore(useChatStore.subscribe, () => useChatStore.getState().activeThreadId)
  const editor = useWriteWorkspaceStore(state => state.imageRegionEditor)
  const shutdownFrozen = useWriteWorkspaceStore(state => state.shutdownFrozen)
  const navigationFrozen = useSyncExternalStore(subscribeImageThreadNavigation, isImageThreadNavigationFrozen)
  const frozen = shutdownFrozen || navigationFrozen
  const imageRef = useRef<HTMLImageElement>(null)
  const noteRef = useRef<HTMLTextAreaElement>(null)
  const frozenRef = useRef(false)
  const shutdownRef = useRef(false)
  const composingRef = useRef(false)
  const drag = useRef<{ x: number; y: number; pointerId: number; original: ImageRegion | null } | null>(null)
  const [referencing, setReferencing] = useState(false)
  const supported = mimeType === 'image/png' || mimeType === 'image/jpeg'
  const owner = editor?.workspace === workspaceRoot && editor.path === filePath && editor.threadId === threadId
  const snapshot = owner && !editor.revoked ? editor.snapshot : null
  const editable = !!snapshot && !editor?.loading && !frozen
  const region = owner && !editor.stale ? editor.region : null
  useEffect(() => {
    const synchronize = () => {
      const store = useWriteWorkspaceStore.getState(), next = useChatStore.getState().activeThreadId
      const changed = store.imageRegionThreadId !== next
      store.setImageRegionThread(next)
      if (changed) drag.current = null
      if (changed && supported && next) void store.openImageRegion(workspaceRoot, filePath, next)
    }
    useWriteWorkspaceStore.getState().setImageRegionThread(useChatStore.getState().activeThreadId)
    return useChatStore.subscribe(synchronize)
  }, [workspaceRoot, filePath, supported])
  useEffect(() => {
    const store = useWriteWorkspaceStore.getState()
    const currentThread = useChatStore.getState().activeThreadId
    store.setImageRegionThread(currentThread)
    if (supported && currentThread) void store.openImageRegion(workspaceRoot, filePath, currentThread)
    return () => { drag.current = null; useWriteWorkspaceStore.getState().invalidateImageRegion() }
  }, [workspaceRoot, filePath, supported])
  useEffect(() => registerWriteShutdownInput({
    composing: () => composingRef.current,
    freeze: value => {
      shutdownRef.current = value
      frozenRef.current = value || isImageThreadNavigationFrozen()
      drag.current = null
      if (noteRef.current) noteRef.current.readOnly = frozenRef.current
    }
  }), [])
  useEffect(() => {
    const synchronize = () => {
      frozenRef.current = shutdownRef.current || isImageThreadNavigationFrozen()
      if (noteRef.current) noteRef.current.readOnly = frozenRef.current
    }
    synchronize()
    return subscribeImageThreadNavigation(synchronize)
  }, [])
  const updateRegion = (x: number, y: number, finish: boolean): void => {
    if (!drag.current || !imageRef.current || !snapshot || frozenRef.current) return
    const point = imagePoint(x, y, imageRef.current.getBoundingClientRect(), snapshot)
    const next = point && imageRectangle(drag.current, point)
    if (next) useWriteWorkspaceStore.getState().updateImageRegion({ region: next })
    if (finish) drag.current = null
  }
  const cancelDrag = (): void => {
    const previous = drag.current
    drag.current = null
    if (previous && !frozenRef.current) useWriteWorkspaceStore.getState().updateImageRegion({ region: previous.original })
  }
  const reference = async (): Promise<void> => {
    if (referencing || frozenRef.current) return
    setReferencing(true)
    try {
      const scope = await useWriteWorkspaceStore.getState().captureImageRegion()
      const current = useWriteWorkspaceStore.getState()
      if (!scope || frozenRef.current || current.imageRegionEditor?.revoked || current.imageRegionEditor?.dirty ||
        current.workspaceRoot !== workspaceRoot || current.activeFilePath !== filePath ||
        scope.threadId !== useChatStore.getState().activeThreadId || current.imageRegionEditor?.snapshot?.sessionId !== scope.sessionId) return
      useNativeReferenceStore.getState().add({ kind: 'image-region', threadId: scope.threadId, workspace: workspaceRoot, path: filePath,
        objectId: scope.objectId, revision: scope.sourceRevision, sessionId: scope.sessionId, scopeId: scope.scopeId,
        annotationRevision: scope.annotationRevision, width: scope.width, height: scope.height, region: scope.region,
        editable: false, text: '', label: writeBasenameFromPath(filePath) })
    } finally { setReferencing(false) }
  }
  const [dimensions, setDimensions] = useState<{ width: number; height: number } | null>(null)
  const [fitMode, setFitMode] = useState<WriteImageFitMode>('fit')
  const [zoom, setZoom] = useState(100)
  const fileName = writeBasenameFromPath(filePath)
  const relativePath = writeRelativeToWorkspace(workspaceRoot, filePath)
  const actualMode = fitMode === 'actual'
  useEffect(() => {
    setDimensions(null)
  }, [src, filePath])
  const openImage = (): void => {
    if (typeof window.analytix?.workspace?.openEditorPath !== 'function') return
    void window.analytix.workspace.openEditorPath({ path: filePath, workspaceRoot, editorId: 'system' }).catch(() => undefined)
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[radial-gradient(circle_at_top,rgba(59,130,216,0.08),transparent_34%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(247,250,255,0.68))] dark:bg-[radial-gradient(circle_at_top,rgba(96,165,250,0.13),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.025))]">
      <div className="flex min-h-[54px] shrink-0 items-center justify-between gap-3 border-b border-ds-border-muted px-4 py-2.5 sm:px-5">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">
            <ImageIcon className="h-[18px] w-[18px]" strokeWidth={1.9} />
          </span>
          <div className="min-w-0">
            <div className="truncate text-[14px] font-semibold text-ds-ink">{fileName}</div>
            <div className="mt-1 truncate text-[12px] text-ds-faint" title={relativePath}>
              {relativePath}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 rounded-xl border border-ds-border-muted bg-white/48 p-1 dark:bg-white/[0.035]">
          <button
            type="button"
            onClick={() => {
              setFitMode('actual')
              setZoom((value) => clampImageZoom(value - IMAGE_ZOOM_STEP))
            }}
            className={toolbarIconButtonClass()}
            title={t('writeImageZoomOut')}
            aria-label={t('writeImageZoomOut')}
          >
            <ZoomOut className="h-4 w-4" strokeWidth={1.85} />
          </button>
          <input
            type="range"
            min={IMAGE_MIN_ZOOM}
            max={IMAGE_MAX_ZOOM}
            step={IMAGE_ZOOM_STEP}
            value={zoom}
            aria-label={t('writeImageZoom')}
            className="h-8 w-24 accent-[var(--ds-accent)]"
            onChange={(event) => {
              setFitMode('actual')
              setZoom(clampImageZoom(Number(event.target.value)))
            }}
          />
          <button
            type="button"
            onClick={() => {
              setFitMode('actual')
              setZoom((value) => clampImageZoom(value + IMAGE_ZOOM_STEP))
            }}
            className={toolbarIconButtonClass()}
            title={t('writeImageZoomIn')}
            aria-label={t('writeImageZoomIn')}
          >
            <ZoomIn className="h-4 w-4" strokeWidth={1.85} />
          </button>
          <button
            type="button"
            onClick={() => setFitMode((mode) => mode === 'fit' ? 'actual' : 'fit')}
            className={`${toolbarMenuButtonClass(fitMode === 'fit')} min-w-[52px] justify-center`}
            title={fitMode === 'fit' ? t('writeImageActualSize') : t('writeImageFit')}
            aria-label={fitMode === 'fit' ? t('writeImageActualSize') : t('writeImageFit')}
          >
            {fitMode === 'fit' ? t('writeImageFitShort') : `${zoom}%`}
          </button>
        </div>
        <button
          type="button"
          onClick={openImage}
          className={toolbarIconButtonClass()}
          title={t('writeImageOpenExternal')}
          aria-label={t('writeImageOpenExternal')}
        >
          <ExternalLink className="h-4 w-4" strokeWidth={1.85} />
        </button>
      </div>

      <div className="ds-page-scroll-edge min-h-0 flex-1 overflow-auto p-4 sm:p-6">
        <div className="flex min-h-full items-center justify-center">
          <div className="relative inline-block max-w-full leading-none" style={actualMode ? { maxWidth: 'none' } : undefined}>
          <img
            ref={imageRef}
            src={snapshot ? `data:${snapshot.mimeType};base64,${snapshot.dataBase64}` : src}
            draggable={false}
            onPointerDown={event => {
              if (!editable || frozenRef.current || event.button !== 0 || !snapshot) return
              const point = imagePoint(event.clientX, event.clientY, event.currentTarget.getBoundingClientRect(), snapshot)
              if (!point) return
              event.preventDefault()
              drag.current = { ...point, pointerId: event.pointerId, original: region ? { ...region } : null }
              event.currentTarget.setPointerCapture?.(event.pointerId)
            }}
            onPointerMove={event => { if (drag.current?.pointerId === event.pointerId) updateRegion(event.clientX, event.clientY, false) }}
            onPointerUp={event => { if (drag.current?.pointerId === event.pointerId) updateRegion(event.clientX, event.clientY, true) }}
            onPointerCancel={cancelDrag}
            onLostPointerCapture={cancelDrag}
            alt={fileName}
            className={`${actualMode ? 'max-w-none' : 'max-h-full max-w-full'} select-none rounded-lg object-contain shadow-[0_18px_50px_rgba(20,47,95,0.16)]`}
            style={{ touchAction: editable ? 'none' : undefined, ...(actualMode && (snapshot || dimensions) ? {
              width: `${Math.round((snapshot || dimensions)!.width * zoom / 100)}px`, height: 'auto'
            } : {}) }}
            onLoad={(event) => {
              const image = event.currentTarget
              setDimensions(snapshot ? { width: snapshot.width, height: snapshot.height } : { width: image.naturalWidth, height: image.naturalHeight })
            }}
          />
          {region && snapshot ? <div data-testid="image-region-overlay" className="pointer-events-none absolute border-2 border-blue-600 bg-blue-500/15"
            style={{ left: `${region.x / snapshot.width * 100}%`, top: `${region.y / snapshot.height * 100}%`,
              width: `${region.width / snapshot.width * 100}%`, height: `${region.height / snapshot.height * 100}%` }} /> : null}
          </div>
        </div>
      </div>

      {supported ? <section aria-label={t('imageRegionTitle')} className="shrink-0 space-y-2 border-t border-ds-border-muted p-3 text-sm">
        <p className="text-xs text-ds-faint">{t('imageRegionDiscussionOnly')}</p>
        {!owner && editor?.dirty ? <p role="alert">{t('imageRegionReturnToTask')}</p> : null}
        {owner && editor.stale ? <p role="alert">{t('imageRegionRecapture')}</p> : null}
        {owner && editor.error ? <p role="alert">{editor.error}</p> : null}
        {owner && editor.loading ? <p role="status">{t('imageRegionLoading')}</p> : null}
        {snapshot ? <>
          <p>{t('imageRegionDraw')}</p>
          <div className="flex flex-wrap gap-2">
            {(['x', 'y', 'width', 'height'] as const).map(key => <label key={key} className="flex items-center gap-1">{key}
              <input type="number" aria-label={t(`imageRegionCoordinate_${key}`)} className="w-20 rounded border p-1"
                disabled={!editable || !region} min={key === 'width' || key === 'height' ? 1 : 0}
                max={key === 'x' || key === 'width' ? snapshot.width : snapshot.height} step={1} value={region?.[key] ?? ''}
                onChange={event => { if (region && !frozenRef.current) useWriteWorkspaceStore.getState().updateImageRegion({ region: { ...region, [key]: Number(event.target.value) } }) }} />
            </label>)}
          </div>
        </> : null}
        <textarea ref={noteRef} aria-label={t('imageRegionNote')} placeholder={t('imageRegionNote')}
          className="w-full rounded border border-ds-border-muted bg-transparent p-2" maxLength={4096}
          value={editor?.note ?? ''} disabled={!snapshot || !!editor?.loading} readOnly={frozen} rows={2}
          onCompositionStart={() => { composingRef.current = true; useWriteWorkspaceStore.getState().setImageRegionComposing(true) }}
          onCompositionEnd={event => { composingRef.current = false; useWriteWorkspaceStore.getState().setImageRegionComposing(false); if (!frozenRef.current) useWriteWorkspaceStore.getState().updateImageRegion({ note: event.currentTarget.value }) }}
          onChange={event => { if (!frozenRef.current) useWriteWorkspaceStore.getState().updateImageRegion({ note: event.target.value }) }} />
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className={toolbarMenuButtonClass()} disabled={!editable || editor?.composing || editor?.stale}
            onClick={() => { if (!frozenRef.current) void useWriteWorkspaceStore.getState().flushSave(workspaceRoot) }}>{t('imageRegionSave')}</button>
          <button type="button" className={toolbarMenuButtonClass()} disabled={!editable || !region || editor?.stale || editor?.status === 'conflict' || referencing || editor?.composing}
            onClick={() => void reference()}>{t('imageRegionReference')}</button>
          <button type="button" className={toolbarMenuButtonClass()} disabled={frozen || !threadId || !!editor?.pending || !!editor?.composing}
            onClick={() => { if (!frozenRef.current && threadId) void useWriteWorkspaceStore.getState().openImageRegion(workspaceRoot, filePath, threadId, true) }}>{t('imageRegionSelectAgain')}</button>
          {owner ? <span role="status">{t(`imageRegionStatus_${editor.status}`)}</span> : null}
        </div>
      </section> : null}

      <div className="flex shrink-0 flex-wrap items-center gap-2 border-t border-ds-border-muted bg-white/44 px-4 py-2 text-[11.5px] text-ds-faint dark:bg-white/[0.035] sm:px-5">
        <span className="rounded-lg bg-ds-hover/70 px-2 py-1 font-mono">{mimeType}</span>
        <span className="rounded-lg bg-ds-hover/70 px-2 py-1 font-mono">{formatBytes(size)}</span>
        {dimensions ? (
          <span className="rounded-lg bg-ds-hover/70 px-2 py-1 font-mono">
            {dimensions.width} x {dimensions.height}
          </span>
        ) : null}
      </div>
    </div>
  )
}
