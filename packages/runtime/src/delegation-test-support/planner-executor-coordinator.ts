import {
  DurableTaskJobManager,
  PLANNER_READ_ONLY_TOOLSET,
  type TaskJobRecord,
  type TaskJobRunner,
  type TaskJobTranscriptRef,
  normalizeParallelTaskPlan,
  validateParallelTaskPlan
} from './job-manager.js'
import type { SubagentToolPolicy } from '../contracts/capabilities.js'

export type PlannerExecutorTask = {
  id: string
  prompt: string
  label?: string
  depends_on?: string[]
}

export type PlannerExecutorCoordinatorResult = {
  status: 'completed' | 'failed' | 'cancelled'
  plannerJob: TaskJobRecord
  order: string[]
  jobs: TaskJobRecord[]
  skipped: Array<{ id: string; reason: string }>
  outputOffsets: Record<string, number>
  plannerReadOnlyToolset: string[]
}

export async function runPlannerExecutorCoordinator(input: {
  taskJobs: DurableTaskJobManager
  parentThreadId: string
  parentTurnId: string
  parentCallId?: string
  childRunId?: string
  permissionPolicy?: SubagentToolPolicy
  plan: PlannerExecutorTask[]
  plannerSummary?: string
  signal?: AbortSignal
  transcriptFor?: (task: PlannerExecutorTask) => TaskJobTranscriptRef | undefined
  runnerFor: (task: PlannerExecutorTask) => TaskJobRunner
}): Promise<PlannerExecutorCoordinatorResult> {
  const plannerJob = await input.taskJobs.startForeground({
    kind: 'planner',
    parentThreadId: input.parentThreadId,
    parentTurnId: input.parentTurnId,
    permissionPolicy: 'readOnly',
    ...(input.parentCallId ? { parentCallId: input.parentCallId } : {}),
    ...(input.childRunId ? { childRunId: input.childRunId } : {})
  }, async () => input.plannerSummary ?? `planner produced ${input.plan.length} task(s)`)
  const order = validateParallelTaskPlan(normalizeParallelTaskPlan(input.plan))
  const byId = new Map(input.plan.map((task) => [task.id, task]))
  const completed = new Set<string>()
  const failed = new Set<string>()
  const jobs: TaskJobRecord[] = []
  const skipped: PlannerExecutorCoordinatorResult['skipped'] = []

  while (completed.size + skipped.length < order.length) {
    if (input.signal?.aborted) {
      for (const id of order) {
        if (!completed.has(id) && !skipped.some((item) => item.id === id)) {
          skipped.push({ id, reason: 'planner executor cancelled before launch' })
        }
      }
      break
    }
    const ready = order
      .filter((id) => !completed.has(id))
      .filter((id) => !skipped.some((item) => item.id === id))
      .filter((id) => (byId.get(id)?.depends_on ?? [])
        .every((dependency) => completed.has(dependency) && !failed.has(dependency)))

    if (ready.length === 0) {
      for (const id of order) {
        if (completed.has(id) || skipped.some((item) => item.id === id)) continue
        const blockingFailures = (byId.get(id)?.depends_on ?? []).filter((dependency) => failed.has(dependency))
        if (blockingFailures.length > 0) {
          skipped.push({
            id,
            reason: `dependency failed: ${blockingFailures.join(', ')}`
          })
        }
      }
      if (completed.size + skipped.length >= order.length) break
      throw new Error('planner executor dependency scheduler stalled')
    }

    const wave = await Promise.all(ready.map(async (id) => {
      const task = byId.get(id)
      if (!task) throw new Error(`planner executor task not found: ${id}`)
      const job = await input.taskJobs.startBackground({
        kind: 'parallel_task',
        parentThreadId: input.parentThreadId,
        parentTurnId: input.parentTurnId,
        label: task.label ?? task.id,
        parallelIndex: order.indexOf(id) + 1,
        dependencies: task.depends_on ?? [],
        permissionPolicy: input.permissionPolicy ?? 'inherit',
        transcript: input.transcriptFor?.(task),
        ...(input.parentCallId ? { parentCallId: input.parentCallId } : {}),
        ...(input.childRunId ? { childRunId: input.childRunId } : {})
      }, input.runnerFor(task))
      return { taskId: id, job: await waitForCoordinatorJob(input.taskJobs, job.id, input.signal) }
    }))
    for (const { taskId, job } of wave) {
      jobs.push(job)
      completed.add(taskId)
      if (job.status !== 'completed') failed.add(taskId)
    }
  }

  const outputOffsets: Record<string, number> = {}
  for (const job of jobs) {
    outputOffsets[job.id] = (await input.taskJobs.output(job.id, { offset: 0 }))?.nextOffset ?? 0
  }

  return {
    status: input.signal?.aborted
      ? 'cancelled'
      : failed.size > 0 || skipped.length > 0
        ? 'failed'
        : 'completed',
    plannerJob,
    order,
    jobs,
    skipped,
    outputOffsets,
    plannerReadOnlyToolset: [...PLANNER_READ_ONLY_TOOLSET]
  }
}

async function waitForCoordinatorJob(
  taskJobs: DurableTaskJobManager,
  jobId: string,
  signal?: AbortSignal
): Promise<TaskJobRecord> {
  while (true) {
    if (signal?.aborted) {
      const killed = await taskJobs.kill(jobId, 'planner executor cancelled')
      if (killed) return killed
    }
    const [job] = await taskJobs.wait([jobId], { timeoutMs: 25 })
    if (!job) throw new Error(`planner executor job disappeared: ${jobId}`)
    if (
      job.status === 'completed' ||
      job.status === 'failed' ||
      job.status === 'interrupted' ||
      job.status === 'killed'
    ) return job
  }
}
