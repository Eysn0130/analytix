import {
  createHash,
  createHmac,
  createPrivateKey,
  createPublicKey,
  sign,
  type KeyObject
} from 'node:crypto'
import { afterEach, describe, expect, it } from 'vitest'
import {
  CONTROLLED_ARTIFACT_HOST_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_URL_ENV,
  ControlledArtifactHostV1,
  type ControlledArtifactContextBindingV1,
  type ControlledArtifactReleaseReceiptV1
} from './host'

const JSON_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-host+json'
const RELEASE_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-release-v1'
const ADMISSION_PURPOSE = 'analytix.controlled-artifact-admission/v1'
const RELEASE_PURPOSE = 'analytix.controlled-artifact-release/v1'
const RELEASE_ACK_PURPOSE = 'analytix.controlled-artifact-release-ack/v1'
const RELEASE_MAGIC = Buffer.from('ANXCRV1\n', 'ascii')
const ACK_DOMAIN = Buffer.from('analytix.controlled-artifact-release-ack/hmac/v1\0', 'utf8')
const HANDLE_DOMAIN = Buffer.from('analytix.controlled-artifact-access/handle/v1\0', 'utf8')
const SLOT_DOMAIN = Buffer.from('analytix.controlled-artifact-access/use-slot/v1\0', 'utf8')
const PRINCIPAL_DOMAIN = Buffer.from('analytix.controlled-artifact-access/renderer-principal/v1\0', 'utf8')
const ACCESS_ID_DOMAIN = Buffer.from('analytix.controlled-artifact-access/id/v1\0', 'utf8')
const RECEIPT_SIGNATURE_DOMAIN = Buffer.from('analytix.controlled-artifact-access-receipt/signature/v1\0', 'utf8')
const RECEIPT_RECORD_DOMAIN = Buffer.from('analytix.controlled-artifact-access-receipt/record/v1\0', 'utf8')
const ED25519_PKCS8_PREFIX = Buffer.from('302e020100300506032b657004220420', 'hex')
const ACCOUNT = '6222020202020202020'

type TestAuthority = { privateKey: KeyObject; publicKey: Buffer; keyId: string; publicKeyText: string }

const TEST_AUTHORITY = testAuthority(0x31)
const OTHER_AUTHORITY = testAuthority(0x57)

const runningHosts: ControlledArtifactHostV1[] = []

afterEach(async () => {
  await Promise.all(runningHosts.splice(0).map((host) => host.stop()))
})

