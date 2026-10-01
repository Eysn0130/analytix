import assert from 'node:assert/strict'
import { generateKeyPairSync, sign } from 'node:crypto'
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { after, before, test } from 'node:test'
import { createRequire } from 'node:module'

import {
  putObject,
  RELEASE_PUBLICATION_AUTHORITY_CONTRACT,
  releasePublicationAuthorityDigest,
  releasePublicationAuthoritySignaturePayload,
  verifyReleasePublicationAuthority
} from './publish-r2.mjs'

const require = createRequire(import.meta.url)
const { _internals: packagedAuthorityContract } = require('./after-pack.cjs')
const TEAM_IDENTIFIER = 'AB12CD34EF'
const SOURCE_COMMIT = '1'.repeat(40)
const TAG = 'v9.9.9'
const CHANNEL = 'stable'

let fixture

function sha256(bytes) {
  return require('node:crypto').createHash('sha256').update(bytes).digest('hex')
}

function stableWrite(path, bytes) {
  writeFileSync(path, bytes, { mode: 0o600 })
  chmodSync(path, 0o600)
}

function cleanWorktreeSnapshot() {
  const current = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(process.cwd())
  const zeroSummary = { count: 0, byteLength: 0, sha256: '0'.repeat(64) }
  const withoutDigest = {
    ...current,
    sourceCommit: SOURCE_COMMIT,
    state: 'clean',
    dirty: false,
    stagedPatch: zeroSummary,
    unstagedPatch: zeroSummary,
    untrackedSource: zeroSummary
  }
  delete withoutDigest.snapshotDigest
  return {
    ...withoutDigest,
    snapshotDigest: packagedAuthorityContract.packagedWorktreeSnapshotDigest(withoutDigest)
  }
}

function artifactBinding(format, arch, seed) {
  return {
    preSignSha256: seed.repeat(64),
    preSignByteLength: 128,
    payloadSha256: seed.repeat(64),
    payloadByteLength: 128,
    format,
    arch
  }
}

function signReceipt(receiptWithoutDigestAndSignature, privateKey) {
  const receiptDigest = releasePublicationAuthorityDigest(receiptWithoutDigestAndSignature)
  const signature = sign(
    null,
    releasePublicationAuthoritySignaturePayload(receiptDigest),
    privateKey
  ).toString('base64')
  return { ...receiptWithoutDigestAndSignature, receiptDigest, signature }
}

