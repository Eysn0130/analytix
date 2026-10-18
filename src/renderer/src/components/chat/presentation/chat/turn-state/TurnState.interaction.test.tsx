// @vitest-environment jsdom
import { parse } from 'postcss'
import { act, createElement, Fragment, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../../../../i18n'
import type { ChatBlock, RuntimeDisclosureMetadata, ThreadUsageSnapshot } from '../../../../../agent/types'
import { TurnProcessView } from '../TurnProcessView'
import { TurnStatusView } from '../TurnStatusView'
import { TurnTailView } from '../TurnTailView'
import { RetryItem, TurnMaxTokensItem } from './StatusItems'
import { dispatchAnalytixRuntimeEvent } from '../../../../../agent/analytix-mapper'
import { buildThreadEventSink } from '../../../../../store/chat-store-runtime'
import type { ChatState, ChatStoreSet } from '../../../../../store/chat-store-types'
import { isStrictPublicRuntimeSseIpcPayload, projectPublicRuntimeSseBlock } from '@shared/public-runtime-sse'
import { PublicRuntimeEventFilter } from '@shared/public-runtime-content'

let root: Root, container: HTMLDivElement
const t = (key: string, options?: Record<string, unknown>) => i18n.t('common:' + key, options)
const usage: ThreadUsageSnapshot = { inputTokens: 4, outputTokens: 6, reasoningTokens: 0, cachedTokens: 0,
  cacheMissTokens: 4, cacheHitRate: 0, totalTokens: 10, costUsd: null, costCny: null, priceConfigured: false,
  tokenEconomySavingsTokens: 0, turns: 1 }
async function render(element: Parameters<Root['render']>[0]) { await act(async () => root.render(element)) }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove()
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('actual shared Turn state owners', () => {
  it('renders settled whole-second fragments and focuses the native keyboard disclosure owner', async () => {
    function Controlled() {
      const [open, setOpen] = useState(false)
      return createElement(TurnProcessView, { open, onToggle: () => setOpen(value => !value), hasContent: true,
        durationMs: 3200, status: 'completed' })
    }
    await render(createElement(Controlled))
    const button = container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!
    expect(button.tagName).toBe('BUTTON'); expect(button.tabIndex).toBe(0)
    expect(button.textContent).toBe('Completed in 3s')
    expect(button.getAttribute('aria-expanded')).toBe('false')
    button.focus(); expect(document.activeElement).toBe(button)
    await act(async () => button.click())
    expect(button.getAttribute('aria-expanded')).toBe('true'); expect(document.activeElement).toBe(button)
    expect(container.querySelector('[role="status"]')!.textContent).toBe(t('processed'))
    expect(container.querySelector('[role="status"]')!.textContent).not.toContain('3')
  })
  it.each(['failed', 'aborted'] as const)('keeps real %s state persistent without a success duration or collapse control', async status => {
    const onToggle = vi.fn()
    await render(createElement(TurnProcessView, { open: false, onToggle, hasContent: true, durationMs: 3200, status, collapsible: false }))
    const header = container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!
    expect(header.tagName).toBe('BUTTON'); expect(header.disabled).toBe(true); expect(header.hasAttribute('data-open')).toBe(true)
    expect(header.getAttribute('aria-expanded')).toBe('true')
    expect(header.textContent).toBe(t(status === 'failed' ? 'dshProcessFailed' : 'dshTurnStopped'))
    expect(header.textContent).not.toContain('3s'); expect(header.querySelector('svg')).toBeNull()
    await act(async () => header.click())
    expect(onToggle).not.toHaveBeenCalled()
  })
  it('keeps no-time completion and unknown terminal state distinct', async () => {
    await render(createElement(TurnProcessView, { open: false, onToggle: vi.fn(), hasContent: true, status: 'completed' }))
    expect(container.querySelector('[data-analytix-turn-process]')!.textContent).toBe(t('processed'))
    await render(createElement(TurnProcessView, { open: false, onToggle: vi.fn(), hasContent: true, durationMs: 6000 }))
    expect(container.querySelector('[data-analytix-turn-process]')!.textContent).toBe(t('dshTurnProcess'))
    expect(container.querySelector('[role="status"]')).toBeNull()
  })
  it('respects supplied nonfoldable policy and no-content state', async () => {
    const onToggle = vi.fn()
    await render(createElement(TurnProcessView, { open: false, onToggle, hasContent: true, status: 'completed', collapsible: false }))
    const header = container.querySelector<HTMLButtonElement>('[data-analytix-turn-process]')!
    expect(header.hasAttribute('data-open')).toBe(true); expect(header.disabled).toBe(true)
    expect(header.getAttribute('aria-expanded')).toBe('true')
    await act(async () => header.click()); expect(onToggle).not.toHaveBeenCalled()
    await render(createElement(TurnProcessView, { open: true, onToggle, hasContent: false, status: 'completed' }))
    expect(header.disabled).toBe(true); expect(header.hasAttribute('data-open')).toBe(false)
    expect(container.querySelector('[aria-expanded]')).toBeNull()
    await act(async () => header.click()); expect(onToggle).not.toHaveBeenCalled()
  })
  it('expands the actual conversation summary and labels estimated tokens and the real inventory count', async () => {
    const block: ChatBlock = { kind: 'compaction', id: 'actual', status: 'success', summaryKind: 'conversation', summary: 'The actual summary.', detail: 'The actual summary.',
      meta: { shadowedItemCount: 7, replacedTokens: 1234, tokenCountKind: 'estimated', tokenEstimatorVersion: 1 } }
    await render(createElement(TurnStatusView, { block }))
    const button = container.querySelector<HTMLButtonElement>('[data-analytix-compaction] button')!
    expect(button.disabled).toBe(false)
    expect(button.textContent).toContain('7 items compacted')
    expect(button.textContent).toContain('≈ 1234 tokens replaced (estimated)')
    expect(container.querySelector('.compactionBody')).toBeNull()
    await act(async () => button.click())
    expect(container.textContent).toContain('The actual summary.')
    expect(button.getAttribute('aria-expanded')).toBe('true')
  })
  it.each(['success', 'error'] as const)('keeps %s compaction unexpandable when only pinned constraints are available', async status => {
    const meta: RuntimeDisclosureMetadata = {}; Reflect.set(meta, 'replacedTokens', 0)
    const block: ChatBlock = { kind: 'compaction', id: 'c', status, summary: 'Context compacted',
      messagesBefore: 99999, messagesAfter: 12345, detail: 'Public pinned constraints are not a summary', meta }
    await render(createElement(TurnStatusView, { block }))
    const button = container.querySelector<HTMLButtonElement>('[data-analytix-compaction] button')!
    expect(button.textContent).toContain('0 tokens replaced')
    expect(button.textContent).not.toContain('99999'); expect(button.textContent).not.toContain('12345')
    expect(button.disabled).toBe(true); expect(button.hasAttribute('aria-expanded')).toBe(false)
    if (status === 'success') expect(button.textContent).toContain('Summary unavailable for this record')
    await act(async () => button.click())
    expect(container.textContent).not.toContain('Public pinned constraints are not a summary')
    expect(container.querySelector('[data-disclosure-row]')).toBeNull()
  })
  it.each([undefined, -1, 1.2, NaN, Infinity])('omits malformed compaction token %s and preserves the unavailable-summary state', async count => {
    const meta: RuntimeDisclosureMetadata = {}; Reflect.set(meta, 'replacedTokens', count)
    await render(createElement(TurnStatusView, { block: { kind: 'compaction', id: 'c', status: 'success', summary: 'actual', detail: 'constraints', meta } }))
    expect(container.textContent).not.toContain('tokens replaced')
    expect(container.querySelector<HTMLButtonElement>('button')!.disabled).toBe(true)
  })
  it.each(['running', 'success', 'error', 'unknown'] as const)('keeps %s compaction honest without an available detail', async status => {
    await render(createElement(TurnStatusView, { block: { kind: 'compaction', id: 'c', status, summary: 'Actual status' } }))
    const row = container.querySelector('[data-analytix-compaction]')!
    expect(row.getAttribute('data-state')).toBe(status)
    expect(row.querySelector<HTMLButtonElement>('button')!.disabled).toBe(true)
    expect(row.querySelector('[aria-expanded]')).toBeNull()
    expect(Boolean(row.querySelector('[data-shimmer]'))).toBe(status === 'running')
  })
  it.each(['scheduled', 'started', 'cancelled'] as const)('animates only supplied scheduled phase, retaining %s semantics', async phase => {
    await render(createElement(RetryItem, { message: 'Actual safe retry failure', attempt: 2, phase, delayMs: 3500, t }))
    const row = container.querySelector('[data-analytix-retry]')!
    expect(row.getAttribute('data-retry-state')).toBe(phase)
    expect(Boolean(row.querySelector('[data-shimmer]'))).toBe(phase === 'scheduled')
    expect(row.textContent).toContain('Request attempt 2 / ?'); expect(row.textContent).toContain('3500 ms')
    expect(row.querySelector('summary')!.tagName).toBe('SUMMARY')
    await act(async () => row.querySelector<HTMLElement>('summary')!.click())
    expect(row.hasAttribute('open')).toBe(true)
  })
  it('retains one native disclosure across actual scheduled to started to cancelled phase changes', async () => {
    await render(createElement(RetryItem, { message: 'Actual failure', attempt: 2, phase: 'scheduled', t }))
    const row = container.querySelector<HTMLDetailsElement>('[data-analytix-retry]')!
    await act(async () => row.querySelector<HTMLElement>('summary')!.click())
    expect(row.open).toBe(true); expect(row.querySelector('[data-shimmer]')).not.toBeNull()
    await render(createElement(RetryItem, { message: 'Actual failure', attempt: 2, phase: 'started', t }))
    expect(container.querySelector('[data-analytix-retry]')).toBe(row)
    expect(row.open).toBe(true); expect(row.querySelector('[data-shimmer]')).toBeNull()
    expect(row.querySelector('[role="status"]')!.textContent).toContain('Retry started')
    await render(createElement(RetryItem, { message: 'Actual failure', attempt: 2, phase: 'cancelled', t }))
    expect(container.querySelector('[data-analytix-retry]')).toBe(row)
    expect(row.open).toBe(true); expect(row.querySelector('[data-shimmer]')).toBeNull()
    expect(row.querySelector('[role="status"]')!.textContent).toContain('Retry cancelled')
  })
  it('keeps the current retry producer phase unknown and static rather than synthesizing a schedule or retry index', async () => {
    vi.useFakeTimers()
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'r', text: 'Provider request is retrying',
      createdAt: '2026-10-07T00:00:00Z', meta: { providerRetry: { attempt: 2, maxAttempt: 3 } } } }))
    const row = container.querySelector('[data-analytix-retry]')!
    expect(row.getAttribute('data-retry-state')).toBe('unknown')
    expect(row.textContent).toContain('Retry phase unavailable'); expect(row.textContent).toContain('Request attempt 2 / 3')
    expect(row.querySelector('[data-shimmer]')).toBeNull(); expect(vi.getTimerCount()).toBe(0)
    const text = row.textContent; await act(async () => vi.advanceTimersByTime(60_000)); expect(row.textContent).toBe(text)
  })
  it.each(['response_received', 'pre_send', 'post_send', 'future_stage'])('retires neutral request telemetry while preserving unknown explicit %s diagnostics', async stage => {
    const meta: RuntimeDisclosureMetadata = { turnId: 'real-turn' }; Reflect.set(meta, 'stage', stage)
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'stage', text: 'DO_NOT_CLASSIFY_PROSE', meta } }))
    const row = container.querySelector<HTMLDetailsElement>('[data-runtime-diagnostics-stage]')
    if (stage !== 'future_stage') {
      expect(row).toBeNull(); expect(container.textContent).not.toContain('DO_NOT_CLASSIFY_PROSE')
      expect(container.querySelector('button')).toBeNull()
      return
    }
    expect(row!.tagName).toBe('DETAILS'); expect(row!.getAttribute('aria-label')).toBe(t('dshProviderRequestDetails'))
    expect(row!.textContent).toContain(stage); expect(row!.textContent).toContain('real-turn')
    expect(row!.textContent).toContain('DO_NOT_CLASSIFY_PROSE'); expect(row!.open).toBe(false)
    const summary = row!.querySelector<HTMLElement>('summary')!
    await act(async () => summary.click()); expect(row!.open).toBe(true)
    expect(summary.textContent).toBe(t('dshProviderRequestDetails'))
    expect(row!.textContent).toContain('Provider stage: future_stage')
  })

  it('keeps persistent safe error text/code and both detail plus metadata under its own native button', async () => {
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'error', severity: 'error', text: 'Actual public failure',
      code: 'http_400', detail: 'Actual safe detail', meta: { providerError: { kind: 'transport', status: 400 } } } }))
    const row = container.querySelector('[data-analytix-turn-error]')!
    expect(row.querySelector('[role="status"]')!.textContent).toContain('Actual public failure')
    expect(row.querySelector('[role="status"]')!.textContent).toContain('http_400')
    const button = row.querySelector<HTMLButtonElement>('[data-disclosure-row]')!
    expect(button.tagName).toBe('BUTTON'); expect(button.getAttribute('aria-expanded')).toBe('false')
    expect(button.textContent).toBe(t('dshStatusDetails')); expect(row.textContent).not.toContain('transport')
    await act(async () => button.click()); expect(button.getAttribute('aria-expanded')).toBe('true')
    const body = document.getElementById(button.getAttribute('aria-controls')!)!
    expect(body.textContent).toContain('Actual safe detail'); expect(row.textContent).toContain('transport')
  })
  it('keeps explicit error severity and code when retry diagnostics also exist', async () => {
    const meta: RuntimeDisclosureMetadata = { providerRetry: { attempt: 3, maxAttempt: 3 } }; Reflect.set(meta, 'stage', 'post_send')
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'mixed', severity: 'error', code: 'http_503',
      text: 'Actual error with retry diagnostics', meta } }))
    expect(container.querySelector('[data-analytix-turn-error]')).not.toBeNull()
    expect(container.querySelector('[data-analytix-retry]')).toBeNull()
    expect(container.textContent).toContain('http_503')
    expect(container.textContent).toContain('Actual error with retry diagnostics')
  })
  it('retains provider-only failure diagnostics inside a default-closed detail', async () => {
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'provider-only', severity: 'error', text: 'Actual failure',
      meta: { providerError: { kind: 'transport', status: 503 } } } }))
    const row = container.querySelector('[data-analytix-turn-error]')!
    expect(row.textContent).toContain('Actual failure'); expect(row.textContent).not.toContain('transport')
    const button = row.querySelector<HTMLButtonElement>('[data-disclosure-row]')!
    expect(button.textContent).toBe(t('dshStatusDetails'))
    await act(async () => button.click())
    expect(row.textContent).toContain('transport'); expect(row.textContent).toContain('503')
  })
  it('retains an actual code on a non-error retry input without inventing a phase', async () => {
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'retry-code', code: 'provider_retry',
      text: 'Actual retry', meta: { providerRetry: { attempt: 2 } } } }))
    const row = container.querySelector('[data-analytix-retry]')!
    expect(row.getAttribute('data-retry-state')).toBe('unknown')
    expect(row.querySelector('code')!.textContent).toBe('provider_retry')
  })
  it('keeps the max-tokens pure notice independent and never guesses it from code/text/errors', async () => {
    await render(createElement(TurnMaxTokensItem, { t }))
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).not.toBeNull()
    expect(container.querySelector('[data-analytix-turn-error]')).toBeNull()
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'e', code: 'max_tokens',
      text: 'Output token limit reached', severity: 'error' } }))
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
    expect(container.querySelector('[data-analytix-turn-error]')).not.toBeNull()
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'e', text: 'Response received' } }))
    expect(container.querySelector('[data-runtime-diagnostics-stage]')).toBeNull()
    expect(container.textContent).toContain('Response received')
  })
  it('ticks only the current mounted Turn, refreshes immediately on start replacement and releases the clock at stop', async () => {
    vi.useFakeTimers(); vi.setSystemTime(20_000)
    let ownerRenders = 0
    function Owner({ start, processing }: { start: number; processing: boolean }) {
      ownerRenders++
      return createElement(Fragment, null,
        createElement('div', { 'data-history': true }, createElement(TurnTailView, { processing: false, startedAt: 1000, hasOutput: true })),
        createElement('div', { 'data-current': true }, createElement(TurnTailView, { processing, running: processing, startedAt: start, hasOutput: true })))
    }
    await render(createElement(Owner, { start: 10_000, processing: true }))
    expect(container.querySelector('[data-history] [data-analytix-live-progress]')).toBeNull()
    const live = container.querySelector('[data-current] [data-analytix-live-progress]')!
    expect(live.textContent).toContain('10s')
    await act(async () => vi.advanceTimersByTime(2000))
    expect(live.textContent).toContain('12s'); expect(ownerRenders).toBe(1)
    expect(live.querySelector('[role="status"]')!.textContent).toBe(t('dshRunning'))
    expect(live.querySelector('[aria-live="off"]')).not.toBeNull()
    await render(createElement(Owner, { start: 21_000, processing: true }))
    expect(live.textContent).toContain('1s'); expect(live.textContent).not.toContain('12s')
    await act(async () => vi.advanceTimersByTime(1000)); expect(live.textContent).toContain('2s')
    await render(createElement(Owner, { start: 21_000, processing: false }))
    expect(container.querySelector('[data-analytix-live-progress]')).toBeNull(); expect(vi.getTimerCount()).toBe(0)
    await act(async () => vi.advanceTimersByTime(60_000)); expect(ownerRenders).toBe(3)
  })
  it.each([undefined, NaN, Infinity, 21_000])('does not invent a running clock or allocate a timer for start %s', async startedAt => {
    vi.useFakeTimers(); vi.setSystemTime(20_000)
    await render(createElement(TurnTailView, { processing: true, running: true, startedAt, hasOutput: false }))
    const live = container.querySelector('[data-analytix-live-progress]')!
    expect(live.querySelector('[aria-live="off"]')).toBeNull(); expect(vi.getTimerCount()).toBe(0)
  })
  it('does not turn a legacy last-result snapshot into a physical-attempt bill', async () => {
    await render(createElement(TurnTailView, { processing: false, ended: true, hasOutput: true,
      finalBlock: { kind: 'assistant', id: 'final', text: 'Real closing reply' }, usage: { ...usage, totalTokens: 0 } }))
    expect(container.querySelector('[data-analytix-turn-usage]')).toBeNull()
    await render(createElement(TurnTailView, { processing: true, hasOutput: true, usage }))
    expect(container.querySelector('[data-analytix-turn-usage]')).toBeNull()
    expect(container.textContent).not.toMatch(/TTFT|decode|tokens per second/i)
  })
  it.each([NaN, Infinity, -1])('omits unavailable total usage %s', async totalTokens => {
    await render(createElement(TurnTailView, { processing: false, hasOutput: true, usage: { ...usage, totalTokens } }))
    expect(container.querySelector('[data-analytix-turn-usage]')).toBeNull()
  })
  it('requires an actual final response for existing fork/rollback actions', async () => {
    const fork = { busy: false, run: vi.fn() }, rollback = { busy: false, run: vi.fn() }
    await render(createElement(TurnTailView, { processing: false, hasOutput: true, fork, rollback }))
    expect(container.querySelector('button')).toBeNull()
    await render(createElement(TurnTailView, { processing: false, hasOutput: true, fork, rollback,
      ended: true, finalBlock: { kind: 'assistant', id: 'a', text: 'Real public final' } }))
    const action = container.querySelector<HTMLButtonElement>('[aria-label="' + t('forkFromAssistantResponse') + '"]')!
    await act(async () => action.click()); expect(fork.run).toHaveBeenCalledOnce(); expect(rollback.run).not.toHaveBeenCalled()
  })
  it('retains source CSS sticky compaction, numeric fragments, focus and reduced motion with admitted theme tokens', async () => {
    const fs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
    const relative = './TurnState.module.css'
    const url = new URL(relative, import.meta.url)
    expect(url.protocol).toBe('file:')
    const css = fs.readFileSync(decodeURIComponent(url.pathname), 'utf8')
    const parsed = parse(css)
    const declarations = (selector: string) => {
      const values: Record<string, string> = {}
      parsed.walkRules(selector, rule => { rule.walkDecls(declaration => { values[declaration.prop] = declaration.value }) })
      return values
    }
    const running = declarations('.running')
    const darkRunning = declarations(":global([data-theme='dark']) .running")
    expect(running.color).toBe('var(--dsw-alias-label-deep-diving)')
    expect(running['--dsw-alias-label-shimmer']).toBe('var(--dsw-alias-label-deep-diving-shimmer)')
    expect(running['--dsw-alias-label-deep-diving']).toBe('color-mix(in srgb, rgb(65, 118, 230) 70%, rgb(23, 37, 84))')
    expect(running['--dsw-alias-label-deep-diving-shimmer']).toBe('color-mix(in srgb, rgb(65, 118, 230) 30%, rgb(23, 37, 84))')
    expect(darkRunning['--dsw-alias-label-deep-diving']).toBe('color-mix(in srgb, rgb(86, 134, 254) 55%, rgb(173, 178, 184))')
    expect(darkRunning['--dsw-alias-label-deep-diving-shimmer']).toBe('color-mix(in srgb, rgb(147, 197, 253) 65%, rgb(122, 170, 255))')
    expect(declarations('.runningDivider').background).toBe('color-mix(in srgb, var(--dsw-alias-border-l1) 75%, var(--dsw-alias-border-l2))')
    expect(declarations('.tail').gap).toBe('16px')
    expect(css).toContain('.compactionRow:has(.compactionBody) .compactionButton')
    expect(css).toContain('font-variant-numeric: tabular-nums')
    expect(css).toContain(':focus-visible')
    let reduced = 0; parsed.walkAtRules('media', rule => { if (rule.params === '(prefers-reduced-motion: reduce)') reduced++ })
    expect(reduced).toBeGreaterThan(0)
    expect(css).not.toMatch(/@font-face|https?:/i)
    expect(css).toContain("url('./running-whale@2x.png')")
    expect(css).toContain(':global(html[data-motion-reduced="true"]) .runningWhaleAnimated { display: none; }')
  })
})


