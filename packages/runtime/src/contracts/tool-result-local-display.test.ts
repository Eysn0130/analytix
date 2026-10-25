import { describe, expect, it } from 'vitest'
import { toolResultLocalPresentOpenRequestSchemaV1, toolResultLocalPresentHistoryRequestSchemaV1, toolResultLocalPresentHistorySchemaV1, toolResultLocalPresentHistoryViewResultSchemaV1, toolResultLocalPresentAssociationRequestSchemaV1, toolResultLocalPresentAssociationResultSchemaV1, toolResultLocalDisplaySchemaV1, toolResultLocalInspectSchemaV1, toolResultLocalViewRequestSchemaV1, toolResultLocalHostDisplaySchemaV1, toolResultLocalNavigateResultSchemaV1, toolResultLocalEffectRequestSchemaV1, toolResultLocalSaveRequestSchemaV1, toolResultLocalWebLinkRequestSchemaV1, toolResultLocalCopyRequestSchemaV1, toolResultLocalScheduleReceiptSchemaV1 } from './tool-result-local-display'
const capture = { kind: 'shell', status: 'timeout', body: 'exitCode=0\nstatus=completed\r\n', label: 'printf', exitCode: 7, durationMs: 3, truncated: false, startLine: 0, endLine: 0, totalLines: 0 }
const display = { schemaVersion: 1, snapshotDigest: 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'bash', capture }
const domainBase = { status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 }
const domainGoal = { threadId: 'thread-a', objective: '  committed text  ', status: 'active', tokenBudget: 100, tokensUsed: 12,
  timeUsedSeconds: 3, createdAt: '2026-10-10T00:00:00Z', updatedAt: '2026-10-10T00:00:00Z' }
const domainTodo = { threadId: 'thread-a', updatedAt: domainGoal.updatedAt, items: [{ id: 'todo-1', content: 'Committed task',
  status: 'failed', statusReasonCode: 'tool_failed', createdAt: domainGoal.createdAt, updatedAt: domainGoal.updatedAt }] }

describe('seven existing successful file results', () => {
  const classification = 'source_exact_local'
  const diff = { kind: 'modify', added: 0, removed: 2, patch: 'ORIGINAL_PATCH\r\n', binary: false, truncated: false }
  const facts = { path: '/synthetic/source', relativePath: 'source' }
  const sources = [
    { operation: 'ls', classification, list: { ...facts, entries: [{ name: '原目录', displayName: '原目录/', kind: 'directory' }],
      names: ['原目录/'], limit: 10, producerTruncated: false, originalCount: 1 } },
    { operation: 'find', classification, find: { ...facts, pattern: '*.go', matches: [{ path: '/synthetic/source/a.go', relativePath: 'source/a.go' }],
      skippedProtected: ['private'], skippedProtectedOriginalCount: 1, limit: 10, producerTruncated: true, originalCount: 1 } },
    { operation: 'code_index', classification, codeIndex: { ...facts, action: 'outline', query: '', kind: '', limit: 1,
      symbols: [{ name: 'Real', kind: 'function', file: 'source/a.go', path: '/synthetic/source/a.go', line: 2, parent: '', signature: 'func Real()' }],
      symbolCount: 1, originalCount: 1, text: 'source/a.go:2: function Real - func Real()', skippedProtected: [], skippedProtectedOriginalCount: 0,
      skippedDirectories: [], skippedDirectoriesOriginalCount: 0, producerTruncated: false, limitReached: true } },
    { operation: 'move_file', classification, move: { sourcePath: '/synthetic/source', destinationPath: '/synthetic/source',
      sourceRelativePath: 'source', destinationRelativePath: 'source', bytesMoved: '0', moved: false, message: 'no changes made' } },
    { operation: 'notebook_edit', classification, notebookEdit: { ...facts, editMode: 'replace', cellNumber: 0, cellCount: 1, cellType: 'code',
      cellId: 'original-id', message: 'Actual cell edit', bytesWritten: '9007199254740993', firstChangedLine: 0, diff } },
    { operation: 'delete_range', classification, deleteRange: { ...facts, startLine: 3, endLine: 6, firstDeletedLine: 4, deletedLines: 2,
      bytesWritten: '13', encoding: 'utf-8', inclusive: false, diff } },
    { operation: 'delete_symbol', classification, deleteSymbol: { ...facts, name: 'Real', kind: 'function', parent: 'OriginalParent',
      line: 2, firstDeletedLine: 2, deletedLines: 2, bytesWritten: '13', encoding: 'utf-8', diff } }
  ]
  const sealed = (fileResult: unknown, toolName: string, truncated = false) => ({ ...display, toolName,
    inspect: { inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null },
    capture: { ...domainBase, kind: 'file_result', fileResult, truncated } })
  it.each(sources)('preserves the original $operation facts and exact operation ownership', fileResult => {
    const value = sealed(fileResult, fileResult.operation)
    expect(toolResultLocalDisplaySchemaV1.parse(value).capture).toEqual(value.capture)
    expect(toolResultLocalHostDisplaySchemaV1.parse(value).capture).toEqual(value.capture)
    for (const changed of [{ ...value, toolName: 'unknown' }, { ...value, inspect: undefined }, { ...value, capture: { ...value.capture, status: 'failed' } },
      { ...value, capture: { ...value.capture, body: 'fake result' } }, { ...value, canNavigate: true },
      { ...value, capture: { ...value.capture, fileResult: { ...fileResult, invented: true } } },
      { ...value, capture: { ...value.capture, fileResult: { ...fileResult, operation: 'read' } } }]) {
      expect(toolResultLocalDisplaySchemaV1.safeParse(changed).success).toBe(false)
    }
  })
  it('keeps producer truncation, limit reached, missing capture and genuine empty distinct', () => {
    const listing = sources[0]
    const value = { ...listing, list: { ...listing.list!, entries: [], names: [] } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed(value, 'ls', true)).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed(value, 'ls')).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...value, list: { ...value.list, originalCount: 0 } }, 'ls')).success).toBe(true)
    const index = sources[2]
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed(index, 'code_index')).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...index, codeIndex: { ...index.codeIndex!, symbols: [], text: undefined, textUnavailable: 'limit' } }, 'code_index', true)).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...index, codeIndex: { ...index.codeIndex!, symbols: [], symbolCount: 0 } }, 'code_index', true)).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...listing, list: { ...listing.list!, names: ['different/'] } }, 'ls')).success).toBe(false)
  })
  it('rejects rounded counters, cut patches, malformed text and over-budget records', () => {
    const notebook = sources[4]
    for (const extra of [{ bytesWritten: Number.MAX_SAFE_INTEGER + 1 }, { bytesWritten: '01' }, { bytesWritten: '9223372036854775808' },
      { cellNumber: Number.MAX_SAFE_INTEGER + 1 }, { path: 'bad\0path' }, { message: '\ud800' },
      { diff: { ...diff, patchUnavailable: 'limit' } }, { message: '中'.repeat(11000) }]) {
      expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...notebook, notebookEdit: { ...notebook.notebookEdit!, ...extra } }, 'notebook_edit')).success).toBe(false)
    }
    expect(toolResultLocalDisplaySchemaV1.safeParse(sealed({ ...notebook, notebookEdit: { ...notebook.notebookEdit!,
      diff: { ...diff, patch: undefined, patchUnavailable: 'limit' } } }, 'notebook_edit', true)).success).toBe(true)
  })
})

