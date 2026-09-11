import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE,
  DOCUMENT_RUNTIME_MANIFEST_FILE,
  DOCUMENT_RUNTIME_UNAVAILABLE,
  verifyPackagedDocumentRuntime
} from './document-runtime-authority'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getAppPath: () => '/Applications/Analytix.app/Contents/Resources/app.asar'
  }
}))

import { resolveDataAnalysisBackendLaunchEnvironment } from './document-runtime-launch-environment'

const require = createRequire(import.meta.url)
const documentContract = require('../../../scripts/document-runtime-contract.cjs') as Record<string, any>
const nativeContract = require('../../../scripts/native-component-contract.cjs') as Record<string, any>
const afterPack = require('../../../scripts/after-pack.cjs') as Record<string, any>

const roots: string[] = []

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function tempRoot(): string {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-document-runtime-')))
  roots.push(root)
  return root
}

function sha256(value: Buffer | string): string {
  return createHash('sha256').update(value).digest('hex')
}

function nativeFixture(
  format: 'mach-o' | 'pe',
  arch: 'arm64' | 'x64',
  marker: string,
  importedLibrary = ''
): Buffer {
  if (format === 'mach-o') {
    const bytes = Buffer.alloc(0x220)
    bytes.writeUInt32LE(0xfeedfacf, 0)
    bytes.writeUInt32LE(arch === 'arm64' ? 0x0100000c : 0x01000007, 4)
    bytes.writeUInt32LE(2, 12)
    const importSize = importedLibrary ? Math.ceil((24 + Buffer.byteLength(importedLibrary) + 1) / 8) * 8 : 0
    bytes.writeUInt32LE(importedLibrary ? 4 : 3, 16)
    bytes.writeUInt32LE(160 + importSize, 20)
    bytes.writeUInt32LE(0x19, 32)
    bytes.writeUInt32LE(72, 36)
    bytes.write('__TEXT', 40, 'ascii')
    bytes.writeBigUInt64LE(0x180n, 80)
    bytes.writeUInt32LE(7, 88)
    bytes.writeUInt32LE(5, 92)
    const linkEdit = 104
    bytes.writeUInt32LE(0x19, linkEdit)
    bytes.writeUInt32LE(72, linkEdit + 4)
    bytes.write('__LINKEDIT', linkEdit + 8, 'ascii')
    bytes.writeBigUInt64LE(0x180n, linkEdit + 40)
    bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), linkEdit + 32)
    bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), linkEdit + 48)
    bytes.writeUInt32LE(1, linkEdit + 56)
    bytes.writeUInt32LE(1, linkEdit + 60)
    let signature = 176
    if (importedLibrary) {
      bytes.writeUInt32LE(0x0c, signature)
      bytes.writeUInt32LE(importSize, signature + 4)
      bytes.writeUInt32LE(24, signature + 8)
      bytes.writeUInt32LE(0, signature + 12)
      bytes.writeUInt32LE(0, signature + 16)
      bytes.writeUInt32LE(0, signature + 20)
      bytes.write(importedLibrary, signature + 24, 'utf8')
      signature += importSize
    }
    bytes.writeUInt32LE(0x1d, signature)
    bytes.writeUInt32LE(16, signature + 4)
    bytes.writeUInt32LE(0x200, signature + 8)
    bytes.writeUInt32LE(0x20, signature + 12)
    Buffer.from(marker).copy(bytes, 0x1c0, 0, 48)
    bytes.writeUInt32BE(0xfade0cc0, 0x200)
    bytes.writeUInt32BE(0x20, 0x204)
    bytes.writeUInt32BE(1, 0x208)
    bytes.writeUInt32BE(0, 0x20c)
    bytes.writeUInt32BE(0x14, 0x210)
    return bytes
  }
  const bytes = Buffer.alloc(0x400)
  bytes.write('MZ', 0, 'ascii')
  const peOffset = 0x80
  bytes.writeUInt32LE(peOffset, 0x3c)
  bytes.write('PE\0\0', peOffset, 'ascii')
  bytes.writeUInt16LE(arch === 'arm64' ? 0xaa64 : 0x8664, peOffset + 4)
  bytes.writeUInt16LE(1, peOffset + 6)
  bytes.writeUInt16LE(0xf0, peOffset + 20)
  bytes.writeUInt16LE(0x22, peOffset + 22)
  const optionalHeader = peOffset + 24
  bytes.writeUInt16LE(0x20b, optionalHeader)
  bytes.writeUInt32LE(0x1000, optionalHeader + 16)
  bytes.writeUInt32LE(0x1000, optionalHeader + 32)
  bytes.writeUInt32LE(0x200, optionalHeader + 36)
  bytes.writeUInt32LE(0x2000, optionalHeader + 56)
  bytes.writeUInt32LE(0x200, optionalHeader + 60)
  bytes.writeUInt32LE(16, optionalHeader + 108)
  if (importedLibrary) {
    bytes.writeUInt32LE(0x1100, optionalHeader + 120)
    bytes.writeUInt32LE(40, optionalHeader + 124)
  }
  const section = optionalHeader + 0xf0
  bytes.write('.text', section, 'ascii')
  bytes.writeUInt32LE(0x100, section + 8)
  bytes.writeUInt32LE(0x1000, section + 12)
  bytes.writeUInt32LE(0x200, section + 16)
  bytes.writeUInt32LE(0x200, section + 20)
  bytes.writeUInt32LE(0x60000020, section + 36)
  Buffer.from(marker).copy(bytes, 0x280, 0, 64)
  if (importedLibrary) {
    bytes.writeUInt32LE(0x1150, 0x300 + 12)
    bytes.write(importedLibrary, 0x350, 'ascii')
  }
  return bytes
}

