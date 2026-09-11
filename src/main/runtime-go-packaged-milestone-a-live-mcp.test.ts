import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { describe, expect, it } from 'vitest'

const milestoneScriptPath = join(
  process.cwd(),
  'scripts/runtime-go-packaged-milestone-a.mjs'
)

async function milestoneModule(): Promise<any> {
  return import(`${pathToFileURL(milestoneScriptPath).href}?live-mcp=${Date.now()}`)
}

function runtimePublicSeamObservation(
  mcpServers: Record<string, unknown>[]
): Record<string, unknown> {
  return {
    health: { service: 'analytix' },
    runtimeInfo: {
      schemaVersion: 2,
      status: 'ready',
      listenerScope: 'loopback',
      port: 43210,
      insecure: false,
      storage: { configured: true, available: true },
      executionPolicy: {
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      }
    },
    runtimeTools: {
      schemaVersion: 2,
      providerCount: 1,
      toolContracts: { count: 4, catalogHash: 'a'.repeat(64) },
      mcpServers,
      commands: []
    },
    runtimeSkills: null,
    runtimeThreadListProbeOk: true,
    runtimeThreadListProbeStatus: 200
  }
}

function connectedLiveSchedule(
  overrides: Record<string, unknown> = {}
): Record<string, unknown> {
  return {
    id: 'gui_schedule',
    status: 'connected',
    enabled: true,
    available: true,
    connected: true,
    transport: 'stdio',
    authStatus: 'none',
    trustScope: 'user',
    schemaHintAvailable: false,
    connectable: false,
    toolCount: 8,
    toolContractQuarantineCount: 0,
    ...overrides
  }
}

describe('packaged Milestone A live ordinary MCP evidence', () => {
  it('accepts an exact healthy live catalog after its cached schema hint is consumed', async () => {
    const { runtimePublicSeamEvidence } = await milestoneModule()
    const evidence = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([connectedLiveSchedule()]),
      43210
    )

    expect(evidence).toEqual(expect.objectContaining({
      ok: true,
      ordinaryMCPAvailable: true,
      ordinaryMCPServerCount: 1,
      ordinaryMCPServerId: 'gui_schedule',
      ordinaryMCPToolCount: 8
    }))
  })

  it('still rejects a live catalog with any quarantined tool contract', async () => {
    const { runtimePublicSeamEvidence } = await milestoneModule()
    const evidence = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([
        connectedLiveSchedule({ toolContractQuarantineCount: 1 })
      ]),
      43210
    )

    expect(evidence.ordinaryMCPAvailable).toBe(false)
  })
})