function makeFixture(core = false) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-release-authority-'))
  const distDir = join(root, 'dist')
  require('node:fs').mkdirSync(distDir)
  const { publicKey, privateKey } = generateKeyPairSync('ed25519')
  const publicKeyPath = join(root, 'release-authority.pub')
  stableWrite(publicKeyPath, publicKey.export({ type: 'spki', format: 'pem' }))

  const targetKey = 'darwin-arm64'
  const golden = JSON.parse(readFileSync(join(process.cwd(), 'packages/runtime-go/internal/domain/packagedbuildauthority/testdata/packaged-build-authority-v2-js-golden.json'), 'utf8'))
  const packagedAuthority = packagedAuthorityContract.createPackagedBuildAuthorityV2({
    ...golden,
    worktreeSnapshot: cleanWorktreeSnapshot(),
    targetKey,
    nativeDisposition: {
      kind: core ? 'core_controlled_release' : 'controlled_release_receipt',
      targetKey,
      ...(core ? {} : { receiptSha256: '2'.repeat(64), manifestSha256: '3'.repeat(64) }),
      signingPolicySha256: '4'.repeat(64),
      signingMode: 'developer-id',
      appleTeamIdentifier: TEAM_IDENTIFIER
    },
    artifacts: {
      fundsPlugin: core ? require('./core-package-profile.cjs').ABSENT_FUNDS : golden.artifacts.fundsPlugin,
      executable: artifactBinding('mach-o', 'arm64', '5'),
      appAsar: { sha256: '6'.repeat(64), byteLength: 128 },
      runtimeServer: artifactBinding('mach-o', 'arm64', '7')
    }
  })
  assert.equal(packagedAuthorityContract.isPackagedBuildAuthorityV2(packagedAuthority), true)
  const packagedAuthorityFileName = 'analytix-packaged-build-authority-darwin-arm64.json'
  const packagedAuthorityPath = join(distDir, packagedAuthorityFileName)
  stableWrite(packagedAuthorityPath, JSON.stringify(packagedAuthority))

  const dmgFileName = `analytix-${core ? 'core-' : ''}9.9.9-mac-arm64.dmg`
  const dmgPath = join(distDir, dmgFileName)
  stableWrite(dmgPath, Buffer.from('signed-notarized-stapled-dmg-fixture', 'utf8'))
  const zipFileName = 'analytix-core-9.9.9-mac-arm64.zip'
  const zipBytes = Buffer.from('synthetic signed ZIP')
  if (core) stableWrite(join(distDir, zipFileName), zipBytes)
  const updateFileName = 'latest-mac.yml'
  const updatePath = join(distDir, updateFileName)
  stableWrite(updatePath, [
    'version: 9.9.9',
    ...(core ? ['analytixProfile: core', 'analytixTarget: darwin-arm64', 'analytixChannel: stable', 'analytixAppId: com.analytix.desktop', 'analytixDataCompatibility: core-v1'] : []),
    'files:',
    ...(core ? [`  - url: ${zipFileName}`, `    sha512: ${require('node:crypto').createHash('sha512').update(zipBytes).digest('base64')}`, `    size: ${zipBytes.length}`] : []),
    `  - url: ${dmgFileName}`,
    `    sha512: ${core ? require('node:crypto').createHash('sha512').update(readFileSync(dmgPath)).digest('base64') : 'fixture-only'}`,
    `    size: ${readFileSync(dmgPath).length}`,
    'releaseDate: 2026-07-23T00:00:00.000Z',
    ''
  ].join('\n'))

  const now = new Date('2026-07-23T10:00:00.000Z')
  const authorityBase = {
    schemaVersion: 1,
    contract: RELEASE_PUBLICATION_AUTHORITY_CONTRACT,
    keyId: 'test-release-ed25519',
    sourceCommit: SOURCE_COMMIT,
    tag: TAG,
    channel: CHANNEL,
    issuedAt: '2026-07-23T09:55:00.000Z',
    expiresAt: '2026-07-23T10:55:00.000Z',
    releaseEligible: true,
    publicationReceiptIssued: true,
    platforms: ['mac'],
    packagedBuildAuthorities: [{
      platform: 'mac',
      targetKey,
      fileName: packagedAuthorityFileName,
      byteLength: readFileSync(packagedAuthorityPath).length,
      sha256: sha256(readFileSync(packagedAuthorityPath)),
      authorityDigest: packagedAuthority.authorityDigest
    }],
    artifacts: [dmgFileName, updateFileName, ...(core ? [zipFileName] : [])].sort().map((fileName) => {
      const bytes = readFileSync(join(distDir, fileName))
      return {
        platform: 'mac',
        fileName,
        targetKeys: [targetKey],
        byteLength: bytes.length,
        sha256: sha256(bytes)
      }
    }),
    attestations: {
      mac: [{
        targetKey,
        developerIdVerified: true,
        teamIdentifier: TEAM_IDENTIFIER,
        secureTimestampVerified: true,
        notaryStatus: 'Accepted',
        notarySubmissionId: '00000000-0000-0000-0000-000000000001',
        stapled: true,
        stapleValidated: true
      }],
      win: [],
      linux: []
    }
  }
  const authorityPath = join(root, 'release-publication-authority.json')
  const writeAuthority = (overrides = {}, effectiveNow = now) => {
    const base = { ...authorityBase, ...overrides }
    const receipt = signReceipt(base, privateKey)
    stableWrite(authorityPath, JSON.stringify(receipt))
    return { receipt, effectiveNow }
  }
  writeAuthority()
  return {
    root,
    distDir,
    publicKeyPath,
    authorityPath,
    authorityBase,
    privateKey,
    now,
    dmgPath,
    writeAuthority
  }
}

before(() => {
  fixture = makeFixture()
})

after(() => {
  rmSync(fixture.root, { recursive: true, force: true })
})

test('accepts a signed, fresh, exact controlled release fixture without claiming a real release', async () => {
  fixture.writeAuthority()
  const result = await verifyReleasePublicationAuthority({
    authorityPath: fixture.authorityPath,
    publicKeyPath: fixture.publicKeyPath,
    tag: TAG,
    channel: CHANNEL,
    sourceCommit: SOURCE_COMMIT,
    platforms: ['mac'],
    distDirs: { mac: fixture.distDir },
    officialTeamIdentifier: TEAM_IDENTIFIER,
    trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
    now: fixture.now
  })
  assert.equal(result.authority.releaseEligible, true)
  assert.deepEqual(result.platforms, ['mac'])
})

test('rejects a caller-provided public key that is not the configured trust anchor', async () => {
  fixture.writeAuthority()
  await assert.rejects(
    verifyReleasePublicationAuthority({
      authorityPath: fixture.authorityPath,
      publicKeyPath: fixture.publicKeyPath,
      tag: TAG,
      channel: CHANNEL,
      sourceCommit: SOURCE_COMMIT,
      platforms: ['mac'],
      distDirs: { mac: fixture.distDir },
      officialTeamIdentifier: TEAM_IDENTIFIER,
      trustedPublicKeySha256: 'f'.repeat(64),
      now: fixture.now
    }),
    /release_publication_authority_trust_anchor_mismatch/
  )
})

