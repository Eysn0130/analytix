import type { ReactElement } from 'react'
import { FilePlus2, FolderOpen, MessageSquare, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

type WriteWorkspaceStartProps = {
  onAskAssistant: () => void
  onCreateDraft: () => void
  onPickWorkspace: () => void
  onRefreshWorkspace: () => void
  workspaceName: string
  workspacePathLabel: string
}

export function WriteWorkspaceStart(props: WriteWorkspaceStartProps): ReactElement {
  const { t } = useTranslation('common')
  const actions = [
    { label: t('writeCreateFile'), icon: FilePlus2, action: props.onCreateDraft },
    { label: t('selectWorkspace'), icon: FolderOpen, action: props.onPickWorkspace },
    { label: t('writeStartAskAiPrompt'), icon: MessageSquare, action: props.onAskAssistant },
    { label: t('refresh'), icon: RefreshCw, action: props.onRefreshWorkspace }
  ]
  return (
    <div className="flex h-full flex-col items-center justify-center gap-6 overflow-auto p-6">
      <p className="text-sm text-ds-muted">{props.workspaceName}</p>
      <div className="flex w-full max-w-xs flex-col gap-2">
        {actions.map(({ label, icon: Icon, action }) => (
          <button key={label} type="button" onClick={action} className="flex items-center gap-3 rounded-lg border border-ds-border-muted px-4 py-3 text-left text-sm hover:bg-ds-hover focus-visible:ring-2 focus-visible:ring-accent">
            <Icon className="h-4 w-4 shrink-0" />{label}
          </button>
        ))}
      </div>
    </div>
  )
}
