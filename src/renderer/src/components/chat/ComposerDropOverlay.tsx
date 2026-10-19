/* Presentation and nested-drag lifecycle adapted from DeepSeek Harness
 * 687f282e8625c6b8d11ff58211a90524be2e4317, ui-attachment DropOverlay/drop-events.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { useId, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import css from './ComposerDropOverlay.module.css'

export type ComposerDropOverlayProps = {
  enabled: boolean
  ownerKey: string
  rootRef: RefObject<HTMLElement | null>
  canAccept: (transfer: DataTransfer) => boolean
  /** Existing composer intake owner; this leaf never reads paths or files. */
  onDrop: (event: DragEvent) => void
  label: string
}

/** A document-level invitation over the composer's existing drop authority. */
export function ComposerDropOverlay({ enabled, ownerKey, rootRef, canAccept, onDrop, label }: ComposerDropOverlayProps) {
  const [activeOwner, setActiveOwner] = useState<string | null>(null)
  const current = useRef({ enabled, ownerKey, rootRef, canAccept, onDrop })
  current.current = { enabled, ownerKey, rootRef, canAccept, onDrop }
  const illustrationId = useId()

  useLayoutEffect(() => {
    let depth = 0
    const reset = (): void => {
      depth = 0
      setActiveOwner(active => active === ownerKey ? null : active)
    }
    reset()
    if (!enabled) return

    const fileTransfer = (event: DragEvent): DataTransfer | null => {
      const live = current.current
      if (!live.enabled || live.ownerKey !== ownerKey || live.rootRef !== rootRef || !rootRef.current) return null
      const transfer = event.dataTransfer
      return transfer && Array.from(transfer.types).includes('Files') ? transfer : null
    }
    const acceptedTransfer = (event: DragEvent): DataTransfer | null => {
      const transfer = fileTransfer(event)
      return transfer && current.current.canAccept(transfer) ? transfer : null
    }
    const enter = (event: DragEvent): void => {
      const transfer = acceptedTransfer(event)
      if (!transfer) return
      event.preventDefault()
      depth += 1
      setActiveOwner(ownerKey)
    }
    const over = (event: DragEvent): void => {
      const transfer = acceptedTransfer(event)
      if (!transfer) {
        if (fileTransfer(event)) reset()
        return
      }
      event.preventDefault()
      transfer.dropEffect = 'copy'
    }
    const leave = (event: DragEvent): void => {
      if (!fileTransfer(event)) return
      depth = Math.max(0, depth - 1)
      const target = event.target
      const leftViewport = event.clientX <= 0 || event.clientY <= 0 ||
        event.clientX >= window.innerWidth || event.clientY >= window.innerHeight
      if (depth === 0 || (target === document.documentElement || target === document.body) && leftViewport) reset()
    }
    const drop = (event: DragEvent): void => {
      const transfer = acceptedTransfer(event)
      const active = depth > 0
      reset()
      const root = rootRef.current
      // The resident card already owns its bubbling drop. Other consumed drop
      // targets keep their owner too; forwarding either would intake twice.
      if (!active || !transfer || !root || event.defaultPrevented || event.target instanceof Node && root.contains(event.target)) return
      event.preventDefault()
      current.current.onDrop(event)
    }
    const escape = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') reset()
    }
    document.addEventListener('dragenter', enter)
    document.addEventListener('dragover', over)
    document.addEventListener('dragleave', leave)
    document.addEventListener('drop', drop)
    document.addEventListener('keydown', escape)
    window.addEventListener('dragend', reset)
    return () => {
      reset()
      document.removeEventListener('dragenter', enter)
      document.removeEventListener('dragover', over)
      document.removeEventListener('dragleave', leave)
      document.removeEventListener('drop', drop)
      document.removeEventListener('keydown', escape)
      window.removeEventListener('dragend', reset)
    }
  }, [enabled, ownerKey, rootRef, canAccept])

  if (!enabled || activeOwner !== ownerKey) return null
  return createPortal(
    <div className={css.mask} role="status" data-composer-drop-overlay data-analytix-chat-token-scope>
      <div className={css.wrap}>
        <div className={css.illustration} aria-hidden="true">
          <svg width="115" height="84" viewBox="0 0 115 84" fill="none" xmlns="http://www.w3.org/2000/svg">
            <g clipPath={`url(#${illustrationId})`}>
              <rect y="17.0742" width="44.1832" height="43.6431" rx="12" transform="rotate(-22.7338 0 17.0742)" fill="#9CE5ED" />
              <rect x="73.4043" y="8.54297" width="43.7267" height="50.5284" rx="8" transform="rotate(17.403 73.4043 8.54297)" fill="#679EFE" />
              <path d="M30.4917 28.1369L40.8865 33.4564L37.2232 34.9524L29.5302 31.0159L26.7919 39.2122L23.1285 40.7082L26.8287 29.6338L16.8967 24.5516L20.5601 23.0556L27.7902 26.7549L30.3639 19.052L34.0273 17.556L30.4917 28.1369Z" fill="white" />
              <path d="M77.5088 26.3047L101.057 33.7966" stroke="white" strokeWidth="3" />
              <path d="M72.2646 42.7871L86.3938 47.2823" stroke="white" strokeWidth="3" />
              <path d="M74.8867 34.5469L98.4353 42.0388" stroke="white" strokeWidth="3" />
              <rect x="31.583" y="38.6641" width="44.9157" height="44.3666" rx="12" transform="rotate(-0.134233 31.583 38.6641)" fill="#3964FE" />
              <path d="M38.9521 73.0337C39.6129 71.7086 41.7113 66.0937 43.5113 61.1663C44.1607 59.3885 46.7484 59.3923 47.4591 61.1465C48.9728 64.8828 50.7969 68.6922 51.9988 69.1925C54.2946 70.1482 57.9854 59.3573 68.0064 70.1801" stroke="white" strokeWidth="3" />
              <circle cx="60.6157" cy="52.247" r="4.38794" transform="rotate(22.5996 60.6157 52.247)" fill="white" />
            </g>
            <defs><clipPath id={illustrationId}><rect width="115" height="84" fill="white" /></clipPath></defs>
          </svg>
        </div>
        <div className={css.title}>{label}</div>
      </div>
    </div>,
    document.body
  )
}
