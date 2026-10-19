import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { useChatStore } from '../store/chat-store'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { SuperDocEditor, type SuperDocRef, type SuperDocInstance } from '@superdoc/react'
import type { SelectionCapture } from 'superdoc/ui'
import '@superdoc/react/style.css'
import { useDocxEditorStore } from './docx-editor-store'
import { type NativeReference, type DocxNativeReference } from './native-reference-store'
import type { DocxEditorRequest,DocxEditorView } from '@shared/docx-editor'
import { WriteInlineAgent } from '../components/write/WriteInlineAgent'
import { inlineAgentPosition } from '../components/write/write-workspace-view-utils'
import { Dialog } from '../data-analysis/components/overlay/Dialog'
import { Download, Pencil, RotateCcw, Save, Trash2, Undo2 } from '../design/AnalytixUiIcons'
import { WorkspaceViewerHeader, WorkspaceViewerIconButton, WorkspaceViewerMenu, type WorkspaceViewerControls } from '../components/workbench/WorkspaceViewerHeader'
import styles from './docx-editor-panel.module.css'

const ownerKey=(view:DocxEditorView)=>JSON.stringify([view.editorId,view.threadId,view.workspace,view.path,view.objectId])
const ui={toolbar:false} as const
const telemetry={enabled:false} as const
declare const __ANALYTIX_DOCX_WORKER_PATHS__:Readonly<{document:string;collaboration:string;reviewIndex:string}>

const mime='application/vnd.openxmlformats-officedocument.wordprocessingml.document'
const fromBase64=(data:string)=>Uint8Array.from(atob(data),c=>c.charCodeAt(0))
async function content(blob:Blob) {
  const bytes=new Uint8Array(await blob.arrayBuffer())
  if (!bytes.length || bytes.length>16*1024*1024) throw new Error('DOCX 文件超过大小限制。')
  let binary=''
  for (let i=0;i<bytes.length;i+=8192) binary+=String.fromCharCode(...bytes.subarray(i,i+8192))
  const hash=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes)),v=>v.toString(16).padStart(2,'0')).join('')
  return {encoding:'base64' as const,kind:'docx' as const,byteLength:bytes.length,sha256:hash,data:btoa(binary)}
}
const success=(result:unknown)=>result===true || !!result && typeof result==='object' && ('success' in result && result.success===true || 'ok' in result && result.ok===true)
const messages:Record<string,string>={unknown:'操作结果尚未确认，请查询本次操作。',conflict:'文件已在外部修改，工作副本保留。',
  unsaved_changes:'请先保存或放弃当前工作副本。',stale_selection:'选区已变化，请重新选择。',unavailable:'文档暂不可用，请重试。',forbidden:'当前权限不允许此操作。'}
