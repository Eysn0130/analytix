import { describe, expect, it } from 'vitest'
import { PublicToolResultProjectionV1, type PublicTaskJobResultDataV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
import { taskJobSummaryV1 } from '../../../../../../../../packages/runtime/src/contracts/task-job-output'
import type { ToolResultLocalGoalDataV1, ToolResultLocalJobDataV1, ToolResultLocalSkillDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolBlock } from '../../../../../agent/types'
import { goalCardModel, protectedJobCardModel, recordedJobCardModel, recordedJobData, skillCardModel } from './recorded-tool-card-model'
const t = (key: string, options?: Record<string, unknown>) => key + (options ? JSON.stringify(options) : '')
const job = taskJobSummaryV1({ id: 'job-a', kind: 'bash', status: 'killed', background: true })
const base = { schemaVersion: 1, projectionKind: 'task_job_result', disclosure: 'metadata_only', status: 'completed',
  messageKey: 'tool_completed', code: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
const block = (data: PublicTaskJobResultDataV1): ToolBlock => ({ kind: 'tool', id: 'block', summary: '', status: 'success', toolKind: 'command_execution',
  meta: { sourceItemId: 'item_result_' + 'a'.repeat(64), callId: 'call_host_' + 'b'.repeat(64),
    toolName: { list: 'list_jobs', output: 'bash_output', kill: 'kill_shell', wait: 'wait' }[data.operation], publicResult: { ...base, taskJob: data } } })
describe('recorded Job and protected Goal/Skill models', () => {
  const privateJob: ToolResultLocalJobDataV1['jobs'][number] = { jobId: 'job-a', kind: 'bash', status: 'running', name: 'Actual name', label: 'Actual label', output: { availability: 'unavailable' } }
  it('keeps true Name/Label and status separate from list and output budgets', () => {
    const data: ToolResultLocalJobDataV1 = { operation: 'list', jobs: [privateJob], total: 4, truncated: true, receipt: null }
    const model = protectedJobCardModel(data, t)
    expect(model.summary).toBe('dshRecordedTool.job.count{"count":4}')
    expect(model.caption).toBe('dshRecordedTool.job.listTruncated{"count":1,"total":4}')
    expect(model.items[0]).toMatchObject({ title: 'Actual name', badge: { label: 'dshRecordedTool.jobStatus.running' } })
    expect(model.items[0].fields.map(field => field.value)).toEqual(['Actual name', 'Actual label', 'job-a', 'dshRecordedTool.jobKind.bash'])
    const absent = protectedJobCardModel({ ...data, jobs: [{ ...privateJob, name: null, label: null }] }, t)
    expect(absent.items[0].title).toBe('job-a')
    expect(absent.items[0].fields.slice(0, 2).map(field => field.value)).toEqual(['dshToolInspect.unavailable', 'dshToolInspect.unavailable'])
    expect(JSON.stringify(model)).not.toMatch(/background|killed|timeout/)
  })
  it.each([
    { operation: 'cancel', status: 'cancellation-requested' }, { operation: 'cancel', status: 'already-finished' },
    { operation: 'wait', status: 'completed' }, { operation: 'wait', status: 'timeout' }
  ] as const)('shows the real $operation/$status receipt without deriving job status', receipt => {
    const model = protectedJobCardModel({ operation: receipt.operation, jobs: [privateJob], total: 1, truncated: false, receipt }, t)
    expect(model.expandedSummary).toBe('dshRecordedTool.job.receipt.' + receipt.operation + '.' + receipt.status)
    expect(model.items[0].badge?.label).toBe('dshRecordedTool.jobStatus.running')
    expect(protectedJobCardModel({ operation: receipt.operation, jobs: [], total: 0, truncated: false, receipt: null }, t).expandedSummary).toBe('dshRecordedTool.job.receiptUnavailable')
  })
  it('selects each closed Job operation without changing a successful call into a failed job', () => {
    const output: PublicTaskJobResultDataV1 = { operation: 'output', result: { schemaVersion: 1, availability: 'withheld', jobId: job.id,
      status: 'failed', reasonCode: 'security_bound_child_output', outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false } }
    for (const data of [{ operation: 'list', result: { jobs: [job], count: 1 } }, { operation: 'kill', result: { job } },
      { operation: 'wait', result: { jobs: [job] } }, output] as PublicTaskJobResultDataV1[]) {
      const b = block(data)
      expect(recordedJobData(b)).toEqual(data)
      const model = recordedJobCardModel(data, t)
      expect(model.items[0].title).toBe(job.id); expect(model.items[0].description).toBe('dshRecordedTool.job.outputWithheld')
      expect(JSON.stringify(model)).not.toMatch(/timedOut|noProgress|receipt|outputText/)
    }
  })
  it('rejects count/active/variant drift, foreign producers, kinds, lifecycle and call identity', () => {
    const data: PublicTaskJobResultDataV1 = { operation: 'list', result: { jobs: [job], count: 1 } }, valid = block(data)
    for (const taskJob of [{ operation: 'list', result: { jobs: [job], count: 2 } },
      { operation: 'list', result: { jobs: [{ ...job, active: false }], count: 1 } },
      { operation: 'kill', result: { jobs: [job] } }]) {
      expect(PublicToolResultProjectionV1.safeParse({ ...base, taskJob }).success).toBe(false)
    }
    expect(recordedJobData({ ...valid, status: 'error' })).toBeNull()
    expect(recordedJobData({ ...valid, toolKind: 'tool_call' })).toBeNull()
    expect(recordedJobData({ ...valid, meta: { ...valid.meta, toolName: 'job_list' } })).toBeNull()
    expect(recordedJobData({ ...valid, meta: { ...valid.meta, callId: 'foreign' } })).toBeNull()
  })
  it('handles empty Job snapshots without claiming a wait timeout or cancel acknowledgement', () => {
    const model = recordedJobCardModel({ operation: 'wait', result: { jobs: [] } }, t)
    expect(model.items).toEqual([]); expect(model.empty).toBe('dshRecordedTool.job.empty')
    expect(JSON.stringify(model)).not.toMatch(/timeout|cancel|accepted|finished/)
  })
  it('renders native Goal status and real budget while omitting unproduced rounds', () => {
    const data: ToolResultLocalGoalDataV1 = { goal: { threadId: 'thread', objective: 'Actual objective', status: 'blocked',
      tokenBudget: 100, tokensUsed: 40, timeUsedSeconds: 12, blockedReason: 'Actual reason', createdAt: 'now', updatedAt: 'now' }, remainingTokens: 60 }
    const model = goalCardModel(data, t)
    expect(model.summary).toBe('Actual objective'); expect(model.items[0].fields.map(field => field.value)).toContain('60')
    expect(JSON.stringify(model)).toContain('Actual reason'); expect(JSON.stringify(model)).not.toMatch(/round|revision|activation/)
    expect(goalCardModel({ goal: null, remainingTokens: null }, t).items).toEqual([])
  })
  it('uses admitted Skill names and inline body; private child IDs never become actions', () => {
    const data: ToolResultLocalSkillDataV1 = { requestedName: 'Actual request', resolved: { id: 'skill-a', name: 'Actual resolved', runAs: 'inline', packageDigest: 'a'.repeat(64) }, child: null, error: null }
    expect(skillCardModel(data, 'Actual instructions', t)).toMatchObject({ summary: 'Actual resolved', instructions: 'Actual instructions' })
    const subagent: ToolResultLocalSkillDataV1 = { ...data, resolved: { ...data.resolved!, runAs: 'subagent' }, child: { jobId: 'job-a', childThreadId: null, childTurnId: null,
      status: 'completed', background: false, outputWithheld: true }, error: { code: 'failure', message: 'First line\r\nFull error' } }
    const model = skillCardModel(subagent, '', t)
    expect(model.summary).toBe('First line'); expect(model.instructions).toBeNull()
    expect(model.details.items[0].description).toContain('Full error'); expect(JSON.stringify(model)).not.toMatch(/capability|navigate|childThreadId/)
  })
})
