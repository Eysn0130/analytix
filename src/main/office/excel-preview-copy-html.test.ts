// @vitest-environment node
import { describe, expect, it } from 'vitest'
import type { Cell, Sheet } from '@fortune-sheet/core'
import { formatExcelCopyHtmlV1 } from './excel-preview-copy-html'
import type { ExcelCopySelectionV1 } from '../services/excel-preview-copy-service'

// Real package functions are imported by
// the implementation; this suite supplies no browser globals or formatter mock.
const selection = (ranges: ExcelCopySelectionV1['ranges'] = [{ row: [0, 0], column: [0, 0] }]): ExcelCopySelectionV1 =>
  ({ sheetIndex: 0, sheetId: '1', ranges })
const sheet = (cells: Cell[][], config: Sheet['config'] = {}): Sheet => ({
  name: 'Synthetic saved values', id: '1', row: cells.length, column: cells[0].length,
  order: 0, hide: 0, config, celldata: cells.flatMap((row, r) => row.map((v, c) => ({ r, c, v })))
})
const texts = (html: string): string[] => [...html.matchAll(/<td\b[^>]*>([\s\S]*?)<\/td>/g)].map(match => match[1])

describe('fixed codec using the actual Fortune HTML formatter', () => {
  it('escapes literal markup exactly once, preserves source, and uses no DOM', () => {
    expect('window' in globalThis).toBe(false)
    expect('document' in globalThis).toBe(false)
    const source = sheet([[{ m: '<b>"&\'literal</b>', v: '<b>"&\'literal</b>' }]])
    const before = structuredClone(source)
    const html = formatExcelCopyHtmlV1(source, selection())
    expect(html).toContain('<table data-type="fortune-copy-action-table">')
    expect(texts(html)).toEqual(['&lt;b&gt;&quot;&amp;&#39;literal&lt;/b&gt;'])
    expect(source).toEqual(before)
    expect('window' in globalThis).toBe(false)
    expect('document' in globalThis).toBe(false)
  })
  it('uses native W/date/nil value choices and retains zero and false', () => {
    const cells: Cell[] = [
      { m: 'W display', v: 0, ct: { fa: 'W0.00', t: 'n' } },
      { m: 'w display', v: false, ct: { fa: 'w', t: 'b' } },
      { m: 'Saved display', v: 1.234, ct: { fa: '0.00', t: 'n' } },
      { m: 'Saved date', v: 45000, ct: { fa: 'W0', t: 'd' } },
      { v: 0, ct: { fa: 'General', t: 'n' } },
      { v: 45000, ct: { t: 'd' } },
      { f: '=1+1', ct: { fa: 'General', t: 'n' } }
    ]
    const html = formatExcelCopyHtmlV1(sheet([cells]), selection([{ row: [0, 0], column: [0, 6] }]))
    expect(texts(html)).toEqual(['0', 'false', 'Saved display', 'Saved date', '0', '', ''])
    expect(html).not.toContain('45000')
    expect(html).not.toContain('=1+1')
  })
  it('projects exported literals before HTML escaping and strips execution metadata', () => {
    const source = sheet([[{ m: '/tmp/synthetic-protected-source.xlsx', v: '/tmp/synthetic-protected-source.xlsx',
      f: '=WEBSERVICE("https://example.invalid")', hl: { r: 0, c: 0, id: 'external-link' },
      ps: { left: 0, top: 0, width: 1, height: 1, value: 'synthetic comment', isShow: true } }]])
    source.luckysheet_conditionformat_save = [{ type: 'formula', cellrange: [{ row: [0, 0], column: [0, 0] }],
      format: { textColor: '#123456', cellColor: '#654321' }, conditionName: 'custom', conditionValue: ['=WEBSERVICE("https://example.invalid")'] }]
    expect(() => formatExcelCopyHtmlV1(source, selection())).toThrow('invalid')
    source.luckysheet_conditionformat_save = []
    const html = formatExcelCopyHtmlV1(source, selection())
    expect(texts(html)).toEqual(['[PRIVATE_PATH]'])
    for (const forbidden of ['synthetic-protected-source', 'WEBSERVICE', 'example.invalid', 'external-link', 'synthetic comment']) expect(html).not.toContain(forbidden)
    expect(html).not.toContain('background:')
  })
  it('preserves actual merge spans and shadow omission, and rejects partial merges', () => {
    const config = { merge: { '0_0': { r: 0, c: 0, rs: 2, cs: 2 } } }
    const source = sheet([
      [{ m: 'Merged', v: 'Merged', mc: { r: 0, c: 0, rs: 2, cs: 2 } }, { m: 'shadow', mc: { r: 0, c: 0 } }],
      [{ m: 'shadow', mc: { r: 0, c: 0 } }, { m: 'shadow', mc: { r: 0, c: 0 } }]
    ], config)
    const html = formatExcelCopyHtmlV1(source, selection([{ row: [0, 1], column: [0, 1] }]))
    expect(texts(html)).toEqual(['Merged'])
    expect(html).toContain('rowspan="2" colspan="2"')
    expect(html).toContain('<tr></tr>')
    expect(html).not.toContain('shadow')
    expect(() => formatExcelCopyHtmlV1(source, selection())).toThrow('invalid')
  })
  it('keeps hidden content and native dimensions/basic font and border styles', () => {
    const source = sheet([
      [{ m: 'Hidden row', v: 'Hidden row', bl: 1, it: 1, cl: 1, fs: 12, fc: '#123456', ht: 0, vt: 2 }, { m: 'Hidden column' }],
      [{ m: 'Visible row' }, { m: 'Visible corner' }]
    ], { rowhidden: { '0': 0 }, colhidden: { '1': 0 }, rowlen: { '0': 40 }, columnlen: { '1': 100 },
      borderInfo: [{ rangeType: 'cell', value: { row_index: 1, col_index: 0, t: { style: 1, color: '#abcdef' } } }] })
    const html = formatExcelCopyHtmlV1(source, selection([{ row: [0, 1], column: [0, 1] }]))
    expect(texts(html)).toEqual(['Hidden row', 'Hidden column', 'Visible row', 'Visible corner'])
    for (const value of ['width="100px"', 'height:40px;', 'font-weight:bold;', 'font-style:italic;',
      'font-size:12pt;', 'color:#123456;', 'text-align:center;', 'align-items:flex-end;',
      'text-decoration:line-through;', 'border-top:0.5pt solid #abcdef;']) expect(html).toContain(value)
  })
  it('retains native rich-inline font omission and does not add absent style behavior', () => {
    const source = sheet([[{ m: 'Saved rich text', v: 'Saved rich text', bl: 1, fs: 12, fc: '#123456', bg: '#abcdef',
      ct: { t: 'inlineStr', s: [{ v: 'Saved rich text', bl: 1, fc: '#123456' }] } }]])
    const html = formatExcelCopyHtmlV1(source, selection())
    expect(texts(html)).toEqual(['Saved rich text'])
    for (const property of ['font-weight:', 'font-size:', 'color:', 'background:']) expect(html).not.toContain(property)
  })
  it('preserves first-seen same-axis range order and rejects unsupported geometry', () => {
    const horizontal = sheet([[{ m: 'First' }, { m: 'Gap' }, { m: 'Last' }]])
    expect(texts(formatExcelCopyHtmlV1(horizontal, selection([
      { row: [0, 0], column: [2, 2] }, { row: [0, 0], column: [0, 0] }
    ])))).toEqual(['Last', 'First'])
    const vertical = sheet([[{ m: 'Top' }], [{ m: 'Bottom' }]])
    expect(texts(formatExcelCopyHtmlV1(vertical, selection([
      { row: [1, 1], column: [0, 0] }, { row: [0, 0], column: [0, 0] }
    ])))).toEqual(['Bottom', 'Top'])
    expect(() => formatExcelCopyHtmlV1(sheet([[{}, {}], [{}, {}]]), selection([
      { row: [0, 0], column: [0, 0] }, { row: [1, 1], column: [1, 1] }
    ]))).toThrow()
  })
  it('fails closed on unsafe styles/dimensions and an unsafe native underline combination', () => {
    expect(() => formatExcelCopyHtmlV1(sheet([[{ m: 'Safe', fc: '#123456";background:url(x)' }]]), selection())).toThrow('invalid')
    expect(() => formatExcelCopyHtmlV1(sheet([[{ m: 'Safe' }]], { rowlen: { '0': Infinity } }), selection())).toThrow('invalid')
    expect(() => formatExcelCopyHtmlV1(sheet([[{ m: 'Safe', un: 1 }]]), selection())).toThrow('invalid')
    expect(() => formatExcelCopyHtmlV1(sheet([[{ m: 'Safe' }]], { borderInfo: [{ rangeType: 'cell',
      value: { row_index: 0, col_index: 0, t: { style: 14, color: '#123456' } } }] }), selection())).toThrow('invalid')
  })
})
