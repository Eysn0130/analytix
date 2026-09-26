// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { CanvasReviewPanel } from './CanvasReviewPanel'
import type { CanvasDocument, CanvasHostRequest, CanvasProposal } from '../../../../packages/runtime/src/contracts/canvas-host'

let root: Root, element: HTMLDivElement
const request = vi.fn(), onCommitted = vi.fn()
const document: CanvasDocument = { threadId: 'thread-review', sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), revision: 'c'.repeat(64), path: '/project/flow.canvas', kind: 'canvas', content: 'e30=' }
const proposed: CanvasProposal = { proposalId: 'd'.repeat(48), baseRevision: document.revision, candidateDigest: 'e'.repeat(64), kind: 'canvas', status: 'proposed', factsDigest: 'f'.repeat(64), sceneDiff: [
  { kind: 'set-display-label', id: 'node-a', target: 'node', field: 'displayLabel', factLabel: 'Protected local original', before: '<script>private original</script>', after: 'Reviewed display' }
] }
const receipt = { operationId: `native_save_${'f'.repeat(64)}`, revision: 'e'.repeat(64), status: 'committed' as const, savedAt: '2026-09-17T00:00:00Z' }
let proposals: CanvasProposal[]
const render = (doc = document, imageSize?: { width: number; height: number }) => act(async () => root.render(createElement(CanvasReviewPanel, { key: doc.sessionId, document: doc, imageSize, onCommitted })))
const button = (key: string) => [...element.querySelectorAll<HTMLButtonElement>('button')].find(b => b.textContent === i18n.t(`canvas:${key}`))!
const click = (key: string) => act(async () => button(key).click())
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  request.mockReset(); onCommitted.mockReset(); proposals = [structuredClone(proposed)]
  request.mockImplementation(async (input: CanvasHostRequest) => {
    if (input.operation === 'proposals-list') return { ok: true, proposals: structuredClone(proposals) }
    if (input.operation === 'object-recovery') return { ok: true, recovery: { current: null, pending: null } }
    if (input.operation === 'proposal-apply' || input.operation === 'proposal-status') return { ok: true, receipt }
    if (input.operation === 'proposal-reject') { proposals = []; return { ok: true, rejected: true } }
    return { ok: false, code: 'unavailable' }
  })
  vi.stubGlobal('analytix', { canvas: { request } })
  element = globalThis.document.createElement('div'); globalThis.document.body.append(element); root = createRoot(element)
  await i18n.changeLanguage('en')
})
afterEach(async () => { await act(async () => root.unmount()); element.remove(); vi.useRealTimers(); vi.unstubAllGlobals() })
describe('Protected-local Canvas review actions', () => {
  it('reads reviews without writing and renders escaped actual before/after differences', async () => {
    await render()
    expect(request.mock.calls.map(c => c[0].operation)).toEqual(['proposals-list', 'object-recovery'])
    expect(element.querySelector('del')?.textContent).toBe('<script>private original</script>')
    expect(element.querySelector('ins')?.textContent).toBe('Reviewed display')
    expect(element.querySelector('script')).toBeNull()
    expect(onCommitted).not.toHaveBeenCalled()
    await click('canvasReject')
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(0)
    expect(element.querySelector('article')).toBeNull()
  })
  it('accepts once and preserves the exact confirmed receipt', async () => {
    await render()
    await click('canvasAccept')
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(1)
    expect(onCommitted).toHaveBeenCalledExactlyOnceWith(receipt)
    expect(button('canvasAccept')).toBeUndefined()
  })
  it('does not double-apply while a request is outstanding or use Apply to query unknown writes', async () => {
    await render()
    let resolve!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const accept = button('canvasAccept')
    await act(async () => { accept.click(); accept.click() })
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(1)
    proposals = [{ ...proposed, status: 'pending' }]
    await act(async () => resolve({ ok: true, receipt: { ...receipt, status: 'unknown', revision: '', savedAt: '' } }))
    expect(onCommitted).not.toHaveBeenCalled()
    expect(button('canvasAccept')).toBeUndefined()
    await click('canvasCheckStatus')
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(1)
    await click('canvasQueryOriginal')
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-status')[0][0]).toEqual({ operation: 'proposal-status', sessionId: document.sessionId, threadId: document.threadId, proposalId: proposed.proposalId })
    expect(onCommitted).toHaveBeenCalledExactlyOnceWith(receipt)
  })
  it('keeps uncertain state after a lost reply and only explicit not-found status can re-enable acceptance', async () => {
    await render(); request.mockRejectedValueOnce(new Error('private transport detail'))
    await click('canvasAccept')
    expect(button('canvasAccept')).toBeUndefined(); expect(element.textContent).not.toContain('private transport detail')
    await click('canvasCheckStatus')
    expect(button('canvasAccept')).toBeUndefined()
    request.mockResolvedValueOnce({ ok: false, code: 'operation_not_found' })
    await click('canvasQueryOriginal')
    expect(button('canvasAccept')?.disabled).toBe(false)
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(1)
  })
  it('ignores a late confirmed write reply after changing presentation owner', async () => {
    await render(); let resolve!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    await click('canvasAccept')
    await render({ ...document, sessionId: 'f'.repeat(48), threadId: 'other-thread' })
    await act(async () => resolve({ ok: true, receipt }))
    expect(onCommitted).not.toHaveBeenCalled()
  })
  it('cannot accept a stale base or apply stale review data following a failed status read', async () => {
    proposals = [{ ...proposed, baseRevision: 'f'.repeat(64) }]
    await render(); expect(button('canvasAccept').disabled).toBe(true)
    proposals = [proposed]; request.mockRejectedValueOnce(new Error('unavailable'))
    await click('canvasCheckStatus'); expect(button('canvasAccept').disabled).toBe(true)
    expect(request.mock.calls.filter(c => c[0].operation === 'proposal-apply')).toHaveLength(0)
  })
  it('does not let an earlier poll overwrite a later accepted operation', async () => {
    vi.useFakeTimers()
    await render()
    let resolve!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    await act(async () => { vi.advanceTimersByTime(2000) })
    await click('canvasAccept')
    expect(onCommitted).toHaveBeenCalledTimes(1)
    await act(async () => resolve({ ok: true, proposals: [proposed] }))
    expect(button('canvasAccept')).toBeUndefined()
    expect(onCommitted).toHaveBeenCalledTimes(1)
  })
  it('only creates a typed PNG proposal; creating it cannot save or call a Provider', async () => {
    proposals = []
    await render({ ...document, kind: 'png', path: '/project/image.png' }, { width: 64, height: 32 })
    request.mockResolvedValueOnce({ ok: true, proposal: { proposalId: proposed.proposalId, baseRevision: document.revision, candidateDigest: 'e'.repeat(64), status: 'proposed', kind: 'png', imageDiff: [{ operation: { kind: 'rotate', degrees: 90 }, before: { width: 64, height: 32 }, after: { width: 32, height: 64 } }] } })
    await click('canvasImageCreateProposal')
    expect(request.mock.calls.find(c => c[0].operation === 'propose-image')?.[0]).toMatchObject({ baseRevision: document.revision, operations: [{ kind: 'rotate', degrees: 90 }] })
    expect(request.mock.calls.some(c => ['proposal-apply', 'resume-change', 'model-selection-propose'].includes(c[0].operation))).toBe(false)
    expect(onCommitted).not.toHaveBeenCalled()
  })
  it.each([
    { operation: 'resume-change', label: 'canvasResume', flag: 'canResume' },
    { operation: 'cancel-change', label: 'canvasCancelPending', flag: 'canCancel' },
    { operation: 'undo-change', label: 'canvasRetryUndo', flag: 'canRetryUndo' }
  ] as const)('requires an explicit click to $operation the exact pending change', async ({ operation, label, flag }) => {
    proposals = []
    const pending = { changeId: 'f'.repeat(64), threadId: document.threadId, proposalId: proposed.proposalId,
      baseRevision: document.revision, revision: '', status: 'unknown', beforeText: 'Before', afterText: 'After',
      saveOperationId: `native_save_${'f'.repeat(64)}`, undoOperationId: `native_undo_${'f'.repeat(64)}`,
      canUndo: false, canCancel: false, canRetryUndo: false, canResume: false, createdAt: '2026-09-17T00:00:00Z', savedAt: '', [flag]: true }
    request.mockImplementation(async (input: CanvasHostRequest) => {
      if (input.operation === 'proposals-list') return { ok: true, proposals: [] }
      if (input.operation === 'object-recovery') return { ok: true, recovery: { current: null, pending } }
      if (input.operation === operation) return operation === 'cancel-change'
        ? { ok: true, recovery: { current: null, pending: null } } : { ok: true, receipt }
      return { ok: false, code: 'unavailable' }
    })
    await render()
    await click('canvasCheckStatus')
    expect(request.mock.calls.every(c => ['proposals-list', 'object-recovery'].includes(c[0].operation))).toBe(true)
    expect(button(label)?.disabled).toBe(false)
    for (const forbidden of ['canvasResume', 'canvasCancelPending', 'canvasRetryUndo'].filter(key => key !== label)) expect(button(forbidden)).toBeUndefined()
    await click(label)
    expect(request.mock.calls.filter(c => c[0].operation === operation).map(c => c[0])).toEqual([
      { operation, sessionId: document.sessionId, threadId: document.threadId, changeId: pending.changeId, baseRevision: document.revision }
    ])
    expect(request.mock.calls.some(c => c[0].operation === 'proposal-apply')).toBe(false)
    if (operation === 'cancel-change') expect(onCommitted).not.toHaveBeenCalled()
    else expect(onCommitted).toHaveBeenCalledExactlyOnceWith(receipt)
  })

})
