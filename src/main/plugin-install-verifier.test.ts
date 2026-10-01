import {
  chmodSync,
  cpSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defaultAnalytixRuntimeSettings } from '../shared/app-settings'
import { HUB_PLUGIN_MARKER_FILENAME } from './plugin-install-marker'
import { verifyInstalledPluginBindingV1 } from './plugin-install-verifier'
import { computePluginSourceTreeIdentity } from './plugin-source-integrity'
import type { BundledFundsMaterializationBindingV1 } from './runtime/bundled-funds-materialization'

let tempRoot: string | null = null

vi.mock('electron', () => ({ app: { isPackaged: false, getAppPath: () => '/tmp/analytix-test-app' } }))
vi.mock('node:os', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:os')>()
  return { ...actual, homedir: () => tempRoot ?? actual.homedir() }
})

function runtimeHome(): string {
  if (!tempRoot) tempRoot = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-plugin-binding-')))
  return tempRoot
}

function writePluginSource(pluginName: string, generation: string, version = '1.0.0'): string {
  const sourcePath = join(
    runtimeHome(),
    '.cache',
    'analytix-hub-plugins',
    'marketplaces',
    'analytix-hub',
    'plugins',
    pluginName,
    generation
  )
  mkdirSync(join(sourcePath, '.codex-plugin'), { recursive: true })
  writeFileSync(join(sourcePath, '.codex-plugin', 'plugin.json'), JSON.stringify({
    name: pluginName,
    version
  }), 'utf8')
  writeFileSync(join(sourcePath, 'entry.mjs'), 'export const value = 1\n', 'utf8')
  return sourcePath
}

function installFixture(kind: 'hub-install' | 'local-remount'): {
  sourcePath: string
  pluginDir: string
  installCacheRoot: string
} {
  const pluginName = 'demo-plugin'
  const version = '1.0.0'
  const packageSha256 = 'a'.repeat(64)
  const sourceGeneration = kind === 'hub-install' ? version : `${version}-local-${packageSha256}`
  const sourcePath = writePluginSource(pluginName, sourceGeneration, version)
  const sourceTree = computePluginSourceTreeIdentity(sourcePath)
  const installCacheRoot = join(runtimeHome(), 'plugins', 'cache')
  const pluginDir = join(installCacheRoot, 'analytix-hub', pluginName, version)
  mkdirSync(join(pluginDir, '..'), { recursive: true })
  cpSync(sourcePath, pluginDir, { recursive: true })
  const marker = kind === 'hub-install'
    ? {
        managedBy: 'analytix-hub',
        marketplaceName: 'analytix-hub',
        upstreamMarketplaceName: 'approved-upstream',
        pluginName,
        version,
        packageSha256,
        sourcePath,
        sourceTreeSha256: sourceTree.treeSha256,
        sourceTreeFileCount: sourceTree.fileCount,
        installType: 'user'
      }
    : {
        managedBy: 'analytix-hub',
        marketplaceName: 'analytix-hub',
        pluginName,
        version,
        packageSha256,
        sourcePath,
        source: { source: 'local', path: `./plugins/${pluginName}/${sourceGeneration}` },
        transactionVersion: 'RuntimeCacheRemountTransactionV1',
        commitOrder: 'marketplace_pointer_last'
      }
  const markerPath = join(pluginDir, HUB_PLUGIN_MARKER_FILENAME)
  writeFileSync(markerPath, JSON.stringify(marker), 'utf8')
  chmodSync(markerPath, 0o600)
  return { sourcePath, pluginDir, installCacheRoot }
}

