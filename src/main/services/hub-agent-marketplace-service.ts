import { existsSync, lstatSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { mkdir, readFile, rm } from 'node:fs/promises'
import http from 'node:http'
import https from 'node:https'
import { homedir } from 'node:os'
import { basename, dirname, extname, isAbsolute, join, relative, resolve } from 'node:path'
import type {
  HubAgentMarketplaceSyncMode,
  HubAgentMarketplaceSyncResult,
  HubAgentPluginMutationRequest,
  HubAgentPluginMutationResult,
  HubAgentSkillMarkdownRequest,
  HubAgentSkillMarkdownResult,
  HubAgentPluginListItem,
  HubAgentPluginPlatform,
  HubAgentPluginSource,
  HubAgentSkillListItem
} from '../../shared/analytix-api'
import {
  atomicWriteFile,
  ensureFileMode
} from '../../../packages/runtime/src/adapters/file/atomic-write.js'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'
import type { InstalledPluginMarkerV1 } from '../plugin-install-marker'
import { verifyInstalledPluginBindingV1 } from '../plugin-install-verifier'
import {
  currentBundledFundsMaterializationBindingV1,
  type BundledFundsMaterializationBindingV1
} from '../runtime/bundled-funds-materialization'
import { recordHubActivity } from '../hub-activity-observation'

recordHubActivity('moduleLoad')

const MARKETPLACE_NAME = 'analytix-hub'
const DEFAULT_HUB_BASE_URL = 'https://analytix.top'
const DEFAULT_TIMEOUT_MS = 12_000
const HUB_SKILL_MARKER_FILENAME = '.analytix-hub-skill.json'
const MAX_PLUGIN_ICON_BYTES = 2 * 1024 * 1024
const REMOTE_MATERIALIZATION_BLOCKER = 'remote_plugin_materialization_requires_go_archive_authority'
const REMOTE_MATERIALIZATION_MESSAGE =
  'Remote Analytix Hub plugin materialization is unavailable until the Go archive authority is active.'
const BUNDLED_FUNDS_PLUGIN_NAME = 'analytix-fund-analysis'

type JsonRecord = Record<string, unknown>

type RemoteMarketplace = {
  name?: unknown
  interface?: unknown
  platform?: unknown
  plugins?: unknown
}

type RequestOptions = {
  env?: NodeJS.ProcessEnv
  timeoutMs?: number
  hostHeader?: string
  insecureTls?: string
}

type SyncOptions = {
  env?: NodeJS.ProcessEnv
  process?: Pick<NodeJS.Process, 'platform' | 'arch'>
  forceRefresh?: boolean
  mode?: HubAgentMarketplaceSyncMode
  timeoutMs?: number
}

function asText(value: unknown): string {
  return String(value ?? '').trim()
}

function objectValue(value: unknown): JsonRecord {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonRecord : {}
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function truthyFlag(value: unknown): boolean {
  return /^(1|true|on|yes|enabled)$/iu.test(asText(value))
}

function safeSegment(value: unknown): string {
  return asText(value)
    .replace(/[^a-zA-Z0-9._-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 120) || 'item'
}

function exactPluginName(value: unknown): string {
  return typeof value === 'string' &&
    value.length > 0 &&
    value === value.trim() &&
    value === value.toLowerCase() &&
    value !== '.' &&
    value !== '..' &&
    !value.startsWith('.') &&
    safeSegment(value) === value
    ? value
    : ''
}

function exactPluginVersion(value: unknown): string {
  return typeof value === 'string' &&
    value.length > 0 &&
    value === value.trim() &&
    value !== '.' &&
    value !== '..' &&
    !value.startsWith('.') &&
    safeSegment(value) === value
    ? value
    : ''
}

export function hubAgentMarketplacePlatformKey(
  source: Pick<NodeJS.Process, 'platform' | 'arch'> = process
): HubAgentPluginPlatform | '' {
  if (source.platform === 'darwin' && source.arch === 'arm64') return 'mac-arm64'
  if (source.platform === 'darwin') return 'mac-x64'
  if (source.platform === 'win32') return 'win'
  return ''
}

function hubBaseUrl(env: NodeJS.ProcessEnv = process.env): string {
  const configured = asText(
    env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_BASE_URL ||
      env.ANALYTIX_HUB_API_BASE_URL ||
      env.VITE_ANALYTIX_HUB_API_BASE_URL
  ).replace(/\/+$/u, '')
  return configured || DEFAULT_HUB_BASE_URL
}

function marketplaceUrl(env: NodeJS.ProcessEnv, platform: string): string {
  const explicit = asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL)
  if (explicit) return explicit
  return `${hubBaseUrl(env)}/api/agent/plugins/marketplace?${new URLSearchParams({ platform })}`
}

function skillCatalogUrl(env: NodeJS.ProcessEnv, platform: string): string {
  const explicit = asText(env.ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_URL)
  if (explicit) return explicit
  return `${hubBaseUrl(env)}/api/agent/plugins/skills?${new URLSearchParams({ platform })}`
}

function installPolicyUrl(env: NodeJS.ProcessEnv, platform: string): string {
  const explicit = asText(env.ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_URL)
  if (explicit) return explicit
  return `${hubBaseUrl(env)}/api/agent/plugins/install-policy?${new URLSearchParams({ platform })}`
}

export function hubAgentMarketplaceRuntimeHome(): string {
  return join(homedir(), '.analytix')
}

function cacheRoot(runtimeHome: string): string {
  return join(runtimeHome, '.cache', 'analytix-hub-plugins')
}

export function generatedHubMarketplaceRoot(runtimeHome: string): string {
  return join(cacheRoot(runtimeHome), 'marketplaces', MARKETPLACE_NAME)
}

function generatedMarketplaceJsonPath(runtimeHome: string): string {
  return join(generatedHubMarketplaceRoot(runtimeHome), '.agents', 'plugins', 'marketplace.json')
}

function generatedCatalogMarketplacePath(runtimeHome: string): string {
  return join(cacheRoot(runtimeHome), 'catalog', 'marketplace.json')
}

function generatedSkillCatalogPath(runtimeHome: string): string {
  return join(cacheRoot(runtimeHome), 'skills', 'catalog.json')
}

function generatedInstallPolicyPath(runtimeHome: string): string {
  return join(cacheRoot(runtimeHome), 'install-policy.json')
}

function marketplacePluginCacheRoot(runtimeHome: string): string {
  return join(generatedHubMarketplaceRoot(runtimeHome), 'plugins')
}

function installedHubPluginDir(runtimeHome: string, pluginName: string, version: string): string {
  return join(runtimeHome, 'plugins', 'cache', MARKETPLACE_NAME, safeSegment(pluginName), safeSegment(version || 'latest'))
}

function currentFundsBindingForRuntimeHome(
  runtimeHome: string
): BundledFundsMaterializationBindingV1 | null {
  const binding = currentBundledFundsMaterializationBindingV1()
  if (!binding || binding.pluginName !== BUNDLED_FUNDS_PLUGIN_NAME ||
      binding.receipt.pluginVersion !== binding.pluginVersion ||
      binding.index.pluginVersion !== binding.pluginVersion ||
      binding.factToolsEnabled || binding.publishable) return null
  const expectedRoot = resolve(
    runtimeHome,
    'plugins',
    'cache',
    MARKETPLACE_NAME,
    BUNDLED_FUNDS_PLUGIN_NAME,
    binding.pluginVersion
  )
  return resolve(binding.activePluginRoot) === expectedRoot ? binding : null
}

function assertInsideRuntimeHome(targetPath: string, runtimeHome: string): void {
  const root = resolve(runtimeHome)
  const target = resolve(targetPath)
  const rel = relative(root, target)
  if (rel === '' || (!!rel && !rel.startsWith('..') && !isAbsolute(rel))) return
  throw new Error(`Refusing to write Analytix Hub plugin cache outside runtime home: ${target}`)
}

function pathInsideOrEqual(targetPath: string, rootPath: string): boolean {
  const root = resolve(rootPath)
  const target = resolve(targetPath)
  const rel = relative(root, target)
  return rel === '' || (!!rel && !rel.startsWith('..') && !isAbsolute(rel))
}

function readJsonFile(targetPath: string): JsonRecord | null {
  try {
    const parsed = JSON.parse(readFileSync(targetPath, 'utf8')) as unknown
    return objectValue(parsed)
  } catch {
    return null
  }
}

async function writeJsonFileIfChanged(targetPath: string, value: unknown): Promise<boolean> {
  const next = `${JSON.stringify(value, null, 2)}\n`
  if (existsSync(targetPath)) {
    const info = lstatSync(targetPath)
    if (!info.isFile() || info.isSymbolicLink()) throw new Error('Analytix Hub JSON target must be a regular file')
  }
  const previous = existsSync(targetPath) ? await readFile(targetPath, 'utf8') : ''
  if (previous === next) {
    await ensureFileMode(targetPath, 0o600)
    return false
  }
  await mkdir(dirname(targetPath), { recursive: true })
  await atomicWriteFile(targetPath, next, { mode: 0o600 })
  return true
}

function requestOptionsForUrl(url: string, options: RequestOptions = {}): {
  parsed: URL
  headers: Record<string, string>
  rejectUnauthorized: boolean
} {
  const env = options.env ?? process.env
  const parsed = new URL(url)
  const headers: Record<string, string> = {}
  const hostHeader = asText(options.hostHeader) || asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_HOST_HEADER)
  if (hostHeader) headers.Host = hostHeader
  return {
    parsed,
    headers,
    rejectUnauthorized:
      !truthyFlag(options.insecureTls) && !truthyFlag(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_INSECURE_TLS)
  }
}

function requestBuffer(url: string, options: RequestOptions = {}, redirectCount = 0): Promise<Buffer> {
  const { parsed, headers, rejectUnauthorized } = requestOptionsForUrl(url, options)
  const timeoutMs = Number(options.timeoutMs || DEFAULT_TIMEOUT_MS)
  const client = parsed.protocol === 'https:' ? https : http
  recordHubActivity('request')
  return new Promise((resolvePromise, reject) => {
    const request = client.request(
      parsed,
      { method: 'GET', headers, rejectUnauthorized },
      (response) => {
        const status = Number(response.statusCode || 0)
        if (status >= 300 && status < 400 && response.headers.location && redirectCount < 3) {
          response.resume()
          requestBuffer(new URL(response.headers.location, parsed).toString(), options, redirectCount + 1)
            .then(resolvePromise, reject)
          return
        }
        if (status < 200 || status >= 300) {
          response.resume()
          reject(new Error(`HTTP ${status}`))
          return
        }
        const chunks: Buffer[] = []
        response.on('data', (chunk) => chunks.push(Buffer.from(chunk)))
        response.on('end', () => resolvePromise(Buffer.concat(chunks)))
      }
    )
    request.setTimeout(Number.isFinite(timeoutMs) && timeoutMs > 0 ? timeoutMs : DEFAULT_TIMEOUT_MS, () => {
      request.destroy(new Error('request timed out'))
    })
    request.on('error', reject)
    request.end()
  })
}

async function fetchJson(url: string, options: RequestOptions = {}): Promise<JsonRecord> {
  const body = await requestBuffer(url, options)
  return objectValue(JSON.parse(body.toString('utf8')))
}

function replaceUrlBase(url: string, baseUrl: string): string {
  const parsed = new URL(url)
  const base = new URL(baseUrl)
  parsed.protocol = base.protocol
  parsed.host = base.host
  return parsed.toString()
}

function fallbackHubRequestOptions(primaryUrl: string, env: NodeJS.ProcessEnv): (RequestOptions & { url: string }) | null {
  if (truthyFlag(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_DISABLED)) return null
  try {
    new URL(primaryUrl)
  } catch {
    return null
  }
  const explicitFallback = asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_BASE_URL).replace(/\/+$/u, '')
  const fallbackBaseUrl = explicitFallback
  if (!fallbackBaseUrl) return null
  return {
    url: replaceUrlBase(primaryUrl, fallbackBaseUrl),
    hostHeader:
      asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_HOST_HEADER) ||
      asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_HOST_HEADER),
    insecureTls: asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_INSECURE_TLS)
  }
}

