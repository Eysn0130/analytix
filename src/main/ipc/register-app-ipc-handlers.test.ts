import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createHash, randomUUID } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { app, BrowserWindow, dialog } from 'electron'
import {
  mergeScheduleSettings,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsPatch,
  type AppSettingsV1
} from '../../shared/app-settings'
import {
  analytixApprovalPath,
  analytixAttachmentContentPath,
  analytixMemoryRecordPath,
  analytixSessionResumePath,
  analytixThreadCheckpointRewindPlanPath,
  analytixThreadRewindPath,
  analytixThreadSteerPath,
  analytixUserInputPath
} from '../../shared/analytix-endpoints'
import { canonicalPath } from '../services/workspace-paths'
import { configureLogger, logError } from '../logger'
import { registerAppIpcHandlers } from './register-app-ipc-handlers'

const handlers = new Map<string, (event: unknown, payload?: unknown) => Promise<unknown>>()

const gitCheckpointMock = vi.hoisted(() => ({
  createGitCheckpoint: vi.fn(),
  restoreGitCheckpoint: vi.fn()
}))

const threadHandoffServiceMock = vi.hoisted(() => ({
  threadHandoffService: {
    subscribe: vi.fn(() => vi.fn()),
    start: vi.fn(),
    retry: vi.fn(),
    get: vi.fn(),
    cancel: vi.fn(),
    remove: vi.fn(),
    completeSwitch: vi.fn(),
    failSwitch: vi.fn()
  }
}))

const writeExportServiceMock = vi.hoisted(() => ({
  exportWriteDocument: vi.fn(),
  copyWriteDocumentAsRichText: vi.fn()
}))

const writeReadMock = vi.hoisted(() => ({
  requestWriteInlineCompletion: vi.fn(),
  retrieveWriteContext: vi.fn()
}))
vi.mock('../services/write-inline-completion-service', async (original) => ({
  ...await original<typeof import('../services/write-inline-completion-service')>(),
  requestWriteInlineCompletion: writeReadMock.requestWriteInlineCompletion
}))
vi.mock('../services/write-retrieval-service', async (original) => ({
  ...await original<typeof import('../services/write-retrieval-service')>(),
  retrieveWriteContext: writeReadMock.retrieveWriteContext
}))

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => tmpdir()),
    on: vi.fn(),
    quit: vi.fn()
  },
  BrowserWindow: {
    getAllWindows: vi.fn(() => [])
  },
  dialog: {
    showOpenDialog: vi.fn(),
    showMessageBox: vi.fn()
  },
  shell: {},
  ipcMain: {
    handle: vi.fn((channel: string, handler: (event: unknown, payload?: unknown) => Promise<unknown>) => {
      handlers.set(channel, handler)
    })
  }
}))

vi.mock('../services/git-checkpoint-service', () => gitCheckpointMock)
vi.mock('../services/thread-handoff-service', () => threadHandoffServiceMock)
vi.mock('../services/write-export-service', () => writeExportServiceMock)

function settings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

function importStageResponseForMainTestV1(selector: string) {
  return {
    ok: true as const,
    status: 200,
    body: JSON.stringify({
      status: 'ready',
      totalRowCount: 1,
      items: [{
        selector,
        sourceIndex: 1,
        sourceCount: 1,
        sourceLabel: 'Source 1 of 1',
        rowCount: 1,
        columnCount: 34,
        status: 'ready'
      }]
    })
  }
}

function cleaningCommitResponseForMainTestV1(selector: string) {
  return {
    ok: true as const,
    status: 200,
    body: JSON.stringify({
      status: 'committed',
      selector,
      ruleGeneration: `tlgen1_${'1'.repeat(64)}`,
      ruleDigest: '2'.repeat(64),
      inputSnapshot: `tlsnap1_${'3'.repeat(64)}`,
      outputSnapshot: `tlsnap1_${'4'.repeat(64)}`,
      transformLineage: `tllin1_${'5'.repeat(64)}`,
      rowCount: 2,
      changedRowCount: 1
    })
  }
}

function registerOptions(overrides: Partial<Parameters<typeof import('./register-app-ipc-handlers').registerAppIpcHandlers>[0]> = {}) {
  const applySettingsPatch = vi.fn(async () => settings())
  const saveSettingsPatch = vi.fn(async () => settings())
  const hubAccountSnapshot = {
    authenticated: false,
    gatewayConfigured: false,
    accountReady: false,
    source: 'none' as const
  }
  const hubAccountService = {
    getSnapshot: vi.fn(async () => hubAccountSnapshot),
    refresh: vi.fn(async () => hubAccountSnapshot),
    login: vi.fn(async () => hubAccountSnapshot),
    register: vi.fn(async () => hubAccountSnapshot),
    logout: vi.fn(async () => hubAccountSnapshot),
    sendRegisterVerificationCode: vi.fn(async () => ({
      ok: true,
      expiresAt: '2026-01-01T00:00:00.000Z',
      retryAfterSeconds: 60
    })),
    sendPasswordResetVerificationCode: vi.fn(async () => ({
      ok: true,
      expiresAt: '2026-01-01T00:00:00.000Z',
      retryAfterSeconds: 60
    })),
    confirmPasswordReset: vi.fn(async () => ({ ok: true })),
    fetchChallenge: vi.fn(async () => ({
      ok: true,
      challenge: {
        challengeId: 'challenge-1',
        prompt: { primary: 'Select tiles', secondary: '' },
        tiles: [],
        expiresAt: '2026-01-01T00:00:00.000Z'
      }
    })),
    fetchChallengeState: vi.fn(async () => ({
      ok: true,
      state: { challengeRequired: false, failedAttempts: 0, threshold: 3 }
    })),
    verifyChallenge: vi.fn(async () => ({
      ok: true,
      verification: {
        ok: true,
        proofToken: 'proof',
        expiresAt: '2026-01-01T00:00:00.000Z',
        remainingUses: 1
      }
    })),
    usage: vi.fn(async () => ({
      ok: true,
      user: {
        id: 'user-1',
        email: 'user@example.test',
        fullName: 'User',
        plan: 'normal' as const,
        accountReady: true
      },
      gatewayUsageSummary: {
        requestCount30d: 0,
        completedCount30d: 0,
        issueCount30d: 0,
        rawTokensToday: 0,
        billableTokensToday: 0,
        rawTokens30d: 0,
        billableTokens30d: 0,
        cachedPromptTokens30d: 0,
        qwenBillableTokens30d: 0,
        deepseekBillableTokens30d: 0,
        mimoBillableTokens30d: 0
      },
      gatewayQuotaWindows: []
    })),
    referral: vi.fn(async () => ({
      ok: true,
      referral: {
        code: 'ABC123',
        shareUrl: 'https://analytix.top/register?ref=ABC123',
        rewardTokenGrant: 0,
        pendingCount: 0,
        completedCount: 0,
        rewardedTokenTotal: 0,
        referrals: []
      }
    })),
    models: vi.fn(async () => ({ ok: true, models: [], defaultModelId: undefined }))
  }
  return {
    store: { load: vi.fn(async () => settings()) } as never,
    loadHubAccountService: vi.fn(async () => hubAccountService as never),
    getMainWindow: () => null,
    canNavigateWindow: () => true,
    applySettingsPatch,
    saveSettingsPatch,
    runtimeRequest: vi.fn() as never,
    localDisplayRequest: vi.fn() as never,
    restartRuntime: vi.fn(async () => undefined),
    fetchUpstreamModels: vi.fn() as never,
    getClawRuntime: () => null,
    getScheduleRuntime: () => null,
    startFeishuInstallQrcode: vi.fn() as never,
    pollFeishuInstall: vi.fn() as never,
    startWeixinInstallQrcode: vi.fn() as never,
    pollWeixinInstall: vi.fn() as never,
    imChannelAccountLifecycle: {
      connect: vi.fn(async () => settings()),
      disconnect: vi.fn(async () => settings()),
      recover: vi.fn(async () => undefined)
    } as never,
    installedExtensionAccountLifecycle: {
      replace: vi.fn(), revoke: vi.fn(), delete: vi.fn(),
      reconcileManagedAccounts: vi.fn(async () => ({ deleted: 0, retained: 0 })),
      deleteVerifiedPluginAccounts: vi.fn(async () => 0)
    } as never,
    providerOAuthAccountManagement: {
      configureProvider: vi.fn(), beginProvider: vi.fn(), beginMcp: vi.fn(), beginExtension: vi.fn(),
      statusProvider: vi.fn(), cancelProvider: vi.fn(), statusMcp: vi.fn(), cancelMcp: vi.fn(),
      statusExtension: vi.fn(), cancelExtension: vi.fn(), revokeProvider: vi.fn(), deleteProvider: vi.fn(),
      replaceProviderSubscription: vi.fn(), revokeMcp: vi.fn(), deleteMcp: vi.fn(),
      revokeExtension: vi.fn(), deleteExtension: vi.fn(), handleNativeCallback: vi.fn()
    } as never,
    resolveAnalytixConfigPath: () => '/tmp/analytix.json',
    showTurnCompleteNotification: vi.fn() as never,
    openThreadInNewWindow: vi.fn(),
    getAppVersion: () => '0.1.0',
    readGuiUpdateState: vi.fn() as never,
    loadGuiUpdaterModule: vi.fn() as never,
    resolveLogDirectory: () => '/tmp/logs',
    logError: vi.fn(),
    ...overrides
  }
}

