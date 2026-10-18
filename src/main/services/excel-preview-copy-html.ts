import type { ExcelCopyCellV1, ExcelCopyFrameV1 } from './excel-preview-copy-service'
import { projectOrdinaryToolExportTextV1 } from './write-export-content'

const TEXT_MAX_BYTES = 1 << 20
const HTML_MAX_BYTES = 16 << 20

/** Native formatter order: first-seen selected rows and columns, including hidden indices. */
function axes(frame: ExcelCopyFrameV1): { rows: number[]; columns: number[] } {
  const rows: number[] = [], columns: number[] = [], rowSeen = new Set<number>(), columnSeen = new Set<number>()
  for (const range of frame.ranges) {
    for (let r = range.row[0]; r <= range.row[1]; r++) if (!rowSeen.has(r)) { rowSeen.add(r); rows.push(r) }
    for (let c = range.column[0]; c <= range.column[1]; c++) if (!columnSeen.has(c)) { columnSeen.add(c); columns.push(c) }
  }
  return { rows, columns }
}
function cellText(cell: ExcelCopyCellV1 | undefined): string {
  if (cell?.mc && cell.mc.rs === undefined) {
    if (cell.m !== undefined || cell.v !== undefined) throw new Error('Unsupported selection geometry.')
    return ''
  }
  const value = cell?.cellType === 'd' ? cell.m
    : cell?.numberFormat && /^(w|W)((0?)|(0\.0+))$/.test(cell.numberFormat) ? cell.v : cell?.m ?? cell?.v
  const original = value === undefined ? '' : String(value)
  if (Buffer.byteLength(original, 'utf8') > TEXT_MAX_BYTES) throw new Error('Unsupported selection size.')
  const projected = projectOrdinaryToolExportTextV1(original)
  if (original !== '' && projected === '') throw new Error('Unsupported selection content.')
  return projected
}

