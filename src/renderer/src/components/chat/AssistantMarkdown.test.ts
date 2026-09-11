import { describe, expect, it } from 'vitest'
import {
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
