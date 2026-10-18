// @vitest-environment jsdom
import {act,createElement} from 'react'
import {createRoot} from 'react-dom/client'
import {it,expect,vi} from 'vitest'
import {BuiltinOfficePlugins} from './BuiltinOfficePlugins'
import {useDocxEditorStore} from '../office/docx-editor-store'
import {useNativeOfficeStore} from '../office/native-office-store'
import {useChatStore} from '../store/chat-store'
import {useWriteWorkspaceStore} from '../write/write-workspace-store'
vi.mock('react-i18next',async importOriginal=>({...await importOriginal<typeof import('react-i18next')>(),useTranslation:()=>({t:(key:string)=>key})}))
it('opens Documents through the DOCX owner while keeping the existing package authority',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
 const select=vi.spyOn(useDocxEditorStore.getState(),'select').mockResolvedValue(true)
 const native=vi.spyOn(useNativeOfficeStore.getState(),'select')
 const flush=vi.spyOn(useWriteWorkspaceStore.getState(),'flushSave').mockResolvedValue(true)
 const open=vi.spyOn(useChatStore.getState(),'openWrite').mockResolvedValue(undefined)
 useChatStore.setState({activeThreadId:'thread',workspaceRoot:'/w'})
 window.analytix={packageHost:{request:vi.fn(async()=>({ok:true,packages:[{packageId:'analytix-documents',displayName:'Documents',available:true,materialized:true,activationState:'recorded',desiredState:'enabled'}]}))},
  office:{pickFile:vi.fn(async()=>({ok:true,path:'/w/report.docx'}))}} as unknown as typeof window.analytix
 const container=document.createElement('div');document.body.append(container);const root=createRoot(container)
 try {
  await act(async()=>root.render(createElement(BuiltinOfficePlugins,{query:''})))
  const button=[...container.querySelectorAll('button')].find(b=>b.textContent==='nativeOfficeOpen')!
  await act(async()=>button.click())
  expect(select).toHaveBeenCalledWith('/w','/w/report.docx','thread',expect.any(Function))
  expect(native).not.toHaveBeenCalled()
 }finally{await act(async()=>root.unmount());container.remove();select.mockRestore();native.mockRestore();flush.mockRestore();open.mockRestore();vi.unstubAllGlobals()}
})
