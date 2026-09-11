import { spawnSync } from 'node:child_process'
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  buildDarwinTerminalSandboxProfile,
  defaultDarwinTerminalProtectedRoots,
  prepareTerminalProcessLaunch,
  TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE
} from './terminal-process-policy'

const taskRoots: string[] = []

function taskRoot(): string {
  const trustedTemporaryRoot = realpathSync(String(process.env.TMPDIR || ''))
  const root = mkdtempSync(join(trustedTemporaryRoot, 'analytix-terminal-policy-'))
  taskRoots.push(root)
  return root
}

afterEach(() => {
  while (taskRoots.length > 0) {
    const root = taskRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

describe('terminal process containment', () => {
  it('matches the runtime child sandbox and keeps ordinary shell work available', () => {
    if (process.platform !== 'darwin') return
    const root = taskRoot()
    const protectedRoot = join(root, 'protected-"quoted"-\\backslash')
    const ordinaryRoot = join(root, 'ordinary')
    mkdirSync(protectedRoot, { recursive: true, mode: 0o700 })
    mkdirSync(ordinaryRoot, { recursive: true, mode: 0o700 })
    const protectedFile = join(protectedRoot, 'secret.txt')
    const ordinaryFile = join(ordinaryRoot, 'ordinary.txt')
    const protectedLink = join(ordinaryRoot, 'protected-link')
    writeFileSync(protectedFile, 'private-terminal-sentinel', { mode: 0o600 })
    writeFileSync(ordinaryFile, 'ordinary-terminal-value', { mode: 0o600 })
    symlinkSync(protectedFile, protectedLink)

    const ordinary = prepareTerminalProcessLaunch({
      shellFile: '/bin/cat',
      shellArgs: [ordinaryFile],
      cwd: ordinaryRoot,
      protectedRoots: [protectedRoot],
      env: process.env
    })
    const ordinaryResult = spawnSync(ordinary.file, ordinary.args, {
      cwd: ordinary.cwd,
      env: ordinary.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(ordinaryResult.status).toBe(0)
    expect(ordinaryResult.stdout).toBe('ordinary-terminal-value')

    for (const shellArgs of [
      [protectedFile],
      [protectedLink]
    ]) {
      const denied = prepareTerminalProcessLaunch({
        shellFile: '/bin/cat',
        shellArgs,
        cwd: ordinaryRoot,
        protectedRoots: [protectedRoot],
        env: process.env
      })
      const deniedResult = spawnSync(denied.file, denied.args, {
        cwd: denied.cwd,
        env: denied.env,
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(deniedResult.status).not.toBe(0)
      expect(deniedResult.stdout).not.toContain('private-terminal-sentinel')
      expect(deniedResult.stderr).not.toContain('private-terminal-sentinel')
    }

    const child = prepareTerminalProcessLaunch({
      shellFile: '/bin/sh',
      shellArgs: ['-c', '/bin/cat "$1"', 'analytix-terminal-test', protectedFile],
      cwd: ordinaryRoot,
      protectedRoots: [protectedRoot],
      env: process.env
    })
    const childResult = spawnSync(child.file, child.args, {
      cwd: child.cwd,
      env: child.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(childResult.status).not.toBe(0)
    expect(childResult.stdout).not.toContain('private-terminal-sentinel')
    expect(childResult.stderr).not.toContain('private-terminal-sentinel')
  })

  it('drops ambient credentials, Analytix authority, and process-injection variables', () => {
    if (process.platform !== 'darwin') return
    const root = taskRoot()
    const protectedRoot = join(root, 'protected')
    const ordinaryRoot = join(root, 'ordinary')
    mkdirSync(protectedRoot, { mode: 0o700 })
    mkdirSync(ordinaryRoot, { mode: 0o700 })
    const launch = prepareTerminalProcessLaunch({
      shellFile: '/usr/bin/env',
      shellArgs: [],
      cwd: ordinaryRoot,
      protectedRoots: [protectedRoot],
      env: {
        HOME: process.env.HOME,
        PATH: `${protectedRoot}:/usr/bin:/bin`,
        LANG: process.env.LANG,
        ANALYTIX_RUNTIME_TOKEN: 'terminal-authority-sentinel',
        OPENAI_API_KEY: 'provider-secret-sentinel',
        GIT_CONFIG_COUNT: '1',
        GIT_CONFIG_KEY_0: 'core.fsmonitor',
        GIT_CONFIG_VALUE_0: '/tmp/attacker',
        DYLD_INSERT_LIBRARIES: '/tmp/attacker.dylib',
        BASH_ENV: '/tmp/attacker-bash-env',
        ENV: '/tmp/attacker-shell-env',
        SSH_AUTH_SOCK: '/tmp/attacker-agent.sock',
        NODE_OPTIONS: '--require=/tmp/attacker.cjs'
      }
    })
    const result = spawnSync(launch.file, launch.args, {
      cwd: launch.cwd,
      env: launch.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stdout).toContain('TERM=xterm-256color')
    expect(result.stdout).toContain('COLORTERM=truecolor')
    expect(result.stdout).toContain('PATH=/usr/bin:/bin')
    expect(result.stdout).toContain('SHELL=/usr/bin/env')
    expect(result.stdout).toContain('HISTFILE=/dev/null')
    expect(result.stdout).not.toContain(protectedRoot)
    for (const forbidden of [
      'terminal-authority-sentinel',
      'provider-secret-sentinel',
      'GIT_CONFIG_',
      'DYLD_INSERT_LIBRARIES',
      'BASH_ENV',
      'SSH_AUTH_SOCK',
      'NODE_OPTIONS'
    ]) {
      expect(result.stdout).not.toContain(forbidden)
      expect(result.stderr).not.toContain(forbidden)
    }
  })

  it('matches the runtime default macOS privacy roots', () => {
    const home = taskRoot()
    expect(defaultDarwinTerminalProtectedRoots(home)).toEqual([
      join(home, 'Music'),
      join(home, 'Pictures'),
      join(home, 'Movies'),
      join(home, 'Library')
    ])
  })

  it('keeps interactive zsh startup files and their history policy out of the PTY', () => {
    if (process.platform !== 'darwin') return
    const root = taskRoot()
    const home = join(root, 'home')
    const protectedRoot = join(root, 'protected')
    const ordinaryRoot = join(root, 'ordinary')
    mkdirSync(home, { mode: 0o700 })
    mkdirSync(protectedRoot, { mode: 0o700 })
    mkdirSync(ordinaryRoot, { mode: 0o700 })
    writeFileSync(
      join(home, '.zshrc'),
      'export ANALYTIX_TERMINAL_RC_CANARY=restored-secret\n' +
      `export HISTFILE=${join(home, 'unsafe-history')}\n`,
      { mode: 0o600 }
    )
    const launch = prepareTerminalProcessLaunch({
      shellFile: '/bin/zsh',
      shellArgs: ['-f', '-i', '-c', '/usr/bin/env'],
      cwd: ordinaryRoot,
      protectedRoots: [protectedRoot],
      env: {
        HOME: home,
        PATH: '/usr/bin:/bin'
      }
    })
    const result = spawnSync(launch.file, launch.args, {
      cwd: launch.cwd,
      env: launch.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stdout).not.toContain('ANALYTIX_TERMINAL_RC_CANARY')
    expect(result.stdout).toContain('HISTFILE=/dev/null')
  })

  it('protects runtime-private children without disabling an ordinary dataDir workspace', () => {
    if (process.platform !== 'darwin') return
    const root = taskRoot()
    const dataDir = join(root, 'runtime-data')
    const workspace = join(dataDir, 'workspace')
    const privateRoot = join(dataDir, 'private')
    const childRunsRoot = join(dataDir, 'child-runs')
    mkdirSync(workspace, { recursive: true, mode: 0o700 })
    mkdirSync(privateRoot, { mode: 0o700 })
    mkdirSync(childRunsRoot, { mode: 0o700 })
    const launch = prepareTerminalProcessLaunch({
      shellFile: '/bin/pwd',
      shellArgs: [],
      cwd: workspace,
      protectedRoots: [privateRoot, childRunsRoot],
      env: process.env
    })
    const result = spawnSync(launch.file, launch.args, {
      cwd: launch.cwd,
      env: launch.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stdout.trim()).toBe(workspace)
  })

  it('rejects direct and symlinked protected working directories before spawn', () => {
    if (process.platform !== 'darwin') return
    const root = taskRoot()
    const protectedRoot = join(root, 'protected')
    const ordinaryRoot = join(root, 'ordinary')
    const protectedLink = join(ordinaryRoot, 'protected-link')
    mkdirSync(protectedRoot, { mode: 0o700 })
    mkdirSync(ordinaryRoot, { mode: 0o700 })
    symlinkSync(protectedRoot, protectedLink)

    for (const cwd of [protectedRoot, protectedLink]) {
      expect(() => prepareTerminalProcessLaunch({
        shellFile: '/bin/zsh',
        shellArgs: [],
        cwd,
        protectedRoots: [protectedRoot],
        env: process.env
      })).toThrow(TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE)
    }
  })

  it('fixes local-deputy and protected-root rules before launching a shell', () => {
    const root = taskRoot()
    const protectedRoot = join(root, 'protected-"quoted"-\\backslash')
    const profile = buildDarwinTerminalSandboxProfile([protectedRoot])

    for (const rule of [
      '(deny appleevent-send)\n',
      '(deny job-creation lsopen system-socket)\n',
      '(deny mach-bootstrap mach-lookup mach-register mach-per-user-lookup mach-cross-domain-lookup)\n',
      '(deny ipc-posix* ipc-sysv*)\n',
      '(deny network-outbound (remote unix-socket))\n',
      '(deny network-outbound (remote ip "localhost:*"))\n'
    ]) {
      expect(profile).toContain(rule)
    }
    expect(profile).toContain(
      '(deny file-read* file-write* process-exec (subpath ' +
      `"${protectedRoot.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"))\n`
    )
  })

  it('fails only the terminal effect on an unsupported host', () => {
    const root = taskRoot()
    const protectedRoot = join(root, 'protected')
    const ordinaryRoot = join(root, 'ordinary')
    mkdirSync(protectedRoot, { mode: 0o700 })
    mkdirSync(ordinaryRoot, { mode: 0o700 })

    expect(() => prepareTerminalProcessLaunch({
      shellFile: '/bin/sh',
      shellArgs: [],
      cwd: ordinaryRoot,
      protectedRoots: [protectedRoot],
      env: process.env,
      platform: 'linux'
    })).toThrow(TERMINAL_PROCESS_CONTAINMENT_UNAVAILABLE)
  })
})
