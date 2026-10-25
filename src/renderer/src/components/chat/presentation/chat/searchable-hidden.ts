/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useEffect, useLayoutEffect, useRef, type RefObject } from 'react'

const PRIVATE_FOCUS_OWNERS = '[data-protected-process-block],[data-protected-process-body],[data-protected-assistant-draft],'
  + '[data-protected-tool-result],[data-protected-tool-summary],[data-protected-user-input]'
const PRIVATE_FOCUS_ROOTS = PRIVATE_FOCUS_OWNERS + ',[data-protected-tool-body]'

/** Public focus can preserve a disclosure; private focus never preserves retired payload. */
export function hasPublicFocus(element: HTMLElement | null): boolean {
  const active = element?.ownerDocument.activeElement
  if (!active || !element?.contains(active)) return false
  const privateRoot = active.closest(PRIVATE_FOCUS_ROOTS)
  if (!privateRoot) return true
  // Only the strict public Job projection owner admits this readonly Inspector.
  // Keep its copy/readonly markers without preserving any protected lease body.
  const publicInspector = privateRoot.closest('[data-public-job-inspector]')?.querySelector('[data-tool-result-inspector]')
  return privateRoot === publicInspector && !privateRoot.matches(PRIVATE_FOCUS_OWNERS) &&
    !privateRoot.parentElement?.closest(PRIVATE_FOCUS_ROOTS)
}

/** Keep public DOM searchable. The reveal callback must change only public display intent. */
export function useSearchableHidden(
  hidden: boolean,
  revealPublic: () => void,
  existingRef?: RefObject<HTMLDivElement | null>,
): RefObject<HTMLDivElement | null> {
  const ownRef = useRef<HTMLDivElement>(null)
  const ref = existingRef ?? ownRef
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    if (hidden && hasPublicFocus(element)) {
      revealPublic()
      return
    }
    if (hidden) element.setAttribute('hidden', 'until-found')
    else element.removeAttribute('hidden')
  }, [hidden, revealPublic, ref])
  useEffect(() => {
    const element = ref.current
    if (!element) return
    element.addEventListener('beforematch', revealPublic)
    return () => element.removeEventListener('beforematch', revealPublic)
  }, [revealPublic, ref])
  return ref
}
