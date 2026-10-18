// @vitest-environment jsdom
import type { ToolResultLocalDisplayV1 } from '../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { ToolRow } from './chat/ToolRow'
import { ProtectedToolResult, ProtectedToolResultScope } from './ProtectedToolResult'
import { WebFetchRow } from './chat/toolviews/web-fetch-row'
import { presentationPolicyFor } from './chat/chat-view-model'
import { parseAnsiLines } from './ansi'
import terminalCss from './TerminalBlock.module.css'
import { getSharedActiveCaseContext, setSharedActiveCaseContext } from '../../../data-analysis/services/analysis/stats-case-overview-resource'
import type { ToolBlock, ThreadEventSink } from '../../../agent/types'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { dispatchAnalytixRuntimeEvent } from '../../../agent/analytix-mapper'
const desktop = vi.hoisted(() => ({ handlers: new Map<string, (...args: unknown[]) => unknown>(), invoke: vi.fn(), on: vi.fn(),
  removeListener: vi.fn(), exposed: undefined as unknown }))
vi.mock('electron', () => ({ clipboard: {}, dialog: {},
  ipcMain: { handle: (channel: string, handler: (...args: unknown[]) => unknown) => desktop.handlers.set(channel, handler) },
  ipcRenderer: { invoke: (...args: unknown[]) => desktop.invoke(...args), on: desktop.on, removeListener: desktop.removeListener },
  contextBridge: { exposeInMainWorld: (_name: string, value: unknown) => { desktop.exposed = value } }, webUtils: { getPathForFile: vi.fn() }
}))
let root: Root, container: HTMLDivElement
const selector = { turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const block: ToolBlock = { kind: 'tool', id: 'tool-a', summary: 'bash', status: 'error', meta: { localResult: selector } }
const display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'bash', capture: {
  kind: 'shell', status: 'timeout', body: '\x1b[31mfirst\x1b[0m\n' + Array.from({length: 20}, (_, i) => 'line ' + i).join('\n') + '\nexitCode=0\nstatus=completed\r\n',
  label: 'printf synthetic', exitCode: 7, durationMs: 5, truncated: false, startLine: 0, endLine: 0, totalLines: 0
} }
const settledShellBlock: ToolBlock = { ...block, toolKind: 'command_execution', meta: { ...block.meta, toolName: 'bash' } }
const shellInspect = (inputJSON?: string, inputTruncated = false) => ({ inputJSON, inputTruncated, schemaTruncated: false,
  callEventAt: null, resultEventAt: null, durationMs: null })
