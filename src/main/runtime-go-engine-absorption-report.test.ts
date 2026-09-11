import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { describe, expect, it } from 'vitest'

describe('formal Go runtime engine absorption report', () => {
  it('keeps the Kun/Reasonix audit contract as a deterministic dry-run gate', () => {
    const result = spawnSync(process.execPath, ['scripts/runtime-go-engine-absorption-report.mjs', '--dry-run', '--json'], {
      cwd: process.cwd(),
      encoding: 'utf8'
    })
    const report = JSON.parse(result.stdout) as {
      id: string
      status: string
      deterministicEvidenceOnly: boolean
      longRunningBenchmarkRequired: boolean
      auditContract: { status: string; section11Adopted: boolean; section12Adopted: boolean; missingMarkers: string[] }
      absorption: {
        legacyMatrixId: string
        legacyCodeStageReportId: string
        areas: string[]
        section11SpeedCachePlanAdopted: boolean
        section12CacheFirstGateAdopted: boolean
      }
      checks: Array<{ id: string; status: string; command?: string }>
    }

    expect(result.status).toBe(1)
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-engine-absorption-report',
      status: 'skipped',
      deterministicEvidenceOnly: true,
      longRunningBenchmarkRequired: false
    }))
    expect(report.auditContract).toEqual(expect.objectContaining({
      status: 'passed',
      section11Adopted: true,
      section12Adopted: true,
      missingMarkers: []
    }))
    expect(report.absorption).toEqual(expect.objectContaining({
      legacyMatrixId: 'd0250c-reasonix-superiority-matrix',
      legacyCodeStageReportId: 'd0250c-go-runtime-code-stage-report',
      section11SpeedCachePlanAdopted: true,
      section12CacheFirstGateAdopted: true
    }))
    expect(report.absorption.areas).toEqual(expect.arrayContaining([
      'typed streaming',
      'provider stream and tool-call parser',
      'DeepSeek cache-first request shaping',
      'history repair and tool-result pairing',
      'MCP lazy catalog and schema cache',
      'subagent task and background job runtime'
    ]))
    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'kun-reasonix-feature-delta-audit', status: 'passed' }),
      expect.objectContaining({
        id: 'default-readiness-report',
        status: 'skipped',
        command: 'npm run runtime:go:default-readiness-report -- --json'
      })
    ]))
  })

  it('does not revive deleted legacy D024/D025 executable script entrypoints', () => {
    const wrapper = readFileSync(join(process.cwd(), 'scripts/runtime-go-validation-command.mjs'), 'utf8')
    const absorption = readFileSync(join(process.cwd(), 'scripts/runtime-go-engine-absorption-report.mjs'), 'utf8')

    expect(wrapper).not.toMatch(/script:\s*'d0(?:24|25)[^']*\.mjs'/i)
    expect(absorption).not.toContain('scripts/d0250c-go-runtime-code-stage-report.mjs')
    expect(absorption).not.toContain('scripts/d0247-go-runtime-readiness-report.mjs')
  })
})
