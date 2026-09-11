import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, mkdtempSync, realpathSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')
// Deliberately not process.env spread: no Provider/Hub credentials, runtime
// overrides, NODE_OPTIONS or browser paths may leak into the isolated app.
const inheritedKeys = [
  'PATH', 'LANG', 'LC_ALL', 'LC_CTYPE', 'TERM', 'COLORTERM', 'DISPLAY',
  'ANALYTIX_DEV_CACHE_ROOT', 'GOCACHE', 'GOMODCACHE', 'GOTMPDIR',
  'CARGO_HOME', 'CARGO_TARGET_DIR', 'RUSTUP_HOME', 'CCACHE_DIR', 'SCCACHE_DIR',
  'XWIN_CACHE_DIR', 'COREPACK_HOME', 'ELECTRON_CACHE', 'ELECTRON_BUILDER_CACHE',
  'PLAYWRIGHT_BROWSERS_PATH', 'NODE_COMPILE_CACHE', 'npm_config_cache',
  'PIP_CACHE_DIR', 'UV_CACHE_DIR', 'PYTHONPYCACHEPREFIX', 'MYPY_CACHE_DIR', 'RUFF_CACHE_DIR'
]

function privateDirectory(directory, create = false) {
  if (create) {
    try { mkdirSync(directory, { mode: 0o700 }) } catch (error) {
      if (error.code !== 'EEXIST') throw error
    }
  }
  const stat = lstatSync(directory)
  if (!stat.isDirectory() || stat.isSymbolicLink() ||
    stat.uid !== process.getuid?.() || (stat.mode & 0o7777) !== 0o700 ||
    realpathSync(directory) !== directory) {
    throw new Error('Development profile requires canonical, owner-only directories; refusing to repair or reuse unsafe state.')
  }
}

export function prepareDevelopmentProfile({ root = repo, env = process.env, profile = 'default', fresh = false } = {}) {
  if (!/^[a-z0-9][a-z0-9_-]{0,47}$/.test(profile)) throw new Error('Invalid development profile name.')
  const cache = env.ANALYTIX_DEV_CACHE_ROOT
  if (!cache || !/^\/[A-Za-z0-9._/-]+$/.test(cache) || resolve(cache) !== cache) {
    throw new Error('A verified ANALYTIX_DEV_CACHE_ROOT is required; source the configured cache helper first.')
  }
  const temporary = join(cache, 'tmp')
  privateDirectory(cache)
  privateDirectory(temporary)
  const checkout = createHash('sha256').update(realpathSync(root)).digest('hex').slice(0, 16)
  const profiles = join(temporary, `analytix-dev-${checkout}`)
  privateDirectory(profiles, true)
  const taskRoot = fresh ? mkdtempSync(join(profiles, 'fresh-')) : join(profiles, `profile-${profile}`)
  privateDirectory(taskRoot, true)
  const userData = join(taskRoot, 'user-data')
  privateDirectory(userData, true)
  const config = join(userData, '.config')
  const appCache = join(userData, '.cache')
  privateDirectory(config, true)
  privateDirectory(appCache, true)
  privateDirectory(join(userData, '.analytix'), true)
  privateDirectory(join(userData, '.analytix', 'data'), true)
  const childEnv = Object.fromEntries(inheritedKeys.filter(key => env[key] !== undefined).map(key => [key, env[key]]))
  Object.assign(childEnv, {
    NODE_ENV: 'development',
    ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE: 'isolated-local-v1',
    ANALYTIX_USER_DATA_DIR: userData,
    // Child-only home: the invoking shell and existing application state are
    // untouched. Libraries using homedir() must see the same isolated boundary.
    HOME: userData,
    XDG_CONFIG_HOME: config,
    XDG_CACHE_HOME: appCache,
    TMPDIR: temporary,
    ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '0'
  })
  return { taskRoot, userData, env: childEnv }
}

// Directory isolation alone is not launch admission. The existing Darwin
// runtime requires a separately provisioned explicit task Keychain. Never
// create a placeholder, select the login Keychain, or silently skip binding.
export function assertDevelopmentProfileReady(profile) {
  try {
    const parent = join(profile.taskRoot, 'darwin-secret-store-keychain')
    privateDirectory(parent)
    const database = join(parent, 'analytix-task.keychain-db')
    const stat = lstatSync(database)
    if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 ||
      stat.uid !== process.getuid?.() || (stat.mode & 0o7777) !== 0o600 ||
      stat.size <= 0 || realpathSync(database) !== database) throw new Error('invalid')
  } catch {
    throw new Error('Isolated development is not configured: an explicitly provisioned task Keychain is required. No default/login Keychain fallback is allowed. See docs/analytix/development-baseline.md.')
  }
}

export function parseDevelopmentArgs(args) {
  const result = { fast: false, fresh: false, profile: 'default' }
  for (let index = 0; index < args.length; index++) {
    if (args[index] === '--fast') result.fast = true
    else if (args[index] === '--fresh') result.fresh = true
    else if (args[index] === '--profile' && args[index + 1]) result.profile = args[++index]
    else throw new Error('Usage: npm run dev:isolated [-- --fast | --fresh | --profile <name>]')
  }
  if (result.fresh && result.profile !== 'default') throw new Error('Choose --fresh or --profile, not both.')
  return result
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const options = parseDevelopmentArgs(process.argv.slice(2))
    // The current native authority is macOS-only. Never silently open a
    // reduced-capability desktop on an unqualified host.
    if (process.platform !== 'darwin') throw new Error('Native desktop development currently requires the qualified macOS build environment. Source checks remain available on other hosts.')
    const profile = prepareDevelopmentProfile(options)
    assertDevelopmentProfileReady(profile)
    for (const script of ['doctor', ...(!options.fast ? ['build:data-native:development', 'build:runtime'] : [])]) {
      const args = script === 'doctor' ? ['run', script, '--', '--native'] : ['run', script]
      const result = spawnSync('npm', args, { cwd: repo, stdio: 'inherit', env: process.env })
      if (result.error || result.signal || result.status !== 0) throw new Error(`Development prerequisite failed: ${script}`)
    }
    console.log(`[dev] Isolated ${options.fresh ? 'fresh' : options.profile} profile; Hub bootstrap disabled. Normal Provider onboarding is unchanged.`)
    console.log('[dev] Profile is retained for restart/recovery; this is not packaged or live-Provider acceptance.')
    // Invoke the local executable directly, without routing through dev:fast.
    const child = spawn(join(repo, 'node_modules/.bin/electron-vite'), ['dev'], {
      cwd: repo, stdio: 'inherit', env: profile.env
    })
    const signals = ['SIGINT', 'SIGTERM']
    const forward = signal => child.kill(signal)
    const listeners = signals.map(signal => {
      const listener = () => forward(signal)
      process.on(signal, listener)
      return listener
    })
    child.once('error', () => { console.error('[dev] Desktop process failed to start.'); process.exitCode = 1 })
    child.once('exit', (code, signal) => {
      signals.forEach((name, index) => process.removeListener(name, listeners[index]))
      process.exitCode = code ?? (signal === 'SIGINT' ? 130 : 1)
    })
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
