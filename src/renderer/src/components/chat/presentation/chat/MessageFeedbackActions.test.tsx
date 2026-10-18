// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { parse } from 'postcss'
import i18n from '../../../../i18n'
import { rendererRuntimeClient } from '../../../../agent/runtime-client'
import { MessageFeedbackActions } from './MessageFeedbackActions'

let root: Root, container: HTMLDivElement, previousTheme: string | null
beforeEach(async () => {
  previousTheme = document.documentElement.getAttribute('data-theme')
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  await i18n.changeLanguage('en')
  vi.spyOn(rendererRuntimeClient, 'onRuntimeStatus').mockReturnValue(() => undefined)
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousTheme === null) document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', previousTheme)
})
const click = async (button: Element) => { await act(async () => button.dispatchEvent(new MouseEvent('click', { bubbles: true }))) }

it.each(['light', 'dark'])('keeps the body portal in the real %s chat token scope with defined fill and radius', async theme => {
  document.documentElement.setAttribute('data-theme', theme)
  vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockResolvedValue({ ok: true, status: 200, body: JSON.stringify({ items: [] }) })
  await act(async () => root.render(createElement(MessageFeedbackActions, { threadId: `ui-theme-${theme}`, messageId: 'message' })))
  await click(container.querySelector('button[aria-label="Good response"]')!)
  const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
  const backdrop = dialog.parentElement!
  expect(backdrop.parentElement).toBe(document.body)
  expect(backdrop.hasAttribute('data-analytix-message-feedback-overlay')).toBe(true)
  const fs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
  const readCss = (relative: string) => {
    const url = new URL(relative, import.meta.url); expect(url.protocol).toBe('file:')
    return fs.readFileSync(decodeURIComponent(url.pathname), 'utf8')
  }
  const variables = new Map<string, string>()
  parse(readCss('../tokens.css')).walkRules(rule => {
    if (rule.selectors.some(selector => backdrop.matches(selector))) {
      rule.walkDecls(declaration => { if (declaration.prop.startsWith('--')) variables.set(declaration.prop, declaration.value) })
    }
  })
  const sheet = parse(readCss('./MessageFeedbackDialog.module.css'))
  sheet.walkRules('.backdrop', rule => { rule.walkDecls(declaration => { if (declaration.prop.startsWith('--')) variables.set(declaration.prop, declaration.value) }) })
  const declarations = (selector: string) => {
    const values = new Map<string, string>()
    sheet.walkRules(selector, rule => { rule.walkDecls(declaration => { values.set(declaration.prop, declaration.value) }) })
    return values
  }
  const resolve = (value: string | undefined): string | undefined => {
    const token = value?.match(/^var\((--[\w-]+)\)$/)?.[1]
    return token ? resolve(variables.get(token)) : value
  }
  const fill = theme === 'dark' ? 'rgb(249, 250, 251)' : 'rgb(15, 17, 21)'
  expect(resolve(declarations('.submit').get('background'))).toBe(fill)
  expect(resolve(declarations('.chip[data-active]').get('background'))).toBe(fill)
  expect(resolve(declarations('.chip[data-active]').get('border-color'))).toBe(fill)
  expect(resolve(declarations('.chip[data-active]').get('color'))).toBe(theme === 'dark' ? 'rgb(15, 17, 21)' : 'rgb(255, 255, 255)')
  expect(resolve(declarations('.dialog').get('border-radius'))).toBe('28px')
  expect(resolve(declarations('.detail').get('border-radius'))).toBe('16px')
  expect(resolve(variables.get('--dsw-alias-bg-base'))).toBe(theme === 'dark' ? 'rgb(21, 21, 23)' : 'rgb(255, 255, 255)')
  expect(declarations('.chip:hover:not(:disabled):not([data-active])').get('background')).toBe('var(--dsw-alias-interactive-bg-hover)')
  await click(dialog.querySelector('button[aria-pressed="false"]')!)
  expect(dialog.querySelector('button[data-active]')?.getAttribute('aria-pressed')).toBe('true')
})

it('opens only after activation, traps focus, keeps a failed draft, and restores the opener', async () => {
  const request = vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockImplementation(async (_path, method) => ({
    ok: method === 'GET', status: method === 'GET' ? 200 : 503, body: JSON.stringify(method === 'GET' ? { items: [] } : {})
  }))
  await act(async () => root.render(createElement(MessageFeedbackActions, { threadId: 'ui-thread', messageId: 'message' })))
  expect(request).not.toHaveBeenCalled()
  const opener = container.querySelector<HTMLButtonElement>('button[aria-label="Good response"]')!
  await act(async () => opener.focus()); await click(opener)
  expect(request).toHaveBeenCalledTimes(1)
  const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
  const textarea = dialog.querySelector<HTMLTextAreaElement>('textarea')!
  expect(document.activeElement).toBe(textarea)
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!
  await act(async () => { valueSetter.call(textarea, 'draft retained'); textarea.dispatchEvent(new Event('input', { bubbles: true })) })
  await click(dialog.querySelector('button[type="submit"]')!)
  expect(textarea.value).toBe('draft retained'); expect(dialog.textContent).toContain('Your draft is kept')
  const last = dialog.querySelector<HTMLButtonElement>('button[type="submit"]')!
  await act(async () => { last.focus(); document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true })) })
  expect(document.activeElement).toBe(dialog.querySelector('button'))
  await click(dialog.querySelector('button[aria-label="Close feedback"]')!)
  expect(document.querySelector('[role="dialog"]')).toBeNull()
  await act(async () => { await Promise.resolve() }); expect(document.activeElement).toBe(opener)
})