async function readRemoteJsonWithFallback(
  primaryUrl: string,
  options: RequestOptions
): Promise<JsonRecord> {
  const env = options.env ?? process.env
  try {
    return await fetchJson(primaryUrl, options)
  } catch (error) {
    const fallback = fallbackHubRequestOptions(primaryUrl, env)
    if (!fallback) throw error
    recordHubActivity('fallback')
    return await fetchJson(fallback.url, {
      ...options,
      hostHeader: fallback.hostHeader,
      insecureTls: fallback.insecureTls
    })
  }
}

async function readRemoteMarketplace(options: SyncOptions & { platform: string }): Promise<RemoteMarketplace> {
  const env = options.env ?? process.env
  const manifestPath = asText(env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_MANIFEST)
  if (manifestPath) return readJsonFile(resolve(manifestPath)) ?? {}
  return await readRemoteJsonWithFallback(marketplaceUrl(env, options.platform), options)
}

async function readRemoteSkillCatalog(options: SyncOptions & { platform: string }): Promise<JsonRecord | null> {
  const env = options.env ?? process.env
  const manifestPath = asText(env.ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_MANIFEST)
  if (manifestPath) return readJsonFile(resolve(manifestPath))
  return await readRemoteJsonWithFallback(skillCatalogUrl(env, options.platform), options)
}