function writeFile(root: string, relativePath: string, bytes: Buffer | string, executable = false): void {
  const path = join(root, ...relativePath.split('/'))
  mkdirSync(join(path, '..'), { recursive: true })
  writeFileSync(path, bytes, { mode: executable ? 0o700 : 0o600 })
}

function writeFixture(platform: 'darwin' | 'win32', arch: 'arm64' | 'x64'): {
  repoRoot: string
  assetRoot: string
  resourcesRoot: string
  packContext: Record<string, any>
  target: Record<string, any>
  manifest: Record<string, any>
  lock: Record<string, any>
  lockBytes: Buffer
} {
  const repoRoot = tempRoot()
  const target = nativeContract.targetContract(platform, arch)
  const assetRoot = join(repoRoot, 'authorized-assets')
  const appOutDir = join(repoRoot, 'package-out')
  const resourcesRoot = platform === 'darwin'
    ? join(appOutDir, 'Analytix.app', 'Contents', 'Resources')
    : join(appOutDir, 'resources')
  const packContext = {
    appOutDir,
    electronPlatformName: platform,
    arch,
    packager: { appInfo: { productFilename: 'Analytix' } }
  }
  const extension = platform === 'win32' ? '.exe' : ''
  const format = platform === 'darwin' ? 'mach-o' : 'pe'
  const soffice = `bin/soffice${extension}`
  const tesseract = `bin/tesseract${extension}`
  const library = `lib/document-runtime-helper${platform === 'win32' ? '.dll' : '.dylib'}`
  const importName = platform === 'win32'
    ? 'document-runtime-helper.dll'
    : '@loader_path/../lib/document-runtime-helper.dylib'
  writeFile(assetRoot, soffice, nativeFixture(format, arch, 'soffice', importName), true)
  writeFile(assetRoot, tesseract, nativeFixture(format, arch, 'tesseract', importName), true)
  writeFile(assetRoot, library, nativeFixture(format, arch, 'library'), true)
  writeFile(assetRoot, 'share/tessdata/chi_sim.traineddata', 'authorized-chi-sim-data')
  writeFile(assetRoot, 'share/tessdata/eng.traineddata', 'authorized-eng-data')
  writeFile(assetRoot, 'licenses/document-runtime.LICENSE', 'Authorized fixture license\n')
  const source = {
    id: 'document-runtime-source',
    name: 'Document Runtime Fixture',
    version: '1.0.0',
    sourceUri: 'https://example.invalid/document-runtime-1.0.0.tar.zst',
    archiveSha256: sha256('authorized-source-archive')
  }
  const license = {
    id: 'document-runtime-license',
    spdxExpression: 'Apache-2.0',
    sourceId: source.id,
    path: 'licenses/document-runtime.LICENSE'
  }
  const metadata = (path: string, kind: string, dependencies: string[] = []) => ({
    path,
    kind,
    sourceId: source.id,
    licenseId: license.id,
    dependencies
  })
  const manifest = documentContract.createDocumentRuntimeManifest(assetRoot, target, {
    sources: [source],
    licenses: [license],
    capabilities: {
      sofficeBinary: soffice,
      tesseractBinary: tesseract,
      tessdataDirectory: 'share/tessdata',
      ocrLanguages: ['chi_sim', 'eng']
    },
    files: [
      metadata(soffice, 'binary', [library]),
      metadata(tesseract, 'binary', [library]),
      metadata(library, 'library'),
      metadata('share/tessdata/chi_sim.traineddata', 'data'),
      metadata('share/tessdata/eng.traineddata', 'data'),
      metadata('licenses/document-runtime.LICENSE', 'license')
    ]
  }) as Record<string, any>
  const manifestBytes = Buffer.from(JSON.stringify(manifest), 'utf8')
  writeFile(assetRoot, DOCUMENT_RUNTIME_MANIFEST_FILE, manifestBytes)
  const lock = {
    schemaVersion: 1,
    contract: documentContract.AUTHORITY_LOCK_CONTRACT,
    targets: [documentContract.createAuthorityLockEntry(manifest, sha256(manifestBytes))]
  }
  const lockBytes = Buffer.from(JSON.stringify(lock), 'utf8')
  writeFile(assetRoot, DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE, lockBytes)
  return { repoRoot, assetRoot, resourcesRoot, packContext, target, manifest, lock, lockBytes }
}

