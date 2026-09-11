import type { ChildRunRecord, DelegationRuntime } from '../../delegation-test-support/delegation-runtime.js'
import {
  TASK_JOB_ROUTE_CONTRACT,
  TASK_JOB_CONTROL_TOOL_CONTRACT,
  TASK_TOOL_CONTRACT,
  PARALLEL_TASKS_TOOL_CONTRACT,
  type DurableTaskJobManager,
  taskJobControlMetadata,
  type TaskJobControlMetadata,
  type TaskJobRecord,
  type TaskJobTranscriptRef,
  type TranscriptIdentity,
  PLANNER_READ_ONLY_TOOLSET,
  normalizeParallelTaskPlan,
  resolveTranscriptOperation,
  stableTranscriptHash,
  validateParallelTaskPlan
} from '../../delegation-test-support/job-manager.js'
import {
  TaskJobOutputResponseV1Schema,
  taskJobSummaryV1
} from '../../contracts/task-job-output.js'
import type { ToolHostContext } from '../../ports/tool-host.js'
import type { CapabilityToolProvider } from './capability-registry.js'
import { resolveChildMaxModelSteps } from './delegation-tool-provider.js'
import { LocalToolHost } from './local-tool-host.js'

export function buildTaskJobToolProviders(input: {
  taskJobs: DurableTaskJobManager | undefined
  delegationRuntime: DelegationRuntime | undefined
}): CapabilityToolProvider[] {
  const taskJobs = input.taskJobs
  const delegationRuntime = input.delegationRuntime
  if (!taskJobs || !delegationRuntime) return []
  const profiles = delegationRuntime.listProfiles()
  const profileNames = profiles.map((profile) => profile.name)
  return [{
    id: 'task-jobs',
    kind: 'delegation',
    enabled: true,
    available: true,
    tools: [
      LocalToolHost.defineTool({
        name: TASK_TOOL_CONTRACT.name,
        description: 'Run an internal child task as a durable job. Use run_in_background=true for long-running work, then poll task-job routes for output/wait/kill.',
        inputSchema: {
          type: 'object',
          properties: {
            label: { type: 'string', description: 'Short label for the task job.' },
            prompt: { type: 'string', description: 'The task for the child agent.' },
            run_in_background: { type: 'boolean', description: 'Start the task and return the job id immediately.' },
            workspace: { type: 'string' },
            model: { type: 'string', description: 'Override the child model. Defaults to the profile model or server default.' },
            effort: { type: 'string', enum: ['off', 'low', 'medium', 'high', 'max'], description: 'Override the child reasoning effort. Defaults to the profile effort.' },
            tools: { type: 'array', items: { type: 'string' }, description: 'Optional explicit child tool scope. Read-only profiles still clamp to safe read tools.' },
            max_steps: { type: 'integer', minimum: 0, description: 'Optional child model/tool loop budget. 0 means unlimited.' },
            profile: profileNames.length
              ? { type: 'string', enum: profileNames, description: 'Subagent role to apply.' }
              : { type: 'string', description: 'Subagent role to apply.' },
            continue_from: { type: 'string', description: 'Resume a prior subagent transcript when the active runtime backend supports durable transcript continuation.' },
            fork_from: { type: 'string', description: 'Fork a prior subagent transcript when the active runtime backend supports durable transcript forking.' }
          },
          required: [TASK_TOOL_CONTRACT.promptField],
          additionalProperties: false
        },
        policy: 'on-request',
        toolKind: 'subagent',
        execute: async (args, context) => {
          const prompt = stringArg(args.prompt)
          if (!prompt) return { output: { error: 'prompt is required' }, isError: true }
          const continueFrom = stringArg(args.continue_from)
          const forkFrom = stringArg(args.fork_from)
          if (continueFrom && forkFrom) {
            return {
              output: {
                code: 'subagent_transcript_invalid_request',
                error: 'continue_from and fork_from are mutually exclusive'
              },
              isError: true
            }
          }
          const label = stringArg(args.label)
          const maxModelSteps = resolveChildMaxModelSteps(context, args.max_steps)
          const childInput = {
            prompt,
            ...(label ? { label } : {}),
            workspace: stringArg(args.workspace) || context.workspace,
            ...(stringArg(args.model) ? { model: stringArg(args.model) } : {}),
            ...(stringArg(args.effort) ? { effort: stringArg(args.effort) } : {}),
            ...(stringListArg(args.tools).length > 0 ? { tools: stringListArg(args.tools) } : {}),
            ...(maxModelSteps !== undefined ? { maxModelSteps } : {}),
            ...(stringArg(args.profile) ? { profile: stringArg(args.profile) } : {}),
            context
          }
          let transcript: TaskJobTranscriptRef | undefined
          if (continueFrom || forkFrom) {
            const prepared = await prepareTranscriptTask({
              runtime: delegationRuntime,
              profiles,
              context,
              mode: continueFrom ? 'continue' : 'fork',
              sourceId: continueFrom || forkFrom,
              childInput
            })
            transcript = prepared.transcript
            childInput.prompt = prepared.prompt
          }
          const jobInput = {
            kind: 'task' as const,
            parentThreadId: context.threadId,
            parentTurnId: context.turnId,
            permissionPolicy: delegationRuntime.defaultToolPolicy,
            ...(label ? { label } : {}),
            ...(context.toolCallId ? { parentCallId: context.toolCallId } : {}),
            ...(transcript ? { transcript } : {})
          }
          const runner = childTaskRunner(delegationRuntime, {
            ...childInput,
            ...(transcript ? { transcript } : {})
          })
          if (args.run_in_background === true) {
            const job = await taskJobs.startBackground(jobInput, runner, {
              parentSignal: context.abortSignal
            })
            return { output: backgroundJobOutput(job) }
          }
          const job = await taskJobs.startForeground(jobInput, runner, {
            parentSignal: context.abortSignal
          })
          return { output: taskJobToolSummary(taskJobControlMetadata(job), false), isError: job.status !== 'completed' }
        }
      }),
      LocalToolHost.defineTool({
        name: PARALLEL_TASKS_TOOL_CONTRACT.name,
        description: 'Run multiple internal child tasks with explicit depends_on ordering. Independent ready tasks are launched in the same dependency wave.',
        inputSchema: {
          type: 'object',
          properties: {
            tasks: {
              type: 'array',
              items: {
                type: 'object',
                properties: {
                  id: { type: 'string' },
                  label: { type: 'string' },
                  prompt: { type: 'string' },
                  depends_on: { type: 'array', items: { type: 'string' } },
                  workspace: { type: 'string' },
                  model: { type: 'string' },
                  effort: { type: 'string', enum: ['off', 'low', 'medium', 'high', 'max'] },
                  tools: { type: 'array', items: { type: 'string' } },
                  max_steps: { type: 'integer', minimum: 0 },
                  profile: profileNames.length
                    ? { type: 'string', enum: profileNames }
                    : { type: 'string' }
                },
                required: ['prompt'],
                additionalProperties: false
              }
            }
          },
          required: [PARALLEL_TASKS_TOOL_CONTRACT.tasksField],
          additionalProperties: false
        },
        policy: 'on-request',
        toolKind: 'subagent',
        execute: async (args, context) => {
          const tasks = parseParallelTasks(args.tasks)
          const order = validateParallelTaskPlan(normalizeParallelTaskPlan(tasks))
          const byId = new Map(tasks.map((task) => [task.id, task]))
          const completed = new Set<string>()
          const settled = new Set<string>()
          const failed = new Set<string>()
          const dependencyResults = new Map<string, string>()
          const skipped: Array<{ id: string; reason: string }> = []
          const jobs: TaskJobRecord[] = []
          const skipRemainingCancelled = (): void => {
            for (const id of order) {
              if (settled.has(id) || skipped.some((item) => item.id === id)) continue
              skipped.push({ id, reason: 'cancelled: parent turn aborted before task execution' })
            }
          }

          while (settled.size + skipped.length < order.length) {
            if (context.abortSignal.aborted) {
              skipRemainingCancelled()
              break
            }
            const ready = order
              .filter((id) => !settled.has(id))
              .filter((id) => !skipped.some((item) => item.id === id))
              .filter((id) => (byId.get(id)?.depends_on ?? [])
                .every((dependency) => completed.has(dependency) && !failed.has(dependency)))
            if (ready.length === 0) {
              for (const id of order) {
                if (settled.has(id) || skipped.some((item) => item.id === id)) continue
                const blockingFailures = (byId.get(id)?.depends_on ?? []).filter((dependency) => failed.has(dependency))
                if (blockingFailures.length > 0) {
                  skipped.push({
                    id,
                    reason: `dependency failed: ${blockingFailures.join(', ')}`
                  })
                }
              }
              if (settled.size + skipped.length >= order.length) break
              throw new Error('parallel task dependency scheduler stalled')
            }
            const wave = await Promise.all(ready.map(async (id) => {
              const task = byId.get(id)
              if (!task) throw new Error(`parallel task not found: ${id}`)
              const maxModelSteps = task.max_steps !== undefined
                ? task.max_steps
                : resolveChildMaxModelSteps(context, undefined)
              const runner = childTaskRunner(delegationRuntime, {
                prompt: promptWithParallelDependencyResults(task, dependencyResults),
                label: task.label ?? task.id,
                workspace: task.workspace || context.workspace,
                ...(task.model ? { model: task.model } : {}),
                ...(task.effort ? { effort: task.effort } : {}),
                ...(task.tools?.length ? { tools: task.tools } : {}),
                ...(maxModelSteps !== undefined ? { maxModelSteps } : {}),
                ...(task.profile ? { profile: task.profile } : {}),
                context
              })
              const job = await taskJobs.startForeground({
                kind: 'parallel_task',
                parentThreadId: context.threadId,
                parentTurnId: context.turnId,
                label: task.label ?? task.id,
                parallelIndex: order.indexOf(id) + 1,
                dependencies: task.depends_on ?? [],
                permissionPolicy: delegationRuntime.defaultToolPolicy,
                ...(context.toolCallId ? { parentCallId: context.toolCallId } : {})
              }, runner, { parentSignal: context.abortSignal })
              return { id, job }
            }))
            for (const { id, job } of wave) {
              jobs.push(job)
              settled.add(id)
              if (job.status === 'completed') completed.add(id)
              else failed.add(id)
              dependencyResults.set(id, parallelTaskJobSummary(id, job))
            }
            if (context.abortSignal.aborted) {
              skipRemainingCancelled()
              break
            }
          }

          return {
            output: {
              order,
              jobs: jobs.map((job) => taskJobToolSummary(taskJobControlMetadata(job), false)),
              skipped
            },
            isError: jobs.some((job) => job.status !== 'completed') || skipped.length > 0
          }
        }
      }),
      LocalToolHost.defineTool({
        name: TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
        description: 'Wait for one or more background sub-agent jobs owned by the current parent thread.',
        inputSchema: {
          type: 'object',
          properties: {
            job_id: { type: 'string' },
            jobId: { type: 'string' },
            job_ids: { type: 'array', items: { type: 'string' } },
            jobIds: { type: 'array', items: { type: 'string' } },
            timeout_ms: { type: 'integer', minimum: 0, maximum: 60000 },
            timeoutMs: { type: 'integer', minimum: 0, maximum: 60000 }
          },
          additionalProperties: false
        },
        policy: 'auto',
        toolKind: 'subagent',
        execute: async (args, context) => {
          const jobIds = jobIdsArg(args)
          const timeoutProvided = Object.hasOwn(args, 'timeout_ms') || Object.hasOwn(args, 'timeoutMs')
          const result = await taskJobs.waitMetadataForParent(jobIds, {
            timeoutMs: timeoutProvided
              ? boundedMsArg(args.timeout_ms ?? args.timeoutMs, 60_000)
              : 60_000,
            parentThreadId: context.threadId
          })
          if (!result.ok) return taskJobToolAccessError(result.reason, jobIds.join(', '))
          return {
            output: { jobs: result.value.map((job) => taskJobToolSummary(job, true)) },
            isError: result.value.some((job) => job.status === 'failed' || job.status === 'killed')
          }
        }
      }),
      LocalToolHost.defineTool({
        name: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
        description: 'Read new output from a background sub-agent job owned by the current parent thread.',
        inputSchema: {
          type: 'object',
          properties: {
            job_id: { type: 'string' },
            jobId: { type: 'string' },
            offset: { type: 'integer', minimum: 0 },
            limit: { type: 'integer', minimum: 0 },
            filter: { type: 'string', description: 'Optional regular expression; only matching output lines are returned.' }
          },
          additionalProperties: false
        },
        policy: 'auto',
        toolKind: 'subagent',
        execute: async (args, context) => {
          const jobId = jobIdArg(args)
          if (!jobId) return { output: { code: 'validation_error', error: 'bash_output requires job_id' }, isError: true }
          const result = await taskJobs.metadataForParent(jobId, {
            parentThreadId: context.threadId
          })
          if (!result.ok) return taskJobToolAccessError(result.reason, jobId)
          return {
            output: TaskJobOutputResponseV1Schema.parse({
              schemaVersion: 1,
              availability: 'withheld',
              jobId: result.value.id,
              status: result.value.status,
              reasonCode: 'security_bound_child_output',
              outputWithheld: true,
              outputTrustStatus: 'untrusted_child_output',
              factAnswerAllowed: false,
              evidenceAuthority: false,
              canReadOutput: false,
              canContinueParent: false
            }),
            isError: result.value.status === 'failed' || result.value.status === 'killed'
          }
        }
      }),
      LocalToolHost.defineTool({
        name: TASK_JOB_CONTROL_TOOL_CONTRACT.kill,
        description: 'Cancel a background sub-agent job owned by the current parent thread.',
        inputSchema: {
          type: 'object',
          properties: {
            job_id: { type: 'string' },
            jobId: { type: 'string' },
            reason: { type: 'string' }
          },
          additionalProperties: false
        },
        policy: 'auto',
        execute: async (args, context) => {
          const jobId = jobIdArg(args)
          if (!jobId) return { output: { code: 'validation_error', error: 'kill_shell requires job_id' }, isError: true }
          const result = await taskJobs.killMetadataForParent(jobId, {
            reason: stringArg(args.reason) || 'killed by parent tool',
            parentThreadId: context.threadId
          })
          if (!result.ok) return taskJobToolAccessError(result.reason, jobId)
          return {
            output: { job: taskJobToolSummary(result.value, true) },
            isError: result.value.status === 'failed'
          }
        }
      })
    ]
  }]
}

