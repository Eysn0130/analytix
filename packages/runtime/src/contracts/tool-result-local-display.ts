import { z } from 'zod'
import { validateScheduleCalendarRule } from './schedule-rule.js'
import { ThreadGoalSchema, ThreadTodoItemSchema, ThreadTodoStatus, ThreadTodoStatusReasonCode } from './threads.js'
import { TaskJobKindV1Schema, TaskJobPublicIdV1Schema, TaskJobStatusV1Schema } from './task-job-output.js'

// Protected-local only. No producer, public event, history or model contract
// may embed this payload. Wire input carries identities, never body/path.
export const TOOL_RESULT_LOCAL_DISPLAY_PATH_V1 = '/v1/local-display/tool-result-snapshot'
export const TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1 = 256 << 10
export const TOOL_RESULT_LOCAL_TODO_DISPLAY_MAX_BYTES_V1 = 2 << 20
export const TOOL_RESULT_LOCAL_WEB_DISPLAY_MAX_BYTES_V1 = 8 << 20
export const TOOL_RESULT_LOCAL_WEB_BODY_MAX_BYTES_V1 = 1_000_000
// Complete current-file export is buffered only in Main; never reuse a preview prefix.
export const TOOL_RESULT_LOCAL_PRESENT_SAVE_MAX_BYTES_V1 = 64 << 20
const recordId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const text = (bytes: number) => z.string().refine(value =>
  !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= bytes)
export const toolResultLocalSelectorSchemaV1 = z.object({
  threadId: recordId, turnId: recordId,
  callId: z.string().regex(/^call_host_[a-f0-9]{64}$/),
  resultItemId: z.string().regex(/^item_result_[a-f0-9]{64}$/)
}).strict()
// Same protected snapshot endpoint and document lease; identities only.
export const toolResultLocalPresentHistoryRequestSchemaV1 = z.object({
  kind: z.literal('present_history'), threadId: recordId, turnId: recordId
}).strict()
export const toolResultLocalPresentHistoryViewRequestSchemaV1 = toolResultLocalPresentHistoryRequestSchemaV1.extend({
  viewId: z.string().uuid()
}).strict()
export const toolResultLocalPresentHistorySchemaV1 = z.object({
  schemaVersion: z.literal(1), kind: z.literal('present_history'), threadId: recordId, turnId: recordId,
  orderedItems: z.array(z.discriminatedUnion('kind', [
    z.object({ kind: z.literal('tool_result'), itemId: toolResultLocalSelectorSchemaV1.shape.resultItemId,
      callId: toolResultLocalSelectorSchemaV1.shape.callId, toolName: z.literal('present') }).strict(),
    z.object({ kind: z.literal('assistant_text'), itemId: recordId }).strict()
  ])).refine(items => new Set(items.map(item => item.itemId)).size === items.length &&
    new Set(items.flatMap(item => item.kind === 'tool_result' ? [item.callId] : [])).size ===
      items.filter(item => item.kind === 'tool_result').length)
}).strict()
export const toolResultLocalViewRequestSchemaV1 = toolResultLocalSelectorSchemaV1.extend({
  viewId: z.string().uuid()
}).strict()
export const toolResultLocalEffectRequestSchemaV1 = toolResultLocalViewRequestSchemaV1.extend({
  snapshotDigest: digest
}).strict()
// Current-file actions address one declaration in the original sealed receipt.
// Native application identity is observed by Main for this file, never a path.
export const toolResultLocalPresentFileRequestSchemaV1 = toolResultLocalEffectRequestSchemaV1.extend({
  index: z.number().int().min(0).max(7)
}).strict()
// The existing save bridge exports either projected result text or one complete
// current Present file. Both branches are closed identity-only requests.
export const toolResultLocalSaveRequestSchemaV1 = z.union([
  toolResultLocalEffectRequestSchemaV1, toolResultLocalPresentFileRequestSchemaV1
])
// Bounded opaque Office candidates are resolved only by the current Host lease.
export const toolResultLocalPresentAssociationRequestSchemaV1 = toolResultLocalPresentFileRequestSchemaV1.extend({
  artifactIds: z.array(z.string().regex(/^[a-f0-9]{64}$/)).max(32)
    .refine(values => new Set(values).size === values.length)
}).strict()
const presentApplicationId = text(4096).refine(value => value.trim() !== '' && !value.includes('\0'))
export const toolResultLocalPresentOpenRequestSchemaV1 = z.union([
  toolResultLocalPresentFileRequestSchemaV1.extend({ applicationId: presentApplicationId.optional() }).strict(),
  // Reveal uses the same sealed declaration, independently of OS associations.
  toolResultLocalPresentFileRequestSchemaV1.extend({ action: z.literal('reveal') }).strict()
])
const ordinaryCaptureSchemaV1 = z.object({
  kind: z.enum(['shell', 'read']),
  status: z.enum(['completed', 'failed', 'timeout', 'canceled', 'unknown']),
  body: text(32 << 10), label: text(1024).refine(value => !value.includes('\0')),
  exitCode: z.number().int().nonnegative().optional(),
  durationMs: z.number().int().nonnegative(), truncated: z.boolean(),
  startLine: z.number().int().nonnegative(), endLine: z.number().int().nonnegative(),
  totalLines: z.number().int().nonnegative()
}).strict().superRefine((capture, context) => {
  if (capture.kind === 'shell'
    ? capture.startLine !== 0 || capture.endLine !== 0 || capture.totalLines !== 0
    : capture.exitCode !== undefined || capture.durationMs !== 0 || capture.status !== 'completed' ||
      capture.endLine < capture.startLine || capture.totalLines > 0 && capture.endLine > capture.totalLines && capture.body !== '') {
    context.addIssue({ code: 'custom', message: 'Invalid protected capture.' })
  }
})
// Web uses its producer's existing byte cap. Shell/Read keep the original
// 32 KiB body and 256 KiB envelope budgets.
const webUrl = text(8192).refine(value => value !== '' && !value.includes('\0'))
const httpTarget = webUrl.refine(value => !/\p{Cc}/u.test(value)).refine(value => {
  try { const url = new URL(value); return (url.protocol === 'http:' || url.protocol === 'https:') && url.hostname !== '' && url.username === '' && url.password === '' }
  catch { return false }
})
const jsonObjectText = text(65536).refine(value => {
  try { const parsed: unknown = JSON.parse(value); return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) }
  catch { return false }
})
const eventTime = text(64).refine(value => /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value)))
function eventTimeNanoseconds(value: string): bigint | null {
  const fractional = /\.(\d{1,9})(?=Z|[+-]\d{2}:\d{2}$)/.exec(value)?.[1] ?? ''
  const seconds = value.replace(/\.\d{1,9}(?=Z|[+-]\d{2}:\d{2}$)/, '')
  const milliseconds = Date.parse(seconds)
  return Number.isFinite(milliseconds) ? BigInt(milliseconds) * 1_000_000n + BigInt(fractional.padEnd(9, '0')) : null
}
function matchesInspectEventDuration(value: { callEventAt: string | null; resultEventAt: string | null; durationMs: number | null }): boolean {
  const call = value.callEventAt === null ? null : eventTimeNanoseconds(value.callEventAt)
  const result = value.resultEventAt === null ? null : eventTimeNanoseconds(value.resultEventAt)
  if (value.durationMs === null) return call === null || result === null || result < call
  if (!Number.isSafeInteger(value.durationMs) || value.durationMs < 0) return false
  return call !== null && result !== null && result >= call && (result - call) / 1_000_000n === BigInt(value.durationMs)
}
// Only the original request supplies input/definition. Missing bytes and
// event times are unavailable, never reconstructed from a current catalog.
const protectedOutputError = z.discriminatedUnion('availability', [
  z.object({ availability: z.literal('available'), entries: z.array(z.object({
    message: text(32 << 10).refine(value => value.trim().length > 0 && !value.includes('\0')),
    classification: z.enum(['source_exact_local', 'credential_redacted']),
    parallelIndex: z.number().int().positive().safe().optional(), jobId: TaskJobPublicIdV1Schema.optional()
  }).strict()).min(1).max(50).refine(entries =>
    entries.reduce((size, entry) => size + new TextEncoder().encode(entry.message).length, 0) <= (32 << 10) &&
    (entries.length === 1 || entries.every(entry => entry.parallelIndex !== undefined)) &&
    new Set(entries.filter(entry => entry.parallelIndex !== undefined).map(entry => entry.parallelIndex)).size === entries.filter(entry => entry.parallelIndex !== undefined).length
  ) }).strict(),
  z.object({ availability: z.literal('unavailable'), reason: z.enum(['missing', 'invalid', 'limit', 'canceled']) }).strict()
])
const protectedOutputText = z.discriminatedUnion('availability', [
  z.object({ availability: z.literal('available'), body: text(32 << 10).refine(value => value.trim() !== '' && !value.includes('\0')),
    classification: z.enum(['source_exact_local', 'credential_redacted']), truncated: z.boolean() }).strict(),
  z.object({ availability: z.literal('unavailable'), reason: z.enum(['limit', 'invalid']), truncated: z.literal(false) }).strict()
])
export const toolResultLocalInspectSchemaV1 = z.object({
  outputText: protectedOutputText.optional(),
  outputError: protectedOutputError.optional(),
  inputJSON: jsonObjectText.optional(), schemaJSON: jsonObjectText.optional(),
  inputTruncated: z.boolean(), schemaTruncated: z.boolean(),
  callEventAt: eventTime.nullable(), resultEventAt: eventTime.nullable(),
  durationMs: z.number().int().nonnegative().safe().nullable()
}).strict().refine(value =>
  (!value.inputTruncated || value.inputJSON === undefined) && (!value.schemaTruncated || value.schemaJSON === undefined) &&
  (!value.outputText || !value.outputError) && matchesInspectEventDuration(value))
