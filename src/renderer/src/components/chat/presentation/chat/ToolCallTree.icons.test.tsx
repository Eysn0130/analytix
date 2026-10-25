// @vitest-environment jsdom
// Expected SVG paths are pinned to DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
// Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger.
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock, RuntimeChildMetadata, ToolBlock } from '../../../../agent/types'
import i18n from '../../../../i18n'
import { useChatStore } from '../../../../store/chat-store'
import { ToolCallTree, type ProcessLocalRows } from './ToolCallTree'
import { ChatGroupSeat } from './ChatGroupSeat'
import { MessageBubble } from '../../message-timeline-bubbles'
import { groupProcessSections, presentationPolicyFor } from './chat-view-model'
import { processDisplayBlockSchemaV1 } from '../../../../../../../packages/runtime/src/contracts/protected-process-local-display'

type GlyphCase = {
  name: string
  activity: 'plan' | 'subagents' | 'questions' | 'webFetch' | 'webSearch'
  toolName: string
  toolKind: NonNullable<ToolBlock['toolKind']>
  paths: readonly Record<string, string>[]
}
const glyphCases: readonly GlyphCase[] = [
  {
    name: 'plan uses Plan', activity: 'plan', toolName: 'update_plan', toolKind: 'tool_call',
    paths: [
      { d: 'M4.9375 5.90295H11.0625', stroke: 'currentColor' },
      { d: 'M4.9375 9.02991H8.27841', stroke: 'currentColor' },
      { d: 'M12.5 1.32617C13.3039 1.32617 14 1.95171 14 2.77637V7.61328L13 8.68164V2.77637C13 2.55186 12.8007 2.32617 12.5 2.32617H3.5C3.1993 2.32617 3 2.55186 3 2.77637V13.2246C3.00044 13.4489 3.19963 13.6738 3.5 13.6738H8.32812L7.39258 14.6738H3.5C2.69637 14.6738 2.00042 14.0489 2 13.2246V2.77637C2 1.95171 2.69613 1.32617 3.5 1.32617H12.5Z', fill: 'currentColor' },
      { d: 'M8.97212 14.3693C9.17511 14.5723 9.37811 14.7753 9.5811 14.9783C9.67012 14.8953 9.75914 14.8123 9.84815 14.7293C11.4505 13.2352 13.0528 11.7411 14.6551 10.247C14.7441 10.164 14.8331 10.081 14.9221 9.99803C14.5989 9.6748 14.2756 9.35157 13.9524 9.02834C13.8694 9.11736 13.7864 9.20637 13.7034 9.29539C12.2093 10.8977 10.7152 12.5 9.22113 14.1023C9.13813 14.1913 9.05513 14.2803 8.97212 14.3693Z', fill: 'currentColor' },
      { d: 'M11.6323 13.7841C11.6323 14.0395 11.6323 14.295 11.6323 14.5504C11.6812 14.5523 11.7301 14.5543 11.779 14.5562C12.659 14.5913 13.539 14.6263 14.419 14.6614C14.4679 14.6633 14.5168 14.6653 14.5657 14.6672C14.5657 14.3339 14.5657 14.0006 14.5657 13.6672C14.5168 13.6692 14.4679 13.6711 14.419 13.6731C13.539 13.7081 12.659 13.7432 11.779 13.7783C11.7301 13.7802 11.6812 13.7821 11.6323 13.7841Z', fill: 'currentColor' }
    ]
  },
  {
    name: 'subagents use AgentPreset', activity: 'subagents', toolName: 'delegate_task', toolKind: 'subagent',
    paths: [
      { d: 'M6.51867 12.3282C7.29816 12.6011 8.16475 12.6514 9.02269 12.4216C9.57879 12.2726 10.0784 12.0185 10.5087 11.6888C10.7819 12.0555 11.1606 12.3304 11.5913 12.4805C10.9688 13.029 10.2149 13.4478 9.35911 13.6771C8.13946 14.0038 6.90632 13.8971 5.82126 13.4533C6.15821 13.1562 6.4021 12.7652 6.51867 12.3282ZM9.17629 2.89409C11.1101 3.34433 12.739 4.81872 13.2889 6.87043C13.4219 7.3665 13.4811 7.8649 13.4774 8.35466C13.0924 8.13213 12.6422 8.01837 12.1741 8.05276L12.1711 8.05257C12.1539 7.77199 12.109 7.48889 12.0334 7.20684C11.6363 5.72533 10.5048 4.6372 9.13549 4.22844C9.25559 3.87667 9.29214 3.49087 9.22309 3.09892C9.2108 3.02922 9.19451 2.96108 9.17629 2.89409ZM4.7311 3.89107L4.78302 4.11879C4.87648 4.4488 5.04146 4.74263 5.25579 4.98896C3.98078 6.01355 3.35848 7.72904 3.8089 9.41059C3.81828 9.44559 3.82866 9.48025 3.83885 9.51479C3.38217 9.61268 2.98548 9.84137 2.68107 10.1556C2.63414 10.022 2.5897 9.88632 2.55244 9.74726C1.93301 7.43489 2.86717 5.07173 4.71504 3.76697L4.7311 3.89107Z', fill: 'currentColor' },
      { d: 'M7.99136 5.28105C8.87501 5.28105 9.59136 4.56471 9.59136 3.68105C9.59136 2.7974 8.87501 2.08105 7.99136 2.08105C7.1077 2.08105 6.39136 2.7974 6.39136 3.68105C6.39136 4.56471 7.1077 5.28105 7.99136 5.28105Z', stroke: 'currentColor' },
      { d: 'M3.94009 12.9417C4.82374 12.9417 5.54009 12.2254 5.54009 11.3417C5.54009 10.458 4.82374 9.7417 3.94009 9.7417C3.05643 9.7417 2.34009 10.458 2.34009 11.3417C2.34009 12.2254 3.05643 12.9417 3.94009 12.9417Z', stroke: 'currentColor' },
      { d: 'M12.0851 12.9417C12.9688 12.9417 13.6851 12.2254 13.6851 11.3417C13.6851 10.458 12.9688 9.7417 12.0851 9.7417C11.2015 9.7417 10.4851 10.458 10.4851 11.3417C10.4851 12.2254 11.2015 12.9417 12.0851 12.9417Z', stroke: 'currentColor' }
    ]
  },
  {
    name: 'questions use Question', activity: 'questions', toolName: 'request_user_input', toolKind: 'tool_call',
    paths: [
      { d: 'M8 14.5C11.5899 14.5 14.5 11.5899 14.5 8C14.5 4.41015 11.5899 1.5 8 1.5C4.41015 1.5 1.5 4.41015 1.5 8C1.5 11.5899 4.41015 14.5 8 14.5Z', stroke: 'currentColor' },
      { d: 'M5.75 6.69646C5.75 6.29865 5.88196 5.90976 6.12919 5.57899C6.37643 5.24821 6.72783 4.99041 7.13896 4.83817C7.5501 4.68593 8.0025 4.6461 8.43895 4.72371C8.87541 4.80132 9.27632 4.99289 9.59099 5.27419C9.90566 5.55549 10.12 5.91388 10.2068 6.30406C10.2936 6.69423 10.249 7.09866 10.0787 7.4662C9.90843 7.83373 9.62004 8.14787 9.25003 8.36889C9.19476 8.4019 9.13803 8.43262 9.08004 8.46099C8.52566 8.73217 8 9.20817 8 9.82532', stroke: 'currentColor' },
      { d: 'M8 10.7416V11.7416', stroke: 'currentColor' }
    ]
  },
  {
    name: 'webFetch uses Browse', activity: 'webFetch', toolName: 'web_fetch', toolKind: 'tool_call',
    paths: [
      { d: 'M4.9375 5.90295H11.0625', stroke: 'currentColor' },
      { d: 'M4.9375 9.02991H8.27841', stroke: 'currentColor' },
      { d: 'M12.5 1.32617C13.3039 1.32617 14 1.95171 14 2.77637V13.2246C13.9996 14.0489 13.3036 14.6738 12.5 14.6738H3.5C2.69637 14.6738 2.00042 14.0489 2 13.2246V2.77637C2 1.95171 2.69613 1.32617 3.5 1.32617H12.5ZM3.5 2.32617C3.1993 2.32617 3 2.55186 3 2.77637V13.2246C3.00044 13.4489 3.19963 13.6738 3.5 13.6738H12.5C12.8004 13.6738 12.9996 13.4489 13 13.2246V2.77637C13 2.55186 12.8007 2.32617 12.5 2.32617H3.5Z', fill: 'currentColor' }
    ]
  },
  {
    name: 'webSearch retains Globe', activity: 'webSearch', toolName: 'web_search', toolKind: 'tool_call',
    paths: [
      { d: 'M7.99986 14.0887C11.3626 14.0887 14.0886 11.3627 14.0886 7.99998C14.0886 4.63727 11.3626 1.91125 7.99986 1.91125C4.63715 1.91125 1.91113 4.63727 1.91113 7.99998C1.91113 11.3627 4.63715 14.0887 7.99986 14.0887Z', stroke: 'currentColor' },
      { d: 'M2.34619 8H13.6538', stroke: 'currentColor', 'stroke-linecap': 'square' },
      { d: 'M7.99976 14.0889C9.23509 14.0889 10.1743 11.3629 10.1743 8.00006C10.1743 4.63739 9.23509 1.91138 7.99976 1.91138', stroke: 'currentColor' },
      { d: 'M7.99973 14.0889C6.76445 14.0889 5.8252 11.3629 5.8252 8.00006C5.8252 4.63739 6.76445 1.91138 7.99973 1.91138', stroke: 'currentColor' }
    ]
  }
]

