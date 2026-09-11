import { describe, expect, it, vi } from 'vitest'
import type { AgentProvider, ChatBlock, NormalizedThread } from '../agent/types'
import {
  buildCreateLoopStepPrompt,
  findPendingWorkflowGate,
  runCreateLoopWorkflow,
  type CreateLoopRuntimeProvider,
  type CreateLoopRun
} from './create-loop-runtime'

type Detail = Awaited<ReturnType<AgentProvider['getThreadDetail']>>

function thread(patch: Partial<NormalizedThread> = {}): NormalizedThread {
  return {
    id: 'thread-workflow',
    title: 'Create Loop',
    updatedAt: '2026-06-20T00:00:00.000Z',
    model: 'model-a',
    providerId: 'provider-a',
    mode: 'agent',
    workspace: '/tmp/workflow',
    status: 'idle',
    ...patch
  }
}

function detail(turnId: string, blocks: ChatBlock[], status = 'completed'): Detail {
  return {
    blocks,
    latestSeq: 1,
    latestTurnId: turnId,
    threadStatus: status
  }
}

function assistant(id: string, text: string): ChatBlock {
  return { kind: 'assistant', id, text }
}

function approval(id = 'approval-1'): ChatBlock {
  return {
    kind: 'approval',
    id,
    approvalId: id,
    summary: 'Approve the plan',
    status: 'pending'
  }
}

function userInput(id = 'input-1'): ChatBlock {
  return {
    kind: 'user_input',
    id,
    requestId: id,
    questions: [
      {
        id: 'choice',
        header: 'Choice',
        question: 'Pick an option',
        options: [{ label: 'A', description: 'Use A' }]
      }
    ],
    status: 'pending'
  }
}

function provider(details: Detail[]): CreateLoopRuntimeProvider & {
  createThread: ReturnType<typeof vi.fn>
  sendUserMessage: ReturnType<typeof vi.fn>
  getThreadDetail: ReturnType<typeof vi.fn>
} {
  let sendCount = 0
  let detailIndex = 0
  return {
    createThread: vi.fn(async () => thread()),
    sendUserMessage: vi.fn(async () => {
      sendCount += 1
      return {
        threadId: 'thread-workflow',
        turnId: `turn-${sendCount}`,
        userMessageItemId: `user-${sendCount}`
      }
    }),
    getThreadDetail: vi.fn(async () => {
      const next = details[Math.min(detailIndex, details.length - 1)]
      detailIndex += 1
      return next
    })
  }
}

