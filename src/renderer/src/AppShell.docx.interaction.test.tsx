// @vitest-environment jsdom
import {act,createElement,useState,useEffect} from 'react'
import {createRoot} from 'react-dom/client'
import {it,expect,vi} from 'vitest'
import AppShell,{prewarmAppShellSurfaces} from './AppShell'
import {useChatStore} from './store/chat-store'
import {useDocxEditorStore} from './office/docx-editor-store'
import type {DocxEditorView} from '@shared/docx-editor'
const state=vi.hoisted(()=>({mounts:0,unmounts:0}))
vi.mock('./components/Workbench',()=>({Workbench:()=>{
 const [draft,setDraft]=useState(0)
 useEffect(()=>{state.mounts++;return()=>{state.unmounts++}},[])
 return createElement('button',{onClick:()=>setDraft(value=>value+1)},'Draft '+draft)
}}))
vi.mock('./components/SettingsView',()=>({SettingsView:()=>createElement('p',null,'Settings')}))
vi.mock('./components/InitialSetupDialog',()=>({InitialSetupDialog:()=>null}))
vi.mock('./components/RuntimeStatusBanner',()=>({RuntimeStatusBanner:()=>null}))
vi.mock('./components/TopNoticeHost',()=>({TopNoticeHost:()=>null}))
await prewarmAppShellSurfaces()
it('retains the real Word owner and local draft across Settings and return',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
 const previousChat=useChatStore.getState(),previousDocx=useDocxEditorStore.getState()
 const previous=Object.getOwnPropertyDescriptor(window,'analytix')
 Object.defineProperty(window,'analytix',{configurable:true,value:{app:{platform:'darwin'}}})
 const view={editorId:'word',phase:'editing',dirty:true} as DocxEditorView
 useDocxEditorStore.setState({views:{word:view}})
 useChatStore.setState({route:'chat',initialSetupOpen:false})
 const container=document.createElement('div');document.body.append(container);const root=createRoot(container)
 try {
  await act(async()=>root.render(createElement(AppShell)))
  await act(async()=>container.querySelector<HTMLButtonElement>('button')!.click())
  expect(container.textContent).toContain('Draft 1')
  await act(async()=>useChatStore.setState({route:'settings'}))
  expect(container.textContent).toContain('Settings')
  expect(container.querySelector('button')!.closest('[hidden]')).toBeTruthy()
  await act(async()=>useChatStore.setState({route:'chat'}))
  expect(container.textContent).toContain('Draft 1')
  expect(state.mounts).toBe(1);expect(state.unmounts).toBe(0)
 }finally{
  await act(async()=>root.unmount());container.remove()
  useChatStore.setState(previousChat);useDocxEditorStore.setState(previousDocx);vi.unstubAllGlobals()
  if(previous)Object.defineProperty(window,'analytix',previous);else Reflect.deleteProperty(window,'analytix')
 }
})
