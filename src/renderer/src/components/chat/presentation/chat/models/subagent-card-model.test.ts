import { describe, expect, it } from 'vitest'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { RuntimeChildMetadata, ToolBlock } from '../../../../../agent/types'
import { nativeSubagentCardModel, subagentCardModel, subagentNativeIdentity, subagentRowState } from './subagent-card-model'

const t = (key: string, options?: Record<string, unknown>) => typeof options?.defaultValue === 'string' ? options.defaultValue : key
const block: ToolBlock = { kind: 'tool', id: 'result', toolKind: 'subagent', status: 'success', summary: 'Public summary', meta: { toolName: 'task' } }
const child: RuntimeChildMetadata = { childId: 'child-a', jobId: 'job-a', parentThreadId: 'thread', parentTurnId: 'turn', childStatus: 'running' }
const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, resultItemId: 'item_result_' + 'a'.repeat(64), snapshotDigest: 'b'.repeat(64), toolName: 'task', canNavigate: false,
  capture: { kind: 'job', status: 'completed', label: '', body: '', startLine: 0, endLine: 0, totalLines: 0, durationMs: 0, truncated: false,
    job: { operation: 'spawn', jobs: [{ jobId: 'job-a', kind: 'task', status: 'running', name: 'Private name', label: 'Private label', output: { availability: 'unavailable' } }],
      total: 1, truncated: false, receipt: null } },
  inspect: { inputJSON: '{"prompt":"Private prompt"}', inputTruncated: false, schemaTruncated: true, callEventAt: null, resultEventAt: null, durationMs: null } }
describe('native DSH subagent observations', () => {
  it('adapts the actual spawn record to Started without treating unavailable output as Reply', () => {
    const model = subagentCardModel(block, display, t)!
    expect(model.items[0].title).toBe('Private prompt')
    expect(model.items[0].badge).toEqual({ label: 'Started', tone: 'info' })
    expect(model.items[0].fields).toContainEqual({ label: 'dshRecordedTool.job.id', value: 'job-a' })
    expect(model.items[0].description).toBe('dshRecordedTool.job.outputUnavailable')
    expect(JSON.stringify(model)).not.toContain('Reply')
  })
  it('keeps child running after a completed call and preserves outer failure and stop', () => {
    expect(subagentRowState(block, child)).toBe('running')
    expect(subagentRowState({ ...block, status: 'running' }, { ...child, childStatus: 'completed' })).toBe('running')
    expect(subagentRowState(block, { ...child, childStatus: 'paused' })).toBe('ok')
    expect(nativeSubagentCardModel(child, t).items[0].badge?.label).toBe('dshRecordedTool.jobStatus.running')
    expect(subagentRowState({ ...block, status: 'error' }, { ...child, childStatus: 'completed' })).toBe('error')
    expect(subagentRowState({ ...block, status: 'error', meta: { ...block.meta, diagnostics: { status: 'canceled' } } }, { ...child, childStatus: 'completed' })).toBe('stopped')
    expect(subagentCardModel({ ...block, status: 'error' }, display, t)?.expandedSummary).toBeUndefined()
    expect(subagentCardModel({ ...block, status: 'error' }, display, t)?.items[0].badge?.tone).toBe('error')
  })
  it('does not derive receipts or synthetic children from missing records or legacy payloads', () => {
    if (display.capture.kind !== 'job') throw new Error('fixture')
    const noRecords: ToolResultLocalDisplayV1 = { ...display, capture: { ...display.capture, kind: 'job', job: { operation: 'spawn', jobs: [], total: 3, truncated: true, receipt: null } } }
    const model = subagentCardModel({ ...block, detail: '{"tasks":[{},{}]}' }, noRecords, t)!
    expect(model.items).toEqual([]); expect(model.expandedSummary).toBeUndefined()
    expect(model.caption).toBe('dshRecordedTool.job.listTruncated')
    expect(nativeSubagentCardModel(null, t).items).toEqual([])
  })
  it('does not pair parallel prompts with returned jobs by index or equal name', () => {
    if (display.capture.kind !== 'job') throw new Error('fixture')
    const record = display.capture.job.jobs[0]
    const parallel: ToolResultLocalDisplayV1 = { ...display, toolName: 'parallel_tasks', inspect: { ...display.inspect!, inputJSON: '{"tasks":[{"prompt":"Prompt B"},{"prompt":"Prompt A"}]}' },
      capture: { ...display.capture, job: { ...display.capture.job, total: 2, jobs: ['job-b', 'job-a'].map(jobId => ({ ...record, jobId, kind: 'parallel_task' as const, name: 'same', label: null })) } } }
    const model = subagentCardModel({ ...block, meta: { toolName: 'parallel_tasks' } }, parallel, t)!
    expect(model.items.map(item => item.id)).toEqual(['job-b', 'job-a'])
    expect(model.items.map(item => item.title)).toEqual(['same', 'same'])
    expect(JSON.stringify(model)).not.toMatch(/Prompt [AB]/)
  })
  it('requires the exact producer, native kind and spawn operation', () => {
    expect(subagentCardModel({ ...block, meta: { toolName: 'custom_task' } }, display, t)).toBeNull()
    expect(subagentCardModel({ ...block, toolKind: 'tool_call' }, display, t)).toBeNull()
    expect(subagentCardModel(block, { ...display, toolName: 'delegate_task' }, t)).toBeNull()
    if (display.capture.kind !== 'job') throw new Error('fixture')
    expect(subagentCardModel(block, { ...display, capture: { ...display.capture, job: { ...display.capture.job, operation: 'list' } } }, t)).toBeNull()
  })
  it('keeps native control identity stable through public lifecycle changes', () => {
    expect(subagentNativeIdentity(child)).toBe(subagentNativeIdentity({ ...child, childStatus: 'completed', childLabel: 'new title' }))
    expect(subagentNativeIdentity(child)).not.toBe(subagentNativeIdentity({ ...child, jobId: 'job-b' }))
    expect(subagentNativeIdentity({ ...child, childRunId: 'run-a' })).not.toBe(subagentNativeIdentity({ ...child, childRunId: 'run-a', jobId: 'job-b' }))
  })
})
