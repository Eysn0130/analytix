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
  readSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import http from 'node:http'
import net from 'node:net'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import process from 'node:process'
import { createIsolatedDarwinLoginKeychain } from './runtime-go-packaged-milestone-a.mjs'

const require = createRequire(import.meta.url)
const {
  PACKAGED_BUILD_AUTHORITY_CONTRACT,
  PACKAGED_BUILD_AUTHORITY_FILE,
  _internals: packagedAuthorityContract
} = require('./after-pack.cjs')

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const goCommand = process.env.GO || 'go'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const actualOnly = args.has('--actual-only')
const reportOnly = args.has('--no-gate')
const noWrite = args.has('--no-write') || skipCommands
const defaultEvidencePath = 'docs/analytix/upstreams/runtime-go-live-evidence/packaged-session-soak.json'
const formalInstanceMode = args.has('--formal-instance') || process.env.ANALYTIX_RUNTIME_GO_FORMAL_INSTANCE === '1'
const actualPackagedSoak = formalInstanceMode || args.has('--actual') ||
  process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK === '1' ||
  process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE === '1'
const DEFAULT_TIMEOUT_MS = 45_000
const REQUIRED_ACTUAL_CHECK_IDS = [
  'packaged-app-artifact',
  'packaged-app-launch',
  'packaged-renderer-bridge',
  'packaged-settings-bridge',
  'packaged-provider-profile-settings',
  'packaged-runtime-restart',
  'packaged-session-thread-create',
  'packaged-session-turn-create',
  'packaged-session-sse-replay',
  'packaged-session-tool-timeline',
  'packaged-session-mimo-plan',
  'packaged-session-attachment-fallback',
  'packaged-session-approval-user-input',
  'packaged-session-fork',
  'packaged-session-resume',
  'packaged-session-thread-list',
  'packaged-session-provider-redaction'
]

const GO_SESSION_DURABLE_TESTS = [
  'TestRuntimeServerForkAndResumePreserveToolPairingHistory',
  'TestRuntimeServerForkTurnIDTruncatesAndRewritesHistory',
  'TestRuntimeServerThreadSummarySearchAndForkCountsMatchProductContract',
  'TestRuntimeServerSSEReplayUsesLastEventIDWhenSinceSeqOmitted',
  'TestRuntimeServerSubagentContinueAndForkUseDurableTranscript',
  'TestRuntimeServerCandidateDurableRootHighRiskFailsClosedAndSurvivesRestart'
]

const commands = [
  {
    id: 'go-session-durable-contract',
    compiledGoTestPackage: 'analytix.local/runtime-go',
    cwd: `${process.cwd()}/packages/runtime-go`,
    tests: GO_SESSION_DURABLE_TESTS
  },
  {
    id: 'typescript-retired-backend-session-contract',
    command: npmCommand,
    args: [
      'run',
      'runtime:go:rollback-retirement-evidence',
      '--',
      '--json'
    ],
    cwd: process.cwd()
  }
]

function argValue(name, fallback = '') {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

const evidencePath = argValue(
  '--output',
  process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOAK_EVIDENCE || defaultEvidencePath
)

function writeEvidenceReport(report) {
  if (noWrite) return
  const absoluteEvidencePath = resolve(process.cwd(), evidencePath)
  report.outputPath = absoluteEvidencePath
  mkdirSync(dirname(absoluteEvidencePath), { recursive: true })
  writeFileSync(absoluteEvidencePath, JSON.stringify(report, null, 2), 'utf8')
}

function commandText(item) {
  if (Array.isArray(item.tests) && item.compiledGoTestPackage) {
    const executions = item.tests
      .map((testName) => `${goCommand} tool test2json -p ${item.compiledGoTestPackage} <task-owned-test-binary> -test.v=test2json -test.run ^${testName}$ -test.count=1`)
      .join(' [fresh-process-serial] ')
    return `${goCommand} test -c -o <task-owned-test-binary> . [compile-once] ${executions}`
  }
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
      if (/(api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|secret|password|signature|sig|auth|jwt)/i.test(key)) {
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
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOAK_APP_PATH ||
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
    appOutDir: dirname(appPath),
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
        const page = targets.find((target) => target.type === 'page' && target.webSocketDebuggerUrl) ||
          targets.find((target) => target.webSocketDebuggerUrl)
        if (page) return page
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
        Math.min(timeoutMs, Math.max(1_000, deadline - Date.now()))
      )
    } catch (error) {
      lastError = error
      if (!isTransientCdpEvaluationError(error)) throw error
      await sleep(Math.min(250 + attempt * 100, 1_000, Math.max(0, deadline - Date.now())))
    }
  }
  throw lastError || new Error('CDP Runtime.evaluate timeout')
}

function startContractProvider() {
  const requests = []
  const server = http.createServer((req, res) => {
    if (req.method !== 'POST' || req.url !== '/v1/chat/completions') {
      res.writeHead(404, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: { message: 'contract provider route not found' } }))
      return
    }
    let raw = ''
    req.setEncoding('utf8')
    req.on('data', (chunk) => { raw += chunk })
    req.on('end', () => {
      const currentPrompt = currentUserPrompt(raw)
      const parsedBody = parseProviderBody(raw)
      const currentToolResult = currentTurnHasToolResult(parsedBody)
      requests.push({
        authorizationConfigured: Boolean(req.headers.authorization),
        requestPath: req.url,
        bodyModel: typeof parsedBody.model === 'string' ? parsedBody.model : '',
        bodyContainsInitialPrompt: raw.includes('Run packaged session soak.'),
        bodyContainsToolTimelinePrompt: raw.includes('Run packaged session tool timeline.'),
        bodyContainsToolTimelineFollowup: raw.includes('Run packaged session tool timeline.') &&
          currentToolResult,
        bodyContainsMimoPlanPrompt: raw.includes('Run packaged MiMo plan mode.'),
        bodyContainsMimoPlanResult: raw.includes('Run packaged MiMo plan mode.') &&
          currentToolResult,
        bodyContainsAttachmentPrompt: raw.includes('Run packaged session attachment fallback.'),
        bodyContainsAttachmentVirtualPath: raw.includes('FilePath: attachment://'),
        bodyContainsAttachmentEnvelope: raw.includes('[Attached file]'),
        bodyContainsAttachmentProtocol: raw.includes('attachment://'),
        bodyContainsAttachmentText: raw.includes('Packaged attachment fallback text'),
        bodyContainsApprovalDenyPrompt: raw.includes('Run packaged session approval deny.'),
        bodyContainsApprovalDenyResult: currentToolResult &&
          raw.includes('Run packaged session approval deny.'),
        bodyContainsApprovalAllowPrompt: raw.includes('Run packaged session approval allow.'),
        bodyContainsApprovalAllowResult: currentToolResult &&
          raw.includes('Run packaged session approval allow.'),
        bodyContainsUserInputPrompt: raw.includes('Run packaged session user input.'),
        bodyContainsUserInputAnswer: currentToolResult &&
          raw.includes('Packaged user input answer'),
        bodyContainsForkPrompt: raw.includes('Continue packaged session soak from fork.'),
        bodyContainsResumePrompt: raw.includes('Continue packaged session soak from resume.'),
        bodyContainsInitialPromptAsHistory: raw.includes('Run packaged session soak.') &&
          (raw.includes('Continue packaged session soak from fork.') || raw.includes('Continue packaged session soak from resume.'))
      })
      const isAttachmentRequest = currentPrompt.includes('Run packaged session attachment fallback.')
      const isToolTimelineRequest = currentPrompt.includes('Run packaged session tool timeline.')
      const isMimoPlanRequest = currentPrompt.includes('Run packaged MiMo plan mode.')
      if (isToolTimelineRequest && !currentToolResult) {
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_packaged_ls","type":"function","function":{"name":"ls","arguments":"{\\"path\\":\\".\\"}"}}]},"finish_reason":"tool_calls"}]}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (isToolTimelineRequest) {
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"content":"packaged tool timeline ok"},"finish_reason":"stop"}]}',
          'data: {"choices":[],"usage":{"prompt_tokens":34,"completion_tokens":5,"total_tokens":39,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":27}}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (isMimoPlanRequest && !currentToolResult) {
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_packaged_mimo_plan","type":"function","function":{"name":"create_plan","arguments":"{\\"markdown\\":\\"# Packaged MiMo plan\\",\\"operation\\":\\"draft\\",\\"source_request\\":\\"Run packaged MiMo plan mode.\\",\\"title\\":\\"Packaged MiMo\\",\\"plan_id\\":\\"packaged-mimo\\",\\"plan_relative_path\\":\\".analytixsdd/plan/packaged-mimo.md\\"}"}}]},"finish_reason":"tool_calls"}]}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (isMimoPlanRequest) {
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"content":"packaged mimo plan saved"},"finish_reason":"stop"}]}',
          'data: {"choices":[],"usage":{"prompt_tokens":36,"completion_tokens":5,"total_tokens":41,"cached_tokens":8}}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (isAttachmentRequest) {
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"content":"packaged attachment fallback ok"},"finish_reason":"stop"}]}',
          'data: {"choices":[],"usage":{"prompt_tokens":29,"completion_tokens":4,"total_tokens":33,"prompt_cache_hit_tokens":6,"prompt_cache_miss_tokens":23}}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (currentPrompt.includes('Run packaged session approval deny.')) {
        if (currentToolResult) {
          res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
          res.end([
            'data: {"choices":[{"delta":{"content":"packaged approval deny ok"},"finish_reason":"stop"}]}',
            'data: {"choices":[],"usage":{"prompt_tokens":31,"completion_tokens":4,"total_tokens":35,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":26}}',
            'data: [DONE]'
          ].join('\n\n'))
          return
        }
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_packaged_approval_deny","type":"function","function":{"name":"write_file","arguments":"{\\"path\\":\\"approval-deny.txt\\",\\"content\\":\\"denied\\"}"}}]},"finish_reason":"tool_calls"}]}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (currentPrompt.includes('Run packaged session approval allow.')) {
        if (currentToolResult) {
          res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
          res.end([
            'data: {"choices":[{"delta":{"content":"packaged approval allow ok"},"finish_reason":"stop"}]}',
            'data: {"choices":[],"usage":{"prompt_tokens":32,"completion_tokens":4,"total_tokens":36,"prompt_cache_hit_tokens":6,"prompt_cache_miss_tokens":26}}',
            'data: [DONE]'
          ].join('\n\n'))
          return
        }
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_packaged_approval_allow","type":"function","function":{"name":"write_file","arguments":"{\\"path\\":\\"approval-allow.txt\\",\\"content\\":\\"allowed\\"}"}}]},"finish_reason":"tool_calls"}]}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      if (currentPrompt.includes('Run packaged session user input.')) {
        if (currentToolResult) {
          res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
          res.end([
            'data: {"choices":[{"delta":{"content":"packaged user input ok"},"finish_reason":"stop"}]}',
            'data: {"choices":[],"usage":{"prompt_tokens":33,"completion_tokens":4,"total_tokens":37,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":26}}',
            'data: [DONE]'
          ].join('\n\n'))
          return
        }
        res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
        res.end([
          'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_packaged_user_input","type":"function","function":{"name":"user_input","arguments":"{\\"prompt\\":\\"Provide packaged session answer\\",\\"questions\\":[{\\"question\\":\\"Provide packaged session answer\\",\\"header\\":\\"Answer\\"}]}"}}]},"finish_reason":"tool_calls"}]}',
          'data: [DONE]'
        ].join('\n\n'))
        return
      }
      res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
      res.end([
        'data: {"choices":[{"delta":{"content":"packaged session soak ok"},"finish_reason":"stop"}]}',
        'data: {"choices":[],"usage":{"prompt_tokens":21,"completion_tokens":4,"total_tokens":25,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":16}}',
        'data: [DONE]'
      ].join('\n\n'))
    })
  })
  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        reject(new Error('contract provider did not bind a TCP port'))
        return
      }
      server.off('error', reject)
      resolve({
        url: `http://127.0.0.1:${address.port}`,
        requests,
        close: () => new Promise((done) => server.close(() => done()))
      })
    })
  })
}

