import { describe, expect, it } from 'vitest'
import { bootstrapThread, makeHarness, makeSilentModel, makeFakeModel } from './loop-test-harness.js'
import { ContextCompactor } from '../src/shared/context-compactor.js'
import type { HookInvocation } from '../src/hooks/hook-engine.js'

function recordingHook(phase: HookInvocation['phase'], sink: HookInvocation[]) {
  return {
    phase,
    run: (invocation: HookInvocation) => {
      sink.push(invocation)
    }
  }
}

describe('agent loop lifecycle hooks', () => {
  it('fires TurnStart and TurnEnd with turn metadata', async () => {
    const seen: HookInvocation[] = []
    const h = makeHarness(makeSilentModel(), {
      hooks: [recordingHook('TurnStart', seen), recordingHook('TurnEnd', seen)]
    })
    await bootstrapThread(h, { workspace: '/tmp/ws', request: { prompt: 'lifecycle check' } })
    const status = await h.loop.runTurn(h.threadId, h.turnId)
    expect(status).toBe('completed')
    expect(seen).toHaveLength(2)
    expect(seen[0]).toMatchObject({
      phase: 'TurnStart',
      threadId: h.threadId,
      turnId: h.turnId,
      prompt: 'lifecycle check',
      workspace: '/tmp/ws'
    })
    expect(seen[1]).toMatchObject({
      phase: 'TurnEnd',
      threadId: h.threadId,
      turnId: h.turnId,
      status: 'completed'
    })
  })

  it('fails the turn when a UserPromptSubmit hook denies it', async () => {
    const h = makeHarness(makeSilentModel(), {
      hooks: [{ phase: 'UserPromptSubmit', run: () => ({ decision: 'deny', message: 'prompt rejected by gate' }) }]
    })
    await bootstrapThread(h)
    const status = await h.loop.runTurn(h.threadId, h.turnId)
    expect(status).toBe('failed')
    const items = await h.sessionStore.loadItems(h.threadId)
    const errorItem = items.find((item) => item.kind === 'error')
    expect(errorItem).toMatchObject({ kind: 'error', code: 'hook_denied', message: 'prompt rejected by gate' })
    const thread = await h.threadStore.get(h.threadId)
    const turn = thread?.turns.find((t) => t.id === h.turnId)
    expect(turn?.status).toBe('failed')
  })

  it('persists UserPromptSubmit additionalContext as a hook-context user message', async () => {
    const h = makeHarness(makeSilentModel(), {
      hooks: [{ phase: 'UserPromptSubmit', run: () => ({ additionalContext: 'deploy freeze until friday' }) }]
    })
    await bootstrapThread(h)
    const status = await h.loop.runTurn(h.threadId, h.turnId)
    expect(status).toBe('completed')
    const items = await h.sessionStore.loadItems(h.threadId)
    const injected = items.find(
      (item) => item.kind === 'user_message' && item.text.includes('<hook-context>')
    )
    expect(injected).toBeDefined()
    expect(injected && injected.kind === 'user_message' ? injected.text : '').toContain(
      'deploy freeze until friday'
    )
  })

  it('fires PreCompact when compaction is planned', async () => {
    const seen: HookInvocation[] = []
    const h = makeHarness(
      makeFakeModel([
        { kind: 'assistant_text_delta', text: 'done' },
        { kind: 'completed', stopReason: 'stop' }
      ]),
      {
        compactor: new ContextCompactor({ softThreshold: 1, hardThreshold: 2 }),
        hooks: [recordingHook('PreCompact', seen)]
      }
    )
    await bootstrapThread(h, { request: { prompt: 'long enough prompt to exceed a one-token threshold' } })
    await h.loop.runTurn(h.threadId, h.turnId)
    expect(seen.length).toBeGreaterThan(0)
    expect(seen[0]).toMatchObject({ phase: 'PreCompact', threadId: h.threadId, turnId: h.turnId })
    expect(seen[0].phase === 'PreCompact' ? seen[0].reason : '').toBeTruthy()
  })

  it('keeps the turn alive when an observer hook crashes and records a warning event', async () => {
    const h = makeHarness(makeSilentModel(), {
      hooks: [
        {
          phase: 'TurnStart',
          run: () => {
            throw new Error('observer exploded')
          }
        }
      ]
    })
    await bootstrapThread(h)
    const status = await h.loop.runTurn(h.threadId, h.turnId)
    expect(status).toBe('completed')
    const warning = h.bus
      .snapshotSince(h.threadId, 0)
      .find((event) => event.kind === 'error' && event.code === 'hook_warning')
    expect(warning).toBeDefined()
  })
})
