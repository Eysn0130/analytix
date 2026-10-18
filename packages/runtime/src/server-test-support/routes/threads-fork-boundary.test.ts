import { expect, it, vi } from 'vitest'
import { forkThread } from './threads.js'
import type { ThreadService } from '../../services-test-support/thread-service.js'

it('does not silently ignore a Footer boundary in the diagnostic TS oracle', async () => {
  const fork = vi.fn()
  for (const body of [{ turnId: 'turn', endSeq: 13 }, { turnId: 'turn' }]) {
    const response = await forkThread({ fork } as unknown as ThreadService, 'thread', new Request('https://fixture.invalid/fork', {
      method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(body)
    }))
    expect(response.status).toBe(409)
    expect(JSON.parse(response.body)).toMatchObject({ code: 'fork_boundary_unavailable' })
  }
  expect(fork).not.toHaveBeenCalled()
})