let invalidate: ((id: string | null) => void) | undefined
let api: ReturnType<typeof makeApi>
function makeApi() {
  return { openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<unknown> => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async () => undefined),
    openToolResultLocalWebLink: vi.fn(async (_request: unknown) => ({ ok: true })),
    copyToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })),
    navigateToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true, target: { path: '/synthetic/actual.go', workspaceRoot: '/synthetic', line: 9 } })),
    saveToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })),
    invalidateToolResultLocalDisplay: vi.fn(async (): Promise<void> => undefined),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidate = listener; return () => { invalidate = undefined } }),
    onRuntimeStatus: vi.fn((_listener: (status: { state: string }) => void) => () => undefined) }
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
async function openSettledShell(command = display.capture.label) {
  api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display,
    inspect: shellInspect(JSON.stringify({ command })), capture: { ...display.capture, status: 'completed' } } } as never)
  await act(async () => root.render(createElement(ProtectedToolResult, { block: settledShellBlock, threadId: 'thread-a' })))
  await click('查看本地结果')
}
describe('protected lazy result display', () => {
  it('keeps typed file result bodies private and readonly under their exact admitted tool kind', async () => {
    const source = { ...display, toolName: 'ls', inspect: shellInspect('{}'), capture: { kind: 'file_result', status: 'completed', body: '', label: '', durationMs: 0,
      truncated: false, startLine: 0, endLine: 0, totalLines: 0, fileResult: { operation: 'ls', classification: 'source_exact_local',
        list: { path: '/synthetic/root', relativePath: '.', entries: [{ name: 'SEALED_FILE', displayName: 'SEALED_FILE', kind: 'file' }],
          names: ['SEALED_FILE'], limit: 200, producerTruncated: false, originalCount: 1 } } } }
    const owner: ToolBlock = { ...block, status: 'success', toolKind: 'tool_call', meta: { ...block.meta, toolName: 'ls' } }
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: source } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: owner, threadId: 'thread-a' })))
    await click('查看本地结果')
    const body = container.querySelector('[data-protected-tool-body] [data-protected-tool-readonly]')!
    expect(body.textContent).toContain('SEALED_FILE')
    const range = document.createRange(); range.selectNodeContents(body); document.getSelection()?.removeAllRanges(); document.getSelection()?.addRange(range)
    const event = new Event('copy', { bubbles: true, cancelable: true }); await act(async () => body.dispatchEvent(event))
    expect(event.defaultPrevented).toBe(true); expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
    await act(async () => invalidate?.(null))
    expect(container.textContent).not.toContain('SEALED_FILE')
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: source } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: { ...owner, toolKind: 'file_change' }, threadId: 'thread-a' })))
    await click('查看本地结果')
    expect(container.textContent).not.toContain('SEALED_FILE')
  })
  it('shows the same sealed private error only while its original result lease is expanded', async () => {
    const message = '  Alice\r\nnumber=9007199254740993\n'
    await act(async () => root.render(createElement(ProtectedToolResult, { block: settledShellBlock, threadId: 'thread-a' })))
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display,
      inspect: { ...shellInspect('{}'), outputError: { availability: 'available', entries: [{ message, classification: 'source_exact_local', parallelIndex: 3 }] } } } } as never)
    expect(container.querySelector('[data-tool-io="output"]')).toBeNull()
    await click('查看本地结果')
    const error = container.querySelector('[data-tool-io="output"] pre')
    expect(error?.textContent).toBe(message)
    expect(error?.getAttribute('data-parallel-index')).toBe('3')
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
    const request = api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }
    await act(async () => invalidate?.(request.viewId))
    expect(container.querySelector('[data-tool-io="output"]')).toBeNull()
    expect(container.textContent).not.toContain('9007199254740993')
  })
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
    await openSettledShell()
    expect(container.textContent).toContain('已完成')
    expect(container.textContent).toContain('退出码 7')
    expect(container.querySelector('[data-terminal]')).not.toBeNull()
    expect(container.textContent).toContain('line 19')
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
  it('renders every original command line with one status dot and only one removed terminator', async () => {
    const command = "  printf '%s\\n' actual\ncat <<'EOF'\n\t" + '長'.repeat(600) + '\nEOF\n\n'
    await openSettledShell(command)
    const terminal = container.querySelector('[data-terminal]')!
    const prompts = [...terminal.querySelectorAll('.' + terminalCss.promptLine)]
    expect(prompts.map(prompt => prompt.querySelector('.' + terminalCss.command)?.textContent))
      .toEqual(command.slice(0, -1).split('\n'))
    expect(terminal.querySelectorAll('.' + terminalCss.runState)).toHaveLength(1)
    expect(prompts.map(prompt => prompt.querySelector('.' + terminalCss.cwd)?.textContent)).toEqual(prompts.map(() => '$'))
    expect(terminal.textContent).not.toContain(display.capture.label)
  })
  it.each([
    ['missing inspect', undefined], ['truncated input', shellInspect(undefined, true)],
    ['missing command', shellInspect('{}')], ['empty command', shellInspect('{"command":""}')],
    ['non-string command', shellInspect('{"command":17}')]
  ])('keeps the actual output visible for %s without inventing a command', async (_name, inspect) => {
    const body = 'ACTUAL_OUTPUT_AND_HOST_ERROR\n'
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display, inspect,
      capture: { ...display.capture, status: 'completed', label: 'SUMMARY_IS_NOT_COMMAND', body } } } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: settledShellBlock, threadId: 'thread-a' })))
    await click('查看本地结果')
    expect(container.querySelector('[data-terminal]')).toBeNull()
    expect(container.querySelector('[data-protected-tool-body] pre')?.textContent).toBe(body)
    expect(container.textContent).not.toContain('SUMMARY_IS_NOT_COMMAND')
  })
  it.each(['command', 'command and output'] as const)('blocks the %s selection without copying Result instead', async selectionKind => {
    await openSettledShell('  printf PRIVATE_ORIGINAL_COMMAND\n')
    const prompt = container.querySelector('[data-terminal] [data-protected-tool-readonly]')!
    const output = container.querySelector('[data-terminal] .' + terminalCss.output)!
    const range = document.createRange()
    if (selectionKind === 'command') range.selectNodeContents(prompt)
    else { range.setStart(prompt, 0); range.setEnd(output, output.childNodes.length) }
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => document.dispatchEvent(copy))
    expect(copy.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('copies only OUT through the existing identity-only effect when the command is visible', async () => {
    await openSettledShell('  printf PRIVATE_ORIGINAL_COMMAND\n')
    const output = container.querySelector('[data-terminal] .' + terminalCss.output)!
    const range = document.createRange(); range.selectNodeContents(output)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => document.dispatchEvent(copy))
    expect(copy.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).toHaveBeenCalledExactlyOnceWith({ ...selector, threadId: 'thread-a',
      viewId: expect.any(String), snapshotDigest: display.snapshotDigest })
    expect(JSON.stringify(api.copyToolResultLocalDisplay.mock.calls)).not.toContain('PRIVATE_ORIGINAL_COMMAND')
    expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('retires the original command with its existing protected display lease', async () => {
    await openSettledShell('printf PRIVATE_ORIGINAL_COMMAND')
    const view = (api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }).viewId
    await act(async () => invalidate?.(view))
    expect(container.textContent).not.toContain('PRIVATE_ORIGINAL_COMMAND')
    expect(container.querySelector('[data-terminal]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenLastCalledWith(view)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('clears old body on Host invalidation and can retry an unavailable result', async () => {
    await click('查看本地结果')
    await act(async () => invalidate?.(null))
    expect(container.querySelector('[data-terminal]')).toBeNull()
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: false, code: 'unavailable' } as never)
    await click('查看本地结果')
    expect(container.textContent).toContain('当前不可用')
    await click('重试')
    expect(container.querySelector('[data-protected-tool-body] pre')?.textContent).toContain('line 19')
    expect(container.querySelector('[data-terminal]')).toBeNull()
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
    api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...display,
      ...(status === 'completed' || status === 'unknown' ? { inspect: shellInspect(JSON.stringify({ command: display.capture.label })) } : {}), capture: {
      ...display.capture, status, body: 'status=completed\nexitCode=0\n',
      exitCode: status === 'failed' || status === 'timeout' ? 7 : status === 'completed' ? 0 : undefined
    } } } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: settledShellBlock, threadId: 'thread-a', renderTrigger: controls =>
      createElement(ToolRow, { title: 'Bash', state: controls.state ?? 'ok', expanded: controls.open, expandable: controls.canOpen,
        externalBodyId: controls.bodyId, onToggle: controls.onToggle }) })))
    await act(async () => container.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    if (['failed', 'timeout', 'canceled'].includes(status)) {
      expect(container.querySelector('[data-disclosure-row]')?.closest('[data-state]')?.getAttribute('data-state')).toBe(status === 'canceled' ? 'stopped' : 'error')
      expect(container.querySelector('[data-terminal]')).toBeNull()
      expect(container.querySelector('[data-protected-tool-body] pre')?.textContent).toBe('status=completed\nexitCode=0\n')
    } else {
      const badge = container.querySelector('[data-tool-status]') as HTMLElement
      expect(badge.textContent).toBe(label)
      expect(badge.getAttribute('data-tool-status')).toBe(status)
      expect(badge.style.color).toBe(color)
      if (status !== 'completed') expect(badge.textContent).not.toBe('已完成')
    }
  })
  it('keeps a current lease when its retirement callback changes and invokes the latest callback on revoke', async () => {
    const first = vi.fn(), latest = vi.fn()
    const draw = async (onInspectorRetire: () => void) => act(async () => root.render(createElement(ProtectedToolResult, {
      block, threadId: 'thread-a', inspectRequest: 1, inspectorOnly: true, onInspectorRetire
    })))
    await draw(first)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    const view = (api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }).viewId
    const closes = api.closeToolResultLocalDisplay.mock.calls.length
    await draw(latest)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledTimes(closes)
    await act(async () => invalidate?.(view))
    expect(first).not.toHaveBeenCalled(); expect(latest).toHaveBeenCalledOnce()
    expect(api.closeToolResultLocalDisplay).toHaveBeenLastCalledWith(view)
  })
  it('bounds pathological ANSI replay and strips incomplete/control sequences', () => {
    const input = '\x1b[38;2;' + '0'.repeat(8000) + ';0;0m' + 'A'.repeat(1000) + '\r\x1b[0m' + '\tB'.repeat(500)
    const plain = parseAnsiLines(input).flatMap(line => line.map(span => span.text)).join('\n')
    expect(plain.length).toBeLessThan(65536)
    expect(plain).not.toContain('\x1b')
    expect(parseAnsiLines('safe\x1b[123').flatMap(line => line.map(span => span.text)).join('')).toBe('safe')
    expect(parseAnsiLines('\x1b[31mred\x1b[0m')[0]?.[0]?.style?.color).toBe('var(--ds-danger)')
  })
  it('makes the single atomic trigger the explicit authorized read and fences late completion on collapse', async () => {
    let finish!: (value: { ok: true; display: typeof display }) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-a',
      renderTrigger: ({ open, pending, canOpen, bodyId, onToggle }) => createElement(ToolRow, {
        title: 'Synthetic Shell', expanded: open, expandable: canOpen, externalBodyId: bodyId, onToggle,
        actions: pending ? createElement('span', { role: 'status' }, 'Synthetic loading') : null
      }) })))
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled(); expect(container.textContent).not.toContain('查看本地结果')
    const trigger = container.querySelector<HTMLElement>('[data-disclosure-row]')!
    await act(async () => { trigger.click(); trigger.click() })
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    expect(container.querySelectorAll('[role="status"]')).toHaveLength(1)
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    await act(async () => trigger.click())
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    await act(async () => finish({ ok: true, display }))
    expect(container.querySelector('[data-terminal]')).toBeNull()
    await act(async () => trigger.click())
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
    expect(document.getElementById(trigger.getAttribute('aria-controls')!)!.querySelector('pre')?.textContent).toContain('line 19')
    expect(container.querySelector('[data-terminal]')).toBeNull()
    await act(async () => trigger.click()); expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
  })

})