function childTaskRunner(
  runtime: DelegationRuntime,
  input: {
    prompt: string
    label?: string
    workspace?: string
    model?: string
    effort?: string
    tools?: string[]
    maxModelSteps?: number
    profile?: string
    transcript?: TaskJobTranscriptRef
    context: ToolHostContext
  }
) {
  return async ({ signal, appendOutput, recordChildRun }: {
    signal: AbortSignal
    appendOutput: (text: string) => Promise<void>
    recordChildRun: (childRunId: string) => Promise<void>
  }): Promise<string> => {
    const transcript = runnableTranscriptRef(input.transcript)
    const child = await runtime.runChild({
      parentThreadId: input.context.threadId,
      parentTurnId: input.context.turnId,
      ...(input.context.toolCallId ? { parentToolCallId: input.context.toolCallId } : {}),
      prompt: input.prompt,
      ...(input.label ? { label: input.label } : {}),
      ...(input.workspace ? { workspace: input.workspace } : {}),
      ...(input.model ? { model: input.model } : {}),
      ...(input.context.modelExecution ? { modelExecution: input.context.modelExecution } : {}),
      ...(input.effort ? { effort: input.effort } : {}),
      ...(input.tools?.length ? { tools: input.tools } : {}),
      approvalPolicy: input.context.approvalPolicy,
      ...(input.context.sandboxMode ? { sandboxMode: input.context.sandboxMode } : {}),
      ...(input.maxModelSteps !== undefined ? { maxModelSteps: input.maxModelSteps } : {}),
      ...(input.profile ? { profile: input.profile } : {}),
      ...(transcript ? { transcript } : {}),
      onChildRunStarted: (record) => recordChildRun(record.id),
      signal
    })
    const summary = child.summary?.trim() || child.error?.trim() || child.status
    await appendOutput(`${summary}\n`)
    if (child.status !== 'completed') {
      throw new Error(child.error || `child task ${child.id} ${child.status}`)
    }
    return summary
  }
}

