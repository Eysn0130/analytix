import { ChevronDown, PauseCircle, Pencil, PlayCircle, Target, Trash2 } from '../../../design/AnalytixUiIcons'
import { useId, useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThreadGoal } from '../../../agent/types'
import { AboveComposerPanelRow } from './AboveComposerPanelRow'

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
  const detailsId = useId()
  if (!goal || goal.status === 'complete') return null
  const active = goal.status === 'active'
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
          <div className="flex items-center justify-between gap-2 pt-1">
            <span className="text-xs text-ds-faint">{elapsedLabel}</span>
            <div className="flex shrink-0 items-center gap-0.5">
              <button
                type="button"
                onClick={onEdit}
                className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={t('goalActionEdit')}
                title={t('goalActionEdit')}
              >
                <Pencil className="h-3.5 w-3.5" strokeWidth={1.9} />
              </button>
              <button
                type="button"
                onClick={onToggleStatus}
                className="flex h-7 w-7 items-center justify-center rounded-lg text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                aria-label={active ? t('goalActionPause') : t('goalActionResume')}
                title={active ? t('goalActionPause') : t('goalActionResume')}
              >
                {active ? (
                  <PauseCircle className="h-3.5 w-3.5" strokeWidth={1.9} />
                ) : (
                  <PlayCircle className="h-3.5 w-3.5" strokeWidth={1.9} />
                )}
              </button>
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