let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>

function groupBlocks(fixture: GlyphCase, running: boolean): ToolBlock[] {
  return [0, 1].map<ToolBlock>(index => {
    const child: RuntimeChildMetadata | undefined = fixture.activity === 'subagents' ? {
      kind: 'subagent', parentThreadId: 'glyph-thread', parentTurnId: 'glyph-turn',
      childId: 'glyph-child-' + index, childRunId: 'glyph-child-run-' + index,
      childThreadId: 'glyph-child-thread-' + index, childStatus: running ? 'running' : 'completed'
    } : undefined
    return {
      kind: 'tool', id: fixture.activity + '-call-' + index, toolKind: fixture.toolKind,
      status: running ? 'running' : 'success', summary: 'Synthetic ' + fixture.toolName,
      meta: { toolName: fixture.toolName, turnId: 'glyph-turn', callId: fixture.activity + '-call-' + index,
        ...(child ? { child } : {}) }
    }
  })
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  previous = useChatStore.getState()
  useChatStore.setState({ workspaceRoot: '' })
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  useChatStore.setState(previous)
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

// This mounts the actual group consumer with public call anchors. No Provider,
// private local display, network, clipboard, file action or scroll fixture is needed.
describe('pinned DSH process-group glyphs', () => {
  it.each(['task', 'delegate_task', 'parallel_tasks'])('%s uses the pinned AgentPreset glyph in the actual atomic row', async toolName => {
    const fixture = glyphCases.find(item => item.activity === 'subagents')!
    const block: ToolBlock = { kind: 'tool', id: 'atomic-' + toolName, toolKind: 'subagent', status: 'success', summary: toolName, meta: { toolName } }
    await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false, policy: presentationPolicyFor('detailed') })))
    const glyph = container.querySelector('[data-subagent-row] [data-disclosure-row] svg')!
    expect(glyph.getAttribute('width')).toBe('14'); expect(glyph.getAttribute('viewBox')).toBe('0 0 16 16')
    expect([...glyph.children].map(path => ({ tag: path.tagName.toLowerCase(), attributes: Object.fromEntries([...path.attributes].map(attribute => [attribute.name, attribute.value])) })))
      .toEqual(fixture.paths.map(attributes => ({ tag: 'path', attributes })))
  })
  it.each(glyphCases)('$name in running and settled real group headers', async fixture => {
    for (const running of [true, false]) {
      await act(async () => root.render(createElement(ToolCallTree, {
        blocks: groupBlocks(fixture, running), processing: running, policy: presentationPolicyFor('compact')
      })))
      const groups = container.querySelectorAll('[data-process-group]')
      expect(groups).toHaveLength(1)
      expect(groups[0].querySelectorAll('[data-chat-seat]')).toHaveLength(2)
      const header = groups[0].querySelector<HTMLButtonElement>(':scope > div:first-child > button')
      expect(header).not.toBeNull()
      if (!running) expect(header!.dataset.processActivity).toBe(fixture.activity)
      const glyph = header!.querySelector<SVGSVGElement>(':scope > span:first-child > span:first-child > svg')
      expect(glyph).not.toBeNull()
      expect(glyph!.getAttribute('width')).toBe('14')
      expect(glyph!.getAttribute('height')).toBe('14')
      expect(glyph!.getAttribute('viewBox')).toBe('0 0 16 16')
      expect(glyph!.getAttribute('fill')).toBe('none')
      expect(glyph!.getAttribute('stroke-width')).toBe('1')
      expect(glyph!.getAttribute('aria-hidden')).toBe('true')
      expect([...glyph!.children].map(path => ({
        tag: path.tagName.toLowerCase(), attributes: Object.fromEntries([...path.attributes].map(attribute => [attribute.name, attribute.value]))
      }))).toEqual(fixture.paths.map(attributes => ({ tag: 'path', attributes })))
    }
  })
})

