/**
 * Design-quality rule registry.
 *
 * Each rule is a pure function over source text that returns raw hits. The
 * facade in `detect.ts` attaches ids/categories/severity, filters by
 * strictness + ignore list, dedupes, and caps. Rules favor precision over
 * recall so findings folded back into the agent remain trustworthy.
 */

import {
  describeColor,
  extractColorLiterals,
  isPurpleOrBlueHue,
  type ColorInfo
} from './color.js'
import type {
  DesignContext,
  DesignFindingSeverity,
  DesignRuleCategory,
  DesignStrictness
} from './types.js'

export type RuleContext = {
  source: string
  lines: readonly string[]
  ext: string
  designContext?: DesignContext
}

export type RuleHit = {
  line: number
  snippet: string
  message?: string
}

export type DesignRule = {
  id: string
  category: DesignRuleCategory
  severity: DesignFindingSeverity
  minStrictness: DesignStrictness
  title: string
  message: string
  run: (ctx: RuleContext) => RuleHit[]
}

const MARKUP_EXTS = new Set(['html', 'htm', 'xhtml', 'svg', 'vue', 'svelte', 'astro'])
const COMPONENT_EXTS = new Set(['jsx', 'tsx', 'vue', 'svelte', 'astro'])
const GENERIC_FONTS = new Set([
  'sans-serif',
  'serif',
  'monospace',
  'system-ui',
  'ui-sans-serif',
  'ui-serif',
  'ui-monospace',
  '-apple-system',
  'blinkmacsystemfont',
  'inherit',
  'cursive',
  'fantasy'
])
const CHROMATIC_TW_FAMILIES =
  'red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose'

function snippetOf(line: string): string {
  const trimmed = line.trim()
  return trimmed.length > 140 ? `${trimmed.slice(0, 137)}...` : trimmed
}

function isWarmLightNeutral(color: ColorInfo): boolean {
  if (color.lightness <= 0.88) return false
  if (color.hue == null || color.hue < 25 || color.hue > 105) return false
  if (color.rgb) {
    const { r, g, b } = color.rgb
    const spread = Math.max(r, g, b) - Math.min(r, g, b)
    return spread > 3 && spread < 60 && r >= g && g >= b
  }
  return color.saturation > 0.01 && color.saturation < 0.22
}

function splitTopLevelCommas(value: string): string[] {
  const parts: string[] = []
  let depth = 0
  let current = ''
  for (const ch of value) {
    if (ch === '(') depth += 1
    else if (ch === ')') depth = Math.max(0, depth - 1)
    if (ch === ',' && depth === 0) {
      parts.push(current)
      current = ''
    } else {
      current += ch
    }
  }
  if (current.trim()) parts.push(current)
  return parts
}

function eachLineMatch(
  ctx: RuleContext,
  regex: RegExp,
  predicate?: (match: RegExpExecArray, line: string) => boolean,
  messageFor?: (match: RegExpExecArray) => string
): RuleHit[] {
  const hits: RuleHit[] = []
  for (let index = 0; index < ctx.lines.length; index += 1) {
    const line = ctx.lines[index]
    regex.lastIndex = 0
    let match: RegExpExecArray | null
    while ((match = regex.exec(line)) !== null) {
      if (!predicate || predicate(match, line)) {
        const hit: RuleHit = { line: index + 1, snippet: snippetOf(line) }
        if (messageFor) hit.message = messageFor(match)
        hits.push(hit)
      }
      if (!regex.global) break
    }
  }
  return hits
}

const purpleBlueGradient: DesignRule = {
  id: 'slop-purple-blue-gradient',
  category: 'slop',
  severity: 'warning',
  minStrictness: 'relaxed',
  title: '紫-蓝渐变',
  message: '紫-蓝渐变是典型 AI 生成痕迹。换成有品牌依据的配色方向，或使用更克制的单色。',
  run: (ctx) => {
    const hits: RuleHit[] = []
    const twGradient = /\b(?:bg-gradient-to-[a-z]{1,2}|bg-linear-to-[a-z]{1,2}|bg-\[(?:linear|radial|conic)-gradient)/
    const twFrom = /\bfrom-(?:violet|purple|fuchsia|indigo|blue)-\d{2,3}\b/
    const twTo = /\bto-(?:violet|purple|fuchsia|indigo|blue|sky|cyan)-\d{2,3}\b/
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      if (twGradient.test(line) && twFrom.test(line) && twTo.test(line)) {
        hits.push({ line: index + 1, snippet: snippetOf(line) })
        continue
      }
      const gradient = /(?:linear|radial|conic)-gradient\(([^;]*?)\)/i.exec(line)
      if (gradient) {
        const chromatic = extractColorLiterals(gradient[1])
          .map(describeColor)
          .filter((color): color is ColorInfo => color != null && color.hue != null)
        if (chromatic.length >= 2 && chromatic.every((color) => isPurpleOrBlueHue(color.hue))) {
          hits.push({ line: index + 1, snippet: snippetOf(line) })
        }
      }
    }
    return hits
  }
}

