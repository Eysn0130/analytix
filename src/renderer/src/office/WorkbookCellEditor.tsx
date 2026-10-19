import { useEffect, useRef, useState } from 'react'
import { nativeTypedWorkbookSelection, type NativeOfficeRequest, type NativeOfficeResponse, type NativeOfficeSelection, type NativeOfficeView } from '@shared/native-office'
import styles from './workbook-cell-editor.module.css'

type CellSelection = Extract<NativeOfficeSelection, {kind:'cells'}>
type CellEdit = Extract<NativeOfficeRequest, {action:'editWorkbookCell'}>['edit']
type Draft = {owner:string; type:CellEdit['type']; value:string}
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
function cellOwner(view:NativeOfficeView, threadId:string | null):string {
  return JSON.stringify([threadId,view.objectId,view.path,view.revision,view.changeSequence ?? 0,currentSelection(view)?.token,
    currentSelection(view)?.ranges,currentSelection(view)?.cells])
}
function initialDraft(owner:string, selection:CellSelection | null):Draft {
  const range = selection?.ranges[0]
  const cell = selection?.ranges.length === 1 && range?.startColumn === range?.endColumn && range?.startRow === range?.endRow && selection.cells?.length === 1 ? selection.cells[0] : undefined
  const type = cell?.valueType === 'empty' ? 'text' : cell?.valueType ?? 'text'
  return {owner,type,value:cell ? type === 'formula' ? cell.formula : type === 'number' ? String(cell.value) : cell.text : ''}
}
export function workbookDraftEdit(type:CellEdit['type'], value:string):CellEdit | null {
  if (type === 'empty') return {type:'empty'}
  if (type === 'text') return value.length > 0 && value.length <= 4096 ? {type:'text',text:value} : null
  if (type === 'formula') return value.startsWith('=') && value.length > 1 && value.length <= 1024 ? {type:'formula',formula:value} : null
  if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value.trim())) return null
  const number = Number(value)
  return Number.isFinite(number) ? {type:'number',value:number} : null
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

