import { describe, expect, it } from 'vitest'
import {
  hookMatchesTool,
  runObserverHooks,
  runPostToolUseHooks,
  runPreToolUseHooks,
  runUserPromptSubmitHooks,
  type ResolvedHook,
  type ToolHookContext
} from '../src/hooks/hook-engine.js'
import { resolveConfiguredHooks, HooksConfigSchema } from '../src/hooks/hook-config.js'
import { LocalToolHost, defaultLocalTools } from '../src/tool-test-support/tool/local-tool-host.js'

const context: ToolHookContext = {
  threadId: 'th',
  turnId: 'tu',
  workspace: '/tmp',
  approvalPolicy: 'on-request'
}

const call = (toolName: string, args: Record<string, unknown> = {}) => ({
  callId: 'c_1',
  toolName,
  arguments: args
})

describe('hookMatchesTool', () => {
  it('matches everything when no matcher or toolNames are set', () => {
    expect(hookMatchesTool({}, 'bash')).toBe(true)
  })

  it('matches exact toolNames', () => {
    expect(hookMatchesTool({ toolNames: ['bash'] }, 'bash')).toBe(true)
    expect(hookMatchesTool({ toolNames: ['bash'] }, 'read_file')).toBe(false)
  })

  it('matches glob patterns with * wildcard', () => {
    expect(hookMatchesTool({ matcher: 'mcp__*' }, 'mcp__github__create_issue')).toBe(true)
    expect(hookMatchesTool({ matcher: 'mcp__*' }, 'bash')).toBe(false)
  })

  it('matches | alternation', () => {
    expect(hookMatchesTool({ matcher: 'read_file|write_file' }, 'write_file')).toBe(true)
    expect(hookMatchesTool({ matcher: 'read_file|write_file' }, 'bash')).toBe(false)
  })

  it('escapes regex specials in glob patterns', () => {
    expect(hookMatchesTool({ matcher: 'a.b' }, 'a.b')).toBe(true)
    expect(hookMatchesTool({ matcher: 'a.b' }, 'axb')).toBe(false)
  })

  it('matches when either toolNames or matcher matches', () => {
    expect(hookMatchesTool({ toolNames: ['bash'], matcher: 'mcp__*' }, 'bash')).toBe(true)
    expect(hookMatchesTool({ toolNames: ['bash'], matcher: 'mcp__*' }, 'mcp__a')).toBe(true)
    expect(hookMatchesTool({ toolNames: ['bash'], matcher: 'mcp__*' }, 'read_file')).toBe(false)
  })
})

describe('runPreToolUseHooks', () => {
  it('chains argument rewrites so later hooks see earlier rewrites', async () => {
    const seen: unknown[] = []
    const hooks: ResolvedHook[] = [
      {
        phase: 'PreToolUse',
        run: (invocation) => {
          if (invocation.phase !== 'PreToolUse') return
          seen.push(invocation.call.arguments)
          return { arguments: { text: 'first' } }
        }
      },
      {
        phase: 'PreToolUse',
        run: (invocation) => {
          if (invocation.phase !== 'PreToolUse') return
          seen.push(invocation.call.arguments)
          return { arguments: { text: `${(invocation.call.arguments as { text: string }).text}+second` } }
        }
      }
    ]
    const outcome = await runPreToolUseHooks(hooks, { call: call('echo', { text: 'original' }), context })
    expect(seen).toEqual([{ text: 'original' }, { text: 'first' }])
    expect(outcome.call.arguments).toEqual({ text: 'first+second' })
    expect(outcome.denied).toBeUndefined()
  })

  it('stops the chain on deny and skips later hooks', async () => {
    let laterRan = false
    const hooks: ResolvedHook[] = [
      { phase: 'PreToolUse', run: () => ({ decision: 'deny', message: 'nope' }) },
      {
        phase: 'PreToolUse',
        run: () => {
          laterRan = true
        }
      }
    ]
    const outcome = await runPreToolUseHooks(hooks, { call: call('echo'), context })
    expect(outcome.denied).toBe('nope')
    expect(laterRan).toBe(false)
  })

  it('reports autoApproved on decision allow unless a later hook denies', async () => {
    const allowed = await runPreToolUseHooks(
      [{ phase: 'PreToolUse', run: () => ({ decision: 'allow' }) }],
      { call: call('echo'), context }
    )
    expect(allowed.autoApproved).toBe(true)
    const denied = await runPreToolUseHooks(
      [
        { phase: 'PreToolUse', run: () => ({ decision: 'allow' }) },
        { phase: 'PreToolUse', run: () => ({ decision: 'deny', message: 'blocked' }) }
      ],
      { call: call('echo'), context }
    )
    expect(denied.denied).toBe('blocked')
    expect(denied.autoApproved).toBe(false)
  })

  it('only runs hooks whose matcher matches the tool', async () => {
    let ran = false
    const hooks: ResolvedHook[] = [
      {
        phase: 'PreToolUse',
        matcher: 'mcp__*',
        run: () => {
          ran = true
        }
      }
    ]
    await runPreToolUseHooks(hooks, { call: call('bash'), context })
    expect(ran).toBe(false)
  })

  it('propagates function hook timeouts', async () => {
    const hooks: ResolvedHook[] = [
      {
        phase: 'PreToolUse',
        timeoutMs: 20,
        run: () => new Promise(() => undefined)
      }
    ]
    await expect(runPreToolUseHooks(hooks, { call: call('echo'), context })).rejects.toThrow(/timed out/)
  })
})

