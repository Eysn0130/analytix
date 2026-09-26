import { beforeEach, expect, test, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
const mock=vi.hoisted(()=>({api:undefined as unknown, listeners:new Map<string,(...args:any[])=>void>(),invoke:vi.fn(async()=>({ok:true}))}))
vi.mock('electron',()=>({contextBridge:{exposeInMainWorld:(_name:string,api:unknown)=>{mock.api=api}},
  ipcRenderer:{on:(name:string,fn:(...args:any[])=>void)=>mock.listeners.set(name,fn),invoke:mock.invoke,removeListener:vi.fn()},webUtils:{getPathForFile:vi.fn()}}))
const requestId='00000000-0000-4000-8000-000000000001'
beforeEach(async()=>{vi.resetModules();mock.listeners.clear();mock.invoke.mockClear();await import('./index')})
const send=(phase='prepare',extra={})=>mock.listeners.get('write:shutdown-request')!({}, {requestId,phase,...extra})
test('resident listener fails closed until the renderer installs its handler',async()=>{
  send();await vi.waitFor(()=>expect(mock.invoke).toHaveBeenCalledWith('write:shutdown-ack',{requestId,phase:'prepare',outcome:{result:'blocked',reason:'unavailable'}}))
})
test('ACK follows actual asynchronous flush and cancel is not queued behind it',async()=>{
  let finish!:(value:{result:'ready'})=>void
  const handler=vi.fn(request=>request.phase==='cancel'?Promise.resolve({result:'ready' as const}):new Promise<{result:'ready'}>(resolve=>{finish=resolve}))
  const dispose=(mock.api as AnalytixApi).write.onShutdown(handler)
  send();expect(handler).toHaveBeenCalledOnce();expect(mock.invoke).not.toHaveBeenCalled()
  send('cancel');await vi.waitFor(()=>expect(mock.invoke).toHaveBeenCalledWith('write:shutdown-ack',expect.objectContaining({phase:'cancel'})))
  finish({result:'ready'});await vi.waitFor(()=>expect(mock.invoke).toHaveBeenCalledTimes(2));dispose()
  send();await vi.waitFor(()=>expect(mock.invoke).toHaveBeenLastCalledWith('write:shutdown-ack',expect.objectContaining({outcome:{result:'blocked',reason:'unavailable'}})))
})
test('rejects malformed requests and never reflects exceptions or unknown response fields',async()=>{
  send('prepare',{extra:'draft'});send('other');expect(mock.invoke).not.toHaveBeenCalled()
  const api=mock.api as AnalytixApi
  const dispose=api.write.onShutdown(async()=>{throw Error('private draft')});send()
  await vi.waitFor(()=>expect(mock.invoke).toHaveBeenCalledWith('write:shutdown-ack',expect.objectContaining({outcome:{result:'blocked',reason:'unavailable'}})));dispose()
  api.write.onShutdown(async()=>({result:'ready',content:'private draft'}) as any);send()
  await vi.waitFor(()=>expect(mock.invoke).toHaveBeenCalledTimes(2));expect(mock.invoke.mock.calls.at(-1)).not.toContain('private draft')
})