it('does not combine a cached public activity title with the next private preview', async () => {
  vi.useFakeTimers()
  const props = { groupKey: 'actual-title', grouped: true, open: true, visible: true, running: true,
    label: (activity: string) => activity, icon: null, onToggle: vi.fn(), children: null }
  await act(async () => root.render(createElement(ChatGroupSeat, { ...props, activity: 'write', detail: 'PUBLIC_WRITE' })))
  await act(async () => root.render(createElement(ChatGroupSeat, { ...props, activity: 'thinking', protectedDetail: createElement('span', { 'data-protected-process-block': 'reasoning' }, 'PRIVATE_NEXT') })))
  const header = container.querySelector('[data-disclosure-row]')!
  expect(header.textContent).toContain('PUBLIC_WRITE'); expect(header.textContent).not.toContain('PRIVATE_NEXT')
  await act(async () => vi.advanceTimersByTimeAsync(150))
  expect(header.textContent).toContain('thinking'); expect(header.textContent).toContain('PRIVATE_NEXT')
  await act(async () => root.render(createElement(ChatGroupSeat, { ...props, activity: 'thinking' })))
  expect(header.textContent).not.toContain('PRIVATE_NEXT')
  vi.useRealTimers()
})


it.each([
  ['compact', true], ['compact', false], ['standard', true], ['standard', false], ['detailed', false]
] as const)('keeps a real single public tool group in %s (processing=%s) without inventing private content', async (mode, processing) => {
  const block: ToolBlock = { kind: 'tool', id: 'single-public', toolKind: 'tool_call', status: processing ? 'running' : 'success',
    summary: 'Public read', detail: 'Public result', meta: { toolName: 'read', callId: 'single-call', turnId: 'single-turn' } }
  const onVisibility = vi.fn()
  await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing, policy: presentationPolicyFor(mode), onVisibility })))
  const group = container.querySelector('[data-process-group]')!
  const header = group.querySelector<HTMLButtonElement>(':scope > div:first-child > button')!
  expect(header).not.toBeNull(); expect(header.getAttribute('aria-expanded')).toBe('false')
  expect(group.querySelectorAll('[data-chat-seat]')).toHaveLength(1)
  expect(group.querySelector('[data-chat-seat="single-public"]')).not.toBeNull()
  expect(group.querySelector('[data-protected-process-block]')).toBeNull()
  expect(onVisibility).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'all_groups_folded', visiblePrivateIds: [], explicitGroupOpen: 0 }))
  await act(async () => header.click())
  expect(header.getAttribute('aria-expanded')).toBe('true'); expect(document.activeElement).toBe(header)
  expect(document.getElementById(header.getAttribute('aria-controls')!)).not.toBeNull()
  expect(onVisibility).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'visible', visiblePrivateIds: [], explicitGroupOpen: 1 }))
})

