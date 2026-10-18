// @vitest-environment jsdom
import { act, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { TrajectoryPanel, type TrajectoryPanelProps } from './TrajectoryPanel'
import { TrajectorySource } from './trajectory-source'
import { errorEvent, readyEvent, retryEvent, stepEvent, toolEvent, userEvent } from './trajectory-test-fixtures'
import { trajectoryStatusLabel } from './TrajectoryTimeline'
import { TrajectoryTable, trajectoryGroups } from './TrajectoryTable'
import type { TrajectoryRecord } from './trajectory-adapter'
let root: Root, host: HTMLDivElement
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  await i18n.changeLanguage('en')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
function sourceFixture() { const source = new TrajectorySource(); source.bindThread('trajectory-thread'); return source }
const button = (label: string) => [...host.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === label)!
const click = async (node: HTMLElement) => { await act(async () => node.click()) }
const render = async (source: TrajectorySource, props: Partial<TrajectoryPanelProps> = {}) => {
  await act(async () => root.render(<TrajectoryPanel threadId="trajectory-thread" source={source} {...props} />))
}
function trackFixture() {
  const track = host.querySelector<HTMLElement>('[aria-label^="Drag to select"]')!
  vi.spyOn(track, 'getBoundingClientRect').mockReturnValue({ left: 0, top: 0, x: 0, y: 0, width: 1000, height: 50, right: 1000, bottom: 50, toJSON() {} })
  return track
}
async function pointer(track: HTMLElement, type: string, x: number, button = 0) {
  const event = new MouseEvent(type, { bubbles: true, clientX: x, button })
  Object.defineProperty(event, 'pointerId', { value: 1 })
  await act(async () => { track.dispatchEvent(event) })
}
function scrollFixture(height = 120) {
  const element = host.querySelector<HTMLElement>('[data-trajectory-scroll]')!
  Object.defineProperty(element, 'clientHeight', { configurable: true, value: height })
  Object.defineProperty(element, 'scrollHeight', { configurable: true, get: () => Number(element.getAttribute('aria-rowcount')) * 30 })
  return element
}
async function scrollTo(element: HTMLElement, top: number) { await act(async () => { element.scrollTop = top; element.dispatchEvent(new Event('scroll', { bubbles: true })) }) }
function tableRecord(kind: TrajectoryRecord['kind'], seq: number, patch: Partial<TrajectoryRecord> = {}): TrajectoryRecord {
  return { id: `public-record-${seq}`, threadId: 'trajectory-thread', turnId: 'trajectory-turn', kind,
    lane: kind === 'tool' ? 'tool' : kind === 'user' ? 'input' : 'model', firstSeq: seq, lastSeq: seq,
    title: kind === 'tool' ? 'read_file' : kind, status: 'recorded', at: null, startedAt: null, endedAt: null, ...patch }
}
async function renderTable(records: readonly TrajectoryRecord[], props: Partial<ComponentProps<typeof TrajectoryTable>> = {}) {
  await act(async () => root.render(<TrajectoryTable records={records} groupIds={trajectoryGroups(records).byRecord}
    selectedId={null} focusedIds={null} onSelect={vi.fn()} t={(key, fallback) => String(i18n.t(`dshTrajectory.${key}`, { defaultValue: fallback }))}
    collapsedTurns={new Set()} collapsedModels={new Set()} collapsedCalls={new Set()}
    onToggleTurn={vi.fn()} onToggleModel={vi.fn()} onToggleCalls={vi.fn()} hasEarlier={false} onLoadEarlier={vi.fn()}
    loading={false} historyStartSeq={0} reveal={null} onRevealed={vi.fn()} {...props} />))
}
describe('independent trajectory working surface', () => {
  it.each(['en', 'zh'])('shows independent public retry/error details in %s without changing step identity', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    source.observe('trajectory-thread', [stepEvent('logical_step_started', 1), retryEvent(2), errorEvent(3)])
    await render(source)
    await click(host.querySelector<HTMLButtonElement>('[data-group="model"]')!)
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(2)
    const selectKind = async (kind: string) => {
      const record = source.getSnapshot('trajectory-thread').records.find(record => record.kind === kind)!
      await click([...host.querySelectorAll<HTMLButtonElement>('button[data-record-id]')].find(node => node.dataset.recordId === record.id)!)
    }
    const field = (label: string) => [...host.querySelectorAll('aside dl > div')].find(node => node.querySelector('dt')?.textContent === label)?.querySelector('dd')?.textContent
    await selectKind('retry')
    expect(field(language === 'en' ? 'Phase' : '阶段')).toBe(language === 'en' ? 'Scheduled' : '已安排')
    expect(field(language === 'en' ? 'Attempt' : '尝试次数')).toBe('2')
    expect(field(language === 'en' ? 'Maximum attempts' : '最多尝试次数')).toBe('4')
    expect(field(language === 'en' ? 'Retry delay' : '重试延迟')).toBe('0 ms')
    expect(field(language === 'en' ? 'Retry source' : '重试来源')).toBe(language === 'en' ? 'Provider retry' : '模型服务重试')
    expect(field(language === 'en' ? 'Logical step' : '逻辑步骤')).toBeUndefined()
    expect(host.querySelector('[data-tab="timing"]')).toBeNull()
    await selectKind('error')
    expect(field(language === 'en' ? 'Public code' : '公开代码')).toBe('provider_unavailable')
    expect(field(language === 'en' ? 'Phase' : '阶段')).toBe(language === 'en' ? 'Live' : '实时')
    await click(host.querySelector<HTMLButtonElement>('[data-tab="record"]')!)
    const record = JSON.parse(host.querySelector('aside pre')!.textContent!)
    expect(record.error).toEqual({ code: 'provider_unavailable', phase: 'live', attempt: 2, maxAttempt: 4 })
    expect(record).not.toHaveProperty('details'); expect(record).not.toHaveProperty('logicalStepId')
  })
  it.each(['en', 'zh'])('retains public content and real step/call summaries when folded in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    const additional = (seq: number, callId: string, toolName: string) => {
      const event = toolEvent(false, seq)
      return { ...event, itemId: callId, item: { ...event.item, id: callId, callId, toolName } }
    }
    source.observe('trajectory-thread', [userEvent(1), stepEvent('logical_step_started', 2), stepEvent('logical_step_assembled', 3),
      toolEvent(false, 4), toolEvent(true, 5), additional(6, 'second-call', 'read_file'), additional(7, 'third-call', 'list_files')])
    await render(source)
    const model = host.querySelector<HTMLButtonElement>('[data-group="model"]')!
    await click(model)
    expect(model.querySelector('[data-fold-summary]')?.textContent).toContain(language === 'en' ? 'Steps: 1' : '步骤: 1')
    const calls = host.querySelector<HTMLButtonElement>('[data-group="calls"]')!
    await click(calls)
    const summary = calls.querySelector('[data-fold-summary]')!.textContent!
    expect(summary).toContain(language === 'en' ? 'Calls: 3' : '调用: 3')
    expect(summary.match(/read_file/gu)).toHaveLength(1); expect(summary).toContain('list_files')
    const turn = host.querySelector<HTMLButtonElement>('[data-turn-id]')!
    await click(turn)
    expect(turn.querySelector('[data-fold-summary]')?.textContent).toContain(language === 'en' ? 'Hidden records: 4' : '隐藏记录: 4')
    expect(turn.querySelector('[data-fold-summary]')?.textContent).toContain(language === 'en' ? 'Steps: 1' : '步骤: 1')
    expect(turn.querySelector('[data-fold-summary]')?.textContent).toContain(language === 'en' ? 'Calls: 3' : '调用: 3')
    const retained = source.getSnapshot('trajectory-thread').records.find(record => record.kind === 'user')!
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(1)
    expect(host.querySelector('button[data-record-id]')?.getAttribute('data-record-id')).toBe(retained.id)
    expect(host.querySelector('button[data-record-id]')?.textContent).toContain('Public request 1')
    await click(turn)
    const reopenedModel = host.querySelector<HTMLButtonElement>('[data-group="model"]')!
    expect(reopenedModel.isConnected).toBe(true); expect(reopenedModel.getAttribute('aria-expanded')).toBe('false')
    expect(reopenedModel.querySelector('[data-fold-summary]')).not.toBeNull()
  })
  it('opens with 50 resident records, extends locally by 50, and keeps history as live records arrive', async () => {
    const source = sourceFixture()
    source.observe('trajectory-thread', Array.from({ length: 105 }, (_, index) => userEvent(index + 1)))
    await render(source)
    expect(host.querySelectorAll('[data-point="true"]')).toHaveLength(50)
    expect(host.textContent).toContain('History completeness unknown')
    await click(button('Load 50 older records'))
    expect(host.querySelectorAll('[data-point="true"]')).toHaveLength(100)
    await act(async () => source.observe('trajectory-thread', [userEvent(106)]))
    expect(host.querySelectorAll('[data-point="true"]')).toHaveLength(101)
  })
  it('supports drag ranges, anchored wheel zoom, right-button panning and reset without a transport', async () => {
    const source = sourceFixture()
    source.observe('trajectory-thread', Array.from({ length: 50 }, (_, index) => userEvent(index + 1)))
    await render(source)
    const track = trackFixture()
    await pointer(track, 'pointerdown', 200); await pointer(track, 'pointermove', 500); await pointer(track, 'pointerup', 500)
    expect(host.querySelector('[data-dimmed="true"]')).not.toBeNull()
    await act(async () => track.dispatchEvent(new WheelEvent('wheel', { bubbles: true, cancelable: true, clientX: 500, deltaY: -900 })))
    expect(host.querySelectorAll('[data-timeline-record]').length).toBeLessThan(50)
    const first = host.querySelector('[data-timeline-record]')?.getAttribute('data-timeline-record')
    await pointer(track, 'pointerdown', 500, 2); await pointer(track, 'pointermove', 700, 2); await pointer(track, 'pointerup', 700, 2)
    expect(host.querySelector('[data-timeline-record]')?.getAttribute('data-timeline-record')).not.toBe(first)
    await click(button('Reset view'))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(50)
    expect(host.querySelector('[data-dimmed="true"]')).toBeNull()
  })
  it('folds real model sections and calls, exposes public tabs, and keeps protected Inspect explicit', async () => {
    const source = sourceFixture(), onInspectTool = vi.fn()
    const renderToolInspector = vi.fn(() => <div data-test-protected-seat />)
    source.observe('trajectory-thread', [userEvent(1), stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_assembled', 3, 2),
      toolEvent(false, 4), toolEvent(true, 7), stepEvent('logical_step_closed', 8)])
    await render(source, { onInspectTool, renderToolInspector })
    const model = host.querySelector<HTMLButtonElement>('[data-group="model"]')!
    await click(model)
    expect(host.querySelector('button[data-record-id][data-lane="model"]')).toBeNull()
    await click(model)
    await click(host.querySelector<HTMLButtonElement>('[aria-label="Collapse calls"]')!)
    expect(host.querySelector('button[data-record-id][data-lane="tool"]')).toBeNull()
    await click(host.querySelector<HTMLButtonElement>('[aria-label="Expand calls"]')!)
    const toolRow = host.querySelector<HTMLButtonElement>('button[data-record-id][data-lane="tool"]')!
    await click(toolRow)
    expect(host.querySelector('[role="tab"][data-tab="timing"]')).not.toBeNull()
    expect(host.querySelector('[role="tab"][data-tab="result"]')).not.toBeNull()
    expect(renderToolInspector).not.toHaveBeenCalled()
    await click(button('Inspect tool'))
    expect(onInspectTool).toHaveBeenCalledExactlyOnceWith({ threadId: 'trajectory-thread', turnId: 'trajectory-turn', callId: 'trajectory-call', toolCallItemId: 'trajectory-tool', toolResultItemId: 'trajectory-result' })
    expect(host.querySelector('[data-test-protected-seat]')).not.toBeNull()
    await click(host.querySelector<HTMLButtonElement>('[role="tab"][data-tab="timing"]')!)
    await click(host.querySelector<HTMLButtonElement>('[role="tab"][data-tab="overview"]')!)
    expect(host.querySelector('[data-test-protected-seat]')).toBeNull()
    await click(host.querySelector<HTMLButtonElement>('button[data-record-id][data-lane="input"]')!)
    await click(host.querySelector<HTMLButtonElement>('[role="tab"][data-tab="content"]')!)
    expect(host.querySelector('pre')?.textContent).toBe('Public request 1')
  })
  it('follows a reader near the tail, stops after scrolling away, and anchors local prepends', async () => {
    const source = sourceFixture()
    source.observe('trajectory-thread', Array.from({ length: 105 }, (_, index) => userEvent(index + 1)))
    await render(source)
    const scroll = scrollFixture()
    await scrollTo(scroll, scroll.scrollHeight - scroll.clientHeight)
    await act(async () => source.observe('trajectory-thread', [userEvent(106)]))
    expect(scroll.scrollTop).toBe(scroll.scrollHeight - scroll.clientHeight)
    await scrollTo(scroll, 90)
    const before = scroll.scrollTop
    await act(async () => source.observe('trajectory-thread', [userEvent(107)]))
    expect(scroll.scrollTop).toBe(before)
    // A near-top user scroll loads one local earlier page and retains its prior first record.
    const count = Number(scroll.getAttribute('aria-rowcount'))
    await scrollTo(scroll, 30)
    expect(Number(scroll.getAttribute('aria-rowcount'))).toBeGreaterThan(count)
    expect(scroll.scrollTop).toBeGreaterThan(30)
  })
  it('precisely locates an older real call, expands its turn/calls and acknowledges only after reveal', async () => {
    const source = sourceFixture(), complete = vi.fn()
    source.observe('trajectory-thread', [toolEvent(false, 1), toolEvent(true, 2), ...Array.from({ length: 105 }, (_, index) => userEvent(index + 10))])
    await render(source, { onFocusRequestComplete: complete })
    const request = { requestId: 'request-1', threadId: 'trajectory-thread', turnId: 'trajectory-turn', callId: 'trajectory-call', toolResultItemId: 'trajectory-result' }
    await render(source, { focusRequest: request, onFocusRequestComplete: complete })
    expect(complete).toHaveBeenCalledExactlyOnceWith('request-1', 'found')
    expect(host.querySelector('[aria-label="Inspector"]')).not.toBeNull()
    expect(host.querySelector('button[data-record-id][data-lane="tool"][aria-pressed="true"]')).not.toBeNull()
    await render(source, { focusRequest: { ...request, requestId: 'request-2' }, onFocusRequestComplete: complete })
    expect(complete).toHaveBeenLastCalledWith('request-2', 'found')
    await render(source, { focusRequest: { ...request, requestId: 'request-3', callId: 'not-loaded-call' }, onFocusRequestComplete: complete })
    expect(complete).toHaveBeenLastCalledWith('request-3', 'not_loaded')
  })
  it('keeps manual collapse and scroll as data changes, and retires view ownership on thread switch', async () => {
    const source = sourceFixture(), released = vi.fn(), subscribe = source.subscribe
    vi.spyOn(source, 'subscribe').mockImplementation((threadId, listener) => { const off = subscribe(threadId, listener); return () => { released(threadId); off() } })
    source.observe('trajectory-thread', [userEvent(1)])
    const other = source.retainThread('other-thread')
    source.observe('other-thread', [userEvent(2, 'other-thread', 'Other public request')])
    await render(source)
    await click(host.querySelector<HTMLButtonElement>('button[data-record-id]')!)
    const turn = host.querySelector<HTMLButtonElement>('[data-turn-id]')!
    await click(turn)
    const scroll = scrollFixture(30)
    await scrollTo(scroll, 90)
    await act(async () => source.observe('trajectory-thread', [userEvent(2)]))
    expect(turn.getAttribute('aria-expanded')).toBe('false')
    await render(source, { threadId: 'other-thread' })
    other()
    expect(released).toHaveBeenCalledWith('trajectory-thread')
    expect(host.querySelector('[aria-label="Inspector"]')).toBeNull()
    expect(host.textContent).toContain('Other public request')
    await act(async () => root.render(null))
    expect(released).toHaveBeenCalledWith('other-thread')
  })
  it('unlocks each local earlier page even when the AND search has zero matching ledger rows', async () => {
    const source = sourceFixture()
    source.observe('trajectory-thread', Array.from({ length: 155 }, (_, index) => userEvent(index + 1)))
    await render(source)
    const search = host.querySelector<HTMLInputElement>('input[type="search"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, 'no-matching-term')
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(0)
    await click(button('Load 50 older records'))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(100)
    await click(button('Load 50 older records'))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(150)
  })
  it('folds separate model runs independently within the same turn', async () => {
    const source = sourceFixture(), secondId = 'a560256d-4adc-401c-9c0a-b5f46052d513'
    const second = (stage: 'logical_step_started' | 'logical_step_assembled', seq: number) => {
      const event = stepEvent(stage, seq)
      return { ...event, details: { ...event.details, logicalStepId: secondId } }
    }
    source.observe('trajectory-thread', [stepEvent('logical_step_started', 1, 1), stepEvent('logical_step_assembled', 2, 2), toolEvent(false, 3), toolEvent(true, 4), second('logical_step_started', 5), second('logical_step_assembled', 6)])
    await render(source)
    const models = host.querySelectorAll<HTMLButtonElement>('[data-group="model"]')
    expect(models).toHaveLength(2)
    await click(models[0])
    expect(models[0].getAttribute('aria-expanded')).toBe('false')
    expect(models[1].getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('button[data-record-id][data-lane="model"]')).toHaveLength(1)
  })

  it('keeps an existing tail reader at the bottom when its viewport shrinks', async () => {
    const observers: (() => void)[] = []
    vi.stubGlobal('ResizeObserver', class { constructor(callback: () => void) { observers.push(callback) } observe() {} disconnect() {} })
    const source = sourceFixture()
    source.observe('trajectory-thread', Array.from({ length: 50 }, (_, index) => userEvent(index + 1)))
    await render(source)
    const scroll = scrollFixture(120)
    await scrollTo(scroll, scroll.scrollHeight - 120)
    Object.defineProperty(scroll, 'clientHeight', { configurable: true, value: 60 })
    await act(async () => { for (const resize of observers) resize() })
    expect(scroll.scrollTop).toBe(scroll.scrollHeight - 60)
  })

  it.each(['turn', 'model', 'calls'] as const)('reveals search hits through a collapsed %s and restores its prior fold after clearing', async group => {
    const source = sourceFixture()
    source.observe('trajectory-thread', [userEvent(1), stepEvent('logical_step_started', 2), stepEvent('logical_step_assembled', 3), toolEvent(false, 4), toolEvent(true, 5)])
    await render(source)
    const record = source.getSnapshot('trajectory-thread').records.find(value => value.kind === (group === 'model' ? 'step' : 'tool'))!
    const row = () => [...host.querySelectorAll<HTMLButtonElement>('button[data-record-id]')].find(node => node.dataset.recordId === record.id)
    const control = () => host.querySelector<HTMLButtonElement>(group === 'turn' ? '[data-turn-id]' : `[data-group="${group}"]`)!
    const search = host.querySelector<HTMLInputElement>('input[type="search"]')!
    const query = async (value: string) => {
      await act(async () => {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, value)
        search.dispatchEvent(new Event('input', { bubbles: true }))
      })
    }
    expect(row()).toBeDefined()
    await click(control())
    expect(row()).toBeUndefined()
    await query('  \t ')
    expect(row()).toBeUndefined(); expect(control().getAttribute('aria-expanded')).toBe('false')
    const term = group === 'model' ? 'model' : 'read_file'
    await query(term)
    expect(row()).toBeDefined(); expect(control().getAttribute('aria-expanded')).toBe('true')
    await query('no-matching-term')
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(0)
    await query(term)
    expect(row()).toBeDefined()
    await query('')
    expect(row()).toBeUndefined(); expect(control().getAttribute('aria-expanded')).toBe('false')
  })

  it.each([
    ['ready', '就绪'], ['running', '进行中'], ['completed', '已完成'], ['failed', '失败'],
    ['blocked', '已阻止'], ['cancelled', '已取消'], ['unknown', '未知'], ['recorded', '已记录']
  ] as const)('localizes %s in the Chinese ledger, Inspector and timeline aria without changing public data', async (status, translated) => {
    await i18n.changeLanguage('zh')
    const source = sourceFixture()
    let events: unknown[]
    if (status === 'ready') events = [readyEvent(1)]
    else if (status === 'running') events = [toolEvent(false, 1)]
    else if (status === 'unknown') events = [stepEvent('logical_step_closed', 1)]
    else if (status === 'recorded') events = [userEvent(1)]
    else {
      const finished = toolEvent(true, 2)
      if (!('output' in finished.item)) throw new Error('Finished tool fixture must contain an output')
      events = [toolEvent(false, 1), { ...finished, item: { ...finished.item,
        status: status === 'completed' ? 'completed' : 'failed', isError: status !== 'completed',
        output: { ...finished.item.output, status } } }]
    }
    source.observe('trajectory-thread', events)
    const record = source.getSnapshot('trajectory-thread').records[0]
    expect(record?.status).toBe(status)
    await render(source)
    const row = host.querySelector<HTMLButtonElement>('button[data-record-id]')!
    expect(row.textContent).toContain(translated); expect(row.textContent).not.toContain(status)
    const marker = [...host.querySelectorAll<HTMLButtonElement>('[data-timeline-record]')].find(node => node.dataset.timelineRecord === record.id)!
    expect(marker.getAttribute('aria-label')).toMatch(new RegExp(` · ${translated}$`))
    await click(row)
    const field = [...host.querySelectorAll('aside dl > div')].find(node => node.querySelector('dt')?.textContent === '状态')!
    expect(field.querySelector('dd')?.textContent).toBe(translated)
    await click(host.querySelector<HTMLButtonElement>('[role="tab"][data-tab="record"]')!)
    expect(JSON.parse(host.querySelector('aside pre')!.textContent!).status).toBe(status)
    expect(source.getSnapshot('trajectory-thread').records[0].status).toBe(status)
    if (status === 'unknown') expect(trajectoryStatusLabel('future-unrecognized-status', (key, fallback) => String(i18n.t(`dshTrajectory.${key}`, { defaultValue: fallback })))).toBe('未知')
  })

  it.each(['en', 'zh'])('provides the 30 used trajectory labels and every status directly in %s resources', language => {
    const keys = [
      'controls', 'expandTurns', 'collapseTurns', 'expandCalls', 'collapseCalls', 'retainedOnly',
      'timelineControls', 'resetView', 'noTiming', 'decode', 'summary', 'preview', 'publicText', 'timing',
      'publicResult', 'publicRecord', 'callItem', 'resultItem', 'copy', 'copied', 'footnotes', 'table',
      'copyFailed', 'resizeInspector', 'ambiguous', 'argumentsWithheld', 'tokens', 'wrap', 'textTruncated', 'compaction'
    ]
    expect(keys).toHaveLength(30)
    for (const key of [...keys, 'retry', 'error', 'retrySource', 'phase', 'attempt', 'maxAttempt', 'retryDelay', 'code', 'steps', 'messages', 'hiddenRecords', 'moreToolNames',
      'emptyRecords', 'noTimingWithSequence', 'noTimelineEntries',
      ...['provider_retry', 'transport_reconnect'].map(source => `retrySourceValue.${source}`),
      ...['scheduled', 'started', 'cancelled', 'setup', 'highest_seq', 'replay', 'live'].map(phase => `phaseValue.${phase}`),
      ...['ready', 'running', 'completed', 'failed', 'blocked', 'cancelled', 'unknown', 'recorded'].map(status => `state.${status}`)]) {
      // Direct lookup cannot pass through defaultValue or a fallback language.
      const value = i18n.getResource(language, 'common', `dshTrajectory.${key}`)
      expect(typeof value, `${language}:dshTrajectory.${key}`).toBe('string')
      expect(value.trim(), `${language}:dshTrajectory.${key}`).not.toBe('')
    }
  })

})

