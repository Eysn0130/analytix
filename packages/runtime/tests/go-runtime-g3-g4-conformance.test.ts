import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  ApprovalUserInputRouteContract,
  GoG3ProviderStreamingUsageCacheContract,
  GoG4ToolsApprovalUserInputMcpContract,
  McpToolLifecycleContract,
  ProviderCacheContract,
  TaskJobOrchestrationContract
} from '../src/conformance/runtime-parity-fixtures.js'

const providerCacheUrl = new URL('../src/conformance/fixtures/provider-cache-contract.json', import.meta.url)
const mcpLifecycleUrl = new URL('../src/conformance/fixtures/mcp-tool-lifecycle-contract.json', import.meta.url)
const approvalUserInputUrl = new URL('../src/conformance/fixtures/approval-user-input-route-contract.json', import.meta.url)
const taskJobUrl = new URL('../src/conformance/fixtures/task-job-orchestration-contract.json', import.meta.url)
const g3Url = new URL('../src/conformance/fixtures/go-g3-provider-streaming-usage-cache-contract.json', import.meta.url)
const g4Url = new URL('../src/conformance/fixtures/go-g4-tools-approval-user-input-mcp-contract.json', import.meta.url)

function readJson(url: URL): unknown {
  return JSON.parse(readFileSync(url, 'utf8'))
}

function sseEventKinds(frames: string[]): string[] {
  return frames.map((frame) => {
    const match = /^event:\s*(.+)$/m.exec(frame)
    return match?.[1] ?? ''
  })
}

type ProviderUsageCase = ReturnType<typeof ProviderCacheContract.parse>['providerUsageCases'][number]

type ParsedProviderUsage = {
  promptTokens?: number
  completionTokens?: number
  reasoningTokens?: number
  totalTokens?: number
  cacheHitTokens?: number
  cacheMissTokens?: number
  cacheHitRate?: number | null
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function numberValue(record: Record<string, unknown>, field: string): number {
  const value = record[field]
  return typeof value === 'number' ? value : 0
}

function usageBody(testCase: ProviderUsageCase): Record<string, unknown> {
  return recordValue(testCase.responseBody.usage)
}

function parsedProviderUsageFromRawPayload(testCase: ProviderUsageCase): ParsedProviderUsage {
  const usage = usageBody(testCase)
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'responses') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const inputDetails = recordValue(usage.input_tokens_details)
    const outputDetails = recordValue(usage.output_tokens_details)
    const cachedTokens = numberValue(inputDetails, 'cached_tokens')
    return {
      promptTokens: inputTokens,
      completionTokens: outputTokens,
      reasoningTokens: numberValue(outputDetails, 'reasoning_tokens'),
      totalTokens: numberValue(usage, 'total_tokens'),
      cacheHitTokens: cachedTokens,
      cacheMissTokens: inputTokens - cachedTokens,
      cacheHitRate: cachedTokens / inputTokens
    }
  }
  if (endpointFormat === 'messages') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const cacheRead = numberValue(usage, 'cache_read_input_tokens')
    const cacheCreation = numberValue(usage, 'cache_creation_input_tokens')
    const promptTokens = inputTokens + cacheRead + cacheCreation
    const cacheMissTokens = inputTokens + cacheCreation
    return {
      promptTokens,
      completionTokens: outputTokens,
      totalTokens: promptTokens + outputTokens,
      cacheHitTokens: cacheRead,
      cacheMissTokens,
      cacheHitRate: cacheRead / (cacheRead + cacheMissTokens)
    }
  }
  const promptTokens = numberValue(usage, 'prompt_tokens')
  const promptDetails = recordValue(usage.prompt_tokens_details)
  const completionDetails = recordValue(usage.completion_tokens_details)
  const nativeHitTokens = numberValue(usage, 'prompt_cache_hit_tokens')
  const nativeMissTokens = numberValue(usage, 'prompt_cache_miss_tokens')
  const cachedTokens = numberValue(promptDetails, 'cached_tokens')
  const parsed: ParsedProviderUsage = {
    promptTokens,
    completionTokens: numberValue(usage, 'completion_tokens'),
    reasoningTokens: numberValue(completionDetails, 'reasoning_tokens') || undefined,
    totalTokens: numberValue(usage, 'total_tokens')
  }
  if (nativeHitTokens > 0 || nativeMissTokens > 0) {
    parsed.cacheHitTokens = nativeHitTokens
    parsed.cacheMissTokens = nativeMissTokens
    parsed.cacheHitRate = nativeHitTokens / (nativeHitTokens + nativeMissTokens)
  } else if (cachedTokens > 0) {
    parsed.cacheHitTokens = cachedTokens
    parsed.cacheMissTokens = promptTokens - cachedTokens
    parsed.cacheHitRate = cachedTokens / promptTokens
  } else {
    parsed.cacheHitRate = null
  }
  return parsed
}

