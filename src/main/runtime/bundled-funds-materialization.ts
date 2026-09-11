import { spawn } from 'node:child_process'
import { createHash, createPublicKey, randomBytes, verify } from 'node:crypto'
import { realpathSync } from 'node:fs'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import { z } from 'zod'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'

const READY_PREFIX = 'ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 '
const READY_PURPOSE = 'analytix.bundled-funds-materialization-ready/v1'
const RECEIPT_PURPOSE = 'analytix.bundled-plugin-materialization-receipt/v1'
const INDEX_PURPOSE = 'analytix.bundled-plugin-materialization-index/v1'
const RECEIPT_SIGNATURE_DOMAIN = Buffer.from('analytix.bundled-plugin-materialization-receipt/signature/v1\0')
const RECEIPT_ID_DOMAIN = Buffer.from('analytix.bundled-plugin-materialization-receipt/id/v1\0')
const INDEX_DIGEST_DOMAIN = Buffer.from('analytix.bundled-plugin-materialization-index/digest/v1\0')
const READY_BINDING_DOMAIN = Buffer.from('analytix.bundled-funds-materialization-ready/binding/v1\0')
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')
const MAX_COMMAND_OUTPUT_BYTES = 512 * 1024
const COMMAND_TIMEOUT_MS = 120_000

const SHA256 = z.string().regex(/^[a-f0-9]{64}$/u)
const ExactTime = z.string().refine((value) => value.endsWith('Z') && Number.isFinite(Date.parse(value)))
const PackageVersion = z.string().max(128).refine(validSemanticVersionV1)
const ActiveRelativePath = z.string().min(1).max(512)
const Target = z.object({
  platform: z.enum(['darwin', 'windows', 'linux']),
  arch: z.enum(['arm64', 'amd64'])
}).strict()
const Receipt = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal(RECEIPT_PURPOSE),
  receiptId: SHA256,
  intentId: SHA256,
  packageAuthoritySha256: SHA256,
  target: Target,
  pluginName: z.literal('analytix-fund-analysis'),
  pluginVersion: PackageVersion,
  generationId: SHA256,
  activeRelativePath: ActiveRelativePath,
  sourceTreeSha256: SHA256,
  sourceTreeFileCount: z.number().int().positive().max(100_000),
  manifestSha256: SHA256,
  entrypointSha256: SHA256,
  factToolsEnabled: z.literal(false),
  issuedAt: ExactTime,
  authorityAlgorithm: z.literal('Ed25519'),
  authorityKeyId: SHA256,
  authorityPublicKey: z.string().min(1).max(128),
  authoritySignature: z.string().min(1).max(256)
}).strict()
const Index = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal(INDEX_PURPOSE),
  indexDigest: SHA256,
  pluginName: z.literal('analytix-fund-analysis'),
  pluginVersion: PackageVersion,
  generationId: SHA256,
  activeRelativePath: ActiveRelativePath,
  receiptId: SHA256,
  receiptSha256: SHA256,
  intentId: SHA256,
  sourceTreeSha256: SHA256,
  sourceTreeFileCount: z.number().int().positive().max(100_000),
  discoverableGenerationCount: z.literal(1),
  factToolsEnabled: z.literal(false),
  committedAt: ExactTime
}).strict()
const PackageAuthority = z.object({
  fileSha256: SHA256,
  authorityDigest: SHA256,
  classification: z.enum([
    'development_clean_non_publishable',
    'development_dirty_non_publishable',
    'controlled_release_clean_candidate_non_publishable',
    'controlled_release_dirty_non_publishable'
  ]),
  dispositionKind: z.enum(['development_non_publishable', 'controlled_release_receipt']),
  platformAnchor: z.enum(['macos_developer_id_resource_seal', 'macos_nonpublishable_resource_seal'])
}).strict()
const RuntimeIdentity = z.object({
  payloadSha256: SHA256,
  payloadBytes: z.number().int().positive(),
  format: z.enum(['mach-o', 'pe', 'elf']),
  arch: z.enum(['arm64', 'x64'])
}).strict()
const Ready = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal(READY_PURPOSE),
  invocationId: SHA256,
  configurationBindingDigest: SHA256,
  packageAuthority: PackageAuthority,
  runtimeIdentity: RuntimeIdentity,
  pluginName: z.literal('analytix-fund-analysis'),
  pluginVersion: PackageVersion,
  activePluginRoot: z.string().min(1),
  receipt: Receipt,
  index: Index,
  publishable: z.literal(false),
  factToolsEnabled: z.literal(false),
  completedAt: ExactTime
}).strict()

