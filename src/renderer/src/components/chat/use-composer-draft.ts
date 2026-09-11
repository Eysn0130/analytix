import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent
} from 'react'

type UseComposerDraftOptions = {
  input: string
  canCompose: boolean
}

type ComposerInputElement = HTMLTextAreaElement | HTMLDivElement

export function useComposerDraft({ input, canCompose }: UseComposerDraftOptions): {
  textareaRef: React.RefObject<ComposerInputElement | null>
  focused: boolean
  focusComposer: () => void
  onFocus: () => void
  onBlur: () => void
  onCompositionStart: () => void
  onCompositionEnd: () => void
  isComposingEvent: (event: ReactKeyboardEvent<ComposerInputElement>) => boolean
} {
  const textareaRef = useRef<ComposerInputElement | null>(null)
  const composingRef = useRef(false)
  const [focused, setFocused] = useState(false)

  const resizeTextarea = useCallback(() => {
    const el = textareaRef.current
    if (!el) return
    if (!(el instanceof HTMLTextAreaElement)) return

    el.style.height = '0px'
    const nextHeight = Math.min(el.scrollHeight, 176)
    const minHeightValue = window
      .getComputedStyle(el)
      .getPropertyValue('--composer-textarea-min-height')
      .trim()
    const minHeight = Number.parseFloat(minHeightValue) || 36
    el.style.height = `${Math.max(nextHeight, minHeight)}px`
    el.style.overflowY = el.scrollHeight > 176 ? 'auto' : 'hidden'
  }, [])

  useLayoutEffect(() => {
    resizeTextarea()
  }, [canCompose, input, resizeTextarea])

  useEffect(() => {
    const el = textareaRef.current
    if (!el || typeof ResizeObserver === 'undefined') return

    let frame = 0
    let previousWidth = el.getBoundingClientRect().width
    const observer = new ResizeObserver(([entry]) => {
      const nextWidth = entry?.contentRect.width ?? el.getBoundingClientRect().width
      if (Math.abs(nextWidth - previousWidth) < 0.5) return
      previousWidth = nextWidth
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(resizeTextarea)
    })

    observer.observe(el)

    return () => {
      window.cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [resizeTextarea])

  const focusComposer = useCallback(() => {
    window.requestAnimationFrame(() => textareaRef.current?.focus())
  }, [])

  return {
    textareaRef,
    focused,
    focusComposer,
    onFocus: () => setFocused(true),
    onBlur: () => setFocused(false),
    onCompositionStart: () => {
      composingRef.current = true
    },
    onCompositionEnd: () => {
      composingRef.current = false
    },
    isComposingEvent: (event) =>
      event.nativeEvent.isComposing || composingRef.current || event.keyCode === 229
  }
}