const bounceEasing: DesignRule = {
  id: 'slop-bounce-elastic-easing',
  category: 'slop',
  severity: 'warning',
  minStrictness: 'relaxed',
  title: '弹跳/橡皮筋缓动',
  message: '弹跳/橡皮筋缓动显得过时。改用更克制的 ease-out 指数曲线。',
  run: (ctx) =>
    eachLineMatch(
      ctx,
      /cubic-bezier\(\s*(-?[\d.]+)\s*,\s*(-?[\d.]+)\s*,\s*(-?[\d.]+)\s*,\s*(-?[\d.]+)\s*\)/gi,
      (match) => {
        const y1 = parseFloat(match[2])
        const y2 = parseFloat(match[4])
        return y1 < -0.02 || y1 > 1.02 || y2 < -0.02 || y2 > 1.02
      }
    )
}

const creamDefaultBg: DesignRule = {
  id: 'slop-cream-default-bg',
  category: 'slop',
  severity: 'warning',
  minStrictness: 'standard',
  title: '米/沙/纸色背景',
  message: '米/沙/纸/象牙色背景是常见 AI 默认底色。用品牌色、纯中性色，或明显属于品牌的中调色。',
  run: (ctx) => {
    const hits: RuleHit[] = []
    const tokenDecl =
      /--(?:paper|cream|sand|bone|linen|parchment|wheat|biscuit|ivory|flour|eggshell|oat|almond|vanilla)\b\s*:\s*([^;{]+)/i
    const bgDecl = /background(?:-color)?\s*:\s*([^;{]+)/i
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      const token = tokenDecl.exec(line)
      if (token) {
        const color = extractColorLiterals(token[1]).map(describeColor).find(Boolean)
        if (color && isWarmLightNeutral(color)) hits.push({ line: index + 1, snippet: snippetOf(line) })
        continue
      }
      const bg = bgDecl.exec(line)
      if (bg) {
        const color = extractColorLiterals(bg[1]).map(describeColor).find(Boolean)
        if (color && isWarmLightNeutral(color)) hits.push({ line: index + 1, snippet: snippetOf(line) })
      }
    }
    return hits
  }
}

const sideTabBorder: DesignRule = {
  id: 'slop-side-tab-border',
  category: 'slop',
  severity: 'warning',
  minStrictness: 'standard',
  title: '侧边强调条',
  message: '单侧彩色粗边框加圆角是典型 AI 侧边强调条。改用整体背景/底色变化或更克制的指示。',
  run: (ctx) => {
    const sideBorder = /\bborder-(?:l|r|t|b|s|e)-(?:2|4|8)\b/
    const rounded = /\brounded(?:-|\b)/
    const colored = new RegExp(`\\bborder-(?:${CHROMATIC_TW_FAMILIES})-\\d{2,3}\\b`)
    return ctx.lines.reduce<RuleHit[]>((acc, line, index) => {
      if (sideBorder.test(line) && rounded.test(line) && colored.test(line)) {
        acc.push({ line: index + 1, snippet: snippetOf(line) })
      }
      return acc
    }, [])
  }
}

const gradientText: DesignRule = {
  id: 'slop-gradient-text',
  category: 'slop',
  severity: 'advisory',
  minStrictness: 'standard',
  title: '渐变文字',
  message: '渐变文字被过度使用且常有可读性/对比问题。仅在真正增益时保留。',
  run: (ctx) =>
    eachLineMatch(ctx, /\bbg-clip-text\b|(?:-webkit-)?background-clip\s*:\s*text\b/gi)
}

const grayTextOnColor: DesignRule = {
  id: 'slop-gray-text-on-color',
  category: 'slop',
  severity: 'advisory',
  minStrictness: 'strict',
  title: '彩色底上的灰字',
  message: '彩色背景上用灰字会发灰发脏。用背景同色系的更深色，或文字色透明度。',
  run: (ctx) => {
    const grayText = /\btext-(?:gray|slate|zinc|neutral|stone)-\d{2,3}\b/
    const colorBg = new RegExp(`\\bbg-(?:${CHROMATIC_TW_FAMILIES})-\\d{2,3}\\b`)
    return ctx.lines.reduce<RuleHit[]>((acc, line, index) => {
      if (grayText.test(line) && colorBg.test(line)) acc.push({ line: index + 1, snippet: snippetOf(line) })
      return acc
    }, [])
  }
}

const darkColoredGlow: DesignRule = {
  id: 'slop-dark-colored-glow',
  category: 'slop',
  severity: 'advisory',
  minStrictness: 'strict',
  title: '彩色辉光',
  message: '彩色 box-shadow 辉光是常见暗色 AI 痕迹。用中性阴影表达层级。',
  run: (ctx) => {
    const hits: RuleHit[] = []
    const decl = /box-shadow\s*:\s*([^;{]+)/gi
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      decl.lastIndex = 0
      let match: RegExpExecArray | null
      while ((match = decl.exec(line)) !== null) {
        for (const layer of splitTopLevelCommas(match[1])) {
          const chromatic = extractColorLiterals(layer).some((literal) => {
            const info = describeColor(literal)
            return info != null && info.saturation > 0.35
          })
          if (!chromatic) continue
          const geometry = layer
            .replace(/#[0-9a-f]{3,8}\b/gi, ' ')
            .replace(/(?:rgba?|hsla?|oklch)\([^)]*\)/gi, ' ')
          const lengths = [...geometry.matchAll(/-?\d+(?:\.\d+)?(?:px|rem|em)?/g)]
            .map((item) => parseFloat(item[0]))
            .filter((n) => !Number.isNaN(n))
          const blur = lengths.length >= 3 ? lengths[2] : 0
          if (blur >= 8) {
            hits.push({ line: index + 1, snippet: snippetOf(line) })
            break
          }
        }
      }
    }
    return hits
  }
}

const overusedFont: DesignRule = {
  id: 'quality-overused-font',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '被滥用的字体',
  message: 'Inter / Arial / Roboto / Helvetica 等是被滥用的默认字体。挑一个有性格、贴合品牌的字族作为主字体。',
  run: (ctx) =>
    eachLineMatch(
      ctx,
      /font-family\s*:\s*['"]?\s*(Inter|Arial|Roboto|Helvetica Neue|Helvetica)\b/gi,
      undefined,
      (match) => `主字体使用了被滥用的「${match[1]}」。挑一个有性格、贴合品牌的字族。`
    )
}

const heroFontCeiling: DesignRule = {
  id: 'quality-hero-font-ceiling',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: 'Display 字号上限',
  message: 'Display 标题 > 6rem（约 96px）像在喊叫而非设计。收回到 6rem 以内。',
  run: (ctx) => {
    const hits: RuleHit[] = []
    const remSize = /font-size\s*:\s*([\d.]+)rem/gi
    const clampBody = /font-size\s*:\s*clamp\(([^)]*)\)/gi
    const twArb = /\btext-\[([\d.]+)rem\]/g
    const tw9xl = /\btext-9xl\b/
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      const over = (regex: RegExp): boolean => {
        regex.lastIndex = 0
        let match: RegExpExecArray | null
        while ((match = regex.exec(line)) !== null) if (parseFloat(match[1]) > 6) return true
        return false
      }
      const clampOver = (): boolean => {
        clampBody.lastIndex = 0
        let match: RegExpExecArray | null
        while ((match = clampBody.exec(line)) !== null) {
          const rems = [...match[1].matchAll(/([\d.]+)rem/g)].map((item) => parseFloat(item[1]))
          if (rems.length > 0 && Math.max(...rems) > 6) return true
        }
        return false
      }
      if (over(remSize) || clampOver() || over(twArb) || tw9xl.test(line)) {
        hits.push({ line: index + 1, snippet: snippetOf(line) })
      }
    }
    return hits
  }
}