export type BundledFundsMaterializationBindingV1 = z.infer<typeof Ready>

let currentRuntimeBindingV1: BundledFundsMaterializationBindingV1 | null = null

export function bindBundledFundsMaterializationToCurrentRuntimeV1(
  binding: BundledFundsMaterializationBindingV1
): void {
  currentRuntimeBindingV1 = deepFreezeBindingV1(Ready.parse(binding))
}

export function clearBundledFundsMaterializationCurrentRuntimeV1(): void {
  currentRuntimeBindingV1 = null
}

export function currentBundledFundsMaterializationBindingV1(): BundledFundsMaterializationBindingV1 | null {
  return currentRuntimeBindingV1
}

export type BundledFundsMaterializationCommandResultV1 = Readonly<{
  exitCode: number | null
  signal: NodeJS.Signals | null
  stdout: Buffer
  stderr: Buffer
  timedOut: boolean
  overflow: boolean
}>

export type BundledFundsMaterializationCommandRunnerV1 = (
  command: string,
  args: readonly string[],
  options: Readonly<{ cwd?: string }>
) => Promise<BundledFundsMaterializationCommandResultV1>

export async function materializeBundledFundsBeforeRuntimeV1(options: Readonly<{
  appIsPackaged: boolean
  launchTarget: Readonly<{
    command: string
    argsPrefix: string[]
    cwd?: string
    mode: 'bundled-binary' | 'go-run-source'
  }>
  dataDir: string
  runner?: BundledFundsMaterializationCommandRunnerV1
}>): Promise<BundledFundsMaterializationBindingV1 | null> {
  if (!options.appIsPackaged) return null
  if (options.launchTarget.mode !== 'bundled-binary') {
    throw new Error('Packaged funds materialization requires the bundled runtime-server.')
  }
  const invocationId = randomBytes(32).toString('hex')
  const result = await (options.runner ?? runCommandV1)(
    options.launchTarget.command,
    [
      ...options.launchTarget.argsPrefix,
      'bundled-plugin',
      'materialize-funds-v1',
      '--data-dir',
      options.dataDir,
      '--invocation-id',
      invocationId
    ],
    { cwd: options.launchTarget.cwd }
  )
  try {
    if (result.exitCode !== 0 || result.signal !== null || result.timedOut || result.overflow || result.stderr.length !== 0) {
      throw new Error('Packaged funds materialization command failed.')
    }
    return verifyBundledFundsMaterializationOutputV1(result.stdout, invocationId, options.dataDir)
  } finally {
    result.stdout.fill(0)
    result.stderr.fill(0)
  }
}

