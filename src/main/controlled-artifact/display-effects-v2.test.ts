import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  BrowserWindow: class {},
  MessageChannelMain: class {}
}))

import {
  ControlledArtifactDisplayTargetV2,
  controlledDisplayDocumentURLV2,
  controlledReauthorizationDocumentURLV2,
  formatControlledArtifactTimestampV2,
  parseControlledArtifactDisplayModelV2,
  type ControlledArtifactDisplayModelV2,
  type ControlledArtifactDisplaySinkV2
} from './display-effects-v2'
import type { ControlledArtifactReleaseEffectInputV2 } from './host-v2'

const FULL_ACCOUNT = '6222021234567890123'
const CASE_SCOPED_ACCOUNT_DIGEST = '9'.repeat(64)
const START = '2026-07-18T12:00:00.123456789Z'
const DEADLINE = '2026-07-18T12:10:00.123456789Z'
const EVIDENCE_RECEIPT_IDS = [
  `evr_${'a'.repeat(64)}`,
  `evr_${'b'.repeat(64)}`
] as const
const ACCEPTED_FINAL_RECEIPT_SET_DIGEST = sha256(Buffer.from(
  JSON.stringify([...EVIDENCE_RECEIPT_IDS].sort()),
  'utf8'
))

describe('controlled artifact V2 isolated display effects', () => {
  it('keeps the trusted sink isolated, non-exporting, and revoke-capable', () => {
    const source = readFileSync(join(
      process.cwd(),
      'src/main/controlled-artifact/display-effects-v2.ts'
    ), 'utf8')
    for (const required of [
      'nodeIntegration: false',
      'contextIsolation: true',
      'sandbox: true',
      'webviewTag: false',
      'devTools: false',
      'window.setContentProtection(true)',
      "setPermissionCheckHandler(() => false)",
      "setWindowOpenHandler(() => ({ action: 'deny' }))",
      "window.webContents.on('before-input-event'",
      'if (!window.isDestroyed()) window.destroy()',
      'expiryTimer = setTimeout(() =>',
      'partition: `analytix-controlled-v2-${partitionNonce}`',
      "root.replaceChildren()"
    ]) {
      expect(source).toContain(required)
    }
    expect(source).not.toMatch(/\bpreload\s*:/u)
    expect(source).not.toMatch(/partition:\s*[`'"]persist:/u)
    expect(source).not.toMatch(/shell\.|openExternal|openPath|writeFile|clipboard\./u)
  })

  it('brands the static data document as the exact PII-free trusted sink', () => {
    const url = controlledDisplayDocumentURLV2()
    const prefix = 'data:text/html;charset=utf-8,'
    expect(url.startsWith(prefix)).toBe(true)
    const html = decodeURIComponent(url.slice(prefix.length))

    expect(html).toContain(
      '<meta name="analytix-controlled-artifact" content="trusted-sink-v2">'
    )
    expect(html).toContain(
      '<main id="controlled-root" data-analytix-controlled-artifact="trusted-sink-v2" aria-live="polite"></main>'
    )
    expect(html).toContain(
      "card.setAttribute('data-evidence-receipt-set-digest', flow.evidenceReceiptSetDigest)"
    )
    expect(html).not.toContain(FULL_ACCOUNT)
    for (const receiptID of EVIDENCE_RECEIPT_IDS) expect(html).not.toContain(receiptID)
    expect(html).not.toMatch(/cer1_[a-f0-9]{64}/u)
  })

  it('renders typed account-flow slots with the exact account but no internal identity', async () => {
    let current = true
    let observed: ControlledArtifactDisplayModelV2 | null = null
    let revoked = false
    let closed = false
    const sink: ControlledArtifactDisplaySinkV2 = Object.freeze({
      open: async ({ model, modelDigest, authorizedUntil, isAuthorityCurrent, signal }) => {
        expect(modelDigest).toBe(sha256(Buffer.from(JSON.stringify(model), 'utf8')))
        expect(authorizedUntil).toBe(DEADLINE)
        expect(isAuthorityCurrent()).toBe(true)
        expect(signal.aborted).toBe(false)
        observed = model
        return Object.freeze({
          committedAt: START,
          revoke: () => { revoked = true },
          close: () => { closed = true }
        })
      }
    })
    const target = new ControlledArtifactDisplayTargetV2({
      webContentsId: 14,
      rendererGeneration: 6,
      isRendererCurrent: () => current,
      random: () => Buffer.alloc(32, 0x42),
      now: () => START,
      sink
    })
    const input = effectInput(target.targetIdentityDigest(), true)
    target.bindInvocationContext(invocationContext(input))
    const prepared = await target.createPreparation()(input, new AbortController().signal)
    const receipt = await prepared.commitBefore(DEADLINE, new AbortController().signal)

    expect(receipt).toEqual({
      releaseTargetIdentityDigest: input.releaseTargetIdentityDigest,
      artifactSha256: input.receipt.artifactSha256,
      artifactByteLength: input.body.length,
      mediaType: input.receipt.mediaType,
      committedAt: START
    })
    const display = observed as ControlledArtifactDisplayModelV2 | null
    if (!display) throw new Error('expected a controlled display model')
    const serialized = JSON.stringify(display)
    expect(serialized).toContain(FULL_ACCOUNT)
    expect(serialized).toContain('流入 CNY 3,280,000.00')
    expect(serialized).toContain('流出 CNY 3,010,000.00')
    expect(serialized).toContain('净额 CNY 270,000.00')
    expect(serialized).toContain('共 2 笔交易')
    expect(serialized).not.toMatch(/acct_ref_|ACCOUNT_A|claim-account|accountValueSha256|evidenceRowOrdinal/u)
    expect(display.accountFlows[0]?.transactions).toEqual([
      { occurredAt: '2026-01-02T03:04:05Z', direction: '流入', amount: 'CNY 3,280,000.00' },
      { occurredAt: '2026-01-03T03:04:05Z', direction: '流出', amount: 'CNY 3,010,000.00' }
    ])
    expect(display.accountFlows[0]?.evidenceReceiptSetDigest).toBe(
      ACCEPTED_FINAL_RECEIPT_SET_DIGEST
    )
    target.revoke()
    expect(revoked).toBe(true)
    expect(closed).toBe(false)
    current = false
    target.close()
    expect(closed).toBe(true)
    input.body.fill(0)
  })

  it('keeps legacy exact-field artifacts displayable without inventing analysis', () => {
    const input = effectInput('a'.repeat(64), false)
    const parsed = parseControlledArtifactDisplayModelV2(input, 'a'.repeat(64))
    expect(parsed.model.accountFlows).toEqual([])
    expect(parsed.model.fields).toEqual([{
      label: '银行账号',
      exactValue: FULL_ACCOUNT,
      evidenceReceiptCount: 2
    }])
    expect(JSON.stringify(parsed.model)).not.toContain('claim-account')
    input.body.fill(0)
  })

  it('uses a fixed PII-free isolated reauthorization document after revocation', () => {
    const url = controlledReauthorizationDocumentURLV2()
    const prefix = 'data:text/html;charset=utf-8,'
    expect(url.startsWith(prefix)).toBe(true)
    const html = decodeURIComponent(url.slice(prefix.length))
    expect(html).toContain('data-controlled-artifact-state="reauthorization-v2"')
    expect(html).toContain('<h1>需要重新授权</h1>')
    expect(html).toContain('完整标识已隐藏，请重新授权后查看。')
    expect(html).not.toContain(FULL_ACCOUNT)
    for (const receiptID of EVIDENCE_RECEIPT_IDS) expect(html).not.toContain(receiptID)
    expect(html).not.toMatch(/cer1_[a-p]{64}/u)
    expect(html).not.toContain('<script')
  })

  it('accepts only the opaque case-scoped account join, never a bare identifier hash', () => {
    const input = effectInput('a'.repeat(64), true)
    expect(CASE_SCOPED_ACCOUNT_DIGEST).not.toBe(sha256(Buffer.from(FULL_ACCOUNT, 'utf8')))
    expect(parseControlledArtifactDisplayModelV2(input, 'a'.repeat(64)).model.accountFlows)
      .toHaveLength(1)

    const legacy = JSON.parse(input.body.toString('utf8')) as Record<string, unknown>
    const fields = legacy.fields as Array<Record<string, unknown>>
    const analyses = legacy.accountFlowAnalyses as Array<Record<string, unknown>>
    const bareDigest = sha256(Buffer.from(FULL_ACCOUNT, 'utf8'))
    fields[0].valueSha256 = bareDigest
    analyses[0].accountValueSha256 = bareDigest
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(input, legacy),
      'a'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')
    input.body.fill(0)
  })

  it('rejects aggregate drift, target drift, cross-context content, and unknown fields', () => {
    const exact = effectInput('b'.repeat(64), true)
    const artifact = JSON.parse(exact.body.toString('utf8')) as Record<string, unknown>
    const analyses = artifact.accountFlowAnalyses as Array<Record<string, unknown>>
    analyses[0].netMinor = '27000001'
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(exact, artifact),
      'b'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')

    expect(() => parseControlledArtifactDisplayModelV2(exact, 'c'.repeat(64))).toThrow(
      'controlled_artifact_display_effect_v2_failed'
    )
    expect(() => parseControlledArtifactDisplayModelV2(
      exact,
      'b'.repeat(64),
      { ...invocationContext(exact), contextEpoch: 8 }
    )).toThrow('controlled_artifact_display_effect_v2_failed')
    const foreign = JSON.parse(exact.body.toString('utf8')) as Record<string, unknown>
    const context = foreign.context as Record<string, unknown>
    context.caseId = 'case-foreign'
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(exact, foreign),
      'b'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')

    const unknown = JSON.parse(exact.body.toString('utf8')) as Record<string, unknown>
    unknown.internalReference = 'acct_ref_17'
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(exact, unknown),
      'b'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')
    exact.body.fill(0)
  })

  it('fails closed on receipt-set or evidence-row mutation', () => {
    const input = effectInput('e'.repeat(64), true)

    const detachedReceipt = JSON.parse(input.body.toString('utf8')) as Record<string, unknown>
    const detachedAnalyses = detachedReceipt.accountFlowAnalyses as Array<Record<string, unknown>>
    detachedAnalyses[0].evidenceReceiptIds = [
      EVIDENCE_RECEIPT_IDS[0],
      `evr_${'c'.repeat(64)}`
    ]
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(input, detachedReceipt),
      'e'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')

    const noncanonicalReceiptSet = JSON.parse(input.body.toString('utf8')) as Record<string, unknown>
    const fields = noncanonicalReceiptSet.fields as Array<Record<string, unknown>>
    const analyses = noncanonicalReceiptSet.accountFlowAnalyses as Array<Record<string, unknown>>
    fields[0].evidenceReceiptIds = [...EVIDENCE_RECEIPT_IDS].reverse()
    analyses[0].evidenceReceiptIds = [...EVIDENCE_RECEIPT_IDS].reverse()
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(input, noncanonicalReceiptSet),
      'e'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')

    const changedRow = JSON.parse(input.body.toString('utf8')) as Record<string, unknown>
    const changedAnalyses = changedRow.accountFlowAnalyses as Array<Record<string, unknown>>
    const transactions = changedAnalyses[0].transactions as Array<Record<string, unknown>>
    transactions[0].amountMinor = '328000001'
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(input, changedRow),
      'e'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')
    input.body.fill(0)
  })

  it('rejects canonical internal entity references from the final display model', () => {
    const input = effectInput('d'.repeat(64), false)
    const artifact = JSON.parse(input.body.toString('utf8')) as Record<string, unknown>
    const fields = artifact.fields as Array<Record<string, unknown>>
    const internalReference = `cer1_${'a'.repeat(64)}`
    fields[0].exactValue = internalReference
    fields[0].valueSha256 = CASE_SCOPED_ACCOUNT_DIGEST
    expect(() => parseControlledArtifactDisplayModelV2(
      rebody(input, artifact),
      'd'.repeat(64)
    )).toThrow('controlled_artifact_display_effect_v2_failed')
    input.body.fill(0)
  })

  it('closes a committed session when the prepared effect is aborted', async () => {
    const revoke = vi.fn()
    const close = vi.fn()
    const target = new ControlledArtifactDisplayTargetV2({
      webContentsId: 15,
      rendererGeneration: 7,
      isRendererCurrent: () => true,
      random: () => Buffer.alloc(32, 0x43),
      now: () => START,
      sink: Object.freeze({
        open: async () => Object.freeze({ committedAt: START, revoke, close })
      })
    })
    const input = effectInput(target.targetIdentityDigest(), false)
    target.bindInvocationContext(invocationContext(input))
    const prepared = await target.createPreparation()(input, new AbortController().signal)
    await prepared.commitBefore(DEADLINE, new AbortController().signal)
    await prepared.abort(new AbortController().signal)
    expect(revoke).toHaveBeenCalledTimes(1)
    expect(close).not.toHaveBeenCalled()
    target.close()
    expect(close).toHaveBeenCalledTimes(1)
    input.body.fill(0)
  })

  it('closes a sink session rejected by post-open validation', async () => {
    const close = vi.fn()
    const target = new ControlledArtifactDisplayTargetV2({
      webContentsId: 16,
      rendererGeneration: 8,
      isRendererCurrent: () => true,
      random: () => Buffer.alloc(32, 0x44),
      now: () => START,
      sink: Object.freeze({
        open: async () => Object.freeze({
          committedAt: '2026-07-18T12:00:01.123456789Z',
          revoke: vi.fn(),
          close
        })
      })
    })
    const input = effectInput(target.targetIdentityDigest(), false)
    target.bindInvocationContext(invocationContext(input))
    const prepared = await target.createPreparation()(input, new AbortController().signal)
    await expect(prepared.commitBefore(
      DEADLINE,
      new AbortController().signal
    )).rejects.toThrow('controlled_artifact_display_effect_v2_failed')
    expect(close).toHaveBeenCalledTimes(1)
    input.body.fill(0)
  })

  it('formats host timestamps in the canonical no-trailing-zero form', () => {
    expect(formatControlledArtifactTimestampV2(
      new Date('2026-07-18T12:00:00.000Z')
    )).toBe('2026-07-18T12:00:00Z')
    expect(formatControlledArtifactTimestampV2(
      new Date('2026-07-18T12:00:00.120Z')
    )).toBe('2026-07-18T12:00:00.12Z')
    expect(formatControlledArtifactTimestampV2(
      new Date('2026-07-18T12:00:00.123Z')
    )).toBe('2026-07-18T12:00:00.123Z')
  })

  it('fails closed when renderer authority is revoked or the deadline has elapsed', async () => {
    let current = true
    const open = vi.fn(async () => Object.freeze({
      committedAt: START,
      revoke: vi.fn(),
      close: vi.fn()
    }))
    const target = new ControlledArtifactDisplayTargetV2({
      webContentsId: 9,
      rendererGeneration: 3,
      isRendererCurrent: () => current,
      random: () => Buffer.alloc(32, 0x24),
      now: () => START,
      sink: Object.freeze({ open })
    })
    const input = effectInput(target.targetIdentityDigest(), false)
    target.bindInvocationContext(invocationContext(input))
    const prepared = await target.createPreparation()(input, new AbortController().signal)
    current = false
    await expect(prepared.commitBefore(DEADLINE, new AbortController().signal)).rejects.toThrow(
      'controlled_artifact_display_effect_v2_failed'
    )
    expect(open).not.toHaveBeenCalled()

    const expired = new ControlledArtifactDisplayTargetV2({
      webContentsId: 10,
      rendererGeneration: 4,
      isRendererCurrent: () => true,
      random: () => Buffer.alloc(32, 0x25),
      now: () => DEADLINE,
      sink: Object.freeze({ open })
    })
    const expiredInput = effectInput(expired.targetIdentityDigest(), false)
    expired.bindInvocationContext(invocationContext(expiredInput))
    const expiredPrepared = await expired.createPreparation()(
      expiredInput,
      new AbortController().signal
    )
    await expect(expiredPrepared.commitBefore(
      DEADLINE,
      new AbortController().signal
    )).rejects.toThrow('controlled_artifact_display_effect_v2_failed')
    expect(open).not.toHaveBeenCalled()
    input.body.fill(0)
    expiredInput.body.fill(0)
  })
})

function effectInput(
  releaseTargetIdentityDigest: string,
  withAccountFlow: boolean
): ControlledArtifactReleaseEffectInputV2 {
  const accountHash = CASE_SCOPED_ACCOUNT_DIGEST
  const context = {
    version: 2,
    threadId: 'thread-1',
    turnId: 'turn-1',
    workspaceRealPath: '/private/case-one',
    tenantId: 'tenant-1',
    userId: 'user-1',
    caseId: 'case-1',
    caseBindingHash: '1'.repeat(64),
    datasetSnapshotId: `dsv2_${'2'.repeat(64)}`,
    sourceManifestHash: '3'.repeat(64),
    contextEpoch: 7,
    contextIssuedAt: '2026-07-18T11:59:00.123456789Z',
    contextDigest: '4'.repeat(64)
  }
  const artifact: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.controlled-pii-artifact/v1',
    mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
    context,
    requesterUserId: 'user-1',
    disclosurePurpose: 'case_report',
    deliveryScope: 'controlled_artifact',
    claimLedgerDigest: '5'.repeat(64),
    projectionRulesetHash: '6'.repeat(64),
    targetIdentityDigest: '7'.repeat(64),
    preservedControlledFieldCount: 1,
    fields: [{
      piiClass: 'financial_account_identifier',
      claimId: 'claim-account',
      claimRecordDigest: '8'.repeat(64),
      claimType: 'account',
      fieldName: 'accountId',
      exactValue: FULL_ACCOUNT,
      valueSha256: accountHash,
      evidenceReceiptIds: [...EVIDENCE_RECEIPT_IDS]
    }]
  }
  if (withAccountFlow) {
    artifact.accountFlowAnalyses = [{
      schemaVersion: 1,
      purpose: 'analytix.controlled-account-flow-analysis/v1',
      accountValueSha256: accountHash,
      startAt: '2026-01-01T00:00:00Z',
      endAt: '2026-01-31T23:59:59Z',
      currency: 'CNY',
      minorUnitScale: 2,
      inflowMinor: '328000000',
      outflowMinor: '301000000',
      netMinor: '27000000',
      transactionCount: '2',
      evidenceTransactionCount: '2',
      evidenceReceiptIds: [...EVIDENCE_RECEIPT_IDS],
      transactions: [{
        occurredAt: '2026-01-02T03:04:05Z',
        direction: 'in',
        amountMinor: '328000000',
        currency: 'CNY',
        minorUnitScale: 2,
        evidenceRowOrdinal: '1'
      }, {
        occurredAt: '2026-01-03T03:04:05Z',
        direction: 'out',
        amountMinor: '301000000',
        currency: 'CNY',
        minorUnitScale: 2,
        evidenceRowOrdinal: '2'
      }]
    }]
  }
  artifact.renderedAt = START
  const body = Buffer.from(JSON.stringify(artifact), 'utf8')
  return {
    action: 'display',
    body,
    releaseTargetIdentityDigest,
    receipt: {
      accessAction: 'display',
      context,
      requesterUserId: 'user-1',
      claimLedgerDigest: '5'.repeat(64),
      targetIdentityDigest: '7'.repeat(64),
      artifactByteLength: body.length,
      artifactSha256: sha256(body),
      mediaType: 'application/vnd.analytix.controlled-case-evidence+json',
      authorizedUntil: DEADLINE
    }
  } as ControlledArtifactReleaseEffectInputV2
}

function invocationContext(input: ControlledArtifactReleaseEffectInputV2) {
  return {
    threadId: input.receipt.context.threadId,
    turnId: input.receipt.context.turnId,
    contextEpoch: input.receipt.context.contextEpoch,
    contextDigest: input.receipt.context.contextDigest
  }
}

function rebody(
  input: ControlledArtifactReleaseEffectInputV2,
  artifact: Record<string, unknown>
): ControlledArtifactReleaseEffectInputV2 {
  const body = Buffer.from(JSON.stringify(artifact), 'utf8')
  return {
    ...input,
    body,
    receipt: {
      ...input.receipt,
      artifactByteLength: body.length,
      artifactSha256: sha256(body)
    }
  }
}

function sha256(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}
