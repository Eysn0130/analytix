import { z } from 'zod'

export const RUNTIME_PARITY_FIXTURE_VERSION = 1

const HttpBodyExpectation = z.object({
  status: z.number().int().positive(),
  body: z.record(z.string(), z.unknown()).optional()
})

const ShadowOnlyProductBoundary = z.object({
  rendererPreloadMainBridgeUnchanged: z.literal(true),
  analytixServeContractUnchanged: z.literal(true),
  reasonixPublicProtocolAllowed: z.literal(false),
  defaultGoBackendAllowed: z.literal(false),
  rendererVisibleGoRoutesAllowed: z.literal(false)
})

const UserInputAnswerExpectation = z.object({
  id: z.string().min(1),
  label: z.string().min(1),
  value: z.string()
})

const UserInputQuestionExpectation = z.object({
  header: z.string().min(1),
  id: z.string().min(1),
  question: z.string().min(1),
  options: z.array(z.object({
    label: z.string().min(1),
    description: z.string()
  }))
})

const UserInputValidationCaseId = z.enum([
  'too_many_questions',
  'single_option',
  'too_many_options',
  'duplicate_labels_case_insensitive'
])

const UserInputStructuredChoiceValidation = z.object({
  maxQuestions: z.literal(3),
  minOptionsWhenProvided: z.literal(2),
  maxOptionsWhenProvided: z.literal(3),
  dedupeLabelsCaseInsensitive: z.literal(true),
  invalidResultCode: z.literal('invalid_user_input_request'),
  opensGateOnInvalid: z.literal(false),
  invalidCases: z.array(UserInputValidationCaseId).min(4)
})

export const ApprovalUserInputRouteContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  runtimeToken: z.string().min(1),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  approval: z.object({
    id: z.string().min(1),
    itemId: z.string().min(1),
    toolName: z.string().min(1),
    summary: z.string().min(1),
    decisionRequest: z.object({
      decision: z.enum(['allow', 'deny']),
      reason: z.string().optional()
    }),
    expectedResponse: HttpBodyExpectation,
    secondDecisionStatus: z.number().int().positive(),
    pendingBefore: z.number().int().nonnegative(),
    pendingAfter: z.number().int().nonnegative()
  }),
  userInput: z.object({
    id: z.string().min(1),
    itemId: z.string().min(1),
    prompt: z.string().min(1),
    questions: z.array(UserInputQuestionExpectation),
    resolveRequest: z.object({
      cancelled: z.boolean().optional(),
      answers: z.array(UserInputAnswerExpectation).optional()
    }),
    expectedResponse: HttpBodyExpectation,
    secondResolveStatus: z.number().int().positive(),
    pendingBefore: z.number().int().nonnegative(),
    pendingAfter: z.number().int().nonnegative()
  }),
  submittedUserInput: z.object({
    threadId: z.string().min(1),
    id: z.string().min(1),
    itemId: z.string().min(1),
    prompt: z.string().min(1),
    questions: z.array(UserInputQuestionExpectation),
    resolveRequest: z.object({
      answers: z.array(UserInputAnswerExpectation).min(1)
    }),
    expectedResolution: z.object({
      status: z.literal('submitted'),
      answers: z.array(UserInputAnswerExpectation).min(1)
    }),
    expectedResponse: HttpBodyExpectation,
    pendingBefore: z.number().int().nonnegative(),
    pendingAfter: z.number().int().nonnegative(),
    resolvedEvent: z.object({
      kind: z.literal('user_input_resolved'),
      status: z.literal('submitted'),
      includesAnswers: z.literal(false)
    })
  }),
  resumePendingGates: z.object({
    sourceThreadId: z.string().min(1),
    approvalId: z.string().min(1),
    userInputId: z.string().min(1),
    expectedSourceStatuses: z.object({
      approval: z.literal('pending'),
      userInput: z.literal('pending')
    }),
    expectedResumedStatuses: z.object({
      approval: z.literal('expired'),
      userInput: z.literal('cancelled')
    })
  }),
  abortCleanup: z.object({
    approvalId: z.string().min(1),
    userInputId: z.string().min(1),
    expectedApprovalStatus: z.literal('expired'),
    expectedUserInputStatus: z.literal('cancelled'),
    lateApprovalDecisionStatus: z.number().int().positive(),
    lateUserInputResolveStatus: z.number().int().positive(),
    expectedReplayKinds: z.array(z.string().min(1))
  }),
  replay: z.object({
    sinceSeq: z.number().int().nonnegative(),
    expectedKindsInOrder: z.array(z.string().min(1))
  }),
  remoteEntry: z.object({
    threadId: z.string().min(1),
    startRequest: z.object({
      prompt: z.string().min(1),
      disableUserInput: z.boolean().optional()
    }),
    expectedPortKeys: z.array(z.string().min(1)),
    forbiddenPortKeys: z.array(z.string().min(1)),
    rejectedOverride: z.record(z.string(), z.unknown())
  })
})
export type ApprovalUserInputRouteContract = z.infer<typeof ApprovalUserInputRouteContract>

const GoG5PromptQuestionReplay = z.object({
  header: z.string().min(1),
  id: z.string().min(1),
  optionLabels: z.array(z.string().min(1)).min(1)
})

const GoG5ApprovalRouteReplay = z.object({
  itemId: z.string().min(1),
  toolName: z.string().min(1),
  summary: z.string().min(1),
  requestBody: z.object({
    decision: z.literal('deny'),
    reason: z.string().min(1)
  }),
  responseStatus: z.literal(200),
  responseBody: z.object({
    approvalId: z.string().min(1),
    decision: z.literal('deny'),
    status: z.literal('denied')
  }),
  pendingBefore: z.literal(1),
  pendingAfter: z.literal(0)
})

const GoG5UserInputCancelRouteReplay = z.object({
  itemId: z.string().min(1),
  prompt: z.string().min(1),
  questions: z.array(GoG5PromptQuestionReplay).min(1),
  requestBody: z.object({
    cancelled: z.literal(true)
  }),
  responseStatus: z.literal(200),
  responseBody: z.object({
    inputId: z.string().min(1),
    status: z.literal('cancelled')
  }),
  secondResolveStatus: z.literal(404),
  pendingBefore: z.literal(1),
  pendingAfter: z.literal(0)
})

const GoG5UserInputSubmitRouteReplay = z.object({
  threadId: z.string().min(1),
  itemId: z.string().min(1),
  prompt: z.string().min(1),
  questions: z.array(GoG5PromptQuestionReplay).min(1),
  requestBody: z.object({
    answers: z.array(UserInputAnswerExpectation).min(1)
  }),
  responseStatus: z.literal(200),
  responseBody: z.object({
    inputId: z.string().min(1),
    status: z.literal('submitted'),
    answers: z.array(UserInputAnswerExpectation).min(1)
  }),
  resolvedEvent: z.object({
    kind: z.literal('user_input_resolved'),
    status: z.literal('submitted'),
    includesAnswers: z.literal(false)
  }),
  pendingBefore: z.literal(1),
  pendingAfter: z.literal(0)
})

const GoG5ApprovalUserInputReplayOutput = z.object({
  approvalRoute: GoG5ApprovalRouteReplay,
  approvalId: z.string().min(1),
  approvalDecision: z.literal('deny'),
  approvalStatus: z.literal('denied'),
  approvalPendingAfter: z.literal(0),
  secondApprovalDecisionStatus: z.literal(409),
  replaySinceSeq: z.literal(0),
  replayKindsInOrder: z.array(z.enum([
    'approval_requested',
    'approval_resolved',
    'user_input_requested',
    'user_input_resolved'
  ])).length(4),
  submittedInputId: z.string().min(1),
  submittedStatus: z.literal('submitted'),
  userInputSubmitRoute: GoG5UserInputSubmitRouteReplay,
  answerCount: z.number().int().positive(),
  httpEchoesAnswers: z.literal(true),
  resolvedEventKind: z.literal('user_input_resolved'),
  resolvedEventIncludesAnswers: z.literal(false),
  cancelledInputId: z.string().min(1),
  cancelledStatus: z.literal('cancelled'),
  userInputCancelRoute: GoG5UserInputCancelRouteReplay,
  lateResolveRejected: z.literal(true),
  pendingAfterSubmit: z.literal(0),
  pendingAfterCancel: z.literal(0),
  abortApprovalId: z.string().min(1),
  abortApprovalStatus: z.literal('expired'),
  abortUserInputId: z.string().min(1),
  abortUserInputStatus: z.literal('cancelled'),
  lateApprovalDecisionStatus: z.literal(409),
  lateUserInputResolveStatus: z.literal(404),
  pendingAfterAbortCleanup: z.literal(0),
  abortReplayKinds: z.array(z.enum([
    'approval_requested',
    'approval_resolved',
    'user_input_requested',
    'user_input_resolved'
  ])).length(4)
})

const GoApprovalUserInputRouteReplayContract = ApprovalUserInputRouteContract.pick({
  id: true,
  approval: true,
  replay: true,
  userInput: true,
  submittedUserInput: true,
  abortCleanup: true
})

const GoApprovalUserInputRouteReplayControlCase = z.object({
  sourceContractId: z.literal('approval-user-input-route-api-gates-v1'),
  contract: GoApprovalUserInputRouteReplayContract,
  expected: GoG5ApprovalUserInputReplayOutput
})

const GoApprovalUserInputInventoryInput = z.object({
  approvalId: z.string().min(1),
  approvalPendingAfter: z.literal(0),
  submittedInputId: z.string().min(1),
  cancelledInputId: z.string().min(1),
  abortApprovalId: z.string().min(1),
  abortUserInputId: z.string().min(1),
  replayKindsInOrder: z.array(z.enum([
    'approval_requested',
    'approval_resolved',
    'user_input_requested',
    'user_input_resolved'
  ])).length(4),
  abortReplayKinds: z.array(z.enum([
    'approval_requested',
    'approval_resolved',
    'user_input_requested',
    'user_input_resolved'
  ])).length(4),
  answerCount: z.number().int().positive(),
  httpEchoesAnswers: z.literal(true),
  resolvedEventIncludesAnswers: z.literal(false),
  lateApprovalDecisionStatus: z.literal(409),
  lateUserInputResolveStatus: z.literal(404),
  pendingAfterSubmit: z.literal(0),
  pendingAfterCancel: z.literal(0),
  pendingAfterAbortCleanup: z.literal(0)
})

const GoApprovalUserInputInventoryOutput = z.object({
  gateIds: z.array(z.string().min(1)).length(5),
  approvalIds: z.array(z.string().min(1)).length(2),
  userInputIds: z.array(z.string().min(1)).length(3),
  routeKinds: z.array(z.enum([
    'approval-decision',
    'user-input-submit',
    'user-input-cancel',
    'abort-cleanup'
  ])).length(4),
  replayKindsInOrder: GoApprovalUserInputInventoryInput.shape.replayKindsInOrder,
  abortReplayKinds: GoApprovalUserInputInventoryInput.shape.abortReplayKinds,
  answerCount: z.number().int().positive(),
  httpEchoesAnswers: z.literal(true),
  resolvedEventIncludesAnswers: z.literal(false),
  lateApprovalDecisionStatus: z.literal(409),
  lateUserInputResolveStatus: z.literal(404),
  pendingAfterAll: z.literal(0),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoApprovalUserInputInventoryControlCase = z.object({
  sourceContractId: z.literal('approval-user-input-route-api-gates-v1'),
  input: GoApprovalUserInputInventoryInput,
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoApprovalUserInputInventoryOutput
})

const CacheShapeExpectation = z.object({
  prefixHash: z.string().min(1),
  systemHash: z.string().min(1),
  modeHash: z.string().min(1).optional(),
  prefixItemsHash: z.string().min(1),
  toolsHash: z.string().min(1),
  toolSchemaTokens: z.number().int().nonnegative(),
  provider: z.string().optional(),
  providerId: z.string().optional(),
  endpointFormat: z.string().optional(),
  model: z.string().optional()
})

const UsageExpectation = z.object({
  promptTokens: z.number().int().nonnegative().optional(),
  completionTokens: z.number().int().nonnegative().optional(),
  reasoningTokens: z.number().int().nonnegative().optional(),
  totalTokens: z.number().int().nonnegative().optional(),
  cacheHitTokens: z.number().int().nonnegative().optional(),
  cacheMissTokens: z.number().int().nonnegative().optional(),
  cacheHitRate: z.number().nullable().optional(),
  absent: z.array(z.string().min(1)).optional()
})

const ProviderCacheCoverageFamily = z.enum([
  'deepseek',
  'openai-compatible',
  'openai-responses',
  'anthropic-messages',
  'custom-full-endpoint'
])

const ProviderLiveLocalHttpContract = z.object({
  fixtureOnly: z.literal(true),
  transport: z.literal('local-http'),
  usesLiveCredentials: z.literal(false),
  preservesOriginalProviderBaseUrl: z.literal(true),
  usageCaseCount: z.literal(5),
  requestShapeCaseCount: z.literal(7),
  expectedPostCount: z.literal(12),
  coveredEndpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  coveredProviderFamilies: z.array(ProviderCacheCoverageFamily).length(5),
  mayClaimLiveSuperiority: z.literal(false)
})

const ProviderUsageContractCase = z.object({
  id: z.string().min(1),
  endpointFormat: z.string().optional(),
  baseUrl: z.string().url(),
  model: z.string().min(1),
  responseBody: z.record(z.string(), z.unknown()),
  expectedUsage: UsageExpectation
})

const ProviderUsageAccountingCase = z.object({
  id: z.string().min(1),
  endpointFormat: z.string().optional(),
  baseUrl: z.string().url(),
  responseBody: z.record(z.string(), z.unknown()),
  expectedUsage: UsageExpectation
})

const ProviderRequestShapeContractCase = z.object({
  id: z.string().min(1),
  endpointFormat: z.enum(['chat_completions', 'responses', 'messages', 'custom_endpoint']),
  baseUrl: z.string().url(),
  model: z.string().min(1),
  reasoningEffort: z.string().optional(),
  expectedUrl: z.string().url(),
  requiredHeaders: z.array(z.string().min(1)),
  forbiddenHeaders: z.array(z.string().min(1)),
  requiredBodyFields: z.array(z.string().min(1)),
  forbiddenBodyFields: z.array(z.string().min(1)),
  expectedToolShape: z.enum(['openai-function', 'responses-function', 'anthropic-input-schema'])
})

const ProviderRequestShapeCoverageCase = z.object({
  id: z.string().min(1),
  endpointFormat: z.enum(['chat_completions', 'responses', 'messages', 'custom_endpoint']),
  baseUrl: z.string().url(),
  expectedUrl: z.string().url(),
  expectedToolShape: z.enum(['openai-function', 'responses-function', 'anthropic-input-schema'])
})

export const ProviderCacheContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  stablePrefix: z.object({
    firstShape: CacheShapeExpectation,
    equivalentShape: CacheShapeExpectation,
    usage: UsageExpectation,
    expectedDiagnostics: z.object({
      prefixChanged: z.boolean(),
      prefixChangeReasons: z.array(z.string()),
      cacheTelemetrySupported: z.boolean(),
      cacheHitTokens: z.number().int().nonnegative().optional(),
      cacheMissTokens: z.number().int().nonnegative().optional(),
      provider: z.string().optional(),
      providerId: z.string().optional(),
      endpointFormat: z.string().optional(),
      model: z.string().optional()
    })
  }),
  driftAttribution: z.object({
    previousShape: CacheShapeExpectation,
    currentShape: CacheShapeExpectation,
    usage: UsageExpectation,
    expectedReasons: z.array(z.string().min(1)),
    expectedTelemetrySupported: z.boolean()
  }),
  providerUsageCases: z.array(ProviderUsageContractCase),
  requestShapeCases: z.array(ProviderRequestShapeContractCase),
  releaseGuard: z.object({
    fixtureOnly: z.literal(true),
    thresholdPercent: z.number().positive(),
    maxLowTailCases: z.literal(0),
    tailWindow: z.number().int().positive(),
    cases: z.array(z.object({
      id: z.string().min(1),
      cacheHitPercentCurve: z.array(z.number().nonnegative()),
      expectedTailAveragePercent: z.number().nonnegative(),
      expectedStatus: z.enum(['pass', 'fail']),
      compactionGuardPaused: z.boolean().optional(),
      maxAllowedCollapses: z.number().int().nonnegative().optional()
    })).min(1)
  }),
  liveLocalHttpContract: ProviderLiveLocalHttpContract,
  privacy: z.object({
    forbiddenDiagnosticsSubstrings: z.array(z.string().min(1))
  }),
  liveCredentialPolicy: z.object({
    fixtureOnly: z.literal(true),
    mayClaimSuperiority: z.literal(false)
  })
})
export type ProviderCacheContract = z.infer<typeof ProviderCacheContract>

const GoProviderUsageCaseSummary = z.object({
  id: z.string().min(1),
  endpointFormat: z.string().min(1),
  baseUrl: z.string().url(),
  model: z.string().min(1),
  responseBody: z.record(z.string(), z.unknown()),
  expectedUsage: UsageExpectation
})

const GoProviderRequestShapeCaseSummary = z.object({
  id: z.string().min(1),
  endpointFormat: z.enum(['chat_completions', 'responses', 'messages', 'custom_endpoint']),
  baseUrl: z.string().url(),
  model: z.string().min(1),
  reasoningEffort: z.string().optional(),
  expectedUrl: z.string().url(),
  requiredHeaders: z.array(z.string().min(1)),
  forbiddenHeaders: z.array(z.string().min(1)),
  requiredBodyFields: z.array(z.string().min(1)),
  forbiddenBodyFields: z.array(z.string().min(1)),
  expectedToolShape: z.enum(['openai-function', 'responses-function', 'anthropic-input-schema'])
})

const GoProviderCacheAccounting = z.object({
  rawPayloadParsedCaseIds: z.array(z.string().min(1)).length(5),
  rawTelemetrySupportedCaseIds: z.array(z.string().min(1)).length(4),
  rawMatchesExpectedUsageCaseIds: z.array(z.string().min(1)).length(5),
  telemetrySupportedCaseIds: z.array(z.string().min(1)).min(1),
  unsupportedUnknownCaseIds: z.array(z.string().min(1)),
  deepseekCaseIds: z.array(z.string().min(1)).min(1),
  openaiCacheCaseIds: z.array(z.string().min(1)).min(1),
  anthropicCacheCaseIds: z.array(z.string().min(1)).min(1),
  totalCacheHitTokens: z.number().int().nonnegative(),
  totalCacheMissTokens: z.number().int().nonnegative(),
  aggregateCacheHitRate: z.number().nonnegative(),
  unsupportedProvidersCountedAsMisses: z.literal(false)
})

const GoProviderReleaseGuardOutput = z.object({
  fixtureOnly: z.literal(true),
  status: z.enum(['pass', 'fail']),
  lowTailCases: z.number().int().nonnegative(),
  maxLowTailCases: z.number().int().nonnegative(),
  thresholdPercent: z.number().positive(),
  tailWindow: z.number().int().positive(),
  cases: z.array(z.object({
    id: z.string().min(1),
    tailAveragePercent: z.number().nonnegative(),
    status: z.enum(['pass', 'fail']),
    collapseCount: z.number().int().nonnegative(),
    compactionGuardPaused: z.boolean()
  })).min(1)
})

const GoProviderUsageParserOutput = z.object({
  caseCount: z.literal(5),
  telemetrySupportedCaseIds: z.array(z.string().min(1)).length(4),
  unsupportedAbsentFields: z.object({
    caseId: z.literal('unsupported-openai-compatible'),
    absentFields: z.array(z.enum(['cacheHitTokens', 'cacheMissTokens'])).length(2),
    cacheHitRateKnown: z.literal(false)
  }),
  deepseekNativePrecedence: z.object({
    caseId: z.literal('deepseek-native-cache-precedence'),
    promptCacheHitTokens: z.literal(700),
    promptCacheMissTokens: z.literal(300),
    promptTokensDetailsCachedTokens: z.literal(999),
    expectedCacheHitTokens: z.literal(700),
    expectedCacheMissTokens: z.literal(300),
    nativeCacheFieldsWin: z.literal(true),
    cacheHitRate: z.literal(0.7)
  }),
  openaiResponsesCachedTokens: z.object({
    caseId: z.literal('openai-responses-cached-tokens'),
    inputTokens: z.literal(400),
    cachedTokens: z.literal(300),
    expectedCacheHitTokens: z.literal(300),
    expectedCacheMissTokens: z.literal(100),
    reasoningTokens: z.literal(11),
    cachedTokensUsed: z.literal(true)
  }),
  anthropicCacheFields: z.object({
    caseId: z.literal('anthropic-cache-fields'),
    inputTokens: z.literal(50),
    cacheReadInputTokens: z.literal(1000),
    cacheCreationInputTokens: z.literal(200),
    expectedPromptTokens: z.literal(1250),
    expectedCacheHitTokens: z.literal(1000),
    expectedCacheMissTokens: z.literal(250),
    cacheFieldsIncludedInPrompt: z.literal(true)
  })
})

const GoProviderUsageParserControlCase = z.object({
  cases: z.array(ProviderUsageContractCase).length(5),
  expected: GoProviderUsageParserOutput
})

const GoProviderReleaseGuardControlCase = z.object({
  fixtureOnly: z.literal(true),
  thresholdPercent: z.literal(90),
  maxLowTailCases: z.literal(0),
  tailWindow: z.literal(4),
  cases: z.array(z.object({
    id: z.string().min(1),
    cacheHitPercentCurve: z.array(z.number().nonnegative()).min(1),
    expectedTailAveragePercent: z.number().nonnegative(),
    expectedStatus: z.enum(['pass', 'fail']),
    compactionGuardPaused: z.boolean().optional(),
    maxAllowedCollapses: z.number().int().nonnegative().optional()
  })).min(1),
  expected: GoProviderReleaseGuardOutput
})

const GoProviderCacheDriftAttribution = z.object({
  previousShape: CacheShapeExpectation,
  currentShape: CacheShapeExpectation,
  usage: UsageExpectation,
  expectedReasons: z.array(z.enum(['tools', 'provider', 'model'])).length(3),
  expectedTelemetrySupported: z.literal(false)
})

const GoProviderCacheDriftAttributionOutput = z.object({
  previousPrefixHash: z.string().min(1),
  currentPrefixHash: z.string().min(1),
  systemHashStable: z.literal(true),
  prefixItemsHashStable: z.literal(true),
  toolsHashChanged: z.literal(true),
  providerChanged: z.literal(true),
  modelChanged: z.literal(true),
  endpointFormatChanged: z.literal(true),
  expectedReasons: z.array(z.enum(['tools', 'provider', 'model'])).length(3),
  telemetrySupported: z.literal(false),
  cacheHitRateKnown: z.literal(false)
})

const GoProviderCacheDriftAttributionControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  driftAttribution: GoProviderCacheDriftAttribution,
  expected: GoProviderCacheDriftAttributionOutput
})

const GoProviderRequestShapeSummaryOutput = z.object({
  caseCount: z.literal(7),
  exactUrlCount: z.literal(7),
  derivedUrlMatchCaseIds: z.array(z.string().min(1)).length(7),
  headerShapeMatchCaseIds: z.array(z.string().min(1)).length(7),
  bodyShapeMatchCaseIds: z.array(z.string().min(1)).length(7),
  toolShapeMatchCaseIds: z.array(z.string().min(1)).length(7),
  matrix: z.array(GoProviderRequestShapeCaseSummary).length(7),
  endpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  fullEndpointCaseIds: z.array(z.string().min(1)).length(3),
  toolShapes: z.array(z.enum([
    'openai-function',
    'responses-function',
    'anthropic-input-schema'
  ])).length(3),
  customFullEndpointExactUrlCaseIds: z.array(z.string().min(1)).length(3),
  customFullEndpointAppendedPathCount: z.literal(0),
  requiredBodyFieldFamilies: z.object({
    messagesFieldCaseCount: z.literal(5),
    inputFieldCaseCount: z.literal(2),
    systemFieldCaseCount: z.literal(2),
    thinkingFieldCaseCount: z.literal(1),
    maxOutputTokensCaseCount: z.literal(2),
    maxTokensCaseCount: z.literal(2)
  }),
  forbiddenBodyFieldFamilies: z.object({
    thinkingForbiddenCaseCount: z.literal(6),
    systemForbiddenCaseCount: z.literal(5),
    inputForbiddenCaseCount: z.literal(5),
    maxOutputTokensForbiddenCaseCount: z.literal(5)
  })
})

