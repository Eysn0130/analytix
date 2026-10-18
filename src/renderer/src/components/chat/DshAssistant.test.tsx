// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { clearFileReferenceValidationCache } from '../../lib/file-reference-validation'
import { DshAssistant } from './DshAssistant'
import { CodeBlock } from './presentation/markdown/CodeBlock'
import { downloadCodeSource } from './presentation/CodeSourceActions'
import { StreamingHighlightSession, highlightLines, highlightToHtml } from './presentation/markdown/highlight'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
const initial = useChatStore.getState()
const labels = { codeLabel: 'Code', wrapLabel: 'Wrap code', unwrapLabel: 'Scroll code horizontally' }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  useChatStore.setState({ workspaceRoot: '/synthetic/workspace', topNotice: null })
  clearFileReferenceValidationCache()
  Object.assign(window, { analytix: { app: { openExternal: vi.fn().mockResolvedValue(true) }, files: { resolve: vi.fn().mockResolvedValue({ ok: false }) } } })
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial); vi.restoreAllMocks(); vi.unstubAllGlobals() })
const render = async (text: string, streaming = false) => act(async () => root.render(createElement(DshAssistant, { text, streaming })))
const textOf = (tokens: readonly (readonly { text: string }[])[] | undefined) => tokens?.map(line => line.map(span => span.text).join('')).join('\n')
const tokenColors = (element: Element) => [...element.querySelectorAll<HTMLElement>('.line > span')].map(token => [token.textContent, token.style.color])
const expectHighlightedSource = (code: string) => {
  const actual = host.querySelector('pre code')!
  expect(actual.textContent).toBe(code)
  const html = highlightToHtml(code, 'typescript')
  expect(html).toBeDefined()
  const expected = document.createElement('div')
  expected.innerHTML = html!
  const tokens = tokenColors(actual)
  expect(new Set(tokens.map(([, color]) => color).filter(Boolean)).size).toBeGreaterThan(1)
  expect(tokens).toEqual(tokenColors(expected))
}

