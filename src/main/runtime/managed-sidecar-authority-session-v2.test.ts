import { createHash } from 'node:crypto'
import { chmodSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_URL_ENV,
  type ControlledArtifactHostOptionsV2,
  type ControlledArtifactHostRuntimeEnvironmentV2
} from '../controlled-artifact/host-v2'
import {
  BackendGenerationAllocatorV1,
  type BackendGenerationAuthorityExecutorV1
} from './backend-generation-allocator-v1'
import {
  ManagedGoSidecarAuthoritySessionV2,
  type ControlledArtifactRuntimeReadyV2
} from './managed-sidecar-authority-session-v2'
import { controlledArtifactSidecarLaunchBindingProofV2 } from './sidecar-launch-binding-v2'

const roots: string[] = []

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

describe('ManagedGoSidecarAuthoritySessionV2', () => {
  it('binds one consumed generation to one exact executable and activates only after a true proof', async () => {
    const allocator = allocatorFixtureV1(async () => ({
      stdout: markerV1(17, 'd'.repeat(64)),
      stderr: ''
    }))
    const host = fakeHostV2(environmentV2(17, 'd'.repeat(64)))
    const session = await ManagedGoSidecarAuthoritySessionV2.prepare({
      allocator,
      acquireRendererLease: () => null,
      createHost: (options) => {
        expect(options.backendGeneration).toBe(17)
        expect(options.allocationRecordDigest).toBe('d'.repeat(64))
        return host.api
      }
    })

    expect(session.runtimeServerRealPath()).toBe(allocator.runtimeServerRealPath())
    expect(session.generation()).toBe(17)
    const environment = session.takeRuntimeEnvironment()
    expect(() => session.takeRuntimeEnvironment()).toThrow('Managed controlled artifact authority is unavailable.')
    const ready = readyV2(environment, 4242, true)
    session.verifyReadyAndActivate(ready, 4242)
    expect(host.activated).toBe(1)
    expect(host.revoked).toBe(0)

    session.quiesce()
    expect(host.quiesced).toBe(1)
    await session.closeAndDrain()
    expect(host.closed).toBe(1)
  })

  it('revokes the transport and never activates on false readiness, PID mismatch or proof mismatch', async () => {
    for (const mutate of [
      (ready: ControlledArtifactRuntimeReadyV2) => ({ ...ready, controlledArtifactHostV2Ready: false }),
      (ready: ControlledArtifactRuntimeReadyV2) => ({ ...ready, runtimePid: ready.runtimePid + 1 }),
      (ready: ControlledArtifactRuntimeReadyV2) => ({
        ...ready,
        controlledArtifactHostV2LaunchBindingProof: 'f'.repeat(64)
      })
    ]) {
      const allocator = allocatorFixtureV1(async () => ({
        stdout: markerV1(19, 'e'.repeat(64)),
        stderr: ''
      }))
      const environment = environmentV2(19, 'e'.repeat(64))
      const host = fakeHostV2(environment)
      const session = await ManagedGoSidecarAuthoritySessionV2.prepare({
        allocator,
        acquireRendererLease: () => null,
        createHost: () => host.api
      })
      session.takeRuntimeEnvironment()
      expect(() => session.verifyReadyAndActivate(mutate(readyV2(environment, 5252, true)), 5252))
        .toThrow('Managed controlled artifact authority is unavailable.')
      expect(host.activated).toBe(0)
      expect(host.revoked).toBe(1)
      await session.closeAndDrain()
    }
  })

  it('burns an allocated generation when host preparation fails', async () => {
    let generation = 0
    const allocator = allocatorFixtureV1(async () => {
      generation += 1
      return {
        stdout: markerV1(generation, generation.toString(16).padStart(64, '0')),
        stderr: ''
      }
    })
    const failed = fakeHostV2(environmentV2(1, '1'.padStart(64, '0')))
    failed.startFailure = true
    await expect(ManagedGoSidecarAuthoritySessionV2.prepare({
      allocator,
      acquireRendererLease: () => null,
      createHost: () => failed.api
    })).rejects.toThrow('Managed controlled artifact authority is unavailable.')

    const secondEnvironment = environmentV2(2, '2'.padStart(64, '0'))
    const second = fakeHostV2(secondEnvironment)
    const session = await ManagedGoSidecarAuthoritySessionV2.prepare({
      allocator,
      acquireRendererLease: () => null,
      createHost: (options: ControlledArtifactHostOptionsV2) => {
        expect(options.backendGeneration).toBe(2)
        return second.api
      }
    })
    expect(session.generation()).toBe(2)
    await session.closeAndDrain()
  })
})

