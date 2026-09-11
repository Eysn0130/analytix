import type { ReactElement } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, Eye, EyeOff, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AcceptedFinalProjectionReceiptV1 } from '../../agent/types'
import type {
  AcceptedSlotDisplayRequest,
  CleaningDiffPreviewRequest,
  DirectSourcePreviewRequest,
  ImportMappingPreviewRequest,
  LocalDisplayMode,
  LocalDisplayResponse,
  LocalDisplayResult
} from '@shared/analytix-api'
import {
  CLEANING_DIFF_PREVIEW_FIELDS,
  DIRECT_SOURCE_PREVIEW_FIELDS,
  IMPORT_MAPPING_PREVIEW_FIELDS
} from '@shared/analytix-api'
import { subscribeStatsCacheInvalidation } from '../../data-analysis/services/analysis/stats-cache-events'
import {
  getSharedActiveCaseContext,
  subscribeSharedActiveCaseContext
} from '../../data-analysis/services/analysis/stats-case-overview-resource'
import { useChatStore } from '../../store/chat-store'

type LocalDisplayLoader = (mode: LocalDisplayMode) => Promise<LocalDisplayResult>
type LocalDisplayPagination = {
  hasPrevious: boolean
  hasNext: boolean
  onPrevious: () => void
  onNext: () => void
}

type EphemeralLocalDisplayResponse = {
  requestAuthority: string
  value: LocalDisplayResponse
}

type LocalDisplayRendererLeaseTicketV1 = {
  authority: string
  generation: number
}

type LocalDisplayRendererLeaseV1 = {
  bind: (authority: string) => void
  issue: (authority: string) => LocalDisplayRendererLeaseTicketV1
  accept: (ticket: LocalDisplayRendererLeaseTicketV1) => boolean
  isCurrent: (ticket: LocalDisplayRendererLeaseTicketV1) => boolean
  revoke: () => void
}

export const LOCAL_DISPLAY_RENDERER_LEASE_MS = 15_000

export function createLocalDisplayRendererLeaseV1(
  onExpire: (authority: string) => void,
  ttlMs = LOCAL_DISPLAY_RENDERER_LEASE_MS
): LocalDisplayRendererLeaseV1 {
  let generation = 0
  let authority: string | null = null
  let expiryTimer: ReturnType<typeof setTimeout> | null = null
  const clearExpiry = (): void => {
    if (expiryTimer !== null) clearTimeout(expiryTimer)
    expiryTimer = null
  }
  const bind = (nextAuthority: string): void => {
    if (authority === nextAuthority) return
    generation += 1
    authority = nextAuthority
    clearExpiry()
  }
  const isCurrent = (ticket: LocalDisplayRendererLeaseTicketV1): boolean =>
    ticket.generation === generation && ticket.authority === authority

  return {
    bind,
    issue(nextAuthority) {
      bind(nextAuthority)
      generation += 1
      authority = nextAuthority
      clearExpiry()
      return { authority: nextAuthority, generation }
    },
    accept(ticket) {
      if (!isCurrent(ticket)) return false
      clearExpiry()
      expiryTimer = setTimeout(() => {
        if (!isCurrent(ticket)) return
        generation += 1
        expiryTimer = null
        onExpire(ticket.authority)
      }, Math.max(1, ttlMs))
      return true
    },
    isCurrent,
    revoke() {
      generation += 1
      authority = null
      clearExpiry()
    }
  }
}

export const DIRECT_SOURCE_PREVIEW_PAGE_SIZE = 25

export function directSourcePreviewPageOffset(
  currentOffset: number,
  direction: 'previous' | 'next',
  hasMore: boolean
): number {
  const offset = Number.isSafeInteger(currentOffset) && currentOffset > 0 ? currentOffset : 0
  if (direction === 'previous') return Math.max(0, offset - DIRECT_SOURCE_PREVIEW_PAGE_SIZE)
  if (!hasMore || offset > 100_000 - DIRECT_SOURCE_PREVIEW_PAGE_SIZE) return offset
  return offset + DIRECT_SOURCE_PREVIEW_PAGE_SIZE
}

