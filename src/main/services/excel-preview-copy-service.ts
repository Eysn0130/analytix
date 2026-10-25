import { spawn } from 'node:child_process'
import { dirname, isAbsolute, join, normalize } from 'node:path'
import { EXCEL_PREVIEW_LIMITS, type ExcelFormat } from '../../shared/excel/format'
import { toolResultLocalPresentExcelSelectionSchemaV1, type ToolResultLocalPresentExcelCopyRequestV1 } from '../../../packages/runtime/src/contracts/tool-result-local-display'
import { ownSpawnedProcess, stopOwnedProcess } from '../runtime/owned-process'

// Fixed sibling program, never a request-controlled executable or entry.
export const EXCEL_PREVIEW_COPY_CODEC_ENTRY_V1 = 'excel-preview-copy-codec-entry.js'
export const EXCEL_PREVIEW_COPY_MAX_OUTPUT_BYTES_V1 = 16 << 20
const MAX_STDERR_BYTES = 64 << 10
const CLOSE_AFTER_STOP_TIMEOUT_MS = 1_000
const MAX_INPUT_BYTES = Math.ceil(EXCEL_PREVIEW_LIMITS.maxBytes / 3) * 4 + 4096

export type ExcelCopySelectionV1 = ToolResultLocalPresentExcelCopyRequestV1['selection']
export type ExcelCopyCellV1 = {
  m?: string
  v?: string | number | boolean
  numberFormat?: string
  cellType?: 'd'
  mc?: { r: number; c: number; rs?: number; cs?: number }
}
export type ExcelCopyFrameV1 = {
  schemaVersion: 1
  op: 'excel-preview-copy-cells-v1'
  sheetIndex: number
  sheetId: string
  rowCount: number
  columnCount: number
  ranges: ExcelCopySelectionV1['ranges']
  // Mechanical sparse cell excerpt. No m/v preference, formula execution,
  // text joining, privacy projection or clipboard effect is owned here.
  cells: Array<[number, number, ExcelCopyCellV1]>
  // Produced only inside the fixed codec; Main checks grammar and literal identity.
  html?: string
}
export type ExcelPreviewCopyReaderV1 = (
  bytes: Uint8Array<ArrayBuffer>, format: ExcelFormat,
  selection: ExcelCopySelectionV1, signal: AbortSignal
) => Promise<ExcelCopyFrameV1>
type DocumentCodecLaunch = { executable: string; entry: string }
type Failure = 'invalid' | 'tooLarge' | 'timeout' | 'aborted' | 'termination-unconfirmed'

export class ExcelPreviewCopyCodecErrorV1 extends Error {
  constructor(readonly code: Failure) {
    super(code)
    this.name = 'ExcelPreviewCopyCodecErrorV1'
  }
}

function dataRecord(value: unknown, required: string[], optional: string[] = []): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    && required.every(key => Object.hasOwn(value, key))
    && Object.keys(value).every(key => required.includes(key) || optional.includes(key))
}
const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max

