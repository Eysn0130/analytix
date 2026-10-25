import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { nativeTypedWorkbookSelection, type NativeOfficeRequest, type NativeOfficeResponse, type NativeOfficeSelection, type NativeOfficeView } from '@shared/native-office'
import { Check, Info, Save, Trash2 } from '../design/AnalytixUiIcons'
import { WorkspaceViewerIconButton, WorkspaceViewerMenu } from '../components/workbench/WorkspaceViewerHeader'
import { useAnchoredPanel } from '../components/chat/presentation/useAnchoredPanel'
import styles from './workbook-cell-editor.module.css'
import {useNativeOfficeStore,nativeWorkbookDraftKey,nativeOfficeTargetKey,nativeWorkbookDraftUnsettled,nativeWorkbookDraftCellMatches,nativeOfficeHasUnresolvedChange,type NativeWorkbookCellDraft} from './native-office-store'
import {registerWriteShutdownInput} from '../write/write-shutdown'

let workbookShutdownRegistered = false
export function ensureNativeWorkbookDraftShutdownGuard() {
  if (workbookShutdownRegistered) return
  workbookShutdownRegistered = true
  // Retained store lifetime: hidden drafts participate in the existing renderer
  // shutdown handshake without pretending to be composition or auto-saving.
  registerWriteShutdownInput({
    composing:() => useNativeOfficeStore.getState().workbookDraftComposing,
    freeze:frozen => {
      useNativeOfficeStore.setState({workbookDraftInputFrozen:frozen})
      if (frozen && Object.values(useNativeOfficeStore.getState().workbookDrafts).some(nativeWorkbookDraftUnsettled)) throw new Error('Unsaved workbook cell draft')
    }
  })
}

type CellSelection = Extract<NativeOfficeSelection, {kind:'cells'}>
type CellEdit = Extract<NativeOfficeRequest, {action:'editWorkbookCell'}>['edit']
type InputMode = NonNullable<NativeWorkbookCellDraft['inputMode']>
export type WorkbookCellContextActions={
  owner:string;objectId:string;revision:string;changeSequence:number;selectionToken:string
  actions:ReadonlyArray<{id:string;label:string;enabled:boolean;run:()=>void}>
}
const decimalValue=/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/
const inputModes:ReadonlyArray<{value:InputMode;label:string}>=[{value:'auto',label:'自动识别'},{value:'text',label:'文本'},
  {value:'number',label:'数值'},{value:'formula',label:'公式'},{value:'empty',label:'清空'}]
const column = (value:number):string => { let label = ''; for (value++; value; value = Math.floor((value - 1) / 26)) label = String.fromCharCode(65 + (value - 1) % 26) + label; return label }

/** Cached native results are display data, never a replacement formula evaluator. */
export function workbookSelectionSum(selection: NativeOfficeSelection | undefined): number | null {
  if (selection?.kind !== 'cells' || !selection.capture?.complete || selection.capture.truncated ||
    selection.ranges.length !== 1 || !selection.cells?.length || selection.cells.length > 256) return null
  const range = selection.ranges[0], width = range.endColumn - range.startColumn + 1
  if (width < 1 || (range.endRow - range.startRow + 1) * width !== selection.cells.length) return null
  let sum = 0, numbers = 0
  for (const [index,cell] of selection.cells.entries()) {
    if (cell.sheet !== range.sheet || cell.column !== range.startColumn + index % width ||
      cell.row !== range.startRow + Math.floor(index / width) || cell.merged) return null
    if (cell.valueType !== 'number' && cell.valueType !== 'formula') continue
    // getValue() also returns zero for text formulas; both native proofs are required.
    if (cell.valueType === 'formula') {
      if (cell.valueError !== false) return null
      if (cell.formulaResultType === 'text') continue
      if (cell.formulaResultType !== 'number') return null
    }
    if (!Number.isFinite(cell.value)) return null
    sum += cell.value; numbers++
    if (!Number.isFinite(sum)) return null
  }
  return numbers ? sum : null
}