describe('runPostToolUseHooks', () => {
  it('chains output rewrites so later hooks see earlier rewrites', async () => {
    const hooks: ResolvedHook[] = [
      {
        phase: 'PostToolUse',
        run: (invocation) => {
          if (invocation.phase !== 'PostToolUse') return
          return { output: { layer: 1, inner: invocation.result.output } }
        }
      },
      {
        phase: 'PostToolUse',
        run: (invocation) => {
          if (invocation.phase !== 'PostToolUse') return
          return { output: { layer: 2, inner: invocation.result.output } }
        }
      }
    ]
    const outcome = await runPostToolUseHooks(hooks, {
      call: call('echo'),
      context,
      result: { output: 'raw' }
    })
    expect(outcome.output).toEqual({ layer: 2, inner: { layer: 1, inner: 'raw' } })
  })

  it('lets a hook flip isError without replacing output', async () => {
    const outcome = await runPostToolUseHooks(
      [{ phase: 'PostToolUse', run: () => ({ isError: true }) }],
      { call: call('echo'), context, result: { output: 'raw' } }
    )
    expect(outcome.output).toBe('raw')
    expect(outcome.isError).toBe(true)
  })
})

describe('runUserPromptSubmitHooks', () => {
  const payload = { threadId: 'th', turnId: 'tu', prompt: 'do the thing' }

  it('collects additionalContext from multiple hooks', async () => {
    const outcome = await runUserPromptSubmitHooks(
      [
        { phase: 'UserPromptSubmit', run: () => ({ additionalContext: 'ctx one' }) },
        { phase: 'UserPromptSubmit', run: () => ({ additionalContext: 'ctx two' }) }
      ],
      payload
    )
    expect(outcome.denied).toBeUndefined()
    expect(outcome.additionalContext).toEqual(['ctx one', 'ctx two'])
  })

  it('denies the turn with the hook message', async () => {
    const outcome = await runUserPromptSubmitHooks(
      [{ phase: 'UserPromptSubmit', run: () => ({ decision: 'deny', message: 'not now' }) }],
      payload
    )
    expect(outcome.denied).toBe('not now')
  })

  it('fails open with a warning when a hook crashes', async () => {
    const outcome = await runUserPromptSubmitHooks(
      [
        {
          phase: 'UserPromptSubmit',
          run: () => {
            throw new Error('boom')
          }
        },
        { phase: 'UserPromptSubmit', run: () => ({ additionalContext: 'still here' }) }
      ],
      payload
    )
    expect(outcome.denied).toBeUndefined()
    expect(outcome.additionalContext).toEqual(['still here'])
    expect(outcome.warnings.some((warning) => warning.includes('boom'))).toBe(true)
  })
})

describe('runObserverHooks', () => {
  it('turns crashes into warnings', async () => {
    const outcome = await runObserverHooks(
      [
        {
          phase: 'TurnEnd',
          run: () => {
            throw new Error('observer down')
          }
        }
      ],
      { phase: 'TurnEnd', threadId: 'th', turnId: 'tu', status: 'completed' }
    )
    expect(outcome.warnings.some((warning) => warning.includes('observer down'))).toBe(true)
  })
})

