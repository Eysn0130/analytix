// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import type { WorkspaceTab } from '../store/workspace-tabs-store'
import type { CanvasHostRequest } from '../../../../packages/runtime/src/contracts/canvas-host'
import { CanvasWorkspacePanel } from './CanvasWorkspacePanel'
import { canvasPreviewSessions } from './canvas-preview-owner'
import { useNativeReferenceStore } from '../office/native-reference-store'

const decode = vi.hoisted(() => vi.fn())
vi.mock('./canvas-preview', () => ({ decodeCanvasPreview: decode }))
// Component tests isolate image decoding. canvas-preview.test.ts checks real
// hashing, shared schema and the actual AtlasFlow renderer independently.
let root: Root, container: HTMLDivElement
const request = vi.fn(), pickFile = vi.fn(), onOpenObject = vi.fn(), writeText = vi.fn()
const revokeObjectURL = vi.fn(), createObjectURL = vi.fn()
const tab: WorkspaceTab = { id: 'sample', kind: 'file', mode: 'file', preview: 'canvas', workspaceRoot: '/project', path: '/project/flow.canvas', title: 'flow.canvas' }
const scene = { schemaVersion: 1, facts: { nodes: [{ id: 'a', label: 'original party', attributes: { amount: '9007199254740993.01' }, sources: [], assumption: true }], edges: [] }, presentation: { nodes: [], edges: [] } }
function render(props: Partial<Parameters<typeof CanvasWorkspacePanel>[0]> = {}) {
  return act(async () => root.render(createElement(CanvasWorkspacePanel, { threadId: 'thread-a', workspaceRoot: '/project', activeTab: tab, visible: true, onOpenObject, ...props })))
}
function reply(threadId: string) {
  return { ok: true, document: { threadId, sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), kind: 'canvas', path: tab.path!, revision: 'c'.repeat(64), content: 'e30=' } }
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  request.mockReset(); pickFile.mockReset(); onOpenObject.mockReset(); writeText.mockReset(); revokeObjectURL.mockReset(); createObjectURL.mockReset(); decode.mockReset()
  useNativeReferenceStore.setState({ references: [], drafts: {} })
  request.mockImplementation(async (input: CanvasHostRequest) => {
    if (input.operation === 'open-object' || input.operation === 'read-object') return reply(input.threadId)
    if (input.operation === 'proposals-list') return { ok: true, proposals: [] }
    if (input.operation === 'object-recovery') return { ok: true, recovery: { current: null, pending: null } }
    if (input.operation === 'capture-selection') return { ok: true, selection: { sessionId: input.sessionId, threadId: input.threadId,
      scopeId: 'd'.repeat(48), baseRevision: input.baseRevision, selectedIds: input.selectedIds, editable: true } }
    return { ok: true, closed: true }
  })
  decode.mockResolvedValue({ blob: new Blob(['svg']), scene, layout: { viewBox: [0, 0, 120, 60], components: [{ id: 'a', x: 0, y: 0, width: 120, height: 60 }], connections: [] } })
  createObjectURL.mockReturnValue('blob:synthetic-preview')
  vi.stubGlobal('analytix', { canvas: { request, pickFile } })
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, writable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, writable: true, value: revokeObjectURL })
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  await i18n.changeLanguage('en')
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => { root.unmount(); await canvasPreviewSessions.close() }); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('Canvas object work surface', () => {
  it('reads only through the finite Host, previews as an image and copies exact facts only on explicit action', async () => {
    writeText.mockResolvedValue(undefined)
    await render()
    expect(request.mock.calls[0][0]).toEqual({ operation: 'open-object', threadId: 'thread-a', kind: 'canvas', object: { workspace: '/project', path: '/project/flow.canvas' } })
    expect(container.querySelector('svg image')?.getAttribute('href')).toBe('blob:synthetic-preview')
    expect(container.querySelector('script, iframe, foreignObject')).toBeNull()
    expect(writeText).not.toHaveBeenCalled()
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })))
    expect(container.querySelector('pre')?.textContent).toContain('9007199254740993.01')
    expect(container.textContent).toContain(i18n.t('canvas:canvasAssumption'))
    const copy = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('canvas:canvasCopySelection')}"]`)!
    await act(async () => copy.click())
    expect(writeText).toHaveBeenCalledExactlyOnceWith(JSON.stringify(scene.facts.nodes[0], null, 2))
    expect(request.mock.calls.map(call => call[0].operation)).toEqual(['open-object', 'proposals-list', 'object-recovery'])
  })
  it('ignores IME selection activation and Escape clears a selected object', async () => {
    await render()
    const hit = container.querySelector('svg rect')!
    await act(async () => hit.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true })))
    expect(container.querySelector('pre')).toBeNull()
    await act(async () => hit.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    await act(async () => hit.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(container.querySelector('pre')).toBeNull()
  })
  it('hides and revokes the old preview on collapse and refuses a different project', async () => {
    await render()
    await render({ visible: false })
    expect(container.querySelector('svg image')).toBeNull()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:synthetic-preview')
    expect(request.mock.calls.some(call => call[0].operation === 'close-object')).toBe(false)
    request.mockClear()
    await render({ workspaceRoot: '/other-project' })
    expect(container.textContent).toContain(i18n.t('canvas:canvasWorkspaceMismatch'))
    expect(container.querySelector('svg image')).toBeNull()
    expect(request).not.toHaveBeenCalled()
  })
  it('never paints a stale decoding result after a thread change', async () => {
    let resolve!: (value: unknown) => void
    decode.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    await render()
    await render({ threadId: 'thread-b' })
    const count = createObjectURL.mock.calls.length
    await act(async () => resolve({ blob: new Blob(['late']), scene, layout: { viewBox: [0, 0, 120, 60], components: [], connections: [] } }))
    expect(createObjectURL).toHaveBeenCalledTimes(count)
    expect(request.mock.calls.filter(call => call[0].operation === 'open-object').map(call => call[0].threadId)).toEqual(['thread-a', 'thread-b'])
  })
  it('retains truthful error state on decoding failure and does not expose raw diagnostics', async () => {
    decode.mockRejectedValue(new Error('private-source-content'))
    await render()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(i18n.t('canvas:canvasPreviewFailed'))
    expect(container.textContent).not.toContain('private-source-content')
    expect(createObjectURL).not.toHaveBeenCalled()
  })
  it('ignores completion of a picker opened in an unmounted object view', async () => {
    let resolve!: (value: unknown) => void
    pickFile.mockImplementation(() => new Promise(done => { resolve = done }))
    await render()
    const open = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('canvas:canvasOpen')}"]`)!
    await act(async () => open.click())
    await act(async () => root.render(null))
    await act(async () => resolve({ ok: true, path: '/project/late.canvas' }))
    expect(onOpenObject).not.toHaveBeenCalled()
  })
  it.each(['reload', 'thread roundtrip', 'collapse roundtrip'])('does not acknowledge an old copy after %s, but allows a fresh copy', async transition => {
    let finish!: () => void
    writeText.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    await render()
    const button = (label: string) => container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t(`canvas:${label}`)}"]`)!
    const selectFirst = () => act(async () => container.querySelector('svg rect')!.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    await selectFirst()
    await act(async () => button('canvasCopySelection').click())
    if (transition === 'reload') await act(async () => button('canvasReload').click())
    else {
      await render(transition === 'thread roundtrip' ? { threadId: 'thread-b' } : { visible: false })
      await render()
    }
    await selectFirst()
    await act(async () => finish())
    expect(container.textContent).not.toContain(i18n.t('canvas:canvasCopied'))
    expect(container.textContent).not.toContain(i18n.t('canvas:canvasCopyFailed'))
    writeText.mockResolvedValueOnce(undefined)
    await act(async () => button('canvasCopySelection').click())
    expect(container.textContent).toContain(i18n.t('canvas:canvasCopied'))
    expect(writeText).toHaveBeenCalledTimes(2)
    expect(writeText.mock.calls[1][0]).toBe(JSON.stringify(scene.facts.nodes[0], null, 2))
  })
  it.each([true, false])('shows the newest copy result (success=%s), regardless of completion order', async success => {
    let oldResolve!: () => void, oldReject!: (error: Error) => void
    let newResolve!: () => void, newReject!: (error: Error) => void
    writeText.mockReturnValueOnce(new Promise<void>((resolve, reject) => { oldResolve = resolve; oldReject = reject }))
      .mockReturnValueOnce(new Promise<void>((resolve, reject) => { newResolve = resolve; newReject = reject }))
    await render()
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    const copy = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('canvas:canvasCopySelection')}"]`)!
    await act(async () => copy.click())
    await act(async () => copy.click())
    await act(async () => { if (success) newResolve(); else newReject(new Error('synthetic new failure')) })
    const message = i18n.t(success ? 'canvas:canvasCopied' : 'canvas:canvasCopyFailed')
    expect(container.querySelector('[role="status"]')?.textContent).toBe(message)
    await act(async () => { if (success) oldReject(new Error('synthetic old failure')); else oldResolve() })
    expect(container.querySelector('[role="status"]')?.textContent).toBe(message)
    expect(writeText).toHaveBeenCalledTimes(2)
    expect(request.mock.calls.map(call => call[0].operation)).toEqual(['open-object', 'proposals-list', 'object-recovery'])
  })
  it('clearing and reselecting an object invalidates a pending copy acknowledgement', async () => {
    let finish!: () => void
    writeText.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    await render()
    const hit = container.querySelector('svg rect')!
    await act(async () => hit.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    const copy = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('canvas:canvasCopySelection')}"]`)!
    await act(async () => copy.click())
    await act(async () => hit.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(container.querySelector('pre')).toBeNull()
    await act(async () => hit.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    await act(async () => finish())
    expect(container.querySelector('[role="status"]')).toBeNull()
    expect(container.querySelector('pre')?.textContent).toContain('9007199254740993.01')
  })
  it('does not use a file-picker reply from before a same-file reload', async () => {
    let finish!: (value: unknown) => void
    pickFile.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render()
    const button = (label: string) => container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t(`canvas:${label}`)}"]`)!
    await act(async () => button('canvasOpen').click())
    await act(async () => button('canvasReload').click())
    await act(async () => finish({ ok: true, path: '/project/late.canvas' }))
    expect(onOpenObject).not.toHaveBeenCalled()
    expect(button('canvasOpen').disabled).toBe(false)
    expect(container.querySelector('svg image')).not.toBeNull()
  })
})

