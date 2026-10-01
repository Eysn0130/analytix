import { beforeEach, expect, test, vi } from 'vitest'
const mocked=vi.hoisted(()=>({load:vi.fn()}))
vi.mock('./office-assets',()=>({loadPrivateOfficeBuffers:mocked.load,officeAssetFailure:()=>Error('office-assets-unavailable')}))
import { createOfficePrivateAdmissionProvider, isOfficePrivateAdmission, loadAdmittedOfficeBuffers, type OfficePrivateAdmission } from './office-private-admission'
import { officePrivateAdmissionPath } from '../../../packages/runtime/src/contracts/office-private-admission'
const resources='/Applications/analytix.app/Contents/Resources',root=resources+'/office-private',qualificationDigest='a'.repeat(64)
beforeEach(()=>{vi.clearAllMocks();mocked.load.mockImplementation(async()=>({buffers:new Map([['/asset',{bytes:Buffer.from('synthetic'),mime:'text/plain'}]]),preloadPath:root+'/preload/office.cjs'}))})
function setup(){const transport=vi.fn(async(_path:string,body:string)=>({ok:true,status:200,body:JSON.stringify({ok:true,requestId:JSON.parse(body).requestId,root,qualificationDigest})}));return {transport,provide:createOfficePrivateAdmissionProvider(transport,resources)}}
test('only current nonce-bound Core replies mint tokens and surface loading rechecks before and after held reads',async()=>{
  const h=setup(),token=await h.provide();expect(isOfficePrivateAdmission(token)).toBe(true)
  const loaded=await loadAdmittedOfficeBuffers(token)
  expect(h.transport).toHaveBeenCalledTimes(3);expect(mocked.load).toHaveBeenCalledWith(root,qualificationDigest)
  const requests=h.transport.mock.calls.map(([path,body])=>{expect(path).toBe(officePrivateAdmissionPath);return JSON.parse(body)})
  expect(requests.every(request=>Object.keys(request).join(',')==='requestId')).toBe(true)
  expect(new Set(requests.map(request=>request.requestId)).size).toBe(3)
  expect(loaded.buffers.size).toBe(1)
  expect(isOfficePrivateAdmission({...token})).toBe(false)
  await expect(loadAdmittedOfficeBuffers({} as OfficePrivateAdmission)).rejects.toThrow('office-assets-unavailable')
})
test.each(['nonce','path','extra','duplicate','status','error','oversize'])('rejects %s without reading local resources or echoing paths',async(mode)=>{
  const h=setup()
  h.transport.mockImplementation(async(_path,body)=>{
    const value={ok:true,requestId:JSON.parse(body).requestId,root,qualificationDigest}
    if(mode==='nonce')value.requestId='00000000-0000-4000-8000-000000000000'
    if(mode==='path')value.root='/private/foreign/path'
    if(mode==='extra')Object.assign(value,{allowPackaged:true})
    const raw=JSON.stringify(value)
    return {ok:mode!=='error',status:mode==='status'?201:200,body:mode==='oversize'?' '.repeat(65537):mode==='duplicate'?raw.replace('"ok":true','"ok":true,"ok":true'):raw}
  })
  await expect(h.provide()).rejects.toThrow(/^office-assets-unavailable$/)
  expect(mocked.load).not.toHaveBeenCalled()
})
test.each(['before','after'])('current Core revocation %s load prevents startup and clears loaded buffers',async(phase)=>{
  const h=setup(),token=await h.provide(),buffers=new Map([['/asset',{bytes:Buffer.from('synthetic'),mime:'text/plain'}]])
  mocked.load.mockResolvedValue({buffers,preloadPath:root+'/preload/office.cjs'})
  const original=h.transport.getMockImplementation()!
  h.transport.mockImplementation(async(path,body)=>{
    const result=await original(path,body)
    if(phase==='before'||h.transport.mock.calls.length===3)result.body=JSON.stringify({...JSON.parse(result.body),qualificationDigest:'b'.repeat(64)})
    return result
  })
  await expect(loadAdmittedOfficeBuffers(token)).rejects.toThrow('office-assets-unavailable')
  if(phase==='before')expect(mocked.load).not.toHaveBeenCalled();else expect(buffers.size).toBe(0)
})
