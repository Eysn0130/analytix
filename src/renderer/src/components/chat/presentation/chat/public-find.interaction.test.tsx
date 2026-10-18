// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProcessDisplayBlockV1, ProcessDisplayUnitV2, ProcessPaneAuthorizeRequestV1, ProcessPaneReadRequestV1 } from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { ToolResultLocalDisplayV1, ToolResultLocalViewRequestV1, ToolResultLocalViewResultV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ToolResultTurnItem, type PublicToolResultProjectionV1 } from '../../../../../../../packages/runtime/src/contracts/items'
import { taskJobSummaryV1 } from '../../../../../../../packages/runtime/src/contracts/task-job-output'
import { chatBlockFromItem } from '../../../../agent/analytix-mapper'
import type { CoreTurnItemJson, CoreChildRuntimeMetadataJson } from '../../../../agent/analytix-contract'
import type { ChatBlock, RuntimeChildMetadata, ToolBlock } from '../../../../agent/types'
import i18n from '../../../../i18n'
import { useChatStore } from '../../../../store/chat-store'
import { ProtectedToolResultScope } from '../ProtectedToolResult'
import { ProtectedProcessPane, type ProcessPaneRoute } from './ProtectedProcessPane'
import { ToolCallTree, type ProcessLocalRows, type ProcessVisibilityReport } from './ToolCallTree'
import { groupProcessSections, presentationPolicyFor } from './chat-view-model'

const viewer = '11111111-1111-4111-8111-111111111111'
const scope = { threadId: 'thread', turnId: 'turn', contextDigest: 'a'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' }
const anchor = { callId: 'call-a', toolCallItemId: 'call-item-a', toolResultItemId: 'result-item-a', startedEventSeq: '10' }
const unit: ProcessDisplayUnitV2 = { unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: 'anchored', calls: [anchor], precedingCalls: [] }
const privateBlock = (expanded = false): ProcessDisplayBlockV1 => ({ blockId: 'block-a', kind: 'edit', state: 'completed', attempt: 1,
  source: { kind: 'tool', unitId: unit.unitId, ...anchor }, title: 'PRIVATE_TITLE', preview: 'PRIVATE_PREVIEW', path: '/synthetic/PRIVATE_PATH',
  durationMs: 10, captureDigest: 'b'.repeat(64), truncated: false, classification: 'credential_redacted',
  ...(expanded ? { body: { text: 'PRIVATE_BODY', before: 'PRIVATE_BEFORE', after: 'PRIVATE_AFTER', error: '', addedLines: 1, removedLines: 1, approximate: false } } : {}) })
const publicTool = (suffix: string): ToolBlock => ({ kind: 'tool', id: 'public-' + suffix, status: 'success', summary: 'Public edit ' + suffix,
  detail: 'PUBLIC_BODY_' + suffix, meta: { toolName: 'edit', turnId: 'turn',
    ...(suffix === 'a' ? { callId: anchor.callId, sourceItemId: anchor.toolResultItemId,
      sources: [{ title: 'PUBLIC_SOURCE', url: 'https://example.invalid/public' }] } : {}) } })
const blocks = [publicTool('a'), publicTool('b')]
const bashBlocks: ToolBlock[] = blocks.map(block => ({ ...block, toolKind: 'command_execution',
  summary: 'Public Bash result', meta: { ...block.meta, toolName: 'bash' } }))
const skillBlocks: ToolBlock[] = blocks.map((block, index) => ({ ...block, toolKind: 'tool_call', status: 'running',
  meta: { ...block.meta, toolName: 'run_skill', child: { kind: 'subagent', parentThreadId: 'thread', parentTurnId: 'turn',
    childId: 'skill-child-' + index, childRunId: 'skill-run-' + index, childThreadId: 'skill-thread-' + index,
    childStatus: 'running', background: true, canAcceptSteer: true } } }))
const toolSelector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const hostProjection = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
  code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } satisfies PublicToolResultProjectionV1
function mappedTool(name: string, output: unknown = hostProjection, toolKind: 'tool_call' | 'command_execution' = 'tool_call', child?: CoreChildRuntimeMetadataJson): ToolBlock {
  const item: CoreTurnItemJson = { id: toolSelector.resultItemId, threadId: 'thread', turnId: 'turn', kind: 'tool_result', role: 'tool',
    status: 'completed', toolName: name, toolKind, callId: toolSelector.callId, createdAt: '2026-10-11T00:00:00Z', output, isError: false }
  expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item, child)
  if (block?.kind !== 'tool') throw new Error('fixture must reach the actual mapper')
  return { ...block, id: 'public-a' }
}
const captureBase = { status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } as const
const skillDisplay: ToolResultLocalDisplayV1 = { schemaVersion: 1, resultItemId: toolSelector.resultItemId, snapshotDigest: 'c'.repeat(64),
  toolName: 'run_skill', canNavigate: false, capture: { ...captureBase, kind: 'skill', body: 'PRIVATE_INLINE_INSTRUCTIONS', skill: {
    requestedName: 'PRIVATE_REQUESTED_SKILL', resolved: { id: 'skill-a', name: 'PRIVATE_RESOLVED_SKILL', runAs: 'inline', packageDigest: 'd'.repeat(64) }, child: null, error: null } } }
