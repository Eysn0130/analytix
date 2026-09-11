import { z } from 'zod'

export const TaskJobPublicIdV1Schema = z.string().regex(/^[A-Za-z0-9_.:-]{1,128}$/)
export const ThreadSummaryTaskJobIdV1Schema = z.string().regex(
  /^(?:(?:taskjob|run):[A-Za-z0-9_.:-]{1,120}|job-[A-Za-z0-9_.:-]{1,124})$/
)
export const ThreadSummaryCommandTaskIdV1Schema = z.string().regex(
  /^command:[A-Za-z0-9_.:-]{1,120}$/
)

export const TaskJobStatusV1Schema = z.enum([
  'queued',
  'running',
  'pause_requested',
  'paused',
  'resume_requested',
  'resuming',
  'completed',
  'failed',
  'aborted',
  'interrupted',
  'killed',
  'canceled',
  'timeout',
  'unknown'
])
export type TaskJobStatusV1 = z.infer<typeof TaskJobStatusV1Schema>

export const TaskJobKindV1Schema = z.enum([
  'task',
  'parallel_task',
  'background-shell',
  'bash',
  'subagent',
  'child-run',
  'parallel-child-run',
  'unknown'
])
export type TaskJobKindV1 = z.infer<typeof TaskJobKindV1Schema>

const TERMINAL_TASK_JOB_STATUSES = new Set<TaskJobStatusV1>([
  'completed',
  'failed',
  'aborted',
  'interrupted',
  'killed',
  'canceled',
  'timeout'
])

const OutputWithheldFieldsV1 = {
  schemaVersion: z.literal(1),
  availability: z.literal('withheld'),
  status: TaskJobStatusV1Schema,
  outputWithheld: z.literal(true),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false)
} as const

export const TaskJobOutputWithheldResponseV1Schema = z.object({
  ...OutputWithheldFieldsV1,
  jobId: TaskJobPublicIdV1Schema,
  reasonCode: z.literal('security_bound_child_output'),
  outputTrustStatus: z.literal('untrusted_child_output')
}).strict()

export const TaskJobOutputResponseV1Schema = TaskJobOutputWithheldResponseV1Schema
export type TaskJobOutputResponseV1 = z.infer<typeof TaskJobOutputResponseV1Schema>

export const TaskJobSummaryV1Schema = z.object({
  schemaVersion: z.literal(1),
  id: TaskJobPublicIdV1Schema,
  kind: TaskJobKindV1Schema,
  status: TaskJobStatusV1Schema,
  background: z.boolean(),
  active: z.boolean().optional(),
  terminal: z.boolean(),
  outputWithheld: z.literal(true),
  outputTrustStatus: z.literal('untrusted_child_output'),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false)
}).strict().superRefine((value, context) => {
  const terminal = TERMINAL_TASK_JOB_STATUSES.has(value.status)
  const active = new Set<TaskJobStatusV1>([
    'queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming'
  ]).has(value.status)
  if (value.terminal !== terminal || (value.active !== undefined && value.active !== active)) {
    context.addIssue({
      code: 'custom',
      path: ['status'],
      message: 'task-job lifecycle flags mismatch'
    })
  }
})
export type TaskJobSummaryV1 = z.infer<typeof TaskJobSummaryV1Schema>

export const TaskJobListResponseV1Schema = z.object({
  jobs: z.array(TaskJobSummaryV1Schema),
  count: z.number().int().nonnegative()
}).strict()

export const TaskJobWaitResponseV1Schema = z.object({
  jobs: z.array(TaskJobSummaryV1Schema)
}).strict()

export const TaskJobKillResponseV1Schema = z.object({
  job: TaskJobSummaryV1Schema
}).strict()

export type TaskJobListResponseV1 = z.infer<typeof TaskJobListResponseV1Schema>
export type TaskJobWaitResponseV1 = z.infer<typeof TaskJobWaitResponseV1Schema>
export type TaskJobKillResponseV1 = z.infer<typeof TaskJobKillResponseV1Schema>

export function normalizeTaskJobStatusV1(value: unknown): TaskJobStatusV1 {
  const parsed = TaskJobStatusV1Schema.safeParse(value)
  return parsed.success ? parsed.data : 'unknown'
}

export function normalizeTaskJobKindV1(value: unknown): TaskJobKindV1 {
  const parsed = TaskJobKindV1Schema.safeParse(value)
  return parsed.success ? parsed.data : 'unknown'
}

export function taskJobSummaryV1(input: {
  id: string
  kind: unknown
  status: unknown
  background?: boolean
  active?: boolean
}): TaskJobSummaryV1 {
  const status = normalizeTaskJobStatusV1(input.status)
  return TaskJobSummaryV1Schema.parse({
    schemaVersion: 1,
    id: input.id,
    kind: normalizeTaskJobKindV1(input.kind),
    status,
    background: input.background === true,
    terminal: TERMINAL_TASK_JOB_STATUSES.has(status),
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false,
    ...(input.active !== undefined ? { active: input.active } : {})
  })
}

export const ThreadSummaryTaskOutputWithheldResponseV1Schema = z.object({
  ...OutputWithheldFieldsV1,
  taskId: ThreadSummaryTaskJobIdV1Schema,
  reasonCode: z.literal('security_bound_child_output'),
  outputTrustStatus: z.literal('untrusted_child_output')
}).strict().or(z.object({
  schemaVersion: z.literal(1),
  availability: z.literal('withheld'),
  status: z.enum([
    'running', 'pending', 'stopped', 'failed', 'aborted', 'killed', 'canceled',
    'timeout', 'error', 'completed', 'done', 'success', 'unknown'
  ]),
  outputWithheld: z.literal(true),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false),
  taskId: ThreadSummaryCommandTaskIdV1Schema,
  reasonCode: z.literal('tool_output_private'),
  outputTrustStatus: z.literal('private_tool_output')
}).strict())

export const ThreadSummaryTaskOutputResponseV1Schema = ThreadSummaryTaskOutputWithheldResponseV1Schema
export type ThreadSummaryTaskOutputResponseV1 = z.infer<typeof ThreadSummaryTaskOutputResponseV1Schema>

export function canonicalThreadSummaryTaskIdV1(value: unknown): string {
  if (typeof value !== 'string') return ''
  const taskId = value.trim()
  if (taskId.startsWith('run:')) return `taskjob:${taskId.slice('run:'.length)}`
  if (taskId.startsWith('job-')) return `taskjob:${taskId}`
  return taskId
}

export function threadSummaryTaskIdentityMatchesV1(left: unknown, right: unknown): boolean {
  const canonicalLeft = canonicalThreadSummaryTaskIdV1(left)
  const canonicalRight = canonicalThreadSummaryTaskIdV1(right)
  return Boolean(canonicalLeft && canonicalLeft === canonicalRight)
}
