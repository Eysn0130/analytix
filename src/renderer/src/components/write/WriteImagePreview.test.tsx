// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { WriteImagePreview } from './WriteImagePreview'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { useNativeReferenceStore } from '../../office/native-reference-store'
import { useChatStore } from '../../store/chat-store'
import { createWriteShutdownHandler } from '../../write/write-shutdown'
import type { ImageAnnotation, ObjectEditingRequest } from '../../../../../packages/runtime/src/contracts/object-editing'
vi.mock('../../store/chat-store', async () => {
  const { createStore } = await import('zustand/vanilla')
  return { useChatStore: createStore(() => ({ activeThreadId: 'thread-a' })) }
})
vi.mock('react-i18next', async importOriginal => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key: string) => key }) }))
const workspace = '/synthetic/images', path = workspace + '/image.png'
const image = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), sourceRevision: 'c'.repeat(64),
  threadId: 'thread-a', path, mimeType: 'image/png', width: 400, height: 200, dataBase64: 'AAAA' }
let annotation: ImageAnnotation, root: Root, container: HTMLDivElement, request: ReturnType<typeof vi.fn<(input: ObjectEditingRequest) => Promise<unknown>>>
let handler: ReturnType<typeof createWriteShutdownHandler>
const state = () => useWriteWorkspaceStore.getState()
const button = (key: string) => [...container.querySelectorAll('button')].find(item => item.textContent === key)!
const render = async (mimeType = 'image/png') => {
  await act(async () => root.render(createElement(WriteImagePreview, { src: 'data:image/gif;base64,OLDPREVIEW', filePath: path,
    workspaceRoot: workspace, mimeType, size: 3 })))
}
function pointer(type: string, x: number, y: number) {
  const event = new MouseEvent(type, { bubbles: true, clientX: x, clientY: y, button: 0 })
  Object.defineProperty(event, 'pointerId', { value: 1 })
  container.querySelector('img')!.dispatchEvent(event)
}
async function draw() {
  const img = container.querySelector('img')!
  vi.spyOn(img, 'getBoundingClientRect').mockReturnValue(new DOMRect(10, 20, 200, 100))
  await act(async () => { pointer('pointerdown', 20, 30); pointer('pointermove', 50, 60); pointer('pointerup', 50, 60) })
}
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  useWriteWorkspaceStore.setState({ imageRegionEditor: null, shutdownFrozen: false }); state().resetWorkspace()
  useWriteWorkspaceStore.setState({ workspaceRoot: workspace, activeFilePath: path, activeFileKind: 'image' })
  useChatStore.setState({ activeThreadId: 'thread-a' })
  useNativeReferenceStore.setState({ references: [] })
  annotation = { objectId: image.objectId, threadId: 'thread-a', annotationRevision: '', sourceRevision: '',
    width: 0, height: 0, regions: [], updatedAt: '', current: false }
  request = vi.fn<(input: ObjectEditingRequest) => Promise<unknown>>(async input => {
    if (input.action === 'image-open') return { ok: true, image: { ...image, threadId: input.threadId } }
    if (input.action === 'image-annotation-read') return { ok: true, annotation: { ...annotation, threadId: input.threadId } }
    if (input.action === 'image-annotation-write') {
      annotation = { ...annotation, annotationRevision: 'd'.repeat(64), sourceRevision: input.sourceRevision,
        width: 400, height: 200, regions: input.regions, current: true, updatedAt: '2026-09-20T00:00:00Z' }
      return { ok: true, annotation }
    }
    if (input.action === 'image-scope-capture') return { ok: true, scope: { kind: 'image-region', sessionId: image.sessionId,
      scopeId: 'e'.repeat(48), objectId: image.objectId, threadId: 'thread-a', sourceRevision: image.sourceRevision,
      annotationRevision: annotation.annotationRevision, width: 400, height: 200, regionId: input.regionId, region: annotation.regions.find(entry => entry.regionId === input.regionId)!.region,
      current: true, editable: false, purpose: 'discuss' } }
    if (input.action === 'close') return { ok: true, closed: true }
    if (input.action === 'image-scope-revoke') return { ok: true, revoked: true }
    throw Error('unexpected operation')
  })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { objects: { request } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  handler = createWriteShutdownHandler()
})
afterEach(async () => {
  await act(async () => { await handler({ requestId: 'image-close', phase: 'cancel' }); root.unmount() })
  container.remove(); useWriteWorkspaceStore.setState({ imageRegionEditor: null }); state().resetWorkspace()
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
test('renders the Core snapshot and maps a real pointer rectangle through zoomed natural pixels', async () => {
  await render()
  expect(container.querySelector('img')!.getAttribute('src')).toBe('data:image/png;base64,AAAA')
  await draw()
  expect(state().imageRegionEditor?.region).toEqual({ x: 20, y: 20, width: 60, height: 60 })
  expect(container.querySelector('[data-testid="image-region-overlay"]')?.getAttribute('style')).toContain('width: 15%')
  const width = container.querySelector('[aria-label="imageRegionCoordinate_width"]') as HTMLInputElement
  expect(width.value).toBe('60')
})
test('reference only adds a discussion scope without sending or replacing a composer draft', async () => {
  await render(); await draw()
  await act(async () => button('imageRegionReference').click())
  expect(useNativeReferenceStore.getState().references).toEqual([expect.objectContaining({ kind: 'image-region', editable: false,
    scopeId: 'e'.repeat(48), text: '', region: { x: 20, y: 20, width: 60, height: 60 } })])
  expect(request.mock.calls.map(([r]) => r.action)).toEqual(['image-open', 'image-annotation-read', 'image-annotation-write', 'image-scope-capture'])
  expect(container.textContent).toContain('imageRegionDiscussionOnly')
})
test('rotated pointer selection saves original coordinates and turning during a gesture cancels only that gesture', async () => {
  await render()
  const rotate = () => container.querySelector<HTMLButtonElement>('[aria-label="writeImageRotatePreview"]')!
  expect(rotate()).not.toBeNull()
  await act(async () => rotate().click())
  const img = container.querySelector('img')!
  vi.spyOn(img, 'getBoundingClientRect').mockReturnValue(new DOMRect(10, 20, 100, 200))
  await act(async () => { pointer('pointerdown', 80, 60); pointer('pointermove', 50, 120); pointer('pointerup', 50, 120) })
  expect(state().imageRegionEditor?.region).toEqual({ x: 80, y: 60, width: 120, height: 60 })
  await act(async () => { state().updateImageRegion({ note: 'keep note' }); await state().flushSave(workspace) })
  const saved = state().imageRegionEditor!.annotation
  expect(saved?.regions[0].region).toEqual({ x: 80, y: 60, width: 120, height: 60 })
  await act(async () => { pointer('pointerdown', 20, 40); pointer('pointermove', 70, 180) })
  await act(async () => rotate().click())
  expect(state().imageRegionEditor).toMatchObject({ region: saved!.regions[0].region, note: 'keep note', dirty: false })
  expect(container.querySelector('[data-testid="image-preview-plane"]')?.getAttribute('style')).toContain('rotate(180deg)')
  expect(request.mock.calls.filter(([r]) => r.action === 'image-annotation-write')).toHaveLength(1)
})
test('unsupported GIF preserves the existing preview without opening a region session', async () => {
  await render('image/gif')
  expect(container.querySelector('img')!.getAttribute('src')).toBe('data:image/gif;base64,OLDPREVIEW')
  expect(container.querySelector('textarea')).toBeNull(); expect(request).not.toHaveBeenCalled()
})
test('unsupported PNG content preserves preview while region editing stays unavailable', async () => {
  request.mockResolvedValue({ ok: false, code: 'invalid_request', message: 'Animated PNG is unsupported' })
  await render()
  expect(container.querySelector('img')!.getAttribute('src')).toContain('OLDPREVIEW')
  expect((container.querySelector('textarea') as HTMLTextAreaElement).disabled).toBe(true)
  expect(container.textContent).toContain('Animated PNG is unsupported')
})
test('mounted note IME blocks shutdown; freeze then blocks pointers and note edits until cancel', async () => {
  await render(); await draw()
  const note = container.querySelector('textarea')!
  await act(async () => note.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
  await act(async () => expect(await handler({ requestId: 'image-close', phase: 'prepare' })).toEqual({ result: 'blocked', reason: 'composing' }))
  await act(async () => note.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
  await act(async () => expect(await handler({ requestId: 'image-close', phase: 'prepare' })).toEqual({ result: 'ready' }))
  expect(note.readOnly).toBe(true)
  const before = state().imageRegionEditor?.region
  await act(async () => { pointer('pointerdown', 40, 40); pointer('pointerup', 80, 80); state().updateImageRegion({ note: 'late' }) })
  expect(state().imageRegionEditor?.region).toEqual(before); expect(state().imageRegionEditor?.note).not.toBe('late')
  await act(async () => { await handler({ requestId: 'image-close', phase: 'cancel' }) })
  expect(note.readOnly).toBe(false)
})
test('thread ABA cannot resurrect an old scope response and the retained note is not transferred', async () => {
  await render(); await draw()
  await act(async () => state().updateImageRegion({ note: 'retain me' }))
  await act(async () => useChatStore.setState({ activeThreadId: 'thread-b' }))
  expect(state().imageRegionEditor).toMatchObject({ threadId: 'thread-a', note: 'retain me', revoked: true })
  expect(container.textContent).toContain('imageRegionReturnToTask')
  await act(async () => useChatStore.setState({ activeThreadId: 'thread-a' }))
  expect(state().imageRegionEditor).toMatchObject({ threadId: 'thread-a', note: 'retain me', revoked: false })
  expect(request.mock.calls.filter(([r]) => r.action === 'image-open')).toHaveLength(2)
})
test('pointer cancel restores pre-gesture geometry without reverting a concurrent note or confirmed CAS', async () => {
  await render(); await draw()
  const original = state().imageRegionEditor!.region
  await act(async () => { await state().flushSave(workspace) })
  await act(async () => { pointer('pointerdown', 40, 40); pointer('pointermove', 90, 90) })
  const moved = state().imageRegionEditor!.region
  expect(moved).not.toEqual(original)
  await act(async () => { await state().flushSave(workspace); state().updateImageRegion({ note: 'retain new note' }) })
  const confirmed = state().imageRegionEditor!.annotation
  await act(async () => pointer('pointercancel', 90, 90))
  expect(state().imageRegionEditor).toMatchObject({ region: original, note: 'retain new note', annotation: confirmed, dirty: true })
  expect(confirmed?.regions[0].region).toEqual(moved)
})
test('lost pointer capture cancels a new rectangle without creating a no-op dirty annotation', async () => {
  await render()
  vi.spyOn(container.querySelector('img')!, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 200, 100))
  await act(async () => { pointer('pointerdown', 10, 10); pointer('pointermove', 60, 60); pointer('lostpointercapture', 60, 60) })
  expect(state().imageRegionEditor).toMatchObject({ region: null, note: '', dirty: false, status: 'saved' })
  expect(request.mock.calls.some(([r]) => r.action === 'image-annotation-write')).toBe(false)
})
test('thread navigation holds the real note input; a queued composition cancels navigation and keeps its text', async () => {
  const { withImageThreadNavigation, isImageThreadNavigationFrozen } = await import('../../write/image-thread-navigation')
  await render(); await draw()
  const note = container.querySelector('textarea')!
  let finish!: () => void
  let switched = false
  let navigation!: Promise<void>
  await act(async () => {
    navigation = withImageThreadNavigation(undefined, async current => {
      await new Promise<void>(resolve => { finish = resolve })
      if (current()) switched = true
    })
  })
  expect(note.readOnly).toBe(true)
  await act(async () => note.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
  expect(note.readOnly).toBe(false)
  expect(isImageThreadNavigationFrozen()).toBe(false)
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(note, '组合中的原稿')
    note.dispatchEvent(new Event('input', { bubbles: true }))
    note.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }))
    finish(); await navigation
  })
  expect(switched).toBe(false)
  expect(state().imageRegionEditor?.note).toBe('组合中的原稿')
})