export function verifyBundledFundsMaterializationOutputV1(
  stdout: Buffer,
  invocationId: string,
  dataDir: string
): BundledFundsMaterializationBindingV1 {
  if (!SHA256.safeParse(invocationId).success || stdout.length === 0 || stdout.length > MAX_COMMAND_OUTPUT_BYTES) {
    throw new Error('Packaged funds materialization ready payload is invalid.')
  }
  const line = stdout.toString('utf8')
  if (!line.startsWith(READY_PREFIX) || !line.endsWith('\n') || line.indexOf('\n') !== line.length - 1) {
    throw new Error('Packaged funds materialization ready marker is invalid.')
  }
  let raw: unknown
  try {
    raw = parseStrictJsonObject(Buffer.from(line.slice(READY_PREFIX.length, -1), 'utf8'), {
      maxBytes: MAX_COMMAND_OUTPUT_BYTES,
      maxDepth: 12,
      maxTokens: 512,
      maxStringBytes: 8 * 1024,
      maxNumberBytes: 32
    })
  } catch {
    throw new Error('Packaged funds materialization ready JSON is invalid.')
  }
  const parsed = Ready.safeParse(raw)
  if (!parsed.success) throw new Error('Packaged funds materialization ready schema is invalid.')
  const ready = parsed.data
  const runtimeHome = basename(dataDir) === 'data' ? dirname(dataDir) : dataDir
  const expectedRoot = resolve(runtimeHome, ready.index.activeRelativePath)
  if (!isAbsolute(dataDir) || resolve(dataDir) !== dataDir || ready.invocationId !== invocationId ||
      !isAbsolute(ready.activePluginRoot) || resolve(ready.activePluginRoot) !== ready.activePluginRoot ||
      ready.activePluginRoot !== expectedRoot || realpathSync(ready.activePluginRoot) !== ready.activePluginRoot ||
      !receiptAndIndexMatchV1(ready) || !verifyReceiptV1(ready.receipt) ||
      ready.configurationBindingDigest !== readyBindingDigestV1(ready)) {
    throw new Error('Packaged funds materialization current-run binding is invalid.')
  }
  return ready
}

function receiptAndIndexMatchV1(ready: BundledFundsMaterializationBindingV1): boolean {
  const receipt = ready.receipt
  const index = ready.index
  const activeRelativePath = `plugins/cache/analytix-hub/${receipt.pluginName}/${receipt.pluginVersion}`
  return ready.pluginName === receipt.pluginName && ready.pluginVersion === receipt.pluginVersion &&
    ready.packageAuthority.fileSha256 === receipt.packageAuthoritySha256 &&
    index.pluginName === receipt.pluginName && index.pluginVersion === receipt.pluginVersion &&
    receipt.activeRelativePath === activeRelativePath && index.generationId === receipt.generationId &&
    index.activeRelativePath === receipt.activeRelativePath &&
    index.receiptId === receipt.receiptId && index.intentId === receipt.intentId &&
    index.sourceTreeSha256 === receipt.sourceTreeSha256 && index.sourceTreeFileCount === receipt.sourceTreeFileCount &&
    index.receiptSha256 === sha256Hex(Buffer.from(goJSONStringify(receiptOrderedV1(receipt)))) &&
    index.indexDigest === domainDigestV1(INDEX_DIGEST_DOMAIN, indexOrderedV1(index, { indexDigest: '' }))
}

function validSemanticVersionV1(value: string): boolean {
  if (value.length === 0 || value.length > 128 || value !== value.trim()) return false
  const buildIndex = value.indexOf('+')
  if (buildIndex >= 0 && value.indexOf('+', buildIndex + 1) >= 0) return false
  const versionWithoutBuild = buildIndex >= 0 ? value.slice(0, buildIndex) : value
  if (buildIndex >= 0 && !validSemanticIdentifiersV1(value.slice(buildIndex + 1), false)) return false
  const preReleaseIndex = versionWithoutBuild.indexOf('-')
  const coreText = preReleaseIndex >= 0
    ? versionWithoutBuild.slice(0, preReleaseIndex)
    : versionWithoutBuild
  if (preReleaseIndex >= 0 &&
      !validSemanticIdentifiersV1(versionWithoutBuild.slice(preReleaseIndex + 1), true)) return false
  const core = coreText.split('.')
  return core.length === 3 && core.every((part) => /^(0|[1-9][0-9]*)$/u.test(part))
}

function validSemanticIdentifiersV1(value: string, rejectNumericLeadingZero: boolean): boolean {
  const parts = value.split('.')
  return parts.length > 0 && parts.every((part) =>
    part.length > 0 && /^[0-9A-Za-z-]+$/u.test(part) &&
    !(rejectNumericLeadingZero && /^[0-9]+$/u.test(part) && part.length > 1 && part.startsWith('0'))
  )
}

