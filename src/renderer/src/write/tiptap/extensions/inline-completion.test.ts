// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Editor, getSchema } from '@tiptap/core'
import type { InlineCompletionSuggestion } from '../../inline-completion/types'
import { EditorState, TextSelection, type Transaction } from '@tiptap/pm/state'
import type { Node as PMNode } from '@tiptap/pm/model'
import { buildWriteRichExtensions, parseWriteMarkdown } from '../markdown-manager'
import { buildWriteRichMarkdownProjection } from '../markdown-projection'
import {
  WriteRichInlineCompletion,
  cancelWriteRichInlineCompletion,
  buildRichCompletionContext,
  insertCompletionText,
  writeRichInlineCompletionTestInternals
} from './inline-completion'

const schema = getSchema(buildWriteRichExtensions())

function docFromMarkdown(markdown: string): PMNode {
  return schema.nodeFromJSON(parseWriteMarkdown(markdown))
}

function stateWithCursorAfter(markdown: string, phrase: string): EditorState {
  const doc = docFromMarkdown(markdown)
  let cursor = -1
  doc.descendants((node, pos) => {
    if (cursor >= 0 || !node.isText || !node.text) return undefined
    const index = node.text.indexOf(phrase)
    if (index >= 0) cursor = pos + index + phrase.length
    return undefined
  })
  if (cursor < 0) throw new Error(`phrase not found: ${phrase}`)
  return EditorState.create({ doc, selection: TextSelection.create(doc, cursor) })
}

describe('buildRichCompletionContext', () => {
  it('produces a markdown-shaped context at the cursor', () => {
    const state = stateWithCursorAfter(
      '# 草稿\n\n- 第一项内容\n- 第二项继续写',
      '第二项继续写'
    )
    const context = buildRichCompletionContext(state, '/tmp/draft.md')
    expect(context).not.toBeNull()
    expect(context?.filePath).toBe('/tmp/draft.md')
    expect(context?.currentLinePrefix).toBe('- 第二项继续写')
    expect(context?.hasListContext).toBe(true)
    expect(context?.isAtLineEnd).toBe(true)
    expect(context?.prefixWindow).toContain('# 草稿')
  })

  it('returns null for non-empty selections', () => {
    const doc = docFromMarkdown('hello world')
    const state = EditorState.create({
      doc,
      selection: TextSelection.create(doc, 1, 6)
    })
    expect(buildRichCompletionContext(state, '/tmp/a.md')).toBeNull()
  })
})

describe('insertCompletionText', () => {
  function apply(markdown: string, phrase: string, completion: string): EditorState {
    let state = stateWithCursorAfter(markdown, phrase)
    const dispatch = (tr: Transaction): void => {
      state = state.apply(tr)
    }
    const inserted = insertCompletionText(state, dispatch, state.selection.head, completion)
    expect(inserted).toBe(true)
    return state
  }

  it('inserts plain prose as text', () => {
    const state = apply('写到一半的句子', '一半的', '内容就这样补全了。')
    expect(state.doc.textContent).toBe('写到一半的内容就这样补全了。句子')
  })

  it('parses markdown completions into real inline marks', () => {
    const state = apply('start ', 'start ', 'with **bold** tail')
    expect(state.doc.textContent).toContain('with bold tail')
    let boldFound = false
    state.doc.descendants((node) => {
      if (node.isText && node.marks.some((mark) => mark.type.name === 'bold')) boldFound = true
      return undefined
    })
    expect(boldFound).toBe(true)
  })

  it('parses multi-line completions into block nodes', () => {
    const state = apply('引子', '引子', '继续。\n\n- 列表一\n- 列表二')
    const projection = buildWriteRichMarkdownProjection(state.doc)
    expect(projection.text).toContain('- 列表一')
    expect(projection.text).toContain('- 列表二')
  })
})

