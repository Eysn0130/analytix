import { spawn, spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { lstatSync, mkdirSync, mkdtempSync, realpathSync } from 'node:fs'
import { dirname, isAbsolute, join, resolve } from 'node:path'
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
  'PIP_CACHE_DIR', 'UV_CACHE_DIR', 'PYTHONPYCACHEPREFIX', 'MYPY_CACHE_DIR', 'RUFF_CACHE_DIR',
  'SystemRoot', 'WINDIR', 'ComSpec', 'PATHEXT'
]

function privateDirectory(directory, create = false) {
  if (process.platform === 'win32') {
    const result = spawnSync('powershell.exe', [
      '-NoProfile', '-NonInteractive', '-File',
      join(repo, 'scripts/windows-development-profile.ps1'), '-Directory', directory,
      ...(create ? ['-Create'] : [])
    ], { cwd: repo, stdio: 'ignore', windowsHide: true, timeout: 10000 })
    if (result.error || result.signal || result.status !== 0) {
      throw new Error('Development profile requires canonical Windows directories with an owner-only native ACL.')
    }
    return
  }
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

function defaultDevelopmentStateRoot(env) {
  return process.platform === 'win32'
    ? join(env.LOCALAPPDATA || join(homedir(), 'AppData', 'Local'), 'AnalytixDevelopment')
    : join(homedir(), '.analytix-development')
}

function entryExists(path) {
  try { lstatSync(path); return true } catch (error) {
    if (error.code === 'ENOENT') return false
    throw error
  }
}

export function prepareDevelopmentProfile({ root = repo, env = process.env, profile = 'default', fresh = false,
  stateRoot = defaultDevelopmentStateRoot(env), credentialMode = 'development' } = {}) {
  if (!/^[a-z0-9][a-z0-9_-]{0,47}$/.test(profile)) throw new Error('Invalid development profile name.')
  if (!['development', 'isolated-keychain'].includes(credentialMode)) throw new Error('Invalid credential mode.')
  if (process.platform === 'win32' && credentialMode === 'isolated-keychain') {
    throw new Error('--isolated-keychain is a macOS-only compatibility QA mode.')
  }
  const cache = env.ANALYTIX_DEV_CACHE_ROOT || (process.platform === 'win32' ? join(stateRoot, 'build-cache') : '')
  if (!cache || !isAbsolute(cache) || resolve(cache) !== cache ||
    (process.platform !== 'win32' && !/^\/[A-Za-z0-9._/-]+$/.test(cache))) {
    throw new Error('A verified ANALYTIX_DEV_CACHE_ROOT is required; source the configured cache helper first.')
  }
  const temporary = join(cache, 'tmp')
  if (process.platform === 'win32') privateDirectory(stateRoot, true)
  privateDirectory(cache, process.platform === 'win32')
  privateDirectory(temporary, process.platform === 'win32')
  const checkout = createHash('sha256').update(realpathSync(root)).digest('hex').slice(0, 16)
  // Protected state must not inherit the storage qualification of a cache.
  // In particular the configured removable APFS cache is not admitted by the
  // Core's managed-local-filesystem validator. Keep retained state on the
  // local home volume; Core still performs its complete filesystem admission.
  if (!isAbsolute(stateRoot) || resolve(stateRoot) !== stateRoot ||
    (process.platform !== 'win32' && !/^\/[A-Za-z0-9._/-]+$/.test(stateRoot))) {
    throw new Error('Development state requires a canonical local state root.')
  }
  privateDirectory(stateRoot, true)
  const profiles = join(stateRoot, `analytix-dev-${checkout}`)
  privateDirectory(profiles, true)
  const prefix = credentialMode === 'development' ? 'development-' : ''
  const taskRoot = fresh
    ? (process.platform === 'win32'
        ? join(profiles, `${prefix}fresh-${randomBytes(12).toString('hex')}`)
        : mkdtempSync(join(profiles, `${prefix}fresh-`)))
    : join(profiles, `${prefix}profile-${profile}`)
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
  if (process.platform === 'win32') {
    privateDirectory(join(homeRoot, 'AppData'), created)
    privateDirectory(join(homeRoot, 'AppData', 'Local'), created)
    privateDirectory(join(homeRoot, 'AppData', 'Roaming'), created)
  }
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
    ...(process.platform === 'win32' ? {
      USERPROFILE: homeRoot,
      APPDATA: join(homeRoot, 'AppData', 'Roaming'),
      LOCALAPPDATA: join(homeRoot, 'AppData', 'Local'),
      TEMP: temporary,
      TMP: temporary,
      ANALYTIX_DEV_CACHE_ROOT: cache
    } : {}),
    ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '0',
    CSC_IDENTITY_AUTO_DISCOVERY: 'false',
    ANALYTIX_UPDATE_CHANNEL: 'beta',
    ANALYTIX_UPDATE_FEED_URL: 'https://example.invalid/analytix/development/'
  })
  if (credentialMode === 'development') {
    const credentialRoot = join(stateRoot, 'provider-credentials')
    privateDirectory(credentialRoot, true)
    childEnv.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR = credentialRoot
  }
  const needsKeychain = credentialMode === 'isolated-keychain' && created
  return { taskRoot, userData, homeRoot, created, needsKeychain, credentialMode, env: childEnv }
}