it.each(['verbose', 'detailed'] as const)('keeps a live single tool expanded without a group disclosure in %s', async mode => {
  const block: ToolBlock = { kind: 'tool', id: 'live-single', toolKind: 'tool_call', status: 'running', summary: 'read', meta: { toolName: 'read' } }
  await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: true, policy: presentationPolicyFor(mode) })))
  expect(container.querySelector('[data-process-group] > div:first-child > button')).toBeNull()
  expect(container.querySelector('[data-chat-seat="live-single"]')).not.toBeNull()
})

it.each(['user', 'assistant'] as const)('keeps a single %s seat outside the tool grouping guard', async kind => {
  const block: ChatBlock = { kind, id: 'standalone', text: 'Independent public text' }
  await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false, policy: presentationPolicyFor('compact') })))
  expect(container.querySelector('[data-process-group] > div:first-child > button')).toBeNull()
  expect(container.querySelector('[data-chat-seat="standalone"]')?.textContent).toContain('Independent public text')
})

it('switches the real activity glyph and title together after the 150ms live window', async () => {
  vi.useFakeTimers()
  try {
    const plan = glyphCases.find(fixture => fixture.activity === 'plan')!
    const web = glyphCases.find(fixture => fixture.activity === 'webFetch')!
    const blocks = groupBlocks(plan, true)
    const props = { blocks, processing: true, policy: presentationPolicyFor('compact') }
    await act(async () => root.render(createElement(ToolCallTree, props)))
    const header = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button')!
    const glyph = () => header.querySelector('span:first-child > span:first-child > svg')!.outerHTML
    const initialGlyph = glyph(), initialTitle = header.textContent
    await act(async () => root.render(createElement(ToolCallTree, { ...props,
      blocks: [...blocks.map<ToolBlock>(block => ({ ...block, status: 'success' })), ...groupBlocks(web, true)] })))
    expect(header.dataset.processActivity).toBe('plan'); expect(header.textContent).toBe(initialTitle); expect(glyph()).toBe(initialGlyph)
    await act(async () => vi.advanceTimersByTimeAsync(149))
    expect(header.dataset.processActivity).toBe('plan'); expect(header.textContent).toBe(initialTitle); expect(glyph()).toBe(initialGlyph)
    await act(async () => vi.advanceTimersByTimeAsync(1))
    expect(header.dataset.processActivity).toBe('webFetch'); expect(header.textContent).not.toBe(initialTitle); expect(glyph()).not.toBe(initialGlyph)
    expect([...header.querySelector('span:first-child > span:first-child > svg')!.children].map(path => path.getAttribute('d'))).toEqual(web.paths.map(path => path.d))
  } finally { vi.useRealTimers() }
})

