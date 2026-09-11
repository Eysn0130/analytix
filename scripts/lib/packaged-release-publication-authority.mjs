import { createHash } from 'node:crypto'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'

const require = createRequire(import.meta.url)
const { policy: macSigningPolicy } = require('../macos-signing-policy.cjs')

function inputValue(rawArgs, env, name, envName) {
  const prefix = `${name}=`
  const inline = rawArgs.find((item) => item.startsWith(prefix))
  if (inline) return inline.slice(prefix.length).trim()
  const index = rawArgs.indexOf(name)
  if (index >= 0) {
    const value = String(rawArgs[index + 1] || '').trim()
    return value.startsWith('--') ? '' : value
  }
  return String(env[envName] || '').trim()
}

function isSha256(value) {
  return /^[0-9a-f]{64}$/u.test(value)
}

function releasePlatform(targetPlatform) {
  if (targetPlatform === 'darwin') return 'mac'
  if (targetPlatform === 'win32') return 'win'
  if (targetPlatform === 'linux') return 'linux'
  return ''
}

function safeBlocker(error) {
  const message = error instanceof Error ? error.message : String(error)
  return /^[a-z0-9_]+$/u.test(message)
    ? message
    : 'release_publication_authority_verification_failed'
}

function rejected(requested, blocker, classification = 'verification_failure') {
  const externalPrerequisite = classification === 'external_prerequisite'
  return {
    requested,
    ok: false,
    blocked: requested && externalPrerequisite,
    failed: requested && !externalPrerequisite,
    classification: requested ? classification : 'not_requested',
    blocker,
    contract: '',
    authoritySha256: '',
    receiptDigest: '',
    keyId: '',
    sourceCommit: '',
    tag: '',
    channel: '',
    platform: '',
    targetKey: '',
    packagedAuthoritySha256: '',
    packagedAuthorityDigest: '',
    releaseArtifactCount: 0,
    releaseArtifactSetSha256: ''
  }
}

export function releasePublicationConfiguration(rawArgs, env = process.env) {
  return {
    authorityPath: inputValue(
      rawArgs,
      env,
      '--release-authority',
      'ANALYTIX_RELEASE_AUTHORITY'
    ),
    publicKeyPath: inputValue(
      rawArgs,
      env,
      '--release-authority-public-key',
      'ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY'
    ),
    tag: inputValue(
      rawArgs,
      env,
      '--release-tag',
      'ANALYTIX_RELEASE_TAG'
    ),
    channel: inputValue(
      rawArgs,
      env,
      '--release-channel',
      'ANALYTIX_RELEASE_CHANNEL'
    ),
    distDir: inputValue(
      rawArgs,
      env,
      '--release-dist',
      'ANALYTIX_RELEASE_DIST'
    )
  }
}

export async function verifyPackagedReleasePublicationAuthority(options) {
  if (options.requested !== true) {
    return rejected(false, 'formal_release_publication_authority_not_requested')
  }
  const platform = releasePlatform(options.target?.platform)
  const config = options.config || {}
  if (!platform) return rejected(true, 'release_publication_authority_platform_invalid')
  if (!config.authorityPath) {
    return rejected(
      true,
      'release_publication_authority_missing',
      'external_prerequisite'
    )
  }
  if (!config.publicKeyPath) {
    return rejected(
      true,
      'release_publication_authority_public_key_missing',
      'external_prerequisite'
    )
  }
  if (!/^v\d+\.\d+\.\d+$/u.test(config.tag || '')) {
    return rejected(
      true,
      'release_publication_authority_tag_missing_or_invalid',
      'external_prerequisite'
    )
  }
  if (config.channel !== 'stable' && config.channel !== 'beta') {
    return rejected(
      true,
      'release_publication_authority_channel_missing_or_invalid',
      'external_prerequisite'
    )
  }
  if (!config.distDir) {
    return rejected(
      true,
      'release_publication_authority_dist_missing',
      'external_prerequisite'
    )
  }
  if (!options.candidate?.ok ||
    !isSha256(options.candidate.sha256 || '') ||
    !isSha256(options.candidate.authorityDigest || '') ||
    !/^[0-9a-f]{40}$/u.test(options.candidate.sourceCommit || '') ||
    options.candidate.targetKey !== options.target.key) {
    return rejected(true, 'release_publication_authority_candidate_invalid')
  }
  const officialTeamIdentifier = platform === 'mac'
    ? String(macSigningPolicy.officialTeamIdentifier || '').trim()
    : ''
  if (platform === 'mac' && !/^[A-Z0-9]{10}$/u.test(officialTeamIdentifier)) {
    return rejected(
      true,
      'official_apple_team_identifier_unavailable',
      'external_prerequisite'
    )
  }

  try {
    const {
      controlledReleaseAuthorityPublicKeySha256,
      verifyReleasePublicationAuthority
    } = await import('../publish-r2.mjs')
    const trustedPublicKeySha256 = controlledReleaseAuthorityPublicKeySha256()
    if (!isSha256(trustedPublicKeySha256)) {
      return rejected(
        true,
        'release_publication_authority_trust_anchor_unavailable',
        'external_prerequisite'
      )
    }
    const verified = await verifyReleasePublicationAuthority({
      authorityPath: resolve(config.authorityPath),
      publicKeyPath: resolve(config.publicKeyPath),
      tag: config.tag,
      channel: config.channel,
      sourceCommit: options.candidate.sourceCommit,
      platforms: [platform],
      distDirs: { [platform]: resolve(config.distDir) },
      officialTeamIdentifier,
      trustedPublicKeySha256,
      now: options.now || new Date()
    })
    const packagedEntries = verified.authority.packagedBuildAuthorities.filter((entry) =>
      entry.platform === platform && entry.targetKey === options.target.key
    )
    if (packagedEntries.length !== 1 ||
      packagedEntries[0].sha256 !== options.candidate.sha256 ||
      packagedEntries[0].authorityDigest !== options.candidate.authorityDigest) {
      return rejected(true, 'release_publication_authority_detached_candidate')
    }
    const artifacts = verified.authority.artifacts.filter((entry) => entry.platform === platform)
    return {
      requested: true,
      ok: true,
      blocked: false,
      failed: false,
      classification: 'verified',
      blocker: '',
      contract: String(verified.authority.contract || ''),
      authoritySha256: verified.authoritySha256,
      receiptDigest: String(verified.authority.receiptDigest || ''),
      keyId: String(verified.authority.keyId || ''),
      sourceCommit: String(verified.authority.sourceCommit || ''),
      tag: String(verified.authority.tag || ''),
      channel: String(verified.authority.channel || ''),
      platform,
      targetKey: options.target.key,
      packagedAuthoritySha256: packagedEntries[0].sha256,
      packagedAuthorityDigest: packagedEntries[0].authorityDigest,
      releaseArtifactCount: artifacts.length,
      releaseArtifactSetSha256: createHash('sha256')
        .update(JSON.stringify(artifacts), 'utf8')
        .digest('hex')
    }
  } catch (error) {
    return rejected(true, safeBlocker(error))
  }
}
