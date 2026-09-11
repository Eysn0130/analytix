import type { LocalTool } from './local-tool-host.js'

export type LocalToolDefinition = Omit<LocalTool, 'policy' | 'toolKind'> & {
  policy?: LocalTool['policy']
  toolKind?: LocalTool['toolKind']
  snipHint?: LocalTool['snipHint']
}

export function defineLocalTool(tool: LocalToolDefinition): LocalTool {
  return {
    policy: tool.policy ?? 'on-request',
    name: tool.name,
    description: tool.description,
    inputSchema: tool.inputSchema,
    toolKind: tool.toolKind ?? 'tool_call',
    ...(tool.snipHint ? { snipHint: tool.snipHint } : {}),
    execute: tool.execute,
    ...(tool.shouldAdvertise ? { shouldAdvertise: tool.shouldAdvertise } : {})
  }
}
