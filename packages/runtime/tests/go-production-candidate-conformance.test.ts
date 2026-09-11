import { spawn, type ChildProcessByStdio } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Readable } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { findGoBinary, goTestEnv } from './go-test-toolchain.js'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..', '..')
const runtimeGoDir = join(repoRoot, 'packages', 'runtime-go')
const productionPrefix = '/v1/internal/go-production-candidate'
type GoTestChildProcess = ChildProcessByStdio<null, Readable, Readable>

type SidecarHandle = {
  url: string
  runtimeToken: string
  process: GoTestChildProcess
  stderr: () => string
}

describe('Go production-candidate runtime parity slice', () => {
  it('runs live-local provider, durable replay, approval/user-input, MCP, and job lineage canaries', async () => {
    const durableTempDir = await mkdtemp(join(tmpdir(), 'analytix-go-contract-'))
    const sidecar = await startGoSidecar(durableTempDir)
    try {
      const boundary = await sidecarJSON(sidecar, `${productionPrefix}/boundary`)
      expect(boundary).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        internalGateOnly: true,
        contractReplayOnly: false,
        testConformanceOnly: false,
        usesContractReplayProviderServer: true,
        usesContractReplayMCPTransport: true,
        usesRealDurableEventSink: true,
        defaultGoBackendEnabled: false,
        rendererVisibleGoRoutesAllowed: false,
        reasonixPublicProtocolAllowed: false,
        rendererPreloadMainBridgeUnchanged: true
      }))

      const provider = await sidecarJSON(sidecar, `${productionPrefix}/provider-live`, 'POST')
      expect(provider).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        providerFamiliesCovered: true,
        contractReplayProviderServer: true,
        readsRealAPIKeys: false,
        externalNetworkUsed: false,
        requestShapeCount: 3,
        streamCompletedCount: 3,
        deepseekCacheHitTokens: 700,
        deepseekCacheMissTokens: 300,
		deepseekCacheFieldsConsistent: true,
        openaiCompatibleCacheHitTokens: 300,
        anthropicCacheHitTokens: 1000,
        stablePrefixEquivalent: true,
        dynamicStateInStablePrefix: false
      }))

      const durable = await sidecarJSON(sidecar, `${productionPrefix}/durable-replay`, 'POST')
      expect(durable).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        durableReplayFromEventSink: true,
        eventCount: 4,
        highestSeq: 4,
        usageCacheAccountingPersisted: true,
        allPersistBeforePublish: true,
        sseReplayFrameCount: 4
      }))
      expect(durable.eventKinds).toEqual([
        'turn_started',
        'pipeline_stage',
        'usage',
        'turn_completed'
      ])
      expect(durable.eventKinds).not.toContain('assistant_text_delta')

      const gate = await sidecarJSON(sidecar, `${productionPrefix}/approval-user-input`, 'POST')
      expect(gate).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        denyStatus: 200,
        deniedToolExecuted: false,
        submitStatus: 200,
        submitBodyEchoesAnswers: true,
        submittedAnswersPersistedInEvents: false,
        submittedAnswersPrivacyBoundary: true,
        cancelStatus: 200,
        timeoutStatus: 200,
        lateApprovalRejected: true
      }))

      const mcp = await sidecarJSON(sidecar, `${productionPrefix}/mcp-manager`, 'POST')
      expect(mcp).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        productionFallbackMCPTransportUsed: false,
        contractReplayMCPTransportUsed: true,
        connectedInitially: true,
        deniedMCPToolExecuted: false,
        approvedMCPToolExecuted: true,
        credentialRead: false,
        topLevelMCPIndexerRouteExposed: false,
        reasonixPublicProtocolAllowed: false
      }))

      const job = await sidecarJSON(sidecar, `${productionPrefix}/job-lineage`, 'POST')
      expect(job).toEqual(expect.objectContaining({
        runtimeGoContractParitySlice: true,
        parentGoalLineageRequired: true,
        sameParentLineage: true,
        topLevelRouteExposed: false,
        reasonixPublicProtocolAllowed: false,
        rendererVisibleRouteExposed: false
      }))

      const checklist = await sidecarJSON(sidecar, `${productionPrefix}/single-baseline-checklist`)
      expect(checklist).toEqual(expect.objectContaining({
        machineTestable: true,
        presentForbiddenRedundancyCount: 0,
        tsRuntimeDefaultRetained: true,
        defaultGoBackendEnabled: false,
        rendererPreloadMainContractChange: false,
        rendererVisibleGoSwitcher: false
      }))

      const canary = await sidecarJSON(sidecar, `${productionPrefix}/canary`, 'POST')
      expect(canary).toEqual(expect.objectContaining({
        ok: true,
        runtimeGoContractParitySlice: true,
        defaultGoBackendEnabled: false,
        rendererVisibleGoRoutesAllowed: false,
        reasonixPublicProtocolAllowed: false,
        rendererPreloadMainBridgeUnchanged: true
      }))
      expect(canary.checks).toEqual(expect.arrayContaining([
        'contract-provider-server',
        'durable-replay',
        'approval-user-input-manager',
        'contract-mcp-manager',
        'job-lineage',
        'single-baseline-checklist'
      ]))
    } finally {
      await stopSidecar(sidecar)
      await rm(durableTempDir, { recursive: true, force: true })
    }
  }, 120_000)
})

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
  method = 'GET'
): Promise<Record<string, unknown>> {
  const response = await fetch(`${sidecar.url}${path}`, {
    method,
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`
    }
  })
  expect(response.status).toBe(200)
  return recordValue(await response.json())
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}
