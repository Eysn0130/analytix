import assert from 'node:assert/strict'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { test } from 'node:test'

const root = process.cwd()
const bashPath = join(root, 'scripts', 'release-win.sh')
const powershellPath = join(root, 'scripts', 'release-win.ps1')
const bashSource = readFileSync(bashPath, 'utf8')
const powershellSource = readFileSync(powershellPath, 'utf8')

function cleanReleaseEnv() {
  const env = { ...process.env }
  for (const name of [
    'ANALYTIX_RELEASE_AUTHORITY',
    'ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY',
    'ANALYTIX_RELEASE_ENV',
    'WIN_CSC_LINK',
    'CSC_LINK',
    'ANALYTIX_WIN_SIGNING_CERT_PATH',
    'WIN_CSC_KEY_PASSWORD',
    'CSC_KEY_PASSWORD',
    'ANALYTIX_WIN_SIGNING_CERT_PASSWORD',
    'ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1',
    'ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA',
    'R2_UPLOAD',
    'R2_PROMOTE'
  ]) {
    delete env[name]
  }
  return env
}

function fileSnapshot(path) {
  if (!existsSync(path)) return null
  return {
    bytes: readFileSync(path),
    mtimeMs: statSync(path).mtimeMs
  }
}

function assertSnapshot(path, before) {
  if (before === null) {
    assert.equal(existsSync(path), false, `${path} was created before authority admission`)
    return
  }
  assert.deepEqual(readFileSync(path), before.bytes, `${path} changed before authority admission`)
  assert.equal(statSync(path).mtimeMs, before.mtimeMs, `${path} mtime changed before authority admission`)
}

test('formal Windows bash entry verifies controlled authority before any write-capable release step', () => {
  const formalCall = bashSource.indexOf('\nverify_formal_publication_authority\n')
  assert.ok(formalCall > 0)
  for (const marker of [
    'release_acquire_lock',
    'audit-windows-package.cjs',
    'release_write_notes_file',
    'release_write_local_release_index',
    'publish-r2.mjs" upload',
    'publish-r2.mjs" promote'
  ]) {
    const index = bashSource.indexOf(marker, formalCall)
    assert.ok(index > formalCall, `${marker} must remain after the formal authority preflight`)
  }
  assert.doesNotMatch(bashSource, /npm run dist:win:official/)
  assert.doesNotMatch(bashSource, /release_clean_dist_artifacts/)
  assert.match(bashSource, /publish-r2\.mjs" verify[\s\S]*--platform win/)
  assert.match(bashSource, /upload[\s\S]*--authority "\$\{RELEASE_AUTHORITY_PATH\}"[\s\S]*--public-key/)
  assert.match(bashSource, /promote[\s\S]*--platforms win[\s\S]*--authority/)
})

test('formal Windows bash entry rejects missing authority without metadata or lock mutation', () => {
  const paths = [
    join(root, 'dist', '.release-meta.env'),
    join(root, 'dist', '.release-assets.txt')
  ]
  const before = paths.map(fileSnapshot)
  const lockPath = join(root, '.cache', 'release.lock')
  const lockBefore = existsSync(lockPath)
  const result = spawnSync('bash', [bashPath, '--tag', 'v99.99.96'], {
    cwd: root,
    env: cleanReleaseEnv(),
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(`${result.stdout}\n${result.stderr}`, /release_publication_authority_missing/)
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, /Building official|npm run dist:win:official|Uploading|Promoting/)
  paths.forEach((path, index) => assertSnapshot(path, before[index]))
  assert.equal(existsSync(lockPath), lockBefore, 'release lock state changed before authority admission')
})

test('formal Windows bash entry rejects missing signing credentials before release mutation', () => {
  const paths = [
    join(root, 'dist', '.release-meta.env'),
    join(root, 'dist', '.release-assets.txt')
  ]
  const before = paths.map(fileSnapshot)
  const lockPath = join(root, '.cache', 'release.lock')
  const lockBefore = existsSync(lockPath)
  const result = spawnSync('bash', [
    bashPath,
    '--tag',
    'v99.99.94',
    '--authority',
    join(root, 'missing-release-authority.json'),
    '--authority-public-key',
    join(root, 'missing-release-authority.pub')
  ], {
    cwd: root,
    env: cleanReleaseEnv(),
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(`${result.stdout}\n${result.stderr}`, /requires a signing certificate and password/)
  paths.forEach((path, index) => assertSnapshot(path, before[index]))
  assert.equal(existsSync(lockPath), lockBefore, 'release lock state changed before signing admission')
})

test('local non-publishable Windows bash path rejects release identity before host or build', () => {
  const result = spawnSync('bash', [
    bashPath,
    '--local-nonpublishable',
    '--tag',
    'v99.99.95'
  ], {
    cwd: root,
    env: cleanReleaseEnv(),
    encoding: 'utf8'
  })
  assert.notEqual(result.status, 0)
  assert.match(`${result.stdout}\n${result.stderr}`, /cannot accept a release tag or publication authority/)
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, /Building explicitly non-publishable/)
  assert.match(bashSource, /--development --platform win32 --arch x64/)
  assert.match(bashSource, /--publish never --win --dir --x64/)
  assert.match(bashSource, /must be isolated from formal release directories/)
  assert.match(bashSource, /ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY ANALYTIX_RELEASE_ENV/)
  assert.match(bashSource, /no release tag, metadata, installer\/update feed, archive, upload, or publication receipt/)
})

test('PowerShell entry keeps formal preflight and local non-publishable paths disjoint', () => {
  assert.match(powershellSource, /\[switch\]\$LocalNonPublishable/)
  assert.match(powershellSource, /\[string\]\$Authority/)
  assert.match(powershellSource, /\[string\]\$AuthorityPublicKey/)
  assert.match(powershellSource, /publish-r2\.mjs'\) verify[\s\S]*--platform win/)
  assert.doesNotMatch(powershellSource, /npm run dist:win:official/)
  assert.doesNotMatch(powershellSource, /Building official Standard Windows installer/)
  assert.doesNotMatch(powershellSource, /Load-LocalReleaseEnv|release\.local\.env|Set-Item -Path "Env:/)

  const formalStart = powershellSource.indexOf("if ($NoCache) {\n  Write-Err '-NoCache is only valid")
  const preflight = powershellSource.indexOf("publish-r2.mjs') verify", formalStart)
  const firstReleaseWrite = powershellSource.indexOf("audit-windows-package.cjs", preflight)
  assert.ok(formalStart > 0 && preflight > formalStart && firstReleaseWrite > preflight)
  const formalPrefix = powershellSource.slice(formalStart, preflight)
  assert.doesNotMatch(formalPrefix, /Set-Content|New-Item|Remove-Item|dist:win:official/)

  assert.match(powershellSource, /--development --platform win32 --arch x64/)
  assert.match(powershellSource, /--publish never --win --dir --x64/)
  assert.match(powershellSource, /must be isolated from formal release directories/)
  assert.match(powershellSource, /'ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY', 'ANALYTIX_RELEASE_ENV'/)
  assert.match(powershellSource, /upload[\s\S]*--authority \$ReleaseAuthority[\s\S]*--public-key/)
  assert.match(powershellSource, /promote[\s\S]*--platforms win[\s\S]*--authority/)
})
