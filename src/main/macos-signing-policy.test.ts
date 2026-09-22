import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

// eslint-disable-next-line @typescript-eslint/no-require-imports
const signingPolicy = require('../../scripts/macos-signing-policy.cjs')
// eslint-disable-next-line @typescript-eslint/no-require-imports
const macSign = require('../../scripts/mac-sign.cjs')
// eslint-disable-next-line @typescript-eslint/no-require-imports
const nativeComponentContract = require('../../scripts/native-component-contract.cjs')

const temporaryRoots: string[] = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

describe('macOS native-code signing authority', () => {
  it('requires the exact declared Core inventory without relaxing the full inventory', () => {
    const runtime = signingPolicy.policy.strictNativeRelativePaths[0]
    const core = new Map([[runtime, 1]])
    expect(() => macSign._internals.validateStrictSignedFiles(core, 'core')).not.toThrow()
    expect(() => macSign._internals.validateStrictSignedFiles(core, 'full')).toThrow(/exactly once/)
    expect(() => macSign._internals.validateStrictSignedFiles(new Map(), 'core')).toThrow(/exactly once/)
    expect(() => macSign._internals.validateStrictSignedFiles(new Map([[runtime, 2]]), 'core')).toThrow(/exactly once/)
    expect(() => macSign._internals.validateStrictSignedFiles(new Map([...core, ['unexpected', 1]]), 'core')).toThrow(/exactly once/)
    expect(() => macSign._internals.validateStrictSignedFiles(core, 'unknown')).toThrow(/release_profile_invalid/)
  })

  it('binds Core signing to its parsed authority and rejects unexpected professional resources', () => {
    const app = join(temporaryRoot(), 'Analytix.app')
    const resources = join(app, 'Contents/Resources')
    mkdirSync(resources, { recursive: true })
    const options = { app, identity: '-' }
    const readAuthority = vi.fn(() => ({
      nativeDisposition: { kind: 'core_no_professional_components' }, authorityDigest: 'a'.repeat(64)
    }))
    expect(macSign._internals.validateCoreSigningProfile(options, 'core', readAuthority)).toBe('a'.repeat(64))
    expect(readAuthority).toHaveBeenCalledWith(expect.objectContaining({ arch: 'arm64', electronPlatformName: 'darwin' }))
    expect(() => macSign._internals.validateCoreSigningProfile(options, 'core', () => ({
      nativeDisposition: { kind: 'development_local_build' }
    }))).toThrow(/core_profile_signing_authority_mismatch/)
    expect(() => macSign._internals.validateCoreSigningProfile({ ...options, identity: 'Developer ID' }, 'core', readAuthority)).toThrow(/core_profile_signing_authority_mismatch/)
    mkdirSync(join(resources, 'runtime'))
    symlinkSync(join(resources, 'missing'), join(resources, 'runtime/analytix-data-engine'))
    expect(() => macSign._internals.validateCoreSigningProfile(options, 'core', readAuthority)).toThrow(/professional_resource_present/)
  })

  it('freezes an empty native entitlement set and the exact executable inventory', () => {
    expect(signingPolicy.policy.strictNativeRelativePaths).toEqual([
      'Contents/Resources/runtime-go/bin/runtime-server',
      'Contents/Resources/runtime/analytix-import-accelerator',
      'Contents/Resources/runtime/analytix-cleaning-ops',
      'Contents/Resources/runtime/analytix-analysis-compute',
      'Contents/Resources/runtime/analytix-data-engine'
    ])
    expect(readFileSync(signingPolicy.policy.nativeEntitlementsAbsolutePath, 'utf8')).toBe(
      '<?xml version="1.0" encoding="UTF-8"?>\n' +
      '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n' +
      '<plist version="1.0">\n' +
      '  <dict/>\n' +
      '</plist>\n'
    )
  })

  it('blocks Developer ID release until the fixed official Team ID is configured', () => {
    expect(() => signingPolicy.requireOfficialTeamIdentifier()).toThrow(/Official Apple Team ID is not configured/)
    const configured = JSON.parse(signingPolicy.policyBytes.toString('utf8'))
    configured.officialTeamIdentifier = 'ABCDE12345'
    expect(signingPolicy.requireOfficialTeamIdentifier(configured)).toBe('ABCDE12345')
  })

  it('assigns empty entitlements and hardened runtime only to the frozen native paths', () => {
    const root = temporaryRoot()
    const app = join(root, 'Analytix.app')
    const strictFiles = new Map<string, number>()
    const defaults = () => ({
      entitlements: '/electron/inherited.plist',
      hardenedRuntime: true,
      timestamp: undefined,
      additionalArguments: ['--unsafe-test-argument']
    })
    for (const relativePath of signingPolicy.policy.strictNativeRelativePaths) {
      const path = join(app, ...relativePath.split('/'))
      mkdirSync(dirname(path), { recursive: true })
      writeFileSync(path, 'native-code', { mode: 0o755 })
      expect(macSign._internals.strictOptionsForFile(
        { app, identity: '-' },
        path,
        defaults,
        strictFiles
      )).toEqual({
        entitlements: signingPolicy.policy.nativeEntitlementsAbsolutePath,
        hardenedRuntime: true,
        requirements: undefined,
        signatureFlags: ['runtime'],
        timestamp: 'none',
        additionalArguments: []
      })
    }
    expect(Array.from(strictFiles.keys()).sort()).toEqual(
      [...signingPolicy.policy.strictNativeRelativePaths].sort()
    )
    expect(() => macSign._internals.validateStrictSignedFiles(strictFiles)).not.toThrow()
    strictFiles.set(signingPolicy.policy.strictNativeRelativePaths[0], 2)
    expect(() => macSign._internals.validateStrictSignedFiles(strictFiles)).toThrow(/exactly once/)

    const ordinary = join(app, 'Contents/MacOS/Analytix')
    mkdirSync(dirname(ordinary), { recursive: true })
    writeFileSync(ordinary, 'electron-code', { mode: 0o755 })
    expect(macSign._internals.strictOptionsForFile(
      { app, identity: '-' },
      ordinary,
      defaults,
      strictFiles
    )).toEqual({
      entitlements: '/electron/inherited.plist',
      hardenedRuntime: true,
      timestamp: 'none',
      additionalArguments: ['--unsafe-test-argument']
    })
  })

  it('rejects a symlink at a frozen native-code path', () => {
    const root = temporaryRoot()
    const app = join(root, 'Analytix.app')
    const target = join(root, 'outside-native')
    const path = join(app, 'Contents/Resources/runtime/analytix-data-engine')
    mkdirSync(dirname(path), { recursive: true })
    writeFileSync(target, 'outside', { mode: 0o755 })
    symlinkSync(target, path)
    expect(() => macSign._internals.strictOptionsForFile(
      { app, identity: '-' },
      path,
      () => ({}),
      new Map()
    )).toThrow(/regular canonical file/)

    const spawnHelper = join(
      app,
      'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper'
    )
    mkdirSync(dirname(spawnHelper), { recursive: true })
    symlinkSync(target, spawnHelper)
    expect(() => macSign._internals.strictNodePtySpawnHelper(app)).toThrow(/canonical executable/)
  })

  it('signs the exact node-pty helper with Hardened Runtime and no entitlement blob', () => {
    const root = temporaryRoot()
    const app = join(root, 'Analytix.app')
    const spawnHelper = join(
      app,
      'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper'
    )
    mkdirSync(dirname(spawnHelper), { recursive: true })
    writeFileSync(spawnHelper, 'node-pty-helper', { mode: 0o755 })
    const codesign = vi.fn((command: string, args: string[]) => {
      expect(command).toBe('/usr/bin/codesign')
      if (args.includes('--entitlements')) {
        return { status: 0, signal: null, stdout: '', stderr: '' }
      }
      if (args.includes('--display')) {
        return { status: 0, signal: null, stdout: '', stderr: 'flags=0x10000(runtime)' }
      }
      return { status: 0, signal: null, stdout: '', stderr: '' }
    })
    expect(macSign._internals.signNodePtySpawnHelper(
      { app, identity: '-' },
      codesign
    )).toEqual(expect.objectContaining({
      filePath: spawnHelper,
      relativePath:
        'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper'
    }))
    expect(codesign).toHaveBeenNthCalledWith(
      1,
      '/usr/bin/codesign',
      ['--force', '--sign', '-', '--options', 'runtime', '--timestamp=none', spawnHelper],
      expect.objectContaining({ encoding: 'utf8' })
    )
    expect(codesign.mock.calls.some(([, args]) => args.includes('--entitlements'))).toBe(true)
  })

  it('uses the electron-builder keychain for Developer ID helper signing', () => {
    const root = temporaryRoot()
    const app = join(root, 'Analytix.app')
    const spawnHelper = join(
      app,
      'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper'
    )
    mkdirSync(dirname(spawnHelper), { recursive: true })
    writeFileSync(spawnHelper, 'node-pty-helper', { mode: 0o755 })
    const codesign = vi.fn((_command: string, args: string[]) => {
      if (args.includes('--entitlements')) {
        return { status: 0, signal: null, stdout: '', stderr: '' }
      }
      if (args.includes('--display')) {
        return { status: 0, signal: null, stdout: '', stderr: 'flags=0x10000(runtime)' }
      }
      return { status: 0, signal: null, stdout: '', stderr: '' }
    })

    macSign._internals.signNodePtySpawnHelper({
      app,
      identity: 'Developer ID Application: Analytix',
      keychain: '/private/tmp/analytix-signing.keychain-db'
    }, codesign)

    expect(codesign).toHaveBeenNthCalledWith(
      1,
      '/usr/bin/codesign',
      [
        '--force',
        '--sign',
        'Developer ID Application: Analytix',
        '--options',
        'runtime',
        '--timestamp',
        '--keychain',
        '/private/tmp/analytix-signing.keychain-db',
        spawnHelper
      ],
      expect.objectContaining({ encoding: 'utf8' })
    )
  })

  it('verifies fixed Developer ID authority, Team ID, runtime flag, timestamp, and empty entitlements', () => {
    const spawnSync = signatureSpawn()
    const result = nativeComponentContract.verifyDarwinCodeSignature('/trusted/native', {
      spawnSync,
      requireDeveloperID: true,
      requireSecureTimestamp: true,
      requireHardenedRuntime: true,
      inspectEntitlements: true,
      requireEmptyEntitlements: true,
      expectedTeamIdentifier: 'ABCDE12345'
    })
    expect(result).toEqual(expect.objectContaining({
      teamIdentifier: 'ABCDE12345',
      secureTimestamp: true,
      codeDirectoryFlags: 0x10000,
      flagNames: ['runtime'],
      entitlements: {}
    }))
    expect(spawnSync.mock.calls[0]?.[1]).toContainEqual(expect.stringContaining(
      'anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists'
    ))
  })

  it('rejects wrong Team, missing runtime, and every non-empty strict entitlement set', () => {
    expect(() => nativeComponentContract.verifyDarwinCodeSignature('/trusted/native', {
      spawnSync: signatureSpawn({ teamIdentifier: 'WRONG12345' }),
      expectedTeamIdentifier: 'ABCDE12345'
    })).toThrow(/TeamIdentifier mismatch/)

    expect(() => nativeComponentContract.verifyDarwinCodeSignature('/trusted/native', {
      spawnSync: signatureSpawn({ flags: '0x2(adhoc)' }),
      requireHardenedRuntime: true
    })).toThrow(/missing Hardened Runtime/)

    expect(() => nativeComponentContract.verifyDarwinCodeSignature('/trusted/native', {
      spawnSync: signatureSpawn({ entitlements: { 'com.apple.security.cs.allow-jit': true } }),
      inspectEntitlements: true,
      requireEmptyEntitlements: true
    })).toThrow(/empty entitlement set/)
  })

  it('signs and mechanically verifies a real ad-hoc macOS strict-native inventory', async () => {
    if (process.platform !== 'darwin') return
    const root = temporaryRoot()
    const app = join(root, 'Analytix.app')
    const mainExecutable = join(app, 'Contents/MacOS/Analytix')
    mkdirSync(dirname(mainExecutable), { recursive: true })
    copyFileSync('/usr/bin/true', mainExecutable)
    chmodSync(mainExecutable, 0o755)
    writeFileSync(join(app, 'Contents/Info.plist'), [
      '<?xml version="1.0" encoding="UTF-8"?>',
      '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">',
      '<plist version="1.0"><dict>',
      '<key>CFBundleExecutable</key><string>Analytix</string>',
      '<key>CFBundleIdentifier</key><string>com.analytix.signing-test</string>',
      '<key>CFBundlePackageType</key><string>APPL</string>',
      '</dict></plist>',
      ''
    ].join('\n'))
    for (const relativePath of signingPolicy.policy.strictNativeRelativePaths) {
      const path = join(app, ...relativePath.split('/'))
      mkdirSync(dirname(path), { recursive: true })
      copyFileSync('/usr/bin/true', path)
      chmodSync(path, 0o755)
    }
    const spawnHelper = join(
      app,
      'Contents/MacOS/analytix-node-pty/prebuilds/darwin-arm64/spawn-helper'
    )
    mkdirSync(dirname(spawnHelper), { recursive: true })
    copyFileSync('/usr/bin/true', spawnHelper)
    chmodSync(spawnHelper, 0o755)

    await macSign({
      app,
      identity: '-',
      identityValidation: false,
      platform: 'darwin',
      preAutoEntitlements: false,
      optionsForFile: () => ({
        entitlements: signingPolicy.policy.nativeEntitlementsAbsolutePath,
        hardenedRuntime: true,
        timestamp: 'none',
        additionalArguments: []
      })
    })

    for (const relativePath of signingPolicy.policy.strictNativeRelativePaths) {
      expect(nativeComponentContract.verifyDarwinCodeSignature(
        join(app, ...relativePath.split('/')),
        {
          requireHardenedRuntime: true,
          inspectEntitlements: true,
          requireEmptyEntitlements: true
        }
      )).toEqual(expect.objectContaining({ status: 'passed', entitlements: {} }))
    }
    expect(nativeComponentContract.verifyDarwinCodeSignature(spawnHelper, {
      requireHardenedRuntime: true,
      inspectEntitlements: true,
      requireEmptyEntitlements: true
    })).toEqual(expect.objectContaining({ status: 'passed', entitlements: {} }))
  })
})

function temporaryRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-mac-signing-'))
  temporaryRoots.push(root)
  return root
}

function signatureSpawn(options: {
  teamIdentifier?: string
  flags?: string
  entitlements?: Record<string, unknown>
} = {}) {
  const teamIdentifier = options.teamIdentifier ?? 'ABCDE12345'
  const flags = options.flags ?? '0x10000(runtime)'
  const entitlements = options.entitlements ?? {}
  return vi.fn((command: string, args: string[]) => {
    if (command === '/usr/bin/plutil') {
      return { status: 0, signal: null, stdout: JSON.stringify(entitlements), stderr: '' }
    }
    if (args.includes('--verify')) {
      return { status: 0, signal: null, stdout: '', stderr: '' }
    }
    if (args.includes('--entitlements')) {
      return {
        status: 0,
        signal: null,
        stdout: '<?xml version="1.0"?><plist version="1.0"><dict/></plist>',
        stderr: ''
      }
    }
    return {
      status: 0,
      signal: null,
      stdout: '',
      stderr: [
        `CodeDirectory v=20500 size=100 flags=${flags} hashes=1+0 location=embedded`,
        'Authority=Developer ID Application: Analytix Test (ABCDE12345)',
        `TeamIdentifier=${teamIdentifier}`,
        'Timestamp=Jul 16, 2026 at 05:00:00'
      ].join('\n')
    }
  })
}
