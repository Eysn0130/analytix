import { getCellValue, hasPartMC, rangeValueToHtml, type Cell, type CellMatrix, type Context, type Sheet, type SheetConfig } from '@fortune-sheet/core'
import { toolResultLocalPresentExcelSelectionSchemaV1 } from '../../../packages/runtime/src/contracts/tool-result-local-display'
import { ExcelPreviewError } from '../../shared/excel/error'
import { EXCEL_PREVIEW_LIMITS, EXCEL_PREVIEW_MAX_OUTPUT_BYTES } from '../../shared/excel/format'
import { projectOrdinaryToolExportTextV1 } from '../services/write-export-content'
import type { ExcelCopySelectionV1 as ExcelSelection } from '../services/excel-preview-copy-service'

// Calls the installed single Fortune formatter inside the fixed codec. This
// formatter; it does not render HTML, calculate formulas, or write a clipboard.
// projectOrdinaryToolExportTextV1 is the exact existing Host SGR/control wrapper
// moved/exported by root, not an additional protection policy.
const fail = (): never => { throw new ExcelPreviewError('invalid') }
const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max
const finite = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0
const record = (value: unknown): Record<string, unknown> => {
  if (value === null || typeof value !== 'object' || Array.isArray(value)
      || ![Object.prototype, null].includes(Object.getPrototypeOf(value))) return fail()
  return value as Record<string, unknown>
}
const color = (value: unknown): string => typeof value === 'string' && /^#[0-9a-f]{6}$/iu.test(value) ? value : fail()

function indexMap(value: unknown, size: number): Record<string, number> {
  const output: Record<string, number> = {}
  for (const [key, item] of Object.entries(record(value ?? {}))) {
    if (!/^(0|[1-9][0-9]*)$/.test(key) || !integer(Number(key), 0, size - 1) || !finite(item) || item > 8192) fail()
    output[key] = item as number
  }
  return output
}

function mergeCell(value: unknown, rows: number, columns: number): NonNullable<Cell['mc']> {
  const item = record(value)
  if (!integer(item.r, 0, rows - 1) || !integer(item.c, 0, columns - 1)
      || (item.rs !== undefined) !== (item.cs !== undefined)) return fail()
  const result: NonNullable<Cell['mc']> = { r: item.r, c: item.c }
  if (item.rs !== undefined) {
    if (!integer(item.rs, 1, rows - item.r) || !integer(item.cs, 1, columns - item.c)) return fail()
    result.rs = item.rs
    result.cs = item.cs
  }
  return result
}

function configData(value: unknown, rows: number, columns: number): SheetConfig {
  const source = record(value ?? {})
  const merge: NonNullable<SheetConfig['merge']> = {}
  for (const [key, value] of Object.entries(record(source.merge ?? {}))) {
    const item = mergeCell(value, rows, columns)
    if (item.rs === undefined || item.cs === undefined || key !== `${item.r}_${item.c}`) fail()
    merge[key] = { r: item.r, c: item.c, rs: item.rs!, cs: item.cs! }
  }
  const borderInfo: NonNullable<SheetConfig['borderInfo']> = []
  if (source.borderInfo !== undefined) {
    if (!Array.isArray(source.borderInfo) || source.borderInfo.length > EXCEL_PREVIEW_LIMITS.maxCells) fail()
    for (const raw of source.borderInfo as unknown[]) {
      const entry = record(raw), border = record(entry.value)
      // The same converter emits only these cell border records. Do not admit
      // renderer-owned range rules, custom CSS, or additional style machinery.
      if (entry.rangeType !== 'cell' || !integer(border.row_index, 0, rows - 1)
          || !integer(border.col_index, 0, columns - 1)) fail()
      const value: Record<string, unknown> = { row_index: border.row_index, col_index: border.col_index }
      for (const side of ['l', 'r', 't', 'b'] as const) if (border[side] !== undefined) {
        const edge = record(border[side])
        if (!integer(edge.style, 0, 13)) fail()
        value[side] = { style: edge.style, color: color(edge.color) }
      }
      borderInfo.push({ rangeType: 'cell', value })
    }
  }
  return { merge, borderInfo, rowlen: indexMap(source.rowlen, rows), columnlen: indexMap(source.columnlen, columns),
    rowhidden: indexMap(source.rowhidden, rows), colhidden: indexMap(source.colhidden, columns) }
}