type RunnableTranscriptRef = {
  mode: 'continue' | 'fork'
  sourceId?: string
  targetId?: string
  identityHash: string
}

function runnableTranscriptRef(transcript: TaskJobTranscriptRef | undefined): RunnableTranscriptRef | undefined {
  if (transcript?.mode !== 'continue' && transcript?.mode !== 'fork') return undefined
  return {
    mode: transcript.mode,
    ...(transcript.sourceId ? { sourceId: transcript.sourceId } : {}),
    ...(transcript.targetId ? { targetId: transcript.targetId } : {}),
    identityHash: transcript.identityHash
  }
}

function taskJobToolSummary(job: TaskJobControlMetadata, background: boolean): Record<string, unknown> {
  const { id, ...summary } = taskJobSummaryV1({
    id: job.id,
    kind: job.kind,
    status: job.status,
    background
  })
  return {
    ...summary,
    jobId: id
  }
}

function backgroundJobOutput(job: TaskJobRecord): Record<string, unknown> {
  return {
    ...taskJobToolSummary(taskJobControlMetadata(job), true),
    routes: TASK_JOB_ROUTE_CONTRACT
  }
}

async function prepareTranscriptTask(input: {
  runtime: DelegationRuntime
  profiles: {
    name: string
    toolPolicy: string
    providerId?: string
    model?: string
    variant?: string
    endpointFormat?: string
    effort?: string
    promptPreambleHash?: string
    tools?: string[]
  }[]
  context: ToolHostContext
  mode: 'continue' | 'fork'
  sourceId: string
  childInput: {
    prompt: string
    workspace?: string
    model?: string
    effort?: string
    profile?: string
    tools?: string[]
  }
}): Promise<{ prompt: string; transcript: TaskJobTranscriptRef }> {
  const source = await input.runtime.loadChildRun(input.sourceId, input.context.threadId)
  if (!source) {
    throw new Error(`subagent transcript not found: ${input.sourceId}`)
  }
  if (source.status !== 'completed') {
    throw new Error(`subagent transcript is not completed: ${input.sourceId}`)
  }
  const requested = requestedTranscriptIdentity(input)
  const transcript = resolveTranscriptOperation({
    mode: input.mode,
    sourceId: source.id,
    source: childRunTranscriptIdentity(source, input.runtime.defaultToolPolicy),
    requested,
    newId: `${source.id}_fork_${input.context.turnId}`
  })
  return {
    transcript,
    prompt: input.childInput.prompt
  }
}

