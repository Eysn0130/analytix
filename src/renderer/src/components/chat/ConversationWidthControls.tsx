/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type WheelEvent } from 'react'
import { useConversationWidthPreference, writeConversationWidthPreference } from '../../lib/chat-presentation-preferences'
import css from './ConversationWidthControls.module.css'

export type ConversationPresentationPhase = 'settling' | 'hero' | 'active'

/** Renderer presentation only; navigation holds cover the existing history paths.
 * Continuable parent readiness must come from its owning projection. */
export function conversationPresentationPhase(input: {
  threadId: string | null
  hasContent: boolean
  historyLoading: boolean
  summaryBlank: boolean
  parentAvailabilityPending?: boolean
}): ConversationPresentationPhase {
  if (input.threadId === null) return 'hero'
  if (input.parentAvailabilityPending || (!input.hasContent && input.historyLoading && !input.summaryBlank)) return 'settling'
  if (input.hasContent) return 'active'
  return 'hero'
}

const CONTENT_MIN = 640
export const CONVERSATION_DEFAULT_MIN_WIDTH = 680
const CONTENT_EDGE_BUDGET = 176
export function resolveConversationContentWidth(columnWidth: number, preference: number | null): number {
  const max = Math.max(CONTENT_MIN, columnWidth - CONTENT_EDGE_BUDGET)
  if (preference !== null) return Math.min(Math.max(preference, CONTENT_MIN), max)
  return Math.max(CONVERSATION_DEFAULT_MIN_WIDTH, Math.min(columnWidth * 0.64, 920))
}

function wheelDeltaY(event: WheelEvent, scrollport: HTMLElement): number {
  if (event.deltaMode === 1) {
    const lineHeight = Number.parseFloat(getComputedStyle(scrollport).lineHeight)
    return event.deltaY * (Number.isFinite(lineHeight) ? lineHeight : 16)
  }
  if (event.deltaMode === 2) return event.deltaY * scrollport.clientHeight
  return event.deltaY
}

type WidthHandleProps = {
  side: 'left' | 'right'
  label: string
  value: number
  max: number
  container: HTMLDivElement
  reservedInlineEnd: number
  onStart: () => number
  onDrag: (width: number) => void
  onCommit: (width: number) => void
  onEnd: () => void
  onReset: () => void
}

function WidthHandle(props: WidthHandleProps) {
  const pointer = useRef<number | null>(null)
  const target = useRef<HTMLDivElement | null>(null)
  const base = useRef(0)
  const origin = useRef(0)
  const latest = useRef(0)
  const frame = useRef<number | null>(null)
  const callbacks = useRef(props)
  useLayoutEffect(() => { callbacks.current = props })

  const outwardWidth = useCallback(() => {
    const dx = latest.current - origin.current
    return base.current + (callbacks.current.side === 'right' ? dx : -dx) * 2
  }, [])
  const cancelFrame = useCallback(() => {
    if (frame.current !== null) { cancelAnimationFrame(frame.current); frame.current = null }
  }, [])
  const end = useCallback(() => {
    const id = pointer.current
    if (id === null) return
    pointer.current = null
    const handle = target.current
    target.current = null
    handle?.toggleAttribute('data-dragging', false)
    cancelFrame()
    if (handle?.hasPointerCapture(id)) handle.releasePointerCapture(id)
    callbacks.current.onEnd()
  }, [cancelFrame])
  useLayoutEffect(() => () => { end(); cancelFrame() }, [end, cancelFrame])
  useLayoutEffect(() => { end() }, [props.container, props.reservedInlineEnd, end])

  const onPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || pointer.current !== null) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    pointer.current = event.pointerId
    target.current = event.currentTarget
    origin.current = latest.current = event.clientX
    base.current = callbacks.current.onStart()
    event.currentTarget.toggleAttribute('data-dragging', true)
  }, [])
  const onPointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (pointer.current !== event.pointerId || !event.currentTarget.hasPointerCapture(event.pointerId)) return
    const box = event.currentTarget.getBoundingClientRect()
    event.currentTarget.style.setProperty('--ax-width-handle-pointer-y', `${event.clientY - box.top}px`)
    latest.current = event.clientX
    frame.current ??= requestAnimationFrame(() => {
      frame.current = null
      callbacks.current.onDrag(outwardWidth())
    })
  }, [outwardWidth])
  const onPointerUp = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (pointer.current !== event.pointerId || !event.currentTarget.hasPointerCapture(event.pointerId)) return
    cancelFrame()
    latest.current = event.clientX
    // A press must preserve a preference currently clamped by a small window.
    if (latest.current !== origin.current) callbacks.current.onCommit(outwardWidth())
    end()
  }, [cancelFrame, end, outwardWidth])
  const onPointerCancel = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (pointer.current === event.pointerId) end()
  }, [end])
  const onWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (event.ctrlKey || event.deltaY === 0) return
    const scrollport = callbacks.current.container.querySelector<HTMLElement>('[data-analytix-message-timeline-scroller]')
    if (scrollport === null) return
    scrollport.scrollBy({ top: wheelDeltaY(event, scrollport) })
  }, [])
  const onKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (pointer.current !== null || event.altKey || event.metaKey || event.ctrlKey) return
    if (event.key === 'Enter') { event.preventDefault(); callbacks.current.onReset(); return }
    const increase = callbacks.current.side === 'left' ? 'ArrowLeft' : 'ArrowRight'
    const decrease = callbacks.current.side === 'left' ? 'ArrowRight' : 'ArrowLeft'
    if (event.key !== increase && event.key !== decrease) return
    event.preventDefault()
    callbacks.current.onCommit(callbacks.current.onStart() + (event.key === increase ? 1 : -1) * (event.shiftKey ? 64 : 24))
    callbacks.current.onEnd()
  }, [])

  return <div className={css.widthHandle} data-side={props.side} data-width-handle={props.side}
    role="separator" tabIndex={0} aria-orientation="vertical" aria-label={props.label}
    aria-valuenow={Math.round(props.value)} aria-valuemin={CONTENT_MIN} aria-valuemax={Math.round(Math.max(props.value, props.max))}
    onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp}
    onPointerCancel={onPointerCancel} onLostPointerCapture={onPointerCancel} onWheel={onWheel} onKeyDown={onKeyDown} />
}

