// @vitest-environment jsdom
import { act, createElement, Profiler, useLayoutEffect, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useAnchoredPanel } from '../useAnchoredPanel'
import panelCss from '../AnchoredPanel.module.css'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { emitRendererSettingsChanged } from '../../../../lib/keyboard-shortcut-settings'
import type { ThreadUsageSnapshot } from '../../../../agent/types'
import { ProviderBillingV1Schema, ConversationMetricsV1Schema, type ProviderBillingV1, type ConversationMetricsV1 } from '../../../../../../../packages/runtime/src/contracts/usage'
import i18n from '../../../../i18n'
import { rendererRuntimeClient } from '../../../../agent/runtime-client'
import { useThreadUsageState, type ThreadUsageState } from '../../../../hooks/use-thread-usage'
import { TurnTailView } from './TurnTailView'
import { ContextCapacityPopover } from '../../ContextCapacityPopover'
import type { ContextCapacity } from '../../../../lib/context-capacity'
import { OwnedSessionStatsPills, publishPerformanceUsageChoice, usePerformanceUsageMode, SessionStatsPills, TurnUsagePanel, formatDshCacheHitPercent, formatDshExactTokens,
  formatDshTokens, turnAccountingFromSnapshot, turnAccountingFromBilling } from './TurnUsagePanel'

let root: Root, host: HTMLDivElement
let clipboardDescriptor: PropertyDescriptor | undefined, execDescriptor: PropertyDescriptor | undefined
const t = (key: string, values?: Record<string, unknown>): string => String(i18n.t('common:' + key, values))
const snapshot: ThreadUsageSnapshot = { inputTokens: 999, outputTokens: 99, totalTokens: 1098,
  reasoningTokens: 0, cachedTokens: 0, cacheMissTokens: 999, cacheHitRate: null,
  costUsd: null, costCny: null, priceConfigured: false, tokenEconomySavingsTokens: 0, turns: 1,
  cacheDiagnostics: { providerAttemptTelemetryValid: true, providerAttemptCount: 2,
    providerAttemptInputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 12 },
    providerAttemptOutputTokens: { complete: true, knownObservationCount: 2, observationCount: 2, value: 4 } } }
// Fixtures pass the producer's strict canonical schemas; legacy 1098 is deliberately unrelated.
function billing(input = 12, output = 4, attempts = 2, split?: { uncached: number; read: number; write: number }): ProviderBillingV1 {
  const count = (value: number, known = attempts, complete = true) => ({ reportedValue: value, knownAttemptCount: known, attemptCount: attempts, complete })
  const unknown = count(0, 0, attempts === 0)
  return ProviderBillingV1Schema.parse({ inventoryComplete: true, closed: true, attemptCount: attempts, settledAttemptCount: attempts,
    inputTokens: count(input), outputTokens: count(output), totalTokens: count(input + output),
    uncachedInputTokens: split ? count(split.uncached) : unknown,
    cacheReadTokens: split ? count(split.read) : unknown, cacheWriteTokens: split ? count(split.write) : unknown,
    reasoningTokens: unknown })
}
function metrics(overrides: Partial<ConversationMetricsV1> = {}): ConversationMetricsV1 {
  return ConversationMetricsV1Schema.parse({ turnCount: 2, stepCount: 3,
    llmMs: { reportedValue: 0, knownStepCount: 3, complete: true },
    toolMs: { reportedValue: 0, knownCallCount: 1, callCount: 1, complete: true },
    ttftMs: 0, ttftSteps: 1, decodeMs: 1000, decodeTokens: 0, decodeSteps: 1, ...overrides })
}
const completeBilling = billing()
const footerBilling = ProviderBillingV1Schema.parse({ ...completeBilling,
  uncachedInputTokens: { ...completeBilling.inputTokens } })
