import { expect, it, vi } from 'vitest'
import { updateThread } from './threads.js'
import type { ThreadService } from '../../services-test-support/thread-service.js'

it.each([true, false])('does not silently accept archived=%s in the diagnostic TS oracle', async archived => {
  const update = vi.fn()
  const response = await updateThread({ update } as unknown as ThreadService, 'thread', new Request('https://fixture.invalid/thread', {
    method: 'PATCH', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ archived })
  }))
  expect(response.status).toBe(409)
  const body = response instanceof Response ? await response.json() : JSON.parse(response.body)
  expect(body).toMatchObject({ code: 'thread_archive_unavailable' })
  expect(update).not.toHaveBeenCalled()
})

it('rejects formal Side promotion in the diagnostic TS oracle before any update', async () => {
  const update = vi.fn()
  const response = await updateThread({ update } as unknown as ThreadService, 'thread', new Request('https://fixture.invalid/thread', {
    method: 'PATCH', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ promoteSide: true })
  }))
  expect(response.status).toBe(409)
  const body = response instanceof Response ? await response.json() : JSON.parse(response.body)
  expect(body).toEqual({ code: 'conflict', message: 'Side promotion requires the Go runtime' })
  expect(update).not.toHaveBeenCalled()
})