// These exercise real renderer consumers, not a screenshot-only surface.
describe('Canvas quotation and suspended presentation', () => {
  it('adds only a bounded Core scope and never sends or overwrites composer drafts', async () => {
    useNativeReferenceStore.getState().setDraft('thread-a/other', { note: 'preserve independent note', dirty: true })
    await render()
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    await act(async () => container.querySelector<HTMLButtonElement>('.canvas-quote-button')!.click())
    const [reference] = useNativeReferenceStore.getState().references
    expect(reference).toMatchObject({ kind: 'canvas', scopeId: 'd'.repeat(48), threadId: 'thread-a', text: '', selectedIds: ['a'] })
    expect(useNativeReferenceStore.getState().drafts['thread-a/other'].note).toBe('preserve independent note')
    expect(request.mock.calls.filter(call => !['open-object', 'proposals-list', 'object-recovery', 'capture-selection'].includes(call[0].operation))).toHaveLength(0)
    expect(request.mock.calls.find(call => call[0].operation === 'capture-selection')?.[0]).toMatchObject({ baseRevision: 'c'.repeat(64), selectedIds: ['a'] })
  })
  it('restores local selection after suspension but rereads Core rather than reusing old bytes', async () => {
    await render()
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    await render({ visible: false }); request.mockClear(); await render()
    expect(request.mock.calls[0][0].operation).toBe('read-object')
    expect(container.querySelector('svg rect')?.getAttribute('aria-pressed')).toBe('true')
    expect(decode).toHaveBeenCalledTimes(2)
  })
  it('ignores a captured selection reply after changing the selected object', async () => {
    await render()
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new MouseEvent('click', { bubbles: true })))
    let resolve!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    await act(async () => container.querySelector<HTMLButtonElement>('.canvas-quote-button')!.click())
    await act(async () => container.querySelector('svg rect')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    await act(async () => resolve({ ok: true, selection: { sessionId: 'a'.repeat(48), threadId: 'thread-a', scopeId: 'd'.repeat(48), baseRevision: 'c'.repeat(64), selectedIds: ['a'], editable: true } }))
    expect(useNativeReferenceStore.getState().references).toHaveLength(0)
  })
  it('keeps a confirmed save receipt truthful when fresh decoding fails and never repeats Apply', async () => {
    const proposed = { proposalId: 'e'.repeat(48), baseRevision: 'c'.repeat(64), candidateDigest: 'f'.repeat(64), kind: 'canvas', status: 'proposed', factsDigest: 'f'.repeat(64), sceneDiff: [
      { kind: 'set-display-label', id: 'a', target: 'node', field: 'displayLabel', factLabel: 'original party', before: 'Before', after: 'After' }
    ] }
    const original = request.getMockImplementation()!
    let saved = false
    request.mockImplementation(async (input: CanvasHostRequest) => {
      if (input.operation === 'proposals-list') return { ok: true, proposals: saved ? [] : [proposed] }
      if (input.operation === 'proposal-apply') {
        saved = true; decode.mockRejectedValueOnce(new Error('private decoder details'))
        return { ok: true, receipt: { operationId: 'native_save_' + 'f'.repeat(64), revision: 'f'.repeat(64), status: 'committed', savedAt: '2026-09-17T00:00:00Z' } }
      }
      if (input.operation === 'read-object' && saved) return { ...reply(input.threadId), document: { ...reply(input.threadId).document, revision: 'f'.repeat(64) } }
      return original(input)
    })
    await render()
    const accept = [...container.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent === i18n.t('canvas:canvasAccept'))!
    await act(async () => accept.click())
    expect(container.querySelector('.canvas-saved-receipt')?.textContent).toContain(i18n.t('canvas:canvasSaved'))
    expect(container.querySelector('.canvas-saved-receipt')?.textContent).toContain(i18n.t('canvas:canvasSavedRefreshFailed'))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(i18n.t('canvas:canvasPreviewFailed'))
    expect(container.textContent).not.toContain('private decoder details')
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(1)
  })

})
