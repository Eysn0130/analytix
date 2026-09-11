export const GO_RUNTIME_CONFORMANCE_VERSION = 1

export const GO_RUNTIME_KERNEL_COMPONENTS = [
  'Provider Registry',
  'Tool Registry',
  'Controller',
  'Session',
  'Event Sink',
  'Job Manager',
  'Permission Gate',
  'MCP Client',
  'Memory/History Retrieval',
  'Goal Runtime'
] as const

export type GoRuntimeKernelComponent = typeof GO_RUNTIME_KERNEL_COMPONENTS[number]
export type GoRuntimeStage = 'G0' | 'G1' | 'G2' | 'G3' | 'G4' | 'G5' | 'G6'
export type GoRuntimeConformanceStatus =
  | 'ts-contract-ready'
  | 'ts-contract-partial'
  | 'go-pending'

export type GoRuntimeKernelComponentMapping = {
  component: GoRuntimeKernelComponent
  goStages: GoRuntimeStage[]
  tsAuthority: string[]
  tsContractTests: string[]
  status: GoRuntimeConformanceStatus
  contractRequiredBeforeGo: string[]
}

export type GoRuntimeShadowRouteContract = {
  id: string
  stage: 'G1' | 'G2'
  method: 'GET' | 'PATCH' | 'POST'
  path: string
  auth: 'none' | 'runtime-token'
  tsAuthority: string
  responseContract: string
}

export type GoRuntimeG5ContractInventoryRef = {
  id: string
  stage: 'G5'
  fixture: string
  tsOwned: true
  shadowOnly: true
}

export type GoRuntimeKernelConformanceManifest = {
  schemaVersion: typeof GO_RUNTIME_CONFORMANCE_VERSION
  mode: 'shadow-conformance-only'
  productBoundary: {
    rendererPreloadMainBridgeUnchanged: true
    analytixServeContractUnchanged: true
    reasonixPublicProtocolAllowed: false
    defaultGoBackendAllowed: false
    rendererVisibleGoRoutesAllowed: false
  }
  components: GoRuntimeKernelComponentMapping[]
  g1ShadowRoutes: GoRuntimeShadowRouteContract[]
  g2ShadowRoutes: GoRuntimeShadowRouteContract[]
  g5ContractInventory: GoRuntimeG5ContractInventoryRef[]
}

