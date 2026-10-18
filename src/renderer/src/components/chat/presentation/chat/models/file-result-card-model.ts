import type { ToolBlock } from '../../../../../agent/types'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalFileResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolDetailItem, ToolDetailsModel } from './details-card-model'
import type { RecordedToolTranslate } from './recorded-tool-card-model'

const fileResultKinds = { ls: 'tool_call', find: 'tool_call', code_index: 'tool_call',
  move_file: 'file_change', notebook_edit: 'file_change', delete_range: 'file_change', delete_symbol: 'file_change' } as const
export function fileResultToolKind(name: unknown): ToolBlock['toolKind'] | undefined {
  return typeof name === 'string' && Object.hasOwn(fileResultKinds, name) ? fileResultKinds[name as keyof typeof fileResultKinds] : undefined
}
type ResultDiff = Extract<ToolResultLocalFileResultV1, { operation: 'delete_range' }>['deleteRange']['diff']
export type FileResultCardModel = ToolDetailsModel & { diff?: ResultDiff; producerTruncated?: boolean }

/** One settled root observation, inside the existing protected result lease. */
export function fileResultCardModel(block: ToolBlock, display: ToolResultLocalDisplayV1, t: RecordedToolTranslate): FileResultCardModel | null {
  const name = block.meta?.toolName, expected = fileResultToolKind(name)
  if (!expected || block.toolKind !== expected || block.status !== 'success' || block.meta?.child !== undefined ||
    block.meta?.parentCallId !== undefined || ('parentCallId' in block && block.parentCallId !== undefined)) return null
  const parsed = toolResultLocalDisplaySchemaV1.safeParse(display)
  if (!parsed.success || parsed.data.toolName !== name || parsed.data.capture.kind !== 'file_result' ||
    block.meta?.localResult?.resultItemId !== parsed.data.resultItemId) return null
  const result = parsed.data.capture.fileResult
  const field = (key: string, value: string | number) => ({ label: t('dshFileResult.' + key), value: String(value) })
  const count = (shown: number, total: number) => t('dshFileResult.captured', { shown, total })
  const empty = (originalCount: number) => t(originalCount === 0 ? 'dshFileResult.empty' : 'dshFileResult.captureUnavailable')
  const item = (title: string, fields: ToolDetailItem['fields'] = [], subtitle?: string): ToolDetailItem => ({ title, fields, subtitle })
  switch (result.operation) {
    case 'ls': {
      const value = result.list
      return { summary: value.relativePath || value.path, expandedSummary: count(value.entries.length, value.originalCount),
        items: value.entries.map(entry => item(entry.displayName, [field('kind', t('dshFileResult.' + entry.kind))])),
        empty: empty(value.originalCount), producerTruncated: value.producerTruncated }
    }
    case 'find': {
      const value = result.find
      return { summary: value.pattern, expandedSummary: count(value.matches.length, value.originalCount),
        items: value.matches.map(match => item(match.relativePath || match.path)), empty: empty(value.originalCount),
        producerTruncated: value.producerTruncated, caption: value.relativePath || value.path }
    }
    case 'code_index': {
      const value = result.codeIndex
      return { summary: value.relativePath || value.path, expandedSummary: count(value.symbols.length, value.symbolCount),
        items: value.symbols.map(symbol => item(symbol.name, [field('kind', symbol.kind), field('line', symbol.line),
          ...(symbol.parent ? [field('parent', symbol.parent)] : []), ...(symbol.signature ? [field('signature', symbol.signature)] : [])], symbol.file)),
        empty: empty(value.originalCount), producerTruncated: value.producerTruncated,
        caption: value.limitReached ? t('dshFileResult.limitReached', { limit: value.limit }) : undefined }
    }
    case 'move_file': {
      const value = result.move
      return { summary: (value.sourceRelativePath || value.sourcePath) + ' → ' + (value.destinationRelativePath || value.destinationPath),
        items: [item(t(value.moved ? 'dshFileResult.moved' : 'dshFileResult.unchanged'), [field('bytesMoved', value.bytesMoved)], value.message || undefined)], empty: '' }
    }
    case 'notebook_edit': {
      const value = result.notebookEdit
      return { summary: value.relativePath || value.path, items: [item(value.message, [field('editMode', value.editMode),
        field('cellNumber', value.cellNumber), field('cellCount', value.cellCount), field('cellType', value.cellType),
        ...(value.cellId === undefined ? [] : [field('cellId', value.cellId)]), field('bytesWritten', value.bytesWritten), field('firstChangedLine', value.firstChangedLine)])],
        empty: '', diff: value.diff }
    }
    case 'delete_range': {
      const value = result.deleteRange
      return { summary: value.relativePath || value.path, items: [item(t('dshFileResult.deletedRange'), [field('startLine', value.startLine),
        field('endLine', value.endLine), field('firstDeletedLine', value.firstDeletedLine), field('deletedLines', value.deletedLines),
        field('bytesWritten', value.bytesWritten), field('encoding', value.encoding), field('inclusive', String(value.inclusive))])], empty: '', diff: value.diff }
    }
    case 'delete_symbol': {
      const value = result.deleteSymbol
      return { summary: value.relativePath || value.path, items: [item(value.name, [field('kind', value.kind), field('line', value.line),
        ...(value.parent === undefined ? [] : [field('parent', value.parent)]), field('firstDeletedLine', value.firstDeletedLine),
        field('deletedLines', value.deletedLines), field('bytesWritten', value.bytesWritten), field('encoding', value.encoding)])], empty: '', diff: value.diff }
    }
  }
}