function requestedTranscriptIdentity(input: {
  runtime: DelegationRuntime
  profiles: {
    name: string
    toolPolicy: string
    providerId?: string
    model?: string
    variant?: string
    endpointFormat?: string
    effort?: string
    promptPreambleHash?: string
    tools?: string[]
  }[]
  context: ToolHostContext
  childInput: {
    workspace?: string
    model?: string
    effort?: string
    profile?: string
    tools?: string[]
  }
}): TranscriptIdentity {
  const profile = input.childInput.profile
    ? input.profiles.find((candidate) => candidate.name === input.childInput.profile)
    : undefined
  const toolPolicy = profile?.toolPolicy ?? input.runtime.defaultToolPolicy
  const toolNames = input.childInput.tools?.length
    ? input.childInput.tools
    : profile?.tools
  const resolvedToolNames = transcriptToolNames(toolPolicy, toolNames)
  const requestedModel = input.childInput.model || profile?.model || input.context.modelExecution?.modelId
  const requestedProviderId = profile?.providerId || input.context.modelExecution?.providerId
  const requestedEndpointFormat = profile?.endpointFormat || input.context.modelExecution?.endpointFormat
  const requestedVariant = profile?.variant || input.context.modelExecution?.variant
  const modelSource = input.childInput.model
    ? 'explicit-input'
    : profile?.model || profile?.providerId || profile?.endpointFormat || profile?.variant
      ? 'subagent-profile'
      : input.context.modelExecution?.source
  return {
    ...(requestedModel ? { model: requestedModel, modelId: requestedModel } : {}),
    ...(requestedProviderId ? { providerId: requestedProviderId } : {}),
    ...(requestedEndpointFormat ? { endpointFormat: requestedEndpointFormat } : {}),
    ...(requestedVariant ? { variant: requestedVariant } : {}),
    ...(modelSource ? { modelSource } : {}),
    ...(input.childInput.effort || profile?.effort ? { effort: input.childInput.effort || profile?.effort } : {}),
    ...(input.childInput.profile ? { profile: input.childInput.profile } : {}),
    ...(input.childInput.workspace || input.context.workspace ? { workspace: input.childInput.workspace || input.context.workspace } : {}),
    toolPolicy,
    toolNames: resolvedToolNames,
    ...(profile?.promptPreambleHash ? { promptPreambleHash: profile.promptPreambleHash } : {}),
    toolSchemaHash: stableTranscriptHash(resolvedToolNames),
    ...(input.context.sandboxMode ? { sandboxMode: input.context.sandboxMode } : {}),
    approvalPolicy: input.context.approvalPolicy,
    ...(input.context.modelExecution?.capabilityFingerprint
      ? { capabilityFingerprint: input.context.modelExecution.capabilityFingerprint }
      : {})
  }
}

