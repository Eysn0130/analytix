const { readFileSync, writeFileSync, renameSync, rmSync } = require('node:fs')
const { join } = require('node:path')
const { installationInput } = require('./ensure-runtime-install.cjs')
const { createHash } = require('node:crypto')

function appInstallationDigest(root) {
  const hash = createHash('sha256').update(installationInput(root).digest)
  // A pull can change native/dependency postinstall behavior without changing
  // dependency versions. Such a pull must invalidate the old installation too.
  for (const file of ['postinstall.cjs', 'ensure-runtime-install.cjs', 'app-install-state.cjs']) {
    hash.update(file).update('\0')
    try { hash.update(readFileSync(join(root, 'scripts', file))) } catch (error) {
      if (error.code !== 'ENOENT') throw error
      hash.update('<absent>')
    }
  }
  return hash.digest('hex')
}

function installedAppDependencies(root) {
  const manifest = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'))
  const lock = JSON.parse(readFileSync(join(root, 'package-lock.json'), 'utf8'))
  return Object.entries({ ...manifest.dependencies, ...manifest.devDependencies }).every(([name, spec]) => {
    try {
      const installed = JSON.parse(readFileSync(join(root, 'node_modules', name, 'package.json'), 'utf8'))
      if (spec.startsWith('file:')) {
        const source = JSON.parse(readFileSync(join(root, spec.slice(5), 'package.json'), 'utf8'))
        return Boolean(installed.version) && installed.version === source.version
      }
      return Boolean(installed.version) && installed.version === lock.packages[`node_modules/${name}`]?.version
    } catch { return false }
  })
}

function appInstallCurrent(root) {
  try {
    const stamp = JSON.parse(readFileSync(join(root, 'node_modules/.analytix-install.json'), 'utf8'))
    return stamp.digest === appInstallationDigest(root) && installedAppDependencies(root)
  } catch { return false }
}

function beginAppInstall(root) {
  const digest = appInstallationDigest(root)
  rmSync(join(root, 'node_modules/.analytix-install.json'), { force: true })
  return digest
}

// Called only after the root postinstall pipeline succeeds. A doctor check
// never stamps or mutates an installation merely because node_modules exists.
function recordAppInstall(root, expectedDigest) {
  if (appInstallationDigest(root) !== expectedDigest || !installedAppDependencies(root)) {
    throw new Error('[app-deps] Installation is incomplete or inputs changed; run npm ci after edits finish.')
  }
  const stamp = join(root, 'node_modules/.analytix-install.json')
  const temporary = `${stamp}.${process.pid}.tmp`
  writeFileSync(temporary, JSON.stringify({ schemaVersion: 1, digest: expectedDigest }) + '\n', { mode: 0o600 })
  renameSync(temporary, stamp)
}

module.exports = { appInstallationDigest, appInstallCurrent, beginAppInstall, recordAppInstall }
