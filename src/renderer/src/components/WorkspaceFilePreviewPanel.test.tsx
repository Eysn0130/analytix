import { createHtmlEmbedHandle } from '../write/html-embed-dom'
// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { WorkspaceFileTarget, WorkspaceImageReadResult, WorkspacePdfReadResult, WorkspaceExcelReadResult, WorkspaceFileSaveAsResult } from '@shared/workspace-file'
import i18n from '../i18n'
import { WorkspaceFilePreviewPanel } from './WorkspaceFilePreviewPanel'
import type { WritePdfViewer } from './write/WritePdfViewer'
import type { ExcelBodyProps } from './file-preview/excel/LazyExcelBody'
import type { ToolResultLocalEffectResultV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
const pdfRender = vi.hoisted(() => vi.fn())
const excelRender = vi.hoisted(() => vi.fn())
vi.mock('./file-preview/excel/LazyExcelBody', () => ({ LazyExcelBody: (props: ExcelBodyProps) => {
  excelRender(props); return createElement('div', { 'data-mocked-excel': true })
} }))
vi.mock('./write/WritePdfViewer', () => ({ WritePdfViewer: (props: ComponentProps<typeof WritePdfViewer>) => { pdfRender(props); return createElement('div', { 'data-mocked-pdf': true }, props.dataBase64) } }))
vi.mock('../lib/code-highlighting', () => ({ highlightCodeHtml: async (text: string) => text, languageFromFilePath: () => 'text', renderFallbackCodeHtml: (text: string) => text }))
const ref = { threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
  snapshotDigest: 'c'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', index: 0 }
let root: Root, container: HTMLDivElement
let clipboardDescriptor: PropertyDescriptor | undefined
const nativeClipboard = vi.fn(async () => undefined)
const excelLimits = { maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000 }
const excelReceipt: Extract<WorkspaceExcelReadResult, { ok: true }> = { ok: true, path: '/synthetic/workbook.xlsx',
  dataBase64: 'YSxi', format: 'xlsx', size: 3, contentDigest: 'd'.repeat(64), limits: excelLimits }
const readExcel = vi.fn(async (): Promise<WorkspaceExcelReadResult> => excelReceipt)
const copy = vi.fn(async (_request: unknown): Promise<ToolResultLocalEffectResultV1> => ({ ok: false, code: 'unsupported' }))
const readImage = vi.fn(async (): Promise<WorkspaceImageReadResult> => ({ ok: true, path: '/synthetic/current.png',
  dataUrl: 'data:image/png;base64,AQAC', mimeType: 'image/png', size: 3 }))
const readPdf = vi.fn(async (): Promise<WorkspacePdfReadResult> => ({ ok: true, path: '/synthetic/current.pdf',
  dataBase64: 'JVBERg==', mimeType: 'application/pdf', size: 4, mtimeMs: 12 }))
const ordinaryOpen = vi.fn(), writeFile = vi.fn()
const saveFile = vi.fn(async (_request: unknown): Promise<WorkspaceFileSaveAsResult> => ({ ok: true, path: '/synthetic/saved.xlsx' }))
const savePresent = vi.fn(async (_request: unknown): Promise<ToolResultLocalEffectResultV1> => ({ ok: true }))
const open = vi.fn(async (_request: unknown): Promise<{ ok: boolean; code?: string }> => ({ ok: false, code: 'unsupported' }))
const read = vi.fn(async () => ({ ok: true, path: '/synthetic/current.md', content: 'Current file source', size: 19, truncated: false }))
beforeEach(async () => {
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); open.mockReset(); read.mockReset(); read.mockResolvedValue({ ok: true, path: '/synthetic/current.md', content: 'Current file source', size: 19, truncated: false }); readImage.mockReset(); readPdf.mockReset(); readExcel.mockReset(); readExcel.mockResolvedValue(excelReceipt); copy.mockReset(); copy.mockResolvedValue({ ok: false, code: 'unsupported' }); excelRender.mockClear(); nativeClipboard.mockClear(); pdfRender.mockClear(); ordinaryOpen.mockClear(); writeFile.mockClear(); saveFile.mockReset(); saveFile.mockResolvedValue({ ok: true, path: '/synthetic/saved.xlsx' }); savePresent.mockReset(); savePresent.mockResolvedValue({ ok: true })
  readImage.mockResolvedValue({ ok: true, path: '/synthetic/current.png', dataUrl: 'data:image/png;base64,AQAC', mimeType: 'image/png', size: 3 })
  readPdf.mockResolvedValue({ ok: true, path: '/synthetic/current.pdf', dataBase64: 'JVBERg==', mimeType: 'application/pdf', size: 4, mtimeMs: 12 })
  open.mockResolvedValue({ ok: false, code: 'unsupported' })
  clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: nativeClipboard } })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { openToolResultLocalPresentFile: open, copyToolResultLocalDisplay: copy, saveToolResultLocalDisplay: savePresent }, files: { read, readImage, readPdf, readExcel, write: writeFile, saveAs: saveFile }, app: { openExternal: ordinaryOpen, openEditorPath: ordinaryOpen } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges(); if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor); else Reflect.deleteProperty(navigator, 'clipboard'); vi.unstubAllGlobals() })
