import {
  objectEditingResponseSchema,
  type ImageAnnotation, type ImageObjectSnapshot, type ImageRegion, type ImageRegionScope,
  type ObjectEditingRequest
} from '../../../../packages/runtime/src/contracts/object-editing'
import type { WriteWorkspaceGet, WriteWorkspaceSet } from './write-workspace-store-types'
import { useNativeReferenceStore } from '../office/native-reference-store'
import i18n from '../i18n'

type ImageRegionDraft = { regionId: string; region: ImageRegion | null; note: string }
type ImageWrite = Extract<ObjectEditingRequest, { action: 'image-annotation-write' }>
export type ImageRegionEditor = {
  workspace: string; path: string; threadId: string
  snapshot: ImageObjectSnapshot | null; annotation: ImageAnnotation | null
  regions: ImageRegionDraft[]; selectedRegionId: string | null
  region: ImageRegion | null; note: string; dirty: boolean; pending: ImageWrite | null
  loading: boolean; revoked: boolean; stale: boolean; composing: boolean
  status: 'saved' | 'dirty' | 'saving' | 'unknown' | 'conflict' | 'error'; error: string | null
}
export const imageRegionHasDraft = (editor: ImageRegionEditor | null): boolean => !!editor && (editor.dirty || !!editor.pending || editor.composing)
const sameRegion = (a: ImageRegion | null, b: ImageRegion | null): boolean =>
  a === null ? b === null : b !== null && a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height
const sameRegions = (a: ImageRegionDraft[], b: ImageRegionDraft[]): boolean => a.length === b.length && a.every((entry, i) =>
  entry.regionId === b[i].regionId && entry.note === b[i].note && sameRegion(entry.region, b[i].region))
const sameWrite = (annotation: ImageAnnotation, write: ImageWrite): boolean => annotation.current &&
  annotation.sourceRevision === write.sourceRevision && sameRegions(annotation.regions, write.regions)
const selection = (regions: ImageRegionDraft[], selectedRegionId: string | null) => {
  const selected = regions.find(entry => entry.regionId === selectedRegionId)
  return { regions, selectedRegionId: selected?.regionId ?? null, region: selected?.region ?? null, note: selected?.note ?? '' }
}
const newRegionId = (): string => Array.from(crypto.getRandomValues(new Uint8Array(24)), value => value.toString(16).padStart(2, '0')).join('')

export type ImagePreviewRotation = 0 | 90 | 180 | 270
/** Invert the preview rotation after mapping its bounding box; saved geometry
 * always names the Core snapshot's original pixels, not the display transform. */
export function imagePoint(clientX: number, clientY: number, box: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>,
  image: Pick<ImageObjectSnapshot, 'width' | 'height'>, rotation: ImagePreviewRotation = 0): { x: number; y: number } | null {
  if (!(box.width > 0 && box.height > 0) || !Number.isFinite(clientX + clientY)) return null
  const u = Math.max(0, Math.min(1, (clientX - box.left) / box.width))
  const v = Math.max(0, Math.min(1, (clientY - box.top) / box.height))
  const [x, y] = rotation === 90 ? [v, 1 - u] : rotation === 180 ? [1 - u, 1 - v] : rotation === 270 ? [1 - v, u] : [u, v]
  return { x: Math.round(x * image.width), y: Math.round(y * image.height) }
}
export function imageRectangle(a: { x: number; y: number }, b: { x: number; y: number }): ImageRegion | null {
  const width = Math.abs(a.x - b.x), height = Math.abs(a.y - b.y)
  return width && height ? { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y), width, height } : null
}
async function request(input: ObjectEditingRequest) {
  return objectEditingResponseSchema.parse(await window.analytix.objects.request(input))
}
const fail = (key: string): string => i18n.t(key)

