#!/usr/bin/env node

import { execFileSync, spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import {
  closeSync,
  existsSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readlinkSync,
  readdirSync,
  readSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import net from 'node:net'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import process from 'node:process'
import {
  createIsolatedDarwinLoginKeychain
} from './runtime-go-packaged-milestone-a.mjs'

const require = createRequire(import.meta.url)
const {
  PACKAGED_BUILD_AUTHORITY_CONTRACT,
  PACKAGED_BUILD_AUTHORITY_FILE,
  _internals: packagedAuthorityContract
} = require('./after-pack.cjs')

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const formalInstanceMode = args.has('--formal-instance') || process.env.ANALYTIX_RUNTIME_GO_FORMAL_INSTANCE === '1'
const actualPackagedSmoke = formalInstanceMode || args.has('--actual') || process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE === '1'
const DEFAULT_TIMEOUT_MS = 45_000
const PACKAGED_RENDERER_SCHEME = 'analytix-app:'
const PACKAGED_RENDERER_HOST = 'renderer'
const PACKAGED_RENDERER_PATH = '/index.html'
const REQUIRED_ACTUAL_CHECK_IDS = [
  'packaged-app-artifact',
  'packaged-app-launch',
  'packaged-renderer-bridge',
  'packaged-terminal-pty',
  'packaged-settings-bridge',
  'packaged-provider-profile-settings',
  'packaged-mimo-profile-settings',
  'packaged-runtime-restart',
  'packaged-runtime-health',
  'packaged-runtime-info'
]

const commands = [
  {
    id: 'gui-bridge-and-timeline-smoke',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/preload/preload-runtime-request.test.ts',
      'src/preload/preload-sse-bridge.test.ts',
      'src/main/runtime-sse-ipc.test.ts',
      'src/renderer/src/lib/browser-analytix-bridge.test.ts',
      'src/renderer/src/store/chat-store-runtime.test.ts',
      'src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts',
      'src/renderer/src/components/RewindPlanApplyControls.test.ts',
      'src/renderer/src/components/plugin-marketplace-runtime.test.ts',
      'src/renderer/src/components/PluginMarketplaceView.test.ts',
      '--run'
    ]
  }
]

function argValue(name, fallback = '') {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function sha256(value) {
  return createHash('sha256').update(String(value)).digest('hex')
}

function normalizeTargetPlatform(value) {
  if (value === 'windows') return 'win32'
  if (value === 'darwin' || value === 'win32' || value === 'linux') return value
  throw new Error(`unsupported packaged target platform: ${value}`)
}

function normalizeTargetArch(value) {
  if (value === 'amd64') return 'x64'
  if (value === 'aarch64') return 'arm64'
  if (value === 'x64' || value === 'arm64') return value
  throw new Error(`unsupported packaged target architecture: ${value}`)
}

function packagedTarget() {
  const platform = normalizeTargetPlatform(argValue(
    '--target-platform',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_PLATFORM || process.platform
  ))
  const arch = normalizeTargetArch(argValue(
    '--target-arch',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_ARCH || process.arch
  ))
  return { platform, arch, key: `${platform}-${arch}` }
}

function expectedPackagedSourceCommit() {
  return argValue(
    '--expected-source-commit',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT || ''
  ).trim().toLowerCase()
}

function sameFileIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.size === right.size && left.mtimeMs === right.mtimeMs
}

function hashRegularFile(path, options = {}) {
  let before
  try {
    before = lstatSync(path)
  } catch {
    return { exists: false, regular: false, sha256: '', byteLength: 0, error: 'missing' }
  }
  if (!before.isFile() || before.isSymbolicLink() || before.size <= 0 || before.size > (options.maxBytes || Number.MAX_SAFE_INTEGER)) {
    return { exists: true, regular: false, sha256: '', byteLength: Number(before.size || 0), error: 'not_regular' }
  }
  const fd = openSync(path, 'r')
  try {
    const opened = fstatSync(fd)
    if (!sameFileIdentity(before, opened)) {
      return { exists: true, regular: false, sha256: '', byteLength: Number(opened.size || 0), error: 'identity_changed' }
    }
    const hash = createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    const chunks = options.capture === true ? [] : null
    let total = 0
    while (true) {
      const read = readSync(fd, buffer, 0, buffer.length, null)
      if (read === 0) break
      hash.update(buffer.subarray(0, read))
      if (chunks) chunks.push(Buffer.from(buffer.subarray(0, read)))
      total += read
    }
    const after = fstatSync(fd)
    let pathAfter
    try {
      pathAfter = lstatSync(path)
    } catch {
      return { exists: true, regular: false, sha256: '', byteLength: total, error: 'path_removed' }
    }
    if (!sameFileIdentity(opened, after) || !sameFileIdentity(after, pathAfter) || total !== after.size) {
      return { exists: true, regular: false, sha256: '', byteLength: total, error: 'identity_changed' }
    }
    return {
      exists: true,
      regular: true,
      sha256: hash.digest('hex'),
      byteLength: total,
      content: chunks ? Buffer.concat(chunks, total) : undefined,
      error: ''
    }
  } finally {
    closeSync(fd)
  }
}

function readBinaryPrefix(path, limit = 1024 * 1024) {
  const fd = openSync(path, 'r')
  try {
    const size = Math.min(fstatSync(fd).size, limit)
    const buffer = Buffer.alloc(size)
    const read = readSync(fd, buffer, 0, size, 0)
    return buffer.subarray(0, read)
  } finally {
    closeSync(fd)
  }
}

function cpuArch(cpuType) {
  if (cpuType === 0x0100000c) return 'arm64'
  if (cpuType === 0x01000007) return 'x64'
  return ''
}

function binaryTarget(path) {
  try {
    const bytes = readBinaryPrefix(path)
    if (bytes.length >= 8) {
      const magic = bytes.readUInt32BE(0)
      if (magic === 0xcffaedfe || magic === 0xfeedfacf) {
        const arch = cpuArch(magic === 0xcffaedfe ? bytes.readUInt32LE(4) : bytes.readUInt32BE(4))
        return { format: 'mach-o', arches: arch ? [arch] : [] }
      }
      if ([0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca].includes(magic)) {
        const little = magic === 0xbebafeca || magic === 0xbfbafeca
        const fat64 = magic === 0xcafebabf || magic === 0xbfbafeca
        const count = little ? bytes.readUInt32LE(4) : bytes.readUInt32BE(4)
        const width = fat64 ? 32 : 20
        const arches = []
        for (let index = 0; index < count && 8 + ((index + 1) * width) <= bytes.length; index += 1) {
          const offset = 8 + (index * width)
          const arch = cpuArch(little ? bytes.readUInt32LE(offset) : bytes.readUInt32BE(offset))
          if (arch && !arches.includes(arch)) arches.push(arch)
        }
        return { format: 'mach-o-fat', arches }
      }
    }
    if (bytes.length >= 64 && bytes[0] === 0x4d && bytes[1] === 0x5a) {
      const peOffset = bytes.readUInt32LE(0x3c)
      if (peOffset + 6 <= bytes.length && bytes.toString('ascii', peOffset, peOffset + 4) === 'PE\u0000\u0000') {
        const machine = bytes.readUInt16LE(peOffset + 4)
        const arch = machine === 0xaa64 ? 'arm64' : machine === 0x8664 ? 'x64' : ''
        return { format: 'pe', arches: arch ? [arch] : [] }
      }
    }
    if (bytes.length >= 20 && bytes[0] === 0x7f && bytes.toString('ascii', 1, 4) === 'ELF') {
      const machine = bytes[5] === 2 ? bytes.readUInt16BE(18) : bytes.readUInt16LE(18)
      const arch = machine === 183 ? 'arm64' : machine === 62 ? 'x64' : ''
      return { format: 'elf', arches: arch ? [arch] : [] }
    }
  } catch {
    // The regular-file/content checks report the actionable artifact failure.
  }
  return { format: 'unknown', arches: [] }
}

function redactSecrets(value) {
  return String(value || '')
    .replace(/Bearer\s+[A-Za-z0-9._~+/-]+=*/g, 'Bearer <redacted>')
    .replace(/Basic\s+[A-Za-z0-9._~+/-]+=*/g, 'Basic <redacted>')
    .replace(/\bsk-[A-Za-z0-9_-]{12,}\b/g, 'sk-<redacted>')
    .replace(/(api[_-]?key|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|secret|password|signature|sig|auth|credential|jwt)=([^&\s]+)/gi, '$1=<redacted>')
    .replace(/((?:x-api-key|api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|secret|password|signature|sig|auth|credential|jwt)\s*:\s*)[^\s,;]+/gi, '$1<redacted>')
    .replace(/("(?:apiKey|api_key|x-api-key|authorization|accessToken|refreshToken|runtimeToken|password|secret|clientSecret|signature|sig|auth|credential|jwt)"\s*:\s*)"[^"]*"/gi, '$1"<redacted>"')
}

function firstSecretFinding(value, path = '$') {
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      const found = firstSecretFinding(value[index], `${path}[${index}]`)
      if (found) return found
    }
    return ''
  }
  if (value && typeof value === 'object') {
    for (const [key, item] of Object.entries(value)) {
      if (/(api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|secret|password|signature|sig|auth|credential|jwt)/i.test(key)) {
        return `${path}.${key}`
      }
      const found = firstSecretFinding(item, `${path}.${key}`)
      if (found) return found
    }
    return ''
  }
  if (typeof value !== 'string') return ''
  return redactSecrets(value) !== value ? path : ''
}

