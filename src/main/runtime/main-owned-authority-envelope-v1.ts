import { createHash } from 'node:crypto'
import { isAbsolute, normalize } from 'node:path'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'

const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const AUTHORITY_ERROR = 'Managed runtime authority is unavailable.'
const AUTHORITY_ANCHOR_MAX_BYTES = 8 << 10

export type MainOwnedRuntimeAuthorityEnvelopeV1 = Readonly<{
  schemaVersion: 1
  purpose: 'analytix.runtime-main-owned-authority/v1'
  authorityAnchorV1: string
  authorityManifestRoot: string
  authorityCredentialProfileRoot: string
  authorityCredentialBundleRoot: string
}>

export type MainOwnedRuntimeAuthoritySourceV1 = Readonly<{
  take: () => Promise<MainOwnedRuntimeAuthorityEnvelopeV1 | null>
}>

let configuredSourceV1: MainOwnedRuntimeAuthoritySourceV1 | null = null

/**
 * Installs the main-process-only source selected by the Electron host. The
 * authority document and its fields are never accepted from renderer input,
 * ordinary settings, runtime argv, or authority-named environment variables.
 */
export function configureMainOwnedRuntimeAuthoritySourceV1(
  source: MainOwnedRuntimeAuthoritySourceV1 | null
): void {
  if (source !== null && (!source || typeof source.take !== 'function')) {
    throw fixedAuthorityErrorV1()
  }
  configuredSourceV1 = source
}

/**
 * Takes a fresh, validated snapshot for one runtime launch. The source remains
 * installed so a restart must obtain and revalidate a new snapshot.
 */
export async function takeMainOwnedRuntimeAuthorityEnvelopeV1(): Promise<
  MainOwnedRuntimeAuthorityEnvelopeV1 | null
> {
  const source = configuredSourceV1
  if (!source) return null
  let candidate: MainOwnedRuntimeAuthorityEnvelopeV1 | null
  try {
    candidate = await source.take()
  } catch {
    throw fixedAuthorityErrorV1()
  }
  return candidate === null ? null : validateMainOwnedRuntimeAuthorityEnvelopeV1(candidate)
}

export function validateMainOwnedRuntimeAuthorityEnvelopeV1(
  candidate: MainOwnedRuntimeAuthorityEnvelopeV1
): MainOwnedRuntimeAuthorityEnvelopeV1 {
  if (!candidate || typeof candidate !== 'object' ||
    Object.keys(candidate).sort().join('\n') !== [
      'authorityAnchorV1',
      'authorityCredentialBundleRoot',
      'authorityCredentialProfileRoot',
      'authorityManifestRoot',
      'purpose',
      'schemaVersion'
    ].join('\n') ||
    candidate.schemaVersion !== 1 ||
    candidate.purpose !== 'analytix.runtime-main-owned-authority/v1') {
    throw fixedAuthorityErrorV1()
  }
  const authorityAnchorV1 = validateCanonicalAuthorityAnchorV1(candidate.authorityAnchorV1)
  const roots = [
    validateExactAbsolutePathV1(candidate.authorityManifestRoot),
    validateExactAbsolutePathV1(candidate.authorityCredentialProfileRoot),
    validateExactAbsolutePathV1(candidate.authorityCredentialBundleRoot)
  ]
  if (new Set(roots).size !== roots.length) throw fixedAuthorityErrorV1()
  return Object.freeze({
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1,
    authorityManifestRoot: roots[0],
    authorityCredentialProfileRoot: roots[1],
    authorityCredentialBundleRoot: roots[2]
  })
}

export function authorityAnchorProjectionV1(authorityAnchorV1: string): Readonly<{
  installationId: string
  authorityKeyId: string
  authorityPublicKey: string
  currentManifestDigest: string
}> {
  const parsed = parseAuthorityAnchorV1(authorityAnchorV1)
  return Object.freeze({
    installationId: String(parsed.installationId),
    authorityKeyId: String(parsed.authorityKeyId),
    authorityPublicKey: String(parsed.authorityPublicKey),
    currentManifestDigest: String(parsed.currentManifestDigest)
  })
}

function validateCanonicalAuthorityAnchorV1(value: string): string {
  const parsed = parseAuthorityAnchorV1(value)
  const publicKey = Buffer.from(String(parsed.authorityPublicKey), 'base64url')
  try {
    const canonical = JSON.stringify({
      schemaVersion: 1,
      installationId: parsed.installationId,
      authorityKeyId: parsed.authorityKeyId,
      authorityPublicKey: parsed.authorityPublicKey,
      currentManifestDigest: parsed.currentManifestDigest
    })
    if (canonical !== value ||
      !SHA256_PATTERN.test(String(parsed.installationId)) ||
      !SHA256_PATTERN.test(String(parsed.authorityKeyId)) ||
      !SHA256_PATTERN.test(String(parsed.currentManifestDigest)) ||
      publicKey.length !== 32 ||
      publicKey.toString('base64url') !== parsed.authorityPublicKey ||
      createHash('sha256').update(publicKey).digest('hex') !== parsed.authorityKeyId) {
      throw fixedAuthorityErrorV1()
    }
    return value
  } finally {
    publicKey.fill(0)
  }
}

function parseAuthorityAnchorV1(value: string): Record<string, unknown> {
  if (typeof value !== 'string' || value === '' || value !== value.trim() ||
    Buffer.byteLength(value, 'utf8') > AUTHORITY_ANCHOR_MAX_BYTES) {
    throw fixedAuthorityErrorV1()
  }
  try {
    const parsed = parseStrictJsonObject(Buffer.from(value, 'utf8'), {
      maxBytes: AUTHORITY_ANCHOR_MAX_BYTES,
      maxDepth: 2,
      maxTokens: 16,
      maxStringBytes: 256,
      maxNumberBytes: 8
    })
    const keys = Object.keys(parsed).sort()
    if (keys.length !== 5 ||
      keys[0] !== 'authorityKeyId' ||
      keys[1] !== 'authorityPublicKey' ||
      keys[2] !== 'currentManifestDigest' ||
      keys[3] !== 'installationId' ||
      keys[4] !== 'schemaVersion' ||
      parsed.schemaVersion !== 1) {
      throw fixedAuthorityErrorV1()
    }
    return parsed
  } catch {
    throw fixedAuthorityErrorV1()
  }
}

function validateExactAbsolutePathV1(value: string): string {
  if (typeof value !== 'string' || value === '' || value !== value.trim() ||
    !isAbsolute(value) || normalize(value) !== value || value.includes('\0')) {
    throw fixedAuthorityErrorV1()
  }
  return value
}

function fixedAuthorityErrorV1(): Error {
  return new Error(AUTHORITY_ERROR)
}
