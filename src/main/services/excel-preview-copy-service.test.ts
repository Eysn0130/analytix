import { EventEmitter } from 'node:events'
import { PassThrough } from 'node:stream'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ spawn: vi.fn(), own: vi.fn(), stop: vi.fn() }))
vi.mock('node:child_process', () => ({ spawn: mocks.spawn }))
vi.mock('../runtime/owned-process', () => ({ ownSpawnedProcess: mocks.own, stopOwnedProcess: mocks.stop }))

import { createExcelPreviewCopyReaderV1, validateExcelCopyFrameV1, type ExcelCopyFrameV1, type ExcelCopySelectionV1 } from './excel-preview-copy-service'

class Child extends EventEmitter {
  stdin = new PassThrough()
  stdout = new PassThrough()
  stderr = new PassThrough()
  pid = 1234
  exitCode: number | null = null
  signalCode: NodeJS.Signals | null = null
  resolveExit!: (value: { code: number | null; signal: NodeJS.Signals | null; spawnError: Error | null }) => void
  exit = new Promise<{ code: number | null; signal: NodeJS.Signals | null; spawnError: Error | null }>(resolve => { this.resolveExit = resolve })
  finishExit(code: number | null = 0, signal: NodeJS.Signals | null = null): void {
    this.exitCode = code; this.signalCode = signal
    this.resolveExit({ code, signal, spawnError: null })
    this.emit('exit', code, signal)
  }
  finishClose(): void { this.emit('close', this.exitCode, this.signalCode) }
}
const selection = (): ExcelCopySelectionV1 => ({ sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 0], column: [0, 0] }] })
const frame = (): ExcelCopyFrameV1 => ({ schemaVersion: 1, op: 'excel-preview-copy-cells-v1', sheetIndex: 0, sheetId: '1',
  rowCount: 1, columnCount: 1, ranges: [{ row: [0, 0], column: [0, 0] }], cells: [[0, 0, { m: 'saved display', v: 'stored value' }]] })
const bytes = Uint8Array.from(Buffer.from('synthetic csv'))
const launch = { executable: '/synthetic/Analytix Helper', entry: '/synthetic/out/office-codec/office-generation-codec-entry.js' }

beforeEach(() => {
  mocks.spawn.mockReset(); mocks.own.mockReset(); mocks.stop.mockReset()
  mocks.own.mockImplementation((child: Child) => ({ process: child, pid: child.pid, processGroupId: child.pid,
    platform: process.platform, exit: child.exit }))
})
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