export type ImageRegionActions = {
  imageRegionEditor: ImageRegionEditor | null
  imageRegionThreadId: string | null
  setImageRegionThread: (threadId: string | null) => void
  invalidateImageRegion: (release?: boolean) => void
  setImageRegionComposing: (composing: boolean) => void
  openImageRegion: (workspace: string, path: string, threadId: string, recapture?: boolean) => Promise<boolean>
  updateImageRegion: (patch: { region?: ImageRegion | null; note?: string }) => void
  selectImageRegion: (regionId: string | null) => void
  removeImageRegion: () => void
  flushImageRegion: () => Promise<boolean>
  closeImageRegion: () => Promise<boolean>
  captureImageRegion: () => Promise<ImageRegionScope | null>
}

/** Local draft state only. Every open/read/write/capture is authorized by Core. */
export function createImageRegionActions(set: WriteWorkspaceSet, get: WriteWorkspaceGet): ImageRegionActions {
  let epoch = 0
  let opening: Promise<boolean> | null = null
  let saving: Promise<boolean> | null = null
  let closing: Promise<boolean> | null = null
  const update = (patch: Partial<ImageRegionEditor>) => {
    const editor = get().imageRegionEditor
    if (editor) set({ imageRegionEditor: { ...editor, ...patch } })
  }
  const owns = (editor: ImageRegionEditor, generation: number) => generation === epoch &&
    get().workspaceRoot === editor.workspace && get().activeFilePath === editor.path &&
    get().imageRegionThreadId === editor.threadId && !get().imageRegionEditor?.revoked
  const invalidate = (release = false) => {
    epoch++
    const editor = get().imageRegionEditor
    if (editor?.snapshot) useNativeReferenceStore.getState().revokeScopes(editor.snapshot.objectId)
    if (editor) update({ revoked: true, loading: false })
    if (release && editor?.snapshot) {
      const operation = request({ action: 'close', sessionId: editor.snapshot.sessionId }).then(() => true, () => false)
      closing = operation
      void operation.finally(() => { if (closing === operation) closing = null })
    }
  }
  return {
    imageRegionEditor: null as ImageRegionEditor | null,
    imageRegionThreadId: null as string | null,
    setImageRegionThread: (threadId: string | null) => {
      if (get().imageRegionThreadId === threadId) return
      invalidate()
      set({ imageRegionThreadId: threadId })
    },
    invalidateImageRegion: invalidate,
    setImageRegionComposing: (composing: boolean) => update({ composing }),
    openImageRegion: async (workspace: string, path: string, threadId: string, recapture = false): Promise<boolean> => {
      if (get().shutdownFrozen || get().imageRegionThreadId !== threadId || get().workspaceRoot !== workspace || get().activeFilePath !== path) return false
      const generation = ++epoch
      if (closing) await closing
      if (opening) await opening
      if (saving) await saving
      if (generation !== epoch || get().shutdownFrozen) return false
      const previous = get().imageRegionEditor
      const sameOwner = previous?.workspace === workspace && previous.path === path && previous.threadId === threadId
      if (imageRegionHasDraft(previous) && (!sameOwner || previous?.composing || recapture && !!previous?.pending)) return false
      if (sameOwner && previous?.snapshot && !previous.revoked && !recapture) return true
      const preserved = sameOwner && imageRegionHasDraft(previous) ? previous : null
      const editor: ImageRegionEditor = { workspace, path, threadId, snapshot: null, annotation: null,
        ...selection(preserved?.regions ?? [], preserved?.selectedRegionId ?? null), dirty: preserved?.dirty ?? false,
        pending: preserved?.pending ?? null, loading: true, revoked: false, stale: false, composing: false, status: preserved ? 'dirty' : 'saved', error: null }
      set({ imageRegionEditor: editor })
      const operation = (async () => {
        let snapshot: ImageObjectSnapshot | null = null
        try {
          const opened = await request({ action: 'image-open', threadId, path })
          if (!opened.ok) throw new Error(opened.message)
          if (!('image' in opened) || opened.image.threadId !== threadId || opened.image.path !== path) throw new Error(fail('imageRegionUnavailable'))
          snapshot = opened.image
          if (!owns(editor, generation) || get().shutdownFrozen) return false
          const response = await request({ action: 'image-annotation-read', sessionId: snapshot.sessionId, threadId })
          if (!owns(editor, generation) || get().shutdownFrozen) return false
          if (!response.ok) throw new Error(response.message)
          if (!('annotation' in response) || response.annotation.objectId !== snapshot.objectId || response.annotation.threadId !== threadId) throw new Error(fail('imageRegionUnavailable'))
          const annotation = response.annotation
          const stale = !!annotation.annotationRevision && (!annotation.current || annotation.sourceRevision !== snapshot.sourceRevision ||
            annotation.width !== snapshot.width || annotation.height !== snapshot.height)
          const preserveRegion = preserved && preserved.snapshot?.sourceRevision === snapshot.sourceRevision && preserved.snapshot.objectId === snapshot.objectId
          const sourceChanged = preserved ? !preserveRegion : stale
          const drafts = preserved?.regions ?? annotation.regions
          const selectedId = (sameOwner ? previous?.selectedRegionId : null) ?? drafts[0]?.regionId ?? null
          const regions = sourceChanged ? drafts.map(entry => ({ ...entry, region: null }))
            : recapture ? drafts.map(entry => entry.regionId === selectedId ? { ...entry, region: null } : entry) : drafts
          update({ snapshot, annotation: preserved ? preserved.annotation : annotation,
            pending: preserved?.pending ? { ...preserved.pending, sessionId: snapshot.sessionId } : null,
            loading: false, stale: sourceChanged && !recapture,
            ...selection(regions, selectedId), dirty: !!preserved || recapture,
            status: preserved || recapture ? 'dirty' : 'saved' })
          return true
        } catch (error) {
          if (owns(editor, generation)) update({ loading: false, status: 'error', error: error instanceof Error ? error.message : fail('imageRegionUnavailable') })
          return false
        } finally {
          // Opens are serialized, so a discarded response cannot close a newer session.
          if (owns(editor, generation) && get().shutdownFrozen) update({ loading: false, revoked: true })
          if (snapshot && (!owns(editor, generation) || get().shutdownFrozen)) {
            await request({ action: 'close', sessionId: snapshot.sessionId }).catch(() => undefined)
          }
        }
      })()
      opening = operation
      try { return await operation } finally { if (opening === operation) opening = null }
    },
    updateImageRegion: (patch: { region?: ImageRegion | null; note?: string }) => {
      const editor = get().imageRegionEditor
      if (!editor?.snapshot || editor.revoked || editor.loading || get().shutdownFrozen || editor.threadId !== get().imageRegionThreadId) return
      const region = patch.region === undefined ? editor.region : patch.region
      if (region && (![region.x, region.y, region.width, region.height].every(Number.isInteger) || region.x < 0 || region.y < 0 ||
        region.width < 1 || region.height < 1 || region.x + region.width > editor.snapshot.width || region.y + region.height > editor.snapshot.height)) return
      const note = patch.note ?? editor.note
      let regions = editor.regions
      let selectedRegionId = editor.selectedRegionId
      if (region || note) {
        if (!selectedRegionId) {
          if (regions.length >= 8) return
          selectedRegionId = newRegionId()
          regions = [...regions, { regionId: selectedRegionId, region, note }]
        } else regions = regions.map(entry => entry.regionId === selectedRegionId ? { ...entry, region, note } : entry)
      } else {
        regions = regions.filter(entry => entry.regionId !== selectedRegionId)
        selectedRegionId = null
      }
      if (regions.reduce((sum, entry) => sum + entry.note.length, 0) > 4096 ||
        regions.reduce((sum, entry) => sum + new TextEncoder().encode(entry.note).length, 0) > 16384) return
      useNativeReferenceStore.getState().revokeScopes(editor.snapshot.objectId)
      const dirty = !!editor.pending || !sameRegions(regions, editor.annotation?.regions ?? []) ||
        regions.length > 0 && editor.annotation?.sourceRevision !== editor.snapshot.sourceRevision
      update({ ...selection(regions, selectedRegionId), dirty, stale: patch.region ? false : editor.stale,
        status: editor.pending ? 'unknown' : dirty ? 'dirty' : 'saved', error: null })
    },
    selectImageRegion: (regionId: string | null) => {
      const editor = get().imageRegionEditor
      if (!editor?.snapshot || editor.revoked || editor.loading || editor.composing || get().shutdownFrozen ||
        editor.threadId !== get().imageRegionThreadId || regionId !== null && !editor.regions.some(entry => entry.regionId === regionId)) return
      update(selection(editor.regions, regionId))
    },
    removeImageRegion: () => {
      const editor = get().imageRegionEditor
      if (!editor?.snapshot || !editor.selectedRegionId || editor.revoked || editor.loading || editor.composing ||
        get().shutdownFrozen || editor.threadId !== get().imageRegionThreadId) return
      const regions = editor.regions.filter(entry => entry.regionId !== editor.selectedRegionId)
      useNativeReferenceStore.getState().revokeScopes(editor.snapshot.objectId)
      const dirty = !!editor.pending || !sameRegions(regions, editor.annotation?.regions ?? [])
      update({ ...selection(regions, regions[0]?.regionId ?? null), dirty,
        status: editor.pending ? 'unknown' : dirty ? 'dirty' : 'saved', error: null })
    },
    flushImageRegion: async (): Promise<boolean> => {
      if (saving) {
        const saved = await saving
        if (!saved && (get().imageRegionEditor?.pending || get().imageRegionEditor?.status !== 'dirty')) return false
        return get().flushImageRegion()
      }
      const editor = get().imageRegionEditor
      if (!editor || !imageRegionHasDraft(editor)) return !editor?.loading
      if (editor.composing || editor.revoked || editor.loading || editor.stale || !editor.snapshot || !editor.annotation ||
        editor.threadId !== get().imageRegionThreadId || editor.regions.some(entry => !entry.region)) return false
      const generation = epoch, image = editor.snapshot
      let pending: ImageWrite = editor.pending ?? { action: 'image-annotation-write', sessionId: image.sessionId, threadId: editor.threadId,
        sourceRevision: image.sourceRevision, expectedAnnotationRevision: editor.annotation.annotationRevision,
        regions: editor.regions.map(entry => ({ ...entry, region: { ...entry.region! } })) }
      update({ pending, status: 'saving', error: null })
      const operation = (async () => {
        try {
          const attempt = async (input: ImageWrite | Extract<ObjectEditingRequest, { action: 'image-annotation-read' }>) => {
            let response = await request(input)
            if (!response.ok && response.code === 'session_invalid' && owns(editor, generation)) {
              const reopened = await request({ action: 'image-open', threadId: editor.threadId, path: editor.path })
              if (!owns(editor, generation)) throw new Error(fail('imageRegionSaveUnconfirmed'))
              if (!reopened.ok || !('image' in reopened) || reopened.image.objectId !== image.objectId ||
                reopened.image.threadId !== editor.threadId || reopened.image.path !== editor.path || reopened.image.sourceRevision !== image.sourceRevision ||
                reopened.image.width !== image.width || reopened.image.height !== image.height) throw new Error(fail('imageRegionRecapture'))
              pending = { ...pending, sessionId: reopened.image.sessionId }
              update({ snapshot: reopened.image, pending })
              response = await request({ ...input, sessionId: reopened.image.sessionId })
            }
            return response
          }
          // A lost ACK retains this exact CAS payload. Core may confirm the same
          // content on retry; a read can confirm it without granting an overwrite.
          let response
          if (editor.pending) {
            response = await attempt({ action: 'image-annotation-read', sessionId: pending.sessionId, threadId: pending.threadId })
            if (!owns(editor, generation)) return false
            if (!response.ok || !('annotation' in response)) throw new Error(fail('imageRegionSaveUnconfirmed'))
            const observed = response.annotation
            if (observed.objectId !== image.objectId || observed.threadId !== editor.threadId) throw new Error(fail('imageRegionSaveUnconfirmed'))
            if (!sameWrite(observed, pending)) {
              if (observed.annotationRevision !== pending.expectedAnnotationRevision) {
                update({ pending: null, status: 'conflict', error: fail('imageRegionStatus_conflict') })
                return false
              }
              response = await attempt(pending)
            }
          } else response = await attempt(pending)
          if (!owns(editor, generation)) return false
          if (!response.ok) {
            update({ status: response.code === 'conflict' ? 'conflict' : 'unknown',
              ...(response.code === 'conflict' ? { pending: null } : {}), error: response.message })
            return false
          }
          if (!('annotation' in response) || response.annotation.objectId !== image.objectId || response.annotation.threadId !== editor.threadId ||
            response.annotation.width !== image.width || response.annotation.height !== image.height || !sameWrite(response.annotation, pending)) throw new Error(fail('imageRegionSaveUnconfirmed'))
          const current = get().imageRegionEditor!
          const unchanged = sameRegions(current.regions, pending.regions)
          update({ annotation: response.annotation, pending: null, dirty: !unchanged, status: unchanged ? 'saved' : 'dirty', error: null })
          return unchanged
        } catch (error) {
          if (owns(editor, generation)) update({ status: 'unknown', error: error instanceof Error ? error.message : fail('imageRegionSaveUnconfirmed') })
          return false
        }
      })()
      saving = operation
      try { return await operation } finally { if (saving === operation) saving = null }
    },
    closeImageRegion: async (): Promise<boolean> => {
      if (closing) return closing
      const editor = get().imageRegionEditor, started = epoch
      const operation = (async () => {
        if (!(await get().flushImageRegion()) || started !== epoch) return false
        if (get().imageRegionEditor?.snapshot?.sessionId !== editor?.snapshot?.sessionId) return false
        invalidate()
        const generation = epoch
        if (opening) await opening
        if (editor?.snapshot) await request({ action: 'close', sessionId: editor.snapshot.sessionId }).catch(() => undefined)
        if (generation !== epoch) return false
        set({ imageRegionEditor: null })
        return true
      })()
      closing = operation
      try { return await operation } finally { if (closing === operation) closing = null }
    },
    captureImageRegion: async (): Promise<ImageRegionScope | null> => {
      const intended = get().imageRegionEditor, started = epoch
      if (get().shutdownFrozen || !intended?.selectedRegionId || !(await get().flushImageRegion())) return null
      const editor = get().imageRegionEditor
      if (started !== epoch || editor?.selectedRegionId !== intended.selectedRegionId ||
        editor?.snapshot?.objectId !== intended.snapshot?.objectId || editor?.snapshot?.sourceRevision !== intended.snapshot?.sourceRevision) return null
      if (!editor?.snapshot || !editor.annotation?.current || editor.revoked || editor.stale || !editor.region || !editor.selectedRegionId) return null
      const generation = epoch
      try {
        const response = await request({ action: 'image-scope-capture', sessionId: editor.snapshot.sessionId, threadId: editor.threadId,
          sourceRevision: editor.snapshot.sourceRevision, annotationRevision: editor.annotation.annotationRevision, regionId: editor.selectedRegionId })
        if (!owns(editor, generation) || get().shutdownFrozen || get().imageRegionEditor !== editor) {
          if (response.ok && 'scope' in response && 'kind' in response.scope) void request({ action: 'image-scope-revoke',
            sessionId: response.scope.sessionId, threadId: response.scope.threadId, scopeId: response.scope.scopeId }).catch(() => undefined)
          return null
        }
        if (!response.ok) { update({ error: response.message }); return null }
        if (!('scope' in response) || !('kind' in response.scope)) return null
        const scope = response.scope
        if (scope.sessionId !== editor.snapshot.sessionId || scope.objectId !== editor.snapshot.objectId || scope.threadId !== editor.threadId ||
          scope.width !== editor.snapshot.width || scope.height !== editor.snapshot.height ||
          scope.sourceRevision !== editor.snapshot.sourceRevision || scope.annotationRevision !== editor.annotation.annotationRevision ||
          scope.regionId !== editor.selectedRegionId || !sameRegion(scope.region, editor.region)) return null
        return scope
      } catch { if (owns(editor, generation)) update({ error: fail('imageRegionUnavailable') }); return null }
    }
  }
}