const trackingFloor: DesignRule = {
  id: 'quality-display-tracking-floor',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '字距下限',
  message: '字距 < -0.04em 会让字母粘连。紧凑 display 用 -0.02 到 -0.03em 已足够。',
  run: (ctx) => {
    const css = /letter-spacing\s*:\s*(-[\d.]+)em/gi
    const tw = /\btracking-\[(-[\d.]+)em\]/g
    const hits: RuleHit[] = []
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      const tooTight = (regex: RegExp): boolean => {
        regex.lastIndex = 0
        let match: RegExpExecArray | null
        while ((match = regex.exec(line)) !== null) if (parseFloat(match[1]) < -0.04) return true
        return false
      }
      if (tooTight(css) || tooTight(tw)) hits.push({ line: index + 1, snippet: snippetOf(line) })
    }
    return hits
  }
}

const lineLength: DesignRule = {
  id: 'quality-body-line-length',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '正文行宽',
  message: '正文行宽 > 75ch 会降低可读性。把 max-width 控制在 65-75ch。',
  run: (ctx) => {
    const css = /max-width\s*:\s*([\d.]+)ch/gi
    const tw = /\bmax-w-\[([\d.]+)ch\]/g
    const hits: RuleHit[] = []
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      const tooWide = (regex: RegExp): boolean => {
        regex.lastIndex = 0
        let match: RegExpExecArray | null
        while ((match = regex.exec(line)) !== null) if (parseFloat(match[1]) > 75) return true
        return false
      }
      if (tooWide(css) || tooWide(tw)) hits.push({ line: index + 1, snippet: snippetOf(line) })
    }
    return hits
  }
}