export function WorkbookCellEditor({view,threadId,active,busy,onRequest}:{
  view:NativeOfficeView; threadId:string | null; active:boolean; busy:boolean
  onRequest:(request:NativeOfficeRequest) => Promise<NativeOfficeResponse | undefined>
}) {
  const selection = currentSelection(view), owner = cellOwner(view,threadId)
  const [draft,setDraft] = useState<Draft>(() => initialDraft(owner,selection))
  const [editing,setEditing] = useState(false)
  const [pending,setPending] = useState(false)
  const [unconfirmedOwner,setUnconfirmedOwner] = useState<string | null>(null)
  const [notice,setNotice] = useState<{owner:string;revision:string;token?:string;text:string;error:boolean} | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const latest = useRef({view,threadId,active,owner}); latest.current = {view,threadId,active,owner}
  const lastIntent = useRef('')
  const bounded = selection && nativeTypedWorkbookSelection(selection)
  const single = !!(bounded && bounded.cells.length === 1)
  const unresolved = [view.recovery?.pending,view.recovery?.current].some(change => ['prepared','unknown','conflict'].includes(change?.status ?? ''))
  const available = !!(active && threadId && view.kind === 'xlsx' && view.status === 'ready' && !view.protectedPresent &&
    view.workbookEditing?.editable && single && !view.dirty && !view.saving && !unresolved)
  const editable = available && !busy && !pending && unconfirmedOwner !== owner
  const currentDraft = draft.owner === owner ? draft : initialDraft(owner,selection)
  const change = workbookDraftEdit(currentDraft.type,currentDraft.value)
  const baseline = initialDraft(owner,selection)
  const original = single && bounded ? bounded.cells[0] : null
  const originalEdit = original ? workbookDraftEdit(original.valueType,original.valueType === 'formula' ? original.formula
    : original.valueType === 'number' ? String(original.value) : original.text) : null
  const changed = editing && (currentDraft.type !== baseline.type || currentDraft.value !== baseline.value) &&
    (!change || JSON.stringify(change) !== JSON.stringify(originalEdit))
  useEffect(() => {
    setDraft(initialDraft(owner,currentSelection(latest.current.view))); setEditing(false); setUnconfirmedOwner(null)
  // The owner includes the entire exact capture; focus alone does not retain a draft.
  }, [owner])
  useEffect(() => {
    const intent = selection?.editIntent, identity = JSON.stringify([owner,intent])
    if (!intent || lastIntent.current === identity || !editable) return
    lastIntent.current = identity; setEditing(true); input.current?.focus(); input.current?.select()
  }, [owner,selection?.editIntent,editable])
  const update = (value:string) => { setDraft({...currentDraft,value}); setEditing(true); setNotice(null) }
  const cancel = () => { setDraft(initialDraft(owner,selection)); setEditing(false); setNotice(null) }
  const save = async () => {
    if (!editable || !changed || !change || !threadId || !selection?.token || latest.current.owner !== owner) return
    const before = view, submittedOwner = owner, submittedThread = threadId
    setPending(true); setNotice(null)
    let response:NativeOfficeResponse | undefined
    try {
      response = await onRequest({action:'editWorkbookCell',objectId:view.objectId,revision:view.revision,
        expectedChangeSequence:view.changeSequence ?? 0,threadId,selectionToken:selection.token,edit:change})
    } catch { /* An uncertain native outcome is never resubmitted here. */ }
    finally { setPending(false) }
    const current = latest.current, receipt = savedReceipt(response,before,submittedThread)
    if (!current.active || current.threadId !== submittedThread || current.view.objectId !== before.objectId || current.view.path !== before.path) return
    if (receipt) {
      if (current.view.revision !== before.revision && current.view.revision !== receipt.revision ||
        current.view.revision === before.revision && current.owner !== submittedOwner ||
        current.view.revision === receipt.revision && current.view.selection?.token !== receipt.selection?.token) return
      setEditing(false)
      setNotice({owner:submittedOwner,revision:receipt.revision,token:receipt.selection?.token,text:'已保存',error:false})
    } else if (current.owner === submittedOwner) {
      if (!response || response.ok || ['unknown','unavailable','save_failed','invalid_response'].includes(response.error ?? '')) setUnconfirmedOwner(submittedOwner)
      setNotice({owner:submittedOwner,revision:before.revision,token:selection.token,text:saveError(response?.error),error:true})
    }
  }
  const address = selection?.ranges.map(range => `${range.sheetName}!${column(range.startColumn)}${range.startRow + 1}${range.endColumn === range.startColumn && range.endRow === range.startRow ? '' : `:${column(range.endColumn)}${range.endRow + 1}`}`).join('；')
  const sum = selection ? workbookSelectionSum(selection) : null
  const partial = !!selection && (!selection.capture?.complete || selection.capture.truncated)
  const currentNotice = notice && (notice.owner === owner || notice.revision === view.revision && notice.token === selection?.token) ? notice : null
  const hint = !view.workbookEditing?.editable ? '此工作簿仅支持只读预览；请在外部应用中编辑。'
    : !selection ? '单击单元格选择；F2 或双击后输入。' : !single ? '选择单个可编辑单元格；区域和整行、整列仍可加入对话。'
      : unresolved || view.dirty || view.saving ? '修改结果待确认；请先核实已有修改。' : currentDraft.type === 'formula' ? '公式仅支持同表有界引用；不支持跨表或整列公式。'
        : currentDraft.type === 'empty' ? '保存后清空当前单元格。' : '本地草稿；点击绿色“保存”后写入文件。'
  return <div className={styles.editor} data-workbook-cell-editor>
    <div className={styles.controls}>
      <span className={styles.address} title={address} aria-label="工作簿选区">{address || '未选择单元格'}{partial ? '（部分捕获）' : ''}</span>
      <select aria-label="单元格输入类型" value={currentDraft.type} disabled={!editable} onChange={event => {
        const type = event.target.value as CellEdit['type']
        setDraft({...currentDraft,type}); setEditing(true); setNotice(null)
        if (type !== 'empty') input.current?.focus()
      }}>
        <option value="text">文本</option><option value="number">数值</option><option value="formula">公式</option><option value="empty">清空</option>
      </select>
      <input ref={input} data-workbook-cell-input aria-label="单元格值或公式" className={styles.value} disabled={!editable || currentDraft.type === 'empty'}
        value={currentDraft.type === 'empty' ? '' : currentDraft.value} maxLength={currentDraft.type === 'formula' ? 1024 : 4096}
        placeholder={currentDraft.type === 'formula' ? '=SUM(A1:A2)' : currentDraft.type === 'number' ? '输入有限数值' : '输入文本'}
        onFocus={() => { if (editable) setEditing(true) }} onChange={event => update(event.target.value)}
        onKeyDown={event => { if (event.nativeEvent.isComposing) return; if (event.key === 'Escape') {event.preventDefault();cancel()} }} />
      <button type="button" className={styles.save} disabled={!editable || !changed || !change} onClick={() => void save()}>{pending ? '保存中…' : '保存'}</button>
      <button type="button" className={styles.cancel} disabled={!editable || !editing} onClick={cancel}>取消</button>
    </div>
    <div className={styles.feedback} role={currentNotice?.error ? 'alert' : 'status'}>
      {sum !== null ? <output className={styles.sum} aria-label="完整选区求和">求和 {sum.toLocaleString(undefined,{maximumSignificantDigits:15})} · </output> : null}
      {pending ? '正在保存；等待文件回执。' : currentNotice?.text ?? (changed && !change ? currentDraft.type === 'number' ? '请输入有限数值；文本请明确选择“文本”类型。'
        : currentDraft.type === 'text' ? '空文本请明确选择清空类型。' : '公式须以 = 开头，且不超过 1024 个字符。' : hint)}
      {partial ? ' 当前引用保留原始范围，捕获内容不完整，不显示整区求和。' : ''}
    </div>
  </div>
}