const finalBlock = { kind: 'assistant' as const, id: 'closing', text: 'Actual closing reply', meta: { turnId: 'turn' } }
const turnUsageOwner = JSON.stringify(['fixture-thread', 'turn'])
const sessionUsageOwner = 'fixture-thread'
async function render(element: ReactNode) { await act(async () => root.render(element)) }
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}
function restore(object: object, key: string, descriptor?: PropertyDescriptor) {
  if (descriptor) Object.defineProperty(object, key, descriptor)
  else Reflect.deleteProperty(object, key)
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  await i18n.changeLanguage('en')
  clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  execDescriptor = Object.getOwnPropertyDescriptor(document, 'execCommand')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove()
  restore(navigator, 'clipboard', clipboardDescriptor); restore(document, 'execCommand', execDescriptor)
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('DSH turn accounting and real footer actions', () => {
  it('requires complete uncached/output/total but permits missing optional buckets and routes', async () => {
    const accounting = turnAccountingFromSnapshot(snapshot)!
    expect(accounting).toEqual({ inputTokens: 12, outputTokens: 4, totalTokens: 16 })
    expect(turnAccountingFromBilling(completeBilling)).toBeUndefined()
    expect(turnAccountingFromBilling(footerBilling)).toMatchObject(accounting)
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: footerBilling }))
    const button = host.querySelector<HTMLButtonElement>('button')!
    expect(button.textContent).toContain('16'); expect(button.textContent).not.toContain('1098')
    await act(async () => button.click())
    const panel = document.querySelector('[role="dialog"]')!
    expect(panel.textContent).toContain(t('dshUsageInput')); expect(panel.textContent).toContain('12')
    expect(panel.textContent).not.toContain(t('dshUsageCacheRead')); expect(panel.textContent).not.toContain(t('dshUsageModel'))
    expect(panel.textContent).not.toMatch(/TTFT|decode|per second/i)
    const incomplete = { ...snapshot, cacheDiagnostics: { ...snapshot.cacheDiagnostics,
      providerAttemptInputTokens: { complete: false, knownObservationCount: 1, observationCount: 2 } } }
    expect(turnAccountingFromSnapshot(incomplete)).toBeUndefined()
    expect(turnAccountingFromSnapshot({ ...snapshot, cacheDiagnostics: undefined })).toBeUndefined()
    expect(turnAccountingFromSnapshot({ ...snapshot, cacheDiagnostics: { ...snapshot.cacheDiagnostics,
      providerAttemptInputTokens: { complete: true, knownObservationCount: 1, observationCount: 2, value: 12 } } })).toBeUndefined()
  })
  it('shows a recorded zero while Compact removes the turn panel', async () => {
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: billing(0, 0, 0) }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    const recordedZero = billing(0, 0, 1, { uncached: 0, read: 0, write: 0 })
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: recordedZero }))
    expect(host.querySelector('button')!.textContent).toContain('0')
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: recordedZero, mode: 'compact' }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
  })
  it('preserves exact grouping and never rounds a partial cache hit to 100%', () => {
    expect(formatDshTokens(1234, t)).toBe('1.2K')
    expect(formatDshExactTokens(1234567, t)).toBe('1,234,567')
    expect(formatDshCacheHitPercent(999999, 1000000)).toBe('99.9999')
    expect(formatDshCacheHitPercent(Number.MAX_SAFE_INTEGER - 1, Number.MAX_SAFE_INTEGER)).not.toBe('100')
    expect(formatDshCacheHitPercent(0, 0)).toBeNull(); expect(formatDshCacheHitPercent(10, 10)).toBe('100')
    expect(formatDshCacheHitPercent(11, 10)).toBeNull()
  })
  it('requires a real turn end and preserves focusable unavailable Fork', async () => {
    const fork = { busy: false, run: vi.fn() }
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, processing: false, hasOutput: true, finalBlock, fork }))
    expect(host.querySelector('button')).toBeNull()
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, processing: false, ended: true, hasOutput: true, finalBlock,
      fork: { ...fork, unavailable: true } }))
    const button = host.querySelector<HTMLButtonElement>(`button[aria-label="${t('forkFromAssistantResponse')}"]`)!
    expect(button.disabled).toBe(false); expect(button.getAttribute('aria-disabled')).toBe('true')
    await act(async () => button.focus()); expect(document.activeElement).toBe(button)
    await act(async () => button.click()); expect(fork.run).not.toHaveBeenCalled()
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, processing: false, ended: true, hasOutput: true, finalBlock, fork }))
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${t('forkFromAssistantResponse')}"]`)!.click())
    expect(fork.run).toHaveBeenCalledOnce()
  })
  it('copies only closing text, gates pending/done writes and clears feedback after one second', async () => {
    vi.useFakeTimers()
    const pending = deferred<void>(), writeText = vi.fn(() => pending.promise)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, processing: false, ended: true, hasOutput: true, finalBlock }))
    const button = host.querySelector<HTMLButtonElement>(`button[aria-label="${t('copyMessage')}"]`)!
    await act(async () => { button.click(); button.click() })
    expect(writeText).toHaveBeenCalledOnce(); expect(writeText).toHaveBeenCalledWith('Actual closing reply')
    await act(async () => { pending.resolve(); await pending.promise })
    expect(button.getAttribute('aria-label')).toBe(t('copySuccess'))
    await act(async () => button.click()); expect(writeText).toHaveBeenCalledOnce()
    await act(async () => vi.advanceTimersByTime(1000))
    expect(button.getAttribute('aria-label')).toBe(t('copyMessage'))
    await act(async () => button.click()); expect(writeText).toHaveBeenCalledTimes(2)
  })
  it('keeps rejected async Copy idle and only falls back when Clipboard API is absent', async () => {
    const select = vi.spyOn(HTMLTextAreaElement.prototype, 'select')
    const exec = vi.fn(() => {
      const area = document.querySelector('textarea')!
      expect(area.value).toBe('Actual closing reply'); expect(area.hasAttribute('readonly')).toBe(true)
      return true
    }), writeText = vi.fn(async () => { throw new Error('Denied') })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, processing: false, ended: true, hasOutput: true, finalBlock }))
    const button = host.querySelector<HTMLButtonElement>(`button[aria-label="${t('copyMessage')}"]`)!
    await act(async () => button.click())
    expect(exec).not.toHaveBeenCalled(); expect(button.getAttribute('aria-label')).toBe(t('copyMessage'))
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    await act(async () => button.click())
    expect(exec).toHaveBeenCalledWith('copy'); expect(select).toHaveBeenCalledOnce(); expect(document.querySelector('textarea')).toBeNull()
    expect(button.getAttribute('aria-label')).toBe(t('copySuccess'))
  })
})