function cellData(value: unknown, rows: number, columns: number, selected: boolean): Cell {
  const source = record(value)
  const cell: Cell = {}
  if (source.mc !== undefined) cell.mc = mergeCell(source.mc, rows, columns)
  // Border computation can inspect adjacent merge geometry. It never needs
  // their saved values, formulas, rich runs, links, comments, or styles.
  if (!selected) return cell
  if (source.m !== undefined) {
    if (typeof source.m !== 'string') fail()
    cell.m = source.m as string
  }
  if (source.v !== undefined) {
    if (typeof source.v !== 'string' && typeof source.v !== 'boolean'
        && !(typeof source.v === 'number' && Number.isFinite(source.v))) fail()
    cell.v = source.v as NonNullable<Cell['v']>
  }
  if (source.ct !== undefined) {
    const ct = record(source.ct)
    cell.ct = {}
    if (ct.fa !== undefined) {
      if (typeof ct.fa !== 'string' || Buffer.byteLength(ct.fa, 'utf8') > 4096) fail()
      cell.ct.fa = ct.fa as string
    }
    if (ct.t !== undefined) {
      if (typeof ct.t !== 'string' || Buffer.byteLength(ct.t, 'utf8') > 64) fail()
      cell.ct.t = ct.t as string
    }
    // Native isInlineStringCell depends on both t and s.length. The formatter
    // omits rich-inline font styles; preserve that flag without copying runs.
    if (ct.t === 'inlineStr' && ct.s !== undefined) {
      const runs = ct.s
      if (!Array.isArray(runs) || runs.length > EXCEL_PREVIEW_LIMITS.maxCells) return fail()
      if (runs.length) cell.ct.s = [{}]
    }
  }
  for (const key of ['bl', 'it', 'cl'] as const) if (source[key] !== undefined) {
    if (!integer(source[key], 0, 1)) fail()
    cell[key] = source[key] as number
  }
  for (const key of ['ht', 'vt'] as const) if (source[key] !== undefined) {
    if (!integer(source[key], 0, 2)) fail()
    cell[key] = source[key] as number
  }
  if (source.un !== undefined) {
    if (!integer(source.un, 0, 4)) fail()
    cell.un = source.un as number
  }
  if (source.fs !== undefined) {
    if (!finite(source.fs) || source.fs > 409) fail()
    cell.fs = source.fs as number
  }
  if (source.fc !== undefined) cell.fc = color(source.fc)
  if (source.bg !== undefined) cell.bg = color(source.bg)
  // The native underline branch reads raw fs/fc, and otherwise emits NaNpx /
  // undefined. Reject this unsafe combination; do not manufacture defaults.
  if (cell.ct?.s === undefined && (cell.un === 1 || cell.un === 3)
      && (cell.fs === undefined || cell.fc === undefined)) fail()
  // No f, hl, ps, hooks, CSS string, CF rule, or executable metadata is copied.
  return cell
}

export function formatExcelCopyHtmlV1(sheet: Sheet, requestedSelection: ExcelSelection): string {
  const selection = toolResultLocalPresentExcelSelectionSchemaV1.parse(requestedSelection)
  if (sheet.luckysheet_conditionformat_save !== undefined
      && (!Array.isArray(sheet.luckysheet_conditionformat_save) || sheet.luckysheet_conditionformat_save.length !== 0)) fail()
  const rows = sheet.row, columns = sheet.column
  if (String(sheet.id) !== selection.sheetId || sheet.hide !== 0
      || !integer(rows, 1, EXCEL_PREVIEW_LIMITS.maxCells) || !integer(columns, 1, EXCEL_PREVIEW_LIMITS.maxCells)
      || rows * columns > EXCEL_PREVIEW_LIMITS.maxCells
      || selection.ranges.some(range => range.row[1] >= rows || range.column[1] >= columns)) return fail()
  const selected = (r: number, c: number): boolean => selection.ranges.some(range =>
    r >= range.row[0] && r <= range.row[1] && c >= range.column[0] && c <= range.column[1])
  const config = configData(sheet.config, rows, columns)
  const data: CellMatrix = Array.from({ length: rows }, () => Array<Cell | null>(columns).fill(null))
  const seen = new Set<string>()
  if (sheet.celldata !== undefined && (!Array.isArray(sheet.celldata) || sheet.celldata.length > rows * columns)) fail()
  for (const item of sheet.celldata ?? []) {
    if (!integer(item.r, 0, rows - 1) || !integer(item.c, 0, columns - 1)) fail()
    const key = `${item.r}:${item.c}`
    if (seen.has(key)) fail()
    seen.add(key)
    if (item.v !== null) data[item.r][item.c] = cellData(item.v, rows, columns, selected(item.r, item.c))
  }
  const ownedSheet: Sheet = { name: '', id: selection.sheetId, row: rows, column: columns,
    hide: 0, data, config, luckysheet_conditionformat_save: [] }
  // This runtime slice is justified by the pinned formatter/helper closure;
  // never call defaultContext, copy, handleCopy, or a DOM/API wrapper with it.
  const ctx = { currentSheetId: selection.sheetId, config, luckysheetfile: [ownedSheet] } as Context
  for (const range of selection.ranges) if (hasPartMC(ctx, config,
    range.row[0], range.row[1], range.column[0], range.column[1])) fail()
  for (const item of sheet.celldata ?? []) {
    if (!selected(item.r, item.c)) continue
    const cell = data[item.r][item.c]
    if (!cell) continue
    if (cell.mc && cell.mc.rs === undefined) { delete cell.m; delete cell.v; continue }
    const rawFormat = cell.ct?.fa
    // Only the library's saved-value helper is called. This is the exact
    // rangeValueToHtml branch discriminator, with the native date rule inside
    // getCellValue; no numeric/date/formula formatter is reimplemented here.
    let value: unknown = rawFormat && /^(w|W)((0?)|(0\.0+))$/.test(rawFormat)
      ? getCellValue(item.r, item.c, data) : getCellValue(item.r, item.c, data, 'm')
    if (value === null || value === undefined) value = getCellValue(item.r, item.c, data)
    const literal = value === null || value === undefined ? '' : String(value)
    if (Buffer.byteLength(literal, 'utf8') > (1 << 20)) throw new ExcelPreviewError('tooLarge')
    const projected = projectOrdinaryToolExportTextV1(literal)
    if (literal !== '' && projected === '') fail()
    // Both native branches receive the same projected display token. Neither
    // a W choice nor a nil fallback can later reveal an unprojected raw value.
    cell.m = projected
    cell.v = projected
  }
  const html = rangeValueToHtml(ctx, selection.sheetId, selection.ranges)
  if (typeof html !== 'string' || html === '') return fail()
  if (Buffer.byteLength(html, 'utf8') > EXCEL_PREVIEW_MAX_OUTPUT_BYTES) throw new ExcelPreviewError('tooLarge')
  return html
}
