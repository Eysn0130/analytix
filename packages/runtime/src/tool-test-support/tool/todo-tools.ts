import type { SetThreadTodosRequest, ThreadTodoList } from '../../contracts/threads.js'
import type { ThreadService } from '../../services-test-support/thread-service.js'
import { LocalToolHost, type LocalTool } from './local-tool-host.js'

export const TODO_LIST_TOOL_NAME = 'todo_list'
export const TODO_WRITE_TOOL_NAME = 'todo_write'
export const TODO_TOOL_NAMES = [TODO_LIST_TOOL_NAME, TODO_WRITE_TOOL_NAME] as const

export function buildTodoLocalTools(threadService: ThreadService): LocalTool[] {
  return [
    createTodoListTool(threadService),
    createTodoWriteTool(threadService)
  ]
}

function createTodoListTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: TODO_LIST_TOOL_NAME,
    description: 'Return the current thread todo list. Use this to inspect structured progress state.',
    inputSchema: {
      type: 'object',
      properties: {},
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (_args, context) => {
      const todos = await threadService.getTodos(context.threadId)
      return { output: todoResponse(todos) }
    }
  })
}

function createTodoWriteTool(threadService: ThreadService): LocalTool {
  return LocalToolHost.defineTool({
    name: TODO_WRITE_TOOL_NAME,
    description: [
      'Replace the current thread todo list with the supplied full list.',
      'Use it for visible task tracking in both Agent and Plan modes.',
      'At most one item may be in_progress; invalid replacement tables are rejected without mutation.',
      'Failed and canceled items require a host-defined statusReasonCode and remain unfinished until an explicit retry.',
      'In Plan mode, save implementation plans with the advertised plan-saving tool; todo_write only updates the progress list.'
    ].join(' '),
    inputSchema: {
      type: 'object',
      properties: {
        todos: {
          type: 'array',
          description: 'Complete replacement todo table for this thread.',
          maxItems: 200,
          items: {
            type: 'object',
            properties: {
              id: { type: 'string' },
              content: { type: 'string' },
              status: {
                type: 'string',
                enum: ['pending', 'in_progress', 'completed', 'failed', 'canceled']
              },
              statusReasonCode: {
                type: 'string',
                enum: [
                  'execution_failed', 'dependency_failed', 'verification_failed', 'timeout',
                  'tool_failed', 'subagent_failed', 'runtime_failed',
                  'user_canceled', 'parent_canceled', 'runtime_canceled', 'superseded'
                ]
              },
              note: { type: 'string' },
              source: {
                oneOf: [
                  {
                    type: 'object',
                    properties: { kind: { type: 'string', enum: ['manual'] } },
                    required: ['kind'],
                    additionalProperties: false
                  },
                  {
                    type: 'object',
                    properties: {
                      kind: { type: 'string', enum: ['plan'] },
                      planId: { type: 'string' },
                      relativePath: { type: 'string' },
                      ordinal: { type: 'integer', minimum: 0 },
                      contentHash: { type: 'string' }
                    },
                    required: ['kind'],
                    additionalProperties: false
                  },
                  {
                    type: 'object',
                    properties: {
                      kind: { type: 'string', enum: ['child'] },
                      parentThreadId: { type: 'string' },
                      childThreadId: { type: 'string' },
                      childRunId: { type: 'string' },
                      jobId: { type: 'string' },
                      projectionId: { type: 'string' }
                    },
                    required: ['kind'],
                    additionalProperties: false
                  }
                ]
              }
            },
            required: ['content', 'status'],
            additionalProperties: false
          }
        }
      },
      required: ['todos'],
      additionalProperties: false
    },
    policy: 'auto',
    toolKind: 'tool_call',
    execute: async (args, context) => {
      if (!Array.isArray(args.todos)) {
        return { output: { error: 'todos must be an array' }, isError: true }
      }
      try {
        const todos = await threadService.setTodos(context.threadId, {
          turnId: context.turnId,
          todos: args.todos as SetThreadTodosRequest['todos']
        })
        return { output: todoResponse(todos) }
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        return { output: { error: message }, isError: true }
      }
    }
  })
}

function todoResponse(todos: ThreadTodoList | null): { todos: ThreadTodoList | null } {
  return { todos }
}
