export type DataAnalysisBackendRuntimeState = {
  apiBase?: string
  wsBase?: string
  healthUrl?: string
  host?: string
  port?: number
  pid?: number
  phase?: 'stopped' | 'starting' | 'running' | 'failed'
  launchId?: string
  generation?: number
  detail?: string
  restartCount?: number
  lastError?: string
  updatedAt?: string
	blocker?: 'data_analysis_native_authority_unavailable'
	authority?: 'go-native' | 'unavailable'
	terminal?: boolean
}

export type DataAnalysisRuntimeInfo = {
  isPackaged: boolean
  backend: DataAnalysisBackendRuntimeState & {
    managed: boolean
    pythonExec?: string
    pythonVersion?: string
    venvDir?: string
  }
}

export type DataAnalysisPickFilesOptions = {
  title?: string
  filters?: Array<{ name: string; extensions: string[] }>
  properties?: string[]
}

export type DataAnalysisPickFilesResult = {
  canceled?: boolean
  paths?: string[]
  filePaths?: string[]
}

export type DataAnalysisPickPathResult = {
  canceled?: boolean
  path?: string
  filePath?: string
}

export type DataAnalysisWindowChromeOptions = {
  backgroundColor?: string
  symbolColor?: string
  height?: number
}

export type DataAnalysisWorkspaceCaseResult =
  { ok: true; caseId: string; workspaceRoot: string } | { ok: false; message: string }

export type DataAnalysisPathOpenResult = { ok: boolean; message?: string }

export type DataAnalysisBridgeApi = {
  ensureBackend: () => Promise<DataAnalysisBackendRuntimeState>
  ensureWorkspaceCase: (payload: { workspaceRoot: string }) => Promise<DataAnalysisWorkspaceCaseResult>
  getRuntimeInfo: () => Promise<DataAnalysisRuntimeInfo>
  onBackendRuntimeState: (handler: (state: DataAnalysisBackendRuntimeState) => void) => () => void
  pickFiles: (options?: DataAnalysisPickFilesOptions) => Promise<string[] | DataAnalysisPickFilesResult>
  pickDirectory: () => Promise<string | DataAnalysisPickPathResult>
  openExternal: (url: string) => Promise<boolean | DataAnalysisPathOpenResult>
  setWindowChrome: (options?: DataAnalysisWindowChromeOptions) => Promise<boolean | DataAnalysisPathOpenResult>
}
