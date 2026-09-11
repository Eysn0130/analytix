import { join } from 'node:path'
import { createHash } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { pathToFileURL } from 'node:url'
import { describe, expect, it } from 'vitest'

type ScanResult = Readonly<{
  accepted: boolean
  completeIdentifierFindingCount: number
  forbiddenHostMaterialFindingCount: number
  providerSafeSemanticBlockCount: number
  providerSafeSemanticValidationCount: number
}>

type ScannerModule = Readonly<{
  createProviderRequestAuditRun(options?: { onFailure?: () => void }): AuditRun
  readExactFrame(socket: TestSocket, run: AuditRun): Promise<{ metadata: Record<string, unknown>, body: Buffer }>
  scanOutboundProviderRequestBody(input: Readonly<{
    body: Buffer
    protectedValues: string[]
    forbiddenValues: string[]
  }>): ScanResult
}>

type AuditRun = Readonly<{
  reject(): false
  acceptProviderId(value: unknown): boolean
  isAccepted(): boolean
  acceptScannedBody(result: { accepted: boolean }, digest: string, expectedDigest: string): boolean
  receiptFields(totals: Record<string, number>): Record<string, number | string>
}>

class TestSocket extends EventEmitter {
  setTimeout() { return this }
  pause() { return this }
}

function auditFrame(providerFamily: unknown, overrides: Record<string, unknown> = {}) {
  const body = Buffer.from('{"synthetic":"body"}')
  const metadata = {
    schemaVersion: 1,
    purpose: 'analytix.provider-request-body-audit/v1',
    providerFamily,
    attempt: 1,
    bodyByteLength: body.length,
    bodySha256: createHash('sha256').update(body).digest('hex'),
    ...overrides
  }
  return Buffer.concat([Buffer.from(`${JSON.stringify(metadata)}\n`), body])
}

function readFrame(scanner: ScannerModule, run: AuditRun, frame: Buffer) {
  const socket = new TestSocket()
  const result = scanner.readExactFrame(socket, run)
  socket.emit('data', frame)
  return result
}

async function scannerModule(): Promise<ScannerModule> {
  return await import(pathToFileURL(join(
    process.cwd(),
    'scripts/provider-request-audit-scanner.mjs'
  )).href) as ScannerModule
}

const protectedValues = ['6222020202020202020', 'PRIVATE-NAME']
const forbiddenValues = [
  '/formal/case-owner',
  '/formal/case-owner/workspace',
  '/formal/case-owner/workspace/baseline.csv',
  '/formal/case-owner/workspace/evolved.csv'
]

function bodyWithContent(content: unknown): Buffer {
  return Buffer.from(JSON.stringify({
    model: 'deepseek-v4-flash',
    messages: [{ role: 'tool', content: JSON.stringify(content) }]
  }))
}

function providerSafeSemantic(): Record<string, unknown> {
  return {
    subjectRef: `cer1_${'a'.repeat(64)}`,
    startInclusive: '2026-01-01T00:00:00.000000Z',
    endInclusive: '2026-01-31T23:59:59.000000Z',
    timezone: 'Z',
    currency: 'CNY',
    minorUnitScale: 2,
    inflowMinor: '12025',
    outflowMinor: '0',
    netMinor: '12025',
    transactionCount: 1,
    evidenceTransactionCount: 1,
    evidenceRowLimit: 512,
    aggregateComplete: true,
    evidenceRowsComplete: true,
    counterpartySemanticsComplete: true,
    coverage: {
      state: 'complete',
      gaps: [],
      normalizedSnapshotRows: 1,
      acceptedSnapshotRows: 1,
      rejectedSnapshotRows: 0,
      duplicateSnapshotRows: 0,
      untimedSubjectRows: 0,
      observedMatchingRows: 1
    },
    transactions: [{
      evidenceRef: `srow1_${'c'.repeat(64)}`,
      counterparty: {
        status: 'resolved',
        reference: `cer1_${'b'.repeat(64)}`,
        display: {
          entityType: 'bank_account_number',
          stableOrdinal: 1,
          safeSuffix: '1234',
          institution: 'Safe Bank',
          accountType: '交易对手账户',
          text: '第1个银行账户（Safe Bank；交易对手账户；尾号1234）'
        }
      },
      occurredAt: '2026-01-02T00:00:00.000000Z',
      direction: 'inflow',
      amountMinor: '12025',
      currency: 'CNY',
      minorUnitScale: 2
    }],
    queryHash: 'a'.repeat(64),
    resultHash: 'b'.repeat(64)
  }
}