async function readRemoteInstallPolicy(options: SyncOptions & { platform: string }): Promise<JsonRecord | null> {
  const env = options.env ?? process.env
  const manifestPath = asText(env.ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_MANIFEST)
  if (manifestPath) return readJsonFile(resolve(manifestPath))
  return await readRemoteJsonWithFallback(installPolicyUrl(env, options.platform), options)
}

function normalizedPluginPolicy(policy: unknown): JsonRecord {
  const source = objectValue(policy)
  const installation = asText(source.installation) || 'AVAILABLE'
  const authentication = asText(source.authentication)
  return {
    ...source,
    installation,
    authentication: authentication === 'ON_INSTALL' || authentication === 'ON_USE' ? authentication : 'ON_INSTALL'
  }
}

function marketplacePluginName(plugin: JsonRecord): string {
  return hubPluginNameFromId(asText(plugin.name || plugin.id || plugin.pluginId || plugin.plugin_id))
}

function hubPluginNameFromId(value: string): string {
  const id = asText(value)
  if (!id) return ''
  if (id.endsWith(`@${MARKETPLACE_NAME}`)) return asText(id.slice(0, -(`@${MARKETPLACE_NAME}`.length)))
  if (id.includes('@')) return ''
  return id
}

function managedSkillMarkerPath(skillDir: string): string {
  return join(skillDir, HUB_SKILL_MARKER_FILENAME)
}

function readLocalIconDataUrl(pluginDir: string, manifest: JsonRecord, entry: JsonRecord): string | undefined {
  const manifestInterface = objectValue(manifest.interface)
  const entryInterface = objectValue(entry.interface)
  const candidates = [
    asText(entryInterface.composerIcon),
    asText(manifestInterface.composerIcon),
    asText(entryInterface.logo),
    asText(manifestInterface.logo)
  ].filter(Boolean)
  for (const candidate of candidates) {
    if (/^[a-z]+:/iu.test(candidate) || candidate.includes('..')) continue
    const targetPath = resolve(pluginDir, ...candidate.split('/'))
    if (!pathInsideOrEqual(targetPath, pluginDir) || !existsSync(targetPath)) continue
    try {
      const info = statSync(targetPath)
      if (!info.isFile() || info.size > MAX_PLUGIN_ICON_BYTES) continue
      const extension = extname(targetPath).slice(1).toLowerCase()
      const mime = extension === 'svg'
        ? 'image/svg+xml'
        : extension === 'webp'
          ? 'image/webp'
          : extension === 'jpg' || extension === 'jpeg'
            ? 'image/jpeg'
            : 'image/png'
      return `data:${mime};base64,${readFileSync(targetPath).toString('base64')}`
    } catch {
      continue
    }
  }
  return undefined
}

function mcpServersFromPluginConfig(config: JsonRecord): JsonRecord {
  const pluginServers = objectValue(config.mcpServers)
  if (Object.keys(pluginServers).length > 0) return pluginServers

  const directServers = objectValue(config.servers)
  if (Object.keys(directServers).length > 0) return directServers

  const capabilities = objectValue(config.capabilities)
  const mcp = objectValue(capabilities.mcp)
  return objectValue(mcp.servers)
}

function resolvePluginRelativePath(pluginDir: string, value: string): string {
  const targetPath = value === '.' || value.startsWith('./') || value.startsWith('../')
    ? resolve(pluginDir, value)
    : resolve(pluginDir, ...value.split('/'))
  return pathInsideOrEqual(targetPath, pluginDir) ? targetPath : ''
}

function readPluginMcpServerIds(pluginDir: string, manifest: JsonRecord): string[] {
  const ids = new Set<string>()
  const addConfig = (config: JsonRecord): void => {
    for (const id of Object.keys(mcpServersFromPluginConfig(config))) {
      const normalized = id.trim()
      if (normalized) ids.add(normalized)
    }
  }

  const rawManifestMcp = manifest.mcpServers ?? manifest.mcp
  if (rawManifestMcp && typeof rawManifestMcp === 'object' && !Array.isArray(rawManifestMcp)) {
    addConfig({ mcpServers: rawManifestMcp as JsonRecord })
  }

  const fileCandidates = new Set<string>()
  if (typeof rawManifestMcp === 'string') {
    const targetPath = resolvePluginRelativePath(pluginDir, rawManifestMcp)
    if (targetPath) fileCandidates.add(targetPath)
  }
  fileCandidates.add(join(pluginDir, '.mcp.json'))

  for (const filePath of fileCandidates) {
    const parsed = readJsonFile(filePath)
    if (parsed) addConfig(parsed)
  }

  return [...ids].sort((left, right) => left.localeCompare(right))
}

function findCachedMaterializedPluginDir(
  runtimeHome: string,
  pluginName: string,
  version: string,
  sha256: string,
  upstreamMarketplaceName = ''
): string {
  if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME) {
    const binding = currentFundsBindingForRuntimeHome(runtimeHome)
    return binding && (!version || version === binding.pluginVersion) ? binding.activePluginRoot : ''
  }
  const versionPrefix = asText(version) ? safeSegment(version) : ''
  const shaPrefix = asText(sha256).toLowerCase().slice(0, 12)
  const roots = [
    join(marketplacePluginCacheRoot(runtimeHome), safeSegment(pluginName)),
    join(runtimeHome, 'plugins', 'cache', MARKETPLACE_NAME, safeSegment(pluginName))
  ]
  const matches: string[] = []
  for (const root of roots) {
    if (!existsSync(root)) continue
    for (const entry of readdirSync(root, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue
      if (versionPrefix && entry.name !== versionPrefix && !entry.name.startsWith(`${versionPrefix}-`)) continue
      const targetDir = join(root, entry.name)
      if (existsSync(join(targetDir, '.codex-plugin', 'plugin.json'))) matches.push(targetDir)
    }
  }
  return matches.find((item) => shaPrefix && basename(item).includes(shaPrefix)) ?? matches[0] ?? ''
}

