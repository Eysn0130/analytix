import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { imagePoint, imageRectangle } from './image-region-session'
import { useWriteWorkspaceStore } from './write-workspace-store'
import { createWriteShutdownHandler } from './write-shutdown'
import { useNativeReferenceStore } from '../office/native-reference-store'
import type { ImageAnnotation, ImageObjectSnapshot, ObjectEditingRequest } from '../../../../packages/runtime/src/contracts/object-editing'

const workspace = '/synthetic/images', path = workspace + '/sample.png', thread = 'thread-a'
const image: ImageObjectSnapshot = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), sourceRevision: 'c'.repeat(64),
  threadId: thread, path, mimeType: 'image/png', width: 400, height: 200, dataBase64: 'AAAA' }
const empty = (): ImageAnnotation => ({ objectId: image.objectId, threadId: thread, annotationRevision: '', sourceRevision: '',
  width: 0, height: 0, region: null, note: '', updatedAt: '', current: false })
let annotation: ImageAnnotation, request: ReturnType<typeof vi.fn<(input: ObjectEditingRequest) => Promise<unknown>>>, handler: ReturnType<typeof createWriteShutdownHandler>
const state = () => useWriteWorkspaceStore.getState()
const region = { x: 20, y: 10, width: 40, height: 30 }
beforeEach(() => {
  useWriteWorkspaceStore.setState({ imageRegionEditor: null, shutdownFrozen: false })
  state().resetWorkspace()
  useWriteWorkspaceStore.setState({ workspaceRoot: workspace, activeFilePath: path, activeFileKind: 'image' })
  state().setImageRegionThread(thread)
  annotation = empty()
  request = vi.fn<(input: ObjectEditingRequest) => Promise<unknown>>(async input => {
    if (input.action === 'image-open') return { ok: true, image: { ...image, threadId: input.threadId } }
    if (input.action === 'image-annotation-read') return { ok: true, annotation: { ...annotation, threadId: input.threadId } }
    if (input.action === 'image-annotation-write') {
      if (input.expectedAnnotationRevision !== annotation.annotationRevision) return { ok: false, code: 'conflict', message: 'CAS conflict' }
      annotation = { ...annotation, annotationRevision: 'd'.repeat(64), sourceRevision: input.sourceRevision,
        width: image.width, height: image.height, note: input.note, region: input.region, updatedAt: '2026-09-20T00:00:00Z', current: true }
      return { ok: true, annotation }
    }
    if (input.action === 'image-scope-capture') return { ok: true, scope: { kind: 'image-region', sessionId: image.sessionId,
      scopeId: 'e'.repeat(48), objectId: image.objectId, threadId: thread, sourceRevision: image.sourceRevision,
      annotationRevision: annotation.annotationRevision, width: image.width, height: image.height, region: annotation.region,
      purpose: 'discuss', editable: false, current: true } }
    if (input.action === 'close') return { ok: true, closed: true }
    if (input.action === 'image-scope-revoke') return { ok: true, revoked: true }
    throw new Error('Unexpected test request')
  })
  vi.stubGlobal('window', { analytix: { objects: { request }, files: { read: vi.fn(), listDirectory: vi.fn() } } })
  handler = createWriteShutdownHandler()
})
afterEach(async () => {
  await handler({ requestId: 'image-close', phase: 'cancel' })
  useWriteWorkspaceStore.setState({ imageRegionEditor: null, shutdownFrozen: false })
  state().resetWorkspace(); vi.unstubAllGlobals(); useNativeReferenceStore.setState({ references: [] })
})
async function draft(note = '独立备注') {
  expect(await state().openImageRegion(workspace, path, thread)).toBe(true)
  state().updateImageRegion({ region, note })
}
test('maps zoomed rendered geometry to clamped natural integer pixels', () => {
  const box = { left: 10, top: 20, width: 200, height: 100 }
  expect(imagePoint(25.2, 29.6, box, image)).toEqual({ x: 30, y: 19 })
  expect(imagePoint(999, -1, box, image)).toEqual({ x: 400, y: 0 })
  expect(imagePoint(1, 1, { ...box, width: 0 }, image)).toBeNull()
  expect(imageRectangle({ x: 80, y: 50 }, { x: 20, y: 10 })).toEqual({ x: 20, y: 10, width: 60, height: 40 })
  expect(imageRectangle({ x: 1, y: 1 }, { x: 1, y: 5 })).toBeNull()
})
test('writes CAS notes without image bytes, closes and reopens the persisted region', async () => {
  await draft()
  expect(await state().flushSave(workspace)).toBe(true)
  expect(request).toHaveBeenCalledWith({ action: 'image-annotation-write', sessionId: image.sessionId, threadId: thread,
    sourceRevision: image.sourceRevision, expectedAnnotationRevision: '', region, note: '独立备注' })
  expect(await state().closeImageRegion()).toBe(true)
  expect(await state().openImageRegion(workspace, path, thread)).toBe(true)
  expect(state().imageRegionEditor).toMatchObject({ region, note: '独立备注', dirty: false, status: 'saved' })
})
test('lost ACK reads and confirms the frozen tuple without writing a newer draft', async () => {
  await draft('first')
  const implementation = request.getMockImplementation()!
  request.mockImplementationOnce(async input => { await implementation(input); throw Error('lost ACK') })
  expect(await state().flushSave(workspace)).toBe(false)
  const pending = state().imageRegionEditor!.pending
  state().updateImageRegion({ note: 'newer' })
  expect(await state().flushSave(workspace)).toBe(false)
  expect(state().imageRegionEditor).toMatchObject({ pending: null, note: 'newer', dirty: true, status: 'dirty' })
  expect(request.mock.calls.filter(([r]) => r.action === 'image-annotation-write')).toHaveLength(1)
  expect(pending?.note).toBe('first')
  expect(await state().flushSave(workspace)).toBe(true)
  expect(annotation.note).toBe('newer')
})
test('uncommitted lost ACK retries only the exact payload after reading unchanged CAS revision', async () => {
  await draft()
  request.mockRejectedValueOnce(Error('transport failed before commit'))
  expect(await state().flushSave(workspace)).toBe(false)
  const pending = state().imageRegionEditor!.pending
  expect(await state().flushSave(workspace)).toBe(true)
  const writes = request.mock.calls.filter(([r]) => r.action === 'image-annotation-write').map(([r]) => r)
  expect(writes).toEqual([pending, pending])
})
test('conflict retains the note, blocks navigation and does not capture a reference', async () => {
  await draft()
  annotation = { ...empty(), annotationRevision: 'f'.repeat(64), sourceRevision: image.sourceRevision, width: 400, height: 200,
    region, note: 'other writer', current: true, updatedAt: '2026-09-20T00:00:00Z' }
  expect(await state().openWorkspaceHome(workspace)).toBe(false)
  expect(await state().captureImageRegion()).toBeNull()
  expect(state().imageRegionEditor).toMatchObject({ note: '独立备注', status: 'conflict', dirty: true })
  expect(state().activeFilePath).toBe(path)
  expect(request.mock.calls.some(([r]) => r.action === 'image-scope-capture')).toBe(false)
})
test('file, workspace, rename and delete transitions cannot discard an unsaved note', async () => {
  await draft()
  request.mockImplementation(async () => ({ ok: false, code: 'persistence_failure', message: 'disk full' }))
  await state().openFile(workspace, workspace + '/next.txt')
  await state().initializeWorkspace('/other')
  expect(await state().renameEntry(workspace, path, 'renamed.png')).toBeNull()
  expect(await state().deleteEntry(workspace, path)).toBe(false)
  state().resetWorkspace()
  expect(state().activeFilePath).toBe(path)
  expect(state().imageRegionEditor?.note).toBe('独立备注')
})
test('a thread round trip rejects a late open, then requires a fresh open', async () => {
  let resolve!: (value: unknown) => void
  request.mockImplementationOnce(() => new Promise(r => { resolve = r }))
  const opening = state().openImageRegion(workspace, path, thread)
  state().setImageRegionThread('thread-b'); state().setImageRegionThread(thread)
  resolve({ ok: true, image })
  expect(await opening).toBe(false)
  expect(state().imageRegionEditor?.snapshot).toBeNull()
  expect(await state().openImageRegion(workspace, path, thread)).toBe(true)
})
test('thread changes preserve old drafts and never transfer them to another owner', async () => {
  await draft()
  state().setImageRegionThread('thread-b')
  expect(await state().openImageRegion(workspace, path, 'thread-b')).toBe(false)
  expect(await state().flushSave(workspace)).toBe(false)
  expect(state().imageRegionEditor).toMatchObject({ note: '独立备注', threadId: thread, revoked: true })
  state().setImageRegionThread(thread)
  expect(await state().openImageRegion(workspace, path, thread)).toBe(true)
  expect(await state().flushSave(workspace)).toBe(true)
})
test('source changes do not move stale coordinates and require an explicit new region', async () => {
  await draft(); await state().flushSave(workspace)
  annotation = { ...annotation, current: false }
  expect(await state().closeImageRegion()).toBe(true)
  await state().openImageRegion(workspace, path, thread)
  expect(state().imageRegionEditor).toMatchObject({ stale: true, region: null, note: '独立备注' })
  expect(await state().captureImageRegion()).toBeNull()
  expect(await state().openImageRegion(workspace, path, thread, true)).toBe(true)
  expect(state().imageRegionEditor?.region).toBeNull()
  state().updateImageRegion({ region: { ...region, x: 100 } })
  expect(await state().flushSave(workspace)).toBe(true)
})
test('shutdown freezes new edits and saves the latest note after an in-flight save', async () => {
  await draft('first')
  const implementation = request.getMockImplementation()!
  let finish!: () => void
  request.mockImplementationOnce(input => new Promise(resolve => { finish = () => { void implementation(input).then(resolve) } }))
  const first = state().flushSave(workspace)
  state().updateImageRegion({ note: 'last input' })
  const shutdown = handler({ requestId: 'image-close', phase: 'prepare' })
  state().updateImageRegion({ note: 'forbidden' })
  finish(); expect(await first).toBe(false)
  expect(await shutdown).toEqual({ result: 'ready' })
  expect(annotation.note).toBe('last input')
})
test('composition blocks navigation and shutdown while preserving the draft', async () => {
  await draft()
  state().setImageRegionComposing(true)
  expect(await state().openWorkspaceHome(workspace)).toBe(false)
  expect(await handler({ requestId: 'image-close', phase: 'prepare' })).toEqual({ result: 'blocked', reason: 'composing' })
  expect(request.mock.calls.some(([r]) => r.action === 'image-annotation-write')).toBe(false)
})
test('late capture after a local edit is revoked and never returned', async () => {
  await draft(); await state().flushSave(workspace)
  const implementation = request.getMockImplementation()!
  let finish!: () => void
  request.mockImplementationOnce(input => new Promise(resolve => { finish = () => { void implementation(input).then(resolve) } }))
  const captured = state().captureImageRegion()
  await vi.waitFor(() => expect(finish).toBeTypeOf('function'))
  state().updateImageRegion({ note: 'changed' }); finish()
  expect(await captured).toBeNull()
  expect(request).toHaveBeenCalledWith(expect.objectContaining({ action: 'image-scope-revoke' }))
})
test('late save after a thread ABA cannot acknowledge the old UI epoch; return rechecks the frozen tuple', async () => {
  await draft()
  const implementation = request.getMockImplementation()!
  let finish!: () => void
  request.mockImplementationOnce(input => new Promise(resolve => { finish = () => { void implementation(input).then(resolve) } }))
  const saving = state().flushSave(workspace)
  state().setImageRegionThread('thread-b'); state().setImageRegionThread(thread)
  finish(); expect(await saving).toBe(false)
  expect(state().imageRegionEditor).toMatchObject({ dirty: true, revoked: true, pending: expect.objectContaining({ note: '独立备注' }) })
  await state().openImageRegion(workspace, path, thread)
  expect(await state().flushSave(workspace)).toBe(true)
  expect(request.mock.calls.filter(([r]) => r.action === 'image-annotation-write')).toHaveLength(1)
})
test('returning to a thread preserves the original CAS rather than silently rebasing over another note', async () => {
  await draft('local')
  annotation = { ...empty(), annotationRevision: 'f'.repeat(64), sourceRevision: image.sourceRevision, width: 400, height: 200,
    region, note: 'other writer', current: true, updatedAt: '2026-09-20T00:00:00Z' }
  state().setImageRegionThread('thread-b'); state().setImageRegionThread(thread)
  await state().openImageRegion(workspace, path, thread)
  expect(await state().flushSave(workspace)).toBe(false)
  expect(annotation.note).toBe('other writer')
  expect(state().imageRegionEditor).toMatchObject({ status: 'conflict', note: 'local' })
})
test('expired Core session reopens only the same source and retries with its original CAS', async () => {
  await draft()
  request.mockResolvedValueOnce({ ok: false, code: 'session_invalid', message: 'expired session' })
  expect(await state().flushSave(workspace)).toBe(true)
  const writes = request.mock.calls.filter(([r]) => r.action === 'image-annotation-write').map(([r]) => r)
  expect(writes).toHaveLength(2)
  expect(writes[1]).toEqual(writes[0])
})
test('unknown image save blocks shutdown and retains a retryable draft after thaw', async () => {
  await draft()
  request.mockRejectedValueOnce(Error('lost confirmation'))
  expect(await handler({ requestId: 'image-close', phase: 'prepare' })).toEqual({ result: 'blocked', reason: 'save_unconfirmed' })
  expect(state()).toMatchObject({ shutdownFrozen: false, imageRegionEditor: expect.objectContaining({ note: '独立备注', status: 'unknown' }) })
  expect(await handler({ requestId: 'image-close', phase: 'prepare' })).toEqual({ result: 'ready' })
})
