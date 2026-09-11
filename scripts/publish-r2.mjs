#!/usr/bin/env node
import {
  CopyObjectCommand,
  GetObjectCommand,
  ListObjectsV2Command,
  PutObjectCommand,
  S3Client
} from '@aws-sdk/client-s3'
import { createHash, createPublicKey, timingSafeEqual, verify as verifySignature } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { createReadStream } from 'node:fs'
import { existsSync, lstatSync, readFileSync, realpathSync } from 'node:fs'
import { readdir, readFile, stat } from 'node:fs/promises'
import { basename, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const { policy: macSigningPolicy } = require('./macos-signing-policy.cjs')
const { _internals: packagedAuthorityContract } = require('./after-pack.cjs')

const PRODUCT_NAME = 'analytix'
const DEFAULT_RELEASE_PREFIX = 'analytix'
const DEFAULT_RELEASE_CHANNEL = 'stable'
const PLATFORMS = ['mac', 'win', 'linux']
const RELEASE_CHANNELS = ['stable', 'beta']
const SCRIPT_DIR = fileURLToPath(new URL('.', import.meta.url))
const ROOT = resolve(SCRIPT_DIR, '..')
const RELEASE_PUBLICATION_AUTHORITY_CONTRACT = 'analytix.release-publication-authority/v1'
const RELEASE_PUBLICATION_AUTHORITY_DOMAIN = Buffer.from(
  'AnalytixReleasePublicationAuthorityV1\0',
  'utf8'
)
const RELEASE_PUBLICATION_SIGNATURE_DOMAIN = Buffer.from(
  'AnalytixReleasePublicationAuthoritySignatureV1\0',
  'utf8'
)
const MAX_RELEASE_AUTHORITY_BYTES = 1024 * 1024
const MAX_RELEASE_AUTHORITY_LIFETIME_MS = 24 * 60 * 60 * 1000
const RELEASE_AUTHORITY_CLOCK_SKEW_MS = 5 * 60 * 1000
// Deliberately unset until the release owner commits the SHA-256 of the
// controlled Ed25519 public key. A caller-provided key is never its own trust
// anchor, so production publication remains fail-closed while this is empty.
const CONTROLLED_RELEASE_AUTHORITY_PUBLIC_KEY_SHA256 = ''

function controlledReleaseAuthorityPublicKeySha256() {
  return CONTROLLED_RELEASE_AUTHORITY_PUBLIC_KEY_SHA256
}

const PLATFORM_SPECS = {
  mac: {
    updateFile: 'latest-mac.yml',
    assetPattern: /^analytix-.+-mac-(arm64|x64)\.(dmg|zip)(\.blockmap)?$/
  },
  win: {
    updateFile: 'latest.yml',
    assetPattern: /^analytix-standard-.+-x64\.exe(\.blockmap)?$/,
    distDir: 'dist-standard-win'
  },
  linux: {
    updateFile: 'latest-linux.yml',
    assetPattern: /^analytix-.+-linux-x86_64\.AppImage(\.blockmap)?$/
  }
}

function usage() {
  console.log(`Usage:
  node scripts/publish-r2.mjs verify --platform mac|win|linux --tag vX.Y.Z --authority receipt.json --public-key release-authority.pub
  node scripts/publish-r2.mjs upload --platform mac|win|linux --tag vX.Y.Z [--channel stable|beta] [--dry-run]
  node scripts/publish-r2.mjs upload --platform win --tag vX.Y.Z [--dist dist-standard-win]
  node scripts/publish-r2.mjs promote --tag vX.Y.Z [--channel stable|beta] [--platforms mac,win,linux] [--dry-run]

If --platforms is omitted, promote uses the platform manifests already uploaded for that tag.
If --channel is omitted, the default channel is stable.

Environment:
  ANALYTIX_RELEASE_ENV=scripts/release.local.env
  RELEASE_CHANNEL=stable|beta
  R2_BUCKET or S3_BUCKET
  R2_ENDPOINT or S3_ENDPOINT
  R2_ACCESS_KEY_ID or S3_ACCESS_KEY_ID
  R2_SECRET_ACCESS_KEY or S3_SECRET_ACCESS_KEY
  R2_PUBLIC_BASE_URL
  ANALYTIX_RELEASE_PREFIX=analytix
  ANALYTIX_RELEASE_AUTHORITY=/controlled/path/release-publication-authority.json
  ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY=/controlled/path/release-authority-ed25519.pub
`)
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value).sort()
  const wanted = [...expected].sort()
  return actual.length === wanted.length && actual.every((key, index) => key === wanted[index])
}

function isSha256(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
}

function isSafeFileName(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= 255 &&
    basename(value) === value && value !== '.' && value !== '..' && !value.includes('\0') &&
    !value.includes('/') && !value.includes('\\')
}

function targetBelongsToPlatform(targetKey, platform) {
  const prefix = platform === 'mac' ? 'darwin-' : platform === 'win' ? 'win32-' : 'linux-'
  return targetKey.startsWith(prefix)
}

function canonicalJson(value) {
  return JSON.stringify(value)
}

function sha256Bytes(value) {
  return createHash('sha256').update(value).digest('hex')
}

function releasePublicationAuthorityDigest(authorityWithoutDigestAndSignature) {
  return createHash('sha256')
    .update(RELEASE_PUBLICATION_AUTHORITY_DOMAIN)
    .update(canonicalJson(authorityWithoutDigestAndSignature), 'utf8')
    .digest('hex')
}

function releasePublicationAuthoritySignaturePayload(receiptDigest) {
  if (!isSha256(receiptDigest)) throw new Error('release_publication_authority_digest_invalid')
  return Buffer.concat([RELEASE_PUBLICATION_SIGNATURE_DOMAIN, Buffer.from(receiptDigest, 'hex')])
}

function readStableRegularFile(path, options = {}) {
  const maximumBytes = options.maximumBytes || Number.MAX_SAFE_INTEGER
  let before
  let after
  let first
  let second
  try {
    before = lstatSync(path, { bigint: true })
    if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1n || before.size <= 0n ||
      before.size > BigInt(maximumBytes) || (before.mode & 0o022n) !== 0n) {
      throw new Error('untrusted regular file')
    }
    first = readFileSync(path)
    after = lstatSync(path, { bigint: true })
    second = readFileSync(path)
  } catch (error) {
    throw new Error(`release_publication_authority_file_untrusted: ${error.message}`)
  }
  if (before.dev !== after.dev || before.ino !== after.ino || before.size !== after.size ||
    before.mtimeNs !== after.mtimeNs || before.ctimeNs !== after.ctimeNs ||
    !timingSafeEqual(createHash('sha256').update(first).digest(), createHash('sha256').update(second).digest())) {
    throw new Error('release_publication_authority_file_changed_during_read')
  }
  return { bytes: first, byteLength: first.length, sha256: sha256Bytes(first) }
}

