const {
  policy,
  requireOfficialTeamIdentifier,
  strictNativeRelativePath
} = require('./macos-signing-policy.cjs')
const { lstatSync, realpathSync } = require('node:fs')
const { basename, dirname, join, relative, resolve, sep } = require('node:path')
const { spawnSync } = require('node:child_process')
const { CORE_DISPOSITION, releaseProfile, assertCoreResourcesAbsent } = require('./core-package-profile.cjs')

const NODE_PTY_SPAWN_HELPER_RELATIVE_PATHS = Object.freeze([
  'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper',
  'Contents/MacOS/analytix-node-pty/prebuilds/darwin-x64/spawn-helper'
])

function nodePtySpawnHelperRelativePath(appPath, filePath) {
  const relativePath = relative(resolve(appPath), resolve(filePath)).split(sep).join('/')
  return NODE_PTY_SPAWN_HELPER_RELATIVE_PATHS.includes(relativePath) ? relativePath : ''
}

function strictNodePtySpawnHelper(appPath) {
  const candidates = []
  for (const relativePath of NODE_PTY_SPAWN_HELPER_RELATIVE_PATHS) {
    const filePath = join(appPath, ...relativePath.split('/'))
    try {
      candidates.push({ filePath, relativePath, stat: lstatSync(filePath) })
    } catch (error) {
      if (error?.code !== 'ENOENT') throw error
    }
  }
  if (candidates.length !== 1) {
    throw new Error('[mac-sign] Packaged node-pty spawn-helper inventory is invalid')
  }
  const helper = candidates[0]
  const realRelativePath = relative(
    realpathSync(appPath),
    realpathSync(helper.filePath)
  ).split(sep).join('/')
  if (
    helper.stat.isSymbolicLink() || !helper.stat.isFile() || helper.stat.nlink !== 1 ||
    (helper.stat.mode & 0o022) !== 0 || (helper.stat.mode & 0o111) === 0 ||
    realRelativePath !== helper.relativePath
  ) {
    throw new Error('[mac-sign] Packaged node-pty spawn-helper must be a canonical executable')
  }
  return helper
}

function runCodesign(spawn, args, errorMessage) {
  const result = spawn('/usr/bin/codesign', args, {
    encoding: 'utf8',
    maxBuffer: 1024 * 1024
  })
  if (result.error || result.signal || result.status !== 0) {
    throw new Error(errorMessage)
  }
  return result
}

function verifyNodePtySpawnHelperSignature(helper, spawn = spawnSync) {
  runCodesign(
    spawn,
    ['--verify', '--strict', '--verbose=2', helper.filePath],
    '[mac-sign] Packaged node-pty spawn-helper signature is invalid'
  )
  const details = runCodesign(
    spawn,
    ['--display', '--verbose=2', helper.filePath],
    '[mac-sign] Packaged node-pty spawn-helper signature details are unavailable'
  )
  if (!/flags=0x[0-9a-f]+\([^)]*runtime[^)]*\)/iu.test(details.stderr || '')) {
    throw new Error('[mac-sign] Packaged node-pty spawn-helper lacks Hardened Runtime')
  }
  const entitlements = runCodesign(
    spawn,
    ['--display', '--entitlements', ':-', helper.filePath],
    '[mac-sign] Packaged node-pty spawn-helper entitlement inspection failed'
  )
  if ((entitlements.stdout || '').trim()) {
    throw new Error('[mac-sign] Packaged node-pty spawn-helper must not carry entitlements')
  }
}

function signNodePtySpawnHelper(options, spawn = spawnSync) {
  const helper = strictNodePtySpawnHelper(options.app)
  const timestamp = options.identity === '-' ? '--timestamp=none' : '--timestamp'
  if (
    options.keychain != null &&
    (typeof options.keychain !== 'string' || !options.keychain.trim())
  ) {
    throw new Error('[mac-sign] Packaged node-pty signing keychain is invalid')
  }
  const keychain = options.keychain ? ['--keychain', options.keychain] : []
  runCodesign(
    spawn,
    [
      '--force', '--sign', options.identity, '--options', 'runtime', timestamp,
      ...keychain, helper.filePath
    ],
    '[mac-sign] Packaged node-pty spawn-helper signing failed'
  )
  verifyNodePtySpawnHelperSignature(helper, spawn)
  return helper
}

