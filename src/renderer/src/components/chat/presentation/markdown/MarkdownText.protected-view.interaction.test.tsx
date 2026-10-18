// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { MarkdownText, type MarkdownLabels } from './MarkdownText'
import { MarkdownDelegateProvider } from './MarkdownDelegate'
import { clearFileReferenceValidationCache } from '../../../../lib/file-reference-validation'
import { WORKSPACE_FILE_PREVIEW_EVENT } from '../../../../lib/workspace-file-preview'
const labels: MarkdownLabels = { code: { copyLabel: 'Copy', copiedLabel: 'Copied' }, footnotes: 'Footnotes',
  table: { title: 'Table', copy: 'Copy', copied: 'Copied', copyFailed: 'Failed', download: 'Export', downloadFailed: 'Failed',
    fullscreen: 'Fullscreen', close: 'Close', csv: 'CSV', tsv: 'TSV', markdown: 'Markdown', exportHint: 'Export table' } }

function workspaceHostSpies() {
  const resolve = vi.fn(async ({ path }: { path: string }) => ({ ok: true, path }))
  const readImage = vi.fn(async () => ({ ok: true, dataUrl: 'data:image/png;base64,iVBORw0KGgo=' }))
  const openEditorPath = vi.fn(async () => ({ ok: true })), preview = vi.fn()
  vi.stubGlobal('analytix', { files: { resolve, readImage }, workspace: { openEditorPath } })
  clearFileReferenceValidationCache()
  window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, preview)
  return {
    resolve, readImage, openEditorPath, preview,
    clear: () => { resolve.mockClear(); readImage.mockClear(); openEditorPath.mockClear(); preview.mockClear(); clearFileReferenceValidationCache() },
    dispose: () => { window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, preview); clearFileReferenceValidationCache() }
  }
}