function parseStrictAuthority(bytes, label) {
  try {
    return parseStrictJsonObject(bytes, {
      maxBytes: MAX_RELEASE_AUTHORITY_BYTES,
      maxDepth: 16,
      maxTokens: 65536,
      maxStringBytes: 64 * 1024,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error(`${label}_json_invalid`)
  }
}

function parseCanonicalInstant(value, label) {
  if (typeof value !== 'string') throw new Error(`${label}_invalid`)
  const millis = Date.parse(value)
  if (!Number.isFinite(millis) || new Date(millis).toISOString() !== value) {
    throw new Error(`${label}_invalid`)
  }
  return millis
}

function requireSortedUnique(values, key, label) {
  const observed = values.map(key)
  const expected = [...observed].sort()
  if (new Set(observed).size !== observed.length || canonicalJson(observed) !== canonicalJson(expected)) {
    throw new Error(`${label}_order_invalid`)
  }
}

function validateReleasePublicationAuthorityShape(authority, options) {
  const expectedKeys = [
    'schemaVersion', 'contract', 'keyId', 'sourceCommit', 'tag', 'channel', 'issuedAt',
    'expiresAt', 'releaseEligible', 'publicationReceiptIssued', 'platforms',
    'packagedBuildAuthorities', 'artifacts', 'attestations', 'receiptDigest', 'signature'
  ]
  if (!exactKeys(authority, expectedKeys) || authority.schemaVersion !== 1 ||
    authority.contract !== RELEASE_PUBLICATION_AUTHORITY_CONTRACT ||
    typeof authority.keyId !== 'string' || !/^[A-Za-z0-9._-]{1,128}$/u.test(authority.keyId) ||
    typeof authority.sourceCommit !== 'string' || !/^[0-9a-f]{40}$/u.test(authority.sourceCommit) ||
    authority.tag !== options.tag || authority.channel !== options.channel ||
    authority.releaseEligible !== true || authority.publicationReceiptIssued !== true ||
    !Array.isArray(authority.platforms) || authority.platforms.length === 0 ||
    authority.platforms.some((platform) => !PLATFORMS.includes(platform)) ||
    !Array.isArray(authority.packagedBuildAuthorities) || authority.packagedBuildAuthorities.length === 0 ||
    !Array.isArray(authority.artifacts) || authority.artifacts.length === 0 ||
    !exactKeys(authority.attestations, ['mac', 'win', 'linux']) ||
    !Array.isArray(authority.attestations.mac) || !Array.isArray(authority.attestations.win) ||
    !Array.isArray(authority.attestations.linux) || !isSha256(authority.receiptDigest) ||
    typeof authority.signature !== 'string') {
    throw new Error('release_publication_authority_invalid')
  }
  requireSortedUnique(authority.platforms, (value) => value, 'release_publication_authority_platform')
  if (authority.sourceCommit !== options.sourceCommit) {
    throw new Error('release_publication_authority_source_commit_mismatch')
  }

  const issuedAt = parseCanonicalInstant(authority.issuedAt, 'release_publication_authority_issued_at')
  const expiresAt = parseCanonicalInstant(authority.expiresAt, 'release_publication_authority_expires_at')
  const now = options.now.getTime()
  if (issuedAt > now + RELEASE_AUTHORITY_CLOCK_SKEW_MS || expiresAt <= now || expiresAt <= issuedAt ||
    expiresAt - issuedAt > MAX_RELEASE_AUTHORITY_LIFETIME_MS) {
    throw new Error('release_publication_authority_stale')
  }

  for (const item of authority.packagedBuildAuthorities) {
    if (!exactKeys(item, [
      'platform', 'targetKey', 'fileName', 'byteLength', 'sha256', 'authorityDigest'
    ]) || !PLATFORMS.includes(item.platform) || typeof item.targetKey !== 'string' ||
      !/^(?:darwin|win32|linux)-(?:arm64|x64)$/u.test(item.targetKey) ||
      !targetBelongsToPlatform(item.targetKey, item.platform) ||
      !isSafeFileName(item.fileName) || !Number.isSafeInteger(item.byteLength) || item.byteLength <= 0 ||
      !isSha256(item.sha256) || !isSha256(item.authorityDigest)) {
      throw new Error('packaged_build_authority_receipt_invalid')
    }
  }
  requireSortedUnique(
    authority.packagedBuildAuthorities,
    (item) => `${item.platform}\0${item.targetKey}`,
    'packaged_build_authority_receipt'
  )

  for (const item of authority.artifacts) {
    if (!exactKeys(item, ['platform', 'fileName', 'targetKeys', 'byteLength', 'sha256']) ||
      !PLATFORMS.includes(item.platform) || !isSafeFileName(item.fileName) ||
      !Array.isArray(item.targetKeys) || item.targetKeys.length === 0 ||
      item.targetKeys.some((targetKey) => typeof targetKey !== 'string') ||
      !Number.isSafeInteger(item.byteLength) || item.byteLength <= 0 || !isSha256(item.sha256)) {
      throw new Error('release_artifact_receipt_invalid')
    }
    requireSortedUnique(item.targetKeys, (value) => value, 'release_artifact_target')
  }
  requireSortedUnique(authority.artifacts, (item) => `${item.platform}\0${item.fileName}`, 'release_artifact')

  for (const item of authority.attestations.mac) {
    if (!exactKeys(item, [
      'targetKey', 'developerIdVerified', 'teamIdentifier', 'secureTimestampVerified',
      'notaryStatus', 'notarySubmissionId', 'stapled', 'stapleValidated'
    ]) || typeof item.targetKey !== 'string' || item.developerIdVerified !== true ||
      !targetBelongsToPlatform(item.targetKey, 'mac') ||
      !/^[A-Z0-9]{10}$/u.test(item.teamIdentifier || '') || item.secureTimestampVerified !== true ||
      item.notaryStatus !== 'Accepted' || typeof item.notarySubmissionId !== 'string' ||
      !/^[A-Za-z0-9-]{8,128}$/u.test(item.notarySubmissionId) || item.stapled !== true ||
      item.stapleValidated !== true) {
      throw new Error('mac_release_attestation_invalid')
    }
  }
  for (const item of authority.attestations.win) {
    if (!exactKeys(item, [
      'targetKey', 'authenticodeVerified', 'signerThumbprint', 'secureTimestampVerified'
    ]) || typeof item.targetKey !== 'string' || item.authenticodeVerified !== true ||
      !targetBelongsToPlatform(item.targetKey, 'win') ||
      !/^[0-9A-F]{40}$/u.test(item.signerThumbprint || '') || item.secureTimestampVerified !== true) {
      throw new Error('windows_release_attestation_invalid')
    }
  }
  for (const item of authority.attestations.linux) {
    if (!exactKeys(item, ['targetKey', 'packageSignatureVerified', 'signingKeyFingerprint']) ||
      typeof item.targetKey !== 'string' || item.packageSignatureVerified !== true ||
      !targetBelongsToPlatform(item.targetKey, 'linux') ||
      !/^[0-9A-F]{40,64}$/u.test(item.signingKeyFingerprint || '')) {
      throw new Error('linux_release_attestation_invalid')
    }
  }
  requireSortedUnique(authority.attestations.mac, (item) => item.targetKey, 'mac_release_attestation')
  requireSortedUnique(authority.attestations.win, (item) => item.targetKey, 'windows_release_attestation')
  requireSortedUnique(authority.attestations.linux, (item) => item.targetKey, 'linux_release_attestation')

  const { receiptDigest, signature, ...withoutDigestAndSignature } = authority
  const expectedDigest = releasePublicationAuthorityDigest(withoutDigestAndSignature)
  if (receiptDigest !== expectedDigest) throw new Error('release_publication_authority_digest_mismatch')
  let decodedSignature
  try {
    decodedSignature = Buffer.from(signature, 'base64')
  } catch {
    throw new Error('release_publication_authority_signature_invalid')
  }
  if (!decodedSignature.length || decodedSignature.toString('base64') !== signature) {
    throw new Error('release_publication_authority_signature_invalid')
  }
  let publicKey
  try {
    publicKey = createPublicKey(options.publicKeyBytes)
  } catch {
    throw new Error('release_publication_authority_public_key_invalid')
  }
  if (publicKey.asymmetricKeyType !== 'ed25519' ||
    !verifySignature(
      null,
      releasePublicationAuthoritySignaturePayload(receiptDigest),
      publicKey,
      decodedSignature
    )) {
    throw new Error('release_publication_authority_signature_invalid')
  }
}

function parseEnvFile(content) {
  const values = new Map()
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const match = line.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/)
    if (!match) continue
    let value = match[2].trim()
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1)
    }
    values.set(match[1], value)
  }
  return values
}

