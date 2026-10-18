import { describe, expect, it } from 'vitest'
import { extractMarkdownPlainText } from './plain-text'

describe('complete DSH plan summary projection', () => {
  it('keeps a distinct heading and first semantic paragraph, preserving labels, alt and literal HTML', () => {
    const markdown = '# **Original title**\n\n```text\nnot the paragraph\n```\n\n> First [reference](https://example.invalid) ![diagram](https://example.invalid/img) <span>literal</span>\n\n## Later\n\nUnabridged tail'
    expect(extractMarkdownPlainText(markdown, { mode: 'first-line' })).toBe('Original title')
    expect(extractMarkdownPlainText(markdown, { mode: 'first-paragraph' })).toBe('First reference diagram <span>literal</span>')
    expect(extractMarkdownPlainText(markdown)).toContain('Unabridged tail')
  })
  it('falls back to the visible code or heading when there is no semantic paragraph', () => {
    expect(extractMarkdownPlainText('# Only heading', { mode: 'first-paragraph' })).toBe('Only heading')
    expect(extractMarkdownPlainText('```text\nOnly code\n```', { mode: 'first-line' })).toBe('Only code')
    expect(extractMarkdownPlainText('')).toBe('')
  })
})
