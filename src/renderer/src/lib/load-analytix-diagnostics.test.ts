import { describe, expect, it } from 'vitest'
import { formatRuntimeError } from './format-runtime-error'
import { loadAnalytixDiagnostics } from './load-analytix-diagnostics'

describe('loadAnalytixDiagnostics', () => {
  it('loads runtime info, tool diagnostics, and memory records together', async () => {
    const runtimeInfo = { pid: 42, capabilities: { model: { id: 'deepseek-v4-pro' } } } as any
    const toolDiagnostics = { providers: [{ id: 'builtin' }] } as any
    const memoryRecords = [{ id: 'mem_1', content: 'remember this' }] as any
    const provider = {
      getRuntimeInfo: async () => runtimeInfo,
      getToolDiagnostics: async () => toolDiagnostics,
      listMemories: async () => memoryRecords
    }

    const loaded = await loadAnalytixDiagnostics(provider, { workspace: '/tmp/project' })

    expect(loaded.runtimeInfo).toBe(runtimeInfo)
    expect(loaded.toolDiagnostics).toBe(toolDiagnostics)
    expect(loaded.memoryRecords).toBe(memoryRecords)
    expect(loaded.errors).toEqual([])
  })

  it('keeps successful diagnostics when memory loading fails', async () => {
    const runtimeInfo = { pid: 42 } as any
    const toolDiagnostics = { providers: [{ id: 'builtin' }], mcpServers: [] } as any
    const provider = {
      getRuntimeInfo: async () => runtimeInfo,
      getToolDiagnostics: async () => toolDiagnostics,
      listMemories: async () => {
        throw new Error('memory store is unavailable')
      }
    }

    const loaded = await loadAnalytixDiagnostics(provider, { workspace: '/tmp/project' })

    expect(loaded.runtimeInfo).toBe(runtimeInfo)
    expect(loaded.toolDiagnostics).toBe(toolDiagnostics)
    expect(loaded.memoryRecords).toEqual([])
    expect(loaded.errors).toEqual([`Memory: ${formatRuntimeError(new Error('memory store is unavailable'))}`])
  })

  it('clears stale diagnostics when a refresh rejects', async () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const provider = {
      getRuntimeInfo: async () => { throw new Error(privateSentinel) },
      getToolDiagnostics: async () => { throw new Error(privateSentinel) },
      listMemories: async () => { throw new Error(privateSentinel) }
    }

    const loaded = await loadAnalytixDiagnostics(provider)

    expect(loaded.runtimeInfo).toBeNull()
    expect(loaded.toolDiagnostics).toBeNull()
    expect(loaded.memoryRecords).toEqual([])
    expect(JSON.stringify(loaded)).not.toContain(privateSentinel)
    expect(loaded.errors).toHaveLength(3)
  })
})