function loadLocalEnv() {
  const configured = process.env.ANALYTIX_RELEASE_ENV?.trim()
  const candidates = [
    configured,
    join(ROOT, 'scripts', 'release.local.env'),
    join(ROOT, 'release.local.env')
  ].filter(Boolean)

  for (const candidate of candidates) {
    if (!existsSync(candidate)) continue
    const values = parseEnvFile(readFileSync(candidate, 'utf8'))
    for (const [key, value] of values) {
      if (!process.env[key]) process.env[key] = value
    }
    console.log(`Loaded local release config: ${candidate}`)
    return candidate
  }
  return null
}

function readArgs(argv) {
  const flags = new Map()
  const positionals = []
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i]
    if (!arg.startsWith('--')) {
      positionals.push(arg)
      continue
    }
    const name = arg.slice(2)
    if (name === 'dry-run' || name === 'help' || name === 'h' || name === 'stable' || name === 'beta') {
      flags.set(name, true)
      continue
    }
    const value = argv[i + 1]
    if (!value || value.startsWith('--')) {
      throw new Error(`Missing value for --${name}`)
    }
    flags.set(name, value)
    i += 1
  }
  return { command: positionals[0], flags }
}

function requireFlag(flags, name) {
  const value = flags.get(name)
  if (typeof value !== 'string' || !value.trim()) {
    throw new Error(`Missing required flag --${name}`)
  }
  return value.trim()
}

function normalizeTag(raw) {
  const tag = raw.trim()
  if (!/^v\d+\.\d+\.\d+$/.test(tag)) {
    throw new Error(`Tag must look like vX.Y.Z. electron-updater cannot use four-part versions, got: ${raw}`)
  }
  return tag
}

function normalizeChannel(raw) {
  const channel = String(raw || '').trim() || DEFAULT_RELEASE_CHANNEL
  if (!RELEASE_CHANNELS.includes(channel)) {
    throw new Error(`Release channel must be one of: ${RELEASE_CHANNELS.join(', ')}`)
  }
  return channel
}

function readChannel(flags) {
  if (flags.has('stable') && flags.has('beta')) {
    throw new Error('Use only one of --stable or --beta.')
  }
  if (flags.has('stable')) return 'stable'
  if (flags.has('beta')) return 'beta'
  return normalizeChannel(
    flags.get('channel') ||
      process.env.RELEASE_CHANNEL ||
      process.env.ANALYTIX_UPDATE_CHANNEL ||
      DEFAULT_RELEASE_CHANNEL
  )
}

function positiveInt(raw, fallback) {
  const value = Number.parseInt(String(raw || '').trim(), 10)
  return Number.isInteger(value) && value > 0 ? value : fallback
}

async function runConcurrently(items, limit, worker) {
  let nextIndex = 0
  const workerCount = Math.min(limit, items.length)
  await Promise.all(
    Array.from({ length: workerCount }, async () => {
      while (nextIndex < items.length) {
        const item = items[nextIndex]
        nextIndex += 1
        await worker(item)
      }
    })
  )
}

function normalizeBaseUrl(raw) {
  return raw.trim().replace(/\/+$/, '')
}

function trimSlashes(value) {
  return value.replace(/^\/+|\/+$/g, '')
}

function joinUrl(base, ...parts) {
  return [normalizeBaseUrl(base), ...parts.map((p) => trimSlashes(p)).filter(Boolean)].join('/')
}

function channelBasePath(prefix, channel) {
  return `${prefix}/channels/${channel}`
}