it('updates a changed fallback glyph even when the activity and public title stay the same', async () => {
  vi.useFakeTimers()
  try {
    const props = { groupKey: 'same-activity', grouped: true, open: true, visible: true, running: true,
      activity: 'tools', label: (activity: string) => activity, onToggle: vi.fn(), children: null }
    await act(async () => root.render(createElement(ChatGroupSeat, { ...props, iconKey: 'tools:tool', icon: createElement('span', { 'data-glyph': 'tool' }) })))
    await act(async () => root.render(createElement(ChatGroupSeat, { ...props, iconKey: 'tools:diff', icon: createElement('span', { 'data-glyph': 'diff' }) })))
    expect(container.querySelector('[data-glyph]')?.getAttribute('data-glyph')).toBe('tool')
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(container.querySelector('[data-glyph]')?.getAttribute('data-glyph')).toBe('diff')
  } finally { vi.useRealTimers() }
})

it('suppresses only the intermediate assistant actions at the process call site', async () => {
  const block: ChatBlock = { kind: 'assistant', id: 'intermediate', text: 'Durable intermediate response', createdAt: '2026-10-15T10:20:30Z' }
  await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false, policy: presentationPolicyFor('verbose') })))
  const seat = container.querySelector('[data-chat-seat="intermediate"]')!
  expect(seat.querySelector('.ds-chat-answer')?.textContent).toContain('Durable intermediate response')
  expect(seat.querySelector('button')).toBeNull()
  await act(async () => root.render(createElement(MessageBubble, { block, nested: true })))
  expect(container.querySelector('.ds-chat-answer')?.textContent).toContain('Durable intermediate response')
  expect(container.querySelector('button')).not.toBeNull()
})