describe('document runtime packaging authority', () => {
  it.each([
    ['macOS arm64 package layout', 'darwin', 'arm64'],
    ['Windows x64 contract only', 'win32', 'x64']
  ] as const)('binds, atomically materializes, and injects only a verified %s', (
    _label,
    platform,
    arch
  ) => {
    const fixture = writeFixture(platform, arch)
    expect(documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: fixture.lock }
    )).toEqual(expect.objectContaining({
      targetKey: fixture.target.key,
      treeSha256: fixture.manifest.treeSha256
    }))

    const materialized = afterPack._internals.materializePackagedDocumentRuntime(fixture.packContext, {
      repoRoot: fixture.repoRoot,
      assetRoot: fixture.assetRoot,
      authorityLock: fixture.lock,
      required: true,
      env: {}
    }) as Record<string, any>
    expect(materialized).toEqual(expect.objectContaining({
      available: true,
      targetKey: fixture.target.key,
      treeSha256: fixture.manifest.treeSha256
    }))

    const authority = verifyPackagedDocumentRuntime({
      resourcesRoot: fixture.resourcesRoot,
      platform,
      arch,
      testOnlyAuthorityLock: fixture.lockBytes
    })
    expect(authority).toEqual(expect.objectContaining({
      targetKey: fixture.target.key,
      treeSha256: fixture.manifest.treeSha256
    }))
    const launch = resolveDataAnalysisBackendLaunchEnvironment({
      env: {
        ANALYTIX_DOCUMENT_SOFFICE_BIN: '/host/soffice',
        ANALYTIX_DOCUMENT_TESSERACT_BIN: '/host/tesseract',
        TESSDATA_PREFIX: '/host/tessdata',
        PATH: '/host/bin',
        LANG: 'C.UTF-8'
      },
      resourcesRoot: fixture.resourcesRoot,
      packaged: true,
      platform,
      arch,
      testOnlyDocumentAuthorityLock: fixture.lockBytes
    })
    expect(launch).toEqual(expect.objectContaining({
      documentRuntimeAvailable: true,
      documentRuntimeBlocker: ''
    }))
    expect(launch.env.ANALYTIX_DOCUMENT_SOFFICE_BIN).toBe(authority.sofficeBinary)
    expect(launch.env.ANALYTIX_DOCUMENT_TESSERACT_BIN).toBe(authority.tesseractBinary)
    expect(launch.env.TESSDATA_PREFIX).toBe(authority.tessdataDirectory)
    expect(JSON.stringify(fixture.manifest)).not.toContain(fixture.repoRoot)
  })

  it('rejects a self-minted manifest that is absent from the frozen per-target authority lock', () => {
    const fixture = writeFixture('darwin', 'arm64')
    expect(() => documentContract.verifyDocumentRuntime(fixture.assetRoot, fixture.target)).toThrow(
      new RegExp(DOCUMENT_RUNTIME_UNAVAILABLE)
    )
    expect(() => documentContract.materializeDocumentRuntime({
      repoRoot: fixture.repoRoot,
      resourcesRoot: fixture.resourcesRoot,
      platform: 'darwin',
      arch: 'arm64',
      assetRoot: fixture.assetRoot,
      required: true,
      env: {}
    })).toThrow(new RegExp(DOCUMENT_RUNTIME_UNAVAILABLE))
  })

  it('makes a missing asset root unavailable without PATH, home, or system fallback', () => {
    const root = tempRoot()
    const resourcesRoot = join(root, 'resources')
    const unavailable = documentContract.materializeDocumentRuntime({
      repoRoot: root,
      resourcesRoot,
      platform: 'darwin',
      arch: 'arm64',
      required: false,
      env: { PATH: '/opt/homebrew/bin', HOME: '/Users/private' }
    })
    expect(unavailable).toEqual({
      available: false,
      blocker: DOCUMENT_RUNTIME_UNAVAILABLE,
      targetKey: 'darwin-arm64'
    })
    expect(existsSync(join(resourcesRoot, 'runtime', 'document-runtime'))).toBe(false)
    expect(() => documentContract.materializeDocumentRuntime({
      repoRoot: root,
      resourcesRoot,
      platform: 'darwin',
      arch: 'arm64',
      required: true,
      env: {}
    })).toThrow(new RegExp(DOCUMENT_RUNTIME_UNAVAILABLE))
  })

  it('rejects duplicate keys, extra inventory, tampering, and an unclosed library path', () => {
    const fixture = writeFixture('darwin', 'arm64')
    const manifestPath = join(fixture.assetRoot, DOCUMENT_RUNTIME_MANIFEST_FILE)
    const original = readFileSync(manifestPath, 'utf8')
    writeFileSync(manifestPath, original.replace('{', '{"schemaVersion":2,'), { mode: 0o600 })
    expect(() => documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: fixture.lock }
    )).toThrow(/duplicate keys/)

    writeFileSync(manifestPath, original, { mode: 0o600 })
    writeFile(fixture.assetRoot, 'unexpected.txt', 'not-authorized')
    expect(() => documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: fixture.lock }
    )).toThrow(/inventory/)
    rmSync(join(fixture.assetRoot, 'unexpected.txt'))

    writeFileSync(join(fixture.assetRoot, 'share/tessdata/eng.traineddata'), 'tampered', { mode: 0o600 })
    expect(() => documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: fixture.lock }
    )).toThrow(/identity mismatch/)

    const invalid = structuredClone(fixture.manifest)
    invalid.files.find((file: Record<string, any>) => file.path.includes('soffice')).dependencies = ['/usr/lib/escape.dylib']
    invalid.treeSha256 = documentContract.treeDigest(invalid)
    expect(documentContract.validateManifestShape(invalid, fixture.target)).toBe(false)
  })

  it('derives the dependency closure from native imports instead of trusting manifest declarations', () => {
    const fixture = writeFixture('darwin', 'arm64')
    const invalid = structuredClone(fixture.manifest)
    invalid.files.find((file: Record<string, any>) => file.path === 'bin/soffice').dependencies = []
    invalid.treeSha256 = documentContract.treeDigest(invalid)
    const manifestBytes = Buffer.from(JSON.stringify(invalid), 'utf8')
    writeFileSync(join(fixture.assetRoot, DOCUMENT_RUNTIME_MANIFEST_FILE), manifestBytes, { mode: 0o600 })
    const lock = {
      schemaVersion: 1,
      contract: documentContract.AUTHORITY_LOCK_CONTRACT,
      targets: [documentContract.createAuthorityLockEntry(invalid, sha256(manifestBytes))]
    }
    writeFileSync(
      join(fixture.assetRoot, DOCUMENT_RUNTIME_AUTHORITY_LOCK_FILE),
      JSON.stringify(lock),
      { mode: 0o600 }
    )
    expect(() => documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: lock }
    )).toThrow(/Native import closure mismatch/)
  })

  it('rejects an extra empty directory from the exact packaged tree', () => {
    const fixture = writeFixture('darwin', 'arm64')
    mkdirSync(join(fixture.assetRoot, 'unlisted-empty-directory'))
    expect(() => documentContract.verifyDocumentRuntime(
      fixture.assetRoot,
      fixture.target,
      { authorityLock: fixture.lock }
    )).toThrow(/inventory/)
  })

  it('clears ordinary host document runtime environment outside verified packaged authority', () => {
    const launch = resolveDataAnalysisBackendLaunchEnvironment({
      env: {
        ANALYTIX_DOCUMENT_SOFFICE_BIN: '/host/soffice',
        ANALYTIX_DOCUMENT_TESSERACT_BIN: '/host/tesseract',
        ANALYTIX_DOCUMENT_TEXTUTIL_BIN: '/usr/bin/textutil',
        ANALYTIX_DOCUMENT_ANTIWORD_BIN: '/host/antiword',
        TESSDATA_PREFIX: '/host/tessdata',
        LANG: 'C.UTF-8'
      },
      resourcesRoot: '/missing/resources',
      packaged: false,
      platform: 'darwin',
      arch: 'arm64'
    })
    expect(launch).toEqual({
      env: { LANG: 'C.UTF-8' },
      documentRuntimeAvailable: false,
      documentRuntimeBlocker: DOCUMENT_RUNTIME_UNAVAILABLE
    })
  })
})
