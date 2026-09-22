import { chmodSync, cpSync, existsSync, linkSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { hostname, tmpdir } from 'node:os'
import { delimiter, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { runInNewContext, runInThisContext } from 'node:vm'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { inspectNativePayload } from './data-analysis/native-payload-identity'
import { verifyRuntimeNativeComponent } from './data-analysis/native-runtime-integrity'
import {
  pythonRuntimeAuthority,
  pythonRuntimeIntegrityInternals
} from './data-analysis/python-runtime-integrity'

const require = createRequire(import.meta.url)
// Configuration-shape tests describe staged resources, not whatever happens
// to be cached on the test machine. Never load an Owner release.local.env.
function builderConfigurationFixture(resourcesPresent = true) {
  const repoRoot = resolve(__dirname, '../..')
  const present = new Set(resourcesPresent ? [
    join(repoRoot, 'managed-chrome'),
    join(repoRoot, 'build/windows-backend-runtime/.python-runtime'),
    join(repoRoot, 'build/windows-backend-runtime/python-site-packages'),
    join(repoRoot, 'build/private-pwsh/pwsh/pwsh.exe')
  ] : [])
  const environment = { env: {} }
  const load = (name: string, base?: unknown) => {
    const filename = join(repoRoot, name)
    const localRequire = createRequire(filename)
    const module = { exports: {} as any }
    runInNewContext(readFileSync(filename, 'utf8'), {
      module, __dirname: repoRoot, process: environment, URL,
      require: (id: string) => {
        if (id === 'node:fs') return {
          existsSync: (path: string) => present.has(resolve(path)),
          readFileSync: () => { throw new Error('configuration fixture attempted private environment access') }
        }
        if (id === './electron-builder.config.cjs') return base
        return localRequire(id)
      }
    }, { filename })
    return module.exports
  }
  const base = load('electron-builder.config.cjs')
  return { base, windows: load('electron-builder.standard-win.cjs', base) }
}

const { base: builderConfig, windows: standardWinConfig } = builderConfigurationFixture()

// Load the unchanged CommonJS implementation with file-local dependencies.
// No global require cache or production environment switch is modified.
function fixtureCommonJS(relativePath: string, dependencies: Record<string, unknown>) {
  const filename = resolve(__dirname, '../..', relativePath)
  const localRequire = createRequire(filename)
  const module = { exports: {} as any }
  const execute = runInThisContext(`(function(exports, require, module, __filename, __dirname) {\n${readFileSync(filename, 'utf8')}\n})`, { filename })
  execute(module.exports, (id: string) => Object.hasOwn(dependencies, id) ? dependencies[id] : localRequire(id),
    module, filename, dirname(filename))
  return module.exports
}

// These are receipt/parser/publication-race tests using synthetic binaries.
// Real toolchain admission remains in rust-native-toolchain-contract.test.ts
// and the candidate build; a version fixture never executes a compiler.
const fixtureToolchain = {
  cargo: resolve('synthetic-toolchain/cargo'),
  rustc: resolve('synthetic-toolchain/rustc'),
  environment: {},
  lock: {
    cargoVersion: 'cargo 1.94.1 (synthetic-parser-fixture)',
    rustcVersion: 'rustc 1.94.1 (synthetic-parser-fixture)',
    cargoExecutableSha256: createHash('sha256').update('synthetic cargo identity').digest('hex'),
    rustcExecutableSha256: createHash('sha256').update('synthetic rustc identity').digest('hex')
  }
}
const nativeComponentContract = fixtureCommonJS('scripts/native-component-contract.cjs', {
  './rust-native-toolchain-contract.cjs': { resolvePinnedRustToolchain: () => fixtureToolchain },
  'node:child_process': {
    spawnSync: (command: string, args: string[]) => {
      if (args.length !== 1 || args[0] !== '--version') throw new Error('unexpected execution in toolchain identity fixture')
      const version = command === fixtureToolchain.cargo ? fixtureToolchain.lock.cargoVersion
        : command === fixtureToolchain.rustc ? fixtureToolchain.lock.rustcVersion : ''
      if (!version) throw new Error('unregistered toolchain identity fixture')
      return { status: 0, signal: null, stdout: `${version}\n`, stderr: '' }
    }
  }
})
const afterPack = fixtureCommonJS('scripts/after-pack.cjs', {
  './native-component-contract.cjs': nativeComponentContract
})
const electronFusePolicy = require('../../scripts/electron-fuse-policy.cjs')
const packagedLifecycleGuard = require('../../scripts/packaged-lifecycle-guard.cjs')
const dataNativeBuild = require('../../scripts/build-data-analysis-native-tools.cjs')
const developmentCacheEnvironment = require('../../scripts/lib/development-cache-environment.cjs')
const nativeBuildProbeAuthority = require('../../scripts/native-build-probe-authority.cjs')
const pythonRuntimeBuild = require('../../scripts/build-windows-backend-runtime-assets.cjs')
const pythonRuntimeContract = require('../../scripts/windows-python-runtime-contract.cjs')
const officialWinCache = require('../../scripts/build-windows-release-cache.cjs')
const officialWinHost = require('../../scripts/require-official-windows-host.cjs')
const macNotarize = require('../../scripts/mac-notarize.cjs')
const rootPackage = require('../../package.json')
const rootPackageLock = require('../../package-lock.json')
const runtimePackage = require('../../packages/runtime/package.json')
const openClawShimPackage = require('../../vendor/openclaw-shim/package.json')
const runtimeBuildConfig = require('../../packages/runtime/tsconfig.build.json')
const productionFundsMcpEntryClosure = require('../../plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure.json')
const devTestAccountLauncherSource = readFileSync(
  resolve(process.cwd(), 'scripts/dev-test-account-launcher.mjs'),
  'utf8'
)
const openClawShimLicense = readFileSync(
  resolve(process.cwd(), 'vendor/openclaw-shim/LICENSE'),
  'utf8'
)
const thirdPartyNotices = readFileSync(
  resolve(process.cwd(), 'THIRD_PARTY_NOTICES.md'),
  'utf8'
)

const zipMacAppScript = resolve(process.cwd(), 'scripts/zip-mac-app.cjs')
const prepareHubPackageScript = resolve(
  process.cwd(),
  'plugins/analytix-fund-analysis/scripts/prepare-hub-package.mjs'
)

const tempRoots: string[] = []

function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-packaging-'))
  tempRoots.push(root)
  return root
}

function touch(path: string): void {
  mkdirSync(join(path, '..'), { recursive: true })
  writeFileSync(path, '{}\n', 'utf8')
}

it('fails closed before a standalone mac zip can reuse an existing app bundle', () => {
  const distDir = tempRoot()
  const appPath = join(distDir, 'mac-arm64', 'analytix.app')
  mkdirSync(appPath, { recursive: true })
  const zipPath = join(distDir, `analytix-${rootPackage.version}-mac-arm64.zip`)
  writeFileSync(zipPath, 'prior-artifact-must-remain-untouched', 'utf8')

  const result = spawnSync(process.execPath, [zipMacAppScript, 'arm64'], {
    cwd: process.cwd(),
    env: { ...process.env, ANALYTIX_DIST_DIR: distDir },
    encoding: 'utf8'
  })

  expect(result.status).toBe(1)
  expect(result.stderr).toContain('package_release_authority_unavailable')
  expect(readFileSync(zipPath, 'utf8')).toBe('prior-artifact-must-remain-untouched')
})

function nativeFixtureBytes(format: string, arch: string): Buffer {
  if (format === 'mach-o') {
    const bytes = Buffer.alloc(0x220)
    bytes.writeUInt32LE(0xfeedfacf, 0)
    bytes.writeUInt32LE(arch === 'arm64' ? 0x0100000c : 0x01000007, 4)
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
    bytes.writeUInt32BE(0xfade0cc0, 0x200)
    bytes.writeUInt32BE(0x20, 0x204)
    bytes.writeUInt32BE(1, 0x208)
    bytes.writeUInt32BE(0, 0x20c)
    bytes.writeUInt32BE(0x14, 0x210)
    return bytes
  }
  if (format === 'pe') {
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
    const section = optionalHeader + 0xf0
    bytes.write('.text', section, 'ascii')
    bytes.writeUInt32LE(0x100, section + 8)
    bytes.writeUInt32LE(0x1000, section + 12)
    bytes.writeUInt32LE(0x200, section + 16)
    bytes.writeUInt32LE(0x200, section + 20)
    bytes.writeUInt32LE(0x60000020, section + 36)
    return bytes
  }
  const bytes = Buffer.alloc(0x200)
  bytes[0] = 0x7f
  bytes.write('ELF', 1, 'ascii')
  bytes[4] = 2
  bytes[5] = 1
  bytes[6] = 1
  bytes.writeUInt16LE(3, 16)
  bytes.writeUInt16LE(arch === 'arm64' ? 183 : 62, 18)
  bytes.writeUInt32LE(1, 20)
  bytes.writeBigUInt64LE(0x400000n, 24)
  bytes.writeBigUInt64LE(64n, 32)
  bytes.writeUInt16LE(64, 52)
  bytes.writeUInt16LE(56, 54)
  bytes.writeUInt16LE(1, 56)
  bytes.writeUInt32LE(1, 64)
  bytes.writeUInt32LE(5, 68)
  bytes.writeBigUInt64LE(0n, 72)
  bytes.writeBigUInt64LE(0x400000n, 80)
  bytes.writeBigUInt64LE(0x400000n, 88)
  bytes.writeBigUInt64LE(BigInt(bytes.length), 96)
  bytes.writeBigUInt64LE(BigInt(bytes.length), 104)
  bytes.writeBigUInt64LE(0x1000n, 112)
  return bytes
}

function resizedMachOFixture(arch: string, signatureBytes: number): Buffer {
  const original = nativeFixtureBytes('mach-o', arch)
  const bytes = Buffer.alloc(0x200 + Math.max(signatureBytes, 0))
  original.copy(bytes, 0, 0, 0x200)
  bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), 104 + 32)
  bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), 104 + 48)
  if (signatureBytes === 0) {
    bytes.writeUInt32LE(2, 16)
    bytes.writeUInt32LE(144, 20)
    bytes.fill(0, 176, 192)
    return bytes
  }
  bytes.writeUInt32LE(signatureBytes, 176 + 12)
  bytes.writeUInt32BE(0xfade0cc0, 0x200)
  bytes.writeUInt32BE(signatureBytes, 0x204)
  bytes.writeUInt32BE(1, 0x208)
  bytes.writeUInt32BE(0, 0x20c)
  bytes.writeUInt32BE(0x14, 0x210)
  return bytes
}

function universalMachOFixture(signatureBytes: number, alignmentPower: number): Buffer {
  const slices = [
    { cpuType: 0x01000007, body: resizedMachOFixture('x64', signatureBytes) },
    {
      cpuType: 0x0100000c,
      body: resizedMachOFixture('arm64', signatureBytes === 0 ? 0 : signatureBytes + 16)
    }
  ]
  const alignment = 2 ** alignmentPower
  let offset = alignment
  const entries = slices.map((slice) => {
    const entry = { ...slice, offset }
    offset = Math.ceil((offset + slice.body.length) / alignment) * alignment
    return entry
  })
  const bytes = Buffer.alloc(entries.at(-1)!.offset + entries.at(-1)!.body.length)
  bytes.writeUInt32BE(0xcafebabe, 0)
  bytes.writeUInt32BE(entries.length, 4)
  for (const [index, entry] of entries.entries()) {
    const entryOffset = 8 + index * 20
    bytes.writeUInt32BE(entry.cpuType, entryOffset)
    bytes.writeUInt32BE(0, entryOffset + 4)
    bytes.writeUInt32BE(entry.offset, entryOffset + 8)
    bytes.writeUInt32BE(entry.body.length, entryOffset + 12)
    bytes.writeUInt32BE(alignmentPower, entryOffset + 16)
    entry.body.copy(bytes, entry.offset)
  }
  return bytes
}

function digest(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function writeNativeComponentBundle(
  context: ReturnType<typeof createMacPackContext> | ReturnType<typeof createWindowsPackContext>,
  binaryTargetOverride?: { format: string; arch: string }
): void {
  const target = nativeComponentContract.targetContract(context.electronPlatformName, context.arch)
  const runtimeDir = join(afterPack._internals.packedResourcesDir(context), 'runtime')
  mkdirSync(runtimeDir, { recursive: true })
  const executionAuthority = {
    schemaVersion: 1,
    trustClass: 'controlled_release',
    protocol: 'analytix-native-build-probe-v1',
    targetKey: target.key,
    binarySha256: digest('native-build-probe-authority'),
    binarySize: 4096,
    sourceSetSha256: digest('native-build-probe-source'),
    buildEnvironmentSha256: digest('native-build-probe-environment'),
    goToolchainKey: target.key,
    goExecutableSha256: digest('native-build-probe-go')
  }
  const publicationAuthority = {
    schemaVersion: 1,
    trustClass: 'controlled_release',
    protocol: 'analytix-cargo-execution-authority-v1',
    targetKey: target.key,
    cargoExecutionId: digest('cargo-execution'),
    cargoExecutionReceiptSha256: digest('cargo-execution-receipt'),
    publicationBindingSha256: digest('publication-binding')
  }
  const components = nativeComponentContract.manifest.components.map((component: {
    id: string
    binaryName: string
    packagePath: string
    sourceRoot: string
    executionProbe: { policySha256: string }
  }) => {
    const binary = nativeComponentContract.binaryName(component, target.platform)
    const bytes = nativeFixtureBytes(
      binaryTargetOverride?.format ?? target.format,
      binaryTargetOverride?.arch ?? target.arch
    )
    const payloadIdentity = nativeComponentContract.nativePayloadIdentity(
      bytes,
      binaryTargetOverride?.format ?? target.format
    )
    writeFileSync(join(runtimeDir, binary), bytes)
    return {
      id: component.id,
      binaryName: binary,
      packagePath: target.platform === 'win32' ? `${component.packagePath}.exe` : component.packagePath,
      sourceDigest: nativeComponentContract.sourceTreeDigest(join(process.cwd(), component.sourceRoot), process.cwd()),
      cargoLockSha256: nativeComponentContract.sha256File(join(process.cwd(), component.sourceRoot, 'Cargo.lock')),
      buildEnvironmentSha256: '3'.repeat(64),
      rawBuildSha256: digest(bytes),
      rawBuildSize: bytes.length,
      stagedImageSha256: digest(bytes),
      stagedImageSize: bytes.length,
      payloadSha256: payloadIdentity.sha256,
      payloadSize: payloadIdentity.size,
      format: binaryTargetOverride?.format ?? target.format,
      arch: binaryTargetOverride?.arch ?? target.arch,
      executionProbe: {
        kind: 'analytix_native_build_probe_receipt',
        schema_version: 1,
        status: 'passed',
        component_id: component.id,
        request_nonce: digest(`nonce:${component.id}`),
        executable_sha256: digest(bytes),
        executable_size: bytes.length,
        manifest_sha256: nativeComponentContract.manifestDigest,
        policy_sha256: component.executionProbe.policySha256,
        authority_sha256: executionAuthority.binarySha256,
        host_platform: 'darwin',
        host_arch: target.arch,
        loaded_image_bound: true,
        working_directory_bound: true,
        guardian_authenticated: true,
        process_tree_empty: true
      }
    }
  })
  writeFileSync(
    join(runtimeDir, nativeComponentContract.RECEIPT_FILE_NAME),
    `${JSON.stringify({
      schemaVersion: nativeComponentContract.RECEIPT_SCHEMA_VERSION,
      manifestSha256: nativeComponentContract.manifestDigest,
      sourceSetSha256: nativeComponentContract.sourceSetDigest(process.cwd()),
      buildEnvironmentSha256: nativeComponentContract.environmentDigest({
        ...nativeComponentContract.nativeBuildChildEnvironment(process.env),
        RUSTC: nativeComponentContract.resolveNativeToolchainExecutable(
          'rustc',
          nativeComponentContract.nativeBuildChildEnvironment(process.env)
        )
      }),
      toolchain: nativeComponentContract.nativeToolchainIdentity({ env: process.env }),
      targetKey: target.key,
      targetTriple: target.triple,
      platform: target.platform,
      arch: target.arch,
      executionAuthority,
      publicationAuthority,
      components
    }, null, 2)}\n`,
    'utf8'
  )
}

function setNestedReceiptValue(value: unknown, path: string[], replacement: unknown): void {
  if (path.length === 0) throw new Error('receipt mutation path is empty')
  let current = value
  for (const segment of path.slice(0, -1)) {
    if (!current || typeof current !== 'object') {
      throw new Error(`receipt mutation path is invalid: ${path.join('.')}`)
    }
    current = (current as Record<string, unknown>)[segment]
  }
  if (!current || typeof current !== 'object') {
    throw new Error(`receipt mutation target is invalid: ${path.join('.')}`)
  }
  const mutationTarget = current as Record<string, unknown>
  mutationTarget[path[path.length - 1]!] = replacement
}

function loadBuilderConfigWithEnv(env: Record<string, string | undefined>): typeof builderConfig {
  const configPath = require.resolve('../../electron-builder.config.cjs')
  const previous = new Map<string, string | undefined>()
  for (const [key, value] of Object.entries(env)) {
    previous.set(key, process.env[key])
    if (value === undefined) {
      delete process.env[key]
    } else {
      process.env[key] = value
    }
  }

  delete require.cache[configPath]
  try {
    return require(configPath)
  } finally {
    delete require.cache[configPath]
    for (const [key, value] of previous) {
      if (value === undefined) {
        delete process.env[key]
      } else {
        process.env[key] = value
      }
    }
    require(configPath)
  }
}

function withProcessEnv<T>(env: Record<string, string | undefined>, action: () => T): T {
  const previous = new Map<string, string | undefined>()
  for (const [key, value] of Object.entries(env)) {
    previous.set(key, process.env[key])
    if (value === undefined) delete process.env[key]
    else process.env[key] = value
  }
  try {
    return action()
  } finally {
    for (const [key, value] of previous) {
      if (value === undefined) delete process.env[key]
      else process.env[key] = value
    }
  }
}

function createMacPackContext(root: string): {
  appOutDir: string
  electronPlatformName: string
  packager: { appInfo: { productFilename: string } }
  arch: string
} {
  return {
    appOutDir: join(root, 'mac-arm64'),
    electronPlatformName: 'darwin',
    arch: 'arm64',
    packager: {
      appInfo: {
        productFilename: 'Analytix'
      }
    }
  }
}

function createWindowsPackContext(root: string): {
  appOutDir: string
  electronPlatformName: string
  packager: { appInfo: { productFilename: string } }
  arch: string
} {
  return {
    appOutDir: join(root, 'win-unpacked'),
    electronPlatformName: 'win32',
    arch: 'x64',
    packager: {
      appInfo: {
        productFilename: 'analytix'
      }
    }
  }
}

function writeBundledFundsPlugin(
  context: ReturnType<typeof createMacPackContext> | ReturnType<typeof createWindowsPackContext>,
  sourceRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
): string {
  const destinationRoot = join(
    afterPack._internals.packedResourcesDir(context),
    'plugins',
    'analytix-fund-analysis'
  )
  mkdirSync(join(destinationRoot, '.analytix-plugin'), { recursive: true })
  mkdirSync(join(destinationRoot, '.codex-plugin'), { recursive: true })
  mkdirSync(join(destinationRoot, 'agents'), { recursive: true })
  mkdirSync(join(destinationRoot, 'mcp'), { recursive: true })
  cpSync(
    join(sourceRoot, '.analytix-plugin', 'package.json'),
    join(destinationRoot, '.analytix-plugin', 'package.json')
  )
  cpSync(
    join(sourceRoot, '.codex-plugin', 'plugin.json'),
    join(destinationRoot, '.codex-plugin', 'plugin.json')
  )
  cpSync(join(sourceRoot, '.mcp.json'), join(destinationRoot, '.mcp.json'))
  cpSync(join(sourceRoot, 'agents', 'openai.yaml'), join(destinationRoot, 'agents', 'openai.yaml'))
  for (const relativePath of afterPack.ANALYTIX_FUNDS_PRODUCTION_MCP_FILES) {
    cpSync(join(sourceRoot, relativePath), join(destinationRoot, relativePath))
  }
  return destinationRoot
}

function prepareAuthorityPublicationFixture(): {
  root: string
  context: ReturnType<typeof createMacPackContext>
  authorityPath: string
  runtimeDir: string
  nativeTrust: unknown
  options: Record<string, unknown>
} {
  const root = tempRoot()
  const context = createMacPackContext(root)
  writeBundledFundsPlugin(context)
  const fundsPluginAdmission = afterPack._internals.validateBundledFundsPlugin(context)
  writeNativeComponentBundle(context)
  const nativeTrust = afterPack._internals.verifyPackagedNativeBeforeRuntimeBuild(context, {
    verifyNativeExecution: false
  })
  for (const [path, bytes] of [
    [afterPack._internals.packagedExecutablePath(context), nativeFixtureBytes('mach-o', 'arm64')],
    [afterPack._internals.packagedAppAsarPath(context), Buffer.from('app-asar\n')],
    [afterPack._internals.bundledGoRuntimeServerPath(context), nativeFixtureBytes('mach-o', 'arm64')]
  ] as const) {
    mkdirSync(join(path, '..'), { recursive: true })
    writeFileSync(path, bytes, { mode: 0o755 })
  }
  const worktreeSnapshot = afterPack._internals.collectPackagedWorktreeSnapshotV1(process.cwd())
  const buildContext = afterPack._internals.collectEffectiveBuilderContextV1({
    electronPlatformName: 'darwin',
    arch: 'arm64',
    targets: [],
    packager: {
      config: {
        appId: 'com.analytix.desktop',
        productName: 'Analytix',
        directories: { output: 'dist' }
      }
    }
  })
  const runtimeDir = join(afterPack._internals.packedResourcesDir(context), 'runtime')
  return {
    root,
    context,
    runtimeDir,
    authorityPath: join(runtimeDir, afterPack.PACKAGED_BUILD_AUTHORITY_FILE),
    nativeTrust,
    options: {
      repoRoot: process.cwd(),
      env: {},
      worktreeSnapshot,
      buildContext,
      fundsPluginAdmission,
      stagedPayload: afterPack._internals.collectStagedPayloadClosureV1(context)
    }
  }
}

function authorityFileIdentity(path: string): { dev: number; ino: number; mode: number; size: number } {
  const stat = lstatSync(path)
  return { dev: stat.dev, ino: stat.ino, mode: stat.mode, size: stat.size }
}

function expectNoAuthorityTemporary(runtimeDir: string): void {
  const prefix = `.${afterPack.PACKAGED_BUILD_AUTHORITY_FILE}.`
  expect(readdirSync(runtimeDir).filter((name) => name.startsWith(prefix))).toEqual([])
}

function expectNoCompletedAfterPackCredential(root: string): void {
  expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
    file: join(root, 'dist', 'Analytix.dmg'),
    arch: 3,
    targetPresentableName: 'DMG'
  })).toThrow(/lacks one exact completed afterPack lifecycle/)
}

