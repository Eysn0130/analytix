// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { ComposerGoalRow } from './ComposerGoalRow'

describe('ComposerGoalRow', () => {
  it('keeps real goal details reachable and dispatches its original actions once', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    await i18n.changeLanguage('en')
    const host = document.createElement('div'); document.body.append(host)
    const root = createRoot(host), onEdit = vi.fn(), onToggleStatus = vi.fn(), onClear = vi.fn()
    try {
      await act(async () => root.render(createElement(ComposerGoalRow, {
        goal: {threadId:'fixture',objective:'A real goal with a long objective',status:'active',tokensUsed:0,timeUsedSeconds:30,createdAt:'2026-10-03',updatedAt:'2026-10-03'},
        elapsedLabel:'30s',onEdit,onToggleStatus,onClear
      })))
      const summary = host.querySelector<HTMLButtonElement>('.ds-composer-goal-summary')!
      const details = document.getElementById(summary.getAttribute('aria-controls')!)!
      expect(summary.getAttribute('aria-expanded')).toBe('false')
      expect(details.hidden).toBe(true)
      await act(async () => summary.click())
      expect(details.hidden).toBe(false)
      expect(details.textContent).toContain('A real goal with a long objective')
      for (const key of ['goalActionEdit','goalActionPause','goalActionClear']) {
        const button = [...details.querySelectorAll('button')].find(b => b.getAttribute('aria-label') === i18n.t(`common:${key}`))!
        await act(async () => button.click())
      }
      expect(onEdit).toHaveBeenCalledTimes(1)
      expect(onToggleStatus).toHaveBeenCalledTimes(1)
      expect(onClear).toHaveBeenCalledTimes(1)
      await act(async () => summary.click())
      expect(details.hidden).toBe(true)
    } finally { await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals() }
  })
})
