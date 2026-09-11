import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { verifyAssets } from './runtime-assets.mjs'

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const { installationInput } = createRequire(import.meta.url)('./ensure-runtime-install.cjs')

function version(value) {
  const parts = /(?:^|\s|go|v)(\d+)\.(\d+)(?:\.(\d+))?/.exec(value || '')
  return parts ? parts.slice(1).map(part => Number(part || 0)) : []
}

export function toolChecks({ node, npm, go, git }) {
  const n = version(node), p = version(npm), g = version(go)
  return [
    { name: 'Node 22.22.1+', passed: n[0] === 22 && (n[1] > 22 || (n[1] === 22 && n[2] >= 1)) },
    { name: 'npm 10.9.4+', passed: p[0] === 10 && (p[1] > 9 || (p[1] === 9 && p[2] >= 4)) },
    { name: 'Go 1.22+ compatible source toolchain', passed: g[0] === 1 && g[1] >= 22 },
    { name: 'Git', passed: /^git version \d+\./.test(git || '') }
  ]
}

function command(name, args) {
  const result = spawnSync(name === 'npm' && process.platform === 'win32' ? 'npm.cmd' : name, args, {
    cwd: repo, encoding: 'utf8', timeout: 15000, maxBuffer: 65536,
    shell: name === 'npm' && process.platform === 'win32',
    env: { ...process.env, GOTOOLCHAIN: 'local' }, stdio: ['ignore', 'pipe', 'pipe']
  })
  return result.status === 0 ? result.stdout.trim() : ''
}

export function dependencyChecks(root) {
  const checks = ['typescript', 'electron-vite', 'electron'].map(name => ({
    name: `App dependency: ${name}`, passed: existsSync(join(root, 'node_modules', name, 'package.json'))
  }))
  let current = false
  try {
    const runtime = join(root, 'packages/runtime')
    const stamp = JSON.parse(readFileSync(join(runtime, 'node_modules/.analytix-install.json'), 'utf8'))
    const input = installationInput(runtime)
    current = input.digest === stamp.digest && Object.keys(input.dependencies)
      .filter(name => name !== 'better-sqlite3')
      .every(name => existsSync(join(runtime, 'node_modules', name, 'package.json')))
  } catch {}
  checks.push({ name: 'Runtime dependencies match current manifests and lockfile', passed: current })
  return checks
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2)
    if (args.some(arg => arg !== '--native')) throw new Error('Usage: npm run doctor [-- --native]')
    const checks = [
      ...toolChecks({ node: process.version, npm: command('npm', ['--version']), go: command('go', ['version']), git: command('git', ['--version']) }),
      ...dependencyChecks(repo)
    ]
    if (args.includes('--native')) {
      const host = `${process.platform}-${process.arch}`
      const goPin = JSON.parse(readFileSync(join(repo, 'scripts/go-runtime-toolchain.json'), 'utf8')).hosts[host]
      const rust = /channel\s*=\s*"([^"]+)"/.exec(readFileSync(join(repo, 'rust-toolchain.toml'), 'utf8'))?.[1]
      const installedRust = command('rustup', ['toolchain', 'list'])
      checks.push(
        { name: 'Native build authority supports this host', passed: process.platform === 'darwin' && Boolean(goPin) },
        { name: 'Pinned native Go version installed (binary authority checked at build)', passed: Boolean(goPin) && command('go', ['version']).split(' ')[2] === goPin.goVersion },
        { name: 'Pinned Rust toolchain installed', passed: Boolean(rust) && installedRust.split('\n').some(line => line.startsWith(`${rust}-`)) },
        { name: 'Pinned runtime assets present', passed: verifyAssets(repo, host).passed }
      )
    }
    for (const check of checks) console.log(`${check.passed ? 'PASS' : 'FAIL'} ${check.name}`)
    console.log('This checks development inputs, not compilation, native signatures, package acceptance or live Provider behavior.')
    if (checks.some(check => !check.passed)) {
      console.error('Resolve the named input; after pulling dependency changes run npm run bootstrap. See docs/analytix/development-baseline.md.')
      process.exitCode = 1
    }
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
