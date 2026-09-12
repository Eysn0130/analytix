import { existsSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { createServer, type Server } from 'node:http'
import { chmod, cp, mkdtemp, mkdir, readFile, readdir, realpath, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { computePluginSourceTreeIdentity } from '../plugin-source-integrity'
import {
  bindBundledFundsMaterializationToCurrentRuntimeV1,
  clearBundledFundsMaterializationCurrentRuntimeV1,
  type BundledFundsMaterializationBindingV1
} from '../runtime/bundled-funds-materialization'

import {
  generatedHubMarketplaceRoot,
  installHubAgentPlugin,
  readHubAgentSkillMarkdown,
  syncHubAgentMarketplace,
  uninstallHubAgentPlugin
} from './hub-agent-marketplace-service'

async function writeCachedHubPlugin(runtimeHome: string, versions = ['1.0.0']): Promise<void> {
  const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
  for (const version of versions) {
    const pluginDir = join(marketplaceRoot, 'plugins', 'demo-plugin', version)
    await mkdir(join(pluginDir, '.codex-plugin'), { recursive: true })
    await writeFile(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'demo-plugin',
      version,
      interface: {
        displayName: 'Demo Plugin',
        shortDescription: 'A cached plugin.'
      }
    }, null, 2))
  }
  await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
  await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
    name: 'analytix-hub',
    platform: 'mac-arm64',
    plugins: versions.map((version) => ({
      name: 'demo-plugin',
      version,
      source: {
        source: 'local',
        path: `plugins/demo-plugin/${version}`,
        sha256: 'a'.repeat(64)
      },
      interface: {
        displayName: 'Demo Plugin',
        shortDescription: 'A cached plugin.'
      }
    }))
  }, null, 2))
}

async function makeRuntimeHome(): Promise<string> {
  return realpath(await mkdtemp(join(tmpdir(), 'analytix-hub-marketplace-')))
}

// The cached catalog and receipts in this suite describe mac-arm64, regardless
// of the machine running these platform-independent parsing/authority tests.
function syncMacFixtureCache(runtimeHome: string) {
  return syncHubAgentMarketplace(runtimeHome, {
    mode: 'cache', process: { platform: 'darwin', arch: 'arm64' }
  })
}

async function snapshotDirectory(root: string): Promise<Record<string, string>> {
  const snapshot: Record<string, string> = {}
  const visit = async (directory: string, prefix: string): Promise<void> => {
    const entries = await readdir(directory, { withFileTypes: true })
    entries.sort((left, right) => left.name.localeCompare(right.name))
    for (const entry of entries) {
      const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name
      const absolutePath = join(directory, entry.name)
      if (entry.isDirectory()) {
        snapshot[`directory:${relativePath}`] = ''
        await visit(absolutePath, relativePath)
      } else if (entry.isFile()) {
        snapshot[`file:${relativePath}`] = (await readFile(absolutePath)).toString('base64')
      } else {
        snapshot[`other:${relativePath}`] = entry.isSymbolicLink() ? 'symlink' : 'non-regular'
      }
    }
  }
  await visit(root, '')
  return snapshot
}

async function startFixtureServer(routes: Record<string, Buffer | string>): Promise<{
  baseUrl: string
  requests: string[]
  close: () => Promise<void>
}> {
  const requests: string[] = []
  const server: Server = createServer((request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    requests.push(url.pathname)
    const body = routes[url.pathname]
    if (!body) {
      response.writeHead(404)
      response.end('not found')
      return
    }
    response.writeHead(200, {
      'content-type': url.pathname.endsWith('.json') ? 'application/json' : 'application/gzip'
    })
    response.end(body)
  })
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => resolve())
  })
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('fixture server did not bind')
  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    requests,
    close: () => new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
  }
}