export type ToolResultLocalInspectV1 = z.infer<typeof toolResultLocalInspectSchemaV1>
const webCaptureDataSchemaV1 = z.object({
  requestUrl: webUrl.optional(), finalUrl: webUrl.optional(),
  requestUrlTruncated: z.boolean(), finalUrlTruncated: z.boolean(),
  inputJSON: jsonObjectText.optional(), schemaJSON: jsonObjectText.optional(),
  inputTruncated: z.boolean(), schemaTruncated: z.boolean(),
  title: text(4096).optional(), statusCode: z.number().int().min(100).max(599).optional(),
  fetchTruncated: z.boolean().optional(), responseComplete: z.boolean(), toolIsError: z.boolean(),
  error: z.object({ code: text(256).refine(value => value !== ''), message: text(32 << 10) }).strict().optional(),
  startedAt: text(64).refine(value => /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value)))
}).strict().refine(web =>
  (!web.requestUrlTruncated || web.requestUrl === undefined) &&
  (!web.finalUrlTruncated || web.finalUrl === undefined) &&
  (!web.inputTruncated || web.inputJSON === undefined) &&
  (!web.schemaTruncated || web.schemaJSON === undefined) &&
  (web.responseComplete ? web.statusCode !== undefined && web.fetchTruncated !== undefined : web.fetchTruncated === undefined && web.toolIsError) &&
  (web.error === undefined || web.toolIsError))
const webCaptureSchemaV1 = z.object({
  kind: z.literal('web_fetch'), status: z.enum(['completed', 'failed', 'timeout', 'canceled', 'unknown']),
  body: text(TOOL_RESULT_LOCAL_WEB_BODY_MAX_BYTES_V1), label: text(1024).refine(value => !value.includes('\0')),
  durationMs: z.number().int().nonnegative(), truncated: z.boolean(),
  startLine: z.literal(0), endLine: z.literal(0), totalLines: z.literal(0), web: webCaptureDataSchemaV1
}).strict().refine(capture => (capture.status === 'completed') === capture.web.responseComplete)
const searchPath = text(4096).refine(value => value !== '' && !/[\0\r\n]/.test(value))
const searchLine = text(32 << 10).refine(value => !/[\0\r\n]/.test(value))
const searchMatchSchemaV1 = z.object({
  lineNumber: z.number().int().positive(), line: searchLine, column: z.number().int().positive(),
  contextBefore: z.array(searchLine).max(20), contextAfter: z.array(searchLine).max(20)
}).strict().refine(match => match.contextBefore.length < match.lineNumber)
const searchFileSchemaV1 = z.object({ path: searchPath, matches: z.array(searchMatchSchemaV1).min(1).max(500) }).strict()
  .refine(file => file.matches.every((match, index) => index === 0 || match.lineNumber > file.matches[index - 1].lineNumber))
