import { describe, expect, it } from 'vitest'
import type { ToolBlock } from '../../../../../agent/types'
import { toolResultLocalDisplaySchemaV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { fileResultCardModel, fileResultToolKind } from './file-result-card-model'
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const base = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId,
  inspect: { inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
const capture = { kind: 'file_result', status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false }
const facts = { path: '/synthetic/original', relativePath: 'original' }
const diff = { kind: 'modify', added: 0, removed: 1, patch: 'ORIGINAL_PATCH', binary: false, truncated: false }
const results = [
  { operation: 'ls', list: { ...facts, entries: [{ name: 'FIRST', displayName: 'FIRST/', kind: 'directory' }, { name: 'SECOND', displayName: 'SECOND', kind: 'file' }], names: ['FIRST/', 'SECOND'], limit: 2, producerTruncated: true, originalCount: 2 } },
  { operation: 'find', find: { ...facts, pattern: '*.go', matches: [{ path: '/synthetic/actual.go', relativePath: 'actual.go' }], skippedProtected: [], skippedProtectedOriginalCount: 0, limit: 5, producerTruncated: false, originalCount: 1 } },
  { operation: 'code_index', codeIndex: { ...facts, action: 'outline', query: '', kind: '', limit: 1,
    symbols: [{ name: 'Actual', kind: 'function', path: '/synthetic/actual.go', file: 'actual.go', line: 7, parent: '', signature: 'func Actual()' }],
    originalCount: 1, symbolCount: 1, text: 'ORIGINAL_FORMATTED_TEXT', skippedProtected: [], skippedProtectedOriginalCount: 0,
    skippedDirectories: [], skippedDirectoriesOriginalCount: 0, producerTruncated: false, limitReached: true } },
  { operation: 'move_file', move: { sourcePath: '/synthetic/original', destinationPath: '/synthetic/original', sourceRelativePath: 'original', destinationRelativePath: 'original', bytesMoved: '0', moved: false, message: 'Original noop receipt' } },
  { operation: 'notebook_edit', notebookEdit: { ...facts, editMode: 'replace', cellNumber: 0, cellCount: 3, cellType: 'code', cellId: 'actual-cell', message: 'Actual notebook receipt', firstChangedLine: 0, bytesWritten: '9007199254740993', diff } },
  { operation: 'delete_range', deleteRange: { ...facts, startLine: 7, endLine: 7, firstDeletedLine: 7, deletedLines: 1, bytesWritten: '3', encoding: 'utf-8', inclusive: true, diff } },
  { operation: 'delete_symbol', deleteSymbol: { ...facts, name: 'Actual', kind: 'function', line: 7, firstDeletedLine: 7, deletedLines: 1, bytesWritten: '3', encoding: 'utf-8', diff } }
]
const t = (key: string, values?: Record<string, unknown>) => key + (values ? ':' + JSON.stringify(values) : '')
const block = (name: string): ToolBlock => ({ kind: 'tool', id: selector.resultItemId, toolKind: fileResultToolKind(name), status: 'success', summary: 'PUBLIC_STATUS_ONLY', meta: { toolName: name, localResult: selector } })
const display = (result: { operation: string }, truncated = false) => toolResultLocalDisplaySchemaV1.parse({ ...base, toolName: result.operation,
  capture: { ...capture, truncated, fileResult: { ...result, classification: 'source_exact_local' } } })

describe('typed file-result cards', () => {
  it.each(results)('consumes only its actual $operation payload without public reconstruction', result => {
    const actual = display(result), owner = block(result.operation)
    const model = fileResultCardModel(owner, actual, t)
    expect(model).not.toBeNull()
    expect(JSON.stringify(model)).not.toContain(owner.summary)
    for (const changed of [{ ...owner, status: 'error' as const }, { ...owner, toolKind: 'command_execution' as const },
      { ...owner, meta: { ...owner.meta, toolName: 'unknown' } }, { ...owner, meta: { ...owner.meta, parentCallId: 'parent' } },
      { ...owner, meta: { ...owner.meta, localResult: { ...selector, resultItemId: 'item_result_' + 'd'.repeat(64) } } }]) {
      expect(fileResultCardModel(changed, actual, t)).toBeNull()
    }
  })
  it('preserves directory display order, real noop, zero-based cells and decimal bytes', () => {
    const list = fileResultCardModel(block('ls'), display(results[0]), t)!
    expect(list.items.map(item => item.title)).toEqual(['FIRST/', 'SECOND'])
    expect(list.producerTruncated).toBe(true)
    const move = fileResultCardModel(block('move_file'), display(results[3]), t)!
    expect(move.items[0].title).toBe('dshFileResult.unchanged')
    expect(move.items[0].subtitle).toBe('Original noop receipt')
    const notebook = fileResultCardModel(block('notebook_edit'), display(results[4]), t)!
    expect(notebook.items[0].fields).toContainEqual({ label: 'dshFileResult.cellNumber', value: '0' })
    expect(notebook.items[0].fields).toContainEqual({ label: 'dshFileResult.bytesWritten', value: '9007199254740993' })
    expect(notebook.diff?.patch).toBe('ORIGINAL_PATCH')
  })
  it('does not turn a missing oversized first record into an empty result or reduce original symbol count', () => {
    const result = { ...results[0], list: { ...results[0].list!, entries: [], names: [] } }
    const model = fileResultCardModel(block('ls'), display(result, true), t)!
    expect(model.empty).toBe('dshFileResult.captureUnavailable')
    expect(model.expandedSummary).toContain('"total":2')
    const index = { ...results[2], codeIndex: { ...results[2].codeIndex!, symbols: [] } }
    const code = fileResultCardModel(block('code_index'), display(index, true), t)!
    expect(code.expandedSummary).toContain('"total":1')
    expect(code.caption).toContain('"limit":1')
  })
})
