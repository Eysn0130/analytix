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
})
