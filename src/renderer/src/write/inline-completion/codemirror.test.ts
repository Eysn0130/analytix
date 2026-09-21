// @vitest-environment jsdom
import { EditorState } from '@codemirror/state'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EditorView } from '@codemirror/view'
import type { InlineCompletionSuggestion } from './types'
import type { InlineCompletionRequestContext } from './types'
import {
  buildInlineCompletionExtension,
  cancelInlineCompletion,
  inlineCompletionMinRequestInterval,
  inlineCompletionRequestSignature,
  inlineEditReplacementAnchor,
  isInlineCompletionEmptyFeedback
} from './codemirror'

describe('inline edit CodeMirror placement', () => {
  it('anchors single-line replacements after the edited text', () => {
    const state = EditorState.create({ doc: 'Alpha helps writers.' })

    expect(inlineEditReplacementAnchor(state, {
      kind: 'edit',
      from: 0,
      to: 5,
      original: 'Alpha',
      replacement: 'Write mode',
      scopeKind: 'selection'
    })).toEqual({
      position: 5,
      leading: false
    })
  })

  it('anchors multi-line replacements at the start of the edited scope', () => {
    const state = EditorState.create({
      doc: 'First paragraph line one\nline two\n\nNext paragraph'
    })

    expect(inlineEditReplacementAnchor(state, {
      kind: 'edit',
      from: 0,
      to: 'First paragraph line one\nline two'.length,
      original: 'First paragraph line one\nline two',
      replacement: 'Shorter paragraph',
      scopeKind: 'selection'
    })).toEqual({
      position: 0,
      leading: true
    })
  })
})

function context(partial: Partial<InlineCompletionRequestContext> = {}): InlineCompletionRequestContext {
  return {
    filePath: '/tmp/workspace/draft.md',
    language: 'markdown',
    head: 19,
    lineNumber: 3,
    column: 11,
    docLength: 46,
    prefix: '# Draft\n\nWrite mode',
    suffix: ' keeps terminology aligned.',
    prefixWindow: '# Draft\n\nWrite mode',
    suffixWindow: ' keeps terminology aligned.',
    currentLinePrefix: 'Write mode',
    currentLineSuffix: ' keeps terminology aligned.',
    currentLineText: 'Write mode keeps terminology aligned.',
    previousLineText: '',
    previousNonEmptyLineText: '# Draft',
    nextLineText: '',
    indentation: '',
    isAtLineEnd: false,
    currentLinePrefixTrimmed: 'Write mode',
    currentLineSuffixTrimmed: 'keeps terminology aligned.',
    docPreview: '# Draft\n\nWrite mode',
    isBlankLine: false,
    hasMeaningfulPrefix: true,
    hasStructuralContext: false,
    hasListContext: false,
    hasQuoteContext: false,
    hasHeadingContext: false,
    hasTableContext: false,
    endsWithWordChar: true,
    endsWithSentencePunctuation: false,
    previousLineEndsWithSentencePunctuation: false,
    prefersNewLineCompletion: false,
    isParagraphBreakOpportunity: false,
    nextCharIsWord: false,
    looksLikeUrlTail: false,
    ...partial
  }
}

describe('inline completion request pacing helpers', () => {
  it('builds a stable signature for identical local request context', () => {
    expect(inlineCompletionRequestSignature(context(), 'short')).toBe(
      inlineCompletionRequestSignature(context(), 'short')
    )
    expect(inlineCompletionRequestSignature(context(), 'short')).not.toBe(
      inlineCompletionRequestSignature(context({ head: 20, currentLinePrefix: 'Write mode ' }), 'short')
    )
  })

  it('uses a longer request interval for long completion', () => {
    expect(inlineCompletionMinRequestInterval('long')).toBeGreaterThan(
      inlineCompletionMinRequestInterval('short')
    )
  })

  it('classifies empty model results for cooldown', () => {
    expect(isInlineCompletionEmptyFeedback('empty-candidate')).toBe(true)
    expect(isInlineCompletionEmptyFeedback('blank-candidate')).toBe(true)
    expect(isInlineCompletionEmptyFeedback('low-confidence')).toBe(false)
  })
})


