// @vitest-environment jsdom
import { act, createElement, createRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock, ToolBlock } from '../../agent/types'
import i18n from '../../i18n'
import { RUNTIME_DIAGNOSTICS_FOCUS_EVENT } from '../../lib/runtime-diagnostics-focus'
import { useChatStore } from '../../store/chat-store'
import { MessageTimeline } from './MessageTimeline'
import { ProcessSectionRow } from './message-timeline-process'
import { formatDuration } from './message-timeline-tools'

let root: Root
let container: HTMLDivElement
let previousState: ReturnType<typeof useChatStore.getState>
const now = 1_800_000_000_000
const tool = (id: string): ToolBlock => ({
  kind: 'tool', id, summary: 'read: public fixture', status: 'success',
  detail: 'tool arguments withheld', meta: { toolName: 'read', diagnostics: { jobId: 'job-safe' } }
})
async function renderSection(grouped: boolean): Promise<void> {
  await act(async () => root.render(createElement(ProcessSectionRow, {
    section: { id: 'section-safe', kind: 'execution', blocks: grouped ? [tool('tool-a'), tool('tool-b')] : [tool('tool-a')] },
    processing: false, viewportRef: createRef<HTMLDivElement>()
  })))
  if (grouped) await act(async () => container.querySelector<HTMLElement>('[data-disclosure-row][role="button"]')!.click())
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  vi.spyOn(Date, 'now').mockReturnValue(now)
  await i18n.changeLanguage('zh')
  previousState = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'presentation-thread', route: 'chat', activeThreadGoal: null })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); useChatStore.setState(previousState)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('existing public process disclosure interaction', () => {
  it('exposes grouped disclosure state and its actual body', async () => {
    await act(async () => root.render(createElement(ProcessSectionRow, {
      section: { id: 'section-safe', kind: 'execution', blocks: [tool('tool-a'), tool('tool-b')] },
      processing: false, viewportRef: createRef<HTMLDivElement>()
    })))
    const header = container.querySelector<HTMLElement>('[data-disclosure-row][role="button"]')!
    expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelectorAll('[data-analytix-process-block-id]')).toHaveLength(0)
    await act(async () => header.click())
    expect(header.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelectorAll('[data-analytix-process-block-id]')).toHaveLength(2)
    await act(async () => header.click())
    expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(container.querySelectorAll('[data-analytix-process-block-id]')).toHaveLength(0)
  })
  it.each([false, true].flatMap(grouped => ['Enter', ' '].flatMap(key => ['chevron', 'diagnostics'].map(control => ({ grouped, key, control })))))(
    'keeps descendant $control $key activation isolated (grouped=$grouped)', async ({ grouped, key, control }) => {
      await renderSection(grouped)
      const row = container.querySelector<HTMLDivElement>('[data-analytix-process-block-id="tool-a"] [role="button"]')!
      const button = control === 'chevron'
        ? row.querySelector<HTMLButtonElement>('button[aria-expanded]')!
        : row.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:openRuntimeDiagnostics')}"]`)!
      expect(row).not.toBeNull(); expect(button).not.toBeNull()
      const initial = row.getAttribute('aria-expanded')
      const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      await act(async () => button.dispatchEvent(event))
      expect(row.getAttribute('aria-expanded')).toBe(initial)
      expect(event.defaultPrevented).toBe(false)
      const focused = vi.fn()
      window.addEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, focused)
      try {
        // jsdom does not synthesize a native button click from dispatchEvent(keydown).
        await act(async () => button.click())
        expect(row.getAttribute('aria-expanded')).toBe(control === 'chevron' ? String(initial !== 'true') : initial)
        expect(focused).toHaveBeenCalledTimes(control === 'diagnostics' ? 1 : 0)
      } finally { window.removeEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, focused) }
    }
  )
  it.each([false, true].flatMap(grouped => ['Enter', ' '].map(key => ({ grouped, key }))))(
    'retains row keyboard activation $key (grouped=$grouped)', async ({ grouped, key }) => {
      await renderSection(grouped)
      const row = container.querySelector<HTMLDivElement>('[data-analytix-process-block-id="tool-a"] [role="button"]')!
      const initial = row.getAttribute('aria-expanded')
      const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      await act(async () => row.dispatchEvent(event))
      expect(event.defaultPrevented).toBe(true)
      expect(row.getAttribute('aria-expanded')).toBe(String(initial !== 'true'))
    }
  )
  it('keeps bodyless running progress noninteractive, then permits real public rows to reopen', async () => {
    const user: ChatBlock = { kind: 'user', id: 'user-safe', text: 'Synthetic request', meta: { turnId: 'turn-safe' } }
    const renderTimeline = async (blocks: ChatBlock[]) => act(async () => root.render(createElement(MessageTimeline, {
      blocks, live: '', activeThreadId: 'presentation-thread', runtimeConnection: 'ready',
      onRetryConnection: () => {}, onOpenSettings: () => {},
      runtimeStateOverride: { busy: true, currentTurnId: 'turn-safe', currentTurnUserId: 'user-safe',
        turnStartedAtByUserId: { 'user-safe': now - 12_000 }, turnDurationByUserId: {} }
    })))
    const label = `${i18n.t('common:processing')} ${formatDuration(12_000)}`
    const progress = () => [...container.querySelectorAll('.tabular-nums')].find(e => e.textContent === label)!
    await renderTimeline([user])
    expect(progress()).toBeDefined()
    expect(progress().closest('button,[role="button"]')).toBeNull()
    const stage = i18n.t('common:providerRequestPreparingStatus')
    await renderTimeline([user, { kind: 'system', id: 'public-stage', text: stage, meta: { turnId: 'turn-safe' } }])
    const header = progress().closest<HTMLButtonElement>('button')!
    expect(header).not.toBeNull()
    expect(container.textContent).toContain(stage)
    await act(async () => header.click())
    expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(container.textContent).not.toContain(stage)
    await act(async () => header.click())
    expect(header.getAttribute('aria-expanded')).toBe('true')
    expect(container.textContent).toContain(stage)
    expect(progress().textContent).toBe(label)
  })
})