function childRunTranscriptIdentity(
  source: ChildRunRecord,
  defaultToolPolicy: string
): TranscriptIdentity {
  return {
    ...(source.model ? { model: source.model, modelId: source.model } : {}),
    ...(source.providerId ? { providerId: source.providerId } : {}),
    ...(source.endpointFormat ? { endpointFormat: source.endpointFormat } : {}),
    ...(source.variant ? { variant: source.variant } : {}),
    ...(source.modelSource ? { modelSource: source.modelSource } : {}),
    ...(source.effort ? { effort: source.effort } : {}),
    ...(source.profile ? { profile: source.profile } : {}),
    ...(source.workspace ? { workspace: source.workspace } : {}),
    toolPolicy: source.toolPolicy ?? defaultToolPolicy,
    toolNames: transcriptToolNames(source.toolPolicy ?? defaultToolPolicy, source.toolScope),
    ...(source.promptPreambleHash ? { promptPreambleHash: source.promptPreambleHash } : {}),
    toolSchemaHash: stableTranscriptHash(transcriptToolNames(source.toolPolicy ?? defaultToolPolicy, source.toolScope)),
    ...(source.sandboxMode ? { sandboxMode: source.sandboxMode } : {}),
    ...(source.approvalPolicy ? { approvalPolicy: source.approvalPolicy } : {}),
    ...(source.modelExecution?.capabilityFingerprint
      ? { capabilityFingerprint: source.modelExecution.capabilityFingerprint }
      : {})
  }
}

