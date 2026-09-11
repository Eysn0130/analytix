import { readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { buildTaskJobToolProviders } from '../src/tool-test-support/tool/task-job-tool-provider.js'
import { LocalToolHost } from '../src/tool-test-support/tool/local-tool-host.js'
import { AnalytixCapabilitiesConfig } from '../src/contracts/capabilities.js'
import { TaskJobOrchestrationContract } from '../src/conformance/runtime-parity-fixtures.js'
import { DelegationRuntime, FileDelegationStore } from '../src/delegation-test-support/delegation-runtime.js'
import {
  DurableTaskJobManager,
  FileTaskJobStore,
  PARALLEL_TASKS_TOOL_CONTRACT,
  PLANNER_READ_ONLY_TOOLSET,
  TASK_JOB_CONTROL_TOOL_CONTRACT,
  TaskJobRecord as TaskJobRecordSchema,
  TASK_TOOL_CONTRACT,
  TASK_JOB_ROUTE_CONTRACT,
  normalizeParallelTaskPlan,
  resolveTranscriptOperation,
  validateParallelTaskPlan
} from '../src/delegation-test-support/job-manager.js'
import { runPlannerExecutorCoordinator } from '../src/delegation-test-support/planner-executor-coordinator.js'
import type { ToolHostContext, ToolHostResult } from '../src/ports/tool-host.js'
import { buildHarness, readJson } from './http-server-test-harness.js'

const contractUrl = new URL(
  '../src/conformance/fixtures/task-job-orchestration-contract.json',
  import.meta.url
)
const PRIVATE_OUTPUT_SENTINEL = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'

function withheldTaskJobOutput(jobId: string, status: string): Record<string, unknown> {
  return {
    schemaVersion: 1,
    availability: 'withheld',
    jobId,
    status,
    reasonCode: 'security_bound_child_output',
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false
  }
}

function withheldTaskJobSummary(
  id: string,
  kind: 'task' | 'parallel_task',
  status: string,
  background: boolean
): Record<string, unknown> {
  return {
    schemaVersion: 1,
    id,
    kind,
    status,
    background,
    terminal: ['completed', 'failed', 'aborted', 'interrupted', 'killed', 'canceled', 'timeout'].includes(status),
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false
  }
}

function loadContract(): TaskJobOrchestrationContract {
  return TaskJobOrchestrationContract.parse(JSON.parse(readFileSync(contractUrl, 'utf8')))
}

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

describe('task/background/planner orchestration contract', () => {
  let dir = ''
  let nowTick = 0

  beforeEach(async () => {
    dir = await mkdtemp(join(tmpdir(), 'analytix-task-jobs-'))
    nowTick = 0
  })

  afterEach(async () => {
    await rm(dir, { recursive: true, force: true })
  })

  it('pins foreground, background, wait/output/kill, parallel, planner, and transcript contracts', async () => {
    const contract = loadContract()
    let seq = 0
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:00:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_${++seq}`
    })

    expect(TASK_TOOL_CONTRACT).toEqual(contract.toolContracts.task)
    expect(PARALLEL_TASKS_TOOL_CONTRACT).toEqual(contract.toolContracts.parallelTasks)
    expect(TASK_JOB_ROUTE_CONTRACT).toMatchObject({
      wait: contract.routeContract.wait,
      output: contract.routeContract.output,
      kill: contract.routeContract.kill
    })
    for (const forbidden of contract.routeContract.forbiddenTopLevelRoutes) {
      expect(Object.values(TASK_JOB_ROUTE_CONTRACT)).not.toContain(forbidden)
    }

    const foreground = await manager.startForeground({
      kind: contract.foreground.kind,
      parentThreadId: 'thr_task',
      parentTurnId: 'turn_foreground'
    }, async ({ recordChildRun }) => {
      await recordChildRun('child_foreground_1')
      return contract.foreground.result
    })
    expect(foreground).toMatchObject({
      status: contract.foreground.status,
      result: contract.foreground.result,
      childRunId: 'child_foreground_1'
    })

    const releaseBackground = deferred<void>()
    const background = await manager.startBackground({
      kind: contract.background.kind,
      parentThreadId: 'thr_task',
      parentTurnId: 'turn_background'
    }, async ({ appendOutput }) => {
      await appendOutput(contract.background.output)
      await releaseBackground.promise
      return contract.background.finalResult
    })
    await waitFor(async () => {
      const output = await manager.output(background.id)
      return output?.output === contract.background.output
    })
    const nextTurnManager = new DurableTaskJobManager({ store })
    expect((await nextTurnManager.list('thr_task')).find((job) => job.id === background.id)).toMatchObject({
      status: contract.background.statusAcrossTurn
    })
    releaseBackground.resolve()
    await waitFor(async () =>
      (await manager.wait([background.id]))[0]?.status === contract.background.finalStatus
    )
    expect((await manager.wait([background.id]))[0]).toMatchObject({
      status: contract.background.finalStatus,
      result: contract.background.finalResult
    })

    const never = deferred<void>()
    const killed = await manager.startBackground({
      kind: 'task',
      parentThreadId: 'thr_task',
      parentTurnId: 'turn_kill'
    }, async ({ signal }) => {
      await new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new Error(contract.waitOutputKill.killedError)), { once: true })
      })
      await never.promise
    })
    expect(await manager.kill(killed.id, contract.waitOutputKill.killedError)).toMatchObject({
      status: contract.waitOutputKill.killedStatus,
      error: contract.waitOutputKill.killedError
    })

    const parentCancelled = new AbortController()
    const parentStarted = deferred<void>()
    const parentKilled = manager.startForeground({
      kind: 'task',
      parentThreadId: 'thr_task',
      parentTurnId: 'turn_parent_cancel'
    }, async ({ signal, appendOutput }) => {
      await appendOutput('partial sample\n')
      parentStarted.resolve()
      await new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new Error('parent cancel sample')), { once: true })
      })
    }, { parentSignal: parentCancelled.signal })
    await parentStarted.promise
    parentCancelled.abort('parent cancel sample')
    await expect(parentKilled).resolves.toMatchObject({
      status: 'killed',
      error: 'parent cancel sample',
      output: [expect.objectContaining({ text: 'partial sample\n' })]
    })

    expect(validateParallelTaskPlan(normalizeParallelTaskPlan([
      { id: 'a' },
      { id: 'b', depends_on: ['a'] },
      { id: 'c', depends_on: ['b'] }
    ]))).toEqual(contract.parallel.validOrder)
    expect(() => validateParallelTaskPlan([{ id: 'a' }]))
      .toThrow(contract.parallel.singleTaskError)
    expect(() => validateParallelTaskPlan([
      { id: 'a' },
      { id: 'a' }
    ])).toThrow(contract.parallel.duplicateIdError)
    expect(() => validateParallelTaskPlan([
      { id: 'a', dependsOn: ['a'] },
      { id: 'b' }
    ])).toThrow(contract.parallel.selfDependencyError)
    expect(() => validateParallelTaskPlan([
      { id: 'a', dependsOn: ['c'] },
      { id: 'b', dependsOn: ['a'] },
      { id: 'c', dependsOn: ['b'] }
    ])).toThrow(contract.parallel.cycleError)
    expect(() => validateParallelTaskPlan([{ id: 'a', dependsOn: ['missing'] }, { id: 'b' }]))
      .toThrow(contract.parallel.unknownDependencyError)

    const nested = await manager.startForeground({
      kind: 'task',
      parentThreadId: 'thr_task',
      parentTurnId: 'turn_nested',
      parentCallId: contract.nestedEvent.parentCallId,
      childRunId: contract.nestedEvent.childRunId,
      permissionPolicy: contract.permissions.inheritedPolicy
    }, async () => 'nested done')
    expect(contract.nestedEvent.nestedSseMetadataFields).toEqual([
      'parentCallId',
      'childRunId',
      contract.parentGoalEvidence.evidenceLedgeredEventKey,
      contract.parentGoalEvidence.evidenceLedgerErrorEventKey
    ])
    expect(nested).toMatchObject({
      parentCallId: contract.nestedEvent.parentCallId,
      childRunId: contract.nestedEvent.childRunId,
      permissionPolicy: contract.permissions.inheritedPolicy
    })
    expect(PLANNER_READ_ONLY_TOOLSET).toEqual(contract.permissions.plannerReadOnlyToolset)
    expect(contract.permissions.plannerForbiddenToolset).toEqual([
      TASK_TOOL_CONTRACT.name,
      PARALLEL_TASKS_TOOL_CONTRACT.name
    ])
    expect(PLANNER_READ_ONLY_TOOLSET).not.toEqual(
      expect.arrayContaining(contract.permissions.plannerForbiddenToolset)
    )

    const identity = {
      model: 'deepseek-v4-pro',
      effort: 'high',
      workspace: '/tmp/ws',
      toolNames: ['read', 'grep']
    }
    expect(resolveTranscriptOperation({
      mode: 'continue',
      sourceId: contract.transcript.sourceId,
      source: identity,
      requested: { ...identity, toolNames: ['grep', 'read'] }
    })).toMatchObject({
      mode: 'continue',
      sourceId: contract.transcript.sourceId,
      targetId: contract.transcript.continueTargetId
    })
    expect(contract.transcript.continuePreservesTarget).toBe(true)
    expect(resolveTranscriptOperation({
      mode: 'fork',
      sourceId: contract.transcript.sourceId,
      source: identity,
      requested: identity,
      newId: contract.transcript.forkTargetId
    })).toMatchObject({
      mode: 'fork',
      sourceId: contract.transcript.sourceId,
      targetId: contract.transcript.forkTargetId
    })
    expect(contract.transcript.forkCreatesDistinctTarget).toBe(true)
    expect(() => resolveTranscriptOperation({
      mode: 'continue',
      sourceId: contract.transcript.sourceId,
      source: identity,
      requested: { ...identity, model: 'other-model' }
    })).toThrow(contract.transcript.incompatibleError)
  })

  it('rejects durable transcript continue when hardened identity fields change', () => {
    const identity = {
      modelId: 'deepseek-v4-pro',
      model: 'deepseek-v4-pro',
      providerId: 'deepseek-main',
      endpointFormat: 'chat_completions',
      variant: 'stable',
      modelSource: 'thread',
      effort: 'high',
      profile: 'reviewer',
      workspace: '/tmp/ws',
      toolPolicy: 'readOnly',
      toolNames: ['read', 'grep'],
      systemPromptHash: 'system-a',
      promptPreambleHash: 'preamble-a',
      toolSchemaHash: 'tools-a',
      sandboxMode: 'workspace-write',
      approvalPolicy: 'auto',
      capabilityFingerprint: 'capability-a'
    }
    const mismatches = [
      { providerId: 'other-provider' },
      { profile: 'other-profile' },
      { toolSchemaHash: 'tools-b' },
      { systemPromptHash: 'system-b' },
      { promptPreambleHash: 'preamble-b' },
      { sandboxMode: 'read-only' },
      { approvalPolicy: 'never' },
      { capabilityFingerprint: 'capability-b' }
    ]

    for (const mismatch of mismatches) {
      expect(() => resolveTranscriptOperation({
        mode: 'continue',
        sourceId: 'child_1',
        source: identity,
        requested: { ...identity, ...mismatch }
      })).toThrow(/subagent transcript identity is incompatible/)
    }
  })

  it('serves task job wait/output/kill through authenticated runtime routes', async () => {
    const contract = loadContract()
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:01:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_route_${nowTick}`
    })
    const h = buildHarness()
    h.runtime.taskJobs = manager
    const post = (path: string, body?: unknown, authorized = true) =>
      dispatchRequest(
        h.router,
        new Request(`http://localhost${path}`, {
          method: 'POST',
          headers: {
            ...(authorized ? { authorization: 'Bearer tok-1' } : {}),
            'content-type': 'application/json'
          },
          body: body === undefined ? undefined : JSON.stringify(body)
        })
      )

    const unauthorizedBodies = {
      wait: { jobIds: ['missing'] },
      output: { jobId: 'missing' },
      kill: { jobId: 'missing', reason: 'auth matrix fixture' }
    }
    for (const route of contract.routeContract.authMatrix.protectedRoutes) {
      const unauthorized = await post(TASK_JOB_ROUTE_CONTRACT[route], unauthorizedBodies[route], false)
      expect(unauthorized.status).toBe(contract.routeContract.authMatrix.unauthorizedStatus)
      expect(unauthorized.status).toBe(contract.routeExecutable.unauthorizedStatus)
    }

    const release = deferred<void>()
    const background = await manager.startBackground({
      kind: 'task',
      parentThreadId: 'thr_route',
      parentTurnId: 'turn_route'
    }, async ({ appendOutput }) => {
      await appendOutput(`route partial\n${PRIVATE_OUTPUT_SENTINEL}\n`)
      await release.promise
      return 'route complete'
    })

    await waitFor(async () => {
      const record = await store.load(background.id)
      return record?.output.map((chunk) => chunk.text).join('').includes(PRIVATE_OUTPUT_SENTINEL) === true
    })
    const output = await post(TASK_JOB_ROUTE_CONTRACT.output, { jobId: background.id, threadId: 'thr_route' })
    expect(output.status).toBe(contract.routeExecutable.output.status)
    const outputBody = await readJson(output)
    expect(outputBody).toEqual(withheldTaskJobOutput(background.id, contract.routeExecutable.output.jobStatus))
    expect(JSON.stringify(outputBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    const crossThreadOutput = await post(TASK_JOB_ROUTE_CONTRACT.output, {
      jobId: background.id,
      threadId: 'thr_other'
    })
    expect(crossThreadOutput.status).toBe(403)
    expect(await readJson(crossThreadOutput)).toMatchObject({
      code: 'forbidden'
    })
    const replay = await post(TASK_JOB_ROUTE_CONTRACT.output, {
      jobId: background.id,
      threadId: 'thr_route',
      offset: contract.routeExecutable.output.replayOffset
    })
    const replayBody = await readJson(replay)
    expect(replayBody).toEqual(withheldTaskJobOutput(background.id, contract.routeExecutable.output.jobStatus))
    expect(JSON.stringify(replayBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    const limited = await post(TASK_JOB_ROUTE_CONTRACT.output, {
      jobId: background.id,
      threadId: 'thr_route',
      offset: Buffer.byteLength('route ', 'utf8'),
      limit: Buffer.byteLength('partial', 'utf8')
    })
    const limitedBody = await readJson(limited)
    expect(limitedBody).toEqual(withheldTaskJobOutput(background.id, contract.routeExecutable.output.jobStatus))
    expect(JSON.stringify(limitedBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    const limitedResume = await post(TASK_JOB_ROUTE_CONTRACT.output, {
      jobId: background.id,
      threadId: 'thr_route',
      offset: Buffer.byteLength('route partial', 'utf8')
    })
    const limitedResumeBody = await readJson(limitedResume)
    expect(limitedResumeBody).toEqual(withheldTaskJobOutput(background.id, contract.routeExecutable.output.jobStatus))
    expect(JSON.stringify(limitedResumeBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)

    const waitResponse = post(TASK_JOB_ROUTE_CONTRACT.wait, {
      jobIds: [background.id],
      threadId: 'thr_route',
      timeoutMs: 500
    })
    const crossThreadWait = await post(TASK_JOB_ROUTE_CONTRACT.wait, {
      jobIds: [background.id],
      threadId: 'thr_other'
    })
    expect(crossThreadWait.status).toBe(403)
    expect(await readJson(crossThreadWait)).toMatchObject({
      code: 'forbidden'
    })
    release.resolve()
    const waitBody = await readJson(await waitResponse)
    expect(waitBody).toEqual({
      jobs: [withheldTaskJobSummary(background.id, 'task', contract.routeExecutable.wait.jobStatus, false)]
    })
    expect(JSON.stringify(waitBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)

    const stalled = await manager.startBackground({
      kind: 'task',
      parentThreadId: 'thr_route',
      parentTurnId: 'turn_kill'
    }, async ({ signal }) => {
      await new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new Error(contract.routeExecutable.kill.error)), { once: true })
      })
    })
    const killed = await post(TASK_JOB_ROUTE_CONTRACT.kill, {
      jobId: stalled.id,
      threadId: 'thr_route',
      reason: contract.routeExecutable.kill.error
    })
    expect(killed.status).toBe(contract.routeExecutable.kill.status)
    const killedBody = await readJson(killed)
    expect(killedBody).toEqual({
      job: withheldTaskJobSummary(stalled.id, 'task', contract.routeExecutable.kill.jobStatus, false)
    })
    expect(JSON.stringify(killedBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    const crossThreadKill = await post(TASK_JOB_ROUTE_CONTRACT.kill, {
      jobId: stalled.id,
      threadId: 'thr_other',
      reason: 'cross-thread kill'
    })
    expect(crossThreadKill.status).toBe(403)
    expect(await readJson(crossThreadKill)).toMatchObject({
      code: 'forbidden'
    })

    const missingThreadScope = await post(TASK_JOB_ROUTE_CONTRACT.output, { jobId: background.id })
    expect(missingThreadScope.status).toBe(400)
    expect(await readJson(missingThreadScope)).toMatchObject({
      code: 'validation_error'
    })
    const missing = await post(TASK_JOB_ROUTE_CONTRACT.output, { jobId: 'missing', threadId: 'thr_route' })
    expect(missing.status).toBe(contract.routeExecutable.missingOutput.status)
  })

  it('executes internal task and parallel_tasks tools through durable child jobs', async () => {
    const contract = loadContract()
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:02:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_tool_${nowTick}`
    })
    const seenEfforts: Array<string | undefined> = []
    const seenToolScopes: Array<string[] | undefined> = []
    const runtime = createDelegationRuntime(dir, { seenEfforts, seenToolScopes })
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })
    const context = toolContext()
    const names = (await host.listTools(context)).map((tool) => tool.name)
    expect(names).toEqual(expect.arrayContaining([
      TASK_TOOL_CONTRACT.name,
      PARALLEL_TASKS_TOOL_CONTRACT.name,
      TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
      TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      TASK_JOB_CONTROL_TOOL_CONTRACT.kill
    ]))

    const foreground = toolOutput(await host.execute({
      callId: 'call_task',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'foreground sample', label: 'foreground', effort: 'low', tools: ['grep', 'read', 'grep'] }
    }, context))
    expect(foreground).toMatchObject({
      jobId: expect.any(String),
      status: 'completed',
      outputWithheld: true,
      canReadOutput: false,
      canContinueParent: false
    })
    expect(foreground).not.toHaveProperty('result')
    expect(foreground).not.toHaveProperty('output')

    const background = toolOutput(await host.execute({
      callId: 'call_background',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'background sample', run_in_background: true, effort: 'high' }
    }, context))
    expect(background).toMatchObject({
      status: 'running',
      routes: TASK_JOB_ROUTE_CONTRACT
    })
    const backgroundJobId = String(background.jobId)
    await waitFor(async () => (await manager.wait([backgroundJobId]))[0]?.status === 'completed')
    expect(await manager.output(backgroundJobId, { offset: 0 })).toMatchObject({
      id: backgroundJobId,
      status: 'completed',
      output: 'child sample: background sample\n',
      nextOffset: Buffer.byteLength('child sample: background sample\n', 'utf8')
    })
    const waited = toolOutput(await host.execute({
      callId: 'call_wait',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
      arguments: { job_id: backgroundJobId, timeout_ms: 1000 }
    }, context))
    expect(waited.jobs).toEqual([
      expect.objectContaining({ jobId: backgroundJobId, status: 'completed', outputWithheld: true })
    ])
    const partialOutput = toolOutput(await host.execute({
      callId: 'call_bash_output_partial',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      arguments: { job_id: backgroundJobId, offset: 0, limit: 13 }
    }, context))
    expect(partialOutput).toEqual(withheldTaskJobOutput(backgroundJobId, 'completed'))
    const restOutput = toolOutput(await host.execute({
      callId: 'call_bash_output_rest',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      arguments: { job_id: backgroundJobId, offset: 13 }
    }, context))
    expect(restOutput).toEqual(withheldTaskJobOutput(backgroundJobId, 'completed'))

    const killGate = deferred<void>()
    const killable = await manager.startBackground({
      kind: 'task',
      parentThreadId: context.threadId,
      parentTurnId: context.turnId,
      label: 'killable'
    }, async ({ signal, appendOutput }) => {
      await appendOutput('running until killed\n')
      if (signal.aborted) return
      await new Promise<void>((resolve) => {
        signal.addEventListener('abort', () => resolve(), { once: true })
        killGate.promise.then(resolve)
      })
    })
    const killed = toolOutput(await host.execute({
      callId: 'call_kill_shell',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.kill,
      arguments: { job_id: killable.id, reason: 'test kill' }
    }, context))
    expect(killed.job).toEqual(expect.objectContaining({
      jobId: killable.id,
      status: 'killed',
      outputWithheld: true
    }))
    killGate.resolve()
    const forbidden = await host.execute({
      callId: 'call_bash_output_forbidden',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      arguments: { job_id: backgroundJobId }
    }, { ...context, threadId: 'thr_other' })
    expect(forbidden.item).toMatchObject({
      kind: 'tool_result',
      isError: true,
      output: {
        code: 'forbidden'
      }
    })

    const parallel = toolOutput(await host.execute({
      callId: 'call_parallel',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      arguments: {
        tasks: [
          { id: 'a', prompt: 'parallel a', effort: 'medium', tools: ['ls'] },
          { id: 'b', prompt: 'parallel b', depends_on: ['a'] },
          { id: 'c', prompt: 'parallel c', depends_on: ['a'] }
        ]
      }
    }, context))
    expect(parallel.order).toEqual(['a', 'b', 'c'])
    expect(parallel.jobs).toEqual([
      expect.objectContaining({ status: 'completed', outputWithheld: true }),
      expect.objectContaining({ status: 'completed', outputWithheld: true }),
      expect.objectContaining({ status: 'completed', outputWithheld: true })
    ])
    expect(JSON.stringify(parallel.jobs)).not.toContain('child sample: parallel a')
    expect(seenEfforts).toEqual(['low', 'high', 'medium', undefined, undefined])
    expect(seenToolScopes).toEqual([['grep', 'read'], undefined, ['ls'], undefined, undefined])

    const failingRuntime = createDelegationRuntime(dir, { failPromptIncludes: 'parallel b' })
    const failingHost = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: failingRuntime
      }))
    })
    const failedResult = await failingHost.execute({
      callId: 'call_parallel_fail',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      arguments: {
        tasks: [
          { id: 'a', prompt: 'parallel a' },
          { id: 'b', prompt: 'parallel b', depends_on: ['a'] },
          { id: 'c', prompt: 'parallel c', depends_on: ['b'] }
        ]
      }
    }, context)
    expect(failedResult.item).toMatchObject({ kind: 'tool_result', isError: true })
    const failedOutput = failedResult.item.kind === 'tool_result'
      ? failedResult.item.output as Record<string, unknown>
      : {}
    expect(failedOutput.skipped).toEqual([{ id: 'c', reason: contract.plannerExecutor.skippedReason }])
  })

  it('supports filtered bash_output and generated parallel task ids in the legacy TypeScript tool host', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:02:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_filter_${nowTick}`
    })
    const runtime = createDelegationRuntime(dir)
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })
    const context = toolContext()

    const background = await manager.startBackground({
      kind: 'task',
      parentThreadId: context.threadId,
      parentTurnId: context.turnId,
      label: 'filtered-output'
    }, async ({ appendOutput }) => {
      await appendOutput('keep alpha\n')
      await appendOutput('drop beta\n')
      await appendOutput('keep gamma\n')
      return 'done'
    })
    await waitFor(async () => (await manager.wait([background.id]))[0]?.status === 'completed')

    const filtered = toolOutput(await host.execute({
      callId: 'call_bash_output_filter',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      arguments: { job_id: background.id, offset: 0, filter: '^keep' }
    }, context))
    expect(filtered).toEqual(withheldTaskJobOutput(background.id, 'completed'))

    const invalidFilter = toolOutput(await host.execute({
      callId: 'call_bash_output_invalid_filter',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      arguments: { job_id: background.id, offset: 0, filter: '[' }
    }, context))
    expect(invalidFilter).toEqual(withheldTaskJobOutput(background.id, 'completed'))

    const parallel = toolOutput(await host.execute({
      callId: 'call_parallel_generated_ids',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      arguments: {
        tasks: [
          { prompt: 'auto parallel left' },
          { prompt: 'auto parallel right' }
        ]
      }
    }, context))
    expect(parallel.order).toEqual(['task_1', 'task_2'])
    expect(parallel.jobs).toEqual([
      expect.objectContaining({ status: 'completed', outputWithheld: true }),
      expect.objectContaining({ status: 'completed', outputWithheld: true })
    ])
  })

  it('continues and forks TypeScript child runs through durable transcript references', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:02:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_transcript_${nowTick}`
    })
    const runtime = createDelegationRuntime(dir)
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })

    const source = toolOutput(await host.execute({
      callId: 'call_task_source',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'source work' }
    }, toolContext()))
    const [sourceChild] = (await runtime.diagnostics('thr_tool')).childRuns
    expect(sourceChild).toMatchObject({ status: 'completed', summary: 'child sample: source work' })
    expect(source).toMatchObject({ status: 'completed', outputWithheld: true, canReadOutput: false })
    expect(source).not.toHaveProperty('childRunId')
    expect(sourceChild.transcriptItems).toEqual([
      expect.objectContaining({ kind: 'user_message', text: 'source work' }),
      expect.objectContaining({ kind: 'assistant_text', text: 'child sample: source work' })
    ])

    const continued = await host.execute({
      callId: 'call_task_continue',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'continue previous work', continue_from: sourceChild.id }
    }, toolContext())
    const forked = await host.execute({
      callId: 'call_task_fork',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'fork previous work', fork_from: sourceChild.id }
    }, toolContext())
    const invalid = await host.execute({
      callId: 'call_task_invalid_transcript',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'invalid transcript request', continue_from: 'sa_a', fork_from: 'sa_b' }
    }, toolContext())

    const continuedOutput = toolOutput(continued)
    const forkedOutput = toolOutput(forked)
    expect(source).toMatchObject({ status: 'completed', outputWithheld: true })
    expect(continuedOutput).toMatchObject({
      status: 'completed',
      outputWithheld: true,
      canReadOutput: false
    })
    expect(forkedOutput).toMatchObject({
      status: 'completed',
      outputWithheld: true,
      canReadOutput: false
    })
    expect(continuedOutput).not.toHaveProperty('result')
    expect(forkedOutput).not.toHaveProperty('result')
    expect(invalid.item).toMatchObject({
      kind: 'tool_result',
      isError: true,
      output: {
        code: 'subagent_transcript_invalid_request'
      }
    })
    const jobs = await manager.list('thr_tool')
    expect(jobs).toHaveLength(3)
    const continuedJob = jobs.find((job) => job.transcript?.mode === 'continue')
    const forkedJob = jobs.find((job) => job.transcript?.mode === 'fork')
    expect(continuedJob?.transcript).toMatchObject({ sourceId: sourceChild.id, targetId: sourceChild.id })
    expect(forkedJob?.transcript?.sourceId).toBe(sourceChild.id)
    expect(forkedJob?.transcript?.targetId).not.toBe(sourceChild.id)
    const childRuns = (await runtime.diagnostics('thr_tool')).childRuns
    expect(childRuns).toHaveLength(2)
    expect(childRuns).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: sourceChild.id,
        summary: expect.stringContaining('continue previous work')
      }),
      expect.objectContaining({
        id: forkedJob?.transcript?.targetId
      })
    ]))
  })

  it('pins task job tool schemas on the registry surface without leaking upstream protocol names', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({ store })
    const runtime = createDelegationRuntime(dir)
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })

    const tools = await host.listTools(toolContext())
    const byName = new Map(tools.map((tool) => [tool.name, tool]))

    expect([...byName.keys()]).toEqual([
      TASK_TOOL_CONTRACT.name,
      PARALLEL_TASKS_TOOL_CONTRACT.name,
      TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
      TASK_JOB_CONTROL_TOOL_CONTRACT.output,
      TASK_JOB_CONTROL_TOOL_CONTRACT.kill
    ])
    expect([...byName.keys()].join(' ')).not.toMatch(/reasonix|sessionapi|kun/i)
    expect(tools.every((tool) => tool.providerId === 'task-jobs')).toBe(true)
    expect(tools.every((tool) => tool.providerKind === 'delegation')).toBe(true)
    expect(byName.get(TASK_TOOL_CONTRACT.name)?.toolKind).toBe('subagent')
    expect(byName.get(PARALLEL_TASKS_TOOL_CONTRACT.name)?.toolKind).toBe('subagent')
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.wait)?.toolKind).toBe('subagent')
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.output)?.toolKind).toBe('subagent')
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.kill)?.toolKind).toBe('tool_call')

    expect(byName.get(TASK_TOOL_CONTRACT.name)?.inputSchema).toMatchObject({
      type: 'object',
      required: [TASK_TOOL_CONTRACT.promptField],
      additionalProperties: false,
      properties: {
        prompt: { type: 'string' },
        run_in_background: { type: 'boolean' },
        continue_from: { type: 'string' },
        fork_from: { type: 'string' },
        profile: { type: 'string' }
      }
    })
    expect(byName.get(PARALLEL_TASKS_TOOL_CONTRACT.name)?.inputSchema).toMatchObject({
      type: 'object',
      required: [PARALLEL_TASKS_TOOL_CONTRACT.tasksField],
      additionalProperties: false,
      properties: {
        tasks: {
          type: 'array',
          items: {
            required: ['prompt'],
            additionalProperties: false,
            properties: {
              depends_on: { type: 'array', items: { type: 'string' } },
              prompt: { type: 'string' },
              profile: { type: 'string' }
            }
          }
        }
      }
    })
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.wait)?.inputSchema).toMatchObject({
      type: 'object',
      additionalProperties: false,
      properties: {
        job_id: { type: 'string' },
        job_ids: { type: 'array', items: { type: 'string' } },
        timeout_ms: { type: 'integer', minimum: 0, maximum: 60000 }
      }
    })
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.output)?.inputSchema).toMatchObject({
      type: 'object',
      additionalProperties: false,
      properties: {
        job_id: { type: 'string' },
        offset: { type: 'integer', minimum: 0 },
        limit: { type: 'integer', minimum: 0 },
        filter: { type: 'string' }
      }
    })
    expect(byName.get(TASK_JOB_CONTROL_TOOL_CONTRACT.kill)?.inputSchema).toMatchObject({
      type: 'object',
      additionalProperties: false,
      properties: {
        job_id: { type: 'string' },
        reason: { type: 'string' }
      }
    })
  })

  it('does not create durable child jobs when task tool approvals are denied', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:02:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_deny_${nowTick}`
    })
    const runtime = createDelegationRuntime(dir)
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })
    const deniedContext: ToolHostContext = {
      ...toolContext(),
      awaitApproval: async () => 'deny'
    }

    const taskDenied = await host.execute({
      callId: 'call_task_denied',
      toolName: TASK_TOOL_CONTRACT.name,
      arguments: { prompt: 'must not start child work' }
    }, deniedContext)
    const parallelDenied = await host.execute({
      callId: 'call_parallel_denied',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      arguments: {
        tasks: [
          { id: 'a', prompt: 'must not start child work a' },
          { id: 'b', prompt: 'must not start child work b' }
        ]
      }
    }, deniedContext)

    expect(taskDenied.approved).toBe(false)
    expect(taskDenied.item).toMatchObject({
      kind: 'approval',
      toolName: TASK_TOOL_CONTRACT.name,
      approvalId: 'appr_call_task_denied'
    })
    expect(parallelDenied.approved).toBe(false)
    expect(parallelDenied.item).toMatchObject({
      kind: 'approval',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      approvalId: 'appr_call_parallel_denied'
    })
    expect(await manager.list('thr_tool')).toEqual([])
    expect((await runtime.diagnostics('thr_tool')).childRuns).toEqual([])
  })

  it('waits for running jobs in the current thread when wait omits job ids', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:02:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_wait_all_${nowTick}`
    })
    const release = deferred<void>()
    const context = toolContext()
    const runtime = createDelegationRuntime(dir)
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })
    const background = await manager.startBackground({
      kind: 'task',
      parentThreadId: context.threadId,
      parentTurnId: context.turnId,
      label: 'wait-all'
    }, async ({ appendOutput }) => {
      await appendOutput('running\n')
      await release.promise
      return 'done'
    })
    await waitFor(async () =>
      (await manager.list(context.threadId)).some((job) =>
        job.id === background.id && job.status === 'running'
      )
    )

    const waited = host.execute({
      callId: 'call_wait_all',
      toolName: TASK_JOB_CONTROL_TOOL_CONTRACT.wait,
      arguments: {}
    }, context)
    await new Promise((resolve) => setTimeout(resolve, 10))
    release.resolve()
    const output = toolOutput(await waited)

    expect(output.jobs).toEqual([
      expect.objectContaining({
        jobId: background.id,
        status: 'completed',
        outputWithheld: true
      })
    ])
  })

  it('preserves completed, killed, and skipped results when parallel_tasks is cancelled by its parent', async () => {
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:06:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_cancel_${nowTick}`
    })
    const parent = new AbortController()
    let childSeq = 0
    const config = AnalytixCapabilitiesConfig.parse({
      subagents: {
        enabled: true,
        maxParallel: 2,
        maxChildRuns: 20
      }
    }).subagents
    const runtime = new DelegationRuntime({
      config,
      store: new FileDelegationStore(join(dir, 'cancel-child-runs')),
      nowIso: () => `2026-06-03T00:07:${String(childSeq).padStart(2, '0')}.000Z`,
      idGenerator: () => `child_cancel_${++childSeq}`,
      executor: async ({ prompt }) => {
        if (prompt.includes('cancel parent')) {
          parent.abort('parent cancelled parallel tasks')
          throw new Error('child observed parent cancel')
        }
        return {
          summary: `child sample: ${prompt}`,
          usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 }
        }
      }
    })
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildTaskJobToolProviders({
        taskJobs: manager,
        delegationRuntime: runtime
      }))
    })

    const result = await host.execute({
      callId: 'call_parallel_cancel',
      toolName: PARALLEL_TASKS_TOOL_CONTRACT.name,
      arguments: {
        tasks: [
          { id: 'a', prompt: 'completed before cancel' },
          { id: 'b', prompt: 'cancel parent', depends_on: ['a'] },
          { id: 'c', prompt: 'not launched c', depends_on: ['b'] },
          { id: 'd', prompt: 'not launched d', depends_on: ['b'] }
        ]
      }
    }, {
      ...toolContext(),
      abortSignal: parent.signal
    })

    expect(result.item).toMatchObject({ kind: 'tool_result', isError: true })
    const output = result.item.kind === 'tool_result'
      ? result.item.output as Record<string, unknown>
      : {}
    expect(output.order).toEqual(['a', 'b', 'c', 'd'])
    expect(output.jobs).toEqual([
      expect.objectContaining({ status: 'completed', outputWithheld: true }),
      expect.objectContaining({ status: 'killed' })
    ])
    expect(output.skipped).toEqual([
      { id: 'c', reason: 'cancelled: parent turn aborted before task execution' },
      { id: 'd', reason: 'cancelled: parent turn aborted before task execution' }
    ])
  })

  it('reconciles orphaned queued or running jobs to an explicit interrupted state on restart', async () => {
    const contract = loadContract()
    const stale = contract.durableRunner.staleReconcile
    const store = new FileTaskJobStore(dir)
    await store.upsert(TaskJobRecordSchema.parse({
      id: stale.runningJobId,
      kind: 'task',
      parentThreadId: 'thr_orphan',
      parentTurnId: 'turn_orphan',
      status: 'running',
      createdAt: '2026-06-03T00:03:00.000Z',
      startedAt: '2026-06-03T00:03:00.000Z',
      updatedAt: '2026-06-03T00:03:00.000Z'
    }))
    await store.upsert(TaskJobRecordSchema.parse({
      id: stale.queuedJobId,
      kind: 'task',
      parentThreadId: 'thr_orphan',
      parentTurnId: 'turn_orphan',
      status: 'queued',
      createdAt: '2026-06-03T00:03:01.000Z',
      updatedAt: '2026-06-03T00:03:01.000Z'
    }))

    const restarted = new DurableTaskJobManager({
      store,
      nowIso: () => '2026-06-03T00:04:00.000Z'
    })
    const reconciled = await restarted.reconcileRunningJobs(stale.reason)
    expect(reconciled).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: stale.runningJobId,
        status: stale.expectedStatus,
        error: stale.reason
      }),
      expect.objectContaining({
        id: stale.queuedJobId,
        status: stale.expectedStatus,
        error: stale.reason
      })
    ]))
    expect(reconciled).toHaveLength(stale.reconciledCount)
    for (const jobId of [stale.runningJobId, stale.queuedJobId]) {
      expect(await store.load(jobId)).toMatchObject({
        status: stale.expectedStatus,
        error: stale.reason
      })
    }
  })

  it('coordinates planner-readonly executor waves with failure propagation and cancellation', async () => {
    const contract = loadContract()
    const store = new FileTaskJobStore(dir)
    const manager = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:06:${String(nowTick++).padStart(2, '0')}.000Z`,
      idGenerator: (kind) => `${kind}_coord_${nowTick}`
    })
    const transcriptIdentity = {
      model: 'deepseek-v4-pro',
      effort: 'high',
      workspace: '/tmp/analytix',
      toolNames: contract.permissions.plannerReadOnlyToolset
    }
    const executorTranscript = resolveTranscriptOperation({
      mode: contract.plannerExecutor.transcriptPropagationMode,
      sourceId: contract.transcript.sourceId,
      source: transcriptIdentity,
      requested: transcriptIdentity,
      newId: contract.transcript.forkTargetId
    })

    const failed = await runPlannerExecutorCoordinator({
      taskJobs: manager,
      parentThreadId: 'thr_coord',
      parentTurnId: 'turn_coord',
      parentCallId: contract.nestedEvent.parentCallId,
      childRunId: contract.nestedEvent.childRunId,
      permissionPolicy: contract.plannerExecutor.executorPolicy,
      plan: [
        { id: 'a', prompt: 'first' },
        { id: 'b', prompt: 'second', depends_on: ['a'] },
        { id: 'c', prompt: 'third', depends_on: ['b'] }
      ],
      transcriptFor: () => executorTranscript,
      runnerFor: (task) => async ({ appendOutput }) => {
        await appendOutput(`${task.id} output\n`)
        if (task.id === 'b') throw new Error('b failed')
        return `${task.id} done`
      }
    })

    expect(failed.status).toBe(contract.plannerExecutor.failureStatus)
    expect(failed.plannerJob).toMatchObject({
      kind: contract.plannerExecutor.plannerKind,
      permissionPolicy: contract.plannerExecutor.plannerPolicy
    })
    expect(failed.order).toEqual(contract.parallel.validOrder)
    expect(failed.jobs).toHaveLength(contract.plannerExecutor.outputOffsetJobCount)
    expect(failed.jobs.map((job) => job.label)).toEqual(['a', 'b'])
    expect(failed.jobs[0]).toMatchObject({
      parentCallId: contract.nestedEvent.parentCallId,
      childRunId: contract.nestedEvent.childRunId
    })
    expect(failed.skipped).toEqual([{ id: 'c', reason: contract.plannerExecutor.skippedReason }])
    expect(Object.values(failed.outputOffsets)).toEqual([
      Buffer.byteLength('a output\n', 'utf8'),
      Buffer.byteLength('b output\n', 'utf8')
    ])
    expect(failed.plannerReadOnlyToolset).toEqual(contract.permissions.plannerReadOnlyToolset)
    expect(failed.jobs.filter((job) =>
      job.transcript?.mode === contract.plannerExecutor.transcriptPropagationMode
    )).toHaveLength(contract.plannerExecutor.transcriptPropagationJobCount)
    expect(failed.jobs.map((job) => job.transcript)).toEqual([
      executorTranscript,
      executorTranscript
    ])

    const abortController = new AbortController()
    const cancelledPromise = runPlannerExecutorCoordinator({
      taskJobs: manager,
      parentThreadId: 'thr_cancel',
      parentTurnId: 'turn_cancel',
      signal: abortController.signal,
      plan: [
        { id: 'a', prompt: 'first' },
        { id: 'b', prompt: 'second' }
      ],
      runnerFor: (task) => async ({ signal, appendOutput }) => {
        await appendOutput(`${task.id} started\n`)
        abortController.abort()
        await new Promise((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(new Error(contract.plannerExecutor.cancelReason)), { once: true })
        })
      }
    })

    const cancelled = await cancelledPromise
    expect(cancelled.status).toBe(contract.plannerExecutor.cancelledStatus)
    expect(cancelled.jobs.every((job) => job.status === contract.waitOutputKill.killedStatus)).toBe(true)
    expect(cancelled.jobs.every((job) => job.error === contract.plannerExecutor.cancelReason)).toBe(true)
  })

  it('rehydrates restart-visible jobs so they can still output, wait, and kill', async () => {
    const contract = loadContract()
    const drill = contract.durableRunner.restartDrill
    const store = new FileTaskJobStore(dir)
    const outputBeforeRestart = `${drill.outputBeforeRestart}${PRIVATE_OUTPUT_SENTINEL}\n`
    await store.upsert(TaskJobRecordSchema.parse({
      id: drill.runningJobId,
      kind: 'task',
      parentThreadId: 'thr_rehydrate',
      parentTurnId: 'turn_rehydrate',
      status: 'running',
      dependencies: [],
      output: [{ offset: 0, text: outputBeforeRestart, at: '2026-06-03T00:07:00.000Z' }],
      createdAt: '2026-06-03T00:07:00.000Z',
      startedAt: '2026-06-03T00:07:00.000Z',
      updatedAt: '2026-06-03T00:07:00.000Z'
    }))
    await store.upsert(TaskJobRecordSchema.parse({
      id: drill.queuedJobId,
      kind: 'task',
      parentThreadId: 'thr_rehydrate',
      parentTurnId: 'turn_rehydrate',
      status: 'queued',
      dependencies: [],
      output: [],
      createdAt: '2026-06-03T00:07:01.000Z',
      updatedAt: '2026-06-03T00:07:01.000Z'
    }))

    const restarted = new DurableTaskJobManager({
      store,
      nowIso: () => `2026-06-03T00:08:${String(nowTick++).padStart(2, '0')}.000Z`
    })
    const rehydrated = await restarted.rehydrateRunnableJobs({
      runnerFor: (record) => async ({ signal, appendOutput }) => {
        if (record.id === drill.runningJobId) {
          await appendOutput(drill.outputAfterRestart)
          return 'rehydrated complete'
        }
        await new Promise((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(new Error(drill.killError)), { once: true })
        })
      }
    })

    expect(rehydrated.restarted).toHaveLength(drill.rehydratedCount)
    const h = buildHarness()
    h.runtime.taskJobs = restarted
    const post = (path: string, body: unknown) =>
      dispatchRequest(
        h.router,
        new Request(`http://localhost${path}`, {
          method: 'POST',
          headers: {
            authorization: 'Bearer tok-1',
            'content-type': 'application/json'
          },
          body: JSON.stringify(body)
        })
      )
    await waitFor(async () => {
      const response = await post(TASK_JOB_ROUTE_CONTRACT.wait, {
        jobIds: [drill.runningJobId],
        threadId: 'thr_rehydrate',
        timeoutMs: 0
      })
      const body = await readJson(response) as { jobs?: Array<{ status?: string }> }
      return body.jobs?.[0]?.status === drill.waitStatus
    })
    const output = await post(TASK_JOB_ROUTE_CONTRACT.output, {
      jobId: drill.runningJobId,
      threadId: 'thr_rehydrate',
      offset: 0
    })
    expect(output.status).toBe(contract.routeExecutable.rehydrated.outputStatus)
    const outputBody = await readJson(output)
    expect(outputBody).toEqual(withheldTaskJobOutput(
      drill.runningJobId,
      contract.routeExecutable.rehydrated.completedStatus
    ))
    expect(JSON.stringify(outputBody)).not.toContain(PRIVATE_OUTPUT_SENTINEL)
    const killed = await post(TASK_JOB_ROUTE_CONTRACT.kill, {
      jobId: drill.queuedJobId,
      threadId: 'thr_rehydrate',
      reason: drill.killError
    })
    expect(killed.status).toBe(contract.routeExecutable.rehydrated.killStatus)
    const killedBody = await readJson(killed)
    expect(killedBody).toEqual({
      job: withheldTaskJobSummary(
        drill.queuedJobId,
        'task',
        contract.routeExecutable.rehydrated.killedStatus,
        false
      )
    })
    expect(JSON.stringify(killedBody)).not.toContain(drill.killError)
    const waitKilled = await post(TASK_JOB_ROUTE_CONTRACT.wait, {
      jobIds: [drill.queuedJobId],
      threadId: 'thr_rehydrate'
    })
    expect(waitKilled.status).toBe(contract.routeExecutable.rehydrated.waitStatus)
    const waitKilledBody = await readJson(waitKilled)
    expect(waitKilledBody).toEqual({
      jobs: [withheldTaskJobSummary(
        drill.queuedJobId,
        'task',
        contract.routeExecutable.rehydrated.killedStatus,
        false
      )]
    })
    expect(JSON.stringify(waitKilledBody)).not.toContain(drill.killError)
  })
})

