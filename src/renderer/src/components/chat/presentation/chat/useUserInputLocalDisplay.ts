import { userInputLocalEffectResultSchemaV1, type UserInputLocalEffectV1 } from '@shared/analytix-api'
import { useCallback, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import {
  userInputLocalSelectorSchemaV1, userInputLocalViewResultSchemaV1,
  type UserInputLocalSelectorV1, type UserInputLocalDisplayV1, type UserInputLocalDisplayApiV1
} from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import {
  sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange
} from '../../../../data-analysis/services/analysis/stats-case-overview-resource'

type Phase = 'disabled' | 'loading' | 'ready' | 'unavailable'
type State = { key: string; phase: Phase; display: UserInputLocalDisplayV1 | null }

// Leave one of the unchanged 32 native seats for a transient association read.
// Orphaned seats retain only lease IDs until confirmed close or Main revocation.
const rawViewSeats = new Set<string>()
const capacityEvent = 'analytix-user-input-association-capacity'
const explicitReopenEvent = 'analytix-user-input-explicit-reopen'
// This metadata-only notice comes from the normal current-owner Respond button.
// It requests another native/Host-authorized read; it grants no answer authority.
export function requestUserInputLocalReopen(selector: UserInputLocalSelectorV1): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  const parsed = userInputLocalSelectorSchemaV1.safeParse(selector)
  if (parsed.success && !document.hidden && !sharedActiveCaseContextTransitionPending()) {
    window.dispatchEvent(new CustomEvent(explicitReopenEvent, { detail: parsed.data }))
  }
}
function matchesExplicitReopen(event: Event, key: string): boolean {
  const parsed = userInputLocalSelectorSchemaV1.safeParse((event as CustomEvent<unknown>).detail)
  return Boolean(key && parsed.success && JSON.stringify(parsed.data) === key && !document.hidden && !sharedActiveCaseContextTransitionPending())
}

// Public notices only invalidate a local snapshot. Host readback remains the
// authority for answer delivery, and idle wake may promote into a later turn.
function continuedDisplayNotice(event: Record<string, unknown>, threadId: string): boolean {
  if (event.threadId !== threadId) return false
  if (['turn_steered', 'turn_completed', 'turn_failed', 'turn_aborted', 'general_terminal_batch', 'accepted_final_batch'].includes(String(event.kind))) return true
  if (!['item_created', 'item_updated', 'item_completed'].includes(String(event.kind)) || !event.item || typeof event.item !== 'object') return false
  const item = event.item as Record<string, unknown>
  return item.kind === 'user_message' && item.role === 'user' && item.status === 'completed'
}

