import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { compactTurn } from '../src/server-test-support/routes/turns.js'
import { createApprovalRequest } from '../src/domain/approval.js'
import { makeAssistantTextItem, makeToolCallItem, makeToolResultItem } from '../src/domain/item.js'
import { encodeSseEvent } from '../src/server-test-support/sse.js'
import { buildHarness, readJson, readSseEvents, usageSnapshot } from './http-server-test-harness.js'
import type { TurnItem } from '../src/contracts/items.js'
import type { RuntimeToolsResponse } from '../src/contracts/runtime-tools.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../src/ports/model-client.js'
import type { SessionLatestUsageSnapshot } from '../src/ports/session-store.js'

describe('HTTP server', () => {
  let dataDir = ''
  beforeEach(async () => {
    dataDir = await mkdtemp(join(tmpdir(), 'analytix-http-'))
  })
  afterEach(async () => {
    await rm(dataDir, { recursive: true, force: true })
  })

  it('returns 200 on /health without auth', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(h.router, new Request('http://localhost/health'))
    expect(response.status).toBe(200)
    const body = await readJson(response)
    expect(body).toEqual({ status: 'ok', service: 'analytix', mode: 'serve' })
  })

  it('passes request abort signals to manual compaction', async () => {
    const controller = new AbortController()
    let seenSignal: AbortSignal | undefined
    const turns = {
      async compact(input: { signal?: AbortSignal }) {
        seenSignal = input.signal
        return {
          threadId: 'thr_compact',
          replacedTokens: 0,
          summary: 'nothing to compact',
          pinnedConstraints: []
        }
      }
    } as unknown as Parameters<typeof compactTurn>[0]

    const response = await compactTurn(
      turns,
      'thr_compact',
      new Request('http://localhost/v1/threads/thr_compact/compact', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ reason: 'manual' }),
        signal: controller.signal
      })
    )

    expect(response.status).toBe(200)
    expect(seenSignal?.aborted).toBe(false)
    controller.abort()
    expect(seenSignal?.aborted).toBe(true)
  })

  it('returns runtime info with disabled capability defaults', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/info', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      schemaVersion?: number
      provider?: { model?: string }
      capabilities?: {
        contractVersion?: number
        mcp?: { available?: boolean; reasonCode?: string }
        web?: { available?: boolean; fetch?: { available?: boolean } }
        attachments?: { available?: boolean; allowedMimeTypes?: string[] }
        cli?: { serve?: { available?: boolean }; run?: { available?: boolean; reasonCode?: string } }
        model?: { inputModalities?: string[]; supportsToolCalling?: boolean; contextWindowTokens?: number }
      }
    }
    expect(body.schemaVersion).toBe(2)
    expect(body.provider?.model).toBe('deepseek-chat')
    expect(body.capabilities?.contractVersion).toBe(1)
    expect(body.capabilities?.model?.inputModalities).toContain('text')
    expect(body.capabilities?.model?.supportsToolCalling).toBe(true)
    expect(body.capabilities?.model?.contextWindowTokens).toBe(1_000_000)
    expect(body.capabilities?.mcp?.available).toBe(false)
    expect(body.capabilities?.mcp?.reasonCode).toBe('disabled_by_config')
    expect(body.capabilities?.web?.fetch?.available).toBe(false)
    expect(body.capabilities?.attachments?.allowedMimeTypes).toContain('image/png')
    expect(body.capabilities?.cli?.serve?.available).toBe(true)
    expect(body.capabilities?.cli?.run?.available).toBe(false)
  })

  it('requires auth for runtime info', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/info')
    )

    expect(response.status).toBe(401)
  })

  it('returns structured validation errors for invalid JSON bodies', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: '{'
      })
    )

    expect(response.status).toBe(400)
    expect(await readJson(response)).toMatchObject({
      code: 'validation_error',
      message: 'The request did not satisfy the runtime contract.'
    })
  })

  it('returns runtime tool diagnostics', async () => {
    const h = buildHarness()
    const baselineResponse = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const baseline = await readJson(baselineResponse) as RuntimeToolsResponse
    h.runtime.toolDiagnostics = () => ({
      ...baseline,
      providerCount: 1,
      toolContracts: { count: 1, catalogHash: 'b'.repeat(64) },
      mcpServers: [{
        id: 'github',
        status: 'error',
        failureCode: 'connection_failed',
        transport: 'stdio',
        authStatus: 'required',
        trustScope: 'user',
        enabled: true,
        available: false,
        connected: false,
        schemaHintAvailable: true,
        connectable: true,
        toolCount: 0,
        promptCount: 0,
        resourceCount: 0,
        toolContractQuarantineCount: 1,
        lowPriority: false,
        backgroundStart: false
      }]
    })
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      schemaVersion: number
      providerCount: number
      toolContracts: { count: number; catalogHash: string }
      mcpServers: Array<{ id: string; failureCode?: string; lastError?: string }>
      mcpPromptCount: number
      mcpResourceCount: number
      networkProxy: { configured: boolean; summary?: string; valid: boolean }
      providers?: unknown
    }
    expect(body.schemaVersion).toBe(2)
    expect(body.providerCount).toBe(1)
    expect(body.providers).toBeUndefined()
    expect(body.toolContracts).toEqual({ count: 1, catalogHash: 'b'.repeat(64) })
    expect(body.mcpServers[0]).toMatchObject({
      id: 'github',
      failureCode: 'connection_failed'
    })
    expect(body.mcpServers[0]?.lastError).toBeUndefined()
    expect(body.mcpPromptCount).toBe(0)
    expect(body.mcpResourceCount).toBe(0)
    expect(body.networkProxy).toMatchObject({ configured: false, valid: true })
    expect(body.networkProxy.summary).toBeUndefined()
  })

  it('requires auth for runtime tool diagnostics', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools')
    )

    expect(response.status).toBe(401)
  })

  it('lists discovered skills through the HTTP layer', async () => {
    const h = buildHarness()
    h.runtime.skills = () => ({
      enabled: true,
      roots: ['/tmp/skills'],
      skills: [
        {
          id: 'review',
          name: 'Review',
          description: 'Review the current change',
          version: '1.0.0',
          root: '/tmp/skills/review',
          scope: 'project',
          legacy: false,
          triggers: { commands: ['/review'], promptPatterns: [], fileTypes: [] },
          allowedTools: ['read']
        }
      ],
      validationErrors: [],
      lastActivations: []
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/skills', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = await readJson(response) as {
      schemaVersion: number
      skillCount: number
      validationErrorCount: number
      skills: Array<{ id: string; name: string; scope: string; legacy: boolean }>
      roots?: unknown
      validationErrors?: unknown
    }
    expect(body).toMatchObject({
      schemaVersion: 2,
      skillCount: 1,
      validationErrorCount: 0
    })
    expect(body.skills[0]).toMatchObject({
      id: 'review',
      name: 'Review',
      scope: 'project',
      legacy: false
    })
    expect(body.roots).toBeUndefined()
    expect(body.validationErrors).toBeUndefined()
  })

  it('returns the real user message item id when starting a turn', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    }, { id: 'thr_1', title: 'demo' })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_1/turns', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ prompt: 'hello', workspaceCheckpointId: 'gcp_1' })
      })
    )

    expect(response.status).toBe(202)
    const body = await readJson(response) as { turnId: string; userMessageItemId: string }
    expect(body.turnId).toMatch(/^turn_/)
    expect(body.userMessageItemId).toBe(`item_${body.turnId}_user`)
    const detail = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_1', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const detailBody = await readJson(detail) as {
      turns: Array<{
        workspaceCheckpointId?: string
        items: Array<{ kind: string; workspaceCheckpointId?: string }>
      }>
    }
    expect(detailBody.turns[0]?.workspaceCheckpointId).toBe('gcp_1')
    expect(detailBody.turns[0]?.items.find((item) => item.kind === 'user_message')?.workspaceCheckpointId).toBe('gcp_1')
    const persistedUserItem = (await h.sessionStore.loadItems('thr_1'))[0]
    expect(persistedUserItem?.kind).toBe('user_message')
    if (persistedUserItem?.kind !== 'user_message') throw new Error('persisted turn item was not a user message')
    expect(persistedUserItem.workspaceCheckpointId).toBe('gcp_1')
    const eventStream = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_1/events?since_seq=0', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect((await readSseEvents(eventStream)).join('\n')).toContain('"workspaceCheckpointId":"gcp_1"')
  })

  it('validates steer admission against the running expected turn', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    }, { id: 'thr_steer', title: 'Steer route' })
    const { turnId } = await h.turnService.startTurn({
      threadId: 'thr_steer',
      request: { prompt: 'keep working' }
    })

    const accepted = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/thr_steer/turns/${turnId}/steer`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          text: 'add this constraint',
          expectedTurnId: turnId,
          clientUserMessageId: 'client_route_steer'
        })
      })
    )
    expect(accepted.status).toBe(200)
    expect(await readJson(accepted)).toMatchObject({
      ok: true,
      threadId: 'thr_steer',
      turnId,
      clientUserMessageId: 'client_route_steer',
      admittedSeq: expect.any(Number)
    })

    const mismatch = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/thr_steer/turns/${turnId}/steer`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ text: 'wrong turn', expectedTurnId: 'turn_elsewhere' })
      })
    )
    expect(mismatch.status).toBe(409)
    expect(await readJson(mismatch)).toMatchObject({ code: 'conflict' })

    await h.turnService.interruptTurn({ threadId: 'thr_steer', turnId, discard: true })
    const inactive = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/thr_steer/turns/${turnId}/steer`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ text: 'too late', expectedTurnId: turnId })
      })
    )
    expect(inactive.status).toBe(409)
    expect(await readJson(inactive)).toMatchObject({ code: 'conflict' })
  })

  it('applies per-turn execution policy to the active thread', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    }, { id: 'thr_policy', title: 'policy' })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_policy/turns', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          prompt: 'inspect only',
          approvalPolicy: 'on-request',
          sandboxMode: 'read-only'
        })
      })
    )

    expect(response.status).toBe(202)
    const thread = await h.threadService.get('thr_policy')
    expect(thread?.approvalPolicy).toBe('on-request')
    expect(thread?.sandboxMode).toBe('read-only')
  })

  it('rejects unadvertised Reasonix auto-plan payload fields on agent start-turn requests', async () => {
    const observedRequests: ModelRequest[] = []
    const model: ModelClient = {
      provider: 'capture',
      model: 'capture',
      async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
        observedRequests.push(request)
        yield { kind: 'completed', stopReason: 'stop' }
      }
    }
    const h = buildHarness({ model })
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_autoplan_payload', title: 'Agent turn' })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_autoplan_payload/turns', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          prompt: 'keep this as an agent turn',
          autoPlan: true,
          auto_plan: true
        })
      })
    )

    expect(response.status).toBe(400)
    expect(await readJson(response)).toEqual(expect.objectContaining({ code: 'validation_error' }))
    const thread = await h.threadService.get('thr_autoplan_payload')
    expect(thread?.turns).toHaveLength(0)
    expect(observedRequests).toHaveLength(0)
  })

  it('creates and lists threads through the HTTP layer', async () => {
    const h = buildHarness()
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp', model: 'deepseek-chat' })
      })
    )
    expect(create.status).toBe(201)
    const created = (await readJson(create)) as { id: string }
    const list = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const listed = (await readJson(list)) as { threads: { id: string }[] }
    expect(listed.threads.map((t) => t.id)).toContain(created.id)
  })

  it('sets, reads, and clears thread goals through the HTTP layer', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_goal', title: 'Goal' })

    const setGoal = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_goal/goal', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ objective: 'ship goal mode', status: 'active' })
      })
    )
    expect(setGoal.status).toBe(200)
    const setBody = await readJson(setGoal) as { goal?: { objective?: string; status?: string } }
    expect(setBody.goal).toMatchObject({ objective: 'ship goal mode', status: 'active' })

    const readGoal = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_goal/goal', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(readGoal.status).toBe(200)
    const readBody = await readJson(readGoal) as { goal?: { objective?: string } | null }
    expect(readBody.goal?.objective).toBe('ship goal mode')

    const clearGoal = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_goal/goal', {
        method: 'DELETE',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(clearGoal.status).toBe(200)
    expect(await readJson(clearGoal)).toEqual({ cleared: true })
  })

  it('returns validation errors when HTTP goal completion has incomplete todos', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_goal_todos', title: 'Goal Todos' })
    await h.threadService.setGoal('thr_goal_todos', {
      objective: 'ship goal mode',
      status: 'active'
    })
    await h.threadService.appendGoalEvidence('thr_goal_todos', {
      step: 'Recorded evidence',
      evidence: ['service-level evidence exists']
    })
    await h.threadService.setTodos('thr_goal_todos', {
      todos: [
        { content: 'Run endpoint regression', status: 'pending' }
      ]
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_goal_todos/goal', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ status: 'complete' })
      })
    )

    expect(response.status).toBe(400)
    expect(await readJson(response)).toMatchObject({
      code: 'validation_error',
      message: expect.stringContaining('incomplete todos')
    })
    expect((await h.threadService.getGoal('thr_goal_todos'))?.status).toBe('active')
  })

  it('returns validation errors when HTTP strict goal completion lacks self-check', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_goal_strict', title: 'Strict Goal' })
    await h.threadService.setGoal('thr_goal_strict', {
      objective: 'ship strict goal mode',
      status: 'active',
      strictCompletion: true
    })
    await h.threadService.appendGoalEvidence('thr_goal_strict', {
      step: 'Recorded strict evidence',
      evidence: ['service-level evidence exists']
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_goal_strict/goal', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ status: 'complete' })
      })
    )

    expect(response.status).toBe(400)
    expect(await readJson(response)).toMatchObject({
      code: 'validation_error',
      message: expect.stringContaining('self-check')
    })
    expect((await h.threadService.getGoal('thr_goal_strict'))?.status).toBe('active')
  })

  it('creates AutoResearch project-local state through the existing goal endpoint', async () => {
    const h = buildHarness()
    const workspace = await mkdtemp(join(tmpdir(), 'analytix-http-research-'))
    try {
      await h.threadService.create({
        workspace,
        model: 'deepseek-chat',
        mode: 'agent'
      }, { id: 'thr_research', title: 'Research Goal' })

      const response = await dispatchRequest(
        h.router,
        new Request('http://localhost/v1/threads/thr_research/goal', {
          method: 'POST',
          headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
          body: JSON.stringify({
            objective: 'Research cache behavior',
            status: 'active',
            research: {
              enabled: true,
              requirements: ['Compare providers', 'Audit unsupported fallback']
            }
          })
        })
      )

      expect(response.status).toBe(200)
      const body = await readJson(response) as { goal?: { research?: { taskSpecPath?: string; requirementCount?: number } } }
      expect(body.goal?.research).toMatchObject({
        taskSpecPath: '.analytix/autoresearch/thr_research/task_spec.md',
        requirementCount: 2
      })
      await expect(readFile(join(workspace, '.analytix/autoresearch/thr_research/task_spec.md'), 'utf8'))
        .resolves.toContain('Research cache behavior')
      await expect(readFile(join(workspace, 'REASONIX.md'), 'utf8')).rejects.toThrow()
      await expect(readFile(join(workspace, 'AGENTS.md'), 'utf8')).rejects.toThrow()
    } finally {
      await rm(workspace, { recursive: true, force: true })
    }
  })

  it('sets, reads, and clears thread todos through the HTTP layer', async () => {
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp',
      model: 'deepseek-chat',
      mode: 'agent'
    }, { id: 'thr_todos', title: 'Todos' })

    const setTodos = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_todos/todos', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          todos: [
            { content: 'Wire API', status: 'completed' },
            { content: 'Render panel', status: 'pending' }
          ]
        })
      })
    )
    expect(setTodos.status).toBe(200)
    const setBody = await readJson(setTodos) as { todos?: { items?: Array<{ content?: string; status?: string }> } }
    expect(setBody.todos?.items).toEqual(expect.arrayContaining([
      expect.objectContaining({ content: 'Wire API', status: 'completed' })
    ]))

    const readTodos = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_todos/todos', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(readTodos.status).toBe(200)
    const readBody = await readJson(readTodos) as { todos?: { items?: Array<{ content?: string }> } | null }
    expect(readBody.todos?.items?.[0]?.content).toBe('Wire API')

    const clearTodos = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_todos/todos', {
        method: 'DELETE',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(clearTodos.status).toBe(200)
    expect(await readJson(clearTodos)).toEqual({ cleared: true })
  })

  it('filters thread lists for search, archives, and limits', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/alpha', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_alpha', title: 'Alpha Project' }
    )
    await h.threadService.create(
      { workspace: '/tmp/beta', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_beta', title: 'Beta Archive' }
    )
    await h.threadService.update('thr_beta', { status: 'archived' })

    const active = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(active.status).toBe(200)
    const activeBody = (await readJson(active)) as { threads: Array<{ id: string }> }
    expect(activeBody.threads.map((thread) => thread.id)).toEqual(['thr_alpha'])

    const archived = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads?archived_only=true', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const archivedBody = (await readJson(archived)) as { threads: Array<{ id: string }> }
    expect(archivedBody.threads.map((thread) => thread.id)).toEqual(['thr_beta'])

    const search = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads?include_archived=true&search=archive', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const searchBody = (await readJson(search)) as { threads: Array<{ id: string }> }
    expect(searchBody.threads.map((thread) => thread.id)).toEqual(['thr_beta'])

    const limited = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads?include_archived=true&limit=1', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const limitedBody = (await readJson(limited)) as { threads: Array<{ id: string }> }
    expect(limitedBody.threads).toHaveLength(1)
  })

  it('deletes threads through the HTTP layer', async () => {
    const h = buildHarness()
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp/delete-me', model: 'deepseek-chat' })
      })
    )
    expect(create.status).toBe(201)
    const created = (await readJson(create)) as { id: string }

    const deleted = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${created.id}`, {
        method: 'DELETE',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(deleted.status).toBe(200)
    expect(await readJson(deleted)).toEqual({ id: created.id, deleted: true })

    const list = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads?include_archived=true', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const listed = (await readJson(list)) as { threads: Array<{ id: string }> }
    expect(listed.threads.map((thread) => thread.id)).not.toContain(created.id)

    const detail = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${created.id}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(detail.status).toBe(404)
  })

  it('returns 404 when deleting a missing thread', async () => {
    const h = buildHarness()
    const deleted = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/missing-thread', {
        method: 'DELETE',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(deleted.status).toBe(404)
    expect(await readJson(deleted)).toMatchObject({
      code: 'not_found',
      message: 'thread not found: missing-thread'
    })
  })

  it('rejects invalid thread creation bodies with 400', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '', model: '' })
      })
    )
    expect(response.status).toBe(400)
  })

  it('starts a turn and serves the SSE backlog', async () => {
    const h = buildHarness()
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp', model: 'deepseek-chat' })
      })
    )
    const thread = (await readJson(create)) as { id: string }
    const turn = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/turns`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ prompt: 'hi' })
      })
    )
    expect(turn.status).toBe(202)
    const turnBody = (await readJson(turn)) as { threadId: string; turnId: string }
    expect(turnBody.threadId).toBe(thread.id)
    const detail = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const detailBody = (await readJson(detail)) as {
      latestSeq: number
      turns: Array<{ items: Array<{ kind: string }> }>
    }
    expect(detailBody.latestSeq).toBeGreaterThan(0)
    expect(detailBody.turns.at(-1)?.items.some((item) => item.kind === 'user_message')).toBe(true)
    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events?since_seq=0`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const events = await readSseEvents(eventStream)
    const kinds = events.flatMap((frame) =>
      frame
        .split('\n')
        .filter((line) => line.startsWith('event:'))
        .map((line) => line.slice(7))
    )
    expect(kinds).toContain('turn_started')
  })

  it('rewinds a thread persistently and records a replay barrier event', async () => {
    const h = buildHarness()
    const thread = await h.threadService.create({ workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' })
    const first = await h.turnService.startTurn({ threadId: thread.id, request: { prompt: 'keep this turn' } })
    await h.turnService.interruptTurn({ threadId: thread.id, turnId: first.turnId })
    const second = await h.turnService.startTurn({ threadId: thread.id, request: { prompt: 'remove this turn' } })
    await h.turnService.interruptTurn({ threadId: thread.id, turnId: second.turnId })

    const response = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/rewind`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ turnId: second.turnId })
      })
    )

    expect(response.status).toBe(200)
    expect(await readJson(response)).toMatchObject({
      threadId: thread.id,
      turnId: second.turnId,
      removedTurns: 1,
      remainingTurns: 1,
      removedTurnIds: [second.turnId]
    })
    const detail = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const detailBody = await readJson(detail) as { turns: Array<{ id: string }> }
    expect(detailBody.turns.map((turn) => turn.id)).toEqual([first.turnId])
    expect((await h.sessionStore.loadItems(thread.id)).map((item) => item.turnId)).toEqual([first.turnId])

    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events?since_seq=0`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const events = await readSseEvents(eventStream)
    expect(events.join('\n')).toContain('event: thread_rewound')
    expect(events.join('\n')).toContain(`"removedTurnIds":["${second.turnId}"]`)
  })

  it('keeps the legacy TypeScript HTTP harness usable for chat, SSE replay, and usage', async () => {
    let requestCount = 0
    const model: ModelClient = {
      provider: 'typescript-legacy-smoke',
      model: 'typescript-legacy-smoke',
      async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
        requestCount += 1
        expect(request.threadId).toBe('thr_ts_legacy')
        expect(request.history.some((item) => item.kind === 'user_message' && item.text === 'hello legacy')).toBe(true)
        yield { kind: 'assistant_text_delta', text: 'hello' }
        yield { kind: 'assistant_text_delta', text: ' legacy' }
        yield {
          kind: 'usage',
          usage: usageSnapshot({
            promptTokens: 14,
            completionTokens: 6,
            totalTokens: 20,
            cachedTokens: 4,
            cacheHitTokens: 4,
            cacheMissTokens: 10,
            cacheHitRate: 4 / 14
          })
        }
        yield { kind: 'completed', stopReason: 'stop' }
      }
    }
    const h = buildHarness({ model })
    await h.threadService.create(
      { workspace: '/tmp/legacy-ts', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_ts_legacy', title: 'TS legacy smoke' }
    )

    const turn = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_ts_legacy/turns', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ prompt: 'hello legacy' })
      })
    )
    expect(turn.status).toBe(202)

    for (let attempt = 0; attempt < 50; attempt += 1) {
      const events = await h.sessionStore.loadEventsSince('thr_ts_legacy', 0)
      if (events.some((event) => event.kind === 'usage') && events.some((event) => event.kind === 'turn_completed')) {
        break
      }
      await new Promise((resolve) => setTimeout(resolve, 5))
    }

    const replay = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_ts_legacy/events?since_seq=0', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(replay.status).toBe(200)
    const frames = await readSseEvents(replay)
    const replayKinds = frames.flatMap((frame) =>
      frame
        .split('\n')
        .filter((line) => line.startsWith('event:'))
        .map((line) => line.slice(7))
    )
    const firstText = replayKinds.indexOf('assistant_text_delta')
    const firstUsage = replayKinds.indexOf('usage')
    const completed = replayKinds.indexOf('turn_completed')
    expect(firstText).toBeGreaterThanOrEqual(0)
    expect(firstUsage).toBeGreaterThan(firstText)
    expect(completed).toBeGreaterThan(firstUsage)

    const detail = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_ts_legacy', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(detail.status).toBe(200)
    const detailBody = (await readJson(detail)) as {
      turns: Array<{ items: Array<{ kind: string; text?: string; status?: string }> }>
    }
    expect(detailBody.turns.at(-1)?.items).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'assistant_text', text: 'hello legacy', status: 'completed' })
    ]))

    const usage = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(usage.status).toBe(200)
    const usageBody = (await readJson(usage)) as {
      total: { promptTokens: number; completionTokens: number; totalTokens: number; cacheHitTokens?: number }
      perThread: Array<{ threadId: string; usage: { totalTokens: number } }>
    }
    expect(usageBody.total).toMatchObject({
      promptTokens: 14,
      completionTokens: 6,
      totalTokens: 20,
      cacheHitTokens: 4
    })
    expect(usageBody.perThread).toEqual([
      expect.objectContaining({
        threadId: 'thr_ts_legacy',
        usage: expect.objectContaining({ totalTokens: 20 })
      })
    ])
    expect(requestCount).toBe(1)
  })

  it('hydrates thread detail items from the session log when the thread snapshot lags', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_lag', title: 'Lagging snapshot' }
    )
    const { turnId } = await h.turnService.startTurn({
      threadId: 'thr_lag',
      request: { prompt: 'hi' }
    })
    await h.sessionStore.appendItem('thr_lag', makeAssistantTextItem({
      id: 'item_answer',
      turnId,
      threadId: 'thr_lag',
      text: 'hello after reload',
      status: 'completed'
    }))
    const snapshot = await h.threadService.get('thr_lag')
    expect(snapshot?.turns.at(-1)?.items.map((item) => item.kind)).toEqual(['user_message'])

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_lag', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      turns: Array<{ items: Array<{ kind: string; text?: string }> }>
    }
    expect(body.turns.at(-1)?.items.map((item) => item.kind)).toEqual(['user_message', 'assistant_text'])
    expect(body.turns.at(-1)?.items.at(-1)).toMatchObject({
      kind: 'assistant_text',
      text: 'hello after reload'
    })
  })

  it('heals stale open session items for finished turns when loading thread detail', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_heal', title: 'Stale session' }
    )
    const { turnId } = await h.turnService.startTurn({
      threadId: 'thr_heal',
      request: { prompt: 'run a tool' }
    })
    await h.turnService.applyItem(
      'thr_heal',
      makeToolCallItem({
        id: 'item_tool_stale',
        turnId,
        threadId: 'thr_heal',
        callId: 'call_stale',
        toolName: 'echo',
        arguments: { text: 'hi' }
      })
    )
    await h.turnService.applyItem(
      'thr_heal',
      makeToolResultItem({
        id: 'item_result_stale',
        turnId,
        threadId: 'thr_heal',
        callId: 'call_stale',
        toolName: 'echo',
        output: { partial: true },
        status: 'running'
      })
    )
    const staleThread = await h.threadStore.get('thr_heal')
    if (!staleThread) throw new Error('expected thread')
    const finishedAt = '2026-06-05T00:00:00.000Z'
    await h.threadStore.upsert({
      ...staleThread,
      status: 'idle',
      turns: staleThread.turns.map((turn) =>
        turn.id === turnId
          ? {
              ...turn,
              status: 'aborted',
              finishedAt,
              items: turn.items.map((item): TurnItem =>
                item.id === 'item_tool_stale' || item.id === 'item_result_stale'
                  ? ({ ...item, status: 'aborted', finishedAt } as TurnItem)
                  : item
              )
            }
          : turn
      )
    })
    const staleById = new Map((await h.sessionStore.loadItems('thr_heal')).map((item) => [item.id, item.status]))
    expect(staleById.get('item_tool_stale')).toBe('pending')
    expect(staleById.get('item_result_stale')).toBe('running')

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_heal', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      turns: Array<{ id: string; items: Array<{ id: string; status: string }> }>
    }
    const responseItems = new Map(
      (body.turns.find((turn) => turn.id === turnId)?.items ?? []).map((item) => [item.id, item.status])
    )
    expect(responseItems.get('item_tool_stale')).toBe('aborted')
    expect(responseItems.get('item_result_stale')).toBe('aborted')
    const healedById = new Map((await h.sessionStore.loadItems('thr_heal')).map((item) => [item.id, item.status]))
    expect(healedById.get('item_tool_stale')).toBe('aborted')
    expect(healedById.get('item_result_stale')).toBe('aborted')
  })

  it('persists GUI plan context from start-turn requests', async () => {
    const h = buildHarness()
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp', model: 'deepseek-chat' })
      })
    )
    const thread = (await readJson(create)) as { id: string }
    const turn = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/turns`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          prompt: 'Plan auth',
          guiPlan: {
            operation: 'draft',
            workspaceRoot: '/tmp',
            relativePath: '.analytix/plan/auth.md',
            planId: '/tmp:.analytix/plan/auth.md',
            sourceRequest: 'Add auth',
            title: 'Auth'
          }
        })
      })
    )
    expect(turn.status).toBe(202)
    const turnBody = (await readJson(turn)) as { turnId: string }
    const detail = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/turns/${turnBody.turnId}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(detail.status).toBe(200)
    const detailBody = (await readJson(detail)) as { guiPlan?: { relativePath?: string; operation?: string } }
    expect(detailBody.guiPlan).toMatchObject({
      operation: 'draft',
      relativePath: '.analytix/plan/auth.md'
    })
  })

  it('groups usage by the usage event model instead of the thread default model', async () => {
    const h = buildHarness()
    const today = new Date().toISOString().slice(0, 10)
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp', model: 'deepseek-chat' })
      })
    )
    expect(create.status).toBe(201)
    const thread = (await readJson(create)) as { id: string }
    const turn = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/turns`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ prompt: 'hi' })
      })
    )
    expect(turn.status).toBe(202)
    const turnBody = (await readJson(turn)) as { turnId: string }
    await h.runtime.events.record({
      kind: 'usage',
      threadId: thread.id,
      turnId: turnBody.turnId,
      model: 'deepseek-v4-pro',
      usage: usageSnapshot({ promptTokens: 30, completionTokens: 10, totalTokens: 40 })
    })

    const usage = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/usage?group_by=model&from=${today}&to=${today}&timezone=UTC`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(usage.status).toBe(200)
    const body = (await readJson(usage)) as {
      buckets: Array<{ model: string; total_tokens: number }>
    }
    expect(body.buckets).toEqual([
      expect.objectContaining({
        model: 'deepseek-v4-pro',
        total_tokens: 40
      })
    ])
  })

  it('replays SSE backlog from Last-Event-ID when since_seq is omitted', async () => {
    const h = buildHarness()
    const create = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp', model: 'deepseek-chat' })
      })
    )
    const thread = (await readJson(create)) as { id: string }
    const turn = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/turns`, {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ prompt: 'hi' })
      })
    )
    expect(turn.status).toBe(202)

    const allEvents = await h.sessionStore.loadEventsSince(thread.id, 0)
    const secondSeq = allEvents[1]?.seq ?? 0
    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events`, {
        headers: { authorization: 'Bearer tok-1', 'Last-Event-ID': String(secondSeq) }
      })
    )
    const events = await readSseEvents(eventStream)
    const ids = events.flatMap((frame) =>
      frame
        .split('\n')
        .filter((line) => line.startsWith('id:'))
        .map((line) => Number(line.slice(3).trim()))
    )
    expect(ids.every((id) => id > secondSeq)).toBe(true)
  })

  it('delivers an event exactly once when it lands in both backlog and live bus', async () => {
    const h = buildHarness()
    const thread = await h.threadService.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_dedup', title: 'Dedup' }
    )
    const recorded = await h.runtime.events.record({ kind: 'heartbeat', threadId: thread.id })

    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events?since_seq=0`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    // Simulate the persist/publish race: the event is already in the replayed
    // backlog when the live bus re-delivers it after the subscription starts.
    await new Promise((resolve) => setTimeout(resolve, 20))
    h.bus.publish(recorded)

    const frames = await readSseEvents(eventStream)
    const occurrences = frames.filter((frame) => frame.includes(`id: ${recorded.seq}\n`))
    expect(occurrences).toHaveLength(1)
  })

  it('skips SSE backlog replay when the client is already caught up', async () => {
    const h = buildHarness()
    const thread = await h.threadService.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_caught_up', title: 'Caught up' }
    )
    const latestSeq = await h.sessionStore.highestSeq(thread.id)
    const loadEventsSince = vi.spyOn(h.sessionStore, 'loadEventsSince')

    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events?since_seq=${latestSeq}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const events = await readSseEvents(eventStream)

    expect(events).toEqual([])
    expect(loadEventsSince).not.toHaveBeenCalled()
  })

  it('resolves an approval through the HTTP endpoint', async () => {
    const h = buildHarness()
    const approval = createApprovalRequest({
      id: 'appr_1',
      threadId: 'thr_1',
      turnId: 'turn_1',
      toolName: 'echo',
      summary: 'run echo'
    })
    const pending = h.approvalGate.request(approval)
    const decide = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/approvals/appr_1', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ decision: 'allow' })
      })
    )
    expect(decide.status).toBe(200)
    const body = (await readJson(decide)) as { decision: string }
    expect(body.decision).toBe('allow')
    await expect(pending).resolves.toBe('allow')
  })

  it('resolves GUI user input through both HTTP compatibility endpoints', async () => {
    const h = buildHarness()
    const pending = h.userInputGate.request({
      id: 'in_1',
      threadId: 'thr_1',
      turnId: 'turn_1',
      itemId: 'item_in_1',
      prompt: 'Pick one',
      questions: []
    })
    const submit = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/user-inputs/in_1', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          answers: [{ id: 'choice', label: 'Yes', value: 'yes' }]
        })
      })
    )
    expect(submit.status).toBe(200)
    await expect(pending).resolves.toEqual({
      status: 'submitted',
      answers: [{ id: 'choice', label: 'Yes', value: 'yes' }]
    })

    const cancelPending = h.userInputGate.request({
      id: 'in_2',
      threadId: 'thr_1',
      turnId: 'turn_1',
      itemId: 'item_in_2',
      prompt: 'Cancel?',
      questions: []
    })
    const cancel = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/user-input/in_2', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ cancelled: true })
      })
    )
    expect(cancel.status).toBe(200)
    await expect(cancelPending).resolves.toEqual({ status: 'cancelled' })
    const events = await h.sessionStore.loadEventsSince('thr_1', 0)
    expect(events.filter((event) => event.kind === 'user_input_resolved')).toHaveLength(2)
  })

  it('forks a thread with copied history and lineage metadata', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_parent', title: 'Parent' }
    )
    await h.turnService.startTurn({
      threadId: 'thr_parent',
      request: { prompt: 'hello' }
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_parent/fork', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(201)
    const fork = (await readJson(response)) as {
      id: string
      forkedFromThreadId?: string
      forkedFromTitle?: string
      forkedFromMessageCount?: number
      forkedFromTurnCount?: number
      turns: Array<{ threadId: string; items: Array<{ threadId: string; kind: string }> }>
    }
    expect(fork.forkedFromThreadId).toBe('thr_parent')
    expect(fork.forkedFromTitle).toBe('Parent')
    expect(fork.forkedFromMessageCount).toBe(1)
    expect(fork.forkedFromTurnCount).toBe(1)
    expect(fork.turns[0]?.threadId).toBe(fork.id)
    expect(fork.turns[0]?.items[0]).toMatchObject({ threadId: fork.id, kind: 'user_message' })
    const copiedItems = await h.sessionStore.loadItems(fork.id)
    expect(copiedItems).toHaveLength(1)
    expect(copiedItems[0]).toMatchObject({ threadId: fork.id, kind: 'user_message' })
  })

  it('forks with relation: side, attaches parentThreadId, and is excluded from the default list', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_parent', title: 'Parent' }
    )
    await h.turnService.startTurn({
      threadId: 'thr_parent',
      request: { prompt: 'seed turn' }
    })

    const forkResponse = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_parent/fork', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ relation: 'side' })
      })
    )
    expect(forkResponse.status).toBe(201)
    const fork = (await readJson(forkResponse)) as {
      id: string
      relation?: string
      parentThreadId?: string
      title: string
    }
    expect(fork.relation).toBe('side')
    expect(fork.parentThreadId).toBe('thr_parent')
    expect(fork.title).toBe('Parent · side')

    // Default list hides side threads.
    const listResponse = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const listBody = (await readJson(listResponse)) as {
      threads: Array<{ id: string; relation?: string }>
    }
    expect(listBody.threads.find((t) => t.id === fork.id)).toBeUndefined()
    expect(listBody.threads.find((t) => t.id === 'thr_parent')).toBeDefined()

    // Opt-in include=side surfaces them.
    const includeResponse = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads?include=side', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const includeBody = (await readJson(includeResponse)) as {
      threads: Array<{ id: string; relation?: string }>
    }
    expect(includeBody.threads.find((t) => t.id === fork.id)).toMatchObject({ relation: 'side' })
  })

  it('bodyless fork still defaults to relation: fork', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_default_fork', title: 'Forker' }
    )
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/threads/thr_default_fork/fork', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(response.status).toBe(201)
    const body = (await readJson(response)) as { relation?: string; parentThreadId?: string }
    expect(body.relation).toBe('fork')
    expect(body.parentThreadId).toBe('thr_default_fork')
  })

  it('resumes a persisted session into a new Analytix thread', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_source', title: 'Source Thread' }
    )
    await h.turnService.startTurn({
      threadId: 'thr_source',
      request: { prompt: 'restore this' }
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/sessions/thr_source/resume-thread', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ workspace: '/tmp/override', model: 'deepseek-coder', mode: 'plan' })
      })
    )

    expect(response.status).toBe(201)
    const body = (await readJson(response)) as {
      thread_id: string
      session_id: string
      message_count: number
      summary: string
    }
    expect(body.session_id).toBe('thr_source')
    expect(body.message_count).toBe(1)
    expect(body.summary).toBe('Source Thread resumed')
    const resumed = await h.threadService.get(body.thread_id)
    expect(resumed).toMatchObject({
      workspace: '/tmp/override',
      model: 'deepseek-coder',
      mode: 'plan',
      status: 'idle',
      forkedFromThreadId: 'thr_source'
    })
    expect(resumed?.turns[0]?.status).toBe('completed')
    expect(resumed?.turns[0]?.items[0]).toMatchObject({
      threadId: body.thread_id,
      kind: 'user_message',
      text: 'restore this'
    })
    const copiedItems = await h.sessionStore.loadItems(body.thread_id)
    expect(copiedItems).toHaveLength(1)
  })

  it('returns 404 when resuming an unknown session', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/sessions/missing/resume-thread', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({})
      })
    )

    expect(response.status).toBe(404)
  })

  it('returns cumulative usage from /v1/usage', async () => {
    const h = buildHarness()
    h.runtime.usageService.record('thr_1', {
      promptTokens: 5,
      completionTokens: 3,
      totalTokens: 8,
      cachedTokens: 2,
      cacheHitTokens: 2,
      cacheMissTokens: 3,
      cacheHitRate: 0.4,
      turns: 1
    })
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(response.status).toBe(200)
    const body = (await readJson(response)) as { total: { promptTokens: number } }
    expect(body.total.promptTokens).toBe(5)
  })

  it('returns live thread-grouped usage buckets from /v1/usage?group_by=thread', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_live', title: 'Live usage' }
    )
    h.runtime.usageService.record('thr_live', usageSnapshot({
      promptTokens: 12,
      completionTokens: 8,
      cacheHitTokens: 9,
      cacheMissTokens: 3,
      cacheHitRate: 9 / 12,
      cacheableTokenHitRate: 9 / 12,
      totalInputTokenHitRate: 9 / 12,
      cacheMissReasons: ['tool_catalog_changed'],
      cacheSuggestions: ['Keep MCP and Skill tools stable within a thread.']
    }))

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage?group_by=thread', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      group_by: string
      buckets: Array<{
        thread_id: string
        total_tokens: number
        turns: number
        last_turn_cacheable_hit_rate?: number
        last_turn_total_input_hit_rate?: number
        last_cache_miss_reasons?: string[]
        last_cache_suggestions?: string[]
      }>
    }
    expect(body.group_by).toBe('thread')
    expect(body.buckets).toEqual([
      expect.objectContaining({ thread_id: 'thr_live', total_tokens: 20, turns: 1 })
    ])
    expect(body.buckets[0]?.last_turn_cacheable_hit_rate).toBeCloseTo(9 / 12)
    expect(body.buckets[0]?.last_turn_total_input_hit_rate).toBeCloseTo(9 / 12)
    expect(body.buckets[0]?.last_cache_miss_reasons).toEqual(['tool_catalog_changed'])
    expect(body.buckets[0]?.last_cache_suggestions).toEqual(['Keep MCP and Skill tools stable within a thread.'])
  })

  it('filters thread-grouped usage buckets by thread_id', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_live', title: 'Live usage' }
    )
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_other', title: 'Other usage' }
    )
    h.runtime.usageService.record('thr_live', usageSnapshot({ promptTokens: 12, completionTokens: 8 }))
    h.runtime.usageService.record('thr_other', usageSnapshot({ promptTokens: 90, completionTokens: 10 }))

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage?group_by=thread&thread_id=thr_live', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      group_by: string
      buckets: Array<{ thread_id: string; total_tokens: number; turns: number }>
    }
    expect(body.group_by).toBe('thread')
    expect(body.buckets).toEqual([
      expect.objectContaining({ thread_id: 'thr_live', total_tokens: 20, turns: 1 })
    ])
  })

  it('derives daily usage from persisted cumulative usage events', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_usage', title: 'Persisted usage' }
    )
    await h.sessionStore.appendEvent('thr_usage', {
      kind: 'usage',
      seq: 2,
      timestamp: '2026-06-02T09:00:00.000Z',
      threadId: 'thr_usage',
      usage: usageSnapshot({
        promptTokens: 10,
        completionTokens: 5,
        totalTokens: 15,
        turns: 1,
        tokenEconomySavingsTokens: 100,
        tokenEconomySavingsUsd: 0.001
      })
    })
    await h.sessionStore.appendEvent('thr_usage', {
      kind: 'usage',
      seq: 3,
      timestamp: '2026-06-02T09:05:00.000Z',
      threadId: 'thr_usage',
      usage: usageSnapshot({
        promptTokens: 30,
        completionTokens: 10,
        totalTokens: 40,
        turns: 2,
        tokenEconomySavingsTokens: 250,
        tokenEconomySavingsUsd: 0.0025
      })
    })

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage?group_by=day&from=2026-06-02&to=2026-06-02&timezone=UTC', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      group_by: string
      buckets: Array<{ date: string; total_tokens: number; turns: number; thread_count: number }>
      totals: {
        total_tokens: number
        turns: number
        active_days: number
        token_economy_savings_tokens: number
        token_economy_savings_usd: number
      }
    }
    expect(body.group_by).toBe('day')
    expect(body.buckets[0]).toMatchObject({
      date: '2026-06-02',
      total_tokens: 40,
      turns: 2,
      thread_count: 1
    })
    expect(body.totals).toMatchObject({
      total_tokens: 40,
      turns: 2,
      active_days: 1,
      token_economy_savings_tokens: 250,
      token_economy_savings_usd: 0.0025
    })
  })

  it('falls back to thread model and provider when indexed usage rows are legacy-shaped', async () => {
    const h = buildHarness()
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'shared-model', providerId: 'deepseek', mode: 'agent' },
      { id: 'thr_indexed_deepseek', title: 'DeepSeek indexed usage' }
    )
    await h.threadService.create(
      { workspace: '/tmp/project', model: 'shared-model', providerId: 'openai-compatible', mode: 'agent' },
      { id: 'thr_indexed_openai', title: 'OpenAI indexed usage' }
    )
    const indexedStore = h.sessionStore as unknown as {
      loadUsageRecords: (options?: { threadId?: string }) => Promise<Array<{
        threadId: string
        completedAt: string
        usage: ReturnType<typeof usageSnapshot>
      }>>
      loadLatestUsageSnapshots: (options?: { threadIds?: string[] }) => Promise<SessionLatestUsageSnapshot[]>
    }
    indexedStore.loadUsageRecords = vi.fn(async () => [
      {
        threadId: 'thr_indexed_deepseek',
        completedAt: '2026-06-02T09:00:00.000Z',
        usage: usageSnapshot({ promptTokens: 20, completionTokens: 5, totalTokens: 25 })
      },
      {
        threadId: 'thr_indexed_openai',
        completedAt: '2026-06-02T09:05:00.000Z',
        usage: usageSnapshot({ promptTokens: 30, completionTokens: 10, totalTokens: 40 })
      }
    ])
    indexedStore.loadLatestUsageSnapshots = vi.fn(async () => [])

    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/usage?group_by=model&from=2026-06-02&to=2026-06-02&timezone=UTC', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(response.status).toBe(200)
    const body = (await readJson(response)) as {
      buckets: Array<{ model: string; provider: string; total_tokens: number; thread_count: number }>
    }
    expect(body.buckets).toEqual([
      expect.objectContaining({
        model: 'shared-model',
        provider: 'mixed',
        total_tokens: 65,
        thread_count: 2
      })
    ])
  })

  it('encodes SSE events with sequence numbers and event names', () => {
    const frame = encodeSseEvent({
      kind: 'heartbeat',
      seq: 7,
      timestamp: 't',
      threadId: 'th'
    })
    expect(frame).toContain('id: 7')
    expect(frame).toContain('event: heartbeat')
    expect(frame.endsWith('\n\n')).toBe(true)
  })

  it('opens thread event streams with a proxy-friendly SSE comment', async () => {
    const h = buildHarness()
    const thread = await h.threadService.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_sse_comment', title: 'SSE comment' }
    )
    const eventStream = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/threads/${thread.id}/events?since_seq=0`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    const reader = eventStream.body?.getReader()
    if (!reader) throw new Error('expected SSE response body')
    const first = await reader.read()
    await reader.cancel()
    expect(first.done).toBe(false)
    expect(new TextDecoder().decode(first.value)).toBe(': connected\n\n')
  })

  it('returns a 404 for unknown routes', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/unknown')
    )
    expect(response.status).toBe(404)
    expect(response.headers.get('content-type')).toContain('application/json')
    expect(await readJson(response)).toEqual({
      code: 'not_found',
      message: 'route not found'
    })
  })

  it.each([
    '/v1/reasonix/sessions',
    '/v1/reasonix/threads/thr_1',
    '/v1/runtime/go',
    '/v1/runtime/go/health',
    '/v1/workflow',
    '/v1/workflows',
    '/v1/create-loop',
    '/v1/subagents',
    '/v1/autoresearch',
    '/v1/mcp-indexer'
  ])('keeps forbidden upstream public route absent: %s', async (path) => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request(`http://localhost${path}`, {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(response.status).toBe(404)
    expect(await readJson(response)).toEqual({
      code: 'not_found',
      message: 'route not found'
    })
  })

  it('streams a workspace status response', async () => {
    const h = buildHarness()
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/workspace/status?path=/tmp', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(response.status).toBe(200)
    const body = (await readJson(response)) as { path: string }
    expect(body.path).toBe('/tmp')
  })
})