test('mounted multi-region controls preserve each note, overlays and stable identity when switching and removing', async () => {
  await render(); await draw()
  await act(async () => state().updateImageRegion({ note: 'first note' }))
  const firstId = state().imageRegionEditor!.selectedRegionId!
  await act(async () => button('imageRegionAdd').click())
  expect(state().imageRegionEditor?.selectedRegionId).toBeNull()
  await draw()
  await act(async () => state().updateImageRegion({ region: { x: 200, y: 50, width: 50, height: 50 }, note: 'second note' }))
  const secondId = state().imageRegionEditor!.selectedRegionId!
  expect(secondId).not.toBe(firstId)
  expect(container.querySelectorAll('[data-testid="image-region-overlay"]')).toHaveLength(1)
  expect(container.querySelectorAll('[data-testid="image-region-inactive-overlay"]')).toHaveLength(1)
  const select = container.querySelector<HTMLSelectElement>('[aria-label="imageRegionSelection"]')!
  await act(async () => { select.value = firstId; select.dispatchEvent(new Event('change', { bubbles: true })) })
  expect(container.querySelector('textarea')!.value).toBe('first note')
  await act(async () => button('imageRegionSave').click())
  expect(annotation.regions).toHaveLength(2)
  await act(async () => button('imageRegionRemove').click())
  expect(state().imageRegionEditor).toMatchObject({ selectedRegionId: secondId, note: 'second note', dirty: true })
  await act(async () => button('imageRegionSave').click())
  expect(annotation.regions).toEqual([expect.objectContaining({ regionId: secondId, note: 'second note' })])
})
