import { describe, expect, it } from 'vitest'
import {
  createStartupOwnerPhaseStreamV1,
  parseBundledFundsStartupStderrV1,
  parseStartupOwnerPhaseLineV1
} from './startup-owner-trace-v1'

describe('fixed startup owner diagnostics', () => {
  it('forwards closed Prepare failures through the existing runtime stderr stream', () => {
    const phases: unknown[] = []
    const stream = createStartupOwnerPhaseStreamV1((phase) => phases.push(phase))
    stream.accept('ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_runtime_')
    stream.accept('components_unknown 12\n')
    stream.accept('ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_/private 12\n')
    stream.accept('ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_unknown 12 extra\n')
    expect(phases).toEqual([{ stage: 'go_prepare_failed_runtime_components_unknown', durationMs: 12 }])
    expect(parseStartupOwnerPhaseLineV1(
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_unknown 12'
    )).toEqual({ stage: 'go_prepare_failed_unknown', durationMs: 12 })
    expect(parseBundledFundsStartupStderrV1(Buffer.from(
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 go_prepare_failed_unknown 12\n'
    ))).toBeNull()
  })
  it('accepts only bounded, exact phase and duration lines', () => {
    expect(parseStartupOwnerPhaseLineV1(
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected 24'
    )).toEqual({ stage: 'funds_package_inspected', durationMs: 24 })
    for (const line of [
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected 24 /private/path',
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected -1',
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected 1800001',
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 unknown_phase 24',
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected 024'
    ]) expect(parseStartupOwnerPhaseLineV1(line)).toBeNull()
  })

  it('keeps materialization stderr strict even when tracing is enabled', () => {
    const marker = 'ANALYTIX_STARTUP_OWNER_PHASE_V1 funds_package_inspected 24\n'
    expect(parseBundledFundsStartupStderrV1(Buffer.from(marker)))
      .toEqual([{ stage: 'funds_package_inspected', durationMs: 24 }])
    expect(parseBundledFundsStartupStderrV1(Buffer.alloc(0))).toEqual([])
    for (const value of [
      marker.slice(0, -1),
      `${marker}private command detail\n`,
      'ANALYTIX_STARTUP_OWNER_PHASE_V1 go_lease_acquired 24\n',
      `${marker.repeat(17)}`
    ]) expect(parseBundledFundsStartupStderrV1(Buffer.from(value))).toBeNull()
  })

  it('recognizes a runtime phase split across chunks without projecting other stderr', () => {
    const phases: unknown[] = []
    const stream = createStartupOwnerPhaseStreamV1((phase) => phases.push(phase))
    stream.accept(Buffer.from('private command detail\nANALYTIX_STARTUP_OWNER_'))
    stream.accept(Buffer.from('PHASE_V1 go_semantic_prepared 31000\n'))
    stream.accept(Buffer.from('ANALYTIX_STARTUP_OWNER_PHASE_V1 unknown_phase 32\n'))
    expect(phases).toEqual([{ stage: 'go_semantic_prepared', durationMs: 31000 }])
  })
})

describe('persistence recovery startup owner diagnostics', () => {
  it.each([
    'go_prepare_failed_persistence_authority_advance_unknown',
    'go_prepare_failed_persistence_report_preservation_unknown',
    'go_prepare_failed_persistence_final_history_bind_unknown',
    'go_prepare_failed_persistence_retained_original_unknown',
    'go_prepare_failed_persistence_journal_load_unknown',
    'go_prepare_failed_persistence_child_revalidate_unknown',
    'go_prepare_failed_persistence_authority_revalidate_unknown',
    'go_prepare_failed_persistence_signed_recovery_validation_unknown',
    'go_prepare_failed_persistence_create_residues_unknown',
    'go_prepare_failed_persistence_semantic_recovery_unknown',
    'go_prepare_failed_persistence_orphan_topology_unknown',
    'go_prepare_failed_persistence_owner_recovery_unknown',
    'go_prepare_failed_persistence_checkpoint_recovery_unknown'
  ])('accepts the fixed phase and split runtime stream for %s', (stage) => {
    const line = `ANALYTIX_STARTUP_OWNER_PHASE_V1 ${stage} 12`
    expect(parseStartupOwnerPhaseLineV1(line)).toEqual({ stage, durationMs: 12 })
    const phases: unknown[] = []
    const stream = createStartupOwnerPhaseStreamV1((phase) => phases.push(phase))
    stream.accept(Buffer.from(line.slice(0, 20)))
    expect(phases).toEqual([])
    stream.accept(Buffer.from(line.slice(20)))
    expect(phases).toEqual([])
    stream.accept(Buffer.from('\n'))
    expect(phases).toEqual([{ stage, durationMs: 12 }])
    expect(parseBundledFundsStartupStderrV1(Buffer.from(`${line}\n`))).toBeNull()
  })

  it('rejects unlisted, decorated and unbounded persistence phase lines', () => {
    const prefix = 'ANALYTIX_STARTUP_OWNER_PHASE_V1 '
    const stage = 'go_prepare_failed_persistence_journal_load_unknown'
    const line = `${prefix}${stage} 12`
    const phases: unknown[] = []
    const stream = createStartupOwnerPhaseStreamV1((phase) => phases.push(phase))
    for (const invalid of [
      `${prefix}go_prepare_failed_persistence_unknown 12`,
      `${prefix}go_prepare_failed_persistence_journal_load_other 12`,
      `${prefix}persistence_journal_load_unknown 12`,
      `${prefix}${stage}/private/path 12`,
      `${line} {"body":"private response"}`,
      `${prefix} ${stage} 12`,
      `${prefix}${stage}  12`,
      `${prefix}${stage} -1`,
      `${prefix}${stage} 1800001`,
      `${prefix}${stage} 012`,
      line.padEnd(129, ' ')
    ]) {
      expect(parseStartupOwnerPhaseLineV1(invalid)).toBeNull()
      stream.accept(`${invalid}\n`)
    }
    expect(phases).toEqual([])
    expect(parseBundledFundsStartupStderrV1(Buffer.from(`${line}\n`))).toBeNull()
  })
})
