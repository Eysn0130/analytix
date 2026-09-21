import { beforeEach, describe, expect, it, vi } from 'vitest'
import { validateImageReferences } from './image-references'
import { nativeReferencesPrompt, useNativeReferenceStore, type ImageRegionNativeReference } from '../office/native-reference-store'
import type { ObjectEditingResponse } from '../../../../packages/runtime/src/contracts/object-editing'

const reference = (): ImageRegionNativeReference => ({ kind: 'image-region', id: 'local', threadId: 'thread-a',
  workspace: '/private/workspace', path: '/private/workspace/private.png', label: 'Private image', text: '',
  objectId: 'a'.repeat(64), revision: 'b'.repeat(64), sessionId: 'c'.repeat(48), scopeId: 'd'.repeat(48),
  annotationRevision: 'e'.repeat(64), regionId: '1'.repeat(48), width: 640, height: 480, region: { x: 10, y: 20, width: 80, height: 60 }, editable: false })
const response = (): ObjectEditingResponse => {
  const r = reference()
  return { ok: true, scope: { kind: 'image-region', sessionId: r.sessionId, scopeId: r.scopeId!, objectId: r.objectId,
    threadId: r.threadId, sourceRevision: r.revision, annotationRevision: r.annotationRevision, width: r.width, height: r.height,
    regionId: r.regionId, region: r.region, purpose: 'discuss', editable: false, current: true } }
}
beforeEach(() => useNativeReferenceStore.setState({ references: [], drafts: {} }))
describe('image region references', () => {
  it('sends only the Core scope handle and does not claim image observation or editing', () => {
    const r = reference(), prompt = nativeReferencesPrompt([r])
    expect(prompt).toContain(r.scopeId)
    expect(prompt).toContain('native_selection_read')
    expect(prompt).toContain('No image pixels have been supplied')
    expect(prompt).not.toContain('native_selection_propose')
    for (const value of [r.path, r.workspace, r.label, r.objectId, r.revision, r.sessionId, r.annotationRevision, r.regionId]) expect(prompt).not.toContain(value)
    expect(() => nativeReferencesPrompt([{ ...r, scopeId: undefined }])).toThrow()
  })
  it('validates the frozen region immediately before sending without replacing the draft', async () => {
    const r = useNativeReferenceStore.getState().add(reference()), request = vi.fn().mockResolvedValue(response())
    useNativeReferenceStore.getState().setDraft('independent', { note: 'unsent note', dirty: true })
    if (r.kind !== 'image-region') throw new Error('fixture')
    expect(await validateImageReferences([r], request, () => true)).toBe(true)
    expect(request).toHaveBeenCalledExactlyOnceWith({ action: 'image-scope-read', sessionId: r.sessionId, threadId: r.threadId, scopeId: r.scopeId })
    expect(useNativeReferenceStore.getState().drafts.independent.note).toBe('unsent note')
  })
  it.each(['sessionId', 'scopeId', 'objectId', 'threadId', 'sourceRevision', 'annotationRevision', 'regionId', 'region', 'width', 'height'])('rejects stale %s', async field => {
    const value = response()
    if (!value.ok || !('scope' in value)) throw new Error('fixture')
    const changed = field === 'region' ? { x: 11, y: 20, width: 80, height: 60 } : field === 'width' ? 641 : field === 'height' ? 481
      : field === 'threadId' ? 'another-thread' : 'f'.repeat(['sessionId', 'scopeId', 'regionId'].includes(field) ? 48 : 64)
    expect(await validateImageReferences([reference()], vi.fn().mockResolvedValue({ ...value, scope: { ...value.scope, [field]: changed } }), () => true)).toBe(false)
  })
  it('drops a late validation while retaining its unsent quotation', async () => {
    const r = useNativeReferenceStore.getState().add(reference())
    let finish!: (value: ObjectEditingResponse) => void, current = true
    const pending = validateImageReferences([r], () => new Promise(resolve => { finish = resolve }), () => current)
    current = false
    finish(response())
    expect(await pending).toBe(false)
    expect(useNativeReferenceStore.getState().references).toEqual([r])
  })
  it('requires every selected region to retain its own Core identity before sending', async () => {
    const first = reference(), second = { ...reference(), regionId: '2'.repeat(48), scopeId: '3'.repeat(48) }
    const firstResponse = response()
    if (!firstResponse.ok || !('scope' in firstResponse)) throw new Error('fixture')
    const secondResponse = { ...firstResponse, scope: { ...firstResponse.scope, regionId: second.regionId, scopeId: second.scopeId } }
    const request = vi.fn().mockResolvedValueOnce(firstResponse).mockResolvedValueOnce(secondResponse)
    expect(await validateImageReferences([first, second], request, () => true)).toBe(true)
    expect(request).toHaveBeenCalledTimes(2)
    const swapped = vi.fn().mockResolvedValueOnce(firstResponse).mockResolvedValueOnce({ ...secondResponse, scope: { ...secondResponse.scope, regionId: first.regionId } })
    expect(await validateImageReferences([first, second], swapped, () => true)).toBe(false)
  })
  it('freezes geometry, replaces only its own quotation and cannot revive a revoked scope', () => {
    const original = reference(), first = useNativeReferenceStore.getState().add(original)
    original.region.x = 90
    expect(first.kind === 'image-region' && first.region.x).toBe(10)
    const other = useNativeReferenceStore.getState().add({ ...reference(), threadId: 'thread-b' })
    const latest = useNativeReferenceStore.getState().add(reference())
    expect(useNativeReferenceStore.getState().references).toEqual([other, latest])
    useNativeReferenceStore.getState().revokeScopes(latest.objectId)
    expect(() => nativeReferencesPrompt(useNativeReferenceStore.getState().references)).toThrow()
  })
})
