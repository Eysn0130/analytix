import { spawn, type ChildProcessByStdio } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Readable } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { findGoBinary, goTestEnv } from './go-test-toolchain.js'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..', '..')
const runtimeGoDir = join(repoRoot, 'packages', 'runtime-go')
const kernelContractUrl = new URL('../src/conformance/fixtures/go-kernel-live-scaffold-contract.json', import.meta.url)
type GoTestChildProcess = ChildProcessByStdio<null, Readable, Readable>

type KernelContract = {
  id: string
  sourceContractIds: string[]
  components: Array<{ id: string; expectedLive: boolean; sideEffectsAllowed: boolean }>
  jobOrchestration: {
    fixtureOnly: boolean
    parentGoalRequired: boolean
    topLevelRoutesExposed: boolean
    publicSubagentProtocolAllowed: boolean
    jobManagerLive: boolean
    backgroundExecutionAllowed: boolean
    expectedStatuses: string[]
    expectedTools: string[]
  }
  retirementCleanup: {
    g6Ready: boolean
    noImmediateProductionDelete: boolean
    currentRetainedPaths: Array<{ id: string; path: string; reason: string }>
    deleteAfterG6: Array<{ id: string; path: string; reason: string }>
    forbiddenRedundancy: Array<{ id: string; token: string; present: boolean }>
    g6Blockers: string[]
  }
  expected: {
    componentCount: number
    liveComponentCount: number
    replaceOrPortCount: number
    contractReimplementCount: number
    deferCount: number
    rejectCount: number
    forbiddenSurfaceCount: number
    currentRetainedCount: number
    deleteAfterG6Count: number
    forbiddenRedundancyCount: number
    presentForbiddenRedundancyCount: number
    g6BlockerCount: number
    sideEffects: Record<string, number>
  }
}

type SidecarHandle = {
  url: string
  runtimeToken: string
  process: GoTestChildProcess
  stderr: () => string
}

