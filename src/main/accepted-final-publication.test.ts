import { createHash, generateKeyPairSync, sign, type KeyObject } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  acceptedFinalPublicationEventId,
  acceptedFinalPublicationPayloadDigest,
  verifyAcceptedFinalRecordIntegrity,
  verifiedAcceptedFinalDeliverySealAuthority,
  verifiedAcceptedFinalItemCompletedEvent
} from './accepted-final-publication'

const SIGNATURE_DOMAIN = Buffer.from('analytix.final-answer-authority/v1\0')
const PUBLIC_VIEW_DOMAIN = Buffer.from('analytix.accepted-final-public-view/v2\0')

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('value is not JSON serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function signAcceptedFinalRecord(unsignedRecord: Record<string, unknown>, privateKey: KeyObject): Record<string, unknown> {
  const signingDigest = createHash('sha256').update(goJSONStringify(unsignedRecord)).digest()
  const authoritySignature = sign(
    null,
    Buffer.concat([SIGNATURE_DOMAIN, signingDigest]),
    privateKey
  ).toString('base64url')
  const signedRecord = { ...unsignedRecord, authoritySignature }
  return { ...signedRecord, recordDigest: sha256(goJSONStringify(signedRecord)) }
}

function digestPublicViewCore(core: Record<string, unknown>): string {
  return sha256(Buffer.concat([PUBLIC_VIEW_DOMAIN, Buffer.from(goJSONStringify(core))]))
}

function expandPublicViewV2(
  record: Record<string, any>,
  core: Record<string, any>,
  publicViewDigest: string
): Record<string, unknown> {
  const { schemaVersion, publicationState, ...fields } = core
  return {
    schemaVersion,
    publicationState,
    acceptedFinalDigest: record.recordDigest,
    publicViewDigest,
    ...fields
  }
}

function signedAcceptedFinalEvent(text = '  verified final\n', historicalFact = false): Record<string, unknown> {
  const authority = generateKeyPairSync('ed25519')
  const publicKeyDER = authority.publicKey.export({ format: 'der', type: 'spki' }) as Buffer
  const publicKey = publicKeyDER.subarray(publicKeyDER.length - 32)
  const acceptedAt = '2026-07-18T01:02:03Z'
  const authorityFields = {
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: sha256(publicKey),
    authorityPublicKey: publicKey.toString('base64url'),
    threadId: 'thread-1',
    turnId: 'turn-1',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-3',
    variant: historicalFact ? 'EvidenceBackedAnswer' : 'GeneralGuidanceAnswer',
    terminalReason: 'success',
    renderedTextSha256: sha256(text),
    registrySequence: 0,
    registryStateDigest: 'd'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1'
  }

  if (historicalFact) {
    const historicalRecord = signAcceptedFinalRecord({
      schemaVersion: 3,
      ...authorityFields,
      finalGateVersion: 'analytix.final-evidence-gate/v2',
      verifierVersion: 'analytix.claim-verifier-policy/v1',
      publicationSnapshotProofDigest: '1'.repeat(64),
      privateRecordDigest: 'e'.repeat(64),
      acceptedAt,
      authoritySignature: '',
      recordDigest: ''
    }, authority.privateKey) as any
    const historicalView = {
      schemaVersion: 1,
      publicationState: 'accepted',
      acceptedFinalDigest: historicalRecord.recordDigest,
      envelopeDigest: historicalRecord.envelopeDigest,
      contextDigest: historicalRecord.contextDigest,
      contextEpoch: historicalRecord.contextEpoch,
      datasetSnapshotId: historicalRecord.datasetSnapshotId,
      variant: historicalRecord.variant,
      terminalReason: historicalRecord.terminalReason,
      blockerCode: '',
      coverageStatus: 'complete',
      checkedScopeDigest: '2'.repeat(64),
      missingScopeCount: 0,
      claimCount: 1,
      claimTypes: ['amount'],
      receiptMetadata: {
        projection: 'masked_metadata_only', count: 1, setDigest: 'f'.repeat(64),
        citations: [{ handle: `cite_${'3'.repeat(64)}`, label: 'evidence-1' }]
      },
      noHitWording: '', envelopeIssuedAt: acceptedAt, acceptedAt
    }
    return acceptedFinalEvent(historicalRecord, historicalView, text, acceptedAt)
  }

  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest: authorityFields.envelopeDigest,
    contextDigest: authorityFields.contextDigest,
    contextEpoch: authorityFields.contextEpoch,
    datasetSnapshotId: authorityFields.datasetSnapshotId,
    variant: authorityFields.variant,
    terminalReason: authorityFields.terminalReason,
    blockerCode: '',
    coverageStatus: 'guidance_only',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: 'f'.repeat(64),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: acceptedAt,
    acceptedAt
  }
  const publicViewDigest = digestPublicViewCore(publicView)
  const record = signAcceptedFinalRecord({
    schemaVersion: 5,
    ...authorityFields,
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest,
    privateRecordDigest: 'e'.repeat(64),
    acceptedAt,
    authoritySignature: '',
    recordDigest: ''
  }, authority.privateKey) as any
  return acceptedFinalEvent(record, expandPublicViewV2(record, publicView, publicViewDigest), text, acceptedAt)
}

