// @vitest-environment jsdom
import { act, createElement, StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { parse } from 'postcss'
import { chatBlockFromItem } from '../../../../agent/analytix-mapper'
import type { ToolBlock } from '../../../../agent/types'
import i18n from '../../../../i18n'
import { conversationCallRecords } from './models/conversation-record-model'
import { ConversationRecords, ConversationRecordDetails } from './ConversationRecords'
import { taskJobSummaryV1 } from '../../../../../../../packages/runtime/src/contracts/task-job-output'
import { PublicToolResultProjectionV1 } from '../../../../../../../packages/runtime/src/contracts/items'
import { toolResultLocalDisplaySchemaV1, toolResultLocalSelectorSchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalViewRequestV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ConversationCallRecord } from './models/conversation-record-model'

let root: Root, container: HTMLDivElement
const block = (): ToolBlock => {
  const value = chatBlockFromItem({ threadId: 'thread', turnId: 'turn', callId: 'call', id: 'real-call', toolName: 'get_goal', toolKind: 'tool_call',
    kind: 'tool_call', role: 'assistant', status: 'running', createdAt: '2026-10-09T00:00:00Z', arguments: {
      schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } })
  if (value?.kind !== 'tool') throw new Error('fixture must map'); return value
}
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
it('resolves Records typography and selection against the real presentation tokens', async () => {
  const { readFileSync } = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
  const recordsCss = readFileSync('src/renderer/src/components/chat/presentation/chat/ConversationRecords.module.css', 'utf8')
  const presentationTokens = readFileSync('src/renderer/src/components/chat/presentation/tokens.css', 'utf8')
  const tokens = new Set<string>()
  parse(presentationTokens).walkDecls(declaration => { if (declaration.prop.startsWith('--')) tokens.add(declaration.prop) })
  const css = parse(recordsCss)
  for (const selector of ['.row', '.row[aria-pressed="true"]']) {
    const values: Record<string, string> = {}
    css.walkRules(selector, rule => { rule.walkDecls(declaration => { values[declaration.prop] = declaration.value }) })
    const value = values[selector === '.row' ? 'font' : 'background']
    expect(value).toBeDefined()
    const variables = [...value.matchAll(/var\((--[\w-]+)/g)].map(match => match[1])
    expect(variables.length).toBeGreaterThan(0)
    for (const variable of variables) expect(tokens.has(variable), variable).toBe(true)
    if (selector === '.row') expect(value).toMatch(/^12px\/20px /)
  }
})
it('expands the actual turn on a new request and passes the exact source identity from its record button', async () => {
  const source = block(), record = conversationCallRecords('thread', [source])[0], inspect = vi.fn()
  const draw = async (version?: number) => act(async () => root.render(createElement(ConversationRecords, { threadId: 'thread', turnId: 'turn',
    blocks: [{ kind: 'user', id: 'real-user', text: 'Admitted user', meta: { turnId: 'turn' } }, source], request: version ? { ...record, version } : null,
    selected: record.key, onInspect: inspect })))
  await draw()
  expect(container.querySelector('[data-conversation-source-item="real-user"]')).not.toBeNull()
  await act(async () => container.querySelector<HTMLButtonElement>('button[aria-expanded]')!.click())
  expect(container.querySelector('[data-conversation-call-key]')).toBeNull()
  await draw(1)
  const row = container.querySelector<HTMLButtonElement>('[data-conversation-call-key]')!
  expect(row.getAttribute('aria-pressed')).toBe('true'); expect(row.dataset.callItemId).toBe('real-call')
  await act(async () => row.click()); expect(inspect).toHaveBeenCalledWith(record)
})
it('reuses the existing Inspector for public call Overview without fake Result/Payload/Schema/Timing', async () => {
  const record = conversationCallRecords('thread', [block()])[0], close = vi.fn()
  await act(async () => root.render(createElement(ConversationRecordDetails, { record, requestVersion: 1, onClose: close, onRetire: vi.fn() })))
  const inspector = container.querySelector('[data-tool-result-inspector]')!
  expect(inspector.textContent).toContain('real-call'); expect(inspector.textContent).toContain('thread')
  expect([...inspector.querySelectorAll('[role="tab"]')].map(tab => tab.textContent)).toEqual(['Summary'])
  expect(inspector.textContent).not.toMatch(/Payload|Schema|Timing|Copy|Save|0 ms/)
  await act(async () => inspector.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
  expect(close).toHaveBeenCalledOnce()
})
it('uses the admitted Job Result in the same Inspector without opening a private lease', async () => {
  const open = vi.fn(); vi.stubGlobal('analytix', { runtime: { openToolResultLocalDisplay: open } })
  const job = taskJobSummaryV1({ id: 'job-a', kind: 'bash', status: 'killed', background: true })
  const mapped = chatBlockFromItem({ threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'e'.repeat(64), id: 'item_result_' + 'f'.repeat(64), toolName: 'list_jobs', toolKind: 'command_execution',
    kind: 'tool_result', role: 'tool', status: 'completed', createdAt: '2026-10-09T00:00:00Z', isError: false, output: {
      schemaVersion: 1, projectionKind: 'task_job_result', disclosure: 'metadata_only', status: 'completed', code: 'tool_completed', messageKey: 'tool_completed',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false, taskJob: { operation: 'list', result: { jobs: [job], count: 1 } } } })!
  const record = conversationCallRecords('thread', [mapped])[0]
  expect(record.toolResultItemId).toBe('item_result_' + 'f'.repeat(64))
  await act(async () => root.render(createElement(ConversationRecordDetails, { record, requestVersion: 1, onClose: vi.fn(), onRetire: vi.fn() })))
  expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
  expect([...container.querySelectorAll('[role="tab"]')].map(tab => tab.textContent)).toEqual(['Summary', 'Result'])
  const result = [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(tab => tab.textContent === 'Result')!
  await act(async () => result.click())
  expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('list')
  expect(open).not.toHaveBeenCalled()
})

const jobCallId = 'call_host_' + 'e'.repeat(64), jobResultId = 'item_result_' + 'f'.repeat(64)
function jobRecord(): ConversationCallRecord {
  const mapped = chatBlockFromItem({ threadId: 'thread', turnId: 'turn', callId: jobCallId, id: jobResultId,
    toolName: 'list_jobs', toolKind: 'command_execution', kind: 'tool_result', role: 'tool', status: 'completed',
    createdAt: '2026-10-09T00:00:00Z', isError: false, output: {
      schemaVersion: 1, projectionKind: 'task_job_result', disclosure: 'metadata_only', status: 'completed', code: 'tool_completed', messageKey: 'tool_completed',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
      taskJob: { operation: 'list', result: { jobs: [taskJobSummaryV1({ id: 'job-a', kind: 'bash', status: 'killed', background: true })], count: 1 } }
    } })!
  const record = conversationCallRecords('thread', [mapped])[0]
  expect(PublicToolResultProjectionV1.safeParse(record.block.meta?.publicResult).success).toBe(true)
  expect(toolResultLocalSelectorSchemaV1.safeParse({ ...record.block.meta?.localResult, threadId: 'thread' }).success).toBe(true)
  return record
}
function privateJobSnapshot(resultItemId = jobResultId): ToolResultLocalDisplayV1 {
  return toolResultLocalDisplaySchemaV1.parse({ schemaVersion: 1, toolName: 'list_jobs', resultItemId, snapshotDigest: 'c'.repeat(64),
    capture: { kind: 'inspect', status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 },
    inspect: { inputJSON: '{"original":"PRIVATE_ORIGINAL_INPUT"}', schemaJSON: '{"name":"list_jobs","description":"PRIVATE_ORIGINAL_SCHEMA","parameters":{"type":"object"}}',
      inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } })
}
function privateApi() {
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  const listeners = new Set<(id: string | null) => void>()
  const api = { openToolResultLocalDisplay: vi.fn(async (_request: ToolResultLocalViewRequestV1) => ({ ok: true, display: privateJobSnapshot() })),
    closeToolResultLocalDisplay: vi.fn(async (_viewId: string) => undefined),
    onToolResultLocalDisplayInvalidated: (listener: (id: string | null) => void) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    onRuntimeStatus: () => () => undefined, copyToolResultLocalDisplay: vi.fn(), saveToolResultLocalDisplay: vi.fn() }
  vi.stubGlobal('analytix', { runtime: api })
  return { api, listeners }
}
const tabNames = () => [...container.querySelectorAll('[role="tab"]')].map(tab => tab.textContent)
const selectTab = async (name: string) => {
  const tab = [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(button => button.textContent === name)!
  expect(tab, name).toBeDefined(); await act(async () => tab.click())
}
const expandResult = async () => {
  for (let count = 0; count < 16; count++) {
    const button = container.querySelector<HTMLButtonElement>('[role="tabpanel"] [data-json-node] button[aria-expanded="false"]')
    if (!button) return
    await act(async () => button.click())
  }
  throw new Error('fixture expansion exceeded its bound')
}
const drawDetails = async (record: ConversationCallRecord, options: { initialView?: 'result' | 'inspect'; presentation?: 'overlay' | 'embedded'; requestVersion?: number; onRetire?: () => void; onViewChange?: (view: 'result' | 'inspect') => void } = {}) =>
  act(async () => root.render(createElement(ConversationRecordDetails, { record, requestVersion: 1, onClose: vi.fn(), onRetire: vi.fn(), ...options })))
it('keeps the public Job Result independent from private Inspect, then retires each explicit lease on return', async () => {
  const record = jobRecord(), { api, listeners } = privateApi()
  await drawDetails(record)
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await selectTab('Result'); await expandResult()
  expect(container.textContent).toContain('job-a'); expect(container.textContent).not.toMatch(/PRIVATE_|Payload|Schema|Timing|Copy|Save/)
  await act(async () => container.querySelector<HTMLButtonElement>('[data-conversation-private-inspect]')!.click())
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  expect(api.openToolResultLocalDisplay.mock.calls[0][0]).toEqual({ threadId: record.threadId, turnId: record.turnId,
    callId: record.callId, resultItemId: record.toolResultItemId, viewId: expect.any(String) })
  expect(tabNames()).toEqual(['Summary', 'Payload', 'Result', 'Schema', 'Timing'])
  await selectTab('Payload'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_INPUT')
  const body = container.querySelector('[data-protected-tool-body]')!, range = document.createRange()
  range.selectNodeContents(body); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
  const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy)
  expect(copy.defaultPrevented).toBe(true); expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  await selectTab('Schema'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_SCHEMA')
  await selectTab('Result'); expect(container.textContent).not.toContain('job-a')
  expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('Unavailable')
  expect(container.textContent).not.toMatch(/Copy|Save/); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  const firstView = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
  await act(async () => container.querySelector<HTMLButtonElement>('[data-conversation-public-result]')!.click())
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.textContent).not.toContain('PRIVATE_')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(firstView)
  expect(document.getSelection()?.isCollapsed).toBe(true); expect(listeners.size).toBe(0)
  await act(async () => container.querySelector<HTMLButtonElement>('[data-conversation-private-inspect]')!.click())
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
  expect(api.openToolResultLocalDisplay.mock.calls[1][0].viewId).not.toBe(firstView)
})
it('returns a revoked Job Inspect to public Result without reviving private content', async () => {
  const record = jobRecord(), { api, listeners } = privateApi(), retire = vi.fn(), viewChange = vi.fn()
  await drawDetails(record, { initialView: 'inspect', onRetire: retire, onViewChange: viewChange }); await selectTab('Payload')
  expect(container.textContent).toContain('PRIVATE_ORIGINAL_INPUT')
  const view = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
  await act(async () => { for (const listener of listeners) listener(view) })
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.textContent).not.toContain('PRIVATE_')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(view); expect(retire).not.toHaveBeenCalled()
  expect(viewChange).toHaveBeenLastCalledWith('result')
  await drawDetails(record, { initialView: 'inspect', onRetire: retire, onViewChange: viewChange })
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  await selectTab('Result'); await expandResult(); expect(container.textContent).toContain('job-a')
})
it('drops a late private reply after a newer public request and resets view intent on request changes', async () => {
  const record = jobRecord(), { api } = privateApi()
  let complete: ((value: { ok: boolean; display: ToolResultLocalDisplayV1 }) => void) | undefined
  api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
  await drawDetails(record, { initialView: 'inspect' }); expect(complete).toBeDefined()
  const retired = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
  await drawDetails(record, { initialView: 'result', requestVersion: 2 })
  await act(async () => complete!({ ok: true, display: privateJobSnapshot() }))
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.textContent).not.toContain('PRIVATE_')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retired)
  await drawDetails(record, { initialView: 'inspect', requestVersion: 3 })
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
  await selectTab('Payload'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_INPUT')
  await drawDetails(record, { initialView: 'result', requestVersion: 4 })
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.textContent).not.toContain('PRIVATE_')
})
it('shows a public-only Job and keeps a private-only Job under the original protected owner', async () => {
  const { api } = privateApi(), admitted = jobRecord()
  const publicOnly = { ...admitted, block: { ...admitted.block, meta: { ...admitted.block.meta, localResult: undefined } } }
  await drawDetails(publicOnly)
  expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.querySelector('[data-conversation-private-inspect]')).toBeNull()
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  const privateOnly = { ...admitted, block: { ...admitted.block, meta: { ...admitted.block.meta, publicResult: undefined } } }
  await drawDetails(privateOnly, { requestVersion: 2 })
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  expect(container.querySelector('[data-tool-result-inspector]')?.closest('[data-protected-tool-result]')).not.toBeNull()
  await selectTab('Payload'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_INPUT')
})
it.each(['invalid-schema', 'wrong-name', 'unknown-kind', 'different-source'] as const)('does not admit a public Job Result from %s', async mutation => {
  const { api } = privateApi(), record = jobRecord()
  const meta: NonNullable<ToolBlock['meta']> = { ...record.block.meta, localResult: undefined }
  if (mutation === 'invalid-schema') meta.publicResult = { ...meta.publicResult!, schemaVersion: 2 } as unknown as typeof meta.publicResult
  if (mutation === 'wrong-name') meta.toolName = 'get_goal'
  if (mutation === 'different-source') meta.sourceItemId = 'item_result_' + 'a'.repeat(64)
  const invalid = { ...record, block: { ...record.block, meta, ...(mutation === 'unknown-kind' ? { toolKind: 'tool_call' as const } : {}) } }
  await drawDetails(invalid)
  expect(tabNames()).toEqual(['Summary']); expect(container.querySelector('[data-conversation-private-inspect]')).toBeNull()
  expect(container.textContent).not.toMatch(/Payload|Schema|Timing|Copy|Save/); expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
})
it('preserves public Result but offers no private mode for an invalid or differently bound locator', async () => {
  const { api } = privateApi(), record = jobRecord()
  for (const [index, localResult] of [
    { ...record.block.meta!.localResult!, extra: 'not-admitted' },
    { ...record.block.meta!.localResult!, turnId: 'other-turn' },
    { ...record.block.meta!.localResult!, callId: 'call_host_' + 'a'.repeat(64) },
    { ...record.block.meta!.localResult!, resultItemId: 'item_result_' + 'b'.repeat(64) }
  ].entries()) {
    await drawDetails({ ...record, block: { ...record.block, meta: { ...record.block.meta, localResult } } }, { initialView: 'inspect', requestVersion: index + 1 })
    expect(tabNames()).toEqual(['Summary', 'Result']); expect(container.querySelector('[data-conversation-private-inspect]')).toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  }
})

it('forwards embedded presentation for both public Job Result and its existing private Inspect leaf', async () => {
  const record = jobRecord(), { api } = privateApi()
  await drawDetails(record, { presentation: 'embedded' })
  expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-presentation')).toBe('embedded')
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLButtonElement>('[data-conversation-private-inspect]')!.click())
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-presentation')).toBe('embedded')
  expect(container.querySelector('[data-tool-result-inspector]')?.closest('[data-protected-tool-result]')).not.toBeNull()
})
it('notifies the embedded owner on targeted Job lease retirement and on its own unmount', async () => {
  const record = jobRecord(), { api, listeners } = privateApi(), retire = vi.fn()
  await drawDetails(record, { presentation: 'embedded', initialView: 'inspect', onRetire: retire })
  const lease = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
  await act(async () => { for (const notify of [...listeners]) notify(lease) })
  expect(retire).toHaveBeenCalledOnce()
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(lease)
  await drawDetails(record, { presentation: 'embedded', initialView: 'inspect', onRetire: retire })
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  await act(async () => root.render(null))
  expect(retire).toHaveBeenCalledTimes(2)
})
it('does not turn default overlay unmount into an embedded owner retirement', async () => {
  const retire = vi.fn(), record = conversationCallRecords('thread', [block()])[0]
  await act(async () => root.render(createElement(ConversationRecordDetails, { record, requestVersion: 1, onClose: vi.fn(), onRetire: retire })))
  expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-presentation')).toBe('overlay')
  await act(async () => root.render(null))
  expect(retire).not.toHaveBeenCalled()
})

it('keeps embedded owner intent through StrictMode setup probes and retires only its real unmount', async () => {
  const retire = vi.fn(), record = conversationCallRecords('thread', [block()])[0]
  await act(async () => root.render(createElement(StrictMode, null, createElement(ConversationRecordDetails,
    { record, requestVersion: 1, presentation: 'embedded', onClose: vi.fn(), onRetire: retire }))))
  await act(async () => { await Promise.resolve() })
  expect(container.querySelector('[data-tool-result-inspector][data-presentation="embedded"]')).not.toBeNull()
  expect(retire).not.toHaveBeenCalled()
  await act(async () => root.render(null))
  expect(retire).toHaveBeenCalledOnce()
})
