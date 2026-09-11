import { spawn, type ChildProcessByStdio } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Readable } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { findGoBinary, goTestEnv } from './go-test-toolchain.js'
import {
  ApprovalUserInputRouteContract,
  GoG4ToolsApprovalUserInputMcpContract,
  GoMinimalAgentLoopContract,
  McpToolLifecycleContract,
  ProviderCacheContract
} from '../src/conformance/runtime-parity-fixtures.js'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..', '..')
const runtimeGoDir = join(repoRoot, 'packages', 'runtime-go')
const loopContractUrl = new URL('../src/conformance/fixtures/go-minimal-agent-loop-contract.json', import.meta.url)
const providerCacheContractUrl = new URL('../src/conformance/fixtures/provider-cache-contract.json', import.meta.url)
const g4ContractUrl = new URL('../src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-contract.json', import.meta.url)
const approvalUserInputContractUrl = new URL('../src/conformance/fixtures/approval-user-input-route-contract.json', import.meta.url)
const mcpLifecycleContractUrl = new URL('../src/conformance/fixtures/mcp-tool-lifecycle-contract.json', import.meta.url)
type GoTestChildProcess = ChildProcessByStdio<null, Readable, Readable>

type SidecarHandle = {
  url: string
  runtimeToken: string
  process: GoTestChildProcess
  stderr: () => string
}

