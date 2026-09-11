import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import acceptedSlotDisplaySource from './AcceptedSlotDisplay.tsx?raw'
import type {
  AcceptedSlotDisplayResponse,
  CleaningDiffPreviewResponse,
  DirectSourcePreviewResponse,
  ImportMappingPreviewResponse,
  LocalDisplayResponse
} from '@shared/analytix-api'
import type { AcceptedFinalProjectionReceiptV1 } from '../../agent/types'
import {
  emitStatsCacheInvalidation,
  subscribeStatsCacheInvalidation
} from '../../data-analysis/services/analysis/stats-cache-events'
import {
  setSharedActiveCaseContext,
  subscribeSharedActiveCaseContext
} from '../../data-analysis/services/analysis/stats-case-overview-resource'
import {
  acceptedFinalHasLocalDisplaySlots,
  acceptedSlotDisplayRequest,
  acceptedSlotDisplayRequestGeneration,
  createLocalDisplayRendererLeaseV1,
  directSourcePreviewPageOffset,
  DIRECT_SOURCE_PREVIEW_PAGE_SIZE,
  LOCAL_DISPLAY_RENDERER_LEASE_MS,
  localDisplayAuthorityGenerationV1,
  localDisplayRequestAuthorityV1,
  localDisplayResponseForCurrentAuthority,
  LocalDisplayView
} from './AcceptedSlotDisplay'

const digest = 'f'.repeat(64)

function receipt(overrides: Partial<AcceptedFinalProjectionReceiptV1> = {}): AcceptedFinalProjectionReceiptV1 {
  return {
    schemaVersion: 1,
    batchId: 'batch-1',
    threadId: 'thread-1',
    turnId: 'turn-1',
    publicationCommitId: digest,
    lastSeq: 7,
    ...overrides
  }
}

function response(
  displayMode: 'full' | 'masked',
  field: 'account' | 'card' = 'account'
): AcceptedSlotDisplayResponse {
  return {
    schemaVersion: 1,
    kind: 'accepted_slot_display',
    threadId: 'thread-1',
    turnId: 'turn-1',
    acceptedFinalDigest: digest,
    caseId: 'case-1',
    datasetSnapshotId: 'dsv2:test',
    contextEpoch: 3,
    displayMode,
    slots: [{
      slotId: `${field}-slot-1`,
      field,
      displayValue: displayMode === 'full' ? '0012345678901234' : '****1234',
      claimIds: ['clm_1'],
      receiptIds: ['evr_1']
    }]
  }
}

function directResponse(displayMode: 'full' | 'masked'): DirectSourcePreviewResponse {
  return {
    schemaVersion: 1,
    kind: 'direct_source_preview',
    caseId: 'case-1',
    datasetSnapshotId: 'dsv2:test',
    displayMode,
    view: 'transactions',
    fields: ['transactionTime', 'account', 'amountText', 'counterpartyName'],
    rowOffset: 0,
    rowLimit: 25,
    hasMore: false,
    rows: [{
      rowIndex: 0,
      cells: [
        { field: 'transactionTime', displayValue: '2026-08-16 10:00:00' },
        { field: 'account', displayValue: displayMode === 'full' ? '0012345678901234' : '****1234' },
        { field: 'amountText', displayValue: '123.45' },
        { field: 'counterpartyName', displayValue: 'SOURCE_EXACT_NAME' }
      ]
    }]
  }
}

