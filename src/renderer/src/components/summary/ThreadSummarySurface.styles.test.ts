import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import postcss from 'postcss'
import tailwindcss, { type Config } from 'tailwindcss'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { ThreadSummaryPanelView } from './ThreadSummaryPanel'

const { readFileSync } = await vi.importActual<{ readFileSync: (path: URL, encoding: 'utf8') => string }>('node:fs')
const { default: config } = await vi.importActual<{ default: Config }>('../../../../../tailwind.config.js')
const shellCss = postcss.parse(readFileSync(new URL('../../styles/base-shell.css', import.meta.url), 'utf8'))

function resolveCardColor(theme: 'light' | 'dark'): string {
  const tokens = new Map<string, string>()
  shellCss.walkRules(rule => {
    if (rule.selector !== ':root' && !(theme === 'dark' && rule.selector === "[data-theme='dark']")) return
    rule.walkDecls(declaration => { if (declaration.prop.startsWith('--')) tokens.set(declaration.prop, declaration.value) })
  })
  let value = 'var(--ds-card-strong)'
  const visited = new Set<string>()
  for (;;) {
    const token = /^var\((--[^)]+)\)$/.exec(value)?.[1]
    if (!token) return value
    if (visited.has(token) || !tokens.has(token)) throw new Error(`Unresolved summary surface token: ${token}`)
    visited.add(token); value = tokens.get(token)!
  }
}

beforeAll(async () => { await i18n.changeLanguage('en') })

describe('thread summary surface background', () => {
  it('compiles an opaque theme surface for the actual card while retaining its border and shadow', async () => {
    const html = renderToStaticMarkup(createElement(ThreadSummaryPanelView, {
      state: { status: 'idle', data: null, error: null }, busyTaskId: null, outputState: { status: 'closed' },
      onCollapse: () => {}, onCloseOutput: () => {}, onOpenThread: () => {}
    }))
    const utilities = await postcss([tailwindcss({ ...config, content: [{ raw: html, extension: 'html' }] })])
      .process('@tailwind utilities;', { from: undefined })
    const backgrounds: string[] = []
    utilities.root.walkDecls('background-color', declaration => { backgrounds.push(declaration.value) })
    expect(backgrounds).toContain('var(--ds-card-strong)')
    expect(resolveCardColor('light')).toMatch(/^#[0-9a-f]{6}$/i)
    expect(resolveCardColor('dark')).toMatch(/^#[0-9a-f]{6}$/i)
    const classes = /class="(ds-thread-summary-card[^"]+)"/.exec(html)?.[1].split(/\s+/) ?? []
    expect(classes).toContain('border')
    expect(classes).toContain('border-ds-border')
    expect(classes).toContain('shadow-md')
    const border: string[] = [], shadow: string[] = []
    utilities.root.walkRules(rule => {
      if (rule.selector === '.border-ds-border') rule.walkDecls('border-color', declaration => { border.push(declaration.value) })
      if (rule.selector === '.shadow-md') rule.walkDecls('box-shadow', declaration => { shadow.push(declaration.value) })
    })
    expect(border).toContain('var(--ds-border)')
    expect(shadow).toHaveLength(1)
  })
})