describe('DSH portal and composer metric contracts', () => {
  it('anchors above the trigger, clamps narrow bounds, tracks resize and dismisses through Escape/capture click', async () => {
    vi.stubGlobal('innerWidth', 400); vi.stubGlobal('innerHeight', 600)
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(280)
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(160)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(new DOMRect(280, 340, 120, 28))
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: footerBilling }))
    const button = host.querySelector<HTMLButtonElement>('button')!
    await act(async () => button.click())
    const panel = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(host.contains(panel)).toBe(false); expect(panel.parentElement).toBe(document.body)
    expect(panel.style.left).toBe('108px'); expect(panel.style.top).toBe('172px')
    vi.stubGlobal('innerWidth', 320)
    await act(async () => window.dispatchEvent(new Event('resize')))
    expect(panel.style.left).toBe('28px')
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    await act(async () => document.dispatchEvent(escape))
    expect(escape.defaultPrevented).toBe(true)
    expect(document.querySelector('[role="dialog"]')).toBeNull(); expect(button.getAttribute('aria-expanded')).toBe('false')
    await act(async () => button.click())
    const outside = document.createElement('button'); document.body.append(outside)
    outside.addEventListener('click', event => event.stopPropagation())
    await act(async () => outside.click())
    expect(document.querySelector('[role="dialog"]')).toBeNull(); outside.remove()
  })
  it('keeps true zero TTFT and decode speed distinct, and never reopens a dialog across modes', async () => {
    const activity = metrics(), usage = billing(20, 4, 2, { uncached: 10, read: 10, write: 0 })
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: activity, billing: usage }))
    const activityButton = host.querySelector<HTMLButtonElement>('[data-composer-stat="activity"] button')!
    expect(activityButton.getAttribute('aria-label')).toContain('0')
    await act(async () => activityButton.click())
    expect(document.querySelector('[data-session-stats-details]')!.textContent).toContain('0s')
    await act(async () => host.querySelector<HTMLButtonElement>('[data-composer-stat="usage"] button')!.click())
    expect(document.querySelector('[data-session-stats-usage]')!.textContent).not.toContain(t('dshUsageCacheWrite'))
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'compact', metrics: activity, billing: usage }))
    expect(document.querySelector('[role="dialog"]')).toBeNull(); expect(host.querySelector('button')).toBeNull()
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: activity, billing: usage }))
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'compact', metrics: metrics({ decodeMs: 0 }),
      billing: billing(0, 4, 2, { uncached: 0, read: 0, write: 0 }) }))
    expect(host.querySelector('[data-composer-stat]')).toBeNull()
  })
  it('drops A usage at the first B render, preserves same-owner refresh and ignores late A resolution', async () => {
    type Response = Awaited<ReturnType<typeof rendererRuntimeClient.runtimeRequest>>
    const a = deferred<Response>(), refresh = deferred<Response>(), b = deferred<Response>()
    vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockImplementationOnce(() => a.promise)
      .mockImplementationOnce(() => refresh.promise).mockImplementationOnce(() => b.promise)
    const seen: { id: string; state: ThreadUsageState }[] = []
    function Probe({ id, refreshKey }: { id: string; refreshKey: number }) {
      const state = useThreadUsageState(id, true, refreshKey); seen.push({ id, state })
      return createElement('output', null, `${id}:${state.usage?.totalTokens ?? 'unknown'}`)
    }
    const response = (id: string, total: number): Response => ({ ok: true, status: 200, body: JSON.stringify({
      buckets: [{ thread_id: id, input_tokens: total, output_tokens: 0, total_tokens: total, turns: 1 }] }) })
    await render(createElement(Probe, { id: 'A', refreshKey: 0 }))
    await act(async () => { a.resolve(response('A', 101)); await a.promise })
    expect(host.textContent).toBe('A:101')
    await render(createElement(Probe, { id: 'A', refreshKey: 1 }))
    expect(host.textContent).toBe('A:101'); expect(seen.at(-1)!.state.loading).toBe(true)
    await render(createElement(Probe, { id: 'B', refreshKey: 1 }))
    expect(seen.filter(value => value.id === 'B').every(value => value.state.usage === null)).toBe(true)
    await act(async () => { refresh.resolve(response('A', 303)); await refresh.promise })
    expect(host.textContent).toBe('B:unknown')
    await act(async () => { b.resolve(response('B', 202)); await b.promise })
    expect(host.textContent).toBe('B:202')
  })
})


