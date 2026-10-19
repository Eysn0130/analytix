// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ToolCallTree } from '../ToolCallTree'
import { ToolRow } from '../ToolRow'
import { processActivityFor, processActivityForName, toolFamilyFor } from '../ToolViews'
import { presentationPolicyFor } from '../chat-view-model'
import { DiffBlock } from '../../DiffBlock'
import { ProcessToolCardBody, toolCardLabels } from './ToolCardBody'
import { diffCardModel } from '../models/diff-card-model'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
const tool = (name: string): ToolBlock => ({ kind: 'tool', id: name, status: 'success', summary: name, detail: 'Actual public output', meta: { toolName: name } })
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
const draw = async (blocks: ToolBlock[]) => {
  await act(async () => root.render(createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('detailed') })))
  for (const header of container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')) {
    await act(async () => header.click())
  }
}
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); previous = useChatStore.getState(); useChatStore.setState({ workspaceRoot: '' }); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals() })
describe('keyed typed tool route and action boundaries', () => {
  it('retains an already public Bash command on failure and keeps the error in its expandable body', async () => {
    const block: ToolBlock = { ...tool('bash'), toolKind: 'command_execution', status: 'error',
      detail: 'Published failure line\nPublished failure detail', meta: { toolName: 'bash', command: 'npm run focused-check' } }
    await draw([block])
    const header = container.querySelector<HTMLElement>('[data-sample="bash"]')!
    expect(header.textContent).toContain('npm run focused-check')
    expect(header.textContent).not.toContain('Published failure line')
    expect(header.dataset.state).toBe('error')
    await act(async () => header.click())
    expect(container.textContent).toContain('Published failure line\nPublished failure detail')
  })
  it('retains the real failure line when a shell route has no command-execution kind', async () => {
    await draw([{ ...tool('bash'), status: 'error', detail: 'Published failure line\nMore detail',
      meta: { toolName: 'bash', command: 'Unusable command hint' } }])
    expect(container.querySelector('[data-sample="bash"]')?.textContent).toContain('Published failure line')
  })
  it.each([
    ['terminal_open', 'commands'], ['terminal_read', 'commands'], ['terminal_list', 'commands'],
    ['terminal_signal', 'commands'], ['terminal_close', 'commands'], ['subagent_wait', 'subagents'],
    ['workspace_inspect', 'search'], ['mcp__server__workspace_inspect', 'search'],
    ['terminal_inspect', 'search'], ['subagent_inspect', 'search']
  ] as const)('classifies pinned presentation name %s as %s without widening typed dispatch', (name, activity) => {
    // These are fixed-pin display-contract inputs, not assertions of built-in registration.
    expect(processActivityForName(name)).toBe(activity)
    expect(processActivityFor(tool(name))).toBe(activity)
    expect(toolFamilyFor(tool(name))).toBe('tool')
  })
  it.each(['task', 'parallel_tasks', 'delegate_task'])('classifies the registered %s child producer without requiring a kind hint', name => {
    expect(processActivityFor(tool(name))).toBe('subagents')
    expect(toolFamilyFor(tool(name))).toBe('subagent')
  })
  it.each([
    'custom_terminal_open', 'terminalopen', 'terminal_', 'custom_subagent_wait', 'subagent_',
    'subagents_wait', 'workspace_inspection', 'workspace_inspect_more', '_inspect',
    'inspect_workbench_history', 'inspect_case_schema', 'cordis_inspect_list',
    'mcp__server__terminal_open', 'mcp__server__subagent_wait', 'custom_read_bash_web_search',
    'constructor', '__proto__', 'toString'
  ])('keeps near-matching name %s generic without a typed kind', name => {
    expect(processActivityForName(name)).toBe('tools')
    expect(toolFamilyFor(tool(name))).toBe('tool')
  })
  it('keeps the existing typed fallback and ignores non-string tool names', () => {
    expect(processActivityForName('custom_terminal_open', 'command_execution')).toBe('commands')
    expect(processActivityForName('custom_subagent_wait', 'subagent')).toBe('subagents')
    for (const name of [undefined, null, 17, {}, ['terminal_open']]) expect(processActivityForName(name)).toBe('tools')
  })
  it('dispatches exact names and falls back safely for names merely containing a family word', async () => {
    expect(toolFamilyFor(tool('read_file'))).toBe('read'); expect(toolFamilyFor(tool('exec_command'))).toBe('shell')
    expect(toolFamilyFor(tool('custom_read_bash_web_search'))).toBe('tool')
    await draw([tool('custom_read_bash_web_search')]); expect(container.textContent).toContain('custom_read_bash_web_search')
    await act(async () => container.querySelector<HTMLElement>('[data-chat-seat] [data-disclosure-row]')!.click())
    expect(container.textContent).toContain('Actual public output'); expect(container.querySelector('[data-read],[data-diff],[data-terminal]')).toBeNull()
  })
  it('renders the independent Bash header and real running lifecycle without a generic ToolRow wrapper', async () => {
    await draw([{ ...tool('bash'), status: 'running' }])
    const header = container.querySelector<HTMLElement>('[data-sample="bash"]')!
    expect(header).not.toBeNull(); expect(header.dataset.state).toBe('running')
    expect(header.parentElement!.querySelectorAll('[data-disclosure-row]').length).toBe(1)
    await act(async () => header.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })))
    expect(container.textContent).toContain('Actual public output')
  })
  it('keeps filePathLine, file open, inspect and disclosure independent under keyboard events', async () => {
    const open = vi.fn(), toggle = vi.fn(), inspect = vi.fn()
    await act(async () => root.render(createElement(ToolRow, { title: 'Read', summary: '/synthetic/a.ts', filePath: '/synthetic/a.ts', filePathLine: 42,
      onOpenFile: open, onToggle: toggle, expandable: true, expanded: true, body: { kind: 'details', content: 'Body' }, inspect: { label: 'Inspect', run: inspect } })))
    const file = container.querySelector<HTMLButtonElement>('[data-tool-file-link]')!
    expect(file.textContent).toContain(':42')
    await act(async () => { file.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })); file.click() })
    expect(open).toHaveBeenCalledWith('/synthetic/a.ts', 42); expect(toggle).not.toHaveBeenCalled()
    await act(async () => [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!.click())
    expect(inspect).toHaveBeenCalledOnce(); expect(toggle).not.toHaveBeenCalled()
  })
  it('keeps HTTP URL keyboard input from toggling the row and withholds unsafe hrefs in adapters', async () => {
    const toggle = vi.fn()
    await act(async () => root.render(createElement(ToolRow, { title: 'Fetch', summary: 'Source', href: 'https://example.invalid/source', expandable: true, onToggle: toggle })))
    const link = container.querySelector<HTMLAnchorElement>('a')!
    await act(async () => link.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true })))
    expect(toggle).not.toHaveBeenCalled()
  })
  it('draws the existing public plan projection as fields, retaining unknown-tool names', async () => {
    const b = tool('create_plan'); b.meta!.publicResult = { schemaVersion: 1, projectionKind: 'plan_status', disclosure: 'metadata_only', status: 'completed',
      messageKey: 'plan_updated', code: 'plan_updated', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
      plan: { planId: 'plan-a', relativePath: 'plan.md', operation: 'draft', contentHash: 'a'.repeat(64), byteSize: 500, savedAt: '2026-10-07T00:00:00Z' } }
    await draw([b]); await act(async () => container.querySelector<HTMLElement>('[data-chat-seat] [data-disclosure-row]')!.click())
    expect(container.querySelector('[data-tool-details]')).not.toBeNull(); expect(container.textContent).toContain('plan.md'); expect(container.textContent).toContain('500')
  })
  it('keeps unproduced Search/Web/Details result shapes on the safe generic route', async () => {
    for (const name of ['grep', 'web_search', 'schedule_list']) {
      const b = tool(name); b.detail = '{"sources":[{"url":"javascript:alert(1)"}],"files":[{"path":"/private/a"}]}'
      await draw([b]); await act(async () => container.querySelector<HTMLElement>('[data-chat-seat] [data-disclosure-row]')!.click())
      expect(container.textContent).toContain(b.detail); expect(container.querySelector('[data-tool-details],a,[data-read],[data-diff]')).toBeNull()
    }
  })
  it('draws an authorized Host applied patch without default clipboard/save or foreign actions', async () => {
    const block: ProcessDisplayBlockV1 = { blockId: 'a', kind: 'write', state: 'completed', attempt: 1,
      source: { kind: 'tool', unitId: 'unit', callId: 'call', toolCallItemId: 'call-item', toolResultItemId: 'result-item' },
      captureDigest: 'a'.repeat(64), path: '/synthetic/file.ts', effectApplied: true, settlementStatus: 'completed', durationMs: 0,
      truncated: false, classification: 'credential_redacted', body: { text: '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old\n+new\n', before: 'PRIVATE_BEFORE', after: 'PRIVATE_AFTER', error: '', addedLines: 1, removedLines: 1, approximate: false } }
    const clipboard = vi.fn(); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboard } })
    await act(async () => root.render(createElement(DiffBlock, { model: diffCardModel(block)!, labels: toolCardLabels(key => key) })))
    expect(container.querySelector('[data-diff]')).not.toBeNull(); expect(container.textContent).toContain('old'); expect(container.textContent).toContain('new')
    expect(container.textContent).not.toContain('PRIVATE_AFTER'); expect(container.querySelector('button[aria-label="protectedToolCopy"]')).toBeNull()
    expect(clipboard).not.toHaveBeenCalled()
    await act(async () => root.render(createElement(ProcessToolCardBody, { block: { ...block, kind: 'shell', effectApplied: undefined, settlementStatus: undefined, path: undefined, title: 'actual command', exitCode: undefined } })))
    expect(container.querySelector('[data-terminal]')).not.toBeNull(); expect(container.textContent).not.toContain('Copy'); expect(container.textContent).not.toContain('Save')
    expect(clipboard).not.toHaveBeenCalled()
  })
})
