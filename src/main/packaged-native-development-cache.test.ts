import { createHash } from 'node:crypto'
import {
  chmodSync,
  copyFileSync,
  existsSync,
  linkSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  renameSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync
} from 'node:fs'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'

const require = createRequire(import.meta.url)
const afterPack = require('../../scripts/after-pack.cjs') as {
  NATIVE_DEVELOPMENT_BUILD_MARKER: string
  NATIVE_DISPOSITION_DEVELOPMENT: string
  _internals: Record<string, (...args: any[]) => any>
}
const dataNativeBuild = require('../../scripts/build-data-analysis-native-tools.cjs') as {
  _internals: Record<string, (...args: any[]) => any>
}
const nativeContract = require('../../scripts/native-component-contract.cjs') as {
  manifest: { components: Array<{ id: string; binaryName: string }> }
  binaryName: (component: { binaryName: string }, platform: string) => string
  targetContract: (platform: string, arch: string) => Record<string, any>
}
const cacheContract = require('../../scripts/lib/development-cache-environment.cjs') as {
  DEVELOPMENT_CACHE_APFS_VOLUME_TYPE: string
  DEVELOPMENT_CACHE_BACKING_MOUNT: string
  DEVELOPMENT_CACHE_BACKING_VOLUME_UUID: string
  DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE: string
  DEVELOPMENT_CACHE_ROOT: string
  DEVELOPMENT_CACHE_IMAGE: string
  DEVELOPMENT_CACHE_LEGACY_IMAGE: string
  DEVELOPMENT_CACHE_MOUNT: string
  DEVELOPMENT_NATIVE_COMPONENT_ROOT: string
  DEVELOPMENT_CACHE_VOLUME_UUID: string
  requireDevelopmentNativeComponentRoot: (env: NodeJS.ProcessEnv) => string
  validateDevelopmentCacheVolumeSnapshot: (
    snapshot: Record<string, any>
  ) => Record<string, any>
}

const roots: string[] = []
let producerFixtureSequence = 0

function taskRoot(label: string): string {
  const root = mkdtempSync(join(tmpdir(), `analytix-${label}-`))
  roots.push(root)
  return root
}

function sha256(value: Buffer | string): string {
  return createHash('sha256').update(value).digest('hex')
}

function machOFixture(marker: string): Buffer {
  const bytes = Buffer.alloc(0x220)
  bytes.writeUInt32LE(0xfeedfacf, 0)
  bytes.writeUInt32LE(0x0100000c, 4)
  bytes.writeUInt32LE(2, 12)
  bytes.writeUInt32LE(3, 16)
  bytes.writeUInt32LE(160, 20)
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
  const codeSignature = 176
  bytes.writeUInt32LE(0x1d, codeSignature)
  bytes.writeUInt32LE(16, codeSignature + 4)
  bytes.writeUInt32LE(0x200, codeSignature + 8)
  bytes.writeUInt32LE(0x20, codeSignature + 12)
  Buffer.from(marker, 'utf8').copy(bytes, 0x1c0, 0, 48)
  bytes.writeUInt32BE(0xfade0cc0, 0x200)
  bytes.writeUInt32BE(0x20, 0x204)
  bytes.writeUInt32BE(1, 0x208)
  bytes.writeUInt32BE(0, 0x20c)
  bytes.writeUInt32BE(0x14, 0x210)
  return bytes
}