describe('shared private settled error', () => {
  const inspect = { inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null,
    outputError: { availability: 'available', entries: [{ message: '  Alice\r\nnumber=9007199254740993\n', classification: 'source_exact_local', parallelIndex: 1 },
      { message: 'actual third failure', classification: 'credential_redacted', parallelIndex: 3 }] } }
  it('preserves source text and actual indices inside the same failed envelope', () => {
    expect(toolResultLocalDisplaySchemaV1.parse({ ...display, inspect }).inspect).toEqual(inspect)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...display, capture: { ...capture, status: 'completed' }, inspect }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, outputError: { availability: 'unavailable', reason: 'missing' } }).success).toBe(true)
  })
  it('rejects shifted ownership, over-limit prefixes and private authority in the DTO', () => {
    for (const outputError of [
      { ...inspect.outputError, entries: [{ ...inspect.outputError.entries[0], parallelIndex: 0 }] },
      { ...inspect.outputError, entries: [inspect.outputError.entries[0], inspect.outputError.entries[0]] },
      { ...inspect.outputError, entries: [{ ...inspect.outputError.entries[0], parallelIndex: undefined }, inspect.outputError.entries[1]] },
      { ...inspect.outputError, entries: [{ message: 'x'.repeat(32769), classification: 'source_exact_local' }] },
      { ...inspect.outputError, entries: [{ message: 'actual\0error', classification: 'source_exact_local' }] },
      { ...inspect.outputError, entries: [{ message: 'actual', classification: 'source_exact_local', executionGrant: {} }] },
      { availability: 'unavailable', reason: 'limit', entries: inspect.outputError.entries }
    ]) expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, outputError }).success).toBe(false)
  })
})
describe('closed native Schedule receipts', () => {
  const task = { id: 'task-1', title: '  Original task  ', enabled: true, prompt: 'PRIVATE_ORIGINAL_PROMPT',
    workspaceRoot: '/synthetic/original', clawChannelId: '', model: 'actual-model', reasoningEffort: 'auto', mode: 'agent',
    schedule: { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '' }, createdAt: 'original-created', updatedAt: 'original-updated',
    lastRunAt: '', nextRunAt: '', lastStatus: 'idle', lastMessage: 'schedule_task_idle', lastThreadId: '' }
  const receipt = { operation: 'create', status: 'success', message: 'Created', task }
  const source = { ...display, toolName: 'mcp__gui_schedule__gui_schedule_create', capture: { ...domainBase, kind: 'schedule', schedule: receipt } }
  it('retains actual native values and original optional/empty identities', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(source).capture).toEqual(source.capture)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, toolName: 'mcp__gui_schedule__claw_schedule_create' }).success).toBe(true)
    expect(toolResultLocalScheduleReceiptSchemaV1.parse({ ...receipt, task: { ...task, providerId: 'unknown-provider' } })).toEqual({ ...receipt, task: { ...task, providerId: 'unknown-provider' } })
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ operation: 'delete', status: 'failure', message: 'Failed', taskId: ' actual returned ID ' }).success).toBe(true)
  })
  it('rejects mismatched operation/status, guessed fields and incomplete/null records', () => {
    for (const changed of [
      { ...source, toolName: 'mcp__other__gui_schedule_create' }, { ...source, toolName: 'schedule_create' },
      { ...source, toolName: 'mcp__gui_schedule__gui_schedule_update' },
      { ...source, canNavigate: true }, { ...source, capture: { ...source.capture, status: 'failed' } },
      { ...source, capture: { ...source.capture, schedule: { ...receipt, task: null } } },
      ...[{ prompt: undefined }, { enabled: undefined }, { providerId: null }, { lastThreadId: undefined }, { timezone: 'UTC' },
        { id: ' ' }, { title: '\ud800' }, { schedule: { ...task.schedule, kind: 'cron' } }].map(extra =>
        ({ ...source, capture: { ...source.capture, schedule: { ...receipt, task: { ...task, ...extra } } } }))
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(changed).success).toBe(false)
    const failure = { ...source, capture: { ...source.capture, status: 'failed', schedule: { ...receipt, status: 'failure', task: null } } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(failure).success).toBe(true)
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ operation: 'delete', status: 'success', message: 'Deleted' }).success).toBe(false)
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ operation: 'delete', status: 'success', message: 'Deleted', taskId: task.id, task: null }).success).toBe(false)
  })
  it('admits complete new calendar receipts and rejects damaged explicit zones', () => {
    for (const schedule of [{ ...task.schedule, timeZone: 'UTC' }, { ...task.schedule, kind: 'weekly', timeZone: 'UTC', weekdays: [1, 2, 3, 4, 5] }, { ...task.schedule, kind: 'cron', timeZone: 'UTC', expression: '0 9 * * 1-5' }]) {
      expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ ...receipt, task: { ...task, schedule } }).success).toBe(true)
    }
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ ...receipt, task: { ...task, schedule: { ...task.schedule, timeZone: '' } } }).success).toBe(false)
  })
  it('rejects byte overflow rather than retaining an incomplete success task', () => {
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ ...receipt, task: { ...task, prompt: '中'.repeat(87382) } }).success).toBe(false)
    expect(toolResultLocalScheduleReceiptSchemaV1.safeParse({ ...receipt, message: 'a'.repeat(140000), task: { ...task, prompt: 'b'.repeat(140000) } }).success).toBe(false)
  })
})
describe('protected Job and common Inspect facts', () => {
  const inspect = { inputJSON: '{"actual":"call-input"}', schemaJSON: '{"name":"ls","description":"original request","parameters":{"type":"object"}}',
    inputTruncated: false, schemaTruncated: false, callEventAt: '2026-10-10T00:00:00.000999999Z',
    resultEventAt: '2026-10-10T00:00:02.000000001Z', durationMs: 1999 }
  it('uses exact recorded event times and preserves unavailable/running facts', () => {
    expect(toolResultLocalInspectSchemaV1.safeParse(inspect).success).toBe(true)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, durationMs: 2000 }).success).toBe(false)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, resultEventAt: null, durationMs: null }).success).toBe(true)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, resultEventAt: null }).success).toBe(false)
    expect(toolResultLocalInspectSchemaV1.safeParse({ inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }).success).toBe(true)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, callEventAt: 'invalid' }).success).toBe(false)
    expect(toolResultLocalInspectSchemaV1.safeParse({ ...inspect, schemaTruncated: true }).success).toBe(false)
    const ordinary = { ...display, toolName: 'ls', capture: { ...domainBase, kind: 'inspect' }, inspect }
    expect(toolResultLocalDisplaySchemaV1.safeParse(ordinary).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...ordinary, executionGrant: {} }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...ordinary, capture: { ...ordinary.capture, body: 'arbitrary result text' } }).success).toBe(false)
  })
  const job = { jobId: 'job-a', kind: 'subagent', status: 'running', name: 'ACTUAL_NAME', label: 'ACTUAL_PRODUCER_LABEL', output: { availability: 'unavailable' } }
  const jobDisplay = { ...display, toolName: 'kill_shell', capture: { ...domainBase, kind: 'job', status: 'failed',
    job: { operation: 'cancel', jobs: [job], total: 1, truncated: false, receipt: { operation: 'cancel', status: 'cancellation-requested' } } } }
  it('keeps cancel acknowledgement distinct from Job status and tool settlement', () => {
    const parsed = toolResultLocalDisplaySchemaV1.parse(jobDisplay)
    expect(parsed.capture).toEqual(jobDisplay.capture)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...jobDisplay, toolName: 'wait' }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...jobDisplay, capture: { ...jobDisplay.capture, job: {
      ...jobDisplay.capture.job, receipt: { operation: 'cancel', status: 'killed' } } } }).success).toBe(false)
  })
  it('closes local output availability without minting result/export authority', () => {
    const output = { ...jobDisplay, toolName: 'bash_output', capture: { ...domainBase, kind: 'job', job: {
      operation: 'output', jobs: [{ ...job, name: null, label: null, output: { availability: 'available', body: 'RETAINED_RECORD_OUTPUT', truncated: false } }],
      total: 1, truncated: false, receipt: null } } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(output).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...output, canReadOutput: true }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...jobDisplay, capture: { ...jobDisplay.capture, job: {
      ...jobDisplay.capture.job, jobs: [{ ...job, output: { availability: 'unavailable', body: 'INJECTED' } }] } } }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...output, capture: { ...output.capture, job: {
      ...output.capture.job, total: 2 } } }).success).toBe(false)
  })
})
describe('selected native domain display records', () => {
  it('retains parallel active items in both committed snapshots and replay', () => {
    const first = { id: 'todo-1', content: 'Committed task', status: 'in_progress',
      createdAt: domainGoal.createdAt, updatedAt: domainGoal.updatedAt }
    const before = { ...domainTodo, items: [first, { ...first, id: 'todo-2', content: 'Second' }] }
    const after = { ...before, items: [{ ...first, content: 'After' }, before.items[1]] }
    const source = { ...display, toolName: 'todo_write', capture: { ...domainBase, kind: 'todo',
      todo: { todos: after, baseline: { availability: 'known', todos: before } } } }
    expect(toolResultLocalDisplaySchemaV1.parse(JSON.parse(JSON.stringify(source)))).toEqual(source)
    for (const bad of [
      { ...after, items: [first, first] }, { ...after, items: [{ ...first, unknown: true }] },
      { ...after, items: [{ ...first, status: 'active' }] }, { ...after, items: [{ ...first, statusReasonCode: 'tool_failed' }] },
      { ...after, items: Array.from({ length: 201 }, (_, index) => ({ ...first, id: String(index) })) }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture,
      todo: { ...source.capture.todo, todos: bad } } }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture,
      todo: { todos: after, baseline: { availability: 'known', todos: { ...before, items: [first, first] } } } } }).success).toBe(false)
  })
  it('retains original projected text, actual budget, nullable goal and known nullable baseline', () => {
    const goal = { ...display, toolName: 'get_goal', capture: { ...domainBase, kind: 'goal', goal: { goal: domainGoal, remainingTokens: 88 } } }
    expect(toolResultLocalDisplaySchemaV1.parse(goal).capture).toEqual(goal.capture)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...goal, capture: { ...goal.capture, goal: { goal: null, remainingTokens: null } } }).success).toBe(true)
    const todo = { ...display, toolName: 'todo_write', capture: { ...domainBase, kind: 'todo', todo: { todos: domainTodo, baseline: { availability: 'known', todos: null } } } }
    expect(toolResultLocalDisplaySchemaV1.parse(todo).capture).toEqual(todo.capture)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...todo, capture: { ...todo.capture, todo: { todos: domainTodo, baseline: { availability: 'unavailable' } } } }).success).toBe(true)
    for (const bad of [
      { ...goal, toolName: 'todo_write' }, { ...goal, canNavigate: true },
      { ...goal, capture: { ...goal.capture, goal: { goal: domainGoal, remainingTokens: 100 } } },
      ...[{ threadId: '../thread' }, { objective: 'a\0b' }, { createdAt: '' }, { evidenceLedger: [] }].map(extra =>
        ({ ...goal, capture: { ...goal.capture, goal: { goal: { ...domainGoal, ...extra }, remainingTokens: 88 } } })),
      { ...todo, capture: { ...todo.capture, todo: { todos: domainTodo, baseline: { availability: 'known' } } } },
      { ...todo, capture: { ...todo.capture, todo: { todos: domainTodo, baseline: { availability: 'unavailable', todos: null } } } },
      { ...todo, capture: { ...todo.capture, todo: { todos: { ...domainTodo, items: [{ ...domainTodo.items[0], id: ' ' }] }, baseline: { availability: 'known', todos: null } } } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(bad).success).toBe(false)
  })
  it('accepts only real Skill identity and withheld child association with explicit nulls', () => {
    const skill = { requestedName: 'alias', resolved: { id: 'skill-a', name: 'Skill A', runAs: 'subagent', packageDigest: 'c'.repeat(64) },
      child: { jobId: 'job-a', childThreadId: null, childTurnId: null, status: 'failed', background: false, outputWithheld: true }, error: null }
    const value = { ...display, toolName: 'run_skill', capture: { ...domainBase, kind: 'skill', status: 'failed', skill } }
    expect(toolResultLocalDisplaySchemaV1.parse(value).capture).toEqual(value.capture)
    for (const bad of [
      { ...value, capture: { ...value.capture, body: 'child output' } },
      { ...value, capture: { ...value.capture, truncated: true } },
      { ...value, capture: { ...value.capture, skill: { ...skill, resolved: { ...skill.resolved, runAs: 'inline' } } } },
      { ...value, capture: { ...value.capture, skill: { ...skill, child: { ...skill.child, childTurnId: undefined } } } },
      { ...value, capture: { ...value.capture, skill: { ...skill, child: { ...skill.child, output: 'private' } } } },
      { ...value, capture: { ...value.capture, skill: { ...skill, error: undefined } } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(bad).success).toBe(false)
  })
})
describe('closed protected-local result contract', () => {
  it('retains exact original body and authoritative timeout/exit evidence', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(display).capture).toEqual(capture)
    expect(toolResultLocalDisplaySchemaV1.parse({ ...display, capture: { ...capture, body: '' } }).capture.body).toBe('')
  })
  it('rejects private envelopes, generic outputs, overflow and malformed read coordinates', () => {
    for (const value of [
      { ...display, principal: {} }, { ...display, path: '/private/source' },
      { ...display, capture: { ...capture, output: 'raw' } },
      { ...display, capture: { ...capture, body: '中'.repeat(10923) } },
      { ...display, capture: { ...capture, body: '\ud800' } },
      { ...display, toolName: 'unknown' },
      { ...display, toolName: 'read', capture: { ...capture, kind: 'read', status: 'completed', exitCode: undefined, durationMs: 0, startLine: 4, endLine: 2 } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
  })
  it('wire input is identity-only and does not allow actor or text substitution', () => {
    const request = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'c'.repeat(64), resultItemId: display.resultItemId }
    expect(toolResultLocalViewRequestSchemaV1.safeParse(request).success).toBe(true)
    for (const extra of [{ body: 'forged' }, { workspace: '/another' }, { principal: 'other' }, { threadId: '../foreign' }]) {
      expect(toolResultLocalViewRequestSchemaV1.safeParse({ ...request, ...extra }).success).toBe(false)
    }
  })
})

