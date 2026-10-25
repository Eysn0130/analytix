// Synthetic, closed public contracts only. No protected capture or provider call.
export const trajectoryStepId = 'd560256d-4adc-401c-9c0a-b5f46052d513'
export const trajectoryTime = (second: number) => `2026-10-15T00:00:${String(second).padStart(2, '0')}.000Z`
export function retryEvent(seq: number, phase = 'scheduled', source = 'provider_retry') {
  return { kind: 'pipeline_stage', seq, timestamp: trajectoryTime(seq), threadId: 'trajectory-thread', turnId: 'trajectory-turn',
    stage: 'provider_retrying', label: 'Retrying provider stream', attempt: source === 'transport_reconnect' ? 3 : 2, maxAttempt: 4,
    details: { retry: { schemaVersion: 1, source, phase, logicalSequence: 7, outerAttempt: 2,
      ...(source === 'transport_reconnect' ? { physicalAttempt: 3 } : {}),
      ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1700000000000 } : {}),
      ...(phase === 'cancelled' ? { cancelReason: 'attempt_rejected' } : {}) } } }
}
export function errorEvent(seq: number) {
  return { kind: 'error', seq, timestamp: trajectoryTime(seq), threadId: 'trajectory-thread', turnId: 'trajectory-turn',
    code: 'provider_unavailable', message: 'The model provider is temporarily unavailable.',
    details: { phase: 'live', attempt: 2, maxAttempt: 4 } }
}
export function terminalErrorBatch(withItem: boolean) {
  const timestamp = trajectoryTime(9), threadId = 'trajectory-thread', turnId = 'trajectory-turn'
  const { code, message } = errorEvent(9)
  const item = { kind: 'item_completed', seq: 9, timestamp, threadId, turnId, itemId: 'terminal-error',
    item: { id: 'terminal-error', threadId, turnId, role: 'system', kind: 'error', status: 'failed',
      createdAt: timestamp, finishedAt: timestamp, code, message, severity: 'error' } }
  const usage = { kind: 'usage', seq: 10, timestamp, threadId, turnId, model: 'fixture-model', usageFinalStatus: 'failed', cacheDiagnostics: {},
    usage: { promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
      cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null, cacheMissReasons: [], cacheSuggestions: [],
      costUsd: 0, costCny: 0, priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0, tokenEconomySavingsTokens: 0, turns: 1 } }
  const terminal = { kind: 'turn_failed', seq: 11, timestamp, threadId, turnId, status: 'failed', terminalReason: 'provider_failure',
    code, message, severity: 'error', ...(withItem ? { itemId: 'terminal-error' } : {}) }
  return { schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1', kind: 'general_terminal_batch',
    threadId, turnId, seq: 11, firstSeq: withItem ? 9 : 10, lastSeq: 11, timestamp,
    batchDigest: 'a'.repeat(64), generalTerminalCommitId: 'b'.repeat(64), generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: 'c'.repeat(64), eventManifestDigest: 'd'.repeat(64), projectedEventsDigest: 'e'.repeat(64),
    transportAuthority: 'host_batch_digest_v1', evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
    events: [...withItem ? [item] : [], usage, terminal],
    eventManifest: [...withItem ? [{ slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) }] : [],
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }] }
}
export function stepEvent(stage: 'logical_step_started' | 'logical_step_assembled' | 'logical_step_closed', seq: number, second = seq, threadId = 'trajectory-thread') {
  return { kind: 'pipeline_stage', seq, timestamp: trajectoryTime(second), threadId, turnId: 'trajectory-turn', stage,
    label: { logical_step_started: 'Logical step started', logical_step_assembled: 'Logical step assembled', logical_step_closed: 'Logical step closed' }[stage],
    details: { schemaVersion: 1, logicalStepId: trajectoryStepId, ...(stage === 'logical_step_closed' ? { assistantMessageAssembled: true, llmMs: 1000 } : {}) } }
}
export function userEvent(seq: number, threadId = 'trajectory-thread', text = `Public request ${seq}`) {
  const id = `trajectory-user-${seq}`
  return { kind: 'item_created', seq, timestamp: trajectoryTime(0), threadId, turnId: 'trajectory-turn', itemId: id,
    item: { id, threadId, turnId: 'trajectory-turn', kind: 'user_message', role: 'user', status: 'completed', text, createdAt: trajectoryTime(0) } }
}
export function readyEvent(seq: number) {
  return { kind: 'tool_call_ready', seq, timestamp: trajectoryTime(2), threadId: 'trajectory-thread', turnId: 'trajectory-turn',
    toolName: 'read_file', callId: 'trajectory-call', readyCount: 1, partial: true }
}
export function toolEvent(finished: boolean, seq: number) {
  const id = finished ? 'trajectory-result' : 'trajectory-tool'
  return { kind: finished ? 'tool_call_finished' : 'tool_call_started', seq, timestamp: trajectoryTime(finished ? 7 : 3),
    threadId: 'trajectory-thread', turnId: 'trajectory-turn', itemId: id,
    item: { id, threadId: 'trajectory-thread', turnId: 'trajectory-turn', role: 'tool', status: finished ? 'completed' : 'running',
      kind: finished ? 'tool_result' : 'tool_call', createdAt: trajectoryTime(finished ? 7 : 3),
      toolName: 'read_file', callId: 'trajectory-call', toolKind: 'tool_call',
      ...finished ? { output: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', status: 'completed',
        messageKey: 'tool_output_withheld', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }, isError: false }
        : { arguments: { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
          privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } } } }
}
