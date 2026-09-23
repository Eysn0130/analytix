import { describe, expect, it } from 'vitest'
import ordinaryPIICorpusJSON from '../../packages/runtime/src/conformance/fixtures/ordinary-pii-projection-v1.json'
import {
  containsInternalCaseEntityReference,
  containsOrdinaryPublicPII,
  containsProtectedCaseFactCandidate,
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
  it('projects after long incomplete escaped mentions without backtracking', () => {
    const incomplete = '$[](' + String.raw`\!`.repeat(5000)
    expect(projectOrdinaryLogPII(`${incomplete}\n/Users/private-owner/source.csv`)).toBe(`${incomplete}\n[PRIVATE_PATH]`)
  })

  it('preserves escaped URI punctuation while rejecting a dangling escape as a reference', () => {
    const valid = String.raw`@[Tool](plugin://report\)part)`
    expect(projectOrdinaryLogPII(valid)).toBe(valid)
    const incomplete = String.raw`@[Tool](plugin://report\)`
    expect(projectOrdinaryLogPII(incomplete)).not.toContain('plugin://')
  })

  it('projects absolute diff header paths without changing ordinary hunks or relative code paths', () => {
    const diff = '--- a//Users/private-owner/Case Files/source.csv\n+++ b//Users/private-owner/Case Files/source.csv\n@@ -1 +1 @@\n-old\n+new\n'
    expect(projectOrdinaryLogPII(diff)).toBe('--- [PRIVATE_PATH]\n+++ [PRIVATE_PATH]\n@@ -1 +1 @@\n-old\n+new\n')
    const ordinary = '--- a/src/main.ts\n+++ b/src/main.ts\n@@ -1 +1 @@\n-old\n+new\n'
    expect(projectOrdinaryLogPII(ordinary)).toBe(ordinary)
  })
  it('preserves validated composer references while projecting labels and surrounding prose', () => {
    const text = String.raw`@[Report \[tool\]](plugin://report) $[Audit](skill://audit%2Fskill) /Users/private-owner/source.csv`
    const expected = String.raw`@[Report \[tool\]](plugin://report) $[Audit](skill://audit%2Fskill) [PRIVATE_PATH]`
    expect(projectOrdinaryLogPII(text)).toBe(expected)
    expect(projectOrdinaryLogPII(expected)).toBe(expected)
    expect(projectOrdinaryLogPII('@[/Users/private-owner/source.csv](plugin://report)')).toBe(String.raw`@[\[PRIVATE_PATH\]](plugin://report)`)
  })

  it.each(['%2FUsers%2Fprivate-owner%2Fsource.csv', '%252FUsers%252Fprivate-owner%252Fsource.csv',
    '13800138000', 'alice%40example.com', 'api_key%3Dsyntheticsecret', '%ZZ', ''])('withholds an unsafe encoded reference ID %s', (id) => {
    expect(projectOrdinaryLogPII(`@[Tool](plugin://${id})`)).toBe('Tool [PRIVATE_REFERENCE]')
  })

  it.each(['cer1_' + 'a'.repeat(64), 'srow1_' + 'a'.repeat(64), String.raw`\u0063er1_` + 'a'.repeat(64),
    '{"purpose":"analytix.raw-artifact-source-locator/v1"}'])('withholds encoded private authority in a reference', (id) => {
    expect(projectOrdinaryLogPII(`@[Tool](plugin://${encodeURIComponent(id)})`)).toBe('Tool [PRIVATE_REFERENCE]')
  })

  it.each([
    '/Users/private-owner/SYNTHETIC-PII.csv',
    '/Volumes/source/notes.md',
    '/private/source/notes.md',
    '/var/source/notes.md',
    '/tmp/source/notes.md',
    '/home/owner/notes.md',
    '/case/source.csv',
    '/cases/source.csv',
    String.raw`C:\Users\private-owner\source.csv`,
    'D:/private-owner/source.csv',
    'file:///Users/private-owner/source.csv',
    'file://server/share/source.csv',
    '~/source.csv',
    '~owner/source.csv',
    String.raw`\\server\share\source.csv`,
    '//server/share/source.csv'
  ])('masks private source locator %s only in ordinary text output', (locator) => {
    const text = `source=${locator}\nordinary document context`
    expect(projectOrdinaryPublicText(text)).toBe('source=[PRIVATE_PATH]\nordinary document context')
    expect(projectOrdinaryLogPII(text)).toBe('source=[PRIVATE_PATH]\nordinary document context')
    expect(projectOrdinaryPublicText(projectOrdinaryPublicText(text))).toBe(projectOrdinaryPublicText(text))
    expect(containsOrdinaryPublicPII(locator)).toBe(false)
    expect(containsProtectedCaseFactCandidate(locator)).toBe(false)
  })

  it.each(['"', "'", '`'])('masks the complete private locator inside closed %s quotes', (quote) => {
    for (const locator of [
      '/Users/private-owner/Case Files/SYNTHETIC-PII.csv',
      '/Users/private-owner/Case Files/SYNTHETIC-PII (copy) [final].csv',
      String.raw`C:\Users\private-owner\Case Files\SYNTHETIC-PII (copy).csv`,
      'file:///Users/private-owner/Case Files/SYNTHETIC-PII.csv'
    ]) {
      const text = `source=${quote}${locator}${quote} ordinary context`
      const expected = `source=${quote}[PRIVATE_PATH]${quote} ordinary context`
      expect(projectOrdinaryPublicText(text)).toBe(expected)
      expect(projectOrdinaryLogPII(text)).toBe(expected)
      expect(projectOrdinaryPublicText(expected)).toBe(expected)
    }
  })

  it('includes parentheses and brackets in unquoted private filenames', () => {
    const text = '/Users/private-owner/SYNTHETIC-PII(copy)[final].csv ordinary context'
    expect(projectOrdinaryPublicText(text)).toBe('[PRIVATE_PATH] ordinary context')
    expect(projectOrdinaryLogPII(text)).toBe('[PRIVATE_PATH] ordinary context')
  })

  it.each(['\n', '\r\n'])('does not let a quoted locator consume the next line (%j)', (newline) => {
    const text = `"/Users/private-owner/source.csv${newline}ordinary document context"`
    expect(projectOrdinaryPublicText(text)).toBe(`"[PRIVATE_PATH]${newline}ordinary document context"`)
  })

  it.each(['"', "'", '`'])('preserves quoted relative paths and HTTPS URLs (%s)', (quote) => {
    for (const value of ['src/Case Files/note (copy) [final].md', 'https://example.com/home/file(copy)[final]']) {
      const text = `${quote}${value}${quote}`
      expect(projectOrdinaryPublicText(text)).toBe(text)
      expect(projectOrdinaryLogPII(text)).toBe(text)
    }
  })

  it.each([
    'src/main/index.ts', './src/main/index.ts', '../notes.md', '/plan',
    'https://example.com/home/file', 'https://example.com/Users/guide.md',
    'https://example.com/?next=/private/guide.md'
  ])('preserves ordinary code paths, commands, and HTTPS URL %s', (text) => {
    expect(projectOrdinaryPublicText(text)).toBe(text)
    expect(projectOrdinaryLogPII(text)).toBe(text)
  })

  it('does not let an HTTPS URL exempt a separate private locator or embedded PII', () => {
    expect(projectOrdinaryPublicText('https://example.com/home/file /Users/private-owner/source.csv')).toBe(
      'https://example.com/home/file [PRIVATE_PATH]'
    )
    expect(projectOrdinaryPublicText('https://example.com/home/13800138000.csv')).toBe(
      'https://example.com/home/[PHONE].csv'
    )
  })

  it('masks Write context locators without changing the prompt markers or safe excerpt', () => {
    const text = '[相关文献上下文]\n[1] notes.md:1-2\n路径：`/Users/private-owner/SYNTHETIC-PII.csv`\nordinary document context\n[/相关文献上下文]'
    expect(projectOrdinaryPublicText(text)).toBe(
      '[相关文献上下文]\n[1] notes.md:1-2\n路径：`[PRIVATE_PATH]`\nordinary document context\n[/相关文献上下文]'
    )
  })

  it('retains PII risk detection inside paths and does not mutate typed path authority', () => {
    const path = '/Users/private-owner/13800138000.csv'
    expect(containsOrdinaryPublicPII(path)).toBe(true)
    expect(containsProtectedCaseFactCandidate(path)).toBe(true)
    expect(containsOrdinaryPublicPII({ path })).toBe(true)
    expect(projectOrdinaryPublicText(path)).toBe('[PRIVATE_PATH]')

    const metadata = { workspace: '/Users/private-owner/workspace', path: '/tmp/source.csv', relativePath: 'src/main.ts' }
    const before = structuredClone(metadata)
    expect(containsOrdinaryPublicPII(metadata)).toBe(false)
    expect(metadata).toEqual(before)
  })

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

  it('only treats exact host user-input question IDs as structural outside prose', () => {
    const inputId = `input_${'123456789012'}${'a'.repeat(52)}`
    const question = { id: `${inputId}_1`, header: 'Path', question: 'Choose a path', options: [] }
    const item = { kind: 'user_input', role: 'system', status: 'pending', inputId, questions: [question] }
    expect(containsOrdinaryPublicPII(item)).toBe(false)
    expect(containsOrdinaryPublicPII({ message: item })).toBe(true)
    expect(containsOrdinaryPublicPII({ ...item,
      questions: [{ ...question, id: `other_${'123456789012'}` }]
    })).toBe(true)
    expect(containsOrdinaryPublicPII({ ...item,
      questions: [{ ...question, question: 'account=6222020202020202020' }]
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