it.each([false, true])('renders protected GFM view-only with no action/network/portal or stale frozen capability, streaming=%s', async streaming => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies(), open = vi.fn(), fetch = vi.fn()
  vi.stubGlobal('fetch', fetch)
  const text = '**PRIVATE_BOLD**\n\n[External](https://private.invalid) [File](/fixture/private.ts#L2) [ref][link]\n\n`https://private.invalid` `/fixture/private.ts`\n\n![ALT_ONLY](https://private.invalid/image.png) ![REF_ALT][image] ![LOCAL_ALT](/fixture/private.png) ![SUFFIX_ALT](/fixture/private%3Fname.png?rev=1#preview) ![SUFFIX_REF][local-image]\n\n```ts\nconst privateValue = 1\n```\n\n| Private | Data |\n| --- | --- |\n| cell | value |\n\nTail\n\n[link]: https://private.invalid/ref\n[image]: https://private.invalid/ref.png\n[local-image]: /fixture/private-ref.png#preview\n'
  const render = async (viewOnly: boolean, value = text) => act(async () => root.render(createElement(MarkdownDelegateProvider,
    { openExternalLink: open, children: createElement(MarkdownText, { text: value, streaming, labels, viewOnly }) })))
  try {
    await render(true)
    expect(host.querySelector('a,img,button,[role="dialog"]')).toBeNull()
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    await render(false)
    expect(host.querySelector('a')).not.toBeNull()
    if (!streaming) { expect(workspace.resolve).toHaveBeenCalled(); expect(workspace.readImage).toHaveBeenCalled() }
    workspace.clear()
    await render(true)
    expect(host.querySelector('strong')?.textContent).toBe('PRIVATE_BOLD')
    expect(host.querySelector('table td')?.textContent).toBe('cell')
    expect(host.querySelector('code')?.textContent).toContain('https://private.invalid')
    expect(host.textContent).toContain('ALT_ONLY'); expect(host.textContent).toContain('REF_ALT'); expect(host.textContent).toContain('LOCAL_ALT')
    expect(host.textContent).toContain('SUFFIX_ALT'); expect(host.textContent).toContain('SUFFIX_REF')
    expect(host.querySelector('a,img,button,[role="dialog"]')).toBeNull()
    await render(true, text + '\n\nAnother paragraph\n')
    expect(host.querySelector('a,img,button,[role="dialog"]')).toBeNull()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(open).not.toHaveBeenCalled(); expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    expect(workspace.openEditorPath).not.toHaveBeenCalled(); expect(workspace.preview).not.toHaveBeenCalled(); expect(fetch).not.toHaveBeenCalled()
    await act(async () => root.render(null))
    expect(document.body.textContent).not.toContain('PRIVATE_BOLD'); expect(document.querySelector('[role="dialog"]')).toBeNull()
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it.each([false, true])('keeps long protected fences fully readable without actions, streaming=%s', async streaming => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const fence = String.fromCharCode(96).repeat(3), body = Array.from({ length: 30 }, (_, index) => 'PRIVATE_LINE_' + index).join('\n')
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: fence + 'text\n' + body + '\n' + fence, labels, viewOnly: true, streaming })))
    expect(host.querySelector('[data-code-collapsed]')).toBeNull()
    expect(host.querySelector('[data-code-block-content]')?.textContent).toContain('PRIVATE_LINE_29')
    expect(host.querySelector('button,a,img,[role="dialog"]')).toBeNull()
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it.each(['protected', 'question'] as const)('retains styled keyboard-scrollable tables without export actions in %s content', async mode => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const text = '| A | B | C | D |\n| - | - | - | - |\n| one | two | three | four |\n\n| A | B |\n| - | - |\n| one | two |'
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels, viewOnly: mode === 'protected',
      questionDetail: mode === 'question' ? { copyCode: vi.fn(async () => true), openLink: vi.fn(async () => true) } : undefined })))
    const [wide, narrow] = [...host.querySelectorAll('table')].map(table => table.parentElement!)
    expect(wide.className).toContain('tableScroll'); expect(wide.className).toContain('md-table-wide')
    expect(narrow.className).toContain('tableScroll'); expect(narrow.className).toContain('tableFill')
    expect(wide.tabIndex).toBe(0); expect(narrow.tabIndex).toBe(0)
    wide.focus(); expect(document.activeElement).toBe(wide)
    expect(host.querySelector('button,a,img,[role="dialog"]')).toBeNull()
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it('keeps ordinary local links and images on the workspace Host and preserves owner external activation', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies(), open = vi.fn()
  const text = '[File](/fixture/file.ts#L2) [External](https://example.test/docs) ![Local image](/fixture/picture.png)'
  try {
    await act(async () => root.render(createElement(MarkdownDelegateProvider,
      { openExternalLink: open, children: createElement(MarkdownText, { text, labels }) })))
    expect(workspace.resolve).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/file.ts', line: 2 }))
    expect(workspace.readImage).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/picture.png' }))
    expect(host.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,iVBORw0KGgo=')
    const file = host.querySelector<HTMLButtonElement>('button[title="/fixture/file.ts"]')!
    expect(file).not.toBeNull()
    await act(async () => file.click())
    expect(workspace.preview).toHaveBeenCalledOnce()
    expect((workspace.preview.mock.calls[0][0] as CustomEvent).detail).toEqual(expect.objectContaining({ path: '/fixture/file.ts', line: 2 }))
    await act(async () => file.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })))
    expect(workspace.openEditorPath).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/file.ts', line: 2 }))
    await act(async () => host.querySelector<HTMLAnchorElement>('a[href="https://example.test/docs"]')!
      .dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true })))
    expect(open).toHaveBeenCalledOnce(); expect(open).toHaveBeenCalledWith('https://example.test/docs')
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it.each([
  { destination: '/fixture/query.png?rev=1', path: '/fixture/query.png' },
  { destination: '/fixture/fragment.png#preview', path: '/fixture/fragment.png' },
  { destination: '/fixture/both.png?rev=1#preview', path: '/fixture/both.png' },
  { destination: '/fixture/a%3Fb%23c.png?rev=1#preview', path: '/fixture/a?b#c.png' },
  { destination: '/fixture/encoded%20image.png#preview', path: '/fixture/encoded image.png' },
  { destination: '%2Ffixture%2Fencoded-root.png?rev=1', path: '/fixture/encoded-root.png' },
  { destination: '/fixture/a%253Fb.png?rev=1', path: '/fixture/a%3Fb.png' },
  { destination: 'C:/fixture/windows.png?rev=1', path: 'C:/fixture/windows.png' },
  { destination: '/fixture/existing.png#L2', path: '/fixture/existing.png', line: 2 },
])('resolves a local Markdown image suffix without re-decoding: $destination', async ({ destination, path, line }) => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: `![Local](${destination})`, labels })))
    expect(workspace.resolve).toHaveBeenCalledOnce()
    expect(workspace.resolve).toHaveBeenCalledWith(expect.objectContaining({ path, ...(line === undefined ? {} : { line }) }))
    expect(workspace.readImage).toHaveBeenCalledOnce()
    expect(workspace.readImage).toHaveBeenCalledWith(expect.objectContaining({ path }))
    expect(host.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,iVBORw0KGgo=')
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it.each([
  '/fixture/bad%ZZ.png?rev=1', '/fixture/a%00.png?rev=1', '//server/fixture/a.png?rev=1',
  '%2F%2Fserver%2Ffixture%2Fa.png?rev=1', 'file:///fixture/a.png?rev=1', 'relative.png?rev=1',
])('does not widen local Markdown image suffix parsing: %s', async destination => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: `![Fallback](${destination})`, labels })))
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    expect(host.querySelector('img')).toBeNull(); expect(host.textContent).toContain('Fallback')
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it.each(['http', 'https'])('preserves an external Markdown image suffix URL for %s', async protocol => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies(), destination = `${protocol}://example.test/image.png?rev=1#preview`
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: `![Remote](${destination})`, labels })))
    expect(host.querySelector('img')?.getAttribute('src')).toBe(destination)
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it('waits until settlement to resolve inline and reference local Markdown image suffixes', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  const text = '![INLINE](/fixture/inline.png?rev=1#preview)\n\n![REFERENCE][picture]\n\nTail\n\nMore\n\n[picture]: /fixture/reference.png#preview\n'
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels, streaming: true })))
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    expect(host.querySelector('img')).toBeNull(); expect(host.textContent).toContain('INLINE')
    await act(async () => root.render(createElement(MarkdownText, { text, labels, streaming: false })))
    expect(workspace.resolve).toHaveBeenCalledTimes(2); expect(workspace.readImage).toHaveBeenCalledTimes(2)
    expect(workspace.readImage).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/inline.png' }))
    expect(workspace.readImage).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/reference.png' }))
    expect(host.querySelectorAll('img')).toHaveLength(2)
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it('waits for Host validation and keeps the local Markdown image suffix fallback after denial', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  let finish!: (result: { ok: boolean; path: string }) => void
  workspace.resolve.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: '![Denied](/fixture/denied.png?rev=1)', labels })))
    expect(workspace.resolve).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/denied.png' }))
    expect(workspace.readImage).not.toHaveBeenCalled(); expect(host.querySelector('img')).toBeNull()
    await act(async () => finish({ ok: false, path: '/fixture/denied.png' }))
    expect(workspace.readImage).not.toHaveBeenCalled(); expect(host.querySelector('img')).toBeNull()
    expect(host.textContent).toContain('Denied')
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it('limits suffix compatibility to Markdown images while keeping ordinary file links unchanged', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  const text = '[Query](/fixture/link.png?rev=1) [Fragment](/fixture/link.png#preview) [Line](/fixture/file.ts#L2) ![Image](/fixture/image.png?rev=1)'
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels })))
    expect(workspace.resolve).toHaveBeenCalledTimes(2)
    expect(workspace.resolve).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/file.ts', line: 2 }))
    expect(workspace.readImage).toHaveBeenCalledOnce()
    expect(workspace.readImage).toHaveBeenCalledWith(expect.objectContaining({ path: '/fixture/image.png' }))
    expect(host.querySelectorAll('.ds-file-reference-link')).toHaveLength(1)
    expect(host.textContent).toContain('Query'); expect(host.textContent).toContain('Fragment')
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it('keeps Question code full and Copy/Wrap actionable without ordinary export extras', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  const fence = String.fromCharCode(96).repeat(3), body = Array.from({ length: 30 }, (_, i) => 'line ' + i).join('\n')
  const text = '[Open](https://example.test/docs) [Local](/fixture/file.md)\n\n' + fence + 'ts\n' + body + '\n' + fence
  let acknowledge!: (ok: boolean) => void
  const copyCode = vi.fn(() => new Promise<boolean>(resolve => { acknowledge = resolve })), openLink = vi.fn(async () => true)
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels: { ...labels, code: {
      ...labels.code, toolbarLabels: { codeLabel: 'Code', wrapLabel: 'Wrap', unwrapLabel: 'Unwrap' }
    } }, questionDetail: { copyCode, openLink } })))
    expect(host.querySelector('[data-code-collapsed]')).toBeNull()
    expect(host.querySelector('[data-code-block-content]')?.textContent).toContain('line 29')
    expect([...host.querySelectorAll('button')].map(button => button.getAttribute('aria-label'))).toEqual(['Wrap', 'Copy'])
    const block = host.querySelector<HTMLElement>('[data-code-wrap]')!
    expect(block.dataset.codeWrap).toBe('true')
    await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Wrap"]')!.click())
    expect(block.dataset.codeWrap).toBe('false')
    await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Copy"]')!.click())
    expect(copyCode).toHaveBeenCalledWith({ start: text.indexOf(fence), end: text.length })
    expect(host.querySelector('[aria-label="Copied"]')).toBeNull()
    await act(async () => acknowledge(true))
    expect(host.querySelector('[aria-label="Copied"]')).not.toBeNull()
    const anchor = host.querySelector<HTMLAnchorElement>('a')!
    const event = new MouseEvent('click', { bubbles: true, cancelable: true, metaKey: true })
    await act(async () => anchor.dispatchEvent(event))
    expect(event.defaultPrevented).toBe(true); expect(anchor.getAttribute('href')).toBe('https://example.test/docs'); expect(anchor.getAttribute('target')).toBe('_blank')
    expect(openLink).toHaveBeenCalledWith({ start: 0, end: text.indexOf(' [Local]') })
    expect(anchor.querySelector('svg')).not.toBeNull()
    expect(host.querySelectorAll('a')).toHaveLength(1)
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    expect(workspace.openEditorPath).not.toHaveBeenCalled(); expect(workspace.preview).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})