describe('external provider request audit scanner', () => {
  it('binds the first valid Provider ID through the actual frame reader and retains exact ACK metadata', async () => {
    const scanner = await scannerModule()
    const run = scanner.createProviderRequestAuditRun()
    for (const attempt of [1, 2]) {
      const frame = await readFrame(scanner, run, auditFrame('local-provider-1', { attempt }))
      expect(frame.metadata).toMatchObject({ providerFamily: 'local-provider-1', attempt })
      frame.body.fill(0)
    }
    expect(run.isAccepted()).toBe(true)
    expect(run.receiptFields({ providerRequestCount: 2, scannedRequestBodyCount: 2 })).toEqual({
      providerFamily: 'local-provider-1', providerRequestCount: 2, scannedRequestBodyCount: 2
    })
  })

  it('replaces a valid receipt identity immediately on mixed IDs and keeps failure sticky without inventing counts', async () => {
    const scanner = await scannerModule()
    const totals = { providerRequestCount: 1, scannedRequestBodyCount: 1 }
    let receipt: Record<string, number | string> = {}
    let failures = 0
    const run = scanner.createProviderRequestAuditRun({ onFailure: () => {
      failures++
      receipt = run.receiptFields(totals)
    } })
    const first = await readFrame(scanner, run, auditFrame('local-provider-1'))
    first.body.fill(0)
    receipt = run.receiptFields(totals)
    expect(receipt.providerFamily).toBe('local-provider-1')
    await expect(readFrame(scanner, run, auditFrame('local-provider-2'))).rejects.toThrow('frame_invalid')
    expect(receipt).toEqual({ providerFamily: '', ...totals })
    await expect(readFrame(scanner, run, auditFrame('local-provider-1'))).rejects.toThrow('frame_invalid')
    expect(run.isAccepted()).toBe(false)
    expect(run.receiptFields(totals)).toEqual({ providerFamily: '', ...totals })
    expect(failures).toBe(1)
  })

  it.each(['analytix-hub', '', 'UPPERCASE', ' leading', 'trailing\n', 'bad/path', 'x'.repeat(97), null, 42])('rejects Hub or invalid Provider ID %j permanently', async (providerId) => {
    const scanner = await scannerModule()
    const run = scanner.createProviderRequestAuditRun()
    await expect(readFrame(scanner, run, auditFrame(providerId))).rejects.toThrow('frame_invalid')
    await expect(readFrame(scanner, run, auditFrame('local-provider-1'))).rejects.toThrow('frame_invalid')
    expect(run.receiptFields({ providerRequestCount: 0, scannedRequestBodyCount: 0 })).toEqual({
      providerFamily: '', providerRequestCount: 0, scannedRequestBodyCount: 0
    })
  })

  it('latches malformed-frame failure after a previously valid Provider binding', async () => {
    const scanner = await scannerModule()
    const run = scanner.createProviderRequestAuditRun()
    const first = await readFrame(scanner, run, auditFrame('local-provider-1'))
    first.body.fill(0)
    await expect(readFrame(scanner, run, auditFrame('local-provider-1', { bodySha256: 'invalid' }))).rejects.toThrow('frame_invalid')
    expect(run.receiptFields({ providerRequestCount: 1, scannedRequestBodyCount: 1 })).toEqual({
      providerFamily: '', providerRequestCount: 1, scannedRequestBodyCount: 1
    })
  })

  it('invalidates the run if a truncated frame closes without an end event', async () => {
    const scanner = await scannerModule()
    const run = scanner.createProviderRequestAuditRun()
    expect(run.acceptProviderId('local-provider-1')).toBe(true)
    const socket = new TestSocket()
    const result = scanner.readExactFrame(socket, run)
    socket.emit('data', auditFrame('local-provider-1').subarray(0, -1))
    socket.emit('close')
    await expect(result).rejects.toThrow('frame_invalid')
    expect(run.receiptFields({ providerRequestCount: 1, scannedRequestBodyCount: 1 })).toEqual({
      providerFamily: '', providerRequestCount: 1, scannedRequestBodyCount: 1
    })
  })

  it.each(['scan', 'digest'])('preserves true counters when %s rejection invalidates a bound run', async (rejection) => {
    const scanner = await scannerModule()
    const run = scanner.createProviderRequestAuditRun()
    expect(run.acceptProviderId('local-provider-1')).toBe(true)
    expect(run.acceptScannedBody({ accepted: rejection !== 'scan' }, 'a'.repeat(64), rejection === 'digest' ? 'b'.repeat(64) : 'a'.repeat(64))).toBe(false)
    expect(run.isAccepted()).toBe(false)
    expect(run.receiptFields({ providerRequestCount: 3, scannedRequestBodyCount: 3, completeIdentifierFindingCount: 0 })).toEqual({
      providerFamily: '', providerRequestCount: 3, scannedRequestBodyCount: 3, completeIdentifierFindingCount: 0
    })
    expect(run.acceptProviderId('local-provider-1')).toBe(false)
    expect(run.acceptScannedBody({ accepted: true }, 'a'.repeat(64), 'a'.repeat(64))).toBe(false)
  })

  it('accepts a bounded provider-safe account-flow projection', async () => {
    const scanner = await scannerModule()
    expect(scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent(providerSafeSemantic()),
      protectedValues,
      forbiddenValues
    })).toEqual({
      accepted: true,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 1,
      providerSafeSemanticValidationCount: 1
    })
  })

  it.each([
    '6222020202020202020',
    '6222 0202-0202\u00a00202 020',
    '\\u0036\\u0032\\u0032\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030\\u0032\\u0030',
    '&#54;&#50;&#50;&#50;&#48;&#50;&#48;&#50;&#48;&#50;&#48;&#50;&#48;&#50;&#48;&#50;&#48;&#50;&#48;',
    '6222<em>0202</em>02020202020'
  ])('rejects complete protected identifiers through supported encodings: %s', async (value) => {
    const scanner = await scannerModule()
    const result = scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent({ text: value }),
      protectedValues,
      forbiddenValues
    })
    expect(result.accepted).toBe(false)
    expect(result.completeIdentifierFindingCount).toBeGreaterThan(0)
  })

  it.each([
    { caseId: 'case-private' },
    { note: '/formal/case-owner/workspace/baseline.csv' },
    { note: '/private/results.sqlite' },
    { note: 'SELECT * FROM private_table' }
  ])('rejects forbidden host material: %j', async (content) => {
    const scanner = await scannerModule()
    const result = scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent(content),
      protectedValues,
      forbiddenValues
    })
    expect(result.accepted).toBe(false)
    expect(result.forbiddenHostMaterialFindingCount).toBeGreaterThan(0)
  })

  it('fails closed when an account-flow semantic projection is incomplete', async () => {
    const scanner = await scannerModule()
    const semantic = providerSafeSemantic()
    delete semantic.resultHash
    expect(scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent(semantic),
      protectedValues,
      forbiddenValues
    })).toEqual(expect.objectContaining({
      accepted: false,
      providerSafeSemanticBlockCount: 1,
      providerSafeSemanticValidationCount: 0
    }))
  })

  it.each([
    'SELECT*FROM private_table',
    'WITH(candidate) AS (VALUES(1))',
    'PRAGMA\nshow_tables',
    '\\u0053\\u0045\\u004c\\u0045\\u0043\\u0054*FROM private_table'
  ])('rejects token-boundary and encoded SQL evasions: %s', async (value) => {
    const scanner = await scannerModule()
    const result = scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent({ text: value }),
      protectedValues,
      forbiddenValues
    })
    expect(result.accepted).toBe(false)
    expect(result.forbiddenHostMaterialFindingCount).toBeGreaterThan(0)
  })

  it('does not misclassify ordinary language containing SQL keyword words', async () => {
    const scanner = await scannerModule()
    expect(scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent({
        text: 'Continue with current evidence. Copy the safe reference. Attach no files.'
      }),
      protectedValues,
      forbiddenValues
    })).toEqual({
      accepted: true,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 0,
      providerSafeSemanticValidationCount: 0
    })
  })

  it('rejects unknown fields and malformed evidence transactions inside semantic data', async () => {
    const scanner = await scannerModule()
    const semantic = providerSafeSemantic()
    semantic.databaseLocator = '/not-a-real-locator'
    expect(scanner.scanOutboundProviderRequestBody({
      body: bodyWithContent(semantic),
      protectedValues,
      forbiddenValues
    })).toEqual(expect.objectContaining({
      accepted: false,
      providerSafeSemanticBlockCount: 1,
      providerSafeSemanticValidationCount: 0
    }))
  })
})