describe('Go kernel live scaffold sidecar conformance', () => {
  it('exposes a fixture-only analytix kernel map across loop, durable, cache, gate, MCP, and job scaffolds', async () => {
    const contract = await readKernelContract()
    const durableTempDir = await mkdtemp(join(tmpdir(), 'analytix-go-kernel-'))
    const sidecar = await startGoSidecar(durableTempDir)
    try {
      const boundary = await sidecarJSON(sidecar, '/v1/conformance/kernel/boundary')
      expect(boundary.fixtureOnly).toBe(true)
      expect(boundary.testConformanceOnly).toBe(true)
      expect(boundary.internalGateOnly).toBe(true)
      expect(boundary.kernelLiveScaffoldPrototype).toBe(true)
      expect(boundary.fixtureBackedKernelOnly).toBe(true)
      expect(boundary.durableStoreAvailable).toBe(true)
      expect(boundary.minimalLoopAvailable).toBe(true)
      expect(boundary.providerCacheReplayAvailable).toBe(true)
      expect(boundary.approvalUserInputGateAvailable).toBe(true)
      expect(boundary.mcpCatalogRecoveryAvailable).toBe(true)
      expect(boundary.jobSubagentFixtureAvailable).toBe(true)
      expect(boundary.providerLiveCallsAllowed).toBe(false)
      expect(boundary.toolExecutionAllowed).toBe(false)
      expect(boundary.approvalExecutionAllowed).toBe(false)
      expect(boundary.mcpConnectionAllowed).toBe(false)
      expect(boundary.defaultGoBackendEnabled).toBe(false)
      expect(boundary.rendererVisibleGoRoutesAllowed).toBe(false)
      expect(boundary.reasonixPublicProtocolAllowed).toBe(false)
      expect(recordValue(boundary.productBoundary)).toEqual(expect.objectContaining({
        kernelLiveScaffoldPrototype: true,
        fixtureBackedKernelOnly: true,
        internalGateOnly: true,
        defaultGoBackendEnabled: false,
        rendererVisibleGoRoutesAllowed: false,
        reasonixPublicProtocolAllowed: false
      }))

      const capabilities = await sidecarJSON(sidecar, '/v1/conformance/kernel/capabilities')
      expect(capabilities.componentCount).toBe(contract.expected.componentCount)
      expect(capabilities.liveComponentCount).toBe(contract.expected.liveComponentCount)
      expect(capabilities.allExpectedLive).toBe(true)
      expect(arrayValue(capabilities.components).map((item) => recordValue(item).id))
        .toEqual(contract.components.map((item) => item.id))
      for (const item of arrayValue(capabilities.components)) {
        expect(recordValue(item)).toEqual(expect.objectContaining({
          live: true,
          matchesExpected: true,
          sideEffectsAllowed: false
        }))
      }
      expect(recordValue(capabilities.sideEffects)).toEqual(contract.expected.sideEffects)

      const absorption = await sidecarJSON(sidecar, '/v1/conformance/kernel/absorption-contract')
      expect(absorption.sourceContractIds).toEqual(contract.sourceContractIds)
      expect(absorption.replaceOrPortCount).toBe(contract.expected.replaceOrPortCount)
      expect(absorption.contractReimplementCount).toBe(contract.expected.contractReimplementCount)
      expect(absorption.deferCount).toBe(contract.expected.deferCount)
      expect(absorption.rejectCount).toBe(contract.expected.rejectCount)
      expect(absorption.matchesExpected).toBe(true)

      const job = await sidecarJSON(sidecar, '/v1/conformance/kernel/job-orchestration')
      expect(job.fixtureOnly).toBe(contract.jobOrchestration.fixtureOnly)
      expect(job.parentGoalRequired).toBe(contract.jobOrchestration.parentGoalRequired)
      expect(job.topLevelRoutesExposed).toBe(false)
      expect(job.publicSubagentProtocolAllowed).toBe(false)
      expect(job.jobManagerLive).toBe(false)
      expect(job.backgroundExecutionAllowed).toBe(false)
      expect(job.expectedStatuses).toEqual(contract.jobOrchestration.expectedStatuses)
      expect(job.expectedTools).toEqual(contract.jobOrchestration.expectedTools)
      expect(job.forbiddenSurfaceCount).toBe(contract.expected.forbiddenSurfaceCount)
      expect(job.matchesExpected).toBe(true)

      const cleanup = await sidecarJSON(sidecar, '/v1/conformance/kernel/g6-retirement-cleanup')
      expect(cleanup.fixtureOnly).toBe(true)
      expect(cleanup.g6Ready).toBe(false)
      expect(cleanup.noImmediateProductionDelete).toBe(true)
      expect(cleanup.tsRuntimeRetained).toBe(true)
      expect(cleanup.currentRetainedCount).toBe(contract.expected.currentRetainedCount)
      expect(cleanup.deleteAfterG6Count).toBe(contract.expected.deleteAfterG6Count)
      expect(cleanup.forbiddenRedundancyCount).toBe(contract.expected.forbiddenRedundancyCount)
      expect(cleanup.presentForbiddenRedundancyCount).toBe(contract.expected.presentForbiddenRedundancyCount)
      expect(cleanup.g6BlockerCount).toBe(contract.expected.g6BlockerCount)
      expect(cleanup.matchesExpected).toBe(true)
      expect(arrayValue(cleanup.currentRetainedPaths).map((item) => recordValue(item).id))
        .toEqual(contract.retirementCleanup.currentRetainedPaths.map((item) => item.id))
      expect(arrayValue(cleanup.deleteAfterG6).map((item) => recordValue(item).id))
        .toEqual(contract.retirementCleanup.deleteAfterG6.map((item) => item.id))
      expect(arrayValue(cleanup.forbiddenRedundancy).every((item) => recordValue(item).present === false))
        .toBe(true)
      expect(cleanup.g6Blockers).toEqual(contract.retirementCleanup.g6Blockers)

      const snapshot = await sidecarJSON(sidecar, '/v1/conformance/kernel/snapshot')
      expect(snapshot.durableStoreAvailable).toBe(true)
      expect(snapshot.minimalLoopAvailable).toBe(true)
      expect(snapshot.providerCacheReplayAvailable).toBe(true)
      expect(snapshot.g4ManagerAvailable).toBe(true)
      expect(recordValue(snapshot.sideEffects)).toEqual(contract.expected.sideEffects)
      expect(Number(snapshot.kernelReplayAttempts)).toBeGreaterThanOrEqual(4)
    } finally {
      await stopSidecar(sidecar)
      await rm(durableTempDir, { recursive: true, force: true })
    }
  }, 120_000)
})

async function readKernelContract(): Promise<KernelContract> {
  return JSON.parse(await readFile(kernelContractUrl, 'utf8')) as KernelContract
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

async function sidecarJSON(sidecar: SidecarHandle, path: string): Promise<Record<string, unknown>> {
  const response = await fetch(`${sidecar.url}${path}`, {
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`
    }
  })
  expect(response.status).toBe(200)
  return recordValue(await response.json())
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}
