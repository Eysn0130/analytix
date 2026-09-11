import { z } from 'zod'

export const TYPED_LOCAL_DATA_SURFACE_SCHEMA_V1 = 'typed-local-data-surface/v1' as const
export const TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1 = 1 as const
export const TYPED_LOCAL_DATA_SURFACE_KINDS_V1 = [
  'import_mapping_preview',
  'cleaning_diff_preview',
  'direct_source_preview',
  'accepted_slot_display'
] as const
export const TYPED_LOCAL_DATA_SURFACE_DISPLAY_MODES_V1 = ['full', 'masked'] as const
export const IMPORT_MAPPING_PREVIEW_FIELDS_V1 = [
  'sourceColumn',
  'sampleValue',
  'inferredType',
  'targetField',
  'parseStatus',
  'mappingStatus'
] as const
export const DIRECT_SOURCE_PREVIEW_FIELDS_V1 = [
  'transactionTime',
  'account',
  'card',
  'accountName',
  'identityNumber',
  'amountText',
  'direction',
  'counterpartyAccount',
  'counterpartyName',
  'counterpartyIdentityNumber',
  'counterpartyBank',
  'summary',
  'currency',
  'merchantName',
  'remark'
] as const
export const CLEANING_DIFF_PREVIEW_FIELDS_V1 = [...DIRECT_SOURCE_PREVIEW_FIELDS_V1] as const

export const TYPED_LOCAL_DATA_SURFACE_MAX_ROW_OFFSET_V1 = 100_000
export const TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1 = 100
export const TYPED_LOCAL_DATA_SURFACE_MAX_CELL_BYTES_V1 = 4_096
export const TYPED_LOCAL_DATA_SURFACE_MAX_RESPONSE_BYTES_V1 = 1 << 20

const selectorPattern = '^tlsel1_[a-f0-9]{64}$'
const generationPattern = '^tlgen1_[a-f0-9]{64}$'
const snapshotPattern = '^tlsnap1_[a-f0-9]{64}$'
const lineagePattern = '^tllin1_[a-f0-9]{64}$'

export const TYPED_LOCAL_DATA_SURFACE_CANONICAL_CONTRACT_V1 = {
  schema: TYPED_LOCAL_DATA_SURFACE_SCHEMA_V1,
  kinds: TYPED_LOCAL_DATA_SURFACE_KINDS_V1,
  displayModes: TYPED_LOCAL_DATA_SURFACE_DISPLAY_MODES_V1,
  fields: {
    import_mapping_preview: IMPORT_MAPPING_PREVIEW_FIELDS_V1,
    cleaning_diff_preview: CLEANING_DIFF_PREVIEW_FIELDS_V1,
    direct_source_preview: DIRECT_SOURCE_PREVIEW_FIELDS_V1,
    accepted_slot_display: ['account', 'card']
  },
  statuses: {
    importParse: ['parsed', 'invalid', 'unsupported'],
    importMapping: ['mapped', 'unmapped', 'conflict'],
    cleaning: ['unchanged', 'changed', 'added', 'removed', 'invalid']
  },
  selectors: {
    request: selectorPattern,
    generation: generationPattern,
    snapshot: snapshotPattern,
    lineage: lineagePattern
  },
  requestProperties: {
    import_mapping_preview: ['kind', 'selector', 'fields', 'rowOffset', 'rowLimit', 'displayMode'],
    cleaning_diff_preview: ['kind', 'selector', 'fields', 'rowOffset', 'rowLimit', 'displayMode'],
    direct_source_preview: ['kind', 'view', 'fields', 'rowOffset', 'rowLimit', 'displayMode'],
    accepted_slot_display: ['kind', 'threadId', 'turnId', 'acceptedFinalDigest', 'displayMode']
  },
  responseProperties: {
    import_mapping_preview: ['schemaVersion', 'kind', 'selector', 'lineage', 'displayMode', 'fields', 'rowOffset', 'rowLimit', 'hasMore', 'rows'],
    cleaning_diff_preview: ['schemaVersion', 'kind', 'selector', 'lineage', 'displayMode', 'fields', 'rowOffset', 'rowLimit', 'hasMore', 'rows'],
    direct_source_preview: ['schemaVersion', 'kind', 'caseId', 'datasetSnapshotId', 'displayMode', 'view', 'fields', 'rowOffset', 'rowLimit', 'hasMore', 'rows'],
    accepted_slot_display: ['schemaVersion', 'kind', 'threadId', 'turnId', 'acceptedFinalDigest', 'caseId', 'datasetSnapshotId', 'contextEpoch', 'displayMode', 'slots']
  },
  lineageProperties: {
    import_mapping_preview: ['importGeneration', 'sourceItemGeneration', 'parserGeneration', 'mappingGeneration'],
    cleaning_diff_preview: ['inputSnapshot', 'ruleGeneration', 'ruleDigest', 'outputSnapshot', 'transformLineage'],
    direct_source_preview: [],
    accepted_slot_display: []
  },
  rowProperties: {
    import_mapping_preview: ['rowIndex', 'parseStatus', 'mappingStatus', 'cells'],
    cleaning_diff_preview: ['rowIndex', 'status', 'cells'],
    direct_source_preview: ['rowIndex', 'cells'],
    accepted_slot_display: ['slotId', 'field', 'displayValue', 'claimIds', 'receiptIds']
  },
  cellProperties: {
    import_mapping_preview: ['field', 'displayValue'],
    cleaning_diff_preview: ['field', 'beforeDisplayValue', 'afterDisplayValue'],
    direct_source_preview: ['field', 'displayValue'],
    accepted_slot_display: []
  },
  limits: {
    rowOffset: TYPED_LOCAL_DATA_SURFACE_MAX_ROW_OFFSET_V1,
    rowLimit: TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1,
    cellBytes: TYPED_LOCAL_DATA_SURFACE_MAX_CELL_BYTES_V1,
    responseBytes: TYPED_LOCAL_DATA_SURFACE_MAX_RESPONSE_BYTES_V1
  }
} as const