const searchDataShape = {
  total: z.number().int().nonnegative().nullable(), truncated: z.boolean(), toolIsError: z.boolean(),
  inputJSON: jsonObjectText.optional(), inputTruncated: z.boolean(),
  startedAt: text(64).refine(value => /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value)))
}
export const toolResultLocalSearchDataSchemaV1 = z.discriminatedUnion('shape', [
  z.object({ ...searchDataShape, shape: z.literal('matches'), files: z.array(searchFileSchemaV1).max(500).default([]) }).strict(),
  z.object({ ...searchDataShape, shape: z.literal('paths'), paths: z.array(searchPath).max(1000).default([]) }).strict()
]).refine(search => {
  const paths = search.shape === 'paths' ? search.paths : search.files.map(file => file.path)
  const shown = search.shape === 'paths' ? search.paths.length : search.files.reduce((sum, file) => sum + file.matches.length, 0)
  return new Set(paths).size === paths.length && shown <= (search.shape === 'paths' ? 1000 : 500) &&
    (search.total === null || search.total >= shown && !search.truncated) &&
    (!search.inputTruncated || search.inputJSON === undefined)
})
export type ToolResultLocalSearchDataV1 = z.infer<typeof toolResultLocalSearchDataSchemaV1>
/** Complete sealed copy, independent of all UI disclosure. Paths are display text. */
export function toolResultLocalSearchTextV1(search: ToolResultLocalSearchDataV1): string {
  if (search.shape === 'paths') return search.paths.join('\n')
  return search.files.map(file => [file.path, ...file.matches.flatMap(match => [
    ...match.contextBefore.map((line, index) => `${match.lineNumber - match.contextBefore.length + index}- ${line}`),
    `${match.lineNumber}: ${match.line}`,
    ...match.contextAfter.map((line, index) => `${match.lineNumber + index + 1}- ${line}`)
  ])].join('\n')).join('\n\n')
}
const searchCaptureSchemaV1 = z.object({
  kind: z.literal('search'), status: z.enum(['completed', 'failed', 'timeout', 'canceled', 'unknown']),
  body: text(32 << 10), label: text(1024).refine(value => !value.includes('\0')),
  durationMs: z.number().int().nonnegative(), truncated: z.boolean(),
  startLine: z.literal(0), endLine: z.literal(0), totalLines: z.literal(0), search: toolResultLocalSearchDataSchemaV1
}).strict().refine(capture => {
  const search = capture.search
  const shown = search.shape === 'paths' ? search.paths.length : search.files.reduce((sum, file) => sum + file.matches.length, 0)
  return (!search.toolIsError || capture.status !== 'completed') && (search.toolIsError ||
    capture.body === toolResultLocalSearchTextV1(search) && (search.total === null ? search.truncated : search.total === shown || capture.truncated))
})
const domainText = (maxBytes: number, nonempty = false, maxUnits?: number) => text(maxBytes).refine(value =>
  !value.includes('\0') && (!nonempty || value.trim() !== '') && (maxUnits === undefined || value.length <= maxUnits))
// Only these existing effects supply successful file results. Original counts,
// producer limits and local capture omissions have separate meanings. Paths
// remain display text and grant no navigation or export authority.
const fileResultCount = z.number().int().safe().nonnegative()
const fileResultText = domainText(32 << 10)
const fileResultPath = domainText(4096)
const fileResultLocation = domainText(4096, true)
const fileResultBytes = z.string().regex(/^(?:0|[1-9][0-9]{0,18})$/).refine(value => BigInt(value) <= 9223372036854775807n)
const fileResultDiff = z.object({
  kind: z.literal('modify'), added: fileResultCount, removed: fileResultCount,
  patch: fileResultText.optional(), binary: z.boolean(), truncated: z.boolean(),
  omittedReason: z.enum(['binary', 'large_change']).optional(), patchUnavailable: z.literal('limit').optional()
}).strict().refine(value =>
  !(value.patch !== undefined && value.patchUnavailable !== undefined) &&
  (value.binary ? value.omittedReason === 'binary' && value.patch === undefined && !value.truncated :
    value.truncated ? value.omittedReason === 'large_change' : value.omittedReason === undefined))
const fileResultList = z.object({
  path: fileResultLocation, relativePath: fileResultLocation,
  entries: z.array(z.object({ name: fileResultLocation, displayName: fileResultLocation,
    kind: z.enum(['file', 'directory']) }).strict()).max(1000), names: z.array(fileResultLocation).max(1000),
  limit: fileResultCount.min(1).max(1000), producerTruncated: z.boolean(), originalCount: fileResultCount.max(1000)
}).strict().refine(value => value.entries.length === value.names.length && value.entries.length <= value.originalCount &&
  value.originalCount <= value.limit && value.names.every((name, index) => name === value.entries[index].displayName))
const fileResultFind = z.object({
  path: fileResultLocation, relativePath: fileResultLocation, pattern: domainText(32 << 10, true),
  matches: z.array(z.object({ path: fileResultLocation, relativePath: fileResultLocation }).strict()).max(1000),
  skippedProtected: z.array(fileResultLocation), skippedProtectedOriginalCount: fileResultCount,
  limit: fileResultCount.min(1).max(1000), producerTruncated: z.boolean(), originalCount: fileResultCount.max(1000)
}).strict().refine(value => value.matches.length <= value.originalCount && value.originalCount <= value.limit &&
  value.skippedProtected.length <= value.skippedProtectedOriginalCount)