function transcriptToolNames(toolPolicy: string, toolScope?: readonly string[]): string[] {
  const scoped = normalizeToolScope(toolScope)
  if (scoped.length > 0) {
    if (toolPolicy !== 'readOnly') return scoped
    const readOnly: ReadonlySet<string> = new Set(PLANNER_READ_ONLY_TOOLSET)
    return scoped.filter((name) => readOnly.has(name))
  }
  return toolPolicy === 'readOnly' ? [...PLANNER_READ_ONLY_TOOLSET] : ['inherit']
}

function truncateTranscriptText(text: string, maxBytes: number): string {
  if (Buffer.byteLength(text, 'utf8') <= maxBytes) return text
  let bytes = 0
  let out = ''
  for (const char of text) {
    const size = Buffer.byteLength(char, 'utf8')
    if (bytes + size > maxBytes) break
    out += char
    bytes += size
  }
  return `${out}\n[truncated]`
}

function stringArg(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function stringListArg(value: unknown): string[] {
  return normalizeToolScope(Array.isArray(value) ? value.map(stringArg) : [])
}

function normalizeToolScope(value: readonly string[] | undefined): string[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of value) {
    const name = item.trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push(name)
  }
  return out.sort()
}

type ParallelTaskInput = {
  id: string
  prompt: string
  label?: string
  depends_on?: string[]
  workspace?: string
  model?: string
  effort?: string
  tools?: string[]
  max_steps?: number
  profile?: string
}

