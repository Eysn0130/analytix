// @vitest-environment jsdom
import { beforeEach,it,expect,vi } from 'vitest'
import { useDocxEditorStore,docxOpenKey } from './docx-editor-store'
import { useChatStore } from '../store/chat-store'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
beforeEach(()=>{useDocxEditorStore.setState(useDocxEditorStore.getInitialState(),true);useWorkspaceTabsStore.setState({tabs:[],activeTabId:null})})
it('does not register a late open receipt in another tab',async()=>{
 let current=true
 const request=vi.fn(async()=>{current=false;return {ok:true,view:{editorId:'owner',workspace:'/w',path:'/w/a.docx',threadId:'thread'}}})
 window.analytix={docxEditor:{request},office:{request:vi.fn()}} as unknown as typeof window.analytix
 expect(await useDocxEditorStore.getState().select('/w','a.docx','thread',()=>current)).toBe(false)
 expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
})
it('does not remove an unresolved owner when close is refused',async()=>{
 const view={editorId:'owner',workspace:'/w',path:'/w/a.docx',dirty:true}
 useDocxEditorStore.setState({views:{owner:view as never}})
 window.analytix={docxEditor:{request:vi.fn(async()=>({ok:false,view,error:'unknown'}))}} as unknown as typeof window.analytix
 expect(await useDocxEditorStore.getState().close('/w','/w/a.docx')).toBe(false)
 expect(useDocxEditorStore.getState().views.owner.dirty).toBe(true)
})

it('local Word keeps a target-specific refusal without registering a text fallback or another tab',async()=>{
 useChatStore.setState({activeThreadId:'thread'})
 window.analytix={docxEditor:{request:vi.fn(async()=>({ok:false,view:null,error:'forbidden'}))}} as unknown as typeof window.analytix
 expect(await useDocxEditorStore.getState().select('/w','a.docx','thread')).toBe(false)
 expect(useDocxEditorStore.getState().openErrors[docxOpenKey('/w','a.docx')]).toBe('forbidden')
 expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
})
it('local Word reuses the same retained owner without reopening its Core object',async()=>{
 useChatStore.setState({activeThreadId:'thread'})
 const view={editorId:'owner',workspace:'/w',path:'/w/a.docx',threadId:'thread'}
 useDocxEditorStore.setState({views:{owner:view as never}})
 const request=vi.fn();window.analytix={docxEditor:{request},office:{request:vi.fn(async()=>({ok:true,view:null}))}} as unknown as typeof window.analytix
 expect(await useDocxEditorStore.getState().select('/w','/w/a.docx','thread')).toBe(true)
 expect(request).not.toHaveBeenCalled();expect(useWorkspaceTabsStore.getState().tabs[0]).toMatchObject({mode:'documents',path:'/w/a.docx'})
})
