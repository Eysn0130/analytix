import { describe, expect, it } from 'vitest'
import { createHash } from 'node:crypto'
import { decodeCanvasPreview } from './canvas-preview'
import type { CanvasDocument } from '../../../../packages/runtime/src/contracts/canvas-host'
const scene = { schemaVersion: 1, facts: { nodes: [{ id: 'node-a', label: '甲主体', attributes: { amount: '9007199254740993.01' }, sources: [{ sourceId: 'source-a', note: '原始记录' }], assumption: false }], edges: [] },
  presentation: { nodes: [{ id: 'node-a', layout: { x: -50, y: 0, width: 120, height: 60 }, displayLabel: '展示甲', style: { fill: '#eeeeee', stroke: '#222222', strokeWidth: 2, dash: 'solid', shape: 'rectangle' } }], edges: [] } }
function document(bytes: Buffer, kind: 'canvas' | 'png' = 'canvas'): CanvasDocument {
  return { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'thread-a', kind, path: `/project/sample.${kind}`,
    revision: createHash('sha256').update(bytes).digest('hex'), content: bytes.toString('base64') }
}
describe('protected-local Canvas preview decoding', () => {
  it('reuses the existing renderer and retains exact facts, coordinates and selection IDs', async () => {
    const before = structuredClone(scene), preview = await decodeCanvasPreview(document(Buffer.from(JSON.stringify(scene))))
    expect(preview.scene).toEqual(before)
    expect(scene).toEqual(before)
    expect(preview.layout?.components[0]).toMatchObject({ id: 'node-a', x: -50 })
    expect(preview.blob.type).toBe('image/svg+xml')
    expect(await preview.blob.text()).toContain('data-canvas-node-id="node-a"')
    expect(await preview.blob.text()).toContain('9007199254740993.01')
  })
  it('rejects a wrong revision, lossy UTF-8, invalid scenes and noncanonical base64', async () => {
    const valid = document(Buffer.from(JSON.stringify(scene)))
    for (const candidate of [ { ...valid, revision: '0'.repeat(64) }, document(Buffer.from([255])), document(Buffer.from('{}')), { ...valid, content: valid.content + '\n' } ]) {
      await expect(decodeCanvasPreview(candidate)).rejects.toThrow()
    }
  })
  it('preserves adversarial labels as data without injecting active DOM or resource elements', async () => {
    const input = structuredClone(scene)
    const attack = '</metadata><script>fetch("https://example.invalid")</script>'
    input.facts.nodes[0].label = attack; input.presentation.nodes[0].displayLabel = attack
    const result = await decodeCanvasPreview(document(Buffer.from(JSON.stringify(input))))
    expect(result.scene?.facts.nodes[0].label).toBe(attack)
    expect(await result.blob.text()).not.toMatch(/<script\b|<iframe\b|<foreignObject\b/)
  })
  it('renders a verified PNG as an image without reencoding pixels', async () => {
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=', 'base64')
    const preview = await decodeCanvasPreview(document(png, 'png'))
    expect(preview.scene).toBeUndefined()
    expect(Buffer.from(await preview.blob.arrayBuffer())).toEqual(png)
    const huge = Buffer.from(png); huge.writeUInt32BE(65536, 16)
    await expect(decodeCanvasPreview(document(huge, 'png'))).rejects.toThrow()
    await expect(decodeCanvasPreview(document(Buffer.from('not png'), 'png'))).rejects.toThrow()
  })
})