function parseProviderBody(raw) {
  try {
    return JSON.parse(raw)
  } catch {
    return {}
  }
}

function currentUserPrompt(raw) {
  try {
    const body = parseProviderBody(raw)
    const messages = Array.isArray(body.messages) ? body.messages : []
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      const item = messages[index]
      if (!item || item.role !== 'user') continue
      return messageContentText(item.content)
    }
  } catch {
    return ''
  }
  return ''
}

function currentTurnHasToolResult(body) {
  const messages = Array.isArray(body.messages) ? body.messages : []
  let lastUserIndex = -1
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.role === 'user') {
      lastUserIndex = index
      break
    }
  }
  return messages.slice(lastUserIndex + 1).some((message) => message?.role === 'tool')
}

function messageContentText(content) {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return ''
  return content.map((part) => {
    if (!part || typeof part !== 'object') return ''
    if (typeof part.text === 'string') return part.text
    if (typeof part.content === 'string') return part.content
    return ''
  }).filter(Boolean).join('\n')
}

function buildRendererExpression({ providerBaseUrl, providerId, model, runtimePort, runtimeDataDir, workspace, syntheticApiKey }) {
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
      title: productTitle,
      apiPresent: !!api,
      domains: api ? Object.keys(api).sort() : [],
      legacyKun: !!window[${JSON.stringify(legacyKunAlias)}],
      legacyReasonix: !!window[${JSON.stringify(legacyEngineAlias)}],
      settingsOk: false,
      settingsRuntimeTopLevel: false,
      settingsProfilePatchAccepted: false,
      settingsCompatPatchUsed: false,
      settingsPatchMode: '',
      settingsPatchError: '',
      restartOk: false,
      restartError: '',
      healthStatus: 0,
      healthService: '',
      threadCreateOk: false,
      turnCreateOk: false,
      sseReplayOk: false,
      toolTurnOk: false,
      toolTimelineOk: false,
      mimoPlanThreadOk: false,
      mimoPlanTurnOk: false,
      mimoPlanReplayOk: false,
      attachmentUploadOk: false,
      attachmentTurnOk: false,
      attachmentSseReplayOk: false,
      attachmentMetadataOk: false,
      approvalDenyTurnOk: false,
      approvalDenyRequestedOk: false,
      approvalDenyResolvedOk: false,
      approvalDenyNoExecuteOk: false,
      approvalAllowTurnOk: false,
      approvalAllowRequestedOk: false,
      approvalAllowResolvedOk: false,
      approvalAllowExecutedOk: false,
      userInputTurnOk: false,
      userInputRequestedOk: false,
      userInputResolvedOk: false,
      userInputReplayOk: false,
      forkOk: false,
      forkTurnOk: false,
      resumeOk: false,
      resumeTurnOk: false,
      threadListOk: false,
      usageOk: false,
      initialThreadId: '',
      initialTurnId: '',
      toolTurnId: '',
      forkThreadId: '',
      resumeThreadId: '',
      error: ''
    };
    function parseJSON(response) {
      try { return JSON.parse(response && response.body || '{}'); } catch { return {}; }
    }
    const replayCursorByThread = new Map();
    function publicReplayEvents(batch) {
      return batch.flatMap((event) => event && event.kind === 'general_terminal_batch' &&
        Array.isArray(event.events) ? event.events : [event]);
    }
    async function acknowledgeReplayBatch(threadId, streamId, batch) {
      if (batch.some((event) => event && event.kind === 'accepted_final_batch')) return false;
      const maxSeq = batch.reduce((seq, event) =>
        Number.isSafeInteger(event && event.seq) ? Math.max(seq, event.seq) : seq, 0);
      if (maxSeq === 0) return true;
      const acknowledged = await api.runtime.ackSseEvent(streamId, maxSeq).catch(() => false);
      if (acknowledged) replayCursorByThread.set(threadId,
        Math.max(replayCursorByThread.get(threadId) || 0, maxSeq));
      return acknowledged;
    }
    async function request(path, method, payload) {
      const response = await api.runtime.runtimeRequest(
        path,
        method || 'GET',
        payload === undefined ? undefined : JSON.stringify(payload)
      );
      const body = parseJSON(response);
      if (!response || response.status < 200 || response.status >= 300) {
        throw new Error(method + ' ' + path + ' returned ' + (response && response.status) + ': ' + String(response && response.body || '').slice(0, 240));
      }
      return body;
    }
    async function collectSseReplay(threadId, turnId, expectedText, requireToolEvents) {
      const streamId = 'packaged-session-soak-' + Math.random().toString(36).slice(2);
      const events = [];
      const errors = [];
      let settled = false;
      let pendingAck = Promise.resolve(true);
      const unsubscribers = [];
      const waitForReplay = new Promise((resolve) => {
        const finish = (ok) => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          resolve(ok);
        };
        const timer = setTimeout(() => finish(false), 30000);
        unsubscribers.push(api.runtime.onSseEvent((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          const batch = Array.isArray(payload.events) ? payload.events : [];
          pendingAck = pendingAck.then((ok) => ok && acknowledgeReplayBatch(threadId, streamId, batch));
          events.push(...publicReplayEvents(batch));
          const turnEvents = events.filter((event) => event && event.turnId === turnId);
          const kinds = new Set(turnEvents.map((event) => event.kind));
          const assistantText = turnEvents.some((event) =>
            (event.kind === 'assistant_text_delta' &&
              (event.text || event.delta || '').includes(expectedText)) ||
            (event.kind === 'item_completed' && event.item &&
              event.item.kind === 'assistant_text' &&
              (event.item.text || '').includes(expectedText)));
          const ok = kinds.has('turn_started') && assistantText &&
            kinds.has('usage') && kinds.has('turn_completed') &&
            (!requireToolEvents ||
              (kinds.has('tool_call_ready') && kinds.has('tool_call_started') &&
                kinds.has('tool_call_finished')));
          if (ok) finish(true);
        }));
        unsubscribers.push(api.runtime.onSseError((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          errors.push(payload);
          finish(false);
        }));
        unsubscribers.push(api.runtime.onSseEnd((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          const turnEvents = events.filter((event) => event && event.turnId === turnId);
          const kinds = new Set(turnEvents.map((event) => event.kind));
          finish(kinds.has('turn_completed') && turnEvents.some((event) =>
            event.kind === 'item_completed' && event.item &&
            event.item.kind === 'assistant_text' &&
            (event.item.text || '').includes(expectedText)) &&
            (!requireToolEvents || (kinds.has('tool_call_ready') &&
              kinds.has('tool_call_started') && kinds.has('tool_call_finished'))));
        }));
      });
      await api.runtime.startSse(threadId, replayCursorByThread.get(threadId) || 0, streamId);
      const ok = await waitForReplay;
      const acked = await pendingAck;
      await api.runtime.stopSse(streamId).catch(() => false);
      for (const unsubscribe of unsubscribers) {
        try { unsubscribe(); } catch {}
      }
      return { ok: ok && acked, eventCount: events.length, errorCount: errors.length,
        eventKinds: [...new Set(events.map((event) => event && event.kind).filter(Boolean))] };
    }
    async function collectGateReplay(threadId, turnId, requiredKind, idField) {
      const streamId = 'packaged-session-gate-' + Math.random().toString(36).slice(2);
      const events = [];
      const errors = [];
      let settled = false;
      let pendingAck = Promise.resolve(true);
      const unsubscribers = [];
      const waitForReplay = new Promise((resolve) => {
        const finish = (result) => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          resolve(result);
        };
        const timer = setTimeout(() => finish({ ok: false, id: '', eventCount: events.length, errorCount: errors.length }), 30000);
        const inspect = () => {
          const matching = events.find((event) => {
            if (!event || event.kind !== requiredKind) return false;
            return event.turnId === turnId || event.turn_id === turnId;
          });
          if (!matching) return;
          const id = matching[idField] || matching.itemId || matching.item_id || '';
          finish({ ok: !!id, id, eventCount: events.length, errorCount: errors.length });
        };
        unsubscribers.push(api.runtime.onSseEvent((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          const batch = Array.isArray(payload.events) ? payload.events : [];
          pendingAck = pendingAck.then((ok) => ok && acknowledgeReplayBatch(threadId, streamId, batch));
          events.push(...publicReplayEvents(batch));
          inspect();
        }));
        unsubscribers.push(api.runtime.onSseError((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          errors.push(payload);
          finish({ ok: false, id: '', eventCount: events.length, errorCount: errors.length });
        }));
        unsubscribers.push(api.runtime.onSseEnd((payload) => {
          if (!payload || payload.streamId !== streamId) return;
          inspect();
          if (!settled) finish({ ok: false, id: '', eventCount: events.length, errorCount: errors.length });
        }));
      });
      await api.runtime.startSse(threadId, replayCursorByThread.get(threadId) || 0, streamId);
      const result = await waitForReplay;
      const acked = await pendingAck;
      await api.runtime.stopSse(streamId).catch(() => false);
      for (const unsubscribe of unsubscribers) {
        try { unsubscribe(); } catch {}
      }
      const turnEvents = events.filter((event) => event &&
        (event.turnId === turnId || event.turn_id === turnId));
      return { ...result, ok: result.ok && acked,
        eventKinds: [...new Set(events.map((event) => event && event.kind).filter(Boolean))],
        turnEventKinds: [...new Set(turnEvents.map((event) => event.kind))] };
    }
    try {
      if (!api) return out;
      const initial = await api.settings.getSettings();
      out.settingsOk = !!(initial && initial.runtime && initial.provider);
      out.settingsRuntimeTopLevel = !!(initial && initial.runtime && !initial.agent && !initial.agents && !initial.reasonix);
      const profilePatch = {
        provider: {
          activeProviderId: ${JSON.stringify(providerId)},
          baseUrl: ${JSON.stringify(`${providerBaseUrl}/v1`)},
          providers: [{
            id: ${JSON.stringify(providerId)},
            name: 'Packaged Session Soak Provider',
            baseUrl: ${JSON.stringify(`${providerBaseUrl}/v1`)},
            endpointFormat: 'chat_completions',
            models: ['mimo-v2.5-pro', 'mimo-v2.5'],
            modelProfiles: {
              'mimo-v2.5-pro': {
                aliases: ['mimo-v2.5-pro-ultraspeed'],
                contextWindowTokens: 1000000,
                inputModalities: ['text'],
                outputModalities: ['text'],
                supportsToolCalling: true,
                messageParts: ['text'],
                reasoning: {
                  supportedEfforts: ['off', 'low', 'medium', 'high', 'max'],
                  defaultEffort: 'high',
                  requestProtocol: 'mimo-chat-completions'
                }
              },
              'mimo-v2.5': {
                contextWindowTokens: 1000000,
                inputModalities: ['text', 'image'],
                outputModalities: ['text'],
                supportsToolCalling: true,
                messageParts: ['text', 'image_url'],
                reasoning: {
                  supportedEfforts: ['off', 'low', 'medium', 'high', 'max'],
                  defaultEffort: 'high',
                  requestProtocol: 'mimo-chat-completions'
                }
              }
            }
          }]
        },
        runtime: {
          port: ${JSON.stringify(runtimePort)},
          dataDir: ${JSON.stringify(runtimeDataDir)},
          autoStart: true,
          providerId: ${JSON.stringify(providerId)},
          model: ${JSON.stringify(model)}
        }
      };
      try {
        await api.settings.setSettings(profilePatch);
        out.settingsProfilePatchAccepted = true;
        out.settingsPatchMode = 'provider-profile';
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
      const health = await api.runtime.runtimeRequest('/health', 'GET');
      out.healthStatus = health.status;
      out.healthService = JSON.parse(health.body || '{}').service || '';
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
            id: ${JSON.stringify(providerId)},
            kind: 'openai-compatible',
            endpoint: ${JSON.stringify(`${providerBaseUrl}/v1`)},
            proxy: '',
            models: ['mimo-v2.5-pro', 'mimo-v2.5'],
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
          ? registry.providers.find((item) => item && item.id === ${JSON.stringify(providerId)}) : null;
        if (!registered || registered.credentialConfigured !== true) {
          throw new Error('synthetic_provider_credential_not_configured');
        }
      } catch (error) {
        out.settingsPatchError = error && (error.stack || error.message) || String(error);
        return out;
      }

      const thread = await request('/v1/threads', 'POST', {
        title: 'Packaged Session Soak',
        workspace: ${JSON.stringify(workspace)},
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        mode: 'agent'
      });
      const threadId = thread.id || thread.thread_id || '';
      out.initialThreadId = threadId;
      out.threadCreateOk = !!threadId;

      const turn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session soak.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: true
      });
      const turnId = turn.turnId || turn.turn_id || '';
      out.initialTurnId = turnId;
      out.turnCreateOk = !!turnId && (turn.threadId === threadId || turn.thread_id === threadId);

      const replay = await collectSseReplay(threadId, turnId, 'packaged session soak ok', false);
      out.initialReplay = replay;
      out.sseReplayOk = replay.ok === true && replay.eventCount > 0 && replay.errorCount === 0;

      const toolTurn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session tool timeline.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: true
      });
      const toolTurnId = toolTurn.turnId || toolTurn.turn_id || '';
      out.toolTurnId = toolTurnId;
      out.toolTurnOk = !!toolTurnId && (toolTurn.threadId === threadId || toolTurn.thread_id === threadId);
      const toolReplay = await collectSseReplay(threadId, toolTurnId, 'packaged tool timeline ok', true);
      out.toolReplay = toolReplay;
      out.toolTimelineOk = toolReplay.ok === true && toolReplay.eventCount > 0 && toolReplay.errorCount === 0;

      const mimoPlanThread = await request('/v1/threads', 'POST', {
        title: 'Packaged MiMo Plan Soak',
        workspace: ${JSON.stringify(workspace)},
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        mode: 'plan'
      });
      const mimoPlanThreadId = mimoPlanThread.id || mimoPlanThread.thread_id || '';
      out.mimoPlanThreadOk = !!mimoPlanThreadId;
      const mimoPlanTurn = await request('/v1/threads/' + encodeURIComponent(mimoPlanThreadId) + '/turns', 'POST', {
        prompt: 'Run packaged MiMo plan mode.',
        async: true,
        mode: 'plan',
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        guiPlan: {
          operation: 'draft',
          workspaceRoot: ${JSON.stringify(workspace)},
          relativePath: '.analytixsdd/plan/packaged-mimo.md',
          planId: 'packaged-mimo',
          sourceRequest: 'Run packaged MiMo plan mode.',
          title: 'Packaged MiMo'
        },
        approvalPolicy: 'never',
        sandboxMode: 'workspace-write',
        disableUserInput: true
      });
      const mimoPlanTurnId = mimoPlanTurn.turnId || mimoPlanTurn.turn_id || '';
      out.mimoPlanTurnOk = !!mimoPlanTurnId && (mimoPlanTurn.threadId === mimoPlanThreadId || mimoPlanTurn.thread_id === mimoPlanThreadId);
      const mimoPlanReplay = await collectSseReplay(mimoPlanThreadId, mimoPlanTurnId, 'packaged mimo plan saved', true);
      out.mimoPlanReplay = mimoPlanReplay;
      out.mimoPlanReplayOk = mimoPlanReplay.ok === true && mimoPlanReplay.eventCount > 0 && mimoPlanReplay.errorCount === 0;

      const attachmentUpload = await request('/v1/attachments', 'POST', {
        name: 'packaged-attachment.txt',
        mimeType: 'text/plain',
        dataBase64: btoa('Packaged attachment fallback text'),
        documentText: 'Packaged attachment fallback text',
        threadId,
        workspace: ${JSON.stringify(workspace)}
      });
      const attachment = attachmentUpload && attachmentUpload.attachment || {};
      const attachmentId = attachment.id || '';
      out.attachmentUploadOk = !!attachmentId;
      out.attachmentMetadataOk = attachment.scope === 'thread' &&
        attachment.name === 'packaged-attachment.txt' && attachment.localFilePath === undefined;
      const attachmentTurn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session attachment fallback.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        attachmentIds: [attachmentId],
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: true
      });
      const attachmentTurnId = attachmentTurn.turnId || attachmentTurn.turn_id || '';
      out.attachmentTurnOk = !!attachmentTurnId && (attachmentTurn.threadId === threadId || attachmentTurn.thread_id === threadId);
      const attachmentReplay = await collectSseReplay(threadId, attachmentTurnId, 'packaged attachment fallback ok', false);
      out.attachmentSseReplayOk = attachmentReplay.ok === true && attachmentReplay.eventCount > 0 && attachmentReplay.errorCount === 0;

      const approvalDenyTurn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session approval deny.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'always',
        sandboxMode: 'workspace-write',
        disableUserInput: true
      });
      const approvalDenyTurnId = approvalDenyTurn.turnId || approvalDenyTurn.turn_id || '';
      out.approvalDenyTurnOk = !!approvalDenyTurnId && (approvalDenyTurn.threadId === threadId || approvalDenyTurn.thread_id === threadId);
      const approvalDenyGate = await collectGateReplay(threadId, approvalDenyTurnId, 'approval_requested', 'approvalId');
      out.approvalDenyGate = approvalDenyGate;
      out.approvalDenyRequestedOk = approvalDenyGate.ok === true;
      if (approvalDenyGate.id) {
        const approvalDeny = await request('/v1/approvals/' + encodeURIComponent(approvalDenyGate.id), 'POST', {
          decision: 'deny'
        });
        out.approvalDenyResolvedOk = approvalDeny.status === 'denied' || approvalDeny.decision === 'deny';
      }
      const approvalDenyReplay = await collectSseReplay(threadId, approvalDenyTurnId, 'packaged approval deny ok', false);
      out.approvalDenyNoExecuteOk = approvalDenyReplay.ok === true && approvalDenyReplay.eventCount > 0 && approvalDenyReplay.errorCount === 0;

      const approvalAllowTurn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session approval allow.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'always',
        sandboxMode: 'workspace-write',
        disableUserInput: true
      });
      const approvalAllowTurnId = approvalAllowTurn.turnId || approvalAllowTurn.turn_id || '';
      out.approvalAllowTurnOk = !!approvalAllowTurnId && (approvalAllowTurn.threadId === threadId || approvalAllowTurn.thread_id === threadId);
      const approvalAllowGate = await collectGateReplay(threadId, approvalAllowTurnId, 'approval_requested', 'approvalId');
      out.approvalAllowGate = approvalAllowGate;
      out.approvalAllowRequestedOk = approvalAllowGate.ok === true;
      if (approvalAllowGate.id) {
        const approvalAllow = await request('/v1/approvals/' + encodeURIComponent(approvalAllowGate.id), 'POST', {
          decision: 'allow'
        });
        out.approvalAllowResolvedOk = approvalAllow.status === 'allowed' || approvalAllow.decision === 'allow';
      }
      const approvalAllowReplay = await collectSseReplay(threadId, approvalAllowTurnId, 'packaged approval allow ok', false);
      out.approvalAllowExecutedOk = approvalAllowReplay.ok === true && approvalAllowReplay.eventCount > 0 && approvalAllowReplay.errorCount === 0;

      const userInputTurn = await request('/v1/threads/' + encodeURIComponent(threadId) + '/turns', 'POST', {
        prompt: 'Run packaged session user input.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: false
      });
      const userInputTurnId = userInputTurn.turnId || userInputTurn.turn_id || '';
      out.userInputTurnOk = !!userInputTurnId && (userInputTurn.threadId === threadId || userInputTurn.thread_id === threadId);
      const userInputGate = await collectGateReplay(threadId, userInputTurnId, 'user_input_requested', 'inputId');
      out.userInputGate = userInputGate;
      out.userInputRequestedOk = userInputGate.ok === true;
      if (userInputGate.id) {
        await request('/v1/user-inputs/' + encodeURIComponent(userInputGate.id), 'POST', {
          answers: [{ id: 'answer', value: 'Packaged user input answer' }]
        });
        out.userInputResolvedOk = true;
      }
      const userInputReplay = await collectSseReplay(threadId, userInputTurnId, 'packaged user input ok', false);
      out.userInputReplayOk = userInputReplay.ok === true && userInputReplay.eventCount > 0 && userInputReplay.errorCount === 0;

      const detail = await request('/v1/threads/' + encodeURIComponent(threadId), 'GET');
      out.threadListOk = detail.id === threadId && Array.isArray(detail.turns);

      const fork = await request('/v1/threads/' + encodeURIComponent(threadId) + '/fork', 'POST', {
        relation: 'fork',
        title: 'Packaged Session Soak Fork'
      });
      const forkId = fork.id || '';
      out.forkThreadId = forkId;
      out.forkOk = !!forkId && fork.parentThreadId === threadId;
      const forkTurn = await request('/v1/threads/' + encodeURIComponent(forkId) + '/turns', 'POST', {
        prompt: 'Continue packaged session soak from fork.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: true
      });
      out.forkTurnOk = !!(forkTurn.turnId || forkTurn.turn_id);

      const resume = await request('/v1/sessions/' + encodeURIComponent(threadId) + '/resume-thread', 'POST', {
        workspace: ${JSON.stringify(workspace)},
        model: ${JSON.stringify(model)},
        mode: 'agent'
      });
      const resumeId = resume.thread_id || resume.threadId || '';
      out.resumeThreadId = resumeId;
      out.resumeOk = resume.session_id === threadId && !!resumeId;
      const resumeTurn = await request('/v1/threads/' + encodeURIComponent(resumeId) + '/turns', 'POST', {
        prompt: 'Continue packaged session soak from resume.',
        async: true,
        providerId: ${JSON.stringify(providerId)},
        model: ${JSON.stringify(model)},
        approvalPolicy: 'never',
        sandboxMode: 'read-only',
        disableUserInput: true
      });
      out.resumeTurnOk = !!(resumeTurn.turnId || resumeTurn.turn_id);

      const list = await request('/v1/threads?include=side&search=Packaged%20Session%20Soak', 'GET');
      const threads = Array.isArray(list.threads) ? list.threads : [];
      out.threadListOk = out.threadListOk && threads.some((item) => item && item.id === threadId) &&
        threads.some((item) => item && item.id === forkId);
      const usage = await request('/v1/usage?group_by=thread&thread_id=' + encodeURIComponent(threadId), 'GET');
      const buckets = Array.isArray(usage.buckets) ? usage.buckets : [];
      out.usageOk = buckets.some((bucket) => {
        const total = Number(bucket && (bucket.total_tokens || bucket.totalTokens || 0));
        return bucket && (bucket.thread_id === threadId || bucket.threadId === threadId) && total > 0;
      });
    } catch (error) {
      out.error = error && (error.stack || error.message) || String(error);
    }
    return out;
  })()`
}

function summarizeError(raw) {
  const text = redactSecrets(String(raw || '')).replace(/\s+/g, ' ').trim()
  if (!text) return ''
  if (text.includes('missing_api_key')) return 'missing_api_key'
  if (text.includes('runtime_unhealthy')) return 'runtime_unhealthy'
  if (text.includes('go-runtime-default')) return 'go-runtime-default-readiness'
  return text.slice(0, 360)
}

function smokeChildEnv({ tempHome, userDataDir, chromiumTempDir }) {
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
    ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK: '1'
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

function providerRequestShapeEvidence(requests) {
  return requests.map((item, index) => ({
    index,
    requestPath: item.requestPath,
    bodyModel: item.bodyModel,
    initial: item.bodyContainsInitialPrompt === true,
    toolTimelinePrompt: item.bodyContainsToolTimelinePrompt === true,
    toolTimelineResult: item.bodyContainsToolTimelineFollowup === true,
    mimoPlanPrompt: item.bodyContainsMimoPlanPrompt === true,
    mimoPlanResult: item.bodyContainsMimoPlanResult === true,
    attachmentPrompt: item.bodyContainsAttachmentPrompt === true,
    attachmentText: item.bodyContainsAttachmentText === true,
    attachmentVirtualPath: item.bodyContainsAttachmentVirtualPath === true,
    attachmentEnvelope: item.bodyContainsAttachmentEnvelope === true,
    attachmentProtocol: item.bodyContainsAttachmentProtocol === true,
    approvalDenyPrompt: item.bodyContainsApprovalDenyPrompt === true,
    approvalAllowPrompt: item.bodyContainsApprovalAllowPrompt === true,
    userInputPrompt: item.bodyContainsUserInputPrompt === true,
    forkPrompt: item.bodyContainsForkPrompt === true,
    resumePrompt: item.bodyContainsResumePrompt === true
  }))
}

async function runActualPackagedSessionSoak() {
  const timeoutMs = Number(argValue(
    '--timeout-ms',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOAK_TIMEOUT_MS ||
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
      : 'isolated packaged soak mode may start one explicitly selected package instance'
  ))
  if (!artifactEvidence.ok || formalInstanceMode) {
    checks.push(actualCheck(
      'packaged-app-launch',
      'Packaged app launch',
      false,
      formalInstanceMode ? 'formal_instance_external_observation_required' : 'packaged app launch withheld because artifact authority did not pass',
      formalInstanceMode || artifactEvidence.blocked ? 'live_blocked' : 'skipped'
    ))
    for (const id of REQUIRED_ACTUAL_CHECK_IDS) {
      if (checks.some((check) => check.id === id)) continue
      checks.push(actualCheck(id, id, false, 'packaged launch evidence is unavailable', 'skipped'))
    }
    return {
      requested: true,
      passed: false,
      status: checks.some((check) => check.status === 'failed')
        ? 'failed'
        : checks.some((check) => check.status === 'live_blocked') ? 'live_blocked' : 'skipped',
      app: artifactEvidence,
      launch: {
        actualPackagedAppLaunched: false,
        formalInstanceMode,
        secondInstanceStarted: false,
        targetKey: target.key,
        packageSourceCommit: artifactEvidence.buildAuthority?.sourceCommit || '',
        executableSha256: artifactEvidence.executableSha256 || '',
        appAsarSha256: artifactEvidence.appAsarSha256 || '',
        runtimeServerSha256: artifactEvidence.runtimeServerSha256 || '',
        debugPortUsed: false,
        runtimePortUsed: false,
        temporaryUserData: false,
        syntheticCredentialUsed: false,
        providerNetworkCalled: false,
        localContractProviderUsed: false,
        launchError: formalInstanceMode ? 'formal_instance_external_observation_required' : artifactEvidence.message,
        stdoutTailHash: '',
        stderrTailHash: ''
      },
      renderer: null,
      provider: {
        localContractProviderUsed: false,
        externalProviderNetworkCalled: false,
        requestCount: 0,
        authorizationConfigured: false,
        requestShapes: []
      },
      redaction: {
        status: 'passed',
        secretMaterialFound: false,
        firstFinding: ''
      },
      checks
    }
  }
  let child = null
  let tempHome = ''
  let runtimeDataDir = ''
  let userDataDir = ''
  let renderer = null
  let launchError = ''
  let stdout = ''
  let stderr = ''
  let debugPort = 0
  let runtimePort = 0
  let contractProvider = null
  let isolatedLoginKeychain = null
  let approvalDenyFileAbsent = false
  let approvalAllowFileMatches = false
  let forkDiagnostic = null
  const providerId = 'xiaomi'
  const model = 'mimo-v2.5-pro'

  try {
    if (artifactEvidence.ok) {
      contractProvider = await startContractProvider()
      tempHome = mkdtempSync(join(tmpdir(), 'analytix-packaged-session-home-'))
      userDataDir = join(tempHome, '.analytix', 'packaged-session-user-data')
      runtimeDataDir = join(tempHome, '.analytix', 'packaged-session-runtime-data')
      const chromiumTempDir = join(tempHome, 'chromium-tmp')
      const workspace = join(tempHome, 'workspace')
      mkdirSync(dirname(userDataDir), { recursive: true, mode: 0o700 })
      mkdirSync(userDataDir, { mode: 0o700 })
      mkdirSync(chromiumTempDir, { mode: 0o700 })
      mkdirSync(workspace, { mode: 0o700 })
      if (target.platform === 'darwin') {
        isolatedLoginKeychain = await createIsolatedDarwinLoginKeychain(tempHome)
        const unlocked = await isolatedLoginKeychain.unlockForLaunch()
        if (!unlocked.ok || !unlocked.defaultKeychainBound) {
          throw new Error('isolated_login_keychain_unavailable')
        }
      }
      debugPort = await getFreePort()
      runtimePort = await getFreePort()
      while (runtimePort === debugPort) runtimePort = await getFreePort()
      const syntheticApiKey = `sk-packaged-session-soak-${sha256(`${Date.now()}:${runtimePort}`).slice(0, 24)}`
      child = spawn(resolveExecutablePath(appPath, target), [`--remote-debugging-port=${debugPort}`], {
        cwd: process.cwd(),
        env: smokeChildEnv({ tempHome, userDataDir, chromiumTempDir }),
        stdio: ['ignore', 'pipe', 'pipe']
      })
      child.stdout?.on('data', (chunk) => { stdout += String(chunk) })
      child.stderr?.on('data', (chunk) => { stderr += String(chunk) })
      try {
        renderer = await evaluateRendererWithRetries({
          debugPort,
          expression: buildRendererExpression({
            providerBaseUrl: contractProvider.url,
            providerId,
            model,
            runtimePort,
            runtimeDataDir,
            workspace,
            syntheticApiKey
          }),
          timeoutMs
        })
        renderer = renderer && typeof renderer === 'object' ? renderer : null
        if (renderer) {
          renderer.restartError = summarizeError(renderer.restartError)
          renderer.settingsPatchError = summarizeError(renderer.settingsPatchError)
          renderer.error = summarizeError(renderer.error)
        }
        if (args.has('--diagnose-fork') && renderer?.error?.includes('/fork returned 502') &&
            /^thr_[A-Za-z0-9_-]+$/.test(renderer.initialThreadId || '')) {
          try {
            const response = await fetch(
              `http://127.0.0.1:${runtimePort}/v1/threads/${renderer.initialThreadId}/fork`,
              {
                method: 'POST',
                headers: { 'content-type': 'application/json' },
                body: JSON.stringify({ relation: 'fork', title: 'Packaged Session Diagnostic Fork' }),
                signal: AbortSignal.timeout(5000)
              }
            )
            const body = await response.json()
            const { ThreadSchema } = await import('../packages/runtime/dist/contracts/threads.js')
            const parsed = ThreadSchema.strict().safeParse(body)
            forkDiagnostic = {
              status: response.status,
              rootKeys: Object.keys(body).sort(),
              schemaValid: parsed.success,
              issuePaths: parsed.success ? [] : parsed.error.issues.slice(0, 20).map((issue) =>
                `${issue.path.join('.')}:${issue.code}`)
            }
          } catch (error) {
            forkDiagnostic = { error: summarizeError(error) }
          }
        }
      } catch (error) {
        launchError = error instanceof Error ? error.message : String(error)
      }
    }
  } finally {
    await stopChild(child)
    await stopSmokeRuntimeOnPort(runtimePort, runtimeDataDir)
    if (contractProvider) await contractProvider.close()
    isolatedLoginKeychain?.dispose()
    if (tempHome) {
      const workspace = join(tempHome, 'workspace')
      approvalDenyFileAbsent = !existsSync(join(workspace, 'approval-deny.txt'))
      try {
        approvalAllowFileMatches = readFileSync(join(workspace, 'approval-allow.txt'), 'utf8') === 'allowed'
      } catch {
        approvalAllowFileMatches = false
      }
      rmSync(tempHome, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 })
    }
  }

  const providerRequests = contractProvider?.requests || []
  const providerInitialPromptSeen = providerRequests.some((item) => item.bodyContainsInitialPrompt)
  const providerForkPromptSeen = providerRequests.some((item) => item.bodyContainsForkPrompt)
  const providerResumePromptSeen = providerRequests.some((item) => item.bodyContainsResumePrompt)
  const providerToolTimelinePromptSeen = providerRequests.some((item) => item.bodyContainsToolTimelinePrompt)
  const providerToolTimelineFollowupSeen = providerRequests.some((item) => item.bodyContainsToolTimelineFollowup)
  const providerMimoPlanPromptSeen = providerRequests.some((item) =>
    item.bodyContainsMimoPlanPrompt &&
    item.requestPath === '/v1/chat/completions' &&
    item.bodyModel === 'mimo-v2.5-pro'
  )
  const providerMimoPlanFollowupSeen = providerRequests.some((item) =>
    item.bodyContainsMimoPlanPrompt &&
    item.bodyContainsMimoPlanResult &&
    item.requestPath === '/v1/chat/completions' &&
    item.bodyModel === 'mimo-v2.5-pro'
  )
  const providerAttachmentPromptSeen = providerRequests.some((item) => item.bodyContainsAttachmentPrompt)
  const providerAttachmentFallbackSeen = providerRequests.some((item) =>
    item.bodyContainsAttachmentPrompt &&
    item.bodyContainsAttachmentText &&
    item.bodyContainsAttachmentVirtualPath
  )
  const providerApprovalDenySeen = providerRequests.some((item) =>
    item.bodyContainsApprovalDenyPrompt &&
    item.bodyContainsApprovalDenyResult
  )
  const providerApprovalAllowSeen = providerRequests.some((item) =>
    item.bodyContainsApprovalAllowPrompt &&
    item.bodyContainsApprovalAllowResult
  )
  const providerUserInputSeen = providerRequests.some((item) =>
    item.bodyContainsUserInputPrompt &&
    item.bodyContainsUserInputAnswer
  )
  const providerHistorySeen = providerRequests.some((item) => item.bodyContainsInitialPromptAsHistory)
  checks.push(actualCheck(
    'packaged-app-launch',
    'Packaged app launch',
    Boolean(renderer),
    renderer ? 'packaged app launched and renderer debugger was reachable' : `packaged app launch did not produce renderer evidence: ${summarizeError(launchError) || 'unknown'}`,
    'failed'
  ))
  checks.push(actualCheck(
    'packaged-renderer-bridge',
    'Packaged renderer bridge',
    renderer?.apiPresent === true &&
      renderer?.title === 'Analytix' &&
      renderer?.legacyKun === false &&
      renderer?.legacyReasonix === false,
    '`window.analytix` must be present, product title must be Analytix, and legacy aliases absent'
  ))
  checks.push(actualCheck(
    'packaged-settings-bridge',
    'Packaged settings bridge',
    renderer?.settingsOk === true && renderer?.settingsRuntimeTopLevel === true,
    'settings bridge must return top-level runtime/provider settings'
  ))
  checks.push(actualCheck(
    'packaged-provider-profile-settings',
    'Packaged provider profile settings',
    renderer?.settingsProfilePatchAccepted === true && renderer?.settingsCompatPatchUsed !== true,
    renderer?.settingsCompatPatchUsed === true
      ? `packaged settings bridge only accepted compat-minimal patch: ${summarizeError(renderer.settingsPatchError) || 'provider profile patch was rejected'}`
      : 'settings bridge must accept provider.activeProviderId plus provider.providers[] profile patches'
  ))
  const restartBlocked = renderer && renderer.restartOk !== true && Boolean(renderer.restartError)
  checks.push(actualCheck(
    'packaged-runtime-restart',
    'Packaged runtime restart',
    renderer?.restartOk === true && renderer?.healthStatus === 200 && renderer?.healthService === 'analytix',
    restartBlocked
      ? `runtime restart blocked by packaged app/runtime state: ${summarizeError(renderer.restartError)}`
      : '`window.analytix.runtime.restartRuntime()` must complete and /health must be reachable',
    restartBlocked ? 'live_blocked' : 'failed'
  ))
  checks.push(actualCheck(
    'packaged-session-thread-create',
    'Packaged session thread create',
    renderer?.threadCreateOk === true,
    renderer?.error || 'packaged app must create a runtime thread through renderer bridge',
    renderer?.restartOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-turn-create',
    'Packaged session turn create',
    renderer?.turnCreateOk === true && providerInitialPromptSeen && providerRequests.length >= 1,
    renderer?.error || 'packaged app must create a turn and reach the local contract provider',
    renderer?.threadCreateOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-sse-replay',
    'Packaged session SSE replay',
    renderer?.sseReplayOk === true,
    renderer?.error || 'packaged app must replay completed assistant text, usage, and turn_completed for the current turn',
    renderer?.turnCreateOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-tool-timeline',
    'Packaged session tool timeline',
    renderer?.toolTurnOk === true &&
      renderer?.toolTimelineOk === true &&
      providerToolTimelinePromptSeen &&
      providerToolTimelineFollowupSeen,
    renderer?.error || 'packaged app must replay tool_call_ready, tool_call_started, tool_call_finished, usage, and final answer before operator gate',
    renderer?.sseReplayOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-mimo-plan',
    'Packaged MiMo plan mode',
    renderer?.mimoPlanThreadOk === true &&
      renderer?.mimoPlanTurnOk === true &&
      renderer?.mimoPlanReplayOk === true &&
      providerMimoPlanPromptSeen &&
      providerMimoPlanFollowupSeen,
    renderer?.error || 'packaged app must create a plan-mode turn with Xiaomi/MiMo provider settings, canonicalize the alias model in the provider body, execute create_plan, and replay the tool timeline',
    renderer?.toolTimelineOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-attachment-fallback',
    'Packaged session attachment fallback',
    renderer?.attachmentUploadOk === true &&
      renderer?.attachmentMetadataOk === true &&
    renderer?.attachmentTurnOk === true &&
      renderer?.attachmentSseReplayOk === true &&
      providerAttachmentPromptSeen &&
      providerAttachmentFallbackSeen,
    renderer?.error || 'packaged app must upload a public document attachment, send bounded text to the provider without a local path, and replay the turn',
    renderer?.mimoPlanReplayOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-approval-user-input',
    'Packaged session approval and user input',
    renderer?.approvalDenyTurnOk === true &&
      renderer?.approvalDenyRequestedOk === true &&
      renderer?.approvalDenyResolvedOk === true &&
      renderer?.approvalDenyNoExecuteOk === true &&
      renderer?.approvalAllowTurnOk === true &&
      renderer?.approvalAllowRequestedOk === true &&
      renderer?.approvalAllowResolvedOk === true &&
      renderer?.approvalAllowExecutedOk === true &&
      renderer?.userInputTurnOk === true &&
      renderer?.userInputRequestedOk === true &&
      renderer?.userInputResolvedOk === true &&
      renderer?.userInputReplayOk === true &&
      approvalDenyFileAbsent &&
      approvalAllowFileMatches &&
      providerApprovalDenySeen &&
      providerApprovalAllowSeen &&
      providerUserInputSeen,
    renderer?.error || 'packaged app must show approval/user_input gates only when tools call them, resolve through renderer bridge, and continue the turn',
    renderer?.attachmentSseReplayOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-fork',
    'Packaged session fork',
    renderer?.forkOk === true && renderer?.forkTurnOk === true && providerForkPromptSeen && providerHistorySeen,
    renderer?.error || 'packaged app must fork, continue, and preserve initial history in provider request',
    renderer?.turnCreateOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-resume',
    'Packaged session resume',
    renderer?.resumeOk === true && renderer?.resumeTurnOk === true && providerResumePromptSeen && providerHistorySeen,
    renderer?.error || 'packaged app must resume, continue, and preserve initial history in provider request',
    renderer?.turnCreateOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-thread-list',
    'Packaged session thread list',
    renderer?.threadListOk === true,
    renderer?.error || 'packaged app must list/search source and forked threads',
    renderer?.forkOk === true ? 'failed' : 'skipped'
  ))
  checks.push(actualCheck(
    'packaged-session-provider-redaction',
    'Packaged session provider redaction',
    providerRequests.every((item) => item.authorizationConfigured === true),
    'local contract provider must receive authorization without recording secret values'
  ))

  const rendererEvidence = renderer
    ? {
        hrefHash: renderer.href ? sha256(renderer.href) : '',
        title: renderer.title || '',
        apiPresent: renderer.apiPresent === true,
        domains: Array.isArray(renderer.domains) ? renderer.domains : [],
        legacyKun: renderer.legacyKun === true,
        legacyReasonix: renderer.legacyReasonix === true,
        settingsOk: renderer.settingsOk === true,
        settingsRuntimeTopLevel: renderer.settingsRuntimeTopLevel === true,
        settingsProfilePatchAccepted: renderer.settingsProfilePatchAccepted === true,
        settingsCompatPatchUsed: renderer.settingsCompatPatchUsed === true,
        settingsPatchMode: renderer.settingsPatchMode || '',
        settingsPatchError: renderer.settingsPatchError || '',
        restartOk: renderer.restartOk === true,
        restartError: renderer.restartError || '',
        healthStatus: Number(renderer.healthStatus || 0),
        healthService: renderer.healthService || '',
        threadCreateOk: renderer.threadCreateOk === true,
        turnCreateOk: renderer.turnCreateOk === true,
        sseReplayOk: renderer.sseReplayOk === true,
        initialReplay: renderer.initialReplay || null,
        toolTurnOk: renderer.toolTurnOk === true,
        toolTimelineOk: renderer.toolTimelineOk === true,
        toolReplay: renderer.toolReplay || null,
        mimoPlanThreadOk: renderer.mimoPlanThreadOk === true,
        mimoPlanTurnOk: renderer.mimoPlanTurnOk === true,
        mimoPlanReplayOk: renderer.mimoPlanReplayOk === true,
        mimoPlanReplay: renderer.mimoPlanReplay || null,
        attachmentUploadOk: renderer.attachmentUploadOk === true,
        attachmentTurnOk: renderer.attachmentTurnOk === true,
        attachmentSseReplayOk: renderer.attachmentSseReplayOk === true,
        attachmentMetadataOk: renderer.attachmentMetadataOk === true,
        approvalDenyTurnOk: renderer.approvalDenyTurnOk === true,
        approvalDenyGate: renderer.approvalDenyGate && {
          ok: renderer.approvalDenyGate.ok === true,
          eventCount: renderer.approvalDenyGate.eventCount,
          errorCount: renderer.approvalDenyGate.errorCount,
          eventKinds: renderer.approvalDenyGate.eventKinds,
          turnEventKinds: renderer.approvalDenyGate.turnEventKinds
        },
        approvalDenyRequestedOk: renderer.approvalDenyRequestedOk === true,
        approvalDenyResolvedOk: renderer.approvalDenyResolvedOk === true,
        approvalDenyNoExecuteOk: renderer.approvalDenyNoExecuteOk === true,
        approvalAllowTurnOk: renderer.approvalAllowTurnOk === true,
        approvalAllowGate: renderer.approvalAllowGate && {
          ok: renderer.approvalAllowGate.ok === true,
          eventCount: renderer.approvalAllowGate.eventCount,
          errorCount: renderer.approvalAllowGate.errorCount,
          eventKinds: renderer.approvalAllowGate.eventKinds,
          turnEventKinds: renderer.approvalAllowGate.turnEventKinds
        },
        approvalAllowRequestedOk: renderer.approvalAllowRequestedOk === true,
        approvalAllowResolvedOk: renderer.approvalAllowResolvedOk === true,
        approvalAllowExecutedOk: renderer.approvalAllowExecutedOk === true,
        userInputTurnOk: renderer.userInputTurnOk === true,
        userInputGate: renderer.userInputGate && {
          ok: renderer.userInputGate.ok === true,
          eventCount: renderer.userInputGate.eventCount,
          errorCount: renderer.userInputGate.errorCount,
          eventKinds: renderer.userInputGate.eventKinds,
          turnEventKinds: renderer.userInputGate.turnEventKinds
        },
        userInputRequestedOk: renderer.userInputRequestedOk === true,
        userInputResolvedOk: renderer.userInputResolvedOk === true,
        userInputReplayOk: renderer.userInputReplayOk === true,
        forkOk: renderer.forkOk === true,
        forkTurnOk: renderer.forkTurnOk === true,
        resumeOk: renderer.resumeOk === true,
        resumeTurnOk: renderer.resumeTurnOk === true,
        threadListOk: renderer.threadListOk === true,
        usageOk: renderer.usageOk === true,
        initialThreadIdHash: renderer.initialThreadId ? sha256(renderer.initialThreadId) : '',
        initialTurnIdHash: renderer.initialTurnId ? sha256(renderer.initialTurnId) : '',
        toolTurnIdHash: renderer.toolTurnId ? sha256(renderer.toolTurnId) : '',
        forkThreadIdHash: renderer.forkThreadId ? sha256(renderer.forkThreadId) : '',
        resumeThreadIdHash: renderer.resumeThreadId ? sha256(renderer.resumeThreadId) : '',
        error: renderer.error || ''
      }
    : null
  const secretFinding = firstSecretFinding({
    checks,
    renderer: rendererEvidence,
    forkDiagnostic,
    launchError: summarizeError(launchError)
  })
  if (secretFinding) {
    checks.push(actualCheck('packaged-session-redaction', 'Packaged session redaction', false, `secret-like material found at ${secretFinding}`))
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
      formalInstanceMode: false,
      secondInstanceStarted: Boolean(child),
      targetKey: target.key,
      packageSourceCommit: artifactEvidence.buildAuthority?.sourceCommit || '',
      executableSha256: artifactEvidence.executableSha256 || '',
      appAsarSha256: artifactEvidence.appAsarSha256 || '',
      runtimeServerSha256: artifactEvidence.runtimeServerSha256 || '',
      debugPortUsed: debugPort > 0,
      runtimePortUsed: runtimePort > 0,
      temporaryUserData: Boolean(tempHome),
      syntheticCredentialUsed: true,
      providerNetworkCalled: false,
      localContractProviderUsed: Boolean(contractProvider),
      launchError: summarizeError(launchError),
      stdoutTailHash: stdout ? sha256(redactSecrets(stdout.slice(-4000))) : '',
      stderrTailHash: stderr ? sha256(redactSecrets(stderr.slice(-4000))) : ''
    },
    renderer: rendererEvidence,
    forkDiagnostic,
    provider: {
      localContractProviderUsed: Boolean(contractProvider),
      externalProviderNetworkCalled: false,
      requestCount: providerRequests.length,
      authorizationConfigured: providerRequests.every((item) => item.authorizationConfigured === true),
      requestShapes: providerRequestShapeEvidence(providerRequests),
      initialPromptSeen: providerInitialPromptSeen,
      toolTimelinePromptSeen: providerToolTimelinePromptSeen,
      toolTimelineFollowupSeen: providerToolTimelineFollowupSeen,
      mimoPlanPromptSeen: providerMimoPlanPromptSeen,
      mimoPlanFollowupSeen: providerMimoPlanFollowupSeen,
      attachmentPromptSeen: providerAttachmentPromptSeen,
      attachmentFallbackSeen: providerAttachmentFallbackSeen,
      approvalDenySeen: providerApprovalDenySeen,
      approvalAllowSeen: providerApprovalAllowSeen,
      userInputSeen: providerUserInputSeen,
      forkPromptSeen: providerForkPromptSeen,
      resumePromptSeen: providerResumePromptSeen,
      historyCarriedToChildOrResume: providerHistorySeen
    },
    isolatedWorkspace: {
      approvalDenyFileAbsent,
      approvalAllowFileMatches
    },
    redaction: {
      status: secretFinding ? 'failed' : 'passed',
      secretMaterialFound: Boolean(secretFinding),
      firstFinding: secretFinding
    },
    checks
  }
}