describe('registerAppIpcHandlers', () => {
  beforeEach(() => {
    writeReadMock.requestWriteInlineCompletion.mockReset()
    writeReadMock.retrieveWriteContext.mockReset()
    handlers.clear()
    gitCheckpointMock.createGitCheckpoint.mockReset()
    gitCheckpointMock.restoreGitCheckpoint.mockReset()
    threadHandoffServiceMock.threadHandoffService.subscribe.mockReset()
    threadHandoffServiceMock.threadHandoffService.subscribe.mockReturnValue(vi.fn())
    threadHandoffServiceMock.threadHandoffService.start.mockReset()
    threadHandoffServiceMock.threadHandoffService.retry.mockReset()
    threadHandoffServiceMock.threadHandoffService.get.mockReset()
    threadHandoffServiceMock.threadHandoffService.cancel.mockReset()
    threadHandoffServiceMock.threadHandoffService.remove.mockReset()
    threadHandoffServiceMock.threadHandoffService.completeSwitch.mockReset()
    threadHandoffServiceMock.threadHandoffService.failSwitch.mockReset()
    writeExportServiceMock.exportWriteDocument.mockReset()
    writeExportServiceMock.copyWriteDocumentAsRichText.mockReset()
    vi.mocked(BrowserWindow.getAllWindows).mockReturnValue([])
    vi.mocked(dialog.showOpenDialog).mockReset()
    vi.mocked(dialog.showMessageBox).mockReset()
  })

  it.each(['write:inline-completion', 'write:retrieve-context'])(
    'rejects untrusted %s before parsing or reading', async (channel) => {
      const frame = {}
      const sender = { mainFrame: frame, isDestroyed: () => false }
      const mainWindow = { webContents: sender, isDestroyed: () => false }
      const options = registerOptions({ getMainWindow: () => mainWindow as never })
      registerAppIpcHandlers(options)
      for (const event of [
        { sender: { ...sender }, senderFrame: frame },
        { sender, senderFrame: {} },
        { sender, senderFrame: null }
      ]) {
        await expect(handlers.get(channel)?.(event, { query: 'synthetic' })).resolves.toMatchObject({ ok: false })
      }
      expect(writeReadMock.requestWriteInlineCompletion).not.toHaveBeenCalled()
      expect(writeReadMock.retrieveWriteContext).not.toHaveBeenCalled()
    }
  )

  it('admits current retrieval but suppresses results after its main frame is replaced', async () => {
    const frame = {}
    const sender = Object.assign(new EventEmitter(), { mainFrame: frame, isDestroyed: () => false })
    const options = registerOptions({ localDisplayRequest: vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot: { threadId: 'thread-1', workspace: '/workspace', binding: 'a'.repeat(64) } }) })), getMainWindow: () => ({ webContents: sender, isDestroyed: () => false }) as never })
    registerAppIpcHandlers(options)
    const context = { source: 'bm25-keyword', snippets: [{ text: 'synthetic-only' }] }
    writeReadMock.retrieveWriteContext.mockResolvedValueOnce(context)
    await expect(handlers.get('write:retrieve-context')?.({ sender, senderFrame: frame }, { query: 'synthetic', threadId: 'thread-1', workspaceRoot: '/workspace' }))
      .resolves.toEqual({ ok: true, context })
    writeReadMock.retrieveWriteContext.mockImplementationOnce(async (_request, options) => {
      expect(options.isCurrent()).toBe(true)
      sender.mainFrame = {}
      expect(options.isCurrent()).toBe(false)
      return context
    })
    await expect(handlers.get('write:retrieve-context')?.({ sender, senderFrame: frame }, { query: 'synthetic', threadId: 'thread-1', workspaceRoot: '/workspace' }))
      .resolves.toMatchObject({ ok: false })
    expect(writeReadMock.retrieveWriteContext).toHaveBeenCalledTimes(2)
    expect(writeReadMock.retrieveWriteContext.mock.calls[1]?.[1]).toEqual({ isCurrent: expect.any(Function), source: expect.objectContaining({ key: expect.any(String) }) })
    expect(sender.eventNames()).toEqual([])
  })

  it('invalidates a retrieval across same-frame same-URL navigation and releases listeners', async () => {
    const frame = {}
    const sender = Object.assign(new EventEmitter(), { mainFrame: frame, isDestroyed: () => false })
    registerAppIpcHandlers(registerOptions({ localDisplayRequest: vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot: { threadId: 'thread-1', workspace: '/workspace', binding: 'a'.repeat(64) } }) })), getMainWindow: () => ({ webContents: sender, isDestroyed: () => false }) as never }))
    let requestCurrent: (() => boolean) | undefined
    writeReadMock.retrieveWriteContext.mockImplementationOnce(async (_request, options) => {
      requestCurrent = options.isCurrent
      sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
      return { snippets: [{ text: 'OLD_DOCUMENT' }] }
    })
    const result = await handlers.get('write:retrieve-context')?.({ sender, senderFrame: frame }, { query: 'synthetic', threadId: 'thread-1', workspaceRoot: '/workspace' })
    expect(result).toMatchObject({ ok: false })
    expect(requestCurrent?.()).toBe(false)
    expect(sender.eventNames()).toEqual([])
  })

  it('does not load Hub compatibility during registration or ordinary IPC and loads it only on explicit Hub invocation', async () => {
    const options = registerOptions()

    registerAppIpcHandlers(options)

    expect(options.loadHubAccountService).not.toHaveBeenCalled()
    expect(handlers.has('provider:probe')).toBe(false)
    await handlers.get('settings:get')?.({})
    await handlers.get('provider-registry:request')?.({}, { schemaVersion: 1, operation: 'list' })
    expect(options.loadHubAccountService).not.toHaveBeenCalled()

    await handlers.get('hub-account:snapshot')?.({})
    expect(options.loadHubAccountService).toHaveBeenCalledTimes(1)
  })

  it('registers protected recovery as four explicit no-payload Main actions', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    registerAppIpcHandlers(registerOptions())

    for (const channel of [
      'provider-credential-recovery:create-destination-request',
      'provider-credential-recovery:create-source-bundle',
      'provider-credential-recovery:apply-destination-bundle',
      'provider-credential-recovery:finalize-source-receipt'
    ]) {
      const handler = handlers.get(channel)
      expect(handler).toBeTypeOf('function')
      await expect(handler?.({}, { path: '/private/credential' })).resolves.toEqual({
        ok: false,
        code: 'invalid_request'
      })
    }
    expect(handlers.has('runtime:request')).toBe(true)
  })

  it('routes Provider OAuth begin through the current Main window without exposing the authorization URL', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const mainFrame = {}
    const sender = { id: 41, mainFrame, isDestroyed: () => false }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    const beginProvider = vi.fn(async () => ({
      ok: true as const,
      authorizationId: 'oauth_public_opaque',
      expiresAt: '2026-08-30T10:10:00.000Z',
      phase: 'pending' as const
    }))

    registerAppIpcHandlers(registerOptions({
      getMainWindow: () => mainWindow as never,
      providerOAuthAccountManagement: { beginProvider } as never
    } as never))

    const handler = handlers.get('provider-oauth:begin')
    expect(handler).toBeTypeOf('function')
    const result = await handler?.(
      { sender, senderFrame: mainFrame },
      { providerId: 'provider-a' }
    )
    expect(beginProvider).toHaveBeenCalledWith({ providerId: 'provider-a' }, {
      webContentsId: 41
    })
    expect(result).toEqual({
      ok: true,
      authorizationId: 'oauth_public_opaque',
      expiresAt: '2026-08-30T10:10:00.000Z',
      phase: 'pending'
    })
    expect(JSON.stringify(result)).not.toMatch(/authorizationUrl|state|nonce|verifier|token|credentialRef/i)
  })

  it('rejects invalid settings patches at the handler boundary', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const applySettingsPatch = vi.fn(async () => settings())

    registerAppIpcHandlers(registerOptions({ applySettingsPatch }))

    const handler = handlers.get('settings:set')
    expect(handler).toBeTypeOf('function')
    await expect(
      handler?.({}, { runtime: { mysteryFlag: true } })
    ).rejects.toThrow(/Invalid payload for settings:set/)
    const canary = ['ipc', 'settings', 'credential'].join('-')
    await expect(handler?.({}, {
      provider: {
        apiKey: canary,
        providers: [{ id: 'deepseek', apiKey: canary }]
      }
    })).rejects.toThrow(/Invalid payload for settings:set/)
    expect(applySettingsPatch).not.toHaveBeenCalled()
  })

  it('projects untrusted renderer logs to fixed managed-log metadata only', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const logDir = mkdtempSync(join(tmpdir(), 'analytix-renderer-ipc-log-'))
    const canaries = [
      'NEUTRAL_EXACT_CELL_CANARY_7F3C',
      'NEUTRAL_RENDERER_CATEGORY_CANARY',
      '\\\\server\\share\\case.csv'
    ]
    configureLogger({ dir: logDir, enabled: true, retentionDays: 2 })
    try {
      registerAppIpcHandlers(registerOptions({ logError }))
      const handler = handlers.get('log:error')
      expect(handler).toBeTypeOf('function')
      await handler?.({}, {
        category: canaries[1],
        message: canaries[0],
        detail: {
          message: canaries[0],
          path: canaries[2],
          resultCount: 2,
          retryable: false
        }
      })
      await handler?.({}, {
        category: 'send-message',
        message: canaries[0],
        detail: { resultCount: 3, retryable: true }
      })

      await vi.waitFor(() => {
        const content = readdirSync(logDir)
          .map((entry) => readFileSync(join(logDir, entry), 'utf8'))
          .join('\n')
        for (const canary of canaries) expect(content).not.toContain(canary)
        expect(content).toContain('[renderer] event=renderer_diagnostic_failed')
        expect(content).toContain('renderer_diagnostic_failed')
        expect(content).toContain('renderer_send_message_failed')
        expect(content).toContain('"resultCount":2')
        expect(content).toContain('"retryable":false')
      })
    } finally {
      configureLogger({ dir: '', enabled: false })
      rmSync(logDir, { recursive: true, force: true })
    }
  })

  it('projects renderer-controlled thread traces before the durable IPC result', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const {
      flushThreadTraceEventsForTests,
      resetThreadTraceWriterForTests
    } = await import('../services/thread-trace-service')
    const userData = mkdtempSync(join(tmpdir(), 'analytix-thread-trace-ipc-'))
    const previousEnv = process.env.ANALYTIX_THREAD_TRACE
    const hostileThreadId =
      'HOSTILE_THREAD_TRACE_IPC_CANARY_/Users/private/synthetic/case-42_PII_ACCOUNT-622202'
    const hostileDataKey =
      'private_path_Users_private_synthetic_case_42_PII_ACCOUNT_622202'

    process.env.ANALYTIX_THREAD_TRACE = '1'
    vi.mocked(app.getPath).mockReturnValue(userData)
    try {
      registerAppIpcHandlers(registerOptions())
      const result = await handlers.get('diagnostics:thread-trace')?.({}, {
        name: 'thread.delta.buffered',
        timestamp: 201,
        threadId: hostileThreadId,
        data: {
          deltas: 3,
          renderer_delta_buffered_at: 202,
          [hostileDataKey]: true
        }
      })
      await flushThreadTraceEventsForTests(userData)

      expect(result).toEqual({
        ok: true,
        path: expect.stringMatching(/^traces\/thread-ref-[a-f0-9]{64}\.jsonl$/)
      })
      const serializedResult = JSON.stringify(result)
      expect(serializedResult).not.toContain(userData)
      expect(serializedResult).not.toContain(hostileThreadId)

      const traceFiles = readdirSync(join(userData, 'traces'))
      expect(traceFiles).toHaveLength(1)
      const body = readFileSync(join(userData, 'traces', traceFiles[0]), 'utf8')
      expect(body).not.toContain(hostileThreadId)
      expect(body).not.toContain(hostileDataKey)
      expect(body).not.toContain('PII_ACCOUNT-622202')
      expect(JSON.parse(body.trim())).toEqual({
        name: 'thread.delta.buffered',
        timestamp: 201,
        threadId: expect.stringMatching(/^ref-[a-f0-9]{64}$/),
        data: { deltas: 3, renderer_delta_buffered_at: 202 }
      })
    } finally {
      resetThreadTraceWriterForTests()
      vi.mocked(app.getPath).mockReturnValue(tmpdir())
      rmSync(userData, { recursive: true, force: true })
      if (previousEnv === undefined) {
        delete process.env.ANALYTIX_THREAD_TRACE
      } else {
        process.env.ANALYTIX_THREAD_TRACE = previousEnv
      }
    }
  })

  it('derives write export workspace authority in Main and rejects non-current frames', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const workspace = await canonicalPath(mkdtempSync(join(tmpdir(), 'analytix-write-export-main-')))
    const currentSettings = settings()
    currentSettings.write = {
      ...currentSettings.write,
      defaultWorkspaceRoot: workspace,
      activeWorkspaceRoot: '/tmp/legacy-unrelated-write',
      workspaces: [workspace]
    }
    const store = { load: vi.fn(async () => currentSettings) }
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    writeExportServiceMock.exportWriteDocument.mockResolvedValue({
      ok: false,
      canceled: true
    })
    const binding = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'current-main',
      baseRevision: 'c'.repeat(64), draftVersion: 'd'.repeat(48) }
    const snapshot = { ...binding, workspace, path: join(workspace, 'draft.md'), content: '# Draft',
      contentDigest: createHash('sha256').update('# Draft').digest('hex') }
    const localDisplayRequest = vi.fn(async (_path: string, _body: string) => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot }) }))
    registerAppIpcHandlers(registerOptions({ store: store as never, getMainWindow: () => mainWindow as never, localDisplayRequest }))
    const payload = { ...binding, format: 'docx' }
    await expect(handlers.get('write:export')?.({ sender, senderFrame: {} }, payload)).resolves.toEqual({
      ok: false,
      canceled: false,
      message: 'Write export requires the current main window.'
    })
    expect(writeExportServiceMock.exportWriteDocument).not.toHaveBeenCalled()

    await expect(handlers.get('write:export')?.({ sender, senderFrame: mainFrame }, payload)).resolves.toEqual({
      ok: false,
      canceled: true
    })
    expect(writeExportServiceMock.exportWriteDocument).toHaveBeenCalledWith(
      { path: snapshot.path, content: snapshot.content, format: 'docx', typography: undefined },
      expect.objectContaining({
        parentWindow: mainWindow,
        workspaceRoot: workspace,
        authorityCurrent: expect.any(Function)
      })
    )
    const options = writeExportServiceMock.exportWriteDocument.mock.calls[0]?.[1] as {
      authorityCurrent: () => Promise<boolean>
    }
    await expect(options.authorityCurrent()).resolves.toBe(true)
    expect(localDisplayRequest.mock.calls.at(-1)?.[0]).toBe('/v1/local-display/object-editing')
    expect(JSON.parse(localDisplayRequest.mock.calls.at(-1)![1])).toEqual({ action: 'export-snapshot', ...binding })
    localDisplayRequest.mockResolvedValueOnce({ ok: false, status: 409, body: '{"ok":false}' })
    await expect(options.authorityCurrent()).resolves.toBe(false)
    rmSync(workspace, { recursive: true, force: true })
  })

  it('rejects a renderer-supplied write export workspace locator', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({ getMainWindow: () => mainWindow as never }))

    await expect(handlers.get('write:export')?.({ sender, senderFrame: mainFrame }, {
      path: '/tmp/workspace/draft.md',
      workspaceRoot: '/tmp/renderer-forged-workspace',
      format: 'docx',
      content: '# Draft'
    })).rejects.toThrow(/Invalid payload for write:export/)
    expect(writeExportServiceMock.exportWriteDocument).not.toHaveBeenCalled()
  })

  it('derives rich clipboard workspace authority in Main and rejects non-current frames', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const workspace = await canonicalPath(mkdtempSync(join(tmpdir(), 'analytix-write-clipboard-main-')))
    const currentSettings = settings()
    currentSettings.write = {
      ...currentSettings.write,
      defaultWorkspaceRoot: workspace,
      activeWorkspaceRoot: '/tmp/legacy-unrelated-write',
      workspaces: [workspace]
    }
    const store = { load: vi.fn(async () => currentSettings) }
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    writeExportServiceMock.copyWriteDocumentAsRichText.mockResolvedValue({
      ok: true,
      copiedAt: '2026-08-26T04:00:00.000Z'
    })
    const binding = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'current-main',
      baseRevision: 'c'.repeat(64), draftVersion: 'd'.repeat(48) }
    const snapshot = { ...binding, workspace, path: join(workspace, 'draft.md'), content: '# Draft',
      contentDigest: createHash('sha256').update('# Draft').digest('hex') }
    const localDisplayRequest = vi.fn(async (_path: string, _body: string) => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot }) }))
    registerAppIpcHandlers(registerOptions({ store: store as never, getMainWindow: () => mainWindow as never, localDisplayRequest }))
    const payload = binding
    await expect(handlers.get('write:copy-rich-text')?.({ sender, senderFrame: {} }, payload)).resolves.toEqual({
      ok: false,
      message: 'Write export requires the current main window.'
    })
    expect(writeExportServiceMock.copyWriteDocumentAsRichText).not.toHaveBeenCalled()

    await expect(handlers.get('write:copy-rich-text')?.({ sender, senderFrame: mainFrame }, payload)).resolves.toEqual({
      ok: true,
      copiedAt: '2026-08-26T04:00:00.000Z'
    })
    expect(writeExportServiceMock.copyWriteDocumentAsRichText).toHaveBeenCalledWith(
      { path: snapshot.path, content: snapshot.content },
      expect.objectContaining({
        workspaceRoot: workspace,
        authorityCurrent: expect.any(Function)
      })
    )
    const options = writeExportServiceMock.copyWriteDocumentAsRichText.mock.calls[0]?.[1] as {
      authorityCurrent: () => Promise<boolean>
    }
    await expect(options.authorityCurrent()).resolves.toBe(true)
    expect(localDisplayRequest.mock.calls.at(-1)?.[0]).toBe('/v1/local-display/object-editing')
    expect(JSON.parse(localDisplayRequest.mock.calls.at(-1)![1])).toEqual({ action: 'export-snapshot', ...binding })
    localDisplayRequest.mockResolvedValueOnce({ ok: false, status: 409, body: '{"ok":false}' })
    await expect(options.authorityCurrent()).resolves.toBe(false)
    rmSync(workspace, { recursive: true, force: true })
  })

  it('passes valid settings patches through to applySettingsPatch', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const applySettingsPatch = vi.fn(async () => settings())

    registerAppIpcHandlers(registerOptions({ applySettingsPatch }))

    const payload = {
      theme: 'dark' as const,
      runtime: {
          port: 9000
        }
    }
    const handler = handlers.get('settings:set')
    await expect(handler?.({}, payload)).resolves.toEqual(settings())
    expect(applySettingsPatch).toHaveBeenCalledWith(payload)
  })

  it('broadcasts query cache invalidations to other windows', async () => {
    const sendA = vi.fn()
    const sendB = vi.fn()
    vi.mocked(BrowserWindow.getAllWindows).mockReturnValue([
      {
        isDestroyed: () => false,
        webContents: {
          id: 1,
          isDestroyed: () => false,
          send: sendA
        }
      },
      {
        isDestroyed: () => false,
        webContents: {
          id: 2,
          isDestroyed: () => false,
          send: sendB
        }
      }
    ] as never)
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')

    registerAppIpcHandlers(registerOptions())

    await expect(handlers.get('query-cache:invalidate')?.(
      { sender: { id: 1 } },
      { queryKey: ['settings'], sourceClientId: 'renderer-a' }
    )).resolves.toBe(true)

    expect(sendA).not.toHaveBeenCalled()
    expect(sendB).toHaveBeenCalledWith('query-cache:invalidate', {
      queryKey: ['settings'],
      sourceClientId: 'renderer-a'
    })
  })

  it('restarts the managed runtime through the restart IPC handler', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const restartRuntime = vi.fn(async () => undefined)

    registerAppIpcHandlers(registerOptions({ restartRuntime }))

    await expect(handlers.get('runtime:restart')?.({})).resolves.toBeUndefined()
    expect(restartRuntime).toHaveBeenCalledTimes(1)
  })

  it('projects hostile runtime restart failures to the closed public runtime error', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const hostileFields = [
      'HOSTILE_SETUP_ERROR',
      '/private/owner/case-42/runtime.json',
      'pii=110101199001011234',
      'model=private-model',
      'pid=424242',
      'timestamp=2026-08-26T12:34:56.789Z',
      'secret=restart-secret-value'
    ]
    const restartRuntime = vi.fn(async () => {
      throw new Error(hostileFields.join(' '))
    })

    registerAppIpcHandlers(registerOptions({ restartRuntime }))

    const rejection = await handlers.get('runtime:restart')?.({}).then(
      () => null,
      (error: unknown) => error
    )
    expect(rejection).toBeInstanceOf(Error)
    const serialized = Buffer.from((rejection as Error).message, 'utf8').toString('utf8')
    expect(serialized).toBe(JSON.stringify({
      code: 'runtime_unavailable',
      message: 'The Analytix runtime is unavailable.'
    }))
    expect(JSON.parse(serialized)).toEqual({
      code: 'runtime_unavailable',
      message: 'The Analytix runtime is unavailable.'
    })
    for (const hostileField of hostileFields) {
      expect(serialized).not.toContain(hostileField)
    }
    expect(restartRuntime).toHaveBeenCalledTimes(1)
  })

  it('rejects forbidden runtime request routes at the IPC handler boundary', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()

    registerAppIpcHandlers(registerOptions({ runtimeRequest: runtimeRequest as never }))

    await expect(
      handlers.get('runtime:request')?.({}, {
        path: '/v1/runtime/go/threads',
        method: 'GET'
      })
    ).rejects.toThrow(/Invalid payload for runtime:request/)
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('uses fixed typed-local-display routes and rejects non-public response fields', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()
    const localDisplayRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        kind: 'direct_source_preview',
        caseId: 'case-1',
        datasetSnapshotId: 'dsv2_' + 'a'.repeat(64),
        displayMode: 'full',
        view: 'transactions',
        fields: ['account', 'amountText'],
        rowOffset: 0,
        rowLimit: 25,
        hasMore: false,
        rows: [{
          rowIndex: 0,
          cells: [
            { field: 'account', displayValue: 'LOCAL_ACCOUNT_VALUE' },
            { field: 'amountText', displayValue: '123.45' }
          ]
        }]
      })
    }))

    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      runtimeRequest: runtimeRequest as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    } as never))
    const handler = handlers.get('runtime:direct-source-preview')
    const request = {
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: ['account', 'amountText'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    }

    await expect(handler?.(event, request)).resolves.toMatchObject({
      schemaVersion: 1,
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: ['account', 'amountText'],
      displayMode: 'full'
    })
    expect(localDisplayRequest).toHaveBeenCalledWith(
      '/v1/local-display/direct-source-preview',
      JSON.stringify({ workspaceRoot: await canonicalPath('/tmp/workspace'), ...request })
    )
    expect(runtimeRequest).not.toHaveBeenCalled()

    await expect(handler?.(event, {
      ...request,
      workspaceRoot: '/tmp/forged-case'
    })).rejects.toThrow(/Invalid payload for \/v1\/local-display\/direct-source-preview/)
    expect(localDisplayRequest).toHaveBeenCalledTimes(1)

    localDisplayRequest.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        kind: 'direct_source_preview',
        caseId: 'case-1',
        datasetSnapshotId: 'dsv2_' + 'a'.repeat(64),
        displayMode: 'full',
        view: 'transactions',
        fields: ['account', 'amountText'],
        rowOffset: 0,
        rowLimit: 25,
        hasMore: false,
        rows: [{
          rowIndex: 0,
          cells: [
            { field: 'account', displayValue: 'LOCAL_ACCOUNT_VALUE', claimIds: ['claim-forbidden'] },
            { field: 'amountText', displayValue: '123.45' }
          ]
        }]
      })
    })

    await expect(handler?.(event, request)).resolves.toEqual({
      ok: false,
      status: 502,
      code: 'invalid_response',
      message: 'Local display response was invalid.'
    })

    localDisplayRequest.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        kind: 'direct_source_preview',
        caseId: 'case-1',
        datasetSnapshotId: 'dsv2_' + 'a'.repeat(64),
        displayMode: 'full',
        view: 'transactions',
        fields: ['account', 'amountText'],
        rowOffset: 0,
        rowLimit: 25,
        hasMore: false,
        entityRef: 'cer1_' + 'a'.repeat(64),
        rows: []
      })
    })

    await expect(handler?.(event, request)).resolves.toEqual({
      ok: false,
      status: 502,
      code: 'invalid_response',
      message: 'Local display response was invalid.'
    })
  })

  it('routes synthetic import and cleaning previews only through closed typed-local IPC sinks', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-preview-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const importSelector = `tlsel1_${'a'.repeat(64)}`
    const cleaningSelector = `tlsel1_${'b'.repeat(64)}`
    const importResponse = {
      schemaVersion: 1,
      kind: 'import_mapping_preview',
      selector: importSelector,
      lineage: {
        importGeneration: `tlgen1_${'1'.repeat(64)}`,
        sourceItemGeneration: `tlgen1_${'2'.repeat(64)}`,
        parserGeneration: `tlgen1_${'3'.repeat(64)}`,
        mappingGeneration: `tlgen1_${'4'.repeat(64)}`
      },
      displayMode: 'full',
      fields: ['sourceColumn', 'sampleValue'],
      rowOffset: 0,
      rowLimit: 25,
      hasMore: false,
      rows: [{
        rowIndex: 0,
        parseStatus: 'parsed',
        mappingStatus: 'mapped',
        cells: [
          { field: 'sourceColumn', displayValue: '交易账号' },
          { field: 'sampleValue', displayValue: 'IMPORT_SOURCE_EXACT_CANARY' }
        ]
      }]
    }
    const cleaningResponse = {
      schemaVersion: 1,
      kind: 'cleaning_diff_preview',
      selector: cleaningSelector,
      lineage: {
        inputSnapshot: `tlsnap1_${'5'.repeat(64)}`,
        ruleGeneration: `tlgen1_${'6'.repeat(64)}`,
        ruleDigest: '7'.repeat(64),
        outputSnapshot: `tlsnap1_${'8'.repeat(64)}`,
        transformLineage: `tllin1_${'9'.repeat(64)}`
      },
      displayMode: 'masked',
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 25,
      hasMore: false,
      rows: [{
        rowIndex: 0,
        status: 'changed',
        cells: [{ field: 'account', beforeDisplayValue: '****1234', afterDisplayValue: '****5678' }]
      }]
    }
    const localDisplayRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/stage')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            status: 'ready',
            totalRowCount: 1,
            items: [{
              selector: importSelector,
              sourceIndex: 1,
              sourceCount: 1,
              sourceLabel: 'Source 1 of 1',
              rowCount: 1,
              columnCount: 2,
              status: 'ready'
            }]
          })
        }
      }
      if (path.endsWith('/funds-cleaning/run')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            status: 'committed',
            selector: cleaningSelector,
            ruleGeneration: cleaningResponse.lineage.ruleGeneration,
            ruleDigest: cleaningResponse.lineage.ruleDigest,
            inputSnapshot: cleaningResponse.lineage.inputSnapshot,
            outputSnapshot: cleaningResponse.lineage.outputSnapshot,
            transformLineage: cleaningResponse.lineage.transformLineage,
            rowCount: 1,
            changedRowCount: 1
          })
        }
      }
      if (path.endsWith('/funds-cleaning/revoke')) {
        return { ok: true, status: 200, body: JSON.stringify({ revoked: true }) }
      }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify(path.includes('import-mapping') ? importResponse : cleaningResponse)
      }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    const restartRuntime = vi.fn(async () => undefined)
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      runtimeRequest: runtimeRequest as never,
      localDisplayRequest,
      restartRuntime,
      getMainWindow: () => mainWindow as never
    } as never))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({
      ok: true,
      items: [{ selector: importSelector }]
    })
    await expect(handlers.get('runtime:run-deterministic-funds-cleaning')?.(event, {
      workspaceRoot: '/renderer/forged',
      caseId: 'renderer-forged'
    })).resolves.toMatchObject({
      ok: true,
      status: 'committed',
      selector: cleaningSelector
    })

    const importRequest = {
      kind: 'import_mapping_preview' as const,
      selector: importSelector,
      fields: ['sourceColumn', 'sampleValue'] as const,
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full' as const
    }
    const cleaningRequest = {
      kind: 'cleaning_diff_preview' as const,
      selector: cleaningSelector,
      fields: ['account'] as const,
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'masked' as const
    }
    await expect(handlers.get('runtime:import-mapping-preview')?.(event, importRequest)).resolves.toEqual(importResponse)
    await expect(handlers.get('runtime:cleaning-diff-preview')?.(event, cleaningRequest)).resolves.toEqual(cleaningResponse)
    expect(localDisplayRequest).toHaveBeenNthCalledWith(
      2, '/v1/local-display/funds-cleaning/run', JSON.stringify({ workspaceRoot: workspace })
    )
    expect(localDisplayRequest).toHaveBeenNthCalledWith(
      3, '/v1/local-display/import-mapping-preview', JSON.stringify(importRequest)
    )
    expect(localDisplayRequest).toHaveBeenNthCalledWith(
      4, '/v1/local-display/cleaning-diff-preview', JSON.stringify(cleaningRequest)
    )
    expect(runtimeRequest).not.toHaveBeenCalled()

    localDisplayRequest.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({
        ...cleaningResponse,
        lineage: { ...cleaningResponse.lineage, outputSnapshot: `tlsnap1_${'f'.repeat(64)}` },
        rows: [{
          rowIndex: 0,
          status: 'changed',
          cells: [{ field: 'account', beforeDisplayValue: 'WRONG_LINEAGE_EXACT', afterDisplayValue: 'WRONG' }]
        }]
      })
    })
    const wrongLineage = await handlers.get('runtime:cleaning-diff-preview')?.(event, cleaningRequest)
    expect(wrongLineage).toMatchObject({ ok: false, status: 502, code: 'invalid_response' })
    expect(JSON.stringify(wrongLineage)).not.toContain('WRONG_LINEAGE_EXACT')

    await expect(handlers.get('runtime:import-mapping-preview')?.(event, {
      ...importRequest,
      path: '/private/source.csv'
    })).rejects.toThrow(/Invalid payload for \/v1\/local-display\/import-mapping-preview/)

    localDisplayRequest.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({ ...importResponse, lineage: undefined })
    })
    await expect(handlers.get('runtime:import-mapping-preview')?.(event, importRequest)).resolves.toMatchObject({
      ok: false,
      status: 502,
      code: 'invalid_response'
    })

    localDisplayRequest.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({ ...importResponse, selector: `tlsel1_${'f'.repeat(64)}` })
    })
    await expect(handlers.get('runtime:import-mapping-preview')?.(event, importRequest)).resolves.toMatchObject({
      ok: false,
      status: 502,
      code: 'invalid_response'
    })

    await handlers.get('runtime:restart')?.(event)
    expect(restartRuntime).toHaveBeenCalledTimes(1)
    await expect(handlers.get('runtime:cleaning-diff-preview')?.(event, cleaningRequest)).resolves.toMatchObject({
      ok: false,
      status: 400,
      code: 'invalid_request'
    })
    rmSync(root, { recursive: true, force: true })
  })

  it('returns outcome_unknown after a dispatched cleaning response is lost or malformed', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-cleaning-unknown-'))
    const workspace = await canonicalPath(root)
    const localDisplayRequest = vi.fn()
      .mockRejectedValueOnce(new Error('synthetic response loss'))
      .mockResolvedValueOnce({ ok: true, status: 200, body: '{malformed' })
      .mockResolvedValueOnce({ ok: false, status: 503, body: '{"code":"unavailable"}' })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        body: '{"status":"pre_cas_failed","path":"/private/case"}'
      })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    for (let attempt = 0; attempt < 4; attempt += 1) {
      const result = await handlers.get('runtime:run-deterministic-funds-cleaning')?.(event)
      expect(result).toMatchObject({
        ok: false,
        status: 'outcome_unknown',
        code: 'outcome_unknown'
      })
      expect(JSON.stringify(result)).not.toMatch(/source|path|account|raw/i)
    }
    expect(localDisplayRequest).toHaveBeenNthCalledWith(
      1, '/v1/local-display/funds-cleaning/run', JSON.stringify({ workspaceRoot: workspace })
    )
    rmSync(root, { recursive: true, force: true })
  })

  it('returns unavailable only for the closed proven pre-CAS response', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-cleaning-pre-cas-'))
    const workspace = await canonicalPath(root)
    const localDisplayRequest = vi.fn().mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: JSON.stringify({ status: 'pre_cas_failed' })
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    const result = await handlers.get('runtime:run-deterministic-funds-cleaning')?.(event)
    expect(result).toEqual({
      ok: false,
      status: 'unavailable',
      code: 'pre_cas_failed',
      message: 'Deterministic cleaning did not advance the current DSV2 snapshot.'
    })
    expect(JSON.stringify(result)).not.toMatch(/source|path|account|raw/i)
    rmSync(root, { recursive: true, force: true })
  })

  it('drops late and old cleaning completions, revokes their selectors, and keeps the newest owner', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-cleaning-generation-'))
    const workspace = await canonicalPath(root)
    const selectors = [
      `tlsel1_${'a'.repeat(64)}`,
      `tlsel1_${'b'.repeat(64)}`,
      `tlsel1_${'c'.repeat(64)}`
    ]
    const pendingRuns: Array<(value: { ok: true; status: number; body: string }) => void> = []
    const localDisplayRequest = vi.fn((path: string) => {
      if (path.endsWith('/funds-cleaning/run')) {
        return new Promise<{ ok: true; status: number; body: string }>((resolve) => pendingRuns.push(resolve))
      }
      if (path.endsWith('/funds-cleaning/revoke')) {
        return Promise.resolve({ ok: true as const, status: 200, body: JSON.stringify({ revoked: true }) })
      }
      return Promise.resolve({
        ok: true as const,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          kind: 'cleaning_diff_preview',
          selector: selectors[1],
          lineage: {
            inputSnapshot: `tlsnap1_${'3'.repeat(64)}`,
            ruleGeneration: `tlgen1_${'1'.repeat(64)}`,
            ruleDigest: '2'.repeat(64),
            outputSnapshot: `tlsnap1_${'4'.repeat(64)}`,
            transformLineage: `tllin1_${'5'.repeat(64)}`
          },
          displayMode: 'full',
          fields: ['account'],
          rowOffset: 0,
          rowLimit: 1,
          hasMore: false,
          rows: []
        })
      })
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    let mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest: localDisplayRequest as never,
      getMainWindow: () => mainWindow as never
    }))

    const older = handlers.get('runtime:run-deterministic-funds-cleaning')?.(event)
    await vi.waitFor(() => expect(pendingRuns).toHaveLength(1))
    const newer = handlers.get('runtime:run-deterministic-funds-cleaning')?.(event)
    await vi.waitFor(() => expect(pendingRuns).toHaveLength(2))
    pendingRuns[1](cleaningCommitResponseForMainTestV1(selectors[1]))
    await expect(newer).resolves.toMatchObject({ ok: true, selector: selectors[1] })
    pendingRuns[0](cleaningCommitResponseForMainTestV1(selectors[0]))
    await expect(older).resolves.toMatchObject({ ok: false, status: 'outcome_unknown' })

    await expect(handlers.get('runtime:cleaning-diff-preview')?.(event, {
      kind: 'cleaning_diff_preview',
      selector: selectors[1],
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 1,
      displayMode: 'full'
    })).resolves.toMatchObject({ kind: 'cleaning_diff_preview', selector: selectors[1] })

    const late = handlers.get('runtime:run-deterministic-funds-cleaning')?.(event)
    await vi.waitFor(() => expect(pendingRuns).toHaveLength(3))
    mainWindow = { isDestroyed: () => false, webContents: { ...sender, mainFrame: {} } }
    pendingRuns[2](cleaningCommitResponseForMainTestV1(selectors[2]))
    await expect(late).resolves.toMatchObject({ ok: false, status: 'outcome_unknown' })
    expect(localDisplayRequest.mock.calls.filter(([path]) => path.endsWith('/revoke'))).toHaveLength(3)
    rmSync(root, { recursive: true, force: true })
  })

  it('rejects non-main-frame and late local-display replies without returning exact bytes', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    let resolveRequest!: (value: { ok: boolean; status: number; body: string }) => void
    const localDisplayRequest = vi.fn(() => new Promise<{ ok: boolean; status: number; body: string }>((resolve) => {
      resolveRequest = resolve
    }))
    const mainFrame = {}
    let destroyed = false
    const sender = { mainFrame, isDestroyed: () => destroyed }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    } as never))
    const handler = handlers.get('runtime:direct-source-preview')
    const request = {
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    }

    await expect(handler?.({ sender, senderFrame: {} }, request)).resolves.toMatchObject({
      ok: false,
      status: 403,
      code: 'forbidden'
    })
    expect(localDisplayRequest).not.toHaveBeenCalled()

    const pending = handler?.({ sender, senderFrame: mainFrame }, request)
    await vi.waitFor(() => expect(localDisplayRequest).toHaveBeenCalledTimes(1))
    destroyed = true
    resolveRequest({
      ok: true,
      status: 200,
      body: JSON.stringify({ displayValue: 'LATE_SOURCE_EXACT_VALUE' })
    })
    await expect(pending).resolves.toEqual({
      ok: false,
      status: 403,
      code: 'forbidden',
      message: 'Local display request failed.'
    })
  })

  it('rejects a late source-exact reply after the main-owned workspace authority changes', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    let currentWorkspace = '/tmp/case-a'
    const store = {
      load: vi.fn(async () => ({ ...settings(), workspaceRoot: currentWorkspace }))
    }
    let resolveRequest!: (value: { ok: boolean; status: number; body: string }) => void
    const localDisplayRequest = vi.fn(() => new Promise<{ ok: boolean; status: number; body: string }>((resolve) => {
      resolveRequest = resolve
    }))
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      store: store as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    const pending = handlers.get('runtime:direct-source-preview')?.(event, {
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    })
    await vi.waitFor(() => expect(localDisplayRequest).toHaveBeenCalledWith(
      '/v1/local-display/direct-source-preview',
      JSON.stringify({
        workspaceRoot: '/tmp/case-a',
        kind: 'direct_source_preview',
        view: 'transactions',
        fields: ['account'],
        rowOffset: 0,
        rowLimit: 25,
        displayMode: 'full'
      })
    ))

    currentWorkspace = '/tmp/case-b'
    resolveRequest({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        kind: 'direct_source_preview',
        caseId: 'case-a',
        datasetSnapshotId: 'dsv2_' + 'a'.repeat(64),
        displayMode: 'full',
        view: 'transactions',
        fields: ['account'],
        rowOffset: 0,
        rowLimit: 25,
        hasMore: false,
        rows: [{
          rowIndex: 0,
          cells: [{ field: 'account', displayValue: 'CASE_A_SOURCE_EXACT_VALUE' }]
        }]
      })
    })

    const result = await pending
    expect(result).toEqual({
      ok: false,
      status: 403,
      code: 'forbidden',
      message: 'Local display request failed.'
    })
    expect(JSON.stringify(result)).not.toContain('CASE_A_SOURCE_EXACT_VALUE')
  })

  it('rejects accepted-slot calls outside the current non-destroyed main frame before runtime or Hub access', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const scenarios = [
      { name: 'missing window', window: null },
      { name: 'destroyed window', windowDestroyed: true },
      { name: 'destroyed sender', senderDestroyed: true },
      { name: 'wrong window', wrongWebContents: true },
      { name: 'missing sender frame', missingSenderFrame: true },
      { name: 'non-main sender frame', nonMainSenderFrame: true }
    ]

    for (const scenario of scenarios) {
      const localDisplayRequest = vi.fn(async () => ({
        ok: true,
        status: 200,
        body: 'INITIAL_AUTHORITY_SOURCE_EXACT_VALUE'
      }))
      const loadHubAccountService = vi.fn(async () => {
        throw new Error('Hub compatibility must stay cold')
      })
      const mainFrame = {}
      const sender = { mainFrame, isDestroyed: () => scenario.senderDestroyed === true }
      const event = {
        sender,
        senderFrame: scenario.missingSenderFrame
          ? null
          : scenario.nonMainSenderFrame
            ? {}
            : mainFrame
      }
      const mainWindow = scenario.window === null
        ? null
        : {
            isDestroyed: () => scenario.windowDestroyed === true,
            webContents: scenario.wrongWebContents ? {} : sender
          }
      registerAppIpcHandlers(registerOptions({
        loadHubAccountService,
        localDisplayRequest,
        getMainWindow: () => mainWindow as never
      } as never))

      const result = await handlers.get('runtime:accepted-slot-display')?.(event, {
        kind: 'accepted_slot_display',
        threadId: 'thread-accepted-authority-denied',
        turnId: 'turn-accepted-authority-denied',
        acceptedFinalDigest: 'a'.repeat(64),
        displayMode: 'full'
      })
      expect(result, scenario.name).toEqual({
        ok: false,
        status: 403,
        code: 'forbidden',
        message: 'Local display request failed.'
      })
      expect(localDisplayRequest, scenario.name).not.toHaveBeenCalled()
      expect(loadHubAccountService, scenario.name).not.toHaveBeenCalled()
      expect(JSON.stringify(result), scenario.name).not.toContain('INITIAL_AUTHORITY_SOURCE_EXACT_VALUE')
    }
  })

  it('requires accepted local-display slots to retain claim and receipt bindings', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const localDisplayRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        kind: 'accepted_slot_display',
        threadId: 'thread-1',
        turnId: 'turn-1',
        acceptedFinalDigest: 'a'.repeat(64),
        caseId: 'case-1',
        datasetSnapshotId: 'dsv2_' + 'a'.repeat(64),
        contextEpoch: 7,
        displayMode: 'full',
        slots: [{
          slotId: 'entity-slot-1',
          field: 'account',
          displayValue: 'LOCAL_ACCOUNT_VALUE',
          claimIds: [],
          receiptIds: []
        }]
      })
    }))
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    } as never))

    await expect(handlers.get('runtime:accepted-slot-display')?.(event, {
      kind: 'accepted_slot_display',
      threadId: 'thread-1',
      turnId: 'turn-1',
      acceptedFinalDigest: 'a'.repeat(64),
      displayMode: 'full'
    })).resolves.toEqual({
      ok: false,
      status: 502,
      code: 'invalid_response',
      message: 'Local display response was invalid.'
    })
  })

  it('rejects missing, mismatched, extra, or mistyped accepted-final response digests without reflecting exact values', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const acceptedFinalDigest = 'd'.repeat(64)
    const exactValue = 'HOSTILE_ACCEPTED_DIGEST_SOURCE_EXACT_VALUE'
    const baseResponse = {
      schemaVersion: 1,
      kind: 'accepted_slot_display',
      threadId: 'thread-accepted-digest',
      turnId: 'turn-accepted-digest',
      caseId: 'case-accepted-digest',
      datasetSnapshotId: 'dsv2_' + 'c'.repeat(64),
      contextEpoch: 8,
      displayMode: 'full',
      slots: [{
        slotId: 'account-slot-1',
        field: 'account',
        displayValue: exactValue,
        claimIds: ['claim-accepted-digest'],
        receiptIds: ['receipt-accepted-digest']
      }]
    }
    const hostileResponses = [
      { name: 'missing digest', body: baseResponse },
      { name: 'mismatched digest', body: { ...baseResponse, acceptedFinalDigest: 'e'.repeat(64) } },
      {
        name: 'extra digest property',
        body: {
          ...baseResponse,
          acceptedFinalDigest,
          acceptedFinalDigestExtra: acceptedFinalDigest
        }
      },
      { name: 'mistyped digest', body: { ...baseResponse, acceptedFinalDigest: 7 } }
    ]
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }

    for (const hostile of hostileResponses) {
      const localDisplayRequest = vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify(hostile.body)
      }))
      registerAppIpcHandlers(registerOptions({
        localDisplayRequest,
        getMainWindow: () => mainWindow as never
      } as never))

      const result = await handlers.get('runtime:accepted-slot-display')?.(event, {
        kind: 'accepted_slot_display',
        threadId: 'thread-accepted-digest',
        turnId: 'turn-accepted-digest',
        acceptedFinalDigest,
        displayMode: 'full'
      })
      expect.soft(result, hostile.name).toEqual({
        ok: false,
        status: 502,
        code: 'invalid_response',
        message: 'Local display response was invalid.'
      })
      expect.soft(JSON.stringify(result), hostile.name).not.toContain(exactValue)
    }
  })

  it('routes the strict accepted-slot contract through Main without widening the sink', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const response = {
      schemaVersion: 1,
      kind: 'accepted_slot_display',
      threadId: 'thread-accepted-route',
      turnId: 'turn-accepted-route',
      acceptedFinalDigest: 'd'.repeat(64),
      caseId: 'case-accepted-route',
      datasetSnapshotId: 'dsv2_' + 'c'.repeat(64),
      contextEpoch: 8,
      displayMode: 'masked',
      slots: [{
        slotId: 'card-slot-1',
        field: 'card',
        displayValue: '****7890',
        claimIds: ['claim-accepted-route'],
        receiptIds: ['receipt-accepted-route']
      }]
    }
    const localDisplayRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify(response)
    }))
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    const options = registerOptions({
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    } as never)
    registerAppIpcHandlers(options)

    const request = {
      kind: 'accepted_slot_display',
      threadId: 'thread-accepted-route',
      turnId: 'turn-accepted-route',
      acceptedFinalDigest: 'd'.repeat(64),
      displayMode: 'masked' as const
    }
    await expect(handlers.get('runtime:accepted-slot-display')?.(event, request)).resolves.toEqual(response)
    expect(localDisplayRequest).toHaveBeenCalledWith(
      '/v1/local-display/accepted-slot-display',
      JSON.stringify(request)
    )
    expect(JSON.stringify(response)).not.toContain('sourceFileId')
    expect(JSON.stringify(response)).not.toContain('sourceRowNumber')
    expect(JSON.stringify(response)).not.toContain('authorityEntityRef')
    expect(options.loadHubAccountService).not.toHaveBeenCalled()
  })

  it('drops late accepted-slot responses after window, navigation, frame, or destruction changes', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const scenarios = [
      'main window replacement',
      'navigation or reload main-frame replacement',
      'sender-frame replacement',
      'current window destruction',
      'sender webContents destruction',
      'current window webContents replacement'
    ] as const

    for (const scenario of scenarios) {
      let resolveRequest!: (value: { ok: boolean; status: number; body: string }) => void
      const localDisplayRequest = vi.fn(() => new Promise<{ ok: boolean; status: number; body: string }>((resolve) => {
        resolveRequest = resolve
      }))
      const loadHubAccountService = vi.fn(async () => {
        throw new Error('Hub compatibility must stay cold')
      })
      let windowDestroyed = false
      let senderDestroyed = false
      const mainFrame = {}
      const sender: { mainFrame: object; isDestroyed: () => boolean } = {
        mainFrame,
        isDestroyed: () => senderDestroyed
      }
      const event: { sender: typeof sender; senderFrame: object | null } = {
        sender,
        senderFrame: mainFrame
      }
      let mainWindow: { isDestroyed: () => boolean; webContents: object } = {
        isDestroyed: () => windowDestroyed,
        webContents: sender
      }
      registerAppIpcHandlers(registerOptions({
        loadHubAccountService,
        localDisplayRequest,
        getMainWindow: () => mainWindow as never
      } as never))

      const pending = handlers.get('runtime:accepted-slot-display')?.(event, {
        kind: 'accepted_slot_display',
        threadId: 'thread-accepted-late',
        turnId: 'turn-accepted-late',
        acceptedFinalDigest: 'a'.repeat(64),
        displayMode: 'full'
      })
      await vi.waitFor(() => expect(localDisplayRequest, scenario).toHaveBeenCalledTimes(1))
      switch (scenario) {
        case 'main window replacement':
          mainWindow = { isDestroyed: () => false, webContents: {} }
          break
        case 'navigation or reload main-frame replacement':
          sender.mainFrame = {}
          break
        case 'sender-frame replacement':
          event.senderFrame = {}
          break
        case 'current window destruction':
          windowDestroyed = true
          break
        case 'sender webContents destruction':
          senderDestroyed = true
          break
        case 'current window webContents replacement':
          mainWindow.webContents = {}
          break
      }
      resolveRequest({
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          kind: 'accepted_slot_display',
          threadId: 'thread-accepted-late',
          turnId: 'turn-accepted-late',
          acceptedFinalDigest: 'a'.repeat(64),
          caseId: 'case-accepted-late',
          datasetSnapshotId: 'dsv2_' + 'b'.repeat(64),
          contextEpoch: 9,
          displayMode: 'full',
          slots: [{
            slotId: 'account-slot-1',
            field: 'account',
            displayValue: 'LATE_ACCEPTED_SOURCE_EXACT_VALUE',
            claimIds: ['claim-accepted-late'],
            receiptIds: ['receipt-accepted-late']
          }]
        })
      })

      const result = await pending
      expect(result, scenario).toEqual({
        ok: false,
        status: 403,
        code: 'forbidden',
        message: 'Local display request failed.'
      })
      expect(JSON.stringify(result), scenario).not.toContain('LATE_ACCEPTED_SOURCE_EXACT_VALUE')
      expect(loadHubAccountService, scenario).not.toHaveBeenCalled()
    }
  })

  it('keeps standard accepted-slot authority independent of Hub account, gateway, and entitlement state', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const hubStates = [
      { name: 'logged out', snapshot: { authenticated: false, source: 'none' } },
      { name: 'login changed', snapshot: { authenticated: true, source: 'hub', user: { id: 'principal-b' } } },
      { name: 'gateway changed', snapshot: { gatewayConfigured: false, gateway: null } },
      { name: 'entitlement changed', snapshot: { entitlement: { edition: 'free', canSwitch: false } } }
    ]

    for (const hubState of hubStates) {
      const response = {
        schemaVersion: 1,
        kind: 'accepted_slot_display',
        threadId: 'thread-local-authority',
        turnId: 'turn-local-authority',
        acceptedFinalDigest: 'c'.repeat(64),
        caseId: 'case-local-authority',
        datasetSnapshotId: 'dsv2_' + 'd'.repeat(64),
        contextEpoch: 12,
        displayMode: 'full',
        slots: [{
          slotId: 'account-slot-1',
          field: 'account',
          displayValue: 'STANDARD_LOCAL_AUTHORITY_VALUE',
          claimIds: ['claim-local-authority'],
          receiptIds: ['receipt-local-authority']
        }]
      }
      const localDisplayRequest = vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify(response)
      }))
      const getSnapshot = vi.fn(async () => hubState.snapshot)
      const loadHubAccountService = vi.fn(async () => ({ getSnapshot }) as never)
      const mainFrame = {}
      const sender = { mainFrame, isDestroyed: () => false }
      const event = { sender, senderFrame: mainFrame }
      const mainWindow = { isDestroyed: () => false, webContents: sender }
      registerAppIpcHandlers(registerOptions({
        loadHubAccountService,
        localDisplayRequest,
        getMainWindow: () => mainWindow as never
      } as never))

      const result = await handlers.get('runtime:accepted-slot-display')?.(event, {
        kind: 'accepted_slot_display',
        threadId: 'thread-local-authority',
        turnId: 'turn-local-authority',
        acceptedFinalDigest: 'c'.repeat(64),
        displayMode: 'full'
      })
      expect(result, hubState.name).toEqual(response)
      expect(loadHubAccountService, hubState.name).not.toHaveBeenCalled()
      expect(getSnapshot, hubState.name).not.toHaveBeenCalled()
    }
  })

  it('keeps generic runtime requests from selecting local-display paths or headers', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()
    registerAppIpcHandlers(registerOptions({ runtimeRequest: runtimeRequest as never }))

    for (const path of [
      '/v1/local-display/import-mapping-preview',
      '/v1/local-display/cleaning-diff-preview',
      '/v1/local-display/direct-source-preview',
      '/v1/local-display/accepted-slot-display',
      '/v1/local-display/funds-import/stage',
      '/v1/local-display/funds-import/confirm',
      '/v1/local-display/funds-import/cancel',
      '/v1/local-display/funds-import/status',
      '/v1/local-display/funds-cleaning/run',
      '/v1/local-display/funds-cleaning/revoke'
    ]) {
      await expect(
        handlers.get('runtime:request')?.({}, {
          path,
          method: 'POST',
          body: '{}',
          headers: { 'X-Analytix-Local-Display': 'typed-v1' }
        })
      ).rejects.toThrow(/Invalid payload for runtime:request/)
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it.each(['confirm', 'cancel', 'workspace-change', 'frame-change', 'malformed'] as const)(
    'requires current native case-creation confirmation: %s', async (mode) => {
      const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
      const root = mkdtempSync(join(tmpdir(), 'analytix-main-case-creation-'))
      try {
        const workspace = await canonicalPath(root)
        let selectedWorkspace = workspace
        const sourcePath = join(workspace, 'synthetic.csv')
        const physicalIdentity = 'b'.repeat(64)
        const selector = `tlsel1_${'a'.repeat(64)}`
        const localDisplayRequest = vi.fn(async (_path: string, _body: string) => ({
          ok: true, status: 202,
          body: JSON.stringify({ status: 'case_creation_required', intent: mode === 'malformed' ? '/private/canary' : physicalIdentity })
        })).mockImplementationOnce(async () => ({
          ok: true, status: 202,
          body: JSON.stringify({ status: 'case_creation_required', intent: mode === 'malformed' ? '/private/canary' : physicalIdentity })
        })).mockImplementationOnce(async () => importStageResponseForMainTestV1(selector))
        const mainFrame = {}
        const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
        const event = { sender, senderFrame: mainFrame }
        const mainWindow = { isDestroyed: () => false, webContents: sender }
        vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
        vi.mocked(dialog.showMessageBox).mockImplementation(async () => {
          if (mode === 'workspace-change') selectedWorkspace = '/private'
          if (mode === 'frame-change') sender.mainFrame = {}
          return { response: mode === 'cancel' ? 0 : 1, checkboxChecked: false }
        })
        registerAppIpcHandlers(registerOptions({
          store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: selectedWorkspace })) } as never,
          localDisplayRequest, getMainWindow: () => mainWindow as never
        }))
        const result = await handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
        if (mode === 'confirm') {
          expect(result).toMatchObject({ ok: true, items: [{ selector }] })
          expect(localDisplayRequest).toHaveBeenNthCalledWith(2, '/v1/local-display/funds-import/stage',
            JSON.stringify({ workspaceRoot: workspace, sourcePath, createCaseIntent: physicalIdentity }))
        } else {
          expect(result).toMatchObject({ ok: false })
          expect(localDisplayRequest).toHaveBeenCalledTimes(1)
        }
        expect(JSON.stringify(result)).not.toContain(physicalIdentity)
        expect(JSON.stringify(result)).not.toContain(sourcePath)
        expect(dialog.showMessageBox).toHaveBeenCalledTimes(mode === 'malformed' ? 0 : 1)
      } finally { rmSync(root, { recursive: true, force: true }) }
    }
  )

  it.each(['capability', 'unknown-code', 'raw-body', 'wrong-status', 'extra-field', 'oversized'] as const)(
    'projects unavailable import capability only from the closed host response: %s', async (mode) => {
      const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
      const root = mkdtempSync(join(tmpdir(), 'analytix-import-capability-'))
      try {
        const workspace = await canonicalPath(root)
        const sourcePath = join(workspace, 'synthetic.csv')
        writeFileSync(sourcePath, 'synthetic input')
        const canary = '/private/IMPORT_CANARY/input.csv 13800138000'
        const response = {
          ok: false, status: mode === 'wrong-status' ? 409 : 503,
          body: mode === 'raw-body' ? canary : JSON.stringify({
            code: mode === 'unknown-code' ? 'unrecognized_capability' : 'funds_import_capability_unavailable',
            message: mode === 'oversized' ? canary.repeat(100) : canary,
            ...(mode === 'extra-field' ? { raw: canary } : {})
          })
        }
        const localDisplayRequest = vi.fn(async () => response)
        const mainFrame = {}
        const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
        const event = { sender, senderFrame: mainFrame }
        const mainWindow = { isDestroyed: () => false, webContents: sender }
        vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
        registerAppIpcHandlers(registerOptions({
          store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
          localDisplayRequest, getMainWindow: () => mainWindow as never
        }))
        const result = await handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
        expect(result).toEqual({
          ok: false, canceled: false,
          code: mode === 'capability' ? 'capability_unavailable' : 'runtime_unavailable',
          message: mode === 'capability' ? 'Trusted data import capability is unavailable in this runtime environment.' : 'Data import failed.'
        })
        expect(JSON.stringify(result)).not.toMatch(/IMPORT_CANARY|13800138000/)
        expect(dialog.showMessageBox).not.toHaveBeenCalled()
        expect(localDisplayRequest).toHaveBeenCalledTimes(1)
      } finally { rmSync(root, { recursive: true, force: true }) }
    }
  )

  it('keeps CSV or ZIP source selection in Main and returns only a short staged inventory', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-csv-admission-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    const sourceBody = Buffer.from('a,b\r\n1,2\r\n', 'utf8')
    writeFileSync(sourcePath, sourceBody, { mode: 0o600 })
    const selector = `tlsel1_${'a'.repeat(64)}`
    const localDisplayRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        status: 'ready',
        totalRowCount: 1,
        items: [{
          selector,
          sourceIndex: 1,
          sourceCount: 1,
          sourceLabel: 'Source 1 of 1',
          rowCount: 1,
          columnCount: 34,
          status: 'ready'
        }]
      })
    }))
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    const result = await handlers.get('runtime:stage-funds-csv-snapshot')?.(event, {
      sourcePath: '/tmp/renderer-forged.csv',
      workspaceRoot: '/tmp/renderer-forged-workspace'
    })
    expect(result).toEqual({
      ok: true,
      status: 'ready',
      totalRowCount: 1,
      items: [{
        selector,
        sourceIndex: 1,
        sourceCount: 1,
        sourceLabel: 'Source 1 of 1',
        rowCount: 1,
        columnCount: 34,
        status: 'ready'
      }]
    })
    expect(JSON.stringify(result)).not.toContain(sourcePath)
    expect(localDisplayRequest).toHaveBeenCalledWith(
      '/v1/local-display/funds-import/stage',
      JSON.stringify({ workspaceRoot: workspace, sourcePath })
    )
    expect(sender.once).toHaveBeenCalledWith('destroyed', expect.any(Function))
    rmSync(root, { recursive: true, force: true })
  })

  it('revokes a staged generation when the renderer authority changes before delivery', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-late-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.zip')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selector = `tlsel1_${'d'.repeat(64)}`
    let rendererDestroyed = false
    const localDisplayRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/cancel')) {
        return { ok: true, status: 200, body: JSON.stringify({ canceled: true }) }
      }
      rendererDestroyed = true
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          status: 'ready',
          totalRowCount: 1,
          items: [{
            selector,
            sourceIndex: 1,
            sourceCount: 1,
            sourceLabel: 'Source 1 of 1',
            rowCount: 1,
            columnCount: 34,
            status: 'ready'
          }]
        })
      }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => rendererDestroyed, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toEqual({
      ok: false,
      canceled: false,
      code: 'forbidden',
      message: 'Data import authority changed.'
    })
    expect(localDisplayRequest).toHaveBeenLastCalledWith(
      '/v1/local-display/funds-import/cancel',
      JSON.stringify({ selector })
    )
    expect(sender.once).not.toHaveBeenCalled()
    rmSync(root, { recursive: true, force: true })
  })

  it('does not let an old destroyed callback revoke a reselected generation', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-reselect-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selectors = [`tlsel1_${'1'.repeat(64)}`, `tlsel1_${'2'.repeat(64)}`]
    let stageIndex = 0
    const localDisplayRequest = vi.fn(async (path: string, body: string) => {
      if (path.endsWith('/stage')) {
        const selector = selectors[stageIndex++]
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            status: 'ready', totalRowCount: 1,
            items: [{
              selector, sourceIndex: 1, sourceCount: 1, sourceLabel: 'Source 1 of 1',
              rowCount: 1, columnCount: 34, status: 'ready'
            }]
          })
        }
      }
      if (path.endsWith('/cancel')) {
        return { ok: true, status: 200, body: JSON.stringify({ canceled: true }) }
      }
      const { selector } = JSON.parse(body) as { selector: string }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          status: 'ready', totalRowCount: 1,
          item: {
            selector, sourceIndex: 1, sourceCount: 1, sourceLabel: 'Source 1 of 1',
            rowCount: 1, columnCount: 34, status: 'ready'
          }
        })
      }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    const oldDestroyed = sender.once.mock.calls[0]?.[1] as (() => void) | undefined
    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    oldDestroyed?.()
    await Promise.resolve()
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, selectors[1])).resolves.toMatchObject({
      ok: true,
      item: { selector: selectors[1] }
    })
    const canceledSelectors = localDisplayRequest.mock.calls
      .filter(([path]) => path.endsWith('/cancel'))
      .map(([, body]) => (JSON.parse(body) as { selector: string }).selector)
    expect(canceledSelectors).toEqual([selectors[0]])
    rmSync(root, { recursive: true, force: true })
  })

  it('keeps the later stage current when an older Go stage response completes late', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-stage-interleave-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const oldSelector = `tlsel1_${'4'.repeat(64)}`
    const newSelector = `tlsel1_${'5'.repeat(64)}`
    let releaseOldStage!: (response: { ok: true, status: number, body: string }) => void
    const oldStageResponse = new Promise<{ ok: true, status: number, body: string }>((resolve) => {
      releaseOldStage = resolve
    })
    let stageCalls = 0
    const localDisplayRequest = vi.fn(async (path: string, body: string) => {
      if (path.endsWith('/stage')) {
        stageCalls += 1
        if (stageCalls === 1) return oldStageResponse
        return importStageResponseForMainTestV1(newSelector)
      }
      if (path.endsWith('/status')) {
        return {
          ok: true as const,
          status: 200,
          body: JSON.stringify({
            status: 'ready', totalRowCount: 1,
            item: {
              selector: newSelector, sourceIndex: 1, sourceCount: 1, sourceLabel: 'Source 1 of 1',
              rowCount: 1, columnCount: 34, status: 'ready'
            }
          })
        }
      }
      if (path.endsWith('/confirm')) {
        return {
          ok: true as const,
          status: 200,
          body: JSON.stringify({
            sourceArtifactSha256: '6'.repeat(64), sourceArtifactByteLength: 1024, sourceRowCount: 1
          })
        }
      }
      expect(JSON.parse(body)).toHaveProperty('selector')
      return { ok: true as const, status: 200, body: JSON.stringify({ canceled: true }) }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    const oldStage = handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
    await vi.waitFor(() => expect(stageCalls).toBe(1))
    const newStage = handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
    await expect(newStage).resolves.toMatchObject({ ok: true, items: [{ selector: newSelector }] })
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, newSelector)).resolves.toMatchObject({
      ok: true, item: { selector: newSelector }
    })

    releaseOldStage(importStageResponseForMainTestV1(oldSelector))
    await expect(oldStage).resolves.toMatchObject({ ok: false, code: 'forbidden' })
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, newSelector)).resolves.toMatchObject({
      ok: true, item: { selector: newSelector }
    })
    await expect(handlers.get('runtime:confirm-funds-csv-snapshot')?.(event, newSelector)).resolves.toEqual({
      ok: true, rowCount: 1
    })
    const canceledSelectors = localDisplayRequest.mock.calls
      .filter(([path]) => path.endsWith('/cancel'))
      .map(([, body]) => (JSON.parse(body) as { selector: string }).selector)
    expect(canceledSelectors).toEqual([oldSelector])
    rmSync(root, { recursive: true, force: true })
  })

  it('keeps the later stage current when an older file dialog completes late', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-dialog-interleave-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selector = `tlsel1_${'9'.repeat(64)}`
    let releaseOldDialog!: (selection: { canceled: false, filePaths: string[] }) => void
    const oldDialog = new Promise<{ canceled: false, filePaths: string[] }>((resolve) => {
      releaseOldDialog = resolve
    })
    vi.mocked(dialog.showOpenDialog)
      .mockImplementationOnce(async () => oldDialog)
      .mockResolvedValueOnce({ canceled: false, filePaths: [sourcePath] })
    const localDisplayRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/stage')) return importStageResponseForMainTestV1(selector)
      if (path.endsWith('/status')) {
        return {
          ok: true as const, status: 200,
          body: JSON.stringify({
            status: 'ready', totalRowCount: 1,
            item: {
              selector, sourceIndex: 1, sourceCount: 1, sourceLabel: 'Source 1 of 1',
              rowCount: 1, columnCount: 34, status: 'ready'
            }
          })
        }
      }
      return {
        ok: true as const, status: 200,
        body: JSON.stringify({
          sourceArtifactSha256: 'a'.repeat(64), sourceArtifactByteLength: 1024, sourceRowCount: 1
        })
      }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    const oldStage = handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
    await vi.waitFor(() => expect(dialog.showOpenDialog).toHaveBeenCalledTimes(1))
    const newStage = handlers.get('runtime:stage-funds-csv-snapshot')?.(event)
    await expect(newStage).resolves.toMatchObject({ ok: true, items: [{ selector }] })
    releaseOldDialog({ canceled: false, filePaths: [sourcePath] })
    await expect(oldStage).resolves.toMatchObject({ ok: false, code: 'forbidden' })
    expect(localDisplayRequest.mock.calls.filter(([path]) => path.endsWith('/stage'))).toHaveLength(1)
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, selector)).resolves.toMatchObject({
      ok: true, item: { selector }
    })
    await expect(handlers.get('runtime:confirm-funds-csv-snapshot')?.(event, selector)).resolves.toEqual({
      ok: true, rowCount: 1
    })
    rmSync(root, { recursive: true, force: true })
  })

  it('rejects inconsistent status echoes without revoking the current generation', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-status-echo-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.zip')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selectors = [`tlsel1_${'7'.repeat(64)}`, `tlsel1_${'8'.repeat(64)}`]
    const acceptedStatus = {
      status: 'ready',
      totalRowCount: 3,
      item: {
        selector: selectors[0], sourceIndex: 1, sourceCount: 2, sourceLabel: 'Source 1 of 2',
        rowCount: 1, columnCount: 34, status: 'ready'
      }
    }
    const statusVariants = [
      {
        name: 'same-selector source item substitution',
        response: {
          ...acceptedStatus,
          item: { ...acceptedStatus.item, sourceIndex: 2, sourceLabel: 'Source 2 of 2', rowCount: 2 }
        }
      },
      {
        name: 'source index drift',
        response: { ...acceptedStatus, item: { ...acceptedStatus.item, sourceIndex: 2 } }
      },
      {
        name: 'source count drift',
        response: {
          ...acceptedStatus,
          item: { ...acceptedStatus.item, sourceCount: 3, sourceLabel: 'Source 1 of 3' }
        }
      },
      {
        name: 'source label drift',
        response: { ...acceptedStatus, item: { ...acceptedStatus.item, sourceLabel: 'Source 2 of 2' } }
      },
      {
        name: 'row count drift',
        response: { ...acceptedStatus, item: { ...acceptedStatus.item, rowCount: 2 } }
      },
      {
        name: 'column count drift',
        response: { ...acceptedStatus, item: { ...acceptedStatus.item, columnCount: 33 } }
      },
      { name: 'total row count drift', response: { ...acceptedStatus, totalRowCount: 4 } },
      {
        name: 'item status drift',
        response: { ...acceptedStatus, item: { ...acceptedStatus.item, status: 'mapping_invalid' } }
      },
      { name: 'generation status drift', response: { ...acceptedStatus, status: 'mapping_invalid' } },
      {
        name: 'coherent status pair drift',
        response: {
          ...acceptedStatus,
          status: 'mapping_invalid',
          item: { ...acceptedStatus.item, status: 'mapping_invalid' }
        }
      }
    ]
    let statusCall = 0
    const localDisplayRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/stage')) {
        return {
          ok: true as const, status: 200,
          body: JSON.stringify({
            status: 'ready', totalRowCount: 3,
            items: selectors.map((selector, index) => ({
              selector, sourceIndex: index + 1, sourceCount: 2, sourceLabel: `Source ${index + 1} of 2`,
              rowCount: index + 1, columnCount: 34, status: 'ready'
            }))
          })
        }
      }
      if (path.endsWith('/status')) {
        const response = statusVariants[statusCall]?.response ?? acceptedStatus
        statusCall += 1
        return { ok: true as const, status: 200, body: JSON.stringify(response) }
      }
      if (path.endsWith('/confirm')) {
        return {
          ok: true as const, status: 200,
          body: JSON.stringify({
            sourceArtifactSha256: 'b'.repeat(64), sourceArtifactByteLength: 1024, sourceRowCount: 3
          })
        }
      }
      return { ok: true as const, status: 200, body: JSON.stringify({ canceled: true }) }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    for (const variant of statusVariants) {
      await expect(
        handlers.get('runtime:status-funds-csv-import')?.(event, selectors[0]),
        variant.name
      ).resolves.toMatchObject({
        ok: false, code: 'runtime_unavailable'
      })
    }
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, selectors[0])).resolves.toMatchObject({
      ok: true, item: { selector: selectors[0] }
    })
    expect(localDisplayRequest.mock.calls.filter(([path]) => path.endsWith('/cancel'))).toHaveLength(0)
    await expect(handlers.get('runtime:confirm-funds-csv-snapshot')?.(event, selectors[0])).resolves.toEqual({
      ok: true, rowCount: 3
    })
    rmSync(root, { recursive: true, force: true })
  })

  it('revokes a staged generation before preview when the Main workspace changes', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const rootA = mkdtempSync(join(tmpdir(), 'analytix-main-import-case-a-'))
    const rootB = mkdtempSync(join(tmpdir(), 'analytix-main-import-case-b-'))
    const workspaceA = await canonicalPath(rootA)
    const workspaceB = await canonicalPath(rootB)
    const sourcePath = join(workspaceA, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selector = `tlsel1_${'3'.repeat(64)}`
    let currentWorkspace = workspaceA
    const localDisplayRequest = vi.fn(async (path: string) => {
      if (path.endsWith('/stage')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            status: 'ready', totalRowCount: 1,
            items: [{
              selector, sourceIndex: 1, sourceCount: 1, sourceLabel: 'Source 1 of 1',
              rowCount: 1, columnCount: 34, status: 'ready'
            }]
          })
        }
      }
      return { ok: true, status: 200, body: JSON.stringify({ canceled: true }) }
    })
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: currentWorkspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))
    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    currentWorkspace = workspaceB
    await expect(handlers.get('runtime:import-mapping-preview')?.(event, {
      kind: 'import_mapping_preview', selector,
      fields: ['sourceColumn', 'sampleValue'], rowOffset: 0, rowLimit: 25, displayMode: 'full'
    })).resolves.toMatchObject({ ok: false, status: 400, code: 'invalid_request' })
    expect(localDisplayRequest).toHaveBeenLastCalledWith(
      '/v1/local-display/funds-import/cancel', JSON.stringify({ selector })
    )
    expect(localDisplayRequest).not.toHaveBeenCalledWith(
      '/v1/local-display/import-mapping-preview', expect.any(String)
    )
    rmSync(rootA, { recursive: true, force: true })
    rmSync(rootB, { recursive: true, force: true })
  })

  it('preserves a selected symlink identity for the Go acquisition owner to reject', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-symlink-'))
    const workspace = await canonicalPath(root)
    const targetPath = join(workspace, 'transactions.csv')
    const linkPath = join(workspace, 'selected.csv')
    writeFileSync(targetPath, Buffer.from('a,b\r\n1,2\r\n', 'utf8'), { mode: 0o600 })
    symlinkSync(targetPath, linkPath)
    const localDisplayRequest = vi.fn(async () => ({
      ok: false,
      status: 400,
      body: JSON.stringify({ code: 'invalid_request' })
    }))
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [linkPath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toEqual({
      ok: false,
      canceled: false,
      code: 'invalid_source',
      message: 'Data import failed.'
    })
    expect(localDisplayRequest).toHaveBeenCalledWith(
      '/v1/local-display/funds-import/stage',
      JSON.stringify({ workspaceRoot: workspace, sourcePath: linkPath })
    )
    expect(localDisplayRequest).not.toHaveBeenCalledWith(
      '/v1/local-display/funds-import/stage',
      JSON.stringify({ workspaceRoot: workspace, sourcePath: targetPath })
    )
    rmSync(root, { recursive: true, force: true })
  })

  it('uses selector-only Main channels for import status, confirm, and cancel', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const root = mkdtempSync(join(tmpdir(), 'analytix-main-import-actions-'))
    const workspace = await canonicalPath(root)
    const sourcePath = join(workspace, 'transactions.csv')
    writeFileSync(sourcePath, Buffer.from('synthetic', 'utf8'), { mode: 0o600 })
    const selectors = ['b', 'c', 'd'].map((value) => `tlsel1_${value.repeat(64)}`)
    let stageIndex = 0
    const mainFrame = {}
    const sender = { mainFrame, isDestroyed: () => false, once: vi.fn() }
    const event = { sender, senderFrame: mainFrame }
    const mainWindow = { isDestroyed: () => false, webContents: sender }
    const localDisplayRequest = vi.fn(async (path: string, body: string) => {
      if (path.endsWith('/stage')) {
        const selector = selectors[stageIndex++]
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            status: 'ready',
            totalRowCount: 2,
            items: [{
              selector,
              sourceIndex: 1,
              sourceCount: 1,
              sourceLabel: 'Source 1 of 1',
              rowCount: 2,
              columnCount: 34,
              status: 'ready'
            }]
          })
        }
      }
      if (path.endsWith('/confirm')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            sourceArtifactSha256: 'c'.repeat(64),
            sourceArtifactByteLength: 1024,
            sourceRowCount: 2
          })
        }
      }
      if (path.endsWith('/cancel')) {
        return { ok: true, status: 200, body: JSON.stringify({ canceled: true }) }
      }
      const { selector } = JSON.parse(body) as { selector: string }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          status: 'ready',
          totalRowCount: 2,
          item: {
            selector,
            sourceIndex: 1,
            sourceCount: 1,
            sourceLabel: 'Source 1 of 1',
            rowCount: 2,
            columnCount: 34,
            status: 'ready'
          }
        })
      }
    })
    vi.mocked(dialog.showOpenDialog).mockResolvedValue({ canceled: false, filePaths: [sourcePath] })
    registerAppIpcHandlers(registerOptions({
      store: { load: vi.fn(async () => ({ ...settings(), workspaceRoot: workspace })) } as never,
      localDisplayRequest,
      getMainWindow: () => mainWindow as never
    }))

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    await expect(handlers.get('runtime:status-funds-csv-import')?.(event, selectors[0])).resolves.toEqual({
      ok: true,
      status: 'ready',
      totalRowCount: 2,
      item: expect.objectContaining({ selector: selectors[0] })
    })
    await expect(handlers.get('runtime:cancel-funds-csv-import')?.(event, selectors[0])).resolves.toEqual({
      ok: true,
      canceled: true
    })

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    await expect(handlers.get('runtime:confirm-funds-csv-snapshot')?.(event, selectors[1])).resolves.toEqual({
      ok: true,
      rowCount: 2
    })

    await expect(handlers.get('runtime:stage-funds-csv-snapshot')?.(event)).resolves.toMatchObject({ ok: true })
    await expect(handlers.get('runtime:cancel-funds-csv-import')?.(event, selectors[2])).resolves.toEqual({
      ok: true,
      canceled: true
    })
    for (const [path, body] of localDisplayRequest.mock.calls.filter(([path]) => !path.endsWith('/stage'))) {
      expect(path).toMatch(/\/(?:status|confirm|cancel)$/)
      expect(body).toMatch(/^\{"selector":"tlsel1_[a-f0-9]{64}"\}$/)
      expect(body).not.toMatch(/workspace|case|snapshot|principal|grant|sourcePath/i)
    }
    await expect(
      handlers.get('runtime:confirm-funds-csv-snapshot')?.(event, { selector: selectors[2], sourcePath: '/forged.csv' })
    ).rejects.toThrow(/Invalid payload/)
    rmSync(root, { recursive: true, force: true })
  })

  it('keeps the local-display runtime header and generation pin in the main-owned narrow dependency', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/main/index.ts'), 'utf8')
    expect(source).toContain('localDisplayRequest: async (path, body) =>')
    expect(source).toContain('const requestSettings = ensuredSettings ?? settings')
    expect(source).toContain('const runtimeAuthority = captureCurrentFinalPublicationAuthorityPin()')
    expect(source).toContain('if (!isCurrentFinalPublicationAuthorityPin(runtimeAuthority))')
    expect(source).toContain("headers.set('X-Analytix-Local-Display', 'typed-v1')")
  })

  it('passes encoded shared endpoint builder paths to the runtime adapter unchanged', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const response = {
      ok: false,
      status: 404,
      body: JSON.stringify({
        code: 'not_found',
        message: 'The requested resource was not found.'
      })
    }
    const attachmentResponse = {
      ok: true,
      status: 200,
      body: JSON.stringify({
        attachment: {
          id: 'att_0123456789abcdef01234567',
          name: 'statement.pdf',
          kind: 'document',
          mimeType: 'application/pdf',
          byteSize: 1,
          scope: 'thread',
          createdAt: '2026-07-14T00:00:00Z',
          updatedAt: '2026-07-14T00:00:00Z'
        },
        dataBase64: 'AA=='
      })
    }
    const runtimeRequest = vi.fn(async (path: string) =>
      path.startsWith('/v1/attachments/') ? attachmentResponse : response
    )
    const threadId = 'thr/with space?x=1#frag'
    const turnId = 'turn/with space?x=1#frag'
    const checkpointId = 'axcp/with space?x=1#frag'
    const requests = [
      {
        path: analytixThreadSteerPath(threadId, turnId),
        method: 'POST',
        body: '{"action":"step"}'
      },
      {
        path: analytixThreadRewindPath(threadId),
        method: 'POST',
        body: '{"turnId":"turn_2"}'
      },
      {
        path: analytixThreadCheckpointRewindPlanPath(threadId, checkpointId),
        method: 'POST',
        body: '{"scope":"combined"}'
      },
      { path: analytixApprovalPath('appr/with space?x=1#frag'), method: 'POST', body: '{}' },
      { path: analytixUserInputPath('input/with space?x=1#frag'), method: 'POST', body: '{}' },
      { path: analytixSessionResumePath('sess/with space?x=1#frag'), method: 'POST', body: '{}' },
      {
        path: `${analytixAttachmentContentPath('att/with space?x=1#frag')}?thread_id=${encodeURIComponent(threadId)}&workspace=${encodeURIComponent('/workspace/with space')}`,
        method: 'GET'
      },
      { path: analytixMemoryRecordPath('mem/with space?x=1#frag'), method: 'PATCH', body: '{}' }
    ] as const

    registerAppIpcHandlers(registerOptions({ runtimeRequest: runtimeRequest as never }))
    const handler = handlers.get('runtime:request')

    for (const request of requests) {
      await expect(handler?.({}, request)).resolves.toEqual(
        request.path.startsWith('/v1/attachments/') ? attachmentResponse : response
      )
    }

    expect(runtimeRequest).toHaveBeenCalledTimes(requests.length)
    for (const [index, request] of requests.entries()) {
      expect(request.path).toContain('%2F')
      expect(runtimeRequest).toHaveBeenNthCalledWith(
        index + 1,
        request.path,
        request.method,
        'body' in request ? request.body : undefined
      )
    }
  })

  it('rejects legacy reasoning-bearing responses at the Electron IPC boundary', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        turns: [{
          items: [
            { kind: 'assistant_reasoning', text: 'PRIVATE_ITEM' },
            { kind: 'assistant_text', role: 'assistant', text: '<think>PRIVATE_TAG</think>PUBLIC' }
          ]
        }]
      })
    }))
    registerAppIpcHandlers(registerOptions({ runtimeRequest: runtimeRequest as never }))

    const response = await handlers.get('runtime:request')?.({}, {
      path: '/v1/threads/thread-1',
      method: 'GET'
    }) as { ok: boolean; status: number; body: string }

    expect(response.ok).toBe(false)
    expect(response.status).toBe(502)
    expect(response.body).not.toMatch(/PRIVATE_ITEM|PRIVATE_TAG|assistant_reasoning|PUBLIC/)
  })

  it.each([
    ['primary', 'connect'], ['primary', 'update'], ['primary', 'credential-replace'],
    ['secondary', 'connect'], ['secondary', 'update'], ['secondary', 'credential-replace']
  ] as const)('admits the trusted %s desktop window for Registry %s', async (windowKind, operation) => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const registryIncarnation = `inc_${'a'.repeat(43)}`
    const providerIncarnation = `inc_${'b'.repeat(43)}`
    const nextRevision = operation === 'connect' ? '1' : '2'
    const syntheticCredential = randomUUID()
    const frame = {}
    const sender = { mainFrame: frame, isDestroyed: () => false }
    const event = { sender, senderFrame: frame }
    const primaryWindow = { isDestroyed: () => false, webContents: windowKind === 'primary' ? sender : {} }
    const trustedSender = vi.fn((candidate) => candidate.sender === sender)
    const runtimeRequest = vi.fn(async (_path: string, _method?: string) => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        registryRevision: nextRevision,
        registryIncarnation,
        provider: {
          id: 'provider-alpha',
          kind: 'openai-compatible',
          endpoint: 'https://provider.invalid/v1',
          models: ['model-alpha'],
          mediaModels: [],
          selectedModel: 'model-alpha',
          selectedRoutes: ['primary'],
          credentialConfigured: true,
          credentialPurpose: 'provider-api-key',
          revision: nextRevision,
          generation: nextRevision,
          incarnation: providerIncarnation,
          tombstone: false
        }
      })
    }))
    registerAppIpcHandlers(registerOptions({
      runtimeRequest: runtimeRequest as never,
      getMainWindow: () => primaryWindow as never,
      isTrustedProviderRegistrySender: trustedSender
    }))

    const result = await handlers.get('provider-registry:request')?.(event, {
      schemaVersion: 1,
      operation,
      ...(operation === 'connect' ? {} : { providerId: 'provider-alpha' }),
      expected: {
        registryRevision: operation === 'connect' ? '0' : '1',
        registryIncarnation,
        providerRevision: operation === 'connect' ? '0' : '1',
        providerGeneration: operation === 'connect' ? '0' : '1',
        providerIncarnation: operation === 'connect' ? '' : providerIncarnation,
        providerCredentialPurpose: operation === 'connect' ? '' : 'provider-api-key'
      },
      ...(operation === 'credential-replace' ? {} : { provider: {
        id: 'provider-alpha',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        proxy: '',
        models: ['model-alpha'],
        mediaModels: [],
        selectedModel: 'model-alpha',
        selectedMediaModel: '',
        selectedRoutes: ['primary']
      } }),
      credential: {
        kind: 'set',
        purpose: 'provider-api-key',
        valueBase64: Buffer.from(syntheticCredential).toString('base64')
      }
    })

    expect(handlers.has('provider-registry:request')).toBe(true)
    expect(trustedSender).toHaveBeenCalledWith(event)
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
    expect(runtimeRequest.mock.calls[0]?.[0]).toBe(
      operation === 'connect' ? '/v1/provider-registry'
        : operation === 'update' ? '/v1/provider-registry/providers/provider-alpha'
          : '/v1/provider-registry/providers/provider-alpha/credential')
    expect(runtimeRequest.mock.calls[0]?.[1]).toBe(
      operation === 'connect' ? 'POST' : operation === 'update' ? 'PATCH' : 'PUT'
    )
    expect(result).toMatchObject({ schemaVersion: 1, provider: { credentialConfigured: true } })
    expect(JSON.stringify(result).includes(syntheticCredential)).toBe(false)
    expect(JSON.stringify(result).includes(Buffer.from(syntheticCredential).toString('base64'))).toBe(false)
  })

  it.each([
    'missing-predicate', 'untrusted', 'throwing-predicate', 'missing-event',
    'missing-frame', 'subframe', 'destroyed-sender', 'replaced-frame'
  ])('denies Registry writes for %s before dispatch', async (scenario) => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const frame = {}
    const sender = { mainFrame: frame, isDestroyed: () => scenario === 'destroyed-sender' }
    const event = scenario === 'missing-event' ? undefined : {
      sender,
      senderFrame: scenario === 'missing-frame' ? null : scenario === 'subframe' ? {} : frame
    }
    if (scenario === 'replaced-frame') sender.mainFrame = {}
    const runtimeRequest = vi.fn()
    const trustedSender = vi.fn(() => {
      if (scenario === 'throwing-predicate') throw new Error('Stale fixture identity')
      return scenario !== 'untrusted'
    })
    registerAppIpcHandlers(registerOptions({
      runtimeRequest: runtimeRequest as never,
      isTrustedProviderRegistrySender: scenario === 'missing-predicate' ? undefined : trustedSender
    }))
    const result = await handlers.get('provider-registry:request')?.(event, { schemaVersion: 1, operation: 'connect' })
    expect(result).toEqual({
      schemaVersion: 1,
      error: { code: 'unauthorized', message: 'Provider registry authentication is required.' }
    })
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('captures the ordinary import fence and exact destination account binding only in the private recovery receipt', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const manifestJson = JSON.stringify({
      schema: 'analytix.provider-portable-manifest/v1',
      providers: [],
      accounts: [{
        correlation: 'account-0', owner: 'mcp', provider: 'synthetic-current-mcp-provider',
        endpoint: 'https://portable-account.invalid/v1', purpose: 'mcp-oauth-access-token',
        intent: 'reentry_required'
      }]
    })
    const destinationBinding = {
      owner: 'mcp' as const,
      provider: 'synthetic-current-mcp-provider',
      accountId: 'synthetic-destination-account',
      channelId: 'synthetic-destination-channel',
      purpose: 'mcp-oauth-access-token' as const,
      fingerprint: 'synthetic-destination-fingerprint'
    }
    const providerIncarnation = `inc_${'f'.repeat(43)}`
    const runtimeRequest = vi.fn(async (path: string, method?: string) => {
      if (path === '/v1/provider-registry/portable-manifest' && method === 'POST') {
        return {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1, providerCount: 0, accountCount: 1, reentryRequired: 1,
            entries: [{
              correlation: 'account-0', destinationProviderId: 'provider-private-destination',
              status: 'reentry_required'
            }]
          })
        }
      }
      return {
        ok: true, status: 200, body: JSON.stringify({
          schemaVersion: 1,
          registryRevision: '19',
          registryIncarnation: `inc_${'r'.repeat(43)}`,
          providers: [{
            id: 'provider-private-destination', kind: 'analytix-private-account',
            endpoint: 'https://portable-account.invalid/v1', models: [], mediaModels: [], selectedRoutes: [],
            credentialConfigured: false, revision: '17', generation: '5',
            incarnation: providerIncarnation, tombstone: false
          }]
        })
      }
    })
    let privatePrepareBody = ''
    const protectedRuntimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      privatePrepareBody = body ?? ''
      return {
        ok: false,
        status: 400,
        body: JSON.stringify({
          schemaVersion: 1,
          error: { code: 'invalid_request', message: 'The provider registry request was rejected.' }
        })
      }
    })
    const frame = { routingId: 71, url: 'file:///isolated/private-recovery.html' }
    const webContents = {
      id: 72, isDestroyed: () => false, mainFrame: frame, getURL: () => frame.url
    }
    const mainWindow = { id: 73, isDestroyed: () => false, webContents }
    let currentBindings = [destinationBinding]
    registerAppIpcHandlers(registerOptions({
      runtimeRequest: runtimeRequest as never,
      protectedRuntimeRequest: protectedRuntimeRequest as never,
      getMainWindow: () => mainWindow as never,
      getCurrentProfile: () => ({ profileBinding: 'synthetic-profile', dataDirectory: '/isolated/profile' }),
      isTrustedProviderRegistrySender: (event) => event.sender === webContents as unknown,
      getCurrentOwnerBindingInventory: () => currentBindings
    }))

    const ordinaryResult = await handlers.get('provider-registry:request')?.({ sender: webContents, senderFrame: frame }, {
      schemaVersion: 1, operation: 'import-portable-manifest', manifestJson
    })
    const ordinaryCalls = runtimeRequest.mock.calls.length
    for (const operation of ['connect', 'update', 'credential-replace', 'select', 'disconnect', 'explicit-delete', 'import-portable-manifest']) {
      const rejected = await handlers.get('provider-registry:request')?.({}, { schemaVersion: 1, operation, manifestJson })
      expect(rejected).toEqual({
        schemaVersion: 1,
        error: { code: 'unauthorized', message: 'Provider registry authentication is required.' }
      })
      expect(runtimeRequest.mock.calls.length).toBe(ordinaryCalls)
    }
    // The same private receipt must survive every rejected ordinary mutation.
    await handlers.get('provider-credential-recovery:create-destination-request')?.({
      sender: webContents, senderFrame: frame
    })

    expect(ordinaryResult).toEqual({
      schemaVersion: 1, providerCount: 0, accountCount: 1, reentryRequired: 1,
      entries: [{
        correlation: 'account-0', destinationProviderId: 'provider-private-destination', status: 'reentry_required'
      }]
    })
    expect(JSON.stringify(ordinaryResult)).not.toMatch(/fence|destinationOwnerBinding|accountId|channelId|fingerprint/)
    const privatePrepare = JSON.parse(privatePrepareBody) as {
      importResult: { entries: Array<Record<string, unknown>> }
    }
    expect(privatePrepare.importResult.entries).toEqual([{
      correlation: 'account-0', destinationProviderId: 'provider-private-destination', status: 'reentry_required',
      fence: { revision: '17', generation: '5', incarnation: providerIncarnation },
      destinationOwnerBinding: { ...destinationBinding, correlation: 'account-0' }
    }])

    for (const invalidInventory of [
      [],
      [destinationBinding, { ...destinationBinding, accountId: 'synthetic-ambiguous-account' }]
    ]) {
      currentBindings = invalidInventory
      protectedRuntimeRequest.mockClear()
      const repeatedOrdinaryResult = await handlers.get('provider-registry:request')?.({ sender: webContents, senderFrame: frame }, {
        schemaVersion: 1, operation: 'import-portable-manifest', manifestJson
      })
      const protectedResult = await handlers.get('provider-credential-recovery:create-destination-request')?.({
        sender: webContents, senderFrame: frame
      })
      expect(repeatedOrdinaryResult).toEqual(ordinaryResult)
      expect(protectedResult).toEqual({ ok: false, code: 'invalid_request' })
      expect(protectedRuntimeRequest).not.toHaveBeenCalled()
    }
  })

  it('routes speech as bounded key-free intent to the fixed private Go media executor', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const privateMediaRequest = vi.fn(async (_body: string) => ({
      ok: true,
      status: 200,
      body: JSON.stringify({ schemaVersion: 1, status: 'ok', transcript: ' current Registry transcript ' })
    }))
    registerAppIpcHandlers(registerOptions({ privateMediaRequest }))

    const result = await handlers.get('speech:transcribe')?.({}, {
      audioBase64: 'UklGRg==',
      mimeType: 'audio/wav',
      durationMs: 500
    })

    expect(result).toEqual({ ok: true, text: 'current Registry transcript' })
    expect(privateMediaRequest).toHaveBeenCalledTimes(1)
    const [rawBody] = privateMediaRequest.mock.calls[0]
    const body = JSON.parse(String(rawBody)) as Record<string, unknown>
    expect(body).toMatchObject({
      schemaVersion: 1,
      operation: 'speech.transcribe',
      audioBase64: 'UklGRg==',
      mimeType: 'audio/wav'
    })
    expect(Object.keys(body).sort()).toEqual([
      'audioBase64', 'mimeType', 'operation', 'schemaVersion', 'timeoutMs'
    ])
    expect(JSON.stringify(body)).not.toMatch(/provider|baseUrl|endpoint|proxy|credential|apiKey|protocol|model/i)

    await expect(handlers.get('speech:transcribe')?.({}, {
      audioBase64: 'UklGRg==',
      mimeType: 'audio/wav',
      speechToText: { baseUrl: 'https://renderer-authority.invalid', apiKey: 'forbidden' }
    })).rejects.toThrow(/Invalid payload for speech:transcribe/)
    expect(privateMediaRequest).toHaveBeenCalledTimes(1)

    await expect(handlers.get('runtime:request')?.({}, {
      path: '/v1/runtime/_private/media-execution',
      method: 'POST',
      body: JSON.stringify(body)
    })).rejects.toThrow(/Invalid payload for runtime:request/)
    expect(privateMediaRequest).toHaveBeenCalledTimes(1)
  })

  it('preserves Core media privacy refusal as an actionable message without returning raw content', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const privateMediaRequest = vi.fn(async () => ({ ok: false, status: 503,
      body: JSON.stringify({ schemaVersion: 1, code: 'privacy_unavailable', message: '/Users/private-owner/case.csv 13800138000' }) }))
    registerAppIpcHandlers(registerOptions({ privateMediaRequest }))
    const result = await handlers.get('speech:transcribe')?.({}, {
      audioBase64: 'UklGRg==', mimeType: 'audio/wav', durationMs: 500
    })
    expect(result).toEqual({ ok: false, message: 'Audio transcription is unavailable until trusted local privacy inspection is supported.' })
    expect(JSON.stringify(result)).not.toMatch(/13800138000|private-owner/)
  })

  it('keeps generated images on the typed private transport and preserves Core refusal for reference edits', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const workspace = mkdtempSync(join(tmpdir(), 'typed-media-ipc-'))
    const imageBase64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
    const runtimeRequest = vi.fn()
    const privateMediaRequest = vi.fn(async (rawBody: string) => {
      const request = JSON.parse(rawBody) as { operation: string }
      return request.operation === 'image.generate'
        ? { ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, status: 'ok', imageBase64, mimeType: 'image/png' }) }
        : { ok: false, status: 503, body: JSON.stringify({ code: 'privacy_unavailable', message: '/Users/private-owner/case.csv' }) }
    })
    registerAppIpcHandlers(registerOptions({ runtimeRequest, privateMediaRequest }))
    try {
      const payload = { text: 'synthetic diagram', filePath: join(workspace, 'document.md'), workspaceRoot: workspace }
      const generated = await handlers.get('write:generate-infographic')?.({}, payload) as { ok: boolean; absolutePath: string }
      expect(generated).toMatchObject({ ok: true })
      expect(readFileSync(generated.absolutePath).toString('base64')).toBe(imageBase64)
      const referenceImagePath = join(workspace, 'reference.png')
      writeFileSync(referenceImagePath, Buffer.from(imageBase64, 'base64'))
      const refused = await handlers.get('write:generate-infographic')?.({}, { ...payload, referenceImagePath })
      expect(refused).toEqual({ ok: false, message: 'This media content cannot be safely projected. Reference image editing requires trusted local privacy inspection.' })
      expect(privateMediaRequest.mock.calls.map(([body]) => JSON.parse(body).operation)).toEqual(['image.generate', 'image.edit'])
      expect(runtimeRequest).not.toHaveBeenCalled()
    } finally {
      rmSync(workspace, { recursive: true, force: true })
    }
  })

  it('fails closed when the private media transport is absent without using generic runtime requests', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()
    registerAppIpcHandlers(registerOptions({ runtimeRequest }))
    const result = await handlers.get('speech:transcribe')?.({}, {
      audioBase64: 'UklGRg==', mimeType: 'audio/wav', durationMs: 500
    })
    expect(result).toEqual({ ok: false, message: 'speech-to-text provider is not configured' })
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('rejects raw dynamic and singular compatibility paths before runtime adapter handoff', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const runtimeRequest = vi.fn()
    const rejectedRoutes = [
      { path: '/v1/threads/thr/raw/rewind', method: 'POST', body: '{}' },
      { path: '/v1/threads/thr/raw/turns/turn/raw/steer', method: 'POST', body: '{}' },
      { path: '/v1/threads/thr/raw/checkpoints/axcp/raw/rewind-plan', method: 'POST', body: '{}' },
      { path: '/v1/approvals/appr/raw', method: 'POST', body: '{}' },
      { path: '/v1/user-input/input_raw', method: 'POST', body: '{}' },
      { path: '/v1/sessions/sess/raw/resume-thread', method: 'POST', body: '{}' },
      { path: '/v1/attachments/att/raw/content', method: 'GET' }
    ] as const

    registerAppIpcHandlers(registerOptions({ runtimeRequest: runtimeRequest as never }))
    const handler = handlers.get('runtime:request')

    for (const route of rejectedRoutes) {
      await expect(handler?.({}, route)).rejects.toThrow(/Invalid payload for runtime:request/)
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('opens a thread in a new window through the thread IPC handler', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const openThreadInNewWindow = vi.fn()

    registerAppIpcHandlers(registerOptions({ openThreadInNewWindow }))

    await expect(handlers.get('thread:open-new-window')?.({}, { threadId: 'thread-123' })).resolves.toBeUndefined()
    expect(openThreadInNewWindow).toHaveBeenCalledWith('thread-123')
  })

  it('routes git checkpoint create and restore through the desktop service', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    gitCheckpointMock.createGitCheckpoint.mockResolvedValue({
      ok: true,
      checkpointId: 'gcp_1',
      repositoryRoot: '/tmp/workspace',
      head: 'abc',
      currentBranch: 'main'
    })
    gitCheckpointMock.restoreGitCheckpoint.mockResolvedValue({
      ok: true,
      checkpointId: 'gcp_1',
      repositoryRoot: '/tmp/workspace',
      head: 'abc',
      currentBranch: 'main',
      rescueCheckpointId: null
    })

    registerAppIpcHandlers(registerOptions())

    await expect(handlers.get('git:checkpoint:create')?.({}, {
      workspaceRoot: '/tmp/workspace',
      threadId: 'thr_1'
    })).resolves.toMatchObject({ ok: true, checkpointId: 'gcp_1' })
    expect(gitCheckpointMock.createGitCheckpoint).toHaveBeenCalledWith(expect.objectContaining({
      workspaceRoot: '/tmp/workspace',
      threadId: 'thr_1'
    }))

    await expect(handlers.get('git:checkpoint:restore')?.({}, {
      checkpointId: 'gcp_1'
    })).resolves.toMatchObject({ ok: true, checkpointId: 'gcp_1' })
    expect(gitCheckpointMock.restoreGitCheckpoint).toHaveBeenCalledWith(expect.objectContaining({
      checkpointId: 'gcp_1'
    }))
  })

  it('routes thread handoff IPC calls through parsed desktop service payloads', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const service = threadHandoffServiceMock.threadHandoffService
    service.start.mockResolvedValue({ id: 'op-start' })
    service.retry.mockResolvedValue({ id: 'op-retry' })
    service.get.mockReturnValue([{ id: 'op-get' }])
    service.cancel.mockReturnValue({ id: 'op-cancel' })
    service.remove.mockReturnValue(true)
    service.completeSwitch.mockResolvedValue({ id: 'op-complete' })
    service.failSwitch.mockResolvedValue({ id: 'op-fail' })
    const send = vi.fn()

    registerAppIpcHandlers(registerOptions({
      getMainWindow: () => ({ webContents: { send } }) as never
    }))

    await expect(handlers.get('thread-handoff:start')?.({}, {
      direction: 'to-worktree',
      sourceThreadId: ' thr_1 ',
      sourceWorkspace: ' /tmp/project ',
      localBranch: ' main '
    })).resolves.toEqual({ operation: { id: 'op-start' } })
    expect(service.start).toHaveBeenCalledWith({
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project',
      localBranch: 'main'
    })

    await expect(handlers.get('thread-handoff:retry')?.({}, {
      operationId: ' op_1 '
    })).resolves.toEqual({ operation: { id: 'op-retry' } })
    expect(service.retry).toHaveBeenCalledWith('op_1')

    await expect(handlers.get('thread-handoff:get')?.({}, {
      operationId: ' op_1 '
    })).resolves.toEqual({ operations: [{ id: 'op-get' }] })
    expect(service.get).toHaveBeenCalledWith('op_1')

    await expect(handlers.get('thread-handoff:get')?.({}, undefined)).resolves.toEqual({
      operations: [{ id: 'op-get' }]
    })
    expect(service.get).toHaveBeenLastCalledWith(undefined)

    await expect(handlers.get('thread-handoff:cancel')?.({}, {
      operationId: ' op_1 '
    })).resolves.toEqual({ operation: { id: 'op-cancel' } })
    expect(service.cancel).toHaveBeenCalledWith('op_1')

    await expect(handlers.get('thread-handoff:remove')?.({}, {
      operationId: ' op_1 '
    })).resolves.toEqual({ removed: true })
    expect(service.remove).toHaveBeenCalledWith('op_1')

    await expect(handlers.get('thread-handoff:complete-switch')?.({}, {
      operationId: ' op_1 ',
      targetThreadId: ' thread_next '
    })).resolves.toEqual({ operation: { id: 'op-complete' } })
    expect(service.completeSwitch).toHaveBeenCalledWith({
      operationId: 'op_1',
      targetThreadId: 'thread_next'
    })

    await expect(handlers.get('thread-handoff:fail-switch')?.({}, {
      operationId: ' op_1 ',
      message: ' renderer switch failed '
    })).resolves.toEqual({ operation: { id: 'op-fail' } })
    expect(service.failSwitch).toHaveBeenCalledWith('op_1', 'renderer switch failed')

    await expect(handlers.get('thread-handoff:start')?.({}, {
      direction: 'to-worktree',
      sourceThreadId: 'thr_1',
      sourceWorkspace: '/tmp/project',
      extra: true
    })).rejects.toThrow(/Invalid payload for thread-handoff:start/)
    expect(service.start).toHaveBeenCalledTimes(1)

    const subscribeCalls = service.subscribe.mock.calls as unknown as Array<[(event: unknown) => void]>
    const listener = subscribeCalls[0]?.[0]
    const event = { operation: { id: 'op-event' } }
    listener?.(event)
    expect(send).toHaveBeenCalledWith('thread-handoff:event', event)
  })

  it('saves generated files to a user-selected path', async () => {
    const { dialog } = await import('electron')
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const temp = mkdtempSync(join(tmpdir(), 'analytix-save-as-'))
    const source = join(temp, 'source.png')
    const target = join(temp, 'downloaded.png')
    writeFileSync(source, 'generated-image')
    ;(dialog as unknown as { showSaveDialog: ReturnType<typeof vi.fn> }).showSaveDialog = vi.fn(async () => ({
      canceled: false,
      filePath: target
    }))

    try {
      registerAppIpcHandlers(registerOptions())

      const handler = handlers.get('file:save-as')
      await expect(handler?.({}, {
        sourcePath: source,
        suggestedName: 'source.png',
        mimeType: 'image/png'
      })).resolves.toEqual({ ok: true, path: target })
      expect(readFileSync(target, 'utf8')).toBe('generated-image')
    } finally {
      rmSync(temp, { recursive: true, force: true })
    }
  })

  it('accepts the full settings snapshot emitted by SettingsView auto-apply', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const applySettingsPatch = vi.fn(async () => settings())

    registerAppIpcHandlers(registerOptions({ applySettingsPatch }))

    const payload = { ...settings(), locale: 'zh' as const }
    const handler = handlers.get('settings:set')
    await expect(handler?.({}, payload)).resolves.toEqual(settings())
    expect(applySettingsPatch).toHaveBeenCalledWith(payload)
  })

  it('passes schedule settings patches through to applySettingsPatch', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const applySettingsPatch = vi.fn(async (partial: AppSettingsPatch) => ({
      ...settings(),
      schedule: mergeScheduleSettings(settings().schedule, partial.schedule)
    }))

    registerAppIpcHandlers(registerOptions({ applySettingsPatch }))

    const payload = {
      schedule: {
        enabled: true,
        keepAwake: true,
        tasks: [{
          id: 'task-1',
          title: 'Daily',
          enabled: true,
          prompt: 'Run',
          schedule: { kind: 'manual' as const }
        }]
      }
    }
    const handler = handlers.get('settings:set')
    await expect(handler?.({}, payload)).resolves.toMatchObject({
      schedule: {
        enabled: true,
        keepAwake: true,
        tasks: [{ id: 'task-1', prompt: 'Run' }]
      }
    })
    expect(applySettingsPatch).toHaveBeenCalledWith(payload)
  })

  it('writes MCP config JSON and notifies the runtime apply hook', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-ipc-'))
    const configPath = join(tempRoot, 'mcp.json')
    const onAnalytixMcpConfigWritten = vi.fn(async () => undefined)
    const content = `${JSON.stringify({
      servers: {
        filesystem: {
          command: 'npx',
          args: ['-y', '@modelcontextprotocol/server-filesystem', '/tmp/project']
        }
      }
    }, null, 2)}\n`

    try {
      registerAppIpcHandlers(registerOptions({
        resolveAnalytixConfigPath: () => configPath,
        onAnalytixMcpConfigWritten
      }))

      await expect(handlers.get('analytix:runtime-config:write')?.({}, content)).resolves.toEqual({
        ok: true,
        path: configPath
      })
      expect(readFileSync(configPath, 'utf8')).toBe(content)
      expect(onAnalytixMcpConfigWritten).toHaveBeenCalledWith(configPath, content)
    } finally {
      rmSync(tempRoot, { recursive: true, force: true })
    }
  })

  it('rejects invalid MCP config JSON before writing or applying it', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const tempRoot = mkdtempSync(join(tmpdir(), 'analytix-ipc-'))
    const configPath = join(tempRoot, 'mcp.json')
    const onAnalytixMcpConfigWritten = vi.fn(async () => undefined)

    try {
      registerAppIpcHandlers(registerOptions({
        resolveAnalytixConfigPath: () => configPath,
        onAnalytixMcpConfigWritten
      }))

      await expect(handlers.get('analytix:runtime-config:write')?.({}, '{')).rejects.toThrow(
        /MCP config must be JSON/
      )
      await expect(handlers.get('analytix:runtime-config:write')?.({}, '[]')).rejects.toThrow(
        /MCP config must be a JSON object/
      )
      expect(existsSync(configPath)).toBe(false)
      expect(onAnalytixMcpConfigWritten).not.toHaveBeenCalled()
    } finally {
      rmSync(tempRoot, { recursive: true, force: true })
    }
  })

  it('uses the GUI-managed WeChat bridge for WeChat install handlers', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const configuredSettings = settings()
    configuredSettings.claw.im.weixinBridgeUrl = 'http://127.0.0.1:8787/rpc'
    const store = { load: vi.fn(async () => configuredSettings) }
    const startWeixinInstallQrcode = vi.fn(async () => ({
      ok: false as const,
      message: 'expected test response'
    }))
    const pollWeixinInstall = vi.fn(async () => ({ done: false as const }))

    registerAppIpcHandlers(registerOptions({
      store: store as never,
      startWeixinInstallQrcode,
      pollWeixinInstall
    }))

    await expect(
      handlers.get('claw:im-install:qrcode')?.({}, { provider: 'weixin' })
    ).resolves.toMatchObject({ ok: false })
    await expect(
      handlers.get('claw:im-install:poll')?.({}, { provider: 'weixin', deviceCode: 'device-1' })
    ).resolves.toEqual({ done: false })

    expect(startWeixinInstallQrcode).toHaveBeenCalledWith()
    expect(pollWeixinInstall).toHaveBeenCalledWith('device-1')
  })

  it('commits a completed WeChat login through the account authority and returns key-free metadata', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const syntheticToken = 'synthetic-weixin-main-private-token'
    const pollWeixinInstall = vi.fn(async () => ({
      done: true as const,
      kind: 'weixin' as const,
      accountId: 'weixin-account',
      token: syntheticToken
    }))
    const connect = vi.fn(async () => settings())
    registerAppIpcHandlers(registerOptions({
      pollWeixinInstall,
      imChannelAccountLifecycle: { connect, disconnect: vi.fn(), recover: vi.fn() } as never
    }))

    const result = await handlers.get('claw:im-install:poll')?.({}, {
      provider: 'weixin',
      deviceCode: 'device-1'
    })
    expect(result).toMatchObject({
      done: true,
      kind: 'weixin',
      accountId: 'weixin-account',
      credentialConfigured: true,
      settingsCommitted: true
    })
    expect(JSON.stringify(result)).not.toContain(syntheticToken)
    expect(JSON.stringify(result)).not.toMatch(/sessionKey|credentialRef|valueBase64/)
    expect(connect).toHaveBeenCalledWith(expect.objectContaining({
      scope: expect.objectContaining({
        owner: 'transport', provider: 'weixin', accountId: 'weixin-account',
        purpose: 'transport-weixin-session-key'
      }),
      credential: { kind: 'weixin', sessionKey: syntheticToken },
      channel: expect.objectContaining({
        provider: 'weixin',
        platformAccount: expect.objectContaining({ kind: 'weixin', accountId: 'weixin-account' })
      })
    }))
  })

  it('registers a dedicated Telegram bot token install handler', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')

    registerAppIpcHandlers(registerOptions())

    await expect(
      handlers.get('claw:im-install:telegram-token')?.({}, {
        botToken: 'not-a-token',
        allowedChatIds: '1001'
      })
    ).resolves.toMatchObject({
      ok: false,
      code: 'invalid_format'
    })
  })

  it('routes schedule task IPC calls to the Schedule runtime', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const scheduleRuntime = {
      status: vi.fn(async () => ({
        internalServerRunning: true,
        internalUrl: 'http://127.0.0.1:8788',
        runningTaskIds: ['task-1'],
        powerSaveBlockerActive: true
      })),
      runTask: vi.fn(async (taskId: string) => ({ ok: true as const, taskId, message: 'Started' })),
      createScheduledTaskFromText: vi.fn(async () => ({
        kind: 'created' as const,
        taskId: 'task-2',
        title: 'Reminder',
        scheduleAt: '2026-06-03T09:00:00.000+08:00',
        confirmationText: 'Scheduled.'
      }))
    }
    registerAppIpcHandlers(registerOptions({
      getScheduleRuntime: () => scheduleRuntime as never
    }))

    await expect(handlers.get('schedule:status')?.({})).resolves.toMatchObject({
      internalServerRunning: true,
      runningTaskIds: ['task-1'],
      powerSaveBlockerActive: true
    })
    await expect(handlers.get('schedule:task:run')?.({}, 'task-1')).resolves.toMatchObject({
      ok: true,
      taskId: 'task-1'
    })
    await expect(
      handlers.get('schedule:task:create-from-text')?.({}, {
        text: 'Remind me tomorrow.',
        workspaceRoot: '/tmp/schedule',
        clawChannelId: 'channel-1',
        modelHint: 'deepseek-v4-flash',
        mode: 'plan'
      })
    ).resolves.toMatchObject({
      kind: 'created',
      taskId: 'task-2'
    })

    expect(scheduleRuntime.runTask).toHaveBeenCalledWith('task-1')
    expect(scheduleRuntime.createScheduledTaskFromText).toHaveBeenCalledWith('Remind me tomorrow.', {
      workspaceRoot: '/tmp/schedule',
      clawChannelId: 'channel-1',
      modelHint: 'deepseek-v4-flash',
      mode: 'plan'
    })
  })

  it('routes desktop command IPC calls to the focused window and web contents', async () => {
    const { registerAppIpcHandlers } = await import('./register-app-ipc-handlers')
    const webContents = {
      undo: vi.fn(),
      redo: vi.fn(),
      cut: vi.fn(),
      copy: vi.fn(),
      paste: vi.fn(),
      selectAll: vi.fn(),
      reload: vi.fn(),
      getZoomLevel: vi.fn(() => 0),
      setZoomLevel: vi.fn(),
      toggleDevTools: vi.fn()
    }
    const mainWindow = {
      isDestroyed: vi.fn(() => false),
      webContents,
      minimize: vi.fn(),
      isMaximized: vi.fn(() => false),
      maximize: vi.fn(),
      unmaximize: vi.fn(),
      isFullScreen: vi.fn(() => false),
      setFullScreen: vi.fn(),
      close: vi.fn()
    }

    registerAppIpcHandlers(registerOptions({
      getMainWindow: () => mainWindow as never
    }))

    const handler = handlers.get('desktop:command')
    await handler?.({ sender: webContents }, 'copy')
    await handler?.({ sender: webContents }, 'zoomIn')
    await handler?.({ sender: webContents }, 'toggleMaximize')
    await handler?.({ sender: webContents }, 'toggleFullscreen')
    await handler?.({ sender: webContents }, 'close')

    expect(webContents.copy).toHaveBeenCalledTimes(1)
    expect(webContents.setZoomLevel).toHaveBeenCalledWith(1)
    expect(mainWindow.maximize).toHaveBeenCalledTimes(1)
    expect(mainWindow.setFullScreen).toHaveBeenCalledWith(true)
    expect(mainWindow.close).toHaveBeenCalledTimes(1)
  })
})


it('desktop reload consults the live window navigation gate while other commands retain their behavior', async () => {
  const contents = { reload: vi.fn(), copy: vi.fn() }
  const main = { isDestroyed: () => false, webContents: contents }
  const canNavigateWindow = vi.fn(() => false)
  registerAppIpcHandlers(registerOptions({ getMainWindow: () => main as never, canNavigateWindow }))
  const command = handlers.get('desktop:command')!
  await command({ sender: {} }, 'reload')
  expect(canNavigateWindow).toHaveBeenCalledExactlyOnceWith(contents)
  expect(contents.reload).not.toHaveBeenCalled()
  await command({ sender: {} }, 'copy')
  expect(contents.copy).toHaveBeenCalledOnce()
  canNavigateWindow.mockReturnValue(true)
  await command({ sender: {} }, 'reload')
  expect(contents.reload).toHaveBeenCalledOnce()
})
