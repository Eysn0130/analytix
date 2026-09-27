import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { runBoundedDiagnosticChild } from '../../../scripts/lib/bounded-diagnostic-child.mjs'

const roots: string[] = []
afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true })
})

describe('bounded packaged diagnostic child', () => {
  it.skipIf(process.platform !== 'darwin')('rejects an oversized output bound before spawning', async () => {
    await expect(runBoundedDiagnosticChild({
      command: process.execPath,
      args: ['/not/executed.mjs'], cwd: tmpdir(), env: process.env,
      timeoutMs: 1_000, maxBuffer: 64 * 1024 * 1024 + 1,
      allowedResidualCommand: () => false
    })).rejects.toThrow('diagnostic_child_bound_invalid')
  })

  it.skipIf(process.platform !== 'darwin')('stops an exact timed-out child and its owned descendant', async () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-bounded-child-'))
    roots.push(root)
    const grandchild = join(root, 'grandchild.mjs')
    const child = join(root, 'child.mjs')
    writeFileSync(grandchild, 'setInterval(() => {}, 1000)\n')
    writeFileSync(child,
      `import { spawn } from 'node:child_process'\n` +
      `spawn(process.execPath, [${JSON.stringify(grandchild)}, '--fixture'], { stdio: 'ignore' })\n` +
      `setInterval(() => {}, 1000)\n`)
    let failure: any = null
    try {
      await runBoundedDiagnosticChild({
        command: process.execPath,
        args: [child, '--fixture'], cwd: root, env: process.env,
        timeoutMs: 1_000, maxBuffer: 1_024,
        allowedResidualCommand: (command) =>
          command.startsWith(`${process.execPath} ${grandchild} --fixture`)
      })
    } catch (error) { failure = error }
    expect(failure?.message).toBe('diagnostic_child_timeout')
    expect(failure?.cleanup).toEqual({ verified: true, reason: '' })
  })
})