function readPluginManifest(runtimeHome: string, entry: JsonRecord): { manifest: JsonRecord; pluginDir: string } {
  const source = objectValue(entry.source)
  const sourcePath = asText(source.path)
  const pluginName = marketplacePluginName(entry)
  let pluginDir = ''
  if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME) {
    pluginDir = currentFundsBindingForRuntimeHome(runtimeHome)?.activePluginRoot ?? ''
  } else if (asText(source.source) === 'local' && sourcePath) {
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    pluginDir = resolve(marketplaceRoot, ...sourcePath.split('/'))
    if (!pathInsideOrEqual(pluginDir, marketplaceRoot)) return { manifest: {}, pluginDir: '' }
  } else {
    const upstream = objectValue(entry.upstream)
    pluginDir = findCachedMaterializedPluginDir(
      runtimeHome,
      pluginName,
      asText(entry.version),
      asText(source.sha256),
      asText(upstream.marketplaceName || entry.upstreamMarketplaceName || entry.marketplaceName)
    )
  }
  if (!pluginDir) return { manifest: {}, pluginDir: '' }
  let manifest: JsonRecord
  try {
    manifest = parseStrictJsonObject(
      readFileSync(join(pluginDir, '.codex-plugin', 'plugin.json')),
      { maxBytes: 256 * 1024, maxDepth: 16, maxTokens: 4096, maxStringBytes: 64 * 1024 }
    )
  } catch {
    return { manifest: {}, pluginDir: '' }
  }
  return {
    manifest,
    pluginDir
  }
}

function requiredPluginNames(installPolicy: JsonRecord): Set<string> {
  return new Set(arrayValue(installPolicy.requiredPlugins)
    .map((item) => {
      const policy = objectValue(item)
      const marketplaceName = asText(policy.marketplaceName || policy.marketplace_name)
      if (marketplaceName && marketplaceName !== MARKETPLACE_NAME) return ''
      return asText(policy.pluginName || policy.plugin_name).toLowerCase()
    })
    .filter(Boolean))
}

function readVerifiedInstalledPluginMarker(
  runtimeHome: string,
  pluginDir: string,
  pluginName: string,
  expectedVersion = ''
): InstalledPluginMarkerV1 | null {
  return verifyInstalledPluginBindingV1({
    runtimeHome,
    pluginDir,
    installCacheRoots: [join(runtimeHome, 'plugins', 'cache')],
    expectedPluginName: pluginName,
    ...(expectedVersion ? { expectedVersion } : {})
  })?.marker ?? null
}

function hubPluginInstalledFromMarker(runtimeHome: string, pluginName: string, version: string): boolean {
  if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME) {
    const hostMaterialization = currentFundsBindingForRuntimeHome(runtimeHome)
    if (!hostMaterialization || (version && version !== hostMaterialization.pluginVersion)) return false
    return verifyInstalledPluginBindingV1({
      runtimeHome,
      pluginDir: hostMaterialization.activePluginRoot,
      installCacheRoots: [join(runtimeHome, 'plugins', 'cache')],
      expectedPluginName: pluginName,
      expectedVersion: hostMaterialization.pluginVersion,
      acceptedKinds: ['hub-install'],
      hostMaterialization
    }) !== null
  }
  const exact = installedHubPluginDir(runtimeHome, pluginName, version)
  if (readVerifiedInstalledPluginMarker(runtimeHome, exact, pluginName, version)) return true
  const pluginRoot = dirname(exact)
  if (!existsSync(pluginRoot)) return false
  for (const entry of readdirSync(pluginRoot, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    if (version && entry.name !== safeSegment(version) && !entry.name.startsWith(`${safeSegment(version)}-local-`)) continue
    if (readVerifiedInstalledPluginMarker(runtimeHome, join(pluginRoot, entry.name), pluginName, version)) return true
  }
  return false
}

function safeRemotePluginIconUrl(value: unknown): string | undefined {
  const candidate = asText(value)
  if (!candidate) return undefined
  try {
    const parsed = new URL(candidate)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? candidate : undefined
  } catch {
    return undefined
  }
}

function bundledFundsMarketplaceEntryProjection(
  entry: JsonRecord,
  manifestInterface: JsonRecord,
  version: string,
  requiresCodexConnector: boolean
): JsonRecord {
  const upstream = objectValue(entry.upstream)
  return {
    name: BUNDLED_FUNDS_PLUGIN_NAME,
    version,
    source: objectValue(entry.source),
    policy: objectValue(entry.policy),
    ...(Object.keys(upstream).length > 0 ? { upstream } : {}),
    ...(requiresCodexConnector ? { requiresCodexConnector: true } : {}),
    category: asText(manifestInterface.category) || 'Other',
    interface: manifestInterface
  }
}

