import type { ReactElement } from 'react'
import { CircleDot, Loader2, Terminal } from 'lucide-react'
import type { CoreThreadSummaryTaskJson } from '../../agent/analytix-contract'
import { SummaryRow } from './SummaryRow'

export function TaskSummaryRows({
  tasks,
  busyTaskId,
  onOpenDiagnostics
}: {
  tasks: CoreThreadSummaryTaskJson[]
  busyTaskId: string | null
  onOpenDiagnostics?: (task: CoreThreadSummaryTaskJson) => void
}): ReactElement {
  if (tasks.length === 0) return <div className="px-2 py-3 text-[12px] text-ds-faint">暂无后台任务</div>
  return (
    <>
      {tasks.map((task) => {
        const active = task.active === true || task.status === 'running' || task.status === 'pending' || task.status === 'queued'
        const busy = busyTaskId === task.id
        const label = task.kind === 'command' ? 'Command' : task.kind
        const detail = taskDetail(task)
        return (
          <SummaryRow
            key={task.id}
            icon={busy || active ? <Loader2 className="h-4 w-4 animate-spin" /> : <Terminal className="h-4 w-4" />}
            label={label}
            detail={detail}
            title={detail ? `${label}\n${detail}` : label}
            trailing={statusLabel(task)}
            actions={
              onOpenDiagnostics ? (
                <IconButton label="打开运行时诊断" onClick={() => onOpenDiagnostics(task)}>
                  <CircleDot className="h-3.5 w-3.5" />
                </IconButton>
              ) : null
            }
          />
        )
      })}
    </>
  )
}

function taskDetail(task: CoreThreadSummaryTaskJson): string {
  return `${task.id} · 输出已隔离`
}

function IconButton({
  label,
  onClick,
  children
}: {
  label: string
  onClick: () => void
  children: ReactElement
}): ReactElement {
  return (
    <button
      type="button"
      className="rounded p-1 text-ds-muted hover:bg-ds-hover hover:text-ds-ink disabled:opacity-50"
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
    >
      {children}
    </button>
  )
}

function statusLabel(task: CoreThreadSummaryTaskJson): string {
  if (task.active) return '运行中'
  if (task.status === 'completed' || task.status === 'done' || task.status === 'success') return '完成'
  if (task.status === 'unknown' || task.status === 'stopped') return '已停止'
  return task.status
}
