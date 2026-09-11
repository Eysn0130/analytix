import { createHash } from 'node:crypto'
import { lstatSync, readFileSync } from 'node:fs'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'
import nativeManifestBytes from '../../../scripts/native-components.json?raw'
import { inspectNativePayload, type NativePayloadFormat } from './native-payload-identity'
import { nativeTargetKey } from './native-runtime-paths'

type RuntimeTarget = {
  platform: string
  arch: string
  format: NativePayloadFormat
  triple: string
}

const TARGETS: Record<string, RuntimeTarget> = {
  'darwin-arm64': { platform: 'darwin', arch: 'arm64', format: 'mach-o', triple: 'aarch64-apple-darwin' },
  'darwin-x64': { platform: 'darwin', arch: 'x64', format: 'mach-o', triple: 'x86_64-apple-darwin' },
  'linux-x64': { platform: 'linux', arch: 'x64', format: 'elf', triple: 'x86_64-unknown-linux-gnu' },
  'win32-x64': { platform: 'win32', arch: 'x64', format: 'pe', triple: 'x86_64-pc-windows-msvc' }
}

const manifest = JSON.parse(nativeManifestBytes) as {
  schemaVersion: number
  receiptSchemaVersion: number
  components: Array<{
    id: string
    binaryName: string
    packagePath: string
    executionProbe: { policySha256: string }
  }>
}
const manifestSha256 = createHash('sha256').update(nativeManifestBytes, 'utf8').digest('hex')
const FROZEN_MANIFEST_SHA256 = '7265c3508f16731b5f8dbb1e08f7d645a2e8ab6d7bf4a35cf64c887e0c129aa4'
const RECEIPT_FILE_NAME = 'analytix-native-components-receipt.json'
const RECEIPT_KEYS = [
  'schemaVersion',
  'manifestSha256',
  'sourceSetSha256',
  'buildEnvironmentSha256',
  'toolchain',
  'targetKey',
  'targetTriple',
  'platform',
  'arch',
  'executionAuthority',
  'publicationAuthority',
  'components'
]
const EXECUTION_AUTHORITY_KEYS = [
  'schemaVersion',
  'trustClass',
  'protocol',
  'targetKey',
  'binarySha256',
  'binarySize',
  'sourceSetSha256',
  'buildEnvironmentSha256',
  'goToolchainKey',
  'goExecutableSha256'
]
const EXECUTION_PROBE_KEYS = [
  'kind',
  'schema_version',
  'status',
  'component_id',
  'request_nonce',
  'executable_sha256',
  'executable_size',
  'manifest_sha256',
  'policy_sha256',
  'authority_sha256',
  'host_platform',
  'host_arch',
  'loaded_image_bound',
  'working_directory_bound',
  'guardian_authenticated',
  'process_tree_empty'
]
const PUBLICATION_AUTHORITY_KEYS = [
  'schemaVersion',
  'trustClass',
  'protocol',
  'targetKey',
  'cargoExecutionId',
  'cargoExecutionReceiptSha256',
  'publicationBindingSha256'
]
const COMPONENT_KEYS = [
  'id',
  'binaryName',
  'packagePath',
  'sourceDigest',
  'cargoLockSha256',
  'buildEnvironmentSha256',
  'rawBuildSha256',
  'rawBuildSize',
  'stagedImageSha256',
  'stagedImageSize',
  'payloadSha256',
  'payloadSize',
  'format',
  'arch',
  'executionProbe'
]

function exactKeys(value: unknown, expected: string[]): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value)
  if (actual.length !== expected.length) return false
  const expectedKeys = new Set(expected)
  return actual.every((key) => expectedKeys.has(key))
}

function isSha256(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
}

function requireCurrentAuthorityTarget(target: RuntimeTarget, key: string): void {
  if (target.platform !== 'darwin') {
    throw new Error(`Native execution authority is unavailable for target: ${key}`)
  }
}

function assertRegularPathWithinRoot(path: string, root: string, kind: 'file' | 'directory'): void {
  const resolvedRoot = resolve(root)
  const resolvedPath = resolve(path)
  const relativePath = relative(resolvedRoot, resolvedPath)
  if (relativePath === '..' || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error(`Native runtime path escapes its package root: ${path}`)
  }
  const rootStat = lstatSync(resolvedRoot)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) throw new Error(`Native package root is not trusted: ${root}`)
  let current = resolvedRoot
  for (const part of relativePath ? relativePath.split(sep) : []) {
    current = join(current, part)
    const stat = lstatSync(current)
    if (stat.isSymbolicLink()) throw new Error(`Native runtime path traverses a symbolic link: ${current}`)
  }
  const stat = lstatSync(resolvedPath)
  if (kind === 'file' ? !stat.isFile() : !stat.isDirectory()) {
    throw new Error(`Native runtime ${kind} is invalid: ${path}`)
  }
}

function parseCanonicalReceipt(path: string): Record<string, unknown> {
  const text = readFileSync(path, 'utf8')
  const value = JSON.parse(text) as unknown
  if (`${JSON.stringify(value, null, 2)}\n` !== text) {
    throw new Error('Native component receipt is not in its canonical duplicate-free encoding')
  }
  if (!exactKeys(value, RECEIPT_KEYS)) throw new Error('Native component receipt schema is invalid')
  return value
}

function validExecutionProbe(
  value: unknown,
  expected: { id: string; executionProbe: { policySha256: string } },
  recorded: Record<string, unknown>,
  authority: Record<string, unknown>
): boolean {
  if (!exactKeys(value, EXECUTION_PROBE_KEYS)) return false
  const authorityArch = authority.targetKey === 'darwin-arm64' ? 'arm64' : 'x64'
  return value.kind === 'analytix_native_build_probe_receipt' && value.schema_version === 1 &&
    value.status === 'passed' && value.component_id === expected.id && isSha256(value.request_nonce) &&
    value.executable_sha256 === recorded.stagedImageSha256 && value.executable_size === recorded.stagedImageSize &&
    value.manifest_sha256 === manifestSha256 && value.policy_sha256 === expected.executionProbe.policySha256 &&
    value.authority_sha256 === authority.binarySha256 && value.host_platform === 'darwin' &&
    value.host_arch === authorityArch && value.loaded_image_bound === true &&
    value.working_directory_bound === true && value.guardian_authenticated === true &&
    value.process_tree_empty === true
}

export function verifyRuntimeNativeComponent(options: {
  root: string
  packaged: boolean
  platform: string
  arch: string
  componentId: string
  hostExecutable?: string
}): string {
  if (
    manifestSha256 !== FROZEN_MANIFEST_SHA256 ||
    manifest.schemaVersion !== 4 ||
    manifest.receiptSchemaVersion !== 6 ||
    manifest.components.length !== 4
  ) {
    throw new Error('Bundled native component registry is invalid')
  }
  const key = nativeTargetKey(options.platform, options.arch)
  const target = key ? TARGETS[key] : undefined
  if (!key || !target) throw new Error(`Unsupported native runtime target: ${options.platform}/${options.arch}`)
  requireCurrentAuthorityTarget(target, key)
  throw new Error('Native component execution is owned by the Go runtime and release authority is unavailable')
}