describe('mapEditActionToDoc', () => {
  const { mapEditActionToDoc } = writeRichInlineCompletionTestInternals

  it('maps projected edit offsets back to the document and validates the original', () => {
    const doc = docFromMarkdown('Alpha helps writers stay aligned.')
    const state = EditorState.create({ doc })
    const projection = buildWriteRichMarkdownProjection(doc)
    const from = projection.text.indexOf('helps')
    const action = {
      kind: 'edit' as const,
      from,
      to: from + 'helps'.length,
      original: 'helps',
      replacement: 'guides',
      scopeKind: 'selection' as const
    }
    const mapped = mapEditActionToDoc(state, action)
    expect(mapped).not.toBeNull()
    expect(doc.textBetween(mapped!.from, mapped!.to)).toBe('helps')
  })

  it('rejects stale actions whose original text no longer matches', () => {
    const doc = docFromMarkdown('Alpha helps writers stay aligned.')
    const state = EditorState.create({ doc })
    const action = {
      kind: 'edit' as const,
      from: 0,
      to: 5,
      original: 'Bravo',
      replacement: 'x',
      scopeKind: 'selection' as const
    }
    expect(mapEditActionToDoc(state, action)).toBeNull()
  })
})


describe('rich completion cancellation', () => {
  let editor: Editor
  let element: HTMLDivElement
  let resolve: (value: InlineCompletionSuggestion | null) => void
  let signal: AbortSignal
  let longEnabled = false
  let request: ReturnType<typeof vi.fn<(context: unknown, mode: unknown, signal: AbortSignal) => Promise<InlineCompletionSuggestion | null>>>
  beforeEach(async () => {
    vi.useFakeTimers()
    longEnabled = false
    element = document.createElement('div')
    document.body.append(element)
    request = vi.fn((_context, _mode, ownedSignal: AbortSignal) => {
      signal = ownedSignal
      return new Promise<InlineCompletionSuggestion | null>(done => { resolve = done })
    })
    editor = new Editor({ element, extensions: [...buildWriteRichExtensions(), WriteRichInlineCompletion.configure({
      getDebounceMs: () => 1, isEnabled: () => true, isLongEnabled: () => longEnabled, getLongDebounceMs: () => 2, getFilePath: () => '/workspace/plan.md', requestCompletion: request
    })], content: '<p>This is an existing paragraph </p>' })
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    await vi.advanceTimersByTimeAsync(10)
    expect(request).toHaveBeenCalledOnce()
    expect(signal).toBeInstanceOf(AbortSignal)
  })
  afterEach(() => {
    editor.destroy()
    element.remove()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })
  it.each(['document', 'selection', 'blur', 'focus', 'cancel', 'destroy'])('aborts on %s and drops a late successful response', async change => {
    const originalSignal = signal
    if (change === 'document') editor.commands.insertContent('more ')
    if (change === 'selection') editor.commands.setTextSelection(3)
    if (change === 'blur') editor.view.dom.dispatchEvent(new FocusEvent('blur'))
    if (change === 'focus') editor.view.dom.dispatchEvent(new FocusEvent('focus'))
    if (change === 'cancel') cancelWriteRichInlineCompletion(editor.view)
    if (change === 'destroy') editor.destroy()
    expect(originalSignal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(request).toHaveBeenCalledOnce()
    resolve({ text: 'a focused continuation', mode: 'short' })
    await Promise.resolve(); await Promise.resolve()
    expect(element.querySelector('.write-rich-ghost-text')).toBeNull()
  })
  it('releases the in-flight mode only after the cancelled promise settles', async () => {
    const originalSignal = signal
    editor.commands.insertContent('more ')
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
    editor.destroy()
    resolve(null)
    await Promise.resolve(); await Promise.resolve()
    request.mockClear()
    editor = new Editor({ element, extensions: [...buildWriteRichExtensions(), WriteRichInlineCompletion.configure({
      getDebounceMs: () => 1, isEnabled: () => true, isLongEnabled: () => true, getLongDebounceMs: () => 2,
      getFilePath: () => '/workspace/plan.md', requestCompletion: request
    })], content: '<p>This is an existing paragraph </p>' })
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
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