function usageNumberMatches(left: number | null | undefined, right: number | null | undefined): boolean {
  return left === right
}

function parsedUsageMatchesExpected(testCase: ProviderUsageCase): boolean {
  const parsed = parsedProviderUsageFromRawPayload(testCase)
  const expected = testCase.expectedUsage
  return usageNumberMatches(parsed.promptTokens, expected.promptTokens) &&
    usageNumberMatches(parsed.completionTokens, expected.completionTokens) &&
    usageNumberMatches(parsed.reasoningTokens, expected.reasoningTokens) &&
    usageNumberMatches(parsed.totalTokens, expected.totalTokens) &&
    usageNumberMatches(parsed.cacheHitTokens, expected.cacheHitTokens) &&
    usageNumberMatches(parsed.cacheMissTokens, expected.cacheMissTokens) &&
    usageNumberMatches(parsed.cacheHitRate, expected.cacheHitRate)
}

function providerCacheAccounting(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const parsedCases = providerCache.providerUsageCases.map((item) => ({
    ...item,
    parsedUsage: parsedProviderUsageFromRawPayload(item)
  }))
  const supported = parsedCases
    .filter((item) => item.parsedUsage.cacheHitTokens !== undefined && item.parsedUsage.cacheMissTokens !== undefined)
  const totalCacheHitTokens = supported.reduce((sum, item) => sum + (item.parsedUsage.cacheHitTokens ?? 0), 0)
  const totalCacheMissTokens = supported.reduce((sum, item) => sum + (item.parsedUsage.cacheMissTokens ?? 0), 0)
  return {
    rawPayloadParsedCaseIds: parsedCases.map((item) => item.id),
    rawTelemetrySupportedCaseIds: supported.map((item) => item.id),
    rawMatchesExpectedUsageCaseIds: providerCache.providerUsageCases
      .filter((item) => parsedUsageMatchesExpected(item))
      .map((item) => item.id),
    telemetrySupportedCaseIds: supported.map((item) => item.id),
    unsupportedUnknownCaseIds: parsedCases
      .filter((item) => item.parsedUsage.cacheHitRate === null)
      .map((item) => item.id),
    deepseekCaseIds: supported
      .filter((item) => item.baseUrl.includes('deepseek'))
      .map((item) => item.id),
    openaiCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'responses')
      .map((item) => item.id),
    anthropicCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'messages')
      .map((item) => item.id),
    totalCacheHitTokens,
    totalCacheMissTokens,
    aggregateCacheHitRate: totalCacheHitTokens / (totalCacheHitTokens + totalCacheMissTokens),
    unsupportedProvidersCountedAsMisses: false
  }
}

function providerCacheDriftAttribution(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const drift = providerCache.driftAttribution
  return {
    previousPrefixHash: drift.previousShape.prefixHash,
    currentPrefixHash: drift.currentShape.prefixHash,
    systemHashStable: drift.previousShape.systemHash === drift.currentShape.systemHash,
    prefixItemsHashStable: drift.previousShape.prefixItemsHash === drift.currentShape.prefixItemsHash,
    toolsHashChanged: drift.previousShape.toolsHash !== drift.currentShape.toolsHash,
    providerChanged: drift.previousShape.providerId !== drift.currentShape.providerId,
    modelChanged: drift.previousShape.model !== drift.currentShape.model,
    endpointFormatChanged: drift.previousShape.endpointFormat !== drift.currentShape.endpointFormat,
    expectedReasons: drift.expectedReasons,
    telemetrySupported: drift.expectedTelemetrySupported,
    cacheHitRateKnown: drift.usage.cacheHitRate !== null
  }
}

function uniqueInOrder(values: string[]): string[] {
  return Array.from(new Set(values))
}

function countRequiredBodyField(
  cases: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'],
  field: string
): number {
  return cases.filter((item) => item.requiredBodyFields.includes(field)).length
}

function countForbiddenBodyField(
  cases: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'],
  field: string
): number {
  return cases.filter((item) => item.forbiddenBodyFields.includes(field)).length
}

function appendProviderEndpointPath(baseUrl: string, versionedPath: string): string {
  const trimmed = baseUrl.replace(/\/+$/, '')
  if (trimmed.endsWith('/v1') && versionedPath.startsWith('/v1/')) {
    return `${trimmed}${versionedPath.slice('/v1'.length)}`
  }
  return `${trimmed}${versionedPath}`
}