describe('Go minimal agent loop sidecar conformance', () => {
  it('executes the fixture-only Go loop over HTTP/SSE and matches the TypeScript-owned contract', async () => {
    const contract = GoMinimalAgentLoopContract.parse(await readJson(loopContractUrl))
    const providerCache = ProviderCacheContract.parse(await readJson(providerCacheContractUrl))
    const g4 = GoG4ToolsApprovalUserInputMcpContract.parse(await readJson(g4ContractUrl))
    const approvalUserInput = ApprovalUserInputRouteContract.parse(await readJson(approvalUserInputContractUrl))
    const mcp = McpToolLifecycleContract.parse(await readJson(mcpLifecycleContractUrl))

    expect(contract.stablePrefix.prefixHash).toBe(providerCache.stablePrefix.firstShape.prefixHash)
    expect(contract.stablePrefix.systemHash).toBe(providerCache.stablePrefix.firstShape.systemHash)
    expect(contract.stablePrefix.prefixItemsHash).toBe(providerCache.stablePrefix.firstShape.prefixItemsHash)
    expect(contract.stablePrefix.toolsHash).toBe(providerCache.stablePrefix.firstShape.toolsHash)
    expect(contract.stablePrefix.toolNames).toEqual(g4.toolCatalog.advertisedToolNames)

    const requestCase = providerCache.requestShapeCases
      .find((item) => item.id === contract.modelRequestShape.caseId)
    expect(requestCase).toBeDefined()
    expect(contract.modelRequestShape).toMatchObject({
      endpointFormat: requestCase?.endpointFormat,
      baseUrl: requestCase?.baseUrl,
      model: requestCase?.model,
      expectedUrl: requestCase?.expectedUrl,
      requiredHeaders: requestCase?.requiredHeaders,
      requiredBodyFields: requestCase?.requiredBodyFields,
      forbiddenBodyFields: requestCase?.forbiddenBodyFields,
      expectedToolShape: requestCase?.expectedToolShape
    })

    const cacheCasesById = new Map(providerCache.providerUsageCases.map((item) => [item.id, item]))
    for (const cacheCase of contract.providerCacheTelemetry.cases) {
      const source = cacheCasesById.get(cacheCase.caseId)
      expect(source?.expectedUsage.cacheHitTokens).toBe(cacheCase.cacheHitTokens)
      expect(source?.expectedUsage.cacheMissTokens).toBe(cacheCase.cacheMissTokens)
      expect(source?.expectedUsage.cacheHitRate).toBe(cacheCase.cacheHitRate)
    }
    expect(contract.approvalDenied).toMatchObject({
      approvalId: approvalUserInput.approval.id,
      toolName: g4.approval.toolName,
      decision: g4.approval.decision,
      status: g4.approval.expectedStatus,
      mustNotExecuteDeniedTool: true
    })
    expect(contract.userInputGates.submittedInputId).toBe(approvalUserInput.submittedUserInput.id)
    expect(contract.userInputGates.cancelledInputId).toBe(approvalUserInput.userInput.id)
    expect(contract.mcpToolCatalog.providerId).toBe(mcp.providerId)
    expect(contract.mcpToolCatalog.toolNames).toEqual(g4.mcp.searchMetaTools.toolNames)

    const durableTempDir = await mkdtemp(join(tmpdir(), 'analytix-go-loop-'))
    const sidecar = await startGoSidecar(durableTempDir)
    try {
      const boundary = await sidecarJSON(sidecar, '/v1/conformance/loop/boundary')
      expect(boundary.fixtureOnly).toBe(true)
      expect(boundary.routePrefix).toBe('/v1/conformance/loop')
      expect(boundary.minimalAgentLoopPrototype).toBe(true)
      expect(boundary.fixtureBackedLoopOnly).toBe(true)
      expect(boundary.tempDurableEventSessionStoreEnabled).toBe(true)
      expect(boundary.publishBeforePersist).toBe(false)
      expect(boundary.persistBeforePublish).toBe(true)
      expect(boundary.providerLiveCallsAllowed).toBe(false)
      expect(boundary.toolExecutionAllowed).toBe(false)
      expect(boundary.approvalExecutionAllowed).toBe(false)
      expect(boundary.mcpConnectionAllowed).toBe(false)
      expect(boundary.defaultGoBackendEnabled).toBe(false)
      expect(boundary.rendererVisibleGoRoutesAllowed).toBe(false)
      expect(recordValue(boundary.productBoundary).tempDurableStorePrototype).toBe(true)
      expect(recordValue(boundary.productBoundary).minimalAgentLoopPrototype).toBe(true)

      const g3Accounting = await sidecarJSON(sidecar, '/v1/conformance/g3/provider/cache-accounting')
      expect(g3Accounting.deepseekCaseIds).toContain('deepseek-prompt-cache')
      expect(g3Accounting.openaiCacheCaseIds).toContain('openai-responses-cached-tokens')
      expect(g3Accounting.anthropicCacheCaseIds).toContain('anthropic-cache-fields')
      const g4Catalog = await sidecarJSON(sidecar, '/v1/conformance/g4/manager/tool-catalog')
      expect(g4Catalog.toolNames).toEqual(g4.toolCatalog.advertisedToolNames)

      const run = await sidecarJSON(sidecar, '/v1/conformance/loop/run', {
        method: 'POST',
        body: JSON.stringify({ threadId: contract.threadId, turnId: contract.turnId })
      })
      expect(run.status).toBe('completed')
      expect(run.eventKinds).toEqual(contract.expected.eventKinds)
      expect(run.itemKinds).toEqual(contract.expected.itemKinds)
      expect(run.highestSeq).toBe(contract.expected.highestSeq)
      expect(run.allPersistBeforePublish).toBe(true)
      expect(recordValue(run.stablePrefix)).toEqual(contract.stablePrefix)
      expect(recordValue(run.modelRequestShape)).toEqual(contract.modelRequestShape)
      expect(recordValue(run.providerCacheTelemetry)).toEqual(contract.providerCacheTelemetry)
      expect(recordValue(run.approvalDenied)).toEqual(contract.approvalDenied)
      expect(recordValue(run.userInputGates)).toEqual(contract.userInputGates)
      expect(recordValue(run.mcpToolCatalog)).toEqual(contract.mcpToolCatalog)
      expect(recordValue(run.stepLimit)).toEqual(contract.control.stepLimit)
      expect(recordValue(run.sideEffects)).toEqual(contract.expected.sideEffects)

      const replay = await sidecarJSON(
        sidecar,
        `/v1/conformance/loop/replay?thread_id=${contract.threadId}&after_seq=${contract.expected.replayAfterSeq}`
      )
      expect(arrayValue(replay.events)).toHaveLength(contract.expected.replayAfterSeqCount)
      expect(replay.eventKinds).toEqual(contract.expected.eventKinds.slice(contract.expected.replayAfterSeq))
      expect(replay.highestSeq).toBe(contract.expected.highestSeq)

      const frames = await sidecarSSE(
        sidecar,
        `/v1/conformance/loop/threads/${contract.threadId}/events?since_seq=0`
      )
      expect(frames).toHaveLength(contract.expected.sseFrameCount)
      expect(frames[0]).toContain('event: turn_started')
      expect(frames.at(-1)).toContain('event: turn_completed')
      const caughtUp = await sidecarSSE(
        sidecar,
        `/v1/conformance/loop/threads/${contract.threadId}/events?since_seq=${contract.expected.highestSeq}`
      )
      expect(caughtUp).toHaveLength(contract.expected.caughtUpReplayFrameCount)

      const recovered = await sidecarJSON(
        sidecar,
        `/v1/conformance/loop/recovered-state?thread_id=${contract.threadId}`
      )
      expect(recovered.eventKinds).toEqual(contract.expected.eventKinds)
      const usage = recordValue(recovered.usageCacheAccounting)
      expect(usage.providers).toEqual(contract.expected.providers)
      expect(usage.totalCacheHitTokens).toBe(contract.expected.totalCacheHitTokens)
      expect(usage.totalCacheMissTokens).toBe(contract.expected.totalCacheMissTokens)
      expect(usage.cacheTelemetryLost).toBe(false)
      expect(recordValue(recovered.approvals).statuses).toEqual(contract.expected.approvalStatuses)
      expect(recordValue(recovered.approvals).approvalExecutionUsed).toBe(false)
      expect(recordValue(recovered.userInputs).statuses).toEqual(contract.expected.userInputStatuses)
      expect(recordValue(recovered.userInputs).answersPersisted).toBe(false)
      expect(recordValue(recovered.mcp).toolNames).toEqual(contract.expected.mcpToolNames)
      expect(recordValue(recovered.mcp).latestFingerprint).toBe(contract.expected.latestMcpFingerprint)
      expect(recordValue(recovered.mcp).mcpConnectionUsed).toBe(false)

      const cancel = await sidecarJSON(sidecar, '/v1/conformance/loop/cancel', {
        method: 'POST',
        body: JSON.stringify({ threadId: contract.cancelThreadId })
      })
      expect(cancel.status).toBe('aborted')
      expect(cancel.resultCode).toBe('tool_call_cancelled')
      expect(cancel.eventKinds).toEqual(contract.control.cancel.eventDrafts.map((event) => event.kind))
      expect(cancel.allPersistBeforePublish).toBe(true)
      expect(recordValue(cancel.sideEffects)).toEqual(contract.expected.sideEffects)

      const resume = await sidecarJSON(sidecar, '/v1/conformance/loop/resume', {
        method: 'POST',
        body: JSON.stringify({ sourceThreadId: contract.threadId })
      })
      expect(resume.sourceThreadId).toBe(contract.threadId)
      expect(resume.resumedThreadId).toBe(contract.resumeThreadId)
      expect(resume.sourceMatchesExpected).toBe(true)
      expect(resume.resumeEventKinds).toEqual(contract.control.resume.eventDrafts.map((event) => event.kind))
      expect(resume.allPersistBeforePublish).toBe(true)
      expect(recordValue(resume.sideEffects)).toEqual(contract.expected.sideEffects)

      const durableSnapshot = await sidecarJSON(sidecar, '/v1/conformance/durable/snapshot')
      expect(Number(durableSnapshot.writeAttempts)).toBeGreaterThanOrEqual(
        contract.expected.highestSeq + contract.control.cancel.eventDrafts.length + contract.control.resume.eventDrafts.length
      )
      expect(durableSnapshot.realWorkspaceWriteAllowed).toBe(false)
      expect(durableSnapshot.credentialReadAllowed).toBe(false)
      expect(durableSnapshot.defaultGoBackendEnabled).toBe(false)
    } finally {
      await stopSidecar(sidecar)
      await rm(durableTempDir, { recursive: true, force: true })
    }
  }, 120_000)
})