describe('typed protected result outcome', () => {
  it('uses the actual Host timeout in the single outer trigger and keeps effects identity-only', async () => {
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-a', renderTrigger: controls =>
      createElement(ToolRow, { title: 'Bash', state: controls.state ?? 'ok', expanded: controls.open, expandable: controls.canOpen,
        externalBodyId: controls.bodyId, onToggle: controls.onToggle }) })))
    await act(async () => container.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    expect(container.querySelectorAll('[data-disclosure-row]')).toHaveLength(1)
    expect(container.querySelector('[data-state="error"]')).not.toBeNull(); expect(container.querySelector('[data-terminal]')).toBeNull()
    const body = container.querySelector('[data-protected-tool-body]')!
    expect(body.querySelector('pre')?.textContent).toContain('line 19')
    const range = document.createRange(); range.selectNodeContents(body); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => body.dispatchEvent(copy)); expect(copy.defaultPrevented).toBe(true)
    expect(api.copyToolResultLocalDisplay).toHaveBeenCalledWith(expect.objectContaining({ ...selector, threadId: 'thread-a', snapshotDigest: display.snapshotDigest }))
    expect(JSON.stringify(api.copyToolResultLocalDisplay.mock.calls)).not.toContain('line 10')
  })
})

describe('protected Read card navigation lifetime', () => {
  const readDisplay = { ...display, canNavigate: true, toolName: 'read', capture: { kind: 'read', status: 'completed', body: 'actual code\n',
    label: '/misleading/label…', durationMs: 0, truncated: false, startLine: 9, endLine: 9, totalLines: 20 } }
  it('uses only identities and the returned Host target for the real workspace preview event', async () => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: readDisplay } as never)
    const preview = vi.fn(), event = 'analytix:workspace-file-preview'
    window.addEventListener(event, preview)
    try {
      await click('查看本地结果'); await click('文件预览')
      expect(api.navigateToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread-a', viewId: expect.any(String), snapshotDigest: display.snapshotDigest })
      expect(preview).toHaveBeenCalledOnce()
      expect((preview.mock.calls[0][0] as CustomEvent).detail).toEqual({ path: '/synthetic/actual.go', workspaceRoot: '/synthetic', line: 9, preview: 'source', protectedRead: { ...selector, threadId: 'thread-a', viewId: expect.any(String), snapshotDigest: display.snapshotDigest } })
      expect(JSON.stringify(api.navigateToolResultLocalDisplay.mock.calls)).not.toContain('label')
      expect(JSON.stringify(api.navigateToolResultLocalDisplay.mock.calls)).not.toContain('actual.go')
    } finally { window.removeEventListener(event, preview) }
  })
  it('does not offer navigation for a legacy targetless Read', async () => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: { ...readDisplay, canNavigate: false } } as never)
    await click('查看本地结果')
    expect(container.querySelector('[aria-label="文件预览"]')).toBeNull()
    expect(container.textContent).toContain('actual code')
  })
  it.each(['collapse', 'thread', 'invalidate', 'case'])('drops a late target after %s retirement', async reason => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: readDisplay } as never)
    let finish!: (value: unknown) => void
    api.navigateToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve as (value: unknown) => void }))
    const preview = vi.fn(), event = 'analytix:workspace-file-preview'
    window.addEventListener(event, preview)
    const originalCase = getSharedActiveCaseContext()
    try {
      await click('查看本地结果'); await click('文件预览')
      await act(async () => {
        if (reason === 'collapse') [...container.querySelectorAll('button')].find(item => item.getAttribute('aria-expanded') === 'true')!.click()
        else if (reason === 'thread') root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-b' }))
        else if (reason === 'case') await setSharedActiveCaseContext({ caseId: 'navigation-synthetic', workspaceRoot: '/synthetic/other' })
        else invalidate?.(null)
      })
      await act(async () => finish({ ok: true, target: { path: '/synthetic/actual.go', workspaceRoot: '/synthetic', line: 9 } }))
      expect(preview).not.toHaveBeenCalled()
    } finally { window.removeEventListener(event, preview); await setSharedActiveCaseContext(originalCase) }
  })
})