describe('command hooks', () => {
  it('parses JSON stdout from a command hook (exit 0)', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'PreToolUse',
        command: `node -e "console.log(JSON.stringify({ arguments: { text: 'patched-by-command' } }))"`
      }
    ])
    const outcome = await runPreToolUseHooks(hooks, { call: call('echo', { text: 'original' }), context })
    expect(outcome.call.arguments).toEqual({ text: 'patched-by-command' })
  })

  it('denies on exit code 2 with stderr as the reason', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'PreToolUse',
        command: `node -e "console.error('blocked by policy'); process.exit(2)"`
      }
    ])
    const outcome = await runPreToolUseHooks(hooks, { call: call('echo'), context })
    expect(outcome.denied).toBe('blocked by policy')
  })

  it('treats other non-zero exits as non-blocking warnings', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'PreToolUse',
        command: `node -e "console.error('flaky'); process.exit(1)"`
      }
    ])
    const outcome = await runPreToolUseHooks(hooks, { call: call('echo'), context })
    expect(outcome.denied).toBeUndefined()
    expect(outcome.warnings).toEqual(['flaky'])
  })

  it('feeds the invocation to the command on stdin', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'PreToolUse',
        command: `node -e "let raw=''; process.stdin.on('data', (c) => raw += c); process.stdin.on('end', () => { const inv = JSON.parse(raw); console.log(JSON.stringify({ arguments: { echoedTool: inv.call.toolName } })) })"`
      }
    ])
    const outcome = await runPreToolUseHooks(hooks, { call: call('my_tool'), context })
    expect(outcome.call.arguments).toEqual({ echoedTool: 'my_tool' })
  })

  it('turns plain stdout into additionalContext for UserPromptSubmit', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'UserPromptSubmit',
        command: `node -e "console.log('remember: deploy freeze today')"`
      }
    ])
    const outcome = await runUserPromptSubmitHooks(hooks, {
      threadId: 'th',
      turnId: 'tu',
      prompt: 'ship it'
    })
    expect(outcome.additionalContext).toEqual(['remember: deploy freeze today'])
  })

  it('kills timed-out command hooks and propagates the timeout for tool phases', async () => {
    const hooks = resolveConfiguredHooks([
      {
        phase: 'PreToolUse',
        timeoutMs: 150,
        command: `node -e "setTimeout(() => undefined, 60000)"`
      }
    ])
    await expect(runPreToolUseHooks(hooks, { call: call('echo'), context })).rejects.toThrow(/timed out/)
  })
})

describe('hooks config schema', () => {
  it('accepts command hook entries and rejects unknown keys', () => {
    expect(
      HooksConfigSchema.safeParse([
        { phase: 'PreToolUse', matcher: 'bash|write_file', command: './check.sh', timeoutMs: 1000 }
      ]).success
    ).toBe(true)
    expect(HooksConfigSchema.safeParse([{ phase: 'PreToolUse', command: 'x', nope: true }]).success).toBe(false)
    expect(HooksConfigSchema.safeParse([{ phase: 'NotAPhase', command: 'x' }]).success).toBe(false)
  })
})

describe('LocalToolHost hook integration', () => {
  it('skips approval when a PreToolUse hook returns decision allow', async () => {
    const guarded = LocalToolHost.defineTool({
      name: 'guarded',
      description: 'always asks',
      inputSchema: { type: 'object', properties: {}, required: [] },
      policy: 'on-request',
      execute: async () => ({ output: { ok: true } })
    })
    const host = new LocalToolHost({
      tools: [...defaultLocalTools, guarded],
      hooks: [{ phase: 'PreToolUse', toolNames: ['guarded'], run: () => ({ decision: 'allow' }) }]
    })
    const result = await host.execute(
      { callId: 'c_allow', toolName: 'guarded', arguments: {} },
      {
        threadId: 'th',
        turnId: 'tu',
        workspace: '/tmp',
        approvalPolicy: 'on-request',
        abortSignal: new AbortController().signal,
        awaitApproval: async () => {
          throw new Error('approval should have been skipped')
        }
      }
    )
    expect(result.item).toMatchObject({ kind: 'tool_result', output: { ok: true } })
    expect(result.approved).toBe(true)
  })
})
