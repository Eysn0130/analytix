import { readFile } from 'node:fs/promises'
import { describe, expect, it } from 'vitest'
import type { IpcMain } from 'electron'
import { registerBackgroundTaskIpc } from './background-task-ipc'

type Handler = (event: unknown, payload: unknown) => Promise<unknown>

function createIpcHarness(): {
  handlers: Map<string, Handler>
  ipcMain: IpcMain
} {
  const handlers = new Map<string, Handler>()
  return {
    handlers,
    ipcMain: {
      handle: (channel: string, handler: Handler) => {
        handlers.set(channel, handler)
      }
    } as unknown as IpcMain
  }
}

async function invoke(
  handlers: Map<string, Handler>,
  channel: string,
  payload: unknown
): Promise<unknown> {
  const handler = handlers.get(channel)
  if (!handler) throw new Error(`Missing IPC handler: ${channel}`)
  return handler({}, payload)
}

describe('registerBackgroundTaskIpc', () => {
  it('keeps the compatibility surface stateless and routes authority to Go', async () => {
    const { handlers, ipcMain } = createIpcHarness()
    registerBackgroundTaskIpc({ ipcMain })

    expect([...handlers.keys()].sort()).toEqual([
      'background-task:kill',
      'background-task:list',
      'background-task:output',
      'background-task:register',
      'background-task:restart',
      'background-task:snapshot'
    ])

    const hostile = {
      threadId: 'PRIVATE_THREAD_6222020202020202020',
      taskId: 'PRIVATE_TASK',
      record: {
        id: 'PRIVATE_TASK',
        threadId: 'PRIVATE_THREAD',
        output: '<think>PRIVATE_REASONING</think>',
        error: 'PRIVATE_ERROR',
        command: 'printf PRIVATE_COMMAND'
      }
    }
    const results = {
      register: await invoke(handlers, 'background-task:register', hostile),
      list: await invoke(handlers, 'background-task:list', hostile),
      snapshot: await invoke(handlers, 'background-task:snapshot', hostile),
      output: await invoke(handlers, 'background-task:output', hostile),
      kill: await invoke(handlers, 'background-task:kill', hostile),
      restart: await invoke(handlers, 'background-task:restart', hostile)
    }

    expect(results.list).toEqual({ tasks: [] })
    expect(results.snapshot).toEqual({ tasks: [] })
    for (const mutation of [results.register, results.kill, results.restart]) {
      expect(mutation).toEqual({
        ok: false,
        message: 'Electron background task compatibility is retired; use the Go runtime task routes.'
      })
    }
    expect(results.output).toEqual({
      ok: true,
      availability: 'withheld',
      reasonCode: 'electron_background_tasks_retired',
      outputWithheld: true,
      canReadOutput: false,
      factAnswerAllowed: false,
      evidenceAuthority: false
    })
    expect(JSON.stringify(results)).not.toMatch(/PRIVATE_|6222020202020202020|<think>|taskId|threadId|outputBytes|status/)
  })

  it('does not instantiate a registry or touch filesystem/process APIs', async () => {
    const ipcSource = await readFile(new URL('./background-task-ipc.ts', import.meta.url), 'utf8')
    const mainSource = await readFile(new URL('./index.ts', import.meta.url), 'utf8')
    const rendererSource = await readFile(
      new URL('../renderer/src/components/summary/ThreadSummaryPanel.tsx', import.meta.url),
      'utf8'
    )

    for (const forbidden of [
      'background-task-registry',
      'BackgroundTaskRegistry',
      'readFile(',
      'writeFile(',
      'atomicWriteFile(',
      'process.kill(',
      'spawn(',
      '.parse(payload)',
      'payload.'
    ]) {
      expect(ipcSource).not.toContain(forbidden)
    }
    expect(mainSource).not.toContain('BackgroundTaskRegistry')
    expect(mainSource).not.toContain("background-tasks.json")
    expect(rendererSource).not.toContain('window.analytix?.backgroundTasks')
    expect(rendererSource).not.toContain("task.source === 'process'")
  })
})
