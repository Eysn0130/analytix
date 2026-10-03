// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { ThreadRenameDialog } from './SidebarProjectsSection'

describe('thread rename overlay', () => {
  it('escapes a clipping sidebar while preserving initial focus, disabled submit and Escape', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const container = document.createElement('div')
    container.style.overflow = 'hidden'
    container.style.transform = 'translateX(0)'
    document.body.append(container)
    const root = createRoot(container)
    const onClose = vi.fn()
    try {
      await act(async () => root.render(createElement(ThreadRenameDialog, {
        state: { thread: { id: 'overlay-test', title: 'Original title', updatedAt: '2026-10-02T00:00:00.000Z', model: 'synthetic-model', mode: 'agent' }, value: 'Original title', submitting: false },
        onClose, onValueChange: vi.fn(), onSubmit: vi.fn(), t: (key) => key
      })))
      const dialog = document.querySelector('[role="dialog"]')
      expect(dialog?.parentElement).toBe(document.body)
      expect(container.contains(dialog)).toBe(false)
      const input = dialog?.querySelector('input')
      expect(document.activeElement).toBe(input)
      expect(input?.value).toBe('Original title')
      expect(dialog?.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
      await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })))
      expect(onClose).toHaveBeenCalledTimes(1)
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})
