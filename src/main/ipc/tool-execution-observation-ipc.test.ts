import { describe, expect, it } from 'vitest'
import { ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH } from '../../shared/analytix-endpoints'
import { runtimeRequestPayloadSchema } from './app-ipc-schemas'

describe('tool execution observation IPC boundary', () => {
  it('allows only the exact authenticated-host POST target', () => {
    const body = JSON.stringify({
      threadId: 'thread-1',
      turnId: 'turn-1',
      toolName: 'bash',
      workspace: '/tmp/analytix-acceptance',
      arguments: { command: 'npm test' }
    })
    expect(runtimeRequestPayloadSchema.parse({
      path: ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH,
      method: 'POST',
      body
    })).toEqual({
      path: ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH,
      method: 'POST',
      body
    })

    for (const payload of [
      { path: ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH, method: 'GET' },
      { path: `${ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH}?`, method: 'POST' },
      { path: `${ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH}?replay=1`, method: 'POST' },
      { path: `${ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH}#`, method: 'POST' },
      { path: `${ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH}#replay`, method: 'POST' },
      { path: '/v1/runtime/ignored/../tool-executions/observe', method: 'POST' }
    ]) {
      expect(runtimeRequestPayloadSchema.safeParse(payload).success).toBe(false)
    }
  })
})