describe('fixed data-only Excel Copy child lifetime', () => {
  it('reuses the existing resolver executable, fixes the second entry, and never inherits process environment', async () => {
    const child = new Child(), resolve = vi.fn(() => launch)
    mocks.spawn.mockReturnValue(child)
    const read = createExcelPreviewCopyReaderV1(resolve)
    const pending = read(bytes, 'csv', selection(), new AbortController().signal)
    expect(mocks.spawn).toHaveBeenCalledExactlyOnceWith(launch.executable,
      ['/synthetic/out/office-codec/excel-preview-copy-codec-entry.js'], {
        cwd: '/synthetic/out/office-codec', env: { ELECTRON_RUN_AS_NODE: '1', LANG: 'C', TZ: 'UTC' },
        detached: process.platform !== 'win32', shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe']
      })
    expect(resolve).toHaveBeenCalledOnce()
    const input = JSON.parse(child.stdin.read().toString())
    expect(input).toEqual({ schemaVersion: 1, op: 'excel-preview-copy-cells-v1', format: 'csv', selection: selection(),
      dataBase64: Buffer.from(bytes).toString('base64') })
    child.stdout.write(JSON.stringify(frame()))
    child.finishExit(); child.finishClose()
    await expect(pending).resolves.toEqual(frame())
  })

  it('does not accept success at exit before pipe close and fences overlapping Copy across Hosts', async () => {
    const child = new Child()
    mocks.spawn.mockReturnValue(child)
    const read = createExcelPreviewCopyReaderV1(() => launch)
    let settled = false
    const pending = read(bytes, 'csv', selection(), new AbortController().signal).finally(() => { settled = true })
    child.stdout.write(JSON.stringify(frame()))
    child.finishExit()
    await Promise.resolve()
    expect(settled).toBe(false)
    await expect(read(bytes, 'csv', selection(), new AbortController().signal)).rejects.toMatchObject({ code: 'invalid' })
    expect(mocks.spawn).toHaveBeenCalledOnce()
    child.finishClose()
    await expect(pending).resolves.toEqual(frame())
    const next = new Child(); mocks.spawn.mockReturnValue(next)
    const successor = read(bytes, 'csv', selection(), new AbortController().signal)
    next.stdout.write(JSON.stringify(frame())); next.finishExit(); next.finishClose()
    await expect(successor).resolves.toEqual(frame())
    expect(mocks.spawn).toHaveBeenCalledTimes(2)
  })

  it('clones the original sheet/ranges identity before caller mutation', async () => {
    const child = new Child(), original = selection()
    mocks.spawn.mockReturnValue(child)
    const pending = createExcelPreviewCopyReaderV1(() => launch)(bytes, 'csv', original, new AbortController().signal)
    original.sheetId = 'mutated'; original.ranges[0].row[0] = 1; original.ranges[0].column[0] = 1
    original.ranges.push({ row: [2, 2], column: [0, 0] })
    child.stdout.write(JSON.stringify(frame())); child.finishExit(); child.finishClose()
    await expect(pending).resolves.toEqual(frame())
  })

  it('aborts through owned termination and still waits for close after observed exit', async () => {
    const child = new Child(), controller = new AbortController()
    mocks.spawn.mockReturnValue(child)
    mocks.stop.mockImplementation(async () => { child.finishExit(null, 'SIGTERM'); return {} })
    let settled = false
    const pending = createExcelPreviewCopyReaderV1(() => launch)(bytes, 'csv', selection(), controller.signal)
    const outcome = pending.catch(error => error).finally(() => { settled = true })
    controller.abort()
    await vi.waitFor(() => expect(mocks.stop).toHaveBeenCalledOnce())
    expect(settled).toBe(false)
    child.finishClose()
    expect(await outcome).toMatchObject({ code: 'aborted' })
  })

  it('bounds missing EOF after an already-exited child and retains the slot until real late close', async () => {
    vi.useFakeTimers()
    const child = new Child()
    mocks.spawn.mockReturnValue(child)
    mocks.stop.mockResolvedValue({ termination: 'already_exited' })
    const read = createExcelPreviewCopyReaderV1(() => launch)
    const outcome = read(bytes, 'csv', selection(), new AbortController().signal).catch(error => error)
    child.stdout.write(JSON.stringify(frame())); child.finishExit()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.stop).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1_000)
    expect(await outcome).toMatchObject({ code: 'termination-unconfirmed' })
    expect(child.stdout.listenerCount('data')).toBe(1)
    await expect(read(bytes, 'csv', selection(), new AbortController().signal)).rejects.toMatchObject({ code: 'invalid' })
    expect(mocks.spawn).toHaveBeenCalledOnce()
    child.stdout.write('discarded late bytes')
    child.finishClose()
    expect(child.stdout.listenerCount('data')).toBe(0)
    const next = new Child(); mocks.spawn.mockReturnValue(next)
    const successor = read(bytes, 'csv', selection(), new AbortController().signal)
    next.stdout.write(JSON.stringify(frame())); next.finishExit(); next.finishClose()
    await expect(successor).resolves.toEqual(frame())
    expect(mocks.spawn).toHaveBeenCalledTimes(2)
  })

  it.each(['timeout', 'outputLimit'] as const)('reaps the child before replying to %s', async cause => {
    vi.useFakeTimers()
    const child = new Child()
    mocks.spawn.mockReturnValue(child)
    mocks.stop.mockImplementation(async () => { child.finishExit(null, 'SIGKILL'); child.finishClose(); return {} })
    const outcome = createExcelPreviewCopyReaderV1(() => launch)(bytes, 'csv', selection(), new AbortController().signal).catch(error => error)
    if (cause === 'timeout') await vi.advanceTimersByTimeAsync(15_000)
    else { child.stdout.write(Buffer.alloc((16 << 20) + 1)); await vi.advanceTimersByTimeAsync(0) }
    expect(await outcome).toMatchObject({ code: cause === 'timeout' ? 'timeout' : 'tooLarge' })
    expect(mocks.stop).toHaveBeenCalledOnce()
    expect(child.stdout.listenerCount('data')).toBe(0)
  })

  it('sanitizes unconfirmed stop errors and drains until close without releasing the child slot', async () => {
    const child = new Child(), controller = new AbortController()
    mocks.spawn.mockReturnValue(child)
    mocks.stop.mockRejectedValue(new Error('synthetic private diagnostics'))
    const read = createExcelPreviewCopyReaderV1(() => launch)
    const outcome = read(bytes, 'csv', selection(), controller.signal).catch(error => error)
    controller.abort()
    const error = await outcome
    expect(error).toMatchObject({ code: 'termination-unconfirmed', message: 'termination-unconfirmed' })
    expect(String(error)).not.toContain('private diagnostics')
    await expect(read(bytes, 'csv', selection(), new AbortController().signal)).rejects.toMatchObject({ code: 'invalid' })
    expect(mocks.spawn).toHaveBeenCalledOnce()
    child.finishExit(null, 'SIGKILL'); child.finishClose()
    expect(child.stdout.listenerCount('data')).toBe(0)
  })

  it('releases the slot after synchronous spawn failure', async () => {
    mocks.spawn.mockImplementationOnce(() => { throw new Error('synthetic private path') })
    const read = createExcelPreviewCopyReaderV1(() => launch)
    await expect(read(bytes, 'csv', selection(), new AbortController().signal)).rejects.toMatchObject({ code: 'invalid', message: 'invalid' })
    const child = new Child(); mocks.spawn.mockReturnValue(child)
    const pending = read(bytes, 'csv', selection(), new AbortController().signal)
    child.stdout.write(JSON.stringify(frame())); child.finishExit(); child.finishClose()
    await expect(pending).resolves.toEqual(frame())
  })
})