function currentSelection(view:NativeOfficeView):CellSelection | null {
  const selected = view.selection
  return selected?.kind === 'cells' && !!selected.token && selected.documentId === view.objectId &&
    selected.version === view.revision && selected.changeSequence === (view.changeSequence ?? 0) ? selected : null
}
function initialValue(cell:NonNullable<CellSelection['cells']>[number] | undefined):{type:CellEdit['type'];value:string} {
  const type=cell?.valueType === 'empty' ? 'text' : cell?.valueType ?? 'text'
  return {type,value:cell ? type === 'formula' ? cell.formula : type === 'number' ? String(cell.value) : cell.text : ''}
}
export function workbookDraftEdit(type:CellEdit['type'], value:string):CellEdit | null {
  if (type === 'empty') return {type:'empty'}
  if (type === 'text') return value.length > 0 && value.length <= 4096 ? {type:'text',text:value} : null
  if (type === 'formula') return value.startsWith('=') && value.length > 1 && value.length <= 1024 ? {type:'formula',formula:value} : null
  if (!decimalValue.test(value.trim())) return null
  const number = Number(value)
  return Number.isFinite(number) ? {type:'number',value:number} : null
}
/** Conservative input intent only; native validation and receipts retain file authority. */
export function workbookAutoInputType(value:string):CellEdit['type'] {
  if (value === '') return 'empty'
  if (value.startsWith('=')) return 'formula'
  const digits=value.split(/[eE]/)[0].replace(/\D/g,'').replace(/^0+/,'')
  const number=Number(value)
  return decimalValue.test(value) && !/^[+-]?0\d/.test(value) && digits.length <= 15 && Number.isFinite(number) &&
    (number !== 0 || !/[1-9]/.test(digits)) ? 'number' : 'text'
}
function savedReceipt(response:NativeOfficeResponse | undefined, before:NativeOfficeView, threadId:string):NativeOfficeView | null {
  const next = response?.view, change = next?.recovery?.current
  return response?.ok && next?.status === 'ready' && next.objectId === before.objectId && next.path === before.path &&
    next.revision !== before.revision && !next.dirty && !next.saving && change && 'origin' in change &&
    change.version === 3 && change.origin === 'manual' && !!change.workbook && change.status === 'committed' && change.threadId === threadId &&
    change.baseRevision === before.revision && change.revision === next.revision ? next : null
}
const saveError = (error:string | undefined):string => error === 'stale_selection' || error === 'conflict'
  ? '选区或文件版本已变化，请重新选择。' : error === 'unsupported_selection' || error === 'invalid_request'
    ? '此输入或公式不受支持；请检查类型与公式范围。' : '保存结果尚未确认，请通过“查询结果”核实。'

