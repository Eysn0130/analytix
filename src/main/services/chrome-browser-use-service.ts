import { execFile, execFileSync } from 'node:child_process'
import { promisify } from 'node:util'
import { existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { homedir as defaultHomedir } from 'node:os'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import { shell } from 'electron'
import type {
  ChromeBrowserUseConnectionState,
  ChromeBrowserUseExtensionPageTarget,
  ChromeBrowserUseExtensionProfile,
  ChromeBrowserUseExtensionStatus,
  ChromeBrowserUseNativeHostStatus,
  ChromeBrowserUseStatus,
  PathOpenResult
} from '../../shared/analytix-api'

const CHROME_EXTENSION_ID = 'bccibejdpcjcdlcnbpempcpjgladapgk'
const CHROME_NATIVE_HOST_NAME = 'top.analytix.codexextension'
const CHROME_NATIVE_HOST_REGISTRY_KEY_PREFIXES = [
  'HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts',
  'HKLM\\Software\\WOW6432Node\\Google\\Chrome\\NativeMessagingHosts',
  'HKLM\\Software\\Google\\Chrome\\NativeMessagingHosts'
]
const CHROME_PREFERENCES_PATH_ENV = 'ANALYTIX_CHROME_PREFERENCES_PATH'
const CHROME_USER_DATA_DIR_ENV = 'ANALYTIX_CHROME_USER_DATA_DIR'
const CHROME_NATIVE_HOST_MANIFEST_PATH_ENV = 'ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH'
const CHROME_BROWSER_CLIENT_PATH_ENV = 'ANALYTIX_CHROME_BROWSER_CLIENT_PATH'
const ANALYTIX_BROWSER_BUNDLED_ROOT_ENV = 'ANALYTIX_BROWSER_BUNDLED_ROOT'
const ANALYTIX_MANAGED_CHROME_ROOT_ENV = 'ANALYTIX_MANAGED_CHROME_ROOT'
const OPENAI_BUNDLED_ROOT_ENV = 'ANALYTIX_OPENAI_BUNDLED_ROOT'
const ANALYTIX_CODEX_HOME_ENV = 'ANALYTIX_CODEX_HOME'
const CHROME_WEBSTORE_URL = `https://chromewebstore.google.com/detail/codex/${CHROME_EXTENSION_ID}`
const CHROME_EXTENSION_SETTINGS_URL = `chrome://extensions/?id=${CHROME_EXTENSION_ID}`
const CHROME_BROWSER_PROBE_TIMEOUT_MS = 7_000
const CHROME_BROWSER_PIPE_CONNECT_TIMEOUT_MS = 2_500
const CHROME_BROWSER_PROBE_RESULT_MARKER = 'ANALYTIX_BROWSER_USE_PROBE_RESULT '

const execFileAsync = promisify(execFile)

type ChromeBrowserUseConnectionProbe = ChromeBrowserUseStatus['connectionProbe']

type ChromeBrowserUseInspectOptions = {
  platform?: NodeJS.Platform
  arch?: NodeJS.Architecture
  homedir?: string
  env?: NodeJS.ProcessEnv
  resourcesPath?: string
  readWindowsRegistryDefaultValue?: (registryKey: string) => string | null
  probeConnection?: () => Promise<ChromeBrowserUseConnectionProbe>
}

type ExecFileSyncForRegistration = (
  file: string,
  args: readonly string[],
  options?: Parameters<typeof execFileSync>[2]
) => string | Buffer

type ChromeBrowserUseNativeHostRegistrationOptions = ChromeBrowserUseInspectOptions & {
  execFileSync?: ExecFileSyncForRegistration
  mkdirSync?: typeof mkdirSync
  writeFileSync?: typeof writeFileSync
}

export type ChromeBrowserUseNativeHostRegistrationResult = {
  ok: boolean
  skipped?: boolean
  reason?: string
  manifestPath?: string
  hostPath?: string
  registryKey?: string
}

type NativeHostManifestLocation = {
  manifestPath: string
  registryKey: string | null
  registryManifestPath: string | null
  registryKeyExists: boolean | null
}

type ChromeExtensionPreferences = {
  preferencesPath: string | null
  registered: boolean
  state: number | null
  path: string | null
  disableReasons: unknown[]
}

function optionPlatform(options: ChromeBrowserUseInspectOptions): NodeJS.Platform {
  return options.platform ?? process.platform
}

function optionHomedir(options: ChromeBrowserUseInspectOptions): string {
  return options.homedir ?? defaultHomedir()
}

function optionEnv(options: ChromeBrowserUseInspectOptions): NodeJS.ProcessEnv {
  return options.env ?? process.env
}

function optionResourcesPath(options: ChromeBrowserUseInspectOptions): string {
  const explicit = options.resourcesPath?.trim()
  if (explicit) return explicit
  const processWithResources = process as NodeJS.Process & { resourcesPath?: string }
  return typeof processWithResources.resourcesPath === 'string' ? processWithResources.resourcesPath : ''
}

function trimEnvValue(env: NodeJS.ProcessEnv, key: string): string {
  return env[key]?.trim() ?? ''
}

function readJsonFileIfPresent(filePath: string): Record<string, unknown> | null {
  if (!existsSync(filePath)) return null
  return JSON.parse(readFileSync(filePath, 'utf8')) as Record<string, unknown>
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function resolveChromeUserDataDirectory(options: ChromeBrowserUseInspectOptions): string {
  const env = optionEnv(options)
  const override = trimEnvValue(env, CHROME_USER_DATA_DIR_ENV)
  if (override) return resolve(override)

  const home = optionHomedir(options)
  const platform = optionPlatform(options)
  if (platform === 'darwin') {
    return join(home, 'Library', 'Application Support', 'Google', 'Chrome')
  }
  if (platform === 'win32') {
    return join(env.LOCALAPPDATA || join(home, 'AppData', 'Local'), 'Google', 'Chrome', 'User Data')
  }
  return join(home, '.config', 'google-chrome')
}

function getChromePreferencesPathOverride(options: ChromeBrowserUseInspectOptions): string | null {
  const override = trimEnvValue(optionEnv(options), CHROME_PREFERENCES_PATH_ENV)
  return override ? resolve(override) : null
}

function resolveChromeBrowserClientPath(
  options: ChromeBrowserUseInspectOptions,
  nativeHost?: ChromeBrowserUseNativeHostStatus
): { path: string; exists: boolean } | null {
  const env = optionEnv(options)
  const override = trimEnvValue(env, CHROME_BROWSER_CLIENT_PATH_ENV)
  if (override) {
    const path = resolve(override)
    return { path, exists: existsSync(path) }
  }

  const resourcesPath = optionResourcesPath(options)
  const resourcesRoots = [
    resourcesPath ? join(resourcesPath, 'managed-chrome') : '',
    resourcesPath ? join(resourcesPath, 'app.asar.unpacked', 'managed-chrome') : '',
    resourcesPath ? join(resourcesPath, 'plugins', 'analytix-bundled') : '',
    resourcesPath ? join(resourcesPath, 'app.asar.unpacked', 'plugins', 'analytix-bundled') : ''
  ].filter(Boolean)
  const fallbackRoots = resourcesPath ? [] : openAiBundledRootsFromEnvironment(options)
  const roots = [
    ...managedChromeRootsFromEnvironment(options),
    ...resourcesRoots,
    ...fallbackRoots
  ].filter(Boolean)
  const candidates = uniquePaths([
    ...browserClientCandidatesForNativeHost(nativeHost),
    ...roots.flatMap(browserClientCandidatesForOpenAiBundledRoot)
  ])
  const path = candidates.find((candidate) => existsSync(candidate))
  if (path) return { path, exists: true }
  const firstExistingRootCandidate = candidates.find((candidate) => existsSync(dirname(dirname(candidate))))
  return firstExistingRootCandidate ? { path: firstExistingRootCandidate, exists: false } : null
}

function uniquePaths(paths: string[]): string[] {
  return paths.filter((item, index, all) => item.trim().length > 0 && all.indexOf(item) === index)
}

function openAiBundledRootsFromEnvironment(options: ChromeBrowserUseInspectOptions): string[] {
  const env = optionEnv(options)
  const home = optionHomedir(options)
  const programData = env.ProgramData || env.PROGRAMDATA
  const localAppData = env.LOCALAPPDATA
  const codexHome = trimEnvValue(env, ANALYTIX_CODEX_HOME_ENV) || trimEnvValue(env, 'CODEX_HOME')
  const explicitRoot = trimEnvValue(env, ANALYTIX_BROWSER_BUNDLED_ROOT_ENV) || trimEnvValue(env, OPENAI_BUNDLED_ROOT_ENV)
  return [
    explicitRoot,
    codexHome ? join(codexHome, 'plugins', 'cache', 'analytix-bundled') : '',
    codexHome ? join(codexHome, 'plugins', 'cache', 'openai-bundled') : '',
    join(home, '.analytix', 'plugins', 'cache', 'analytix-bundled'),
    join(home, '.codex', 'plugins', 'cache', 'openai-bundled'),
    join(home, '.analytix', 'plugins', 'cache', 'openai-bundled'),
    programData ? join(programData, 'analytix', 'codex-extension') : '',
    programData ? join(programData, 'analytix', 'openai-bundled') : '',
    programData ? join(programData, 'OpenAI', 'Codex', 'plugins', 'cache', 'openai-bundled') : '',
    localAppData ? join(localAppData, 'OpenAI', 'Codex', 'plugins', 'cache', 'openai-bundled') : ''
  ].filter(Boolean)
}

function managedChromeRootsFromEnvironment(options: ChromeBrowserUseInspectOptions): string[] {
  const env = optionEnv(options)
  const explicitRoot = trimEnvValue(env, ANALYTIX_MANAGED_CHROME_ROOT_ENV)
  return explicitRoot ? [explicitRoot] : []
}

function browserClientCandidatesForOpenAiBundledRoot(root: string): string[] {
  const candidates = [
    join(root, 'chrome', 'scripts', 'browser-client.mjs'),
    join(root, 'plugins', 'chrome', 'scripts', 'browser-client.mjs')
  ]
  for (const chromeRoot of [join(root, 'chrome'), join(root, 'plugins', 'chrome')]) {
    if (!existsSync(chromeRoot)) continue
    try {
      for (const entry of readdirSync(chromeRoot, { withFileTypes: true })) {
        if (entry.isDirectory()) {
          candidates.push(join(chromeRoot, entry.name, 'scripts', 'browser-client.mjs'))
        }
      }
    } catch {
      // Best-effort diagnostics only; missing/unreadable plugin roots simply
      // leave the Browser Use handshake unavailable.
    }
  }
  return candidates
}

function browserClientCandidatesForNativeHost(nativeHost?: ChromeBrowserUseNativeHostStatus): string[] {
  const hostPath = nativeHost?.actualHostPath
  if (!hostPath) return []
  const match = hostPath.match(/^(.*)[\\/]+extension-host[\\/]/)
  return match?.[1] ? [join(match[1], 'scripts', 'browser-client.mjs')] : []
}

function isUsableChromeProfile(userDataDirectory: string, profileDirectory: unknown): profileDirectory is string {
  return typeof profileDirectory === 'string' &&
    profileDirectory.length > 0 &&
    existsSync(join(userDataDirectory, profileDirectory, 'Preferences'))
}

function chromeProfileDirectorySortKey(profileDirectory: string): number {
  if (profileDirectory === 'Default') return 0
  const match = profileDirectory.match(/^Profile (\d+)$/)
  return match ? Number(match[1]) : -1
}

function compareChromeProfileDirectories(first: string, second: string): number {
  return chromeProfileDirectorySortKey(first) - chromeProfileDirectorySortKey(second)
}

function chooseLatestUsableChromeProfile(
  userDataDirectory: string,
  profileDirectories: unknown[]
): string | null {
  const usableProfiles = profileDirectories.filter((profileDirectory) =>
    isUsableChromeProfile(userDataDirectory, profileDirectory)
  )
  if (usableProfiles.length === 0) return null
  return usableProfiles.sort(compareChromeProfileDirectories).at(-1) ?? null
}

function resolveChromeProfileDirectoryFromLocalState(userDataDirectory: string): string | null {
  const localState = readJsonFileIfPresent(join(userDataDirectory, 'Local State'))
  const profile = objectValue(localState?.profile)
  if (!profile) return null

  if (isUsableChromeProfile(userDataDirectory, profile.last_used)) {
    return profile.last_used
  }
  if (Array.isArray(profile.last_active_profiles)) {
    return chooseLatestUsableChromeProfile(userDataDirectory, profile.last_active_profiles)
  }
  return null
}

function listChromeProfileDirectories(userDataDirectory: string): string[] {
  if (!existsSync(userDataDirectory)) return []
  return readdirSync(userDataDirectory, { withFileTypes: true })
    .filter((entry) =>
      entry.isDirectory() &&
      (entry.name === 'Default' || /^Profile \d+$/.test(entry.name)) &&
      isUsableChromeProfile(userDataDirectory, entry.name)
    )
    .map((entry) => entry.name)
    .sort(compareChromeProfileDirectories)
}

function resolveChromeProfileDirectory(userDataDirectory: string): string | null {
  return resolveChromeProfileDirectoryFromLocalState(userDataDirectory) ??
    chooseLatestUsableChromeProfile(userDataDirectory, listChromeProfileDirectories(userDataDirectory))
}

function getDisableReasons(disableReasons: unknown): unknown[] {
  if (Array.isArray(disableReasons)) return disableReasons
  if (typeof disableReasons === 'number' && disableReasons !== 0) return [disableReasons]
  return []
}

function getChromeExtensionPreferences(profilePath: string, extensionId: string): ChromeExtensionPreferences {
  const preferencesPaths = [
    join(profilePath, 'Secure Preferences'),
    join(profilePath, 'Preferences')
  ]
  for (const preferencesPath of preferencesPaths) {
    const preferences = readJsonFileIfPresent(preferencesPath)
    const extensions = objectValue(preferences?.extensions)
    const settings = objectValue(extensions?.settings)
    const extensionSettings = objectValue(settings?.[extensionId])
    if (!extensionSettings) continue
    return {
      preferencesPath,
      registered: true,
      state: typeof extensionSettings.state === 'number' ? extensionSettings.state : null,
      path: typeof extensionSettings.path === 'string' ? extensionSettings.path : null,
      disableReasons: getDisableReasons(extensionSettings.disable_reasons)
    }
  }
  return {
    preferencesPath: null,
    registered: false,
    state: null,
    path: null,
    disableReasons: []
  }
}

function getChromeExtensionInstallStatusForProfile(
  userDataDirectory: string,
  profileDirectory: string,
  extensionId: string,
  profilePathOverride?: string | null
): Omit<ChromeBrowserUseExtensionProfile, 'selected'> {
  const profilePath = profilePathOverride ?? join(userDataDirectory, profileDirectory)
  const preferences = getChromeExtensionPreferences(profilePath, extensionId)
  const extensionsDirectory = join(profilePath, 'Extensions')
  const extensionPath = join(extensionsDirectory, extensionId)
  const versions = existsSync(extensionPath) && statSync(extensionPath).isDirectory()
    ? readdirSync(extensionPath, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort()
    : []
  const unpackedInstalled =
    preferences.path !== null &&
    existsSync(preferences.path) &&
    statSync(preferences.path).isDirectory()
  const installed = versions.length > 0 || unpackedInstalled
  const disabled = preferences.state === 0 || preferences.disableReasons.length > 0
  const enabled = installed && preferences.registered && !disabled
  return {
    profileDirectory,
    profilePath,
    preferencesPath: preferences.preferencesPath,
    extensionPath,
    installed,
    registered: preferences.registered,
    enabled,
    disabled,
    state: preferences.state,
    disableReasons: preferences.disableReasons,
    versions
  }
}

export function inspectChromeExtensionStatus(
  options: ChromeBrowserUseInspectOptions = {}
): ChromeBrowserUseExtensionStatus {
  try {
    const preferencesPathOverride = getChromePreferencesPathOverride(options)
    const userDataDirectory = preferencesPathOverride
      ? dirname(dirname(preferencesPathOverride))
      : resolveChromeUserDataDirectory(options)
    const selectedProfilePath = preferencesPathOverride ? dirname(preferencesPathOverride) : null
    const selectedProfileDirectory = preferencesPathOverride
      ? basename(dirname(preferencesPathOverride))
      : resolveChromeProfileDirectory(userDataDirectory)
    const profileDirectories: string[] = preferencesPathOverride && selectedProfileDirectory
      ? [selectedProfileDirectory]
      : listChromeProfileDirectories(userDataDirectory)
    if (!selectedProfileDirectory || profileDirectories.length === 0) {
      return {
        status: 'missing',
        extensionId: CHROME_EXTENSION_ID,
        userDataDirectory,
        selectedProfileDirectory: selectedProfileDirectory ?? undefined,
        profiles: [],
        problem: existsSync(userDataDirectory)
          ? `Could not find a Chrome profile with Preferences in ${userDataDirectory}.`
          : `Chrome user data directory does not exist: ${userDataDirectory}.`
      }
    }

    const profiles = profileDirectories.map((profileDirectory) => {
      const profileStatus = getChromeExtensionInstallStatusForProfile(
        userDataDirectory,
        profileDirectory,
        CHROME_EXTENSION_ID,
        profileDirectory === selectedProfileDirectory && selectedProfilePath ? selectedProfilePath : undefined
      )
      return {
        ...profileStatus,
        selected: profileDirectory === selectedProfileDirectory
      }
    })
    const selectedProfile = profiles.find((profile) => profile.selected) ?? profiles[0]
    const status = selectedProfile.enabled
      ? 'enabled'
      : selectedProfile.installed
        ? 'disabled'
        : 'missing'
    return {
      status,
      extensionId: CHROME_EXTENSION_ID,
      userDataDirectory,
      selectedProfileDirectory,
      selectedProfilePath: selectedProfile.profilePath,
      profiles,
      problem: status === 'enabled'
        ? undefined
        : status === 'disabled'
          ? 'Chrome extension is installed in the selected profile but disabled or not registered.'
          : 'Chrome extension is not installed in the selected profile.'
    }
  } catch (error) {
    return {
      status: 'error',
      extensionId: CHROME_EXTENSION_ID,
      profiles: [],
      problem: error instanceof Error ? error.message : String(error)
    }
  }
}

function readRegistryValue(output: string, valueName: string): string | null {
  let firstRegistryValue: string | null = null
  for (const line of output.split(/\r?\n/)) {
    const match = line.match(/^\s*(.*?)\s+REG_\w+\s+(.+?)\s*$/)
    if (!match) continue
    const name = match[1].trim()
    const value = match[2].replace(/^"(.*)"$/, '$1')
    firstRegistryValue ??= value
    if (name === valueName) return value
    if (valueName === '(Default)' && /^\(.+\)$/.test(name)) return value
  }
  if (valueName === '(Default)') return firstRegistryValue
  return null
}

export function readWindowsRegistryDefaultValueFromOutput(output: string): string | null {
  return readRegistryValue(output, '(Default)')
}

function defaultReadWindowsRegistryDefaultValue(registryKey: string): string | null {
  try {
    const output = execFileSync('reg', ['query', registryKey, '/ve'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore']
    })
    return readRegistryValue(output, '(Default)')
  } catch {
    return null
  }
}

function getNativeHostManifestLocation(
  options: ChromeBrowserUseInspectOptions
): NativeHostManifestLocation | null {
  const env = optionEnv(options)
  const override = trimEnvValue(env, CHROME_NATIVE_HOST_MANIFEST_PATH_ENV)
  if (override) {
    return {
      manifestPath: resolve(override),
      registryKey: null,
      registryManifestPath: null,
      registryKeyExists: null
    }
  }

  const home = optionHomedir(options)
  const platform = optionPlatform(options)
  const manifestName = `${CHROME_NATIVE_HOST_NAME}.json`
  if (platform === 'darwin') {
    return {
      manifestPath: join(home, 'Library', 'Application Support', 'Google', 'Chrome', 'NativeMessagingHosts', manifestName),
      registryKey: null,
      registryManifestPath: null,
      registryKeyExists: null
    }
  }
  if (platform === 'linux') {
    return {
      manifestPath: join(home, '.config', 'google-chrome', 'NativeMessagingHosts', manifestName),
      registryKey: null,
      registryManifestPath: null,
      registryKeyExists: null
    }
  }
  if (platform === 'win32') {
    const registryKeys = CHROME_NATIVE_HOST_REGISTRY_KEY_PREFIXES.map((prefix) =>
      `${prefix}\\${CHROME_NATIVE_HOST_NAME}`
    )
    const readRegistry = options.readWindowsRegistryDefaultValue ?? defaultReadWindowsRegistryDefaultValue
    for (const registryKey of registryKeys) {
      const registryManifestPath = readRegistry(registryKey)
      if (registryManifestPath) {
        return {
          manifestPath: registryManifestPath,
          registryKey,
          registryManifestPath,
          registryKeyExists: true
        }
      }
    }
    return {
      manifestPath: join(home, 'AppData', 'Local', 'Analytix', 'extension', manifestName),
      registryKey: registryKeys.join('; '),
      registryManifestPath: null,
      registryKeyExists: false
    }
  }
  return null
}

function bundledChromeHostBinaryName(platform: NodeJS.Platform): string {
  return platform === 'win32' ? 'extension-host.exe' : 'extension-host'
}

function chromeBundlePlatformDirectory(platform: NodeJS.Platform): string | null {
  if (platform === 'win32') return 'windows'
  if (platform === 'darwin') return 'macos'
  if (platform === 'linux') return 'linux'
  return null
}

function chromeBundleArchDirectory(arch: string = process.arch): string | null {
  if (arch === 'x64' || arch === 'amd64') return 'x64'
  if (arch === 'arm64') return 'arm64'
  return null
}

function bundledManagedChromeRoots(options: ChromeBrowserUseInspectOptions): string[] {
  const resourcesPath = optionResourcesPath(options)
  return uniquePaths([
    ...managedChromeRootsFromEnvironment(options),
    resourcesPath ? join(resourcesPath, 'managed-chrome') : '',
    resourcesPath ? join(resourcesPath, 'app.asar.unpacked', 'managed-chrome') : '',
    resourcesPath ? '' : join(process.cwd(), 'managed-chrome')
  ])
}

function resolveBundledChromeNativeHostPath(options: ChromeBrowserUseInspectOptions): string | null {
  const platform = optionPlatform(options)
  const platformDir = chromeBundlePlatformDirectory(platform)
  const archDir = chromeBundleArchDirectory(options.arch ?? process.arch)
  if (!platformDir || !archDir) return null
  for (const root of bundledManagedChromeRoots(options)) {
    const candidate = join(root, 'chrome', 'extension-host', platformDir, archDir, bundledChromeHostBinaryName(platform))
    if (existsSync(candidate)) return candidate
  }
  return null
}

function windowsNativeHostManifestPath(options: ChromeBrowserUseInspectOptions): string {
  const env = optionEnv(options)
  const localAppData = env.LOCALAPPDATA || join(optionHomedir(options), 'AppData', 'Local')
  return join(localAppData, 'Analytix', 'extension', `${CHROME_NATIVE_HOST_NAME}.json`)
}

export function ensureChromeBrowserUseNativeHostRegistration(
  options: ChromeBrowserUseNativeHostRegistrationOptions = {}
): ChromeBrowserUseNativeHostRegistrationResult {
  if (optionPlatform(options) !== 'win32') {
    return { ok: true, skipped: true, reason: 'Chrome native host registry is only needed on Windows.' }
  }
  const hostPath = resolveBundledChromeNativeHostPath(options)
  if (!hostPath) {
    return {
      ok: false,
      reason: 'Bundled Analytix Chrome native host executable was not found.'
    }
  }

  const manifestPath = windowsNativeHostManifestPath(options)
  const registryKey = `HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\${CHROME_NATIVE_HOST_NAME}`
  const manifest = {
    name: CHROME_NATIVE_HOST_NAME,
    description: 'Analytix Chrome Browser Use native messaging host',
    type: 'stdio',
    path: hostPath,
    allowed_origins: [`chrome-extension://${CHROME_EXTENSION_ID}/`]
  }
  try {
    const mkdir = options.mkdirSync ?? mkdirSync
    const write = options.writeFileSync ?? writeFileSync
    const exec = options.execFileSync ?? execFileSync
    mkdir(dirname(manifestPath), { recursive: true })
    write(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, 'utf8')
    exec('reg', ['add', registryKey, '/ve', '/t', 'REG_SZ', '/d', manifestPath, '/f'], {
      stdio: ['ignore', 'ignore', 'pipe']
    })
    return { ok: true, manifestPath, hostPath, registryKey }
  } catch (error) {
    return {
      ok: false,
      manifestPath,
      hostPath,
      registryKey,
      reason: error instanceof Error ? error.message : String(error)
    }
  }
}

function resolveNativeHostExecutablePath(manifestPath: string, hostPath: unknown): string | undefined {
  if (typeof hostPath !== 'string' || hostPath.trim().length === 0) return undefined
  const trimmed = hostPath.trim()
  return isAbsolute(trimmed) ? resolve(trimmed) : resolve(dirname(manifestPath), trimmed)
}

function fileExists(path: string | undefined): boolean {
  if (!path || !existsSync(path)) return false
  try {
    return statSync(path).isFile()
  } catch {
    return false
  }
}

function describeNativeHostProblem({
  nameMatches,
  hasExpectedOrigin,
  registryMatchesManifestPath,
  typeMatches,
  hostPathExists
}: {
  nameMatches: boolean
  hasExpectedOrigin: boolean
  registryMatchesManifestPath: boolean
  typeMatches: boolean
  hostPathExists: boolean
}): string {
  const problems: string[] = []
  if (!nameMatches) problems.push(`manifest name does not match ${CHROME_NATIVE_HOST_NAME}`)
  if (!typeMatches) problems.push('manifest type must be stdio')
  if (!hostPathExists) problems.push('manifest host executable path does not exist or is not a file')
  if (!hasExpectedOrigin) {
    problems.push(`allowed_origins does not include chrome-extension://${CHROME_EXTENSION_ID}/`)
  }
  if (!registryMatchesManifestPath) {
    problems.push('registry manifest path does not match checked manifest path')
  }
  return problems.join('; ')
}

type BrowserClientProbeProcessResult = {
  ok: boolean
  browserCount?: number
  error?: string
}

const CHROME_BROWSER_CLIENT_PROBE_SCRIPT = `
const realProcess = process;
const stdoutWrite = realProcess.stdout.write.bind(realProcess.stdout);
const { pathToFileURL } = require('node:url');
const { randomUUID } = require('node:crypto');
const { tmpdir } = require('node:os');
const net = require('node:net');

const browserClientPath = realProcess.argv[1];
const pipeConnectTimeoutMs = Number(realProcess.argv[2]) || ${CHROME_BROWSER_PIPE_CONNECT_TIMEOUT_MS};

function writeProbeResult(result) {
  stdoutWrite('\\n${CHROME_BROWSER_PROBE_RESULT_MARKER}' + JSON.stringify(result) + '\\n');
}

function summarizeError(error) {
  if (error instanceof Error) return error.message;
  return String(error);
}

function createNativePipeConnection(pipeName) {
  return new Promise((resolve, reject) => {
    const socket = net.connect(pipeName);
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      socket.destroy();
      reject(new Error('Timed out connecting to native pipe: ' + pipeName));
    }, pipeConnectTimeoutMs);
    const cleanup = () => {
      clearTimeout(timer);
      socket.off('connect', onConnect);
      socket.off('error', onError);
    };
    const onConnect = () => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(socket);
    };
    const onError = (error) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(error);
    };
    socket.once('connect', onConnect);
    socket.once('error', onError);
  });
}

(async () => {
  globalThis.nodeRepl = {
    cwd: realProcess.cwd(),
    tmpDir: tmpdir(),
    env: {
      ...realProcess.env,
      BROWSER_USE_AVAILABLE_BACKENDS: 'chrome'
    },
    requestMeta: {
      'x-codex-turn-metadata': {
        session_id: 'analytix-chrome-probe-' + randomUUID(),
        turn_id: 'analytix-probe-' + randomUUID()
      }
    },
    nativePipe: {
      createConnection: createNativePipeConnection
    },
    emitImage: async () => undefined,
    setResponseMeta: () => undefined
  };

  const module = await import(pathToFileURL(browserClientPath).href + '?analytixProbe=' + Date.now());
  if (typeof module.setupBrowserRuntime !== 'function') {
    throw new Error('browser-client.mjs does not export setupBrowserRuntime');
  }
  await module.setupBrowserRuntime({
    globals: globalThis,
    elicitationDisplayName: 'Chrome Browser Use probe'
  });
  const browsers = await globalThis.agent?.browsers?.list?.();
  writeProbeResult({
    ok: true,
    browserCount: Array.isArray(browsers) ? browsers.length : 0
  });
  realProcess.exit(0);
})().catch((error) => {
  writeProbeResult({
    ok: false,
    error: summarizeError(error)
  });
  realProcess.exit(1);
});
`

function parseBrowserClientProbeResult(output: string): BrowserClientProbeProcessResult | null {
  const line = output
    .split(/\r?\n/)
    .reverse()
    .find((item) => item.startsWith(CHROME_BROWSER_PROBE_RESULT_MARKER))
  if (!line) return null
  try {
    const parsed = JSON.parse(line.slice(CHROME_BROWSER_PROBE_RESULT_MARKER.length)) as BrowserClientProbeProcessResult
    return typeof parsed.ok === 'boolean' ? parsed : null
  } catch {
    return null
  }
}

async function runBrowserClientConnectionProbe(browserClientPath: string): Promise<ChromeBrowserUseConnectionProbe> {
  try {
    const { stdout, stderr } = await execFileAsync(
      process.execPath,
      ['-e', CHROME_BROWSER_CLIENT_PROBE_SCRIPT, browserClientPath, String(CHROME_BROWSER_PIPE_CONNECT_TIMEOUT_MS)],
      {
        env: {
          ...process.env,
          ELECTRON_RUN_AS_NODE: '1'
        },
        encoding: 'utf8',
        timeout: CHROME_BROWSER_PROBE_TIMEOUT_MS,
        windowsHide: true,
        maxBuffer: 96 * 1024
      }
    )
    const result = parseBrowserClientProbeResult(`${stdout}\n${stderr}`)
    if (result?.ok && (result.browserCount ?? 0) > 0) {
      return { status: 'passed', browserClientPath }
    }
    if (result?.ok) {
      return {
        status: 'unavailable',
        browserClientPath,
        problem: 'Browser Use browser-client ran, but no Chrome extension backend completed a real handshake. Start Chrome with the Codex extension enabled to verify the connection.'
      }
    }
    return {
      status: 'unavailable',
      browserClientPath,
      problem: `Browser Use browser-client handshake did not pass: ${result?.error ?? 'no probe result was returned'}.`
    }
  } catch (error) {
    const detail = summarizeExecError(error)
    const output = execErrorOutput(error)
    const result = parseBrowserClientProbeResult(output)
    return {
      status: 'unavailable',
      browserClientPath,
      problem: `Browser Use browser-client handshake did not pass: ${result?.error ?? detail}.`
    }
  }
}

function execErrorOutput(error: unknown): string {
  if (!error || typeof error !== 'object') return ''
  const maybeOutput = error as { stdout?: unknown; stderr?: unknown }
  return `${typeof maybeOutput.stdout === 'string' ? maybeOutput.stdout : ''}\n${typeof maybeOutput.stderr === 'string' ? maybeOutput.stderr : ''}`
}

function summarizeExecError(error: unknown): string {
  if (error instanceof Error) return error.message
  return String(error)
}

export function inspectChromeNativeHostStatus(
  options: ChromeBrowserUseInspectOptions = {}
): ChromeBrowserUseNativeHostStatus {
  const expectedOrigin = `chrome-extension://${CHROME_EXTENSION_ID}/`
  const base = {
    expectedHostName: CHROME_NATIVE_HOST_NAME,
    expectedExtensionId: CHROME_EXTENSION_ID,
    expectedOrigin
  }
  try {
    const location = getNativeHostManifestLocation(options)
    if (!location) {
      return {
        ...base,
        status: 'unsupported',
        problem: `Unsupported platform for Chrome native host manifest check: ${optionPlatform(options)}.`
      }
    }
    if (location.registryKeyExists === false) {
      return {
        ...base,
        status: 'missing',
        manifestPath: location.manifestPath,
        registryKey: location.registryKey,
        registryManifestPath: location.registryManifestPath,
        problem: `Windows native host registry key does not exist: ${location.registryKey}.`
      }
    }
    if (!existsSync(location.manifestPath)) {
      return {
        ...base,
        status: 'missing',
        manifestPath: location.manifestPath,
        registryKey: location.registryKey,
        registryManifestPath: location.registryManifestPath,
        problem: `Native host manifest does not exist: ${location.manifestPath}.`
      }
    }

    const manifest = JSON.parse(readFileSync(location.manifestPath, 'utf8')) as Record<string, unknown>
    const allowedOrigins = Array.isArray(manifest.allowed_origins)
      ? manifest.allowed_origins.filter((value): value is string => typeof value === 'string')
      : []
    const actualHostName = typeof manifest.name === 'string' ? manifest.name : undefined
    const actualType = typeof manifest.type === 'string' ? manifest.type : undefined
    const actualHostPath = resolveNativeHostExecutablePath(location.manifestPath, manifest.path)
    const nameMatches = actualHostName === CHROME_NATIVE_HOST_NAME
    const typeMatches = actualType === 'stdio'
    const hostPathExists = fileExists(actualHostPath)
    const hasExpectedOrigin = allowedOrigins.includes(expectedOrigin)
    const registryMatchesManifestPath = location.registryManifestPath === null ||
      resolve(location.registryManifestPath) === resolve(location.manifestPath)
    const correct = nameMatches && typeMatches && hostPathExists && hasExpectedOrigin && registryMatchesManifestPath
    return {
      ...base,
      status: correct ? 'configured' : 'invalid',
      manifestPath: location.manifestPath,
      registryKey: location.registryKey,
      registryManifestPath: location.registryManifestPath,
      actualHostName,
      actualType,
      actualHostPath,
      hostPathExists,
      allowedOrigins,
      problem: correct
        ? undefined
        : describeNativeHostProblem({
          nameMatches,
          hasExpectedOrigin,
          registryMatchesManifestPath,
          typeMatches,
          hostPathExists
        })
    }
  } catch (error) {
    return {
      ...base,
      status: 'error',
      problem: error instanceof Error ? error.message : String(error)
    }
  }
}

function deriveChromeBrowserUseState(
  extension: ChromeBrowserUseExtensionStatus,
  nativeHost: ChromeBrowserUseNativeHostStatus,
  connectionProbe: ChromeBrowserUseConnectionProbe
): { state: ChromeBrowserUseConnectionState; reason?: string } {
  if (extension.status === 'missing') return { state: 'extensionMissing', reason: extension.problem }
  if (extension.status === 'disabled') return { state: 'extensionDisabled', reason: extension.problem }
  if (extension.status === 'error' || extension.status === 'unknown') {
    return { state: 'diagnosticsUnavailable', reason: extension.problem }
  }
  if (nativeHost.status === 'missing') return { state: 'nativeHostMissing', reason: nativeHost.problem }
  if (nativeHost.status === 'invalid' || nativeHost.status === 'error') {
    return { state: 'nativeHostInvalid', reason: nativeHost.problem }
  }
  if (nativeHost.status === 'unsupported') return { state: 'unsupported', reason: nativeHost.problem }

  if (connectionProbe.status === 'passed') {
    return { state: 'connected' }
  }
  if (connectionProbe.status === 'failed') {
    return {
      state: 'disconnected',
      reason: connectionProbe.problem ?? 'Chrome extension and native host are configured, but the Browser Use connection probe failed.'
    }
  }

  return {
    state: 'configuredNotVerified',
    reason: connectionProbe.problem ??
      'Chrome extension and native host are configured, but analytix has not bundled the Browser Use connection probe yet.'
  }
}

async function probeChromeBrowserUseConnection(
  options: ChromeBrowserUseInspectOptions,
  nativeHost?: ChromeBrowserUseNativeHostStatus
): Promise<ChromeBrowserUseConnectionProbe> {
  if (typeof options.probeConnection === 'function') {
    try {
      return await options.probeConnection()
    } catch (error) {
      return {
        status: 'failed',
        problem: error instanceof Error ? error.message : String(error)
      }
    }
  }

  const browserClient = resolveChromeBrowserClientPath(options, nativeHost)
  if (!browserClient) {
    return {
      status: 'unavailable',
      problem: 'Browser Use browser-client handshake is not bundled in this analytix build.'
    }
  }
  if (!browserClient.exists) {
    return {
      status: 'unavailable',
      browserClientPath: browserClient.path,
      problem: `Configured Browser Use browser-client does not exist: ${browserClient.path}.`
    }
  }

  return runBrowserClientConnectionProbe(browserClient.path)
}

export async function inspectChromeBrowserUseStatus(
  options: ChromeBrowserUseInspectOptions = {}
): Promise<ChromeBrowserUseStatus> {
  const extension = inspectChromeExtensionStatus(options)
  const nativeHost = inspectChromeNativeHostStatus(options)
  const connectionProbe =
    extension.status === 'enabled' && nativeHost.status === 'configured'
      ? await probeChromeBrowserUseConnection(options, nativeHost)
      : {
          status: 'unavailable' as const,
          problem: 'Connection probe requires both the Chrome extension and Native Host to be configured first.'
        }
  const { state, reason } = deriveChromeBrowserUseState(extension, nativeHost, connectionProbe)
  return {
    platform: optionPlatform(options),
    checkedAt: new Date().toISOString(),
    extensionId: CHROME_EXTENSION_ID,
    nativeHostName: CHROME_NATIVE_HOST_NAME,
    connected: state === 'connected',
    state,
    reason,
    extension,
    nativeHost,
    connectionProbe
  }
}

function projectChromeBrowserUseStatus(status: ChromeBrowserUseStatus): ChromeBrowserUseStatus {
  return {
    platform: status.platform,
    checkedAt: status.checkedAt,
    extensionId: CHROME_EXTENSION_ID,
    nativeHostName: CHROME_NATIVE_HOST_NAME,
    connected: status.connected,
    state: status.state,
    extension: {
      status: status.extension.status,
      extensionId: CHROME_EXTENSION_ID,
      profiles: []
    },
    nativeHost: {
      status: status.nativeHost.status,
      expectedHostName: CHROME_NATIVE_HOST_NAME,
      expectedExtensionId: CHROME_EXTENSION_ID,
      expectedOrigin: `chrome-extension://${CHROME_EXTENSION_ID}/`
    },
    connectionProbe: {
      status: status.connectionProbe.status
    }
  }
}

export async function getChromeBrowserUseStatus(
  options: ChromeBrowserUseInspectOptions = {}
): Promise<ChromeBrowserUseStatus> {
  return projectChromeBrowserUseStatus(await inspectChromeBrowserUseStatus(options))
}

function chromeExecutableCandidates(options: ChromeBrowserUseInspectOptions = {}): string[] {
  const home = optionHomedir(options)
  const env = optionEnv(options)
  const platform = optionPlatform(options)
  if (platform === 'darwin') {
    return ['/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']
  }
  if (platform === 'win32') {
    return [
      env.LOCALAPPDATA ? join(env.LOCALAPPDATA, 'Google', 'Chrome', 'Application', 'chrome.exe') : '',
      env.PROGRAMFILES ? join(env.PROGRAMFILES, 'Google', 'Chrome', 'Application', 'chrome.exe') : '',
      env['PROGRAMFILES(X86)'] ? join(env['PROGRAMFILES(X86)'], 'Google', 'Chrome', 'Application', 'chrome.exe') : '',
      join(home, 'AppData', 'Local', 'Google', 'Chrome', 'Application', 'chrome.exe')
    ].filter(Boolean)
  }
  return [
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser'
  ]
}

async function openChromeInternalUrl(url: string): Promise<PathOpenResult> {
  if (process.platform === 'darwin') {
    try {
      await execFileAsync('open', ['-a', 'Google Chrome', url])
      return { ok: true }
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) }
    }
  }

  const candidates = chromeExecutableCandidates()
  for (const candidate of candidates) {
    if (!existsSync(candidate)) continue
    try {
      await execFileAsync(candidate, [url])
      return { ok: true }
    } catch {
      // Try the next known Chrome path.
    }
  }

  return {
    ok: false,
    message: 'Google Chrome executable was not found; cannot open the Chrome extension manager.'
  }
}

export async function openChromeBrowserUseExtensionPage(
  target: ChromeBrowserUseExtensionPageTarget
): Promise<PathOpenResult> {
  try {
    if (target === 'webstore') {
      await shell.openExternal(CHROME_WEBSTORE_URL)
      return { ok: true }
    }
    const result = await openChromeInternalUrl(CHROME_EXTENSION_SETTINGS_URL)
    return result.ok
      ? result
      : { ok: false, message: 'Could not open the Chrome Browser Use extension page.' }
  } catch {
    return { ok: false, message: 'Could not open the Chrome Browser Use extension page.' }
  }
}