describe('owned normal preference and missing whole-log groups', () => {
  it('hides missing groups and retires the actual dialog at the first owner switch', async () => {
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1))
    const projection = { threadId: 'A', billing: billing(5, 1, 1, { uncached: 3, read: 2, write: 0 }) }
    await render(createElement(OwnedSessionStatsPills, { threadId: 'A' }))
    expect(host.querySelector('[data-composer-stat]')).toBeNull()
    await render(createElement(OwnedSessionStatsPills, { threadId: 'A', projection }))
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    await render(createElement(OwnedSessionStatsPills, { threadId: 'B', projection }))
    expect(host.querySelector('[data-composer-stat]')).toBeNull(); expect(document.querySelector('[role="dialog"]')).toBeNull()
    await render(createElement(OwnedSessionStatsPills, { threadId: 'A', projection }))
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    await render(createElement(OwnedSessionStatsPills, { threadId: 'B', projection: { ...projection, threadId: 'B' } }))
    expect(host.querySelector('[data-composer-stat]')).not.toBeNull()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await render(createElement(OwnedSessionStatsPills, { threadId: 'A', projection }))
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
  it('keeps an immediate pure UI choice ahead of a stale Host read and adopts real accepted settings events', async () => {
    const read = deferred<AppSettingsV1>()
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockReturnValue(read.promise)
    function PreferenceTurn() { const mode = usePerformanceUsageMode(); return createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, mode, billing: billing(3, 1, 2, { uncached: 3, read: 0, write: 0 }) }) }
    await render(createElement(PreferenceTurn))
    await act(async () => publishPerformanceUsageChoice('compact'))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await act(async () => { read.resolve(normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1)); await read.promise })
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await act(async () => emitRendererSettingsChanged(normalizeAppSettings({ performanceUsageMode: 'detailed' } as AppSettingsV1)))
    expect(host.querySelector('[data-analytix-turn-usage]')).not.toBeNull()
  })
})


