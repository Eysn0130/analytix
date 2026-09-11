import type { AgentProvider, ChatBlock, NormalizedThread } from '../agent/types'

export type CreateLoopStepKind = 'plan' | 'execute' | 'review'
export type CreateLoopStepStatus = 'pending' | 'running' | 'waiting' | 'success' | 'error'
export type CreateLoopRunStatus = 'running' | 'waiting' | 'success' | 'error'

export type CreateLoopStepDefinition = {
  id: CreateLoopStepKind
  title: string
  description: string
}

export type CreateLoopDefinition = {
  id: 'analytix-create-loop'
  title: string
  steps: CreateLoopStepDefinition[]
}

export type CreateLoopStepRun = {
  id: CreateLoopStepKind
  title: string
  status: CreateLoopStepStatus
  startedAt?: string
  finishedAt?: string
  turnId?: string
  userMessageItemId?: string
  message?: string
  output?: string
  error?: string
  waitingFor?: 'approval' | 'user-input'
  waitingItemId?: string
}

export type CreateLoopRun = {
  id: string
  workflowId: CreateLoopDefinition['id']
  title: string
  objective: string
  status: CreateLoopRunStatus
  threadId: string
  model?: string
  providerId?: string
  workspace?: string
  createdAt: string
  updatedAt: string
  steps: CreateLoopStepRun[]
  lastMessage?: string
  lastError?: string
}

export type CreateLoopRuntimeProvider = Pick<
  AgentProvider,
  'createThread' | 'sendUserMessage' | 'getThreadDetail'
>

export type CreateLoopRunOptions = {
  provider: CreateLoopRuntimeProvider
  objective: string
  workspace?: string
  model?: string
  providerId?: string
  existingRun?: CreateLoopRun
  pollIntervalMs?: number
  timeoutMs?: number
  now?: () => Date
  onUpdate?: (run: CreateLoopRun) => void
}

type ThreadDetail = Awaited<ReturnType<AgentProvider['getThreadDetail']>>

type WaitResult =
  | { status: 'success'; output: string; message: string }
  | { status: 'waiting'; waitingFor: 'approval' | 'user-input'; waitingItemId: string; message: string; output: string }
  | { status: 'error'; message: string; output: string }

const DEFAULT_POLL_INTERVAL_MS = 1_000
const DEFAULT_TIMEOUT_MS = 30 * 60_000
const ACTIVE_THREAD_STATUSES = new Set([
  'queued',
  'pending',
  'running',
  'streaming',
  'in_progress',
  'waiting',
  'awaiting_approval',
  'awaiting_user_input'
])
const ERROR_THREAD_STATUSES = new Set(['failed', 'error', 'aborted', 'cancelled', 'canceled'])
const COMPLETE_THREAD_STATUSES = new Set(['completed', 'complete', 'success', 'succeeded'])

export const ANALYTIX_CREATE_LOOP_DEFINITION: CreateLoopDefinition = {
  id: 'analytix-create-loop',
  title: 'Create Loop',
  steps: [
    {
      id: 'plan',
      title: 'Plan',
      description: 'Convert the request into an actionable plan.'
    },
    {
      id: 'execute',
      title: 'Execute',
      description: 'Run the plan through the analytix runtime.'
    },
    {
      id: 'review',
      title: 'Review',
      description: 'Summarize outputs, generated files, risks, and next actions.'
    }
  ]
}

function runId(now: Date): string {
  const suffix = globalThis.crypto?.randomUUID?.() ?? `${now.getTime()}-${Math.random().toString(16).slice(2)}`
  return `workflow-${suffix}`
}

function cloneRun(run: CreateLoopRun): CreateLoopRun {
  return {
    ...run,
    steps: run.steps.map((step) => ({ ...step }))
  }
}