/** Existing explicit TSV text MIME; browser innerText/textContent parity remains unverified. */
export function excelSelectionTextV1(frame: ExcelCopyFrameV1): string {
  const cells = new Map(frame.cells.map(([r, c, cell]) => [`${r}:${c}`, cell]))
  const { rows: rowIndices, columns: columnIndices } = axes(frame)
  const rows: string[] = []
  let bytes = 0
  for (const [rowPosition, r] of rowIndices.entries()) {
    const columns: string[] = []
    for (const [columnPosition, c] of columnIndices.entries()) {
      const projected = cellText(cells.get(`${r}:${c}`))
      const field = /["\t\r\n]/.test(projected) ? `"${projected.replaceAll('"', '""')}"` : projected
      bytes += Buffer.byteLength(field, 'utf8') + (columnPosition ? 1 : 0)
      if (bytes > TEXT_MAX_BYTES) throw new Error('Unsupported selection size.')
      columns.push(field)
    }
    bytes += rowPosition ? 1 : 0
    if (bytes > TEXT_MAX_BYTES) throw new Error('Unsupported selection size.')
    rows.push(columns.join('\t'))
  }
  const combined = rows.join('\n')
  const projected = projectOrdinaryToolExportTextV1(combined)
  // A whole-selection rewrite can redact tokens split across cells. Refuse
  // the paired export rather than preserve those unprojected tokens in HTML.
  if (projected !== combined) throw new Error('Unsupported selection content.')
  if (Buffer.byteLength(projected, 'utf8') > TEXT_MAX_BYTES) throw new Error('Unsupported selection size.')
  return projected
}

function checkedStyle(style: string): void {
  if (style === '') return
  if (!style.endsWith(';')) throw new Error('Unsupported selection HTML.')
  for (const declaration of style.slice(0, -1).split(';')) {
    const split = declaration.indexOf(':')
    const name = declaration.slice(0, split), value = declaration.slice(split + 1)
    let valid = false
    if (name === 'height') valid = dimension(value, 'px', 8192)
    else if (name === 'font-size') valid = dimension(value, 'pt', 409)
    else if (name === 'color' || name === 'background') valid = /^#[\da-f]{6}$/i.test(value)
    else if (name === 'font-weight') valid = value === 'bold'
    else if (name === 'font-style') valid = value === 'italic'
    else if (name === 'text-align') valid = value === 'center' || value === 'right'
    else if (name === 'align-items') valid = value === 'center' || value === 'flex-end'
    else if (name === 'text-decoration') valid = value === 'line-through'
    else if (/^border-(top|right|bottom|left)$/.test(name)) {
      const match = /^([\d.]+(?:px|pt)) (solid|double|dashed|dotted) (#[\da-f]{6})$/i.exec(value)
      valid = !!match && (dimension(match[1], 'px', 45) || dimension(match[1], 'pt', 45))
    }
    if (!valid) throw new Error('Unsupported selection HTML.')
  }
}
function dimension(value: string, unit: 'px' | 'pt', max: number): boolean {
  const match = new RegExp(`^([0-9]+(?:\\.[0-9]+)?)${unit}$`).exec(value)
  return !!match && Number.isFinite(Number(match[1])) && Number(match[1]) >= 0 && Number(match[1]) <= max
}
function decodeEscapedLiteral(value: string): string {
  // The reviewed formatter applies lodash.escape once. Never recursively decode
  // entities or accept markup, alternate numeric entities, URLs or style text.
  if (/[<>]/.test(value) || /&(?!amp;|lt;|gt;|quot;|#39;)/.test(value)) throw new Error('Unsupported selection HTML.')
  const entities = { amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'" } as const
  return value.replace(/&(amp|lt|gt|quot|#39);/g, (_whole, name: string) => entities[name as keyof typeof entities])
}

/**
 * Linear admission of the actual fixed Fortune table grammar, never a formatter.
 * Every decoded td must equal its Host-projected selected literal. No DOM, HTML
 * rewriting, arbitrary tags/attributes, URL-bearing CSS or raw clipboard route.
 */
function admitExcelCopyHtmlV1(frame: ExcelCopyFrameV1): string {
  const html = frame.html
  if (typeof html !== 'string' || html === '' || Buffer.byteLength(html, 'utf8') > HTML_MAX_BYTES) throw new Error('Unsupported selection HTML.')
  const { rows, columns } = axes(frame)
  const cells = new Map(frame.cells.map(([r, c, cell]) => [`${r}:${c}`, cell]))
  let offset = 0, mergedArea = 0
  const covered = new Set<string>()
  const literal = (value: string): void => {
    if (!html.startsWith(value, offset)) throw new Error('Unsupported selection HTML.')
    offset += value.length
  }
  const take = (pattern: RegExp): RegExpExecArray => {
    pattern.lastIndex = offset
    const match = pattern.exec(html)
    if (!match) throw new Error('Unsupported selection HTML.')
    offset = pattern.lastIndex
    return match
  }
  literal('<table data-type="fortune-copy-action-table">')
  for (let index = 0; index < columns.length; index++) {
    const width = take(/<colgroup width="([^"]*)"><\/colgroup>/y)[1]
    if (!dimension(width, 'px', 8192)) throw new Error('Unsupported selection HTML.')
  }
  for (const r of rows) {
    literal('<tr>')
    for (const c of columns) {
      const cell = cells.get(`${r}:${c}`)
      const merge = cell?.mc
      if (merge && merge.rs === undefined) {
        const master = cells.get(`${merge.r}:${merge.c}`)?.mc
        if (!master?.rs || !master.cs || r < master.r || r >= master.r + master.rs
            || c < master.c || c >= master.c + master.cs || cell.m !== undefined || cell.v !== undefined) throw new Error('Unsupported selection HTML.')
        continue
      }
      const opening = take(/<td (?:(?:rowspan="([1-9][0-9]*)" colspan="([1-9][0-9]*)" )| )style="([^"]*)">/y)
      if (merge) {
        if (merge.r !== r || merge.c !== c || merge.rs === undefined || merge.cs === undefined
            || String(merge.rs) !== opening[1] || String(merge.cs) !== opening[2]) throw new Error('Unsupported selection HTML.')
        mergedArea += merge.rs * merge.cs
        if (mergedArea > rows.length * columns.length) throw new Error('Unsupported selection HTML.')
        for (let row = r; row < r + merge.rs; row++) for (let column = c; column < c + merge.cs; column++) {
          const key = `${row}:${column}`, member = cells.get(key)?.mc
          if (!member || member.r !== r || member.c !== c || covered.has(key)) throw new Error('Unsupported selection HTML.')
          covered.add(key)
        }
      } else if (opening[1] !== undefined || opening[2] !== undefined) throw new Error('Unsupported selection HTML.')
      checkedStyle(opening[3])
      const end = html.indexOf('</td>', offset)
      if (end < offset || decodeEscapedLiteral(html.slice(offset, end)) !== cellText(cell)) throw new Error('Unsupported selection HTML.')
      offset = end
      literal('</td>')
    }
    literal('</tr>')
  }
  literal('</table>')
  if (offset !== html.length) throw new Error('Unsupported selection HTML.')
  return html
}


/** One privacy decision for both native formats, before the final lease/digest effect. */
export function projectExcelCopyContentV1(frame: ExcelCopyFrameV1): { text: string; html: string } {
  const text = excelSelectionTextV1(frame)
  return { text, html: admitExcelCopyHtmlV1(frame) }
}
export function validateExcelCopyHtmlV1(frame: ExcelCopyFrameV1): string {
  return projectExcelCopyContentV1(frame).html
}