function exactGoTestPassed(output, testName) {
  let passed = false
  let failed = false
  let skipped = false
  for (const line of String(output || '').split('\n')) {
    let event
    try {
      event = JSON.parse(line)
    } catch {
      continue
    }
    if (event?.Test !== testName) continue
    if (event.Action === 'pass') passed = true
    if (event.Action === 'fail') failed = true
    if (event.Action === 'skip') skipped = true
  }
  return passed && !failed && !skipped
}

function spawnCollected(command, commandArgs, options = {}) {
  return new Promise((resolveResult) => {
    const stdout = []
    const stderr = []
    let stdoutBytes = 0
    let stderrBytes = 0
    let error
    let overflow = false
    const maxBuffer = options.maxBuffer || (96 * 1024 * 1024)
    const child = spawn(command, commandArgs, {
      cwd: options.cwd,
      env: options.env,
      stdio: ['ignore', 'pipe', 'pipe']
    })
    const collect = (chunks, streamName) => (chunk) => {
      const bytes = Buffer.from(chunk)
      if (streamName === 'stdout') stdoutBytes += bytes.length
      else stderrBytes += bytes.length
      if (stdoutBytes > maxBuffer || stderrBytes > maxBuffer) {
        if (!overflow) {
          overflow = true
          error = new Error(`${streamName} exceeded maxBuffer`)
          child.kill('SIGKILL')
        }
        return
      }
      chunks.push(bytes)
    }
    child.stdout.on('data', collect(stdout, 'stdout'))
    child.stderr.on('data', collect(stderr, 'stderr'))
    child.once('error', (childError) => {
      error = childError
    })
    child.once('close', (status, signal) => {
      resolveResult({
        status,
        signal,
        error,
        stdout: Buffer.concat(stdout).toString('utf8'),
        stderr: Buffer.concat(stderr).toString('utf8')
      })
    })
  })
}