export type TypedLocalDataSurfaceKindV1 = typeof TYPED_LOCAL_DATA_SURFACE_KINDS_V1[number]
export type TypedLocalDataSurfaceDisplayModeV1 = typeof TYPED_LOCAL_DATA_SURFACE_DISPLAY_MODES_V1[number]
export type ImportMappingPreviewFieldV1 = typeof IMPORT_MAPPING_PREVIEW_FIELDS_V1[number]
export type CleaningDiffPreviewFieldV1 = typeof CLEANING_DIFF_PREVIEW_FIELDS_V1[number]
export type DirectSourcePreviewFieldV1 = typeof DIRECT_SOURCE_PREVIEW_FIELDS_V1[number]

const displayModeSchema = z.enum(TYPED_LOCAL_DATA_SURFACE_DISPLAY_MODES_V1)
const selectorSchema = z.string().regex(new RegExp(selectorPattern))
const generationSchema = z.string().regex(new RegExp(generationPattern))
const snapshotSchema = z.string().regex(new RegExp(snapshotPattern))
const lineageSchema = z.string().regex(new RegExp(lineagePattern))
const digestSchema = z.string().regex(/^[a-f0-9]{64}$/)
const boundedIDSchema = z.string().trim().min(1).max(256)
const rowOffsetSchema = z.number().int().min(0).max(TYPED_LOCAL_DATA_SURFACE_MAX_ROW_OFFSET_V1).safe()
const rowLimitSchema = z.number().int().min(1).max(TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1).safe()

function uniqueFieldsSchema<T extends readonly [string, ...string[]]>(values: T) {
  return z.array(z.enum(values)).min(1).max(values.length).superRefine((fields, context) => {
    if (new Set(fields).size !== fields.length) {
      context.addIssue({ code: 'custom', message: 'typed-local fields must be unique' })
    }
  })
}

function utf8CellSchema() {
  return z.string().refine(
    (value) => new TextEncoder().encode(value).byteLength <= TYPED_LOCAL_DATA_SURFACE_MAX_CELL_BYTES_V1,
    'typed-local cell exceeds the UTF-8 byte limit'
  )
}

const importFieldsSchema = uniqueFieldsSchema(IMPORT_MAPPING_PREVIEW_FIELDS_V1)
const cleaningFieldsSchema = uniqueFieldsSchema(CLEANING_DIFF_PREVIEW_FIELDS_V1)
const directFieldsSchema = uniqueFieldsSchema(DIRECT_SOURCE_PREVIEW_FIELDS_V1)

export const importMappingPreviewRequestSchemaV1 = z.object({
  kind: z.literal('import_mapping_preview'),
  selector: selectorSchema,
  fields: importFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  displayMode: displayModeSchema
}).strict()

export const cleaningDiffPreviewRequestSchemaV1 = z.object({
  kind: z.literal('cleaning_diff_preview'),
  selector: selectorSchema,
  fields: cleaningFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  displayMode: displayModeSchema
}).strict()