export function WorkbookCellEditor({view,threadId,active,busy,onRequest,workspace='',path=view.path,toolbarTarget,onContextActionsChange}:{
  view:NativeOfficeView; threadId:string | null; active:boolean; busy:boolean; workspace?:string; path?:string
  toolbarTarget?:HTMLElement | null
  onContextActionsChange?:(context:WorkbookCellContextActions|null)=>void
  onRequest:(request:NativeOfficeRequest) => Promise<NativeOfficeResponse | undefined>
}) {
  const preferredDraftKey=nativeWorkbookDraftKey(workspace,path,threadId)
  const drafts=useNativeOfficeStore(state=>state.workbookDrafts)
  // A prior thread's retained input must stay visible and discardable when it
  // vetoes this document's close. Its owning thread still gates Save.
  const retained=Object.entries(drafts).find(([,value])=>value.workspace === workspace && value.path === path && nativeWorkbookDraftUnsettled(value))
  const draftKey=drafts[preferredDraftKey] ? preferredDraftKey : retained?.[0] ?? preferredDraftKey
  const draft=drafts[draftKey]
  const frozen=useNativeOfficeStore(state=>state.workbookDraftInputFrozen)
  const closing=useNativeOfficeStore(state=>!!state.workbookClosing[nativeOfficeTargetKey({workspace,path})])
  const selection=currentSelection(view),bounded=selection && nativeTypedWorkbookSelection(selection)
  const cell=bounded?.cells.length === 1 ? bounded.cells[0] : undefined
  const [notice,setNotice]=useState<{owner:string;text:string;error:boolean;revisions?:string[]} | null>(null)
  const input=useRef<HTMLInputElement>(null),lastIntent=useRef('')
  const latest=useRef({view,threadId,active});latest.current={view,threadId,active}
  const pending=!!draft?.pending,unresolved=[view.recovery?.pending,view.recovery?.current].some(change=>['prepared','unknown','conflict'].includes(change?.status ?? ''))
  const transitioning=view.kind === 'xlsx' && (view.status !== 'ready' || !!view.saving || nativeOfficeHasUnresolvedChange(view,null))
  const inputBlocked=busy || transitioning
  const available=!!(active && threadId && view.kind === 'xlsx' && view.status === 'ready' && !view.protectedPresent &&
    view.workbookEditing?.editable && cell && !view.dirty && !view.saving && !unresolved)
  // A draft's identity is the original cell and revision. A fresh native token
  // can validate that same target, but a different target/version never receives
  // these words. Lost authority disables Save without erasing the user's input.
  const sameTarget=!!(!draft || draft.threadId === threadId && draft.objectId === view.objectId && draft.revision === view.revision &&
    bounded?.sheetName === draft.sheetName && cell && nativeWorkbookDraftCellMatches(cell,draft.cell))
  const editable=active && !inputBlocked && !pending && !frozen && !closing && !draft?.unconfirmed && (!!draft || available)
  const canSave=available && sameTarget && editable && !draft?.unconfirmed
  const form=draft ?? initialValue(cell),change=workbookDraftEdit(form.type,form.value)
  const inputMode:InputMode=draft ? draft.inputMode ?? draft.type : 'auto'
  const changed=!!draft?.changed
  const owner=draft?.id ?? JSON.stringify([draftKey,view.objectId,view.revision,cell?.sheet,cell?.column,cell?.row])
  const contextCell=JSON.stringify([bounded?.sheetName,cell])
  const contextOwner=JSON.stringify([owner,workspace,path,threadId,view.objectId,view.revision,view.changeSequence ?? 0,selection?.token,contextCell,inputMode,form.value])
  const currentContext=useRef<string|null>(contextOwner);currentContext.current=active && editable && sameTarget ? contextOwner : null
  const applyContextMode=useRef<(mode:InputMode)=>void>(()=>{})
  const feedbackPanel=useAnchoredPanel<HTMLSpanElement>({enabled:active && toolbarTarget !== null,ownerKey:owner})
  const typeMenu=useAnchoredPanel<HTMLSpanElement>({enabled:editable,ownerKey:contextOwner})
  useEffect(()=>{
    const panel=typeMenu.panelRef.current
    if (typeMenu.open) (panel?.querySelector<HTMLButtonElement>('button[aria-checked="true"]:not(:disabled)') ?? panel?.querySelector<HTMLButtonElement>('button:not(:disabled)'))?.focus()
  },[typeMenu.open,typeMenu.panelRef])
  const address=draft ? `${draft.sheetName}!${column(draft.cell.column)}${draft.cell.row+1}`
    : selection?.ranges.map(range=>`${range.sheetName}!${column(range.startColumn)}${range.startRow+1}${range.endColumn === range.startColumn && range.endRow === range.startRow ? '' : `:${column(range.endColumn)}${range.endRow+1}`}`).join('；')
  useLayoutEffect(()=>{ensureNativeWorkbookDraftShutdownGuard();return ()=>{currentContext.current=null;useNativeOfficeStore.setState({workbookDraftComposing:false})}},[])
  useEffect(()=>{
    const intent=selection?.editIntent,identity=JSON.stringify([owner,intent])
    if (!intent || lastIntent.current === identity || !editable || !sameTarget) return
    lastIntent.current=identity;input.current?.focus();input.current?.select()
  },[owner,selection?.editIntent,editable,sameTarget])
  const update=(mode:InputMode,value:string)=>{
    if (!editable || !threadId || !draft && (!cell || !bounded)) return
    const type=mode === 'auto' ? workbookAutoInputType(value) : mode
    const base=draft ?? {id:crypto.randomUUID(),workspace,path,threadId,objectId:view.objectId,revision:view.revision,
      sheetName:bounded!.sheetName,cell:structuredClone(cell!),...initialValue(cell),changed:false}
    const original=initialValue(base.cell),edit=workbookDraftEdit(type,value)
    const originalEdit=workbookDraftEdit(base.cell.valueType,base.cell.valueType === 'formula' ? base.cell.formula : base.cell.valueType === 'number' ? String(base.cell.value) : base.cell.text)
    const next:NativeWorkbookCellDraft={...base,type,value,inputMode:mode,changed:(type !== original.type || value !== original.value) && (!edit || JSON.stringify(edit) !== JSON.stringify(originalEdit))}
    useNativeOfficeStore.getState().setWorkbookDraft(draftKey,next,draft?.id);setNotice(null)
  }
  const openTypeMenu=()=>{
    if (!editable || useNativeOfficeStore.getState().workbookDraftComposing) return false
    input.current?.focus({preventScroll:true});typeMenu.setOpen(true);return true
  }
  applyContextMode.current=mode=>{update(mode,form.value);typeMenu.setOpen(false);input.current?.focus({preventScroll:true})}
  useEffect(()=>{
    if (!onContextActionsChange) return
    if (!editable || !sameTarget || !selection?.token) {onContextActionsChange(null);return}
    const context:WorkbookCellContextActions={owner:contextOwner,objectId:view.objectId,revision:view.revision,
      changeSequence:view.changeSequence ?? 0,selectionToken:selection.token,actions:inputModes.map(mode=>({
        id:`workbook-input-${mode.value}`,label:mode.label,enabled:true,run:()=>{
          const state=useNativeOfficeStore.getState()
          const live=state.view,selected=live && currentSelection(live),liveCells=selected && nativeTypedWorkbookSelection(selected)
          if (currentContext.current !== contextOwner || state.workbookDraftInputFrozen || state.workbookDraftComposing ||
            state.workbookClosing[nativeOfficeTargetKey({workspace,path})] || state.workbookDrafts[draftKey] !== draft ||
            !live || live.kind !== 'xlsx' || live.protectedPresent || live.status !== 'ready' || live.saving || nativeOfficeHasUnresolvedChange(live,state.error) ||
            live.objectId !== context.objectId || live.revision !== context.revision ||
            (live.changeSequence ?? 0) !== context.changeSequence || selected?.token !== context.selectionToken ||
            !liveCells || liveCells.cells.length !== 1 || JSON.stringify([liveCells.sheetName,liveCells.cells[0]]) !== contextCell) return
          applyContextMode.current(mode.value)
        }
      }))}
    onContextActionsChange(context)
    return ()=>onContextActionsChange(null)
  },[onContextActionsChange,editable,sameTarget,selection?.token,contextOwner,contextCell,view.objectId,view.revision,view.changeSequence,workspace,path,draftKey,draft])
  const discard=()=>{
    if (!draft || pending || frozen || inputBlocked) return
    useNativeOfficeStore.getState().setWorkbookDraft(draftKey,null,draft.id)
    setNotice({owner:JSON.stringify([draftKey,view.objectId,view.revision,cell?.sheet,cell?.column,cell?.row]),text:'本地草稿已放弃。',error:false})
  }
  const keep=()=>{
    if (!draft || pending || frozen || inputBlocked) return
    useNativeOfficeStore.getState().setWorkbookDraft(draftKey,{...draft,closeRequested:false},draft.id)
    setNotice({owner:draft.id,text:'草稿已保留；切换文档不会丢失，保存仍需原单元格和原版本的有效选区。',error:false})
  }
  const save=async()=>{
    if (!canSave || !draft || !changed || !change || !threadId || !selection?.token) return
    const before=view,submitted=draft,submittedThread=threadId
    useNativeOfficeStore.getState().setWorkbookDraft(draftKey,{...submitted,pending:true,submitted:{edit:change}},submitted.id);setNotice(null)
    let response:NativeOfficeResponse | undefined
    try {response=await onRequest({action:'editWorkbookCell',objectId:view.objectId,revision:view.revision,
      expectedChangeSequence:view.changeSequence ?? 0,threadId,selectionToken:selection.token,edit:change})} catch { /* Keep unconfirmed words and forbid automatic resubmission. */ }
    const currentDraft=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    if (currentDraft?.id !== submitted.id) return
    const receipt=savedReceipt(response,before,submittedThread),current=latest.current
    if (receipt) {
      useNativeOfficeStore.getState().setWorkbookDraft(draftKey,null,submitted.id)
      if (current.active && current.threadId === submittedThread && current.view.objectId === before.objectId &&
        current.view.path === before.path && [before.revision,receipt.revision].includes(current.view.revision))
        setNotice({owner:JSON.stringify([draftKey,receipt.objectId]),revisions:[before.revision,receipt.revision],text:`${submitted.sheetName}!${column(submitted.cell.column)}${submitted.cell.row+1} 已保存`,error:false})
    } else {
      const unconfirmed=!response || response.ok || ['unknown','unavailable','save_failed','invalid_response'].includes(response.error ?? '')
      useNativeOfficeStore.getState().setWorkbookDraft(draftKey,{...currentDraft,pending:false,unconfirmed},submitted.id)
      if (current.view.objectId === before.objectId && current.threadId === submittedThread)
        setNotice({owner:submitted.id,text:saveError(response?.error),error:true})
    }
  }
  const sum=selection ? workbookSelectionSum(selection) : null,partial=!!selection && (!selection.capture?.complete || selection.capture.truncated)
  const hint=draft?.closeRequested ? '尚有未保存的单元格草稿。请保存、放弃或保留草稿；保留会取消关闭。'
    : draft && !sameTarget ? '草稿仍属于原会话、原单元格和原版本；当前会话、选区或版本已变化，文字已保留。请返回原会话并重新选择原单元格后保存，或明确放弃草稿。'
      : !view.workbookEditing?.editable ? '此工作簿仅支持只读预览；草稿保留，请在外部应用中编辑。'
        : !selection ? '单击单元格选择；F2 或双击后输入。' : !cell ? '选择单个可编辑单元格；区域和整行、整列仍可加入对话。'
          : unresolved || view.dirty || view.saving ? '修改结果待确认；请先核实已有修改。' : form.type === 'formula' ? '公式仅支持同表有界引用；不支持跨表或整列公式。'
            : form.type === 'empty' ? '保存后清空当前单元格。' : '本地草稿；点击绿色“保存”后写入文件。'
  const currentNotice=notice && (notice.owner === owner || notice.owner === JSON.stringify([draftKey,view.objectId]) && notice.revisions?.includes(view.revision)) ? notice : null
  const feedback=pending ? '正在保存；等待文件回执。' : draft?.closeRequested ? hint : currentNotice?.text ?? (changed && !change ? form.type === 'number' ? '请输入有限数值；文本请明确选择“文本”类型。'
    : form.type === 'text' ? '空文本请明确选择清空类型。' : '公式须以 = 开头，且不超过 1024 个字符。' : hint)
  const feedbackText=`${sum !== null ? `求和 ${sum.toLocaleString(undefined,{maximumSignificantDigits:15})} · ` : ''}${feedback}${partial ? ' 当前引用保留原始范围，捕获内容不完整，不显示整区求和。' : ''}`
  const feedbackError=!!(currentNotice?.error || draft?.closeRequested)
  const {setOpen:setFeedbackOpen}=feedbackPanel
  // Errors and close vetoes remain visible immediately, in an anchored surface
  // rather than a permanent second toolbar row. Regular status stays available
  // through the same labelled icon and its hover/focus explanation.
  useEffect(()=>{if (feedbackError) setFeedbackOpen(true)},[feedbackError,feedbackText,setFeedbackOpen])
  const actions=<>
    <WorkspaceViewerIconButton label={pending ? '保存中…' : '保存'} tone={canSave && changed && change ? 'commit' : undefined}
      disabled={!canSave || !changed || !change} onClick={()=>void save()}><Save aria-hidden="true" /></WorkspaceViewerIconButton>
    <WorkspaceViewerMenu label="单元格编辑选项" ownerKey={owner} enabled={active}>
      <button type="button" role="menuitem" aria-label="放弃草稿" disabled={!draft || pending || frozen || inputBlocked} onClick={discard}><Trash2 aria-hidden="true" />放弃草稿</button>
      {draft ? <button type="button" role="menuitem" aria-label="保留草稿" disabled={pending || frozen || inputBlocked} onClick={keep}><Check aria-hidden="true" />保留草稿</button> : null}
    </WorkspaceViewerMenu>
    <span ref={feedbackPanel.rootRef} className={styles.feedbackAnchor} data-feedback-error={feedbackError || undefined}>
      <WorkspaceViewerIconButton ref={feedbackPanel.triggerRef} label={feedbackText} className={styles.feedbackButton}
        aria-haspopup="dialog" aria-expanded={feedbackPanel.open} aria-controls={feedbackPanel.id}
        onClick={()=>setFeedbackOpen(!feedbackPanel.open)}><Info aria-hidden="true" /></WorkspaceViewerIconButton>
      <span className={styles.srOnly} aria-hidden={feedbackPanel.open || undefined} role={feedbackPanel.open ? undefined : feedbackError ? 'alert' : 'status'}>{feedbackText}</span>
      {feedbackPanel.open ? createPortal(<div ref={feedbackPanel.panelRef} id={feedbackPanel.id} role="dialog" aria-label="单元格编辑状态"
        className={styles.feedback} style={feedbackPanel.style}>
        <p role={feedbackError ? 'alert' : 'status'}>{sum !== null ? <output className={styles.sum} aria-label="完整选区求和">求和 {sum.toLocaleString(undefined,{maximumSignificantDigits:15})} · </output> : null}
          {feedback}{partial ? ' 当前引用保留原始范围，捕获内容不完整，不显示整区求和。' : ''}</p>
      </div>,document.body) : null}
    </span>
  </>
  return <div className={styles.editor} data-workbook-cell-editor data-workbook-draft-dirty={changed || pending || !!draft?.unconfirmed || undefined}>
    <div className={styles.controls}>
      <span className={styles.address} title={address} aria-label="工作簿选区">{address || '未选择单元格'}{partial ? '（部分捕获）' : ''}</span>
      <span className={styles.fx} aria-hidden="true">fx</span>
      <span ref={typeMenu.rootRef} className={styles.valueWrap} onBlurCapture={event=>{
        const next=event.relatedTarget
        if (next instanceof Node && (typeMenu.rootRef.current?.contains(next) || typeMenu.panelRef.current?.contains(next))) return
        typeMenu.setOpen(false)
      }}>
      <input ref={input} data-workbook-cell-input aria-label="单元格值或公式" className={styles.value} disabled={!editable} readOnly={inputMode === 'empty'}
        value={form.type === 'empty' ? '' : form.value} maxLength={inputMode !== 'auto' && form.type === 'formula' ? 1024 : 4096}
        aria-haspopup="menu" aria-expanded={typeMenu.open} aria-controls={typeMenu.id}
        placeholder={form.type === 'formula' ? '=SUM(A1:A2)' : form.type === 'number' ? '输入有限数值' : '输入文本'}
        onChange={event=>update(inputMode,event.target.value)}
        onContextMenu={event=>{if (openTypeMenu()) event.preventDefault()}}
        onCompositionStart={()=>useNativeOfficeStore.setState({workbookDraftComposing:true})} onCompositionEnd={()=>useNativeOfficeStore.setState({workbookDraftComposing:false})}
        onKeyDown={event=>{
          if (event.nativeEvent.isComposing) return
          if (!event.ctrlKey && !event.metaKey && !event.altKey && (event.key === 'ContextMenu' || event.key === 'F10' && event.shiftKey)) {
            if (openTypeMenu()) {event.preventDefault();event.stopPropagation()}
          } else if ((event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === 's') {event.preventDefault();void save()}
          else if (event.key === 'Escape') {event.preventDefault();keep()}
        }} />
      {typeMenu.open ? createPortal(<div ref={typeMenu.panelRef} id={typeMenu.id} role="menu" aria-label="单元格输入类型"
        className={styles.typeMenu} style={typeMenu.style} onKeyDown={event=>{
          if (event.nativeEvent.isComposing) return
          if (event.key === 'Escape') {event.preventDefault();event.stopPropagation();typeMenu.setOpen(false);input.current?.focus({preventScroll:true});return}
          if (!['ArrowDown','ArrowUp','Home','End'].includes(event.key)) return
          const items=[...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
          if (!items.length) return
          event.preventDefault();event.stopPropagation()
          const current=items.indexOf(document.activeElement as HTMLButtonElement)
          const next=event.key === 'Home' ? 0 : event.key === 'End' ? items.length-1 : (current+(event.key === 'ArrowUp' ? -1 : 1)+items.length)%items.length
          items[next].focus()
        }}>{inputModes.map(mode=><button key={mode.value} type="button" role="menuitemradio" aria-label={mode.label}
          aria-checked={inputMode === mode.value} data-workbook-input-mode={mode.value} onClick={()=>{
            update(mode.value,form.value);typeMenu.setOpen(false);input.current?.focus({preventScroll:true})
          }}>{mode.label}{inputMode === mode.value ? <Check aria-hidden="true" /> : null}</button>)}</div>,document.body) : null}
      </span>
      {toolbarTarget ? createPortal(actions,toolbarTarget) : toolbarTarget === undefined ? <span className={styles.fallbackActions}>{actions}</span> : null}
    </div>
  </div>
}