async function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-packaged-session-soak] ${item.id}`)
  if (skipCommands) {
    return {
      id: item.id,
      status: 'skipped',
      command: commandText(item),
      durationMs: 0
    }
  }
  if (Array.isArray(item.tests) && item.compiledGoTestPackage) {
    const subchecks = []
    const binaryDir = mkdtempSync(join(tmpdir(), 'analytix-packaged-session-soak-binary-'))
    const binaryPath = join(binaryDir, process.platform === 'win32' ? 'runtime-go.test.exe' : 'runtime-go.test')
    let compileDurationMs = 0
    try {
      const compileStarted = Date.now()
      const compiled = await spawnCollected(goCommand, ['test', '-c', '-o', binaryPath, '.'], {
        cwd: item.cwd,
        env: process.env,
        maxBuffer: 96 * 1024 * 1024
      })
      compileDurationMs = Date.now() - compileStarted
      const compileExitStatus = compiled.status ?? (compiled.signal || compiled.error ? 1 : 0)
      if (compileExitStatus !== 0 || compiled.error) {
        return {
          id: item.id,
          status: 'failed',
          command: commandText(item),
          durationMs: Date.now() - started,
          compileDurationMs,
          exitStatus: compileExitStatus,
          reason: compiled.error?.message || `test binary compilation exited with status ${compileExitStatus}`,
          subchecks
        }
      }
      for (const testName of item.tests) {
        const shardStarted = Date.now()
        const shardArgs = [
          'tool',
          'test2json',
          '-p',
          item.compiledGoTestPackage,
          binaryPath,
          '-test.v=test2json',
          '-test.run',
          `^${testName}$`,
          '-test.count=1'
        ]
        const result = await spawnCollected(goCommand, shardArgs, {
          cwd: item.cwd,
          env: process.env,
          maxBuffer: 96 * 1024 * 1024
        })
        const exitStatus = result.status ?? (result.signal || result.error ? 1 : 0)
        const exactPass = exactGoTestPassed(result.stdout, testName)
        const shardPassed = exitStatus === 0 && !result.error && exactPass
        if (!shardPassed && !jsonOutput) {
          if (result.stdout) process.stdout.write(result.stdout)
          if (result.stderr) process.stderr.write(result.stderr)
        }
        subchecks.push({
          id: testName,
          status: shardPassed ? 'passed' : 'failed',
          command: `${goCommand} tool test2json -p ${item.compiledGoTestPackage} <task-owned-test-binary> -test.v=test2json -test.run ^${testName}$ -test.count=1`,
          durationMs: Date.now() - shardStarted,
          exitStatus,
          reason: result.error
            ? result.error.message
            : exitStatus !== 0
              ? `command exited with status ${exitStatus}`
              : exactPass
                ? ''
                : 'expected Go test did not report an exact pass',
          ...(result.signal ? { signal: result.signal } : {})
        })
        if (!shardPassed) break
      }
    } finally {
      rmSync(binaryDir, { recursive: true, force: true })
    }
    const firstFailure = subchecks.find((check) => check.status !== 'passed')
    return {
      id: item.id,
      status: firstFailure ? 'failed' : 'passed',
      command: commandText(item),
      durationMs: Date.now() - started,
      compileDurationMs,
      exitStatus: firstFailure?.exitStatus || 0,
      reason: firstFailure ? `${firstFailure.id}: ${firstFailure.reason}` : '',
      subchecks
    }
  }
  const result = await spawnCollected(item.command, item.args, {
    cwd: item.cwd,
    env: process.env,
    maxBuffer: 96 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  if (!jsonOutput) {
    if (result.stdout) process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  }
  // Keep --json output machine-readable even when a child validation command
  // emits npm banners or test logs before failing.
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
  // Keep the nested TypeScript validation owner out of the Go test window.
  // Overlap makes a standalone soak faster but can starve the nested Vitest
  // process when this contract itself runs inside the full root suite.
  const checks = []
  if (!actualOnly) {
    const goCheck = await runCheck(commands[0])
    checks.push(goCheck)
    if (goCheck.status === 'passed') {
      checks.push(await runCheck(commands[1]))
    }
  }
  const failed = checks.some((check) => check.status !== 'passed')

  const actual = actualPackagedSoak && !skipCommands ? await runActualPackagedSessionSoak() : {
    requested: actualPackagedSoak,
    passed: false,
    status: actualPackagedSoak ? 'skipped' : 'not-requested',
    launch: {
      actualPackagedAppLaunched: false,
      formalInstanceMode,
      secondInstanceStarted: false,
      temporaryUserData: false,
      syntheticCredentialUsed: false,
      providerNetworkCalled: false
    },
    provider: {
      externalProviderNetworkCalled: false,
      requestCount: 0
    },
    checks: []
  }
  const actualRequiredAndFailed = actualPackagedSoak && actual.passed !== true
  const passed = !skipCommands && !actualOnly && !failed && checks.every((item) => item.status === 'passed') && !actualRequiredAndFailed
  const deterministicChecksFailed = failed || checks.some((item) => item.status === 'failed')
  const status = actualOnly && actual.passed === true
    ? 'partial'
    : passed
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
  const goDurableSessionCovered = checks.find((item) => item.id === 'go-session-durable-contract')?.status === 'passed'
  const typeScriptRetiredBackendSessionCovered = checks.find((item) => item.id === 'typescript-retired-backend-session-contract')?.status === 'passed'
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    generatedAt: new Date().toISOString(),
    sourceCommit: actualPackagedSoak ? packageSourceCommit : currentGitCommit(),
    testHarnessCommit: currentGitCommit(),
    status,
    passed,
    soak: {
      actualOnly,
      deterministicContractOnly: actualPackagedSoak !== true,
      formalInstanceMode,
      secondInstanceStarted: actual.launch?.secondInstanceStarted === true,
      actualPackagedAppLaunched: actual.launch?.actualPackagedAppLaunched === true,
      actualPackagedAppEvidenceRequiredForFinalGate: actualPackagedSoak !== true ||
        actual.passed !== true || !packageReleaseAuthorityAccepted,
      actualPackagedSessionSoakRequested: actualPackagedSoak,
      actualPackagedSessionSoakPassed: actual.passed === true,
      packageClassification: actual.app?.buildAuthority?.classification || '',
      packagePublishable: actual.app?.buildAuthority?.publishable === true,
      packageReleaseEligible: actual.app?.buildAuthority?.releaseEligible === true,
      packagePublicationReceiptIssued: actual.app?.buildAuthority?.publicationReceiptIssued === true,
      syntheticCredentialUsed: actual.launch?.syntheticCredentialUsed === true,
      providerNetworkCalled: actual.launch?.providerNetworkCalled === true,
      localContractProviderUsed: actual.provider?.localContractProviderUsed === true,
      goDurableSessionCovered,
      typeScriptRetiredBackendSessionCovered,
      resumeForkPairingCovered: goDurableSessionCovered && (actualPackagedSoak !== true || (actual.renderer?.forkOk === true && actual.renderer?.resumeOk === true)),
      sseReplayCovered: goDurableSessionCovered && (actualPackagedSoak !== true || actual.renderer?.sseReplayOk === true),
      mimoPlanCovered: actualPackagedSoak !== true ||
        (actual.renderer?.mimoPlanThreadOk === true &&
          actual.renderer?.mimoPlanTurnOk === true &&
          actual.renderer?.mimoPlanReplayOk === true &&
          actual.provider?.mimoPlanPromptSeen === true &&
          actual.provider?.mimoPlanFollowupSeen === true),
      attachmentFallbackCovered: actualPackagedSoak !== true ||
        (actual.renderer?.attachmentUploadOk === true &&
          actual.renderer?.attachmentMetadataOk === true &&
          actual.renderer?.attachmentTurnOk === true &&
          actual.renderer?.attachmentSseReplayOk === true &&
          actual.provider?.attachmentFallbackSeen === true),
      approvalUserInputCovered: actualPackagedSoak !== true ||
        (actual.renderer?.approvalDenyRequestedOk === true &&
          actual.renderer?.approvalDenyResolvedOk === true &&
          actual.renderer?.approvalAllowRequestedOk === true &&
          actual.renderer?.approvalAllowResolvedOk === true &&
          actual.renderer?.userInputRequestedOk === true &&
          actual.renderer?.userInputResolvedOk === true &&
          actual.provider?.approvalDenySeen === true &&
          actual.provider?.approvalAllowSeen === true &&
          actual.provider?.userInputSeen === true),
      threadListSearchCovered: actualPackagedSoak !== true || actual.renderer?.threadListOk === true,
      usageCovered: actualPackagedSoak !== true || actual.renderer?.usageOk === true
    },
    actualPackagedSessionSoak: actual,
    checks
  }

  writeEvidenceReport(report)

  if (jsonOutput) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    console.log(`${status.toUpperCase()} ${report.id}`)
    for (const item of checks.filter((check) => check.status !== 'passed')) {
      console.log(`FAILED ${item.id}: ${item.reason}`)
    }
    if (actualPackagedSoak && actual?.checks) {
      for (const item of actual.checks.filter((check) => check.status !== 'passed')) {
        console.log(`FAILED ${item.id}: ${item.message || item.status}`)
      }
    }
  }

  if (!reportOnly && !passed) process.exitCode = 1
}

main().catch((error) => {
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    generatedAt: new Date().toISOString(),
    status: 'failed',
    passed: false,
    soak: {
      deterministicContractOnly: actualPackagedSoak !== true,
      actualPackagedAppLaunched: false,
      actualPackagedAppEvidenceRequiredForFinalGate: true,
      actualPackagedSessionSoakRequested: actualPackagedSoak,
      actualPackagedSessionSoakPassed: false
    },
    checks: [{
      id: 'runtime-go-packaged-session-soak',
      status: 'failed',
      command: 'runtime-go-packaged-session-soak',
      durationMs: 0,
      exitStatus: 1,
      reason: error instanceof Error ? redactSecrets(error.message) : redactSecrets(String(error))
    }]
  }
  writeEvidenceReport(report)
  if (jsonOutput) console.log(JSON.stringify(report, null, 2))
  else console.error(`FAILED ${report.id}: ${report.checks[0].reason}`)
  process.exitCode = 1
})
