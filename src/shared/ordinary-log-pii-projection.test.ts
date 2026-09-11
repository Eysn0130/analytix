import { describe, expect, it } from 'vitest'
import ordinaryPIICorpusJSON from '../../packages/runtime/src/conformance/fixtures/ordinary-pii-projection-v1.json'
import {
  containsInternalCaseEntityReference,
  containsOrdinaryPublicPII,
  projectOrdinaryLogPII,
  projectOrdinaryPublicText
} from './ordinary-log-pii-projection'

type OrdinaryPIICorpus = {
  schemaVersion: number
  syntheticOnly: boolean
  untrustedTextCases: Array<{
    id: string
    input: string
    expected: 'mask' | 'preserve'
    sensitiveCanonical?: string
  }>
}

const ordinaryPIICorpus = ordinaryPIICorpusJSON as OrdinaryPIICorpus

function canonicalDigits(value: string): string {
  return value.normalize('NFKC').replace(/[^0-9]/g, '')
}

describe('ordinary log PII projection', () => {
  it('detects only the closed internal authority-reference grammars', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const sourceRowRef = `srow1_${'b'.repeat(64)}`

    expect(containsInternalCaseEntityReference(authorityRef)).toBe(true)
    expect(containsInternalCaseEntityReference(`prefix ${authorityRef} suffix`)).toBe(true)
    expect(containsInternalCaseEntityReference(sourceRowRef)).toBe(true)
    expect(containsInternalCaseEntityReference(`prefix ${sourceRowRef} suffix`)).toBe(true)
    expect(containsInternalCaseEntityReference('acct:1')).toBe(false)
    expect(containsInternalCaseEntityReference('card:1')).toBe(false)
    expect(containsInternalCaseEntityReference('ab'.repeat(32))).toBe(false)
  })

  it('withholds closed internal references from ordinary public text', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const sourceRowRef = `srow1_${'b'.repeat(64)}`
    const escapedAuthorityRef = `\\u0063\\u0065\\u0072\\u0031\\u005f${'a'.repeat(64)}`
    const escapedSourceRowRef = `\\u0073\\u0072\\u006f\\u0077\\u0031\\u005f${'b'.repeat(64)}`

    expect(projectOrdinaryPublicText(authorityRef)).toBe('')
    expect(projectOrdinaryPublicText(`prefix ${sourceRowRef} suffix`)).toBe('')
    expect(projectOrdinaryPublicText(escapedAuthorityRef)).toBe('')
    expect(projectOrdinaryPublicText(`prefix ${escapedSourceRowRef} suffix`)).toBe('')
  })

  it('withholds partial restricted envelopes without internal references', () => {
    const canonicalEvidenceV3 = JSON.stringify({
      purpose: 'analytix.canonical-evidence/v3'
    })
    const sourceRowLocatorV1 = JSON.stringify({
      purpose: 'analytix.source-row-locator/v1',
      sourceFileId: 'opaque-source-file',
      sourceRowNumber: 7,
      field: 'account'
    })

    expect(projectOrdinaryPublicText(`prefix ${canonicalEvidenceV3} suffix`)).toBe('')
    expect(projectOrdinaryPublicText(`prefix ${sourceRowLocatorV1} suffix`)).toBe('')
  })

  it('preserves benign public prose, short aliases, and ordinary opaque hashes', () => {
    expect(projectOrdinaryPublicText('ordinary public prose')).toBe('ordinary public prose')
    expect(projectOrdinaryPublicText('acct:1 card:1')).toBe('acct:1 card:1')
    expect(projectOrdinaryPublicText('ab'.repeat(32))).toBe('ab'.repeat(32))
  })

  it('conforms to the shared synthetic ordinary PII corpus', () => {
    expect(ordinaryPIICorpus.schemaVersion).toBe(1)
    expect(ordinaryPIICorpus.syntheticOnly).toBe(true)
    for (const testCase of ordinaryPIICorpus.untrustedTextCases) {
      const projected = projectOrdinaryLogPII(testCase.input)
      if (testCase.expected === 'preserve') {
        expect(projected, testCase.id).toBe(testCase.input)
        continue
      }
      expect(projected, testCase.id).not.toBe(testCase.input)
      if (testCase.sensitiveCanonical) {
        expect(canonicalDigits(projected), testCase.id).not.toContain(testCase.sensitiveCanonical)
      }
    }
  })

  it('withholds complete restricted identifiers without changing safe diagnostics', () => {
    const safe = 'provider request failed with status 503'
    expect(projectOrdinaryLogPII(safe)).toBe(safe)

    const restricted = [
      'account=0000123456789012345',
      '银行卡号：6222 0202 0202 0202 020',
      '身份证号 110101199001011234',
      '电话 13800138000',
      'email=investigator@example.com',
      'ip=192.168.10.22',
      'mac=AA:BB:CC:DD:EE:FF'
    ].join(' | ')
    const projected = projectOrdinaryLogPII(restricted)

    for (const value of [
      '0000123456789012345',
      '6222 0202 0202 0202 020',
      '110101199001011234',
      '13800138000',
      'investigator@example.com',
      '192.168.10.22',
      'AA:BB:CC:DD:EE:FF'
    ]) {
      expect(projected).not.toContain(value)
    }
    expect(projected).toContain('[ACCOUNT]')
    expect(projected).toContain('[IDENTITY]')
    expect(projected).toContain('[PHONE]')
    expect(projected).toContain('[EMAIL]')
    expect(projected).toContain('[IP]')
    expect(projected).toContain('[DEVICE]')
  })

  it('uses one ordinary public projection for credentials and restricted identifiers', () => {
    const projected = projectOrdinaryPublicText(
      'Authorization: Bearer secret-token account=6222020202020202020'
    )
    expect(projected).toBe('Authorization=<redacted> account=[ACCOUNT]')
    expect(projected).not.toContain('secret-token')
    expect(projected).not.toContain('6222020202020202020')
  })

  it('detects PII in root text and public text carriers', () => {
    expect(containsOrdinaryPublicPII('account=6222020202020202020')).toBe(true)
    expect(containsOrdinaryPublicPII({
      kind: 'tool_progress',
      message: 'contact investigator@example.com'
    })).toBe(true)
    expect(containsOrdinaryPublicPII({
      kind: 'usage',
      totalTokens: 12,
      contextDigest: 'a'.repeat(64)
    })).toBe(false)
  })

  it('keeps a canonical plan content digest opaque without exempting text or numeric identifiers', () => {
    const digest = `ab${'1'.repeat(60)}cd`

    expect(containsOrdinaryPublicPII({
      output: { plan: { contentHash: digest } }
    })).toBe(false)
    expect(containsOrdinaryPublicPII({ message: digest })).toBe(true)
    expect(containsOrdinaryPublicPII({
      output: { plan: { contentHash: '1'.repeat(64) } }
    })).toBe(true)
  })

  it('keeps every schema-valid closed tool-manifest digest opaque without exempting arbitrary detail text', () => {
    const digest = `ab${'1'.repeat(60)}cd`
    const diagnostic = {
      details: {
        rejectedToolNormalizedNameSha256: digest,
        advertisedToolManifestHash: digest,
        advertisedNameSetSortedHash: digest,
        providerRequestToolManifestHash: digest,
        runToolStepManifestHash: digest
      }
    }

    expect(containsOrdinaryPublicPII(diagnostic)).toBe(false)
    expect(containsOrdinaryPublicPII({
      details: { rejectedToolNormalizedNameSha256: '1'.repeat(64) }
    })).toBe(false)
    expect(containsOrdinaryPublicPII({ details: { arbitraryHash: digest } })).toBe(true)
  })

  it.each([
    ['email', 'victim@example.com'],
    ['personName', '张三'],
    ['address', '北京市朝阳区建国路'],
    ['deviceFingerprint', 'opaque-device-fingerprint'],
    ['accountId', 'opaque-account-reference']
  ])('fails closed for non-empty typed PII field %s', (key, value) => {
    expect(containsOrdinaryPublicPII({ [key]: value })).toBe(true)
  })

  it('admits only canonical masked values in typed PII fields', () => {
    for (const value of [
      '<redacted>',
      '<redacted-identifier:7890>',
      '[ACCOUNT]',
      '[IDENTITY]',
      '[PHONE]',
      '[EMAIL]',
      '[IP]',
      '[DEVICE]',
      '[PERSON]',
      '[ADDRESS]'
    ]) {
      expect(containsOrdinaryPublicPII({ account: value })).toBe(false)
    }
    expect(containsOrdinaryPublicPII({ account: '' })).toBe(false)
    expect(containsOrdinaryPublicPII({ account: null })).toBe(false)
  })

  it('fails closed on cyclic, over-depth, and over-budget public values', () => {
    const cyclic: Record<string, unknown> = { message: 'safe' }
    cyclic.self = cyclic
    expect(containsOrdinaryPublicPII(cyclic)).toBe(true)

    let deep: Record<string, unknown> = { message: 'safe' }
    for (let index = 0; index < 40; index += 1) deep = { child: deep }
    expect(containsOrdinaryPublicPII(deep)).toBe(true)

    expect(containsOrdinaryPublicPII({ values: new Array(100_001).fill('safe') })).toBe(true)
  })
})
