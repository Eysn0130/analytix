/* Selected creation tail adapted from pinned DSH 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../agent/types'
import type { ToolResultLocalDisplayV1, ToolResultLocalScheduleTaskV1 } from '../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ProtectedToolResult } from '../chat/presentation/ProtectedToolResult'
import { openTaskDetail } from '../workbench-schedule-controller'
import { ScheduleCreateCard } from './ScheduleCreateCard'
import { scheduleTaskSummary } from './ScheduleTasksView'
import { nativeScheduleOperation, scheduleCreationCalls } from './schedule-turn'
import { useCurrentScheduleTask } from './schedule-task-catalog'
import css from './ScheduleTurnCard.module.css'

/** Lives beside the folded tool group. Only this mounted lease owns the creation receipt. */
export function ScheduleTurnCard({ threadId, turnId, blocks, ended, onOpenTaskDetail = openTaskDetail }: {
  threadId: string | null; turnId: string | null; blocks: readonly ChatBlock[]; ended: boolean
  onOpenTaskDetail?: (taskId: string, title: string) => void
}) {
  const calls = scheduleCreationCalls(threadId, turnId, blocks, ended)
  if (!calls.length) return null
  return <div className={css.list} data-schedule-turn-cards>{calls.map(({ key, block }) =>
    <ProtectedToolResult key={key} block={block} threadId={threadId} preload hideUnavailable
      renderView={({ display }) => <AdmittedCreation display={display} onOpenTaskDetail={onOpenTaskDetail} />} />)}</div>
}

function AdmittedCreation({ display, onOpenTaskDetail }: {
  display: ToolResultLocalDisplayV1 | null; onOpenTaskDetail: (taskId: string, title: string) => void
}) {
  if (!display || nativeScheduleOperation(display.toolName) !== 'create' || display.capture.kind !== 'schedule' || display.capture.status !== 'completed') return null
  const receipt = display.capture.schedule
  return receipt.operation === 'create' && receipt.status === 'success'
    ? <CurrentCreation creation={receipt.task} onOpenTaskDetail={onOpenTaskDetail} /> : null
}

function CurrentCreation({ creation, onOpenTaskDetail }: {
  creation: ToolResultLocalScheduleTaskV1; onOpenTaskDetail: (taskId: string, title: string) => void
}) {
  const { t } = useTranslation('common'), { task } = useCurrentScheduleTask(creation.id)
  return <div data-protected-tool-summary data-protected-tool-readonly><ScheduleCreateCard
    creation={{ taskId: creation.id, title: creation.title, frequency: scheduleTaskSummary(creation, t) }}
    currentTask={task === undefined || task === null ? task : { title: task.title, frequency: scheduleTaskSummary(task, t) }}
    labels={{ open: t('open'), deleted: t('dshSchedule.deleted', { defaultValue: 'Deleted' }),
      openLabel: title => t('dshSchedule.openTask', { title, defaultValue: 'Open {{title}}' }) }}
    openTaskDetail={onOpenTaskDetail} /></div>
}
