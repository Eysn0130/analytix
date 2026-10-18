import { Target } from '../../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import type { ReactElement } from 'react'
import type { ThreadGoal } from '../../../agent/types'
import type { GoalActivationV1 } from '../../../../../../packages/runtime/src/contracts/goal'

export function GoalFacts({ goal, activation, historical = false }: {
  goal: ThreadGoal; activation?: GoalActivationV1 | null; historical?: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const unknown = t('goalFactUnknown', { defaultValue: 'Unknown' })
  const knownActivation = activation != null && activation.id === goal.id && activation.revision === goal.revision ? activation.activation : undefined
  return <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-ds-faint">
    <span>{t('goalRoundsStarted', { defaultValue: 'Rounds started' })}: {goal.roundsStarted ?? unknown}</span>
    <span>{t('goalMaxRounds', { defaultValue: 'Round limit' })}: {goal.maxGoalRounds ?? unknown}</span>
    {!historical ? <span>{t('goalActivation', { defaultValue: 'Automatic continuation' })}: {knownActivation === 'armed'
      ? t('goalActivationArmed', { defaultValue: 'On' }) : knownActivation === 'disarmed'
        ? t('goalActivationDisarmed', { defaultValue: 'Off' }) : unknown}</span> : null}
    {goal.blockedReason ? <span className="basis-full break-words">{goal.blockedReason}</span> : null}
  </div>
}

/** Immutable transcript record. It never borrows the current Goal or activation. */
export function GoalBar({ goal, operation }: { goal: ThreadGoal | null; operation: string }): ReactElement {
  const { t } = useTranslation('common')
  const operationLabel: Record<string,string> = { create:'Created',edit:'Edited',pause:'Paused',resume:'Resumed',clear:'Cleared',
    legacy_update:'Updated',legacy_clear:'Cleared',evidence:'Evidence recorded',model_update:'Updated',round_limit:'Round limit reached',round_started:'Round started' }
  return <details className="my-2 min-w-0 text-[13px] leading-5" data-analytix-goal-history>
    <summary className="flex cursor-pointer items-center gap-2 text-ds-muted">
      <Target className="h-3.5 w-3.5 shrink-0" aria-hidden />
      <span className="shrink-0">{goal ? t(`goalStatusShort.${goal.status}`) : t('goalClearedTimeline')}</span>
      {goal ? <span className="truncate">{goal.objective}</span> : null}
    </summary>
    {goal ? <div className="space-y-2 py-2 pl-5"><p className="break-words">{goal.objective}</p><GoalFacts goal={goal} historical /></div> : null}
    <span className="sr-only">{t(`goalHistoryOperation.${operation}`, { defaultValue: operationLabel[operation] ?? 'Updated' })}</span>
  </details>
}