it.each<ChatBlock>([
  { kind: 'user', id: 'steering-boundary', text: 'Next direction' },
  { kind: 'assistant', id: 'reply-boundary', text: 'Actual public reply' },
  { kind: 'system', id: 'retry-boundary', text: 'Retry', meta: { providerRetry: { attempt: 2, maxAttempt: 3 } } },
  { kind: 'system', id: 'error-boundary', text: 'Error', severity: 'error' }
])('closes the earlier header at $id while retaining its running leaf and the live tail', async boundary => {
  const before: ToolBlock = { kind: 'tool', id: 'before-boundary', toolKind: 'tool_call', status: 'running', summary: 'OLD_LIVE_DETAIL', meta: { toolName: 'read' } }
  const after = { ...before, id: 'after-boundary', summary: 'NEW_LIVE_DETAIL' }
  const props = { processing: true, policy: presentationPolicyFor('standard') }
  await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: [before] })))
  const owner = container.querySelector('[data-process-group="public:before-boundary"]')!
  const header = owner.querySelector<HTMLButtonElement>('button[data-process-activity]')!
  expect(header.querySelector('[data-shimmer="true"]')).not.toBeNull()
  await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: [before, boundary, after] })))
  expect(container.querySelector('[data-process-group="public:before-boundary"]')).toBe(owner)
  expect(header.querySelector('[data-shimmer="true"]')).toBeNull()
  expect(header.textContent).not.toContain('OLD_LIVE_DETAIL')
  expect(container.querySelector('[data-process-group="public:after-boundary"] button[data-process-activity] [data-shimmer="true"]')).not.toBeNull()
  await act(async () => header.click())
  expect(owner.querySelector('[data-chat-seat="before-boundary"] [data-state="running"]')).not.toBeNull()
  expect(before.status).toBe('running')
})

it('preserves child merging across ordinary tools but stops merging across actual presentation boundaries', () => {
  const fixture = glyphCases.find(item => item.activity === 'subagents')!
  const first = groupBlocks(fixture, true)[0]
  const update = { ...first, id: 'later-child-observation' }
  const ordinary: ToolBlock = { kind: 'tool', id: 'ordinary-success', summary: 'Read', status: 'success', meta: { toolName: 'read' } }
  const interleaved = groupProcessSections([first, ordinary, update])
  expect(interleaved.map(section => section.blocks.map(block => block.id))).toEqual([[update.id], [ordinary.id]])
  expect(interleaved.every(section => section.presentationClosed === undefined)).toBe(true)
  const reply: ChatBlock = { kind: 'assistant', id: 'child-reply', text: 'Actual reply' }
  const separated = groupProcessSections([first, ordinary, reply, update])
  expect(separated.map(section => section.blocks.map(block => block.id))).toEqual([[first.id], [ordinary.id], [reply.id], [update.id]])
  expect(separated.slice(0, 2).every(section => section.presentationClosed)).toBe(true)
  expect(separated.at(-1)?.presentationClosed).toBeUndefined()
  const extractedReply = groupProcessSections([first, update], [first.id])
  expect(extractedReply.map(section => section.blocks.map(block => block.id))).toEqual([[first.id], [update.id]])
  expect(extractedReply[0].presentationClosed).toBe(true)
  expect(extractedReply[1].presentationClosed).toBeUndefined()
})