export function buildGoRuntimeKernelConformanceManifest(): GoRuntimeKernelConformanceManifest {
  return {
    schemaVersion: GO_RUNTIME_CONFORMANCE_VERSION,
    mode: 'shadow-conformance-only',
    productBoundary: {
      rendererPreloadMainBridgeUnchanged: true,
      analytixServeContractUnchanged: true,
      reasonixPublicProtocolAllowed: false,
      defaultGoBackendAllowed: false,
      rendererVisibleGoRoutesAllowed: false
    },
    components: [
      {
        component: 'Provider Registry',
        goStages: ['G1', 'G3'],
        tsAuthority: [
          'packages/runtime/src/contracts/runtime-info.ts',
          'packages/runtime/src/contracts/capabilities.ts',
          'packages/runtime/src/model-test-support/model/compat-model-client.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/http-server.test.ts',
          'packages/runtime/tests/model-client.test.ts',
          'packages/runtime/tests/provider-cache-contract.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'health/info capability snapshot matches TypeScript runtime',
          'provider request URL/body/header/stream/usage fixtures stay identical',
          'unsupported cache telemetry remains unknown',
          'offline cache curve guard stays green before any live-provider claim'
        ]
      },
      {
        component: 'Tool Registry',
        goStages: ['G1', 'G4'],
        tsAuthority: [
          'packages/runtime/src/tool-test-support/tool/capability-registry.ts',
          'packages/runtime/src/tool-test-support/tool/local-tool-host.ts',
          'packages/runtime/src/cache/tool-catalog-fingerprint.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/capability-registry.test.ts',
          'packages/runtime/tests/mcp-tool-provider.test.ts',
          'packages/runtime/tests/builtin-tools.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'dynamic source connect/disconnect diagnostics match TypeScript registry',
          'canonical tool catalog fingerprint remains deterministic',
          'provider-owned advertisement order remains intact'
        ]
      },
      {
        component: 'Controller',
        goStages: ['G1', 'G5'],
        tsAuthority: [
          'packages/runtime/src/loop-test-support/agent-loop.ts',
          'packages/runtime/src/services-test-support/turn-service.ts',
          'packages/runtime/src/services-test-support/remote-entry-control-port.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/loop.test.ts',
          'packages/runtime/tests/remote-entry-control-port.test.ts',
          'packages/runtime/tests/approval-user-input-route-contract.test.ts',
          'packages/runtime/tests/thread-service.test.ts'
        ],
        status: 'ts-contract-partial',
        contractRequiredBeforeGo: [
          'remote entry stays lifecycle/turn/approval/user-input only',
          'turn steering, interrupt, and resume preserve event ordering',
          'controller locks cannot block approval/status reads'
        ]
      },
      {
        component: 'Session',
        goStages: ['G1', 'G2'],
        tsAuthority: [
          'packages/runtime/src/services-test-support/thread-service.ts',
          'packages/runtime/src/adapters/file/file-session-store.ts',
          'packages/runtime/src/adapters/file/file-thread-store.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/thread-service.test.ts',
          'packages/runtime/tests/file-session-store.test.ts',
          'packages/runtime/tests/hybrid-store.test.ts',
          'packages/runtime/tests/go-durable-sidecar-conformance.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'thread list/search/archive/fork/session summaries match TypeScript',
          'events.jsonl replay and malformed-line recovery match TypeScript',
          'sidecar summary authority remains backend-neutral'
        ]
      },
      {
        component: 'Event Sink',
        goStages: ['G1', 'G2', 'G5'],
        tsAuthority: [
          'packages/runtime/src/contracts/events.ts',
          'packages/runtime/src/services-test-support/runtime-event-recorder.ts',
          'packages/runtime/src/domain/runtime-event-reducer.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/runtime-event-recorder.test.ts',
          'packages/runtime/tests/runtime-event-reducer.test.ts',
          'packages/runtime/tests/go-durable-sidecar-conformance.test.ts',
          'src/renderer/src/agent/analytix-mapper.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'SSE sequence and durable event log match TypeScript',
          'nested child metadata remains backend-neutral',
          'renderer projection does not expose Go or Reasonix event semantics'
        ]
      },
      {
        component: 'Job Manager',
        goStages: ['G1', 'G5'],
        tsAuthority: [
          'packages/runtime/src/delegation-test-support/delegation-runtime.ts',
          'packages/runtime/src/delegation-test-support/job-manager.ts',
          'packages/runtime/src/server-test-support/routes/task-jobs.ts',
          'packages/runtime/src/loop-test-support/agent-loop.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/task-job-orchestration-contract.test.ts',
          'packages/runtime/tests/delegation-runtime.test.ts',
          'packages/runtime/tests/child-agent-executor.test.ts',
          'packages/runtime/tests/loop.test.ts'
        ],
        status: 'ts-contract-partial',
        contractRequiredBeforeGo: [
          'delegate_task fan-out, maxParallel queueing, abort, failure, and interruption match TypeScript',
          'completed child work can append parent Goal evidence',
          'first-class task/parallel/background/planner contract covers dependency/cycle, wait/output/kill, nested SSE metadata, parent Goal evidence, and transcript continue/fork identity'
        ]
      },
      {
        component: 'Permission Gate',
        goStages: ['G1', 'G4', 'G5'],
        tsAuthority: [
          'packages/runtime/src/tool-test-support/tool/local-tool-host.ts',
          'packages/runtime/src/loop-test-support/agent-loop.ts',
          'packages/runtime/src/contracts/policy.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/approval-user-input-route-contract.test.ts',
          'packages/runtime/tests/builtin-tools.test.ts',
          'packages/runtime/tests/agent-loop-sandbox.test.ts',
          'packages/runtime/tests/child-agent-executor.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'denied approval never executes tool body',
          'ask/auto/yolo posture matrix matches TypeScript',
          'headless/subagent execution inherits or narrows parent policy'
        ]
      },
      {
        component: 'MCP Client',
        goStages: ['G1', 'G4'],
        tsAuthority: [
          'packages/runtime/src/contracts/capabilities.ts',
          'packages/runtime/src/tool-test-support/tool/mcp-tool-provider.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts',
          'packages/runtime/tests/mcp-config.test.ts',
          'packages/runtime/tests/mcp-tool-provider.test.ts',
          'packages/runtime/tests/capability-registry.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'MCP config normalization and malformed schema fallback match TypeScript',
          'tool diagnostics redact secrets',
          'dynamic MCP source lifecycle preserves stable tool schema',
          'retry-all failed startup servers, late provider suspension, and codegraph/codebase-memory cwd/low-priority/background overrides match TypeScript'
        ]
      },
      {
        component: 'Memory/History Retrieval',
        goStages: ['G1', 'G3', 'G5'],
        tsAuthority: [
          'packages/runtime/src/memory/memory-store.ts',
          'packages/runtime/src/shared/context-compactor.ts',
          'packages/runtime/src/loop-test-support/agent-loop.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/memory-store.test.ts',
          'packages/runtime/tests/request-history-hygiene.test.ts',
          'packages/runtime/tests/context-compactor.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'history repair keeps provider tool-call history valid',
          'memory retrieval remains approval-gated where mutating',
          'compaction archive and token economy snapshots match TypeScript'
        ]
      },
      {
        component: 'Goal Runtime',
        goStages: ['G1', 'G5'],
        tsAuthority: [
          'packages/runtime/src/contracts/threads.ts',
          'packages/runtime/src/services-test-support/thread-service.ts',
          'packages/runtime/src/tool-test-support/tool/goal-tools.ts',
          'packages/runtime/src/research/autoresearch-store.ts'
        ],
        tsContractTests: [
          'packages/runtime/tests/goal-tools.test.ts',
          'packages/runtime/tests/goal-repetition-guard.test.ts',
          'packages/runtime/tests/autoresearch-store.test.ts'
        ],
        status: 'ts-contract-ready',
        contractRequiredBeforeGo: [
          'complete_step evidence ledger remains append-only',
          'goal completion without evidence is rejected at tool and service layers',
          'AutoResearch requirement audit and blocked-state events match TypeScript'
        ]
      }
    ],
    g5ContractInventory: [
      {
        id: 'go-g5-full-loop-contract-v1',
        stage: 'G5',
        fixture: 'packages/runtime/src/conformance/fixtures/go-g5-full-loop-contract.json',
        tsOwned: true,
        shadowOnly: true
      }
    ],
    g1ShadowRoutes: [
      {
        id: 'health',
        stage: 'G1',
        method: 'GET',
        path: '/health',
        auth: 'none',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/health.ts',
        responseContract: '{"status":"ok","service":"analytix","mode":"serve"}'
      },
      {
        id: 'runtime-info',
        stage: 'G1',
        method: 'GET',
        path: '/v1/runtime/info',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/contracts/runtime-info.ts',
        responseContract: 'RuntimeInfoResponse'
      },
      {
        id: 'runtime-tools',
        stage: 'G1',
        method: 'GET',
        path: '/v1/runtime/tools',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/contracts/runtime-tools.ts',
        responseContract: 'RuntimeToolsResponse public diagnostics v2'
      }
    ],
    g2ShadowRoutes: [
      {
        id: 'thread-list-default',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ListThreadsResponse active default filter'
      },
      {
        id: 'thread-archive-patch',
        stage: 'G2',
        method: 'PATCH',
        path: '/v1/threads/thr_g2_beta',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ThreadSchema archived update'
      },
      {
        id: 'thread-list-archived-only',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads?archived_only=true',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ListThreadsResponse archived_only filter'
      },
      {
        id: 'thread-search-archived',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads?include_archived=true&search=archive',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ListThreadsResponse include_archived search filter'
      },
      {
        id: 'thread-read-detail',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads/thr_g2_read',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ThreadSchema plus latestSeq'
      },
      {
        id: 'thread-update-title-workspace',
        stage: 'G2',
        method: 'PATCH',
        path: '/v1/threads/thr_g2_read',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/threads.ts',
        responseContract: 'ThreadSchema title/workspace update'
      },
      {
        id: 'thread-fork-side',
        stage: 'G2',
        method: 'POST',
        path: '/v1/threads/thr_g2_parent/fork',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/services-test-support/thread-service.ts',
        responseContract: 'ThreadSchema side fork lineage snapshot'
      },
      {
        id: 'session-resume-thread',
        stage: 'G2',
        method: 'POST',
        path: '/v1/sessions/thr_g2_source/resume-thread',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/sessions.ts',
        responseContract: 'resume-thread compatibility response'
      },
      {
        id: 'events-since-seq',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=1',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/events.ts',
        responseContract: 'SSE replay frames with seq greater than since_seq'
      },
      {
        id: 'events-caught-up-since-seq',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=3',
        auth: 'runtime-token',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/events.ts',
        responseContract: 'SSE replay returns no backlog when since_seq is current'
      },
      {
        id: 'events-unauthorized-since-seq',
        stage: 'G2',
        method: 'GET',
        path: '/v1/threads/thr_g2_events/events?since_seq=0',
        auth: 'none',
        tsAuthority: 'packages/runtime/src/server-test-support/routes/events.ts',
        responseContract: 'SSE replay route rejects missing runtime token'
      }
    ]
  }
}