function bundledFundsPluginListItem(
  runtimeHome: string,
  entry: JsonRecord,
  requiredNames: Set<string>,
  marketplace: JsonRecord,
  platform: string
): HubAgentPluginListItem {
  const binding = currentFundsBindingForRuntimeHome(runtimeHome)
  const version = binding?.pluginVersion ?? ''
  const candidate = readPluginManifest(runtimeHome, entry)
  const installed = binding !== null && hubPluginInstalledFromMarker(
    runtimeHome,
    BUNDLED_FUNDS_PLUGIN_NAME,
    version
  )
  const manifest = installed ? candidate.manifest : {}
  const pluginDir = installed ? candidate.pluginDir : ''
  const manifestInterface = objectValue(manifest.interface)
  const upstream = objectValue(entry.upstream)
  const displayName = asText(manifestInterface.displayName) || BUNDLED_FUNDS_PLUGIN_NAME
  const brandColor = asText(manifestInterface.brandColor)
  const requiresCodexConnector = Boolean(entry.requiresCodexConnector || manifest.apps)
  const marketplaceEntry = bundledFundsMarketplaceEntryProjection(
    entry,
    manifestInterface,
    version,
    requiresCodexConnector
  )
  const iconDataUrl = readLocalIconDataUrl(pluginDir, manifest, marketplaceEntry)
  const iconUrl = safeRemotePluginIconUrl(
    manifestInterface.composerIcon || manifestInterface.logo
  )
  return {
    name: BUNDLED_FUNDS_PLUGIN_NAME,
    marketplaceName: MARKETPLACE_NAME,
    upstreamMarketplaceName: asText(
      upstream.marketplaceName || entry.upstreamMarketplaceName || entry.marketplaceName
    ) || MARKETPLACE_NAME,
    pluginName: BUNDLED_FUNDS_PLUGIN_NAME,
    platform: (asText(marketplace.platform) || platform) as HubAgentPluginPlatform,
    version,
    displayName,
    shortDescription: asText(manifestInterface.shortDescription),
    category: asText(manifestInterface.category) || 'Other',
    developerName: asText(manifestInterface.developerName),
    source: objectValue(entry.source) as HubAgentPluginSource,
    policy: objectValue(entry.policy),
    manifest,
    marketplaceEntry,
    mcpServerIds: pluginDir ? readPluginMcpServerIds(pluginDir, manifest) : [],
    ...(iconDataUrl ? { iconDataUrl } : {}),
    ...(!iconDataUrl && iconUrl ? { iconUrl } : {}),
    ...(brandColor ? { brandColor } : {}),
    requiresCodexConnector,
    requiredInstall: requiredNames.has(BUNDLED_FUNDS_PLUGIN_NAME),
    installed
  }
}

function cachedHubMarketplace(runtimeHome: string): JsonRecord {
  return cachedMarketplace(runtimeHome) ?? cachedCatalogMarketplace(runtimeHome) ?? emptyMarketplace(hubAgentMarketplacePlatformKey(process))
}

export async function installHubAgentPlugin(
  runtimeHome = hubAgentMarketplaceRuntimeHome(),
  request: HubAgentPluginMutationRequest
): Promise<HubAgentPluginMutationResult> {
  const pluginName = exactPluginName(request.pluginName)
  if (!pluginName) return { ok: false, message: 'Plugin name is required.' }
  return { ok: false, message: REMOTE_MATERIALIZATION_MESSAGE }
}

export async function uninstallHubAgentPlugin(
  runtimeHome = hubAgentMarketplaceRuntimeHome(),
  request: HubAgentPluginMutationRequest
): Promise<HubAgentPluginMutationResult> {
  const pluginName = exactPluginName(request.pluginName)
  if (!pluginName) return { ok: false, message: 'Plugin name is required.' }
  const installPolicy = readJsonFile(generatedInstallPolicyPath(runtimeHome)) ?? { requiredPlugins: [] }
  if (requiredPluginNames(installPolicy).has(pluginName.toLowerCase())) {
    return {
      ok: false,
      managed: true,
      message: `Analytix Hub plugin "${pluginName}" is managed by the admin install policy. Disable it in Analytix Hub admin console first.`
    }
  }

  const version = request.version ? exactPluginVersion(request.version) : ''
  if (request.version && !version) return { ok: false, message: 'Plugin version is invalid.' }
  const pluginRoot = join(runtimeHome, 'plugins', 'cache', MARKETPLACE_NAME, safeSegment(pluginName))
  assertInsideRuntimeHome(pluginRoot, runtimeHome)
  if (!existsSync(pluginRoot)) {
    return { ok: true, pluginName, version, path: pluginRoot }
  }

  const removed: string[] = []
  const verifiedTargets: Array<{ path: string; marker: InstalledPluginMarkerV1 }> = []
  for (const entry of readdirSync(pluginRoot, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    if (version && entry.name !== safeSegment(version) && !entry.name.startsWith(`${safeSegment(version)}-`)) continue
    const targetDir = join(pluginRoot, entry.name)
    const marker = readVerifiedInstalledPluginMarker(runtimeHome, targetDir, pluginName, version)
    if (!marker) continue
    verifiedTargets.push({ path: targetDir, marker })
  }
  const localRemount = verifiedTargets.find((entry) => entry.marker.kind === 'local-remount')
  if (localRemount) {
    return {
      ok: false,
      message: `Analytix Hub plugin "${pluginName}" is an authorized local remount and cannot be removed by ordinary uninstall.`
    }
  }
  for (const target of verifiedTargets) {
    const targetDir = target.path
    assertInsideRuntimeHome(targetDir, runtimeHome)
    await rm(targetDir, { recursive: true, force: true })
    removed.push(targetDir)
  }
  return { ok: true, pluginName, version, path: removed[0] ?? pluginRoot }
}

function listPluginsFromMarketplace(
  runtimeHome: string,
  marketplace: JsonRecord,
  installPolicy: JsonRecord,
  platform: string
): HubAgentPluginListItem[] {
  const requiredNames = requiredPluginNames(installPolicy)

  return arrayValue(marketplace.plugins).map((raw): HubAgentPluginListItem | null => {
    const entry = objectValue(raw)
    const pluginName = marketplacePluginName(entry)
    if (!pluginName) return null
    if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME) {
      return bundledFundsPluginListItem(runtimeHome, entry, requiredNames, marketplace, platform)
    }
    const { manifest, pluginDir } = readPluginManifest(runtimeHome, entry)
    const entryInterface = objectValue(entry.interface)
    const manifestInterface = objectValue(manifest.interface)
    const upstream = objectValue(entry.upstream)
    const displayName = asText(entryInterface.displayName || manifestInterface.displayName || pluginName)
    const brandColor = asText(entryInterface.brandColor || manifestInterface.brandColor)
    const iconDataUrl = readLocalIconDataUrl(pluginDir, manifest, entry)
    const iconCandidate = asText(
      entryInterface.composerIcon ||
      manifestInterface.composerIcon ||
      entryInterface.logo ||
      manifestInterface.logo
    )
    const iconUrl = /^[a-z]+:/iu.test(iconCandidate) ? iconCandidate : undefined
    const requiredInstall = requiredNames.has(pluginName.toLowerCase())
    const version = asText(entry.version || manifest.version)
    return {
      name: asText(entry.name) || pluginName,
      marketplaceName: MARKETPLACE_NAME,
      upstreamMarketplaceName: asText(upstream.marketplaceName || entry.upstreamMarketplaceName || entry.marketplaceName) || MARKETPLACE_NAME,
      pluginName,
      platform: (asText(marketplace.platform) || platform) as HubAgentPluginPlatform,
      version,
      displayName,
      shortDescription: asText(entryInterface.shortDescription || manifestInterface.shortDescription),
      category: asText(entry.category || entryInterface.category || manifestInterface.category) || 'Other',
      developerName: asText(entryInterface.developerName || manifestInterface.developerName),
      source: objectValue(entry.source) as HubAgentPluginSource,
      policy: objectValue(entry.policy),
      manifest,
      marketplaceEntry: entry,
      mcpServerIds: pluginDir ? readPluginMcpServerIds(pluginDir, manifest) : [],
      ...(iconDataUrl ? { iconDataUrl } : {}),
      ...(!iconDataUrl && iconUrl ? { iconUrl } : {}),
      ...(brandColor ? { brandColor } : {}),
      requiresCodexConnector: Boolean(entry.requiresCodexConnector || manifest.apps),
      requiredInstall,
      installed: hubPluginInstalledFromMarker(runtimeHome, pluginName, version)
    }
  }).filter((item): item is HubAgentPluginListItem => Boolean(item))
}