export function validateExcelCopyFrameV1(value: unknown, selection: ExcelCopySelectionV1): ExcelCopyFrameV1 {
  if (!toolResultLocalPresentExcelSelectionSchemaV1.safeParse(selection).success) throw new ExcelPreviewCopyCodecErrorV1('invalid')
  if (!dataRecord(value, ['schemaVersion', 'op', 'sheetIndex', 'sheetId', 'rowCount', 'columnCount', 'ranges', 'cells'], ['html'])
      || value.schemaVersion !== 1 || value.op !== 'excel-preview-copy-cells-v1'
      || value.sheetIndex !== selection.sheetIndex || value.sheetId !== selection.sheetId
      || !integer(value.rowCount, 1, EXCEL_PREVIEW_LIMITS.maxCells) || !integer(value.columnCount, 1, EXCEL_PREVIEW_LIMITS.maxCells)
      || value.rowCount * value.columnCount > EXCEL_PREVIEW_LIMITS.maxCells
      || JSON.stringify(value.ranges) !== JSON.stringify(selection.ranges)
      || selection.ranges.some(range => range.row[1] >= Number(value.rowCount) || range.column[1] >= Number(value.columnCount))
      || !Array.isArray(value.cells)) throw new ExcelPreviewCopyCodecErrorV1('invalid')
  if (Object.hasOwn(value, 'html') && (typeof value.html !== 'string' || value.html === ''
      || Buffer.byteLength(value.html, 'utf8') > EXCEL_PREVIEW_COPY_MAX_OUTPUT_BYTES_V1)) throw new ExcelPreviewCopyCodecErrorV1('invalid')
  const area = selection.ranges.reduce((total, range) => total + (range.row[1] - range.row[0] + 1) * (range.column[1] - range.column[0] + 1), 0)
  if (value.cells.length > area) throw new ExcelPreviewCopyCodecErrorV1('invalid')
  const positions = new Set<string>()
  const cells: ExcelCopyFrameV1['cells'] = []
  for (const tuple of value.cells) {
    if (!Array.isArray(tuple) || tuple.length !== 3
        || !integer(tuple[0], 0, value.rowCount - 1)
        || !integer(tuple[1], 0, value.columnCount - 1)
        || !selection.ranges.some(range => tuple[0] >= range.row[0] && tuple[0] <= range.row[1] && tuple[1] >= range.column[0] && tuple[1] <= range.column[1])
        || !dataRecord(tuple[2], [], ['m', 'v', 'mc', 'numberFormat', 'cellType'])) throw new ExcelPreviewCopyCodecErrorV1('invalid')
    const key = `${tuple[0]}:${tuple[1]}`
    if (positions.has(key)) throw new ExcelPreviewCopyCodecErrorV1('invalid')
    positions.add(key)
    const source = tuple[2], cell: ExcelCopyCellV1 = {}
    if (Object.hasOwn(source, 'm')) {
      if (typeof source.m !== 'string') throw new ExcelPreviewCopyCodecErrorV1('invalid')
      cell.m = source.m
    }
    if (Object.hasOwn(source, 'v')) {
      if (typeof source.v !== 'string' && typeof source.v !== 'boolean'
          && !(typeof source.v === 'number' && Number.isFinite(source.v))) throw new ExcelPreviewCopyCodecErrorV1('invalid')
      cell.v = source.v
    }
    if (Object.hasOwn(source, 'numberFormat')) {
      if (typeof source.numberFormat !== 'string' || Buffer.byteLength(source.numberFormat, 'utf8') > 4096) throw new ExcelPreviewCopyCodecErrorV1('invalid')
      cell.numberFormat = source.numberFormat
    }
    if (Object.hasOwn(source, 'cellType')) {
      if (source.cellType !== 'd') throw new ExcelPreviewCopyCodecErrorV1('invalid')
      cell.cellType = 'd'
    }
    if (Object.hasOwn(source, 'mc')) {
      const merge = source.mc
      if (!dataRecord(merge, ['r', 'c'], ['rs', 'cs'])
          || !integer(merge.r, 0, value.rowCount - 1) || !integer(merge.c, 0, value.columnCount - 1)
          || Object.hasOwn(merge, 'rs') !== Object.hasOwn(merge, 'cs')) throw new ExcelPreviewCopyCodecErrorV1('invalid')
      cell.mc = { r: merge.r, c: merge.c }
      if (Object.hasOwn(merge, 'rs')) {
        if (!integer(merge.rs, 1, value.rowCount - merge.r)
            || !integer(merge.cs, 1, value.columnCount - merge.c)) throw new ExcelPreviewCopyCodecErrorV1('invalid')
        cell.mc.rs = merge.rs
        cell.mc.cs = merge.cs
      }
    }
    cells.push([tuple[0], tuple[1], cell])
  }
  return { schemaVersion: 1, op: 'excel-preview-copy-cells-v1', sheetIndex: selection.sheetIndex,
    sheetId: selection.sheetId, rowCount: value.rowCount, columnCount: value.columnCount,
    ranges: selection.ranges.map(range => ({ row: [...range.row] as [number, number], column: [...range.column] as [number, number] })), cells, ...(typeof value.html === 'string' ? { html: value.html } : {}) }
}

