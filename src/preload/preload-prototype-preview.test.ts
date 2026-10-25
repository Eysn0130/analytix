import { beforeEach, expect, test, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
const mock = vi.hoisted(() => ({ api: undefined as unknown, invoke: vi.fn(async (_channel: string, _payload: unknown): Promise<unknown> => ({ ok: false, message: 'unavailable' })) }))
vi.mock('electron', () => ({ contextBridge: { exposeInMainWorld: (_name: string, api: unknown) => { mock.api = api } },
  ipcRenderer: { on: vi.fn(), invoke: mock.invoke, removeListener: vi.fn() }, webUtils: { getPathForFile: vi.fn() } }))
beforeEach(async () => { vi.resetModules(); mock.invoke.mockReset(); await import('./index') })
test('authorizes only strict selector requests and validates the returned isolated grant', async () => {
  const request = { protectedPresent: { threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64),
    resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0 }, mode: 'workspace-html' as const }
  const id = 'a'.repeat(48), grant = { ok: true, absolutePath: '/workspace/current.html',
    fileUrl: 'analytix-prototype://' + id + '/current.html', grantId: id, partition: 'analytix-proto-' + id }
  const api = (mock.api as AnalytixApi).write
  expect(await api.authorizeWritePrototype({ ...request, path: '/forged' } as never)).toMatchObject({ ok: false })
  expect(mock.invoke).not.toHaveBeenCalled()
  mock.invoke.mockResolvedValueOnce(grant)
  expect(await api.authorizeWritePrototype(request)).toEqual(grant)
  expect(mock.invoke).toHaveBeenCalledExactlyOnceWith('write:authorize-prototype', request)
  mock.invoke.mockResolvedValueOnce({ ...grant, fileUrl: 'file:///workspace/current.html' })
  expect(await api.authorizeWritePrototype(request)).toMatchObject({ ok: false })
})
