import { resolveAnalytixRuntimeSettings, type AppSettingsV1 } from '../../shared/app-settings'
import { redactSecretText } from '../../shared/secret-redaction'
import {
  containsPrivateReasoningContent,
  PublicRuntimeEventFilter,
  sanitizePublicSerializedText
} from '../../shared/public-runtime-content'
import {
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock
} from '../../shared/public-runtime-sse'
import { isAnalytixHealthResponseBody } from '../analytix-health'
import { RuntimeInfoResponse as RuntimeInfoResponseSchema } from '../../../packages/runtime/src/contracts/runtime-info.js'
import { RuntimeToolsResponse as RuntimeToolsResponseSchema } from '../../../packages/runtime/src/contracts/runtime-tools.js'
import type {
  AnalytixRuntimeGoBackendId,
  GoConformanceRuntimeCanaryResult,
  GoRuntimeG6ReadinessStatus
} from './analytix-adapter'

async function fetchTextWithTimeout(
  url: string,
  headers?: Headers,
  init?: Omit<RequestInit, 'headers' | 'signal'>
): Promise<{ status: number; ok: boolean; text: string }> {
  const res = await fetch(url, {
    ...init,
    headers,
    signal: AbortSignal.timeout(2_000)
  })
  return { status: res.status, ok: res.ok, text: await res.text() }
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function runtimeCapabilityDiagnosticValid(value: unknown): boolean {
  const capability = recordValue(value)
  const available = capability.available
  const enabled = capability.enabled
  const reasonCode = capability.reasonCode
  if (typeof available !== 'boolean') return false
  if (enabled !== undefined && typeof enabled !== 'boolean') return false
  if (available === true) return reasonCode === 'available'
  return reasonCode === 'disabled_by_config' || reasonCode === 'unavailable'
}

function parseJSONRecord(text: string): Record<string, unknown> {
  return recordValue(JSON.parse(text) as unknown)
}

type StrictRuntimeSseReplay =
  | { ok: true; payloads: Record<string, unknown>[] }
  | { ok: false; reason: string }

function strictRuntimeSseReplay(text: string, threadId: string): StrictRuntimeSseReplay {
  const filter = new PublicRuntimeEventFilter()
  const payloads: Record<string, unknown>[] = []
  let buffer = text
  let previousSeq = 0
  while (true) {
    const next = takePublicRuntimeSseBlock(buffer)
    if (next === null) break
    buffer = next.rest
    const decision = projectPublicRuntimeSseBlock(next.block, threadId, filter)
    if (decision === null) continue
    if (decision.status !== 'emit') {
      const eventName = /^event: ([a-z][a-z0-9_]*)$/m.exec(next.block)?.[1] ?? 'unknown'
      const reason = decision.status === 'invalid' ? decision.reason : decision.status
      return { ok: false, reason: `${reason}:${eventName}:${payloads.length + 1}` }
    }
    if (decision.event.kind === 'accepted_final_batch' ||
        decision.event.kind === 'general_terminal_batch') {
      const firstSeq = decision.event.firstSeq
      const lastSeq = decision.event.lastSeq
      if (typeof firstSeq !== 'number' || typeof lastSeq !== 'number' ||
          firstSeq !== previousSeq + 1 || lastSeq !== decision.seq) {
        return { ok: false, reason: 'non_contiguous_sequence' }
      }
    } else if (decision.seq !== previousSeq + 1) {
      return { ok: false, reason: 'non_contiguous_sequence' }
    }
    previousSeq = decision.seq
    payloads.push(decision.event)
  }
  if (buffer.trim() !== '') return { ok: false, reason: 'truncated_frame' }
  return { ok: true, payloads }
}

function runtimeJSONHeaders(headers: Headers): Headers {
  const next = new Headers(headers)
  next.set('Content-Type', 'application/json')
  next.set('Accept', 'application/json')
  return next
}

function goRuntimeG6ReadinessPassed(status: GoRuntimeG6ReadinessStatus): boolean {
  return status.ready === true &&
    status.explicitReadyGate === true &&
    status.defaultGoBackendEnabled === true &&
    status.rendererVisibleGoSwitcher === false &&
    status.durableRestartEvidence.status === 'passed' &&
    status.providerMatrix.status === 'passed' &&
    status.mcpMatrix.status === 'passed' &&
    status.packagedQa.status === 'passed' &&
    status.operatorGate.status === 'passed' &&
    status.missingRequiredChecks.length === 0
}

function goRuntimeProductDefaultReady(runtimeInfoBody: Record<string, unknown>): { ok: boolean, missing: string[] } {
  const serializedInfo = JSON.stringify(runtimeInfoBody)
  const capabilities = recordValue(runtimeInfoBody.capabilities)
  const mcp = recordValue(capabilities.mcp)
  const subagents = recordValue(capabilities.subagents)
  const missing: string[] = []
  if (capabilities.upstreamAbsorption !== undefined) missing.push('capabilities.upstreamAbsorption:hidden')
  const legacyMcpLocalMarker = ['mcp', 'Local', 'Pr', 'oof'].join('')
  if (serializedInfo.includes(legacyMcpLocalMarker)) missing.push(`${legacyMcpLocalMarker}:hidden`)
  if (serializedInfo.includes('reasonix-capability-audit')) missing.push('reasonix-capability-audit:hidden')
  if (serializedInfo.includes('Reasonix')) missing.push('Reasonix:hidden')
  if (runtimeInfoBody.schemaVersion !== 2) missing.push('schemaVersion')
  if (runtimeInfoBody.listenerScope !== 'loopback') missing.push('listenerScope')
  if (typeof capabilities.contractVersion !== 'number') missing.push('capabilities.contractVersion')
  if (typeof runtimeInfoBody.startedAt !== 'string') missing.push('startedAt')
  const storage = recordValue(runtimeInfoBody.storage)
  if (storage.configured !== true || storage.available !== true) missing.push('storage')
  if (mcp.available !== true && !runtimeCapabilityDiagnosticValid(mcp)) missing.push('capabilities.mcp.diagnostic')
  if (subagents.available !== true && !runtimeCapabilityDiagnosticValid(subagents)) missing.push('capabilities.subagents.diagnostic')
  return { ok: missing.length === 0, missing }
}

async function fetchRuntimeJSON(
  baseUrl: string,
  path: string,
  headers: Headers,
  init?: Omit<RequestInit, 'headers' | 'signal'>
): Promise<{ status: number; ok: boolean; text: string; body: Record<string, unknown> }> {
  const response = await fetchTextWithTimeout(
    `${baseUrl}${path}`,
    init?.body ? runtimeJSONHeaders(headers) : headers,
    init
  )
  return {
    ...response,
    body: response.text.trim() ? parseJSONRecord(response.text) : {}
  }
}

function redactedExcerpt(text: string, maxLength: number): string {
  return sanitizePublicSerializedText(redactSecretText(text)).slice(0, maxLength)
}

function redactedErrorMessage(error: unknown): string {
  return sanitizePublicSerializedText(redactSecretText(error instanceof Error ? error.message : String(error)))
}

export async function probeGoRuntimeCanary(
  settings: AppSettingsV1,
  baseUrl: string,
  backend: AnalytixRuntimeGoBackendId,
  g6Readiness: GoRuntimeG6ReadinessStatus,
  headers: Headers
): Promise<GoConformanceRuntimeCanaryResult> {
  const checks: string[] = []

  try {
    const health = await fetchTextWithTimeout(`${baseUrl}/health`)
    if (!health.ok || !isAnalytixHealthResponseBody(health.text)) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'health',
        message: `expected analytix health response, got ${health.status}: ${redactedExcerpt(health.text, 200)}`
      }
    }
    checks.push('health')

    const threads = await fetchTextWithTimeout(`${baseUrl}/v1/threads?limit=1`, headers)
    const threadsBody = threads.ok ? parseJSONRecord(threads.text) : {}
    if (!threads.ok || !Array.isArray(threadsBody.threads)) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'thread-api',
        message: `expected thread list response, got ${threads.status}: ${redactedExcerpt(threads.text, 200)}`
      }
    }
    checks.push('thread-api')

    if (backend === 'go-runtime-candidate' || backend === 'go-runtime-default') {
      const runtimeSettings = resolveAnalytixRuntimeSettings(settings)
      const canaryModel = runtimeSettings.model.trim() || 'deepseek-chat'
      const canaryProviderId = runtimeSettings.providerId.trim()
      const canaryModelRequest = {
        model: canaryModel,
        ...(canaryProviderId ? { providerId: canaryProviderId } : {})
      }
      const runtimeInfo = await fetchRuntimeJSON(baseUrl, '/v1/runtime/info', headers)
      const parsedRuntimeInfo = RuntimeInfoResponseSchema.safeParse(runtimeInfo.body)
      const capabilities = parsedRuntimeInfo.success
        ? recordValue(parsedRuntimeInfo.data.capabilities)
        : {}
      const infoCheckId = backend === 'go-runtime-default' ? 'go-runtime-default-info' : 'go-runtime-candidate-info'
      if (!runtimeInfo.ok ||
        !parsedRuntimeInfo.success ||
        parsedRuntimeInfo.data.listenerScope !== 'loopback' ||
        !runtimeCapabilityDiagnosticValid(capabilities.mcp) ||
        !runtimeCapabilityDiagnosticValid(capabilities.subagents)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: infoCheckId,
          message: `expected Go runtime public diagnostics v2 contract, got status ${runtimeInfo.status}`
        }
      }
      checks.push(infoCheckId)
      const runtimeTools = await fetchRuntimeJSON(baseUrl, '/v1/runtime/tools', headers)
      const parsedRuntimeTools = RuntimeToolsResponseSchema.safeParse(runtimeTools.body)
      const toolsCheckId = backend === 'go-runtime-default' ? 'go-runtime-default-tools' : 'go-runtime-candidate-tools'
      if (!runtimeTools.ok || !parsedRuntimeTools.success) {
        return {
          ok: false,
          baseUrl,
          failedCheck: toolsCheckId,
          message: `expected Go runtime public tool diagnostics v2 contract, got status ${runtimeTools.status}`
        }
      }
      checks.push(toolsCheckId)
      if (backend === 'go-runtime-default') {
        const productDefault = goRuntimeProductDefaultReady(recordValue(parsedRuntimeInfo.data))
        if (!productDefault.ok) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-default-product-readiness',
            message: `expected product-clean Go default readiness, missing ${productDefault.missing.join(', ')}`
          }
        }
        checks.push('go-runtime-default-product-readiness', 'post-cutover-live-validation-pending')
      } else if (!goRuntimeG6ReadinessPassed(g6Readiness)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-g6-readiness',
          message: `expected formal Go runtime readiness evidence before accepting Go runtime-candidate, got ${redactedExcerpt(JSON.stringify(g6Readiness), 600)}`
        }
      } else {
        checks.push(
          'go-runtime-candidate-g6-readiness',
          'durable-restart-evidence',
          'provider-matrix-status',
          'mcp-matrix-status',
          'packaged-qa-status'
        )
      }

      const runtimeCanaryPrefix = backend === 'go-runtime-default' ? 'go-runtime-default' : 'go-runtime-candidate'
      const canaryTitle = backend === 'go-runtime-default' ? 'Go Runtime Default Canary' : 'Go Runtime Candidate Canary'
      const canaryThread = await fetchRuntimeJSON(
        baseUrl,
        '/v1/threads',
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            title: canaryTitle,
            workspace: '/tmp/analytix-go-runtime-candidate',
            ...canaryModelRequest,
            mode: 'agent'
          })
        }
      )
      const threadId = typeof canaryThread.body.id === 'string' ? canaryThread.body.id : ''
      if (!canaryThread.ok || canaryThread.status !== 201 || !threadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-thread-create`,
          message: `expected isolated canary thread creation, got ${canaryThread.status}: ${redactedExcerpt(canaryThread.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-thread-create`)
      const thread = await fetchRuntimeJSON(baseUrl, `/v1/threads/${encodeURIComponent(threadId)}`, headers)
      if (!thread.ok || thread.body.id !== threadId || typeof thread.body.latestSeq !== 'number') {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-thread-read`,
          message: `expected thread read contract, got ${thread.status}: ${redactedExcerpt(thread.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-thread-read`)

      const fork = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}/fork`,
        headers,
        { method: 'POST', body: JSON.stringify({ relation: 'side', title: `${canaryTitle} Side` }) }
      )
      const forkThreadId = typeof fork.body.id === 'string' ? fork.body.id : ''
      if (!fork.ok || fork.status !== 201 || fork.body.relation !== 'side' || fork.body.parentThreadId !== threadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-fork`,
          message: `expected fork contract, got ${fork.status}: ${redactedExcerpt(fork.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-fork`)

      const resume = await fetchRuntimeJSON(
        baseUrl,
        `/v1/sessions/${encodeURIComponent(threadId)}/resume-thread`,
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            workspace: '/tmp/analytix-go-runtime-candidate',
            ...canaryModelRequest,
            mode: 'agent'
          })
        }
      )
      const resumedThreadId = typeof resume.body.thread_id === 'string' ? resume.body.thread_id : ''
      if (!resume.ok || resume.status !== 201 || resume.body.session_id !== threadId || !resumedThreadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-resume`,
          message: `expected resume contract, got ${resume.status}: ${redactedExcerpt(resume.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-resume`)

      const patch = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}`,
        headers,
        { method: 'PATCH', body: JSON.stringify({ title: canaryTitle, status: 'archived' }) }
      )
      if (!patch.ok || patch.body.title !== canaryTitle || patch.body.status !== 'archived') {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-patch`,
          message: `expected patch contract, got ${patch.status}: ${redactedExcerpt(patch.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-patch`)

      if (backend === 'go-runtime-default') {
        const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
        const internalCandidateRoute = await fetchTextWithTimeout(`${baseUrl}/v1/internal/go-production-candidate/boundary`, headers)
        if (forbiddenRoute.status !== 404 || internalCandidateRoute.status !== 404) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-default-hidden-routes',
            message: `expected Go public/internal candidate routes hidden, got /v1/runtime/go ${forbiddenRoute.status} and internal candidate ${internalCandidateRoute.status}`
          }
        }
        checks.push('renderer-visible-go-route-hidden', 'go-production-internal-route-hidden')

        const cleanupIds = [...new Set([threadId, forkThreadId, resumedThreadId].filter(Boolean))]
        for (const cleanupId of cleanupIds) {
          const cleanup = await fetchRuntimeJSON(
            baseUrl,
            `/v1/threads/${encodeURIComponent(cleanupId)}`,
            headers,
            { method: 'DELETE' }
          )
          if (!cleanup.ok || cleanup.body.deleted !== true) {
            return {
              ok: false,
              baseUrl,
              failedCheck: 'go-runtime-default-thread-delete',
              message: `expected canary thread cleanup for ${cleanupId}, got ${cleanup.status}: ${redactedExcerpt(cleanup.text, 400)}`
            }
          }
        }
        checks.push('go-runtime-default-thread-delete')

        return { ok: true, baseUrl, checks }
      }

      const turn = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}/turns`,
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            prompt: 'Run the Go runtime candidate contract canary.',
            ...canaryModelRequest
          })
        }
      )
      const turnId = typeof turn.body.turnId === 'string' ? turn.body.turnId : ''
      if (!turn.ok || turn.status !== 202 || turn.body.threadId !== threadId || !turnId || typeof turn.body.userMessageItemId !== 'string') {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-turn',
          message: `expected turn-create contract, got ${turn.status}: ${redactedExcerpt(turn.text, 400)}`
        }
      }
      checks.push('go-runtime-candidate-turn')

      const replay = await fetchTextWithTimeout(`${baseUrl}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, headers)
      const requiredReplayTokens = [
        'event: turn_started',
        'event: item_created',
        'event: approval_requested',
        'event: user_input_requested',
        'event: tool_catalog_changed',
        'event: accepted_final_batch',
        '"publicationSlot":"assistant-final"',
        '"publicationSlot":"usage"',
        '"cacheHitTokens":700',
        'event: pipeline_stage',
        '"parentThreadId":"' + threadId + '"',
        '"publicationSlot":"terminal"'
      ]
      if (replay.text.includes('event: assistant_text_delta')) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-assistant-draft-exposed',
          message: 'runtime candidate replay exposed a non-authoritative assistant draft'
        }
      }
      if (containsPrivateReasoningContent(replay.text)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-private-reasoning',
          message: 'runtime candidate replay exposed private reasoning'
        }
      }
      const strictReplay = strictRuntimeSseReplay(replay.text, threadId)
      if (!strictReplay.ok) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-sse-replay',
          message: `runtime candidate returned an invalid SSE replay: ${strictReplay.reason}`
        }
      }
      const replayPayloads = strictReplay.payloads
      const targetBatches = replayPayloads.filter((payload) =>
        payload.kind === 'accepted_final_batch' &&
        payload.threadId === threadId &&
        payload.turnId === turnId
      )
      const targetBatch = targetBatches[0]
      const targetBatchEvents = Array.isArray(targetBatch?.events)
        ? targetBatch.events.filter((event): event is Record<string, unknown> => (
            Boolean(event) && typeof event === 'object' && !Array.isArray(event)
          ))
        : []
      const atomicAssistantFinals = targetBatchEvents.filter((payload) => {
        const item = recordValue(payload.item)
        return payload.kind === 'item_completed' &&
          payload.threadId === threadId &&
          payload.turnId === turnId &&
          payload.itemId === `item_${turnId}_assistant` &&
          item.id === payload.itemId &&
          item.threadId === threadId &&
          item.turnId === turnId &&
          item.kind === 'assistant_text' &&
          item.role === 'assistant' &&
          item.status === 'completed' &&
          typeof item.text === 'string' && item.text.length > 0
      })
      const targetUsages = targetBatchEvents.filter((payload) => {
        const usage = recordValue(payload.usage)
        return payload.kind === 'usage' &&
          payload.threadId === threadId &&
          payload.turnId === turnId &&
          payload.publicationSlot === 'usage' &&
          usage.cacheHitTokens === 700
      })
      const targetTerminals = targetBatchEvents.filter((payload) =>
        payload.kind === 'turn_completed' &&
        payload.threadId === threadId &&
        payload.turnId === turnId &&
        payload.status === 'completed' &&
        payload.publicationSlot === 'terminal'
      )
      if (!replay.ok || !requiredReplayTokens.every((token) => replay.text.includes(token)) ||
        targetBatches.length !== 1 || targetBatchEvents.length !== 3 ||
        atomicAssistantFinals.length !== 1 || targetUsages.length !== 1 || targetTerminals.length !== 1) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-sse-replay',
          message: `expected one sealed assistant-final/usage/terminal publication batch in the turn event replay contract, got ${replay.status}: ${redactedExcerpt(replay.text, 600)}`
        }
      }
      if (replayPayloads.some((payload) => payload.kind === 'assistant_text_delta' || (
        payload.kind === 'item_completed' && recordValue(payload.item).kind === 'assistant_text'
      ))) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-assistant-draft-exposed',
          message: 'runtime candidate replay exposed a non-authoritative assistant draft'
        }
      }
      if (replayPayloads.some((payload) => containsPrivateReasoningContent(payload))) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-private-reasoning',
          message: 'runtime candidate replay exposed private reasoning'
        }
      }
      checks.push('go-runtime-candidate-sse-replay')

      const approval = await fetchRuntimeJSON(
        baseUrl,
        `/v1/approvals/${encodeURIComponent(`appr_${turnId}`)}`,
        headers,
        { method: 'POST', body: JSON.stringify({ decision: 'deny' }) }
      )
      const userInput = await fetchRuntimeJSON(
        baseUrl,
        `/v1/user-inputs/${encodeURIComponent(`input_${turnId}`)}`,
        headers,
        { method: 'POST', body: JSON.stringify({ answers: [{ id: 'q1', label: 'Ship', value: 'yes' }] }) }
      )
      const resolvedReplay = await fetchTextWithTimeout(`${baseUrl}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, headers)
      if (!approval.ok ||
        approval.body.status !== 'denied' ||
        !userInput.ok ||
        userInput.body.status !== 'submitted' ||
        !resolvedReplay.ok ||
        !resolvedReplay.text.includes('event: approval_resolved') ||
        !resolvedReplay.text.includes('event: user_input_resolved') ||
        resolvedReplay.text.includes('"answers"')) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-gates',
          message: `expected approval/user-input gate contract, got approval ${approval.status}, input ${userInput.status}, replay ${resolvedReplay.status}`
        }
      }
      checks.push('go-runtime-candidate-gates')

      const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
      const internalCandidateRoute = await fetchTextWithTimeout(`${baseUrl}/v1/internal/go-production-candidate/boundary`, headers)
      if (forbiddenRoute.status !== 404 || internalCandidateRoute.status !== 404) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-hidden-routes',
          message: `expected Go public/internal candidate routes hidden, got /v1/runtime/go ${forbiddenRoute.status} and internal candidate ${internalCandidateRoute.status}`
        }
      }
      checks.push('renderer-visible-go-route-hidden', 'go-production-internal-route-hidden')

      const cleanupIds = [...new Set([threadId, forkThreadId, resumedThreadId].filter(Boolean))]
      for (const cleanupId of cleanupIds) {
        const cleanup = await fetchRuntimeJSON(
          baseUrl,
          `/v1/threads/${encodeURIComponent(cleanupId)}`,
          headers,
          { method: 'DELETE' }
        )
        if (!cleanup.ok || cleanup.body.deleted !== true) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-candidate-thread-delete',
            message: `expected canary thread cleanup for ${cleanupId}, got ${cleanup.status}: ${redactedExcerpt(cleanup.text, 400)}`
          }
        }
      }
      checks.push('go-runtime-candidate-thread-delete')

      return { ok: true, baseUrl, checks }
    }

    const boundary = await fetchTextWithTimeout(`${baseUrl}/v1/conformance/loop/boundary`, headers)
    const boundaryBody = boundary.ok ? parseJSONRecord(boundary.text) : {}
    if (!boundary.ok ||
      boundaryBody.testConformanceOnly !== true ||
      boundaryBody.minimalAgentLoopPrototype !== true ||
      boundaryBody.fixtureBackedLoopOnly !== true ||
      boundaryBody.defaultGoBackendEnabled !== false ||
      boundaryBody.rendererVisibleGoRoutesAllowed !== false ||
      boundaryBody.reasonixPublicProtocolAllowed !== false ||
      boundaryBody.electronMainConnected !== false) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'go-conformance-boundary',
        message: `expected fixture-only Go boundary, got ${boundary.status}: ${redactedExcerpt(boundary.text, 400)}`
      }
    }
    checks.push('go-conformance-boundary')

    const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
    if (forbiddenRoute.status !== 404) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'renderer-visible-go-route',
        message: `expected /v1/runtime/go to stay hidden, got ${forbiddenRoute.status}`
      }
    }
    checks.push('renderer-visible-go-route-hidden')

    if (backend === 'go-production-candidate') {
      const productionBoundary = await fetchTextWithTimeout(
        `${baseUrl}/v1/internal/go-production-candidate/boundary`,
        headers
      )
      const productionBoundaryBody = productionBoundary.ok
        ? parseJSONRecord(productionBoundary.text)
        : {}
      const hasRuntimeContractSlice = productionBoundaryBody.runtimeGoContractParitySlice === true ||
        productionBoundaryBody.productionCandidateGoRuntimeParitySlice === true
      const usesContractReplayProvider = productionBoundaryBody.usesContractReplayProviderServer === true
      if (!productionBoundary.ok ||
        !hasRuntimeContractSlice ||
        productionBoundaryBody.internalGateOnly !== true ||
        !usesContractReplayProvider ||
        productionBoundaryBody.usesRealDurableEventSink !== true ||
        productionBoundaryBody.defaultGoBackendEnabled !== false ||
        productionBoundaryBody.rendererVisibleGoRoutesAllowed !== false ||
        productionBoundaryBody.reasonixPublicProtocolAllowed !== false) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-production-candidate-boundary',
          message: `expected production-candidate Go boundary, got ${productionBoundary.status}: ${redactedExcerpt(productionBoundary.text, 400)}`
        }
      }
      checks.push('go-production-candidate-boundary')

      const productionCanary = await fetchTextWithTimeout(
        `${baseUrl}/v1/internal/go-production-candidate/canary`,
        headers
      )
      const productionCanaryBody = productionCanary.ok
        ? parseJSONRecord(productionCanary.text)
        : {}
      const productionChecks = Array.isArray(productionCanaryBody.checks)
        ? productionCanaryBody.checks.filter((item): item is string => typeof item === 'string')
        : []
      const requiredProductionChecks: Array<string | string[]> = [
        'contract-provider-server',
        'durable-replay',
        'approval-user-input-manager',
        'contract-mcp-manager',
        'job-lineage',
        'single-baseline-checklist'
      ]
      if (!productionCanary.ok ||
        productionCanaryBody.ok !== true ||
        productionCanaryBody.defaultGoBackendEnabled !== false ||
        productionCanaryBody.rendererVisibleGoRoutesAllowed !== false ||
        productionCanaryBody.reasonixPublicProtocolAllowed !== false ||
        !requiredProductionChecks.every((check) => {
          const aliases = Array.isArray(check) ? check : [check]
          return aliases.some((alias) => productionChecks.includes(alias))
        })) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-production-candidate-canary',
          message: `expected production-candidate live provider/durable/gate/MCP/job canary, got ${productionCanary.status}: ${redactedExcerpt(productionCanary.text, 600)}`
        }
      }
      checks.push(...requiredProductionChecks.map((check) => Array.isArray(check) ? check[0] : check))
    }

    return { ok: true, baseUrl, checks }
  } catch (error) {
    return {
      ok: false,
      baseUrl,
      failedCheck: 'fetch',
      message: redactedErrorMessage(error)
    }
  }
}

