import type * as Md from 'mdast'

/** Public cell text only. Export encodings never mutate this matrix. */
export interface MarkdownTableData {
  headers: readonly string[]
  rows: readonly (readonly string[])[]
  align: readonly Md.AlignType[]
}

function cellText(node: Md.TableCell['children'][number]): string {
  switch (node.type) {
    case 'image': case 'imageReference': return node.alt ?? ''
    case 'break': return '\n'
    case 'footnoteReference': return `[^${node.identifier}]`
    case 'inlineCode': return node.value.replace(/\r\n|\r|\n/g, ' ')
    case 'text': case 'html': case 'inlineMath': return node.value
    case 'strong': case 'emphasis': case 'delete': case 'link': case 'linkReference':
      return node.children.map(cellText).join('')
    default: return ''
  }
}

export function publicTableData(node: Md.Table): MarkdownTableData {
  const [head, ...rows] = node.children
  const columns = node.align?.length ?? head?.children.length ?? 0
  const values = (row: Md.TableRow | undefined): string[] => Array.from({ length: columns }, (_, index) =>
    row?.children[index]?.children.map(cellText).join('') ?? '')
  return { headers: values(head), rows: rows.map(values), align: node.align ?? Array(columns).fill(null) }
}

/** Spreadsheet-facing text encoding, including whitespace/control and locale variants.
 * This changes the export representation, not the public value. No claim is made
 * about every spreadsheet or a receiver stripping escapes on save/reopen. */
export function spreadsheetCellText(value: string): string {
  return /^[\s\u0000-\u001f]*[=+\-@＝＋－＠]/u.test(value) || /^[\t\r\n]/.test(value)
    ? `'${value}` : value
}

export function tableDataToCsv(data: MarkdownTableData): string {
  const cell = (value: string): string => {
    const text = spreadsheetCellText(value)
    return /[",\r\n]/.test(text) || text !== value ? `"${text.replace(/"/g, '""')}"` : text
  }
  return [data.headers, ...data.rows].map(row => row.map(cell).join(',')).join('\r\n')
}

export function tableDataToTsv(data: MarkdownTableData): string {
  const cell = (value: string): string => spreadsheetCellText(value)
    .replace(/\\/g, '\\\\').replace(/\t/g, '\\t').replace(/\r/g, '\\r').replace(/\n/g, '\\n')
  return [data.headers, ...data.rows].map(row => row.map(cell).join('\t')).join('\n')
}

export function tableDataToMarkdown(data: MarkdownTableData): string {
  const cell = (value: string): string => value.replace(/[\\|`*_{}\[\]<>!~$&]/g, character => `\\${character}`)
    .replace(/\r/g, '\\r').replace(/\n/g, '\\n').replace(/\t/g, '\\t')
  const row = (values: readonly string[]): string => `| ${values.map(cell).join(' | ')} |`
  const separators = data.headers.map((_, index) => data.align[index] === 'left' ? ':---'
    : data.align[index] === 'right' ? '---:' : data.align[index] === 'center' ? ':---:' : '---')
  return [row(data.headers), row(separators), ...data.rows.map(row)].join('\n')
}

export function tableDataToHtml(data: MarkdownTableData): string {
  const escape = (value: string): string => spreadsheetCellText(value).replace(/&/g, '&amp;')
    .replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
    .replace(/\r\n|\r|\n/g, '<br>')
  const row = (values: readonly string[], tag: 'th' | 'td'): string =>
    `<tr>${values.map(value => `<${tag}>${escape(value)}</${tag}>`).join('')}</tr>`
  return `<table><thead>${row(data.headers, 'th')}</thead><tbody>${data.rows.map(values => row(values, 'td')).join('')}</tbody></table>`
}