const fileResultCodeIndex = z.object({
  action: z.enum(['outline', 'search']), path: fileResultLocation, relativePath: fileResultLocation,
  query: fileResultText, kind: fileResultPath, limit: fileResultCount.min(1).max(200),
  symbols: z.array(z.object({ name: fileResultPath, kind: fileResultPath, file: fileResultPath, path: fileResultPath,
    line: fileResultCount.positive(), parent: fileResultPath, signature: fileResultText }).strict()).max(200),
  symbolCount: fileResultCount.max(200), originalCount: fileResultCount.max(200),
  text: fileResultText.optional(), textUnavailable: z.literal('limit').optional(),
  skippedProtected: z.array(fileResultLocation), skippedProtectedOriginalCount: fileResultCount,
  skippedDirectories: z.array(fileResultLocation), skippedDirectoriesOriginalCount: fileResultCount,
  producerTruncated: z.boolean(), limitReached: z.boolean()
}).strict().refine(value => value.symbolCount === value.originalCount && value.symbols.length <= value.originalCount &&
  value.originalCount <= value.limit && value.limitReached === (value.originalCount >= value.limit) &&
  (value.text !== undefined) !== (value.textUnavailable !== undefined) &&
  value.skippedProtected.length <= value.skippedProtectedOriginalCount && value.skippedDirectories.length <= value.skippedDirectoriesOriginalCount)
const fileResultMove = z.object({
  sourcePath: fileResultLocation, destinationPath: fileResultLocation, sourceRelativePath: fileResultLocation, destinationRelativePath: fileResultLocation,
  bytesMoved: fileResultBytes, moved: z.boolean(), message: fileResultText
}).strict()
const fileResultNotebookEdit = z.object({
  path: fileResultLocation, relativePath: fileResultLocation, editMode: z.enum(['insert', 'replace', 'delete']),
  cellNumber: fileResultCount, cellCount: fileResultCount, cellType: fileResultPath, cellId: fileResultLocation.optional(),
  message: fileResultText, bytesWritten: fileResultBytes, firstChangedLine: fileResultCount, diff: fileResultDiff
}).strict()
const fileResultDeleteRange = z.object({
  path: fileResultLocation, relativePath: fileResultLocation, startLine: fileResultCount.positive(), endLine: fileResultCount.positive(),
  firstDeletedLine: fileResultCount.positive(), deletedLines: fileResultCount, bytesWritten: fileResultBytes,
  encoding: fileResultPath, inclusive: z.boolean(), diff: fileResultDiff
}).strict().refine(value => value.endLine >= value.startLine && (value.inclusive
  ? value.firstDeletedLine === value.startLine && value.deletedLines === value.endLine - value.startLine + 1
  : value.endLine > value.startLine && value.firstDeletedLine === value.startLine + 1 && value.deletedLines === value.endLine - value.startLine - 1))
const fileResultDeleteSymbol = z.object({
  path: fileResultLocation, relativePath: fileResultLocation, name: fileResultPath, kind: fileResultPath, parent: fileResultPath.optional(),
  line: fileResultCount.positive(), firstDeletedLine: fileResultCount.positive(), deletedLines: fileResultCount,
  bytesWritten: fileResultBytes, encoding: fileResultPath, diff: fileResultDiff
}).strict()
const fileResultBase = { classification: z.enum(['source_exact_local', 'credential_redacted']) }
export const toolResultLocalFileResultSchemaV1 = z.discriminatedUnion('operation', [
  z.object({ ...fileResultBase, operation: z.literal('ls'), list: fileResultList }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('find'), find: fileResultFind }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('code_index'), codeIndex: fileResultCodeIndex }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('move_file'), move: fileResultMove }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('notebook_edit'), notebookEdit: fileResultNotebookEdit }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('delete_range'), deleteRange: fileResultDeleteRange }).strict(),
  z.object({ ...fileResultBase, operation: z.literal('delete_symbol'), deleteSymbol: fileResultDeleteSymbol }).strict()
]).refine(value => new TextEncoder().encode(JSON.stringify(value)).length <= (32 << 10))
export type ToolResultLocalFileResultV1 = z.infer<typeof toolResultLocalFileResultSchemaV1>
const fileResultCaptureSchemaV1 = z.object({
  kind: z.literal('file_result'), status: z.literal('completed'), body: z.literal(''), label: z.literal(''),
  durationMs: z.literal(0), startLine: z.literal(0), endLine: z.literal(0), totalLines: z.literal(0),
  truncated: z.boolean(), fileResult: toolResultLocalFileResultSchemaV1
}).strict().refine(capture => {
  const value = capture.fileResult
  const omitted = value.operation === 'ls' ? value.list.entries.length < value.list.originalCount :
    value.operation === 'find' ? value.find.matches.length < value.find.originalCount || value.find.skippedProtected.length < value.find.skippedProtectedOriginalCount :
    value.operation === 'code_index' ? value.codeIndex.symbols.length < value.codeIndex.originalCount || value.codeIndex.textUnavailable !== undefined ||
      value.codeIndex.skippedProtected.length < value.codeIndex.skippedProtectedOriginalCount || value.codeIndex.skippedDirectories.length < value.codeIndex.skippedDirectoriesOriginalCount :
    value.operation === 'move_file' ? false : (value.operation === 'notebook_edit' ? value.notebookEdit :
      value.operation === 'delete_range' ? value.deleteRange : value.deleteSymbol).diff.patchUnavailable !== undefined
  return capture.truncated === omitted
})
export const toolResultLocalGoalRecordSchemaV1 = ThreadGoalSchema.pick({
  id: true, revision: true, maxGoalRounds: true, roundsStarted: true, threadId: true, objective: true, status: true, tokenBudget: true,
  tokensUsed: true, timeUsedSeconds: true, blockedReason: true, createdAt: true, updatedAt: true
}).extend({
  id: domainText(4096).optional(), threadId: recordId, objective: domainText(16000, true, 4000),
  blockedReason: domainText(4000, false, 1000).optional(), createdAt: domainText(64, true), updatedAt: domainText(64, true)
}).strict()
export const toolResultLocalGoalDataSchemaV1 = z.object({
  goal: toolResultLocalGoalRecordSchemaV1.nullable(),
  remainingTokens: z.number().int().nonnegative().nullable()
}).strict().refine(value => value.goal === null ? value.remainingTokens === null :
  typeof value.goal.tokenBudget === 'number' ?
    value.remainingTokens === Math.max(0, value.goal.tokenBudget - value.goal.tokensUsed) : value.remainingTokens === null)
