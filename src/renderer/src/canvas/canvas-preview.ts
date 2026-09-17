import { canvasSceneSchema, MaxCanvasSceneBytes, MaxCanvasImageDimension, MaxCanvasImagePixels, type CanvasScene } from '../../../../packages/runtime/src/contracts/canvas-editing'
import type { CanvasDocument } from '../../../../packages/runtime/src/contracts/canvas-host'
import { renderCanvasScene } from '../../../../plugins/atlasflow/skills/atlasflow/renderers/architecture/canvas-scene.mjs'

export type CanvasPreview = {
  blob: Blob
  scene?: CanvasScene
  layout?: ReturnType<typeof renderCanvasScene>['layout']
}

/** Decode only Core-returned protected-local bytes. No paths, remote URLs, HTML
 * insertion, network resources, model calls or caller-provided SVG are accepted. */
export async function decodeCanvasPreview(document: CanvasDocument): Promise<CanvasPreview> {
  const maximum = document.kind === 'canvas' ? MaxCanvasSceneBytes : 16 << 20
  if (document.content.length > Math.ceil(maximum / 3) * 4) throw new Error('canvas_preview_invalid')
  const binary = atob(document.content)
  if (!binary.length || binary.length > maximum || btoa(binary) !== document.content) throw new Error('canvas_preview_invalid')
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
  const digest = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), byte => byte.toString(16).padStart(2, '0')).join('')
  if (digest !== document.revision) throw new Error('canvas_preview_invalid')
  if (document.kind === 'png') {
    const signature = [137, 80, 78, 71, 13, 10, 26, 10]
    if (bytes.length < 33 || !signature.every((value, index) => bytes[index] === value) ||
        new DataView(bytes.buffer).getUint32(8) !== 13 || String.fromCharCode(...bytes.slice(12, 16)) !== 'IHDR') throw new Error('canvas_preview_invalid')
    const header = new DataView(bytes.buffer), width = header.getUint32(16), height = header.getUint32(20)
    if (!width || !height || width > MaxCanvasImageDimension || height > MaxCanvasImageDimension || width * height > MaxCanvasImagePixels) throw new Error('canvas_preview_invalid')
    return { blob: new Blob([bytes], { type: 'image/png' }) }
  }
  const scene = canvasSceneSchema.parse(JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)))
  const rendered = renderCanvasScene(scene)
  // SVG is displayed as an image, never parsed into the application's DOM.
  return { blob: new Blob([rendered.svg], { type: 'image/svg+xml' }), scene, layout: rendered.layout }
}
