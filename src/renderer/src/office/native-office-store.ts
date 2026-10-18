import { create } from 'zustand'
import { pluginPackageHostResponseSchema } from '../../../../packages/runtime/src/contracts/plugin-package-host'
import type { NativeOfficeView, NativeOfficeRequest, NativeOfficeResponse } from '@shared/native-office'
import { nativeOfficeCreateInputSchema, nativeOfficeCreationSchema, type NativeOfficeCreateInput, type NativeOfficeCreation } from '../../../../packages/runtime/src/contracts/native-office-editing'
import type { ToolResultLocalPresentFileRequestV1 } from '../../../../packages/runtime/src/contracts/tool-result-local-display'

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
  views: Record<string, NativeOfficeView>
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
  target: null, view: null, views: {}, error: null, receiveSequence: 0,
  proposalInFlight: false, annotationInputFrozen: false, manualDiscardAttempts: {}, manualOwners: {}, creationAttempt: null,
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
    set({ target: { workspace, path }, view: get().views[key(workspace, path)] ?? null, error: null })
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
        const views = {...state.views}
        if (views[identity] === view) delete views[identity]
        return {views,...(state.target === owner && owner?.protectedPresent && nativeOfficeTargetKey(owner) === identity ? {target:null,view:null,error:null} : {})}
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
    return { view, error, manualOwners, receiveSequence:state.receiveSequence + 1, ...(view && target ? { views: {...state.views, [nativeOfficeTargetKey(target)]: view} } : {}) }
  }),
  close: async (workspace, path) => {
    const identity = key(workspace, path), view = get().views[identity]
    if (view) {
      const result = await window.analytix.office.request({action:'close', objectId:view.objectId})
      if (!result.ok) { set({error:result.error ?? 'unavailable'}); return false }
    }
    set(state => {
      const views = {...state.views}; delete views[identity]
      const manualOwners = {...state.manualOwners}; if (view) delete manualOwners[view.objectId]
      return {views, manualOwners, ...(!state.target?.protectedPresent && state.target?.workspace === workspace && state.target.path === path ? {target:null, view:null, error:null} : {})}
    })
    return true
  }
}))