export const toolResultLocalTodoItemSchemaV1 = z.object({
  id: domainText(4096, true), content: domainText(4000, true, 1000),
  status: ThreadTodoStatus, statusReasonCode: ThreadTodoStatusReasonCode.optional(),
  createdAt: domainText(64, true), updatedAt: domainText(64, true)
}).strict().refine(value => ThreadTodoItemSchema.safeParse(value).success)
export const toolResultLocalTodoListSchemaV1 = z.object({
  threadId: recordId, turnId: recordId.optional(), items: z.array(toolResultLocalTodoItemSchemaV1).max(200), updatedAt: domainText(64, true)
}).strict().refine(value => new Set(value.items.map(item => item.id)).size === value.items.length)
export const toolResultLocalTodoDataSchemaV1 = z.object({
  todos: toolResultLocalTodoListSchemaV1,
  baseline: z.discriminatedUnion('availability', [
    z.object({ availability: z.literal('known'), todos: toolResultLocalTodoListSchemaV1.nullable() }).strict(),
    z.object({ availability: z.literal('unavailable') }).strict()
  ])
}).strict().refine(value => value.baseline.availability !== 'known' || value.baseline.todos === null ||
  value.baseline.todos.threadId === value.todos.threadId)
export const toolResultLocalSkillDataSchemaV1 = z.object({
  requestedName: domainText(1024),
  resolved: z.object({
    id: domainText(1024, true), name: domainText(1024, true),
    runAs: z.enum(['inline', 'subagent']), packageDigest: digest
  }).strict().nullable(),
  child: z.object({
    jobId: TaskJobPublicIdV1Schema, childThreadId: recordId.nullable(), childTurnId: recordId.nullable(),
    status: TaskJobStatusV1Schema, background: z.boolean(), outputWithheld: z.literal(true)
  }).strict().nullable(),
  error: z.object({ code: domainText(256, true), message: domainText(32 << 10) }).strict().nullable()
}).strict().refine(value => value.child === null || value.resolved?.runAs === 'subagent')
// Reserved legacy text/measurement fields have no domain meaning. Domain data
// is typed; no arbitrary result JSON or requested state is an observation.
const domainCaptureBase = {
  label: z.literal(''), durationMs: z.literal(0), startLine: z.literal(0), endLine: z.literal(0), totalLines: z.literal(0)
}
const goalCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('goal'), status: z.literal('completed'),
  body: z.literal(''), truncated: z.literal(false), goal: toolResultLocalGoalDataSchemaV1 }).strict()
const todoCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('todo'), status: z.literal('completed'),
  body: z.literal(''), truncated: z.literal(false), todo: toolResultLocalTodoDataSchemaV1 }).strict()
const skillCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('skill'), status: z.enum(['completed', 'failed']),
  body: text(32 << 10), truncated: z.boolean(), skill: toolResultLocalSkillDataSchemaV1 }).strict().refine(value =>
  (value.status !== 'completed' || value.skill.resolved !== null && value.skill.error === null) &&
  (value.skill.resolved?.runAs === 'inline' || value.body === '' && !value.truncated))
export const toolResultLocalJobReceiptSchemaV1 = z.discriminatedUnion('operation', [
  z.object({ operation: z.literal('cancel'), status: z.enum(['cancellation-requested', 'already-finished']) }).strict(),
  z.object({ operation: z.literal('wait'), status: z.enum(['completed', 'timeout']) }).strict()
])
export const toolResultLocalJobRecordSchemaV1 = z.object({
  jobId: TaskJobPublicIdV1Schema, kind: TaskJobKindV1Schema, status: TaskJobStatusV1Schema,
  name: domainText(1024).nullable(), label: domainText(1024).nullable(),
  output: z.discriminatedUnion('availability', [
    z.object({ availability: z.literal('available'), body: text(32 << 10), truncated: z.boolean() }).strict(),
    z.object({ availability: z.literal('unavailable') }).strict()
  ])
}).strict()
export const toolResultLocalJobDataSchemaV1 = z.object({
  operation: z.enum(['spawn', 'list', 'wait', 'output', 'cancel', 'restart']),
  jobs: z.array(toolResultLocalJobRecordSchemaV1).max(50), total: z.number().int().nonnegative(), truncated: z.boolean(),
  receipt: toolResultLocalJobReceiptSchemaV1.nullable()
}).strict().refine(value =>
  value.total >= value.jobs.length && value.truncated === (value.total > value.jobs.length) &&
  new Set(value.jobs.map(job => job.jobId)).size === value.jobs.length &&
  (value.receipt === null || value.receipt.operation === value.operation) &&
  (value.operation === 'output' || value.jobs.every(job => job.output.availability === 'unavailable')))
const jobCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('job'), status: z.enum(['completed', 'failed']),
  body: z.literal(''), truncated: z.literal(false), job: toolResultLocalJobDataSchemaV1 }).strict()
