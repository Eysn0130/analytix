// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { parse } from 'postcss'
import { afterEach, beforeAll, beforeEach, expect, it, vi } from 'vitest'
import { applyTheme, applyUiFontScale } from '../../../lib/apply-theme'
import { HoverCard } from './HoverCard'
import { Tooltip } from './Tooltip'

let tokens: ReturnType<typeof parse>
let compiledTokens: ReturnType<typeof parse>
const gateConsumers: { css: ReturnType<typeof parse>; primary: string; button: string }[] = []
beforeAll(async () => {
  const fs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
  const { fileURLToPath } = await vi.importActual<{ fileURLToPath: (url: string) => string }>('node:url')
  const readCss = (relative: string): string => {
    const url = new URL(relative, import.meta.url)
    if (url.protocol !== 'file:') throw new Error('Expected local CSS source')
    return fs.readFileSync(fileURLToPath(url.href), 'utf8')
  }
  tokens = parse(readCss('./tokens.css'))
  // Vite/esbuild must use Node's own typed-array realm, rather than jsdom's.
  const { execFileSync } = await vi.importActual<typeof import('node:child_process')>('node:child_process')
  const { dirname, resolve } = await vi.importActual<typeof import('node:path')>('node:path')
  const files = [
    './tokens.css', './chat/QuestionComposer.module.css', './chat/PlanReviewPanel.module.css'
  ].map(relative => fileURLToPath(new URL(relative, import.meta.url).href))
  const [shared, ...consumers] = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', `
    import { readFileSync } from 'node:fs'
    import { preprocessCSS, resolveConfig } from 'vite'
    const config = await resolveConfig({ configFile: false, envFile: false, root: process.cwd() }, 'serve')
    const results = await Promise.all(JSON.parse(process.argv[1]).map(filename =>
      preprocessCSS(readFileSync(filename, 'utf8'), filename, config)))
    console.log(JSON.stringify(results.map(({ code, modules }) => ({ code, modules }))))
  `, JSON.stringify(files)], { cwd: resolve(dirname(files[0]), '../../../../../../'), encoding: 'utf8' })) as
    { code: string; modules?: Record<string, string> }[]
  compiledTokens = parse(shared.code)
  for (const consumer of consumers) {
    if (!consumer.modules?.primary) throw new Error('Expected compiled gate primary CSS Module')
    gateConsumers.push({ css: parse(consumer.code), primary: consumer.modules.primary, button: consumer.modules.button ?? '' })
  }
})
const origins = ['data-analytix-message-timeline-content', 'data-analytix-chat-view', 'data-analytix-gate-composer']
let host: HTMLDivElement, root: Root, previousTheme: string | null, previousStyle: string | null
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe(): void {} disconnect(): void {} })
  vi.useFakeTimers()
  previousTheme = document.documentElement.getAttribute('data-theme')
  previousStyle = document.documentElement.getAttribute('style')
  applyTheme('light'); applyUiFontScale('small')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousTheme === null) document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', previousTheme)
  if (previousStyle === null) document.documentElement.removeAttribute('style')
  else document.documentElement.setAttribute('style', previousStyle)
})

// Follow the actual DOM ancestry and selectors. This exercises the CSS source
// seam without pretending jsdom supplies native custom-property layout.
function variables(element: Element, sheet = tokens): Map<string, string> {
  const chain: Element[] = []
  for (let node: Element | null = element; node; node = node.parentElement) chain.unshift(node)
  const values = new Map<string, string>()
  for (const node of chain) {
    sheet.walkRules(rule => {
      if (rule.selectors.some(selector => node.matches(selector))) {
        rule.walkDecls(declaration => { if (declaration.prop.startsWith('--')) values.set(declaration.prop, declaration.value) })
      }
    })
    if (node instanceof HTMLElement) for (let index = 0; index < node.style.length; index++) {
      const name = node.style.item(index)
      if (name.startsWith('--')) values.set(name, node.style.getPropertyValue(name))
    }
  }
  return values
}

// Resolve these two real consumer declarations against the compiled token
// scope. Hover is an explicit selector state; this is not native computed style.
function gateBackground(button: HTMLButtonElement, sheet: ReturnType<typeof parse>, hover: boolean): string | undefined {
  let background: string | undefined
  sheet.walkRules(rule => {
    if (rule.parent?.type !== 'root') return
    let value: string | undefined
    rule.walkDecls('background', declaration => { value = declaration.value })
    if (value === undefined) return
    // Pseudo-elements do not style the button element's own background.
    if (rule.selectors.some(selector => !selector.includes('::') && (!selector.includes(':hover') || hover) &&
      button.matches(selector.replaceAll(':hover', '')))) background = value
  })
  const alias = background && /^var\((--[\w-]+)\)$/.exec(background)
  return alias ? variables(button, compiledTokens).get(alias[1]) : background
}

it.each([
  { theme: 'light' as const, fill: 'rgb(15, 17, 21)', hover: 'rgb(67, 69, 74)' },
  { theme: 'dark' as const, fill: 'rgb(249, 250, 251)', hover: 'rgb(235, 238, 242)' }
])('compiles both gate primary consumers with inherited $theme fill and hover in their actual token scopes', ({ theme, fill, hover }) => {
  applyTheme(theme)
  for (const origin of [...origins, 'data-analytix-chat-token-scope']) {
    host.setAttribute(origin, 'true')
    for (const consumer of gateConsumers) {
      const card = document.createElement('section'), button = document.createElement('button')
      button.className = `${consumer.button} ${consumer.primary}`
      card.append(button); host.append(card)
      expect(gateBackground(button, consumer.css, false)).toBe(fill)
      expect(gateBackground(button, consumer.css, true)).toBe(hover)
      button.disabled = true
      expect(gateBackground(button, consumer.css, true)).toBe(fill)
      card.remove()
    }
    host.removeAttribute(origin)
  }
  for (const consumer of gateConsumers) {
    const button = document.createElement('button'); button.className = `${consumer.button} ${consumer.primary}`; host.append(button)
    expect(gateBackground(button, consumer.css, false)).toBeUndefined()
    expect(gateBackground(button, consumer.css, true)).toBeUndefined()
    button.remove()
  }
})
async function enter(target: HTMLElement, pointer: boolean): Promise<void> {
  const event = new MouseEvent(pointer ? 'pointerover' : 'mouseover', { bubbles: true })
  if (pointer) Object.defineProperty(event, 'pointerType', { value: 'mouse' })
  await act(async () => { target.dispatchEvent(event); await vi.advanceTimersByTimeAsync(0) })
}
async function open(kind: 'tooltip' | 'hovercard', origin?: string): Promise<HTMLElement> {
  if (origin) host.setAttribute(origin, 'true')
  const anchor = createElement('button', null, 'Anchor')
  await act(async () => root.render(kind === 'tooltip'
    ? createElement(Tooltip, { label: 'Actual tooltip', portal: true, children: anchor })
    : createElement(HoverCard, { anchor, inline: true, openDelayMs: 0, content: createElement('p', null, 'Actual image preview') })))
  await enter(host.querySelector('button')!, kind === 'hovercard')
  const portal = [...document.body.children].find(node => node !== host) as HTMLElement | undefined
  expect(portal).toBeDefined(); expect(portal!.parentElement).toBe(document.body)
  return portal!
}

it.each(origins.flatMap(origin => ['tooltip', 'hovercard'].map(kind => ({ origin, kind: kind as 'tooltip' | 'hovercard' }))))(
  'keeps the $kind body portal in the $origin token scope', async ({ origin, kind }) => {
    const portal = await open(kind, origin), values = variables(portal)
    expect(portal.getAttribute('data-analytix-chat-token-scope')).toBe('true')
    expect(values.get('--dsw-radius-sm')).toBe('8px'); expect(values.get('--dsw-radius-lg')).toBe('16px')
    expect(values.get('--dsw-alias-bg-layer-1')).toBe('rgb(255, 255, 255)')
    expect(values.get('--dsw-font-markdown-base')).toContain('var(--dsh-content-font-size, 14px)')
  })
it.each(['tooltip', 'hovercard'] as const)('keeps ordinary %s portals in their existing root token scope', async kind => {
  const portal = await open(kind), values = variables(portal)
  expect(portal.hasAttribute('data-analytix-chat-token-scope')).toBe(false)
  expect(values.get('--dsw-radius-sm')).toBe('6px'); expect(values.get('--dsw-radius-lg')).toBe('12px')
  expect(values.get('--dsw-font-markdown-base')).toBe('15px/1.7 var(--dsw-font-family)')
})
it('keeps an in-place tooltip in its DOM scope without adding a portal token marker', async () => {
  host.setAttribute(origins[0], 'true')
  await act(async () => root.render(createElement(Tooltip, { label: 'In place', children: createElement('button', null, 'Anchor') })))
  await enter(host.querySelector('button')!, false)
  const tooltip = host.querySelector<HTMLElement>('[role="tooltip"]')!
  expect(tooltip).not.toBeNull()
  expect(tooltip.hasAttribute('data-analytix-chat-token-scope')).toBe(false)
  expect(variables(tooltip).get('--dsw-radius-sm')).toBe('8px')
})
it.each(['tooltip', 'hovercard'] as const)('keeps theme and font changes live while the %s portal remains open', async kind => {
  const portal = await open(kind, origins[0])
  applyTheme('dark'); applyUiFontScale('large')
  const dark = variables(portal)
  expect(portal.isConnected).toBe(true)
  expect(dark.get('--dsw-alias-bg-layer-1')).toBe('rgb(35, 35, 36)')
  expect(dark.get('--dsh-content-font-size')).toBe(`${14 / 0.82}px`)
  expect(dark.get('--dsw-font-markdown-base')).toContain('var(--dsh-content-font-size, 14px)')
  expect(portal.style.getPropertyValue('--dsh-content-font-size')).toBe('')
  expect(portal.hasAttribute('data-theme')).toBe(false)
  applyTheme('light'); applyUiFontScale('small')
  const light = variables(portal)
  expect(light.get('--dsw-alias-bg-layer-1')).toBe('rgb(255, 255, 255)')
  expect(light.get('--dsh-content-font-size')).toBe('14px')
})
it('inherits scope from the main-chat table origin marker', async () => {
  const portal = await open('tooltip', 'data-analytix-main-chat-table')
  expect(portal.getAttribute('data-analytix-chat-token-scope')).toBe('true')
  expect(variables(portal).get('--dsw-radius-sm')).toBe('8px')
})
it('preserves the chat scope through a tooltip nested inside an already portaled hover card', async () => {
  host.setAttribute(origins[0], 'true')
  await act(async () => root.render(createElement(HoverCard, { anchor: createElement('button', null, 'Outer'), openDelayMs: 0,
    content: createElement(Tooltip, { portal: true, label: 'Nested tooltip', children: createElement('button', null, 'Nested') }) })))
  await enter(host.querySelector('button')!, true)
  const outer = [...document.body.children].find(node => node !== host) as HTMLElement
  await enter(outer.querySelector('button')!, false)
  const tooltip = document.querySelector<HTMLElement>('[role="tooltip"]')!
  expect(tooltip.parentElement).toBe(document.body)
  expect(tooltip.getAttribute('data-analytix-chat-token-scope')).toBe('true')
  expect(variables(tooltip).get('--dsw-radius-sm')).toBe('8px')
})