it('separates the Host read target from renderer display and accepts only identity effects', () => {
  const read = { ...display, toolName: 'read', capture: { ...capture, kind: 'read', status: 'completed', exitCode: undefined,
    durationMs: 0, startLine: 9, endLine: 9, totalLines: 20 } }
  const target = { path: '/synthetic/actual.go', workspaceRoot: '/synthetic', startLine: 9, contentDigest: 'f'.repeat(64) }
  expect(toolResultLocalHostDisplaySchemaV1.parse({ ...read, readTarget: target }).readTarget).toEqual(target)
  expect(toolResultLocalDisplaySchemaV1.safeParse({ ...read, readTarget: target }).success).toBe(false)
  expect(toolResultLocalDisplaySchemaV1.parse({ ...read, canNavigate: true }).canNavigate).toBe(true)
  expect(toolResultLocalDisplaySchemaV1.safeParse({ ...display, canNavigate: true }).success).toBe(false)
  expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...read, readTarget: { ...target, startLine: 1 } }).success).toBe(false)
  expect(toolResultLocalNavigateResultSchemaV1.safeParse({ ok: true, target: { path: target.path, workspaceRoot: target.workspaceRoot, line: 9, contentDigest: target.contentDigest } }).success).toBe(false)
  const effect = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: display.resultItemId, snapshotDigest: display.snapshotDigest }
  expect(toolResultLocalEffectRequestSchemaV1.safeParse(effect).success).toBe(true)
  for (const extra of [{ path: target.path }, { line: 1 }, { workspaceRoot: '/foreign' }, { readTarget: target }]) {
    expect(toolResultLocalEffectRequestSchemaV1.safeParse({ ...effect, ...extra }).success).toBe(false)
  }
})

