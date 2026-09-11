import { describe, expect, expectTypeOf, it } from 'vitest'
import type {
  CoreAcceptedFinalPublicViewJson,
  CoreAcceptedFinalPublicViewV2Json,
  CoreAcceptedFinalRecordV3Json,
  CoreBoundaryAcceptedFinalRecordV3Json,
  CoreBoundaryAcceptedFinalRecordV5Json,
  CoreWitnessedFactAcceptedFinalRecordV4Json,
  CoreRuntimeEventJson,
  CoreTurnItemJson,
  CoreTurnJson
} from './analytix-contract'
import type {
  AcceptedFinalPublicView,
  AcceptedFinalPublicViewCoreV2,
  AcceptedFinalPublicViewV2,
  AcceptedFinalRecordV3,
  BoundaryAcceptedFinalRecordV3,
  BoundaryAcceptedFinalRecordV5,
  WitnessedFactAcceptedFinalRecordV4
} from '../../../../packages/runtime/src/contracts/items.js'

const digest = (character: string) => character.repeat(64)

const signedPublicView = {
  schemaVersion: 2,
  publicationState: 'accepted',
  envelopeDigest: digest('b'),
  contextDigest: digest('c'),
  contextEpoch: 7,
  datasetSnapshotId: 'snapshot_case_7',
  variant: 'SourceUnavailableAnswer',
  terminalReason: 'source_unavailable',
  blockerCode: 'current_case_source_unavailable',
  coverageStatus: 'unavailable',
  checkedScopeDigest: '',
  missingScopeCount: 0,
  claimCount: 0,
  claimTypes: [],
  receiptMetadata: {
    projection: 'masked_metadata_only',
    count: 0,
    setDigest: digest('8'),
    citations: []
  },
  noHitWording: '',
  envelopeIssuedAt: '2026-07-11T01:02:03Z',
  acceptedAt: '2026-07-11T01:02:03Z'
} satisfies AcceptedFinalPublicViewCoreV2

const acceptedFinal = {
  schemaVersion: 5,
  authorityPurpose: 'analytix.case-final/v1',
  authorityAlgorithm: 'Ed25519',
  authorityKeyId: digest('a'),
  authorityPublicKey: 'A'.repeat(43),
  threadId: 'thr_case',
  turnId: 'turn_case',
  envelopeDigest: signedPublicView.envelopeDigest,
  contextDigest: signedPublicView.contextDigest,
  contextEpoch: signedPublicView.contextEpoch,
  datasetSnapshotId: signedPublicView.datasetSnapshotId,
  variant: signedPublicView.variant,
  terminalReason: signedPublicView.terminalReason,
  renderedTextSha256: digest('d'),
  registrySequence: 0,
  registryStateDigest: digest('e'),
  rendererVersion: 'analytix.host-final-renderer/v1',
  finalGateVersion: 'analytix.final-evidence-gate/v4',
  verifierVersion: 'analytix.claim-verifier-policy/v1',
  publicView: signedPublicView,
  publicViewDigest: digest('7'),
  privateRecordDigest: digest('f'),
  acceptedAt: signedPublicView.acceptedAt,
  authoritySignature: 'A'.repeat(86),
  recordDigest: digest('9')
} as const satisfies CoreBoundaryAcceptedFinalRecordV5Json

const acceptedFinalView = {
  ...signedPublicView,
  acceptedFinalDigest: acceptedFinal.recordDigest,
  publicViewDigest: acceptedFinal.publicViewDigest
} as const satisfies CoreAcceptedFinalPublicViewV2Json

const genericAcceptedFinalView = {
  schemaVersion: 3 as const,
  acceptedFinalDigest: acceptedFinal.recordDigest,
  publicationState: 'accepted' as const,
  variant: signedPublicView.variant,
  terminalReason: signedPublicView.terminalReason,
  blockerCode: signedPublicView.blockerCode,
  coverageStatus: signedPublicView.coverageStatus,
  checkedScopeDigest: signedPublicView.checkedScopeDigest,
  missingScopeCount: signedPublicView.missingScopeCount,
  claimCount: signedPublicView.claimCount,
  claimTypes: signedPublicView.claimTypes,
  receiptMetadata: signedPublicView.receiptMetadata,
  noHitWording: signedPublicView.noHitWording,
  acceptedAt: signedPublicView.acceptedAt
} as const satisfies CoreAcceptedFinalPublicViewJson

describe('analytix accepted-final contract', () => {
  it('types the V3 projection shape without treating a standalone turn or item as authority', () => {
    const projectionShape: CoreAcceptedFinalPublicViewJson = genericAcceptedFinalView
    expect(projectionShape.blockerCode).toBe('current_case_source_unavailable')
    expect(projectionShape.publicationState).toBe('accepted')
    expectTypeOf<CoreTurnJson['acceptedFinal']>().toEqualTypeOf<undefined>()
    expectTypeOf<CoreTurnItemJson['acceptedFinal']>().toEqualTypeOf<undefined>()
    expectTypeOf<NonNullable<CoreTurnJson['acceptedFinalView']>>()
      .toEqualTypeOf<CoreAcceptedFinalPublicViewJson>()
    expectTypeOf<CoreAcceptedFinalRecordV3Json>().toEqualTypeOf<AcceptedFinalRecordV3>()
    expectTypeOf<CoreBoundaryAcceptedFinalRecordV3Json>().toEqualTypeOf<BoundaryAcceptedFinalRecordV3>()
    expectTypeOf<CoreBoundaryAcceptedFinalRecordV5Json>().toEqualTypeOf<BoundaryAcceptedFinalRecordV5>()
    expectTypeOf<CoreWitnessedFactAcceptedFinalRecordV4Json>().toEqualTypeOf<WitnessedFactAcceptedFinalRecordV4>()
    expectTypeOf<CoreAcceptedFinalPublicViewV2Json>().toEqualTypeOf<AcceptedFinalPublicViewV2>()
    expectTypeOf<CoreAcceptedFinalPublicViewJson>().toEqualTypeOf<AcceptedFinalPublicView>()
  })

  it('exposes accepted-final digest and terminal reason on runtime events', () => {
    const terminal: CoreRuntimeEventJson = {
      kind: 'turn_completed',
      seq: 11,
      timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      status: 'completed',
      acceptedFinalDigest: acceptedFinal.recordDigest,
      terminalReason: acceptedFinal.terminalReason
    }

    expect(terminal.acceptedFinalDigest).toBe(acceptedFinal.recordDigest)
    expectTypeOf<CoreRuntimeEventJson['acceptedFinalDigest']>().toEqualTypeOf<string | undefined>()
    expectTypeOf<CoreRuntimeEventJson['terminalReason']>()
      .toEqualTypeOf<CoreAcceptedFinalRecordV3Json['terminalReason'] | undefined>()
  })
})
