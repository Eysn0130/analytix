// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { markPublicProjectionRevoked, resetPublicProjectionRevocationsForTests } from '../../../../lib/public-projection-revocation'
import { useTurnTiming } from './use-turn-timing'
const { detail } = vi.hoisted(() => ({ detail: vi.fn() }))
vi.mock('../../../../agent/registry', () => ({ getProvider: () => ({ getThreadDetail: detail }) }))
let root: Root, container: HTMLDivElement
function deferred() { let resolve!: (value: object) => void; const promise = new Promise<object>(done => { resolve = done }); return { promise, resolve } }
function Probe({ thread, settlement, supplied }: { thread: string; settlement: string; supplied?: {} }) {
  const facts = useTurnTiming(thread, settlement, supplied)
  return createElement('pre', null, JSON.stringify(facts))
}
async function render(thread: string, settlement = 'running', supplied?: {}) {
  await act(async () => root.render(createElement(Probe, { thread, settlement, supplied })))
}
beforeEach(() => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); detail.mockReset(); resetPublicProjectionRevocationsForTests(); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); resetPublicProjectionRevocationsForTests(); vi.unstubAllGlobals() })
describe('central timing detail generation', () => {
  it('does not publish stale A → B → A facts and reads once per settlement', async () => {
    const first = deferred(), middle = deferred(), latest = deferred()
    detail.mockReturnValueOnce(first.promise).mockReturnValueOnce(middle.promise).mockReturnValueOnce(latest.promise)
    await render('a'); await render('b'); await render('a')
    await act(async () => first.resolve({ authoritativeTurnStatuses: { old: 'completed' } }))
    await act(async () => middle.resolve({ authoritativeTurnStatuses: { middle: 'failed' } }))
    expect(container.textContent).not.toContain('old'); expect(container.textContent).not.toContain('middle')
    await act(async () => latest.resolve({ authoritativeTurnStatuses: { latest: 'aborted' } }))
    expect(container.textContent).toContain('aborted')
    await render('a'); expect(detail).toHaveBeenCalledTimes(3)
    detail.mockResolvedValueOnce({ authoritativeTurnStatuses: { latest: 'completed' } })
    await render('a', 'settled'); expect(detail).toHaveBeenCalledTimes(4)
    expect(container.textContent).toContain('completed')
  })
  it('rejects an in-flight result after revocation and skips supplied owner facts', async () => {
    const pending = deferred(); detail.mockReturnValueOnce(pending.promise)
    await render('revoked'); markPublicProjectionRevoked('revoked')
    await act(async () => pending.resolve({ authoritativeTurnStatuses: { forbidden: 'completed' } }))
    expect(container.textContent).not.toContain('forbidden')
    await render('owner', 'settled', {}); expect(detail).toHaveBeenCalledOnce()
  })
})