describe('hub-agent-marketplace-service plugin install state', () => {
  afterEach(() => clearBundledFundsMaterializationCurrentRuntimeV1())

  it('observes only module, request, and fallback activity for an explicit marketplace action', async () => {
    vi.resetModules()
    const observation = await import('../hub-activity-observation')
    observation.resetHubActivityForTests()
    const marketplace = await import('./hub-agent-marketplace-service')
    const primary = await startFixtureServer({})
    const fallback = await startFixtureServer({
      '/marketplace.json': JSON.stringify({
        name: 'analytix-hub',
        platform: 'mac-arm64',
        plugins: []
      }),
      '/skills.json': JSON.stringify({ platform: 'mac-arm64', skills: [] }),
      '/install-policy.json': JSON.stringify({
        platform: 'mac-arm64',
        requiredPlugins: [],
        requiredSkills: []
      })
    })
    try {
      const result = await marketplace.syncHubAgentMarketplace(await makeRuntimeHome(), {
        mode: 'catalog',
        env: {
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL: `${primary.baseUrl}/marketplace.json`,
          ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_URL: `${primary.baseUrl}/skills.json`,
          ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_URL: `${primary.baseUrl}/install-policy.json`,
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_BASE_URL: fallback.baseUrl
        },
        process: { platform: 'darwin', arch: 'arm64' }
      })

      expect(result.ok).toBe(true)
      expect(observation.observeHubActivity()).toEqual({
        schemaVersion: 1,
        moduleLoad: 1,
        serviceInstance: 0,
        refreshTimer: 0,
        tokenRead: 0,
        request: 6,
        fallback: 3,
        anyActivity: true
      })
    } finally {
      await primary.close()
      await fallback.close()
    }
  })

  it('keeps manifest discovery inside the Analytix-owned runtime cache', async () => {
    const source = await readFile(new URL('./hub-agent-marketplace-service.ts', import.meta.url), 'utf8')
    expect(source).not.toContain("'.codex', 'plugins', 'cache'")
  })

  it('does not ship a default insecure Hub IP fallback', async () => {
    const source = await readFile(new URL('./hub-agent-marketplace-service.ts', import.meta.url), 'utf8')
    expect(source).not.toContain('8.148.152.181')
    expect(source).not.toContain("ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_INSECURE_TLS) || '1'")
  })

  it('contains no direct system archive extractor or package download implementation', async () => {
    const source = await readFile(new URL('./hub-agent-marketplace-service.ts', import.meta.url), 'utf8')
    expect(source).not.toContain('node:child_process')
    expect(source).not.toContain('execFile')
    expect(source).not.toContain("['-xzf'")
    expect(source).not.toContain('streamDownload')
    expect(source).not.toContain('extractTarGz')
  })

  it('never materializes a cached Hub package without a Go host receipt', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome)

    const before = await syncMacFixtureCache(runtimeHome)
    expect(before.ok && before.plugins[0]).toMatchObject({
      pluginName: 'demo-plugin',
      installed: false,
      requiredInstall: false
    })
    expect(before.ok && before.plugins[0]?.marketplaceEntry).toEqual({
      name: 'demo-plugin',
      version: '1.0.0',
      source: {
        source: 'local',
        path: 'plugins/demo-plugin/1.0.0',
        sha256: 'a'.repeat(64)
      },
      interface: {
        displayName: 'Demo Plugin',
        shortDescription: 'A cached plugin.'
      }
    })

    const beforeInstall = await snapshotDirectory(runtimeHome)
    await expect(installHubAgentPlugin(runtimeHome, {
      pluginName: 'demo-plugin',
      version: '1.0.0',
      upstreamMarketplaceName: 'analytix-hub'
    })).resolves.toEqual({
      ok: false,
      message: 'Remote Analytix Hub plugin materialization is unavailable until the Go archive authority is active.'
    })
    expect(await snapshotDirectory(runtimeHome)).toEqual(beforeInstall)
    const installedDir = join(runtimeHome, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '1.0.0')
    const markerPath = join(installedDir, '.analytix-hub-installed-plugin.json')
    expect(existsSync(installedDir)).toBe(false)
    expect(existsSync(markerPath)).toBe(false)

    const installed = await syncMacFixtureCache(runtimeHome)
    expect(installed.ok && installed.plugins[0]).toMatchObject({
      pluginName: 'demo-plugin',
      installed: false
    })
  })

  it('projects bundled Funds UI bytes only from the current runtime binding and admitted manifest', async () => {
    const runtimeHome = await makeRuntimeHome()
    const binding = await writeCurrentFundsBinding(runtimeHome, { pluginVersion: '0.16.17' })
    bindBundledFundsMaterializationToCurrentRuntimeV1(binding)
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    const catalogSentinels = [
      'catalog-version-conflict-u212b',
      'catalog-display-name-conflict-u212b',
      'catalog-short-description-conflict-u212b',
      'catalog-long-description-conflict-u212b',
      'catalog-category-conflict-u212b',
      'catalog-developer-conflict-u212b',
      'catalog-brand-color-conflict-u212b',
      'catalog-composer-icon-conflict-u212b',
      'catalog-logo-conflict-u212b',
      'catalog-default-prompt-conflict-u212b',
      'catalog-capability-conflict-u212b',
      'catalog-website-conflict-u212b',
      'catalog-privacy-conflict-u212b',
      'catalog-terms-conflict-u212b'
    ]
    await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
    await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
      name: 'analytix-hub',
      platform: 'mac-arm64',
      plugins: [{
        name: 'analytix-fund-analysis',
        version: catalogSentinels[0],
        source: {
          source: 'download',
          url: 'https://analytix.top/packages/funds.tgz',
          sha256: 'a'.repeat(64)
        },
        policy: { installation: 'REQUIRED', authentication: 'ON_USE' },
        upstream: { marketplaceName: 'analytix-upstream' },
        requiresCodexConnector: true,
        category: catalogSentinels[4],
        interface: {
          displayName: catalogSentinels[1],
          shortDescription: catalogSentinels[2],
          longDescription: catalogSentinels[3],
          category: catalogSentinels[4],
          developerName: catalogSentinels[5],
          brandColor: catalogSentinels[6],
          composerIcon: `https://catalog.invalid/${catalogSentinels[7]}.png`,
          logo: `https://catalog.invalid/${catalogSentinels[8]}.png`,
          defaultPrompt: [catalogSentinels[9]],
          capabilities: [catalogSentinels[10]],
          websiteURL: `https://catalog.invalid/${catalogSentinels[11]}`,
          privacyPolicyURL: `https://catalog.invalid/${catalogSentinels[12]}`,
          termsOfServiceURL: `https://catalog.invalid/${catalogSentinels[13]}`
        }
      }]
    }), 'utf8')
    const cacheRoot = join(runtimeHome, '.cache', 'analytix-hub-plugins')
    await mkdir(join(cacheRoot, 'skills'), { recursive: true })
    await writeFile(join(cacheRoot, 'skills', 'catalog.json'), JSON.stringify({
      platform: 'mac-arm64',
      skills: [{
        id: 'analytix-fund-analysis:quick-fact',
        skillName: 'quick-fact',
        pluginName: 'analytix-fund-analysis',
        version: '0.16.15',
        skillPath: 'skills/quick-fact/SKILL.md',
        sourceKind: 'plugin',
        requiredInstall: true
      }]
    }), 'utf8')
    await writeFile(join(cacheRoot, 'install-policy.json'), JSON.stringify({
      platform: 'mac-arm64',
      requiredPlugins: [{ pluginName: 'analytix-fund-analysis' }],
      requiredSkills: [{ skillName: 'quick-fact' }]
    }), 'utf8')

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok).toBe(true)
    if (!listed.ok) return
    const funds = listed.plugins.find((plugin) => plugin.pluginName === 'analytix-fund-analysis')
    expect(funds).toMatchObject({
      name: 'analytix-fund-analysis',
      pluginName: 'analytix-fund-analysis',
      version: '0.16.17',
      displayName: 'Analytix Fund Analysis',
      shortDescription: 'Receipt-bound Funds interface.',
      category: 'Productivity',
      developerName: 'Analytix',
      brandColor: '#2563EB',
      requiresCodexConnector: true,
      mcpServerIds: ['analytix_funds'],
      installed: true
    })
    expect(funds?.iconDataUrl).toBe('data:image/png;base64,Y3VycmVudCBmdW5kcyBpY29uCg==')
    expect(funds?.iconUrl).toBeUndefined()
    expect(funds?.marketplaceEntry).toEqual({
      name: 'analytix-fund-analysis',
      version: '0.16.17',
      source: {
        source: 'download',
        url: 'https://analytix.top/packages/funds.tgz',
        sha256: 'a'.repeat(64)
      },
      policy: { installation: 'REQUIRED', authentication: 'ON_USE' },
      upstream: { marketplaceName: 'analytix-upstream' },
      requiresCodexConnector: true,
      category: 'Productivity',
      interface: {
        displayName: 'Analytix Fund Analysis',
        shortDescription: 'Receipt-bound Funds interface.',
        longDescription: 'Current admitted Funds interface.',
        developerName: 'Analytix',
        category: 'Productivity',
        capabilities: ['Interactive', 'Read'],
        websiteURL: 'https://analytix.top',
        privacyPolicyURL: 'https://analytix.top/privacy',
        termsOfServiceURL: 'https://analytix.top/terms',
        defaultPrompt: ['Use the current admitted Funds capability.'],
        brandColor: '#2563EB',
        composerIcon: './assets/icon.png',
        logo: './assets/logo.png'
      }
    })
    const serializedFunds = JSON.stringify(funds)
    for (const sentinel of catalogSentinels) expect(serializedFunds).not.toContain(sentinel)
    expect(listed.skills).toContainEqual(expect.objectContaining({
      pluginName: 'analytix-fund-analysis',
      skillName: 'quick-fact',
      version: '0.16.17',
      installed: true
    }))
    await expect(readHubAgentSkillMarkdown(runtimeHome, {
      id: 'analytix-fund-analysis:quick-fact',
      skillName: 'quick-fact',
      pluginName: 'analytix-fund-analysis',
      version: '0.16.17',
      skillPath: 'skills/quick-fact/SKILL.md'
    })).resolves.toMatchObject({ ok: true, source: 'plugin-cache' })
    await expect(readHubAgentSkillMarkdown(runtimeHome, {
      id: 'analytix-fund-analysis:quick-fact',
      skillName: 'quick-fact',
      pluginName: 'analytix-fund-analysis',
      version: '0.16.15',
      skillPath: 'skills/quick-fact/SKILL.md'
    })).resolves.toMatchObject({ ok: false })
  })

  it('uses a safe Funds UI fallback when the current admitted manifest interface is not an object', async () => {
    const runtimeHome = await makeRuntimeHome()
    const binding = await writeCurrentFundsBinding(runtimeHome, { manifestInterface: null })
    bindBundledFundsMaterializationToCurrentRuntimeV1(binding)
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    const catalogSentinel = 'catalog-invalid-interface-fallback-conflict-u212b'
    await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
    await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
      name: 'analytix-hub',
      platform: 'mac-arm64',
      plugins: [{
        name: 'analytix-fund-analysis',
        version: catalogSentinel,
        source: { source: 'download', url: 'https://analytix.top/packages/funds.tgz' },
        policy: { installation: 'REQUIRED' },
        interface: {
          displayName: catalogSentinel,
          shortDescription: catalogSentinel,
          category: catalogSentinel,
          developerName: catalogSentinel,
          brandColor: catalogSentinel,
          composerIcon: `https://catalog.invalid/${catalogSentinel}.png`,
          capabilities: [catalogSentinel],
          defaultPrompt: [catalogSentinel]
        }
      }]
    }), 'utf8')

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok).toBe(true)
    if (!listed.ok) return
    const funds = listed.plugins.find((plugin) => plugin.pluginName === 'analytix-fund-analysis')
    expect(funds).toMatchObject({
      name: 'analytix-fund-analysis',
      pluginName: 'analytix-fund-analysis',
      version: '0.16.16',
      displayName: 'analytix-fund-analysis',
      shortDescription: '',
      category: 'Other',
      developerName: '',
      manifest: {
        name: 'analytix-fund-analysis',
        version: '0.16.16',
        interface: null
      },
      marketplaceEntry: {
        name: 'analytix-fund-analysis',
        version: '0.16.16',
        source: { source: 'download', url: 'https://analytix.top/packages/funds.tgz' },
        policy: { installation: 'REQUIRED' },
        category: 'Other',
        interface: {}
      },
      installed: true
    })
    expect(funds?.iconDataUrl).toBeUndefined()
    expect(funds?.iconUrl).toBeUndefined()
    expect(funds?.brandColor).toBeUndefined()
    expect(JSON.stringify(funds)).not.toContain(catalogSentinel)
  })

  it('withholds Funds manifest UI when the current binding no longer verifies its active tree', async () => {
    const runtimeHome = await makeRuntimeHome()
    const binding = await writeCurrentFundsBinding(runtimeHome)
    bindBundledFundsMaterializationToCurrentRuntimeV1(binding)
    const tamperedManifestSentinel = 'tampered-active-manifest-conflict-u212b'
    await writeFile(join(binding.activePluginRoot, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'analytix-fund-analysis',
      version: '0.16.16',
      interface: {
        displayName: tamperedManifestSentinel,
        shortDescription: tamperedManifestSentinel,
        category: tamperedManifestSentinel
      }
    }), 'utf8')
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    const catalogSentinel = 'catalog-tree-verification-fallback-conflict-u212b'
    await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
    await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
      name: 'analytix-hub',
      platform: 'mac-arm64',
      plugins: [{
        name: 'analytix-fund-analysis',
        version: catalogSentinel,
        source: { source: 'download', url: 'https://analytix.top/packages/funds.tgz' },
        policy: { installation: 'REQUIRED' },
        interface: {
          displayName: catalogSentinel,
          shortDescription: catalogSentinel,
          category: catalogSentinel
        }
      }]
    }), 'utf8')

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok).toBe(true)
    if (!listed.ok) return
    const funds = listed.plugins.find((plugin) => plugin.pluginName === 'analytix-fund-analysis')
    expect(funds).toMatchObject({
      version: '0.16.16',
      displayName: 'analytix-fund-analysis',
      shortDescription: '',
      category: 'Other',
      developerName: '',
      manifest: {},
      marketplaceEntry: {
        name: 'analytix-fund-analysis',
        version: '0.16.16',
        source: { source: 'download', url: 'https://analytix.top/packages/funds.tgz' },
        policy: { installation: 'REQUIRED' },
        category: 'Other',
        interface: {}
      },
      mcpServerIds: [],
      installed: false
    })
    const serializedFunds = JSON.stringify(funds)
    expect(serializedFunds).not.toContain(tamperedManifestSentinel)
    expect(serializedFunds).not.toContain(catalogSentinel)
  })

  it('lists MCP server ids declared by cached Hub plugin packages', async () => {
    const runtimeHome = await makeRuntimeHome()
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    const pluginDir = join(marketplaceRoot, 'plugins', 'playwright-mcp', '0.0.76')
    await mkdir(join(pluginDir, '.codex-plugin'), { recursive: true })
    await writeFile(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'playwright-mcp',
      version: '0.0.76',
      mcpServers: './.mcp.json',
      interface: {
        displayName: 'Playwright MCP',
        shortDescription: 'Control browsers through Playwright.'
      }
    }, null, 2), 'utf8')
    await writeFile(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        playwright: {
          command: 'npx',
          args: ['-y', '@playwright/mcp@0.0.76']
        }
      }
    }, null, 2), 'utf8')
    await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
    await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
      name: 'analytix-hub',
      platform: 'mac-arm64',
      plugins: [{
        name: 'playwright-mcp',
        version: '0.0.76',
        source: {
          source: 'local',
          path: 'plugins/playwright-mcp/0.0.76',
          sha256: 'abc123'
        },
        interface: {
          displayName: 'Playwright MCP',
          shortDescription: 'Control browsers through Playwright.'
        }
      }]
    }, null, 2), 'utf8')

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok && listed.plugins[0]).toMatchObject({
      pluginName: 'playwright-mcp',
      mcpServerIds: ['playwright']
    })
  })

  it('keeps plugin apps and resources visible without registering them as MCP servers', async () => {
    const runtimeHome = await makeRuntimeHome()
    const marketplaceRoot = generatedHubMarketplaceRoot(runtimeHome)
    const pluginDir = join(marketplaceRoot, 'plugins', 'browser-app', '1.0.0')
    await mkdir(join(pluginDir, '.codex-plugin'), { recursive: true })
    await writeFile(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'browser-app',
      version: '1.0.0',
      apps: {
        browser: {
          resources: ['browser://tabs'],
          connector: 'browser'
        }
      },
      interface: {
        displayName: 'Browser App',
        shortDescription: 'Connector metadata only.'
      }
    }, null, 2), 'utf8')
    await mkdir(join(marketplaceRoot, '.agents', 'plugins'), { recursive: true })
    await writeFile(join(marketplaceRoot, '.agents', 'plugins', 'marketplace.json'), JSON.stringify({
      name: 'analytix-hub',
      platform: 'mac-arm64',
      plugins: [{
        name: 'browser-app',
        version: '1.0.0',
        source: {
          source: 'local',
          path: 'plugins/browser-app/1.0.0',
          sha256: 'abc123'
        },
        interface: {
          displayName: 'Browser App',
          shortDescription: 'Connector metadata only.'
        }
      }]
    }, null, 2), 'utf8')

    const listed = await syncMacFixtureCache(runtimeHome)

    expect(listed.ok && listed.plugins[0]).toMatchObject({
      pluginName: 'browser-app',
      mcpServerIds: [],
      requiresCodexConnector: true,
      manifest: {
        apps: {
          browser: {
            resources: ['browser://tabs'],
            connector: 'browser'
          }
        }
      }
    })
  })

  it('refreshes catalog metadata without downloading or materializing remote plugin packages', async () => {
    const runtimeHome = await makeRuntimeHome()
    const server = await startFixtureServer({
      '/packages/demo-plugin-2.0.0.tar.gz': 'untrusted archive bytes',
      '/marketplace.json': JSON.stringify({
        name: 'analytix-hub',
        platform: 'mac-arm64',
        plugins: [{
          name: 'demo-plugin',
          version: '2.0.0',
          source: {
            source: 'download',
            url: 'http://fixture.invalid/packages/demo-plugin-2.0.0.tar.gz',
            sha256: 'a'.repeat(64)
          },
          interface: {
            displayName: 'Demo Plugin',
            shortDescription: 'Catalog metadata only.'
          }
        }]
      }),
      '/skills.json': JSON.stringify({ platform: 'mac-arm64', skills: [] }),
      '/install-policy.json': JSON.stringify({ platform: 'mac-arm64', requiredPlugins: [], requiredSkills: [] })
    })
    try {
      const result = await syncHubAgentMarketplace(runtimeHome, {
        mode: 'catalog',
        env: {
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL: `${server.baseUrl}/marketplace.json`,
          ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_URL: `${server.baseUrl}/skills.json`,
          ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_URL: `${server.baseUrl}/install-policy.json`
        },
        process: { platform: 'darwin', arch: 'arm64' }
      })

      if (!result.ok) throw new Error(result.message)
      expect(result.errors).toEqual([])
      expect(result.plugins[0]).toMatchObject({
        pluginName: 'demo-plugin',
        version: '2.0.0',
        displayName: 'Demo Plugin',
        installed: false,
        shortDescription: 'Catalog metadata only.'
      })
      expect([...server.requests].sort()).toEqual([
        '/install-policy.json',
        '/marketplace.json',
        '/skills.json'
      ])
      expect(server.requests).not.toContain('/packages/demo-plugin-2.0.0.tar.gz')
      expect(existsSync(join(generatedHubMarketplaceRoot(runtimeHome), 'plugins'))).toBe(false)

      const beforeInstall = await snapshotDirectory(runtimeHome)
      await expect(installHubAgentPlugin(runtimeHome, {
        pluginName: 'demo-plugin',
        version: '2.0.0',
        upstreamMarketplaceName: 'analytix-hub'
      })).resolves.toEqual({
        ok: false,
        message: 'Remote Analytix Hub plugin materialization is unavailable until the Go archive authority is active.'
      })
      expect(server.requests).toHaveLength(3)
      expect(await snapshotDirectory(runtimeHome)).toEqual(beforeInstall)
    } finally {
      await server.close()
    }
  })

  it('projects hostile remote catalog failures before returning sync diagnostics', async () => {
    const runtimeHome = await makeRuntimeHome()
    const marketplaceSentinel = 'customer-pii-13900000025'
    const skillsSentinel = 'customer-pii-13900000026'
    const installPolicySentinel = 'customer-pii-13900000027'
    const server = await startFixtureServer({
      '/marketplace.json': `/private/${marketplaceSentinel} raw-marketplace-body`,
      '/skills.json': `/private/${skillsSentinel} raw-skills-body`,
      '/install-policy.json': `/private/${installPolicySentinel} raw-install-policy-body`
    })
    try {
      const result = await syncHubAgentMarketplace(runtimeHome, {
        mode: 'catalog',
        env: {
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL: `${server.baseUrl}/marketplace.json`,
          ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_URL: `${server.baseUrl}/skills.json`,
          ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_URL: `${server.baseUrl}/install-policy.json`
        },
        process: { platform: 'darwin', arch: 'arm64' }
      })

      expect(result.ok).toBe(true)
      if (!result.ok) return
      expect(result.errors).toEqual([
        'skills: unavailable',
        'install-policy: unavailable',
        'marketplace: unavailable'
      ])
      expect(JSON.stringify(result)).not.toMatch(
        new RegExp([marketplaceSentinel, skillsSentinel, installPolicySentinel].join('|'))
      )
      expect([...server.requests].sort()).toEqual([
        '/install-policy.json',
        '/marketplace.json',
        '/skills.json'
      ])
    } finally {
      await server.close()
    }
  })

  it('does not locally uninstall plugins required by the admin policy', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome)
    await mkdir(join(runtimeHome, '.cache', 'analytix-hub-plugins'), { recursive: true })
    await writeFile(join(runtimeHome, '.cache', 'analytix-hub-plugins', 'install-policy.json'), JSON.stringify({
      platform: 'mac-arm64',
      requiredPlugins: [{
        marketplaceName: 'analytix-hub',
        pluginName: 'demo-plugin',
        version: '1.0.0'
      }]
    }, null, 2))

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok && listed.plugins[0]).toMatchObject({
      pluginName: 'demo-plugin',
      requiredInstall: true,
      installed: false
    })

    const result = await uninstallHubAgentPlugin(runtimeHome, {
      pluginName: 'demo-plugin',
      version: '1.0.0',
      upstreamMarketplaceName: 'analytix-hub'
    })
    expect(result).toMatchObject({ ok: false, managed: true })
  })

  it('ignores admin install policies for other marketplaces', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome)
    await mkdir(join(runtimeHome, '.cache', 'analytix-hub-plugins'), { recursive: true })
    await writeFile(join(runtimeHome, '.cache', 'analytix-hub-plugins', 'install-policy.json'), JSON.stringify({
      platform: 'mac-arm64',
      requiredPlugins: [{
        marketplaceName: 'other-marketplace',
        pluginName: 'demo-plugin',
        version: '1.0.0'
      }]
    }, null, 2))

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok && listed.plugins[0]).toMatchObject({
      pluginName: 'demo-plugin',
      requiredInstall: false
    })

    const result = await uninstallHubAgentPlugin(runtimeHome, {
      pluginName: 'demo-plugin',
      upstreamMarketplaceName: 'analytix-hub'
    })
    expect(result).toMatchObject({ ok: true })
  })

  it('blocks every cached generation before any install mutation', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome, ['1.0.0', '2.0.0'])
    const beforeInstall = await snapshotDirectory(runtimeHome)

    for (const version of ['1.0.0', '2.0.0']) {
      await expect(installHubAgentPlugin(runtimeHome, {
        pluginName: 'demo-plugin',
        version,
        upstreamMarketplaceName: 'analytix-hub'
      })).resolves.toEqual({
        ok: false,
        message: 'Remote Analytix Hub plugin materialization is unavailable until the Go archive authority is active.'
      })
    }
    expect(await snapshotDirectory(runtimeHome)).toEqual(beforeInstall)

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok && listed.plugins.some((plugin) => plugin.installed)).toBe(false)
  })

  it('shows a verified local remount as installed but refuses ordinary uninstall', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome)
    const packageSha256 = 'c'.repeat(64)
    const generation = `1.0.0-local-${packageSha256}`
    const marketplacePluginRoot = join(generatedHubMarketplaceRoot(runtimeHome), 'plugins', 'demo-plugin')
    const sourcePath = join(marketplacePluginRoot, generation)
    await cp(join(marketplacePluginRoot, '1.0.0'), sourcePath, { recursive: true })
    const installedDir = join(runtimeHome, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '1.0.0')
    await mkdir(join(installedDir, '..'), { recursive: true })
    await cp(sourcePath, installedDir, { recursive: true })
    const markerPath = join(installedDir, '.analytix-hub-installed-plugin.json')
    await writeFile(markerPath, JSON.stringify({
      managedBy: 'analytix-hub',
      marketplaceName: 'analytix-hub',
      pluginName: 'demo-plugin',
      version: '1.0.0',
      packageSha256,
      sourcePath,
      source: { source: 'local', path: `./plugins/demo-plugin/${generation}` },
      transactionVersion: 'RuntimeCacheRemountTransactionV1',
      commitOrder: 'marketplace_pointer_last'
    }), 'utf8')
    await chmod(markerPath, 0o600)

    const listed = await syncMacFixtureCache(runtimeHome)
    expect(listed.ok && listed.plugins[0]).toMatchObject({
      pluginName: 'demo-plugin',
      installed: true
    })
    await expect(uninstallHubAgentPlugin(runtimeHome, {
      pluginName: 'demo-plugin',
      version: '1.0.0',
      upstreamMarketplaceName: 'analytix-hub'
    })).resolves.toEqual({
      ok: false,
      message: 'Analytix Hub plugin "demo-plugin" is an authorized local remount and cannot be removed by ordinary uninstall.'
    })
    expect(existsSync(installedDir)).toBe(true)
  })

  it('blocks full remote materialization before network, process, archive, or generation effects', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome, ['1.0.0'])

    const cachedGeneration = join(generatedHubMarketplaceRoot(runtimeHome), 'plugins', 'demo-plugin', '1.0.0')
    const installedGeneration = join(runtimeHome, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '1.0.0')
    await writeFile(join(cachedGeneration, 'preserve.txt'), 'cached generation must remain', 'utf8')
    const before = await snapshotDirectory(runtimeHome)

    const server = await startFixtureServer({
      '/packages/demo-plugin-2.0.0.tar.gz': 'hostile archive bytes',
      '/marketplace.json': JSON.stringify({
        name: 'analytix-hub',
        platform: 'mac-arm64',
        plugins: [{
          name: 'demo-plugin',
          version: '2.0.0',
          source: {
            source: 'download',
            url: 'http://fixture.invalid/packages/demo-plugin-2.0.0.tar.gz',
            sha256: 'b'.repeat(64)
          },
          interface: {
            displayName: 'Demo Plugin',
            shortDescription: 'Version 2.0.0.'
          }
        }]
      }),
      '/skills.json': JSON.stringify({ platform: 'mac-arm64', skills: [] }),
      '/install-policy.json': JSON.stringify({ platform: 'mac-arm64', requiredPlugins: [], requiredSkills: [] })
    })
    try {
      const options = {
        mode: 'full',
        env: {
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL: `${server.baseUrl}/marketplace.json`,
          ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_URL: `${server.baseUrl}/skills.json`,
          ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_URL: `${server.baseUrl}/install-policy.json`,
          ANALYTIX_AGENT_PLUGIN_MARKETPLACE_PACKAGE_BASE_URL: server.baseUrl
        },
        process: { platform: 'darwin', arch: 'arm64' }
      } as const

      const explicitFull = await syncHubAgentMarketplace(runtimeHome, options)
      const defaultFull = await syncHubAgentMarketplace(runtimeHome, {
        ...options,
        mode: undefined
      })

      for (const result of [explicitFull, defaultFull]) {
        expect(result).toEqual({
          ok: false,
          message: 'Remote Analytix Hub plugin materialization is unavailable until the Go archive authority is active.',
          platform: 'mac-arm64',
          errors: ['remote_plugin_materialization_requires_go_archive_authority']
        })
      }
      expect(server.requests).toEqual([])
      expect(await snapshotDirectory(runtimeHome)).toEqual(before)
      expect(await readFile(join(cachedGeneration, 'preserve.txt'), 'utf8')).toBe('cached generation must remain')
      expect(existsSync(installedGeneration)).toBe(false)
      expect(existsSync(join(runtimeHome, '.cache', 'analytix-hub-plugins', 'archives'))).toBe(false)
      expect(existsSync(`${cachedGeneration}.extracting`)).toBe(false)
      expect(existsSync(join(runtimeHome, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '2.0.0'))).toBe(false)
    } finally {
      await server.close()
    }
  })

  it('reads SKILL.md content from a cached Hub plugin package without installing it', async () => {
    const runtimeHome = await makeRuntimeHome()
    await writeCachedHubPlugin(runtimeHome)
    const pluginDir = join(generatedHubMarketplaceRoot(runtimeHome), 'plugins', 'demo-plugin', '1.0.0')
    await mkdir(join(pluginDir, 'skills', 'documents'), { recursive: true })
    await writeFile(join(pluginDir, 'skills', 'documents', 'SKILL.md'), [
      '---',
      'name: documents',
      '---',
      '',
      '# Documents',
      '',
      'Use this skill to create document files.'
    ].join('\n'), 'utf8')

    const result = await readHubAgentSkillMarkdown(runtimeHome, {
      skillName: 'documents',
      displayName: 'Documents',
      pluginName: 'demo-plugin',
      version: '1.0.0',
      upstreamMarketplaceName: 'analytix-hub',
      skillPath: 'skills/documents/SKILL.md',
      sourceKind: 'plugin'
    })

    expect(result).toMatchObject({
      ok: true,
      source: 'plugin-cache',
      content: expect.stringContaining('Use this skill to create document files.')
    })
  })
})

