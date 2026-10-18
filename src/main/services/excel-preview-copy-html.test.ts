// @vitest-environment node
import { describe, expect, it } from 'vitest'
import type { Cell, Sheet } from '@fortune-sheet/core'
import { formatExcelCopyHtmlV1 } from '../office/excel-preview-copy-html'
import { excelSelectionTextV1, validateExcelCopyHtmlV1 } from './excel-preview-copy-html'
import type { ExcelCopyFrameV1 } from './excel-preview-copy-service'

function frame(values: Cell[][], config: Sheet['config'] = {}): ExcelCopyFrameV1 {
  const ranges: ExcelCopyFrameV1['ranges'] = [{ row: [0, values.length - 1], column: [0, values[0].length - 1] }]
  const cells: ExcelCopyFrameV1['cells'] = values.flatMap((row, r) => row.map((value, c) => [r, c, {
    ...(value.m !== undefined ? { m: value.m } : {}), ...(value.v !== undefined ? { v: value.v } : {}),
    ...(value.mc !== undefined ? { mc: value.mc } : {}), ...(value.ct?.fa !== undefined ? { numberFormat: value.ct.fa } : {}),
    ...(value.ct?.t === 'd' ? { cellType: 'd' as const } : {})
  }] as ExcelCopyFrameV1['cells'][number]))
  const source: Sheet = { name: 'Finite native formatter input', id: '1', hide: 0,
    row: values.length, column: values[0].length, config,
    celldata: values.flatMap((row, r) => row.map((v, c) => ({ r, c, v }))) }
  return { schemaVersion: 1, op: 'excel-preview-copy-cells-v1', sheetIndex: 0, sheetId: '1',
    rowCount: values.length, columnCount: values[0].length, ranges, cells,
    html: formatExcelCopyHtmlV1(source, { sheetIndex: 0, sheetId: '1', ranges }) }
}

describe('Excel Host admits only native HTML for its projected cells', () => {
  it('admits the real single formatter output, projected literals, safe styles and dimensions', () => {
    const value = frame([[{ m: 'account: 6222020202020202020', fs: 12, fc: '#123456', bl: 1 }, { m: '<b>"&\'literal</b>' }]],
      { rowlen: { 0: 40 }, columnlen: { 1: 100 } })
    expect(validateExcelCopyHtmlV1(value)).toBe(value.html)
    expect(value.html).toContain('account: [ACCOUNT]')
    expect(value.html).toContain('font-size:12pt;')
    expect(value.html).toContain('width="100px"')
    expect(excelSelectionTextV1(value)).toBe('account: [ACCOUNT]\t"<b>""&\'literal</b>"')
  })
  it('admits actual merge structure and empty shadows and rejects forged shadow values', () => {
    const value = frame([
      [{ m: 'Merged', mc: { r: 0, c: 0, rs: 2, cs: 2 } }, { mc: { r: 0, c: 0 } }],
      [{ mc: { r: 0, c: 0 } }, { mc: { r: 0, c: 0 } }]
    ], { merge: { '0_0': { r: 0, c: 0, rs: 2, cs: 2 } } })
    expect(validateExcelCopyHtmlV1(value)).toBe(value.html)
    const forged = structuredClone(value); forged.cells[1][2].m = 'shadow value'
    expect(() => validateExcelCopyHtmlV1(forged)).toThrow('Unsupported selection geometry.')
    expect(() => excelSelectionTextV1(forged)).toThrow()
    expect(() => validateExcelCopyHtmlV1({ ...value, html: value.html!.replace('rowspan="2"', 'rowspan="1"') })).toThrow()
  })
  it('retains native W/date choices and cached zero/false in both checked formats', () => {
    const value = frame([[{ m: 'W display', v: 0, ct: { fa: 'W0', t: 'n' } },
      { m: 'w display', v: false, ct: { fa: 'w', t: 'b' } }, { m: 'Saved date', v: 45000, ct: { t: 'd' } }, {}]])
    expect(excelSelectionTextV1(value)).toBe('0\tfalse\tSaved date\t')
    expect(validateExcelCopyHtmlV1(value)).toBe(value.html)
  })
  it('decodes lodash entities once and rejects alternate entities or a different cell body', () => {
    const value = frame([[{ m: '&lt;literal&gt;' }]])
    expect(validateExcelCopyHtmlV1(value)).toBe(value.html)
    for (const html of [value.html!.replace('&amp;lt;', '&lt;'),
      value.html!.replace('&amp;lt;', '&#x3c;'), value.html!.replace('&amp;lt;literal&amp;gt;', 'other')]) {
      expect(() => validateExcelCopyHtmlV1({ ...value, html })).toThrow()
    }
  })
  it('rejects executable elements, extra attributes, remote CSS and trailing markup', () => {
    const value = frame([[{ m: 'Saved', fs: 12, fc: '#123456' }]])
    for (const html of [value.html!.replace('Saved</td>', '<script>Saved</script></td>'),
      value.html!.replace('<td  style=', '<td onclick="bad()" style='),
      value.html!.replace('font-size:12pt;', 'background:url(https://example.invalid);'),
      value.html!.replace('font-size:12pt;', 'font-size:12pt;position:absolute;'), value.html! + '<img src="https://example.invalid">']) {
      expect(() => validateExcelCopyHtmlV1({ ...value, html })).toThrow()
    }
  })
  it('refuses absent, oversized and mismatched output without a text-only fallback', () => {
    const value = frame([[{ m: 'Saved' }]])
    expect(() => validateExcelCopyHtmlV1({ ...value, html: undefined })).toThrow()
    expect(() => validateExcelCopyHtmlV1({ ...value, html: 'x'.repeat((16 << 20) + 1) })).toThrow()
    expect(() => validateExcelCopyHtmlV1({ ...value, html: value.html!.replace('Saved', 'unprojected') })).toThrow()
  })
})


it.each(['horizontal', 'vertical'] as const)('refuses %s split identifiers for both checked formats', direction => {
  const value = frame(direction === 'horizontal'
    ? [[{ m: '62220202' }, { m: '02020202' }]] : [[{ m: '62220202' }], [{ m: '02020202' }]])
  expect(() => excelSelectionTextV1(value)).toThrow('Unsupported selection content.')
  expect(() => validateExcelCopyHtmlV1(value)).toThrow('Unsupported selection content.')
})