it('connects actual Main document lease and safe file read through real preload and the mounted card callback', async () => {
  // Load real native owners only in this runner fixture. Their Node ambient
  // declarations must not change the production browser timer type graph.
  const { mkdtemp, realpath, writeFile, rm } = await vi.importActual<{
    mkdtemp: (prefix: string) => Promise<string>; realpath: (path: string) => Promise<string>;
    writeFile: (path: string, content: string) => Promise<void>; rm: (path: string, options: { recursive: boolean }) => Promise<void>
  }>('node:fs/promises')
  const { tmpdir } = await vi.importActual<{ tmpdir: () => string }>('node:os')
  const { join } = await vi.importActual<{ join: (...paths: string[]) => string }>('node:path')
  const { createHash } = await vi.importActual<{ createHash: (algorithm: string) => { update: (body: string) => { digest: (encoding: string) => string } } }>('node:crypto')
  const { EventEmitter } = await vi.importActual<{ EventEmitter: new () => { emit: (event: string, ...args: unknown[]) => boolean } }>('node:events')
  const workspace = await realpath(await mkdtemp(join(tmpdir(), 'analytix-read-facade-'))), path = join(workspace, 'actual.go')
  const body = 'first\nsecond\n'
  await writeFile(path, body)
  const hostDisplay = { schemaVersion: 1, snapshotDigest: display.snapshotDigest, resultItemId: selector.resultItemId, toolName: 'read',
    capture: { kind: 'read', status: 'completed', body: 'second\n', label: '/misleading/label…', durationMs: 0, truncated: false, startLine: 2, endLine: 2, totalLines: 2 },
    inspect: { inputJSON: '{"path":"actual.go"}', schemaJSON: '{"name":"read","description":"Original model request schema","parameters":{"type":"object"}}',
      inputTruncated: false, schemaTruncated: false, callEventAt: '2026-10-10T00:00:00Z', resultEventAt: '2026-10-10T00:00:00.1Z', durationMs: 100 },
    readTarget: { path, workspaceRoot: workspace, startLine: 2, contentDigest: createHash('sha256').update(body).digest('hex') } }
  const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(hostDisplay) }))
  const sender = Object.assign(new EventEmitter(), { id: 19, mainFrame: {}, isDestroyed: () => false,
    send: (channel: string, ...payload: unknown[]) => {
      for (const [name, listener] of desktop.on.mock.calls) if (name === channel) listener({}, ...payload)
    } })
  const main = Object.assign(new EventEmitter(), { webContents: sender, isDestroyed: () => false, isVisible: () => true, isMinimized: () => false })
  const event = { sender, senderFrame: sender.mainFrame }
  const { registerToolResultLocalDisplayIpcV1 } = await vi.importActual<{ registerToolResultLocalDisplayIpcV1: (getWindow: () => unknown, transport: () => Promise<{ ok: boolean; status: number; body: string }>) => (event: unknown, request: unknown) => Promise<import('@shared/workspace-file').WorkspaceFileReadResult> }>('../../../../../main/ipc/tool-result-local-display-ipc')
  const readPreview = registerToolResultLocalDisplayIpcV1(() => main as never, transport)
  desktop.handlers.set('file:read-workspace', (event, payload) => readPreview(event as never, (payload as { protectedRead: unknown }).protectedRead))
  desktop.invoke.mockImplementation(async (channel, payload) => desktop.handlers.get(channel)?.(event, payload))
  await vi.importActual('../../../../../preload/index')
  const actual = desktop.exposed as import('../../../../../shared/analytix-api').AnalytixApi
  Object.defineProperty(window, 'analytix', { configurable: true, value: actual })
  const preview = vi.fn(), previewEvent = 'analytix:workspace-file-preview'
  window.addEventListener(previewEvent, preview)
  try {
    await act(async () => root.render(createElement(ProtectedToolResult, { block: { ...block, toolKind: 'tool_call', meta: { ...block.meta, toolName: 'read' } }, threadId: 'thread-a' })))
    await click('查看本地结果')
    // The actual Main eligibility read crosses filesystem I/O, unlike the
    // synchronous renderer-only fixture. Wait for the real action to appear.
    let opened: unknown
    await act(async () => {
      const openCall = desktop.invoke.mock.calls.findIndex(([channel]) => channel === 'tool-result-local:open')
      expect(openCall).toBeGreaterThanOrEqual(0)
      opened = await desktop.invoke.mock.results[openCall].value
    })
    expect(opened).toMatchObject({ ok: true, display: { canNavigate: true, inspect: hostDisplay.inspect } })
    expect(container.querySelector('[aria-label="文件预览"]')).not.toBeNull()
    await click('文件预览')
    await act(async () => {
      const navigateCall = desktop.invoke.mock.calls.findIndex(([channel]) => channel === 'tool-result-local:navigate')
      expect(navigateCall).toBeGreaterThanOrEqual(0)
      await desktop.invoke.mock.results[navigateCall].value
    })
    expect(preview).toHaveBeenCalledOnce()
    const target = (preview.mock.calls[0][0] as CustomEvent).detail
    expect(target).toMatchObject({ path, workspaceRoot: workspace, line: 2, preview: 'source' })
    expect(JSON.stringify(desktop.invoke.mock.calls)).not.toContain(path)
    await expect(actual.files.read(target)).resolves.toEqual({ ok: true, path, content: body, size: body.length, truncated: false, line: 2 })
    // Same-process document replacement retires the actual native lease even
    // when WebContents and its main frame identity stay the same.
    await act(async () => sender.emit('did-start-navigation', {}, 'https://synthetic.invalid', false, true))
    await expect(actual.files.read(target)).resolves.toMatchObject({ ok: false })
    expect(container.textContent).not.toContain('second')
    expect(transport).toHaveBeenCalledTimes(5)
  } finally {
    window.removeEventListener(previewEvent, preview)
    await act(async () => { sender.emit('destroyed') })
    await rm(workspace, { recursive: true })
  }
})