function firstEnv(...names) {
  for (const name of names) {
    const value = process.env[name]?.trim()
    if (value) return value
  }
  return ''
}

function normalizeS3Endpoint(rawEndpoint, bucket) {
  const value = rawEndpoint.trim()
  if (!value) return ''
  const url = new URL(value)
  const normalizedBucket = bucket.trim()
  const path = url.pathname.replace(/\/+$/, '')
  if (normalizedBucket && path === `/${normalizedBucket}`) {
    url.pathname = ''
  }
  return url.toString().replace(/\/+$/, '')
}

function readConfig({ dryRun = false } = {}) {
  loadLocalEnv()
  const accountId = firstEnv('R2_ACCOUNT_ID')
  const bucket = firstEnv('R2_BUCKET', 'S3_BUCKET')
  const accessKeyId = firstEnv('R2_ACCESS_KEY_ID', 'S3_ACCESS_KEY_ID', 'AWS_ACCESS_KEY_ID')
  const secretAccessKey = firstEnv(
    'R2_SECRET_ACCESS_KEY',
    'S3_SECRET_ACCESS_KEY',
    'AWS_SECRET_ACCESS_KEY'
  )
  const endpoint = normalizeS3Endpoint(firstEnv('R2_ENDPOINT', 'S3_ENDPOINT'), bucket)
  const publicBaseUrl = firstEnv('R2_PUBLIC_BASE_URL', 'PUBLIC_DOWNLOAD_BASE_URL')
  const prefix = trimSlashes(firstEnv('ANALYTIX_RELEASE_PREFIX') || DEFAULT_RELEASE_PREFIX)

  if (!publicBaseUrl) {
    throw new Error('R2_PUBLIC_BASE_URL is required so manifests can contain public download URLs.')
  }
  if (!dryRun && /(^|\.)downloads\.example\.com$/i.test(new URL(publicBaseUrl).hostname)) {
    throw new Error('Replace the placeholder R2_PUBLIC_BASE_URL with your real R2 custom domain.')
  }

  if (!dryRun) {
    const missing = []
    if (!endpoint && !accountId) missing.push('R2_ENDPOINT or R2_ACCOUNT_ID')
    if (!accessKeyId) missing.push('R2_ACCESS_KEY_ID or S3_ACCESS_KEY_ID')
    if (!secretAccessKey) missing.push('R2_SECRET_ACCESS_KEY or S3_SECRET_ACCESS_KEY')
    if (!bucket) missing.push('R2_BUCKET or S3_BUCKET')
    if (missing.length) throw new Error(`Missing environment variable(s): ${missing.join(', ')}`)
  }

  const resolvedEndpoint = endpoint || `https://${accountId}.r2.cloudflarestorage.com`
  const client = dryRun
    ? null
    : new S3Client({
        region: 'auto',
        endpoint: resolvedEndpoint,
        credentials: { accessKeyId, secretAccessKey },
        forcePathStyle: true
      })

  return { bucket, publicBaseUrl: normalizeBaseUrl(publicBaseUrl), prefix, client }
}

function quoteScalar(value) {
  const trimmed = value.trim()
  return trimmed.replace(/^['"]|['"]$/g, '')
}

function parseUpdateYml(source) {
  const version = quoteScalar(source.match(/^version:\s*(.+)$/m)?.[1] ?? '')
  const releaseDate = quoteScalar(source.match(/^releaseDate:\s*(.+)$/m)?.[1] ?? '')
  const files = []
  let current = null

  for (const line of source.split(/\r?\n/)) {
    const url = line.match(/^\s*-\s+url:\s*(.+)$/)
    if (url) {
      current = { url: quoteScalar(url[1]), sha512: '', size: 0 }
      files.push(current)
      continue
    }
    if (!current) continue
    const prop = line.match(/^\s+(sha512|size|blockMapSize):\s*(.+)$/)
    if (!prop) continue
    const [, key, value] = prop
    current[key] = key === 'sha512' ? quoteScalar(value) : Number.parseInt(value, 10) || 0
  }

  if (!version) throw new Error('Update metadata is missing version.')
  if (!files.length) throw new Error('Update metadata is missing files.')
  return { version, releaseDate, files }
}

function distDirectoryForPlatform(platform, overrides = {}) {
  return resolve(overrides[platform] || PLATFORM_SPECS[platform]?.distDir || 'dist')
}

function safeFileInDirectory(directory, fileName) {
  if (!isSafeFileName(fileName)) throw new Error('release_artifact_file_name_invalid')
  const root = realpathSync(directory)
  const path = resolve(root, fileName)
  const pathFromRoot = relative(root, path)
  if (!pathFromRoot || pathFromRoot === '..' || pathFromRoot.startsWith(`..${sep}`) ||
    isAbsolute(pathFromRoot) || basename(pathFromRoot) !== pathFromRoot) {
    throw new Error('release_artifact_path_escape')
  }
  const realPath = realpathSync(path)
  if (relative(root, realPath) !== fileName) throw new Error('release_artifact_path_untrusted')
  return path
}

async function requiredLocalPublicationFileNames(distDir, platform, expectedVersion) {
  const spec = PLATFORM_SPECS[platform]
  const entries = await readdir(distDir)
  const updatePath = safeFileInDirectory(distDir, spec.updateFile)
  const updateText = await readFile(updatePath, 'utf8')
  const updateMetadata = parseUpdateYml(updateText)
  if (updateMetadata.version !== expectedVersion) {
    throw new Error('release_update_metadata_version_mismatch')
  }
  const referenced = updateMetadata.files.map((file) => basename(file.url))
  if (referenced.some((fileName) => !isSafeFileName(fileName))) {
    throw new Error('release_update_metadata_file_name_invalid')
  }
  const assets = entries.filter((name) => spec.assetPattern.test(name))
  if (assets.length === 0) throw new Error(`No ${platform} release artifacts found in ${distDir}`)
  const required = Array.from(new Set([spec.updateFile, ...assets, ...referenced])).sort()
  for (const fileName of required) safeFileInDirectory(distDir, fileName)
  return required
}

function expectedTargetKeysForArtifact(platform, fileName, targetKeys) {
  if (fileName === PLATFORM_SPECS[platform].updateFile) {
    return [...targetKeys].sort()
  }
  if (platform === 'mac') {
    if (fileName.includes('-arm64.')) return ['darwin-arm64']
    if (fileName.includes('-x64.')) return ['darwin-x64']
  }
  if (platform === 'win') return ['win32-x64']
  if (platform === 'linux') return ['linux-x64']
  throw new Error(`release_artifact_target_unresolved: ${fileName}`)
}

function productionSourceCommit() {
  const value = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: ROOT,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe']
  }).trim()
  if (!/^[0-9a-f]{40}$/u.test(value)) throw new Error('release_source_commit_invalid')
  return value
}

