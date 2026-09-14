import type { WorkspaceEntry } from '@shared/workspace-file'
import { isWriteWorkspaceFilePath } from '@shared/write-text-file'
import type { ReactElement } from 'react'
import type { TFunction } from 'i18next'
import type { ComposerFileReference } from '../../lib/composer-file-references'
import { isWorkspaceTextPreviewPath } from '../../lib/workspace-text-preview'
import { WriteSidebar } from '../write/WriteSidebar'
import { PanelCollapseButton } from '../workbench/PanelCollapseButton'

export type ChatFileTreeReference = ComposerFileReference & { type: 'file' | 'directory' }

type Props = {
  workspaceRoot: string
  selectedPath?: string | null
  onPreviewFile: (path: string, workspaceRoot: string) => void
  onAddReference: (reference: ChatFileTreeReference) => void
  onCollapse?: () => void
  t: TFunction
  fill?: boolean
}

export function isChatFileTreeIgnoredDirectory(name: string): boolean {
  return new Set(['.git', '.hg', '.svn', '.deepseek', 'node_modules']).has(name.toLowerCase())
}

export function isChatFileTreePreviewableEntry(entry: WorkspaceEntry): boolean {
  const path = entry.path || entry.name
  return entry.type === 'file' && (isWriteWorkspaceFilePath(path) || isWorkspaceTextPreviewPath(path))
}

export function formatChatFileTreeUnsupportedMessage(name: string): string {
  return `${name} is not a supported preview.`
}

// Both the Files tab and document drawer share this browser and the existing
// workspace store. Opening an entry is delegated to the Workbench dispatcher.
export function ChatFileTreePanel({ workspaceRoot, selectedPath, onPreviewFile, onAddReference, onCollapse, t, fill = false }: Props): ReactElement | null {
  if (!workspaceRoot.trim()) return null
  return (
    <div className={`ds-no-drag flex min-h-0 flex-col ${fill ? 'h-full' : 'max-h-[34vh]'}`}>
      {onCollapse ? <div className="ds-right-panel-topbar border-b border-ds-border-muted">
        <span className="ds-right-panel-title flex-1">{t('fileTreeTitle')}</span>
        <PanelCollapseButton onClick={onCollapse} ariaLabel={t('rightPanelCollapse')} title={t('rightPanelCollapse')} />
      </div> : null}
      <WriteSidebar initialWorkspaceRoot={workspaceRoot} selectedPath={selectedPath}
        onOpenFile={(root, path) => onPreviewFile(path, root)} onAddReference={onAddReference} />
    </div>
  )
}
