/** Types for the existing pure renderer; no model, filesystem or Host authority. */
export function renderCanvasScene(input: unknown, options?: { title?: string }): {
  svg: string
  html: string
  scene: unknown
  layout: {
    viewBox: [number, number, number, number]
    components: Array<{ id: string; x: number; y: number; width: number; height: number }>
    connections: Array<{ id: string; from: string; to: string; points: Array<[number, number]> }>
  }
}
export function canvasSceneToPlan(input: unknown): { scene: unknown; plan: unknown }
