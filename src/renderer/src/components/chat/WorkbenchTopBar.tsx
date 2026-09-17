import { workspaceShortcutLabels } from '@shared/native-office'
import type { ReactElement } from 'react'
import { PanelBottomClose, PanelBottomOpen, PanelRightClose, PanelRightOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { shellToolbarIconButtonClass, ToolbarTooltip } from '../shell/ShellToolbar'

/** Compatibility entry names; workspace-tabs-store owns the actual dock and instances. */
export type RightPanelMode =
  | 'documents' | 'files' | 'todo' | 'changes' | 'browser' | 'file'
  | 'plan' | 'summary' | 'sdd-ai' | 'child-agent' | null

type Props = {
  workspaceOpen: boolean
  onToggleWorkspace: () => void
  terminalOpen: boolean
  onToggleTerminal: () => void
}

export function WorkbenchTopBar({ workspaceOpen, onToggleWorkspace, terminalOpen, onToggleTerminal }: Props): ReactElement {
  const { t } = useTranslation('common')
  const workspaceLabel = t(workspaceOpen ? 'workbenchCollapse' : 'workbenchOpen', { defaultValue: workspaceOpen ? '收起工作区' : '展开工作区' })
  const terminalLabel = `${t(terminalOpen ? 'collapse' : 'open', { defaultValue: terminalOpen ? '收起' : '展开' })}${t('rightPanelTerminal')}`
  const RightIcon = workspaceOpen ? PanelRightClose : PanelRightOpen
  const BottomIcon = terminalOpen ? PanelBottomClose : PanelBottomOpen
  return (
    <div className="chat-workbench-topbar ds-no-drag flex shrink-0 items-center gap-1" role="group" aria-label={t('workspaceLayout', { defaultValue: '布局' })}>
      <ToolbarTooltip label={`${workspaceLabel} (${workspaceShortcutLabels.workspace})`}>
        <button type="button" onClick={onToggleWorkspace} className={shellToolbarIconButtonClass(workspaceOpen)} aria-label={workspaceLabel} aria-pressed={workspaceOpen} aria-controls="workbench-right-workspace">
          <RightIcon className="ds-toolbar-icon-svg" aria-hidden="true" />
        </button>
      </ToolbarTooltip>
      <ToolbarTooltip label={`${terminalLabel} (${workspaceShortcutLabels.terminal})`}>
        <button type="button" onClick={onToggleTerminal} className={shellToolbarIconButtonClass(terminalOpen)} aria-label={terminalLabel} aria-pressed={terminalOpen}>
          <BottomIcon className="ds-toolbar-icon-svg" aria-hidden="true" />
        </button>
      </ToolbarTooltip>
    </div>
  )
}
