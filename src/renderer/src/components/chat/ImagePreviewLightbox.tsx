import { useId, useLayoutEffect, useRef, useState, type ReactElement } from 'react'
import { createPortal } from 'react-dom'
import { Download, Minus, Plus, X } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import { useModalFocus } from '../../hooks/use-modal-focus'

type ImagePreviewLightboxProps = {
  open: boolean
  src: string
  alt: string
  title?: string
  downloadHref?: string
  downloadName?: string
  downloadDisabled?: boolean
  downloadLabel?: string
  onDownload?: () => void | Promise<void>
  onClose: () => void
}

const MIN_ZOOM = 0.5
const MAX_ZOOM = 3
const ZOOM_STEP = 0.25

function clampZoom(value: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value))
}

export function ImagePreviewLightbox(props: ImagePreviewLightboxProps): ReactElement | null {
  // The modal lifecycle belongs to the actual mounted dialog. Existing
  // attachment/composer consumers keep this wrapper mounted while closed.
  return props.open ? <MountedImagePreviewLightbox {...props} /> : null
}

function MountedImagePreviewLightbox({
  open,
  src,
  alt,
  title,
  downloadHref,
  downloadName,
  downloadDisabled = false,
  downloadLabel,
  onDownload,
  onClose
}: ImagePreviewLightboxProps): ReactElement | null {
  const { t } = useTranslation('common')
  const [zoom, setZoom] = useState(1)
  const [naturalSize, setNaturalSize] = useState<{ src: string; width: number; height: number } | null>(null)
  const [availableSize, setAvailableSize] = useState({ width: 0, height: 0 })
  const viewportRef = useRef<HTMLDivElement>(null)
  const imageRef = useRef<HTMLImageElement>(null)
  const currentSrc = useRef(src)
  currentSrc.current = src
  const titleId = useId()
  const modalRef = useModalFocus(onClose, true, true)
  const closeLabel = t('imagePreviewClose')
  const resolvedTitle = title || alt || t('imagePreviewTitle')
  const resolvedDownloadLabel = downloadLabel ?? t('imagePreviewDownload')

  useLayoutEffect(() => {
    setZoom(1)
    if (viewportRef.current) {
      viewportRef.current.scrollLeft = 0
      viewportRef.current.scrollTop = 0
    }
    const image = imageRef.current
    if (image?.complete && image.naturalWidth > 0 && image.naturalHeight > 0) {
      setNaturalSize({ src, width: image.naturalWidth, height: image.naturalHeight })
    }
  }, [src])

  useLayoutEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const measure = (): void => {
      const style = getComputedStyle(viewport)
      const width = Math.max(0, viewport.clientWidth - parseFloat(style.paddingLeft || '0') - parseFloat(style.paddingRight || '0'))
      const height = Math.max(0, viewport.clientHeight - parseFloat(style.paddingTop || '0') - parseFloat(style.paddingBottom || '0'))
      setAvailableSize(previous => previous.width === width && previous.height === height ? previous : { width, height })
    }
    measure()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(viewport)
    window.addEventListener('resize', measure)
    return () => {
      observer?.disconnect()
      window.removeEventListener('resize', measure)
    }
  }, [])

  if (!open || typeof document === 'undefined') return null

  const zoomPercent = `${Math.round(zoom * 100)}%`
  const canDownload = !downloadDisabled && (typeof onDownload === 'function' || Boolean(downloadHref))
  const natural = naturalSize?.src === src ? naturalSize : null
  const fit = natural && availableSize.width > 0 && availableSize.height > 0
    ? Math.min(1, availableSize.width / natural.width, availableSize.height / natural.height) : null
  const imageStyle = natural && fit !== null ? { width: natural.width * fit * zoom, height: natural.height * fit * zoom } : undefined

  const downloadControl = onDownload ? (
    <button
      type="button"
      onClick={() => void onDownload()}
      disabled={!canDownload}
      aria-label={resolvedDownloadLabel}
      title={resolvedDownloadLabel}
      className="inline-flex h-11 w-11 items-center justify-center rounded-full bg-white text-zinc-700 shadow-[0_14px_34px_rgba(0,0,0,0.22)] transition hover:bg-zinc-50 hover:text-zinc-950 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-800"
    >
      <Download className="h-5 w-5" strokeWidth={1.9} />
    </button>
  ) : downloadHref ? (
    <a
      href={downloadHref}
      download={downloadName || resolvedTitle}
      aria-label={resolvedDownloadLabel}
      title={resolvedDownloadLabel}
      className="inline-flex h-11 w-11 items-center justify-center rounded-full bg-white text-zinc-700 shadow-[0_14px_34px_rgba(0,0,0,0.22)] transition hover:bg-zinc-50 hover:text-zinc-950 dark:bg-zinc-100 dark:text-zinc-800"
    >
      <Download className="h-5 w-5" strokeWidth={1.9} />
    </a>
  ) : null

  return createPortal(
    <div
      className="ds-no-drag fixed inset-0 z-[1100] bg-zinc-950/[0.82] text-white backdrop-blur-[2px]"
      ref={modalRef}
      tabIndex={-1}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <h2 id={titleId} className="sr-only">
        {resolvedTitle}
      </h2>
      <div className="absolute right-3 top-3 z-10 flex items-center gap-2 sm:right-4 sm:top-4">
        {downloadControl}
        <button
          type="button"
          onClick={onClose}
          data-modal-autofocus
          aria-label={closeLabel}
          title={closeLabel}
          className="inline-flex h-11 w-11 items-center justify-center rounded-full bg-white text-zinc-700 shadow-[0_14px_34px_rgba(0,0,0,0.22)] transition hover:bg-zinc-50 hover:text-zinc-950 dark:bg-zinc-100 dark:text-zinc-800"
        >
          <X className="h-5 w-5" strokeWidth={2} />
        </button>
      </div>
      <div className="flex h-full w-full items-center justify-center px-4 py-20 sm:px-8">
        <div ref={viewportRef} tabIndex={0} aria-label={resolvedTitle}
          className="h-full max-h-[calc(100dvh-128px)] w-full max-w-[min(1120px,calc(100vw-32px))] overflow-auto rounded-[18px] border border-white/[0.16] bg-[rgba(255,250,242,0.96)] p-2 shadow-[0_30px_90px_rgba(0,0,0,0.42)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white dark:bg-zinc-950/[0.88] sm:max-h-[calc(100dvh-144px)]">
          <div className="flex min-h-full min-w-full items-center justify-center" style={imageStyle}>
          <img
            key={src}
            ref={imageRef}
            src={src}
            alt={alt}
            className={imageStyle ? 'max-w-none shrink-0 object-contain' : 'max-h-full max-w-full object-contain'}
            style={imageStyle}
            onLoad={event => {
              const image = event.currentTarget
              if (currentSrc.current === src && image.naturalWidth > 0 && image.naturalHeight > 0) {
                setNaturalSize({ src, width: image.naturalWidth, height: image.naturalHeight })
              }
            }}
            draggable={false}
            referrerPolicy="no-referrer"
          />
          </div>
        </div>
      </div>
      <div className="absolute bottom-5 left-1/2 flex -translate-x-1/2 items-center overflow-hidden rounded-full bg-white text-zinc-700 shadow-[0_14px_34px_rgba(0,0,0,0.24)] dark:bg-zinc-100 dark:text-zinc-800">
        <button
          type="button"
          onClick={() => setZoom((value) => clampZoom(value - ZOOM_STEP))}
          disabled={zoom <= MIN_ZOOM}
          aria-label={t('imagePreviewZoomOut')}
          title={t('imagePreviewZoomOut')}
          className="inline-flex h-10 w-11 items-center justify-center transition hover:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-45"
        >
          <Minus className="h-4 w-4" strokeWidth={2} />
        </button>
        <button
          type="button"
          onClick={() => setZoom(1)}
          aria-label={t('imagePreviewResetZoom')}
          title={t('imagePreviewResetZoom')}
          className="h-10 min-w-16 px-3 text-[13px] font-semibold transition hover:bg-zinc-100"
        >
          {zoomPercent}
        </button>
        <button
          type="button"
          onClick={() => setZoom((value) => clampZoom(value + ZOOM_STEP))}
          disabled={zoom >= MAX_ZOOM}
          aria-label={t('imagePreviewZoomIn')}
          title={t('imagePreviewZoomIn')}
          className="inline-flex h-10 w-11 items-center justify-center transition hover:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-45"
        >
          <Plus className="h-4 w-4" strokeWidth={2} />
        </button>
      </div>
    </div>,
    document.body
  )
}
