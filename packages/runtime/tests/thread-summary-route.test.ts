import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { DurableTaskJobManager, FileTaskJobStore } from '../src/delegation-test-support/job-manager.js'
import type { ThreadRecord } from '../src/contracts/threads.js'
import {
  makePublicToolCallArgumentsProjection,
  makePublicToolResultWithheldProjection
} from '../src/domain/item.js'
import { buildHarness, readJson } from './http-server-test-harness.js'

const PRIVATE_OUTPUT_SENTINEL = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

async function waitFor(assertion: () => Promise<boolean> | boolean): Promise<void> {
  const deadline = Date.now() + 1000
  while (Date.now() < deadline) {
    if (await assertion()) return
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
  throw new Error('condition was not met')
}

function authGet(path: string): Request {
  return new Request(`http://localhost${path}`, {
    headers: { authorization: 'Bearer tok-1' }
  })
}

function authPost(path: string): Request {
  return new Request(`http://localhost${path}`, {
    method: 'POST',
    headers: { authorization: 'Bearer tok-1' }
  })
}

describe('thread summary route', () => {
  let dir = ''
  let tick = 0

  beforeEach(async () => {
    dir = await mkdtemp(join(tmpdir(), 'analytix-thread-summary-'))
    tick = 0
  })

  afterEach(async () => {
    await rm(dir, { recursive: true, force: true })
  })

  it('separates event-projected child runs from manual side chats', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:00:${String(tick++).padStart(2, '0')}.000Z`
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_parent', title: 'Parent' })
    const side = await h.threadService.fork('thr_parent', {
      relation: 'side',
      title: 'Side research'
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, {
      id: 'thr_child',
      title: 'Research child thread',
      relation: 'side',
      parentThreadId: 'thr_parent'
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      text: 'child summary',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        parentToolCallId: 'call_child',
        childId: 'child_1',
        childRunId: 'run_1',
        childThreadId: 'thr_child',
        childLabel: 'Research child',
        childStatus: 'completed',
        childModel: 'deepseek-chat',
        childProviderId: 'anthropic-main',
        childEndpointFormat: 'messages',
        childModelSource: 'thread',
        childModelExecution: {
          providerId: 'anthropic-main',
          modelId: 'deepseek-chat',
          endpointFormat: 'messages',
          source: 'thread',
          resolvedAt: '2026-06-29T00:00:00.000Z'
        },
        childEffort: 'high',
        background: true,
        totalTokens: 42,
        evidenceLedgered: true,
        cacheHitRate: 0.5
      }
    })

    const response = await dispatchRequest(h.router, authGet('/v1/threads/thr_parent/summary'))
    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      latestSeq: number
      subagents: Array<{
        key: string
        status: string
        displayName?: string
        agentNickname?: string
        title?: string
        label?: string
        childThreadId?: string
        providerId?: string
        endpointFormat?: string
        modelSource?: string
        totalTokens?: number
        evidenceLedgered?: boolean
        cacheHitRate?: number | null
      }>
      sideChats: Array<{ threadId: string; title: string; relation: string; parentThreadId: string }>
    }

    expect(body.latestSeq).toBeGreaterThanOrEqual(2)
    expect(body.subagents).toEqual(expect.arrayContaining([
      expect.objectContaining({
        key: 'run:run_1',
        status: 'done',
        displayName: 'Research child',
        agentNickname: 'Research child',
        title: 'Research child',
        label: 'Research child',
        childThreadId: 'thr_child',
        providerId: 'anthropic-main',
        endpointFormat: 'messages',
        modelSource: 'thread',
        totalTokens: 42,
        evidenceLedgered: true,
        cacheHitRate: 0.5
      })
    ]))
    expect(JSON.stringify(body)).not.toContain('modelExecution')
    expect(body.subagents).toHaveLength(1)
    expect(body.sideChats).toEqual([expect.objectContaining({
      threadId: side.id,
      title: 'Side research',
      relation: 'side',
      parentThreadId: 'thr_parent'
    })])
  })

  it('recovers subagent labels from child thread metadata when old records say task', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:00:${String(tick++).padStart(2, '0')}.000Z`
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_parent_generic', title: 'Parent' })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, {
      id: 'thr_child_generic',
      title: 'Child agent: task',
      relation: 'side',
      parentThreadId: 'thr_parent_generic'
    })
    await h.sessionStore.appendItem('thr_child_generic', {
      id: 'item_child_prompt',
      threadId: 'thr_child_generic',
      turnId: 'turn_child_prompt',
      kind: 'user_message',
      role: 'user',
      status: 'completed',
      text: 'Inspect model capability boundaries',
      createdAt: h.nowIso()
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_parent_generic',
      turnId: 'turn_parent_generic',
      text: 'done',
      child: {
        parentThreadId: 'thr_parent_generic',
        parentTurnId: 'turn_parent_generic',
        parentToolCallId: 'call_child_generic',
        childId: 'child_generic',
        childRunId: 'run_generic',
        childThreadId: 'thr_child_generic',
        childLabel: 'task',
        childStatus: 'completed'
      }
    })

    const response = await dispatchRequest(h.router, authGet('/v1/threads/thr_parent_generic/summary'))
    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      subagents: Array<{ key: string; displayName?: string; agentNickname?: string; title?: string; label?: string }>
      sideChats: unknown[]
    }
    expect(body.subagents).toHaveLength(1)
    expect(body.subagents[0]).toEqual(expect.objectContaining({
      key: 'run:run_generic'
    }))
    expect(body.subagents[0]?.displayName).toBe(body.subagents[0]?.title)
    expect(body.subagents[0]?.agentNickname).toBe(body.subagents[0]?.title)
    expect(body.subagents[0]?.title).toBe('Inspect model capability boundaries')
    expect(body.subagents[0]?.title).not.toBe('task')
    expect(body.subagents[0]?.label).toBe(body.subagents[0]?.title)
    expect(body.sideChats).toEqual([])
  })

  it('recovers Chinese subagent prompts when generic labels and titles are all that old records have', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:00:${String(tick++).padStart(2, '0')}.000Z`
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_parent_chinese_prompt', title: 'Parent' })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, {
      id: 'thr_child_chinese_prompt',
      title: 'Child agent: delegate_task',
      relation: 'side',
      parentThreadId: 'thr_parent_chinese_prompt'
    })
    await h.sessionStore.appendItem('thr_child_chinese_prompt', {
      id: 'item_child_chinese_prompt',
      threadId: 'thr_child_chinese_prompt',
      turnId: 'turn_child_chinese_prompt',
      kind: 'user_message',
      role: 'user',
      status: 'completed',
      text: '分析桌面文件并总结关键风险',
      createdAt: h.nowIso()
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_parent_chinese_prompt',
      turnId: 'turn_parent_chinese_prompt',
      text: 'done',
      child: {
        parentThreadId: 'thr_parent_chinese_prompt',
        parentTurnId: 'turn_parent_chinese_prompt',
        parentToolCallId: 'call_child_chinese_prompt',
        childId: 'child_chinese_prompt',
        childRunId: 'run_chinese_prompt',
        childThreadId: 'thr_child_chinese_prompt',
        childLabel: 'delegate_task',
        childStatus: 'completed'
      }
    })

    const response = await dispatchRequest(h.router, authGet('/v1/threads/thr_parent_chinese_prompt/summary'))
    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      subagents: Array<{ key: string; displayName?: string; agentNickname?: string; title?: string; label?: string }>
      sideChats: unknown[]
    }
    expect(body.subagents).toEqual([
      expect.objectContaining({
        key: 'run:run_chinese_prompt',
        displayName: '分析桌面文件并总结关键风险',
        agentNickname: '分析桌面文件并总结关键风险',
        title: '分析桌面文件并总结关键风险',
        label: '分析桌面文件并总结关键风险'
      })
    ])
    expect(body.sideChats).toEqual([])
  })

  it('uses profile names before Codex-style nicknames when no better metadata exists', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:00:${String(tick++).padStart(2, '0')}.000Z`
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_parent_profile_name', title: 'Parent' })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, {
      id: 'thr_child_profile_name',
      title: 'Child agent: design-reviewer',
      relation: 'side',
      parentThreadId: 'thr_parent_profile_name'
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_parent_profile_name',
      turnId: 'turn_parent_profile_name',
      text: 'done',
      child: {
        parentThreadId: 'thr_parent_profile_name',
        parentTurnId: 'turn_parent_profile_name',
        parentToolCallId: 'call_child_profile_name',
        childId: 'child_1',
        childRunId: 'job-16',
        childThreadId: 'thr_child_profile_name',
        childLabel: 'task',
        childProfile: 'design-reviewer',
        parallelIndex: 1,
        childStatus: 'completed'
      }
    })

    const response = await dispatchRequest(h.router, authGet('/v1/threads/thr_parent_profile_name/summary'))
    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      subagents: Array<{ displayName?: string; agentNickname?: string; title?: string; label?: string; profile?: string; parallelIndex?: number }>
    }
    expect(body.subagents).toEqual([
      expect.objectContaining({
        displayName: 'design-reviewer',
        agentNickname: 'design-reviewer',
        title: 'design-reviewer',
        label: 'design-reviewer',
        profile: 'design-reviewer',
        parallelIndex: 1
      })
    ])
  })

  it('uses child names before generic labels and titles', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:00:${String(tick++).padStart(2, '0')}.000Z`
    })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_parent_child_name', title: 'Parent' })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, {
      id: 'thr_child_name',
      title: 'Child agent: task',
      relation: 'side',
      parentThreadId: 'thr_parent_child_name'
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_parent_child_name',
      turnId: 'turn_parent_child_name',
      text: 'done',
      child: {
        parentThreadId: 'thr_parent_child_name',
        parentTurnId: 'turn_parent_child_name',
        parentToolCallId: 'call_child_name',
        childId: 'child_name',
        childRunId: 'run_child_name',
        childThreadId: 'thr_child_name',
        childLabel: 'task',
        childName: '桌面文件概览分析',
        parallelIndex: 2,
        childStatus: 'completed'
      }
    })

    const response = await dispatchRequest(h.router, authGet('/v1/threads/thr_parent_child_name/summary'))
    expect(response.status).toBe(200)
    expect(await readJson(response)).toMatchObject({
      subagents: [
        expect.objectContaining({
          key: 'run:run_child_name',
          displayName: '桌面文件概览分析',
          agentNickname: '桌面文件概览分析',
          title: '桌面文件概览分析',
          label: '桌面文件概览分析',
          parallelIndex: 2
        })
      ],
      sideChats: []
    })
  })

  it('aggregates task jobs and enforces thread ownership for output and kill', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:01:${String(tick++).padStart(2, '0')}.000Z`
    })
    const manager = new DurableTaskJobManager({
      store: new FileTaskJobStore(dir),
      nowIso: h.nowIso,
      idGenerator: (kind) => `${kind}_summary_1`
    })
    h.runtime.taskJobs = manager
    await h.threadService.create({ workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' }, { id: 'thr_tasks' })
    await h.threadService.create({ workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' }, { id: 'thr_other' })

    const release = deferred<string>()
    const job = await manager.startBackground({
      kind: 'task',
      parentThreadId: 'thr_tasks',
      parentTurnId: 'turn_tasks',
      label: 'Long task',
      childRunId: 'child_long_task'
    }, async ({ appendOutput }) => {
      await appendOutput(`task output\n${PRIVATE_OUTPUT_SENTINEL}\n`)
      return release.promise
    })
    await waitFor(async () => (await manager.output(job.id, { offset: 0 }))?.output.includes(PRIVATE_OUTPUT_SENTINEL) === true)
    const parallel = await manager.startForeground({
      kind: 'parallel_task',
      parentThreadId: 'thr_tasks',
      parentTurnId: 'turn_tasks',
      label: 'Follow-up task',
      dependencies: ['a'],
      childRunId: 'child_parallel_task'
    }, async ({ appendOutput }) => {
      await appendOutput(`parallel output\n${PRIVATE_OUTPUT_SENTINEL}\n`)
      return PRIVATE_OUTPUT_SENTINEL
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_tasks',
      turnId: 'turn_tasks',
      text: 'long task child summary',
      child: {
        parentThreadId: 'thr_tasks',
        parentTurnId: 'turn_tasks',
        childId: 'child_long_task',
        childRunId: 'child_long_task',
        childThreadId: 'thr_child_long_task',
        childStatus: 'completed'
      }
    })
    await h.runtime.events.record({
      kind: 'turn_completed',
      threadId: 'thr_tasks',
      turnId: 'turn_tasks',
      text: 'parallel child summary',
      child: {
        parentThreadId: 'thr_tasks',
        parentTurnId: 'turn_tasks',
        childId: 'child_parallel_task',
        childRunId: 'child_parallel_task',
        childThreadId: 'thr_child_parallel_task',
        childStatus: 'completed'
      }
    })

    const summary = await dispatchRequest(h.router, authGet('/v1/threads/thr_tasks/summary'))
    expect(summary.status).toBe(200)
    const summaryBody = await readJson(summary) as {
      tasks: Array<{
        schemaVersion: number
        id: string
        kind: string
        status: string
        background: boolean
        terminal: boolean
        outputWithheld: boolean
        outputTrustStatus: string
        factAnswerAllowed: boolean
        evidenceAuthority: boolean
        canReadOutput: boolean
        canContinueParent: boolean
      }>
    }
    expect(summaryBody.tasks).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: `taskjob:${job.id}`,
        kind: 'task',
        status: 'running',
        background: false,
        terminal: false,
        outputWithheld: true,
        outputTrustStatus: 'untrusted_child_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      }),
      expect.objectContaining({
        id: `taskjob:${parallel.id}`,
        kind: 'parallel_task',
        status: 'completed',
        background: true,
        terminal: true,
        outputWithheld: true,
        outputTrustStatus: 'untrusted_child_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      })
    ]))
    expect(JSON.stringify(summaryBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)

    const taskId = encodeURIComponent(`taskjob:${job.id}`)
    const output = await dispatchRequest(h.router, authGet(`/v1/threads/thr_tasks/summary/tasks/${taskId}/output?offset=0`))
    expect(output.status).toBe(200)
    const outputBody = await readJson(output)
    expect(outputBody).toEqual({
      schemaVersion: 1,
      availability: 'withheld',
      taskId: `taskjob:${job.id}`,
      status: 'running',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    })
    expect(JSON.stringify(outputBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)

    const crossThreadOutput = await dispatchRequest(h.router, authGet(`/v1/threads/thr_other/summary/tasks/${taskId}/output?offset=0`))
    expect(crossThreadOutput.status).toBe(403)
    expect(await readJson(crossThreadOutput)).toMatchObject({ code: 'forbidden' })

    const crossThreadKill = await dispatchRequest(h.router, authPost(`/v1/threads/thr_other/summary/tasks/${taskId}/kill`))
    expect(crossThreadKill.status).toBe(403)
    expect(await readJson(crossThreadKill)).toMatchObject({ code: 'forbidden' })

    const kill = await dispatchRequest(h.router, authPost(`/v1/threads/thr_tasks/summary/tasks/${taskId}/kill`))
    expect(kill.status).toBe(200)
    const killBody = await readJson(kill)
    expect(killBody).toMatchObject({
      task: {
        id: `taskjob:${job.id}`,
        kind: 'task',
        status: 'killed',
        terminal: true,
        outputWithheld: true,
        outputTrustStatus: 'untrusted_child_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      }
    })
    expect(JSON.stringify(killBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    release.resolve('done')
  })

  it('does not reconstruct command restart authority from public metadata', async () => {
    const h = buildHarness({
      nowIso: () => `2026-06-29T00:03:${String(tick++).padStart(2, '0')}.000Z`
    })
    const workspace = join(dir, 'workspace')
    const outside = join(dir, 'outside')
    h.runtime.taskJobs = new DurableTaskJobManager({
      store: new FileTaskJobStore(dir),
      nowIso: h.nowIso,
      idGenerator: (kind) => `${kind}_restart_1`
    })

    const allowed = await h.threadService.create({
      workspace,
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'auto',
      sandboxMode: 'workspace-write'
    }, { id: 'thr_restart_allowed' })
    await putCommandThread(h, allowed, { cwd: workspace, command: 'printf restarted' })

    const allowedSummary = await dispatchRequest(h.router, authGet('/v1/threads/thr_restart_allowed/summary'))
    expect(allowedSummary.status).toBe(200)
    expect(await readJson(allowedSummary)).toMatchObject({
      tasks: [expect.objectContaining({
        id: 'command:call_restart',
        kind: 'command',
        outputWithheld: true,
        outputTrustStatus: 'private_tool_output',
        factAnswerAllowed: false,
        evidenceAuthority: false,
        canReadOutput: false,
        canContinueParent: false
      })]
    })
    const commandOutput = await dispatchRequest(
      h.router,
      authGet('/v1/threads/thr_restart_allowed/summary/tasks/command%3Acall_restart/output?offset=0')
    )
    expect(commandOutput.status).toBe(200)
    expect(await readJson(commandOutput)).toEqual({
      schemaVersion: 1,
      availability: 'withheld',
      taskId: 'command:call_restart',
      status: 'completed',
      reasonCode: 'tool_output_private',
      outputWithheld: true,
      outputTrustStatus: 'private_tool_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    })
    const restarted = await dispatchRequest(h.router, authPost('/v1/threads/thr_restart_allowed/summary/tasks/command%3Acall_restart/restart'))
    expect(restarted.status).toBe(409)
    expect(await readJson(restarted)).toMatchObject({
      code: 'conflict',
      message: 'The request conflicts with the current runtime state.'
    })

    const policyBlocked = await h.threadService.create({
      workspace,
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'never',
      sandboxMode: 'workspace-write'
    }, { id: 'thr_restart_policy' })
    await putCommandThread(h, policyBlocked, { cwd: workspace, command: 'printf blocked' })
    const policySummary = await dispatchRequest(h.router, authGet('/v1/threads/thr_restart_policy/summary'))
    expect(await readJson(policySummary)).toMatchObject({
      tasks: [expect.objectContaining({
        id: 'command:call_restart',
        kind: 'command',
        outputWithheld: true,
        canReadOutput: false
      })]
    })
    const deniedPolicy = await dispatchRequest(h.router, authPost('/v1/threads/thr_restart_policy/summary/tasks/command%3Acall_restart/restart'))
    expect(deniedPolicy.status).toBe(409)
    expect(await readJson(deniedPolicy)).toMatchObject({
      code: 'conflict',
      message: 'The request conflicts with the current runtime state.'
    })

    const outsideBlocked = await h.threadService.create({
      workspace,
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'auto',
      sandboxMode: 'workspace-write'
    }, { id: 'thr_restart_outside' })
    await putCommandThread(h, outsideBlocked, { cwd: outside, command: 'printf outside' })
    const outsideSummary = await dispatchRequest(h.router, authGet('/v1/threads/thr_restart_outside/summary'))
    expect(await readJson(outsideSummary)).toMatchObject({
      tasks: [expect.objectContaining({
        id: 'command:call_restart',
        kind: 'command',
        outputWithheld: true,
        canReadOutput: false
      })]
    })
    const deniedOutside = await dispatchRequest(h.router, authPost('/v1/threads/thr_restart_outside/summary/tasks/command%3Acall_restart/restart'))
    expect(deniedOutside.status).toBe(409)
    expect(await readJson(deniedOutside)).toMatchObject({
      code: 'conflict',
      message: 'The request conflicts with the current runtime state.'
    })
  })
})

async function putCommandThread(
  h: ReturnType<typeof buildHarness>,
  thread: ThreadRecord,
  input: { cwd: string; command: string }
): Promise<void> {
  const now = h.nowIso()
  await h.threadStore.upsert({
    ...thread,
    turns: [{
      id: 'turn_restart',
      threadId: thread.id,
      status: 'completed',
      prompt: 'restart command',
      steering: [],
      createdAt: now,
      attachmentIds: [],
      activeSkillIds: [],
      injectedMemoryIds: [],
      items: [
        {
          id: `item_call_${thread.id}`,
          threadId: thread.id,
          turnId: 'turn_restart',
          role: 'assistant',
          kind: 'tool_call',
          toolName: 'bash',
          toolKind: 'command_execution',
          callId: 'call_restart',
          status: 'completed',
          arguments: makePublicToolCallArgumentsProjection(),
          createdAt: now
        },
        {
          id: `item_result_${thread.id}`,
          threadId: thread.id,
          turnId: 'turn_restart',
          role: 'tool',
          kind: 'tool_result',
          toolName: 'bash',
          toolKind: 'command_execution',
          callId: 'call_restart',
          status: 'completed',
          isError: false,
          output: makePublicToolResultWithheldProjection({ status: 'completed' }),
          createdAt: now,
          finishedAt: now
        }
      ]
    }]
  })
}
