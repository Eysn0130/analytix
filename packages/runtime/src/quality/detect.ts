import { DESIGN_RULES, type RuleContext } from './rules.js'
import {
  DESIGN_STRICTNESS_LEVELS,
  type DesignFinding,
  type DesignRuleMeta,
  type DesignStrictness,
  type DetectOptions
} from './types.js'

export const FRONTEND_EXTENSIONS: readonly string[] = [
  'html',
  'htm',
  'xhtml',
  'css',
  'scss',
  'sass',
  'less',
  'jsx',
  'tsx',
  'vue',
  'svelte',
  'astro',
  'svg'
]

const FRONTEND_EXT_SET = new Set(FRONTEND_EXTENSIONS)

export function extensionOf(filePath: string | undefined): string {
  if (!filePath) return ''
  const base = filePath.split(/[\\/]/).pop() ?? ''
  const dot = base.lastIndexOf('.')
  return dot > 0 ? base.slice(dot + 1).toLowerCase() : ''
}

export function isFrontendPath(filePath: string | undefined): boolean {
  return FRONTEND_EXT_SET.has(extensionOf(filePath))
}

function strictnessRank(level: DesignStrictness): number {
  const index = DESIGN_STRICTNESS_LEVELS.indexOf(level)
  return index < 0 ? 1 : index
}

export function detectFrontend(source: string, options: DetectOptions = {}): DesignFinding[] {
  if (options.filePath !== undefined && !isFrontendPath(options.filePath)) return []
  if (typeof source !== 'string' || source.length === 0) return []

  const strictness = options.strictness ?? 'standard'
  const want = strictnessRank(strictness)
  const ignore = new Set(options.ignoreRules ?? [])
  const ext = extensionOf(options.filePath)
  const lines = source.split(/\r\n|\r|\n/)
  const ctx: RuleContext = {
    source,
    lines,
    ext,
    ...(options.designContext ? { designContext: options.designContext } : {})
  }

  const findings: DesignFinding[] = []
  for (const rule of DESIGN_RULES) {
    if (ignore.has(rule.id)) continue
    if (strictnessRank(rule.minStrictness) > want) continue
    let hits
    try {
      hits = rule.run(ctx)
    } catch {
      continue
    }
    for (const hit of hits) {
      findings.push({
        ruleId: rule.id,
        category: rule.category,
        severity: rule.severity,
        message: hit.message ?? rule.message,
        line: hit.line,
        snippet: hit.snippet
      })
    }
  }

  const seen = new Set<string>()
  const deduped = findings.filter((finding) => {
    const key = `${finding.ruleId}:${finding.line}:${finding.snippet}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
  deduped.sort((a, b) => a.line - b.line || a.ruleId.localeCompare(b.ruleId))

  const max = options.maxFindings && options.maxFindings > 0 ? options.maxFindings : 12
  return deduped.slice(0, max)
}

export function listDesignRules(): DesignRuleMeta[] {
  return DESIGN_RULES.map((rule) => ({
    id: rule.id,
    category: rule.category,
    severity: rule.severity,
    minStrictness: rule.minStrictness,
    title: rule.title
  }))
}