const GoProviderRequestShapeControlCase = z.object({
  cases: z.array(ProviderRequestShapeContractCase).length(7),
  expected: GoProviderRequestShapeSummaryOutput
})

const GoProviderLiveLocalHttpContractControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  contract: ProviderLiveLocalHttpContract,
  providerUsageCaseIds: z.array(z.string().min(1)).length(5),
  requestShapeCaseIds: z.array(z.string().min(1)).length(7),
  requestShapeEndpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  expected: ProviderLiveLocalHttpContract
})

const GoGoalPersistenceOffLockControlCase = z.object({
  sourceContractId: z.literal('thread-service-goal-persistence-v1'),
  sourceFiles: z.object({
    threadService: z.literal('packages/runtime/src/services-test-support/thread-service.ts'),
    threadServiceTest: z.literal('packages/runtime/tests/thread-service.test.ts')
  }),
  setGoalOrder: z.array(z.enum(['touchThread', 'persistGoalThread:set', 'goal_updated'])).length(3),
  clearGoalOrder: z.array(z.enum(['touchThread', 'persistGoalThread:clear', 'goal_cleared'])).length(3),
  forbiddenLockSubstrings: z.array(z.string().min(1)).min(1),
  forbiddenLockSubstringsPresent: z.array(z.string().min(1)).length(0),
  warningEvidence: z.object({
    warningEvent: z.literal('[analytix] event=ANALYTIX_GOAL_PERSISTENCE_FAILED'),
    threadId: z.literal('thr_goal_persist__/private/thread-pii-13900000000'),
    originalError: z.literal('disk full: /private/error-pii-13900000001'),
    actionMarker: z.literal('during set'),
    logsPersistenceFailure: z.literal(true),
    surfacesOriginalError: z.literal(true),
    warningIsExactValueFreeEvent: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    statusApprovalLockShared: z.literal(false),
    defaultGoBackendEnabled: z.literal(false),
    rendererVisibleGoRoute: z.literal(false)
  }),
  expected: z.object({
    setGoalPersistsBeforeEvent: z.literal(true),
    clearGoalPersistsBeforeEvent: z.literal(true),
    noForbiddenControllerLock: z.literal(true),
    goalWritesOutsideSharedStatusApprovalLock: z.literal(true),
    persistenceFailureWarns: z.literal(true),
    persistenceFailureSurfaces: z.literal(true),
    warningIsExactValueFreeEvent: z.literal(true),
    warningIncludesThreadId: z.literal(false),
    warningIncludesOriginalError: z.literal(false),
    warningIncludesAction: z.literal(false),
    usesReasonixProtocol: z.literal(false),
    statusApprovalLockShared: z.literal(false),
    defaultGoBackendEnabled: z.literal(false),
    rendererVisibleGoRoute: z.literal(false)
  })
})

const GoToolResultFileImageBoundaryControlCase = z.object({
  sourceContractId: z.literal('tool-result-file-image-boundary-v1'),
  sourceFiles: z.object({
    toolResultImage: z.literal('packages/runtime/src/shared/tool-result-image.ts'),
    toolResultImageTest: z.literal('packages/runtime/src/loop/tool-result-image.test.ts'),
    attachmentStoreTest: z.literal('packages/runtime/tests/attachment-store.test.ts'),
    rendererMapperTest: z.literal('src/renderer/src/agent/analytix-mapper.test.ts')
  }),
  inlineImageKinds: z.array(z.enum(['image', 'computer_screenshot'])).length(2),
  evictedPayloadMarkers: z.object({
    payload: z.literal('HUGE_BASE64_PAYLOAD'),
    preservedMetadata: z.array(z.enum(['computer_screenshot', '1280'])).length(2)
  }),
  capPolicy: z.object({
    historyImageCount: z.literal(4),
    maxKept: z.literal(2),
    evictedCount: z.literal(2),
    newestImageDataBase64: z.literal('IMG_D')
  }),
  attachmentFallback: z.object({
    localFilePath: z.literal('/tmp/picked/shot.png'),
    textFallbackPrefix: z.literal('[Attached image as base64 text]'),
    mimeType: z.literal('image/webp'),
    dimensions: z.literal('1280x720'),
    fallbackBase64: z.literal('YWJj'),
    deepseekV4TextFallback: z.literal(true)
  }),
  generatedFiles: z.object({
    toolName: z.literal('generate_image'),
    attachmentId: z.literal('att_abc'),
    generatedRelativePath: z.literal('.analytix-images/img-1.png'),
    speechRelativePath: z.literal('.analytix-audio/speech.mp3')
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: z.object({
    inlineImageKindsPreserved: z.literal(true),
    evictedBase64Omitted: z.literal(true),
    evictedMetadataPreserved: z.literal(true),
    onlyNewestImagesInline: z.literal(true),
    attachmentLocalFilePathPreserved: z.literal(true),
    textFallbackCarriesFilePath: z.literal(true),
    deepseekV4TextFallback: z.literal(true),
    generatedFileMetaLifted: z.literal(true),
    toolAttachmentMetaLifted: z.literal(true),
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  })
})

const GoEventJsonlReplayBoundaryControlCase = z.object({
  sourceContractId: z.literal('event-jsonl-replay-boundary-v1'),
  sourceFiles: z.object({
    fileSessionStore: z.literal('packages/runtime/src/adapters/file/file-session-store.ts'),
    loopTest: z.literal('packages/runtime/tests/loop.test.ts'),
    runtimeEventRecorderTest: z.literal('packages/runtime/tests/runtime-event-recorder.test.ts'),
    fileSessionStoreTest: z.literal('packages/runtime/tests/file-session-store.test.ts')
  }),
  jsonlFiles: z.array(z.enum(['events.jsonl', 'messages.jsonl'])).length(2),
  recorderEvidence: z.object({
    persistsBeforePublish: z.literal(true),
    concurrentSeqsUnique: z.literal(true),
    persistedHighWaterReadOnce: z.literal(true)
  }),
  replayEvidence: z.object({
    appendNewlineTerminated: z.literal(true),
    loadEventsSinceFiltersAndSorts: z.literal(true),
    highestSeqUsesMax: z.literal(true),
    malformedJsonlLineSkipped: z.literal(true)
  }),
  usageCompactionEvidence: z.object({
    compactedSeqs: z.array(z.number().int().positive()).length(5),
    highestSeq: z.literal(7),
    failureKeepsAppendedSeqs: z.array(z.number().int().positive()).length(3),
    warningPrefix: z.literal('[analytix] usage event compaction failed')
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: z.object({
    appendNewlineTerminated: z.literal(true),
    loadEventsSinceFiltersAndSorts: z.literal(true),
    highestSeqPreservesMax: z.literal(true),
    malformedJsonlLineSkipped: z.literal(true),
    persistsBeforePublish: z.literal(true),
    concurrentSeqsUnique: z.literal(true),
    persistedHighWaterReadOnce: z.literal(true),
    usageCompactionKeepsCarryover: z.literal(true),
    compactionFailureKeepsAppendOnlyLog: z.literal(true),
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  })
})

const GoMcpMalformedSchemaBoundaryControlCase = z.object({
  sourceContractId: z.literal('mcp-malformed-schema-boundary-v1'),
  sourceFiles: z.object({
    mcpToolProvider: z.literal('packages/runtime/src/tool-test-support/tool/mcp-tool-provider.ts'),
    mcpToolProviderTest: z.literal('packages/runtime/tests/mcp-tool-provider.test.ts')
  }),
  malformedTools: z.object({
    nonObjectSchemaToolName: z.literal('mcp_github_bad_schema'),
    malformedRequiredToolName: z.literal('mcp_github_bad_required'),
    rawNonObjectSchemaKind: z.literal('array'),
    rawRequiredMixedCount: z.literal(3)
  }),
  normalizedSchemas: z.object({
    badSchema: z.object({
      type: z.literal('object'),
      propertiesEmpty: z.literal(true),
      additionalProperties: z.literal(true)
    }),
    badRequired: z.object({
      type: z.literal('object'),
      required: z.array(z.literal('query')).length(1),
      propertiesDropped: z.literal(true)
    })
  }),
  normalizerEvidence: z.object({
    nonRecordDefaults: z.literal(true),
    nonObjectTypeDefaults: z.literal(true),
    propertiesMustBeRecord: z.literal(true),
    requiredFiltersStrings: z.literal(true),
    outputSchemaRecordOnly: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: z.object({
    nonObjectSchemaDefaults: z.literal(true),
    propertiesArrayDropped: z.literal(true),
    requiredNonStringsDropped: z.literal(true),
    advertisedToolNamesPreserved: z.literal(true),
    modelCatalogSchemaSafe: z.literal(true),
    outputSchemaNonRecordOmitted: z.literal(true),
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  })
})

const GoProviderCacheCoverageFloorOutput = z.object({
  requiredProviderFamilies: z.array(ProviderCacheCoverageFamily).length(5),
  coveredProviderFamilies: z.array(ProviderCacheCoverageFamily).length(5),
  providerFamilyCoverageComplete: z.literal(true),
  providerUsageCaseIds: z.array(z.string().min(1)).length(5),
  requestShapeCaseIds: z.array(z.string().min(1)).length(7),
  requestShapeEndpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  telemetrySupportedCaseIds: z.array(z.string().min(1)).length(4),
  unsupportedUnknownCaseIds: z.array(z.string().min(1)).length(1),
  customFullEndpointCaseIds: z.array(z.string().min(1)).length(3),
  customFullEndpointExactUrlCaseIds: z.array(z.string().min(1)).length(3),
  customFullEndpointToolShapes: z.array(z.enum([
    'responses-function',
    'anthropic-input-schema',
    'openai-function'
  ])).length(3),
  customFullEndpointTelemetryCaseIds: z.array(z.string().min(1)).length(0),
  customFullEndpointRequestShapeOnly: z.literal(true),
  telemetrySupportedExcludesCustomFullEndpoints: z.literal(true),
  customProviderCacheTelemetryClaimAllowed: z.literal(false),
  deepseekUsageCaseIds: z.array(z.string().min(1)).length(2),
  deepseekRequestShapeCaseIds: z.array(z.string().min(1)).length(1),
  openaiCompatibleChatRequestShapeCaseIds: z.array(z.string().min(1)).length(1),
  openaiResponsesUsageCaseIds: z.array(z.string().min(1)).length(1),
  openaiResponsesRequestShapeCaseIds: z.array(z.string().min(1)).length(1),
  anthropicMessagesUsageCaseIds: z.array(z.string().min(1)).length(1),
  anthropicMessagesRequestShapeCaseIds: z.array(z.string().min(1)).length(1),
  liveCredentialsUsed: z.literal(false),
  mayClaimLiveSuperiority: z.literal(false),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoProviderCacheCoverageFloorControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  providerUsageCases: z.array(ProviderUsageAccountingCase).length(5),
  requestShapeCases: z.array(ProviderRequestShapeCoverageCase).length(7),
  liveLocalHttpContract: ProviderLiveLocalHttpContract,
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoProviderCacheCoverageFloorOutput
})

const GoProviderCacheAccountingControlCase = z.object({
  cases: z.array(ProviderUsageAccountingCase).length(5),
  expected: GoProviderCacheAccounting
})

const GoProviderOfflineParitySealOutput = z.object({
  fixtureOnly: z.literal(true),
  mayClaimLiveSuperiority: z.literal(false),
  stablePrefixEquivalent: z.literal(true),
  prefixItemsHashStable: z.literal(true),
  toolsHashStable: z.literal(true),
  stablePrefixHash: z.string().min(1),
  equivalentPrefixHash: z.string().min(1),
  deepseekProviderId: z.literal('deepseek-default'),
  deepseekEndpointFormat: z.literal('chat_completions'),
  deepseekModel: z.string().min(1),
  deepseekStableCacheHitTokens: z.number().int().positive(),
  deepseekStableCacheMissTokens: z.number().int().positive(),
  deepseekStableCacheHitRate: z.number().positive(),
  diagnosticsPrefixChanged: z.literal(false),
  diagnosticsTelemetrySupported: z.literal(true),
  providerUsageCaseCount: z.literal(5),
  requestShapeCaseCount: z.literal(7),
  requestShapeEndpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  releaseGuardStatus: z.literal('fail'),
  releaseGuardThresholdPercent: z.literal(90),
  releaseGuardTailWindow: z.literal(4)
})

const GoProviderOfflineParitySealControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  stablePrefix: ProviderCacheContract.shape.stablePrefix,
  providerUsageCaseIds: z.array(z.string().min(1)).length(5),
  requestShapeCaseIds: z.array(z.string().min(1)).length(7),
  requestShapeEndpointFormats: z.array(z.enum([
    'chat_completions',
    'responses',
    'messages',
    'custom_endpoint'
  ])).length(4),
  releaseGuard: z.object({
    status: z.literal('fail'),
    thresholdPercent: z.literal(90),
    tailWindow: z.literal(4)
  }),
  liveCredentialPolicy: ProviderCacheContract.shape.liveCredentialPolicy,
  expected: GoProviderOfflineParitySealOutput
})

const GoProviderCachePrivacyOutput = z.object({
  fixtureOnly: z.literal(true),
  diagnosticCheckedFieldCount: z.number().int().positive(),
  forbiddenDiagnosticsSubstringCount: z.number().int().positive(),
  diagnosticsLeakForbiddenSubstrings: z.literal(false),
  liveCredentialsUsed: z.literal(false),
  mayClaimLiveSuperiority: z.literal(false),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoProviderCachePrivacyControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  privacy: ProviderCacheContract.shape.privacy,
  diagnostics: z.record(z.string(), z.unknown()),
  liveCredentialPolicy: ProviderCacheContract.shape.liveCredentialPolicy,
  liveLocalHttpContract: ProviderLiveLocalHttpContract,
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoProviderCachePrivacyOutput
})

const GoProviderCacheInventoryOutput = z.object({
  stablePrefixHash: z.string().min(1),
  toolsHash: z.string().min(1),
  providerUsageCaseIds: z.array(z.string().min(1)).length(5),
  requestShapeCaseIds: z.array(z.string().min(1)).length(7),
  providerUsageCaseCount: z.literal(5),
  requestShapeCaseCount: z.literal(7),
  stablePrefixEquivalent: z.literal(true),
  toolsHashStable: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoProviderCacheInventoryControlCase = z.object({
  sourceContractId: z.literal('provider-cache-prefix-matrix-v1'),
  stablePrefix: ProviderCacheContract.shape.stablePrefix,
  providerUsageCaseIds: z.array(z.string().min(1)).length(5),
  requestShapeCaseIds: z.array(z.string().min(1)).length(7),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoProviderCacheInventoryOutput
})

const GoProviderStreamingReplayOutput = z.object({
  sourceContractId: z.literal('go-g3-provider-streaming-usage-cache-contract-v1'),
  threadId: z.literal('thr_go_g3_stream'),
  sinceSeq: z.literal(2),
  sseFrameCount: z.literal(3),
  streamingKinds: z.array(z.enum(['item_delta', 'usage', 'turn_completed'])).length(3),
  expectedKindsInOrder: z.array(z.enum(['item_delta', 'usage', 'turn_completed'])).length(3),
  expectedUsageCaseId: z.literal('deepseek-prompt-cache'),
  usageEventMatchesExpectedCase: z.literal(true),
  usagePromptTokens: z.literal(1000),
  usageCompletionTokens: z.literal(10),
  usageReasoningTokens: z.literal(9),
  usageTotalTokens: z.literal(1010),
  usageCacheHitTokens: z.literal(930),
  usageCacheMissTokens: z.literal(70),
  usageCacheHitRate: z.literal(0.93),
  cacheTelemetrySupported: z.literal(true),
  productBoundary: ShadowOnlyProductBoundary
})

const GoProviderStreamingControlCase = z.object({
  sourceContractId: z.literal('go-g3-provider-streaming-usage-cache-contract-v1'),
  providerUsageMatrix: z.array(GoProviderUsageCaseSummary).length(5),
  streaming: z.object({
    threadId: z.literal('thr_go_g3_stream'),
    sinceSeq: z.literal(2),
    sseFrames: z.array(z.string().min(1)).length(3),
    expectedKindsInOrder: z.array(z.enum(['item_delta', 'usage', 'turn_completed'])).length(3),
    expectedUsageCaseId: z.literal('deepseek-prompt-cache')
  }),
  productBoundary: ShadowOnlyProductBoundary,
  expected: GoProviderStreamingReplayOutput
})

const GoG2RouteReplayCase = z.object({
  id: z.string().min(1),
  setup: z.enum([
    'thread-list',
    'thread-archive',
    'thread-search-archive',
    'thread-read',
    'thread-fork',
    'session-resume',
    'events'
  ]),
  method: z.enum(['GET', 'PATCH', 'POST']),
  path: z.string().min(1),
  auth: z.enum(['none', 'runtime-token']),
  responseKind: z.enum(['json', 'sse']),
  body: z.unknown().optional(),
  response: z.object({
    status: z.number().int(),
    body: z.unknown().optional()
  }),
  sseFrames: z.array(z.string().min(1)).optional()
})

const GoSessionRouteStatusReplayOutput = z.object({
  routeCount: z.literal(11),
  jsonRouteCount: z.literal(9),
  sseRouteCount: z.literal(2),
  statusCodes: z.object({
    ok: z.literal(8),
    created: z.literal(2),
    unauthorized: z.literal(1)
  }),
  auth: z.object({
    protectedRouteId: z.literal('events-unauthorized-since-seq'),
    protectedPath: z.literal('/v1/threads/thr_g2_events/events?since_seq=0'),
    protectedStatus: z.literal(401),
    protectedBodyCode: z.literal('unauthorized'),
    sseFrameCount: z.literal(0)
  }),
  archive: z.object({
    patchStatus: z.literal(200),
    archivedStatus: z.literal('archived'),
    archivedOnlyCount: z.literal(1),
    searchArchivedCount: z.literal(1),
    searchFirstStatus: z.literal('archived')
  }),
  readUpdate: z.object({
    readStatus: z.literal(200),
    readLatestSeq: z.literal(2),
    readTurnCount: z.literal(1),
    updateStatus: z.literal(200),
    updatedWorkspace: z.literal('/tmp/read-updated')
  }),
  fork: z.object({
    status: z.literal(201),
    relation: z.literal('side'),
    parentThreadId: z.literal('thr_g2_parent'),
    forkedFromTurnCount: z.literal(1),
    forkedTurnCount: z.literal(1)
  }),
  resume: z.object({
    status: z.literal(201),
    sessionId: z.literal('thr_g2_source'),
    messageCount: z.literal(1),
    summary: z.literal('Source Thread resumed')
  }),
  sse: z.object({
    replayStatus: z.literal(200),
    replayFrameCount: z.literal(2),
    caughtUpStatus: z.literal(200),
    caughtUpFrameCount: z.literal(0),
    replayEventNames: z.array(z.enum(['turn_started', 'item_created'])).length(2)
  })
})

const GoSessionRouteStatusControlCase = z.object({
  sourceContractId: z.literal('go-g2-route-replay-contract-v1'),
  routes: z.array(GoG2RouteReplayCase).length(11),
  expected: GoSessionRouteStatusReplayOutput
})

const GoSessionRouteInventoryRoute = GoG2RouteReplayCase.pick({
  id: true,
  setup: true,
  path: true,
  auth: true,
  responseKind: true
})

const GoSessionRouteInventoryOutput = z.object({
  routeIds: z.array(z.string().min(1)).length(11),
  jsonRouteIds: z.array(z.string().min(1)).length(9),
  sseRouteIds: z.array(z.string().min(1)).length(2),
  eventRouteIds: z.array(z.string().min(1)).length(3),
  resumeRouteIds: z.array(z.string().min(1)).length(1),
  forkRouteIds: z.array(z.string().min(1)).length(1),
  archiveRouteIds: z.array(z.string().min(1)).length(3),
  searchRouteIds: z.array(z.string().min(1)).length(2),
  readUpdateRouteIds: z.array(z.string().min(1)).length(2),
  runtimeTokenRouteCount: z.literal(10),
  unauthorizedRouteIds: z.array(z.literal('events-unauthorized-since-seq')).length(1),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoSessionRouteInventoryControlCase = z.object({
  sourceContractId: z.literal('go-g2-route-replay-contract-v1'),
  routes: z.array(GoSessionRouteInventoryRoute).length(11),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoSessionRouteInventoryOutput
})

const GoSessionRouteReplayRow = z.object({
  id: z.string().min(1),
  method: z.enum(['GET', 'PATCH', 'POST']),
  path: z.string().min(1),
  setup: z.enum([
    'thread-list',
    'thread-archive',
    'thread-search-archive',
    'thread-read',
    'thread-fork',
    'session-resume',
    'events'
  ]),
  auth: z.enum(['none', 'runtime-token']),
  responseKind: z.enum(['json', 'sse']),
  status: z.number().int().positive(),
  requestBodyHash: z.string(),
  responseBodyShape: z.enum(['none', 'object', 'array']),
  responseBodyHash: z.string(),
  sseFrameCount: z.number().int().nonnegative(),
  sseEventNames: z.array(z.string()),
  sseFramesHash: z.string()
})

const GoSessionRouteReplayBoundary = z.object({
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false),
  rendererVisibleGoRoute: z.literal(false),
  defaultGoBackend: z.literal(false)
})

const GoSessionRouteReplayOutput = z.object({
  routeCount: z.literal(11),
  matrix: z.array(GoSessionRouteReplayRow).length(11),
  exactJsonBodyRouteCount: z.literal(9),
  exactSseRouteCount: z.literal(2),
  runtimeTokenRouteCount: z.literal(10),
  unauthorizedRouteIds: z.array(z.literal('events-unauthorized-since-seq')).length(1),
  archiveResponseHash: z.string().length(16),
  searchResponseHash: z.string().length(16),
  forkResponseHash: z.string().length(16),
  resumeResponseHash: z.string().length(16),
  replaySseHash: z.string().length(16),
  caughtUpSseHash: z.literal(''),
  unauthorizedBodyHash: z.string().length(16),
  everyRouteHasMethodPathStatus: z.literal(true),
  jsonRoutesHaveBodyHash: z.literal(true),
  sseRoutesHaveExactFrames: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false),
  rendererVisibleGoRoute: z.literal(false),
  defaultGoBackend: z.literal(false)
})

const GoSessionRouteReplayControlCase = z.object({
  sourceContractId: z.literal('go-g2-route-replay-contract-v1'),
  routes: z.array(GoSessionRouteReplayRow).length(11),
  productBoundary: GoSessionRouteReplayBoundary,
  expected: GoSessionRouteReplayOutput
})

export const GoG3ProviderStreamingUsageCacheContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  stage: z.literal('G3'),
  mode: z.literal('shadow-conformance-only'),
  sourceContractIds: z.array(z.string().min(1)),
  productBoundary: ShadowOnlyProductBoundary,
  providerUsageMatrix: z.array(GoProviderUsageCaseSummary),
  requestShapeCaseIds: z.array(z.string().min(1)),
  requestShapeMatrix: z.array(GoProviderRequestShapeCaseSummary).min(1),
  streaming: z.object({
    threadId: z.string().min(1),
    sinceSeq: z.number().int().nonnegative(),
    sseFrames: z.array(z.string().min(1)),
    expectedKindsInOrder: z.array(z.string().min(1)),
    expectedUsageCaseId: z.string().min(1)
  }),
  cacheDiagnostics: z.object({
    prefixHash: z.string().min(1),
    systemHash: z.string().min(1),
    prefixItemsHash: z.string().min(1),
    toolsHash: z.string().min(1),
    toolSchemaTokens: z.number().int().nonnegative(),
    provider: z.string().min(1),
    providerId: z.string().min(1),
    endpointFormat: z.string().min(1),
    model: z.string().min(1),
    sanitizedRequestUrl: z.string().url(),
    forbiddenDiagnosticsSubstrings: z.array(z.string().min(1))
  }),
  cacheDriftAttribution: GoProviderCacheDriftAttribution,
  cacheAccounting: GoProviderCacheAccounting,
  expectedOutput: z.object({
    stage: z.literal('G3'),
    mode: z.literal('shadow-conformance-only'),
    sourceContractIds: z.array(z.string().min(1)),
    providerUsageCaseIds: z.array(z.string().min(1)),
    requestShapeCaseIds: z.array(z.string().min(1)),
    requestShapeSummary: GoProviderRequestShapeSummaryOutput,
    streamingKinds: z.array(z.string().min(1)),
    cacheTelemetrySupported: z.literal(true),
    cacheDriftAttribution: GoProviderCacheDriftAttributionOutput,
    cacheAccounting: GoProviderCacheAccounting,
    productBoundary: ShadowOnlyProductBoundary
  })
})
export type GoG3ProviderStreamingUsageCacheContract = z.infer<typeof GoG3ProviderStreamingUsageCacheContract>

const McpApprovalAnnotationsContract = z.object({
  serverId: z.string().min(1),
  toolName: z.string().min(1),
  normalizedToolName: z.string().min(1),
  annotations: z.object({
    destructiveHint: z.literal(true),
    openWorldHint: z.literal(true).optional()
  }),
  approvalId: z.string().min(1),
  decision: z.literal('deny'),
  resultKind: z.literal('approval'),
  executed: z.literal(false)
})

const McpSearchRefreshDriftContract = z.object({
  serverId: z.string().min(1),
  initialToolNames: z.array(z.string().min(1)).min(1),
  expandedToolNames: z.array(z.string().min(1)).min(1),
  expectedTotalIndexed: z.number().int().positive(),
  expectedCatalogDrift: z.literal(true)
})

const McpSearchWorkspaceBoundaryContract = z.object({
  trustedWorkspace: z.string().min(1),
  untrustedWorkspace: z.string().min(1),
  query: z.string().min(1),
  trustedToolId: z.string().min(1),
  untrustedSearchedTools: z.literal(0),
  unknownToolError: z.string().min(1),
  callPolicy: z.literal('on-request'),
  deniedCallExecuted: z.literal(false)
})

const McpSearchMetaToolsContract = z.object({
  toolNames: z.array(z.enum(['mcp_search', 'mcp_describe', 'mcp_call', 'mcp_refresh_catalog'])).length(4),
  trustedWorkspace: z.string().min(1),
  untrustedWorkspace: z.string().min(1),
  trustedToolId: z.string().min(1),
  query: z.string().min(1),
  untrustedSearchedTools: McpSearchWorkspaceBoundaryContract.shape.untrustedSearchedTools,
  unknownToolError: McpSearchWorkspaceBoundaryContract.shape.unknownToolError,
  callPolicy: McpSearchWorkspaceBoundaryContract.shape.callPolicy,
  deniedCallExecuted: McpSearchWorkspaceBoundaryContract.shape.deniedCallExecuted,
  refreshDrift: McpSearchRefreshDriftContract
})

const McpBackgroundReconnectContract = z.object({
  failedServerIds: z.array(z.string().min(1)).min(1),
  suspendedProviderId: z.string().min(1),
  suspendedReason: z.string().min(1),
  expectedConnectedServerIds: z.array(z.string().min(1)),
  expectedErrorServerIds: z.array(z.string().min(1)),
  attemptsPerFailedServer: z.number().int().positive(),
  requiresRuntimeRestart: z.literal(false)
})

const McpKnownOverrideVariantContract = z.object({
  serverId: z.string().min(1),
  explicitCwd: z.string().min(1).optional(),
  workspaceRoot: z.string().min(1),
  daemonIdleTimeoutMs: z.string().min(1).optional(),
  diagnostic: z.object({
    knownOverride: z.enum(['codegraph', 'codebase-memory']),
    effectiveCwd: z.string().min(1),
    lowPriority: z.literal(true),
    backgroundStart: z.literal(true)
  })
})

const McpLiveLocalIndexerContract = z.object({
  serverId: z.string().min(1),
  cwd: z.string().min(1),
  lowPriority: z.literal(true),
  backgroundStart: z.literal(true),
  failedServerIds: z.array(z.string().min(1)).min(1),
  attemptsPerFailedServer: z.number().int().positive(),
  initialFiles: z.array(z.object({
    path: z.string().min(1),
    digest: z.string().min(1)
  })).min(1),
  lateTombstonePath: z.string().min(1),
  resumeFiles: z.array(z.object({
    path: z.string().min(1),
    digest: z.string().min(1)
  })).min(1),
  secretDiagnostic: z.string().min(1),
  expectedOutput: z.object({
    serverId: z.string().min(1),
    cwd: z.string().min(1),
    lowPriority: z.literal(true),
    backgroundStart: z.literal(true),
    retryAttempts: z.record(z.string(), z.number().int().positive()),
    activePaths: z.array(z.string().min(1)),
    tombstoneCount: z.number().int().nonnegative(),
    restartedFromSnapshot: z.literal(true),
    secretSafeDiagnostic: z.string().min(1)
  }),
  executionError: z.object({
    toolName: z.string().min(1),
    isError: z.literal(true),
    secretSafeError: z.string().min(1),
    leaksSecret: z.literal(false)
  })
})

const McpCallReconnectContract = z.object({
  serverId: z.string().min(1),
  toolName: z.string().min(1),
  normalizedToolName: z.string().min(1),
  transportError: z.string().min(1),
  protocolError: z.string().min(1),
  retryOnTransportError: z.literal(true),
  retryOnProtocolError: z.literal(false),
  maxAttempts: z.literal(2),
  staleConnection: z.object({
    factoryAttempts: z.literal(2),
    closeCount: z.literal(1),
    resultInstance: z.literal(2),
    isError: z.literal(false)
  }),
  protocolFailure: z.object({
    factoryAttempts: z.literal(1),
    closeCount: z.literal(0),
    code: z.literal('tool_execution_failed'),
    isError: z.literal(true)
  })
})

export const McpToolLifecycleContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  providerId: z.string().min(1),
  connect: z.object({
    toolNames: z.array(z.string().min(1)),
    diagnostic: z.object({
      available: z.literal(true),
      toolCount: z.number().int().nonnegative()
    })
  }),
  disconnect: z.object({
    reason: z.string().min(1),
    toolNames: z.array(z.string()),
    diagnostic: z.object({
      available: z.literal(false),
      toolCount: z.number().int().nonnegative()
    })
  }),
  reload: z.object({
    toolNames: z.array(z.string().min(1)),
    schemaOrderStable: z.literal(true)
  }),
  cancel: z.object({
    errorSubstring: z.string().min(1),
    executed: z.literal(false)
  }),
  error: z.object({
    code: z.literal('tool_execution_failed'),
    approved: z.literal(true)
  }),
  callReconnect: McpCallReconnectContract,
  approvalAnnotations: McpApprovalAnnotationsContract,
  searchMetaTools: McpSearchMetaToolsContract,
  diagnosticsRedaction: z.object({
    secret: z.string().min(1),
    replacement: z.string().min(1)
  }),
  knownOverride: z.object({
    serverId: z.string().min(1),
    explicitCwd: z.string().min(1),
    workspaceRoot: z.string().min(1),
    daemonIdleTimeoutMs: z.string().min(1),
    diagnostic: z.object({
      knownOverride: z.enum(['codegraph', 'codebase-memory']),
      effectiveCwd: z.string().min(1),
      lowPriority: z.literal(true),
      backgroundStart: z.literal(true)
    })
  }),
  knownOverrideVariants: z.array(McpKnownOverrideVariantContract).min(1),
  backgroundReconnect: McpBackgroundReconnectContract,
  liveLocalIndexer: McpLiveLocalIndexerContract
})
export type McpToolLifecycleContract = z.infer<typeof McpToolLifecycleContract>

const GoMcpCoreLifecycleOutput = z.object({
  providerId: z.literal('mcp:research'),
  connectToolNames: z.array(z.string().min(1)).min(1),
  connectAvailable: z.literal(true),
  connectToolCount: z.number().int().positive(),
  disconnectReason: z.string().min(1),
  disconnectToolNames: z.array(z.string()),
  disconnectAvailable: z.literal(false),
  disconnectToolCount: z.number().int().positive(),
  reloadToolNames: z.array(z.string().min(1)).min(2),
  schemaOrderStable: z.literal(true),
  cancelErrorSubstring: z.string().min(1),
  cancelExecuted: z.literal(false),
  errorCode: z.literal('tool_execution_failed'),
  errorApproved: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoMcpCoreLifecycleControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  providerId: z.literal('mcp:research'),
  coreLifecycle: McpToolLifecycleContract.pick({
    connect: true,
    disconnect: true,
    reload: true,
    cancel: true,
    error: true
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoMcpCoreLifecycleOutput
})

const GoMcpBackgroundReconnectOutput = z.object({
  failedServerIds: z.array(z.string().min(1)).min(1),
  suspendedProviderId: z.string().min(1),
  suspendedReason: z.string().min(1),
  connectedServerIds: z.array(z.string().min(1)),
  errorServerIds: z.array(z.string().min(1)),
  attemptsPerFailedServer: z.number().int().positive(),
  retryAllFailedServers: z.literal(true),
  requiresRuntimeRestart: z.literal(false)
})

const GoMcpBackgroundReconnectControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  backgroundReconnect: McpBackgroundReconnectContract,
  expected: GoMcpBackgroundReconnectOutput
})

const GoMcpCallReconnectOutput = z.object({
  serverId: z.string().min(1),
  toolName: z.string().min(1),
  normalizedToolName: z.string().min(1),
  transportErrorRetried: z.literal(true),
  protocolErrorRetried: z.literal(false),
  maxAttempts: z.literal(2),
  staleFactoryAttempts: z.literal(2),
  staleCloseCount: z.literal(1),
  staleResultInstance: z.literal(2),
  staleCallSucceeded: z.literal(true),
  protocolFactoryAttempts: z.literal(1),
  protocolCloseCount: z.literal(0),
  protocolErrorCode: z.literal('tool_execution_failed'),
  protocolCallReturnedError: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoMcpCallReconnectControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  callReconnect: McpCallReconnectContract,
  expected: GoMcpCallReconnectOutput
})

const GoMcpKnownOverrideDiagnosticRow = z.object({
  serverId: z.string().min(1),
  knownOverride: z.enum(['codegraph', 'codebase-memory']),
  effectiveCwd: z.string().min(1),
  lowPriority: z.literal(true),
  backgroundStart: z.literal(true),
  workspaceRoot: z.string().min(1),
  explicitCwd: z.string().min(1).optional(),
  daemonIdleTimeoutMs: z.string().min(1).optional()
})

const GoMcpKnownOverrideDiagnosticsOutput = z.object({
  diagnostics: z.array(GoMcpKnownOverrideDiagnosticRow).min(1),
  variantCount: z.number().int().positive(),
  knownOverrideKinds: z.array(z.enum(['codegraph', 'codebase-memory'])).min(1),
  workspaceRoots: z.array(z.string().min(1)).min(1),
  explicitCwdServerIds: z.array(z.string().min(1)),
  daemonIdleTimeoutServerIds: z.array(z.string().min(1)),
  allLowPriority: z.literal(true),
  allBackgroundStart: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoMcpKnownOverrideDiagnosticsControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  knownOverrideVariants: z.array(McpKnownOverrideVariantContract).min(1),
  expected: GoMcpKnownOverrideDiagnosticsOutput
})

const GoMcpLiveLocalIndexerOutput = z.object({
  serverId: z.string().min(1),
  cwd: z.string().min(1),
  lowPriority: z.literal(true),
  backgroundStart: z.literal(true),
  retryServerIds: z.array(z.string().min(1)).min(1),
  attemptsPerFailedServer: z.number().int().positive(),
  retryAttempts: z.record(z.string(), z.number().int().positive()),
  initialPaths: z.array(z.string().min(1)).min(1),
  resumePaths: z.array(z.string().min(1)).min(1),
  activePaths: z.array(z.string().min(1)).min(1),
  tombstoneCount: z.number().int().positive(),
  restartedFromSnapshot: z.literal(true),
  lateTombstonePath: z.string().min(1),
  secretSafeDiagnostic: z.string().min(1),
  leaksSecret: z.literal(false),
  executionErrorToolName: z.string().min(1),
  executionErrorIsError: z.literal(true),
  executionErrorSafe: z.string().min(1),
  executionErrorLeaksSecret: z.literal(false),
  topLevelRouteExposed: z.literal(false),
  usesReasonixProtocol: z.literal(false)
})

const GoMcpLiveLocalIndexerControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  liveLocalIndexer: McpLiveLocalIndexerContract,
  expected: GoMcpLiveLocalIndexerOutput
})

