import { describe, expect, it } from 'vitest'
import { applyWriteBlockTypeToLines, detectWriteBlockTypeFromLine } from './block-type'

describe('detectWriteBlockTypeFromLine', () => {
  it('detects headings by level', () => {
    expect(detectWriteBlockTypeFromLine('# Title')).toBe('heading1')
    expect(detectWriteBlockTypeFromLine('## Title')).toBe('heading2')
    expect(detectWriteBlockTypeFromLine('### Title')).toBe('heading3')
    expect(detectWriteBlockTypeFromLine('#### Deep')).toBe('heading3')
  })

  it('detects quotes, lists, and code fences', () => {
    expect(detectWriteBlockTypeFromLine('> quote')).toBe('quote')
    expect(detectWriteBlockTypeFromLine('- item')).toBe('bullet')
    expect(detectWriteBlockTypeFromLine('* item')).toBe('bullet')
    expect(detectWriteBlockTypeFromLine('1. item')).toBe('ordered')
    expect(detectWriteBlockTypeFromLine('2) item')).toBe('ordered')
    expect(detectWriteBlockTypeFromLine('```ts')).toBe('code')
  })

  it('falls back to paragraph and ignores leading indentation', () => {
    expect(detectWriteBlockTypeFromLine('plain text')).toBe('paragraph')
    expect(detectWriteBlockTypeFromLine('  ## indented heading')).toBe('heading2')
    expect(detectWriteBlockTypeFromLine('#nospace')).toBe('paragraph')
  })
})

describe('applyWriteBlockTypeToLines', () => {
  it('adds a heading marker, stripping any existing marker', () => {
    expect(applyWriteBlockTypeToLines(['hello'], 'heading2')).toEqual(['## hello'])
    expect(applyWriteBlockTypeToLines(['# hello'], 'heading2')).toEqual(['## hello'])
    expect(applyWriteBlockTypeToLines(['- hello'], 'heading1')).toEqual(['# hello'])
  })

  it('numbers ordered lists sequentially over non-empty lines', () => {
    expect(applyWriteBlockTypeToLines(['a', 'b', 'c'], 'ordered')).toEqual(['1. a', '2. b', '3. c'])
  })

  it('converts to paragraph by removing markers', () => {
    expect(applyWriteBlockTypeToLines(['## hello', '> world'], 'paragraph')).toEqual(['hello', 'world'])
  })

  it('wraps content in a fenced block for code', () => {
    expect(applyWriteBlockTypeToLines(['const a = 1'], 'code')).toEqual(['```', 'const a = 1', '```'])
  })

  it('preserves indentation and leaves blank lines unprefixed', () => {
    expect(applyWriteBlockTypeToLines(['  hello', '', 'world'], 'bullet')).toEqual([
      '  - hello',
      '',
      '- world'
    ])
  })
})
