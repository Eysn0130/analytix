import { WRITE_SHUTDOWN_REQUEST, WRITE_SHUTDOWN_ACK, writeShutdownRequestSchema, writeShutdownResultSchema, type WriteShutdownHandler } from '../shared/write-shutdown'
import { canvasHostRequestSchema, canvasHostResponseSchema } from '../../packages/runtime/src/contracts/canvas-host'
import { nativeOfficeActionChoiceSchema, nativeOfficeMenuTargetSchema, nativeOfficePickerResponseSchema, nativeOfficeResponseSchema, nativeOfficeViewSchema, nativeWorkspaceCommandSchema, nativeOfficeInputFreezeSchema } from '../shared/native-office'
import { contextBridge, ipcRenderer, webUtils } from 'electron'
import type { AnalytixApi, AnalytixFlatApi } from '../shared/analytix-api'
import { WINDOW_STARTUP_SURFACE_READY_CHANNEL } from '../shared/window-startup'
import {
  isStrictPublicRuntimeSseEndPayload,
  isStrictPublicRuntimeSseErrorPayload,
  isStrictPublicRuntimeSseIpcPayload
} from '../shared/public-runtime-sse'
import { parseRuntimeStatusPublicV1 } from '../shared/analytix-runtime-status'

let officeAnnotationInputFrozen = false
const officeAnnotationFreezeListeners = new Set<(frozen: boolean) => void>()
ipcRenderer.on('office:annotation-input-freeze', (_event, value: unknown) => {
  const request = nativeOfficeInputFreezeSchema.safeParse(value)
  if (!request.success) return
  officeAnnotationInputFrozen = request.data.frozen
  try {
    for (const listener of officeAnnotationFreezeListeners) listener(officeAnnotationInputFrozen)
  } catch { return } // A failed UI freeze cannot acknowledge safe shutdown.
  // Earlier annotationSave invokes have already been sent on this renderer's
  // IPC stream. Main drains them only after receiving this acknowledgement.
  void ipcRenderer.invoke('office:annotation-input-frozen',request.data).catch(() => undefined)
})

let writeShutdownHandler: WriteShutdownHandler | undefined
ipcRenderer.on(WRITE_SHUTDOWN_REQUEST, (_event, value: unknown) => {
  const parsed = writeShutdownRequestSchema.safeParse(value)
  if (!parsed.success) return
  const request = parsed.data
  // Do not serialize cancel behind a slow prepare/save. The renderer fences
  // stale completions by request ID while the existing save remains in flight.
  void (async () => {
    let outcome: import('../shared/write-shutdown').WriteShutdownResult = { result: 'blocked', reason: 'unavailable' }
    try {
      if (writeShutdownHandler) {
        const result = writeShutdownResultSchema.safeParse(await writeShutdownHandler(request))
        if (result.success) outcome = result.data
      }
    } catch { /* Fail closed without exposing draft contents. */ }
    await ipcRenderer.invoke(WRITE_SHUTDOWN_ACK, { ...request, outcome }).catch(() => undefined)
  })()
})

