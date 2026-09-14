import { beforeEach, expect, test, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
const electron=vi.hoisted(()=>({api:undefined as unknown,on:vi.fn(),invoke:vi.fn(),removeListener:vi.fn()}))
vi.mock('electron',()=>({contextBridge:{exposeInMainWorld:(_name:string,api:unknown)=>{electron.api=api}},ipcRenderer:{on:electron.on,invoke:electron.invoke,removeListener:electron.removeListener},webUtils:{getPathForFile:vi.fn()}}))
const freeze={requestId:'00000000-0000-4000-8000-000000000001',frozen:true}
const save={action:'annotationSave' as const,objectId:'a'.repeat(64),threadId:'thread',note:'last input',sourceRevision:'b'.repeat(64)}
beforeEach(()=>{vi.resetModules();vi.clearAllMocks();electron.invoke.mockResolvedValue({ok:true,view:null})})
async function setup(){
  await import('./index');const api=electron.api as AnalytixApi
  const listener=electron.on.mock.calls.find(([channel])=>channel==='office:annotation-input-freeze')![1]
  return {api,emit:(payload:unknown)=>listener({},payload)}
}
test('resident preload listener freezes synchronously before ACK and blocks later annotation writes',async()=>{
  const h=await setup(),order:string[]=[]
  const unsubscribe=h.api.office.onAnnotationInputFreeze(value=>order.push(`UI:${value}`))
  electron.invoke.mockImplementation(async(channel:string)=>{order.push(channel);return {ok:true,view:null}})
  await h.api.office.request(save)
  h.emit(freeze)
  expect(order).toEqual(['UI:false','office:request','UI:true','office:annotation-input-frozen'])
  expect(await h.api.office.request({...save,note:'forbidden'})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(electron.invoke.mock.calls.filter(([channel])=>channel==='office:request')).toHaveLength(1)
  h.emit({...freeze,frozen:false});expect(order.at(-2)).toBe('UI:false');expect(order.at(-1)).toBe('office:annotation-input-frozen')
  await h.api.office.request({...save,note:'after thaw'})
  expect(electron.invoke.mock.calls.filter(([channel])=>channel==='office:request')).toHaveLength(2)
  unsubscribe();h.emit(freeze);expect(order.at(-1)).toBe('office:annotation-input-frozen')
})
test('freeze survives component unmount and a newly mounted listener receives the current frozen state',async()=>{
  const h=await setup();h.emit(freeze)
  const changed=vi.fn(),unsubscribe=h.api.office.onAnnotationInputFreeze(changed)
  expect(changed).toHaveBeenCalledWith(true);unsubscribe()
  expect(await h.api.office.request(save)).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(electron.invoke).toHaveBeenCalledWith('office:annotation-input-frozen',freeze)
})
test('malformed requests and a failed synchronous UI freeze cannot emit a successful ACK',async()=>{
  const h=await setup()
  for(const payload of [{...freeze,extra:true},{...freeze,frozen:'true'},{...freeze,requestId:'invalid'}])h.emit(payload)
  expect(electron.invoke).not.toHaveBeenCalled()
  h.api.office.onAnnotationInputFreeze(value=>{if(value)throw Error('UI failed')})
  h.emit(freeze);expect(electron.invoke).not.toHaveBeenCalled()
  expect(await h.api.office.request(save)).toMatchObject({ok:false,error:'unsaved_changes'})
})