describe('Host retry wire to actual store and status leaf', () => {
  it('preserves one disclosure across real phases and never starts a scheduled zero countdown', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-07T00:00:00Z'))
    let state = { activeThreadId: 'leaf-thread', currentTurnId: 'leaf-turn', currentTurnUserId: null,
      busy: true, liveAssistant: '', blocks: [], lastSeq: 0, error: null,
      turnStartedAtByUserId: {}, turnDurationByUserId: {} } as unknown as ChatState
    const set: ChatStoreSet = partial => { state = { ...state, ...(typeof partial === 'function' ? partial(state) : partial) } }
    const sink = buildThreadEventSink(set, () => state, { threadId: 'leaf-thread' })
    const phase = async (value: 'scheduled' | 'started' | 'cancelled', seq: number) => {
      const event = { kind: 'pipeline_stage', seq, threadId: 'leaf-thread', turnId: 'leaf-turn',
        timestamp: '2026-10-07T00:00:00.123456789Z', stage: 'provider_retrying', label: 'Retrying provider stream',
        attempt: 2, maxAttempt: 3, details: { retry: { schemaVersion: 1, source: 'provider_retry', phase: value,
          logicalSequence: 7, outerAttempt: 2, ...(value === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1 } : {}),
          ...(value === 'cancelled' ? { cancelReason: 'deadline_exceeded' } : {}) } } }
      const decision = projectPublicRuntimeSseBlock(`id: ${seq}\nevent: pipeline_stage\ndata: ${JSON.stringify(event)}`,
        'leaf-thread', new PublicRuntimeEventFilter())
      expect(decision?.status).toBe('emit')
      if (decision?.status !== 'emit') throw new Error('invalid retry fixture')
      expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'leaf-retry', events: [decision.event] })).toBe(true)
      await dispatchAnalytixRuntimeEvent(decision.event as Parameters<typeof dispatchAnalytixRuntimeEvent>[0], sink, async () => undefined)
      const block = state.blocks[0]
      if (block.kind !== 'system') throw new Error('missing retry status')
      await render(createElement(TurnStatusView, { block }))
    }
    await phase('scheduled', 40)
    const row = container.querySelector<HTMLDetailsElement>('[data-analytix-retry]')!
    expect(row.getAttribute('data-retry-state')).toBe('scheduled')
    expect(row.textContent).toContain('Scheduled delay 0 ms')
    expect(row.querySelector('[data-retry-deadline-unix-ms]')?.textContent).toContain('0s')
    expect(vi.getTimerCount()).toBe(0)
    await act(async () => row.querySelector<HTMLElement>('summary')!.click())
    expect(row.open).toBe(true)
    await act(async () => vi.advanceTimersByTime(60_000))
    expect(row.getAttribute('data-retry-state')).toBe('scheduled')
    await phase('started', 41)
    expect(container.querySelector('[data-analytix-retry]')).toBe(row); expect(row.open).toBe(true)
    expect(row.getAttribute('data-retry-state')).toBe('started')
    expect(row.querySelector('[data-shimmer]')).toBeNull()
    expect(row.querySelector('[data-retry-deadline-unix-ms]')).toBeNull()
    await phase('cancelled', 42)
    expect(container.querySelector('[data-analytix-retry]')).toBe(row); expect(row.open).toBe(true)
    expect(row.getAttribute('data-retry-state')).toBe('cancelled')
    expect(row.querySelector('[data-shimmer]')).toBeNull(); expect(vi.getTimerCount()).toBe(0)
    expect(state).toMatchObject({ busy: true, currentTurnId: 'leaf-turn' })
  })
  it('counts down only a supplied future Host deadline without inferring a start', async () => {
    vi.useFakeTimers(); vi.setSystemTime(10_000)
    await render(createElement(RetryItem, { message: 'Retry', phase: 'scheduled', delayMs: 2000,
      deadlineUnixMs: 12_000, t }))
    const row = container.querySelector('[data-analytix-retry]')!
    expect(row.querySelector('[data-retry-deadline-unix-ms]')?.textContent).toContain('2s')
    expect(vi.getTimerCount()).toBe(1)
    await act(async () => vi.advanceTimersByTime(2000))
    expect(row.querySelector('[data-retry-deadline-unix-ms]')?.textContent).toContain('0s')
    expect(row.getAttribute('data-retry-state')).toBe('scheduled')
    expect(vi.getTimerCount()).toBe(0)
  })
  it('keeps actual errors above retry metadata and length observations below turn-terminal authority', async () => {
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'error-retry', severity: 'error',
      code: 'http_503', text: 'Actual failure', meta: { providerRetry: { attempt: 2,
        lifecycle: { schemaVersion: 1, source: 'provider_retry', logicalSequence: 7, outerAttempt: 2, phase: 'started' } } } } }))
    expect(container.querySelector('[data-analytix-turn-error]')).not.toBeNull()
    expect(container.textContent).toContain('http_503'); expect(container.querySelector('[data-analytix-retry]')).toBeNull()
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'output-cap', text: 'Response received',
      meta: { stage: 'response_received', providerOutputCap: { stopReason: 'length', streamCompleted: false, logicalSequence: 7 } } } }))
    expect(container.querySelector('[data-runtime-diagnostics-stage="response_received"]')).toBeNull()
    expect(container.querySelector('[data-analytix-turn-max-tokens]')).toBeNull()
    expect(container.querySelector('[data-analytix-turn-error]')).toBeNull()
  })
})

describe('discarded reply history notice', () => {
  it.each(['en', 'zh'])('shows a static %s historical fact with no execution or diagnostics affordance', async locale => {
    await i18n.changeLanguage(locale)
    await render(createElement(TurnStatusView, { block: { kind: 'system', id: 'history:discard', text: 'DO_NOT_DISPLAY_RAW_TEXT',
      createdAt: '2026-10-10T00:00:02Z', code: 'history_user_input_discarded', meta: { turnId: 'turn_attempt' } } }))
    const notice = container.querySelector('[data-user-input-discard-history]')!
    expect(notice.textContent).toContain(t('userInputLateDiscardedHistory'))
    expect(notice.querySelector('time')?.getAttribute('datetime')).toBe('2026-10-10T00:00:02Z')
    expect(notice.textContent).not.toContain('DO_NOT_DISPLAY_RAW_TEXT')
    expect(notice.textContent).not.toContain('history_user_input_discarded')
    expect(container.querySelector('button, input, textarea, details, [data-analytix-turn-error], [data-runtime-diagnostics-stage]')).toBeNull()
    expect(notice.getAttribute('aria-live')).toBe('off')
  })
})
