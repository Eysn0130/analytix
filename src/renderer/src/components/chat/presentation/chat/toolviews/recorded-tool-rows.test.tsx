// @vitest-environment jsdom
import { act, createElement, type ContextType } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalViewResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ToolResultTurnItem } from '../../../../../../../../packages/runtime/src/contracts/items'
import { taskJobSummaryV1 } from '../../../../../../../../packages/runtime/src/contracts/task-job-output'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { CoreTurnItemJson, CoreChildRuntimeMetadataJson } from '../../../../../agent/analytix-contract'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { groupProcessSections, presentationPolicyFor } from '../chat-view-model'
import { SkillChildInspectionContext } from './skill-child-inspection'
import { ToolRawIO } from './ToolRawIO'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
const timestamp = '2026-10-09T00:00:00Z'
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
  code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
const item = (toolName: string, output: unknown = host, toolKind: 'tool_call' | 'command_execution' = 'tool_call'): CoreTurnItemJson => ({
  id: selector.resultItemId, threadId: 'thread', turnId: selector.turnId, kind: 'tool_result', role: 'tool', status: 'completed',
  toolName, toolKind, callId: selector.callId, createdAt: timestamp, output, isError: false })
function hydrate(value: CoreTurnItemJson, child?: CoreChildRuntimeMetadataJson): ToolBlock {
  expect(ToolResultTurnItem.safeParse(value).success).toBe(true)
  const block = chatBlockFromItem(value, child)
  if (block?.kind !== 'tool') throw new Error('fixture must reach actual mapper')
  return block
}
function historicalJob(value: CoreTurnItemJson): ToolBlock {
  const block = hydrate(value)
  const { localResult: _unavailable, ...meta } = block.meta ?? {}
  return { ...block, meta }
}
const captureBase = { status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } as const
const nativeChild: CoreChildRuntimeMetadataJson = { parentThreadId: 'thread', parentTurnId: 'turn', parentToolCallId: selector.callId,
  childId: 'child-a', kind: 'subagent', childStatus: 'running', childThreadId: 'actual-child-thread', childTurnId: 'actual-child-turn', background: false }
function privateDisplay(toolName: 'create_goal' | 'get_goal' | 'update_goal' | 'todo_write' | 'run_skill'): ToolResultLocalDisplayV1 {
  const base = { schemaVersion: 1 as const, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName, canNavigate: false }
  if (toolName === 'todo_write') return { ...base, capture: { ...captureBase, kind: 'todo', todo: {
    todos: { threadId: 'thread', items: [{ id: 'todo-a', content: 'PRIVATE_ACTIVE_TODO', status: 'in_progress', createdAt: timestamp, updatedAt: timestamp }], updatedAt: timestamp },
    baseline: { availability: 'known', todos: null } } } }
  if (toolName === 'run_skill') return { ...base, capture: { ...captureBase, kind: 'skill', body: 'PRIVATE_INLINE_INSTRUCTIONS', skill: {
    requestedName: 'PRIVATE_REQUESTED_SKILL', resolved: { id: 'skill-a', name: 'PRIVATE_RESOLVED_SKILL', runAs: 'inline', packageDigest: 'd'.repeat(64) }, child: null, error: null } } }
  return { ...base, capture: { ...captureBase, kind: 'goal', goal: { goal: { threadId: 'thread', objective: 'PRIVATE_ACTUAL_OBJECTIVE',
    status: 'active', tokensUsed: 40, timeUsedSeconds: 12, tokenBudget: 100, createdAt: timestamp, updatedAt: timestamp }, remainingTokens: 60 } } }
}
let root: Root, container: HTMLDivElement, display: ToolResultLocalDisplayV1, invalidate: ((id: string | null) => void) | undefined
let api: ReturnType<typeof makeApi>, previous: ReturnType<typeof useChatStore.getState>
function makeApi() {
  const listeners = new Set<(id: string | null) => void>()
  invalidate = id => { for (const listener of [...listeners]) listener(id) }
  return { openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async (_id: string) => undefined),
    copyToolResultLocalDisplay: vi.fn(async () => ({ ok: true })), saveToolResultLocalDisplay: vi.fn(async () => ({ ok: true })),
    openToolResultLocalWebLink: vi.fn(async () => ({ ok: true })), navigateToolResultLocalDisplay: vi.fn(async () => ({ ok: true })),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }),
    onRuntimeStatus: vi.fn(() => () => undefined) }
}
const draw = async (block: ToolBlock, props: Partial<Parameters<typeof ToolCallTree>[0]> = {}, scope = 'thread') => {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: scope }, createElement(ToolCallTree,
    { blocks: [block], processing: false, policy: presentationPolicyFor('detailed'), ...props }))))
  if (props.visible !== false && props.publicVisible !== false) {
    for (const header of container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')) {
      await act(async () => header.click())
    }
  }
}
const childDisplay = (): ToolResultLocalDisplayV1 => ({ ...privateDisplay('run_skill'), capture: { ...captureBase, kind: 'skill',
  skill: { requestedName: 'Actual skill', resolved: { id: 'skill-a', name: 'Actual skill', runAs: 'subagent', packageDigest: 'd'.repeat(64) },
    child: { jobId: 'job-a', childThreadId: 'actual-child-thread', childTurnId: 'actual-child-turn', status: 'completed',
      background: false, outputWithheld: true }, error: null } } })
