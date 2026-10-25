import { ChevronDown, PauseCircle, Pencil, PlayCircle, Target, Trash2 } from '../../../design/AnalytixUiIcons'
import { useEffect, useId, useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThreadGoal } from '../../../agent/types'
import { AboveComposerPanelRow } from './AboveComposerPanelRow'
import { GoalFacts } from './GoalBar'
import { useChatStore } from '../../../store/chat-store'
import { countedGoalRefV1 } from '../../../../../../packages/runtime/src/contracts/threads'
import type { GoalActivationV1 } from '../../../../../../packages/runtime/src/contracts/goal'

export function shouldPauseGoal(goal: ThreadGoal | null, activation: GoalActivationV1 | null | undefined): boolean {
  return goal?.status === 'active' && (countedGoalRefV1(goal) === null ||
    activation != null && activation.id === goal.id && activation.revision === goal.revision && activation.activation === 'armed')
}

export function goalRoundLimitExhausted(goal: ThreadGoal | null): boolean {
  return countedGoalRefV1(goal) !== null && goal!.roundsStarted! >= goal!.maxGoalRounds!
}

type Props = {
  goal: ThreadGoal | null
  elapsedLabel?: string | null
  onEdit: () => void
  onToggleStatus: () => void
  onClear: () => void
}

export function ComposerGoalRow({
  goal,
  elapsedLabel,
  onEdit,
  onToggleStatus,
  onClear
}: Props): ReactElement | null {
  const { t } = useTranslation('common')
  const [expanded, setExpanded] = useState(false)
  const [budget, setBudget] = useState(String(goal?.maxGoalRounds ?? ''))
  const [saving, setSaving] = useState(false)
  const activation = useChatStore(state => state.activeGoalActivation)
  const activeThreadId = useChatStore(state => state.activeThreadId)
  const setGoal = useChatStore(state => state.setActiveThreadGoal)
  useEffect(() => setBudget(String(goal?.maxGoalRounds ?? '')), [goal?.id, goal?.revision, goal?.maxGoalRounds])
  const detailsId = useId()
  if (!goal) return null
  const counted = countedGoalRefV1(goal) !== null
  const active = shouldPauseGoal(goal, activation)
  const complete = goal.status === 'complete'
  const canManage = activeThreadId === goal.threadId
  const statusKey = goal.status

  return (
    <AboveComposerPanelRow className="ds-composer-goal-row">
      <div className="flex min-w-0 flex-col">
        <button type="button" className="ds-composer-goal-summary" aria-expanded={expanded}
          aria-controls={detailsId} title={goal.objective} onClick={() => setExpanded(value => !value)}>
          <Target className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.9} aria-hidden />
          <span className="shrink-0 font-medium">{t(`goalStatusShort.${statusKey}`)}</span>
          <span className="min-w-0 flex-1 truncate text-ds-muted">{goal.objective}</span>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-ds-faint ${expanded ? 'rotate-180' : ''}`} aria-hidden />
        </button>
        <div id={detailsId} hidden={!expanded} className="ds-composer-goal-details">
          <p className="break-words text-[13px] leading-5 text-ds-muted">{goal.objective}</p>
          <GoalFacts goal={goal} activation={activation} />
          {counted && canManage && !complete ? <form className="flex items-center gap-2 pt-1" onSubmit={event => {
            event.preventDefault()
            const value = Number(budget)
            if (!Number.isSafeInteger(value) || value < 1 || value < (goal.roundsStarted ?? 0)) return
            setSaving(true)
            void setGoal(goal.objective, { maxGoalRounds: value }).finally(() => setSaving(false))
          }}>
            <label className="text-xs text-ds-faint" htmlFor={`${detailsId}-budget`}>{t('goalMaxRounds', { defaultValue: 'Round limit' })}</label>
            <input id={`${detailsId}-budget`} type="number" min={Math.max(1, goal.roundsStarted ?? 0)} step={1} value={budget}
              onChange={event => setBudget(event.target.value)} disabled={saving} className="w-20 rounded border border-ds-border bg-transparent px-2 py-1 text-xs" />
            <button type="submit" disabled={saving || budget === String(goal.maxGoalRounds)} className="text-xs text-ds-muted disabled:opacity-50">
              {t('save', { defaultValue: 'Save' })}
            </button>
          </form> : null}
          <div className="flex items-center justify-between gap-2 pt-1">
            <span className="text-xs text-ds-faint">{elapsedLabel}</span>
            <div className="flex shrink-0 items-center gap-0.5">
              {!complete ? <button
                type="button"
                onClick={onEdit}
                className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('goalActionEdit')}
                title={t('goalActionEdit')}
              >
                <Pencil className="h-3.5 w-3.5" strokeWidth={1.9} />
              </button> : null}
              {!complete ? <button
                type="button"
                onClick={onToggleStatus}
                disabled={!active && goalRoundLimitExhausted(goal)}
                className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={active ? t('goalActionPause') : t('goalActionResume')}
                title={active ? t('goalActionPause') : t('goalActionResume')}
              >
                {active ? (
                  <PauseCircle className="h-3.5 w-3.5" strokeWidth={1.9} />
                ) : (
                  <PlayCircle className="h-3.5 w-3.5" strokeWidth={1.9} />
                )}
              </button> : null}
              <button
                type="button"
                onClick={onClear}
                className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('goalActionClear')}
                title={t('goalActionClear')}
              >
                <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
              </button>
            </div>
          </div>
        </div>
      </div>
    </AboveComposerPanelRow>
  )
}
