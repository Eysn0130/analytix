import type { ReactElement } from 'react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  ExternalLink,
  Loader2,
  Play,
  RotateCw,
  Workflow
} from 'lucide-react'
import { getProvider } from '../../agent/registry'
import {
  ANALYTIX_CREATE_LOOP_DEFINITION,
  runCreateLoopWorkflow,
  type CreateLoopRun,
  type CreateLoopRuntimeProvider,
  type CreateLoopRunStatus,
  type CreateLoopStepRun,
  type CreateLoopStepStatus
} from '../../workflow/create-loop-runtime'

type Props = {
  leftSidebarCollapsed: boolean
  onOpenThread?: (threadId: string) => void
  initialRuns?: CreateLoopRun[]
  providerFactory?: () => CreateLoopRuntimeProvider
}

function statusTone(status: CreateLoopRunStatus | CreateLoopStepStatus): string {
  switch (status) {
    case 'running':
      return 'border-amber-400/35 bg-amber-500/10 text-amber-800 dark:text-amber-100'
    case 'waiting':
      return 'border-sky-400/35 bg-sky-500/10 text-sky-800 dark:text-sky-100'
    case 'success':
      return 'border-emerald-400/35 bg-emerald-500/10 text-emerald-800 dark:text-emerald-100'
    case 'error':
      return 'border-red-400/35 bg-red-500/10 text-red-800 dark:text-red-100'
    case 'pending':
    default:
      return 'border-[var(--ds-border-subtle)] bg-ds-subtle text-ds-muted'
  }
}

function StatusIcon({ status }: { status: CreateLoopRunStatus | CreateLoopStepStatus }): ReactElement {
  if (status === 'running') return <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
  if (status === 'waiting') return <AlertTriangle className="h-3.5 w-3.5" strokeWidth={1.9} />
  if (status === 'success') return <CheckCircle2 className="h-3.5 w-3.5" strokeWidth={1.9} />
  if (status === 'error') return <AlertTriangle className="h-3.5 w-3.5" strokeWidth={1.9} />
  return <Circle className="h-3.5 w-3.5" strokeWidth={1.9} />
}

function formatStepStatus(step: CreateLoopStepRun, fallback: string): string {
  if (step.status === 'waiting' && step.waitingFor) return step.waitingFor === 'approval' ? 'approval' : 'input'
  return step.status || fallback
}

function truncate(value: string, max = 420): string {
  const text = value.trim()
  return text.length > max ? `${text.slice(0, max).trim()}...` : text
}