function developmentFixture(root: string) {
  const developmentRoot = join(root, 'development-v3')
  const nativeComponentRoot = join(
    developmentRoot,
    'native-components-development'
  )
  const sourceDir = join(
    nativeComponentRoot,
    'darwin-arm64'
  )
  const appOutDir = join(root, 'package')
  const resources = join(
    appOutDir,
    'analytix.app',
    'Contents',
    'Resources'
  )
  const runtimeDir = join(resources, 'runtime')
  mkdirSync(sourceDir, { recursive: true, mode: 0o700 })
  chmodSync(developmentRoot, 0o700)
  chmodSync(nativeComponentRoot, 0o700)
  chmodSync(sourceDir, 0o700)
  mkdirSync(runtimeDir, { recursive: true, mode: 0o755 })
  const components = nativeContract.manifest.components.map((component) => {
    const binaryName = component.binaryName
    const path = join(sourceDir, binaryName)
    writeFileSync(path, machOFixture(component.id), { mode: 0o700 })
    const binding = afterPack._internals.signingInvariantNativeArtifactBinding(path, {
      format: 'mach-o',
      arch: 'arm64'
    })
    return {
      id: component.id,
      sourceDigest: '1'.repeat(64),
      cargoLockSha256: '2'.repeat(64),
      buildEnvironmentSha256: '3'.repeat(64),
      binaryName,
      binarySha256: binding.preSignSha256,
      binarySize: binding.preSignByteLength,
      payloadSha256: binding.payloadSha256,
      payloadSize: binding.payloadByteLength,
      format: 'mach-o',
      arch: 'arm64'
    }
  })
  const marker = {
    schemaVersion: 1,
    kind: 'analytix_native_development_build',
    classification: afterPack.NATIVE_DISPOSITION_DEVELOPMENT,
    publishable: false,
    releaseEligible: false,
    authorityUse: 'development_only',
    targetKey: 'darwin-arm64',
    targetTriple: 'aarch64-apple-darwin',
    platform: 'darwin',
    arch: 'arm64',
    sourceSetSha256: '4'.repeat(64),
    buildContextSha256: '5'.repeat(64),
    buildEnvironmentSha256: '6'.repeat(64),
    toolchain: {
      cargoExecutableSha256: '7'.repeat(64),
      cargoVersion: 'cargo fixture',
      rustcExecutableSha256: '8'.repeat(64),
      rustcVersion: 'rustc fixture'
    },
    components
  }
  writeFileSync(
    join(sourceDir, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER),
    JSON.stringify(marker),
    { mode: 0o600 }
  )
  return {
    cacheMountRoot: root,
    nativeComponentRoot,
    sourceDir,
    runtimeDir,
    context: {
      appOutDir,
      electronPlatformName: 'darwin',
      arch: 'arm64',
      packager: {
        appInfo: { productFilename: 'analytix' },
        config: { executableName: 'analytix' },
        platformSpecificBuildOptions: { executableName: 'analytix' }
      }
    }
  }
}

function producerGenerationInput(root: string, label: string) {
  producerFixtureSequence += 1
  const target = nativeContract.targetContract('darwin', 'arm64')
  const temporaryDir = join(
    root,
    `.darwin-arm64.build-${producerFixtureSequence}`
  )
  mkdirSync(temporaryDir, { mode: 0o700 })
  const sourceComponents = nativeContract.manifest.components.map((component) => ({
    id: component.id,
    source_root: `native/${component.id}`,
    source_digest: sha256(`source:${label}:${component.id}`),
    cargo_lock_sha256: sha256(`lock:${label}:${component.id}`)
  }))
  const builtComponents = nativeContract.manifest.components.map((component) => {
    const binaryName = nativeContract.binaryName(component, target.platform)
    const path = join(temporaryDir, binaryName)
    writeFileSync(path, machOFixture(`${label}:${component.id}`), { mode: 0o700 })
    const binding = afterPack._internals.signingInvariantNativeArtifactBinding(path, target)
    return {
      id: component.id,
      binaryName,
      buildEnvironmentSha256: sha256(`environment:${label}:${component.id}`),
      binarySha256: binding.preSignSha256,
      binarySize: binding.preSignByteLength,
      payloadSha256: binding.payloadSha256,
      payloadSize: binding.payloadByteLength,
      format: target.format,
      arch: target.arch
    }
  })
  return {
    temporaryDir,
    target,
    sourceSetSha256: sha256(`source-set:${label}`),
    buildContextSha256: sha256(`build-context:${label}`),
    buildEnvironmentSha256: sha256(`build-environment:${label}`),
    toolchain: {
      cargoExecutableSha256: sha256(`cargo:${label}`),
      cargoVersion: `cargo ${label}`,
      rustcExecutableSha256: sha256(`rustc:${label}`),
      rustcVersion: `rustc ${label}`
    },
    sourceComponents,
    builtComponents
  }
}

function publishProducerGeneration(
  outputDir: string,
  root: string,
  label: string,
  callbacks: Record<string, () => void> = {}
) {
  return dataNativeBuild._internals.publishDevelopmentBuild({
    outputDir,
    ...producerGenerationInput(root, label),
    ...callbacks
  })
}