function strictOptionsForFile(options, filePath, defaultOptionsForFile, strictSignedFiles) {
  const defaults = defaultOptionsForFile(filePath) || {}
  const strictPath = strictNativeRelativePath(options.app, filePath)
  if (!strictPath) {
    return options.identity === '-' ? { ...defaults, timestamp: 'none' } : defaults
  }
  const stat = lstatSync(filePath)
  const realRelativePath = relative(realpathSync(options.app), realpathSync(filePath)).split(sep).join('/')
  if (
    stat.isSymbolicLink() || !stat.isFile() || stat.nlink !== 1 ||
    (stat.mode & 0o022) !== 0 || realRelativePath !== strictPath
  ) {
    throw new Error(`[mac-sign] Strict native code must be a regular canonical file: ${strictPath}`)
  }
  strictSignedFiles.set(strictPath, (strictSignedFiles.get(strictPath) || 0) + 1)
  return {
    ...defaults,
    entitlements: policy.nativeEntitlementsAbsolutePath,
    hardenedRuntime: true,
    requirements: undefined,
    signatureFlags: ['runtime'],
    timestamp: options.identity === '-' ? 'none' : defaults.timestamp,
    additionalArguments: []
  }
}

function validateStrictSignedFiles(strictSignedFiles, profile = 'full') {
  const expected = releaseProfile(profile) === 'core'
    ? policy.strictNativeRelativePaths.slice(0, 1)
    : policy.strictNativeRelativePaths
  if (
    strictSignedFiles.size !== expected.length ||
    expected.some((path) => strictSignedFiles.get(path) !== 1)
  ) {
    throw new Error('[mac-sign] Strict native code inventory was not signed exactly once')
  }
}

function validateCoreSigningProfile(options, profile, readAuthority) {
  if (releaseProfile(profile) !== 'core') return ''
  if (options.identity !== '-') throw Error('core_profile_signing_authority_mismatch')
  // The normal after-pack owner has already issued the canonical authority.
  // Reuse its bounded parser; the environment alone cannot shrink the inventory.
  readAuthority ||= require('./after-pack.cjs')._internals.readPackagedBuildAuthorityV2
  const authority = readAuthority({
    appOutDir: dirname(options.app), electronPlatformName: 'darwin', arch: 'arm64',
    packager: { appInfo: { productFilename: basename(options.app, '.app') } }
  })
  if (authority.nativeDisposition.kind !== CORE_DISPOSITION) {
    throw Error('core_profile_signing_authority_mismatch')
  }
  assertCoreResourcesAbsent(join(options.app, 'Contents', 'Resources'))
  return authority.authorityDigest
}

async function signMacApp(options) {
  if (!options || !options.app || !options.identity || typeof options.optionsForFile !== 'function') {
    throw new Error('[mac-sign] Electron signing input is incomplete')
  }
  if (options.identity !== '-') {
    requireOfficialTeamIdentifier()
  }
  const profile = releaseProfile(process.env.ANALYTIX_RELEASE_PROFILE)
  const coreAuthorityDigest = validateCoreSigningProfile(options, profile)
  const nodePtySpawnHelper = signNodePtySpawnHelper(options)
  const priorIgnore = options.ignore == null
    ? []
    : Array.isArray(options.ignore)
      ? [...options.ignore]
      : [options.ignore]
  if (priorIgnore.some((entry) =>
    typeof entry !== 'function' && typeof entry !== 'string' && !(entry instanceof RegExp))) {
    throw new Error('[mac-sign] Electron signing ignore policy is invalid')
  }
  // @electron/osx-sign normalizes a scalar ignore into an array, while its
  // current array normalization drops an already-array value. Supply one
  // fail-closed predicate and preserve every caller rule inside it.
  options.ignore = (filePath) =>
    resolve(filePath) === resolve(nodePtySpawnHelper.filePath) ||
    priorIgnore.some((entry) =>
      typeof entry === 'function' ? entry(filePath) : Boolean(filePath.match(entry)))
  const defaultOptionsForFile = options.optionsForFile
  const strictSignedFiles = new Map()
  options.preAutoEntitlements = false
  options.optionsForFile = (filePath) => strictOptionsForFile(
    options,
    filePath,
    defaultOptionsForFile,
    strictSignedFiles
  )
  // Load the pinned project dependency only on the macOS signing path. This
  // keeps configuration/unit-test imports free of ambient npx module lookup.
  const { signAsync } = require('@electron/osx-sign')
  await signAsync(options)
  validateStrictSignedFiles(strictSignedFiles, profile)
  if (validateCoreSigningProfile(options, profile) !== coreAuthorityDigest) {
    throw Error('core_profile_signing_authority_changed')
  }
  verifyNodePtySpawnHelperSignature(nodePtySpawnHelper)
}

module.exports = signMacApp
module.exports._internals = {
  signMacApp,
  nodePtySpawnHelperRelativePath,
  strictNodePtySpawnHelper,
  signNodePtySpawnHelper,
  verifyNodePtySpawnHelperSignature,
  strictOptionsForFile,
  validateCoreSigningProfile,
  validateStrictSignedFiles
}