function verifyReceiptV1(receipt: z.infer<typeof Receipt>): boolean {
  try {
    const publicKey = Buffer.from(receipt.authorityPublicKey, 'base64url')
    const signature = Buffer.from(receipt.authoritySignature, 'base64url')
    if (publicKey.length !== 32 || signature.length !== 64 ||
        publicKey.toString('base64url') !== receipt.authorityPublicKey ||
        signature.toString('base64url') !== receipt.authoritySignature ||
        sha256Hex(publicKey) !== receipt.authorityKeyId ||
        receipt.receiptId !== domainDigestV1(RECEIPT_ID_DOMAIN, receiptOrderedV1(receipt, {
          receiptId: '', authoritySignature: ''
        }))) return false
    const unsigned = receiptOrderedV1(receipt, { authoritySignature: '' })
    const digest = createHash('sha256').update(goJSONStringify(unsigned)).digest()
    const key = createPublicKey({
      key: Buffer.concat([ED25519_SPKI_PREFIX, publicKey]), format: 'der', type: 'spki'
    })
    return verify(null, Buffer.concat([RECEIPT_SIGNATURE_DOMAIN, digest]), key, signature)
  } catch {
    return false
  }
}

function readyBindingDigestV1(ready: BundledFundsMaterializationBindingV1): string {
  return domainDigestV1(READY_BINDING_DOMAIN, readyOrderedV1(ready, { configurationBindingDigest: '' }))
}

function deepFreezeBindingV1(
  value: BundledFundsMaterializationBindingV1
): BundledFundsMaterializationBindingV1 {
  const freeze = (candidate: unknown): void => {
    if (!candidate || typeof candidate !== 'object' || Object.isFrozen(candidate)) return
    for (const child of Object.values(candidate)) freeze(child)
    Object.freeze(candidate)
  }
  freeze(value)
  return value
}

function receiptOrderedV1(
  value: z.infer<typeof Receipt>,
  overrides: Partial<z.infer<typeof Receipt>> = {}
): Record<string, unknown> {
  const receipt = { ...value, ...overrides }
  return {
    schemaVersion: receipt.schemaVersion, purpose: receipt.purpose, receiptId: receipt.receiptId,
    intentId: receipt.intentId, packageAuthoritySha256: receipt.packageAuthoritySha256,
    target: { platform: receipt.target.platform, arch: receipt.target.arch },
    pluginName: receipt.pluginName, pluginVersion: receipt.pluginVersion, generationId: receipt.generationId,
    activeRelativePath: receipt.activeRelativePath, sourceTreeSha256: receipt.sourceTreeSha256,
    sourceTreeFileCount: receipt.sourceTreeFileCount, manifestSha256: receipt.manifestSha256,
    entrypointSha256: receipt.entrypointSha256, factToolsEnabled: receipt.factToolsEnabled,
    issuedAt: receipt.issuedAt, authorityAlgorithm: receipt.authorityAlgorithm,
    authorityKeyId: receipt.authorityKeyId, authorityPublicKey: receipt.authorityPublicKey,
    authoritySignature: receipt.authoritySignature
  }
}

function indexOrderedV1(
  value: z.infer<typeof Index>,
  overrides: Partial<z.infer<typeof Index>> = {}
): Record<string, unknown> {
  const index = { ...value, ...overrides }
  return {
    schemaVersion: index.schemaVersion, purpose: index.purpose, indexDigest: index.indexDigest,
    pluginName: index.pluginName, pluginVersion: index.pluginVersion, generationId: index.generationId,
    activeRelativePath: index.activeRelativePath, receiptId: index.receiptId, receiptSha256: index.receiptSha256,
    intentId: index.intentId, sourceTreeSha256: index.sourceTreeSha256,
    sourceTreeFileCount: index.sourceTreeFileCount,
    discoverableGenerationCount: index.discoverableGenerationCount,
    factToolsEnabled: index.factToolsEnabled, committedAt: index.committedAt
  }
}