function releaseAuthorityPaths(flags) {
  const authorityPath = String(
    flags.get('authority') || process.env.ANALYTIX_RELEASE_AUTHORITY || ''
  ).trim()
  const publicKeyPath = String(
    flags.get('public-key') || process.env.ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY || ''
  ).trim()
  if (!authorityPath) throw new Error('release_publication_authority_missing')
  if (!publicKeyPath) throw new Error('release_publication_authority_public_key_missing')
  return { authorityPath: resolve(authorityPath), publicKeyPath: resolve(publicKeyPath) }
}

async function verifyReleasePublicationAuthority(options) {
  const authorityFile = readStableRegularFile(options.authorityPath, {
    maximumBytes: MAX_RELEASE_AUTHORITY_BYTES
  })
  const publicKeyFile = readStableRegularFile(options.publicKeyPath, { maximumBytes: 64 * 1024 })
  if (!isSha256(options.trustedPublicKeySha256) ||
    publicKeyFile.sha256 !== options.trustedPublicKeySha256) {
    throw new Error('release_publication_authority_trust_anchor_mismatch')
  }
  const authority = parseStrictAuthority(authorityFile.bytes, 'release_publication_authority')
  if (canonicalJson(authority) !== authorityFile.bytes.toString('utf8')) {
    throw new Error('release_publication_authority_not_canonical')
  }
  validateReleasePublicationAuthorityShape(authority, {
    tag: options.tag,
    channel: options.channel,
    sourceCommit: options.sourceCommit,
    now: options.now || new Date(),
    publicKeyBytes: publicKeyFile.bytes
  })

  const platforms = options.platforms?.length ? [...options.platforms] : [...authority.platforms]
  if (platforms.some((platform) => !authority.platforms.includes(platform))) {
    throw new Error('release_publication_authority_platform_mismatch')
  }
  requireSortedUnique([...platforms].sort(), (value) => value, 'release_publication_authority_requested_platform')
  const platformSet = new Set(platforms)
  const packagedByTarget = new Map()

  for (const entry of authority.packagedBuildAuthorities) {
    if (!platformSet.has(entry.platform)) continue
    const distDir = distDirectoryForPlatform(entry.platform, options.distDirs)
    const authorityPath = safeFileInDirectory(distDir, entry.fileName)
    const stable = readStableRegularFile(authorityPath, { maximumBytes: MAX_RELEASE_AUTHORITY_BYTES })
    if (stable.byteLength !== entry.byteLength || stable.sha256 !== entry.sha256) {
      throw new Error('packaged_build_authority_file_mismatch')
    }
    const packaged = parseStrictAuthority(stable.bytes, 'packaged_build_authority')
    if (canonicalJson(packaged) !== stable.bytes.toString('utf8') ||
      !packagedAuthorityContract.isPackagedBuildAuthorityV2(packaged) ||
      packaged.authorityDigest !== entry.authorityDigest || packaged.targetKey !== entry.targetKey ||
      packaged.sourceCommit !== authority.sourceCommit || packaged.worktreeSnapshot?.dirty !== false ||
      packaged.classification !== 'controlled_release_clean_candidate_non_publishable' ||
      packaged.publishable !== false || packaged.releaseEligible !== false ||
      packaged.publicationReceiptIssued !== false ||
      packaged.nativeDisposition?.kind !== 'controlled_release_receipt') {
      throw new Error('packaged_build_authority_not_controlled_clean_candidate')
    }
    if (entry.platform === 'mac') {
      const officialTeamIdentifier = String(options.officialTeamIdentifier || '').trim()
      if (!officialTeamIdentifier) throw new Error('official_apple_team_identifier_unavailable')
      if (packaged.nativeDisposition.signingMode !== 'developer-id' ||
        packaged.nativeDisposition.appleTeamIdentifier !== officialTeamIdentifier) {
        throw new Error('packaged_build_authority_developer_id_mismatch')
      }
    }
    packagedByTarget.set(entry.targetKey, packaged)
  }

  for (const platform of platforms) {
    const platformAuthorities = authority.packagedBuildAuthorities.filter((entry) => entry.platform === platform)
    if (platformAuthorities.length === 0) throw new Error('packaged_build_authority_missing')
    const targetKeys = platformAuthorities.map((entry) => entry.targetKey).sort()
    const distDir = distDirectoryForPlatform(platform, options.distDirs)
    const requiredFileNames = await requiredLocalPublicationFileNames(
      distDir,
      platform,
      authority.tag.slice(1)
    )
    const artifactEntries = authority.artifacts.filter((entry) => entry.platform === platform)
    const artifactFileNames = artifactEntries.map((entry) => entry.fileName).sort()
    if (canonicalJson(artifactFileNames) !== canonicalJson(requiredFileNames)) {
      throw new Error('release_artifact_receipt_set_mismatch')
    }
    for (const entry of artifactEntries) {
      const expectedTargets = expectedTargetKeysForArtifact(platform, entry.fileName, targetKeys)
      if (canonicalJson(entry.targetKeys) !== canonicalJson(expectedTargets) ||
        entry.targetKeys.some((targetKey) => !packagedByTarget.has(targetKey))) {
        throw new Error('release_artifact_target_mismatch')
      }
      const artifactPath = safeFileInDirectory(distDir, entry.fileName)
      const stable = readStableRegularFile(artifactPath)
      if (stable.byteLength !== entry.byteLength || stable.sha256 !== entry.sha256) {
        throw new Error('release_artifact_hash_mismatch')
      }
    }

    const attestations = authority.attestations[platform]
    const attestedTargets = attestations.map((entry) => entry.targetKey).sort()
    if (canonicalJson(attestedTargets) !== canonicalJson(targetKeys)) {
      throw new Error(`${platform}_release_attestation_set_mismatch`)
    }
    if (platform === 'mac') {
      const teamIdentifier = String(options.officialTeamIdentifier || '').trim()
      if (attestations.some((entry) => entry.teamIdentifier !== teamIdentifier)) {
        throw new Error('mac_release_attestation_team_mismatch')
      }
    }
  }

  return { authority, platforms, authoritySha256: authorityFile.sha256 }
}