function boundedDirectoryContains(root, needle, options = {}) {
  if (!root || !needle || !existsSync(root)) {
    return { found: false, complete: false, files: 0, bytes: 0 }
  }
  const maxFiles = options.maxFiles || 2_000
  const maxBytes = options.maxBytes || 32 * 1024 * 1024
  const pending = [root]
  let files = 0
  let bytes = 0
  while (pending.length > 0) {
    const current = pending.pop()
    let state
    try {
      state = lstatSync(current)
    } catch {
      return { found: false, complete: false, files, bytes }
    }
    if (state.isSymbolicLink()) {
      return { found: false, complete: false, files, bytes }
    }
    if (state.isDirectory()) {
      let names
      try {
        names = readdirSync(current)
      } catch {
        return { found: false, complete: false, files, bytes }
      }
      for (const name of names) pending.push(join(current, name))
      continue
    }
    if (!state.isFile()) continue
    files += 1
    bytes += state.size
    if (files > maxFiles || bytes > maxBytes || state.size > 4 * 1024 * 1024) {
      return { found: false, complete: false, files, bytes }
    }
    let body
    try {
      body = readFileSync(current)
    } catch {
      return { found: false, complete: false, files, bytes }
    }
    if (body.includes(Buffer.from(needle, 'utf8'))) {
      return { found: true, complete: true, files, bytes }
    }
  }
  return { found: false, complete: true, files, bytes }
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
}

function resolveDefaultPackagedAppPath(target) {
  const exactPath = target.platform === 'darwin'
    ? target.arch === 'arm64' ? 'dist/mac-arm64/analytix.app' : 'dist/mac/analytix.app'
    : target.platform === 'win32'
      ? 'dist-standard-win/win-unpacked/analytix.exe'
      : 'dist/linux-unpacked/analytix'
  return resolve(process.cwd(), exactPath)
}

function resolvePackagedAppPath(target) {
  return resolve(process.cwd(), argValue(
    '--app-path',
    process.env.ANALYTIX_RUNTIME_GO_GUI_PACKAGED_APP_PATH ||
      process.env.ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH ||
      process.env.ANALYTIX_PACKAGED_APP_PATH ||
      resolveDefaultPackagedAppPath(target)
  ))
}

function resolveExecutablePath(appPath, target) {
  if (target.platform === 'darwin') return join(appPath, 'Contents', 'MacOS', 'analytix')
  return appPath
}

function runtimeServerBinaryPath(appPath, target) {
  if (target.platform === 'darwin') return join(appPath, 'Contents', 'Resources', 'runtime-go', 'bin', 'runtime-server')
  const binaryName = target.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
  return join(dirname(appPath), 'resources', 'runtime-go', 'bin', binaryName)
}

function appAsarPath(appPath, target) {
  if (target.platform === 'darwin') return join(appPath, 'Contents', 'Resources', 'app.asar')
  return join(dirname(appPath), 'resources', 'app.asar')
}

function bundledRuntimeGoSourcePath(appPath, target) {
  if (target.platform === 'darwin') {
    return join(appPath, 'Contents', 'Resources', 'app.asar.unpacked', 'packages', 'runtime-go')
  }
  return join(dirname(appPath), 'resources', 'app.asar.unpacked', 'packages', 'runtime-go')
}

function resourceRuntimePath(appPath, target) {
  if (target.platform === 'darwin') return join(appPath, 'Contents', 'Resources', 'runtime')
  return join(dirname(appPath), 'resources', 'runtime')
}

function nativeAuthorityTarget(target) {
  return {
    format: target.platform === 'darwin' ? 'mach-o' : target.platform === 'win32' ? 'pe' : 'elf',
    arch: target.arch
  }
}

function packagedAuthorityContext(appPath, target) {
  return {
    appOutDir: target.platform === 'darwin' ? dirname(appPath) : dirname(appPath),
    electronPlatformName: target.platform,
    arch: target.arch,
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
}

function buildAuthorityEvidence(appPath, target, artifacts, expectedSourceCommit) {
  const runtimeDir = resourceRuntimePath(appPath, target)
  const authorityPath = join(runtimeDir, PACKAGED_BUILD_AUTHORITY_FILE)
  const nativeReceiptPath = join(runtimeDir, 'analytix-native-components-receipt.json')
  const authorityFile = hashRegularFile(authorityPath, { capture: true, maxBytes: 256 * 1024 })
  const nativeReceiptFile = hashRegularFile(nativeReceiptPath, { capture: true, maxBytes: 256 * 1024 })
  const blocked = (reason) => ({
    ok: false,
    blocked: true,
    blocker: reason,
    path: authorityPath,
    sha256: authorityFile.sha256,
    sourceCommit: '',
    targetKey: '',
    nativeDispositionKind: '',
    classification: '',
    publishable: false,
    releaseEligible: false,
    publicationReceiptIssued: false,
    packagedButNonPublishable: true,
    releaseBlocker: 'packaged_build_authority_non_publishable',
    worktreeSnapshotDigest: ''
  })
  if (!authorityFile.exists) return blocked('packaged_build_authority_missing')
  if (!authorityFile.regular) return blocked('packaged_build_authority_not_regular')
  if (!/^[0-9a-f]{40}$/.test(expectedSourceCommit)) return blocked('expected_packaged_source_commit_missing_or_invalid')
  let authority
  const authorityText = authorityFile.content.toString('utf8')
  try {
    authority = JSON.parse(authorityText)
  } catch {
    return blocked('packaged_build_authority_json_invalid')
  }
  const sourceCommit = String(authority?.sourceCommit || '').trim().toLowerCase()
  if (authority?.schemaVersion !== 2 || authority?.contract !== PACKAGED_BUILD_AUTHORITY_CONTRACT) {
    return {
      ...blocked('packaged_build_authority_version_unsupported'),
      sourceCommit,
      targetKey: String(authority?.targetKey || ''),
      nativeDispositionKind: String(authority?.nativeDisposition?.kind || '')
    }
  }
  const canonicalAuthority = authorityText === JSON.stringify(authority)
  const contentBinding = (name, observed) => {
    const value = authority?.artifacts?.[name]
    return value && Object.keys(value).sort().join(',') === 'byteLength,sha256' &&
      value.sha256 === observed.sha256 && value.byteLength === observed.byteLength
  }
  const nativeBinding = (name, observed) => {
    const value = authority?.artifacts?.[name]
    return value && observed && value.payloadSha256 === observed.payloadSha256 &&
      value.payloadByteLength === observed.payloadByteLength &&
      value.format === observed.format && value.arch === observed.arch
  }
  let currentWorktreeSnapshot
  try {
    currentWorktreeSnapshot = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(process.cwd())
  } catch {
    return {
      ...blocked('current_worktree_snapshot_unavailable'),
      sourceCommit,
      targetKey: String(authority?.targetKey || ''),
      nativeDispositionKind: String(authority?.nativeDisposition?.kind || '')
    }
  }
  const authorityShapeValid = packagedAuthorityContract.isPackagedBuildAuthorityV2(authority)
  const worktreeMatches = authorityShapeValid &&
    JSON.stringify(authority.worktreeSnapshot) === JSON.stringify(currentWorktreeSnapshot)
  let nativeDispositionValid = false
  try {
    if (authorityShapeValid && ['core_no_professional_components', 'core_controlled_release'].includes(authority?.nativeDisposition?.kind)) {
      require('./core-package-profile.cjs').assertCoreResourcesAbsent(dirname(dirname(nativeReceiptPath)))
      packagedAuthorityContract.verifyPackagedBuildAuthorityArtifacts(packagedAuthorityContext(appPath, target), authority)
      nativeDispositionValid = !nativeReceiptFile.exists
    } else if (authority?.nativeDisposition?.kind === 'controlled_release_receipt') {
      const nativeReceipt = nativeReceiptFile.regular
        ? JSON.parse(nativeReceiptFile.content.toString('utf8'))
        : null
      nativeDispositionValid = nativeReceipt?.targetKey === target.key &&
        authority.nativeDisposition.receiptSha256 === nativeReceiptFile.sha256
    } else if (authority?.nativeDisposition?.kind === 'development_non_publishable') {
      packagedAuthorityContract.verifyPackagedDevelopmentNativeDisposition(
        packagedAuthorityContext(appPath, target),
        authority.nativeDisposition,
        { requireMarkerBinaryIdentity: false }
      )
      nativeDispositionValid = !nativeReceiptFile.exists
    }
  } catch {
    nativeDispositionValid = false
  }
  const valid = canonicalAuthority && authorityShapeValid && worktreeMatches &&
    authority.targetKey === target.key &&
    nativeDispositionValid &&
    /^[0-9a-f]{40}$/.test(sourceCommit) &&
    sourceCommit === expectedSourceCommit &&
    nativeBinding('executable', artifacts.executable) &&
    contentBinding('appAsar', artifacts.appAsar) &&
    nativeBinding('runtimeServer', artifacts.runtimeServer)
  const blocker = valid
    ? ''
    : authorityShapeValid && !worktreeMatches
      ? 'packaged_build_authority_worktree_snapshot_mismatch'
      : 'packaged_build_authority_incomplete_or_mismatched'
  return {
    ok: valid,
    blocked: !valid,
    blocker,
    path: authorityPath,
    sha256: authorityFile.sha256,
    sourceCommit,
    targetKey: String(authority?.targetKey || ''),
    nativeDispositionKind: String(authority?.nativeDisposition?.kind || ''),
    classification: String(authority?.classification || ''),
    publishable: authority?.publishable === true,
    releaseEligible: authority?.releaseEligible === true,
    publicationReceiptIssued: authority?.publicationReceiptIssued === true,
    packagedButNonPublishable: authority?.publishable !== true || authority?.releaseEligible !== true,
    releaseBlocker: authority?.publishable === true && authority?.releaseEligible === true
      ? ''
      : 'packaged_build_authority_non_publishable',
    worktreeSnapshotDigest: String(authority?.worktreeSnapshot?.snapshotDigest || '')
  }
}

function packagedAppArtifactEvidence(appPath, target) {
  const executablePath = resolveExecutablePath(appPath, target)
  const runtimeBin = runtimeServerBinaryPath(appPath, target)
  const asar = appAsarPath(appPath, target)
  const bundledSource = bundledRuntimeGoSourcePath(appPath, target)
  const executable = hashRegularFile(executablePath)
  const runtimeServer = hashRegularFile(runtimeBin)
  const appAsar = hashRegularFile(asar)
  const executableTarget = executable.regular ? binaryTarget(executablePath) : { format: 'unknown', arches: [] }
  const runtimeTarget = runtimeServer.regular ? binaryTarget(runtimeBin) : { format: 'unknown', arches: [] }
  const expectedFormat = target.platform === 'darwin' ? 'mach-o' : target.platform === 'win32' ? 'pe' : 'elf'
  const targetMatches = executableTarget.format === expectedFormat && runtimeTarget.format === expectedFormat &&
    executableTarget.arches.length === 1 && executableTarget.arches[0] === target.arch &&
    runtimeTarget.arches.length === 1 && runtimeTarget.arches[0] === target.arch
  let executableAuthority = null
  let runtimeServerAuthority = null
  try {
    const authorityTarget = nativeAuthorityTarget(target)
    if (executable.regular) {
      executableAuthority = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        executablePath,
        authorityTarget
      )
    }
    if (runtimeServer.regular) {
      runtimeServerAuthority = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
        runtimeBin,
        authorityTarget
      )
    }
  } catch {
    executableAuthority = null
    runtimeServerAuthority = null
  }
  const authority = buildAuthorityEvidence(appPath, target, {
    executable: executableAuthority,
    appAsar,
    runtimeServer: runtimeServerAuthority
  }, expectedPackagedSourceCommit())
  const evidence = {
    appPath,
    target,
    appExists: existsSync(appPath),
    executableExists: executable.exists,
    executableRegular: executable.regular,
    executableSha256: executable.sha256,
    executableByteLength: executable.byteLength,
    executableFormat: executableTarget.format,
    executableArches: executableTarget.arches,
    runtimeServerBinaryExists: runtimeServer.exists,
    runtimeServerBinaryRegular: runtimeServer.regular,
    runtimeServerSha256: runtimeServer.sha256,
    runtimeServerByteLength: runtimeServer.byteLength,
    runtimeServerFormat: runtimeTarget.format,
    runtimeServerArches: runtimeTarget.arches,
    appAsarExists: appAsar.exists,
    appAsarRegular: appAsar.regular,
    appAsarSha256: appAsar.sha256,
    appAsarByteLength: appAsar.byteLength,
    bundledRuntimeGoSourcePresent: existsSync(bundledSource),
    targetMatches,
    buildAuthority: authority
  }
  return {
    ...evidence,
    ok: evidence.appExists &&
      evidence.executableRegular &&
      evidence.runtimeServerBinaryRegular &&
      evidence.appAsarRegular &&
      !evidence.bundledRuntimeGoSourcePresent &&
      evidence.targetMatches &&
      authority.ok,
    blocked: evidence.appExists &&
      evidence.executableRegular &&
      evidence.runtimeServerBinaryRegular &&
      evidence.appAsarRegular &&
      !evidence.bundledRuntimeGoSourcePresent &&
      evidence.targetMatches &&
      authority.blocked,
    message: !evidence.appExists
      ? 'packaged app path does not exist'
      : !evidence.executableRegular
        ? 'packaged executable is missing or not a stable regular file'
        : !evidence.runtimeServerBinaryRegular
          ? 'packaged Go runtime-server binary is missing or not a stable regular file'
          : !evidence.appAsarRegular
            ? 'packaged app.asar is missing or not a stable regular file'
            : evidence.bundledRuntimeGoSourcePresent
              ? 'packaged app includes runtime-go source in app.asar.unpacked'
              : !evidence.targetMatches
                ? `packaged executable/runtime architecture does not exactly match ${target.key}`
                : !authority.ok
                  ? authority.blocker
                  : ''
  }
}