const webCapture = { kind: 'web_fetch', status: 'completed', body: '真实网页正文\n', label: 'Fetch', durationMs: 7,
  truncated: false, startLine: 0, endLine: 0, totalLines: 0,
  web: { requestUrl: 'https://synthetic.invalid/request?q=真实', finalUrl: 'https://synthetic.invalid/final',
    requestUrlTruncated: false, finalUrlTruncated: false, inputTruncated: false, schemaTruncated: false,
    inputJSON: '{"url":"https://synthetic.invalid/request?q=真实"}', schemaJSON: '{"type":"object"}',
    responseComplete: true, toolIsError: true, statusCode: 404, fetchTruncated: false,
    startedAt: '2026-10-08T12:00:00.123456789Z' } }
const webDisplay = { ...display, toolName: 'web_fetch', capture: webCapture }
describe('closed protected Web display', () => {
  it('retains actual non-2xx completed text and UTF-8 budget without broadening Shell/Read', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(webDisplay).capture).toEqual(webCapture)
    const body = '中'.repeat(333333) + 'a'
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...webCapture, body } }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...webCapture, body: body + 'a' } }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...display, capture: { ...capture, body: 'a'.repeat(32769) } }).success).toBe(false)
  })
  it('rejects forged kinds, arbitrary JSON fields, invalid status and false completion', () => {
    for (const bad of [
      { ...webDisplay, toolName: 'read' }, { ...display, capture: webCapture },
      { ...webDisplay, capture: { ...webCapture, webTargets: {} } },
      { ...webDisplay, capture: { ...webCapture, exitCode: 0 } },
      { ...webDisplay, capture: { ...webCapture, status: 'failed' } },
      { ...webDisplay, capture: { ...webCapture, web: { ...webCapture.web, inputJSON: 'invalid JSON' } } },
      { ...webDisplay, capture: { ...webCapture, web: { ...webCapture.web, rawBody: '<html>' } } },
      { ...webDisplay, capture: { ...webCapture, web: { ...webCapture.web, statusCode: 600 } } },
      { ...webDisplay, capture: { ...webCapture, web: { ...webCapture.web, startedAt: 'yesterday' } } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(bad).success).toBe(false)
  })
  it('retains an actual invalid request URL with failed output without turning it into a Host target', () => {
    const capture = { ...webCapture, status: 'failed', body: 'Actual URL parse error.',
      web: { ...webCapture.web, requestUrl: 'https://exa mple.test/', finalUrl: undefined, responseComplete: false,
        fetchTruncated: undefined, statusCode: undefined, error: { code: 'invalid_request', message: 'Actual URL parse error.' } } }
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay,
      capture: { ...capture, web: { ...capture.web, requestUrl: 'https://synthetic.invalid/a\0b' } } }).success).toBe(false)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...webDisplay, capture, webTargets: { requestUrl: capture.web.requestUrl } }).success).toBe(false)
    for (const web of [
      { ...webCapture.web, statusCode: undefined }, { ...webCapture.web, fetchTruncated: undefined },
      { ...capture.web, toolIsError: false }, { ...capture.web, fetchTruncated: false },
      { ...capture.web, error: { code: '', message: '' } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...capture, status: web.responseComplete ? 'completed' : 'failed', web } }).success).toBe(false)
  })
  it('requires omitted truncated JSON/URL fields and never accepts clickable prefixes', () => {
    const web = { ...webCapture.web, inputTruncated: true, inputJSON: undefined, requestUrlTruncated: true, requestUrl: undefined }
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...webCapture, web } }).success).toBe(true)
    for (const extra of [{ inputJSON: '{}' }, { requestUrl: 'https://prefix.invalid' }]) {
      expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...webCapture, web: { ...web, ...extra } } }).success).toBe(false)
    }
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, capture: { ...webCapture, web }, canOpenWeb: { request: true, result: false } }).success).toBe(false)
  })
  it('separates sealed Host URL targets from renderer and accepts only role identity', () => {
    const targets = { requestUrl: webCapture.web.requestUrl, resultUrl: webCapture.web.finalUrl }
    const spacedUrl = 'https://synthetic.invalid/actual path'
    expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...webDisplay,
      capture: { ...webCapture, web: { ...webCapture.web, requestUrl: spacedUrl } }, webTargets: { requestUrl: spacedUrl } }).success).toBe(true)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...webDisplay, webTargets: targets }).success).toBe(true)
    for (const target of ['file:///synthetic/source', 'javascript:alert(1)', 'https://another.invalid', 'https://user:pass@synthetic.invalid/request?q=真实']) {
      expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...webDisplay, webTargets: { requestUrl: target } }).success).toBe(false)
    }
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...webDisplay, webTargets: targets }).success).toBe(false)
    const effect = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
      callId: 'call_host_' + 'a'.repeat(64), resultItemId: display.resultItemId, snapshotDigest: display.snapshotDigest }
    expect(toolResultLocalWebLinkRequestSchemaV1.safeParse({ ...effect, targetRole: 'result' }).success).toBe(true)
    for (const extra of [{ url: targets.resultUrl }, { targetRole: 'arbitrary' }, { section: 'payload' }]) {
      expect(toolResultLocalWebLinkRequestSchemaV1.safeParse({ ...effect, targetRole: 'request', ...extra }).success).toBe(false)
    }
    expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...effect, section: 'payload' }).success).toBe(true)
    expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...effect, section: 'schema', body: '{}' }).success).toBe(false)
  })
})

