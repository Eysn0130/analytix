import { createHash } from 'node:crypto'
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'

const contract = require('../../scripts/rust-native-toolchain-contract.cjs')

const temporaryRoots: string[] = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

describe('Rust native build toolchain authority', () => {
  it('rejects duplicate keys and every field outside the closed lock schema', () => {
    const valid = readFileSync(contract.lockPath)
    const duplicate = Buffer.from(valid.toString('utf8').replace(
      '"schemaVersion": 1,',
      '"schemaVersion": 1, "schemaVersion": 1,'
    ))
    expect(() => contract.parseRustNativeToolchainLock(duplicate)).toThrow(/duplicate key/)

    for (const mutate of [
      (value: Record<string, unknown>) => { value.unexpected = true },
      (value: Record<string, any>) => { value.hosts['unsupported-host'] = value.hosts['darwin-arm64'] },
      (value: Record<string, any>) => { value.hosts['darwin-arm64'].unexpected = true },
      (value: Record<string, any>) => { value.hosts['darwin-arm64'].tools.unexpected = '0'.repeat(64) }
    ]) {
      const value = JSON.parse(valid.toString('utf8'))
      mutate(value)
      expect(() => contract.parseRustNativeToolchainLock(Buffer.from(JSON.stringify(value)))).toThrow(/Unexpected/)
    }
  })

  it('resolves only a complete pre-authorized toolchain tree and platform tool set', () => {
    const fixture = createFixture()
    const resolved = contract.resolvePinnedRustToolchain(fixture.options)

    expect(resolved).toEqual(expect.objectContaining({
      key: 'darwin-arm64',
      root: fixture.toolchainRoot,
      cargo: fixture.cargo,
      rustc: fixture.rustc,
      developerDirectory: fixture.developerDirectory,
      sdkRoot: fixture.sdkRoot,
      tree: fixture.tree
    }))
    expect(resolved.tools).toEqual(fixture.toolPaths)
    expect(fixture.execCalls).toEqual([
      { file: fixture.cargo, args: ['--version'] },
      { file: fixture.rustc, args: ['--version'] }
    ])
    expect(resolved.environment.PATH).not.toContain('/attacker')
    expect(resolved.environment.RUSTC).toBe(fixture.rustc)
    expect(resolved.environment.SDKROOT).toBe(fixture.sdkRoot)
  })

  it('fails closed on a changed byte, extra file, unknown host, or unpinned platform tool', () => {
    for (const mutate of [
      (fixture: ReturnType<typeof createFixture>) => writeFileSync(fixture.rustc, 'changed-rustc'),
      (fixture: ReturnType<typeof createFixture>) => writeExecutable(join(fixture.toolchainRoot, 'bin', 'extra'), 'extra'),
      (fixture: ReturnType<typeof createFixture>) => writeFileSync(fixture.toolPaths.clang, 'changed-clang')
    ]) {
      const fixture = createFixture()
      mutate(fixture)
      expect(() => contract.resolvePinnedRustToolchain(fixture.options)).toThrow(/identity mismatch/)
    }

    const fixture = createFixture()
    expect(() => contract.resolvePinnedRustToolchain({
      ...fixture.options,
      platform: 'linux',
      arch: 'x64'
    })).toThrow(/No pinned Rust toolchain/)
  })

  it('blocks an euid-owned toolchain before version probes or build-state effects', () => {
    const fixture = createFixture()
    const preflight = contract.nativeRustToolchainReleasePreflight({
      ...fixture.options,
      euid: process.geteuid?.() ?? 501
    })

    expect(preflight).toEqual(expect.objectContaining({
      purpose: 'analytix-cargo-execution',
      status: 'blocked',
      trustClass: 'local_provisional',
      releaseEligible: false,
      blocker: 'toolchain_user_writable',
      ownershipClass: 'user_writable',
      targetKey: 'darwin-arm64',
      targetTriple: 'aarch64-apple-darwin'
    }))
    expect(fixture.execCalls).toEqual([])

    const buildScript = readFileSync(join(process.cwd(), 'scripts', 'build-data-analysis-native-tools.cjs'), 'utf8')
    const buildTargetOffset = buildScript.indexOf('function buildTarget(')
    const coordinatorOffset = buildScript.indexOf('runNativeBuildCoordinator({', buildTargetOffset)
    expect(buildTargetOffset).toBeGreaterThan(0)
    expect(coordinatorOffset).toBeGreaterThan(buildTargetOffset)
    expect(buildScript.match(/runNativeBuildCoordinator\(\{/g)).toHaveLength(1)
    expect(buildScript).not.toContain('nativeRustToolchainReleasePreflight()')
    expect(buildScript).not.toContain('publishNativeComponentGeneration(')
    expect(buildScript).not.toContain('openNativeSourceSnapshotSession(')
    for (const effect of ['acquireTargetBuildLock(', 'mkdtempSync(', 'resolveDevelopmentRustToolchain()']) {
      expect(coordinatorOffset).toBeLessThan(buildScript.indexOf(effect, buildTargetOffset))
    }
  })

  it('allows the repository-pinned version from local Rustup only as non-release development input', () => {
    const fixture = createFixture()
    const localLock = structuredClone((fixture.options as any).lock)
    const host = localLock.hosts['darwin-arm64']
    host.cargoExecutableSha256 = digestFile(writeExecutable(join(temporaryRoot(), 'other-cargo'), 'other'))
    host.rustcExecutableSha256 = digestFile(writeExecutable(join(temporaryRoot(), 'other-rustc'), 'other'))
    host.toolchainTreeSha256 = 'f'.repeat(64)
    host.sdkSettingsJsonSha256 = 'e'.repeat(64)
    host.sdkSettingsPlistSha256 = 'd'.repeat(64)
    host.tools = Object.fromEntries(Object.keys(host.tools).map((name) => [name, 'c'.repeat(64)]))

    const resolved = contract.resolveDevelopmentRustToolchain({
      ...fixture.options,
      lock: localLock,
      developerDirectory: fixture.developerDirectory,
      sdkRoot: fixture.sdkRoot,
      sdkVersion: 'fixture'
    })

    expect(resolved).toEqual(expect.objectContaining({
      key: 'darwin-arm64',
      root: fixture.toolchainRoot,
      cargo: fixture.cargo,
      rustc: fixture.rustc,
      releaseEligible: false,
      trustClass: 'development_local_non_authoritative'
    }))
    expect(resolved.lock).toEqual(expect.objectContaining({
      cargoExecutableSha256: digestFile(fixture.cargo),
      rustcExecutableSha256: digestFile(fixture.rustc)
    }))
    expect(resolved.environment.RUSTC).toBe(fixture.rustc)
    expect(resolved.environment.SDKROOT).toBe(fixture.sdkRoot)
    expect(fixture.execCalls).toEqual([
      { file: fixture.cargo, args: ['--version'] },
      { file: fixture.rustc, args: ['--version'] },
      { file: fixture.rustc, args: ['-vV'] }
    ])
  })
})

function createFixture(): {
  options: Record<string, unknown>
  toolchainRoot: string
  cargo: string
  rustc: string
  developerDirectory: string
  sdkRoot: string
  toolPaths: Record<string, string>
  tree: { fileCount: number; totalBytes: number; sha256: string }
  execCalls: Array<{ file: string; args: string[] }>
} {
  const root = temporaryRoot()
  const toolchainRoot = join(root, 'toolchain')
  const bin = join(toolchainRoot, 'bin')
  const developerDirectory = join(root, 'developer')
  const sdkRoot = join(root, 'sdk')
  mkdirSync(bin, { recursive: true })
  mkdirSync(join(developerDirectory, 'usr', 'bin'), { recursive: true })
  mkdirSync(sdkRoot, { recursive: true })
  const sdkSettingsJSON = writeExecutable(join(sdkRoot, 'SDKSettings.json'), 'fixture-sdk-json')
  const sdkSettingsPlist = writeExecutable(join(sdkRoot, 'SDKSettings.plist'), 'fixture-sdk-plist')
  const cargo = writeExecutable(join(bin, 'cargo'), 'pinned-cargo')
  const rustc = writeExecutable(join(bin, 'rustc'), 'pinned-rustc')
  const toolPaths = Object.fromEntries(
    ['ar', 'clang', 'codesign', 'ld'].map((name) => [
      name,
      writeExecutable(join(developerDirectory, 'usr', 'bin', name), `pinned-${name}`)
    ])
  )
  const tree = contract.toolchainTreeIdentity(toolchainRoot)
  const locked = {
    hostTriple: 'aarch64-apple-darwin',
    cargoExecutableSha256: digestFile(cargo),
    cargoVersion: 'cargo 1.94.1 (fixture 2026-01-01)',
    rustcExecutableSha256: digestFile(rustc),
    rustcVersion: 'rustc 1.94.1 (fixture 2026-01-01)',
    toolchainFileCount: tree.fileCount,
    toolchainTreeSha256: tree.sha256,
    developerDirectory,
    sdkRoot,
    sdkVersion: 'fixture',
    sdkSettingsJsonSha256: digestFile(sdkSettingsJSON),
    sdkSettingsPlistSha256: digestFile(sdkSettingsPlist),
    tools: Object.fromEntries(Object.entries(toolPaths).map(([name, path]) => [name, digestFile(path)]))
  }
  const execCalls: Array<{ file: string; args: string[] }> = []
  return {
    toolchainRoot,
    cargo,
    rustc,
    developerDirectory,
    sdkRoot,
    toolPaths,
    tree,
    execCalls,
    options: {
      platform: 'darwin',
      arch: 'arm64',
      home: root,
      toolchainRoot,
      toolPaths,
      lock: {
        schemaVersion: 1,
        rustVersion: '1.94.1',
        hosts: { 'darwin-arm64': locked }
      },
      execFileSync: (file: string, args: string[]) => {
        execCalls.push({ file, args })
        if (file === cargo) return `${locked.cargoVersion}\n`
        if (file === rustc && args[0] === '-vV') {
          return `${locked.rustcVersion}\nhost: ${locked.hostTriple}\n`
        }
        if (file === rustc) return `${locked.rustcVersion}\n`
        throw new Error(`unexpected executable: ${file}`)
      }
    }
  }
}

function temporaryRoot(): string {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'analytix-rust-native-toolchain-'))
  temporaryRoots.push(root)
  return root
}

function writeExecutable(path: string, value: string): string {
  writeFileSync(path, value, 'utf8')
  chmodSync(path, 0o755)
  return path
}

function digestFile(path: string): string {
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}