async function readJson(url: URL): Promise<unknown> {
  return JSON.parse(await readFile(url, 'utf8'))
}

async function startGoSidecar(durableTempDir: string): Promise<SidecarHandle> {
  const go = await findGoBinary()
  const child = spawn(go, [
    'run',
    './cmd/contract-sidecar',
    '--addr',
    '127.0.0.1:0',
    '--fixtures-dir',
    '../runtime/src/conformance/fixtures',
    '--durable-temp-dir',
    durableTempDir
  ], {
    cwd: runtimeGoDir,
    env: goTestEnv(),
    stdio: ['ignore', 'pipe', 'pipe']
  })
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  let stderr = ''
  child.stderr.on('data', (chunk) => {
    stderr += String(chunk)
  })
  const ready = await waitForReadyLine(child, () => stderr)
  return {
    url: ready.url,
    runtimeToken: ready.runtimeToken,
    process: child,
    stderr: () => stderr
  }
}

async function stopSidecar(sidecar: SidecarHandle): Promise<void> {
  if (sidecar.process.exitCode !== null) return
  sidecar.process.kill('SIGTERM')
  await new Promise<void>((resolve) => {
    const timeout = setTimeout(resolve, 2_000)
    sidecar.process.once('exit', () => {
      clearTimeout(timeout)
      resolve()
    })
  })
}