describe('protected Search DTO', () => {
  const search = { shape: 'matches' as const, files: [{ path: '/synthetic/source.go', matches: [{ lineNumber: 2, line: 'SOURCE_MATCH', column: 1, contextBefore: ['before'], contextAfter: ['after'] }] }],
    total: 1, truncated: false, toolIsError: false, inputJSON: '{"pattern":"SOURCE"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
  const source = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'grep', capture: {
    kind: 'search', status: 'completed', body: '/synthetic/source.go\n1- before\n2: SOURCE_MATCH\n3- after', label: 'grep', durationMs: 1,
    truncated: false, startLine: 0, endLine: 0, totalLines: 0, search } }
  it('keeps actual context, unknown total and the two independent truncation reasons', () => {
    expect(toolResultLocalDisplaySchemaV1.safeParse(source).success).toBe(true)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse(source).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture, truncated: true, search: { ...search, total: null, truncated: true } } }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, toolName: 'glob', capture: { ...source.capture, body: '', search: { ...search, shape: 'paths', files: undefined, paths: [] } } }).success).toBe(false)
  })
  it('rejects mismatched body/count/kind, injected spill authority and unknown fields', () => {
    for (const bad of [
      { ...source, toolName: 'read' }, { ...source, capture: { ...source.capture, body: 'FORGED' } },
      { ...source, capture: { ...source.capture, search: { ...search, total: 0 } } },
      { ...source, capture: { ...source.capture, search: { ...search, total: null } } },
      { ...source, capture: { ...source.capture, search: { ...search, total: 2 } } },
      { ...source, capture: { ...source.capture, search: { ...search, truncated: true } } },
      { ...source, capture: { ...source.capture, search: { ...search, spill: '/invented/full-result' } } },
      { ...source, canNavigate: true }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(bad).success).toBe(false)
  })
  it('keeps new request facts in Inspect and preserves strict legacy input requirements', () => {
    const withoutLegacyInput = { ...source, capture: { ...source.capture, search: { ...search, inputJSON: undefined } } }
    const inspect = { inputJSON: search.inputJSON, inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...withoutLegacyInput, inspect }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, inspect }).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse(withoutLegacyInput).success).toBe(false)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...withoutLegacyInput, capture: { ...withoutLegacyInput.capture,
      search: { ...withoutLegacyInput.capture.search, inputTruncated: true } } }).success).toBe(true)
  })
})


