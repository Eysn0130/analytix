import { describe, expect, it, vi } from 'vitest'
import workbench from '../Workbench.tsx?raw'
import controls from './ConversationWidthControls.tsx?raw'
import sideConversation from './SideConversationPanel.tsx?raw'
import inspector from '../summary/SubagentInspectorPanel.tsx?raw'
import sddAssistant from '../sdd/SddAssistantPanel.tsx?raw'
import preferences from '../../lib/chat-presentation-preferences.ts?raw'
import threadActions from '../../store/chat-store-thread-actions.ts?raw'
import handoffModal from './above-composer/ThreadHandoffProgressModal.tsx?raw'
import composer from './InputBar.tsx?raw'
import timeline from './MessageTimeline.tsx?raw'

// Vitest's CSS transform also intercepts ?raw; read the actual candidate source.
// Keep the test-only Node API local without adding Node globals to the web graph.
const { readFileSync } = await vi.importActual<{ readFileSync: (path: URL, encoding: 'utf8') => string }>('node:fs')
const css = readFileSync(new URL('./ConversationWidthControls.module.css', import.meta.url), 'utf8')
const shellCss = readFileSync(new URL('../../styles/base-shell.css', import.meta.url), 'utf8')

function rule(source: string, selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 's').exec(source)
  expect(match, `missing ${selector}`).not.toBeNull()
  return match?.[1] ?? ''
}

