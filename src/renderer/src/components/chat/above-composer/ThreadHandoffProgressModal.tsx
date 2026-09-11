import { AlertTriangle, CheckCircle2, GitBranch, X, XCircle } from 'lucide-react'
import { type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { threadHandoffStepsWithRollback, type ThreadHandoffOperation } from '@shared/thread-handoff'
import { useChatStore } from '../../../store/chat-store'
import { AboveComposerPanelRow } from './AboveComposerPanelRow'
import { AboveComposerActionRow } from './AboveComposerActionRow'
import { ThreadHandoffOperationStepRow } from './ThreadHandoffStepRow'

function titleForOperation(t: ReturnType<typeof useTranslation>['t'], operation: ThreadHandoffOperation): string {
  if (operation.direction === 'to-worktree') return t('threadHandoffProgressTitleWorktree')
  if (operation.direction === 'to-host-worktree') return t('threadHandoffProgressTitleHostWorktree')
  return t('threadHandoffProgressTitleLocal')
}

function terminalTitle(t: ReturnType<typeof useTranslation>['t'], operation: ThreadHandoffOperation): string {
  if (operation.status === 'success') return t('threadHandoffSuccessTitle')
  if (operation.status === 'warning') return t('threadHandoffWarningTitle')
  return t('threadHandoffErrorTitle')
}

export function ThreadHandoffInlineProgress(): ReactElement | null {
  const { t } = useTranslation('common')
  const operations = useChatStore((s) => s.threadHandoffOperations)
  const activeOperationId = useChatStore((s) => s.activeThreadHandoffOperationId)
  const openOperation = operations.find((operation) => operation.id === activeOperationId)
    ?? operations.find((operation) => operation.status === 'running' || operation.status === 'queued')
    ?? null
  if (!openOperation) return null
  const steps = threadHandoffStepsWithRollback(openOperation.steps)
  const runningStep = steps.find((step) => step.status === 'running')
    ?? steps.find((step) => step.status === 'failed')
    ?? steps[steps.length - 1]

  return (
    <AboveComposerPanelRow>
      <div className="px-3 py-2">
        <AboveComposerActionRow
          icon={<GitBranch className="h-3.5 w-3.5 text-ds-faint" strokeWidth={1.9} />}
          title={<span className="truncate text-[13px] font-medium text-ds-ink">{titleForOperation(t, openOperation)}</span>}
          meta={<span className="text-[12px]">{openOperation.status}</span>}
        />
        {runningStep ? (
          <div className="mt-1.5">
            <ThreadHandoffOperationStepRow compact operation={openOperation} step={runningStep} />
          </div>
        ) : null}
      </div>
    </AboveComposerPanelRow>
  )
}

export function ThreadHandoffProgressModal(): ReactElement | null {
  const operations = useChatStore((s) => s.threadHandoffOperations)
  const activeOperationId = useChatStore((s) => s.activeThreadHandoffOperationId)
  const retryThreadHandoff = useChatStore((s) => s.retryThreadHandoff)
  const closeThreadHandoffOperation = useChatStore((s) => s.closeThreadHandoffOperation)
  const operation = operations.find((item) => item.id === activeOperationId) ?? null
  if (!operation) return null

  return (
    <ThreadHandoffProgressModalContent
      operation={operation}
      onClose={(operationId) => void closeThreadHandoffOperation(operationId)}
      onRetry={(operationId) => void retryThreadHandoff(operationId)}
    />
  )
}

export function ThreadHandoffProgressModalContent({
  operation,
  onClose,
  onRetry
}: {
  operation: ThreadHandoffOperation
  onClose: (operationId: string) => void
  onRetry: (operationId: string) => void
}): ReactElement {
  const { t } = useTranslation('common')
  const isRunning = operation.status === 'queued' || operation.status === 'running'
  const steps = threadHandoffStepsWithRollback(operation.steps)
  const icon = operation.status === 'success'
    ? <CheckCircle2 className="h-7 w-7 text-emerald-600 dark:text-emerald-400" strokeWidth={1.8} />
    : operation.status === 'warning'
      ? <AlertTriangle className="h-7 w-7 text-amber-600 dark:text-amber-400" strokeWidth={1.8} />
      : operation.status === 'error'
        ? <XCircle className="h-7 w-7 text-ds-danger" strokeWidth={1.8} />
        : <GitBranch className="h-7 w-7 text-ds-ink" strokeWidth={1.8} />

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-black/18 px-4 py-6 backdrop-blur-[2px]">
      <div className="max-h-[calc(100dvh-2rem)] w-full max-w-[520px] overflow-y-auto rounded-2xl border border-ds-border bg-ds-card px-6 py-5 shadow-[0_22px_70px_rgba(20,47,95,0.22)]">
        <div className="flex items-start gap-4">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-ds-hover/60">
            {icon}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-base font-semibold leading-6 text-ds-ink">
              {isRunning ? titleForOperation(t, operation) : terminalTitle(t, operation)}
            </div>
            <div className="mt-1 text-sm leading-6 text-ds-muted">
              {isRunning ? t('threadHandoffProgressSubtitle') : operation.errorMessage || operation.warningMessage || t('threadHandoffSuccessSubtitle')}
            </div>
          </div>
          <button
            type="button"
            className="rounded-lg p-1.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
            onClick={() => onClose(operation.id)}
            aria-label={t('threadHandoffClose')}
            title={t('threadHandoffClose')}
          >
            <X className="h-4 w-4" strokeWidth={1.9} />
          </button>
        </div>

        {isRunning ? (
          <div className="mt-5 flex flex-col gap-4">
            {steps.map((step) => (
              <ThreadHandoffOperationStepRow key={step.id} operation={operation} step={step} />
            ))}
          </div>
        ) : null}

        {operation.status === 'error' && operation.execOutput?.output ? (
          <pre className="mt-5 max-h-[36vh] overflow-auto whitespace-pre-wrap rounded-xl border border-ds-border bg-ds-main px-3 py-2 font-mono text-xs leading-5 text-ds-ink">
            {operation.execOutput.command ? `$ ${operation.execOutput.command}\n` : ''}
            {operation.execOutput.output}
          </pre>
        ) : null}

        {operation.status === 'error' ? (
          <div className="mt-5 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => onClose(operation.id)}
              className="h-8 rounded-full border border-ds-border px-4 text-sm font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
            >
              {t('threadHandoffClose')}
            </button>
            <button
              type="button"
              onClick={() => onRetry(operation.id)}
              className="h-8 rounded-full bg-ds-ink px-4 text-sm font-semibold text-ds-main transition hover:opacity-85"
            >
              {t('threadHandoffRetry')}
            </button>
          </div>
        ) : null}
      </div>
    </div>
  )
}
