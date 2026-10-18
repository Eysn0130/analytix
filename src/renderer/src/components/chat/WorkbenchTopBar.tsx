import { workspaceShortcutLabels } from '@shared/native-office'
import type { ReactElement } from 'react'
import { PanelBottomClose, PanelBottomOpen, Pin } from '../../design/AnalytixUiIcons'
import { WorkspacePanelIcon } from '../workbench/WorkspaceTabs'
import { useTranslation } from 'react-i18next'
import { shellToolbarIconButtonClass, ToolbarTooltip } from '../shell/ShellToolbar'

/** Compatibility entry names; workspace-tabs-store owns the actual dock and instances. */
export type RightPanelMode =
  | 'documents' | 'files' | 'todo' | 'changes' | 'browser' | 'file'
  | 'plan' | 'sdd-ai' | 'child-agent' | 'schedule-task' | null

type Props = {
  workspaceOpen: boolean
  onToggleWorkspace: () => void
  terminalOpen: boolean
  terminalConstrained?: boolean
  onToggleTerminal: () => void
  terminalAvailable?: boolean
  summaryAvailable?: boolean
  summaryOpen?: boolean
  onToggleSummary?: () => void
}

export function WorkbenchTopBar({ workspaceOpen, onToggleWorkspace, terminalOpen, terminalConstrained = false, onToggleTerminal, terminalAvailable = true, summaryAvailable = false, summaryOpen = false, onToggleSummary }: Props): ReactElement {
  const { t } = useTranslation('common')
  const workspaceLabel = t('workbenchOpen', { defaultValue: '展开工作区' })
  const terminalLabel = t(terminalOpen ? 'workbenchCollapseTerminal' : 'workbenchOpenTerminal')
  const terminalHeightHint = terminalConstrained ? t('workbenchTerminalHeightLimited') : undefined
  const BottomIcon = terminalOpen ? PanelBottomClose : PanelBottomOpen
  return (
    <div className="chat-workbench-topbar ds-no-drag flex shrink-0 items-center gap-1" role="group" aria-label={t('workspaceLayout')}>
      {summaryAvailable && onToggleSummary ? <ToolbarTooltip label={t('rightPanelSummary')}>
        <button id="workbench-summary-toggle" type="button" onClick={onToggleSummary} className={shellToolbarIconButtonClass(summaryOpen)}
          aria-label={t('rightPanelSummary')} aria-expanded={summaryOpen} aria-controls="workbench-thread-summary">
          <Pin className="ds-toolbar-icon-svg" aria-hidden="true" />
        </button>
      </ToolbarTooltip> : null}
      {terminalAvailable ? <ToolbarTooltip label={`${terminalHeightHint ?? terminalLabel} (${workspaceShortcutLabels.terminal})`}>
        <button id="workbench-terminal-toggle" type="button" onClick={onToggleTerminal} className={shellToolbarIconButtonClass(terminalOpen)} aria-label={terminalLabel} aria-description={terminalHeightHint} aria-pressed={terminalOpen} aria-expanded={terminalOpen && !terminalConstrained} aria-controls="workbench-terminal-panel">
          <BottomIcon className="ds-toolbar-icon-svg" aria-hidden="true" />
        </button>
      </ToolbarTooltip> : null}
      {!workspaceOpen ? (
        <ToolbarTooltip label={`${workspaceLabel} (${workspaceShortcutLabels.workspace})`}>
          <button id="workbench-workspace-expand" type="button" data-sidebar-right-expand aria-keyshortcuts="Control+Shift+B Meta+Shift+B" onClick={onToggleWorkspace} className={shellToolbarIconButtonClass()} aria-label={workspaceLabel} aria-expanded={false} aria-controls="workbench-right-workspace">
            <WorkspacePanelIcon className="ds-toolbar-icon-svg" />
          </button>
        </ToolbarTooltip>
      ) : null}
    </div>
  )
}