describe('main Conversation width boundary', () => {
  it('mounts exactly once in the non-scrolling body below header and before terminal, sharing message/composer geometry', () => {
    expect(workbench.match(/<ConversationWidthControls\b/g)).toHaveLength(1)
    const header = workbench.indexOf('<header ref={windowChromeRef}')
    const sdd = workbench.indexOf('{activeSddDraft ? (', header)
    const analysis = workbench.indexOf(') : activeDataAnalysis ? (', sdd)
    const stage = workbench.indexOf('className="ds-chat-stage ', analysis)
    const body = workbench.indexOf('ref={setConversationBody} data-analytix-conversation-body', stage)
    const timeline = workbench.indexOf('<ChatTimelineIsland', body)
    const composer = workbench.indexOf('ref={terminalComposerRef}', timeline)
    const handles = workbench.indexOf('<ConversationWidthControls', composer)
    const terminal = workbench.indexOf('{terminalOpen || terminalPanelMotion.isMounted', handles)
    expect([header, sdd, analysis, stage, body, timeline, composer, handles, terminal].every((position, index, all) => position >= 0 && (index === 0 || position > all[index - 1]))).toBe(true)
    expect(workbench).toMatch(/\{route === 'chat' \? <>\s*<ConversationWidthControls container=\{conversationBody\} phase=\{conversationPhase\}[^>]*reservedInlineEnd=\{summaryReservedInlineEnd\}/)
    for (const embedded of [sideConversation, inspector, sddAssistant]) expect(embedded).not.toContain('ConversationWidthControls')
    expect(controls).toContain("container.style.setProperty('--ax-chat-content-axis'")
    expect(controls).toContain("container.querySelector<HTMLElement>('[data-analytix-message-timeline-scroller]')")
    expect(controls).not.toContain('document.querySelector')
    expect(controls).not.toContain('--dsh-chat-user-width')
    expect(shellCss).toContain('--dsh-chat-content-width: var(--ax-chat-content-axis)')
    expect(shellCss).toContain('max-width: var(--dsh-chat-content-width)')
    expect(shellCss).toContain('--dsh-composer-card-max-width: calc(var(--dsh-chat-content-width) + 32px)')
  })
  it('uses existing renderer content and navigation inputs without equating a thread or running flag with active', () => {
    expect(workbench).toContain('Boolean(s.blocks.length || s.liveAssistant.trim() || s.currentTurnId)')
    expect(workbench).toContain('useSyncExternalStore(subscribeImageThreadNavigation, isImageThreadNavigationFrozen')
    expect(workbench).toContain('summaryBlank: activeThread?.messageCount === 0')
    expect(controls).not.toContain('input.running')
    expect(threadActions).toContain('selectThread: async (id, options) => withImageThreadNavigation')
    expect(threadActions).toContain('subscribeThreadEventsLive: async (threadId) => withImageThreadNavigation')
    expect(controls).toContain("if (container === null || phase !== 'active') return null")
    expect(controls).not.toContain('runtimeConnection')
  })
  it('keeps the 10px strips outside content and column safe zones with a 72px glow and 16px core', () => {
    const handle = rule(css, '.widthHandle')
    expect(handle).toContain('width: min(10px, calc((100% - var(--ax-thread-summary-reserve, 0px) - var(--ax-chat-content-axis)) / 2 - 24px - 24px))')
    expect(handle).toContain('position: absolute')
    expect(handle).toContain('z-index: 0')
    expect(handle).toContain('--dsw-alias-scrollbar-bg-l1: rgb(229, 229, 229)')
    expect(rule(css, ":global([data-theme='dark']) .widthHandle")).toContain('--dsw-alias-scrollbar-bg-l1: rgb(60, 60, 61)')
    const indicator = rule(css, '.widthHandle::after')
    expect(indicator).toContain('width: 2px')
    for (const offset of ['- 36px', '- 8px', '+ 8px', '+ 36px']) expect(indicator).toContain(offset)
    expect(indicator).toContain('var(--dsw-alias-scrollbar-bg-l1)')
    expect(rule(css, ".widthHandle[data-dragging]")).toContain('z-index: 8')
    expect(rule(shellCss, '.ds-chat-stage [data-analytix-message-timeline-content]')).toContain('z-index: 1')
    expect(rule(shellCss, '.ds-chat-stage .ds-main-composer-host')).toContain('z-index: 7')
    expect(css).toContain(':has([data-conversation-composer-overlay])')
    expect(css).toContain('display: none')
  })
  it('uses the Records and Inspect marker for a full-bleed scroller and measured floating composer without changing ordinary chat', () => {
    expect(timeline).toContain("data-conversation-composer-overlay={viewMode !== 'conversation' ? '' : undefined}")
    expect(workbench).toContain('data-analytix-conversation-view')
    expect(workbench).toContain("'--ax-conversation-composer-height': `${composerHeight}px`")
    expect(rule(shellCss, '.ds-chat-stage [data-analytix-message-timeline-scroller]')).toContain('scrollbar-gutter: stable both-edges')
    expect(rule(shellCss, '.ds-chat-stage [data-analytix-conversation-body]:has([data-conversation-composer-overlay]) > [data-analytix-conversation-view]')).toContain('overflow: hidden')
    expect(rule(shellCss, '.ds-chat-stage [data-analytix-message-timeline-scroller][data-conversation-composer-overlay]')).toContain('scrollbar-gutter: auto')
    const content = rule(shellCss, '.ds-chat-stage [data-conversation-composer-overlay] > .ds-message-timeline-content.ds-chat-column-inset')
    expect(content).toContain('width: 100%'); expect(content).toContain('max-width: none')
    expect(content).toContain('padding-bottom: calc(var(--ax-conversation-composer-height, 0px) + 16px)')
    const seat = rule(shellCss, '.ds-chat-stage [data-analytix-conversation-body]:has([data-conversation-composer-overlay]) > .ds-main-composer-host')
    expect(seat).toContain('position: absolute'); expect(seat).toContain('inset-inline-start: 10px')
    expect(seat).toContain('inset-inline-end: calc(10px + var(--ax-thread-summary-reserve, 0px))'); expect(seat).toContain('bottom: 0')
    expect(seat).toContain('var(--ds-bg-main) 36px')
  })
  it('keeps persistence in the existing presentation/browser storage owner', () => {
    expect(preferences).toContain("CONVERSATION_WIDTH_STORAGE_KEY = 'analytix.chatContentWidth'")
    expect(preferences).toContain('readBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY)')
    expect(preferences).toContain('writeBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY')
    expect(preferences).toContain('removeBrowserStorageItem(CONVERSATION_WIDTH_STORAGE_KEY)')
    expect(controls).not.toContain('localStorage')
    expect(preferences + controls).not.toContain('dsh.conversation.contentWidth')
  })
  it('uses actual modal mounting to retire drag controls rather than a running operation or CSS alone', () => {
    expect(handoffModal).toContain('<div data-analytix-thread-handoff-modal className="fixed inset-0')
    expect(handoffModal).toContain('if (!operation) return null')
    expect(composer).toContain("composerGate?.status === 'pending' ? null")
    expect(composer).toContain('hideThreadContextPanels ? null : <ThreadHandoffProgressModal />')
    expect(controls).toContain("container.querySelector('[data-analytix-thread-handoff-modal]')")
    expect(controls).toContain('observer.observe(container, { childList: true, subtree: true })')
    expect(controls).toContain('if (geometry.handleSpace <= 0 || blockedByModal) return null')
    expect(controls).not.toContain('activeThreadHandoffOperationId')
  })
})