describe('common Inspect facts in the existing protected result owner', () => {
  const common = { inputJSON: '{"command":"PRIVATE_ORIGINAL_INPUT"}', schemaJSON: '{"name":"bash","description":"PRIVATE_ORIGINAL_SCHEMA","parameters":{"type":"object"}}',
    inputTruncated: false, schemaTruncated: false, callEventAt: '2026-10-10T00:00:00Z', resultEventAt: '2026-10-10T00:00:00.125Z', durationMs: 125 }
  const inspectBlock: ToolBlock = { ...block, toolKind: 'command_execution', meta: { ...block.meta, toolName: 'bash', turnId: selector.turnId, callId: selector.callId } }
  const renderInspector = async (value: unknown = { ...display, inspect: common }, target = inspectBlock, version = 1) => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: value } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: target, threadId: 'thread-a', inspectorOnly: true, inspectRequest: version })))
  }
  it('opens the real ordinary call Inspector and blocks common fact selection without copying Result', async () => {
    await i18n.changeLanguage('en'); await renderInspector()
    expect(container.querySelector('[data-tool-result-inspector]')).not.toBeNull()
    await click('Payload'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_INPUT')
    const panel = container.querySelector('[role="tabpanel"]')!, range = document.createRange()
    range.selectNodeContents(panel); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    for (const type of ['copy', 'cut', 'dragstart']) {
      const event = new Event(type, { bubbles: true, cancelable: true })
      await act(async () => panel.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
    }
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    await click('Schema'); expect(container.textContent).toContain('PRIVATE_ORIGINAL_SCHEMA'); expect(container.textContent).not.toContain('Copy')
    await click('Timing'); expect(container.textContent).toContain('125 ms')
    expect([...container.querySelectorAll('[role="tabpanel"] dd')].map(node => node.textContent)).not.toContain('5 ms')
    await act(async () => invalidate?.(null))
    expect(container.textContent).not.toContain('PRIVATE_ORIGINAL_'); expect(document.getSelection()?.rangeCount).toBe(0)
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it.each(['job', 'inspect'] as const)('keeps %s Result private without export controls or transfer effects', async kind => {
    await i18n.changeLanguage('en')
    const reserved = { status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false }
    const value = kind === 'job' ? { ...display, toolName: 'bash_output', capture: { ...reserved, kind, job: { operation: 'output', jobs: [{ jobId: 'job-a', kind: 'bash', status: 'running',
      name: 'PRIVATE_JOB_NAME', label: null, output: { availability: 'available', body: 'PRIVATE_JOB_BODY', truncated: false } }], total: 1, truncated: false, receipt: null } } }
      : { ...display, inspect: common, capture: { ...reserved, kind } }
    await renderInspector(value, { ...inspectBlock, meta: { ...inspectBlock.meta, toolName: kind === 'job' ? 'bash_output' : 'bash' } })
    await click('Result'); const panel = container.querySelector('[role="tabpanel"]')!
    const range = document.createRange(); range.selectNodeContents(panel); document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    const event = new Event('copy', { bubbles: true, cancelable: true })
    await act(async () => panel.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
    expect(container.textContent).not.toMatch(/Copy|Save/)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('rejects ordinary common facts returned for another producer or call kind', async () => {
    await renderInspector({ ...display, inspect: common }, { ...inspectBlock, meta: { ...inspectBlock.meta, toolName: 'read' } })
    expect(container.textContent).not.toContain('PRIVATE_ORIGINAL_INPUT'); expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
    await renderInspector({ ...display, inspect: common }, { ...inspectBlock, toolKind: 'tool_call' }, 2)
    expect(container.textContent).not.toContain('PRIVATE_ORIGINAL_INPUT'); expect(container.querySelector('[data-terminal]')).toBeNull()
  })
  it('drops a late common-facts reply after the selected call identity changes', async () => {
    let finish!: (value: unknown) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve as (value: unknown) => void }))
    await act(async () => root.render(createElement(ProtectedToolResult, { block: inspectBlock, threadId: 'thread-a', inspectorOnly: true, inspectRequest: 1 })))
    await act(async () => root.render(createElement(ProtectedToolResult, { block: inspectBlock, threadId: 'other-thread', inspectorOnly: true, inspectRequest: 1 })))
    await act(async () => finish({ ok: true, display: { ...display, inspect: common } }))
    expect(container.textContent).not.toContain('PRIVATE_ORIGINAL_'); expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
})

describe('protected native Schedule receipts', () => {
  const task = normalizeAppSettings({ schedule: { tasks: [{ id: 'receipt-task', title: 'FIXED_RECEIPT_TITLE', prompt: 'Synthetic prompt' }] } } as AppSettingsV1).schedule.tasks[0]
  const received = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'mcp__gui_schedule__gui_schedule_create',
    capture: { kind: 'schedule', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
      schedule: { operation: 'create', status: 'success', message: '', task } } }
  const target: ToolBlock = { ...block, toolKind: 'tool_call', status: 'success', meta: { localResult: selector, toolName: 'mcp__gui_schedule__gui_schedule_create' } }
  it('keeps the immutable receipt readable in Inspect without copy or save effects', async () => {
    await i18n.changeLanguage('en')
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: received } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: target, threadId: 'thread-a', inspectorOnly: true, inspectRequest: 1 })))
    await click('Result')
    for (let count = 0; count < 12; count++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    const panel = container.querySelector('[role="tabpanel"]')!, range = document.createRange()
    expect(panel.textContent).toContain('FIXED_RECEIPT_TITLE')
    range.selectNodeContents(panel)
    const selection = document.getSelection()!
    selection.removeAllRanges(); selection.addRange(range)
    expect(selection.isCollapsed).toBe(false)
    for (const operation of ['copy', 'cut', 'dragstart']) {
      const event = new Event(operation, { bubbles: true, cancelable: true })
      await act(async () => panel.dispatchEvent(event))
      expect(event.defaultPrevented, operation).toBe(true)
    }
    expect(container.textContent).not.toMatch(/Copy|Save/)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it.each(['producer', 'kind', 'operation', 'ordinary-capture'] as const)('rejects a Schedule receipt with a mismatched %s', async mismatch => {
    const source = mismatch === 'producer' ? { ...received, toolName: 'mcp__gui_schedule__claw_schedule_create' }
      : mismatch === 'operation' ? { ...received, capture: { ...received.capture, schedule: { ...received.capture.schedule, operation: 'update' } } }
        : mismatch === 'ordinary-capture' ? { ...display, toolName: 'mcp__gui_schedule__gui_schedule_create' } : received
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: source } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, {
      block: mismatch === 'kind' ? { ...target, toolKind: 'command_execution' } : target, threadId: 'thread-a', preload: true
    })))
    expect(container.textContent).not.toContain('FIXED_RECEIPT_TITLE')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
})