function derivedProviderRequestUrl(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string {
  if (testCase.endpointFormat === 'custom_endpoint') return testCase.baseUrl
  if (testCase.endpointFormat === 'responses') {
    return appendProviderEndpointPath(testCase.baseUrl, '/v1/responses')
  }
  if (testCase.endpointFormat === 'messages') {
    return appendProviderEndpointPath(testCase.baseUrl, '/v1/messages')
  }
  return appendProviderEndpointPath(testCase.baseUrl, '/v1/chat/completions')
}

function derivedProviderToolShape(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string {
  if (testCase.endpointFormat === 'responses' || testCase.baseUrl.endsWith('/responses')) {
    return 'responses-function'
  }
  if (testCase.endpointFormat === 'messages' || testCase.baseUrl.endsWith('/messages')) {
    return 'anthropic-input-schema'
  }
  return 'openai-function'
}

function derivedProviderRequiredHeaders(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  if (derivedProviderToolShape(testCase) === 'anthropic-input-schema') {
    return ['Authorization', 'x-api-key', 'anthropic-version']
  }
  return ['Authorization']
}

function derivedProviderForbiddenHeaders(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  if (derivedProviderToolShape(testCase) === 'anthropic-input-schema') return []
  return ['x-api-key', 'anthropic-version']
}

function derivedProviderRequiredBodyFields(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  const toolShape = derivedProviderToolShape(testCase)
  if (toolShape === 'responses-function') {
    return ['model', 'stream', 'input', 'tools', 'max_output_tokens']
  }
  if (toolShape === 'anthropic-input-schema') {
    return ['model', 'stream', 'system', 'messages', 'tools', 'max_tokens']
  }
  const fields = ['model', 'stream', 'messages', 'tools']
  if (testCase.baseUrl.includes('deepseek')) fields.push('thinking')
  fields.push('reasoning_effort')
  return fields
}

function derivedProviderForbiddenBodyFields(
  testCase: ReturnType<typeof ProviderCacheContract.parse>['requestShapeCases'][number]
): string[] {
  const toolShape = derivedProviderToolShape(testCase)
  if (toolShape === 'responses-function') return ['messages', 'system', 'thinking']
  if (toolShape === 'anthropic-input-schema') return ['input', 'max_output_tokens', 'thinking']
  const fields = ['input', 'system', 'max_output_tokens']
  if (!testCase.baseUrl.includes('deepseek')) fields.push('thinking')
  return fields
}

function sameStringArray(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((item, index) => item === right[index])
}

function providerRequestShapeSummary(providerCache: ReturnType<typeof ProviderCacheContract.parse>) {
  const cases = providerCache.requestShapeCases
  return {
    caseCount: cases.length,
    exactUrlCount: cases.filter((item) => item.expectedUrl.length > 0).length,
    derivedUrlMatchCaseIds: cases
      .filter((item) => derivedProviderRequestUrl(item) === item.expectedUrl)
      .map((item) => item.id),
    headerShapeMatchCaseIds: cases
      .filter((item) =>
        sameStringArray(derivedProviderRequiredHeaders(item), item.requiredHeaders) &&
        sameStringArray(derivedProviderForbiddenHeaders(item), item.forbiddenHeaders)
      )
      .map((item) => item.id),
    bodyShapeMatchCaseIds: cases
      .filter((item) =>
        sameStringArray(derivedProviderRequiredBodyFields(item), item.requiredBodyFields) &&
        sameStringArray(derivedProviderForbiddenBodyFields(item), item.forbiddenBodyFields)
      )
      .map((item) => item.id),
    toolShapeMatchCaseIds: cases
      .filter((item) => derivedProviderToolShape(item) === item.expectedToolShape)
      .map((item) => item.id),
    matrix: cases,
    endpointFormats: uniqueInOrder(cases.map((item) => item.endpointFormat)),
    fullEndpointCaseIds: cases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.id),
    toolShapes: uniqueInOrder(cases.map((item) => item.expectedToolShape)),
    customFullEndpointExactUrlCaseIds: cases
      .filter((item) =>
        item.endpointFormat === 'custom_endpoint' &&
        derivedProviderRequestUrl(item) === item.baseUrl &&
        item.expectedUrl === item.baseUrl
      )
      .map((item) => item.id),
    customFullEndpointAppendedPathCount: cases
      .filter((item) => item.endpointFormat === 'custom_endpoint' && item.expectedUrl !== item.baseUrl)
      .length,
    requiredBodyFieldFamilies: {
      messagesFieldCaseCount: countRequiredBodyField(cases, 'messages'),
      inputFieldCaseCount: countRequiredBodyField(cases, 'input'),
      systemFieldCaseCount: countRequiredBodyField(cases, 'system'),
      thinkingFieldCaseCount: countRequiredBodyField(cases, 'thinking'),
      maxOutputTokensCaseCount: countRequiredBodyField(cases, 'max_output_tokens'),
      maxTokensCaseCount: countRequiredBodyField(cases, 'max_tokens')
    },
    forbiddenBodyFieldFamilies: {
      thinkingForbiddenCaseCount: countForbiddenBodyField(cases, 'thinking'),
      systemForbiddenCaseCount: countForbiddenBodyField(cases, 'system'),
      inputForbiddenCaseCount: countForbiddenBodyField(cases, 'input'),
      maxOutputTokensForbiddenCaseCount: countForbiddenBodyField(cases, 'max_output_tokens')
    }
  }
}

describe('Go G3/G4 conformance contracts', () => {
  it('ties G3 provider streaming, usage, and cache output to the TypeScript provider/cache contract', () => {
    const providerCache = ProviderCacheContract.parse(readJson(providerCacheUrl))
    const g3 = GoG3ProviderStreamingUsageCacheContract.parse(readJson(g3Url))

    expect(g3.sourceContractIds).toEqual([providerCache.id])
    expect(g3.providerUsageMatrix).toEqual(
      providerCache.providerUsageCases.map((item) => ({
        id: item.id,
        endpointFormat: item.endpointFormat ?? 'chat_completions',
        baseUrl: item.baseUrl,
        model: item.model,
        responseBody: item.responseBody,
        expectedUsage: item.expectedUsage
      }))
    )
    expect(g3.requestShapeCaseIds)
      .toEqual(providerCache.requestShapeCases.map((item) => item.id))
    expect(g3.requestShapeMatrix).toEqual(providerCache.requestShapeCases)
    expect(g3.cacheDiagnostics).toMatchObject({
      prefixHash: providerCache.stablePrefix.firstShape.prefixHash,
      systemHash: providerCache.stablePrefix.firstShape.systemHash,
      prefixItemsHash: providerCache.stablePrefix.firstShape.prefixItemsHash,
      toolsHash: providerCache.stablePrefix.firstShape.toolsHash,
      toolSchemaTokens: providerCache.stablePrefix.firstShape.toolSchemaTokens,
      provider: providerCache.stablePrefix.firstShape.provider,
      providerId: providerCache.stablePrefix.firstShape.providerId,
      endpointFormat: providerCache.stablePrefix.firstShape.endpointFormat,
      model: providerCache.stablePrefix.firstShape.model,
      forbiddenDiagnosticsSubstrings: providerCache.privacy.forbiddenDiagnosticsSubstrings
    })
    expect(g3.cacheDriftAttribution).toEqual(providerCache.driftAttribution)
    expect(g3.cacheAccounting).toEqual(providerCacheAccounting(providerCache))
    expect(sseEventKinds(g3.streaming.sseFrames)).toEqual(g3.streaming.expectedKindsInOrder)
    expect(g3.expectedOutput).toEqual({
      stage: 'G3',
      mode: 'shadow-conformance-only',
      sourceContractIds: [providerCache.id],
      providerUsageCaseIds: g3.providerUsageMatrix.map((item) => item.id),
      requestShapeCaseIds: providerCache.requestShapeCases.map((item) => item.id),
      requestShapeSummary: providerRequestShapeSummary(providerCache),
      streamingKinds: g3.streaming.expectedKindsInOrder,
      cacheTelemetrySupported: true,
      cacheDriftAttribution: providerCacheDriftAttribution(providerCache),
      cacheAccounting: providerCacheAccounting(providerCache),
      productBoundary: g3.productBoundary
    })
  })

  it('ties G4 tools, approvals, user input, and MCP output to TypeScript route/lifecycle contracts', () => {
    const mcp = McpToolLifecycleContract.parse(readJson(mcpLifecycleUrl))
    const approvalUserInput = ApprovalUserInputRouteContract.parse(readJson(approvalUserInputUrl))
    const taskJob = TaskJobOrchestrationContract.parse(readJson(taskJobUrl))
    const g4 = GoG4ToolsApprovalUserInputMcpContract.parse(readJson(g4Url))

    expect(g4.sourceContractIds).toEqual([mcp.id, approvalUserInput.id, taskJob.id])
    expect(g4.toolCatalog.forbiddenTopLevelRoutes).toEqual(taskJob.routeContract.forbiddenTopLevelRoutes)
    expect(g4.approval).toMatchObject({
      id: approvalUserInput.approval.id,
      toolName: approvalUserInput.approval.toolName,
      decision: approvalUserInput.approval.decisionRequest.decision,
      expectedStatus: approvalUserInput.approval.expectedResponse.body?.status,
      secondDecisionStatus: approvalUserInput.approval.secondDecisionStatus,
      pendingBefore: approvalUserInput.approval.pendingBefore,
      pendingAfter: approvalUserInput.approval.pendingAfter
    })
    expect(g4.userInput).toMatchObject({
      id: approvalUserInput.userInput.id,
      resolution: 'cancelled',
      expectedStatus: approvalUserInput.userInput.expectedResponse.body?.status,
      secondResolveStatus: approvalUserInput.userInput.secondResolveStatus,
      pendingBefore: approvalUserInput.userInput.pendingBefore,
      pendingAfter: approvalUserInput.userInput.pendingAfter,
      remoteDisableUserInputPreserved: approvalUserInput.remoteEntry.startRequest.disableUserInput
    })
    expect(g4.userInput.structuredChoiceValidation).toMatchObject({
      maxQuestions: 3,
      minOptionsWhenProvided: 2,
      maxOptionsWhenProvided: 3,
      dedupeLabelsCaseInsensitive: true,
      invalidResultCode: 'invalid_user_input_request',
      opensGateOnInvalid: false,
      invalidCases: [
        'too_many_questions',
        'single_option',
        'too_many_options',
        'duplicate_labels_case_insensitive'
      ]
    })
    expect(g4.userInput.submittedRoute).toEqual({
      status: approvalUserInput.submittedUserInput.expectedResolution.status,
      answersEchoed: approvalUserInput.submittedUserInput.expectedResponse.body?.answers !== undefined,
      resolvedEventIncludesAnswers: approvalUserInput.submittedUserInput.resolvedEvent.includesAnswers
    })
    expect(g4.mcp).toMatchObject({
      providerId: mcp.providerId,
      connectToolNames: mcp.connect.toolNames,
      reloadToolNames: mcp.reload.toolNames,
      disconnectReason: mcp.disconnect.reason,
      cancel: mcp.cancel,
      diagnosticsRedaction: mcp.diagnosticsRedaction,
      approvalAnnotations: {
        normalizedToolName: mcp.approvalAnnotations.normalizedToolName,
        decision: mcp.approvalAnnotations.decision,
        executed: mcp.approvalAnnotations.executed
      },
      searchMetaTools: {
        toolNames: mcp.searchMetaTools.toolNames,
        trustedToolId: mcp.searchMetaTools.trustedToolId,
        untrustedSearchedTools: mcp.searchMetaTools.untrustedSearchedTools,
        unknownToolError: mcp.searchMetaTools.unknownToolError,
        callPolicy: mcp.searchMetaTools.callPolicy,
        deniedCallExecuted: mcp.searchMetaTools.deniedCallExecuted
      },
      knownOverrideDiagnostic: mcp.knownOverride.diagnostic
    })
    expect(g4.plannerExecutor.plannerReadOnlyToolset)
      .toEqual(taskJob.permissions.plannerReadOnlyToolset)
    expect(g4.remoteEntryBoundary).toEqual({
      expectedPortKeys: approvalUserInput.remoteEntry.expectedPortKeys,
      forbiddenPortKeys: approvalUserInput.remoteEntry.forbiddenPortKeys
    })
    expect(g4.expectedOutput).toEqual({
      stage: 'G4',
      mode: 'shadow-conformance-only',
      sourceContractIds: [mcp.id, approvalUserInput.id, taskJob.id],
      toolNames: g4.toolCatalog.advertisedToolNames,
      approvalDeniedNoExecute: true,
      userInputCancelled: true,
      userInputValidationCode: g4.userInput.structuredChoiceValidation.invalidResultCode,
      userInputValidationCases: g4.userInput.structuredChoiceValidation.invalidCases,
      userInputInvalidOpensGate: false,
      userInputSubmittedAnswersEchoed: true,
      userInputResolvedEventOmitsAnswers: true,
      mcpApprovalAnnotatedNoExecute: true,
      mcpSearchMetaToolsAdvertised: true,
      mcpSearchUntrustedWorkspaceHidden: true,
      mcpSearchCallDeniedNoExecute: true,
      mcpKnownOverride: mcp.knownOverride.diagnostic.knownOverride,
      plannerReadOnlyToolset: taskJob.permissions.plannerReadOnlyToolset,
      productBoundary: g4.productBoundary
    })
  })
})