async function draw(target: WorkspaceFileTarget | null, tabbedWorkspace = false, workspaceRoot = '/synthetic') {
  await act(async () => root.render(createElement(WorkspaceFilePreviewPanel, { target, tabbedWorkspace, workspaceRoot, onClose: () => undefined })))
}
async function fileMenu(): Promise<HTMLElement> {
  if (!document.querySelector('[role="menu"]')) {
    await act(async () => container.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-haspopup="menu"]')!.click())
  }
  return document.querySelector<HTMLElement>('[role="menu"]')!
}
async function menuButton(label: string): Promise<HTMLButtonElement> {
  const menu = await fileMenu()
  return menu.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!
}
it('retries the current file read once and keeps close available while loading', async () => {
  let finish!: (value: Awaited<ReturnType<typeof read>>) => void
  read.mockRejectedValueOnce(new Error('Current read failed'))
    .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const target = { path: '/synthetic/current.md' }
  await draw(target)
  expect(container.querySelector('[role="alert"]')?.textContent).toBe('Current read failed')
  const retry = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === 'Retry')!
  await act(async () => { retry.click(); retry.click() })
  expect(read).toHaveBeenCalledTimes(2)
  expect(read).toHaveBeenLastCalledWith({ ...target, workspaceRoot: '/synthetic' })
  expect(container.textContent).not.toContain('Current read failed')
  const close = container.querySelector<HTMLButtonElement>('button[aria-label="Collapse right sidebar"]')
  expect(close).not.toBeNull()
  expect(close!.disabled).toBe(false)
  await act(async () => finish({ ok: true, path: target.path, content: 'Recovered file', size: 14, truncated: false }))
  expect(container.textContent).toContain('Recovered file')
})
it('retries a failed protected typed read with the same selector and no ordinary fallback', async () => {
  readExcel.mockResolvedValueOnce({ ok: false, message: 'Workbook unavailable' })
  const target: WorkspaceFileTarget = { path: 'current.xlsx', preview: 'source', protectedPresent: ref }
  await draw(target)
  const retry = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === 'Retry')!
  await act(async () => retry.click())
  expect(readExcel).toHaveBeenCalledTimes(2)
  expect(readExcel).toHaveBeenLastCalledWith({ ...target, workspaceRoot: '/synthetic' })
  expect(read).not.toHaveBeenCalled()
  expect(ordinaryOpen).not.toHaveBeenCalled()
  expect(container.querySelector('[data-mocked-excel]')).not.toBeNull()
})
it('discards a retry completion after selecting another file', async () => {
  let finish!: (value: Awaited<ReturnType<typeof read>>) => void
  read.mockRejectedValueOnce(new Error('Old read failed'))
    .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw({ path: '/synthetic/old.md' })
  const retry = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === 'Retry')!
  await act(async () => retry.click())
  await draw({ path: '/synthetic/current.md' })
  await act(async () => finish({ ok: true, path: '/synthetic/old.md', content: 'STALE_RETRY_CONTENT', size: 19, truncated: false }))
  expect(container.textContent).toContain('Current file source')
  expect(container.textContent).not.toContain('STALE_RETRY_CONTENT')
})
it('does not offer an ineffective retry for an unsupported ordinary file', async () => {
  await draw({ path: '/synthetic/unsupported.bin' })
  expect(read).not.toHaveBeenCalled()
  expect(container.querySelector('[role="alert"]')).not.toBeNull()
  expect(Array.from(container.querySelectorAll('button')).some(button => button.textContent === 'Retry')).toBe(false)
  expect(container.querySelector('button[aria-label="Collapse right sidebar"]')).not.toBeNull()
})
it('uses protected current source and reports unavailable native open visibly without ordinary editor fallback', async () => {
  const target: WorkspaceFileTarget = { path: 'current.md', preview: 'source', protectedPresent: ref }
  await draw(target)
  expect(read).toHaveBeenCalledWith({ ...target, workspaceRoot: '/synthetic' })
  expect(container.textContent).toContain('Current file source')
  const button = await menuButton('Open')
  expect(button).not.toBeNull(); await act(async () => button.click())
  expect(open).toHaveBeenCalledExactlyOnceWith(ref)
  const status = container.querySelector<HTMLElement>('[role="status"]')!
  expect(status?.textContent).toBe('Could not open this file'); expect(status?.className).not.toContain('sr-only')
  expect((await menuButton('Copy content')).disabled).toBe(true)
})
it('prevents repeated open and masks late completion after the preview owner changes', async () => {
  let finish!: (value: { ok: boolean }) => void
  open.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw({ path: 'first.md', preview: 'source', protectedPresent: ref })
  const button = await menuButton('Open')
  await act(async () => button.click())
  const pendingButton = await menuButton('Open')
  expect(pendingButton.disabled).toBe(true)
  await act(async () => pendingButton.click()); expect(open).toHaveBeenCalledOnce()
  await draw({ path: 'second.md', preview: 'source', protectedPresent: { ...ref, index: 1 } })
  await act(async () => finish({ ok: true }))
  expect(container.querySelector('[role="status"]')).toBeNull()
})


it.each(['image', 'pdf'] as const)('Present %s uses the passive viewer and keeps copy, drag and context export blocked', async kind => {
  const target: WorkspaceFileTarget = { path: kind === 'image' ? 'label.png' : 'label.pdf', preview: 'source', protectedPresent: ref }
  await draw(target)
  expect(kind === 'image' ? readImage : readPdf).toHaveBeenCalledWith({ ...target, workspaceRoot: '/synthetic' })
  expect(read).not.toHaveBeenCalled(); expect(kind === 'image' ? readPdf : readImage).not.toHaveBeenCalled()
  const body = container.querySelector<HTMLElement>(`[data-present-media-preview="${kind}"]`)!
  expect(body).not.toBeNull()
  if (kind === 'image') expect(body.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,AQAC')
  else {
    expect(pdfRender).toHaveBeenLastCalledWith(expect.objectContaining({ filePath: '/synthetic/current.pdf', dataBase64: 'JVBERg==', mtimeMs: 12, workspaceRoot: '' }))
    const selection = pdfRender.mock.calls.at(-1)![0].onSelectionChange
    selection({ text: 'PRIVATE_SELECTION' })
    await draw({ ...target, protectedPresent: { ...ref, index: 1 } })
    expect(pdfRender.mock.calls.at(-1)![0].onSelectionChange).toBe(selection)
  }
  const currentBody = container.querySelector<HTMLElement>(`[data-present-media-preview="${kind}"]`)!
  for (const type of ['copy', 'cut', 'dragstart', 'contextmenu']) {
    const event = new Event(type, { bubbles: true, cancelable: true }); currentBody.dispatchEvent(event); expect(event.defaultPrevented).toBe(true)
  }
  expect((await menuButton('Copy content')).disabled).toBe(true)
  expect(open).not.toHaveBeenCalled(); expect(ordinaryOpen).not.toHaveBeenCalled(); expect(writeFile).not.toHaveBeenCalled(); expect(saveFile).not.toHaveBeenCalled()
  await draw(null); expect(container.querySelector('[data-present-media-preview]')).toBeNull()
})
it('Present media does not change frozen Read routing even when its display label has a media suffix', async () => {
  const { index: _index, ...protectedRead } = ref
  await draw({ path: 'misleading.pdf', preview: 'source', protectedRead })
  expect(read).toHaveBeenCalled(); expect(readImage).not.toHaveBeenCalled(); expect(readPdf).not.toHaveBeenCalled()
  expect(container.textContent).toContain('Current file source')
})
it.each(['success', 'reject'] as const)('Present late image %s cannot replace the next index PDF body', async completion => {
  let finish!: (value: WorkspaceImageReadResult) => void, fail!: (error: Error) => void
  readImage.mockImplementationOnce(() => new Promise((resolve, reject) => { finish = resolve; fail = reject }))
  await draw({ path: 'same.png', preview: 'source', protectedPresent: ref })
  await draw({ path: 'same.pdf', preview: 'source', protectedPresent: { ...ref, index: 1 } })
  const pdf = container.querySelector('[data-present-media-preview="pdf"]')
  expect(pdf).not.toBeNull()
  await act(async () => {
    if (completion === 'reject') fail(new Error('STALE_IMAGE_ERROR'))
    else finish({ ok: true, path: '/synthetic/old.png', dataUrl: 'data:image/png;base64,OLD', mimeType: 'image/png', size: 3 })
  })
  expect(container.querySelector('[data-present-media-preview="pdf"]')).toBe(pdf)
  expect(container.querySelector('img')).toBeNull(); expect(container.textContent).not.toContain('STALE_IMAGE_ERROR')
})
it.each(['image', 'pdf'] as const)('Present %s late reply stays masked after target retirement', async kind => {
  let finish!: () => void
  if (kind === 'image') readImage.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve({ ok: true, path: '/old.png', dataUrl: 'data:image/png;base64,OLD', mimeType: 'image/png', size: 3 }) }))
  else readPdf.mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve({ ok: true, path: '/old.pdf', dataBase64: 'OLD', mimeType: 'application/pdf', size: 3, mtimeMs: 1 }) }))
  await draw({ path: kind === 'image' ? 'old.png' : 'old.pdf', preview: 'source', protectedPresent: ref })
  await draw(null); await act(async () => finish())
  expect(container.querySelector('[data-present-media-preview]')).toBeNull(); expect(pdfRender).not.toHaveBeenCalled()
})
it.each(['image', 'pdf'] as const)('Present %s read failure remains visible without text or native fallback', async kind => {
  if (kind === 'image') readImage.mockResolvedValueOnce({ ok: false, message: 'CURRENT_MEDIA_UNAVAILABLE' })
  else readPdf.mockRejectedValueOnce(new Error('CURRENT_MEDIA_UNAVAILABLE'))
  await draw({ path: kind === 'image' ? 'bad.png' : 'bad.pdf', preview: 'source', protectedPresent: ref })
  expect(container.textContent).toContain('CURRENT_MEDIA_UNAVAILABLE')
  expect(container.querySelector('[data-present-media-preview]')).toBeNull(); expect(read).not.toHaveBeenCalled(); expect(open).not.toHaveBeenCalled()
})
it('Present image decode error is visible and never opens its source URL', async () => {
  await draw({ path: 'broken.png', preview: 'source', protectedPresent: ref })
  const image = container.querySelector('img')!
  await act(async () => image.dispatchEvent(new Event('error')))
  expect(container.querySelector('img')).toBeNull()
  expect(container.querySelector('[role="status"]')).not.toBeNull(); expect(ordinaryOpen).not.toHaveBeenCalled()
})

