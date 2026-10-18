import { describe, expect, it } from 'vitest'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import { toolPreparationRowModel } from './tool-call-model'

const preparation = (extra: Partial<ProcessDisplayBlockV1> = {}): ProcessDisplayBlockV1 => ({
  blockId: 'block_prepare', kind: 'tool_preparation', state: 'preparing', attempt: 1,
  source: { kind: 'tool_preparation', unitId: 'unit_real', producerId: 'producer_real', providerSequence: '3', attempt: 1, callId: 'call_host', sourceOrdinal: '1' },
  toolName: 'write', path: 'real.txt', inputUtf16CodeUnits: 1025,
  durationMs: 2, truncated: false, classification: 'source_exact_local', ...extra
})

describe('protected tool preparation row model', () => {
  it('uses only the host summary and decoded size, keeping one unexpandable call identity', () => {
    expect(toolPreparationRowModel(preparation())).toEqual({
      callId: 'call_host', toolName: 'write', state: 'preparing', expandable: false,
      summary: 'real.txt', inputKilobytes: 2
    })
    expect(toolPreparationRowModel(preparation({ toolName: 'mcp__example__write', path: undefined, preview: 'Actual description', inputUtf16CodeUnits: undefined })))
      .toEqual({ callId: 'call_host', toolName: 'mcp__example__write', state: 'preparing', expandable: false, summary: 'Actual description', inputKilobytes: undefined })
    expect(toolPreparationRowModel(preparation({ state: 'withheld', toolName: undefined, path: undefined, inputUtf16CodeUnits: undefined, classification: 'withheld' }))).toBeNull()
  })
})