describe('closed protected Present declarations', () => {
  const inspect = { inputJSON: '{"files":[{"path":"report.md"},{"path":"report.md","description":""},{"path":"  report.md  ","description":"  Actual description  "}]}', inputTruncated: false, schemaTruncated: false,
    callEventAt: null, resultEventAt: null, durationMs: null }
  const files = [{ index: 0, path: 'report.md' }, { index: 1, path: 'report.md', description: '' },
    { index: 2, path: '  report.md  ', description: '  Actual description  ' }]
  const source = { ...display, toolName: 'present', inspect, capture: { ...domainBase, kind: 'present', present: { files } } }
  const presentTargets = { workspaceRoot: '/synthetic', files: files.map(file => ({ index: file.index, path: '/synthetic/report.md' })) }
  it('retains declaration order, duplicate paths and absent versus empty descriptions', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(source).capture).toEqual(source.capture)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture, present: { files: [{ index: 0, path: '\ufeff' }] } } }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture, present: { files: [{ index: 0, path: '\u0085' }] } } }).success).toBe(false)
    expect(toolResultLocalHostDisplaySchemaV1.parse({ ...source, presentTargets }).presentTargets).toEqual(presentTargets)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, presentTargets }).success).toBe(false)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse(source).success).toBe(false)
  })
  it('requires the original producer, successful fixed capture and original Inspect', () => {
    for (const bad of [
      { ...source, toolName: 'read' }, { ...source, inspect: undefined }, { ...source, canNavigate: true },
      ...[{ status: 'failed' }, { body: 'guessed prose' }, { label: 'guessed name' }, { truncated: true },
        { durationMs: 1 }, { exitCode: 0 }, { presentTargets }].map(extra => ({ ...source, capture: { ...source.capture, ...extra } }))
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(bad).success).toBe(false)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...display, presentTargets }).success).toBe(false)
    expect(toolResultLocalHostDisplaySchemaV1.safeParse({ ...source, presentTargets: { ...presentTargets, files: presentTargets.files.slice(1) } }).success).toBe(false)
  })
  it('rejects incomplete, reordered, unbounded or fabricated declarations', () => {
    for (const files of [[], [{ index: 1, path: 'report.md' }],
      Array.from({ length: 9 }, (_, index) => ({ index, path: 'report.md' })),
      ...[{ path: ' ' }, { path: 'report\0.md' }, { path: '中'.repeat(1366) }, { path: '\ud800' },
        { description: null }, { description: '中'.repeat(1366) }, { description: 'a\0b' }, { seq: 1 }, { applicationId: 'forged' }]
        .map(extra => [{ index: 0, path: 'report.md', ...extra }])
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, capture: { ...source.capture, present: { files } } }).success).toBe(false)
  })
})