describe('retained public rows in collapsed trajectory sections', () => {
  it('keeps the first actual content row and its selection identity, then restores every record once', async () => {
    const content = tableRecord('user', 2, { text: 'a'.repeat(159) + '😀tail' })
    const records = [tableRecord('step', 1), content, tableRecord('message', 3, { text: 'Later assistant content' }), tableRecord('tool', 4)]
    const collapsedTurns = new Set([content.turnId]), onSelect = vi.fn()
    await renderTable(records, { collapsedTurns, onSelect })
    const row = host.querySelector<HTMLButtonElement>('button[data-record-id]')!
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(1)
    expect(row.dataset.recordId).toBe(content.id)
    expect(row.querySelector('[role="cell"]:last-child > span:first-child')?.textContent).toBe(content.text)
    expect(host.querySelector('[data-fold-summary]')?.textContent).toContain('Hidden records: 3')
    await click(row)
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(content); expect(onSelect.mock.calls[0][0]).toBe(content)
    await renderTable(records, { collapsedTurns, onSelect, selectedId: content.id })
    expect(host.querySelector('button[data-record-id][aria-pressed="true"]')?.getAttribute('data-record-id')).toBe(content.id)
    await renderTable(records, { selectedId: content.id })
    expect([...host.querySelectorAll<HTMLElement>('button[data-record-id]')].map(row => row.dataset.recordId)).toEqual(records.map(record => record.id))
    expect(host.querySelector('button[data-record-id][aria-pressed="true"]')?.getAttribute('data-record-id')).toBe(content.id)
    await renderTable([content], { collapsedTurns })
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(1)
    expect(host.querySelector('[data-fold-summary]')).toBeNull()
    await renderTable([records[0], records[3]], { collapsedTurns })
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(0)
    expect(host.querySelector('[data-fold-summary]')?.textContent).toContain('Hidden records: 2')
  })
  it('retains the original assistant message in a model run and reveals a hidden record only after expansion', async () => {
    const first = tableRecord('message', 2, { text: 'First assistant 😀' }), later = tableRecord('message', 3, { text: 'Later assistant' })
    const records = [tableRecord('step', 1), first, later], key = trajectoryGroups(records).modelIds[0]
    const collapsedModels = new Set([key]), onSelect = vi.fn(), onRevealed = vi.fn(), reveal = { recordId: later.id, version: 1 }
    await renderTable(records, { collapsedModels, onSelect, onRevealed })
    const row = host.querySelector<HTMLButtonElement>('button[data-record-id]')!
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(1); expect(row.dataset.recordId).toBe(first.id)
    expect(row.textContent).toContain(first.text)
    expect(host.querySelector('[data-fold-summary]')?.textContent).toContain('Messages: 1')
    await click(row); expect(onSelect.mock.calls[0][0]).toBe(first)
    await renderTable(records, { collapsedModels, reveal, onRevealed })
    expect(onRevealed).not.toHaveBeenCalled()
    await renderTable(records, { reveal, onRevealed, selectedId: later.id })
    expect([...host.querySelectorAll<HTMLElement>('button[data-record-id]')].map(row => row.dataset.recordId)).toEqual(records.map(record => record.id))
    expect(document.activeElement?.getAttribute('data-record-id')).toBe(later.id)
    expect(onRevealed).toHaveBeenCalledExactlyOnceWith(reveal)
    await renderTable(records, { reveal, onRevealed, selectedId: later.id })
    expect(onRevealed).toHaveBeenCalledTimes(1)
    await renderTable([records[0]], { collapsedModels })
    expect(host.querySelector('button[data-record-id]')).toBeNull()
  })
  it('counts retained rows in virtualization and reveals the original offscreen row once', async () => {
    const records = Array.from({ length: 120 }, (_, index) => [
      tableRecord('user', index * 2 + 1, { turnId: `turn-${index}`, text: `Content ${index}` }),
      tableRecord('step', index * 2 + 2, { turnId: `turn-${index}` })
    ]).flat()
    const collapsedTurns = new Set(records.map(record => record.turnId)), first = records[0], onRevealed = vi.fn()
    await renderTable(records, { collapsedTurns, onRevealed })
    expect(host.querySelector('[data-trajectory-scroll]')?.getAttribute('aria-rowcount')).toBe('240')
    expect(host.querySelectorAll('button[data-record-id]').length).toBeLessThan(120)
    expect([...host.querySelectorAll<HTMLElement>('button[data-record-id]')].some(row => row.dataset.recordId === first.id)).toBe(false)
    const reveal = { recordId: first.id, version: 1 }
    await renderTable(records, { collapsedTurns, onRevealed, reveal, selectedId: first.id })
    const row = [...host.querySelectorAll<HTMLElement>('button[data-record-id]')].find(row => row.dataset.recordId === first.id)!
    expect(document.activeElement).toBe(row); expect(row.getAttribute('aria-pressed')).toBe('true')
    expect(row.parentElement?.getAttribute('aria-rowindex')).toBe('2')
    expect(onRevealed).toHaveBeenCalledExactlyOnceWith(reveal)
    await renderTable(records, { collapsedTurns: new Set([...collapsedTurns].filter(id => id !== first.turnId)), onRevealed, reveal, selectedId: first.id })
    expect(host.querySelector('[data-trajectory-scroll]')?.getAttribute('aria-rowcount')).toBe('242')
    expect(host.querySelector('button[data-record-id][aria-pressed="true"]')?.getAttribute('data-record-id')).toBe(first.id)
    expect(onRevealed).toHaveBeenCalledTimes(1)
  })
  it.each(['en', 'zh'])('bounds tool names and summary codepoints while retaining full content and exact counts in %s', async language => {
    await i18n.changeLanguage(language)
    const content = tableRecord('user', 1, { text: 'a'.repeat(159) + '😀tail' })
    const tools = Array.from({ length: 8 }, (_, index) => tableRecord('tool', index + 2, { title: `tool_${index}_${'😀'.repeat(60)}` }))
    tools.push(tableRecord('tool', 10, { title: tools[0].title }))
    const records = [content, ...tools], collapsedTurns = new Set([content.turnId])
    await renderTable(records, { collapsedTurns })
    const summary = host.querySelector('[data-fold-summary]')!.textContent!
    expect(summary).toContain(language === 'en' ? 'Calls: 9' : '调用: 9')
    expect(summary).toContain(language === 'en' ? 'More tool names: 5' : '更多工具名称: 5')
    expect(summary.match(/tool_\d_/gu)).toHaveLength(3); expect(summary).not.toContain('tool_3_')
    expect([...summary].length).toBeLessThanOrEqual(160)
    expect([...summary].every(point => point.length === 2 || point.charCodeAt(0) < 0xd800 || point.charCodeAt(0) > 0xdfff)).toBe(true)
    const row = host.querySelector('button[data-record-id]')!
    expect(row.getAttribute('data-record-id')).toBe(content.id)
    expect(row.querySelector('[role="cell"]:last-child > span:first-child')?.textContent).toBe(content.text)
    await renderTable(records)
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(10)
    expect(host.querySelectorAll('[data-fold-summary]')).toHaveLength(0)
  })
})

