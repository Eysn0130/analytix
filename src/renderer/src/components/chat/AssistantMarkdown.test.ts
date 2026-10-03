// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import i18n from '../../i18n'
import {
  AssistantMarkdown,
  getMarkdownFinalizationPolicy,
  nextPlainTextVisibleLength,
  shouldDeferMarkdownFinalizationForPacedText,
  visiblePlainTextForTypewriter
} from './AssistantMarkdown'

describe('getMarkdownFinalizationPolicy', () => {
  it('allows small markdown to finalize through the normal queue', () => {
    expect(getMarkdownFinalizationPolicy('Short **answer**.')).toMatchObject({
      mode: 'immediate',
      chars: 17,
      codeFences: 0
    })
  })

  it('defers long markdown instead of rich-rendering it immediately', () => {
    const text = Array.from({ length: 220 }, (_, index) => `line ${index + 1}`).join('\n')

    expect(getMarkdownFinalizationPolicy(text)).toMatchObject({
      mode: 'defer',
      estimatedLines: 220
    })
  })

  it('keeps very heavy code-heavy markdown on the lightweight surface', () => {
    const code = 'x'.repeat(17000)
    const text = [
      'Analysis:',
      '',
      '```ts',
      code,
      '```'
    ].join('\n')

    expect(getMarkdownFinalizationPolicy(text)).toMatchObject({
      mode: 'lightweight',
      codeBlockChars: 17001
    })
  })
})

describe('visiblePlainTextForTypewriter', () => {
  it('does not cut a surrogate-pair emoji in half', () => {
    expect(visiblePlainTextForTypewriter('A🙂B', 2)).toBe('A🙂')
  })

  it('keeps combining marks attached to their base character', () => {
    const text = `Cafe\u0301 done`

    expect(visiblePlainTextForTypewriter(text, 4)).toBe('Cafe\u0301')
  })

  it('does not cut a zwj emoji sequence in half', () => {
    expect(visiblePlainTextForTypewriter('A👨‍👩‍👧‍👦B', 2)).toBe('A👨‍👩‍👧‍👦')
  })

  it('snaps backwards on replacement without negative animation', () => {
    expect(nextPlainTextVisibleLength(20, 5)).toBe(5)
  })

  it('catches up large streaming backlogs without a long typewriter queue', () => {
    expect(nextPlainTextVisibleLength(0, 10000)).toBeGreaterThanOrEqual(1000)
  })

  it('keeps the plain paced surface until a completed stream catches up', () => {
    expect(shouldDeferMarkdownFinalizationForPacedText({
      streaming: false,
      hadStreaming: true,
      pacedCaughtUp: false
    })).toBe(true)
    expect(shouldDeferMarkdownFinalizationForPacedText({
      streaming: false,
      hadStreaming: true,
      pacedCaughtUp: true
    })).toBe(false)
  })
})

const heavyCases = [
  ['characters', 'x'.repeat(24001)],
  ['lines', Array.from({ length: 701 }, () => 'line').join('\n')],
  ['five closed fences', Array.from({ length: 5 }, () => '```text\nx\n```').join('\n')],
  ['aggregate code', '```text\n' + 'x'.repeat(16000) + '\n```']
] as const

describe('unchanged heavyweight policy boundaries', () => {
  it.each([
    ['characters at limit', 'x'.repeat(24000)],
    ['lines at limit', Array.from({ length: 700 }, () => 'line').join('\n')],
    ['eight delimiters', Array.from({ length: 4 }, () => '```text\nx\n```').join('\n')],
    ['aggregate code at limit', '```text\n' + 'x'.repeat(15999) + '\n```']
  ])('keeps %s eligible for deferred rich rendering', (_name, text) => {
    expect(getMarkdownFinalizationPolicy(text).mode).toBe('defer')
  })
  it.each(heavyCases)('keeps %s on the bounded plain surface', (_name, text) => {
    expect(getMarkdownFinalizationPolicy(text).mode).toBe('lightweight')
  })
})

describe('terminal plain-text fallback on actual DOM', () => {
  let root: Root
  let container: HTMLDivElement
  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    vi.unstubAllGlobals()
    await i18n.changeLanguage('en')
  })
  it.each(heavyCases)('announces %s fallback and preserves its full literal payload', async (_name, text) => {
    const finalized = vi.fn()
    await act(async () => root.render(createElement(AssistantMarkdown, {
      text, streaming: false, className: 'ds-markdown', onFinalized: finalized
    })))
    const note = container.querySelector('[role="note"]')
    expect(note?.textContent).toBe('Displayed as plain text to keep this long reply responsive.')
    expect(container.textContent).toContain(text)
    expect(container.querySelector('[data-streamdown="code-block"]')).toBeNull()
    expect(finalized).not.toHaveBeenCalled()
  })
  it('keeps the notice off streaming text and removes it when the public text is replaced', async () => {
    const text = heavyCases[0][1]
    await act(async () => root.render(createElement(AssistantMarkdown, { text, streaming: true })))
    expect(container.querySelector('[role="note"]')).toBeNull()
    await act(async () => root.unmount())
    root = createRoot(container)
    await i18n.changeLanguage('zh')
    await act(async () => root.render(createElement(AssistantMarkdown, { text, streaming: false })))
    expect(container.querySelector('[role="note"]')?.textContent).toBe('为保持响应，此长回复以纯文本显示。')
    await act(async () => root.render(createElement(AssistantMarkdown, { text: '', streaming: false })))
    expect(container.querySelector('[role="note"]')).toBeNull()
    expect(container.textContent).not.toContain(text)
  })
})
