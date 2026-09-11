import { describe, expect, it, vi } from 'vitest'
import { buildRouter } from './index.js'
import type { ServerRuntime } from './server-runtime.js'

describe('runtime routes', () => {
  it('starts the first turn and requests an automatic thread title', async () => {
    const startTurn = vi.fn(async () => ({
      threadId: 'thr_route',
      turnId: 'turn_route',
      userMessageItemId: 'item_turn_route_user'
    }))
    const generateForFirstUserPrompt = vi.fn(async () => undefined)
    const runTurn = vi.fn()
    const runtime = {
      runtimeToken: 'route-token',
      insecure: false,
      turnService: { startTurn },
      threadTitleService: { generateForFirstUserPrompt },
      runTurn
    } as unknown as ServerRuntime

    const router = buildRouter(runtime)
    const match = router.match('POST', '/v1/threads/thr_route/turns')
    expect(match).toBeDefined()

    const response = await match!.handler(
      new Request('http://runtime.test/v1/threads/thr_route/turns', {
        method: 'POST',
        headers: {
          authorization: 'Bearer route-token',
          'content-type': 'application/json'
        },
        body: JSON.stringify({
          prompt: 'Runtime prompt with hidden context',
          displayText: 'User visible first prompt',
          model: 'deepseek-title',
          async: true
        })
      }),
      { params: match!.params }
    )

    expect(response.status).toBe(202)
    expect(startTurn).toHaveBeenCalledWith({
      threadId: 'thr_route',
      request: expect.objectContaining({
        prompt: 'Runtime prompt with hidden context',
        displayText: 'User visible first prompt',
        model: 'deepseek-title'
      })
    })
    expect(startTurn).toHaveBeenCalledWith(expect.objectContaining({
      request: expect.objectContaining({ async: true })
    }))
    expect(generateForFirstUserPrompt).toHaveBeenCalledWith({
      threadId: 'thr_route',
      prompt: 'User visible first prompt',
      model: 'deepseek-title'
    })
    expect(runTurn).toHaveBeenCalledWith('thr_route', 'turn_route')
  })
})
