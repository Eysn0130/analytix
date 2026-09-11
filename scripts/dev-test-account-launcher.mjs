#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'
import { existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import process from 'node:process'

const require = createRequire(import.meta.url)
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const cliArgs = process.argv.slice(2)
const passthroughIndex = cliArgs.indexOf('--')
const rawArgs = passthroughIndex >= 0 ? cliArgs.slice(0, passthroughIndex) : cliArgs
const electronArgs = passthroughIndex >= 0 ? cliArgs.slice(passthroughIndex + 1) : []
const mode = rawArgs.includes('--built') ? 'built' : 'dev'
const envFile = argValue('--env-file', '.env.analytix-e2e.local')
const shouldBuild = rawArgs.includes('--build') || (mode === 'built' && !rawArgs.includes('--no-build'))

function argValue(name, fallback = '') {
  const inline = rawArgs.find((arg) => arg.startsWith(`${name}=`))
  if (inline) return inline.slice(name.length + 1)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : fallback
}

function parseEnvFile(filePath) {
  if (!existsSync(filePath)) return {}
  const parsed = {}
  for (const rawLine of readFileSync(filePath, 'utf8').split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const equalIndex = line.indexOf('=')
    if (equalIndex <= 0) continue
    const key = line.slice(0, equalIndex).trim()
    let value = line.slice(equalIndex + 1).trim()
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1)
    }
    parsed[key] = value
  }
  return parsed
}

function runChecked(command, args) {
  const result = spawnSync(command, args, {
    cwd: root,
    stdio: 'inherit',
    env: process.env
  })
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}

function requireValue(env, name, alternateNames = []) {
  if (env[name]) return
  if (alternateNames.some((alternate) => env[alternate])) return
  console.error(`[dev-test-account] Missing ${name}.`)
  process.exit(1)
}

const localEnv = parseEnvFile(resolve(root, envFile))
const childEnv = {
  ...localEnv,
  // Explicit invocation environment wins over optional local defaults.
  ...process.env,
  // This launcher is an unpackaged test-only entrypoint. Electron/Vite may
  // otherwise inherit a production NODE_ENV and disable the guarded test
  // account bootstrap even though the caller selected this script explicitly.
  NODE_ENV: 'development',
  ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '1',
  ELECTRON_ENABLE_LOGGING: process.env.ELECTRON_ENABLE_LOGGING || localEnv.ELECTRON_ENABLE_LOGGING || '1'
}

if (mode === 'dev') {
  // Development E2E must build/verify the runtime from the current source
  // digest; an ambient prebuilt path can otherwise select stale code.
  delete childEnv.ANALYTIX_GO_RUNTIME_SERVER_BIN
}

if (!childEnv.ANALYTIX_HUB_TEST_GATEWAY_TOKEN || !childEnv.ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN) {
  requireValue(childEnv, 'ANALYTIX_HUB_TEST_EMAIL')
  requireValue(childEnv, 'ANALYTIX_HUB_TEST_PASSWORD')
}

if (!childEnv.ANALYTIX_USER_DATA_DIR) {
  childEnv.ANALYTIX_USER_DATA_DIR = mkdtempSync(join(tmpdir(), 'analytix-test-account-'))
}

const testHome = childEnv.ANALYTIX_TEST_HOME?.trim() || mkdtempSync(join(tmpdir(), 'analytix-test-home-'))
mkdirSync(testHome, { recursive: true, mode: 0o700 })
if (process.platform === 'win32') {
  childEnv.USERPROFILE = testHome
  childEnv.APPDATA = join(testHome, 'AppData', 'Roaming')
  childEnv.LOCALAPPDATA = join(testHome, 'AppData', 'Local')
  mkdirSync(childEnv.APPDATA, { recursive: true })
  mkdirSync(childEnv.LOCALAPPDATA, { recursive: true })
} else {
  childEnv.HOME = testHome
  childEnv.XDG_CONFIG_HOME = join(testHome, '.config')
  mkdirSync(childEnv.XDG_CONFIG_HOME, { recursive: true, mode: 0o700 })
}

if (shouldBuild) {
  runChecked(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['run', 'build:data-native:development'])
  runChecked(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['run', 'build'])
}

console.log(`[dev-test-account] mode=${mode}`)
console.log(`[dev-test-account] envFile=${envFile}${existsSync(resolve(root, envFile)) ? '' : ' (not found)'}`)
console.log(`[dev-test-account] userData=${childEnv.ANALYTIX_USER_DATA_DIR}`)
console.log(`[dev-test-account] testHome=${testHome}`)
console.log(`[dev-test-account] goRuntime=${mode === 'dev' ? 'current-source-verified development build' : childEnv.ANALYTIX_GO_RUNTIME_SERVER_BIN || 'bundled runtime'}`)
console.log('[dev-test-account] Hub test bootstrap enabled; secrets are read from local environment only.')

const command = mode === 'dev'
  ? (process.platform === 'win32' ? 'npm.cmd' : 'npm')
  : require('electron')
const args = mode === 'dev'
  ? ['run', 'dev:fast', ...(electronArgs.length > 0 ? ['--', ...electronArgs] : [])]
  : [...electronArgs, '.']

const child = spawn(command, args, {
  cwd: root,
  stdio: 'inherit',
  env: childEnv
})

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 0)
})