function hostMaterializedFundsFixture(): {
  pluginDir: string
  installCacheRoot: string
  hostMaterialization: BundledFundsMaterializationBindingV1
} {
  const pluginName = 'analytix-fund-analysis'
  const version = '0.16.16'
  const packageSha256 = 'd'.repeat(64)
  const installCacheRoot = join(runtimeHome(), 'plugins', 'cache')
  const pluginDir = join(installCacheRoot, 'analytix-hub', pluginName, version)
  mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
  mkdirSync(join(pluginDir, 'mcp'), { recursive: true })
  const manifestBytes = Buffer.from(JSON.stringify({
    name: pluginName,
    version,
    mcpServers: './.mcp.json'
  }))
  const entrypointBytes = Buffer.from('export const bundledFundsEntrypoint = true\n')
  writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), manifestBytes)
  writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
    mcpServers: {
      analytix_funds: {
        enabled: false,
        command: 'node',
        args: ['./mcp/server.mjs']
      }
    }
  }), 'utf8')
  writeFileSync(join(pluginDir, 'mcp', 'server.mjs'), entrypointBytes)
  const tree = computePluginSourceTreeIdentity(pluginDir)
  const sourcePath = join(runtimeHome(), 'packaged-resources', 'plugins', pluginName)
  mkdirSync(join(sourcePath, '..'), { recursive: true })
  cpSync(pluginDir, sourcePath, { recursive: true })
  const markerPath = join(pluginDir, HUB_PLUGIN_MARKER_FILENAME)
  writeFileSync(markerPath, JSON.stringify({
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    upstreamMarketplaceName: 'analytix-packaged',
    pluginName,
    version,
    packageSha256,
    sourcePath,
    sourceTreeSha256: tree.treeSha256,
    sourceTreeFileCount: tree.fileCount,
    installType: 'user'
  }), 'utf8')
  chmodSync(markerPath, 0o600)
  const receiptId = 'e'.repeat(64)
  const receiptSha256 = 'f'.repeat(64)
  const generationId = '1'.repeat(64)
  const intentId = '2'.repeat(64)
  const activeRelativePath = 'plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16' as const
  const hostMaterialization: BundledFundsMaterializationBindingV1 = {
    schemaVersion: 1,
    purpose: 'analytix.bundled-funds-materialization-ready/v1',
    invocationId: '3'.repeat(64),
    configurationBindingDigest: '4'.repeat(64),
    packageAuthority: {
      fileSha256: packageSha256,
      authorityDigest: '5'.repeat(64),
      classification: 'controlled_release_clean_candidate_non_publishable',
      dispositionKind: 'controlled_release_receipt',
      platformAnchor: 'macos_developer_id_resource_seal'
    },
    runtimeIdentity: {
      payloadSha256: '6'.repeat(64),
      payloadBytes: 4096,
      format: 'mach-o',
      arch: 'arm64'
    },
    pluginName,
    pluginVersion: version,
    activePluginRoot: pluginDir,
    receipt: {
      schemaVersion: 1,
      purpose: 'analytix.bundled-plugin-materialization-receipt/v1',
      receiptId,
      intentId,
      packageAuthoritySha256: packageSha256,
      target: { platform: 'darwin', arch: 'arm64' },
      pluginName,
      pluginVersion: version,
      generationId,
      activeRelativePath,
      sourceTreeSha256: tree.treeSha256,
      sourceTreeFileCount: tree.fileCount,
      manifestSha256: createHash('sha256').update(manifestBytes).digest('hex'),
      entrypointSha256: createHash('sha256').update(entrypointBytes).digest('hex'),
      factToolsEnabled: false,
      issuedAt: '2026-07-23T18:00:00Z',
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: '7'.repeat(64),
      authorityPublicKey: Buffer.alloc(32, 8).toString('base64url'),
      authoritySignature: Buffer.alloc(64, 9).toString('base64url')
    },
    index: {
      schemaVersion: 1,
      purpose: 'analytix.bundled-plugin-materialization-index/v1',
      indexDigest: 'a'.repeat(64),
      pluginName,
      pluginVersion: version,
      generationId,
      activeRelativePath,
      receiptId,
      receiptSha256,
      intentId,
      sourceTreeSha256: tree.treeSha256,
      sourceTreeFileCount: tree.fileCount,
      discoverableGenerationCount: 1,
      factToolsEnabled: false,
      committedAt: '2026-07-23T18:00:00Z'
    },
    publishable: false,
    factToolsEnabled: false,
    completedAt: '2026-07-23T18:00:00Z'
  }
  return { pluginDir, installCacheRoot, hostMaterialization }
}

afterEach(() => {
  if (tempRoot) rmSync(tempRoot, { recursive: true, force: true })
  tempRoot = null
})

