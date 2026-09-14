import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, mkdtempSync, realpathSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { homedir } from 'node:os'
import { assertCommittedDevelopmentKeychainIdentity, prepareDevelopmentKeychain, promptDevelopmentKeychainPassword } from './development-keychain.mjs'

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

function entryExists(path) {
  try { lstatSync(path); return true } catch (error) {
    if (error.code === 'ENOENT') return false
    throw error
  }
}

export function prepareDevelopmentProfile({ root = repo, env = process.env, profile = 'default', fresh = false,
  stateRoot = join(homedir(), '.analytix-development') } = {}) {
  if (!/^[a-z0-9][a-z0-9_-]{0,47}$/.test(profile)) throw new Error('Invalid development profile name.')
  const cache = env.ANALYTIX_DEV_CACHE_ROOT
  if (!cache || !/^\/[A-Za-z0-9._/-]+$/.test(cache) || resolve(cache) !== cache) {
    throw new Error('A verified ANALYTIX_DEV_CACHE_ROOT is required; source the configured cache helper first.')
  }
  const temporary = join(cache, 'tmp')
  privateDirectory(cache)
  privateDirectory(temporary)
  const checkout = createHash('sha256').update(realpathSync(root)).digest('hex').slice(0, 16)
  // Protected state must not inherit the storage qualification of a cache.
  // In particular the configured removable APFS cache is not admitted by the
  // Core's managed-local-filesystem validator. Keep retained state on the
  // local home volume; Core still performs its complete filesystem admission.
  if (!/^\/[A-Za-z0-9._/-]+$/.test(stateRoot) || resolve(stateRoot) !== stateRoot) {
    throw new Error('Development state requires a canonical local state root.')
  }
  privateDirectory(stateRoot, true)
  const profiles = join(stateRoot, `analytix-dev-${checkout}`)
  privateDirectory(profiles, true)
  const taskRoot = fresh ? mkdtempSync(join(profiles, 'fresh-')) : join(profiles, `profile-${profile}`)
  const created = fresh || !entryExists(taskRoot)
  privateDirectory(taskRoot, created)
  const userData = join(taskRoot, 'user-data')
  privateDirectory(userData, created)
  const homeRoot = join(taskRoot, 'home')
  privateDirectory(homeRoot, created)
  const config = join(homeRoot, '.config')
  const appCache = join(homeRoot, '.cache')
  privateDirectory(config, created)
  privateDirectory(appCache, created)
  privateDirectory(join(homeRoot, '.analytix'), created)
  privateDirectory(join(homeRoot, '.analytix', 'data'), created)
  const childEnv = Object.fromEntries(inheritedKeys.filter(key => env[key] !== undefined).map(key => [key, env[key]]))
  Object.assign(childEnv, {
    NODE_ENV: 'development',
    ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE: 'isolated-local-v1',
    ANALYTIX_USER_DATA_DIR: userData,
    ANALYTIX_DEV_STATE_ROOT: stateRoot,
    // Child-only home: the invoking shell and existing application state are
    // untouched. Libraries using homedir() must see the same isolated boundary.
    HOME: homeRoot,
    XDG_CONFIG_HOME: config,
    XDG_CACHE_HOME: appCache,
    TMPDIR: temporary,
    ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '0',
    CSC_IDENTITY_AUTO_DISCOVERY: 'false',
    ANALYTIX_UPDATE_CHANNEL: 'beta',
    ANALYTIX_UPDATE_FEED_URL: 'https://example.invalid/analytix/development/'
  })
  const needsKeychain = created
  return { taskRoot, userData, homeRoot, created, needsKeychain, env: childEnv }
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
    assertCommittedDevelopmentKeychainIdentity(profile, { allowCoreRecovery: true })
  } catch {
    throw new Error('Isolated development requires its original task Keychain. Missing or incomplete state is preserved; use --fresh for a separate profile after cancelled setup. No default/login Keychain fallback is allowed. See docs/analytix/development-baseline.md.')
  }
}

export function parseDevelopmentArgs(args) {
  const result = { fast: false, fresh: false, profile: 'default', unlockKeychain: false }
  for (let index = 0; index < args.length; index++) {
    if (args[index] === '--fast') result.fast = true
    else if (args[index] === '--fresh') result.fresh = true
    else if (args[index] === '--unlock-keychain') result.unlockKeychain = true
    else if (args[index] === '--profile' && args[index + 1]) result.profile = args[++index]
    else throw new Error('Usage: npm run dev:isolated [-- --fast | --fresh | --profile <name> | --unlock-keychain]')
  }
  if (result.fresh && result.profile !== 'default') throw new Error('Choose --fresh or --profile, not both.')
  return result
}

export function developmentBuildEnvironment(profile, env = process.env) {
  // Rustup installation identity is a tool input, not an application home.
  // Retain the isolated HOME and cache-only Cargo state while allowing the
  // native builder to verify the already installed pinned Rust toolchain.
  return { ...profile.env, RUSTUP_HOME: env.RUSTUP_HOME || join(homedir(), '.rustup') }
}

export async function launchDevelopment(profile, options, {
  assertProfileReady = assertDevelopmentProfileReady,
  promptPassword = promptDevelopmentKeychainPassword,
  prepareKeychain = prepareDevelopmentKeychain,
  runPrerequisite = script => {
    const args = script === 'doctor' ? ['run', script, '--', '--native'] : ['run', script]
    const result = spawnSync('npm', args, { cwd: repo, stdio: 'inherit', env: developmentBuildEnvironment(profile) })
    if (result.error || result.signal || result.status !== 0) throw new Error(`Development prerequisite failed: ${script}`)
  },
  startApplication = () => {
    console.log(`[dev] Isolated ${options.fresh ? 'fresh' : options.profile} profile; Hub bootstrap disabled. Normal Provider onboarding is unchanged.`)
    console.log('[dev] Profile is retained for restart/recovery; this is not packaged or live-Provider acceptance.')
    // Invoke the local executable directly, without routing through dev:fast.
    return spawn(join(repo, 'node_modules/.bin/electron-vite'), ['dev'], {
      cwd: repo, stdio: 'inherit', env: profile.env
    })
  }
} = {}) {
  if (!profile.needsKeychain) assertProfileReady(profile)
  for (const script of ['doctor', ...(!options.fast ? ['build:data-native:development', 'build:runtime'] : [])]) {
    await runPrerequisite(script)
  }
  // Build time must not consume the task Keychain's unlocked window. A failed
  // prerequisite also leaves a new profile without a provisioned Keychain.
  if (profile.needsKeychain || options.unlockKeychain) {
    const password = promptPassword()
    try { await prepareKeychain(profile, password, { create: profile.needsKeychain }) }
    finally { password.fill(0) }
  }
  assertProfileReady(profile)
  return startApplication()
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const options = parseDevelopmentArgs(process.argv.slice(2))
    // Task process and its descendants only; never changes the invoking shell.
    process.umask(0o077)
    // The current native authority is macOS-only. Never silently open a
    // reduced-capability desktop on an unqualified host.
    if (process.platform !== 'darwin') throw new Error('Native desktop development currently requires the qualified macOS build environment. Source checks remain available on other hosts.')
    const profile = prepareDevelopmentProfile(options)
    const child = await launchDevelopment(profile, options)
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
