export type DesignFindingSeverity = 'warning' | 'advisory'

export type DesignRuleCategory = 'slop' | 'quality' | 'drift'

export type DesignStrictness = 'relaxed' | 'standard' | 'strict'

export const DESIGN_STRICTNESS_LEVELS: readonly DesignStrictness[] = [
  'relaxed',
  'standard',
  'strict'
]

export type DesignFinding = {
  ruleId: string
  category: DesignRuleCategory
  severity: DesignFindingSeverity
  message: string
  line: number
  snippet: string
}

export type DesignContext = {
  designType?: 'brand' | 'product'
  brandColor?: string
  tone?: readonly string[]
  allowedFonts?: readonly string[]
}

export type DetectOptions = {
  filePath?: string
  strictness?: DesignStrictness
  ignoreRules?: readonly string[]
  designContext?: DesignContext
  maxFindings?: number
}

export type DesignRuleMeta = {
  id: string
  category: DesignRuleCategory
  severity: DesignFindingSeverity
  minStrictness: DesignStrictness
  title: string
}
