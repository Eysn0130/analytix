// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { toolResultLocalDisplaySchemaV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { PublicToolResultProjectionV1, ToolResultTurnItem } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ChatBlock, ToolBlock } from '../../../../../agent/types'
import type { CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import { AnalytixRuntimeProvider } from '../../../../../agent/analytix-runtime'
import { rendererRuntimeClient } from '../../../../../agent/runtime-client'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { MessageTimeline } from '../../../MessageTimeline'
import { WebFetchRow } from './web-fetch-row'
import { presentationPolicyFor } from '../chat-view-model'

const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const requestUrl = 'https://request.invalid/actual?query=1', finalUrl = 'https://result.invalid/actual-final'
const base = { schemaVersion: 1, projectionKind: 'web_fetch_status', disclosure: 'metadata_only',
  privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
const publicResult = (statusCode = 201, truncated = false) => PublicToolResultProjectionV1.parse(statusCode >= 400
  ? { ...base, status: 'failed', messageKey: 'tool_failed', code: 'tool_failed', web: { outcome: 'http_error', statusCode, byteCount: 7, truncated } }
  : { ...base, status: 'completed', messageKey: 'tool_completed', code: 'tool_completed', web: { outcome: 'success', statusCode, byteCount: 7, truncated } })
function hydrate(statusCode = 201, toolName = 'web_fetch', toolKind: NonNullable<ToolBlock['toolKind']> = 'tool_call'): ToolBlock {
  const item: CoreTurnItemJson = { id: selector.resultItemId, turnId: selector.turnId, threadId: 'thread', role: 'tool',
    status: statusCode >= 400 ? 'failed' : 'completed', createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result',
    toolName, callId: selector.callId, toolKind, output: publicResult(statusCode), isError: statusCode >= 400 }
  const block = chatBlockFromItem(item)
  if (block?.kind !== 'tool') throw new Error('fixture must reach the actual mapper tool block')
  return block
}
const response = (statusCode = 201, body = 'actual extracted text'): ToolResultLocalDisplayV1 => ({
  schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'web_fetch',
  canOpenWeb: { request: true, result: true }, capture: { kind: 'web_fetch', status: 'completed', label: 'web_fetch',
    body, durationMs: 27, truncated: false, startLine: 0, endLine: 0, totalLines: 0, web: {
      requestUrl, finalUrl, inputJSON: JSON.stringify({ url: requestUrl }), schemaJSON: JSON.stringify({ name: 'web_fetch', description: 'Fetch an actual URL', parameters: { type: 'object', properties: { url: { type: 'string' } } } }),
      title: 'Actual response title', statusCode, fetchTruncated: false, responseComplete: true, toolIsError: statusCode >= 400,
      startedAt: '2026-10-08T00:00:00Z', inputTruncated: false, schemaTruncated: false, requestUrlTruncated: false, finalUrlTruncated: false,
      ...(statusCode >= 400 ? { error: { code: 'fetch_failed', message: 'HTTP ' + statusCode } } : {})
    } }
})
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>, invalidate: ((id: string | null) => void) | undefined
let display: ToolResultLocalDisplayV1, api: ReturnType<typeof makeApi>
function makeApi() {
  return { openToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async () => undefined), openToolResultLocalWebLink: vi.fn(async (_request: unknown) => ({ ok: true })),
    copyToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })), saveToolResultLocalDisplay: vi.fn(async () => ({ ok: true })),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidate = listener; return () => { invalidate = undefined } }),
    onRuntimeStatus: vi.fn(() => () => undefined) }
}
const draw = async (blocks: ToolBlock[], props: Partial<Parameters<typeof ToolCallTree>[0]> = {}) => {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' },
    createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('detailed'), ...props }))))
  if (props.visible !== false && props.publicVisible !== false) {
    for (const header of container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')) {
      await act(async () => header.click())
    }
  }
}
const expand = async () => {
  const row = container.querySelector<HTMLElement>('[data-web-fetch-row] [data-disclosure-row]')
  if (!row) throw new Error('fixture must reach the actual Fetch row')
  await act(async () => row.click())
}
const click = async (label: string) => {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent?.trim() === label || node.getAttribute('aria-label') === label)
  expect(button, label).toBeDefined()
  await act(async () => { button!.focus(); button!.click() })
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en')
  previous = useChatStore.getState(); useChatStore.setState({ workspaceRoot: '' })
  display = response(); api = makeApi(); Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals()
})