export const directSourcePreviewRequestSchemaV1 = z.object({
  kind: z.literal('direct_source_preview'),
  view: z.literal('transactions'),
  fields: directFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  displayMode: displayModeSchema
}).strict()

export const acceptedSlotDisplayRequestSchemaV1 = z.object({
  kind: z.literal('accepted_slot_display'),
  threadId: boundedIDSchema,
  turnId: boundedIDSchema,
  acceptedFinalDigest: digestSchema,
  displayMode: displayModeSchema
}).strict()

export const typedLocalDataSurfaceRequestSchemaV1 = z.union([
  importMappingPreviewRequestSchemaV1,
  cleaningDiffPreviewRequestSchemaV1,
  directSourcePreviewRequestSchemaV1,
  acceptedSlotDisplayRequestSchemaV1
])

const importCellSchema = z.object({
  field: z.enum(IMPORT_MAPPING_PREVIEW_FIELDS_V1),
  displayValue: utf8CellSchema()
}).strict()
const cleaningCellSchema = z.object({
  field: z.enum(CLEANING_DIFF_PREVIEW_FIELDS_V1),
  beforeDisplayValue: utf8CellSchema(),
  afterDisplayValue: utf8CellSchema()
}).strict().superRefine((value, context) => {
  const bytes = new TextEncoder().encode(value.beforeDisplayValue).byteLength +
    new TextEncoder().encode(value.afterDisplayValue).byteLength
  if (bytes > TYPED_LOCAL_DATA_SURFACE_MAX_CELL_BYTES_V1) {
    context.addIssue({ code: 'custom', message: 'typed-local cleaning cell exceeds the UTF-8 byte limit' })
  }
})
const directCellSchema = z.object({
  field: z.enum(DIRECT_SOURCE_PREVIEW_FIELDS_V1),
  displayValue: utf8CellSchema()
}).strict()

function validateWindowAndCells(
  value: {
    fields: readonly string[]
    rowOffset: number
    rowLimit: number
    rows: readonly { rowIndex: number; cells: readonly { field: string }[] }[]
  },
  context: z.RefinementCtx
): void {
  if (value.rows.length > value.rowLimit) {
    context.addIssue({ code: 'custom', path: ['rows'], message: 'typed-local rows exceed the requested limit' })
  }
  value.rows.forEach((row, rowPosition) => {
    if (row.rowIndex !== value.rowOffset + rowPosition) {
      context.addIssue({
        code: 'custom',
        path: ['rows', rowPosition, 'rowIndex'],
        message: 'typed-local row is outside the stable requested window'
      })
    }
    if (
      row.cells.length !== value.fields.length ||
      row.cells.some((cell, cellPosition) => cell.field !== value.fields[cellPosition])
    ) {
      context.addIssue({
        code: 'custom',
        path: ['rows', rowPosition, 'cells'],
        message: 'typed-local cells do not exactly match the requested field order'
      })
    }
  })
}

export const importMappingPreviewResponseSchemaV1 = z.object({
  schemaVersion: z.literal(TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1),
  kind: z.literal('import_mapping_preview'),
  selector: selectorSchema,
  lineage: z.object({
    importGeneration: generationSchema,
    sourceItemGeneration: generationSchema,
    parserGeneration: generationSchema,
    mappingGeneration: generationSchema
  }).strict(),
  displayMode: displayModeSchema,
  fields: importFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  hasMore: z.boolean(),
  rows: z.array(z.object({
    rowIndex: rowOffsetSchema,
    parseStatus: z.enum(['parsed', 'invalid', 'unsupported']),
    mappingStatus: z.enum(['mapped', 'unmapped', 'conflict']),
    cells: z.array(importCellSchema).min(1).max(IMPORT_MAPPING_PREVIEW_FIELDS_V1.length)
  }).strict()).max(TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1)
}).strict().superRefine((value, context) => {
  validateWindowAndCells(value, context)
  value.rows.forEach((row, rowPosition) => {
    row.cells.forEach((cell, cellPosition) => {
      if (
        (cell.field === 'parseStatus' && cell.displayValue !== row.parseStatus) ||
        (cell.field === 'mappingStatus' && cell.displayValue !== row.mappingStatus)
      ) {
        context.addIssue({
          code: 'custom',
          path: ['rows', rowPosition, 'cells', cellPosition, 'displayValue'],
          message: 'typed-local import status cell does not match its typed row status'
        })
      }
    })
  })
})