function excelTarget(index = 0): WorkspaceFileTarget {
  return { path: 'label.xlsx', preview: 'source', protectedPresent: { ...ref, index } }
}
function deferredExcel<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
function excelProps(): ExcelBodyProps { return excelRender.mock.calls.at(-1)![0] as ExcelBodyProps }

it('Present Excel uses complete Host bytes/format and synchronously requests a cloned protected selection with the original index', async () => {
  readExcel.mockResolvedValueOnce({ ...excelReceipt, path: '/synthetic/host-owned.tsv', format: 'tsv', dataBase64: 'YQli' })
  const pending = deferredExcel<ToolResultLocalEffectResultV1>(); copy.mockImplementationOnce(() => pending.promise)
  const target = excelTarget(1)
  await draw(target)
  expect(readExcel).toHaveBeenCalledExactlyOnceWith({ ...target, workspaceRoot: '/synthetic' })
  expect(read).not.toHaveBeenCalled(); expect(readImage).not.toHaveBeenCalled(); expect(readPdf).not.toHaveBeenCalled()
  expect(excelProps()).toMatchObject({ owner: target, format: 'tsv', limits: excelLimits })
  expect(excelProps().data).toEqual(new Uint8Array([97, 9, 98]))
  const row: [number, number] = [0, 1], column: [number, number] = [2, 3]
  let accepted: boolean | undefined
  await act(async () => { accepted = excelProps().onProtectedCopy!(1, 'second', [{ row, column }]) })
  expect(accepted).toBe(true); expect(typeof accepted).toBe('boolean')
  row[0] = 99; column[1] = 99
  expect(copy).toHaveBeenCalledExactlyOnceWith({ ...ref, index: 1, contentDigest: excelReceipt.contentDigest,
    selection: { sheetIndex: 1, sheetId: 'second', ranges: [{ row: [0, 1], column: [2, 3] }] } })
  expect(nativeClipboard).not.toHaveBeenCalled(); expect(writeFile).not.toHaveBeenCalled(); expect(saveFile).not.toHaveBeenCalled()
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('Copied')
})

it('ordinary spreadsheet entry keeps DSH default Copy while a frozen Read with the same suffix stays source-only', async () => {
  const ordinary: WorkspaceFileTarget = { path: 'ordinary.csv' }
  await draw(ordinary)
  expect(readExcel).toHaveBeenCalledWith({ ...ordinary, workspaceRoot: '/synthetic' })
  expect(excelProps().onProtectedCopy).toBeUndefined(); expect(copy).not.toHaveBeenCalled()
  readExcel.mockClear(); excelRender.mockClear()
  const { index: _index, ...protectedRead } = ref
  await draw({ path: 'frozen.xlsx', preview: 'source', protectedRead })
  expect(readExcel).not.toHaveBeenCalled(); expect(excelRender).not.toHaveBeenCalled()
  expect(read).toHaveBeenCalled(); expect(container.textContent).toContain('Current file source')
})

it('keeps protected DOM transfer blocked while allowing the spreadsheet context Copy menu to open', async () => {
  await draw(excelTarget())
  const body = container.querySelector<HTMLElement>('[data-present-media-preview="excel"]')!
  expect(body).not.toBeNull()
  for (const type of ['copy', 'cut', 'dragstart']) {
    const event = new Event(type, { bubbles: true, cancelable: true }); body.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
  }
  const menu = new Event('contextmenu', { bubbles: true, cancelable: true }); body.dispatchEvent(menu)
  expect(menu.defaultPrevented).toBe(false)
  expect((await fileMenu()).querySelector('button[aria-label="Copy content"]')).toBeNull()
  expect(nativeClipboard).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled()
})

it.each(['unavailable', 'reject'] as const)('Present Excel %s remains visible without source or native fallback', async kind => {
  if (kind === 'unavailable') readExcel.mockResolvedValueOnce({ ok: false, message: 'CURRENT_EXCEL_UNAVAILABLE' })
  else readExcel.mockRejectedValueOnce(new Error('CURRENT_EXCEL_UNAVAILABLE'))
  await draw(excelTarget())
  expect(container.textContent).toContain('CURRENT_EXCEL_UNAVAILABLE')
  expect(excelRender).not.toHaveBeenCalled(); expect(read).not.toHaveBeenCalled(); expect(open).not.toHaveBeenCalled()
  expect(copy).not.toHaveBeenCalled(); expect(nativeClipboard).not.toHaveBeenCalled()
})

