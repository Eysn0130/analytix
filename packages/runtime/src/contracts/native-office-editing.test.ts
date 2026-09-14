import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { MaxNativeOfficeBytes, nativeOfficeCommitInputSchema, nativeOfficeStatusInputSchema, nativeOfficeSelectionCaptureInputSchema, nativeOfficeProposalRejectInputSchema, nativeOfficeProposalDecisionInputSchema, nativeOfficeSelectionResponseSchema, nativeOfficeProposalSchema, nativeOfficeReplacementSchema } from './native-office-editing'
import { objectEditingRequestSchema, objectEditingResponseSchema } from './object-editing'

const input = () => ({ sessionId: 'a'.repeat(48), operationId: 'save_0001', baseRevision: 'b'.repeat(64),
  content: { encoding: 'base64', kind: 'docx', byteLength: 1, sha256: createHash('sha256').update('a').digest('hex'), data: 'YQ==' } })

describe('protected-local native Office input', () => {
  it('supports three explicit kinds without widening the text contract', () => {
    for (const kind of ['docx', 'xlsx', 'pptx']) {
      const request = input(); request.content.kind = kind
      expect(nativeOfficeCommitInputSchema.safeParse(request).success).toBe(true)
      expect(objectEditingRequestSchema.safeParse({ action: 'commit', ...request }).success).toBe(false)
    }
    expect(nativeOfficeStatusInputSchema.safeParse({ sessionId: input().sessionId, operationId: input().operationId }).success).toBe(true)
    expect(objectEditingResponseSchema.safeParse({ ok: true, receipt: { operationId: 'save_0001', revision: 'c'.repeat(64), status: 'committed', savedAt: '2026-09-14T00:00:00Z' } }).success).toBe(true)
  })
  it('rejects ambiguous encoding, length, digest and authority input', () => {
    for (const delta of [{ data: 'YR==' }, { data: 'YQ==\n' }, { data: 'YQ' }, { data: 'YQ===' }, { data: 'Y===' }, { byteLength: 2 }, { byteLength: 0 }, { byteLength: 1.5 }, { byteLength: MaxNativeOfficeBytes + 1 }, { sha256: 'A'.repeat(64) }, { kind: 'doc' }, { encoding: 'utf-8' }, { path: '/private' }]) {
      const request = input()
      expect(nativeOfficeCommitInputSchema.safeParse({ ...request, content: { ...request.content, ...delta } }).success).toBe(false)
    }
    expect(nativeOfficeCommitInputSchema.safeParse({ ...input(), path: '/private' }).success).toBe(false)
    expect(nativeOfficeStatusInputSchema.safeParse(input()).success).toBe(false)
  })
  it('accepts the exact decoded limit and rejects a byte over', () => {
    for (const size of [MaxNativeOfficeBytes, MaxNativeOfficeBytes + 1]) {
      const request = input()
      const bytes = Buffer.alloc(size, 97)
      request.content = { ...request.content, data: bytes.toString('base64'), byteLength: size }
      expect(nativeOfficeCommitInputSchema.safeParse(request).success).toBe(size === MaxNativeOfficeBytes)
    }
  })
})


describe('native selection proposal transport', () => {
  const capture = () => ({ sessionId: 'a'.repeat(48), threadId: 'thread_main', selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64), text: 'Selected text', editable: true })
  it('keeps captures, decision targets and approved replacement on explicit private shapes', () => {
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse(capture()).success).toBe(true)
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text: '\ud800' }).success).toBe(false)
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), command: '.uno:Shell' }).success).toBe(false)
    const reject = { sessionId: 'a'.repeat(48), scopeId: 'c'.repeat(48), proposalId: 'd'.repeat(48), operationId: 'reject_001' }
    expect(nativeOfficeProposalRejectInputSchema.safeParse(reject).success).toBe(true)
    expect(nativeOfficeProposalDecisionInputSchema.safeParse(reject).success).toBe(false)
    expect(nativeOfficeProposalDecisionInputSchema.safeParse({ ...reject, selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64) }).success).toBe(true)
    const scope = { ...capture(), scopeId: 'c'.repeat(48), parts: [{ kind: 'literal', text: 'Projected text' }] }
    const { text: _privateText, ...projectedScope } = scope
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, scope: projectedScope }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, scope }).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, proposals: [{ proposalId: reject.proposalId, status: 'approved', parts: scope.parts }] }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, proposals: [{ proposalId: reject.proposalId, status: 'applied', parts: scope.parts }] }).success).toBe(false)
  })
  it('uses the native 4096 UTF-16 bound without reducing discussion or generic text editing', () => {
    for (const text of ['x'.repeat(4096), 'é'.repeat(4096), '😀'.repeat(2048)]) {
      const proposal = { proposalId: 'c'.repeat(48), status: 'proposed', parts: [{ kind: 'literal', text }] }
      const replacement = { proposalId: 'c'.repeat(48), operationId: 'accept_001', text, selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64) }
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text }).success).toBe(true)
      expect(nativeOfficeProposalSchema.safeParse(proposal).success).toBe(true)
      expect(nativeOfficeReplacementSchema.safeParse(replacement).success).toBe(true)
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text: text + 'x' }).success).toBe(false)
      expect(nativeOfficeProposalSchema.safeParse({ ...proposal, parts: [...proposal.parts, { kind: 'literal', text: 'x' }] }).success).toBe(false)
      expect(nativeOfficeReplacementSchema.safeParse({ ...replacement, text: text + 'x' }).success).toBe(false)
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), editable: false, text: text + 'x' }).success).toBe(true)
      expect(objectEditingRequestSchema.safeParse({ action: 'commit', sessionId: capture().sessionId, baseRevision: capture().baseRevision, operationId: 'generic_01', content: text + 'x' }).success).toBe(true)
    }
  })
  it('strictly accepts the shared Core revocation acknowledgement', () => {
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: true }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: false }).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: true, text: 'unexpected' }).success).toBe(false)
  })
})