// This is the native Host's committed receipt, not a current task catalog or
// a DSH wire record. All task text stays behind the protected-local boundary.
const scheduleText = text(TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1)
export const toolResultLocalScheduleTaskSchemaV1 = z.object({
  id: scheduleText.refine(value => value !== '' && value.trim() === value), title: scheduleText, enabled: z.boolean(),
  prompt: scheduleText, workspaceRoot: scheduleText, clawChannelId: scheduleText, providerId: scheduleText.optional(),
  model: scheduleText, reasoningEffort: z.enum(['auto', 'off', 'low', 'medium', 'high', 'max']),
  mode: z.enum(['agent', 'plan']),
  schedule: z.object({ kind: z.enum(['manual', 'at', 'daily', 'interval', 'weekly', 'cron']), everyMinutes: z.number().finite(),
    timeOfDay: scheduleText, atTime: scheduleText, timeZone: scheduleText.optional(),
    weekdays: z.array(z.number().int().min(1).max(7)).max(7).optional(), expression: scheduleText.optional()
  }).strict().superRefine((value, ctx) => {
    try { validateScheduleCalendarRule(value, true) }
    catch { ctx.addIssue({ code: 'custom', message: 'Invalid committed calendar rule.' }) }
  }),
  createdAt: scheduleText, updatedAt: scheduleText, lastRunAt: scheduleText, nextRunAt: scheduleText,
  lastStatus: z.enum(['idle', 'running', 'success', 'error']), lastMessage: scheduleText, lastThreadId: scheduleText
}).strict()
export const toolResultLocalScheduleReceiptSchemaV1 = z.union([
  z.object({ operation: z.enum(['create', 'update']), status: z.literal('success'), message: scheduleText,
    task: toolResultLocalScheduleTaskSchemaV1 }).strict(),
  z.object({ operation: z.enum(['create', 'update']), status: z.literal('failure'), message: scheduleText,
    task: z.null() }).strict(),
  z.object({ operation: z.literal('delete'), status: z.enum(['success', 'failure']), message: scheduleText,
    taskId: scheduleText }).strict()
]).refine(value => new TextEncoder().encode(JSON.stringify(value)).length <= TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1)
const scheduleCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('schedule'),
  status: z.enum(['completed', 'failed']), body: z.literal(''), truncated: z.literal(false),
  schedule: toolResultLocalScheduleReceiptSchemaV1 }).strict().refine(value =>
  (value.status === 'completed') === (value.schedule.status === 'success'))
// Present is a committed declaration. Repeated paths and optional empty
// descriptions retain their original per-call indices and text.
export const toolResultLocalPresentReceiptSchemaV1 = z.object({
  files: z.array(z.object({
    index: z.number().int().min(0).max(7), path: domainText(4096).refine(value => !/^\p{White_Space}*$/u.test(value)), description: domainText(4096).optional()
  }).strict()).min(1).max(8).refine(files => files.every((file, index) => file.index === index))
}).strict()
const presentCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('present'), status: z.literal('completed'),
  body: z.literal(''), truncated: z.literal(false), present: toolResultLocalPresentReceiptSchemaV1 }).strict()
const inspectCaptureSchemaV1 = z.object({ ...domainCaptureBase, kind: z.literal('inspect'),
  status: z.enum(['completed', 'failed', 'timeout', 'canceled', 'unknown']), body: z.literal(''), truncated: z.literal(false) }).strict()
export type ToolResultLocalJobDataV1 = z.infer<typeof toolResultLocalJobDataSchemaV1>
export type ToolResultLocalGoalDataV1 = z.infer<typeof toolResultLocalGoalDataSchemaV1>
export type ToolResultLocalTodoDataV1 = z.infer<typeof toolResultLocalTodoDataSchemaV1>
export type ToolResultLocalSkillDataV1 = z.infer<typeof toolResultLocalSkillDataSchemaV1>
export type ToolResultLocalScheduleTaskV1 = z.infer<typeof toolResultLocalScheduleTaskSchemaV1>
export type ToolResultLocalScheduleReceiptV1 = z.infer<typeof toolResultLocalScheduleReceiptSchemaV1>
export type ToolResultLocalPresentReceiptV1 = z.infer<typeof toolResultLocalPresentReceiptSchemaV1>
export const toolResultLocalCaptureSchemaV1 = z.union([ordinaryCaptureSchemaV1, webCaptureSchemaV1, searchCaptureSchemaV1,
  goalCaptureSchemaV1, todoCaptureSchemaV1, skillCaptureSchemaV1, jobCaptureSchemaV1, scheduleCaptureSchemaV1, presentCaptureSchemaV1, inspectCaptureSchemaV1, fileResultCaptureSchemaV1])
export const toolResultLocalWebTargetsSchemaV1 = z.object({
  requestUrl: httpTarget.optional(), resultUrl: httpTarget.optional()
}).strict()
const excelCoordinateV1 = z.number().int().min(0).max(249_999)
const excelRangeSchemaV1 = z.object({ row: z.tuple([excelCoordinateV1, excelCoordinateV1]),
  column: z.tuple([excelCoordinateV1, excelCoordinateV1]) }).strict().refine(range =>
    range.row[0] <= range.row[1] && range.column[0] <= range.column[1])