async function writeCurrentFundsBinding(
  runtimeHome: string,
  options: { manifestInterface?: unknown; pluginVersion?: string } = {}
): Promise<BundledFundsMaterializationBindingV1> {
  const pluginVersion = options.pluginVersion ?? '0.16.16'
  const activeRelativePath = `plugins/cache/analytix-hub/analytix-fund-analysis/${pluginVersion}`
  const activePluginRoot = join(
    runtimeHome,
    'plugins',
    'cache',
    'analytix-hub',
    'analytix-fund-analysis',
    pluginVersion
  )
  const manifestPath = join(activePluginRoot, '.codex-plugin', 'plugin.json')
  const entrypointPath = join(activePluginRoot, 'mcp', 'server.mjs')
  const skillPath = join(activePluginRoot, 'skills', 'quick-fact', 'SKILL.md')
  await mkdir(join(activePluginRoot, '.codex-plugin'), { recursive: true })
  await mkdir(join(activePluginRoot, 'mcp'), { recursive: true })
  await mkdir(join(activePluginRoot, 'skills', 'quick-fact'), { recursive: true })
  await mkdir(join(activePluginRoot, 'assets'), { recursive: true })
  const manifestInterface = Object.prototype.hasOwnProperty.call(options, 'manifestInterface')
    ? options.manifestInterface
    : {
        displayName: 'Analytix Fund Analysis',
        shortDescription: 'Receipt-bound Funds interface.',
        longDescription: 'Current admitted Funds interface.',
        developerName: 'Analytix',
        category: 'Productivity',
        capabilities: ['Interactive', 'Read'],
        websiteURL: 'https://analytix.top',
        privacyPolicyURL: 'https://analytix.top/privacy',
        termsOfServiceURL: 'https://analytix.top/terms',
        defaultPrompt: ['Use the current admitted Funds capability.'],
        brandColor: '#2563EB',
        composerIcon: './assets/icon.png',
        logo: './assets/logo.png'
      }
  const manifestBytes = Buffer.from(JSON.stringify({
    name: 'analytix-fund-analysis',
    version: pluginVersion,
    interface: manifestInterface,
    mcpServers: './.mcp.json'
  }))
  const entrypointBytes = Buffer.from('export const factsEnabled = false\n')
  await writeFile(manifestPath, manifestBytes)
  await writeFile(entrypointPath, entrypointBytes)
  await writeFile(join(activePluginRoot, '.mcp.json'), JSON.stringify({
    mcpServers: {
      analytix_funds: {
        command: 'node',
        args: ['./mcp/server.mjs']
      }
    }
  }), 'utf8')
  await writeFile(join(activePluginRoot, 'assets', 'icon.png'), 'current funds icon\n', 'utf8')
  await writeFile(join(activePluginRoot, 'assets', 'logo.png'), 'current funds logo\n', 'utf8')
  await writeFile(skillPath, [
    '---', 'name: quick-fact', 'description: receipt-bound current skill', '---', '', 'current'
  ].join('\n'), 'utf8')
  const tree = computePluginSourceTreeIdentity(activePluginRoot)
  const packageAuthoritySha256 = '1'.repeat(64)
  await writeFile(join(activePluginRoot, '.analytix-hub-installed-plugin.json'), JSON.stringify({
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    pluginName: 'analytix-fund-analysis',
    version: pluginVersion,
    packageSha256: packageAuthoritySha256,
    sourcePath: activePluginRoot,
    sourceTreeSha256: tree.treeSha256,
    sourceTreeFileCount: tree.fileCount,
    installType: 'user'
  }), { mode: 0o600 })
  const sha256 = (body: Buffer): string => createHash('sha256').update(body).digest('hex')
  const receiptId = '2'.repeat(64)
  return {
    schemaVersion: 1,
    purpose: 'analytix.bundled-funds-materialization-ready/v1',
    invocationId: '3'.repeat(64),
    configurationBindingDigest: '4'.repeat(64),
    packageAuthority: {
      fileSha256: packageAuthoritySha256,
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
    pluginName: 'analytix-fund-analysis',
    pluginVersion,
    activePluginRoot,
    receipt: {
      schemaVersion: 1,
      purpose: 'analytix.bundled-plugin-materialization-receipt/v1',
      receiptId,
      intentId: '7'.repeat(64),
      packageAuthoritySha256,
      target: { platform: 'darwin', arch: 'arm64' },
      pluginName: 'analytix-fund-analysis',
      pluginVersion,
      generationId: '8'.repeat(64),
      activeRelativePath,
      sourceTreeSha256: tree.treeSha256,
      sourceTreeFileCount: tree.fileCount,
      manifestSha256: sha256(manifestBytes),
      entrypointSha256: sha256(entrypointBytes),
      factToolsEnabled: false,
      issuedAt: '2026-07-23T18:00:00Z',
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: '9'.repeat(64),
      authorityPublicKey: 'AA',
      authoritySignature: 'AA'
    },
    index: {
      schemaVersion: 1,
      purpose: 'analytix.bundled-plugin-materialization-index/v1',
      indexDigest: 'a'.repeat(64),
      pluginName: 'analytix-fund-analysis',
      pluginVersion,
      generationId: '8'.repeat(64),
      activeRelativePath,
      receiptId,
      receiptSha256: 'b'.repeat(64),
      intentId: '7'.repeat(64),
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
}
