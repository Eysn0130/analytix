// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { writeClipboard } from './clipboard'

const descriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
afterEach(() => {
  vi.unstubAllGlobals()
  if (descriptor) Object.defineProperty(navigator, 'clipboard', descriptor)
  else Reflect.deleteProperty(navigator, 'clipboard')
  Reflect.deleteProperty(document, 'execCommand')
})

function clipboard(value: unknown): void { Object.defineProperty(navigator, 'clipboard', { configurable: true, value }) }
function read(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = reject; reader.readAsText(blob) })
}

it('writes both explicit safe HTML and plain text when rich clipboard is available', async () => {
  class Item { constructor(public data: Record<string, Blob>) {} }
  vi.stubGlobal('ClipboardItem', Item)
  const write = vi.fn(async (_items: Item[]) => undefined), writeText = vi.fn()
  clipboard({ write, writeText })
  expect(await writeClipboard('plain', '<table><tbody></tbody></table>')).toBe(true)
  const item = write.mock.calls[0][0][0]
  expect(await read(item.data['text/plain'])).toBe('plain')
  expect(await read(item.data['text/html'])).toBe('<table><tbody></tbody></table>')
  expect(writeText).not.toHaveBeenCalled()
})

it('keeps ordinary code copy on the existing text route and falls back only for missing rich capability', async () => {
  vi.stubGlobal('ClipboardItem', class {})
  const write = vi.fn(), writeText = vi.fn(async () => undefined)
  clipboard({ write, writeText })
  expect(await writeClipboard('code\nraw')).toBe(true)
  expect(writeText).toHaveBeenCalledWith('code\nraw')
  expect(write).not.toHaveBeenCalled()
  vi.stubGlobal('ClipboardItem', undefined)
  expect(await writeClipboard('fallback', '<table/>')).toBe(true)
  expect(writeText).toHaveBeenLastCalledWith('fallback')
})

it('reports a denied rich write without attempting another permission route', async () => {
  vi.stubGlobal('ClipboardItem', class {})
  const writeText = vi.fn(), exec = vi.fn()
  clipboard({ write: vi.fn(async () => { throw new Error('denied') }), writeText })
  Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })
  expect(await writeClipboard('text', '<table/>')).toBe(false)
  expect(writeText).not.toHaveBeenCalled(); expect(exec).not.toHaveBeenCalled()
})

it('reports legacy copy success/failure and removes its temporary textarea', async () => {
  clipboard(undefined)
  const exec = vi.fn(() => true)
  Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })
  expect(await writeClipboard('legacy')).toBe(true)
  expect(document.querySelector('textarea')).toBeNull()
  exec.mockReturnValue(false)
  expect(await writeClipboard('legacy')).toBe(false)
})
