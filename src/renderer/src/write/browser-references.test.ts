import { expect, test, vi } from 'vitest'
import { validateBrowserReferences } from './browser-references'
import { nativeReferencesPrompt, type BrowserNativeReference } from '../office/native-reference-store'
const scope = { scopeId: 'a'.repeat(48), documentId: 'b'.repeat(48), selectionId: 'c'.repeat(48), threadId: 'thread-1', workspace: '/workspace' }
const ref: BrowserNativeReference = { kind: 'browser-selection', id: 'id', threadId: scope.threadId, workspace: scope.workspace, path: '', text: '', label: 'Browser', objectId: scope.documentId, revision: scope.selectionId, scopeId: scope.scopeId, editable: false, browserScope: scope }
test('only opaque scope enters prompt and send requires current Core validation', async () => {
  const request = vi.fn().mockResolvedValue({ ok: true })
  expect(await validateBrowserReferences([ref], request, () => true)).toBe(true)
  expect(request).toHaveBeenCalledWith({ action: 'validate', scope })
  const prompt = nativeReferencesPrompt([ref]); expect(prompt).toContain(scope.scopeId); expect(prompt).not.toContain(scope.documentId); expect(prompt).not.toContain('/workspace')
  expect(await validateBrowserReferences([{ ...ref, threadId: 'other' }], request, () => true)).toBe(false)
  expect(await validateBrowserReferences([ref], vi.fn().mockResolvedValue({ ok: false }), () => true)).toBe(false)
})
test('late Core validation cannot authorize a replaced composer owner', async () => {
  let current = true, finish!: (v: {ok:true}) => void
  const pending = validateBrowserReferences([ref], () => new Promise(resolve => { finish = resolve }), () => current)
  current = false; finish({ ok: true }); expect(await pending).toBe(false)
})