let transportEpoch = 0
let releaseActiveAssociation: (() => void) | null = null
let transportInvalidationOff: (() => void) | undefined
function observeTransportInvalidation(api: Partial<UserInputLocalDisplayApiV1>): boolean {
  if (transportInvalidationOff) return true
  if (!api.onUserInputLocalDisplayInvalidated) return false
  try { transportInvalidationOff = api.onUserInputLocalDisplayInvalidated(invalidateTransportSeats); return true } catch { return false }
}
function releaseTransportObserverWhenEmpty(): void {
  if (!rawViewSeats.size && !associationReaders) { transportInvalidationOff?.(); transportInvalidationOff = undefined }
}
function invalidateTransportSeats(id: string | null): void {
  if (id !== null) {
    if (rawViewSeats.delete(id)) window.dispatchEvent(new Event(capacityEvent))
    releaseTransportObserverWhenEmpty(); return
  }
  rawViewSeats.clear()
  transportEpoch++
  releaseActiveAssociation?.()
  releaseActiveAssociation = null
  window.dispatchEvent(new Event(capacityEvent))
  releaseTransportObserverWhenEmpty()
}
function reserveRawView(id: string): boolean {
  if (rawViewSeats.size >= 31) return false
  rawViewSeats.add(id)
  return true
}
// A view lease and raw snapshot belong only to this mounted question leaf.
export function useUserInputLocalDisplay(selector: UserInputLocalSelectorV1 | null | undefined, status: string, presented = true, onAuthorityRetired?: () => void) {
  const authorityRetired = useRef(onAuthorityRetired); authorityRetired.current = onAuthorityRetired
  const browserEnvironment = typeof window !== 'undefined' && typeof document !== 'undefined'
  const api = (browserEnvironment ? window.analytix?.runtime : undefined) as Partial<typeof window.analytix.runtime> | undefined
  const parsed = userInputLocalSelectorSchemaV1.safeParse(selector)
  const selectorKey = parsed.success ? JSON.stringify(parsed.data) : ''
  const key = selectorKey + ':' + status
  const currentKey = useRef(key); currentKey.current = key
  const selectorOwner = useRef(selectorKey), wasPresented = useRef(presented), revoked = useRef(false)
  const backgroundRetired = useRef(false)
  const retirementReason = useRef<'background' | 'authority'>('authority')
  const generation = useRef(0), viewId = useRef<string | null>(null), awaitingCapacity = useRef(false)
  const [state, setState] = useState<State | null>(null)
  const [revision, setRevision] = useState(0)
  const [privacyRevision, setPrivacyRevision] = useState(0)
  const [reload, setReload] = useState(0)
  const close = useCallback(async (id: string): Promise<boolean> => {
    try {
      if (!api?.closeUserInputLocalDisplay) return false
      await api.closeUserInputLocalDisplay(id)
      if (rawViewSeats.delete(id)) window.dispatchEvent(new Event(capacityEvent))
      releaseTransportObserverWhenEmpty()
      return true
    } catch { return false }
  }, [api])
  const retire = useCallback(() => {
    generation.current++
    const id = viewId.current; viewId.current = null; awaitingCapacity.current = false
    setState(null); setRevision(generation.current)
    if (id) void close(id)
  }, [close])
  const refresh = useCallback((expectedRevision?: number) => {
    if (expectedRevision !== undefined && expectedRevision !== generation.current) return
    // Only an explicit reread may reauthorize a privacy-revoked leaf.
    revoked.current = false
    retire(); setReload(value => value + 1)
  }, [retire])

  useLayoutEffect(() => {
    if (selectorOwner.current !== selectorKey || presented && !wasPresented.current) revoked.current = false
    selectorOwner.current = selectorKey; wasPresented.current = presented
    retire()
    if (!browserEnvironment) {
      setState({ key, phase: selector === undefined || !presented ? 'disabled' : 'unavailable', display: null })
      return
    }
    const unavailable = () => setState({ key, phase: 'unavailable', display: null })
    const clear = (background = false) => {
      retirementReason.current = background ? 'background' : 'authority'
      if (background) backgroundRetired.current = !revoked.current
      else {
        revoked.current = true; backgroundRetired.current = false
        if (currentKey.current === key) authorityRetired.current?.()
      }
      retire(); setPrivacyRevision(value => value + 1); unavailable()
    }
    const off = api?.onUserInputLocalDisplayInvalidated?.((id, reason) => {
      if (id === null || id === viewId.current) clear(reason === 'background')
    })
    const sseOff = status === 'continued' ? api?.onSseEvent?.(payload => {
      if (!revoked.current && viewId.current && currentKey.current === key && payload.events.some(event => parsed.success && continuedDisplayNotice(event, parsed.data.threadId))) {
        // An already queued refresh must still honor a later privacy revoke.
        retire(); setReload(value => value + 1)
      }
    }) : undefined
    const runtimeOff = api?.onRuntimeStatus?.(runtime => { if (runtime.state !== 'running') clear() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(() => clear())
    const hidden = () => {
      if (document.hidden) clear(true)
      else if (backgroundRetired.current && !revoked.current && presented && currentKey.current === key && !sharedActiveCaseContextTransitionPending()) {
        backgroundRetired.current = false
        retire(); setReload(value => value + 1)
      }
    }
    document.addEventListener('visibilitychange', hidden)
    const capacity = () => {
      if (awaitingCapacity.current && !revoked.current && currentKey.current === key) { awaitingCapacity.current = false; setReload(value => value + 1) }
    }
    window.addEventListener(capacityEvent, capacity)
    const reopen = (event: Event) => { if (matchesExplicitReopen(event, selectorKey) && currentKey.current === key) refresh() }
    window.addEventListener(explicitReopenEvent, reopen)
    if (selector === undefined || !presented) setState({ key, phase: 'disabled', display: null })
    else if (revoked.current || !parsed.success || !api?.openUserInputLocalDisplay || !api.closeUserInputLocalDisplay ||
      !api.onUserInputLocalDisplayInvalidated || document.hidden || sharedActiveCaseContextTransitionPending()) unavailable()
    else {
      const id = crypto.randomUUID()
      if (!observeTransportInvalidation(api) || !reserveRawView(id)) { awaitingCapacity.current = true; unavailable() }
      else {
        const epoch = generation.current
        viewId.current = id; setState({ key, phase: 'loading', display: null })
        const current = () => epoch === generation.current && currentKey.current === key && viewId.current === id && !revoked.current &&
          !document.hidden && !sharedActiveCaseContextTransitionPending()
        void (async () => {
          try {
            const result = userInputLocalViewResultSchemaV1.safeParse(await api.openUserInputLocalDisplay!({ ...parsed.data, viewId: id }))
            if (!current()) { void close(id); return }
            if (result.success && result.data.ok) {
              const display = result.data.display
              const bound = display.threadId === parsed.data.threadId && display.turnId === parsed.data.turnId &&
                display.itemId === parsed.data.itemId && display.requestId === parsed.data.requestId
              if (bound && (status === 'pending' || display.status === status)) { setState({ key, phase: 'ready', display }); return }
            }
            retire(); unavailable()
          } catch {
            if (!current()) { void close(id); return }
            retire(); unavailable()
          }
        })()
      }
    }
    return () => { retire(); off?.(); sseOff?.(); runtimeOff?.(); caseOff(); window.removeEventListener(explicitReopenEvent, reopen); window.removeEventListener(capacityEvent, capacity); document.removeEventListener('visibilitychange', hidden) }
    // Complete validated selector identity owns the leaf; revocation does not reopen it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [browserEnvironment, api, key, reload, retire, close, selector === undefined, presented, refresh])

  const matches = state?.key === key
  const current = browserEnvironment && matches && !revoked.current && !document.hidden && !sharedActiveCaseContextTransitionPending()
  return {
    display: presented && current ? state.display : null,
    phase: !browserEnvironment ? selector === undefined || !presented ? 'disabled' : 'unavailable'
      : matches ? state.phase : selector === undefined || !presented ? 'disabled' : parsed.success ? 'loading' : 'unavailable',
    revision, privacyRevision, retirementReason: retirementReason.current, refresh,
    isAuthorityRetired: () => revoked.current,
    canTransfer: () => Boolean(browserEnvironment && viewId.current && !revoked.current && !document.hidden && !sharedActiveCaseContextTransitionPending()),
    effect: async (effect: UserInputLocalEffectV1): Promise<boolean> => {
      const id = viewId.current, epoch = generation.current
      if (!parsed.success || !id || !current || !presented || !api?.performUserInputLocalEffect) return false
      try {
        const result = userInputLocalEffectResultSchemaV1.safeParse(await api.performUserInputLocalEffect({ selector: parsed.data, viewId: id, effect }))
        return Boolean(result.success && result.data.ok && epoch === generation.current && viewId.current === id && !revoked.current && !document.hidden && !sharedActiveCaseContextTransitionPending())
      } catch { return false }
    }
  }
}

export function useUserInputProtectedTransfer(rootRef: RefObject<HTMLElement | null>, active: boolean, isCurrent?: () => boolean): void {
  useLayoutEffect(() => {
    if (!active || !rootRef.current) return
    const preventTransfer = (event: Event) => {
      // Normal authorized local selection is supported. Only stale/retired
      // private DOM is fenced during the synchronous authority transition.
      if (isCurrent?.()) return
      const body = rootRef.current
      if (!body) return
      const target = event.target instanceof Element ? event.target : null
      for (const element of event.type === 'dragstart' ? [target] : [target, document.activeElement]) {
        if (element instanceof Element && element.closest('input,textarea,[contenteditable]:not([contenteditable="false"]),[role="textbox"]') &&
          !body.contains(element)) return
      }
      const selection = document.getSelection()
      let intersects = false
      for (let index = 0; selection && !selection.isCollapsed && index < selection.rangeCount; index++) {
        try { if (selection.getRangeAt(index).intersectsNode(body)) intersects = true } catch { /* Retired range. */ }
      }
      if (intersects || target && body.contains(target)) event.preventDefault()
    }
    for (const type of ['copy', 'cut', 'dragstart']) document.addEventListener(type, preventTransfer, true)
    return () => { for (const type of ['copy', 'cut', 'dragstart']) document.removeEventListener(type, preventTransfer, true) }
  }, [active, rootRef, isCurrent])
}

// Pure transport serialization: one active read and at most 32 admitted readers.
// Capacity retries remain in their mounted leaf; no payload or verdict is shared.
let associationTail: Promise<void> = Promise.resolve()
let associationReaders = 0
function serialAssociationRead(read: () => Promise<boolean>): boolean {
  if (associationReaders >= 32) return false
  associationReaders++
  const previous = associationTail, epoch = transportEpoch
  let release!: () => void
  associationTail = new Promise<void>(resolve => { release = resolve })
  void (async () => {
    await previous
    let closed = false
    let finished = false
    const finish = () => {
      if (finished) return
      finished = true; associationReaders--; release()
      if (releaseActiveAssociation === finish) releaseActiveAssociation = null
      window.dispatchEvent(new Event(capacityEvent))
      releaseTransportObserverWhenEmpty()
    }
    releaseActiveAssociation = finish
    try { closed = await read() }
    finally {
      if (closed || epoch !== transportEpoch) finish()
      // An uncertain close retains the active seat until trusted Main revocation.
    }
  })()
  return true
}
export type UserInputAssociationTarget = { kind: 'tool'; id: string; turnId: string; callId: string }
  | { kind: 'reply'; id: string; turnId: string; clientUserMessageId: string }
function privateAssociationIds(value: unknown, selector: UserInputLocalSelectorV1, status: string, targets: UserInputAssociationTarget[]): string[] | null {
  const result = userInputLocalViewResultSchemaV1.safeParse(value)
  if (!result.success || !result.data.ok) return null
  const display = result.data.display
  if (display.threadId !== selector.threadId || display.turnId !== selector.turnId || display.itemId !== selector.itemId || display.requestId !== selector.requestId || status !== 'pending' && display.status !== status) return null
  return targets.filter(target => target.kind === 'tool'
    ? display.callId === target.callId && display.turnId === target.turnId
    : Boolean(display.callId && display.submission && (display.historyReply
      ? display.historyReply.itemId === target.id && display.historyReply.clientUserMessageId === target.clientUserMessageId && display.historyReply.turnId === target.turnId
      : display.lateReply?.state === 'accepted' && display.lateReply.clientUserMessageId === target.clientUserMessageId && display.lateReply.turnId === target.turnId))).map(target => target.id)
}
// Raw parsing stays in this private leaf's async stack. Only validated existing
// machine IDs survive confirmed close; no snapshot enters React or shared state.
export function useUserInputLocalAssociation(selector: UserInputLocalSelectorV1 | null, status: string, targets: UserInputAssociationTarget[]): string[] {
  const browserEnvironment = typeof window !== 'undefined' && typeof document !== 'undefined'
  const api = (browserEnvironment ? window.analytix?.runtime : undefined) as Partial<typeof window.analytix.runtime> | undefined
  const parsed = userInputLocalSelectorSchemaV1.safeParse(selector)
  const selectorKey = parsed.success ? JSON.stringify(parsed.data) : ''
  const targetKey = JSON.stringify(targets), key = selectorKey + ':' + status + ':' + targetKey
  const currentKey = useRef(key); currentKey.current = key
  const selectorOwner = useRef(selectorKey), revoked = useRef(false), awaitingCapacity = useRef(false), backgroundRetired = useRef(false)
  const generation = useRef(0), viewId = useRef<string | null>(null)
  const [state, setState] = useState<{ key: string; ids: string[] } | null>(null)
  const [reload, setReload] = useState(0)
  useLayoutEffect(() => {
    if (selectorOwner.current !== selectorKey) revoked.current = false
    selectorOwner.current = selectorKey
    const epoch = ++generation.current
    setState(null)
    if (!browserEnvironment) return
    const close = async (id: string): Promise<boolean> => {
      try { if (!api?.closeUserInputLocalDisplay) return false; await api.closeUserInputLocalDisplay(id); return true } catch { return false }
    }
    // The serialized reader owns close confirmation even when the leaf retires.
    const retire = () => { generation.current++; viewId.current = null; awaitingCapacity.current = false; setState(null) }
    const revoke = (background = false) => {
      if (background) backgroundRetired.current = !revoked.current
      else { revoked.current = true; backgroundRetired.current = false }
      retire()
    }
    const off = api?.onUserInputLocalDisplayInvalidated?.((id, reason) => {
      if (id === null || id === viewId.current) revoke(reason === 'background')
    })
    const runtimeOff = api?.onRuntimeStatus?.(runtime => { if (runtime.state !== 'running') revoke() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(() => revoke())
    const hidden = () => {
      if (document.hidden) revoke(true)
      else if (backgroundRetired.current && !revoked.current && currentKey.current === key && !sharedActiveCaseContextTransitionPending()) {
        backgroundRetired.current = false
        retire(); setReload(value => value + 1)
      }
    }
    document.addEventListener('visibilitychange', hidden)
    const capacity = () => {
      if (awaitingCapacity.current && !revoked.current && currentKey.current === key) { awaitingCapacity.current = false; setReload(value => value + 1) }
    }
    window.addEventListener(capacityEvent, capacity)
    const reopen = (event: Event) => {
      if (matchesExplicitReopen(event, selectorKey) && currentKey.current === key) { revoked.current = false; retire(); setReload(value => value + 1) }
    }
    window.addEventListener(explicitReopenEvent, reopen)
    const sseOff = status === 'continued' ? api?.onSseEvent?.(payload => {
      if (!revoked.current && currentKey.current === key && parsed.success && payload.events.some(event => continuedDisplayNotice(event, parsed.data.threadId))) { retire(); setReload(value => value + 1) }
    }) : undefined
    const current = () => epoch === generation.current && currentKey.current === key && !revoked.current && !document.hidden && !sharedActiveCaseContextTransitionPending()
    if (parsed.success && api?.openUserInputLocalDisplay && api.closeUserInputLocalDisplay && api.onUserInputLocalDisplayInvalidated && targets.length > 0 && current() && observeTransportInvalidation(api)) {
      awaitingCapacity.current = !serialAssociationRead(async () => {
        if (!current()) return true
        const id = crypto.randomUUID(); viewId.current = id
        let closed = false, ids: string[] | null = null
        try {
          // Derive IDs synchronously so a slow/failed close retains no raw value.
          const derivedIds = privateAssociationIds(await api.openUserInputLocalDisplay!({ ...parsed.data, viewId: id }), parsed.data, status, targets)
          if (viewId.current === id) viewId.current = null
          closed = await close(id)
          if (closed && current()) ids = derivedIds
        } catch { /* Do not publish an unverified association. */ }
        finally { if (viewId.current === id) viewId.current = null; if (!closed) closed = await close(id) }
        if (closed && current() && ids) setState({ key, ids })
        return closed
      })
    }
    return () => { retire(); off?.(); runtimeOff?.(); caseOff(); sseOff?.(); window.removeEventListener(explicitReopenEvent, reopen); window.removeEventListener(capacityEvent, capacity); document.removeEventListener('visibilitychange', hidden) }
    // Complete validated identities represent the selector and targets.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [browserEnvironment, api, key, reload])
  return browserEnvironment && state?.key === key && !revoked.current && !document.hidden && !sharedActiveCaseContextTransitionPending() ? state.ids : []
}