describe('canonical billing and timing coverage through real panels', () => {
  it('requires ended and closed complete turn billing without using legacy diagnostics', async () => {
    const common = { processing: false, hasOutput: true, finalBlock, usage: snapshot }
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, ...common, ended: true }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, ...common, billing: footerBilling }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    const open = ProviderBillingV1Schema.parse({ ...footerBilling, closed: false })
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, ...common, ended: true, billing: open }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await render(createElement(TurnTailView, { ownerKey: turnUsageOwner, ...common, ended: true, billing: footerBilling }))
    const usage = host.querySelector('[data-analytix-turn-usage]')!
    expect(usage.textContent).toContain('16'); expect(usage.textContent).not.toContain('1098')
  })
  it('keeps separately reported token subtotals partial and the unpaired total unknown', async () => {
    const partialCount = (value: number, known = 1) => ({ reportedValue: value, knownAttemptCount: known, attemptCount: 2, complete: false })
    const partial = ProviderBillingV1Schema.parse({ ...completeBilling,
      inputTokens: partialCount(12), outputTokens: partialCount(4), uncachedInputTokens: partialCount(12),
      totalTokens: partialCount(0, 0) })
    expect(turnAccountingFromBilling(partial)).toBeUndefined()
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: partial }))
    expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', billing: partial }))
    const button = host.querySelector<HTMLButtonElement>('[data-composer-stat="usage"] button')!
    expect(button.textContent).toBe(t('dshUsageUnknown'))
    await act(async () => button.click())
    const panel = document.querySelector('[data-session-stats-usage]')!
    const values = Array.from(panel.querySelectorAll('dd'), value => value.textContent)
    expect(values).toContain(t('dshUsageReportedPartial', { value: t('dshUsageTokens', { tokens: '12' }) }))
    expect(values).toContain(t('dshUsageReportedPartial', { value: t('dshUsageTokens', { tokens: '4' }) }))
    expect(values).toContain(t('dshUsageUnknown'))
    expect(document.querySelector('[role="dialog"]')!.textContent).not.toMatch(/16|1098/)
    expect(panel.textContent).not.toContain(t('dshUsageCacheHit'))
  })
  it('shows the actual partial intersection total instead of independently adding input and output', async () => {
    const count = (reportedValue: number, knownAttemptCount: number) => ({ reportedValue, knownAttemptCount, attemptCount: 3, complete: false })
    const partial = ProviderBillingV1Schema.parse({ inventoryComplete: true, closed: true, attemptCount: 3, settledAttemptCount: 3,
      inputTokens: count(12, 2), outputTokens: count(4, 2), totalTokens: count(7, 1),
      uncachedInputTokens: count(12, 2), cacheReadTokens: count(0, 0), cacheWriteTokens: count(0, 0), reasoningTokens: count(0, 0) })
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', billing: partial }))
    const button = host.querySelector<HTMLButtonElement>('[data-composer-stat="usage"] button')!
    expect(button.textContent).toBe(t('dshUsageReportedPartial', { value: t('dshUsageTokens', { tokens: '7' }) }))
    expect(button.textContent).not.toContain('16')
    await act(async () => button.click())
    expect(document.querySelector('[role="dialog"]')!.textContent).not.toContain('16')
    expect(turnAccountingFromBilling(partial)).toBeUndefined()
  })
  it('distinguishes a reported partial zero from a field with no known attempts', async () => {
    const partialZero = ProviderBillingV1Schema.parse({ ...completeBilling,
      uncachedInputTokens: { reportedValue: 0, knownAttemptCount: 1, attemptCount: 2, complete: false } })
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', billing: partialZero }))
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    const values = Array.from(document.querySelectorAll('[data-session-stats-usage] dd'), value => value.textContent)
    expect(values).toContain(t('dshUsageReportedPartial', { value: t('dshUsageTokens', { tokens: '0' }) }))
    expect(values).toContain(t('dshUsageUnknown'))
  })
  it('uses approved frozen routes while omitting an unknown cache rate', async () => {
    const routed = ProviderBillingV1Schema.parse({ ...footerBilling, routes: [{ provider: 'approved-provider', model: 'frozen-model' }] })
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: routed }))
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    const panel = document.querySelector('[data-turn-usage-details]')!
    expect(panel.textContent).toContain('approved-provider/frozen-model')
    expect(panel.textContent).not.toContain(t('dshUsageCacheHit'))
  })
  it('displays recorded disjoint input buckets and a known zero without filling unknown values', async () => {
    await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: billing(20, 4, 2, { uncached: 10, read: 10, write: 0 }) }))
    expect(host.querySelector('button')!.textContent).toContain('24')
    expect(host.querySelector('button')!.textContent).not.toContain('14')
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    const rows = Array.from(document.querySelectorAll('[data-turn-usage-details] dt'), term => [term.textContent, term.nextElementSibling!.textContent])
    expect(rows).toContainEqual([t('dshUsageInput'), t('dshUsageTokens', { tokens: '10' })])
    expect(rows).toContainEqual([t('dshUsageCacheRead'), t('dshUsageTokens', { tokens: '10' })])
    expect(rows).toContainEqual([t('dshUsageCacheWrite'), t('dshUsageTokens', { tokens: '0' })])
    expect(rows).toContainEqual([t('dshUsageOutput'), t('dshUsageTokens', { tokens: '4' })])
    expect(rows).toContainEqual([t('dshUsageCacheHit'), '50%'])
  })
  it('preserves nullable step coverage and independent zero TTFT and paired decode samples', async () => {
    const samples = metrics({ turnCount: null, stepCount: null, llmMs: { reportedValue: 1000, knownStepCount: 1, complete: false },
      toolMs: { reportedValue: 0, knownCallCount: 1, callCount: 2, complete: false } })
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: samples }))
    const activity = host.querySelector<HTMLButtonElement>('[data-composer-stat="activity"] button')!
    expect(activity.textContent).toContain(t('dshUsageUnknown'))
    await act(async () => activity.click())
    const panel = document.querySelector('[data-session-stats-details]')!
    const rows = Array.from(panel.querySelectorAll('dt'), term => [term.textContent, term.nextElementSibling!.textContent])
    expect(rows).toContainEqual([t('dshStatsLlmTime'), t('dshUsageReportedPartial', { value: t('dshDurationCompactSeconds', { seconds: 1 }) })])
    expect(rows).toContainEqual([t('dshStatsToolTime'), t('dshUsageReportedPartial', { value: t('dshDurationCompactSeconds', { seconds: 0 }) })])
    expect(rows).toContainEqual([t('dshStatsTtft'), t('dshDurationCompactSeconds', { seconds: 0 })])
    expect(rows).toContainEqual([t('dshStatsSpeed'), t('dshUsageTokensPerSecond', { tps: 0 })])
  })
  it.each(['ttft', 'decode'] as const)('keeps recorded %s samples independent of the missing timing group', async (kind) => {
    const samples = metrics(kind === 'ttft' ? { decodeMs: 0, decodeTokens: 0, decodeSteps: 0 }
      : { ttftMs: 0, ttftSteps: 0 })
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: samples }))
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    const labels = Array.from(document.querySelectorAll('[data-session-stats-details] dt'), term => term.textContent)
    expect(labels).toContain(t(kind === 'ttft' ? 'dshStatsTtft' : 'dshStatsSpeed'))
    expect(labels).not.toContain(t(kind === 'ttft' ? 'dshStatsSpeed' : 'dshStatsTtft'))
  })
  it('never divides by a recorded zero decode duration or invents timing from counts', async () => {
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'compact', metrics: metrics({ decodeMs: 0, decodeTokens: 9 }) }))
    expect(host.querySelector('[data-composer-stat="activity"]')).toBeNull()
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: metrics({ stepCount: null,
      llmMs: { reportedValue: 0, knownStepCount: 0, complete: false },
      toolMs: { reportedValue: 0, knownCallCount: 0, callCount: 0, complete: true },
      ttftMs: 0, ttftSteps: 0, decodeMs: 0, decodeTokens: 0, decodeSteps: 0 }) }))
    expect(host.querySelector('[data-composer-stat="activity"]')!.textContent).toContain(t('dshUsageUnknown'))
    expect(host.querySelector('[data-composer-stat="activity"] button')).toBeNull()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
})


