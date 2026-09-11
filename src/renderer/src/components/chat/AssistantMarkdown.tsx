import type { ReactElement } from 'react'
import { lazy, Suspense, useEffect, useId, useRef, useState } from 'react'
import { sharedMarkdownFinalizationScheduler } from '../../thread/streaming/markdown-finalization-queue'
import {
  createThreadTraceEvent,
  PersistedThreadTraceSink
} from '../../thread/tracing/thread-performance-trace'

const loadStreamdownAssistant = () =>
  import('./StreamdownAssistant').then((module) => ({ default: module.StreamdownAssistant }))

const LazyStreamdownAssistant = lazy(loadStreamdownAssistant)

export function preloadAssistantMarkdownRenderer(): void {
  void loadStreamdownAssistant()
}

const markdownTraceSink = new PersistedThreadTraceSink()
const DEFER_MARKDOWN_CHARS = 6000
const DEFER_MARKDOWN_CODE_FENCES = 4
const DEFER_MARKDOWN_LINES = 200
const DEFER_MARKDOWN_CODE_CHARS = 8000
const LIGHTWEIGHT_MARKDOWN_CHARS = 24000
const LIGHTWEIGHT_MARKDOWN_CODE_FENCES = 10
const LIGHTWEIGHT_MARKDOWN_LINES = 700
const LIGHTWEIGHT_MARKDOWN_CODE_CHARS = 16000
const DEFERRED_FINALIZATION_DELAY_MS = 240
const STREAM_PREFIX_CHARS = 24
const CATCHUP_DIVISOR = 6
const SMALL_BACKLOG_STEP = 64
const MEDIUM_BACKLOG_STEP = 256
const LARGE_BACKLOG_STEP = 1024
const COMBINING_MARK_REGEX = /\p{Mark}/u
const VARIATION_SELECTOR_REGEX = /\p{Variation_Selector}/u
const graphemeSegmenter =
  typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function'
    ? new Intl.Segmenter(undefined, { granularity: 'grapheme' })
    : null

export type MarkdownFinalizationMode = 'immediate' | 'defer' | 'lightweight'

export type MarkdownFinalizationPolicy = {
  mode: MarkdownFinalizationMode
  chars: number
  codeFences: number
  estimatedLines: number
  codeBlockChars: number
}

function countCodeBlockChars(text: string): number {
  let total = 0
  const fencedBlockPattern = /```[^\n\r]*(?:\r?\n)?([\s\S]*?)```/g
  for (const match of text.matchAll(fencedBlockPattern)) {
    total += match[1]?.length ?? 0
  }
  return total
}