it('rejects malformed complete-byte transport before mounting or copying a Workbook', async () => {
  readExcel.mockResolvedValueOnce({ ...excelReceipt, dataBase64: '???' })
  await draw(excelTarget())
  expect(excelRender).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(nativeClipboard).not.toHaveBeenCalled()
  expect(container.textContent).toContain('Could not read the file.')
})

it('late Excel bytes cannot replace the current index or paint after retirement', async () => {
  const first = deferredExcel<WorkspaceExcelReadResult>()
  readExcel.mockImplementationOnce(() => first.promise)
  const old = excelTarget(0), current = excelTarget(1)
  await draw(old); await draw(current)
  const body = container.querySelector('[data-present-media-preview="excel"]')
  expect(excelProps().owner).toBe(current)
  await act(async () => { first.resolve({ ...excelReceipt, dataBase64: 'b2xk', contentDigest: 'f'.repeat(64) }); await first.promise })
  expect(container.querySelector('[data-present-media-preview="excel"]')).toBe(body)
  expect(excelProps().owner).toBe(current); expect(excelProps().data).toEqual(new Uint8Array([97, 44, 98]))
  const retiring = deferredExcel<WorkspaceExcelReadResult>(); readExcel.mockImplementationOnce(() => retiring.promise)
  await draw(excelTarget(2)); await draw(null)
  const renders = excelRender.mock.calls.length
  await act(async () => { retiring.resolve(excelReceipt); await retiring.promise })
  expect(container.querySelector('[data-present-media-preview="excel"]')).toBeNull()
  expect(excelRender).toHaveBeenCalledTimes(renders)
})

