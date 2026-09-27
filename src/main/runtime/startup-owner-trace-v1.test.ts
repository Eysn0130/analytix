import { describe, expect, it } from 'vitest'
import {
  createStartupOwnerPhaseStreamV1,
  parseBundledFundsStartupStderrV1,
  parseStartupOwnerPhaseLineV1
} from './startup-owner-trace-v1'

describe('fixed startup owner diagnostics', () => {
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
