import { describe, expect, it, vi } from 'vitest'
import type { SetThreadTodosRequest, ThreadTodoList } from '../../contracts/threads.js'
import type { ThreadService } from '../../services-test-support/thread-service.js'
import type { ToolHostContext } from '../../ports/tool-host.js'
import { buildTodoLocalTools, TODO_WRITE_TOOL_NAME } from '../../tool-test-support/tool/todo-tools.js'

describe('todo tools', () => {
  it('binds todo_write updates to the executing turn id', async () => {
    const setTodos = vi.fn(async (
      threadId: string,
      request: SetThreadTodosRequest
    ): Promise<ThreadTodoList> => ({
      threadId,
      ...(request.turnId ? { turnId: request.turnId } : {}),
      items: [{
        id: 'todo_1',
        content: request.todos[0]?.content ?? 'missing',
        status: request.todos[0]?.status ?? 'pending',
        createdAt: '2026-06-29T00:00:00.000Z',
        updatedAt: '2026-06-29T00:00:00.000Z'
      }],
      updatedAt: '2026-06-29T00:00:00.000Z'
    }))
    const threadService = { setTodos } as unknown as ThreadService
    const tool = buildTodoLocalTools(threadService).find((candidate) => candidate.name === TODO_WRITE_TOOL_NAME)
    const context: ToolHostContext = {
      threadId: 'thr_1',
      turnId: 'turn_1',
      workspace: '/workspace',
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access',
      abortSignal: new AbortController().signal,
      awaitApproval: async () => 'allow'
    }

    const result = await tool?.execute({
      todos: [{ content: 'Implement progress pill', status: 'in_progress' }]
    }, context)

    expect(setTodos).toHaveBeenCalledWith('thr_1', {
      turnId: 'turn_1',
      todos: [{ content: 'Implement progress pill', status: 'in_progress' }]
    })
    expect(result?.output).toEqual({
      todos: expect.objectContaining({
        threadId: 'thr_1',
        turnId: 'turn_1'
      })
    })
  })
})
