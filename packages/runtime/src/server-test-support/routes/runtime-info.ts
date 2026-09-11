import { RuntimeInfoResponse } from '../../contracts/runtime-info.js'
import { RuntimeToolsResponse, type RuntimeToolsResponse as RuntimeToolsResponseType } from '../../contracts/runtime-tools.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import type { ServerRuntime } from './server-runtime.js'

export function runtimeInfoJsonResponse(runtime: ServerRuntime): JsonResponse {
  return jsonResponse(RuntimeInfoResponse.parse(runtime.info()))
}

export async function runtimeToolDiagnosticsJsonResponse(runtime: ServerRuntime): Promise<JsonResponse> {
  return jsonResponse(RuntimeToolsResponse.parse(await (runtime.toolDiagnostics?.() ?? emptyRuntimeToolsResponse())))
}

function emptyRuntimeToolsResponse(): RuntimeToolsResponseType {
  return {
    schemaVersion: 2,
    providerCount: 0,
    toolContracts: { count: 0, catalogHash: 'a'.repeat(64) },
    mcpServers: [],
    mcpSearch: {
      enabled: false,
      mode: 'auto',
      active: false,
      available: false,
      reasonCode: 'disabled_by_config',
      indexedToolCount: 0,
      advertisedToolCount: 0,
      autoThresholdToolCount: 0,
      topKDefault: 0,
      topKMax: 0,
      minScore: 0,
      catalogDrift: false
    },
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: {
      mode: 'off',
      configured: false,
      source: 'unknown',
      valid: true,
      credentialsMasked: true
    },
    webProviderCount: 0,
    skills: {
      enabled: false,
      available: false,
      reasonCode: 'disabled_by_config',
      configuredRootCount: 0,
      skillCount: 0,
      validationErrorCount: 0
    },
    attachments: {
      enabled: false,
      count: 0,
      totalBytes: 0,
      maxImageBytes: 0,
      maxImageDimension: 0,
      allowedMimeTypes: [],
      allowedDocumentMimeTypes: [],
      maxDocumentBytes: 0,
      maxDocumentTextChars: 0
    },
    memory: {
      enabled: false,
      status: 'unavailable',
      reasonCode: 'memory_store_unavailable'
    },
    subagents: {
      status: 'disabled',
      enabled: false,
      available: false,
      reasonCode: 'disabled_by_config',
      active: 0,
      queued: 0,
      profileCount: 0,
      maxParallel: 0,
      maxChildRuns: 0,
      defaultToolPolicy: 'readOnly',
      internalLineageAvailable: false,
      parallelExecutionAvailable: false,
      taskToolAvailable: false,
      parallelTasksToolAvailable: false,
      backgroundTaskJobsAvailable: false,
      backgroundShellAvailable: false,
      backgroundSubagentJobsAvailable: false
    }
  }
}
