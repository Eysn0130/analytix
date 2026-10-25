import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { CONVERSATION_DEFAULT_MIN_WIDTH } from '../chat/ConversationWidthControls'

export const THREAD_SUMMARY_SURFACE_ID = 'workbench-thread-summary'
export const THREAD_SUMMARY_TITLE_ID = 'workbench-thread-summary-title'
export const THREAD_SUMMARY_TRIGGER_ID = 'workbench-summary-toggle'
const CARD_WIDTH = 300
const GAP = 12
// The existing composer adds 32px around the content axis and 16px on each side.
const CHAT_MIN_WIDTH = CONVERSATION_DEFAULT_MIN_WIDTH + 32 + 2 * 16
const PINNED_RESERVE = CARD_WIDTH + 2 * GAP

type ChatBounds = { left: number; top: number; width: number; height: number }
export function resolveThreadSummaryLayout(bounds: ChatBounds, pinPreference: boolean) {
  return {
    mode: pinPreference && bounds.width >= CHAT_MIN_WIDTH + PINNED_RESERVE ? 'pinned' as const : 'floating' as const,
    left: bounds.left + Math.max(GAP, bounds.width - CARD_WIDTH - GAP),
    top: bounds.top + GAP,
    width: Math.min(CARD_WIDTH, Math.max(0, bounds.width - 2 * GAP)),
    maxHeight: Math.max(0, bounds.height - 2 * GAP)
  }
}

/** One chat-owned leaf keeps its state while only its position and reserve change. */
export function ThreadSummarySurface({ container, openIntent, pinPreference, onReserveChange, onDismiss, children }: {
  container: HTMLDivElement | null
  openIntent: boolean
  pinPreference: boolean
  onReserveChange: (width: number) => void
  onDismiss: (restoreFocus: boolean) => void
  children: ReactNode
}) {
  const panel = useRef<HTMLDivElement>(null)
  const openingHandled = useRef(false)
  const [layout, setLayout] = useState<ReturnType<typeof resolveThreadSummaryLayout> | null>(null)

  useLayoutEffect(() => {
    if (!openIntent || container === null) { setLayout(null); return }
    const measure = () => {
      const rect = container.getBoundingClientRect()
      // Clip to the chat's visible bounds, so a portal cannot cover the file dock.
      const left = Math.max(0, rect.left), top = Math.max(0, rect.top)
      const right = Math.min(window.innerWidth, rect.right), bottom = Math.min(window.innerHeight, rect.bottom)
      const next = resolveThreadSummaryLayout({ left, top, width: Math.max(0, right - left), height: Math.max(0, bottom - top) }, pinPreference)
      setLayout(previous => previous && Object.keys(next).every(key => previous[key as keyof typeof next] === next[key as keyof typeof next]) ? previous : next)
    }
    measure()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(container)
    window.addEventListener('resize', measure)
    window.addEventListener('scroll', measure, true)
    return () => { observer?.disconnect(); window.removeEventListener('resize', measure); window.removeEventListener('scroll', measure, true) }
  }, [container, openIntent, pinPreference])

  const pinned = openIntent && layout?.mode === 'pinned'
  useLayoutEffect(() => { onReserveChange(pinned ? PINNED_RESERVE : 0) }, [onReserveChange, pinned])
  useLayoutEffect(() => () => onReserveChange(0), [onReserveChange])

  useLayoutEffect(() => {
    if (!openIntent) { openingHandled.current = false; return }
    if (!layout || layout.width === 0 || layout.maxHeight === 0 || openingHandled.current) return
    openingHandled.current = true
    // An explicit opening can enter the dialog; responsive mode changes never steal focus.
    if (layout.mode === 'floating' && document.activeElement === document.getElementById(THREAD_SUMMARY_TRIGGER_ID)) {
      panel.current?.focus({ preventScroll: true })
    }
  }, [layout, openIntent])

  useEffect(() => {
    if (!openIntent || layout?.mode !== 'floating') return
    const outside = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || panel.current?.contains(event.target) || document.getElementById(THREAD_SUMMARY_TRIGGER_ID)?.contains(event.target)) return
      // Let the target finish its ordinary click/focus behavior.
      onDismiss(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || event.isComposing || event.keyCode === 229 || event.repeat) return
      if (event.target instanceof Element && event.target.closest('[role="dialog"][aria-modal="true"]')) return
      event.preventDefault()
      event.stopPropagation()
      onDismiss(true)
    }
    document.addEventListener('pointerdown', outside, true)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('pointerdown', outside, true); document.removeEventListener('keydown', escape) }
  }, [layout?.mode, onDismiss, openIntent])

  if (!openIntent || container === null) return null
  return createPortal(<div ref={panel} id={THREAD_SUMMARY_SURFACE_ID} data-thread-summary-surface data-mode={layout?.mode ?? 'floating'}
    className="ds-thread-summary-surface ds-no-drag" role={pinned ? 'region' : 'dialog'} aria-modal={pinned ? undefined : false}
    tabIndex={pinned ? undefined : -1}
    aria-labelledby={THREAD_SUMMARY_TITLE_ID} style={{ position: 'fixed', left: layout?.left ?? 0, top: layout?.top ?? 0,
      width: layout?.width ?? CARD_WIDTH, maxHeight: layout?.maxHeight ?? 0,
      visibility: layout && layout.width > 0 && layout.maxHeight > 0 ? undefined : 'hidden' }}>
    {children}
  </div>, document.body)
}