function touchWindowsBackendRuntime(context: ReturnType<typeof createWindowsPackContext>): void {
  touch(afterPack._internals.bundledWindowsBackendPythonExecutablePath(context))
  const sitePackages = afterPack._internals.bundledWindowsBackendSitePackagesDir(context)
  touch(join(sitePackages, 'analytix-site-packages-manifest.json'))
  touch(join(sitePackages, 'fastapi/__init__.py'))
  touch(join(sitePackages, 'uvicorn/__init__.py'))
  touch(join(sitePackages, 'pandas/__init__.py'))
  touch(join(sitePackages, 'duckdb/__init__.py'))
  touch(join(afterPack._internals.packedResourcesDir(context), 'backend/app/main.py'))
}

function touchManagedChrome(context: ReturnType<typeof createWindowsPackContext>): void {
  for (const relativePath of afterPack.ANALYTIX_MANAGED_CHROME_REQUIRED_PATHS) {
    touch(afterPack._internals.bundledManagedChromePath(context, relativePath))
  }
}

afterEach(() => {
  packagedLifecycleGuard._internals.resetForTests()
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

type LockedRuntimePackage = {
  dependencies?: Record<string, string>
  optionalDependencies?: Record<string, string>
  peerDependencies?: Record<string, string>
  peerDependenciesMeta?: Record<string, { optional?: boolean }>
  link?: boolean
}

// Test-only traversal of the pinned npm runtime graph. Follow actual nested
// resolution and fail on unresolved required edges; unrelated plugins must not
// expand the DOCX adapter's dependency admission.
function lockedRuntimeClosure(packages: Record<string, LockedRuntimePackage>, root: string): string[] {
  const visited = new Set<string>()
  const queue = [root]
  while (queue.length > 0) {
    const key = queue.pop()!
    if (visited.has(key)) continue
    const pkg = packages[key]
    if (!pkg || pkg.link) throw new Error(`Unresolved locked package: ${key}`)
    visited.add(key)
    const names = new Set([
      ...Object.keys(pkg.dependencies ?? {}),
      ...Object.keys(pkg.optionalDependencies ?? {}),
      ...Object.keys(pkg.peerDependencies ?? {})
    ])
    for (const name of names) {
      if (!/^(?:@[A-Za-z0-9_.-]+\/)?[A-Za-z0-9_.-]+$/.test(name) || name.split('/').some(part => part === '.' || part === '..')) {
        throw new Error('Invalid locked dependency name')
      }
      let scope = key
      let resolved: string | undefined
      while (true) {
        const candidate = `${scope ? `${scope}/` : ''}node_modules/${name}`
        if (Object.hasOwn(packages, candidate)) { resolved = candidate; break }
        if (!scope) break
        const parent = scope.lastIndexOf('/node_modules/')
        scope = parent < 0 ? '' : scope.slice(0, parent)
      }
      const optional = Object.hasOwn(pkg.optionalDependencies ?? {}, name) ||
        (!Object.hasOwn(pkg.dependencies ?? {}, name) && pkg.peerDependenciesMeta?.[name]?.optional === true)
      if (!resolved) {
        if (optional) continue
        throw new Error(`Missing locked dependency: ${key} -> ${name}`)
      }
      queue.push(resolved)
    }
  }
  return [...visited].sort()
}

describe('locked runtime dependency closure', () => {
  it('follows nested and scoped peers, terminates cycles, and ignores unrelated packages', () => {
    const packages: Record<string, LockedRuntimePackage> = {
      'node_modules/docx': { dependencies: { child: '1' } },
      'node_modules/docx/node_modules/child': { dependencies: { docx: '1' }, peerDependencies: { '@scope/peer': '1' } },
      'node_modules/@scope/peer': {},
      'node_modules/child': { dependencies: { 'image-size': '1' } },
      'node_modules/image-size': {}
    }
    expect(lockedRuntimeClosure(packages, 'node_modules/docx')).toEqual([
      'node_modules/@scope/peer', 'node_modules/docx', 'node_modules/docx/node_modules/child'
    ])
    packages['node_modules/docx/node_modules/child'].dependencies!['image-size'] = '1'
    expect(lockedRuntimeClosure(packages, 'node_modules/docx')).toContain('node_modules/image-size')
  })

  it('rejects missing required or linked packages while allowing absent optional dependencies', () => {
    expect(() => lockedRuntimeClosure({ 'node_modules/docx': { dependencies: { missing: '1' } } }, 'node_modules/docx'))
      .toThrow('Missing locked dependency')
    expect(() => lockedRuntimeClosure({ 'node_modules/docx': { peerDependencies: { missing: '1' } } }, 'node_modules/docx'))
      .toThrow('Missing locked dependency')
    expect(() => lockedRuntimeClosure({ 'node_modules/docx': { link: true } }, 'node_modules/docx'))
      .toThrow('Unresolved locked package')
    expect(lockedRuntimeClosure({ 'node_modules/docx': {
      optionalDependencies: { absent: '1' }, peerDependencies: { peer: '1' }, peerDependenciesMeta: { peer: { optional: true } }
    } }, 'node_modules/docx')).toEqual(['node_modules/docx'])
  })
})

describe('electron-builder Analytix packaging', () => {
  it('omits unstaged optional resources while including every staged resource', () => {
    const absent = builderConfigurationFixture(false)
    for (const destination of ['managed-chrome', '.python-runtime', 'python-site-packages', 'pwsh']) {
      expect(absent.windows.extraResources.some((entry: { to: string }) => entry.to === destination)).toBe(false)
      expect(standardWinConfig.extraResources.some((entry: { to: string }) => entry.to === destination)).toBe(true)
    }
  })
  it('packages the admitted pure DOCX document model without the retired HTML converter or native document runtimes', () => {
    const docxSource = readFileSync(
      join(process.cwd(), 'src/main/services/write-docx-service.ts'),
      'utf8'
    )
    const docxImports = docxSource
      .split('\n')
      .filter((line) => line.startsWith('import '))
      .join('\n')

    expect(rootPackage.dependencies).toMatchObject({
      docx: '9.7.1',
      jszip: '3.10.1',
      'remark-parse': '11.0.0',
      unified: '11.0.5'
    })
    expect(rootPackage.dependencies).not.toHaveProperty('html-to-docx')
    expect(rootPackage.devDependencies).not.toHaveProperty('@types/html-to-docx')
    expect(Object.keys(rootPackageLock.packages)).not.toContain('node_modules/html-to-docx')
    const docxClosure = lockedRuntimeClosure(rootPackageLock.packages, 'node_modules/docx')
    expect(docxClosure.some(key => /(?:^|\/)node_modules\/image-size$/.test(key))).toBe(false)
    expect(rootPackage.dependencies).not.toHaveProperty('image-size')
    // The admitted PPTX generator uses its own locked image dimension helper;
    // that does not make it part of the pure DOCX generator.
    expect(lockedRuntimeClosure(rootPackageLock.packages, 'node_modules/pptxgenjs'))
      .toContain('node_modules/image-size')
    expect(docxImports).not.toMatch(/node:child_process|packages\/runtime-go|provider|mcp/i)
    expect(docxSource).not.toMatch(/pandoc|libreoffice|html-to-docx/i)
  })

  it('resolves only the canonical versioned Electron framework fuse target', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const frameworkRoot = join(
      context.appOutDir,
      'Analytix.app',
      'Contents',
      'Frameworks',
      'Electron Framework.framework'
    )
    const versionsRoot = join(frameworkRoot, 'Versions')
    const versionRoot = join(versionsRoot, 'A')
    const target = join(versionRoot, 'Electron Framework')
    const currentAlias = join(versionsRoot, 'Current')
    const binaryAlias = join(frameworkRoot, 'Electron Framework')
    mkdirSync(versionRoot, { recursive: true })
    writeFileSync(target, 'canonical-electron-framework\n', { mode: 0o755 })
    symlinkSync('A', currentAlias)
    symlinkSync('Versions/Current/Electron Framework', binaryAlias)

    expect(afterPack._internals.canonicalElectronFrameworkFuseTargetV1(context))
      .toEqual(expect.objectContaining({ path: target, version: 'A' }))

    rmSync(currentAlias)
    symlinkSync('../../outside-version', currentAlias)
    expect(() => afterPack._internals.canonicalElectronFrameworkFuseTargetV1(context))
      .toThrow(/framework version link is invalid/)

    rmSync(currentAlias)
    symlinkSync('A', currentAlias)
    linkSync(target, join(root, 'framework-hardlink'))
    expect(() => afterPack._internals.canonicalElectronFrameworkFuseTargetV1(context))
      .toThrow(/target is not a single-link regular file/)
  })

  it('keeps universal Mach-O staged payload identity invariant across signing envelopes', () => {
    const context = createMacPackContext(tempRoot())
    mkdirSync(context.appOutDir, { recursive: true })
    const nativePath = join(context.appOutDir, 'universal.node')
    const firstImage = universalMachOFixture(0, 12)
    writeFileSync(nativePath, firstImage)
    const first = afterPack._internals.collectStagedPayloadClosureV1(context)
    expect(first).toEqual(expect.objectContaining({
      entryCount: 1,
      nativeFileCount: 1,
      regularFileCount: 0,
      contentByteLength: 1024
    }))

    const resignedImage = universalMachOFixture(96, 14)
    expect(digest(resignedImage)).not.toBe(digest(firstImage))
    expect(nativeComponentContract.nativePayloadIdentity(resignedImage, 'mach-o')).toEqual(
      nativeComponentContract.nativePayloadIdentity(firstImage, 'mach-o')
    )
    writeFileSync(nativePath, resignedImage)
    expect(afterPack._internals.collectStagedPayloadClosureV1(context)).toEqual(first)

    const tamperedImage = Buffer.from(resignedImage)
    const arm64Offset = tamperedImage.readUInt32BE(8 + 20 + 8)
    tamperedImage[arm64Offset + 0x1f0] ^= 0xff
    writeFileSync(nativePath, tamperedImage)
    expect(afterPack._internals.collectStagedPayloadClosureV1(context).manifestSha256)
      .not.toBe(first.manifestSha256)

    const hiddenPadding = Buffer.from(firstImage)
    hiddenPadding[8 + 2 * 20] = 1
    writeFileSync(nativePath, hiddenPadding)
    expect(() => afterPack._internals.collectStagedPayloadClosureV1(context))
      .toThrow(/padding or overlap/)
  })

  it('binds the browser-process V8 snapshot fuse to the exact packaged inventory', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const frameworkRoot = join(
      context.appOutDir,
      'Analytix.app',
      'Contents',
      'Frameworks',
      'Electron Framework.framework'
    )
    const versionRoot = join(frameworkRoot, 'Versions', 'A')
    const resourcesRoot = join(versionRoot, 'Resources')
    const target = join(versionRoot, 'Electron Framework')
    mkdirSync(resourcesRoot, { recursive: true })
    writeFileSync(target, 'canonical-electron-framework\n', { mode: 0o755 })
    symlinkSync('A', join(frameworkRoot, 'Versions', 'Current'))
    symlinkSync('Versions/Current/Electron Framework', join(frameworkRoot, 'Electron Framework'))
    writeFileSync(join(resourcesRoot, 'v8_context_snapshot.arm64.bin'), 'standard-snapshot\n')

    expect(afterPack._internals.verifyElectronV8SnapshotInventoryV1(context))
      .toEqual(expect.objectContaining({
        standardFileName: 'v8_context_snapshot.arm64.bin',
        standardByteLength: 18,
        browserProcessSpecific: false,
        browserFileName: '',
        browserByteLength: 0
      }))

    writeFileSync(join(resourcesRoot, 'browser_v8_context_snapshot.bin'), 'unbound-snapshot\n')
    expect(() => afterPack._internals.verifyElectronV8SnapshotInventoryV1(context))
      .toThrow(/inventory conflicts with fuse policy/)

    rmSync(join(resourcesRoot, 'browser_v8_context_snapshot.bin'))
    rmSync(join(resourcesRoot, 'v8_context_snapshot.arm64.bin'))
    expect(() => afterPack._internals.verifyElectronV8SnapshotInventoryV1(context))
      .toThrow(/inventory is ambiguous/)
  })

  it('keeps the JavaScript packaged authority producer byte-compatible with the Go parser', () => {
    const emptySummary = {
      count: 0,
      byteLength: 0,
      sha256: '0'.repeat(64)
    }
    const withoutWorktreeDigest = {
      schemaVersion: 1,
      contract: afterPack.PACKAGED_WORKTREE_SNAPSHOT_CONTRACT,
      sourceCommit: '1'.repeat(40),
      state: 'clean',
      dirty: false,
      exclusionPolicySha256: '3f4a6441cb53c9b916c6e3f81eaf7b29a26136ce0700b1b44d3bd5053e1a515b',
      stagedPatch: emptySummary,
      unstagedPatch: emptySummary,
      untrackedSource: emptySummary
    }
    const worktreeSnapshot = {
      ...withoutWorktreeDigest,
      snapshotDigest: afterPack._internals.packagedWorktreeSnapshotDigest(withoutWorktreeDigest)
    }
    const buildContext = afterPack._internals.collectEffectiveBuilderContextV1({
      electronPlatformName: 'darwin',
      arch: 'arm64',
      targets: [{ name: 'zip' }, { name: 'dmg' }],
      packager: {
        config: {
          appId: 'com.analytix.desktop',
          productName: 'analytix',
          directories: { output: 'dist' }
        }
      }
    })
    const authority = afterPack._internals.createPackagedBuildAuthorityV2({
      worktreeSnapshot,
      targetKey: 'darwin-arm64',
      buildContext,
      nativeDisposition: {
        kind: afterPack.NATIVE_DISPOSITION_CONTROLLED_RELEASE,
        targetKey: 'darwin-arm64',
        receiptSha256: '2'.repeat(64),
        manifestSha256: '3'.repeat(64),
        signingPolicySha256: '4'.repeat(64),
        signingMode: 'developer-id',
        appleTeamIdentifier: 'ABCDE12345'
      },
      artifacts: {
        executable: {
          preSignSha256: '5'.repeat(64),
          preSignByteLength: 2048,
          payloadSha256: '6'.repeat(64),
          payloadByteLength: 1536,
          format: 'mach-o',
          arch: 'arm64'
        },
        appAsar: { sha256: '7'.repeat(64), byteLength: 4096 },
        runtimeServer: {
          preSignSha256: '8'.repeat(64),
          preSignByteLength: 3072,
          payloadSha256: '9'.repeat(64),
          payloadByteLength: 2560,
          format: 'mach-o',
          arch: 'arm64'
        },
        fundsPlugin: {
          treeSha256: 'b'.repeat(64),
          fileCount: 12,
          manifestSha256: 'c'.repeat(64),
          entrypointSha256: 'd'.repeat(64),
          totalBytes: 8192
        }
      },
      stagedPayload: {
        schemaVersion: 1,
        contract: afterPack.STAGED_PAYLOAD_CONTRACT,
        exclusionPolicySha256: '21d491819a899a7c12f4366e5c62858c97def101cc69ba92e11a59049baee5cd',
        entryCount: 12,
        directoryCount: 4,
        regularFileCount: 6,
        nativeFileCount: 2,
        symlinkCount: 0,
        contentByteLength: 16384,
        manifestSha256: 'a'.repeat(64)
      }
    })
    const golden = readFileSync(resolve(
      process.cwd(),
      'packages/runtime-go/internal/domain/packagedbuildauthority/testdata/packaged-build-authority-v2-js-golden.json'
    ), 'utf8')

    expect(golden).toBe(`${JSON.stringify(authority)}\n`)
    expect(authority.authorityDigest).toBe(
      '1103e3fa03b4bd8e7231f82a2669d244bc2ff39727e7601d118c839680807597'
    )
  })

  it('pins release identity and bundled runtime CLI to analytix', () => {
    const defaultConfig = loadBuilderConfigWithEnv({
      ANALYTIX_APP_VERSION: undefined,
      ANALYTIX_UPDATE_CHANNEL: undefined,
      ANALYTIX_RELEASE_BASE_URL: undefined,
      ANALYTIX_UPDATE_FEED_URL: undefined,
      ANALYTIX_DESKTOP_UPDATE_FEED_URL: undefined
    })

    expect(rootPackage.name).toBe('analytix')
    expect(rootPackage.productName).toBe('Analytix')
    expect(runtimePackage.name).toBe('analytix-runtime')
    expect(runtimePackage.bin).toEqual({
      analytix: './dist/cli/serve-entry.js'
    })
    expect(Object.keys(runtimePackage.exports).sort()).toEqual([
      '.',
      './cli',
      './config',
      './contracts',
      './telemetry'
    ])
    expect(runtimePackage.exports).not.toHaveProperty('./server')
    expect(runtimePackage.exports).not.toHaveProperty('./loop')
    expect(runtimePackage.exports).not.toHaveProperty('./adapters')
    expect(defaultConfig.appId).toBe('com.analytix.desktop')
    expect(defaultConfig.productName).toBe('Analytix')
    expect(defaultConfig.executableName).toBe('analytix')
    expect(defaultConfig.protocols).toEqual([
      {
        name: 'Analytix OAuth Callback',
        schemes: ['com.analytix.desktop']
      }
    ])
    expect(defaultConfig).not.toHaveProperty('electronFuses')
    expect(electronFusePolicy.ELECTRON_FUSE_POLICY_V1).toEqual({
      0: true,
      1: true,
      2: false,
      3: false,
      4: true,
      5: true,
      6: false,
      7: false,
      8: true,
      version: '1',
      strictlyRequireAllFuses: true
    })
    expect(electronFusePolicy.electronFusePolicyV1Digest()).toBe(
      'a11a3d69fb77157f56af8fd3332ae08059dd66a967d52facc373c36573b2b62c'
    )
    expect(rootPackage.devDependencies['@electron/fuses']).toBe('2.1.3')
    expect(rootPackage.devDependencies.electron).toBe('41.10.3')
    expect(defaultConfig.artifactName).toBe('analytix-${version}-${os}-${arch}.${ext}')
    expect(defaultConfig.publish).toEqual([
      {
        provider: 'generic',
        url: 'https://analytix.top/desktop/releases/standard-win/'
      }
    ])
    expect(defaultConfig).not.toHaveProperty('beforePack')
    expect(defaultConfig.artifactBuildStarted).toBe('./scripts/packaged-lifecycle-guard.cjs')
    expect(defaultConfig.artifactBuildCompleted).toBe('./scripts/packaged-lifecycle-guard.cjs')
    expect(defaultConfig.afterExtract).toBe('./scripts/after-extract.cjs')
    expect(defaultConfig.afterPack).toBe('./scripts/after-pack.cjs')
    expect(standardWinConfig).not.toHaveProperty('beforePack')
    expect(standardWinConfig.artifactBuildStarted).toBe('./scripts/packaged-lifecycle-guard.cjs')
    expect(standardWinConfig.artifactBuildCompleted).toBe('./scripts/packaged-lifecycle-guard.cjs')
    expect(standardWinConfig.afterExtract).toBe('./scripts/after-extract.cjs')
    expect(standardWinConfig.afterPack).toBe('./scripts/after-pack.cjs')
    expect(defaultConfig.nsis.shortcutName).toBe('Analytix')
    expect(defaultConfig.nsis.uninstallDisplayName).toBe('Analytix')
  })

  it('rejects electron-builder prepackaged input before it can bypass lifecycle hooks', () => {
    for (const argument of [
      '--prepackaged', '--prepackaged=/tmp/untrusted-app',
      '--pd', '--pd=/tmp/untrusted-app', '-pd', '-pd=/tmp/untrusted-app'
    ]) {
      expect(() => packagedLifecycleGuard._internals.assertNoPrepackagedCommandLine([
        'node', 'electron-builder', argument
      ])).toThrow(/Prepackaged electron-builder input is not eligible/)
    }

    const outDir = join(tempRoot(), 'dist')
    const context = {
      outDir,
      arch: 3,
      targets: [{ name: 'dmg' }, { name: 'zip' }],
      packager: { packagerOptions: { prepackaged: null } }
    }
    packagedLifecycleGuard._internals.recordCompletedAfterPack(context, 'a'.repeat(64))
    const dmgEvent = {
      targetPresentableName: 'DMG',
      file: join(outDir, 'analytix-1.0.0-mac-arm64.dmg'),
      arch: 3
    }
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted(dmgEvent)).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      file: `${dmgEvent.file}.blockmap`,
      arch: null,
      target: { name: 'dmg' },
      packager: context.packager,
      safeArtifactName: 'analytix.dmg.blockmap',
      updateInfo: { size: 1 }
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      ...dmgEvent,
      target: { name: 'dmg' },
      packager: context.packager
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted(dmgEvent)).toThrow(
      /lacks one exact completed afterPack lifecycle/
    )
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      targetPresentableName: 'macOS zip',
      file: join(outDir, 'analytix-1.0.0-mac-arm64.zip'),
      arch: 3
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      file: join(outDir, 'analytix-1.0.0-mac-arm64.zip'),
      arch: 3,
      target: { name: 'zip' },
      packager: context.packager
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      targetPresentableName: 'DMG',
      file: join(outDir, 'replayed.dmg'),
      arch: 3
    })).toThrow(/lacks one exact completed afterPack lifecycle/)

    // This is the complete official electron-builder 26.15.3 event shape. A
    // synthetic `packager` property is rejected instead of becoming a false
    // prepackaged check that the real hook can never perform.
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      ...dmgEvent,
      packager: { packagerOptions: { prepackaged: '/tmp/untrusted-app' } }
    })).toThrow(/ArtifactBuildStarted event is invalid/)
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      file: `${dmgEvent.file}.blockmap`, arch: null, target: { name: 'dmg' },
      packager: context.packager
    })).toThrow(/sub-completion lacks one exact start credential/)
    expect(() => packagedLifecycleGuard._internals.recordCompletedAfterPack({
      ...context,
      packager: { packagerOptions: { prepackaged: '/tmp/untrusted-app' } }
    }, 'b'.repeat(64))).toThrow(/Prepackaged electron-builder input is not eligible/)

    const interruptedOutDir = join(tempRoot(), 'dist')
    const interrupted = { ...context, outDir: interruptedOutDir }
    packagedLifecycleGuard._internals.recordCompletedAfterPack(interrupted, 'e'.repeat(64))
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      targetPresentableName: 'DMG', file: join(interruptedOutDir, 'failed.dmg'), arch: 3
    })).not.toThrow()
    // The first target never completes. A later programmatic prepackaged build
    // can consume the unused zip start credential, but the real completed
    // event carries PackagerOptions and definitively fails the build.
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      targetPresentableName: 'macOS zip', file: join(interruptedOutDir, 'replay.zip'), arch: 3
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      file: join(interruptedOutDir, 'replay.zip'),
      arch: 3,
      target: { name: 'zip' },
      packager: { packagerOptions: { prepackaged: '/tmp/untrusted-app' } }
    })).toThrow(/Prepackaged electron-builder input is not eligible/)

    packagedLifecycleGuard._internals.recordCompletedAfterPack({
      ...context,
      targets: []
    }, 'c'.repeat(64))
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted(dmgEvent)).toThrow(
      /lacks one exact completed afterPack lifecycle/
    )
    expect(() => packagedLifecycleGuard._internals.recordCompletedAfterPack({
      ...context,
      targets: [{ name: 'dir' }]
    }, 'c'.repeat(64))).toThrow(/Electron builder target is unsupported/)

    const linuxOutDir = join(tempRoot(), 'linux-dist')
    const linuxContext = {
      outDir: linuxOutDir,
      arch: 1,
      targets: [{ name: 'appImage' }],
      packager: { packagerOptions: { prepackaged: null } }
    }
    packagedLifecycleGuard._internals.recordCompletedAfterPack(linuxContext, 'd'.repeat(64))
    const appImagePath = join(linuxOutDir, 'analytix-1.0.0-linux-x86_64.AppImage')
    expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
      targetPresentableName: 'AppImage', file: appImagePath, arch: 1
    })).not.toThrow()
    expect(() => packagedLifecycleGuard._internals.artifactBuildCompleted({
      file: appImagePath, arch: 1, target: { name: 'appImage' }, packager: linuxContext.packager
    })).not.toThrow()
  })

  it('keeps the migrated standard Windows installer entrypoint available', () => {
    expect(rootPackage.scripts['build:data-native']).toBe('node ./scripts/build-data-analysis-native-tools.cjs')
    expect(rootPackage.scripts['build:data-native:development']).toBe(
      'node ./scripts/build-data-analysis-native-tools.cjs --development'
    )
    expect(rootPackage.scripts.dev).toContain('npm run build:data-native:development')
    expect(rootPackage.scripts['build:data-native:package']).toContain('--all-package-targets')
    expect(rootPackage.scripts['build:data-native:cached']).toBe('node ./scripts/build-windows-release-cache.cjs data-native')
    expect(rootPackage.scripts['build:backend-win-runtime']).toBe('node ./scripts/build-windows-backend-runtime-assets.cjs')
    expect(rootPackage.scripts['build:backend-win-runtime:cached']).toBe('node ./scripts/build-windows-release-cache.cjs backend-win-runtime')
    expect(rootPackage.scripts.dist).toContain('npm run build:data-native:package')
    expect(rootPackage.scripts.dist).not.toContain('build:data-native:development')
    expect(rootPackage.scripts['dist:mac:arm64:dmg']).toContain('--platform darwin --arch arm64')
    expect(rootPackage.scripts['dist:mac:x64:dmg']).toContain('--platform darwin --arch x64')
    expect(rootPackage.scripts['dist:win']).toBe('npm run dist:win:official')
    expect(rootPackage.scripts['dist:win:standard']).toBe('npm run dist:win:official')
    expect(rootPackage.scripts['dist:win:official']).toContain('scripts/require-official-windows-host.cjs')
    expect(rootPackage.scripts['dist:win:official']).toContain('npm run build:backend-win-runtime:cached')
    expect(rootPackage.scripts['dist:win:official']).toContain('npm run build:data-native:cached')
    expect(rootPackage.scripts['dist:win:official']).toContain('electron-builder.standard-win.cjs')
    expect(rootPackage.scripts['dist:win:official']).toContain('scripts/postpackage-standard-win-bootstrapper.cjs')
    expect(rootPackage.scripts['dist:win:official']).toContain('scripts/audit-windows-package.cjs')
    expect(rootPackage.scripts['release:win:official']).toBe(rootPackage.scripts['release:win'])
    expect(standardWinConfig.productName).toBe('Analytix灵鉴')
    expect(standardWinConfig.executableName).toBe('analytix')
    expect(standardWinConfig.artifactName).toBe('analytix-standard-${version}-${arch}.${ext}')
    expect(standardWinConfig.directories.output).toBe('dist-standard-win')
    expect(standardWinConfig.publish).toEqual([
      {
        provider: 'generic',
        url: 'https://analytix.top/desktop/releases/standard-win/'
      }
    ])
    expect(standardWinConfig.win.target).toEqual([{ target: 'nsis', arch: ['x64'] }])
    expect(standardWinConfig.win.forceCodeSigning).toBe(true)
    expect(standardWinConfig.win.signtoolOptions).toEqual(expect.objectContaining({
      signingHashAlgorithms: ['sha256'],
      rfc3161TimeStampServer: 'http://timestamp.digicert.com'
    }))
    expect(standardWinConfig.nsis).toEqual(expect.objectContaining({
      oneClick: true,
      perMachine: true,
      allowElevation: true,
      packElevateHelper: true,
      allowToChangeInstallationDirectory: false,
      runAfterFinish: false,
      createDesktopShortcut: true,
      createStartMenuShortcut: true,
      shortcutName: 'Analytix灵鉴',
      menuCategory: 'Analytix灵鉴',
      uninstallDisplayName: 'Analytix灵鉴',
      include: 'build/standard-win-installer.nsh'
    }))
  })

  it('keeps formal local macOS arm64 packaging independent of release authority', () => {
    const command = rootPackage.scripts['dist:mac:arm64']
    const signedCommand = rootPackage.scripts['dist:mac:signed']

    expect(command).toContain(
      'npm run build:data-native:development -- --platform darwin --arch arm64'
    )
    expect(command).toContain('npm run build')
    expect(command).toContain('electron-builder@26.15.3')
    expect(command).toContain('--publish never --mac dmg --arm64')
    expect(command).not.toContain('dist:mac:arm64:dmg')
    expect(command).not.toContain('dist:mac:arm64:zip')

    expect(rootPackage.scripts['dist:mac:arm64:dmg']).toContain(
      'build-data-analysis-native-tools.cjs --platform darwin --arch arm64'
    )
    expect(rootPackage.scripts['dist:mac:arm64:dmg']).not.toContain('--development')

    expect(signedCommand).not.toContain('npm run dist:mac:arm64 &&')
    expect(signedCommand).toContain('MAC_SIGN=1 npm run dist:mac:arm64:dmg')
    expect(signedCommand).toContain('MAC_SIGN=1 npm run dist:mac:arm64:zip')
  })

  it('isolates the desktop test-account home from the real user data root', () => {
    expect(devTestAccountLauncherSource).toContain("mkdtempSync(join(tmpdir(), 'analytix-test-home-'))")
    expect(devTestAccountLauncherSource).toContain('childEnv.HOME = testHome')
    expect(devTestAccountLauncherSource).toContain('childEnv.USERPROFILE = testHome')
    expect(devTestAccountLauncherSource).toContain("childEnv.XDG_CONFIG_HOME = join(testHome, '.config')")
    expect(devTestAccountLauncherSource).toContain('...localEnv,')
    expect(devTestAccountLauncherSource.indexOf('...localEnv,')).toBeLessThan(
      devTestAccountLauncherSource.indexOf('...process.env,')
    )
    expect(devTestAccountLauncherSource).toContain("NODE_ENV: 'development'")
    expect(devTestAccountLauncherSource).toContain('delete childEnv.ANALYTIX_GO_RUNTIME_SERVER_BIN')
  })

  it('keeps the Windows backend Python runtime source reproducible', () => {
    const script = readFileSync(join(process.cwd(), 'scripts/build-windows-backend-runtime-assets.cjs'), 'utf8')
    const backendManager = readFileSync(join(process.cwd(), 'src/main/data-analysis/backend-manager.ts'), 'utf8')
    const nativeRuntimeIntegrity = readFileSync(join(process.cwd(), 'src/main/data-analysis/native-runtime-integrity.ts'), 'utf8')
    const pythonRuntimeIntegrity = readFileSync(join(process.cwd(), 'src/main/data-analysis/python-runtime-integrity.ts'), 'utf8')
    const nativeRuntimePaths = readFileSync(join(process.cwd(), 'src/main/data-analysis/native-runtime-paths.ts'), 'utf8')
    const auditScript = readFileSync(join(process.cwd(), 'scripts/audit-windows-package.cjs'), 'utf8')

    expect(pythonRuntimeContract.lock.pythonRuntime.version).toBe('3.11.15+20260623')
    expect(pythonRuntimeContract.lock.pythonRuntime.archiveSha256).toBe(
      '7e0a8abfee952efc63dff290022a73f0185b586f522678ae7a757a56f23c289b'
    )
    expect(script).toContain('ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE')
    expect(script).toContain('ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE_SHA256')
    expect(script).toContain('ANALYTIX_WINDOWS_BACKEND_PYTHON_URL')
    expect(script).toContain('ANALYTIX_WINDOWS_BACKEND_ALLOW_UPSTREAM_DOWNLOAD')
    expect(readFileSync(join(process.cwd(), 'scripts/windows-python-runtime-contract.cjs'), 'utf8')).toContain(
      'analytix-python-runtime-manifest.json'
    )
    expect(script).toContain('The build intentionally does not resolve GitHub latest by default.')
    expect(script).toContain('pythonRuntimeVersionText')
    expect(script).toContain('verified by fixed asset sha256')
    expect(script).toContain('REQUIREMENTS_LOCK_PATH')
    expect(script).toContain("'--require-hashes'")
    expect(readFileSync(join(process.cwd(), 'scripts/windows-backend-requirements.lock.txt'), 'utf8')).toContain(
      'uvicorn==0.51.0 --hash=sha256:'
    )
    expect(readFileSync(join(process.cwd(), 'scripts/windows-backend-requirements.lock.txt'), 'utf8')).toContain(
      'httptools==0.8.0 --hash=sha256:'
    )
    expect(script).not.toContain('uvicorn[standard]')
    expect(script).not.toContain('uvloop')
    expect(script).not.toContain('releases/latest')
    expect(script).not.toContain('api.github.com/repos/astral-sh/python-build-standalone/releases/latest')
    expect(script).toContain("'7zip-bin', 'win', 'x64', '7za.exe'")
    expect(script).toContain('analytix-archive-extractor-manifest.json')
    expect(backendManager).toContain('data_analysis_native_authority_unavailable')
    expect(backendManager).not.toContain('node:child_process')
    expect(backendManager).not.toContain('verifyRuntimeNativeComponent')
    expect(backendManager).not.toContain('verifyPackagedPythonBundle')
    expect(backendManager).not.toContain('sanitizePackagedNativeEnvironment')
    expect(backendManager).not.toContain('ANALYTIX_DATA_ANALYSIS_PYTHON')
    expect(backendManager).not.toContain('ANALYTIX_DATA_ANALYSIS_BACKEND_DIR')
    expect(backendManager).not.toContain('ANALYTIX_DATA_ENGINE_BIN')
    expect(backendManager).not.toContain('ANALYTIX_ANALYSIS_COMPUTE_BIN')
    expect(backendManager).not.toContain('ANALYTIX_IMPORT_ACCELERATOR_BIN')
    expect(backendManager).not.toContain('ANALYTIX_CLEANING_OPS_BIN')
    for (const integritySource of [nativeRuntimeIntegrity, pythonRuntimeIntegrity]) {
      expect(integritySource).not.toContain('node:child_process')
      expect(integritySource).not.toContain('spawnSync(')
      expect(integritySource).not.toContain('powershell')
      expect(integritySource).not.toContain('/usr/bin/codesign')
    }
    expect(pythonRuntimeIntegrity).toContain('data_analysis_native_authority_unavailable')
    expect(nativeRuntimePaths).toContain('void options')
    expect(nativeRuntimePaths).not.toContain("'data-native'")
    expect(nativeRuntimePaths).not.toContain('codex-packaged-smoke')
    expect(auditScript).toContain('resources/runtime/7za.exe')
    expect(auditScript).toContain('resources/app.asar.unpacked/node_modules/node-pty/prebuilds/win32-x64/pty.node')
    expect(auditScript).toContain('resources/app.asar.unpacked/node_modules/better-sqlite3/build/Release/better_sqlite3.node')
    expect(auditScript).toContain('resources/app.asar.unpacked/packages/runtime/node_modules/analytix-computer-use/dist/windows/amd64/analytix-computer-use.exe')
    expect(auditScript).toContain('resources/plugins/analytix-fund-analysis/.mcp.json')
    expect(auditScript).toContain('resources/plugins/analytix-fund-analysis/.codex-plugin/plugin.json')
    expect(auditScript).toContain('process.env.ANALYTIX_APP_VERSION')
    expect(auditScript).toContain('bootstrapperPayload')
    expect(auditScript).toContain('AnalytixPayload.7z')
    expect(auditScript).toContain('AnalytixPayloadArchive')
    expect(auditScript).toContain('PayloadExtractorExe')
    expect(auditScript).toContain('NullsoftInst')
    expect(auditScript).toContain('nodeModulesExamplesDocs')
    expect(auditScript).toContain('privacyProjectionNative')
    expect(auditScript).toContain('nonWin32X64Native')
    expect(auditScript).toContain('@napi-rs')
    expect(auditScript).toContain('oldTypeScriptRuntimeSource')
    expect(auditScript).toContain('data-engine')
    expect(auditScript).toContain('collectSymlinks')
    expect(auditScript).toContain('packaged symlinks/reparse points found')
    expect(auditScript).toContain('Get-AuthenticodeSignature')
    expect(rootPackage.scripts['dist:win:official']).toContain('scripts/stage-private-pwsh.cjs')
    expect(readFileSync(join(process.cwd(), 'scripts/stage-private-pwsh.cjs'), 'utf8')).toContain('ANALYTIX_PRIVATE_PWSH_SOURCE')
    expect(readFileSync(join(process.cwd(), 'scripts/stage-private-pwsh.cjs'), 'utf8')).toContain('ANALYTIX_PRIVATE_PWSH_SHA256')
    expect(readFileSync(join(process.cwd(), 'electron-builder.config.cjs'), 'utf8')).toContain("to: 'pwsh'")
    expect(auditScript).toContain('analytix-private-pwsh-manifest.json')
  })

  it('binds the Windows Python runtime and wheels to signing-stable frozen content', () => {
    expect(pythonRuntimeContract.lock.pythonRuntime.archiveSha256).toBe(
      '7e0a8abfee952efc63dff290022a73f0185b586f522678ae7a757a56f23c289b'
    )
    expect(pythonRuntimeAuthority).toEqual(expect.objectContaining({
      runtimeContentSha256: pythonRuntimeContract.lock.pythonRuntime.runtimeContentSha256,
      sitePackagesContentSha256: pythonRuntimeContract.lock.sitePackages.contentSha256,
      wheelSetSha256: pythonRuntimeContract.lock.sitePackages.wheelSetSha256
    }))

    const root = tempRoot()
    const executable = join(root, 'python.exe')
    writeFileSync(join(root, 'module.py'), 'value = 1\n')
    const unsigned = nativeFixtureBytes('pe', 'x64')
    writeFileSync(executable, unsigned)
    const nodeUnsigned = pythonRuntimeContract.signingInvariantTreeIdentity(root)
    expect(pythonRuntimeIntegrityInternals.treeIdentity(root, '<none>')).toEqual(nodeUnsigned)

    const signed = Buffer.concat([unsigned, Buffer.alloc(16)])
    const optionalHeader = 0x80 + 24
    signed.writeUInt32LE(0x12345678, optionalHeader + 64)
    signed.writeUInt32LE(unsigned.length, optionalHeader + 144)
    signed.writeUInt32LE(16, optionalHeader + 148)
    signed.writeUInt32LE(16, unsigned.length)
    signed.writeUInt16LE(0x0200, unsigned.length + 4)
    signed.writeUInt16LE(0x0002, unsigned.length + 6)
    writeFileSync(executable, signed)
    expect(pythonRuntimeContract.signingInvariantTreeIdentity(root)).toEqual(nodeUnsigned)
    expect(pythonRuntimeIntegrityInternals.treeIdentity(root, '<none>')).toEqual(nodeUnsigned)

    const tampered = Buffer.from(signed)
    tampered[0x250] ^= 0xff
    writeFileSync(executable, tampered)
    expect(pythonRuntimeContract.signingInvariantTreeIdentity(root)).not.toEqual(nodeUnsigned)

    const source = tempRoot()
    mkdirSync(join(source, 'python'), { recursive: true })
    writeFileSync(join(source, 'python', 'python.exe'), unsigned)
    expect(() => pythonRuntimeBuild.stagePythonRuntimeFromSource({
      sourceRoot: source,
      currentRoot: join(tempRoot(), 'current'),
      approvedSourceSha256: '0'.repeat(64)
    })).toThrow(/must equal the frozen approved Python source tree SHA-256/)
  })

  it('keeps official Windows release caching limited to reproducible prerequisites', () => {
    const root = tempRoot()
    const outputDir = join(root, 'python-site-packages')
    touch(join(outputDir, 'package/module.py'))
    const originalOutputs = officialWinCache._internals.outputStatus([outputDir])

    expect(officialWinHost.isOfficialWindowsHost()).toBe(process.platform === 'win32' && process.arch === 'x64')
    expect(officialWinHost.unsignedQaOverrideEnabled({ ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA: '1' })).toBe(true)
    expect(officialWinHost.unsignedQaOverrideEnabled({})).toBe(false)
    expect(officialWinHost.expectedSignerConfigured({ ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1: 'A'.repeat(40) })).toBe(true)
    expect(officialWinHost.expectedSignerConfigured({})).toBe(false)
    expect(Object.keys(officialWinCache._internals.steps).sort()).toEqual(['backend-win-runtime'])
    expect(officialWinCache._internals.steps['backend-win-runtime'].outputs).toEqual(expect.arrayContaining([
      'build/windows-backend-runtime/.python-runtime/current/python/python.exe',
      'build/windows-backend-runtime/python-site-packages/analytix-site-packages-manifest.json',
      'runtime/7za.exe'
    ]))
    expect(officialWinCache._internals.steps['backend-win-runtime'].env).toEqual(expect.arrayContaining([
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE',
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_RUNTIME_SOURCE_SHA256',
      'ANALYTIX_WINDOWS_BACKEND_PYTHON_URL',
      'ANALYTIX_WINDOWS_BACKEND_ALLOW_UPSTREAM_DOWNLOAD'
    ]))
    expect(() => officialWinCache.runStep('data-native')).toThrow(/native_component_release_authority_unavailable/)
    expect(() => officialWinCache._internals.computeStepKey('data-native')).toThrow(
      /native_component_release_authority_unavailable/
    )
    expect(originalOutputs[0]).toEqual(expect.objectContaining({
      exists: true,
      kind: 'directory',
      size: expect.any(Number),
      sha256: expect.any(String)
    }))
    expect(officialWinCache._internals.outputsMatchState(originalOutputs, {
      outputs: originalOutputs
    })).toBe(true)

    touch(join(outputDir, 'package/changed.py'))

    expect(officialWinCache._internals.outputsMatchState(
      officialWinCache._internals.outputStatus([outputDir]),
      { outputs: originalOutputs }
    )).toBe(false)
  })

  it('keeps public Go runtime package scripts product-clean', () => {
    const publicRuntimeScripts = Object.entries(rootPackage.scripts)
      .filter(([name]) => name === 'qa:runtime:packaged' || name.startsWith('runtime:go:'))
    const legacyRuntimeMarker = /\b(?:d0(?:24|25)[a-z0-9_-]*|proof|conformance|fixture|fake|mcpLocalProof)\b/i

    expect(rootPackage.scripts['runtime:go:performance-check']).toBe(
      'node ./scripts/runtime-go-validation-command.mjs performance-check'
    )
    expect(publicRuntimeScripts.length).toBeGreaterThan(0)
    for (const [name, command] of publicRuntimeScripts) {
      expect(`${name}: ${command}`).not.toMatch(legacyRuntimeMarker)
      if (name === 'runtime:go:core-stage') {
        expect(command).toBe('node ./scripts/runtime-go-release-gate.mjs --core-stage --json')
      } else {
        expect(command).toContain('scripts/runtime-go-validation-command.mjs')
      }
    }
  })

  it('keeps packaged Go runtime on a binary-only production boundary', () => {
    expect(runtimeBuildConfig.include).toEqual(expect.arrayContaining([
      'src/index.ts',
      'src/contracts/**/*.ts',
      'src/config/**/*.ts',
      'src/cli/**/*.ts',
      'src/telemetry/**/*.ts',
      'src/hooks/hook-config.ts',
      'src/hooks/hook-phases.ts'
    ]))
    expect(runtimeBuildConfig.include).not.toContain('src/**/*')
    expect(afterPack.ANALYTIX_RUNTIME_GO_REQUIRED_PATHS).toEqual([
      'runtime-go/bin/runtime-server'
    ])
    expect(afterPack.ANALYTIX_RUNTIME_FORBIDDEN_PATHS).toEqual(expect.arrayContaining([
      'packages/runtime/dist/server',
      'packages/runtime/dist/loop',
      'packages/runtime/dist/adapters/model'
    ]))
    expect(afterPack.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS).toEqual(expect.arrayContaining([
      'packages/runtime-go/cmd/contract-sidecar/main.go',
      'packages/runtime-go/cmd/runtime-engine-absorption-report/main.go',
      'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
      'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
      'packages/runtime-go/internal/mcp/manager_test_double.go',
      'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
      'packages/runtime-go/internal/provider/provider_contract_matrix.go',
      'packages/runtime-go/internal/testsupport/providerscript/provider_script.go',
      'packages/runtime-go/internal/readiness/mcp_matrix_contract.go',
      'packages/runtime-go/internal/readiness/provider_matrix_contract.go',
      'packages/runtime-go/internal/conformance/livelocal/harness.go',
      'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
      'packages/runtime-go/internal/conformance/livelocal/production_candidate.go'
    ]))
    const forbiddenPatternSources = afterPack.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS
      .map((pattern: RegExp) => pattern.source)
    expect(forbiddenPatternSources).toEqual(expect.arrayContaining([
      '^packages\\/runtime-go\\/internal\\/upstreamaudit\\/',
      '^packages\\/runtime-go\\/internal\\/conformance\\/',
      '^packages\\/runtime-go\\/internal\\/testsupport\\/providerscript\\/',
      '^packages\\/runtime-go\\/internal\\/readiness\\/.*_conformance\\.go$',
      '^packages\\/runtime-go\\/internal\\/server\\/.*_conformance\\.go$'
    ]))
  })

  it('includes Analytix runtime dependencies in the packaged app', () => {
    expect(builderConfig.files).toEqual(expect.arrayContaining([
      'packages/runtime/dist/cli/**/*',
      'packages/runtime/dist/config/**/*',
      'packages/runtime/dist/contracts/**/*',
      'packages/runtime/dist/hooks/**/*',
      'packages/runtime/package.json',
      'packages/runtime/package-lock.json',
      'packages/runtime/node_modules/**/*',
      '!**/.DS_Store',
      '!**/._*'
    ]))
    expect(builderConfig.files).toEqual(expect.arrayContaining([
      '!**/test/**',
      '!**/tests/**',
      '!**/__tests__/**',
      '!**/fixtures/**',
      '!**/__fixtures__/**',
      '!**/testsupport/**',
      '!**/conformance/**',
      '!**/upstreamaudit/**',
      '!**/readiness/**',
      '!**/benchmark/**',
      '!**/benchmarks/**',
      '!**/coverage/**',
      '!**/demo/**',
      '!**/demos/**',
      '!**/doc/**',
      '!**/docs/**',
      '!**/documentation/**',
      '!**/example/**',
      '!**/examples/**',
      '!**/README*',
      '!**/Readme*',
      '!**/readme*',
      '!**/CHANGELOG*',
      '!**/ChangeLog*',
      '!**/changelog*'
    ]))
    expect(builderConfig.files).not.toContain('packages/runtime/dist/**/*')
    for (const forbiddenPattern of [
      'runtime',
      'packages/runtime-go/go.mod',
      'packages/runtime-go/cmd/runtime-server/**/*',
      'packages/runtime-go/internal/server/**/*',
      'packages/runtime/dist/server/**/*',
      'packages/runtime/dist/loop/**/*',
      'packages/runtime/dist/adapters/model/**/*'
    ]) {
      expect(builderConfig.files).not.toContain(forbiddenPattern)
    }
    expect(builderConfig.asarUnpack).toEqual(expect.arrayContaining([
      '**/packages/runtime/dist/cli/**/*',
      '**/packages/runtime/dist/config/**/*',
      '**/packages/runtime/dist/contracts/**/*',
      '**/packages/runtime/dist/hooks/**/*',
      '**/packages/runtime/package*.json',
      '**/packages/runtime/node_modules/**/*'
    ]))
    expect(builderConfig.asarUnpack).not.toContain('**/packages/runtime/dist/**/*')
    expect(builderConfig.asarUnpack).toContain('**/node_modules/analytix-computer-use/**/*')
    expect(builderConfig.asarUnpack).toContain('**/node_modules/node-pty/**/*')
    expect(builderConfig.asarUnpack).not.toContain('**/packages/runtime-go/**/*')
    const expectedRuntimeResource = {
      from: 'runtime',
      to: 'runtime',
      filter: [
        '7za.exe',
        'analytix-archive-extractor-manifest.json',
        '!**/.DS_Store',
        '!**/._*'
      ]
    }
    const expectedFundsPluginFilter = [
      '.analytix-plugin/package.json',
      '.codex-plugin/plugin.json',
      '.mcp.json',
      'agents/**/*',
      'assets/**/*',
      'references/**/*',
      'skills/**/*',
      ...productionFundsMcpEntryClosure.files,
      '!**/.DS_Store',
      '!**/._*'
    ]
    for (const resources of [builderConfig.extraResources, standardWinConfig.extraResources]) {
      const runtimeRoot = resolve(process.cwd(), 'runtime')
      const runtimeResources = resources.filter((resource: { from?: unknown }) => (
        typeof resource.from === 'string' &&
        resolve(process.cwd(), resource.from) === runtimeRoot
      ))
      expect(runtimeResources).toEqual([expectedRuntimeResource])
      expect(resources.filter((resource: { from?: unknown }) => (
        resource.from === 'plugins/analytix-fund-analysis'
      ))).toEqual([{
        from: 'plugins/analytix-fund-analysis',
        to: 'plugins/analytix-fund-analysis',
        filter: expectedFundsPluginFilter
      }])
      for (const resource of resources) {
        expect(JSON.stringify(resource)).not.toMatch(/(?:data-native|native-components)/)
        if (typeof resource.from !== 'string') continue
        const source = resolve(process.cwd(), resource.from)
        const sourceToRuntime = relative(source, runtimeRoot)
        const runtimeToSource = relative(runtimeRoot, source)
        const sourceContainsRuntime = sourceToRuntime === '' || (
          sourceToRuntime !== '..' &&
          !sourceToRuntime.startsWith(`..${sep}`) &&
          !isAbsolute(sourceToRuntime)
        )
        const sourceIsInsideRuntime = runtimeToSource === '' || (
          runtimeToSource !== '..' &&
          !runtimeToSource.startsWith(`..${sep}`) &&
          !isAbsolute(runtimeToSource)
        )
        if (sourceContainsRuntime || sourceIsInsideRuntime) {
          expect(source).toBe(runtimeRoot)
        }
      }
    }
    expect(builderConfig.extraResources).toEqual(expect.arrayContaining([
      expectedRuntimeResource,
      expect.objectContaining({
        from: 'backend',
        filter: expect.arrayContaining(['!**/.DS_Store', '!**/._*', '!**/.pytest_cache/**', '!**/tests/**', '!**/test/**'])
      })
    ]))
    expect(builderConfig.asarUnpack).not.toEqual(expect.arrayContaining([
      '**/node_modules/node-bin-darwin-*/*',
      '**/node_modules/node-bin-linux-*/*',
      '**/node_modules/node-bin-win-*/*',
      '**/node_modules/openclaw/**/*',
      '**/node_modules/@tencent-weixin/openclaw-weixin/**/*'
    ]))
    // The openclaw shim (vendor/openclaw-shim) must ship: the WeChat bridge
    // imports the bundled plugin's dist at runtime to send media, and that
    // import chain resolves openclaw/plugin-sdk/*.
    expect(builderConfig.files).not.toEqual(expect.arrayContaining([
      '!**/node_modules/openclaw/**/*'
    ]))
    expect(standardWinConfig.extraResources).toEqual(expect.arrayContaining([
      expect.objectContaining({
        from: expect.stringMatching(/managed-chrome$/),
        to: 'managed-chrome',
        filter: expect.arrayContaining([
          'codex-extension/**/*',
          'chrome/extension-host/windows/x64/extension-host.exe',
          'chrome/scripts/browser-client.mjs'
        ])
      })
    ]))
  })

  it('keeps the exact OpenClaw-derived shim metadata and MIT license in the package plan', () => {
    expect(rootPackage.dependencies.openclaw).toBe('file:vendor/openclaw-shim')
    expect(rootPackageLock.packages['node_modules/openclaw']).toEqual({
      resolved: 'vendor/openclaw-shim',
      link: true
    })
    expect(rootPackageLock.packages['vendor/openclaw-shim']).toMatchObject({
      name: 'openclaw',
      version: '2026.6.6+analytix.shim.0',
      license: 'MIT'
    })
    expect(openClawShimPackage).toMatchObject({
      name: 'openclaw',
      version: '2026.6.6+analytix.shim.0',
      author: 'Guoqin He (GitHub: Eysn0130)',
      maintainers: ['Guoqin He (GitHub: Eysn0130)'],
      contributors: [
        'OpenClaw contributors (upstream v2026.5.18; https://github.com/openclaw/openclaw)'
      ],
      license: 'MIT',
      analytixProvenance: {
        upstreamRepository: 'https://github.com/openclaw/openclaw',
        upstreamTag: 'v2026.5.18',
        upstreamTagObject: '0c5e335df4311f135a36f7f72fcd87784dd01c96',
        upstreamCommit: '50a2481652b6a62d573ece3cead60400dc77020d'
      }
    })
    expect(createHash('sha256').update(openClawShimLicense).digest('hex')).toBe(
      '62316704df7426e5a79d2827ff8aca36e9abb3a73b8e68557030749ebefec667'
    )
    expect(thirdPartyNotices).toContain('## OpenClaw-derived compatibility shim')
    expect(thirdPartyNotices).toContain('Copyright (c) 2025 Peter Steinberger')
    for (const config of [builderConfig, standardWinConfig]) {
      expect(config.files).toContain('node_modules/openclaw/LICENSE')
      expect(config.files).not.toEqual(expect.arrayContaining([
        '!**/node_modules/openclaw/**/*',
        '!**/node_modules/openclaw/LICENSE',
        '!**/LICENSE'
      ]))
    }
  })

  it('accepts only the exact hashed funds MCP production closure in packaged resources', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const packagedPluginRoot = writeBundledFundsPlugin(context)
    expect(() => afterPack._internals.validateBundledFundsPlugin(context)).not.toThrow()

    const packagedDeclaration = join(packagedPluginRoot, '.analytix-plugin', 'package.json')
    rmSync(packagedDeclaration)
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_plugin_declaration_packaged_invalid/)
    cpSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', '.analytix-plugin', 'package.json'),
      packagedDeclaration
    )
    writeFileSync(packagedDeclaration, `${readFileSync(packagedDeclaration, 'utf8')} `, 'utf8')
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_plugin_declaration_hash_mismatch/)
    cpSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', '.analytix-plugin', 'package.json'),
      packagedDeclaration
    )

    const dormantModule = join(packagedPluginRoot, 'mcp', 'tool-call-runtime.mjs')
    cpSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', 'mcp', 'tool-call-runtime.mjs'),
      dormantModule
    )
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_mcp_inventory_mismatch/)
    rmSync(dormantModule)

    const packagedServer = join(packagedPluginRoot, 'mcp', 'server.mjs')
    rmSync(packagedServer)
    symlinkSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', 'mcp', 'server.mjs'),
      packagedServer
    )
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_mcp_packaged_file_invalid/)
    rmSync(packagedServer)
    cpSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', 'mcp', 'server.mjs'),
      packagedServer
    )
    writeFileSync(packagedServer, `${readFileSync(packagedServer, 'utf8')}\n// mutation\n`, 'utf8')
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_mcp_hash_mismatch/)

    cpSync(
      join(process.cwd(), 'plugins', 'analytix-fund-analysis', 'mcp', 'server.mjs'),
      packagedServer
    )
    const packagedMcpConfigPath = join(packagedPluginRoot, '.mcp.json')
    const packagedMcpConfig = JSON.parse(readFileSync(packagedMcpConfigPath, 'utf8'))
    packagedMcpConfig.mcpServers.analytix_funds.disabled = false
    writeFileSync(packagedMcpConfigPath, JSON.stringify(packagedMcpConfig), 'utf8')
    expect(() => afterPack._internals.validateBundledFundsPlugin(context))
      .toThrow(/funds_plugin_config_hash_mismatch/)
  })

  it('rejects staged funds projection drift before packaged authority publication', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const sourceRoot = join(root, 'isolated-funds-source')
    cpSync(join(process.cwd(), 'plugins', 'analytix-fund-analysis'), sourceRoot, { recursive: true })
    const manifestPath = join(sourceRoot, '.codex-plugin', 'plugin.json')
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
    manifest.version = '0.16.15'
    writeFileSync(manifestPath, JSON.stringify(manifest), 'utf8')
    writeBundledFundsPlugin(context, sourceRoot)

    const authorityPath = join(
      afterPack._internals.packedResourcesDir(context),
      'runtime',
      afterPack.PACKAGED_BUILD_AUTHORITY_FILE
    )
    expect(existsSync(authorityPath)).toBe(false)
    expect(() => afterPack._internals.validateBundledFundsPlugin(context, { sourceRoot }))
      .toThrow(/funds_plugin_projection_parity_invalid/)
    expect(existsSync(authorityPath)).toBe(false)
  })

  it('fails Hub package preparation before effects when the canonical version is the only changed projection', () => {
    const root = tempRoot()
    const canonicalRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
    const driftedRoot = join(root, 'drifted-funds-source')
    cpSync(canonicalRoot, driftedRoot, { recursive: true })
    const declarationPath = join(driftedRoot, '.analytix-plugin', 'package.json')
    const declaration = JSON.parse(readFileSync(declarationPath, 'utf8'))
    declaration.packageVersion = '0.16.17'
    writeFileSync(declarationPath, `${JSON.stringify(declaration)}\n`, 'utf8')
    const driftedOutRoot = join(root, 'drifted-out')
    const driftedArchive = join(root, 'drifted.tar.gz')

    const drifted = spawnSync(process.execPath, [
      prepareHubPackageScript,
      '--plugin-root', driftedRoot,
      '--out-root', driftedOutRoot,
      '--archive', driftedArchive,
      '--json'
    ], {
      cwd: process.cwd(),
      env: process.env,
      encoding: 'utf8'
    })

    expect(drifted.status).toBe(1)
    expect(JSON.parse(drifted.stdout)).toMatchObject({
      ok: false,
      error: expect.stringMatching(/funds_plugin_projection_parity_invalid/)
    })
    expect(existsSync(driftedOutRoot)).toBe(false)
    expect(existsSync(driftedArchive)).toBe(false)

    const validRoot = join(root, 'valid-funds-source')
    cpSync(canonicalRoot, validRoot, { recursive: true })
    const validOutRoot = join(root, 'valid-out')
    const validArchive = join(root, 'valid.tar.gz')
    const valid = spawnSync(process.execPath, [
      prepareHubPackageScript,
      '--plugin-root', validRoot,
      '--out-root', validOutRoot,
      '--archive', validArchive,
      '--json'
    ], {
      cwd: process.cwd(),
      env: process.env,
      encoding: 'utf8'
    })
    expect(valid.status).toBe(0)
    const result = JSON.parse(valid.stdout)
    const canonical = JSON.parse(readFileSync(join(validRoot, '.analytix-plugin', 'package.json'), 'utf8'))
    const manifest = JSON.parse(readFileSync(join(validRoot, '.codex-plugin', 'plugin.json'), 'utf8'))
    const marketplace = JSON.parse(readFileSync(result.marketplace_path, 'utf8'))
    expect(result).toMatchObject({
      ok: true,
      plugin: `${canonical.packageId}@${canonical.packageVersion}`,
      canonical_version: canonical.packageVersion,
      manifest_version: canonical.packageVersion,
      server_version: canonical.packageVersion
    })
    expect(result.archive).toMatchObject({ path: validArchive })
    expect(existsSync(validArchive)).toBe(true)
    const archiveListing = spawnSync('tar', ['-tzf', validArchive], { encoding: 'utf8' })
    expect(archiveListing.status).toBe(0)
    const archiveRoot = `${canonical.packageId}/`
    const archiveEntries = archiveListing.stdout.split('\n').filter(Boolean)
    expect(archiveEntries).toContain(archiveRoot)
    expect(archiveEntries.every((entry) => entry.startsWith(archiveRoot))).toBe(true)
    expect(marketplace.plugins).toEqual([
      expect.objectContaining({
        name: canonical.packageId,
        version: canonical.packageVersion,
        category: manifest.interface.category,
        interface: manifest.interface,
        source: { source: 'local', path: `./plugins/${canonical.packageId}` }
      })
    ])
  })

  it('requires canonical Funds public UI contributions to resolve to stable regular files', () => {
    const root = tempRoot()
    const sourceRoot = join(root, 'funds-source')
    cpSync(join(process.cwd(), 'plugins', 'analytix-fund-analysis'), sourceRoot, { recursive: true })
    expect(() => afterPack._internals.inspectFundsPluginSourceProjectionsV1(sourceRoot)).not.toThrow()

    const publicUiPath = join(sourceRoot, 'agents', 'openai.yaml')
    const outsidePath = join(root, 'outside-openai.yaml')
    writeFileSync(outsidePath, 'interface: {}\n', 'utf8')
    rmSync(publicUiPath)
    symlinkSync(outsidePath, publicUiPath)
    expect(() => afterPack._internals.inspectFundsPluginSourceProjectionsV1(sourceRoot))
      .toThrow(/funds_plugin_public_ui_invalid/)
  })

  it('fails Hub package preparation before effects when a public UI parent symlink escapes the package root', () => {
    const root = tempRoot()
    const sourceRoot = join(root, 'funds-source')
    cpSync(join(process.cwd(), 'plugins', 'analytix-fund-analysis'), sourceRoot, { recursive: true })
    const outsideAgentsRoot = join(root, 'outside-agents')
    mkdirSync(outsideAgentsRoot)
    cpSync(join(sourceRoot, 'agents', 'openai.yaml'), join(outsideAgentsRoot, 'openai.yaml'))
    rmSync(join(sourceRoot, 'agents'), { recursive: true })
    symlinkSync(outsideAgentsRoot, join(sourceRoot, 'agents'))
    const outRoot = join(root, 'escaped-out')
    const archivePath = join(root, 'escaped.tar.gz')

    const escaped = spawnSync(process.execPath, [
      prepareHubPackageScript,
      '--plugin-root', sourceRoot,
      '--out-root', outRoot,
      '--archive', archivePath,
      '--json'
    ], {
      cwd: process.cwd(),
      env: process.env,
      encoding: 'utf8'
    })

    expect(escaped.status).toBe(1)
    expect(JSON.parse(escaped.stdout)).toMatchObject({
      ok: false,
      error: expect.stringMatching(/funds_plugin_public_ui_invalid/)
    })
    expect(existsSync(outRoot)).toBe(false)
    expect(existsSync(archivePath)).toBe(false)
  })

  it('rejects the complete closed declaration v1 corpus at the production staged gate', () => {
    const canonicalRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
    const canonicalBody = readFileSync(
      join(canonicalRoot, '.analytix-plugin', 'package.json'),
      'utf8'
    )
    const canonical = JSON.parse(canonicalBody)
    const cases: Array<{ name: string; body: () => string }> = [
      {
        name: 'unknown top-level field',
        body: () => JSON.stringify({ ...structuredClone(canonical), unknown: true })
      },
      {
        name: 'duplicate top-level field',
        body: () => canonicalBody.replace(
          '"schemaVersion":1,',
          '"schemaVersion":1,"schemaVersion":1,'
        )
      },
      {
        name: 'wrong schema type',
        body: () => JSON.stringify({ ...structuredClone(canonical), schemaVersion: '1' })
      },
      {
        name: 'non-integer schema number',
        body: () => canonicalBody.replace('"schemaVersion":1', '"schemaVersion":1e0')
      },
      {
        name: 'invalid semantic version',
        body: () => JSON.stringify({ ...structuredClone(canonical), packageVersion: '01.0.0' })
      },
      {
        name: 'unsafe contribution path',
        body: () => {
          const declaration = structuredClone(canonical)
          declaration.contributions.skills[0].path = '../escape.md'
          return JSON.stringify(declaration)
        }
      },
      {
        name: 'duplicate contribution identity',
        body: () => {
          const declaration = structuredClone(canonical)
          declaration.contributions.assets[0].id = declaration.contributions.skills[0].id
          return JSON.stringify(declaration)
        }
      },
      {
        name: 'duplicate capability request',
        body: () => {
          const declaration = structuredClone(canonical)
          declaration.requestedCapabilities.push(
            structuredClone(declaration.requestedCapabilities[0])
          )
          return JSON.stringify(declaration)
        }
      },
      {
        name: 'invalid lifecycle protocol',
        body: () => {
          const declaration = structuredClone(canonical)
          declaration.lifecycle.protocolVersion = 0
          return JSON.stringify(declaration)
        }
      }
    ]

    const corpus = [
      { name: 'canonical', body: canonicalBody, valid: true },
      ...cases.map((attack) => ({ name: attack.name, body: attack.body(), valid: false }))
    ]
    const conformanceRoot = tempRoot()
    const corpusPath = join(conformanceRoot, 'declaration-v1-corpus.json')
    const overlayTestPath = join(conformanceRoot, 'declaration_v1_test.go')
    const overlayPath = join(conformanceRoot, 'overlay.json')
    const goModuleRoot = resolve(process.cwd(), 'packages/runtime-go')
    const canonicalGoTestPath = join(
      goModuleRoot,
      'internal/domain/pluginpackage/declaration_v1_test.go'
    )
    const conformanceGo = [
      '',
      'type afterPackDeclarationV1CorpusCase struct {',
      '\tName string `json:"name"`',
      '\tBody string `json:"body"`',
      '\tValid bool `json:"valid"`',
      '}',
      '',
      'func TestAfterPackDeclarationV1Conformance(t *testing.T) {',
      '\tbody, err := os.ReadFile(os.Getenv("ANALYTIX_DECLARATION_V1_CORPUS"))',
      '\tif err != nil { t.Fatal(err) }',
      '\tvar corpus []afterPackDeclarationV1CorpusCase',
      '\tif err := json.Unmarshal(body, &corpus); err != nil { t.Fatal(err) }',
      '\tfor _, testCase := range corpus {',
      '\t\t_, parseErr := ParseDeclarationV1([]byte(testCase.Body))',
      '\t\tif (parseErr == nil) != testCase.Valid {',
      '\t\t\tt.Fatalf("%s: valid=%v err=%v", testCase.Name, testCase.Valid, parseErr)',
      '\t\t}',
      '\t}',
      '}',
      ''
    ].join('\n')
    const canonicalGoTest = readFileSync(canonicalGoTestPath, 'utf8')
    writeFileSync(
      overlayTestPath,
      `${canonicalGoTest.replace('"encoding/json"\n', '"encoding/json"\n\t"os"\n')}${conformanceGo}`,
      'utf8'
    )
    writeFileSync(corpusPath, `${JSON.stringify(corpus)}\n`, 'utf8')
    writeFileSync(
      overlayPath,
      `${JSON.stringify({ Replace: { [canonicalGoTestPath]: overlayTestPath } })}\n`,
      'utf8'
    )
    const goConformance = spawnSync('go', [
      'test', '-overlay', overlayPath, './internal/domain/pluginpackage',
      '-run', '^TestAfterPackDeclarationV1Conformance$', '-count=1'
    ], {
      cwd: goModuleRoot,
      env: { ...process.env, ANALYTIX_DECLARATION_V1_CORPUS: corpusPath },
      encoding: 'utf8'
    })
    expect(
      goConformance.status,
      `Go declaration v1 conformance failed:\n${goConformance.stdout}\n${goConformance.stderr}`
    ).toBe(0)

    for (const attack of cases) {
      const root = tempRoot()
      const context = createMacPackContext(root)
      const sourceRoot = join(root, 'isolated-funds-source')
      cpSync(canonicalRoot, sourceRoot, { recursive: true })
      writeFileSync(
        join(sourceRoot, '.analytix-plugin', 'package.json'),
        `${attack.body().trimEnd()}\n`,
        'utf8'
      )
      writeBundledFundsPlugin(context, sourceRoot)
      expect(
        () => afterPack._internals.validateBundledFundsPlugin(context, { sourceRoot }),
        attack.name
      ).toThrow(/funds_plugin_declaration_v1_invalid|funds_plugin_json_invalid/)
    }
  })

  it('requires the declared funds MCP entrypoint to be the fixed staged regular file', () => {
    const canonicalRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
    for (const materializeOtherEntrypoint of [false, true]) {
      const root = tempRoot()
      const context = createMacPackContext(root)
      const sourceRoot = join(root, 'isolated-funds-source')
      cpSync(canonicalRoot, sourceRoot, { recursive: true })
      const declarationPath = join(sourceRoot, '.analytix-plugin', 'package.json')
      const declaration = JSON.parse(readFileSync(declarationPath, 'utf8'))
      declaration.contributions.mcpServers[0].entrypoint = 'assets/other.mjs'
      writeFileSync(declarationPath, `${JSON.stringify(declaration)}\n`, 'utf8')
      const mcpConfigPath = join(sourceRoot, '.mcp.json')
      const mcpConfig = JSON.parse(readFileSync(mcpConfigPath, 'utf8'))
      mcpConfig.mcpServers.analytix_funds.args = ['./assets/other.mjs']
      writeFileSync(mcpConfigPath, `${JSON.stringify(mcpConfig)}\n`, 'utf8')
      const packagedRoot = writeBundledFundsPlugin(context, sourceRoot)
      if (materializeOtherEntrypoint) {
        for (const pluginRoot of [sourceRoot, packagedRoot]) {
          mkdirSync(join(pluginRoot, 'assets'), { recursive: true })
          writeFileSync(join(pluginRoot, 'assets', 'other.mjs'), 'export {}\n', 'utf8')
        }
      }

      expect(
        () => afterPack._internals.validateBundledFundsPlugin(context, { sourceRoot }),
        materializeOtherEntrypoint ? 'non-fixed entrypoint' : 'missing entrypoint'
      ).toThrow(/funds_plugin_projection_parity_invalid/)
    }
  })

  it('does not publish packaged authority across either post-admission mutation window', () => {
    for (const mutationWindow of ['before authority write', 'after authority rename'] as const) {
      const root = tempRoot()
      const context = createMacPackContext(root)
      const packagedRoot = writeBundledFundsPlugin(context)
      let admittedFundsPlugin: unknown
      expect(() => {
        admittedFundsPlugin = afterPack._internals.validateBundledFundsPlugin(context)
      }).not.toThrow()

      writeNativeComponentBundle(context)
      const nativeTrust = afterPack._internals.verifyPackagedNativeBeforeRuntimeBuild(context, {
        verifyNativeExecution: false
      })
      for (const [path, bytes] of [
        [afterPack._internals.packagedExecutablePath(context), nativeFixtureBytes('mach-o', 'arm64')],
        [afterPack._internals.packagedAppAsarPath(context), Buffer.from('app-asar\n')],
        [afterPack._internals.bundledGoRuntimeServerPath(context), nativeFixtureBytes('mach-o', 'arm64')]
      ] as const) {
        mkdirSync(join(path, '..'), { recursive: true })
        writeFileSync(path, bytes, { mode: 0o755 })
      }

      const worktreeSnapshot = afterPack._internals.collectPackagedWorktreeSnapshotV1(
        process.cwd()
      )
      const buildContext = afterPack._internals.collectEffectiveBuilderContextV1({
        electronPlatformName: 'darwin',
        arch: 'arm64',
        targets: [],
        packager: {
          config: {
            appId: 'com.analytix.desktop',
            productName: 'Analytix',
            directories: { output: 'dist' }
          }
        }
      })
      const authorityPath = join(
        afterPack._internals.packedResourcesDir(context),
        'runtime',
        afterPack.PACKAGED_BUILD_AUTHORITY_FILE
      )
      const mutateDeclaration = () => {
        const declarationPath = join(packagedRoot, '.analytix-plugin', 'package.json')
        const declaration = JSON.parse(readFileSync(declarationPath, 'utf8'))
        declaration.postAdmissionMutation = true
        writeFileSync(declarationPath, `${JSON.stringify(declaration)}\n`, 'utf8')
      }
      const mutationHooks = mutationWindow === 'before authority write'
        ? { afterFundsPluginAdmission: mutateDeclaration }
        : { afterFundsPluginAuthorityRename: mutateDeclaration }
      packagedLifecycleGuard._internals.resetForTests()
      expect(() => afterPack._internals.writePackagedBuildAuthorityV2(context, nativeTrust, {
        repoRoot: process.cwd(),
        env: {},
        worktreeSnapshot,
        buildContext,
        fundsPluginAdmission: admittedFundsPlugin,
        stagedPayload: afterPack._internals.collectStagedPayloadClosureV1(context),
        ...mutationHooks
      }), mutationWindow).toThrow(/funds_plugin_admission_changed/)
      expect(existsSync(authorityPath), mutationWindow).toBe(false)
      expectNoAuthorityTemporary(join(afterPack._internals.packedResourcesDir(context), 'runtime'))
      expect(() => packagedLifecycleGuard._internals.artifactBuildStarted({
        file: join(root, 'dist', 'Analytix.dmg'),
        arch: 3,
        targetPresentableName: 'DMG'
      }), mutationWindow).toThrow(/lacks one exact completed afterPack lifecycle/)
    }
  }, 15_000)

  it('rejects a preexisting regular authority without replacing its bytes, mode, or identity', () => {
    const fixture = prepareAuthorityPublicationFixture()
    const priorBytes = Buffer.from('preexisting-authority\n')
    writeFileSync(fixture.authorityPath, priorBytes, { mode: 0o640 })
    chmodSync(fixture.authorityPath, 0o640)
    const priorIdentity = authorityFileIdentity(fixture.authorityPath)

    expect(() => afterPack._internals.writePackagedBuildAuthorityV2(
      fixture.context,
      fixture.nativeTrust,
      fixture.options
    )).toThrow(/authority.*exists|authority.*conflict/i)
    expect(readFileSync(fixture.authorityPath)).toEqual(priorBytes)
    expect(authorityFileIdentity(fixture.authorityPath)).toEqual(priorIdentity)
    expectNoAuthorityTemporary(fixture.runtimeDir)
    expectNoCompletedAfterPackCredential(fixture.root)
  }, 15_000)

  it('uses the publication primitive to reject a competitor created after the precheck', () => {
    const fixture = prepareAuthorityPublicationFixture()
    const competitorBytes = Buffer.from('concurrent-authority\n')
    let competitorIdentity: ReturnType<typeof authorityFileIdentity> | undefined

    expect(() => afterPack._internals.writePackagedBuildAuthorityV2(
      fixture.context,
      fixture.nativeTrust,
      {
        ...fixture.options,
        afterPackagedBuildAuthorityPrecheck() {
          writeFileSync(fixture.authorityPath, competitorBytes, { flag: 'wx', mode: 0o604 })
          chmodSync(fixture.authorityPath, 0o604)
          competitorIdentity = authorityFileIdentity(fixture.authorityPath)
        }
      }
    )).toThrow(/authority.*exists|authority.*conflict/i)
    expect(readFileSync(fixture.authorityPath)).toEqual(competitorBytes)
    expect(authorityFileIdentity(fixture.authorityPath)).toEqual(competitorIdentity)
    expectNoAuthorityTemporary(fixture.runtimeDir)
    expectNoCompletedAfterPackCredential(fixture.root)
  }, 15_000)

  it('preserves a competitor that replaces this call authority before validation fails', () => {
    const fixture = prepareAuthorityPublicationFixture()
    const competitorBytes = Buffer.from('replacement-authority\n')
    let competitorIdentity: ReturnType<typeof authorityFileIdentity> | undefined

    expect(() => afterPack._internals.writePackagedBuildAuthorityV2(
      fixture.context,
      fixture.nativeTrust,
      {
        ...fixture.options,
        afterFundsPluginAuthorityRename() {
          rmSync(fixture.authorityPath)
          writeFileSync(fixture.authorityPath, competitorBytes, { flag: 'wx', mode: 0o604 })
          chmodSync(fixture.authorityPath, 0o604)
          competitorIdentity = authorityFileIdentity(fixture.authorityPath)
        }
      }
    )).toThrow(/stable readback failed/)
    expect(existsSync(fixture.authorityPath)).toBe(true)
    expect(readFileSync(fixture.authorityPath)).toEqual(competitorBytes)
    expect(authorityFileIdentity(fixture.authorityPath)).toEqual(competitorIdentity)
    expectNoAuthorityTemporary(fixture.runtimeDir)
    expectNoCompletedAfterPackCredential(fixture.root)
  }, 15_000)

  it('publishes a fresh authority from a synced temporary file without a partial final path', () => {
    const fixture = prepareAuthorityPublicationFixture()
    let finalVisibleBeforePublish = true

    const authority = afterPack._internals.writePackagedBuildAuthorityV2(
      fixture.context,
      fixture.nativeTrust,
      {
        ...fixture.options,
        afterPackagedBuildAuthorityPrecheck() {
          finalVisibleBeforePublish = existsSync(fixture.authorityPath)
        }
      }
    )

    expect(finalVisibleBeforePublish).toBe(false)
    expect(JSON.parse(readFileSync(fixture.authorityPath, 'utf8'))).toEqual(authority)
    expect(lstatSync(fixture.authorityPath).nlink).toBe(1)
    expect(lstatSync(fixture.authorityPath).mode & 0o777).toBe(0o600)
    expect(afterPack._internals.readPackagedBuildAuthorityV2(fixture.context)).toEqual(authority)
    expect(() => afterPack._internals.verifyPackagedBuildAuthorityArtifacts(
      fixture.context,
      authority,
      { verifyFuses: false }
    )).not.toThrow()
    expectNoAuthorityTemporary(fixture.runtimeDir)
    expectNoCompletedAfterPackCredential(fixture.root)
  }, 15_000)

  it('binds platform and MCP projections to the canonical declaration without mapping requests to grants', () => {
    const sourceRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
    const declaration = JSON.parse(readFileSync(join(sourceRoot, '.analytix-plugin', 'package.json'), 'utf8'))
    const manifest = JSON.parse(readFileSync(join(sourceRoot, '.codex-plugin', 'plugin.json'), 'utf8'))
    const mcpConfig = JSON.parse(readFileSync(join(sourceRoot, '.mcp.json'), 'utf8'))
    const runtimeIdentity = {
      serverName: 'analytix_funds',
      serverVersion: manifest.version,
      handlerVersion: manifest.version
    }
    const validate = (nextDeclaration: unknown, nextManifest: unknown, nextConfig: unknown, nextRuntime = runtimeIdentity): void => {
      afterPack._internals.validateFundsPluginProjectionParity(
        nextDeclaration,
        nextManifest,
        nextConfig,
        nextRuntime
      )
    }

    expect(declaration.requestedCapabilities.map((item: { id: string }) => item.id))
      .not.toEqual(manifest.interface.capabilities)
    expect(() => validate(declaration, manifest, mcpConfig)).not.toThrow()

    const attacks = [
      () => [{ ...declaration, packageVersion: '0.16.17' }, manifest, mcpConfig, runtimeIdentity],
      () => {
        const nextDeclaration = structuredClone(declaration)
        nextDeclaration.contributions.mcpServers[0].id = 'analytix_other'
        return [nextDeclaration, manifest, mcpConfig, runtimeIdentity]
      },
      () => {
        const nextConfig = structuredClone(mcpConfig)
        nextConfig.mcpServers.analytix_funds.command = 'bash'
        return [declaration, manifest, nextConfig, runtimeIdentity]
      },
      () => {
        const nextConfig = structuredClone(mcpConfig)
        nextConfig.mcpServers.analytix_funds.cwd = '..'
        return [declaration, manifest, nextConfig, runtimeIdentity]
      },
      () => {
        const nextConfig = structuredClone(mcpConfig)
        nextConfig.mcpServers.analytix_funds.args.push('--unsafe')
        return [declaration, manifest, nextConfig, runtimeIdentity]
      },
      () => {
        const nextConfig = structuredClone(mcpConfig)
        nextConfig.mcpServers.analytix_funds.args = ['./mcp/other.mjs']
        return [declaration, manifest, nextConfig, runtimeIdentity]
      },
      () => [declaration, manifest, mcpConfig, { ...runtimeIdentity, serverName: 'analytix_other' }]
    ]
    for (const attack of attacks) {
      const [nextDeclaration, nextManifest, nextConfig, nextRuntime] = attack()
      expect(() => validate(nextDeclaration, nextManifest, nextConfig, nextRuntime))
        .toThrow(/funds_plugin_projection_parity_invalid/)
    }
  })

  it('rejects unsafe funds plugin metadata even when it is supplied as source metadata', () => {
    const sourceRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
    const sourceManifest = JSON.parse(readFileSync(join(sourceRoot, '.codex-plugin', 'plugin.json'), 'utf8'))
    const sourceMcpConfig = JSON.parse(readFileSync(join(sourceRoot, '.mcp.json'), 'utf8'))
    const runtimeVersions = {
      serverVersion: sourceManifest.version,
      handlerVersion: sourceManifest.version
    }
    const validate = (manifest: unknown, config: unknown): void => {
      afterPack._internals.validateFundsPluginMetadata(manifest, config, runtimeVersions)
    }
    expect(() => validate(sourceManifest, sourceMcpConfig)).not.toThrow()

    const attackCases = [
      () => {
        const config = structuredClone(sourceMcpConfig)
        config.mcpServers.attacker = structuredClone(config.mcpServers.analytix_funds)
        return [sourceManifest, config]
      },
      () => {
        const config = structuredClone(sourceMcpConfig)
        config.mcpServers.analytix_funds.env_vars.push('ARBITRARY_SECRET')
        return [sourceManifest, config]
      },
      () => {
        const config = structuredClone(sourceMcpConfig)
        config.mcpServers.analytix_funds.enabled = true
        return [sourceManifest, config]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.version = '999.0.0'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.mcpServers = './evil.json'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.author.name = 'Analytix'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.author.url = 'https://analytix.top'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        delete manifest.author
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.author.name = 'guoqin he'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.author.url = 'https://github.com/Eysn0130/'
        return [manifest, sourceMcpConfig]
      },
      () => {
        const manifest = structuredClone(sourceManifest)
        manifest.interface.capabilities.push('Write')
        return [manifest, sourceMcpConfig]
      }
    ]
    for (const attack of attackCases) {
      const [manifest, config] = attack()
      expect(() => validate(manifest, config)).toThrow(/funds_plugin_(?:manifest_contract|quarantine_config)_invalid/)
    }
  })

  it('blocks packaging before any native trust or runtime build while release authority is unavailable', async () => {
    const previousReleaseIntent = process.env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE
    process.env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE = '1'
    try {
      const root = tempRoot()
      const context = { ...createMacPackContext(root), outDir: join(root, 'dist') }
      await expect(afterPack.default(context)).rejects.toThrow(
        /Required same-process afterExtract snapshot is missing/
      )
      expect(() => afterPack._internals.assertNativePackageQuarantined(context)).toThrow(
        /native_component_release_authority_unavailable/
      )
      expect(existsSync(afterPack._internals.bundledGoRuntimeServerPath(context))).toBe(false)

      const legacy = afterPack._internals.bundledDataAnalysisNativeToolPath(
        context,
        'analytix-import-accelerator'
      )
      touch(legacy)
      expect(() => afterPack._internals.assertNativePackageQuarantined(context)).toThrow(
        /legacy_native_layout_forbidden/
      )
      expect(existsSync(legacy)).toBe(true)
      expect(existsSync(afterPack._internals.bundledGoRuntimeServerPath(context))).toBe(false)

      rmSync(legacy)
      const nestedLegacy = join(
        afterPack._internals.packedResourcesDir(context),
        'runtime',
        'data-native',
        'darwin-arm64',
        'analytix-data-engine'
      )
      touch(nestedLegacy)
      expect(() => afterPack._internals.assertNativePackageQuarantined(context)).toThrow(
        /legacy_native_layout_forbidden/
      )
      expect(existsSync(nestedLegacy)).toBe(true)
      expect(existsSync(afterPack._internals.bundledGoRuntimeServerPath(context))).toBe(false)

      rmSync(join(afterPack._internals.packedResourcesDir(context), 'runtime', 'data-native'), {
        recursive: true,
        force: true
      })
      const generation = join(
        afterPack._internals.packedResourcesDir(context),
        'runtime',
        'native-components',
        'darwin-arm64',
        'current'
      )
      touch(join(generation, 'analytix-data-engine'))
      expect(() => afterPack._internals.assertNativePackageQuarantined(context)).toThrow(
        /generation_transport_invalid/
      )
      expect(existsSync(join(generation, 'analytix-data-engine'))).toBe(true)
      for (const name of [
        ...afterPack.ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS,
        ...afterPack.ANALYTIX_NATIVE_GENERATION_METADATA
      ]) {
        touch(join(generation, name))
      }
      expect(() => afterPack._internals.assertNativePackageQuarantined(context)).toThrow(
        /native_component_release_authority_unavailable/
      )
      expect(existsSync(afterPack._internals.bundledGoRuntimeServerPath(context))).toBe(false)
    } finally {
      if (previousReleaseIntent === undefined) delete process.env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE
      else process.env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE = previousReleaseIntent
    }
  })

  it('validates the unpacked Analytix runtime before release artifacts are created', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const unpackedRoot = afterPack._internals.unpackedAppRoot(context)

    for (const relativePath of afterPack.ANALYTIX_RUNTIME_REQUIRED_PATHS) {
      touch(join(unpackedRoot, relativePath))
    }
    touch(afterPack._internals.bundledGoRuntimeServerPath(context))
    writeNativeComponentBundle(context)
    touchWindowsBackendRuntime(context)
    touch(afterPack._internals.bundledOpenComputerUseNativePath(context))
    touch(join(unpackedRoot, 'node_modules/better-sqlite3/package.json'))
    touch(join(unpackedRoot, 'packages/runtime/node_modules/.bin/analytix-computer-use.cmd'))
    touch(join(unpackedRoot, 'packages/runtime/node_modules/.bin/open-computer-use.cmd'))
    for (const relativePath of afterPack._internals.nodePtyRequiredPaths(context)) {
      touch(join(unpackedRoot, relativePath))
    }
    const validationOptions = {
      verifyNativeExecution: false,
      verifyPythonAuthority: false,
      authorityUse: 'release'
    }

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, {
      verifyNativeExecution: false,
      verifyPythonAuthority: false
    })).not.toThrow()
    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).not.toThrow()

    rmSync(join(unpackedRoot, 'packages/runtime/node_modules/zod'), { recursive: true, force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /packages\/runtime\/node_modules\/zod\/package\.json/
    )

    touch(join(unpackedRoot, 'packages/runtime/node_modules/zod/package.json'))
    rmSync(afterPack._internals.bundledGoRuntimeServerPath(context), { force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /Go runtime server binary/
    )

    touch(afterPack._internals.bundledGoRuntimeServerPath(context))
    writeNativeComponentBundle(context)
    rmSync(afterPack._internals.bundledOpenComputerUseNativePath(context), { force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /Analytix Computer Use native runtime/
    )

    touch(afterPack._internals.bundledOpenComputerUseNativePath(context))
    touch(join(unpackedRoot, afterPack.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS[0]))

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, {
      verifyNativeExecution: false,
      authorityUse: 'release'
    })).toThrow(
      /Forbidden Go runtime internal validation file/
    )

    rmSync(join(unpackedRoot, afterPack.ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS[0]), { force: true })
    touch(join(unpackedRoot, 'packages/runtime-go/internal/upstreamaudit/reasonix_red_matrix.go'))

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, {
      verifyNativeExecution: false,
      authorityUse: 'release'
    })).toThrow(
      /Forbidden Go runtime internal validation file/
    )

    rmSync(join(unpackedRoot, 'packages/runtime-go/internal/evidence'), { recursive: true, force: true })
    touch(join(unpackedRoot, 'packages/runtime-go/internal/readiness/readiness_extra_conformance.go'))

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, {
      verifyNativeExecution: false,
      authorityUse: 'release'
    })).toThrow(
      /Forbidden Go runtime internal validation file/
    )
  })

  it('accepts the production Go native coordinator component receipt contract', () => {
    const requestNonce = digest('native-build-coordinator-request')
    const response = {
      kind: 'analytix_native_build_coordinator_response',
      schema_version: 1,
      status: 'published',
      request_nonce: requestNonce,
      blocker: '',
      generation_id: digest('generation'),
      inventory_sha256: digest('inventory'),
      generation_receipt_sha256: digest('generation-receipt'),
      component_receipt_sha256: digest('component-receipt'),
      publication_binding_sha256: digest('publication-binding')
    }
    expect(nativeBuildProbeAuthority._internals.validBuildCoordinatorResponse(response, requestNonce)).toBe(true)
    const missingComponentReceipt = { ...response } as Partial<typeof response>
    delete missingComponentReceipt.component_receipt_sha256
    expect(nativeBuildProbeAuthority._internals.validBuildCoordinatorResponse(
      missingComponentReceipt,
      requestNonce
    )).toBe(false)
    expect(nativeBuildProbeAuthority._internals.validBuildCoordinatorResponse(
      { ...response, component_receipt_sha256: 'invalid' },
      requestNonce
    )).toBe(false)
    expect(nativeBuildProbeAuthority._internals.validBuildCoordinatorResponse(
      { ...response, caller_release_authority: true },
      requestNonce
    )).toBe(false)
  })

  it('binds native data builds and packaged receipts to the requested architecture', () => {
    const x64Target = nativeComponentContract.targetContract('darwin', 'x64')
    expect(Object.isFrozen(nativeComponentContract.manifest)).toBe(true)
    expect(Object.isFrozen(nativeComponentContract.manifest.components)).toBe(true)
    expect(Object.isFrozen(nativeComponentContract.manifest.components[0].executionProbe.authorityTargets)).toBe(true)
    const registryClone = JSON.parse(JSON.stringify(nativeComponentContract.manifest))
    expect(() => nativeComponentContract.validateNativeComponentManifest(registryClone)).not.toThrow()
    registryClone.components[0].executionProbe.policySha256 = '0'.repeat(64)
    expect(() => nativeComponentContract.validateNativeComponentManifest(registryClone)).toThrow(/not frozen/)
    const extraRegistry = JSON.parse(JSON.stringify(nativeComponentContract.manifest))
    extraRegistry.components.push(extraRegistry.components[0])
    expect(() => nativeComponentContract.validateNativeComponentManifest(extraRegistry)).toThrow(/exactly the authorized/)
    const authorityEnvironment = (root: string, goarch = 'arm64') => ({
      HOME: `${root}/home`,
      GOPATH: `${root}/home`,
      GOCACHE: `${root}/cache`,
      GOTMPDIR: `${root}/tmp`,
      GOOS: 'darwin',
      GOARCH: goarch,
      GOTOOLCHAIN: 'local'
    })
    const authorityDirectories = (root: string) => ({
      home: `${root}/home`,
      cache: `${root}/cache`,
      temporary: `${root}/tmp`
    })
    const firstAuthorityEnvironment = nativeBuildProbeAuthority._internals.canonicalAuthorityBuildEnvironmentDigest(
      authorityEnvironment('/private/tmp/authority-a'),
      authorityDirectories('/private/tmp/authority-a')
    )
    const secondAuthorityEnvironment = nativeBuildProbeAuthority._internals.canonicalAuthorityBuildEnvironmentDigest(
      authorityEnvironment('/private/tmp/authority-b'),
      authorityDirectories('/private/tmp/authority-b')
    )
    expect(firstAuthorityEnvironment).toBe(secondAuthorityEnvironment)
    expect(nativeBuildProbeAuthority._internals.canonicalAuthorityBuildEnvironmentDigest(
      authorityEnvironment('/private/tmp/authority-c', 'amd64'),
      authorityDirectories('/private/tmp/authority-c')
    )).not.toBe(firstAuthorityEnvironment)
    expect(() => nativeBuildProbeAuthority._internals.canonicalAuthorityBuildEnvironmentDigest(
      authorityEnvironment('/private/tmp/authority-d'),
      authorityDirectories('/private/tmp/authority-e')
    )).toThrow(/not isolated/)
    expect(() => dataNativeBuild._internals.parseBuildArgs(['--arch'])).toThrow(/requires a non-empty value/)
    expect(() => dataNativeBuild._internals.parseBuildArgs(['--arch='])).toThrow(/requires a non-empty value/)
    expect(() => dataNativeBuild._internals.parseBuildArgs(['--arch', 'x64', '--arch', 'arm64'])).toThrow(/Duplicate argument/)
    expect(() => dataNativeBuild._internals.parseBuildArgs(['--all-package-targets', '--arch', 'x64'])).toThrow(
      /cannot be combined/
    )
    expect(dataNativeBuild._internals.parseBuildArgs(['--development'])).toEqual(expect.objectContaining({
      development: true,
      allPackageTargets: false
    }))
    expect(() => dataNativeBuild._internals.parseBuildArgs(['--development=true'])).toThrow(/does not accept a value/)
    expect(() => dataNativeBuild._internals.parseBuildArgs([
      '--development',
      '--all-package-targets'
    ])).toThrow(/cannot be combined/)
    const developmentPublishRoot = tempRoot()
    const developmentTemporary = join(developmentPublishRoot, 'temporary')
    const developmentOutput = join(developmentPublishRoot, 'output')
    mkdirSync(developmentTemporary, { mode: 0o700 })
    const developmentSourceComponents = nativeComponentContract.manifest.components.map((component: {
      id: string
      sourceRoot: string
    }) => ({
      id: component.id,
      source_root: component.sourceRoot,
      source_digest: digest(`source:${component.id}`),
      cargo_lock_sha256: digest(`lock:${component.id}`)
    }))
    const developmentBuiltComponents = nativeComponentContract.manifest.components.map((component: {
      id: string
      binaryName: string
    }) => {
      const binaryName = nativeComponentContract.binaryName(component, 'darwin')
      const bytes = nativeFixtureBytes('mach-o', 'x64')
      const payload = nativeComponentContract.nativePayloadIdentity(bytes, 'mach-o')
      writeFileSync(join(developmentTemporary, binaryName), bytes, { mode: 0o700 })
      return {
        id: component.id,
        binaryName,
        buildEnvironmentSha256: digest(`environment:${component.id}`),
        binarySha256: digest(bytes),
        binarySize: bytes.length,
        payloadSha256: payload.sha256,
        payloadSize: payload.size,
        format: 'mach-o',
        arch: 'x64'
      }
    })
    const developmentMarkerPath = dataNativeBuild._internals.publishDevelopmentBuild({
      outputDir: developmentOutput,
      temporaryDir: developmentTemporary,
      target: x64Target,
      sourceSetSha256: digest('development-source-set'),
      buildContextSha256: digest('development-build-context'),
      buildEnvironmentSha256: digest('development-build-environment'),
      toolchain: {
        cargoExecutableSha256: digest('development-cargo'),
        cargoVersion: 'cargo test',
        rustcExecutableSha256: digest('development-rustc'),
        rustcVersion: 'rustc test'
      },
      sourceComponents: developmentSourceComponents,
      builtComponents: developmentBuiltComponents
    })
    const developmentMarker = JSON.parse(readFileSync(developmentMarkerPath, 'utf8'))
    expect(developmentMarker).toEqual(expect.objectContaining({
      kind: 'analytix_native_development_build',
      classification: 'development_non_publishable',
      publishable: false,
      releaseEligible: false,
      authorityUse: 'development_only',
      targetKey: 'darwin-x64'
    }))
    expect(developmentMarker.components).toHaveLength(nativeComponentContract.manifest.components.length)
    expect(existsSync(join(developmentOutput, 'receipt.v1.json'))).toBe(false)
    expect(existsSync(join(developmentOutput, 'analytix-native-components-receipt.json'))).toBe(false)
    expect(() => dataNativeBuild._internals.parseBuildArgs([], {
      ANALYTIX_DATA_NATIVE_OUTPUT_DIR: '/tmp/unsafe-stage'
    })).toThrow(/is forbidden/)
    expect(() => dataNativeBuild._internals.requireLocalBuildAuthorityTarget(
      nativeComponentContract.targetContract('win32', 'x64')
    )).toThrow(/execution authority is unavailable for target win32-x64/)
    expect(() => dataNativeBuild._internals.requireLocalBuildAuthorityTarget(
      nativeComponentContract.targetContract('linux', 'x64')
    )).toThrow(/execution authority is unavailable for target linux-x64/)
    expect(dataNativeBuild._internals.requireLocalBuildAuthorityTarget(x64Target)).toBe(x64Target)
    const stablePath = ['/usr/bin', '/bin'].join(delimiter)
    const nestedNpmPath = ['/usr/bin', '/bin', '/usr/bin', '/bin'].join(delimiter)
    expect(nativeComponentContract.normalizeExecutableSearchPath(nestedNpmPath)).toBe(
      nativeComponentContract.normalizeExecutableSearchPath(stablePath)
    )
    const hermeticPolicyFixture = {
      PATH: stablePath,
      HOME: process.env.HOME || '/tmp'
    }
    expect(() => nativeComponentContract.assertHermeticCargoEnvironment(process.cwd(), {
      ...hermeticPolicyFixture,
      cargo_profile_release_lto: 'thin'
    })).toThrow(/CARGO_PROFILE_RELEASE_LTO/)
    expect(() => nativeComponentContract.assertHermeticCargoEnvironment(process.cwd(), {
      ...hermeticPolicyFixture,
      dyld_insert_libraries: '/tmp/evil.dylib'
    })).toThrow(/DYLD_INSERT_LIBRARIES/)
    expect(() => nativeComponentContract.assertHermeticCargoEnvironment(process.cwd(), {
      ...hermeticPolicyFixture,
      RUSTUP_TOOLCHAIN: 'nightly'
    })).toThrow(/RUSTUP_TOOLCHAIN/)
    expect(() => nativeComponentContract.assertHermeticCargoEnvironment(process.cwd(), {
      ...hermeticPolicyFixture,
      CARGO_TARGET_DIR: '/tmp/shared-target'
    })).toThrow(/CARGO_TARGET_DIR/)
    const ambientCargoRoot = tempRoot()
    mkdirSync(join(ambientCargoRoot, '.cargo'), { recursive: true })
    writeFileSync(join(ambientCargoRoot, '.cargo', 'config.toml'), '[build]\nrustflags = ["-C", "link-arg=evil"]\n')
    const ambientSourceSnapshot = join(ambientCargoRoot, 'snapshot')
    mkdirSync(ambientSourceSnapshot)
    expect(() => nativeComponentContract.assertHermeticCargoEnvironment(ambientSourceSnapshot, hermeticPolicyFixture)).toThrow(
      /Ambient Cargo configuration/
    )
    const baseBuildEnvironment = nativeComponentContract.nativeBuildChildEnvironment(hermeticPolicyFixture)
    const importEnvironment = nativeComponentContract.nativeComponentBuildEnvironment(
      nativeComponentContract.manifest.components[0],
      nativeComponentContract.targetContract('win32', 'x64'),
      { baseEnvironment: baseBuildEnvironment, rustcExecutable: '/trusted/rustc', cargoTargetDirectory: '/isolated/a' }
    )
    const engineEnvironment = nativeComponentContract.nativeComponentBuildEnvironment(
      nativeComponentContract.manifest.components[3],
      nativeComponentContract.targetContract('win32', 'x64'),
      { baseEnvironment: baseBuildEnvironment, rustcExecutable: '/trusted/rustc', cargoTargetDirectory: '/isolated/a' }
    )
    expect(importEnvironment.RUSTFLAGS).toBeUndefined()
    expect(engineEnvironment.RUSTFLAGS).toBe('-C link-arg=Rstrtmgr.lib')
    expect(nativeComponentContract.environmentDigest(importEnvironment)).not.toBe(
      nativeComponentContract.environmentDigest(engineEnvironment)
    )
    const importEnvironmentAtAnotherTargetDir = {
      ...importEnvironment,
      CARGO_TARGET_DIR: '/isolated/b'
    }
    expect(nativeComponentContract.nativeComponentBuildEnvironmentDigest(importEnvironment)).toBe(
      nativeComponentContract.nativeComponentBuildEnvironmentDigest(importEnvironmentAtAnotherTargetDir)
    )
    const lockRoot = tempRoot()
    const lockOutput = join(lockRoot, 'darwin-x64')
    const lock = dataNativeBuild._internals.acquireTargetBuildLock(lockOutput, x64Target)
    expect(() => dataNativeBuild._internals.acquireTargetBuildLock(lockOutput, x64Target)).toThrow(
      /Cannot acquire exclusive target build lock/
    )
    expect(() => dataNativeBuild._internals.assertTargetBuildLock(lock)).not.toThrow()
    dataNativeBuild._internals.releaseTargetBuildLock(lock)
    const exitedOwner = spawnSync(process.execPath, ['-e', ''])
    expect(exitedOwner.status).toBe(0)
    const staleLockPath = `${lockOutput}.build.lock`
    const staleOwner = {
      schemaVersion: 1,
      pid: exitedOwner.pid,
      hostname: hostname(),
      nonce: 'a'.repeat(32),
      targetKey: x64Target.key,
      issuedAt: new Date().toISOString()
    }
    mkdirSync(staleLockPath, { mode: 0o700 })
    writeFileSync(
      join(staleLockPath, 'owner.json'),
      `${JSON.stringify(staleOwner, null, 2)}\n`,
      { mode: 0o600 }
    )
    const recoveredLock = dataNativeBuild._internals.acquireTargetBuildLock(lockOutput, x64Target)
    expect(recoveredLock.owner.pid).toBe(process.pid)
    dataNativeBuild._internals.releaseTargetBuildLock(recoveredLock)
    const interruptedReleasePath = `${staleLockPath}.released`
    mkdirSync(interruptedReleasePath, { mode: 0o700 })
    writeFileSync(
      join(interruptedReleasePath, 'owner.json'),
      `${JSON.stringify(staleOwner, null, 2)}\n`,
      { mode: 0o600 }
    )
    const afterInterruptedRelease = dataNativeBuild._internals.acquireTargetBuildLock(
      lockOutput,
      x64Target
    )
    expect(existsSync(interruptedReleasePath)).toBe(false)
    dataNativeBuild._internals.releaseTargetBuildLock(afterInterruptedRelease)
    const interruptedRetirementPath = `${staleLockPath}.retired`
    mkdirSync(interruptedRetirementPath, { mode: 0o700 })
    const afterInterruptedRetirement = dataNativeBuild._internals.acquireTargetBuildLock(
      lockOutput,
      x64Target
    )
    expect(existsSync(interruptedRetirementPath)).toBe(false)
    dataNativeBuild._internals.releaseTargetBuildLock(afterInterruptedRetirement)
    const sourceRoot = join(lockRoot, 'source')
    mkdirSync(sourceRoot, { recursive: true })
    writeFileSync(join(sourceRoot, 'main.rs'), 'fn main() {}\n')
    symlinkSync(join(sourceRoot, 'main.rs'), join(sourceRoot, 'linked.rs'))
    expect(() => nativeComponentContract.sourceTreeDigest(sourceRoot)).toThrow(/symbolic link/)

    const closureRoot = tempRoot()
    for (const component of nativeComponentContract.manifest.components) {
      const componentRoot = join(closureRoot, component.sourceRoot)
      mkdirSync(join(componentRoot, 'src'), { recursive: true })
      writeFileSync(join(componentRoot, 'Cargo.toml'), `[package]\nname = "${component.binaryName}"\n`)
      writeFileSync(join(componentRoot, 'Cargo.lock'), `lock:${component.id}\n`)
      writeFileSync(join(componentRoot, 'src', 'main.rs'), 'fn main() {}\n')
    }
    const sourceSetBefore = nativeComponentContract.sourceSetDigest(closureRoot)
    const nestedTarget = join(
      closureRoot,
      nativeComponentContract.manifest.components[0].sourceRoot,
      'src',
      'target',
      'included.bin'
    )
    mkdirSync(join(nestedTarget, '..'), { recursive: true })
    writeFileSync(nestedTarget, 'included source input')
    expect(nativeComponentContract.sourceSetDigest(closureRoot)).not.toBe(sourceSetBefore)
    const sourceSetWithNestedTarget = nativeComponentContract.sourceSetDigest(closureRoot)
    mkdirSync(join(closureRoot, '.cargo'), { recursive: true })
    writeFileSync(join(closureRoot, '.cargo', 'config.toml'), '[build]\nrustflags = ["-C", "opt-level=2"]\n')
    expect(nativeComponentContract.sourceSetDigest(closureRoot)).not.toBe(sourceSetWithNestedTarget)
    rmSync(join(closureRoot, '.cargo', 'config.toml'))
    symlinkSync('missing-config', join(closureRoot, '.cargo', 'config.toml'))
    expect(() => nativeComponentContract.sourceSetDigest(closureRoot)).toThrow(/symbolic link/)

    const unicodeSourceRoot = tempRoot()
    writeFileSync(join(unicodeSourceRoot, '\uE000.txt'), 'bmp')
    writeFileSync(join(unicodeSourceRoot, '\u{10000}.txt'), 'astral')
    expect(nativeComponentContract.sourceTreeDigest(unicodeSourceRoot)).toBe(
      'ee16b29c304cb973ad21a41f0f0d54169fd73631da1c37e9f2139abef7c4acda'
    )

    const realAncestorRoot = tempRoot()
    const linkedAncestorRoot = tempRoot()
    const linkedComponent = nativeComponentContract.manifest.components[0]
    const realComponentRoot = join(realAncestorRoot, linkedComponent.sourceRoot)
    mkdirSync(realComponentRoot, { recursive: true })
    writeFileSync(join(realComponentRoot, 'Cargo.toml'), '[package]\nname = "linked"\n')
    symlinkSync(join(realAncestorRoot, 'tools'), join(linkedAncestorRoot, 'tools'))
    expect(() => nativeComponentContract.sourceTreeDigest(
      join(linkedAncestorRoot, linkedComponent.sourceRoot),
      linkedAncestorRoot
    )).toThrow(/traverse a symbolic link/)
    expect(dataNativeBuild._internals.cargoBuildArgs(
      nativeComponentContract.manifest.components[0],
      x64Target
    )).toEqual([
      'build',
      '--release',
      '--frozen',
      '--target',
      'x86_64-apple-darwin',
      '--bin',
      'analytix-import-accelerator'
    ])
    const componentReceipts = nativeComponentContract.manifest.components.map((component: {
      id: string
      sourceRoot: string
    }) => ({
      id: component.id,
      source_root: component.sourceRoot,
      source_digest: nativeComponentContract.sourceTreeDigest(
        join(process.cwd(), component.sourceRoot),
        process.cwd()
      ),
      cargo_lock_sha256: nativeComponentContract.sha256File(join(process.cwd(), component.sourceRoot, 'Cargo.lock')),
      file_count: 1,
      total_bytes: 1
    }))
    const buildContextDigest = nativeComponentContract.nativeBuildContextDigest(process.cwd())
    expect(nativeComponentContract.sourceSetDigestFromComponentReceipts(
      componentReceipts,
      buildContextDigest
    )).toBe(nativeComponentContract.sourceSetDigest(process.cwd()))
    expect(() => nativeComponentContract.sourceSetDigestFromComponentReceipts([
      { ...componentReceipts[1] },
      { ...componentReceipts[0] },
      ...componentReceipts.slice(2)
    ], buildContextDigest)).toThrow(/receipt is invalid/)
    expect(() => nativeComponentContract.sourceSetDigestFromComponentReceipts([
      { ...componentReceipts[0], unknown: true },
      ...componentReceipts.slice(1)
    ], buildContextDigest)).toThrow(/receipt is invalid/)
    expect(() => nativeComponentContract.sourceSetDigestFromComponentReceipts(
      componentReceipts,
      'not-a-digest'
    )).toThrow(/receipts are invalid/)

    const root = tempRoot()
    const context = { ...createMacPackContext(root), arch: 'x64' }
    writeNativeComponentBundle(context, { format: 'mach-o', arch: 'arm64' })
    const runtimeDir = join(afterPack._internals.packedResourcesDir(context), 'runtime')
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, x64Target, {
      verifyExecution: false,
      verifyBuildEnvironment: false,
      authorityUse: 'release'
    })).toThrow(
      /expected mach-o\/x64, got mach-o\/arm64/
    )

    writeNativeComponentBundle(context)
    const binary = afterPack._internals.bundledDataAnalysisNativeToolPath(
      context,
      'analytix-import-accelerator'
    )
    const tampered = Buffer.concat([readFileSync(binary), Buffer.from([0])])
    writeFileSync(binary, tampered)
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, x64Target, {
      verifyExecution: false,
      verifyBuildEnvironment: false,
      authorityUse: 'release'
    })).toThrow(
      /code-signature range|payload receipt mismatch/
    )

    const signedMachO = nativeFixtureBytes('mach-o', 'x64')
    const resignedMachO = Buffer.from(signedMachO)
    resignedMachO[resignedMachO.length - 1] ^= 0xff
    expect(digest(resignedMachO)).not.toBe(digest(signedMachO))
    expect(nativeComponentContract.nativePayloadIdentity(resignedMachO, 'mach-o')).toEqual(
      nativeComponentContract.nativePayloadIdentity(signedMachO, 'mach-o')
    )
    expect(inspectNativePayload(signedMachO, 'mach-o')).toEqual(expect.objectContaining({
      arch: 'x64',
      payloadSha256: nativeComponentContract.nativePayloadIdentity(signedMachO, 'mach-o').sha256,
      payloadSize: nativeComponentContract.nativePayloadIdentity(signedMachO, 'mach-o').size
    }))
    const tamperedMachOPayload = Buffer.from(signedMachO)
    tamperedMachOPayload[0x1f0] ^= 0xff
    expect(nativeComponentContract.nativePayloadIdentity(tamperedMachOPayload, 'mach-o')).not.toEqual(
      nativeComponentContract.nativePayloadIdentity(signedMachO, 'mach-o')
    )

    writeNativeComponentBundle(context)
    const packagedMachO = afterPack._internals.bundledDataAnalysisNativeToolPath(
      context,
      'analytix-import-accelerator'
    )
    const signatureOnlyChange = Buffer.from(readFileSync(packagedMachO))
    signatureOnlyChange[signatureOnlyChange.length - 1] ^= 0xff
    writeFileSync(packagedMachO, signatureOnlyChange)
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, x64Target, {
      verifyExecution: false,
      verifyBuildEnvironment: false,
      requireBuildIdentity: false,
      authorityUse: 'release'
    })).not.toThrow()
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, x64Target, {
      verifyExecution: false,
      verifyBuildEnvironment: false,
      requireBuildIdentity: true,
      authorityUse: 'release'
    })).toThrow(/build identity mismatch/)

    writeNativeComponentBundle(context)
    const payloadChange = Buffer.from(readFileSync(packagedMachO))
    payloadChange[0x1f0] ^= 0xff
    writeFileSync(packagedMachO, payloadChange)
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, x64Target, {
      verifyExecution: false,
      verifyBuildEnvironment: false,
      requireBuildIdentity: false,
      authorityUse: 'release'
    })).toThrow(/payload receipt mismatch/)

    const unsignedPE = nativeFixtureBytes('pe', 'x64')
    const signedPE = Buffer.concat([unsignedPE, Buffer.alloc(16)])
    const peOptionalHeader = 0x80 + 24
    signedPE.writeUInt32LE(0x12345678, peOptionalHeader + 64)
    signedPE.writeUInt32LE(unsignedPE.length, peOptionalHeader + 144)
    signedPE.writeUInt32LE(16, peOptionalHeader + 148)
    signedPE.writeUInt32LE(16, unsignedPE.length)
    signedPE.writeUInt16LE(0x0200, unsignedPE.length + 4)
    signedPE.writeUInt16LE(0x0002, unsignedPE.length + 6)
    signedPE.writeUInt32LE(0xfeedface, unsignedPE.length + 8)
    expect(digest(signedPE)).not.toBe(digest(unsignedPE))
    expect(nativeComponentContract.nativePayloadIdentity(signedPE, 'pe')).toEqual(
      nativeComponentContract.nativePayloadIdentity(unsignedPE, 'pe')
    )
    expect(inspectNativePayload(signedPE, 'pe')).toEqual(expect.objectContaining({
      arch: 'x64',
      payloadSha256: nativeComponentContract.nativePayloadIdentity(signedPE, 'pe').sha256,
      payloadSize: nativeComponentContract.nativePayloadIdentity(signedPE, 'pe').size
    }))
    const peWithOverlay = Buffer.concat([unsignedPE, Buffer.from([0])])
    expect(() => nativeComponentContract.nativePayloadIdentity(peWithOverlay, 'pe')).toThrow(/overlay or gap/)

    const elf = nativeFixtureBytes('elf', 'x64')
    expect(inspectNativePayload(elf, 'elf')).toEqual(expect.objectContaining({
      arch: 'x64',
      payloadSha256: nativeComponentContract.nativePayloadIdentity(elf, 'elf').sha256,
      payloadSize: nativeComponentContract.nativePayloadIdentity(elf, 'elf').size
    }))

    writeNativeComponentBundle(context)
    expect(() => verifyRuntimeNativeComponent({
      root: afterPack._internals.packedResourcesDir(context),
      packaged: false,
      platform: 'darwin',
      arch: 'x64',
      componentId: 'data-engine'
    })).toThrow(/owned by the Go runtime and release authority is unavailable/)

    const truncated = join(root, 'truncated-mach-o')
    const truncatedBytes = Buffer.alloc(8)
    truncatedBytes.writeUInt32LE(0xfeedfacf, 0)
    truncatedBytes.writeUInt32LE(0x01000007, 4)
    writeFileSync(truncated, truncatedBytes)
    expect(() => nativeComponentContract.inspectNativeBinary(truncated)).toThrow(/Unrecognized native binary format/)

    const previousOverride = process.env.ANALYTIX_RUNTIME_GO_BUILD_ARCH
    process.env.ANALYTIX_RUNTIME_GO_BUILD_ARCH = 'arm64'
    try {
      expect(afterPack._internals.goArchForTarget('x64')).toBe('amd64')
    } finally {
      if (previousOverride === undefined) delete process.env.ANALYTIX_RUNTIME_GO_BUILD_ARCH
      else process.env.ANALYTIX_RUNTIME_GO_BUILD_ARCH = previousOverride
    }
  })

  it('binds the native build authority digest to assembly, embed inputs, and a link-free source tree', () => {
    const moduleRoot = tempRoot()
    mkdirSync(join(moduleRoot, 'cmd', 'probe'), { recursive: true })
    mkdirSync(join(moduleRoot, 'internal', 'authority'), { recursive: true })
    writeFileSync(join(moduleRoot, 'go.mod'), 'module fixture.local/authority\n\ngo 1.22\n')
    writeFileSync(join(moduleRoot, 'cmd', 'probe', 'main.go'), 'package main\nfunc main() {}\n')
    const assembly = join(moduleRoot, 'internal', 'authority', 'spawn_darwin.s')
    writeFileSync(assembly, 'TEXT ·spawn(SB),NOSPLIT,$0-0\nRET\n')

    const initial = nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)
    writeFileSync(assembly, 'TEXT ·spawn(SB),NOSPLIT,$0-0\nNOP\nRET\n')
    const assemblyChanged = nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)
    expect(assemblyChanged).not.toBe(initial)

    writeFileSync(join(moduleRoot, 'internal', 'authority', 'policy.bin'), Buffer.from([0, 1, 2, 3]))
    const arbitraryInputChanged = nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)
    expect(arbitraryInputChanged).not.toBe(assemblyChanged)
    for (const directory of ['target', 'vendor']) {
      const embedded = join(moduleRoot, 'internal', 'authority', directory, 'payload.bin')
      mkdirSync(join(embedded, '..'), { recursive: true })
      writeFileSync(embedded, 'alpha')
      const before = nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)
      writeFileSync(embedded, 'bravo')
      expect(nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)).not.toBe(before)
    }
    expect(nativeComponentContract.NATIVE_BUILD_CONTEXT_PATHS).toContain(
      'packages/runtime-go/internal/adapters/outbound/processauthority/spawn_darwin.s'
    )
    expect(nativeComponentContract.NATIVE_BUILD_CONTEXT_PATHS).toEqual(expect.arrayContaining([
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/manifest.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/main_darwin.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/metadata_darwin.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/snapshot_darwin.go'
    ]))

    symlinkSync('spawn_darwin.s', join(moduleRoot, 'internal', 'authority', 'linked-input'))
    expect(() => nativeBuildProbeAuthority._internals.authoritySourceSetDigest(moduleRoot)).toThrow(/symbolic link/)
    rmSync(join(moduleRoot, 'internal', 'authority', 'linked-input'))
    const aliasRoot = join(tempRoot(), 'module-link')
    symlinkSync(moduleRoot, aliasRoot)
    expect(() => nativeBuildProbeAuthority._internals.authoritySourceSetDigest(aliasRoot)).toThrow(/source root is unsafe/)
  })

  it('strictly binds native source snapshot responses to operation, nonce, generation, and component order', () => {
    const request = {
      kind: 'analytix_native_source_snapshot_request',
      schema_version: 1,
      operation: 'create',
      request_nonce: digest('snapshot-request'),
      expected_generation_id: '',
      expected_inventory_sha256: '',
      expected_generation_receipt_sha256: '',
      session_name: 'analytix-native-source-session-test01'
    }
    const components = nativeComponentContract.manifest.components.map((component: {
      id: string
      sourceRoot: string
    }, index: number) => ({
      id: component.id,
      source_root: component.sourceRoot,
      source_digest: digest(`source:${component.id}`),
      cargo_lock_sha256: digest(`lock:${component.id}`),
      file_count: index + 1,
      total_bytes: (index + 1) * 10
    }))
    const response = {
      kind: 'analytix_native_source_snapshot_response',
      schema_version: 1,
      status: 'committed',
      operation: 'create',
      request_nonce: request.request_nonce,
      session_name: request.session_name,
      manifest_sha256: nativeComponentContract.manifestDigest,
      generation_id: digest('snapshot-generation'),
      inventory_sha256: digest('snapshot-inventory'),
      generation_receipt_sha256: digest('snapshot-receipt'),
      file_count: components.reduce((total: number, value: { file_count: number }) => total + value.file_count, 0) + 3,
      total_bytes: components.reduce((total: number, value: { total_bytes: number }) => total + value.total_bytes, 0) + 100,
      components
    }
    const decoded = nativeBuildProbeAuthority._internals.decodeSourceSnapshotResponse(
      Buffer.from(`${JSON.stringify(response)}\n`)
    )
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotResponse(decoded, request)).not.toThrow()
    const verified = {
      ...response,
      status: 'verified',
      operation: 'verify',
      request_nonce: digest('snapshot-verify')
    }
    expect(nativeBuildProbeAuthority._internals.sameSourceSnapshotReceipt(response, verified)).toBe(true)
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotResponse(verified, {
      ...request,
      operation: 'verify',
      request_nonce: verified.request_nonce,
      expected_generation_id: response.generation_id,
      expected_inventory_sha256: response.inventory_sha256,
      expected_generation_receipt_sha256: response.generation_receipt_sha256
    })).not.toThrow()
    const discarded = {
      kind: 'analytix_native_source_snapshot_response',
      schema_version: 1,
      status: 'discarded',
      operation: 'discard',
      request_nonce: digest('snapshot-discard'),
      session_name: request.session_name,
      disposition: 'generation_discarded',
      generation_id: response.generation_id,
      inventory_sha256: response.inventory_sha256,
      generation_receipt_sha256: response.generation_receipt_sha256,
      session_removed: true
    }
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotDiscardResponse(discarded, {
      ...request,
      operation: 'discard',
      request_nonce: discarded.request_nonce,
      expected_generation_id: response.generation_id,
      expected_inventory_sha256: response.inventory_sha256,
      expected_generation_receipt_sha256: response.generation_receipt_sha256
    })).not.toThrow()
    const reconciled = {
      ...discarded,
      operation: 'reconcile_discard',
      request_nonce: digest('snapshot-reconcile'),
      disposition: 'already_discarded',
      generation_id: '',
      inventory_sha256: '',
      generation_receipt_sha256: ''
    }
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotDiscardResponse(reconciled, {
      ...request,
      operation: 'reconcile_discard',
      request_nonce: reconciled.request_nonce
    })).not.toThrow()
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotDiscardResponse({
      ...discarded,
      inventory_sha256: digest('mismatch')
    }, {
      ...request,
      operation: 'discard',
      request_nonce: discarded.request_nonce,
      expected_generation_id: response.generation_id,
      expected_inventory_sha256: response.inventory_sha256,
      expected_generation_receipt_sha256: response.generation_receipt_sha256
    })).toThrow(/discard receipt mismatch/)
    expect(nativeBuildProbeAuthority._internals.sameSourceSnapshotReceipt(response, {
      ...verified,
      inventory_sha256: digest('mismatch')
    })).toBe(false)
    expect(() => nativeBuildProbeAuthority._internals.validateSourceSnapshotResponse({
      ...response,
      components: [components[1], components[0], ...components.slice(2)]
    }, request)).toThrow(/component receipt mismatch/)
    expect(() => nativeBuildProbeAuthority._internals.decodeSourceSnapshotResponse(
      Buffer.from(`${JSON.stringify({ ...response, unknown: true })}\n`)
    )).toThrow(/schema is invalid/)
    expect(() => nativeBuildProbeAuthority._internals.decodeSourceSnapshotResponse(
      Buffer.from(`${JSON.stringify(response)}\n{}\n`)
    )).toThrow(/channel is invalid/)
  })

  it('keeps native snapshot evidence invalid while reconciling lost create and discard responses', () => {
    const components = nativeComponentContract.manifest.components.map((component: {
      id: string
      sourceRoot: string
    }, index: number) => ({
      id: component.id,
      source_root: component.sourceRoot,
      source_digest: digest(`controller-source:${component.id}`),
      cargo_lock_sha256: digest(`controller-lock:${component.id}`),
      file_count: index + 1,
      total_bytes: (index + 1) * 10
    }))
    const generation = {
      generation_id: digest('controller-generation'),
      inventory_sha256: digest('controller-inventory'),
      generation_receipt_sha256: digest('controller-receipt')
    }
    const fullResponse = (request: Record<string, unknown>, status: string) => ({
      kind: 'analytix_native_source_snapshot_response',
      schema_version: 1,
      status,
      operation: request.operation,
      request_nonce: request.request_nonce,
      session_name: request.session_name,
      manifest_sha256: nativeComponentContract.manifestDigest,
      ...generation,
      file_count: components.reduce((total: number, value: { file_count: number }) => total + value.file_count, 0) + 3,
      total_bytes: components.reduce((total: number, value: { total_bytes: number }) => total + value.total_bytes, 0) + 100,
      components
    })
    const discardResponse = (
      request: Record<string, unknown>,
      disposition: 'generation_discarded' | 'already_discarded'
    ) => ({
      kind: 'analytix_native_source_snapshot_response',
      schema_version: 1,
      status: 'discarded',
      operation: request.operation,
      request_nonce: request.request_nonce,
      session_name: request.session_name,
      disposition,
      generation_id: disposition === 'generation_discarded' ? generation.generation_id : '',
      inventory_sha256: disposition === 'generation_discarded' ? generation.inventory_sha256 : '',
      generation_receipt_sha256: disposition === 'generation_discarded'
        ? generation.generation_receipt_sha256
        : '',
      session_removed: true
    })
    const frame = (value: unknown) => Buffer.from(`${JSON.stringify(value)}\n`)
    const ok = (stdout: Buffer) => ({ exitCode: 0, signal: 0, stdout, stderr: Buffer.alloc(0) })
    const invocations: Array<{ profile: string; request: Record<string, unknown> }> = []
    let discardAttempts = 0
    let reconcileAttempts = 0
    let closed = 0
    let nonce = 0
    const controller = nativeBuildProbeAuthority._internals.createSourceSnapshotSessionControllerV1({
      sessionName: 'analytix-native-source-session-test01',
      currentRoot: '/private/tmp/analytix-native-source-session-test01/source/current',
      sourceDescriptors: [3, 4, 5, 6],
      cleanupDescriptors: [5, 6],
      nonce: () => digest(`controller-nonce:${nonce++}`),
      closeDescriptors: () => { closed += 1 },
      invoke: (profile: string, input: Buffer) => {
        const request = JSON.parse(input.toString('utf8')) as Record<string, unknown>
        invocations.push({ profile, request })
        if (request.operation === 'create') return ok(frame(fullResponse(request, 'committed')))
        if (request.operation === 'verify') return ok(frame(fullResponse(request, 'verified')))
        if (request.operation === 'discard') {
          discardAttempts += 1
          return ok(Buffer.from('{"lost":true}\n'))
        }
        reconcileAttempts += 1
        if (reconcileAttempts === 1) return ok(Buffer.from('{"lost":true}\n'))
        return ok(frame(discardResponse(request, 'already_discarded')))
      }
    })
    expect(() => controller.close()).toThrow(/cleanup is required/)
    const created = controller.create()
    controller.verify()
    expect(() => controller.discard()).toThrow(/schema is invalid/)
    const discardRequest = invocations.find((value) => value.request.operation === 'discard')?.request
    expect(discardRequest).toMatchObject({
      expected_generation_id: created.generation_id,
      expected_inventory_sha256: created.inventory_sha256,
      expected_generation_receipt_sha256: created.generation_receipt_sha256
    })
    expect(controller.cleanup()).toMatchObject({ disposition: 'already_discarded', session_removed: true })
    controller.close()
    controller.close()
    expect(discardAttempts).toBe(1)
    expect(reconcileAttempts).toBe(2)
    expect(closed).toBe(1)
    expect(invocations.map((value) => value.profile)).toEqual([
      'source_snapshot',
      'source_snapshot',
      'source_snapshot_discard',
      'source_snapshot_discard',
      'source_snapshot_discard'
    ])
  })

  it('reconciles an unknown committed snapshot without upgrading the failed create', () => {
    let nonce = 0
    let invokeCount = 0
    let closed = 0
    const controller = nativeBuildProbeAuthority._internals.createSourceSnapshotSessionControllerV1({
      sessionName: 'analytix-native-source-session-test02',
      currentRoot: '/private/tmp/analytix-native-source-session-test02/source/current',
      sourceDescriptors: [3, 4, 5, 6],
      cleanupDescriptors: [5, 6],
      nonce: () => digest(`unknown-nonce:${nonce++}`),
      closeDescriptors: () => { closed += 1 },
      invoke: (profile: string, input: Buffer) => {
        invokeCount += 1
        const request = JSON.parse(input.toString('utf8')) as Record<string, unknown>
        if (invokeCount === 1) {
          expect(profile).toBe('source_snapshot')
          return { exitCode: 0, signal: 0, stdout: Buffer.from('{"lost":true}\n'), stderr: Buffer.alloc(0) }
        }
        expect(profile).toBe('source_snapshot_discard')
        return {
          exitCode: 0,
          signal: 0,
          stderr: Buffer.alloc(0),
          stdout: Buffer.from(`${JSON.stringify({
            kind: 'analytix_native_source_snapshot_response',
            schema_version: 1,
            status: 'discarded',
            operation: request.operation,
            request_nonce: request.request_nonce,
            session_name: request.session_name,
            disposition: 'generation_discarded',
            generation_id: digest('unknown-generation'),
            inventory_sha256: digest('unknown-inventory'),
            generation_receipt_sha256: digest('unknown-receipt'),
            session_removed: true
          })}\n`)
        }
      }
    })
    expect(() => controller.create()).toThrow(/schema is invalid/)
    expect(() => controller.verify()).toThrow(/state is invalid/)
    expect(controller.cleanup()).toMatchObject({ disposition: 'generation_discarded' })
    expect(() => controller.create()).toThrow(/state is invalid/)
    controller.close()
    expect(closed).toBe(1)
  })

  it('rejects every native execution and publication authority downgrade in Node and runtime readers', () => {
    const root = tempRoot()
    const context = { ...createMacPackContext(root), arch: 'x64' }
    writeNativeComponentBundle(context)
    const runtimeDir = join(afterPack._internals.packedResourcesDir(context), 'runtime')
    const receiptPath = join(runtimeDir, nativeComponentContract.RECEIPT_FILE_NAME)
    const original = JSON.parse(readFileSync(receiptPath, 'utf8'))
    const target = nativeComponentContract.targetContract('darwin', 'x64')
    expect(() => verifyRuntimeNativeComponent({
      root: afterPack._internals.packedResourcesDir(context),
      packaged: true,
      platform: 'darwin',
      arch: 'x64',
      componentId: 'data-engine'
    })).toThrow(/owned by the Go runtime and release authority is unavailable/)
    const mutations: Array<{ name: string; path: string[]; value: unknown }> = [
      { name: 'old receipt schema', path: ['schemaVersion'], value: 5 },
      { name: 'missing root field', path: ['manifestSha256'], value: undefined },
      { name: 'unknown root field', path: ['unknown'], value: true },
      { name: 'authority schema', path: ['executionAuthority', 'schemaVersion'], value: 2 },
      { name: 'unrecognized trust class', path: ['executionAuthority', 'trustClass'], value: 'release' },
      { name: 'authority protocol', path: ['executionAuthority', 'protocol'], value: 'unknown' },
      { name: 'authority target', path: ['executionAuthority', 'targetKey'], value: 'linux-x64' },
      { name: 'authority digest mismatch', path: ['executionAuthority', 'binarySha256'], value: '0'.repeat(64) },
      { name: 'authority size', path: ['executionAuthority', 'binarySize'], value: 0 },
      { name: 'authority source set', path: ['executionAuthority', 'sourceSetSha256'], value: '0' },
      { name: 'authority environment', path: ['executionAuthority', 'buildEnvironmentSha256'], value: '0' },
      { name: 'authority toolchain mismatch', path: ['executionAuthority', 'goToolchainKey'], value: 'linux-x64' },
      { name: 'authority Go executable', path: ['executionAuthority', 'goExecutableSha256'], value: '0' },
      { name: 'publication schema', path: ['publicationAuthority', 'schemaVersion'], value: 2 },
      { name: 'missing publication authority', path: ['publicationAuthority'], value: undefined },
      { name: 'publication trust class', path: ['publicationAuthority', 'trustClass'], value: 'local' },
      { name: 'publication protocol', path: ['publicationAuthority', 'protocol'], value: 'unknown' },
      { name: 'publication target', path: ['publicationAuthority', 'targetKey'], value: 'linux-x64' },
      { name: 'publication execution id', path: ['publicationAuthority', 'cargoExecutionId'], value: '0' },
      { name: 'publication execution receipt', path: ['publicationAuthority', 'cargoExecutionReceiptSha256'], value: '0' },
      { name: 'publication binding', path: ['publicationAuthority', 'publicationBindingSha256'], value: '0' },
      { name: 'raw build digest', path: ['components', '0', 'rawBuildSha256'], value: '0' },
      { name: 'missing component field', path: ['components', '0', 'rawBuildSha256'], value: undefined },
      { name: 'unknown component field', path: ['components', '0', 'unknown'], value: true },
      { name: 'raw build size', path: ['components', '0', 'rawBuildSize'], value: 0 },
      { name: 'staged image digest', path: ['components', '0', 'stagedImageSha256'], value: '0' },
      { name: 'staged image size', path: ['components', '0', 'stagedImageSize'], value: 0 },
      { name: 'probe kind', path: ['components', '0', 'executionProbe', 'kind'], value: 'unknown' },
      { name: 'probe schema', path: ['components', '0', 'executionProbe', 'schema_version'], value: 2 },
      { name: 'failed probe', path: ['components', '0', 'executionProbe', 'status'], value: 'failed' },
      { name: 'wrong component', path: ['components', '0', 'executionProbe', 'component_id'], value: 'cleaning-ops' },
      { name: 'invalid challenge', path: ['components', '0', 'executionProbe', 'request_nonce'], value: '0' },
      { name: 'wrong executable digest', path: ['components', '0', 'executionProbe', 'executable_sha256'], value: '0'.repeat(64) },
      { name: 'wrong executable size', path: ['components', '0', 'executionProbe', 'executable_size'], value: 1 },
      { name: 'wrong manifest', path: ['components', '0', 'executionProbe', 'manifest_sha256'], value: '0'.repeat(64) },
      { name: 'wrong policy', path: ['components', '0', 'executionProbe', 'policy_sha256'], value: '0'.repeat(64) },
      { name: 'wrong authority', path: ['components', '0', 'executionProbe', 'authority_sha256'], value: '0'.repeat(64) },
      { name: 'wrong host architecture', path: ['components', '0', 'executionProbe', 'host_arch'], value: 'arm64' },
      { name: 'wrong host platform', path: ['components', '0', 'executionProbe', 'host_platform'], value: 'linux' },
      { name: 'loaded image unbound', path: ['components', '0', 'executionProbe', 'loaded_image_bound'], value: false },
      { name: 'working directory unbound', path: ['components', '0', 'executionProbe', 'working_directory_bound'], value: false },
      { name: 'guardian unauthenticated', path: ['components', '0', 'executionProbe', 'guardian_authenticated'], value: false },
      { name: 'process tree not empty', path: ['components', '0', 'executionProbe', 'process_tree_empty'], value: false },
      { name: 'unknown probe field', path: ['components', '0', 'executionProbe', 'unknown'], value: true },
      {
        name: 'cross-component replay',
        path: ['components', '0', 'executionProbe'],
        value: original.components[1].executionProbe
      }
    ]

    for (const mutation of mutations) {
      const receipt = JSON.parse(JSON.stringify(original))
      setNestedReceiptValue(receipt, mutation.path, mutation.value)
      const encoded = `${JSON.stringify(receipt, null, 2)}\n`
      writeFileSync(receiptPath, encoded, 'utf8')
      expect(
        () => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
          verifyExecution: false,
          authorityUse: 'release'
        }),
        mutation.name
      ).toThrow()
    }

    const crossTargetReplay = JSON.parse(JSON.stringify(original))
    crossTargetReplay.executionAuthority.targetKey = 'darwin-arm64'
    crossTargetReplay.executionAuthority.goToolchainKey = 'darwin-arm64'
    for (const component of crossTargetReplay.components) {
      component.executionProbe.host_arch = 'arm64'
    }
    writeFileSync(receiptPath, `${JSON.stringify(crossTargetReplay, null, 2)}\n`, 'utf8')
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
      verifyExecution: false,
      authorityUse: 'release'
    })).toThrow(/execution authority receipt is invalid/)

    writeFileSync(receiptPath, `${JSON.stringify(original, null, 2)}\n`, 'utf8')
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
      verifyExecution: false,
      authorityUse: 'local-build'
    })).toThrow(/publication authority receipt is invalid/)
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
      verifyExecution: false
    })).not.toThrow()
    const exactDevelopmentEnvironment = {
      ...process.env,
      ANALYTIX_DEV_CACHE_ROOT: developmentCacheEnvironment.DEVELOPMENT_CACHE_ROOT,
      ...developmentCacheEnvironment.DEVELOPMENT_CACHE_ENVIRONMENT
    }
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
      env: exactDevelopmentEnvironment,
      verifyExecution: false
    })).not.toThrow()
    expect(() => nativeComponentContract.verifyPackagedComponents(runtimeDir, target, {
      env: {
        ...exactDevelopmentEnvironment,
        CARGO_TARGET_DIR: '/tmp/untrusted-cargo-target'
      },
      verifyExecution: false
    })).toThrow(/Development cache path mismatch: CARGO_TARGET_DIR/)
    expect(() => afterPack._internals.verifyPackagedNativeBeforeRuntimeBuild(context, {
      verifyNativeExecution: false
    })).not.toThrow()
  })

  it('rejects non-canonical native receipts in Node and runtime readers', () => {
    const root = tempRoot()
    const context = { ...createMacPackContext(root), arch: 'x64' }
    writeNativeComponentBundle(context)
    const runtimeDir = join(afterPack._internals.packedResourcesDir(context), 'runtime')
    const receiptPath = join(runtimeDir, nativeComponentContract.RECEIPT_FILE_NAME)
    const receipt = JSON.parse(readFileSync(receiptPath, 'utf8'))
    writeFileSync(receiptPath, JSON.stringify(receipt), 'utf8')
    expect(() => nativeComponentContract.verifyPackagedComponents(
      runtimeDir,
      nativeComponentContract.targetContract('darwin', 'x64'),
      { verifyExecution: false, authorityUse: 'release' }
    )).toThrow(/canonical duplicate-free JSON/)

    writeNativeComponentBundle(context)
    const canonical = readFileSync(receiptPath, 'utf8')
    writeFileSync(
      receiptPath,
      canonical.replace(
        '  "schemaVersion": 6,',
        '  "schemaVersion": 6,\n  "schemaVersion": 6,'
      ),
      'utf8'
    )
    expect(() => nativeComponentContract.verifyPackagedComponents(
      runtimeDir,
      nativeComponentContract.targetContract('darwin', 'x64'),
      { verifyExecution: false, authorityUse: 'release' }
    )).toThrow(/duplicate key/)

    writeFileSync(
      receiptPath,
      canonical.replace(
        '  "publicationAuthority": {\n    "schemaVersion": 1,',
        '  "publicationAuthority": {\n    "schemaVersion": 1,\n    "schemaVersion": 1,'
      ),
      'utf8'
    )
    expect(() => nativeComponentContract.verifyPackagedComponents(
      runtimeDir,
      nativeComponentContract.targetContract('darwin', 'x64'),
      { verifyExecution: false, authorityUse: 'release' }
    )).toThrow(/duplicate key/)

    expect(() => verifyRuntimeNativeComponent({
      root: afterPack._internals.packedResourcesDir(context),
      packaged: false,
      platform: 'darwin',
      arch: 'x64',
      componentId: 'data-engine'
    })).toThrow(/owned by the Go runtime and release authority is unavailable/)
  })

  it('validates Windows runtime-server, Analytix Computer Use, and node-pty prebuilds in the package', () => {
    const root = tempRoot()
    const context = createWindowsPackContext(root)
    const unpackedRoot = afterPack._internals.unpackedAppRoot(context)
    const validationOptions = {
      verifyNativeExecution: false,
      verifyPythonAuthority: false,
      authorityUse: 'release',
      testOnlyNativeComponentVerifier: () => undefined
    }

    for (const relativePath of afterPack.ANALYTIX_RUNTIME_REQUIRED_PATHS) {
      touch(join(unpackedRoot, relativePath))
    }
    touch(afterPack._internals.bundledGoRuntimeServerPath(context))
    writeNativeComponentBundle(context)
    touchWindowsBackendRuntime(context)
    touchManagedChrome(context)
    touch(afterPack._internals.bundledOpenComputerUseNativePath(context))
    touch(join(unpackedRoot, 'node_modules/better-sqlite3/package.json'))
    touch(join(unpackedRoot, 'packages/runtime/node_modules/.bin/analytix-computer-use.cmd'))
    touch(join(unpackedRoot, 'packages/runtime/node_modules/.bin/open-computer-use.cmd'))
    for (const relativePath of afterPack._internals.nodePtyRequiredPaths(context)) {
      touch(join(unpackedRoot, relativePath))
    }

    expect(afterPack._internals.bundledGoRuntimeServerPath(context)).toMatch(/runtime-server\.exe$/)
    expect(afterPack._internals.dataAnalysisNativeBinaryName(context, 'analytix-import-accelerator')).toBe(
      'analytix-import-accelerator.exe'
    )
    expect(afterPack._internals.bundledDataAnalysisNativeToolPath(context, 'analytix-cleaning-ops')).toMatch(
      /analytix-cleaning-ops\.exe$/
    )
    expect(afterPack._internals.bundledDataAnalysisNativeToolPath(context, 'analytix-analysis-compute')).toMatch(
      /analytix-analysis-compute\.exe$/
    )
    expect(afterPack._internals.bundledWindowsBackendPythonExecutablePath(context)).toMatch(
      /resources[/\\]\.python-runtime[/\\]current[/\\]python[/\\]python\.exe$/
    )
    expect(afterPack._internals.bundledOpenComputerUseNativePath(context)).toMatch(/analytix-computer-use\.exe$/)
    expect(afterPack._internals.nodePtyRequiredPaths(context)).toEqual(expect.arrayContaining([
      'node_modules/node-pty/prebuilds/win32-x64/pty.node',
      'node_modules/node-pty/prebuilds/win32-x64/conpty.node',
      'node_modules/node-pty/prebuilds/win32-x64/conpty/conpty.dll',
      'node_modules/node-pty/prebuilds/win32-x64/winpty.dll',
      'node_modules/node-pty/prebuilds/win32-x64/winpty-agent.exe'
    ]))
    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, {
      verifyNativeExecution: false,
      verifyPythonAuthority: false,
      authorityUse: 'release'
    })).toThrow(/execution authority is unavailable for target win32-x64/)
    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).not.toThrow()

    rmSync(afterPack._internals.bundledWindowsBackendPythonExecutablePath(context), { force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /Windows data analysis Python runtime/
    )

    touch(afterPack._internals.bundledWindowsBackendPythonExecutablePath(context))
    rmSync(afterPack._internals.bundledDataAnalysisNativeToolPath(context, 'analytix-import-accelerator'), { force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /data analysis native tool \(analytix-import-accelerator\)/
    )

    writeNativeComponentBundle(context)
    rmSync(join(unpackedRoot, 'node_modules/node-pty/prebuilds/win32-x64/pty.node'), { force: true })

    expect(() => afterPack._internals.validateBundledAnalytixRuntime(context, validationOptions)).toThrow(
      /node_modules\/node-pty\/prebuilds\/win32-x64\/pty\.node/
    )
  })

  it('signs the afterPack Windows runtime-server with the electron-builder signing queue', async () => {
    const signIf = vi.fn(async () => true)
    const output = join(tempRoot(), 'runtime-server.exe')
    const context = {
      electronPlatformName: 'win32',
      packager: { signIf }
    }
    await expect(afterPack._internals.signBundledWindowsRuntime(context, output)).resolves.toBe(true)
    expect(signIf).toHaveBeenCalledTimes(1)
    expect(signIf).toHaveBeenCalledWith(output)

    await expect(afterPack._internals.signBundledWindowsRuntime({
      electronPlatformName: 'win32',
      packager: { signIf: vi.fn(async () => false) }
    }, output)).rejects.toThrow(/was not signed/)
    await expect(afterPack._internals.signBundledWindowsRuntime({
      electronPlatformName: 'win32',
      packager: {}
    }, output)).rejects.toThrow(/signer is unavailable/)
  })

  it('creates Windows npm shims for packaged Analytix Computer Use', () => {
    const root = tempRoot()
    const context = createWindowsPackContext(root)
    const unpackedRoot = afterPack._internals.unpackedAppRoot(context)

    afterPack._internals.ensureBundledOpenComputerUseWindowsShims(context)

    const shim = join(unpackedRoot, 'packages/runtime/node_modules/.bin/analytix-computer-use.cmd')
    const legacyShim = join(unpackedRoot, 'packages/runtime/node_modules/.bin/open-computer-use.cmd')
    expect(existsSync(shim)).toBe(true)
    expect(existsSync(legacyShim)).toBe(true)
    expect(readFileSync(shim, 'utf8')).toContain('..\\analytix-computer-use\\bin\\analytix-computer-use')
    expect(readFileSync(legacyShim, 'utf8')).toContain('..\\analytix-computer-use\\bin\\open-computer-use')
  })

  it('builds a production-tagged Go runtime server binary for the packaged target', () => {
    const root = tempRoot()
    const context = {
      ...createMacPackContext(root),
      arch: 'arm64'
    }
    const calls: Array<{ command: string; args: string[]; options: { cwd: string; env: NodeJS.ProcessEnv } }> = []
    const chmods: Array<{ path: string; mode: number }> = []
    const moduleCache = join(root, 'go-module-cache')
    mkdirSync(moduleCache, { recursive: true })
    const nativeTrust = {
      receiptSHA256: 'a'.repeat(64),
      manifestSHA256: 'b'.repeat(64),
      targetKey: 'darwin-arm64',
      signingPolicySHA256: 'c'.repeat(64),
      signingMode: 'ad-hoc',
      appleTeamIdentifier: ''
    }

    const result = afterPack._internals.buildBundledGoRuntimeServer(context, nativeTrust, {
      toolchain: {
        key: 'darwin-arm64',
        executable: '/trusted/go/bin/go',
        moduleCache
      },
      execFileSync(command: string, args: string[], options: { cwd: string; env: NodeJS.ProcessEnv }) {
        calls.push({ command, args, options })
        const outputIndex = args.indexOf('-o')
        if (outputIndex >= 0) writeFileSync(args[outputIndex + 1]!, 'runtime-server', { mode: 0o755 })
      },
      chmodSync(path: string, mode: number) {
        chmods.push({ path, mode })
      }
    })

    const output = afterPack._internals.bundledGoRuntimeServerPath(context)
    expect(result).toEqual({
      output, goos: 'darwin', goarch: 'arm64', toolchainKey: 'darwin-arm64', nativeTrust
    })
    expect(calls).toHaveLength(2)
    expect(calls[0]?.command).toBe('/trusted/go/bin/go')
    expect(calls[0]?.args).toEqual(['mod', 'verify'])
    expect(calls[1]?.command).toBe('/trusted/go/bin/go')
    expect(calls[1]?.args).toEqual([
      'build',
      '-mod=readonly',
      '-buildvcs=false',
      '-trimpath',
      '-tags',
      'analytix_prod',
      '-ldflags',
      [
        `-X analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs.embeddedReleaseProfile=full`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedReceiptSHA256=${nativeTrust.receiptSHA256}`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedManifestSHA256=${nativeTrust.manifestSHA256}`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedTargetKey=${nativeTrust.targetKey}`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningPolicySHA256=${nativeTrust.signingPolicySHA256}`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedSigningMode=${nativeTrust.signingMode}`,
        `-X analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry.embeddedAppleTeamIdentifier=${nativeTrust.appleTeamIdentifier}`
      ].join(' '),
      '-o',
      expect.stringContaining('.runtime-server-build-'),
      './cmd/runtime-server'
    ])
    expect(calls[1]?.options.cwd.split(/[\\/]/).slice(-2)).toEqual(['packages', 'runtime-go'])
    expect(calls[1]?.options.env).toEqual(calls[0]?.options.env)
    expect(calls[1]?.options.env.GOOS).toBe('darwin')
    expect(calls[1]?.options.env.GOARCH).toBe('arm64')
    expect(calls[1]?.options.env.CGO_ENABLED).toBe('0')
    expect(JSON.stringify(calls[1]?.options.env)).not.toContain('CSC_')
    expect(chmods).toEqual([{ path: output, mode: 0o755 }])
  })

  it('validates the bundled macOS Computer Use helper as Analytix Computer Use', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const helperApp = afterPack._internals.bundledOpenComputerUseMacAppPath(context)
    const infoPlist = join(helperApp, 'Contents/Info.plist')
    const executable = join(helperApp, 'Contents/MacOS/OpenComputerUse')
    const icon = join(helperApp, 'Contents/Resources/AnalytixComputerUse.icns')
    touch(infoPlist)
    touch(executable)
    touch(icon)
    const calls: Array<{ command: string; args: string[] }> = []

    afterPack._internals.validateBundledOpenComputerUseMacApp(context, {
      execFileSync(command: string, args: string[]) {
        calls.push({ command, args })
        const key = args[1]
        if (key === 'CFBundleName') return 'Analytix Computer Use\n'
        if (key === 'CFBundleDisplayName') return 'Analytix Computer Use\n'
        if (key === 'CFBundleIdentifier') return 'com.analytix.computer-use\n'
        if (key === 'CFBundleIconFile') return 'AnalytixComputerUse.icns\n'
        return ''
      }
    })

    expect(calls).toEqual([
      { command: 'plutil', args: ['-extract', 'CFBundleName', 'raw', infoPlist] },
      { command: 'plutil', args: ['-extract', 'CFBundleDisplayName', 'raw', infoPlist] },
      { command: 'plutil', args: ['-extract', 'CFBundleIdentifier', 'raw', infoPlist] },
      { command: 'plutil', args: ['-extract', 'CFBundleIconFile', 'raw', infoPlist] }
    ])
  })

  it('restores file-linked Analytix Computer Use into the packaged runtime', () => {
    const root = tempRoot()
    const context = createMacPackContext(root)
    const sourceRoot = join(root, 'local-analytix-computer-use')
    const destination = afterPack._internals.bundledOpenComputerUsePackageRoot(context)

    touch(join(sourceRoot, 'package.json'))
    touch(join(sourceRoot, 'bin/analytix-computer-use'))
    touch(join(sourceRoot, 'dist/Analytix Computer Use.app/Contents/MacOS/OpenComputerUse'))

    expect(existsSync(join(destination, 'package.json'))).toBe(false)

    const result = afterPack._internals.ensureBundledOpenComputerUsePackage(context, {
      sourceRoot
    })

    expect(result).toEqual(expect.objectContaining({ copied: true, destination }))
    expect(existsSync(join(destination, 'package.json'))).toBe(true)
    expect(existsSync(join(destination, 'bin/analytix-computer-use'))).toBe(true)
    expect(existsSync(afterPack._internals.bundledOpenComputerUseNativePath(context))).toBe(true)
  })

  it('prunes build-only and non-target native payload from the Windows package', () => {
    const root = tempRoot()
    const context = createWindowsPackContext(root)
    const unpackedRoot = afterPack._internals.unpackedAppRoot(context)
    const runtimeComputerUseRoot = afterPack._internals.bundledOpenComputerUsePackageRoot(context)
    const rootComputerUseRoot = join(unpackedRoot, 'node_modules/analytix-computer-use')

    touch(join(unpackedRoot, 'node_modules/node-pty/prebuilds/win32-x64/pty.node'))
    touch(join(unpackedRoot, 'node_modules/node-pty/prebuilds/win32-arm64/pty.node'))
    touch(join(unpackedRoot, 'node_modules/node-pty/build/Release/pty.iobj'))
    touch(join(unpackedRoot, 'node_modules/node-pty/third_party/conpty/OpenConsole.exe'))
    touch(join(unpackedRoot, 'node_modules/better-sqlite3/build/Release/better_sqlite3.node'))
    touch(join(unpackedRoot, 'node_modules/better-sqlite3/deps/sqlite3/sqlite3.c'))
    touch(join(unpackedRoot, 'node_modules/@napi-rs/canvas-win32-x64-msvc/skia.win32-x64-msvc.node'))
    touch(join(unpackedRoot, 'node_modules/@napi-rs/canvas-darwin-arm64/skia.darwin-arm64.node'))
    touch(join(unpackedRoot, 'node_modules/@jimp/png/test/images/sample.png'))
    touch(join(unpackedRoot, 'node_modules/@jimp/png/examples/lenna.png'))
    touch(join(unpackedRoot, 'node_modules/await-to-js/dist/docs/index.html'))
    touch(join(unpackedRoot, 'node_modules/fast-uri/benchmark/benchmark.mjs'))
    touch(join(unpackedRoot, 'node_modules/gifwrap/test/fixtures/sample.gif'))
    touch(join(unpackedRoot, 'node_modules/zod/index.d.ts'))
    touch(join(unpackedRoot, 'node_modules/zod/README.md'))
    touch(join(unpackedRoot, 'node_modules/bytes/Readme.md'))
    touch(join(unpackedRoot, 'node_modules/zod/tsconfig.json'))
    touch(join(unpackedRoot, 'node_modules/zod/._package.json'))
    touch(join(root, 'win-unpacked/resources/backend/app/main.py'))
    touch(join(root, 'win-unpacked/resources/backend/.pytest_cache/CACHEDIR.TAG'))
    touch(join(root, 'win-unpacked/resources/.python-runtime/current/python/Lib/idlelib/README.txt'))
    touch(join(root, 'win-unpacked/resources/.python-runtime/current/python/Lib/test/test_sample.py'))
    touch(join(root, 'win-unpacked/resources/python-site-packages/pandas/tests/test_sample.py'))
    touch(join(root, 'win-unpacked/resources/python-site-packages/pip/__pycache__/main.pyc'))
    for (const tool of afterPack.ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS) {
      touch(join(root, 'win-unpacked/resources/runtime', tool))
      touch(join(root, 'win-unpacked/resources/runtime', `${tool}.exe`))
    }
    mkdirSync(join(unpackedRoot, 'packages/runtime/node_modules'), { recursive: true })
    symlinkSync(
      join(unpackedRoot, 'packages'),
      join(unpackedRoot, 'packages/runtime/node_modules/analytix'),
      'junction'
    )

    touch(join(runtimeComputerUseRoot, 'package.json'))
    touch(join(runtimeComputerUseRoot, 'bin/analytix-computer-use'))
    touch(join(runtimeComputerUseRoot, 'dist/windows/amd64/analytix-computer-use.exe'))
    touch(join(runtimeComputerUseRoot, 'dist/windows/arm64/analytix-computer-use.exe'))
    touch(join(runtimeComputerUseRoot, 'dist/linux/amd64/analytix-computer-use'))
    touch(join(runtimeComputerUseRoot, 'dist/Analytix Computer Use.app/Contents/MacOS/OpenComputerUse'))
    touch(join(rootComputerUseRoot, 'package.json'))
    touch(join(rootComputerUseRoot, 'dist/windows/amd64/analytix-computer-use.exe'))

    afterPack._internals.prunePackedProductionArtifacts(context)

    expect(existsSync(join(unpackedRoot, 'node_modules/node-pty/prebuilds/win32-x64/pty.node'))).toBe(true)
    expect(existsSync(join(unpackedRoot, 'node_modules/node-pty/prebuilds/win32-arm64/pty.node'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/node-pty/build'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/node-pty/third_party'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/better-sqlite3/build/Release/better_sqlite3.node'))).toBe(true)
    expect(existsSync(join(unpackedRoot, 'node_modules/better-sqlite3/deps'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/@napi-rs/canvas-win32-x64-msvc/skia.win32-x64-msvc.node'))).toBe(true)
    expect(existsSync(join(unpackedRoot, 'node_modules/@napi-rs/canvas-darwin-arm64'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/@jimp/png/test'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/@jimp/png/examples'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/await-to-js/dist/docs'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/fast-uri/benchmark'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/gifwrap/test'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/zod/index.d.ts'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/zod/README.md'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/bytes/Readme.md'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/zod/tsconfig.json'))).toBe(false)
    expect(existsSync(join(unpackedRoot, 'node_modules/zod/._package.json'))).toBe(false)
    expect(existsSync(join(root, 'win-unpacked/resources/backend/app/main.py'))).toBe(true)
    expect(existsSync(join(root, 'win-unpacked/resources/backend/.pytest_cache'))).toBe(false)
    expect(existsSync(join(root, 'win-unpacked/resources/.python-runtime/current/python/Lib/idlelib/README.txt'))).toBe(false)
    expect(existsSync(join(root, 'win-unpacked/resources/.python-runtime/current/python/Lib/test'))).toBe(false)
    expect(existsSync(join(root, 'win-unpacked/resources/python-site-packages/pandas/tests'))).toBe(false)
    expect(existsSync(join(root, 'win-unpacked/resources/python-site-packages/pip/__pycache__'))).toBe(false)
    for (const tool of afterPack.ANALYTIX_DATA_NATIVE_REQUIRED_TOOLS) {
      expect(existsSync(join(root, 'win-unpacked/resources/runtime', tool))).toBe(false)
      expect(existsSync(join(root, 'win-unpacked/resources/runtime', `${tool}.exe`))).toBe(true)
    }
    expect(existsSync(join(unpackedRoot, 'packages/runtime/node_modules/analytix'))).toBe(false)
    expect(existsSync(join(runtimeComputerUseRoot, 'dist/windows/amd64/analytix-computer-use.exe'))).toBe(true)
    expect(existsSync(join(runtimeComputerUseRoot, 'dist/windows/arm64'))).toBe(false)
    expect(existsSync(join(runtimeComputerUseRoot, 'dist/linux'))).toBe(false)
    expect(existsSync(join(runtimeComputerUseRoot, 'dist/Analytix Computer Use.app'))).toBe(false)
    expect(existsSync(rootComputerUseRoot)).toBe(false)
  })

  it('keeps only target-platform conditional native dependencies in both packaged module roots', () => {
    const cases = [
      {
        context: createMacPackContext(tempRoot()),
        targetLibnut: 'libnut-darwin',
        targetClipboard: ''
      },
      {
        context: createWindowsPackContext(tempRoot()),
        targetLibnut: 'libnut-win32',
        targetClipboard: 'fallbacks/windows/clipboard_x86_64.exe'
      },
      {
        context: {
          appOutDir: join(tempRoot(), 'linux-unpacked'),
          electronPlatformName: 'linux',
          arch: 'x64',
          packager: { appInfo: { productFilename: 'analytix' } }
        },
        targetLibnut: 'libnut-linux',
        targetClipboard: 'fallbacks/linux/xsel'
      }
    ]

    for (const { context, targetLibnut, targetClipboard } of cases) {
      const unpackedRoot = afterPack._internals.unpackedAppRoot(context)
      const moduleRoots = [
        join(unpackedRoot, 'node_modules'),
        join(unpackedRoot, 'packages/runtime/node_modules')
      ]
      for (const moduleRoot of moduleRoots) {
        const computerUseRoot = join(moduleRoot, '@computer-use')
        touch(join(computerUseRoot, 'libnut/package.json'))
        touch(join(computerUseRoot, 'node-mac-permissions/build/Release/permissions.node'))
        for (const packageName of ['libnut-darwin', 'libnut-linux', 'libnut-win32']) {
          touch(join(computerUseRoot, packageName, 'package.json'))
          touch(join(computerUseRoot, packageName, 'build/Release/libnut.node'))
          touch(join(
            computerUseRoot,
            packageName,
            'node_modules/@computer-use/node-mac-permissions/build/Release/permissions.node'
          ))
        }
        touch(join(moduleRoot, 'clipboardy/package.json'))
        touch(join(moduleRoot, 'clipboardy/fallbacks/linux/xsel'))
        touch(join(moduleRoot, 'clipboardy/fallbacks/windows/clipboard_i686.exe'))
        touch(join(moduleRoot, 'clipboardy/fallbacks/windows/clipboard_x86_64.exe'))
      }

      afterPack._internals.pruneConditionalNativeDependenciesForTarget(context)

      for (const moduleRoot of moduleRoots) {
        const computerUseRoot = join(moduleRoot, '@computer-use')
        for (const packageName of ['libnut-darwin', 'libnut-linux', 'libnut-win32']) {
          expect(existsSync(join(computerUseRoot, packageName, 'build/Release/libnut.node')))
            .toBe(packageName === targetLibnut)
        }
        expect(existsSync(join(moduleRoot, 'clipboardy', targetClipboard || 'fallbacks')))
          .toBe(Boolean(targetClipboard))
        expect(existsSync(join(moduleRoot, 'clipboardy/fallbacks/windows/clipboard_i686.exe')))
          .toBe(false)
      }
    }

    const missingTargetContext = createWindowsPackContext(tempRoot())
    const missingTargetModules = join(
      afterPack._internals.unpackedAppRoot(missingTargetContext),
      'node_modules'
    )
    touch(join(missingTargetModules, 'clipboardy/package.json'))
    touch(join(missingTargetModules, 'clipboardy/fallbacks/windows/clipboard_i686.exe'))
    expect(() => afterPack._internals.pruneConditionalNativeDependenciesForTarget(
      missingTargetContext
    )).toThrow(/target clipboardy fallback/)
  })

  it('runs npm through cmd.exe during Windows afterPack hooks', () => {
    expect(afterPack._internals.npmCommand(['prune'], 'win32')).toEqual({
      command: 'cmd.exe',
      args: ['/d', '/s', '/c', 'npm', 'prune']
    })
    expect(afterPack._internals.npmCommand(['prune'], 'darwin')).toEqual({
      command: 'npm',
      args: ['prune']
    })
  })

  it('uses generated Analytix icons for app packages and Windows shortcuts', () => {
    expect(builderConfig.mac.icon).toBe('./build/icon.icns')
    expect(builderConfig.win.icon).toBe('./build/icon.ico')
    expect(builderConfig.linux.icon).toBe('./src/asset/brand/analytix-app-icon-512.png')
  })

  it('always signs after fuse mutation and blocks an unconfigured Developer ID release', () => {
    expect(builderConfig.mac.identity).toBe('-')
    expect(builderConfig.mac.hardenedRuntime).toBe(true)
    expect(builderConfig.mac.forceCodeSigning).toBe(true)
    expect(builderConfig.mac.preAutoEntitlements).toBe(false)
    expect(builderConfig.mac.sign).toBe('./scripts/mac-sign.cjs')
    expect(builderConfig.mac.timestamp).toBeNull()

    expect(() => loadBuilderConfigWithEnv({ MAC_SIGN: '1' })).toThrow(
      /Official Apple Team ID is not configured/
    )
  })

  it('rejects partial or ambiguous Apple notary credentials', () => {
    const empty = {
      APPLE_API_KEY_ID: undefined,
      APPLE_API_ISSUER: undefined,
      APPLE_API_KEY: undefined,
      APPLE_API_KEY_BASE64: undefined
    }
    expect(withProcessEnv(empty, () => macNotarize._internals.getNotaryCredentials())).toBeNull()
    expect(() => withProcessEnv({
      ...empty,
      APPLE_API_KEY_ID: 'KEY-ID'
    }, () => macNotarize._internals.getNotaryCredentials())).toThrow(/exactly one key source/)
    expect(() => withProcessEnv({
      APPLE_API_KEY_ID: 'KEY-ID',
      APPLE_API_ISSUER: 'ISSUER',
      APPLE_API_KEY: '/private/key.p8',
      APPLE_API_KEY_BASE64: Buffer.from('second-key').toString('base64')
    }, () => macNotarize._internals.getNotaryCredentials())).toThrow(/exactly one key source/)
  })

  it('checks timestamp candidates across nested macOS signed code', () => {
    const root = tempRoot()
    const appBundle = join(root, 'Analytix.app')
    const mainExecutable = join(appBundle, 'Contents/MacOS/Analytix')
    const framework = join(appBundle, 'Contents/Frameworks/Electron Framework.framework')
    const nativeAddon = join(
      appBundle,
      'Contents/Resources/app.asar.unpacked/node_modules/better-sqlite3/build/Release/better_sqlite3.node'
    )
    const resourceScript = join(appBundle, 'Contents/Resources/postinstall.sh')
    const dataNativeHelper = join(
      appBundle,
      'Contents/Resources/runtime/analytix-import-accelerator'
    )
    const runtimeServer = join(
      appBundle,
      'Contents/Resources/runtime-go/bin/runtime-server'
    )

    touch(mainExecutable)
    touch(join(framework, 'Versions/A/Electron Framework'))
    touch(nativeAddon)
    touch(resourceScript)
    touch(dataNativeHelper)
    touch(runtimeServer)
    chmodSync(mainExecutable, 0o755)
    chmodSync(resourceScript, 0o755)
    chmodSync(dataNativeHelper, 0o755)
    chmodSync(runtimeServer, 0o755)

    const expectedCandidates =
      process.platform === 'win32'
        ? [appBundle, framework, nativeAddon]
        : [appBundle, framework, mainExecutable, nativeAddon, runtimeServer, dataNativeHelper]

    expect(macNotarize._internals.collectSignedCodeCandidates(appBundle)).toEqual(expectedCandidates)
    expect(macNotarize._internals.normalizeBuilderArch(1)).toBe('x64')
    expect(macNotarize._internals.normalizeBuilderArch(3)).toBe('arm64')
  })
})