export function localDisplayResponseForCurrentAuthority(
  response: LocalDisplayResponse | null,
  responseAuthority: string | undefined,
  currentAuthority: string
): LocalDisplayResponse | null {
  return response && responseAuthority === currentAuthority ? response : null
}

export function localDisplayAuthorityGenerationV1(input: {
  rendererSession: string
  caseBinding: string
  snapshotGeneration: number
}): string {
  return JSON.stringify([
    input.rendererSession,
    input.caseBinding,
    input.snapshotGeneration
  ])
}

function isLocalDisplayResponse(result: LocalDisplayResult): result is LocalDisplayResponse {
  return !('ok' in result)
}

export function acceptedSlotDisplayRequest(
  receipt: AcceptedFinalProjectionReceiptV1,
  mode: LocalDisplayMode
): AcceptedSlotDisplayRequest | null {
  if (
    receipt.schemaVersion !== 1 ||
    !receipt.threadId.trim() ||
    !receipt.turnId.trim() ||
    !/^[a-f0-9]{64}$/.test(receipt.publicationCommitId)
  ) {
    return null
  }
  return {
    kind: 'accepted_slot_display',
    threadId: receipt.threadId,
    turnId: receipt.turnId,
    acceptedFinalDigest: receipt.publicationCommitId,
    displayMode: mode
  }
}

export function acceptedSlotDisplayRequestGeneration(
  receipt: AcceptedFinalProjectionReceiptV1
): string {
  const request = acceptedSlotDisplayRequest(receipt, 'full')
  return request
    ? JSON.stringify([request.threadId, request.turnId, request.acceptedFinalDigest])
    : 'invalid-accepted-slot-display-request'
}

export function localDisplayRequestAuthorityV1(
  authorityGeneration: string,
  requestGeneration: string,
  mode: LocalDisplayMode
): string {
  return JSON.stringify([authorityGeneration, requestGeneration, mode])
}

export function acceptedFinalHasLocalDisplaySlots(
  view: { claimCount: number; receiptMetadata: { count: number } } | null | undefined
): boolean {
  return Boolean(view && view.claimCount > 0 && view.receiptMetadata.count > 0)
}

export function useLocalDisplayAuthorityGeneration(): string {
  const chatAuthority = useChatStore((state) =>
    `${state.activeThreadId ?? ''}:${state.workspaceRoot}:${state.runtimeConnection}:${state.lastSeq}`
  )
  const [caseAuthority, setCaseAuthority] = useState(() => {
    const current = getSharedActiveCaseContext()
    return `${current?.caseId ?? ''}:${current?.workspaceRoot ?? ''}:${current?.updatedAt ?? 0}`
  })
  const [snapshotGeneration, setSnapshotGeneration] = useState(0)

  useEffect(() => subscribeSharedActiveCaseContext((current) => {
    setCaseAuthority(`${current?.caseId ?? ''}:${current?.workspaceRoot ?? ''}:${current?.updatedAt ?? 0}`)
  }, { emitCurrent: true }), [])

  useEffect(() => subscribeStatsCacheInvalidation((event) => {
    if (event.caseId === getSharedActiveCaseContext()?.caseId) {
      setSnapshotGeneration((current) => current + 1)
    }
  }), [])

  return localDisplayAuthorityGenerationV1({
    rendererSession: chatAuthority,
    caseBinding: caseAuthority,
    snapshotGeneration
  })
}

