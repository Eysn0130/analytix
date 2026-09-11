import { createHash } from 'node:crypto'
import {
  CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_URL_ENV,
  ControlledArtifactHostV2,
  type ControlledArtifactHostOptionsV2,
  type ControlledArtifactHostRuntimeEnvironmentV2,
  type ControlledArtifactRendererLeaseV2
} from '../controlled-artifact/host-v2'
import {
  BackendGenerationAllocatorV1,
  type ConsumedBackendGenerationV1
} from './backend-generation-allocator-v1'
import { verifyControlledArtifactSidecarLaunchBindingProofV2 } from './sidecar-launch-binding-v2'

const SHA256_PATTERN = /^[a-f0-9]{64}$/
const SESSION_ERROR = 'Managed controlled artifact authority is unavailable.'
const ENVIRONMENT_KEYS = [
  CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_URL_ENV
].sort()

export type ControlledArtifactRuntimeReadyV2 = Readonly<{
  url: string
  runtimePid: number
  runtimeTokenConfigured: boolean
  persistenceRootsConfigured: boolean
  productionRuntime: boolean
  controlledArtifactHostV2Configured: boolean
  controlledArtifactHostV2Ready: boolean
  controlledArtifactHostV2BackendGeneration: number
  controlledArtifactHostV2LaunchBindingProof: string
}>

type ManagedControlledArtifactHostV2 = Pick<
  ControlledArtifactHostV2,
  'start' | 'runtimeEnvironment' | 'activateInvocations' | 'quiesceInvocations' |
  'revokeRenderer' | 'revokeTransport' | 'closeAndDrain'
>

export type ManagedGoSidecarAuthoritySessionOptionsV2 = Readonly<{
  allocator: BackendGenerationAllocatorV1
  acquireRendererLease: (
    webContentsId: number,
    rendererGeneration: number
  ) => ControlledArtifactRendererLeaseV2 | null
  signal?: AbortSignal
  createHost?: (options: ControlledArtifactHostOptionsV2) => ManagedControlledArtifactHostV2
}>

type SessionStateV2 = 'prepared' | 'active' | 'quiescing' | 'closed'

export class ManagedGoSidecarAuthoritySessionV2 {
  private state: SessionStateV2 = 'prepared'
  private environmentTaken = false
  private closePromise: Promise<void> | null = null

  private constructor(
    private readonly allocator: BackendGenerationAllocatorV1,
    private readonly allocation: ConsumedBackendGenerationV1,
    private readonly host: ManagedControlledArtifactHostV2,
    private environment: ControlledArtifactHostRuntimeEnvironmentV2 | null
  ) {}

  static async prepare(
    options: ManagedGoSidecarAuthoritySessionOptionsV2
  ): Promise<ManagedGoSidecarAuthoritySessionV2> {
    if (!options || !(options.allocator instanceof BackendGenerationAllocatorV1) ||
      typeof options.acquireRendererLease !== 'function' ||
      options.signal !== undefined && !(options.signal instanceof AbortSignal)) {
      throw fixedSessionErrorV2()
    }
    let host: ManagedControlledArtifactHostV2 | null = null
    try {
      const allocation = await options.allocator.consume(options.signal)
      options.allocator.assertExecutableIdentity()
      if (options.signal?.aborted) throw fixedSessionErrorV2()
      const createHost = options.createHost ?? ((hostOptions) => new ControlledArtifactHostV2(hostOptions))
      host = createHost({
        backendGeneration: allocation.generation,
        allocationRecordDigest: allocation.allocationRecordDigest,
        acquireRendererLease: options.acquireRendererLease
      })
      await host.start()
      if (options.signal?.aborted) throw fixedSessionErrorV2()
      const environment = validateRuntimeEnvironmentV2(host.runtimeEnvironment(), allocation)
      options.allocator.assertExecutableIdentity()
      return new ManagedGoSidecarAuthoritySessionV2(
        options.allocator,
        allocation,
        host,
        environment
      )
    } catch {
      host?.revokeTransport()
      await host?.closeAndDrain().catch(() => undefined)
      throw fixedSessionErrorV2()
    }
  }

  runtimeServerRealPath(): string {
    return this.allocator.runtimeServerRealPath()
  }

  generation(): number {
    return this.allocation.generation
  }

  allocationRecordDigest(): string {
    return this.allocation.allocationRecordDigest
  }

  takeRuntimeEnvironment(): ControlledArtifactHostRuntimeEnvironmentV2 {
    if (this.state !== 'prepared' || this.environmentTaken || !this.environment) {
      throw fixedSessionErrorV2()
    }
    this.environmentTaken = true
    return Object.freeze({ ...this.environment })
  }

