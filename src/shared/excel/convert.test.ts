import { describe, expect, it } from 'vitest'
import ExcelJS from 'exceljs'
import { zipSync } from 'fflate'
import { utils, write } from 'xlsx'
import { convertExcel } from './convert'
import { excelFormat } from './format'

const limits = { maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000 }
const encode = (text: string) => new TextEncoder().encode(text)

describe('saved-value spreadsheet format admission', () => {
  it('selects only the four admitted filename suffixes', () => {
    for (const format of ['xlsx', 'xls', 'csv', 'tsv'] as const) expect(excelFormat('Host.' + format.toUpperCase())).toBe(format)
    for (const path of ['Host.pdf', 'Host.xlsx.html', 'Host']) expect(() => excelFormat(path)).toThrow('invalid')
  })
  it('preserves CSV quoting, empty cells, literal formulas and HTML as text', async () => {
    const source = encode('"a,b",=1+1,<b>literal</b>\n,,tail')
    const before = source.slice(), value = await convertExcel(source, 'csv', limits)
    expect(source).toEqual(before)
    expect(value.sheets[0].celldata).toMatchObject([
      { r: 0, c: 0, v: { v: 'a,b', m: 'a,b', ct: { t: 's' } } },
      { r: 0, c: 1, v: { v: '=1+1', m: '=1+1' } },
      { r: 0, c: 2, v: { v: '<b>literal</b>', m: '<b>literal</b>' } },
      { r: 1, c: 0, v: { v: '', m: '' } }, { r: 1, c: 1, v: { v: '', m: '' } },
      { r: 1, c: 2, v: { v: 'tail', m: 'tail' } }
    ])
    expect(value.sheets[0].celldata?.every(cell => cell.v?.f === undefined)).toBe(true)
    expect(value.missingResults).toBe(0)
  })
  it('handles BOM UTF-16 TSV and rejects invalid encoding or excess matrix area', async () => {
    const text = 'saved\t001', utf16 = new Uint8Array(2 + text.length * 2), view = new DataView(utf16.buffer)
    view.setUint16(0, 0xfeff, true)
    for (let i = 0; i < text.length; i++) view.setUint16(2 + i * 2, text.charCodeAt(i), true)
    const value = await convertExcel(utf16, 'tsv', limits)
    expect(value.sheets[0].celldata?.map(cell => cell.v?.v)).toEqual(['saved', '001'])
    await expect(convertExcel(new Uint8Array([0xff]), 'csv', limits)).rejects.toMatchObject({ code: 'encoding' })
    await expect(convertExcel(encode('a,b\nc,d'), 'csv', { ...limits, maxCells: 3 })).rejects.toMatchObject({ code: 'tooLarge' })
    await expect(convertExcel(encode('a,b'), 'csv', { ...limits, maxBytes: 2 })).rejects.toMatchObject({ code: 'tooLarge' })
  })
  it('uses saved XLSX zero/false formula caches without calculating, and preserves layout', async () => {
    const workbook = new ExcelJS.Workbook(), sheet = workbook.addWorksheet('Saved')
    sheet.getCell('A1').value = { formula: '1-1', result: 0 }
    sheet.getCell('B1').value = { formula: '1=2', result: false }
    sheet.getCell('C1').value = { formula: '1+1' }
    sheet.mergeCells('A2:B2'); sheet.getCell('A2').value = 'Merged'
    sheet.getColumn(1).width = 12; sheet.getRow(1).height = 24
    const bytes = new Uint8Array(await workbook.xlsx.writeBuffer() as ArrayBuffer), before = bytes.slice()
    const value = await convertExcel(bytes, 'xlsx', limits), cells = value.sheets[0].celldata!
    expect(bytes).toEqual(before)
    expect(cells.find(cell => cell.r === 0 && cell.c === 0)?.v).toMatchObject({ f: '=1-1', v: 0, m: '0' })
    expect(cells.find(cell => cell.r === 0 && cell.c === 1)?.v).toMatchObject({ f: '=1=2', v: false, m: 'false' })
    expect(cells.find(cell => cell.r === 0 && cell.c === 2)?.v?.v).toBeUndefined()
    expect(value.missingResults).toBe(1)
    expect(value.sheets[0].config?.merge?.['1_0']).toMatchObject({ r: 1, c: 0, rs: 1, cs: 2 })
    expect(value.sheets[0].config?.rowlen?.[0]).toBe(32)
  })
  it('admits actual BIFF and rejects renamed text and malformed XLSX', async () => {
    const workbook = utils.book_new(); utils.book_append_sheet(workbook, utils.aoa_to_sheet([['Saved', '001'], [0, false]]), 'Legacy')
    const bytes = new Uint8Array(write(workbook, { bookType: 'biff8', type: 'array' }) as ArrayBuffer)
    const value = await convertExcel(bytes, 'xls', limits)
    expect(value.sheets[0].name).toBe('Legacy')
    expect(value.sheets[0].celldata?.find(cell => cell.r === 1 && cell.c === 0)?.v?.v).toBe(0)
    await expect(convertExcel(encode('renamed,csv'), 'xls', limits)).rejects.toMatchObject({ code: 'invalid' })
    await expect(convertExcel(encode('renamed,csv'), 'xlsx', limits)).rejects.toMatchObject({ code: 'invalid' })
  })
})


describe('XLSX archive expansion admission', () => {
  it('rejects the declared expansion before inflating or handing bytes to ExcelJS', async () => {
    const compressed = new Uint8Array(zipSync({ 'xl/workbook.xml': new Uint8Array(4096) }))
    await expect(convertExcel(compressed, 'xlsx', { ...limits, maxUncompressedBytes: 256 })).rejects.toMatchObject({ code: 'tooLarge' })
  })
  it('rejects case-equivalent duplicate parts and unsupported ZIP64 declarations', async () => {
    const ambiguous = new Uint8Array(zipSync({ 'xl/a.xml': encode('a'), 'XL/A.XML': encode('b') }))
    await expect(convertExcel(ambiguous, 'xlsx', limits)).rejects.toMatchObject({ code: 'invalid' })
    const zip64 = new Uint8Array(zipSync({ 'xl/a.xml': encode('a') }))
    new DataView(zip64.buffer).setUint16(zip64.length - 22 + 10, 0xffff, true)
    await expect(convertExcel(zip64, 'xlsx', limits)).rejects.toMatchObject({ code: 'invalid' })
  })
})


it('rejects repeated shared-string output amplification without truncating cells', async () => {
  const workbook = new ExcelJS.Workbook(), sheet = workbook.addWorksheet('Repeated')
  const repeated = 'x'.repeat(1024)
  for (let row = 1; row <= 9000; row++) sheet.getCell(row, 1).value = repeated
  const bytes = new Uint8Array(await workbook.xlsx.writeBuffer({ useSharedStrings: true }) as ArrayBuffer)
  expect(bytes.length).toBeLessThan(limits.maxBytes)
  await expect(convertExcel(bytes, 'xlsx', limits)).rejects.toMatchObject({ code: 'tooLarge' })
})
