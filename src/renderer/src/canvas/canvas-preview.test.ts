import { describe, expect, it } from 'vitest'
import { decodeCanvasPreview } from './canvas-preview'
import type { CanvasDocument } from '../../../../packages/runtime/src/contracts/canvas-host'
const scene = { schemaVersion: 1, facts: { nodes: [{ id: 'node-a', label: '甲主体', attributes: { amount: '9007199254740993.01' }, sources: [{ sourceId: 'source-a', note: '原始记录' }], assumption: false }], edges: [] },
  presentation: { nodes: [{ id: 'node-a', layout: { x: -50, y: 0, width: 120, height: 60 }, displayLabel: '展示甲', style: { fill: '#eeeeee', stroke: '#222222', strokeWidth: 2, dash: 'solid', shape: 'rectangle' } }], edges: [] } }
const encode = (text: string) => new TextEncoder().encode(text)
async function document(bytes: Uint8Array, kind: 'canvas' | 'png' = 'canvas'): Promise<CanvasDocument> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', Uint8Array.from(bytes)))
  return { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'thread-a', kind, path: `/project/sample.${kind}`,
    revision: Array.from(digest, byte => byte.toString(16).padStart(2, '0')).join(''),
    content: btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join('')) }
}
describe('protected-local Canvas preview decoding', () => {
  it('reuses the existing renderer and retains exact facts, coordinates and selection IDs', async () => {
    const before = structuredClone(scene), preview = await decodeCanvasPreview(await document(encode(JSON.stringify(scene))))
    expect(preview.scene).toEqual(before)
    expect(scene).toEqual(before)
    expect(preview.layout?.components[0]).toMatchObject({ id: 'node-a', x: -50 })
    expect(preview.blob.type).toBe('image/svg+xml')
    expect(await preview.blob.text()).toContain('data-canvas-node-id="node-a"')
    expect(await preview.blob.text()).toContain('9007199254740993.01')
  })
  it('rejects a wrong revision, lossy UTF-8, invalid scenes and noncanonical base64', async () => {
    const valid = await document(encode(JSON.stringify(scene)))
    for (const candidate of [ { ...valid, revision: '0'.repeat(64) }, await document(Uint8Array.of(255)), await document(encode('{}')), { ...valid, content: valid.content + '\n' } ]) {
      await expect(decodeCanvasPreview(candidate)).rejects.toThrow()
    }
  })
  it('preserves adversarial labels as data without injecting active DOM or resource elements', async () => {
    const input = structuredClone(scene)
    const attack = '</metadata><script>fetch("https://example.invalid")</script>'
    input.facts.nodes[0].label = attack; input.presentation.nodes[0].displayLabel = attack
    const result = await decodeCanvasPreview(await document(encode(JSON.stringify(input))))
    expect(result.scene?.facts.nodes[0].label).toBe(attack)
    expect(await result.blob.text()).not.toMatch(/<script\b|<iframe\b|<foreignObject\b/)
  })
  it('renders a verified PNG as an image without reencoding pixels', async () => {
    const png = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII='), character => character.charCodeAt(0))
    const source = await document(png, 'png')
    // Independent fixed fixture identity prevents encoder/decoder agreement from hiding a wrong digest.
    expect(source.revision).toBe('3fd24bcfdb17a6027c5f3ed896d17a020d6ff9d9e4d06362461a9b44b6ac1b52')
    const preview = await decodeCanvasPreview(source)
    expect(preview.scene).toBeUndefined()
    expect(new Uint8Array(await preview.blob.arrayBuffer())).toEqual(png)
    const huge = new Uint8Array(png); new DataView(huge.buffer).setUint32(16, 65536)
    await expect(decodeCanvasPreview(await document(huge, 'png'))).rejects.toThrow()
    await expect(decodeCanvasPreview(await document(encode('not png'), 'png'))).rejects.toThrow()
  })
})
