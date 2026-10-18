import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  userInputLocalDraftSnapshotSchemaV1, userInputLocalSaveDraftRequestSchemaV1, userInputLocalSelectorSchemaV1,
  type UserInputLocalDraftSnapshotV1, type UserInputLocalDisplayV1, type UserInputLocalSelectorV1
} from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { userInputDraftSaveResultSchemaV1, type UserInputDraftSaveResultV1 } from '../../../../../../shared/analytix-api'
import type { UserInputDraftInterruption } from '../../../../store/chat-store-types'

type FailureCode = Extract<UserInputDraftSaveResultV1, { ok: false }>['code']
export type UserInputPrivateDraftState = {
  phase: 'idle' | 'pending' | 'saving' | 'saved' | 'error'
  code: FailureCode | null
  canRetry: boolean
}
export type UserInputPrivateDraftProps = {
  selector: UserInputLocalSelectorV1 | null | undefined
  display: UserInputLocalDisplayV1 | null
  editable: boolean
  presented: boolean
  privacyRevision: number
  retirementReason?: 'background' | 'authority'
  isAuthorityRetired?: () => boolean
  readInterruption?: (key: string) => UserInputDraftInterruption | null
  recordInterruption?: (key: string, value: UserInputDraftInterruption | null) => void
  refresh: () => void
  onRestore?: (snapshot: UserInputLocalDraftSnapshotV1) => void
}
type Attempt = { body: string; revision: string; parent: string | null; readbacks: number; generation: number }
type Owner = {
  lastAdmission: number | null
  key: string; privacy: number; active: boolean; generation: number; cursor: string | null; readbacks: number
  protocol: boolean; hostEditable: boolean; supersededCursors: Set<string | null>; staleReadRequested: boolean; dirty: boolean; invalidChange: boolean
  attempt: Attempt | null; sending: boolean; pending: UserInputLocalDraftSnapshotV1 | null; ready: boolean
  phase: UserInputPrivateDraftState['phase']; code: FailureCode | null; reconcileAfter: number | null
  debounce?: ReturnType<typeof setTimeout>; maxWait?: ReturnType<typeof setTimeout>
}
const idle = (): UserInputPrivateDraftState => ({ phase: 'idle', code: null, canRetry: false })
const freshOwner = (key: string, privacy: number): Owner => ({ key, privacy, active: true, generation: 0,
  lastAdmission: null, cursor: null, readbacks: 0, protocol: false, hostEditable: false, supersededCursors: new Set(), staleReadRequested: false, dirty: false, invalidChange: false,
  attempt: null, sending: false, pending: null, ready: false, phase: 'idle', code: null, reconcileAfter: null })