it('does not make an inline URL inside an authored Question link a second native action', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const tick = String.fromCharCode(96), text = '[' + tick + 'https://inner.test' + tick + '](https://outer.test)'
  const openLink = vi.fn(async () => true)
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels,
      questionDetail: { copyCode: vi.fn(async () => true), openLink } })))
    expect(host.querySelectorAll('a')).toHaveLength(1)
    await act(async () => host.querySelector('code')!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true })))
    expect(openLink).toHaveBeenCalledOnce(); expect(openLink).toHaveBeenCalledWith({ start: 0, end: text.length })
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it('preserves the authored outer destination for an ordinary inline URL inside a link', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const tick = String.fromCharCode(96), text = '[' + tick + 'https://inner.test' + tick + '](https://outer.test)'
  const open = vi.fn()
  try {
    await act(async () => root.render(createElement(MarkdownDelegateProvider,
      { openExternalLink: open, children: createElement(MarkdownText, { text, labels }) })))
    expect(host.querySelectorAll('a')).toHaveLength(1)
    expect(host.querySelector('a')?.getAttribute('href')).toBe('https://outer.test')
    await act(async () => host.querySelector('code')!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true })))
    expect(open).toHaveBeenCalledOnce(); expect(open).toHaveBeenCalledWith('https://outer.test')
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it('opens Question links on middle activation and keeps right activation inert', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const tick = String.fromCharCode(96), text = tick + 'https:example.test/docs' + tick
  const openLink = vi.fn(async () => true)
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels,
      questionDetail: { copyCode: vi.fn(async () => true), openLink } })))
    const anchor = host.querySelector<HTMLAnchorElement>('a')!
    expect(anchor).not.toBeNull()
    const right = new MouseEvent('auxclick', { button: 2, bubbles: true, cancelable: true })
    await act(async () => anchor.dispatchEvent(right))
    expect(openLink).not.toHaveBeenCalled(); expect(right.defaultPrevented).toBe(false)
    const middle = new MouseEvent('auxclick', { button: 1, bubbles: true, cancelable: true })
    await act(async () => anchor.dispatchEvent(middle))
    expect(middle.defaultPrevented).toBe(true)
    expect(openLink).toHaveBeenCalledOnce(); expect(openLink).toHaveBeenCalledWith({ start: 0, end: text.length })
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it('keeps viewOnly authoritative when a Question action context is also supplied', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const openLink = vi.fn(async () => true), copyCode = vi.fn(async () => true)
  try {
    await act(async () => root.render(createElement(MarkdownText, { text: '[Open](https://example.test/docs)', labels, viewOnly: true,
      questionDetail: { copyCode, openLink } })))
    expect(host.textContent).toBe('Open'); expect(host.querySelector('a,button,img')).toBeNull()
    expect(openLink).not.toHaveBeenCalled(); expect(copyCode).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
})

