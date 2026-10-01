import type { ReactElement, ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import analytixAppIcon512 from '../../../asset/brand/analytix-app-icon-512.png'
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  FolderOpen,
  Loader2,
  MessageCircleMore,
  MoreHorizontal,
  Package,
  Plus,
  RefreshCw,
  Search,
  Settings,
  X
} from 'lucide-react'
import { rendererRuntimeClient } from '../agent/runtime-client'
import {
  loadPreferredSkillRootId,
  savePreferredSkillRootId,
  type SkillRootId
} from '../lib/skill-root-preference'
import { readBrowserStorageItem, writeBrowserStorageItem } from '../lib/browser-storage'
import { desktopQueryCache } from '../lib/desktop-query-cache'
import { normalizeWorkspaceRoot } from '../lib/workspace-path'
import { usePersistentStringState } from '../lib/use-persistent-string-state'
import { getProvider } from '../agent/registry'
import type {
  HubAgentMarketplaceSyncMode,
  HubAgentMarketplaceSyncResult,
  HubAgentPluginMutationRequest,
  HubAgentSkillMarkdownRequest,
  HubAgentPluginListItem,
  HubAgentSkillListItem,
  SkillListItem,
  SkillRootListItem
} from '@shared/analytix-api'
import type {
  CoreRuntimeToolDiagnosticsJson
} from '../agent/analytix-contract'
import { redactSecretText } from '@shared/secret-redaction'
import { useChatStore } from '../store/chat-store'
import { TabButton, type MarketplaceNotice } from './PluginMarketplaceParts'
import { BuiltinOfficePlugins } from './BuiltinOfficePlugins'

type CatalogTab = 'plugin' | 'skill'
type PluginKind = 'mcp' | 'skill'
type PluginFilter = 'all' | 'recommended' | 'installed'
type HubMarketplaceSyncOptions = {
  forceRefresh?: boolean
  mode?: HubAgentMarketplaceSyncMode
}

type Notice = MarketplaceNotice

export type MarketplaceItem = {
  id: string
  kind: PluginKind
  titleKey?: string
  descriptionKey?: string
  title?: string
  description?: string
  iconSrc?: string
  iconBleed?: boolean
  group: 'recommended' | 'personal'
  sourceLabel?: string
  detail?: string
  statusTone?: 'default' | 'success' | 'warning' | 'error'
  systemManaged?: boolean
  mcpConfig?: (workspaceRoot: string) => JsonRecord
  mcpConfigured?: boolean
  mcpRuntimeSeen?: boolean
  skillInstructions?: string
  skillRootPath?: string
  skillEntryPath?: string
}

type JsonRecord = Record<string, unknown>

export type MarketplaceInstallState = {
  configuredMcpIds: Set<string>
  discoveredSkillIds: Set<string>
  installedIds: string[]
  mcpConfigText: string
}

type SkillRootOption = {
  id: SkillRootId
  label: string
  path: string
  scope: 'project' | 'global'
  enabled: boolean
  exists: boolean
  skillCount: number
}

const INSTALLED_STORAGE_KEY = 'analytix.installedPlugins'
const MARKETPLACE_TAB_STORAGE_KEY = 'analytix.pluginMarketplace.tab.v1'
const MARKETPLACE_FILTER_STORAGE_KEY = 'analytix.pluginMarketplace.filter.v1'
const MARKETPLACE_HUB_CATEGORY_STORAGE_KEY = 'analytix.pluginMarketplace.hubCategory.v1'
const MARKETPLACE_HUB_SOURCE_STORAGE_KEY = 'analytix.pluginMarketplace.hubSource.v1'
const GUI_SCHEDULE_MCP_SERVER_ID = 'gui_schedule'
const HUB_SYNC_INTERVAL_MS = 30_000
const SKILL_LIST_STALE_MS = 60_000
const SKILL_ROOTS_STALE_MS = 60_000
const CATALOG_TABS = ['plugin', 'skill'] as const
const PLUGIN_FILTERS = ['all', 'recommended', 'installed'] as const

function loadInstalledPlugins(): string[] {
  try {
    const raw = readBrowserStorageItem(INSTALLED_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []
  } catch {
    return []
  }
}

function saveInstalledPlugins(ids: string[]): void {
  writeBrowserStorageItem(INSTALLED_STORAGE_KEY, JSON.stringify([...new Set(ids)]))
}

function storageKey(kind: PluginKind, id: string): string {
  return `${kind}:${id}`
}

export function marketplaceItemIsInstalled(
  item: Pick<MarketplaceItem, 'kind' | 'id' | 'systemManaged'> & { group?: MarketplaceItem['group'] },
  state: MarketplaceInstallState
): boolean {
  if (item.systemManaged) return true
  if (item.kind === 'mcp') {
    return state.configuredMcpIds.has(item.id) || mcpConfigHasServer(state.mcpConfigText, item.id)
  }
  if (item.group === 'personal') return true
  if (state.discoveredSkillIds.has(item.id)) return true
  return state.installedIds.includes(storageKey(item.kind, item.id))
}

function localCatalogEntryKey(item: Pick<MarketplaceItem, 'kind' | 'id'>): string {
  return `local:${storageKey(item.kind, item.id)}`
}

function normalizeSkillId(id: string): string {
  return id.trim().replace(/^\/?skill:/i, '').trim()
}

function normalizeDisabledSkillIds(ids: unknown): string[] {
  if (!Array.isArray(ids)) return []
  return [...new Set(ids
    .filter((id): id is string => typeof id === 'string')
    .map(normalizeSkillId)
    .filter(Boolean))]
}

function normalizePluginId(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function isJsonRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function parseMcpJsonConfig(content: string): JsonRecord {
  const trimmed = content.trim()
  if (!trimmed) return {}
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`MCP config must be JSON: ${message}`)
  }
  if (!isJsonRecord(parsed)) {
    throw new Error('MCP config must be a JSON object.')
  }
  return parsed
}

function buildStdioMcpServer(
  command: string,
  args: string[],
  options: {
    trustScope?: 'workspace' | 'user'
    trustedWorkspaceRoots?: string[]
    env?: JsonRecord
  } = {}
): JsonRecord {
  const trustScope = options.trustScope ?? 'user'
  return {
    enabled: true,
    transport: 'stdio',
    command,
    args,
    env: options.env ?? {},
    trustScope,
    ...(trustScope === 'workspace'
      ? {
          trustedWorkspaceRoots: options.trustedWorkspaceRoots?.length
            ? options.trustedWorkspaceRoots
            : ['/path/to/workspace']
        }
      : {}),
    timeoutMs: 30_000
  }
}

export function buildMcpConfig(
  id: string,
  command: string,
  args: string[],
  options?: Parameters<typeof buildStdioMcpServer>[2]
): JsonRecord {
  return {
    servers: {
      [id]: buildStdioMcpServer(command, args, options)
    }
  }
}

function mcpServersFromConfig(config: JsonRecord): JsonRecord {
  if (isJsonRecord(config.servers)) return config.servers
  const capabilities = isJsonRecord(config.capabilities) ? config.capabilities : undefined
  const mcp = isJsonRecord(capabilities?.mcp) ? capabilities.mcp : undefined
  return isJsonRecord(mcp?.servers) ? mcp.servers : {}
}

function mcpServerConfigFromText(content: string, id: string): JsonRecord | undefined {
  try {
    const server = mcpServersFromConfig(parseMcpJsonConfig(content))[id]
    return isJsonRecord(server) ? server : undefined
  } catch {
    return undefined
  }
}

function mcpServerEnabledFromConfig(config: JsonRecord | undefined): boolean {
  return !(config?.enabled === false || config?.disabled === true)
}

function mcpServerDescription(server: JsonRecord | undefined, fallback: string): string {
  if (!server) return fallback
  const transport = typeof server.transport === 'string' ? server.transport : ''
  const command = typeof server.command === 'string' ? server.command : ''
  const url = typeof server.url === 'string' ? server.url : ''
  const status = typeof server.status === 'string' ? server.status : ''
  const failureCode = server.failureCode === 'connection_failed' ? 'connection failed' : ''
  const toolCount = typeof server.toolCount === 'number' && Number.isFinite(server.toolCount)
    ? server.toolCount
    : undefined
  const parts = [
    status ? `status: ${status}` : '',
    transport,
    command || url,
    toolCount != null ? `${toolCount} tools` : '',
    failureCode ? `error: ${failureCode}` : ''
  ].filter(Boolean)
  return parts.length ? redactSecretText(parts.join(' · ')) : fallback
}

function mcpServerStatus(diagnostic: JsonRecord | undefined, config: JsonRecord | undefined): string {
  const diagnosticStatus = typeof diagnostic?.status === 'string' ? diagnostic.status : ''
  if (diagnosticStatus) return diagnosticStatus
  if (config?.enabled === false || config?.disabled === true) return 'disabled'
  return ''
}

function mcpStatusTone(status: string): MarketplaceItem['statusTone'] {
  if (status === 'connected' || status === 'available') return 'success'
  if (status === 'error' || status === 'unavailable') return 'error'
  if (status === 'disabled') return 'warning'
  return 'default'
}

export function mcpConfigHasServer(content: string, id: string): boolean {
  try {
    return Object.prototype.hasOwnProperty.call(mcpServersFromConfig(parseMcpJsonConfig(content)), id)
  } catch {
    return false
  }
}

export function customMcpConfigFragment(id: string, raw: string, fallback: JsonRecord): JsonRecord {
  const trimmed = raw.trim()
  if (!trimmed) return fallback
  const parsed = parseMcpJsonConfig(trimmed)
  if (isJsonRecord(parsed.servers)) return parsed
  if (isJsonRecord(parsed.capabilities)) {
    const mcp = isJsonRecord(parsed.capabilities.mcp) ? parsed.capabilities.mcp : undefined
    if (isJsonRecord(mcp?.servers)) return { servers: mcp.servers }
  }
  if (parsed.command !== undefined || parsed.url !== undefined || parsed.transport !== undefined) {
    return { servers: { [id]: parsed } }
  }
  throw new Error('MCP JSON config must include a servers object or a single server object.')
}

export function mergeMcpJsonConfig(content: string, fragment: JsonRecord): { alreadyExists: boolean; text: string } {
  const current = parseMcpJsonConfig(content)
  const currentServers = mcpServersFromConfig(current)
  const fragmentServers = mcpServersFromConfig(fragment)
  const fragmentServerIds = Object.keys(fragmentServers)
  if (fragmentServerIds.length === 0) {
    throw new Error('MCP JSON config must include at least one server.')
  }
  const alreadyExists = fragmentServerIds.some((id) =>
    Object.prototype.hasOwnProperty.call(currentServers, id)
  )
  if (alreadyExists) {
    return { alreadyExists: true, text: `${JSON.stringify(current, null, 2)}\n` }
  }

  const fragmentRest = { ...fragment }
  delete fragmentRest.servers
  const next = {
    ...current,
    ...fragmentRest,
    servers: {
      ...currentServers,
      ...fragmentServers
    }
  }
  return { alreadyExists: false, text: `${JSON.stringify(next, null, 2)}\n` }
}

export function setMcpServerEnabled(content: string, id: string, enabled: boolean): string {
  const current = parseMcpJsonConfig(content)
  const updateServer = (servers: JsonRecord): JsonRecord => {
    const rawServer = servers[id]
    if (!isJsonRecord(rawServer)) {
      throw new Error(`MCP server "${id}" does not exist.`)
    }
    return {
      ...servers,
      [id]: {
        ...rawServer,
        enabled,
        ...(enabled ? { disabled: undefined } : {})
      }
    }
  }

  if (isJsonRecord(current.servers)) {
    return `${JSON.stringify({ ...current, servers: updateServer(current.servers) }, null, 2)}\n`
  }

  const capabilities = isJsonRecord(current.capabilities) ? current.capabilities : undefined
  const mcp = isJsonRecord(capabilities?.mcp) ? capabilities.mcp : undefined
  if (isJsonRecord(mcp?.servers)) {
    return `${JSON.stringify({
      ...current,
      capabilities: {
        ...capabilities,
        mcp: {
          ...mcp,
          servers: updateServer(mcp.servers)
        }
      }
    }, null, 2)}\n`
  }

  throw new Error(`MCP server "${id}" does not exist.`)
}

export function removeMcpServerFromConfig(content: string, id: string): string {
  const current = parseMcpJsonConfig(content)
  const removeServer = (servers: JsonRecord): JsonRecord => {
    if (!Object.prototype.hasOwnProperty.call(servers, id)) {
      return servers
    }
    const next = { ...servers }
    delete next[id]
    return next
  }

  if (isJsonRecord(current.servers)) {
    return `${JSON.stringify({ ...current, servers: removeServer(current.servers) }, null, 2)}\n`
  }

  const capabilities = isJsonRecord(current.capabilities) ? current.capabilities : undefined
  const mcp = isJsonRecord(capabilities?.mcp) ? capabilities.mcp : undefined
  if (isJsonRecord(mcp?.servers)) {
    return `${JSON.stringify({
      ...current,
      capabilities: {
        ...capabilities,
        mcp: {
          ...mcp,
          servers: removeServer(mcp.servers)
        }
      }
    }, null, 2)}\n`
  }

  return content
}

function buildSkillContent(id: string, title: string, description: string, instructions: string): string {
  return [
    '---',
    `name: ${id}`,
    `description: ${description}`,
    '---',
    '',
    `# ${title}`,
    '',
    instructions
  ].join('\n')
}

function itemTitle(item: MarketplaceItem, t: (key: string) => string): string {
  return item.title ?? (item.titleKey ? t(item.titleKey) : item.id)
}

function itemDescription(item: MarketplaceItem, t: (key: string) => string): string {
  return item.description ?? (item.descriptionKey ? t(item.descriptionKey) : '')
}

export function skillMarketplaceItemsFromDiscoveredSkills(
  skills: SkillListItem[],
  labels: { project: string; global: string }
): MarketplaceItem[] {
  return skills.map((skill) => ({
    id: skill.id,
    kind: 'skill' as const,
    title: skill.name,
    description: skill.description ?? skill.root,
    group: 'personal' as const,
    sourceLabel: skill.scope === 'project' ? labels.project : labels.global,
    detail: skill.entryPath,
    skillRootPath: skill.root,
    skillEntryPath: skill.entryPath
  }))
}

/** Last two path segments, e.g. `/Users/me/.claude/skills` → `.claude/skills`. */
export function skillRootShortLabel(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts.slice(-2).join('/') || path
}

/**
 * Builds the skill-root picker options from the backend's detected roots
 * (`skill:list-roots`) — the same source the settings page renders — so the
 * marketplace stays in sync instead of hardcoding a fixed subset of dirs.
 * Common dirs use their i18n label; user-added extra dirs fall back to a short
 * path label. (#321)
 */
export function skillRootOptionsFromRoots(
  roots: SkillRootListItem[],
  t: (key: string) => string
): SkillRootOption[] {
  return roots.map((root) => ({
    id: root.id,
    label: root.labelKey ? t(root.labelKey) : skillRootShortLabel(root.path),
    path: root.path,
    scope: root.scope,
    enabled: root.enabled,
    exists: root.exists,
    skillCount: root.skillCount
  }))
}

export function mcpMarketplaceItemsFromConfigAndDiagnostics(
  configText: string,
  diagnostics: CoreRuntimeToolDiagnosticsJson | null,
  labels: {
    configured: string
    connected: string
    error: string
    disabled: string
  }
): MarketplaceItem[] {
  const servers = new Map<string, {
    id: string
    config?: JsonRecord
    diagnostic?: JsonRecord
  }>()
  try {
    const configServers = mcpServersFromConfig(parseMcpJsonConfig(configText))
    for (const [id, value] of Object.entries(configServers)) {
      if (!id.trim()) continue
      servers.set(id, {
        id,
        config: isJsonRecord(value) ? value : {}
      })
    }
  } catch {
    /* Invalid config is surfaced elsewhere; keep the marketplace render resilient. */
  }
  for (const diagnostic of diagnostics?.mcpServers ?? []) {
    const id = typeof diagnostic.id === 'string' ? diagnostic.id.trim() : ''
    if (!id) continue
    const existing = servers.get(id)
    servers.set(id, {
      id,
      config: existing?.config,
      diagnostic: { ...diagnostic }
    })
  }
  return [...servers.values()].map(({ id, config, diagnostic }) => {
    const status = mcpServerStatus(diagnostic, config)
    const details = { ...(config ?? {}), ...(diagnostic ?? {}) }
    const sourceLabel =
      status === 'connected' || status === 'available' ? labels.connected :
      status === 'error' || status === 'unavailable' ? labels.error :
      status === 'disabled' ? labels.disabled :
      labels.configured
    const detail = mcpServerDescription(details, labels.configured)
    const catalogItem = RECOMMENDED_ITEMS.find((entry) => entry.kind === 'mcp' && entry.id === id)
    return {
      id,
      kind: 'mcp' as const,
      title: id,
      // Keep the catalog description for known servers so installing an item
      // does not replace its human-readable intro with the raw status string (#211).
      ...(catalogItem?.descriptionKey
        ? { descriptionKey: catalogItem.descriptionKey }
        : catalogItem?.description
          ? { description: catalogItem.description }
          : { description: detail }),
      detail,
      group: 'personal' as const,
      sourceLabel,
      statusTone: mcpStatusTone(status),
      mcpConfigured: Boolean(config),
      mcpRuntimeSeen: Boolean(diagnostic)
    }
  }).sort((left, right) => left.title.localeCompare(right.title))
}