it('does not infer closure for an unanchored private tail from public boundaries or a retained identity', async () => {
  const source: ProcessLocalRows['identities'][number]['source'] = { kind: 'provider_reasoning', unitId: 'unknown-unit',
    producerId: 'unknown-producer', providerSequence: '1', attempt: 1, sourceOrdinal: '1' }
  const local: ProcessLocalRows = { threadId: 'unknown-thread', turnId: 'unknown-turn',
    units: [{ unitId: 'unknown-unit', producerId: 'unknown-producer', providerSequence: '1', placement: 'unit_only', calls: [], precedingCalls: [] }],
    identities: [{ blockId: 'unknown-reasoning', source }],
    blocks: [processDisplayBlockSchemaV1.parse({ blockId: 'unknown-reasoning', source, kind: 'native_reasoning', state: 'running', attempt: 1,
      durationMs: 0, truncated: false, classification: 'credential_redacted', preview: 'FRESH_PRIVATE_PREVIEW' })],
    render: vi.fn(() => null), capture: vi.fn(() => ({ blockId: 'unknown-reasoning', preview: null, body: null,
      expanded: false, expandable: false, setOpen: vi.fn() })) }
  const blocks: ChatBlock[] = [{ kind: 'tool', id: 'public-before', summary: 'Read', status: 'running', meta: { toolName: 'read' } },
    { kind: 'assistant', id: 'public-reply', text: 'Actual reply' }]
  const props = { blocks, processing: true, policy: presentationPolicyFor('standard') }
  await act(async () => root.render(createElement(ToolCallTree, { ...props, local })))
  const privateHeader = container.querySelector('[data-process-group="unit:unknown-unit"] button[data-process-activity]')!
  expect(privateHeader.querySelector('[data-shimmer="true"]')).not.toBeNull()
  expect(container.querySelector('[data-process-group="public:public-before"] button[data-process-activity] [data-shimmer="true"]')).toBeNull()
  await act(async () => root.render(createElement(ToolCallTree, { ...props, local: { ...local, blocks: [] } })))
  expect(privateHeader.querySelector('[data-shimmer="true"]')).toBeNull()
  expect(privateHeader.textContent).toBe(i18n.t('common:dshTurnProcess'))
  const gap: ToolBlock = { kind: 'tool', id: 'public-gap', summary: 'Read done', status: 'success', meta: { toolName: 'read' } }
  await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: [...blocks, gap], local: { ...local, blocks: [] } })))
  const publicGapHeader = container.querySelector('[data-process-group="public:public-gap"] button[data-process-activity]')!
  expect(publicGapHeader.getAttribute('data-process-activity')).toBe('thinking')
  expect(publicGapHeader.querySelector('[data-shimmer="true"]')).not.toBeNull()
  expect(privateHeader.querySelector('[data-shimmer="true"]')).toBeNull()
  expect(privateHeader.textContent).toBe(i18n.t('common:dshTurnProcess'))
  expect(local.render).not.toHaveBeenCalled()
})


it('keeps the known open public tail thinking between observations while closed groups and leaves retain their states', async () => {
  vi.useFakeTimers()
  try {
    const fixture = glyphCases.find(item => item.activity === 'webFetch')!
    const running = groupBlocks(fixture, true)
    const settled = running.map<ToolBlock>(block => ({ ...block, status: 'success' }))
    const props = { processing: true, policy: presentationPolicyFor('standard') }
    await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: running })))
    const owner = container.querySelector('[data-process-group]')!
    const header = owner.querySelector<HTMLButtonElement>('button[data-process-activity]')!
    await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: settled })))
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(header.dataset.processActivity).toBe('thinking')
    expect(header.querySelector('[data-shimmer="true"]')).not.toBeNull()
    await act(async () => header.click())
    expect(owner.querySelector('[data-chat-seat] [data-state="running"]')).toBeNull()
    expect(owner.querySelector('[data-chat-seat] [data-state="ok"]')).not.toBeNull()
    expect(settled.every(block => block.status === 'success')).toBe(true)
    const reply: ChatBlock = { kind: 'assistant', id: 'gap-reply', text: 'Actual reply' }
    const next = { ...settled[0], id: 'next-public-gap' }
    const closed = settled.map<ToolBlock>(block => ({ ...block, summary: 'CLOSED_DETAIL_MUST_NOT_APPEAR' }))
    await act(async () => root.render(createElement(ToolCallTree, { ...props, blocks: [...closed, reply, next] })))
    expect(container.querySelector(`[data-process-group="public:${running[0].id}"]`)).toBe(owner)
    expect(header.querySelector('[data-shimmer="true"]')).toBeNull()
    expect(header.textContent).not.toContain('CLOSED_DETAIL_MUST_NOT_APPEAR')
    await act(async () => vi.advanceTimersByTimeAsync(150))
    expect(header.querySelector('[data-shimmer="true"]')).toBeNull()
    expect(header.textContent).not.toContain('CLOSED_DETAIL_MUST_NOT_APPEAR')
    const nextHeader = container.querySelector('[data-process-group="public:next-public-gap"] button[data-process-activity]')!
    expect(nextHeader.getAttribute('data-process-activity')).toBe('thinking')
    expect(nextHeader.querySelector('[data-shimmer="true"]')).not.toBeNull()
  } finally { vi.useRealTimers() }
})
