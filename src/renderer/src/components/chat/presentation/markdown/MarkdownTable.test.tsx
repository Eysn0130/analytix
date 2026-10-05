// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { MarkdownTable, type MarkdownTableLabels } from './MarkdownTable'
import type { MarkdownTableData } from './table-data'

const labels: MarkdownTableLabels = { title: 'Table', copy: 'Copy table', copied: 'Copied', copyFailed: 'Copy failed', download: 'Download table', downloadFailed: 'Download failed', fullscreen: 'Fullscreen', close: 'Close table', csv: 'CSV', tsv: 'TSV', markdown: 'Markdown', exportHint: 'Export as text' }
const data: MarkdownTableData = { headers: ['中英', 'Value'], rows: [['汉字, "quote"', '=1']], align: [null, 'right'] }
let host: HTMLDivElement, root: Root, frames: Map<number, FrameRequestCallback>, frameId: number
const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
const createUrlDescriptor = Object.getOwnPropertyDescriptor(URL, 'createObjectURL')
const revokeUrlDescriptor = Object.getOwnPropertyDescriptor(URL, 'revokeObjectURL')
let writeText: ReturnType<typeof vi.fn>, overflow: string
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  frames = new Map(); frameId = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ClipboardItem', undefined)
  vi.stubGlobal('ResizeObserver', class { observe(): void {} unobserve(): void {} disconnect(): void {} })
  writeText = vi.fn(async () => undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  overflow = document.body.style.overflow
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove(); document.body.style.overflow = overflow
  vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
  else Reflect.deleteProperty(navigator, 'clipboard')
  if (createUrlDescriptor) Object.defineProperty(URL, 'createObjectURL', createUrlDescriptor)
  else Reflect.deleteProperty(URL, 'createObjectURL')
  if (revokeUrlDescriptor) Object.defineProperty(URL, 'revokeObjectURL', revokeUrlDescriptor)
  else Reflect.deleteProperty(URL, 'revokeObjectURL')
})
async function flush(): Promise<void> { await act(async () => { for (const [id, callback] of [...frames]) { frames.delete(id); callback(0) } }) }
function button(label: string, scope: ParentNode = document): HTMLButtonElement {
  const found = [...scope.querySelectorAll<HTMLButtonElement>('button')].find(item => item.getAttribute('aria-label') === label || item.textContent === label)
  if (!found) throw new Error('Missing button: ' + label)
  return found
}
async function click(target: HTMLElement): Promise<void> { await act(async () => target.click()); await flush() }
async function key(target: HTMLElement, value: string, extra: KeyboardEventInit = {}): Promise<KeyboardEvent> {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...extra })
  await act(async () => target.dispatchEvent(event)); await flush(); return event
}
async function render(value = data, busy = false, two = false): Promise<void> {
  const element = (key: string) => createElement(MarkdownTable, { key, data: value, labels, wide: true, busy,
    table: createElement('table', null, createElement('tbody', null, createElement('tr', null, createElement('td', null, value.rows[0]?.[0])))) })
  await act(async () => root.render(createElement('div', null, element('first'), two ? element('second') : null)))
}
function read(blob: Blob): Promise<string> { return new Promise(resolve => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blob) }) }

it('copies each chosen format from public data, reports failure and keeps two tables independent', async () => {
  await render(data, false, true)
  const tables = host.querySelectorAll('[data-markdown-table]')
  await click(button('Copy table', tables[0])); await click(button('CSV'))
  expect(writeText).toHaveBeenLastCalledWith('中英,Value\r\n"汉字, ""quote""","\'=1"')
  expect(button('Copied', tables[0])).toBeDefined(); expect(button('Copy table', tables[1])).toBeDefined()
  await click(button('Copied', tables[0])); await click(button('TSV'))
  expect(writeText).toHaveBeenLastCalledWith('中英\tValue\n汉字, "quote"\t\'=1')
  await click(button('Copied', tables[0])); await click(button('Markdown'))
  expect(String(writeText.mock.calls.at(-1)?.[0])).toContain('| --- | ---: |')
  writeText.mockRejectedValueOnce(new Error('denied'))
  await click(button('Copy table', tables[1])); await click(button('CSV'))
  expect(tables[1].querySelector('[role="status"]')?.textContent).toBe('Copy failed')
})

