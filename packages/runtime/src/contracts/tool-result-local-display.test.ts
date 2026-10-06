import { describe, expect, it } from 'vitest'
import { toolResultLocalDisplaySchemaV1, toolResultLocalViewRequestSchemaV1 } from './tool-result-local-display'
const capture = { kind: 'shell', status: 'timeout', body: 'exitCode=0\nstatus=completed\r\n', label: 'printf', exitCode: 7, durationMs: 3, truncated: false, startLine: 0, endLine: 0, totalLines: 0 }
const display = { schemaVersion: 1, snapshotDigest: 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'bash', capture }
describe('closed protected-local result contract', () => {
  it('retains exact original body and authoritative timeout/exit evidence', () => {
    expect(toolResultLocalDisplaySchemaV1.parse(display).capture).toEqual(capture)
    expect(toolResultLocalDisplaySchemaV1.parse({ ...display, capture: { ...capture, body: '' } }).capture.body).toBe('')
  })
  it('rejects private envelopes, generic outputs, overflow and malformed read coordinates', () => {
    for (const value of [
      { ...display, principal: {} }, { ...display, path: '/private/source' },
      { ...display, capture: { ...capture, output: 'raw' } },
      { ...display, capture: { ...capture, body: '中'.repeat(10923) } },
      { ...display, capture: { ...capture, body: '\ud800' } },
      { ...display, toolName: 'unknown' },
      { ...display, toolName: 'read', capture: { ...capture, kind: 'read', status: 'completed', exitCode: undefined, durationMs: 0, startLine: 4, endLine: 2 } }
    ]) expect(toolResultLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
  })
  it('wire input is identity-only and does not allow actor or text substitution', () => {
    const request = { viewId: '11111111-1111-4111-8111-111111111111', threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'c'.repeat(64), resultItemId: display.resultItemId }
    expect(toolResultLocalViewRequestSchemaV1.safeParse(request).success).toBe(true)
    for (const extra of [{ body: 'forged' }, { workspace: '/another' }, { principal: 'other' }, { threadId: '../foreign' }]) {
      expect(toolResultLocalViewRequestSchemaV1.safeParse({ ...request, ...extra }).success).toBe(false)
    }
  })
})