function listSkillsFromCatalog(
  runtimeHome: string,
  skillCatalog: JsonRecord,
  installPolicy: JsonRecord,
  platform: string
): HubAgentSkillListItem[] {
  const requiredNames = new Set(arrayValue(installPolicy.requiredSkills)
    .map((item) => asText(objectValue(item).skillName || objectValue(item).name).toLowerCase())
    .filter(Boolean))
  return arrayValue(skillCatalog.skills).map((raw): HubAgentSkillListItem | null => {
    const skill = objectValue(raw)
    const skillName = asText(skill.skillName || skill.name)
    if (!skillName) return null
    const iface = objectValue(skill.interface)
    const definition = objectValue(skill.skill)
    const pluginName = asText(skill.pluginName)
    const fundsBinding = pluginName === BUNDLED_FUNDS_PLUGIN_NAME
      ? currentFundsBindingForRuntimeHome(runtimeHome)
      : null
    return {
      id: asText(skill.id) || `${asText(skill.pluginName)}:${skillName}`,
      skillName,
      platform: (asText(skill.platform) || platform || 'all') as HubAgentSkillListItem['platform'],
      displayName: asText(skill.displayName || iface.displayName || skillName),
      shortDescription: asText(skill.shortDescription || iface.shortDescription || definition.description),
      scope: (asText(skill.scope) || 'plugin') as HubAgentSkillListItem['scope'],
      sourceKind: (asText(skill.sourceKind) || 'plugin') as HubAgentSkillListItem['sourceKind'],
      upstreamMarketplaceName: asText(skill.upstreamMarketplaceName) || MARKETPLACE_NAME,
      pluginName,
      version: fundsBinding?.pluginVersion ?? asText(skill.version),
      category: asText(skill.category) || 'Other',
      iconUrl: asText(skill.iconUrl) || undefined,
      skillPath: asText(skill.skillPath),
      requiredInstall: skill.requiredInstall === true || requiredNames.has(skillName.toLowerCase()),
      installed: pluginName === BUNDLED_FUNDS_PLUGIN_NAME
        ? fundsBinding !== null && hubPluginInstalledFromMarker(runtimeHome, pluginName, fundsBinding.pluginVersion)
        : skill.requiredInstall === true || requiredNames.has(skillName.toLowerCase())
    }
  }).filter((item): item is HubAgentSkillListItem => Boolean(item))
}

function cachedMarketplace(runtimeHome: string): JsonRecord | null {
  const marketplace = readJsonFile(generatedMarketplaceJsonPath(runtimeHome))
  return asText(marketplace?.name) === MARKETPLACE_NAME ? marketplace : null
}

function cachedCatalogMarketplace(runtimeHome: string): JsonRecord | null {
  const marketplace = readJsonFile(generatedCatalogMarketplacePath(runtimeHome))
  return asText(marketplace?.name) === MARKETPLACE_NAME ? marketplace : null
}

function emptyMarketplace(platform: string): JsonRecord {
  return {
    name: MARKETPLACE_NAME,
    interface: { displayName: 'Analytix Hub' },
    platform,
    plugins: []
  }
}

function marketplaceFromRemoteCatalog(remote: RemoteMarketplace, platform: string): JsonRecord {
  return {
    name: MARKETPLACE_NAME,
    interface: objectValue(remote.interface).displayName ? remote.interface : { displayName: 'Analytix Hub' },
    platform: asText(remote.platform) || platform,
    plugins: arrayValue(remote.plugins)
      .map((raw) => objectValue(raw))
      .filter((entry) => Boolean(marketplacePluginName(entry)))
      .map((entry) => ({
        ...entry,
        name: marketplacePluginName(entry),
        policy: normalizedPluginPolicy(entry.policy),
        category: asText(entry.category) || asText(objectValue(entry.interface).category) || 'Productivity'
      }))
  }
}

type SkillMarkdownCandidate = {
  path: string
  source: 'managed-skill' | 'plugin-cache'
  marker?: JsonRecord
  pluginDir?: string
}

function pathParts(value: string): string[] {
  return value.split(/[\\/]+/u).filter(Boolean)
}

