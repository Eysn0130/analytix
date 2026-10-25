import type { ReactElement } from 'react'
import { useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import {
  FileQuestion,
  NewChat,
  Search
} from '../../design/AnalytixUiIcons'
import type { CaseProjectIndexStatus, NormalizedCaseProject, NormalizedThread } from '../../agent/types'
import { useChatStore, type SettingsRouteSection } from '../../store/chat-store'
import type { SddDraft } from '../../sdd/sdd-draft-store'
import type { DataAnalysisItemId } from '../../data-analysis/DataAnalysisSurface'
import type {
  ClawImChannelV1,
} from '@shared/app-settings'
import {
  ClawSidebarContent
} from './SidebarClaw'
import type { ClawImDialogMode } from './SidebarClawDialogHelpers'
import { ClawAddImDialog } from './SidebarClawDialog'
import { ConnectPhoneSidebarPanel } from './ConnectPhoneView'
import { SidebarProjectsSection } from './SidebarProjectsSection'
import { isClawThread } from '../../store/chat-store-helpers'
import { isSddAssistantThread } from '../../sdd/sdd-thread-registry'
import {
  SidebarCommandRow,
  SidebarFrame,
  SidebarIconButton
} from '../sidebar/SidebarPrimitives'

type Props = {
  threads: NormalizedThread[]
  caseProjects: NormalizedCaseProject[]
  caseProjectIndexStatus: CaseProjectIndexStatus
  caseProjectThreadsById: Record<string, NormalizedThread[]>
  caseProjectLoadingById: Record<string, boolean>
  caseProjectErrorsById: Record<string, string | null>
  caseProjectExpandedById: Record<string, boolean>
  activeThreadId: string | null
  activeView: 'chat' | 'write' | 'claw' | 'schedule'
  connectPhoneSidebarOpen: boolean
  runtimeReady: boolean
  threadSearch: string
  showArchivedThreads: boolean
  onThreadSearchChange: (query: string) => void
  onSetCaseProjectExpanded: (caseProjectId: string, expanded: boolean) => void
  onSelectThread: (id: string) => void
  onRenameThread: (id: string, title: string) => Promise<void>
  onArchiveThread: (id: string) => Promise<void>
  onDeleteThread: (id: string) => Promise<void>
  onRestoreThread: (id: string) => Promise<void>
  onOpenSideChat: (threadId: string) => Promise<void> | void
  onForkThread: (threadId: string) => Promise<void> | void
  onNewChat: () => void
  onNewChatInWorkspace: (workspaceRoot: string) => void
  onNewRequirement: () => void
  onOpenRequirementDraft: (draft: SddDraft) => void
  activeDataAnalysis: { workspaceRoot: string; itemId: DataAnalysisItemId } | null
  onOpenDataAnalysis: (workspaceRoot: string, itemId: DataAnalysisItemId) => void
  onOpenSettings: (section?: SettingsRouteSection) => void
  onToggleConnectPhone: () => void
  onScheduleOpen: () => void
}

export function Sidebar({
  threads,
  caseProjects,
  caseProjectIndexStatus,
  caseProjectThreadsById,
  caseProjectLoadingById,
  caseProjectErrorsById,
  caseProjectExpandedById,
  activeThreadId,
  activeView,
  connectPhoneSidebarOpen,
  runtimeReady,
  threadSearch,
  showArchivedThreads,
  onThreadSearchChange,
  onSetCaseProjectExpanded,
  onSelectThread,
  onRenameThread,
  onArchiveThread,
  onDeleteThread,
  onRestoreThread,
  onOpenSideChat,
  onForkThread,
  onNewChat,
  onNewChatInWorkspace,
  onNewRequirement,
  onOpenRequirementDraft,
  activeDataAnalysis,
  onOpenDataAnalysis,
  onOpenSettings,
  onToggleConnectPhone,
  onScheduleOpen
}: Props): ReactElement {
  const { t, i18n } = useTranslation('common')
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const codeWorkspaceRoots = useChatStore((s) => s.codeWorkspaceRoots)
  const chooseWorkspace = useChatStore((s) => s.chooseWorkspace)
  const deleteWorkspace = useChatStore((s) => s.deleteWorkspace)
  const busy = useChatStore((s) => s.busy)
  const watchTurnCompletion = useChatStore((s) => s.watchTurnCompletion)
  const unreadThreadIds = useChatStore((s) => s.unreadThreadIds)
  const clawChannels = useChatStore((s) => s.clawChannels)
  const activeClawChannelId = useChatStore((s) => s.activeClawChannelId)
  const selectClawChannel = useChatStore((s) => s.selectClawChannel)
  const addClawChannel = useChatStore((s) => s.addClawChannel)
  const deleteClawChannel = useChatStore((s) => s.deleteClawChannel)
  const resetClawChannelSession = useChatStore((s) => s.resetClawChannelSession)
  const threadSearchResults = useChatStore((s) => s.threadSearchResults)
  const displayedThreads = useMemo(() => {
    const query = threadSearch.trim()
    if (!query) return threads
    if (threadSearchResults?.query !== query) return []
    return threadSearchResults.threads.filter((thread) =>
      !isClawThread(thread, clawChannels) && !isSddAssistantThread(thread)
    )
  }, [clawChannels, threadSearch, threadSearchResults, threads])

  const [projectSearchOpen, setProjectSearchOpen] = useState(false)
  const [imDialogMode, setImDialogMode] = useState<ClawImDialogMode | null>(null)
  const imDialogPortalTarget = typeof document === 'undefined' ? null : document.body

  const activeClawChannel = useMemo(
    () => clawChannels.find((channel) => channel.id === activeClawChannelId) ?? clawChannels[0] ?? null,
    [clawChannels, activeClawChannelId]
  )

  return (
    <>
    <SidebarFrame
      title={t('appName')}
      header={<>
        <span className="ds-sidebar-brand-name">{t('appName')}</span>
        {activeView !== 'claw' && !connectPhoneSidebarOpen ? <SidebarIconButton
          onClick={() => {
            setProjectSearchOpen((open) => !open)
            if (!projectSearchOpen) requestAnimationFrame(() =>
              document.querySelector<HTMLInputElement>('#sidebar-thread-search input')?.focus())
          }}
          active={projectSearchOpen || Boolean(threadSearch.trim())}
          title={t('sidebarSearchThreads')} ariaLabel={t('sidebarSearchThreads')}
        ><Search className="h-4 w-4" aria-hidden /></SidebarIconButton> : null}
      </>}
    >
      <div className="ds-no-drag flex flex-col px-1">
        {activeView !== 'claw' && activeView !== 'schedule' ? (
          <>
            <SidebarCommandRow
              icon={<NewChat className="h-4 w-4" />}
              label={t('newAgent')}
              onClick={runtimeReady ? onNewChat : undefined}
              disabled={!runtimeReady}
              disabledHint={t('runtimeActionNeedsConnection')}
              variant="accent"
            />
            <SidebarCommandRow
              icon={<FileQuestion className="h-4 w-4" strokeWidth={1.9} />}
              label={t('sddNewRequirement')}
              onClick={runtimeReady ? onNewRequirement : undefined}
              disabled={!runtimeReady}
              disabledHint={t('runtimeActionNeedsConnection')}
              variant="accent"
            />
          </>
        ) : null}
      </div>

      <div className="ds-no-drag mx-1 my-1" />

      {connectPhoneSidebarOpen ? (
        <ConnectPhoneSidebarPanel
          channels={clawChannels}
          onAddProvider={async (provider, agentProfile, platformAccount, options) => {
            await addClawChannel(provider, agentProfile, platformAccount, options)
            onToggleConnectPhone()
          }}
          onDisconnect={(channelId) => deleteClawChannel(channelId)}
          onOpenSettings={() => onOpenSettings('claw')}
        />
      ) : activeView === 'claw' ? (
        <ClawSidebarContent
          channels={clawChannels}
          activeChannelId={activeClawChannelId}
          activeThreadId={activeThreadId}
          runtimeReady={runtimeReady}
          onSelectChannel={(channelId) => void selectClawChannel(channelId)}
          onAddChannel={() => setImDialogMode('add')}
          onResetChannel={(channelId) => void resetClawChannelSession(channelId)}
          onOpenSettings={() => setImDialogMode('edit')}
          t={t}
        />
      ) : (
        <SidebarProjectsSection
	        threads={displayedThreads}
	        caseProjects={caseProjects}
	        caseProjectIndexStatus={caseProjectIndexStatus}
	        caseProjectThreadsById={caseProjectThreadsById}
	        caseProjectLoadingById={caseProjectLoadingById}
	        caseProjectErrorsById={caseProjectErrorsById}
	        caseProjectExpandedById={caseProjectExpandedById}
	        activeView={activeView === 'write' ? 'write' : 'chat'}
        activeThreadId={activeThreadId}
        runtimeReady={runtimeReady}
        searchQuery={threadSearch}
          searchOpen={projectSearchOpen}
        showArchived={showArchivedThreads}
        workspaceRoot={workspaceRoot}
        workspaceRoots={codeWorkspaceRoots}
        busy={busy}
        watchTurnCompletion={watchTurnCompletion}
        unreadThreadIds={unreadThreadIds}
        locale={i18n.language}
        onPickWorkspace={() => void chooseWorkspace()}
        onRemoveWorkspace={deleteWorkspace}
        onCreateThreadInWorkspace={onNewChatInWorkspace}
        onOpenRequirementDraft={onOpenRequirementDraft}
        activeDataAnalysisEntry={activeDataAnalysis}
        onOpenDataAnalysis={onOpenDataAnalysis}
        onSelectThread={onSelectThread}
        onRenameThread={onRenameThread}
        onArchiveThread={onArchiveThread}
        onDeleteThread={onDeleteThread}
        onRestoreThread={onRestoreThread}
        onOpenSideChat={onOpenSideChat}
        onForkThread={onForkThread}
        onOpenSchedule={onScheduleOpen}
	        onSearchQueryChange={onThreadSearchChange}
	        onSetCaseProjectExpanded={onSetCaseProjectExpanded}
	        t={t}
	      />
      )}

    </SidebarFrame>

    {imDialogMode && imDialogPortalTarget ? createPortal(
      <ClawAddImDialog
        mode={imDialogMode}
        initialProvider={activeClawChannel?.provider}
        initialChannelId={imDialogMode === 'edit' ? activeClawChannel?.id : undefined}
        channels={clawChannels}
        onClose={() => setImDialogMode(null)}
        onAddProvider={(provider, agentProfile, platformAccount, options) =>
          addClawChannel(provider, agentProfile, platformAccount, options)
        }
        onDeleteChannel={(channelId) => deleteClawChannel(channelId)}
        t={t}
      />,
      imDialogPortalTarget
    ) : null}
    </>
  )
}
