import {
  chmodSync,
  linkSync,
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  parseInstalledPluginMarkerV1,
  readInstalledPluginMarkerV1
} from './plugin-install-marker'

const SHA = 'a'.repeat(64)
let tempRoot: string | null = null

function root(): string {
  if (!tempRoot) tempRoot = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-plugin-marker-')))
  return tempRoot
}

function hubMarker(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    pluginName: 'demo-plugin',
    version: '1.0.0',
    packageSha256: SHA,
    sourcePath: join(root(), 'marketplaces', 'analytix-hub', 'plugins', 'demo-plugin', '1.0.0'),
    sourceTreeSha256: 'b'.repeat(64),
    sourceTreeFileCount: 3,
    installType: 'user',
    ...overrides
  }
}

function localRemountMarker(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const generation = `1.0.0-local-${SHA}`
  return {
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    pluginName: 'demo-plugin',
    version: '1.0.0',
    packageSha256: SHA,
    sourcePath: join(root(), 'marketplaces', 'analytix-hub', 'plugins', 'demo-plugin', generation),
    source: { source: 'local', path: `./plugins/demo-plugin/${generation}` },
    transactionVersion: 'RuntimeCacheRemountTransactionV1',
    commitOrder: 'marketplace_pointer_last',
    ...overrides
  }
}

function bytes(value: unknown): Buffer {
  return Buffer.from(JSON.stringify(value), 'utf8')
}

afterEach(() => {
  if (tempRoot) rmSync(tempRoot, { recursive: true, force: true })
  tempRoot = null
})

describe('plugin install marker V1', () => {
  it('accepts only the exact hub-install and local-remount unions', () => {
    expect(parseInstalledPluginMarkerV1(bytes(hubMarker()))?.kind).toBe('hub-install')
    expect(parseInstalledPluginMarkerV1(bytes(hubMarker({ upstreamMarketplaceName: 'approved-upstream' })))?.kind)
      .toBe('hub-install')
    expect(parseInstalledPluginMarkerV1(bytes(localRemountMarker()))?.kind).toBe('local-remount')
  })

  it.each([
    ['unknown hub key', () => hubMarker({ unexpected: true })],
    ['empty upstream', () => hubMarker({ upstreamMarketplaceName: '' })],
    ['trimmed upstream alias', () => hubMarker({ upstreamMarketplaceName: ' upstream ' })],
    ['short package digest', () => hubMarker({ packageSha256: 'abc123' })],
    ['uppercase package digest', () => hubMarker({ packageSha256: 'A'.repeat(64) })],
    ['zero source file count', () => hubMarker({ sourceTreeFileCount: 0 })],
    ['hybrid marker', () => hubMarker({ source: { source: 'local', path: './plugins/demo-plugin/x' } })],
    ['wrong transaction version', () => localRemountMarker({ transactionVersion: 'RuntimeCacheRemountTransactionV2' })],
    ['wrong commit order', () => localRemountMarker({ commitOrder: 'plugin_first' })],
    ['wrong local source path', () => localRemountMarker({ source: { source: 'local', path: './plugins/other/x' } })],
    ['unknown local source key', () => localRemountMarker({ source: { source: 'local', path: './plugins/demo-plugin/x', extra: true } })]
  ])('rejects %s', (_label, build) => {
    expect(parseInstalledPluginMarkerV1(bytes(build()))).toBeNull()
  })

  it('rejects duplicate JSON keys before constructing marker authority', () => {
    const sourcePath = JSON.stringify(join(root(), 'marketplaces', 'analytix-hub', 'plugins', 'demo-plugin', '1.0.0'))
    const duplicate = Buffer.from(
      `{"managedBy":"analytix-hub","managedBy":"attacker","marketplaceName":"analytix-hub",` +
      `"pluginName":"demo-plugin","version":"1.0.0","packageSha256":"${SHA}",` +
      `"sourcePath":${sourcePath},"sourceTreeSha256":"${'b'.repeat(64)}",` +
      '"sourceTreeFileCount":3,"installType":"user"}',
      'utf8'
    )
    expect(parseInstalledPluginMarkerV1(duplicate)).toBeNull()
  })

  it('reads only a stable, private, single-link regular marker file', () => {
    const directory = join(root(), 'installed')
    mkdirSync(directory, { recursive: true })
    const markerPath = join(directory, '.analytix-hub-installed-plugin.json')
    writeFileSync(markerPath, bytes(hubMarker()))
    chmodSync(markerPath, 0o600)
    expect(readInstalledPluginMarkerV1(markerPath)?.kind).toBe('hub-install')

    if (process.platform !== 'win32') {
      chmodSync(markerPath, 0o644)
      expect(readInstalledPluginMarkerV1(markerPath)).toBeNull()
      chmodSync(markerPath, 0o600)
    }

    const hardLinkPath = join(directory, 'marker-hard-link.json')
    linkSync(markerPath, hardLinkPath)
    expect(readInstalledPluginMarkerV1(markerPath)).toBeNull()
    rmSync(hardLinkPath)

    const symlinkPath = join(directory, 'marker-symlink.json')
    symlinkSync(markerPath, symlinkPath)
    expect(readInstalledPluginMarkerV1(symlinkPath)).toBeNull()
  })
})