describe('Web prefetch lifetime in the existing protected result owner', () => {
  const webDisplay = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'web_fetch',
    canOpenWeb: { request: true, result: true }, capture: { kind: 'web_fetch', status: 'completed', label: 'web_fetch',
      body: 'PRIVATE_ACTUAL_RESPONSE', durationMs: 14, truncated: false, startLine: 0, endLine: 0, totalLines: 0,
      web: { requestUrl: 'https://private.invalid/actual', finalUrl: 'https://private.invalid/final', inputJSON: '{"url":"https://private.invalid/actual"}',
        statusCode: 200, fetchTruncated: false, responseComplete: true, toolIsError: false, startedAt: '2026-10-08T00:00:00Z',
        inputTruncated: false, schemaTruncated: false, requestUrlTruncated: false, finalUrlTruncated: false } } }
  const webBlock: ToolBlock = { ...block, toolKind: 'tool_call', status: 'success', meta: { localResult: selector, toolName: 'web_fetch' } }
  const renderWeb = async (threadId = 'thread-a', visible = true) => {
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: threadId }, createElement(WebFetchRow, {
      block: webBlock, visible, processing: false, policy: presentationPolicyFor('detailed'), privateGeneration: 0, resetKey: 'web'
    }))))
  }
  it.each(['hidden', 'runtime', 'invalidate', 'fold'])('retires the folded URL and same-leaf Inspector on %s', async reason => {
    await i18n.changeLanguage('en'); api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: webDisplay } as never)
    await renderWeb()
    expect(container.textContent).toContain('https://private.invalid/actual')
    expect(container.textContent).not.toContain('PRIVATE_ACTUAL_RESPONSE')
    await act(async () => container.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
    await click('Inspect'); await click('Result')
    expect(container.textContent).toContain('PRIVATE_ACTUAL_RESPONSE')
    if (reason === 'hidden') {
      const descriptor = Object.getOwnPropertyDescriptor(document, 'hidden')
      Object.defineProperty(document, 'hidden', { configurable: true, value: true })
      try { await act(async () => document.dispatchEvent(new Event('visibilitychange'))) }
      finally { if (descriptor) Object.defineProperty(document, 'hidden', descriptor); else Reflect.deleteProperty(document, 'hidden') }
    } else if (reason === 'runtime') {
      const listener = api.onRuntimeStatus.mock.calls.at(-1)?.[0] as unknown as (status: { state: string }) => void
      await act(async () => listener({ state: 'stopped' }))
    } else if (reason === 'fold') await renderWeb('thread-a', false)
    else await act(async () => invalidate?.(null))
    expect(container.innerHTML).not.toContain('private.invalid')
    expect(container.textContent).not.toContain('PRIVATE_ACTUAL_RESPONSE')
    expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
  it('rejects a genuine Web snapshot referenced through a Bash consumer and retires its Host view without effects', async () => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: webDisplay } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: { ...block, toolKind: 'command_execution', meta: { ...block.meta, toolName: 'bash' } }, threadId: 'thread-a' })))
    await click('查看本地结果')
    expect(container.textContent).not.toContain('PRIVATE_ACTUAL_RESPONSE')
    expect(container.innerHTML).not.toContain('private.invalid')
    expect(container.querySelector('[data-tool-result-inspector],[data-terminal]')).toBeNull()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(api.openToolResultLocalWebLink).not.toHaveBeenCalled()
  })
  it('recovers a folded request summary by a single fresh read when the document becomes visible again', async () => {
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: webDisplay } as never)
    await renderWeb()
    const descriptor = Object.getOwnPropertyDescriptor(document, 'hidden')
    try {
      Object.defineProperty(document, 'hidden', { configurable: true, value: true })
      await act(async () => document.dispatchEvent(new Event('visibilitychange')))
      expect(container.innerHTML).not.toContain('private.invalid')
      Object.defineProperty(document, 'hidden', { configurable: true, value: false })
      await act(async () => document.dispatchEvent(new Event('visibilitychange')))
      expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('https://private.invalid/actual')
      expect(container.textContent).not.toContain('PRIVATE_ACTUAL_RESPONSE')
      expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
    } finally { if (descriptor) Object.defineProperty(document, 'hidden', descriptor); else Reflect.deleteProperty(document, 'hidden') }
  })
  it('closes the pending Host view when the prefetch reply rejects after opening', async () => {
    api.openToolResultLocalDisplay.mockRejectedValueOnce(new Error('synthetic transport refusal'))
    await renderWeb()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(expect.any(String))
    expect(container.innerHTML).not.toContain('private.invalid')
    expect(container.querySelector('[data-tool-result-inspector],[data-web-fetch-card]')).toBeNull()
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it('drops late prefetch after the mounted scope retires and preserves Shell/Read click-only behavior', async () => {
    let finish!: (value: unknown) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve as (value: unknown) => void }))
    await renderWeb()
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
    await renderWeb('thread-a', false)
    await act(async () => finish({ ok: true, display: webDisplay }))
    expect(container.innerHTML).not.toContain('private.invalid')
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread-a' })))
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  })
})