describe('Create Loop runtime', () => {
  it('creates a runtime thread and relays each step with the selected provider/model', async () => {
    const runtime = provider([
      detail('turn-1', [assistant('a1', 'plan output')]),
      detail('turn-2', [assistant('a2', 'execute output')]),
      detail('turn-3', [assistant('a3', 'review output')])
    ])
    const updates: CreateLoopRun[] = []

    const run = await runCreateLoopWorkflow({
      provider: runtime,
      objective: 'Ship workflow support',
      workspace: '/tmp/workflow',
      model: 'model-a',
      providerId: 'provider-a',
      pollIntervalMs: 0,
      onUpdate: (next) => updates.push(next)
    })

    expect(run.status).toBe('success')
    expect(run.steps.map((step) => step.status)).toEqual(['success', 'success', 'success'])
    expect(runtime.createThread).toHaveBeenCalledWith({
      workspace: '/tmp/workflow',
      title: 'Create Loop: Ship workflow support',
      mode: 'agent',
      model: 'model-a',
      providerId: 'provider-a'
    })
    expect(runtime.sendUserMessage).toHaveBeenCalledTimes(3)
    for (const call of runtime.sendUserMessage.mock.calls) {
      expect(call[2]).toMatchObject({ model: 'model-a', providerId: 'provider-a', mode: 'agent' })
    }
    const executePrompt = String(runtime.sendUserMessage.mock.calls[1][1])
    expect(executePrompt).toContain('plan output')
    expect(updates.at(-1)?.status).toBe('success')
  })

  it('pauses on approval and resumes the same step without creating a new thread', async () => {
    const firstRuntime = provider([detail('turn-1', [approval()])])
    const waiting = await runCreateLoopWorkflow({
      provider: firstRuntime,
      objective: 'Needs approval',
      model: 'model-a',
      providerId: 'provider-a',
      pollIntervalMs: 0
    })

    expect(waiting.status).toBe('waiting')
    expect(waiting.steps[0]).toMatchObject({
      status: 'waiting',
      waitingFor: 'approval',
      waitingItemId: 'approval-1'
    })

    const resumeRuntime = provider([
      detail('turn-1', [assistant('a1', 'approved plan')]),
      detail('turn-1', [assistant('a2', 'executed')]),
      detail('turn-2', [assistant('a3', 'reviewed')])
    ])
    const resumed = await runCreateLoopWorkflow({
      provider: resumeRuntime,
      objective: 'Needs approval',
      existingRun: waiting,
      pollIntervalMs: 0
    })

    expect(resumed.status).toBe('success')
    expect(resumeRuntime.createThread).not.toHaveBeenCalled()
    expect(resumeRuntime.sendUserMessage).toHaveBeenCalledTimes(2)
    expect(resumed.threadId).toBe(waiting.threadId)
  })

  it('detects runtime user-input gates as workflow waits', async () => {
    expect(findPendingWorkflowGate([userInput()])).toEqual({
      kind: 'user-input',
      itemId: 'input-1',
      message: 'Pick an option'
    })

    const runtime = provider([detail('turn-1', [userInput()])])
    const run = await runCreateLoopWorkflow({
      provider: runtime,
      objective: 'Needs input',
      pollIntervalMs: 0
    })

    expect(run.status).toBe('waiting')
    expect(run.steps[0].waitingFor).toBe('user-input')
  })

  it('supports retry after a failed step', async () => {
    const failedRuntime = provider([detail('turn-1', [], 'failed')])
    const failed = await runCreateLoopWorkflow({
      provider: failedRuntime,
      objective: 'Retry me',
      pollIntervalMs: 0
    })

    expect(failed.status).toBe('error')
    expect(failed.steps[0].status).toBe('error')

    const retryRuntime = provider([
      detail('turn-1', [assistant('a1', 'plan after retry')]),
      detail('turn-2', [assistant('a2', 'execute after retry')]),
      detail('turn-3', [assistant('a3', 'review after retry')])
    ])
    const retried = await runCreateLoopWorkflow({
      provider: retryRuntime,
      objective: 'Retry me',
      existingRun: failed,
      pollIntervalMs: 0
    })

    expect(retried.status).toBe('success')
    expect(retryRuntime.createThread).not.toHaveBeenCalled()
    expect(retryRuntime.sendUserMessage).toHaveBeenCalledTimes(3)
    expect(retried.threadId).toBe(failed.threadId)
  })

  it('builds a review prompt that carries prior outputs', () => {
    const run: CreateLoopRun = {
      id: 'run-1',
      workflowId: 'analytix-create-loop',
      title: 'Create Loop',
      objective: 'Summarize files',
      status: 'running',
      threadId: 'thread-1',
      createdAt: '2026-06-20T00:00:00.000Z',
      updatedAt: '2026-06-20T00:00:00.000Z',
      steps: [
        { id: 'plan', title: 'Plan', status: 'success', output: 'Plan text' },
        { id: 'execute', title: 'Execute', status: 'success', output: 'Generated /tmp/out.md' },
        { id: 'review', title: 'Review', status: 'pending' }
      ]
    }

    const prompt = buildCreateLoopStepPrompt(run, 'review')
    expect(prompt).toContain('Plan text')
    expect(prompt).toContain('Generated /tmp/out.md')
    expect(prompt).toContain('generated files')
  })
})
