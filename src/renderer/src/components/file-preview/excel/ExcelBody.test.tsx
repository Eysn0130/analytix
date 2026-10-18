// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workbook } from '@fortune-sheet/react'
import type { ExcelPreview } from '@shared/excel/model'
import type { WorkspaceFileTarget } from '@shared/workspace-file'
import i18n from '../../../i18n'
import { en, zh } from './locales'
import { ExcelBody } from './ExcelBody'
import type { LoadedExcelBodyProps } from './LazyExcelBody'

// This owner test captures the actual JSX prop shape. The separately admitted
// Fortune core/react integration tests must prove native Copy dispatch is cut off.
const io = vi.hoisted(() => ({ parse: vi.fn(), workbook: vi.fn() }))
vi.mock('./parse', () => ({ parseExcel: io.parse }))
vi.mock('@fortune-sheet/react/dist/index.css?inline', () => ({ default: '' }))
vi.mock('@fortune-sheet/react', () => ({ Workbook: (props: ComponentProps<typeof Workbook>) => {
  io.workbook(props)
  return createElement('div', { 'data-mocked-workbook': true })
} }))

type WorkbookProps = ComponentProps<typeof Workbook>
type ProtectedCopy = NonNullable<WorkbookProps['protectedCopy']>
const limits = { maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000 }
const data = new Uint8Array([65, 44, 66])
const owner: WorkspaceFileTarget = { path: 'first.xlsx', preview: 'source', protectedPresent: {
  threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
  snapshotDigest: 'c'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', index: 0
} }
const preview: ExcelPreview = { sheets: [
  { id: 'first', name: 'First', row: 1, column: 1, celldata: [{ r: 0, c: 0, v: { f: '=1-1', v: 0, m: '0' } }] },
  { id: 'second', name: 'Second', row: 8, column: 4, celldata: [] }
], missingResults: 0, unsupportedFeatures: ['images'] }
const selection: Parameters<ProtectedCopy>[0] = { sheetId: 'second', ranges: [{ row: [0, 1], column: [0, 1] }] }
let root: Root, container: HTMLDivElement
const observerInstances: Array<{ observe: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn>; callback: ResizeObserverCallback }> = []
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
async function draw(overrides: Partial<LoadedExcelBodyProps> = {}) {
  const props: LoadedExcelBodyProps = { owner, data, format: 'xlsx', limits,
    loading: createElement('p', { role: 'status' }, 'WAITING_CURRENT_WORKBOOK'), ...overrides }
  await act(async () => root.render(createElement(ExcelBody, props)))
}
async function resolveParse(pending: ReturnType<typeof deferred<ExcelPreview>>, value = preview) {
  await act(async () => { pending.resolve(value); await pending.promise })
}
function workbookProps(): WorkbookProps { return io.workbook.mock.calls.at(-1)![0] as WorkbookProps }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  i18n.addResourceBundle('en', 'excel', en, true, true); i18n.addResourceBundle('zh', 'excel', zh, true, true)
  await i18n.changeLanguage('en')
  io.parse.mockReset(); io.parse.mockResolvedValue(preview); io.workbook.mockClear(); observerInstances.length = 0
  vi.stubGlobal('ResizeObserver', class {
    observe = vi.fn(); disconnect = vi.fn(); unobserve = vi.fn()
    constructor(readonly callback: ResizeObserverCallback) { observerInstances.push(this) }
  })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('current workbook readonly and protected Copy prop', () => {
  it('keeps the admitted readonly controls, scoped styles, notices and pane resize lifetime', async () => {
    await draw({ onProtectedCopy: null })
    expect(io.parse).toHaveBeenCalledExactlyOnceWith(data, 'xlsx', limits, expect.any(AbortSignal))
    expect(workbookProps()).toMatchObject({ data: preview.sheets, allowEdit: false, forceCalculation: false,
      showToolbar: false, showFormulaBar: true, showSheetTabs: true, readonlyViewport: true, cellContextMenu: ['copy'],
      headerContextMenu: [], sheetTabContextMenu: [], filterContextMenu: [] })
    expect(typeof workbookProps().protectedCopy).toBe('function')
    expect(container.querySelector('[data-excel-preview] style')?.textContent).toContain('@scope (:is([data-excel-preview], [data-excel-preview-controls]))')
    expect(container.querySelector('[data-excel-unsupported-notice]')?.textContent).toContain('images')
    expect(container.querySelector('[data-excel-formula-warning]')?.getAttribute('aria-label')).toBe(en.formulaWarning)
    const observer = observerInstances.at(-1)!
    expect(observer.observe).toHaveBeenCalledOnce()
    const dispatch = vi.spyOn(window, 'dispatchEvent')
    observer.callback([], {} as ResizeObserver)
    expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ type: 'resize' }))
    await act(async () => root.render(null))
    expect(observer.disconnect).toHaveBeenCalledOnce()
    const signal = io.parse.mock.calls[0][3] as AbortSignal
    expect(signal.aborted).toBe(true)
  })

  it('sends only the local chrome anchor while keeping original sheet data and parser lifetime', async () => {
    const zoomControlTarget = document.createElement('div')
    await draw({ zoomControlTarget })
    expect(workbookProps().zoomControlTarget).toBe(zoomControlTarget)
    expect(workbookProps().data).toBe(preview.sheets)
    expect(preview.sheets.map(sheet => [sheet.row, sheet.column])).toEqual([[1, 1], [8, 4]])
    await draw({ zoomControlTarget: document.createElement('div') })
    expect(io.parse).toHaveBeenCalledOnce()
  })

  it('keeps null protected rather than omitting takeover and keeps undefined ordinary', async () => {
    const ordinaryOwner = { path: 'ordinary.csv' }
    await draw({ owner: ordinaryOwner, onProtectedCopy: null })
    expect(workbookProps().protectedCopy!(selection)).toBe(false)
    await draw({ owner: ordinaryOwner })
    expect(workbookProps().protectedCopy).toBeUndefined()
    expect(io.parse).toHaveBeenCalledOnce()
  })

  it('synchronously maps current sheet identity and clones the single-range array before accepting the private request', async () => {
    const copy = vi.fn((_index: number, _id: string, ranges: Array<{ row: [number, number]; column: [number, number] }>) => {
      ranges[0].row[0] = 99; ranges[0].column[1] = 99; return true
    })
    await draw({ onProtectedCopy: copy })
    const input = { sheetId: 'second', ranges: [{ row: [0, 1] as [number, number], column: [0, 1] as [number, number] }] }
    const accepted = workbookProps().protectedCopy!(input)
    expect(accepted).toBe(true); expect(typeof accepted).toBe('boolean')
    expect(copy).toHaveBeenCalledOnce(); expect(copy.mock.calls[0].slice(0, 2)).toEqual([1, 'second'])
    expect(input).toEqual(selection)
    expect(copy.mock.calls[0][2]).not.toBe(input.ranges)
    expect(copy.mock.calls[0][2][0]).not.toBe(input.ranges[0])
    expect(copy.mock.calls[0][2][0].row).not.toBe(input.ranges[0].row)
    expect(copy.mock.calls[0][2][0].column).not.toBe(input.ranges[0].column)
    expect(workbookProps().protectedCopy!({ ...selection, sheetId: 'retired-sheet' })).toBe(false)
    expect(copy).toHaveBeenCalledOnce()
  })

  it('clones every legal same-column range without sorting their input order', async () => {
    const input: Parameters<ProtectedCopy>[0] = { sheetId: 'second', ranges: [
      { row: [5, 6], column: [0, 1] }, { row: [1, 2], column: [0, 1] }
    ] }
    const before = structuredClone(input)
    const captured: unknown[] = []
    const copy = vi.fn((_index: number, _id: string, ranges: Array<{ row: [number, number]; column: [number, number] }>) => {
      captured.push(structuredClone(ranges))
      ranges[0].row[0] = 99; ranges[1].column[1] = 99; ranges.reverse(); return true
    })
    await draw({ onProtectedCopy: copy })
    expect(workbookProps().protectedCopy!(input)).toBe(true)
    expect(copy).toHaveBeenCalledOnce(); expect(copy.mock.calls[0].slice(0, 2)).toEqual([1, 'second'])
    expect(captured).toEqual([before.ranges]); expect(input).toEqual(before)
    const sent = copy.mock.calls[0][2]
    expect(sent).not.toBe(input.ranges)
    for (const range of sent) {
      expect(input.ranges).not.toContain(range)
      for (const source of input.ranges) {
        expect(range.row).not.toBe(source.row); expect(range.column).not.toBe(source.column)
      }
    }
  })

  it.each(['throw', 'promise'] as const)('rejects %s callbacks with a synchronous false result', async mode => {
    const copy = vi.fn(() => { if (mode === 'throw') throw new Error('COPY_NOT_READY'); return Promise.resolve(true) })
    await draw({ onProtectedCopy: copy as unknown as NonNullable<LoadedExcelBodyProps['onProtectedCopy']> })
    const accepted = workbookProps().protectedCopy!(selection)
    expect(accepted).toBe(false); expect(typeof accepted).toBe('boolean'); expect(copy).toHaveBeenCalledOnce()
  })

  it('does not reparse identical owner, retained bytes, format and limits when only the current Copy callback changes', async () => {
    const first = vi.fn(() => true), next = vi.fn(() => true)
    await draw({ onProtectedCopy: first }); const takeOver = workbookProps().protectedCopy!
    await draw({ onProtectedCopy: next })
    expect(io.parse).toHaveBeenCalledOnce(); expect(workbookProps().protectedCopy).toBe(takeOver)
    expect(takeOver(selection)).toBe(true); expect(first).not.toHaveBeenCalled(); expect(next).toHaveBeenCalledOnce()
  })

  it('rejects an old handler immediately while the same bytes belong to a replacement owner, then rejects late parser data', async () => {
    const copy = vi.fn(() => true)
    await draw({ onProtectedCopy: copy }); const retiredCopy = workbookProps().protectedCopy!
    const old = deferred<ExcelPreview>(), next = deferred<ExcelPreview>()
    io.parse.mockImplementationOnce(() => old.promise).mockImplementationOnce(() => next.promise)
    const firstNewOwner = { ...owner, path: 'second.xlsx' }, finalOwner = { ...owner, path: 'third.xlsx' }
    await draw({ owner: firstNewOwner, onProtectedCopy: copy })
    expect(container.querySelector('[data-mocked-workbook]')).toBeNull()
    expect(retiredCopy(selection)).toBe(false); expect(copy).not.toHaveBeenCalled()
    await draw({ owner: finalOwner, onProtectedCopy: copy })
    expect((io.parse.mock.calls[1][3] as AbortSignal).aborted).toBe(true)
    await resolveParse(old, { ...preview, sheets: [{ id: 'old-only', name: 'Retired', celldata: [] }] })
    expect(container.querySelector('[data-mocked-workbook]')).toBeNull()
    await resolveParse(next)
    expect(workbookProps().data).toEqual(preview.sheets)
    expect(retiredCopy(selection)).toBe(false); expect(copy).not.toHaveBeenCalled()
    expect(workbookProps().protectedCopy!(selection)).toBe(true); expect(copy).toHaveBeenCalledOnce()
  })

  it('shows parser unavailability, retires its signal, and only makes Retry selectable after a fresh success', async () => {
    const copy = vi.fn(() => true)
    io.parse.mockRejectedValueOnce(new Error('timeout'))
    await draw({ onProtectedCopy: copy })
    expect(container.querySelector('[role="alert"]')?.textContent).toContain(en.timeout)
    expect(container.querySelector('[data-mocked-workbook]')).toBeNull()
    const retry = deferred<ExcelPreview>(); io.parse.mockImplementationOnce(() => retry.promise)
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
    expect((io.parse.mock.calls[0][3] as AbortSignal).aborted).toBe(true)
    expect(container.textContent).toContain('WAITING_CURRENT_WORKBOOK')
    expect(container.querySelector('[data-mocked-workbook]')).toBeNull(); expect(copy).not.toHaveBeenCalled()
    await resolveParse(retry)
    expect(workbookProps().protectedCopy!(selection)).toBe(true); expect(copy).toHaveBeenCalledOnce()
  })

  it('does not lend a retired Workbook callback to a new limits parse or a closed document', async () => {
    const copy = vi.fn(() => true)
    await draw({ onProtectedCopy: copy }); const retiredCopy = workbookProps().protectedCopy!
    const pending = deferred<ExcelPreview>()
    io.parse.mockImplementationOnce(() => pending.promise)
    await draw({ limits: { ...limits, maxCells: 125_000 }, onProtectedCopy: copy })
    expect(retiredCopy(selection)).toBe(false)
    await act(async () => root.render(null))
    expect(retiredCopy(selection)).toBe(false)
    expect((io.parse.mock.calls[1][3] as AbortSignal).aborted).toBe(true)
    await resolveParse(pending)
    expect(container.querySelector('[data-mocked-workbook]')).toBeNull(); expect(copy).not.toHaveBeenCalled()
  })
})