it('uses the normal Question image and pinned failed-image fallback, with no image-only anchor glyph or lightbox', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const workspace = workspaceHostSpies()
  const text = '[![Original alt](https://example.test/picture.png)](https://example.test/page) ![Local alt](/fixture/local.png)'
  const openLink = vi.fn(async () => true), copyCode = vi.fn(async () => true)
  try {
    await act(async () => root.render(createElement(MarkdownText, { text, labels, questionDetail: { copyCode, openLink } })))
    const anchor = host.querySelector<HTMLAnchorElement>('a')!, picture = host.querySelector('img')!
    expect(anchor.querySelector('svg')).toBeNull(); expect(anchor.getAttribute('href')).toBe('https://example.test/page')
    expect(picture.getAttribute('loading')).toBe('lazy'); expect(picture.getAttribute('referrerpolicy')).toBe('no-referrer')
    expect(host.querySelector('button')).toBeNull(); expect(host.textContent).toContain('Local alt')
    await act(async () => picture.dispatchEvent(new Event('error')))
    expect(host.querySelector('img')).toBeNull(); expect(anchor.textContent).toBe('Original alt')
    expect(host.querySelector('button,[role="dialog"]')).toBeNull(); expect(openLink).not.toHaveBeenCalled()
    await act(async () => root.render(createElement(MarkdownText, { text: '![New alt](https://example.test/new.png)', labels, questionDetail: { copyCode, openLink } })))
    expect(host.querySelector('img')?.getAttribute('src')).toBe('https://example.test/new.png')
    expect(workspace.resolve).not.toHaveBeenCalled(); expect(workspace.readImage).not.toHaveBeenCalled()
    expect(workspace.openEditorPath).not.toHaveBeenCalled(); expect(workspace.preview).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove(); workspace.dispose(); vi.unstubAllGlobals() }
})