export function getMarkdownFinalizationPolicy(text: string): MarkdownFinalizationPolicy {
  const chars = text.length
  const codeFences = text.match(/```/g)?.length ?? 0
  const estimatedLines = text.length === 0 ? 0 : text.split(/\r\n|\r|\n/).length
  const codeBlockChars = countCodeBlockChars(text)

  if (
    chars > LIGHTWEIGHT_MARKDOWN_CHARS ||
    codeFences >= LIGHTWEIGHT_MARKDOWN_CODE_FENCES ||
    estimatedLines > LIGHTWEIGHT_MARKDOWN_LINES ||
    codeBlockChars > LIGHTWEIGHT_MARKDOWN_CODE_CHARS
  ) {
    return { mode: 'lightweight', chars, codeFences, estimatedLines, codeBlockChars }
  }

  if (
    chars > DEFER_MARKDOWN_CHARS ||
    codeFences >= DEFER_MARKDOWN_CODE_FENCES ||
    estimatedLines > DEFER_MARKDOWN_LINES ||
    codeBlockChars > DEFER_MARKDOWN_CODE_CHARS
  ) {
    return { mode: 'defer', chars, codeFences, estimatedLines, codeBlockChars }
  }

  return { mode: 'immediate', chars, codeFences, estimatedLines, codeBlockChars }
}

function scheduleDeferredMarkdownFinalization(callback: () => void): () => void {
  if (typeof window === 'undefined') {
    const handle = globalThis.setTimeout(callback, 0)
    return () => globalThis.clearTimeout(handle)
  }

  let cancelled = false
  let timeoutHandle: number | null = null
  let frameHandle: number | null = null

  const runWhenIdle = (): void => {
    if (cancelled) return
    const requestIdle = window.requestIdleCallback
    if (typeof requestIdle === 'function') {
      timeoutHandle = requestIdle(() => {
        timeoutHandle = null
        if (!cancelled) callback()
      }, { timeout: DEFERRED_FINALIZATION_DELAY_MS }) as unknown as number
      return
    }
    frameHandle = window.requestAnimationFrame(() => {
      frameHandle = null
      if (!cancelled) callback()
    })
  }

  timeoutHandle = window.setTimeout(runWhenIdle, DEFERRED_FINALIZATION_DELAY_MS)
  return () => {
    cancelled = true
    if (timeoutHandle !== null) {
      window.clearTimeout(timeoutHandle)
      if (typeof window.cancelIdleCallback === 'function') {
        window.cancelIdleCallback(timeoutHandle)
      }
    }
    if (frameHandle !== null) window.cancelAnimationFrame(frameHandle)
  }
}

function fallbackBoundary(text: string, length: number): number {
  let boundary = length
  const previousCode = text.charCodeAt(boundary - 1)
  if (previousCode >= 0xd800 && previousCode <= 0xdbff && boundary < text.length) {
    boundary += 1
  }

  while (boundary < text.length) {
    const codePoint = text.codePointAt(boundary)
    if (codePoint == null) break
    const char = String.fromCodePoint(codePoint)
    if (COMBINING_MARK_REGEX.test(char) || VARIATION_SELECTOR_REGEX.test(char)) {
      boundary += char.length
      continue
    }
    if (codePoint === 0x200d) {
      boundary += 1
      const joinedCodePoint = text.codePointAt(boundary)
      if (joinedCodePoint == null) break
      boundary += String.fromCodePoint(joinedCodePoint).length
      continue
    }
    break
  }

  return boundary
}

function nextTextBoundary(text: string, visibleLength: number): number {
  const length = Math.max(0, Math.min(visibleLength, text.length))
  if (length === 0 || length === text.length) return length

  if (graphemeSegmenter) {
    for (const segment of graphemeSegmenter.segment(text)) {
      const boundary = segment.index + segment.segment.length
      if (boundary >= length) return boundary
    }
  }

  return fallbackBoundary(text, length)
}

export function visiblePlainTextForTypewriter(text: string, visibleLength: number): string {
  return text.slice(0, nextTextBoundary(text, visibleLength))
}

export function nextPlainTextVisibleLength(current: number, target: number): number {
  if (current === target) return current
  if (current > target) return target
  const backlog = target - current
  const maxStep = backlog > 8000
    ? LARGE_BACKLOG_STEP
    : backlog > 2000
      ? MEDIUM_BACKLOG_STEP
      : SMALL_BACKLOG_STEP
  const proportionalStep = Math.ceil(backlog / CATCHUP_DIVISOR)
  return current + Math.min(backlog, Math.max(1, Math.min(maxStep, proportionalStep)))
}

export function shouldDeferMarkdownFinalizationForPacedText({
  streaming,
  hadStreaming,
  pacedCaughtUp
}: {
  streaming: boolean
  hadStreaming: boolean
  pacedCaughtUp: boolean
}): boolean {
  return !streaming && hadStreaming && !pacedCaughtUp
}

function usePacedPlainText(text: string, pace: boolean): { text: string; caughtUp: boolean } {
  const canAnimate = typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function'
  const [visibleLength, setVisibleLength] = useState(() => (
    pace && canAnimate ? Math.min(text.length, STREAM_PREFIX_CHARS) : text.length
  ))
  const targetRef = useRef(text.length)
  targetRef.current = text.length

  useEffect(() => {
    if (!pace || !canAnimate) {
      setVisibleLength(text.length)
      return
    }
    setVisibleLength((current) => {
      if (current > text.length) return text.length
      if (current === 0 && text.length > 0) return Math.min(text.length, STREAM_PREFIX_CHARS)
      return current
    })
  }, [canAnimate, pace, text.length])

  useEffect(() => {
    if (!pace || !canAnimate) return
    if (visibleLength >= text.length) return
    const frame = window.requestAnimationFrame(() => {
      setVisibleLength((current) => nextPlainTextVisibleLength(current, targetRef.current))
    })
    return () => window.cancelAnimationFrame(frame)
  }, [canAnimate, pace, text.length, visibleLength])

  if (!pace || !canAnimate) return { text, caughtUp: true }
  const boundedVisibleLength = Math.min(visibleLength, text.length)
  return {
    text: visiblePlainTextForTypewriter(text, boundedVisibleLength),
    caughtUp: boundedVisibleLength >= text.length
  }
}

export function AssistantMarkdown({
  text,
  streaming,
  className,
  rowId,
  onFinalized
}: {
  text: string
  streaming: boolean
  className?: string
  rowId?: string
  onFinalized?: () => void
}): ReactElement {
  const generatedRowId = useId()
  const effectiveRowId = rowId ?? `assistant-markdown:${generatedRowId}`
  const [finalized, setFinalized] = useState(() => (
    !streaming && getMarkdownFinalizationPolicy(text).mode === 'immediate'
  ))
  const latestTextRef = useRef(text)
  const lastFinalizedTextRef = useRef(finalized ? text : '')
  const hadStreamingRef = useRef(streaming)
  const shouldPacePlainText = streaming || (hadStreamingRef.current && !finalized)
  const pacedPlainText = usePacedPlainText(text, shouldPacePlainText)

  useEffect(() => {
    latestTextRef.current = text
  }, [text])

  useEffect(() => {
    if (streaming) {
      hadStreamingRef.current = true
      lastFinalizedTextRef.current = ''
      setFinalized(false)
      return
    }
    if (shouldDeferMarkdownFinalizationForPacedText({
      streaming,
      hadStreaming: hadStreamingRef.current,
      pacedCaughtUp: pacedPlainText.caughtUp
    })) {
      setFinalized(false)
      return
    }
    if (finalized && lastFinalizedTextRef.current === text) return

    const policy = getMarkdownFinalizationPolicy(text)
    setFinalized(false)
    if (policy.mode === 'lightweight') {
      lastFinalizedTextRef.current = ''
      hadStreamingRef.current = false
      return
    }

    const enqueue = (): (() => void) => sharedMarkdownFinalizationScheduler.enqueue({
      rowId: effectiveRowId,
      text,
      onFinalize: (job) => {
        if (latestTextRef.current !== job.text) return
        lastFinalizedTextRef.current = job.text
        hadStreamingRef.current = false
        setFinalized(true)
        markdownTraceSink.record(
          createThreadTraceEvent('thread.markdown.finalized', {
            data: {
              chars: policy.chars,
              codeFences: policy.codeFences,
              estimatedLines: policy.estimatedLines,
              codeBlockChars: policy.codeBlockChars,
              deferred: policy.mode === 'defer'
            }
          })
        )
      }
    })

    if (policy.mode === 'defer') {
      let cancelQueuedJob: (() => void) | undefined
      const cancelSchedule = scheduleDeferredMarkdownFinalization(() => {
        cancelQueuedJob = enqueue()
      })
      return () => {
        cancelSchedule()
        cancelQueuedJob?.()
      }
    }

    return enqueue()
  }, [effectiveRowId, finalized, pacedPlainText.caughtUp, streaming, text])

  useEffect(() => {
    if (!finalized || streaming || !onFinalized || typeof window === 'undefined') return
    let frame = 0
    let frameId: number | null = null
    const settle = (): void => {
      onFinalized()
      frame += 1
      if (frame < 8) {
        frameId = window.requestAnimationFrame(settle)
      }
    }
    frameId = window.requestAnimationFrame(settle)
    return () => {
      if (frameId !== null) window.cancelAnimationFrame(frameId)
    }
  }, [finalized, onFinalized, streaming, text])

  if (shouldPacePlainText) {
    return (
      <div className={className}>
        <span className="whitespace-pre-wrap break-words">{pacedPlainText.text}</span>
      </div>
    )
  }

  if (!finalized) {
    return (
      <div className={className}>
        <span className="whitespace-pre-wrap break-words">{text}</span>
      </div>
    )
  }

  return (
    <Suspense
      fallback={
        <div className={className}>
          {text}
        </div>
      }
    >
      <LazyStreamdownAssistant text={text} streaming={streaming} className={className} />
    </Suspense>
  )
}