it('fails closed for a protected Present owner even when its Copy consumer is unavailable', async () => {
  await draw()
  expect(typeof workbookProps().protectedCopy).toBe('function')
  expect(workbookProps().protectedCopy!(selection)).toBe(false)
})

it('retains the hidden-sheet ordinal and hidden row/column metadata for a visible-sheet Copy', async () => {
  const parsed: ExcelPreview = { ...preview, sheets: [
    { id: 'hidden-first', name: 'Hidden', order: 0, hide: 1, status: 0,
      row: 8, column: 4, config: { rowhidden: { 2: 0 }, colhidden: { 1: 0 } }, celldata: [] },
    { ...preview.sheets[1], id: 'visible-second', name: 'Visible', order: 1, hide: 0, status: 1,
      config: { rowhidden: { 2: 0 }, colhidden: { 1: 0 } },
      celldata: [{ r: 0, c: 0, v: { v: 'VIEWER_ONLY', m: '<b>VIEWER_ONLY</b>', fc: '#123456', bg: '#abcdef' } }] }
  ] }
  io.parse.mockResolvedValueOnce(parsed)
  const copy = vi.fn((_index: number, _id: string, _ranges: Parameters<NonNullable<LoadedExcelBodyProps['onProtectedCopy']>>[2]) => true)
  await draw({ onProtectedCopy: copy })
  expect(workbookProps().data).toEqual(parsed.sheets)
  expect(workbookProps().data).toMatchObject([
    { id: 'hidden-first', hide: 1, status: 0 },
    { id: 'visible-second', hide: 0, status: 1,
      config: { rowhidden: { 2: 0 }, colhidden: { 1: 0 } } }
  ])
  const ranges: Parameters<ProtectedCopy>[0]['ranges'] = [{ row: [0, 3], column: [0, 2] }]
  expect(workbookProps().protectedCopy!({ sheetId: 'visible-second', ranges })).toBe(true)
  expect(copy).toHaveBeenCalledExactlyOnceWith(1, 'visible-second', [{ row: [0, 3], column: [0, 2] }])
  // Hidden sheets remain in the original parser list. Filtering them here
  // would turn this selector into index 0 and disagree with the Host parser.
})
