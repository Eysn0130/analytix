// @vitest-environment jsdom
import { act, createElement, type ButtonHTMLAttributes } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { Tooltip } from './Tooltip'
import css from './Tooltip.module.css'

let root: Root
let container: HTMLDivElement

class TooltipResizeObserver implements ResizeObserver {
  constructor(private readonly callback: ResizeObserverCallback) {}
  observe(target: Element): void {
    this.callback([{
      target,
      borderBoxSize: [{ inlineSize: 100, blockSize: 24 }],
      contentBoxSize: [{ inlineSize: 100, blockSize: 24 }],
      devicePixelContentBoxSize: [{ inlineSize: 100, blockSize: 24 }],
      contentRect: new DOMRect(0, 0, 100, 24)
    }], this)
  }
  unobserve(): void {}
  disconnect(): void {}
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', TooltipResizeObserver)
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  vi.unstubAllGlobals()
})

async function render(portal = false, bubbleClassName?: string): Promise<void> {
  const anchorProps: ButtonHTMLAttributes<HTMLButtonElement> & { 'data-tooltip-anchor': boolean } = {
    type: 'button', 'data-tooltip-anchor': true
  }
  await act(async () => root.render(createElement(Tooltip, {
    label: 'Viewer action', portal, bubbleClassName,
    children: createElement('button', anchorProps, 'Action')
  })))
}

async function show(): Promise<HTMLSpanElement> {
  const anchor = container.querySelector<HTMLButtonElement>('[data-tooltip-anchor]')!
  await act(async () => anchor.dispatchEvent(new MouseEvent('mouseover', { bubbles: true })))
  const bubble = document.querySelector<HTMLSpanElement>('[role="tooltip"]')!
  expect(bubble).not.toBeNull()
  return bubble
}

it('retains the exact default bubble class when no local style is supplied', async () => {
  await render()
  const bubble = await show()
  expect(bubble.className).toBe(css.bubble)
  expect(container.contains(bubble)).toBe(true)
})

it('carries only the caller bubble class through a body portal while preserving chat token scope', async () => {
  container.setAttribute('data-analytix-chat-view', '')
  await render(true, 'viewer-local-tooltip')
  const bubble = await show()
  expect(bubble.parentElement).toBe(document.body)
  expect(bubble.classList.contains(css.bubble)).toBe(true)
  expect(bubble.classList.contains('viewer-local-tooltip')).toBe(true)
  expect(bubble.getAttribute('data-analytix-chat-token-scope')).toBe('true')
  expect(container.querySelector('[data-tooltip-anchor]')?.classList.contains('viewer-local-tooltip')).toBe(false)
  expect(document.body.classList.contains('viewer-local-tooltip')).toBe(false)
})

it('withdraws a caller class without leaking it or token scope into the next default instance', async () => {
  container.setAttribute('data-analytix-chat-view', '')
  await render(true, 'viewer-local-tooltip')
  const bubble = await show()
  await render(true)
  expect(bubble.className).toBe(css.bubble)
  expect(bubble.classList.contains('viewer-local-tooltip')).toBe(false)
  await act(async () => root.unmount())
  expect(document.querySelector('[role="tooltip"]')).toBeNull()
  container.removeAttribute('data-analytix-chat-view')
  root = createRoot(container)
  await render(true)
  const next = await show()
  expect(next.className).toBe(css.bubble)
  expect(next.getAttribute('data-analytix-chat-token-scope')).toBeNull()
})