describe('trajectory empty-state semantics', () => {
  const label = (key: string) => String(i18n.t(`dshTrajectory.${key}`))
  const ledgerStatus = () => host.querySelector('[data-trajectory-scroll] [role="status"]')?.textContent
  const timelineStatus = () => host.querySelector(`section[aria-label="${label('timeline')}"] [role="status"]`)?.textContent
  const query = async (value: string) => {
    const search = host.querySelector<HTMLInputElement>('input[type="search"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, value)
      search.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  it.each(['en', 'zh'])('describes only the currently loaded empty records for unknown and partial history in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    await render(source)
    expect(source.getSnapshot('trajectory-thread').history).toBe('unknown')
    expect(ledgerStatus()).toBe(label('emptyRecords')); expect(timelineStatus()).toBe(label('emptyRecords'))
    await query('unmatched-search')
    expect(ledgerStatus()).toBe(label('emptyRecords'))
    await act(async () => { source.beginReplay('trajectory-thread', 1).fail() })
    expect(source.getSnapshot('trajectory-thread').history).toBe('partial')
    expect(ledgerStatus()).toBe(label('emptyRecords')); expect(timelineStatus()).toBe(label('emptyRecords'))
    expect(host.textContent).not.toContain(label('noMatches'))
    expect(host.textContent).not.toContain(label('noTimingWithSequence'))
  })
  it.each(['en', 'zh'])('keeps initial loading separate from a settled empty replay in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture(), replay = source.beginReplay('trajectory-thread', 0)
    await render(source)
    expect(ledgerStatus()).toBe(label('loading')); expect(timelineStatus()).toBe(label('loading'))
    expect(host.textContent).not.toContain(label('emptyRecords')); expect(host.textContent).not.toContain(label('noMatches'))
    await act(async () => replay.complete())
    expect(source.getSnapshot('trajectory-thread').history).toBe('complete')
    expect(ledgerStatus()).toBe(label('emptyRecords')); expect(timelineStatus()).toBe(label('emptyRecords'))
  })
  it.each(['en', 'zh'])('shows no matches only for filtered records and retains the unfiltered timeline through loading in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    source.observe('trajectory-thread', [userEvent(1)])
    const record = source.getSnapshot('trajectory-thread').records[0]
    await render(source)
    await query('unmatched-search')
    expect(ledgerStatus()).toBe(label('noMatches')); expect(timelineStatus()).toBeUndefined()
    expect(host.querySelectorAll('button[data-record-id]')).toHaveLength(0)
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(1)
    expect(host.querySelector('[data-timeline-record]')?.getAttribute('data-search-match')).toBe('false')
    let replay: ReturnType<TrajectorySource['beginReplay']>
    await act(async () => { replay = source.beginReplay('trajectory-thread', 1) })
    expect(ledgerStatus()).toBe(label('loading')); expect(timelineStatus()).toBeUndefined()
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(1)
    await act(async () => replay.fail())
    expect(ledgerStatus()).toBe(label('noMatches'))
    await query('  \t ')
    expect(ledgerStatus()).toBeUndefined()
    expect(host.querySelector('button[data-record-id]')?.getAttribute('data-record-id')).toBe(record.id)
    expect(source.getSnapshot('trajectory-thread').records[0]).toBe(record)
  })
  it.each(['en', 'zh'])('distinguishes missing timestamps from real sequence records without synthesizing time in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    source.observe('trajectory-thread', [stepEvent('logical_step_closed', 1)])
    const record = source.getSnapshot('trajectory-thread').records[0]
    expect(record).toMatchObject({ at: null, startedAt: null, endedAt: null })
    await render(source)
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(1); expect(timelineStatus()).toBeUndefined()
    await click(button(label('duration')))
    expect(timelineStatus()).toBe(label('noTimingWithSequence'))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(0)
    expect(host.querySelector('button[data-record-id]')?.getAttribute('data-record-id')).toBe(record.id)
    expect(host.textContent).toContain(`${label('unknownTime')} (1)`)
    await click(button(label('duration')))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(1); expect(timelineStatus()).toBeUndefined()
    expect(source.getSnapshot('trajectory-thread').records[0]).toBe(record)
    expect(record).toMatchObject({ at: null, startedAt: null, endedAt: null })
  })
  it.each(['en', 'zh'])('keeps timestamped message-only records distinct from empty or untimed events in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture(), message = tableRecord('message', 1, { at: '2026-10-15T00:00:01.000Z', text: 'Public assistant message' })
    // A stable display snapshot exercises the existing message-exclusion rule, without changing an event contract.
    const snapshot = { ...source.getSnapshot('trajectory-thread'), records: [message], loadedFromSeq: 1, loadedThroughSeq: 1,
      completeThroughSeq: 1, history: 'complete' as const }
    const getSnapshot = vi.spyOn(source, 'getSnapshot').mockReturnValue(snapshot)
    await render(source)
    expect(ledgerStatus()).toBeUndefined(); expect(timelineStatus()).toBe(label('noTimelineEntries'))
    expect(host.querySelector('button[data-record-id]')?.getAttribute('data-record-id')).toBe(message.id)
    await click(button(label('duration')))
    expect(timelineStatus()).toBe(label('noTimelineEntries'))
    expect(host.textContent).not.toContain(label('noTimingWithSequence')); expect(host.textContent).not.toContain(label('emptyRecords'))
    const mixed = { ...snapshot, records: [message, tableRecord('step', 2)], loadedThroughSeq: 2, completeThroughSeq: 2 }
    getSnapshot.mockReturnValue(mixed)
    await render(source)
    expect(timelineStatus()).toBe(label('noTimingWithSequence'))
    expect(host.querySelectorAll('[data-timeline-record]')).toHaveLength(0)
    expect(source.getSnapshot('trajectory-thread').records[0].at).toBe(message.at)
    getSnapshot.mockReturnValue({ ...mixed, history: 'loading' })
    await render(source)
    expect(ledgerStatus()).toBe(label('loading')); expect(timelineStatus()).toBe(label('loading'))
    expect(host.querySelector('button[data-record-id]')?.getAttribute('data-record-id')).toBe(message.id)
  })
  it.each(['en', 'zh'])('loads an evicted prefix and returns to latest with bounded resident records in %s', async language => {
    await i18n.changeLanguage(language)
    const source = sourceFixture()
    const events = Array.from({ length: 2001 }, (_, index) => userEvent(index + 1))
    source.observe('trajectory-thread', events)
    const reader = vi.fn(async (_through: number, accept: (events: readonly unknown[]) => void) => { accept(events) })
    source.setHistoryReader('trajectory-thread', reader)
    await render(source)
    for (let index = 0; index < 40; index++) await click(button(language === 'en' ? 'Load 50 older records' : '加载更早的 50 条记录'))
    expect(reader).toHaveBeenCalledOnce()
    expect(host.querySelectorAll('[data-point="true"]')).toHaveLength(1)
    expect(host.textContent).toContain('Public request 1')
    expect(host.textContent).toContain(language === 'en' ? 'Bounded history window' : '有界历史窗口')
    await click(button(language === 'en' ? 'Return to latest' : '返回最新轨迹'))
    expect(host.querySelectorAll('[data-point="true"]')).toHaveLength(50)
    expect(source.getSnapshot('trajectory-thread').records.at(-1)?.firstSeq).toBe(2001)
  })
  it('shows unavailable earlier history after replay failure while retaining a retry entry', async () => {
    const source = sourceFixture()
    source.observe('trajectory-thread', [userEvent(10)])
    source.setHistoryReader('trajectory-thread', async () => { throw new Error('replay unavailable') })
    await render(source)
    await click(button('Load 50 older records'))
    expect(host.textContent).toContain('Earlier public history is currently unavailable')
    expect(button('Load 50 older records')).toBeDefined()
    expect(source.getSnapshot('trajectory-thread').records.map(record => record.firstSeq)).toEqual([10])
  })

})