it('rejects a retained selection callback after target replacement and masks the pending Copy response after close', async () => {
  await draw(excelTarget(0)); const retained = excelProps().onProtectedCopy!
  await draw(excelTarget(1))
  expect(retained(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(false); expect(copy).not.toHaveBeenCalled()
  const pending = deferredExcel<ToolResultLocalEffectResultV1>(); copy.mockImplementationOnce(() => pending.promise)
  await act(async () => { expect(excelProps().onProtectedCopy!(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(true) })
  await draw(null)
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(container.querySelector('[role="status"]')).toBeNull(); expect(nativeClipboard).not.toHaveBeenCalled()
})

it.each(['unavailable', 'reject', 'throw'] as const)('reports a current protected Copy %s visibly without exporting renderer bytes', async kind => {
  if (kind === 'unavailable') copy.mockResolvedValueOnce({ ok: false, code: 'unavailable' })
  else if (kind === 'reject') copy.mockRejectedValueOnce(new Error('CURRENT_COPY_UNAVAILABLE'))
  else copy.mockImplementationOnce(() => { throw new Error('CURRENT_COPY_UNAVAILABLE') })
  await draw(excelTarget())
  await act(async () => { expect(excelProps().onProtectedCopy!(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(kind !== 'throw') })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('This local result is unavailable.')
  expect(copy).toHaveBeenCalledOnce(); expect(nativeClipboard).not.toHaveBeenCalled()
  const request = copy.mock.calls[0][0] as Record<string, unknown>
  for (const key of ['path', 'text', 'html', 'checkedHtml', 'dataBase64', 'format', 'fileIndex']) expect(request).not.toHaveProperty(key)
})

it('rejects old callbacks and Copy feedback after a same-owner media refresh', async () => {
  const target = excelTarget(), pending = deferredExcel<ToolResultLocalEffectResultV1>()
  copy.mockImplementationOnce(() => pending.promise)
  await draw(target); const previous = excelProps().onProtectedCopy!
  await act(async () => { expect(previous(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(true) })
  readExcel.mockResolvedValueOnce({ ...excelReceipt, dataBase64: 'eCx5', contentDigest: 'f'.repeat(64) })
  await act(async () => { await i18n.changeLanguage('zh') })
  expect(readExcel).toHaveBeenCalledTimes(2); expect(excelProps().owner).toBe(target)
  expect(excelProps().data).toEqual(new Uint8Array([120, 44, 121]))
  expect(previous(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(false); expect(copy).toHaveBeenCalledOnce()
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(container.querySelector('[role="status"]')).toBeNull()
})

it('only displays the latest accepted selection Copy result for the current media', async () => {
  const first = deferredExcel<ToolResultLocalEffectResultV1>(), latest = deferredExcel<ToolResultLocalEffectResultV1>()
  copy.mockImplementationOnce(() => first.promise).mockImplementationOnce(() => latest.promise)
  await draw(excelTarget()); const request = excelProps().onProtectedCopy!
  await act(async () => {
    expect(request(0, 'first', [{ row: [0, 0], column: [0, 0] }])).toBe(true)
    expect(request(1, 'second', [{ row: [0, 1], column: [0, 1] }])).toBe(true)
  })
  await act(async () => { latest.resolve({ ok: false, code: 'unavailable' }); await latest.promise })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('This local result is unavailable.')
  await act(async () => { first.resolve({ ok: true }); await first.promise })
  expect(container.querySelector('[role="status"]')?.textContent).toBe('This local result is unavailable.')
  expect(copy).toHaveBeenCalledTimes(2); expect(nativeClipboard).not.toHaveBeenCalled()
})

it('captures all same-axis Copy ranges in their original order while retaining the sealed index and digest', async () => {
  const pending = deferredExcel<ToolResultLocalEffectResultV1>(); copy.mockImplementationOnce(() => pending.promise)
  await draw(excelTarget(2))
  const ranges: Parameters<NonNullable<ExcelBodyProps['onProtectedCopy']>>[2] = [
    { row: [5, 6], column: [2, 3] }, { row: [1, 2], column: [2, 3] }
  ]
  const expected = structuredClone(ranges)
  await act(async () => { expect(excelProps().onProtectedCopy!(1, 'second', ranges)).toBe(true) })
  ranges[0].row[0] = 99; ranges[1].column[1] = 99; ranges.reverse()
  expect(copy).toHaveBeenCalledExactlyOnceWith({ ...ref, index: 2, contentDigest: excelReceipt.contentDigest,
    selection: { sheetIndex: 1, sheetId: 'second', ranges: expected } })
  const sent = (copy.mock.calls[0][0] as { selection: { ranges: typeof ranges } }).selection.ranges
  expect(sent).not.toBe(ranges)
  for (const range of sent) for (const source of ranges) {
    expect(range).not.toBe(source); expect(range.row).not.toBe(source.row); expect(range.column).not.toBe(source.column)
  }
  expect(nativeClipboard).not.toHaveBeenCalled()
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
})

it('copies only sealed identities and range coordinates even when the selection object carries renderer HTML, style and value', async () => {
  copy.mockResolvedValueOnce({ ok: true })
  await draw(excelTarget(1))
  const ranges = [{ row: [0, 1] as [number, number], column: [2, 3] as [number, number],
    html: '<table><tr><td>FORGED_RENDERER_HTML</td></tr></table>',
    style: { color: '#123456' }, value: 'FORGED_RENDERER_VALUE',
    config: { rowhidden: { 0: 0 }, colhidden: { 2: 0 } }, rowCount: 8, columnCount: 4 }]
  await act(async () => { expect(excelProps().onProtectedCopy!(1, 'visible-second', ranges)).toBe(true) })
  expect(copy).toHaveBeenCalledExactlyOnceWith({ ...ref, index: 1, contentDigest: excelReceipt.contentDigest,
    selection: { sheetIndex: 1, sheetId: 'visible-second', ranges: [{ row: [0, 1], column: [2, 3] }] } })
  expect(nativeClipboard).not.toHaveBeenCalled(); expect(writeFile).not.toHaveBeenCalled(); expect(saveFile).not.toHaveBeenCalled()
})


it('keeps Excel identity, direct download and native zoom anchor in the single shared operation row', async () => {
  await draw(excelTarget(), true)
  const zoom = container.querySelector<HTMLElement>('[data-excel-preview-controls]')
  const header = container.querySelector<HTMLElement>('[data-workspace-viewer-header]')!
  expect(zoom).not.toBeNull(); expect(excelProps().zoomControlTarget).toBe(zoom)
  expect(container.querySelectorAll('header')).toHaveLength(1)
  expect(header.querySelector('button[aria-haspopup="menu"]')?.textContent).toBe('workbook.xlsx')
  expect(header.querySelector<HTMLButtonElement>('button[aria-label="Download"]')?.disabled).toBe(false)
  expect(header.contains(zoom)).toBe(true)
  const menu = await fileMenu()
  expect(container.contains(menu)).toBe(false)
  expect(menu.querySelector<HTMLButtonElement>('button[aria-label="Open"]')?.disabled).toBe(false)
  expect(menu.querySelector('button[aria-label="Copy content"]')).toBeNull()
  expect(menu.querySelector('button[aria-label="Download"]')).toBeNull()
  const trigger = header.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]')!
  await act(async () => menu.querySelector<HTMLButtonElement>('button:not(:disabled)')!.focus())
  const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
  await act(async () => menu.dispatchEvent(escape))
  expect(escape.defaultPrevented).toBe(true)
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(document.activeElement).toBe(trigger)
  await fileMenu()
  await act(async () => document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(container.querySelector('.ds-code-sidebar-breadcrumbs')).toBeNull()
  expect(nativeClipboard).not.toHaveBeenCalled(); expect(saveFile).not.toHaveBeenCalled()
  await draw(null)
  expect(container.querySelector('[data-excel-preview-controls]')).toBeNull()
})

it('preserves standalone multi-file navigation and close callbacks inside the filename menu', async () => {
  const first = excelTarget(), second = { path: '/synthetic/second.csv' }
  const onSelectTarget = vi.fn(), onCloseTarget = vi.fn()
  await act(async () => root.render(createElement(WorkspaceFilePreviewPanel, {
    target: first, openTargets: [first, second], workspaceRoot: '/synthetic', onClose: () => undefined, onSelectTarget, onCloseTarget
  })))
  const menu = await fileMenu()
  const tabs = menu.querySelectorAll<HTMLElement>('[role="tab"]')
  expect(tabs).toHaveLength(2)
  expect(tabs[0].getAttribute('aria-selected')).toBe('true')
  await act(async () => tabs[1].dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })))
  expect(onSelectTarget).toHaveBeenCalledExactlyOnceWith(second)
  const close = tabs[1].querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:filePreviewCloseTab', { file: 'second.csv' })}"]`)!
  await act(async () => close.click())
  expect(onCloseTarget).toHaveBeenCalledExactlyOnceWith(second)
  expect(onSelectTarget).toHaveBeenCalledTimes(1)
  expect(container.querySelectorAll('[data-workspace-viewer-header]')).toHaveLength(1)
  expect(container.querySelector('.ds-code-sidebar-tabs')).toBeNull()
})

function downloadStatus(): string | null { return container.querySelector('[data-workspace-viewer-header] [role="status"]')?.textContent ?? null }
async function clickDownload(): Promise<HTMLButtonElement> {
  const button = container.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-label="Download"]')!
  expect(button).not.toBeNull()
  await act(async () => button.click())
  return button
}

it('downloads an ordinary original file through existing Save As using its target workspace without preview bytes', async () => {
  await draw({ path: 'original.csv', workspaceRoot: '/synthetic/owner' }, true)
  await clickDownload()
  expect(saveFile).toHaveBeenCalledExactlyOnceWith({ sourcePath: 'original.csv', workspaceRoot: '/synthetic/owner' })
  expect(savePresent).not.toHaveBeenCalled(); expect(writeFile).not.toHaveBeenCalled(); expect(nativeClipboard).not.toHaveBeenCalled()
  expect(downloadStatus()).toBe('Saved')
  expect(container.querySelector('[data-workspace-viewer-header] [role="status"]')).not.toBeNull()
})

it('saves Present original bytes through exactly seven cloned identity fields without any ordinary fallback', async () => {
  const protectedPresent = { ...ref, index: 1, path: '/forged/source.xlsx', contentDigest: 'e'.repeat(64),
    dataBase64: 'FORGED_BYTES', destination: '/forged/destination', section: 'FORGED_SECTION',
    selection: { sheetIndex: 0, sheetId: 'FORGED_SHEET', ranges: [] } }
  await draw({ path: 'display-label.xlsx', preview: 'source', protectedPresent }, true)
  await clickDownload()
  expect(savePresent).toHaveBeenCalledExactlyOnceWith({ ...ref, index: 1 })
  expect(savePresent.mock.calls[0][0]).not.toBe(protectedPresent)
  expect(downloadStatus()).toBe('Saved')
  expect(saveFile).not.toHaveBeenCalled(); expect(copy).not.toHaveBeenCalled(); expect(writeFile).not.toHaveBeenCalled(); expect(nativeClipboard).not.toHaveBeenCalled()
})

it.each(['canceled', 'unavailable', 'reject', 'malformed'] as const)('reports Present save %s honestly and permits retry without ordinary Save As', async outcome => {
  if (outcome === 'reject') savePresent.mockRejectedValueOnce(new Error('PRIVATE_SAVE_ERROR'))
  else if (outcome === 'malformed') savePresent.mockResolvedValueOnce({ ok: true, path: 'PRIVATE_SAVE_ERROR' } as unknown as ToolResultLocalEffectResultV1)
  else savePresent.mockResolvedValueOnce({ ok: false, code: outcome })
  await draw(excelTarget())
  const button = await clickDownload()
  expect(downloadStatus()).toBe(outcome === 'canceled' ? 'Cancelled' : 'Save failed')
  expect(container.textContent).not.toContain('PRIVATE_SAVE_ERROR')
  expect(button.disabled).toBe(false); expect(saveFile).not.toHaveBeenCalled()
  await clickDownload()
  expect(savePresent).toHaveBeenCalledTimes(2); expect(downloadStatus()).toBe('Saved'); expect(saveFile).not.toHaveBeenCalled()
})

it.each(['canceled', 'failure', 'reject', 'malformed'] as const)('reports ordinary Save As %s without exposing backend error content', async outcome => {
  if (outcome === 'reject') saveFile.mockRejectedValueOnce(new Error('PRIVATE_SAVE_ERROR'))
  else if (outcome === 'malformed') saveFile.mockResolvedValueOnce({ ok: true, path: '' })
  else saveFile.mockResolvedValueOnce({ ok: false, canceled: outcome === 'canceled', message: 'PRIVATE_SAVE_ERROR' })
  await draw({ path: 'original.csv' })
  const button = await clickDownload()
  expect(saveFile).toHaveBeenCalledExactlyOnceWith({ sourcePath: 'original.csv', workspaceRoot: '/synthetic' })
  expect(downloadStatus()).toBe(outcome === 'canceled' ? 'Cancelled' : 'Save failed')
  expect(container.textContent).not.toContain('PRIVATE_SAVE_ERROR')
  expect(button.disabled).toBe(false); expect(savePresent).not.toHaveBeenCalled()
})

it('keeps original download available during a pending preview and after the preview byte limit refuses it', async () => {
  const preview = deferredExcel<WorkspaceExcelReadResult>()
  readExcel.mockImplementationOnce(() => preview.promise)
  await draw(excelTarget())
  expect(container.textContent).toContain('Reading file…')
  expect((await clickDownload()).disabled).toBe(false)
  expect(downloadStatus()).toBe('Saved')
  await act(async () => { preview.resolve({ ok: false, message: 'PREVIEW_BYTE_LIMIT' }); await preview.promise })
  expect(container.textContent).toContain('PREVIEW_BYTE_LIMIT')
  await clickDownload()
  expect(savePresent).toHaveBeenCalledTimes(2); expect(saveFile).not.toHaveBeenCalled()
})

it.each(['source', 'frozenRead'] as const)('does not offer ordinary download for %s authority', async kind => {
  const { index: _index, ...protectedRead } = ref
  await draw({ path: 'same.xlsx', preview: 'source', ...(kind === 'frozenRead' ? { protectedRead } : {}) })
  expect(container.querySelector('button[aria-label="Download"]')).toBeNull()
  expect(saveFile).not.toHaveBeenCalled(); expect(savePresent).not.toHaveBeenCalled()
})

it('prevents same-frame repeated download while a native save is pending', async () => {
  const pending = deferredExcel<ToolResultLocalEffectResultV1>()
  savePresent.mockImplementationOnce(() => pending.promise)
  await draw(excelTarget())
  const button = container.querySelector<HTMLButtonElement>('button[aria-label="Download"]')!
  await act(async () => { button.click(); button.click() })
  expect(savePresent).toHaveBeenCalledOnce(); expect(button.disabled).toBe(true)
  expect(button.getAttribute('aria-busy')).toBe('true'); expect(downloadStatus()).toBe('Saving…')
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(downloadStatus()).toBe('Saved'); expect(button.disabled).toBe(false); expect(button.getAttribute('aria-busy')).toBe('false')
})

it.each(['replace', 'retire', 'return', 'root'] as const)('masks a late download after owner %s and allows the current owner to save', async transition => {
  const pending = deferredExcel<ToolResultLocalEffectResultV1>()
  savePresent.mockImplementationOnce(() => pending.promise)
  const old = excelTarget(), next = excelTarget(1)
  await draw(old); await clickDownload()
  if (transition === 'root') await draw(old, false, '/synthetic/other')
  else {
    await draw(transition === 'retire' ? null : next)
    if (transition === 'return') await draw(old)
  }
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(downloadStatus()).toBeNull()
  if (transition === 'retire') {
    expect(container.querySelector('button[aria-label="Download"]')).toBeNull()
    await draw(next)
  }
  await clickDownload()
  expect(savePresent).toHaveBeenCalledTimes(2); expect(downloadStatus()).toBe('Saved'); expect(saveFile).not.toHaveBeenCalled()
})

it('discards a native save completion after unmount without painting a remounted panel', async () => {
  const pending = deferredExcel<ToolResultLocalEffectResultV1>()
  savePresent.mockImplementationOnce(() => pending.promise)
  await draw(excelTarget()); await clickDownload()
  await act(async () => root.unmount())
  root = createRoot(container)
  await draw(excelTarget(1))
  await act(async () => { pending.resolve({ ok: true }); await pending.promise })
  expect(downloadStatus()).toBeNull()
  await clickDownload()
  expect(downloadStatus()).toBe('Saved'); expect(savePresent).toHaveBeenCalledTimes(2)
})


it('wires only the current PDF zoom anchor while keeping protected selection passive', async () => {
  const target: WorkspaceFileTarget = { path: 'first.pdf', preview: 'source', protectedPresent: ref }
  await draw(target, true)
  const anchor = container.querySelector<HTMLElement>('[data-pdf-preview-controls]')!
  expect(anchor?.isConnected).toBe(true)
  expect(pdfRender.mock.calls.at(-1)![0].zoomControlTarget).toBe(anchor)
  const selection = pdfRender.mock.calls.at(-1)![0].onSelectionChange
  await draw({ path: 'second.pdf', preview: 'source', protectedPresent: { ...ref, index: 1 } }, true)
  const next = container.querySelector<HTMLElement>('[data-pdf-preview-controls]')!
  expect(next).not.toBe(anchor); expect(anchor.isConnected).toBe(false)
  expect(pdfRender.mock.calls.at(-1)![0].zoomControlTarget).toBe(next)
  expect(pdfRender.mock.calls.at(-1)![0].onSelectionChange).toBe(selection)
  await draw(null, true)
  expect(next.isConnected).toBe(false)
  expect(container.querySelector('[data-pdf-preview-controls]')).toBeNull()
  expect(copy).not.toHaveBeenCalled(); expect(nativeClipboard).not.toHaveBeenCalled()
})

const htmlGrant = (id: string) => ({ ok: true as const, absolutePath: '/synthetic/page.html',
  fileUrl: 'analytix-prototype://' + id + '/page.html', grantId: id, partition: 'analytix-proto-' + id })
function htmlBridge() {
  const authorize = vi.fn(async (_payload: unknown) => htmlGrant('a'.repeat(48)))
  const revoke = vi.fn(async (_payload: unknown) => ({ ok: true }))
  Object.assign(window.analytix, { write: { authorizeWritePrototype: authorize, revokeWritePrototype: revoke } })
  read.mockResolvedValue({ ok: true, path: '/synthetic/page.html', content: '<script>window.secret = true</script>', size: 42, truncated: false })
  return { authorize, revoke }
}
async function clickHtmlRun() {
  const button = container.querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:writeHtmlEmbedRun') + '"]')!
  expect(button).not.toBeNull(); await act(async () => button.click())
}
it('defaults HTML to Source and mounts only after explicit Run with the granted partition', async () => {
  const { authorize, revoke } = htmlBridge()
  await draw({ path: 'page.html' })
  expect(authorize).not.toHaveBeenCalled(); expect(container.querySelector('webview')).toBeNull()
  await clickHtmlRun()
  expect(authorize).toHaveBeenCalledExactlyOnceWith({ path: '/synthetic/page.html', workspaceRoot: '/synthetic', mode: 'workspace-html' })
  expect(container.querySelector('webview')?.getAttribute('partition')).toBe('analytix-proto-' + 'a'.repeat(48))
  const source = container.querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:filePreviewShowSource') + '"]')!
  await act(async () => source.click())
  expect(container.querySelector('webview')).toBeNull(); expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
})
it.each(['source', 'read'] as const)('never authorizes protected/source HTML (%s)', async kind => {
  const { authorize } = htmlBridge()
  const { index: _index, ...protectedRead } = ref
  await draw({ path: 'page.html', ...(kind === 'source' ? { preview: 'source' as const } : { protectedRead }) })
  expect(container.querySelector('button[aria-label="' + i18n.t('common:writeHtmlEmbedRun') + '"]')).toBeNull()
  expect(authorize).not.toHaveBeenCalled(); expect(container.querySelector('webview')).toBeNull()
})
it('retires late old authorization without mounting or revoking the next HTML grant', async () => {
  const { authorize, revoke } = htmlBridge()
  let finish!: (value: ReturnType<typeof htmlGrant>) => void
  authorize.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw({ path: 'first.html' }); await clickHtmlRun()
  authorize.mockResolvedValueOnce(htmlGrant('b'.repeat(48)))
  await draw({ path: 'second.html' }); await clickHtmlRun()
  await act(async () => finish(htmlGrant('a'.repeat(48))))
  expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
  expect(container.querySelector('webview')?.getAttribute('partition')).toBe('analytix-proto-' + 'b'.repeat(48))
  await draw(null); expect(revoke).toHaveBeenLastCalledWith({ grantId: 'b'.repeat(48) })
})
it('masks late HTML before the next Office preview and releases on panel close', async () => {
  const { authorize, revoke } = htmlBridge()
  let finish!: (value: ReturnType<typeof htmlGrant>) => void
  authorize.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw({ path: 'old.html' }); await clickHtmlRun()
  await draw({ path: 'workbook.xlsx' })
  await act(async () => finish(htmlGrant('a'.repeat(48))))
  expect(container.querySelector('webview')).toBeNull(); expect(container.querySelector('[data-present-media-preview="excel"]')).not.toBeNull()
  expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
  await draw({ path: 'new.html' }); await clickHtmlRun()
  const close = container.querySelector<HTMLButtonElement>('.ds-file-viewer-close')!
  await act(async () => close.click())
  expect(container.querySelector('webview')).toBeNull(); expect(revoke).toHaveBeenCalledTimes(2)
})
it('fails closed without revocation capability', async () => {
  const { authorize } = htmlBridge()
  delete window.analytix.write.revokeWritePrototype
  await draw({ path: 'page.html' }); await clickHtmlRun()
  expect(authorize).not.toHaveBeenCalled(); expect(container.querySelector('webview')).toBeNull()
  expect(container.querySelector('[role="status"]')?.textContent).toBe(i18n.t('common:writeHtmlEmbedLoadFailed'))
})

it('retires detached legacy embeds and never remounts a late grant after reattachment', async () => {
  const { authorize, revoke } = htmlBridge()
  let finish!: (value: ReturnType<typeof htmlGrant>) => void
  authorize.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const handle = createHtmlEmbedHandle({ rawSrc: '/synthetic/proto/page.html', alt: '', filePath: '/synthetic/doc.md', workspaceRoot: '/synthetic' })
  container.append(handle.element)
  handle.run(); handle.element.remove(); container.append(handle.element)
  await act(async () => finish(htmlGrant('a'.repeat(48))))
  expect(container.querySelector('webview')).toBeNull(); expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
  handle.dispose()
})
it('reload obtains a fresh grant and authorization failures never mount active content', async () => {
  const { authorize, revoke } = htmlBridge()
  await draw({ path: 'page.html' }); await clickHtmlRun()
  authorize.mockResolvedValueOnce(htmlGrant('b'.repeat(48)))
  const reload = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:writeHtmlEmbedReload'))!
  await act(async () => reload.click())
  expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
  expect(container.querySelector('webview')?.getAttribute('partition')).toBe('analytix-proto-' + 'b'.repeat(48))
  authorize.mockRejectedValueOnce(Error('secret must not display'))
  await act(async () => reload.click())
  expect(container.querySelector('webview')).toBeNull(); expect(container.textContent).not.toContain('secret must not display')
  expect(container.querySelector('[role="status"]')?.textContent).toBe(i18n.t('common:writeHtmlEmbedLoadFailed'))
})

it('runs Present HTML only with its exact selector and retires it on current view invalidation', async () => {
  const { authorize, revoke } = htmlBridge()
  let invalidate!: (viewId: string | null) => void
  Object.assign(window.analytix.runtime, { onToolResultLocalDisplayInvalidated: vi.fn(handler => { invalidate = handler; return vi.fn() }) })
  await draw({ path: 'page.html', preview: 'source', protectedPresent: ref })
  expect(authorize).not.toHaveBeenCalled(); expect(container.querySelector('[data-protected-source-preview]')).not.toBeNull()
  await clickHtmlRun()
  expect(authorize).toHaveBeenCalledExactlyOnceWith({ protectedPresent: ref, mode: 'workspace-html' })
  expect(container.querySelector('webview')).not.toBeNull()
  await act(async () => invalidate('22222222-2222-4222-8222-222222222222'))
  expect(container.querySelector('webview')).not.toBeNull()
  await act(async () => invalidate(ref.viewId))
  expect(container.querySelector('webview')).toBeNull()
  expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
})
it('retires a late Present grant on global invalidation and rejects a malformed guest grant', async () => {
  const { authorize, revoke } = htmlBridge()
  let invalidate!: (viewId: string | null) => void, finish!: (grant: ReturnType<typeof htmlGrant>) => void
  Object.assign(window.analytix.runtime, { onToolResultLocalDisplayInvalidated: vi.fn(handler => { invalidate = handler; return vi.fn() }) })
  authorize.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw({ path: 'page.html', protectedPresent: ref }); await clickHtmlRun()
  await act(async () => invalidate(null)); await act(async () => finish(htmlGrant('a'.repeat(48))))
  expect(container.querySelector('webview')).toBeNull(); expect(revoke).toHaveBeenCalledOnce()
  authorize.mockResolvedValueOnce({ ...htmlGrant('b'.repeat(48)), partition: 'persist:ordinary' })
  await clickHtmlRun(); expect(container.querySelector('webview')).toBeNull()
})

it('does not inherit HTML Run when the same path receives a different complete Present selector', async () => {
  const { authorize, revoke } = htmlBridge()
  const target: WorkspaceFileTarget = { path: 'page.html', preview: 'source', protectedPresent: ref }
  await draw(target); await clickHtmlRun(); expect(container.querySelector('webview')).not.toBeNull()
  target.protectedPresent = { ...ref, index: 1, snapshotDigest: 'd'.repeat(64) }
  await draw(target)
  expect(container.querySelector('webview')).toBeNull(); expect(authorize).toHaveBeenCalledOnce()
  expect(revoke).toHaveBeenCalledExactlyOnceWith({ grantId: 'a'.repeat(48) })
})

it('fences old invalidators when an existing target moves to a new Present view', async () => {
  const { authorize, revoke } = htmlBridge(), callbacks: ((viewId: string | null) => void)[] = []
  Object.assign(window.analytix.runtime, { onToolResultLocalDisplayInvalidated: vi.fn(handler => { callbacks.push(handler); return vi.fn() }) })
  const target: WorkspaceFileTarget = { path: 'page.html', protectedPresent: ref }
  await draw(target); await clickHtmlRun()
  const oldCallback = callbacks[0]
  const next = { ...ref, viewId: '22222222-2222-4222-8222-222222222222' }
  target.protectedPresent = next; await draw(target)
  authorize.mockResolvedValueOnce(htmlGrant('b'.repeat(48))); await clickHtmlRun()
  expect(authorize).toHaveBeenLastCalledWith({ protectedPresent: next, mode: 'workspace-html' })
  await act(async () => oldCallback(null))
  await act(async () => callbacks.at(-1)!(ref.viewId))
  expect(container.querySelector('webview')?.getAttribute('partition')).toBe('analytix-proto-' + 'b'.repeat(48))
  await act(async () => callbacks.at(-1)!(next.viewId))
  expect(container.querySelector('webview')).toBeNull()
  expect(revoke.mock.calls.map(([payload]) => payload)).toEqual([{ grantId: 'a'.repeat(48) }, { grantId: 'b'.repeat(48) }])
})


it('leaves workspace collapse and tab close to the shared strip while preserving file controls', async () => {
  readExcel.mockResolvedValue({ ...excelReceipt, path: '/synthetic/report.csv', format: 'csv' })
  await draw({ path: 'report.csv' }, true)
  expect(container.querySelector('.ds-file-viewer-close')).toBeNull()
  expect(container.querySelector('button[aria-label="Collapse right sidebar"]')).toBeNull()
  expect(container.querySelector('.ds-code-sidebar-tabs')).toBeNull()
  expect(container.querySelector('[data-workspace-viewer-header] button[aria-haspopup="menu"]')?.textContent).toBe('report.csv')
  await draw({ path: 'report.csv' }, false)
  expect(container.querySelector('button[aria-label="Collapse right sidebar"]')).not.toBeNull()
})


it('keeps direct download, view mode and workspace controls in one row without duplicate collapse or metadata rows', async () => {
  const onToggleFocus = vi.fn(), onCollapse = vi.fn(), onClose = vi.fn()
  await act(async () => root.render(createElement(WorkspaceFilePreviewPanel, {
    target: { path: '/synthetic/current.md' }, workspaceRoot: '/synthetic', tabbedWorkspace: true, onClose,
    viewerControls: { focused: false, onToggleFocus, onCollapse }
  })))
  const header = container.querySelector<HTMLElement>('[data-workspace-viewer-header]')!
  expect(container.querySelectorAll('header')).toHaveLength(1)
  expect(container.querySelector('.ds-code-sidebar-breadcrumbs')).toBeNull()
  expect(container.querySelector('.ds-file-viewer-close')).toBeNull()
  const download = header.querySelector<HTMLButtonElement>('button[aria-label="Download"]')!
  expect(download.querySelector('svg[aria-hidden="true"]')).not.toBeNull()
  expect(download.textContent).toBe('')
  const filename = header.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]')!
  expect(filename.textContent).toBe('current.md')
  expect(filename.getAttribute('aria-label')).toContain('19 B')
  expect(filename.getAttribute('aria-label')).toContain('text')
  const source = header.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:filePreviewShowSource')}"]`)!
  expect(source.querySelector('svg')).not.toBeNull()
  await act(async () => source.click())
  expect(header.querySelector(`[aria-label="${i18n.t('common:filePreviewRenderMarkdown')}"]`)).not.toBeNull()
  await act(async () => header.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:workbenchFocus')}"]`)!.click())
  await act(async () => header.querySelector<HTMLButtonElement>(`[aria-label="${i18n.t('common:workbenchCollapse')}"]`)!.click())
  expect(onToggleFocus).toHaveBeenCalledTimes(1); expect(onCollapse).toHaveBeenCalledTimes(1); expect(onClose).not.toHaveBeenCalled()
  expect(container.querySelector('button[aria-label="Collapse right sidebar"]')).toBeNull()
})


it('retires the filename portal when a retained preview becomes hidden without changing its read owner', async () => {
  const target = { path: '/synthetic/current.md' }, onClose = vi.fn()
  const render = async (visible: boolean) => act(async () => root.render(createElement(WorkspaceFilePreviewPanel, {
    target, workspaceRoot: '/synthetic', tabbedWorkspace: true, visible, onClose
  })))
  await render(true)
  await fileMenu()
  expect(document.querySelector('[role="menu"]')).not.toBeNull()
  expect(read).toHaveBeenCalledTimes(1)
  await render(false)
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(container.querySelector<HTMLButtonElement>('[data-workspace-viewer-header] button[aria-haspopup="menu"]')!.disabled).toBe(true)
  expect(container.textContent).toContain('Current file source')
  expect(read).toHaveBeenCalledTimes(1)
  expect(open).not.toHaveBeenCalled(); expect(saveFile).not.toHaveBeenCalled(); expect(savePresent).not.toHaveBeenCalled()
})