describe('protected Present declaration identity', () => {
  const received = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'present',
    inspect: shellInspect('{"files":[{"path":"REQUESTED_FILE.md"}]}'),
    capture: { kind: 'present', status: 'completed', label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0,
      truncated: false, present: { files: [{ index: 0, path: 'ACTUAL_DECLARATION.md', description: 'Original declaration' }] } } }
  const target: ToolBlock = { ...block, toolKind: 'tool_call', status: 'success', meta: { localResult: selector, toolName: 'present' } }
  it('keeps exact declared Result and original Payload readable with no export effects', async () => {
    await i18n.changeLanguage('en')
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: received } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, { block: target, threadId: 'thread-a', inspectorOnly: true, inspectRequest: 1 })))
    await click('Payload')
    for (let count = 0; count < 12; count++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    expect(container.textContent).toContain('REQUESTED_FILE.md')
    await click('Result')
    for (let count = 0; count < 12; count++) {
      const expand = container.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
      if (!expand) break
      await act(async () => expand.click())
    }
    const panel = container.querySelector('[role="tabpanel"]')!
    expect(panel.textContent).toContain('ACTUAL_DECLARATION.md')
    expect(panel.textContent).not.toContain('REQUESTED_FILE.md')
    const range = document.createRange(); range.selectNodeContents(panel)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    for (const operation of ['copy', 'cut', 'dragstart']) {
      const event = new Event(operation, { bubbles: true, cancelable: true })
      await act(async () => panel.dispatchEvent(event)); expect(event.defaultPrevented, operation).toBe(true)
    }
    expect(container.textContent).not.toMatch(/Copy|Save/)
    expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  })
  it.each(['kind', 'producer', 'ordinary-capture'] as const)('rejects a declaration with mismatched %s', async mismatch => {
    const source = mismatch === 'producer' ? { ...received, toolName: 'read' }
      : mismatch === 'ordinary-capture' ? { ...display, toolName: 'present', inspect: received.inspect } : received
    api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: source } as never)
    await act(async () => root.render(createElement(ProtectedToolResult, {
      block: mismatch === 'kind' ? { ...target, toolKind: 'command_execution' } : target, threadId: 'thread-a', preload: true
    })))
    expect(container.textContent).not.toContain('ACTUAL_DECLARATION.md')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  })
})

