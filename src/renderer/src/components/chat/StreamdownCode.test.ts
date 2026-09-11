import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { shouldDeferCodeHighlight, StreamdownCode } from './StreamdownCode'
import {
  clearHighlightCodeCache,
  hasCachedHighlightCode,
  highlightCodeCacheSize,
  highlightCodeHtml,
  MAX_HIGHLIGHT_CACHE_ENTRIES
} from '../../lib/code-highlighting'

describe('StreamdownCode plain text fences', () => {
  it('renders text fenced blocks without code block chrome', () => {
    const html = renderToStaticMarkup(
      createElement(
        StreamdownCode,
        { className: 'language-text', 'data-block': true },
        'refactor(chat): simplify composer\n\n- Keep only Stop\n'
      )
    )

    expect(html).toContain('ds-plain-text-block')
    expect(html).toContain('ds-plain-code-block')
    expect(html).toContain('refactor(chat): simplify composer')
    expect(html).toContain('- Keep only Stop')
    expect(html).not.toContain('ds-code-block-header')
    expect(html).not.toContain('Download code')
    expect(html).not.toContain('Copy code')
  })

  it('hides empty plain text fenced blocks', () => {
    const html = renderToStaticMarkup(
      createElement(
        StreamdownCode,
        { className: 'language-text', 'data-block': true },
        '\n'
      )
    )

    expect(html).toBe('')
  })

  it('defers highlighting very large code blocks until user intent', () => {
    expect(shouldDeferCodeHighlight('console.log("small")')).toBe(false)
    expect(shouldDeferCodeHighlight('x'.repeat(12001))).toBe(true)
    expect(shouldDeferCodeHighlight(Array.from({ length: 241 }, () => 'x').join('\n'))).toBe(true)
  })

  it('reuses highlight output for repeated code and keeps a 200-entry LRU', async () => {
    clearHighlightCodeCache()

    await highlightCodeHtml('same code', 'text')
    await highlightCodeHtml('same code', 'text')

    expect(highlightCodeCacheSize()).toBe(1)
    expect(hasCachedHighlightCode('same code', 'text')).toBe(true)
    expect(MAX_HIGHLIGHT_CACHE_ENTRIES).toBe(200)
  })
})
