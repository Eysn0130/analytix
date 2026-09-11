import type { SddDesignContext } from './sdd-draft-store'

/** Suggested tone chips offered in the requirement design-context form. */
export const SDD_DESIGN_TONE_OPTIONS = [
  '编辑风',
  '专业',
  '活泼',
  '极简',
  '大胆',
  '温暖',
  '科技感',
  '严肃'
] as const

const DESIGN_TYPE_LABEL: Record<NonNullable<SddDesignContext['designType']>, string> = {
  brand: 'Brand-led (marketing / landing / portfolio; design is part of the offer)',
  product: 'Product-led (app UI / dashboard / tool; design supports repeated use)'
}

export function formatSddDesignContextLines(ctx: SddDesignContext | undefined): string[] {
  if (!ctx) return []
  const parts: string[] = []
  if (ctx.designType) parts.push(`- Surface: ${DESIGN_TYPE_LABEL[ctx.designType]}`)
  if (ctx.brandColor) {
    parts.push(
      `- Brand color anchor: ${ctx.brandColor}; compose the palette around this instead of falling back to generic purple-blue gradients.`
    )
  }
  if (ctx.tone?.length) parts.push(`- Tone: ${ctx.tone.join('、')}`)
  if (parts.length === 0) return []
  return [
    'Design context (honor it in every visual decision):',
    ...parts,
    '- Avoid generic AI tells: cream/sand default backgrounds, purple-blue gradients, bounce/elastic easing, nested cards, gray text on colored backgrounds. Verify text contrast and provide a prefers-reduced-motion fallback.',
    ''
  ]
}
