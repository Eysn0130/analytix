import { chmod, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import type { ToolHostContext } from '../../ports/tool-host.js'
import { createFindLocalTool, searchRelativePath } from '../../tool-test-support/tool/builtin-search-tools.js'

function toolContext(workspace: string): ToolHostContext {
  return {
    threadId: 'thread_find',
    turnId: 'turn_find',
    workspace,
    approvalPolicy: 'auto',
    sandboxMode: 'danger-full-access',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow'
  }
}

describe('builtin search tools', () => {
  it('uses workspace-relative paths for workspace searches and root-relative paths for external searches', () => {
    expect(searchRelativePath('/workspace/project', '/workspace/project/src', '/workspace/project/src/app.ts')).toBe(
      'src/app.ts'
    )
    expect(searchRelativePath('/workspace/project', '/external/docs', '/external/docs/nested/report.txt')).toBe(
      'nested/report.txt'
    )
  })

  it('resolves relative fd output against the tool workspace root', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-find-fd-'))
    try {
      const workspace = join(base, 'workspace')
      const bin = join(base, 'bin')
      const fakeFd = join(bin, process.platform === 'win32' ? 'fd.cmd' : 'fd')
      await mkdir(join(workspace, 'nested'), { recursive: true })
      await mkdir(bin, { recursive: true })
      await writeFile(join(workspace, 'nested', 'report.txt'), 'report\n')
      if (process.platform === 'win32') {
        await writeFile(fakeFd, '@echo off\r\nif "%1"=="--version" exit /b 0\r\necho nested\\report.txt\r\n')
      } else {
        await writeFile(fakeFd, '#!/bin/sh\nif [ "$1" = "--version" ]; then exit 0; fi\nprintf "nested/report.txt\\n"\n')
        await chmod(fakeFd, 0o755)
      }

      const tool = createFindLocalTool({ fdExecutableCandidates: [fakeFd], rgExecutableCandidates: [] })
      const result = await tool.execute({ pattern: '*.txt', path: '.', limit: 5 }, toolContext(workspace))
      const output = result.output as {
        matches: Array<{ path: string; relative_path: string }>
      }

      expect(result.isError).toBeFalsy()
      expect(output.matches).toEqual([{
        path: join(workspace, 'nested', 'report.txt'),
        relative_path: 'nested/report.txt'
      }])
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })

  it('resolves relative fd output against an external search root when needed', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-find-fd-external-'))
    try {
      const workspace = join(base, 'workspace')
      const external = join(base, 'external')
      const bin = join(base, 'bin')
      const fakeFd = join(bin, process.platform === 'win32' ? 'fd.cmd' : 'fd')
      await mkdir(workspace, { recursive: true })
      await mkdir(join(external, 'nested'), { recursive: true })
      await mkdir(bin, { recursive: true })
      await mkdir(join(workspace, 'nested'), { recursive: true })
      await writeFile(join(workspace, 'nested', 'report.txt'), 'workspace report\n')
      await writeFile(join(external, 'nested', 'report.txt'), 'report\n')
      if (process.platform === 'win32') {
        await writeFile(fakeFd, '@echo off\r\nif "%1"=="--version" exit /b 0\r\necho nested\\report.txt\r\n')
      } else {
        await writeFile(fakeFd, '#!/bin/sh\nif [ "$1" = "--version" ]; then exit 0; fi\nprintf "nested/report.txt\\n"\n')
        await chmod(fakeFd, 0o755)
      }

      const tool = createFindLocalTool({ fdExecutableCandidates: [fakeFd], rgExecutableCandidates: [] })
      const result = await tool.execute({ pattern: '*.txt', path: external, limit: 5 }, toolContext(workspace))
      const output = result.output as {
        matches: Array<{ path: string; relative_path: string }>
      }

      expect(result.isError).toBeFalsy()
      expect(output.matches).toEqual([{
        path: join(external, 'nested', 'report.txt'),
        relative_path: 'nested/report.txt'
      }])
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })

  it('matches external scan results relative to the external search root', async () => {
    const base = await mkdtemp(join(tmpdir(), 'analytix-find-external-'))
    try {
      const workspace = join(base, 'workspace')
      const external = join(base, 'external')
      await mkdir(workspace, { recursive: true })
      await mkdir(join(external, 'nested'), { recursive: true })
      await writeFile(join(external, 'nested', 'report.txt'), 'report\n')

      const tool = createFindLocalTool({ fdExecutableCandidates: [], rgExecutableCandidates: [] })
      const result = await tool.execute({ pattern: 'nested/*.txt', path: external, limit: 5 }, toolContext(workspace))
      const output = result.output as {
        matches: Array<{ path: string; relative_path: string }>
      }

      expect(result.isError).toBeFalsy()
      expect(output.matches).toEqual([{
        path: join(external, 'nested', 'report.txt'),
        relative_path: 'nested/report.txt'
      }])
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })
})
