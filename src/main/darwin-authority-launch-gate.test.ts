import {
  chmodSync,
  closeSync,
  copyFileSync,
  mkdtempSync,
  openSync,
  readFileSync,
  renameSync,
  rmSync
} from 'node:fs'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'

const require = createRequire(import.meta.url)
const gate = require('../../scripts/darwin-authority-launch-gate.cjs')
const nativeComponentContract = require('../../scripts/native-component-contract.cjs')
const repoRoot = process.cwd()
const tempRoots: string[] = []

function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-darwin-launch-gate-test-'))
  tempRoots.push(root)
  return root
}

function sha256(bytes: Buffer): string {
  return createHash('sha256').update(bytes).digest('hex')
}

afterEach(() => {
  for (const root of tempRoots.splice(0)) rmSync(root, { recursive: true, force: true })
})

describe('Darwin descriptor-bound authority launch gate', () => {
  it('keeps the gate build closure and forbids the former path-based authority spawn', () => {
    const authoritySource = readFileSync(
      join(repoRoot, 'scripts', 'native-build-probe-authority.cjs'),
      'utf8'
    )
    const wrapperSource = readFileSync(
      join(repoRoot, 'scripts', 'darwin-authority-launch-gate.cjs'),
      'utf8'
    )
    const nativeSource = readFileSync(
      join(repoRoot, 'scripts', 'native', 'darwin-authority-launch-gate.c'),
      'utf8'
    )

    expect(authoritySource).not.toMatch(/spawnSync\(authority\.path/u)
    expect(authoritySource.match(/invokeDarwinAuthorityPinnedV1\(/gu)).toHaveLength(4)
    expect(authoritySource).toContain("'build_coordinator'")
    expect(wrapperSource).not.toMatch(/resolvePinnedRustToolchain|\bcargo\b|\brustc\b|\bxcrun\b/u)
    expect(wrapperSource).toContain("'-o', '/dev/fd/3'")
    expect(wrapperSource).toContain("'-o', '/dev/fd/4'")
    expect(nativeSource).toContain('POSIX_SPAWN_START_SUSPENDED')
    expect(nativeSource).toContain('PROC_PIDREGIONPATHINFO')
    expect(nativeSource).toContain('ANALYTIX_GATE_CS_OPS_CDHASH')
    expect(nativeSource).toContain('F_DUPFD_CLOEXEC')
    expect(nativeSource).toContain('analytix_gate_close_fd_once')
    expect(nativeSource).toContain('napi_wrap')
    expect(nativeSource).not.toContain('g_poisoned')
    expect(nativeSource).not.toMatch(/while\s*\(close\(/u)
    expect(nativeComponentContract.NATIVE_BUILD_CONTEXT_PATHS).toEqual(expect.arrayContaining([
      'scripts/darwin-authority-launch-gate.cjs',
      'scripts/darwin-authority-launch-gate.json',
      'scripts/native/darwin-authority-launch-gate.c'
    ]))
  })

  it('loads a deterministic N-API v1 gate from its opened descriptor or rejects the host explicitly', () => {
    if (process.platform !== 'darwin') {
      expect(() => gate.darwinAuthorityLaunchGateIdentity()).toThrow(/unavailable on this host/)
      return
    }
    const identity = gate.darwinAuthorityLaunchGateIdentity()
    expect(identity).toMatchObject({
      schemaVersion: 1,
      protocol: 'analytix-darwin-authority-launch-gate-v1',
      trustClass: 'local_provisional',
      hostKey: `darwin-${process.arch}`,
      napiVersion: 1,
      descriptorLoaded: true
    })
    expect(identity.sourceSha256).toMatch(/^[0-9a-f]{64}$/u)
    expect(identity.binarySha256).toMatch(/^[0-9a-f]{64}$/u)
    expect(identity.toolchainSha256).toMatch(/^[0-9a-f]{64}$/u)
    expect(identity.binarySize).toBeGreaterThan(0)
  })

  it('executes the opened vnode only while its locator still names that exact vnode', () => {
    if (process.platform !== 'darwin') {
      expect(() => gate.invokeDarwinAuthorityPinnedV1(
        'build_probe', 0, '0'.repeat(64), 1, Buffer.from('x'), [1]
      )).toThrow(/unavailable|rejected/)
      return
    }
    const root = tempRoot()
    const originalPath = join(root, 'authority')
    const pinnedPath = join(root, 'authority-pinned')
    copyFileSync('/bin/cat', originalPath)
    const signed = spawnSync('/usr/bin/codesign', [
      '--force', '--sign', '-', '--timestamp=none', originalPath
    ], { encoding: 'utf8' })
    expect(signed.status).toBe(0)
    chmodSync(originalPath, 0o500)
    const bytes = readFileSync(originalPath)
    const authorityFd = openSync(originalPath, 'r')
    const inheritedFd = openSync(originalPath, 'r')
    try {
      const input = Buffer.from('descriptor-bound\n', 'utf8')
      const stable = gate.invokeDarwinAuthorityPinnedV1(
        'build_probe', authorityFd, sha256(bytes), bytes.length, input, [inheritedFd]
      )
      expect(stable).toMatchObject({
        exitCode: 0,
        signal: 0,
        loadedCdhash: expect.stringMatching(/^[0-9a-f]{40}$/u),
        outerProcessReaped: true
      })
      expect(stable.stdout).toEqual(input)
      expect(stable.stderr).toEqual(Buffer.alloc(0))

      renameSync(originalPath, pinnedPath)
      copyFileSync('/usr/bin/false', originalPath)
      chmodSync(originalPath, 0o500)
      const afterReplacement = gate.invokeDarwinAuthorityPinnedV1(
        'build_probe', authorityFd, sha256(bytes), bytes.length, input, [inheritedFd]
      )
      expect(afterReplacement.exitCode).toBe(0)
      expect(afterReplacement.signal).toBe(0)
      expect(afterReplacement.stdout).toEqual(input)
      expect(afterReplacement.stderr).toEqual(Buffer.alloc(0))
    } finally {
      closeSync(inheritedFd)
      closeSync(authorityFd)
    }
  })

  it('rejects a mismatched opened-image digest before returning an authority result', () => {
    if (process.platform !== 'darwin') {
      expect(() => gate.darwinAuthorityLaunchGateIdentity()).toThrow()
      return
    }
    const root = tempRoot()
    const executable = join(root, 'authority')
    copyFileSync('/bin/cat', executable)
    const signed = spawnSync('/usr/bin/codesign', [
      '--force', '--sign', '-', '--timestamp=none', executable
    ], { encoding: 'utf8' })
    expect(signed.status).toBe(0)
    chmodSync(executable, 0o500)
    const bytes = readFileSync(executable)
    const authorityFd = openSync(executable, 'r')
    const inheritedFd = openSync(executable, 'r')
    try {
      let rejection: Error & { code?: string } | undefined
      try {
        gate.invokeDarwinAuthorityPinnedV1(
          'build_probe', authorityFd, '0'.repeat(64), bytes.length, Buffer.from('x'), [inheritedFd]
        )
      } catch (error) {
        rejection = error as Error & { code?: string }
      }
      expect(rejection?.message).toMatch(/rejected execution/)
      expect(rejection?.code).toBe('ANALYTIX_DARWIN_AUTHORITY_GATE_REJECTED')
      expect(gate.invokeDarwinAuthorityPinnedV1(
        'build_probe', authorityFd, sha256(bytes), bytes.length, Buffer.from('clean\n'), [inheritedFd]
      ).stdout).toEqual(Buffer.from('clean\n'))
    } finally {
      closeSync(inheritedFd)
      closeSync(authorityFd)
    }
  })

  it('classifies post-resume output overflow as indeterminate and poisons that gate instance', () => {
    if (process.platform !== 'darwin') {
      expect(() => gate.darwinAuthorityLaunchGateIdentity()).toThrow()
      return
    }
    const root = tempRoot()
    const executable = join(root, 'authority')
    copyFileSync('/usr/bin/yes', executable)
    const signed = spawnSync('/usr/bin/codesign', [
      '--force', '--sign', '-', '--timestamp=none', executable
    ], { encoding: 'utf8' })
    expect(signed.status).toBe(0)
    chmodSync(executable, 0o500)
    const bytes = readFileSync(executable)
    const authorityFd = openSync(executable, 'r')
    const inheritedFd = openSync(executable, 'r')
    try {
      let indeterminate: Error & { code?: string } | undefined
      try {
        gate.invokeDarwinAuthorityPinnedV1(
          'build_probe', authorityFd, sha256(bytes), bytes.length, Buffer.from('x'), [inheritedFd]
        )
      } catch (error) {
        indeterminate = error as Error & { code?: string }
      }
      expect(indeterminate?.code).toBe(
        'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME'
      )
      let poisoned: Error & { code?: string } | undefined
      try {
        gate.invokeDarwinAuthorityPinnedV1(
          'build_probe', authorityFd, sha256(bytes), bytes.length, Buffer.from('x'), [inheritedFd]
        )
      } catch (error) {
        poisoned = error as Error & { code?: string }
      }
      expect(poisoned?.code).toBe('ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED')

      const recoveryExecutable = join(root, 'recovery-authority')
      copyFileSync('/bin/cat', recoveryExecutable)
      const recoverySigned = spawnSync('/usr/bin/codesign', [
        '--force', '--sign', '-', '--timestamp=none', recoveryExecutable
      ], { encoding: 'utf8' })
      expect(recoverySigned.status).toBe(0)
      chmodSync(recoveryExecutable, 0o500)
      const recoveryBytes = readFileSync(recoveryExecutable)
      const recoveryFd = openSync(recoveryExecutable, 'r')
      const recoveryInheritedFd = openSync(recoveryExecutable, 'r')
      try {
        const fresh = gate._internals.freshDarwinAuthorityLaunchGateV1()
        const freshResult = gate._internals.invokeDarwinAuthorityPinnedWithGateV1(
          fresh,
          'build_probe',
          recoveryFd,
          sha256(recoveryBytes),
          recoveryBytes.length,
          Buffer.from('fresh-state\n'),
          [recoveryInheritedFd]
        )
        expect(freshResult.stdout).toEqual(Buffer.from('fresh-state\n'))
        expect(() => gate.invokeDarwinAuthorityPinnedV1(
          'build_probe',
          recoveryFd,
          sha256(recoveryBytes),
          recoveryBytes.length,
          Buffer.from('cached-state\n'),
          [recoveryInheritedFd]
        )).toThrow(/poisoned/)
      } finally {
        closeSync(recoveryInheritedFd)
        closeSync(recoveryFd)
      }
    } finally {
      closeSync(inheritedFd)
      closeSync(authorityFd)
    }
  })

  it('loads the same N-API v1 gate under the packaged Electron Node ABI on Darwin', () => {
    if (process.platform !== 'darwin') {
      expect(process.platform).not.toBe('darwin')
      return
    }
    const electron = require('electron') as string
    const script = [
      `const gate=require(${JSON.stringify(join(repoRoot, 'scripts', 'darwin-authority-launch-gate.cjs'))})`,
      'const identity=gate.darwinAuthorityLaunchGateIdentity()',
      'process.stdout.write(JSON.stringify(identity))'
    ].join(';')
    const result = spawnSync(electron, ['-e', script], {
      env: { ...process.env, ELECTRON_RUN_AS_NODE: '1' },
      encoding: 'utf8',
      timeout: 120_000,
      maxBuffer: 1024 * 1024,
      killSignal: 'SIGKILL'
    })
    expect(result.error).toBeUndefined()
    expect(result.signal).toBeNull()
    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout)).toMatchObject({
      protocol: 'analytix-darwin-authority-launch-gate-v1',
      napiVersion: 1,
      descriptorLoaded: true
    })
  })
})
