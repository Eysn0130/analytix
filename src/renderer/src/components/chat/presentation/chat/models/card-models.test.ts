import { describe, expect, it } from 'vitest'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { PublicToolResultProjectionV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ToolBlock } from '../../../../../agent/types'
import { readCardModel } from './read-card-model'
import { terminalCardModel, terminalFailed } from './terminal-card-model'
import { diffCardModel } from './diff-card-model'
import { detailsCardModel } from './details-card-model'
import { toolRowState, safeWebHref } from './tool-call-model'
const shellInspect = (inputJSON?: string, inputTruncated = false) => ({ inputJSON, inputTruncated, schemaTruncated: false,
  callEventAt: null, resultEventAt: null, durationMs: null })
const display = (kind: 'read' | 'shell' = 'read'): ToolResultLocalDisplayV1 => ({ schemaVersion: 1, snapshotDigest: 'a'.repeat(64),
  resultItemId: 'item_result_' + 'b'.repeat(64), toolName: kind === 'shell' ? 'bash' : 'read',
  capture: { kind, status: 'completed', label: '/synthetic/read.ts', body: 'first\nsecond\n',
    startLine: kind === 'read' ? 5 : 0, endLine: kind === 'read' ? 6 : 0, totalLines: kind === 'read' ? 20 : 0,
    durationMs: 0, truncated: false }, ...(kind === 'shell' ? { inspect: shellInspect(JSON.stringify({ command: 'actual shell command' })) } : {}) })
const mutation = (): ProcessDisplayBlockV1 => ({ blockId: 'block-a', kind: 'edit', state: 'completed', attempt: 1,
  source: { kind: 'tool', unitId: 'unit-a', callId: 'call-a', toolCallItemId: 'item-call-a', toolResultItemId: 'item-result-a' },
  captureDigest: 'a'.repeat(64), path: '/synthetic/file.ts', effectApplied: true, settlementStatus: 'completed',
  durationMs: 1, truncated: false, classification: 'credential_redacted',
  body: { text: '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old\n+new\n',
    before: 'NOT_THE_HOST_DIFF_BEFORE', after: 'NOT_THE_HOST_DIFF_AFTER', error: '', addedLines: 91, removedLines: 83, approximate: true } })