it('loads generic literal OUT lazily through the exact result owner and clears it on revoke', async () => {
  const textDisplay: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'mcp__docs__lookup',
    capture: { kind: 'inspect', status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 },
    inspect: { outputText: { availability: 'available', body: '{ "n":9007199254740993 }\r\nsecond', classification: 'source_exact_local', truncated: true },
      inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
  const target: ToolBlock = { ...block, status: 'success', toolKind: 'tool_call', meta: { ...block.meta, toolName: textDisplay.toolName } }
  api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: textDisplay })
  await act(async () => root.render(createElement(ProtectedToolResult, { block: target, threadId: 'thread-a' })))
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread-a', ...selector }))
  expect(container.querySelector('[data-tool-text-output]')?.textContent).toBe(textDisplay.inspect!.outputText!.availability === 'available' ? textDisplay.inspect!.outputText!.body : '')
  expect(container.querySelectorAll('[data-json-node],img')).toHaveLength(0)
  expect(container.querySelector('[data-tool-text-output]')?.closest('[data-protected-tool-readonly]')).not.toBeNull()
  expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => invalidate?.(null))
  expect(container.querySelector('[data-tool-text-output]')).toBeNull()
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
})

it('does not fabricate generic OUT for empty captures or accept another result identity', async () => {
  const source: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'mcp__docs__lookup',
    capture: { kind: 'inspect', status: 'completed', body: '', label: '', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0 },
    inspect: { inputJSON: '{}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null } }
  const target: ToolBlock = { ...block, status: 'success', toolKind: 'tool_call', meta: { ...block.meta, toolName: source.toolName } }
  api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: source })
  await act(async () => root.render(createElement(ProtectedToolResult, { block: target, threadId: 'thread-a' })))
  await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
  expect(container.querySelector('[data-protected-tool-body]')).not.toBeNull()
  expect(container.querySelector('[data-tool-text-output], [data-tool-io="output"]')).toBeNull()
  await act(async () => invalidate?.(null))
  api.openToolResultLocalDisplay.mockResolvedValueOnce({ ok: true, display: { ...source, resultItemId: 'item_result_' + 'd'.repeat(64),
    inspect: { ...source.inspect!, outputText: { availability: 'available', body: 'WRONG_RESULT_PRIVATE_TEXT', classification: 'source_exact_local', truncated: false } } } })
  await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
  expect(container.textContent).not.toContain('WRONG_RESULT_PRIVATE_TEXT')
})
