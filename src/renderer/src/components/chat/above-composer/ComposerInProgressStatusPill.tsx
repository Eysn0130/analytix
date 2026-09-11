import { Ban, Check, Circle, Loader2, X } from 'lucide-react'
import { type CSSProperties, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThreadTodoList, ThreadTodoStatus } from '../../../agent/types'
import type { CurrentTurnDiffSummary } from '../../../lib/composer-change-summary'

export type ComposerTodoPlanStep = {
  step: string
  status: ThreadTodoStatus
}

export type ComposerTodoPlanPillState = {
  steps: ComposerTodoPlanStep[]
  currentIndex: number
  stepNumber: number
  stepCount: number
  completedCount: number
  completedPercent: number
}

type Props = {
  todos: ThreadTodoList | null
  diffSummary: CurrentTurnDiffSummary | null
  busy: boolean
  currentTurnId: string | null
  hasBlockingRequest?: boolean
  showDiffSummary?: boolean
  onOpenChanges?: () => void
  className?: string
}

function todoPlanStepStatusKey(status: ThreadTodoStatus): string {
  switch (status) {
    case 'in_progress':
      return 'progressStepStatusRunning'
    case 'completed':
      return 'progressStepStatusDone'
    case 'pending':
      return 'progressStepStatusPending'
    case 'failed':
      return 'progressStepStatusFailed'
    case 'canceled':
      return 'progressStepStatusCanceled'
  }
}

export function buildTodoPlanPillState(todos: ThreadTodoList | null): ComposerTodoPlanPillState | null {
  const items = todos?.items ?? []
  if (items.length === 0) return null

  const steps = items.map((item) => ({
    step: item.content,
    status: item.status
  }))
  const inProgressIndex = steps.findIndex((item) => item.status === 'in_progress')
  const firstIncompleteIndex = steps.findIndex((item) => item.status !== 'completed')
  const currentIndex = inProgressIndex >= 0
    ? inProgressIndex
    : firstIncompleteIndex >= 0
      ? firstIncompleteIndex
      : steps.length - 1
  const completedCount = steps.reduce(
    (count, item) => count + (item.status === 'completed' ? 1 : 0),
    0
  )

  return {
    steps,
    currentIndex,
    stepNumber: currentIndex + 1,
    stepCount: steps.length,
    completedCount,
    completedPercent: (completedCount / steps.length) * 100
  }
}

export function todoPlanBelongsToCurrentTurn(
  todos: ThreadTodoList | null,
  currentTurnId: string | null
): boolean {
  if (!todos) return false
  return Boolean(todos.turnId && todos.turnId === currentTurnId)
}

function TodoPlanDonut({ percent }: { percent: number }): ReactElement {
  const clampedPercent = Math.max(0, Math.min(100, percent))
  const style = {
    background: `conic-gradient(rgb(59 130 246) ${clampedPercent}%, rgba(147,197,253,0.28) 0)`
  } satisfies CSSProperties

  return (
    <span
      aria-hidden
      className="relative h-[18px] w-[18px] shrink-0 rounded-full p-[2px]"
      style={style}
    >
      <span className="block h-full w-full rounded-full bg-white/95 shadow-[inset_0_0_0_1px_rgba(255,255,255,0.6)] dark:bg-ds-card" />
    </span>
  )
}

function TodoPlanStepIcon({ status }: { status: ThreadTodoStatus }): ReactElement {
  if (status === 'in_progress') {
    return (
      <span aria-hidden className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden text-blue-500">
        <Loader2 className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" strokeWidth={2} />
      </span>
    )
  }

  if (status === 'completed') {
    return (
      <span aria-hidden className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden text-ds-faint">
        <Check className="h-3.5 w-3.5" strokeWidth={2} />
      </span>
    )
  }

  if (status === 'failed' || status === 'canceled') {
    const Icon = status === 'failed' ? X : Ban
    return (
      <span aria-hidden className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden text-red-500">
        <Icon className="h-3.5 w-3.5" strokeWidth={2} />
      </span>
    )
  }

  return (
    <span aria-hidden className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden text-ds-faint">
      <Circle className="h-3.5 w-3.5" strokeWidth={2} />
    </span>
  )
}