/** Own only the existing main body axis; embedded conversations keep their sizing. */
export function ConversationWidthControls({ container, phase, label, reservedInlineEnd = 0 }: {
  container: HTMLDivElement | null
  phase: ConversationPresentationPhase
  label: string
  reservedInlineEnd?: number
}) {
  const preference = useConversationWidthPreference()
  const saved = useRef(preference)
  const draft = useRef<number | null>(null)
  const mounted = useRef(false)
  const modalVisible = useRef(false)
  const [blockedByModal, setBlockedByModal] = useState(false)
  useLayoutEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const [geometry, setGeometry] = useState({ width: 680, max: CONTENT_MIN, handleSpace: 0 })
  const publishWidths = useCallback(() => {
    if (container === null || !mounted.current) return
    const column = Math.max(0, container.offsetWidth - reservedInlineEnd)
    const desired = draft.current ?? saved.current
    const width = resolveConversationContentWidth(column, desired)
    if (desired === null && reservedInlineEnd === 0) container.style.removeProperty('--ax-chat-content-axis')
    else container.style.setProperty('--ax-chat-content-axis', `${width}px`)
    const max = Math.max(CONTENT_MIN, column - CONTENT_EDGE_BUDGET)
    const handleSpace = (column - width) / 2 - 48
    setGeometry(previous => previous.width === width && previous.max === max && previous.handleSpace === handleSpace
      ? previous : { width, max, handleSpace })
  }, [container, reservedInlineEnd])

  useLayoutEffect(() => { saved.current = preference; publishWidths() }, [preference, publishWidths])
  useLayoutEffect(() => {
    if (container === null) return
    const observer = new ResizeObserver(publishWidths)
    observer.observe(container)
    publishWidths()
    return () => { observer.disconnect(); container.style.removeProperty('--ax-chat-content-axis') }
  }, [container, publishWidths])
  useLayoutEffect(() => {
    let retired = false
    const refresh = () => {
      if (retired) return
      const visible = container !== null && container.querySelector('[data-analytix-thread-handoff-modal]') !== null
      modalVisible.current = visible
      if (visible) { draft.current = null; publishWidths() }
      setBlockedByModal(visible)
    }
    refresh()
    if (container === null) return
    // Observe the actual modal seat: active operations can exist while the
    // composer branch (and therefore the modal) is not mounted.
    const observer = new MutationObserver(refresh)
    observer.observe(container, { childList: true, subtree: true })
    return () => { retired = true; observer.disconnect(); modalVisible.current = false }
  }, [container, publishWidths])
  const onStart = useCallback(() => container === null ? 680
    : resolveConversationContentWidth(container.offsetWidth - reservedInlineEnd, saved.current), [container, reservedInlineEnd])
  const onDrag = useCallback((width: number) => {
    if (modalVisible.current || container?.querySelector('[data-analytix-thread-handoff-modal]')) return
    draft.current = width; publishWidths()
  }, [container, publishWidths])
  const onCommit = useCallback((width: number) => {
    if (container === null || modalVisible.current || container.querySelector('[data-analytix-thread-handoff-modal]')) return
    saved.current = resolveConversationContentWidth(container.offsetWidth - reservedInlineEnd, width)
    writeConversationWidthPreference(saved.current)
  }, [container, reservedInlineEnd])
  const onEnd = useCallback(() => { draft.current = null; publishWidths() }, [publishWidths])
  const onReset = useCallback(() => {
    if (modalVisible.current || container?.querySelector('[data-analytix-thread-handoff-modal]')) return
    saved.current = null; writeConversationWidthPreference(null); onEnd()
  }, [container, onEnd])

  if (container === null || phase !== 'active') return null
  if (geometry.handleSpace <= 0 || blockedByModal) return null
  return (['left', 'right'] as const).map(side => <WidthHandle key={side} side={side} label={label}
    container={container} reservedInlineEnd={reservedInlineEnd} value={geometry.width} max={geometry.max}
    onStart={onStart} onDrag={onDrag} onCommit={onCommit} onEnd={onEnd} onReset={onReset} />)
}
