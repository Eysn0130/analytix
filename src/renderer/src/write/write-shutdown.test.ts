import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { createWriteShutdownHandler, isWriteShutdownFrozen, registerWriteShutdownInput } from './write-shutdown'
import { useWriteWorkspaceStore } from './write-workspace-store'
import type { ObjectEditingRequest } from '../../../../packages/runtime/src/contracts/object-editing'

const workspace='/synthetic/shutdown', path=workspace+'/draft.md', objectId='b'.repeat(64), sessionId='a'.repeat(48)
const randomUUID=()=>crypto.randomUUID()
let hash:(text:string)=>string
let disk:string, request:ReturnType<typeof vi.fn<(input:ObjectEditingRequest)=>Promise<any>>>, currentHandler:ReturnType<typeof createWriteShutdownHandler>, currentId:string
const disposers:Array<()=>void>=[]
beforeEach(async()=>{
  const {createHash}=await vi.importActual<{createHash:(algorithm:string)=>{update:(text:string)=>{digest:(encoding:string)=>string}}}>('node:crypto')
  hash=(text:string)=>createHash('sha256').update(text).digest('hex')
  disk='original'
  request=vi.fn(async(input:ObjectEditingRequest):Promise<any>=>{
    if(input.action==='open')return {ok:true,document:{sessionId,objectId,path,content:disk,revision:hash(disk)}}
    if(input.action==='commit') { expect(input.baseRevision).toBe(hash(disk));disk=input.content;return {ok:true,receipt:{operationId:input.operationId,revision:hash(disk),status:'committed',savedAt:'2026-09-15T00:00:00Z'}} }
    throw Error('unexpected operation')
  })
  vi.stubGlobal('window',{analytix:{objects:{request}}})
  useWriteWorkspaceStore.getState().resetWorkspace();useWriteWorkspaceStore.setState({workspaceRoot:workspace})
  await useWriteWorkspaceStore.getState().openFile(workspace,path)
  currentHandler=createWriteShutdownHandler();currentId=randomUUID()
})
afterEach(async()=>{
  await currentHandler({requestId:currentId,phase:'cancel'})
  for(const dispose of disposers.splice(0))dispose()
  useWriteWorkspaceStore.getState().resetWorkspace();vi.unstubAllGlobals()
})
const prepare=()=>currentHandler({requestId:currentId,phase:'prepare'})
const cancel=()=>currentHandler({requestId:currentId,phase:'cancel'})
test('flushes the latest frozen draft after an earlier save settles, preserving exact Core revisions',async()=>{
  useWriteWorkspaceStore.getState().setFileContent('first')
  const implementation=request.getMockImplementation()!;let finish!:()=>void
  request.mockImplementationOnce(input=>new Promise(resolve=>{finish=()=>{void implementation(input).then(resolve)}}))
  const earlier=useWriteWorkspaceStore.getState().flushSave(workspace)
  useWriteWorkspaceStore.getState().setFileContent('last key')
  const preparing=prepare()
  expect(isWriteShutdownFrozen()).toBe(true)
  useWriteWorkspaceStore.getState().setFileContent('forbidden after freeze')
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('last key')
  finish();expect(await earlier).toBe(false);expect(await preparing).toEqual({result:'ready'})
  expect(disk).toBe('last key');expect(request.mock.calls.filter(([i])=>i.action==='commit')).toHaveLength(2)
  expect(useWriteWorkspaceStore.getState()).toMatchObject({pendingSave:null,saveStatus:'saved',shutdownFrozen:true})
  await cancel();useWriteWorkspaceStore.getState().setFileContent('after cancel');expect(useWriteWorkspaceStore.getState().fileContent).toBe('after cancel')
})
test('cancel does not wait for receipt and a late completion cannot release a newer freeze or authorize the old close',async()=>{
  useWriteWorkspaceStore.getState().setFileContent('saved attempt')
  const implementation=request.getMockImplementation()!;let finish!:()=>void
  request.mockImplementationOnce(input=>new Promise(resolve=>{finish=()=>{void implementation(input).then(resolve)}}))
  const first=prepare();await vi.waitFor(()=>expect(typeof finish).toBe('function'));await cancel()
  expect(isWriteShutdownFrozen()).toBe(false)
  useWriteWorkspaceStore.getState().setFileContent('new input after timeout')
  currentId=randomUUID();const second=prepare();finish()
  expect(await first).toEqual({result:'blocked',reason:'unavailable'})
  expect(await second).toEqual({result:'ready'});expect(disk).toBe('new input after timeout');expect(isWriteShutdownFrozen()).toBe(true)
})
test('active IME blocks without invoking freeze, blur or any commit',async()=>{
  const freeze=vi.fn();disposers.push(registerWriteShutdownInput({composing:()=>true,freeze}));freeze.mockClear()
  useWriteWorkspaceStore.getState().setFileContent('组合输入')
  expect(await prepare()).toEqual({result:'blocked',reason:'composing'});expect(freeze).not.toHaveBeenCalled();expect(isWriteShutdownFrozen()).toBe(false)
  expect(request.mock.calls.map(([i])=>i.action)).toEqual(['open']);expect(useWriteWorkspaceStore.getState().fileContent).toBe('组合输入')
})
test('export hold is neither awaited nor released by an attempted quit',async()=>{
  useWriteWorkspaceStore.getState().setFileContent('export draft')
  const hold=useWriteWorkspaceStore.getState().beginExport()
  expect(await prepare()).toEqual({result:'blocked',reason:'exporting'})
  expect(useWriteWorkspaceStore.getState()).toMatchObject({exportInProgress:true,shutdownFrozen:false,fileContent:'export draft'})
  expect(request.mock.calls.map(([i])=>i.action)).toEqual(['open']);hold.release()
})
test.each(['conflict','review'] as const)('%s blocks without clearing the retained draft',async kind=>{
  useWriteWorkspaceStore.getState().setFileContent('retained')
  useWriteWorkspaceStore.setState(kind==='conflict'?{saveStatus:'conflict'}:{reviewActive:true})
  expect(await prepare()).toEqual({result:'blocked',reason:'conflict'})
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('retained');expect(isWriteShutdownFrozen()).toBe(false)
})
test('unknown receipt keeps the exact attempt; later explicit quit queries it without an extra commit',async()=>{
  useWriteWorkspaceStore.getState().setFileContent('unknown receipt draft')
  const implementation=request.getMockImplementation()!;let receipt:any
  request.mockImplementationOnce(async input=>{receipt=(await implementation(input)).receipt;throw Error('lost receipt')})
  expect(await prepare()).toEqual({result:'blocked',reason:'save_unconfirmed'})
  const pending=useWriteWorkspaceStore.getState().pendingSave
  expect(pending?.content).toBe('unknown receipt draft');expect(isWriteShutdownFrozen()).toBe(false)
  request.mockImplementationOnce(async input=>{expect(input).toEqual({action:'status',sessionId,operationId:pending!.operationId});return {ok:true,receipt}})
  currentId=randomUUID();expect(await prepare()).toEqual({result:'ready'})
  expect(request.mock.calls.filter(([i])=>i.action==='commit')).toHaveLength(1)
})
test('mounted input stays frozen until cancel, including an editor mounted during preparation',async()=>{
  const freeze=vi.fn();disposers.push(registerWriteShutdownInput({composing:()=>false,freeze}))
  expect(await prepare()).toEqual({result:'ready'});expect(freeze).toHaveBeenLastCalledWith(true)
  const later=vi.fn();disposers.push(registerWriteShutdownInput({composing:()=>false,freeze:later}));expect(later).toHaveBeenLastCalledWith(true)
  await currentHandler({requestId:randomUUID(),phase:'cancel'});expect(isWriteShutdownFrozen()).toBe(true)
  await cancel();expect(later).toHaveBeenLastCalledWith(false)
})
test('new file navigation is blocked while frozen without clearing the current document',async()=>{
  expect(await prepare()).toEqual({result:'ready'})
  await useWriteWorkspaceStore.getState().openFile(workspace,workspace+'/other.txt')
  useWriteWorkspaceStore.getState().resetWorkspace()
  expect(useWriteWorkspaceStore.getState().activeFilePath).toBe(path)
  expect(request.mock.calls.map(([i])=>i.action)).toEqual(['open'])
})
