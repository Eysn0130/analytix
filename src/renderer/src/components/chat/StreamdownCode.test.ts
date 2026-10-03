// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, createElement, useLayoutEffect } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import i18n from '../../i18n'
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
  it.each(['text', 'plaintext', ''])('renders %s fences as code with the existing copy chrome', (language) => {
    const html = renderToStaticMarkup(createElement(StreamdownCode,
      { className: language ? `language-${language}` : undefined, 'data-block': true },
      'refactor(chat): simplify composer\n\n- Keep only Stop\n'))

    expect(html).toContain('<pre')
    expect(html).toContain('<code>')
    expect(html).toContain('ds-code-block-header')
    expect(html).toContain('Copy code')
    expect(html).toContain('refactor(chat): simplify composer')
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

describe('code controls on actual DOM', () => {
  let container: HTMLDivElement
  let root: Root
  let clipboardBefore: PropertyDescriptor | undefined
  const writeText = vi.fn(async (_text: string) => {})

  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    await i18n.changeLanguage('en')
    writeText.mockClear()
    clipboardBefore = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    if (clipboardBefore) Object.defineProperty(navigator, 'clipboard', clipboardBefore)
    else Reflect.deleteProperty(navigator, 'clipboard')
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    await i18n.changeLanguage('en')
  })
  const codeElement = (language: string, code: string) => createElement(StreamdownCode,
    { className: language ? `language-${language}` : undefined, 'data-block': true }, code)
  const longCode = Array.from({ length: 40 }, (_, i) => `const row${i} = ${i};`).join('\n')
  function measuredCode() {
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(function (this: HTMLElement) {
      return this.classList.contains('ds-code-block-html') ? 400 : 0
    })
  }

  it.each(['text', '', 'typescript'])('copies the full %s payload including Unicode, CRLF and trailing blank lines', async language => {
    const code = 'const label = "安全 🙂 <tag>";\r\n\r\n'
    await act(async () => root.render(codeElement(language, code)))
    const button = container.querySelector<HTMLButtonElement>('button[aria-label="Copy code"]')
    expect(button).not.toBeNull()
    await act(async () => button!.click())
    expect(writeText).toHaveBeenCalledExactlyOnceWith(code)
    expect(container.querySelector('pre code')).not.toBeNull()
  })

  it('associates both disclosure controls with the same existing preview and updates state once', async () => {
    measuredCode()
    await act(async () => root.render(codeElement('typescript', longCode)))
    const header = container.querySelector<HTMLButtonElement>('.ds-code-block-actions button[aria-label="Expand code"]')!
    const fade = container.querySelector<HTMLButtonElement>('.ds-code-block-fade')!
    expect(header).not.toBeNull()
    expect(fade).not.toBeNull()
    expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(fade.getAttribute('aria-expanded')).toBe('false')
    const id = header.getAttribute('aria-controls')
    expect(id).toBeTruthy()
    expect(fade.getAttribute('aria-controls')).toBe(id)
    const preview = document.getElementById(id!)!
    expect(preview.querySelector('pre code')).not.toBeNull()
    expect(preview.hidden).toBe(false)
    await act(async () => { header.focus(); header.click() })
    expect(header.getAttribute('aria-expanded')).toBe('true')
    expect(header.getAttribute('aria-label')).toBe('Collapse code')
    expect(container.querySelector('.ds-code-block-fade')).toBeNull()
    expect(document.activeElement).toBe(header)
    await act(async () => header.click())
    expect(header.getAttribute('aria-expanded')).toBe('false')
    await act(async () => {
      const fade = container.querySelector<HTMLButtonElement>('.ds-code-block-fade')!
      fade.focus()
      fade.click()
    })
    expect(header.getAttribute('aria-expanded')).toBe('true')
    expect(document.activeElement).toBe(header)
  })

  it.each(['text', 'typescript'])('commits the current %s payload immediately when replaced or cleared', async language => {
    const commits: string[] = []
    function CurrentFence({ payload }: { payload: string }) {
      useLayoutEffect(() => {
        commits.push(container.querySelector('pre code')?.textContent ?? '')
      }, [payload])
      return codeElement(language, payload)
    }
    await act(async () => root.render(createElement(CurrentFence, { payload: 'public fence A' })))
    await act(async () => root.render(createElement(CurrentFence, { payload: 'public fence B' })))
    await act(async () => root.render(createElement(CurrentFence, { payload: '' })))
    // The existing typed fallback reserves an empty line with one space.
    expect(commits).toEqual(['public fence A', 'public fence B', language === 'text' ? '' : ' '])
  })

  it('uses unique control targets for two independent code blocks', async () => {
    measuredCode()
    await act(async () => root.render(createElement('div', null,
      codeElement('typescript', longCode), codeElement('python', longCode))))
    const buttons = [...container.querySelectorAll<HTMLButtonElement>('.ds-code-block-actions button[aria-label="Expand code"]')]
    expect(buttons).toHaveLength(2)
    const targets = buttons.map(button => button.getAttribute('aria-controls'))
    expect(targets.every(target => Boolean(target && document.getElementById(target)))).toBe(true)
    expect(new Set(targets).size).toBe(2)
  })

  it('localizes copy, download and both expansion controls in Chinese', async () => {
    measuredCode()
    await i18n.changeLanguage('zh')
    await act(async () => root.render(codeElement('typescript', longCode)))
    expect(container.querySelector('button[aria-label="复制代码"]')).not.toBeNull()
    expect(container.querySelector('button[aria-label="下载代码"]')).not.toBeNull()
    expect(container.querySelectorAll('button[aria-label="展开代码"]')).toHaveLength(2)
    const toggle = container.querySelector<HTMLButtonElement>('.ds-code-block-actions button[aria-label="展开代码"]')!
    await act(async () => toggle.click())
    expect(toggle.getAttribute('aria-label')).toBe('收起代码')
  })
})