test('rejects missing authority before an upload or promote network action', () => {
  const cleanEnv = { ...process.env }
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY
  delete cleanEnv.ANALYTIX_RELEASE_ENV
  for (const command of [
    ['upload', '--platform', 'mac', '--tag', TAG, '--dist', fixture.distDir, '--dry-run'],
    ['promote', '--platforms', 'mac', '--tag', TAG, '--dry-run']
  ]) {
    const result = spawnSync(process.execPath, [join(process.cwd(), 'scripts/publish-r2.mjs'), ...command], {
      cwd: process.cwd(),
      env: cleanEnv,
      encoding: 'utf8'
    })
    assert.notEqual(result.status, 0)
    assert.match(result.stderr, /release_publication_authority_missing/)
    assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, /\[dry-run\] (?:put|copy)/)
  }
})

test('production upload does not trust a caller-supplied authority key', () => {
  const cleanEnv = { ...process.env, ANALYTIX_RELEASE_ENV: join(fixture.root, 'missing-release.env') }
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY
  const result = spawnSync(process.execPath, [
    join(process.cwd(), 'scripts/publish-r2.mjs'),
    'upload', '--platform', 'mac', '--tag', TAG, '--dist', fixture.distDir, '--dry-run',
    '--authority', fixture.authorityPath, '--public-key', fixture.publicKeyPath
  ], {
    cwd: process.cwd(),
    env: cleanEnv,
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /release_publication_authority_trust_anchor_unavailable/)
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, /\[dry-run\] put/)
})