const drawChild = async (inspect: NonNullable<ContextType<typeof SkillChildInspectionContext>>, block = hydrate(item('run_skill')), scope = 'thread', visible = true) => {
  await act(async () => root.render(createElement(SkillChildInspectionContext.Provider, { value: inspect },
    createElement(ProtectedToolResultScope.Provider, { value: scope }, createElement(ToolCallTree,
      { blocks: [block], processing: false, visible, policy: presentationPolicyFor('detailed') })))))
  if (visible) {
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')
    if (header) await act(async () => header.click())
  }
}
const row = () => container.querySelector<HTMLElement>('[data-chat-seat] [data-disclosure-row]')!
const expand = async () => { await act(async () => row().click()) }
const inspector = () => container.querySelector<HTMLElement>('[data-tool-result-inspector]')
const clickInspector = async (text: string) => {
  const button = [...inspector()!.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === text || node.getAttribute('aria-label') === text)!
  expect(button).toBeDefined(); await act(async () => button.click())
}
const expandResult = async () => {
  await clickInspector('Result')
  for (let count = 0; count < 100; count++) {
    const button = inspector()?.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
    if (!button) return
    await act(async () => button.click())
  }
  throw new Error('finite fixture tree did not finish expanding')
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en')
  previous = useChatStore.getState(); useChatStore.setState({ workspaceRoot: '' })
  display = privateDisplay('create_goal'); api = makeApi(); Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('actual nine-key mapper and specialized leaf dispatch', () => {
  it.each(['create_goal', 'get_goal', 'update_goal', 'list_jobs', 'bash_output', 'kill_shell', 'wait'])('dispatches real running %s to its existing row with the shared input slot', async name => {
    const job = ['list_jobs', 'bash_output', 'kill_shell', 'wait'].includes(name)
    const callItemId = 'call-item-running'
    const mapped = chatBlockFromItem({ id: callItemId, threadId: 'thread', turnId: 'turn', kind: 'tool_call', role: 'assistant',
      status: 'running', toolName: name, toolKind: job ? 'command_execution' : 'tool_call', callId: selector.callId, createdAt: timestamp,
      arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } })
    if (mapped?.kind !== 'tool') throw new Error('running fixture must reach real mapper')
    const call = { callId: selector.callId, toolCallItemId: callItemId, startedEventSeq: '12' }
    const raw: ProcessDisplayBlockV1 = { blockId: 'raw-input', source: { kind: 'tool', unitId: 'unit-a', ...call }, kind: 'tool_input',
      state: 'running', attempt: 0, durationMs: 0, truncated: false, classification: 'source_exact_local',
      input: { availability: 'available', inputJSON: ' {"sequence":9007199254740993,"exponent":1e+03} ' } }
    let expanded = false
    const setOpen = vi.fn((open: boolean) => { expanded = open })
    const local = { threadId: 'thread', turnId: 'turn', units: [{ unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1',
      placement: 'anchored' as const, calls: [call], precedingCalls: [] }], identities: [raw], blocks: [raw], render: () => null,
      capture: () => ({ blockId: raw.blockId, preview: null, body: createElement(ToolRawIO, { input: raw.input }), expanded, expandable: true, setOpen }) }
    await draw(mapped, { local, policy: presentationPolicyFor('verbose') })
    expect(container.querySelector(job ? '[data-recorded-job-row]' : '[data-recorded-domain-row="goal"]')).not.toBeNull()
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await expand(); expect(setOpen).toHaveBeenLastCalledWith(true)
    await draw(mapped, { local, policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-tool-raw-input]')?.textContent).toBe(raw.input?.availability === 'available' ? raw.input.inputJSON : undefined)
    expect(container.querySelector('[data-job-output]')).toBeNull()
    await draw(mapped, { local, visible: false, privateGeneration: 1, policy: presentationPolicyFor('verbose') })
    expect(container.querySelector('[data-tool-raw-input]')).toBeNull()
  })
  it.each([['list_jobs', 'list'], ['bash_output', 'output'], ['kill_shell', 'cancel'], ['wait', 'wait']] as const)(
    'dispatches %s through its real mapper selector and private %s observation', async (name, operation) => {
      const receipt = operation === 'cancel' ? { operation, status: 'cancellation-requested' } as const : operation === 'wait' ? { operation, status: 'timeout' } as const : null
      display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: name, canNavigate: false,
        capture: { ...captureBase, kind: 'job', job: { operation, jobs: [{ jobId: 'job-a', kind: 'bash', status: 'running', name: 'PRIVATE_ACTUAL_NAME', label: 'PRIVATE_ACTUAL_LABEL',
          output: operation === 'output' ? { availability: 'available', body: 'PRIVATE_ACTUAL_OUTPUT\nsecond line', truncated: true } : { availability: 'unavailable' } }], total: 1, truncated: false, receipt } } }
      const block = hydrate(item(name, host, 'command_execution'))
      expect(block.meta?.localResult).toEqual(selector)
      await draw(block)
      expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce(); expect(container.querySelector('[data-recorded-job-row]')).not.toBeNull()
      expect(container.textContent).not.toContain('PRIVATE_ACTUAL_OUTPUT')
      await expand()
      expect(container.textContent).toContain('PRIVATE_ACTUAL_NAME'); expect(container.textContent).toContain('PRIVATE_ACTUAL_LABEL'); expect(container.textContent).toContain('Running')
      expect(container.textContent).not.toContain('Killed')
      if (operation === 'output') { expect(container.querySelector('[data-job-output] pre')?.textContent).toBe('PRIVATE_ACTUAL_OUTPUT\nsecond line'); expect(container.textContent).toContain('Local capture is incomplete') }
      else expect(container.querySelector('[data-job-output]')).toBeNull()
      if (receipt) expect(container.textContent).toContain(operation === 'cancel' ? 'Cancellation requested' : 'Wait timed out')
      const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
      await act(async () => button.click()); expect(inspector()?.getAttribute('data-call-id')).toBe(selector.callId)
      await expandResult(); expect(inspector()?.textContent).toContain('job-a')
      expect(container.textContent).not.toMatch(/Copy|Save/)
      const selected = container.querySelector('[data-tool-details]')!, range = document.createRange()
      range.selectNodeContents(selected); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
      const event = new Event('copy', { bubbles: true, cancelable: true }); await act(async () => selected.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
      expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
      await act(async () => invalidate?.(null)); expect(container.textContent).not.toContain('PRIVATE_ACTUAL_'); expect(inspector()).toBeNull()
    })
  it.each([['en', 'Restart job'], ['zh', '重启任务']] as const)('presents the admitted restart Job snapshot in %s without inventing a receipt', async (language, title) => {
    await i18n.changeLanguage(language)
    display = toolResultLocalDisplaySchemaV1.parse({ schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId,
      toolName: 'restart_job', canNavigate: false, capture: { ...captureBase, kind: 'job', job: { operation: 'restart',
        jobs: [{ jobId: 'job-restarted', kind: 'subagent', status: 'running', name: 'PRIVATE_RESTARTED_NAME', label: 'PRIVATE_RESTARTED_LABEL',
          output: { availability: 'unavailable' } }], total: 1, truncated: false, receipt: null } } })
    const block = hydrate(item('restart_job', host, 'command_execution'))
    expect(block.meta?.localResult).toEqual(selector)
    await draw(block)
    const jobRow = container.querySelector<HTMLElement>('[data-recorded-job-row="restart"]')!
    expect(jobRow).not.toBeNull(); expect(jobRow.textContent).toContain(title)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    await expand()
    expect(jobRow.querySelector('[data-tool-details]')).not.toBeNull()
    for (const value of ['job-restarted', 'PRIVATE_RESTARTED_NAME', 'PRIVATE_RESTARTED_LABEL', i18n.t('dshRecordedTool.jobStatus.running')]) {
      expect(jobRow.textContent).toContain(value)
    }
    expect(jobRow.querySelector('[data-job-receipt],[data-job-output]')).toBeNull()
    for (const receipt of ['cancel.cancellation-requested', 'cancel.already-finished', 'wait.completed', 'wait.timeout']) {
      expect(jobRow.textContent).not.toContain(i18n.t('dshRecordedTool.job.receipt.' + receipt))
    }
  })
  it.each(['foreign-name', 'foreign-capture', 'foreign-operation', 'wrong-kind'] as const)('rejects Job %s without exposing output or changing its public call state', async variant => {
    const block = hydrate(item('bash_output', host, 'command_execution'))
    expect(block.meta?.localResult).toEqual(selector)
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: variant === 'foreign-name' ? 'list_jobs' : 'bash_output',
      capture: { ...captureBase, kind: 'job', job: { operation: variant === 'foreign-name' || variant === 'foreign-operation' ? 'list' : 'output',
        jobs: [], total: 0, truncated: false, receipt: null } } }
    if (variant === 'foreign-capture') display = { ...display, toolName: 'bash', capture: { ...captureBase, kind: 'shell', status: 'failed', label: 'foreign', body: 'PRIVATE_FOREIGN_BODY', exitCode: 1 } }
    await draw(variant === 'wrong-kind' ? { ...block, toolKind: 'tool_call' } : block)
    expect(container.textContent).not.toContain('PRIVATE_FOREIGN'); expect(container.querySelector('[data-terminal],[data-job-output]')).toBeNull()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    if (variant !== 'wrong-kind') expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('retires a pending Job lease on hide and ignores its late output', async () => {
    const block = hydrate(item('bash_output', host, 'command_execution'))
    expect(block.meta?.localResult).toEqual(selector)
    let finish!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await draw(block); await draw(block, { visible: false })
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash_output', capture: { ...captureBase, kind: 'job', job: {
      operation: 'output', jobs: [{ jobId: 'job-a', kind: 'bash', status: 'completed', name: 'PRIVATE_LATE_NAME', label: null, output: { availability: 'available', body: 'PRIVATE_LATE_OUTPUT', truncated: false } }], total: 1, truncated: false, receipt: null } } }
    await act(async () => finish({ ok: true, display }))
    expect(container.textContent).not.toContain('PRIVATE_LATE'); expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it.each(['empty', 'unavailable'] as const)('keeps %s Job output and nullable names truthful', async availability => {
    const block = hydrate(item('bash_output', host, 'command_execution'))
    expect(block.meta?.localResult).toEqual(selector)
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash_output', capture: { ...captureBase, kind: 'job', job: {
      operation: 'output', jobs: [{ jobId: 'job-a', kind: 'bash', status: 'running', name: null, label: null,
        output: availability === 'empty' ? { availability: 'available', body: '', truncated: false } : { availability: 'unavailable' } }], total: 1, truncated: false, receipt: null } } }
    await draw(block); await expand(); expect(container.textContent).toContain('Unavailable')
    if (availability === 'empty') expect(container.querySelector('[data-job-output] pre')?.textContent).toBe('')
    else { expect(container.querySelector('[data-job-output] pre')).toBeNull(); expect(container.textContent).toContain('Output unavailable') }
    expect(container.textContent).not.toMatch(/Killed|Copy|Save/)
  })
  it.each(['bash', 'read', 'session_event_read'] as const)('opens ordinary %s Inspect from its real keyed row and preserves the actual callback', async name => {
    const block = hydrate(item(name, host, name === 'bash' ? 'command_execution' : 'tool_call'))
    expect(block.meta?.localResult).toEqual(selector)
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: name,
      inspect: { inputJSON: '{"original":"PRIVATE_MODEL_INPUT"}', schemaJSON: JSON.stringify({ name, description: 'PRIVATE_MODEL_SCHEMA', parameters: { type: 'object' } }),
        inputTruncated: false, schemaTruncated: false, callEventAt: timestamp, resultEventAt: timestamp, durationMs: 0 },
      capture: name === 'bash' ? { ...captureBase, kind: 'shell', label: 'bash', body: 'PRIVATE_RESULT' } : name === 'read' ? { ...captureBase, kind: 'read', label: 'read', body: 'PRIVATE_RESULT' } : { ...captureBase, kind: 'inspect' } }
    await draw(block); expect(container.textContent).not.toContain('PRIVATE_MODEL_INPUT')
    await expand(); const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    expect(button).toBeDefined(); await act(async () => button.click()); expect(inspector()).not.toBeNull()
    await clickInspector('Payload'); expect(inspector()?.textContent).toContain('PRIVATE_MODEL_INPUT'); expect(inspector()?.textContent).not.toContain('Copy')
    await clickInspector('Schema'); expect(inspector()?.textContent).toContain('PRIVATE_MODEL_SCHEMA'); expect(inspector()?.textContent).not.toContain('Copy')
    await clickInspector('Timing'); expect(inspector()?.textContent).toContain('0 ms')
    await clickInspector('Close details')
    const onInspect = vi.fn(); await draw(block, { onInspect })
    const delegated = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    await act(async () => delegated.click()); expect(onInspect).toHaveBeenCalledExactlyOnceWith(selector.turnId, selector.callId)
  })
  it.each(['create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill'] as const)('keeps original Inspect facts reachable when %s has no typed Result', async name => {
    const block = hydrate({ ...item(name, { ...host, status: 'failed', code: 'tool_failed', messageKey: 'tool_failed' }), status: 'failed', isError: true })
    expect(block.meta?.localResult).toEqual(selector)
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: name,
      capture: { ...captureBase, kind: 'inspect', status: 'failed' }, inspect: { inputJSON: '{"original":"PRIVATE_FAILED_INPUT"}',
        inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
    await draw(block); await expand()
    if (name === 'todo_write') {
      expect(container.querySelector('[data-todo-input]')?.textContent).toContain('PRIVATE_FAILED_INPUT')
      expect(container.querySelector('[data-tool-details],[data-change]')).toBeNull()
      expect([...container.querySelectorAll('span')].some(node => node.textContent === 'OUT')).toBe(false)
    }
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    expect(button).toBeDefined(); await act(async () => button.click())
    await clickInspector('Payload'); expect(inspector()?.textContent).toContain('PRIVATE_FAILED_INPUT')
    await clickInspector('Result'); expect(inspector()?.textContent).toContain('Unavailable')
    expect(inspector()?.textContent).not.toMatch(/Copy|Save/)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('dispatches an actual stopped Todo result to its protected original IN body', async () => {
    const block = hydrate({ ...item('todo_write', { ...host, status: 'cancelled', code: 'tool_cancelled', messageKey: 'tool_cancelled' }),
      status: 'aborted', isError: true })
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'todo_write',
      capture: { ...captureBase, kind: 'inspect', status: 'canceled' }, inspect: { inputJSON: '{"todos":[{"content":"PRIVATE_STOPPED_IN","status":"pending"}]}',
        inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
    await draw(block); await expand()
    expect(container.querySelector('[data-state="stopped"]')).not.toBeNull()
    expect(container.querySelector('[data-todo-input]')?.textContent).toContain('PRIVATE_STOPPED_IN')
    expect(container.querySelector('[data-tool-details],[data-change],[data-analytix-diff-stats]')).toBeNull()
    expect([...container.querySelectorAll('span')].some(node => node.textContent === 'OUT')).toBe(false)
    await draw(block, { visible: false })
    expect(container.textContent).not.toContain('PRIVATE_STOPPED_IN')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it.each(['create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill'] as const)('renders %s with real private capture and protected folded summary', async name => {
    display = privateDisplay(name); const block = hydrate(item(name))
    await draw(block)
    expect(container.querySelector(name === 'run_skill' ? '[data-skill-row]' : '[data-recorded-domain-row]')).not.toBeNull()
    expect(container.querySelector('[data-protected-tool-summary]')).not.toBeNull()
    expect(container.querySelector('[data-tool-details]')).toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    await expand()
    expect(container.querySelector('[data-protected-tool-body]')).not.toBeNull()
    if (name === 'run_skill') { expect(container.textContent).toContain('PRIVATE_INLINE_INSTRUCTIONS'); expect(container.textContent).toContain('Instructions') }
    else if (name === 'todo_write') { expect(container.textContent).toContain('0/1 completed'); expect(container.querySelector('[data-change="added"]')).not.toBeNull() }
    else { expect(container.textContent).toContain('Tokens remaining'); expect(container.textContent).toContain('60'); expect(container.textContent).not.toMatch(/Rounds|Revision/) }
    await expand()
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
    expect(container.querySelector('[data-protected-tool-summary]')).not.toBeNull()
    expect(container.querySelector('[data-tool-file-link],[data-tool-web-link],a')).toBeNull()
    expect(container.textContent).not.toMatch(/Copy|Save/)
  })
  it.each(['create_goal', 'todo_write', 'run_skill'] as const)('blocks copy/cut/drag for %s summaries and body without export calls', async name => {
    display = privateDisplay(name); await draw(hydrate(item(name))); await expand()
    for (const node of container.querySelectorAll('[data-protected-tool-summary],[data-protected-tool-body]')) {
      const range = document.createRange(); range.selectNodeContents(node); const selection = document.getSelection()!; selection.removeAllRanges(); selection.addRange(range)
      for (const type of ['copy', 'cut', 'dragstart']) {
        const event = new Event(type, { bubbles: true, cancelable: true })
        await act(async () => node.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
      }
    }
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(api.openToolResultLocalWebLink).not.toHaveBeenCalled(); expect(api.navigateToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it.each(['create_goal', 'todo_write', 'run_skill'] as const)('retires %s private summaries, body and late replies when hidden or revoked', async name => {
    display = privateDisplay(name); const block = hydrate(item(name)); await draw(block); await expand()
    await act(async () => invalidate?.(null)); expect(container.textContent).not.toContain('PRIVATE_')
    await draw(block, { visible: false }); expect(container.textContent).not.toContain('PRIVATE_')
    let release!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementation(() => new Promise(resolve => { release = resolve }))
    await draw(block, { privateGeneration: 1 }); await draw(block, { visible: false, privateGeneration: 1 })
    await act(async () => release({ ok: true, display })); expect(container.textContent).not.toContain('PRIVATE_')
  })
  it('rejects a foreign producer capture and keeps old no-capture results honestly unavailable', async () => {
    display = privateDisplay('run_skill'); await draw(hydrate(item('create_goal')))
    expect(container.textContent).not.toContain('PRIVATE_'); expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: false, code: 'unavailable' })
    await draw(hydrate(item('todo_write')), { privateGeneration: 1 }); await expand()
    expect(container.textContent).not.toContain('Initial list'); expect(container.querySelector('[data-change]')).toBeNull()
  })
  it('does not turn private Skill child observations or nullable IDs into navigation or controls', async () => {
    display = { ...privateDisplay('run_skill'), capture: { ...captureBase, kind: 'skill', skill: { requestedName: 'Actual skill',
      resolved: { id: 'skill-a', name: 'Actual skill', runAs: 'subagent', packageDigest: 'd'.repeat(64) },
      child: { jobId: 'job-a', childThreadId: null, childTurnId: null, status: 'completed', background: true, outputWithheld: true }, error: null } } }
    await draw(hydrate(item('run_skill'))); await expand()
    expect(container.textContent).toContain('Output withheld'); expect(container.textContent).not.toContain('Instructions')
    expect(container.querySelector('a,[data-tool-file-link],[data-tool-web-link]')).toBeNull()
    expect(container.textContent).not.toMatch(/Pause|Resume|Open child/)
  })
  it('enters child inspection from the actual protected Skill capture without native child metadata', async () => {
    display = childDisplay()
    const inspect = vi.fn<NonNullable<ContextType<typeof SkillChildInspectionContext>>>(async () => true), block = hydrate(item('run_skill'))
    expect(block.meta?.child).toBeUndefined()
    await drawChild(inspect, block); await expand()
    const button = container.querySelector<HTMLButtonElement>('[data-skill-child-inspect]')!
    expect(button).not.toBeNull()
    await act(async () => button.click())
    expect(inspect).toHaveBeenCalledOnce()
    const [request, signal] = inspect.mock.calls[0] as unknown as [unknown, AbortSignal]
    expect(request).toEqual({ parentThreadId: 'thread', parentTurnId: 'turn', parentToolCallId: selector.callId,
      resultItemId: selector.resultItemId, child: display.capture.kind === 'skill' ? display.capture.skill.child : null })
    expect(signal.aborted).toBe(false)
    expect(block.meta?.child).toBeUndefined()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    await act(async () => invalidate?.(null))
    expect(signal.aborted).toBe(true); expect(container.querySelector('[data-skill-child-inspect]')).toBeNull()
  })
  it.each(['missing-thread', 'foreign-source', 'foreign-scope'] as const)('does not inspect a Skill capture with %s', async variant => {
    display = childDisplay()
    if (variant === 'missing-thread' && display.capture.kind === 'skill') display.capture.skill.child!.childThreadId = null
    const block = hydrate(item('run_skill')), inspect = vi.fn<NonNullable<ContextType<typeof SkillChildInspectionContext>>>(async () => true)
    if (variant === 'foreign-source') block.meta!.toolResultIdentity = { ...block.meta!.toolResultIdentity!, callId: 'foreign-call' }
    await drawChild(inspect, block, variant === 'foreign-scope' ? 'other-thread' : 'thread'); await expand()
    expect(container.querySelector('[data-skill-child-inspect]')).toBeNull(); expect(inspect).not.toHaveBeenCalled()
  })
  it('aborts child inspection on hide and ignores its late unavailable result', async () => {
    display = childDisplay()
    let finish!: (value: boolean) => void
    const inspect = vi.fn<NonNullable<ContextType<typeof SkillChildInspectionContext>>>(() => new Promise<boolean>(resolve => { finish = resolve }))
    const block = hydrate(item('run_skill'))
    await drawChild(inspect, block); await expand()
    await act(async () => container.querySelector<HTMLButtonElement>('[data-skill-child-inspect]')!.click())
    const signal = inspect.mock.calls[0][1] as AbortSignal
    expect(signal.aborted).toBe(false)
    await drawChild(inspect, block, 'thread', false)
    expect(signal.aborted).toBe(true)
    await act(async () => finish(false))
    expect(container.querySelector('[data-skill-child-inspect]')).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_')
  })
  it('retires a replaced callback owner without leaving the Skill child button pending', async () => {
    display = childDisplay()
    let finish!: (value: boolean) => void
    const first = vi.fn<NonNullable<ContextType<typeof SkillChildInspectionContext>>>(() => new Promise(resolve => { finish = resolve }))
    const next = vi.fn<NonNullable<ContextType<typeof SkillChildInspectionContext>>>(async () => true)
    const block = hydrate(item('run_skill'))
    await drawChild(first, block); await expand()
    await act(async () => container.querySelector<HTMLButtonElement>('[data-skill-child-inspect]')!.click())
    const firstSignal = first.mock.calls[0][1]
    await drawChild(next, block)
    expect(firstSignal.aborted).toBe(true)
    const button = container.querySelector<HTMLButtonElement>('[data-skill-child-inspect]')!
    expect(button.disabled).toBe(false)
    await act(async () => button.click())
    expect(next).toHaveBeenCalledOnce()
    await act(async () => finish(false))
    expect(button.disabled).toBe(false)
    expect(container.textContent).not.toContain(i18n.t('common:protectedToolUnavailable'))
  })
  it('preserves the settled Skill call while merging later real child progress through the production grouping', async () => {
    display = privateDisplay('run_skill')
    const settled = hydrate(item('run_skill'), nativeChild)
    const progress = chatBlockFromItem({ id: 'progress-child-a', threadId: 'thread', turnId: 'turn', kind: 'tool_progress', role: 'tool',
      status: 'running', createdAt: timestamp, toolName: 'run_skill', toolKind: 'subagent', callId: selector.callId }, { ...nativeChild, childStatus: 'paused' })!
    const [section] = groupProcessSections([settled, progress])
    expect(section.blocks).toHaveLength(1)
    expect(section.blocks[0]).toMatchObject({ id: settled.id, status: 'success', toolKind: 'tool_call', meta: {
      toolName: 'run_skill', sourceItemId: selector.resultItemId, localResult: selector, child: { childStatus: 'paused' } } })
    await draw(settled, { blocks: [settled, progress] }); await expand()
    expect(container.querySelector('[data-skill-row]')).not.toBeNull(); expect(container.textContent).toContain('PRIVATE_INLINE_INSTRUCTIONS')
    expect(container.querySelector('[data-state="ok"]')).not.toBeNull()
  })
  it('keeps real native child navigation available when the historical Skill private snapshot is unavailable', async () => {
    const selectThread = vi.fn(async () => undefined)
    useChatStore.setState({ selectThread })
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: false, code: 'unavailable' })
    await draw(hydrate(item('run_skill'), nativeChild)); await expand()
    const button = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:subagentOpenSession')}"]`)!
    expect(button).not.toBeNull()
    await act(async () => button.click())
    expect(selectThread).toHaveBeenCalledWith('actual-child-thread')
    expect(container.textContent).not.toContain('PRIVATE_'); expect(row().getAttribute('aria-expanded')).toBe('true')
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('keeps Skill keyboard repeat/composition and nested controls from toggling disclosure', async () => {
    display = privateDisplay('run_skill'); await draw(hydrate(item('run_skill')))
    for (const init of [{ key: 'Enter', repeat: true }, { key: 'Enter', isComposing: true }, { key: 'Enter', keyCode: 229 }]) {
      await act(async () => row().dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, ...init })))
      expect(row().getAttribute('aria-expanded')).toBe('false')
    }
    await act(async () => row().dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true })))
    expect(row().getAttribute('aria-expanded')).toBe('true')
  })
  it.each(['create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill'] as const)('opens %s actual read-only call details without requiring a production callback', async name => {
    display = privateDisplay(name); const inspect = vi.fn(), block = hydrate(item(name))
    await draw(block); await expand()
    const initialButton = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    await act(async () => { initialButton.focus(); initialButton.click() })
    expect(inspector()?.getAttribute('data-call-id')).toBe(selector.callId)
    expect(inspector()?.textContent).toContain(selector.resultItemId)
    expect(inspector()?.textContent).toContain(display.snapshotDigest)
    expect([...inspector()!.querySelectorAll('[role="tab"]')].map(node => node.textContent)).toEqual(['Summary', 'Result'])
    await expandResult()
    expect(inspector()?.textContent).toContain(name === 'todo_write' ? 'PRIVATE_ACTIVE_TODO' : name === 'run_skill' ? 'PRIVATE_INLINE_INSTRUCTIONS' : 'PRIVATE_ACTUAL_OBJECTIVE')
    expect(inspector()?.textContent).not.toMatch(/Copy|Save|Schema|Timing/)
    const range = document.createRange(); range.selectNodeContents(inspector()!)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    for (const type of ['copy', 'cut', 'dragstart']) {
      const event = new Event(type, { bubbles: true, cancelable: true })
      await act(async () => inspector()!.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
    }
    await clickInspector('Close details'); expect(inspector()).toBeNull(); expect(document.activeElement).toBe(initialButton)
    await draw(block, { onInspect: inspect });
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    expect(button).toBeDefined()
    await act(async () => { button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })); button.click() })
    expect(inspect).toHaveBeenCalledOnce(); expect(inspect).toHaveBeenCalledWith(selector.turnId, selector.callId)
    expect(row().getAttribute('aria-expanded')).toBe('true'); expect(inspector()).toBeNull()
    await act(async () => invalidate?.(null)); expect(inspector()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_')
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it.each([['list_jobs', 'list'], ['bash_output', 'output'], ['kill_shell', 'kill'], ['wait', 'wait']] as const)(
    'dispatches %s to the %s recorded Job leaf with withheld output and actual state', async (name, operation) => {
      const job = taskJobSummaryV1({ id: 'job-a', kind: 'bash', status: 'killed', background: true })
      const result = operation === 'list' ? { jobs: [job], count: 1 } : operation === 'wait' ? { jobs: [job] } : operation === 'kill' ? { job } :
        { schemaVersion: 1, availability: 'withheld', jobId: job.id, status: 'killed', reasonCode: 'security_bound_child_output', outputWithheld: true,
          outputTrustStatus: 'untrusted_child_output', factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false }
      await draw(historicalJob(item(name, { ...host, projectionKind: 'task_job_result', taskJob: { operation, result } }, 'command_execution')))
      expect(container.querySelector('[data-recorded-job-row]')?.getAttribute('data-recorded-job-row')).toBe(operation)
      expect(container.querySelector('[data-state="ok"]')).not.toBeNull(); await expand()
      expect(container.textContent).toContain('Killed'); expect(container.textContent).toContain('Output withheld')
      expect(container.querySelector('[data-terminal],pre,a')).toBeNull(); expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
      expect(container.textContent).not.toMatch(/Copy|Save|no progress|already finished|accepted/i)
      const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
      expect(button).toBeDefined(); await act(async () => button.click())
      expect(row().getAttribute('aria-expanded')).toBe('true')
      expect(inspector()?.getAttribute('data-call-id')).toBe(selector.callId)
      expect(inspector()?.textContent).toContain(selector.resultItemId)
      expect(inspector()?.textContent).not.toContain('c'.repeat(64))
      await expandResult(); expect(inspector()?.textContent).toContain('job-a'); expect(inspector()?.textContent).toContain(operation)
      expect(inspector()?.textContent).not.toMatch(/Copy|Save|Payload|Schema|Timing/)
      await clickInspector('Close details')
      const inspect = vi.fn(); await draw(historicalJob(item(name, { ...host, projectionKind: 'task_job_result', taskJob: { operation, result } }, 'command_execution')), { onInspect: inspect })
      const delegated = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
      await act(async () => delegated.click()); expect(inspect).toHaveBeenCalledWith(selector.turnId, selector.callId); expect(inspector()).toBeNull()
      await act(async () => invalidate?.(null)); expect(inspector()).toBeNull()
    })
  it('shares one current private/public Inspect selection and retires it on reset, visibility and late replies', async () => {
    const goal = hydrate(item('create_goal'))
    const secondId = 'item_result_' + 'e'.repeat(64), secondCall = 'call_host_' + 'f'.repeat(64)
    const todo = hydrate({ ...item('todo_write'), id: secondId, callId: secondCall })
    const goalDisplay = privateDisplay('create_goal'), todoDisplay = { ...privateDisplay('todo_write'), resultItemId: secondId }
    api.openToolResultLocalDisplay.mockImplementation(async request => ({ ok: true,
      display: (request as { resultItemId: string }).resultItemId === secondId ? todoDisplay : goalDisplay }))
    const job = historicalJob({ ...item('list_jobs', { ...host, projectionKind: 'task_job_result', taskJob: {
      operation: 'list', result: { jobs: [], count: 0 } } }, 'command_execution'),
      id: 'item_result_' + '1'.repeat(64), callId: 'call_host_' + '2'.repeat(64) })
    const blocks = [goal, todo, job]
    const props = { blocks, policy: presentationPolicyFor('verbose') }
    await draw(goal, props)
    for (const node of container.querySelectorAll<HTMLElement>('[data-disclosure-row]')) await act(async () => node.click())
    const buttons = [...container.querySelectorAll<HTMLButtonElement>('button')].filter(node => node.textContent === 'Inspect')
    expect(buttons).toHaveLength(3)
    for (const index of [0, 1, 2, 0]) {
      await act(async () => buttons[index].click())
      expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
      expect(inspector()?.getAttribute('data-call-id')).toBe(blocks[index].meta?.callId)
    }
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledTimes(0)
    await draw(goal, { ...props, privateGeneration: 1, visible: false })
    expect(inspector()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledTimes(2)
    const late: ((value: ToolResultLocalViewResultV1) => void)[] = []
    api.openToolResultLocalDisplay.mockImplementation(() => new Promise(resolve => { late.push(resolve) }))
    await draw(goal, { ...props, privateGeneration: 2 }); expect(late).toHaveLength(2)
    await draw(goal, { ...props, privateGeneration: 2, visible: false })
    await act(async () => { late[0]({ ok: true, display: goalDisplay }); late[1]({ ok: true, display: todoDisplay }) })
    expect(inspector()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_')
  })
  it('retires a public Job Inspector on actual document hiding without opening a private lease', async () => {
    const block = historicalJob(item('list_jobs', { ...host, projectionKind: 'task_job_result', taskJob: {
      operation: 'list', result: { jobs: [], count: 0 } } }, 'command_execution'))
    await draw(block); await expand()
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    await act(async () => { button.focus(); button.click() }); expect(inspector()).not.toBeNull()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await act(async () => document.dispatchEvent(new Event('visibilitychange')))
    expect(inspector()).toBeNull(); expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
})