function mixedSkill() {
  const block = mappedTool('run_skill', hostProjection, 'tool_call', { parentThreadId: 'thread', parentTurnId: 'turn', parentToolCallId: toolSelector.callId,
    childId: 'child-a', kind: 'subagent', childStatus: 'running', childThreadId: 'actual-child-thread', childTurnId: 'actual-child-turn',
    background: true, canAcceptSteer: true })
  expect(block.meta?.localResult).toEqual(toolSelector)
  return block
}
function publicJob(local = false) {
  const job = taskJobSummaryV1({ id: 'PUBLIC_JOB', kind: 'bash', status: 'running', background: true })
  const block = mappedTool('list_jobs', { ...hostProjection, projectionKind: 'task_job_result', taskJob: { operation: 'list', result: { jobs: [job], count: 1 } } }, 'command_execution')
  if (local) return block
  const { localResult: _privateReference, ...meta } = block.meta ?? {}
  return { ...block, meta }
}
function withSecond(block: ToolBlock): ToolBlock[] {
  const callId = 'call_host_' + 'e'.repeat(64), child = block.meta?.child as RuntimeChildMetadata | undefined
  const second: ToolBlock = { ...block, id: 'public-b', meta: { ...block.meta, localResult: undefined,
    callId, sourceItemId: 'item_result_' + 'f'.repeat(64), toolResultIdentity: undefined,
    ...(child ? { child: { ...child, parentToolCallId: callId, childId: child.childId + '-second',
      childRunId: (child.childRunId ?? child.childId) + '-second',
      childThreadId: (child.childThreadId ?? 'child-thread') + '-second',
      childTurnId: (child.childTurnId ?? 'child-turn') + '-second' } } : {}) } }
  const result = [block, second]
  // Both seats must survive native child deduplication in the same foldable group.
  if (child) expect(groupProcessSections(result).map(section => section.blocks.map(seat => seat.id)))
    .toEqual([['public-a', 'public-b']])
  return result
}
const privateShellBlock = (expanded = false): ProcessDisplayBlockV1 => ({ blockId: 'block-a', kind: 'shell', state: 'completed', attempt: 1,
  source: { kind: 'tool', unitId: unit.unitId, ...anchor }, title: 'PRIVATE_TITLE', preview: 'PRIVATE_PREVIEW',
  durationMs: 10, exitCode: 0, captureDigest: 'b'.repeat(64), truncated: false, classification: 'credential_redacted',
  ...(expanded ? { body: privateBlock(true).body } : {}) })
type PublicFamily = 'edit' | 'bash'
const familyBlocks = (family: PublicFamily) => family === 'bash' ? bashBlocks : blocks
const route = (visible = true): ProcessPaneRoute => ({ surface: 'main', ownerKey: 'main', selectionKey: 'selection', visible })
function makeApi(displayBlock: (expanded: boolean) => ProcessDisplayBlockV1 = privateBlock) {
  let expanded = false
  return {
    authorizeProcessPane: vi.fn(async (request: ProcessPaneAuthorizeRequestV1) => ({ status: 'authorized', schemaVersion: 2,
      viewerId: viewer, revision: '1', scope: { ...scope, threadId: request.threadId, turnId: request.turnId }, presence: { nativeBlockCount: 0, toolCaptureCount: 1 } })),
    readProcessPane: vi.fn(async (_request: ProcessPaneReadRequestV1) => ({ status: 'snapshot', schemaVersion: 2,
      revision: '2', scope, units: [unit], blocks: [displayBlock(expanded)] })),
    setProcessPaneExpanded: vi.fn(async (request: { expandedBlockIds: string[] }) => { expanded = request.expandedBlockIds.includes('block-a'); return { status: 'ok' } }),
    closeProcessPane: vi.fn(async () => ({ status: 'ok' })),
    openProcessCapture: vi.fn(async () => ({ status: 'unavailable' })),
    openProcessWebLink: vi.fn(async () => ({ status: 'ok' })),
    openToolResultLocalDisplay: vi.fn(async (_request: ToolResultLocalViewRequestV1): Promise<ToolResultLocalViewResultV1> => ({ ok: false, code: 'unavailable' })),
    closeToolResultLocalDisplay: vi.fn(async (_id: string) => undefined),
    onToolResultLocalDisplayInvalidated: vi.fn(() => () => undefined),
    onProcessPaneInvalidated: vi.fn((_request: unknown, handler: () => void) => { invalidators.add(handler); return () => invalidators.delete(handler) }),
    onRuntimeStatus: vi.fn(() => () => undefined)
  }
}
let root: Root, container: HTMLDivElement, api: ReturnType<typeof makeApi>, invalidators: Set<() => void>
let previousApi: PropertyDescriptor | undefined, previousStore: ReturnType<typeof useChatStore.getState>
const onToggle = vi.fn(), onRequestProcessDisplay = vi.fn()
function configureFamilyCapture(family: PublicFamily) {
  api = makeApi(family === 'bash' ? privateShellBlock : privateBlock)
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
}
async function renderPane(props: Partial<Parameters<typeof ProtectedProcessPane>[0]> = {}) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(ProtectedProcessPane, {
    route: route(), threadId: 'thread', turnId: 'turn', processOpen: true, processBlocks: blocks, processing: false,
    status: 'completed', runtimeAvailable: true, policy: presentationPolicyFor('verbose'), onToggle, onRequestProcessDisplay, ...props
  }))))
}
async function renderTree(props: Partial<Parameters<typeof ToolCallTree>[0]> = {}) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(ToolCallTree, {
    blocks, processing: false, policy: presentationPolicyFor('compact'), ...props
  }))))
}
const group = () => container.querySelector<HTMLElement>('[data-process-group="public:public-a"]')!
const groupBody = () => group().querySelector<HTMLDivElement>('[data-step-process-body]')!
const groupToggle = () => group().querySelector<HTMLButtonElement>(':scope > div:first-child button')!
const publicSeat = () => container.querySelector<HTMLElement>('[data-chat-seat="public-a"]')!
const publicRow = () => publicSeat().querySelector<HTMLElement>('[data-disclosure-row]')!
const rowBody = () => document.getElementById(publicRow().getAttribute('aria-controls')!)!
const hostCounts = () => [api.authorizeProcessPane, api.readProcessPane, api.setProcessPaneExpanded, api.closeProcessPane,
  api.openProcessCapture, api.openProcessWebLink, api.openToolResultLocalDisplay].map(method => method.mock.calls.length)
