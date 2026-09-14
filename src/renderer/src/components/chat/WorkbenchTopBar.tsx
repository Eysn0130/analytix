import type { ReactElement, SVGProps } from 'react'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import type { EditorInfo } from '@shared/editor'
import {
  Check,
  ChevronDown,
  Code2,
  Files,
  FilePenLine,
  FolderOpen,
  MessageCirclePlus,
  Terminal
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'
import { readPreferredEditorId, writePreferredEditorId } from '../../lib/editor-preferences'
import { shellToolbarIconButtonClass, ToolbarTooltip } from '../shell/ShellToolbar'

export type RightPanelMode =
  | 'documents'
  | 'todo'
  | 'changes'
  | 'browser'
  | 'file'
  | 'plan'
  | 'summary'
  | 'sdd-ai'
  | 'child-agent'
  | null

type Props = {
  rightPanelMode: RightPanelMode
  onToggleRightPanelMode: (mode: Exclude<RightPanelMode, null>) => void
  planPanelEnabled?: boolean
  terminalOpen?: boolean
  onToggleTerminal?: () => void
  sideChatOpen?: boolean
  sideChatEnabled?: boolean
  fileTreeOpen?: boolean
  fileTreeEnabled?: boolean
  onToggleFileTree?: () => void
  onOpenSideChat?: () => void
  onPreloadRightPanelMode?: (mode: Exclude<RightPanelMode, null>) => void
}

export function WorkbenchTopBar({
  rightPanelMode,
  onToggleRightPanelMode,
  planPanelEnabled = false,
  terminalOpen = false,
  onToggleTerminal,
  sideChatOpen = false,
  sideChatEnabled = true,
  fileTreeOpen = false,
  fileTreeEnabled = true,
  onToggleFileTree,
  onOpenSideChat,
  onPreloadRightPanelMode
}: Props): ReactElement {
  const { t } = useTranslation(['common', 'settings'])
  const [editors, setEditors] = useState<EditorInfo[]>([])
  const [selectedEditorId, setSelectedEditorId] = useState(() => readPreferredEditorId() ?? '')
  const [editorMenuOpen, setEditorMenuOpen] = useState(false)
  const [failedIconIds, setFailedIconIds] = useState<Set<string>>(() => new Set())
  const editorMenuRef = useRef<HTMLDivElement>(null)
  const items = [
    { mode: 'documents' as const, label: t('workbenchDocuments'), icon: FilePenLine },
    { mode: 'todo' as const, label: t('rightPanelTodo'), icon: AnalytixIconRegistry.icons.taskList },
    { mode: 'summary' as const, label: t('rightPanelSummary'), icon: PinnedSummaryIcon },
    ...(planPanelEnabled
      ? [{ mode: 'plan' as const, label: t('rightPanelPlan'), icon: AnalytixIconRegistry.icons.plan }]
      : []),
    { mode: 'changes' as const, label: t('rightPanelChanges'), icon: AnalytixIconRegistry.icons.edit },
    { mode: 'browser' as const, label: t('rightPanelBrowser'), icon: AnalytixIconRegistry.icons.browser }
  ]
  const selectedEditor = useMemo(
    () => editors.find((editor) => editor.id === selectedEditorId) ?? editors[0],
    [editors, selectedEditorId]
  )
  const editorTooltipLabel = selectedEditor
    ? t('editorPickerTitleWithEditor', { editor: selectedEditor.label })
    : t('editorPickerTitle')

  useEffect(() => {
    let cancelled = false
    if (typeof window.analytix?.workspace?.listEditors !== 'function') return

    void window.analytix.workspace.listEditors()
      .then((result) => {
        if (cancelled) return
        const available = result.editors.filter((editor) => editor.available)
        const stored = readPreferredEditorId()
        const nextId =
          stored && available.some((editor) => editor.id === stored)
            ? stored
            : result.defaultEditorId
        setEditors(available)
        setSelectedEditorId(nextId)
        writePreferredEditorId(nextId)
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!editorMenuOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (target instanceof Node && editorMenuRef.current?.contains(target)) return
      setEditorMenuOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    return () => window.removeEventListener('pointerdown', onPointerDown)
  }, [editorMenuOpen])

  const chooseEditor = (editor: EditorInfo): void => {
    setSelectedEditorId(editor.id)
    writePreferredEditorId(editor.id)
    setEditorMenuOpen(false)
  }

  const markEditorIconFailed = (editorId: string): void => {
    setFailedIconIds((prev) => {
      if (prev.has(editorId)) return prev
      const next = new Set(prev)
      next.add(editorId)
      return next
    })
  }

  const renderEditorIcon = (editor: EditorInfo | null | undefined, className: string): ReactElement => {
    const Icon =
      editor?.kind === 'terminal' ? Terminal : editor?.kind === 'viewer' ? FolderOpen : Code2

    if (editor?.iconDataUrl && !failedIconIds.has(editor.id)) {
      return (
        <img
          src={editor.iconDataUrl}
          alt=""
          aria-hidden="true"
          className={`${className} shrink-0 rounded-[4px] object-contain`}
          onError={() => markEditorIconFailed(editor.id)}
        />
      )
    }

    return <Icon className={`${className} shrink-0`} strokeWidth={1.8} />
  }

  return (
    <div className="chat-workbench-topbar ds-no-drag flex min-w-0 shrink-0 flex-nowrap items-center justify-end gap-1">
      <div ref={editorMenuRef} className="relative flex items-center">
        <ToolbarTooltip label={editorTooltipLabel} hidden={editorMenuOpen}>
          <button
            type="button"
            onClick={() => setEditorMenuOpen((value) => !value)}
            className={shellToolbarIconButtonClass(editorMenuOpen, 'ds-toolbar-icon-button-menu')}
            data-state={editorMenuOpen ? 'open' : 'closed'}
            aria-label={t('editorPickerTitle')}
            aria-expanded={editorMenuOpen}
          >
            {renderEditorIcon(selectedEditor, 'ds-toolbar-icon-svg')}
            <ChevronDown className="ds-toolbar-icon-svg opacity-60" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>

        {editorMenuOpen ? (
          <div
            role="menu"
            className="ds-card-strong ds-no-drag absolute right-0 top-full z-50 mt-2 w-64 overflow-hidden rounded-[18px] border border-ds-border py-1.5 shadow-[0_18px_52px_rgba(20,47,95,0.18)] backdrop-blur-xl dark:shadow-[0_22px_58px_rgba(0,0,0,0.38)]"
          >
            <div className="border-b border-ds-border-muted px-3 pb-2 pt-1.5 text-[11px] font-semibold text-ds-faint">
              {t('editorPickerMenuTitle')}
            </div>
            {editors.map((editor) => {
              const active = editor.id === selectedEditor?.id
              return (
                <button
                  key={editor.id}
                  type="button"
                  onClick={() => chooseEditor(editor)}
                  className={`flex w-full items-center gap-3 px-3 py-2.5 text-left text-[14px] transition ${
                    active
                      ? 'bg-ds-hover text-ds-ink'
                      : 'text-ds-muted hover:bg-ds-hover/70 hover:text-ds-ink'
                  }`}
                >
                  {renderEditorIcon(editor, 'h-4 w-4')}
                  <span className="min-w-0 flex-1 truncate">{editor.label}</span>
                  {editor.supportsLine ? (
                    <span className="shrink-0 rounded-md bg-accent/10 px-1.5 py-0.5 text-[10px] font-semibold text-accent">
                      {t('editorLineBadge')}
                    </span>
                  ) : null}
                  {active ? <Check className="h-4 w-4 shrink-0 text-accent" strokeWidth={2} /> : null}
                </button>
              )
            })}
          </div>
        ) : null}
      </div>

      {onOpenSideChat ? (
        <ToolbarTooltip label={t('sidePanelOpen')}>
          <button
            type="button"
            onClick={onOpenSideChat}
            disabled={!sideChatEnabled}
            className={shellToolbarIconButtonClass(sideChatOpen)}
            aria-label={t('sidePanelOpen')}
            aria-pressed={sideChatOpen}
          >
            <MessageCirclePlus className="ds-toolbar-icon-svg" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>
      ) : null}

      {onToggleFileTree ? (
        <ToolbarTooltip label={t('rightPanelFiles')}>
          <button
            type="button"
            onClick={onToggleFileTree}
            disabled={!fileTreeEnabled}
            className={shellToolbarIconButtonClass(fileTreeOpen)}
            aria-label={t('rightPanelFiles')}
            aria-pressed={fileTreeOpen}
          >
            <Files className="ds-toolbar-icon-svg" />
          </button>
        </ToolbarTooltip>
      ) : null}

      {items.map((item) => {
        const active = rightPanelMode === item.mode
        const Icon = item.icon
        const isChanges = item.mode === 'changes'
        return (
          <Fragment key={item.mode}>
            <ToolbarTooltip label={item.label}>
              <button
                type="button"
                onPointerEnter={() => onPreloadRightPanelMode?.(item.mode)}
                onPointerDown={() => onPreloadRightPanelMode?.(item.mode)}
                onFocus={() => onPreloadRightPanelMode?.(item.mode)}
                onClick={() => onToggleRightPanelMode(item.mode)}
                className={shellToolbarIconButtonClass(active)}
                aria-label={item.label}
                aria-pressed={active}
              >
                <Icon className="ds-toolbar-icon-svg" />
              </button>
            </ToolbarTooltip>
            {isChanges && onToggleTerminal ? (
              <ToolbarTooltip label={t('rightPanelTerminal')}>
                <button
                  type="button"
                  onClick={onToggleTerminal}
                  className={shellToolbarIconButtonClass(terminalOpen)}
                  aria-label={t('rightPanelTerminal')}
                  aria-pressed={terminalOpen}
                >
                  <AnalytixIconRegistry.icons.terminal className="ds-toolbar-icon-svg" />
                </button>
              </ToolbarTooltip>
            ) : null}
          </Fragment>
        )
      })}
    </div>
  )
}

function PinnedSummaryIcon(props: SVGProps<SVGSVGElement>): ReactElement {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={20}
      height={20}
      fill="currentColor"
      viewBox="0 0 20 20"
      {...props}
    >
      <path d="M5.693 11.056a2.71 2.71 0 0 1 2.432 2.694l-.015.277a2.71 2.71 0 0 1-2.694 2.432l-.276-.015a2.71 2.71 0 0 1-2.418-2.417l-.014-.277a2.709 2.709 0 0 1 2.708-2.708l.277.014Zm-.277 1.316a1.378 1.378 0 1 0 0 2.757 1.378 1.378 0 0 0 0-2.757Zm11.384.727a.665.665 0 0 1 0 1.302l-.134.014h-5.833a.665.665 0 0 1 0-1.33h5.833l.135.014ZM5.693 3.556A2.71 2.71 0 0 1 8.125 6.25l-.015.277A2.71 2.71 0 0 1 5.416 8.96l-.276-.015a2.71 2.71 0 0 1-2.418-2.417l-.014-.277a2.709 2.709 0 0 1 2.708-2.708l.277.014Zm-.277 1.316a1.378 1.378 0 1 0 .001 2.757 1.378 1.378 0 0 0-.001-2.757Zm11.384.727a.665.665 0 0 1 0 1.302l-.134.014h-5.833a.665.665 0 0 1 0-1.33h5.833l.135.014Z" />
    </svg>
  )
}
