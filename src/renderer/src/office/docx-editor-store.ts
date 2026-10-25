import { create } from 'zustand'
import type { DocxEditorRequest, DocxEditorResponse, DocxEditorView } from '@shared/docx-editor'
import { useWorkspaceTabsStore,workspaceObjectTabId } from '../store/workspace-tabs-store'
import { useChatStore } from '../store/chat-store'
import type { ThreadSelectionReceipt } from '../store/chat-store-types'
import { normalizeWorkspaceRoot } from '../lib/workspace-path'

export const docxKey=(workspace:string,path:string,threadId:string)=>JSON.stringify([workspace,path,threadId])
export const isDocxPath=(path:string)=>/\.docx$/i.test(path)
export const docxOpenKey=(workspace:string,path:string)=>JSON.stringify([workspace,path])
/** A document intent may materialize a real Core owner before any AI prompt. */
export async function ensureLocalDocumentThread(workspace:string,isCurrent:()=>boolean,onOwner?:(threadId:string)=>void):Promise<ThreadSelectionReceipt|null> {
  const origin=useChatStore.getState(),route=origin.route,root=normalizeWorkspaceRoot(origin.workspaceRoot)
  let allowedThread=origin.activeThreadId
  const current=()=>isCurrent() && useChatStore.getState().route===route &&
    normalizeWorkspaceRoot(useChatStore.getState().workspaceRoot)===root && useChatStore.getState().activeThreadId===allowedThread
  const selected:{receipt?:ThreadSelectionReceipt}={}
  const active=origin.threads.find(thread=>thread.id===allowedThread)
  if (allowedThread && normalizeWorkspaceRoot(active?.workspace || origin.workspaceRoot)===normalizeWorkspaceRoot(workspace)) {
    onOwner?.(allowedThread)
    await origin.selectThread(allowedThread,{localDocument:true,isCurrent:current,onSelected:receipt=>{selected.receipt=receipt}})
  } else {
    await origin.createThread({workspaceRoot:workspace,materialize:true,forceNew:true,localDocument:true,isCurrent:current,
      beforeSelect:target=>{
        if (!current() || normalizeWorkspaceRoot(target.workspace)!==normalizeWorkspaceRoot(workspace)) return false
        allowedThread=target.threadId;onOwner?.(target.threadId);return true
      },onSelected:receipt=>{selected.receipt=receipt}})
  }
  if (!current()) return null
  if (!selected.receipt?.isCurrent()) throw new Error(useChatStore.getState().error || '无法建立本地文档会话，请重试。')
  return selected.receipt
}
type State={views:Record<string,DocxEditorView>; error:string|null; openErrors:Record<string,string>;
  select:(workspace:string,path:string,threadId?:string,isCurrent?:()=>boolean,onOwner?:(threadId:string)=>void)=>Promise<boolean>;
  request:(request:DocxEditorRequest)=>Promise<DocxEditorResponse>;
  close:(workspace:string,path:string)=>Promise<boolean>}
export const useDocxEditorStore=create<State>((set,get)=>({
  views:{},error:null,openErrors:{},
  request:async request=>{
    const result=await window.analytix.docxEditor.request(request)
    set(state=>{
      const views={...state.views}
      if (result.view) views[result.view.editorId]=result.view
      if (result.ok && !result.view && 'editorId' in request) delete views[request.editorId]
      return {views,error:result.error ?? null}
    })
    return result
  },
  select:async(workspace,path,threadId,isCurrent=()=>true,onOwner)=>{
    if (!isCurrent()) return false
    const key=docxOpenKey(workspace,path),originThread=useChatStore.getState().activeThreadId
    let current=()=>isCurrent() && useChatStore.getState().activeThreadId===originThread
    set(state=>{const openErrors={...state.openErrors};delete openErrors[key];return {openErrors}})
    try {
      if (!threadId) {
        const receipt=await ensureLocalDocumentThread(workspace,isCurrent,onOwner)
        if (!receipt) return false
        threadId=receipt.threadId;current=()=>isCurrent() && receipt.isCurrent()
      }
      if (!current()) return false
      const retained=Object.values(get().views).find(view=>view.workspace===workspace &&
        (view.path===path || view.path===workspace.replace(/\/$/,'')+'/'+path) && view.threadId===threadId)
      const result=retained?{ok:true,view:retained}:await get().request({action:'open',workspace,path,threadId})
      if (!current()) return false
      if (!result.ok || !result.view) {
        set(state=>({openErrors:{...state.openErrors,[key]:'error' in result ? result.error || 'unavailable' : 'unavailable'}}))
        return false
      }
      await window.analytix.office.request({action:'hide'}).catch(()=>undefined)
      if (!current()) return false
      useWorkspaceTabsStore.getState().openTab({id:workspaceObjectTabId(workspace,result.view.path),kind:'document',mode:'documents',title:result.view.path.split('/').at(-1)??'Word',workspaceRoot:workspace,path:result.view.path})
      return true
    } catch(error) {
      if (isCurrent()) set(state=>({openErrors:{...state.openErrors,[key]:error instanceof Error?error.message:'unavailable'}}))
      return false
    }
  },
  close:async(workspace,path)=>{
    const owners=Object.values(get().views).filter(view=>view.workspace===workspace && view.path===path)
    for (const owner of owners) if (!(await get().request({action:'close',editorId:owner.editorId})).ok) return false
    return true
  }
}))
