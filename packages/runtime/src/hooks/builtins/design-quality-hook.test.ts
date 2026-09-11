import { describe, expect, it } from 'vitest'
import type { QualityConfig } from '../../config/analytix-config.js'
import type { HookInvocation, HookResult } from '../hook-engine.js'
import { buildDesignQualityHook, matchesGlob } from './design-quality-hook.js'

const baseConfig: QualityConfig = {
  enabled: true,
  strictness: 'standard',
  ignoreRules: [],
  ignoreFiles: [],
  maxFindings: 12
}

function postToolUse(args: Record<string, unknown>, output: unknown, isError = false): HookInvocation {
  return {
    phase: 'PostToolUse',
    call: { callId: 'call_1', toolName: 'write', arguments: args },
    context: { threadId: 'thread_1', turnId: 'turn_1', workspace: '/ws', approvalPolicy: 'never' } as never,
    result: { output, isError }
  }
}

function runHook(config: QualityConfig, invocation: HookInvocation): HookResult | void {
  const hook = buildDesignQualityHook(config)
  if (!hook || !('run' in hook)) throw new Error('expected function hook')
  return hook.run(invocation) as HookResult | void
}

describe('matchesGlob', () => {
  it('matches ** across segments and * within one segment', () => {
    expect(matchesGlob('**/vendor/**', 'src/vendor/x.css')).toBe(true)
    expect(matchesGlob('*.css', 'a.css')).toBe(true)
    expect(matchesGlob('*.css', 'dir/a.css')).toBe(false)
    expect(matchesGlob('src/**/*.tsx', 'src/a/b/c.tsx')).toBe(true)
  })
})

describe('buildDesignQualityHook', () => {
  it('is disabled when quality is disabled', () => {
    expect(buildDesignQualityHook({ ...baseConfig, enabled: false })).toBeNull()
  })

  it('declares a PostToolUse hook for file-writing tools', () => {
    const hook = buildDesignQualityHook(baseConfig)
    expect(hook?.phase).toBe('PostToolUse')
    expect(hook?.toolNames).toEqual(['write', 'edit', 'write_file', 'edit_file'])
  })

  it('folds design findings into frontend tool output', () => {
    const content = '<div className="bg-gradient-to-r from-violet-500 to-blue-500">Launch</div>'
    const result = runHook(
      baseConfig,
      postToolUse({ path: '/ws/a.css', content }, { path: '/ws/a.css', relative_path: 'a.css' })
    )
    const review = (result as { output: Record<string, unknown> }).output.design_quality_review as {
      findings: unknown[]
      note: string
    }
    expect(review.findings.length).toBeGreaterThan(0)
    expect(review.note).toContain('Analytix')
    expect((result as { output: Record<string, unknown> }).output.relative_path).toBe('a.css')
  })

  it('skips clean, ignored, non-frontend, and errored results', () => {
    expect(
      runHook(
        baseConfig,
        postToolUse({ path: '/ws/a.css', content: 'body { color: #111; }' }, { path: '/ws/a.css', relative_path: 'a.css' })
      )
    ).toBeUndefined()
    expect(
      runHook(
        { ...baseConfig, ignoreFiles: ['vendor/**'] },
        postToolUse(
          { path: '/ws/vendor/a.css', content: '.hero { background: linear-gradient(90deg, purple, blue); }' },
          { path: '/ws/vendor/a.css', relative_path: 'vendor/a.css' }
        )
      )
    ).toBeUndefined()
    expect(
      runHook(
        baseConfig,
        postToolUse(
          { path: '/ws/a.ts', content: 'background: linear-gradient(90deg, purple, blue);' },
          { path: '/ws/a.ts', relative_path: 'a.ts' }
        )
      )
    ).toBeUndefined()
    expect(
      runHook(baseConfig, postToolUse({ path: '/ws/a.css', content: 'x' }, { error: 'nope' }, true))
    ).toBeUndefined()
  })
})
