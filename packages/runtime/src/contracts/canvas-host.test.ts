import { describe, expect, it } from 'vitest'
import { canvasHostRequestSchema, canvasHostResponseSchema } from './canvas-host'
const sessionId = 'a'.repeat(48)
const revision = 'b'.repeat(64)
const binding = { sessionId, threadId: 'main-thread' }

describe('Canvas protected-local Host contract', () => {
  it('uses the existing Host object selector and rejects caller candidate bytes', () => {
    expect(canvasHostRequestSchema.safeParse({ operation: 'open-object', threadId: 'main-thread', object: { workspace: '/workspace', path: 'chart.canvas' }, kind: 'canvas' }).success).toBe(true)
    expect(canvasHostRequestSchema.safeParse({ operation: 'open-object', threadId: 'main-thread', workspace: '/workspace', path: 'chart.canvas', kind: 'canvas' }).success).toBe(false)
    const apply = { operation: 'proposal-apply', ...binding, proposalId: 'c'.repeat(48) }
    expect(canvasHostRequestSchema.safeParse(apply).success).toBe(true)
    for (const extra of ['content', 'path', 'command', 'operations']) expect(canvasHostRequestSchema.safeParse({ ...apply, [extra]: 'injected' }).success).toBe(false)
  })
  it('preserves typed coordinates and reviewed receipt semantics', () => {
    const request = { operation: 'propose-scene', ...binding, baseRevision: revision, selectedIds: ['n1'], operations: [{ kind: 'set-node-layout', id: 'n1', layout: { x: 0, y: -10, width: 120, height: 60 } }] }
    expect(canvasHostRequestSchema.safeParse(request).success).toBe(true)
    expect(canvasHostRequestSchema.safeParse({ ...request, operations: [{ kind: 'set-node-layout', id: 'n1', layout: { y: -10, width: 120, height: 60 } }] }).success).toBe(false)
    const receipt = { operationId: 'native_save_' + revision, revision, status: 'committed', savedAt: '2026-09-15T01:00:00Z' }
    expect(canvasHostResponseSchema.safeParse({ ok: true, receipt }).success).toBe(true)
    expect(canvasHostResponseSchema.safeParse({ ok: true, receipt: { ...receipt, savedAt: '' } }).success).toBe(false)
    expect(canvasHostResponseSchema.safeParse({ ok: false, code: 'unavailable', receipt: { ...receipt, revision: '', status: 'unknown', savedAt: '' } }).success).toBe(true)
  })
  it('admits only bounded, unique actual selections and never a raw model payload', () => {
    const capture = { operation: 'capture-selection', ...binding, baseRevision: revision, selectedIds: ['n1', 'e1'] }
    expect(canvasHostRequestSchema.safeParse(capture).success).toBe(true)
    for (const selectedIds of [[], ['n1', 'n1'], Array.from({ length: 65 }, (_, i) => `n${i}`), ['../other']]) {
      expect(canvasHostRequestSchema.safeParse({ ...capture, selectedIds }).success).toBe(false)
    }
    for (const extra of ['content', 'scopeId', 'principal', 'securityBinding', 'publicationPolicy', 'canvas']) {
      expect(canvasHostRequestSchema.safeParse({ ...capture, [extra]: 'untrusted' }).success).toBe(false)
    }
    for (const operation of ['model-selection-read', 'model-selection-propose']) {
      expect(canvasHostRequestSchema.safeParse({ operation, ...binding, scopeId: 'c'.repeat(48) }).success).toBe(false)
    }
  })
  it('keeps validation and proposal status read-only at the public boundary', () => {
    const validate = { operation: 'validate-selection', ...binding, scopeId: 'c'.repeat(48) }
    const status = { operation: 'proposal-status', ...binding, proposalId: 'd'.repeat(48) }
    expect(canvasHostRequestSchema.safeParse(validate).success).toBe(true)
    expect(canvasHostRequestSchema.safeParse(status).success).toBe(true)
    for (const value of [validate, status]) {
      for (const extra of ['apply', 'operations', 'content', 'changeId', 'path']) {
        expect(canvasHostRequestSchema.safeParse({ ...value, [extra]: true }).success).toBe(false)
      }
    }
    const selection = { ...binding, scopeId: 'c'.repeat(48), baseRevision: revision, selectedIds: ['n1'], editable: true }
    expect(canvasHostResponseSchema.safeParse({ ok: true, selection }).success).toBe(true)
    expect(canvasHostResponseSchema.safeParse({ ok: true, selection: { ...selection, rawText: 'private' } }).success).toBe(false)
    expect(canvasHostResponseSchema.safeParse({ ok: true, selection: { ...selection, editable: false } }).success).toBe(false)
  })
  it('bounds review lists, accepts stale intent, and rejects duplicate proposal owners', () => {
    const proposal = { proposalId: 'c'.repeat(48), baseRevision: revision, candidateDigest: 'd'.repeat(64),
      kind: 'canvas', factsDigest: 'e'.repeat(64), status: 'stale', sceneDiff: [
        { kind: 'set-display-label', id: 'n1', target: 'node', field: 'displayLabel', factLabel: 'Original', before: 'Before', after: 'After' }
      ] }
    expect(canvasHostResponseSchema.safeParse({ ok: true, proposals: [proposal] }).success).toBe(true)
    expect(canvasHostResponseSchema.safeParse({ ok: true, proposals: [proposal, proposal] }).success).toBe(false)
    const tooMany = Array.from({ length: 17 }, (_, index) => ({ ...proposal, proposalId: index.toString(16).padStart(48, '0') }))
    expect(canvasHostResponseSchema.safeParse({ ok: true, proposals: tooMany }).success).toBe(false)
    expect(canvasHostResponseSchema.safeParse({ ok: true, proposals: [proposal], applied: true }).success).toBe(false)
  })

})
