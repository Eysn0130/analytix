type Position = { id: string; x: number; y: number; w: number; h: number }

export type PresentationObjectV1 = Position & (
  | { kind: 'text'; text: string; fontSize?: number; bold?: boolean; color?: string; align?: 'left' | 'center' | 'right' }
  | { kind: 'shape'; shape: 'rect' | 'ellipse' | 'line'; fill?: string; lineColor?: string; lineWidth?: number }
  | { kind: 'chart'; chartType: 'bar' | 'line' | 'pie'; title?: string; categories: string[]; series: Array<{ name: string; values: number[] }> }
  | { kind: 'image'; imageId: string }
)

export type PresentationSpecV1 = {
  slides: Array<{ id: string; background?: string; objects: PresentationObjectV1[] }>
}