  verifyReadyAndActivate(ready: ControlledArtifactRuntimeReadyV2, childPID: number): void {
    try {
      if (this.state !== 'prepared' || !this.environmentTaken || !this.environment ||
        !Number.isSafeInteger(childPID) || childPID <= 0 || ready.runtimePid !== childPID ||
        ready.runtimeTokenConfigured !== true || ready.persistenceRootsConfigured !== true ||
        ready.productionRuntime !== true || ready.controlledArtifactHostV2Configured !== true ||
        ready.controlledArtifactHostV2Ready !== true ||
        ready.controlledArtifactHostV2BackendGeneration !== this.allocation.generation) {
        throw fixedSessionErrorV2()
      }
      this.allocator.assertExecutableIdentity()
      const rootDER = decodeCanonicalBase64URL(
        this.environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV]
      )
      try {
        const binding = {
          runtimeURL: ready.url,
          runtimePID: ready.runtimePid,
          controlledArtifactHostURL: this.environment[CONTROLLED_ARTIFACT_HOST_V2_URL_ENV],
          backendGeneration: this.allocation.generation,
          allocationRecordDigest: this.allocation.allocationRecordDigest,
          tlsRootCertificateSHA256: createHash('sha256').update(rootDER).digest('hex'),
          tlsLeafSPKISHA256: this.environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV],
          controlledArtifactHostReady: true,
          runtimeTokenConfigured: true as const,
          persistenceRootsConfigured: true as const,
          productionRuntime: true as const
        }
        if (!verifyControlledArtifactSidecarLaunchBindingProofV2(
          this.environment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV],
          binding,
          ready.controlledArtifactHostV2LaunchBindingProof
        )) {
          throw fixedSessionErrorV2()
        }
      } finally {
        rootDER.fill(0)
      }
      this.allocator.assertExecutableIdentity()
      this.host.activateInvocations()
      this.state = 'active'
    } catch {
      this.revokeUnverifiedTransport()
      throw fixedSessionErrorV2()
    }
  }

  quiesce(): void {
    if (this.state === 'closed' || this.state === 'quiescing') return
    this.host.quiesceInvocations()
    this.state = 'quiescing'
  }

  revokeRenderer(webContentsId: number, rendererGeneration?: number): void {
    if (this.state === 'closed') return
    this.host.revokeRenderer(webContentsId, rendererGeneration)
  }

  revokeUnverifiedTransport(): void {
    if (this.state === 'closed') return
    this.host.revokeTransport()
    this.state = 'quiescing'
  }

  closeAndDrain(): Promise<void> {
    if (this.closePromise) return this.closePromise
    this.quiesce()
    this.environment = null
    const operation = this.host.closeAndDrain().then(() => {
      this.state = 'closed'
    }, (error) => {
      this.state = 'closed'
      throw error
    })
    this.closePromise = operation
    return operation
  }
}

function validateRuntimeEnvironmentV2(
  environment: ControlledArtifactHostRuntimeEnvironmentV2,
  allocation: ConsumedBackendGenerationV1
): ControlledArtifactHostRuntimeEnvironmentV2 {
  const keys = Object.keys(environment).sort()
  const rootText = environment?.[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV]
  const rootDER = typeof rootText === 'string' ? decodeCanonicalBase64URL(rootText) : Buffer.alloc(0)
  const secretText = environment?.[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV]
  const secret = typeof secretText === 'string' ? decodeCanonicalBase64URL(secretText) : Buffer.alloc(0)
  try {
    if (keys.length !== ENVIRONMENT_KEYS.length ||
      keys.some((key, index) => key !== ENVIRONMENT_KEYS[index]) ||
      environment[CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV] !== String(allocation.generation) ||
      environment[CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV] !==
        allocation.allocationRecordDigest ||
      !validExactHostOrigin(environment[CONTROLLED_ARTIFACT_HOST_V2_URL_ENV]) ||
      secret.length !== 32 || rootDER.length === 0 || rootDER.length > 2 << 10 ||
      !SHA256_PATTERN.test(environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV])) {
      throw fixedSessionErrorV2()
    }
    return Object.freeze({ ...environment })
  } finally {
    rootDER.fill(0)
    secret.fill(0)
  }
}

function decodeCanonicalBase64URL(value: string): Buffer {
  if (typeof value !== 'string' || value === '' || value.includes('=')) throw fixedSessionErrorV2()
  const decoded = Buffer.from(value, 'base64url')
  if (decoded.length === 0 || decoded.toString('base64url') !== value) {
    decoded.fill(0)
    throw fixedSessionErrorV2()
  }
  return decoded
}

function validExactHostOrigin(value: string): boolean {
  try {
    const parsed = new URL(value)
    const port = Number(parsed.port)
    return parsed.protocol === 'https:' && parsed.hostname === '127.0.0.1' &&
      parsed.host === `127.0.0.1:${port}` && parsed.username === '' && parsed.password === '' &&
      parsed.pathname === '/' && parsed.search === '' && parsed.hash === '' &&
      Number.isSafeInteger(port) && port > 0 && port <= 65_535
  } catch {
    return false
  }
}

function fixedSessionErrorV2(): Error {
  return new Error(SESSION_ERROR)
}