function normalizedSkillMatchToken(value: string): string {
  return asText(value)
    .replace(/^#+\s*/u, '')
    .replace(/\s*#+$/u, '')
    .replace(/[\\`*_~]/g, '')
    .replace(/^\$/u, '')
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fa5]+/gu, '')
}

function unquoteYamlValue(value: string): string {
  const trimmed = value.trim()
  if (
    (trimmed.startsWith('"') && trimmed.endsWith('"')) ||
    (trimmed.startsWith("'") && trimmed.endsWith("'"))
  ) {
    return trimmed.slice(1, -1)
  }
  return trimmed
}

function frontmatterValue(content: string, key: string): string {
  const normalized = content.replace(/\r\n/g, '\n')
  if (!normalized.startsWith('---\n')) return ''
  const end = normalized.indexOf('\n---', 4)
  if (end === -1) return ''
  const frontmatter = normalized.slice(4, end)
  const pattern = new RegExp(`^${key}\\s*:\\s*(.+)$`, 'imu')
  const match = frontmatter.match(pattern)
  return match?.[1] ? unquoteYamlValue(match[1]) : ''
}

function firstMarkdownHeading(content: string): string {
  for (const line of content.replace(/\r\n/g, '\n').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const match = trimmed.match(/^#\s+(.+)$/u)
    return match?.[1]?.trim() ?? ''
  }
  return ''
}

function skillExpectedTokens(request: HubAgentSkillMarkdownRequest): Set<string> {
  const tokens = [
    request.skillName,
    request.displayName,
    request.id,
    request.skillPath ? basename(dirname(request.skillPath)) : '',
    request.skillPath ? basename(request.skillPath).replace(/\.[^/.]+$/u, '') : ''
  ]
    .map((item) => normalizedSkillMatchToken(item ?? ''))
    .filter(Boolean)
  return new Set(tokens)
}

function scoreSkillMarkdownCandidate(
  candidate: SkillMarkdownCandidate,
  request: HubAgentSkillMarkdownRequest,
  expectedTokens: Set<string>,
  totalCandidates: number
): number {
  let score = 0
  const content = readFileSync(candidate.path, 'utf8')
  const skillPath = asText(request.skillPath)
  if (skillPath && candidate.pluginDir) {
    const expectedPath = resolve(candidate.pluginDir, ...pathParts(skillPath))
    if (resolve(candidate.path) === expectedPath) score += 1000
  }
  const markerSkillName = normalizedSkillMatchToken(asText(candidate.marker?.skillName))
  const markerPluginName = asText(candidate.marker?.pluginName).toLowerCase()
  const markerVersion = asText(candidate.marker?.version)
  if (markerSkillName && expectedTokens.has(markerSkillName)) score += 500
  if (markerPluginName && markerPluginName === asText(request.pluginName).toLowerCase()) score += 80
  if (markerVersion && markerVersion === asText(request.version)) score += 40

  const frontmatterName = normalizedSkillMatchToken(frontmatterValue(content, 'name'))
  const directoryName = normalizedSkillMatchToken(basename(dirname(candidate.path)))
  const heading = normalizedSkillMatchToken(firstMarkdownHeading(content))
  const pathToken = normalizedSkillMatchToken(relative(candidate.pluginDir ?? dirname(candidate.path), candidate.path))
  if (frontmatterName && expectedTokens.has(frontmatterName)) score += 350
  if (directoryName && expectedTokens.has(directoryName)) score += 300
  if (heading && expectedTokens.has(heading)) score += 260
  if ([...expectedTokens].some((token) => token && pathToken.includes(token))) score += 120
  if (totalCandidates === 1 && asText(request.pluginName)) score += 60
  return score
}

function collectSkillMarkdownFiles(rootDir: string): string[] {
  if (!existsSync(rootDir)) return []
  const results: string[] = []
  const queue = [rootDir]
  while (queue.length > 0 && results.length < 500) {
    const current = queue.shift()
    if (!current || !existsSync(current)) continue
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const targetPath = join(current, entry.name)
      if (entry.isDirectory()) {
        queue.push(targetPath)
      } else if (entry.isFile() && entry.name.toLowerCase() === 'skill.md') {
        results.push(targetPath)
      }
    }
  }
  return results.sort()
}

function managedSkillMarkdownCandidates(
  runtimeHome: string,
  request: HubAgentSkillMarkdownRequest
): SkillMarkdownCandidate[] {
  const skillsRoot = join(runtimeHome, 'skills')
  if (!existsSync(skillsRoot)) return []
  const pluginName = asText(request.pluginName).toLowerCase()
  if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME) return []
  const version = asText(request.version)
  const candidates: SkillMarkdownCandidate[] = []
  for (const entry of readdirSync(skillsRoot, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    const skillDir = join(skillsRoot, entry.name)
    const marker = readJsonFile(managedSkillMarkerPath(skillDir))
    if (!marker) continue
    if (pluginName && asText(marker.pluginName).toLowerCase() !== pluginName) continue
    if (version && asText(marker.version) && asText(marker.version) !== version) continue
    const markdownPath = join(skillDir, 'SKILL.md')
    if (existsSync(markdownPath)) {
      candidates.push({ path: markdownPath, source: 'managed-skill', marker })
    }
  }
  return candidates
}

function pluginSkillMarkdownCandidates(
  runtimeHome: string,
  request: HubAgentSkillMarkdownRequest
): SkillMarkdownCandidate[] {
  const pluginName = asText(request.pluginName)
  if (!pluginName) return []
  const fundsBinding = pluginName === BUNDLED_FUNDS_PLUGIN_NAME
    ? currentFundsBindingForRuntimeHome(runtimeHome)
    : null
  if (pluginName === BUNDLED_FUNDS_PLUGIN_NAME &&
      (!fundsBinding || (asText(request.version) && asText(request.version) !== fundsBinding.pluginVersion))) return []
  const pluginDir = fundsBinding?.activePluginRoot ?? findCachedMaterializedPluginDir(
    runtimeHome,
    pluginName,
    asText(request.version),
    '',
    asText(request.upstreamMarketplaceName)
  )
  if (!pluginDir) return []
  const candidates = new Set<string>()
  const skillPath = asText(request.skillPath)
  if (skillPath) {
    const expectedPath = resolve(pluginDir, ...pathParts(skillPath))
    if (pathInsideOrEqual(expectedPath, pluginDir) && existsSync(expectedPath)) {
      candidates.add(expectedPath)
    }
  }
  for (const file of collectSkillMarkdownFiles(join(pluginDir, 'skills'))) {
    if (pathInsideOrEqual(file, pluginDir)) candidates.add(file)
  }
  return [...candidates].sort().map((path) => ({ path, source: 'plugin-cache' as const, pluginDir }))
}

export async function readHubAgentSkillMarkdown(
  runtimeHome = hubAgentMarketplaceRuntimeHome(),
  request: HubAgentSkillMarkdownRequest
): Promise<HubAgentSkillMarkdownResult> {
  const candidates = [
    ...managedSkillMarkdownCandidates(runtimeHome, request),
    ...pluginSkillMarkdownCandidates(runtimeHome, request)
  ].filter((candidate) => {
    try {
      const info = statSync(candidate.path)
      return info.isFile() && info.size <= 1_000_000
    } catch {
      return false
    }
  })
  if (candidates.length === 0) {
    return { ok: false, message: 'Skill contents are not available in the local Hub cache.' }
  }
  const expectedTokens = skillExpectedTokens(request)
  const scored = candidates
    .map((candidate) => ({
      candidate,
      score: scoreSkillMarkdownCandidate(candidate, request, expectedTokens, candidates.length)
    }))
    .sort((left, right) => right.score - left.score || left.candidate.path.localeCompare(right.candidate.path))
  const best = scored[0]
  if (!best || best.score <= 0) {
    return { ok: false, message: 'Unable to match this Skill to a local SKILL.md file.' }
  }
  return {
    ok: true,
    path: best.candidate.path,
    content: await readFile(best.candidate.path, 'utf8'),
    source: best.candidate.source
  }
}

function createSyncResult(params: {
  runtimeHome: string
  marketplaceRoot: string
  platform: HubAgentPluginPlatform | ''
  changed: boolean
  marketplace: JsonRecord
  skillCatalog: JsonRecord
  installPolicy: JsonRecord
  errors: string[]
  copiedSkills?: string[]
  removedSkills?: string[]
}): HubAgentMarketplaceSyncResult {
  return {
    ok: true,
    platform: params.platform,
    marketplaceName: MARKETPLACE_NAME,
    marketplaceRoot: params.marketplaceRoot,
    generatedAt: new Date().toISOString(),
    changed: params.changed,
    plugins: listPluginsFromMarketplace(params.runtimeHome, params.marketplace, params.installPolicy, params.platform),
    skills: listSkillsFromCatalog(params.runtimeHome, params.skillCatalog, params.installPolicy, params.platform),
    requiredPluginNames: arrayValue(params.installPolicy.requiredPlugins)
      .map((item) => asText(objectValue(item).pluginName || objectValue(item).plugin_name))
      .filter(Boolean),
    requiredSkillNames: arrayValue(params.installPolicy.requiredSkills)
      .map((item) => asText(objectValue(item).skillName || objectValue(item).name))
      .filter(Boolean),
    copiedSkills: params.copiedSkills ?? [],
    removedSkills: params.removedSkills ?? [],
    errors: params.errors
  }
}

export function syncHubAgentMarketplace(
  runtimeHome = hubAgentMarketplaceRuntimeHome(),
  options: SyncOptions = {}
): Promise<HubAgentMarketplaceSyncResult> {
  const mode = options.mode ?? 'full'
  const platform = hubAgentMarketplacePlatformKey(options.process ?? process)
  if (mode === 'full') {
    return Promise.resolve({
      ok: false,
      message: REMOTE_MATERIALIZATION_MESSAGE,
      platform,
      errors: [REMOTE_MATERIALIZATION_BLOCKER]
    })
  }
  return syncHubAgentMarketplaceOnce(runtimeHome, { ...options, mode })
}

async function syncHubAgentMarketplaceOnce(
  runtimeHome = hubAgentMarketplaceRuntimeHome(),
  options: SyncOptions = {}
): Promise<HubAgentMarketplaceSyncResult> {
  const env = options.env ?? process.env
  const mode = options.mode ?? 'full'
  const platform = hubAgentMarketplacePlatformKey(options.process ?? process)
  if (mode === 'full') {
    return {
      ok: false,
      message: REMOTE_MATERIALIZATION_MESSAGE,
      platform,
      errors: [REMOTE_MATERIALIZATION_BLOCKER]
    }
  }
  if (!platform) {
    return { ok: false, message: 'Analytix Hub plugins are not available on this platform.', platform }
  }

  await mkdir(runtimeHome, { recursive: true })
  const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
  const errors: string[] = []
  let changed = false
  let marketplace = cachedMarketplace(runtimeHome) ?? cachedCatalogMarketplace(runtimeHome) ?? emptyMarketplace(platform)
  let skillCatalog = readJsonFile(generatedSkillCatalogPath(runtimeHome)) ?? { platform, generatedAt: new Date().toISOString(), skills: [] }
  let installPolicy = readJsonFile(generatedInstallPolicyPath(runtimeHome)) ?? {
    platform,
    generatedAt: new Date().toISOString(),
    requiredPlugins: [],
    requiredSkills: []
  }

  if (mode === 'cache') {
    return createSyncResult({
      runtimeHome,
      marketplaceRoot,
      platform,
      changed: false,
      marketplace,
      skillCatalog,
      installPolicy,
      errors: []
    })
  }

  const remoteMarketplacePromise = readRemoteMarketplace({ ...options, env, platform })
  const remoteSkillCatalogPromise = readRemoteSkillCatalog({ ...options, env, platform })
  const remoteInstallPolicyPromise = readRemoteInstallPolicy({ ...options, env, platform })
  const [remoteMarketplaceResult, skillCatalogResult, installPolicyResult] = await Promise.allSettled([
    remoteMarketplacePromise,
    remoteSkillCatalogPromise,
    remoteInstallPolicyPromise
  ])

  if (skillCatalogResult.status === 'fulfilled' && skillCatalogResult.value) {
    skillCatalog = skillCatalogResult.value
    changed = await writeJsonFileIfChanged(generatedSkillCatalogPath(runtimeHome), {
      platform: asText(skillCatalog.platform) || platform,
      generatedAt: asText(skillCatalog.generatedAt) || new Date().toISOString(),
      skills: arrayValue(skillCatalog.skills)
    }) || changed
  } else if (skillCatalogResult.status === 'rejected') {
    errors.push('skills: unavailable')
  }

  if (installPolicyResult.status === 'fulfilled' && installPolicyResult.value) {
    installPolicy = installPolicyResult.value
    changed = await writeJsonFileIfChanged(generatedInstallPolicyPath(runtimeHome), installPolicy) || changed
  } else if (installPolicyResult.status === 'rejected') {
    errors.push('install-policy: unavailable')
  }

  if (remoteMarketplaceResult.status === 'fulfilled') {
    const remote = remoteMarketplaceResult.value
    marketplace = marketplaceFromRemoteCatalog(remote, platform)
    changed = await writeJsonFileIfChanged(generatedCatalogMarketplacePath(runtimeHome), marketplace) || changed
  } else {
    errors.push('marketplace: unavailable')
    if (!cachedMarketplace(runtimeHome) && !cachedCatalogMarketplace(runtimeHome)) {
      changed = await writeJsonFileIfChanged(generatedCatalogMarketplacePath(runtimeHome), marketplace) || changed
    }
  }

  return createSyncResult({
    runtimeHome,
    marketplaceRoot,
    platform,
    changed,
    marketplace,
    skillCatalog,
    installPolicy,
    errors
  })
}