function installMultipleCalls(separateTurns = false) {
  const fixtures = ['a', 'd'].map((token, index) => {
    const identity = { turnId: separateTurns ? 'turn-' + index : 'turn', callId: 'call_host_' + token.repeat(64), resultItemId: 'item_result_' + token.repeat(64) }
    const block = chatBlockFromItem({ kind: 'tool_result', id: identity.resultItemId, threadId: 'thread',
      turnId: identity.turnId, callId: identity.callId, toolName: 'web_fetch', toolKind: 'tool_call', role: 'tool',
      status: 'failed', isError: true, createdAt: '2026-10-08T00:00:00Z', output: publicResult(503) })
    if (block?.kind !== 'tool') throw new Error('fixture must reach the actual mapper tool block')
    const value = { ...response(503, 'HTTP 503\nPRIVATE_CALL_BODY_' + index), resultItemId: identity.resultItemId, snapshotDigest: token.repeat(64) }
    return { identity, block, value }
  })
  api.openToolResultLocalDisplay.mockImplementation(async (request: unknown) => {
    const identity = request as { resultItemId: string }
    const fixture = fixtures.find(item => item.identity.resultItemId === identity.resultItemId)
    if (!fixture) throw new Error('Unknown synthetic call')
    return { ok: true, display: fixture.value }
  })
  const listeners = new Set<(id: string | null) => void>()
  api.onToolResultLocalDisplayInvalidated.mockImplementation(listener => {
    listeners.add(listener)
    invalidate = id => { for (const current of listeners) current(id) }
    return () => { listeners.delete(listener) }
  })
  return fixtures
}
async function inspectCall(block: ToolBlock) {
  const selector = `[data-web-fetch-row][data-analytix-process-block-id="${block.id}"]`
  let row = container.querySelector<HTMLElement>(selector)
  expect(row, 'actual Web call row').not.toBeNull()
  const turn = row!.closest('[data-analytix-chat-view]')?.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')
  if (turn?.getAttribute('aria-expanded') === 'false') {
    expect(turn.disabled).toBe(false); expect(turn.closest('[hidden]')).toBeNull()
    await act(async () => turn.click())
    expect(turn.getAttribute('aria-expanded')).toBe('true')
  }
  row = container.querySelector<HTMLElement>(selector)!
  const group = row.closest('[data-process-group]')?.querySelector<HTMLButtonElement>(':scope > div:first-child > button')
  if (group?.getAttribute('aria-expanded') === 'false') {
    expect(group.disabled).toBe(false); expect(group.closest('[hidden]')).toBeNull()
    await act(async () => group.click())
    expect(group.getAttribute('aria-expanded')).toBe('true')
  }
  row = container.querySelector<HTMLElement>(selector)!
  expect(row.closest('[hidden]')).toBeNull()
  const disclosure = row.querySelector<HTMLElement>('[data-disclosure-row]')!
  if (disclosure.getAttribute('aria-expanded') !== 'true') await act(async () => disclosure.click())
  expect(disclosure.getAttribute('aria-expanded')).toBe('true')
  const inspect = [...row.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent?.trim() === 'Inspect')
  expect(inspect, 'actual call Inspect action').toBeDefined()
  await act(async () => { inspect!.focus(); inspect!.click() })
}
async function drawMultiTurnTimeline(blocks: ChatBlock[], threadId = 'thread') {
  await act(async () => {
    useChatStore.setState({ activeThreadId: threadId, workspaceRoot: '', busy: false, currentTurnId: null, currentTurnUserId: null })
    root.render(createElement(MessageTimeline, { blocks, live: '', activeThreadId: threadId,
      runtimeConnection: 'ready', onRetryConnection() {}, onOpenSettings() {} }))
  })
}