describe('CodeMirror completion cancellation', () => {
  let view: EditorView
  let parent: HTMLDivElement
  let resolve: (value: InlineCompletionSuggestion | null) => void
  let signal: AbortSignal
  let longEnabled = false
  let request: ReturnType<typeof vi.fn<(context: unknown, mode: unknown, signal: AbortSignal) => Promise<InlineCompletionSuggestion | null>>>
  beforeEach(async () => {
    vi.useFakeTimers()
    longEnabled = false
    const range = document.createRange.bind(document)
    vi.spyOn(document, 'createRange').mockImplementation(() => Object.assign(range(), {
      getClientRects: () => [], getBoundingClientRect: () => new DOMRect()
    }))
    parent = document.createElement('div')
    document.body.append(parent)
    request = vi.fn((_context, _mode, ownedSignal: AbortSignal) => {
      signal = ownedSignal
      return new Promise<InlineCompletionSuggestion | null>(done => { resolve = done })
    })
    const text = 'This is an existing paragraph '
    view = new EditorView({ parent, state: EditorState.create({ doc: text, selection: { anchor: text.length },
      extensions: buildInlineCompletionExtension({ debounceMs: 1, isEnabled: () => true, isLongEnabled: () => longEnabled, getLongDebounceMs: () => 2,
        getFilePath: () => '/workspace/plan.md', requestCompletion: request }) }) })
    await vi.advanceTimersByTimeAsync(10)
    expect(request).toHaveBeenCalledOnce()
    expect(signal).toBeInstanceOf(AbortSignal)
  })
  afterEach(() => {
    view.destroy()
    parent.remove()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })
  it.each(['document', 'selection', 'blur', 'focus', 'cancel', 'destroy'])('aborts on %s and drops a late successful response', async change => {
    const originalSignal = signal
    if (change === 'document') view.dispatch({ changes: { from: view.state.doc.length, insert: 'more ' }, selection: { anchor: view.state.doc.length + 5 } })
    if (change === 'selection') view.dispatch({ selection: { anchor: 3 } })
    if (change === 'blur') view.contentDOM.dispatchEvent(new FocusEvent('blur'))
    if (change === 'focus') view.contentDOM.dispatchEvent(new FocusEvent('focus'))
    if (change === 'cancel') cancelInlineCompletion(view)
    if (change === 'destroy') view.destroy()
    expect(originalSignal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(request).toHaveBeenCalledOnce()
    resolve({ text: 'a focused continuation', mode: 'short' })
    await Promise.resolve(); await Promise.resolve()
    expect(parent.querySelector('.cm-inline-completion')).toBeNull()
  })
  it('keeps a cancelled request in flight until settlement, then schedules the current document', async () => {
    const originalSignal = signal
    view.dispatch({ changes: { from: view.state.doc.length, insert: 'more ' }, selection: { anchor: view.state.doc.length + 5 } })
    await vi.advanceTimersByTimeAsync(5000)
    expect(originalSignal.aborted).toBe(true)
    expect(request).toHaveBeenCalledOnce()
    resolve(null)
    await vi.advanceTimersByTimeAsync(10)
    expect(request).toHaveBeenCalledTimes(2)
    expect(signal).not.toBe(originalSignal)
    expect(signal.aborted).toBe(false)
    resolve(null)
    await Promise.resolve()
  })
  it('serializes short and long requests until the shared Core owner settles', async () => {
    view.destroy()
    resolve(null)
    await Promise.resolve(); await Promise.resolve()
    request.mockClear()
    const text = 'This is an existing paragraph '
    view = new EditorView({ parent, state: EditorState.create({ doc: text, selection: { anchor: text.length },
      extensions: buildInlineCompletionExtension({ debounceMs: 1, isEnabled: () => true,
        isLongEnabled: () => true, getLongDebounceMs: () => 2,
        getFilePath: () => '/workspace/plan.md', requestCompletion: request }) }) })
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledOnce()
    expect(request.mock.calls[0][1]).toBe('short')
    const shortSignal = signal
    await vi.advanceTimersByTimeAsync(5000)
    expect(request).toHaveBeenCalledOnce()
    expect(shortSignal.aborted).toBe(false)
    resolve(null)
    await vi.advanceTimersByTimeAsync(10)
    expect(request).toHaveBeenCalledTimes(2)
    expect(request.mock.calls[1][1]).toBe('long')
    resolve(null)
    await Promise.resolve()
  })

})