function useEphemeralLocalDisplay(
  loader: LocalDisplayLoader,
  authorityGeneration: string,
  requestGeneration = ''
): {
  mode: LocalDisplayMode
  response: LocalDisplayResponse | null
  unavailable: boolean
  setMode: (mode: LocalDisplayMode) => void
} {
  const [mode, setMode] = useState<LocalDisplayMode>('full')
  const [response, setResponse] = useState<EphemeralLocalDisplayResponse | null>(null)
  const [unavailableAuthority, setUnavailableAuthority] = useState<string | null>(null)
  const requestAuthority = localDisplayRequestAuthorityV1(
    authorityGeneration,
    requestGeneration,
    mode
  )
  const lease = useRef<LocalDisplayRendererLeaseV1 | null>(null)
  if (lease.current === null) {
    lease.current = createLocalDisplayRendererLeaseV1((expiredAuthority) => {
      setResponse(null)
      setUnavailableAuthority(expiredAuthority)
    })
  }
  lease.current.bind(requestAuthority)

  const refresh = useCallback(() => {
    const currentLease = lease.current
    if (!currentLease) return
    const currentRequestAuthority = requestAuthority
    const ticket = currentLease.issue(currentRequestAuthority)
    setResponse(null)
    setUnavailableAuthority(null)
    void loader(mode)
      .then((result) => {
        if (!currentLease.isCurrent(ticket)) return
        if (isLocalDisplayResponse(result)) {
          if (!currentLease.accept(ticket)) return
          setResponse({ requestAuthority: currentRequestAuthority, value: result })
          return
        }
        setUnavailableAuthority(currentRequestAuthority)
      })
      .catch(() => {
        if (currentLease.isCurrent(ticket)) {
          setUnavailableAuthority(currentRequestAuthority)
        }
      })
  }, [loader, mode, requestAuthority])

  useEffect(() => {
    refresh()
    const revokeRendererLease = (): void => {
      lease.current?.revoke()
      setResponse(null)
      setUnavailableAuthority(null)
    }
    const refreshVisibleAuthority = (): void => {
      if (document.visibilityState === 'visible') {
        refresh()
      } else {
        revokeRendererLease()
      }
    }
    window.addEventListener('focus', refresh)
    window.addEventListener('pagehide', revokeRendererLease)
    document.addEventListener('visibilitychange', refreshVisibleAuthority)
    return () => {
      lease.current?.revoke()
      window.removeEventListener('focus', refresh)
      window.removeEventListener('pagehide', revokeRendererLease)
      document.removeEventListener('visibilitychange', refreshVisibleAuthority)
    }
  }, [refresh])

  return {
    mode,
    response: localDisplayResponseForCurrentAuthority(
      response?.value ?? null,
      response?.requestAuthority,
      requestAuthority
    ),
    unavailable: unavailableAuthority === requestAuthority,
    setMode
  }
}

function LocalDisplayPaginationControls({
  pagination
}: {
  pagination?: LocalDisplayPagination
}): ReactElement | null {
  const { t } = useTranslation('common')
  if (!pagination) return null
  return (
    <div className="mt-2 flex items-center justify-end gap-1.5">
      <button
        type="button"
        data-analytix-source-page="previous"
        onClick={pagination.onPrevious}
        disabled={!pagination.hasPrevious}
        className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11.5px] text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('directSourcePreviewPreviousPage')}
      >
        <ChevronLeft className="h-3.5 w-3.5" />
        {t('directSourcePreviewPreviousPage')}
      </button>
      <button
        type="button"
        data-analytix-source-page="next"
        onClick={pagination.onNext}
        disabled={!pagination.hasNext}
        className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11.5px] text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('directSourcePreviewNextPage')}
      >
        {t('directSourcePreviewNextPage')}
        <ChevronRight className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