describe('private Present primary history witness and association', () => {
  const request = { kind: 'present_history', threadId: 'thread-a', turnId: 'turn-a' }
  const result = { kind: 'tool_result', itemId: 'item_result_' + 'a'.repeat(64), callId: 'call_host_' + 'b'.repeat(64), toolName: 'present' }
  const history = { ...request, schemaVersion: 1, orderedItems: [result, { kind: 'assistant_text', itemId: 'closing' }] }
  it('preserves exact real order and permits an ended turn with no approved closing', () => {
    expect(toolResultLocalPresentHistoryRequestSchemaV1.parse(request)).toEqual(request)
    expect(toolResultLocalPresentHistorySchemaV1.parse(history)).toEqual(history)
    expect(toolResultLocalPresentHistorySchemaV1.parse({ ...history, orderedItems: [result] }).orderedItems).toEqual([result])
    expect(toolResultLocalPresentHistoryViewResultSchemaV1.safeParse({ ok: true, display }).success).toBe(false)
  })
  it('rejects extra authority, bodies, synthetic order, duplicates and non-Present anchors', () => {
    for (const extra of [{ path: '/private' }, { actor: 'system' }, { orderedItems: [] }, { viewId: '11111111-1111-4111-8111-111111111111' }]) {
      expect(toolResultLocalPresentHistoryRequestSchemaV1.safeParse({ ...request, ...extra }).success).toBe(false)
    }
    for (const orderedItems of [[result, result], [{ ...result, toolName: 'bash' }], [{ ...result, seq: 1 }],
      [{ kind: 'assistant_text', itemId: 'closing', text: 'body' }], [result, { ...result, itemId: 'item_result_' + 'c'.repeat(64) }]]) {
      expect(toolResultLocalPresentHistorySchemaV1.safeParse({ ...history, orderedItems }).success).toBe(false)
    }
  })
  it('bounds opaque Office candidates and does not accept path or application authority', () => {
    const indexed = { threadId: 'thread-a', turnId: 'turn-a', callId: result.callId, resultItemId: result.itemId,
      viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0, artifactIds: ['d'.repeat(64)] }
    expect(toolResultLocalPresentAssociationRequestSchemaV1.parse(indexed)).toEqual(indexed)
    for (const extra of [{ path: '/private' }, { artifactIds: [indexed.artifactIds[0], indexed.artifactIds[0]] },
      { artifactIds: Array.from({ length: 33 }, (_, i) => i.toString(16).padStart(64, '0')) }]) {
      expect(toolResultLocalPresentAssociationRequestSchemaV1.safeParse({ ...indexed, ...extra }).success).toBe(false)
    }
    const proof = { ok: true, fileIdentity: indexed.viewId, artifactIds: indexed.artifactIds }
    expect(toolResultLocalPresentAssociationResultSchemaV1.parse(proof)).toEqual(proof)
    expect(toolResultLocalPresentAssociationResultSchemaV1.safeParse({ ...proof, path: '/private' }).success).toBe(false)
  })
})


describe('closed Present native open and Reveal gestures', () => {
  const indexed = { ...toolResultLocalEffectRequestSchemaV1.parse({ threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64),
    viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64) }), index: 0 }
  it('retains default and observed-application requests and admits identity-only Reveal separately', () => {
    for (const value of [indexed, { ...indexed, applicationId: 'org.synthetic.Actual' }, { ...indexed, action: 'reveal' }]) {
      expect(toolResultLocalPresentOpenRequestSchemaV1.parse(value)).toEqual(value)
    }
  })
  it('rejects mixed native actions, path authority, unknown actions and invalid declaration indexes', () => {
    for (const extra of [{ applicationId: 'org.synthetic.Actual' }, { applicationId: undefined }, { action: 'open' },
      { action: null }, { path: '/forged' }, { appPath: '/forged.app' }, { index: -1 }, { index: 8 }]) {
      expect(toolResultLocalPresentOpenRequestSchemaV1.safeParse({ ...indexed, action: 'reveal', ...extra }).success).toBe(false)
    }
  })
})


