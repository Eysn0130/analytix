// @vitest-environment jsdom
import { createElement, act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it } from 'vitest'
import { ReasoningRow, reasoningPreview } from './ReasoningRow'

describe('protected reasoning preview', () => {
  it('uses only the latest newline-complete paragraph while running and the first line when settled', () => {
    expect(reasoningPreview('unfinished single line', true)).toBe('')
    expect(reasoningPreview('**First**\nbody\n\nunfinished latest', true)).toBe('First')
    expect(reasoningPreview('**First**\nbody\n\n**Latest**\nunfinished body', true)).toBe('Latest')
    expect(reasoningPreview('**First**\nbody\n\n**Latest**\nbody', false)).toBe('First')
  })

  it('renders the authoritative projected running preview without parsing it as raw body again', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(createElement(ReasoningRow, {
        id: 'actual-reasoning', running: true, title: 'Think', preview: 'projected latest line',
        protectedBody: createElement('div', null, 'protected body'), expanded: false, expandable: true, onToggle: () => undefined
      })))
      expect(container.querySelector('[data-reasoning-id="actual-reasoning"]')?.getAttribute('data-preview')).toBe('true')
      expect(container.textContent).toContain('projected latest line')
      expect(container.textContent).not.toContain('protected body')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})