async function findPublic(target: HTMLElement = publicSeat()) {
  await act(async () => target.dispatchEvent(new Event('beforematch', { bubbles: true })))
}
async function draftSteer() {
  const input = publicSeat().querySelector<HTMLInputElement>('[data-dsh-subagent-details] input')!
  expect(input).not.toBeNull(); input.focus({ preventScroll: true })
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'UNSUBMITTED_PUBLIC_STEER')
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  expect(input.closest('form')?.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
  return input
}
const publicInspectButton = () => [...rowBody().querySelectorAll<HTMLButtonElement>('button')]
  .find(button => button.textContent === i18n.t('dshToolInspect.inspect'))!
function expectNoPrivatePayload() {
  expect(container.innerHTML).not.toContain('PRIVATE_')
  expect(container.querySelector('[data-protected-process-block],[data-protected-process-body],[data-protected-assistant-draft],'
    + '[data-protected-tool-result],[data-protected-tool-body],[data-protected-user-input],[role="dialog"]')).toBeNull()
}
function localRows() {
  const setOpen = vi.fn()
  const capture = vi.fn<ProcessLocalRows['capture']>(() => ({ blockId: 'block-a', preview: 'PRIVATE_PREVIEW', body: null,
    expanded: false, expandable: true, setOpen }))
  const render = vi.fn<ProcessLocalRows['render']>(() => createElement('span', { 'data-protected-process-block': 'block-a' }, 'PRIVATE_RENDER'))
  const local: ProcessLocalRows = { threadId: 'thread', turnId: 'turn', units: [unit], identities: [{ blockId: 'block-a', source: privateBlock().source, state: 'completed' }],
    blocks: [privateBlock()], capture, render }
  return { local, capture, render, setOpen }
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.useFakeTimers(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  await i18n.changeLanguage('zh'); invalidators = new Set(); api = makeApi(); onToggle.mockClear(); onRequestProcessDisplay.mockClear()
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  previousStore = useChatStore.getState(); useChatStore.setState({ activeThreadId: 'thread', route: 'chat', activeThreadGoal: null })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix')
  useChatStore.setState(previousStore); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('public find and focus without private display authority', () => {
  it('reveals a retained public group repeatedly without private visibility, capture or manual open intent', async () => {
    const { local, capture, render } = localRows(), onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ local, onVisibility })
    const seat = publicSeat(), body = groupBody(), control = groupToggle()
    const report = { kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0, draftsOnly: true }
    expect(body.getAttribute('hidden')).toBe('until-found')
    expect(onVisibility).toHaveBeenLastCalledWith(report)
    Object.defineProperties(body, { clientHeight: { configurable: true, value: 100 }, scrollHeight: { configurable: true, value: 500 } })
    body.scrollTop = 42
    await findPublic(); await findPublic()
    expect(publicSeat()).toBe(seat); expect(groupBody()).toBe(body); expect(groupToggle()).toBe(control)
    expect(body.hasAttribute('hidden')).toBe(false); expect(control.getAttribute('aria-expanded')).toBe('true')
    expect(body.scrollTop).toBe(42)
    for (const [value] of onVisibility.mock.calls) expect(value).toEqual(report)
    expect(capture).not.toHaveBeenCalled(); expect(render).not.toHaveBeenCalled(); expectNoPrivatePayload()
    // A leaf's own closed body stays unmounted; only retained group DOM is searchable.
    expect(container.textContent).not.toContain('PUBLIC_BODY_a')
  })

  it('reveals historical public group and whole wrappers without opening the ChatView private entry', async () => {
    // ChatView gates historical private acquisition with route.visible=false and this separate explicit entry.
    await renderPane({ route: route(false), processOpen: false, offerProcessEntry: true, requestProcessDisplay: true, policy: presentationPolicyFor('compact') })
    const whole = container.querySelector<HTMLDivElement>('[data-public-process-body]')!, seat = publicSeat()
    expect(whole.getAttribute('hidden')).toBe('until-found'); expect(groupBody().getAttribute('hidden')).toBe('until-found')
    await findPublic(); await findPublic()
    expect(publicSeat()).toBe(seat); expect(whole.hasAttribute('hidden')).toBe(false); expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
    const header = container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!
    expect(header.getAttribute('aria-expanded')).toBe('true')
    await act(async () => header.click())
    expect(document.activeElement).toBe(header); expect(whole.getAttribute('hidden')).toBe('until-found')
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it('keeps a terminal error visible while public find reveals only folded public siblings', async () => {
    const error: ChatBlock = { kind: 'system', id: 'persistent-error', severity: 'error', text: 'Safe failure' }
    await renderPane({ route: route(false), processOpen: false, offerProcessEntry: true, requestProcessDisplay: true,
      policy: presentationPolicyFor('compact'), processBlocks: [...blocks, error] })
    const errorSeat = container.querySelector('[data-chat-seat="persistent-error"]')!
    expect(errorSeat.closest('[hidden]')).toBeNull()
    expect(groupBody().getAttribute('hidden')).toBe('until-found')
    await findPublic()
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(container.querySelector('[data-public-process-body]')?.hasAttribute('hidden')).toBe(false)
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
  })

  it.each(['edit', 'bash'] as const)('preserves focused expanded public %s details before a whole fold resets the view generation', async family => {
    const processBlocks = familyBlocks(family)
    await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks })
    await act(async () => publicRow().click())
    const seat = publicSeat(), body = rowBody(), link = body.querySelector<HTMLAnchorElement>('a')!
    expect(body.textContent).toContain('PUBLIC_BODY_a'); expect(link).not.toBeNull()
    link.focus({ preventScroll: true })
    await renderPane({ route: route(false), requestProcessDisplay: true, processOpen: false, policy: presentationPolicyFor('compact'), processBlocks })
    expect(publicSeat()).toBe(seat); expect(rowBody()).toBe(body); expect(document.activeElement).toBe(link)
    expect(container.querySelector('[data-public-process-body]')?.hasAttribute('hidden')).toBe(false)
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
  })

  it('keeps public body focus through a mode fold while retiring a mixed private preview', async () => {
    const { local, capture, setOpen } = localRows(), onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ local, onVisibility, policy: presentationPolicyFor('verbose') })
    await act(async () => publicRow().click())
    const body = rowBody(), link = body.querySelector<HTMLAnchorElement>('a')!
    expect(container.textContent).toContain('PRIVATE_PREVIEW'); link.focus({ preventScroll: true })
    await renderTree({ local, onVisibility, policy: presentationPolicyFor('compact') })
    expect(rowBody()).toBe(body); expect(document.activeElement).toBe(link); expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(onVisibility).toHaveBeenLastCalledWith({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0, draftsOnly: true })
    expectNoPrivatePayload()
    const captures = capture.mock.calls.length
    setOpen.mockClear()
    await findPublic()
    expect(capture).toHaveBeenCalledTimes(captures)
    await renderTree({ local, onVisibility, policy: presentationPolicyFor('verbose') })
    expect(rowBody()).toBe(body); expect(container.textContent).toContain('PRIVATE_PREVIEW')
    expect(setOpen).not.toHaveBeenCalled()
  })

  it('keeps a public Bash source focused across mode folding', async () => {
    const onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ blocks: bashBlocks, policy: presentationPolicyFor('verbose'), onVisibility })
    await act(async () => publicRow().click())
    const body = rowBody(), link = body.querySelector<HTMLAnchorElement>('a')!
    expect(publicSeat().querySelector('[data-sample="bash"]')).not.toBeNull()
    link.focus({ preventScroll: true })
    await renderTree({ blocks: bashBlocks, policy: presentationPolicyFor('compact'), onVisibility })
    expect(rowBody()).toBe(body); expect(document.activeElement).toBe(link)
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(onVisibility).toHaveBeenLastCalledWith({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0, draftsOnly: true })
    await findPublic()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
  })

  it('keeps a public Bash source focused before a reset folds its group without repeating private open intent', async () => {
    const onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ blocks: bashBlocks, onVisibility, resetKey: 'original' })
    await act(async () => groupToggle().click())
    await act(async () => publicRow().click())
    const body = rowBody(), link = body.querySelector<HTMLAnchorElement>('a')!
    link.focus({ preventScroll: true })
    await renderTree({ blocks: bashBlocks, onVisibility, resetKey: 'folded' })
    expect(rowBody()).toBe(body); expect(document.activeElement).toBe(link)
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    expect(onVisibility).toHaveBeenLastCalledWith({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 1, draftsOnly: true })
    const counts = hostCounts(); await findPublic(); await findPublic()
    expect(hostCounts()).toEqual(counts); expectNoPrivatePayload()
  })

  it.each(['mode', 'whole', 'group'] as const)('retains a running Skill child steer input and local draft through %s folding', async mode => {
    if (mode === 'whole') await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks: skillBlocks })
    else await renderTree({ blocks: skillBlocks, policy: presentationPolicyFor(mode === 'group' ? 'compact' : 'verbose'), resetKey: 'original' })
    if (mode === 'group') await act(async () => groupToggle().click())
    await act(async () => publicRow().click())
    const body = rowBody(), input = await draftSteer()
    if (mode === 'whole') await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks: skillBlocks,
      processOpen: false, policy: presentationPolicyFor('compact') })
    else await renderTree({ blocks: skillBlocks, policy: presentationPolicyFor('compact'), resetKey: mode === 'group' ? 'folded' : 'original' })
    expect(rowBody()).toBe(body); expect(publicSeat().querySelector('[data-dsh-subagent-details] input')).toBe(input)
    expect(document.activeElement).toBe(input); expect(input.value).toBe('UNSUBMITTED_PUBLIC_STEER')
    expect(input.closest('form')?.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    await findPublic()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
  })

  it.each(['pointer', 'Enter', ' '])('allows explicit Skill disclosure close after retaining a focused steer draft (%s)', async action => {
    await renderTree({ blocks: skillBlocks, policy: presentationPolicyFor('verbose') })
    await act(async () => publicRow().click())
    const body = rowBody(), input = await draftSteer()
    await renderTree({ blocks: skillBlocks, policy: presentationPolicyFor('compact') })
    expect(document.activeElement).toBe(input); expect(input.value).toBe('UNSUBMITTED_PUBLIC_STEER')
    const header = publicRow(); header.focus()
    await act(async () => {
      if (action === 'pointer') header.click()
      else header.dispatchEvent(new KeyboardEvent('keydown', { key: action, bubbles: true, cancelable: true }))
    })
    expect(header.getAttribute('aria-expanded')).toBe('false'); expect(body.isConnected).toBe(false); expect(input.isConnected).toBe(false)
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
  })

  it.each(['mode', 'whole', 'generation'] as const)('keeps one public Skill steer owner while its actual private lease retires on %s', async mode => {
    const toolBlocks = withSecond(mixedSkill())
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: skillDisplay })
    if (mode === 'whole') await renderPane({ processBlocks: toolBlocks })
    else await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose'), privateGeneration: 0 })
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    const retiredViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    await act(async () => publicRow().click())
    expect(publicSeat().textContent).toContain('PRIVATE_INLINE_INSTRUCTIONS')
    const childBody = publicSeat().querySelector('[data-dsh-subagent-details]')!, input = await draftSteer()
    expect(input.closest('[data-protected-tool-result]')).toBeNull()
    if (mode === 'whole') await renderPane({ processBlocks: toolBlocks, processOpen: false, policy: presentationPolicyFor('compact') })
    else await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor(mode === 'generation' ? 'verbose' : 'compact'), privateGeneration: mode === 'generation' ? 1 : 0 })
    expect(publicSeat().querySelector('[data-dsh-subagent-details]')).toBe(childBody)
    expect(publicSeat().querySelectorAll('[data-dsh-subagent-details]')).toHaveLength(1)
    expect(document.activeElement).toBe(input); expect(input.value).toBe('UNSUBMITTED_PUBLIC_STEER')
    expect(input.closest('form')?.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(false)
    expect(container.innerHTML).not.toContain('PRIVATE_INLINE_INSTRUCTIONS'); expect(container.innerHTML).not.toContain('PRIVATE_RESOLVED_SKILL')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retiredViewId)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(mode === 'generation' ? 2 : 1)
    const counts = hostCounts(); await findPublic(); expect(hostCounts()).toEqual(counts)
  })

  it('ignores a late private Skill preload after folding while retaining the public steer draft', async () => {
    const toolBlocks = withSecond(mixedSkill())
    let finish!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose') })
    const retiredViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    await act(async () => publicRow().click())
    const input = await draftSteer()
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact') })
    const counts = hostCounts()
    await act(async () => finish({ ok: true, display: skillDisplay })); await findPublic()
    expect(document.activeElement).toBe(input); expect(input.value).toBe('UNSUBMITTED_PUBLIC_STEER')
    expect(container.innerHTML).not.toContain('PRIVATE_'); expect(hostCounts()).toEqual(counts)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retiredViewId)
  })

  it('retires focused private Skill instructions rather than preserving them through the public child', async () => {
    const toolBlocks = withSecond(mixedSkill())
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: skillDisplay })
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    const retiredViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    const body = publicSeat().querySelector<HTMLElement>('[data-protected-tool-body]')!, control = document.createElement('button')
    body.append(control); control.focus()
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact') })
    expect(body.isConnected).toBe(false); expect(control.isConnected).toBe(false); expect(container.innerHTML).not.toContain('PRIVATE_')
    expect(publicSeat().querySelector('[data-dsh-subagent-details] input')).toBeNull()
    const counts = hostCounts(); await findPublic(); expect(hostCounts()).toEqual(counts)
    // Find can reveal the still-running public child after the private focus retires.
    const input = publicSeat().querySelector<HTMLInputElement>('[data-dsh-subagent-details] input')!
    expect(input).not.toBeNull(); expect(input.closest('[data-protected-tool-result],[data-protected-tool-body]')).toBeNull()
    expect(container.innerHTML).not.toContain('PRIVATE_')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retiredViewId)
  })

  it('opens strictly admitted public Job details and readonly Inspector after find with zero private Host effects', async () => {
    const processBlocks = withSecond(publicJob())
    await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks, processOpen: false, policy: presentationPolicyFor('compact') })
    await findPublic(); await act(async () => publicRow().click())
    expect(rowBody().textContent).toContain('PUBLIC_JOB'); expect(publicInspectButton()).toBeDefined()
    await act(async () => publicInspectButton().click())
    const inspector = publicSeat().querySelector<HTMLElement>('[data-public-job-inspector] [data-tool-result-inspector]')!
    expect(inspector).not.toBeNull(); expect(inspector.hasAttribute('data-protected-tool-readonly')).toBe(true)
    const resultTab = [...inspector.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(button => button.textContent === i18n.t('dshToolInspect.result'))!
    await act(async () => resultTab.click())
    expect(inspector.querySelector('[data-protected-tool-copy-section]')).toBeNull()
    expect(inspector.querySelectorAll('[role="tab"]')).toHaveLength(2)
    expect(inspector.textContent).not.toMatch(/PRIVATE_|Copy|Save|复制|保存/)
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
  })

  it.each(['mode', 'whole', 'group'] as const)('retains the public Job details control through %s folding', async mode => {
    const toolBlocks = withSecond(publicJob())
    if (mode === 'whole') await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks: toolBlocks })
    else await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor(mode === 'group' ? 'compact' : 'verbose'), resetKey: 'original' })
    if (mode === 'group') await act(async () => groupToggle().click())
    await act(async () => publicRow().click())
    const body = rowBody(), inspect = publicInspectButton(); inspect.focus()
    if (mode === 'whole') await renderPane({ route: route(false), requestProcessDisplay: true, processBlocks: toolBlocks, processOpen: false, policy: presentationPolicyFor('compact') })
    else await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact'), resetKey: mode === 'group' ? 'folded' : 'original' })
    expect(rowBody()).toBe(body); expect(document.activeElement).toBe(inspect); expect(groupBody().hasAttribute('hidden')).toBe(false)
    await act(async () => inspect.click())
    expect(publicSeat().querySelector('[data-public-job-inspector]')).not.toBeNull()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it.each(['mode', 'generation', 'reset'] as const)('preserves focused public Job Inspector tabs through a %s reset and permits explicit close', async mode => {
    const toolBlocks = withSecond(publicJob())
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose'), resetKey: 'original', privateGeneration: 0 }); await act(async () => publicRow().click())
    await act(async () => publicInspectButton().click())
    const inspector = publicSeat().querySelector<HTMLElement>('[data-tool-result-inspector]')!, tab = inspector.querySelector<HTMLButtonElement>('[role="tab"]')!
    expect(document.activeElement).toBe(tab)
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact'), resetKey: mode === 'reset' ? 'folded' : 'original',
      privateGeneration: mode === 'generation' ? 1 : 0 })
    expect(publicSeat().querySelector('[data-tool-result-inspector]')).toBe(inspector); expect(document.activeElement).toBe(tab)
    expect(groupBody().hasAttribute('hidden')).toBe(false)
    await act(async () => inspector.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })))
    expect(inspector.isConnected).toBe(false); expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it('retires a focused public Job Inspector when the actual thread owner changes', async () => {
    const toolBlocks = withSecond(publicJob())
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    await act(async () => publicInspectButton().click())
    const inspector = publicSeat().querySelector<HTMLElement>('[data-tool-result-inspector]')!
    expect(inspector.contains(document.activeElement)).toBe(true)
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'other-thread' }, createElement(ToolCallTree,
      { blocks: toolBlocks, processing: false, policy: presentationPolicyFor('verbose') }))))
    expect(inspector.isConnected).toBe(false); expect(container.querySelector('[data-public-job-inspector]')).toBeNull()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it('retires a focused private Skill Inspector when the private generation changes', async () => {
    const toolBlocks = withSecond(mixedSkill())
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: skillDisplay })
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose'), privateGeneration: 0 })
    const retiredViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    await act(async () => publicRow().click())
    const inspect = [...publicSeat().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('dshToolInspect.inspect'))!
    await act(async () => inspect.click())
    const inspector = publicSeat().querySelector<HTMLElement>('[data-tool-result-inspector]')!
    expect(inspector).not.toBeNull(); expect(inspector.closest('[data-protected-tool-result]')).not.toBeNull()
    expect(inspector.contains(document.activeElement)).toBe(true)
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose'), privateGeneration: 1 })
    expect(inspector.isConnected).toBe(false); expect(container.innerHTML).not.toContain('PRIVATE_')
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull(); expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retiredViewId)
  })

  it('retires a focused standalone public Job Inspector when its local turn owner changes', async () => {
    const toolBlocks = withSecond(publicJob()), { local } = localRows()
    await renderTree({ blocks: toolBlocks, local, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    await act(async () => publicInspectButton().click())
    const inspector = publicSeat().querySelector<HTMLElement>('[data-tool-result-inspector]')!
    expect(inspector.contains(document.activeElement)).toBe(true)
    await renderTree({ blocks: toolBlocks, local: { ...local, turnId: 'next-turn' }, policy: presentationPolicyFor('verbose') })
    expect(inspector.isConnected).toBe(false); expect(container.querySelector('[data-public-job-inspector]')).toBeNull()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it.each(['malformed', 'operation', 'identity'] as const)('does not grant public Job expansion to a %s projection', async variant => {
    // Inject malformed boundary data deliberately; production admission must still reject it.
    const original = publicJob(), block: ToolBlock = { ...original, detail: 'UNVALIDATED_RAW_JOB_TEXT', meta: { ...original.meta,
      ...(variant === 'malformed' ? { publicResult: { ...hostProjection, projectionKind: 'task_job_result', taskJob: { operation: 'list', result: { jobs: 'invalid', count: 1 } } } as unknown as PublicToolResultProjectionV1 }
        : variant === 'operation' ? { toolName: 'wait' } : { callId: 'unadmitted-call' }) } }
    await renderTree({ blocks: withSecond(block), visible: false, publicVisible: true })
    await findPublic()
    expect(publicRow().getAttribute('aria-expanded')).toBeNull()
    await act(async () => publicRow().click())
    expect(publicSeat().querySelector('[data-tool-details],[data-public-job-inspector]')).toBeNull()
    expect(publicSeat().textContent).not.toContain('UNVALIDATED_RAW_JOB_TEXT')
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })

  it('retires the actual private Job body before allowing its separate admitted public fallback', async () => {
    const toolBlocks = withSecond(publicJob(true)), display: ToolResultLocalDisplayV1 = { schemaVersion: 1, resultItemId: toolSelector.resultItemId,
      snapshotDigest: 'c'.repeat(64), toolName: 'list_jobs', canNavigate: false, capture: { ...captureBase, kind: 'job', job: { operation: 'list',
        jobs: [{ jobId: 'PRIVATE_JOB', kind: 'bash', status: 'running', name: 'PRIVATE_JOB_NAME', label: null, output: { availability: 'unavailable' } }],
        total: 1, truncated: false, receipt: null } } }
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display })
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    const retiredViewId = api.openToolResultLocalDisplay.mock.calls[0][0].viewId
    const body = publicSeat().querySelector<HTMLElement>('[data-protected-tool-body]')!, control = document.createElement('button')
    expect(body.textContent).toContain('PRIVATE_JOB_NAME'); body.append(control); control.focus()
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact') })
    expect(body.isConnected).toBe(false); expect(control.isConnected).toBe(false); expect(container.innerHTML).not.toContain('PRIVATE_')
    const counts = hostCounts(); await findPublic(); await act(async () => publicRow().click())
    expect(rowBody().textContent).toContain('PUBLIC_JOB'); await act(async () => publicInspectButton().click())
    expect(publicSeat().querySelector('[data-public-job-inspector]')).not.toBeNull()
    expect(container.innerHTML).not.toContain('PRIVATE_'); expect(hostCounts()).toEqual(counts)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(retiredViewId)
  })

  it.each(['edit', 'bash'] as const)('clears an expanded private %s capture on whole fold and does not recover it through public find', async family => {
    configureFamilyCapture(family)
    const processBlocks = familyBlocks(family)
    await renderPane({ processBlocks })
    if (family === 'edit') expect(container.textContent).toContain('PRIVATE_PREVIEW')
    expect(publicSeat().querySelector('[data-tool-view]')?.getAttribute('data-tool-view')).toBe(family === 'bash' ? 'shell' : 'diff')
    await act(async () => publicRow().click())
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: ['block-a'] })
    expect(container.innerHTML).toContain('PRIVATE_BODY')
    const privateBody = container.querySelector<HTMLElement>('[data-protected-process-body]')!, privateControl = document.createElement('button')
    if (family === 'bash') expect(privateBody.querySelector('[data-terminal]')).not.toBeNull()
    privateBody.append(privateControl); privateControl.focus()
    await renderPane({ processOpen: false, policy: presentationPolicyFor('compact'), processBlocks })
    expect(api.setProcessPaneExpanded).toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: [] }); expectNoPrivatePayload()
    expect(privateControl.isConnected).toBe(false)
    expect(container.querySelector('[data-public-process-body]')?.getAttribute('hidden')).toBe('until-found')
    const counts = hostCounts()
    await findPublic(); await findPublic()
    expect(container.querySelector('[data-public-process-body]')?.hasAttribute('hidden')).toBe(false)
    expect(groupBody().hasAttribute('hidden')).toBe(false); expect(hostCounts()).toEqual(counts)
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled(); expectNoPrivatePayload()
    await renderPane({ policy: presentationPolicyFor('verbose'), processBlocks })
    if (family === 'edit') expect(container.textContent).toContain('PRIVATE_PREVIEW')
    expect(container.textContent).not.toContain('PRIVATE_BODY')
    expect(api.setProcessPaneExpanded).not.toHaveBeenLastCalledWith({ viewerId: viewer, expandedBlockIds: ['block-a'] })
  })

  it.each(['edit', 'bash'] as const)('clears focused private %s content on revocation and find cannot reacquire its lease', async family => {
    configureFamilyCapture(family)
    const processBlocks = familyBlocks(family)
    await renderPane({ processBlocks }); await act(async () => publicRow().click())
    const privateBody = container.querySelector<HTMLElement>('[data-protected-process-body]')!
    expect(privateBody).not.toBeNull()
    const control = document.createElement('button'); privateBody.append(control); control.focus()
    await act(async () => [...invalidators][0]())
    expectNoPrivatePayload(); expect(control.isConnected).toBe(false)
    await renderPane({ processOpen: false, policy: presentationPolicyFor('compact'), processBlocks })
    const counts = hostCounts(); await findPublic()
    expect(hostCounts()).toEqual(counts); expectNoPrivatePayload()
  })

  it.each(['edit', 'bash'] as const)('fences a late private %s reply after revoke even when browser find reveals the public wrappers', async family => {
    configureFamilyCapture(family)
    const processBlocks = familyBlocks(family)
    let finish!: (value: Awaited<ReturnType<typeof api.readProcessPane>>) => void
    api.readProcessPane.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await renderPane({ processBlocks }); expect(api.readProcessPane).toHaveBeenCalledOnce()
    await renderPane({ processOpen: false, policy: presentationPolicyFor('compact'), processBlocks })
    await act(async () => [...invalidators][0]())
    const counts = hostCounts(); await findPublic()
    await act(async () => finish({ status: 'snapshot', schemaVersion: 2, revision: '3', scope, units: [unit], blocks: [(family === 'bash' ? privateShellBlock : privateBlock)(true)] }))
    expect(hostCounts()).toEqual(counts); expectNoPrivatePayload()
    expect(container.querySelector('[data-public-process-body]')?.hasAttribute('hidden')).toBe(false)
  })

  it('retires the old public owner and its beforematch listener on a thread transition', async () => {
    const props = { route: route(false), processOpen: false, requestProcessDisplay: true, policy: presentationPolicyFor('compact') }
    await renderPane(props)
    const oldBody = groupBody(), oldSeat = publicSeat()
    await renderPane({ ...props, threadId: 'other-thread', turnId: 'other-turn' })
    const nextBody = groupBody(), nextSeat = publicSeat(), nextWhole = container.querySelector('[data-public-process-body]')!
    expect(oldSeat.isConnected).toBe(false); expect(nextSeat).not.toBe(oldSeat)
    await act(async () => oldBody.dispatchEvent(new Event('beforematch', { bubbles: true })))
    expect(nextBody.getAttribute('hidden')).toBe('until-found'); expect(nextWhole.getAttribute('hidden')).toBe('until-found')
    expect(onToggle).not.toHaveBeenCalled(); expect(onRequestProcessDisplay).not.toHaveBeenCalled()
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
  })

  it('retires a focused private Bash terminal across a real pane generation change', async () => {
    configureFamilyCapture('bash')
    await renderPane({ processBlocks: bashBlocks }); await act(async () => publicRow().click())
    const privateBody = container.querySelector<HTMLElement>('[data-protected-process-body]')!, terminal = privateBody.querySelector<HTMLElement>('[data-terminal]')!
    const privateControl = document.createElement('button')
    expect(terminal).not.toBeNull(); terminal.append(privateControl); privateControl.focus()
    const oldGroupBody = groupBody()
    await renderPane({ processBlocks: bashBlocks, route: { ...route(), selectionKey: 'next-selection' } })
    expect(privateBody.isConnected).toBe(false); expect(privateControl.isConnected).toBe(false)
    expect(container.querySelector('[data-terminal]')).toBeNull(); expect(container.innerHTML).not.toContain('PRIVATE_BODY')
    expect(api.authorizeProcessPane).toHaveBeenCalledTimes(2)
    expect(api.setProcessPaneExpanded).toHaveBeenCalledTimes(1)
    const counts = hostCounts()
    await findPublic()
    await act(async () => oldGroupBody.dispatchEvent(new Event('beforematch', { bubbles: true })))
    expect(hostCounts()).toEqual(counts)
    expect(container.querySelector('[data-terminal]')).toBeNull(); expect(container.innerHTML).not.toContain('PRIVATE_BODY')
  })

  it('closes a focus-preserved public group from its header without opening a folded private capture', async () => {
    const { local, capture } = localRows(), onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ local, onVisibility, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    rowBody().querySelector<HTMLAnchorElement>('a')!.focus()
    await renderTree({ local, onVisibility, policy: presentationPolicyFor('compact') })
    const header = groupToggle(), body = groupBody(), captures = capture.mock.calls.length
    expect(header.getAttribute('aria-expanded')).toBe('true')
    await act(async () => header.click())
    expect(document.activeElement).toBe(header); expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(body.getAttribute('hidden')).toBe('until-found'); expect(capture).toHaveBeenCalledTimes(captures)
    expect(onVisibility).toHaveBeenLastCalledWith({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0, draftsOnly: true })
    expectNoPrivatePayload()
  })

  it('closes a focus-preserved public Bash group from its header', async () => {
    const onVisibility = vi.fn<(report: ProcessVisibilityReport) => void>()
    await renderTree({ blocks: bashBlocks, policy: presentationPolicyFor('verbose'), onVisibility })
    await act(async () => publicRow().click()); rowBody().querySelector<HTMLAnchorElement>('a')!.focus()
    await renderTree({ blocks: bashBlocks, policy: presentationPolicyFor('compact'), onVisibility })
    const header = groupToggle(), body = groupBody()
    expect(header.getAttribute('aria-expanded')).toBe('true')
    await act(async () => header.click())
    expect(document.activeElement).toBe(header); expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(body.getAttribute('hidden')).toBe('until-found')
    expect(onVisibility).toHaveBeenLastCalledWith({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0, draftsOnly: true })
    expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0]); expectNoPrivatePayload()
  })

  it.each((['edit', 'bash'] as const).flatMap(family => ['pointer', 'Enter', ' '].map(action => ({ family, action }))))('keeps explicit public $family disclosure close working after focus protection ($action)', async ({ family, action }) => {
    const toolBlocks = familyBlocks(family)
    await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('verbose') }); await act(async () => publicRow().click())
    const row = publicRow(), body = rowBody(), link = body.querySelector<HTMLAnchorElement>('a')!
    link.focus(); await renderTree({ blocks: toolBlocks, policy: presentationPolicyFor('compact') })
    expect(rowBody()).toBe(body); expect(document.activeElement).toBe(link)
    row.focus()
    await act(async () => {
      if (action === 'pointer') row.click()
      else row.dispatchEvent(new KeyboardEvent('keydown', { key: action, bubbles: true, cancelable: true }))
    })
    expect(row.getAttribute('aria-expanded')).toBe('false'); expect(body.isConnected).toBe(false)
    expect(container.textContent).not.toContain('PUBLIC_BODY_a'); expect(hostCounts()).toEqual([0, 0, 0, 0, 0, 0, 0])
  })
})