async function requireReleasePublicationAuthority(flags, options) {
  const paths = releaseAuthorityPaths(flags)
  if (!isSha256(CONTROLLED_RELEASE_AUTHORITY_PUBLIC_KEY_SHA256)) {
    throw new Error('release_publication_authority_trust_anchor_unavailable')
  }
  const requestedPlatforms = options.platforms?.length ? [...options.platforms].sort() : undefined
  const officialTeamIdentifier = requestedPlatforms?.includes('mac') ||
    (!requestedPlatforms && options.mayIncludeMac !== false)
    ? macSigningPolicy.officialTeamIdentifier
    : ''
  return verifyReleasePublicationAuthority({
    ...paths,
    tag: options.tag,
    channel: options.channel,
    sourceCommit: productionSourceCommit(),
    platforms: requestedPlatforms,
    distDirs: options.distDirs,
    officialTeamIdentifier,
    trustedPublicKeySha256: CONTROLLED_RELEASE_AUTHORITY_PUBLIC_KEY_SHA256,
    now: new Date()
  })
}

async function hashFile(path, algorithm, encoding) {
  const hash = createHash(algorithm)
  await new Promise((resolvePromise, reject) => {
    createReadStream(path)
      .on('data', (chunk) => hash.update(chunk))
      .on('error', reject)
      .on('end', resolvePromise)
  })
  return hash.digest(encoding)
}

function contentType(fileName) {
  if (fileName.endsWith('.yml') || fileName.endsWith('.yaml')) return 'text/yaml; charset=utf-8'
  if (fileName.endsWith('.json')) return 'application/json; charset=utf-8'
  if (fileName.endsWith('.zip')) return 'application/zip'
  if (fileName.endsWith('.dmg')) return 'application/x-apple-diskimage'
  if (fileName.endsWith('.exe')) return 'application/vnd.microsoft.portable-executable'
  return 'application/octet-stream'
}

function cacheControlFor(key) {
  if (/\/latest\/latest(?:-[\w]+)?\.(?:json|yml)$/.test(key)) {
    return 'public, max-age=60, must-revalidate'
  }
  if (/\/latest\/.+\.(?:dmg|zip|exe|AppImage|blockmap)$/.test(key)) {
    return 'public, max-age=31536000, immutable'
  }
  return 'public, max-age=31536000, immutable'
}

function classifyDownload(fileName, platform) {
  const extension = fileName.endsWith('.AppImage')
    ? 'AppImage'
    : fileName.endsWith('.dmg')
      ? 'dmg'
      : fileName.endsWith('.zip')
        ? 'zip'
        : fileName.endsWith('.exe')
          ? 'exe'
          : 'bin'

  if (platform === 'mac') {
    const arch = fileName.includes('-arm64.') ? 'arm64' : 'x64'
    return {
      platform,
      arch,
      format: extension,
      label: arch === 'arm64' ? `macOS Apple Silicon ${extension.toUpperCase()}` : `macOS Intel ${extension.toUpperCase()}`
    }
  }
  if (platform === 'win') {
    return { platform, arch: 'x64', format: extension, label: 'Windows x64 installer' }
  }
  return { platform, arch: 'x64', format: extension, label: 'Linux x64 AppImage' }
}

async function collectPlatformRelease({ distDir, platform, tag, channel, config }) {
  const spec = PLATFORM_SPECS[platform]
  if (!spec) throw new Error(`Unsupported platform: ${platform}`)

  const entries = await readdir(distDir)
  const updatePath = join(distDir, spec.updateFile)
  const updateText = await readFile(updatePath, 'utf8')
  const updateMetadata = parseUpdateYml(updateText)
  const tagVersion = tag.slice(1)
  if (updateMetadata.version !== tagVersion) {
    throw new Error(
      `${spec.updateFile} version ${updateMetadata.version} does not match ${tag}. Rebuild with ANALYTIX_APP_VERSION=${tagVersion}.`
    )
  }

  const referenced = new Set(updateMetadata.files.map((file) => basename(file.url)))
  const assets = entries.filter((name) => spec.assetPattern.test(name))
  for (const name of referenced) {
    if (!entries.includes(name)) {
      throw new Error(`${spec.updateFile} references ${name}, but it was not found in ${distDir}`)
    }
  }

  const fileNames = Array.from(new Set([spec.updateFile, ...assets, ...referenced])).sort()
  const files = []
  const downloadByName = new Map(updateMetadata.files.map((file) => [basename(file.url), file]))

  for (const fileName of fileNames) {
    const path = join(distDir, fileName)
    const info = await stat(path)
    if (!info.isFile()) continue
    const basePath = channelBasePath(config.prefix, channel)
    const archiveKey = `${basePath}/releases/${tag}/${fileName}`
    const sha256 = await hashFile(path, 'sha256', 'hex')
    const sha512 = await hashFile(path, 'sha512', 'base64')
    files.push({
      fileName,
      path,
      key: archiveKey,
      size: info.size,
      sha256,
      sha512,
      contentType: contentType(fileName),
      updateMetadata: fileName === spec.updateFile,
      downloadable: downloadByName.has(fileName)
    })
  }

  const filesByName = new Map(files.map((file) => [file.fileName, file]))
  const downloads = updateMetadata.files.map((file) => {
    const fileName = basename(file.url)
    const local = filesByName.get(fileName)
    if (!local) throw new Error(`Missing collected file: ${fileName}`)
    return {
      ...classifyDownload(fileName, platform),
      fileName,
      size: local.size,
      sha256: local.sha256,
      sha512: file.sha512 || local.sha512,
      blockMapSize: file.blockMapSize,
      archiveUrl: joinUrl(config.publicBaseUrl, config.prefix, 'channels', channel, 'releases', tag, fileName),
      latestUrl: joinUrl(config.publicBaseUrl, config.prefix, 'channels', channel, 'latest', fileName)
    }
  })

  return {
    schemaVersion: 1,
    productName: PRODUCT_NAME,
    tag,
    channel,
    platform,
    version: updateMetadata.version,
    releaseDate: updateMetadata.releaseDate,
    generatedAt: new Date().toISOString(),
    updateMetadata: {
      fileName: spec.updateFile,
      archiveUrl: joinUrl(config.publicBaseUrl, config.prefix, 'channels', channel, 'releases', tag, spec.updateFile),
      latestUrl: joinUrl(config.publicBaseUrl, config.prefix, 'channels', channel, 'latest', spec.updateFile)
    },
    files,
    downloads
  }
}