it('downloads CSV with UTF-8 BOM and Markdown with original formula text, revoking every object URL', async () => {
  const blobs: Blob[] = [], names: string[] = []
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn((blob: Blob) => { blobs.push(blob); return 'blob:table' }) })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { names.push(this.download) })
  await render()
  await click(button('Download table')); await click(button('CSV'))
  expect(names).toEqual(['table.csv']); expect(blobs[0].type).toBe('text/csv;charset=utf-8')
  // FileReader strips a Unicode BOM when decoding; inspect the first bytes too.
  const bytes = await new Promise<ArrayBuffer>(resolve => { const reader = new FileReader(); reader.onload = () => resolve(reader.result as ArrayBuffer); reader.readAsArrayBuffer(blobs[0]) })
  expect([...new Uint8Array(bytes).slice(0, 3)]).toEqual([239, 187, 191])
  await click(button('Download table')); await click(button('Markdown'))
  expect(names).toEqual(['table.csv', 'table.md']); expect(await read(blobs[1])).toContain('| 汉字, "quote" | =1 |')
  expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2); expect(document.querySelector('a[download]')).toBeNull()
})

it('retains the original scroll node through fullscreen and closes menu before dialog with IME-safe focus', async () => {
  document.body.style.overflow = 'clip'
  await render()
  const original = host.querySelector<HTMLDivElement>('[tabindex="0"]')!
  original.scrollLeft = 137
  const opener = button('Fullscreen'); await act(async () => opener.focus()); await click(opener)
  const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
  expect(dialog.contains(document.activeElement)).toBe(true); expect(document.body.style.overflow).toBe('hidden')
  const copy = button('Copy table', dialog)
  await key(copy, 'ArrowUp'); expect(document.activeElement).toBe(button('Markdown', dialog))
  await key(document.activeElement as HTMLElement, 'Escape', { isComposing: true })
  expect(dialog.querySelector('[role="menu"]')).not.toBeNull()
  await key(document.activeElement as HTMLElement, 'Escape')
  expect(dialog.querySelector('[role="menu"]')).toBeNull(); expect(document.activeElement).toBe(copy)
  await key(copy, 'ArrowDown'); await key(document.activeElement as HTMLElement, 'Tab', { shiftKey: true })
  expect(dialog.contains(document.activeElement)).toBe(true)
  await key(document.activeElement as HTMLElement, 'Escape')
  expect(document.querySelector('[role="dialog"]')).toBeNull(); expect(document.activeElement).toBe(opener)
  expect(host.querySelector('[tabindex="0"]')).toBe(original); expect(original.scrollLeft).toBe(137)
  expect(document.body.style.overflow).toBe('clip')
})

it('invalidates pending feedback on source replacement and disables every action while streaming', async () => {
  let resolve!: () => void
  writeText.mockImplementationOnce(() => new Promise<void>(done => { resolve = done }))
  await render(); await click(button('Copy table')); await click(button('CSV'))
  expect(button('Copy table').getAttribute('aria-busy')).toBe('true')
  await render({ ...data, rows: [['new', '42']] })
  await act(async () => resolve())
  expect(host.querySelector('[role="status"]')?.textContent).toBe('')
  await click(button('Copy table')); await click(button('CSV'))
  expect(writeText).toHaveBeenLastCalledWith('中英,Value\r\nnew,42')
  await render(data, true)
  expect([...host.querySelectorAll<HTMLButtonElement>('button')].every(item => item.disabled)).toBe(true)
  expect(document.querySelector('[role="menu"]')).toBeNull()
})

it('exits a fullscreen format menu through actual visible links and skips hidden targets', async () => {
  await act(async () => root.render(createElement(MarkdownTable, { data, labels, busy: false, wide: true,
    table: createElement('table', null, createElement('tbody', null, createElement('tr', null,
      createElement('td', null, createElement('a', { href: 'https://example.test/visible' }, 'Visible'),
        createElement('a', { href: 'https://example.test/hidden', hidden: true }, 'Hidden'))))) })))
  await click(button('Fullscreen'))
  const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
  const copy = button('Copy table', dialog)
  await key(copy, 'ArrowDown'); await key(document.activeElement as HTMLElement, 'Tab', { shiftKey: true })
  expect(document.activeElement).toBe(dialog.querySelector('a[href$="/visible"]'))
  expect(dialog.querySelector('[role="menu"]')).toBeNull()
})