function skillNameLooksValid(raw: string): boolean {
  const value = raw.trim()
  return !!value && value !== '.' && value !== '..' && !/[\\/]/.test(value)
}

type HubPluginGroup = HubAgentPluginListItem[]
type HubSkillGroup = HubAgentSkillListItem[]

type MarketplaceDetail =
  | { kind: 'hub-plugin'; group: HubPluginGroup }
  | { kind: 'hub-skill'; group: HubSkillGroup }
  | { kind: 'local-item'; item: MarketplaceItem }

type DetailField = {
  label: string
  value: string
  mono?: boolean
}

type DetailListEntry = {
  title: string
  description?: string
  badge?: string
  detail?: MarketplaceDetail
}

type PluginCatalogEntry =
  | { kind: 'hub'; key: string; group: HubPluginGroup }
  | { kind: 'local'; key: string; item: MarketplaceItem }

type PluginCatalogSection = {
  id: string
  title: string
  entries: PluginCatalogEntry[]
}

function hubPluginGroupKey(plugin: HubAgentPluginListItem): string {
  return `${plugin.upstreamMarketplaceName}:${plugin.pluginName}`
}

function hubPluginBusyKey(plugin: HubAgentPluginListItem): string {
  return `hub:${hubPluginGroupKey(plugin)}`
}

function hubSkillGroupKey(skill: HubAgentSkillListItem): string {
  return `${skill.sourceKind}:${skill.upstreamMarketplaceName}:${skill.pluginName}:${skill.skillName}`
}

function hubSkillOwningPluginKey(skill: HubAgentSkillListItem): string {
  return `${skill.upstreamMarketplaceName}:${skill.pluginName}`
}

function hubPluginDisplayName(plugin: HubAgentPluginListItem): string {
  return plugin.displayName || plugin.pluginName
}

function hubPluginGroupInstalled(group: HubPluginGroup): boolean {
  return group.some((item) => item.installed || item.requiredInstall)
}

function hubPluginMutationRequest(
  plugin: HubAgentPluginListItem,
  options?: { includeVersion?: boolean }
): HubAgentPluginMutationRequest {
  return {
    pluginName: plugin.pluginName,
    ...(options?.includeVersion === false ? {} : { version: plugin.version }),
    upstreamMarketplaceName: plugin.upstreamMarketplaceName
  }
}

function hubPluginUninstallRequest(group: HubPluginGroup): HubAgentPluginMutationRequest | null {
  const target = group.find((item) => item.installed) ?? group.find((item) => item.requiredInstall) ?? group[0]
  return target ? hubPluginMutationRequest(target, { includeVersion: false }) : null
}

function hubSkillDisplayName(skill: HubAgentSkillListItem): string {
  return skill.displayName || skill.skillName
}

function hubSkillToggleId(skill: HubAgentSkillListItem): string {
  return normalizeSkillId(skill.skillName || skill.id)
}

function skillPathDirectory(path: string): string {
  return path.replace(/[\\/][^\\/]*$/u, '')
}

function hubSkillMarkdownRequest(skill: HubAgentSkillListItem): HubAgentSkillMarkdownRequest {
  return {
    id: skill.id,
    skillName: skill.skillName,
    displayName: skill.displayName,
    pluginName: skill.pluginName,
    version: skill.version,
    upstreamMarketplaceName: skill.upstreamMarketplaceName,
    skillPath: skill.skillPath,
    sourceKind: skill.sourceKind
  }
}

function stripSkillMarkdownFrontmatter(content: string): string {
  const normalized = content.replace(/\r\n/g, '\n')
  if (!normalized.startsWith('---\n')) return normalized
  const end = normalized.indexOf('\n---', 4)
  if (end === -1) return normalized
  const next = normalized.slice(end + 4)
  return next.startsWith('\n') ? next.slice(1) : next
}

function skillHeadingPathTitle(path?: string): string {
  if (!path) return ''
  const parts = path.replace(/\\/g, '/').replace(/\/+$/u, '').split('/').filter(Boolean)
  const last = parts[parts.length - 1] ?? ''
  if (last.toLowerCase() === 'skill.md' && parts.length > 1) {
    return parts[parts.length - 2] ?? ''
  }
  return last.replace(/\.[^/.]+$/u, '')
}

