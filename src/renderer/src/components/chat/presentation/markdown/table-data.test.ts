import type { Table } from 'mdast'
import { describe, expect, it } from 'vitest'
import { parseGfmWithMath } from './parse'
import { publicTableData, spreadsheetCellText, tableDataToCsv, tableDataToHtml, tableDataToMarkdown, tableDataToTsv } from './table-data'

describe('public Markdown table exports', () => {
  it('projects only visible labels, literals and math, retaining column alignment and raw values', () => {
    const source = '| 中英 | Details |\n| :--- | ---: |\n| **汉字** `code` | [label](https://example.test/private) ![alt](https://example.test/image.png) $x^2$ <b>literal</b> |\n| short |\n| one | two | ignored |'
    const ast = parseGfmWithMath(source)
    const before = JSON.stringify(ast)
    const data = publicTableData(ast.children[0] as Table)
    expect(data).toEqual({ headers: ['中英', 'Details'], align: ['left', 'right'], rows: [
      ['汉字 code', 'label alt x^2 <b>literal</b>'], ['short', ''], ['one', 'two']
    ] })
    expect(JSON.stringify(ast)).toBe(before)
    expect(JSON.stringify(data)).not.toContain('https:')
  })

  it('escapes spreadsheet fields and HTML without changing the public matrix', () => {
    const data = { headers: ['Name', 'Value'], align: [null, null], rows: [
      ['汉字, "quote"', 'line\r\nnext'], ['danger', ' =SUM(A1)'], ['html', '<img src=x onerror=alert(1)>'], ['tab', '\t@cmd']
    ] }
    const before = JSON.stringify(data)
    expect(tableDataToCsv(data)).toBe('Name,Value\r\n"汉字, ""quote""","line\r\nnext"\r\ndanger,"\' =SUM(A1)"\r\nhtml,<img src=x onerror=alert(1)>\r\ntab,"\'\t@cmd"')
    expect(tableDataToTsv(data)).toContain("tab\t'\\t@cmd")
    const html = tableDataToHtml(data)
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;')
    expect(html).toContain('&#39; =SUM(A1)')
    expect(html).toContain('line<br>next')
    expect(html).not.toContain('<img')
    expect(JSON.stringify(data)).toBe(before)
  })

  it.each(['=1', '+cmd', '-1', '@cmd', '  =1', '\u0001+cmd', '＝1', '＋1', '－1', '＠cmd', '\tplain', '\rplain', '\nplain'])(
    'encodes formula/control-like value %j as text', value => expect(spreadsheetCellText(value)).toBe("'" + value)
  )

  it('exports Markdown text with literal syntax and explicit control escapes, preserving formula source', () => {
    const data = { headers: ['A|B'], align: ['center' as const], rows: [['![x](url) <b> \\ `code`\n\t=1']] }
    expect(tableDataToMarkdown(data)).toBe('| A\\|B |\n| :---: |\n| \\!\\[x\\](url) \\<b\\> \\\\ \\`code\\`\\n\\t=1 |')
    expect(spreadsheetCellText('ordinary')).toBe('ordinary')
  })

  it('preserves text that resembles strikethrough, math and entities when parsed again', () => {
    const data = { headers: ['Literal'], align: [null], rows: [['~~keep~~ $x$ &amp;']] }
    const table = parseGfmWithMath(tableDataToMarkdown(data)).children[0] as Table
    expect(publicTableData(table).rows).toEqual(data.rows)
  })
})