describe('private selected Present Excel Copy request', () => {
  const effect = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), snapshotDigest: 'c'.repeat(64) }
  const selected = { ...effect, index: 0, contentDigest: 'd'.repeat(64), selection: {
    sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 1], column: [0, 2] }]
  } }
  it('preserves ordinary sections and accepts only original-index bounded selections', () => {
    expect(toolResultLocalCopyRequestSchemaV1.parse(selected)).toEqual(selected)
    for (const section of ['payload', 'result', 'schema', 'request-url', 'result-url'] as const) {
      expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...effect, section }).success).toBe(true)
      expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...selected, section }).success).toBe(false)
    }
    expect(toolResultLocalEffectRequestSchemaV1.safeParse(selected).success).toBe(false)
  })
  it.each([
    { path: '/forged' }, { format: 'xlsx' }, { body: 'renderer text' }, { fileIndex: 0 },
    { text: 'renderer text' }, { html: '<table>renderer HTML</table>' }, { checkedHtml: '<table>forged checked HTML</table>' },
    { index: 8 }, { contentDigest: 'bad' }, { selection: { ...selected.selection, kind: 'excel' } },
    { selection: { ...selected.selection, sheetIndex: -1 } }, { selection: { ...selected.selection, sheetId: '' } },
    { selection: { ...selected.selection, range: selected.selection.ranges[0] } },
    { selection: { ...selected.selection, ranges: [{ row: [1, 0], column: [0, 0] }] } },
    { selection: { ...selected.selection, ranges: [{ row: [0, 0], column: [0, 250_000] }] } },
    { selection: { ...selected.selection, ranges: [{ row: [0, 499], column: [0, 500] }] } },
    { selection: { ...selected.selection, ranges: [{ row: [0, 0.5], column: [0, 0] }] } }
  ])('rejects renderer content authority and invalid coordinates %j', extra => {
    expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...selected, ...extra }).success).toBe(false)
  })
  it.each([
    { label: 'horizontal ranges', ranges: [{ row: [0, 1], column: [4, 5] }, { row: [0, 1], column: [1, 2] }] },
    { label: 'vertical ranges', ranges: [{ row: [4, 5], column: [0, 1] }, { row: [1, 2], column: [0, 1] }] },
    { label: '32 disjoint ranges', ranges: Array.from({ length: 32 }, (_, i) => ({ row: [0, 0], column: [i * 2, i * 2] })) },
    { label: 'exact Cartesian budget', ranges: [{ row: [0, 999], column: [0, 124] }, { row: [0, 999], column: [125, 249] }] },
    { label: 'exact coordinate and cell cap', ranges: [{ row: [0, 249_999], column: [0, 0] }] }
  ])('keeps first-seen range order and accepts $label', ({ ranges }) => {
    const request = { ...selected, selection: { ...selected.selection, ranges } }
    expect(toolResultLocalCopyRequestSchemaV1.parse(request)).toEqual(request)
  })
  it.each([
    { label: 'zero ranges', ranges: [] },
    { label: '33 ranges', ranges: Array.from({ length: 33 }, (_, i) => ({ row: [0, 0], column: [i * 2, i * 2] })) },
    { label: 'diagonal ranges', ranges: [{ row: [0, 0], column: [0, 0] }, { row: [1, 1], column: [1, 1] }] },
    { label: 'neither row nor column intervals agree', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [0, 2], column: [2, 3] }] },
    { label: 'horizontal touching overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [0, 1], column: [1, 2] }] },
    { label: 'vertical touching overlap', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [1, 2], column: [0, 1] }] },
    { label: 'identical rectangles', ranges: [{ row: [0, 1], column: [0, 1] }, { row: [0, 1], column: [0, 1] }] },
    { label: 'Cartesian budget exceeded', ranges: [{ row: [0, 999], column: [0, 124] }, { row: [0, 999], column: [125, 250] }] }
  ])('rejects $label', ({ ranges }) => {
    expect(toolResultLocalCopyRequestSchemaV1.safeParse({ ...selected, selection: { ...selected.selection, ranges } }).success).toBe(false)
  })
})


it('closes save requests over projected text or a single Present declaration identity', () => {
  const identity = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a',
    callId: 'call_host_' + 'a'.repeat(64), resultItemId: display.resultItemId, snapshotDigest: display.snapshotDigest }
  expect(toolResultLocalSaveRequestSchemaV1.parse(identity)).toEqual(identity)
  expect(toolResultLocalSaveRequestSchemaV1.parse({ ...identity, index: 7 })).toEqual({ ...identity, index: 7 })
  for (const extra of [{ index: -1 }, { index: 8 }, { index: 0.5 }, { index: 0, path: '/forged' },
    { index: 0, dataBase64: 'Zm9yZ2Vk' }, { body: 'forged' }, { index: 0, section: 'result' },
    { index: 0, applicationId: 'org.synthetic.editor' }, { index: 0, snapshotDigest: 'invalid' }]) {
    expect(toolResultLocalSaveRequestSchemaV1.safeParse({ ...identity, ...extra }).success).toBe(false)
  }
})


describe('bounded private generic success text over the existing Inspect snapshot', () => {
  const outputText = { availability: 'available', body: 'first\r\n{ "n": 9007199254740993 }', classification: 'source_exact_local', truncated: false }
  const source = { ...display, toolName: 'mcp__docs__lookup', capture: { ...domainBase, kind: 'inspect' },
    inspect: { outputText, inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
  it('preserves literal text and accepts an explicit capture limit without adding raw data', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(source).inspect?.outputText).toEqual(outputText)
    expect(toolResultLocalHostDisplaySchemaV1.parse(source).inspect?.outputText).toEqual(outputText)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, inspect: { ...source.inspect,
      outputText: { availability: 'unavailable', reason: 'limit', truncated: false } } }).success).toBe(true)
    expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, inspect: { ...source.inspect, outputText: undefined } }).success).toBe(true)
  })
  it('refuses wrong outcome, malformed text, arbitrary structures and new effects', () => {
    for (const body of ['', ' \r\n', 'bad\0text', '\ud800', '中'.repeat(11000)]) {
      expect(toolResultLocalDisplaySchemaV1.safeParse({ ...source, inspect: { ...source.inspect, outputText: { ...outputText, body } } }).success).toBe(false)
    }
    for (const changed of [{ ...source, toolName: 'read' }, { ...source, capture: { ...source.capture, status: 'failed' } },
      { ...source, capture: { ...source.capture, status: 'canceled' } }, { ...source, capture: { ...source.capture, body: 'raw fallback' } },
      { ...source, canNavigate: true }, { ...source, inspect: { ...source.inspect, outputText: { ...outputText, images: [] } } },
      { ...source, inspect: { ...source.inspect, outputText: { ...outputText, raw: {} } } },
      { ...source, inspect: { ...source.inspect, outputError: { availability: 'unavailable', reason: 'missing' } } }]) {
      expect(toolResultLocalDisplaySchemaV1.safeParse(changed).success).toBe(false)
    }
  })
})
