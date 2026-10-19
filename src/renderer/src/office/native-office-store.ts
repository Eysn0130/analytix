import { create } from 'zustand'
import { pluginPackageHostResponseSchema } from '../../../../packages/runtime/src/contracts/plugin-package-host'
import type { NativeOfficeView, NativeOfficeRequest, NativeOfficeResponse } from '@shared/native-office'
import { nativeOfficeCreateInputSchema, nativeOfficeCreationSchema, type NativeOfficeCreateInput, type NativeOfficeCreation } from '../../../../packages/runtime/src/contracts/native-office-editing'
import type { ToolResultLocalPresentFileRequestV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { NativeWorkbookSelection, NativeWorkbookManualEdit } from '../../../../packages/runtime/src/contracts/native-office-editing'

type NativeOfficeCreateDocumentInput = Omit<Extract<NativeOfficeRequest, {action:'create'}>, 'action' | 'operationId'>
export type NativeOfficeCreateAttempt = {
  input: NativeOfficeCreateInput
  phase: 'pending' | 'unknown' | 'failed' | 'created'
  error?: string
  creation?: NativeOfficeCreation
  /** Receipt has been presented in its owning workspace tab, not native open success. */
  presented?: boolean
}
export function nativeOfficeCreateFileNameValid(fileName: string): boolean {
  return nativeOfficeCreateInputSchema.shape.fileName.safeParse(fileName).success
}

/** Transient form readiness only; Main/Core retain creation authority. */
export type NativeOfficeCreateReadiness = 'checking' | 'ready' | 'not-enabled' | 'unavailable' | 'unknown'
export async function readNativeOfficeCreateReadiness(): Promise<Exclude<NativeOfficeCreateReadiness,'checking'>> {
  try {
    const parsed = pluginPackageHostResponseSchema.safeParse(await window.analytix.packageHost.request({action:'list'}))
    if (!parsed.success || !parsed.data.ok || !('packages' in parsed.data)) return 'unknown'
    const packages = parsed.data.packages
    if (new Set(packages.map(pkg => pkg.packageId)).size !== packages.length) return 'unknown'
    const pkg = packages.find(pkg => pkg.packageId === 'analytix-documents')
    if (!pkg || !pkg.materialized || pkg.activationState === 'unavailable') return 'unavailable'
    if (pkg.activationState === 'unset' || pkg.desiredState === 'disabled') return 'not-enabled'
    return pkg.available && pkg.activationState === 'recorded' && pkg.desiredState === 'enabled' &&
      ['create-object','open-object','close-object'].every(operation => pkg.operations.includes(operation)) ? 'ready' : 'unavailable'
  } catch { return 'unknown' }
}
export function nativeOfficeCreateErrorKey(error: string | undefined): string {
  switch (error) {
    case 'forbidden': return 'nativeOfficeForbidden'
    case 'conflict': return 'workspaceWordNameConflict'
    case 'invalid_request': return 'workspaceWordInvalidRequest'
    case 'package_unavailable': return 'workspaceWordDocumentsNotReady'
    case 'package_changed': return 'workspaceWordDocumentsChanged'
    case 'capacity': return 'workspaceWordCapacity'
    default: return 'workspaceWordCreateFailed'
  }
}

const key = (workspace: string, path: string) => JSON.stringify([workspace, path])
/** Local input only, never a Core scope or a native mutation authority. */
export type NativeWorkbookCellDraft = {
  id:string; workspace:string; path:string; threadId:string; objectId:string; revision:string
  sheetName:string; cell:NativeWorkbookSelection['cells'][number]
  type:NativeWorkbookManualEdit['type']; value:string; changed:boolean
  pending?:boolean; unconfirmed?:boolean; closeRequested?:boolean
  submitted?:{edit:NativeWorkbookManualEdit;changeId?:string;saveOperationId?:string}
}
export const nativeWorkbookDraftKey = (workspace:string,path:string,threadId:string|null) => JSON.stringify([workspace,path,threadId])
export const nativeWorkbookDraftUnsettled = (draft:NativeWorkbookCellDraft) => draft.changed || !!draft.pending || !!draft.unconfirmed
export function nativeWorkbookDraftCellMatches(left:NativeWorkbookCellDraft['cell'],right:NativeWorkbookCellDraft['cell']) {
  return (['sheet','column','row','text','formula','value','valueType','numberFormat','rowVisible','columnVisible','merged'] as const)
    .every(field=>left[field] === right[field])
}
function workspaceViews(engineViews:Record<string,NativeOfficeView>,drafts:Record<string,NativeWorkbookCellDraft>) {
  return Object.fromEntries(Object.entries(engineViews).map(([identity,view]) => [identity,
    !view.protectedPresent && view.kind === 'xlsx' && Object.values(drafts).some(d => key(d.workspace,d.path) === identity && nativeWorkbookDraftUnsettled(d))
      ? {...view,dirty:true} : view]))
}
function matchingWorkbookDraftChange(draft:NativeWorkbookCellDraft,view:NativeOfficeView) {
  if (!draft.submitted || view.protectedPresent || draft.objectId !== view.objectId) return undefined
  return [view.recovery?.current,view.recovery?.pending].find(change => {
    if (!change || !('version' in change) || change.version !== 3 || change.origin !== 'manual' || change.threadId !== draft.threadId ||
      change.baseRevision !== draft.revision || !change.workbook || draft.submitted?.changeId && draft.submitted.changeId !== change.changeId ||
      draft.submitted?.saveOperationId && draft.submitted.saveOperationId !== change.saveOperationId) return false
    const index=change.workbook.before.cells.findIndex(cell=>cell.sheet === draft.cell.sheet && cell.column === draft.cell.column && cell.row === draft.cell.row)
    const before=change.workbook.before.cells[index],after=change.workbook.after.cells[index],edit=draft.submitted!.edit
    if (!before || !after || change.workbook.before.sheetName !== draft.sheetName || !nativeWorkbookDraftCellMatches(before,draft.cell) || after.valueType !== edit.type) return false
    // Conservative equality only; no frontend formula parsing or coercion.
    return edit.type === 'number' ? after.value === edit.value : edit.type === 'text' ? after.text === edit.text
      : edit.type === 'formula' ? after.formula === edit.formula : after.text === '' && after.formula === '' && after.value === 0
  })
}
const presentCloses = new Map<string,number>()
/** In-memory identities of explicit discards whose replies may be lost. No file authority. */
export type NativeOfficeManualDiscardAttempt = {owner: string; objectId: string; threadId: string; changeId: string; baseRevision: string}
/** Exact successful begin receipt remembered across Panel mounts; never a Core grant. */
export type NativeOfficeManualOwner = {owner: string; objectId: string; threadId: string; changeId: string; baseRevision: string; generation: number}
export type NativeOfficeTarget = { workspace: string; path: string; protectedPresent?: ToolResultLocalPresentFileRequestV1 }
const presentKey = (s: ToolResultLocalPresentFileRequestV1) => JSON.stringify([s.threadId,s.turnId,s.callId,s.resultItemId,s.viewId,s.snapshotDigest,s.index])
export const nativeOfficeTargetKey = (target: NativeOfficeTarget) => target.protectedPresent ? JSON.stringify(['present',presentKey(target.protectedPresent)]) : key(target.workspace,target.path)
export function nativeOfficeTargetMatches(target: NativeOfficeTarget, view: NativeOfficeView | null): boolean {
  if (!view) return false
  if (target.protectedPresent) return !!view.protectedPresent && presentKey(target.protectedPresent) === presentKey(view.protectedPresent)
  return !view.protectedPresent && (view.path === target.path || view.path === `${target.workspace.replace(/\/$/, '')}/${target.path}`)
}
/** Annotation mode alone never proves manual native input authority. */
export function nativeOfficeManualPhase(view: NativeOfficeView | null): 'unavailable' | 'preview' | 'active' | 'frozen' {
  if (!view || view.protectedPresent || view.kind !== 'docx' || view.status !== 'ready' ||
    ['running','paused'].includes(view.playback?.phase ?? '')) return 'unavailable'
  const manual = view.manual
  if (!manual) return 'preview'
  if (manual.version !== 2 || manual.origin !== 'manual' || manual.baseRevision !== view.revision || !view.editing) return 'unavailable'
  return manual.frozen || view.saving ? 'frozen' : 'active'
}
/** An unknown operation is queried through its existing Host operation, never resubmitted. */
export function nativeOfficeHasUnresolvedChange(view: NativeOfficeView | null, error: string | null): boolean {
  return [error,view?.error].some(code => code === 'unknown' || code === 'conflict') ||
    [view?.recovery?.pending,view?.recovery?.current].some(change => change?.status === 'unknown' || change?.status === 'conflict')
}
/** Protected-local presentation metadata only. Core owns sessions and receipts. */
export const useNativeOfficeStore = create<{
  target: NativeOfficeTarget | null
  view: NativeOfficeView | null
  /** Exact Host snapshots; only views below project local draft dirtiness. */
  engineViews: Record<string, NativeOfficeView>
  views: Record<string, NativeOfficeView>
  workbookDrafts: Record<string, NativeWorkbookCellDraft>
  workbookDraftInputFrozen:boolean
  workbookDraftComposing:boolean
  workbookClosing:Record<string,true>
  setWorkbookDraft:(key:string,draft:NativeWorkbookCellDraft|null,expectedId?:string) => void
  error: string | null
  receiveSequence: number
  proposalInFlight: boolean
  annotationInputFrozen: boolean
  manualDiscardAttempts: Record<string, NativeOfficeManualDiscardAttempt>
  manualOwners: Record<string, NativeOfficeManualOwner>
  creationAttempt: NativeOfficeCreateAttempt | null
  createDocument: (input: NativeOfficeCreateDocumentInput) => Promise<NativeOfficeResponse | undefined>
  beginProposalPoll: () => boolean
  endProposalPoll: () => void
  select: (workspace: string, path: string, isCurrent?: () => boolean) => Promise<boolean>
  selectPresent: (workspace: string, path: string, selector: ToolResultLocalPresentFileRequestV1, isCurrent?: () => boolean) => Promise<boolean>
  closePresent: (selector: ToolResultLocalPresentFileRequestV1) => Promise<boolean>
  receive: (view: NativeOfficeView | null, error?: string | null) => void
  close: (workspace: string, path: string) => Promise<boolean>
}>((set, get) => ({
  target: null, view: null, engineViews:{}, views: {}, workbookDrafts:{}, workbookDraftInputFrozen:false,workbookDraftComposing:false,workbookClosing:{},error: null, receiveSequence: 0,
  proposalInFlight: false, annotationInputFrozen: false, manualDiscardAttempts: {}, manualOwners: {}, creationAttempt: null,
  setWorkbookDraft:(identity,draft,expectedId) => {
    set(state => {
      if (expectedId && state.workbookDrafts[identity]?.id !== expectedId) return {}
      const previous=state.workbookDrafts[identity]
      if (draft && (state.workbookDraftInputFrozen || state.workbookClosing[key(draft.workspace,draft.path)]) &&
        (!previous || previous.id !== draft.id || previous.type !== draft.type || previous.value !== draft.value)) return {}
      const workbookDrafts={...state.workbookDrafts}
      if (draft) workbookDrafts[identity]=draft
      else delete workbookDrafts[identity]
      return {workbookDrafts,views:workspaceViews(state.engineViews,workbookDrafts)}
    })
  },
  createDocument: async input => {
    const prior = get().creationAttempt
    if (prior?.phase === 'pending') return undefined
    const retry = !!prior && prior.phase !== 'created' && prior.input.threadId === input.threadId && prior.input.fileName === input.fileName
    // An unresolved request can only replay its original identity and payload.
    if (prior?.phase === 'unknown' && !retry) return undefined
    const recoveringUnknown = retry && prior!.phase === 'unknown'
    const operationId = retry ? prior!.input.operationId : crypto.randomUUID()
    const candidate = {threadId:input.threadId,operationId,fileName:input.fileName}
    const parsed = nativeOfficeCreateInputSchema.safeParse(candidate)
    if (!parsed.success) {
      set({creationAttempt:{input:candidate,phase:'failed',error:'invalid_request'}})
      return {ok:false,view:null,error:'invalid_request'}
    }
    const attempt = {input:parsed.data,phase:'pending' as const}
    set({creationAttempt:attempt})
    const finish = (next: NativeOfficeCreateAttempt) => set(state =>
      state.creationAttempt?.input.operationId === operationId ? {creationAttempt:next} : {})
    try {
      const result = await window.analytix.office.request({action:'create',...parsed.data,bounds:input.bounds,...(input.appearance ? {appearance:input.appearance} : {})})
      const receipt = nativeOfficeCreationSchema.safeParse(result.creation)
      if (receipt.success && receipt.data.operationId === operationId &&
        receipt.data.path.replaceAll('\\','/').split('/').at(-1) === parsed.data.fileName) {
        // Creation may be committed even when opening its native view failed.
        finish({...attempt,phase:'created',creation:receipt.data,...(!result.ok ? {error:result.error ?? 'unavailable'} : {})})
        return {...result,creation:receipt.data}
      }
      const {creation: _unverified, ...response} = result
      const error = result.creation || result.ok ? 'invalid_response' : result.error ?? 'unknown'
      const definiteRefusal = ['invalid_request','forbidden','conflict','capacity','package_unavailable','package_changed'].includes(error)
      // A refusal of this replay cannot disprove an earlier uncertain creation.
      finish({...attempt,phase:!recoveringUnknown && definiteRefusal ? 'failed' : 'unknown',error})
      return {...response,ok:false,error}
    } catch {
      finish({...attempt,phase:'unknown',error:'unknown'})
      return {ok:false,view:null,error:'unknown'}
    }
  },
  beginProposalPoll: () => {
    if (get().proposalInFlight) return false
    set({proposalInFlight:true})
    return true
  },
  endProposalPoll: () => set({proposalInFlight:false}),
  select: async (workspace, path, isCurrent = () => true) => {
    if (!isCurrent()) return false
    const old = get().target
    if (!old?.protectedPresent && old?.workspace === workspace && old.path === path) return true
    if (typeof window !== 'undefined') await window.analytix?.office?.request({ action: 'hide' }).catch(() => undefined)
    if (!isCurrent()) return false
    set({ target: { workspace, path }, view: get().engineViews[key(workspace, path)] ?? get().views[key(workspace,path)] ?? null, error: null })
    return true
  },
  selectPresent: async (workspace,path,selector,isCurrent = () => true) => {
    if (!isCurrent()) return false
    const target = {workspace,path,protectedPresent:structuredClone(selector)}, identity = nativeOfficeTargetKey(target)
    if (!presentCloses.has(identity) && get().target?.protectedPresent && nativeOfficeTargetKey(get().target!) === identity) return true
    if (typeof window !== 'undefined') await window.analytix?.office?.request({action:'hide'}).catch(() => undefined)
    if (!isCurrent()) return false
    set({target,view:null,error:null}); return true
  },
  closePresent: async selector => {
    const identity = nativeOfficeTargetKey({workspace:'',path:'',protectedPresent:selector}), view = get().views[identity], owner = get().target
    presentCloses.set(identity,(presentCloses.get(identity) ?? 0)+1)
    try {
      if (view) {
        const result = await window.analytix.office.request({action:'close',objectId:view.objectId})
        if (!result.ok) return false
      } else if (owner?.protectedPresent && nativeOfficeTargetKey(owner) === identity) {
        await window.analytix.office.request({action:'hide'}).catch(() => undefined)
      }
      set(state => {
        const views = {...state.views},engineViews={...state.engineViews}
        if (views[identity] === view) {delete views[identity];delete engineViews[identity]}
        return {views,engineViews,...(state.target === owner && owner?.protectedPresent && nativeOfficeTargetKey(owner) === identity ? {target:null,view:null,error:null} : {})}
      })
      return true
    } finally { const count = presentCloses.get(identity)!-1; if (count) presentCloses.set(identity,count); else presentCloses.delete(identity) }
  },
  receive: (view, error = null) => set(state => {
    const target = state.target
    // Late A updates must never replace B's title, content or selection.
    if (view && !target) return {}
    if (view && target && !nativeOfficeTargetMatches(target,view)) return {}
    const previous = state.view
    if (view && previous?.manual && view.manual && previous.objectId === view.objectId && previous.revision === view.revision &&
      previous.manual.changeId === view.manual.changeId && previous.manual.generation === view.manual.generation &&
      (view.changeSequence ?? 0) < (previous.changeSequence ?? 0)) return {}
    const owner = view && state.manualOwners[view.objectId]
    let manualOwners = state.manualOwners
    if (view && !view.protectedPresent && target && !target.protectedPresent && owner && owner.owner === nativeOfficeTargetKey(target) && (!view.manual || owner.changeId !== view.manual.changeId || owner.generation !== view.manual.generation || owner.baseRevision !== view.revision)) {
      manualOwners = {...manualOwners}; delete manualOwners[view.objectId]
    }
    const engineViews=view && target ? {...state.engineViews,[nativeOfficeTargetKey(target)]:view} : state.engineViews
    const workbookDrafts={...state.workbookDrafts}
    if (view) for (const [identity,draft] of Object.entries(workbookDrafts)) {
      const change=matchingWorkbookDraftChange(draft,view)
      if (!change) continue
      if (!draft.pending && draft.unconfirmed && !view.dirty && !view.saving && change.status === 'committed' && change.revision === view.revision)
        delete workbookDrafts[identity]
      else workbookDrafts[identity]={...draft,submitted:{...draft.submitted!,changeId:change.changeId,saveOperationId:change.saveOperationId}}
    }
    return { view, error, manualOwners, engineViews, workbookDrafts, views:workspaceViews(engineViews,workbookDrafts), receiveSequence:state.receiveSequence + 1 }
  }),
  close: async (workspace, path) => {
    const identity = key(workspace, path), view = get().engineViews[identity] ?? get().views[identity]
    if (get().workbookClosing[identity]) return false
    const drafts=Object.entries(get().workbookDrafts).filter(([,draft]) => key(draft.workspace,draft.path) === identity && nativeWorkbookDraftUnsettled(draft))
    if (drafts.length) {
      for (const [draftKey,draft] of drafts) get().setWorkbookDraft(draftKey,{...draft,closeRequested:true},draft.id)
      return false
    }
    set(state=>({workbookClosing:{...state.workbookClosing,[identity]:true}}))
    try {
      if (view) {
        const result = await window.analytix.office.request({action:'close', objectId:view.objectId})
        if (!result.ok) { set({error:result.error ?? 'unavailable'}); return false }
      }
      if (Object.values(get().workbookDrafts).some(draft=>key(draft.workspace,draft.path) === identity && nativeWorkbookDraftUnsettled(draft))) return false
      set(state => {
        const views = {...state.views},engineViews={...state.engineViews}; delete views[identity];delete engineViews[identity]
        const workbookDrafts=Object.fromEntries(Object.entries(state.workbookDrafts).filter(([,draft]) => key(draft.workspace,draft.path)!==identity))
        const manualOwners = {...state.manualOwners}; if (view) delete manualOwners[view.objectId]
        return {views, engineViews, workbookDrafts, manualOwners, ...(!state.target?.protectedPresent && state.target?.workspace === workspace && state.target.path === path ? {target:null, view:null, error:null} : {})}
      })
      return true
    } catch { set({error:'unavailable'});return false }
    finally {set(state=>{const workbookClosing={...state.workbookClosing};delete workbookClosing[identity];return {workbookClosing}})}
  }
}))