describe('logical-step unknown counts and required turn accounting', () => {
  it('keeps unknown counts visible without creating timing rows, while a known empty log stays empty', async () => {
    const empty = { llmMs: { reportedValue: 0, knownStepCount: 0, complete: false },
      toolMs: { reportedValue: 0, knownCallCount: 0, callCount: 0, complete: true },
      ttftMs: 0, ttftSteps: 0, decodeMs: 0, decodeTokens: 0, decodeSteps: 0 }
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: metrics({ ...empty, turnCount: null, stepCount: null }) }))
    const activity = host.querySelector('[data-composer-stat="activity"]')!
    expect(activity.textContent).toContain(t('dshStatsCounts', { turns: t('dshUsageUnknown'), steps: t('dshUsageUnknown') }))
    expect(activity.querySelector('button')).toBeNull()
    expect(document.querySelector('[data-session-stats-details]')).toBeNull()
    await render(createElement(SessionStatsPills, { ownerKey: sessionUsageOwner, mode: 'detailed', metrics: metrics({ ...empty, turnCount: 0, stepCount: 0,
      llmMs: { ...empty.llmMs, complete: true } }) }))
    expect(host.querySelector('[data-composer-stat="activity"]')).toBeNull()
  })
  it.each(['uncachedInputTokens', 'outputTokens', 'totalTokens'] as const)('withholds a turn panel when required %s is partial or unknown', async key => {
    for (const knownAttemptCount of [0, 1]) {
      const value = ProviderBillingV1Schema.parse({ ...footerBilling, [key]: {
        ...footerBilling[key], reportedValue: knownAttemptCount ? footerBilling[key].reportedValue : 0,
        complete: false, knownAttemptCount },
        ...(key === 'outputTokens' ? { totalTokens: { ...footerBilling.totalTokens, complete: false,
          knownAttemptCount, reportedValue: 0 } } : {}) })
      expect(turnAccountingFromBilling(value)).toBeUndefined()
      await render(createElement(TurnUsagePanel, { ownerKey: turnUsageOwner, billing: value }))
      expect(host.querySelector('[data-analytix-turn-usage]')).toBeNull()
    }
  })
})


function AnchoredPanelFixture({ ownerKey, enabled = true, commits }: {
  ownerKey: string; enabled?: boolean; commits?: Array<{ ownerKey: string; open: boolean }>
}) {
  const panel = useAnchoredPanel<HTMLSpanElement>({ ownerKey, enabled })
  useLayoutEffect(() => { commits?.push({ ownerKey, open: panel.open }) })
  return <span ref={panel.rootRef}>
    <button ref={panel.triggerRef} type="button" aria-expanded={panel.open}
      aria-controls={panel.open ? panel.id : undefined} onClick={() => panel.setOpen(!panel.open)}>Details</button>
    {panel.open ? createPortal(<div id={panel.id} ref={panel.panelRef} role="dialog"
      className={panelCss.panel} style={panel.style}><button type="button">Panel action</button></div>, document.body) : null}
  </span>
}

