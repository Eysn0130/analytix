const crypto = require('node:crypto')
const { lstatSync, readFileSync, realpathSync } = require('node:fs')
const { isAbsolute, join, relative, resolve, sep } = require('node:path')

const repoRoot = join(__dirname, '..')
const policyPath = join(__dirname, 'macos-signing-policy.json')
const policyBytes = readFileSync(policyPath)
const policy = JSON.parse(policyBytes.toString('utf8'))
const expectedKeys = [
  'schemaVersion',
  'officialTeamIdentifier',
  'nativeEntitlementsPath',
  'nativeEntitlementsSha256',
  'strictNativeRelativePaths'
]

function assertNoSymlinkPath(path, boundary) {
  const resolvedBoundary = resolve(boundary)
  const resolvedPath = resolve(path)
  const relativePath = relative(resolvedBoundary, resolvedPath)
  if (relativePath === '..' || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error('[mac-signing-policy] Native entitlement policy escapes the repository')
  }
  let current = resolvedBoundary
  if (lstatSync(current).isSymbolicLink()) {
    throw new Error('[mac-signing-policy] Repository boundary cannot be a symbolic link')
  }
  for (const part of relativePath.split(sep).filter(Boolean)) {
    current = join(current, part)
    if (lstatSync(current).isSymbolicLink()) {
      throw new Error('[mac-signing-policy] Native entitlement policy path cannot contain a symbolic link')
    }
  }
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value)
  return keys.length === expected.length && keys.every((key) => expected.includes(key))
}

function sha256(bytes) {
  return crypto.createHash('sha256').update(bytes).digest('hex')
}

function validatePolicy(value = policy) {
  if (!exactKeys(value, expectedKeys) || value.schemaVersion !== 1 ||
    (value.officialTeamIdentifier !== null && !/^[A-Z0-9]{10}$/.test(value.officialTeamIdentifier)) ||
    typeof value.nativeEntitlementsPath !== 'string' || isAbsolute(value.nativeEntitlementsPath) ||
    !/^[0-9a-f]{64}$/.test(value.nativeEntitlementsSha256 || '') ||
    !Array.isArray(value.strictNativeRelativePaths) || value.strictNativeRelativePaths.length !== 5) {
    throw new Error('[mac-signing-policy] Signing policy schema is invalid')
  }
  const expectedPaths = [
    'Contents/Resources/runtime-go/bin/runtime-server',
    'Contents/Resources/runtime/analytix-import-accelerator',
    'Contents/Resources/runtime/analytix-cleaning-ops',
    'Contents/Resources/runtime/analytix-analysis-compute',
    'Contents/Resources/runtime/analytix-data-engine'
  ]
  if (JSON.stringify(value.strictNativeRelativePaths) !== JSON.stringify(expectedPaths)) {
    throw new Error('[mac-signing-policy] Strict native code inventory is not frozen')
  }
  const entitlementsPath = resolve(repoRoot, value.nativeEntitlementsPath)
  assertNoSymlinkPath(entitlementsPath, repoRoot)
  const pathFromRoot = relative(realpathSync(repoRoot), realpathSync(entitlementsPath))
  const stat = lstatSync(entitlementsPath)
  if (pathFromRoot === '..' || pathFromRoot.startsWith(`..${sep}`) || isAbsolute(pathFromRoot) ||
    stat.isSymbolicLink() || !stat.isFile() || stat.nlink !== 1 || stat.mode & 0o022 ||
    sha256(readFileSync(entitlementsPath)) !== value.nativeEntitlementsSha256) {
    throw new Error('[mac-signing-policy] Native entitlement policy identity is invalid')
  }
  return { ...value, nativeEntitlementsAbsolutePath: entitlementsPath }
}

function requireOfficialTeamIdentifier(value = policy) {
  const validated = validatePolicy(value)
  if (!validated.officialTeamIdentifier) {
    throw new Error('[mac-signing-policy] Official Apple Team ID is not configured; Developer ID release is blocked')
  }
  return validated.officialTeamIdentifier
}

function strictNativeRelativePath(appPath, filePath, value = policy) {
  const validated = validatePolicy(value)
  const relativePath = relative(resolve(appPath), resolve(filePath)).split(sep).join('/')
  return validated.strictNativeRelativePaths.includes(relativePath) ? relativePath : ''
}

const validatedPolicy = validatePolicy(policy)

module.exports = {
  policy: validatedPolicy,
  policyBytes,
  policyDigest: sha256(policyBytes),
  policyPath,
  requireOfficialTeamIdentifier,
  strictNativeRelativePath,
  validatePolicy
}
