import { basename, isAbsolute } from 'node:path'
import { parseStrictJsonObject } from './controlled-artifact/strict-json'
import { readPrivateRegularFile } from './controlled-artifact/private-regular-file'

export const HUB_PLUGIN_MARKER_FILENAME = '.analytix-hub-installed-plugin.json'
export const MAX_PLUGIN_MARKER_JSON_BYTES = 64 * 1024

export type HubInstallPluginMarkerV1 = {
  managedBy: 'analytix-hub'
  marketplaceName: 'analytix-hub'
  upstreamMarketplaceName?: string
  pluginName: string
  version: string
  packageSha256: string
  sourcePath: string
  sourceTreeSha256: string
  sourceTreeFileCount: number
  installType: 'user'
}

export type LocalRemountPluginMarkerV1 = {
  managedBy: 'analytix-hub'
  marketplaceName: 'analytix-hub'
  pluginName: string
  version: string
  packageSha256: string
  sourcePath: string
  source: { source: 'local'; path: string }
  transactionVersion: 'RuntimeCacheRemountTransactionV1'
  commitOrder: 'marketplace_pointer_last'
}

export type InstalledPluginMarkerV1 =
  | { kind: 'hub-install'; marker: HubInstallPluginMarkerV1 }
  | { kind: 'local-remount'; marker: LocalRemountPluginMarkerV1 }

const SHA256 = /^[a-f0-9]{64}$/u
const HUB_INSTALL_REQUIRED_KEYS = [
  'managedBy',
  'marketplaceName',
  'pluginName',
  'version',
  'packageSha256',
  'sourcePath',
  'sourceTreeSha256',
  'sourceTreeFileCount',
  'installType'
] as const
const HUB_INSTALL_OPTIONAL_KEYS = ['upstreamMarketplaceName'] as const
const LOCAL_REMOUNT_KEYS = [
  'managedBy',
  'marketplaceName',
  'pluginName',
  'version',
  'packageSha256',
  'sourcePath',
  'source',
  'transactionVersion',
  'commitOrder'
] as const

export function parseInstalledPluginMarkerV1(bytes: Buffer): InstalledPluginMarkerV1 | null {
  let value: Record<string, unknown>
  try {
    value = parseStrictJsonObject(bytes, {
      maxBytes: MAX_PLUGIN_MARKER_JSON_BYTES,
      maxDepth: 4,
      maxTokens: 64,
      maxStringBytes: 4096
    })
  } catch {
    return null
  }
  if (value.managedBy !== 'analytix-hub' || value.marketplaceName !== 'analytix-hub') return null
  const pluginName = exactNonEmptyString(value.pluginName)
  const version = exactNonEmptyString(value.version)
  const sourcePath = exactAbsolutePath(value.sourcePath)
  const packageSha256 = exactSha256(value.packageSha256)
  if (!pluginName || !version || !sourcePath || !packageSha256) return null

  if (hasExactKeys(value, HUB_INSTALL_REQUIRED_KEYS, HUB_INSTALL_OPTIONAL_KEYS)) {
    const upstreamMarketplaceName = Object.prototype.hasOwnProperty.call(value, 'upstreamMarketplaceName')
      ? exactNonEmptyString(value.upstreamMarketplaceName)
      : undefined
    const sourceTreeSha256 = exactSha256(value.sourceTreeSha256)
    const sourceTreeFileCount = exactPositiveSafeInteger(value.sourceTreeFileCount)
    if (
      value.installType !== 'user' ||
      (Object.prototype.hasOwnProperty.call(value, 'upstreamMarketplaceName') && !upstreamMarketplaceName) ||
      !sourceTreeSha256 ||
      sourceTreeFileCount === null
    ) return null
    return {
      kind: 'hub-install',
      marker: {
        managedBy: 'analytix-hub',
        marketplaceName: 'analytix-hub',
        ...(upstreamMarketplaceName ? { upstreamMarketplaceName } : {}),
        pluginName,
        version,
        packageSha256,
        sourcePath,
        sourceTreeSha256,
        sourceTreeFileCount,
        installType: 'user'
      }
    }
  }

  if (!hasExactKeys(value, LOCAL_REMOUNT_KEYS)) return null
  const source = objectValue(value.source)
  if (!source || !hasExactKeys(source, ['source', 'path'])) return null
  const generation = `${version}-local-${packageSha256}`
  if (
    value.transactionVersion !== 'RuntimeCacheRemountTransactionV1' ||
    value.commitOrder !== 'marketplace_pointer_last' ||
    source.source !== 'local' ||
    source.path !== `./plugins/${pluginName}/${generation}` ||
    basename(sourcePath) !== generation
  ) return null
  return {
    kind: 'local-remount',
    marker: {
      managedBy: 'analytix-hub',
      marketplaceName: 'analytix-hub',
      pluginName,
      version,
      packageSha256,
      sourcePath,
      source: { source: 'local', path: source.path },
      transactionVersion: 'RuntimeCacheRemountTransactionV1',
      commitOrder: 'marketplace_pointer_last'
    }
  }
}

export function readInstalledPluginMarkerV1(markerPath: string): InstalledPluginMarkerV1 | null {
  const state = readPrivateRegularFile(markerPath, MAX_PLUGIN_MARKER_JSON_BYTES)
  return state.status === 'valid' ? parseInstalledPluginMarkerV1(state.bytes) : null
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function exactNonEmptyString(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 && value === value.trim()
    ? value
    : null
}

function exactAbsolutePath(value: unknown): string | null {
  const path = exactNonEmptyString(value)
  return path && isAbsolute(path) ? path : null
}

function exactSha256(value: unknown): string | null {
  return typeof value === 'string' && SHA256.test(value) ? value : null
}

function exactPositiveSafeInteger(value: unknown): number | null {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 ? value : null
}

function hasExactKeys(
  value: Record<string, unknown>,
  required: readonly string[],
  optional: readonly string[] = []
): boolean {
  const keys = Object.keys(value)
  return required.every((key) => Object.prototype.hasOwnProperty.call(value, key)) &&
    keys.every((key) => required.includes(key) || optional.includes(key))
}