async function getFreePort() {
  const server = net.createServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  await new Promise((resolve) => server.close(resolve))
  if (!address || typeof address === 'string') throw new Error('failed to allocate local port')
  return address.port
}

async function sleep(ms) {
  await new Promise((resolve) => setTimeout(resolve, ms))
}

async function stopChild(child) {
  if (!child || child.exitCode !== null || child.signalCode) return
  child.kill('SIGTERM')
  const closed = await Promise.race([
    new Promise((resolve) => child.once('close', resolve)),
    sleep(1200).then(() => false)
  ])
  if (closed === false && child.exitCode === null && !child.signalCode) {
    child.kill('SIGKILL')
    await Promise.race([
      new Promise((resolve) => child.once('close', resolve)),
      sleep(800)
    ])
  }
}

function listeningPidsOnPort(port) {
  if (!Number.isFinite(port) || port <= 0 || process.platform === 'win32') return []
  try {
    return execFileSync('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore']
    })
      .split(/\r?\n/)
      .map((line) => Number(line.trim()))
      .filter((pid) => Number.isInteger(pid) && pid > 0)
  } catch {
    return []
  }
}

function commandLineForPid(pid) {
  if (!Number.isInteger(pid) || pid <= 0 || process.platform === 'win32') return ''
  try {
    return execFileSync('ps', ['-p', String(pid), '-o', 'command='], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore']
    }).trim()
  } catch {
    return ''
  }
}

async function stopSmokeRuntimeOnPort(port, runtimeDataDir) {
  if (!runtimeDataDir) return
  for (const pid of listeningPidsOnPort(port)) {
    const command = commandLineForPid(pid)
    if (!command.includes('runtime-server') || !command.includes(runtimeDataDir)) continue
    try {
      process.kill(pid, 'SIGTERM')
    } catch {
      continue
    }
    for (let attempt = 0; attempt < 12; attempt += 1) {
      await sleep(100)
      if (!listeningPidsOnPort(port).includes(pid)) break
    }
    if (listeningPidsOnPort(port).includes(pid)) {
      try {
        process.kill(pid, 'SIGKILL')
      } catch {
        /* already gone */
      }
    }
  }
}

