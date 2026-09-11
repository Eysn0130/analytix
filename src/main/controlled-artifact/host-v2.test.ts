import {
  createHash,
  createHmac,
  generateKeyPairSync,
  sign,
  X509Certificate,
  type KeyObject
} from 'node:crypto'
import { request as httpsRequest, type RequestOptions as HTTPSRequestOptions } from 'node:https'
import { mkdtemp, readFile, readdir, realpath, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { ConnectionOptions as TLSConnectionOptions } from 'node:tls'
import { afterEach, describe, expect, it } from 'vitest'
import { generateEphemeralControlledArtifactTLSIdentityV2 } from './ephemeral-tls-identity'
import {
  CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_URL_ENV,
  ControlledArtifactHostV2,
  type ControlledArtifactContextBindingV2,
  type ControlledArtifactHostOptionsV2,
  type ControlledArtifactHostRuntimeEnvironmentV2,
  type ControlledArtifactPreparedReleaseV2,
  type ControlledArtifactReleasePreparationV2,
  type ControlledArtifactReleaseReceiptV2
} from './host-v2'
import { ControlledArtifactExportJournalV2 } from './export-journal-v2'
import { freezeControlledArtifactExportTargetV2 } from './file-effects-v2'

const JSON_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-host-v2+json'
const RELEASE_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-release-v2'
const RELEASE_MAGIC = Buffer.from('ANXCRV2\n', 'ascii')
const HANDLE_DOMAIN = Buffer.from('analytix.controlled-artifact-access/handle/v2\0')
const SLOT_DOMAIN = Buffer.from('analytix.controlled-artifact-access/use-slot/v2\0')
const PRINCIPAL_DOMAIN = Buffer.from('analytix.controlled-artifact-access/renderer-principal/v2\0')
const ACCESS_ID_DOMAIN = Buffer.from('analytix.controlled-artifact-access/id/v2\0')
const RECEIPT_SIGNING_DOMAIN = Buffer.from('analytix.controlled-artifact-access-receipt/signature/v2\0')
const RECEIPT_RECORD_DOMAIN = Buffer.from('analytix.controlled-artifact-access-receipt/record/v2\0')
const ACK_DOMAIN = Buffer.from('analytix.controlled-artifact-release-ack/hmac/v2\0')

const runningHosts: ControlledArtifactHostV2[] = []

afterEach(async () => {
  await Promise.all(runningHosts.splice(0).map((host) => host.closeAndDrain()))
})

describe('ControlledArtifactHostV2', () => {
  it('serves a byte-canonical TLS V2 admission and one exact controlled release', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    const protectedBody = Buffer.from('{"bankAccount":"0006222020202020202020"}', 'utf8')
    const fixture = await createHostFixture(() => now, 9, undefined, protectedBody)
    const admission = await postAdmission(fixture.environment, fixture.admissionRequest)

    expect(admission.status).toBe(200)
    expect(admission.contentType).toBe(JSON_MEDIA_TYPE)
    expect(JSON.parse(admission.body.toString('utf8'))).toEqual({
      schemaVersion: 2,
      purpose: 'analytix.controlled-artifact-admission/v2',
      contextDigest: fixture.context.contextDigest,
      accessAction: 'display',
      controlledHandleDigest: digest(HANDLE_DOMAIN, fixture.invocation.controlledHandle),
      useSlotDigest: digest(SLOT_DOMAIN, fixture.invocation.useSlot),
      rendererPrincipalDigest: digest(PRINCIPAL_DOMAIN, fixture.invocation.rendererPrincipal),
      rendererGeneration: 7,
      backendGeneration: 9,
      deliveryId: hex('7'),
      deliveryOutcomeRecordDigest: hex('8'),
      publicationCommitDigest: hex('9'),
      releaseTargetIdentityDigest: fixture.releaseTargetIdentityDigest,
      authorizedUntil: fixture.authorizedUntil
    })

    const receipt = createSignedReceipt(fixture, protectedBody)
    const frame = createReleaseFrame(receipt, protectedBody)
    now = '2026-07-18T12:00:02.123456788Z'
    const released = await post(fixture.environment, '/v2/controlled-artifacts/release', RELEASE_MEDIA_TYPE, frame)

    expect(released.status).toBe(200)
    expect(fixture.releasedBodies).toEqual(['{"bankAccount":"0006222020202020202020"}'])
    const ack = JSON.parse(released.body.toString('utf8')) as Record<string, unknown>
    expect(ack).toMatchObject({
      schemaVersion: 2,
      purpose: 'analytix.controlled-artifact-release-ack/v2',
      committed: true,
      accessId: receipt.accessId,
      accessReceiptDigest: receipt.recordDigest,
      contextDigest: fixture.context.contextDigest,
      deliveryId: receipt.deliveryId,
      deliveryOutcomeRecordDigest: receipt.deliveryOutcomeRecordDigest,
      publicationCommitDigest: receipt.publicationCommitDigest,
      releaseTargetIdentityDigest: receipt.releaseTargetIdentityDigest,
      artifactSha256: receipt.artifactSha256,
      releasedByteLength: protectedBody.length,
      committedAt: now
    })
    expect(ack.mac).toBe(expectedAckMAC(fixture.environment, ack))

    const committedAdmission = await postAdmission(fixture.environment, fixture.admissionRequest)
    expect(committedAdmission.status).toBe(200)
    const replay = await post(fixture.environment, '/v2/controlled-artifacts/release', RELEASE_MEDIA_TYPE, frame)
    expect(replay.status).toBe(409)
    expect(fixture.releasedBodies).toHaveLength(1)
  })

  it('rejects a legacy DSV1 context at the V2 admission boundary', async () => {
    const fixture = await createHostFixture(() => '2026-07-18T12:00:00.123456789Z', 10)
    const legacyRequest = {
      ...fixture.admissionRequest,
      context: {
        ...fixture.context,
        datasetSnapshotId: `dsv1_${hex('2')}`
      }
    }

    expect((await postAdmission(fixture.environment, legacyRequest)).status).toBe(400)
  })

  it('makes concurrent release a single host-side effect', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let enterEffect!: () => void
    let finishEffect!: () => void
    const effectEntered = new Promise<void>((resolve) => { enterEffect = resolve })
    const effectMayFinish = new Promise<void>((resolve) => { finishEffect = resolve })
    const body = Buffer.from('{"bankAccount":"0000000000000000000001"}', 'utf8')
    const fixture = await createHostFixture(() => now, 11, async (body) => {
      fixture.releasedBodies.push(body.toString('utf8'))
      enterEffect()
      await effectMayFinish
    }, body)
    const frame = createReleaseFrame(createSignedReceipt(fixture, body), body)
    now = '2026-07-18T12:00:01.123456789Z'
    const requests = Array.from({ length: 16 }, () =>
      post(fixture.environment, '/v2/controlled-artifacts/release', RELEASE_MEDIA_TYPE, frame))
    await effectEntered
    now = '2026-07-18T12:00:02.123456788Z'
    finishEffect()
    const responses = await Promise.all(requests)

    expect(responses.filter((response) => response.status === 200)).toHaveLength(1)
    expect(responses.filter((response) => response.status === 409)).toHaveLength(15)
    expect(fixture.releasedBodies).toEqual(['{"bankAccount":"0000000000000000000001"}'])
  })

  it('drains an admitted release before clearing its TLS generation', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let enterEffect!: () => void
    let finishEffect!: () => void
    const effectEntered = new Promise<void>((resolve) => { enterEffect = resolve })
    const effectMayFinish = new Promise<void>((resolve) => { finishEffect = resolve })
    const body = Buffer.from('{"bankAccount":"0000000000000000000002"}', 'utf8')
    const fixture = await createHostFixture(() => now, 12, async () => {
      enterEffect()
      await effectMayFinish
    }, body)
    const frame = createReleaseFrame(createSignedReceipt(fixture, body), body)
    now = '2026-07-18T12:00:01.123456789Z'
    const releasePromise = post(
      fixture.environment,
      '/v2/controlled-artifacts/release',
      RELEASE_MEDIA_TYPE,
      frame
    )
    await effectEntered
    let closed = false
    const closePromise = fixture.host.closeAndDrain().then(() => { closed = true })
    await Promise.resolve()
    expect(closed).toBe(false)
    now = '2026-07-18T12:00:02.123456788Z'
    finishEffect()

    expect((await releasePromise).status).toBe(200)
    await closePromise
    expect(() => fixture.host.runtimeEnvironment()).toThrow('host is not running')
  })

  it('rotates every authority value between sidecar generations and rejects V1 paths', async () => {
    const first = await createHostFixture(() => '2026-07-18T12:00:00.123456789Z', 21)
    const firstEnvironment = first.environment
    const v1 = await post(firstEnvironment, '/v1/controlled-artifacts/admission', JSON_MEDIA_TYPE,
      Buffer.from('{}'))
    expect(v1.status).toBe(404)
    await first.host.closeAndDrain()

    const second = await createHostFixture(() => '2026-07-18T12:00:00.123456789Z', 22)
    expect(second.environment[CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV]).toBe('22')
    expect(second.environment[CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV])
      .not.toBe(firstEnvironment[CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV])
    expect(second.environment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV])
      .not.toBe(firstEnvironment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV])
    expect(second.environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV])
      .not.toBe(firstEnvironment[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV])
    expect(second.environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV])
      .not.toBe(firstEnvironment[CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV])
  })

  it('proves the exact quiescent TLS transport generation before invocation activation', async () => {
    const host = new ControlledArtifactHostV2({
      backendGeneration: 23,
      allocationRecordDigest: '7'.repeat(64),
      acquireRendererLease: () => null
    })
    runningHosts.push(host)
    await host.start()
    const environment = host.runtimeEnvironment()
    const probe = {
      schemaVersion: 2,
      purpose: 'analytix.controlled-artifact-host-probe-request/v2',
      probeNonce: Buffer.alloc(32, 0x42).toString('base64url'),
      backendGeneration: 23
    }
    const response = await post(
      environment,
      '/v2/controlled-artifacts/probe',
      JSON_MEDIA_TYPE,
      Buffer.from(goJSONStringify(probe), 'utf8')
    )
    expect(response.status).toBe(200)
    expect(response.body.toString('utf8')).toBe(goJSONStringify({
      schemaVersion: 2,
      purpose: 'analytix.controlled-artifact-host-probe-response/v2',
      probeNonce: probe.probeNonce,
      backendGeneration: 23,
      transportReady: true,
      invocationsActive: false
    }))

    const duplicate = Buffer.from(
      `{"schemaVersion":2,"schemaVersion":2,"purpose":"${probe.purpose}",` +
      `"probeNonce":"${probe.probeNonce}","backendGeneration":23}`,
      'utf8'
    )
    expect((await post(environment, '/v2/controlled-artifacts/probe', JSON_MEDIA_TYPE, duplicate)).status)
      .toBe(400)
    host.activateInvocations()
    expect((await post(
      environment,
      '/v2/controlled-artifacts/probe',
      JSON_MEDIA_TYPE,
      Buffer.from(goJSONStringify(probe), 'utf8')
    )).status).toBe(409)
  })

  it('single-flights concurrent starts and cannot revive after close-during-start', async () => {
    const identity = await generateEphemeralControlledArtifactTLSIdentityV2()
    let allowIdentity!: () => void
    const identityGate = new Promise<void>((resolve) => { allowIdentity = resolve })
    let identityCalls = 0
    const host = new ControlledArtifactHostV2({
      backendGeneration: 31,
      allocationRecordDigest: '3'.repeat(64),
      createTLSIdentity: async () => {
        identityCalls += 1
        await identityGate
        return identity
      },
      acquireRendererLease: () => ({
        signal: new AbortController().signal,
        isCurrent: () => true,
        release: () => undefined
      })
    })
    runningHosts.push(host)
    const starts = Array.from({ length: 16 }, () => host.start())
    const close = host.closeAndDrain()
    allowIdentity()

    await Promise.all([...starts, close])
    expect(identityCalls).toBe(1)
    expect(() => host.runtimeEnvironment()).toThrow('host is not running')
  })

  it('freezes constructor authority and issues the sidecar environment exactly once', async () => {
    const options: ControlledArtifactHostOptionsV2 = {
      backendGeneration: 41,
      allocationRecordDigest: '4'.repeat(64),
      acquireRendererLease: () => ({
        signal: new AbortController().signal,
        isCurrent: () => true,
        release: () => undefined
      })
    }
    const host = new ControlledArtifactHostV2(options)
    runningHosts.push(host)
    options.backendGeneration = 42
    options.acquireRendererLease = () => null
    await host.start()

    const environment = host.runtimeEnvironment()
    expect(environment[CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV]).toBe('41')
    expect(() => host.runtimeEnvironment()).toThrow('host is not running')
  })

  it('keeps quiescing one-way while allowing an already-issued admission to drain', async () => {
    const fixture = await createHostFixture(() => '2026-07-18T12:00:00.123456789Z', 42)
    fixture.host.quiesceInvocations()

    expect(() => fixture.host.activateInvocations()).toThrow('host is not running')
    expect((await postAdmission(fixture.environment, fixture.admissionRequest)).status).toBe(200)
  })

  it('rejects a future-dated signed receipt before any release preparation', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let effects = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000003"}', 'utf8')
    const fixture = await createHostFixture(() => now, 43, async () => { effects += 1 }, body)
    const receipt = createSignedReceipt(fixture, body)
    receipt.requestedAt = '2026-07-18T12:00:05.123456789Z'
    resignReceipt(fixture, receipt)
    now = '2026-07-18T12:00:01.123456789Z'

    const released = await post(
      fixture.environment,
      '/v2/controlled-artifacts/release',
      RELEASE_MEDIA_TYPE,
      createReleaseFrame(receipt, body)
    )
    expect(released.status).toBe(409)
    expect(effects).toBe(0)
  })

  it('rejects a validly re-signed receipt whose target binding differs from the invocation', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let effects = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000004"}', 'utf8')
    const fixture = await createHostFixture(() => now, 44, async () => { effects += 1 }, body)
    const receipt = createSignedReceipt(fixture, body)
    receipt.targetIdentityDigest = hex('3')
    resignReceipt(fixture, receipt)
    now = '2026-07-18T12:00:01.123456789Z'

    const released = await post(
      fixture.environment,
      '/v2/controlled-artifacts/release',
      RELEASE_MEDIA_TYPE,
      createReleaseFrame(receipt, body)
    )
    expect(released.status).toBe(409)
    expect(effects).toBe(0)
  })

  it('rejects a validly re-signed receipt whose trusted-sink target differs from the invocation', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let effects = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000009"}', 'utf8')
    const fixture = await createHostFixture(() => now, 50, async () => { effects += 1 }, body)
    const receipt = createSignedReceipt(fixture, body)
    receipt.releaseTargetIdentityDigest = hex('4')
    resignReceipt(fixture, receipt)
    now = '2026-07-18T12:00:01.123456789Z'

    const released = await post(
      fixture.environment,
      '/v2/controlled-artifacts/release',
      RELEASE_MEDIA_TYPE,
      createReleaseFrame(receipt, body)
    )
    expect(released.status).toBe(409)
    expect(effects).toBe(0)
  })

  it('never acknowledges a commit whose host clock reaches the authorization boundary', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let effects = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000005"}', 'utf8')
    const fixture = await createHostFixture(() => now, 45, async () => {
      effects += 1
      now = '2026-07-18T12:10:00.123456789Z'
    }, body)
    const receipt = createSignedReceipt(fixture, body)
    now = '2026-07-18T12:00:01.123456789Z'

    const released = await post(
      fixture.environment,
      '/v2/controlled-artifacts/release',
      RELEASE_MEDIA_TYPE,
      createReleaseFrame(receipt, body)
    )
    expect(released.status).toBe(409)
    expect(effects).toBe(1)
  })

  it('revokes the generation on wall-clock rollback and cannot be revived', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let monotonic = 100n
    const fixture = await createHostFixture(() => now, 46, undefined, undefined, () => monotonic)
    now = '2026-07-18T11:59:59.123456789Z'
    monotonic += 1n

    expect((await postAdmission(fixture.environment, fixture.admissionRequest)).status).toBe(500)
    expect(() => fixture.host.runtimeEnvironment()).toThrow('host is not running')
    expect(() => fixture.host.activateInvocations()).toThrow('host is not running')
  })

  it('tracks a preparation that ignores abort and scrubs it before close can finish', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let enterPreparation!: () => void
    let resolvePreparation!: (prepared: ControlledArtifactPreparedReleaseV2) => void
    const preparationEntered = new Promise<void>((resolve) => { enterPreparation = resolve })
    const latePreparation = new Promise<ControlledArtifactPreparedReleaseV2>((resolve) => {
      resolvePreparation = resolve
    })
    let aborts = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000006"}', 'utf8')
    const prepareRelease: ControlledArtifactReleasePreparationV2 = async () => {
      enterPreparation()
      return latePreparation
    }
    const fixture = await createHostFixture(
      () => now,
      47,
      undefined,
      body,
      undefined,
      10,
      prepareRelease
    )
    const frame = createReleaseFrame(createSignedReceipt(fixture, body), body)
    now = '2026-07-18T12:00:01.123456789Z'
    const release = post(fixture.environment, '/v2/controlled-artifacts/release', RELEASE_MEDIA_TYPE, frame)
    await preparationEntered
    expect((await release).status).toBe(500)

    let closed = false
    const closing = fixture.host.closeAndDrain().then(() => { closed = true })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(closed).toBe(false)
    resolvePreparation({
      commitBefore: async () => { throw new Error('must not commit a timed-out preparation') },
      abort: async () => { aborts += 1 }
    })
    await closing
    expect(aborts).toBe(1)
  })

  it('does not finish close while an abort-ignoring commit can still publish late', async () => {
    let now = '2026-07-18T12:00:00.123456789Z'
    let enterCommit!: () => void
    let resolveCommit!: () => void
    const commitEntered = new Promise<void>((resolve) => { enterCommit = resolve })
    let lateEffects = 0
    let aborts = 0
    const body = Buffer.from('{"bankAccount":"0000000000000000000007"}', 'utf8')
    const prepareRelease: ControlledArtifactReleasePreparationV2 = async ({
      receipt,
      releaseTargetIdentityDigest
    }) => ({
      commitBefore: async () => {
        enterCommit()
        await new Promise<void>((resolve) => {
          resolveCommit = () => {
            lateEffects += 1
            resolve()
          }
        })
        return {
          releaseTargetIdentityDigest,
          artifactSha256: receipt.artifactSha256,
          artifactByteLength: receipt.artifactByteLength,
          mediaType: receipt.mediaType,
          committedAt: now
        }
      },
      abort: async () => { aborts += 1 }
    })
    const fixture = await createHostFixture(
      () => now,
      48,
      undefined,
      body,
      undefined,
      10,
      prepareRelease
    )
    const frame = createReleaseFrame(createSignedReceipt(fixture, body), body)
    now = '2026-07-18T12:00:01.123456789Z'
    const release = post(fixture.environment, '/v2/controlled-artifacts/release', RELEASE_MEDIA_TYPE, frame)
    await commitEntered
    expect((await release).status).toBe(500)
    expect(aborts).toBe(1)

    let closed = false
    const closing = fixture.host.closeAndDrain().then(() => { closed = true })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(closed).toBe(false)
    expect(lateEffects).toBe(0)
    resolveCommit()
    await closing
    expect(lateEffects).toBe(1)
  })

  it('propagates renderer revocation into the final synchronous export boundary', async () => {
    const root = await realpath(await mkdtemp(join(tmpdir(), 'analytix-host-v2-renderer-revoke-')))
    let now = '2026-07-18T12:00:00.123456789Z'
    try {
      const journal = await ControlledArtifactExportJournalV2.open(join(root, 'journal'))
      const targetPath = join(root, 'must-not-publish.json')
      const target = await freezeControlledArtifactExportTargetV2(targetPath, journal, () => now)
      const rendererAuthority = new AbortController()
      const underlying = target.createPreparation()
      const prepareRelease: ControlledArtifactReleasePreparationV2 = async (input, signal) => {
        const prepared = await underlying(input, signal)
        return {
          commitBefore: async (authorizedUntil, commitSignal) => {
            rendererAuthority.abort()
            return prepared.commitBefore(authorizedUntil, commitSignal)
          },
          abort: (abortSignal) => prepared.abort(abortSignal)
        }
      }
      const body = Buffer.from('{"bankAccount":"0000000000000000000008"}', 'utf8')
      const fixture = await createHostFixture(
        () => now,
        49,
        undefined,
        body,
        undefined,
        undefined,
        prepareRelease,
        hex('6'),
        target.targetIdentityDigest(),
        rendererAuthority
      )
      const frame = createReleaseFrame(createSignedReceipt(fixture, body), body)
      now = '2026-07-18T12:00:01.123456789Z'

      const released = await post(
        fixture.environment,
        '/v2/controlled-artifacts/release',
        RELEASE_MEDIA_TYPE,
        frame
      )
      expect(released.status).toBe(500)
      await expect(readFile(targetPath)).rejects.toMatchObject({ code: 'ENOENT' })
      expect(await readdir(join(root, 'journal'))).toEqual([])
      await journal.close()
      frame.fill(0)
      body.fill(0)
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})

type HostFixture = Awaited<ReturnType<typeof createHostFixture>>

async function createHostFixture(
  now: () => string,
  backendGeneration: number,
  release?: (body: Buffer) => Promise<void>,
  artifactBody: Buffer = Buffer.from('{"bankAccount":"0000000000000000000000"}', 'utf8'),
  monotonicNow?: () => bigint,
  releaseEffectTimeoutMs?: number,
  prepareReleaseOverride?: ControlledArtifactReleasePreparationV2,
  targetIdentityDigest: string = hex('6'),
  releaseTargetIdentityDigest: string = hex('5'),
  rendererAuthorityController?: AbortController
) {
  const releasedBodies: string[] = []
  let randomCounter = backendGeneration
  const host = new ControlledArtifactHostV2({
    backendGeneration,
    allocationRecordDigest: backendGeneration.toString(16).padStart(64, '0'),
    now,
    monotonicNow,
    releaseEffectTimeoutMs,
    abortEffectTimeoutMs: releaseEffectTimeoutMs,
    random: (size) => {
      const value = Buffer.alloc(size)
      value.writeUInt32BE(++randomCounter, size - 4)
      return value
    },
    acquireRendererLease: (webContentsId, generation) =>
      webContentsId === 5 && generation === 7
        ? {
            signal: (rendererAuthorityController ?? new AbortController()).signal,
            isCurrent: () => !(rendererAuthorityController?.signal.aborted ?? false),
            release: () => undefined
          }
        : null
  })
  runningHosts.push(host)
  await host.start()
  host.activateInvocations()
  const authority = generateKeyPairSync('ed25519')
  const authorityPublicKey = rawEd25519PublicKey(authority.publicKey)
  const context: ControlledArtifactContextBindingV2 = {
    version: 2,
    threadId: 'thread-controlled-v2',
    turnId: 'turn-controlled-v2',
    workspaceRealPath: '/workspace/<case>&\u2028evidence',
    tenantId: 'tenant-controlled-v2',
    userId: 'user-controlled-v2',
    caseId: 'case-controlled-v2',
    caseBindingHash: hex('1'),
    datasetSnapshotId: `dsv2_${hex('2')}`,
    sourceManifestHash: hex('3'),
    contextEpoch: 4,
    contextIssuedAt: '2026-07-18T11:59:59.123456789Z',
    contextDigest: hex('4')
  }
  const authorizedUntil = '2026-07-18T12:10:00.123456789Z'
  const invocation = host.createInvocation({
    context: {
      threadId: context.threadId,
      turnId: context.turnId,
      contextEpoch: context.contextEpoch,
      contextDigest: context.contextDigest
    },
    accessPolicyDigest: hex('a'),
    retentionPolicyDigest: hex('b'),
    deliveryId: hex('7'),
    deliveryOutcomeRecordDigest: hex('8'),
    publicationCommitDigest: hex('9'),
    publicationReceiptDigest: hex('c'),
    piiProjectionDigest: hex('d'),
    piiAuthorizationDigest: hex('e'),
    claimLedgerDigest: hex('f'),
    targetIdentityDigest,
    releaseTargetIdentityDigest,
    artifactSha256: sha256(artifactBody),
    artifactByteLength: artifactBody.length,
    mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
    authorityKeyId: sha256(authorityPublicKey),
    authorityPublicKey: authorityPublicKey.toString('base64url'),
    action: 'display',
    webContentsId: 5,
    rendererGeneration: 7,
    authorizedUntil,
    prepareRelease: prepareReleaseOverride ?? (async ({ body, releaseTargetIdentityDigest }) => {
      const staged = Buffer.from(body)
      let finalized = false
      return {
        commitBefore: async () => {
          try {
            if (release) {
              await release(staged)
            } else {
              releasedBodies.push(staged.toString('utf8'))
            }
            finalized = true
            return {
              releaseTargetIdentityDigest,
              artifactSha256: sha256(artifactBody),
              artifactByteLength: artifactBody.length,
              mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
              committedAt: now()
            }
          } finally {
            staged.fill(0)
          }
        },
        abort: async () => {
          if (!finalized) staged.fill(0)
        }
      }
    })
  })
  const admissionRequest = {
    schemaVersion: 2,
    purpose: 'analytix.controlled-artifact-admission/v2',
    context,
    accessAction: invocation.accessAction,
    controlledHandle: invocation.controlledHandle,
    useSlot: invocation.useSlot,
    rendererPrincipal: invocation.rendererPrincipal,
    rendererGeneration: invocation.rendererGeneration,
    backendGeneration: invocation.backendGeneration
  }
  return {
    host,
    environment: host.runtimeEnvironment(),
    authority,
    authorityPublicKey,
    context,
    targetIdentityDigest,
    releaseTargetIdentityDigest,
    authorizedUntil,
    invocation,
    admissionRequest,
    releasedBodies
  }
}

function createSignedReceipt(fixture: HostFixture, body: Buffer): ControlledArtifactReleaseReceiptV2 {
  const useSlotDigest = digest(SLOT_DOMAIN, fixture.invocation.useSlot)
  const receipt: ControlledArtifactReleaseReceiptV2 = {
    schemaVersion: 2,
    purpose: 'analytix.controlled-artifact-access-receipt/v2',
    accessId: sha256(Buffer.concat([ACCESS_ID_DOMAIN, Buffer.from(useSlotDigest)])),
    context: fixture.context,
    requesterUserId: fixture.context.userId,
    accessAction: fixture.invocation.accessAction,
    controlledHandleDigest: digest(HANDLE_DOMAIN, fixture.invocation.controlledHandle),
    useSlotDigest,
    rendererPrincipalDigest: digest(PRINCIPAL_DOMAIN, fixture.invocation.rendererPrincipal),
    rendererGeneration: fixture.invocation.rendererGeneration,
    backendGeneration: fixture.invocation.backendGeneration,
    accessPolicyDigest: hex('a'),
    retentionPolicyDigest: hex('b'),
    deliveryId: hex('7'),
    deliveryOutcomeRecordDigest: hex('8'),
    publicationCommitDigest: hex('9'),
    publicationReceiptDigest: hex('c'),
    piiProjectionDigest: hex('d'),
    piiAuthorizationDigest: hex('e'),
    claimLedgerDigest: hex('f'),
    targetIdentityDigest: fixture.targetIdentityDigest,
    releaseTargetIdentityDigest: fixture.releaseTargetIdentityDigest,
    artifactSha256: sha256(body),
    artifactByteLength: body.length,
    mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
    requestedAt: '2026-07-18T12:00:01.123456789Z',
    authorizedUntil: fixture.authorizedUntil,
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: sha256(fixture.authorityPublicKey),
    authorityPublicKey: fixture.authorityPublicKey.toString('base64url'),
    authoritySignature: '',
    recordDigest: ''
  }
  resignReceipt(fixture, receipt)
  return receipt
}

function resignReceipt(fixture: HostFixture, receipt: ControlledArtifactReleaseReceiptV2): void {
  receipt.authoritySignature = ''
  receipt.recordDigest = ''
  const signingDigest = createHash('sha256').update(goJSONStringify(receipt)).digest()
  receipt.authoritySignature = sign(null, Buffer.concat([RECEIPT_SIGNING_DOMAIN, signingDigest]),
    fixture.authority.privateKey).toString('base64url')
  receipt.recordDigest = sha256(Buffer.concat([
    RECEIPT_RECORD_DOMAIN,
    Buffer.from(goJSONStringify(receipt))
  ]))
}

function createReleaseFrame(receipt: ControlledArtifactReleaseReceiptV2, body: Buffer): Buffer {
  const metadata = Buffer.from(goJSONStringify({
    schemaVersion: 2,
    purpose: 'analytix.controlled-artifact-release/v2',
    receipt
  }))
  const frame = Buffer.alloc(RELEASE_MAGIC.length + 4 + metadata.length + body.length)
  RELEASE_MAGIC.copy(frame)
  frame.writeUInt32BE(metadata.length, RELEASE_MAGIC.length)
  metadata.copy(frame, RELEASE_MAGIC.length + 4)
  body.copy(frame, RELEASE_MAGIC.length + 4 + metadata.length)
  return frame
}

async function postAdmission(
  environment: ControlledArtifactHostRuntimeEnvironmentV2,
  request: Record<string, unknown>
): Promise<{ status: number; contentType: string; body: Buffer }> {
  return post(
    environment,
    '/v2/controlled-artifacts/admission',
    JSON_MEDIA_TYPE,
    Buffer.from(goJSONStringify(request), 'utf8')
  )
}

async function post(
  environment: ControlledArtifactHostRuntimeEnvironmentV2,
  path: string,
  contentType: string,
  body: Buffer
): Promise<{ status: number; contentType: string; body: Buffer }> {
  const url = new URL(environment[CONTROLLED_ARTIFACT_HOST_V2_URL_ENV])
  const root = new X509Certificate(Buffer.from(
    environment[CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV],
    'base64url'
  ))
  return new Promise((resolve, reject) => {
    const requestOptions: HTTPSRequestOptions & TLSConnectionOptions = {
      protocol: 'https:',
      hostname: '127.0.0.1',
      port: Number(url.port),
      path,
      method: 'POST',
      ca: root.toString(),
      minVersion: 'TLSv1.3',
      maxVersion: 'TLSv1.3',
      ALPNProtocols: ['http/1.1'],
      agent: false,
      headers: {
        Authorization: `Bearer ${environment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV]}`,
        Accept: JSON_MEDIA_TYPE,
        'Content-Type': contentType,
        'Content-Length': String(body.length)
      }
    }
    const request = httpsRequest(requestOptions, (response) => {
      const chunks: Buffer[] = []
      response.on('data', (chunk) => chunks.push(Buffer.from(chunk)))
      response.once('error', reject)
      response.once('end', () => resolve({
        status: response.statusCode ?? 0,
        contentType: String(response.headers['content-type'] ?? ''),
        body: Buffer.concat(chunks)
      }))
    })
    request.once('error', reject)
    request.end(body)
  })
}

function expectedAckMAC(
  environment: ControlledArtifactHostRuntimeEnvironmentV2,
  ack: Record<string, unknown>
): string {
  const values = [
    ack.schemaVersion, ack.purpose, ack.committed, ack.accessId, ack.accessReceiptDigest,
    ack.contextDigest, ack.deliveryId, ack.deliveryOutcomeRecordDigest, ack.publicationCommitDigest,
    ack.releaseTargetIdentityDigest, ack.artifactSha256, ack.releasedByteLength,
    ack.accessAction, ack.controlledHandleDigest,
    ack.useSlotDigest, ack.rendererPrincipalDigest, ack.rendererGeneration,
    ack.backendGeneration, ack.committedAt
  ].map(String)
  const parts = [ACK_DOMAIN]
  for (const value of values) {
    parts.push(Buffer.from(`${Buffer.byteLength(value)}:`), Buffer.from(value))
  }
  return createHmac('sha256', Buffer.from(
    environment[CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV],
    'base64url'
  )).update(Buffer.concat(parts)).digest('base64url')
}

function rawEd25519PublicKey(publicKey: KeyObject): Buffer {
  const der = publicKey.export({ format: 'der', type: 'spki' })
  return Buffer.from(der.subarray(der.length - 32))
}

function digest(domain: Buffer, value: string): string {
  return createHash('sha256').update(domain).update(value).digest('hex')
}

function sha256(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function hex(value: string): string {
  return value.repeat(64)
}

function goJSONStringify(value: unknown): string {
  return JSON.stringify(value)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}
