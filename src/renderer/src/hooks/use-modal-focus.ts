import { useLayoutEffect, useRef, useState, type RefCallback } from 'react'

// Only these existing renderer dialogs share this focus lifecycle. Their owners
// retain draft, save, close and required-onboarding decisions.
const modalLayers: HTMLElement[] = []
const modalOpeners = new WeakMap<HTMLElement, HTMLElement[]>()
const scrollLocks = new Set<HTMLElement>()
let unlockedBodyOverflow = ''
const focusableSelector = 'button,input:not([type="hidden"]),select,textarea,a[href],[tabindex]:not([tabindex="-1"])'

function available(element: HTMLElement): boolean {
  if (element.matches(':disabled') || element.closest('[hidden],[inert],[aria-hidden="true"]')) return false
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    const style = getComputedStyle(current)
    if (style.display === 'none' || style.visibility === 'hidden') return false
  }
  return true
}

export function modalFocusableElements(element: HTMLElement): HTMLElement[] {
  return [...element.querySelectorAll<HTMLElement>(focusableSelector)].filter(field => field.tabIndex >= 0 && available(field))
}

export function useModalFocus(onClose: () => void, canClose = true, lockBodyScroll = false, initialOpener?: HTMLElement | null): RefCallback<HTMLElement> {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const options = useRef({ onClose, canClose })
  options.current = { onClose, canClose }
  const openers = useRef<HTMLElement[]>([])
  const explicitOpener = useRef(initialOpener)
  const started = useRef(false)
  const leavingTopmost = useRef(false)

  useLayoutEffect(() => {
    if (!element) return
    if (!started.current) {
      const opener = explicitOpener.current ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
      const parent = opener ? [...modalLayers].reverse().find(layer => layer.contains(opener)) : undefined
      openers.current = opener ? [opener, ...(parent ? modalOpeners.get(parent) ?? [] : [])] : []
      started.current = true
    }
    modalOpeners.set(element, openers.current)
    modalLayers.push(element)
    if (lockBodyScroll) {
      if (scrollLocks.size === 0) unlockedBodyOverflow = document.body.style.overflow
      scrollLocks.add(element)
      document.body.style.overflow = 'hidden'
    }
    const topmost = (): boolean => modalLayers.at(-1) === element
    const fields = (): HTMLElement[] => modalFocusableElements(element)
    const focusFirst = (): void => {
      const initial = element.querySelector<HTMLElement>('[data-modal-autofocus]')
      const target = initial && available(initial) ? initial : fields()[0] ?? element
      target.focus({ preventScroll: true })
    }
    if (topmost()) focusFirst()
    const onKeyDown = (event: KeyboardEvent): void => {
      if (!topmost() || event.isComposing || event.keyCode === 229 || event.defaultPrevented) return
      // The in-dialog format menu owns its first Escape and exit Tab.
      if ((event.key === 'Escape' || event.key === 'Tab') && event.target instanceof Element &&
        element.contains(event.target) && event.target.closest('[data-markdown-table-menu]')) return
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopImmediatePropagation()
        if (options.current.canClose) options.current.onClose()
      } else if (event.key === 'Tab') {
        const items = fields()
        const index = items.indexOf(document.activeElement as HTMLElement)
        if (!items.length || index < 0 || (event.shiftKey ? index === 0 : index === items.length - 1)) {
          event.preventDefault()
          const target = event.shiftKey ? items.at(-1) : items[0]
          ;(target ?? element).focus({ preventScroll: true })
        }
      }
    }
    const onFocusIn = (event: FocusEvent): void => {
      if (topmost() && !element.contains(event.target as Node)) focusFirst()
    }
    document.addEventListener('keydown', onKeyDown, true)
    document.addEventListener('focusin', onFocusIn)
    return () => {
      leavingTopmost.current = topmost()
      const index = modalLayers.indexOf(element)
      if (index >= 0) modalLayers.splice(index, 1)
      if (scrollLocks.delete(element) && scrollLocks.size === 0) {
        document.body.style.overflow = unlockedBodyOverflow
      }
      document.removeEventListener('keydown', onKeyDown, true)
      document.removeEventListener('focusin', onFocusIn)
    }
  }, [element, lockBodyScroll])

  useLayoutEffect(() => () => {
    if (!leavingTopmost.current) return
    // Verify after DOM deletion: another closing layer may own the opener.
    queueMicrotask(() => {
      const remaining = modalLayers.at(-1)
      const target = openers.current.find(candidate => candidate.isConnected && available(candidate)
        && (!remaining || remaining.contains(candidate)))
      if (target) {
        target.focus({ preventScroll: true })
      } else if (remaining?.isConnected) {
        remaining.focus({ preventScroll: true })
      }
    })
  }, [])
  return setElement
}
