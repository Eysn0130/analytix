import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, cpSync, existsSync, linkSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { delimiter, join, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const require = createRequire(import.meta.url)
const {
  NATIVE_DEVELOPMENT_BUILD_MARKER,
  NATIVE_DISPOSITION_DEVELOPMENT,
  ANALYTIX_FUNDS_PRODUCTION_MCP_FILES,
  PACKAGED_BUILD_AUTHORITY_CONTRACT,
  PACKAGED_BUILD_AUTHORITY_FILE,
  _internals: packagedAuthorityContract
} = require('../../scripts/after-pack.cjs') as {
  NATIVE_DEVELOPMENT_BUILD_MARKER: string
  NATIVE_DISPOSITION_DEVELOPMENT: string
  ANALYTIX_FUNDS_PRODUCTION_MCP_FILES: string[]
  PACKAGED_BUILD_AUTHORITY_CONTRACT: string
  PACKAGED_BUILD_AUTHORITY_FILE: string
  _internals: Record<string, (...args: any[]) => any>
}
const macNotarize = require('../../scripts/mac-notarize.cjs') as {
  _internals: Record<string, (...args: any[]) => any>
}

const DEVELOPMENT_NATIVE_COMPONENTS = [
  ['import-accelerator', 'analytix-import-accelerator'],
  ['cleaning-ops', 'analytix-cleaning-ops'],
  ['analysis-compute', 'analytix-analysis-compute'],
  ['data-engine', 'analytix-data-engine']
] as const

const ACTUAL_PACKAGED_ENV_KEYS = [
  'ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE',
  'ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK',
  'ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH',
  'ANALYTIX_RUNTIME_GO_GUI_PACKAGED_APP_PATH',
  'ANALYTIX_PACKAGED_APP_PATH',
  'ANALYTIX_RUNTIME_GO_FORMAL_INSTANCE',
  'ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT',
  'ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_ARCH',
  'ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_PLATFORM'
]

const FINAL_EVIDENCE_ENV_KEYS = [
  'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
  'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS',
  'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_READY',
  'ANALYTIX_RUNTIME_READY_EVIDENCE',
  'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT',
  'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE'
]

function reportEnv(extraEnv: NodeJS.ProcessEnv = {}): NodeJS.ProcessEnv {
  const env = { ...process.env, ...extraEnv }
  for (const key of [...ACTUAL_PACKAGED_ENV_KEYS, ...FINAL_EVIDENCE_ENV_KEYS]) {
    if (!Object.hasOwn(extraEnv, key)) delete env[key]
  }
  return env
}

function runReport(
  script: string,
  extraArgs: string[] = [],
  extraEnv: NodeJS.ProcessEnv = {}
): Record<string, unknown> {
  const result = spawnSync(process.execPath, [script, '--dry-run', '--json', ...extraArgs], {
    cwd: process.cwd(),
    env: reportEnv(extraEnv),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  expect(result.status).toBe(1)
  expect(result.stderr).toBe('')
  return JSON.parse(result.stdout) as Record<string, unknown>
}

function runReportWithoutDryRun(
  script: string,
  extraArgs: string[] = [],
  extraEnv: NodeJS.ProcessEnv = {}
): Record<string, unknown> {
  const result = spawnSync(process.execPath, [script, '--json', ...extraArgs], {
    cwd: process.cwd(),
    env: reportEnv(extraEnv),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  expect(result.status).toBe(0)
  expect(result.stderr).toBe('')
  return JSON.parse(result.stdout) as Record<string, unknown>
}

function parseLastJSONObject(text: string): Record<string, any> {
  let start = text.lastIndexOf('{')
  while (start >= 0) {
    const candidate = text.slice(start).trim()
    try {
      const value = JSON.parse(candidate)
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        return value as Record<string, any>
      }
    } catch {
      start = text.lastIndexOf('{', start - 1)
      continue
    }
    start = text.lastIndexOf('{', start - 1)
  }
  throw new Error('no JSON object found in command output')
}

function writeExecutable(path: string, source: string): void {
  writeFileSync(path, source, { encoding: 'utf8', mode: 0o755 })
}

function sha256Bytes(value: Buffer | string): string {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value) as string
}

function packagedQAHarnessEntries(): Array<Record<string, unknown>> {
  return [
    ['runtime-go-packaged-milestone-a.mjs', 'scripts/runtime-go-packaged-milestone-a.mjs'],
    ['local-provider-acceptance.mjs', 'scripts/lib/local-provider-acceptance.mjs'],
    ['local-provider-credential-scan.mjs', 'scripts/lib/local-provider-credential-scan.mjs'],
    ['runtime-go-packaged-qa.mjs', 'scripts/runtime-go-packaged-qa.mjs'],
    ['runtime-go-live-evidence-collector.mjs', 'scripts/runtime-go-live-evidence-collector.mjs'],
    ['runtime-go-validation-command.mjs', 'scripts/runtime-go-validation-command.mjs'],
    ['runtime-go-cutover-report.mjs', 'scripts/runtime-go-cutover-report.mjs'],
    ['runtime-go-packaged-gui-smoke.mjs', 'scripts/runtime-go-packaged-gui-smoke.mjs'],
    ['runtime-go-packaged-session-soak.mjs', 'scripts/runtime-go-packaged-session-soak.mjs'],
    ['runtime-go-rollback-retirement-evidence.mjs', 'scripts/runtime-go-rollback-retirement-evidence.mjs'],
    ['packaging-config.test.ts', 'src/main/packaging-config.test.ts'],
    ['after-pack.cjs', 'scripts/after-pack.cjs'],
    ['packaged-release-publication-authority.mjs', 'scripts/lib/packaged-release-publication-authority.mjs'],
    ['publish-r2.mjs', 'scripts/publish-r2.mjs'],
    ['strict-json.cjs', 'scripts/lib/strict-json.cjs'],
    ['macos-signing-policy.cjs', 'scripts/macos-signing-policy.cjs'],
    ['macos-signing-policy.json', 'scripts/macos-signing-policy.json'],
    ['entitlements.mac.native-helper.plist', 'build/entitlements.mac.native-helper.plist'],
    ['package.json', 'package.json'],
    ['package-lock.json', 'package-lock.json'],
    ['scripts/use-analytix-cache.sh', 'scripts/use-analytix-cache.sh'],
    ['scripts/analytix-cache-storage.zsh', 'scripts/analytix-cache-storage.zsh']
  ].map(([name, relativePath]) => {
    const path = resolve(process.cwd(), relativePath)
    const stat = lstatSync(path)
    return {
      name,
      regular: stat.isFile() && !stat.isSymbolicLink(),
      byteLength: stat.size,
      sha256: sha256Bytes(readFileSync(path))
    }
  })
}

function runGit(cwd: string, args: string[]): string {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8', stdio: 'pipe' })
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr}`)
  }
  return result.stdout.trim()
}

function nativeFixtureBytes(format: 'mach-o' | 'pe', arch: 'arm64' | 'x64', marker: string): Buffer {
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
    Buffer.from(marker, 'utf8').copy(bytes, 0x1c0, 0, 48)
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
  bytes.write('PE\u0000\u0000', peOffset, 'ascii')
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
  Buffer.from(marker, 'utf8').copy(bytes, 0x300, 0, 64)
  return bytes
}

type FormalPackageFixture = {
  appPath: string
  appOutDir: string
  runtimeDir: string
  executable: Buffer
  appAsar: Buffer
  runtimeServer: Buffer
  sourceCommit: string
  targetPlatform: 'darwin' | 'win32'
  targetArch: 'arm64' | 'x64'
}

function currentPackagedWorktreeSnapshot(): Record<string, any> {
  return packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(process.cwd()) as Record<string, any>
}

function cleanFixtureWorktreeSnapshot(root: string): Record<string, any> {
  const repository = join(root, 'source-repository')
  mkdirSync(repository)
  runGit(repository, ['init', '--quiet'])
  writeFileSync(join(repository, 'source.ts'), 'export const source = true\n', 'utf8')
  runGit(repository, ['add', 'source.ts'])
  runGit(repository, [
    '-c', 'user.name=Analytix Test',
    '-c', 'user.email=test@analytix.invalid',
    'commit', '--quiet', '-m', 'fixture'
  ])
  return packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(repository) as Record<string, any>
}

function writeBundledFundsPlugin(context: Record<string, any>): void {
  const sourceRoot = join(process.cwd(), 'plugins', 'analytix-fund-analysis')
  const destinationRoot = join(
    packagedAuthorityContract.packedResourcesDir(context),
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
  for (const relativePath of ANALYTIX_FUNDS_PRODUCTION_MCP_FILES) {
    cpSync(join(sourceRoot, relativePath), join(destinationRoot, relativePath))
  }
}

function writeFixtureBuildAuthority(
  context: Record<string, any>,
  runtimeDir: string,
  targetKey: string,
  nativeReceipt: string,
  executable: Buffer,
  appAsar: Buffer,
  runtimeServer: Buffer,
  options: {
    version?: 1 | 2
    worktreeSnapshot?: Record<string, any>
    executableBinding?: Record<string, any>
    runtimeServerBinding?: Record<string, any>
  } = {}
): Record<string, any> {
  const worktreeSnapshot = options.worktreeSnapshot || currentPackagedWorktreeSnapshot()
  const artifacts = {
    executable: options.executableBinding,
    appAsar: { sha256: sha256Bytes(appAsar), byteLength: appAsar.length },
    runtimeServer: options.runtimeServerBinding,
    fundsPlugin: packagedAuthorityContract.collectPackagedFundsPluginIdentityV2(context)
  }
  const authority = options.version === 1
    ? {
        schemaVersion: 1,
        contract: 'analytix.packaged-build-authority/v1',
        sourceCommit: worktreeSnapshot.sourceCommit,
        targetKey,
        nativeComponentsReceiptSha256: sha256Bytes(nativeReceipt),
        artifacts: {
          executable: { sha256: sha256Bytes(executable), byteLength: executable.length },
          appAsar: artifacts.appAsar,
          runtimeServer: { sha256: sha256Bytes(runtimeServer), byteLength: runtimeServer.length }
        }
      }
    : packagedAuthorityContract.createPackagedBuildAuthorityV2({
        worktreeSnapshot,
        targetKey,
        buildContext: packagedAuthorityContract.collectEffectiveBuilderContextV1(context),
        nativeDisposition: {
          kind: 'controlled_release_receipt',
          targetKey,
          receiptSha256: sha256Bytes(nativeReceipt),
          manifestSha256: 'b'.repeat(64),
          signingPolicySha256: targetKey.startsWith('darwin-') ? 'c'.repeat(64) : '',
          signingMode: targetKey.startsWith('darwin-') ? 'ad-hoc' : '',
          appleTeamIdentifier: ''
        },
        artifacts,
        stagedPayload: packagedAuthorityContract.collectStagedPayloadClosureV1(context)
      })
  writeFileSync(join(runtimeDir, PACKAGED_BUILD_AUTHORITY_FILE), JSON.stringify(authority), 'utf8')
  return authority as Record<string, any>
}

function writeFormalMacPackageFixture(
  root: string,
  options: {
    arch?: 'arm64' | 'x64'
    includeBuildAuthority?: boolean
    authorityVersion?: 1 | 2
    worktreeSnapshot?: Record<string, any>
  } = {}
): FormalPackageFixture {
  const arch = options.arch || 'arm64'
  const worktreeSnapshot = options.worktreeSnapshot || currentPackagedWorktreeSnapshot()
  const sourceCommit = String(worktreeSnapshot.sourceCommit)
  const appPath = join(root, 'analytix.app')
  const executablePath = join(appPath, 'Contents', 'MacOS', 'analytix')
  const resources = join(appPath, 'Contents', 'Resources')
  const runtimeServerPath = join(resources, 'runtime-go', 'bin', 'runtime-server')
  const runtimeDir = join(resources, 'runtime')
  const appAsarPath = join(resources, 'app.asar')
  mkdirSync(join(appPath, 'Contents', 'MacOS'), { recursive: true })
  mkdirSync(join(resources, 'runtime-go', 'bin'), { recursive: true })
  mkdirSync(runtimeDir, { recursive: true })
  const executable = nativeFixtureBytes('mach-o', arch, 'fixture-electron')
  const runtimeServer = nativeFixtureBytes('mach-o', arch, 'fixture-runtime-server')
  const appAsar = Buffer.from('fixture-app-asar-content', 'utf8')
  writeFileSync(executablePath, executable, { mode: 0o755 })
  writeFileSync(runtimeServerPath, runtimeServer, { mode: 0o755 })
  writeFileSync(appAsarPath, appAsar)
  const targetKey = `darwin-${arch}`
  const nativeReceiptPath = join(runtimeDir, 'analytix-native-components-receipt.json')
  const nativeReceipt = `${JSON.stringify({ schemaVersion: 1, targetKey })}\n`
  writeFileSync(nativeReceiptPath, nativeReceipt, 'utf8')
  const context = {
    appOutDir: root,
    electronPlatformName: 'darwin',
    arch,
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
  writeBundledFundsPlugin(context)
  if (options.includeBuildAuthority !== false) {
    writeFixtureBuildAuthority(context, runtimeDir, targetKey, nativeReceipt, executable, appAsar, runtimeServer, {
      version: options.authorityVersion,
      worktreeSnapshot,
      executableBinding: packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        executablePath,
        { format: 'mach-o', arch }
      ),
      runtimeServerBinding: packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        runtimeServerPath,
        { format: 'mach-o', arch }
      )
    })
  }
  return {
    appPath,
    appOutDir: root,
    runtimeDir,
    executable,
    appAsar,
    runtimeServer,
    sourceCommit,
    targetPlatform: 'darwin',
    targetArch: arch
  }
}

function writeFormalWindowsPackageFixture(
  root: string,
  options: { includeBuildAuthority?: boolean; worktreeSnapshot?: Record<string, any> } = {}
): FormalPackageFixture {
  const worktreeSnapshot = options.worktreeSnapshot || currentPackagedWorktreeSnapshot()
  const sourceCommit = String(worktreeSnapshot.sourceCommit)
  const appPath = join(root, 'win-unpacked', 'analytix.exe')
  const appOutDir = join(root, 'win-unpacked')
  const resources = join(root, 'win-unpacked', 'resources')
  const runtimeServerPath = join(resources, 'runtime-go', 'bin', 'runtime-server.exe')
  const runtimeDir = join(resources, 'runtime')
  const appAsarPath = join(resources, 'app.asar')
  mkdirSync(join(resources, 'runtime-go', 'bin'), { recursive: true })
  mkdirSync(runtimeDir, { recursive: true })
  const executable = nativeFixtureBytes('pe', 'x64', 'fixture-electron')
  const runtimeServer = nativeFixtureBytes('pe', 'x64', 'fixture-runtime-server')
  const appAsar = Buffer.from('fixture-windows-app-asar-content', 'utf8')
  writeFileSync(appPath, executable)
  writeFileSync(runtimeServerPath, runtimeServer)
  writeFileSync(appAsarPath, appAsar)
  const targetKey = 'win32-x64'
  const nativeReceiptPath = join(runtimeDir, 'analytix-native-components-receipt.json')
  const nativeReceipt = `${JSON.stringify({ schemaVersion: 1, targetKey })}\n`
  writeFileSync(nativeReceiptPath, nativeReceipt, 'utf8')
  const context = {
    appOutDir,
    electronPlatformName: 'win32',
    arch: 'x64',
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
  writeBundledFundsPlugin(context)
  if (options.includeBuildAuthority !== false) {
    writeFixtureBuildAuthority(context, runtimeDir, targetKey, nativeReceipt, executable, appAsar, runtimeServer, {
      worktreeSnapshot,
      executableBinding: packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        appPath,
        { format: 'pe', arch: 'x64' }
      ),
      runtimeServerBinding: packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        runtimeServerPath,
        { format: 'pe', arch: 'x64' }
      )
    })
  }
  return {
    appPath,
    appOutDir,
    runtimeDir,
    executable,
    appAsar,
    runtimeServer,
    sourceCommit,
    targetPlatform: 'win32',
    targetArch: 'x64'
  }
}

type DevelopmentNativeFixture = {
  context: Record<string, any>
  runtimeDir: string
  sourceDir: string
  targetKey: 'darwin-arm64' | 'win32-x64'
  format: 'mach-o' | 'pe'
  arch: 'arm64' | 'x64'
  binaries: Array<{ id: string; binaryName: string; bytes: Buffer }>
}

function writeDevelopmentNativeFixture(
  root: string,
  platform: 'darwin' | 'win32',
  arch: 'arm64' | 'x64'
): DevelopmentNativeFixture {
  const targetKey = `${platform}-${arch}` as 'darwin-arm64' | 'win32-x64'
  const format = platform === 'darwin' ? 'mach-o' : 'pe'
  const targetTriple = platform === 'darwin' ? 'aarch64-apple-darwin' : 'x86_64-pc-windows-msvc'
  const sourceDir = join(root, 'runtime', 'native-components-development', targetKey)
  const appOutDir = platform === 'darwin'
    ? join(root, 'package')
    : join(root, 'package', 'win-unpacked')
  const runtimeDir = platform === 'darwin'
    ? join(appOutDir, 'analytix.app', 'Contents', 'Resources', 'runtime')
    : join(appOutDir, 'resources', 'runtime')
  const resourcesDir = join(runtimeDir, '..')
  const executablePath = platform === 'darwin'
    ? join(appOutDir, 'analytix.app', 'Contents', 'MacOS', 'analytix')
    : join(appOutDir, 'analytix.exe')
  const runtimeServerPath = join(
    resourcesDir,
    'runtime-go',
    'bin',
    platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
  )
  mkdirSync(sourceDir, { recursive: true, mode: 0o700 })
  chmodSync(join(root, 'runtime'), 0o700)
  chmodSync(join(root, 'runtime', 'native-components-development'), 0o700)
  chmodSync(sourceDir, 0o700)
  mkdirSync(runtimeDir, { recursive: true })
  mkdirSync(join(executablePath, '..'), { recursive: true })
  mkdirSync(join(runtimeServerPath, '..'), { recursive: true })
  writeFileSync(executablePath, nativeFixtureBytes(format, arch, 'development-electron'), {
    mode: platform === 'darwin' ? 0o755 : 0o644
  })
  writeFileSync(runtimeServerPath, nativeFixtureBytes(format, arch, 'development-runtime'), {
    mode: platform === 'darwin' ? 0o755 : 0o644
  })
  writeFileSync(join(resourcesDir, 'app.asar'), 'development-app-asar', 'utf8')
  const binaries = DEVELOPMENT_NATIVE_COMPONENTS.map(([id, baseName]) => {
    const binaryName = platform === 'win32' ? `${baseName}.exe` : baseName
    const bytes = nativeFixtureBytes(format, arch, `development-${id}`)
    const path = join(sourceDir, binaryName)
    writeFileSync(path, bytes, { mode: 0o700 })
    return { id, binaryName, bytes, path }
  })
  const components = binaries.map(({ id, binaryName, path }) => {
    const binding = packagedAuthorityContract.signingInvariantNativeArtifactBinding(path, {
      format,
      arch
    }) as Record<string, any>
    return {
      id,
      sourceDigest: '1'.repeat(64),
      cargoLockSha256: '2'.repeat(64),
      buildEnvironmentSha256: '3'.repeat(64),
      binaryName,
      binarySha256: binding.preSignSha256,
      binarySize: binding.preSignByteLength,
      payloadSha256: binding.payloadSha256,
      payloadSize: binding.payloadByteLength,
      format,
      arch
    }
  })
  const marker = {
    schemaVersion: 1,
    kind: 'analytix_native_development_build',
    classification: NATIVE_DISPOSITION_DEVELOPMENT,
    publishable: false,
    releaseEligible: false,
    authorityUse: 'development_only',
    targetKey,
    targetTriple,
    platform,
    arch,
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
  writeFileSync(join(sourceDir, NATIVE_DEVELOPMENT_BUILD_MARKER), JSON.stringify(marker), {
    mode: 0o600
  })
  const context = {
    appOutDir,
    electronPlatformName: platform,
    arch,
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
  writeBundledFundsPlugin(context)
  return {
    context,
    runtimeDir,
    sourceDir,
    targetKey,
    format,
    arch,
    binaries: binaries.map(({ path: _path, ...binary }) => binary)
  }
}

function runFormalPackagedRunner(
  script: 'scripts/runtime-go-packaged-gui-smoke.mjs' | 'scripts/runtime-go-packaged-session-soak.mjs',
  fixture: FormalPackageFixture,
  root: string,
  extraArgs: string[] = [],
  options: { expectedSourceCommit?: string; formal?: boolean } = {}
): Record<string, any> {
  const bin = join(root, 'bin')
  mkdirSync(bin, { recursive: true })
  const fakeNpm = join(bin, process.platform === 'win32' ? 'npm.cmd' : 'npm')
  const fakeGo = join(bin, process.platform === 'win32' ? 'go.cmd' : 'go')
  writeExecutable(fakeNpm, '#!/usr/bin/env node\nprocess.exit(0)\n')
  writeExecutable(fakeGo, `#!/usr/bin/env node
const args = process.argv.slice(2)
const runIndex = Math.max(args.indexOf('-run'), args.indexOf('-test.run'))
if ((args.includes('-json') || args.includes('test2json')) && runIndex >= 0) {
  const testName = String(args[runIndex + 1] || '').replace(/^\\^|\\$$/g, '')
  if (testName) {
    console.log(JSON.stringify({ Action: 'pass', Package: 'analytix.local/runtime-go', Test: testName }))
    console.log(JSON.stringify({ Action: 'pass', Package: 'analytix.local/runtime-go' }))
  }
}
process.exit(0)
`)
  const tempRoot = join(root, 'tmp')
  mkdirSync(tempRoot, { recursive: true })
  const result = spawnSync(process.execPath, [
    script,
    '--json',
    '--no-gate',
    '--no-write',
    options.formal === false ? '--actual' : '--formal-instance',
    '--app-path', fixture.appPath,
    '--target-platform', fixture.targetPlatform,
    '--target-arch', fixture.targetArch,
    '--expected-source-commit', options.expectedSourceCommit || fixture.sourceCommit,
    ...extraArgs
  ], {
    cwd: process.cwd(),
    env: reportEnv({
      GO: fakeGo,
      PATH: `${bin}${delimiter}${process.env.PATH || ''}`,
      TMPDIR: tempRoot
    }),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  expect(result.status).toBe(0)
  expect(result.stderr).toBe('')
  expect(readdirSync(tempRoot)).toEqual([])
  return JSON.parse(result.stdout) as Record<string, any>
}

function authorityBoundarySmokeFields(): Record<string, unknown> {
  return {
    actualRuntimeProcessStarted: true,
    productionRuntime: true,
    durableRootConfigured: true,
    dataDirScope: 'temporary',
    usesRealProviderConfig: false,
    providerConfigMode: 'isolated-contract-provider-config',
    productChainMode: 'production-authority-boundary',
    productChainOk: false,
    authorityBoundaryOk: true,
    hostAcceptedFinalOk: true,
    providerPositiveChainOk: false,
    providerAgentLoopReady: false,
    providerAgentLoopCoverage: 'blocked-before-dispatch',
    providerExecutionExpected: false,
    providerExecutionBlocked: true,
    turnCreateOk: true,
    sseReplayOk: true,
    usageOk: false,
    providerUsageObserved: false,
    providerUsageAbsentOk: true,
    attachmentOk: true,
    providerDraftWithheld: true,
    contractProviderRequestCount: 0,
    actualPackagedAppLaunched: false,
    healthOk: true,
    runtimeInfoOk: true,
    runtimeToolsOk: true,
    productionCapabilitiesOk: true,
    secretsPrinted: false
  }
}

function firstRunnableSmokeFields(): Record<string, unknown> {
  return {
    actualRuntimeProcessStarted: true,
    productionRuntime: true,
    durableRootConfigured: true,
    dataDirScope: 'temporary',
    usesRealProviderConfig: false,
    providerConfigMode: 'isolated-contract-provider-config',
    productChainMode: 'production-authority-enrolled-provider-agent-chain',
    productChainOk: true,
    authorityBoundaryOk: true,
    hostAcceptedFinalOk: true,
    providerPositiveChainOk: true,
    providerAgentLoopReady: true,
    providerAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
    providerExecutionExpected: true,
    providerExecutionBlocked: false,
    protectedTurnCreateOk: true,
    ordinaryTurnCreateOk: true,
    protectedSseReplayOk: true,
    ordinarySseReplayOk: true,
    ordinaryThreadDistinct: true,
    ordinaryPublicFinalOk: true,
    turnCreateOk: true,
    sseReplayOk: true,
    usageOk: true,
    providerUsageObserved: true,
    providerUsageAbsentOk: true,
    providerDraftWithheld: true,
    protectedProviderDispatchAbsent: true,
    protectedProviderRequestCount: 0,
    attachmentOk: true,
    contractProviderRequestCount: 1,
    contractProviderPromptSeen: true,
    providerAuthorizationConfigured: true,
    actualPackagedAppLaunched: false,
    healthOk: true,
    runtimeInfoOk: true,
    runtimeToolsOk: true,
    productionCapabilitiesOk: true,
    secretsPrinted: false
  }
}

function firstRunnableHealthReport(): Record<string, unknown> {
  return {
    schemaVersion: 2,
    id: 'runtime-go-runtime-health-smoke',
    status: 'passed',
    passed: true,
    smoke: firstRunnableSmokeFields(),
    checks: [
      { id: 'runtime-server-build', status: 'passed' },
      { id: 'runtime-server-ready', status: 'passed' },
      { id: 'runtime-health', status: 'passed' },
      { id: 'runtime-info', status: 'passed' },
      { id: 'runtime-tools', status: 'passed' },
      { id: 'runtime-authority-boundary', status: 'passed' },
      { id: 'runtime-provider-agent-chain', status: 'passed' },
      { id: 'runtime-server-cleanup', status: 'passed' }
    ]
  }
}

function firstRunnableRuntimeHealthFields(): Record<string, unknown> {
  return {
    passed: true,
    runtimeIdentityEvidenceValid: true,
    boundaryEvidenceValid: true,
    providerAgentChainEvidenceValid: true,
    ...firstRunnableSmokeFields(),
    usesIsolatedEmptyProviderConfig: false,
    usesIsolatedContractProviderConfig: true
  }
}

function runCutoverWithFirstRunnableMutation(
  mutate: (runtimeHealth: Record<string, unknown>) => void
): Record<string, unknown> {
  const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-first-runnable-mutation-'))
  try {
    const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const runtimeHealth = firstRunnableRuntimeHealthFields()
    mutate(runtimeHealth)
    const candidate = {
      schemaVersion: 1,
      id: 'runtime-go-default-readiness-report',
      status: 'passed',
      passed: true,
      goDefaultReady: true,
      productRegression: {
        passed: true,
        features: ['TypeScript retired backend'],
        rowCount: 33,
        publicLegacyExposureCount: 0,
        internalLegacyDelegateCount: 0,
        temporaryGoProdViolationCount: 0,
        productionGoSourceCount: 28,
        retiredProductionMarkerViolationCount: 0,
        legacyUpstreamFieldKeyViolationCount: 0
      },
      speedCacheGate: {
        passed: true,
        checkCount: 14
      },
      runtimeHealth
    }
    writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:default-readiness-report') {
  console.log(${JSON.stringify(JSON.stringify(candidate))})
  process.exit(0)
}
if (args[0] === 'run' && (args[1] === 'typecheck' || args[1] === 'build:runtime')) {
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'diff' && args[1] === '--check') process.exit(0)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)
    return runReportWithoutDryRun('scripts/runtime-go-cutover-report.mjs', [
      '--live-evidence-json',
      join(dir, 'missing-live-evidence.json')
    ], {
      PATH: `${dir}${delimiter}${process.env.PATH || ''}`
    })
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

function authorityBoundaryRuntimeHealthFields(): Record<string, unknown> {
  return {
    passed: true,
    boundaryEvidenceValid: true,
    ...authorityBoundarySmokeFields(),
    usesIsolatedEmptyProviderConfig: false,
    usesIsolatedContractProviderConfig: true
  }
}

describe('formal runtime Go packaged contract reports', () => {
  it('keeps the historical runtime closure matrix as an independent later-scope audit', () => {
    const productRegression = readFileSync('scripts/runtime-go-product-regression.mjs', 'utf8')
    const preflight = readFileSync('scripts/runtime-go-preflight.mjs', 'utf8')
    const releaseGateEntry = readFileSync('scripts/runtime-go-release-gate.mjs', 'utf8')
    const releaseFinalization = readFileSync('scripts/runtime-go-release-finalization.mjs', 'utf8')
    const releaseGate = `${releaseGateEntry}\n${releaseFinalization}`
    const upstreamAudit = readFileSync('scripts/upstream-source-audit.mjs', 'utf8')

    expect(productRegression).not.toContain("id: 'runtime-closure-matrix-audit'")
    expect(productRegression).not.toContain(
      "args: ['scripts/runtime-closure-matrix-audit.mjs', '--json']"
    )
    expect(productRegression).toContain("assertGitNotIgnored('scripts/runtime-closure-matrix-audit.mjs')")
    expect(productRegression).toContain("id: 'go-build-cache-maintenance-preflight'")
    expect(productRegression).toContain("args: ['build', './internal/contracts']")
    expect(productRegression.indexOf("id: 'go-build-cache-maintenance-preflight'"))
      .toBeLessThan(productRegression.indexOf("id: 'production-product-regression-matrix'"))
    for (const requiredCheck of [
      'production-product-regression-matrix',
      'production-runtime-build-tag',
      'runtime-go-contracts',
      'product-sovereignty-scan'
    ]) {
      expect(productRegression).toContain(`id: '${requiredCheck}'`)
    }
    expect(preflight).toContain("args: ['run', 'runtime:go:product-regression']")
    expect(releaseGate).toContain("args: ['./scripts/runtime-go-preflight.mjs', '--rc-control-plane', '--json']")
    expect(releaseGate).not.toContain("args: ['run', 'runtime:go:product-regression']")
    expect(releaseGate).not.toContain("args: ['run', 'runtime:go:speed-cache-gate']")
    expect(releaseGate).toContain("const gateMode = controlPlaneOnly ? 'control-plane' : executionOnly ? 'execution' : 'release'")
    expect(releaseGate).toContain("const status = controlPlaneOnly")
    expect(releaseGate).toContain('passed: productRcGatePassed')
    expect(releaseGate).toContain('openRcRequiredIds: rcLedger.openRcRequiredIds')
    expect(releaseGate).toContain("id: 'artifact-legal-obligations-audit'")
    expect(releaseGate).toContain("./scripts/artifact-legal-obligations-audit.mjs")
    expect(releaseGate).toContain("item.id !== 'artifact-legal-obligations-audit'")
    expect(releaseGate).toContain('delete environment[exactArtifactEnvironmentKey]')
    expect(releaseGate).toContain('env: commandEnvironment(item)')
    expect(releaseGate).toContain('artifactLegalPlanPassed')
    expect(releaseGate).toContain("id: 'upstream-source-audit'")
    expect(releaseGate).toContain('./scripts/upstream-source-audit.mjs')
    expect(releaseGate).toContain("'--release-head'")
    expect(releaseGate).toContain("'--release-tree'")
    expect(releaseGate).toContain('upstreamArtifactProjectionValid')
    expect(releaseGate).toContain('upstreamArtifactLicenseClassGatePassed')
    expect(releaseGate).toContain('upstreamArtifactAdmissionGatePassed')
    expect(releaseGate).toContain('upstreamArtifactAdmission.projectionSha256')
    expect(releaseGate).toContain('upstreamArtifactAdmission.releaseSource')
    expect(releaseGate).toContain('normalizeUpstreamArtifactProjection')
    expect(releaseGate).toContain('upstreamArtifactState.entryViews')
    expect(releaseGate).toContain("from './upstream-source-audit.mjs'")
    expect(upstreamAudit).toContain('entry problems must be an array of non-empty strings')
    expect(releaseGate).toContain('upstreamArtifactAdmissionBlockers')
    expect(releaseGate).toContain('researchFreshness')
    expect(releaseGate).toMatch(
      /const executionPassed = controlPlaneReady &&\s+artifactAdmissionGatePassed &&\s+upstreamArtifactAdmissionGatePassed &&/
    )
    expect(releaseGate).toMatch(
      /const upstreamArtifactAdmissionGatePassed = upstreamArtifactProjectionValid &&\s+upstreamArtifactLicenseClassGatePassed &&/
    )
    expect(releaseGate).not.toMatch(
      /const controlPlaneReady =[\s\S]*?upstreamArtifactAdmission\.ok/
    )
    expect(releaseGate).not.toContain('openBenchmarkResearchIds')
    expect(releaseGate).toContain('rcLedger.rcLedgerValid')
    expect(releaseGate).toContain('process.exitCode = gateExitCode({ mode: gateMode, controlPlaneReady, executionPassed, passed: productRcGatePassed })')
    expect(releaseGateEntry.indexOf('if (finalizationOnly)')).toBeLessThan(releaseGateEntry.indexOf('const receiptRun = createReceiptRun'))
    expect(releaseGateEntry).toContain('const productRcGatePassed = false')
    expect(releaseFinalization).toContain("fileName: 'execution-success-seal.json'")
    expect(releaseFinalization).toContain('ledger.openRcRequiredIds.length === 0')
    expect(releaseGate).toContain('receiptIntegrityValid')
    expect(releaseGate).toContain('boundaryProjectionValid')
    expect(releaseGate).toContain("from './runtime-go-formal-evidence.mjs'")
    expect(releaseGate).toContain("fileName: 'formal-evidence-ingestion.json'")
    expect(releaseGate).toContain('...formalEvidenceBlockers')
    expect(releaseGate).not.toContain("'commercial-release:not_authorized'")
    expect(releaseGate).not.toContain("'artifact-formal:not_executed'")
    expect(releaseGate).toContain('engineeringBlocking: false')

    const selfTest = spawnSync(
      process.execPath,
      ['scripts/runtime-closure-matrix-audit.mjs', '--self-test', '--json'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )
    expect(selfTest.status).toBe(0)
    expect(selfTest.stderr).toBe('')
    expect(JSON.parse(selfTest.stdout)).toEqual(expect.objectContaining({ ok: true }))
  })

  it('binds the reasoning/cache gate to the current exact DeepSeek continuation test', () => {
    const speedCacheGate = readFileSync('scripts/runtime-go-speed-cache-gate.mjs', 'utf8')
    const runtimeTests = readFileSync('packages/runtime-go/runtime_server_test.go', 'utf8')
    const currentTest = 'TestRuntimeServerDeepSeekToolContinuationReplaysReasoningOnlyWithinExactAttempt'

    expect(runtimeTests).toContain(`func ${currentTest}(`)
    expect(speedCacheGate).toContain(currentTest)
    expect(speedCacheGate).not.toContain(
      'TestRuntimeServerDeepSeekToolContinuationNeverReuploadsReasoning'
    )
  })

  it('measures atomic terminal publication from the provider done boundary', () => {
    const speedCacheGate = readFileSync('scripts/runtime-go-speed-cache-gate.mjs', 'utf8')

    expect(speedCacheGate).toContain(
      'const providerDoneToAtomicTerminalMs = Number(timings.providerDoneToAtomicTerminalMs)'
    )
    expect(speedCacheGate).toContain(
      "atomic_terminal_publish_ms: metricMs(providerDoneToAtomicTerminalMs, 1000, 'runtime-go-performance-check timings.providerDoneToAtomicTerminalMs')"
    )
    expect(speedCacheGate).toContain(
      "safe_progress_to_terminal_ms: metricMs(safeProgressToTerminalMs, 1000, 'runtime-go-performance-check atomicTerminalMs - firstSafeProgressMs')"
    )
  })

  it('keeps the release gate production test fresh and package-serial', () => {
    const report = runReport('scripts/runtime-go-release-gate.mjs') as Record<string, any>
    const controlPlaneReport = runReport(
      'scripts/runtime-go-release-gate.mjs',
      ['--control-plane-only']
    ) as Record<string, any>
    const formalDryRunReport = runReport(
      'scripts/runtime-go-release-gate.mjs',
      ['--formal-evidence-dir', '/path-that-must-not-be-read-by-dry-run']
    ) as Record<string, any>
    const manifest = report.executionManifest as Array<Record<string, unknown>>

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-release-gate',
      commandMode: 'release',
      status: 'skipped',
      passed: false,
      commandPassed: false,
      controlPlaneReady: false,
      productRcGatePassed: false,
      artifactAdmissionGatePassed: false,
      upstreamArtifactAdmissionGatePassed: false,
      releaseAuthorized: false,
      dryRunCannotAuthorizeRelease: true,
      formalEvidence: {
        electron: 'not_executed',
        package: 'not_executed',
        a0: 'not_executed',
        b1: 'not_executed',
        commercialRelease: 'not_authorized'
      },
      externalReleaseAuthorization: expect.objectContaining({
        status: 'not_authorized',
        engineeringBlocking: false
      })
    }))
    expect(controlPlaneReport).toEqual(expect.objectContaining({
      id: 'runtime-go-rc-control-plane',
      commandMode: 'control-plane',
      status: 'skipped',
      passed: false,
      commandPassed: false,
      controlPlaneReady: false,
      productRcGatePassed: false,
      releaseAuthorized: false,
      dryRunCannotAuthorizeRelease: true
    }))
    expect(controlPlaneReport.executionManifest).toEqual(report.executionManifest)
    expect(formalDryRunReport.formalEvidenceInput).toEqual({
      requested: true,
      consumed: false
    })
    expect(formalDryRunReport.formalEvidence).toEqual(report.formalEvidence)
    expect(manifest).toContainEqual(expect.objectContaining({
      id: 'upstream-source-audit',
      expectedExecutions: 1,
      command: expect.stringContaining('./scripts/upstream-source-audit.mjs --json --release-head')
    }))
    expect(manifest).toContainEqual(expect.objectContaining({
      id: 'go-prod-matrix',
      expectedExecutions: 1,
      command: 'go test -p=1 -count=1 -tags analytix_prod -timeout=15m ./...'
    }))
    expect(manifest).toContainEqual(expect.objectContaining({
      id: 'go-race-matrix',
      expectedExecutions: 1,
      command: 'go test -p=1 -count=1 -race -timeout=15m ./...'
    }))
    expect(manifest).toContainEqual(expect.objectContaining({
      id: 'rc-structural-and-performance-suite',
      expectedExecutions: 1,
      logicalChecks: ['structural-performance', 'rc-performance-benchmark']
    }))
  })

  it('audits the source plan while fail-closing exact artifact admission', () => {
    const builderConfig = readFileSync('electron-builder.config.cjs', 'utf8')
    const notice = readFileSync('THIRD_PARTY_NOTICES.md', 'utf8')

    expect(builderConfig).toContain("from: 'THIRD_PARTY_NOTICES.md'")
    expect(builderConfig).toContain("to: 'THIRD_PARTY_NOTICES.md'")
    expect(builderConfig).toContain("from: 'LICENSE'")
    expect(builderConfig).toContain("to: 'LICENSE'")
    expect(notice).toContain('Copyright (c) 2026 Reasonix Contributors')
    expect(notice).toContain('Permission is hereby granted, free of charge')
    expect(notice).toContain('The above copyright notice and this permission notice')
    expect(notice).toContain('THE SOFTWARE IS PROVIDED "AS IS"')
    expect(notice).not.toContain('code-reuse-provenance')
    expect(notice).not.toContain('packages/runtime-go/internal/')

    const selfTest = spawnSync(
      process.execPath,
      ['scripts/artifact-legal-obligations-audit.mjs', '--self-test', '--json'],
      {
        cwd: process.cwd(),
        encoding: 'utf8',
        stdio: 'pipe',
        env: {
          ...process.env,
          ANALYTIX_EXACT_ARTIFACT_PATH: '/must/not/affect/programmatic-self-test'
        }
      }
    )
    expect(selfTest.status).toBe(0)
    expect(selfTest.stderr).toBe('')
    expect(JSON.parse(selfTest.stdout)).toEqual(expect.objectContaining({
      id: 'analytix-artifact-legal-obligations-self-test',
      status: 'passed',
      passed: true,
      cases: expect.objectContaining({
        mit: true,
        apache: true,
        dualChoicePreserved: true,
        missingLicenseBlocked: true,
        internalMetadataSeparated: true,
        missingExactInputUnverified: true,
        unreadableExactInputBlocked: true,
        commercialDecisionSeparated: true,
        duplicateInstancesRetained: true,
        sourcePlanDoesNotAuthorizeExact: true
      })
    }))

    const audit = spawnSync(
      process.execPath,
      ['scripts/artifact-legal-obligations-audit.mjs', '--json'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )
    expect(audit.status).toBe(0)
    expect(audit.stderr).toBe('')
    expect(JSON.parse(audit.stdout)).toEqual(expect.objectContaining({
      id: 'analytix.artifact-legal-obligations/v1',
      status: 'passed',
      passed: true,
      artifactLegalPlanPassed: true,
      exactFormalArtifact: expect.objectContaining({
        status: 'UNVERIFIED',
        provided: false,
        passed: false,
        exactArtifactReadable: false
      }),
      artifactAdmissionGatePassed: false,
      formalArtifactNotChecked: true,
      artifactAdmissionBlockers: []
    }))
  })

  it('rejects stale, command-drifted, and tampered RC receipts', () => {
    const result = spawnSync(
      process.execPath,
      ['scripts/runtime-go-rc-receipt.mjs', '--self-test'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )

    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout)).toEqual(expect.objectContaining({
      id: 'runtime-go-rc-receipt-self-test',
      status: 'passed',
      passed: true,
      cases: {
        valid: true,
        commandDriftRejected: true,
        staleRunRejected: true,
        sourceDriftRejected: true,
        tamperRejected: true,
        duplicateFormalRunRejected: true,
        validLedgerProjected: true,
        duplicateTaskRejected: true,
        unclassifiedTaskRejected: true,
        invalidCheckboxRejected: true,
        classificationMismatchRejected: true,
        benchmarkResearchRejected: true,
        completedBenchmarkResearchRejected: true,
        taskHashDriftRejected: true,
        missingChangeRejected: true,
        releaseFalseExitsNonzero: true,
        releaseTrueExitsZero: true,
        controlPlaneReadyExitsZero: true,
        controlPlaneFailureExitsNonzero: true
      }
    }))
  })

  it('skips product regression child commands when requested', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-product-regression-skip-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const marker = join(dir, 'unexpected-command.txt')
      const failIfCalled = `#!/usr/bin/env node
require('node:fs').appendFileSync(${JSON.stringify(marker)}, process.argv.join(' ') + '\\n')
console.error('unexpected product-regression child command')
process.exit(91)
`
      writeExecutable(fakeGo, failIfCalled)
      writeExecutable(fakeNpm, failIfCalled)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-product-regression.mjs', '--json', '--skip-commands'],
        {
          cwd: process.cwd(),
          env: reportEnv({
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            GO: fakeGo
          }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(existsSync(marker)).toBe(false)
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-product-regression',
        status: 'skipped',
        passed: false,
        dryRunCannotAuthorizeCutover: true,
        expectedBlocked: true
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'temporary-evidence-exposure-guard',
        status: 'passed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'product-regression-matrix-snapshot',
        status: 'skipped',
        reason: 'dry run; product regression command was not executed'
      }))
      expect(
        checks
          .filter((check) => check.id !== 'temporary-evidence-exposure-guard')
          .every((check) => check.status === 'skipped')
      ).toBe(true)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('classifies temporary Go sources by anchored owner and exact production exclusion', () => {
    const result = spawnSync(
      process.execPath,
      ['scripts/runtime-go-product-regression.mjs', '--self-test-temporary-source-classification'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )

    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout)).toEqual({
      id: 'runtime-go-temporary-source-classification-self-test',
      status: 'passed',
      passed: true,
      cases: {
        productionProofAllowed: true,
        temporaryWithoutExclusionRejected: true,
        temporaryWithExclusionAllowed: true,
        temporaryConditionalExclusionRejected: true,
        exactValidationWithoutExclusionRejected: true
      }
    })
  })

  it('keeps sovereignty filename classification and the formal evaluator exception exact', () => {
    const result = spawnSync(
      process.execPath,
      ['scripts/scan-product-sovereignty.cjs', '--self-test-source-classification'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )

    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout)).toEqual({
      id: 'analytix-product-sovereignty-source-classification-self-test',
      status: 'passed',
      passed: true,
      cases: {
        productionBindingProofAllowed: true,
        standaloneProofRejected: true,
        standaloneOracleRejected: true,
        standaloneFakeRejected: true,
        numberedLegacyStageRejected: true,
        exactFormalEvaluatorAllowed: true,
        selectorAssignmentStillRejected: true,
        incompleteProcessCountStillRejected: true,
        collectKunCredentialsAgentsReadAllowed: true,
        collectKunCredentialsLocatorAllowed: true,
        inspectKunSettingsFileComparisonAllowed: true,
        cleanupKunSettingsFileComparisonAllowed: true,
        privateLegacyMigrationLocatorAllowed: true,
        anotherPathStillRejected: true,
        ordinaryAgentsKunUseStillRejected: true,
        windowKunStillRejected: true,
        kunAgentProviderStillRejected: true,
        kunEnvironmentStillRejected: true
      }
    })
  })

  it('keeps product regression reporting later checks after an early child failure', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-product-regression-no-fail-fast-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const marker = join(dir, 'commands.txt')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
fs.appendFileSync(${JSON.stringify(marker)}, 'go ' + args.join(' ') + '\\n')
if (args.includes('TestProductRegressionMatrixJSONSnapshot')) {
  console.log('ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON=' + JSON.stringify([{ feature: 'Go-only runtime boundary' }]))
}
if (args[0] === 'test' && args.includes('./...') && !args.includes('-tags')) {
  process.exit(29)
}
process.exit(0)
`)
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
fs.appendFileSync(${JSON.stringify(marker)}, 'npm ' + args.join(' ') + '\\n')
process.exit(0)
`)

      const result = spawnSync(process.execPath, ['scripts/runtime-go-product-regression.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>
      const commands = readFileSync(marker, 'utf8')

      expect(result.status).toBe(1)
      expect(result.stderr).toContain('[runtime-go-product-regression:progress] ')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-product-regression',
        status: 'failed',
        passed: false
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'runtime-go-contracts',
        status: 'failed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'product-sovereignty-scan',
        status: 'passed'
      }))
      expect(commands).toContain('tests/contracts.test.ts')
      expect(commands).toContain('scan:product-sovereignty')
      expect(commands).toContain('go test -p=1 -count=1 ./...')
      expect(commands).toContain('go test -p=1 -count=1 -tags analytix_prod ./...')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('fails product regression when a focused Go check selects no tests', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-product-regression-empty-go-selection-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakePython = join(dir, process.platform === 'win32' ? 'python.cmd' : 'python')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args.includes('TestProductRegressionMatrixJSONSnapshot')) {
  console.log('ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON=' + JSON.stringify([{ feature: 'Go-only runtime boundary' }]))
}
`)
      writeExecutable(fakeNpm, '#!/usr/bin/env node\n')
      writeExecutable(fakePython, '#!/usr/bin/env node\n')

      const result = spawnSync(process.execPath, ['scripts/runtime-go-product-regression.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
          PYTHON: fakePython
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>

      expect(result.status).toBe(1)
      expect(result.stderr).toContain(
        'production-product-regression-matrix did not execute any Go tests selected by -run'
      )
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'production-product-regression-matrix',
        status: 'failed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'product-sovereignty-scan',
        status: 'passed'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('reports a heartbeat while a JSON-mode product regression child is silent', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-product-regression-heartbeat-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakePython = join(dir, process.platform === 'win32' ? 'python.cmd' : 'python')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const args = process.argv.slice(2)
const runIndex = args.findIndex((arg) => arg === '-run' || arg === '-test.run')
const runPattern = runIndex >= 0 ? String(args[runIndex + 1] || '') : ''
if (args.includes('TestProductRegressionMatrixJSONSnapshot')) {
  setTimeout(() => {
    console.log('ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON=' + JSON.stringify([{ feature: 'Go-only runtime boundary' }]))
  }, 180)
}
if (args[0] === 'build' && args.includes('./internal/contracts')) {
  setTimeout(() => {}, 180)
}
if (runPattern.includes('TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite')) {
  for (const testName of [
    'TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite',
    'TestRuntimeServerAllowsRepeatedReadOnlyObservation'
  ]) {
    console.log(JSON.stringify({ Action: 'pass', Test: testName }))
  }
} else if (args.includes('-json') && runIndex >= 0 && !args.includes('TestProductRegressionMatrixJSONSnapshot')) {
  console.log(JSON.stringify({ Action: 'pass', Test: 'TestSyntheticFocusedSelection' }))
}
`)
      writeExecutable(fakeNpm, "#!/usr/bin/env node\nconsole.log('synthetic child output')\n")
      writeExecutable(fakePython, '#!/usr/bin/env node\n')

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-validation-command.mjs', 'product-regression', '--json'],
        {
          cwd: process.cwd(),
          env: reportEnv({
            ANALYTIX_RUNTIME_VALIDATION_HEARTBEAT_MS: '25',
            GO: fakeGo,
            NODE_ENV: 'test',
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            PYTHON: fakePython
          }),
          encoding: 'utf8',
          stdio: 'pipe',
          timeout: 15_000
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>
      const progress = result.stderr
        .split(/\r?\n/)
        .filter((line) => line.startsWith('[runtime-go-product-regression:progress] '))
        .map((line) => JSON.parse(line.slice('[runtime-go-product-regression:progress] '.length)))
      const startedCheckIds = progress
        .filter((event) => event.event === 'check_started')
        .map((event) => event.checkId)

      expect(result.status).toBe(0)
      expect(result.stderr).toContain('synthetic child output')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-product-regression',
        status: 'passed',
        passed: true
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_started',
        checkId: 'product-regression-matrix-snapshot'
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_heartbeat',
        checkId: 'product-regression-matrix-snapshot',
        elapsedMs: expect.any(Number),
        silentForMs: expect.any(Number)
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_finished',
        checkId: 'product-regression-matrix-snapshot',
        status: 'passed'
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_started',
        checkId: 'go-build-cache-maintenance-preflight'
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_heartbeat',
        checkId: 'go-build-cache-maintenance-preflight',
        elapsedMs: expect.any(Number),
        silentForMs: expect.any(Number)
      }))
      expect(progress).toContainEqual(expect.objectContaining({
        event: 'check_finished',
        checkId: 'go-build-cache-maintenance-preflight',
        status: 'passed'
      }))
      expect(startedCheckIds.indexOf('go-build-cache-maintenance-preflight'))
        .toBeLessThan(startedCheckIds.indexOf('product-regression-matrix-snapshot'))

      const wrapper = readFileSync('scripts/runtime-go-validation-command.mjs', 'utf8')
      expect(wrapper).toContain("import { spawn } from 'node:child_process'")
      expect(wrapper).not.toContain('spawnSync')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it('forwards termination through the product regression process tree', async () => {
    if (process.platform === 'win32') return

    const dir = mkdtempSync(join(tmpdir(), 'analytix-product-regression-signal-'))
    const fakeGo = join(dir, 'go')
    const fakeNpm = join(dir, 'npm')
    const fakePython = join(dir, 'python')
    const childPidPath = join(dir, 'child.pid')
    let childPid = 0
    let runner: ReturnType<typeof spawn> | undefined

    const processExists = (pid: number): boolean => {
      if (!pid) return false
      try {
        process.kill(pid, 0)
        return true
      } catch {
        return false
      }
    }
    const waitFor = async (predicate: () => boolean, timeoutMs: number): Promise<boolean> => {
      const deadline = Date.now() + timeoutMs
      while (Date.now() < deadline) {
        if (predicate()) return true
        await new Promise((resolvePromise) => setTimeout(resolvePromise, 25))
      }
      return predicate()
    }

    try {
      writeExecutable(fakeGo, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
if (args.includes('TestProductRegressionMatrixJSONSnapshot')) {
  fs.writeFileSync(${JSON.stringify(childPidPath)}, String(process.pid))
  setInterval(() => {}, 1000)
}
`)
      writeExecutable(fakeNpm, '#!/usr/bin/env node\n')
      writeExecutable(fakePython, '#!/usr/bin/env node\n')

      runner = spawn(
        process.execPath,
        ['scripts/runtime-go-validation-command.mjs', 'product-regression', '--json'],
        {
          cwd: process.cwd(),
          env: reportEnv({
            GO: fakeGo,
            NODE_ENV: 'test',
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            PYTHON: fakePython
          }),
          stdio: ['ignore', 'pipe', 'pipe']
        }
      )
      runner.stdout!.resume()
      runner.stderr!.resume()

      expect(await waitFor(() => existsSync(childPidPath), 5_000)).toBe(true)
      childPid = Number.parseInt(readFileSync(childPidPath, 'utf8'), 10)
      expect(processExists(childPid)).toBe(true)

      const closed = new Promise<{ status: number | null; signal: NodeJS.Signals | null }>((resolvePromise) => {
        runner?.once('close', (status, signal) => resolvePromise({ status, signal }))
      })
      runner.kill('SIGTERM')
      const outcome = await Promise.race([
        closed,
        new Promise<never>((_, reject) => {
          setTimeout(() => reject(new Error('validation wrapper did not exit after SIGTERM')), 5_000)
        })
      ])

      expect(outcome).toEqual({ status: 143, signal: null })
      expect(await waitFor(() => !processExists(childPid), 3_000)).toBe(true)
    } finally {
      if (runner && runner.exitCode === null && runner.signalCode === null) runner.kill('SIGKILL')
      if (processExists(childPid)) process.kill(childPid, 'SIGKILL')
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('skips speed/cache gate child commands when requested', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-speed-cache-skip-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const marker = join(dir, 'unexpected-command.txt')
      const failIfCalled = `#!/usr/bin/env node
require('node:fs').appendFileSync(${JSON.stringify(marker)}, process.argv.join(' ') + '\\n')
console.error('unexpected speed/cache child command')
process.exit(91)
`
      writeExecutable(fakeGo, failIfCalled)
      writeExecutable(fakeNpm, failIfCalled)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-speed-cache-gate.mjs', '--json', '--skip-commands'],
        {
          cwd: process.cwd(),
          env: reportEnv({
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            GO: fakeGo
          }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(existsSync(marker)).toBe(false)
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-speed-cache-gate',
        status: 'skipped',
        passed: false,
        dryRunCannotAuthorizeCutover: true,
        expectedBlocked: true
      }))
      expect(checks.length).toBeGreaterThan(0)
      expect(checks.every((check) => check.status === 'skipped')).toBe(true)
      expect(checks[0]).toEqual(expect.objectContaining({
        id: 'rc-structural-and-performance-suite',
        reason: 'dry run; speed/cache gate command was not executed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'provider-settings-resolution',
        reason: 'dry run; speed/cache gate command was not executed'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('separates structural correctness from the controlled p95 performance contract', () => {
    const source = readFileSync('scripts/runtime-go-performance-check.mjs', 'utf8')

    expect(source).not.toContain("'runtime-live-sse-first-safe-progress-bounded'")
    expect(source).toContain('const RC_WARMUP_COUNT = 2')
    expect(source).toContain('const RC_MEASURED_COUNT = 20')
    expect(source).toContain('const RC_FIRST_EVENT_P95_MS = 250')
    expect(source).toContain('nearestRank(measuredValues, 0.95)')
    expect(source).toContain('maxIsGateStatistic: false')
    expect(source).toContain("gateKind: 'structural-correctness'")
    expect(source).toContain('latencySloApplied: false')
    expect(source).toContain('buildCount: 1')
    expect(source).toContain('productSseContractTestsDelegatedToParent: true')
    expect(source).toContain('rcPerformanceAuthorization: passed && source.sourceBoundToHeadTree')
    expect(source).toContain('validWarmupCount === RC_WARMUP_COUNT')
    expect(source).toContain('validMeasuredCount === RC_MEASURED_COUNT')
    expect(source).toContain("sampleResult(report, index + 1, 'warmup')")
    expect(source).toContain("sampleResult(report, index + 1, 'measured')")

    const result = spawnSync(
      process.execPath,
      ['scripts/runtime-go-performance-check.mjs', '--self-test-rc-contract', '--json', '--gate'],
      { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' }
    )
    expect(result.status).toBe(0)
    expect(JSON.parse(result.stdout)).toEqual(expect.objectContaining({
      id: 'runtime-go-rc-performance-contract-self-test',
      status: 'passed',
      passed: true,
      warmupCount: 2,
      measuredCount: 20,
      thresholdMs: 250,
      p50: 10,
      p95: 19,
      porcelainPrefixPreserved: true
    }))
  })

  it('keeps speed/cache gate reporting later checks after an early child failure', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-speed-cache-no-fail-fast-'))
    try {
	      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
	      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
	      const marker = join(dir, 'commands.txt')
	      writeExecutable(fakeNpm, `#!/usr/bin/env node
	const fs = require('node:fs')
const args = process.argv.slice(2)
fs.appendFileSync(${JSON.stringify(marker)}, args.join(' ') + '\\n')
if (args[0] === 'run' && args[1] === 'test' && args.includes('src/main/runtime-sse-ipc.test.ts')) {
  process.exit(37)
}
	process.exit(0)
	`)
	      writeExecutable(fakeGo, `#!/usr/bin/env node
	const fs = require('node:fs')
	const args = process.argv.slice(2)
	fs.appendFileSync(${JSON.stringify(marker)}, 'go ' + args.join(' ') + '\\n')
	process.exit(0)
	`)

	      const result = spawnSync(process.execPath, ['scripts/runtime-go-speed-cache-gate.mjs', '--json'], {
	        cwd: process.cwd(),
	        env: reportEnv({
	          PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
	          GO: fakeGo
	        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = parseLastJSONObject(result.stdout)
      const checks = report.checks as Array<Record<string, unknown>>
      const commands = readFileSync(marker, 'utf8')

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-speed-cache-gate',
        status: 'failed',
        passed: false
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'main-renderer-streaming-contract',
        status: 'failed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'typescript-provider-cache-contract',
        status: 'passed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'go-mcp-schema-cache-contract',
        status: 'passed'
      }))
      expect(commands).toContain('src/main/runtime-sse-ipc.test.ts')
      expect(commands).toContain('tests/model-client.test.ts')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it('keeps deterministic packaged session soak from overwriting live evidence files', () => {
    const productRegression = readFileSync('scripts/runtime-go-product-regression.mjs', 'utf8')
    const packagedQA = readFileSync('scripts/runtime-go-packaged-qa.mjs', 'utf8')
    const localValidation = readFileSync('scripts/runtime-go-local-validation.mjs', 'utf8')

    expect(productRegression).toContain(
      "args: ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write']"
    )
    expect(packagedQA).toContain(
      "const packagedSessionSoakArgs = ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write']"
    )
    expect(localValidation).toContain(
      "args: ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write']"
    )
  })

  it('writes packaged session soak evidence to the requested output path', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-packaged-soak-output-'))
    try {
      const outputPath = join(dir, 'packaged-session-soak.json')
      const report = runReportWithoutDryRun('scripts/runtime-go-packaged-session-soak.mjs', [
        '--no-gate',
        '--output',
        outputPath
      ])

      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-session-soak',
        outputPath,
        status: 'passed',
        passed: true
      }))
      expect(existsSync(outputPath)).toBe(true)
      const written = JSON.parse(readFileSync(outputPath, 'utf8')) as Record<string, unknown>
      expect(written).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-session-soak',
        outputPath,
        status: 'passed',
        passed: true
      }))
      expect(written.soak).toEqual(expect.objectContaining({
        deterministicContractOnly: true,
        actualPackagedAppEvidenceRequiredForFinalGate: true
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 300_000)

  it('serializes packaged session soak owners without merging Go test processes', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-packaged-soak-serial-'))
    try {
      const expectedGoTests = [
        'TestRuntimeServerForkAndResumePreserveToolPairingHistory',
        'TestRuntimeServerForkTurnIDTruncatesAndRewritesHistory',
        'TestRuntimeServerThreadSummarySearchAndForkCountsMatchProductContract',
        'TestRuntimeServerSSEReplayUsesLastEventIDWhenSinceSeqOmitted',
        'TestRuntimeServerSubagentContinueAndForkUseDurableTranscript',
        'TestRuntimeServerCandidateDurableRootHighRiskFailsClosedAndSurvivesRestart'
      ]
      const goComplete = join(dir, 'go-complete')
      const rollbackStarted = join(dir, 'rollback-started')
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
if (args[0] === 'test' && args[1] === '-c') {
  process.exit(0)
}
const runIndex = args.indexOf('-test.run')
const testName = String(args[runIndex + 1] || '').replace(/^\\^|\\$$/g, '')
if (fs.existsSync(${JSON.stringify(rollbackStarted)})) process.exit(79)
if (testName === ${JSON.stringify('TestRuntimeServerCandidateDurableRootHighRiskFailsClosedAndSurvivesRestart')}) {
  fs.writeFileSync(${JSON.stringify(goComplete)}, 'complete')
}
console.log(JSON.stringify({ Action: 'pass', Package: 'analytix.local/runtime-go', Test: testName }))
process.exit(0)
`)
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const fs = require('node:fs')
if (!fs.existsSync(${JSON.stringify(goComplete)})) process.exit(78)
fs.writeFileSync(${JSON.stringify(rollbackStarted)}, 'started')
process.exit(0)
`)

      const result = spawnSync(process.execPath, [
        'scripts/runtime-go-packaged-session-soak.mjs',
        '--json',
        '--no-gate',
        '--no-write'
      ], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(report.checks).toEqual([
        expect.objectContaining({
          id: 'go-session-durable-contract',
          status: 'passed',
          subchecks: expect.arrayContaining(expectedGoTests.map((id) =>
            expect.objectContaining({ id, status: 'passed' })
          ))
        }),
        expect.objectContaining({
          id: 'typescript-retired-backend-session-contract',
          status: 'passed'
        })
      ])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('keeps packaged session soak json parseable when a child command fails', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-packaged-soak-json-failure-'))
    try {
      const outputPath = join(dir, 'packaged-session-soak.json')
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      writeExecutable(fakeGo, `#!/usr/bin/env node
console.log('child stdout that must stay out of the parent JSON report')
console.error('child stderr that must stay out of the parent stderr')
process.exit(91)
`)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-packaged-session-soak.mjs', '--json', '--no-gate', '--output', outputPath],
        {
          cwd: process.cwd(),
          env: reportEnv({
            GO: fakeGo,
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`
          }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(result.stdout).not.toContain('child stdout')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-session-soak',
        status: 'failed',
        passed: false,
        outputPath
      }))
      expect(report.checks[0]).toEqual(expect.objectContaining({
        id: 'go-session-durable-contract',
        status: 'failed',
        exitStatus: 91
      }))
      expect(existsSync(outputPath)).toBe(true)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('requires runtime health smoke in the Go default candidate and cutover reports', () => {
    const candidateReport = runReport('scripts/runtime-go-default-readiness-report.mjs')
    const candidateChecks = candidateReport.checks as Array<Record<string, unknown>>
    const defaultReadiness = candidateReport.defaultReadiness as Record<string, unknown>

    expect(candidateReport).toEqual(expect.objectContaining({
      id: 'runtime-go-default-readiness-report',
      status: 'skipped',
      passed: false,
      goDefaultReady: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      notFinalUntil: expect.arrayContaining([
        'run with --gate so preflight final gate is executed'
      ])
    }))
    expect(candidateChecks.map((item) => item.id)).toContain('runtime-health-smoke')
    expect(defaultReadiness).toEqual(expect.objectContaining({
      runtimeHealthPassed: false,
      runtimeHealthBoundaryEvidenceValid: false,
      runtimeHealthActualProcessStarted: false,
      runtimeHealthProductionRuntime: false,
      runtimeHealthUsesTemporaryDataDir: false,
      runtimeHealthUsesRealProviderConfig: false,
      runtimeHealthProviderConfigMode: 'not-run',
      runtimeHealthUsesIsolatedEmptyProviderConfig: false,
      runtimeHealthUsesIsolatedContractProviderConfig: false,
      runtimeHealthProductChainMode: 'not-run',
      runtimeHealthProductChainOk: false,
      runtimeHealthAuthorityBoundaryOk: false,
      runtimeHealthHostAcceptedFinalOk: false,
      runtimeHealthProviderPositiveChainOk: false,
      runtimeHealthProviderAgentLoopReady: false,
      runtimeHealthProviderAgentLoopCoverage: 'not-run',
      runtimeHealthProviderExecutionExpected: false,
      runtimeHealthProviderExecutionBlocked: false,
      runtimeHealthTurnCreateOk: false,
      runtimeHealthSseReplayOk: false,
      runtimeHealthUsageOk: false,
      runtimeHealthProviderUsageObserved: false,
      runtimeHealthProviderUsageAbsentOk: false,
      runtimeHealthAttachmentOk: false,
      runtimeHealthContractProviderRequestCount: null,
      runtimeHealthActualPackagedAppLaunched: false,
      runtimeHealthRuntimeInfoOk: false,
      runtimeHealthRuntimeToolsOk: false,
      runtimeHealthProductionCapabilitiesOk: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      notFinalUntil: expect.arrayContaining([
        'run with --gate so preflight final gate is executed'
      ])
    }))

    const cutoverReport = runReport('scripts/runtime-go-cutover-report.mjs')
    const cutover = cutoverReport.cutover as Record<string, unknown>
    expect(cutoverReport).toEqual(expect.objectContaining({
      id: 'runtime-go-cutover-report',
      status: 'skipped',
      passed: false,
      goRuntimeDefaultCutoverReady: false,
      deterministicCutoverReady: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      finalStatus: 'blocked'
    }))
    expect(cutover).toEqual(expect.objectContaining({
      runtimeHealthPassed: false,
      runtimeHealthBoundaryEvidenceValid: false,
      runtimeHealthActualProcessStarted: false,
      runtimeHealthProductionRuntime: false,
      runtimeHealthUsesTemporaryDataDir: false,
      runtimeHealthUsesRealProviderConfig: false,
      runtimeHealthProviderConfigMode: 'not-run',
      runtimeHealthUsesIsolatedEmptyProviderConfig: false,
      runtimeHealthUsesIsolatedContractProviderConfig: false,
      runtimeHealthProductChainMode: 'not-run',
      runtimeHealthProductChainOk: false,
      runtimeHealthAuthorityBoundaryOk: false,
      runtimeHealthHostAcceptedFinalOk: false,
      runtimeHealthProviderPositiveChainOk: false,
      runtimeHealthProviderAgentLoopReady: false,
      runtimeHealthProviderAgentLoopCoverage: 'not-run',
      runtimeHealthProviderExecutionExpected: false,
      runtimeHealthProviderExecutionBlocked: false,
      runtimeHealthTurnCreateOk: false,
      runtimeHealthSseReplayOk: false,
      runtimeHealthUsageOk: false,
      runtimeHealthProviderUsageObserved: false,
      runtimeHealthProviderUsageAbsentOk: false,
      runtimeHealthAttachmentOk: false,
      runtimeHealthContractProviderRequestCount: null,
      runtimeHealthActualPackagedAppLaunched: false,
      runtimeHealthRuntimeInfoOk: false,
      runtimeHealthRuntimeToolsOk: false,
      runtimeHealthProductionCapabilitiesOk: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      finalStatus: 'blocked',
      missingFinalCoverage: expect.arrayContaining([
        'credentialed-provider',
        'production-authority-enrolled-provider-agent-chain',
        'credentialed-mcp',
        'packaged-desktop-qa',
        'operator-gate',
        'typescript-retired-backend'
      ])
    }))
  })

  it('allows formal cutover coverage to be satisfied by current readiness env names', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-final-coverage-'))
    try {
      const provider = join(dir, 'provider.json')
      const mcp = join(dir, 'mcp.json')
      const packaged = join(dir, 'packaged.json')
      const operator = join(dir, 'operator.json')
	      writeJSON(provider, providerEvidence([
	        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
	        { id: 'openai-compatible', status: 'passed', credentialed: true, skipped: false },
	        { id: 'anthropic-compatible', status: 'skipped', credentialed: true, skipped: true },
	        { id: 'custom-endpoint', status: 'skipped', credentialed: true, skipped: true }
	      ], { readsRealApiKeysByDefault: true }))
      writeJSON(mcp, mcpEvidence())
      writeJSON(packaged, {
        id: 'runtime-go-packaged-qa',
        status: 'passed',
        passed: true,
        checks: [
          'packaged-app-startup',
          'health',
          'runtime-info',
          'thread-list',
          'turn-create',
          'sse-replay',
          'go-runtime-default-gate',
          'typescript-retired-backend'
        ].map((id) => ({ id, status: 'passed' }))
      })
      writeJSON(operator, operatorEvidence())

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: provider,
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: mcp,
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: packaged,
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_READY_EVIDENCE: operator
      })
      const cutover = report.cutover as Record<string, unknown>
      const finalCoverage = report.finalCoverage as Array<Record<string, unknown>>
      const covered = finalCoverage.filter((item) => item.status === 'covered').map((item) => item.id)

      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-cutover-report',
        status: 'skipped',
        finalAcceptanceReady: false,
        finalAcceptanceStatus: 'blocked',
        finalStatus: 'blocked'
      }))
      expect(covered).toEqual(expect.arrayContaining([
        'durable-restart',
        'credentialed-provider',
        'credentialed-mcp',
        'packaged-desktop-qa',
        'operator-gate'
      ]))
      expect(cutover.missingFinalCoverage).toEqual([
        'production-authority-enrolled-provider-agent-chain',
        'typescript-retired-backend'
      ])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('ignores retired numbered final-coverage env names', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-retired-final-env-'))
    try {
      const provider = join(dir, 'provider.json')
      writeJSON(provider, providerEvidence([
        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
        { id: 'openai-compatible', status: 'passed', credentialed: true, skipped: false }
      ]))
      const retiredPrefix = ['ANALYTIX', ['D', '0243'].join('')].join('_')
      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        [`${retiredPrefix}_PROVIDER_MATRIX_STATUS`]: 'passed',
        [`${retiredPrefix}_PROVIDER_MATRIX_STATUS_EVIDENCE`]: provider
      })
      const providerRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'credentialed-provider')

      expect(providerRow).toEqual(expect.objectContaining({
        id: 'credentialed-provider',
        status: 'missing',
        statusValue: 'missing',
        evidencePresent: false,
        reason: 'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS is not passed'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('carries the protected boundary and ordinary provider evidence into the default readiness report', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-default-product-chain-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const healthSmoke = firstRunnableHealthReport()
      const productRegression = {
        id: 'runtime-go-product-regression',
        status: 'passed',
        matrix: {
          rowCount: 33,
          features: ['TypeScript retired backend']
        },
        retirement: {
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      const speedCache = {
        id: 'runtime-go-speed-cache-gate',
        status: 'passed',
        checks: [{ id: 'first-runtime-event-fixture', status: 'passed' }]
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:health-smoke') {
  console.log(${JSON.stringify(JSON.stringify(healthSmoke))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:product-regression') {
  console.log(${JSON.stringify(JSON.stringify(productRegression))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:speed-cache-gate') {
  console.log(${JSON.stringify(JSON.stringify(speedCache))})
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const report = runReportWithoutDryRun('scripts/runtime-go-default-readiness-report.mjs', [], {
        PATH: `${dir}${delimiter}${process.env.PATH || ''}`
      })
      const runtimeHealth = report.runtimeHealth as Record<string, unknown>
      const defaultReadiness = report.defaultReadiness as Record<string, unknown>

      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-default-readiness-report',
        status: 'passed',
        passed: true,
        deterministicEvidenceOnly: true
      }))
      expect(runtimeHealth).toEqual(expect.objectContaining({
        runtimeIdentityEvidenceValid: true,
        boundaryEvidenceValid: true,
        providerAgentChainEvidenceValid: true,
        providerConfigMode: 'isolated-contract-provider-config',
        usesRealProviderConfig: false,
        usesIsolatedContractProviderConfig: true,
        usesIsolatedEmptyProviderConfig: false,
        productChainMode: 'production-authority-enrolled-provider-agent-chain',
        productChainOk: true,
        authorityBoundaryOk: true,
        hostAcceptedFinalOk: true,
        providerPositiveChainOk: true,
        providerAgentLoopReady: true,
        providerAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
        providerExecutionExpected: true,
        providerExecutionBlocked: false,
        protectedTurnCreateOk: true,
        ordinaryTurnCreateOk: true,
        protectedSseReplayOk: true,
        ordinarySseReplayOk: true,
        ordinaryThreadDistinct: true,
        ordinaryPublicFinalOk: true,
        turnCreateOk: true,
        sseReplayOk: true,
        usageOk: true,
        providerUsageObserved: true,
        providerUsageAbsentOk: true,
        protectedProviderDispatchAbsent: true,
        protectedProviderRequestCount: 0,
        attachmentOk: true,
        contractProviderRequestCount: 1,
        contractProviderPromptSeen: true,
        providerAuthorizationConfigured: true
      }))
      expect(defaultReadiness).toEqual(expect.objectContaining({
        runtimeHealthRuntimeIdentityEvidenceValid: true,
        runtimeHealthBoundaryEvidenceValid: true,
        runtimeHealthProviderAgentChainEvidenceValid: true,
        runtimeHealthProviderConfigMode: 'isolated-contract-provider-config',
        runtimeHealthUsesRealProviderConfig: false,
        runtimeHealthUsesIsolatedContractProviderConfig: true,
        runtimeHealthProductChainMode: 'production-authority-enrolled-provider-agent-chain',
        runtimeHealthProductChainOk: true,
        runtimeHealthAuthorityBoundaryOk: true,
        runtimeHealthHostAcceptedFinalOk: true,
        runtimeHealthProviderPositiveChainOk: true,
        runtimeHealthProviderAgentLoopReady: true,
        runtimeHealthProviderAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
        runtimeHealthProviderExecutionExpected: true,
        runtimeHealthProviderExecutionBlocked: false,
        runtimeHealthProtectedTurnCreateOk: true,
        runtimeHealthOrdinaryTurnCreateOk: true,
        runtimeHealthProtectedSseReplayOk: true,
        runtimeHealthOrdinarySseReplayOk: true,
        runtimeHealthOrdinaryThreadDistinct: true,
        runtimeHealthOrdinaryPublicFinalOk: true,
        runtimeHealthTurnCreateOk: true,
        runtimeHealthSseReplayOk: true,
        runtimeHealthUsageOk: true,
        runtimeHealthProviderUsageObserved: true,
        runtimeHealthProviderUsageAbsentOk: true,
        runtimeHealthProviderDraftWithheld: true,
        runtimeHealthProtectedProviderDispatchAbsent: true,
        runtimeHealthProtectedProviderRequestCount: 0,
        runtimeHealthAttachmentOk: true,
        runtimeHealthContractProviderRequestCount: 1,
        runtimeHealthContractProviderPromptSeen: true,
        runtimeHealthProviderAuthorizationConfigured: true
      }))
      expect(defaultReadiness.notes).toEqual(expect.arrayContaining([
        expect.stringContaining('protected Funds-unavailable turn'),
        expect.stringContaining('distinct ordinary turn')
      ]))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it.each([
    ['stale schema', (report: Record<string, unknown>) => { report.schemaVersion = 1 }, 'product-chain'],
    ['provider dispatch during the protected boundary', (report: Record<string, unknown>) => {
      const smoke = report.smoke as Record<string, unknown>
      smoke.protectedProviderRequestCount = 1
      smoke.protectedProviderDispatchAbsent = false
    }, 'authority-boundary'],
    ['missing exact ordinary prompt marker', (report: Record<string, unknown>) => {
      const smoke = report.smoke as Record<string, unknown>
      smoke.contractProviderPromptSeen = false
    }, 'provider Agent-chain'],
    ['runtime process not started', (report: Record<string, unknown>) => {
      const smoke = report.smoke as Record<string, unknown>
      smoke.actualRuntimeProcessStarted = false
    }, 'runtime identity'],
    ['temporary data scope missing', (report: Record<string, unknown>) => {
      const smoke = report.smoke as Record<string, unknown>
      delete smoke.dataDirScope
    }, 'runtime identity'],
    ['runtime info health flipped', (report: Record<string, unknown>) => {
      const smoke = report.smoke as Record<string, unknown>
      smoke.runtimeInfoOk = false
    }, 'runtime identity'],
    ['runtime build check removed', (report: Record<string, unknown>) => {
      report.checks = (report.checks as Array<Record<string, unknown>>)
        .filter((item) => item.id !== 'runtime-server-build')
    }, 'runtime identity'],
    ['runtime cleanup check failed', (report: Record<string, unknown>) => {
      const cleanup = (report.checks as Array<Record<string, unknown>>)
        .find((item) => item.id === 'runtime-server-cleanup')
      if (cleanup) cleanup.status = 'failed'
    }, 'runtime identity']
  ])('rejects %s as current runtime product-chain evidence', (_name, mutate, reasonFragment) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-default-boundary-reject-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const healthSmoke = firstRunnableHealthReport()
      mutate(healthSmoke)
      const productRegression = {
        id: 'runtime-go-product-regression',
        status: 'passed',
        matrix: { rowCount: 33, features: ['TypeScript retired backend'] },
        retirement: {
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      const speedCache = {
        id: 'runtime-go-speed-cache-gate',
        status: 'passed',
        checks: [{ id: 'first-runtime-event-fixture', status: 'passed' }]
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:health-smoke') {
  console.log(${JSON.stringify(JSON.stringify(healthSmoke))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:product-regression') {
  console.log(${JSON.stringify(JSON.stringify(productRegression))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:speed-cache-gate') {
  console.log(${JSON.stringify(JSON.stringify(speedCache))})
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      const result = spawnSync(process.execPath, [
        'scripts/runtime-go-default-readiness-report.mjs',
        '--json'
      ], {
        cwd: process.cwd(),
        env: reportEnv({ PATH: `${dir}${delimiter}${process.env.PATH || ''}` }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(result.status).toBe(1)
      const report = parseLastJSONObject(result.stdout)
      const healthCheck = (report.checks as Array<Record<string, unknown>>)
        .find((item) => item.id === 'runtime-health-smoke')
      expect(healthCheck).toEqual(expect.objectContaining({
        status: 'failed',
        reason: expect.stringContaining(reasonFragment)
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('routes candidate gate mode through preflight final blockers', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-candidate-preflight-gate-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const healthSmoke = firstRunnableHealthReport()
      const productRegression = {
        id: 'runtime-go-product-regression',
        status: 'passed',
        matrix: {
          rowCount: 33,
          features: ['TypeScript retired backend']
        },
        retirement: {
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      const speedCache = {
        id: 'runtime-go-speed-cache-gate',
        status: 'passed',
        checks: [{ id: 'first-runtime-event-fixture', status: 'passed' }]
      }
      const preflight = {
        id: 'runtime-go-preflight',
        status: 'live_blocked',
        localChecksStatus: 'passed',
        gateMode: true,
        finalGateBlocked: true,
        finalGateBlockers: [
          'worktree:tracked-generated-artifacts',
          'worktree:non-goal-dirty-files'
        ],
        finalGateBlockerSummary: {
          schemaVersion: 1,
          dryRun: [],
          externalInput: [],
          operator: [],
          evidenceHygiene: [],
          worktreeHygiene: [
            'worktree:tracked-generated-artifacts',
            'worktree:non-goal-dirty-files'
          ],
          deterministic: [],
          other: [],
          requiresExternalInput: false,
          requiresOperatorApproval: false,
          requiresLocalHygiene: true,
          requiresDeterministicFix: false
        },
        operatorDependencySummary: {
          schemaVersion: 1,
          envGate: {
            runtimeReady: true,
            operatorApprovesDefault: false
          },
          evidence: {
            providerPassed: true,
            mcpPassed: true,
            packagedPassed: true,
            credentialedEvidenceReviewed: false
          },
          commitBinding: {
            required: false,
            relevantEvidenceClean: false,
            bound: true
          },
          blockers: ['operator post-cutover live-validation approval env is not set']
        },
        operatorCurrentEnvGate: {
          runtimeReady: true,
          operatorApprovesDefault: false
        }
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:health-smoke') {
  console.log(${JSON.stringify(JSON.stringify(healthSmoke))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:product-regression') {
  console.log(${JSON.stringify(JSON.stringify(productRegression))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:speed-cache-gate') {
  console.log(${JSON.stringify(JSON.stringify(speedCache))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:preflight') {
  console.log(${JSON.stringify(JSON.stringify(preflight))})
  process.exit(1)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-default-readiness-report.mjs', '--json', '--gate'],
        {
          cwd: process.cwd(),
          env: reportEnv({ PATH: `${dir}${delimiter}${process.env.PATH || ''}` }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-default-readiness-report',
        status: 'live_blocked',
        passed: false,
        goDefaultReady: false,
        deterministicDefaultReady: true,
        gateMode: true,
        finalGateEnabled: true,
        finalGateBlocked: true,
        finalGateBlockers: [
          'worktree:tracked-generated-artifacts',
          'worktree:non-goal-dirty-files'
        ]
      }))
      expect(report.preflightGate).toEqual(expect.objectContaining({
        status: 'live_blocked',
        reportId: 'runtime-go-preflight',
        reportStatus: 'live_blocked',
        localChecksStatus: 'passed',
        gateMode: true,
        finalGateBlocked: true,
        finalGateBlockers: [
          'worktree:tracked-generated-artifacts',
          'worktree:non-goal-dirty-files'
        ],
        operatorDependencySummary: expect.objectContaining({
          envGate: {
            runtimeReady: true,
            operatorApprovesDefault: false
          },
          blockers: ['operator post-cutover live-validation approval env is not set']
        }),
        operatorCurrentEnvGate: {
          runtimeReady: true,
          operatorApprovesDefault: false
        },
        command: 'npm run runtime:go:preflight -- --json --gate'
      }))
      expect(report.preflightGate.finalGateBlockerSummary).toEqual(expect.objectContaining({
        worktreeHygiene: [
          'worktree:tracked-generated-artifacts',
          'worktree:non-goal-dirty-files'
        ],
        requiresLocalHygiene: true
      }))
      expect(report.defaultReadiness).toEqual(expect.objectContaining({
        deterministicDefaultReady: true,
        finalGateEnabled: true,
        finalGateBlocked: true,
        finalGateBlockerSummary: expect.objectContaining({
          worktreeHygiene: [
            'worktree:tracked-generated-artifacts',
            'worktree:non-goal-dirty-files'
          ],
          requiresLocalHygiene: true
        }),
        preflightGateStatus: 'live_blocked'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('reports deterministic candidate blockers before running the final preflight gate', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-candidate-deterministic-blocker-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const healthSmoke = firstRunnableHealthReport()
      const productRegression = {
        id: 'runtime-go-product-regression',
        status: 'failed',
        matrix: { rowCount: 33, features: ['TypeScript retired backend'] },
        retirement: {
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 1,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:health-smoke') {
  console.log(${JSON.stringify(JSON.stringify(healthSmoke))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:product-regression') {
  console.log(${JSON.stringify(JSON.stringify(productRegression))})
  process.exit(1)
}
if (args[0] === 'run' && args[1] === 'runtime:go:preflight') {
  console.error('preflight must not run when deterministic candidate checks fail')
  process.exit(1)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-default-readiness-report.mjs', '--json', '--gate'],
        {
          cwd: process.cwd(),
          env: reportEnv({ PATH: `${dir}${delimiter}${process.env.PATH || ''}` }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(1)
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-default-readiness-report',
        status: 'failed',
        passed: false,
        deterministicDefaultReady: false,
        gateMode: true,
        finalGateEnabled: true,
        finalGateBlocked: true,
        finalGateBlockers: [
          'deterministic-default-readiness-not-passed',
          'deterministic-check:product-regression'
        ],
        deterministicGateBlockers: [
          'deterministic-default-readiness-not-passed',
          'deterministic-check:product-regression'
        ],
        finalGateBlockerSummary: expect.objectContaining({
          deterministic: [
            'deterministic-default-readiness-not-passed',
            'deterministic-check:product-regression'
          ],
          requiresDeterministicFix: true
        })
      }))
      expect(report.preflightGate).toEqual(expect.objectContaining({
        status: 'not_run',
        finalGateBlocked: false,
        finalGateBlockers: [],
        reason: 'deterministic checks did not pass; preflight final gate was not run'
      }))
      expect(report.defaultReadiness).toEqual(expect.objectContaining({
        deterministicDefaultReady: false,
        finalGateBlocked: true,
        finalGateBlockerSummary: expect.objectContaining({
          deterministic: [
            'deterministic-default-readiness-not-passed',
            'deterministic-check:product-regression'
          ],
          requiresDeterministicFix: true
        }),
        deterministicGateBlockers: [
          'deterministic-default-readiness-not-passed',
          'deterministic-check:product-regression'
        ],
        preflightGateStatus: 'not_run'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('runs candidate deterministic child gates in JSON mode', () => {
    const source = readFileSync(join(process.cwd(), 'scripts/runtime-go-default-readiness-report.mjs'), 'utf8')

    expect(source).toContain("args: ['run', 'runtime:go:product-regression', '--', '--json']")
    expect(source).toContain("args: ['run', 'runtime:go:speed-cache-gate', '--', '--json']")
  })

  it('does not leak operator gate env into deterministic candidate child checks', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-candidate-env-isolation-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const healthSmoke = firstRunnableHealthReport()
      const productRegression = {
        id: 'runtime-go-product-regression',
        status: 'passed',
        matrix: { rowCount: 33, features: ['TypeScript retired backend'] },
        retirement: {
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      const speedCache = {
        id: 'runtime-go-speed-cache-gate',
        status: 'passed',
        checks: [{ id: 'first-runtime-event-fixture', status: 'passed' }]
      }
      const preflight = {
        id: 'runtime-go-preflight',
        status: 'live_blocked',
        localChecksStatus: 'passed',
        gateMode: true,
        finalGateBlocked: true,
        finalGateBlockers: ['provider:at-least-one-non-deepseek-provider'],
        finalGateBlockerSummary: {
          schemaVersion: 1,
          dryRun: [],
          externalInput: ['provider:at-least-one-non-deepseek-provider'],
          operator: [],
          evidenceHygiene: [],
          worktreeHygiene: [],
          deterministic: [],
          other: [],
          requiresExternalInput: true,
          requiresNonDeepSeekProvider: true,
          providerGateDetail: {
            present: true,
            evidencePath: '/tmp/provider-matrix.json',
            candidateProviderIds: ['openai-compatible', 'anthropic-compatible', 'custom-endpoint'],
            candidates: [
              {
                id: 'openai-compatible',
                status: 'skipped',
                missingEnv: [
                  'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
                  'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
                  'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
                ],
                missingSettingsProfileFields: ['provider.providers[].matchingProfile']
              }
            ],
            acceptedSettingsProfilePath: 'provider.providers[]',
            settingsProfileCoverage: {
              status: 'loaded',
              source: 'default-app-settings',
              providerCount: 1,
              matchedProviderIds: ['deepseek'],
              usableProviderIds: ['deepseek'],
              usableNonDeepSeekProviderIds: [],
              deepseekSettingsProfileUsable: true,
              nonDeepSeekSettingsProfileUsable: false,
              satisfiesFinalProviderCoverageFromSettings: false,
              credentialValuesRecorded: false
            },
            credentialedDeepSeekPassed: true,
            credentialedNonDeepSeekProviderIds: [],
            requiresNonDeepSeekProvider: true
          },
          requiresOperatorApproval: false,
          requiresLocalHygiene: false,
          requiresDeterministicFix: false
        },
        operatorCurrentEnvGate: {
          runtimeReady: true,
          operatorApprovesDefault: true
        }
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && ['runtime:go:health-smoke', 'runtime:go:product-regression', 'runtime:go:speed-cache-gate'].includes(args[1])) {
  if (process.env.ANALYTIX_RUNTIME_READY ||
    process.env.ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT ||
    process.env.ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS ||
    process.env.ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS ||
    process.env.ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE) {
    console.error('operator/final-gate env leaked into deterministic child check')
    process.exit(1)
  }
  if (args[1] === 'runtime:go:health-smoke') console.log(${JSON.stringify(JSON.stringify(healthSmoke))})
  if (args[1] === 'runtime:go:product-regression') console.log(${JSON.stringify(JSON.stringify(productRegression))})
  if (args[1] === 'runtime:go:speed-cache-gate') console.log(${JSON.stringify(JSON.stringify(speedCache))})
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:preflight') {
  if (process.env.ANALYTIX_RUNTIME_READY !== '1' || process.env.ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT !== '1') {
    console.error('operator env missing from preflight gate')
    process.exit(1)
  }
  console.log(${JSON.stringify(JSON.stringify(preflight))})
  process.exit(1)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-default-readiness-report.mjs', '--json', '--gate'],
        {
          cwd: process.cwd(),
          env: reportEnv({
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            ANALYTIX_RUNTIME_READY: '1',
            ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1',
            ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
            ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS: 'passed',
            ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE: 'passed'
          }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        status: 'live_blocked',
        deterministicDefaultReady: true,
        finalGateBlockers: ['provider:at-least-one-non-deepseek-provider'],
        finalGateBlockerSummary: expect.objectContaining({
          externalInput: ['provider:at-least-one-non-deepseek-provider'],
          requiresExternalInput: true,
          requiresNonDeepSeekProvider: true,
          providerGateDetail: expect.objectContaining({
            present: true,
            credentialedDeepSeekPassed: true,
            credentialedNonDeepSeekProviderIds: [],
            requiresNonDeepSeekProvider: true,
            settingsProfileCoverage: expect.objectContaining({
              providerCount: 1,
              usableProviderIds: ['deepseek'],
              usableNonDeepSeekProviderIds: []
            }),
            candidates: expect.arrayContaining([
              expect.objectContaining({
                id: 'openai-compatible',
                missingEnv: expect.arrayContaining(['ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY'])
              })
            ])
          })
        })
      }))
      expect(report.preflightGate).toEqual(expect.objectContaining({
        status: 'live_blocked',
        gateMode: true,
        finalGateBlocked: true,
        operatorCurrentEnvGate: {
          runtimeReady: true,
          operatorApprovesDefault: true
        }
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('does not leak operator gate env into preflight local checks', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-env-isolation-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const liveEvidence = join(dir, 'live-evidence-report.json')
      writeFileSync(liveEvidence, JSON.stringify({
        schemaVersion: 1,
        id: 'runtime-go-live-evidence-report',
        status: 'live_blocked',
        passed: false,
        missingExternalInputs: [
          { id: 'operator:gate', reason: 'operator approval missing' }
        ]
      }), 'utf8')
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && ['runtime:go:product-regression', 'runtime:go:speed-cache-gate', 'typecheck', 'build:runtime'].includes(args[1])) {
  if (process.env.ANALYTIX_RUNTIME_READY || process.env.ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT) {
    console.error('operator env leaked into preflight local check ' + args[1])
    process.exit(1)
  }
  console.log(JSON.stringify({ id: args[1], status: 'passed' }))
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'diff' && args[1] === '--check') process.exit(0)
if (args[0] === 'status' && args[1] === '--porcelain=v1') process.exit(0)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--json', '--gate', '--live-evidence-json', liveEvidence],
        {
          cwd: process.cwd(),
          env: reportEnv({
            PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
            ANALYTIX_RUNTIME_READY: '1',
            ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
          }),
          encoding: 'utf8',
          stdio: 'pipe'
        }
      )
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-preflight',
        status: 'live_blocked',
        localChecksStatus: 'passed',
        gateMode: true,
        finalGateBlocked: true,
        finalGateBlockers: ['operator:gate'],
        operatorCurrentEnvGate: {
          runtimeReady: true,
          operatorApprovesDefault: true
        }
      }))
      expect(report.finalGateBlockerSummary).toEqual(expect.objectContaining({
        requiresOperatorApproval: false,
        requiresOperatorEvidenceRefresh: true,
        operatorBlockedByDependencies: false,
        operatorGateDetail: expect.objectContaining({
          present: true,
          requiresOperatorApproval: false,
          requiresOperatorEvidenceRefresh: true,
          blockedByDependencies: false,
          currentEnvGate: {
            runtimeReady: true,
            operatorApprovesDefault: true
          }
        })
      }))
      expect(report.checks).toEqual(expect.arrayContaining([
        expect.objectContaining({ id: 'product-regression', status: 'passed' }),
        expect.objectContaining({ id: 'speed-cache-gate', status: 'passed' }),
        expect.objectContaining({ id: 'typecheck', status: 'passed' }),
        expect.objectContaining({ id: 'build-runtime', status: 'passed' }),
        expect.objectContaining({ id: 'diff-check', status: 'passed' })
      ]))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('accepts current aggregate packaged QA evidence for final coverage', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-packaged-aggregate-'))
    try {
      const packaged = join(dir, 'packaged.json')
      writeJSON(packaged, packagedAggregateEvidence())

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: packaged
      })
      const packagedRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'packaged-desktop-qa')

      expect(packagedRow).toEqual(expect.objectContaining({
        id: 'packaged-desktop-qa',
        status: 'covered',
        reason: 'credentialed evidence accepted'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('rejects aggregate packaged QA evidence that still lists packaged desktop QA as missing', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-packaged-aggregate-missing-'))
    try {
      const packaged = join(dir, 'packaged.json')
      writeJSON(packaged, packagedAggregateEvidence({
        packaged: {
          missingFinalCoverage: ['packaged-desktop-qa'],
          actualPackagedDesktopQAPassed: false
        }
      }))

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: packaged
      })
      const packagedRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'packaged-desktop-qa')

      expect(packagedRow).toEqual(expect.objectContaining({
        id: 'packaged-desktop-qa',
        status: 'missing',
        reason: expect.stringContaining('packaged.missingFinalCoverage:packaged-desktop-qa')
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

	  it('does not accept live provider evidence without a credentialed non-DeepSeek provider', () => {
	    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-provider-nondeepseek-'))
	    try {
      const provider = join(dir, 'provider.json')
      writeJSON(provider, providerEvidence([
        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
        { id: 'openai-compatible', status: 'skipped', credentialed: true, skipped: true },
        { id: 'anthropic-compatible', status: 'skipped', credentialed: true, skipped: true },
        { id: 'custom-endpoint', status: 'skipped', credentialed: true, skipped: true }
      ]))

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: provider
      })
      const providerRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'credentialed-provider')

      expect(providerRow).toEqual(expect.objectContaining({
        status: 'missing',
        evidencePresent: true,
        reason: 'provider evidence missing at least one credentialed non-DeepSeek probe'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
	    }
	  })

	  it('does not accept live provider evidence that recorded credential values', () => {
	    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-provider-credential-values-'))
	    try {
	      const provider = join(dir, 'provider.json')
	      const evidence = providerEvidence([
	        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
	        { id: 'openai-compatible', status: 'passed', credentialed: true, skipped: false }
	      ], { readsRealApiKeysByDefault: true })
	      evidence.credentialValuesRecorded = true
	      writeJSON(provider, evidence)

	      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
	        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
	        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: provider
	      })
	      const providerRow = (report.finalCoverage as Array<Record<string, unknown>>)
	        .find((item) => item.id === 'credentialed-provider')

	      expect(providerRow).toEqual(expect.objectContaining({
	        status: 'missing',
	        evidencePresent: true,
	        reason: 'provider evidence is not credentialed/redacted root evidence'
	      }))
	    } finally {
	      rmSync(dir, { recursive: true, force: true })
	    }
	  })

	  it('does not accept live provider evidence with status-only passed probes', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-provider-probe-details-'))
    try {
      const provider = join(dir, 'provider.json')
      writeJSON(provider, providerEvidence([
        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
        { id: 'openai-compatible', status: 'passed', credentialed: true, skipped: false }
      ], { decoratePassedProbes: false }))

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: provider
      })
      const providerRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'credentialed-provider')

      expect(providerRow).toEqual(expect.objectContaining({
        status: 'missing',
        evidencePresent: true,
        reason: expect.stringContaining('provider evidence missing probe details')
      }))
      expect(providerRow?.reason).toContain('deepseek.streamParsing')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('does not accept MCP evidence without tool catalog and approval details', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-mcp-probe-details-'))
    try {
      const mcp = join(dir, 'mcp.json')
      writeJSON(mcp, mcpEvidence({ includeProbeDetails: false }))

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: mcp
      })
      const mcpRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'credentialed-mcp')

      expect(mcpRow).toEqual(expect.objectContaining({
        status: 'missing',
        evidencePresent: true,
        reason: expect.stringContaining('MCP evidence missing coverage')
      }))
      expect(mcpRow?.reason).toContain('toolCatalogDigest')
      expect(mcpRow?.reason).toContain('approvalUserInputEvidence')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('does not accept operator evidence without digest-backed review details', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-operator-details-'))
    try {
      const operator = join(dir, 'operator.json')
      writeJSON(operator, operatorEvidence({ includeReviewDetails: false }))

      const report = runReport('scripts/runtime-go-cutover-report.mjs', [], {
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_READY_EVIDENCE: operator
      })
      const operatorRow = (report.finalCoverage as Array<Record<string, unknown>>)
        .find((item) => item.id === 'operator-gate')

      expect(operatorRow).toEqual(expect.objectContaining({
        status: 'missing',
        evidencePresent: true,
        reason: expect.stringContaining('operator evidence missing review details')
      }))
      expect(operatorRow?.reason).toContain('evidenceDigests.provider')
      expect(operatorRow?.reason).toContain('evidenceReviewed.provider-matrix-credentialed')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('marks deterministic cutover pass as partial until live final evidence is covered', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-deterministic-partial-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const candidate = {
        schemaVersion: 1,
        id: 'runtime-go-default-readiness-report',
        status: 'passed',
        passed: true,
        goDefaultReady: true,
        productRegression: {
          passed: true,
          features: ['TypeScript retired backend'],
          rowCount: 33,
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        },
        speedCacheGate: {
          passed: true,
          checkCount: 14
        },
        runtimeHealth: firstRunnableRuntimeHealthFields()
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:default-readiness-report') {
  console.log(${JSON.stringify(JSON.stringify(candidate))})
  process.exit(0)
}
if (args[0] === 'run' && (args[1] === 'typecheck' || args[1] === 'build:runtime')) {
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'diff' && args[1] === '--check') process.exit(0)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const report = runReportWithoutDryRun('scripts/runtime-go-cutover-report.mjs', [
        '--live-evidence-json',
        join(dir, 'missing-live-evidence.json')
      ], {
        PATH: `${dir}${delimiter}${process.env.PATH || ''}`
      })
      const cutover = report.cutover as Record<string, unknown>

      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-cutover-report',
        status: 'passed',
        passed: true,
        goRuntimeDefaultCutoverReady: true,
        deterministicCutoverReady: true,
        finalAcceptanceReady: false,
        finalAcceptanceStatus: 'partial',
        finalStatus: 'partial',
        finalAcceptanceReason: 'deterministic checks passed, but final live credentialed/package/operator coverage is missing',
        deterministicEvidenceOnly: true
      }))
      expect(report.missingFinalCoverage).toEqual(expect.arrayContaining([
        'credentialed-provider',
        'credentialed-mcp',
        'packaged-desktop-qa',
        'operator-gate'
      ]))
      expect(report.missingFinalCoverage).not.toContain('typescript-retired-backend')
      expect(report.missingFinalCoverage).not.toContain('production-authority-enrolled-provider-agent-chain')
      expect(cutover).toEqual(expect.objectContaining({
        finalAcceptanceReady: false,
        finalAcceptanceStatus: 'partial',
        finalStatus: 'partial',
        runtimeHealthRuntimeIdentityEvidenceValid: true,
        runtimeHealthBoundaryEvidenceValid: true,
        runtimeHealthProviderAgentChainEvidenceValid: true,
        runtimeHealthProviderConfigMode: 'isolated-contract-provider-config',
        runtimeHealthUsesRealProviderConfig: false,
        runtimeHealthUsesIsolatedContractProviderConfig: true,
        runtimeHealthUsesIsolatedEmptyProviderConfig: false,
        runtimeHealthProductChainMode: 'production-authority-enrolled-provider-agent-chain',
        runtimeHealthProductChainOk: true,
        runtimeHealthAuthorityBoundaryOk: true,
        runtimeHealthHostAcceptedFinalOk: true,
        runtimeHealthProviderPositiveChainOk: true,
        runtimeHealthProviderAgentLoopReady: true,
        runtimeHealthProviderAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
        runtimeHealthProviderExecutionExpected: true,
        runtimeHealthProviderExecutionBlocked: false,
        runtimeHealthProtectedTurnCreateOk: true,
        runtimeHealthOrdinaryTurnCreateOk: true,
        runtimeHealthProtectedSseReplayOk: true,
        runtimeHealthOrdinarySseReplayOk: true,
        runtimeHealthOrdinaryThreadDistinct: true,
        runtimeHealthOrdinaryPublicFinalOk: true,
        runtimeHealthTurnCreateOk: true,
        runtimeHealthSseReplayOk: true,
        runtimeHealthUsageOk: true,
        runtimeHealthProviderUsageObserved: true,
        runtimeHealthProviderUsageAbsentOk: true,
        runtimeHealthProviderDraftWithheld: true,
        runtimeHealthProtectedProviderDispatchAbsent: true,
        runtimeHealthProtectedProviderRequestCount: 0,
        runtimeHealthAttachmentOk: true,
        runtimeHealthContractProviderRequestCount: 1,
        runtimeHealthContractProviderPromptSeen: true,
        runtimeHealthProviderAuthorizationConfigured: true,
        runtimeHealthHealthOk: true,
        runtimeHealthSecretsPrinted: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it.each([
    ['ordinary SSE replay missing', (runtimeHealth: Record<string, unknown>) => {
      runtimeHealth.ordinarySseReplayOk = false
    }],
    ['propagated runtime identity invalid', (runtimeHealth: Record<string, unknown>) => {
      runtimeHealth.runtimeIdentityEvidenceValid = false
    }],
    ['runtime process identity member false', (runtimeHealth: Record<string, unknown>) => {
      runtimeHealth.actualRuntimeProcessStarted = false
    }]
  ])('marks the First Runnable cutover row missing when %s', (_name, mutate) => {
    const report = runCutoverWithFirstRunnableMutation(mutate)
    const row = (report.finalCoverage as Array<Record<string, unknown>>)
      .find((item) => item.id === 'production-authority-enrolled-provider-agent-chain')

    expect(row).toEqual(expect.objectContaining({
      status: 'missing',
      evidencePresent: true
    }))
    expect(report.missingFinalCoverage).toContain('production-authority-enrolled-provider-agent-chain')
  })

  it('uses covered live-evidence env as final coverage fallback without approving blocked gates', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-live-evidence-env-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const mcp = join(dir, 'mcp.json')
      const packaged = join(dir, 'packaged.json')
      const liveEvidence = join(dir, 'live-evidence-report.json')
      writeJSON(mcp, mcpEvidence())
      writeJSON(packaged, packagedAggregateEvidence())
      writeJSON(liveEvidence, {
        id: 'runtime-go-live-evidence-report',
        status: 'live_blocked',
        passed: false,
        credentialSecretsRecorded: false,
        componentStatus: {
          provider: 'live_blocked',
          mcp: 'passed',
          packaged: 'passed',
          operator: 'live_blocked'
        },
        missingExternalInputs: [
          {
            id: 'provider:openai-compatible',
            status: 'skipped',
            reason: 'missing env-gated credential/baseUrl/model; skipped without counting as pass',
            missingEnv: [
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
            ],
            missingSettingsProfileFields: [
              'provider.providers[].matchingProfile',
              'sk-' + 'syntheticScanNotIssued'.padEnd(36, '0')
            ]
          },
          {
            id: 'operator:gate',
            status: 'live_blocked',
            reason: 'ANALYTIX_RUNTIME_READY is not set to 1'
          }
        ],
        coveredFinalCoverageEnv: {
          ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
          ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS: 'passed',
          ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
          ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: mcp,
          ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
          ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: packaged,
          ANALYTIX_RUNTIME_READY: ''
        },
        nextEvidenceActions: {
          providerMatrix: {
            settingsProfileCoverage: {
              status: 'loaded',
              source: 'default-app-settings',
              providerCount: 1,
              matchedProviderIds: ['deepseek', 'sk-' + 'syntheticMatchedNotIssued'.padEnd(38, '0')],
              usableProviderIds: ['deepseek', 'sk-' + 'syntheticCanaryNotIssued'.padEnd(40, '0')],
              usableNonDeepSeekProviderIds: [],
              deepseekSettingsProfileUsable: true,
              nonDeepSeekSettingsProfileUsable: false,
              satisfiesFinalProviderCoverageFromSettings: false,
              credentialValuesRecorded: false
            }
          }
        },
        strictGateEnvWhenPassed: {}
      })
      const candidate = {
        schemaVersion: 1,
        id: 'runtime-go-default-readiness-report',
        status: 'passed',
        passed: true,
        goDefaultReady: true,
        productRegression: {
          passed: true,
          features: ['TypeScript retired backend'],
          rowCount: 33,
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        },
        speedCacheGate: { passed: true, checkCount: 14 },
        runtimeHealth: authorityBoundaryRuntimeHealthFields()
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:default-readiness-report') {
  console.log(${JSON.stringify(JSON.stringify(candidate))})
  process.exit(0)
}
if (args[0] === 'run' && (args[1] === 'typecheck' || args[1] === 'build:runtime')) {
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'diff' && args[1] === '--check') process.exit(0)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const report = runReportWithoutDryRun('scripts/runtime-go-cutover-report.mjs', [
        '--live-evidence-json',
        liveEvidence
      ], {
        PATH: `${dir}${delimiter}${process.env.PATH || ''}`
      })
      const finalCoverage = report.finalCoverage as Array<Record<string, unknown>>
      const covered = finalCoverage.filter((item) => item.status === 'covered').map((item) => item.id)

      expect(covered).toEqual(expect.arrayContaining([
        'durable-restart',
        'credentialed-mcp',
        'packaged-desktop-qa',
        'typescript-retired-backend'
      ]))
      expect(covered).not.toContain('credentialed-provider')
      expect(covered).not.toContain('operator-gate')
      expect(report.missingFinalCoverage).toEqual([
        'credentialed-provider',
        'production-authority-enrolled-provider-agent-chain',
        'operator-gate'
      ])
      expect(report.finalCoverageEvidenceEnvSource).toEqual(expect.objectContaining({
        source: 'live-evidence-covered-env',
        path: liveEvidence,
        keys: [
          'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
          'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
          'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
          'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
          'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
          'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE'
        ]
      }))
      expect(report.finalCoverageLiveEvidence).toEqual(expect.objectContaining({
        id: 'runtime-go-live-evidence-report',
        status: 'live_blocked',
        passed: false,
        credentialSecretsRecorded: false,
        componentStatus: expect.objectContaining({
          provider: 'live_blocked',
          mcp: 'passed',
          packaged: 'passed',
          operator: 'live_blocked'
        }),
        providerSettingsProfileCoverage: {
          status: 'loaded',
          source: 'default-app-settings',
          providerCount: 1,
          matchedProviderIds: ['deepseek'],
          usableProviderIds: ['deepseek'],
          usableNonDeepSeekProviderIds: [],
          deepseekSettingsProfileUsable: true,
          nonDeepSeekSettingsProfileUsable: false,
          satisfiesFinalProviderCoverageFromSettings: false,
          credentialValuesRecorded: false
        },
        coveredFinalCoverageEnvKeys: [
          'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
          'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
          'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
          'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
          'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
          'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
          'ANALYTIX_RUNTIME_READY'
        ],
        strictGateEnvWhenPassedKeys: []
      }))
      expect(report.finalCoverageLiveEvidence).toEqual(expect.objectContaining({
        missingExternalInputs: expect.arrayContaining([
          expect.objectContaining({
            id: 'provider:openai-compatible',
            status: 'skipped',
            missingEnv: expect.arrayContaining([
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
              'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
            ]),
            missingSettingsProfileFields: ['provider.providers[].matchingProfile']
          }),
          expect.objectContaining({
            id: 'operator:gate',
            status: 'live_blocked',
            reason: 'ANALYTIX_RUNTIME_READY is not set to 1'
          })
        ])
      }))
      expect(JSON.stringify(report.finalCoverageLiveEvidence)).not.toContain('sk-' + 'syntheticScanNotIssued'.padEnd(36, '0'))
      expect(JSON.stringify(report.finalCoverageLiveEvidence)).not.toContain('sk-' + 'syntheticMatchedNotIssued'.padEnd(38, '0'))
      expect(JSON.stringify(report.finalCoverageLiveEvidence)).not.toContain('sk-' + 'syntheticCanaryNotIssued'.padEnd(40, '0'))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('blocks cutover final acceptance when worktree final blockers remain', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-cutover-worktree-blocker-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
      const provider = join(dir, 'provider.json')
      const mcp = join(dir, 'mcp.json')
      const packaged = join(dir, 'packaged.json')
      const operator = join(dir, 'operator.json')
      writeJSON(provider, providerEvidence([
        { id: 'deepseek', status: 'passed', credentialed: true, skipped: false },
        { id: 'openai-compatible', status: 'passed', credentialed: true, skipped: false }
      ], { readsRealApiKeysByDefault: true }))
      writeJSON(mcp, mcpEvidence())
      writeJSON(packaged, packagedAggregateEvidence())
      writeJSON(operator, operatorEvidence())
      const candidate = {
        schemaVersion: 1,
        id: 'runtime-go-default-readiness-report',
        status: 'passed',
        passed: true,
        goDefaultReady: true,
        productRegression: {
          passed: true,
          features: ['TypeScript retired backend'],
          rowCount: 33,
          publicLegacyExposureCount: 0,
          internalLegacyDelegateCount: 0,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        },
        speedCacheGate: { passed: true, checkCount: 15 },
        runtimeHealth: authorityBoundaryRuntimeHealthFields()
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'run' && args[1] === 'runtime:go:default-readiness-report') {
  console.log(${JSON.stringify(JSON.stringify(candidate))})
  process.exit(0)
}
if (args[0] === 'run' && (args[1] === 'typecheck' || args[1] === 'build:runtime')) {
  process.exit(0)
}
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'diff' && args[1] === '--check') process.exit(0)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log(' M tmp/cache.pyc\\n M "\\\\351\\\\207\\\\215\\\\346\\\\236\\\\204\\\\345\\\\215\\\\207\\\\347\\\\272\\\\247\\\\346\\\\226\\\\271\\\\346\\\\241\\\\210.md"')
  process.exit(0)
}
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const report = runReportWithoutDryRun('scripts/runtime-go-cutover-report.mjs', [], {
        PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
        ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: provider,
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
        ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: mcp,
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: packaged,
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_READY_EVIDENCE: operator
      })
      const cutover = report.cutover as Record<string, unknown>
      const worktree = report.worktree as Record<string, unknown>

      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-cutover-report',
        status: 'passed',
        passed: true,
        goRuntimeDefaultCutoverReady: true,
        deterministicCutoverReady: true,
        finalAcceptanceReady: false,
        finalAcceptanceStatus: 'blocked',
        finalStatus: 'blocked',
        finalAcceptanceReason: 'deterministic checks passed, but preflight final blockers remain',
        missingFinalCoverage: ['production-authority-enrolled-provider-agent-chain'],
        finalGateBlocked: true,
        finalGateBlockers: expect.arrayContaining([
          'worktree:non-goal-dirty-files',
          'worktree:generated-artifacts',
          'worktree:tracked-generated-artifacts'
        ])
      }))
      expect(worktree).toEqual(expect.objectContaining({
        status: 'dirty',
        dirtyFileCount: 2,
        goalScopeDirtyFileCount: 1,
        nonGoalScopeDirtyFileCount: 1,
        generatedArtifactDirtyFileCount: 1,
        trackedGeneratedArtifactDirtyFileCount: 1,
        finalAcceptanceRequiresCleanOrClassifiedTree: true
      }))
      expect(cutover).toEqual(expect.objectContaining({
        finalGateBlocked: true,
        finalGateBlockers: expect.arrayContaining([
          'worktree:non-goal-dirty-files',
          'worktree:generated-artifacts',
          'worktree:tracked-generated-artifacts'
        ]),
        worktreeFinalAcceptanceRequiresCleanOrClassifiedTree: true
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('documents that runtime health smoke uses a temporary real runtime process', () => {
    const report = runReport('scripts/runtime-go-runtime-health-smoke.mjs')
    const smoke = report.smoke as Record<string, unknown>

    expect(report).toEqual(expect.objectContaining({
      schemaVersion: 2,
      id: 'runtime-go-runtime-health-smoke',
      status: 'skipped',
      passed: false
    }))
    expect(smoke).toEqual(expect.objectContaining({
      actualRuntimeProcessStarted: false,
      dataDirScope: 'temporary',
      usesRealProviderConfig: false,
      providerConfigMode: 'isolated-empty-provider-config',
      productChainMode: 'not-run',
      productChainOk: false,
      authorityBoundaryOk: false,
      hostAcceptedFinalOk: false,
      providerPositiveChainOk: false,
      providerAgentLoopReady: false,
      providerAgentLoopCoverage: 'not-run',
      providerExecutionExpected: false,
      providerExecutionBlocked: false,
      usageOk: false,
      providerUsageObserved: false,
      providerUsageAbsentOk: false,
      contractProviderRequestCount: null,
      actualPackagedAppLaunched: false,
      runtimeInfoOk: false,
      runtimeToolsOk: false,
      productionCapabilitiesOk: false,
      secretsPrinted: false
    }))
  })

  it('builds runtime health smoke with the production Go build tag', () => {
    const script = readFileSync(join(process.cwd(), 'scripts/runtime-go-runtime-health-smoke.mjs'), 'utf8')

    expect(script).toContain("'build',")
    expect(script).toContain("'-tags',")
    expect(script).toContain("'analytix_prod',")
    expect(script.indexOf("'-tags',")).toBeLessThan(script.indexOf("'./cmd/runtime-server'"))
  })

  it('keeps runtime readiness and Go build timeout budgets independently bounded', () => {
    const script = readFileSync(join(process.cwd(), 'scripts/runtime-go-runtime-health-smoke.mjs'), 'utf8')

    expect(script).toContain("const timeoutMs = Number.parseInt(argValue('--timeout-ms', '30000'), 10) || 30000")
    expect(script).toContain("const buildTimeoutMs = Number.parseInt(argValue('--build-timeout-ms', '120000'), 10) || 120000")

    const buildBlockStart = script.indexOf('const buildResult = spawnSync(goCommand')
    const buildBlockEnd = script.indexOf('\n    const buildExitStatus', buildBlockStart)
    expect(buildBlockStart).toBeGreaterThan(-1)
    expect(buildBlockEnd).toBeGreaterThan(buildBlockStart)
    const buildBlock = script.slice(buildBlockStart, buildBlockEnd)
    expect(buildBlock).toContain('timeout: buildTimeoutMs')
    expect(buildBlock).not.toContain('timeout: timeoutMs')

    const runSmokeStart = script.indexOf('async function runSmoke() {\n  const startedAt = Date.now()')
    const childSpawnStart = script.indexOf('child = spawn(runtimeServerBinary', runSmokeStart)
    const stdoutListener = script.indexOf("child.stdout.on('data'", childSpawnStart)
    const stderrListener = script.indexOf("child.stderr.on('data'", stdoutListener)
    const readinessStartedAt = script.indexOf('const readinessStartedAt = Date.now()', stderrListener)
    const waitForReadyCall = script.indexOf(
      'const ready = await waitForReady(child, stdoutChunks, stderrChunks, readinessStartedAt)',
      readinessStartedAt
    )
    const readyDuration = script.indexOf('const readyDuration = Date.now() - readinessStartedAt', waitForReadyCall)
    expect(runSmokeStart).toBeGreaterThan(-1)
    expect(childSpawnStart).toBeGreaterThan(runSmokeStart)
    expect(stdoutListener).toBeGreaterThan(childSpawnStart)
    expect(stderrListener).toBeGreaterThan(stdoutListener)
    expect(readinessStartedAt).toBeGreaterThan(stderrListener)
    expect(waitForReadyCall).toBeGreaterThan(readinessStartedAt)
    expect(readyDuration).toBeGreaterThan(waitForReadyCall)

    const catchBlockStart = script.indexOf('\n  } catch (error) {', readyDuration)
    expect(catchBlockStart).toBeGreaterThan(readyDuration)
    expect(script.slice(catchBlockStart)).toContain('durationMs: Date.now() - startedAt')
  })

  it('covers the protected boundary and ordinary provider chain without live credentials', () => {
    const script = readFileSync('scripts/runtime-go-runtime-health-smoke.mjs', 'utf8')
    const report = runReportWithoutDryRun('scripts/runtime-go-runtime-health-smoke.mjs')
    const smoke = report.smoke as Record<string, unknown>
    const checkIds = (report.checks as Array<Record<string, unknown>>).map((item) => item.id)

    expect(report).toEqual(expect.objectContaining({
      schemaVersion: 2,
      id: 'runtime-go-runtime-health-smoke',
      status: 'passed',
      passed: true
    }))
    expect(smoke).toEqual(expect.objectContaining({
      actualRuntimeProcessStarted: true,
      usesRealProviderConfig: false,
      providerConfigMode: 'isolated-contract-provider-config',
      productChainMode: 'production-authority-enrolled-provider-agent-chain',
      productChainOk: true,
      authorityBoundaryOk: true,
      hostAcceptedFinalOk: true,
      providerPositiveChainOk: true,
      providerAgentLoopReady: true,
      providerAgentLoopCoverage: 'ordinary-turn-with-protected-boundary',
      providerExecutionExpected: true,
      providerExecutionBlocked: false,
      protectedTurnCreateOk: true,
      ordinaryTurnCreateOk: true,
      protectedSseReplayOk: true,
      ordinarySseReplayOk: true,
      ordinaryThreadDistinct: true,
      ordinaryPublicFinalOk: true,
      turnCreateOk: true,
      sseReplayOk: true,
      usageOk: true,
      providerUsageObserved: true,
      providerUsageAbsentOk: true,
      protectedProviderDispatchAbsent: true,
      protectedProviderRequestCount: 0,
      attachmentOk: true,
      providerDraftWithheld: true,
      contractProviderRequestCount: 1,
      contractProviderPromptSeen: true,
      providerAuthorizationConfigured: true,
      secretsPrinted: false
    }))
    expect(checkIds).toEqual(expect.arrayContaining([
      'runtime-health',
      'runtime-info',
      'runtime-tools',
      'runtime-authority-boundary',
      'runtime-provider-agent-chain'
    ]))
    expect(script).toContain('authorizationConfigured: req.headers.authorization === `Bearer ${contractProviderApiKey}`')
    expect(script).not.toContain('authorizationConfigured: Boolean(req.headers.authorization)')
  }, 180_000)

  it('keeps TypeScript retired backend evidence explicit about runtime-health provider config mode', () => {
    const report = runReport('scripts/runtime-go-rollback-retirement-evidence.mjs')
    const rollback = report.rollback as Record<string, unknown>

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-rollback-retirement-evidence',
      status: 'skipped',
      passed: false
    }))
    expect(rollback).toEqual(expect.objectContaining({
      runtimeHealthPassed: false,
      runtimeHealthUsesRealProviderConfig: false,
      runtimeHealthProviderConfigMode: 'not-run',
      runtimeHealthUsesIsolatedEmptyProviderConfig: false,
      explicitTypeScriptOverrideRetained: false,
      typeScriptOverrideRetired: true,
      typeScriptCodePathDeleted: true,
      typeScriptRuntimeSourceDeleted: true,
      productionForbiddenImportCount: 0,
      packagedDistRuntimeModuleCount: 0,
      packageExportViolationCount: 0,
      buildConfigViolationCount: 0,
      overrideEnv: 'ANALYTIX_RUNTIME_BACKEND=typescript'
    }))
    expect(rollback.sourceRuntimeImplementationCount).toBe(0)
    expect((report.checks as Array<Record<string, unknown>>).map((item) => item.id)).toEqual([
      'typescript-retired-backend',
      'legacy-child-retired',
      'analytix-serve-go-launcher',
      'packaged-go-boundary',
      'typescript-codepath-scan'
    ])
  })

  it('keeps rollback retirement evidence from hiding later checks after an early failure', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-rollback-no-fail-fast-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const marker = join(dir, 'commands.txt')
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
fs.appendFileSync(${JSON.stringify(marker)}, args.join(' ') + '\\n')
if (args[0] === 'run' && args[1] === 'test' && args.includes('src/main/runtime/analytix-adapter.test.ts')) {
  process.exit(17)
}
if (args[0] === 'run' && args[1] === 'test') process.exit(0)
if (args[0] === 'run' && args[1] === '--prefix' && args.includes('tests/serve-entry-go-launcher.test.ts')) {
  process.exit(0)
}
process.exit(91)
`)

      const result = spawnSync(process.execPath, ['scripts/runtime-go-rollback-retirement-evidence.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>
      const commands = readFileSync(marker, 'utf8')

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-rollback-retirement-evidence',
        status: 'failed',
        passed: false
      }))
      expect(checks.map((item) => item.id)).toEqual([
        'typescript-retired-backend',
        'legacy-child-retired',
        'analytix-serve-go-launcher',
        'packaged-go-boundary',
        'typescript-codepath-scan'
      ])
      expect(checks[0]).toEqual(expect.objectContaining({
        id: 'typescript-retired-backend',
        status: 'failed',
        exitStatus: 17
      }))
      expect(checks.slice(1, 4).every((item) => item.status === 'passed')).toBe(true)
      expect(checks[4]).toEqual(expect.objectContaining({
        id: 'typescript-codepath-scan',
        status: 'passed'
      }))
      expect(commands).toContain('src/main/analytix-process.test.ts')
      expect(commands).toContain('tests/serve-entry-go-launcher.test.ts')
      expect(commands).toContain('src/main/packaging-config.test.ts')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('keeps rollback retirement on formal runtime-go validation entrypoints', () => {
    const report = runReport('scripts/runtime-go-rollback-retirement-report.mjs')
    const rollback = report.rollback as Record<string, unknown>
    const checks = report.checks as Array<Record<string, unknown>>
    const validationWrapper = readFileSync(join(process.cwd(), 'scripts/runtime-go-validation-command.mjs'), 'utf8')
    const reportScript = readFileSync(join(process.cwd(), 'scripts/runtime-go-rollback-retirement-report.mjs'), 'utf8')

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-rollback-retirement-report',
      status: 'skipped',
      passed: false
    }))
    expect(rollback).toEqual(expect.objectContaining({
      explicitTypeScriptOverrideRetained: false,
      typeScriptCodePathDeleted: true,
      typeScriptRuntimeSourceDeleted: true,
      productionForbiddenImportCount: 0,
      packagedDistRuntimeModuleCount: 0,
      packageExportViolationCount: 0,
      buildConfigViolationCount: 0,
      overrideEnv: 'ANALYTIX_RUNTIME_BACKEND=typescript',
      packagedQAPassed: false
    }))
    expect(rollback.sourceRuntimeImplementationCount).toBe(0)
    expect(checks.map((item) => item.id)).toEqual([
      'rollback-evidence',
      'packaged-qa',
      'typescript-codepath-scan'
    ])
    expect(reportScript).toContain("id: 'packaged-qa'")
    expect(reportScript).toContain("args: ['run', 'qa:runtime:packaged', '--', '--json', '--no-write']")
    expect(reportScript).toContain('runTypeScriptRetirementScan')
    expect(reportScript).not.toContain('typeScriptCodePathDeleted: false')
    expect(validationWrapper).toContain("'approval-user-input-evidence': { script: 'runtime-go-approval-user-input-evidence.mjs' }")
    expect(readFileSync(join(process.cwd(), 'package.json'), 'utf8')).toContain('"runtime:go:approval-user-input-evidence": "node ./scripts/runtime-go-validation-command.mjs approval-user-input-evidence"')
    expect(validationWrapper).toContain("'rollback-retirement-evidence': { script: 'runtime-go-rollback-retirement-evidence.mjs' }")
    expect(validationWrapper).toContain("'rollback-retirement-report': { script: 'runtime-go-rollback-retirement-report.mjs' }")
    expect(validationWrapper).toContain("'ts-retirement-scan': { script: 'runtime-go-ts-retirement-scan.mjs' }")
    expect(readFileSync(join(process.cwd(), 'package.json'), 'utf8')).toContain('"runtime:go:ts-retirement-scan": "node ./scripts/runtime-go-validation-command.mjs ts-retirement-scan"')
    expect(validationWrapper).not.toMatch(/script:\s*'d0253[^']*\.mjs'/i)
    expect(existsSync(join(process.cwd(), 'scripts/d0253-retirement-evidence-collector.mjs'))).toBe(false)
    expect(existsSync(join(process.cwd(), 'scripts/d0253-fallback-retirement-report.mjs'))).toBe(false)
  })

  it('keeps rollback retirement report from hiding packaged QA after rollback evidence fails', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-rollback-report-no-fail-fast-'))
    try {
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      const marker = join(dir, 'commands.txt')
      const rollback = {
        id: 'runtime-go-rollback-retirement-evidence',
        status: 'failed',
        passed: false,
        rollback: {
          typeScriptOverrideRetired: true,
          retiredBackendCovered: false
        }
      }
      const packaged = {
        id: 'runtime-go-packaged-qa',
        status: 'passed',
        passed: true,
        packaged: {
          productionGoSourceCount: 32
        }
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const fs = require('node:fs')
const args = process.argv.slice(2)
fs.appendFileSync(${JSON.stringify(marker)}, args.join(' ') + '\\n')
if (args[0] === 'run' && args[1] === 'runtime:go:rollback-retirement-evidence') {
  console.log(JSON.stringify(${JSON.stringify(rollback)}))
  process.exit(19)
}
if (args[0] === 'run' && args[1] === 'qa:runtime:packaged') {
  console.log(JSON.stringify(${JSON.stringify(packaged)}))
  process.exit(0)
}
process.exit(91)
`)

      const result = spawnSync(process.execPath, ['scripts/runtime-go-rollback-retirement-report.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>
      const checks = report.checks as Array<Record<string, unknown>>
      const commands = readFileSync(marker, 'utf8')

      expect(result.status).toBe(1)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-rollback-retirement-report',
        status: 'failed',
        passed: false
      }))
      expect(checks.map((item) => item.id)).toEqual([
        'rollback-evidence',
        'packaged-qa',
        'typescript-codepath-scan'
      ])
      expect(checks[0]).toEqual(expect.objectContaining({
        id: 'rollback-evidence',
        status: 'failed',
        exitStatus: 19
      }))
      expect(checks[1]).toEqual(expect.objectContaining({
        id: 'packaged-qa',
        status: 'passed'
      }))
      expect(checks[2]).toEqual(expect.objectContaining({
        id: 'typescript-codepath-scan',
        status: 'passed'
      }))
      expect((report.rollback as Record<string, unknown>).packagedQAPassed).toBe(true)
      expect(commands).toContain('runtime:go:rollback-retirement-evidence')
      expect(commands).toContain('qa:runtime:packaged')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('keeps retired TypeScript runtime barrels deleted from the Go-only code path', () => {
    const retiredBarrels = [
      'packages/runtime/src/adapters/index.ts',
      'packages/runtime/src/adapters/tool/index.ts',
      'packages/runtime/src/adapters/tool/bash.ts',
      'packages/runtime/src/adapters/tool/edit.ts',
      'packages/runtime/src/adapters/tool/find.ts',
      'packages/runtime/src/adapters/tool/grep.ts',
      'packages/runtime/src/adapters/tool/ls.ts',
      'packages/runtime/src/adapters/tool/read.ts',
      'packages/runtime/src/adapters/tool/write.ts',
      'packages/runtime/src/delegation/index.ts',
      'packages/runtime/src/loop/append-only-session-log.ts',
      'packages/runtime/src/loop/model-request-estimator.ts',
      'packages/runtime/src/loop/tool-result-image.ts',
      'packages/runtime/src/loop/context-estimator.ts',
      'packages/runtime/src/loop/index.ts',
      'packages/runtime/src/review/review-prompt.ts',
      'packages/runtime/src/review/git-review-target.ts',
      'packages/runtime/src/review/review-output.ts',
      'packages/runtime/src/adapters/model/tool-argument-repair.ts',
      'packages/runtime/src/adapters/model/vision-bridge.ts',
      'packages/runtime/src/server/runtime-factory.ts',
      'packages/runtime/src/server/node-http-server.ts',
      'packages/runtime/src/server/index.ts',
      'packages/runtime/src/services/index.ts'
    ]
    for (const path of retiredBarrels) {
      expect(existsSync(join(process.cwd(), path))).toBe(false)
    }

    const result = spawnSync(process.execPath, ['scripts/runtime-go-ts-retirement-scan.mjs', '--json'], {
      cwd: process.cwd(),
      env: reportEnv(),
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    const report = JSON.parse(result.stdout) as Record<string, unknown>
    expect(report).toEqual(expect.objectContaining({
      typeScriptCodePathDeleted: true,
      productionForbiddenImportCount: 0,
      packagedDistRuntimeModuleCount: 0,
      packageExportViolationCount: 0,
      buildConfigViolationCount: 0
    }))
    expect(report.allowedRuntimePackageExports).toEqual([
      '.',
      './contracts',
      './config',
      './cli',
      './telemetry'
    ])
    expect(report.sourceRuntimeImplementationCount).toBe(0)
  })

  it('keeps packaged QA actual desktop and TypeScript retired backend evidence explicit', () => {
    const report = runReport('scripts/runtime-go-packaged-qa.mjs')
    const packaged = report.packaged as Record<string, unknown>
    const checks = report.checks as Array<Record<string, unknown>>
    const reportScript = readFileSync(join(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs'), 'utf8')

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'skipped',
      passed: false,
      credentialSecretsRecorded: false,
      redaction: expect.objectContaining({ status: 'passed', secretMaterialFound: false })
    }))
    expect(checks.map((item) => item.id)).toEqual([
      'cutover-report',
      'gui-smoke',
      'session-soak',
      'milestone-a',
      'rollback-evidence',
      'packaging-config',
      'commercial-release:formal-release-publication-authority',
      'commercial-release:controlled-release-native-receipt'
    ])
    expect(packaged).toEqual(expect.objectContaining({
      finalAcceptanceReady: false,
      actualPackagedSmokeRequested: false,
      actualPackagedSmokePassed: false,
      actualPackagedDesktopQAPassed: false,
      typeScriptRetiredBackendPassed: false,
      actualPackagedAppEvidenceRequiredForFinalGate: true,
      packagingConfigPassed: false
    }))
    expect((packaged.missingFinalCoverage as string[])).toContain('typescript-runtime-source-deleted')
    expect(reportScript).toContain("id: 'cutover-report'")
    expect(reportScript).toContain("id: 'gui-smoke'")
    expect(reportScript).toContain("id: 'session-soak'")
    expect(reportScript).toContain("id: 'milestone-a'")
    expect(reportScript).toContain("id: 'rollback-evidence'")
    expect(reportScript).toContain("id: 'packaging-config'")
    expect(reportScript).toContain('typeScriptCodePathDeleted')
    expect(reportScript).toContain("missingFinalCoverage.delete('packaged-desktop-qa')")
    expect(reportScript).toContain("missingFinalCoverage.delete('typescript-retired-backend')")
    expect(reportScript).toContain("missingFinalCoverage.add('typescript-runtime-source-deleted')")
    expect(reportScript).toContain("missingFinalCoverage.delete('durable-restart')")
  })

  it('writes formal approval/user-input evidence for MCP live evidence', () => {
    const dryRun = runReport('scripts/runtime-go-approval-user-input-evidence.mjs')
    expect(dryRun).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-approval-user-input-evidence',
      status: 'skipped',
      passed: false,
      approvalUserInput: false,
      rawValueRecorded: false,
      credentialSecretsRecorded: false
    }))

    const dir = mkdtempSync(join(tmpdir(), 'analytix-approval-user-input-evidence-'))
    try {
      const fakeGo = join(dir, 'go')
      const output = join(dir, 'mcp-approval-user-input-evidence.json')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'test' && args[1] === '.' && args.includes('TestRuntimeServer(ApprovalDenyAndAllowControlToolExecution|BashApprovalDenyAndAllowControlExecution|UserInputOnlyAppearsWhenModelCallsTool|DisableUserInputRemovesInteractiveToolSchemas|NormalTurnsDoNotCreateSyntheticApprovalOrUserInputGates)$')) {
  process.exit(0)
}
console.error('unexpected go command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(process.execPath, ['scripts/runtime-go-approval-user-input-evidence.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: output
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(existsSync(output)).toBe(true)
      const stdoutReport = JSON.parse(result.stdout) as Record<string, unknown>
      const fileReport = JSON.parse(readFileSync(output, 'utf8')) as Record<string, unknown>
      for (const report of [stdoutReport, fileReport]) {
        expect(report).toEqual(expect.objectContaining({
          id: 'runtime-go-mcp-approval-user-input-evidence',
          status: 'passed',
          passed: true,
          approvalUserInput: true,
          approval: true,
          userInput: true,
          rawValueRecorded: false,
          credentialSecretsRecorded: false,
          outputPath: output,
          redaction: expect.objectContaining({
            status: 'passed',
            secretMaterialFound: false
          })
        }))
      }
      expect((fileReport.checks as Array<Record<string, unknown>>).map((item) => item.id)).toEqual([
        'go-approval-user-input-contract'
      ])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('writes actual packaged QA aggregate evidence for live evidence reuse', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-packaged-qa-write-'))
    try {
      const fakeNpm = join(dir, 'npm')
      const fakeGit = join(dir, 'git')
      const output = join(dir, 'runtime-go-live-evidence', 'packaged-go-runtime-qa.json')
      const cutover = {
        id: 'runtime-go-cutover-report',
        status: 'passed',
        passed: true,
        goRuntimeDefaultCutoverReady: true,
        finalAcceptanceReady: false,
        missingFinalCoverage: [
          'durable-restart',
          'credentialed-provider',
          'credentialed-mcp',
          'packaged-desktop-qa',
          'operator-gate'
        ],
        cutover: {
          runtimeHealthPassed: true,
          runtimeHealthRuntimeInfoOk: true,
          runtimeHealthRuntimeToolsOk: true,
          runtimeHealthProductionCapabilitiesOk: true,
          directLegacyExposureCount: 0,
          internalLegacyDelegateCount: 2,
          temporaryGoProdViolationCount: 0,
          productionGoSourceCount: 28,
          retiredProductionMarkerViolationCount: 0,
          legacyUpstreamFieldKeyViolationCount: 0
        }
      }
      const guiSmoke = {
        id: 'runtime-go-packaged-gui-smoke',
        status: 'passed',
        passed: true,
        smoke: {
          actualPackagedAppEvidenceRequiredForFinalGate: false,
          actualPackagedSmokeRequested: true,
          actualPackagedSmokePassed: true,
          actualPackagedAppLaunched: true,
          settingsProfilePatchAccepted: true,
          settingsCompatPatchUsed: false,
          settingsPatchMode: 'provider-profile',
          deterministicContractOnly: false
        }
      }
      const sessionSoak = {
        id: 'runtime-go-packaged-session-soak',
        status: 'passed',
        passed: true,
        soak: {
          deterministicContractOnly: false,
          actualPackagedAppLaunched: true,
          actualPackagedAppEvidenceRequiredForFinalGate: false,
          actualPackagedSessionSoakRequested: true,
          actualPackagedSessionSoakPassed: true,
          goDurableSessionCovered: true,
          typeScriptRetiredBackendSessionCovered: true,
          resumeForkPairingCovered: true,
          sseReplayCovered: true,
          threadListSearchCovered: true,
          usageCovered: true
        },
        actualPackagedSessionSoak: {
          status: 'passed',
          passed: true
        }
      }
      const rollback = {
        id: 'runtime-go-rollback-retirement-evidence',
        status: 'passed',
        passed: true,
        rollback: {
          explicitTypeScriptOverrideRetained: false,
          typeScriptOverrideRetired: true,
          typeScriptCodePathDeleted: true,
          typeScriptRuntimeSourceDeleted: true,
          sourceRuntimeImplementationCount: 0,
          retiredBackendCovered: true,
          legacyChildRetiredCovered: true,
          analytixServeGoLauncherCovered: true,
          packagedGoBoundaryCovered: true
        }
      }
      const milestoneHarnessEntries = packagedQAHarnessEntries()
      const milestoneHarnessDigest = sha256Bytes(canonicalJSON(milestoneHarnessEntries))
      const milestoneScriptSha256 = String(
        milestoneHarnessEntries.find((item) => item.name === 'runtime-go-packaged-milestone-a.mjs')?.sha256
      )
      const sourceClosureSnapshotDigest = 'a'.repeat(64)
      const milestoneA = {
        id: 'runtime-go-packaged-milestone-a',
        status: 'passed',
        passed: true,
        syntheticProviderUsed: false,
        localProviderUsed: false,
        directRuntimeTurnDriverUsed: false,
        composerDomAndPrimaryButtonRequired: true,
        cdpAndBridgeObservationOnly: true,
        harness: {
          scriptSha256: milestoneScriptSha256,
          contractManifestSha256: milestoneHarnessDigest,
          sourceClosureBound: true,
          sourceClosureSnapshotDigest,
          packagedSourceClosureSnapshotDigest: sourceClosureSnapshotDigest,
          entries: milestoneHarnessEntries
        },
        provider: {
          configured: true,
          credentialConfigured: true,
          credentialAuthorityBound: true,
          credentialAuthority: 'local-provider-registry-secret-store',
          normalLocalProviderSetupObserved: true,
      localCredentialEvidence: { ok: true, blocked: false, blocker: null, sourceBound: true, entryMethod: 'visible-computer-use', automatedCredentialEntryUsed: true, expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1, status: 'passed', scannedFileCount: 3, findingCount: 0, settingsFindingCount: 0, reportFindingCount: 0, unsafeEntryCount: 0, symlinkCount: 0, encodingCoverage: ['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'], protectedStoreOwnerPrivate: true },
          credentialRecorded: false,
          networkTurnCompleted: true,
          threadProviderBound: true,
          threadModelBound: true,
          resultTurnProviderReceiptBound: true,
          providerAttemptTelemetryValid: true,
          providerAttemptReceiptDigest: 'b'.repeat(64),
          providerLogicalCallCount: 2,
          providerAttemptCount: 2,
          successfulProviderAttemptCount: 2
        },
        workflow: {
          planModeSelected: true,
          planTurnObserved: true,
          planProviderReceiptBound: true,
          agentModeRestored: true,
          sameThreadPlanAgent: true,
          readObserved: true,
          planObserved: true,
          todoObserved: true,
          writeObserved: true,
          realTestObserved: true,
          subagentObserved: true,
          childSubagentProviderReceiptBound: true,
          gitCommandObserved: true,
          skillObserved: true,
          ordinaryMCPObserved: true,
          researchWritingObserved: true,
          relaunchProviderContinuationObserved: true,
          successfulToolResultsObserved: true,
          todosCompleted: true,
          exactlyOneBoundedSubagentCompleted: true,
          compactionCount: 1,
          firstLaunchObserved: true,
          normalFirstQuitObserved: true,
          freshProcessRelaunchObserved: true,
          exactThreadRecovered: true,
          exactTodosRecovered: true,
          exactSubagentsRecovered: true,
          exactCompactionsRecovered: true,
          exactResultRecovered: true,
          rendererVisibleThreadRecovered: true,
          rendererVisibleTodosRecovered: true,
          rendererVisibleSubagentRecovered: true,
          rendererVisibleCompactionRecovered: true,
          rendererVisibleResultRecovered: true,
          normalFinalQuitObserved: true,
          zeroResidualProcesses: true
        },
        publicSeams: {
          firstLaunch: {
            gitCommandAvailable: true,
            skillCatalogAvailable: true,
            ordinaryMCPAvailable: true
          },
          secondLaunch: {
            gitCommandAvailable: true,
            skillCatalogAvailable: true,
            ordinaryMCPAvailable: true
          }
        },
        caseCapability: {
          additiveNotReplacement: true,
          privateAuthorityInputsAbsent: true,
          unavailableWhileOrdinaryCapabilitiesRetained: true
        },
        visualEvidence: {
          screenshotsUsedAsFunctionalVerdict: false,
          credentialScanCovered: true,
          firstCompleted: {
            captured: true,
            sha256: 'c'.repeat(64),
            width: 1200,
            height: 800,
            viewport: { width: 1200, height: 800, scale: 2 }
          },
          recoveredAfterRelaunch: {
            captured: true,
            sha256: 'd'.repeat(64),
            width: 1200,
            height: 800,
            viewport: { width: 1200, height: 800, scale: 2 }
          }
        },
        isolation: {
          sameDataDirsOnRelaunch: true,
          sandboxRemoved: true
        },
        app: {
          ok: true,
          controlledReleaseNativeReceipt: true
        },
        releasePublicationAuthority: { ok: true },
        commercialRelease: {
          status: 'passed',
          passed: true,
          requiredForMilestoneA: false,
          requiredForCommercialPublication: true,
          checks: [
            { id: 'formal-release-publication-authority', status: 'passed' },
            { id: 'controlled-release-native-receipt', status: 'passed' }
          ]
        }
      }
      writeExecutable(fakeNpm, `#!/usr/bin/env node
const args = process.argv.slice(2)
function print(value) {
  console.log(JSON.stringify(value))
  process.exit(0)
}
if (args[0] === 'run' && args[1] === 'runtime:go:cutover-report') print(${JSON.stringify(cutover)})
if (args[0] === 'run' && args[1] === 'runtime:go:packaged-gui-smoke') print(${JSON.stringify(guiSmoke)})
if (args[0] === 'run' && args[1] === 'runtime:go:packaged-soak') print(${JSON.stringify(sessionSoak)})
if (args[0] === 'run' && args[1] === 'runtime:go:packaged-milestone-a') print(${JSON.stringify(milestoneA)})
if (args[0] === 'run' && args[1] === 'runtime:go:rollback-retirement-evidence') print(${JSON.stringify(rollback)})
if (args[0] === 'run' && args[1] === 'test') process.exit(0)
console.error('unexpected npm command: ' + args.join(' '))
process.exit(1)
`)
      writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('0123456789abcdef0123456789abcdef01234567')
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

      const result = spawnSync(process.execPath, ['scripts/runtime-go-packaged-qa.mjs', '--json'], {
        cwd: process.cwd(),
        env: reportEnv({
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`,
          ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE: '1',
          ANALYTIX_RUNTIME_GO_PACKAGED_QA_EVIDENCE: output
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(existsSync(output)).toBe(true)
      const stdoutReport = JSON.parse(result.stdout) as Record<string, unknown>
      const fileReport = JSON.parse(readFileSync(output, 'utf8')) as Record<string, unknown>

      expect(stdoutReport).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-qa',
        status: 'passed',
        passed: true,
        outputPath: output
      }))
      expect(fileReport).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-qa',
        status: 'passed',
        passed: true,
        outputPath: output,
        packaged: expect.objectContaining({
          actualPackagedSmokeRequested: true,
          actualPackagedSmokePassed: true,
          actualPackagedDesktopQAPassed: true,
          actualPackagedAppLaunched: true,
          settingsProfilePatchAccepted: true,
          typeScriptRetiredBackendPassed: true,
          actualPackagedAppEvidenceRequiredForFinalGate: false,
          deterministicContractOnly: false,
          sessionSoakDeterministicContractOnly: false,
          actualPackagedSessionSoakRequested: true,
          actualPackagedSessionSoakPassed: true,
          sessionSoakActualEvidenceRequired: false,
          durableRestartPassed: true
        })
      }))
      expect((fileReport.packaged as Record<string, unknown>).missingFinalCoverage).toEqual([
        'credentialed-provider',
        'credentialed-mcp',
        'operator-gate'
      ])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('does not present GUI bridge contract tests as a real packaged app launch', () => {
    const report = runReport('scripts/runtime-go-packaged-gui-smoke.mjs')
    const smoke = report.smoke as Record<string, unknown>

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-gui-smoke',
      status: 'skipped',
      passed: false
    }))
    expect(smoke).toEqual(expect.objectContaining({
      deterministicContractOnly: true,
      actualPackagedAppLaunched: false,
      actualPackagedAppEvidenceRequiredForFinalGate: true,
      bridgePreserved: false,
      sseBridgePreserved: false,
      rendererTimelinePreserved: false,
      approvalAndUserInputCardsCovered: false,
      unobservedPackagedCoverage: [
        'sse-bridge',
        'renderer-timeline',
        'approval-card',
        'user-input-card'
      ]
    }))
  })

  it('does not present session contract soak as a real packaged app launch', () => {
    const report = runReport('scripts/runtime-go-packaged-session-soak.mjs')
    const soak = report.soak as Record<string, unknown>

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-session-soak',
      status: 'skipped',
      passed: false
    }))
    expect(soak).toEqual(expect.objectContaining({
      deterministicContractOnly: true,
      actualPackagedAppLaunched: false,
      actualPackagedAppEvidenceRequiredForFinalGate: true
    }))
    const durable = (report.checks as Array<Record<string, any>>)
      .find((check) => check.id === 'go-session-durable-contract')
    expect(durable).toEqual(expect.objectContaining({
      status: 'skipped',
      command: expect.stringContaining('go test -c -o <task-owned-test-binary> . [compile-once]')
    }))
    expect(durable?.command).toContain('-test.run ^TestRuntimeServerForkAndResumePreserveToolPairingHistory$')
    expect(durable?.command).toContain('TestRuntimeServerCandidateDurableRootHighRiskFailsClosedAndSurvivesRestart')
    expect(durable?.command).not.toContain('TestRuntimeServerCandidateDurableRootSurvivesRestartAndRestoresGates')
  })

  it('rejects exit-zero Go output that does not prove the exact durable session test', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-packaged-session-exact-go-'))
    try {
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      writeExecutable(fakeGo, '#!/usr/bin/env node\nprocess.exit(0)\n')
      const result = spawnSync(process.execPath, [
        'scripts/runtime-go-packaged-session-soak.mjs',
        '--json',
        '--no-gate',
        '--no-write'
      ], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-packaged-session-soak',
        status: 'failed',
        passed: false
      }))
      expect(report.checks[0]).toEqual(expect.objectContaining({
        id: 'go-session-durable-contract',
        status: 'failed',
        exitStatus: 0,
        reason: expect.stringContaining('expected Go test did not report an exact pass')
      }))
      expect(report.checks[0].subchecks).toEqual([
        expect.objectContaining({
          id: 'TestRuntimeServerForkAndResumePreserveToolPairingHistory',
          status: 'failed',
          exitStatus: 0,
          reason: 'expected Go test did not report an exact pass'
        })
      ])
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke', 'npm'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak', 'go']
  ] as const)('does not let a packaged live blocker hide a deterministic contract failure: %s', (script, actualKey, failingCommand) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-deterministic-failure-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { includeBuildAuthority: false })
      const fakeGo = join(dir, process.platform === 'win32' ? 'go.cmd' : 'go')
      const fakeNpm = join(dir, process.platform === 'win32' ? 'npm.cmd' : 'npm')
      writeExecutable(fakeGo, `#!/usr/bin/env node
const args = process.argv.slice(2)
const runIndex = Math.max(args.indexOf('-run'), args.indexOf('-test.run'))
const testName = runIndex >= 0 ? String(args[runIndex + 1] || '').replace(/^\\^|\\$$/g, '') : ''
if (${JSON.stringify(failingCommand)} === 'go') process.exit(91)
if (testName) console.log(JSON.stringify({ Action: 'pass', Package: 'analytix.local/runtime-go', Test: testName }))
process.exit(0)
`)
      writeExecutable(fakeNpm, `#!/usr/bin/env node
process.exit(${failingCommand === 'npm' ? 91 : 0})
`)
      const result = spawnSync(process.execPath, [
        script,
        '--json',
        '--no-gate',
        '--no-write',
        '--actual',
        '--app-path', fixture.appPath,
        '--target-platform', fixture.targetPlatform,
        '--target-arch', fixture.targetArch,
        '--expected-source-commit', fixture.sourceCommit
      ], {
        cwd: process.cwd(),
        env: reportEnv({
          GO: fakeGo,
          PATH: `${dir}${delimiter}${process.env.PATH || ''}`
        }),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      const report = JSON.parse(result.stdout) as Record<string, any>

      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      expect(report).toEqual(expect.objectContaining({
        status: 'failed',
        passed: false
      }))
      expect(report[actualKey]).toEqual(expect.objectContaining({
        status: 'live_blocked',
        passed: false
      }))
      expect(report.checks[0]).toEqual(expect.objectContaining({
        status: 'failed',
        exitStatus: 91
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it('allocates distinct nonzero debug and runtime ports for the packaged session', () => {
    const source = readFileSync('scripts/runtime-go-packaged-session-soak.mjs', 'utf8')
    expect(source).toContain('debugPort = await getFreePort()')
    expect(source).toContain('runtimePort = await getFreePort()')
    expect(source).toContain('while (runtimePort === debugPort) runtimePort = await getFreePort()')
  })

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('binds formal packaged evidence to exact content and never starts a second instance: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-package-evidence-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir)
      const report = runFormalPackagedRunner(script, fixture, dir)
      const actual = report[actualKey] as Record<string, any>
      const packagedSummary = report[actualKey === 'actualPackagedSmoke' ? 'smoke' : 'soak'] as Record<string, any>
      const checks = actual.checks as Array<Record<string, unknown>>

      expect(report).toEqual(expect.objectContaining({
        sourceCommit: fixture.sourceCommit,
        status: 'live_blocked',
        passed: false
      }))
      expect(report.testHarnessCommit).toMatch(/^[0-9a-f]{40}$/)
      expect(actual.app).toEqual(expect.objectContaining({
        ok: true,
        targetMatches: true,
        executableSha256: sha256Bytes(fixture.executable),
        appAsarSha256: sha256Bytes(fixture.appAsar),
        runtimeServerSha256: sha256Bytes(fixture.runtimeServer),
        executableArches: ['arm64'],
        runtimeServerArches: ['arm64'],
        buildAuthority: expect.objectContaining({
          ok: true,
          sourceCommit: fixture.sourceCommit,
          targetKey: 'darwin-arm64',
          classification: expect.stringMatching(/^controlled_release_(?:dirty|clean_candidate)_non_publishable$/),
          nativeDispositionKind: 'controlled_release_receipt',
          publishable: false,
          releaseEligible: false,
          publicationReceiptIssued: false,
          packagedButNonPublishable: true,
          releaseBlocker: 'packaged_build_authority_non_publishable'
        })
      }))
      expect(packagedSummary).toEqual(expect.objectContaining({
        actualPackagedAppEvidenceRequiredForFinalGate: true,
        packageClassification: actual.app.buildAuthority.classification,
        packagePublishable: false,
        packageReleaseEligible: false,
        packagePublicationReceiptIssued: false
      }))
      expect(actual.app).not.toHaveProperty('appPathHash')
      expect(actual.app).not.toHaveProperty('executablePathHash')
      expect(actual.app).not.toHaveProperty('runtimeServerBinaryPathHash')
      expect(actual.launch).toEqual(expect.objectContaining({
        actualPackagedAppLaunched: false,
        formalInstanceMode: true,
        secondInstanceStarted: false,
        temporaryUserData: false,
        syntheticCredentialUsed: false,
        targetKey: 'darwin-arm64',
        packageSourceCommit: fixture.sourceCommit,
        executableSha256: sha256Bytes(fixture.executable),
        appAsarSha256: sha256Bytes(fixture.appAsar),
        runtimeServerSha256: sha256Bytes(fixture.runtimeServer)
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'packaged-app-artifact',
        status: 'passed'
      }))
      expect(checks).toContainEqual(expect.objectContaining({
        id: 'packaged-app-launch',
        status: 'live_blocked',
        message: 'formal_instance_external_observation_required'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('binds a Windows x64 formal package without attempting to execute it on macOS: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-windows-package-evidence-'))
    try {
      const fixture = writeFormalWindowsPackageFixture(dir)
      const report = runFormalPackagedRunner(script, fixture, dir)
      const actual = report[actualKey] as Record<string, any>

      expect(report).toEqual(expect.objectContaining({
        sourceCommit: fixture.sourceCommit,
        status: 'live_blocked',
        passed: false
      }))
      expect(actual.app).toEqual(expect.objectContaining({
        ok: true,
        targetMatches: true,
        executableFormat: 'pe',
        runtimeServerFormat: 'pe',
        executableArches: ['x64'],
        runtimeServerArches: ['x64']
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        formalInstanceMode: true,
        secondInstanceStarted: false,
        temporaryUserData: false,
        targetKey: 'win32-x64'
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('rejects legacy v1 package authority before any launch: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-package-v1-authority-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { authorityVersion: 1 })
      const report = runFormalPackagedRunner(script, fixture, dir, [], { formal: false })
      const actual = report[actualKey] as Record<string, any>

      expect(actual.app).toEqual(expect.objectContaining({
        ok: false,
        blocked: true,
        buildAuthority: expect.objectContaining({
          ok: false,
          blocker: 'packaged_build_authority_version_unsupported'
        })
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        actualPackagedAppLaunched: false,
        secondInstanceStarted: false,
        temporaryUserData: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('rejects an internally valid authority bound to another worktree snapshot: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-package-stale-worktree-'))
    try {
      const current = currentPackagedWorktreeSnapshot()
      const staleWithoutDigest = JSON.parse(JSON.stringify(current)) as Record<string, any>
      delete staleWithoutDigest.snapshotDigest
      staleWithoutDigest.untrackedSource.sha256 = staleWithoutDigest.untrackedSource.sha256 === '0'.repeat(64)
        ? '1'.repeat(64)
        : '0'.repeat(64)
      const stale = {
        ...staleWithoutDigest,
        snapshotDigest: packagedAuthorityContract.packagedWorktreeSnapshotDigest(staleWithoutDigest)
      }
      const fixture = writeFormalMacPackageFixture(dir, { worktreeSnapshot: stale })
      const report = runFormalPackagedRunner(script, fixture, dir, [], { formal: false })
      const actual = report[actualKey] as Record<string, any>

      expect(actual.app).toEqual(expect.objectContaining({
        ok: false,
        blocked: true,
        buildAuthority: expect.objectContaining({
          ok: false,
          blocker: 'packaged_build_authority_worktree_snapshot_mismatch',
          worktreeSnapshotDigest: stale.snapshotDigest
        })
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        actualPackagedAppLaunched: false,
        secondInstanceStarted: false,
        temporaryUserData: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['darwin-arm64', writeFormalMacPackageFixture],
    ['win32-x64', writeFormalWindowsPackageFixture]
  ] as const)('writes a strict stable-read v2 packaged authority for %s', (targetKey, fixtureWriter) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-after-pack-v2-authority-'))
    try {
      const fixture = fixtureWriter(dir, { includeBuildAuthority: false } as never)
      const nativeReceiptPath = join(fixture.runtimeDir, 'analytix-native-components-receipt.json')
      const context = {
        appOutDir: fixture.appOutDir,
        electronPlatformName: fixture.targetPlatform,
        arch: fixture.targetArch,
        packager: {
          appInfo: { productFilename: 'analytix' },
          config: { executableName: 'analytix' },
          platformSpecificBuildOptions: { executableName: 'analytix' }
        }
      }
      const fundsPluginAdmission = packagedAuthorityContract.validateBundledFundsPlugin(context)
      const authority = packagedAuthorityContract.writePackagedBuildAuthorityV2(context, {
        targetKey,
        receiptSHA256: sha256Bytes(readFileSync(nativeReceiptPath)),
        manifestSHA256: 'b'.repeat(64),
        signingPolicySHA256: targetKey.startsWith('darwin-') ? 'c'.repeat(64) : '',
        signingMode: targetKey.startsWith('darwin-') ? 'ad-hoc' : '',
        appleTeamIdentifier: ''
      }, {
        worktreeSnapshot: currentPackagedWorktreeSnapshot(),
        env: {},
        fundsPluginAdmission
      }) as Record<string, any>
      const authorityText = readFileSync(join(fixture.runtimeDir, PACKAGED_BUILD_AUTHORITY_FILE), 'utf8')

      expect(PACKAGED_BUILD_AUTHORITY_CONTRACT).toBe('analytix.packaged-build-authority/v2')
      expect(authorityText).toBe(JSON.stringify(authority))
      expect(packagedAuthorityContract.isPackagedBuildAuthorityV2(authority)).toBe(true)
      expect(authority).toEqual(expect.objectContaining({
        schemaVersion: 2,
        contract: PACKAGED_BUILD_AUTHORITY_CONTRACT,
        sourceCommit: fixture.sourceCommit,
        targetKey,
        classification: expect.stringMatching(/^controlled_release_(?:dirty|clean_candidate)_non_publishable$/),
        publishable: false,
        releaseEligible: false,
        publicationReceiptIssued: false,
        nativeDisposition: expect.objectContaining({
          kind: 'controlled_release_receipt',
          targetKey,
          receiptSha256: sha256Bytes(readFileSync(nativeReceiptPath))
        }),
        artifacts: expect.objectContaining({
          executable: expect.objectContaining({
            preSignSha256: sha256Bytes(fixture.executable),
            preSignByteLength: fixture.executable.length,
            arch: fixture.targetArch
          }),
          appAsar: { sha256: sha256Bytes(fixture.appAsar), byteLength: fixture.appAsar.length },
          runtimeServer: expect.objectContaining({
            preSignSha256: sha256Bytes(fixture.runtimeServer),
            preSignByteLength: fixture.runtimeServer.length,
            arch: fixture.targetArch
          }),
          fundsPlugin: expect.objectContaining({
            treeSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
            fileCount: expect.any(Number)
          })
        }),
        buildContext: expect.objectContaining({
          contract: 'analytix.electron-builder-effective-context/v1',
          contextDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
        }),
        stagedPayload: expect.objectContaining({
          contract: 'analytix.packaged-staged-payload/v1',
          manifestSha256: expect.stringMatching(/^[0-9a-f]{64}$/)
        })
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('rejects a hard-linked app.asar during authority issuance and verification', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-after-pack-app-asar-hardlink-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { includeBuildAuthority: false })
      const context = {
        appOutDir: fixture.appOutDir,
        electronPlatformName: fixture.targetPlatform,
        arch: fixture.targetArch,
        packager: {
          appInfo: { productFilename: 'analytix' },
          config: { executableName: 'analytix' },
          platformSpecificBuildOptions: { executableName: 'analytix' }
        }
      }
      const nativeReceiptPath = join(
        fixture.runtimeDir,
        'analytix-native-components-receipt.json'
      )
      const nativeTrust = {
        targetKey: 'darwin-arm64',
        receiptSHA256: sha256Bytes(readFileSync(nativeReceiptPath)),
        manifestSHA256: 'b'.repeat(64),
        signingPolicySHA256: 'c'.repeat(64),
        signingMode: 'ad-hoc',
        appleTeamIdentifier: ''
      }
      const fundsPluginAdmission = packagedAuthorityContract.validateBundledFundsPlugin(context)
      const appAsarPath = packagedAuthorityContract.packagedAppAsarPath(context)
      const hardLinkPath = join(dir, 'app-asar-hardlink')
      linkSync(appAsarPath, hardLinkPath)

      expect(() => packagedAuthorityContract.writePackagedBuildAuthorityV2(
        context,
        nativeTrust,
        { worktreeSnapshot: currentPackagedWorktreeSnapshot(), env: {}, fundsPluginAdmission }
      )).toThrow(/Untrusted regular file identity/)

      rmSync(hardLinkPath)
      const authority = packagedAuthorityContract.writePackagedBuildAuthorityV2(
        context,
        nativeTrust,
        { worktreeSnapshot: currentPackagedWorktreeSnapshot(), env: {}, fundsPluginAdmission }
      )
      linkSync(appAsarPath, hardLinkPath)

      expect(() => packagedAuthorityContract.verifyPackagedBuildAuthorityArtifacts(
        context,
        authority,
        { verifyFuses: false }
      )).toThrow(/Untrusted regular file identity/)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it.each([
    ['macOS arm64', 'darwin', 'arm64'],
    ['Windows x64 contract-only', 'win32', 'x64']
  ] as const)('atomically materializes a host-generated development native marker for %s', (
    _label,
    platform,
    arch
  ) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-development-native-package-'))
    try {
      const fixture = writeDevelopmentNativeFixture(dir, platform, arch)
      const fundsPluginAdmission = packagedAuthorityContract.validateBundledFundsPlugin(
        fixture.context
      )
      const disposition = packagedAuthorityContract.materializePackagedDevelopmentNativeComponents(
        fixture.context,
        { repoRoot: dir, verifyCurrentSource: false }
      ) as Record<string, any>

      expect(disposition).toEqual(expect.objectContaining({
        kind: NATIVE_DISPOSITION_DEVELOPMENT,
        targetKey: fixture.targetKey,
        marker: expect.objectContaining({ sha256: expect.stringMatching(/^[0-9a-f]{64}$/) }),
        components: expect.arrayContaining(
          fixture.binaries.map(({ id, binaryName }) => expect.objectContaining({ id, binaryName }))
        )
      }))
      expect(existsSync(join(fixture.runtimeDir, 'analytix-native-components-receipt.json'))).toBe(false)
      expect(readFileSync(join(fixture.runtimeDir, NATIVE_DEVELOPMENT_BUILD_MARKER))).toEqual(
        readFileSync(join(fixture.sourceDir, NATIVE_DEVELOPMENT_BUILD_MARKER))
      )
      for (const binary of fixture.binaries) {
        expect(readFileSync(join(fixture.runtimeDir, binary.binaryName))).toEqual(binary.bytes)
      }
      expect(packagedAuthorityContract.verifyPackagedDevelopmentNativeDisposition(
        fixture.context,
        disposition,
        { repoRoot: dir, verifyCurrentSource: false, requireMarkerBinaryIdentity: true }
      )).toEqual(expect.objectContaining({ disposition }))

      const authority = packagedAuthorityContract.writePackagedBuildAuthorityV2(
        fixture.context,
        disposition,
        {
          repoRoot: dir,
          verifyCurrentSource: false,
          worktreeSnapshot: currentPackagedWorktreeSnapshot(),
          env: {},
          fundsPluginAdmission
        }
      ) as Record<string, any>
      expect(authority).toEqual(expect.objectContaining({
        classification: expect.stringMatching(/^development_(?:dirty|clean)_non_publishable$/),
        publishable: false,
        releaseEligible: false,
        publicationReceiptIssued: false,
        nativeDisposition: disposition
      }))
      expect(packagedAuthorityContract.readPackagedBuildAuthorityV2(fixture.context)).toEqual(authority)
      expect(packagedAuthorityContract.verifyPackagedBuildAuthorityArtifacts(
        fixture.context,
        authority,
        { verifyFuses: false }
      )).toEqual(expect.objectContaining({
        executable: expect.objectContaining({ format: fixture.format, arch: fixture.arch }),
        runtimeServer: expect.objectContaining({ format: fixture.format, arch: fixture.arch })
      }))
      expect(() => packagedAuthorityContract.writePackagedBuildAuthorityV2(
        fixture.context,
        disposition,
        {
          repoRoot: dir,
          verifyCurrentSource: false,
          env: { ANALYTIX_RELEASE_BUILD: '1' },
          fundsPluginAdmission
        }
      )).toThrow(/packaged_development_native_disposition_forbidden_for_release/)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('keeps development native packages local-signable but outside Developer ID and notarization', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-development-native-signing-boundary-'))
    try {
      const fixture = writeDevelopmentNativeFixture(dir, 'darwin', 'arm64')
      const fundsPluginAdmission = packagedAuthorityContract.validateBundledFundsPlugin(
        fixture.context
      )
      const disposition = packagedAuthorityContract.materializePackagedDevelopmentNativeComponents(
        fixture.context,
        { repoRoot: dir, verifyCurrentSource: false }
      ) as Record<string, any>
      const authority = packagedAuthorityContract.writePackagedBuildAuthorityV2(
        fixture.context,
        disposition,
        {
          repoRoot: dir,
          verifyCurrentSource: false,
          worktreeSnapshot: currentPackagedWorktreeSnapshot(),
          env: {},
          fundsPluginAdmission
        }
      ) as Record<string, any>

      expect(macNotarize._internals.assertNativeDispositionSigningBoundary(authority, {
        requireDeveloperID: false
      })).toBe(NATIVE_DISPOSITION_DEVELOPMENT)
      expect(() => macNotarize._internals.assertNativeDispositionSigningBoundary(authority, {
        requireDeveloperID: true
      })).toThrow(/cannot enter Developer ID or notarization/)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('builds the development runtime with analytix_prod and only compiled ad-hoc local policy', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-development-runtime-build-'))
    try {
      const fixture = writeDevelopmentNativeFixture(dir, 'darwin', 'arm64')
      const disposition = packagedAuthorityContract.materializePackagedDevelopmentNativeComponents(
        fixture.context,
        { repoRoot: dir, verifyCurrentSource: false }
      ) as Record<string, any>
      const calls: Array<{ command: string; args: string[] }> = []
      const moduleCache = join(dir, 'go-module-cache')
      mkdirSync(moduleCache, { recursive: true })

      packagedAuthorityContract.buildBundledGoRuntimeServer(fixture.context, disposition, {
        toolchain: {
          key: 'darwin-arm64',
          executable: '/trusted/go/bin/go',
          moduleCache
        },
        execFileSync(command: string, args: string[]) {
          calls.push({ command, args })
          const outputIndex = args.indexOf('-o')
          if (outputIndex >= 0) {
            writeFileSync(args[outputIndex + 1]!, 'runtime-server', { mode: 0o755 })
          }
        },
        chmodSync() {}
      })

      expect(calls).toHaveLength(2)
      const build = calls[1]!
      expect(build.args).toContain('analytix_prod')
      const ldflags = build.args[build.args.indexOf('-ldflags') + 1]!
      for (const key of [
        'embeddedReceiptSHA256',
        'embeddedManifestSHA256',
        'embeddedTargetKey',
        'embeddedSigningPolicySHA256',
        'embeddedSigningMode',
        'embeddedAppleTeamIdentifier'
      ]) {
        expect(ldflags).toContain(`nativecomponentregistry.${key}=`)
      }
      expect(ldflags).not.toContain(disposition.marker.sha256)
      expect(ldflags).not.toContain(fixture.targetKey)
      expect(ldflags).toContain('nativecomponentregistry.embeddedSigningPolicySHA256=7625f1de94282690dc72ce92f7e42a293437c24fb9f4a725ff82bfec157ecaf3')
      expect(ldflags).toContain('nativecomponentregistry.embeddedSigningMode=ad-hoc')
      expect(ldflags).toContain('nativecomponentregistry.embeddedAppleTeamIdentifier=')
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('accepts only a controlled native receipt at the pre-sign formal intent boundary', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-after-pack-formal-reject-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { includeBuildAuthority: false })
      const nativeReceiptPath = join(fixture.runtimeDir, 'analytix-native-components-receipt.json')
      const context = {
        appOutDir: fixture.appOutDir,
        electronPlatformName: fixture.targetPlatform,
        arch: fixture.targetArch,
        packager: {
          appInfo: { productFilename: 'analytix' },
          config: { executableName: 'analytix' },
          platformSpecificBuildOptions: { executableName: 'analytix' }
        }
      }
      const fundsPluginAdmission = packagedAuthorityContract.validateBundledFundsPlugin(context)
      const authority = packagedAuthorityContract.writePackagedBuildAuthorityV2(context, {
        targetKey: 'darwin-arm64',
        receiptSHA256: sha256Bytes(readFileSync(nativeReceiptPath)),
        manifestSHA256: 'b'.repeat(64),
        signingPolicySHA256: 'c'.repeat(64),
        signingMode: 'developer-id',
        appleTeamIdentifier: 'A1B2C3D4E5'
      }, {
        worktreeSnapshot: cleanFixtureWorktreeSnapshot(dir),
        env: { MAC_SIGN: '1' },
        fundsPluginAdmission
      }) as Record<string, any>
      expect(authority).toEqual(expect.objectContaining({
        nativeDisposition: expect.objectContaining({ kind: 'controlled_release_receipt' }),
        publishable: false,
        releaseEligible: false,
        publicationReceiptIssued: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 15_000)

  it('binds tracked and untracked source bytes while excluding generated and credential material', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-worktree-snapshot-v1-'))
    try {
      runGit(dir, ['init', '--quiet'])
      writeFileSync(join(dir, '.gitignore'), [
        'dist/',
        'out/',
        'node_modules/',
        'logs/',
        '.env.local',
        'credentials.pem',
        'docs/analytix/upstreams/runtime-go-live-evidence/',
        'ignored.ts',
        ''
      ].join('\n'), 'utf8')
      writeFileSync(join(dir, 'base.ts'), 'export const value = 1\n', 'utf8')
      runGit(dir, ['add', '.gitignore', 'base.ts'])
      runGit(dir, ['-c', 'user.name=Analytix Test', '-c', 'user.email=test@analytix.invalid', 'commit', '--quiet', '-m', 'fixture'])
      const clean = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      expect(clean).toEqual(expect.objectContaining({ state: 'clean', dirty: false }))

      for (const [relativePath, content] of [
        ['dist/generated.ts', 'dist bytes'],
        ['out/generated.ts', 'out bytes'],
        ['node_modules/example/index.ts', 'dependency bytes'],
        ['logs/runtime.ts', 'log bytes'],
        ['.env.local', 'TOKEN=secret'],
        ['credentials.pem', 'private material'],
        ['docs/analytix/upstreams/runtime-go-live-evidence/result.json', '{"generated":true}'],
        ['ignored.ts', 'ignored bytes']
      ]) {
        const path = join(dir, relativePath)
        mkdirSync(join(path, '..'), { recursive: true })
        writeFileSync(path, content, 'utf8')
      }
      const exclusionsOnly = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      expect(exclusionsOnly).toEqual(clean)

      const untrackedPath = join(dir, 'feature.ts')
      writeFileSync(untrackedPath, 'export const feature = 1\n', 'utf8')
      const untrackedOne = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      writeFileSync(untrackedPath, 'export const feature = 2\n', 'utf8')
      const untrackedTwo = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      expect(untrackedOne).toEqual(expect.objectContaining({ state: 'dirty', dirty: true }))
      expect(untrackedOne.untrackedSource.count).toBe(1)
      expect(untrackedTwo.snapshotDigest).not.toBe(untrackedOne.snapshotDigest)

      rmSync(untrackedPath)
      writeFileSync(join(dir, 'base.ts'), 'export const value = 2\n', 'utf8')
      const unstaged = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      expect(unstaged.unstagedPatch.count).toBe(1)
      expect(unstaged.snapshotDigest).not.toBe(clean.snapshotDigest)
      runGit(dir, ['add', 'base.ts'])
      const staged = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(dir) as Record<string, any>
      expect(staged.stagedPatch.count).toBe(1)
      expect(staged.unstagedPatch.count).toBe(0)
      expect(staged.snapshotDigest).not.toBe(unstaged.snapshotDigest)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('mechanically blocks actual evidence before launch when packaged build authority is absent: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-package-authority-missing-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { includeBuildAuthority: false })
      const report = runFormalPackagedRunner(script, fixture, dir, [], { formal: false })
      const actual = report[actualKey] as Record<string, any>

      expect(report).toEqual(expect.objectContaining({
        sourceCommit: '',
        status: 'live_blocked',
        passed: false
      }))
      expect(actual.app).toEqual(expect.objectContaining({
        ok: false,
        blocked: true,
        executableSha256: sha256Bytes(fixture.executable),
        appAsarSha256: sha256Bytes(fixture.appAsar),
        runtimeServerSha256: sha256Bytes(fixture.runtimeServer),
        buildAuthority: expect.objectContaining({
          ok: false,
          blocked: true,
          blocker: 'packaged_build_authority_missing'
        })
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        secondInstanceStarted: false,
        temporaryUserData: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('blocks a stale package whose authority source commit is not the explicitly requested source: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-stale-formal-package-evidence-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir)
      const report = runFormalPackagedRunner(script, fixture, dir, [], {
        expectedSourceCommit: 'c'.repeat(40)
      })
      const actual = report[actualKey] as Record<string, any>

      expect(report).toEqual(expect.objectContaining({
        sourceCommit: fixture.sourceCommit,
        status: 'live_blocked',
        passed: false
      }))
      expect(actual.app).toEqual(expect.objectContaining({
        ok: false,
        blocked: true,
        buildAuthority: expect.objectContaining({
          ok: false,
          sourceCommit: fixture.sourceCommit,
          blocker: 'packaged_build_authority_incomplete_or_mismatched'
        })
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        secondInstanceStarted: false,
        temporaryUserData: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it.each([
    ['scripts/runtime-go-packaged-gui-smoke.mjs', 'actualPackagedSmoke'],
    ['scripts/runtime-go-packaged-session-soak.mjs', 'actualPackagedSessionSoak']
  ] as const)('rejects an x64 package selected for an arm64 formal target: %s', (script, actualKey) => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-formal-package-arch-mismatch-'))
    try {
      const fixture = writeFormalMacPackageFixture(dir, { arch: 'x64' })
      const report = runFormalPackagedRunner(script, { ...fixture, targetArch: 'arm64' }, dir)
      const actual = report[actualKey] as Record<string, any>

      expect(report).toEqual(expect.objectContaining({ status: 'failed', passed: false }))
      expect(actual.app).toEqual(expect.objectContaining({
        ok: false,
        blocked: false,
        targetMatches: false,
        executableArches: ['x64'],
        runtimeServerArches: ['x64']
      }))
      expect(actual.launch).toEqual(expect.objectContaining({
        actualPackagedAppLaunched: false,
        secondInstanceStarted: false,
        temporaryUserData: false
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  }, 20_000)

  it('uses one exact default package path per target instead of cross-architecture fallback candidates', () => {
    for (const script of [
      'scripts/runtime-go-packaged-gui-smoke.mjs',
      'scripts/runtime-go-packaged-session-soak.mjs'
    ]) {
      const source = readFileSync(script, 'utf8')
      expect(source).toContain("target.arch === 'arm64' ? 'dist/mac-arm64/analytix.app' : 'dist/mac/analytix.app'")
      expect(source).not.toContain('const candidates =')
      expect(source).not.toContain('appPathHash:')
      expect(source).not.toContain('executablePathHash:')
      expect(source).not.toContain('runtimeServerBinaryPathHash:')
      expect(source).not.toContain("'ELECTRON_RENDERER_URL'")
      expect(source).toContain('packageReleaseAuthorityAccepted')
      expect(source).toContain('!packageReleaseAuthorityAccepted')
      expect(source).not.toContain("const PACKAGED_BUILD_AUTHORITY_CONTRACT = 'analytix.packaged-build-authority/v1'")
    }
    const afterPackSource = readFileSync('scripts/after-pack.cjs', 'utf8')
    expect(afterPackSource).toContain('writePackagedBuildAuthorityV2(context, nativeDisposition, {')
    expect(afterPackSource).toContain("const PACKAGED_BUILD_AUTHORITY_CONTRACT = 'analytix.packaged-build-authority/v2'")
  })

  it('keeps local validation actual packaged smoke evidence separate from deterministic soak evidence', () => {
    const report = runReport('scripts/runtime-go-local-validation.mjs', [
      '--actual',
      '--app-path=/tmp/Analytix.app',
      '--timeout-ms',
      '1234'
    ])
    const local = report.local as Record<string, unknown>
    const checks = report.checks as Array<Record<string, unknown>>
    const guiSmoke = checks.find((item) => item.id === 'gui-smoke')

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-local-validation',
      status: 'skipped',
      passed: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      notFinalUntil: expect.arrayContaining([
        'runtime:go:preflight -- --gate passes',
        'credentialed DeepSeek and at least one non-DeepSeek provider evidence passes',
        'operator approval evidence passes',
        'relevant evidence scope is clean or committed'
      ])
    }))
    expect(guiSmoke?.command).toContain('runtime:go:packaged-gui-smoke')
    expect(guiSmoke?.command).toContain('--actual')
    expect(guiSmoke?.command).toContain('--app-path=/tmp/Analytix.app')
    expect(guiSmoke?.command).toContain('--timeout-ms 1234')
    expect(local).toEqual(expect.objectContaining({
      actualPackagedSmokeRequested: true,
      actualPackagedSmokePassed: false,
      actualPackagedGuiEvidenceRequiredForFinalGate: false,
      sessionSoakDeterministicOnly: false,
      deterministicContractOnly: false,
      finalAcceptanceReady: false,
      finalAcceptanceStatus: 'blocked',
      notFinalUntil: expect.arrayContaining([
        'runtime:go:preflight -- --gate passes',
        'credentialed DeepSeek and at least one non-DeepSeek provider evidence passes',
        'operator approval evidence passes',
        'relevant evidence scope is clean or committed'
      ]),
      actualPackagedAppLaunched: false,
      actualPackagedAppEvidenceRequiredForFinalGate: false
    }))
  })

  it('applies the packaged QA timeout to each aggregate child command', () => {
    const source = readFileSync(join(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs'), 'utf8')

    expect(source).toContain('const commandTimeoutMs = Number(optionValue')
    expect(source).toContain('timeout: commandTimeoutMs')
    expect(source).toContain("appendOptionPassThrough(packagedGuiSmokeArgs, '--timeout-ms')")
    expect(source).toContain("appendOptionPassThrough(packagedSessionSoakArgs, '--timeout-ms')")
  })
})

function writeJSON(path: string, value: unknown): void {
  writeFileSync(path, JSON.stringify(value), 'utf8')
}

function providerEvidence(
  credentialedProbes: Array<Record<string, unknown>>,
  options: { decoratePassedProbes?: boolean; readsRealApiKeysByDefault?: boolean } = {}
): Record<string, unknown> {
  const decoratePassedProbes = options.decoratePassedProbes !== false
  return {
    id: 'runtime-go-provider-matrix',
    status: 'passed',
    passed: true,
	    credentialSecretsRecorded: false,
	    credentialedMatrixEnvGated: true,
	    readsRealApiKeysByDefault: options.readsRealApiKeysByDefault === true,
	    credentialValuesRecorded: false,
	    redaction: { status: 'passed', secretMaterialFound: false },
    requiredCredentialedProviderCoverage: ['deepseek', 'at-least-one-non-deepseek-provider'],
    credentialedProbes: decoratePassedProbes
      ? credentialedProbes.map(providerProbeEvidence)
      : credentialedProbes
  }
}

function providerProbeEvidence(probe: Record<string, unknown>): Record<string, unknown> {
  if (probe.status !== 'passed' || probe.skipped === true || probe.credentialed !== true) {
    return probe
  }
  return {
    label: String(probe.id ?? 'provider'),
    family: 'openai-chat-completions',
    endpointFamily: 'openai-chat-completions',
    endpointFormat: 'openai-chat-completions',
    requestUrl: 'https://provider.example/v1/chat/completions',
    requestUrlHash: '0'.repeat(64),
    missingEnv: [],
    redaction: {
      status: 'passed',
      credentialEnvNames: ['ANALYTIX_RUNTIME_PROVIDER_API_KEY'],
      credentialValueRecorded: false
    },
    provider: {
      providerId: probe.id,
      model: `${String(probe.id ?? 'provider')}-model`,
      hasApiKey: true,
      source: 'env'
    },
    requestShape: {
      method: 'POST',
      endpointFamily: 'openai-chat-completions',
      endpointFormat: 'openai-chat-completions',
      credentialConfigured: true,
      credentialValueRecorded: false,
      streamRequested: true
    },
    requestShapeValidation: {
      status: 'passed',
      missing: []
    },
    streamParsing: {
      status: 'passed',
      mode: 'sse',
      frameCount: 2,
      jsonFrameCount: 1,
      parseErrorCount: 0
    },
    usageParsing: {
      status: 'passed',
      usageObjectPresent: true,
      tokenFields: ['prompt_tokens', 'completion_tokens', 'total_tokens']
    },
    cacheTelemetry: {
      status: 'passed',
      providerScoped: probe.id === 'deepseek',
      deepSeekSpecificRequestFieldsPresent: false
    },
    errorHandling: {
      status: 'passed',
      httpStatus: 400,
      errorProbePassed: true,
      requestShape: {
        method: 'POST',
        sameRequestUrl: true,
        intentionallyInvalidBody: true,
        credentialValueRecorded: false,
        requestBodyRecorded: false
      },
      rawResponseRecorded: false,
      credentialValueRecorded: false
    },
    ...probe
  }
}

function mcpEvidence(options: { includeProbeDetails?: boolean } = {}): Record<string, unknown> {
  const includeProbeDetails = options.includeProbeDetails !== false
  const probe: Record<string, unknown> = {
    id: 'credentialed-mcp',
    status: 'passed',
    skipped: false,
    credentialedExecution: true,
    connect: true,
    toolDiscoverySearch: true,
    toolCall: true,
    approvalUserInput: true,
    reconnect: true,
    credentialRedaction: true,
    commandConfigured: true
  }
  if (includeProbeDetails) {
    Object.assign(probe, {
      requestShape: {
        protocol: 'mcp-jsonrpc-over-stdio',
        methods: ['initialize', 'notifications/initialized', 'tools/list', 'tools/call'],
        credentialValueRecorded: false,
        toolArgsValueRecorded: false
      },
      calledToolNameHash: '1'.repeat(64),
      toolCatalogDigest: '2'.repeat(64),
      reconnectToolNameHash: '1'.repeat(64),
      reconnectToolCatalogDigest: '2'.repeat(64),
      approvalUserInputEvidence: {
        status: 'passed',
        evidenceSha256: '3'.repeat(64),
        evidenceDigestAlgorithm: 'sha256:file-bytes-v1',
        rawValueRecorded: false,
        credentialSecretsRecorded: false
      }
    })
  }
  return {
    id: 'runtime-go-mcp-execution',
    status: 'passed',
    passed: true,
    credentialedExecution: true,
    topLevelMcpIndexerExposed: false,
    reasonixPublicProtocolUsed: false,
    redaction: true,
    credentialedProbes: [probe]
  }
}

function operatorEvidence(options: { includeReviewDetails?: boolean } = {}): Record<string, unknown> {
  const includeReviewDetails = options.includeReviewDetails !== false
  return {
    id: 'runtime-go-operator-gate',
    status: 'passed',
    passed: true,
    operatorGate: 'ANALYTIX_RUNTIME_READY=1',
    explicitEnvGate: true,
    credentialedEvidenceReviewed: true,
    defaultRuntimeApproved: true,
    goDefaultApproved: true,
    typeScriptFallbackRetained: false,
    goDefaultBackendEnabled: true,
    rendererVisibleGoSwitcher: false,
    reasonixEngineRuntimeOnly: true,
    ...(includeReviewDetails
      ? {
          commitHash: '0123456789abcdef0123456789abcdef01234567',
          evidenceTargetCommit: '0123456789abcdef0123456789abcdef01234567',
          evidenceDigestAlgorithm: 'sha256:canonical-json-v1',
          evidenceDigests: {
            provider: '4'.repeat(64),
            mcp: '5'.repeat(64),
            packaged: '6'.repeat(64)
          },
          reportPaths: {
            provider: '/tmp/provider.json',
            mcp: '/tmp/mcp.json',
            packaged: '/tmp/packaged.json'
          },
          evidenceReviewed: [
            { id: 'provider-matrix-credentialed', status: 'passed', path: '/tmp/provider.json' },
            { id: 'mcp-matrix-credentialed', status: 'passed', path: '/tmp/mcp.json' },
            { id: 'packaged-qa', status: 'passed', path: '/tmp/packaged.json' }
          ]
        }
      : {})
  }
}

function packagedAggregateEvidence(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const packagedOverrides = overrides.packaged &&
    typeof overrides.packaged === 'object' &&
    !Array.isArray(overrides.packaged)
    ? overrides.packaged as Record<string, unknown>
    : {}
  const report = {
    id: 'runtime-go-packaged-qa',
    status: 'passed',
    passed: true,
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false },
    packaged: {
      cutoverReady: true,
      cutoverFinalAcceptanceReady: false,
      finalAcceptanceReady: false,
      missingFinalCoverage: ['credentialed-provider', 'credentialed-mcp', 'operator-gate'],
      runtimeHealthPassed: true,
      runtimeHealthRuntimeInfoOk: true,
      runtimeHealthRuntimeToolsOk: true,
      runtimeHealthProductionCapabilitiesOk: true,
      guiSmokePassed: true,
      sessionSoakPassed: true,
      actualPackagedSmokeRequested: true,
      actualPackagedSmokePassed: true,
      actualPackagedDesktopQAPassed: true,
      actualPackagedAppLaunched: true,
      settingsProfilePatchAccepted: true,
      settingsCompatPatchUsed: false,
      sessionSoakDeterministicContractOnly: false,
      actualPackagedSessionSoakRequested: true,
      actualPackagedSessionSoakPassed: true,
      sessionSoakActualEvidenceRequired: false,
      typeScriptRetiredBackendPassed: true,
      actualPackagedAppEvidenceRequiredForFinalGate: false,
      packagingConfigPassed: true,
      directLegacyExposureCount: 0,
      internalLegacyDelegateCount: 2,
      temporaryGoProdViolationCount: 0,
      productionGoSourceCount: 28,
      retiredProductionMarkerViolationCount: 0,
      legacyUpstreamFieldKeyViolationCount: 0,
      ...packagedOverrides
    },
    checks: [
      'cutover-report',
      'gui-smoke',
      'session-soak',
      'rollback-evidence',
      'packaging-config'
    ].map((id) => ({ id, status: 'passed' }))
  }
  return {
    ...report,
    ...overrides,
    packaged: {
      ...report.packaged,
      ...packagedOverrides
    }
  }
}

it('R130 aggregate requires normal local authority and preserves every provider receipt gate', async () => {
  // @ts-expect-error The production harness helper is JavaScript.
  const { localProviderPublicSeamPassed } = await import('../../scripts/lib/local-provider-acceptance.mjs')
  const report: Record<string, any> = {
    syntheticProviderUsed: false, localProviderUsed: false,
    provider: {
      configured: true, credentialConfigured: true, credentialAuthorityBound: true,
      credentialAuthority: 'local-provider-registry-secret-store', normalLocalProviderSetupObserved: true,
      localCredentialEvidence: { ok: true, blocked: false, blocker: null, sourceBound: true, entryMethod: 'visible-computer-use', automatedCredentialEntryUsed: true, expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1, status: 'passed', scannedFileCount: 3, findingCount: 0, settingsFindingCount: 0, reportFindingCount: 0, unsafeEntryCount: 0, symlinkCount: 0, encodingCoverage: ['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'], protectedStoreOwnerPrivate: true },
      credentialRecorded: false, networkTurnCompleted: true, threadProviderBound: true, threadModelBound: true,
      resultTurnProviderReceiptBound: true, providerAttemptTelemetryValid: true,
      providerAttemptReceiptDigest: 'a'.repeat(64), providerLogicalCallCount: 1,
      providerAttemptCount: 1, successfulProviderAttemptCount: 1
    }
  }
  expect(localProviderPublicSeamPassed(report)).toBe(true)
  for (const field of ['configured', 'credentialConfigured', 'credentialAuthorityBound',
    'normalLocalProviderSetupObserved', 'networkTurnCompleted', 'threadProviderBound',
    'threadModelBound', 'resultTurnProviderReceiptBound', 'providerAttemptTelemetryValid']) {
    expect(localProviderPublicSeamPassed({ ...report, provider: { ...report.provider, [field]: false } })).toBe(false)
  }
  expect(localProviderPublicSeamPassed({ ...report, provider: { ...report.provider, normalHubLoginObserved: true } })).toBe(false)
  for (const change of [{ sourceSecretCount: 0 }, { sourceBound: false }, { scannedFileCount: 0 },
    { status: 'blocked' }, { findingCount: 1 }, { encodingCoverage: ['utf8'] }, { rawCredential: 'unrelated-canary' }]) {
    expect(localProviderPublicSeamPassed({ ...report, provider: { ...report.provider,
      localCredentialEvidence: { ...report.provider.localCredentialEvidence, ...change } } })).toBe(false)
  }
  expect(readFileSync(join(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs'), 'utf8'))
    .toContain('localProviderPublicSeamPassed(milestoneA)')
})
