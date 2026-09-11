import { statSync } from 'node:fs'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { homedir, tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import {
  expandHomePath,
  hasUnquotedShellConditionalSequence,
  makeListEntry,
  normalizeToolPath,
  resolveReadablePath,
  resolveToolOutputPath,
  resolveWorkspacePath,
  resolveExecutable,
  shellConfig,
  shellCommandArgs,
  shellDisplayName,
  shellRuntimeInfo,
  shellRuntimeInstruction,
  shellSupportsConditionalChaining,
  toolRelativePath
} from '../../tool-test-support/tool/builtin-tool-utils.js'
import { terminateSpawnTree } from '../../shared/spawn-tree.js'
import type { ToolHostContext } from '../../ports/tool-host.js'

function lookup(results: Record<string, string>) {
  return ((command: string, args: string[]) => {
    const key = `${command} ${args.join(' ')}`
    const stdout = results[key] ?? ''
    return {
      status: stdout ? 0 : 1,
      stdout
    }
  }) as never
}

function toolContext(workspace: string, sandboxMode: ToolHostContext['sandboxMode'] = 'workspace-write'): ToolHostContext {
  return {
    threadId: 'thread_paths',
    turnId: 'turn_paths',
    workspace,
    approvalPolicy: 'always',
    sandboxMode,
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow'
  }
}

describe('shellConfig', () => {
  it('prefers PowerShell on Windows even when Git Bash is available', () => {
    expect(shellConfig('win32', lookup({
      'where pwsh.exe': 'C:\\Program Files\\PowerShell\\7\\pwsh.exe\r\n',
      'where bash.exe': 'C:\\Program Files\\Git\\bin\\bash.exe\r\n'
    }))).toEqual({
      shell: 'C:\\Program Files\\PowerShell\\7\\pwsh.exe',
      args: ['-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command']
    })
  })

  it('falls back to Windows PowerShell when pwsh is unavailable', () => {
    expect(shellConfig('win32', lookup({
      'where powershell.exe': 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe\r\n'
    }))).toEqual({
      shell: 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe',
      args: ['-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command']
    })
  })

  it('finds the standard PowerShell 7 install path when pwsh is not on PATH', () => {
    expect(shellConfig(
      'win32',
      lookup({ 'where powershell.exe': 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe\r\n' }),
      (path) => path === 'C:\\Program Files\\PowerShell\\7\\pwsh.exe',
      { ProgramFiles: 'C:\\Program Files' }
    )).toEqual({
      shell: 'C:\\Program Files\\PowerShell\\7\\pwsh.exe',
      args: ['-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command']
    })
  })

  it('falls back to Git Bash on Windows when PowerShell is unavailable', () => {
    expect(shellConfig('win32', lookup({
      'where bash.exe': 'C:\\Program Files\\Git\\bin\\bash.exe\r\n'
    }))).toEqual({
      shell: 'C:\\Program Files\\Git\\bin\\bash.exe',
      args: ['-lc']
    })
  })

  it('falls back to cmd.exe on Windows when no richer shell is available', () => {
    expect(shellConfig('win32', lookup({}))).toEqual({
      shell: 'cmd.exe',
      args: ['/d', '/s', '/c']
    })
  })

  it('keeps the POSIX shell behavior on non-Windows platforms', () => {
    expect(shellConfig('darwin', lookup({}), () => true)).toEqual({
      shell: '/bin/bash',
      args: ['-lc']
    })
  })
})

describe('resolveExecutable', () => {
  it('uses where on Windows to find executables on PATH', () => {
    expect(resolveExecutable(
      ['rg'],
      'win32',
      lookup({ 'where rg': 'C:\\Tools\\ripgrep\\rg.exe\r\n' }),
      () => false,
      () => true
    )).toBe('C:\\Tools\\ripgrep\\rg.exe')
  })

  it('treats Windows backslash candidates as explicit paths', () => {
    expect(resolveExecutable(
      ['C:\\Tools\\fd.exe'],
      'win32',
      lookup({}),
      (path) => path === 'C:\\Tools\\fd.exe',
      () => true
    )).toBe('C:\\Tools\\fd.exe')
  })

  it('keeps using which on non-Windows platforms', () => {
    expect(resolveExecutable(
      ['rg'],
      'darwin',
      lookup({ 'which rg': '/opt/homebrew/bin/rg\n' }),
      () => false,
      () => true
    )).toBe('/opt/homebrew/bin/rg')
  })
})

describe('shell runtime metadata', () => {
  it('normalizes shell display names', () => {
    expect(shellDisplayName('C:\\Windows\\System32\\cmd.exe')).toBe('cmd.exe')
    expect(shellDisplayName('C:\\Program Files\\PowerShell\\7\\pwsh.exe')).toBe('pwsh')
    expect(shellDisplayName('/bin/bash')).toBe('bash')
  })

  it('tracks conditional chaining support across Windows shell variants', () => {
    expect(shellSupportsConditionalChaining('C:\\Program Files\\PowerShell\\7\\pwsh.exe')).toBe(true)
    expect(shellSupportsConditionalChaining('C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe')).toBe(false)
    expect(shellSupportsConditionalChaining('/bin/bash')).toBe(true)
  })

  it('detects only unquoted shell conditional chaining sequences', () => {
    expect(hasUnquotedShellConditionalSequence('Write-Output a && Write-Output b')).toBe(true)
    expect(hasUnquotedShellConditionalSequence('Write-Output a || Write-Output b')).toBe(true)
    expect(hasUnquotedShellConditionalSequence('Write-Output "a && b"')).toBe(false)
    expect(hasUnquotedShellConditionalSequence("Write-Output 'a || b'")).toBe(false)
  })

  it('describes the syntax for the current shell', () => {
    expect(shellRuntimeInfo({ shell: 'C:\\Windows\\System32\\cmd.exe', args: ['/d', '/s', '/c'] })).toMatchObject({
      name: 'cmd.exe',
      syntax: 'cmd.exe batch'
    })
    const instruction = shellRuntimeInstruction({
      shell: 'C:\\Program Files\\PowerShell\\7\\pwsh.exe',
      args: ['-Command']
    })
    expect(instruction).toContain('<shell_environment>')
    expect(instruction).toContain('<shell>pwsh</shell>')
    expect(instruction).toContain('<syntax>PowerShell 7; conditional && and || chaining is supported</syntax>')
    // Factual block only: no imperative directives the model would echo back.
    expect(instruction).not.toMatch(/Do not assume|Write shell commands/)
  })

  it('runs PowerShell commands through a UTF-8 output preamble', () => {
    const args = shellCommandArgs(
      {
        shell: 'C:\\Program Files\\PowerShell\\7\\pwsh.exe',
        args: ['-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command']
      },
      'Write-Output "测试"'
    )

    expect(args.slice(0, -1)).toEqual([
      '-NoLogo',
      '-NoProfile',
      '-ExecutionPolicy',
      'Bypass',
      '-EncodedCommand'
    ])
    const script = Buffer.from(args.at(-1) ?? '', 'base64').toString('utf16le')
    expect(script).toContain('[Console]::OutputEncoding = $OutputEncoding')
    expect(script).toContain('Write-Output "测试"')
  })

  it('keeps non-PowerShell command arguments unchanged', () => {
    expect(shellCommandArgs({ shell: '/bin/bash', args: ['-lc'] }, 'echo hi')).toEqual(['-lc', 'echo hi'])
  })
})

describe('terminateSpawnTree', () => {
  it('uses taskkill to terminate process trees on Windows', () => {
    const calls: Array<{ command: string; args: string[] }> = []
    const child = {
      pid: 1234,
      kill: vi.fn()
    }
    const spawnImpl = vi.fn((command: string, args: string[]) => {
      calls.push({ command, args })
      return {
        once: vi.fn(),
        unref: vi.fn()
      }
    })

    terminateSpawnTree(child as never, {
      platform: 'win32',
      spawnImpl: spawnImpl as never
    })

    expect(calls).toEqual([{ command: 'taskkill', args: ['/pid', '1234', '/T', '/F'] }])
    expect(child.kill).not.toHaveBeenCalled()
  })

  it('falls back to child.kill when no pid is available', () => {
    const child = {
      kill: vi.fn()
    }

    terminateSpawnTree(child as never, { platform: 'win32' })

    expect(child.kill).toHaveBeenCalledWith('SIGTERM')
  })
})

describe('tool path normalization', () => {
  it('expands home-relative tool paths with POSIX and Windows separators', () => {
    expect(expandHomePath('~/Desktop', '/Users/alice')).toBe(join('/Users/alice', 'Desktop'))
    expect(expandHomePath('~\\Desktop', '/Users/alice')).toBe(join('/Users/alice', 'Desktop'))
    expect(expandHomePath('~other/Desktop', '/Users/alice')).toBe('~other/Desktop')
  })

  it('normalizes Windows separators in tool-facing paths', () => {
    expect(normalizeToolPath('src\\main\\index.ts')).toBe('src/main/index.ts')
  })

  it('normalizes ls relative paths', () => {
    const fileStat = statSync(new URL('../../tool-test-support/tool/builtin-tool-utils.ts', import.meta.url))
    const entry = makeListEntry('/workspace/src/index.ts', '/workspace', fileStat)

    expect(entry.relative_path).toBe('src/index.ts')
  })

  it('keeps external tool-facing paths absolute instead of workspace-escape relative paths', () => {
    expect(toolRelativePath('/workspace/project', '/workspace/project/src/index.ts')).toBe('src/index.ts')
    expect(toolRelativePath('/workspace/project', '/workspace/other/index.ts')).toBe('/workspace/other/index.ts')
  })

  it('resolves child-process relative output against the tool cwd', () => {
    const root = resolve('workspace-project-test')
    const absolute = resolve(root, '..', 'other', 'index.ts')
    expect(resolveToolOutputPath(root, 'src/index.ts')).toBe(resolve(root, 'src', 'index.ts'))
    expect(resolveToolOutputPath(root, absolute)).toBe(absolute)
  })

  it('resolves child-process relative output against an existing external search root', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-tool-output-'))
    try {
      const workspace = join(base, 'workspace')
      const external = join(base, 'external')
      await mkdir(join(external, 'nested'), { recursive: true })
      await writeFile(join(external, 'nested', 'report.txt'), 'report\n')

      expect(resolveToolOutputPath(workspace, 'nested/report.txt', external)).toBe(
        join(external, 'nested', 'report.txt')
      )
      await mkdir(join(workspace, 'nested'), { recursive: true })
      await writeFile(join(workspace, 'nested', 'report.txt'), 'workspace report\n')
      expect(resolveToolOutputPath(workspace, 'nested/report.txt', external)).toBe(
        join(workspace, 'nested', 'report.txt')
      )
      expect(resolveToolOutputPath(workspace, 'nested/report.txt', external, true)).toBe(
        join(external, 'nested', 'report.txt')
      )
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })

  it('expands home-relative readable paths before resolving against the workspace', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-tool-home-'))
    try {
      const workspace = join(base, 'workspace')
      await mkdir(workspace, { recursive: true })

      const resolved = await resolveReadablePath('~/Desktop/notes.txt', toolContext(workspace, 'danger-full-access'))

      expect(resolved.absolutePath).toBe(resolve(homedir(), 'Desktop', 'notes.txt'))
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })

  it('expands home-relative write paths under danger-full-access', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-tool-home-write-'))
    try {
      const workspace = join(base, 'workspace')
      await mkdir(workspace, { recursive: true })

      const resolved = await resolveWorkspacePath('~\\Desktop\\notes.txt', toolContext(workspace, 'danger-full-access'))

      const expected = resolve(homedir(), 'Desktop', 'notes.txt')
      expect(resolved.absolutePath).toBe(expected)
      expect(resolved.relativePath).toBe(normalizeToolPath(expected))
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })

  it('allows workspace escapes only under danger-full-access', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-tool-sandbox-'))
    try {
      const workspace = join(base, 'workspace')
      const outside = join(base, 'outside', 'notes.txt')
      await mkdir(workspace, { recursive: true })
      await mkdir(join(base, 'outside'), { recursive: true })
      await writeFile(outside, 'external')

      await expect(resolveWorkspacePath('../outside/notes.txt', toolContext(workspace))).rejects.toThrow(
        /escapes the workspace root/
      )
      await expect(resolveReadablePath(outside, toolContext(workspace))).rejects.toThrow(/escapes the workspace root/)

      await expect(resolveWorkspacePath('../outside/notes.txt', toolContext(workspace, 'danger-full-access')))
        .resolves.toMatchObject({ absolutePath: outside })
      await expect(resolveReadablePath(outside, toolContext(workspace, 'danger-full-access')))
        .resolves.toMatchObject({ absolutePath: outside })
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })
})