function createDelegationRuntime(
  rootDir: string,
  options: {
    failPromptIncludes?: string
    seenEfforts?: Array<string | undefined>
    seenToolScopes?: Array<string[] | undefined>
  } = {}
): DelegationRuntime {
  let childSeq = 0
  const config = AnalytixCapabilitiesConfig.parse({
    subagents: {
      enabled: true,
      maxParallel: 2,
      maxChildRuns: 20
    }
  }).subagents
  return new DelegationRuntime({
    config,
    store: new FileDelegationStore(join(rootDir, 'child-runs')),
    nowIso: () => `2026-06-03T00:05:${String(childSeq).padStart(2, '0')}.000Z`,
    idGenerator: () => `child_tool_${++childSeq}`,
    executor: async ({ childId, prompt, effort, toolScope }) => {
      options.seenEfforts?.push(effort)
      options.seenToolScopes?.push(toolScope)
      if (options.failPromptIncludes && prompt.includes(options.failPromptIncludes)) {
        throw new Error(`child failed: ${prompt}`)
      }
      const childTurnId = `${childId}_turn`
      return {
        summary: `child sample: ${prompt}`,
        usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
        transcriptItems: [
          {
            id: `${childId}_user`,
            threadId: childId,
            turnId: childTurnId,
            role: 'user',
            status: 'completed',
            kind: 'user_message',
            text: prompt,
            createdAt: '2026-06-03T00:05:00.000Z',
            finishedAt: '2026-06-03T00:05:00.000Z'
          },
          {
            id: `${childId}_assistant`,
            threadId: childId,
            turnId: childTurnId,
            role: 'assistant',
            status: 'completed',
            kind: 'assistant_text',
            text: `child sample: ${prompt}`,
            createdAt: '2026-06-03T00:05:00.000Z',
            finishedAt: '2026-06-03T00:05:00.000Z'
          }
        ]
      }
    }
  })
}

function toolContext(): ToolHostContext {
  return {
    threadId: 'thr_tool',
    turnId: 'turn_tool',
    workspace: '/tmp/analytix',
    approvalPolicy: 'on-request',
    modelExecution: {
      providerId: 'anthropic-main',
      modelId: 'claude-3-5-sonnet',
      endpointFormat: 'messages',
      source: 'thread',
      resolvedAt: '2026-06-03T00:00:00.000Z'
    },
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow'
  }
}

function toolOutput(result: ToolHostResult): Record<string, unknown> {
  expect(result.item.kind).toBe('tool_result')
  if (result.item.kind !== 'tool_result') return {}
  expect(result.item.isError).toBe(false)
  return result.item.output as Record<string, unknown>
}