function strictCacheOptions(fixture: ReturnType<typeof developmentFixture>) {
  let closed = false
  let verifyCount = 0
  return {
    repoRoot: dirname(dirname(fixture.nativeComponentRoot)),
    verifyCurrentSource: false,
    nativeComponentRoot: fixture.nativeComponentRoot,
    cacheMountRoot: fixture.cacheMountRoot,
    cacheVolume: {
      verify() {
        if (closed) throw new Error('test cache verifier is closed')
        verifyCount += 1
        return { verifyCount }
      },
      close() {
        closed = true
      }
    },
    verificationCount: () => verifyCount
  }
}

function authoritativeVolumeSnapshot(): Record<string, any> {
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  const directory = (path: string, dev: string) => ({
    path,
    realpath: path,
    dev,
    ino: '1',
    uid,
    gid: 20,
    mode: 0o700,
    directory: true,
    symlink: false
  })
  const device = 'disk99s1'
  return {
    backing: {
      MountPoint: cacheContract.DEVELOPMENT_CACHE_BACKING_MOUNT,
      VolumeUUID: cacheContract.DEVELOPMENT_CACHE_BACKING_VOLUME_UUID,
      Writable: true
    },
    cache: {
      DeviceIdentifier: device,
      MountPoint: cacheContract.DEVELOPMENT_CACHE_MOUNT,
      FilesystemType: 'apfs',
      Writable: true,
      GlobalPermissionsEnabled: true,
      VolumeUUID: cacheContract.DEVELOPMENT_CACHE_VOLUME_UUID
    },
    hdiutil: {
      images: [{
        'image-path': cacheContract.DEVELOPMENT_CACHE_IMAGE,
        'system-entities': [{
          'content-hint': cacheContract.DEVELOPMENT_CACHE_APFS_VOLUME_TYPE,
          'dev-entry': `/dev/${device}`,
          'mount-point': cacheContract.DEVELOPMENT_CACHE_MOUNT
        }]
      }]
    },
    mounts: `/dev/${device} on ${cacheContract.DEVELOPMENT_CACHE_MOUNT} (apfs, local, journaled)\n`,
    effectiveUid: uid,
    paths: {
      backing: directory(cacheContract.DEVELOPMENT_CACHE_BACKING_MOUNT, '11'),
      cacheMount: directory(cacheContract.DEVELOPMENT_CACHE_MOUNT, '22'),
      image: directory(cacheContract.DEVELOPMENT_CACHE_IMAGE, '11'),
      damagedV2Image: null,
      legacyImage: null
    }
  }
}

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