function parseParallelTasks(value: unknown): ParallelTaskInput[] {
  if (!Array.isArray(value)) throw new Error('parallel_tasks requires a tasks array')
  return value.map((candidate, index) => {
    if (!candidate || typeof candidate !== 'object') throw new Error('parallel task entries must be objects')
    const item = candidate as Record<string, unknown>
    const id = stringArg(item.id) || `task_${index + 1}`
    const prompt = stringArg(item.prompt)
    if (!prompt) throw new Error(`parallel task prompt is required: ${id}`)
    const dependsOn = Array.isArray(item.depends_on)
      ? item.depends_on.map(stringArg).filter(Boolean)
      : undefined
    const parsed = {
      id,
      prompt,
      ...(stringArg(item.label) ? { label: stringArg(item.label) } : {}),
      ...(dependsOn ? { depends_on: dependsOn } : {}),
      ...(stringArg(item.workspace) ? { workspace: stringArg(item.workspace) } : {}),
      ...(stringArg(item.model) ? { model: stringArg(item.model) } : {}),
      ...(stringArg(item.effort) ? { effort: stringArg(item.effort) } : {}),
      ...(stringListArg(item.tools).length > 0 ? { tools: stringListArg(item.tools) } : {}),
      ...(nonNegativeIntArg(item.max_steps) !== undefined ? { max_steps: nonNegativeIntArg(item.max_steps) } : {}),
      ...(stringArg(item.profile) ? { profile: stringArg(item.profile) } : {})
    }
    return parsed
  })
}

function promptWithParallelDependencyResults(
  task: ParallelTaskInput,
  dependencyResults: ReadonlyMap<string, string>
): string {
  const dependencies = task.depends_on ?? []
  if (dependencies.length === 0) return task.prompt
  const results = dependencies
    .map((id) => dependencyResults.get(id))
    .filter((result): result is string => Boolean(result?.trim()))
  if (results.length === 0) return task.prompt
  return [
    'Previous parallel task results:',
    results.join('\n\n'),
    'Current task:',
    task.prompt
  ].join('\n\n')
}

function parallelTaskJobSummary(taskId: string, job: TaskJobRecord): string {
  return `Task ${taskId} status=${job.status}; child output is withheld by the host security boundary.`
}

function jobIdArg(args: Record<string, unknown>): string {
  return stringArg(args.job_id) || stringArg(args.jobId)
}

function jobIdsArg(args: Record<string, unknown>): string[] {
  const direct = jobIdArg(args)
  const list = Array.isArray(args.job_ids)
    ? args.job_ids
    : Array.isArray(args.jobIds)
      ? args.jobIds
      : []
  return [...new Set([
    direct,
    ...list.map(stringArg)
  ].filter(Boolean))]
}

function boundedMsArg(value: unknown, max: number): number | undefined {
  const parsed = nonNegativeIntArg(value)
  return parsed === undefined ? undefined : Math.min(parsed, max)
}

function nonNegativeIntArg(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return undefined
  return Math.floor(value)
}

function taskJobToolAccessError(reason: 'not_found' | 'forbidden', id: string): { output: Record<string, unknown>; isError: true } {
  if (reason === 'forbidden') {
    return { output: { code: 'forbidden', error: `task job does not belong to this thread: ${id}` }, isError: true }
  }
  return { output: { code: 'not_found', error: `task job not found: ${id}` }, isError: true }
}