describe('actual mapper to protected DSH Fetch consumer', () => {
  it('shows common original Input once in incomplete failed Fetch and keeps it read-only beside the existing Result export', async () => {
    display = response(503, 'HTTP 503\nACTUAL_RESULT_BODY')
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    display.capture.status = 'failed'
    display.capture.web.responseComplete = false
    delete display.capture.web.fetchTruncated
    delete display.capture.web.inputJSON; delete display.capture.web.schemaJSON
    display.inspect = { inputJSON: '{"url":"https://original.invalid/PRIVATE_MODEL_INPUT"}', schemaJSON: '{"name":"web_fetch","description":"PRIVATE_MODEL_SCHEMA","parameters":{"type":"object"}}',
      inputTruncated: false, schemaTruncated: false, callEventAt: '2026-10-10T00:00:00Z', resultEventAt: '2026-10-10T00:00:00.125Z', durationMs: 125 }
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    await draw([hydrate(503)]); await expand()
    const input = container.querySelector<HTMLElement>('[data-tool-io] [data-protected-tool-readonly]')!
    expect(input.textContent).toContain('PRIVATE_MODEL_INPUT')
    expect([...container.querySelectorAll('pre')].filter(node => node.textContent?.includes('PRIVATE_MODEL_INPUT'))).toHaveLength(1)
    const selection = document.getSelection()!, range = document.createRange(); range.selectNodeContents(input); selection.removeAllRanges(); selection.addRange(range)
    const copyInput = new Event('copy', { bubbles: true, cancelable: true }); await act(async () => input.dispatchEvent(copyInput))
    expect(copyInput.defaultPrevented).toBe(true); expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
    selection.removeAllRanges()
    const result = container.querySelector<HTMLElement>('[data-tool-io] [data-protected-tool-copy-section="result"]')!
    range.selectNodeContents(result); selection.addRange(range)
    const copyResult = new Event('copy', { bubbles: true, cancelable: true }); await act(async () => result.dispatchEvent(copyResult))
    expect(copyResult.defaultPrevented).toBe(true); expect(api.copyToolResultLocalDisplay).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ section: 'result' }))
    selection.removeAllRanges(); await click('Inspect'); await click('Payload')
    expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('PRIVATE_MODEL_INPUT')
    expect(container.querySelector('[data-tool-result-inspector]')?.textContent).not.toContain('Copy')
    await click('Schema'); expect(container.textContent).toContain('Model tool schema'); expect(container.textContent).toContain('PRIVATE_MODEL_SCHEMA')
    await click('Timing'); expect(container.textContent).toContain('125 ms'); expect(container.textContent).not.toContain('27 ms')
    await act(async () => invalidate?.(null)); expect(container.textContent).not.toContain('PRIVATE_MODEL_')
  })
  it('shows omitted common Input as capped without reviving a legacy input', async () => {
    display = response(503, 'HTTP 503')
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    delete display.capture.web.inputJSON; delete display.capture.web.schemaJSON
    display.inspect = { inputTruncated: true, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null }
    await draw([hydrate(503)]); await expand()
    expect(container.querySelector('[data-tool-io] [data-protected-tool-readonly]')).toBeNull()
    expect(container.textContent).toContain(i18n.t('dshWebFetch.fieldLimit', { ns: 'common' }))
    await click('Inspect'); await click('Payload'); expect(container.querySelector('[role="tabpanel"]')?.textContent).toContain('Local capture is incomplete')
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  })

  describe('P2 current Inspect selection', () => {
    it('keeps one current Inspector for multiple real calls through A to B to A and close', async () => {
      const [a, b] = installMultipleCalls()
      await draw([a.block, b.block], { policy: presentationPolicyFor('verbose') })
      for (const current of [a, b, a]) {
        await inspectCall(current.block)
        expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
        expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-call-id')).toBe(current.identity.callId)
        await click('Result')
        expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain(current.value.capture.body)
        const other = current === a ? b : a
        expect(container.querySelector('[data-tool-result-inspector]')?.textContent).not.toContain(other.value.capture.body)
      }
      await click(i18n.t('common:dshToolInspect.copy'))
      expect(api.copyToolResultLocalDisplay).toHaveBeenLastCalledWith({ ...a.identity, threadId: 'thread',
        viewId: expect.any(String), snapshotDigest: a.value.snapshotDigest, section: 'result' })
      await click(i18n.t('common:dshToolInspect.close'))
      expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
      expect(container.textContent).not.toContain('PRIVATE_CALL_BODY_')
      await draw([a.block, b.block], { policy: presentationPolicyFor('verbose') })
      expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    })
    it('shares the selected Inspector across real Timeline turns and clears it on thread switch or revoke', async () => {
      vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
      const frames = new Map<number, FrameRequestCallback>(); let serial = 0
      vi.stubGlobal('requestAnimationFrame', (run: FrameRequestCallback) => { frames.set(++serial, run); return serial })
      vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
      vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
      const descriptors = Object.fromEntries(['scrollTo', 'scrollIntoView'].map(key => [key, Object.getOwnPropertyDescriptor(HTMLElement.prototype, key)]))
      Object.defineProperty(HTMLElement.prototype, 'scrollTo', { configurable: true, value: vi.fn() })
      Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
      const flush = async () => { for (let count = 0; frames.size && count < 8; count++) await act(async () => {
        const work = [...frames.values()]; frames.clear(); for (const run of work) run(performance.now())
      }) }
      try {
        const [a, b] = installMultipleCalls(true)
        const blocks: ChatBlock[] = [a, b].flatMap((fixture, index) => [
          { kind: 'user', id: 'user-' + index, text: 'Synthetic request ' + index, meta: { turnId: fixture.identity.turnId } }, fixture.block
        ])
        await drawMultiTurnTimeline(blocks)
        expect(container.querySelectorAll('[data-analytix-chat-view]')).toHaveLength(2)
        await inspectCall(a.block); await flush()
        expect(container.querySelector('[data-analytix-chat-view]')).toBeNull()
        for (const current of [a, b, a]) {
          const row = container.querySelector<HTMLButtonElement>(`[data-conversation-call-key][data-result-item-id="${current.identity.resultItemId}"]`)!
          expect(row, 'actual admitted Records row').not.toBeNull()
          await act(async () => row.click()); await flush()
          expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
          expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-call-id')).toBe(current.identity.callId)
        }
        await click(i18n.t('common:dshToolInspect.close'))
        expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
        await click(i18n.t('common:dshRecords.conversation'))
        await inspectCall(b.block); await flush()
        expect(container.querySelector('[data-tool-result-inspector]')?.getAttribute('data-call-id')).toBe(b.identity.callId)
        await act(async () => invalidate?.(null))
        expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
        expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
        await drawMultiTurnTimeline([], 'other-thread')
        expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
        await drawMultiTurnTimeline(blocks, 'thread')
        expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
        expect(container.textContent).not.toContain('PRIVATE_CALL_BODY_')
      } finally {
        for (const [key, descriptor] of Object.entries(descriptors)) {
          if (descriptor) Object.defineProperty(HTMLElement.prototype, key, descriptor); else Reflect.deleteProperty(HTMLElement.prototype, key)
        }
      }
    })

  })
  it('prefetches the authorized request summary without putting response text into the folded DOM or public block', async () => {
    const block = hydrate(), before = JSON.stringify(block)
    expect(block.meta?.localResult).toEqual(selector)
    await draw([block])
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String) })
    expect(container.textContent).toContain(requestUrl)
    expect(container.textContent).not.toContain('actual extracted text')
    expect(container.querySelector('[data-web-fetch-card],[data-tool-result-inspector]')).toBeNull()
    expect(container.querySelector('[href]')).toBeNull()
    expect(JSON.stringify(block)).toBe(before)
    expect(container.querySelector('[data-web-fetch-row] svg')?.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(container.querySelector('[data-web-fetch-row] svg')?.getAttribute('width')).toBe('14')
  })
  it.each([201, 204, 404, 503])('shows actual HTTP %s, final URL and complete response text through Inspect', async statusCode => {
    display = response(statusCode, (statusCode >= 400 ? 'HTTP ' + statusCode + '\n' : '') + 'FULL_RESPONSE_' + statusCode)
    await draw([hydrate(statusCode)]); await expand()
    expect(container.querySelector('[data-web-fetch-card]')?.textContent).toContain(finalUrl)
    expect(container.querySelector('[data-web-fetch-meta]')?.textContent).toBe('HTTP ' + statusCode)
    expect(container.querySelector('[data-web-fetch-card]')?.textContent).not.toContain('bytes')
    // This fixture explicitly supplies toolIsError for its failed HTTP captures.
    expect(container.querySelector('[data-web-fetch-row] [data-state]')?.getAttribute('data-state')).toBe(statusCode >= 400 ? 'error' : 'ok')
    if (statusCode >= 400) expect(container.querySelector('[data-protected-tool-summary]')?.textContent).toBe('HTTP ' + statusCode)
    expect(container.textContent).not.toContain('FULL_RESPONSE_')
    await click('Inspect'); await click('Result')
    expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain('FULL_RESPONSE_' + statusCode)
  })
  it('opens request and final links only by Host identity/role without raw href or toggling the disclosure', async () => {
    await draw([hydrate()])
    const request = container.querySelector<HTMLButtonElement>('[data-tool-web-link]')!
    await act(async () => request.click())
    expect(api.openToolResultLocalWebLink).toHaveBeenLastCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String), snapshotDigest: display.snapshotDigest, targetRole: 'request' })
    expect(container.querySelector('[data-web-fetch-card]')).toBeNull()
    await expand(); await act(async () => container.querySelector<HTMLButtonElement>('[data-web-fetch-card] button')!.click())
    expect(api.openToolResultLocalWebLink).toHaveBeenLastCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String), snapshotDigest: display.snapshotDigest, targetRole: 'result' })
    expect(container.querySelector('[href]')).toBeNull()
    expect(JSON.stringify(api.openToolResultLocalWebLink.mock.calls)).not.toContain('https://')
    await expand()
    expect(container.textContent).toContain(requestUrl)
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
  })
  it('shows actual failed Input/Output and error first line without inventing a successful response card', async () => {
    const value = response()
    if (value.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    display = { ...value, canOpenWeb: { request: true, result: false }, capture: { ...value.capture, status: 'failed',
      body: 'Actual network failure\nActual detail', web: { ...value.capture.web, finalUrl: undefined, statusCode: undefined,
        responseComplete: false, toolIsError: true, fetchTruncated: undefined, error: { code: 'fetch_failed', message: 'Actual network failure' } } } }
    await draw([hydrate()])
    expect(container.textContent).toContain('Actual network failure')
    expect(container.textContent).not.toContain('Actual detail')
    expect(container.querySelector('[data-tool-web-link]')).toBeNull()
    await expand()
    expect(container.querySelector('[data-web-fetch-card]')).toBeNull()
    expect(container.querySelector('[data-tool-io]')?.textContent).toContain(JSON.stringify({ url: requestUrl }))
    expect(container.querySelector('[data-tool-io]')?.textContent).toContain('Actual detail')
  })
  it('retains real producer truncation and the separate capture limit without calling a cap complete', async () => {
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    display.capture.web.fetchTruncated = true; display.capture.truncated = true
    await draw([hydrate()]); await expand()
    expect(container.querySelector('[data-web-fetch-meta]')?.textContent).toBe('HTTP 201Content truncated')
    expect(container.textContent).toContain('Local capture is incomplete')
  })
  it('copies a folded URL from its sealed Host section and retires both URL and detail on invalidation', async () => {
    await draw([hydrate()])
    const summary = container.querySelector('[data-protected-tool-summary]')!
    const range = document.createRange(); range.selectNodeContents(summary)
    const selection = document.getSelection()!; selection.removeAllRanges(); selection.addRange(range)
    expect(selection.isCollapsed).toBe(false); expect(selection.getRangeAt(0).intersectsNode(summary)).toBe(true)
    expect(selection.toString()).toBe(requestUrl)
    const event = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => summary.dispatchEvent(event))
    expect(event.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String), snapshotDigest: display.snapshotDigest, section: 'request-url' })
    await expand(); await click('Inspect')
    await act(async () => invalidate?.(null))
    expect(container.textContent).not.toContain(requestUrl)
    expect(container.querySelector('[data-tool-result-inspector],[data-web-fetch-card]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('keeps complete large text in the private Result leaf beyond the old 32 KiB cap', async () => {
    const text = 'ACTUAL_LONG_BODY\n' + 'z'.repeat(80_000)
    display = response(201, text)
    await draw([hydrate()]); expect(container.textContent).not.toContain(text)
    await expand(); await click('Inspect'); await click('Result')
    expect(container.querySelector('[role="tabpanel"] pre')?.textContent).toBe(text)
  })
  it('refuses wrong result identity and malformed private JSON without exposing body or guessed status', async () => {
    display = { ...response(), resultItemId: 'item_result_' + 'd'.repeat(64) }
    await draw([hydrate()]); await expand()
    expect(container.textContent).not.toContain(requestUrl)
    expect(container.querySelector('[data-web-fetch-card],[data-tool-result-inspector]')).toBeNull()
    await draw([], { resetKey: 'reset' })
    display = { ...response(), untrusted: 'PRIVATE_EXTRA' } as never
    await draw([hydrate()]); await expand()
    expect(container.innerHTML).not.toContain('PRIVATE_EXTRA')
    expect(container.querySelector('[data-web-fetch-card]')).toBeNull()
  })
  it('retires a selected Inspector and re-reads the same call when resetKey or privateGeneration changes', async () => {
    await draw([hydrate()], { resetKey: 'a', privateGeneration: 1 }); await expand(); await click('Inspect'); await click('Result')
    expect(container.textContent).toContain('actual extracted text')
    await draw([hydrate()], { resetKey: 'b', privateGeneration: 1 })
    expect(container.querySelector('[data-tool-result-inspector],[data-web-fetch-card]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled(); expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
    await expand(); await click('Inspect')
    await draw([hydrate()], { resetKey: 'b', privateGeneration: 2 })
    expect(container.querySelector('[data-tool-result-inspector],[data-web-fetch-card]')).toBeNull()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(3)
  })
  it('uses only actual active process input and clears it when its private generation retires', async () => {
    const block = { ...hydrate(), status: 'running' as const }, openWebLink = vi.fn()
    const capture = { blockId: 'private-active', requestUrl, openWebLink, preview: null, body: null, expanded: false, expandable: false, setOpen: vi.fn() }
    await act(async () => root.render(createElement(WebFetchRow, { block, processing: true, visible: true, resetKey: 'active', privateGeneration: 1,
      policy: presentationPolicyFor('detailed'), capture })))
    expect(container.textContent).toContain(requestUrl)
    expect(container.querySelector('[data-protected-process-block="private-active"]')?.textContent).toBe(requestUrl)
    await act(async () => container.querySelector<HTMLButtonElement>('[data-tool-web-link]')!.click())
    expect(openWebLink).toHaveBeenCalledOnce(); expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(container.querySelector('[data-web-fetch-card],[data-tool-result-inspector],[href]')).toBeNull()
    await act(async () => root.render(createElement(WebFetchRow, { block, processing: true, visible: true, resetKey: 'active', privateGeneration: 2, policy: presentationPolicyFor('detailed') })))
    expect(container.textContent).not.toContain(requestUrl)
  })
  it('refuses a Shell snapshot under the Web consumer and ignores public URL/body bait without a local selector', async () => {
    display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: {
      kind: 'shell', status: 'completed', body: 'PRIVATE_SHELL_BODY', label: 'PRIVATE_SHELL_COMMAND', durationMs: 3, truncated: false,
      startLine: 0, endLine: 0, totalLines: 0
    } }
    await draw([hydrate()]); await expand()
    expect(container.innerHTML).not.toContain('PRIVATE_SHELL_')
    expect(container.querySelector('[data-terminal],[data-read],[data-web-fetch-card]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
    expect(api.openToolResultLocalWebLink).not.toHaveBeenCalled(); expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    const bait: ToolBlock = { ...hydrate(), meta: { toolName: 'web_fetch', url: 'https://bait.invalid', text: 'PRIVATE_BAIT' }, detail: 'PRIVATE_BAIT', summary: 'PRIVATE_BAIT' }
    await draw([bait])
    expect(container.innerHTML).not.toContain('PRIVATE_BAIT'); expect(container.innerHTML).not.toContain('bait.invalid')
    expect(container.querySelector('[data-web-fetch-card],[href]')).toBeNull()
  })
  it.each([{ toolName: 'WEB_FETCH', toolKind: 'tool_call' }, { toolName: 'web_fetch', toolKind: 'command_execution' }])('does not authorize a direct consumer with wrong raw binding %j', async ({ toolName, toolKind }) => {
    const block = { ...hydrate(), toolKind: toolKind as ToolBlock['toolKind'], meta: { ...hydrate().meta, toolName } }
    await act(async () => root.render(createElement(WebFetchRow, { block, processing: false, visible: true, resetKey: 'wrong', privateGeneration: 0, policy: presentationPolicyFor('detailed') })))
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled(); expect(container.querySelector('[data-web-fetch-row]')).toBeNull()
  })
  it.each(['web_search', 'fetch_url', 'browse', 'WEB_FETCH'])('preserves reachable %s owner without dispatching the dedicated Fetch consumer', async toolName => {
    await draw([hydrate(201, toolName)])
    expect(container.querySelector('[data-web-fetch-row]')).toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  })
})


describe('historical Core DTO isolation through ProtectedToolResult', () => {
  const coreItem = (statusCode = 201): CoreTurnItemJson => ({ id: selector.resultItemId, turnId: selector.turnId, threadId: 'thread', role: 'tool',
    status: statusCode >= 400 ? 'failed' : 'completed', createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result',
    toolName: 'web_fetch', callId: selector.callId, toolKind: 'tool_call', output: publicResult(statusCode), isError: statusCode >= 400 })
  const turnContext = { workspaceCheckpointId: 'checkpoint_turn', attachmentIds: ['att_turn'], activeSkillIds: ['skill_turn'],
    injectedMemoryIds: ['memory_turn'], skillInjectionBytes: 14 }
  async function history(item: CoreTurnItemJson, context: Partial<typeof turnContext> = {}) {
    vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockResolvedValue({ ok: true, status: 200, body: JSON.stringify({
      id: 'thread', title: 'Synthetic history', workspace: '/synthetic', mode: 'agent', status: 'idle', latestSeq: 1,
      createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:00:00Z', turns: [{
        id: 'turn', threadId: 'thread', status: 'completed', prompt: 'Synthetic request', createdAt: '2026-10-08T00:00:00Z', ...context,
        items: [{ id: 'user_history', turnId: 'turn', threadId: 'thread', role: 'user', status: 'completed', kind: 'user_message',
          createdAt: '2026-10-08T00:00:00Z', text: 'Synthetic request', workspaceCheckpointId: 'checkpoint_user' }, item,
          { id: 'approval_history', turnId: 'turn', threadId: 'thread', role: 'assistant', status: 'pending', kind: 'approval',
            createdAt: '2026-10-08T00:00:00Z', approvalId: 'approval_history', summary: 'Synthetic approval' }]
      }]
    }) })
    return new AnalytixRuntimeProvider().getThreadDetail('thread')
  }
  it.each([[201, false], [201, true], [503, false], [503, true]] as const)(
    'loads original HTTP %s Core history with populated turn context=%s into the real protected consumer', async (statusCode, populated) => {
      const item = coreItem(statusCode), original = JSON.stringify(item)
      expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
      display = response(statusCode, (statusCode >= 400 ? 'HTTP ' + statusCode + '\n' : '') + 'PRIVATE_HISTORICAL_FETCH_BODY')
      const detail = await history(item, populated ? turnContext : {})
      const block = detail.blocks.find((value): value is ToolBlock => value.kind === 'tool')
      expect(block?.meta?.localResult).toEqual(selector)
      expect(block?.meta?.publicResult).toEqual(publicResult(statusCode))
      expect(detail.blocks.find(value => value.kind === 'user')?.meta?.workspaceCheckpointId).toBe('checkpoint_user')
      if (populated) {
        expect(block?.meta).toMatchObject(turnContext)
        expect(detail.blocks.find(value => value.kind === 'approval')?.meta).toMatchObject(turnContext)
      }
      expect(JSON.stringify(detail.blocks)).not.toContain('PRIVATE_HISTORICAL_FETCH_BODY')
      expect(JSON.stringify(item)).toBe(original)
      expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
      expect(block).toBeDefined()
      await draw([block!]); await expand()
      expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String) })
      if (statusCode >= 400) expect(container.querySelector('[data-protected-tool-summary]')?.textContent).toBe('HTTP ' + statusCode)
      expect(container.textContent).not.toContain('PRIVATE_HISTORICAL_FETCH_BODY')
      await click('Inspect'); await click('Result')
      expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain('PRIVATE_HISTORICAL_FETCH_BODY')
    })
  it.each([{ untrustedExtra: 'PRIVATE_UNKNOWN_HISTORY' }, { activeSkillIds: ['PRIVATE_UNKNOWN_HISTORY'] }])(
    'keeps unknown original history fields rejected before the protected consumer %#', async extra => {
      const item = { ...coreItem(), ...extra }
      expect(ToolResultTurnItem.safeParse(item).success).toBe(false)
      const detail = await history(item)
      const block = detail.blocks.find((value): value is ToolBlock => value.kind === 'tool')
      expect(block?.meta?.localResult).toBeUndefined()
      expect(JSON.stringify(detail.blocks)).not.toContain('PRIVATE_UNKNOWN_HISTORY')
      await draw(block ? [block] : [])
      expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
      expect(container.querySelector('[data-protected-tool-result]')).toBeNull()
      expect(container.textContent).not.toContain(display.capture.body)
    })
  it('rejects raw body injection in the original public projection after real history loading', async () => {
    const item = { ...coreItem(), output: { ...publicResult(), rawBody: 'PRIVATE_UNKNOWN_HISTORY' } }
    expect(ToolResultTurnItem.safeParse(item).success).toBe(false)
    const detail = await history(item), block = detail.blocks.find((value): value is ToolBlock => value.kind === 'tool')
    expect(block?.meta?.localResult).toBeUndefined()
    expect(JSON.stringify(detail.blocks)).not.toContain('PRIVATE_UNKNOWN_HISTORY')
    await draw(block ? [block] : [])
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  })
})
