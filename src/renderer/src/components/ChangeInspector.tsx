import type { ReactElement } from 'react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { FileEdit, RotateCcw } from 'lucide-react'
import type { ChatBlock, ToolBlock } from '../agent/types'
import {
  countDiffStats,
  extractDiffFilePath,
  extractUnifiedDiffText,
  formatFilePathForDisplay,
} from '../lib/diff-stats'
import { useChatStore } from '../store/chat-store'
import { DiffView } from './DiffView'
import { RewindPlanApplyControls } from './RewindPlanApplyControls'
import { PanelCollapseButton } from './workbench/PanelCollapseButton'

/**
 * Right-side change inspector — file_change items only.
 * Selecting a row reveals the unified patch in the bottom panel.
 */
export function ChangeInspector({
  blocks,
  className,
  onCollapse
}: {
  blocks: ChatBlock[]
  className?: string
  onCollapse: () => void
}): ReactElement {
  const { t } = useTranslation('common')
  const selectedId = useChatStore((s) => s.inspectorSelectedId)
  const selectInspectorItem = useChatStore((s) => s.selectInspectorItem)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)

  const fileChanges = useMemo<ToolBlock[]>(() => {
    return blocks.flatMap((block): ToolBlock[] => {
      if (!(block.kind === 'tool' && block.toolKind === 'file_change')) {
        return []
      }

      const detailText = extractUnifiedDiffText(block.detail)
      const rewindPlan = block.meta?.rewindPlan
      if (rewindPlan) {
        return [
          {
            ...block,
            detail: '',
            filePath: t('rewindPlanCardTitle', { count: rewindPlan.summary.fileCount })
          }
        ]
      }
      if (!detailText) return []

      return [
        {
          ...block,
          detail: detailText,
          filePath: extractDiffFilePath(detailText, block.filePath)
        }
      ]
    })
  }, [blocks, t])

  useEffect(() => {
    if (fileChanges.length === 0 && selectedId !== null) {
      selectInspectorItem(null)
      return
    }
    if (selectedId && !fileChanges.some((b) => b.id === selectedId)) {
      selectInspectorItem(fileChanges[fileChanges.length - 1]?.id ?? null)
    }
  }, [fileChanges, selectedId, selectInspectorItem])

  const active = fileChanges.find((b) => b.id === selectedId) ?? fileChanges[fileChanges.length - 1]
  const inspectorSummary = fileChanges.length > 0
    ? t('inspectorSummaryFiles', { count: fileChanges.length })
    : t('inspectorEmpty')

  return (
    <aside
      className={`ds-no-drag ds-panel-ghost flex flex-col border-l border-ds-border-muted backdrop-blur-xl ${className ?? ''}`}
    >
      <div className="ds-right-panel-topbar border-b border-ds-border-muted">
        <div className="ds-right-panel-title-group">
          <FileEdit className="ds-right-panel-title-icon" strokeWidth={1.75} />
          <span className="ds-right-panel-title">{t('inspectorTitle')}</span>
          <span className="ds-right-panel-subtitle" title={inspectorSummary}>
            {inspectorSummary}
          </span>
        </div>
        <PanelCollapseButton
          onClick={onCollapse}
          ariaLabel={t('rightPanelCollapse')}
          title={t('rightPanelCollapse')}
        />
      </div>

      <div className="flex min-h-0 flex-1 flex-col">
        {fileChanges.length === 0 ? (
          <div className="flex flex-1 items-center justify-center px-6 py-10 text-center">
            <div>
              <FileEdit className="mx-auto h-7 w-7 text-ds-faint" strokeWidth={1.25} />
              <div className="mt-3 text-[12px] font-medium text-ds-muted">
                {t('inspectorEmptyTitle')}
              </div>
              <div className="mt-1 text-[11px] leading-6 text-ds-faint">{t('inspectorEmpty')}</div>
            </div>
          </div>
        ) : (
          <>
            <div className="max-h-[42%] min-h-0 overflow-y-auto py-2">
              <ul className="divide-y divide-ds-border-muted/60">
                {fileChanges.map((b) => {
                  const stats = countDiffStats(b.detail)
                  const rewindPlan = b.meta?.rewindPlan
                  const displayPath = formatFilePathForDisplay(b.filePath, workspaceRoot)
                  return (
                    <li key={b.id}>
                      <button
                        type="button"
                        onClick={() => selectInspectorItem(b.id)}
                        className={`flex w-full items-start gap-2 px-4 py-2.5 text-left transition ${
                          active?.id === b.id
                            ? 'bg-ds-hover text-ds-ink'
                            : 'text-ds-ink hover:bg-ds-hover/70'
                        }`}
                      >
                        {rewindPlan ? (
                          <RotateCcw
                            className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${
                              b.status === 'error' ? 'text-red-700' : 'text-ds-muted'
                            }`}
                            strokeWidth={1.75}
                          />
                        ) : (
                          <FileEdit
                            className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${
                              b.status === 'error' ? 'text-red-700' : 'text-ds-muted'
                            }`}
                            strokeWidth={1.75}
                          />
                        )}
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-[12px] text-ds-ink">
                            {displayPath ?? t('toolActionFile')}
                          </div>
                          {rewindPlan ? (
                            <div className="mt-0.5 text-[10px] text-ds-faint">
                              {t('rewindPlanNoApply')}
                            </div>
                          ) : stats ? (
                            <div className="mt-0.5 flex gap-2 text-[10px] font-mono">
                              <span className="text-ds-diff-added">
                                +{stats.added}
                              </span>
                              <span className="text-ds-diff-removed">
                                -{stats.removed}
                              </span>
                            </div>
                          ) : null}
                        </div>
                        {b.status === 'running' ? (
                          <span className="rounded-full bg-amber-200/40 px-2 py-0.5 text-[10px] font-medium text-amber-900 dark:bg-amber-700/30 dark:text-amber-100">
                            {t('inspectorStatusRunning')}
                          </span>
                        ) : null}
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>

            <div className="ds-panel-strip flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden border-t border-ds-border-muted">
              {active?.meta?.rewindPlan ? (
                <RewindPlanInspectorDetails plan={active.meta.rewindPlan} />
              ) : active?.detail ? (
                <DiffView patch={active.detail} maxHeight={9999} className="h-full min-w-0 rounded-none border-0" />
              ) : (
                <div className="ds-surface-soft flex h-full items-center justify-center border border-dashed border-ds-border-muted px-4 py-6 text-center text-[11px] leading-6 text-ds-muted">
                  {t('inspectorSelectHint')}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </aside>
  )
}

function RewindPlanInspectorDetails({
  plan
}: {
  plan: NonNullable<ToolBlock['meta']>['rewindPlan']
}): ReactElement | null {
  const { t } = useTranslation('common')
  if (!plan) return null
  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-ds-card-muted/35 px-4 py-4">
      <div className="rounded-md border border-ds-border-muted bg-ds-card px-3 py-3">
        <div className="text-[12px] font-semibold text-ds-ink">
          {t('rewindPlanCardTitle', { count: plan.summary.fileCount })}
        </div>
        <div className="mt-1 text-[11px] leading-5 text-ds-muted">
          {t('rewindPlanNoApply')}
        </div>
        <div className="mt-3 grid grid-cols-3 gap-2 text-[10px]">
          <span className="rounded-md bg-ds-card-muted px-2 py-1 text-ds-muted">
            {t('rewindPlanReadyCount', { count: plan.summary.readyFileCount })}
          </span>
          <span className="rounded-md bg-ds-card-muted px-2 py-1 text-ds-muted">
            {t('rewindPlanManualReviewCount', { count: plan.summary.manualReviewFileCount })}
          </span>
          <span className="rounded-md bg-ds-card-muted px-2 py-1 text-ds-muted">
            {t('rewindPlanBlockedCount', { count: plan.summary.blockedFileCount })}
          </span>
        </div>
        {plan.conversation ? (
          <div className="mt-3 text-[11px] leading-5 text-ds-muted">
            {t('rewindPlanConversationCounts', {
              events: plan.summary.removedEventCount,
              turns: plan.summary.removedTurnCount
            })}
          </div>
        ) : null}
        <RewindPlanApplyControls plan={plan} />
      </div>

      <ul className="mt-3 divide-y divide-ds-border-muted rounded-md border border-ds-border-muted bg-ds-card">
        {plan.files.map((file) => (
          <li key={`${file.relativePath}:${file.action}`} className="px-3 py-2.5">
            <div className="flex min-w-0 items-start gap-2">
              <span className={`mt-1 h-2 w-2 shrink-0 rounded-full ${
                file.status === 'blocked'
                  ? 'bg-red-500'
                  : file.status === 'manual_review'
                    ? 'bg-amber-500'
                    : 'bg-emerald-500'
              }`} />
              <div className="min-w-0 flex-1">
                <div className="truncate font-mono text-[11.5px] text-ds-ink" title={file.relativePath}>
                  {file.relativePath}
                </div>
                <div className="mt-1 text-[11px] leading-5 text-ds-muted">
                  {file.action} · {file.reason}
                </div>
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
