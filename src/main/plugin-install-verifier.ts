import { createHash } from 'node:crypto'
import { basename, isAbsolute, join, relative, resolve } from 'node:path'
import { parseStrictJsonObject } from './controlled-artifact/strict-json'
import {
  HUB_PLUGIN_MARKER_FILENAME,
  readInstalledPluginMarkerV1,
  type InstalledPluginMarkerV1
} from './plugin-install-marker'
import {
  computePluginSourceTreeIdentity,
  readStablePluginSourceFile,
  type PluginSourceTreeIdentity
} from './plugin-source-integrity'
import type { BundledFundsMaterializationBindingV1 } from './runtime/bundled-funds-materialization'

const MAX_PLUGIN_MANIFEST_JSON_BYTES = 256 * 1024

export type VerifiedInstalledPluginBindingV1 = {
  marker: InstalledPluginMarkerV1
  pluginName: string
  version: string
  manifest: Record<string, unknown>
  manifestBytes: Buffer
  installedTree: PluginSourceTreeIdentity
  sourceTree: PluginSourceTreeIdentity
}

export type VerifyInstalledPluginBindingV1Options = {
  runtimeHome: string
  pluginDir: string
  installCacheRoots: readonly string[]
  expectedPluginName: string
  expectedVersion?: string
  acceptedKinds?: readonly InstalledPluginMarkerV1['kind'][]
  hostMaterialization?: BundledFundsMaterializationBindingV1
}

export function verifyInstalledPluginBindingV1(
  options: VerifyInstalledPluginBindingV1Options
): VerifiedInstalledPluginBindingV1 | null {
  try {
    const parsedMarker = readInstalledPluginMarkerV1(join(options.pluginDir, HUB_PLUGIN_MARKER_FILENAME))
    if (!parsedMarker) return null
    // A hub-install marker remains user-writable consistency data. It can only
    // accompany a current-run, nonce-bound Go materialization result carrying
    // the installation-authority signed receipt and matching signed index.
    if (parsedMarker.kind === 'hub-install') {
      return verifyHostMaterializedPluginBindingV1(options, parsedMarker)
    }
    if (options.acceptedKinds && !options.acceptedKinds.includes(parsedMarker.kind)) return null
    const marker = parsedMarker.marker
    const pluginName = marker.pluginName
    const version = marker.version
    if (
      pluginName !== options.expectedPluginName ||
      (options.expectedVersion && version !== options.expectedVersion)
    ) return null

    const trustedInstallRoots = options.installCacheRoots.map((root) => (
      join(root, 'analytix-hub', pluginName)
    ))
    if (!trustedInstallRoots.some((root) => pathInsideOrEqual(options.pluginDir, root))) return null
    const generationName = basename(options.pluginDir)
    const expectedLocalGeneration = `${version}-local-${marker.packageSha256}`
    if (generationName !== version && generationName !== expectedLocalGeneration) return null

    const trustedSourceRoot = join(
      options.runtimeHome,
      '.cache',
      'analytix-hub-plugins',
      'marketplaces',
      'analytix-hub',
      'plugins',
      pluginName
    )
    if (
      !pathInsideOrEqual(marker.sourcePath, trustedSourceRoot) ||
      resolve(marker.sourcePath) === resolve(options.pluginDir)
    ) return null

    const installedTreeBefore = pluginTree(options.pluginDir)
    const sourceTreeBefore = pluginTree(marker.sourcePath)
    if (!sameTree(installedTreeBefore, sourceTreeBefore)) return null
    const installedManifestPath = join(installedTreeBefore.rootRealPath, '.codex-plugin', 'plugin.json')
    const sourceManifestPath = join(sourceTreeBefore.rootRealPath, '.codex-plugin', 'plugin.json')
    const manifestBytes = readStablePluginSourceFile(installedManifestPath, MAX_PLUGIN_MANIFEST_JSON_BYTES)
    const sourceManifestBytes = readStablePluginSourceFile(sourceManifestPath, MAX_PLUGIN_MANIFEST_JSON_BYTES)
    if (!manifestBytes.equals(sourceManifestBytes)) return null
    const manifest = parseStrictJsonObject(manifestBytes, {
      maxBytes: MAX_PLUGIN_MANIFEST_JSON_BYTES,
      maxDepth: 16,
      maxTokens: 4096,
      maxStringBytes: 64 * 1024
    })
    if (manifest.name !== pluginName || manifest.version !== version) return null

    const installedTree = pluginTree(options.pluginDir)
    const sourceTree = pluginTree(marker.sourcePath)
    if (
      !sameTree(installedTreeBefore, installedTree) ||
      !sameTree(sourceTreeBefore, sourceTree) ||
      !sameTree(installedTree, sourceTree)
    ) return null
    return {
      marker: parsedMarker,
      pluginName,
      version,
      manifest,
      manifestBytes,
      installedTree,
      sourceTree
    }
  } catch {
    return null
  }
}

