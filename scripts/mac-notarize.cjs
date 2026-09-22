const { execFileSync, spawnSync } = require('node:child_process')
const { existsSync, lstatSync, mkdtempSync, readdirSync, rmSync, writeFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const { join } = require('node:path')
const {
  targetContract: nativeTargetContract,
  verifyDarwinCodeSignature,
  verifyPackagedComponents
} = require('./native-component-contract.cjs')
const {
  policy: macSigningPolicy,
  requireOfficialTeamIdentifier,
  strictNativePathsForProfile
} = require('./macos-signing-policy.cjs')
const { CORE_DISPOSITION, assertCoreResourcesAbsent } = require('./core-package-profile.cjs')
const {
  NATIVE_DISPOSITION_CONTROLLED_RELEASE,
  NATIVE_DISPOSITION_DEVELOPMENT,
  _internals: packagedAuthorityContract
} = require('./after-pack.cjs')

function normalizeBuilderArch(value) {
  if (value === 1 || value === '1' || value === 'x64' || value === 'amd64') return 'x64'
  if (value === 3 || value === '3' || value === 'arm64' || value === 'aarch64') return 'arm64'
  throw new Error(`[mac-notarize] Unsupported package architecture: ${value}`)
}

function appBundlePath(context) {
  return join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`)
}

function assertNativeDispositionSigningBoundary(authority, options = {}) {
  if (!packagedAuthorityContract.isPackagedBuildAuthorityV2(authority)) {
    throw new Error('[mac-notarize] Packaged build authority is invalid')
  }
  const disposition = authority.nativeDisposition
  if (disposition.kind === NATIVE_DISPOSITION_DEVELOPMENT || disposition.kind === CORE_DISPOSITION) {
    if (options.requireDeveloperID === true) {
      throw new Error('[mac-notarize] development_non_publishable cannot enter Developer ID or notarization')
    }
    return disposition.kind
  }
  if (disposition.kind !== NATIVE_DISPOSITION_CONTROLLED_RELEASE) {
    throw new Error('[mac-notarize] Native disposition is unsupported')
  }
  if (options.requireDeveloperID === true && disposition.signingMode !== 'developer-id') {
    throw new Error('[mac-notarize] Developer ID signing requires a controlled developer-id receipt')
  }
  return disposition.kind
}

function verifyDataNativeAfterSign(context, options = {}) {
  const appBundle = appBundlePath(context)
  const runtimeDir = join(appBundle, 'Contents', 'Resources', 'runtime')
  const target = nativeTargetContract('darwin', normalizeBuilderArch(context.arch))
  const authority = packagedAuthorityContract.readPackagedBuildAuthorityV2(context)
  const dispositionKind = assertNativeDispositionSigningBoundary(authority, options)
  packagedAuthorityContract.verifyPackagedBuildAuthorityArtifacts(context, authority)
  if (dispositionKind === CORE_DISPOSITION) {
    assertCoreResourcesAbsent(join(appBundle, 'Contents', 'Resources'))
  } else if (dispositionKind === NATIVE_DISPOSITION_DEVELOPMENT) {
    packagedAuthorityContract.verifyPackagedDevelopmentNativeDisposition(
      context,
      authority.nativeDisposition,
      { requireMarkerBinaryIdentity: false }
    )
  } else {
    verifyPackagedComponents(runtimeDir, target, {
      verifyExecution: options.verifyExecution !== false,
      requireBuildIdentity: false,
      authorityUse: options.authorityUse
    })
  }
  const expectedTeamIdentifier = options.requireDeveloperID === true
    ? requireOfficialTeamIdentifier()
    : ''
  const signatureOptions = {
    requireDeveloperID: options.requireDeveloperID === true,
    requireSecureTimestamp: options.requireSecureTimestamp === true,
    requireHardenedRuntime: true,
    expectedTeamIdentifier,
    spawnSync: options.spawnSync
  }
  const appSignature = verifyDarwinCodeSignature(appBundle, signatureOptions)
  for (const relativePath of strictNativePathsForProfile(dispositionKind === CORE_DISPOSITION ? 'core' : 'full')) {
    const nativePath = join(appBundle, ...relativePath.split('/'))
    const nativeSignature = verifyDarwinCodeSignature(nativePath, {
      ...signatureOptions,
      inspectEntitlements: true,
      requireEmptyEntitlements: true
    })
    if (options.requireDeveloperID === true && nativeSignature.teamIdentifier !== appSignature.teamIdentifier) {
      throw new Error(`[mac-notarize] Strict native TeamIdentifier does not match the app: ${relativePath}`)
    }
  }
  return { appSignature, expectedTeamIdentifier, dispositionKind }
}

function getNotaryCredentials() {
  const keys = ['APPLE_API_KEY_ID', 'APPLE_API_ISSUER', 'APPLE_API_KEY', 'APPLE_API_KEY_BASE64']
  if (!keys.some((key) => process.env[key] !== undefined)) {
    return null
  }
  const keyId = String(process.env.APPLE_API_KEY_ID || '').trim()
  const issuer = String(process.env.APPLE_API_ISSUER || '').trim()
  const keyPath = String(process.env.APPLE_API_KEY || '').trim()
  const keyBase64 = String(process.env.APPLE_API_KEY_BASE64 || '').replace(/\s+/gu, '')
  if (!keyId || !issuer || Boolean(keyPath) === Boolean(keyBase64)) {
    throw new Error('[mac-notarize] Apple notary credentials must provide key ID, issuer, and exactly one key source')
  }

  if (keyPath) {
    const stat = lstatSync(keyPath)
    if (stat.isSymbolicLink() || !stat.isFile() || stat.nlink !== 1 || (stat.mode & 0o077) !== 0) {
      throw new Error('[mac-notarize] Apple notary key file must be a private regular file')
    }
    return { keyId, issuer, keyPath, cleanup: null }
  }

  const tempDir = mkdtempSync(join(tmpdir(), 'analytix-notary-'))
  const tempKeyPath = join(tempDir, `AuthKey_${keyId}.p8`)
  const decodedKey = Buffer.from(keyBase64, 'base64')
  if (decodedKey.length === 0 || decodedKey.toString('base64').replace(/=+$/u, '') !== keyBase64.replace(/=+$/u, '')) {
    rmSync(tempDir, { recursive: true, force: true })
    throw new Error('[mac-notarize] APPLE_API_KEY_BASE64 is invalid')
  }
  writeFileSync(tempKeyPath, decodedKey, { mode: 0o600 })

  return {
    keyId,
    issuer,
    keyPath: tempKeyPath,
    cleanup: () => rmSync(tempDir, { recursive: true, force: true })
  }
}

function runNotaryToolJson(args) {
  const output = execFileSync('xcrun', ['notarytool', ...args, '--output-format', 'json'], {
    encoding: 'utf8'
  })
  console.log(output.trim())

  try {
    return JSON.parse(output)
  } catch (error) {
    throw new Error(`Failed to parse notarytool JSON output: ${error.message}`)
  }
}

function isBundleLike(path) {
  return /\.(app|appex|bundle|framework|plugin|xpc)$/i.test(path)
}

function isLikelySignedFile(path, info) {
  if (/\.(dylib|node|so)$/i.test(path)) return true
  if (!info.isFile() || (info.mode & 0o111) === 0) return false
  if (/\/Contents\/(?:MacOS|Frameworks)\//.test(path)) return true
  return macSigningPolicy.strictNativeRelativePaths.some(
    (relativePath) => path.endsWith(`/${relativePath}`)
  )
}

function collectSignedCodeCandidates(appBundle) {
  const candidates = new Set([appBundle])
  const stack = [appBundle]

  while (stack.length) {
    const current = stack.pop()
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name)
      const info = lstatSync(path)
      if (info.isSymbolicLink()) continue

      if (info.isDirectory()) {
        if (isBundleLike(path)) candidates.add(path)
        stack.push(path)
        continue
      }

      if (isLikelySignedFile(path, info)) {
        candidates.add(path)
      }
    }
  }

  return Array.from(candidates).sort()
}

function readCodeSignatureDetails(path) {
  const result = spawnSync('/usr/bin/codesign', ['--display', '--verbose=4', path], {
    encoding: 'utf8'
  })
  if (result.error) {
    throw result.error
  }
  const details = `${result.stdout || ''}${result.stderr || ''}`

  if (result.status !== 0) {
    throw new Error(`codesign --display failed for ${path} with status ${result.status}\n${details}`)
  }

  return details
}

function verifySecureTimestamps(appBundle) {
  execFileSync('/usr/bin/codesign', ['--verify', '--deep', '--strict', '--verbose=2', appBundle], {
    stdio: 'inherit'
  })

  const candidates = collectSignedCodeCandidates(appBundle)
  console.log(`[mac-notarize] Verifying secure timestamps for ${candidates.length} signed code candidate(s).`)
  for (const candidate of candidates) {
    const details = readCodeSignatureDetails(candidate)
    if (!/^Timestamp=/m.test(details)) {
      throw new Error(
        `The signature is missing a secure timestamp: ${candidate}. Ensure electron-builder mac.timestamp is enabled.`
      )
    }
  }
}

exports.default = async function afterSign(context) {
  if (context.electronPlatformName !== 'darwin') {
    return
  }

  const appBundle = appBundlePath(context)
  if (!existsSync(appBundle)) {
    throw new Error(`App bundle not found after signing: ${appBundle}`)
  }
  const developerIdSigningEnabled = Boolean(
    process.env.CSC_LINK ||
      process.env.CSC_NAME ||
      process.env.CSC_KEY_PASSWORD ||
      process.env.MAC_SIGN === '1'
  )
  const creds = getNotaryCredentials()
  if (creds && !developerIdSigningEnabled) {
    creds.cleanup?.()
    throw new Error('[mac-notarize] Notarization requires an authorized Developer ID signature')
  }
  verifyDataNativeAfterSign(context, {
    requireDeveloperID: developerIdSigningEnabled,
    requireSecureTimestamp: developerIdSigningEnabled
  })

  if (!creds) {
    console.log('[mac-notarize] Verified signed native payloads; no Apple notary credentials found, skipping notarization.')
    return
  }

  const zipPath = join(context.appOutDir, `${context.packager.appInfo.productFilename}-notary.zip`)

  try {
    verifySecureTimestamps(appBundle)

    execFileSync('ditto', ['-c', '-k', '--sequesterRsrc', '--keepParent', appBundle, zipPath], {
      stdio: 'inherit'
    })

    const submitResult = runNotaryToolJson([
        'submit',
        zipPath,
        '--wait',
        '--key',
        creds.keyPath,
        '--key-id',
        creds.keyId,
        '--issuer',
        creds.issuer
      ])

    if (submitResult.status !== 'Accepted') {
      if (submitResult.id) {
        try {
          const logResult = runNotaryToolJson([
            'log',
            submitResult.id,
            '--key',
            creds.keyPath,
            '--key-id',
            creds.keyId,
            '--issuer',
            creds.issuer
          ])
          console.log(`[mac-notarize] Detailed log URL: ${logResult.developerLogUrl || '<none>'}`)
        } catch (error) {
          console.error(`[mac-notarize] Failed to fetch Apple notary log: ${error.message}`)
        }
      }

      throw new Error(
        `Apple notarization failed with status: ${submitResult.status || 'unknown'}`
      )
    }

    execFileSync('xcrun', ['stapler', 'staple', appBundle], { stdio: 'inherit' })
    execFileSync('xcrun', ['stapler', 'validate', appBundle], { stdio: 'inherit' })
    verifyDataNativeAfterSign(context, { requireDeveloperID: true, requireSecureTimestamp: true })
    execFileSync('/usr/sbin/spctl', ['--assess', '--type', 'execute', '--verbose=4', appBundle], {
      stdio: 'inherit'
    })
  } finally {
    rmSync(zipPath, { force: true })
    creds.cleanup?.()
  }
}

exports._internals = {
  appBundlePath,
  collectSignedCodeCandidates,
  isBundleLike,
  isLikelySignedFile,
  normalizeBuilderArch,
  assertNativeDispositionSigningBoundary,
  verifyDataNativeAfterSign,
  getNotaryCredentials,
  readCodeSignatureDetails
}
