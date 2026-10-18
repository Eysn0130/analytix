import { describe, expect, it } from 'vitest'
import { isHtmlEmbedSrc, writePrototypeAuthorizePayloadSchema, writePrototypeAuthorizeResultSchema } from './write-prototype'

describe('isHtmlEmbedSrc', () => {
  it('matches local html paths only', () => {
    expect(isHtmlEmbedSrc('../../proto/page.html')).toBe(true)
    expect(isHtmlEmbedSrc('proto/page.htm')).toBe(true)
    expect(isHtmlEmbedSrc('img/photo.png')).toBe(false)
    expect(isHtmlEmbedSrc('https://example.com/page.html')).toBe(false)
    expect(isHtmlEmbedSrc('analytix-pending-infographic://abc')).toBe(false)
    expect(isHtmlEmbedSrc('proto/page.html?x=1')).toBe(false)
    expect(isHtmlEmbedSrc('#anchor.html')).toBe(false)
    expect(isHtmlEmbedSrc(undefined)).toBe(false)
  })
})

describe('desktop prototype authorization contract', () => {
  const protectedPresent = { threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64),
    resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0 }
  const present = { protectedPresent, mode: 'workspace-html' }
  it('accepts only mutually exclusive ordinary paths or sealed selectors', () => {
    expect(writePrototypeAuthorizePayloadSchema.safeParse(present).success).toBe(true)
    expect(writePrototypeAuthorizePayloadSchema.safeParse({ path: 'page.html', workspaceRoot: '/workspace' }).success).toBe(true)
    for (const extra of [{ path: '/forged.html' }, { workspaceRoot: '/forged' }, { body: '<html>' }, { owner: 12 }]) {
      expect(writePrototypeAuthorizePayloadSchema.safeParse({ ...present, ...extra }).success).toBe(false)
    }
    expect(writePrototypeAuthorizePayloadSchema.safeParse({ ...present, mode: 'write-prototype' }).success).toBe(false)
  })
  it('requires an exact ephemeral grant URL and partition', () => {
    const id = 'a'.repeat(48)
    const grant = { ok: true, absolutePath: '/workspace/page.html', grantId: id,
      fileUrl: 'analytix-prototype://' + id + '/page.html', partition: 'analytix-proto-' + id }
    expect(writePrototypeAuthorizeResultSchema.safeParse(grant).success).toBe(true)
    for (const change of [{ fileUrl: 'file:///workspace/page.html' }, { fileUrl: 'analytix-prototype://' + 'b'.repeat(48) + '/page.html' },
      { partition: 'persist:ordinary' }, { body: 'private' }]) {
      expect(writePrototypeAuthorizeResultSchema.safeParse({ ...grant, ...change }).success).toBe(false)
    }
  })
})
