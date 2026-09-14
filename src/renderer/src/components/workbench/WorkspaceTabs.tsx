import { useEffect, useRef, useState, type KeyboardEvent, type ReactElement } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { File, Files, Globe, ListChecks, MessageSquare, Plus, ScanEye, Terminal, Users, X, Maximize2, Minimize2, PanelRightClose, FilePlus2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { WorkspaceTab } from '../../store/workspace-tabs-store'
import './workspace-tabs.css'

export function workspaceTabDomId(id: string): string { return `workspace-tab-${encodeURIComponent(id)}` }
export function workspacePanelDomId(id: string): string { return `workspace-panel-${encodeURIComponent(id)}` }

function SortableWorkspaceTab({ tab, active, focusable, onFocus, onSelect, onClose, onKeyDown }: {
  tab: WorkspaceTab; active: boolean; focusable: boolean; onFocus: () => void
  onSelect: () => void; onClose: () => void; onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
}): ReactElement {
  const { t } = useTranslation('common')
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: tab.id })
  const Icon = tab.mode === 'browser' ? Globe : tab.mode === 'files' ? Files : tab.mode === 'changes' ? ScanEye : tab.mode === 'child-agent' ? (tab.id.startsWith('sidechat:') ? MessageSquare : Users) : tab.mode === 'summary' ? Users : tab.mode === 'todo' || tab.mode === 'plan' ? ListChecks : File
  const status = tab.error ? t('error') : tab.loading ? t('loading') : tab.dirty ? t('unsavedChanges', { defaultValue: '未保存' }) : ''
  return (
    <div ref={setNodeRef} className="workspace-tab" data-active={active} data-dragging={isDragging}
      style={{ transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined, transition }}>
      <button {...attributes} {...listeners} type="button" role="tab" id={workspaceTabDomId(tab.id)} aria-selected={active}
        aria-controls={workspacePanelDomId(tab.id)} aria-label={status ? `${tab.title} · ${status}` : tab.title}
        tabIndex={focusable ? 0 : -1} title={tab.title} onFocus={onFocus} onClick={onSelect} onKeyDown={onKeyDown}>
        <Icon aria-hidden="true" size={15} />
        <span className="workspace-tab-title">{tab.title}</span>
        {status ? <span className="workspace-tab-status" data-error={tab.error} aria-hidden="true">{tab.error ? '!' : tab.loading ? '…' : '•'}</span> : null}
      </button>
      <button type="button" className="workspace-tab-close" aria-label={t('closeTab', { defaultValue: '关闭 {{title}}', title: tab.title })} title={t('close')} onClick={onClose}><X size={13} aria-hidden="true" /></button>
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
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const rovingId = tabs.some((tab) => tab.id === focusedId) ? focusedId : activeTabId ?? tabs[0]?.id
  const focusTab = (id: string): void => {
    setFocusedId(id)
    document.getElementById(workspaceTabDomId(id))?.focus()
    document.getElementById(workspaceTabDomId(id))?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
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
    if (event.nativeEvent.isComposing || event.keyCode === 229) return
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
          <div className="workspace-tablist" role="tablist" aria-label={t('workspaceTabs', { defaultValue: '工作区标签' })}>
            {tabs.map((tab, index) => <SortableWorkspaceTab key={tab.id} tab={tab} active={!selectorOpen && tab.id === activeTabId}
              focusable={tab.id === rovingId} onFocus={() => setFocusedId(tab.id)} onSelect={() => onSelect(tab.id)} onClose={() => closeAndRestoreFocus(index)} onKeyDown={(event) => handleKey(event, index)} />)}
          </div>
        </SortableContext>
      </DndContext>
      <div className="workspace-tabs-actions">
        <button ref={addRef} type="button" onClick={onAdd} aria-label={t('workspaceAddTab', { defaultValue: '打开工具或文件' })} title={t('workspaceAddTab', { defaultValue: '打开工具或文件' })} aria-pressed={selectorOpen}><Plus size={16} /></button>
        <button type="button" onClick={onToggleFocus} aria-label={t(focused ? 'workbenchDock' : 'workbenchFocus')} title={t(focused ? 'workbenchDock' : 'workbenchFocus')} aria-pressed={focused}>{focused ? <Minimize2 size={15} /> : <Maximize2 size={15} />}</button>
        <button type="button" onClick={() => {
          onCollapse()
          requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('[aria-controls="workbench-right-workspace"]')?.focus())
        }} aria-label={t('workbenchCollapse')} title={t('workbenchCollapse')}><PanelRightClose size={16} /></button>
      </div>
    </header>
  )
}

type SelectorAction = 'files' | 'documents' | 'browser' | 'changes' | 'summary' | 'sidechat' | 'terminal' | 'todo' | 'plan'
export function WorkspaceToolSelector({ onOpen, sideChatEnabled, filesEnabled, planEnabled }: {
  onOpen: (action: SelectorAction) => void; sideChatEnabled: boolean; filesEnabled: boolean; planEnabled: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const items = [
    { id: 'files' as const, icon: Files, label: t('rightPanelFiles'), enabled: filesEnabled },
    { id: 'documents' as const, icon: FilePlus2, label: t('workspaceNewDocument', { defaultValue: '新建文档' }), enabled: filesEnabled },
    { id: 'browser' as const, icon: Globe, label: t('rightPanelBrowser'), enabled: true },
    { id: 'changes' as const, icon: ScanEye, label: t('rightPanelChanges'), enabled: true },
    { id: 'summary' as const, icon: Users, label: t('workspaceTasks', { defaultValue: '子代理与任务' }), enabled: true },
    { id: 'sidechat' as const, icon: MessageSquare, label: t('sidePanelOpen'), enabled: sideChatEnabled },
    { id: 'terminal' as const, icon: Terminal, label: t('rightPanelTerminal'), enabled: true },
    { id: 'todo' as const, icon: ListChecks, label: t('rightPanelTodo'), enabled: true },
    ...(planEnabled ? [{ id: 'plan' as const, icon: ListChecks, label: t('rightPanelPlan'), enabled: true }] : [])
  ]
  return <section className="workspace-tool-selector" aria-label={t('workspaceAddTab', { defaultValue: '打开工具或文件' })}>
    <h2>{t('workspaceSelectTool', { defaultValue: '打开工作面' })}</h2>
    <div>{items.map(({ id, icon: Icon, label, enabled }) => <button key={id} type="button" disabled={!enabled} onClick={() => onOpen(id)}><Icon size={17} aria-hidden="true" /><span>{label}</span></button>)}</div>
  </section>
}