describe('Excel Copy mechanical ranges and cell metadata', () => {
  it.each(['horizontal', 'vertical'] as const)('accepts disjoint %s ranges in their original order and clones their tuples', direction => {
    const ranges: ExcelCopySelectionV1['ranges'] = direction === 'horizontal'
      ? [{ row: [0, 1], column: [4, 5] }, { row: [0, 1], column: [0, 1] }]
      : [{ row: [4, 5], column: [0, 1] }, { row: [0, 1], column: [0, 1] }]
    const original: ExcelCopySelectionV1 = { ...selection(), ranges }
    const source: ExcelCopyFrameV1 = { ...frame(), rowCount: 6, columnCount: 6, ranges,
      cells: [[direction === 'horizontal' ? 1 : 5, direction === 'horizontal' ? 5 : 1, { m: 'first range' }], [0, 0, { v: 'last range' }]] }
    const parsed = validateExcelCopyFrameV1(source, original)
    expect(parsed).toEqual(source)
    expect(parsed.ranges).not.toBe(ranges)
    for (const [index, range] of ranges.entries()) {
      expect(parsed.ranges[index]).not.toBe(range)
      expect(parsed.ranges[index].row).not.toBe(range.row)
      expect(parsed.ranges[index].column).not.toBe(range.column)
    }
    const expected = ranges.map(range => ({ row: [...range.row], column: [...range.column] }))
    ranges[0].row[0] = 99; ranges[0].column[0] = 99; ranges.reverse()
    expect(parsed.ranges).toEqual(expected)
  })

  it('accepts 32 non-overlapping ranges without filling the coordinate gaps', () => {
    const ranges = Array.from({ length: 32 }, (_, index): ExcelCopySelectionV1['ranges'][number] =>
      ({ row: [0, 0], column: [index * 2, index * 2] }))
    const original = { ...selection(), ranges }
    const source = { ...frame(), ranges, rowCount: 1, columnCount: 63, cells: [] }
    expect(validateExcelCopyFrameV1(source, original)).toEqual(source)
  })

  it('accepts the 250000 selected Cartesian cell boundary in caller range order', () => {
    const ranges: ExcelCopySelectionV1['ranges'] = [
      { row: [0, 999], column: [125, 249] }, { row: [0, 999], column: [0, 124] }
    ]
    const original = { ...selection(), ranges }
    const source = { ...frame(), ranges, rowCount: 1000, columnCount: 250, cells: [] }
    expect(validateExcelCopyFrameV1(source, original)).toEqual(source)
  })

  it.each(['row', 'column'] as const)('accepts coordinate 249999 on the %s axis', axis => {
    const ranges: ExcelCopySelectionV1['ranges'] = [{
      row: axis === 'row' ? [249999, 249999] : [0, 0],
      column: axis === 'column' ? [249999, 249999] : [0, 0]
    }]
    const original = { ...selection(), ranges }
    const source = { ...frame(), ranges, rowCount: axis === 'row' ? 250000 : 1,
      columnCount: axis === 'column' ? 250000 : 1, cells: [[ranges[0].row[0], ranges[0].column[0], { v: 0 }]] }
    expect(validateExcelCopyFrameV1(source, original)).toEqual(source)
  })

  it.each([
    { name: 'no ranges', ranges: [], rowCount: 1, columnCount: 1 },
    { name: '33 ranges', ranges: Array.from({ length: 33 }, (_, index) => ({ row: [0, 0], column: [index * 2, index * 2] })), rowCount: 1, columnCount: 65 },
    { name: 'negative row', ranges: [{ row: [-1, 0], column: [0, 0] }], rowCount: 1, columnCount: 1 },
    { name: 'fractional column', ranges: [{ row: [0, 0], column: [0, 0.5] }], rowCount: 1, columnCount: 1 },
    { name: 'coordinate above 249999', ranges: [{ row: [0, 250000], column: [0, 0] }], rowCount: 250001, columnCount: 1 },
    { name: 'reversed row interval', ranges: [{ row: [1, 0], column: [0, 0] }], rowCount: 2, columnCount: 1 },
    { name: 'reversed column interval', ranges: [{ row: [0, 0], column: [1, 0] }], rowCount: 1, columnCount: 2 },
    { name: 'short tuple', ranges: [{ row: [0], column: [0, 0] }], rowCount: 1, columnCount: 1 },
    { name: 'long tuple', ranges: [{ row: [0, 0, 0], column: [0, 0] }], rowCount: 1, columnCount: 1 },
    { name: 'unaligned rectangles', ranges: [{ row: [0, 0], column: [0, 0] }, { row: [1, 1], column: [1, 1] }], rowCount: 2, columnCount: 2 },
    { name: 'duplicate rectangle', ranges: [{ row: [0, 0], column: [0, 0] }, { row: [0, 0], column: [0, 0] }], rowCount: 1, columnCount: 1 },
    { name: 'horizontal closed-interval overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [0, 1], column: [1, 2] }], rowCount: 2, columnCount: 3 },
    { name: 'vertical closed-interval overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [1, 2], column: [0, 1] }], rowCount: 3, columnCount: 2 },
    { name: 'selected Cartesian area above 250000', ranges: [{ row: [0, 999], column: [0, 249] }, { row: [0, 999], column: [250, 250] }], rowCount: 1000, columnCount: 251 }
  ])('rejects matching malformed frame/request $name', ({ ranges, rowCount, columnCount }) => {
    const source = { ...frame(), ranges, rowCount, columnCount, cells: [] }
    const original = { ...selection(), ranges }
    expect(() => validateExcelCopyFrameV1(source, original as never)).toThrowError('invalid')
  })

  it('rejects frame ranges that differ from request coordinates, count or order', () => {
    const original: ExcelCopySelectionV1 = { ...selection(), ranges: [
      { row: [0, 0], column: [2, 2] }, { row: [0, 0], column: [0, 0] }
    ] }
    const mismatches: ExcelCopySelectionV1['ranges'][] = [
      [{ row: [0, 0], column: [1, 1] }, { row: [0, 0], column: [0, 0] }],
      [{ row: [0, 0], column: [2, 2] }],
      [...original.ranges].reverse()
    ]
    for (const ranges of mismatches) {
      expect(() => validateExcelCopyFrameV1({ ...frame(), rowCount: 1, columnCount: 3, ranges, cells: [] }, original)).toThrowError('invalid')
    }
  })

  it('rejects a cell in a coordinate gap between otherwise valid ranges', () => {
    const original: ExcelCopySelectionV1 = { ...selection(), ranges: [
      { row: [0, 0], column: [2, 2] }, { row: [0, 0], column: [0, 0] }
    ] }
    const source = { ...frame(), rowCount: 1, columnCount: 3, ranges: original.ranges, cells: [[0, 1, { m: 'gap' }]] }
    expect(() => validateExcelCopyFrameV1(source, original)).toThrowError('invalid')
  })

  it('keeps numberFormat and cellType optional without synthesizing metadata', () => {
    const parsed = validateExcelCopyFrameV1(frame(), selection())
    expect(parsed).toEqual(frame())
    expect(parsed.cells[0][2]).not.toHaveProperty('numberFormat')
    expect(parsed.cells[0][2]).not.toHaveProperty('cellType')
  })

  it.each([
    { name: 'ordinary format', numberFormat: '0.00' },
    { name: '4096 ASCII bytes', numberFormat: 'a'.repeat(4096) },
    { name: '1024 four-byte emoji', numberFormat: '🙂'.repeat(1024) }
  ])('accepts $name with the neutral date type and clones the cell', ({ numberFormat }) => {
    const original = frame(), cell = { ...original.cells[0][2], numberFormat, cellType: 'd' as const }
    original.cells[0][2] = cell
    const parsed = validateExcelCopyFrameV1(original, selection())
    expect(parsed).toEqual(original)
    expect(parsed.cells[0][2]).not.toBe(cell)
    cell.numberFormat = 'changed'; cell.m = 'changed'; original.ranges[0].row[0] = 1
    expect(parsed.cells[0][2]).toEqual({ m: 'saved display', v: 'stored value', numberFormat, cellType: 'd' })
    expect(parsed.ranges).toEqual([{ row: [0, 0], column: [0, 0] }])
  })

  it.each([
    { name: 'numberFormat alone', metadata: { numberFormat: '0.00' } },
    { name: 'date cellType alone', metadata: { cellType: 'd' as const } }
  ])('accepts independently optional $name', ({ metadata }) => {
    const original = frame(), cell = { ...original.cells[0][2], ...metadata }
    original.cells[0][2] = cell
    const parsed = validateExcelCopyFrameV1(original, selection())
    expect(parsed).toEqual(original)
    expect(parsed.cells[0][2]).not.toBe(cell)
  })

  it.each([
    { name: 'null numberFormat', metadata: { numberFormat: null } },
    { name: 'numeric numberFormat', metadata: { numberFormat: 12 } },
    { name: 'object numberFormat', metadata: { numberFormat: { fa: '0.00' } } },
    { name: '4097 ASCII format bytes', metadata: { numberFormat: 'a'.repeat(4097) } },
    { name: '1025 four-byte format emoji', metadata: { numberFormat: '🙂'.repeat(1025) } },
    { name: 'null cellType', metadata: { cellType: null } },
    { name: 'numeric cellType', metadata: { cellType: 12 } },
    { name: 'object cellType', metadata: { cellType: { t: 'd' } } },
    { name: 'unsupported cellType', metadata: { cellType: 'n' } },
    { name: 'empty cellType', metadata: { cellType: '' } },
    { name: 'raw format metadata', metadata: { ct: { fa: '0.00', t: 'd' } } },
    { name: 'formula metadata', metadata: { f: 'A1' } },
    { name: 'computed text metadata', metadata: { text: 'prejoined text' } }
  ])('rejects $name at the mechanical cell boundary', ({ metadata }) => {
    const original = frame()
    const invalid = { ...original, cells: [[0, 0, { ...original.cells[0][2], ...metadata }]] }
    expect(() => validateExcelCopyFrameV1(invalid, selection())).toThrowError('invalid')
  })

  it.each([{ text: 'prejoined text' }, { numberFormat: '0.00' }, { path: '/synthetic/not-an-authority' }])(
    'rejects frame-level metadata %j', metadata => {
      expect(() => validateExcelCopyFrameV1({ ...frame(), ...metadata }, selection())).toThrowError('invalid')
    }
  )

  it.each([
    { name: 'object numberFormat', metadata: { numberFormat: { fa: '0.00' } } },
    { name: 'null cellType', metadata: { cellType: null } },
    { name: 'raw format metadata', metadata: { ct: { fa: '0.00' } } }
  ])('rejects child $name after complete exit and pipe close', async ({ metadata }) => {
    const child = new Child(), original = frame()
    const malformed = { ...original, cells: [[0, 0, { ...original.cells[0][2], ...metadata }]] }
    mocks.spawn.mockReturnValue(child)
    const outcome = createExcelPreviewCopyReaderV1(() => launch)(bytes, 'csv', selection(), new AbortController().signal).catch(error => error)
    child.stdout.write(JSON.stringify(malformed))
    child.finishExit(); child.finishClose()
    expect(await outcome).toMatchObject({ code: 'invalid', message: 'invalid' })
    expect(mocks.stop).not.toHaveBeenCalled()
    expect(child.stdout.listenerCount('data')).toBe(0)
  })
})


describe('Excel private child HTML field remains bounded and data-only', () => {
  it('clones only its bounded HTML string alongside validated selected cells', () => {
    const value = { ...frame(), html: '<table data-type="fortune-copy-action-table"></table>' }
    expect(validateExcelCopyFrameV1(value, selection())).toEqual(value)
  })
  it.each([123, null, '', 'x'.repeat((16 << 20) + 1)])('rejects invalid or oversized private HTML output', html => {
    expect(() => validateExcelCopyFrameV1({ ...frame(), html }, selection())).toThrow('invalid')
  })
  it.each([{ text: 'prejoined text' }, { checkedHtml: '<table>forged authority</table>' }])('rejects extra child authority fields', extra => {
    expect(() => validateExcelCopyFrameV1({ ...frame(), ...extra }, selection())).toThrow('invalid')
  })
})