async function putObject({ config, key, body, contentType: type, cacheControl, contentLength, dryRun }) {
  if (dryRun) {
    console.log(`[dry-run] put s3://${config.bucket || '<bucket>'}/${key}`)
    return
  }
  const input = {
    Bucket: config.bucket,
    Key: key,
    Body: body,
    ContentType: type,
    CacheControl: cacheControl
  }
  if (typeof contentLength === 'number') input.ContentLength = contentLength
  await config.client.send(new PutObjectCommand(input))
}

async function copyObject({ config, fromKey, toKey, type, dryRun }) {
  if (dryRun) {
    console.log(`[dry-run] copy s3://${config.bucket}/${fromKey} -> s3://${config.bucket}/${toKey}`)
    return
  }
  const copySource = `${config.bucket}/${fromKey}`
    .split('/')
    .map((part) => encodeURIComponent(part))
    .join('/')
  await config.client.send(
    new CopyObjectCommand({
      Bucket: config.bucket,
      Key: toKey,
      CopySource: copySource,
      ContentType: type,
      CacheControl: cacheControlFor(toKey),
      MetadataDirective: 'REPLACE'
    })
  )
}

async function uploadPlatform({ flags, dryRun }) {
  const platform = requireFlag(flags, 'platform')
  if (!PLATFORMS.includes(platform)) {
    throw new Error(`--platform must be one of: ${PLATFORMS.join(', ')}`)
  }
  const tag = normalizeTag(requireFlag(flags, 'tag'))
  const channel = readChannel(flags)
  const spec = PLATFORM_SPECS[platform]
  const distDir = resolve(flags.get('dist') || spec.distDir || 'dist')
  const publicationAuthority = await requireReleasePublicationAuthority(flags, {
    tag,
    channel,
    platforms: [platform],
    distDirs: { [platform]: distDir }
  })
  const config = readConfig({ dryRun })
  const release = await collectPlatformRelease({ distDir, platform, tag, channel, config })
  const authorizedArtifacts = new Map(
    publicationAuthority.authority.artifacts
      .filter((entry) => entry.platform === platform)
      .map((entry) => [entry.fileName, entry])
  )
  if (release.files.length !== authorizedArtifacts.size || release.files.some((file) => {
    const authorized = authorizedArtifacts.get(file.fileName)
    return !authorized || authorized.byteLength !== file.size || authorized.sha256 !== file.sha256
  })) {
    throw new Error('release_artifact_changed_after_authority_preflight')
  }

  console.log(
    `Uploading ${PRODUCT_NAME} ${release.version} ${platform} assets to R2 ${channel} archive ${tag}`
  )
  const uploadConcurrency = positiveInt(
    process.env.R2_UPLOAD_CONCURRENCY || process.env.RELEASE_UPLOAD_CONCURRENCY,
    4
  )
  console.log(`Using R2 upload concurrency: ${uploadConcurrency}`)
  await runConcurrently(release.files, uploadConcurrency, async (file) => {
    await putObject({
      config,
      key: file.key,
      body: createReadStream(file.path),
      contentType: file.contentType,
      cacheControl: cacheControlFor(file.key),
      contentLength: file.size,
      dryRun
    })
    console.log(`  ${file.fileName}`)
  })

  const manifestKey = `${channelBasePath(config.prefix, channel)}/releases/${tag}/release-${platform}.json`
  const manifest = JSON.stringify(
    {
      ...release,
      files: release.files.map(({ path: _path, ...file }) => file)
    },
    null,
    2
  )
  await putObject({
    config,
    key: manifestKey,
    body: manifest,
    contentType: 'application/json; charset=utf-8',
    cacheControl: 'public, max-age=31536000, immutable',
    dryRun
  })
  console.log(`  release-${platform}.json`)
}

async function listReleaseKeys(config, tag, channel) {
  const prefix = `${channelBasePath(config.prefix, channel)}/releases/${tag}/`
  const keys = []
  let ContinuationToken
  do {
    const res = await config.client.send(
      new ListObjectsV2Command({
        Bucket: config.bucket,
        Prefix: prefix,
        ContinuationToken
      })
    )
    for (const item of res.Contents ?? []) {
      if (item.Key) keys.push(item.Key)
    }
    ContinuationToken = res.NextContinuationToken
  } while (ContinuationToken)
  return keys
}

async function getJson(config, key) {
  const res = await config.client.send(new GetObjectCommand({ Bucket: config.bucket, Key: key }))
  const text = await res.Body.transformToString()
  return JSON.parse(text)
}