function verifyHostMaterializedPluginBindingV1(
  options: VerifyInstalledPluginBindingV1Options,
  parsedMarker: Extract<InstalledPluginMarkerV1, { kind: 'hub-install' }>
): VerifiedInstalledPluginBindingV1 | null {
  const host = options.hostMaterialization
  if (!host || (options.acceptedKinds && !options.acceptedKinds.includes('hub-install'))) return null
  const marker = parsedMarker.marker
  const pluginDir = resolve(options.pluginDir)
  const trustedInstallRoots = options.installCacheRoots.map((root) => (
    join(root, 'analytix-hub', marker.pluginName)
  ))
  if (
    pluginDir !== resolve(host.activePluginRoot) ||
    !trustedInstallRoots.some((root) => pathInsideOrEqual(pluginDir, root)) ||
    basename(pluginDir) !== host.pluginVersion ||
    marker.pluginName !== host.pluginName || marker.version !== host.pluginVersion ||
    marker.pluginName !== options.expectedPluginName ||
    (options.expectedVersion && marker.version !== options.expectedVersion) ||
    marker.packageSha256 !== host.packageAuthority.fileSha256 ||
    marker.sourceTreeSha256 !== host.receipt.sourceTreeSha256 ||
    marker.sourceTreeFileCount !== host.receipt.sourceTreeFileCount ||
    host.index.receiptId !== host.receipt.receiptId ||
    host.index.receiptSha256.length !== 64 || host.index.indexDigest.length !== 64 ||
    host.factToolsEnabled || host.publishable || host.receipt.factToolsEnabled || host.index.factToolsEnabled
  ) return null

  const installedTreeBefore = pluginTree(pluginDir)
  if (
    installedTreeBefore.treeSha256 !== host.receipt.sourceTreeSha256 ||
    installedTreeBefore.fileCount !== host.receipt.sourceTreeFileCount
  ) return null
  const manifestPath = join(installedTreeBefore.rootRealPath, '.codex-plugin', 'plugin.json')
  const entrypointPath = join(installedTreeBefore.rootRealPath, 'mcp', 'server.mjs')
  const manifestBytes = readStablePluginSourceFile(manifestPath, MAX_PLUGIN_MANIFEST_JSON_BYTES)
  const entrypointBytes = readStablePluginSourceFile(entrypointPath, 128 * 1024 * 1024)
  if (
    createHash('sha256').update(manifestBytes).digest('hex') !== host.receipt.manifestSha256 ||
    createHash('sha256').update(entrypointBytes).digest('hex') !== host.receipt.entrypointSha256
  ) return null
  const manifest = parseStrictJsonObject(manifestBytes, {
    maxBytes: MAX_PLUGIN_MANIFEST_JSON_BYTES,
    maxDepth: 16,
    maxTokens: 4096,
    maxStringBytes: 64 * 1024
  })
  if (manifest.name !== marker.pluginName || manifest.version !== marker.version) return null
  const installedTree = pluginTree(pluginDir)
  if (!sameTree(installedTreeBefore, installedTree)) return null
  return {
    marker: parsedMarker,
    pluginName: marker.pluginName,
    version: marker.version,
    manifest,
    manifestBytes,
    installedTree,
    sourceTree: installedTree
  }
}

function pluginTree(rootPath: string): PluginSourceTreeIdentity {
  return computePluginSourceTreeIdentity(rootPath, {
    excludeRelativePaths: [HUB_PLUGIN_MARKER_FILENAME]
  })
}

function sameTree(left: PluginSourceTreeIdentity, right: PluginSourceTreeIdentity): boolean {
  return left.treeSha256 === right.treeSha256 && left.fileCount === right.fileCount
}

function pathInsideOrEqual(targetPath: string, rootPath: string): boolean {
  const root = resolve(rootPath)
  const target = resolve(targetPath)
  const rel = relative(root, target)
  return rel === '' || (!!rel && !rel.startsWith('..') && !isAbsolute(rel))
}
