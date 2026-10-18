import { workspaceShortcutLabels } from '@shared/native-office'
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactElement } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { File, Files, Globe, ListChecks, MessageSquare, Plus, ScanEye, Users, X, FilePlus2, Shapes } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import { CanvasOpenButton } from '../../canvas/CanvasOpenButton'
import type { WorkspaceTab } from '../../store/workspace-tabs-store'
import { nativeOfficeCreateFileNameValid, readNativeOfficeCreateReadiness, type NativeOfficeCreateReadiness } from '../../office/native-office-store'
import { DshClockOutlineRegular } from '../chat/presentation/DshToolIcons'
import './workspace-tabs.css'

/* Guide/compass and mirrored panel artwork ported from DeepSeek Harness
 * 5badb15009ae1756c3afe0ae0cef1faafc290ccc, ui-sidebar-right GuideBody/GuideTitle,
 * shell/ExpandButton and ui-primitives IconPanelLeftOutlineRegular.
 * Guide artwork: ui-primitives guide-artwork.tsx (Files/Browser) and
 * plugin-artwork.tsx (Terminal); fullscreen glyphs: shell/SidebarRight.tsx.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
export function WorkspacePanelIcon({ size = 16, className }: { size?: number; className?: string }): ReactElement {
  return <svg width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden="true" className={className}>
    <g transform="translate(16 0) scale(-1 1)">
      <path d="M13.5 1.5H2.5C1.94772 1.5 1.5 1.94772 1.5 2.5V13.5C1.5 14.0523 1.94772 14.5 2.5 14.5H13.5C14.0523 14.5 14.5 14.0523 14.5 13.5V2.5C14.5 1.94772 14.0523 1.5 13.5 1.5Z" stroke="currentColor" />
      <path d="M5.5 1.5V14.5" stroke="currentColor" />
    </g>
  </svg>
}
function WorkspaceGuideCompass(): ReactElement {
  return <svg width={56} height={56} viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M8 14C11.3137 14 14 11.3137 14 8C14 4.68629 11.3137 2 8 2C4.68629 2 2 4.68629 2 8C2 11.3137 4.68629 14 8 14Z" stroke="currentColor" />
    <path d="M10.6101 5.39014L8.99014 8.99014L5.39014 10.6101L7.01014 7.01014L10.6101 5.39014Z" fill="currentColor" />
  </svg>
}

type WorkspaceGuideArtworkProps = { size?: number; className?: string }

function GuideArtworkFiles({ size = 36, className }: WorkspaceGuideArtworkProps) {
  return (
    <svg width={size} height={size} className={className} aria-hidden="true" viewBox="0 0 36 36" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M10.7603 27.922H24.6817C26.3441 27.922 27.1753 27.922 27.8102 27.5984C28.3687 27.3139 28.8228 26.8598 29.1074 26.3012C29.4309 25.6663 29.4309 24.8351 29.4309 23.1727V15.4936" stroke="#FFCD78" strokeWidth="1.97886" />
      <path d="M13.1597 8.07812C13.4182 8.07818 13.6727 8.14336 13.8989 8.26855L16.7554 9.84961C16.9817 9.97485 17.2369 10.041 17.4956 10.041H26.106C26.9492 10.0412 27.6323 10.7251 27.6323 11.5684V24.5371C27.6323 25.3805 26.9483 26.0645 26.105 26.0645H8.09619C7.25281 26.0645 6.56884 25.3805 6.56885 24.5371V9.60449C6.56909 8.76133 7.25297 8.07812 8.09619 8.07812H13.1597ZM9.81592 14.5508V16.5293H24.3999V14.5508H9.81592Z" fill="#FFBC4D" />
    </svg>
  )
}

function GuideArtworkBrowser({ size = 36, className }: WorkspaceGuideArtworkProps) {
  return (
    <svg width={size} height={size} className={className} aria-hidden="true" viewBox="0 0 36 36" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M17.9995 28.1465C23.6034 28.1465 28.1461 23.6038 28.1461 18C28.1461 12.3963 23.6034 7.85352 17.9995 7.85352C12.3958 7.85352 7.85303 12.3963 7.85303 18C7.85303 23.6038 12.3958 28.1465 17.9995 28.1465Z" stroke="#539CFA" strokeWidth="2" />
      <path d="M8.57764 18H27.4211" stroke="#539CFA" strokeWidth="2" strokeLinecap="square" />
      <path d="M17.999 28.1467C20.0576 28.1467 21.6228 23.6039 21.6228 18C21.6228 12.3963 20.0576 7.85352 17.999 7.85352" stroke="#539CFA" strokeWidth="2" />
      <path d="M17.9992 28.1467C15.9407 28.1467 14.3755 23.6039 14.3755 18C14.3755 12.3963 15.9407 7.85352 17.9992 7.85352" stroke="#539CFA" strokeWidth="2" />
    </svg>
  )
}

const PluginArtworkTerminal = ({ size = 36, className }: WorkspaceGuideArtworkProps) => (
  <svg width={size} height={size} className={className} viewBox="0 0 36 36" fill="none" aria-hidden="true" xmlns="http://www.w3.org/2000/svg">
    <path d="M10 11L16.606 17.606C16.6841 17.6841 16.6841 17.8107 16.606 17.8888L10 24.4948" stroke="#679EFE" strokeWidth="3.5" />
    <path d="M20.1211 24.4946H26.8685" stroke="#679EFE" strokeWidth="3.5" />
  </svg>
)

function FullscreenGlyph(): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.33203 10.4054V13.1681C2.33229 13.444 2.55605 13.6681 2.83203 13.6681H5.49512V14.6681H2.83203C2.00376 14.6681 1.33229 13.9963 1.33203 13.1681V10.4054H2.33203ZM14.6689 13.1681C14.6687 13.996 13.9968 14.6676 13.1689 14.6681H10.4951V13.6681H13.1689C13.4445 13.6676 13.6687 13.4437 13.6689 13.1681V10.4054H14.6689V13.1681ZM13.1689 1.33118C13.9969 1.33163 14.6688 2.00315 14.6689 2.83118V5.4054H13.6689V2.83118C13.6688 2.55544 13.4446 2.33162 13.1689 2.33118H10.4951V1.33118H13.1689ZM5.49512 2.33118H2.83203C2.55598 2.33118 2.33218 2.55516 2.33203 2.83118V5.4054H1.33203V2.83118C1.33218 2.00288 2.00369 1.33118 2.83203 1.33118H5.49512V2.33118Z" fill="currentColor" />
    </svg>
  )
}

function ExitFullscreenGlyph(): ReactElement {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M9 2.5V6C9 6.26522 9.10536 6.51957 9.29289 6.70711C9.48043 6.89464 9.73478 7 10 7H13.5" stroke="currentColor" />
      <path d="M7 13.5V10C7 9.73478 6.89464 9.48043 6.70711 9.29289C6.51957 9.10536 6.26522 9 6 9H2.5" stroke="currentColor" />
    </svg>
  )
}

export function workspaceTabDomId(id: string): string { return `workspace-tab-${encodeURIComponent(id)}` }
export function workspacePanelDomId(id: string): string { return `workspace-panel-${encodeURIComponent(id)}` }

function SortableWorkspaceTab({ tab, active, focusable, onFocus, onSelect, onClose, onKeyDown }: {
  tab: WorkspaceTab; active: boolean; focusable: boolean; onFocus: () => void
  onSelect: () => void; onClose: () => void; onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
}): ReactElement {
  const { t } = useTranslation('common')
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: tab.id })
  const Icon = tab.mode === 'schedule-task' ? DshClockOutlineRegular : tab.preview === 'canvas' || /\.canvas$/i.test(tab.path ?? '') ? Shapes : tab.mode === 'browser' ? Globe : tab.mode === 'files' ? Files : tab.mode === 'changes' ? ScanEye : tab.mode === 'child-agent' ? (tab.id.startsWith('sidechat:') ? MessageSquare : Users) : tab.mode === 'todo' || tab.mode === 'plan' ? ListChecks : File
  const status = tab.error ? t('error') : tab.loading ? t('loading') : tab.dirty ? t('unsavedChanges', { defaultValue: '未保存' }) : ''
  return (
    <div ref={setNodeRef} className="workspace-tab" data-kind={tab.kind} data-active={active} data-dragging={isDragging}
      style={{ transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined, transition }}>
      <button {...attributes} {...listeners} type="button" role="tab" id={workspaceTabDomId(tab.id)} aria-selected={active}
        aria-controls={workspacePanelDomId(tab.id)} aria-label={status ? `${tab.title} · ${status}` : tab.title}
        tabIndex={focusable ? 0 : -1} title={tab.title} onFocus={onFocus} onClick={onSelect} onKeyDown={onKeyDown}>
        <Icon aria-hidden="true" size={16} />
        <span className="workspace-tab-title">{tab.title}</span>
        {status ? <span className="workspace-tab-status" data-error={tab.error} aria-hidden="true">{tab.error ? '!' : tab.loading ? '…' : '•'}</span> : null}
      </button>
      <button type="button" className="workspace-tab-close" aria-label={t('closeTab', { defaultValue: '关闭 {{title}}', title: tab.title })} title={t('close')} onClick={onClose}><X size={14} aria-hidden="true" /></button>
    </div>
  )
}

export function WorkspaceTabs({ tabs, activeTabId, selectorOpen, focused, onSelect, onClose, onReorder, onAdd, onToggleFocus, onCollapse }: {
  tabs: WorkspaceTab[]; activeTabId: string | null; selectorOpen: boolean; focused: boolean
  onSelect: (id: string) => void; onClose: (id: string) => void | Promise<void>; onReorder: (id: string, index: number) => void
  onAdd: () => void; onToggleFocus: () => void; onCollapse: () => void
}): ReactElement {
  const { t } = useTranslation('common')
  const [focusedId, setFocusedId] = useState<string | null>(activeTabId)
  useEffect(() => setFocusedId(activeTabId), [activeTabId])
  const addRef = useRef<HTMLButtonElement>(null)
  const tablistRef = useRef<HTMLDivElement>(null)
  const tabIds = JSON.stringify(tabs.map(tab => tab.id))
  const revealTab = useCallback((id: string): void => {
    const list = tablistRef.current
    const tab = document.getElementById(workspaceTabDomId(id))?.closest<HTMLElement>('.workspace-tab')
    if (!list || !tab || !list.contains(tab) || !list.clientWidth) return
    const viewport = list.getBoundingClientRect(), bounds = tab.getBoundingClientRect()
    if (!viewport.width) return
    const delta = bounds.width > viewport.width
      ? bounds.right - viewport.right
      : Math.min(bounds.left - viewport.left, 0) || Math.max(bounds.right - viewport.right, 0)
    if (Math.abs(delta) > 0.5) {
      // Scroll only this strip, including close. Instant also honors reduced motion.
      list.scrollTo({ left: list.scrollLeft + delta / (viewport.width / list.clientWidth), behavior: 'instant' })
    }
  }, [])
  useLayoutEffect(() => {
    if (!activeTabId || selectorOpen) return
    revealTab(activeTabId)
    if (typeof ResizeObserver === 'undefined') return
    const list = tablistRef.current
    const tab = document.getElementById(workspaceTabDomId(activeTabId))?.closest<HTMLElement>('.workspace-tab')
    if (!list || !tab || !list.contains(tab)) return
    const observer = new ResizeObserver(() => revealTab(activeTabId))
    observer.observe(list); observer.observe(tab)
    return () => observer.disconnect()
  }, [activeTabId, selectorOpen, tabIds, revealTab])
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const rovingId = tabs.some((tab) => tab.id === focusedId) ? focusedId : activeTabId ?? tabs[0]?.id
  const focusTab = (id: string): void => {
    setFocusedId(id)
    document.getElementById(workspaceTabDomId(id))?.focus({ preventScroll: true })
    revealTab(id)
  }
  const closeAndRestoreFocus = (index: number): void => {
    const tab = tabs[index]
    void Promise.resolve(onClose(tab.id)).then(() => requestAnimationFrame(() => {
      // Dirty-close rejection keeps the original focus; successful close restores its neighbor.
      if (document.getElementById(workspaceTabDomId(tab.id))) return
      const next = tabs[index + 1] ?? tabs[index - 1]
      if (next) focusTab(next.id)
      else addRef.current?.focus()
    }))
  }
  const handleKey = (event: KeyboardEvent<HTMLButtonElement>, index: number): void => {
    if (event.defaultPrevented || event.nativeEvent.isComposing || event.keyCode === 229) return
    const tab = tabs[index]
    if (event.altKey && event.shiftKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
      event.preventDefault(); onReorder(tab.id, index + (event.key === 'ArrowLeft' ? -1 : 1)); return
    }
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
      event.preventDefault()
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowLeft' ? -1 : 1) + tabs.length) % tabs.length
      focusTab(tabs[next].id)
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault(); onSelect(tab.id)
    } else if (event.key === 'Delete' || ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'w')) {
      event.preventDefault(); closeAndRestoreFocus(index)
    }
  }
  const handleDragEnd = ({ active, over }: DragEndEvent): void => {
    if (over && active.id !== over.id) onReorder(String(active.id), tabs.findIndex((tab) => tab.id === over.id))
  }
  return (
    <header className="workspace-tabs-header">
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={tabs.map((tab) => tab.id)} strategy={horizontalListSortingStrategy}>
          <div ref={tablistRef} className="workspace-tablist" role="tablist" aria-label={t('workspaceTabs', { defaultValue: '工作区标签' })}>
            {tabs.map((tab, index) => <SortableWorkspaceTab key={tab.id} tab={tab} active={!selectorOpen && tab.id === activeTabId}
              focusable={tab.id === rovingId} onFocus={() => setFocusedId(tab.id)} onSelect={() => onSelect(tab.id)} onClose={() => closeAndRestoreFocus(index)} onKeyDown={(event) => handleKey(event, index)} />)}
          </div>
        </SortableContext>
      </DndContext>
      <div className="workspace-tabs-actions">
        <button ref={addRef} type="button" onClick={onAdd} aria-label={t('workspaceAddTab', { defaultValue: '打开工具或文件' })} title={t('workspaceAddTab', { defaultValue: '打开工具或文件' })} aria-pressed={selectorOpen}><Plus size={20} /></button>
        <button type="button" onClick={onToggleFocus} aria-label={t(focused ? 'workbenchDock' : 'workbenchFocus')}
          title={t(focused ? 'workbenchDock' : 'workbenchFocus')} aria-pressed={focused}>
          {focused ? <ExitFullscreenGlyph /> : <FullscreenGlyph />}
        </button>
        <button id="workbench-workspace-collapse" type="button" onClick={() => onCollapse()} aria-label={t('workbenchCollapse')}
          title={`${t('workbenchCollapse')} (${workspaceShortcutLabels.workspace})`} aria-keyshortcuts="Control+Shift+B Meta+Shift+B"
          aria-controls="workbench-right-workspace" aria-expanded={true}><WorkspacePanelIcon /></button>
      </div>
    </header>
  )
}

type SelectorAction = 'files' | 'browser' | 'changes' | 'sidechat' | 'terminal' | 'todo' | 'plan'
const MAX_DESCRIBED_ENTRIES = 4
export function WorkspaceToolSelector({ onOpen, sideChatEnabled, filesEnabled, planEnabled, terminalAvailable = true, onCreateWord, onOpenCreatedWord, wordEnabled = false, wordCreation, onOpenWordPlugins }: {
  onOpen: (action: SelectorAction) => void; sideChatEnabled: boolean; filesEnabled: boolean; planEnabled: boolean
  terminalAvailable?: boolean
  onCreateWord?: (fileName: string) => void | Promise<void | NativeOfficeCreateReadiness>
  onOpenWordPlugins?: () => void
  onOpenCreatedWord?: () => void
  wordEnabled?: boolean
  wordCreation?: {phase:'pending' | 'unknown' | 'failed';fileName:string;message?:string}
}): ReactElement {
  const { t } = useTranslation('common')
  const [wordOpen, setWordOpen] = useState(false)
  const wordNameRef = useRef<HTMLInputElement>(null)
  useEffect(() => { if (wordOpen) wordNameRef.current?.focus() }, [wordOpen])
  const [wordFileName, setWordFileName] = useState(() => wordCreation?.fileName ?? t('workspaceNewWordFileName'))
  useEffect(() => {
    if (wordCreation?.phase === 'pending' || wordCreation?.phase === 'unknown') setWordFileName(wordCreation.fileName)
  }, [wordCreation?.phase,wordCreation?.fileName])
  const wordPending = wordCreation?.phase === 'pending'
  const wordUnknown = wordCreation?.phase === 'unknown'
  const [wordReadiness, setWordReadiness] = useState<NativeOfficeCreateReadiness>('checking')
  const wordReadinessEpoch = useRef(0)
  const refreshWordReadiness = useCallback(async () => {
    const epoch = ++wordReadinessEpoch.current
    setWordReadiness('checking')
    const readiness = await readNativeOfficeCreateReadiness()
    if (epoch === wordReadinessEpoch.current) setWordReadiness(readiness)
  }, [])
  useEffect(() => {
    if (wordOpen && wordEnabled) void refreshWordReadiness()
    return () => { wordReadinessEpoch.current++ }
  }, [wordOpen,wordEnabled,refreshWordReadiness])
  const wordCanSubmit = wordEnabled && !wordPending && (wordUnknown || wordReadiness === 'ready')
  const wordReadinessKey = wordReadiness === 'checking' ? 'workspaceWordDocumentsChecking'
    : wordReadiness === 'not-enabled' ? 'workspaceWordDocumentsNotEnabled'
    : wordReadiness === 'unknown' ? 'workspaceWordDocumentsUnknown' : 'workspaceWordDocumentsNotReady'
  const items = [
    { id: 'files' as const, icon: GuideArtworkFiles, label: t('rightPanelFiles'), description: t('workspaceGuideFilesDescription'), enabled: filesEnabled },
    ...(onCreateWord ? [{id:'word' as const,icon:FilePlus2,label:t('workspaceNewWord'),description:t('workspaceNewWordDescription'),enabled:wordEnabled}] : []),
    { id: 'browser' as const, icon: GuideArtworkBrowser, label: t('rightPanelBrowser'), description: t('workspaceGuideBrowserDescription'), enabled: true },
    { id: 'changes' as const, icon: ScanEye, label: t('rightPanelChanges'), description: t('workspaceGuideChangesDescription'), enabled: true },
    { id: 'sidechat' as const, icon: MessageSquare, label: t('sidePanelOpen'), description: t('workspaceGuideSideChatDescription'), enabled: sideChatEnabled },
    ...(terminalAvailable ? [{ id: 'terminal' as const, icon: PluginArtworkTerminal, label: t('rightPanelTerminal'), description: t('workspaceGuideTerminalDescription'), enabled: true, shortcut: workspaceShortcutLabels.terminal }] : []),
    { id: 'todo' as const, icon: ListChecks, label: t('rightPanelTodo'), description: t('workspaceGuideTodoDescription'), enabled: true },
    ...(planEnabled ? [{ id: 'plan' as const, icon: ListChecks, label: t('rightPanelPlan'), description: t('workspaceGuidePlanDescription'), enabled: true }] : [])
  ]
  // Analytix contributes its real existing entries; the guide drops all descriptions
  // together when this list grows, just like the pinned DSH GuideBody.
  const described = items.length + 1 <= MAX_DESCRIBED_ENTRIES
  return <section className="workspace-tool-selector" data-analytix-chat-token-scope aria-label={t('workspaceSelectTool')}>
    <div className="workspace-guide" data-sidebar-right-guide>
      <span className="workspace-guide-hero" aria-hidden="true"><WorkspaceGuideCompass /></span>
      <div className="workspace-guide-entry-cell workspace-guide-canvas"><CanvasOpenButton enabled={filesEnabled} /></div>
      {items.map(({ id, icon: Icon, label, description, enabled }) => <div key={id} className="workspace-guide-entry-cell">
        <button type="button" className="workspace-guide-entry" data-sidebar-right-guide-entry={id} disabled={!enabled}
          aria-keyshortcuts={id === 'terminal' ? 'Control+` Meta+`' : undefined} onClick={() => { if (id === 'word') setWordOpen(true); else onOpen(id) }}>
          <span className="workspace-guide-entry-icon"><Icon size={described ? 26 : 22} aria-hidden="true" /></span>
          <span className="workspace-guide-entry-text"><span className="workspace-guide-entry-title">{label}</span>
            {described ? <span className="workspace-guide-entry-description">{description}</span> : null}</span>
          {id === 'terminal' ? <kbd className="workspace-guide-shortcut" aria-hidden="true">{workspaceShortcutLabels.terminal}</kbd> : null}
        </button>
      </div>)}
    </div>
    {onOpenCreatedWord ? <button type="button" className="mx-3 mb-3 rounded border border-ds-border px-2 py-1 text-xs"
      data-word-open-created onClick={onOpenCreatedWord}>{t('workspaceWordOpenCreated')}</button> : null}
    {wordOpen && onCreateWord ? <form className="mx-3 mb-3 flex flex-wrap items-center gap-2" aria-label={t('workspaceNewWord')} onSubmit={async event => {
      event.preventDefault()
      if (!wordCanSubmit || !nativeOfficeCreateFileNameValid(wordFileName)) return
      const epoch = wordReadinessEpoch.current
      const readiness = await onCreateWord(wordFileName)
      if (readiness && epoch === wordReadinessEpoch.current) setWordReadiness(readiness)
    }}>
      <label className="flex min-w-0 flex-1 items-center gap-2 text-xs">
        <span>{t('workspaceWordFileName')}</span>
        <input ref={wordNameRef} className="min-w-0 flex-1 rounded border border-ds-border bg-ds-card px-2 py-1" required maxLength={255}
          aria-label={t('workspaceWordFileName')} disabled={wordPending || wordUnknown} value={wordFileName} onChange={event => setWordFileName(event.target.value)} />
      </label>
      <button type="submit" className="rounded border border-ds-border px-2 py-1 text-xs" disabled={!wordCanSubmit || !nativeOfficeCreateFileNameValid(wordFileName)}>
        {t(wordPending ? 'workspaceWordCreating' : wordUnknown ? 'workspaceWordRetry' : 'workspaceWordCreate')}
      </button>
      <button type="button" className="px-2 py-1 text-xs" disabled={wordPending} onClick={() => setWordOpen(false)}>{t('close')}</button>
      {!wordPending && !wordUnknown && wordReadiness !== 'ready' ? <div className="flex w-full flex-wrap items-center gap-2 text-xs">
        <p role="status" className="w-full text-ds-muted" data-word-readiness>{t(wordReadinessKey)}</p>
        <button type="button" disabled={wordReadiness === 'checking'} onClick={() => void refreshWordReadiness()}
          className="rounded border border-ds-border px-2 py-1" data-word-readiness-refresh>{t('officePluginsRefresh')}</button>
        {onOpenWordPlugins ? <button type="button" onClick={onOpenWordPlugins}
          className="rounded border border-ds-border px-2 py-1" data-word-open-plugins>{t('workspaceWordOpenPlugins')}</button> : null}
      </div> : null}
      {wordCreation?.message ? <p className="w-full text-xs text-ds-muted" role={wordCreation.phase === 'failed' ? 'alert' : 'status'}>{wordCreation.message}</p> : null}
    </form> : null}
  </section>
}