function acceptedFinalEvent(
  record: Record<string, any>,
  view: Record<string, unknown>,
  text: string,
  acceptedAt: string
): Record<string, unknown> {
  const itemId = 'item-turn-1-assistant'
  const event: Record<string, unknown> = {
    kind: 'item_completed',
    seq: 2,
    timestamp: acceptedAt,
    threadId: record.threadId,
    turnId: record.turnId,
    itemId,
    item: {
      id: itemId,
      turnId: record.turnId,
      threadId: record.threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedAt,
      finishedAt: acceptedAt,
      kind: 'assistant_text',
      text,
      acceptedFinal: record,
      acceptedFinalView: view
    },
    acceptedFinalDigest: record.recordDigest,
    publicationCommitId: record.recordDigest,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, 'assistant-final'),
    publicationSlot: 'assistant-final'
  }
  event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
  return event
}

function authorityPinForEvent(event: Record<string, unknown>): { keyId: string, publicKey: string } {
  const record = (event.item as Record<string, any>).acceptedFinal
  return { keyId: record.authorityKeyId, publicKey: record.authorityPublicKey }
}

function signedWitnessedFactEvent(
  text = '经核验，涉案金额为 4200000 元。',
  invalidDigest: 'none' | 'binding' | 'admission' = 'none'
): Record<string, unknown> {
  const authority = generateKeyPairSync('ed25519')
  const publicKeyDER = authority.publicKey.export({ format: 'der', type: 'spki' }) as Buffer
  const publicKey = publicKeyDER.subarray(publicKeyDER.length - 32)
  const authorityKeyId = sha256(publicKey)
  const acceptedAt = '2026-07-18T01:02:03Z'
  const contextDigest = 'c'.repeat(64)
  const envelopeDigest = 'b'.repeat(64)
  const renderedTextSha256 = sha256(text)
  const publicationSnapshotProofDigest = '1'.repeat(64)
  const registryStateDigest = 'd'.repeat(64)
  const evidenceAuthorityBundleDigest = '4'.repeat(64)
  const evidenceAuthorityBundleGeneration = 5
  const witnessBinding = {
    schemaVersion: 1,
    purpose: 'analytix.evidence-authority-witness-binding/v1',
    installationId: '6'.repeat(64),
    enrollmentId: '7'.repeat(64),
    namespace: 'analytix.evidence-registry-authority/v1',
    authorityKeyId,
    witnessKeyId: '8'.repeat(64),
    bundleRecordDigest: evidenceAuthorityBundleDigest,
    bundleGeneration: evidenceAuthorityBundleGeneration,
    observeRequestDigest: '9'.repeat(64),
    checkpointDigest: '0'.repeat(64),
    observationDigest: '1'.repeat(64),
    bindingDigest: ''
  }
  witnessBinding.bindingDigest = sha256(goJSONStringify(witnessBinding))
  if (invalidDigest === 'binding') witnessBinding.bindingDigest = 'f'.repeat(64)

  const factFinalWitnessAdmission = {
    schemaVersion: 1,
    purpose: 'analytix.fact-final-witness-admission/v1',
    contextDigest,
    datasetSnapshotId: 'snapshot-3',
    sourceManifestHash: '2'.repeat(64),
    envelopeDigest,
    renderedTextSha256,
    publicationSnapshotProofDigest,
    registrySequence: 3,
    registryStateDigest,
    evidenceReceiptIdsDigest: '3'.repeat(64),
    evidenceReceiptCount: 1,
    evidenceAuthorityBundleDigest,
    evidenceAuthorityBundleGeneration,
    evidenceRegistryIndexDigest: '5'.repeat(64),
    evidenceRegistryCount: 3,
    selectedRegistryIndexDigest: '4'.repeat(64),
    selectedRegistryIndexGeneration: 2,
    selectedRegistryCapsuleDigest: '6'.repeat(64),
    witnessBinding,
    admittedAt: '2026-07-18T01:02:02.5Z',
    admissionDigest: ''
  }
  factFinalWitnessAdmission.admissionDigest = sha256(goJSONStringify(factFinalWitnessAdmission))
  if (invalidDigest === 'admission') factFinalWitnessAdmission.admissionDigest = 'e'.repeat(64)

  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest,
    contextDigest,
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-3',
    variant: 'EvidenceBackedAnswer',
    terminalReason: 'success',
    blockerCode: '',
    coverageStatus: 'complete',
    checkedScopeDigest: '2'.repeat(64),
    missingScopeCount: 0,
    claimCount: 1,
    claimTypes: ['amount'],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 1,
      setDigest: 'f'.repeat(64),
      citations: [{ handle: `cite_${'3'.repeat(64)}`, label: 'evidence-1' }]
    },
    noHitWording: '',
    envelopeIssuedAt: acceptedAt,
    acceptedAt
  }
  const publicViewDigest = digestPublicViewCore(publicView)
  const unsignedRecord = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId,
    authorityPublicKey: publicKey.toString('base64url'),
    threadId: 'thread-1',
    turnId: 'turn-1',
    envelopeDigest,
    contextDigest,
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-3',
    variant: 'EvidenceBackedAnswer',
    terminalReason: 'success',
    renderedTextSha256,
    registrySequence: 3,
    registryStateDigest,
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest,
    publicationSnapshotProofDigest,
    factFinalWitnessAdmission,
    privateRecordDigest: 'e'.repeat(64),
    acceptedAt,
    authoritySignature: '',
    recordDigest: ''
  }
  const record = signAcceptedFinalRecord(unsignedRecord, authority.privateKey) as any
  return acceptedFinalEvent(record, expandPublicViewV2(record, publicView, publicViewDigest), text, acceptedAt)
}