describe('installed plugin semantic binding V1', () => {
  it('preserves independent config and retained state across unbound and bound Funds sync', async () => {
    const funds = hostMaterializedFundsFixture()
    const other = installFixture('local-remount')
    const otherManifest = JSON.stringify({ name: 'demo-plugin', version: '1.0.0', mcpServers: './.mcp.json' })
    const otherMcp = JSON.stringify({ mcpServers: { demo_mcp: { command: 'demo-mcp', args: ['--stdio'] } } })
    for (const root of [other.sourcePath, other.pluginDir]) {
      writeFileSync(join(root, '.codex-plugin', 'plugin.json'), otherManifest)
      writeFileSync(join(root, '.mcp.json'), otherMcp)
    }
    const dataDir = join(runtimeHome(), 'data')
    const configPath = join(dataDir, 'config.json')
    const manualSkillRoot = join(runtimeHome(), 'manual-skills')
    const historyPath = join(dataDir, 'threads', 'synthetic-thread', 'thread.json')
    const evidencePath = join(dataDir, 'private', 'case-evidence', 'synthetic.json')
    mkdirSync(manualSkillRoot, { recursive: true })
    mkdirSync(join(historyPath, '..'), { recursive: true })
    mkdirSync(join(evidencePath, '..'), { recursive: true })
    writeFileSync(historyPath, '{"id":"synthetic-thread","status":"idle"}')
    writeFileSync(evidencePath, '{"case":"synthetic"}')
    writeFileSync(configPath, JSON.stringify({ capabilities: {
      mcp: { servers: { manual: { transport: 'stdio', command: 'manual-mcp', trustScope: 'user' } } },
      skills: { roots: [manualSkillRoot] },
      web: { fetchEnabled: false }
    } }))
    const historyBefore = readFileSync(historyPath)
    const evidenceBefore = readFileSync(evidencePath)
    const { syncGuiManagedAnalytixConfig } = await import('./analytix-process')
    const sync = (bound: boolean) => syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath: join(runtimeHome(), 'missing-mcp.json'),
      ...(bound ? { bundledFundsMaterialization: funds.hostMaterialization } : {})
    })
    const readConfig = (): any => JSON.parse(readFileSync(configPath, 'utf8'))

    expect(verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(), pluginDir: funds.pluginDir,
      installCacheRoots: [funds.installCacheRoot], expectedPluginName: 'analytix-fund-analysis',
      hostMaterialization: funds.hostMaterialization
    })).not.toBeNull()
    await sync(true)
    // This Funds package has fact tools disabled: its bound capability is the
    // private Go stage route, so config sync must never expose its MCP server.
    expect(readConfig().capabilities.mcp.servers.analytix_funds).toBeUndefined()
    const stale = readConfig()
    stale.capabilities.mcp.servers.analytix_funds = {
      transport: 'stdio', command: 'stale-funds-mcp', trustScope: 'user'
    }
    writeFileSync(configPath, JSON.stringify(stale))
    const configBytes = readFileSync(configPath)
    const sidecarDir = join(dataDir, '.cache')
    mkdirSync(sidecarDir, { recursive: true })
    const sidecarPath = join(sidecarDir, 'gui-plugin-mcp-server-ids.json')
    writeFileSync(sidecarPath, JSON.stringify({
      schemaVersion: 1, contract: 'analytix-plugin-managed-mcp-server-ids',
      configSha256: createHash('sha256').update(configBytes).digest('hex'),
      serverIds: ['analytix_funds']
    }))
    chmodSync(sidecarPath, 0o600)
    await sync(false)
    const ordinary = readConfig()
    expect(ordinary.capabilities.mcp.servers.analytix_funds).toBeUndefined()
    expect(ordinary.capabilities.mcp.servers.demo_mcp).toBeDefined()
    expect(ordinary.capabilities.mcp.servers.manual).toMatchObject({ command: 'manual-mcp' })
    expect(ordinary.capabilities.skills.roots).toContain(manualSkillRoot)
    expect(ordinary.capabilities.web.fetchEnabled).toBe(false)
    expect(readFileSync(historyPath).equals(historyBefore)).toBe(true)
    expect(readFileSync(evidencePath).equals(evidenceBefore)).toBe(true)

    await sync(true)
    const activated = readConfig()
    expect(activated.capabilities.mcp.servers.analytix_funds).toBeUndefined()
    expect(activated.capabilities.mcp.servers.demo_mcp).toEqual(ordinary.capabilities.mcp.servers.demo_mcp)
    expect(activated.capabilities.mcp.servers.manual).toEqual(ordinary.capabilities.mcp.servers.manual)
    expect(activated.capabilities.skills.roots).toContain(manualSkillRoot)
    expect(activated.capabilities.web.fetchEnabled).toBe(false)
    expect(readFileSync(historyPath).equals(historyBefore)).toBe(true)
    expect(readFileSync(evidencePath).equals(evidenceBefore)).toBe(true)
  })

  it('verifies a complete authorized local-remount binding', () => {
    const fixture = installFixture('local-remount')
    const verified = verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [fixture.installCacheRoot],
      expectedPluginName: 'demo-plugin',
      expectedVersion: '1.0.0'
    })
    expect(verified).toMatchObject({
      marker: { kind: 'local-remount' },
      pluginName: 'demo-plugin',
      version: '1.0.0',
      manifest: { name: 'demo-plugin', version: '1.0.0' }
    })
  })

  it('rejects a structurally valid self-authored hub-install marker without a Go receipt', () => {
    const fixture = installFixture('hub-install')
    expect(verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [fixture.installCacheRoot],
      expectedPluginName: 'demo-plugin',
      expectedVersion: '1.0.0',
      acceptedKinds: ['hub-install']
    })).toBeNull()
  })

  it('accepts only the current-run host binding for the exact signed funds tree', () => {
    const fixture = hostMaterializedFundsFixture()
    const verified = verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [fixture.installCacheRoot],
      expectedPluginName: 'analytix-fund-analysis',
      expectedVersion: '0.16.16',
      acceptedKinds: ['hub-install'],
      hostMaterialization: fixture.hostMaterialization
    })
    expect(verified).toMatchObject({
      marker: { kind: 'hub-install' },
      pluginName: 'analytix-fund-analysis',
      version: '0.16.16'
    })

    writeFileSync(join(fixture.pluginDir, 'mcp', 'server.mjs'), 'export const drift = true\n', 'utf8')
    expect(verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [fixture.installCacheRoot],
      expectedPluginName: 'analytix-fund-analysis',
      expectedVersion: '0.16.16',
      acceptedKinds: ['hub-install'],
      hostMaterialization: fixture.hostMaterialization
    })).toBeNull()
  })

  it('enforces the caller accepted-kind boundary', () => {
    const fixture = installFixture('local-remount')
    expect(verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [fixture.installCacheRoot],
      expectedPluginName: 'demo-plugin',
      acceptedKinds: ['hub-install']
    })).toBeNull()
  })

  it('rejects source, installed tree, manifest, and trusted-root mismatches', () => {
    const cases: Array<(fixture: ReturnType<typeof installFixture>) => void> = [
      (fixture) => writeFileSync(join(fixture.sourcePath, 'source-only.txt'), 'changed', 'utf8'),
      (fixture) => writeFileSync(join(fixture.pluginDir, 'installed-only.txt'), 'changed', 'utf8'),
      (fixture) => writeFileSync(join(fixture.pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
        name: 'other-plugin',
        version: '1.0.0'
      }), 'utf8')
    ]
    for (const mutate of cases) {
      const fixture = installFixture('hub-install')
      mutate(fixture)
      expect(verifyInstalledPluginBindingV1({
        runtimeHome: runtimeHome(),
        pluginDir: fixture.pluginDir,
        installCacheRoots: [fixture.installCacheRoot],
        expectedPluginName: 'demo-plugin'
      })).toBeNull()
      rmSync(runtimeHome(), { recursive: true, force: true })
      tempRoot = null
    }

    const fixture = installFixture('hub-install')
    expect(verifyInstalledPluginBindingV1({
      runtimeHome: runtimeHome(),
      pluginDir: fixture.pluginDir,
      installCacheRoots: [join(runtimeHome(), 'other-cache')],
      expectedPluginName: 'demo-plugin'
    })).toBeNull()
  })
})
