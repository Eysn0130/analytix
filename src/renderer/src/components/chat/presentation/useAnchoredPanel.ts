/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties } from 'react'
import { overlayTopMargin } from './overlay-top-margin'
import { useDismissOnOutsidePointer } from './useDismissOnOutsidePointer'

/** Click-owned, nonmodal renderer panel. Source data and authority stay with its consumer. */
export function useAnchoredPanel<TAnchor extends HTMLElement>({ enabled, ownerKey }: {
  enabled: boolean
  ownerKey?: string | null
}) {
  const [state, setState] = useState({ ownerKey, enabled, open: false })
  // Mask and retire on the first render, including A -> B -> A. A passive
  // cleanup would expose the old owner's panel for a committed frame.
  const sameOwner = state.ownerKey === ownerKey && state.enabled === enabled
  if (!sameOwner) setState({ ownerKey, enabled, open: false })
  const open = sameOwner && enabled && state.open
  const setOpen = useCallback((next: boolean) => {
    setState({ ownerKey, enabled, open: enabled && next })
  }, [ownerKey, enabled])
  const rootRef = useRef<TAnchor>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState<CSSProperties | null>(null)
  const id = useId()

  useLayoutEffect(() => {
    if (!open) { setPosition(null); return }
    const place = (): void => {
      const rect = rootRef.current?.getBoundingClientRect(), panel = panelRef.current
      if (!rect || !panel) return
      const topMargin = overlayTopMargin(12)
      const maxHeight = Math.max(0, window.innerHeight - topMargin - 12)
      const width = Math.min(panel.offsetWidth, Math.max(0, window.innerWidth - 24))
      const height = Math.min(panel.offsetHeight, maxHeight)
      const left = Math.max(12, Math.min(rect.left, window.innerWidth - width - 12))
      const top = Math.max(topMargin, Math.min(rect.top - 8 - height, window.innerHeight - height - 12))
      setPosition({ left, top, maxHeight })
    }
    place()
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(place)
    if (rootRef.current) observer?.observe(rootRef.current)
    if (panelRef.current) observer?.observe(panelRef.current)
    return () => {
      observer?.disconnect()
      window.removeEventListener('scroll', place, true)
      window.removeEventListener('resize', place)
    }
  }, [open])

  useDismissOnOutsidePointer(rootRef, open, setOpen, panelRef)
  useEffect(() => {
    if (!open) return
    const outside = (event: Event): void => {
      if (event.target instanceof Node && rootRef.current?.contains(event.target) !== true &&
          panelRef.current?.contains(event.target) !== true) setOpen(false)
    }
    const escape = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      event.stopPropagation()
      // Never steal focus from the editor or an outside-pointer destination.
      // Only return focus when dismissing an interaction inside this panel.
      if (panelRef.current?.contains(document.activeElement)) triggerRef.current?.focus({ preventScroll: true })
      setOpen(false)
    }
    document.addEventListener('click', outside, true)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('click', outside, true)
      document.removeEventListener('keydown', escape)
    }
  }, [open, setOpen])

  return { open, setOpen, rootRef, triggerRef, panelRef, id,
    style: position ?? { visibility: 'hidden', left: 0, top: 0 } as CSSProperties }
}