/** The existing resolver is supplied by Main composition, avoiding a second Mac Helper resolver. */
export function createExcelPreviewCopyReaderV1(resolveGenerationLaunch: () => DocumentCodecLaunch): ExcelPreviewCopyReaderV1 {
  // This factory is composed once for Main and shared by all Host instances.
  // A reported timeout is not proof of EOF; only the captured child's close
  // releases the slot, including after an unconfirmed termination reply.
  let active = false
  return async (bytes, format, selection, signal) => {
    if (active) throw new ExcelPreviewCopyCodecErrorV1('invalid')
    if (signal.aborted) throw new ExcelPreviewCopyCodecErrorV1('aborted')
    if (bytes.byteLength > EXCEL_PREVIEW_LIMITS.maxBytes) throw new ExcelPreviewCopyCodecErrorV1('tooLarge')
    if (!toolResultLocalPresentExcelSelectionSchemaV1.safeParse(selection).success) throw new ExcelPreviewCopyCodecErrorV1('invalid')
    const capturedSelection: ExcelCopySelectionV1 = { sheetIndex: selection.sheetIndex, sheetId: selection.sheetId,
      ranges: selection.ranges.map(range => ({ row: [...range.row] as [number, number], column: [...range.column] as [number, number] })) }
    let launch: DocumentCodecLaunch
    try { launch = resolveGenerationLaunch() } catch { throw new ExcelPreviewCopyCodecErrorV1('invalid') }
    const entry = join(dirname(launch.entry), EXCEL_PREVIEW_COPY_CODEC_ENTRY_V1)
    if (!isAbsolute(launch.executable) || normalize(launch.executable) !== launch.executable
        || !isAbsolute(entry) || normalize(entry) !== entry) throw new ExcelPreviewCopyCodecErrorV1('invalid')
    const input = Buffer.from(JSON.stringify({ schemaVersion: 1, op: 'excel-preview-copy-cells-v1',
      format, selection: capturedSelection, dataBase64: Buffer.from(bytes).toString('base64') }))
    if (input.length > MAX_INPUT_BYTES) { input.fill(0); throw new ExcelPreviewCopyCodecErrorV1('tooLarge') }
    const env: NodeJS.ProcessEnv = { ELECTRON_RUN_AS_NODE: '1', LANG: 'C', TZ: 'UTC' }
    const detached = process.platform !== 'win32'
    let child: ReturnType<typeof spawn>
    active = true
    try {
      child = spawn(launch.executable, [entry], {
        cwd: dirname(entry), env, detached, shell: false, windowsHide: true,
        stdio: ['pipe', 'pipe', 'pipe']
      })
    } catch { active = false; input.fill(0); throw new ExcelPreviewCopyCodecErrorV1('invalid') }
    const owned = ownSpawnedProcess(child, { detached, env })
    let failure: Failure | null = null
    let resolveFailure!: () => void
    const failed = new Promise<void>(resolve => { resolveFailure = resolve })
    const requestFailure = (code: Failure): void => {
      if (failure !== null) return
      failure = code
      resolveFailure()
    }
    const abort = (): void => requestFailure('aborted')
    let stdoutBytes = 0, stderrBytes = 0, didClose = false, discarding = false
    const chunks: Buffer[] = []
    const onStdout = (chunk: Buffer | string): void => {
      if (discarding) return
      const size = Buffer.byteLength(chunk)
      stdoutBytes += size
      if (stdoutBytes > EXCEL_PREVIEW_COPY_MAX_OUTPUT_BYTES_V1) { requestFailure('tooLarge'); return }
      if (failure === null) chunks.push(Buffer.from(chunk))
    }
    const onStderr = (chunk: Buffer | string): void => {
      if (discarding) return
      stderrBytes += Buffer.byteLength(chunk)
      // Never retain or log parser diagnostics, including on failure.
      if (stderrBytes > MAX_STDERR_BYTES) requestFailure('tooLarge')
    }
    const onError = (): void => requestFailure('invalid')
    const closed = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>(resolve => {
      child.once('close', (code, exitSignal) => {
        didClose = true
        active = false
        resolve({ code, signal: exitSignal })
        if (discarding) removeListeners()
      })
    })
    const removeListeners = (): void => {
      child.stdout?.off('data', onStdout)
      child.stderr?.off('data', onStderr)
      child.stdout?.off('error', onError)
      child.stderr?.off('error', onError)
      child.off('error', onError)
      child.stdin?.off('error', onError)
    }
    child.stdout?.on('data', onStdout)
    child.stderr?.on('data', onStderr)
    child.stdout?.on('error', onError)
    child.stderr?.on('error', onError)
    child.once('error', onError)
    child.stdin?.on('error', onError)
    const timer = setTimeout(() => requestFailure('timeout'), EXCEL_PREVIEW_LIMITS.timeoutMs)
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) abort()
    try {
      if (!child.stdin || !child.stdout || !child.stderr) requestFailure('invalid')
      else child.stdin.end(input)
      await Promise.race([closed, failed])
      if (failure !== null) {
        child.stdin?.destroy()
        try { await stopOwnedProcess(owned, { graceMs: 0, killWaitMs: 1_000 }) }
        catch { throw new ExcelPreviewCopyCodecErrorV1('termination-unconfirmed') }
      }
      // `exit` alone does not establish stdout/stderr EOF. An already-exited
      // child can still have an unclosed inherited pipe: bound that separate
      // wait after owned termination rather than leaving the Copy pending.
      let close: Awaited<typeof closed>
      if (failure !== null) {
        let closeTimer: ReturnType<typeof setTimeout> | undefined
        try {
          close = await Promise.race([closed, new Promise<never>((_, reject) => {
            closeTimer = setTimeout(() => reject(new ExcelPreviewCopyCodecErrorV1('termination-unconfirmed')), CLOSE_AFTER_STOP_TIMEOUT_MS)
          })])
        } finally { if (closeTimer) clearTimeout(closeTimer) }
      } else close = await closed
      const exit = await owned.exit
      if (failure !== null) throw new ExcelPreviewCopyCodecErrorV1(failure)
      if (signal.aborted) throw new ExcelPreviewCopyCodecErrorV1('aborted')
      if (exit.spawnError !== null || exit.code !== 0 || exit.signal !== null
          || close.code !== 0 || close.signal !== null || stderrBytes !== 0) throw new ExcelPreviewCopyCodecErrorV1('invalid')
      const output = Buffer.concat(chunks, stdoutBytes)
      try {
        const text = new TextDecoder('utf-8', { fatal: true }).decode(output)
        return validateExcelCopyFrameV1(JSON.parse(text), capturedSelection)
      } catch (error) {
        if (error instanceof ExcelPreviewCopyCodecErrorV1) throw error
        throw new ExcelPreviewCopyCodecErrorV1('invalid')
      } finally { output.fill(0) }
    } finally {
      clearTimeout(timer)
      signal.removeEventListener('abort', abort)
      input.fill(0)
      for (const chunk of chunks) chunk.fill(0)
      chunks.length = 0
      discarding = true
      // An exceptional unconfirmed termination is fail-closed and not reported
      // as reaped. Keep draining until the captured child reports close; no unref.
      if (didClose) removeListeners()
    }
  }
}