const block = (meta: ToolBlock['meta'] = {}): ToolBlock => ({ kind: 'tool', id: 'tool-a', status: 'success', summary: 'tool', meta })
const publicBase = { schemaVersion: 1, disclosure: 'metadata_only', status: 'completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
const t = (key: string) => key

describe('Host typed card adapters (DSH model semantics, renderer authority)', () => {
  it('retains actual Read coordinates, total lines, grammar and truncation without a terminator row', () => {
    const d = display(); d.capture.truncated = true
    expect(readCardModel(d)).toEqual({ label: '/synthetic/read.ts', lines: [{ number: 5, text: 'first' }, { number: 6, text: 'second' }],
      totalLines: 20, lang: 'ts', truncated: true, filePathLine: 5 })
  })
  it('accepts empty files and preserves real interior blank lines', () => {
    const d = display(); Object.assign(d.capture, { body: '', startLine: 0, endLine: 0, totalLines: 0 })
    expect(readCardModel(d)?.lines).toEqual([])
    Object.assign(d.capture, { body: 'a\n\n', startLine: 1, endLine: 2, totalLines: 2 })
    expect(readCardModel(d)?.lines).toEqual([{ number: 1, text: 'a' }, { number: 2, text: '' }])
  })
  it.each([{ startLine: 0 }, { totalLines: 5 }, { endLine: 5 }])('rejects invalid Read coordinates %j instead of making line numbers', patch => {
    const d = display(); Object.assign(d.capture, patch); expect(readCardModel(d)).toBeNull()
  })
  it('uses Host Shell outcome without exit markers, and preserves nonzero exit even on a completed call', () => {
    const d = display('shell')
    if (d.capture.kind !== 'shell') throw new Error('fixture requires Shell')
    Object.assign(d.capture, { status: 'unknown', body: 'exit code: 0; success' })
    expect(terminalCardModel(d)).toMatchObject({ status: 'unknown', output: 'exit code: 0; success', exitCode: undefined })
    Object.assign(d.capture, { status: 'completed', exitCode: 7 }); expect(terminalFailed(terminalCardModel(d)!)).toBe(true)
    delete d.capture.exitCode; expect(terminalCardModel(d)?.exitCode).toBeUndefined()
  })
  it('preserves the full original Shell command independently of its label and bounded output', () => {
    const d = display('shell'), command = "  printf '%s\\n' actual\ncat <<'EOF'\n\t" + '長'.repeat(600) + '\nEOF\n'
    d.inspect = shellInspect(JSON.stringify({ command, description: 'PUBLIC_DESCRIPTION_ONLY' }))
    Object.assign(d.capture, { label: 'PUBLIC_LABEL_ONLY', truncated: true })
    expect(terminalCardModel(d)).toMatchObject({ command, output: d.capture.body, truncated: true })
    expect(terminalCardModel(d)?.command).not.toBe(d.capture.label)
  })
  it.each([
    ['missing input', undefined], ['incomplete JSON', '{"command":"partial"'],
    ['multiple roots', '{"command":"first"} {"command":"second"}'], ['null root', 'null'],
    ['array root', '[{"command":"printf"}]'], ['string root', '"printf"'],
    ['missing command', '{}'], ['empty command', '{"command":""}'], ['null command', '{"command":null}'],
    ['numeric command', '{"command":17}'], ['array command', '{"command":["printf"]}'],
    ['object command', '{"command":{"value":"printf"}}'], ['PTC program', '{"code":"tools.read()"}']
  ])('rejects %s without recovering a command from the label or output', (_name, inputJSON) => {
    const d = display('shell'); d.inspect = shellInspect(inputJSON)
    Object.assign(d.capture, { label: 'SUMMARY_IS_NOT_COMMAND', body: 'command=FAKE_OUTPUT_COMMAND\n' })
    expect(terminalCardModel(d)).toBeNull()
  })
  it('rejects missing or truncated original input while preserving the generic result path', () => {
    const d = display('shell'); delete d.inspect
    expect(terminalCardModel(d)).toBeNull()
    d.inspect = shellInspect(JSON.stringify({ command: 'partial command' }), true)
    expect(terminalCardModel(d)).toBeNull()
  })
  it.each(['failed', 'timeout', 'canceled'] as const)('keeps Host %s on the existing error fallback', status => {
    const d = display('shell'); Object.assign(d.capture, { status, body: 'status=completed\nexit code: 0\n' })
    expect(terminalCardModel(d)).toBeNull()
  })
  it('does not treat a Read capture with a command argument as Shell', () => {
    const d = display(); d.inspect = shellInspect(JSON.stringify({ command: 'printf actual' }))
    expect(terminalCardModel(d)).toBeNull()
  })
  it('draws Host patch and retains Host counts without rediffing separately capped before/after', () => {
    const m = diffCardModel(mutation())!
    expect(m.rows).toEqual([{ kind: 'path', text: '/synthetic/file.ts' }, { kind: 'del', text: 'old' }, { kind: 'add', text: 'new' }])
    expect(m.totals).toEqual({ added: 91, removed: 83 }); expect(m.approximate).toBe(true)
    expect(JSON.stringify(m)).not.toContain('NOT_THE_HOST_DIFF')
  })
  it('preserves actual hunk content that resembles patch file headers', () => {
    const m = mutation(); m.body!.text = '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n--- old\n+++ new\n'
    expect(diffCardModel(m)?.rows.slice(1)).toEqual([{ kind: 'del', text: '-- old' }, { kind: 'add', text: '++ new' }])
  })
  it.each([false, null, undefined])('does not make an applied diff when effectApplied is %s', effectApplied => {
    expect(diffCardModel({ ...mutation(), effectApplied })).toBeNull()
  })
  it('keeps an actual applied patch when settlement failed and keeps it truncated', () => {
    const m = { ...mutation(), state: 'failed' as const, settlementStatus: 'failed' as const, truncated: true }
    m.body!.text = '--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old\n+part'
    expect(diffCardModel(m)?.rows.at(-1)).toEqual({ kind: 'add', text: 'part' })
  })
  it('does not fabricate an intended replacement on an identical overwrite or malformed patch', () => {
    const m = mutation(); m.body!.text = ''; expect(diffCardModel(m)).toBeNull()
    m.body!.text = 'Success! +new'; expect(diffCardModel(m)).toBeNull()
    expect(diffCardModel({ ...mutation(), classification: 'withheld' })).toBeNull()
  })
  it.each([
    { projectionKind: 'host_status', messageKey: 'tool_completed', code: 'tool_completed' },
    { projectionKind: 'withheld', messageKey: 'tool_output_withheld', code: 'tool_output_private' },
    { projectionKind: 'case_source_status', messageKey: 'case_source_private', code: 'case_source_result_private' },
    { projectionKind: 'plan_status', messageKey: 'plan_updated', code: 'plan_updated', plan: { planId: 'plan-a', relativePath: 'plan.md', operation: 'draft', contentHash: 'a'.repeat(64), byteSize: 1025, savedAt: '2026-10-07T00:00:00Z' } },
    { projectionKind: 'artifact_status', messageKey: 'artifact_created', code: 'artifact_created', artifact: { artifactId: 'a'.repeat(64), kind: 'docx', contentHash: 'b'.repeat(64), byteSize: 120, savedAt: '2026-10-07T00:00:00Z' } },
    { projectionKind: 'mcp_diagnostic', messageKey: 'mcp_request_rejected', code: 'mcp_request_rejected', rpcError: { code: -32601, class: 'method_not_found', dataPresent: false } }
  ])('adapts real public $projectionKind fields with no raw-result parser', result => {
    const failed = result.projectionKind === 'mcp_diagnostic'
    const b = { ...block({ publicResult: { ...publicBase, ...result, status: failed ? 'failed' : 'completed' } as PublicToolResultProjectionV1 }), status: failed ? 'error' as const : 'success' as const }
    const model = detailsCardModel(b, t)!
    expect(model).not.toBeNull(); expect(model.items[0].fields.map(f => f.value)).toContain(result.messageKey)
    if ('plan' in result) expect(model.items[0].fields.map(f => f.value)).toContain('1025')
    if ('artifact' in result) expect(model.items[0].fields.map(f => f.value)).toContain('docx')
  })
  it('rejects hostile extra result fields and ignores DSH-like JSON in public detail', () => {
    const b = block({ publicResult: { ...publicBase, projectionKind: 'host_status', messageKey: 'tool_completed', rawbody: 'PRIVATE' } as never })
    expect(detailsCardModel(b, t)).toBeNull()
    expect(detailsCardModel({ ...block(), detail: '{"goal":{"objective":"unpublished"}}' }, t)).toBeNull()
  })
  it('respects cancelled and unknown outcomes and real running state without global processing', () => {
    expect(toolRowState({ ...block(), status: 'running' })).toBe('running')
    const result = { ...publicBase, projectionKind: 'host_status', status: 'cancelled', messageKey: 'tool_cancelled', code: 'cancelled' }
    expect(toolRowState(block({ publicResult: result as never }))).toBe('stopped')
    Object.assign(result, { status: 'unknown', messageKey: 'tool_outcome_unknown', code: 'tool_outcome_unknown_after_restart' })
    expect(toolRowState(block({ publicResult: result as never }))).toBe('unknown')
    expect(toolRowState({ ...block({ publicResult: { ...publicBase, projectionKind: 'host_status', messageKey: 'tool_completed' } as never }), status: 'error' })).toBe('error')
  })
  it.each(['javascript:alert(1)', 'file:///private/file', 'data:text/plain,x', 'not a URL'])('does not create a URL action for %s', url => {
    expect(safeWebHref(url)).toBeUndefined()
  })
})
