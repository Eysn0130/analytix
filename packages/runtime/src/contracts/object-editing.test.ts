import { describe, expect, it } from 'vitest'
import { objectEditingRequestSchema, objectEditingResponseSchema, objectExportSnapshotRequestSchema, objectExportSnapshotResponseSchema } from './object-editing'

const token = 'a'.repeat(48)
const revision = 'b'.repeat(64)
const binding = { sessionId: token, scopeId: token, draftVersion: token, threadId: 'thread-1', purpose: 'edit' }
const parts = [{ kind: 'literal', text: 'Hello ' }, { kind: 'protected', protectedRef: `protected_${token}` }]
const requests = [
  { action: 'open', workspace: '/tmp', path: 'document.md' },
  { action: 'commit', sessionId: token, operationId: 'save_0001', baseRevision: revision, content: '' },
  { action: 'status', sessionId: token, operationId: 'save_0001' },
  { action: 'close', sessionId: token },
  { action: 'draft-update', sessionId: token, baseRevision: revision, expectedVersion: '', content: '😀' },
  { action: 'draft-read', sessionId: token },
  { action: 'scope-capture', sessionId: token, draftVersion: token, threadId: 'thread-1', purpose: 'edit', range: { start: 0, end: 2 } },
  { action: 'scope-read', ...binding }, { action: 'scope-revoke', ...binding },
  { action: 'proposal-create', ...binding, operationId: 'propose_001', parts },
  { action: 'proposal-accept', ...binding, operationId: 'accept_001', proposalId: token },
  { action: 'proposal-reject', ...binding, operationId: 'reject_001', proposalId: token }
]

describe('protected-local object editing contract', () => {
  it.each(requests)('requires exact, non-null keys for $action', request => {
    expect(objectEditingRequestSchema.safeParse(request).success).toBe(true)
    for (const key of Object.keys(request)) {
      const copy: Record<string, unknown> = { ...request }
      delete copy[key]
      expect(objectEditingRequestSchema.safeParse(copy).success).toBe(false)
      expect(objectEditingRequestSchema.safeParse({ ...copy, [key]: null }).success).toBe(false)
      expect(objectEditingRequestSchema.safeParse({ ...copy, [key.toUpperCase()]: Reflect.get(request, key) }).success).toBe(false)
    }
    expect(objectEditingRequestSchema.safeParse({ ...request, principal: 'forged' }).success).toBe(false)
  })

  it('matches Core byte, Unicode, UTF16 range and tagged-part bounds', () => {
    const draft = requests[4]
    for (const content of ['\ud800', '\udfff', '😀'.repeat(393_217)]) {
      expect(objectEditingRequestSchema.safeParse({ ...draft, content }).success).toBe(false)
    }
    expect(objectEditingRequestSchema.safeParse({ ...draft, content: '😀'.repeat(393_216) }).success).toBe(true)
    for (const range of [{ start: 0, end: null }, { Start: 0, end: 1 }, { start: 0.5, end: 1 }, { start: 0, end: 1_572_865 }, { start: 1, end: 0 }, { start: 0, end: {} }]) {
      expect(objectEditingRequestSchema.safeParse({ ...requests[6], range }).success).toBe(false)
    }
    const invalidParts = [
      [{ kind: 'literal', text: null }], [{ kind: 'literal', text: 'x', protectedRef: 'x' }],
      [{ kind: 'protected', protectedRef: 'x' }], [{ kind: 'Literal', text: 'x' }], null,
      [{ kind: 'literal', text: '\ud800' }], Array.from({ length: 257 }, () => ({ kind: 'literal', text: 'x' })),
      [{ kind: 'literal', text: 'x'.repeat(32_769) }, { kind: 'literal', text: 'x'.repeat(32_769) }]
    ]
    for (const parts of invalidParts) expect(objectEditingRequestSchema.safeParse({ ...requests[9], parts }).success).toBe(false)
  })

  it('parses the exact private response vocabulary without weakening receipts', () => {
    const draft = { objectId: revision, baseRevision: revision, version: token, content: 'private local text' }
    const scope = { scopeId: token, objectId: revision, threadId: 'thread-1', purpose: 'edit', draftVersion: token, baseRevision: revision, range: { start: 0, end: 0 }, parts: [], current: false }
    const proposal = { proposalId: token, scopeId: token, draftVersion: token, parts, status: 'proposed' }
    const decision = { proposalId: token, status: 'accepted', draftVersion: token }
    for (const payload of [{ draft }, { scope }, { proposal }, { decision }, { revoked: true }]) {
      expect(objectEditingResponseSchema.safeParse({ ok: true, ...payload }).success).toBe(true)
      expect(objectEditingResponseSchema.safeParse({ ok: true, ...payload, content: 'untyped' }).success).toBe(false)
    }
    for (const code of ['draft_stale', 'scope_invalid', 'proposal_invalid', 'projection_unavailable', 'protected_span_invalid']) {
      expect(objectEditingResponseSchema.safeParse({ ok: false, code, message: 'The object operation could not be completed.' }).success).toBe(true)
    }
    expect(objectEditingResponseSchema.safeParse({ ok: false, code: 'raw_cause', message: '' }).success).toBe(false)
    const receipt = { operationId: 'save_0001', revision: '', status: 'unknown', savedAt: '' }
    expect(objectEditingResponseSchema.safeParse({ ok: false, code: 'persistence_failure', message: 'Unknown.', receipt }).success).toBe(true)
    expect(objectEditingResponseSchema.safeParse({ ok: true, receipt: { ...receipt, status: 'committed' } }).success).toBe(false)
    expect(objectEditingResponseSchema.safeParse({ ok: true, receipt: { ...receipt, status: 'committed', revision, savedAt: '2026-09-14T01:00:00Z' } }).success).toBe(true)
  })
})


describe('Main-only export snapshot contract', () => {
  const request = { action: 'export-snapshot', sessionId: token, objectId: revision, threadId: 'thread-1', baseRevision: revision, draftVersion: token }
  it('is not admitted by the generic Renderer object lane and requires exact identity', () => {
    expect(objectExportSnapshotRequestSchema.safeParse(request).success).toBe(true)
    expect(objectEditingRequestSchema.safeParse(request).success).toBe(false)
    for (const key of Object.keys(request)) {
      const incomplete = { ...request } as Record<string, unknown>
      delete incomplete[key]
      expect(objectExportSnapshotRequestSchema.safeParse(incomplete).success).toBe(false)
    }
    for (const extra of ['content', 'workspace', 'path', 'scopeId', 'purpose']) {
      expect(objectExportSnapshotRequestSchema.safeParse({ ...request, [extra]: 'forged' }).success).toBe(false)
    }
  })
  it('bounds protected text and requires complete snapshot identity', () => {
    const { action: _, ...binding } = request
    const snapshot = { ...binding, contentDigest: revision, workspace: '/workspace', path: '/workspace/note.md', content: '草稿' }
    expect(objectExportSnapshotResponseSchema.safeParse({ ok: true, snapshot }).success).toBe(true)
    for (const content of ['\ud800', 'x'.repeat(1_572_865)]) expect(objectExportSnapshotResponseSchema.safeParse({ ok: true, snapshot: { ...snapshot, content } }).success).toBe(false)
    expect(objectEditingResponseSchema.safeParse({ ok: true, snapshot }).success).toBe(false)
  })
})