export const toolResultLocalPresentExcelSelectionSchemaV1 = z.object({
  sheetIndex: excelCoordinateV1,
  sheetId: text(256).refine(value => value !== '' && !value.includes('\0')),
  ranges: z.array(excelRangeSchemaV1).min(1).max(32)
}).strict().refine(({ ranges }) => {
  const first = ranges[0]
  const sameRows = ranges.every(range => range.row[0] === first.row[0] && range.row[1] === first.row[1])
  const sameColumns = ranges.every(range => range.column[0] === first.column[0] && range.column[1] === first.column[1])
  if (!sameRows && !sameColumns) return false
  let area = 0
  for (let index = 0; index < ranges.length; index++) {
    const range = ranges[index]
    area += (range.row[1] - range.row[0] + 1) * (range.column[1] - range.column[0] + 1)
    if (!Number.isSafeInteger(area) || area > 250_000) return false
    for (let previous = 0; previous < index; previous++) {
      const other = ranges[previous]
      if (range.row[0] <= other.row[1] && other.row[0] <= range.row[1] &&
          range.column[0] <= other.column[1] && other.column[0] <= range.column[1]) return false
    }
  }
  return true
})
export const toolResultLocalPresentExcelCopyRequestSchemaV1 = toolResultLocalPresentFileRequestSchemaV1.extend({
  contentDigest: digest, selection: toolResultLocalPresentExcelSelectionSchemaV1
}).strict()
export const toolResultLocalCopyRequestSchemaV1 = z.union([
  toolResultLocalEffectRequestSchemaV1.extend({
    section: z.enum(['payload', 'result', 'schema', 'request-url', 'result-url']).optional()
  }).strict(), toolResultLocalPresentExcelCopyRequestSchemaV1
])
export const toolResultLocalWebLinkRequestSchemaV1 = toolResultLocalEffectRequestSchemaV1.extend({
  targetRole: z.enum(['request', 'result'])
}).strict()
const readTargetPath = text(4096).refine(value => value !== '' && !value.includes('\0'))
export const toolResultLocalReadTargetSchemaV1 = z.object({
  path: readTargetPath, workspaceRoot: readTargetPath,
  startLine: z.number().int().positive(), contentDigest: digest
}).strict()
// Paths are Host-only observations. Main validates native absolute/clean paths
// and current file identity before every effect; renderer requests carry index.
export const toolResultLocalPresentTargetsSchemaV1 = z.object({
  workspaceRoot: readTargetPath,
  files: z.array(z.object({ index: z.number().int().min(0).max(7), path: domainText(4096, true) }).strict())
    .min(1).max(8).refine(files => files.every((file, index) => file.index === index))
}).strict()
const displayShape = {
  schemaVersion: z.literal(1), snapshotDigest: digest,
  resultItemId: z.string().regex(/^item_result_[a-f0-9]{64}$/),
  toolName: domainText(256, true).refine(value => value.trim() === value), capture: toolResultLocalCaptureSchemaV1,
  inspect: toolResultLocalInspectSchemaV1.optional()
}
const jobOperationForTool: Record<string, string> = { task: 'spawn', delegate_task: 'spawn', parallel_tasks: 'spawn',
  list_jobs: 'list', wait: 'wait', bash_output: 'output', kill_shell: 'cancel', restart_job: 'restart' }
const scheduleOperationForTool: Record<string, string> = {
  mcp__gui_schedule__gui_schedule_create: 'create', mcp__gui_schedule__claw_schedule_create: 'create',
  mcp__gui_schedule__gui_schedule_update: 'update', mcp__gui_schedule__claw_schedule_update: 'update',
  mcp__gui_schedule__gui_schedule_delete: 'delete', mcp__gui_schedule__claw_schedule_delete: 'delete'
}
const validKind = (value: { toolName: string; capture: { kind: string; search?: { shape: string };
  job?: { operation: string }; schedule?: { operation: string }; fileResult?: { operation: string } } }) =>
  value.capture.kind === 'file_result' ? value.toolName === value.capture.fileResult?.operation :
  value.capture.kind === 'present' ? value.toolName === 'present' :
  value.capture.kind === 'schedule' ? value.capture.schedule?.operation === scheduleOperationForTool[value.toolName] :
  value.capture.kind === 'inspect' || value.capture.kind === 'job' ? value.capture.kind === 'inspect' ||
    value.capture.job?.operation === jobOperationForTool[value.toolName] :
  value.toolName === 'bash' ? value.capture.kind === 'shell' :
    value.toolName === 'web_fetch' ? value.capture.kind === 'web_fetch' :
      value.toolName === 'create_goal' || value.toolName === 'get_goal' || value.toolName === 'update_goal' ? value.capture.kind === 'goal' :
        value.toolName === 'todo_write' ? value.capture.kind === 'todo' : value.toolName === 'run_skill' ? value.capture.kind === 'skill' :
      value.toolName === 'grep' || value.toolName === 'glob' ? value.capture.kind === 'search' &&
        value.capture.search?.shape === (value.toolName === 'grep' ? 'matches' : 'paths') :
          (value.toolName === 'read' || value.toolName === 'read_file') && value.capture.kind === 'read'
const validInspect = (value: { toolName: string; capture: { kind: string; status: string;
  search?: { inputJSON?: string; inputTruncated: boolean; toolIsError?: boolean };
  web?: { inputJSON?: string; inputTruncated: boolean; schemaJSON?: string; schemaTruncated: boolean; toolIsError?: boolean }
}; inspect?: ToolResultLocalInspectV1 }) => {
  const { search, web } = value.capture
  if (value.inspect?.outputText && (value.capture.kind !== 'inspect' || value.capture.status !== 'completed' ||
    !value.toolName.startsWith('mcp__') || value.inspect.outputError)) return false
  if (value.inspect?.outputError && !['failed', 'timeout', 'canceled'].includes(value.capture.status) && !search?.toolIsError && !web?.toolIsError) return false
  if (value.inspect) {
    if (value.inspect.inputJSON === undefined && !value.inspect.inputTruncated) return false
    if (search && (search.inputJSON !== undefined || search.inputTruncated) ||
      web && (web.inputJSON !== undefined || web.inputTruncated || web.schemaJSON !== undefined || web.schemaTruncated)) return false
  } else if (search && (search.inputJSON === undefined) !== search.inputTruncated ||
    web && web.inputJSON === undefined && !web.inputTruncated) return false
  if ((value.capture.kind === 'present' || value.capture.kind === 'file_result') && !value.inspect) return false
  if (value.capture.kind === 'inspect' && (!value.inspect || value.inspect.inputJSON === undefined && !value.inspect.inputTruncated)) return false
  if (value.inspect?.schemaJSON === undefined) return true
  try {
    const schema = JSON.parse(value.inspect.schemaJSON) as Record<string, unknown>
    return schema.name === value.toolName && typeof schema.description === 'string' &&
      schema.parameters !== null && typeof schema.parameters === 'object' && !Array.isArray(schema.parameters) &&
      Object.keys(schema).length === 3 && Object.keys(schema).every(key => ['name', 'description', 'parameters'].includes(key))
  } catch { return false }
}
// Only Main consumes protected navigation targets; Search paths are display text.
export const toolResultLocalHostDisplaySchemaV1 = z.object({
  ...displayShape, readTarget: toolResultLocalReadTargetSchemaV1.optional(), webTargets: toolResultLocalWebTargetsSchemaV1.optional(),
  presentTargets: toolResultLocalPresentTargetsSchemaV1.optional()
}).strict().refine(value => validKind(value) && validInspect(value) && (!value.readTarget || value.capture.kind === 'read' &&
  value.readTarget.startLine === Math.max(1, value.capture.startLine)) &&
  (!value.webTargets || value.capture.kind === 'web_fetch' &&
    (value.webTargets.requestUrl === undefined || value.webTargets.requestUrl === value.capture.web.requestUrl) &&
    (value.webTargets.resultUrl === undefined || value.webTargets.resultUrl === value.capture.web.finalUrl)) &&
  (value.capture.kind === 'present' ? value.presentTargets !== undefined &&
    value.presentTargets.files.length === value.capture.present.files.length : value.presentTargets === undefined))