test('formal mac release entry rejects missing authority before tag or release metadata mutation', () => {
  const tag = 'v99.99.98'
  const metaPath = join(process.cwd(), 'dist', '.release-meta.env')
  const metaBefore = existsSync(metaPath)
    ? { bytes: readFileSync(metaPath), mtimeMs: statSync(metaPath).mtimeMs }
    : null
  const cleanEnv = { ...process.env, ANALYTIX_RELEASE_ENV: join(fixture.root, 'missing-release.env') }
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY
  const result = spawnSync('bash', [join(process.cwd(), 'scripts/release-mac.sh'), '--tag', tag], {
    cwd: process.cwd(),
    env: cleanEnv,
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /release_publication_authority_missing/)
  const tagProbe = spawnSync('git', ['rev-parse', '-q', '--verify', `refs/tags/${tag}`], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  assert.notEqual(tagProbe.status, 0)
  if (metaBefore) {
    assert.deepEqual(readFileSync(metaPath), metaBefore.bytes)
    assert.equal(statSync(metaPath).mtimeMs, metaBefore.mtimeMs)
  } else {
    assert.equal(existsSync(metaPath), false)
  }
})

test('local non-publishable package path rejects tag or publication authority before building', () => {
  const tag = 'v99.99.97'
  const cleanEnv = { ...process.env, ANALYTIX_RELEASE_ENV: join(fixture.root, 'missing-release.env') }
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY
  delete cleanEnv.ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY
  const result = spawnSync('bash', [
    join(process.cwd(), 'scripts/release-mac.sh'),
    '--local-nonpublishable', '--arch', 'arm64', '--tag', tag
  ], {
    cwd: process.cwd(),
    env: cleanEnv,
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /cannot accept a release tag or publication authority/)
  const tagProbe = spawnSync('git', ['rev-parse', '-q', '--verify', `refs/tags/${tag}`], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  assert.notEqual(tagProbe.status, 0)
})

test('rejects a cryptographically signed authority whose releaseEligible flag is false', async () => {
  fixture.writeAuthority({ releaseEligible: false })
  await assert.rejects(
    verifyReleasePublicationAuthority({
      authorityPath: fixture.authorityPath,
      publicKeyPath: fixture.publicKeyPath,
      tag: TAG,
      channel: CHANNEL,
      sourceCommit: SOURCE_COMMIT,
      platforms: ['mac'],
      distDirs: { mac: fixture.distDir },
      officialTeamIdentifier: TEAM_IDENTIFIER,
      trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
      now: fixture.now
    }),
    /release_publication_authority_invalid/
  )
})

for (const [name, attestationPatch] of [
  ['Developer ID is false', { developerIdVerified: false }],
  ['notary status is not accepted', { notaryStatus: 'Rejected' }],
  ['staple is missing', { stapled: false }],
  ['staple validation is false', { stapleValidated: false }]
]) {
  test(`rejects signed authority when ${name}`, async () => {
    const mac = [{ ...fixture.authorityBase.attestations.mac[0], ...attestationPatch }]
    fixture.writeAuthority({
      attestations: { mac, win: [], linux: [] }
    })
    await assert.rejects(
      verifyReleasePublicationAuthority({
        authorityPath: fixture.authorityPath,
        publicKeyPath: fixture.publicKeyPath,
        tag: TAG,
        channel: CHANNEL,
        sourceCommit: SOURCE_COMMIT,
        platforms: ['mac'],
        distDirs: { mac: fixture.distDir },
        officialTeamIdentifier: TEAM_IDENTIFIER,
        trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
        now: fixture.now
      }),
      /mac_release_attestation_invalid/
    )
  })
}

test('rejects tampered artifact bytes even when the receipt signature is valid', async () => {
  fixture.writeAuthority()
  const original = readFileSync(fixture.dmgPath)
  stableWrite(fixture.dmgPath, Buffer.concat([original, Buffer.from('tampered')]))
  try {
    await assert.rejects(
      verifyReleasePublicationAuthority({
        authorityPath: fixture.authorityPath,
        publicKeyPath: fixture.publicKeyPath,
        tag: TAG,
        channel: CHANNEL,
        sourceCommit: SOURCE_COMMIT,
        platforms: ['mac'],
        distDirs: { mac: fixture.distDir },
        officialTeamIdentifier: TEAM_IDENTIFIER,
        trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
        now: fixture.now
      }),
      /release_artifact_hash_mismatch/
    )
  } finally {
    stableWrite(fixture.dmgPath, original)
  }
})

test('rejects an expired signed authority', async () => {
  fixture.writeAuthority({
    issuedAt: '2026-07-22T08:00:00.000Z',
    expiresAt: '2026-07-22T09:00:00.000Z'
  })
  await assert.rejects(
    verifyReleasePublicationAuthority({
      authorityPath: fixture.authorityPath,
      publicKeyPath: fixture.publicKeyPath,
      tag: TAG,
      channel: CHANNEL,
      sourceCommit: SOURCE_COMMIT,
      platforms: ['mac'],
      distDirs: { mac: fixture.distDir },
      officialTeamIdentifier: TEAM_IDENTIFIER,
      trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
      now: fixture.now
    }),
    /release_publication_authority_stale/
  )
})

test('rejects a tampered receipt that was not re-signed', async () => {
  const { receipt } = fixture.writeAuthority()
  stableWrite(fixture.authorityPath, JSON.stringify({ ...receipt, publicationReceiptIssued: false }))
  await assert.rejects(
    verifyReleasePublicationAuthority({
      authorityPath: fixture.authorityPath,
      publicKeyPath: fixture.publicKeyPath,
      tag: TAG,
      channel: CHANNEL,
      sourceCommit: SOURCE_COMMIT,
      platforms: ['mac'],
      distDirs: { mac: fixture.distDir },
      officialTeamIdentifier: TEAM_IDENTIFIER,
      trustedPublicKeySha256: sha256(readFileSync(fixture.publicKeyPath)),
      now: fixture.now
    }),
    /release_publication_authority_invalid/
  )
})

test('controlled Core publication requires signed exact authority and compatible update bytes', async () => {
  const core = makeFixture(true)
  const options = { authorityPath: core.authorityPath, publicKeyPath: core.publicKeyPath, tag: TAG, channel: CHANNEL, sourceCommit: SOURCE_COMMIT, platforms: ['mac'], distDirs: { mac: core.distDir }, officialTeamIdentifier: TEAM_IDENTIFIER, trustedPublicKeySha256: sha256(readFileSync(core.publicKeyPath)), now: core.now }
  try {
    const result = await verifyReleasePublicationAuthority(options)
    assert.equal(result.releaseProfile, 'core')
    // A test signature exercises the owner; it is not the production trust root.
    stableWrite(join(core.distDir, 'analytix-core-9.9.9-mac-arm64.zip'), Buffer.from('damaged archive'))
    await assert.rejects(verifyReleasePublicationAuthority(options), /digest_mismatch|hash_mismatch/)
  } finally { rmSync(core.root, {recursive:true}) }
})


test('version archive uploads never overwrite while channel pointers remain separately mutable', async () => {
  const seen=[]
  const config={bucket:'test-only',client:{send:async command=>{seen.push(command.input)}}}
  await putObject({config,key:'analytix/core/darwin-arm64/channels/stable/releases/v1.0.7/file.zip',body:'synthetic',dryRun:false})
  await putObject({config,key:'analytix/core/darwin-arm64/channels/stable/latest/latest-mac.yml',body:'synthetic',dryRun:false})
  assert.equal(seen[0].IfNoneMatch,'*')
  assert.equal(seen[1].IfNoneMatch,undefined)
  config.client.send=async()=>{throw Object.assign(Error('synthetic collision'),{name:'PreconditionFailed'})}
  await assert.rejects(putObject({config,key:'analytix/channels/stable/releases/v1.0.7/file.zip',body:'new bytes',dryRun:false}),/collision/)
})