export const cleaningDiffPreviewResponseSchemaV1 = z.object({
  schemaVersion: z.literal(TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1),
  kind: z.literal('cleaning_diff_preview'),
  selector: selectorSchema,
  lineage: z.object({
    inputSnapshot: snapshotSchema,
    ruleGeneration: generationSchema,
    ruleDigest: digestSchema,
    outputSnapshot: snapshotSchema,
    transformLineage: lineageSchema
  }).strict(),
  displayMode: displayModeSchema,
  fields: cleaningFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  hasMore: z.boolean(),
  rows: z.array(z.object({
    rowIndex: rowOffsetSchema,
    status: z.enum(['unchanged', 'changed', 'added', 'removed', 'invalid']),
    cells: z.array(cleaningCellSchema).min(1).max(CLEANING_DIFF_PREVIEW_FIELDS_V1.length)
  }).strict()).max(TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1)
}).strict().superRefine(validateWindowAndCells)

export const directSourcePreviewResponseSchemaV1 = z.object({
  schemaVersion: z.literal(TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1),
  kind: z.literal('direct_source_preview'),
  caseId: boundedIDSchema,
  datasetSnapshotId: boundedIDSchema,
  displayMode: displayModeSchema,
  view: z.literal('transactions'),
  fields: directFieldsSchema,
  rowOffset: rowOffsetSchema,
  rowLimit: rowLimitSchema,
  hasMore: z.boolean(),
  rows: z.array(z.object({
    rowIndex: rowOffsetSchema,
    cells: z.array(directCellSchema).min(1).max(DIRECT_SOURCE_PREVIEW_FIELDS_V1.length)
  }).strict()).max(TYPED_LOCAL_DATA_SURFACE_MAX_ROW_LIMIT_V1)
}).strict().superRefine(validateWindowAndCells)

export const acceptedSlotDisplayResponseSchemaV1 = z.object({
  schemaVersion: z.literal(TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1),
  kind: z.literal('accepted_slot_display'),
  threadId: boundedIDSchema,
  turnId: boundedIDSchema,
  acceptedFinalDigest: digestSchema,
  caseId: boundedIDSchema,
  datasetSnapshotId: boundedIDSchema,
  contextEpoch: z.number().int().positive().safe(),
  displayMode: displayModeSchema,
  slots: z.array(z.object({
    slotId: boundedIDSchema,
    field: z.enum(['account', 'card']),
    displayValue: utf8CellSchema(),
    claimIds: z.array(boundedIDSchema).min(1).max(64),
    receiptIds: z.array(boundedIDSchema).min(1).max(64)
  }).strict()).max(256)
}).strict()

export const typedLocalDataSurfaceResponseSchemaV1 = z.union([
  importMappingPreviewResponseSchemaV1,
  cleaningDiffPreviewResponseSchemaV1,
  directSourcePreviewResponseSchemaV1,
  acceptedSlotDisplayResponseSchemaV1
]).superRefine((value, context) => {
  if (new TextEncoder().encode(JSON.stringify(value)).byteLength > TYPED_LOCAL_DATA_SURFACE_MAX_RESPONSE_BYTES_V1) {
    context.addIssue({ code: 'custom', message: 'typed-local response exceeds the UTF-8 byte limit' })
  }
})

export type ImportMappingPreviewRequestV1 = z.infer<typeof importMappingPreviewRequestSchemaV1>
export type CleaningDiffPreviewRequestV1 = z.infer<typeof cleaningDiffPreviewRequestSchemaV1>
export type DirectSourcePreviewRequestV1 = z.infer<typeof directSourcePreviewRequestSchemaV1>
export type AcceptedSlotDisplayRequestV1 = z.infer<typeof acceptedSlotDisplayRequestSchemaV1>
export type TypedLocalDataSurfaceRequestV1 = z.infer<typeof typedLocalDataSurfaceRequestSchemaV1>
export type ImportMappingPreviewResponseV1 = z.infer<typeof importMappingPreviewResponseSchemaV1>
export type CleaningDiffPreviewResponseV1 = z.infer<typeof cleaningDiffPreviewResponseSchemaV1>
export type DirectSourcePreviewResponseV1 = z.infer<typeof directSourcePreviewResponseSchemaV1>
export type AcceptedSlotDisplayResponseV1 = z.infer<typeof acceptedSlotDisplayResponseSchemaV1>
export type TypedLocalDataSurfaceResponseV1 = z.infer<typeof typedLocalDataSurfaceResponseSchemaV1>