function formatError(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function initialStepRuns(): CreateLoopStepRun[] {
  return ANALYTIX_CREATE_LOOP_DEFINITION.steps.map((step) => ({
    id: step.id,
    title: step.title,
    status: 'pending'
  }))
}

function createRun(options: CreateLoopRunOptions, thread: NormalizedThread, now: Date): CreateLoopRun {
  return {
    id: runId(now),
    workflowId: ANALYTIX_CREATE_LOOP_DEFINITION.id,
    title: ANALYTIX_CREATE_LOOP_DEFINITION.title,
    objective: options.objective.trim(),
    status: 'running',
    threadId: thread.id,
    model: options.model?.trim() || thread.model,
    providerId: options.providerId?.trim() || thread.providerId,
    workspace: options.workspace?.trim() || thread.workspace,
    createdAt: now.toISOString(),
    updatedAt: now.toISOString(),
    steps: initialStepRuns(),
    lastMessage: 'Started'
  }
}

function ensureStepRuns(run: CreateLoopRun): CreateLoopStepRun[] {
  return ANALYTIX_CREATE_LOOP_DEFINITION.steps.map((definition) => {
    const existing = run.steps.find((step) => step.id === definition.id)
    return existing ? { ...existing, title: definition.title } : { id: definition.id, title: definition.title, status: 'pending' }
  })
}

function previousOutputs(run: CreateLoopRun, beforeStepId: CreateLoopStepKind): string {
  const parts: string[] = []
  for (const step of run.steps) {
    if (step.id === beforeStepId) break
    if (step.status !== 'success' || !step.output?.trim()) continue
    parts.push(`## ${step.title}\n${step.output.trim()}`)
  }
  return parts.join('\n\n')
}

export function buildCreateLoopStepPrompt(run: CreateLoopRun, stepId: CreateLoopStepKind): string {
  const context = previousOutputs(run, stepId)
  if (stepId === 'plan') {
    return [
      'You are running an Analytix Create Loop workflow.',
      'Create a concise, executable plan for the user objective.',
      '',
      `Objective:\n${run.objective}`,
      '',
      'Return: numbered plan, assumptions, and explicit approval or input needed before execution.'
    ].join('\n')
  }
  if (stepId === 'execute') {
    return [
      'Continue the Analytix Create Loop workflow by executing the approved plan.',
      '',
      `Objective:\n${run.objective}`,
      context ? `\nPrior workflow output:\n${context}` : '',
      '',
      'Use available tools when needed. Preserve generated file paths and review-relevant details in the final answer.'
    ].join('\n').trim()
  }
  return [
    'Finish the Analytix Create Loop workflow with a review summary.',
    '',
    `Objective:\n${run.objective}`,
    context ? `\nPrior workflow output:\n${context}` : '',
    '',
    'Return: outcome, generated files or attachments, risks, unresolved approvals or user inputs, and recommended next action.'
  ].join('\n').trim()
}

export function findPendingWorkflowGate(
  blocks: readonly ChatBlock[]
): { kind: 'approval' | 'user-input'; itemId: string; message: string } | null {
  for (const block of [...blocks].reverse()) {
    if (block.kind === 'approval' && block.status === 'pending') {
      return {
        kind: 'approval',
        itemId: block.id,
        message: block.summary || 'Approval required'
      }
    }
    if (block.kind === 'user_input' && block.status === 'pending') {
      return {
        kind: 'user-input',
        itemId: block.id,
        message: block.questions[0]?.question || 'User input required'
      }
    }
  }
  return null
}

export function latestAssistantOutput(blocks: readonly ChatBlock[]): string {
  for (const block of [...blocks].reverse()) {
    if (block.kind === 'assistant') return block.text.trim()
    if (block.kind === 'review') return (block.reviewText || block.title).trim()
    if (block.kind === 'system' && block.severity === 'error') return block.text.trim()
  }
  return ''
}

function threadStatusError(status: string | undefined): string | null {
  const normalized = status?.trim().toLowerCase()
  if (!normalized) return null
  return ERROR_THREAD_STATUSES.has(normalized) ? normalized : null
}

function threadLooksActive(detail: ThreadDetail): boolean {
  const normalized = detail.threadStatus?.trim().toLowerCase()
  return normalized ? ACTIVE_THREAD_STATUSES.has(normalized) : false
}

function threadLooksComplete(detail: ThreadDetail): boolean {
  const normalized = detail.threadStatus?.trim().toLowerCase()
  return normalized ? COMPLETE_THREAD_STATUSES.has(normalized) : false
}

function sleep(ms: number): Promise<void> {
  if (ms <= 0) return Promise.resolve()
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function waitForTurn(
  provider: CreateLoopRuntimeProvider,
  threadId: string,
  options: { pollIntervalMs: number; timeoutMs: number; turnId?: string }
): Promise<WaitResult> {
  const deadline = Date.now() + options.timeoutMs
  let lastOutput = ''
  let hasSeenTargetTurn = !options.turnId
  for (;;) {
    const detail = await provider.getThreadDetail(threadId)
    if (detail.latestTurnId === options.turnId) hasSeenTargetTurn = true
    const output = latestAssistantOutput(detail.blocks)
    if (output) lastOutput = output
    const gate = findPendingWorkflowGate(detail.blocks)
    if (gate) {
      return {
        status: 'waiting',
        waitingFor: gate.kind,
        waitingItemId: gate.itemId,
        message: gate.message,
        output: lastOutput
      }
    }
    const statusError = threadStatusError(detail.threadStatus)
    if (statusError) {
      return {
        status: 'error',
        message: `Runtime thread ${statusError}`,
        output: lastOutput
      }
    }
    if (!threadLooksActive(detail) && (output || (hasSeenTargetTurn && threadLooksComplete(detail)))) {
      return {
        status: 'success',
        output: lastOutput,
        message: lastOutput ? 'Completed' : 'Completed without assistant output'
      }
    }
    if (Date.now() >= deadline) {
      return {
        status: 'error',
        message: 'Timed out waiting for runtime turn to finish',
        output: lastOutput
      }
    }
    await sleep(options.pollIntervalMs)
  }
}

function firstRunnableStepIndex(run: CreateLoopRun): number {
  const index = run.steps.findIndex((step) => step.status !== 'success')
  return index < 0 ? run.steps.length : index
}

function updateRun(run: CreateLoopRun, now: () => Date, patch: Partial<CreateLoopRun>): CreateLoopRun {
  return {
    ...run,
    ...patch,
    updatedAt: now().toISOString(),
    steps: patch.steps ?? run.steps
  }
}

export async function runCreateLoopWorkflow(options: CreateLoopRunOptions): Promise<CreateLoopRun> {
  const now = options.now ?? (() => new Date())
  const emit = (run: CreateLoopRun): void => options.onUpdate?.(cloneRun(run))
  const pollIntervalMs = options.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS

  let run = options.existingRun ? cloneRun(options.existingRun) : null
  if (run) {
    run = updateRun(run, now, {
      status: 'running',
      lastError: undefined,
      lastMessage: 'Resumed',
      steps: ensureStepRuns(run)
    })
  } else {
    const objective = options.objective.trim()
    const thread = await options.provider.createThread({
      workspace: options.workspace,
      title: objective ? `Create Loop: ${objective.slice(0, 72)}` : 'Create Loop',
      mode: 'agent',
      model: options.model,
      providerId: options.providerId
    })
    run = createRun(options, thread, now())
  }
  emit(run)

  for (let index = firstRunnableStepIndex(run); index < run.steps.length; index += 1) {
    const current = run.steps[index]
    const startedAt = current.startedAt ?? now().toISOString()
    let turnId = current.turnId
    let userMessageItemId = current.userMessageItemId

    run.steps[index] = {
      ...current,
      status: 'running',
      startedAt,
      waitingFor: undefined,
      waitingItemId: undefined,
      error: undefined,
      message: current.status === 'waiting' && current.turnId ? 'Checking pending runtime work' : 'Sending turn'
    }
    run = updateRun(run, now, { status: 'running', lastMessage: run.steps[index].message, steps: [...run.steps] })
    emit(run)

    try {
      if (!(current.status === 'waiting' && current.turnId)) {
        const sent = await options.provider.sendUserMessage(
          run.threadId,
          buildCreateLoopStepPrompt(run, current.id),
          {
            mode: 'agent',
            model: run.model,
            providerId: run.providerId,
            displayText: `Create Loop / ${current.title}: ${run.objective}`
          }
        )
        turnId = sent.turnId
        userMessageItemId = sent.userMessageItemId
        run.steps[index] = {
          ...run.steps[index],
          turnId,
          userMessageItemId,
          message: 'Waiting for runtime'
        }
        run = updateRun(run, now, { steps: [...run.steps], lastMessage: run.steps[index].message })
        emit(run)
      }

      const result = await waitForTurn(options.provider, run.threadId, {
        pollIntervalMs,
        timeoutMs,
        turnId
      })
      if (result.status === 'waiting') {
        run.steps[index] = {
          ...run.steps[index],
          status: 'waiting',
          waitingFor: result.waitingFor,
          waitingItemId: result.waitingItemId,
          message: result.message,
          output: result.output,
          turnId,
          userMessageItemId
        }
        run = updateRun(run, now, {
          status: 'waiting',
          lastMessage: result.message,
          steps: [...run.steps]
        })
        emit(run)
        return cloneRun(run)
      }
      if (result.status === 'error') {
        run.steps[index] = {
          ...run.steps[index],
          status: 'error',
          error: result.message,
          output: result.output,
          finishedAt: now().toISOString(),
          turnId,
          userMessageItemId
        }
        run = updateRun(run, now, {
          status: 'error',
          lastError: result.message,
          lastMessage: result.message,
          steps: [...run.steps]
        })
        emit(run)
        return cloneRun(run)
      }

      run.steps[index] = {
        ...run.steps[index],
        status: 'success',
        output: result.output,
        message: result.message,
        finishedAt: now().toISOString(),
        turnId,
        userMessageItemId
      }
      run = updateRun(run, now, {
        status: 'running',
        lastMessage: `${current.title} completed`,
        steps: [...run.steps]
      })
      emit(run)
    } catch (error) {
      const message = formatError(error)
      run.steps[index] = {
        ...run.steps[index],
        status: 'error',
        error: message,
        finishedAt: now().toISOString(),
        turnId,
        userMessageItemId
      }
      run = updateRun(run, now, {
        status: 'error',
        lastError: message,
        lastMessage: message,
        steps: [...run.steps]
      })
      emit(run)
      return cloneRun(run)
    }
  }

  run = updateRun(run, now, {
    status: 'success',
    lastError: undefined,
    lastMessage: 'Create Loop completed',
    steps: [...run.steps]
  })
  emit(run)
  return cloneRun(run)
}
