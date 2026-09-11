import { describe, expect, it } from 'vitest'
import {
  PublicMcpCatalogV2,
  PublicMcpSearchV2,
  PublicModelReasoningV2,
  PublicRuntimeCapabilityStateV2,
  PublicRuntimeIdentifierV2
} from '../src/contracts/runtime-info.js'
import {
  PublicCommandDiagnosticV2,
  PublicMcpServerDiagnosticV2,
  PublicSkillDiagnosticsV2,
  PublicSubagentDiagnosticsV2
} from '../src/contracts/runtime-tools.js'

describe('public runtime diagnostics v2 mechanical invariants', () => {
  it('accepts only identifiers the Go projector can issue', () => {
    for (const value of ['deepseek-v4-pro', 'mimo/v2.5+fast', '分析模型-1']) {
      expect(PublicRuntimeIdentifierV2.safeParse(value).success).toBe(true)
    }
    for (const value of [
      ' padded ',
      '/private/case',
      '~/private/case',
      'C:/private/case',
      '1:/private/case',
      'owner@example.invalid',
      'server-6222021234567890',
      'contains space',
      'contains<angle>'
    ]) {
      expect(PublicRuntimeIdentifierV2.safeParse(value).success).toBe(false)
    }
  })

  it('rejects contradictory generic capability states', () => {
    expect(PublicRuntimeCapabilityStateV2.safeParse({
      status: 'available', enabled: true, available: true, reasonCode: 'available'
    }).success).toBe(true)
    for (const value of [
      { status: 'available', enabled: true, available: false, reasonCode: 'available' },
      { status: 'disabled', enabled: true, available: false, reasonCode: 'disabled_by_config' },
      { status: 'unavailable', enabled: true, available: false, reasonCode: 'available' }
    ]) {
      expect(PublicRuntimeCapabilityStateV2.safeParse(value).success).toBe(false)
    }
  })

  it('requires a unique supported reasoning default and an executable protocol', () => {
    expect(PublicModelReasoningV2.safeParse({
      supportedEfforts: ['off', 'low', 'medium', 'high'],
      defaultEffort: 'high',
      requestProtocol: 'mimo-chat-completions'
    }).success).toBe(true)
    expect(PublicModelReasoningV2.safeParse({
      supportedEfforts: ['off', 'high'], defaultEffort: 'medium', requestProtocol: 'mimo-chat-completions'
    }).success).toBe(false)
    expect(PublicModelReasoningV2.safeParse({
      supportedEfforts: ['auto'], defaultEffort: 'auto', requestProtocol: 'none'
    }).success).toBe(false)
  })

  it('binds MCP search and catalog reason codes to their state', () => {
    const search = {
      enabled: true,
      mode: 'auto',
      active: false,
      available: false,
      reasonCode: 'unavailable',
      indexedToolCount: 0,
      advertisedToolCount: 0,
      autoThresholdToolCount: 0,
      topKDefault: 0,
      topKMax: 0,
      minScore: 0,
      catalogDrift: false
    }
    expect(PublicMcpSearchV2.safeParse(search).success).toBe(true)
    expect(PublicMcpSearchV2.safeParse({ ...search, active: true }).success).toBe(false)
    expect(PublicMcpSearchV2.safeParse({ ...search, reasonCode: 'available' }).success).toBe(false)

    expect(PublicMcpCatalogV2.safeParse({
      status: 'cached',
      reasonCode: 'schema_hint_only',
      toolCount: 0,
      advertisedToolCount: 0,
      promptCount: 0,
      resourceCount: 0,
      catalogDrift: false
    }).success).toBe(true)
    expect(PublicMcpCatalogV2.safeParse({
      status: 'cached',
      reasonCode: 'available',
      toolCount: 0,
      advertisedToolCount: 0,
      promptCount: 0,
      resourceCount: 0,
      catalogDrift: false
    }).success).toBe(false)
  })

  it('rejects impossible MCP server, command, skill, and subagent diagnostics', () => {
    const server = {
      id: 'funds',
      status: 'connected',
      transport: 'stdio',
      authStatus: 'none',
      trustScope: 'workspace',
      enabled: true,
      available: true,
      connected: true,
      schemaHintAvailable: true,
      connectable: true,
      toolCount: 1,
      promptCount: 0,
      resourceCount: 0,
      toolContractQuarantineCount: 0,
      sourceProbeCount: 1,
      lowPriority: false,
      backgroundStart: false
    }
    const parsed = PublicMcpServerDiagnosticV2.safeParse(server)
    expect(parsed.success).toBe(true)
    if (parsed.success) expect(parsed.data.sourceProbeCount).toBe(1)
    expect(PublicMcpServerDiagnosticV2.safeParse({ ...server, sourceProbeCount: undefined }).success).toBe(true)
    for (const sourceProbeCount of [-1, 1.5, 1_000_000_001, '1', Number.NaN]) {
      expect(PublicMcpServerDiagnosticV2.safeParse({ ...server, sourceProbeCount }).success).toBe(false)
    }
    for (const forbidden of [
      'sourceReady',
      'sourceProbeDigest',
      'sourceCaseId',
      'datasetSnapshotId',
      'rowCount',
      'projectionDigest',
      'rawResultSHA256',
      'grantDigest',
      'authorityDigest',
      'connectionEpoch',
      'sourcePath',
      'sourceValue',
      'error'
    ]) {
      expect(PublicMcpServerDiagnosticV2.safeParse({ ...server, [forbidden]: 'private-source-sentinel' }).success).toBe(false)
    }
    expect(PublicMcpServerDiagnosticV2.safeParse({ ...server, connected: false }).success).toBe(false)
    expect(PublicMcpServerDiagnosticV2.safeParse({ ...server, status: 'error' }).success).toBe(false)
    expect(PublicCommandDiagnosticV2.safeParse({ binary: 'go', found: true, status: 'unavailable' }).success).toBe(false)
    expect(PublicSkillDiagnosticsV2.safeParse({
      enabled: false,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 0,
      skillCount: 0,
      validationErrorCount: 0
    }).success).toBe(false)
    expect(PublicSubagentDiagnosticsV2.safeParse({
      status: 'available',
      enabled: true,
      available: false,
      reasonCode: 'available',
      active: 0,
      queued: 0,
      profileCount: 0,
      maxParallel: 0,
      maxChildRuns: 0,
      defaultToolPolicy: 'readOnly',
      internalLineageAvailable: false,
      parallelExecutionAvailable: false,
      taskToolAvailable: false,
      parallelTasksToolAvailable: false,
      backgroundTaskJobsAvailable: false,
      backgroundShellAvailable: false,
      backgroundSubagentJobsAvailable: false
    }).success).toBe(false)
  })
})
