export type RuntimeDiagnosticsStatusFilter =
  | 'all'
  | 'running'
  | 'stale'
  | 'lease_expired'
  | 'orphaned'
  | 'recovering'
  | 'recovered'
  | 'dead_lettered'
  | 'delivery_pending'
  | 'delivered'
  | 'skipped'
  | 'retrying'
  | 'dead_letter'
  | 'paused'

export type RuntimeDiagnosticsFocus = {
  jobId?: string
  deliveryId?: string
  childThreadId?: string
  childTurnId?: string
  parentThreadId?: string
  parentTurnId?: string
  sourceBlockId?: string
  status?: RuntimeDiagnosticsStatusFilter
}

export const RUNTIME_DIAGNOSTICS_FOCUS_EVENT = 'analytix:runtime-diagnostics:focus'

export function dispatchRuntimeDiagnosticsFocus(focus: RuntimeDiagnosticsFocus): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<RuntimeDiagnosticsFocus>(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, {
    detail: normalizeRuntimeDiagnosticsFocus(focus)
  }))
}

export function listenRuntimeDiagnosticsFocus(
  handler: (focus: RuntimeDiagnosticsFocus) => void
): () => void {
  if (typeof window === 'undefined') return () => {}
  const listener = (event: Event): void => {
    const detail = event instanceof CustomEvent ? event.detail : null
    if (!detail || typeof detail !== 'object') return
    handler(normalizeRuntimeDiagnosticsFocus(detail as RuntimeDiagnosticsFocus))
  }
  window.addEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, listener)
  return () => window.removeEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, listener)
}

function normalizeRuntimeDiagnosticsFocus(focus: RuntimeDiagnosticsFocus): RuntimeDiagnosticsFocus {
  return {
    jobId: clean(focus.jobId),
    deliveryId: clean(focus.deliveryId),
    childThreadId: clean(focus.childThreadId),
    childTurnId: clean(focus.childTurnId),
    parentThreadId: clean(focus.parentThreadId),
    parentTurnId: clean(focus.parentTurnId),
    sourceBlockId: clean(focus.sourceBlockId),
    status: focus.status
  }
}

function clean(value: string | undefined): string | undefined {
  const trimmed = value?.trim()
  return trimmed || undefined
}