const arbitraryZIndex: DesignRule = {
  id: 'quality-arbitrary-z-index',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '魔法 z-index',
  message: '魔法 z-index（999 / 9999）说明缺少层级体系。建立语义化刻度。',
  run: (ctx) =>
    eachLineMatch(
      ctx,
      /z-index\s*:\s*(\d{3,})|(?<![\w-])z-\[(\d{3,})\]/gi,
      (match) => {
        const n = parseInt(match[1] ?? match[2], 10)
        return n >= 999
      }
    )
}

const skippedHeading: DesignRule = {
  id: 'quality-skipped-heading-level',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '标题层级跳级',
  message: '标题层级跳级破坏文档结构与可访问性。逐级递进。',
  run: (ctx) => {
    if (!MARKUP_EXTS.has(ctx.ext) && ctx.ext !== 'jsx' && ctx.ext !== 'tsx') return []
    const hits: RuleHit[] = []
    const regex = /<h([1-6])\b/gi
    let previous = 0
    for (let index = 0; index < ctx.lines.length; index += 1) {
      const line = ctx.lines[index]
      regex.lastIndex = 0
      let match: RegExpExecArray | null
      while ((match = regex.exec(line)) !== null) {
        const level = parseInt(match[1], 10)
        if (previous > 0 && level - previous > 1) {
          hits.push({
            line: index + 1,
            snippet: snippetOf(line),
            message: `标题从 h${previous} 跳到 h${level}，跳过了 h${previous + 1}。逐级递进以保持结构与可访问性。`
          })
        }
        previous = level
      }
    }
    return hits
  }
}

const missingReducedMotion: DesignRule = {
  id: 'quality-missing-reduced-motion',
  category: 'quality',
  severity: 'warning',
  minStrictness: 'standard',
  title: '缺少 reduced-motion',
  message: '存在动画但缺少 `@media (prefers-reduced-motion: reduce)` 兜底。提供淡入或瞬时替代。',
  run: (ctx) => {
    if (/prefers-reduced-motion/i.test(ctx.source)) return []
    const motion = COMPONENT_EXTS.has(ctx.ext)
      ? /@keyframes\b|animation\s*:\s*(?!\s*none\b)|animation-name\s*:\s*(?!\s*none\b)|animation-duration\s*:\s*(?!\s*0s\b)\d/i
      : /@keyframes\b|animation\s*:\s*(?!\s*none\b)|animation-name\s*:\s*(?!\s*none\b)|animation-duration\s*:\s*(?!\s*0s\b)\d|transition\s*:\s*(?!\s*none\b)[^;{]*\d|transition-duration\s*:\s*(?!\s*0s\b)\d|(?:^|[\s"'`])animate-(?!none\b)[a-z]/i
    for (let index = 0; index < ctx.lines.length; index += 1) {
      if (motion.test(ctx.lines[index])) return [{ line: index + 1, snippet: snippetOf(ctx.lines[index]) }]
    }
    return []
  }
}

const fontDrift: DesignRule = {
  id: 'drift-font-not-in-system',
  category: 'drift',
  severity: 'advisory',
  minStrictness: 'strict',
  title: '字体偏离设计语境',
  message: '该字体不在设计语境声明的字族内。',
  run: (ctx) => {
    const allowed = ctx.designContext?.allowedFonts
    if (!allowed || allowed.length === 0) return []
    const allowedLower = new Set(allowed.map((font) => font.trim().toLowerCase()))
    return eachLineMatch(
      ctx,
      /font-family\s*:\s*['"]?\s*([A-Za-z][A-Za-z0-9 _-]+?)['"]?\s*[,;}]/gi,
      (match) => {
        const font = match[1].trim().toLowerCase()
        return !GENERIC_FONTS.has(font) && !allowedLower.has(font)
      },
      (match) => `字体「${match[1].trim()}」不在设计语境允许的字族内（${allowed.join('、')}）。`
    )
  }
}

export const DESIGN_RULES: readonly DesignRule[] = [
  purpleBlueGradient,
  bounceEasing,
  creamDefaultBg,
  sideTabBorder,
  gradientText,
  grayTextOnColor,
  darkColoredGlow,
  overusedFont,
  heroFontCeiling,
  trackingFloor,
  lineLength,
  arbitraryZIndex,
  skippedHeading,
  missingReducedMotion,
  fontDrift
]
