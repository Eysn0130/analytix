import type { IpcMain } from 'electron'
import type {
  BackgroundTaskListResult,
  BackgroundTaskMutationResult,
  BackgroundTaskOutputResult
} from '../shared/background-task'

export type RegisterBackgroundTaskIpcOptions = {
  ipcMain: IpcMain
}

export function registerBackgroundTaskIpc(options: RegisterBackgroundTaskIpcOptions): void {
  const { ipcMain } = options

  ipcMain.handle('background-task:register', async (): Promise<BackgroundTaskMutationResult> => retiredMutation())

  ipcMain.handle('background-task:list', async (): Promise<BackgroundTaskListResult> => ({ tasks: [] }))

  ipcMain.handle('background-task:snapshot', async (): Promise<BackgroundTaskListResult> => ({ tasks: [] }))

  ipcMain.handle('background-task:output', async (): Promise<BackgroundTaskOutputResult> => ({
    ok: true,
    availability: 'withheld',
    reasonCode: 'electron_background_tasks_retired',
    outputWithheld: true,
    canReadOutput: false,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }))

  ipcMain.handle('background-task:kill', async (): Promise<BackgroundTaskMutationResult> => retiredMutation())

  ipcMain.handle('background-task:restart', async (): Promise<BackgroundTaskMutationResult> => retiredMutation())
}

function retiredMutation(): BackgroundTaskMutationResult {
  return {
    ok: false,
    message: 'Electron background task compatibility is retired; use the Go runtime task routes.'
  }
}