it('renders actual GFM, CJK strong and bounded math while raw HTML and unsafe URLs stay inert', async () => {
  await render('中文**重点**说明\n\n| 项目 | 值 |\n| --- | --- |\n| 安全 | 1 |\n\n$x^2$\n\n<script>sentinel</script>\n\n[bad](javascript:alert%281%29)\n\n$\\href{javascript:alert(1)}{unsafe}$')
  expect(host.querySelector('strong')?.textContent).toBe('重点')
  expect(host.querySelector('table')?.textContent).toContain('安全')
  expect(host.querySelector('.katex')).not.toBeNull()
  expect(host.querySelector('script,a[href^="javascript:"]')).toBeNull()
  expect(host.textContent).toContain('<script>sentinel</script>')
})
it('keeps append, replacement and settle on one semantic markdown tree without duplicated text', async () => {
  await render('## Public\n\nHello', true)
  await render('## Public\n\nHello **world**.\n\n```text\nline one', true)
  expect(host.querySelectorAll('h2')).toHaveLength(1)
  expect(host.querySelector('pre code')?.textContent).toBe('line one')
  await render('## Replaced\n\nCurrent **value**.', true)
  await render('## Replaced\n\nCurrent **value**.')
  expect(host.textContent).toBe('Replaced\nCurrent value.')
  expect(host.textContent).not.toContain('Public')
})
it('retains one table action owner while append/replacement settles sanitized public cells', async () => {
  const first = '| Name | Value |\n| --- | --- |\n| **Public** | 1 |'
  await render(first, true)
  expect(host.querySelectorAll('[data-markdown-table]')).toHaveLength(1)
  expect(host.querySelector<HTMLButtonElement>('button[aria-label="Copy table"]')?.disabled).toBe(true)
  await render(first + '\n| Appended | 2 |', true)
  expect(host.querySelectorAll('[data-markdown-table]')).toHaveLength(1)
  const replaced = '| Label | Math |\n| --- | --- |\n| [bad](javascript:alert%281%29) <img> | $x^2$ |'
  await render(replaced, true); await render(replaced)
  expect(host.querySelectorAll('[data-markdown-table]')).toHaveLength(1)
  expect(host.querySelectorAll('table')).toHaveLength(1)
  expect(host.querySelector<HTMLButtonElement>('button[aria-label="Copy table"]')?.disabled).toBe(false)
  expect(host.querySelector('.katex')).not.toBeNull()
  expect(host.querySelector('a[href^="javascript:"],td img')).toBeNull()
  expect(host.querySelector('td')?.textContent).toContain('bad <img>')
  expect(host.textContent).not.toContain('Appended')
})
it.each(['', 'text', 'typescript'])('copies and downloads original %s source including Unicode, CRLF and real trailing blank lines', async lang => {
  const code = 'const label = "安全 🙂 <tag>";\r\n\r\n'
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  await act(async () => root.render(createElement(CodeBlock, { code: code + '\n', lang, copyLabel: 'Copy code', copiedLabel: 'Copied', toolbarLabels: labels })))
  await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="Copy code"]')!.click())
  expect(writeText).toHaveBeenCalledExactlyOnceWith(code)
  const create = vi.fn((_blob: Blob) => 'blob:synthetic-code'), revoke = vi.fn()
  Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
  downloadCodeSource(code, lang)
  const blob = create.mock.calls[0][0] as Blob
  const reader = new FileReader(); const content = new Promise(resolve => { reader.onload = () => resolve(reader.result) }); reader.readAsText(blob)
  expect(await content).toBe(code); expect(revoke).toHaveBeenCalledExactlyOnceWith('blob:synthetic-code')
})
it('links each independent collapse control to retained code and keeps keyboard focus during expand/collapse', async () => {
  const code = Array.from({ length: 40 }, (_, i) => `row ${i}`).join('\n')
  const fence = () => createElement(CodeBlock, { code: code + '\n', lang: 'text', copyLabel: 'Copy code', copiedLabel: 'Copied', toolbarLabels: labels })
  await act(async () => root.render(createElement('div', null, fence(), fence())))
  const buttons = [...host.querySelectorAll<HTMLButtonElement>('button[aria-label="Expand code"]')]
  expect(buttons).toHaveLength(2)
  const ids = buttons.map(b => b.getAttribute('aria-controls')!)
  expect(new Set(ids).size).toBe(2)
  for (const id of ids) expect(document.getElementById(id)?.querySelector('pre code')?.textContent).toBe(code)
  await act(async () => { buttons[0].focus(); buttons[0].click() })
  expect(buttons[0].getAttribute('aria-expanded')).toBe('true'); expect(document.activeElement).toBe(buttons[0])
  await act(async () => buttons[0].click())
  expect(buttons[0].getAttribute('aria-expanded')).toBe('false')
})
it.each([false, true])('renders ordinary assistant fences beyond twelve lines in full, streaming=%s', async streaming => {
  const code = Array.from({ length: 16 }, (_, i) => `const value${i} = ${i}`).join('\n')
  await render('```typescript\n' + code + '\n```', streaming)
  expect(host.querySelector('pre code')?.textContent).toBe(code)
  expect(host.querySelector('[data-code-collapsed]')).toBeNull()
  expect(host.querySelector('button[aria-label="Expand code"]')).toBeNull()
  expectHighlightedSource(code)
})
it.each([
  ['24,000 characters', Array.from({ length: 200 }, (_, i) => `const value${i} = "${'x'.repeat(120)}"`).join('\n')],
  ['700 lines', Array.from({ length: 701 }, (_, i) => `const v${i} = 1`).join('\n')],
])('highlights expanded source beyond %s with actual Shiki token colors', async (limit, code) => {
  expect(code.length > 24000).toBe(limit === '24,000 characters')
  expect(code.split('\n').length > 700).toBe(limit === '700 lines')
  await act(async () => root.render(createElement(CodeBlock, { code: code + '\n', lang: 'typescript', copyLabel: 'Copy code', copiedLabel: 'Copied', toolbarLabels: labels })))
  const expand = host.querySelector<HTMLButtonElement>('button[aria-label="Expand code"]')!
  expect(expand.getAttribute('aria-expanded')).toBe('false')
  await act(async () => expand.click())
  expect(expand.getAttribute('aria-expanded')).toBe('true')
  expectHighlightedSource(code)
})
it('retains completed assistant code lines and tokens across streaming groups and settle', async () => {
  const prefix = Array.from({ length: 40 }, (_, i) => `const value${i} = ${i}`).join('\n') + '\n'
  await render('```typescript\n' + prefix + 'const tail = "', true)
  const codeElement = host.querySelector('pre code')!
  const completed = [...codeElement.querySelectorAll('.line')].slice(0, 40)
  expect(completed).toHaveLength(40)
  const completedTokens = completed.map(line => [...line.children])
  const code = prefix + 'const tail = "done";\n' + Array.from({ length: 40 }, (_, i) => `const next${i} = ${i}`).join('\n')
  await render('```typescript\n' + code, true)
  expect(host.querySelector('pre code')).toBe(codeElement)
  const appended = [...codeElement.querySelectorAll('.line')]
  completed.forEach((line, index) => {
    expect(appended[index]).toBe(line)
    completedTokens[index].forEach((token, tokenIndex) => expect(appended[index].children[tokenIndex]).toBe(token))
  })
  expectHighlightedSource(code)
  const appendedTokens = appended.map(line => [...line.children])
  await render('```typescript\n' + code + '\n```')
  expect(host.querySelector('pre code')).toBe(codeElement)
  const settled = [...codeElement.querySelectorAll('.line')]
  expect(settled).toHaveLength(appended.length)
  appended.forEach((line, index) => {
    expect(settled[index]).toBe(line)
    appendedTokens[index].forEach((token, tokenIndex) => expect(settled[index].children[tokenIndex]).toBe(token))
  })
  expectHighlightedSource(code)
})
it('keeps unknown assistant fence languages as literal plain text while streaming and settled', async () => {
  const code = 'const value = "<span>literal</span>";\n'.repeat(16).trimEnd()
  expect(highlightToHtml(code, 'unknown-language')).toBeUndefined()
  for (const streaming of [true, false]) {
    await render('```unknown-language\n' + code + '\n```', streaming)
    const source = host.querySelector('pre code')!
    expect(source.textContent).toBe(code)
    expect(source.children).toHaveLength(0)
    expect(host.querySelector('[data-code-collapsed]')).toBeNull()
  }
})
it('retains checked body and inline file links with line/column, and withholds unvalidated replacement targets', async () => {
  const resolve = vi.fn().mockResolvedValue({ ok: true, path: '/synthetic/workspace/one.ts' })
  Object.assign(window.analytix.files, { resolve })
  await render('File src/one.ts:8:3 and `src/one.ts:8:3`.')
  expect(resolve).toHaveBeenCalledWith(expect.objectContaining({ path: 'src/one.ts', line: 8, column: 3 }))
  expect(host.querySelectorAll('.ds-file-reference-link')).toHaveLength(2)
  resolve.mockImplementation(() => new Promise(() => undefined))
  await render('File src/two.ts:2:1 and `src/two.ts:2:1`.')
  expect(host.querySelectorAll('.ds-file-reference-link')).toHaveLength(0)
})
it('uses the real external navigation bridge for HTTP and mailto', async () => {
  await render('[site](https://example.com) [email](mailto:synthetic@example.invalid)')
  await act(async () => host.querySelectorAll<HTMLAnchorElement>('a').forEach(a => a.click()))
  expect(window.analytix.app.openExternal).toHaveBeenCalledWith('https://example.com')
  expect(window.analytix.app.openExternal).toHaveBeenCalledWith('mailto:synthetic@example.invalid')
  expect(useChatStore.getState().topNotice).toBeNull()
})
it.each(['bridge absent', 'app absent', 'method absent', 'promise rejected', 'synchronous throw'] as const)(
  'reports a safe external-link error when %s', async failure => {
    const openExternal = vi.fn<(href: string) => Promise<void>>().mockResolvedValue(undefined)
    if (failure === 'promise rejected') openExternal.mockRejectedValue(new Error('RAW_EXTERNAL_OPEN_ERROR'))
    if (failure === 'synchronous throw') openExternal.mockImplementation(() => { throw new Error('RAW_EXTERNAL_OPEN_ERROR') })
    const bridge = failure === 'bridge absent' ? undefined : failure === 'app absent' ? {}
      : { app: failure === 'method absent' ? {} : { openExternal } }
    Object.assign(window, { analytix: bridge })
    await render('[site](https://example.invalid/external-target)')
    const event = new MouseEvent('click', { button: 0, bubbles: true, cancelable: true })
    await act(async () => { host.querySelector<HTMLAnchorElement>('a')!.dispatchEvent(event) })
    expect(event.defaultPrevented).toBe(true)
    expect(useChatStore.getState().topNotice).toMatchObject({ tone: 'error', message: 'Cannot open this link.' })
    expect(openExternal).toHaveBeenCalledTimes(failure === 'promise rejected' || failure === 'synchronous throw' ? 1 : 0)
  }
)
it('uses actual Shiki 3.23 for multiline streaming and lazy Python without changing source', async () => {
  const session = new StreamingHighlightSession()
  session.update('/* comment\n', 'typescript')
  const code = '/* comment\ncontinued */\nconst answer = 42'
  expect(textOf(session.update(code, 'typescript'))).toBe(code)
  const coloredCharacters = (lines: readonly (readonly { text: string; style: { color?: string } }[])[] | undefined) => lines?.map(line => line.flatMap(span => [...span.text].filter(char => !/\s/.test(char)).map(char => [char, span.style.color])))
  expect(coloredCharacters(session.update(code, 'typescript'))).toEqual(coloredCharacters(highlightLines(code, 'typescript')))
  highlightToHtml('print("safe")', 'python')
  await vi.dynamicImportSettled()
  expect(highlightToHtml('print("safe")', 'python')).toContain('print')
})