describe('cache-owned packaged native development input', () => {
  it('binds a writable ownership-enabled APFS mount to only the trusted v3 sparsebundle', () => {
    const valid = authoritativeVolumeSnapshot()
    expect(cacheContract.validateDevelopmentCacheVolumeSnapshot(valid)).toBe(valid)

    const readOnly = structuredClone(valid)
    readOnly.cache.Writable = false
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(readOnly))
      .toThrow(/Cache-volume identity/)

    const ownershipDisabled = structuredClone(valid)
    ownershipDisabled.cache.GlobalPermissionsEnabled = false
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(ownershipDisabled))
      .toThrow(/Cache-volume identity/)

    const noOwners = structuredClone(valid)
    noOwners.mounts = noOwners.mounts.replace(
      '(apfs, local, journaled)',
      '(apfs, local, journaled, noowners)'
    )
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(noOwners))
      .toThrow(/Cache mount flags/)

    const retiredPresent = structuredClone(valid)
    retiredPresent.paths.damagedV2Image = {
      ...retiredPresent.paths.image,
      path: cacheContract.DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE,
      realpath: cacheContract.DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE
    }
    retiredPresent.paths.legacyImage = {
      ...retiredPresent.paths.image,
      path: cacheContract.DEVELOPMENT_CACHE_LEGACY_IMAGE,
      realpath: cacheContract.DEVELOPMENT_CACHE_LEGACY_IMAGE
    }
    expect(cacheContract.validateDevelopmentCacheVolumeSnapshot(retiredPresent))
      .toBe(retiredPresent)

    const missingRetirementIdentity = structuredClone(valid)
    delete missingRetirementIdentity.paths.damagedV2Image
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(missingRetirementIdentity))
      .toThrow(/retired damaged v2 sparsebundle path identity/)

    const escapedRetiredImage = structuredClone(retiredPresent)
    escapedRetiredImage.paths.damagedV2Image.dev = '99'
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(escapedRetiredImage))
      .toThrow(/retired damaged v2 sparsebundle escaped/)

    const damagedV2Attached = structuredClone(valid)
    damagedV2Attached.hdiutil.images.push({
      'image-path': cacheContract.DEVELOPMENT_CACHE_DAMAGED_V2_IMAGE,
      'system-entities': []
    })
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(damagedV2Attached))
      .toThrow(/Damaged v2 cache image/)

    const legacyAttached = structuredClone(valid)
    legacyAttached.hdiutil.images.push({
      'image-path': cacheContract.DEVELOPMENT_CACHE_LEGACY_IMAGE,
      'system-entities': []
    })
    expect(() => cacheContract.validateDevelopmentCacheVolumeSnapshot(legacyAttached))
      .toThrow(/Legacy damaged cache image/)
  })

  it('revalidates the exact cache authority after volume verification and before receipt publication', () => {
    const source = readFileSync(
      new URL('../../scripts/verify-analytix-cache.sh', import.meta.url),
      'utf8'
    )
    const verifyIndex = source.indexOf(
      'if ! /usr/sbin/diskutil verifyVolume "$analytix_cache_verify_mount"'
    )
    const detachIndex = source.indexOf(
      '/usr/bin/hdiutil detach "$analytix_cache_verify_post_whole"',
      verifyIndex
    )
    const postAuthorityIndex = source.indexOf(
      "'post-verify cache authority helper rejected the host'",
      verifyIndex
    )
    const probeIndex = source.indexOf(
      '/usr/bin/mktemp -d "$TMPDIR/analytix-cache-verification.XXXXXXXX"'
    )
    const finalAuthorityIndex = source.indexOf(
      "'final cache authority helper rejected the host'",
      probeIndex
    )
    const receiptIndex = source.indexOf(
      '/usr/bin/mktemp "$analytix_cache_verify_evidence/cache-verification-v1.receipt.XXXXXXXX"'
    )

    expect(verifyIndex).toBeGreaterThan(-1)
    expect(detachIndex).toBeGreaterThan(verifyIndex)
    expect(postAuthorityIndex).toBeGreaterThan(detachIndex)
    expect(probeIndex).toBeGreaterThan(postAuthorityIndex)
    expect(finalAuthorityIndex).toBeGreaterThan(probeIndex)
    expect(receiptIndex).toBeGreaterThan(finalAuthorityIndex)
    expect(source).toContain(
      '/usr/sbin/lsof -t +f -- "$analytix_cache_verify_mount"'
    )
    expect(source).not.toContain(
      'hdiutil detach "$analytix_cache_verify_post_whole" -force'
    )
  })

  it('derives the only development native root from the verified cache marker', () => {
    expect(cacheContract.requireDevelopmentNativeComponentRoot({
      ANALYTIX_DEV_CACHE_ROOT: cacheContract.DEVELOPMENT_CACHE_ROOT
    })).toBe(cacheContract.DEVELOPMENT_NATIVE_COMPONENT_ROOT)
    expect(() => cacheContract.requireDevelopmentNativeComponentRoot({})).toThrow(
      /not authoritative/
    )
    expect(() => cacheContract.requireDevelopmentNativeComponentRoot({
      ANALYTIX_DEV_CACHE_ROOT: '/tmp/not-analytix-cache'
    })).toThrow(/not authoritative/)
    expect(() => dataNativeBuild._internals.parseBuildArgs([
      '--development',
      '--output-dir',
      '/tmp/native-output'
    ])).toThrow(/only the verified Analytix cache output/)

    const target = nativeContract.targetContract('darwin', 'arm64')
    const options = dataNativeBuild._internals.parseBuildArgs(['--development'])
    expect(dataNativeBuild._internals.targetOutputDirectory(
      options,
      target,
      { ANALYTIX_DEV_CACHE_ROOT: cacheContract.DEVELOPMENT_CACHE_ROOT }
    )).toBe(
      join(cacheContract.DEVELOPMENT_NATIVE_COMPONENT_ROOT, 'darwin-arm64')
    )
  })

  it('publishes and reconciles fixed producer generations without residue', () => {
    const root = taskRoot('native-producer-transaction')
    const output = join(root, 'darwin-arm64')
    let buildVerifications = 0
    let publicationVerifications = 0
    const callbacks = {
      verifyBuildEffect() {
        buildVerifications += 1
      },
      verifyPublicationEffect() {
        publicationVerifications += 1
      }
    }

    publishProducerGeneration(output, root, 'first', callbacks)
    const firstMarker = readFileSync(
      join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
    )
    publishProducerGeneration(output, root, 'second', callbacks)
    const secondMarker = readFileSync(
      join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
    )
    expect(secondMarker).not.toEqual(firstMarker)
    expect(existsSync(`${output}.previous`)).toBe(false)

    renameSync(output, `${output}.previous`)
    publishProducerGeneration(output, root, 'previous-only-recovery', callbacks)
    expect(existsSync(`${output}.previous`)).toBe(false)

    const alternate = join(root, 'alternate')
    publishProducerGeneration(alternate, root, 'alternate', callbacks)
    renameSync(alternate, `${output}.previous`)
    publishProducerGeneration(output, root, 'current-and-previous', callbacks)
    expect(existsSync(`${output}.previous`)).toBe(false)
    expect(buildVerifications).toBeGreaterThan(8)
    expect(publicationVerifications).toBeGreaterThan(16)
  })

  it('never overwrites a collided leaf in a fresh producer generation', () => {
    const root = taskRoot('native-producer-leaf-collision')
    const source = join(root, 'source')
    const destination = join(root, 'destination')
    const sentinel = Buffer.from('unknown-collided-generation-leaf')
    writeFileSync(source, 'new-native-component', { mode: 0o700 })
    writeFileSync(destination, sentinel, { mode: 0o700 })
    expect(() => dataNativeBuild._internals.copyBuiltComponentExclusive(
      source,
      destination
    )).toThrow()
    expect(readFileSync(destination)).toEqual(sentinel)
  })

  it('recovers only a captured producer generation after transient verifier failures', () => {
    const root = taskRoot('native-producer-recovery')
    const output = join(root, 'darwin-arm64')
    publishProducerGeneration(output, root, 'baseline')
    const baselineMarker = readFileSync(
      join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
    )

    let retirementFailureInjected = false
    expect(() => publishProducerGeneration(output, root, 'retirement-failure', {
      verifyPublicationEffect() {
        if (!retirementFailureInjected &&
          existsSync(`${output}.previous`) && !existsSync(output)) {
          retirementFailureInjected = true
          throw new Error('injected retirement verifier failure')
        }
      }
    })).toThrow(/injected retirement verifier failure/)
    expect(retirementFailureInjected).toBe(true)
    expect(readFileSync(
      join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
    )).toEqual(baselineMarker)
    expect(existsSync(`${output}.previous`)).toBe(false)

    const stagedFailure = producerGenerationInput(root, 'staged-failure')
    let stagedFailureInjected = false
    expect(() => dataNativeBuild._internals.publishDevelopmentBuild({
      outputDir: output,
      ...stagedFailure,
      verifyPublicationEffect() {
        if (!stagedFailureInjected &&
          existsSync(output) &&
          existsSync(`${output}.previous`) &&
          !existsSync(stagedFailure.temporaryDir)) {
          stagedFailureInjected = true
          throw new Error('injected staged verifier failure')
        }
      }
    })).toThrow(/injected staged verifier failure/)
    expect(stagedFailureInjected).toBe(true)
    expect(existsSync(output)).toBe(true)
    expect(existsSync(`${output}.previous`)).toBe(true)

    publishProducerGeneration(output, root, 'post-failure-reconciliation')
    expect(existsSync(`${output}.previous`)).toBe(false)
  })

  it('rejects invalid or occupied producer generations without deleting them', () => {
    const brokenOutputRoot = taskRoot('native-producer-broken-output')
    const brokenOutput = join(brokenOutputRoot, 'darwin-arm64')
    symlinkSync(join(brokenOutputRoot, 'missing-output'), brokenOutput)
    expect(() => publishProducerGeneration(
      brokenOutput,
      brokenOutputRoot,
      'broken-output'
    )).toThrow(/generation directory is unsafe/)
    expect(lstatSync(brokenOutput).isSymbolicLink()).toBe(true)

    const brokenPreviousRoot = taskRoot('native-producer-broken-previous')
    const brokenPreviousOutput = join(brokenPreviousRoot, 'darwin-arm64')
    const brokenPrevious = `${brokenPreviousOutput}.previous`
    symlinkSync(join(brokenPreviousRoot, 'missing-previous'), brokenPrevious)
    expect(() => publishProducerGeneration(
      brokenPreviousOutput,
      brokenPreviousRoot,
      'broken-previous'
    )).toThrow(/generation directory is unsafe/)
    expect(lstatSync(brokenPrevious).isSymbolicLink()).toBe(true)

    const mutationCases = [
      {
        label: 'extra-entry',
        mutate(output: string, root: string) {
          writeFileSync(join(output, 'unexpected'), 'preserve-me', { mode: 0o700 })
          return join(output, 'unexpected')
        },
        expected: /generation inventory is invalid/
      },
      {
        label: 'wrong-marker-mode',
        mutate(output: string) {
          const marker = join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
          chmodSync(marker, 0o644)
          return marker
        },
        expected: /generation marker is unsafe/
      },
      {
        label: 'hard-linked-marker',
        mutate(output: string, root: string) {
          const marker = join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
          linkSync(marker, join(root, 'outside-marker-link'))
          return marker
        },
        expected: /generation marker is unsafe/
      },
      {
        label: 'changed-binary',
        mutate(output: string) {
          const binary = join(output, nativeContract.manifest.components[0].binaryName)
          const bytes = readFileSync(binary)
          bytes[0x1c0] ^= 0xff
          writeFileSync(binary, bytes, { mode: 0o700 })
          return binary
        },
        expected: /generation component is invalid/
      },
      {
        label: 'oversized-marker',
        mutate(output: string) {
          const marker = join(output, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER)
          writeFileSync(marker, Buffer.alloc(256 * 1024 + 1, 0x20), { mode: 0o600 })
          return marker
        },
        expected: /generation marker is unsafe/
      }
    ]
    for (const mutation of mutationCases) {
      const root = taskRoot(`native-producer-${mutation.label}`)
      const output = join(root, 'darwin-arm64')
      publishProducerGeneration(output, root, `${mutation.label}-baseline`)
      const preserved = mutation.mutate(output, root)
      expect(() => publishProducerGeneration(
        output,
        root,
        `${mutation.label}-replacement`
      )).toThrow(mutation.expected)
      expect(existsSync(preserved)).toBe(true)
    }
  })

  it('requires a canonical owner-private source chain and rejects a symlinked cache root', () => {
    const root = taskRoot('native-private-chain')
    const developmentRoot = join(root, 'development-v3')
    const cacheRoot = join(developmentRoot, 'native-components-development')
    const sourceRoot = join(cacheRoot, 'darwin-arm64')
    mkdirSync(sourceRoot, { recursive: true, mode: 0o700 })
    chmodSync(developmentRoot, 0o700)
    chmodSync(cacheRoot, 0o700)
    chmodSync(sourceRoot, 0o700)
    const sourceStats = afterPack._internals.assertPrivateNativeCacheSource(
      sourceRoot,
      cacheRoot,
      { mountRoot: root, expectedCacheRoot: cacheRoot }
    )
    expect(sourceStats.isDirectory()).toBe(true)

    const outside = join(root, 'outside')
    mkdirSync(outside, { mode: 0o700 })
    rmSync(cacheRoot, { recursive: true })
    symlinkSync(outside, cacheRoot)
    expect(() => afterPack._internals.assertPrivateNativeCacheSource(
      join(cacheRoot, 'darwin-arm64'),
      cacheRoot,
      { mountRoot: root, expectedCacheRoot: cacheRoot }
    )).toThrow(/canonical directory/)
  })

  it('publishes without overwriting an occupied destination', () => {
    const root = taskRoot('native-no-overwrite')
    const fixture = developmentFixture(root)
    const occupied = join(fixture.runtimeDir, 'analytix-data-engine')
    const sentinel = Buffer.from('preexisting-native-destination')
    writeFileSync(occupied, sentinel, { mode: 0o755 })

    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      fixture.context,
      strictCacheOptions(fixture)
    )).toThrow(/destination is already occupied/)
    expect(readFileSync(occupied)).toEqual(sentinel)
    expect(existsSync(join(
      fixture.runtimeDir,
      afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER
    ))).toBe(false)
  })

  it('rejects hard-linked or incorrectly permissioned cache source leaves', () => {
    const hardLinkRoot = taskRoot('native-hardlink-source')
    const hardLinkFixture = developmentFixture(hardLinkRoot)
    linkSync(
      join(hardLinkFixture.sourceDir, 'analytix-data-engine'),
      join(hardLinkRoot, 'outside-hardlink')
    )
    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      hardLinkFixture.context,
      strictCacheOptions(hardLinkFixture)
    )).toThrow(/invalid generation file entry|source file is unsafe/)

    const modeRoot = taskRoot('native-wrong-mode')
    const modeFixture = developmentFixture(modeRoot)
    chmodSync(
      join(modeFixture.sourceDir, afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER),
      0o644
    )
    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      modeFixture.context,
      strictCacheOptions(modeFixture)
    )).toThrow(/source file is unsafe/)
  })

  it('never removes a replacement raced into an atomic publication path', () => {
    const root = taskRoot('native-publish-swap')
    const fixture = developmentFixture(root)
    const sentinel = Buffer.from('unknown-raced-replacement')
    let swappedPath = ''
    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      fixture.context,
      {
        ...strictCacheOptions(fixture),
        linkSync(source: string, destination: string) {
          linkSync(source, destination)
          unlinkSync(destination)
          writeFileSync(destination, sentinel, { mode: 0o755 })
          swappedPath = destination
        }
      }
    )).toThrow(/publication identity changed/)
    expect(swappedPath).not.toBe('')
    expect(readFileSync(swappedPath)).toEqual(sentinel)
    expect(existsSync(join(
      fixture.runtimeDir,
      afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER
    ))).toBe(false)
  })

  it('leaves an after-link failure inside the invalid package instead of deleting by pathname', () => {
    const root = taskRoot('native-after-link-failure')
    const fixture = developmentFixture(root)
    let linkedPath = ''
    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      fixture.context,
      {
        ...strictCacheOptions(fixture),
        linkSync(source: string, destination: string) {
          linkSync(source, destination)
          linkedPath = destination
          throw new Error('injected after-link failure')
        }
      }
    )).toThrow(/injected after-link failure/)
    expect(linkedPath).not.toBe('')
    expect(existsSync(linkedPath)).toBe(true)
    expect(existsSync(join(
      fixture.runtimeDir,
      afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER
    ))).toBe(false)
  })

  it('fails closed and leaves no output when a source changes around the copy', () => {
    const root = taskRoot('native-source-swap')
    const fixture = developmentFixture(root)
    let changed = false
    expect(() => afterPack._internals.materializePackagedDevelopmentNativeComponents(
      fixture.context,
      {
        ...strictCacheOptions(fixture),
        copyFileSync(source: string, destination: string, mode: number) {
          copyFileSync(source, destination, mode)
          if (!changed && source.endsWith('analytix-import-accelerator')) {
            changed = true
            writeFileSync(source, machOFixture('changed-source'), { mode: 0o700 })
          }
        }
      }
    )).toThrow(/does not match its marker|changed around atomic copy/)
    expect(changed).toBe(true)
    for (const component of nativeContract.manifest.components) {
      expect(existsSync(join(fixture.runtimeDir, component.binaryName))).toBe(false)
    }
    expect(existsSync(join(
      fixture.runtimeDir,
      afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER
    ))).toBe(false)
  })

  it('keeps a successful local package explicitly non-publishable', () => {
    const root = taskRoot('native-success')
    const fixture = developmentFixture(root)
    const cacheOptions = strictCacheOptions(fixture)
    const disposition = afterPack._internals.materializePackagedDevelopmentNativeComponents(
      fixture.context,
      cacheOptions
    )
    expect(disposition).toEqual(expect.objectContaining({
      kind: 'development_non_publishable',
      targetKey: 'darwin-arm64'
    }))
    expect(sha256(readFileSync(join(
      fixture.runtimeDir,
      afterPack.NATIVE_DEVELOPMENT_BUILD_MARKER
    )))).toBe(disposition.marker.sha256)
    for (const component of nativeContract.manifest.components) {
      expect(existsSync(join(fixture.runtimeDir, component.binaryName))).toBe(true)
    }
    expect(existsSync(join(
      fixture.runtimeDir,
      'analytix-native-components-receipt.json'
    ))).toBe(false)
    expect(cacheOptions.verificationCount()).toBeGreaterThan(8)
  })
})