describe('accepted final publication authority', () => {
  it('verifies the exact Go-issued V5 record with a strict V2 fact-final witness admission', () => {
    const record = JSON.parse(readFileSync(new URL(
      '../../packages/runtime-go/internal/domain/evidence/testdata/accepted-final-v5-fact-witness-admission-v2.json',
      import.meta.url
    ), 'utf8')) as Record<string, any>
    const pin = {
      keyId: record.authorityKeyId as string,
      publicKey: record.authorityPublicKey as string
    }
    expect(record.factFinalWitnessAdmission).toMatchObject({
      schemaVersion: 2,
      purpose: 'analytix.fact-final-witness-admission/v2'
    })
    expect(record.publicationSnapshotProofDigest).toMatch(/^[a-f0-9]{64}$/)
    expect(verifyAcceptedFinalRecordIntegrity(record, pin)).toBe(true)

    const admission = record.factFinalWitnessAdmission as Record<string, any>
    for (const required of [
      'datasetSnapshotIndexDigest',
      'datasetSnapshotCount',
      'selectedDatasetSnapshotIndexDigest',
      'selectedDatasetSnapshotIndexGeneration',
      'selectedDatasetSnapshotRecordDigest',
      'datasetSnapshotManifestDigest',
      'fundsProducerContentId',
      'fundsProducerContentManifestSha256'
    ]) {
      expect(verifyAcceptedFinalRecordIntegrity({
        ...record,
        factFinalWitnessAdmission: { ...admission, [required]: undefined }
      }, pin)).toBe(false)
    }

    for (const field of [
      'datasetSnapshotIndexDigest',
      'selectedDatasetSnapshotIndexDigest',
      'selectedDatasetSnapshotRecordDigest',
      'datasetSnapshotManifestDigest',
      'fundsProducerContentManifestSha256'
    ]) {
      expect(verifyAcceptedFinalRecordIntegrity({
        ...record,
        factFinalWitnessAdmission: { ...admission, [field]: '0'.repeat(64) }
      }, pin)).toBe(false)
    }

    for (const privateField of [
      'publicationSnapshotProof',
      'securityContext',
      'publicationIntent',
      'storeDigest',
      'registryHead',
      'rawProviderBody',
      'sourceExactValue',
      'privatePII'
    ]) {
      expect(verifyAcceptedFinalRecordIntegrity({
        ...record,
        [privateField]: { privateSentinel: true }
      }, pin)).toBe(false)
    }

    for (const hostileAdmission of [
      {
        ...admission,
        schemaVersion: 1,
        purpose: 'analytix.fact-final-witness-admission/v1'
      },
      { ...admission, schemaVersion: 2, purpose: 'analytix.fact-final-witness-admission/v1' },
      { ...admission, schemaVersion: 3 },
      { ...admission, datasetSnapshotId: `dsv2_${'0'.repeat(64)}` },
      { ...admission, fundsProducerContentId: `fpc2_${'0'.repeat(64)}` },
      { ...admission, selectedRegistryIndexGeneration: admission.evidenceRegistryCount + 1 },
      { ...admission, selectedDatasetSnapshotIndexGeneration: admission.datasetSnapshotCount + 1 },
      { ...admission, datasetSnapshotCount: Number.MAX_SAFE_INTEGER + 1 },
      { ...admission, unknownAuthorityField: 'private' },
      { ...admission, admissionDigest: '0'.repeat(64) },
      {
        ...admission,
        witnessBinding: { ...admission.witnessBinding, bindingDigest: '0'.repeat(64) }
      },
      {
        ...admission,
        witnessBinding: { ...admission.witnessBinding, unknownAuthorityField: 'private' }
      }
    ]) {
      expect(verifyAcceptedFinalRecordIntegrity({
        ...record,
        factFinalWitnessAdmission: hostileAdmission
      }, pin)).toBe(false)
    }
  })

  it('verifies the byte-exact Go-generated accepted-final delivery seal golden', () => {
    const pin = {
      keyId: '56475aa75463474c0285df5dbf2bcab73da651358839e9b77481b2eab107708c',
      publicKey: 'A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg'
    }
    const seal = {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: '0e791187f9378e42889c94b07aaf1f10f68c51ba7f226973dc8b73e8950518f9',
      threadId: 'golden-thread',
      turnId: 'golden-turn',
      publicationCommitId: sha256('golden-commit'),
      acceptedFinalDispositionDigest: sha256('golden-accepted-disposition'),
      terminalDispositionId: sha256('golden-terminal-disposition'),
      eventManifestDigest: sha256('golden-manifest'),
      sequencedEventsDigest: sha256('golden-sequenced-events'),
      batchId: sha256('golden-batch'),
      firstSeq: 41,
      lastSeq: 43,
      timestamp: '2026-07-18T01:02:03.456789Z',
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: pin.keyId,
      authorityPublicKey: pin.publicKey,
      authoritySignature: 'HYSwxE8d_aMP_8BV1_JTYtnZ9PFw1mI9vnSfPLSuHlJQImI0WNOVD5OuZNVzJHFk0Ngz1JEe_UxId9c9-lE9DQ'
    }

    expect(verifiedAcceptedFinalDeliverySealAuthority(seal, pin)).toEqual(seal)
    expect(verifiedAcceptedFinalDeliverySealAuthority({ ...seal, firstSeq: 40 }, pin)).toBeNull()
    expect(verifiedAcceptedFinalDeliverySealAuthority({ ...seal, extra: true }, pin)).toBeNull()
    expect(verifiedAcceptedFinalDeliverySealAuthority(seal, {
      keyId: '1'.repeat(64),
      publicKey: pin.publicKey
    })).toBeNull()
    expect(verifiedAcceptedFinalDeliverySealAuthority(seal, null)).toBeNull()
  })

  it('admits a byte-exact host-signed final and rejects every mutated authority surface', () => {
    const event = signedAcceptedFinalEvent()
    const pin = authorityPinForEvent(event)
    expect(verifiedAcceptedFinalItemCompletedEvent(event, pin)?.item.text).toBe('  verified final\n')

    for (const mutation of [
      (copy: any) => { copy.item.text = 'mutated' },
      (copy: any) => { copy.item.acceptedFinal.authoritySignature = 'A'.repeat(86) },
      (copy: any) => { copy.item.acceptedFinal.recordDigest = '0'.repeat(64) },
      (copy: any) => { copy.publicationEventId = '1'.repeat(64) },
      (copy: any) => { copy.publicationPayloadDigest = '2'.repeat(64) },
      (copy: any) => { copy.item.text = '<think>private</think>public' }
    ]) {
      const copy = structuredClone(event)
      mutation(copy)
      expect(verifiedAcceptedFinalItemCompletedEvent(copy, pin)).toBeNull()
    }
  })

  it('rejects detached, rehashed, and synchronously rewritten public views', () => {
    const original = signedAcceptedFinalEvent()
    const pin = authorityPinForEvent(original)

    const detached = structuredClone(original) as any
    detached.item.acceptedFinalView.receiptMetadata.setDigest = '9'.repeat(64)
    detached.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(detached)
    expect(verifiedAcceptedFinalItemCompletedEvent(detached, pin)).toBeNull()

    const coreChanged = structuredClone(original) as any
    coreChanged.item.acceptedFinal.publicView.receiptMetadata.setDigest = '9'.repeat(64)
    coreChanged.item.acceptedFinalView.receiptMetadata.setDigest = '9'.repeat(64)
    coreChanged.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(coreChanged)
    expect(verifiedAcceptedFinalItemCompletedEvent(coreChanged, pin)).toBeNull()

    const rehashed = structuredClone(original) as any
    const changedCore = rehashed.item.acceptedFinal.publicView
    changedCore.receiptMetadata.setDigest = '9'.repeat(64)
    rehashed.item.acceptedFinal.publicViewDigest = digestPublicViewCore(changedCore)
    rehashed.item.acceptedFinalView = expandPublicViewV2(
      rehashed.item.acceptedFinal,
      changedCore,
      rehashed.item.acceptedFinal.publicViewDigest
    )
    rehashed.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(rehashed)
    expect(verifiedAcceptedFinalItemCompletedEvent(rehashed, pin)).toBeNull()
  })

  it('admits witnessed v5 facts, rejects historical v3 facts, and validates nested authority digests', () => {
    const witnessed = signedWitnessedFactEvent()
    const pin = authorityPinForEvent(witnessed)
    expect(verifiedAcceptedFinalItemCompletedEvent(witnessed, pin)?.item.text).toContain('4200000')
    expect(verifyAcceptedFinalRecordIntegrity((witnessed as any).item.acceptedFinal, pin)).toBe(true)

    const historical = signedAcceptedFinalEvent('historical v3 fact', true)
    expect(verifiedAcceptedFinalItemCompletedEvent(
      historical, authorityPinForEvent(historical)
    )).toBeNull()
    const invalidBinding = signedWitnessedFactEvent('fact', 'binding')
    const invalidAdmission = signedWitnessedFactEvent('fact', 'admission')
    expect(verifiedAcceptedFinalItemCompletedEvent(invalidBinding, authorityPinForEvent(invalidBinding))).toBeNull()
    expect(verifiedAcceptedFinalItemCompletedEvent(invalidAdmission, authorityPinForEvent(invalidAdmission))).toBeNull()

    const reordered = structuredClone(witnessed) as any
    const originalRecord = reordered.item.acceptedFinal
    const originalAdmission = originalRecord.factFinalWitnessAdmission
    const reversedBinding = Object.fromEntries(Object.entries(originalAdmission.witnessBinding).reverse())
    const reversedAdmission = Object.fromEntries(Object.entries({
      ...originalAdmission,
      witnessBinding: reversedBinding
    }).reverse())
    reordered.item.acceptedFinal = Object.fromEntries(Object.entries({
      ...originalRecord,
      factFinalWitnessAdmission: reversedAdmission
    }).reverse())
    reordered.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(reordered)
    expect(verifyAcceptedFinalRecordIntegrity(reordered.item.acceptedFinal, pin)).toBe(true)
    expect(verifiedAcceptedFinalItemCompletedEvent(reordered, pin)?.item.text).toContain('4200000')

    const foreign = signedAcceptedFinalEvent('foreign self-signed final')
    expect(verifiedAcceptedFinalItemCompletedEvent(witnessed, authorityPinForEvent(foreign))).toBeNull()
    expect(verifyAcceptedFinalRecordIntegrity((witnessed as any).item.acceptedFinal, null)).toBe(false)
  })
})
