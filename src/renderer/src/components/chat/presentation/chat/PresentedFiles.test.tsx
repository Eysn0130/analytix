// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import i18n from '../../../../i18n'
import type { ToolResultLocalDisplayV1, ToolResultLocalViewRequestV1, ToolResultLocalViewResultV1,
  ToolResultLocalPresentHistoryV1, ToolResultLocalPresentHistoryViewResultV1, ToolResultLocalPresentFileRequestV1,
  ToolResultLocalPresentAssociationRequestV1, ToolResultLocalPresentAssociationResultV1, ToolResultLocalPresentApplicationsResultV1, ToolResultLocalEffectResultV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { GeneratedArtifactMetadata } from '../../../../../../../packages/runtime/src/contracts/generated-artifact'
import { getSharedActiveCaseContext, setSharedActiveCaseContext } from '../../../../data-analysis/services/analysis/stats-case-overview-resource'
import { PresentedFiles, usePresentHistory } from './PresentedFiles'
import { presentCallsForClosing } from './presented-files'
import { ChatView } from './ChatView'
import { deriveTurnSections } from '../../../../thread/projection/derive-turn-sections'
import type { ChatBlock } from '../../../../agent/types'
import { useChatStore } from '../../../../store/chat-store'
const preview = vi.hoisted(() => vi.fn())
const openArtifact = vi.hoisted(() => vi.fn(async () => true))
vi.mock('../../../../office/open-generated-artifact', () => ({ openGeneratedArtifact: openArtifact }))
let priorChatState: ReturnType<typeof useChatStore.getState>
vi.mock('../../../../lib/workspace-file-preview', () => ({ previewWorkspaceFile: preview }))
let root: Root, container: HTMLDivElement
const listeners = new Set<(id: string | null) => void>()
const displays = new Map<string, ToolResultLocalDisplayV1>()
const uuid = (index: number) => '11111111-1111-4111-8111-' + index.toString().padStart(12, '0')
const resultId = (key: string) => 'item_result_' + key.repeat(64)
const closing: ChatBlock = { kind: 'assistant', id: 'closing', text: 'Actual closing', meta: { turnId: 'turn' },
  assistantItemIdentity: { threadId: 'thread', turnId: 'turn', itemId: 'closing' } }
let history: ToolResultLocalPresentHistoryV1
function invalidate(id: string | null) { for (const listener of [...listeners]) listener(id) }
function makeApi() {
  return {
    openToolResultLocalPresentHistory: vi.fn(async (_request: unknown): Promise<ToolResultLocalPresentHistoryViewResultV1> => ({ ok: true, history })),
    openToolResultLocalDisplay: vi.fn(async (request: ToolResultLocalViewRequestV1): Promise<ToolResultLocalViewResultV1> => {
      const display = displays.get(request.resultItemId)
      return display ? { ok: true, display } : { ok: false, code: 'unavailable' }
    }),
    associateToolResultLocalPresentFile: vi.fn(async (request: ToolResultLocalPresentAssociationRequestV1): Promise<ToolResultLocalPresentAssociationResultV1> => ({
      ok: true, fileIdentity: uuid(parseInt(request.resultItemId.at(-1)!, 16) + 1), artifactIds: []
    })),
    listToolResultLocalPresentApplications: vi.fn(async (_request: ToolResultLocalPresentFileRequestV1): Promise<ToolResultLocalPresentApplicationsResultV1> => ({ ok: true, applications: [
      { applicationId: 'org.synthetic.NonDefault', label: 'Actual alternative', isDefault: false },
      { applicationId: 'org.synthetic.Default', label: 'Actual default', isDefault: true }
    ] })),
    openToolResultLocalPresentFile: vi.fn(async (_request: unknown): Promise<ToolResultLocalEffectResultV1> => ({ ok: true })),
    closeToolResultLocalDisplay: vi.fn(async (_id: string) => undefined),
    copyToolResultLocalDisplay: vi.fn(), saveToolResultLocalDisplay: vi.fn(),
    invalidateToolResultLocalDisplay: vi.fn(async () => { invalidate(null) }),
    onToolResultLocalDisplayInvalidated: (listener: (id: string | null) => void) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    onRuntimeStatus: () => () => undefined
  }
}
let api: ReturnType<typeof makeApi>
function fixture(keys = ['a']) {
  const anchors: ToolResultLocalPresentHistoryV1['orderedItems'] = keys.map(key => {
    const itemId = resultId(key)
    displays.set(itemId, { schemaVersion: 1, snapshotDigest: key.repeat(64), resultItemId: itemId, toolName: 'present',
      inspect: { inputJSON: JSON.stringify({ files: [{ path: 'file-' + key + '.md', description: 'Declaration ' + key }] }), inputTruncated: false, schemaTruncated: true,
        callEventAt: null, resultEventAt: null, durationMs: null },
      capture: { kind: 'present', status: 'completed', label: '', body: '', truncated: false, durationMs: 0, startLine: 0, endLine: 0, totalLines: 0,
        present: { files: [{ index: 0, path: 'file-' + key + '.md', description: 'Declaration ' + key }] } } })
    return { kind: 'tool_result', itemId, callId: 'call_host_' + key.repeat(64), toolName: 'present' }
  })
  history = { schemaVersion: 1, kind: 'present_history', threadId: 'thread', turnId: 'turn', orderedItems: [...anchors, { kind: 'assistant_text', itemId: 'closing' }] }
  return presentCallsForClosing('thread', 'turn', [], history, closing, true)
}
async function draw(keys = ['a'], artifacts: GeneratedArtifactMetadata[] = []) {
  const calls = fixture(keys)
  await act(async () => root.render(createElement(PresentedFiles, { threadId: 'thread', calls, artifacts })))
  return calls
}
function button(label: string, within: ParentNode = container) {
  const target = [...within.querySelectorAll<HTMLButtonElement>('button')].find(node => node.getAttribute('aria-label') === label || node.textContent?.trim() === label)
  expect(target, label).toBeDefined(); return target!
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en')
  priorChatState = useChatStore.getState(); openArtifact.mockReset(); openArtifact.mockResolvedValue(true)
  displays.clear(); listeners.clear(); preview.mockClear(); api = makeApi(); fixture()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges(); useChatStore.setState(priorChatState)
  document.body.style.removeProperty('zoom'); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
it('shares one Office/Present group, first position/last declaration and the winning original Inspect', async () => {
  const office: GeneratedArtifactMetadata = { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64), byteSize: 42, savedAt: '2026-10-11T00:00:00Z' }
  api.associateToolResultLocalPresentFile.mockImplementation(async request => ({ ok: true, fileIdentity: uuid(1),
    artifactIds: request.resultItemId === resultId('a') ? [office.artifactId] : [] }))
  await draw(['a', 'b'], [office])
  expect(container.querySelectorAll('[data-presented-deliverables]')).toHaveLength(1)
  expect(container.querySelectorAll('[data-generated-office-artifact]')).toHaveLength(0)
  expect(container.querySelectorAll('[data-presented-file]')).toHaveLength(1)
  expect(container.textContent).toContain('Declaration b'); expect(container.textContent).not.toContain('Declaration a')
  await act(async () => button('Inspect').click())
  const inspector = container.querySelector<HTMLElement>('[data-tool-result-inspector]')!
  expect(inspector?.dataset.callId).toBe('call_host_' + 'b'.repeat(64))
  expect(inspector.textContent).toContain(resultId('b'))
  await act(async () => button('Result', inspector).click())
  for (let count = 0; count < 12; count++) {
    const expand = inspector.querySelector<HTMLButtonElement>('[data-json-node] button[aria-expanded="false"]')
    if (!expand) break
    await act(async () => expand.click())
  }
  expect(inspector.querySelector('[role="tabpanel"]')?.textContent).toContain('file-b.md')
  expect(inspector.querySelector('[role="tabpanel"]')?.textContent).not.toContain('file-a.md')
  expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
})
it.each([true, false])('opens the Host-associated Office receipt from the winning Present card; success=%s', async success => {
  useChatStore.setState({ activeThreadId: 'thread', workspaceRoot: '/synthetic-workspace' })
  openArtifact.mockResolvedValue(success)
  const office: GeneratedArtifactMetadata = { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64), byteSize: 42, savedAt: '2026-10-11T00:00:00Z' }
  const calls = fixture(['a', 'b'])
  for (const display of displays.values()) if (display.capture.kind === 'present') display.capture.present.files[0].path = 'report.docx'
  api.associateToolResultLocalPresentFile.mockImplementation(async request => ({ ok: true, fileIdentity: uuid(1),
    artifactIds: request.resultItemId === resultId('a') ? [office.artifactId] : [] }))
  await act(async () => root.render(createElement(PresentedFiles, { threadId: 'thread', calls, artifacts: [office] })))
  expect(container.querySelectorAll('[data-presented-file]')).toHaveLength(1)
  expect(container.querySelector('[data-generated-office-artifact]')).toBeNull()
  await act(async () => button('Preview report.docx').click())
  expect(openArtifact).toHaveBeenCalledExactlyOnceWith(office.artifactId, { threadId: 'thread', workspace: '/synthetic-workspace' })
  expect(preview).not.toHaveBeenCalled()
  expect(api.openToolResultLocalPresentFile).not.toHaveBeenCalled()
  if (!success) expect(container.textContent).toContain(i18n.t('common:dshPresent.unavailable'))
})
it('uses one four-card expander for mixed Office and Present cards', async () => {
  const office: GeneratedArtifactMetadata = { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64), byteSize: 42, savedAt: '2026-10-11T00:00:00Z' }
  await draw(['a', 'b', 'c', 'd'], [office])
  expect(container.querySelectorAll('[data-presented-file]')).toHaveLength(3)
  expect(container.querySelectorAll('[data-generated-office-artifact]')).toHaveLength(1)
  const toggle = container.querySelector<HTMLButtonElement>('[aria-expanded="false"]:not([aria-haspopup])')!
  expect(toggle).not.toBeNull(); await act(async () => toggle.click())
  expect(container.querySelectorAll('[data-presented-file]')).toHaveLength(4)
  expect(container.querySelectorAll('[aria-expanded="true"]:not([aria-haspopup])')).toHaveLength(1)
})
it('sends only original index authority for preview/default/actual alternative and handles zoomed portal keys', async () => {
  await draw()
  await act(async () => button('Preview file-a.md').click())
  expect(preview).toHaveBeenCalledWith({ path: 'file-a.md', preview: 'source', protectedPresent: expect.objectContaining({ resultItemId: resultId('a'), index: 0 }) })
  await act(async () => button('Open in Actual default').click())
  expect(api.openToolResultLocalPresentFile.mock.calls[0][0]).toMatchObject({ resultItemId: resultId('a'), index: 0 })
  expect(api.openToolResultLocalPresentFile.mock.calls[0][0]).not.toHaveProperty('path')
  expect(api.openToolResultLocalPresentFile.mock.calls[0][0]).not.toHaveProperty('applicationId')
  document.body.style.setProperty('zoom', '2')
  const trigger = button('Open with…')
  vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(new DOMRect(400, 100, 40, 20))
  await act(async () => trigger.click())
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(menu).not.toBeNull(); expect(parseFloat(menu.style.left)).toBe(4)
  expect(document.activeElement?.textContent).toContain('Actual alternative')
  await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true })))
  expect(document.activeElement?.textContent).toContain('Show file location')
  await act(async () => button('Actual alternative', menu).click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenLastCalledWith(expect.objectContaining({ applicationId: 'org.synthetic.NonDefault', index: 0 }))
})
it('promotes the first actual application without an OS default marker and keeps Reveal last', async () => {
  api.listToolResultLocalPresentApplications.mockResolvedValue({ ok: true, applications: [{ applicationId: 'org.synthetic.Only', label: 'Actual only', isDefault: false }] })
  await draw(); expect(button('Open in Actual only').disabled).toBe(false)
  await act(async () => button('Open in Actual only').click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenLastCalledWith(expect.objectContaining({ applicationId: 'org.synthetic.Only' }))
  await act(async () => button('Open with…').click())
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(button('Actual only (default)', menu).getAttribute('aria-checked')).toBe('true')
  expect(menu.querySelector('button:last-child')?.textContent).toBe('Show file location')
  await act(async () => button('Show file location', menu).click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenLastCalledWith(expect.objectContaining({ action: 'reveal', index: 0 }))
  expect(api.openToolResultLocalPresentFile.mock.lastCall![0]).not.toHaveProperty('applicationId')
})
it('refreshes actual associations on every menu-open gesture and names the current preferred application', async () => {
  await draw()
  expect(api.listToolResultLocalPresentApplications).toHaveBeenCalledOnce()
  api.listToolResultLocalPresentApplications.mockResolvedValue({ ok: true, applications: [
    { applicationId: 'org.synthetic.New', label: 'Current preference', isDefault: false },
    { applicationId: 'org.synthetic.Second', label: 'Current alternative', isDefault: false }
  ] })
  await act(async () => button('Open with…').click())
  expect(api.listToolResultLocalPresentApplications).toHaveBeenCalledTimes(2)
  expect(button('Open in Current preference').disabled).toBe(false)
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(menu.textContent).not.toContain('Actual default')
  expect(button('Current preference (default)', menu).getAttribute('aria-checked')).toBe('true')
  await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(api.listToolResultLocalPresentApplications).toHaveBeenCalledTimes(2)
  await act(async () => button('Open with…').click())
  expect(api.listToolResultLocalPresentApplications).toHaveBeenCalledTimes(3)
  await act(async () => button('Current alternative', document).click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenLastCalledWith(expect.objectContaining({ applicationId: 'org.synthetic.Second', index: 0 }))
})
it('keeps application-query failure disabled in the menu and a protected Reveal footer enabled', async () => {
  api.listToolResultLocalPresentApplications.mockRejectedValue(new Error('synthetic association failure'))
  await draw()
  await act(async () => button('Open with…').click())
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(button('Could not load applications', menu).disabled).toBe(true)
  expect(button('Show file location (default)', menu).disabled).toBe(false)
  expect(document.activeElement?.textContent).toContain('Show file location')
  await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true })))
  expect(document.activeElement?.textContent).toContain('Show file location')
  await act(async () => button('Show file location (default)', menu).click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenLastCalledWith(expect.objectContaining({ action: 'reveal', resultItemId: resultId('a'), index: 0 }))
  expect(api.openToolResultLocalPresentFile.mock.lastCall![0]).not.toHaveProperty('path')
})
it('uses Reveal as the default for a successful empty association list without inventing an application', async () => {
  api.listToolResultLocalPresentApplications.mockResolvedValue({ ok: true, applications: [] })
  await draw()
  expect(container.querySelector('[aria-haspopup="menu"]')).toBeNull()
  expect(button('Show file location').disabled).toBe(false)
  await act(async () => button('Show file location').click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ action: 'reveal', index: 0 }))
  expect(api.openToolResultLocalPresentFile.mock.lastCall![0]).not.toHaveProperty('applicationId')
  expect(container.querySelector('[role="status"]')?.textContent).toContain('Requested display in file manager')
})
it.each([false, true])('serializes the pending %s reveal gesture and exposes failure before retry', async reveal => {
  let finish!: (value: ToolResultLocalEffectResultV1) => void
  api.openToolResultLocalPresentFile.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw()
  if (reveal) {
    await act(async () => button('Open with…').click())
    await act(async () => button('Show file location', document).click())
  } else await act(async () => button('Open in Actual default').click())
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(button('Open in Actual default').disabled).toBe(true)
  expect(button('Open with…').disabled).toBe(true)
  expect(container.querySelector('[role="status"]')?.textContent).toContain(reveal ? 'Showing in file manager…' : 'Opening…')
  await act(async () => button('Open in Actual default').click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenCalledOnce()
  await act(async () => finish({ ok: false, code: 'unavailable' }))
  expect(button('Open in Actual default').disabled).toBe(false)
  expect(container.querySelector('[role="status"]')?.getAttribute('data-error')).toBe('true')
  expect(container.querySelector('[role="status"]')?.textContent).toContain(reveal ? 'Could not show in file manager' : 'Could not open this file')
  await act(async () => button('Open in Actual default').click())
  expect(api.openToolResultLocalPresentFile).toHaveBeenCalledTimes(2)
})
it('masks a pending menu refresh and its late application list after the original lease is revoked', async () => {
  await draw()
  let finish!: (value: ToolResultLocalPresentApplicationsResultV1) => void
  api.listToolResultLocalPresentApplications.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await act(async () => button('Open with…').click())
  expect(button('Open with…').disabled).toBe(true)
  expect(button('Open in Actual default').disabled).toBe(true)
  expect(document.querySelector('[role="menu"]')).toBeNull()
  await act(async () => button('Open in Actual default').click())
  expect(api.openToolResultLocalPresentFile).not.toHaveBeenCalled()
  const identity = api.openToolResultLocalDisplay.mock.calls[0][0]
  await act(async () => invalidate(identity.viewId))
  await act(async () => finish({ ok: true, applications: [{ applicationId: 'org.synthetic.Late', label: 'Late application', isDefault: true }] }))
  expect(container.querySelector('[data-presented-file]')).toBeNull()
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(document.body.textContent).not.toContain('Late application')
})
it('honors Escape while a menu refresh is pending and never reopens it on the late reply', async () => {
  await draw()
  let finish!: (value: ToolResultLocalPresentApplicationsResultV1) => void
  api.listToolResultLocalPresentApplications.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await act(async () => button('Open with…').click())
  expect(document.querySelector('[role="menu"]')).toBeNull()
  await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
  await act(async () => finish({ ok: true, applications: [{ applicationId: 'org.synthetic.Current', label: 'Current app', isDefault: true }] }))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(button('Open with…').getAttribute('aria-expanded')).toBe('false')
  expect(button('Open in Current app').disabled).toBe(false)
})
it('honors Tab during pending menu refresh without canceling normal focus movement or stealing it on the late reply', async () => {
  await draw()
  let finish!: (value: ToolResultLocalPresentApplicationsResultV1) => void
  api.listToolResultLocalPresentApplications.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const trigger = button('Open with…'), editor = document.createElement('textarea')
  container.append(editor); trigger.focus()
  await act(async () => trigger.click())
  expect(document.querySelector('[role="menu"]')).toBeNull()
  const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
  await act(async () => document.activeElement!.dispatchEvent(tab))
  expect(tab.defaultPrevented).toBe(false)
  // jsdom dispatches the real DOM key event but has no Tab default navigation.
  // Model the browser's uncanceled move to the next ordinary focus target.
  editor.focus()
  expect(document.activeElement).toBe(editor)
  await act(async () => finish({ ok: true, applications: [{ applicationId: 'org.synthetic.Current', label: 'Current app', isDefault: true }] }))
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(button('Open with…').getAttribute('aria-expanded')).toBe('false')
  expect(button('Open in Current app').disabled).toBe(false)
  expect(document.activeElement).toBe(editor)
})
it('positions the refreshed menu using its current longer application list before paint', async () => {
  await draw()
  let finish!: (value: ToolResultLocalPresentApplicationsResultV1) => void
  api.listToolResultLocalPresentApplications.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const trigger = button('Open with…')
  vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(new DOMRect(600, window.innerHeight - 30, 40, 20))
  await act(async () => trigger.click())
  await act(async () => finish({ ok: true, applications: Array.from({ length: 7 }, (_, index) => ({
    applicationId: 'org.synthetic.Current' + index, label: 'Current app ' + index, isDefault: index === 0
  })) }))
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!
  expect(menu).not.toBeNull()
  expect(parseFloat(menu.style.top)).toBe(window.innerHeight - (8 * 36 + 16) - 8)
  expect(menu.querySelectorAll('[role="menuitemradio"]')).toHaveLength(7)
  expect(menu.querySelector('button:last-child')?.textContent).toBe('Show file location')
})
it('ignores a late native Reveal completion after the whole closing cohort retires', async () => {
  let finish!: (value: ToolResultLocalEffectResultV1) => void
  api.openToolResultLocalPresentFile.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  await draw()
  await act(async () => button('Open with…').click())
  await act(async () => button('Show file location', document).click())
  const identity = api.openToolResultLocalDisplay.mock.calls[0][0]
  await act(async () => invalidate(identity.viewId))
  await act(async () => finish({ ok: true }))
  expect(container.querySelector('[data-presented-file]')).toBeNull()
  expect(container.querySelector('[role="status"]')).toBeNull()
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(api.openToolResultLocalPresentFile).toHaveBeenCalledOnce()
})
it('blocks private drag/copy without intercepting an ordinary focused editor', async () => {
  await draw(); const card = container.querySelector<HTMLElement>('[data-presented-file]')!
  const editor = document.createElement('textarea'); document.body.append(editor); editor.focus()
  const ordinary = new Event('copy', { bubbles: true, cancelable: true }); editor.dispatchEvent(ordinary)
  expect(ordinary.defaultPrevented).toBe(false)
  const drag = new Event('dragstart', { bubbles: true, cancelable: true }); card.dispatchEvent(drag)
  expect(drag.defaultPrevented).toBe(true)
  editor.blur(); const selection = document.getSelection()!, range = document.createRange(); range.selectNodeContents(card); selection.removeAllRanges(); selection.addRange(range)
  const copy = new Event('copy', { bubbles: true, cancelable: true }); document.dispatchEvent(copy)
  expect(copy.defaultPrevented).toBe(true)
  editor.remove(); expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
})
it('retires the entire closing cohort including a pending second hydration and open portal', async () => {
  let finish!: (result: ToolResultLocalViewResultV1) => void
  api.openToolResultLocalDisplay.mockImplementation(async request => request.resultItemId === resultId('b')
    ? new Promise(resolve => { finish = resolve }) : { ok: true, display: displays.get(request.resultItemId)! })
  await draw(['a', 'b']); await act(async () => button('Open with…').click())
  const first = api.openToolResultLocalDisplay.mock.calls.find(([request]) => request.resultItemId === resultId('a'))![0]
  await act(async () => invalidate(first.viewId))
  expect(document.querySelector('[role="menu"]')).toBeNull(); expect(container.querySelector('[data-presented-file]')).toBeNull()
  await act(async () => finish({ ok: true, display: displays.get(resultId('b'))! }))
  expect(container.querySelector('[data-presented-file]')).toBeNull()
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
})
it('retains the Office-only fallback when no private bridge exists', async () => {
  Reflect.deleteProperty(window, 'analytix')
  const office: GeneratedArtifactMetadata = { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64),
    byteSize: 42, savedAt: '2026-10-11T00:00:00Z' }
  await act(async () => root.render(createElement(PresentedFiles, { threadId: 'thread', calls: [], artifacts: [office] })))
  expect(container.querySelectorAll('[data-generated-office-artifact]')).toHaveLength(1)
  expect(container.querySelector('[data-presented-file]')).toBeNull()
  expect(api.openToolResultLocalPresentHistory).not.toHaveBeenCalled()
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
})
it('places a case tail after the actual closing even when public history contains no tool result', async () => {
  const user: ChatBlock = { kind: 'user', id: 'user', text: 'Request', meta: { turnId: 'turn' } }
  const turn = { user, blocks: [closing] }
  await act(async () => root.render(createElement(ChatView, { threadId: 'thread', turnId: 'turn', turn,
    sections: deriveTurnSections({ turn, isProcessing: false, liveProcessText: '', liveContent: '', workspaceRoot: '/synthetic' }), processing: false, turnStatus: 'completed' })))
  const answer = [...container.querySelectorAll('.ds-chat-answer')].find(node => node.textContent?.includes('Actual closing'))
  const card = container.querySelector('[data-presented-file]')
  expect(card).not.toBeNull(); expect(answer).toBeDefined()
  expect(answer!.compareDocumentPosition(card!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
it.each(['failed', 'aborted'] as const)('uses true %s end when partial text has no approved closing identity', async status => {
  history = { ...history, orderedItems: history.orderedItems.filter(item => item.kind !== 'assistant_text') }
  const partial: ChatBlock = { kind: 'assistant', id: 'partial', text: 'Partial', meta: { turnId: 'turn' } }
  const office: ChatBlock = { kind: 'tool', id: 'office', summary: 'Generate document', status: 'success', meta: { turnId: 'turn',
    generatedArtifact: { artifactId: 'd'.repeat(64), kind: 'docx', contentHash: 'e'.repeat(64), byteSize: 42, savedAt: '2026-10-11T00:00:00Z' } } }
  const turn = { blocks: [office, partial] }
  await act(async () => root.render(createElement(ChatView, { threadId: 'thread', turnId: 'turn', turn,
    sections: deriveTurnSections({ turn, isProcessing: false, liveProcessText: '', liveContent: '', workspaceRoot: '/synthetic' }), processing: true, turnStatus: 'running' })))
  expect(container.querySelector('[data-presented-file]')).toBeNull()
  await act(async () => root.render(createElement(ChatView, { threadId: 'thread', turnId: 'turn', turn,
    sections: deriveTurnSections({ turn, isProcessing: false, liveProcessText: '', liveContent: '', workspaceRoot: '/synthetic' }), processing: false, turnStatus: status })))
  expect(container.querySelectorAll('[data-presented-file]')).toHaveLength(1)
  expect(container.querySelectorAll('[data-presented-deliverables]')).toHaveLength(1)
  expect(container.querySelectorAll('[data-generated-office-deliverables]')).toHaveLength(0)
  expect(container.querySelector('[data-presented-deliverables]')?.querySelectorAll('[data-generated-office-artifact]')).toHaveLength(1)
})
function HistoryHarness() {
  const current = usePresentHistory('thread', 'turn', [closing], false, 'completed')
  return createElement('div', { 'data-history-current': current ? 'true' : 'false' })
}
it('clears pending old history through case ACK and reacquires only the new current lease', async () => {
  const prior = getSharedActiveCaseContext()
  let ack!: () => void, late!: (value: ToolResultLocalPresentHistoryViewResultV1) => void
  api.invalidateToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { ack = resolve }))
  api.openToolResultLocalPresentHistory.mockImplementationOnce(() => new Promise(resolve => { late = resolve }))
  await act(async () => root.render(createElement(HistoryHarness)))
  let transition!: Promise<unknown>
  await act(async () => { transition = setSharedActiveCaseContext({ caseId: 'present-fixture', workspaceRoot: '/synthetic' }) })
  await act(async () => late({ ok: true, history }))
  expect(container.querySelector('[data-history-current]')?.getAttribute('data-history-current')).toBe('false')
  await act(async () => { ack(); await transition })
  expect(api.openToolResultLocalPresentHistory.mock.calls.length).toBeGreaterThan(1)
  expect(container.querySelector('[data-history-current]')?.getAttribute('data-history-current')).toBe('true')
  await act(async () => root.render(createElement('div')))
  await setSharedActiveCaseContext(prior)
})
