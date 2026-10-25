// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { ScheduleCreateCard } from './ScheduleCreateCard'

describe('admitted Schedule creation card presentation', () => {
  it('uses current title and frequency, falls back to creation, and removes both deletion targets', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const container = document.createElement('div'), root = createRoot(container), open = vi.fn()
    const creation = { taskId: 'task', title: 'Created title', frequency: 'Daily at 09:00' }
    const labels = { open: 'Open', deleted: 'Deleted', openLabel: (title: string) => 'Open ' + title }
    const draw = async (currentTask?: { title: string; frequency: string } | null) => act(async () => root.render(
      createElement(ScheduleCreateCard, { creation, currentTask, labels, openTaskDetail: open })))
    await draw({ title: 'Current title', frequency: 'Every 60 minutes' })
    expect(container.textContent).toContain('Current title')
    expect(container.textContent).toContain('Every 60 minutes')
    const targets = container.querySelectorAll('button')
    expect(targets).toHaveLength(2)
    await act(async () => { targets[0].click(); targets[1].click() })
    expect(open.mock.calls).toEqual([['task', 'Current title'], ['task', 'Current title']])
    await draw(undefined)
    expect(container.textContent).toContain('Created title')
    await draw(null)
    expect(container.textContent).toContain('Deleted')
    expect(container.querySelectorAll('button')).toHaveLength(0)
    await act(async () => root.unmount())
  })
})
