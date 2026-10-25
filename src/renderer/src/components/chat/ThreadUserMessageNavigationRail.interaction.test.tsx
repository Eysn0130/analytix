// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThreadUserMessageNavigationRail } from './ThreadUserMessageNavigationRail'

let root: Root
let stage: HTMLDivElement
let host: HTMLDivElement
let timeline: HTMLDivElement
let outside: HTMLButtonElement

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('IntersectionObserver', undefined)
  stage = document.createElement('div')
  stage.className = 'ds-chat-stage'
  host = document.createElement('div')
  timeline = document.createElement('div')
  outside = document.createElement('button')
  stage.append(host, timeline)
  document.body.append(outside, stage)
  root = createRoot(host)
})

afterEach(async () => {
  await act(async () => root.unmount())
  stage.remove()
  outside.remove()
  vi.unstubAllGlobals()
})

function setGeometry(node: HTMLElement, offsetTop: number): void {
  Object.defineProperties(node, {
    offsetTop: { configurable: true, value: offsetTop },
    offsetHeight: { configurable: true, value: 20 }
  })
}

async function mountRail(id: string, decoyId = `${id}-decoy`): Promise<{
  list: HTMLDivElement
  target: HTMLButtonElement
  triggerEffect: () => Promise<void>
}> {
  outside.dataset.threadUserMessageNavigationItemId = id
  setGeometry(outside, 1)
  const items = [decoyId, 'filler-a', 'filler-b', id].map((itemId, index) => ({
    id: itemId, turnKey: `turn-${index}`, label: `${index + 1}`, title: `Message ${index + 1}`,
    preview: '', position: index + 1
  }))
  await act(async () => root.render(createElement(ThreadUserMessageNavigationRail, {
    items, containerRef: { current: timeline }, railLabel: 'Messages', noContentLabel: 'Empty',
    itemAriaLabel: item => item.label, onSelectItem: vi.fn()
  })))
  const list = stage.querySelector<HTMLDivElement>('[data-thread-user-message-navigation-rail-list]')!
  const buttons = [...list.querySelectorAll<HTMLButtonElement>('button')]
  const target = buttons[3]
  expect(target.dataset.threadUserMessageNavigationItemId).toBe(id)
  Object.defineProperty(list, 'clientHeight', { configurable: true, value: 100 })
  list.scrollTop = 50
  setGeometry(buttons[0], 5)
  setGeometry(target, 20)
  return {
    list, target,
    // Opening the tooltip reruns the real scroll effect after jsdom geometry is installed.
    triggerEffect: async () => { await act(async () => buttons[0].focus()) }
  }
}

describe.each([
  { environment: 'CSS is absent', css: undefined },
  { environment: 'CSS.escape is absent', css: {} }
])('$environment', ({ css }) => {
  beforeEach(() => vi.stubGlobal('CSS', css))

  it.each([
    { character: 'quotes', id: 'message"quote', decoyId: 'message-quote' },
    { character: 'backslash', id: 'message\\66', decoyId: 'messagef' },
    { character: 'newline', id: 'message\nline', decoyId: 'messageline' },
    { character: 'brackets', id: 'message[brackets]', decoyId: 'messagebrackets' },
    { character: 'comma', id: 'message,other', decoyId: 'message-other' },
    { character: 'Unicode', id: '消息🧭', decoyId: '消息' },
    { character: 'combined characters', id: 'message"\\\n[brackets],消息🧭', decoyId: 'message' }
  ])('scrolls to the first exact in-rail ID with $character', async ({ id, decoyId }) => {
    const { list, target, triggerEffect } = await mountRail(id, decoyId)
    const laterMatch = document.createElement('button')
    laterMatch.dataset.threadUserMessageNavigationItemId = id
    setGeometry(laterMatch, 300)
    list.append(laterMatch)

    await triggerEffect()

    expect(list.scrollTop).toBe(20)
    expect(list.contains(target)).toBe(true)
    expect(list.contains(outside)).toBe(false)
  })

  it.each([
    { position: 'above the viewport', offsetTop: 20, missing: false, expected: 20 },
    { position: 'below the viewport', offsetTop: 160, missing: false, expected: 81 },
    { position: 'already visible', offsetTop: 80, missing: false, expected: 50 },
    { position: 'at the top edge', offsetTop: 50, missing: false, expected: 50 },
    { position: 'at the bottom edge', offsetTop: 130, missing: false, expected: 50 },
    { position: 'missing from the rail', offsetTop: 20, missing: true, expected: 50 }
  ])('preserves scrolling when the active item is $position', async ({ offsetTop, missing, expected }) => {
    const { list, target, triggerEffect } = await mountRail('active-message')
    setGeometry(target, offsetTop)
    if (missing) target.remove()

    await triggerEffect()

    expect(list.scrollTop).toBe(expected)
  })
})