export const toolResultLocalDisplaySchemaV1 = z.object({
  ...displayShape, canNavigate: z.boolean().optional(),
  canOpenWeb: z.object({ request: z.boolean(), result: z.boolean() }).strict().optional()
}).strict().refine(value => validKind(value) && validInspect(value) && (!value.canNavigate || value.capture.kind === 'read') &&
  (!value.canOpenWeb || value.capture.kind === 'web_fetch' &&
    (!value.canOpenWeb.request || value.capture.web.requestUrl !== undefined) &&
    (!value.canOpenWeb.result || value.capture.web.finalUrl !== undefined)))
export const toolResultLocalFailureSchemaV1 = z.object({
  ok: z.literal(false), code: z.enum(['unavailable', 'unsupported', 'canceled'])
}).strict()
export const toolResultLocalViewResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), display: toolResultLocalDisplaySchemaV1 }).strict(),
  toolResultLocalFailureSchemaV1
])
export const toolResultLocalPresentHistoryViewResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), history: toolResultLocalPresentHistorySchemaV1 }).strict(), toolResultLocalFailureSchemaV1
])
export const toolResultLocalEffectResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true) }).strict(), toolResultLocalFailureSchemaV1
])
export const toolResultLocalPresentApplicationSchemaV1 = z.object({
  applicationId: presentApplicationId, label: domainText(4096, true), isDefault: z.boolean(),
  iconDataUrl: text(256 << 10).refine(value => /^data:image\/png;base64,[A-Za-z0-9+/]+={0,2}$/.test(value)).optional()
}).strict()
export const toolResultLocalPresentApplicationsResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), applications: z.array(toolResultLocalPresentApplicationSchemaV1).max(32)
    .refine(values => new Set(values.map(value => value.applicationId)).size === values.length &&
      values.filter(value => value.isDefault).length <= 1) }).strict(), toolResultLocalFailureSchemaV1
])
export const toolResultLocalPresentAssociationResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), fileIdentity: z.string().uuid(),
    artifactIds: z.array(z.string().regex(/^[a-f0-9]{64}$/)).max(32)
      .refine(values => new Set(values).size === values.length) }).strict(), toolResultLocalFailureSchemaV1
])
export const toolResultLocalNavigateResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), target: z.object({
    path: readTargetPath, workspaceRoot: readTargetPath, line: z.number().int().positive()
  }).strict() }).strict(), toolResultLocalFailureSchemaV1
])
export type ToolResultLocalHostDisplayV1 = z.infer<typeof toolResultLocalHostDisplaySchemaV1>
export type ToolResultLocalNavigateResultV1 = z.infer<typeof toolResultLocalNavigateResultSchemaV1>
export type ToolResultLocalSelectorV1 = z.infer<typeof toolResultLocalSelectorSchemaV1>
export type ToolResultLocalViewRequestV1 = z.infer<typeof toolResultLocalViewRequestSchemaV1>
export type ToolResultLocalEffectRequestV1 = z.infer<typeof toolResultLocalEffectRequestSchemaV1>
export type ToolResultLocalSaveRequestV1 = z.infer<typeof toolResultLocalSaveRequestSchemaV1>
export type ToolResultLocalDisplayV1 = z.infer<typeof toolResultLocalDisplaySchemaV1>
export type ToolResultLocalViewResultV1 = z.infer<typeof toolResultLocalViewResultSchemaV1>
export type ToolResultLocalEffectResultV1 = z.infer<typeof toolResultLocalEffectResultSchemaV1>

export type ToolResultLocalWebLinkRequestV1 = z.infer<typeof toolResultLocalWebLinkRequestSchemaV1>
export type ToolResultLocalCopyRequestV1 = z.infer<typeof toolResultLocalCopyRequestSchemaV1>

export type ToolResultLocalPresentFileRequestV1 = z.infer<typeof toolResultLocalPresentFileRequestSchemaV1>
export type ToolResultLocalPresentOpenRequestV1 = z.infer<typeof toolResultLocalPresentOpenRequestSchemaV1>
export type ToolResultLocalPresentApplicationV1 = z.infer<typeof toolResultLocalPresentApplicationSchemaV1>
export type ToolResultLocalPresentApplicationsResultV1 = z.infer<typeof toolResultLocalPresentApplicationsResultSchemaV1>

export type ToolResultLocalPresentAssociationRequestV1 = z.infer<typeof toolResultLocalPresentAssociationRequestSchemaV1>
export type ToolResultLocalPresentAssociationResultV1 = z.infer<typeof toolResultLocalPresentAssociationResultSchemaV1>

export type ToolResultLocalPresentHistoryV1 = z.infer<typeof toolResultLocalPresentHistorySchemaV1>
export type ToolResultLocalPresentHistoryViewRequestV1 = z.infer<typeof toolResultLocalPresentHistoryViewRequestSchemaV1>
export type ToolResultLocalPresentHistoryViewResultV1 = z.infer<typeof toolResultLocalPresentHistoryViewResultSchemaV1>

export type ToolResultLocalPresentExcelCopyRequestV1 = z.infer<typeof toolResultLocalPresentExcelCopyRequestSchemaV1>