function normalizedSkillHeading(value: string): string {
  return value
    .replace(/^#+\s*/u, '')
    .replace(/\s*#+$/u, '')
    .replace(/[\\`*_~]/g, '')
    .replace(/^\$/u, '')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()
}

function stripRepeatedSkillHeading(
  content: string,
  options: { path?: string; expectedTitle?: string }
): string {
  const aliases = [
    options.expectedTitle ?? '',
    skillHeadingPathTitle(options.path)
  ]
    .map(normalizedSkillHeading)
    .filter(Boolean)
  if (aliases.length === 0) return content
  const lines = content.split('\n')
  let index = 0
  while (index < lines.length && lines[index]?.trim() === '') index += 1
  if (index >= lines.length) return content
  const first = lines[index]?.trim() ?? ''
  if (!/^#\s+/u.test(first)) return content
  const firstTitle = normalizedSkillHeading(first)
  if (!aliases.includes(firstTitle)) return content
  index += 1
  while (index < lines.length && lines[index]?.trim() === '') index += 1
  return lines.slice(index).join('\n')
}

function skillMarkdownBody(content: string, options: { path?: string; expectedTitle?: string }): string {
  return stripRepeatedSkillHeading(stripSkillMarkdownFrontmatter(content), options).trim()
}

function skillContentErrorMessage(error: unknown, t: (key: string) => string): string {
  const message = error instanceof Error ? error.message : String(error)
  if (
    message.includes('No handler registered') ||
    message.includes('hub-agent-marketplace:read-skill-markdown')
  ) {
    return t('pluginSkillContentRestartRequired')
  }
  return message
}

function hubSkillTryPrompt(skill: HubAgentSkillListItem, t: (key: string, values?: Record<string, unknown>) => string): string {
  return t('pluginSkillTryPrompt', {
    name: hubSkillDisplayName(skill),
    skill: skill.skillName
  })
}

function hubPluginInitials(name: string): string {
  return name
    .split(/[\s_-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part.slice(0, 1).toUpperCase())
    .join('') || 'P'
}

function hubPluginBrandColor(plugin: HubAgentPluginListItem): string {
  return /^#[0-9a-f]{6}$/iu.test(plugin.brandColor ?? '') ? plugin.brandColor ?? '#111827' : '#111827'
}

function hubPluginMarketplaceLabel(plugin: HubAgentPluginListItem): string {
  const name = (plugin.upstreamMarketplaceName || plugin.marketplaceName || '').trim().toLowerCase()
  if (['openai-bundled', 'openai-curated', 'openai-primary-runtime', 'codex official'].includes(name)) {
    return 'Built by OpenAI'
  }
  return plugin.upstreamMarketplaceName || plugin.marketplaceName || 'Marketplace'
}

const HUB_MANAGED_MCP_SERVER_FALLBACKS: Record<string, string[]> = {
  'playwright-mcp': ['playwright'],
  github: ['github'],
  context7: ['context7'],
  'sequential-thinking': ['sequential-thinking'],
  memory: ['memory'],
  'brave-search': ['brave-search']
}

type HubManagedMcpCatalogItem = Pick<HubAgentPluginListItem, 'pluginName'> &
  Partial<Pick<HubAgentPluginListItem, 'mcpServerIds'>>

function normalizedMcpServerId(id: string): string {
  return id.trim().toLowerCase()
}

export function hubManagedMcpServerIds(plugins: HubManagedMcpCatalogItem[]): Set<string> {
  const ids = new Set<string>()
  for (const fallbackIds of Object.values(HUB_MANAGED_MCP_SERVER_FALLBACKS)) {
    for (const id of fallbackIds) ids.add(normalizedMcpServerId(id))
  }
  for (const plugin of plugins) {
    const pluginName = normalizedMcpServerId(plugin.pluginName)
    if (pluginName) ids.add(pluginName)
    for (const id of plugin.mcpServerIds ?? []) {
      const normalized = normalizedMcpServerId(id)
      if (normalized) ids.add(normalized)
    }
    for (const id of HUB_MANAGED_MCP_SERVER_FALLBACKS[pluginName] ?? []) {
      ids.add(normalizedMcpServerId(id))
    }
  }
  return ids
}

export function excludeHubManagedLocalMcpItems(
  items: MarketplaceItem[],
  hubPlugins: HubManagedMcpCatalogItem[]
): MarketplaceItem[] {
  const hubManagedIds = hubManagedMcpServerIds(hubPlugins)
  return items.filter((item) =>
    item.kind !== 'mcp' ||
    item.systemManaged ||
    !hubManagedIds.has(normalizedMcpServerId(item.id))
  )
}

function hubPlatformLabel(platform: string): string {
  if (platform === 'mac-arm64') return 'macOS Apple Silicon'
  if (platform === 'mac-x64') return 'macOS Intel'
  if (platform === 'win') return 'Windows'
  return platform === 'all' ? 'All' : platform
}

function hubPluginPlatformSummary(items: HubPluginGroup): string {
  return [...new Set(items.map((item) => hubPlatformLabel(item.platform)).filter(Boolean))].join(' / ')
}

function hubSkillPlatformSummary(items: HubSkillGroup): string {
  return [...new Set(items.map((item) => hubPlatformLabel(item.platform)).filter(Boolean))].join(' / ')
}

function hubSkillScopeLabel(skill: HubAgentSkillListItem, t?: (key: string) => string): string {
  if (skill.scope === 'user' || skill.sourceKind === 'runtime-user') return t ? t('pluginDetailScopeUser') : '个人'
  if (skill.scope === 'plugin' || skill.sourceKind === 'plugin') return skill.pluginName || (t ? t('pluginDetailScopePlugin') : '插件')
  return t ? t('pluginDetailScopeSystem') : '系统'
}

function detailText(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return ''
}

function detailRecord(value: unknown): JsonRecord {
  return isJsonRecord(value) ? value : {}
}

function detailChild(record: JsonRecord, key: string): JsonRecord {
  return detailRecord(record[key])
}

function detailFirstText(...values: unknown[]): string {
  for (const value of values) {
    const text = detailText(value)
    if (text) return text
  }
  return ''
}

function detailList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value
      .map((item) => {
        if (typeof item === 'string') return item.trim()
        const record = detailRecord(item)
        return detailFirstText(record.displayName, record.name, record.title, record.description)
      })
      .filter(Boolean)
  }
  if (isJsonRecord(value)) {
    return Object.entries(value)
      .map(([key, item]) => {
        if (typeof item === 'string') return item.trim() || key
        const record = detailRecord(item)
        return detailFirstText(record.displayName, record.name, record.title, record.description, key)
      })
      .filter(Boolean)
  }
  const text = detailText(value)
  return text ? [text] : []
}

function hubPluginInterface(plugin: HubAgentPluginListItem): JsonRecord {
  return {
    ...detailChild(plugin.manifest, 'interface'),
    ...detailChild(plugin.marketplaceEntry, 'interface')
  }
}

function hubPluginLongDescription(plugin: HubAgentPluginListItem, fallback: string): string {
  const pluginInterface = hubPluginInterface(plugin)
  return detailFirstText(
    pluginInterface.longDescription,
    plugin.shortDescription,
    plugin.manifest.description,
    fallback
  )
}

function hubPluginLinks(plugin: HubAgentPluginListItem): DetailField[] {
  const pluginInterface = hubPluginInterface(plugin)
  return [
    { label: 'website', value: detailFirstText(pluginInterface.websiteURL, pluginInterface.websiteUrl, plugin.manifest.homepage) },
    { label: 'privacy', value: detailFirstText(pluginInterface.privacyPolicyURL, pluginInterface.privacyPolicyUrl) },
    { label: 'terms', value: detailFirstText(pluginInterface.termsOfServiceURL, pluginInterface.termsOfServiceUrl) }
  ].filter((field) => Boolean(field.value))
}

function hubPluginCapabilities(plugin: HubAgentPluginListItem): string[] {
  const pluginInterface = hubPluginInterface(plugin)
  return detailList(pluginInterface.capabilities)
}

function hubPluginDefaultPrompts(plugin: HubAgentPluginListItem): string[] {
  const pluginInterface = hubPluginInterface(plugin)
  return detailList(pluginInterface.defaultPrompt)
}

function hubPluginIncludes(
  plugin: HubAgentPluginListItem,
  relatedSkills: HubSkillGroup[],
  t?: (key: string) => string
): DetailListEntry[] {
  const entries: DetailListEntry[] = []
  const manifest = plugin.manifest
  if (manifest.mcpServers !== undefined || manifest.mcp !== undefined) {
    entries.push({
      title: 'MCP',
      description: detailFirstText(manifest.mcpServers, manifest.mcp, '.mcp.json')
    })
  }
  if (manifest.apps !== undefined) {
    entries.push({
      title: 'Apps',
      description: detailFirstText(manifest.apps)
    })
  }
  if (manifest.hooks !== undefined) {
    entries.push({
      title: 'Hooks',
      description: detailFirstText(manifest.hooks)
    })
  }
  for (const group of relatedSkills) {
    const primary = group.find((item) => item.requiredInstall) ?? group[0]
    if (!primary) continue
    entries.push({
      title: hubSkillDisplayName(primary),
      description: primary.shortDescription || primary.skillPath,
      badge: hubSkillScopeLabel(primary, t),
      detail: { kind: 'hub-skill', group }
    })
  }
  return entries
}

function localItemKindLabel(kind: PluginKind, t: (key: string) => string): string {
  return kind === 'skill' ? t('pluginTabSkill') : 'MCP'
}

type LocalMcpDetailMetadata = {
  developer: string
  category: string
  version?: string
  capabilities: string[]
}

const LOCAL_MCP_DETAIL_METADATA: Record<string, LocalMcpDetailMetadata> = {
  [GUI_SCHEDULE_MCP_SERVER_ID]: {
    developer: 'Analytix',
    category: 'Automation',
    capabilities: ['Scheduled tasks', 'Task management']
  },
  playwright: {
    developer: 'Microsoft / Playwright MCP',
    category: 'Engineering',
    version: 'latest',
    capabilities: ['Browser automation', 'Page inspection', 'Testing']
  },
  github: {
    developer: 'Model Context Protocol',
    category: 'Engineering',
    version: 'latest',
    capabilities: ['Repositories', 'Issues', 'Pull requests', 'CI context']
  },
  context7: {
    developer: 'Upstash',
    category: 'Engineering',
    version: 'latest',
    capabilities: ['Library documentation', 'Code context']
  },
  'sequential-thinking': {
    developer: 'Model Context Protocol',
    category: 'Reasoning',
    version: 'latest',
    capabilities: ['Step planning', 'Reasoning trace']
  },
  memory: {
    developer: 'Model Context Protocol',
    category: 'Productivity',
    version: 'latest',
    capabilities: ['Persistent memory', 'Project preferences']
  },
  'brave-search': {
    developer: 'Model Context Protocol',
    category: 'Search',
    version: 'latest',
    capabilities: ['Web search', 'Current information']
  }
}

function localMcpDetailMetadata(item: MarketplaceItem): LocalMcpDetailMetadata {
  return LOCAL_MCP_DETAIL_METADATA[item.id] ?? {
    developer: item.group === 'personal' ? 'Custom MCP' : 'Model Context Protocol',
    category: item.group === 'personal' ? 'Personal' : 'Engineering',
    capabilities: []
  }
}

function localMcpSourceLabel(item: MarketplaceItem, t: (key: string) => string): string {
  if (item.systemManaged) return t('pluginLocalMcpBuiltIn')
  if (item.group === 'personal') return item.sourceLabel || t('pluginLocalMcpPersonal')
  return t('pluginLocalMcpRecommended')
}

function localMcpServerFromItem(item: MarketplaceItem): JsonRecord | undefined {
  if (item.kind !== 'mcp' || !item.mcpConfig) return undefined
  const server = mcpServersFromConfig(item.mcpConfig(''))[item.id]
  return isJsonRecord(server) ? server : undefined
}

export function localMcpCommandSummary(
  item: Pick<MarketplaceItem, 'kind' | 'id' | 'mcpConfig'>,
  configuredServer?: JsonRecord
): string {
  const server = configuredServer ?? (item.kind === 'mcp' && item.mcpConfig
    ? mcpServersFromConfig(item.mcpConfig(''))[item.id]
    : undefined)
  if (!isJsonRecord(server)) return ''
  const command = detailText(server.command)
  const args = Array.isArray(server.args)
    ? server.args.map((arg) => detailText(arg)).filter(Boolean)
    : []
  const url = detailText(server.url)
  if (command) return redactSecretText([command, ...args].join(' '))
  return redactSecretText(url)
}

function localMcpIncludes(item: MarketplaceItem, configuredServer?: JsonRecord): DetailListEntry[] {
  if (item.kind !== 'mcp') return []
  const summary = localMcpCommandSummary(item, configuredServer)
  const sourceServer = configuredServer ?? localMcpServerFromItem(item)
  const transport = detailText(sourceServer?.transport)
  const entries: DetailListEntry[] = []
  entries.push({
    title: 'MCP',
    description: summary || item.id,
    badge: transport || item.id
  })
  return entries
}

function groupHubPlugins(plugins: HubAgentPluginListItem[]): HubPluginGroup[] {
  const groups = new Map<string, HubPluginGroup>()
  for (const plugin of plugins) {
    const key = hubPluginGroupKey(plugin)
    groups.set(key, [...(groups.get(key) ?? []), plugin])
  }
  return [...groups.values()].map((items) =>
    [...items].sort((left, right) => left.platform.localeCompare(right.platform))
  )
}

function groupHubSkills(skills: HubAgentSkillListItem[]): HubSkillGroup[] {
  const groups = new Map<string, HubSkillGroup>()
  for (const skill of skills) {
    const key = hubSkillGroupKey(skill)
    groups.set(key, [...(groups.get(key) ?? []), skill])
  }
  return [...groups.values()].map((items) =>
    [...items].sort((left, right) => left.platform.localeCompare(right.platform))
  )
}

function hubItemMatchesQuery(values: Array<string | undefined>, query: string): boolean {
  if (!query) return true
  return values.filter(Boolean).join(' ').toLowerCase().includes(query)
}

function hubSyncResultHasItems(result: HubAgentMarketplaceSyncResult): boolean {
  const plugins = result.ok ? result.plugins : result.plugins ?? []
  const skills = result.ok ? result.skills : result.skills ?? []
  return plugins.length > 0 || skills.length > 0
}

function hubSyncResultNeedsFullSync(result: HubAgentMarketplaceSyncResult, forceRefresh: boolean): boolean {
  if (!result.ok) return false
  if (forceRefresh || result.changed) return true
  return result.plugins.some((plugin) => plugin.requiredInstall && !plugin.installed)
}

function isLegacyHubSyncPayloadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  return message.includes('hub-agent-marketplace:sync') &&
    message.includes('Unrecognized key') &&
    message.includes('"mode"')
}

function hubPluginSectionId(title: string): string {
  return `hub-plugins-${title.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'section'}`
}

function hubSkillSectionId(title: string): string {
  return `hub-skills-${title.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'section'}`
}

const RECOMMENDED_ITEMS: MarketplaceItem[] = [
  {
    id: GUI_SCHEDULE_MCP_SERVER_ID,
    kind: 'mcp',
    titleKey: 'pluginMcpGuiScheduleTitle',
    descriptionKey: 'pluginMcpGuiScheduleDesc',
    iconSrc: analytixAppIcon512,
    iconBleed: true,
    group: 'recommended',
    systemManaged: true
  },
  {
    id: 'code-review',
    kind: 'skill',
    titleKey: 'pluginSkillReviewTitle',
    descriptionKey: 'pluginSkillReviewDesc',
    group: 'recommended',
    skillInstructions:
      'Use this skill when reviewing a code change. Prioritize correctness, regressions, security, performance, and missing tests. Lead with concrete findings and file references.'
  },
  {
    id: 'frontend-polish',
    kind: 'skill',
    titleKey: 'pluginSkillFrontendTitle',
    descriptionKey: 'pluginSkillFrontendDesc',
    group: 'recommended',
    skillInstructions:
      'Use this skill when improving UI. Preserve the product style, check responsive states, avoid generic layouts, and verify the result visually before handing it back.'
  },
  {
    id: 'bug-hunt',
    kind: 'skill',
    titleKey: 'pluginSkillBugTitle',
    descriptionKey: 'pluginSkillBugDesc',
    group: 'recommended',
    skillInstructions:
      'Use this skill when investigating bugs. Reproduce or narrow the symptom, trace the data flow, identify the smallest fix, and add focused verification where possible.'
  },
  {
    id: 'release-notes',
    kind: 'skill',
    titleKey: 'pluginSkillReleaseTitle',
    descriptionKey: 'pluginSkillReleaseDesc',
    group: 'recommended',
    skillInstructions:
      'Use this skill when preparing release notes. Group user-facing changes by outcome, call out migrations or risks, and keep wording concise and scannable.'
  }
]

export function recommendedMarketplaceItemIds(): string[] {
  return RECOMMENDED_ITEMS.map((item) => item.id)
}

export function PluginMarketplaceView({
  leftSidebarCollapsed = false
}: {
  leftSidebarCollapsed?: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const workspaceRoot = normalizeWorkspaceRoot(useChatStore((s) => s.workspaceRoot))
  const showTopNotice = useChatStore((s) => s.showTopNotice)
  const [activeKind, setActiveKind] = usePersistentStringState<CatalogTab>(
    MARKETPLACE_TAB_STORAGE_KEY,
    'plugin',
    CATALOG_TABS
  )
  const [query, setQuery] = useState('')
  const [filter, setFilter] = usePersistentStringState<PluginFilter>(
    MARKETPLACE_FILTER_STORAGE_KEY,
    'all',
    PLUGIN_FILTERS
  )
  const [installed, setInstalled] = useState<string[]>(() => loadInstalledPlugins())
  const [busyId, setBusyId] = useState<string | null>(null)
  const [customOpen, setCustomOpen] = useState(false)
  const [customName, setCustomName] = useState('')
  const [customDescription, setCustomDescription] = useState('')
  const [customCommand, setCustomCommand] = useState('')
  const [customArgs, setCustomArgs] = useState('')
  const [customConfig, setCustomConfig] = useState('')
  const [customSkillBody, setCustomSkillBody] = useState('')
  const [skillRootId, setSkillRootId] = useState<SkillRootId>(() => loadPreferredSkillRootId())
  const [mcpConfigText, setMcpConfigText] = useState('')
  const [mcpLoaded, setMcpLoaded] = useState(false)
  const [toolDiagnostics, setToolDiagnostics] = useState<CoreRuntimeToolDiagnosticsJson | null>(null)
  const [discoveredSkills, setDiscoveredSkills] = useState<SkillListItem[]>([])
  const [skillListLoading, setSkillListLoading] = useState(false)
  const [skillListError, setSkillListError] = useState('')
  const [skillRoots, setSkillRoots] = useState<SkillRootListItem[]>([])
  const [disabledSkillIds, setDisabledSkillIds] = useState<string[]>([])
  const [skillToggleBusyId, setSkillToggleBusyId] = useState<string | null>(null)
  const [hubCatalog, setHubCatalog] = useState<HubAgentMarketplaceSyncResult | null>(null)
  const [hubCatalogLoading, setHubCatalogLoading] = useState(false)
  const [hubCategoryFilter, setHubCategoryFilter] = usePersistentStringState<string>(
    MARKETPLACE_HUB_CATEGORY_STORAGE_KEY,
    'all'
  )
  const [hubSourceFilter, setHubSourceFilter] = usePersistentStringState<string>(
    MARKETPLACE_HUB_SOURCE_STORAGE_KEY,
    'all'
  )
  const [selectedDetail, setSelectedDetail] = useState<MarketplaceDetail | null>(null)
  const [detailModal, setDetailModal] = useState<MarketplaceDetail | null>(null)
  const hubCatalogRequestIdRef = useRef(0)
  const hubFullSyncPromiseRef = useRef<Promise<void> | null>(null)
  const hubSyncModeSupportedRef = useRef(true)

  const activeMarketplaceKind: PluginKind = activeKind === 'skill' ? 'skill' : 'mcp'

  const showMarketplaceNotice = useCallback((nextNotice: Notice | null): void => {
    if (!nextNotice) return
    showTopNotice({ tone: nextNotice.tone, message: nextNotice.message })
  }, [showTopNotice])

  const showHubCatalogError = useCallback((message: string): void => {
    const trimmed = message.trim()
    if (!trimmed) return
    showTopNotice({ tone: 'info', message: trimmed })
  }, [showTopNotice])

  const showInstallSuccessNotice = (): void => {
    showMarketplaceNotice({ tone: 'success', message: t('pluginHubInstalled') })
  }

  const showUninstallSuccessNotice = (): void => {
    showMarketplaceNotice({ tone: 'success', message: t('pluginUninstalled') })
  }

  const skillRootOptions = useMemo<SkillRootOption[]>(
    () => skillRootOptionsFromRoots(skillRoots, t),
    [skillRoots, t]
  )

  const selectedSkillRoot =
    skillRootOptions.find((option) => option.id === skillRootId) ??
    skillRootOptions.find((option) => option.enabled) ??
    skillRootOptions[0]

  useEffect(() => {
    if (skillRootOptions.length === 0) return
    if (skillRootOptions.some((option) => option.id === skillRootId)) {
      savePreferredSkillRootId(skillRootId)
      return
    }
    const fallback = skillRootOptions.find((option) => option.enabled) ?? skillRootOptions[0]
    if (fallback && fallback.id !== skillRootId) {
      setSkillRootId(fallback.id)
    }
  }, [skillRootId, skillRootOptions])

  const readMcpConfig = useCallback(async (): Promise<string> => {
    if (typeof window.analytix?.runtime?.getAnalytixConfigFile !== 'function') return mcpConfigText
    const file = await window.analytix.runtime.getAnalytixConfigFile()
    setMcpConfigText(file.content)
    setMcpLoaded(true)
    return file.content
  }, [mcpConfigText])

  useEffect(() => {
    if (activeKind !== 'plugin' || mcpLoaded) return
    void readMcpConfig().catch((e) => {
      showMarketplaceNotice({ tone: 'error', message: e instanceof Error ? e.message : String(e) })
    })
  }, [activeKind, mcpLoaded, readMcpConfig, showMarketplaceNotice])

  const refreshMcpRuntimeOverlay = useCallback(async (): Promise<void> => {
    const provider = getProvider()
    if (!provider.getToolDiagnostics) {
      setToolDiagnostics(null)
      return
    }
    try {
      setToolDiagnostics(await provider.getToolDiagnostics())
    } catch {
      setToolDiagnostics(null)
    }
  }, [])

  useEffect(() => {
    if (activeKind !== 'plugin') return
    void refreshMcpRuntimeOverlay()
  }, [activeKind, refreshMcpRuntimeOverlay])

  const refreshSkillList = useCallback(async (forceRefresh = false): Promise<void> => {
    const listSkills = window.analytix?.app?.listSkills
    if (typeof listSkills !== 'function') {
      setDiscoveredSkills([])
      setSkillListError(t('pluginSkillScanUnavailable'))
      return
    }
    setSkillListLoading(true)
    setSkillListError('')
    try {
      const result = await desktopQueryCache.fetchQuery({
        queryKey: ['skills', 'list', workspaceRoot],
        staleTimeMs: SKILL_LIST_STALE_MS,
        forceRefresh,
        fetcher: () => listSkills(workspaceRoot || undefined)
      })
      if (!result.ok) {
        setDiscoveredSkills([])
        setSkillListError(result.message)
        return
      }
      setDiscoveredSkills(result.skills)
      if (result.validationErrors.length > 0) {
        setSkillListError(result.validationErrors[0]?.message ?? t('pluginSkillScanPartial'))
      }
    } catch (error) {
      setDiscoveredSkills([])
      setSkillListError(error instanceof Error ? error.message : String(error))
    } finally {
      setSkillListLoading(false)
    }
  }, [t, workspaceRoot])

  const refreshSkillRoots = useCallback(async (forceRefresh = false): Promise<void> => {
    const listSkillRoots = window.analytix?.app?.listSkillRoots
    if (typeof listSkillRoots !== 'function') {
      setSkillRoots([])
      return
    }
    try {
      const result = await desktopQueryCache.fetchQuery({
        queryKey: ['skills', 'roots', workspaceRoot],
        staleTimeMs: SKILL_ROOTS_STALE_MS,
        forceRefresh,
        fetcher: () => listSkillRoots(workspaceRoot || undefined)
      })
      setSkillRoots(result.ok ? result.roots : [])
    } catch {
      setSkillRoots([])
    }
  }, [workspaceRoot])

  useEffect(() => {
    if (activeKind !== 'skill') return
    void refreshSkillList()
    void refreshSkillRoots()
  }, [activeKind, refreshSkillList, refreshSkillRoots])

  const restartRuntimeAfterPluginChange = useCallback(async (): Promise<void> => {
    desktopQueryCache.invalidateQueries({ queryKey: ['skills'], broadcast: true })
    desktopQueryCache.invalidateQueries({ queryKey: ['plugins'], broadcast: true })
    await rendererRuntimeClient.restartRuntime()
    await Promise.allSettled([
      refreshMcpRuntimeOverlay(),
      refreshSkillList(true),
      refreshSkillRoots(true)
    ])
  }, [refreshMcpRuntimeOverlay, refreshSkillList, refreshSkillRoots])

  useEffect(() => {
    if (activeKind !== 'skill') return
    let cancelled = false
    void rendererRuntimeClient.getSettings({ forceRefresh: true })
      .then((settings) => {
        if (!cancelled) setDisabledSkillIds(normalizeDisabledSkillIds(settings.disabledSkillIds))
      })
      .catch(() => {
        if (!cancelled) setDisabledSkillIds([])
      })
    return () => {
      cancelled = true
    }
  }, [activeKind])

  const applyHubCatalogResult = useCallback((result: HubAgentMarketplaceSyncResult): void => {
    setHubCatalog(result)
    if (!result.ok) {
      showHubCatalogError(result.message)
    } else if (result.errors.length > 0) {
      showHubCatalogError(result.errors[0] ?? t('pluginHubPartialError'))
    } else {
      showHubCatalogError('')
    }
  }, [showHubCatalogError, t])

  const syncHubAgentMarketplace = useCallback(async (
    options: HubMarketplaceSyncOptions = {}
  ): Promise<HubAgentMarketplaceSyncResult> => {
    const sync = window.analytix?.app?.syncHubAgentMarketplace
    if (typeof sync !== 'function') {
      throw new Error(t('pluginHubUnavailable'))
    }
    if (!options.mode || hubSyncModeSupportedRef.current) {
      try {
        return await sync(options)
      } catch (error) {
        if (!options.mode || !isLegacyHubSyncPayloadError(error)) throw error
        hubSyncModeSupportedRef.current = false
      }
    }
    return sync({ forceRefresh: options.forceRefresh })
  }, [t])

  const runHubFullSync = useCallback((forceRefresh = false, pendingRequiredPluginKeys = new Set<string>()): void => {
    if (typeof window.analytix?.app?.syncHubAgentMarketplace !== 'function') return
    if (hubFullSyncPromiseRef.current) return
    const syncPromise = syncHubAgentMarketplace({ forceRefresh, mode: 'full' })
      .then(async (result) => {
        applyHubCatalogResult(result)
        if (!result.ok) return
        const installedRequiredPlugin = result.plugins.some((plugin) =>
          pendingRequiredPluginKeys.has(hubPluginGroupKey(plugin)) &&
          plugin.requiredInstall &&
          plugin.installed
        )
        const changedSkills = result.copiedSkills.length > 0 || result.removedSkills.length > 0
        if (installedRequiredPlugin || changedSkills) {
          await restartRuntimeAfterPluginChange()
        }
      })
      .catch((error) => {
        showHubCatalogError(error instanceof Error ? error.message : String(error))
      })
      .finally(() => {
        hubFullSyncPromiseRef.current = null
      })
    hubFullSyncPromiseRef.current = syncPromise
  }, [applyHubCatalogResult, restartRuntimeAfterPluginChange, showHubCatalogError, syncHubAgentMarketplace])

  const refreshHubCatalog = useCallback(async (forceRefresh = false): Promise<void> => {
    if (typeof window.analytix?.app?.syncHubAgentMarketplace !== 'function') {
      setHubCatalog(null)
      showHubCatalogError('')
      return
    }
    const requestId = hubCatalogRequestIdRef.current + 1
    hubCatalogRequestIdRef.current = requestId
    setHubCatalogLoading(true)
    showHubCatalogError('')
    try {
      if (!forceRefresh && hubSyncModeSupportedRef.current) {
        const cached = await syncHubAgentMarketplace({ mode: 'cache' })
        if (hubCatalogRequestIdRef.current === requestId && (hubSyncResultHasItems(cached) || !hubSyncModeSupportedRef.current)) {
          applyHubCatalogResult(cached)
        }
        if (!hubSyncModeSupportedRef.current) return
      }
      const result = hubSyncModeSupportedRef.current
        ? await syncHubAgentMarketplace({ forceRefresh, mode: 'catalog' })
        : await syncHubAgentMarketplace({ forceRefresh })
      if (hubCatalogRequestIdRef.current !== requestId) return
      applyHubCatalogResult(result)
      if (hubSyncModeSupportedRef.current && result.ok && hubSyncResultNeedsFullSync(result, forceRefresh)) {
        runHubFullSync(
          forceRefresh,
          new Set(result.plugins
            .filter((plugin) => plugin.requiredInstall && !plugin.installed)
            .map(hubPluginGroupKey))
        )
      }
    } catch (error) {
      if (hubCatalogRequestIdRef.current === requestId) {
        showHubCatalogError(error instanceof Error ? error.message : String(error))
      }
    } finally {
      if (hubCatalogRequestIdRef.current === requestId) {
        setHubCatalogLoading(false)
      }
    }
  }, [applyHubCatalogResult, runHubFullSync, showHubCatalogError, syncHubAgentMarketplace])

  useEffect(() => {
    void refreshHubCatalog(false)
    if (activeKind !== 'plugin') return
    const timer = window.setInterval(() => {
      void refreshHubCatalog(false)
    }, HUB_SYNC_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [activeKind, refreshHubCatalog])

  useEffect(() => {
    showMarketplaceNotice(null)
    setCustomOpen(false)
    setSelectedDetail(null)
    setDetailModal(null)
  }, [activeKind, showMarketplaceNotice])

  const markInstalled = (key: string): void => {
    setInstalled((prev) => {
      const next = [...new Set([...prev, key])]
      saveInstalledPlugins(next)
      return next
    })
  }

  const removeInstalled = (key: string): void => {
    setInstalled((prev) => {
      const next = prev.filter((item) => item !== key)
      saveInstalledPlugins(next)
      return next
    })
  }

  const hubPlugins = useMemo(
    () => hubCatalog?.ok ? hubCatalog.plugins : hubCatalog?.plugins ?? [],
    [hubCatalog]
  )
  const hubSkills = useMemo(
    () => hubCatalog?.ok ? hubCatalog.skills : hubCatalog?.skills ?? [],
    [hubCatalog]
  )
  const discoveredSkillIds = useMemo(
    () => new Set(discoveredSkills.map((skill) => skill.id)),
    [discoveredSkills]
  )
  const discoveredSkillItems = useMemo(
    () => skillMarketplaceItemsFromDiscoveredSkills(discoveredSkills, {
      project: t('pluginSkillSourceProject'),
      global: t('pluginSkillSourceGlobal')
    }),
    [discoveredSkills, t]
  )
  const discoveredMcpItems = useMemo(
    () => excludeHubManagedLocalMcpItems(
      mcpMarketplaceItemsFromConfigAndDiagnostics(mcpConfigText, toolDiagnostics, {
        configured: t('pluginMcpSourceConfigured'),
        connected: t('pluginMcpSourceConnected'),
        error: t('pluginMcpSourceError'),
        disabled: t('pluginMcpSourceDisabled')
      }).filter((item) => item.id !== GUI_SCHEDULE_MCP_SERVER_ID),
      hubPlugins
    ),
    [hubPlugins, mcpConfigText, t, toolDiagnostics]
  )
  const discoveredMcpIds = useMemo(
    () => new Set(discoveredMcpItems.filter((item) => item.mcpConfigured).map((item) => item.id)),
    [discoveredMcpItems]
  )
  const marketplaceItems = useMemo(
    () => activeMarketplaceKind === 'skill'
      ? [...RECOMMENDED_ITEMS, ...discoveredSkillItems]
      : [...RECOMMENDED_ITEMS, ...discoveredMcpItems],
    [activeMarketplaceKind, discoveredMcpItems, discoveredSkillItems]
  )

  const isInstalled = useCallback((
    item: Pick<MarketplaceItem, 'kind' | 'id'> & Partial<Pick<MarketplaceItem, 'group' | 'systemManaged'>>
  ): boolean => {
    const catalogItem = RECOMMENDED_ITEMS.find((candidate) => candidate.kind === item.kind && candidate.id === item.id)
    return marketplaceItemIsInstalled(
      { ...item, systemManaged: item.systemManaged ?? catalogItem?.systemManaged },
      {
        configuredMcpIds: discoveredMcpIds,
        discoveredSkillIds,
        installedIds: installed,
        mcpConfigText
      }
    )
  }, [discoveredMcpIds, discoveredSkillIds, installed, mcpConfigText])

  const visibleItems = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    return marketplaceItems.filter((item) => item.kind === activeMarketplaceKind)
      .filter((item) => {
        const title = itemTitle(item, t).toLowerCase()
        const description = itemDescription(item, t).toLowerCase()
        const source = item.sourceLabel?.toLowerCase() ?? ''
        return !normalizedQuery ||
          title.includes(normalizedQuery) ||
          description.includes(normalizedQuery) ||
          source.includes(normalizedQuery) ||
          item.id.includes(normalizedQuery)
      })
      .filter((item) => {
        if (filter === 'recommended') return item.group === 'recommended'
        if (filter === 'installed') return isInstalled(item)
        return true
      })
  }, [activeMarketplaceKind, filter, isInstalled, marketplaceItems, query, t])

  const builtInItems = visibleItems.filter((item) => item.systemManaged)
  const recommendedItems = visibleItems.filter((item) =>
    item.group === 'recommended' && !item.systemManaged && !isInstalled(item)
  )
  const personalItems = visibleItems.filter((item) =>
    (item.group === 'personal' && isInstalled(item)) ||
    (!item.systemManaged && isInstalled(item) && !discoveredSkillIds.has(item.id) && !discoveredMcpIds.has(item.id))
  )
  const groupedHubPlugins = useMemo(
    () => groupHubPlugins(hubPlugins),
    [hubPlugins]
  )
  const installedHubPluginKeys = useMemo(
    () => new Set(groupedHubPlugins
      .filter(hubPluginGroupInstalled)
      .map((group) => group[0])
      .filter(Boolean)
      .map((plugin) => hubPluginGroupKey(plugin))),
    [groupedHubPlugins]
  )
  const hubSkillsWithInstallState = useMemo(
    () => hubSkills.map((skill) => {
      if (skill.installed || skill.requiredInstall || skill.sourceKind !== 'plugin') return skill
      return installedHubPluginKeys.has(hubSkillOwningPluginKey(skill))
        ? { ...skill, installed: true }
        : skill
    }),
    [hubSkills, installedHubPluginKeys]
  )
  const groupedHubSkills = useMemo(
    () => groupHubSkills(hubSkillsWithInstallState),
    [hubSkillsWithInstallState]
  )
  const installedPluginCount = useMemo(() => {
    const localIds = new Set<string>()
    for (const item of [...RECOMMENDED_ITEMS, ...discoveredMcpItems]) {
      if (item.kind === 'mcp' && isInstalled(item)) localIds.add(item.id)
    }
    return localIds.size + installedHubPluginKeys.size
  }, [discoveredMcpItems, installedHubPluginKeys, isInstalled])
  const installedSkillCount = useMemo(() => {
    const localIds = new Set<string>()
    for (const item of [...RECOMMENDED_ITEMS, ...discoveredSkillItems]) {
      if (item.kind === 'skill' && isInstalled(item)) localIds.add(item.id)
    }
    const hubInstalledCount = groupedHubSkills.filter((group) =>
      group.some((item) => item.installed || item.requiredInstall)
    ).length
    return localIds.size + hubInstalledCount
  }, [discoveredSkillItems, groupedHubSkills, isInstalled])
  const hubSourceOptions = useMemo(
    () => [...new Set(groupedHubPlugins
      .map((items) => items[0])
      .filter(Boolean)
      .map((plugin) => hubPluginMarketplaceLabel(plugin)))].sort((left, right) => left.localeCompare(right)),
    [groupedHubPlugins]
  )
  const hubCategoryOptions = useMemo(
    () => [...new Set(groupedHubPlugins
      .map((items) => items[0]?.category || 'Other'))].sort((left, right) => left.localeCompare(right)),
    [groupedHubPlugins]
  )
  useEffect(() => {
    if (hubSourceFilter !== 'all' && !hubSourceOptions.includes(hubSourceFilter)) {
      setHubSourceFilter('all')
    }
  }, [hubSourceFilter, hubSourceOptions, setHubSourceFilter])
  useEffect(() => {
    if (hubCategoryFilter !== 'all' && !hubCategoryOptions.includes(hubCategoryFilter)) {
      setHubCategoryFilter('all')
    }
  }, [hubCategoryFilter, hubCategoryOptions, setHubCategoryFilter])
  const hubPluginSections = useMemo<PluginCatalogSection[]>(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const filteredGroups = groupedHubPlugins.filter((items) => {
      const primary = items[0]
      if (!primary) return false
      const sourceLabel = hubPluginMarketplaceLabel(primary)
      const matchesSource = hubSourceFilter === 'all' || sourceLabel === hubSourceFilter
      const matchesCategory = hubCategoryFilter === 'all' || (primary.category || 'Other') === hubCategoryFilter
      const matchesSearch = items.some((item) =>
        hubItemMatchesQuery([
          item.pluginName,
          item.displayName,
          item.shortDescription,
          item.category,
          item.developerName,
          item.version,
          item.upstreamMarketplaceName
        ], normalizedQuery)
      )
      return matchesSource && matchesCategory && matchesSearch
    })
    const featuredOrder = [
      'computer-use',
      'browser',
      'browser-use',
      'chrome',
      'chrome-internal',
      'playwright-mcp',
      'playwright-agent-skills',
      'spreadsheets',
      'presentations',
      'documents',
      'analytix-fund-analysis',
      'github',
      'context7',
      'sequential-thinking',
      'memory',
      'brave-search'
    ]
    const byName = new Map(filteredGroups.map((items) => [items[0]?.pluginName, items] as const))
    const used = new Set<string>()
    const featured = featuredOrder
      .map((name) => byName.get(name))
      .filter((items): items is HubPluginGroup => Boolean(items))
      .filter((items) => {
        const key = hubPluginGroupKey(items[0])
        if (used.has(key)) return false
        used.add(key)
        return true
      })
      .map((group) => ({
        kind: 'hub' as const,
        key: `hub:${hubPluginGroupKey(group[0])}`,
        group
      }))
    const localFeatured = recommendedItems
      .filter((item) => item.kind === 'mcp')
      .map((item) => ({
        kind: 'local' as const,
        key: localCatalogEntryKey(item),
        item
      }))
    const builtInEntries = builtInItems
      .filter((item) => item.kind === 'mcp')
      .map((item) => ({
        kind: 'local' as const,
        key: localCatalogEntryKey(item),
        item
      }))
    const personalEntries = personalItems
      .filter((item) => item.kind === 'mcp')
      .map((item) => ({
        kind: 'local' as const,
        key: localCatalogEntryKey(item),
        item
      }))
    const categoryGroups = new Map<string, HubPluginGroup[]>()
    for (const items of filteredGroups) {
      const primary = items[0]
      if (!primary) continue
      const key = hubPluginGroupKey(primary)
      if (used.has(key)) continue
      const category = primary.category || 'Other'
      categoryGroups.set(category, [...(categoryGroups.get(category) ?? []), items])
    }
    const sections: PluginCatalogSection[] = []
    const featuredEntries = [...featured, ...localFeatured]
    if (featuredEntries.length > 0) {
      sections.push({ id: 'hub-plugins-featured', title: t('pluginHubFeatured'), entries: featuredEntries })
    }
    if (builtInEntries.length > 0) {
      sections.push({ id: 'local-built-in-mcp', title: t('pluginLocalMcpBuiltIn'), entries: builtInEntries })
    }
    if (personalEntries.length > 0) {
      sections.push({ id: 'local-personal-mcp', title: t('pluginLocalMcpPersonal'), entries: personalEntries })
    }
    for (const [title, groups] of [...categoryGroups.entries()].sort(([left], [right]) => left.localeCompare(right))) {
      sections.push({
        id: hubPluginSectionId(title),
        title,
        entries: groups.map((group) => ({
          kind: 'hub' as const,
          key: `hub:${hubPluginGroupKey(group[0])}`,
          group
        }))
      })
    }
    return sections
  }, [builtInItems, groupedHubPlugins, hubCategoryFilter, hubSourceFilter, personalItems, query, recommendedItems, t])
  const hubSkillSections = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const filteredGroups = groupedHubSkills.filter((items) => {
      if (!normalizedQuery) return true
      return items.some((item) =>
        hubItemMatchesQuery([
          item.skillName,
          item.displayName,
          item.shortDescription,
          item.scope,
          item.sourceKind,
          item.pluginName,
          item.upstreamMarketplaceName
        ], normalizedQuery)
      )
    })
    const required = filteredGroups.filter((items) => items.some((item) => item.requiredInstall))
    const available = filteredGroups.filter((items) => !items.some((item) => item.requiredInstall))
    const sections: Array<{ id: string; title: string; groups: HubSkillGroup[] }> = []
    if (required.length > 0) sections.push({ id: 'hub-skills-required', title: t('pluginHubRequiredSkills'), groups: required })
    if (available.length > 0) sections.push({ id: hubSkillSectionId('Available'), title: t('pluginHubAvailableSkills'), groups: available })
    return sections
  }, [groupedHubSkills, query, t])
  const resolvedSelectedDetail = useMemo<MarketplaceDetail | null>(() => {
    if (!selectedDetail) return null
    if (selectedDetail.kind === 'hub-plugin') {
      const selectedPrimary = selectedDetail.group[0]
      if (!selectedPrimary) return null
      const key = hubPluginGroupKey(selectedPrimary)
      const latest = groupedHubPlugins.find((group) => group[0] && hubPluginGroupKey(group[0]) === key)
      return { kind: 'hub-plugin', group: latest ?? selectedDetail.group }
    }
    if (selectedDetail.kind === 'hub-skill') {
      const selectedPrimary = selectedDetail.group[0]
      if (!selectedPrimary) return null
      const key = hubSkillGroupKey(selectedPrimary)
      const latest = groupedHubSkills.find((group) => group[0] && hubSkillGroupKey(group[0]) === key)
      return { kind: 'hub-skill', group: latest ?? selectedDetail.group }
    }
    const matchingItems = marketplaceItems.filter((item) =>
      item.kind === selectedDetail.item.kind && item.id === selectedDetail.item.id
    )
    const latestItem =
      matchingItems.find((item) => item.group === 'personal' && isInstalled(item)) ??
      matchingItems.find((item) => item.group === 'recommended') ??
      matchingItems[0]
    if (!latestItem && selectedDetail.item.group === 'personal') return null
    return { kind: 'local-item', item: latestItem ?? selectedDetail.item }
  }, [groupedHubPlugins, groupedHubSkills, isInstalled, marketplaceItems, selectedDetail])

  const openMarketplaceDetail = useCallback((detail: MarketplaceDetail): void => {
    setSelectedDetail(detail)
    setDetailModal(null)
  }, [])

  const openNestedMarketplaceDetail = useCallback((detail: MarketplaceDetail): void => {
    if (detail.kind === 'hub-skill') {
      setDetailModal(detail)
      return
    }
    setSelectedDetail(detail)
    setDetailModal(null)
  }, [])

  const closeMarketplaceDetail = useCallback((): void => {
    setSelectedDetail(null)
    setDetailModal(null)
  }, [])

  const appendMcpConfig = async (id: string, config: JsonRecord): Promise<boolean> => {
    const content = mcpLoaded ? mcpConfigText : await readMcpConfig()
    const merged = mergeMcpJsonConfig(content, config)
    if (merged.alreadyExists) {
      markInstalled(storageKey('mcp', id))
      showMarketplaceNotice({ tone: 'info', message: t('pluginAlreadyAdded') })
      return false
    }
    await window.analytix.runtime.setAnalytixConfigFile(merged.text)
    setMcpConfigText(merged.text)
    setMcpLoaded(true)
    markInstalled(storageKey('mcp', id))
    return true
  }

  const addItem = async (item: MarketplaceItem): Promise<void> => {
    setBusyId(storageKey(item.kind, item.id))
    showMarketplaceNotice(null)
    try {
      if (item.kind === 'mcp') {
        if (!item.mcpConfig) return
        const changed = await appendMcpConfig(item.id, item.mcpConfig(workspaceRoot))
        if (changed) await restartRuntimeAfterPluginChange()
        if (changed) showInstallSuccessNotice()
        return
      }

      if (!selectedSkillRoot?.path) {
        showMarketplaceNotice({ tone: 'error', message: t('pluginSkillRootMissing') })
        return
      }
      if (item.group === 'personal') return
      const title = itemTitle(item, t)
      const description = itemDescription(item, t)
      const content = buildSkillContent(
        item.id,
        title,
        description,
        item.skillInstructions ?? description
      )
      const result = await window.analytix.app.saveSkillFile(selectedSkillRoot.path, item.id, content)
      if (!result.ok) {
        showMarketplaceNotice({ tone: 'error', message: result.message })
        return
      }
      markInstalled(storageKey('skill', item.id))
      await Promise.all([refreshSkillList(), refreshSkillRoots()])
      await restartRuntimeAfterPluginChange()
      showInstallSuccessNotice()
    } catch (e) {
      showMarketplaceNotice({ tone: 'error', message: e instanceof Error ? e.message : String(e) })
    } finally {
      setBusyId(null)
    }
  }

  const refreshHubCatalogFromCache = async (): Promise<void> => {
    try {
      const result = await syncHubAgentMarketplace({ mode: 'cache' })
      applyHubCatalogResult(result)
    } catch {
      await refreshHubCatalog(false)
    }
  }

  const installHubPluginGroup = async (group: HubPluginGroup): Promise<void> => {
    const primary = group.find((item) => item.requiredInstall) ?? group[0]
    if (!primary) return
    setBusyId(hubPluginBusyKey(primary))
    showMarketplaceNotice(null)
    try {
      const install = window.analytix?.app?.installHubAgentPlugin
      if (typeof install !== 'function') throw new Error(t('pluginHubUnavailable'))
      const result = await install(hubPluginMutationRequest(primary))
      if (!result.ok) {
        showMarketplaceNotice({ tone: 'error', message: result.message })
        return
      }
      await refreshHubCatalogFromCache()
      await restartRuntimeAfterPluginChange()
      showInstallSuccessNotice()
    } catch (error) {
      showMarketplaceNotice({ tone: 'error', message: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusyId(null)
    }
  }

  const uninstallHubPluginGroup = async (group: HubPluginGroup): Promise<void> => {
    const primary = group.find((item) => item.requiredInstall) ?? group[0]
    if (!primary) return
    setBusyId(hubPluginBusyKey(primary))
    showMarketplaceNotice(null)
    try {
      const uninstall = window.analytix?.app?.uninstallHubAgentPlugin
      if (typeof uninstall !== 'function') throw new Error(t('pluginHubUnavailable'))
      const request = hubPluginUninstallRequest(group)
      if (!request) return
      const result = await uninstall(request)
      if (!result.ok) {
        showMarketplaceNotice({ tone: result.managed ? 'info' : 'error', message: result.managed ? t('pluginHubManagedUninstall') : result.message })
        return
      }
      await refreshHubCatalogFromCache()
      await restartRuntimeAfterPluginChange()
      showUninstallSuccessNotice()
    } catch (error) {
      showMarketplaceNotice({ tone: 'error', message: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusyId(null)
    }
  }

  const uninstallLocalItem = async (item: MarketplaceItem): Promise<void> => {
    const key = storageKey(item.kind, item.id)
    setBusyId(key)
    showMarketplaceNotice(null)
    try {
      if (item.kind === 'mcp') {
        const content = mcpLoaded ? mcpConfigText : await readMcpConfig()
        const nextText = removeMcpServerFromConfig(content, item.id)
        await window.analytix.runtime.setAnalytixConfigFile(nextText)
        setMcpConfigText(nextText)
        setMcpLoaded(true)
        removeInstalled(key)
        await restartRuntimeAfterPluginChange()
        showUninstallSuccessNotice()
        return
      }

      if (!item.skillRootPath) {
        showMarketplaceNotice({ tone: 'error', message: t('pluginSkillRootMissing') })
        return
      }
      const result = await window.analytix.app.deleteSkill(item.skillRootPath, item.id)
      if (!result.ok) {
        showMarketplaceNotice({ tone: 'error', message: result.message })
        return
      }
      removeInstalled(key)
      await restartRuntimeAfterPluginChange()
      showUninstallSuccessNotice()
    } catch (error) {
      showMarketplaceNotice({ tone: 'error', message: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusyId(null)
    }
  }

  const addCustom = async (): Promise<void> => {
    const id = normalizePluginId(customName)
    if (!id) {
      showMarketplaceNotice({ tone: 'error', message: t('pluginCustomNameRequired') })
      return
    }
    const description = customDescription.trim() || t('pluginCustomFallbackDesc')
    setBusyId(`custom:${activeMarketplaceKind}`)
    showMarketplaceNotice(null)
    try {
      if (activeMarketplaceKind === 'mcp') {
        const fallback = buildMcpConfig(
          id,
          customCommand.trim() || 'npx',
          customArgs
            .split('\n')
            .map((arg) => arg.trim())
            .filter(Boolean)
        )
        const changed = await appendMcpConfig(id, customMcpConfigFragment(id, customConfig, fallback))
        if (changed) await restartRuntimeAfterPluginChange()
        if (changed) showInstallSuccessNotice()
      } else {
        if (!selectedSkillRoot?.path) {
          showMarketplaceNotice({ tone: 'error', message: t('pluginSkillRootMissing') })
          return
        }
        const body = customSkillBody.trim() || t('pluginCustomSkillFallbackBody')
        const content = buildSkillContent(id, customName.trim() || id, description, body)
        const result = await window.analytix.app.saveSkillFile(selectedSkillRoot.path, id, content)
        if (!result.ok) {
          showMarketplaceNotice({ tone: 'error', message: result.message })
          return
        }
        markInstalled(storageKey('skill', id))
        await Promise.all([refreshSkillList(), refreshSkillRoots()])
        await restartRuntimeAfterPluginChange()
        showInstallSuccessNotice()
      }
      setCustomName('')
      setCustomDescription('')
      setCustomCommand('')
      setCustomArgs('')
      setCustomConfig('')
      setCustomSkillBody('')
      setCustomOpen(false)
    } catch (e) {
      showMarketplaceNotice({ tone: 'error', message: e instanceof Error ? e.message : String(e) })
    } finally {
      setBusyId(null)
    }
  }

  const toggleSkillEnabled = async (id: string, enabled: boolean): Promise<void> => {
    const normalizedId = normalizeSkillId(id)
    if (!normalizedId) return
    setSkillToggleBusyId(normalizedId)
    showMarketplaceNotice(null)
    try {
      const next = enabled
        ? disabledSkillIds.filter((item) => item !== normalizedId)
        : [...new Set([...disabledSkillIds, normalizedId])]
      const settings = await rendererRuntimeClient.setSettings({ disabledSkillIds: next })
      const normalized = normalizeDisabledSkillIds(settings.disabledSkillIds)
      setDisabledSkillIds(normalized)
      useChatStore.setState({ disabledSkillIds: normalized })
      showMarketplaceNotice(null)
    } catch (error) {
      showMarketplaceNotice({ tone: 'error', message: error instanceof Error ? error.message : String(error) })
    } finally {
      setSkillToggleBusyId(null)
    }
  }

  const openManageTarget = async (): Promise<void> => {
    try {
      if (activeMarketplaceKind === 'mcp') {
        const result = await window.analytix.runtime.openAnalytixConfigDir()
        if (!result.ok) showMarketplaceNotice({ tone: 'error', message: result.message ?? t('pluginActionFailed') })
        return
      }
      if (!selectedSkillRoot?.path) {
        showMarketplaceNotice({ tone: 'error', message: t('pluginSkillRootMissing') })
        return
      }
      const result = await window.analytix.app.openSkillRoot(selectedSkillRoot.path)
      if (!result.ok) showMarketplaceNotice({ tone: 'error', message: result.message ?? t('pluginActionFailed') })
    } catch (e) {
      showMarketplaceNotice({ tone: 'error', message: e instanceof Error ? e.message : String(e) })
    }
  }

  return (
    <div className="ds-no-drag h-full min-h-0 overflow-y-auto bg-white px-6 py-6 text-[#0d0d0d] [scrollbar-gutter:stable] dark:bg-ds-main dark:text-ds-ink md:px-10">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          className="ds-plugin-marketplace-tabs flex items-center gap-1"
          data-left-sidebar-collapsed={leftSidebarCollapsed ? 'true' : 'false'}
        >
          <TabButton active={activeKind === 'plugin'} count={installedPluginCount} onClick={() => setActiveKind('plugin')}>
            {t('pluginTabMcp')}
          </TabButton>
          <TabButton active={activeKind === 'skill'} count={installedSkillCount} tone="skill" onClick={() => setActiveKind('skill')}>
            {t('pluginTabSkill')}
          </TabButton>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => void openManageTarget()}
            className="inline-flex h-9 items-center gap-2 rounded-lg bg-black/[0.05] px-3 text-[15px] font-normal text-[#0d0d0d] transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink dark:hover:bg-ds-hover"
          >
            <Settings className="h-4 w-4" strokeWidth={1.75} />
            {t('pluginManage')}
          </button>
          <button
            type="button"
            onClick={() => setCustomOpen((value) => !value)}
            className="inline-flex h-9 items-center gap-2 rounded-lg bg-black/[0.05] px-3 text-[15px] font-normal text-[#0d0d0d] transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink dark:hover:bg-ds-hover"
          >
            <Plus className="h-4 w-4" strokeWidth={1.9} />
            {t('pluginCreate')}
          </button>
        </div>
      </div>

      <div className="mx-auto w-full max-w-[960px]">
        {!resolvedSelectedDetail ? (
          <div className="mt-24 flex flex-col items-center text-center md:mt-28">
            <h1 className="text-[34px] font-normal leading-[1.12] tracking-normal text-[#0d0d0d] dark:text-ds-ink md:text-[40px]">
              {t('pluginCatalogTitle')}
            </h1>
          </div>
        ) : null}

        {!resolvedSelectedDetail ? (
          <div className="sticky top-0 z-10 mt-12 bg-gradient-to-b from-white via-white to-white/80 pb-5 dark:from-ds-main dark:via-ds-main dark:to-ds-main/80">
            <div className="flex flex-col gap-3 md:flex-row md:items-center md:gap-2">
              <label className="relative min-w-0 flex-1">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#6f6f6f] dark:text-ds-faint" />
                <input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  className="h-11 w-full rounded-lg border border-[#dedede] bg-white pl-9 pr-4 text-[16px] leading-[18px] text-[#0d0d0d] shadow-sm outline-none transition placeholder:text-[#8f8f8f] focus:border-[#bcbcbc] dark:border-ds-border dark:bg-ds-card dark:text-ds-ink"
                  placeholder={activeKind === 'plugin' ? t('pluginSearchPlugin') : t('pluginSearchSkill')}
                />
              </label>
              {activeKind === 'plugin' ? (
                <>
                  <label className="relative w-full md:w-[184px]">
                    <select
                      value={hubSourceFilter}
                      onChange={(event) => setHubSourceFilter(event.target.value)}
                      className="h-11 w-full appearance-none rounded-lg border border-transparent bg-black/[0.05] px-3 pr-8 text-[16px] font-normal text-[#0d0d0d] outline-none transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink"
                    >
                      <option value="all">{t('pluginHubSourceAll')}</option>
                      {hubSourceOptions.map((source) => (
                        <option key={source} value={source}>{source}</option>
                      ))}
                    </select>
                    <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8f8f8f]" />
                  </label>
                  <label className="relative w-full md:w-[112px]">
                    <select
                      value={hubCategoryFilter}
                      onChange={(event) => setHubCategoryFilter(event.target.value)}
                      className="h-11 w-full appearance-none rounded-lg border border-transparent bg-black/[0.05] px-3 pr-8 text-[16px] font-normal text-[#0d0d0d] outline-none transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink"
                    >
                      <option value="all">{t('pluginFilterAll')}</option>
                      {hubCategoryOptions.map((category) => (
                        <option key={category} value={category}>{category}</option>
                      ))}
                    </select>
                    <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8f8f8f]" />
                  </label>
                </>
              ) : (
                <label className="relative w-full md:w-[168px]">
                  <select
                    value={filter}
                    onChange={(event) => setFilter(event.target.value as PluginFilter)}
                    className="h-11 w-full appearance-none rounded-lg border border-transparent bg-black/[0.05] px-3 pr-8 text-[16px] font-normal text-[#0d0d0d] outline-none transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink"
                  >
                    <option value="all">{t('pluginFilterAll')}</option>
                    <option value="recommended">{t('pluginFilterRecommended')}</option>
                    <option value="installed">{t('pluginFilterInstalled')}</option>
                  </select>
                  <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8f8f8f]" />
                </label>
              )}
              <button
                type="button"
                onClick={() => void refreshHubCatalog(true)}
                disabled={hubCatalogLoading}
                className="inline-flex h-11 shrink-0 items-center justify-center gap-2 rounded-lg bg-black/[0.05] px-3 text-[16px] font-normal text-[#0d0d0d] transition hover:bg-black/[0.1] disabled:cursor-not-allowed disabled:opacity-60 dark:bg-ds-subtle dark:text-ds-ink dark:hover:bg-ds-hover"
              >
                {hubCatalogLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                <span>{hubCatalogLoading ? t('pluginHubChecking') : t('pluginHubCheckUpdates')}</span>
              </button>
            </div>
          </div>
        ) : null}

        {activeKind === 'plugin' && !resolvedSelectedDetail ? <BuiltinOfficePlugins query={query} /> : null}

        {activeKind === 'skill' && !resolvedSelectedDetail ? (
          <div className="mt-4 flex flex-col gap-2 md:flex-row md:items-center">
            <select
              value={selectedSkillRoot?.id ?? ''}
              onChange={(event) => setSkillRootId(event.target.value as SkillRootId)}
              disabled={skillRootOptions.length === 0}
              className="h-10 rounded-xl border border-ds-border bg-ds-card px-3 text-[13px] text-ds-ink shadow-sm outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {skillRootOptions.length === 0 ? (
                <option value="">{t('pluginSkillRootNone')}</option>
              ) : (
                skillRootOptions.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.enabled ? option.label : `${option.label} · ${t('pluginSkillStatusDisabled')}`}
                  </option>
                ))
              )}
            </select>
            <button
              type="button"
              onClick={() => void openManageTarget()}
              className="inline-flex h-10 items-center gap-2 rounded-xl border border-ds-border bg-ds-card px-3 text-[13px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover"
            >
              <FolderOpen className="h-4 w-4" />
              {t('pluginOpenLocation')}
            </button>
            <button
              type="button"
              onClick={() => void Promise.all([refreshSkillList(), refreshSkillRoots()])}
              disabled={skillListLoading}
              className="inline-flex h-10 items-center gap-2 rounded-xl border border-ds-border bg-ds-card px-3 text-[13px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover disabled:cursor-not-allowed disabled:opacity-60"
            >
              {skillListLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              {t('pluginSkillRefresh')}
            </button>
            {skillListError ? (
              <span className="text-[12px] text-red-700 dark:text-red-300">
                {skillListError}
              </span>
            ) : (
              <span className="text-[12px] text-ds-faint">
                {t('pluginSkillDiscoveredCountWithEnabled', {
                  count: discoveredSkills.length,
                  enabled: discoveredSkills.filter((skill) => !disabledSkillIds.includes(normalizeSkillId(skill.id))).length
                })}
              </span>
            )}
          </div>
        ) : null}

        {customOpen && !resolvedSelectedDetail ? (
          <CustomPluginPanel
            activeKind={activeMarketplaceKind}
            customName={customName}
            customDescription={customDescription}
            customCommand={customCommand}
            customArgs={customArgs}
            customConfig={customConfig}
            customSkillBody={customSkillBody}
            busy={busyId === `custom:${activeMarketplaceKind}`}
            onNameChange={setCustomName}
            onDescriptionChange={setCustomDescription}
            onCommandChange={setCustomCommand}
            onArgsChange={setCustomArgs}
            onConfigChange={setCustomConfig}
            onSkillBodyChange={setCustomSkillBody}
            onAdd={() => void addCustom()}
          />
        ) : null}

        {resolvedSelectedDetail ? (
          <MarketplaceDetailView
            detail={resolvedSelectedDetail}
            hubSkills={groupedHubSkills}
            hubCatalogLoading={hubCatalogLoading}
            busyId={busyId}
            isInstalled={isInstalled}
            disabledSkillIds={disabledSkillIds}
            skillToggleBusyId={skillToggleBusyId}
            mcpConfigText={mcpConfigText}
            onBack={closeMarketplaceDetail}
            onOpenDetail={openNestedMarketplaceDetail}
            onInstallHubGroup={(group) => void installHubPluginGroup(group)}
            onUninstallHubGroup={(group) => void uninstallHubPluginGroup(group)}
            onAdd={(item) => void addItem(item)}
            onUninstallItem={(item) => void uninstallLocalItem(item)}
            onToggleSkillEnabled={(id, enabled) => void toggleSkillEnabled(id, enabled)}
            t={t}
          />
        ) : (
          <>
            {activeKind === 'plugin' ? (
              <HubPluginCatalog
                sections={hubPluginSections}
                loading={hubCatalogLoading && hubPluginSections.length === 0}
                emptyText={hubPlugins.length > 0 ? t('pluginHubNoMatches') : t('pluginHubEmpty')}
                onRefresh={() => void refreshHubCatalog(true)}
                onSelectGroup={(group) => openMarketplaceDetail({ kind: 'hub-plugin', group })}
                busyId={busyId}
                isInstalled={isInstalled}
                onAddGroup={installHubPluginGroup}
                onAdd={addItem}
                onSelectItem={(item) => openMarketplaceDetail({ kind: 'local-item', item })}
                t={t}
              />
            ) : (
              <HubSkillCatalog
                sections={hubSkillSections}
                loading={hubCatalogLoading && hubSkillSections.length === 0}
                emptyText={hubSkills.length > 0 ? t('pluginHubNoMatchingSkills') : t('pluginHubSkillsEmpty')}
                onSelectGroup={(group) => openMarketplaceDetail({ kind: 'hub-skill', group })}
                t={t}
              />
            )}

            {activeKind === 'skill' ? (
              <>
                <PluginSection
                  title={t('pluginRecommended')}
                  emptyText={t('pluginNoResults')}
                  items={recommendedItems}
                  busyId={busyId}
                  isInstalled={isInstalled}
                  onAdd={addItem}
                  onSelectItem={(item) => openMarketplaceDetail({ kind: 'local-item', item })}
                  t={t}
                />

                <PluginSection
                  title={t('pluginPersonal')}
                  emptyText={t('pluginPersonalEmpty')}
                  items={personalItems}
                  busyId={busyId}
                  isInstalled={isInstalled}
                  onAdd={addItem}
                  onSelectItem={(item) => openMarketplaceDetail({ kind: 'local-item', item })}
                  t={t}
                />
              </>
            ) : null}
          </>
        )}
        {detailModal?.kind === 'hub-skill' ? (
          <HubSkillDetailModal
            group={detailModal.group}
            onClose={() => setDetailModal(null)}
            disabledSkillIds={disabledSkillIds}
            skillToggleBusyId={skillToggleBusyId}
            onToggleSkillEnabled={toggleSkillEnabled}
            t={t}
          />
        ) : null}
      </div>
    </div>
  )
}

function PluginListActionButton({
  installed,
  busy,
  onClick,
  t
}: {
  installed: boolean
  busy: boolean
  onClick: () => void
  t: (key: string) => string
}): ReactElement {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
      className={`inline-flex h-8 min-w-[86px] shrink-0 items-center justify-center rounded-xl border px-3 text-[14px] font-medium transition disabled:cursor-not-allowed disabled:opacity-60 ${
        installed
          ? 'border-[#e5e5e5] bg-white text-[#6f6f6f] hover:bg-black/[0.03] dark:border-ds-border dark:bg-ds-card dark:text-ds-muted dark:hover:bg-ds-hover'
          : 'border-[#d8d8d8] bg-white text-[#0d0d0d] hover:bg-black/[0.04] dark:border-ds-border dark:bg-ds-card dark:text-ds-ink dark:hover:bg-ds-hover'
      }`}
    >
      {busy ? <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2} /> : installed ? t('pluginInstalled') : t('pluginInstall')}
    </button>
  )
}

function PluginUninstallButton({
  busy,
  onClick,
  t
}: {
  busy: boolean
  onClick: () => void
  t: (key: string) => string
}): ReactElement {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onClick}
      className="inline-flex h-10 items-center justify-center rounded-xl bg-red-50 px-4 text-[15px] font-semibold text-red-600 transition hover:bg-red-100 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-red-950/30 dark:text-red-200 dark:hover:bg-red-950/45"
    >
      {busy ? <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2} /> : t('pluginUninstall')}
    </button>
  )
}

function HubPluginCatalog({
  sections,
  loading,
  emptyText,
  onRefresh,
  onSelectGroup,
  busyId,
  isInstalled,
  onAddGroup,
  onAdd,
  onSelectItem,
  t
}: {
  sections: PluginCatalogSection[]
  loading: boolean
  emptyText: string
  onRefresh: () => void
  onSelectGroup: (group: HubPluginGroup) => void
  busyId: string | null
  isInstalled: (item: Pick<MarketplaceItem, 'kind' | 'id'>) => boolean
  onAddGroup: (group: HubPluginGroup) => Promise<void>
  onAdd: (item: MarketplaceItem) => Promise<void>
  onSelectItem: (item: MarketplaceItem) => void
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  if (loading) {
    return (
      <div className="mt-12 flex items-center justify-center gap-2 py-10 text-[14px] text-[#6f6f6f]">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span>{t('pluginHubLoading')}</span>
      </div>
    )
  }
  if (sections.length === 0) {
    return (
      <div className="mt-12 flex flex-col items-center gap-3 py-10 text-center text-[14px] text-[#6f6f6f]">
        <Package className="h-8 w-8 opacity-60" strokeWidth={1.6} />
        <span>{emptyText}</span>
        <button
          type="button"
          onClick={onRefresh}
          className="inline-flex h-9 items-center gap-2 rounded-lg bg-black/[0.05] px-3 text-[14px] text-[#0d0d0d] transition hover:bg-black/[0.1]"
        >
          <RefreshCw className="h-4 w-4" />
          {t('pluginHubCheckUpdates')}
        </button>
      </div>
    )
  }
  return (
    <div className="mt-12 flex flex-col gap-10">
      {sections.map((section) => (
        <section key={section.id} className="flex flex-col gap-5">
          <div className="flex items-center justify-between gap-3 border-b border-[#ececec] pb-2">
            <h2 className="text-[20px] font-normal leading-6 text-[#0d0d0d] dark:text-ds-ink">{section.title}</h2>
            <span className="text-[14px] text-[#6f6f6f] dark:text-ds-muted">{section.entries.length}</span>
          </div>
          <div className="grid gap-x-10 gap-y-6 md:grid-cols-2">
            {section.entries.map((entry) => {
              if (entry.kind === 'hub') {
                const items = entry.group
                const primary = items.find((item) => item.requiredInstall) ?? items[0]
                if (!primary) return null
                const title = hubPluginDisplayName(primary)
                const description = primary.shortDescription || t('pluginHubPluginFallbackDesc')
                const installed = hubPluginGroupInstalled(items)
                const busy = busyId === hubPluginBusyKey(primary)
                return (
                  <div
                    key={entry.key}
                    role="button"
                    tabIndex={0}
                    onClick={() => onSelectGroup(items)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault()
                        onSelectGroup(items)
                      }
                    }}
                    className="group flex min-h-[76px] cursor-pointer items-center gap-3 rounded-xl border border-transparent p-2 transition-colors hover:bg-black/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:hover:bg-ds-hover dark:focus-visible:ring-white/15"
                  >
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-[#e5e5e5] bg-transparent text-[13px] font-semibold text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted">
                      {primary.iconDataUrl || primary.iconUrl ? (
                        <img src={primary.iconDataUrl || primary.iconUrl} alt={title} className="h-full w-full object-contain" />
                      ) : (
                        <span style={{ color: hubPluginBrandColor(primary) }}>{hubPluginInitials(title)}</span>
                      )}
                    </div>
                    <div className="flex min-w-0 flex-1 items-center gap-3">
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-[16px] font-semibold leading-5 text-[#0d0d0d] dark:text-ds-ink">
                          {title}
                        </div>
                        <div className="mt-1 truncate text-[14px] leading-5 text-[#6f6f6f] dark:text-ds-muted" title={description}>
                          {description}
                        </div>
                      </div>
                      <PluginListActionButton
                        installed={installed}
                        busy={busy}
                        onClick={() => {
                          if (installed) onSelectGroup(items)
                          else void onAddGroup(items)
                        }}
                        t={t}
                      />
                    </div>
                  </div>
                )
              }

              const item = entry.item
              const itemKey = storageKey(item.kind, item.id)
              const installed = isInstalled(item)
              const busy = busyId === itemKey
              const titleText = itemTitle(item, t)
              const descriptionText = itemDescription(item, t)
              return (
                <div
                  key={entry.key}
                  role="button"
                  tabIndex={0}
                  onClick={() => onSelectItem(item)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      onSelectItem(item)
                    }
                  }}
                  className="group flex min-h-[76px] cursor-pointer items-center gap-3 rounded-xl border border-transparent p-2 transition-colors hover:bg-black/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:hover:bg-ds-hover dark:focus-visible:ring-white/15"
                >
                  <LocalItemIcon title={titleText} iconSrc={item.iconSrc} iconBleed={item.iconBleed} size="small" />
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-[16px] font-semibold leading-5 text-[#0d0d0d] dark:text-ds-ink">
                          {titleText}
                        </span>
                      </div>
                      <p className="mt-1 truncate text-[14px] leading-5 text-[#6f6f6f] dark:text-ds-muted" title={descriptionText}>
                        {descriptionText}
                      </p>
                    </div>
                    <PluginListActionButton
                      installed={installed}
                      busy={busy}
                      onClick={() => {
                        if (installed) onSelectItem(item)
                        else void onAdd(item)
                      }}
                      t={t}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      ))}
    </div>
  )
}

function HubSkillCatalog({
  sections,
  loading,
  emptyText,
  onSelectGroup,
  t
}: {
  sections: Array<{ id: string; title: string; groups: HubSkillGroup[] }>
  loading: boolean
  emptyText: string
  onSelectGroup: (group: HubSkillGroup) => void
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  if (loading) {
    return (
      <div className="mt-12 flex items-center justify-center gap-2 py-10 text-[14px] text-[#6f6f6f]">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span>{t('pluginHubLoading')}</span>
      </div>
    )
  }
  if (sections.length === 0) {
    return (
      <div className="mt-12 flex items-center justify-center py-10 text-[14px] text-[#6f6f6f]">
        {emptyText}
      </div>
    )
  }
  return (
    <div className="mt-12 flex flex-col gap-10">
      {sections.map((section) => (
        <section key={section.id} className="flex flex-col gap-5">
          <div className="flex items-center justify-between gap-3 border-b border-[#ececec] pb-2">
            <h2 className="text-[20px] font-normal leading-6 text-[#0d0d0d] dark:text-ds-ink">{section.title}</h2>
            <span className="text-[14px] text-[#6f6f6f] dark:text-ds-muted">{section.groups.length}</span>
          </div>
          <div className="grid gap-x-10 gap-y-8 md:grid-cols-2">
            {section.groups.map((items) => {
              const primary = items.find((item) => item.requiredInstall) ?? items[0]
              const title = hubSkillDisplayName(primary)
              const description = primary.shortDescription || t('pluginHubSkillFallbackDesc')
              return (
                <div
                  key={hubSkillGroupKey(primary)}
                  role="button"
                  tabIndex={0}
                  onClick={() => onSelectGroup(items)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      onSelectGroup(items)
                    }
                  }}
                  className="group flex min-h-[90px] cursor-pointer items-center gap-3 rounded-2xl border border-transparent p-2.5 transition-colors hover:bg-black/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:hover:bg-ds-hover dark:focus-visible:ring-white/15"
                >
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-[#e5e5e5] bg-transparent text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted">
                    <Package className="h-4 w-4" strokeWidth={1.75} />
                  </div>
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-[16px] font-semibold leading-5 text-[#0d0d0d] dark:text-ds-ink">
                        {title}
                      </div>
                      <div className="mt-1 truncate text-[14px] leading-5 text-[#6f6f6f] dark:text-ds-muted" title={description}>
                        {description}
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[13px] text-[#8f8f8f] dark:text-ds-faint">
                        <span>{hubSkillScopeLabel(primary, t)}</span>
                        <span>/</span>
                        <span>{hubSkillPlatformSummary(items)}</span>
                      </div>
                    </div>
                    <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-black/[0.05] text-[#0d0d0d] dark:bg-ds-subtle dark:text-ds-ink">
                      {primary.installed || primary.requiredInstall ? <CheckCircle2 className="h-4 w-4 opacity-70" /> : <Plus className="h-4 w-4" />}
                    </span>
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      ))}
    </div>
  )
}

function PluginSection({
  title,
  emptyText,
  items,
  busyId,
  isInstalled,
  onAdd,
  onSelectItem,
  t
}: {
  title: string
  emptyText: string
  items: MarketplaceItem[]
  busyId: string | null
  isInstalled: (item: Pick<MarketplaceItem, 'kind' | 'id'>) => boolean
  onAdd: (item: MarketplaceItem) => Promise<void>
  onSelectItem: (item: MarketplaceItem) => void
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  return (
    <section className="mt-12 flex flex-col gap-5">
      <div className="flex items-center justify-between gap-3 border-b border-[#ececec] pb-2">
        <h2 className="text-[20px] font-normal leading-6 text-[#0d0d0d] dark:text-ds-ink">
          {title}
        </h2>
        <span className="text-[14px] text-[#6f6f6f] dark:text-ds-muted">{items.length}</span>
      </div>
      {items.length === 0 ? (
        <div className="py-8 text-[14px] text-[#6f6f6f] dark:text-ds-muted">{emptyText}</div>
      ) : (
        <div className="grid gap-x-10 gap-y-8 md:grid-cols-2">
          {items.map((item) => {
            const itemKey = storageKey(item.kind, item.id)
            const installed = isInstalled(item)
            const busy = busyId === itemKey
            const titleText = itemTitle(item, t)
            const descriptionText = itemDescription(item, t)
            return (
              <div
                key={itemKey}
                role="button"
                tabIndex={0}
                onClick={() => onSelectItem(item)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    onSelectItem(item)
                  }
                }}
                className="group flex min-h-[90px] cursor-pointer items-center gap-3 rounded-2xl border border-transparent p-2.5 transition-colors hover:bg-black/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:hover:bg-ds-hover dark:focus-visible:ring-white/15"
              >
                <LocalItemIcon title={titleText} iconSrc={item.iconSrc} iconBleed={item.iconBleed} size="small" />
                <div className="flex min-w-0 flex-1 items-center gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="truncate text-[16px] font-semibold leading-5 text-[#0d0d0d] dark:text-ds-ink">
                        {titleText}
                      </span>
                      {item.sourceLabel ? (
                        <span className="shrink-0 rounded-md border border-[#e5e5e5] bg-transparent px-1.5 py-0.5 text-[11px] font-medium text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted">
                          {item.sourceLabel}
                        </span>
                      ) : null}
                    </div>
                    <p className="mt-1 truncate text-[14px] leading-5 text-[#6f6f6f] dark:text-ds-muted" title={descriptionText}>
                      {descriptionText}
                    </p>
                    {item.detail && item.detail !== descriptionText ? (
                      <p className="mt-1 truncate font-mono text-[13px] leading-5 text-[#8f8f8f] dark:text-ds-faint" title={item.detail}>
                        {item.detail}
                      </p>
                    ) : null}
                  </div>
                  <PluginListActionButton
                    installed={installed}
                    busy={busy}
                    onClick={() => {
                      if (installed) onSelectItem(item)
                      else void onAdd(item)
                    }}
                    t={t}
                  />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

function MarketplaceDetailView({
  detail,
  hubSkills,
  hubCatalogLoading,
  busyId,
  isInstalled,
  disabledSkillIds,
  skillToggleBusyId,
  mcpConfigText,
  onBack,
  onOpenDetail,
  onInstallHubGroup,
  onUninstallHubGroup,
  onAdd,
  onUninstallItem,
  onToggleSkillEnabled,
  t
}: {
  detail: MarketplaceDetail
  hubSkills: HubSkillGroup[]
  hubCatalogLoading: boolean
  busyId: string | null
  isInstalled: (item: Pick<MarketplaceItem, 'kind' | 'id'>) => boolean
  disabledSkillIds: string[]
  skillToggleBusyId: string | null
  mcpConfigText: string
  onBack: () => void
  onOpenDetail: (detail: MarketplaceDetail) => void
  onInstallHubGroup: (group: HubPluginGroup) => void
  onUninstallHubGroup: (group: HubPluginGroup) => void
  onAdd: (item: MarketplaceItem) => void
  onUninstallItem: (item: MarketplaceItem) => void
  onToggleSkillEnabled: (id: string, enabled: boolean) => void
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  if (detail.kind === 'hub-plugin') {
    const primary = detail.group.find((item) => item.requiredInstall) ?? detail.group[0]
    if (!primary) return <DetailMissing onBack={onBack} t={t} />
    const title = hubPluginDisplayName(primary)
    const installed = hubPluginGroupInstalled(detail.group)
    const actionBusy = busyId === hubPluginBusyKey(primary)
    const sourceLabel = hubPluginMarketplaceLabel(primary)
    const relatedSkills = hubSkills.filter((group) => {
      const skill = group[0]
      return Boolean(skill) &&
        skill.pluginName === primary.pluginName &&
        skill.upstreamMarketplaceName === primary.upstreamMarketplaceName
    })
    const fields: DetailField[] = [
      { label: t('pluginDetailDeveloper'), value: primary.developerName || t('pluginDetailUnavailable') },
      { label: t('pluginDetailCategory'), value: primary.category || t('pluginDetailUnavailable') },
      { label: t('pluginDetailVersion'), value: primary.version || t('pluginDetailUnavailable') },
      { label: t('pluginDetailPlatform'), value: hubPluginPlatformSummary(detail.group) || t('pluginDetailUnavailable') },
      { label: t('pluginDetailMarketplace'), value: sourceLabel },
      { label: t('pluginDetailStatus'), value: installed ? t('pluginAdded') : t('pluginDetailAvailable') },
      ...(primary.requiresCodexConnector ? [{ label: t('pluginDetailConnector'), value: t('pluginDetailConnectorRequired') }] : [])
    ]
    const links = hubPluginLinks(primary).map((field) => ({
      ...field,
      label:
        field.label === 'website' ? t('pluginDetailWebsite') :
        field.label === 'privacy' ? t('pluginDetailPrivacy') :
        t('pluginDetailTerms')
    }))
    const capabilities = hubPluginCapabilities(primary)
    const prompts = hubPluginDefaultPrompts(primary)
    const includes = hubPluginIncludes(primary, relatedSkills, t)
    return (
      <DetailShell
        onBack={onBack}
        backLabel={t('pluginDetailBack')}
        icon={<HubPluginIcon plugin={primary} title={title} size="large" />}
        title={title}
        badge={sourceLabel}
        description={hubPluginLongDescription(primary, t('pluginHubPluginFallbackDesc'))}
        action={installed ? (
          <PluginUninstallButton
            busy={actionBusy || hubCatalogLoading}
            onClick={() => onUninstallHubGroup(detail.group)}
            t={t}
          />
        ) : (
          <button
            type="button"
            disabled={actionBusy || hubCatalogLoading}
            onClick={() => onInstallHubGroup(detail.group)}
            className="inline-flex h-10 items-center justify-center rounded-xl border border-[#d8d8d8] bg-white px-4 text-[15px] font-semibold text-[#0d0d0d] transition hover:bg-black/[0.04] disabled:cursor-not-allowed disabled:opacity-60 dark:border-ds-border dark:bg-ds-card dark:text-ds-ink dark:hover:bg-ds-hover"
          >
            {actionBusy || hubCatalogLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : t('pluginInstall')}
          </button>
        )}
      >
        <DetailSection title={t('pluginDetailInformation')}>
          <DetailInfoGrid fields={fields} />
        </DetailSection>
        {capabilities.length > 0 ? (
          <DetailSection title={t('pluginDetailCapabilities')}>
            <div className="flex flex-wrap gap-2">
              {capabilities.map((capability) => (
                <span
                  key={capability}
                  className="rounded-md border border-[#e5e5e5] bg-transparent px-2 py-1 text-[13px] text-[#4f4f4f] dark:border-ds-border dark:text-ds-muted"
                >
                  {capability}
                </span>
              ))}
            </div>
          </DetailSection>
        ) : null}
        {includes.length > 0 ? (
          <DetailSection title={t('pluginDetailIncludes')}>
            <DetailEntryList entries={includes} onOpenDetail={onOpenDetail} />
          </DetailSection>
        ) : null}
        {prompts.length > 0 ? (
          <DetailSection title={t('pluginDetailPrompts')}>
            <DetailEntryList entries={prompts.map((prompt) => ({ title: prompt }))} />
          </DetailSection>
        ) : null}
        {links.length > 0 ? (
          <DetailSection title={t('pluginDetailLinks')}>
            <DetailLinkList links={links} />
          </DetailSection>
        ) : null}
      </DetailShell>
    )
  }

  if (detail.kind === 'hub-skill') {
    const primary = detail.group.find((item) => item.requiredInstall) ?? detail.group[0]
    if (!primary) return <DetailMissing onBack={onBack} t={t} />
    const title = hubSkillDisplayName(primary)
    const installed = detail.group.some((item) => item.installed || item.requiredInstall)
    const fields: DetailField[] = [
      { label: t('pluginDetailPlugin'), value: primary.pluginName || t('pluginDetailUnavailable') },
      { label: t('pluginDetailScope'), value: hubSkillScopeLabel(primary, t) },
      { label: t('pluginDetailVersion'), value: primary.version || t('pluginDetailUnavailable') },
      { label: t('pluginDetailPlatform'), value: hubSkillPlatformSummary(detail.group) || t('pluginDetailUnavailable') },
      { label: t('pluginDetailMarketplace'), value: primary.upstreamMarketplaceName || t('pluginDetailUnavailable') },
      { label: t('pluginDetailCategory'), value: primary.category || t('pluginDetailUnavailable') },
      { label: t('pluginDetailSkillPath'), value: primary.skillPath || t('pluginDetailUnavailable'), mono: true },
      { label: t('pluginDetailStatus'), value: installed ? t('pluginAdded') : t('pluginDetailAvailable') }
    ]
    return (
      <DetailShell
        onBack={onBack}
        backLabel={t('pluginDetailBack')}
        icon={<HubSkillIcon skill={primary} title={title} size="large" />}
        title={title}
        badge={hubSkillScopeLabel(primary, t)}
        description={primary.shortDescription || t('pluginHubSkillFallbackDesc')}
        action={(
          <span className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-black/[0.06] px-4 text-[15px] font-normal text-[#0d0d0d] dark:bg-ds-subtle dark:text-ds-ink">
            {installed ? <CheckCircle2 className="h-4 w-4" strokeWidth={1.8} /> : <Plus className="h-4 w-4" strokeWidth={1.8} />}
            {installed ? t('pluginAdded') : t('pluginDetailAvailable')}
          </span>
        )}
      >
        <DetailSection title={t('pluginDetailInformation')}>
          <DetailInfoGrid fields={fields} />
        </DetailSection>
      </DetailShell>
    )
  }

  const item = detail.item
  const title = itemTitle(item, t)
  const description = itemDescription(item, t)
  const itemKey = storageKey(item.kind, item.id)
  const installed = isInstalled(item)
  const busy = busyId === itemKey
  const normalizedSkillId = normalizeSkillId(item.id)
  const skillDisabled = item.kind === 'skill' && disabledSkillIds.includes(normalizedSkillId)
  const canToggleSkill = item.kind === 'skill' && item.group === 'personal'
  const toggleBusy = skillToggleBusyId === normalizedSkillId
  const mcpConfig = item.kind === 'mcp' ? mcpServerConfigFromText(mcpConfigText, item.id) : undefined
  const mcpDisabled = item.kind === 'mcp' && !mcpServerEnabledFromConfig(mcpConfig)
  const canToggleMcp = item.kind === 'mcp' && item.group === 'personal' && !!mcpConfig
  const canUninstallLocalItem =
    (item.kind === 'mcp' || (item.kind === 'skill' && item.group === 'personal')) &&
    installed &&
    !item.systemManaged
  const isLocalMcp = item.kind === 'mcp'
  const localMcpMeta = isLocalMcp ? localMcpDetailMetadata(item) : null
  const localMcpSource = isLocalMcp ? localMcpSourceLabel(item, t) : ''
  const localStatus = installed
    ? mcpDisabled ? t('pluginMcpStatusDisabled') : t('pluginAdded')
    : t('pluginDetailAvailable')
  const fields: DetailField[] = isLocalMcp && localMcpMeta ? [
    { label: t('pluginDetailDeveloper'), value: localMcpMeta.developer },
    { label: t('pluginDetailCategory'), value: localMcpMeta.category },
    ...(localMcpMeta.version ? [{ label: t('pluginDetailVersion'), value: localMcpMeta.version }] : []),
    { label: t('pluginDetailMarketplace'), value: localMcpSource },
    { label: t('pluginDetailStatus'), value: localStatus },
    { label: t('pluginDetailId'), value: item.id, mono: true }
  ] : [
    { label: t('pluginDetailKind'), value: localItemKindLabel(item.kind, t) },
    { label: t('pluginDetailGroup'), value: item.group === 'personal' ? t('pluginPersonal') : t('pluginRecommended') },
    { label: t('pluginDetailStatus'), value: localStatus },
    ...(item.sourceLabel ? [{ label: t('pluginDetailSource'), value: item.sourceLabel }] : []),
    ...(item.detail ? [{ label: t('pluginDetailRuntime'), value: item.detail, mono: true }] : []),
    { label: t('pluginDetailId'), value: item.id, mono: true }
  ]
  const localCapabilities = localMcpMeta?.capabilities ?? []
  const localIncludes = isLocalMcp ? localMcpIncludes(item, mcpConfig) : []
  const skillToggleAction = canToggleSkill ? (
    <button
      type="button"
      disabled={toggleBusy}
      onClick={() => onToggleSkillEnabled(item.id, skillDisabled)}
      className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-black/[0.06] px-4 text-[15px] font-normal text-[#0d0d0d] transition hover:bg-black/[0.1] disabled:cursor-not-allowed disabled:opacity-60 dark:bg-ds-subtle dark:text-ds-ink dark:hover:bg-ds-hover"
    >
      {toggleBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
      {skillDisabled ? t('pluginSkillEnable') : t('pluginSkillDisable')}
    </button>
  ) : null
  const localAction = canUninstallLocalItem ? (
    <div className="flex items-center gap-2">
      {skillToggleAction}
      <PluginUninstallButton
        busy={busy}
        onClick={() => onUninstallItem(item)}
        t={t}
      />
    </div>
  ) : canToggleSkill ? (
    skillToggleAction
  ) : canToggleMcp ? (
    <span className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-black/[0.06] px-4 text-[15px] font-normal text-[#0d0d0d] dark:bg-ds-subtle dark:text-ds-ink">
      <CheckCircle2 className="h-4 w-4" strokeWidth={1.8} />
      {mcpDisabled ? t('pluginMcpStatusDisabled') : t('pluginAdded')}
    </span>
  ) : (
    <button
      type="button"
      disabled={installed || busy}
      onClick={() => onAdd(item)}
      className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-[#d8d8d8] bg-white px-4 text-[15px] font-semibold text-[#0d0d0d] transition hover:bg-black/[0.04] disabled:cursor-not-allowed disabled:opacity-60 dark:border-ds-border dark:bg-ds-card dark:text-ds-ink dark:hover:bg-ds-hover"
    >
      {busy ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : installed ? (
        <CheckCircle2 className="h-4 w-4" strokeWidth={1.8} />
      ) : null}
      {installed ? t('pluginAdded') : t('pluginInstall')}
    </button>
  )
  return (
    <DetailShell
      onBack={onBack}
      backLabel={t('pluginDetailBack')}
      icon={<LocalItemIcon title={title} iconSrc={item.iconSrc} iconBleed={item.iconBleed} />}
      title={title}
      badge={isLocalMcp ? localMcpSource : localItemKindLabel(item.kind, t)}
      description={description || item.detail || t('pluginCustomFallbackDesc')}
      action={localAction}
    >
      <DetailSection title={t('pluginDetailInformation')}>
        <DetailInfoGrid fields={fields} />
      </DetailSection>
      {localCapabilities.length > 0 ? (
        <DetailSection title={t('pluginDetailCapabilities')}>
          <div className="flex flex-wrap gap-2">
            {localCapabilities.map((capability) => (
              <span
                key={capability}
                className="rounded-md border border-[#e5e5e5] bg-transparent px-2 py-1 text-[13px] text-[#4f4f4f] dark:border-ds-border dark:text-ds-muted"
              >
                {capability}
              </span>
            ))}
          </div>
        </DetailSection>
      ) : null}
      {localIncludes.length > 0 ? (
        <DetailSection title={t('pluginDetailIncludes')}>
          <DetailEntryList entries={localIncludes} />
        </DetailSection>
      ) : null}
      {item.skillInstructions ? (
        <DetailSection title={t('pluginDetailInstructions')}>
          <p className="whitespace-pre-wrap text-[14px] leading-6 text-[#6f6f6f] dark:text-ds-muted">
            {item.skillInstructions}
          </p>
        </DetailSection>
      ) : null}
    </DetailShell>
  )
}

function DetailShell({
  onBack,
  backLabel,
  icon,
  title,
  badge,
  description,
  action,
  children
}: {
  onBack: () => void
  backLabel: string
  icon: ReactElement
  title: string
  badge?: string
  description: string
  action: ReactNode
  children: ReactNode
}): ReactElement {
  return (
    <div className="mt-12">
      <button
        type="button"
        onClick={onBack}
        className="inline-flex h-9 items-center gap-2 rounded-lg bg-black/[0.05] px-3 text-[14px] font-normal text-[#0d0d0d] transition hover:bg-black/[0.1] dark:bg-ds-subtle dark:text-ds-ink dark:hover:bg-ds-hover"
      >
        <ArrowLeft className="h-4 w-4" strokeWidth={1.8} />
        {backLabel}
      </button>
      <div className="mt-10 flex flex-col gap-5 border-b border-[#ececec] pb-8 md:flex-row md:items-start md:justify-between dark:border-ds-border">
        <div className="flex min-w-0 gap-4">
          {icon}
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="min-w-0 text-[32px] font-normal leading-tight tracking-normal text-[#0d0d0d] dark:text-ds-ink md:text-[38px]">
                {title}
              </h1>
              {badge ? (
                <span className="rounded-md border border-[#e5e5e5] bg-transparent px-2 py-1 text-[12px] font-medium text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted">
                  {badge}
                </span>
              ) : null}
            </div>
            <p className="mt-3 max-w-[720px] text-[15px] leading-7 text-[#6f6f6f] dark:text-ds-muted">
              {description}
            </p>
          </div>
        </div>
        <div className="shrink-0 md:pt-1">
          {action}
        </div>
      </div>
      <div className="divide-y divide-[#ececec] dark:divide-ds-border">
        {children}
      </div>
    </div>
  )
}

function DetailSection({
  title,
  children
}: {
  title: string
  children: ReactNode
}): ReactElement {
  return (
    <section className="py-7">
      <h2 className="text-[20px] font-normal leading-6 text-[#0d0d0d] dark:text-ds-ink">
        {title}
      </h2>
      <div className="mt-5">
        {children}
      </div>
    </section>
  )
}

function DetailInfoGrid({ fields }: { fields: DetailField[] }): ReactElement {
  const visibleFields = fields.filter((field) => field.value)
  return (
    <div className="grid gap-x-10 gap-y-4 md:grid-cols-2">
      {visibleFields.map((field) => (
        <div key={`${field.label}:${field.value}`} className="flex min-w-0 items-baseline justify-between gap-4 border-b border-[#f1f1f1] pb-3 dark:border-ds-border-muted">
          <span className="shrink-0 text-[13px] text-[#8f8f8f] dark:text-ds-faint">
            {field.label}
          </span>
          <span className={`min-w-0 truncate text-right text-[14px] text-[#4f4f4f] dark:text-ds-muted ${field.mono ? 'font-mono' : ''}`} title={field.value}>
            {field.value}
          </span>
        </div>
      ))}
    </div>
  )
}

function DetailEntryList({
  entries,
  onOpenDetail
}: {
  entries: DetailListEntry[]
  onOpenDetail?: (detail: MarketplaceDetail) => void
}): ReactElement {
  return (
    <div className="grid gap-x-10 gap-y-5 md:grid-cols-2">
      {entries.map((entry) => {
        const content = (
          <>
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[15px] font-semibold leading-5 text-[#0d0d0d] dark:text-ds-ink">
                {entry.title}
              </span>
              {entry.badge ? (
                <span className="shrink-0 rounded-md border border-[#e5e5e5] px-1.5 py-0.5 text-[11px] text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted">
                  {entry.badge}
                </span>
              ) : null}
            </div>
            {entry.description ? (
              <p className="mt-1 truncate text-[14px] leading-5 text-[#6f6f6f] dark:text-ds-muted" title={entry.description}>
                {entry.description}
              </p>
            ) : null}
          </>
        )
        const key = `${entry.title}:${entry.description ?? ''}`
        const targetDetail = entry.detail
        if (targetDetail && onOpenDetail) {
          return (
            <button
              key={key}
              type="button"
              onClick={() => onOpenDetail(targetDetail)}
              className="group -mx-2 flex min-w-0 items-center justify-between gap-3 rounded-lg border-b border-[#f1f1f1] px-2 pb-4 pt-1 text-left transition hover:bg-black/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:border-ds-border-muted dark:hover:bg-ds-hover dark:focus-visible:ring-white/15"
            >
              <div className="min-w-0">
                {content}
              </div>
              <ChevronRight className="h-4 w-4 shrink-0 text-[#8f8f8f] transition group-hover:translate-x-0.5 dark:text-ds-faint" strokeWidth={1.8} />
            </button>
          )
        }
        return (
          <div key={key} className="min-w-0 border-b border-[#f1f1f1] pb-4 dark:border-ds-border-muted">
            {content}
          </div>
        )
      })}
    </div>
  )
}

function DetailLinkList({ links }: { links: DetailField[] }): ReactElement {
  return (
    <div className="grid gap-x-10 gap-y-3 md:grid-cols-2">
      {links.map((link) => (
        <a
          key={`${link.label}:${link.value}`}
          href={link.value}
          target="_blank"
          rel="noreferrer"
          className="flex min-w-0 items-center justify-between gap-3 rounded-lg px-0 py-2 text-[14px] text-[#0d0d0d] transition hover:text-[#4f4f4f] dark:text-ds-ink dark:hover:text-ds-muted"
        >
          <span>{link.label}</span>
          <span className="flex min-w-0 items-center gap-2 text-[#8f8f8f] dark:text-ds-faint">
            <span className="truncate">{link.value}</span>
            <ExternalLink className="h-3.5 w-3.5 shrink-0" strokeWidth={1.8} />
          </span>
        </a>
      ))}
    </div>
  )
}

function HubPluginIcon({
  plugin,
  title,
  size = 'small'
}: {
  plugin: HubAgentPluginListItem
  title: string
  size?: 'small' | 'large'
}): ReactElement {
  const sizeClass = size === 'large' ? 'h-16 w-16 rounded-xl text-[18px]' : 'h-10 w-10 rounded-lg text-[13px]'
  return (
    <div className={`flex shrink-0 items-center justify-center overflow-hidden border border-[#e5e5e5] bg-transparent font-semibold text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted ${sizeClass}`}>
      {plugin.iconDataUrl || plugin.iconUrl ? (
        <img src={plugin.iconDataUrl || plugin.iconUrl} alt={title} className="h-full w-full object-contain" />
      ) : (
        <span style={{ color: hubPluginBrandColor(plugin) }}>{hubPluginInitials(title)}</span>
      )}
    </div>
  )
}

function HubSkillIcon({
  skill,
  title,
  size = 'small'
}: {
  skill: HubAgentSkillListItem
  title: string
  size?: 'small' | 'large'
}): ReactElement {
  const sizeClass = size === 'large' ? 'h-16 w-16 rounded-xl' : 'h-10 w-10 rounded-lg'
  const iconSrc = skill.iconDataUrl || (/^[a-z]+:/iu.test(skill.iconUrl ?? '') ? skill.iconUrl : '')
  return (
    <div className={`flex shrink-0 items-center justify-center overflow-hidden border border-[#e5e5e5] bg-transparent text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted ${sizeClass}`}>
      {iconSrc ? (
        <img src={iconSrc} alt={title} className="h-full w-full object-contain" />
      ) : (
        <Package className={size === 'large' ? 'h-6 w-6' : 'h-4 w-4'} strokeWidth={1.75} />
      )}
    </div>
  )
}

function HubSkillDetailModal({
  group,
  onClose,
  disabledSkillIds,
  skillToggleBusyId,
  onToggleSkillEnabled,
  t
}: {
  group: HubSkillGroup
  onClose: () => void
  disabledSkillIds: string[]
  skillToggleBusyId: string | null
  onToggleSkillEnabled: (id: string, enabled: boolean) => Promise<void>
  t: (key: string, values?: Record<string, unknown>) => string
}): ReactElement {
  const primary = group.find((item) => item.requiredInstall) ?? group[0]
  const title = primary ? hubSkillDisplayName(primary) : ''
  const installed = group.some((item) => item.installed || item.requiredInstall)
  const toggleId = primary ? hubSkillToggleId(primary) : ''
  const skillDisabled = toggleId ? disabledSkillIds.includes(toggleId) : false
  const canToggleSkill = Boolean(toggleId && installed)
  const toggleBusy = toggleId ? skillToggleBusyId === toggleId : false
  const enabled = canToggleSkill ? !skillDisabled : installed
  const [moreOpen, setMoreOpen] = useState(false)
  const [contentState, setContentState] = useState<{
    loading: boolean
    content: string
    path: string
    error: string
  }>({
    loading: false,
    content: '',
    path: '',
    error: ''
  })
  const sendMessage = useChatStore((state) => state.sendMessage)
  const runtimeConnection = useChatStore((state) => state.runtimeConnection)
  const busy = useChatStore((state) => state.busy)
  const canTryInChat = runtimeConnection === 'ready' && !busy

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  useEffect(() => {
    if (!primary) return
    let cancelled = false
    const request = hubSkillMarkdownRequest(primary)
    const readMarkdown = async (allowFullSync: boolean): Promise<void> => {
      const read = window.analytix?.app?.readHubAgentSkillMarkdown
      if (typeof read !== 'function') {
        throw new Error(t('pluginSkillContentUnavailable'))
      }
      let result = await read(request)
      if (!result.ok && allowFullSync && typeof window.analytix?.app?.syncHubAgentMarketplace === 'function') {
        await window.analytix.app.syncHubAgentMarketplace({ mode: 'full' })
        result = await read(request)
      }
      if (!result.ok) {
        throw new Error(result.message)
      }
      const content = skillMarkdownBody(result.content, {
        path: result.path || primary.skillPath,
        expectedTitle: title
      })
      if (!cancelled) {
        setContentState({
          loading: false,
          content: content || result.content,
          path: result.path,
          error: ''
        })
      }
    }

    setContentState({ loading: true, content: '', path: '', error: '' })
    void readMarkdown(true).catch((error) => {
      if (cancelled) return
      setContentState({
        loading: false,
        content: '',
        path: '',
        error: skillContentErrorMessage(error, t)
      })
    })
    return () => {
      cancelled = true
    }
  }, [primary, t, title])

  const handleTryInChat = useCallback(async (): Promise<void> => {
    if (!primary || !canTryInChat) return
    const sent = await sendMessage(hubSkillTryPrompt(primary, t), 'agent', {
      displayText: t('pluginSkillTryDisplay', { name: title })
    })
    if (sent) onClose()
  }, [canTryInChat, onClose, primary, sendMessage, t, title])

  const handleToggleSkill = useCallback(async (): Promise<void> => {
    if (!toggleId || !canToggleSkill || toggleBusy) return
    await onToggleSkillEnabled(toggleId, !enabled)
  }, [canToggleSkill, enabled, onToggleSkillEnabled, toggleBusy, toggleId])

  const handleOpenSkillLocation = useCallback(async (): Promise<void> => {
    const dir = contentState.path ? skillPathDirectory(contentState.path) : ''
    if (!dir) return
    setMoreOpen(false)
    const open = window.analytix?.app?.openSkillRoot
    if (typeof open !== 'function') return
    const result = await open(dir)
    if (!result.ok) {
      setContentState((state) => ({
        ...state,
        error: result.message ?? t('pluginActionFailed')
      }))
    }
  }, [contentState.path, t])

  if (!primary) return <></>

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4 py-8">
      <button
        type="button"
        aria-label={t('pluginDetailClose')}
        onClick={onClose}
        className="absolute inset-0 bg-black/25 backdrop-blur-[2px] dark:bg-black/55"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="plugin-skill-detail-title"
        className="relative flex max-h-[84vh] w-full max-w-[820px] flex-col overflow-hidden rounded-[22px] border border-[#d9d9d9] bg-white shadow-2xl dark:border-ds-border dark:bg-ds-card"
      >
        <div className="flex items-start justify-between gap-5 px-6 pb-5 pt-6 dark:border-ds-border">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-3">
              <h2 id="plugin-skill-detail-title" className="min-w-0 truncate text-[26px] font-semibold leading-tight text-[#0d0d0d] dark:text-ds-ink">
                {title}
              </h2>
              <span className="shrink-0 text-[26px] font-normal leading-tight text-[#6f6f6f] dark:text-ds-muted">
                Skill
              </span>
            </div>
            <p className="mt-3 max-w-[650px] text-[17px] leading-7 text-[#6f6f6f] dark:text-ds-muted">
              {primary.shortDescription || t('pluginHubSkillFallbackDesc')}
            </p>
          </div>
          <div className="flex shrink-0 items-start gap-2">
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              aria-label={enabled ? t('pluginSkillDisable') : t('pluginSkillEnable')}
              disabled={!canToggleSkill || toggleBusy}
              onClick={() => void handleToggleSkill()}
              className={`mt-1 flex h-7 w-12 items-center rounded-full p-0.5 transition disabled:cursor-not-allowed disabled:opacity-60 ${enabled ? 'justify-end bg-[#2f98ff]' : 'justify-start bg-[#d8d8d8] dark:bg-ds-border'}`}
            >
              {toggleBusy ? (
                <span className="flex h-6 w-6 items-center justify-center rounded-full bg-white shadow-sm">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-[#6f6f6f]" strokeWidth={2} />
                </span>
              ) : (
                <span className="h-6 w-6 rounded-full bg-white shadow-sm" />
              )}
            </button>
            <div className="relative">
              <button
                type="button"
                aria-label={t('pluginSkillMoreActions')}
                disabled={!contentState.path}
                onClick={() => setMoreOpen((value) => !value)}
                className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[#6f6f6f] transition hover:bg-black/[0.06] disabled:cursor-not-allowed disabled:opacity-45 dark:text-ds-muted dark:hover:bg-ds-hover"
              >
                <MoreHorizontal className="h-4 w-4" strokeWidth={1.8} />
              </button>
              {moreOpen ? (
                <div className="absolute right-0 top-9 z-10 w-40 rounded-xl border border-[#e5e5e5] bg-white p-1 shadow-lg dark:border-ds-border dark:bg-ds-card">
                  <button
                    type="button"
                    onClick={() => void handleOpenSkillLocation()}
                    className="flex w-full items-center rounded-lg px-3 py-2 text-left text-[13px] text-[#0d0d0d] transition hover:bg-black/[0.05] dark:text-ds-ink dark:hover:bg-ds-hover"
                  >
                    {t('pluginOpenLocation')}
                  </button>
                </div>
              ) : null}
            </div>
            <div className="min-w-0">
              <button
                type="button"
                onClick={onClose}
                aria-label={t('pluginDetailClose')}
                className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[#0d0d0d] transition hover:bg-black/[0.06] dark:text-ds-ink dark:hover:bg-ds-hover"
              >
                <X className="h-4 w-4" strokeWidth={1.8} />
              </button>
            </div>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-6">
          <div className="min-h-[300px] rounded-xl border border-[#ececec] bg-[#fbfbfb] px-5 py-5 dark:border-ds-border dark:bg-ds-subtle/40">
            {contentState.loading ? (
              <div className="flex items-center gap-2 text-[15px] text-[#6f6f6f] dark:text-ds-muted">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>{t('pluginSkillContentLoading')}</span>
              </div>
            ) : contentState.error ? (
              <div className="text-[15px] leading-6 text-[#6f6f6f] dark:text-ds-muted">
                <p>{t('pluginSkillContentError')}</p>
                <p className="mt-2 font-mono text-[12px] text-[#8f8f8f] dark:text-ds-faint">{contentState.error}</p>
              </div>
            ) : (
              <div className="max-w-none text-[#4f4f4f] dark:text-ds-muted">
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    p: ({ children }) => <p className="mb-4 text-[15px] leading-6 text-[#4f4f4f] dark:text-ds-muted">{children}</p>,
                    h1: ({ children }) => <h1 className="mb-2 mt-4 text-[18px] font-semibold leading-6 text-[#4f4f4f] dark:text-ds-ink">{children}</h1>,
                    h2: ({ children }) => <h2 className="mb-2 mt-4 text-[16px] font-semibold leading-6 text-[#4f4f4f] dark:text-ds-ink">{children}</h2>,
                    h3: ({ children }) => <h3 className="mb-1.5 mt-3 text-[15px] font-semibold leading-6 text-[#4f4f4f] dark:text-ds-ink">{children}</h3>,
                    ul: ({ children }) => <ul className="mb-4 list-disc space-y-2 pl-6 text-[15px] leading-6 text-[#4f4f4f] dark:text-ds-muted">{children}</ul>,
                    ol: ({ children }) => <ol className="mb-4 list-decimal space-y-2 pl-6 text-[15px] leading-6 text-[#4f4f4f] dark:text-ds-muted">{children}</ol>,
                    li: ({ children }) => <li className="pl-1">{children}</li>,
                    strong: ({ children }) => <strong className="font-semibold text-[#4f4f4f] dark:text-ds-ink">{children}</strong>,
                    code: ({ children }) => <code className="rounded-md bg-black/[0.06] px-1.5 py-0.5 font-mono text-[0.9em] text-[#4f4f4f] dark:bg-white/10 dark:text-ds-ink">{children}</code>,
                    pre: ({ children }) => <pre className="mb-4 overflow-x-auto rounded-lg bg-black/[0.04] p-3 text-[13px] leading-5 dark:bg-black/25">{children}</pre>
                  }}
                >
                  {contentState.content}
                </ReactMarkdown>
              </div>
            )}
          </div>
        </div>
        <div className="flex shrink-0 justify-end px-6 pb-6 pt-4">
          <button
            type="button"
            onClick={() => void handleTryInChat()}
            disabled={!canTryInChat}
            className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-[#181818] px-4 text-[14px] font-semibold text-white shadow-sm transition hover:bg-[#2a2a2a] disabled:cursor-not-allowed disabled:opacity-50 dark:bg-white dark:text-black dark:hover:bg-white/90"
          >
            <MessageCircleMore className="h-4 w-4" strokeWidth={1.8} />
            {t('pluginSkillTryInChat')}
          </button>
        </div>
      </div>
    </div>
  )
}

function LocalItemIcon({
  title,
  iconSrc,
  iconBleed = false,
  size = 'large'
}: {
  title: string
  iconSrc?: string
  iconBleed?: boolean
  size?: 'small' | 'large'
}): ReactElement {
  const sizeClass = size === 'large' ? 'h-16 w-16 rounded-xl text-[18px]' : 'h-10 w-10 rounded-lg text-[13px]'
  const frameClass = iconSrc
    ? `flex shrink-0 items-center justify-center overflow-hidden bg-[#151515] ${sizeClass}`
    : `flex shrink-0 items-center justify-center overflow-hidden border border-[#e5e5e5] bg-transparent font-semibold text-[#6f6f6f] dark:border-ds-border dark:text-ds-muted ${sizeClass}`
  const imageClass = iconBleed
    ? 'block h-full w-full scale-[1.2] object-cover'
    : 'block h-full w-full object-cover'
  return (
    <div className={frameClass}>
      {iconSrc ? (
        <img src={iconSrc} alt={title} className={imageClass} />
      ) : (
        <span>{hubPluginInitials(title)}</span>
      )}
    </div>
  )
}

function DetailMissing({
  onBack,
  t
}: {
  onBack: () => void
  t: (key: string) => string
}): ReactElement {
  return (
    <DetailShell
      onBack={onBack}
      backLabel={t('pluginDetailBack')}
      icon={<LocalItemIcon title="?" />}
      title={t('pluginDetailMissing')}
      description={t('pluginDetailMissingDescription')}
      action={<span />}
    >
      <DetailSection title={t('pluginDetailInformation')}>
        <p className="text-[14px] text-[#6f6f6f] dark:text-ds-muted">
          {t('pluginDetailUnavailable')}
        </p>
      </DetailSection>
    </DetailShell>
  )
}

function CustomPluginPanel({
  activeKind,
  customName,
  customDescription,
  customCommand,
  customArgs,
  customConfig,
  customSkillBody,
  busy,
  onNameChange,
  onDescriptionChange,
  onCommandChange,
  onArgsChange,
  onConfigChange,
  onSkillBodyChange,
  onAdd
}: {
  activeKind: PluginKind
  customName: string
  customDescription: string
  customCommand: string
  customArgs: string
  customConfig: string
  customSkillBody: string
  busy: boolean
  onNameChange: (value: string) => void
  onDescriptionChange: (value: string) => void
  onCommandChange: (value: string) => void
  onArgsChange: (value: string) => void
  onConfigChange: (value: string) => void
  onSkillBodyChange: (value: string) => void
  onAdd: () => void
}): ReactElement {
  const { t } = useTranslation('common')
  return (
    <section className="mt-6 rounded-2xl border border-ds-border bg-ds-card/95 p-4 shadow-sm">
      <div className="grid gap-3 md:grid-cols-2">
        <input
          value={customName}
          onChange={(event) => onNameChange(event.target.value)}
          className="h-10 rounded-xl border border-ds-border bg-ds-main/45 px-3 text-[14px] text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
          placeholder={t('pluginCustomName')}
        />
        <input
          value={customDescription}
          onChange={(event) => onDescriptionChange(event.target.value)}
          className="h-10 rounded-xl border border-ds-border bg-ds-main/45 px-3 text-[14px] text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
          placeholder={t('pluginCustomDescription')}
        />
      </div>
      {activeKind === 'mcp' ? (
        <div className="mt-3 grid gap-3">
          <div className="grid gap-3 md:grid-cols-2">
            <input
              value={customCommand}
              onChange={(event) => onCommandChange(event.target.value)}
              className="h-10 rounded-xl border border-ds-border bg-ds-main/45 px-3 text-[14px] text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
              placeholder={t('pluginCustomCommand')}
            />
            <textarea
              value={customArgs}
              onChange={(event) => onArgsChange(event.target.value)}
              className="min-h-[80px] rounded-xl border border-ds-border bg-ds-main/45 px-3 py-2 font-mono text-[13px] leading-5 text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
              placeholder={t('pluginCustomArgs')}
              spellCheck={false}
            />
          </div>
          <textarea
            value={customConfig}
            onChange={(event) => onConfigChange(event.target.value)}
            className="min-h-[120px] rounded-xl border border-ds-border bg-ds-main/45 px-3 py-2 font-mono text-[13px] leading-5 text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
            placeholder={t('pluginCustomMcpConfig')}
            spellCheck={false}
          />
        </div>
      ) : (
        <textarea
          value={customSkillBody}
          onChange={(event) => onSkillBodyChange(event.target.value)}
          className="mt-3 min-h-[140px] w-full rounded-xl border border-ds-border bg-ds-main/45 px-3 py-2 font-mono text-[13px] leading-5 text-ds-ink outline-none focus:border-accent/40 focus:ring-1 focus:ring-accent/30"
          placeholder={t('pluginCustomSkillBody')}
          spellCheck={false}
        />
      )}
      <div className="mt-3 flex justify-end">
        <button
          type="button"
          onClick={onAdd}
          disabled={busy}
          className="inline-flex items-center gap-2 rounded-xl bg-ds-userbubble px-4 py-2 text-[13px] font-semibold text-ds-userbubbleFg shadow-sm transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-55"
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2} /> : <Plus className="h-4 w-4" strokeWidth={2} />}
          {t('pluginAddCustom')}
        </button>
      </div>
    </section>
  )
}