describe('AcceptedSlotDisplay', () => {
  it('observes active-case switches and current-case snapshot invalidation on existing seams', () => {
    const cases: string[] = []
    const snapshots: string[] = []
    const stopCases = subscribeSharedActiveCaseContext((current) => cases.push(current?.caseId ?? ''))
    const stopSnapshots = subscribeStatsCacheInvalidation((event) => snapshots.push(event.caseId))
    try {
      setSharedActiveCaseContext({ caseId: 'case-a', workspaceRoot: '/workspace' })
      setSharedActiveCaseContext({ caseId: 'case-b', workspaceRoot: '/workspace' })
      emitStatsCacheInvalidation({
        caseId: 'case-b',
        eventId: 'snapshot-b-2',
        source: 'test',
        occurredAt: 1
      })
      expect(cases).toEqual(['case-a', 'case-b'])
      expect(snapshots).toEqual(['case-b'])
    } finally {
      stopCases()
      stopSnapshots()
      setSharedActiveCaseContext(null)
    }
  })

  it('only mounts for accepted finals with both claims and receipts', () => {
    expect(acceptedFinalHasLocalDisplaySlots(undefined)).toBe(false)
    expect(acceptedFinalHasLocalDisplaySlots({
      claimCount: 0,
      receiptMetadata: { count: 1 }
    } as any)).toBe(false)
    expect(acceptedFinalHasLocalDisplaySlots({
      claimCount: 1,
      receiptMetadata: { count: 0 }
    } as any)).toBe(false)
    expect(acceptedFinalHasLocalDisplaySlots({
      claimCount: 1,
      receiptMetadata: { count: 1 }
    } as any)).toBe(true)
  })

  it('derives the typed request only from the committed projection receipt', () => {
    expect(acceptedSlotDisplayRequest(receipt(), 'full')).toEqual({
      kind: 'accepted_slot_display',
      threadId: 'thread-1',
      turnId: 'turn-1',
      acceptedFinalDigest: digest,
      displayMode: 'full'
    })
    expect(acceptedSlotDisplayRequest(receipt({ publicationCommitId: 'not-a-digest' }), 'full')).toBeNull()
    expect(acceptedSlotDisplayRequest(receipt({ threadId: ' ' }), 'full')).toBeNull()
  })

  it('invalidates the prior ticket and response synchronously when the receipt prop generation changes', () => {
    const authorityGeneration = localDisplayAuthorityGenerationV1({
      rendererSession: 'renderer-session-a',
      caseBinding: 'case-a:binding-a:3',
      snapshotGeneration: 7
    })
    const priorReceipt = receipt()
    const replacementReceipt = receipt({
      publicationCommitId: 'e'.repeat(64)
    })
    const priorGeneration = acceptedSlotDisplayRequestGeneration(priorReceipt)
    const replacementGeneration = acceptedSlotDisplayRequestGeneration(replacementReceipt)
    const priorAuthority = localDisplayRequestAuthorityV1(
      authorityGeneration,
      priorGeneration,
      'full'
    )
    const replacementAuthority = localDisplayRequestAuthorityV1(
      authorityGeneration,
      replacementGeneration,
      'full'
    )
    const lease = createLocalDisplayRendererLeaseV1(() => undefined)
    const priorTicket = lease.issue(priorAuthority)
    expect(lease.accept(priorTicket)).toBe(true)
    expect(localDisplayResponseForCurrentAuthority(
      response('full'),
      priorAuthority,
      priorAuthority
    )).not.toBeNull()

    // This is the render-time prop transition: bind runs before effect cleanup/refresh.
    lease.bind(replacementAuthority)

    expect(priorGeneration).toBe(JSON.stringify(['thread-1', 'turn-1', digest]))
    expect(replacementGeneration).toBe(JSON.stringify([
      'thread-1',
      'turn-1',
      'e'.repeat(64)
    ]))
    expect(replacementAuthority).not.toBe(priorAuthority)
    expect(localDisplayResponseForCurrentAuthority(
      response('full'),
      priorAuthority,
      replacementAuthority
    )).toBeNull()
    expect(lease.isCurrent(priorTicket)).toBe(false)
    expect(lease.accept(priorTicket)).toBe(false)
    lease.revoke()
  })

  it('renders source-exact full bytes only inside the typed local display sink', () => {
    const exact = '0012345678901234'
    const html = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: response('full'),
      unavailable: false,
      onModeChange: () => undefined
    }))
    expect(html).toContain('data-analytix-local-display="accepted_slot_display"')
    expect(html).toContain('data-analytix-local-display-mode="full"')
    expect(html).toContain(exact)
    expect(html).not.toContain('clm_1')
    expect(html).not.toContain('evr_1')
    expect(html).not.toContain('case-1')
    expect(html).not.toContain('dsv2:test')
    expect(html).not.toContain(digest)
    expect(html).not.toContain('account-slot-1')
    expect(html).toContain('Account')
  })

  it('renders masked bytes without changing typed claim or receipt bindings', () => {
    const masked = response('masked', 'card')
    const html = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: masked,
      unavailable: false,
      onModeChange: () => undefined
    }))
    expect(masked.slots[0]?.claimIds).toEqual(['clm_1'])
    expect(masked.slots[0]?.receiptIds).toEqual(['evr_1'])
    expect(masked.slots[0]?.field).toBe('card')
    expect(html).toContain('****1234')
    expect(html).not.toContain('0012345678901234')
  })

  it('renders typed transaction rows without exposing case or snapshot authority metadata', () => {
    const html = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: directResponse('full') satisfies LocalDisplayResponse,
      unavailable: false,
      onModeChange: () => undefined,
      pagination: {
        hasPrevious: false,
        hasNext: true,
        onPrevious: () => undefined,
        onNext: () => undefined
      }
    }))
    expect(html).toContain('data-analytix-local-display="direct_source_preview"')
    expect(html).toContain('2026-08-16 10:00:00')
    expect(html).toContain('0012345678901234')
    expect(html).toContain('SOURCE_EXACT_NAME')
    expect(html).not.toContain('case-1')
    expect(html).not.toContain('dsv2:test')
    expect(html).toContain('data-analytix-source-page="previous"')
    expect(html).toContain('data-analytix-source-page="next"')
    expect(html.match(/disabled=""/g)).toHaveLength(1)
  })

  it('renders synthetic import and cleaning canaries only inside their typed component-local views', () => {
    const selector = `tlsel1_${'a'.repeat(64)}`
    const importResponse: ImportMappingPreviewResponse = {
      schemaVersion: 1,
      kind: 'import_mapping_preview',
      selector,
      lineage: {
        importGeneration: `tlgen1_${'1'.repeat(64)}`,
        sourceItemGeneration: `tlgen1_${'2'.repeat(64)}`,
        parserGeneration: `tlgen1_${'3'.repeat(64)}`,
        mappingGeneration: `tlgen1_${'4'.repeat(64)}`
      },
      displayMode: 'full',
      fields: ['sourceColumn', 'sampleValue'],
      rowOffset: 0,
      rowLimit: 25,
      hasMore: false,
      rows: [{
        rowIndex: 0,
        parseStatus: 'parsed',
        mappingStatus: 'mapped',
        cells: [
          { field: 'sourceColumn', displayValue: '交易账号' },
          { field: 'sampleValue', displayValue: 'IMPORT_SOURCE_EXACT_CANARY' }
        ]
      }]
    }
    const cleaningResponse: CleaningDiffPreviewResponse = {
      schemaVersion: 1,
      kind: 'cleaning_diff_preview',
      selector,
      lineage: {
        inputSnapshot: `tlsnap1_${'5'.repeat(64)}`,
        ruleGeneration: `tlgen1_${'6'.repeat(64)}`,
        ruleDigest: '7'.repeat(64),
        outputSnapshot: `tlsnap1_${'8'.repeat(64)}`,
        transformLineage: `tllin1_${'9'.repeat(64)}`
      },
      displayMode: 'full',
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 25,
      hasMore: false,
      rows: [{
        rowIndex: 0,
        status: 'changed',
        cells: [{
          field: 'account',
          beforeDisplayValue: 'CLEANING_BEFORE_CANARY',
          afterDisplayValue: 'CLEANING_AFTER_CANARY'
        }]
      }]
    }
    const importHTML = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: importResponse,
      unavailable: false,
      onModeChange: () => undefined,
      pagination: {
        hasPrevious: false,
        hasNext: true,
        onPrevious: () => undefined,
        onNext: () => undefined
      }
    }))
    const cleaningHTML = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: cleaningResponse,
      unavailable: false,
      onModeChange: () => undefined
    }))
    expect(importHTML).toContain('data-analytix-local-display="import_mapping_preview"')
    expect(importHTML).toContain('IMPORT_SOURCE_EXACT_CANARY')
    expect(importHTML).toContain('data-analytix-source-page="previous"')
    expect(importHTML).toContain('data-analytix-source-page="next"')
    expect(cleaningHTML).toContain('data-analytix-local-display="cleaning_diff_preview"')
    expect(cleaningHTML).toContain('CLEANING_BEFORE_CANARY')
    expect(cleaningHTML).toContain('CLEANING_AFTER_CANARY')
    for (const html of [importHTML, cleaningHTML]) {
      expect(html).not.toContain(selector)
      expect(html).not.toContain('tlgen1_')
      expect(html).not.toContain('tlsnap1_')
      expect(html).not.toContain('tllin1_')
    }
  })

  it('uses a fixed bounded page size and only advances when hasMore is true', () => {
    expect(DIRECT_SOURCE_PREVIEW_PAGE_SIZE).toBe(25)
    expect(directSourcePreviewPageOffset(0, 'previous', false)).toBe(0)
    expect(directSourcePreviewPageOffset(25, 'previous', false)).toBe(0)
    expect(directSourcePreviewPageOffset(0, 'next', false)).toBe(0)
    expect(directSourcePreviewPageOffset(0, 'next', true)).toBe(25)
    expect(directSourcePreviewPageOffset(100_000, 'next', true)).toBe(100_000)
  })

  it('clears the typed value when current authority is unavailable', () => {
    const html = renderToStaticMarkup(createElement(LocalDisplayView, {
      response: null,
      unavailable: true,
      onModeChange: () => undefined
    }))
    expect(html).toContain('data-analytix-local-display="unavailable"')
    expect(html).not.toContain('0012345678901234')
  })

  it('withholds a prior response synchronously after case, snapshot, or renderer session changes', () => {
    const prior = directResponse('full')
    const base = {
      rendererSession: 'thread-a:/workspace-a:connected:17',
      caseBinding: 'case-a:/workspace-a:1',
      snapshotGeneration: 0
    }
    const original = localDisplayAuthorityGenerationV1(base)
    expect(localDisplayResponseForCurrentAuthority(prior, original, original)).toBe(prior)
    for (const changed of [
      { ...base, rendererSession: 'thread-b:/workspace-a:connected:18' },
      { ...base, caseBinding: 'case-b:/workspace-b:2' },
      { ...base, snapshotGeneration: 1 }
    ]) {
      const current = localDisplayResponseForCurrentAuthority(
        prior,
        original,
        localDisplayAuthorityGenerationV1(changed)
      )
      const html = renderToStaticMarkup(createElement(LocalDisplayView, {
        response: current,
        unavailable: false,
        onModeChange: () => undefined
      }))
      expect(current).toBeNull()
      expect(html).not.toContain('0012345678901234')
      expect(html).not.toContain('SOURCE_EXACT_NAME')
    }
  })

  it('keeps ordinary local display source free of Hub account authority', () => {
    expect(acceptedSlotDisplaySource).not.toContain('hub-account-store')
    expect(acceptedSlotDisplaySource).not.toContain('useHubAccountStore')
    expect(acceptedSlotDisplaySource).not.toContain('principalAuthority')
  })

  it('withholds reuse after selector, generation, mode, or page changes', () => {
    const prior = directResponse('full')
    const priorAuthority = 'renderer:principal:case:snapshot:tlsel1_generation-a:0:full'
    for (const currentAuthority of [
      'renderer:principal:case:snapshot:tlsel1_generation-b:0:full',
      'renderer:principal:case:snapshot:tlsel1_generation-a:25:full',
      'renderer:principal:case:snapshot:tlsel1_generation-a:0:masked',
      'renderer:principal:case:new-snapshot:tlsel1_generation-a:0:full'
    ]) {
      expect(localDisplayResponseForCurrentAuthority(prior, priorAuthority, currentAuthority)).toBeNull()
    }
  })

  it('expires component-local exact bytes and rejects a response that settles after expiry', () => {
    vi.useFakeTimers()
    try {
      const expired: string[] = []
      const lease = createLocalDisplayRendererLeaseV1((authority) => expired.push(authority))
      const ticket = lease.issue('renderer:principal:case:snapshot:full')
      expect(lease.accept(ticket)).toBe(true)

      vi.advanceTimersByTime(LOCAL_DISPLAY_RENDERER_LEASE_MS)

      expect(expired).toEqual(['renderer:principal:case:snapshot:full'])
      expect(lease.isCurrent(ticket)).toBe(false)
      expect(lease.accept(ticket)).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps only the newest generation and makes revoke or unmount invalidate late responses', () => {
    const lease = createLocalDisplayRendererLeaseV1(() => undefined)
    const first = lease.issue('case-a:snapshot-1:full')
    const newest = lease.issue('case-a:snapshot-2:full')

    expect(lease.isCurrent(first)).toBe(false)
    expect(lease.accept(first)).toBe(false)
    expect(lease.accept(newest)).toBe(true)

    lease.revoke()
    expect(lease.isCurrent(newest)).toBe(false)
    expect(lease.accept(newest)).toBe(false)
  })

  it('wires renderer-lease revocation into literal component effect cleanup', () => {
    expect(acceptedSlotDisplaySource).toContain(`return () => {
      lease.current?.revoke()`)
    expect(acceptedSlotDisplaySource).toContain('lease.current.bind(requestAuthority)')
    expect(acceptedSlotDisplaySource).toContain(
      'const requestGeneration = acceptedSlotDisplayRequestGeneration(receipt)'
    )
    for (const component of [
      'AcceptedSlotDisplay',
      'DirectSourcePreview',
      'ImportMappingPreview',
      'CleaningDiffPreview'
    ]) {
      expect(acceptedSlotDisplaySource).toContain(`function ${component}`)
    }
    expect(acceptedSlotDisplaySource.match(/useEphemeralLocalDisplay\(/g)).toHaveLength(5)
  })
})