// Only this mounted private leaf owns the unsent bytes. The Host owns the
// signed chain; a save result never resolves a Question or admits a late reply.
export function useUserInputPrivateDraft(props: UserInputPrivateDraftProps) {
  const parsed = userInputLocalSelectorSchemaV1.safeParse(props.selector)
  const key = parsed.success ? JSON.stringify(parsed.data) : ''
  const latest = useRef({ ...props, key, selector: parsed.success ? parsed.data : null })
  latest.current = { ...props, key, selector: parsed.success ? parsed.data : null }
  const owner = useRef<Owner | null>(null)
  // Retired content is erased; only bounded owner/revision metadata survives
  // an ordinary background to explain unadmitted edits on fresh restoration.
  const interrupted = useRef<{ key: string; revision: string | null; unsent: boolean } | null>(null)
  const [interruption, setInterruption] = useState<string | null>(null)
  const [view, setView] = useState({ key, privacy: props.privacyRevision, state: idle() })
  const controls = useMemo(() => {
    const current = (cell: Owner) => owner.current === cell && cell.active && cell.key !== '' &&
      latest.current.key === cell.key && latest.current.privacyRevision === cell.privacy
    const writable = (cell: Owner) => current(cell) && latest.current.editable && !latest.current.display?.readOnly && cell.protocol && cell.hostEditable &&
      (latest.current.display !== null || !latest.current.presented)
    const recordInterruption = (key: string, value: UserInputDraftInterruption | null) => {
      interrupted.current = value === null ? null : { key, ...value }
      setInterruption(value === null ? null : key)
      latest.current.recordInterruption?.(key, value)
    }
    const clearTimers = (cell: Owner) => {
      clearTimeout(cell.debounce); clearTimeout(cell.maxWait)
      cell.debounce = undefined; cell.maxWait = undefined
    }
    const canRetry = (cell: Owner) => writable(cell) && latest.current.presented && !cell.sending &&
      (cell.code === 'unavailable' ? cell.attempt !== null :
        cell.code !== null && cell.code !== 'retired' && cell.pending !== null && cell.reconcileAfter !== null && cell.readbacks > cell.reconcileAfter)
    const publish = (cell: Owner) => {
      if (!current(cell)) return
      setView({ key: cell.key, privacy: cell.privacy, state: { phase: cell.phase, code: cell.code, canRetry: canRetry(cell) } })
    }
    const clearWork = (cell: Owner) => {
      cell.generation++; clearTimers(cell)
      if (cell.attempt) cell.attempt.body = ''
      cell.attempt = null; cell.sending = false; cell.pending = null; cell.ready = false
      cell.dirty = false; cell.invalidChange = false; cell.phase = 'idle'; cell.code = null; cell.reconcileAfter = null
    }
    const reread = () => { try { latest.current.refresh() } catch { /* Private readback remains unavailable. */ } }
    const restore = (cell: Owner, display: UserInputLocalDisplayV1 | null) => {
      if (!writable(cell) || !latest.current.presented || cell.dirty || !display?.draft) return
      const snapshot = userInputLocalDraftSnapshotSchemaV1.safeParse(display.draft)
      if (snapshot.success) latest.current.onRestore?.(snapshot.data)
    }
    const releaseAttempt = (cell: Owner) => {
      if (cell.attempt) cell.attempt.body = ''
      cell.attempt = null
    }
    const send = (cell: Owner, attempt: Attempt): void => {
      if (!writable(cell) || cell.sending || cell.attempt !== attempt) return
      cell.sending = true; cell.phase = 'saving'; cell.code = null; clearTimers(cell); publish(cell)
      void (async () => {
        let result: UserInputDraftSaveResultV1 = { ok: false, code: 'unavailable' }
        try {
          const api = window.analytix?.runtime.saveUserInputDraft
          if (!api) throw new Error('Unavailable')
          const received: unknown = await api(userInputLocalSaveDraftRequestSchemaV1.parse(JSON.parse(attempt.body)))
          if (!current(cell) || cell.generation !== attempt.generation || cell.attempt !== attempt) {
            // Main validates the receipt. A late save may prompt a fresh read
            // for the same visible owner, but never reapplies its old raw data.
            const receipt = userInputDraftSaveResultSchemaV1.safeParse(received)
            const retired = interrupted.current
            if (retired?.key === cell.key && !retired.unsent && receipt.success && receipt.data.ok &&
                receipt.data.receipt.draftRevision === retired.revision &&
                owner.current?.active && owner.current.key === cell.key && latest.current.key === cell.key && latest.current.presented && !document.hidden) reread()
            return
          }
          const checked = userInputDraftSaveResultSchemaV1.safeParse(received)
          if (checked.success) {
            result = checked.data
            if (result.ok) {
              const request = userInputLocalSaveDraftRequestSchemaV1.parse(JSON.parse(attempt.body))
              const receipt = result.receipt
              if (['threadId', 'turnId', 'itemId', 'requestId'].some(field =>
                receipt[field as keyof UserInputLocalSelectorV1] !== request[field as keyof UserInputLocalSelectorV1]) ||
                receipt.draftRevision !== attempt.revision || JSON.stringify(receipt.draft) !== JSON.stringify(request.draft)) {
                result = { ok: false, code: 'unavailable' }
              }
            }
          }
        } catch { result = { ok: false, code: 'unavailable' } }
        if (!current(cell) || !latest.current.editable || cell.generation !== attempt.generation || cell.attempt !== attempt) return
        cell.sending = false
        if (result.ok) {
          // A verified child ACK permanently supersedes its parent cursor. The
          // Host retains at most 4096 drafts; this leaf retains only their IDs.
          if (cell.supersededCursors.size < 4096) cell.supersededCursors.add(attempt.parent)
          // A same-set retry may ACK a superseded receipt. Only settle this
          // attempt; never move a newer observed Host cursor back to its UUID.
          const head = cell.readbacks <= attempt.readbacks || cell.cursor === attempt.parent || cell.cursor === attempt.revision
          if (head) cell.cursor = result.receipt.draftRevision
          releaseAttempt(cell)
          if (!head && cell.pending) {
            cell.code = 'conflict'; cell.phase = 'error'; cell.reconcileAfter = cell.readbacks - 1
          } else if (cell.invalidChange) {
            cell.code = 'invalid'; cell.phase = 'error'; cell.reconcileAfter = null
          } else if (cell.pending) {
            cell.phase = 'pending'; cell.code = null
          } else {
            cell.dirty = false; cell.phase = head ? 'saved' : 'idle'; cell.code = null
            if (head && interrupted.current?.key === cell.key) recordInterruption(cell.key, null)
            if (!head) restore(cell, latest.current.display)
          }
          publish(cell)
          if (cell.pending && cell.code === null) start(cell)
          return
        }
        cell.phase = 'error'; cell.code = result.code; cell.reconcileAfter = cell.readbacks
        if (result.code !== 'unavailable') {
          const failed = userInputLocalSaveDraftRequestSchemaV1.parse(JSON.parse(attempt.body)).draft
          releaseAttempt(cell)
          if (result.code === 'retired') {
            cell.pending = null; cell.ready = false; cell.dirty = false; cell.invalidChange = false
          } else if (!cell.pending && !cell.invalidChange) cell.pending = failed
        }
        cell.ready = cell.pending !== null
        clearTimers(cell); publish(cell); reread()
      })()
    }
    const start = (cell: Owner): void => {
      if (!writable(cell) || cell.sending || cell.attempt || cell.code !== null || !cell.pending || !cell.ready) return
      const request = userInputLocalSaveDraftRequestSchemaV1.safeParse({ ...latest.current.selector, action: 'saveDraft',
        revision: crypto.randomUUID(), expectedRevision: cell.cursor, draft: cell.pending })
      if (!request.success) {
        cell.pending = null; cell.ready = false; cell.invalidChange = true; cell.phase = 'error'; cell.code = 'invalid'; publish(cell); return
      }
      cell.attempt = { body: JSON.stringify(request.data), revision: request.data.revision,
        parent: request.data.expectedRevision, readbacks: cell.readbacks, generation: cell.generation }
      cell.pending = null; cell.ready = false; cell.lastAdmission = Date.now(); send(cell, cell.attempt)
    }
    const flush = () => {
      const cell = owner.current
      if (!cell || !writable(cell)) return
      clearTimers(cell)
      if (cell.pending) { cell.ready = true; start(cell) }
    }
    const schedule = (cell: Owner) => {
      if (cell.code !== null || cell.ready) return
      clearTimeout(cell.debounce)
      const due = () => { if (!current(cell)) return; clearTimers(cell); cell.ready = true; start(cell) }
      cell.debounce = setTimeout(due, 750)
      cell.maxWait ??= setTimeout(due, 3000)
    }
    return {
      attach: (nextKey: string, privacy: number) => {
        const cell = freshOwner(nextKey, privacy); owner.current = cell
        const stored = latest.current.readInterruption?.(nextKey)
        if (stored) { interrupted.current = { key: nextKey, ...stored }; setInterruption(nextKey) }
        else if (interrupted.current?.key !== nextKey) { interrupted.current = null; setInterruption(null) }
        publish(cell)
        return () => {
          // A changed native privacy epoch proves authority retirement. The
          // default disposition alone cannot turn a normal unmount into one.
          const authorityRetired = latest.current.isAuthorityRetired?.() === true ||
            latest.current.key === cell.key && latest.current.privacyRevision !== cell.privacy && latest.current.retirementReason !== 'background'
          if (authorityRetired) recordInterruption(cell.key, null)
          else if (cell.dirty || cell.code !== null) recordInterruption(cell.key, {
            revision: cell.attempt?.revision ?? null,
            unsent: cell.pending !== null || cell.invalidChange || cell.code !== null && cell.code !== 'unavailable'
          })
          clearWork(cell); cell.active = false; if (owner.current === cell) owner.current = null
        }
      },
      read: (display: UserInputLocalDisplayV1 | null) => {
        const cell = owner.current
        if (!cell || !current(cell) || !display) return
        if (['threadId', 'turnId', 'itemId', 'requestId'].some(field =>
          display[field as keyof UserInputLocalSelectorV1] !== latest.current.selector?.[field as keyof UserInputLocalSelectorV1])) return
        const supported = Object.prototype.hasOwnProperty.call(display, 'draftRevision') && Object.prototype.hasOwnProperty.call(display, 'draft')
        if (!supported) { clearWork(cell); cell.protocol = false; cell.hostEditable = false; publish(cell); return }
        const retired = interrupted.current
        if (retired?.key === cell.key && !retired.unsent && retired.revision !== null && display.draftRevision === retired.revision) {
          recordInterruption(cell.key, null)
        }
        cell.protocol = true
        const nextCursor = display.draftRevision ?? null
        if (nextCursor !== cell.cursor && cell.supersededCursors.has(nextCursor)) {
          // An older in-flight private read may arrive after a child save ACK.
          // Cleared content retains its cursor, so that parent is provably old.
          // Ask once for current authority; never restore its stale snapshot.
          if (!cell.staleReadRequested) { cell.staleReadRequested = true; reread() }
          return
        }
        cell.staleReadRequested = false; cell.readbacks++; cell.cursor = nextCursor
        cell.hostEditable = !display.readOnly && (display.status === 'pending' || display.status === 'continued') &&
          display.lateReply?.state !== 'queued' && display.lateReply?.state !== 'accepted'
        if (!cell.hostEditable) { clearWork(cell); recordInterruption(cell.key, null); publish(cell); return }
        if (cell.code === 'retired' && cell.pending) {
          cell.code = null; cell.reconcileAfter = null; cell.phase = 'pending'
          if (cell.ready) start(cell)
          else schedule(cell)
        }
        restore(cell, display); publish(cell)
        // A visible loading-null read pauses dispatch but retains a due edit.
        // Fresh Host readback supplies its current cursor before it can start.
        if (cell.pending && cell.code === null && cell.ready) start(cell)
      },
      change: (snapshot: UserInputLocalDraftSnapshotV1) => {
        const cell = owner.current
        if (!cell || !writable(cell) || !latest.current.presented || !latest.current.display) return
        const checked = userInputLocalDraftSnapshotSchemaV1.safeParse(snapshot)
        if (!checked.success) {
          cell.pending = null; cell.ready = false; cell.invalidChange = true; cell.dirty = true; clearTimers(cell)
          cell.phase = 'error'; cell.code = 'invalid'; cell.reconcileAfter = null; publish(cell); return
        }
        const value = JSON.stringify(checked.data)
        if (cell.pending && JSON.stringify(cell.pending) === value || !cell.pending && cell.attempt &&
          JSON.stringify(userInputLocalSaveDraftRequestSchemaV1.parse(JSON.parse(cell.attempt.body)).draft) === value) return
        cell.pending = checked.data; cell.dirty = true; cell.invalidChange = false
        if (cell.code === 'invalid' && cell.reconcileAfter === null) cell.code = null
        if (cell.code !== null) cell.ready = true
        else {
          if (!cell.sending) cell.phase = 'pending'
          // The first edit is admitted promptly. Subsequent edits reuse the
          // existing finite debounce/max-wait, avoiding a receipt per key.
          // A named flush marks the whole latest single-writer chain due.
          if (cell.ready || cell.lastAdmission === null || Date.now() - cell.lastAdmission >= 750) {
            clearTimers(cell); cell.ready = true; start(cell)
          } else schedule(cell)
        }
        publish(cell)
      },
      flush,
      retire: () => { const cell = owner.current; if (cell) { clearWork(cell); publish(cell) } },
      retry: () => {
        const cell = owner.current
        if (!cell || !canRetry(cell)) return
        if (cell.code === 'unavailable' && cell.attempt) { send(cell, cell.attempt); return }
        cell.code = null; cell.reconcileAfter = null; cell.phase = 'pending'; cell.ready = true; start(cell)
      }
    }
  }, [])
  useLayoutEffect(() => controls.attach(key, props.privacyRevision), [controls, key, props.privacyRevision])
  useLayoutEffect(() => controls.read(props.display), [controls, props.display, key, props.privacyRevision])
  useLayoutEffect(() => {
    if (!props.editable) controls.retire()
    else if (!props.presented) controls.flush()
  }, [controls, props.editable, props.presented, key, props.privacyRevision])
  return { change: controls.change, flush: controls.flush, interrupted: interruption === key, retire: controls.retire, retry: controls.retry,
    state: view.key === key && view.privacy === props.privacyRevision && props.editable
      ? { ...view.state, canRetry: props.presented && props.display !== null && view.state.canRetry } : idle() }
}