// Internal IPC adapter. The renderer receives only the domain facade below.
const flatApi = {
  platform: process.platform,
  startupSurfaceReady: () => ipcRenderer.send(WINDOW_STARTUP_SURFACE_READY_CHANNEL),
  getSettings: () => ipcRenderer.invoke('settings:get'),
  setSettings: (partial) =>
    ipcRenderer.invoke('settings:set', partial),
  saveSettingsSilent: (partial) =>
    ipcRenderer.invoke('settings:save-silent', partial),
  getHubAccountSnapshot: () =>
    ipcRenderer.invoke('hub-account:snapshot'),
  refreshHubAccount: () =>
    ipcRenderer.invoke('hub-account:refresh'),
  loginHubAccount: (request) =>
    ipcRenderer.invoke('hub-account:login', request),
  registerHubAccount: (request) =>
    ipcRenderer.invoke('hub-account:register', request),
  logoutHubAccount: () =>
    ipcRenderer.invoke('hub-account:logout'),
  sendHubRegisterVerificationCode: (request) =>
    ipcRenderer.invoke('hub-account:register-code', request),
  sendHubPasswordResetVerificationCode: (request) =>
    ipcRenderer.invoke('hub-account:password-reset-code', request),
  confirmHubPasswordReset: (request) =>
    ipcRenderer.invoke('hub-account:password-reset-confirm', request),
  fetchHubAuthChallenge: (mode) =>
    ipcRenderer.invoke('hub-account:challenge', mode),
  fetchHubAuthChallengeState: (mode) =>
    ipcRenderer.invoke('hub-account:challenge-state', mode),
  verifyHubAuthChallenge: (request) =>
    ipcRenderer.invoke('hub-account:challenge-verify', request),
  updateHubProfile: (request) =>
    ipcRenderer.invoke('hub-account:profile-update', request),
  syncHubProfileEvents: (request) =>
    ipcRenderer.invoke('hub-account:profile-events', request),
  syncHubLocalProfileEvents: () =>
    ipcRenderer.invoke('hub-account:profile-events:local-sync'),
  getHubUsage: () =>
    ipcRenderer.invoke('hub-account:usage'),
  getHubReferral: () =>
    ipcRenderer.invoke('hub-account:referral'),
  getHubModels: () =>
    ipcRenderer.invoke('hub-account:models'),
  providerRegistryRequest: (request) =>
    ipcRenderer.invoke('provider-registry:request', request),
  createDestinationRequest: () =>
    ipcRenderer.invoke('provider-credential-recovery:create-destination-request'),
  createSourceBundle: () =>
    ipcRenderer.invoke('provider-credential-recovery:create-source-bundle'),
  applyDestinationBundle: () =>
    ipcRenderer.invoke('provider-credential-recovery:apply-destination-bundle'),
  finalizeSourceReceipt: () =>
    ipcRenderer.invoke('provider-credential-recovery:finalize-source-receipt'),
  configureProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:configure', request),
  beginProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:begin', request),
  statusProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:status', request),
  cancelProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:cancel', request),
  revokeProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:revoke', request),
  deleteProviderOAuth: (request) => ipcRenderer.invoke('provider-oauth:delete', request),
  replaceProviderOAuthSubscription: (request) =>
    ipcRenderer.invoke('provider-oauth:replace-subscription', request),
  beginMcpOAuth: (request) => ipcRenderer.invoke('mcp-oauth:begin', request),
  statusMcpOAuth: (request) => ipcRenderer.invoke('mcp-oauth:status', request),
  cancelMcpOAuth: (request) => ipcRenderer.invoke('mcp-oauth:cancel', request),
  revokeMcpOAuth: (request) => ipcRenderer.invoke('mcp-oauth:revoke', request),
  deleteMcpOAuth: (request) => ipcRenderer.invoke('mcp-oauth:delete', request),
  beginExtensionOAuth: (request) => ipcRenderer.invoke('extension-oauth:begin', request),
  statusExtensionOAuth: (request) => ipcRenderer.invoke('extension-oauth:status', request),
  cancelExtensionOAuth: (request) => ipcRenderer.invoke('extension-oauth:cancel', request),
  revokeExtensionOAuth: (request) => ipcRenderer.invoke('extension-oauth:revoke', request),
  deleteExtensionOAuth: (request) => ipcRenderer.invoke('extension-oauth:delete', request),
  invalidateQueryCache: (payload) =>
    ipcRenderer.invoke('query-cache:invalidate', payload),
  onQueryCacheInvalidated: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('query-cache:invalidate', wrapped)
    return () => ipcRenderer.removeListener('query-cache:invalidate', wrapped)
  },
  runtimeRequest: (path, method, body) =>
    ipcRenderer.invoke('runtime:request', { path, method, body }),
  importMappingPreview: (request) =>
    ipcRenderer.invoke('runtime:import-mapping-preview', request),
  cleaningDiffPreview: (request) =>
    ipcRenderer.invoke('runtime:cleaning-diff-preview', request),
  directSourcePreview: (request) =>
    ipcRenderer.invoke('runtime:direct-source-preview', request),
  acceptedSlotDisplay: (request) =>
    ipcRenderer.invoke('runtime:accepted-slot-display', request),
  stageFundsCSVSnapshot: () =>
    ipcRenderer.invoke('runtime:stage-funds-csv-snapshot'),
  confirmFundsCSVSnapshot: (selector) =>
    ipcRenderer.invoke('runtime:confirm-funds-csv-snapshot', selector),
  cancelFundsCSVImport: (selector) =>
    ipcRenderer.invoke('runtime:cancel-funds-csv-import', selector),
  statusFundsCSVImport: (selector) =>
    ipcRenderer.invoke('runtime:status-funds-csv-import', selector),
  runDeterministicFundsCleaning: () =>
    ipcRenderer.invoke('runtime:run-deterministic-funds-cleaning'),
  revokeCleaningDiffPreview: (selector) =>
    ipcRenderer.invoke('runtime:revoke-cleaning-diff-preview', selector),
  restartRuntime: () => ipcRenderer.invoke('runtime:restart'),
  fetchUpstreamModels: () => ipcRenderer.invoke('upstream:models'),
  probeModelCapabilities: (payload) => ipcRenderer.invoke('provider:capability-probe', payload),
  getClawStatus: () => ipcRenderer.invoke('claw:status'),
  runClawTask: (taskId) =>
    ipcRenderer.invoke('claw:task:run', taskId),
  getScheduleStatus: () => ipcRenderer.invoke('schedule:status'),
  runScheduleTask: (taskId) =>
    ipcRenderer.invoke('schedule:task:run', taskId),
  startClawImInstallQr: (provider, options) =>
    ipcRenderer.invoke('claw:im-install:qrcode', { provider, isLark: options?.isLark }),
  pollClawImInstall: (provider, deviceCode) =>
    ipcRenderer.invoke('claw:im-install:poll', { provider, deviceCode }),
  connectTelegramBot: (botToken, allowedChatIds) =>
    ipcRenderer.invoke('claw:im-install:telegram-token', { botToken, allowedChatIds }),
  disconnectClawImChannel: (channelId) =>
    ipcRenderer.invoke('claw:im-channel:disconnect', { channelId }),
  pickWorkspaceDirectory: (defaultPath) =>
    ipcRenderer.invoke('workspace:pick-directory', defaultPath),
  pickLocalFiles: (defaultPath) =>
    ipcRenderer.invoke('file:pick-local-files', defaultPath),
  confirmDialog: (options) =>
    ipcRenderer.invoke('dialog:confirm', options),
  detectLegacySessions: () =>
    ipcRenderer.invoke('analytix:sessions:detect-legacy-kun'),
  importLegacySessions: (sourceDir) =>
    ipcRenderer.invoke('analytix:sessions:import-legacy-kun', { sourceDir }),
  pickLegacySessionDir: () =>
    ipcRenderer.invoke('analytix:sessions:pick-legacy-kun-source-dir'),
  listSkills: (workspaceRoot) =>
    ipcRenderer.invoke('skill:list', { workspaceRoot }),
  listSkillRoots: (workspaceRoot) =>
    ipcRenderer.invoke('skill:list-roots', { workspaceRoot }),
  saveSkillFile: (rootPath, skillName, content) =>
    ipcRenderer.invoke('skill:save-file', { rootPath, skillName, content }),
  deleteSkill: (rootPath, skillName) =>
    ipcRenderer.invoke('skill:delete', { rootPath, skillName }),
  openSkillRoot: (rootPath) =>
    ipcRenderer.invoke('skill:open-root', rootPath),
  listUiPlugins: () =>
    ipcRenderer.invoke('ui-plugin:list'),
  installUiPlugin: () =>
    ipcRenderer.invoke('ui-plugin:install'),
  removeUiPlugin: (id) =>
    ipcRenderer.invoke('ui-plugin:remove', { id }),
  loadUiPlugin: (id) =>
    ipcRenderer.invoke('ui-plugin:load', { id }),
  syncHubAgentMarketplace: (options) =>
    ipcRenderer.invoke('hub-agent-marketplace:sync', options ?? {}),
  installHubAgentPlugin: (request) =>
    ipcRenderer.invoke('hub-agent-marketplace:install-plugin', request),
  uninstallHubAgentPlugin: (request) =>
    ipcRenderer.invoke('hub-agent-marketplace:uninstall-plugin', request),
  readHubAgentSkillMarkdown: (request) =>
    ipcRenderer.invoke('hub-agent-marketplace:read-skill-markdown', request),
  getAnalytixConfigFile: () =>
    ipcRenderer.invoke('analytix:runtime-config:read'),
  setAnalytixConfigFile: (content) =>
    ipcRenderer.invoke('analytix:runtime-config:write', content),
  openAnalytixConfigDir: () =>
    ipcRenderer.invoke('analytix:runtime-config:open-dir'),
  getGitBranches: (workspaceRoot) =>
    ipcRenderer.invoke('git:branches', workspaceRoot),
  switchGitBranch: (workspaceRoot, branch) =>
    ipcRenderer.invoke('git:switch-branch', { workspaceRoot, branch }),
  createAndSwitchGitBranch: (workspaceRoot, branch) =>
    ipcRenderer.invoke('git:create-and-switch-branch', { workspaceRoot, branch }),
  createGitCheckpoint: (payload) =>
    ipcRenderer.invoke('git:checkpoint:create', payload),
  restoreGitCheckpoint: (payload) =>
    ipcRenderer.invoke('git:checkpoint:restore', payload),
  acquireWorktree: (params) =>
    ipcRenderer.invoke('worktree:acquire', params),
  releaseWorktree: (params) =>
    ipcRenderer.invoke('worktree:release', params),
  listWorktrees: (params) =>
    ipcRenderer.invoke('worktree:list', params),
  removeWorktree: (params) =>
    ipcRenderer.invoke('worktree:remove', params),
  getWorktreeChanges: (params) =>
    ipcRenderer.invoke('worktree:changes', params),
  commitWorktree: (params) =>
    ipcRenderer.invoke('worktree:commit', params),
  mergeWorktree: (params) =>
    ipcRenderer.invoke('worktree:merge', params),
  abortWorktreeMerge: (params) =>
    ipcRenderer.invoke('worktree:abort-merge', params),
  continueWorktreeMerge: (params) =>
    ipcRenderer.invoke('worktree:continue-merge', params),
  syncWorktreeFromMain: (params) =>
    ipcRenderer.invoke('worktree:sync', params),
  abortWorktreeRebase: (params) =>
    ipcRenderer.invoke('worktree:abort-rebase', params),
  cleanupWorktrees: (params) =>
    ipcRenderer.invoke('worktree:cleanup', params),
  findAvailableWorktreePoolIndex: (params) =>
    ipcRenderer.invoke('worktree:find-available', params),
  startThreadHandoff: (request) =>
    ipcRenderer.invoke('thread-handoff:start', request),
  retryThreadHandoff: (params) =>
    ipcRenderer.invoke('thread-handoff:retry', params),
  getThreadHandoffOperations: (params) =>
    ipcRenderer.invoke('thread-handoff:get', params ?? {}),
  cancelThreadHandoff: (params) =>
    ipcRenderer.invoke('thread-handoff:cancel', params),
  removeThreadHandoff: (params) =>
    ipcRenderer.invoke('thread-handoff:remove', params),
  completeThreadHandoffSwitch: (params) =>
    ipcRenderer.invoke('thread-handoff:complete-switch', params),
  failThreadHandoffSwitch: (params) =>
    ipcRenderer.invoke('thread-handoff:fail-switch', params),
  onThreadHandoffEvent: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('thread-handoff:event', wrapped)
    return () => ipcRenderer.removeListener('thread-handoff:event', wrapped)
  },
  listEditors: () => ipcRenderer.invoke('editor:list'),
  openEditorPath: (options) =>
    ipcRenderer.invoke('editor:open-path', options),
  listWorkspaceDirectory: (options) =>
    ipcRenderer.invoke('file:list-workspace-directory', options),
  resolveWorkspaceFile: (options) =>
    ipcRenderer.invoke('file:resolve-workspace', options),
  readWorkspaceFile: (options) =>
    ipcRenderer.invoke('file:read-workspace', options),
  readWorkspaceImage: (options) =>
    ipcRenderer.invoke('file:read-workspace-image', options),
  readWorkspacePdf: (options) =>
    ipcRenderer.invoke('file:read-workspace-pdf', options),
  readLocalPdfText: (options) =>
    ipcRenderer.invoke('file:read-local-pdf-text', options),
  saveWorkspaceFileAs: (payload) =>
    ipcRenderer.invoke('file:save-as', payload),
  writeWorkspaceFile: (payload) =>
    ipcRenderer.invoke('file:write-workspace', payload),
  createWorkspaceFile: (payload) =>
    ipcRenderer.invoke('file:create-workspace', payload),
  createWorkspaceDirectory: (payload) =>
    ipcRenderer.invoke('file:create-workspace-directory', payload),
  saveWorkspaceClipboardImage: (payload) =>
    ipcRenderer.invoke('file:save-workspace-clipboard-image', payload),
  readClipboardImage: () =>
    ipcRenderer.invoke('clipboard:read-image'),
  getPathForFile: (file) =>
    webUtils.getPathForFile(file),
  renameWorkspaceEntry: (payload) =>
    ipcRenderer.invoke('file:rename-workspace-entry', payload),
  deleteWorkspaceEntry: (payload) =>
    ipcRenderer.invoke('file:delete-workspace-entry', payload),
  watchWorkspaceFile: (payload) =>
    ipcRenderer.invoke('file:watch-workspace', payload),
  unwatchWorkspaceFile: (watchId) =>
    ipcRenderer.invoke('file:unwatch-workspace', watchId),
  onWorkspaceFileChanged: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('file:workspace-changed', wrapped)
    return () => ipcRenderer.removeListener('file:workspace-changed', wrapped)
  },
  exportWriteDocument: (payload) =>
    ipcRenderer.invoke('write:export', payload),
  copyWriteDocumentAsRichText: (payload) =>
    ipcRenderer.invoke('write:copy-rich-text', payload),
  requestWriteInlineCompletion: (payload) =>
    ipcRenderer.invoke('write:inline-completion', payload),
  cancelWriteInlineCompletion: (payload) =>
    ipcRenderer.invoke('write:inline-completion:cancel', payload),
  retrieveWriteContext: (payload) =>
    ipcRenderer.invoke('write:retrieve-context', payload),
  generateWriteInfographic: (payload) =>
    ipcRenderer.invoke('write:generate-infographic', payload),
  authorizeWritePrototype: (payload) =>
    ipcRenderer.invoke('write:authorize-prototype', payload),
  openWritePrototype: (payload) =>
    ipcRenderer.invoke('write:open-prototype', payload),
  transcribeSpeech: (payload) =>
    ipcRenderer.invoke('speech:transcribe', payload),
  listWriteInlineCompletionDebugEntries: () =>
    ipcRenderer.invoke('write:inline-completion-debug:list'),
  clearWriteInlineCompletionDebugEntries: () =>
    ipcRenderer.invoke('write:inline-completion-debug:clear'),
  startSse: (threadId, sinceSeq, streamId) =>
    ipcRenderer.invoke('runtime:sse:start', { threadId, sinceSeq, streamId }),
  ackSseEvent: (streamId, seq, acceptedFinal) =>
    ipcRenderer.invoke('runtime:sse:ack', acceptedFinal
      ? { streamId, seq, ...acceptedFinal }
      : { streamId, seq }),
  stopSse: (streamId) => ipcRenderer.invoke('runtime:sse:stop', streamId),
  onSseEvent: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: unknown
    ) => {
      if (isStrictPublicRuntimeSseIpcPayload(payload)) handler(payload)
    }
    ipcRenderer.on('runtime:sse-event', wrapped)
    return () => ipcRenderer.removeListener('runtime:sse-event', wrapped)
  },
  onSseEnd: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: unknown
    ) => {
      if (isStrictPublicRuntimeSseEndPayload(payload)) handler(payload)
    }
    ipcRenderer.on('runtime:sse-end', wrapped)
    return () => ipcRenderer.removeListener('runtime:sse-end', wrapped)
  },
  onSseError: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: unknown
    ) => {
      if (isStrictPublicRuntimeSseErrorPayload(payload)) handler(payload)
    }
    ipcRenderer.on('runtime:sse-error', wrapped)
    return () => ipcRenderer.removeListener('runtime:sse-error', wrapped)
  },
  onClawChannelActivity: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('claw:channel-activity', wrapped)
    return () => ipcRenderer.removeListener('claw:channel-activity', wrapped)
  },
  onRuntimeStatus: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: unknown
    ) => {
      const publicStatus = parseRuntimeStatusPublicV1(payload)
      if (publicStatus) handler(publicStatus)
    }
    ipcRenderer.on('runtime:status', wrapped)
    return () => ipcRenderer.removeListener('runtime:status', wrapped)
  },
  mirrorClawChannelMessage: (threadId, text, direction) =>
    ipcRenderer.invoke('claw:channel:mirror', { threadId, text, direction }),
  mirrorClawChannelMessageToFeishu: (threadId, text, direction) =>
    ipcRenderer.invoke('claw:channel:mirror-to-feishu', { threadId, text, direction }),
  createClawTaskFromText: (text, options) =>
    ipcRenderer.invoke('claw:task:create-from-text', {
      text,
      channelId: options?.channelId,
      providerId: options?.providerId,
      modelHint: options?.modelHint,
      reasoningEffort: options?.reasoningEffort,
      mode: options?.mode
    }),
  createScheduleTaskFromText: (text, options) =>
    ipcRenderer.invoke('schedule:task:create-from-text', {
      text,
      workspaceRoot: options?.workspaceRoot,
      clawChannelId: options?.clawChannelId,
      providerId: options?.providerId,
      modelHint: options?.modelHint,
      reasoningEffort: options?.reasoningEffort,
      mode: options?.mode
    }),
  runDesktopCommand: (command) =>
    ipcRenderer.invoke('desktop:command', command),
  openThreadInNewWindow: (threadId) =>
    ipcRenderer.invoke('thread:open-new-window', { threadId }),
  openExternal: (url) => ipcRenderer.invoke('shell:open-external', url),
  getComputerUsePermissions: () => ipcRenderer.invoke('computer-use:permissions'),
  getComputerUseDoctor: () => ipcRenderer.invoke('computer-use:doctor'),
  getChromeBrowserUseStatus: () => ipcRenderer.invoke('chrome-browser-use:status'),
  openChromeBrowserUseExtensionPage: (target) =>
    ipcRenderer.invoke('chrome-browser-use:open-extension-page', target),
  requestComputerUsePermission: (kind) =>
    ipcRenderer.invoke('computer-use:request-permission', kind),
  showTurnCompleteNotification: (payload) => ipcRenderer.invoke('notification:turn-complete', payload),
  getAppVersion: () => ipcRenderer.invoke('app:version'),
  getGuiUpdateState: () => ipcRenderer.invoke('gui:update-state'),
  checkGuiUpdate: (channel) =>
    ipcRenderer.invoke('gui:update-check', channel),
  downloadGuiUpdate: (channel) =>
    ipcRenderer.invoke('gui:update-download', channel),
  installGuiUpdate: () => ipcRenderer.invoke('gui:update-install'),
  onGuiUpdateState: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('gui:update-state', wrapped)
    return () => ipcRenderer.removeListener('gui:update-state', wrapped)
  },
  logError: (category, message, detail) =>
    ipcRenderer.invoke('log:error', { category, message, detail }),
  getLogPath: () => ipcRenderer.invoke('log:get-path'),
  openLogDir: () => ipcRenderer.invoke('log:open-dir'),
  recordThreadTrace: (event) =>
    ipcRenderer.invoke('diagnostics:thread-trace', event),
  createTerminal: (payload) => ipcRenderer.invoke('terminal:create', payload),
  writeToTerminal: (payload) => ipcRenderer.invoke('terminal:write', payload),
  resizeTerminal: (payload) => ipcRenderer.invoke('terminal:resize', payload),
  disposeTerminal: (sessionId) => ipcRenderer.invoke('terminal:dispose', sessionId),
  onTerminalData: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('terminal:data', wrapped)
    return () => ipcRenderer.removeListener('terminal:data', wrapped)
  },
  onTerminalExit: (handler) => {
    const wrapped = (
      _: Electron.IpcRendererEvent,
      payload: Parameters<typeof handler>[0]
    ) => handler(payload)
    ipcRenderer.on('terminal:exit', wrapped)
    return () => ipcRenderer.removeListener('terminal:exit', wrapped)
  },
  registerBackgroundTask: (payload) =>
    ipcRenderer.invoke('background-task:register', payload),
  listBackgroundTasks: (payload) =>
    ipcRenderer.invoke('background-task:list', payload),
  snapshotBackgroundTasks: (payload) =>
    ipcRenderer.invoke('background-task:snapshot', payload),
  killBackgroundTask: (payload) =>
    ipcRenderer.invoke('background-task:kill', payload),
  restartBackgroundTask: (payload) =>
    ipcRenderer.invoke('background-task:restart', payload),
  readBackgroundTaskOutput: (payload) =>
    ipcRenderer.invoke('background-task:output', payload)
} satisfies AnalytixFlatApi

