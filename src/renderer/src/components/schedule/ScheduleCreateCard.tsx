/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { DshClockOutlineRegular } from '../chat/presentation/DshToolIcons'
import css from './ScheduleCreateCard.module.css'

/** Presentation fields supplied only after the Host receipt has been admitted. */
export type ScheduleCreationCardModel = { taskId: string; title: string; frequency: string }
export type ScheduleCreateCardLabels = { open: string; deleted: string; openLabel: (title: string) => string }

export function ScheduleCreateCard({ creation, currentTask, openTaskDetail, labels }: {
  creation: ScheduleCreationCardModel
  currentTask?: Pick<ScheduleCreationCardModel, 'title' | 'frequency'> | null
  openTaskDetail: (taskId: string, title: string) => void
  labels: ScheduleCreateCardLabels
}) {
  const task = currentTask ?? creation
  const deleted = currentTask === null
  const title = currentTask == null ? creation.title : currentTask.title
  const open = () => openTaskDetail(creation.taskId, title)
  return <div className={css.card} data-tool="schedule_create" data-task-id={creation.taskId}>
    {!deleted ? <button type="button" className={css.cardOpen} aria-label={labels.openLabel(title)} onClick={open} /> : null}
    <div className={css.row}>
      <span className={css.leading}><DshClockOutlineRegular size={18} /></span>
      <span className={css.body}>
        <span className={css.title}>{title}</span>
        <span className={css.frequency}>{deleted ? labels.deleted : task.frequency}</span>
      </span>
      {!deleted ? <button type="button" className={css.openButton} onClick={open}>{labels.open}</button> : null}
    </div>
  </div>
}
