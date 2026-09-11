import { describe, expect, it } from 'vitest'
import { canWritePath, sandboxBlockForTool } from '../../tool-test-support/tool/sandbox-policy.js'

describe('sandbox policy', () => {
  it('allows external file mutations under danger-full-access', () => {
    const context = {
      workspace: '/repo/workspace',
      sandboxMode: 'danger-full-access' as const
    }

    expect(canWritePath('/repo/other/app.ts', context)).toEqual({ ok: true })
  })

  it('limits workspace-write file mutations to the workspace', () => {
    const context = {
      workspace: '/repo/workspace',
      sandboxMode: 'workspace-write' as const
    }

    expect(canWritePath('/repo/workspace/src/app.ts', context)).toEqual({ ok: true })
    expect(canWritePath('/repo/other/app.ts', context)).toMatchObject({
      ok: false,
      block: {
        code: 'sandbox_write_blocked'
      }
    })
  })

  it('keeps command execution blocked in workspace-write mode', () => {
    expect(sandboxBlockForTool(
      { name: 'bash', toolKind: 'command_execution' },
      { sandboxMode: 'workspace-write' }
    )).toMatchObject({
      code: 'sandbox_command_blocked'
    })
  })
})