describe('ControlledArtifactHostV1', () => {
  it('admits the exact main-frame generation and releases full PII once with an authenticated ack', async () => {
    const clock = { value: new Date('2026-07-16T12:00:00Z') }
    let rendererValid = true
    const releases: Buffer[] = []
    const host = new ControlledArtifactHostV1({
      validateRenderer: (webContentsId, generation) => rendererValid && webContentsId === 41 && generation === 7,
      now: () => new Date(clock.value),
      randomToken: deterministicTokenFactory()
    })
    runningHosts.push(host)
    await host.start()
    const context = contextFixture()
    const invocation = host.createInvocation({
      context,
      publicationCommitDigest: digest('publication-commit'),
      ...authorityInvocationBinding(),
      action: 'export',
      webContentsId: 41,
      rendererGeneration: 7,
      authorizedUntil: new Date('2026-07-16T12:10:00Z'),
      release: async ({ body }) => { releases.push(Buffer.from(body)) }
    })
    expect(Object.keys(invocation)).toEqual([])
    expect(() => JSON.stringify(invocation)).toThrow('controlled artifact invocation is main-process-only')
    const environment = host.runtimeEnvironment()
    const admission = await post(environment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(admission.response.status).toBe(200)
    expect(admission.body).toMatchObject({
      schemaVersion: 1,
      purpose: ADMISSION_PURPOSE,
      rendererGeneration: 7,
      backendGeneration: invocation.backendGeneration,
      publicationCommitDigest: digest('publication-commit'),
      authorizedUntil: '2026-07-16T12:10:00Z'
    })
    const artifact = Buffer.from(`{"schemaVersion":1,"exactValue":"${ACCOUNT}"}`, 'utf8')
    const receipt = receiptFixture(context, invocation, artifact, digest('publication-commit'), '2026-07-16T12:10:00Z')
    clock.value = new Date('2026-07-16T12:01:00Z')
    const release = await postRaw(environment, '/v1/controlled-artifacts/release', releaseFrame(receipt, artifact), RELEASE_MEDIA_TYPE)
    expect(release.response.status).toBe(200)
    expect(releases).toHaveLength(1)
    expect(releases[0].equals(artifact)).toBe(true)
    expect(releases[0].includes(Buffer.from(ACCOUNT))).toBe(true)
    expect(JSON.stringify(release.body)).not.toContain(ACCOUNT)
    expect(release.body).toMatchObject({
      committed: true,
      accessId: receipt.accessId,
      accessReceiptDigest: receipt.recordDigest,
      artifactSha256: receipt.artifactSha256,
      releasedByteLength: artifact.length,
      rendererGeneration: invocation.rendererGeneration,
      backendGeneration: invocation.backendGeneration,
      committedAt: '2026-07-16T12:01:00Z'
    })
    expect(release.body.mac).toBe(releaseAckMAC(environment[CONTROLLED_ARTIFACT_HOST_TOKEN_ENV], release.body))

    const postReleaseAdmission = await post(environment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(postReleaseAdmission.response.status).toBe(200)
    const duplicate = await postRaw(environment, '/v1/controlled-artifacts/release', releaseFrame(receipt, artifact), RELEASE_MEDIA_TYPE)
    expect(duplicate.response.status).toBe(409)
    expect(releases).toHaveLength(1)

    rendererValid = false
    const stale = await post(environment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(stale.response.status).toBe(409)
  })

  it('rejects duplicate keys, a wrong bearer, stale renderer authority, and invocation replay after restart', async () => {
    const clock = { value: new Date('2026-07-16T12:00:00Z') }
    let rendererValid = true
    const tokens = deterministicTokenFactory()
    const host = new ControlledArtifactHostV1({
      validateRenderer: () => rendererValid,
      now: () => new Date(clock.value),
      randomToken: tokens
    })
    runningHosts.push(host)
    await host.start()
    const context = contextFixture()
    const invocation = host.createInvocation({
      context,
      publicationCommitDigest: digest('restart-commit'),
      ...authorityInvocationBinding(),
      action: 'display',
      webContentsId: 9,
      rendererGeneration: 3,
      authorizedUntil: new Date('2026-07-16T12:05:00Z'),
      release: async () => undefined
    })
    const firstEnvironment = host.runtimeEnvironment()
    const duplicateBody = Buffer.from(JSON.stringify(admissionRequest(context, invocation)).replace('{', '{"schemaVersion":1,'), 'utf8')
    const duplicate = await postRaw(firstEnvironment, '/v1/controlled-artifacts/admission', duplicateBody, JSON_MEDIA_TYPE)
    expect(duplicate.response.status).toBe(400)

    const wrongSecret = {
      ...firstEnvironment,
      [CONTROLLED_ARTIFACT_HOST_TOKEN_ENV]: Buffer.alloc(32, 0xaa).toString('base64url')
    }
    const unauthorized = await post(wrongSecret, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(unauthorized.response.status).toBe(401)

    rendererValid = false
    const revoked = await post(firstEnvironment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(revoked.response.status).toBe(409)
    rendererValid = true
    host.revokeRenderer(9, 3)
    const explicitlyRevoked = await post(firstEnvironment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(explicitlyRevoked.response.status).toBe(409)

    await host.stop()
    await host.start()
    const secondEnvironment = host.runtimeEnvironment()
    expect(secondEnvironment[CONTROLLED_ARTIFACT_HOST_TOKEN_ENV]).not.toBe(firstEnvironment[CONTROLLED_ARTIFACT_HOST_TOKEN_ENV])
    const replay = await post(secondEnvironment, '/v1/controlled-artifacts/admission', admissionRequest(context, invocation))
    expect(replay.response.status).toBe(409)
  })

  it('marks a failed side effect non-retryable and never returns protected bytes or target data', async () => {
    const clock = { value: new Date('2026-07-16T12:00:00Z') }
    let calls = 0
    const host = new ControlledArtifactHostV1({
      validateRenderer: () => true,
      now: () => new Date(clock.value),
      randomToken: deterministicTokenFactory()
    })
    runningHosts.push(host)
    await host.start()
    const context = contextFixture()
    const invocation = host.createInvocation({
      context,
      publicationCommitDigest: digest('failed-effect-commit'),
      ...authorityInvocationBinding(),
      action: 'export',
      webContentsId: 11,
      rendererGeneration: 5,
      authorizedUntil: new Date('2026-07-16T12:05:00Z'),
      release: async () => {
        calls += 1
        throw new Error(`must-not-leak-${ACCOUNT}`)
      }
    })
    const environment = host.runtimeEnvironment()
    const artifact = Buffer.from(`{"exactValue":"${ACCOUNT}"}`, 'utf8')
    const receipt = receiptFixture(context, invocation, artifact, digest('failed-effect-commit'), '2026-07-16T12:05:00Z')
    const first = await postRaw(environment, '/v1/controlled-artifacts/release', releaseFrame(receipt, artifact), RELEASE_MEDIA_TYPE)
    const second = await postRaw(environment, '/v1/controlled-artifacts/release', releaseFrame(receipt, artifact), RELEASE_MEDIA_TYPE)
    expect(first.response.status).toBe(500)
    expect(second.response.status).toBe(409)
    expect(calls).toBe(1)
    expect(JSON.stringify(first.body)).not.toContain(ACCOUNT)
    expect(JSON.stringify(second.body)).not.toContain(ACCOUNT)
  })

  it('rejects a valid self-signed receipt from any authority other than the frozen installation key', async () => {
    let calls = 0
    const host = new ControlledArtifactHostV1({
      validateRenderer: () => true,
      now: () => new Date('2026-07-16T12:00:00Z'),
      randomToken: deterministicTokenFactory()
    })
    runningHosts.push(host)
    await host.start()
    const context = contextFixture()
    const publicationCommitDigest = digest('authority-pinned-commit')
    const invocation = host.createInvocation({
      context,
      publicationCommitDigest,
      ...authorityInvocationBinding(),
      action: 'export',
      webContentsId: 15,
      rendererGeneration: 8,
      authorizedUntil: new Date('2026-07-16T12:05:00Z'),
      release: async () => { calls += 1 }
    })
    const artifact = Buffer.from(`{"exactValue":"${ACCOUNT}"}`, 'utf8')
    const forged = receiptFixture(
      context,
      invocation,
      artifact,
      publicationCommitDigest,
      '2026-07-16T12:05:00Z',
      OTHER_AUTHORITY
    )
    const forgedResponse = await postRaw(
      host.runtimeEnvironment(),
      '/v1/controlled-artifacts/release',
      releaseFrame(forged, artifact),
      RELEASE_MEDIA_TYPE
    )
    expect(forgedResponse.response.status).toBe(409)
    const malformed = receiptFixture(
      context,
      invocation,
      artifact,
      publicationCommitDigest,
      '2026-07-16T12:05:00Z'
    )
    malformed.authoritySignature = Buffer.alloc(64, 0x7f).toString('base64url')
    const malformedResponse = await postRaw(
      host.runtimeEnvironment(),
      '/v1/controlled-artifacts/release',
      releaseFrame(malformed, artifact),
      RELEASE_MEDIA_TYPE
    )
    expect(malformedResponse.response.status).toBe(400)
    expect(calls).toBe(0)
    expect(JSON.stringify(forgedResponse.body)).not.toContain(ACCOUNT)
    expect(JSON.stringify(malformedResponse.body)).not.toContain(ACCOUNT)
  })
})

function contextFixture(): ControlledArtifactContextBindingV1 {
  return {
    threadId: 'thread-controlled-artifact',
    turnId: 'turn-controlled-artifact',
    caseId: 'case-controlled-artifact',
    caseBindingHash: digest('case-binding'),
    datasetSnapshotId: `dsv1_${digest('dataset')}`,
    sourceManifestHash: digest('source-manifest'),
    contextEpoch: 4,
    contextDigest: digest('context')
  }
}

function admissionRequest(
  context: ControlledArtifactContextBindingV1,
  invocation: ReturnType<ControlledArtifactHostV1['createInvocation']>
): Record<string, unknown> {
  return {
    schemaVersion: 1,
    purpose: ADMISSION_PURPOSE,
    ...context,
    accessAction: invocation.accessAction,
    controlledHandle: invocation.controlledHandle,
    useSlot: invocation.useSlot,
    rendererPrincipal: invocation.rendererPrincipal,
    rendererGeneration: invocation.rendererGeneration,
    backendGeneration: invocation.backendGeneration
  }
}

function receiptFixture(
  context: ControlledArtifactContextBindingV1,
  invocation: ReturnType<ControlledArtifactHostV1['createInvocation']>,
  artifact: Buffer,
  publicationCommitDigest: string,
  authorizedUntil: string,
  authority: TestAuthority = TEST_AUTHORITY
): ControlledArtifactReleaseReceiptV1 {
  const controlledHandleDigest = opaqueDigest(HANDLE_DOMAIN, invocation.controlledHandle)
  const useSlotDigest = opaqueDigest(SLOT_DOMAIN, invocation.useSlot)
  const rendererPrincipalDigest = opaqueDigest(PRINCIPAL_DOMAIN, invocation.rendererPrincipal)
  const receipt: ControlledArtifactReleaseReceiptV1 = {
    schemaVersion: 1,
    purpose: 'analytix.controlled-artifact-access-receipt/v1',
    accessId: createHash('sha256').update(ACCESS_ID_DOMAIN).update(useSlotDigest, 'utf8').digest('hex'),
    context: {
      version: 2,
      threadId: context.threadId,
      turnId: context.turnId,
      workspaceRealPath: '/workspace/controlled-artifact',
      tenantId: 'local',
      userId: 'local',
      caseId: context.caseId,
      caseBindingHash: context.caseBindingHash,
      datasetSnapshotId: context.datasetSnapshotId,
      sourceManifestHash: context.sourceManifestHash,
      contextEpoch: context.contextEpoch,
      contextIssuedAt: '2026-07-16T11:59:00Z',
      contextDigest: context.contextDigest
    },
    requesterUserId: 'local',
    accessAction: invocation.accessAction,
    controlledHandleDigest,
    useSlotDigest,
    rendererPrincipalDigest,
    rendererGeneration: invocation.rendererGeneration,
    backendGeneration: invocation.backendGeneration,
    accessPolicyDigest: digest('access-policy'),
    retentionPolicyDigest: digest('retention-policy'),
    publicationCommitDigest,
    publicationReceiptDigest: digest('publication-receipt'),
    piiProjectionDigest: digest('pii-projection'),
    piiAuthorizationDigest: digest('pii-authorization'),
    claimLedgerDigest: digest('claim-ledger'),
    targetIdentityDigest: digest('target-identity'),
    artifactSha256: sha256(artifact),
    artifactByteLength: artifact.length,
    mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
    requestedAt: '2026-07-16T12:00:00Z',
    authorizedUntil,
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: authority.keyId,
    authorityPublicKey: authority.publicKeyText,
    authoritySignature: '',
    recordDigest: ''
  }
  const signingRecord = { ...receipt, authoritySignature: '', recordDigest: '' }
  const signingDigest = createHash('sha256').update(goJSONStringify(signingRecord), 'utf8').digest()
  receipt.authoritySignature = sign(
    null,
    Buffer.concat([RECEIPT_SIGNATURE_DOMAIN, signingDigest]),
    authority.privateKey
  ).toString('base64url')
  receipt.recordDigest = sha256(Buffer.concat([
    RECEIPT_RECORD_DOMAIN,
    Buffer.from(goJSONStringify({ ...receipt, recordDigest: '' }), 'utf8')
  ]))
  return receipt
}

function releaseFrame(receipt: ControlledArtifactReleaseReceiptV1, body: Buffer): Buffer {
  const metadata = Buffer.from(JSON.stringify({ schemaVersion: 1, purpose: RELEASE_PURPOSE, receipt }), 'utf8')
  const length = Buffer.alloc(4)
  length.writeUInt32BE(metadata.length)
  return Buffer.concat([RELEASE_MAGIC, length, metadata, body])
}

async function post(
  environment: Record<string, string>,
  path: string,
  value: unknown
): Promise<{ response: Response; body: Record<string, unknown> }> {
  return postRaw(environment, path, Buffer.from(JSON.stringify(value), 'utf8'), JSON_MEDIA_TYPE)
}

async function postRaw(
  environment: Record<string, string>,
  path: string,
  body: Buffer,
  contentType: string
): Promise<{ response: Response; body: Record<string, unknown> }> {
  const response = await fetch(`${environment[CONTROLLED_ARTIFACT_HOST_URL_ENV]}${path}`, {
    method: 'POST',
    redirect: 'manual',
    headers: {
      Authorization: `Bearer ${environment[CONTROLLED_ARTIFACT_HOST_TOKEN_ENV]}`,
      Accept: JSON_MEDIA_TYPE,
      'Content-Type': contentType,
      'Content-Length': String(body.length)
    },
    body: body as unknown as BodyInit
  })
  const value = await response.json() as Record<string, unknown>
  return { response, body: value }
}

function releaseAckMAC(secret: string, ack: Record<string, unknown>): string {
  const values = [
    String(ack.schemaVersion), String(ack.purpose), String(ack.committed), String(ack.accessId),
    String(ack.accessReceiptDigest), String(ack.artifactSha256), String(ack.releasedByteLength),
    String(ack.accessAction), String(ack.controlledHandleDigest), String(ack.useSlotDigest),
    String(ack.rendererPrincipalDigest), String(ack.rendererGeneration), String(ack.backendGeneration),
    String(ack.committedAt)
  ]
  const hmac = createHmac('sha256', Buffer.from(secret, 'base64url')).update(ACK_DOMAIN)
  for (const value of values) hmac.update(`${Buffer.byteLength(value, 'utf8')}:`, 'ascii').update(value, 'utf8')
  return hmac.digest('base64url')
}

function deterministicTokenFactory(): () => string {
  let value = 0
  return () => Buffer.alloc(32, ++value).toString('base64url')
}

function authorityInvocationBinding(): { authorityKeyId: string; authorityPublicKey: string } {
  return { authorityKeyId: TEST_AUTHORITY.keyId, authorityPublicKey: TEST_AUTHORITY.publicKeyText }
}

function testAuthority(seedByte: number): TestAuthority {
  const privateKey = createPrivateKey({
    key: Buffer.concat([ED25519_PKCS8_PREFIX, Buffer.alloc(32, seedByte)]),
    format: 'der',
    type: 'pkcs8'
  })
  const publicDER = createPublicKey(privateKey).export({ format: 'der', type: 'spki' })
  const publicKey = Buffer.from(publicDER).subarray(-32)
  return {
    privateKey,
    publicKey,
    keyId: sha256(publicKey),
    publicKeyText: publicKey.toString('base64url')
  }
}

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('test receipt is not serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function opaqueDigest(domain: Buffer, value: string): string {
  return createHash('sha256').update(domain).update(value, 'utf8').digest('hex')
}

function digest(value: string): string {
  return createHash('sha256').update(value, 'utf8').digest('hex')
}

function sha256(body: Buffer): string {
  return createHash('sha256').update(body).digest('hex')
}
