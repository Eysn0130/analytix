const { existsSync, rmSync } = require('node:fs')
const { spawnSync } = require('node:child_process')

const REQUIRED_PATHS = [
  'packages/runtime/package-lock.json',
  'packages/runtime/node_modules/diff/package.json',
  'packages/runtime/node_modules/zod/package.json',
  'packages/runtime/node_modules/@modelcontextprotocol/sdk/package.json'
]
const ANALYTIX_SQLITE_MODULE_PATH = 'packages/runtime/node_modules/better-sqlite3'

function run(command, args) {
  return spawnSync(command, args, {
    stdio: 'inherit',
    shell: process.platform === 'win32',
    env: {
      ...process.env,
      npm_config_audit: 'false',
      npm_config_fund: 'false'
    }
  })
}

function ensureAnalytixInstall() {
  if (!REQUIRED_PATHS.every((path) => existsSync(path))) {
    const installAnalytix = run('npm', ['--prefix', 'packages/runtime', 'ci'])
    if (installAnalytix.status !== 0) {
      process.exit(installAnalytix.status || 1)
    }
  }

  if (existsSync(ANALYTIX_SQLITE_MODULE_PATH)) {
    rmSync(ANALYTIX_SQLITE_MODULE_PATH, { recursive: true, force: true })
    return
  }
}

ensureAnalytixInstall()
