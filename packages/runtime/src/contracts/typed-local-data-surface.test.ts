import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  CLEANING_DIFF_PREVIEW_FIELDS_V1,
  DIRECT_SOURCE_PREVIEW_FIELDS_V1,
  IMPORT_MAPPING_PREVIEW_FIELDS_V1,
  TYPED_LOCAL_DATA_SURFACE_CANONICAL_CONTRACT_V1,
  TYPED_LOCAL_DATA_SURFACE_KINDS_V1,
  TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1,
  acceptedSlotDisplayResponseSchemaV1,
  cleaningDiffPreviewRequestSchemaV1,
  importMappingPreviewRequestSchemaV1,
  typedLocalDataSurfaceResponseSchemaV1
} from './typed-local-data-surface.js'

const digest = 'a'.repeat(64)
const selector = `tlsel1_${digest}`

describe('typed-local-data-surface/v1 public contract', () => {
  it('is the exact closed four-variant family and matches the Go canonical contract', () => {
    expect(TYPED_LOCAL_DATA_SURFACE_KINDS_V1).toEqual([
      'import_mapping_preview',
      'cleaning_diff_preview',
      'direct_source_preview',
      'accepted_slot_display'
    ])
    expect(CLEANING_DIFF_PREVIEW_FIELDS_V1).toEqual(DIRECT_SOURCE_PREVIEW_FIELDS_V1)
    expect(IMPORT_MAPPING_PREVIEW_FIELDS_V1).toEqual([
      'sourceColumn',
      'sampleValue',
      'inferredType',
      'targetField',
      'parseStatus',
      'mappingStatus'
    ])

    const goSource = readFileSync(
      fileURLToPath(new URL('../../../runtime-go/internal/domain/localdisplay/contract_v1.go', import.meta.url)),
      'utf8'
    )
    const canonicalJSON = goSource.match(/const CanonicalContractJSONV1 = `([^`]+)`/)?.[1]
    expect(canonicalJSON).toBeTruthy()
    expect(JSON.parse(canonicalJSON ?? '{}')).toEqual(TYPED_LOCAL_DATA_SURFACE_CANONICAL_CONTRACT_V1)
    expect(TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1).toBe(1)
    expect(goSource).toMatch(/ResponseSchemaVersionV1\s*=\s*1/)
  })

  it('fails closed on unknown kinds, fields, properties, duplicates, and request bounds', () => {
    const validImport = {
      kind: 'import_mapping_preview',
      selector,
      fields: ['sourceColumn', 'sampleValue'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    } as const
    expect(importMappingPreviewRequestSchemaV1.safeParse(validImport).success).toBe(true)
    for (const invalid of [
      { ...validImport, kind: 'unknown_preview' },
      { ...validImport, fields: ['unknownField'] },
      { ...validImport, fields: ['sourceColumn', 'sourceColumn'] },
      { ...validImport, rowOffset: 100_001 },
      { ...validImport, rowLimit: 101 },
      { ...validImport, path: '/private/source.csv' }
    ]) {
      expect(importMappingPreviewRequestSchemaV1.safeParse(invalid).success).toBe(false)
    }
    expect(cleaningDiffPreviewRequestSchemaV1.safeParse({
      ...validImport,
      kind: 'cleaning_diff_preview',
      fields: ['account']
    }).success).toBe(true)
  })

  it('requires an exact accepted-final digest and rejects unknown response properties', () => {
    const valid = {
      schemaVersion: 1,
      kind: 'accepted_slot_display',
      threadId: 'thread-accepted',
      turnId: 'turn-accepted',
      acceptedFinalDigest: digest,
      caseId: 'case-accepted',
      datasetSnapshotId: `dsv2_${digest}`,
      contextEpoch: 1,
      displayMode: 'full',
      slots: [{
        slotId: 'account-slot-1',
        field: 'account',
        displayValue: 'SOURCE_EXACT_CANARY',
        claimIds: ['claim-accepted'],
        receiptIds: ['receipt-accepted']
      }]
    } as const
    expect(acceptedSlotDisplayResponseSchemaV1.safeParse(valid).success).toBe(true)
    for (const invalid of [
      { ...valid, acceptedFinalDigest: undefined },
      { ...valid, acceptedFinalDigest: 'not-a-digest' },
      { ...valid, acceptedFinalDigest: 7 },
      { ...valid, acceptedFinalDigestExtra: digest }
    ]) {
      expect(acceptedSlotDisplayResponseSchemaV1.safeParse(invalid).success).toBe(false)
    }
  })

  it('rejects missing lineage, reordered or unrequested cells, and UTF-8 cell overflow', () => {
    const valid = {
      schemaVersion: 1,
      kind: 'import_mapping_preview',
      selector,
      lineage: {
        importGeneration: `tlgen1_${digest}`,
        sourceItemGeneration: `tlgen1_${'b'.repeat(64)}`,
        parserGeneration: `tlgen1_${'c'.repeat(64)}`,
        mappingGeneration: `tlgen1_${'d'.repeat(64)}`
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
          { field: 'sampleValue', displayValue: 'SOURCE_EXACT_CANARY' }
        ]
      }]
    } as const
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse(valid).success).toBe(true)
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse({ ...valid, lineage: undefined }).success).toBe(false)
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse({
      ...valid,
      rows: [{ ...valid.rows[0], cells: [...valid.rows[0].cells].reverse() }]
    }).success).toBe(false)
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse({
      ...valid,
      rows: [{
        ...valid.rows[0],
        cells: [valid.rows[0].cells[0], { field: 'targetField', displayValue: 'account' }]
      }]
    }).success).toBe(false)
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse({
      ...valid,
      rows: [{
        ...valid.rows[0],
        cells: [valid.rows[0].cells[0], { field: 'sampleValue', displayValue: '💠'.repeat(1_025) }]
      }]
    }).success).toBe(false)

    const oversizedRows = Array.from({ length: 100 }, (_, rowIndex) => ({
      rowIndex,
      status: 'changed',
      cells: DIRECT_SOURCE_PREVIEW_FIELDS_V1.map((field) => ({
        field,
        beforeDisplayValue: 'a'.repeat(2_048),
        afterDisplayValue: 'b'.repeat(2_048)
      }))
    }))
    expect(typedLocalDataSurfaceResponseSchemaV1.safeParse({
      schemaVersion: 1,
      kind: 'cleaning_diff_preview',
      selector,
      lineage: {
        inputSnapshot: `tlsnap1_${'1'.repeat(64)}`,
        ruleGeneration: `tlgen1_${'2'.repeat(64)}`,
        ruleDigest: '3'.repeat(64),
        outputSnapshot: `tlsnap1_${'4'.repeat(64)}`,
        transformLineage: `tllin1_${'5'.repeat(64)}`
      },
      displayMode: 'full',
      fields: [...DIRECT_SOURCE_PREVIEW_FIELDS_V1],
      rowOffset: 0,
      rowLimit: 100,
      hasMore: false,
      rows: oversizedRows
    }).success).toBe(false)
  })
})
