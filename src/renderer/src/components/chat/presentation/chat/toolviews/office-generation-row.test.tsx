// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PublicToolCallArgumentsProjectionV1, ToolCallTurnItem, ToolResultTurnItem } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ToolResultLocalDisplayV1, ToolResultLocalViewResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { presentationPolicyFor } from '../chat-view-model'

const timestamp = '2026-10-11T00:00:00Z'
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
  code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
const receipt = { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64), byteSize: 2048, savedAt: timestamp }
function hydrate(variant: 'success' | 'running' | 'error' = 'success', toolKind: NonNullable<ToolBlock['toolKind']> = 'file_change', toolName = 'generate_office_document') {
  const common = { threadId: 'thread', turnId: selector.turnId, callId: selector.callId, createdAt: timestamp, toolName, toolKind }
  const item: CoreTurnItemJson = variant === 'running'
    ? { ...common, id: 'item-call', kind: 'tool_call', role: 'assistant', status: 'running', arguments: PublicToolCallArgumentsProjectionV1.parse({
      schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }) }
    : { ...common, id: selector.resultItemId, kind: 'tool_result', role: 'tool', status: variant === 'error' ? 'failed' : 'completed',
      isError: variant === 'error', output: variant === 'error' ? { ...host, status: 'failed', code: 'tool_failed', messageKey: 'tool_failed' }
        : { ...host, projectionKind: 'artifact_status', code: 'artifact_created', messageKey: 'artifact_created', artifact: receipt } }
  expect((variant === 'running' ? ToolCallTurnItem : ToolResultTurnItem).safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item)
  if (block?.kind !== 'tool') throw new Error('fixture must reach the real Office mapper')
  return block
}
const inspectDisplay = (status: ToolResultLocalDisplayV1['capture']['status'] = 'completed'): ToolResultLocalDisplayV1 => ({
  schemaVersion: 1, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName: 'generate_office_document', canNavigate: false,
  inspect: { inputJSON: '{"path":"/PRIVATE/original.docx"}', schemaJSON: '{"name":"generate_office_document","description":"Original Office request schema","parameters":{"type":"object"}}',
    inputTruncated: false, schemaTruncated: false, callEventAt: timestamp, resultEventAt: timestamp, durationMs: 0 },
  capture: { kind: 'inspect', status, label: '', body: '', truncated: false, durationMs: 0, startLine: 0, endLine: 0, totalLines: 0 }
})
let root: Root, container: HTMLDivElement, display: ToolResultLocalDisplayV1
let invalidate: ((id: string | null) => void) | undefined
let api: ReturnType<typeof makeApi>
function makeApi() {
  return {
    openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async (_id: string) => undefined),
    copyToolResultLocalDisplay: vi.fn(async () => ({ ok: true })), saveToolResultLocalDisplay: vi.fn(async () => ({ ok: true })),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidate = listener; return () => { invalidate = undefined } }),
    onRuntimeStatus: vi.fn(() => () => undefined)
  }
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  display = inspectDisplay(); api = makeApi()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
const openProcessGroup = async () => {
  const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button[aria-expanded="false"]')
  if (!header) return
  expect(header.disabled).toBe(false); expect(header.closest('[hidden]')).toBeNull()
  await act(async () => header.click())
  expect(header.getAttribute('aria-expanded')).toBe('true')
}
const draw = async (block: ToolBlock, props: Partial<Parameters<typeof ToolCallTree>[0]> = {}, openGroup = true) => {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' },
    createElement(ToolCallTree, { blocks: [block], processing: false, policy: presentationPolicyFor('detailed'), ...props }))))
  if (openGroup && props.visible !== false && props.publicVisible !== false) await openProcessGroup()
}
const leaf = () => container.querySelector<HTMLElement>('[data-office-generation-row]')!
const row = () => leaf().querySelector<HTMLElement>('[data-disclosure-row]')!
const expand = async () => { await act(async () => row().click()) }
describe('actual Office file-change dispatch and protected Inspect', () => {
  it('dispatches the real successful mapper receipt and retains actual call/result identity', async () => {
    const block = hydrate()
    expect(block.toolKind).toBe('file_change'); expect(block.meta?.localResult).toEqual(selector)
    await draw(block, {}, false)
    const group = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button')!
    expect(group).not.toBeNull(); expect(group.getAttribute('aria-expanded')).toBe('false')
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await openProcessGroup()
    expect(group.getAttribute('aria-expanded')).toBe('true')
    expect(row().getAttribute('aria-expanded')).toBe('false')
    expect(leaf()).not.toBeNull(); expect(leaf().textContent).toContain('Generate Office file')
    expect(leaf().textContent).toContain('Generated'); expect(leaf().textContent).toContain('DOCX · 2 KB')
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await expand()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    expect(api.openToolResultLocalDisplay.mock.calls[0][0]).toMatchObject({ threadId: 'thread', ...selector })
    expect(container.textContent).not.toContain('/PRIVATE')
    const inspect = [...leaf().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Inspect')!
    expect(inspect).toBeDefined(); await act(async () => inspect.click())
    const panel = container.querySelector('[data-tool-result-inspector]')!
    expect(panel.getAttribute('data-call-id')).toBe(selector.callId)
    const payload = [...panel.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Payload')!
    await act(async () => payload.click())
    expect(panel.textContent).toContain('/PRIVATE/original.docx')
    await act(async () => invalidate?.(null))
    expect(container.textContent).not.toContain('/PRIVATE'); expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('forwards the existing Inspect callback using actual identities', async () => {
    const onInspect = vi.fn()
    await draw(hydrate(), { onInspect }); await expand()
    const inspect = [...leaf().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Inspect')!
    await act(async () => inspect.click())
    expect(onInspect).toHaveBeenCalledExactlyOnceWith(selector.turnId, selector.callId)
  })
  it('preserves real running and failure states without inventing a filename or running shimmer', async () => {
    await draw(hydrate('running'), { processing: true })
    expect(leaf().querySelector('[data-tool]')?.getAttribute('data-state')).toBe('running')
    expect(leaf().textContent).toContain('Running')
    expect(leaf().querySelector('[aria-expanded]')).toBeNull()
    expect(leaf().querySelector('[data-shimmer="true"]')).toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await draw(hydrate('error'))
    expect(leaf().querySelector('[data-tool]')?.getAttribute('data-state')).toBe('error')
    expect(leaf().textContent).toContain('Failed'); expect(leaf().textContent).not.toContain('DOCX')
    display = inspectDisplay('canceled')
    await expand()
    expect(leaf().querySelector('[data-tool]')?.getAttribute('data-state')).toBe('stopped')
  })
  it.each(['wrong-kind', 'wrong-name'] as const)('keeps %s outside the specialized Office branch', async variant => {
    await draw(hydrate('success', variant === 'wrong-kind' ? 'tool_call' : 'file_change',
      variant === 'wrong-name' ? 'write' : 'generate_office_document'))
    expect(container.querySelector('[data-office-generation-row]')).toBeNull()
  })
  it('closes a pending protected lease on hide and discards late private Inspect data', async () => {
    let finish!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const block = hydrate()
    await draw(block); await expand(); await draw(block, { visible: false })
    await act(async () => finish({ ok: true, display }))
    expect(container.textContent).not.toContain('/PRIVATE')
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
})