export function WorkflowCreateLoopView({
  leftSidebarCollapsed,
  onOpenThread,
  initialRuns = [],
  providerFactory
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const [objective, setObjective] = useState('')
  const [runs, setRuns] = useState<CreateLoopRun[]>(initialRuns)
  const [activeRunId, setActiveRunId] = useState(initialRuns[0]?.id ?? '')
  const [runningRunId, setRunningRunId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const activeRun = useMemo(
    () => runs.find((run) => run.id === activeRunId) ?? runs[0] ?? null,
    [activeRunId, runs]
  )
  const currentSteps = activeRun?.steps ?? ANALYTIX_CREATE_LOOP_DEFINITION.steps.map((step) => ({
    id: step.id,
    title: step.title,
    status: 'pending' as const
  }))

  const upsertRun = (next: CreateLoopRun): void => {
    setRuns((current) => {
      const index = current.findIndex((run) => run.id === next.id)
      if (index < 0) return [next, ...current].slice(0, 12)
      const copy = [...current]
      copy[index] = next
      return copy
    })
    setActiveRunId(next.id)
  }

  const runWorkflow = async (existingRun?: CreateLoopRun): Promise<void> => {
    const nextObjective = existingRun?.objective ?? objective.trim()
    if (!nextObjective) {
      setError(t('workflowObjectiveRequired'))
      return
    }
    setError(null)
    setRunningRunId(existingRun?.id ?? 'new')
    try {
      const result = await runCreateLoopWorkflow({
        provider: providerFactory ? providerFactory() : getProvider(),
        objective: nextObjective,
        existingRun,
        model: existingRun?.model,
        providerId: existingRun?.providerId,
        workspace: existingRun?.workspace,
        onUpdate: upsertRun
      })
      upsertRun(result)
      if (!existingRun) setObjective('')
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : String(runError))
    } finally {
      setRunningRunId(null)
    }
  }

  const busy = runningRunId !== null
  const canRun = objective.trim().length > 0 && !busy
  const canResume = activeRun && (activeRun.status === 'waiting' || activeRun.status === 'error') && !busy

  return (
    <div className="flex h-full min-h-0 flex-col bg-ds-main text-ds-text">
      <div
        className={`ds-no-drag ds-shell-controls-safe-motion flex shrink-0 items-center gap-3 border-b border-[var(--ds-border-subtle)] px-4 py-3 ${
          leftSidebarCollapsed ? 'ds-shell-controls-safe-inset' : ''
        }`}
      >
        <Workflow className="h-5 w-5 text-accent" strokeWidth={1.9} />
        <div className="min-w-0">
          <h1 className="truncate text-[15px] font-semibold text-ds-text">{t('workflowCreateLoop')}</h1>
          <p className="truncate text-xs text-ds-muted">{t('workflowCreateLoopSubtitle')}</p>
        </div>
      </div>

      <div className="flex min-h-0 flex-1">
        <div className="min-h-0 w-[340px] shrink-0 overflow-y-auto border-r border-[var(--ds-border-subtle)] px-4 py-4">
          <label className="text-xs font-medium uppercase tracking-[0.08em] text-ds-muted" htmlFor="workflow-objective">
            {t('workflowObjective')}
          </label>
          <textarea
            id="workflow-objective"
            value={objective}
            onChange={(event) => setObjective(event.currentTarget.value)}
            placeholder={t('workflowObjectivePlaceholder')}
            className="mt-2 h-32 w-full resize-none rounded-lg border border-[var(--ds-border-subtle)] bg-[var(--ds-input-bg)] px-3 py-2 text-sm outline-none transition focus:border-accent/50 focus:ring-2 focus:ring-accent/15"
          />
          <button
            type="button"
            onClick={() => void runWorkflow()}
            disabled={!canRun}
            className="mt-3 inline-flex h-9 w-full items-center justify-center gap-2 rounded-lg bg-accent px-3 text-sm font-medium text-white shadow-sm transition hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Play className="h-4 w-4" strokeWidth={2} />
            {t('workflowRun')}
          </button>
          {error ? (
            <div className="mt-3 rounded-lg border border-red-400/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-100">
              {error}
            </div>
          ) : null}

          <div className="mt-5">
            <div className="mb-2 text-xs font-medium uppercase tracking-[0.08em] text-ds-muted">
              {t('workflowRecentRuns')}
            </div>
            <div className="space-y-2">
              {runs.length === 0 ? (
                <div className="rounded-lg border border-dashed border-[var(--ds-border-subtle)] px-3 py-6 text-center text-xs text-ds-muted">
                  {t('workflowNoRuns')}
                </div>
              ) : runs.map((run) => (
                <button
                  key={run.id}
                  type="button"
                  onClick={() => setActiveRunId(run.id)}
                  className={`w-full rounded-lg border px-3 py-2 text-left text-sm transition ${
                    activeRun?.id === run.id
                      ? 'border-accent/45 bg-accent/10'
                      : 'border-[var(--ds-border-subtle)] bg-ds-panel hover:bg-ds-subtle'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="min-w-0 truncate font-medium">{run.objective}</span>
                    <span className={`inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] ${statusTone(run.status)}`}>
                      <StatusIcon status={run.status} />
                      {run.status}
                    </span>
                  </div>
                  <div className="mt-1 truncate text-xs text-ds-muted">{run.lastMessage || run.threadId}</div>
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto px-5 py-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="text-xs font-medium uppercase tracking-[0.08em] text-ds-muted">
                {t('workflowRunDetail')}
              </div>
              <h2 className="mt-1 truncate text-xl font-semibold text-ds-text">
                {activeRun?.objective || t('workflowNoActiveRun')}
              </h2>
            </div>
            <div className="flex items-center gap-2">
              {activeRun?.threadId && onOpenThread ? (
                <button
                  type="button"
                  onClick={() => onOpenThread(activeRun.threadId)}
                  className="inline-flex h-9 items-center gap-2 rounded-lg border border-[var(--ds-border-subtle)] bg-ds-panel px-3 text-sm font-medium hover:bg-ds-subtle"
                >
                  <ExternalLink className="h-4 w-4" strokeWidth={1.9} />
                  {t('workflowOpenThread')}
                </button>
              ) : null}
              {canResume ? (
                <button
                  type="button"
                  onClick={() => void runWorkflow(activeRun)}
                  className="inline-flex h-9 items-center gap-2 rounded-lg bg-accent px-3 text-sm font-medium text-white hover:bg-accent/90"
                >
                  <RotateCw className="h-4 w-4" strokeWidth={1.9} />
                  {activeRun.status === 'waiting' ? t('workflowResume') : t('workflowRetry')}
                </button>
              ) : null}
            </div>
          </div>

          <div className="mt-5 grid gap-3">
            {currentSteps.map((step) => (
              <div key={step.id} className="rounded-lg border border-[var(--ds-border-subtle)] bg-ds-panel p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] ${statusTone(step.status)}`}>
                        <StatusIcon status={step.status} />
                        {formatStepStatus(step, t('workflowPending'))}
                      </span>
                      <h3 className="truncate text-sm font-semibold text-ds-text">{step.title}</h3>
                    </div>
                    <p className="mt-2 text-sm text-ds-muted">
                      {ANALYTIX_CREATE_LOOP_DEFINITION.steps.find((item) => item.id === step.id)?.description}
                    </p>
                  </div>
                  {step.turnId ? (
                    <span className="shrink-0 rounded-md bg-ds-subtle px-2 py-1 text-[11px] text-ds-muted">
                      {step.turnId}
                    </span>
                  ) : null}
                </div>
                {step.error ? (
                  <pre className="mt-3 max-h-44 overflow-auto whitespace-pre-wrap rounded-lg bg-red-500/10 p-3 text-xs text-red-800 dark:text-red-100">
                    {step.error}
                  </pre>
                ) : step.output ? (
                  <pre className="mt-3 max-h-56 overflow-auto whitespace-pre-wrap rounded-lg bg-ds-subtle p-3 text-xs text-ds-text">
                    {truncate(step.output)}
                  </pre>
                ) : step.message ? (
                  <div className="mt-3 text-xs text-ds-muted">{step.message}</div>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
