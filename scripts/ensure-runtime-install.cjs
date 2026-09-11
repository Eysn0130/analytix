const { existsSync, readFileSync, writeFileSync, renameSync, rmSync } = require('node:fs')
const { spawnSync } = require('node:child_process')
const { createHash } = require('node:crypto')
const { join, resolve } = require('node:path')

function installationInput(runtime) {
  const manifest = readFileSync(join(runtime, 'package.json'), 'utf8')
  const dependencies = JSON.parse(manifest).dependencies || {}
  const hash = createHash('sha256').update(JSON.stringify({
    schema: 1, platform: process.platform, arch: process.arch, abi: process.versions.modules
  })).update(manifest).update(readFileSync(join(runtime, 'package-lock.json')))
  // npm file dependencies are linked sources. Their manifest may change after
  // pull without changing the parent lockfile; refresh their dependency graph.
  for (const [name, version] of Object.entries(dependencies).sort()) {
    if (version.startsWith('file:')) {
      hash.update(name).update(readFileSync(resolve(runtime, version.slice(5), 'package.json')))
    }
  }
  return { digest: hash.digest('hex'), dependencies }
}

function ensureAnalytixInstall(repoRoot = resolve(__dirname, '..')) {
  const runtime = join(repoRoot, 'packages/runtime')
  const stamp = join(runtime, 'node_modules/.analytix-install.json')
  const input = installationInput(runtime)
  let recorded = ''
  try { recorded = JSON.parse(readFileSync(stamp, 'utf8')).digest } catch {}
  const installed = () => Object.keys(input.dependencies).filter(name => name !== 'better-sqlite3')
    .every(name => existsSync(join(runtime, 'node_modules', name, 'package.json')))
  if (!installed() || recorded !== input.digest) {
    rmSync(stamp, { force: true })
    const result = spawnSync(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['ci'], {
      cwd: runtime,
      stdio: 'inherit',
      shell: process.platform === 'win32',
      env: { ...process.env, npm_config_audit: 'false', npm_config_fund: 'false' }
    })
    if (result.error || result.signal || result.status !== 0) {
      throw new Error('[runtime-deps] npm ci failed; runtime dependencies remain unverified')
    }
    if (installationInput(runtime).digest !== input.digest) {
      throw new Error('[runtime-deps] Dependency inputs changed during installation; retry after edits finish')
    }
    if (!installed()) throw new Error('[runtime-deps] npm ci did not produce the required dependencies')
    const temporary = `${stamp}.${process.pid}.tmp`
    writeFileSync(temporary, JSON.stringify({ schemaVersion: 1, digest: input.digest }) + '\n', { mode: 0o600 })
    renameSync(temporary, stamp)
  }
  // Keep the existing Electron-ABI SQLite ownership at the app root.
  rmSync(join(runtime, 'node_modules/better-sqlite3'), { recursive: true, force: true })
}

module.exports = { ensureAnalytixInstall, installationInput }
if (require.main === module) {
  try { ensureAnalytixInstall() } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