const GoMcpApprovalAnnotationOutput = z.object({
  serverId: z.string().min(1),
  toolName: z.string().min(1),
  normalizedToolName: z.string().min(1),
  destructiveHint: z.literal(true),
  openWorldHint: z.literal(true),
  approvalId: z.string().min(1),
  decision: z.literal('deny'),
  resultKind: z.literal('approval'),
  executed: z.literal(false),
  deniedNoExecute: z.literal(true)
})

const GoMcpApprovalAnnotationControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  approvalAnnotations: McpApprovalAnnotationsContract,
  expected: GoMcpApprovalAnnotationOutput
})

const GoMcpSearchMetaToolsOutput = z.object({
  toolNames: z.array(z.enum(['mcp_search', 'mcp_describe', 'mcp_call', 'mcp_refresh_catalog'])).length(4),
  toolCount: z.literal(4),
  refreshToolAdvertised: z.literal(true),
  trustedWorkspace: z.string().min(1),
  untrustedWorkspace: z.string().min(1),
  query: z.string().min(1),
  trustedToolId: z.string().min(1),
  untrustedSearchedTools: z.literal(0),
  unknownToolError: z.string().min(1),
  callPolicy: z.literal('on-request'),
  deniedCallExecuted: z.literal(false),
  deniedNoExecute: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoMcpSearchMetaToolsControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  searchMetaTools: McpSearchMetaToolsContract,
  expected: GoMcpSearchMetaToolsOutput
})

const GoMcpSearchRefreshDriftOutput = z.object({
  serverId: z.string().min(1),
  initialToolNames: z.array(z.string().min(1)).min(1),
  expandedToolNames: z.array(z.string().min(1)).min(1),
  totalIndexed: z.number().int().positive(),
  catalogDrift: z.literal(true),
  topLevelRouteExposed: z.literal(false)
})

const GoMcpSearchRefreshDriftControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  refreshDrift: McpSearchRefreshDriftContract,
  expected: GoMcpSearchRefreshDriftOutput
})

const GoMcpSearchWorkspaceBoundaryOutput = z.object({
  trustedWorkspace: z.string().min(1),
  untrustedWorkspace: z.string().min(1),
  query: z.string().min(1),
  trustedToolId: z.string().min(1),
  untrustedSearchedTools: z.literal(0),
  unknownToolError: z.string().min(1),
  callPolicy: z.literal('on-request'),
  deniedNoExecute: z.literal(true)
})

const GoMcpSearchWorkspaceBoundaryControlCase = z.object({
  sourceContractId: z.literal('mcp-tool-lifecycle-fixture-v1'),
  searchWorkspaceBoundary: McpSearchWorkspaceBoundaryContract,
  expected: GoMcpSearchWorkspaceBoundaryOutput
})

export const TaskJobOrchestrationContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  toolContracts: z.object({
    task: z.object({
      name: z.literal('task'),
      promptField: z.literal('prompt'),
      backgroundField: z.literal('run_in_background'),
      continueField: z.literal('continue_from'),
      forkField: z.literal('fork_from'),
      internalRuntimeOnly: z.literal(true),
      requiresPermissionGate: z.literal(true),
      mayAppendParentGoalEvidence: z.literal(true)
    }),
    parallelTasks: z.object({
      name: z.literal('parallel_tasks'),
      tasksField: z.literal('tasks'),
      dependencyField: z.literal('depends_on'),
      internalRuntimeOnly: z.literal(true),
      requiresDependencyValidation: z.literal(true),
      requiresPlannerReadOnlyToolset: z.literal(true)
    })
  }),
  routeContract: z.object({
    wait: z.string().min(1),
    output: z.string().min(1),
    kill: z.string().min(1),
    forbiddenTopLevelRoutes: z.array(z.string().min(1)),
    authMatrix: z.object({
      protectedRoutes: z.array(z.enum(['wait', 'output', 'kill'])).length(3),
      unauthorizedStatus: z.literal(401)
    })
  }),
  routeExecutable: z.object({
    unauthorizedStatus: z.literal(401),
    output: z.object({
      status: z.literal(200),
      jobStatus: z.literal('running'),
      output: z.string().min(1),
      nextOffset: z.literal(14),
      replayOffset: z.literal(0)
    }),
    wait: z.object({
      status: z.literal(200),
      jobStatus: z.literal('completed'),
      result: z.string().min(1)
    }),
    kill: z.object({
      status: z.literal(200),
      jobStatus: z.literal('killed'),
      error: z.string().min(1)
    }),
    missingOutput: z.object({
      status: z.literal(404)
    }),
    rehydrated: z.object({
      outputStatus: z.literal(200),
      waitStatus: z.literal(200),
      killStatus: z.literal(200),
      completedStatus: z.literal('completed'),
      completedNextOffset: z.literal(29),
      killedStatus: z.literal('killed')
    })
  }),
  foreground: z.object({
    kind: z.literal('task'),
    status: z.literal('completed'),
    result: z.string().min(1)
  }),
  background: z.object({
    kind: z.literal('task'),
    statusAcrossTurn: z.literal('running'),
    output: z.string().min(1),
    finalStatus: z.literal('completed'),
    finalResult: z.string().min(1)
  }),
  waitOutputKill: z.object({
    killedStatus: z.literal('killed'),
    killedError: z.string().min(1)
  }),
  parallel: z.object({
    dependencyField: z.literal('depends_on'),
    validOrder: z.array(z.string().min(1)),
    singleTaskError: z.string().min(1),
    duplicateIdError: z.string().min(1),
    selfDependencyError: z.string().min(1),
    cycleError: z.string().min(1),
    unknownDependencyError: z.string().min(1)
  }),
  plannerExecutor: z.object({
    plannerKind: z.literal('planner'),
    plannerPolicy: z.literal('readOnly'),
    executorPolicy: z.literal('inherit'),
    failureStatus: z.literal('failed'),
    cancelledStatus: z.literal('cancelled'),
    skippedReason: z.string().min(1),
    cancelReason: z.string().min(1),
    outputOffsetJobCount: z.number().int().positive(),
    requiresFailurePropagation: z.literal(true),
    requiresCancellationPropagation: z.literal(true),
    requiresTranscriptPropagation: z.literal(true),
    transcriptPropagationMode: z.enum(['continue', 'fork']),
    transcriptPropagationJobCount: z.number().int().positive()
  }),
  durableRunner: z.object({
    restartDrill: z.object({
      runningJobId: z.string().min(1),
      queuedJobId: z.string().min(1),
      rehydratedCount: z.number().int().positive(),
      outputBeforeRestart: z.string().min(1),
      outputAfterRestart: z.string().min(1),
      waitStatus: z.literal('completed'),
      killStatus: z.literal('killed'),
      killError: z.string().min(1)
    }),
    staleReconcile: z.object({
      runningJobId: z.string().min(1),
      queuedJobId: z.string().min(1),
      reason: z.string().min(1),
      expectedStatus: z.literal('interrupted'),
      reconciledCount: z.literal(2)
    })
  }),
  approvalDenyNoExecute: z.object({
    deniedToolNames: z.array(z.string().min(1)).min(2),
    approvalIds: z.array(z.string().min(1)).min(2),
    createsDurableJobs: z.literal(false),
    createsChildRuns: z.literal(false)
  }),
  nestedEvent: z.object({
    parentCallId: z.string().min(1),
    childRunId: z.string().min(1),
    nestedSseMetadataFields: z.array(z.string().min(1))
  }),
  parentGoalEvidence: z.object({
    requiresActiveGoal: z.literal(true),
    evidenceLedgeredEventKey: z.literal('evidenceLedgered'),
    evidenceLedgerErrorEventKey: z.literal('evidenceLedgerError')
  }),
  permissions: z.object({
    inheritedPolicy: z.literal('inherit'),
    plannerReadOnlyToolset: z.array(z.string().min(1)),
    plannerForbiddenToolset: z.array(z.string().min(1))
  }),
  transcript: z.object({
    sourceId: z.string().min(1),
    continueTargetId: z.string().min(1),
    forkTargetId: z.string().min(1),
    incompatibleError: z.string().min(1),
    sameIdentityRequired: z.literal(true),
    continuePreservesTarget: z.literal(true),
    forkCreatesDistinctTarget: z.literal(true)
  })
})
export type TaskJobOrchestrationContract = z.infer<typeof TaskJobOrchestrationContract>

export const GoG4ToolsApprovalUserInputMcpContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  stage: z.literal('G4'),
  mode: z.literal('shadow-conformance-only'),
  sourceContractIds: z.array(z.string().min(1)),
  productBoundary: ShadowOnlyProductBoundary,
  toolCatalog: z.object({
    advertisedToolNames: z.array(z.string().min(1)),
    canonicalOrderStable: z.literal(true),
    forbiddenTopLevelRoutes: z.array(z.string().min(1))
  }),
  approval: z.object({
    id: z.string().min(1),
    toolName: z.string().min(1),
    decision: z.literal('deny'),
    expectedStatus: z.literal('denied'),
    secondDecisionStatus: z.number().int().positive(),
    pendingBefore: z.number().int().nonnegative(),
    pendingAfter: z.number().int().nonnegative(),
    mustNotExecuteDeniedTool: z.literal(true)
  }),
  userInput: z.object({
    id: z.string().min(1),
    resolution: z.literal('cancelled'),
    expectedStatus: z.literal('cancelled'),
    secondResolveStatus: z.number().int().positive(),
    pendingBefore: z.number().int().nonnegative(),
    pendingAfter: z.number().int().nonnegative(),
    remoteDisableUserInputPreserved: z.literal(true),
    structuredChoiceValidation: UserInputStructuredChoiceValidation,
    submittedRoute: z.object({
      status: z.literal('submitted'),
      answersEchoed: z.literal(true),
      resolvedEventIncludesAnswers: z.literal(false)
    })
  }),
  mcp: z.object({
    providerId: z.string().min(1),
    connectToolNames: z.array(z.string().min(1)),
    reloadToolNames: z.array(z.string().min(1)),
    disconnectReason: z.string().min(1),
    cancel: z.object({
      errorSubstring: z.string().min(1),
      executed: z.literal(false)
    }),
    diagnosticsRedaction: z.object({
      secret: z.string().min(1),
      replacement: z.string().min(1)
    }),
    approvalAnnotations: z.object({
      normalizedToolName: z.string().min(1),
      decision: z.literal('deny'),
      executed: z.literal(false)
    }),
    searchMetaTools: z.object({
      toolNames: z.array(z.enum(['mcp_search', 'mcp_describe', 'mcp_call', 'mcp_refresh_catalog'])).length(4),
      trustedToolId: z.string().min(1),
      untrustedSearchedTools: z.literal(0),
      unknownToolError: z.string().min(1),
      callPolicy: z.literal('on-request'),
      deniedCallExecuted: z.literal(false)
    }),
    knownOverrideDiagnostic: z.object({
      knownOverride: z.enum(['codegraph', 'codebase-memory']),
      effectiveCwd: z.string().min(1),
      lowPriority: z.literal(true),
      backgroundStart: z.literal(true)
    })
  }),
  plannerExecutor: z.object({
    plannerReadOnlyToolset: z.array(z.string().min(1)),
    backgroundJobRoutesInternalOnly: z.literal(true),
    topLevelWorkflowRoutesExposed: z.literal(false)
  }),
  remoteEntryBoundary: z.object({
    expectedPortKeys: z.array(z.string().min(1)),
    forbiddenPortKeys: z.array(z.string().min(1))
  }),
  expectedOutput: z.object({
    stage: z.literal('G4'),
    mode: z.literal('shadow-conformance-only'),
    sourceContractIds: z.array(z.string().min(1)),
    toolNames: z.array(z.string().min(1)),
    approvalDeniedNoExecute: z.literal(true),
    userInputCancelled: z.literal(true),
    userInputValidationCode: z.literal('invalid_user_input_request'),
    userInputValidationCases: z.array(UserInputValidationCaseId).min(4),
    userInputInvalidOpensGate: z.literal(false),
    userInputSubmittedAnswersEchoed: z.literal(true),
    userInputResolvedEventOmitsAnswers: z.literal(true),
      mcpApprovalAnnotatedNoExecute: z.literal(true),
      mcpSearchMetaToolsAdvertised: z.literal(true),
      mcpSearchUntrustedWorkspaceHidden: z.literal(true),
      mcpSearchCallDeniedNoExecute: z.literal(true),
      mcpKnownOverride: z.enum(['codegraph', 'codebase-memory']),
    plannerReadOnlyToolset: z.array(z.string().min(1)),
    productBoundary: ShadowOnlyProductBoundary
  })
})
export type GoG4ToolsApprovalUserInputMcpContract = z.infer<typeof GoG4ToolsApprovalUserInputMcpContract>