// Application-state isolation and credential isolation have distinct owners.
// Explicit QA keeps its original task Keychain; ordinary source development
// admits only its private shared authority directory.
export function assertDevelopmentProfileReady(profile) {
  if (profile.credentialMode === 'development') {
    privateDirectory(profile.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR)
    return
  }
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
  const result = { fast: false, fresh: false, profile: 'default', unlockKeychain: false, credentialMode: 'development' }
  for (let index = 0; index < args.length; index++) {
    if (args[index] === '--fast') result.fast = true
    else if (args[index] === '--fresh') result.fresh = true
    else if (args[index] === '--isolated-keychain') result.credentialMode = 'isolated-keychain'
    else if (args[index] === '--unlock-keychain') result.unlockKeychain = true
    else if (args[index] === '--profile' && args[index + 1]) result.profile = args[++index]
    else throw new Error('Usage: npm run dev:isolated [-- --fast | --fresh | --profile <name> | --isolated-keychain [--unlock-keychain]]')
  }
  if (result.fresh && result.profile !== 'default') throw new Error('Choose --fresh or --profile, not both.')
  if (result.unlockKeychain && result.credentialMode !== 'isolated-keychain') throw new Error('--unlock-keychain requires --isolated-keychain.')
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
    const args = script === 'doctor' && process.platform !== 'win32' ? ['run', script, '--', '--native'] : ['run', script]
    const result = spawnSync(process.platform === 'win32' ? 'npm.cmd' : 'npm', args, {
      cwd: repo, stdio: 'inherit', env: developmentBuildEnvironment(profile), shell: process.platform === 'win32'
    })
    if (result.error || result.signal || result.status !== 0) throw new Error(`Development prerequisite failed: ${script}`)
  },
  startApplication = () => {
    console.log(`[dev] ${profile.credentialMode} credentials; isolated ${options.fresh ? 'fresh' : options.profile} profile; Hub bootstrap disabled. Normal Provider onboarding is unchanged.`)
    console.log('[dev] Profile is retained for restart/recovery; this is not packaged or live-Provider acceptance.')
    // Invoke the local executable directly, without routing through dev:fast.
    return spawn(join(repo, 'node_modules/.bin', process.platform === 'win32' ? 'electron-vite.cmd' : 'electron-vite'), ['dev'], {
      cwd: repo, stdio: 'inherit', env: profile.env, shell: process.platform === 'win32'
    })
  }
} = {}) {
  if (process.platform === 'win32' && !options.fast) {
    throw new Error('Windows native development build is not yet qualified; use --fast only with a previously prepared local runtime.')
  }
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
    if (process.platform !== 'darwin' && process.platform !== 'win32') throw new Error('Native desktop development currently requires macOS or a prepared Windows --fast environment.')
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