export function TodoPlanPillSegment({
  state,
  threadId
}: {
  state: ComposerTodoPlanPillState
  threadId: string
}): ReactElement {
  const { t } = useTranslation('common')
  const tooltipId = `todo-plan-progress-${threadId}`
  const label = t('todoPlanPillProgress', {
    stepNumber: state.stepNumber,
    stepCount: state.stepCount
  })

  return (
    <span className="group/todo-plan relative inline-flex max-w-full min-w-0">
      <span
        role="status"
        aria-describedby={tooltipId}
        aria-label={label}
        title={label}
        className="inline-flex max-w-full min-w-0 cursor-default items-center gap-1.5 text-[13px] font-medium leading-none text-ds-muted transition-colors hover:text-ds-ink"
      >
        <TodoPlanDonut percent={state.completedPercent} />
        <span className="whitespace-nowrap tabular-nums">{label}</span>
      </span>

      <span
        id={tooltipId}
        role="tooltip"
        className="pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 w-max max-w-[min(20rem,calc(100vw-16px))] -translate-x-1/2 rounded-xl border border-ds-border bg-white/95 p-2 text-ds-ink opacity-0 shadow-[0_18px_48px_rgba(20,47,95,0.18)] backdrop-blur-xl transition-opacity duration-150 before:absolute before:inset-x-0 before:-bottom-2 before:h-2 before:content-[''] group-hover/todo-plan:pointer-events-auto group-hover/todo-plan:opacity-100 dark:bg-ds-card/95"
      >
        <span className="vertical-scroll-fade-mask flex max-h-[min(22rem,calc(100vh-16px))] min-h-0 flex-col gap-2 overflow-y-auto px-2 py-2 [--edge-fade-distance:1rem]">
          {state.steps.map((step, index) => (
            <span
              key={`${index}-${step.step}`}
              className="flex max-w-80 min-w-0 items-start gap-2"
            >
              <TodoPlanStepIcon status={step.status} />
              <span className="sr-only">{t(todoPlanStepStatusKey(step.status))}</span>
              <span
                className={[
                  'min-w-0 max-w-72 break-words text-[13px] leading-4',
                  step.status === 'completed' ? 'text-ds-faint' : 'text-ds-muted'
                ].join(' ')}
              >
                {step.step}
              </span>
            </span>
          ))}
        </span>
      </span>
    </span>
  )
}

export function TurnDiffSummarySegment({
  summary,
  showLeadingSeparator,
  onOpenChanges
}: {
  summary: CurrentTurnDiffSummary
  showLeadingSeparator: boolean
  onOpenChanges?: () => void
}): ReactElement {
  const { t } = useTranslation('common')
  const content = (
    <>
      <span className="block min-w-0 truncate">
        {t('turnDiffFilesChanged', { count: summary.fileCount })}
      </span>
      <span className="font-mono text-[12px] leading-none text-ds-diff-added">
        {t('turnDiffLinesAdded', { count: summary.added })}
      </span>
      <span className="font-mono text-[12px] leading-none text-ds-diff-removed">
        {t('turnDiffLinesDeleted', { count: summary.removed })}
      </span>
    </>
  )

  return (
    <span className="flex min-w-0 items-center gap-2">
      {showLeadingSeparator ? (
        <span aria-hidden className="text-ds-muted">
          ·
        </span>
      ) : null}
      {onOpenChanges ? (
        <button
          type="button"
          onClick={onOpenChanges}
          className="flex min-w-0 cursor-pointer items-center gap-1 rounded-sm text-[13px] font-medium leading-none text-ds-muted transition hover:text-ds-ink focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ds-accent"
          aria-label={t('turnDiffFilesChanged', { count: summary.fileCount })}
          title={t('turnDiffFilesChanged', { count: summary.fileCount })}
        >
          {content}
        </button>
      ) : (
        <span className="flex min-w-0 items-center gap-1 text-[13px] font-medium leading-none text-ds-muted">
          {content}
        </span>
      )}
    </span>
  )
}

export function ComposerInProgressStatusPill({
  todos,
  diffSummary,
  busy,
  currentTurnId,
  hasBlockingRequest = false,
  showDiffSummary = true,
  onOpenChanges,
  className = ''
}: Props): ReactElement | null {
  const todoState = todoPlanBelongsToCurrentTurn(todos, currentTurnId)
    ? buildTodoPlanPillState(todos)
    : null
  const visibleDiffSummary = showDiffSummary ? diffSummary : null
  const shouldRender =
    busy &&
    currentTurnId != null &&
    !hasBlockingRequest &&
    (todoState != null || visibleDiffSummary != null)

  if (!shouldRender) return null

  return (
    <div
      className={[
        'pointer-events-none relative z-20 flex w-full justify-center px-3 pb-2',
        className
      ].filter(Boolean).join(' ')}
    >
      <div className="pointer-events-auto relative z-10 w-fit max-w-full min-w-0 overflow-visible rounded-3xl">
        <div className="flex w-max max-w-full min-w-0 items-center gap-2 rounded-3xl border border-ds-border/80 bg-white/80 px-3 py-1.5 text-ds-ink shadow-[0_10px_26px_rgba(20,47,95,0.14)] backdrop-blur-xl dark:bg-ds-card/80">
          {todoState ? (
            <TodoPlanPillSegment state={todoState} threadId={todos?.threadId ?? 'active'} />
          ) : null}
          {visibleDiffSummary ? (
            <TurnDiffSummarySegment
              summary={visibleDiffSummary}
              showLeadingSeparator={todoState != null}
              onOpenChanges={onOpenChanges}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}