describe('shared anchored panel owner and nonmodal focus', () => {
  it('masks a changed owner before commit and never reopens through A-B-A or disable-enable', async () => {
    const commits: Array<{ ownerKey: string; open: boolean }> = []
    await render(<AnchoredPanelFixture ownerKey="a" commits={commits} />)
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    commits.length = 0
    await render(<AnchoredPanelFixture ownerKey="b" commits={commits} />)
    expect(commits.every(commit => commit.ownerKey !== 'b' || !commit.open)).toBe(true)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await render(<AnchoredPanelFixture ownerKey="a" commits={commits} />)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await act(async () => host.querySelector<HTMLButtonElement>('button')!.click())
    await render(<AnchoredPanelFixture ownerKey="a" enabled={false} />)
    await render(<AnchoredPanelFixture ownerKey="a" />)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
  it('keeps editor focus on open, returns internal focus on Escape and bounds a tall panel', async () => {
    vi.stubGlobal('innerHeight', 120)
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(400)
    await render(<AnchoredPanelFixture ownerKey="a" />)
    const trigger = host.querySelector<HTMLButtonElement>('button')!
    const editor = document.createElement('textarea'); document.body.append(editor); editor.focus()
    await act(async () => trigger.click())
    expect(document.activeElement).toBe(editor)
    const panel = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(panel.style.top).toBe('12px'); expect(panel.style.maxHeight).toBe('96px')
    panel.querySelector('button')!.focus()
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    await act(async () => document.activeElement!.dispatchEvent(escape))
    expect(escape.defaultPrevented).toBe(true); expect(document.activeElement).toBe(trigger)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await act(async () => trigger.click())
    editor.focus()
    await act(async () => editor.dispatchEvent(new Event('pointerdown', { bubbles: true })))
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(editor); editor.remove()
  })
})


describe('actual turn-stat owner propagation', () => {
  it.each([
    ['thread-a', 'turn-a', 'thread-b', 'turn-b'],
    ['thread-a', 'shared-turn', 'thread-b', 'shared-turn']
  ])('retires %s/%s at the first %s/%s commit without reopening on A-B-A', async (threadA, turnA, threadB, turnB) => {
    const commits: Array<{ owner: string; open: boolean }> = []
    const show = (threadId: string, turnId: string) => {
      const owner = JSON.stringify([threadId, turnId])
      return render(createElement(Profiler, { id: 'turn-stat-owner', onRender: () => {
        commits.push({ owner, open: document.querySelector('[role="dialog"]') !== null })
      } }, createElement(TurnTailView, { ownerKey: owner, processing: false, ended: true, hasOutput: true,
        finalBlock: { ...finalBlock, meta: { turnId } }, billing: footerBilling })))
    }
    await show(threadA, turnA)
    await act(async () => host.querySelector<HTMLButtonElement>('[data-analytix-turn-usage] button')!.click())
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    commits.length = 0
    await show(threadB, turnB)
    // B has valid, identical billing and the same mounted Tail component.
    expect(host.querySelector('[data-analytix-turn-usage] button')).not.toBeNull()
    expect(commits.some(commit => commit.owner === JSON.stringify([threadB, turnB]))).toBe(true)
    expect(commits.every(commit => !commit.open)).toBe(true)
    await show(threadA, turnA)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
  it('keeps missing owner identity as a plain reading and retires an open panel synchronously', async () => {
    const props = { processing: false, ended: true, hasOutput: true, finalBlock, billing: footerBilling }
    await render(createElement(TurnTailView, { ...props, ownerKey: turnUsageOwner }))
    await act(async () => host.querySelector<HTMLButtonElement>('[data-analytix-turn-usage] button')!.click())
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    await render(createElement(TurnTailView, props))
    expect(host.querySelector('[data-analytix-turn-usage]')).not.toBeNull()
    expect(host.querySelector('[data-analytix-turn-usage] button')).toBeNull()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    await render(createElement(TurnTailView, { ...props, ownerKey: turnUsageOwner }))
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
})

describe('whole-session confirmed-zero billing visibility',()=>{
  const tokenKeys=['inputTokens','outputTokens','totalTokens','uncachedInputTokens','cacheReadTokens','cacheWriteTokens','reasoningTokens'] as const
  const zeroBilling=():ProviderBillingV1=>{
    const count={reportedValue:0,knownAttemptCount:1,attemptCount:1,complete:true}
    return ProviderBillingV1Schema.parse({inventoryComplete:true,closed:true,attemptCount:1,settledAttemptCount:1,
      inputTokens:count,outputTokens:count,totalTokens:count,uncachedInputTokens:count,
      cacheReadTokens:count,cacheWriteTokens:count,reasoningTokens:count})
  }
  it('hides only confirmed all-zero usage while retaining independent real activity',async()=>{
    await render(createElement(SessionStatsPills,{ownerKey:sessionUsageOwner,mode:'detailed',billing:zeroBilling(),metrics:metrics()}))
    expect(host.querySelector('[data-composer-stat="usage"]')).toBeNull()
    expect(host.querySelector('[data-composer-stat="activity"]')).not.toBeNull()
    await render(createElement(SessionStatsPills,{ownerKey:sessionUsageOwner,mode:'detailed',billing:billing(1,1,1,{uncached:1,read:0,write:0})}))
    expect(host.querySelector('[data-composer-stat="usage"]')).not.toBeNull()
  })
  it.each(tokenKeys)('keeps zero-valued but unknown or partial %s visible',async key=>{
    for(const knownAttemptCount of [0,1]){
      const value=zeroBilling()
      value[key]={...value[key],knownAttemptCount,complete:false}
      if(key==='inputTokens'||key==='outputTokens')value.totalTokens={...value.totalTokens,knownAttemptCount,complete:false}
      const validated=ProviderBillingV1Schema.parse(value)
      await render(createElement(SessionStatsPills,{ownerKey:sessionUsageOwner,mode:'detailed',billing:validated}))
      expect(host.querySelector('[data-composer-stat="usage"]')).not.toBeNull()
    }
  })
  it('keeps an unsettled zero report visible until the inventory is closed',async()=>{
    const value=ProviderBillingV1Schema.parse({...zeroBilling(),closed:false})
    await render(createElement(SessionStatsPills,{ownerKey:sessionUsageOwner,mode:'detailed',billing:value}))
    expect(host.querySelector('[data-composer-stat="usage"]')).not.toBeNull()
  })
})


describe('context capacity body preserves the actual five-category model', () => {
  function capacity(usedRatio = 0.5, estimated = true, hasMeasuredTotal = true): ContextCapacity {
    return { windowTokens: 10000, usedTokens: usedRatio * 10000, freeTokens: (1 - usedRatio) * 10000,
      usedRatio, freeRatio: 1 - usedRatio, estimated, hasMeasuredTotal,
      categories: ['tools', 'system', 'skills', 'messages', 'other'].map(key => ({
        key: key as ContextCapacity['categories'][number]['key'], tokens: usedRatio * 2000, ratio: usedRatio / 5
      })) }
  }

  it('retains all five categories and free, with mapped segment and legend colors', async () => {
    await render(createElement(ContextCapacityPopover, { capacity: capacity() }))
    const bar = host.querySelector('[role="img"]')!
    const colors = ['rgb(167, 139, 250)', 'rgb(173, 178, 184)', 'rgb(29, 158, 117)', 'rgb(77, 147, 248)', 'rgb(216, 145, 13)']
    expect([...bar.children].map(element => (element as HTMLElement).style.background)).toEqual(colors)
    const rows = [...host.querySelectorAll<HTMLElement>('[class*="capacityRow"]')]
    expect(rows).toHaveLength(6)
    for (const [index, key] of ['tools', 'system', 'skills', 'messages', 'other'].entries()) {
      expect(rows[index]!.textContent).toContain(t(`contextCapacityCat_${key}`))
      expect((rows[index]!.firstElementChild as HTMLElement).style.background).toBe(colors[index])
      expect(rows[index]!.textContent).toContain('10%')
    }
    expect(rows[5]!.textContent).toContain(t('contextCapacityCat_free'))
    expect(rows[5]!.textContent).toContain('50%')
    expect(host.textContent).toContain(t('contextCapacityEstimatedBreakdown'))
  })

  it.each([
    { ratio: 0.74, key: 'contextCapacityShareNote' },
    { ratio: 0.75, key: 'contextCapacityNearLimit' },
    { ratio: 0.9, key: 'contextCapacityOverLimit' }
  ])('retains the real $ratio status and threshold', async ({ ratio, key }) => {
    await render(createElement(ContextCapacityPopover, { capacity: capacity(ratio), thresholdRatio: 0.9 }))
    expect(host.textContent).toContain(t(key))
    expect(host.textContent).toContain(t('contextCapacityThresholdLabel', { percent: '90%' }))
  })

  it('keeps zero categories in the legend and distinguishes measured/estimated totals', async () => {
    const value = capacity(0.5, true, false)
    value.categories[0] = { key: 'tools', tokens: 0, ratio: 0 }
    value.categories[3] = { key: 'messages', tokens: 2000, ratio: 0.2 }
    await render(createElement(ContextCapacityPopover, { capacity: value }))
    expect(host.querySelector('[role="img"]')!.children).toHaveLength(4)
    expect(host.querySelectorAll('[class*="capacityRow"]')).toHaveLength(6)
    expect(host.textContent).toContain(t('contextCapacityEstimatedAll'))
    await render(createElement(ContextCapacityPopover, { capacity: { ...value, estimated: false, hasMeasuredTotal: true } }))
    expect(host.textContent).not.toContain(t('contextCapacityEstimatedAll'))
    expect(host.textContent).not.toContain(t('contextCapacityEstimatedBreakdown'))
  })
})