export function LocalDisplayView({
  response,
  unavailable,
  onModeChange,
  pagination
}: {
  response: LocalDisplayResponse | null
  unavailable: boolean
  onModeChange: (mode: LocalDisplayMode) => void
  pagination?: LocalDisplayPagination
}): ReactElement | null {
  const { t } = useTranslation('common')
  if (!response && !unavailable) return null
  if (unavailable) {
    return (
      <div
        data-analytix-local-display="unavailable"
        className="mt-2 rounded-lg border border-ds-border-muted bg-ds-card/60 px-3 py-2 text-xs text-ds-faint"
      >
        {t('localDisplayUnavailable')}
      </div>
    )
  }
  if (!response) return null
  if (response.kind === 'accepted_slot_display' && response.slots.length === 0) return null

  const masked = response.displayMode === 'masked'
  const title = response.kind === 'direct_source_preview'
    ? t('directSourcePreviewTitle')
    : response.kind === 'accepted_slot_display'
      ? t('acceptedSlotDisplayTitle')
      : response.kind === 'import_mapping_preview'
        ? 'Import mapping preview'
        : 'Cleaning diff preview'
  return (
    <section
      data-analytix-local-display={response.kind}
      data-analytix-local-display-mode={response.displayMode}
      className="mt-2 rounded-xl border border-ds-border-muted bg-ds-card/70 px-3 py-2.5"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2 text-xs font-medium text-ds-muted">
          <ShieldCheck className="h-3.5 w-3.5 shrink-0" strokeWidth={1.8} />
          <span className="truncate">
            {title}
          </span>
        </div>
        <button
          type="button"
          onClick={() => onModeChange(masked ? 'full' : 'masked')}
          className="inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-1 text-[11.5px] text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
          aria-label={masked ? t('localDisplayShowFull') : t('localDisplayShowMasked')}
        >
          {masked ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
          {masked ? t('localDisplayFull') : t('localDisplayMasked')}
        </button>
      </div>
      {response.kind === 'direct_source_preview' ? (
        <div>
          <div className="mt-2 max-h-[52vh] overflow-auto rounded-lg border border-ds-border-muted">
            <table className="min-w-full border-collapse text-left text-xs">
              <thead className="sticky top-0 bg-ds-elevated text-ds-faint">
                <tr>
                  {response.fields.map((field) => (
                    <th key={field} scope="col" className="whitespace-nowrap border-b border-ds-border-muted px-2 py-1.5 font-medium">
                      {t(`directSourcePreviewField_${field}`)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {response.rows.map((row) => (
                  <tr key={row.rowIndex} data-analytix-source-row={row.rowIndex} className="border-b border-ds-border-muted last:border-b-0">
                    {row.cells.map((cell) => (
                      <td key={cell.field} className="max-w-[24rem] whitespace-pre-wrap break-words px-2 py-1.5 align-top font-mono text-ds-ink">
                        {cell.displayValue}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            {response.rows.length === 0 ? (
              <div className="px-3 py-4 text-center text-xs text-ds-faint">{t('directSourcePreviewEmpty')}</div>
            ) : null}
          </div>
          <LocalDisplayPaginationControls pagination={pagination} />
        </div>
      ) : response.kind === 'import_mapping_preview' ? (
        <div>
          <div className="mt-2 max-h-[52vh] overflow-auto rounded-lg border border-ds-border-muted">
            <table className="min-w-full border-collapse text-left text-xs">
            <thead className="sticky top-0 bg-ds-elevated text-ds-faint">
              <tr>
                {response.fields.map((field) => (
                  <th key={field} scope="col" className="whitespace-nowrap border-b border-ds-border-muted px-2 py-1.5 font-medium">
                    {field}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {response.rows.map((row) => (
                <tr key={row.rowIndex} className="border-b border-ds-border-muted last:border-b-0">
                  {row.cells.map((cell) => (
                    <td key={cell.field} className="max-w-[24rem] whitespace-pre-wrap break-words px-2 py-1.5 align-top font-mono text-ds-ink">
                      {cell.displayValue}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
            </table>
          </div>
          <LocalDisplayPaginationControls pagination={pagination} />
        </div>
      ) : response.kind === 'cleaning_diff_preview' ? (
        <div>
          <div className="mt-2 max-h-[52vh] overflow-auto rounded-lg border border-ds-border-muted">
            <table className="min-w-full border-collapse text-left text-xs">
            <thead className="sticky top-0 bg-ds-elevated text-ds-faint">
              <tr>
                {response.fields.map((field) => (
                  <th key={field} scope="col" className="whitespace-nowrap border-b border-ds-border-muted px-2 py-1.5 font-medium">
                    {field}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {response.rows.map((row) => (
                <tr key={row.rowIndex} className="border-b border-ds-border-muted last:border-b-0">
                  {row.cells.map((cell) => (
                    <td key={cell.field} className="max-w-[32rem] whitespace-pre-wrap break-words px-2 py-1.5 align-top font-mono text-ds-ink">
                      <span>{cell.beforeDisplayValue}</span>
                      <span aria-hidden="true"> → </span>
                      <span>{cell.afterDisplayValue}</span>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
            </table>
          </div>
          <LocalDisplayPaginationControls pagination={pagination} />
        </div>
      ) : (
        <dl className="mt-2 grid gap-1.5">
          {response.slots.map((slot) => (
            <div key={slot.slotId} className="flex min-w-0 items-baseline justify-between gap-3 text-sm">
              <dt className="shrink-0 text-xs text-ds-faint">
                {t(`directSourcePreviewField_${slot.field}`)}
              </dt>
              <dd className="min-w-0 break-all font-mono text-ds-ink">{slot.displayValue}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  )
}

export function AcceptedSlotDisplay({
  receipt
}: {
  receipt: AcceptedFinalProjectionReceiptV1
}): ReactElement | null {
  const authorityGeneration = useLocalDisplayAuthorityGeneration()
  const loader = useCallback<LocalDisplayLoader>(async (mode) => {
    const request = acceptedSlotDisplayRequest(receipt, mode)
    if (!request) {
      return { ok: false, status: 400, code: 'invalid_request', message: 'local display request is invalid' }
    }
    return window.analytix.runtime.acceptedSlotDisplay(request)
  }, [receipt])
  const requestGeneration = acceptedSlotDisplayRequestGeneration(receipt)
  const display = useEphemeralLocalDisplay(loader, authorityGeneration, requestGeneration)
  return (
    <LocalDisplayView
      response={display.response}
      unavailable={display.unavailable}
      onModeChange={display.setMode}
    />
  )
}

export function DirectSourcePreview(): ReactElement | null {
  const authorityGeneration = useLocalDisplayAuthorityGeneration()
  const [rowOffset, setRowOffset] = useState(0)
  const loader = useCallback<LocalDisplayLoader>(
    (mode) => window.analytix.runtime.directSourcePreview({
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: [...DIRECT_SOURCE_PREVIEW_FIELDS],
      rowOffset,
      rowLimit: DIRECT_SOURCE_PREVIEW_PAGE_SIZE,
      displayMode: mode
    } satisfies DirectSourcePreviewRequest),
    [rowOffset]
  )
  const display = useEphemeralLocalDisplay(loader, authorityGeneration, String(rowOffset))
  const response = display.response?.kind === 'direct_source_preview' &&
    display.response.rowOffset === rowOffset &&
    display.response.rowLimit === DIRECT_SOURCE_PREVIEW_PAGE_SIZE
    ? display.response
    : null

  useEffect(() => setRowOffset(0), [authorityGeneration])

  const setDisplayMode = display.setMode
  const changeMode = useCallback((mode: LocalDisplayMode) => {
    setRowOffset(0)
    setDisplayMode(mode)
  }, [setDisplayMode])

  return (
    <LocalDisplayView
      response={response}
      unavailable={display.unavailable}
      onModeChange={changeMode}
      pagination={response ? {
        hasPrevious: rowOffset > 0,
        hasNext: response.hasMore,
        onPrevious: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'previous', false)),
        onNext: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'next', response.hasMore))
      } : undefined}
    />
  )
}

function validTypedLocalSelectorV1(selector: string): boolean {
  return /^tlsel1_[a-f0-9]{64}$/.test(selector)
}

// Main-owned selectors keep exact local display data outside ordinary renderer state.
export function ImportMappingPreview({ selector }: { selector: string }): ReactElement | null {
  const authorityGeneration = useLocalDisplayAuthorityGeneration()
  const [rowOffset, setRowOffset] = useState(0)
  const loader = useCallback<LocalDisplayLoader>((mode) => {
    if (!validTypedLocalSelectorV1(selector)) {
      return Promise.resolve({ ok: false, status: 400, code: 'invalid_request', message: 'local display request is invalid' })
    }
    return window.analytix.runtime.importMappingPreview({
      kind: 'import_mapping_preview', selector,
      fields: [...IMPORT_MAPPING_PREVIEW_FIELDS], rowOffset,
      rowLimit: DIRECT_SOURCE_PREVIEW_PAGE_SIZE, displayMode: mode
    } satisfies ImportMappingPreviewRequest)
  }, [rowOffset, selector])
  const display = useEphemeralLocalDisplay(loader, authorityGeneration, `${selector}:${rowOffset}`)
  const response = display.response?.kind === 'import_mapping_preview' &&
    display.response.selector === selector && display.response.rowOffset === rowOffset
    ? display.response
    : null
  useEffect(() => setRowOffset(0), [authorityGeneration, selector])
  return (
    <LocalDisplayView
      response={response}
      unavailable={display.unavailable}
      onModeChange={display.setMode}
      pagination={response ? {
        hasPrevious: rowOffset > 0,
        hasNext: response.hasMore,
        onPrevious: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'previous', false)),
        onNext: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'next', response.hasMore))
      } : undefined}
    />
  )
}

export function CleaningDiffPreview({ selector }: { selector: string }): ReactElement | null {
  const authorityGeneration = useLocalDisplayAuthorityGeneration()
  const [rowOffset, setRowOffset] = useState(0)
  const loader = useCallback<LocalDisplayLoader>((mode) => {
    if (!validTypedLocalSelectorV1(selector)) {
      return Promise.resolve({ ok: false, status: 400, code: 'invalid_request', message: 'local display request is invalid' })
    }
    return window.analytix.runtime.cleaningDiffPreview({
      kind: 'cleaning_diff_preview', selector,
      fields: [...CLEANING_DIFF_PREVIEW_FIELDS], rowOffset,
      rowLimit: DIRECT_SOURCE_PREVIEW_PAGE_SIZE, displayMode: mode
    } satisfies CleaningDiffPreviewRequest)
  }, [rowOffset, selector])
  const display = useEphemeralLocalDisplay(loader, authorityGeneration, `${selector}:${rowOffset}`)
  const response = display.response?.kind === 'cleaning_diff_preview' &&
    display.response.selector === selector && display.response.rowOffset === rowOffset
    ? display.response
    : null
  useEffect(() => setRowOffset(0), [authorityGeneration, selector])
  const revocationLease = useRef<{ selector: string; revision: number } | null>(null)
  useEffect(() => {
    const previous = revocationLease.current
    const current = { selector, revision: (previous?.revision ?? 0) + 1 }
    revocationLease.current = current
    if (previous && previous.selector !== selector) {
      void window.analytix.runtime.revokeCleaningDiffPreview(previous.selector)
    }
    return () => queueMicrotask(() => {
      // React StrictMode immediately remounts effects in development. Only a
      // lease still current after that microtask represents a real unmount.
      if (revocationLease.current === current) {
        void window.analytix.runtime.revokeCleaningDiffPreview(current.selector)
      }
    })
  }, [selector])
  return (
    <LocalDisplayView
      response={response}
      unavailable={display.unavailable}
      onModeChange={display.setMode}
      pagination={response ? {
        hasPrevious: rowOffset > 0,
        hasNext: response.hasMore,
        onPrevious: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'previous', false)),
        onNext: () => setRowOffset((current) => directSourcePreviewPageOffset(current, 'next', response.hasMore))
      } : undefined}
    />
  )
}