const GoMinimalLoopProductBoundary = ShadowOnlyProductBoundary.extend({
  tempDurableStorePrototype: z.literal(true),
  minimalAgentLoopPrototype: z.literal(true),
  fixtureBackedLoopOnly: z.literal(true)
})

const GoMinimalLoopUsageCase = z.object({
  caseId: z.string().min(1),
  provider: z.enum(['deepseek', 'openai', 'anthropic']),
  providerId: z.string().min(1),
  endpointFormat: z.enum(['chat_completions', 'responses', 'messages']),
  model: z.string().min(1),
  cacheHitTokens: z.number().int().nonnegative(),
  cacheMissTokens: z.number().int().nonnegative(),
  cacheHitRate: z.number().nonnegative()
})

const GoMinimalLoopEventDraft = z.record(z.string(), z.unknown()).refine(
  (value) => typeof value.kind === 'string' && typeof value.threadId === 'string',
  'loop event drafts require kind and threadId'
)

export const GoMinimalAgentLoopContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.literal('go-minimal-agent-loop-contract-v1'),
  stage: z.literal('D-0237'),
  mode: z.literal('conformance-only-minimal-agent-loop'),
  runtimeToken: z.string().min(1),
  sourceContractIds: z.array(z.string().min(1)).min(5),
  productBoundary: GoMinimalLoopProductBoundary,
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  cancelThreadId: z.string().min(1),
  resumeThreadId: z.string().min(1),
  userInput: z.object({
    itemId: z.string().min(1),
    role: z.literal('user'),
    text: z.string().min(1),
    singleTurn: z.literal(true)
  }),
  stablePrefix: z.object({
    prefixHash: z.string().min(1),
    systemHash: z.string().min(1),
    prefixItemsHash: z.string().min(1),
    toolsHash: z.string().min(1),
    toolSchemaTokens: z.number().int().nonnegative(),
    toolCatalogFingerprint: z.string().min(1),
    toolNames: z.array(z.string().min(1)).min(1),
    systemPrefixStable: z.literal(true),
    dynamicStateInStablePrefix: z.literal(false)
  }),
  modelRequestShape: z.object({
    caseId: z.literal('deepseek-chat-request-shape'),
    provider: z.string().min(1),
    providerId: z.string().min(1),
    endpointFormat: z.literal('chat_completions'),
    baseUrl: z.string().url(),
    model: z.string().min(1),
    expectedUrl: z.string().url(),
    stream: z.literal(true),
    requiredHeaders: z.array(z.string().min(1)).min(1),
    requiredBodyFields: z.array(z.string().min(1)).min(1),
    forbiddenBodyFields: z.array(z.string().min(1)),
    expectedToolShape: z.literal('openai-function'),
    toolCatalogFingerprint: z.string().min(1),
    stableSystemPrefix: z.literal(true),
    externalNetworkUsed: z.literal(false),
    apiKeyRead: z.literal(false)
  }),
  providerCacheTelemetry: z.object({
    cases: z.array(GoMinimalLoopUsageCase).length(3),
    expectedProviders: z.array(z.enum(['anthropic', 'deepseek', 'openai'])).length(3),
    totalCacheHitTokens: z.number().int().nonnegative(),
    totalCacheMissTokens: z.number().int().nonnegative(),
    cacheTelemetryLost: z.literal(false),
    unsupportedProvidersCountedAsMisses: z.literal(false)
  }),
  approvalDenied: z.object({
    approvalId: z.string().min(1),
    toolName: z.string().min(1),
    decision: z.literal('deny'),
    status: z.literal('denied'),
    mustNotExecuteDeniedTool: z.literal(true)
  }),
  userInputGates: z.object({
    submittedInputId: z.string().min(1),
    cancelledInputId: z.string().min(1),
    pendingStatus: z.literal('pending'),
    submittedStatus: z.literal('submitted'),
    cancelledStatus: z.literal('cancelled'),
    submittedAnswersPersistedInEvents: z.literal(false)
  }),
  mcpToolCatalog: z.object({
    providerId: z.string().min(1),
    toolNames: z.array(z.string().min(1)).min(1),
    fingerprint: z.string().min(1),
    visibleToModel: z.literal(true),
    mcpConnectionUsed: z.literal(false),
    credentialRead: z.literal(false)
  }),
  loopScript: z.object({
    fixtureModelOnly: z.literal(true),
    modelChunkKinds: z.array(z.enum([
      'assistant_reasoning_delta',
      'assistant_text_delta',
      'tool_call_complete',
      'usage',
      'completed'
    ])).min(1),
    toolCall: z.object({
      callId: z.string().min(1),
      toolName: z.string().min(1),
      executed: z.literal(false)
    }),
    toolResult: z.object({
      callId: z.string().min(1),
      status: z.literal('completed'),
      isError: z.literal(true),
      code: z.literal('approval_denied')
    })
  }),
  eventDrafts: z.array(GoMinimalLoopEventDraft).min(1),
  control: z.object({
    stepLimit: z.object({
      maxModelSteps: z.number().int().positive(),
      requestedModelSteps: z.number().int().positive(),
      executedModelSteps: z.number().int().positive(),
      stepLimitHit: z.literal(true),
      errorCode: z.literal('model_step_limit_exceeded'),
      dynamicStateInStablePrefix: z.literal(false)
    }),
    cancel: z.object({
      eventDrafts: z.array(GoMinimalLoopEventDraft).min(1),
      status: z.literal('aborted'),
      resultCode: z.literal('tool_call_cancelled'),
      recordsNoToolExecution: z.literal(true)
    }),
    resume: z.object({
      eventDrafts: z.array(GoMinimalLoopEventDraft).min(1),
      sourceThreadId: z.string().min(1),
      resumedThreadId: z.string().min(1),
      resumesAfterSeq: z.number().int().positive(),
      recoveredStateMustMatch: z.literal(true)
    })
  }),
  expected: z.object({
    eventKinds: z.array(z.string().min(1)).min(1),
    itemKinds: z.array(z.string().min(1)).min(1),
    highestSeq: z.number().int().positive(),
    replayAfterSeq: z.number().int().nonnegative(),
    replayAfterSeqCount: z.number().int().nonnegative(),
    sseFrameCount: z.number().int().positive(),
    caughtUpReplayFrameCount: z.literal(0),
    providers: z.array(z.enum(['anthropic', 'deepseek', 'openai'])).length(3),
    totalCacheHitTokens: z.number().int().nonnegative(),
    totalCacheMissTokens: z.number().int().nonnegative(),
    approvalStatuses: z.array(z.enum(['pending', 'denied'])).length(2),
    userInputStatuses: z.array(z.enum(['pending', 'submitted', 'pending', 'cancelled'])).length(4),
    mcpToolNames: z.array(z.string().min(1)).min(1),
    latestMcpFingerprint: z.string().min(1),
    recoveredStateMatches: z.literal(true),
    sideEffects: z.object({
      providerCallAttempts: z.literal(0),
      toolExecutionAttempts: z.literal(0),
      approvalExecutionAttempts: z.literal(0),
      mcpConnectionAttempts: z.literal(0),
      credentialReadAttempts: z.literal(0),
      fileMutationAttempts: z.literal(0),
      realWorkspaceWriteAttempts: z.literal(0)
    })
  })
})
export type GoMinimalAgentLoopContract = z.infer<typeof GoMinimalAgentLoopContract>