/** Upstream owns layout, selection, formatting and local history; Core owns saves. */
export function DocxEditorPanel({view,visible,onSubmitPrompt,onPrepareDraft,fileActions,viewerControls}: {
  view:DocxEditorView;visible:boolean;onSubmitPrompt:(value:string,references?:NativeReference[])=>void;
  onPrepareDraft?:(value:string,references:NativeReference[])=>boolean
  fileActions?:(beforeOpen:(open:()=>void)=>void,exportCurrent:()=>void)=>ReactNode
  viewerControls?:WorkspaceViewerControls
}) {
  const [workerUrls]=useState(()=>{
    if (typeof __ANALYTIX_DOCX_WORKER_PATHS__==='undefined') throw new Error('DOCX worker paths were not supplied by the renderer build.')
    const paths=__ANALYTIX_DOCX_WORKER_PATHS__
    return {document:new URL('./'+paths.document,window.location.href).href,
      collaboration:new URL('./'+paths.collaboration,window.location.href).href,reviewIndex:new URL('./'+paths.reviewIndex,window.location.href).href}
  })
  const ref=useRef<SuperDocRef>(null), instance=useRef<SuperDocInstance|null>(null), editorHost=useRef<HTMLDivElement>(null)
  const instanceId=useRef(crypto.randomUUID()), mounted=useRef(true), shown=useRef(visible);shown.current=visible
  const latest=useRef(view);latest.current=view
  const localVersion=useRef(view.version), busy=useRef(false), applying=useRef(false)
  const mutationEpoch=useRef(0), unrecorded=useRef(false)
  const appliedProof=useRef<{captureId:string;version:number;scopeId:string}|null>(null)
  const captured=useRef<{capture:SelectionCapture;id:string;version:number;revision:string;owner:string;scopeId?:string}|null>(null)
  const retained=useRef<typeof captured.current>(null), recapturing=useRef<typeof captured.current>(null)
  const cancelRestore=useRef<(()=>void)|null>(null)
  const [applyFailed,setApplyFailed]=useState(false)
  const [ready,setReady]=useState(false), [working,setWorking]=useState(false), [error,setError]=useState('')
  const [selection,setSelection]=useState<{text:string;left:number;width:number;top:number;bottom:number;bounds:{left:number;right:number}}|null>(null)
  const [prompt,setPrompt]=useState(''), textarea=useRef<HTMLTextAreaElement>(null)
  const [pendingOpen,setPendingOpen]=useState<{owner:string;open:()=>void;waitingForSave?:boolean}|null>(null)
  const openIntent=useRef<typeof pendingOpen>(null)
  const rememberOpen=(intent:typeof pendingOpen)=>{openIntent.current=intent;setPendingOpen(intent)}
  const [source,setSource]=useState(()=>new File([fromBase64(view.content.data)],view.path.split('/').at(-1)??'document.docx',{type:mime}))
  const [sourceVersion,setSourceVersion]=useState(0)
  const modelRevision=useRef(view.revision)
  const current=useCallback(()=>mounted.current && shown.current && useChatStore.getState().route!=='settings' && useDocxEditorStore.getState().views[view.editorId]?.editorId===view.editorId,[view.editorId])
  const request=useCallback(async(input:DocxEditorRequest)=>{
    const result=await useDocxEditorStore.getState().request(input)
    if (mounted.current && !result.ok) setError(messages[result.error??'unavailable']??'操作未完成，请重试。')
    return result
  },[])
  const exclusive=async(work:()=>Promise<void>)=>{
    if (busy.current || !current()) return
    busy.current=true;setWorking(true);setError('')
    try {await work()} catch(e) {if (mounted.current) setError(e instanceof Error?e.message:'操作未完成，请重试。')}
    finally {
      try {if (unrecorded.current && current()) await recordUnobserved()}
      catch {if(mounted.current)setError('文档更新尚未记录，请保留工作副本并重试。')}
      finally {busy.current=false;if(mounted.current)setWorking(false)}
    }
  }
  async function exportBlob() {
    const sd=instance.current
    if (!sd) throw new Error('编辑器尚未就绪。')
    const blob=await sd.export({exportType:['docx'],triggerDownload:false})
    if (!(blob instanceof Blob)) throw new Error('编辑器未返回 DOCX 字节。')
    return blob
  }
  async function recordUnobserved() {
    const owner=useDocxEditorStore.getState().views[view.editorId]
    if (!owner || !unrecorded.current || owner.scope) return
    if (owner.phase==='ready') {
      const begun=await request({action:'begin',editorId:owner.editorId,version:owner.version})
      if (!begun.ok) return
    } else if (owner.phase!=='editing') return
    localVersion.current=Math.max(localVersion.current,owner.version)+1
    const recorded=await request({action:'changed',editorId:owner.editorId,version:localVersion.current})
    if (recorded.ok) unrecorded.current=false
  }
  async function save() {
    const before=latest.current, version=localVersion.current, epoch=mutationEpoch.current
    const bytes=await content(await exportBlob())
    if (!current() || version!==localVersion.current || latest.current.editorId!==before.editorId) return false
    if (epoch!==mutationEpoch.current) throw new Error('文档仍在更新，请重新保存。')
    const result=await request({action:'save',editorId:before.editorId,version,content:bytes})
    const saved=result.ok && result.view?.phase==='ready' && !result.view.dirty
    if (saved) {modelRevision.current=result.view!.revision;appliedProof.current=null;retireCapture();setApplyFailed(false)}
    return saved
  }
  async function exportCurrent() {
    const bytes=await content(await exportBlob())
    if(current()) await window.analytix.files.saveAs({suggestedName:view.path.split('/').at(-1),dataBase64:bytes.data,mimeType:mime})
  }
  const beforeOpen=(open:()=>void)=>{
    if (!current() || busy.current) return
    const owner=useDocxEditorStore.getState().views[view.editorId]
    if (!owner) return
    if (owner.dirty || owner.phase!=='ready' || unrecorded.current) rememberOpen({owner:ownerKey(owner),open})
    else open()
  }
  const pendingOpenCurrent=(intent:NonNullable<typeof pendingOpen>)=>current() && intent.owner===ownerKey(latest.current)
  async function openAfterSave() {
    const intent=pendingOpen
    if (!intent || !pendingOpenCurrent(intent)) {rememberOpen(null);return}
    intent.waitingForSave=true
    if (await save() && openIntent.current===intent && pendingOpenCurrent(intent)) {rememberOpen(null);intent.open()}
  }
  async function undoSaved() {
    const result=await request({action:'undoChange',editorId:view.editorId,version:localVersion.current})
    const retained=useDocxEditorStore.getState().views[view.editorId]
    if (!mounted.current || !result.ok || result.view?.phase!=='ready' || result.view.dirty ||
      ownerKey(result.view)!==ownerKey(latest.current) || !retained || retained.revision!==result.view.revision || retained.version!==result.view.version) return
    localVersion.current=result.view.version;appliedProof.current=null;retireCapture();setApplyFailed(false)
    // A Core undo changes disk bytes. Replace the SDK document only after its receipt and fresh read.
    instance.current=null;setReady(false)
    setSource(new File([fromBase64(result.view.content.data)],view.path.split('/').at(-1)??'document.docx',{type:mime}))
    modelRevision.current=result.view.revision
    setSourceVersion(value=>value+1)
  }
  async function changed() {
    if (!instance.current || !ready || applying.current || latest.current.phase!=='editing') return
    mutationEpoch.current++;unrecorded.current=true
    captured.current=null;setSelection(null)
    if (busy.current) return
    const version=++localVersion.current
    const result=await request({action:'changed',editorId:view.editorId,version})
    if (result.ok) unrecorded.current=false
  }
  const retireCapture=useCallback(()=>{captured.current=null;retained.current=null;setSelection(null);cancelRestore.current?.()},[])
  const captureCurrent=useCallback((cap:NonNullable<typeof captured.current>,allowScopeRotation=false)=>current() && captured.current===cap &&
    cap.version===localVersion.current && cap.version===latest.current.version && cap.revision===latest.current.revision && cap.owner===ownerKey(latest.current) &&
    cap.owner===ownerKey(useDocxEditorStore.getState().views[latest.current.editorId]) &&
    (!cap.scopeId || useDocxEditorStore.getState().views[latest.current.editorId]?.scope?.scopeId===cap.scopeId ||
      allowScopeRotation && recapturing.current===cap && !!useDocxEditorStore.getState().views[latest.current.editorId]?.scope),[current])
  useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;captured.current=null;retained.current=null;cancelRestore.current?.()}},[])
  useEffect(()=>{if(captured.current && !captureCurrent(captured.current,true))retireCapture()},
    [visible,view.editorId,view.threadId,view.workspace,view.path,view.objectId,view.version,view.revision,view.scope?.scopeId,captureCurrent,retireCapture])
  useEffect(()=>{localVersion.current=Math.max(localVersion.current,view.version)},[view.version])
  const updateGeometry=useCallback(()=>{
    const cap=captured.current,sd=instance.current,host=editorHost.current
    if (!cap?.capture.selectionTarget || !sd || !host || !current() || busy.current || applying.current ||
      cap.version!==localVersion.current || cap.revision!==latest.current.revision) {setSelection(null);return}
    try {
      const geometry=sd.ui.viewport.getRect({target:cap.capture.selectionTarget})
      const bounds=host.getBoundingClientRect()
      const left=Math.max(0,bounds.left),right=Math.min(window.innerWidth,bounds.right)
      const top=Math.max(0,bounds.top),bottom=Math.min(window.innerHeight,bounds.bottom)
      const rect=geometry.found && geometry.rects.find(rect=>[rect.left,rect.right,rect.top,rect.bottom].every(Number.isFinite) &&
        rect.right>rect.left && rect.bottom>rect.top && rect.right>left && rect.left<right && rect.bottom>top && rect.top<bottom)
      if (!rect || right<=left || bottom<=top) {setSelection(null);return}
      const selectionLeft=Math.max(left,rect.left)
      setSelection({text:cap.capture.quotedText,left:selectionLeft,width:Math.min(right,rect.right)-selectionLeft,
        top:Math.max(top,rect.top),bottom:Math.min(bottom,rect.bottom),bounds:{left,right}})
    } catch {setSelection(null)}
  },[current])
  useEffect(()=>{
    if (!ready || !instance.current) return
    const sd=instance.current
    let active=true
    const detachSelection=sd.ui.selection.observe(snapshot=>{
      if (!active || busy.current || applying.current || !shown.current) return
      if (snapshot.status!=='ready' || snapshot.empty || !snapshot.quotedText?.trim()) {
        // Application input owns focus; the captured SDK target survives its empty DOM selection.
        if(captured.current && retained.current===captured.current && captureCurrent(captured.current))return
        retireCapture();return
      }
      const capture=sd.ui.selection.capture()
      if (!capture?.selectionTarget || capture.quotedText.length>4096) {retireCapture();return}
      const previous=captured.current
      if (previous && JSON.stringify(previous.capture.selectionTarget)!==JSON.stringify(capture.selectionTarget)) {
        void useDocxEditorStore.getState().request({action:'revoke',editorId:view.editorId})
      }
      if (!previous || previous.version!==localVersion.current || previous.revision!==latest.current.revision ||
        previous.capture.quotedText!==capture.quotedText || JSON.stringify(previous.capture.selectionTarget)!==JSON.stringify(capture.selectionTarget)) {
        retained.current=null
        captured.current={capture,id:crypto.randomUUID(),version:localVersion.current,revision:latest.current.revision,
          owner:ownerKey(latest.current),scopeId:previous?undefined:latest.current.scope?.scopeId}
      }
      updateGeometry()
    })
    const detachViewport=sd.ui.viewport.observe(()=>{if(active)updateGeometry()})
    updateGeometry()
    return()=>{active=false;detachSelection();detachViewport()}
  },[ready,view.editorId,updateGeometry,captureCurrent,retireCapture])
  useEffect(()=>{if(visible && !working)updateGeometry()},[visible,working,updateGeometry])
  useEffect(()=>{
    if (!visible) {openIntent.current=null;setPendingOpen(null);captured.current=null;setSelection(null);if (latest.current.scope && latest.current.phase==='ready') void request({action:'revoke',editorId:view.editorId})}
  },[visible,view.editorId,request])
  const scopeId=view.scope?.scopeId
  useEffect(()=>{
    if (!visible || !scopeId || view.phase!=='ready') return
    let stopped=false
    const refresh=async()=>{if(!stopped && !busy.current) await request({action:'proposals',editorId:view.editorId})}
    void refresh();const timer=setInterval(()=>{void refresh()},1500)
    return()=>{stopped=true;clearInterval(timer)}
  },[visible,scopeId,view.phase,view.editorId,request])
  const selectionCurrent=useCallback((capture:NonNullable<typeof captured.current>)=>captureCurrent(capture) &&
    instance.current?.ui.selection.getSnapshot().status==='ready' &&
    instance.current.ui.selection.getSnapshot().quotedText===capture.capture.quotedText &&
    JSON.stringify(instance.current.ui.selection.capture()?.selectionTarget)===JSON.stringify(capture.capture.selectionTarget),[captureCurrent])
  async function restoreSelection(cap:NonNullable<typeof captured.current>) {
    if(selectionCurrent(cap))return true
    const sd=instance.current
    if(!sd || retained.current!==cap || !captureCurrent(cap))return false
    // restore() may settle through the worker after returning ok; require the real SDK snapshot.
    return new Promise<boolean>(resolve=>{
      let finished=false,restored=false,detach=()=>{},detachOwner=()=>{}
      const finish=(valid:boolean)=>{
        if(finished)return
        finished=true;window.clearTimeout(timer);detach();detachOwner();cancelRestore.current=null
        if(!valid && captured.current===cap)retireCapture()
        resolve(valid)
      }
      const check=()=>{
        if(!captureCurrent(cap)){finish(false);return}
        if(selectionCurrent(cap)){finish(true);return}
        const snapshot=sd.ui.selection.getSnapshot()
        if(snapshot.status==='ready' && !snapshot.empty)finish(false)
      }
      const timer=window.setTimeout(()=>finish(false),5000)
      cancelRestore.current=()=>finish(false)
      detachOwner=useDocxEditorStore.subscribe(state=>{
        const owner=state.views[view.editorId]
        if(!owner || ownerKey(owner)!==cap.owner || owner.version!==cap.version || owner.revision!==cap.revision ||
          cap.scopeId && owner.scope?.scopeId!==cap.scopeId)finish(false)
      })
      detach=sd.ui.selection.observe(()=>{if(restored)check()})
      restored=sd.ui.selection.restore(cap.capture).ok
      if(!restored)finish(false);else check()
      if(finished)detach()
    })
  }
  useEffect(()=>window.analytix.docxEditor.onCapture(challenge=>{
    const cap=captured.current,sd=instance.current
    if (!cap || !sd || challenge.editorId!==view.editorId || challenge.version!==localVersion.current || challenge.revision!==latest.current.revision || !selectionCurrent(cap)) return null
    const actual=sd.ui.selection.capture()
    if (!actual?.selectionTarget || actual.quotedText!==cap.capture.quotedText) return null
    return {...challenge,instanceId:instanceId.current,captureId:cap.id,text:actual.quotedText,target:actual.selectionTarget as unknown as Record<string,unknown>}
  }),[view.editorId,selectionCurrent])
  async function reference(editable:boolean,instruction:string,draft=false) {
    const capture=captured.current
    if (!capture || !await restoreSelection(capture) || latest.current.phase!=='ready') {setError('请先保存，再重新选择要引用的内容。');return}
    // Observe the owner synchronously: a revoke must survive React batching with a late response.
    const detachOwner=useDocxEditorStore.subscribe(state=>{
      if(captured.current!==capture)return
      const owner=state.views[view.editorId]
      if(!owner || ownerKey(owner)!==capture.owner || owner.version!==capture.version || owner.revision!==capture.revision ||
        capture.scopeId && !owner.scope)retireCapture()
    })
    recapturing.current=capture
    try {
      const response=await request({action:'capture',editorId:view.editorId,version:localVersion.current,instanceId:instanceId.current,
        captureId:capture.id,editable})
      if(response.ok && response.view?.scope && captureCurrent(capture,true))capture.scopeId=response.view.scope.scopeId
      if (!response.ok || !response.view?.scope || !selectionCurrent(capture)) {
        if(captured.current===capture)retireCapture()
        if(response.ok) await request({action:'revoke',editorId:view.editorId})
        return
      }
      const v=response.view,s=v.scope!
      const item:Omit<DocxNativeReference,'id'>={kind:'docx',threadId:v.threadId,workspace:v.workspace,path:v.path,objectId:v.objectId,
        revision:v.revision,label:`${v.path.split('/').at(-1)} · 选中文本`,text:'',editorId:v.editorId,version:v.version,
        scopeId:s.scopeId,selectionToken:s.selectionToken,editable}
      const reference={...item,id:crypto.randomUUID()}
      if (draft) onPrepareDraft?.(instruction,[reference])
      else onSubmitPrompt(instruction,[reference])
      setPrompt('')
    } finally {
      detachOwner()
      if(recapturing.current===capture)recapturing.current=null
      if(captured.current===capture && !captureCurrent(capture))retireCapture()
    }
  }
  async function format(command:string,value?:string|number) {
    const sd=instance.current,capture=captured.current
    if (!sd || !capture || latest.current.phase!=='editing' || !await restoreSelection(capture) || latest.current.phase!=='editing' || latest.current.scope) return
    if (!sd.ui.selection.restore(capture.capture).ok || !sd.ui.commands.get(command)?.getState().enabled) return
    const result=await sd.ui.commands.executeAsync(command,value)
    if (!success(result)) throw new Error('当前选区不支持此格式操作。')
    // Formatting updates arrive while the action holds input; record this mutation explicitly.
    localVersion.current++;await request({action:'changed',editorId:view.editorId,version:localVersion.current})
    unrecorded.current=false;captured.current=null;setSelection(null)
  }
  async function decide(proposalId:string,accept:boolean) {
    const capture=captured.current,sd=instance.current,scope=latest.current.scope
    if (!capture || !sd || !scope || !await restoreSelection(capture)) {setError('选区已过期，请重新选择。');return}
    const result=await request({action:'decide',editorId:view.editorId,version:localVersion.current,scopeId:scope.scopeId,proposalId,accept,
      instanceId:instanceId.current,captureId:capture.id})
    if (!result.ok || !accept) return
    if (!current() || !selectionCurrent(capture) || typeof result.replacement!=='string') {setApplyFailed(true);return}
    applying.current=true
    try {
      if (!sd.ui.selection.restore(capture.capture).ok) throw new Error('选区已过期，未应用建议。')
      sd.setDocumentMode('editing')
      const document=sd.activeEditor?.doc
      if (!document) throw new Error('编辑器尚未就绪，工作副本保留。')
      const applied=await document.replace({target:capture.capture.selectionTarget!,text:result.replacement},{changeMode:'direct'})
      if (!applied.success) throw new Error('建议未能应用，工作副本保留；请放弃后重新捕获。')
      appliedProof.current={captureId:capture.id,version:capture.version,scopeId:scope.scopeId}
      const acknowledged=await request({action:'applied',editorId:view.editorId,version:capture.version,instanceId:instanceId.current,captureId:capture.id})
      if (!acknowledged.ok || !current()) {setApplyFailed(true);return}
      localVersion.current=acknowledged.view!.version
      const bytes=await content(await exportBlob())
      if (!current()) return
      const saved=await request({action:'save',editorId:view.editorId,version:localVersion.current,content:bytes})
      // The old range is no longer usable after replacement. Keep the applied proof
      // through a failed/unknown save so status can recover without replacing again.
      if (saved.ok && saved.view?.phase==='ready' && !saved.view.dirty) {modelRevision.current=saved.view.revision;appliedProof.current=null;retireCapture();setApplyFailed(false)}
    } catch(error) {setApplyFailed(true);throw error} finally {sd.setDocumentMode('viewing');applying.current=false}
  }
  async function reopenTab(tabId:string|null) {
    const owns=()=>{const tabs=useWorkspaceTabsStore.getState(),tab=tabs.tabs.find(item=>item.id===tabId)
      return tabs.activeTabId===tabId && tab?.path===view.path && tab.workspaceRoot===view.workspace && useChatStore.getState().activeThreadId===view.threadId}
    if (owns()) await useDocxEditorStore.getState().select(view.workspace,view.path,view.threadId,owns)
  }
  async function status() {
    const tabId=useWorkspaceTabsStore.getState().activeTabId
    let result=await request({action:'status',editorId:view.editorId})
    if (result.ok && !result.view) {await reopenTab(tabId);return}
    const proof=appliedProof.current
    if (proof && result.ok && result.view?.scope?.scopeId===proof.scopeId) {
      if (result.view.phase==='applying') result=await request({action:'applied',editorId:view.editorId,version:proof.version,instanceId:instanceId.current,captureId:proof.captureId})
      if (result.ok && result.view?.phase==='editing' && result.view.version===proof.version+1) {localVersion.current=result.view.version;setApplyFailed(false)}
    }
    if (result.ok && result.view?.phase==='ready') {
      appliedProof.current=null;setApplyFailed(false)
      if (result.view.revision!==modelRevision.current) {
        instance.current=null;setReady(false);localVersion.current=result.view.version;retireCapture()
        setSource(new File([fromBase64(result.view.content.data)],view.path.split('/').at(-1)??'document.docx',{type:mime}))
        modelRevision.current=result.view.revision
        setSourceVersion(value=>value+1)
      }
      const intent=openIntent.current
      if (intent?.waitingForSave && pendingOpenCurrent(intent)) {rememberOpen(null);intent.open()}
    }
  }
  const resumeDocumentSelection=(event:{type:string})=>{
    // Focus may belong to this operation; new pointer/key intent cancels a pending restore.
    if(applying.current || busy.current && (event.type==='focus' || !cancelRestore.current))return
    const cap=captured.current;retireCapture()
    if(cap?.scopeId && latest.current.phase==='ready')void request({action:'revoke',editorId:view.editorId})
  }
  const inlineAction=selection?inlineAgentPosition({anchorRect:selection},{bounds:selection.bounds}):null
  return <section className={styles.panel} hidden={!visible} inert={!visible} aria-label="Word 文档">
    <WorkspaceViewerHeader title={view.path.split('/').at(-1) ?? 'Word'} viewerControls={visible ? viewerControls : undefined}
      titleControl={fileActions ? <span className={styles.fileActions} inert={!ready||working}>{fileActions(beforeOpen,()=>void exclusive(exportCurrent))}</span> : undefined}
      status={error || (applyFailed || view.phase==='unknown'?'结果待确认':view.phase==='conflict'?'文件冲突':view.phase==='applying'?'更新中':view.dirty?'未保存':view.phase==='editing'?'工作副本编辑中':'已保存')}
      statusRole={error || applyFailed || view.phase==='unknown'||view.phase==='conflict'?'alert':'status'}
      actions={<>
        <WorkspaceViewerIconButton label="编辑" disabled={!ready||working||view.phase!=='ready'}
          onClick={()=>void exclusive(async()=>{await request({action:'begin',editorId:view.editorId,version:localVersion.current})})}><Pencil aria-hidden="true" /></WorkspaceViewerIconButton>
        <WorkspaceViewerIconButton label="保存" tone="commit" disabled={!ready||working||applyFailed||view.phase!=='editing'}
          onClick={()=>void exclusive(async()=>{await save()})}><Save aria-hidden="true" /></WorkspaceViewerIconButton>
        {view.phase==='unknown'||view.phase==='applying'||applyFailed ? <WorkspaceViewerIconButton label="查询结果" disabled={working}
          onClick={()=>void exclusive(status)}><RotateCcw aria-hidden="true" /></WorkspaceViewerIconButton> : null}
        <WorkspaceViewerIconButton label="撤销工作副本编辑" title="仅更改工作副本，保存后才写入文件。"
          disabled={!ready||working||view.phase!=='editing'||!!view.scope||applyFailed} onClick={()=>void exclusive(async()=>{
            const sd=instance.current;if(sd?.ui.commands.get('undo')?.getState().enabled){await sd.ui.commands.executeAsync('undo');localVersion.current++;await request({action:'changed',editorId:view.editorId,version:localVersion.current})}
          })}><Undo2 aria-hidden="true" /></WorkspaceViewerIconButton>
        {!fileActions ? <WorkspaceViewerIconButton shape="pill" label="另存为" disabled={!ready||working} onClick={()=>void exclusive(exportCurrent)}><Download aria-hidden="true" /></WorkspaceViewerIconButton> : null}
        <WorkspaceViewerMenu label="更多 Word 操作" ownerKey={ownerKey(view)} enabled={visible}>
          <button type="button" role="menuitem" disabled={working} onClick={()=>void exclusive(async()=>{
            const tabId=useWorkspaceTabsStore.getState().activeTabId
            const result=await request({action:'close',editorId:view.editorId,discard:true})
            if (result.ok) await reopenTab(tabId)
          })}><Trash2 aria-hidden="true" />放弃工作副本</button>
          <button type="button" role="menuitem" title="恢复保存前的内容并保存文件。"
            disabled={working||!ready||!view.canUndo||view.dirty||view.phase!=='ready'&&!(view.phase==='unknown'&&view.recovery?.current?.canRetryUndo)}
            onClick={()=>void exclusive(undoSaved)}><Undo2 aria-hidden="true" />{view.phase==='unknown'&&view.recovery?.current?.canRetryUndo?'完成已保存撤销':'撤销已保存修改'}</button>
        </WorkspaceViewerMenu>
      </>} />
    {error&&!pendingOpen?<p role="alert" className={styles.notice}>{error}</p>:null}
    <div ref={editorHost} className={styles.editor} aria-busy={working} inert={working}
      onPointerDownCapture={resumeDocumentSelection} onFocusCapture={resumeDocumentSelection} onKeyDownCapture={resumeDocumentSelection}>
      <SuperDocEditor key={sourceVersion} ref={ref} document={source} contained allowSelectionInViewMode ui={ui} telemetry={telemetry} workerUrls={workerUrls}
        documentMode={view.phase==='editing'&&!view.scope&&!applyFailed?'editing':'viewing'}
        onReady={({superdoc})=>{instance.current=superdoc;setReady(true)}} onEditorUpdate={()=>{void changed()}} />
    </div>
    <div style={{display:'contents'}} onPointerDownCapture={()=>{retained.current=captured.current}}
      onFocusCapture={()=>{retained.current=captured.current}}>
    {selection && !working ? <>
      {view.phase==='editing'&&!view.scope?<div className={styles.formats} style={{left:selection.left,top:Math.max(8,selection.top-42)}} onMouseDown={event=>{if(event.target instanceof HTMLButtonElement)event.preventDefault()}}>
        <button aria-label="加粗" onClick={()=>void exclusive(()=>format('bold'))}><b>B</b></button>
        <select aria-label="字体" defaultValue="" onChange={e=>void exclusive(()=>format('font-family',e.target.value))}>
          <option value="" disabled>字体</option>{['Arial','Calibri','宋体','黑体'].map(value=><option key={value}>{value}</option>)}
        </select>
        <select aria-label="字号" defaultValue="" onChange={e=>void exclusive(()=>format('font-size',Number(e.target.value)))}>
          <option value="" disabled>字号</option>{[10,12,14,16,18,24,36].map(value=><option key={value}>{value}</option>)}
        </select>
        <input aria-label="文字颜色" type="color" onChange={e=>void exclusive(()=>format('text-color',e.target.value))}/>
      </div>:inlineAction?<WriteInlineAgent action={inlineAction} value={prompt} inFlight={working}
        textareaRef={textarea} onValueChange={setPrompt} formattingEnabled={false} onSubmitPrompt={value=>void exclusive(()=>reference(false,value))}
        onApplyEdit={value=>void exclusive(()=>reference(true,value))} onQuoteSelection={()=>void exclusive(()=>reference(false,'请参考这段文档内容。',true))}
        quickActions={[{id:'polish',label:'润色',prompt:'润色选中内容，保留事实。',mode:'edit'},{id:'expand',label:'扩写',prompt:'扩写选中内容，保留事实，不编造。',mode:'edit'},{id:'shorten',label:'精简',prompt:'精简选中内容，保留关键事实与原意。',mode:'edit'}]}
        onQuickAction={action=>void exclusive(()=>reference(action.mode==='edit',action.prompt))}/>:null}</>:null}
    {view.reviews.map(review=><aside key={review.proposalId} className={styles.review}>
      <p>Agent 建议</p><dl className={styles.comparison}><div><dt>原文</dt><dd>{review.beforeText}</dd></div><div><dt>修改后</dt><dd>{review.afterText}</dd></div></dl>
      <button className={styles.accept} disabled={working||view.phase!=='ready'||view.proposals.find(p=>p.proposalId===review.proposalId)?.status!=='proposed'} onClick={()=>void exclusive(()=>decide(review.proposalId,true))}>应用并保存</button>
      <button disabled={working||view.phase!=='ready'} onClick={()=>void exclusive(()=>decide(review.proposalId,false))}>拒绝</button>
      <button disabled={working||view.phase!=='ready'} onClick={()=>void exclusive(()=>reference(true,'请重新生成选中内容的修改建议。'))}>重试</button>
    </aside>)}
    </div>
    {visible && pendingOpen?<Dialog open title="打开 Word 文档" onClose={()=>{if(!working)rememberOpen(null)}}
      footer={<>{view.phase==='unknown'?<button disabled={working} onClick={()=>void exclusive(status)}>查询结果</button>:null}
        <button disabled={working||applyFailed||view.phase!=='editing'} onClick={()=>void exclusive(openAfterSave)}>保存并打开</button>
        <button disabled={working} onClick={()=>{const intent=pendingOpen;rememberOpen(null);if(pendingOpenCurrent(intent))intent.open()}}>打开已保存版本</button>
        <button disabled={working} onClick={()=>rememberOpen(null)}>取消</button></>}>
      <p>当前工作副本尚未保存。选择保存后打开，或打开磁盘上的已保存版本。</p>
      {error?<p role="alert">{error}</p>:null}
      {view.phase==='unknown'?<p>保存结果待确认，请先查询本次操作。</p>:null}
    </Dialog>:null}
  </section>
}