function allocatorFixtureV1(execute: BackendGenerationAuthorityExecutorV1): BackendGenerationAllocatorV1 {
  const root = mkdtempSync(join(tmpdir(), 'analytix-sidecar-authority-session-'))
  roots.push(root)
  const runtimeServerPath = join(root, 'runtime-server')
  const userDataRealPath = join(root, 'user-data')
  writeFileSync(runtimeServerPath, 'test-runtime-server', { mode: 0o700 })
  chmodSync(runtimeServerPath, 0o700)
  mkdirSync(userDataRealPath, { mode: 0o700 })
  return new BackendGenerationAllocatorV1({
    runtimeServerPath: realpathSync(runtimeServerPath),
    userDataRealPath: realpathSync(userDataRealPath),
    execute
  })
}

function environmentV2(
  generation: number,
  allocationRecordDigest: string
): ControlledArtifactHostRuntimeEnvironmentV2 {
  return Object.freeze({
    [CONTROLLED_ARTIFACT_HOST_V2_URL_ENV]: 'https://127.0.0.1:45124',
    [CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV]: Buffer.alloc(32, 0x51).toString('base64url'),
    [CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV]: String(generation),
    [CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV]: allocationRecordDigest,
    [CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV]: Buffer.from('test-root-der').toString('base64url'),
    [CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV]: 'a'.repeat(64)
  })
}

function readyV2(
  environment: ControlledArtifactHostRuntimeEnvironmentV2,
  runtimePid: number,
  controlledArtifactHostV2Ready: boolean
): ControlledArtifactRuntimeReadyV2 {
  const generation = Number(environment[CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV])
  const binding = {
    runtimeURL: 'http://127.0.0.1:45123/',
    runtimePID: runtimePid,
    controlledArtifactHostURL: environment[CONTROLLED_ARTIFACT_HOST_V2_URL_ENV],
    backendGeneration: generation,
    allocationRecordDigest: environment[CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV],
    tlsRootCertificateSHA256: createHash('sha256')
      .update(Buffer.from(environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV], 'base64url'))
      .digest('hex'),
    tlsLeafSPKISHA256: environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV],
    controlledArtifactHostReady: controlledArtifactHostV2Ready,
    runtimeTokenConfigured: true as const,
    persistenceRootsConfigured: true as const,
    productionRuntime: true as const
  }
  return {
    url: binding.runtimeURL,
    runtimePid,
    runtimeTokenConfigured: true,
    persistenceRootsConfigured: true,
    productionRuntime: true,
    controlledArtifactHostV2Configured: true,
    controlledArtifactHostV2Ready,
    controlledArtifactHostV2BackendGeneration: generation,
    controlledArtifactHostV2LaunchBindingProof: controlledArtifactSidecarLaunchBindingProofV2(
      environment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV],
      binding
    )
  }
}

function fakeHostV2(environment: ControlledArtifactHostRuntimeEnvironmentV2): {
  api: {
    start: () => Promise<void>
    runtimeEnvironment: () => ControlledArtifactHostRuntimeEnvironmentV2
    activateInvocations: () => void
    quiesceInvocations: () => void
    revokeRenderer: () => void
    revokeTransport: () => void
    closeAndDrain: () => Promise<void>
  }
  activated: number
  quiesced: number
  revoked: number
  closed: number
  startFailure: boolean
} {
  const state = {
    activated: 0,
    quiesced: 0,
    revoked: 0,
    closed: 0,
    startFailure: false,
    api: {
      start: async () => {
        if (state.startFailure) throw new Error('PRIVATE-HOST-FAILURE')
      },
      runtimeEnvironment: () => environment,
      activateInvocations: () => { state.activated += 1 },
      quiesceInvocations: () => { state.quiesced += 1 },
      revokeRenderer: () => undefined,
      revokeTransport: () => { state.revoked += 1 },
      closeAndDrain: async () => { state.closed += 1 }
    }
  }
  return state
}

function markerV1(generation: number, digest: string): string {
  return `ANALYTIX_BACKEND_GENERATION_CONSUMED ${JSON.stringify({
    schemaVersion: 1,
    generation,
    allocationRecordDigest: digest
  })}\n`
}