async function waitForReadyLine(
  child: GoTestChildProcess,
  stderr: () => string
): Promise<{ url: string; runtimeToken: string }> {
  return new Promise((resolve, reject) => {
    let stdout = ''
    const timer = setTimeout(() => {
      reject(new Error(`Go sidecar did not become ready. stderr:\n${stderr()}`))
    }, 60_000)
    child.once('exit', (code) => {
      clearTimeout(timer)
      reject(new Error(`Go sidecar exited before ready with code ${code}. stderr:\n${stderr()}`))
    })
    child.stdout.on('data', (chunk) => {
      stdout += String(chunk)
      const line = stdout.split('\n').find((item) => item.startsWith('ANALYTIX_SIDECAR_READY '))
      if (!line) return
      clearTimeout(timer)
      const parsed = JSON.parse(line.slice('ANALYTIX_SIDECAR_READY '.length)) as {
        url: string
        runtimeToken: string
      }
      resolve(parsed)
    })
  })
}

async function sidecarJSON(
  sidecar: SidecarHandle,
  path: string,
  options: {
    method?: string
    body?: string
    expectedStatus?: number
  } = {}
): Promise<Record<string, unknown>> {
  const response = await fetch(`${sidecar.url}${path}`, {
    method: options.method ?? 'GET',
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`,
      ...(options.body ? { 'content-type': 'application/json' } : {})
    },
    body: options.body
  })
  expect(response.status).toBe(options.expectedStatus ?? 200)
  return recordValue(await response.json())
}

async function sidecarSSE(sidecar: SidecarHandle, path: string): Promise<string[]> {
  const response = await fetch(`${sidecar.url}${path}`, {
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`
    }
  })
  expect(response.status).toBe(200)
  expect(response.headers.get('content-type') ?? '').toContain('text/event-stream')
  const body = await response.text()
  return body.trim() ? body.trim().split('\n\n') : []
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}
