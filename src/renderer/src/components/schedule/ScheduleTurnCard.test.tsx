// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import type { ToolBlock } from '../../agent/types'
import type { ToolResultLocalDisplayV1, ToolResultLocalScheduleReceiptV1, ToolResultLocalViewResultV1 } from '../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ScheduleTurnCard } from './ScheduleTurnCard'
import { scheduleTaskCatalog } from './schedule-task-catalog'
import i18n from '../../i18n'

const task = normalizeAppSettings({ schedule: { tasks: [{ id: 'same-task', title: 'Creation title', prompt: 'Synthetic prompt' }] } } as AppSettingsV1).schedule.tasks[0]
function call(character = 'a'): ToolBlock {
  const threadId = 'thread', turnId = 'turn', callId = 'call_host_' + character.repeat(64)
  const callItem = 'item_call_' + character.repeat(64), resultItem = 'item_result_' + character.repeat(64)
  return { kind: 'tool', id: callId, summary: 'Schedule', status: 'success', toolKind: 'tool_call', meta: {
    toolName: 'mcp__gui_schedule__gui_schedule_create', recordThreadId: threadId, turnId, callId, toolCallItemId: callItem, toolResultItemId: resultItem,
    toolCallIdentity: { threadId, turnId, callId, itemId: callItem }, toolResultIdentity: { threadId, turnId, callId, itemId: resultItem },
    localResult: { turnId, callId, resultItemId: resultItem }
  } }
}
function display(block: ToolBlock, receipt: ToolResultLocalScheduleReceiptV1 = { operation: 'create', status: 'success', message: '', task }): ToolResultLocalDisplayV1 {
  return { schemaVersion: 1, snapshotDigest: 'f'.repeat(64), resultItemId: block.meta!.localResult!.resultItemId, toolName: 'mcp__gui_schedule__gui_schedule_create',
    capture: { kind: 'schedule', status: receipt.status === 'success' ? 'completed' : 'failed', schedule: receipt,
      label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } }
}
let root: Root, host: HTMLDivElement
let api: ReturnType<typeof makeApi>
const invalidations = new Set<(viewId: string | null) => void>()
function makeApi() {
  return { openToolResultLocalDisplay: vi.fn(async (request: { callId: string; viewId: string }): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display: display(call(request.callId.at(-1)!)) })),
    closeToolResultLocalDisplay: vi.fn(async () => {}),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (viewId: string | null) => void) => { invalidations.add(listener); return () => { invalidations.delete(listener) } }),
    onRuntimeStatus: vi.fn(() => () => {}) }
}
async function draw(blocks: ToolBlock[], ended = true) {
  await act(async () => root.render(createElement(ScheduleTurnCard, { threadId: 'thread', turnId: 'turn', blocks, ended })))
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en')
  invalidations.clear()
  api = makeApi()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api,
    schedule: { listTasks: vi.fn(() => new Promise(() => {})) } } })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('admitted Schedule creation tail', () => {
  it('shows complete successful root receipts without final prose and preserves distinct calls for the same task', async () => {
    const a = call(), b = call('b'), failed = { ...call('c'), status: 'error' as const }
    await draw([a, { ...a }, failed, b])
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
    expect(host.querySelectorAll('[data-tool="schedule_create"]')).toHaveLength(2)
    expect(host.querySelectorAll('[data-protected-tool-summary][data-protected-tool-readonly]')).toHaveLength(2)
    expect(host.textContent).toContain('Creation title')
  })
  it('waits for the turn end boundary and rejects child, incomplete and foreign pairs', async () => {
    const a = call()
    await draw([a], false)
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await draw([
      { ...a, meta: { ...a.meta, child: { parentThreadId: 'thread', parentTurnId: 'turn', childId: 'child', childStatus: 'completed' } } },
      { ...a, meta: { ...a.meta, toolCallItemId: undefined } },
      { ...a, meta: { ...a.meta, turnId: 'other-turn' } }
    ])
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(host.querySelector('[data-tool="schedule_create"]')).toBeNull()
  })
  it.each(['failure', 'update', 'delete', 'unavailable', 'malformed'] as const)('never turns a %s receipt or orphan read into a card', async kind => {
    const a = call()
    const received: unknown = kind === 'unavailable' ? { ok: false, code: 'unavailable' }
      : { ok: true, display: kind === 'failure' ? display(a, { operation: 'create', status: 'failure', message: 'Synthetic failure', task: null })
        : kind === 'update' ? display(a, { operation: 'update', status: 'success', message: '', task })
          : kind === 'delete' ? display(a, { operation: 'delete', status: 'success', message: '', taskId: task.id })
            : { ...display(a), capture: { ...display(a).capture, schedule: { operation: 'create', status: 'success', task: null } } } }
    api.openToolResultLocalDisplay.mockResolvedValueOnce(received as ToolResultLocalViewResultV1)
    await draw([a])
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(1)
    expect(host.querySelector('[data-tool="schedule_create"]')).toBeNull()
    expect(host.querySelector('button')).toBeNull()
    expect(host.textContent).not.toContain('Synthetic failure')
  })
  it('retires a pending Host read when the owning turn is unmounted', async () => {
    let finish!: (result: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(done => { finish = done }))
    await draw([call()]); await draw([])
    await act(async () => finish({ ok: true, display: display(call()) }))
    expect(host.querySelector('[data-tool="schedule_create"]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledTimes(1)
  })
  it('retires only the revoked receipt and cannot revive it from a current catalog refresh', async () => {
    await draw([call(), call('b')])
    expect(host.querySelectorAll('[data-tool="schedule_create"]')).toHaveLength(2)
    const firstViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    await act(async () => invalidations.forEach(notify => notify(firstViewId)))
    expect(host.querySelectorAll('[data-tool="schedule_create"]')).toHaveLength(1)
    await act(async () => scheduleTaskCatalog.invalidate())
    expect(host.querySelectorAll('[data-tool="schedule_create"]')).toHaveLength(1)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
  })
  it('does not reconstruct an uncaptured receipt from common Inspect input or a catalog task', async () => {
    const a = call(), original = display(a)
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...original,
      inspect: { inputJSON: '{"title":"Creation title","taskId":"same-task"}', inputTruncated: false, schemaTruncated: true,
        callEventAt: null, resultEventAt: null, durationMs: null },
      capture: { kind: 'inspect', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false }
    } })
    await draw([a])
    expect(host.querySelector('[data-tool="schedule_create"]')).toBeNull()
    expect(host.textContent).not.toContain('Creation title')
    expect(window.analytix.schedule.listTasks).not.toHaveBeenCalled()
  })
})
