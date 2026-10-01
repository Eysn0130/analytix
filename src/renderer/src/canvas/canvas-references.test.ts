import { beforeEach, describe, expect, it, vi } from 'vitest'
import { validateCanvasReferences } from './canvas-references'
import { nativeReferencesPrompt, useNativeReferenceStore, type CanvasNativeReference } from '../office/native-reference-store'
import type { CanvasHostResponse } from '../../../../packages/runtime/src/contracts/canvas-host'

const reference = (): CanvasNativeReference => ({ id: 'local-reference', kind: 'canvas', threadId: 'thread-a', workspace: '/private/project', path: '/private/project/sensitive.canvas',
  objectId: 'a'.repeat(64), revision: 'b'.repeat(64), sessionId: 'c'.repeat(48), scopeId: 'd'.repeat(48), editable: true,
  selectedIds: ['PRIVATE-NODE'], label: 'Sensitive original label', text: '' })
const response = (): CanvasHostResponse => ({ ok: true, selection: { sessionId: 'c'.repeat(48), threadId: 'thread-a', scopeId: 'd'.repeat(48),
  baseRevision: 'b'.repeat(64), selectedIds: ['PRIVATE-NODE'], editable: true } })
beforeEach(() => useNativeReferenceStore.setState({ references: [], drafts: {} }))
describe('Canvas main-conversation references', () => {
  it('sends only a temporary scope handle and fixed instructions, never local labels/paths/facts/ids', () => {
    const r = reference(), prompt = nativeReferencesPrompt([r])
    expect(prompt).toContain(r.scopeId)
    for (const privateValue of [r.path, r.workspace, r.label, r.objectId, r.revision, r.sessionId, ...r.selectedIds]) expect(prompt).not.toContain(privateValue)
    expect(prompt).toContain('native_selection_read')
    expect(prompt).toContain('native_selection_propose')
  })
  it('cannot fall back to discussion text when a scope is missing or revoked', () => {
    expect(() => nativeReferencesPrompt([{ ...reference(), editable: false }])).toThrow()
    expect(() => nativeReferencesPrompt([{ ...reference(), scopeId: undefined }])).toThrow()
  })
  it('validates the exact selection immediately before submission without mutating any frozen input', async () => {
    const r = reference(), before = structuredClone(r), request = vi.fn().mockResolvedValue(response())
    expect(await validateCanvasReferences([r], request, () => true)).toBe(true)
    expect(r).toEqual(before)
    expect(request).toHaveBeenCalledExactlyOnceWith({ operation: 'validate-selection', sessionId: r.sessionId, threadId: r.threadId, scopeId: r.scopeId })
  })
  it.each(['threadId', 'sessionId', 'scopeId', 'baseRevision', 'selectedIds'] as const)('rejects mismatched returned %s', async field => {
    const reply = response(); if (!reply.ok || !('selection' in reply)) throw new Error('fixture')
    const value = field === 'selectedIds' ? ['OTHER'] : field === 'threadId' ? 'another-thread' : 'e'.repeat(field === 'baseRevision' ? 64 : 48)
    const invalid = { ...reply, selection: { ...reply.selection, [field]: value } }
    expect(await validateCanvasReferences([reference()], vi.fn().mockResolvedValue(invalid), () => true)).toBe(false)
  })
  it('keeps quotes and independent notes when asynchronous validation fails or the thread switches', async () => {
    const r = useNativeReferenceStore.getState().add(reference())
    useNativeReferenceStore.getState().setDraft('separate-office', { note: 'unsent draft', dirty: true })
    let finish!: (value: CanvasHostResponse) => void, current = true
    const request = vi.fn(() => new Promise<CanvasHostResponse>(resolve => { finish = resolve }))
    const pending = validateCanvasReferences([r], request, () => current)
    current = false; finish(response())
    expect(await pending).toBe(false)
    expect(useNativeReferenceStore.getState().references).toEqual([r])
    expect(useNativeReferenceStore.getState().drafts['separate-office'].note).toBe('unsent draft')
  })
  it('snapshots selected ids and replaces only the same-object/same-thread Canvas quotation', () => {
    const original = reference(); const first = useNativeReferenceStore.getState().add(original)
    original.selectedIds[0] = 'MUTATED'
    expect(first.kind === 'canvas' && first.selectedIds[0]).toBe('PRIVATE-NODE')
    const another = useNativeReferenceStore.getState().add({ ...reference(), threadId: 'thread-b' })
    const next = useNativeReferenceStore.getState().add(reference())
    expect(useNativeReferenceStore.getState().references).toEqual([another, next])
    useNativeReferenceStore.getState().revokeScopes(next.objectId)
    expect(useNativeReferenceStore.getState().references.every(value => !value.scopeId && !value.editable)).toBe(true)
  })
})
