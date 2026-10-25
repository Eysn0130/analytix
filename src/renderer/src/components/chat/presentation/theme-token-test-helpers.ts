import type { Root } from 'postcss'

// Test-only source seam: root accents and the finite chat consumer alias chains.
// This does not model native computed style, media queries or a general cascade.
export function themeVariables(element: Element, sheets: readonly Root[]): Map<string, string> {
  const chain: Element[] = []
  for (let node: Element | null = element; node; node = node.parentElement) chain.unshift(node)
  const values = new Map<string, string>()
  for (const node of chain) {
    for (const sheet of sheets) sheet.walkRules(rule => {
      if (rule.parent?.type !== 'root') return
      const declarations = rule.nodes.filter(entry => entry.type === 'decl' && entry.prop.startsWith('--'))
      if (!declarations.length || !rule.selectors.some(selector => node.matches(selector))) return
      for (const declaration of declarations) {
        if (declaration.type === 'decl') values.set(declaration.prop, declaration.value)
      }
    })
    if (node instanceof HTMLElement) for (let index = 0; index < node.style.length; index++) {
      const name = node.style.item(index)
      if (name.startsWith('--')) values.set(name, node.style.getPropertyValue(name))
    }
  }
  return values
}

export function resolveThemeToken(value: string | undefined, values: ReadonlyMap<string, string>, seen = new Set<string>()): string | undefined {
  const reference = value?.trim().match(/^var\(\s*(--[\w-]+)\s*(?:,\s*(.+))?\)$/)
  if (!reference) return value
  const [, name, fallback] = reference
  if (seen.has(name)) return undefined
  const next = new Set(seen).add(name)
  return resolveThemeToken(values.get(name) ?? fallback, values, next)
}

// Independent concrete oracle; never derive expectations from the production palette.
export const primaryPaletteCases = [
  { theme: 'light', accent: undefined, label: 'default blue', fill: '#3659ef', foreground: '#ffffff', hover: '#2949da', pressed: '#203bb8' },
  { theme: 'dark', accent: undefined, label: 'default blue', fill: '#7291ff', foreground: '#101a36', hover: '#86a1ff', pressed: '#6482ed' },
  { theme: 'light', accent: 'orange', label: 'orange', fill: '#ce7919', foreground: '#140b02', hover: '#c16f13', pressed: '#b96810' },
  { theme: 'dark', accent: 'orange', label: 'orange', fill: '#ffb667', foreground: '#2b1a08', hover: '#ffc180', pressed: '#eba153' }
] as const