const api = {
  settings: {
    getSettings: flatApi.getSettings,
    setSettings: flatApi.setSettings,
    saveSettingsSilent: flatApi.saveSettingsSilent
  },
  account: {
    getSnapshot: flatApi.getHubAccountSnapshot,
    refresh: flatApi.refreshHubAccount,
    login: flatApi.loginHubAccount,
    register: flatApi.registerHubAccount,
    logout: flatApi.logoutHubAccount,
    sendRegisterVerificationCode: flatApi.sendHubRegisterVerificationCode,
    sendPasswordResetVerificationCode: flatApi.sendHubPasswordResetVerificationCode,
    confirmPasswordReset: flatApi.confirmHubPasswordReset,
    fetchChallenge: flatApi.fetchHubAuthChallenge,
    fetchChallengeState: flatApi.fetchHubAuthChallengeState,
    verifyChallenge: flatApi.verifyHubAuthChallenge,
    updateProfile: flatApi.updateHubProfile,
    syncProfileEvents: flatApi.syncHubProfileEvents,
    syncLocalProfileEvents: flatApi.syncHubLocalProfileEvents,
    getUsage: flatApi.getHubUsage,
    getReferral: flatApi.getHubReferral,
    getModels: flatApi.getHubModels
  },
  providerRegistry: {
    request: flatApi.providerRegistryRequest
  },
  providerCredentialRecovery: {
    createDestinationRequest: flatApi.createDestinationRequest,
    createSourceBundle: flatApi.createSourceBundle,
    applyDestinationBundle: flatApi.applyDestinationBundle,
    finalizeSourceReceipt: flatApi.finalizeSourceReceipt
  },
  providerOAuth: {
    configure: flatApi.configureProviderOAuth,
    begin: flatApi.beginProviderOAuth,
    status: flatApi.statusProviderOAuth,
    cancel: flatApi.cancelProviderOAuth,
    revoke: flatApi.revokeProviderOAuth,
    delete: flatApi.deleteProviderOAuth,
    replaceSubscription: flatApi.replaceProviderOAuthSubscription
  },
  mcpOAuth: {
    begin: flatApi.beginMcpOAuth,
    status: flatApi.statusMcpOAuth,
    cancel: flatApi.cancelMcpOAuth,
    revoke: flatApi.revokeMcpOAuth,
    delete: flatApi.deleteMcpOAuth
  },
  extensionOAuth: {
    begin: flatApi.beginExtensionOAuth,
    status: flatApi.statusExtensionOAuth,
    cancel: flatApi.cancelExtensionOAuth,
    revoke: flatApi.revokeExtensionOAuth,
    delete: flatApi.deleteExtensionOAuth
  },
  runtime: {
    runtimeRequest: flatApi.runtimeRequest,
    importMappingPreview: flatApi.importMappingPreview,
    cleaningDiffPreview: flatApi.cleaningDiffPreview,
    directSourcePreview: flatApi.directSourcePreview,
    acceptedSlotDisplay: flatApi.acceptedSlotDisplay,
    stageFundsCSVSnapshot: flatApi.stageFundsCSVSnapshot,
    confirmFundsCSVSnapshot: flatApi.confirmFundsCSVSnapshot,
    cancelFundsCSVImport: flatApi.cancelFundsCSVImport,
    statusFundsCSVImport: flatApi.statusFundsCSVImport,
    runDeterministicFundsCleaning: flatApi.runDeterministicFundsCleaning,
    revokeCleaningDiffPreview: flatApi.revokeCleaningDiffPreview,
    restartRuntime: flatApi.restartRuntime,
    fetchUpstreamModels: flatApi.fetchUpstreamModels,
    probeModelCapabilities: flatApi.probeModelCapabilities,
    startSse: flatApi.startSse,
    ackSseEvent: flatApi.ackSseEvent,
    stopSse: flatApi.stopSse,
    onSseEvent: flatApi.onSseEvent,
    onSseEnd: flatApi.onSseEnd,
    onSseError: flatApi.onSseError,
    onRuntimeStatus: flatApi.onRuntimeStatus,
    getAnalytixConfigFile: flatApi.getAnalytixConfigFile,
    setAnalytixConfigFile: flatApi.setAnalytixConfigFile,
    openAnalytixConfigDir: flatApi.openAnalytixConfigDir
  },
  connectPhone: {
    getStatus: flatApi.getClawStatus,
    runTask: flatApi.runClawTask,
    startImInstallQr: flatApi.startClawImInstallQr,
    pollImInstall: flatApi.pollClawImInstall,
    connectTelegramBot: flatApi.connectTelegramBot,
    disconnectImChannel: flatApi.disconnectClawImChannel,
    onChannelActivity: flatApi.onClawChannelActivity,
    mirrorChannelMessage: flatApi.mirrorClawChannelMessage,
    mirrorChannelMessageToFeishu: flatApi.mirrorClawChannelMessageToFeishu,
    createTaskFromText: flatApi.createClawTaskFromText
  },
  schedule: {
    getStatus: flatApi.getScheduleStatus,
    runTask: flatApi.runScheduleTask,
    createTaskFromText: flatApi.createScheduleTaskFromText
  },
  workspace: {
    pickDirectory: flatApi.pickWorkspaceDirectory,
    getGitBranches: flatApi.getGitBranches,
    switchGitBranch: flatApi.switchGitBranch,
    createAndSwitchGitBranch: flatApi.createAndSwitchGitBranch,
    createGitCheckpoint: flatApi.createGitCheckpoint,
    restoreGitCheckpoint: flatApi.restoreGitCheckpoint,
    acquireWorktree: flatApi.acquireWorktree,
    releaseWorktree: flatApi.releaseWorktree,
    listWorktrees: flatApi.listWorktrees,
    removeWorktree: flatApi.removeWorktree,
    getWorktreeChanges: flatApi.getWorktreeChanges,
    commitWorktree: flatApi.commitWorktree,
    mergeWorktree: flatApi.mergeWorktree,
    abortWorktreeMerge: flatApi.abortWorktreeMerge,
    continueWorktreeMerge: flatApi.continueWorktreeMerge,
    syncWorktreeFromMain: flatApi.syncWorktreeFromMain,
    abortWorktreeRebase: flatApi.abortWorktreeRebase,
    cleanupWorktrees: flatApi.cleanupWorktrees,
    findAvailableWorktreePoolIndex: flatApi.findAvailableWorktreePoolIndex,
    startThreadHandoff: flatApi.startThreadHandoff,
    retryThreadHandoff: flatApi.retryThreadHandoff,
    getThreadHandoffOperations: flatApi.getThreadHandoffOperations,
    cancelThreadHandoff: flatApi.cancelThreadHandoff,
    removeThreadHandoff: flatApi.removeThreadHandoff,
    completeThreadHandoffSwitch: flatApi.completeThreadHandoffSwitch,
    failThreadHandoffSwitch: flatApi.failThreadHandoffSwitch,
    onThreadHandoffEvent: flatApi.onThreadHandoffEvent,
    listEditors: flatApi.listEditors,
    openEditorPath: flatApi.openEditorPath
  },
  office: {
    onAnnotationInputFreeze: (handler) => {
      officeAnnotationFreezeListeners.add(handler); handler(officeAnnotationInputFrozen)
      return () => { officeAnnotationFreezeListeners.delete(handler) }
    },
    onMenuRequested: (handler) => {
      const listener = (_: Electron.IpcRendererEvent, value: unknown) => {
        const parsed = nativeOfficeMenuTargetSchema.safeParse(value)
        if (parsed.success) handler(parsed.data)
      }
      ipcRenderer.on('office:menu-requested', listener)
      return () => ipcRenderer.removeListener('office:menu-requested', listener)
    },
    showActionMenu: async (request) => {
      const parsed = nativeOfficeActionChoiceSchema.safeParse(await ipcRenderer.invoke('office:action-menu', request))
      return parsed.success ? parsed.data : {actionId:null}
    },
    onWorkspaceCommand: (handler) => {
      const listener = (_: Electron.IpcRendererEvent, value: unknown) => {
        const parsed = nativeWorkspaceCommandSchema.safeParse(value)
        if (parsed.success) handler(parsed.data)
      }
      ipcRenderer.on('office:workspace-command', listener)
      return () => ipcRenderer.removeListener('office:workspace-command', listener)
    },
    pickFile: async (request) => {
      const parsed = nativeOfficePickerResponseSchema.safeParse(await ipcRenderer.invoke('office:pick-file', request))
      return parsed.success ? parsed.data : { ok: false, error: 'unavailable' }
    },
    request: async (request) => {
      if (officeAnnotationInputFrozen && request.action === 'annotationSave') return {ok:false,view:null,error:'unsaved_changes'}
      const parsed = nativeOfficeResponseSchema.safeParse(await ipcRenderer.invoke('office:request', request))
      return parsed.success ? parsed.data : { ok: false, view: null, error: 'unavailable' }
    },
    onChange: (handler) => {
      const listener = (_: Electron.IpcRendererEvent, value: unknown) => {
        const parsed = nativeOfficeViewSchema.nullable().safeParse(value)
        if (parsed.success) handler(parsed.data)
      }
      ipcRenderer.on('office:changed', listener)
      return () => ipcRenderer.removeListener('office:changed', listener)
    }
  },
  packageHost: {
    request: (request) => ipcRenderer.invoke('plugin:package-host', request)
  },
  canvas: {
    request: async request => {
      const input = canvasHostRequestSchema.safeParse(request)
      if (!input.success) return { ok: false, code: 'invalid_request' }
      try {
        const result = canvasHostResponseSchema.safeParse(await ipcRenderer.invoke('canvas:request', input.data))
        if (result.success) return result.data
      } catch { /* Unknown operations are reconciled from Core recovery. */ }
      return { ok: false, code: 'unavailable' }
    },
    pickFile: async request => {
      try {
        const result: unknown = await ipcRenderer.invoke('canvas:pick-file', request)
        if (result && typeof result === 'object' && 'ok' in result && result.ok === true && 'path' in result &&
          (result.path === null || (typeof result.path === 'string' && result.path.length <= 4096 && !result.path.includes('\0')))) return { ok: true, path: result.path }
      } catch { /* Preserve the current surface. */ }
      return { ok: false }
    }
  },
  browserSelection: {
    request: (request) => ipcRenderer.invoke('browser:selection', request)
  },
  objects: {
    resolveArtifact: (request) => ipcRenderer.invoke('object:resolve-artifact', request),
    request: (request) => ipcRenderer.invoke('object:editing', request)
  },
  files: {
    listDirectory: flatApi.listWorkspaceDirectory,
    resolve: flatApi.resolveWorkspaceFile,
    read: flatApi.readWorkspaceFile,
    readImage: flatApi.readWorkspaceImage,
    readPdf: flatApi.readWorkspacePdf,
    readLocalPdfText: flatApi.readLocalPdfText,
    saveAs: flatApi.saveWorkspaceFileAs,
    write: flatApi.writeWorkspaceFile,
    createFile: flatApi.createWorkspaceFile,
    createDirectory: flatApi.createWorkspaceDirectory,
    saveClipboardImage: flatApi.saveWorkspaceClipboardImage,
    readClipboardImage: flatApi.readClipboardImage,
    pickLocalFiles: flatApi.pickLocalFiles,
    renameEntry: flatApi.renameWorkspaceEntry,
    deleteEntry: flatApi.deleteWorkspaceEntry,
    watch: flatApi.watchWorkspaceFile,
    unwatch: flatApi.unwatchWorkspaceFile,
    onChanged: flatApi.onWorkspaceFileChanged,
    getPathForFile: flatApi.getPathForFile
  },
  write: {
    onShutdown: handler => {
      if (writeShutdownHandler) throw new Error('Write shutdown handler already installed.')
      writeShutdownHandler = handler
      return () => { if (writeShutdownHandler === handler) writeShutdownHandler = undefined }
    },
    requestWriteInlineCompletion: flatApi.requestWriteInlineCompletion,
    cancelWriteInlineCompletion: flatApi.cancelWriteInlineCompletion,
    retrieveWriteContext: flatApi.retrieveWriteContext,
    generateWriteInfographic: flatApi.generateWriteInfographic,
    authorizeWritePrototype: flatApi.authorizeWritePrototype,
    openWritePrototype: flatApi.openWritePrototype,
    listWriteInlineCompletionDebugEntries: flatApi.listWriteInlineCompletionDebugEntries,
    clearWriteInlineCompletionDebugEntries: flatApi.clearWriteInlineCompletionDebugEntries,
    exportWriteDocument: flatApi.exportWriteDocument,
    copyWriteDocumentAsRichText: flatApi.copyWriteDocumentAsRichText
  },
  speech: {
    transcribe: flatApi.transcribeSpeech
  },
  terminal: {
    create: flatApi.createTerminal,
    write: flatApi.writeToTerminal,
    resize: flatApi.resizeTerminal,
    dispose: flatApi.disposeTerminal,
    onData: flatApi.onTerminalData,
    onExit: flatApi.onTerminalExit
  },
  backgroundTasks: {
    register: flatApi.registerBackgroundTask,
    list: flatApi.listBackgroundTasks,
    snapshot: flatApi.snapshotBackgroundTasks,
    kill: flatApi.killBackgroundTask,
    restart: flatApi.restartBackgroundTask,
    output: flatApi.readBackgroundTaskOutput
  },
  updates: {
    getState: flatApi.getGuiUpdateState,
    check: flatApi.checkGuiUpdate,
    download: flatApi.downloadGuiUpdate,
    install: flatApi.installGuiUpdate,
    onState: flatApi.onGuiUpdateState
  },
  logs: {
    error: flatApi.logError,
    getPath: flatApi.getLogPath,
    openDir: flatApi.openLogDir
  },
  app: {
    platform: flatApi.platform,
    startupSurfaceReady: flatApi.startupSurfaceReady,
    confirmDialog: flatApi.confirmDialog,
    runDesktopCommand: flatApi.runDesktopCommand,
    openThreadInNewWindow: flatApi.openThreadInNewWindow,
    openExternal: flatApi.openExternal,
    getComputerUsePermissions: flatApi.getComputerUsePermissions,
    getComputerUseDoctor: flatApi.getComputerUseDoctor,
    getChromeBrowserUseStatus: flatApi.getChromeBrowserUseStatus,
    openChromeBrowserUseExtensionPage: flatApi.openChromeBrowserUseExtensionPage,
    requestComputerUsePermission: flatApi.requestComputerUsePermission,
    invalidateQueryCache: flatApi.invalidateQueryCache,
    onQueryCacheInvalidated: flatApi.onQueryCacheInvalidated,
    showTurnCompleteNotification: flatApi.showTurnCompleteNotification,
    getVersion: flatApi.getAppVersion,
    listSkills: flatApi.listSkills,
    listSkillRoots: flatApi.listSkillRoots,
    saveSkillFile: flatApi.saveSkillFile,
    deleteSkill: flatApi.deleteSkill,
    openSkillRoot: flatApi.openSkillRoot,
    listUiPlugins: flatApi.listUiPlugins,
    installUiPlugin: flatApi.installUiPlugin,
    removeUiPlugin: flatApi.removeUiPlugin,
    loadUiPlugin: flatApi.loadUiPlugin,
    syncHubAgentMarketplace: flatApi.syncHubAgentMarketplace,
    installHubAgentPlugin: flatApi.installHubAgentPlugin,
    uninstallHubAgentPlugin: flatApi.uninstallHubAgentPlugin,
    readHubAgentSkillMarkdown: flatApi.readHubAgentSkillMarkdown
  },
  diagnostics: {
    detectLegacySessions: flatApi.detectLegacySessions,
    importLegacySessions: flatApi.importLegacySessions,
    pickLegacySessionDir: flatApi.pickLegacySessionDir,
    listWriteInlineCompletionDebugEntries: flatApi.listWriteInlineCompletionDebugEntries,
    clearWriteInlineCompletionDebugEntries: flatApi.clearWriteInlineCompletionDebugEntries,
    logError: flatApi.logError,
    getLogPath: flatApi.getLogPath,
    runtimeRequest: flatApi.runtimeRequest,
    recordThreadTrace: flatApi.recordThreadTrace
  },
  dataAnalysis: {
    ensureBackend: () => ipcRenderer.invoke('data-analysis:ensure-backend'),
    ensureWorkspaceCase: (payload) =>
      ipcRenderer.invoke('data-analysis:ensure-workspace-case', payload),
    getRuntimeInfo: () => ipcRenderer.invoke('data-analysis:runtime-info'),
    onBackendRuntimeState: (handler) => {
      const wrapped = (
        _: Electron.IpcRendererEvent,
        payload: Parameters<typeof handler>[0]
      ) => handler(payload)
      ipcRenderer.on('data-analysis:backend-runtime-state', wrapped)
      return () => ipcRenderer.removeListener('data-analysis:backend-runtime-state', wrapped)
    },
    pickFiles: (options) =>
      ipcRenderer.invoke('data-analysis:pick-files', options ?? {}),
    pickDirectory: () =>
      ipcRenderer.invoke('data-analysis:pick-directory'),
    openExternal: (url) => ipcRenderer.invoke('shell:open-external', url),
    setWindowChrome: () => Promise.resolve(false)
  }
} satisfies AnalytixApi

contextBridge.exposeInMainWorld('analytix', api)
