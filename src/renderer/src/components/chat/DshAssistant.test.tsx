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
  useChatStore.setState({ workspaceRoot: '/synthetic/workspace' })
  clearFileReferenceValidationCache()
  Object.assign(window, { analytix: { app: { openExternal: vi.fn().mockResolvedValue(true) }, files: { resolve: vi.fn().mockResolvedValue({ ok: false }) } } })
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial); vi.restoreAllMocks(); vi.unstubAllGlobals() })
const render = async (text: string, streaming = false) => act(async () => root.render(createElement(DshAssistant, { text, streaming })))
const textOf = (tokens: readonly (readonly { text: string }[])[] | undefined) => tokens?.map(line => line.map(span => span.text).join('')).join('\n')

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
})
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
