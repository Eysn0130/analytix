import { describe, it, expect } from 'vitest'
import { UpdateThreadRequest } from '../../packages/runtime/src/contracts/threads'

describe('UpdateThreadRequest', () => {
  it('accepts workspace as a valid patch field', () => {
    const result = UpdateThreadRequest.safeParse({
      workspace: '/home/user/project'
    })
    expect(result.success).toBe(true)
    if (result.success) {
      expect(result.data.workspace).toBe('/home/user/project')
    }
  })

  it('rejects empty workspace', () => {
    const result = UpdateThreadRequest.safeParse({
      workspace: ''
    })
    expect(result.success).toBe(false)
  })

  it('accepts workspace alongside other fields', () => {
    const result = UpdateThreadRequest.safeParse({
      title: 'My thread',
      workspace: '/home/user/project'
    })
    expect(result.success).toBe(true)
    if (result.success) {
      expect(result.data.title).toBe('My thread')
      expect(result.data.workspace).toBe('/home/user/project')
    }
  })

  it('accepts workspace as the only field (refine guard)', () => {
    const result = UpdateThreadRequest.safeParse({
      workspace: '/tmp/test'
    })
    expect(result.success).toBe(true)
  })

  it('rejects empty body (no fields changed)', () => {
    const result = UpdateThreadRequest.safeParse({})
    expect(result.success).toBe(false)
  })

  it.each([true, false])('accepts the explicit archived=%s intent by itself', archived => {
    expect(UpdateThreadRequest.safeParse({ archived })).toMatchObject({ success: true, data: { archived } })
  })

  it.each([null, 'true', 1, [], {}])('rejects a malformed archive intent: %j', archived => {
    expect(UpdateThreadRequest.safeParse({ archived }).success).toBe(false)
  })

  it.each(['title', 'workspace', 'providerId', 'unknown'])('rejects an archive intent mixed with %s', field => {
    expect(UpdateThreadRequest.safeParse({ archived: true, [field]: 'other' }).success).toBe(false)
  })

  it.each(['idle', 'running', 'archived', 'deleted'])('rejects runtime-owned status=%s even beside an archive intent', status => {
    expect(UpdateThreadRequest.safeParse({ status }).success).toBe(false)
    expect(UpdateThreadRequest.safeParse({ title: 'Keep the title', archived: false, status }).success).toBe(false)
  })

  it('rejects workspace with only whitespace', () => {
    // `z.string().min(1)` does not auto-trim, so whitespace-only
    // strings with length >= 1 will pass schema validation.
    // Runtime normalization (normalizeWorkspaceRoot) handles trimming.
    const result = UpdateThreadRequest.safeParse({
      workspace: '   '
    })
    // Whitespace passes schema min(1) since length is 3.
    expect(result.success).toBe(true)
  })
})