function readyOrderedV1(
  value: BundledFundsMaterializationBindingV1,
  overrides: Partial<BundledFundsMaterializationBindingV1> = {}
): Record<string, unknown> {
  const ready = { ...value, ...overrides }
  return {
    schemaVersion: ready.schemaVersion, purpose: ready.purpose, invocationId: ready.invocationId,
    configurationBindingDigest: ready.configurationBindingDigest,
    packageAuthority: {
      fileSha256: ready.packageAuthority.fileSha256,
      authorityDigest: ready.packageAuthority.authorityDigest,
      classification: ready.packageAuthority.classification,
      dispositionKind: ready.packageAuthority.dispositionKind,
      platformAnchor: ready.packageAuthority.platformAnchor
    },
    runtimeIdentity: {
      payloadSha256: ready.runtimeIdentity.payloadSha256, payloadBytes: ready.runtimeIdentity.payloadBytes,
      format: ready.runtimeIdentity.format, arch: ready.runtimeIdentity.arch
    },
    pluginName: ready.pluginName, pluginVersion: ready.pluginVersion,
    activePluginRoot: ready.activePluginRoot, receipt: receiptOrderedV1(ready.receipt),
    index: indexOrderedV1(ready.index), publishable: ready.publishable,
    factToolsEnabled: ready.factToolsEnabled, completedAt: ready.completedAt
  }
}

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('value is not JSON serializable')
  return encoded
    .replace(/</gu, '\\u003c')
    .replace(/>/gu, '\\u003e')
    .replace(/&/gu, '\\u0026')
    .replace(/\u2028/gu, '\\u2028')
    .replace(/\u2029/gu, '\\u2029')
}

function domainDigestV1(domain: Buffer, value: Record<string, unknown>): string {
  return sha256Hex(Buffer.concat([domain, Buffer.from(goJSONStringify(value), 'utf8')]))
}

function sha256Hex(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

async function runCommandV1(
  command: string,
  args: readonly string[],
  options: Readonly<{ cwd?: string }>
): Promise<BundledFundsMaterializationCommandResultV1> {
  return new Promise((resolveResult) => {
    const child = spawn(command, [...args], {
      cwd: options.cwd,
      env: commandEnvironmentV1(),
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true
    })
    let stdout: Buffer = Buffer.alloc(0)
    let stderr: Buffer = Buffer.alloc(0)
    let overflow = false
    let timedOut = false
    const append = (current: Buffer, chunk: Buffer | string): Buffer => {
      const incoming = Buffer.isBuffer(chunk) ? Buffer.from(chunk) : Buffer.from(chunk, 'utf8')
      if (current.length + incoming.length > MAX_COMMAND_OUTPUT_BYTES) {
        overflow = true
        child.kill('SIGKILL')
        incoming.fill(0)
        return current
      }
      const next = Buffer.concat([current, incoming])
      current.fill(0)
      incoming.fill(0)
      return next
    }
    child.stdout?.on('data', (chunk: Buffer | string) => { stdout = append(stdout, chunk) })
    child.stderr?.on('data', (chunk: Buffer | string) => { stderr = append(stderr, chunk) })
    const timer = setTimeout(() => {
      timedOut = true
      child.kill('SIGKILL')
    }, COMMAND_TIMEOUT_MS)
    child.once('error', () => {
      clearTimeout(timer)
      resolveResult({ exitCode: null, signal: null, stdout, stderr, timedOut, overflow })
    })
    child.once('exit', (exitCode, signal) => {
      clearTimeout(timer)
      resolveResult({ exitCode, signal, stdout, stderr, timedOut, overflow })
    })
  })
}

function commandEnvironmentV1(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { LANG: 'C', LC_ALL: 'C' }
  for (const key of ['HOME', 'TMPDIR', 'TEMP', 'TMP', 'SYSTEMROOT', 'WINDIR']) {
    const value = process.env[key]
    if (value) env[key] = value
  }
  env.PATH = process.platform === 'win32'
    ? process.env.PATH
    : '/usr/bin:/bin:/usr/sbin:/sbin'
  return env
}
