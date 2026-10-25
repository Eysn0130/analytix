import { convertExcel } from '../../shared/excel/convert'
import { formatExcelCopyHtmlV1 } from './excel-preview-copy-html'
import { excelFormat, EXCEL_PREVIEW_LIMITS } from '../../shared/excel/format'
import { toolResultLocalPresentExcelSelectionSchemaV1 } from '../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ExcelCopyCellV1, ExcelCopyFrameV1 } from '../services/excel-preview-copy-service'

// This is a fixed standalone second entry. It is never a generation protocol
// operation and receives no paths, credentials, executable code or URLs.
const MAX_BYTES = EXCEL_PREVIEW_LIMITS.maxBytes
const MAX_INPUT_BYTES = Math.ceil(MAX_BYTES / 3) * 4 + 4096
const MAX_OUTPUT_BYTES = 16 << 20
const LIMITS = EXCEL_PREVIEW_LIMITS
const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max

async function main(): Promise<void> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const incoming of process.stdin) {
    const chunk = Buffer.isBuffer(incoming) ? incoming : Buffer.from(incoming)
    size += chunk.length
    if (size > MAX_INPUT_BYTES) throw new Error('invalid')
    chunks.push(chunk)
  }
  const input = Buffer.concat(chunks, size)
  let bytes: Buffer | undefined
  try {
    const value: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(input))
    if (value === null || typeof value !== 'object' || Array.isArray(value)
        || Object.keys(value).length !== 5
        || !['schemaVersion', 'op', 'format', 'selection', 'dataBase64'].every(key => Object.hasOwn(value, key))) throw new Error('invalid')
    const request = value as Record<string, unknown>
    if (request.schemaVersion !== 1 || request.op !== 'excel-preview-copy-cells-v1'
        || !['xlsx', 'xls', 'csv', 'tsv'].includes(String(request.format))
        || typeof request.dataBase64 !== 'string'
        || request.dataBase64.length > Math.ceil(MAX_BYTES / 3) * 4) throw new Error('invalid')
    const selection = toolResultLocalPresentExcelSelectionSchemaV1.parse(request.selection)
    bytes = Buffer.from(request.dataBase64, 'base64')
    if (bytes.length > MAX_BYTES || bytes.toString('base64') !== request.dataBase64) throw new Error('invalid')
    // Format is supplied by the Host's sealed suffix. This fixed program has
    // no file/network access call and never evaluates formulas/external links.
    const format = excelFormat(`file.${String(request.format)}`)
    const data = Uint8Array.from(bytes)
    let preview: Awaited<ReturnType<typeof convertExcel>>
    try { preview = await convertExcel(data, format, LIMITS) }
    finally { data.fill(0) }
    const sheet = preview.sheets[selection.sheetIndex]
    if (!sheet || String(sheet.id) !== selection.sheetId || sheet.hide !== 0
        || !integer(sheet.row, 1, LIMITS.maxCells) || !integer(sheet.column, 1, LIMITS.maxCells)
        || sheet.row * sheet.column > LIMITS.maxCells
        || selection.ranges.some(range => range.row[1] >= Number(sheet.row) || range.column[1] >= Number(sheet.column))) throw new Error('invalid')
    const cells: ExcelCopyFrameV1['cells'] = []
    for (const item of sheet.celldata ?? []) {
      if (!selection.ranges.some(range => item.r >= range.row[0] && item.r <= range.row[1] && item.c >= range.column[0] && item.c <= range.column[1])) continue
      if (!integer(item.r, 0, sheet.row - 1) || !integer(item.c, 0, sheet.column - 1)
          || !item.v || typeof item.v !== 'object') throw new Error('invalid')
      const cell: ExcelCopyCellV1 = {}
      if (item.v.m !== undefined) {
        if (typeof item.v.m !== 'string') throw new Error('invalid')
        cell.m = item.v.m
      }
      if (item.v.v !== undefined) {
        const raw: unknown = item.v.v
        if (typeof raw !== 'string' && typeof raw !== 'boolean'
            && !(typeof raw === 'number' && Number.isFinite(raw))) throw new Error('invalid')
        cell.v = raw
      }
      if (item.v.ct?.fa !== undefined) {
        if (typeof item.v.ct.fa !== 'string' || Buffer.byteLength(item.v.ct.fa, 'utf8') > 4096) throw new Error('invalid')
        cell.numberFormat = item.v.ct.fa
      }
      if (item.v.ct?.t === 'd') cell.cellType = 'd'
      if (item.v.mc !== undefined) {
        const merge = item.v.mc
        if (!integer(merge.r, 0, sheet.row - 1) || !integer(merge.c, 0, sheet.column - 1)
            || (merge.rs !== undefined) !== (merge.cs !== undefined)) throw new Error('invalid')
        cell.mc = { r: merge.r, c: merge.c }
        if (merge.rs !== undefined) {
          if (!integer(merge.rs, 1, sheet.row - merge.r) || !integer(merge.cs, 1, sheet.column - merge.c)) throw new Error('invalid')
          cell.mc.rs = merge.rs
          cell.mc.cs = merge.cs
        }
      }
      cells.push([item.r, item.c, cell])
    }
    // CF styles require evaluation; this fixed Copy never executes such rules.
    if (preview.unsupportedFeatures.includes('conditionalFormatting')) throw new Error('invalid')
    const html = formatExcelCopyHtmlV1(sheet, selection)
    const frame: ExcelCopyFrameV1 = { schemaVersion: 1, op: 'excel-preview-copy-cells-v1',
      sheetIndex: selection.sheetIndex, sheetId: String(sheet.id), rowCount: sheet.row, columnCount: sheet.column,
      ranges: selection.ranges.map(range => ({ row: [...range.row] as [number, number], column: [...range.column] as [number, number] })), cells, html }
    const output = Buffer.from(JSON.stringify(frame))
    if (output.length > MAX_OUTPUT_BYTES) { output.fill(0); throw new Error('invalid') }
    try {
      await new Promise<void>((resolve, reject) => process.stdout.write(output, error => error ? reject(new Error('invalid')) : resolve()))
    } finally { output.fill(0) }
  } finally {
    input.fill(0)
    for (const chunk of chunks) chunk.fill(0)
    bytes?.fill(0)
  }
}

// Parser diagnostics and input bodies never leave through logs or stderr.
void main().catch(() => { process.exitCode = 1 })