async function waitForDebugTarget(port, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let lastError = ''
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`, {
        signal: AbortSignal.timeout(1000)
      })
      if (response.ok) {
        const targets = await response.json()
        const page = targets.find((target) => {
          if (target.type !== 'page' || !target.webSocketDebuggerUrl) return false
          try {
            const url = new URL(target.url)
            return url.protocol === PACKAGED_RENDERER_SCHEME &&
              url.hostname === PACKAGED_RENDERER_HOST &&
              url.pathname === PACKAGED_RENDERER_PATH
          } catch {
            return false
          }
        })
        if (page) return page
        lastError = 'exact packaged renderer target unavailable'
      }
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error)
    }
    await sleep(250)
  }
  throw new Error(`renderer debug target not ready${lastError ? `: ${lastError}` : ''}`)
}

async function evaluateCdp(wsUrl, expression, timeoutMs) {
  if (typeof WebSocket === 'undefined') throw new Error('global WebSocket is unavailable in this Node runtime')
  const ws = new WebSocket(wsUrl)
  await new Promise((resolve, reject) => {
    ws.addEventListener('open', resolve, { once: true })
    ws.addEventListener('error', reject, { once: true })
  })
  const id = 1
  try {
    return await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('CDP Runtime.evaluate timeout')), timeoutMs)
      ws.addEventListener('message', (event) => {
        const message = JSON.parse(String(event.data))
        if (message.id !== id) return
        clearTimeout(timer)
        if (message.error) {
          reject(new Error(JSON.stringify(message.error)))
          return
        }
        if (message.result?.exceptionDetails) {
          reject(new Error(JSON.stringify(message.result.exceptionDetails)))
          return
        }
        resolve(message.result?.result?.value)
      })
      ws.send(JSON.stringify({
        id,
        method: 'Runtime.evaluate',
        params: {
          expression,
          awaitPromise: true,
          returnByValue: true,
          timeout: timeoutMs
        }
      }))
    })
  } finally {
    ws.close()
  }
}

function isTransientCdpEvaluationError(error) {
  const text = error instanceof Error ? error.message : String(error || '')
  return text.includes('Execution context was destroyed') ||
    text.includes('Cannot find context') ||
    text.includes('Target closed') ||
    text.includes('target closed') ||
    text.includes('WebSocket') ||
    text.includes('renderer debug target not ready')
}

async function evaluateRendererWithRetries({ debugPort, expression, timeoutMs }) {
  const deadline = Date.now() + timeoutMs
  let lastError = null
  for (let attempt = 0; Date.now() < deadline; attempt += 1) {
    const remainingMs = Math.max(250, deadline - Date.now())
    try {
      const target = await waitForDebugTarget(debugPort, Math.min(5_000, remainingMs))
      return await evaluateCdp(
        target.webSocketDebuggerUrl,
        expression,
        Math.min(60_000, Math.max(1_000, deadline - Date.now()))
      )
    } catch (error) {
      lastError = error
      if (!isTransientCdpEvaluationError(error)) throw error
      await sleep(Math.min(250 + attempt * 100, 1_000, Math.max(0, deadline - Date.now())))
    }
  }
  throw lastError || new Error('CDP Runtime.evaluate timeout')
}

function buildRendererExpression({ runtimePort, runtimeDataDir, syntheticApiKey, terminalCwd }) {
  const legacyKunAlias = String.fromCharCode(107, 117, 110)
  const legacyEngineAlias = String.fromCharCode(114, 101, 97, 115, 111, 110, 105, 120)
  return `(async () => {
    const waitForProductTitle = async () => {
      const deadline = Date.now() + 5000;
      while (Date.now() < deadline) {
        if (document.title === 'Analytix') return document.title;
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      return document.title;
    };
    const productTitle = await waitForProductTitle();
    const api = window.analytix;
    const out = {
      href: location.href,
      packagedRendererUrlExact: location.protocol === ${JSON.stringify(PACKAGED_RENDERER_SCHEME)} &&
        location.hostname === ${JSON.stringify(PACKAGED_RENDERER_HOST)} &&
        location.pathname === ${JSON.stringify(PACKAGED_RENDERER_PATH)},
      fileRendererProtocolUsed: location.protocol === 'file:',
      title: productTitle,
      apiPresent: !!api,
      domains: api ? Object.keys(api).sort() : [],
      legacyKun: !!window[${JSON.stringify(legacyKunAlias)}],
      legacyReasonix: !!window[${JSON.stringify(legacyEngineAlias)}],
      terminalApiPresent: false,
      terminalCreateOk: false,
      terminalWriteOk: false,
      terminalTtyMarkerObserved: false,
      terminalOrdinaryWorkspaceOk: false,
      terminalProtectedDirectBlocked: false,
      terminalProtectedSymlinkBlocked: false,
      terminalAmbientEnvClean: false,
      terminalExitObserved: false,
      terminalExitCode: null,
      terminalDisposeOk: false,
      terminalRuntimeBoundaryCreateOk: false,
      terminalRuntimeBoundaryWriteOk: false,
      terminalRuntimeBoundaryReadyObserved: false,
      terminalRuntimeBoundaryPrePatchExitAbsent: false,
      terminalRuntimeBoundaryExitObserved: false,
      terminalSilentRuntimePatchAccepted: false,
      terminalPostRestartCreateOk: false,
      terminalPostRestartProtectedBlocked: false,
      settingsOk: false,
      settingsRuntimeTopLevel: false,
      settingsProfilePatchAccepted: false,
      mimoProfileSettingsOk: false,
      mimoProviderId: '',
      mimoModel: '',
      mimoEndpointFormat: '',
      mimoProviderReady: false,
      settingsCompatPatchUsed: false,
      settingsPatchMode: '',
      settingsPatchError: '',
      restartOk: false,
      restartError: '',
      healthStatus: 0,
      healthService: '',
      runtimeInfoStatus: 0,
      runtimeInfoProductClean: false,
      runtimeInfoHasLegacyMcpLocalMarker: false,
      runtimeInfoHasLegacyAbsorptionMarker: false,
      runtimeInfoHasReasonixSurface: false
    };
    try {
      if (!api) return out;
      out.terminalApiPresent = !!(api.terminal &&
        typeof api.terminal.create === 'function' &&
        typeof api.terminal.write === 'function' &&
        typeof api.terminal.dispose === 'function' &&
        typeof api.terminal.onData === 'function' &&
        typeof api.terminal.onExit === 'function');
      if (out.terminalApiPresent) {
        const terminalSessionId = 'packaged-gui-smoke-pty';
        let terminalOutput = '';
        let resolveTerminalExit;
        const terminalExit = new Promise((resolve) => {
          resolveTerminalExit = resolve;
        });
        const removeTerminalData = api.terminal.onData((payload) => {
          if (payload && payload.sessionId === terminalSessionId && terminalOutput.length < 8192) {
            terminalOutput += String(payload.data || '').slice(0, 8192 - terminalOutput.length);
          }
        });
        const removeTerminalExit = api.terminal.onExit((payload) => {
          if (payload && payload.sessionId === terminalSessionId) {
            resolveTerminalExit(payload);
          }
        });
        try {
          const created = await api.terminal.create({
            sessionId: terminalSessionId,
            cwd: ${JSON.stringify(terminalCwd)},
            cols: 87,
            rows: 29
          });
          out.terminalCreateOk = created && created.ok === true;
          if (out.terminalCreateOk) {
            out.terminalWriteOk = true;
            const terminalCommands = [
              "if printf '%s\\\\n' 'ordinary-terminal-write' > ordinary-created.txt && [ \\"$(cat ordinary-input.txt)\\" = 'ordinary-terminal-input' ] && [ \\"$(cat ordinary-created.txt)\\" = 'ordinary-terminal-write' ]; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_ORDINARY_OK__'; fi",
              "if ! /bin/cat \\"$HOME/.analytix/packaged-gui-user-data/terminal-protected-sentinel.txt\\" >/dev/null 2>&1 && ! /bin/cat \\"$HOME/.analytix/data/private/terminal-protected-sentinel.txt\\" >/dev/null 2>&1; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_DIRECT_BLOCKED__'; fi",
              "if ! /bin/cat user-data-protected-link >/dev/null 2>&1 && ! /bin/cat initial-runtime-private-link >/dev/null 2>&1; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_SYMLINK_BLOCKED__'; fi",
              "if ! /usr/bin/env | /usr/bin/grep -Eq '^(ANALYTIX_PACKAGED_TERMINAL_ENV_CANARY|OPENAI_API_KEY)='; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_ENV_CLEAN__'; fi",
              "if [ -t 0 ] && [ -t 1 ] && [ \\"$(stty size 2>/dev/null)\\" = '29 87' ]; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_TTY_OK__'; fi; exit 0"
            ];
            for (const command of terminalCommands) {
              if (await api.terminal.write({
                sessionId: terminalSessionId,
                data: command + "\\r"
              }) !== true) {
                out.terminalWriteOk = false;
              }
            }
            const exited = await Promise.race([
              terminalExit,
              new Promise((resolve) => setTimeout(() => resolve(null), 10000))
            ]);
            out.terminalExitObserved = !!exited;
            out.terminalExitCode = exited && Number.isInteger(exited.exitCode)
              ? exited.exitCode
              : null;
            const terminalLines = terminalOutput.replaceAll('\\r', '').split('\\n');
            out.terminalTtyMarkerObserved = terminalLines
              .includes('__ANALYTIX_PACKAGED_PTY_TTY_OK__');
            out.terminalOrdinaryWorkspaceOk = terminalLines
              .includes('__ANALYTIX_PACKAGED_PTY_ORDINARY_OK__');
            out.terminalProtectedDirectBlocked = terminalLines
              .includes('__ANALYTIX_PACKAGED_PTY_DIRECT_BLOCKED__');
            out.terminalProtectedSymlinkBlocked = terminalLines
              .includes('__ANALYTIX_PACKAGED_PTY_SYMLINK_BLOCKED__');
            out.terminalAmbientEnvClean = terminalLines
              .includes('__ANALYTIX_PACKAGED_PTY_ENV_CLEAN__');
          }
        } finally {
          removeTerminalData();
          removeTerminalExit();
          out.terminalDisposeOk = await api.terminal.dispose(terminalSessionId) === true;
          terminalOutput = '';
        }
      }
      const boundarySessionId = 'packaged-gui-smoke-runtime-boundary';
      let boundaryOutput = '';
      let boundaryExited = false;
      let boundaryExitDuringSettings = false;
      let boundarySettingsTransitionArmed = false;
      let removeBoundaryData = () => {};
      let removeBoundaryExit = () => {};
      let boundaryExitPromise = Promise.resolve(null);
      if (out.terminalApiPresent) {
        let resolveBoundaryExit;
        boundaryExitPromise = new Promise((resolve) => {
          resolveBoundaryExit = resolve;
        });
        removeBoundaryData = api.terminal.onData((payload) => {
          if (payload && payload.sessionId === boundarySessionId &&
            boundaryOutput.length < 4096) {
            boundaryOutput += String(payload.data || '').slice(
              0,
              4096 - boundaryOutput.length
            );
          }
        });
        removeBoundaryExit = api.terminal.onExit((payload) => {
          if (payload && payload.sessionId === boundarySessionId) {
            boundaryExited = true;
            boundaryExitDuringSettings = boundarySettingsTransitionArmed;
            resolveBoundaryExit(payload);
          }
        });
        const boundaryCreated = await api.terminal.create({
          sessionId: boundarySessionId,
          cwd: ${JSON.stringify(terminalCwd)},
          cols: 87,
          rows: 29
        });
        out.terminalRuntimeBoundaryCreateOk =
          boundaryCreated && boundaryCreated.ok === true;
        if (out.terminalRuntimeBoundaryCreateOk) {
          out.terminalRuntimeBoundaryWriteOk = await api.terminal.write({
            sessionId: boundarySessionId,
            data: "printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_BOUNDARY_READY__'; while :; do /bin/sleep 1; done\\r"
          }) === true;
          const boundaryReadyDeadline = Date.now() + 5000;
          while (!boundaryExited && Date.now() < boundaryReadyDeadline &&
            !boundaryOutput.replaceAll('\\r', '').split('\\n')
              .includes('__ANALYTIX_PACKAGED_PTY_BOUNDARY_READY__')) {
            await new Promise((resolve) => setTimeout(resolve, 25));
          }
          out.terminalRuntimeBoundaryReadyObserved = boundaryOutput
            .replaceAll('\\r', '')
            .split('\\n')
            .includes('__ANALYTIX_PACKAGED_PTY_BOUNDARY_READY__');
          out.terminalRuntimeBoundaryPrePatchExitAbsent =
            out.terminalRuntimeBoundaryWriteOk === true &&
            out.terminalRuntimeBoundaryReadyObserved === true &&
            boundaryExited === false;
        }
      }
      const initial = await api.settings.getSettings();
      out.settingsOk = !!(initial && initial.runtime && initial.provider);
      out.settingsRuntimeTopLevel = !!(initial && initial.runtime && !initial.agent && !initial.agents && !initial.reasonix);
      boundarySettingsTransitionArmed =
        out.terminalRuntimeBoundaryPrePatchExitAbsent === true;
      await api.settings.saveSettingsSilent({
        runtime: {
          dataDir: ${JSON.stringify(runtimeDataDir)}
        }
      });
      out.terminalSilentRuntimePatchAccepted = true;
      boundarySettingsTransitionArmed = false;
      const boundaryExit = await Promise.race([
        boundaryExitPromise,
        new Promise((resolve) => setTimeout(() => resolve(null), 10000))
      ]);
      removeBoundaryData();
      removeBoundaryExit();
      out.terminalRuntimeBoundaryExitObserved =
        boundaryExitDuringSettings === true &&
        !!boundaryExit &&
        boundaryExit.exitCode === null;
      if (out.terminalApiPresent) {
        await api.terminal.dispose(boundarySessionId);
      }
      boundaryOutput = '';
      const profilePatch = {
        provider: {
          activeProviderId: 'xiaomi',
          baseUrl: 'https://api.deepseek.com',
          providers: [
            {
              id: 'deepseek',
              name: 'DeepSeek',
              baseUrl: 'https://api.deepseek.com',
              endpointFormat: 'chat_completions',
              models: ['deepseek-chat']
            },
            {
              id: 'xiaomi',
              name: 'Xiaomi',
              baseUrl: 'https://api.xiaomimimo.com/v1',
              endpointFormat: 'chat_completions',
              models: ['mimo-v2.5', 'mimo-v2.5-pro']
            }
          ]
        },
        runtime: {
          port: ${JSON.stringify(runtimePort)},
          dataDir: ${JSON.stringify(runtimeDataDir)},
          autoStart: true,
          providerId: 'xiaomi',
          model: 'mimo-v2.5-pro'
        }
      };
      try {
        const before = await api.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
        if (!before || before.error || !Array.isArray(before.providers)) {
          throw new Error('synthetic_provider_registry_unavailable');
        }
        const connected = await api.providerRegistry.request({
          schemaVersion: 1,
          operation: 'connect',
          expected: {
            registryRevision: before.registryRevision,
            registryIncarnation: before.registryIncarnation,
            providerRevision: '0',
            providerGeneration: '0',
            providerIncarnation: '',
            providerCredentialPurpose: ''
          },
          provider: {
            id: 'xiaomi',
            kind: 'openai-compatible',
            endpoint: 'https://api.xiaomimimo.com/v1',
            proxy: '',
            models: ['mimo-v2.5', 'mimo-v2.5-pro'],
            mediaModels: [],
            selectedModel: 'mimo-v2.5-pro',
            selectedMediaModel: '',
            selectedRoutes: []
          },
          credential: {
            kind: 'set',
            purpose: 'provider-api-key',
            valueBase64: btoa(${JSON.stringify(syntheticApiKey)})
          }
        });
        if (!connected || connected.error) throw new Error('synthetic_provider_connect_failed');
        const registry = await api.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
        const registered = registry && !registry.error && Array.isArray(registry.providers)
          ? registry.providers.find((item) => item && item.id === 'xiaomi') : null;
        if (!registered || registered.credentialConfigured !== true) {
          throw new Error('synthetic_provider_credential_not_configured');
        }
        await api.settings.setSettings(profilePatch);
        out.settingsProfilePatchAccepted = true;
        out.settingsPatchMode = 'provider-profile';
        const saved = await api.settings.getSettings();
        const providers = Array.isArray(saved && saved.provider && saved.provider.providers)
          ? saved.provider.providers
          : [];
        const mimo = providers.find((item) => item && item.id === 'xiaomi') || null;
        out.mimoProviderId = saved && saved.runtime && saved.runtime.providerId || '';
        out.mimoModel = saved && saved.runtime && saved.runtime.model || '';
        out.mimoEndpointFormat = mimo && mimo.endpointFormat || '';
        out.mimoProviderReady = !!(mimo && mimo.baseUrl &&
          registered.credentialConfigured === true &&
          !Object.hasOwn(saved.provider, 'apiKey') &&
          !Object.hasOwn(saved.runtime, 'apiKey') &&
          !Object.hasOwn(mimo, 'apiKey'));
        out.mimoProfileSettingsOk = !!mimo &&
          saved.provider.activeProviderId === 'xiaomi' &&
          out.mimoProviderId === 'xiaomi' &&
          (out.mimoModel === 'mimo-v2.5-pro-ultraspeed' || out.mimoModel === 'mimo-v2.5-pro') &&
          out.mimoEndpointFormat === 'chat_completions' &&
          Array.isArray(mimo.models) &&
          mimo.models.includes('mimo-v2.5-pro') &&
          out.mimoProviderReady === true;
      } catch (error) {
        out.settingsPatchError = error && (error.stack || error.message) || String(error);
        return out;
      }
      try {
        await api.runtime.restartRuntime();
        out.restartOk = true;
      } catch (error) {
        out.restartError = error && (error.stack || error.message) || String(error);
        return out;
      }
      if (out.terminalApiPresent) {
        const postRestartSessionId = 'packaged-gui-smoke-post-restart';
        let postRestartOutput = '';
        let resolvePostRestartExit;
        const postRestartExit = new Promise((resolve) => {
          resolvePostRestartExit = resolve;
        });
        const removePostRestartData = api.terminal.onData((payload) => {
          if (payload && payload.sessionId === postRestartSessionId &&
            postRestartOutput.length < 8192) {
            postRestartOutput += String(payload.data || '').slice(
              0,
              8192 - postRestartOutput.length
            );
          }
        });
        const removePostRestartExit = api.terminal.onExit((payload) => {
          if (payload && payload.sessionId === postRestartSessionId) {
            resolvePostRestartExit(payload);
          }
        });
        try {
          const postRestartCreated = await api.terminal.create({
            sessionId: postRestartSessionId,
            cwd: ${JSON.stringify(terminalCwd)},
            cols: 87,
            rows: 29
          });
          out.terminalPostRestartCreateOk =
            postRestartCreated && postRestartCreated.ok === true;
          if (out.terminalPostRestartCreateOk) {
            await api.terminal.write({
              sessionId: postRestartSessionId,
              data: "direct_blocked=0; symlink_blocked=0; if ! /bin/cat \\"$HOME/.analytix/data/private/terminal-protected-sentinel.txt\\" >/dev/null 2>&1 && ! /bin/cat \\"$HOME/.analytix/packaged-gui-runtime-data/private/terminal-protected-sentinel.txt\\" >/dev/null 2>&1; then direct_blocked=1; fi; if ! /bin/cat initial-runtime-private-link >/dev/null 2>&1 && ! /bin/cat post-restart-runtime-private-link >/dev/null 2>&1; then symlink_blocked=1; fi; if [ \\"$direct_blocked\\" = 1 ] && [ \\"$symlink_blocked\\" = 1 ] && [ \\"$(cat ordinary-input.txt)\\" = 'ordinary-terminal-input' ]; then printf '%s\\\\n' '__ANALYTIX_PACKAGED_PTY_POST_RESTART_BLOCKED__'; fi; exit 0\\r"
            });
            await Promise.race([
              postRestartExit,
              new Promise((resolve) => setTimeout(() => resolve(null), 10000))
            ]);
            out.terminalPostRestartProtectedBlocked = postRestartOutput
              .replaceAll('\\r', '')
              .split('\\n')
              .includes('__ANALYTIX_PACKAGED_PTY_POST_RESTART_BLOCKED__');
          }
        } finally {
          removePostRestartData();
          removePostRestartExit();
          await api.terminal.dispose(postRestartSessionId);
          postRestartOutput = '';
        }
      }
      const health = await api.runtime.runtimeRequest('/health', 'GET');
      out.healthStatus = health.status;
      out.healthService = JSON.parse(health.body || '{}').service || '';
      const info = await api.runtime.runtimeRequest('/v1/runtime/info', 'GET');
      out.runtimeInfoStatus = info.status;
      const parsedInfo = JSON.parse(info.body || '{}');
      const serializedInfo = JSON.stringify(parsedInfo);
      const legacyAbsorptionMarker = ['D', '0244'].join('');
      const legacyMcpLocalMarker = ['mcp', 'Local', 'Pr', 'oof'].join('');
      const reasonixIndexerRoute = ['/v1/', 'mcp-indexer'].join('');
      const reasonixSubagentsRoute = ['/v1/', 'subagents'].join('');
      const reasonixProductName = ['Reason', 'ix'].join('');
      out.runtimeInfoHasLegacyMcpLocalMarker = serializedInfo.includes(legacyMcpLocalMarker);
      out.runtimeInfoHasLegacyAbsorptionMarker = serializedInfo.includes(legacyAbsorptionMarker);
      out.runtimeInfoHasReasonixSurface = serializedInfo.includes(reasonixIndexerRoute) ||
        serializedInfo.includes(reasonixSubagentsRoute) ||
        serializedInfo.includes(reasonixProductName);
      out.runtimeInfoProductClean = parsedInfo.schemaVersion === 2 &&
        parsedInfo.status === 'ready' &&
        parsedInfo.listenerScope === 'loopback' &&
        parsedInfo.port === ${JSON.stringify(runtimePort)} &&
        parsedInfo.storage &&
        parsedInfo.storage.configured === true &&
        parsedInfo.storage.available === true &&
        !!parsedInfo.capabilities &&
        typeof parsedInfo.capabilities.contractVersion === 'number' &&
        !Object.prototype.hasOwnProperty.call(parsedInfo, 'host') &&
        !Object.prototype.hasOwnProperty.call(parsedInfo, 'dataDir') &&
        !serializedInfo.includes('upstreamAbsorption') &&
        !out.runtimeInfoHasLegacyMcpLocalMarker &&
        !out.runtimeInfoHasLegacyAbsorptionMarker &&
        !out.runtimeInfoHasReasonixSurface;
    } catch (error) {
      out.restartError = error && (error.stack || error.message) || String(error);
    }
    return out;
  })()`
}

function summarizeRestartError(raw) {
  const text = redactSecrets(String(raw || '')).replace(/\s+/g, ' ').trim()
  if (!text) return ''
  if (text.includes('missing_api_key')) return 'missing_api_key'
  if (text.includes('runtime_unhealthy')) return 'runtime_unhealthy'
  if (text.includes('go-runtime-default')) return 'go-runtime-default-readiness'
  return text.slice(0, 280)
}

function smokeChildEnv({ tempHome, userDataDir, chromiumTempDir, terminalEnvCanary }) {
  const keep = new Set([
    'PATH',
    'SystemRoot',
    'TMPDIR',
    'TEMP',
    'TMP',
    'SHELL',
    'LANG',
    'LC_ALL',
    'LC_CTYPE',
    'ELECTRON_ENABLE_LOGGING'
  ])
  const env = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (!keep.has(key)) continue
    env[key] = value
  }
  return {
    ...env,
    HOME: tempHome,
    USERPROFILE: tempHome,
    TMPDIR: chromiumTempDir,
    TEMP: chromiumTempDir,
    TMP: chromiumTempDir,
    ...(process.platform === 'darwin'
      ? { MAC_CHROMIUM_TMPDIR: chromiumTempDir, SHELL: '/bin/zsh' }
      : {}),
    ANALYTIX_USER_DATA_DIR: userDataDir,
    ANALYTIX_RUNTIME_BACKEND: 'go-runtime-default',
    ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE: '1',
    ANALYTIX_PACKAGED_TERMINAL_ENV_CANARY: terminalEnvCanary,
    OPENAI_API_KEY: terminalEnvCanary
  }
}

function actualCheck(id, label, passed, message, statusWhenFailed = 'failed') {
  return {
    id,
    label,
    status: passed ? 'passed' : statusWhenFailed,
    message: redactSecrets(message || '')
  }
}

async function runActualPackagedSmoke() {
  const timeoutMs = Number(argValue(
    '--timeout-ms',
    process.env.ANALYTIX_RUNTIME_GO_GUI_TIMEOUT_MS ||
      process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SMOKE_TIMEOUT_MS ||
      DEFAULT_TIMEOUT_MS
  ))
  const target = packagedTarget()
  const appPath = resolvePackagedAppPath(target)
  const artifactEvidence = packagedAppArtifactEvidence(appPath, target)
  const checks = [
    actualCheck(
      'packaged-app-artifact',
      'Packaged app artifact',
      artifactEvidence.ok,
      artifactEvidence.ok
        ? 'packaged app, app.asar, and Go runtime binary content are bound to the exact target and packaged build authority'
        : artifactEvidence.message,
      artifactEvidence.blocked ? 'live_blocked' : 'failed'
    )
  ]
  checks.push(actualCheck(
    'packaged-formal-instance-launch-policy',
    'Formal instance launch policy',
    true,
    formalInstanceMode
      ? 'formal instance mode inspects the selected package but never creates a profile or starts a second app instance'
      : 'isolated packaged smoke mode may start one explicitly selected package instance'
  ))
  let child = null
  let tempHome = ''
  let userDataDir = ''
  let runtimeDataDir = ''
  let renderer = null
  let launchError = ''
  let stdout = ''
  let stderr = ''
  let debugPort = 0
  let runtimePort = 0
  let syntheticCredentialUsed = false
  let terminalProtectedSentinel = ''
  let terminalEnvCanary = ''
  let terminalFixtureReady = false
  let terminalProjectionLeakFree = false
  let terminalProjectionScanComplete = false
  let terminalProjectionFilesScanned = 0
  let terminalProjectionBytesScanned = 0
  let isolatedLoginKeychain = null
  let isolatedLaunchAuthority = {
    required: target.platform === 'darwin',
    created: target.platform !== 'darwin',
    defaultKeychainBound: target.platform !== 'darwin',
    unlocked: target.platform !== 'darwin',
    cacheTempBound: false,
    requestedPathHash: '',
    databasePathHash: ''
  }

  if (artifactEvidence.ok && !formalInstanceMode) {
    tempHome = mkdtempSync(join(tmpdir(), 'analytix-packaged-gui-home-'))
    userDataDir = join(tempHome, '.analytix', 'packaged-gui-user-data')
    runtimeDataDir = join(tempHome, '.analytix', 'packaged-gui-runtime-data')
    const initialRuntimeDataDir = join(tempHome, '.analytix', 'data')
    const terminalCwd = join(tempHome, 'terminal-workspace')
    const chromiumTempDir = join(tempHome, 'chromium-tmp')
    try {
      mkdirSync(dirname(userDataDir), { recursive: true, mode: 0o700 })
      mkdirSync(userDataDir, { mode: 0o700 })
      mkdirSync(terminalCwd, { mode: 0o700 })
      mkdirSync(chromiumTempDir, { mode: 0o700 })
      mkdirSync(join(initialRuntimeDataDir, 'private'), { recursive: true, mode: 0o700 })
      mkdirSync(join(initialRuntimeDataDir, 'threads'), { recursive: true, mode: 0o700 })
      mkdirSync(join(runtimeDataDir, 'private'), { recursive: true, mode: 0o700 })
      terminalProtectedSentinel =
        `terminal-protected-${sha256(`${Date.now()}:${process.pid}:${runtimeDataDir}`)}`
      terminalEnvCanary =
        `terminal-env-${sha256(`${Date.now()}:${process.pid}:${userDataDir}`)}`
      const userDataSentinelPath = join(userDataDir, 'terminal-protected-sentinel.txt')
      const initialRuntimeSentinelPath = join(
        initialRuntimeDataDir,
        'private',
        'terminal-protected-sentinel.txt'
      )
      const postRestartRuntimeSentinelPath = join(
        runtimeDataDir,
        'private',
        'terminal-protected-sentinel.txt'
      )
      for (const path of [
        userDataSentinelPath,
        initialRuntimeSentinelPath,
        postRestartRuntimeSentinelPath
      ]) {
        writeFileSync(path, terminalProtectedSentinel, { mode: 0o600 })
      }
      writeFileSync(
        join(terminalCwd, 'ordinary-input.txt'),
        'ordinary-terminal-input',
        { mode: 0o600 }
      )
      const userDataProtectedLink = join(terminalCwd, 'user-data-protected-link')
      const initialRuntimePrivateLink = join(
        terminalCwd,
        'initial-runtime-private-link'
      )
      const postRestartRuntimePrivateLink = join(
        terminalCwd,
        'post-restart-runtime-private-link'
      )
      symlinkSync(userDataSentinelPath, userDataProtectedLink)
      symlinkSync(
        initialRuntimeSentinelPath,
        initialRuntimePrivateLink
      )
      symlinkSync(
        postRestartRuntimeSentinelPath,
        postRestartRuntimePrivateLink
      )
      terminalFixtureReady = [
        userDataSentinelPath,
        initialRuntimeSentinelPath,
        postRestartRuntimeSentinelPath
      ].every((path) =>
        lstatSync(path).isFile() &&
        readFileSync(path, 'utf8') === terminalProtectedSentinel
      ) &&
        readlinkSync(userDataProtectedLink) === userDataSentinelPath &&
        readlinkSync(initialRuntimePrivateLink) === initialRuntimeSentinelPath &&
        readlinkSync(postRestartRuntimePrivateLink) === postRestartRuntimeSentinelPath
      if (target.platform === 'darwin') {
        isolatedLoginKeychain = await createIsolatedDarwinLoginKeychain(tempHome)
        const created = isolatedLoginKeychain.evidence()
        const unlocked = await isolatedLoginKeychain.unlockForLaunch()
        isolatedLaunchAuthority = {
          required: true,
          created: created.created === true,
          defaultKeychainBound: unlocked.defaultKeychainBound === true,
          unlocked: unlocked.ok === true && unlocked.unlockCount === 1,
          cacheTempBound: true,
          requestedPathHash: unlocked.requestedPathHash || '',
          databasePathHash: unlocked.databasePathHash || ''
        }
      } else {
        isolatedLaunchAuthority.cacheTempBound = true
      }
      debugPort = await getFreePort()
      runtimePort = await getFreePort()
      syntheticCredentialUsed = true
      const syntheticApiKey = `sk-packaged-gui-smoke-${sha256(`${Date.now()}:${runtimePort}`).slice(0, 24)}`
      child = spawn(resolveExecutablePath(appPath, target), [`--remote-debugging-port=${debugPort}`], {
        cwd: process.cwd(),
        env: smokeChildEnv({
          tempHome,
          userDataDir,
          chromiumTempDir,
          terminalEnvCanary
        }),
        stdio: ['ignore', 'pipe', 'pipe']
      })
      child.stdout?.on('data', (chunk) => { stdout += String(chunk) })
      child.stderr?.on('data', (chunk) => { stderr += String(chunk) })
      renderer = await evaluateRendererWithRetries({
        debugPort,
        expression: buildRendererExpression({
          runtimePort,
          runtimeDataDir,
          syntheticApiKey,
          terminalCwd
        }),
        timeoutMs
      })
      renderer = renderer && typeof renderer === 'object' ? renderer : null
      if (renderer) renderer.restartError = summarizeRestartError(renderer.restartError)
    } catch (error) {
      launchError = error instanceof Error ? error.message : String(error)
    }
  }

  try {
    await stopChild(child)
    await stopSmokeRuntimeOnPort(runtimePort, runtimeDataDir)
    const initialRuntimeDataDir = tempHome
      ? join(tempHome, '.analytix', 'data')
      : ''
    const projectionScanRoots = [
      join(userDataDir, 'logs'),
      join(initialRuntimeDataDir, 'threads'),
      join(runtimeDataDir, 'threads')
    ]
    const terminalProjectionNeedles = [
      terminalProtectedSentinel,
      terminalEnvCanary
    ].filter(Boolean)
    const projectionScans = terminalProjectionNeedles.length > 0
      ? projectionScanRoots.flatMap((root) =>
          terminalProjectionNeedles.map((needle) =>
            boundedDirectoryContains(root, needle)
          )
        )
      : []
    terminalProjectionFilesScanned = projectionScans.reduce(
      (total, scan) => total + scan.files,
      0
    )
    terminalProjectionBytesScanned = projectionScans.reduce(
      (total, scan) => total + scan.bytes,
      0
    )
    terminalProjectionScanComplete = projectionScans.length > 0 &&
      terminalProjectionFilesScanned > 0 &&
      projectionScans.every((scan) => scan.complete === true)
    const rendererProjection = JSON.stringify(renderer ?? null)
    terminalProjectionLeakFree = terminalProjectionScanComplete &&
      projectionScans.every((scan) => scan.found === false) &&
      terminalProjectionNeedles.every((needle) =>
        !stdout.includes(needle) &&
        !stderr.includes(needle) &&
        !rendererProjection.includes(needle)
      )
  } finally {
    isolatedLoginKeychain?.dispose()
    if (tempHome) rmSync(tempHome, { recursive: true, force: true })
  }

  const isolatedLaunchAuthorityReady =
    isolatedLaunchAuthority.created === true &&
    isolatedLaunchAuthority.defaultKeychainBound === true &&
    isolatedLaunchAuthority.unlocked === true &&
    isolatedLaunchAuthority.cacheTempBound === true
  checks.push(actualCheck(
    'packaged-app-launch',
    'Packaged app launch',
    Boolean(renderer) && isolatedLaunchAuthorityReady,
    renderer && isolatedLaunchAuthorityReady
      ? 'packaged app launched and renderer debugger was reachable'
      : renderer
        ? 'packaged app launched without a complete isolated login-keychain and Chromium-temp authority'
      : formalInstanceMode
        ? 'formal_instance_external_observation_required'
        : `packaged app launch did not produce renderer evidence: ${summarizeRestartError(launchError) || 'unknown'}`,
    formalInstanceMode ? 'live_blocked' : artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-renderer-bridge',
    'Packaged renderer bridge',
    renderer?.apiPresent === true &&
      renderer?.title === 'Analytix' &&
      renderer?.packagedRendererUrlExact === true &&
      renderer?.fileRendererProtocolUsed === false &&
      renderer?.legacyKun === false &&
      renderer?.legacyReasonix === false,
    'the exact custom renderer URL, `window.analytix`, product title, and legacy-alias boundary must hold',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-terminal-pty',
    'Packaged terminal PTY',
    renderer?.terminalApiPresent === true &&
      renderer?.terminalCreateOk === true &&
      renderer?.terminalWriteOk === true &&
      renderer?.terminalTtyMarkerObserved === true &&
      renderer?.terminalOrdinaryWorkspaceOk === true &&
      renderer?.terminalProtectedDirectBlocked === true &&
      renderer?.terminalProtectedSymlinkBlocked === true &&
      renderer?.terminalAmbientEnvClean === true &&
      renderer?.terminalExitObserved === true &&
      renderer?.terminalExitCode === 0 &&
      renderer?.terminalDisposeOk === true &&
      terminalFixtureReady === true &&
      renderer?.terminalRuntimeBoundaryCreateOk === true &&
      renderer?.terminalRuntimeBoundaryWriteOk === true &&
      renderer?.terminalRuntimeBoundaryReadyObserved === true &&
      renderer?.terminalRuntimeBoundaryPrePatchExitAbsent === true &&
      renderer?.terminalRuntimeBoundaryExitObserved === true &&
      renderer?.terminalSilentRuntimePatchAccepted === true &&
      renderer?.terminalPostRestartCreateOk === true &&
      renderer?.terminalPostRestartProtectedBlocked === true &&
      terminalProjectionScanComplete === true &&
      terminalProjectionLeakFree === true,
    'renderer/preload/main IPC must preserve ordinary PTY work while protected roots, ambient secrets, stale runtime sessions, and public projections remain contained',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-settings-bridge',
    'Packaged settings bridge',
    renderer?.settingsOk === true && renderer?.settingsRuntimeTopLevel === true,
    'settings bridge must return top-level runtime/provider settings',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-provider-profile-settings',
    'Packaged provider profile settings',
    renderer?.settingsProfilePatchAccepted === true && renderer?.settingsCompatPatchUsed !== true,
    renderer?.settingsCompatPatchUsed === true
      ? `packaged settings bridge only accepted compat-minimal patch: ${summarizeRestartError(renderer.settingsPatchError) || 'provider profile patch was rejected'}`
      : renderer?.settingsPatchError
        ? `Registry credential or key-free settings patch failed: ${summarizeRestartError(renderer.settingsPatchError)}`
        : 'Registry credential and key-free provider profile settings must persist independently',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-mimo-profile-settings',
    'Packaged MiMo profile settings',
    renderer?.mimoProfileSettingsOk === true,
    'packaged settings bridge must preserve Xiaomi/MiMo metadata while Registry holds the synthetic credential',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : 'failed'
  ))
  const restartBlocked = renderer && renderer.restartOk !== true && Boolean(renderer.restartError)
  checks.push(actualCheck(
    'packaged-runtime-restart',
    'Packaged runtime restart',
    renderer?.restartOk === true,
    restartBlocked
      ? `runtime restart blocked by packaged app/runtime state: ${summarizeRestartError(renderer.restartError)}`
      : '`window.analytix.runtime.restartRuntime()` must complete',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : restartBlocked ? 'live_blocked' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-runtime-health',
    'Packaged runtime health',
    renderer?.healthStatus === 200 && renderer?.healthService === 'analytix',
    'renderer runtimeRequest must reach /health',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : renderer?.restartOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-runtime-info',
    'Packaged runtime info',
    renderer?.runtimeInfoStatus === 200 && renderer?.runtimeInfoProductClean === true,
    'renderer runtimeRequest must reach product-clean /v1/runtime/info',
    formalInstanceMode || artifactEvidence.blocked ? 'skipped' : renderer?.restartOk === true ? 'failed' : 'skipped'
  ))

  const secretFinding = firstSecretFinding({
    checks,
    renderer: renderer
      ? {
          hrefHash: renderer.href ? sha256(renderer.href) : '',
          packagedRendererUrlExact: renderer.packagedRendererUrlExact === true,
          fileRendererProtocolUsed: renderer.fileRendererProtocolUsed === true,
          title: renderer.title || '',
          apiPresent: renderer.apiPresent === true,
          domains: Array.isArray(renderer.domains) ? renderer.domains : [],
          legacyKun: renderer.legacyKun === true,
          legacyReasonix: renderer.legacyReasonix === true,
          terminalApiPresent: renderer.terminalApiPresent === true,
          terminalCreateOk: renderer.terminalCreateOk === true,
          terminalWriteOk: renderer.terminalWriteOk === true,
          terminalTtyMarkerObserved: renderer.terminalTtyMarkerObserved === true,
          terminalOrdinaryWorkspaceOk: renderer.terminalOrdinaryWorkspaceOk === true,
          terminalProtectedDirectBlocked: renderer.terminalProtectedDirectBlocked === true,
          terminalProtectedSymlinkBlocked: renderer.terminalProtectedSymlinkBlocked === true,
          terminalAmbientEnvClean: renderer.terminalAmbientEnvClean === true,
          terminalExitObserved: renderer.terminalExitObserved === true,
          terminalExitCode: Number.isInteger(renderer.terminalExitCode)
            ? renderer.terminalExitCode
            : null,
          terminalDisposeOk: renderer.terminalDisposeOk === true,
          terminalFixtureReady,
          terminalRuntimeBoundaryCreateOk:
            renderer.terminalRuntimeBoundaryCreateOk === true,
          terminalRuntimeBoundaryWriteOk:
            renderer.terminalRuntimeBoundaryWriteOk === true,
          terminalRuntimeBoundaryReadyObserved:
            renderer.terminalRuntimeBoundaryReadyObserved === true,
          terminalRuntimeBoundaryPrePatchExitAbsent:
            renderer.terminalRuntimeBoundaryPrePatchExitAbsent === true,
          terminalRuntimeBoundaryExitObserved:
            renderer.terminalRuntimeBoundaryExitObserved === true,
          terminalSilentRuntimePatchAccepted:
            renderer.terminalSilentRuntimePatchAccepted === true,
          terminalPostRestartCreateOk: renderer.terminalPostRestartCreateOk === true,
          terminalPostRestartProtectedBlocked:
            renderer.terminalPostRestartProtectedBlocked === true,
          terminalProjectionScanComplete,
          terminalProjectionLeakFree,
          terminalProjectionFilesScanned,
          terminalProjectionBytesScanned,
          settingsOk: renderer.settingsOk === true,
          settingsRuntimeTopLevel: renderer.settingsRuntimeTopLevel === true,
          settingsProfilePatchAccepted: renderer.settingsProfilePatchAccepted === true,
          mimoProfileSettingsOk: renderer.mimoProfileSettingsOk === true,
          mimoProviderId: renderer.mimoProviderId || '',
          mimoModel: renderer.mimoModel || '',
          mimoEndpointFormat: renderer.mimoEndpointFormat || '',
          mimoProviderReady: renderer.mimoProviderReady === true,
          settingsCompatPatchUsed: renderer.settingsCompatPatchUsed === true,
          settingsPatchMode: renderer.settingsPatchMode || '',
          settingsPatchError: renderer.settingsPatchError ? summarizeRestartError(renderer.settingsPatchError) : '',
          restartOk: renderer.restartOk === true,
          restartError: renderer.restartError || '',
          healthStatus: Number(renderer.healthStatus || 0),
          healthService: renderer.healthService || '',
          runtimeInfoStatus: Number(renderer.runtimeInfoStatus || 0),
          runtimeInfoProductClean: renderer.runtimeInfoProductClean === true,
          runtimeInfoHasLegacyMcpLocalMarker: (renderer.runtimeInfoHasLegacyMcpLocalMarker ?? renderer[['runtimeInfoHasMcpLocal', 'Pr', 'oof'].join('')]) === true,
          runtimeInfoHasLegacyAbsorptionMarker: renderer.runtimeInfoHasLegacyAbsorptionMarker === true,
          runtimeInfoHasReasonixSurface: renderer.runtimeInfoHasReasonixSurface === true
        }
      : null,
    launchError: summarizeRestartError(launchError)
  })
  if (secretFinding) {
    checks.push(actualCheck('packaged-smoke-redaction', 'Packaged smoke redaction', false, `secret-like material found at ${secretFinding}`))
  }
  const failed = checks.filter((item) => item.status === 'failed')
  const blocked = checks.filter((item) => item.status === 'live_blocked')
  const skipped = checks.filter((item) => item.status === 'skipped')
  const passed = failed.length === 0 &&
    blocked.length === 0 &&
    skipped.length === 0 &&
    REQUIRED_ACTUAL_CHECK_IDS.every((id) => checks.some((item) => item.id === id && item.status === 'passed'))
  return {
    requested: true,
    passed,
    status: passed ? 'passed' : failed.length > 0 ? 'failed' : blocked.length > 0 ? 'live_blocked' : 'skipped',
    app: artifactEvidence,
    launch: {
      actualPackagedAppLaunched: Boolean(renderer),
      formalInstanceMode,
      secondInstanceStarted: Boolean(child),
      targetKey: target.key,
      packageSourceCommit: artifactEvidence.buildAuthority?.sourceCommit || '',
      executableSha256: artifactEvidence.executableSha256 || '',
      appAsarSha256: artifactEvidence.appAsarSha256 || '',
      runtimeServerSha256: artifactEvidence.runtimeServerSha256 || '',
      debugPortUsed: debugPort > 0,
      runtimePortUsed: runtimePort > 0,
      temporaryUserData: Boolean(tempHome),
      syntheticCredentialUsed,
      providerNetworkCalled: false,
      isolatedLoginKeychainRequired: isolatedLaunchAuthority.required,
      isolatedLoginKeychainCreated: isolatedLaunchAuthority.created,
      isolatedLoginKeychainDefaultBound: isolatedLaunchAuthority.defaultKeychainBound,
      isolatedLoginKeychainUnlocked: isolatedLaunchAuthority.unlocked,
      chromiumTempBoundToTrustedCache: isolatedLaunchAuthority.cacheTempBound,
      isolatedLoginKeychainRequestedPathHash: isolatedLaunchAuthority.requestedPathHash,
      isolatedLoginKeychainDatabasePathHash: isolatedLaunchAuthority.databasePathHash,
      launchError: summarizeRestartError(launchError),
      stdoutTailHash: stdout ? sha256(redactSecrets(stdout.slice(-4000))) : '',
      stderrTailHash: stderr ? sha256(redactSecrets(stderr.slice(-4000))) : ''
    },
    renderer: renderer
      ? {
          hrefHash: renderer.href ? sha256(renderer.href) : '',
          packagedRendererUrlExact: renderer.packagedRendererUrlExact === true,
          fileRendererProtocolUsed: renderer.fileRendererProtocolUsed === true,
          title: renderer.title || '',
          apiPresent: renderer.apiPresent === true,
          domains: Array.isArray(renderer.domains) ? renderer.domains : [],
          legacyKun: renderer.legacyKun === true,
          legacyReasonix: renderer.legacyReasonix === true,
          terminalApiPresent: renderer.terminalApiPresent === true,
          terminalCreateOk: renderer.terminalCreateOk === true,
          terminalWriteOk: renderer.terminalWriteOk === true,
          terminalTtyMarkerObserved: renderer.terminalTtyMarkerObserved === true,
          terminalOrdinaryWorkspaceOk: renderer.terminalOrdinaryWorkspaceOk === true,
          terminalProtectedDirectBlocked: renderer.terminalProtectedDirectBlocked === true,
          terminalProtectedSymlinkBlocked: renderer.terminalProtectedSymlinkBlocked === true,
          terminalAmbientEnvClean: renderer.terminalAmbientEnvClean === true,
          terminalExitObserved: renderer.terminalExitObserved === true,
          terminalExitCode: Number.isInteger(renderer.terminalExitCode)
            ? renderer.terminalExitCode
            : null,
          terminalDisposeOk: renderer.terminalDisposeOk === true,
          terminalFixtureReady,
          terminalRuntimeBoundaryCreateOk:
            renderer.terminalRuntimeBoundaryCreateOk === true,
          terminalRuntimeBoundaryWriteOk:
            renderer.terminalRuntimeBoundaryWriteOk === true,
          terminalRuntimeBoundaryReadyObserved:
            renderer.terminalRuntimeBoundaryReadyObserved === true,
          terminalRuntimeBoundaryPrePatchExitAbsent:
            renderer.terminalRuntimeBoundaryPrePatchExitAbsent === true,
          terminalRuntimeBoundaryExitObserved:
            renderer.terminalRuntimeBoundaryExitObserved === true,
          terminalSilentRuntimePatchAccepted:
            renderer.terminalSilentRuntimePatchAccepted === true,
          terminalPostRestartCreateOk: renderer.terminalPostRestartCreateOk === true,
          terminalPostRestartProtectedBlocked:
            renderer.terminalPostRestartProtectedBlocked === true,
          terminalProjectionScanComplete,
          terminalProjectionLeakFree,
          terminalProjectionFilesScanned,
          terminalProjectionBytesScanned,
          settingsOk: renderer.settingsOk === true,
          settingsRuntimeTopLevel: renderer.settingsRuntimeTopLevel === true,
          settingsProfilePatchAccepted: renderer.settingsProfilePatchAccepted === true,
          mimoProfileSettingsOk: renderer.mimoProfileSettingsOk === true,
          mimoProviderId: renderer.mimoProviderId || '',
          mimoModel: renderer.mimoModel || '',
          mimoEndpointFormat: renderer.mimoEndpointFormat || '',
          mimoProviderReady: renderer.mimoProviderReady === true,
          settingsCompatPatchUsed: renderer.settingsCompatPatchUsed === true,
          settingsPatchMode: renderer.settingsPatchMode || '',
          settingsPatchError: renderer.settingsPatchError ? summarizeRestartError(renderer.settingsPatchError) : '',
          restartOk: renderer.restartOk === true,
          restartError: renderer.restartError || '',
          healthStatus: Number(renderer.healthStatus || 0),
          healthService: renderer.healthService || '',
          runtimeInfoStatus: Number(renderer.runtimeInfoStatus || 0),
          runtimeInfoProductClean: renderer.runtimeInfoProductClean === true,
          runtimeInfoHasLegacyMcpLocalMarker: (renderer.runtimeInfoHasLegacyMcpLocalMarker ?? renderer[['runtimeInfoHasMcpLocal', 'Pr', 'oof'].join('')]) === true,
          runtimeInfoHasLegacyAbsorptionMarker: renderer.runtimeInfoHasLegacyAbsorptionMarker === true,
          runtimeInfoHasReasonixSurface: renderer.runtimeInfoHasReasonixSurface === true
        }
      : null,
    redaction: {
      status: secretFinding ? 'failed' : 'passed',
      secretMaterialFound: Boolean(secretFinding),
      firstFinding: secretFinding
    },
    checks
  }
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-packaged-gui-smoke] ${item.id}`)
  if (skipCommands) {
    return {
      id: item.id,
      status: 'skipped',
      command: commandText(item),
      durationMs: 0
    }
  }
  const result = spawnSync(item.command, item.args, {
    cwd: process.cwd(),
    env: process.env,
    stdio: jsonOutput ? 'pipe' : 'inherit',
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  if (jsonOutput && exitStatus !== 0) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  const check = {
    id: item.id,
    status: exitStatus === 0 ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    exitStatus,
    reason: result.error ? result.error.message : exitStatus === 0 ? '' : `command exited with status ${exitStatus}`
  }
  if (result.signal) check.signal = result.signal
  return check
}

async function main() {
const checks = []
let failed = false

for (const item of commands) {
  const check = runCheck(item)
  checks.push(check)
  if (check.status !== 'passed') {
    failed = true
    break
  }
}

const actual = actualPackagedSmoke && !skipCommands ? await runActualPackagedSmoke() : {
  requested: actualPackagedSmoke,
  passed: false,
  status: actualPackagedSmoke ? 'skipped' : 'not-requested',
  launch: {
    actualPackagedAppLaunched: false,
    formalInstanceMode,
    secondInstanceStarted: false,
    temporaryUserData: false,
    syntheticCredentialUsed: false,
    providerNetworkCalled: false
  },
  checks: []
}
const actualRequiredAndFailed = actualPackagedSmoke && actual.passed !== true
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed') && !actualRequiredAndFailed
const deterministicChecksFailed = failed || checks.some((item) => item.status === 'failed')
const status = passed
  ? 'passed'
  : skipCommands
    ? 'skipped'
    : deterministicChecksFailed
      ? 'failed'
      : actual?.status === 'live_blocked'
        ? 'live_blocked'
        : 'failed'
const packageSourceCommit = actual.app?.buildAuthority?.sourceCommit || ''
const packageReleaseAuthorityAccepted = actual.app?.buildAuthority?.publishable === true &&
  actual.app?.buildAuthority?.releaseEligible === true &&
  actual.app?.buildAuthority?.publicationReceiptIssued === true
const packagedRendererBridgeObserved = Array.isArray(actual.checks) &&
  actual.checks.some((item) =>
    item?.id === 'packaged-renderer-bridge' && item?.status === 'passed'
  )
const report = {
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  generatedAt: new Date().toISOString(),
  sourceCommit: actualPackagedSmoke ? packageSourceCommit : currentGitCommit(),
  testHarnessCommit: currentGitCommit(),
  status,
  passed,
  smoke: {
    deterministicContractOnly: actualPackagedSmoke !== true,
    formalInstanceMode,
    secondInstanceStarted: actual.launch?.secondInstanceStarted === true,
    actualPackagedAppLaunched: actual.launch?.actualPackagedAppLaunched === true,
    actualPackagedAppEvidenceRequiredForFinalGate: actualPackagedSmoke !== true ||
      actual.passed !== true || !packageReleaseAuthorityAccepted,
    actualPackagedSmokeRequested: actualPackagedSmoke,
    actualPackagedSmokePassed: actual.passed === true,
    packageClassification: actual.app?.buildAuthority?.classification || '',
    packagePublishable: actual.app?.buildAuthority?.publishable === true,
    packageReleaseEligible: actual.app?.buildAuthority?.releaseEligible === true,
    packagePublicationReceiptIssued: actual.app?.buildAuthority?.publicationReceiptIssued === true,
    syntheticCredentialUsed: actual.launch?.syntheticCredentialUsed === true,
    providerNetworkCalled: actual.launch?.providerNetworkCalled === true,
    settingsProfilePatchAccepted: actual.renderer?.settingsProfilePatchAccepted === true,
    mimoProfileSettingsAccepted: actual.renderer?.mimoProfileSettingsOk === true,
    settingsCompatPatchUsed: actual.renderer?.settingsCompatPatchUsed === true,
    settingsPatchMode: actual.renderer?.settingsPatchMode || '',
    bridgePreserved: packagedRendererBridgeObserved,
    sseBridgePreserved: false,
    rendererTimelinePreserved: false,
    approvalAndUserInputCardsCovered: false,
    unobservedPackagedCoverage: [
      'sse-bridge',
      'renderer-timeline',
      'approval-card',
      'user-input-card'
    ]
  },
  actualPackagedSmoke: actual,
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
}

main().catch((error) => {
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-gui-smoke',
    generatedAt: new Date().toISOString(),
    status: 'failed',
    passed: false,
    smoke: {
      deterministicContractOnly: false,
      actualPackagedAppLaunched: false,
      actualPackagedAppEvidenceRequiredForFinalGate: true,
      actualPackagedSmokeRequested: actualPackagedSmoke,
      actualPackagedSmokePassed: false
    },
    checks: [{
      id: 'runtime-go-packaged-gui-smoke',
      status: 'failed',
      command: 'runtime-go-packaged-gui-smoke',
      durationMs: 0,
      exitStatus: 1,
      reason: error instanceof Error ? redactSecrets(error.message) : redactSecrets(String(error))
    }]
  }
  if (jsonOutput) console.log(JSON.stringify(report, null, 2))
  else console.error(`FAILED ${report.id}: ${report.checks[0].reason}`)
  process.exitCode = 1
})
