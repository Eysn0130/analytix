// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { ProtectedToolResult } from './ProtectedToolResult'
import { parseAnsiLines } from './ansi'
import { getSharedActiveCaseContext, setSharedActiveCaseContext } from '../../../data-analysis/services/analysis/stats-case-overview-resource'
import type { ToolBlock, ThreadEventSink } from '../../../agent/types'
import { dispatchAnalytixRuntimeEvent } from '../../../agent/analytix-mapper'
let root: Root, container: HTMLDivElement
const selector = { turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const block: ToolBlock = { kind: 'tool', id: 'tool-a', summary: 'bash', status: 'error', meta: { localResult: selector } }
const display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: {
  kind: 'shell', status: 'timeout', body: '\x1b[31mfirst\x1b[0m\n' + Array.from({length: 20}, (_, i) => 'line ' + i).join('\n') + '\nexitCode=0\nstatus=completed\r\n',
  label: 'printf synthetic', exitCode: 7, durationMs: 5, truncated: false, startLine: 0, endLine: 0, totalLines: 0
} }
let invalidate: ((id: string | null) => void) | undefined
let api: ReturnType<typeof makeApi>
function makeApi() {
  return { openToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async () => undefined),
    copyToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })),
    saveToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })),
    invalidateToolResultLocalDisplay: vi.fn(async (): Promise<void> => undefined),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidate = listener; return () => { invalidate = undefined } }),
    onRuntimeStatus: vi.fn(() => () => undefined) }
}
beforeEach(async () => {
  await i18n.changeLanguage('zh')
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  api = makeApi()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-a' })))
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); document.getSelection()?.removeAllRanges(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
async function click(label: string) {
  const button = [...container.querySelectorAll('button')].find(item => item.textContent?.trim() === label || item.getAttribute('aria-label') === label)
  expect(button, label).toBeDefined()
  await act(async () => button!.click())
}
describe('protected lazy result display', () => {
  it('opens the identity received through the actual live mapper port', async () => {
    const onTool = vi.fn()
    await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_finished', threadId: 'thread-a', turnId: 'turn-a', item: {
      id: selector.resultItemId, turnId: 'turn-a', threadId: 'thread-a', role: 'tool', createdAt: '2026-10-06T00:00:00Z', kind: 'tool_result',
      callId: selector.callId, toolName: 'bash', toolKind: 'command_execution', status: 'failed', isError: true,
      output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'failed',
        messageKey: 'tool_failed', code: 'tool_failed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
    } }, { onTool } as unknown as ThreadEventSink, async () => undefined)
    expect(onTool).toHaveBeenCalledTimes(1)
    const event = onTool.mock.calls[0][0]
    await act(async () => root.render(createElement(ProtectedToolResult, { threadId: 'thread-a', block: {
      kind: 'tool', id: event.itemId, summary: event.summary, status: event.status, meta: event.meta
    } })))
    await click('查看本地结果')
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({
      threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64),
      resultItemId: 'item_result_' + 'b'.repeat(64), viewId: expect.any(String)
    })
  })
  it.each([
    { localResult: { ...selector, threadId: 'other-thread' } },
    { localResult: selector, turnId: 'other-turn' },
    { localResult: selector, callId: 'call_host_' + 'd'.repeat(64) }
  ])('refuses a selector outside its closed block identity %#', async (meta) => {
    await act(async () => root.render(createElement(ProtectedToolResult, { threadId: 'thread-a', block: { ...block, meta } })))
    expect(container.querySelector('[data-protected-tool-result]')).toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  })

  it('expands actual bounded terminal content, trusts Host status, and sends identity-only copy/save actions', async () => {
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    await click('查看本地结果')
    expect(container.textContent).toContain('已超时')
    expect(container.textContent).toContain('退出码 7')
    expect(container.querySelector('[data-terminal]')).not.toBeNull()
    await click('展开 7 行')
    expect(container.textContent).toContain('line 10')
    expect(container.textContent).toContain('status=completed')
    await click('复制脱敏文本')
    const payload = api.copyToolResultLocalDisplay.mock.calls[0]?.[0] as unknown as Record<string, unknown>
    expect(payload).toEqual({ ...selector, threadId: 'thread-a', viewId: expect.any(String), snapshotDigest: 'c'.repeat(64) })
    expect(JSON.stringify(payload)).not.toContain('printf synthetic')
    await click('保存脱敏片段')
    expect(api.saveToolResultLocalDisplay).toHaveBeenCalledOnce()
    await click('收起详情')
    expect(container.querySelector('[data-terminal]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('clears old body on Host invalidation and can retry an unavailable result', async () => {
    await click('查看本地结果')
    await act(async () => invalidate?.(null))
    expect(container.querySelector('[data-terminal]')).toBeNull()
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: false, code: 'unavailable' } as never)
    await click('查看本地结果')
    expect(container.textContent).toContain('当前不可用')
    await click('重试')
    expect(container.querySelector('[data-terminal]')).not.toBeNull()
  })
  it('attaches transfer listeners only for a current open body and retires them on collapse, revoke, and hide', async () => {
    const added = vi.spyOn(document, 'addEventListener')
    const removed = vi.spyOn(document, 'removeEventListener')
    const transfers = ['copy', 'cut', 'dragstart']
    expect(added.mock.calls.filter(([type]) => transfers.includes(type))).toHaveLength(0)
    const expectRetired = async (retire: () => Promise<void>) => {
      await click('查看本地结果')
      const listeners = transfers.map(type => added.mock.calls.filter(([eventType]) => eventType === type).at(-1))
      for (const listener of listeners) expect(listener).toBeDefined()
      await retire()
      expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
      for (const listener of listeners) expect(removed.mock.calls).toContainEqual(listener)
      const copy = new Event('copy', { bubbles: true, cancelable: true })
      document.dispatchEvent(copy)
      expect(copy.defaultPrevented).toBe(false)
    }
    await expectRetired(() => click('收起详情'))
    await expectRetired(() => act(async () => invalidate?.(null)))
    await expectRetired(async () => {
      vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
      await act(async () => document.dispatchEvent(new Event('visibilitychange')))
    })
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('cancels a collapsed in-flight open and ignores its late body', async () => {
    let complete!: (result: unknown) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { complete = resolve as (result: unknown) => void }))
    await click('查看本地结果')
    await click('收起详情')
    await act(async () => complete({ ok: true, display }))
    expect(container.querySelector('[data-terminal]')).toBeNull()
  })
  it('renders read code with original line numbers and plain fallback without path/URL actions', async () => {
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display, toolName: 'read', capture: {
      kind: 'read', status: 'completed', body: 'package main\r\n// source\r\n', label: '/synthetic/case-a/code.go', durationMs: 0, truncated: false, startLine: 9, endLine: 10, totalLines: 50
    } } } as never)
    await click('查看本地结果')
    expect(container.querySelector('[data-read]')).not.toBeNull()
    expect(container.textContent).toContain('package main')
    expect(container.textContent).toContain('50')
    expect(container.querySelector('a')).toBeNull()
  })
  it('routes a protected selection after focus moves, while ordinary chat/composer copy remains default', async () => {
    await click('查看本地结果')
    const body = container.querySelector('[data-protected-tool-body]')!
    const range = document.createRange(); range.selectNodeContents(body)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => document.dispatchEvent(copy))
    expect(copy.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).toHaveBeenCalledOnce()
    const composer = document.createElement('textarea'); composer.value = 'ordinary composer'; document.body.append(composer); composer.focus(); composer.select()
    const ordinary = new Event('copy', { bubbles: true, cancelable: true }); composer.dispatchEvent(ordinary)
    expect(ordinary.defaultPrevented).toBe(false)
    const ordinaryDrag = new Event('dragstart', { bubbles: true, cancelable: true }); composer.dispatchEvent(ordinaryDrag)
    expect(ordinaryDrag.defaultPrevented).toBe(false)
    for (const value of ['', 'plaintext-only']) {
      const editor = document.createElement('div'); editor.setAttribute('contenteditable', value); editor.textContent = 'ordinary editor'; document.body.append(editor)
      const editorCopy = new Event('copy', { bubbles: true, cancelable: true }); editor.dispatchEvent(editorCopy)
      expect(editorCopy.defaultPrevented).toBe(false)
      editor.remove()
    }
    const drag = new Event('dragstart', { bubbles: true, cancelable: true }); body.dispatchEvent(drag)
    expect(drag.defaultPrevented).toBe(true)
    composer.remove()
    const chat = document.createElement('p'); chat.textContent = 'ordinary chat'; document.body.append(chat)
    const chatRange = document.createRange(); chatRange.selectNodeContents(chat)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(chatRange)
    const chatCopy = new Event('copy', { bubbles: true, cancelable: true }); chat.dispatchEvent(chatCopy)
    expect(chatCopy.defaultPrevented).toBe(false)
    expect(api.copyToolResultLocalDisplay).toHaveBeenCalledOnce()
    chat.remove()
  })
  it('blocks a cross-leaf selection without choosing either result as the copy source', async () => {
    await act(async () => root.render(createElement('div', {},
      createElement(ProtectedToolResult, { block, threadId: 'thread-a' }),
      createElement(ProtectedToolResult, { block, threadId: 'thread-b' }))))
    const leaves = [...container.querySelectorAll('[data-protected-tool-result]')]
    for (const leaf of leaves) await act(async () => (leaf.querySelector('button') as HTMLButtonElement).click())
    const range = document.createRange(); range.setStartBefore(leaves[0]!.querySelector('[data-protected-tool-body]')!); range.setEndAfter(leaves[1]!.querySelector('[data-protected-tool-body]')!)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy)
    expect(copy.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('retires payload when eligibility changes and waits for case ACK with newest intent winning', async () => {
    await click('查看本地结果')
    await act(async () => root.render(createElement(ProtectedToolResult, { block: { ...block, status: 'running' }, threadId: 'thread-a' })))
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-a' })))
    await act(async () => { await setSharedActiveCaseContext({ caseId: 'case-a', workspaceRoot: '/synthetic' }) })
    await click('查看本地结果')
    let acknowledge!: () => void
    api.invalidateToolResultLocalDisplay.mockImplementationOnce(() => new Promise<void>(resolve => { acknowledge = resolve }))
    let pendingB!: Promise<unknown>, pendingA!: Promise<unknown>
    await act(async () => {
      pendingB = setSharedActiveCaseContext({ caseId: 'case-b', workspaceRoot: '/synthetic' })
      pendingA = setSharedActiveCaseContext({ caseId: 'case-a', workspaceRoot: '/synthetic' })
    })
    expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
    expect(getSharedActiveCaseContext()?.caseId).toBe('case-a')
    await act(async () => { acknowledge(); await pendingB; await pendingA })
    expect(getSharedActiveCaseContext()?.caseId).toBe('case-a')
    expect((container.querySelector('button') as HTMLButtonElement).disabled).toBe(false)
    api.invalidateToolResultLocalDisplay.mockRejectedValueOnce(new Error('unavailable'))
    await act(async () => { await setSharedActiveCaseContext({ caseId: 'case-b', workspaceRoot: '/synthetic' }) })
    expect(getSharedActiveCaseContext()?.caseId).toBe('case-a')
    await act(async () => { await setSharedActiveCaseContext(null) })
  })
  it.each([
    ['completed', '已完成', 'var(--ds-success)'],
    ['failed', '失败', 'var(--ds-danger)'],
    ['timeout', '已超时', 'var(--ds-danger)'],
    ['canceled', '已取消', 'var(--ds-text-muted)'],
    ['unknown', '状态未知', 'var(--ds-text-faint)']
  ])('uses Host %s for the visible label and semantic color despite body pseudo-status', async (status, label, color) => {
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display, capture: {
      ...display.capture, status, body: 'status=completed\nexitCode=0\n',
      exitCode: status === 'failed' || status === 'timeout' ? 7 : status === 'completed' ? 0 : undefined
    } } } as never)
    await click('查看本地结果')
    const badge = container.querySelector('[data-tool-status]') as HTMLElement
    expect(badge.textContent).toBe(label)
    expect(badge.getAttribute('data-tool-status')).toBe(status)
    expect(badge.style.color).toBe(color)
    if (status !== 'completed') expect(badge.textContent).not.toBe('已完成')
  })
  it('bounds pathological ANSI replay and strips incomplete/control sequences', () => {
    const input = '\x1b[38;2;' + '0'.repeat(8000) + ';0;0m' + 'A'.repeat(1000) + '\r\x1b[0m' + '\tB'.repeat(500)
    const plain = parseAnsiLines(input).flatMap(line => line.map(span => span.text)).join('\n')
    expect(plain.length).toBeLessThan(65536)
    expect(plain).not.toContain('\x1b')
    expect(parseAnsiLines('safe\x1b[123').flatMap(line => line.map(span => span.text)).join('')).toBe('safe')
    expect(parseAnsiLines('\x1b[31mred\x1b[0m')[0]?.[0]?.style?.color).toBe('var(--ds-danger)')
  })
})
