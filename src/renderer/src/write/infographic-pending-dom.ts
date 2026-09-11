import i18n from '../i18n'
import { isPendingInfographicActive, pendingInfographicKind } from './infographic-pending'

/**
 * Placeholder shown where a generating infographic will land: a white canvas
 * on which a brush "paints" sketch strokes in a loop. Plain DOM so the same
 * markup serves the TipTap node view, the CodeMirror live-preview widget and
 * the split markdown preview. Styles live in styles/write-editor.css under
 * `.write-infographic-pending`.
 */

const SVG_NS = 'http://www.w3.org/2000/svg'

type SketchStroke = {
  d: string
  width: number
  tone: 'accent' | 'muted' | 'wash'
  /** Suffix of the per-stroke draw keyframes (`wip-draw-<step>`). */
  step: number
}

// Sketched "infographic": title, subtitle, donut chart, axis, bars, copy
// lines, footer band. Draw order matches the brush tour below.
const SKETCH_STROKES: SketchStroke[] = [
  { d: 'M40 56 H236', width: 9, tone: 'accent', step: 1 },
  { d: 'M40 84 H188', width: 5, tone: 'muted', step: 2 },
  { d: 'M90 140 a36 36 0 1 1 -0.1 0', width: 9, tone: 'accent', step: 3 },
  { d: 'M168 132 V216 H268', width: 4, tone: 'muted', step: 4 },
  { d: 'M190 216 V178 M216 216 V156 M242 216 V192', width: 10, tone: 'accent', step: 5 },
  { d: 'M40 260 H260 M40 284 H236', width: 5, tone: 'muted', step: 6 },
  { d: 'M40 308 H260 M40 332 H184', width: 5, tone: 'muted', step: 7 },
  { d: 'M44 366 H256', width: 12, tone: 'wash', step: 8 }
]

// Brush tour visiting every stroke in draw order, ending lifted off-canvas.
const BRUSH_TOUR =
  'M40 56 L236 56 L40 84 L188 84 L90 140 a36 36 0 1 1 -0.1 0 ' +
  'L168 132 L168 216 L268 216 L190 216 L190 178 L216 216 L216 156 L242 216 L242 192 ' +
  'L40 260 L260 260 L40 284 L236 284 L40 308 L260 308 L40 332 L184 332 ' +
  'L44 366 L256 366 L292 396'

function svgElement<K extends keyof SVGElementTagNameMap>(
  tag: K,
  attrs: Record<string, string>
): SVGElementTagNameMap[K] {
  const element = document.createElementNS(SVG_NS, tag)
  for (const [name, value] of Object.entries(attrs)) element.setAttribute(name, value)
  return element
}

function buildSketchSvg(): SVGSVGElement {
  const svg = svgElement('svg', {
    class: 'write-infographic-pending-sketch',
    viewBox: '0 0 300 400',
    'aria-hidden': 'true'
  })

  const strokes = svgElement('g', { class: 'wip-strokes' })
  for (const stroke of SKETCH_STROKES) {
    strokes.appendChild(
      svgElement('path', {
        class: `wip-stroke wip-stroke-${stroke.tone} wip-step-${stroke.step}`,
        d: stroke.d,
        pathLength: '1',
        'stroke-width': String(stroke.width)
      })
    )
  }
  svg.appendChild(strokes)

  const brush = svgElement('g', { class: 'wip-brush' })
  // The tour lives next to the stroke data it follows; CSS only animates
  // offset-distance along it.
  brush.style.offsetPath = `path('${BRUSH_TOUR}')`
  const brushArt = svgElement('g', { class: 'wip-brush-art' })
  // Brush drawn tip-down at the group origin: bristles, ferrule, handle.
  brushArt.appendChild(
    svgElement('path', {
      class: 'wip-brush-bristles',
      d: 'M0 0 C 4 -3 5 -8 7 -13 L 13 -7 C 8 -5 3 -4 0 0 Z'
    })
  )
  brushArt.appendChild(
    svgElement('path', { class: 'wip-brush-ferrule', d: 'M7 -13 L 10 -16 L 16 -10 L 13 -7 Z' })
  )
  brushArt.appendChild(
    svgElement('path', {
      class: 'wip-brush-handle',
      d: 'M11.5 -14.5 L 26 -29 L 31 -24 L 16.5 -9.5 Z'
    })
  )
  brush.appendChild(brushArt)
  svg.appendChild(brush)
  return svg
}

export function createInfographicPendingElement(id: string): HTMLElement {
  const stale = !isPendingInfographicActive(id)
  const root = document.createElement('span')
  root.className = 'write-infographic-pending'
  root.dataset.state = stale ? 'stale' : 'active'
  root.dataset.pendingId = id
  root.contentEditable = 'false'

  const canvas = document.createElement('span')
  canvas.className = 'write-infographic-pending-canvas'
  canvas.appendChild(buildSketchSvg())
  root.appendChild(canvas)

  const label = document.createElement('span')
  label.className = 'write-infographic-pending-label'
  const text = document.createElement('span')
  const kind = pendingInfographicKind(id)
  // Stale tokens keep the shared label: the kind is gone with the registry.
  text.textContent = stale
    ? i18n.t('common:writeInfographicStale')
    : kind === 'design'
      ? i18n.t('common:writeDesignDraftDrawing')
      : kind === 'prototype'
        ? i18n.t('common:writePrototypeBuilding')
        : i18n.t('common:writeInfographicDrawing')
  label.appendChild(text)
  if (!stale) {
    const dots = document.createElement('span')
    dots.className = 'write-infographic-pending-dots'
    for (let index = 0; index < 3; index += 1) {
      const dot = document.createElement('span')
      dot.textContent = '.'
      dots.appendChild(dot)
    }
    label.appendChild(dots)
  }
  root.appendChild(label)
  return root
}