async function promoteRelease({ flags, dryRun }) {
  const tag = normalizeTag(requireFlag(flags, 'tag'))
  const channel = readChannel(flags)
  const requestedPlatforms = flags.has('platforms')
  const platforms = String(flags.get('platforms') || '')
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean)
  if (requestedPlatforms && platforms.length === 0) {
    throw new Error('--platforms must select at least one platform')
  }
  for (const platform of platforms) {
    if (!PLATFORMS.includes(platform)) throw new Error(`Unsupported platform in --platforms: ${platform}`)
  }

  const publicationAuthority = await requireReleasePublicationAuthority(flags, {
    tag,
    channel,
    platforms: requestedPlatforms ? platforms : undefined,
    mayIncludeMac: true
  })
  if (!requestedPlatforms) platforms.push(...publicationAuthority.platforms)

  const config = readConfig({ dryRun: false })
  const releaseKeys = await listReleaseKeys(config, tag, channel)
  if (!releaseKeys.length) throw new Error(`No archived R2 objects found for ${tag}`)

  if (!platforms.length) {
    throw new Error(
      `No platform manifests found for ${tag}. Run upload for at least one platform before promoting.`
    )
  }

  const platformManifests = []
  for (const platform of platforms) {
    const key = `${channelBasePath(config.prefix, channel)}/releases/${tag}/release-${platform}.json`
    if (!releaseKeys.includes(key)) {
      throw new Error(`Missing ${key}. Run upload for ${platform} before promoting.`)
    }
    const manifest = await getJson(config, key)
    const authorized = new Map(
      publicationAuthority.authority.artifacts
        .filter((entry) => entry.platform === platform)
        .map((entry) => [entry.fileName, entry])
    )
    const manifestFileNames = Array.isArray(manifest?.files)
      ? manifest.files.map((file) => file?.fileName)
      : []
    const authorizedFileNames = [...authorized.keys()].sort()
    if (!manifest || manifest.schemaVersion !== 1 || manifest.productName !== PRODUCT_NAME ||
      manifest.tag !== tag || manifest.channel !== channel || manifest.platform !== platform ||
      manifest.version !== tag.slice(1) || !Array.isArray(manifest.files) ||
      canonicalJson([...manifestFileNames].sort()) !== canonicalJson(authorizedFileNames) ||
      new Set(manifestFileNames).size !== manifestFileNames.length ||
      manifest.files.some((file) => {
        const receipt = authorized.get(file?.fileName)
        return !receipt || file.size !== receipt.byteLength || file.sha256 !== receipt.sha256 ||
          typeof file.key !== 'string' ||
          file.key !== `${channelBasePath(config.prefix, channel)}/releases/${tag}/${file.fileName}`
      }) || !Array.isArray(manifest.downloads) || manifest.downloads.some((download) => {
        const receipt = authorized.get(download?.fileName)
        return !receipt || !isSafeFileName(download.fileName) ||
          download.size !== receipt.byteLength || download.sha256 !== receipt.sha256
      })) {
      throw new Error(`archived_release_manifest_authority_mismatch: ${platform}`)
    }
    platformManifests.push(manifest)
  }

  const allFiles = new Map()
  for (const manifest of platformManifests) {
    for (const file of manifest.files) {
      allFiles.set(file.fileName, file)
    }
  }

  const latestTargets = [{ basePath: channelBasePath(config.prefix, channel), label: `${channel} latest` }]

  console.log(`Promoting ${PRODUCT_NAME} ${tag} to R2 ${channel} latest (${platforms.join(', ')})`)
  for (const target of latestTargets) {
    console.log(`Target: ${target.label}`)
    for (const file of allFiles.values()) {
      const toKey = `${target.basePath}/latest/${file.fileName}`
      await copyObject({
        config,
        fromKey: file.key,
        toKey,
        type: file.contentType,
        dryRun
      })
      console.log(`  ${file.fileName}`)
    }
  }

  const versions = new Set(platformManifests.map((manifest) => manifest.version))
  if (versions.size > 1) {
    throw new Error(`Cannot promote mixed versions: ${Array.from(versions).join(', ')}`)
  }
  const version = platformManifests[0].version
  const releaseDates = platformManifests
    .map((manifest) => manifest.releaseDate)
    .filter(Boolean)
    .sort()
  const releaseDate = releaseDates[releaseDates.length - 1] ?? new Date().toISOString()

  for (const target of latestTargets) {
    const downloads = platformManifests.flatMap((manifest) =>
      manifest.downloads.map((download) => ({
        ...download,
        url: joinUrl(config.publicBaseUrl, target.basePath, 'latest', download.fileName)
      }))
    )

    const latestManifest = {
      schemaVersion: 1,
      productName: PRODUCT_NAME,
      channel,
      version,
      tag,
      releaseDate,
      generatedAt: new Date().toISOString(),
      releaseUrl: '',
      updateBaseUrl: joinUrl(config.publicBaseUrl, target.basePath, 'latest') + '/',
      updateMetadata: Object.fromEntries(
        platformManifests.map((manifest) => [
          manifest.platform,
          {
            fileName: manifest.updateMetadata.fileName,
            url: joinUrl(config.publicBaseUrl, target.basePath, 'latest', manifest.updateMetadata.fileName)
          }
        ])
      ),
      downloads
    }

    const latestKey = `${target.basePath}/latest/latest.json`
    await putObject({
      config,
      key: latestKey,
      body: JSON.stringify(latestManifest, null, 2),
      contentType: 'application/json; charset=utf-8',
      cacheControl: 'public, max-age=60, must-revalidate',
      dryRun
    })
    console.log(`  ${target.label}/latest.json`)
    console.log(`Latest manifest: ${joinUrl(config.publicBaseUrl, target.basePath, 'latest', 'latest.json')}`)
  }
}

async function main() {
  const { command, flags } = readArgs(process.argv.slice(2))
  if (flags.has('help') || flags.has('h') || !command) {
    usage()
    return
  }
  const dryRun = flags.has('dry-run')

  if (command === 'verify') {
    const tag = normalizeTag(requireFlag(flags, 'tag'))
    const channel = readChannel(flags)
    const platformFlag = flags.get('platform')
    const platformList = flags.get('platforms')
    if (platformFlag && platformList) throw new Error('Use only one of --platform or --platforms.')
    const platforms = String(platformList || platformFlag || '')
      .split(',')
      .map((platform) => platform.trim())
      .filter(Boolean)
    if (platforms.length === 0 || platforms.some((platform) => !PLATFORMS.includes(platform))) {
      throw new Error(`--platform or --platforms must select: ${PLATFORMS.join(', ')}`)
    }
    const distDirs = Object.fromEntries(platforms.map((platform) => [
      platform,
      resolve(flags.get('dist') || PLATFORM_SPECS[platform].distDir || 'dist')
    ]))
    const verified = await requireReleasePublicationAuthority(flags, {
      tag,
      channel,
      platforms,
      distDirs
    })
    console.log(
      `Verified controlled release publication authority ${verified.authority.receiptDigest} for ${platforms.join(',')}`
    )
    return
  }

  if (command === 'upload') {
    await uploadPlatform({ flags, dryRun })
    return
  }
  if (command === 'promote') {
    await promoteRelease({ flags, dryRun })
    return
  }
  throw new Error(`Unknown command: ${command}`)
}

const invokedPath = process.argv[1] ? resolve(process.argv[1]) : ''
if (invokedPath === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`[publish-r2] ${error instanceof Error ? error.message : String(error)}`)
    process.exitCode = 1
  })
}

export {
  controlledReleaseAuthorityPublicKeySha256,
  RELEASE_PUBLICATION_AUTHORITY_CONTRACT,
  releasePublicationAuthorityDigest,
  releasePublicationAuthoritySignaturePayload,
  validateReleasePublicationAuthorityShape,
  verifyReleasePublicationAuthority
}
