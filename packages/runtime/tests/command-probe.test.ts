import { homedir } from 'node:os'
import { describe, expect, it } from 'vitest'
import {
  RuntimeCommandProbe,
  buildRuntimeEnvironmentInstruction
} from '../src/environment/command-probe.js'

describe('RuntimeCommandProbe', () => {
  it('returns sorted and sanitized command diagnostics', async () => {
    const calls: Array<{ file: string; args: readonly string[]; timeout: number }> = []
    const home = homedir()
    const probe = new RuntimeCommandProbe({
      commands: [
        'missing --version',
        'node --version',
        'slow --version',
        'python --version'
      ],
      timeoutMs: 123,
      execFile: async (file, args, options) => {
        calls.push({ file, args, timeout: options.timeout })
        if (file === 'node') return { stdout: 'v20.0.0\nignored', stderr: '' }
        if (file === 'python') return { stdout: `\u001b[32m${home}/bin/python\u001b[0m\n`, stderr: '' }
        if (file === 'missing') {
          throw Object.assign(new Error('spawn missing ENOENT'), { code: 'ENOENT' })
        }
        throw Object.assign(new Error('timed out'), { code: 'ETIMEDOUT' })
      }
    })

    await expect(probe.diagnostics()).resolves.toEqual([
      { command: 'node --version', binary: 'node', found: true, output: 'v20.0.0' },
      { command: 'python --version', binary: 'python', found: true, output: '~/bin/python' },
      { command: 'missing --version', binary: 'missing', found: false, error: 'not found' },
      { command: 'slow --version', binary: 'slow', found: false, error: 'timeout' }
    ])
    expect(calls).toEqual([
      { file: 'missing', args: ['--version'], timeout: 123 },
      { file: 'node', args: ['--version'], timeout: 123 },
      { file: 'slow', args: ['--version'], timeout: 123 },
      { file: 'python', args: ['--version'], timeout: 123 }
    ])
  })

  it('formats command diagnostics as model-facing runtime environment context', () => {
    const instruction = buildRuntimeEnvironmentInstruction({
      maxRenderedCommands: 1,
      commands: [
        { command: 'node --version', binary: 'node', found: true, output: 'v20.0.0' },
        { command: 'npm --version', binary: 'npm', found: true, output: '10.0.0' },
        { command: 'go version', binary: 'go', found: false, error: 'not found' },
        { command: 'docker --version', binary: 'docker', found: false, error: 'timeout' }
      ]
    })

    expect(instruction).toContain('Runtime environment diagnostics for this model request:')
    expect(instruction).toContain('Detected commands:')
    expect(instruction).toContain('- node: v20.0.0')
    expect(instruction).toContain('- ... 1 more detected commands omitted')
    expect(instruction).toContain('Unavailable commands:')
    expect(instruction).toContain('- go: not found')
    expect(instruction).toContain('- ... 1 more unavailable commands omitted')
    expect(instruction).toContain('Treat this block as environment context')
    expect(buildRuntimeEnvironmentInstruction({ commands: [] })).toBeNull()
  })

  it('rejects command probes resolved inside denied roots without executing them', async () => {
    const calls: string[] = []
    const probe = new RuntimeCommandProbe({
      commands: ['fake --version'],
      denyRoots: ['/workspace/project'],
      resolveCommand: (binary) => binary === 'fake' ? '/workspace/project/bin/fake' : undefined,
      execFile: async (file) => {
        calls.push(file)
        return { stdout: 'should not run', stderr: '' }
      }
    })

    await expect(probe.diagnostics()).resolves.toEqual([
      { command: 'fake --version', binary: 'fake', found: false, error: 'not trusted' }
    ])
    expect(calls).toEqual([])
  })

  it('uses trusted resolved command paths when a resolver is provided', async () => {
    const calls: string[] = []
    const probe = new RuntimeCommandProbe({
      commands: ['node --version'],
      resolveCommand: (binary) => binary === 'node' ? '/usr/local/bin/node' : undefined,
      execFile: async (file) => {
        calls.push(file)
        return { stdout: 'v20.0.0', stderr: '' }
      }
    })

    await expect(probe.diagnostics()).resolves.toEqual([
      { command: 'node --version', binary: 'node', found: true, output: 'v20.0.0' }
    ])
    expect(calls).toEqual(['/usr/local/bin/node'])
  })

  it('caches cloned diagnostics until the TTL expires', async () => {
    let now = 1_000
    let calls = 0
    const probe = new RuntimeCommandProbe({
      commands: ['node --version'],
      cacheTtlMs: 100,
      nowMs: () => now,
      execFile: async () => {
        calls += 1
        return { stdout: `v${calls}`, stderr: '' }
      }
    })

    const first = await probe.diagnostics()
    first[0]!.output = 'mutated'
    const cached = await probe.diagnostics()
    now = 1_100
    const refreshed = await probe.diagnostics()

    expect(calls).toBe(2)
    expect(cached).toEqual([
      { command: 'node --version', binary: 'node', found: true, output: 'v1' }
    ])
    expect(refreshed).toEqual([
      { command: 'node --version', binary: 'node', found: true, output: 'v2' }
    ])
  })

  it('coalesces concurrent diagnostics calls', async () => {
    let calls = 0
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const probe = new RuntimeCommandProbe({
      commands: ['node --version'],
      cacheTtlMs: 0,
      execFile: async () => {
        calls += 1
        await gate
        return { stdout: 'v20.0.0', stderr: '' }
      }
    })

    const first = probe.diagnostics()
    const second = probe.diagnostics()
    release()
    const [firstResult, secondResult] = await Promise.all([first, second])

    expect(calls).toBe(1)
    expect(firstResult).toEqual(secondResult)
    expect(firstResult[0]).not.toBe(secondResult[0])
  })
})