const GoG5ControlReplay = z.object({
  cancel: z.object({
    runningTurnEscapableAfterCancel: z.literal(true),
    toolResultsPairedByCallId: z.literal(true),
    preservesCompletedBatchResults: z.literal(true),
    cancelledResultCode: z.literal('tool_call_cancelled'),
    unstartedResultStatus: z.literal('aborted'),
    stopsSchedulingNewParallelTools: z.literal(true)
  }),
  taskJobs: z.object({
    parentSignalKillsRunningJobs: z.literal(true),
    preservesOutputOffsets: z.literal(true),
    staleRestartReconcileFailsQueuedAndRunning: z.literal(true),
    skippedUnstartedReason: z.literal('cancelled: parent turn aborted before task execution')
  }),
  approvalDeny: z.object({
    deniedTaskToolsReturnApprovalItems: z.literal(true),
    createsDurableJobs: z.literal(false),
    createsChildRuns: z.literal(false)
  }),
  userInput: z.object({
    submittedAnswersEchoedByHttp: z.literal(true),
    resolvedEventOmitsAnswers: z.literal(true),
    cancelledResolutionStatus: z.literal('cancelled'),
    lateResolveRejected: z.literal(true)
  }),
  abortCleanup: z.object({
    approvalStatus: z.literal('expired'),
    userInputStatus: z.literal('cancelled'),
    lateApprovalDecisionStatus: z.literal(409),
    lateUserInputResolveStatus: z.literal(404),
    pendingAfterCleanup: z.literal(0),
    replayKinds: z.array(z.enum([
      'approval_requested',
      'approval_resolved',
      'user_input_requested',
      'user_input_resolved'
    ])).length(4),
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  resumePendingGates: z.object({
    sourceApprovalStatus: z.literal('pending'),
    sourceUserInputStatus: z.literal('pending'),
    resumedApprovalStatus: z.literal('expired'),
    resumedUserInputStatus: z.literal('cancelled'),
    answersCopiedToResume: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  autoResearch: z.object({
    projectLocalState: z.literal(true),
    unknownRequirementRejected: z.literal(true),
    directionTrackingRecorded: z.literal(true),
    directionRequiresActiveResearchGoal: z.literal(true),
    recordResearchDirectionToolPresent: z.literal(true),
    stablePrefixIsolation: z.literal(true),
    topLevelRouteExposed: z.literal(false)
  }),
  mcpLifecycle: z.object({
    retryAllFailedServers: z.literal(true),
    tombstonesPersistAcrossRestart: z.literal(true),
    diagnosticsRedacted: z.literal(true),
    searchMetaToolsAdvertised: z.literal(true),
    searchUntrustedWorkspaceHidden: z.literal(true),
    searchCallDeniedNoExecute: z.literal(true),
    topLevelRouteExposed: z.literal(false)
  }),
  checkpointRewind: z.object({
    checkpointIdPrefix: z.literal('axcp_'),
    planIdPrefix: z.literal('axrp_'),
    applyIdPrefix: z.literal('axra_'),
    rescueIdPrefix: z.literal('axrr_'),
    pathEscapeBlocked: z.literal(true),
    conversationAuditAppendOnly: z.literal(true),
    requiresConfirmationPhrase: z.literal(true),
    topLevelRouteExposed: z.literal(false)
  }),
  remoteEntry: z.object({
    exposesOnlyLifecycleTurnsApprovals: z.literal(true),
    forbiddenControlPlanesOmitted: z.literal(true),
    rejectsPolicyOverrides: z.literal(true),
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  stepLimits: z.object({
    defaultMaxModelSteps: z.literal(64),
    userGlobalOverride: z.literal(true),
    sessionOverride: z.literal(true),
    turnOverride: z.literal(true),
    plannerOverride: z.literal(true),
    headlessOverride: z.literal(true),
    zeroDefaultFallsBackToBoundedLimit: z.literal(true),
    dynamicLimitInStablePrefix: z.literal(false),
    errorCode: z.literal('turn_step_limit_exceeded'),
    delegateTaskInheritsParentHalfMinFive: z.literal(true)
  }),
  planner: z.object({
    readOnlyPlusCreatePlan: z.literal(true),
    forbiddenTaskToolsRejected: z.literal(true),
    rejectionCode: z.literal('tool_dispatch_rejected'),
    createPlanToolName: z.literal('create_plan')
  }),
  combined: z.object({
    autoRouteCacheReusedUntilStepLimit: z.literal(true),
    cancelledResultsPreservedWithStepCache: z.literal(true),
    dynamicControlStateInStablePrefix: z.literal(false),
    stepLimitErrorCode: z.literal('turn_step_limit_exceeded'),
    cancelledResultCode: z.literal('tool_call_cancelled')
  })
})

const GoG5ParallelValidationCaseId = z.enum([
  'single_task',
  'duplicate_id',
  'self_dependency',
  'cycle',
  'unknown_dependency'
])

const GoG5ParallelPlanItem = z.object({
  id: z.string().min(1),
  depends_on: z.array(z.string().min(1)).optional()
})

const GoG5TaskJobRouteExecutableExpected = z.object({
  unauthorizedStatus: z.literal(401),
  outputStatus: z.literal(200),
  outputJobStatus: z.literal('running'),
  outputNextOffset: z.literal(14),
  outputReplayOffset: z.literal(0),
  waitStatus: z.literal(200),
  waitJobStatus: z.literal('completed'),
  killStatus: z.literal(200),
  killJobStatus: z.literal('killed'),
  missingOutputStatus: z.literal(404),
  rehydratedOutputStatus: z.literal(200),
  rehydratedWaitStatus: z.literal(200),
  rehydratedKillStatus: z.literal(200),
  rehydratedCompletedStatus: z.literal('completed'),
  rehydratedCompletedNextOffset: z.literal(29),
  rehydratedKilledStatus: z.literal('killed'),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5ThreadSummaryRouteExecutableExpected = z.object({
  summaryStatus: z.literal(200),
  taskCount: z.literal(2),
  subagentCount: z.literal(2),
  runningTaskStatus: z.literal('running'),
  runningTaskTerminal: z.literal(false),
  completedTaskStatus: z.literal('completed'),
  completedTaskTerminal: z.literal(true),
  taskOutputWithheld: z.literal(true),
  taskFactAnswerAllowed: z.literal(false),
  taskEvidenceAuthority: z.literal(false),
  taskCanReadOutput: z.literal(false),
  runningSubagentStatus: z.literal('active'),
  completedSubagentStatus: z.literal('done'),
  subagentCanOpenThread: z.literal(true),
  subagentCanKill: z.literal(false),
  outputStatus: z.literal(200),
  output: z.literal('route output\n'),
  outputBytes: z.literal(13),
  crossThreadOutputStatus: z.literal(403),
  crossThreadKillStatus: z.literal(403),
  killStatus: z.literal(200),
  killedTaskStatus: z.literal('killed'),
  killedTaskTerminal: z.literal(true),
  commandTaskStatus: z.literal('completed'),
  commandTaskTerminal: z.literal(true),
  restartStatus: z.literal(409),
  restartErrorCode: z.literal('conflict'),
  restartMessage: z.literal('The request conflicts with the current runtime state.'),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5ThreadSummaryRouteExecutableCase = z.object({
  sourceContractId: z.literal('thread-summary-task-routes-v1'),
  routes: z.object({
    summary: z.literal('/v1/threads/{id}/summary'),
    output: z.literal('/v1/threads/{id}/summary/tasks/{task}/output'),
    kill: z.literal('/v1/threads/{id}/summary/tasks/{task}/kill'),
    restart: z.literal('/v1/threads/{id}/summary/tasks/{task}/restart')
  }),
  seeded: z.object({
    parentThreadId: z.literal('thr_g5_summary'),
    otherThreadId: z.literal('thr_g5_summary_other'),
    runningJobId: z.literal('task_g5_summary_1'),
    completedJobId: z.literal('parallel_task_g5_summary_1'),
    childRunId: z.literal('run_g5_summary_task'),
    parallelChildRunId: z.literal('run_g5_summary_parallel'),
    childThreadId: z.literal('thr_g5_summary_child'),
    parallelChildThreadId: z.literal('thr_g5_summary_parallel_child'),
    restartThreadId: z.literal('thr_g5_summary_restart'),
    commandTaskId: z.literal('command:call_g5_restart'),
    restartJobId: z.literal('task_g5_summary_restart_1')
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelRouteExposed: z.literal(false)
  }),
  expected: GoG5ThreadSummaryRouteExecutableExpected
})

const GoG5TaskJobLifecycleExpected = z.object({
  foregroundKind: z.literal('task'),
  foregroundStatus: z.literal('completed'),
  foregroundResult: z.string().min(1),
  backgroundKind: z.literal('task'),
  backgroundStatusAcrossTurn: z.literal('running'),
  backgroundOutput: z.string().min(1),
  backgroundFinalStatus: z.literal('completed'),
  backgroundFinalResult: z.string().min(1),
  waitOutputKillStatus: z.literal('killed'),
  waitOutputKillError: z.string().min(1),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5TaskJobPlannerExecutorExpected = z.object({
  plannerKind: z.literal('planner'),
  plannerPolicy: z.literal('readOnly'),
  executorPolicy: z.literal('inherit'),
  failureStatus: z.literal('failed'),
  cancelledStatus: z.literal('cancelled'),
  skippedReason: z.string().min(1),
  cancelReason: z.string().min(1),
  outputOffsetJobCount: z.number().int().positive(),
  failurePropagates: z.literal(true),
  cancellationPropagates: z.literal(true),
  transcriptPropagates: z.literal(true),
  transcriptPropagationMode: z.enum(['continue', 'fork']),
  transcriptPropagationJobCount: z.number().int().positive(),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5ProductBoundaryOutput = ShadowOnlyProductBoundary.extend({
  electronMainConnected: z.literal(false),
  defaultGoBackendEnabled: z.literal(false),
  usesReasonixProtocol: z.literal(false),
  rendererRouteExposed: z.literal(false)
})

const GoG5ProductBoundaryControlCase = z.object({
  sourceContractId: z.literal('go-g5-full-loop-contract-v1'),
  productBoundary: ShadowOnlyProductBoundary,
  electronMainConnected: z.literal(false),
  defaultGoBackendEnabled: z.literal(false),
  expected: GoG5ProductBoundaryOutput
})

const GoG5PackageRuntimeIdentityOutput = z.object({
  rootPackageNameAnalytix: z.literal(true),
  rootProductNameAnalytix: z.literal(true),
  runtimePackageNameAnalytixRuntime: z.literal(true),
  runtimeBinAnalytixServeEntry: z.literal(true),
  runtimeServeScriptUsesServeEntry: z.literal(true),
  builderAppIdAnalytix: z.literal(true),
  builderProductNameAnalytix: z.literal(true),
  builderExecutableNameLowercaseAnalytix: z.literal(true),
  builderArtifactNameAnalytix: z.literal(true),
  nsisNamesAnalytix: z.literal(true),
  appProductNameAnalytix: z.literal(true),
  appUserDataDirectoryLowercaseAnalytix: z.literal(true),
  windowsAppUserModelIdAnalytix: z.literal(true),
  resolveBundledServeEntry: z.literal(true),
  serveUsageAnalytixServe: z.literal(true),
  serveEntryAllowsOnlyAnalytixServe: z.literal(true),
  readyHandshakeAnalytix: z.literal(true),
  afterPackRequiresServeEntry: z.literal(true),
  releaseEnvAnalytixPrefixed: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  kunIdentityExposed: z.literal(false),
  defaultGoBackendEnabled: z.literal(false)
})

const GoG5PackageRuntimeIdentityCase = z.object({
  sourceContractId: z.literal('package-runtime-cli-identity-v1'),
  sourceFiles: z.object({
    rootPackage: z.literal('package.json'),
    runtimePackage: z.literal('packages/runtime/package.json'),
    electronBuilderConfig: z.literal('electron-builder.config.cjs'),
    electronBuilderStandardWinConfig: z.literal('electron-builder.standard-win.cjs'),
    appIdentity: z.literal('src/main/app-identity.ts'),
    mainIndex: z.literal('src/main/index.ts'),
    resolveAnalytixBinary: z.literal('src/main/resolve-analytix-binary.ts'),
    runtimeServeEntry: z.literal('packages/runtime/src/cli/serve-entry.ts'),
    runtimeServe: z.literal('packages/runtime/src/cli/serve.ts'),
    afterPack: z.literal('scripts/after-pack.cjs'),
    packagingConfigTest: z.literal('src/main/packaging-config.test.ts'),
    releaseWorkflow: z.literal('.github/workflows/release.yml')
  }),
  releaseIdentity: z.object({
    rootPackageName: z.literal('analytix'),
    rootProductName: z.literal('Analytix'),
    runtimePackageName: z.literal('analytix-runtime'),
    runtimeBinName: z.literal('analytix'),
    runtimeBinPath: z.literal('./dist/cli/serve-entry.js'),
    runtimeServeScript: z.literal('node ./dist/cli/serve-entry.js'),
    appId: z.literal('com.analytix.desktop'),
    builderProductName: z.literal('Analytix'),
    builderExecutableName: z.literal('analytix'),
    artifactNamePrefix: z.literal('analytix-'),
    nsisShortcutName: z.literal('Analytix灵鉴'),
    nsisUninstallDisplayName: z.literal('Analytix灵鉴'),
    appProductName: z.literal('Analytix'),
    appUserDataDirectoryName: z.literal('analytix'),
    windowsAppUserModelId: z.literal('com.analytix.desktop')
  }),
  runtimeCli: z.object({
    bundledEntryCandidate: z.literal('packages/runtime/dist/cli/serve-entry.js'),
    afterPackRequiredPath: z.literal('packages/runtime/dist/cli/serve-entry.js'),
    readyPrefix: z.literal('ANALYTIX_READY '),
    serveUsagePrefix: z.literal('analytix serve [options]'),
    supportedCommand: z.literal('serve'),
    unknownCommandMessage: z.literal("Only 'analytix serve' is supported.")
  }),
  forbiddenIdentityFields: z.array(z.enum(['kun', 'reasonix', 'deepseek'])).length(3),
  forbiddenIdentityFieldsPresent: z.array(z.enum(['kun', 'reasonix', 'deepseek'])).length(0),
  evidence: z.object({
    packagingTestPinsReleaseIdentity: z.literal(true),
    resolveUsesBundledServeEntry: z.literal(true),
    serveEntryOnlySupportsServe: z.literal(true),
    serveUsageMentionsAnalytixServe: z.literal(true),
    afterPackRequiresServeEntry: z.literal(true),
    releaseWorkflowUsesAnalytixEnv: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    kunIdentityExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: GoG5PackageRuntimeIdentityOutput
})

const GoG5RuntimeHTTPRouteSovereigntyOutput = z.object({
  routeCountExact: z.literal(true),
  onlyHealthUnauthenticated: z.literal(true),
  allRuntimeRoutesAnalytixOwned: z.literal(true),
  healthUnauthenticatedStatusOk: z.literal(true),
  allAuthenticatedRoutesRejectMissingAuth: z.literal(true),
  unauthorizedBodyShapeStable: z.literal(true),
  authMatrixCoversAllRegisteredRoutes: z.literal(true),
  sensitiveRoutesProtected: z.literal(true),
  forbiddenDispatchReturnsStructuredNotFound: z.literal(true),
  forbiddenDispatchCoversAllTokens: z.literal(true),
  forbiddenProtocolTokensRejected: z.literal(true),
  forbiddenHiddenSurfaceTokensRejected: z.literal(true),
  forbiddenDispatchUsesRuntimeAuth: z.literal(true),
  sharedEndpointBuilderCaseCountExact: z.literal(true),
  sharedEndpointBuildersEncodeRouteIds: z.literal(true),
  sharedEndpointTemplatesAnalytixOwned: z.literal(true),
  sharedEndpointCanonicalUserInputPlural: z.literal(true),
  sharedEndpointSensitiveBuildersPresent: z.literal(true),
  sharedEndpointBuilderUnitEvidencePresent: z.literal(true),
  sseRouteAnalytixThreadEvents: z.literal(true),
  threadLifecycleRoutesPresent: z.literal(true),
  approvalUserInputRoutesPresent: z.literal(true),
  taskJobRoutesInternalOnly: z.literal(true),
  notFoundStructured: z.literal(true),
  noForbiddenRoutes: z.literal(true),
  singularUserInputCompatibilityOnly: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelHiddenEntryExposed: z.literal(false),
  defaultGoBackendEnabled: z.literal(false)
})

const GoG5RuntimeHTTPRouteSovereigntyCase = z.object({
  sourceContractId: z.literal('runtime-http-route-sovereignty-v1'),
  sourceFiles: z.object({
    serverRoutesIndex: z.literal('packages/runtime/src/server-test-support/routes/index.ts'),
    router: z.literal('packages/runtime/src/server-test-support/router.ts'),
    httpServer: z.literal('packages/runtime/src/server-test-support/http-server.ts'),
    sharedEndpoints: z.literal('src/shared/analytix-endpoints.ts'),
    sharedEndpointTest: z.literal('src/shared/analytix-endpoints.test.ts'),
    httpServerTest: z.literal('packages/runtime/tests/http-server.test.ts')
  }),
  routeCount: z.literal(52),
  routes: z.array(z.object({
    method: z.enum(['GET', 'POST', 'PATCH', 'DELETE']),
    path: z.string().min(1)
  })).length(52),
  unauthenticatedRoutes: z.array(z.literal('GET /health')).length(1),
  authenticatedRouteCount: z.literal(51),
  authMatrix: z.object({
    healthRoute: z.literal('GET /health'),
    healthStatus: z.literal(200),
    unauthorizedStatus: z.literal(401),
    unauthorizedBody: z.object({
      code: z.literal('unauthorized'),
      message: z.literal('Runtime authentication is required.')
    }),
    protectedRouteCount: z.literal(51),
    protectedRouteKeys: z.array(z.string().min(1)).length(51),
    sensitiveRouteKeys: z.array(z.enum([
      'GET /v1/threads/:id/events',
      'POST /v1/runtime/task-jobs/wait',
      'POST /v1/approvals/:id',
      'POST /v1/user-inputs/:id',
      'POST /v1/sessions/:id/resume-thread'
    ])).length(5)
  }),
  sseRoutes: z.array(z.literal('GET /v1/threads/:id/events')).length(1),
  internalTaskJobRoutes: z.array(z.enum([
    'POST /v1/runtime/task-jobs/wait',
    'POST /v1/runtime/task-jobs/output',
    'POST /v1/runtime/task-jobs/kill'
  ])).length(3),
  compatibilityOnlyRoutes: z.array(z.literal('POST /v1/user-input/:id')).length(1),
  forbiddenRouteTokens: z.array(z.string().min(1)).min(8),
  forbiddenRouteTokensPresent: z.array(z.string()).length(0),
  forbiddenDispatchMatrix: z.object({
    authMode: z.literal('valid-runtime-token'),
    status: z.literal(404),
    body: z.object({
      code: z.literal('not_found'),
      message: z.literal('route not found')
    }),
    tokenCount: z.literal(13),
    tokens: z.array(z.string().min(1)).length(13),
    protocolTokens: z.array(z.enum([
      '/v1/reasonix',
      '/v1/kun',
      '/v1/deepseek',
      '/v1/runtime/go',
      '/session-api',
      '/api/session'
    ])).length(6),
    hiddenSurfaceTokens: z.array(z.enum([
      '/v1/workflow',
      '/v1/create-loop',
      '/v1/subagent',
      '/v1/subagents',
      '/v1/autoresearch',
      '/v1/auto-research',
      '/v1/mcp-indexer'
    ])).length(7)
  }),
  sharedEndpointBuilderMatrix: z.object({
    sourceId: z.literal('id/with space?x=1#frag'),
    turnId: z.literal('turn/with space?x=1#frag'),
    taskId: z.literal('task/with space?x=1#frag'),
    checkpointId: z.literal('axcp/with space?x=1#frag'),
    encodedSourceId: z.literal('id%2Fwith%20space%3Fx%3D1%23frag'),
    encodedTurnId: z.literal('turn%2Fwith%20space%3Fx%3D1%23frag'),
    encodedTaskId: z.literal('task%2Fwith%20space%3Fx%3D1%23frag'),
    encodedCheckpointId: z.literal('axcp%2Fwith%20space%3Fx%3D1%23frag'),
    encodedCaseProjectId: z.literal('case%2Fwith%20space%3Fx%3D1%23frag'),
    builderCases: z.array(z.object({
      name: z.enum([
        'analytixThreadPath',
        'analytixThreadSummaryPath',
        'analytixThreadSummaryTaskOutputPath',
        'analytixThreadSummaryTaskKillPath',
        'analytixThreadSummaryTaskRestartPath',
        'analytixThreadForkPath',
        'analytixThreadGoalPath',
        'analytixThreadTodosPath',
        'analytixThreadCompactPath',
        'analytixThreadReviewPath',
        'analytixThreadTurnsPath',
        'analytixThreadRewindPath',
        'analytixThreadSteerPath',
        'analytixThreadInterruptPath',
        'analytixThreadEventsPath',
        'analytixThreadCheckpointRewindPlanPath',
        'analytixThreadCheckpointRewindApplyPath',
        'analytixApprovalPath',
        'analytixUserInputPath',
        'analytixSessionResumePath',
        'analytixAttachmentPath',
        'analytixAttachmentContentPath',
        'analytixMemoryRecordPath',
        'analytixCaseProjectThreadsPath',
        'analytixCaseProjectDetailPath'
      ]),
      template: z.string().min(1),
      outputPath: z.string().min(1),
      encodedSegments: z.array(z.string().min(1)).min(1)
    })).length(25),
    exportedEndpointStrings: z.array(z.string().min(1)).min(20),
    forbiddenTokens: z.array(z.string().min(1)).length(10),
    forbiddenTokensPresent: z.array(z.string()).length(0),
    canonicalUserInputTemplate: z.literal('/v1/user-inputs/{id}'),
    singularUserInputTemplateExported: z.literal(false),
    sensitiveBuilderNames: z.array(z.enum([
      'analytixThreadSummaryPath',
      'analytixThreadSummaryTaskOutputPath',
      'analytixThreadSummaryTaskKillPath',
      'analytixThreadSummaryTaskRestartPath',
      'analytixThreadEventsPath',
      'analytixThreadRewindPath',
      'analytixApprovalPath',
      'analytixUserInputPath',
      'analytixSessionResumePath',
      'analytixThreadCheckpointRewindPlanPath',
      'analytixThreadCheckpointRewindApplyPath'
    ])).length(11),
    unitTestEvidencePresent: z.literal(true)
  }),
  evidence: z.object({
    routerFirstMatch: z.literal(true),
    structuredNotFound: z.literal(true),
    healthNoAuth: z.literal(true),
    v1AuthGuardsCoverRoutes: z.literal(true),
    sseUsesBuildEventStreamResponse: z.literal(true),
    taskJobRoutesUseInternalHandlers: z.literal(true),
    singularUserInputCompatibility: z.literal(true),
    sharedThreadEventsTemplatePresent: z.literal(true),
    sharedSessionResumeTemplatePresent: z.literal(true),
    sharedApprovalUserInputTemplatesPresent: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelHiddenEntryExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: GoG5RuntimeHTTPRouteSovereigntyOutput
})

const GoG5DesktopSovereigntyEvidenceId = z.enum([
  'preload-sandbox-single-analytix-bridge',
  'window-type-analytix-only',
  'shared-api-owned-facade',
  'shared-api-no-upstream-public-types',
  'runtime-request-analytix-ipc',
  'runtime-sse-analytix-ipc',
  'settings-drop-reasonix-auto-plan',
  'settings-drop-legacy-agent-envelope',
  'settings-endpoint-format-top-level-runtime',
  'renderer-generic-runtime-bypass-absent',
  'renderer-settings-read-bypass-absent',
  'renderer-optional-chain-bridge-scan',
  'renderer-provider-shared-root-paths',
  'renderer-provider-runtime-client-facade',
  'renderer-provider-endpoint-builder-unit-evidence',
  'renderer-provider-alias-guard-evidence',
  'renderer-provider-lifecycle-alias-evidence',
  'renderer-provider-dynamic-route-alias-evidence',
  'renderer-provider-facade-source-evidence',
  'renderer-provider-facade-lifecycle-evidence',
  'renderer-provider-direct-bypass-scan-evidence',
  'side-conversation-relation-provider-contract',
  'side-conversation-relation-store-evidence',
  'side-conversation-direct-bypass-scan-evidence',
  'renderer-usage-runtime-client-source-evidence',
  'renderer-usage-runtime-client-settings-diagnostics-evidence',
  'renderer-usage-runtime-client-unit-evidence',
  'renderer-usage-direct-bypass-scan-evidence',
  'renderer-settings-read-facade-source-evidence',
  'renderer-settings-read-event-sync-evidence',
  'renderer-settings-read-direct-bypass-scan-evidence',
  'renderer-runtime-client-request-evidence',
  'renderer-runtime-client-restart-evidence',
  'renderer-runtime-client-sse-evidence',
  'renderer-settings-cache-evidence',
  'renderer-settings-runtime-patch-evidence',
  'renderer-settings-legacy-alias-evidence',
  'preload-runtime-request-path-method-body-evidence',
  'preload-runtime-restart-evidence',
  'preload-diagnostics-runtime-request-same-channel',
  'preload-sse-start-stop-evidence',
  'preload-sse-payload-only-listeners'
])

const GoG5NamedRuntimeApi = z.enum([
  'acceptedSlotDisplay',
  'cancelFundsCSVImport',
  'cleaningDiffPreview',
  'confirmFundsCSVSnapshot',
  'directSourcePreview',
  'fetchUpstreamModels',
  'getAnalytixConfigFile',
  'importMappingPreview',
  'onRuntimeStatus',
  'openAnalytixConfigDir',
  'probeModelCapabilities',
  'restartRuntime',
  'revokeCleaningDiffPreview',
  'runDeterministicFundsCleaning',
  'stageFundsCSVSnapshot',
  'statusFundsCSVImport',
  'setAnalytixConfigFile'
])

const GoG5NamedSettingsApi = z.enum([
  'saveSettingsSilent',
  'setSettings'
])

const GoG5ForbiddenRuntimeIpcChannel = z.enum([
  'reasonix:request',
  'kun:request',
  'runtime:go:request',
  'workflow:request',
  'reasonix:sse',
  'kun:sse',
  'runtime:go:sse',
  'workflow:sse'
])

const GoG5DesktopSovereigntyOutput = z.object({
  exposesOnlyAnalytixBridge: z.literal(true),
  windowTypeOnlyAnalytix: z.literal(true),
  facadeDomainsAnalytixOwned: z.literal(true),
  publicApiTypesAnalytixOwned: z.literal(true),
  runtimeRequestUsesAnalytixIpc: z.literal(true),
  runtimeSseUsesAnalytixIpc: z.literal(true),
  rendererNamedRuntimeApisAllowListed: z.literal(true),
  rendererNamedSettingsApisAllowListed: z.literal(true),
  rendererGenericRuntimeBypassAbsent: z.literal(true),
  rendererSettingsReadBypassAbsent: z.literal(true),
  optionalChainBridgeAccessScanned: z.literal(true),
  rendererProviderUsesSharedRootPaths: z.literal(true),
  rendererProviderEncodesDynamicRouteIds: z.literal(true),
  rendererProviderRuntimePathsAnalytixOwned: z.literal(true),
  rendererProviderUsesRuntimeClientFacade: z.literal(true),
  rendererProviderEndpointBuilderUnitEvidencePresent: z.literal(true),
  rendererProviderAliasGuardInstallsThrowingAliases: z.literal(true),
  rendererProviderAliasGuardCoversRuntimeRoutes: z.literal(true),
  rendererProviderAliasGuardCoversLifecycleAndGates: z.literal(true),
  rendererProviderAliasGuardCoversForkResumeAndEncoding: z.literal(true),
  rendererProviderAliasGuardRejectsForbiddenRoutes: z.literal(true),
  rendererProviderAliasGuardUnitEvidencePresent: z.literal(true),
  rendererProviderFacadeSealUsesRuntimeClient: z.literal(true),
  rendererProviderFacadeSealRejectsDirectBridge: z.literal(true),
  rendererProviderFacadeSealCoversArchiveRestore: z.literal(true),
  rendererProviderFacadeSealCoversRelationPatch: z.literal(true),
  rendererProviderFacadeSealScanGuardPresent: z.literal(true),
  rendererProviderFacadeSealUnitEvidencePresent: z.literal(true),
  sideConversationRelationContractOptionalProvider: z.literal(true),
  sideConversationRelationPromotesThroughProvider: z.literal(true),
  sideConversationRelationRefreshesAndCloses: z.literal(true),
  sideConversationRelationRejectsDirectBridge: z.literal(true),
  sideConversationRelationScanGuardPresent: z.literal(true),
  sideConversationRelationUnitEvidencePresent: z.literal(true),
  rendererUsageRuntimeClientCoversThreadUsage: z.literal(true),
  rendererUsageRuntimeClientCoversDailyUsage: z.literal(true),
  rendererUsageRuntimeClientCoversModelUsage: z.literal(true),
  rendererUsageRuntimeClientCoversSettingsDiagnostics: z.literal(true),
  rendererUsageRuntimeClientRejectsDirectBridge: z.literal(true),
  rendererUsageRuntimeClientScanGuardPresent: z.literal(true),
  rendererUsageRuntimeClientUnitEvidencePresent: z.literal(true),
  rendererSettingsReadFacadeCoversKeyboardShortcuts: z.literal(true),
  rendererSettingsReadFacadeCoversSpeechToText: z.literal(true),
  rendererSettingsReadFacadeCoversUsageModelLabel: z.literal(true),
  rendererSettingsReadFacadePreservesSettingsChangedEvent: z.literal(true),
  rendererSettingsReadFacadeRejectsDirectBridge: z.literal(true),
  rendererSettingsReadFacadeScanGuardPresent: z.literal(true),
  rendererRuntimeClientRuntimeRequestPreservesArguments: z.literal(true),
  rendererRuntimeClientRestartUsesRuntimeApi: z.literal(true),
  rendererRuntimeClientSseControlsPreserveArguments: z.literal(true),
  rendererRuntimeClientSseListenersPreserveHandlers: z.literal(true),
  rendererRuntimeClientLegacyAliasesUnread: z.literal(true),
  rendererRuntimeClientUnitEvidencePresent: z.literal(true),
  rendererSettingsBridgeUsesAnalytixSettingsApi: z.literal(true),
  rendererSettingsBridgeCachesReads: z.literal(true),
  rendererSettingsBridgeRefreshesCacheAfterWrite: z.literal(true),
  rendererSettingsBridgePreservesTopLevelRuntimePatch: z.literal(true),
  rendererSettingsBridgeLegacyAliasesUnread: z.literal(true),
  rendererSettingsBridgeUnitEvidencePresent: z.literal(true),
  preloadRuntimeRequestPreservesPathMethodBody: z.literal(true),
  preloadDiagnosticsRuntimeRequestUsesSameChannel: z.literal(true),
  preloadRuntimeRestartUsesAnalytixIpc: z.literal(true),
  preloadRuntimeRequestExposesOnlyAnalytixApi: z.literal(true),
  preloadRuntimeRequestUnitEvidencePresent: z.literal(true),
  preloadSseStartStopPreservesArguments: z.literal(true),
  preloadSsePayloadListenersOmitElectronEvent: z.literal(true),
  preloadSseListenerCleanupUsesSameWrapper: z.literal(true),
  preloadSseBridgeUnitEvidencePresent: z.literal(true),
  dropsReasonixAutoPlanConfig: z.literal(true),
  stripsLegacyRuntimeSettings: z.literal(true),
  writesTopLevelRuntimeSettings: z.literal(true),
  deprecatedBridgeAliasExposed: z.literal(false),
  forbiddenRuntimeIpcExposed: z.literal(false),
  deprecatedSettingsFallbackWritten: z.literal(false),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5DesktopSovereigntyControlCase = z.object({
  sourceFiles: z.object({
    preload: z.literal('src/preload/index.ts'),
    windowTypes: z.literal('src/preload/index.d.ts'),
    sharedApi: z.literal('src/shared/analytix-api.ts'),
    settingsStoreTest: z.literal('src/main/settings-store.test.ts'),
    runtimeNormalizer: z.literal('src/shared/app-settings-runtime.ts'),
    productSovereigntyScan: z.literal('scripts/scan-product-sovereignty.cjs'),
    rendererSourceRoot: z.literal('src/renderer/src'),
    rendererRuntimeClient: z.literal('src/renderer/src/agent/runtime-client.ts'),
    rendererRuntimeClientTest: z.literal('src/renderer/src/agent/runtime-client.test.ts'),
    rendererProvider: z.literal('src/renderer/src/agent/analytix-runtime.ts'),
    rendererProviderTest: z.literal('src/renderer/src/agent/analytix-runtime.test.ts'),
    rendererAgentTypes: z.literal('src/renderer/src/agent/types.ts'),
    rendererSideActions: z.literal('src/renderer/src/store/chat-store-side-actions.ts'),
    rendererSideActionsTest: z.literal('src/renderer/src/store/chat-store-side-actions.test.ts'),
    rendererThreadUsage: z.literal('src/renderer/src/hooks/use-thread-usage.ts'),
    rendererThreadUsageTest: z.literal('src/renderer/src/hooks/use-thread-usage.test.ts'),
    rendererDailyUsage: z.literal('src/renderer/src/hooks/use-daily-usage.ts'),
    rendererDailyUsageTest: z.literal('src/renderer/src/hooks/use-daily-usage.test.ts'),
    rendererModelUsage: z.literal('src/renderer/src/hooks/use-model-usage.ts'),
    rendererModelUsageTest: z.literal('src/renderer/src/hooks/use-model-usage.test.ts'),
    rendererSettingsAgents: z.literal('src/renderer/src/components/settings-section-agents.tsx'),
    rendererLlmDebug: z.literal('src/renderer/src/components/settings-section-llm-debug.tsx'),
    rendererKeyboardShortcutSettings: z.literal('src/renderer/src/lib/keyboard-shortcut-settings.ts'),
    rendererVoiceDictation: z.literal('src/renderer/src/components/chat/use-voice-dictation.ts'),
    rendererInitialUsageHeatmap: z.literal('src/renderer/src/components/chat/InitialSessionUsageHeatmap.tsx'),
    preloadRuntimeRequestTest: z.literal('src/preload/preload-runtime-request.test.ts'),
    preloadSseBridgeTest: z.literal('src/preload/preload-sse-bridge.test.ts')
  }),
  bridgeName: z.literal('analytix'),
  runtimeIpcChannels: z.object({
    request: z.literal('runtime:request'),
    sseStart: z.literal('runtime:sse:start'),
    sseStop: z.literal('runtime:sse:stop'),
    sseEvent: z.literal('runtime:sse-event'),
    sseEnd: z.literal('runtime:sse-end'),
    sseError: z.literal('runtime:sse-error')
  }),
  exposedBridgeNames: z.array(z.string().min(1)).length(1),
  windowTypeProperties: z.array(z.string().min(1)).length(1),
  facadeDomains: z.array(z.string().min(1)).min(1),
  forbiddenBridgeAliases: z.array(z.enum([
    'analytixGui',
    'deepseek',
    'deepseekGui',
    'kun',
    'reasonix'
  ])).min(1),
  forbiddenRuntimeIpcChannels: z.array(GoG5ForbiddenRuntimeIpcChannel).length(8),
  forbiddenRuntimeIpcChannelsExposed: z.array(GoG5ForbiddenRuntimeIpcChannel).length(0),
  forbiddenSettingsKeys: z.array(z.enum([
    'agent',
    'agentProvider',
    'agents',
    'autoPlan',
    'auto_plan',
    'deepseek',
    'reasonix'
  ])).min(1),
  rendererBridgeAllowList: z.object({
    allowedNamedRuntimeApis: z.array(GoG5NamedRuntimeApi).length(GoG5NamedRuntimeApi.options.length),
    allowedNamedSettingsApis: z.array(GoG5NamedSettingsApi).length(2),
    directRuntimeApis: z.array(GoG5NamedRuntimeApi).length(GoG5NamedRuntimeApi.options.length),
    directSettingsApis: z.array(GoG5NamedSettingsApi).length(2),
    directRuntimeViolations: z.array(z.string()).length(0),
    directSettingsViolations: z.array(z.string()).length(0),
    genericRuntimeBypassCount: z.literal(0),
    settingsReadBypassCount: z.literal(0),
    optionalChainPatternCovered: z.literal(true)
  }),
  rendererRuntimeClientBridgeMatrix: z.object({
    sourceIds: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      turnId: z.literal('turn/with space?x=1#frag')
    }),
    encodedIds: z.object({
      threadId: z.literal('thr%2Fwith%20space%3Fx%3D1%23frag'),
      turnId: z.literal('turn%2Fwith%20space%3Fx%3D1%23frag')
    }),
    runtimeRequestCalls: z.array(z.object({
      path: z.literal('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/turns/turn%2Fwith%20space%3Fx%3D1%23frag/steer'),
      method: z.literal('POST').optional(),
      body: z.literal('{"action":"step"}').optional(),
      argumentCount: z.union([z.literal(1), z.literal(2), z.literal(3)])
    })).length(3),
    sseStartCall: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      sinceSeq: z.literal(7),
      streamId: z.literal('stream-renderer')
    }),
    sseStopCall: z.object({
      streamId: z.literal('stream-renderer')
    }),
    listenerApis: z.array(z.enum(['onSseEvent', 'onSseEnd', 'onSseError'])).length(3),
    sourceUsesAnalytixRuntime: z.literal(true),
    sourcePassesRuntimeRequestArgumentsUnchanged: z.literal(true),
    sourcePassesRestartUnchanged: z.literal(true),
    sourcePassesSseControlsUnchanged: z.literal(true),
    sourcePassesSseListenersUnchanged: z.literal(true),
    runtimeRequestUnitEvidencePresent: z.literal(true),
    restartUnitEvidencePresent: z.literal(true),
    sseUnitEvidencePresent: z.literal(true),
    legacyAliasUnitEvidencePresent: z.literal(true)
  }),
  rendererSettingsBridgeMatrix: z.object({
    patch: z.object({
      workspaceRoot: z.literal('/tmp/next'),
      runtimeModel: z.literal('deepseek-reasoner'),
      approvalPolicy: z.literal('never')
    }),
    cacheExpectations: z.object({
      getSettingsCallsForDoubleRead: z.literal(1),
      getSettingsCallsAfterSetSettings: z.literal(1),
      setSettingsCallsAfterWrite: z.literal(1)
    }),
    sourceUsesAnalytixSettings: z.literal(true),
    sourceCachesSettingsReads: z.literal(true),
    sourceRefreshesCacheAfterSetSettings: z.literal(true),
    cacheUnitEvidencePresent: z.literal(true),
    refreshUnitEvidencePresent: z.literal(true),
    topLevelRuntimePatchUnitEvidencePresent: z.literal(true),
    legacyAliasUnitEvidencePresent: z.literal(true)
  }),
  rendererProviderEndpointMatrix: z.object({
    rootPaths: z.object({
      health: z.literal('/health'),
      threads: z.literal('/v1/threads')
    }),
    rootConstantsUsed: z.literal(true),
    sourceIds: z.object({
      threadId: z.literal('thr/route?x=1#frag'),
      turnId: z.literal('turn/route?x=1#frag'),
      approvalId: z.literal('appr/route?x=1#frag'),
      inputId: z.literal('input/route?x=1#frag'),
      sessionId: z.literal('sess/route?x=1#frag')
    }),
    encodedIds: z.object({
      threadId: z.literal('thr%2Froute%3Fx%3D1%23frag'),
      turnId: z.literal('turn%2Froute%3Fx%3D1%23frag'),
      approvalId: z.literal('appr%2Froute%3Fx%3D1%23frag'),
      inputId: z.literal('input%2Froute%3Fx%3D1%23frag'),
      sessionId: z.literal('sess%2Froute%3Fx%3D1%23frag')
    }),
    encodedRuntimeRequestPaths: z.array(z.string().min(1)).length(8),
    sensitivePathKinds: z.array(z.enum([
      'turns',
      'steer',
      'interrupt',
      'compact',
      'approval',
      'user-input',
      'fork',
      'resume-thread'
    ])).length(8),
    usesRendererRuntimeClientFacade: z.literal(true),
    pathOwnershipGuardPresent: z.literal(true),
    unitTestEvidencePresent: z.literal(true)
  }),
  rendererProviderAliasGuardMatrix: z.object({
    helperName: z.literal('installDsGui'),
    forbiddenAliases: z.array(z.enum(['kun', 'reasonix'])).length(2),
    guardedTestNames: z.array(z.enum([
      'keeps renderer runtime requests on analytix-owned HTTP routes',
      'uses Analytix thread lifecycle endpoints for list, archive, restore, rename, workspace, and delete',
      'calls Analytix fork and user-input compatibility endpoints',
      'URL-encodes dynamic runtime route ids before calling the bridge',
      'resumes a session through the Analytix HTTP runtime'
    ])).length(5),
    coverage: z.object({
      routeOwnership: z.literal(true),
      threadLifecycle: z.literal(true),
      approvalUserInput: z.literal(true),
      forkResume: z.literal(true),
      dynamicRouteEncoding: z.literal(true)
    }),
    sourceInstallsThrowingAliases: z.literal(true),
    sourceUsesAnalytixOnly: z.literal(true),
    routeOwnershipEvidencePresent: z.literal(true),
    lifecycleEvidencePresent: z.literal(true),
    approvalUserInputEvidencePresent: z.literal(true),
    forkResumeEvidencePresent: z.literal(true),
    dynamicEncodingEvidencePresent: z.literal(true),
    forbiddenRouteGuardPresent: z.literal(true)
  }),
  rendererProviderFacadeSealMatrix: z.object({
    facade: z.literal('rendererRuntimeClient.runtimeRequest'),
    forbiddenDirectBridge: z.literal('window.analytix.runtime.runtimeRequest'),
    sealedMethods: z.array(z.enum(['archiveThread', 'updateThreadRelation'])).length(2),
    sourceUsesRuntimeClient: z.literal(true),
    sourceRejectsDirectBridgeBypass: z.literal(true),
    archiveRestoreSourceEvidencePresent: z.literal(true),
    relationSourceEvidencePresent: z.literal(true),
    lifecycleUnitEvidencePresent: z.literal(true),
    scanGuardPresent: z.literal(true),
    providerFacadeTokenScanPresent: z.literal(true)
  }),
  sideConversationRelationContractMatrix: z.object({
    providerMethod: z.literal('updateThreadRelation'),
    storeAction: z.literal('promoteSideConversation'),
    relation: z.literal('primary'),
    providerContractOptional: z.literal(true),
    providerImplementationUsesRuntimeClient: z.literal(true),
    storeUsesProviderContract: z.literal(true),
    storeRefreshesAndCloses: z.literal(true),
    storeRejectsDirectRuntimeBridge: z.literal(true),
    unitEvidencePresent: z.literal(true),
    scanGuardPresent: z.literal(true)
  }),
  rendererUsageRuntimeClientFacadeMatrix: z.object({
    facade: z.literal('rendererRuntimeClient.runtimeRequest'),
    forbiddenDirectBridge: z.literal('window.analytix.runtime.runtimeRequest'),
    usageLoaders: z.array(z.enum(['loadThreadUsage', 'loadDailyUsage', 'loadModelUsage'])).length(3),
    diagnosticsLoaders: z.array(z.enum(['loadTokenEconomySavingsSummary', 'LlmDebugSettingsSection'])).length(2),
    threadUsageSourceUsesRuntimeClient: z.literal(true),
    dailyUsageSourceUsesRuntimeClient: z.literal(true),
    modelUsageSourceUsesRuntimeClient: z.literal(true),
    tokenEconomySourceUsesRuntimeClient: z.literal(true),
    llmDebugSourceUsesRuntimeClient: z.literal(true),
    sourceRejectsDirectBridgeBypass: z.literal(true),
    usageUnitEvidencePresent: z.literal(true),
    scanGuardPresent: z.literal(true),
    usageFacadeTokenScanPresent: z.literal(true)
  }),
  rendererSettingsReadFacadeMatrix: z.object({
    facade: z.literal('rendererRuntimeClient.getSettings'),
    forbiddenDirectBridge: z.literal('window.analytix.settings.getSettings'),
    settingsReaders: z.array(z.enum([
      'useKeyboardShortcutSettings',
      'useSpeechToTextEnabled',
      'InitialSessionUsageHeatmapView'
    ])).length(3),
    keyboardShortcutSourceUsesSettingsClient: z.literal(true),
    speechToTextSourceUsesSettingsClient: z.literal(true),
    usageHeatmapSourceUsesSettingsClient: z.literal(true),
    settingsChangedEventPreserved: z.literal(true),
    sourceRejectsDirectSettingsBypass: z.literal(true),
    scanGuardPresent: z.literal(true),
    settingsReadFacadeTokenScanPresent: z.literal(true)
  }),
  preloadRuntimeRequestBridgeMatrix: z.object({
    channel: z.literal('runtime:request'),
    restartChannel: z.literal('runtime:restart'),
    sourceIds: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      turnId: z.literal('turn/with space?x=1#frag'),
      inputId: z.literal('input/with space?x=1#frag')
    }),
    encodedIds: z.object({
      threadId: z.literal('thr%2Fwith%20space%3Fx%3D1%23frag'),
      turnId: z.literal('turn%2Fwith%20space%3Fx%3D1%23frag'),
      inputId: z.literal('input%2Fwith%20space%3Fx%3D1%23frag')
    }),
    runtimeFacadeCall: z.object({
      path: z.literal('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/turns/turn%2Fwith%20space%3Fx%3D1%23frag/steer'),
      method: z.literal('POST'),
      body: z.literal('{"action":"step"}')
    }),
    diagnosticsFacadeCall: z.object({
      path: z.literal('/v1/user-inputs/input%2Fwith%20space%3Fx%3D1%23frag'),
      method: z.literal('POST'),
      body: z.literal('{}')
    }),
    sourcePassesArgumentsUnchanged: z.literal(true),
    sourcePassesRestartUnchanged: z.literal(true),
    runtimeFacadeUnitEvidencePresent: z.literal(true),
    restartUnitEvidencePresent: z.literal(true),
    diagnosticsFacadeUnitEvidencePresent: z.literal(true),
    exposesOnlyAnalytixApi: z.literal(true)
  }),
  preloadSseBridgeMatrix: z.object({
    channels: z.object({
      start: z.literal('runtime:sse:start'),
      stop: z.literal('runtime:sse:stop'),
      event: z.literal('runtime:sse-event'),
      end: z.literal('runtime:sse-end'),
      error: z.literal('runtime:sse-error')
    }),
    startCall: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      sinceSeq: z.literal(42),
      streamId: z.literal('stream-provided')
    }),
    stopCall: z.object({
      streamId: z.literal('stream-provided')
    }),
    eventPayload: z.object({
      streamId: z.literal('stream-a'),
      seq: z.literal(1),
      kind: z.literal('thread_event')
    }),
    endPayload: z.object({
      streamId: z.literal('stream-a')
    }),
    errorPayload: z.object({
      streamId: z.literal('stream-a'),
      status: z.literal(404)
    }),
    sourcePassesStartStopUnchanged: z.literal(true),
    sourcePayloadOnlyWrappers: z.literal(true),
    sourceCleanupUsesSameWrapper: z.literal(true),
    startStopUnitEvidencePresent: z.literal(true),
    payloadUnitEvidencePresent: z.literal(true),
    exposesOnlyAnalytixApi: z.literal(true)
  }),
  evidenceIds: z.array(GoG5DesktopSovereigntyEvidenceId).length(42),
  expected: GoG5DesktopSovereigntyOutput
})

const GoG5DesktopMainIpcBoundaryOutput = z.object({
  runtimeRequestSchemaStrict: z.literal(true),
  runtimeRequestAllowsAnalytixRoutes: z.literal(true),
  runtimeRequestRejectsForbiddenRoutes: z.literal(true),
  runtimeRequestHandlerRejectsBeforeRuntimeCall: z.literal(true),
  mainIpcEndpointBuilderAcceptsSharedPaths: z.literal(true),
  mainIpcEndpointBuilderRejectsRawDynamicRoutes: z.literal(true),
  mainIpcEndpointBuilderUsesSharedTemplates: z.literal(true),
  mainIpcEndpointBuilderUnitEvidencePresent: z.literal(true),
  mainIpcEndpointBuilderRejectsSingularUserInput: z.literal(true),
  mainIpcRuntimeAdapterPreservesEncodedPaths: z.literal(true),
  mainIpcRuntimeAdapterPreservesMethodAndBody: z.literal(true),
  mainIpcRuntimeAdapterRejectsRawDynamicRoutes: z.literal(true),
  mainIpcRuntimeAdapterRejectsBeforeCall: z.literal(true),
  mainIpcRuntimeAdapterUnitEvidencePresent: z.literal(true),
  mainRuntimeHostHandoffPreservesEncodedPathAndQuery: z.literal(true),
  mainRuntimeHostHandoffPreservesMethodHeadersBody: z.literal(true),
  mainRuntimeHostHandoffUsesEnsuredSettings: z.literal(true),
  mainRuntimeHostHandoffUnitEvidencePresent: z.literal(true),
  mainSseHostEncodesThreadIdAndCursor: z.literal(true),
  mainSseHostPreservesHeadersAndStreamId: z.literal(true),
  mainSseHostRejectsForbiddenRouteTokens: z.literal(true),
  mainSseHostEncodingUnitEvidencePresent: z.literal(true),
  sseStartSchemaStrict: z.literal(true),
  sseRejectsReasonixSessionPayload: z.literal(true),
  sseUsesAnalytixThreadEventsRoute: z.literal(true),
  sseStopParsesStreamId: z.literal(true),
  sseStopMatchesStreamIdOnly: z.literal(true),
  sseReconnectCursorPreserved: z.literal(true),
  sseBatchesEventsAt100ms: z.literal(false),
  sseFlushesFirstEventAndBatchesAt16ms: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  rendererRouteExposed: z.literal(false),
  defaultGoBackendEnabled: z.literal(false)
})

const GoG5DesktopMainIpcBoundaryControlCase = z.object({
  sourceContractId: z.literal('desktop-main-ipc-boundary-v1'),
  sourceFiles: z.object({
    appIpcSchemas: z.literal('src/main/ipc/app-ipc-schemas.ts'),
    appIpcSchemasTest: z.literal('src/main/ipc/app-ipc-schemas.test.ts'),
    registerAppIpcHandlers: z.literal('src/main/ipc/register-app-ipc-handlers.ts'),
    registerAppIpcHandlersTest: z.literal('src/main/ipc/register-app-ipc-handlers.test.ts'),
    runtimeSseIpc: z.literal('src/main/runtime-sse-ipc.ts'),
    runtimeSseIpcTest: z.literal('src/main/runtime-sse-ipc.test.ts'),
    runtimeAdapter: z.literal('src/main/runtime/analytix-adapter.ts'),
    runtimeAdapterTest: z.literal('src/main/runtime/analytix-adapter.test.ts')
  }),
  runtimeRequest: z.object({
    handlerChannel: z.literal('runtime:request'),
    allowedAnalytixRouteExamples: z.array(z.string().min(1)).min(4),
    forbiddenRuntimeRoutes: z.array(z.enum([
      '/v1/reasonix/session?thread_id=thr_1',
      '/v1/runtime/go/threads',
      '/v1/workflow',
      '/v1/workflows',
      '/v1/create-loop',
      '/v1/subagents',
      '/v1/autoresearch',
      '/v1/mcp-indexer'
    ])).length(8),
    evidence: z.object({
      schemaStrict: z.literal(true),
      refinesAllowedSurface: z.literal(true),
      normalizesRelativePath: z.literal(true),
      handlerParsesBeforeRuntimeCall: z.literal(true),
      forbiddenHandlerNoExecute: z.literal(true)
    })
  }),
  sse: z.object({
    startChannel: z.literal('runtime:sse:start'),
    stopChannel: z.literal('runtime:sse:stop'),
    eventChannel: z.literal('runtime:sse-event'),
    errorChannel: z.literal('runtime:sse-error'),
    endChannel: z.literal('runtime:sse-end'),
    routePathTemplate: z.literal('/v1/threads/{id}/events'),
    batchMs: z.literal(16),
    evidence: z.object({
      startSchemaStrict: z.literal(true),
      rejectsReasonixSessionId: z.literal(true),
      usesAnalytixThreadEventsPath: z.literal(true),
      stopParsesStreamId: z.literal(true),
      stopMatchesStreamIdOnly: z.literal(true),
      reconnectLastEventId: z.literal(true),
      batchesEventsAt100ms: z.literal(false),
      flushesFirstEventAndBatchesAt16ms: z.literal(true)
    })
  }),
  endpointBuilderAllowListMatrix: z.object({
    sourceIds: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      turnId: z.literal('turn/with space?x=1#frag'),
      checkpointId: z.literal('axcp/with space?x=1#frag'),
      approvalId: z.literal('appr/with space?x=1#frag'),
      inputId: z.literal('input/with space?x=1#frag'),
      sessionId: z.literal('sess/with space?x=1#frag'),
      attachmentId: z.literal('att/with space?x=1#frag'),
      memoryId: z.literal('mem/with space?x=1#frag')
    }),
    encodedIds: z.object({
      threadId: z.literal('thr%2Fwith%20space%3Fx%3D1%23frag'),
      turnId: z.literal('turn%2Fwith%20space%3Fx%3D1%23frag'),
      checkpointId: z.literal('axcp%2Fwith%20space%3Fx%3D1%23frag'),
      approvalId: z.literal('appr%2Fwith%20space%3Fx%3D1%23frag'),
      inputId: z.literal('input%2Fwith%20space%3Fx%3D1%23frag'),
      sessionId: z.literal('sess%2Fwith%20space%3Fx%3D1%23frag'),
      attachmentId: z.literal('att%2Fwith%20space%3Fx%3D1%23frag'),
      memoryId: z.literal('mem%2Fwith%20space%3Fx%3D1%23frag')
    }),
    acceptedSharedBuilderRequests: z.array(z.object({
      name: z.enum([
        'thread-read',
        'thread-patch',
        'thread-fork',
        'thread-rewind',
        'thread-turns',
        'thread-steer',
        'thread-interrupt',
        'checkpoint-rewind-plan',
        'checkpoint-rewind-apply',
        'approval-submit',
        'user-input-submit',
        'session-resume',
        'attachment-content',
        'memory-record'
      ]),
      path: z.string().min(1),
      method: z.enum(['GET', 'POST', 'PATCH'])
    })).length(14),
    rejectedRawDynamicRequests: z.array(z.object({
      path: z.enum([
        '/v1/threads/thr/raw/turns',
        '/v1/threads/thr/raw/rewind',
        '/v1/threads/thr/raw/turns/turn/raw/steer',
        '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan',
        '/v1/approvals/appr/raw',
        '/v1/user-input/input_raw',
        '/v1/sessions/sess/raw/resume-thread',
        '/v1/memory/mem/raw',
        '/v1/attachments/att/raw/content'
      ]),
      method: z.enum(['GET', 'POST', 'PATCH'])
    })).length(9),
    sharedTemplatesCompiled: z.array(z.enum([
      'ANALYTIX_THREAD_TEMPLATE',
      'ANALYTIX_THREAD_FORK_TEMPLATE',
      'ANALYTIX_THREAD_REWIND_TEMPLATE',
      'ANALYTIX_THREAD_TURNS_TEMPLATE',
      'ANALYTIX_THREAD_STEER_TEMPLATE',
      'ANALYTIX_THREAD_INTERRUPT_TEMPLATE',
      'ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE',
      'ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE',
      'ANALYTIX_APPROVAL_TEMPLATE',
      'ANALYTIX_USER_INPUT_TEMPLATE',
      'ANALYTIX_SESSION_RESUME_TEMPLATE',
      'ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE',
      'ANALYTIX_MEMORY_RECORD_TEMPLATE'
    ])).length(13),
    schemaUsesSharedTemplates: z.literal(true),
    unitTestEvidencePresent: z.literal(true),
    rawRouteRejectionEvidencePresent: z.literal(true)
  }),
  runtimeAdapterHandoffMatrix: z.object({
    handlerChannel: z.literal('runtime:request'),
    adapterFunction: z.literal('runtimeRequest'),
    acceptedAdapterCalls: z.array(z.object({
      name: z.enum([
        'thread-steer',
        'thread-rewind',
        'checkpoint-rewind-plan',
        'approval-submit',
        'user-input-submit',
        'session-resume',
        'attachment-content',
        'memory-record'
      ]),
      path: z.string().min(1),
      method: z.enum(['GET', 'POST', 'PATCH']),
      body: z.string().optional()
    })).length(8),
    rejectedBeforeAdapterCalls: z.array(z.object({
      path: z.enum([
        '/v1/threads/thr/raw/rewind',
        '/v1/threads/thr/raw/turns/turn/raw/steer',
        '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan',
        '/v1/approvals/appr/raw',
        '/v1/user-input/input_raw',
        '/v1/sessions/sess/raw/resume-thread',
        '/v1/attachments/att/raw/content'
      ]),
      method: z.enum(['GET', 'POST']),
      body: z.string().optional()
    })).length(7),
    parseBeforeAdapterCall: z.literal(true),
    encodedPathEvidencePresent: z.literal(true),
    methodAndBodyEvidencePresent: z.literal(true),
    rejectsBeforeAdapterEvidencePresent: z.literal(true)
  }),
  runtimeHostHandoffMatrix: z.object({
    adapterFunction: z.literal('runtimeRequestViaHost'),
    baseUrlFunction: z.literal('getRuntimeBaseUrlForSettings'),
    sourceIds: z.object({
      threadId: z.literal('thr/with space?x=1#frag'),
      turnId: z.literal('turn/with space?x=1#frag')
    }),
    encodedIds: z.object({
      threadId: z.literal('thr%2Fwith%20space%3Fx%3D1%23frag'),
      turnId: z.literal('turn%2Fwith%20space%3Fx%3D1%23frag')
    }),
    request: z.object({
      path: z.literal('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/turns/turn%2Fwith%20space%3Fx%3D1%23frag/steer?dry_run=true&timezone=Asia%2FShanghai'),
      method: z.literal('POST'),
      body: z.literal('{"action":"step"}'),
      authHeader: z.literal('Bearer usage-token'),
      contentType: z.literal('application/json'),
      customHeaderName: z.literal('X-Analytix-Evidence'),
      customHeaderValue: z.literal('runtime-host-path')
    }),
    sourceEvidence: z.object({
      usesEnsuredSettings: z.literal(true),
      joinsBaseWithNormalizedPath: z.literal(true),
      setsBearerAuth: z.literal(true),
      forwardsCustomHeaders: z.literal(true),
      defaultsJsonContentType: z.literal(true),
      forwardsMethodAndBody: z.literal(true)
    }),
    unitTestEvidencePresent: z.literal(true),
    ensureRuntimePortEvidencePresent: z.literal(true)
  }),
  mainSseHostEncodingMatrix: z.object({
    handlerChannel: z.literal('runtime:sse:start'),
    eventChannel: z.literal('runtime:sse-error'),
    sourceThreadId: z.literal('thr/with space?x=1#frag'),
    encodedThreadId: z.literal('thr%2Fwith%20space%3Fx%3D1%23frag'),
    request: z.object({
      path: z.literal('/v1/threads/thr%2Fwith%20space%3Fx%3D1%23frag/events'),
      sinceSeq: z.literal(9),
      lastEventId: z.literal('9'),
      accept: z.literal('text/event-stream'),
      authorization: z.literal('Bearer runtime-token'),
      streamId: z.literal('stream-encoded')
    }),
    errorPayload: z.object({
      streamId: z.literal('stream-encoded'),
      status: z.literal(404)
    }),
    sourceBuildsAnalytixEventsPath: z.literal(true),
    encodedUnitEvidencePresent: z.literal(true),
    headerUnitEvidencePresent: z.literal(true),
    errorUnitEvidencePresent: z.literal(true),
    forbiddenRouteGuardPresent: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    rendererRouteExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: GoG5DesktopMainIpcBoundaryOutput
})

const GoG5RendererRouteSurfaceSovereigntyOutput = z.object({
  appRouteUnionKunCompatible: z.literal(true),
  noForbiddenTopLevelRouteTokens: z.literal(true),
  noForbiddenEntrypointSymbols: z.literal(true),
  dormantWorkflowCodeQuarantined: z.literal(true),
  browserPreviewBridgeAnalytixOnly: z.literal(true),
  pluginMarketplaceSafeAreaPropagates: z.literal(true),
  shellNavigationNoDrag: z.literal(true),
  nativeControlsSafeInset: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelHiddenEntryExposed: z.literal(false),
  defaultGoBackendEnabled: z.literal(false)
})

const GoG5RendererRouteSurfaceSovereigntyCase = z.object({
  sourceContractId: z.literal('renderer-route-surface-sovereignty-v1'),
  sourceFiles: z.object({
    workbench: z.literal('src/renderer/src/components/Workbench.tsx'),
    workbenchRouteSurfaceTest: z.literal('src/renderer/src/components/Workbench.route-surface.test.ts'),
    chatStoreTypes: z.literal('src/renderer/src/store/chat-store-types.ts'),
    sidebar: z.literal('src/renderer/src/components/chat/Sidebar.tsx'),
    pluginMarketplaceView: z.literal('src/renderer/src/components/PluginMarketplaceView.tsx'),
    browserAnalytixBridge: z.literal('src/renderer/src/lib/browser-analytix-bridge.ts'),
    workflowCreateLoopView: z.literal('src/renderer/src/components/workflow/WorkflowCreateLoopView.tsx'),
    createLoopRuntime: z.literal('src/renderer/src/workflow/create-loop-runtime.ts'),
    shellNavigationControls: z.literal('src/renderer/src/components/shell/ShellNavigationControls.tsx'),
    workbenchShell: z.literal('src/renderer/src/components/workbench/WorkbenchShell.tsx'),
    baseShellCss: z.literal('src/renderer/src/styles/base-shell.css')
  }),
  appRoutes: z.array(z.enum(['chat', 'write', 'settings', 'plugins', 'claw', 'schedule'])).length(6),
  forbiddenTopLevelRouteTokens: z.array(z.string().min(1)).min(10),
  forbiddenTopLevelRouteTokensPresent: z.array(z.string()).length(0),
  forbiddenEntrypointSymbols: z.array(z.string().min(1)).min(5),
  forbiddenEntrypointSymbolsPresent: z.array(z.string()).length(0),
  quarantinedWorkflowSymbols: z.array(z.string().min(1)).length(4),
  quarantinedWorkflowEntrySurfaceHits: z.array(z.string()).length(0),
  evidence: z.object({
    dormantWorkflowCodeExists: z.literal(true),
    browserPreviewInstallsWindowAnalytix: z.literal(true),
    browserPreviewForbiddenAliasesAbsent: z.literal(true),
    pluginMarketplaceReceivesLeftSidebarCollapsed: z.literal(true),
    pluginMarketplaceTabsBeforeContent: z.literal(true),
    shellNavigationControlsNoDrag: z.literal(true),
    nativeSafeInsetCssPresent: z.literal(true)
  }),
  productBoundary: z.object({
    usesReasonixProtocol: z.literal(false),
    topLevelHiddenEntryExposed: z.literal(false),
    defaultGoBackendEnabled: z.literal(false)
  }),
  expected: GoG5RendererRouteSurfaceSovereigntyOutput
})

const GoG5TaskJobToolContractBoundaryOutput = z.object({
  taskToolName: z.literal('task'),
  parallelToolName: z.literal('parallel_tasks'),
  taskInternalRuntimeOnly: z.literal(true),
  parallelInternalRuntimeOnly: z.literal(true),
  requiresPermissionGate: z.literal(true),
  mayAppendParentGoalEvidence: z.literal(true),
  requiresDependencyValidation: z.literal(true),
  requiresPlannerReadOnlyToolset: z.literal(true),
  routes: z.array(z.string().min(1)).length(3),
  protectedRoutes: z.array(z.enum(['wait', 'output', 'kill'])).length(3),
  unauthorizedStatus: z.literal(401),
  forbiddenTopLevelRoutes: z.array(z.string().min(1)).min(1),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5TaskJobToolContractBoundaryCase = z.object({
  sourceContractId: z.literal('task-job-orchestration-fixture-v1'),
  task: z.object({
    name: z.literal('task'),
    promptField: z.literal('prompt'),
    backgroundField: z.literal('run_in_background'),
    continueField: z.literal('continue_from'),
    forkField: z.literal('fork_from'),
    internalRuntimeOnly: z.literal(true),
    requiresPermissionGate: z.literal(true),
    mayAppendParentGoalEvidence: z.literal(true)
  }),
  parallelTasks: z.object({
    name: z.literal('parallel_tasks'),
    tasksField: z.literal('tasks'),
    dependencyField: z.literal('depends_on'),
    internalRuntimeOnly: z.literal(true),
    requiresDependencyValidation: z.literal(true),
    requiresPlannerReadOnlyToolset: z.literal(true)
  }),
  routes: z.array(z.string().min(1)).length(3),
  protectedRoutes: z.array(z.enum(['wait', 'output', 'kill'])).length(3),
  unauthorizedStatus: z.literal(401),
  forbiddenTopLevelRoutes: z.array(z.string().min(1)).min(1),
  expected: GoG5TaskJobToolContractBoundaryOutput
})

const GoG5TaskJobTranscriptIdentityOutput = z.object({
  sourceId: z.string().min(1),
  continueTargetId: z.string().min(1),
  forkTargetId: z.string().min(1),
  incompatibleError: z.string().min(1),
  sameTranscriptIdentityRequired: z.literal(true),
  continuePreservesTarget: z.literal(true),
  forkCreatesDistinctTarget: z.literal(true),
  continueTargetMatchesSource: z.literal(true),
  forkTargetDistinctFromSource: z.literal(true),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5TaskJobTranscriptIdentityCase = z.object({
  sourceContractId: z.literal('task-job-orchestration-fixture-v1'),
  transcript: z.object({
    sourceId: z.string().min(1),
    continueTargetId: z.string().min(1),
    forkTargetId: z.string().min(1),
    incompatibleError: z.string().min(1),
    sameIdentityRequired: z.literal(true),
    continuePreservesTarget: z.literal(true),
    forkCreatesDistinctTarget: z.literal(true)
  }),
  expected: GoG5TaskJobTranscriptIdentityOutput
})

const GoG5TaskJobNestedSseMetadataOutput = z.object({
  parentCallId: z.string().min(1),
  childRunId: z.string().min(1),
  nestedSseMetadataFields: z.array(z.string().min(1)).length(4),
  includesParentCallId: z.literal(true),
  includesChildRunId: z.literal(true),
  includesEvidenceLedgered: z.literal(true),
  includesEvidenceLedgerError: z.literal(true),
  parentChildDistinct: z.literal(true),
  requiresActiveGoal: z.literal(true),
  evidenceLedgeredEventKey: z.literal('evidenceLedgered'),
  evidenceLedgerErrorEventKey: z.literal('evidenceLedgerError'),
  usesReasonixProtocol: z.literal(false),
  topLevelRouteExposed: z.literal(false)
})

const GoG5TaskJobNestedSseMetadataCase = z.object({
  sourceContractId: z.literal('task-job-orchestration-fixture-v1'),
  nestedEvent: z.object({
    parentCallId: z.string().min(1),
    childRunId: z.string().min(1),
    nestedSseMetadataFields: z.array(z.string().min(1)).length(4)
  }),
  parentGoalEvidence: z.object({
    requiresActiveGoal: z.literal(true),
    evidenceLedgeredEventKey: z.literal('evidenceLedgered'),
    evidenceLedgerErrorEventKey: z.literal('evidenceLedgerError')
  }),
  expected: GoG5TaskJobNestedSseMetadataOutput
})

const GoG5ToolResult = z.object({
  callId: z.string().min(1),
  status: z.enum(['completed', 'aborted']),
  code: z.string(),
  output: z.string()
})

const GoG5ControlExecutableCases = z.object({
  productBoundary: GoG5ProductBoundaryControlCase,
  packageRuntimeIdentity: GoG5PackageRuntimeIdentityCase,
  runtimeHttpRouteSovereignty: GoG5RuntimeHTTPRouteSovereigntyCase,
  desktopSovereignty: GoG5DesktopSovereigntyControlCase,
  desktopMainIpcBoundary: GoG5DesktopMainIpcBoundaryControlCase,
  rendererRouteSurfaceSovereignty: GoG5RendererRouteSurfaceSovereigntyCase,
  goalPersistenceOffLock: GoGoalPersistenceOffLockControlCase,
  toolResultFileImageBoundary: GoToolResultFileImageBoundaryControlCase,
  eventJsonlReplayBoundary: GoEventJsonlReplayBoundaryControlCase,
  mcpMalformedSchemaBoundary: GoMcpMalformedSchemaBoundaryControlCase,
  cancel: z.object({
    acceptedToolCalls: z.array(z.object({
      callId: z.string().min(1),
      state: z.enum(['completed', 'running', 'unstarted']),
      output: z.string()
    })).min(1),
    cancelledResultCode: z.literal('tool_call_cancelled'),
    expectedResults: z.array(GoG5ToolResult).min(1),
    scheduledAfterCancel: z.literal(0)
  }),
  taskJobs: z.object({
    toolContractBoundary: GoG5TaskJobToolContractBoundaryCase,
    transcriptIdentity: GoG5TaskJobTranscriptIdentityCase,
    nestedSseMetadata: GoG5TaskJobNestedSseMetadataCase,
    jobs: z.array(z.object({
      id: z.string().min(1),
      state: z.enum(['completed', 'running', 'queued']),
      output: z.string(),
      offset: z.number().int().min(0)
    })).min(1),
    skippedUnstartedReason: z.literal('cancelled: parent turn aborted before task execution'),
    expectedJobs: z.array(z.object({
      id: z.string().min(1),
      status: z.enum(['completed', 'killed', 'skipped']),
      output: z.string(),
      offset: z.number().int().min(0),
      reason: z.string()
    })).min(1),
    staleReconcile: z.object({
      jobs: z.array(z.object({
        id: z.string().min(1),
        state: z.enum(['running', 'queued']),
        output: z.string(),
        offset: z.number().int().min(0)
      })).length(2),
      reason: z.string().min(1),
      expectedJobs: z.array(z.object({
        id: z.string().min(1),
        status: z.literal('interrupted'),
        output: z.string(),
        offset: z.number().int().min(0),
        reason: z.string().min(1)
      })).length(2)
    }),
    restartDrill: z.object({
      jobs: z.array(z.object({
        id: z.string().min(1),
        state: z.enum(['running', 'queued']),
        output: z.string(),
        offset: z.number().int().min(0)
      })).length(2),
      outputAfterRestart: z.string().min(1),
      waitStatus: z.literal('completed'),
      killStatus: z.literal('killed'),
      killError: z.string().min(1),
      expected: z.object({
        rehydratedCount: z.literal(2),
        completed: z.object({
          id: z.string().min(1),
          status: z.literal('completed'),
          output: z.string().min(1),
          nextOffset: z.number().int().positive()
        }),
        killed: z.object({
          id: z.string().min(1),
          status: z.literal('killed'),
          error: z.string().min(1)
        })
      })
    }),
    routeExecutable: z.object({
      unauthorizedStatus: z.literal(401),
      output: z.object({
        status: z.literal(200),
        jobStatus: z.literal('running'),
        output: z.string().min(1),
        nextOffset: z.literal(14),
        replayOffset: z.literal(0)
      }),
      wait: z.object({
        status: z.literal(200),
        jobStatus: z.literal('completed'),
        result: z.string().min(1)
      }),
      kill: z.object({
        status: z.literal(200),
        jobStatus: z.literal('killed'),
        error: z.string().min(1)
      }),
      missingOutput: z.object({
        status: z.literal(404)
      }),
      rehydrated: z.object({
        outputStatus: z.literal(200),
        waitStatus: z.literal(200),
        killStatus: z.literal(200),
        completedStatus: z.literal('completed'),
        completedNextOffset: z.literal(29),
        killedStatus: z.literal('killed')
      }),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: GoG5TaskJobRouteExecutableExpected
    }),
    summaryRouteExecutable: GoG5ThreadSummaryRouteExecutableCase,
    lifecycle: z.object({
      foreground: z.object({
        kind: z.literal('task'),
        status: z.literal('completed'),
        result: z.string().min(1)
      }),
      background: z.object({
        kind: z.literal('task'),
        statusAcrossTurn: z.literal('running'),
        output: z.string().min(1),
        finalStatus: z.literal('completed'),
        finalResult: z.string().min(1)
      }),
      waitOutputKill: z.object({
        killedStatus: z.literal('killed'),
        killedError: z.string().min(1)
      }),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: GoG5TaskJobLifecycleExpected
    }),
    plannerExecutor: z.object({
      plannerKind: z.literal('planner'),
      plannerPolicy: z.literal('readOnly'),
      executorPolicy: z.literal('inherit'),
      failureStatus: z.literal('failed'),
      cancelledStatus: z.literal('cancelled'),
      skippedReason: z.string().min(1),
      cancelReason: z.string().min(1),
      outputOffsetJobCount: z.number().int().positive(),
      requiresFailurePropagation: z.literal(true),
      requiresCancellationPropagation: z.literal(true),
      requiresTranscriptPropagation: z.literal(true),
      transcriptPropagationMode: z.enum(['continue', 'fork']),
      transcriptPropagationJobCount: z.number().int().positive(),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: GoG5TaskJobPlannerExecutorExpected
    }),
    plannerToolsetInventory: z.object({
      readOnlyToolset: z.array(z.string().min(1)).length(4),
      forbiddenToolset: z.array(z.enum(['task', 'parallel_tasks'])).length(2),
      taskToolName: z.literal('task'),
      parallelToolName: z.literal('parallel_tasks'),
      plannerPolicy: z.literal('readOnly'),
      executorPolicy: z.literal('inherit'),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: z.object({
        readOnlyToolset: z.array(z.string().min(1)).length(4),
        forbiddenToolset: z.array(z.enum(['task', 'parallel_tasks'])).length(2),
        readOnlyToolCount: z.literal(4),
        forbiddenToolCount: z.literal(2),
        readOnlyExcludesTaskTools: z.literal(true),
        forbiddenMatchesTaskTools: z.literal(true),
        plannerPolicy: z.literal('readOnly'),
        executorPolicy: z.literal('inherit'),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      })
    }),
    parentGoalEvidence: z.object({
      requiresActiveGoal: z.literal(true),
      ledgeredEventKey: z.literal('evidenceLedgered'),
      ledgerErrorEventKey: z.literal('evidenceLedgerError'),
      activeGoalEventMetadata: z.object({
        evidenceLedgered: z.literal(true),
        evidenceLedgerError: z.literal('')
      }),
      missingGoalEventMetadata: z.object({
        evidenceLedgered: z.literal(false),
        evidenceLedgerError: z.string().min(1)
      }),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: z.object({
        requiresActiveGoal: z.literal(true),
        ledgeredEventKey: z.literal('evidenceLedgered'),
        ledgerErrorEventKey: z.literal('evidenceLedgerError'),
        ledgeredWhenActiveGoal: z.literal(true),
        errorsWithoutActiveGoal: z.literal(true),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      })
    }),
    parallelValidation: z.object({
      dependencyField: z.literal('depends_on'),
      validPlan: z.array(GoG5ParallelPlanItem).min(2),
      invalidPlans: z.array(z.object({
        caseId: GoG5ParallelValidationCaseId,
        tasks: z.array(GoG5ParallelPlanItem).min(1)
      })).length(5),
      productBoundary: z.object({
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      expected: z.object({
        dependencyField: z.literal('depends_on'),
        validOrder: z.array(z.string().min(1)).min(3),
        invalidResults: z.array(z.object({
          caseId: GoG5ParallelValidationCaseId,
          error: z.string().min(1)
        })).length(5),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      })
    })
  }),
  approvalDeny: z.object({
    attemptedToolNames: z.array(z.string().min(1)).min(2),
    approvalIds: z.array(z.string().min(1)).min(2),
    expected: z.object({
      deniedToolNames: z.array(z.string().min(1)).min(2),
      approvalIds: z.array(z.string().min(1)).min(2),
      approvalItemCount: z.number().int().min(2),
      createsDurableJobs: z.literal(false),
      createsChildRuns: z.literal(false)
    })
  }),
  userInput: z.object({
    submitted: z.object({
      id: z.string().min(1),
      itemId: z.string().min(1),
      answers: z.array(UserInputAnswerExpectation).min(1),
      resolvedEventKind: z.literal('user_input_resolved'),
      resolvedEventIncludesAnswers: z.literal(false),
      pendingBefore: z.literal(1),
      pendingAfter: z.literal(0)
    }),
    cancelled: z.object({
      id: z.string().min(1),
      status: z.literal('cancelled'),
      secondResolveStatus: z.literal(404),
      pendingBefore: z.literal(1),
      pendingAfter: z.literal(0)
    }),
    structuredChoiceValidation: UserInputStructuredChoiceValidation,
    expected: z.object({
      submittedInputId: z.string().min(1),
      submittedStatus: z.literal('submitted'),
      answerCount: z.number().int().positive(),
      httpEchoesAnswers: z.literal(true),
      resolvedEventKind: z.literal('user_input_resolved'),
      resolvedEventIncludesAnswers: z.literal(false),
      cancelledInputId: z.string().min(1),
      cancelledStatus: z.literal('cancelled'),
      secondResolveStatus: z.literal(404),
      pendingAfterSubmit: z.literal(0),
      pendingAfterCancel: z.literal(0),
      structuredChoiceValidation: z.object({
        maxQuestions: z.literal(3),
        minOptionsWhenProvided: z.literal(2),
        maxOptionsWhenProvided: z.literal(3),
        dedupeLabelsCaseInsensitive: z.literal(true),
        invalidResultCode: z.literal('invalid_user_input_request'),
        invalidCases: z.array(UserInputValidationCaseId).min(4),
        invalidCaseCount: z.literal(4),
        rejectsTooManyQuestions: z.literal(true),
        rejectsSingleOption: z.literal(true),
        rejectsTooManyOptions: z.literal(true),
        rejectsDuplicateLabelsCaseInsensitive: z.literal(true),
        opensGateOnInvalid: z.literal(false),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      })
    })
  }),
  approvalUserInputRouteReplay: GoApprovalUserInputRouteReplayControlCase,
  approvalUserInputInventory: GoApprovalUserInputInventoryControlCase,
  abortCleanup: z.object({
    approvalId: z.string().min(1),
    userInputId: z.string().min(1),
    expectedApprovalStatus: z.literal('expired'),
    expectedUserInputStatus: z.literal('cancelled'),
    lateApprovalDecisionStatus: z.literal(409),
    lateUserInputResolveStatus: z.literal(404),
    replayKinds: z.array(z.enum([
      'approval_requested',
      'approval_resolved',
      'user_input_requested',
      'user_input_resolved'
    ])).length(4),
    expected: z.object({
      approvalId: z.string().min(1),
      approvalStatus: z.literal('expired'),
      userInputId: z.string().min(1),
      userInputStatus: z.literal('cancelled'),
      lateApprovalDecisionStatus: z.literal(409),
      lateUserInputResolveStatus: z.literal(404),
      pendingAfterCleanup: z.literal(0),
      replayKinds: z.array(z.enum([
        'approval_requested',
        'approval_resolved',
        'user_input_requested',
        'user_input_resolved'
      ])).length(4),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  resumePendingGates: z.object({
    sourceThreadId: z.string().min(1),
    approvalId: z.string().min(1),
    userInputId: z.string().min(1),
    sourceStatuses: z.object({
      approval: z.literal('pending'),
      userInput: z.literal('pending')
    }),
    resumedStatuses: z.object({
      approval: z.literal('expired'),
      userInput: z.literal('cancelled')
    }),
    expected: z.object({
      sourceApprovalStatus: z.literal('pending'),
      sourceUserInputStatus: z.literal('pending'),
      resumedApprovalStatus: z.literal('expired'),
      resumedUserInputStatus: z.literal('cancelled'),
      pendingAfterResume: z.literal(0),
      answersCopiedToResume: z.literal(false),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  autoResearch: z.object({
    threadId: z.string().min(1),
    objective: z.string().min(1),
    requirements: z.array(z.string().min(1)).min(1),
    expectedStateRelativePath: z.string().startsWith('.analytix/autoresearch/'),
    expectedFiles: z.array(z.enum([
      'task_spec.md',
      'progress.json',
      'findings.jsonl',
      'directions_tried.json',
      'iteration_log.jsonl'
    ])).length(5),
    direction: z.string().min(1),
    directionOutcome: z.enum(['tried', 'promising', 'dead_end']),
    directionSummary: z.string().min(1),
    recordDirectionToolName: z.literal('record_research_direction'),
    directionRequiresActiveResearchGoal: z.literal(true),
    unknownRequirementId: z.string().min(1),
    expected: z.object({
      stateRelativePath: z.string().startsWith('.analytix/autoresearch/'),
      fileCount: z.literal(5),
      writesReasonixFile: z.literal(false),
      writesAgentsFile: z.literal(false),
      unknownRequirementAccepted: z.literal(false),
      findingsWrittenForUnknownRequirement: z.literal(false),
      directionTrackingFileWritten: z.literal(true),
      iterationLogRecordsDirection: z.literal(true),
      recordResearchDirectionToolPresent: z.literal(true),
      recordDirectionRequiresActiveResearchGoal: z.literal(true),
      stablePrefixContainsState: z.literal(false),
      toolSchemaContainsState: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  mcpLifecycle: z.object({
    providerId: z.string().min(1),
    failedServerIds: z.array(z.string().min(1)).min(1),
    expectedConnectedServerIds: z.array(z.string().min(1)),
    expectedErrorServerIds: z.array(z.string().min(1)),
    attemptsPerFailedServer: z.number().int().positive(),
    requiresRuntimeRestart: z.literal(false),
    liveLocal: z.object({
      serverId: z.string().min(1),
      cwd: z.string().min(1),
      lowPriority: z.literal(true),
      backgroundStart: z.literal(true),
      initialPaths: z.array(z.string().min(1)).min(1),
      lateTombstonePath: z.string().min(1),
      resumePaths: z.array(z.string().min(1)).min(1),
      secretDiagnostic: z.string().min(1),
      replacement: z.string().min(1)
    }),
    expected: z.object({
      providerId: z.string().min(1),
      retryAttempts: z.record(z.string(), z.number().int().positive()),
      connectedServerIds: z.array(z.string().min(1)),
      errorServerIds: z.array(z.string().min(1)),
      requiresRuntimeRestart: z.literal(false),
      activePaths: z.array(z.string().min(1)),
      tombstoneCount: z.literal(1),
      restartedFromSnapshot: z.literal(true),
      secretSafeDiagnostic: z.string().min(1),
      leaksSecret: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  mcpCoreLifecycle: GoMcpCoreLifecycleControlCase,
  mcpBackgroundReconnect: GoMcpBackgroundReconnectControlCase,
  mcpCallReconnect: GoMcpCallReconnectControlCase,
  mcpKnownOverrideDiagnostics: GoMcpKnownOverrideDiagnosticsControlCase,
  mcpLiveLocalIndexer: GoMcpLiveLocalIndexerControlCase,
  mcpApprovalAnnotations: GoMcpApprovalAnnotationControlCase,
  mcpSearchMetaTools: GoMcpSearchMetaToolsControlCase,
  mcpSearchRefreshDrift: GoMcpSearchRefreshDriftControlCase,
  mcpSearchWorkspaceBoundary: GoMcpSearchWorkspaceBoundaryControlCase,
  checkpointRewind: z.object({
    checkpointId: z.string().regex(/^axcp_[A-Za-z0-9._-]+$/),
    planId: z.string().regex(/^axrp_[A-Za-z0-9._-]+$/),
    applyId: z.string().regex(/^axra_[A-Za-z0-9._-]+$/),
    rescueId: z.string().regex(/^axrr_[A-Za-z0-9._-]+$/),
    threadId: z.string().min(1),
    turnId: z.string().min(1),
    workspace: z.string().min(1),
    createdAt: z.string().min(1),
    changedFiles: z.array(z.object({
      relativePath: z.string().min(1),
      changeKind: z.enum(['created', 'modified', 'deleted', 'unknown']),
      beforeHash: z.string().optional(),
      afterHash: z.string().optional()
    })).min(1),
    pathRisks: z.array(z.object({
      relativePath: z.string().min(1),
      exists: z.boolean(),
      isSymlink: z.boolean().optional()
    })),
    confirmation: z.object({
      confirmed: z.literal(true),
      destructive: z.literal(true),
      phrase: z.literal('APPLY_CHECKPOINT_REWIND')
    }),
    eventKinds: z.array(z.enum([
      'checkpoint_captured',
      'checkpoint_rewind_rescue_created',
      'checkpoint_rewind_applied'
    ])).length(3),
    expected: z.object({
      checkpointIdPrefix: z.literal('axcp_'),
      planIdPrefix: z.literal('axrp_'),
      applyIdPrefix: z.literal('axra_'),
      rescueIdPrefix: z.literal('axrr_'),
      readyFileCount: z.number().int().nonnegative(),
      blockedFileCount: z.number().int().nonnegative(),
      symlinkBlocked: z.literal(true),
      pathEscapeBlocked: z.literal(true),
      legalDotDotFilenameReady: z.literal(true),
      requiresExplicitConfirmation: z.literal(true),
      conversationAuditAppendOnly: z.literal(true),
      rewritesTranscript: z.literal(false),
      usesGitRefs: z.literal(false),
      eventKinds: z.array(z.enum([
        'checkpoint_captured',
        'checkpoint_rewind_rescue_created',
        'checkpoint_rewind_applied'
      ])).length(3),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  remoteEntry: z.object({
    expectedPortKeys: z.array(z.enum(['approvals', 'lifecycle', 'turns'])).length(3),
    forbiddenPortKeys: z.array(z.string().min(1)).min(1),
    rejectedOverrideKeys: z.array(z.string().min(1)).min(1),
    expected: z.object({
      exposedPortKeys: z.array(z.enum(['approvals', 'lifecycle', 'turns'])).length(3),
      forbiddenPortKeysAbsent: z.literal(true),
      rejectedOverrideAccepted: z.literal(false),
      goalAccess: z.literal(false),
      checkpointAccess: z.literal(false),
      memoryAccess: z.literal(false),
      storageAccess: z.literal(false),
      toolHostAccess: z.literal(false),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  historyRepair: z.object({
    items: z.array(z.object({
      id: z.string().min(1),
      kind: z.enum([
        'user_message',
        'assistant_text',
        'assistant_reasoning',
        'approval',
        'user_input',
        'error',
        'tool_call',
        'tool_result'
      ]),
      turnId: z.string().min(1),
      callId: z.string().optional(),
      toolName: z.string().optional()
    })).min(1),
    expected: z.object({
      repairedIds: z.array(z.string().min(1)).min(1),
      droppedIds: z.array(z.string().min(1)).min(1),
      keptCallIds: z.array(z.string().min(1)).min(1),
      keptResultCallIds: z.array(z.string().min(1)).min(1),
      orphanResultDropped: z.literal(true),
      missingResultCallDropped: z.literal(false),
      duplicateResultDropped: z.literal(true),
      bridgeTextPreserved: z.literal(true),
      stablePrefixContainsRepairState: z.literal(false),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  compactionBoundary: z.object({
    items: z.array(z.object({
      id: z.string().min(1),
      kind: z.enum([
        'user_message',
        'assistant_text',
        'assistant_reasoning',
        'approval',
        'user_input',
        'error',
        'tool_call',
        'tool_result',
        'compaction'
      ]),
      turnId: z.string().min(1),
      replacedTokens: z.number().int().nonnegative().optional(),
      summary: z.string().optional()
    })).min(1),
    expected: z.object({
      effectiveIds: z.array(z.string().min(1)).min(1),
      droppedIds: z.array(z.string().min(1)).min(1),
      latestCompactionId: z.string().min(1),
      latestCompactionFirst: z.literal(true),
      latestCompactionPreserved: z.literal(true),
      olderCompactionDropped: z.literal(true),
      noopCompactionDropped: z.literal(true),
      postCompactionUserPreserved: z.literal(true),
      preCompactionUserDropped: z.literal(true),
      stablePrefixContainsCompactionState: z.literal(false),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  stepLimits: z.object({
    defaultMaxModelSteps: z.number().int().min(0),
    userGlobalMaxModelSteps: z.number().int().positive(),
    sessionMaxModelSteps: z.number().int().positive(),
    turnMaxModelSteps: z.number().int().positive(),
    plannerMaxModelSteps: z.number().int().positive(),
    headlessMaxModelSteps: z.number().int().positive(),
    zeroDefaultMaxModelSteps: z.literal(0),
    delegateParentMaxModelSteps: z.number().int().positive(),
    delegateFloorParentMaxModelSteps: z.number().int().positive(),
    expected: z.object({
      default: z.literal(64),
      userGlobal: z.number().int().positive(),
      session: z.number().int().positive(),
      turn: z.number().int().positive(),
      planner: z.number().int().positive(),
      headless: z.number().int().positive(),
      zeroDefault: z.literal(64),
      delegateInherited: z.number().int().positive(),
      delegateFloorInherited: z.literal(5),
      dynamicLimitInStablePrefix: z.literal(false),
      errorCode: z.literal('turn_step_limit_exceeded'),
      matrix: z.array(z.object({
        scope: z.enum([
          'default',
          'userGlobal',
          'session',
          'turn',
          'planner',
          'headless',
          'zeroDefault',
          'delegate',
          'delegateFloor'
        ]),
        configured: z.number().int().min(0),
        fallback: z.number().int().min(0),
        effective: z.number().int().min(0),
        source: z.enum([
          'default',
          'user',
          'session',
          'turn',
          'planner',
          'headless',
          'parent-half'
        ]),
        dynamicLimitInStablePrefix: z.literal(false),
        disablesGuard: z.boolean(),
        delegateMinFloorApplied: z.boolean()
      })).length(9)
    })
  }),
  planner: z.object({
    availableToolset: z.array(z.string().min(1)).min(1),
    readOnlyToolset: z.array(z.string().min(1)).min(1),
    forbiddenToolset: z.array(z.string().min(1)).min(1),
    blockedToolset: z.array(z.string().min(1)).min(1),
    planToolName: z.literal('create_plan'),
    forgedToolName: z.string().min(1),
    normalModeAdvertised: z.array(z.string().min(1)).min(1),
    expectedCapabilityAdvertised: z.array(z.string().min(1)).min(1),
    expectedStep0Advertised: z.array(z.string().min(1)).min(1),
    expectedStep1Advertised: z.array(z.string().min(1)).min(1),
    expectedRejectedCall: z.object({
      toolName: z.string().min(1),
      status: z.literal('failed'),
      code: z.literal('tool_dispatch_rejected'),
      executed: z.literal(false)
    }),
    expectedRejectedCalls: z.array(z.object({
      toolName: z.string().min(1),
      status: z.literal('failed'),
      code: z.literal('tool_dispatch_rejected'),
      executed: z.literal(false)
    })).min(1),
    expected: z.object({
      normalModeHidesPlanTool: z.literal(true),
      capabilityGateAdvertised: z.array(z.string().min(1)).min(1),
      step0Advertised: z.array(z.string().min(1)).min(1),
      step1Advertised: z.array(z.string().min(1)).length(1),
      step0ExcludesBlockedTools: z.literal(true),
      step1OnlyCreatePlan: z.literal(true),
      rejectedToolNames: z.array(z.string().min(1)).min(1),
      rejectedCallCount: z.number().int().positive(),
      allForgedCallsRejected: z.literal(true),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  autoRouterClassifier: z.object({
    routerModel: z.literal('deepseek-v4-flash'),
    defaultTimeoutMs: z.literal(4000),
    fingerprint: z.string().length(16),
    isolatedRequest: z.object({
      turnIdSuffix: z.literal('_auto_router'),
      stream: z.literal(false),
      maxTokens: z.literal(96),
      temperature: z.literal(0),
      responseFormat: z.literal('json_object'),
      reasoningEffort: z.literal('off'),
      prefixItemCount: z.literal(0),
      toolCount: z.literal(0),
      carriesContextInstructions: z.literal(false)
    }),
    fingerprintDrift: z.object({
      routerModelChangeInvalidates: z.literal(true),
      systemPromptChangeInvalidates: z.literal(true),
      timeoutChangeInvalidates: z.literal(true),
      maxTokensChangeInvalidates: z.literal(true),
      temperatureChangeInvalidates: z.literal(true),
      reasoningEffortChangeInvalidates: z.literal(true),
      defaultFlashAliasStable: z.literal(true),
      responseFormatDefaultStable: z.literal(true)
    }),
    timeoutFallback: z.object({
      timeoutMs: z.literal(1),
      timeoutFingerprint: z.string().length(16),
      fallbackModel: z.literal('deepseek-v4-pro'),
      fallbackReasoningEffort: z.literal('max'),
      fallbackSource: z.literal('heuristic'),
      abortsClassifier: z.literal(true)
    }),
    recommendationParsing: z.array(z.object({
      id: z.enum([
        'pro-max-json',
        'flash-no-thinking-noise',
        'auto-rejected',
        'malformed-rejected'
      ]),
      raw: z.string().min(1),
      accepted: z.boolean(),
      expectedModel: z.enum(['deepseek-v4-flash', 'deepseek-v4-pro']).nullable(),
      expectedReasoningEffort: z.enum(['off', 'high', 'max']).nullable()
    })).length(4),
    contextBoundary: z.object({
      currentTurnId: z.literal('turn_3'),
      includedRows: z.array(z.string().min(1)).length(3),
      excludedLatestText: z.literal('latest'),
      activeTurnExcluded: z.literal(true),
      toolResultSummarized: z.literal(true)
    }),
    productBoundary: z.object({
      publicAutoPlanSetting: z.literal(false),
      projectAutoPlanOverride: z.literal(false),
      reasonixControllerProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    }),
    expected: z.object({
      fingerprintCurrent: z.literal(true),
      contractDriftInvalidatesCache: z.literal(true),
      requestIsolated: z.literal(true),
      timeoutFallsBackToHeuristic: z.literal(true),
      abortsTimedOutClassifier: z.literal(true),
      acceptedRecommendationCount: z.literal(2),
      rejectedRecommendationCount: z.literal(2),
      proMaxRecommendationAccepted: z.literal(true),
      autoRecommendationRejected: z.literal(true),
      malformedRecommendationRejected: z.literal(true),
      activeTurnExcludedFromRecentContext: z.literal(true),
      recentContextPreservesToolSummary: z.literal(true),
      stablePrefixContainsClassifierState: z.literal(false),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false)
    })
  }),
  combined: z.object({
    autoRouteCache: z.object({
      sameTurnRouterCalls: z.literal(1),
      mainModelSteps: z.number().int().positive(),
      nextTurnRouterCalls: z.literal(1),
      classifierStateInStablePrefix: z.literal(false)
    }),
    stepLimit: z.object({
      maxModelSteps: z.number().int().positive(),
      errorCode: z.literal('turn_step_limit_exceeded'),
      dynamicLimitInStablePrefix: z.literal(false)
    }),
    cancel: z.object({
      acceptedToolCalls: z.array(z.object({
        callId: z.string().min(1),
        state: z.enum(['completed', 'running', 'unstarted']),
        output: z.string()
      })).min(1),
      cancelledResultCode: z.literal('tool_call_cancelled')
    }),
    expected: z.object({
      sameTurnRouterCalls: z.literal(1),
      mainModelSteps: z.number().int().positive(),
      maxModelSteps: z.number().int().positive(),
      nextTurnRouterCalls: z.literal(1),
      routeCacheReusedUntilStepLimit: z.literal(true),
      nextTurnReroutes: z.literal(true),
      stepLimitErrorCode: z.literal('turn_step_limit_exceeded'),
      dynamicControlStateInStablePrefix: z.literal(false),
      cancelledResultCode: z.literal('tool_call_cancelled'),
      classifierStateInStablePrefix: z.literal(false),
      stepLimitDynamicInStablePrefix: z.literal(false),
      acceptedToolCallCount: z.literal(3),
      cancelResultCount: z.literal(3),
      completedResultCount: z.literal(1),
      abortedResultCount: z.literal(2),
      cancelResults: z.array(GoG5ToolResult).length(3),
      completedResultPreserved: z.literal(true),
      unstartedResultStatus: z.literal('aborted')
    })
  }),
  planStepCancelCache: z.object({
    mode: z.literal('plan'),
    model: z.literal('plan-cache-cancel'),
    requestCount: z.literal(3),
    abortedRunStatus: z.literal('aborted'),
    retryRunStatus: z.literal('failed'),
    step0MustAdvertise: z.array(z.enum(['create_plan', 'ls'])).length(2),
    step0MustNotAdvertise: z.array(z.enum(['bash'])).length(1),
    followUpRequiredToolName: z.literal('create_plan'),
    followUpTools: z.array(z.literal('create_plan')).length(1),
    retryMustAdvertise: z.array(z.enum(['create_plan', 'ls'])).length(2),
    usageEventCount: z.literal(2),
    provider: z.literal('deepseek'),
    endpointFormat: z.literal('chat_completions'),
    cacheHitTokens: z.literal(80),
    cacheMissTokens: z.literal(20),
    prefixChanged: z.literal(false),
    prefixChangeReasons: z.array(z.never()).length(0),
    productBoundary: z.object({
      publicAutoPlanSetting: z.literal(false),
      reasonixControllerProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false),
      defaultGoBackendEnabled: z.literal(false)
    }),
    expected: z.object({
      step0ReadOnlyPlusPlan: z.literal(true),
      followUpOnlyCreatePlan: z.literal(true),
      cancelledStepDoesNotAdvanceCacheBaseline: z.literal(true),
      nextPlanReusesOriginalCacheBaseline: z.literal(true),
      usageEventCount: z.literal(2),
      allPrefixChangedFalse: z.literal(true),
      cacheTelemetryPreserved: z.literal(true),
      forbiddenShellExcluded: z.literal(true),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false),
      defaultGoBackendEnabled: z.literal(false)
    })
  }),
  planCancelStateReset: z.object({
    sourceTestName: z.literal('does not leak cancelled plan mode into later normal or auto-routed turns'),
    previousMode: z.literal('plan'),
    abortedRunStatus: z.literal('aborted'),
    planToolName: z.literal('create_plan'),
    normalModel: z.literal('fixed-after-plan'),
    normalMustNotAdvertise: z.array(z.literal('create_plan')).length(1),
    normalModeInstructionPresent: z.literal(false),
    normalRequiredToolNamePresent: z.literal(false),
    autoRequestedModel: z.literal('auto'),
    autoRouterCalls: z.literal(1),
    autoRouterTurnIdSuffix: z.literal('_auto_router'),
    autoRouterToolCount: z.literal(0),
    autoRouterPrefixItemCount: z.literal(0),
    autoRouterModeInstructionPresent: z.literal(false),
    autoRealModel: z.literal('deepseek-v4-pro'),
    autoReasoningEffort: z.literal('max'),
    autoMustNotAdvertise: z.array(z.literal('create_plan')).length(1),
    autoModeInstructionPresent: z.literal(false),
    autoRequiredToolNamePresent: z.literal(false),
    stablePrefixContainsPlanState: z.literal(false),
    stablePrefixContainsClassifierState: z.literal(false),
    productBoundary: z.object({
      publicAutoPlanSetting: z.literal(false),
      projectAutoPlanOverride: z.literal(false),
      reasonixControllerProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false),
      defaultGoBackendEnabled: z.literal(false)
    }),
    expected: z.object({
      cancelledPlanDoesNotLeakMode: z.literal(true),
      normalTurnHidesCreatePlan: z.literal(true),
      normalTurnHasNoPlanRequirement: z.literal(true),
      autoTurnReroutesAfterCancel: z.literal(true),
      autoRouterRequestIsolated: z.literal(true),
      autoRecommendationCurrent: z.literal(true),
      autoTurnHidesCreatePlan: z.literal(true),
      autoTurnHasNoPlanRequirement: z.literal(true),
      stablePrefixClean: z.literal(true),
      usesReasonixProtocol: z.literal(false),
      topLevelRouteExposed: z.literal(false),
      defaultGoBackendEnabled: z.literal(false)
    })
  }),
  providerCacheReleaseGuard: GoProviderReleaseGuardControlCase,
  providerUsageParser: GoProviderUsageParserControlCase,
  providerRequestShape: GoProviderRequestShapeControlCase,
  providerLiveLocalHttpContract: GoProviderLiveLocalHttpContractControlCase,
  providerCacheCoverageFloor: GoProviderCacheCoverageFloorControlCase,
  providerDriftAttribution: GoProviderCacheDriftAttributionControlCase,
  providerCacheAccounting: GoProviderCacheAccountingControlCase,
  providerOfflineParitySeal: GoProviderOfflineParitySealControlCase,
  providerCachePrivacy: GoProviderCachePrivacyControlCase,
  providerCacheInventory: GoProviderCacheInventoryControlCase,
  providerStreaming: GoProviderStreamingControlCase,
  sessionRouteStatus: GoSessionRouteStatusControlCase,
  sessionRouteInventory: GoSessionRouteInventoryControlCase,
  sessionRouteReplay: GoSessionRouteReplayControlCase
})

export const GoG5FullLoopContract = z.object({
  schemaVersion: z.literal(RUNTIME_PARITY_FIXTURE_VERSION),
  id: z.string().min(1),
  stage: z.literal('G5'),
  mode: z.literal('shadow-conformance-only'),
  sourceContractIds: z.array(z.string().min(1)),
  productBoundary: ShadowOnlyProductBoundary,
  contractInventory: z.object({
    fullLoop: z.array(z.string().min(1)).min(1),
    jobManager: z.object({
      tests: z.array(z.string().min(1)).min(1),
      routeContract: z.array(z.string().min(1)).min(1),
      requiredBehaviors: z.array(z.string().min(1)).min(1)
    }),
    cacheCompaction: z.array(z.string().min(1)).min(1),
    resumeInterrupt: z.array(z.string().min(1)).min(1),
    mcpIndexer: z.array(z.string().min(1)).min(1)
  }),
  remainingBlockers: z.array(z.string().min(1)),
  controlReplay: GoG5ControlReplay,
  controlExecutableCases: GoG5ControlExecutableCases,
  shadowSlicesExpectedOutput: z.object({
    stage: z.literal('G5'),
    mode: z.literal('shadow-conformance-only'),
    sourceContractIds: z.array(z.string().min(1)),
    jobReplay: z.object({
      taskToolName: z.string().min(1),
      parallelToolName: z.string().min(1),
      toolContractBoundary: z.object({
        task: z.object({
          name: z.literal('task'),
          promptField: z.literal('prompt'),
          backgroundField: z.literal('run_in_background'),
          continueField: z.literal('continue_from'),
          forkField: z.literal('fork_from'),
          internalRuntimeOnly: z.literal(true),
          requiresPermissionGate: z.literal(true),
          mayAppendParentGoalEvidence: z.literal(true)
        }),
        parallelTasks: z.object({
          name: z.literal('parallel_tasks'),
          tasksField: z.literal('tasks'),
          dependencyField: z.literal('depends_on'),
          internalRuntimeOnly: z.literal(true),
          requiresDependencyValidation: z.literal(true),
          requiresPlannerReadOnlyToolset: z.literal(true)
        }),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      routes: z.array(z.string().min(1)).min(1),
      routeBoundary: z.object({
        protectedRoutes: z.array(z.enum(['wait', 'output', 'kill'])).length(3),
        unauthorizedStatus: z.literal(401),
        forbiddenTopLevelRoutes: z.array(z.string().min(1)).min(1)
      }),
      routeExecutable: z.object({
        unauthorizedStatus: z.literal(401),
        outputStatus: z.literal(200),
        outputJobStatus: z.literal('running'),
        outputNextOffset: z.literal(14),
        outputReplayOffset: z.literal(0),
        waitStatus: z.literal(200),
        waitJobStatus: z.literal('completed'),
        killStatus: z.literal(200),
        killJobStatus: z.literal('killed'),
        missingOutputStatus: z.literal(404),
        rehydratedOutputStatus: z.literal(200),
        rehydratedWaitStatus: z.literal(200),
        rehydratedKillStatus: z.literal(200),
        rehydratedCompletedStatus: z.literal('completed'),
        rehydratedCompletedNextOffset: z.literal(29),
        rehydratedKilledStatus: z.literal('killed')
      }),
      lifecycle: z.object({
        foreground: z.object({
          kind: z.literal('task'),
          status: z.literal('completed'),
          result: z.string().min(1)
        }),
        background: z.object({
          kind: z.literal('task'),
          statusAcrossTurn: z.literal('running'),
          output: z.string().min(1),
          finalStatus: z.literal('completed'),
          finalResult: z.string().min(1)
        }),
        waitOutputKill: z.object({
          killedStatus: z.literal('killed'),
          killedError: z.string().min(1)
        })
      }),
      dependencyOrder: z.array(z.string().min(1)).min(1),
      plannerReadOnlyToolset: z.array(z.string().min(1)).min(1),
      plannerForbiddenToolset: z.array(z.string().min(1)).min(1),
      nestedSseMetadataFields: z.array(z.string().min(1)).min(1),
      parentGoalEvidence: z.object({
        requiresActiveGoal: z.literal(true),
        evidenceLedgeredEventKey: z.literal('evidenceLedgered'),
        evidenceLedgerErrorEventKey: z.literal('evidenceLedgerError'),
        usesReasonixProtocol: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      sameTranscriptIdentityRequired: z.boolean(),
      continuePreservesTarget: z.boolean(),
      forkCreatesDistinctTarget: z.boolean(),
      plannerExecutor: z.object({
        plannerKind: z.literal('planner'),
        plannerPolicy: z.literal('readOnly'),
        executorPolicy: z.literal('inherit'),
        failureStatus: z.literal('failed'),
        cancelledStatus: z.literal('cancelled'),
        skippedReason: z.string().min(1),
        cancelReason: z.string().min(1),
        outputOffsetJobCount: z.number().int().positive(),
        requiresFailurePropagation: z.literal(true),
        requiresCancellationPropagation: z.literal(true),
        requiresTranscriptPropagation: z.literal(true),
        transcriptPropagationMode: z.enum(['continue', 'fork']),
        transcriptPropagationJobCount: z.number().int().positive()
      }),
      durableRunnerRestart: z.object({
        runningJobId: z.string().min(1),
        queuedJobId: z.string().min(1),
        rehydratedCount: z.number().int().positive(),
        waitStatus: z.literal('completed'),
        killStatus: z.literal('killed'),
        outputAfterRestart: z.string().min(1)
      }),
      approvalDenyNoExecute: z.object({
        deniedToolNames: z.array(z.string().min(1)).min(2),
        approvalIds: z.array(z.string().min(1)).min(2),
        createsDurableJobs: z.literal(false),
        createsChildRuns: z.literal(false)
      }),
      parallelValidation: z.object({
        dependencyField: z.literal('depends_on'),
        validOrder: z.array(z.string().min(1)).min(3),
        invalidCaseIds: z.array(z.enum([
          'single_task',
          'duplicate_id',
          'self_dependency',
          'cycle',
          'unknown_dependency'
        ])).length(5),
        singleTaskError: z.string().min(1),
        duplicateIdError: z.string().min(1),
        selfDependencyError: z.string().min(1),
        cycleError: z.string().min(1),
        unknownDependencyError: z.string().min(1)
      })
    }),
    cacheReplay: z.object({
      stablePrefixHash: z.string().min(1),
      toolsHash: z.string().min(1),
      providerUsageCaseIds: z.array(z.string().min(1)).min(1),
      requestShapeCaseIds: z.array(z.string().min(1)).min(1),
      requestShapeReplay: GoProviderRequestShapeSummaryOutput,
      releaseGuardStatuses: z.array(z.string().min(1)).min(1),
      releaseGuard: GoProviderReleaseGuardOutput,
      cacheAccounting: GoProviderCacheAccounting,
      usageParserReplay: GoProviderUsageParserOutput,
      liveLocalHttpContract: ProviderLiveLocalHttpContract,
      offlineParitySeal: GoProviderOfflineParitySealOutput,
      providerCachePrivacy: GoProviderCachePrivacyOutput,
      driftAttribution: z.object({
        previousPrefixHash: z.string().min(1),
        currentPrefixHash: z.string().min(1),
        systemHashStable: z.literal(true),
        prefixItemsHashStable: z.literal(true),
        toolsHashChanged: z.literal(true),
        providerChanged: z.literal(true),
        modelChanged: z.literal(true),
        endpointFormatChanged: z.literal(true),
        expectedReasons: z.array(z.enum(['tools', 'provider', 'model'])).length(3),
        telemetrySupported: z.literal(false),
        cacheHitRateKnown: z.literal(false)
      }),
      forbiddenDiagnosticsSubstrings: z.array(z.string().min(1)),
      liveSuperiorityClaimAllowed: z.literal(false)
    }),
    providerStreamingReplay: GoProviderStreamingReplayOutput,
    sessionReplay: z.object({
      routeIds: z.array(z.string().min(1)).min(1),
      resumeRouteIds: z.array(z.string().min(1)),
      forkRouteIds: z.array(z.string().min(1)),
      sseRouteIds: z.array(z.string().min(1)),
      routeStatusReplay: GoSessionRouteStatusReplayOutput
    }),
    approvalUserInputReplay: GoG5ApprovalUserInputReplayOutput,
    controlReplay: GoG5ControlReplay,
    mcpReplay: z.object({
      providerId: z.string().min(1),
      lifecycle: GoMcpCoreLifecycleOutput,
      retryFailedServerIds: z.array(z.string().min(1)),
      expectedConnectedServerIds: z.array(z.string().min(1)),
      backgroundReconnect: GoMcpBackgroundReconnectOutput,
      knownOverride: z.enum(['codegraph', 'codebase-memory']),
      knownOverrideDiagnostics: z.array(z.object({
        serverId: z.string().min(1),
        knownOverride: z.enum(['codegraph', 'codebase-memory']),
        effectiveCwd: z.string().min(1),
        lowPriority: z.literal(true),
        backgroundStart: z.literal(true),
        workspaceRoot: z.string().min(1),
        explicitCwd: z.string().min(1).optional(),
        daemonIdleTimeoutMs: z.string().min(1).optional()
      })).min(2),
      liveLocalActivePaths: z.array(z.string().min(1)),
      lateTombstonePath: z.string().min(1),
      secretSafeDiagnostic: z.string().min(1),
      liveLocalIndexer: z.object({
        serverId: z.string().min(1),
        cwd: z.string().min(1),
        lowPriority: z.literal(true),
        backgroundStart: z.literal(true),
        retryAttempts: z.record(z.string(), z.number().int().positive()),
        activePaths: z.array(z.string().min(1)),
        tombstoneCount: z.number().int().nonnegative(),
        restartedFromSnapshot: z.literal(true),
        lateTombstonePath: z.string().min(1),
        secretSafeDiagnostic: z.string().min(1),
        leaksSecret: z.literal(false),
        topLevelRouteExposed: z.literal(false)
      }),
      approvalAnnotations: GoMcpApprovalAnnotationOutput,
      searchMetaToolNames: z.array(z.enum([
        'mcp_search',
        'mcp_describe',
        'mcp_call',
        'mcp_refresh_catalog'
      ])).length(4),
      searchTrustedToolId: z.string().min(1),
      searchUnknownToolError: z.string().min(1),
      searchUntrustedSearchedTools: z.literal(0),
      searchCallPolicy: z.literal('on-request'),
      searchCallDeniedNoExecute: z.literal(true),
      searchRefreshDrift: GoMcpSearchRefreshDriftOutput,
      searchWorkspaceBoundary: GoMcpSearchWorkspaceBoundaryOutput
    }),
    productBoundary: ShadowOnlyProductBoundary
  }),
  expectedOutput: z.object({
    stage: z.literal('G5'),
    mode: z.literal('shadow-conformance-only'),
    sourceContractIds: z.array(z.string().min(1)),
    tsOwnedContractInventory: z.literal(true),
    electronMainConnected: z.literal(false),
    defaultGoBackendEnabled: z.literal(false),
    fullLoopTests: z.array(z.string().min(1)).min(1),
    jobManager: z.object({
      tests: z.array(z.string().min(1)).min(1),
      routes: z.array(z.string().min(1)).min(1),
      requiredBehaviors: z.array(z.string().min(1)).min(1)
    }),
    cacheCompactionTests: z.array(z.string().min(1)).min(1),
    resumeInterruptTests: z.array(z.string().min(1)).min(1),
    mcpIndexerTests: z.array(z.string().min(1)).min(1),
    remainingBlockers: z.array(z.string().min(1)),
    productBoundary: ShadowOnlyProductBoundary
  })
})
export type GoG5FullLoopContract = z.infer<typeof GoG5FullLoopContract>
