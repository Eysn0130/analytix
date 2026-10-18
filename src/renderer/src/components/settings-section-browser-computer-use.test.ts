// @vitest-environment jsdom
import { act, createElement, type ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ComputerUsePermissions, ModelCapabilityProbeResult } from '@shared/analytix-api'
import {
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings
} from '@shared/app-settings'
import { BrowserSettingsSection } from './settings-section-browser'
import {
  ComputerUseChromeSettingsDetail,
  ComputerUseSettingsSection,
  requestComputerUsePermissionsIfNeeded
} from './settings-section-computer-use'

const labels: Record<string, string> = {
  browser: 'Browser',
  browserPageSubtitle: 'Manage the browser',
  browserPageSubtitleComputerUse: 'Computer control settings',
  browserBuiltInControl: 'Browser',
  browserBuiltInControlDesc: 'Allow Analytix control',
  browserGeneralTitle: 'General',
  browserLocalUrlTarget: 'Local URL open target',
  browserLocalUrlTargetDesc: 'Local dev target',
  browserLocalUrlTargetAnalytix: 'Analytix',
  browserLocalUrlTargetSystem: 'System browser',
  browserClearData: 'Browsing data',
  browserClearDataDescCompact: 'Clear history and cache',
  browserClearAllData: 'Clear all browsing data',
  browserClearDataUnavailable: 'Clear unavailable',
  browserAnnotationScreenshots: 'Annotation screenshots',
  browserAnnotationScreenshotsDesc: 'Screenshots help',
  browserAnnotationScreenshotsAlways: 'Always include',
  browserAnnotationScreenshotsNecessary: 'Only on selection',
  browserAnnotationScreenshotsOff: 'Off',
  browserPermissionsSection: 'Permissions',
  browserApproval: 'Approval',
  browserApprovalDesc: 'Choose approval',
  browserLearnMore: 'Learn more',
  browserLearnMoreUnavailable: 'Help unavailable',
  browserApprovalAlwaysAllow: 'Always allow',
  browserApprovalAlwaysAsk: 'Always ask',
  browserSitePermissionsTitle: 'Site permissions',
  browserSitePermissionsSubtitle: 'Override defaults',
  browserSitePermissionsAdd: 'Add',
  browserSitePermissionsAddUnavailable: 'Add unavailable',
  browserSitePermissionsEmpty: 'No site-specific permissions yet',
  browserSitePermissionAlwaysAllow: 'Global allow',
  browserDeveloperMode: 'Developer mode',
  browserRiskElevated: 'Elevated risk',
  browserFullCdp: 'Enable full CDP access',
  browserFullCdpDesc: 'Full CDP description',
  computerUseTitle: 'Computer control',
  computerUseSubtitle: 'Manage other apps',
  computerUseControlSection: 'Control',
  computerUseAnyApp: 'Any app',
  computerUseAnyAppDesc: 'Use analytix-computer-use',
  computerUseBackendUnavailable: 'Backend unavailable',
  computerUseChrome: 'Google Chrome',
  computerUseChromeConnected: 'Connected',
  computerUseChromeDisconnected: 'Disconnected',
  computerUseChromeConfigured: 'Configured, pending connection check',
  computerUseChromeExtensionMissing: 'Extension missing',
  computerUseChromeExtensionDisabled: 'Extension disabled',
  computerUseChromeNativeHostMissing: 'Native host missing',
  computerUseChromeNativeHostInvalid: 'Native host invalid',
  computerUseChromeDiagnosticsUnavailable: 'Diagnostics unavailable',
  computerUseChromeUnsupported: 'Unsupported',
  computerUseChromeControlOff: 'Chrome off',
  computerUseChromeConnectedShort: 'Connected',
  computerUseChromeDisconnectedShort: 'Not connected',
  computerUseChromeConfiguredShort: 'Pending check',
  computerUseChromeExtensionMissingShort: 'Missing',
  computerUseChromeExtensionDisabledShort: 'Disabled',
  computerUseChromeNativeHostMissingShort: 'Host missing',
  computerUseChromeNativeHostInvalidShort: 'Host invalid',
  computerUseChromeDiagnosticsUnavailableShort: 'Diagnostics unavailable',
  computerUseChromeUnsupportedShort: 'Unsupported',
  computerUseChromeControlOffShort: 'Off',
  computerUseManage: 'Manage',
  computerUseBack: 'Back',
  computerUseChromeBreadcrumbParent: 'Computer control',
  computerUseChromeReinstallExtension: 'Reinstall extension',
  computerUseChromeRemoveExtension: 'Remove extension',
  computerUseChromeOpenExtensionSettings: 'Open extension settings',
  computerUseChromeExtensionActionUnavailable: 'Extension action unavailable',
  computerUseChromeExtensionOpenFailed: 'Could not open extension page',
  computerUseChromeDiagnosticsTitle: 'Connection diagnostics',
  computerUseChromeExtensionStatus: 'Extension',
  computerUseChromeNativeHostStatus: 'Native Host',
  computerUseChromeConnectionProbe: 'Connection probe',
  computerUseChromeStatusApiMissing: 'Chrome Browser Use diagnostics API missing',
  computerUseChromeExtensionStatus_enabled: 'Enabled',
  computerUseChromeExtensionStatus_disabled: 'Disabled',
  computerUseChromeExtensionStatus_missing: 'Not installed',
  computerUseChromeExtensionStatus_unknown: 'Unknown',
  computerUseChromeExtensionStatus_error: 'Error',
  computerUseChromeNativeHostStatus_configured: 'Configured',
  computerUseChromeNativeHostStatus_missing: 'Missing',
  computerUseChromeNativeHostStatus_invalid: 'Invalid',
  computerUseChromeNativeHostStatus_unsupported: 'Unsupported',
  computerUseChromeNativeHostStatus_error: 'Error',
  computerUseChromeConnectionProbe_unavailable: 'Not wired',
  computerUseChromeConnectionProbe_passed: 'Passed',
  computerUseChromeConnectionProbe_failed: 'Failed',
  computerUseChromeUserDataDirectory: 'User data directory',
  computerUseChromeSelectedProfile: 'Selected profile',
  computerUseChromePreferencesPath: 'Preferences',
  computerUseChromeExtensionPath: 'Extension path',
  computerUseChromeNativeHostManifest: 'Manifest',
  computerUseChromeWindowsRegistry: 'Windows registry',
  computerUseChromeRegistryManifest: 'Registry manifest',
  computerUseChromeNativeHostPath: 'Host path',
  computerUseChromeAllowedOrigins: 'Allowed origins',
  computerUseChromeBrowserClient: 'Browser client',
  computerUseChromeCheckedAt: 'Checked at',
  computerUseChromePermissionsTitle: 'Permissions',
  computerUseChromeHistory: 'History',
  computerUseChromeHistoryDesc: 'Choose history approval',
  computerUseChromeDownload: 'Downloads',
  computerUseChromeDownloadDesc: 'Choose download approval',
  computerUseChromeUpload: 'Uploads',
  computerUseChromeUploadDesc: 'Choose upload approval',
  computerUseChromePermissionUnavailable: 'Permission unavailable',
  computerUseLockOperation: 'Lock screen operation',
  computerUseLockOperationDesc: 'Allow locked operation',
  computerUseAllowedAppsAdd: 'Add',
  computerUseAllowedAppsAddUnavailable: 'Allowed app unavailable',
  computerUseAlwaysAllowedApps: 'Always-allowed apps',
  computerUseAllowedAppsEmpty: 'None',
  computerUsePermissions: 'System permissions',
  computerUsePermissionsDesc: 'Permission description',
  computerUseBackendPreferred: 'Preferred backend',
  computerUseRecheck: 'Re-check',
  computerUseAccessibility: 'Accessibility',
  computerUseScreenRecording: 'Screen Recording',
  computerUseGrantAccessibility: 'Grant Accessibility',
  computerUseGrantScreenRecording: 'Grant Screen Recording',
  computerUsePermission_granted: 'granted',
  computerUsePermission_denied: 'denied',
  computerUsePermission_unknown: 'unknown',
  computerUseRestartHint: 'Restart hint',
  computerUseModeAuto: 'Auto',
  computerUseModeAlways: 'Always allow',
  computerUseModeOff: 'Off',
  computerUseVisionTitle: 'Vision model and routing',
  computerUseVisionSubtitle: 'Keeps model settings',
  computerUseRouteStatus: 'Current route',
  computerUsePrimaryModel: 'Primary model',
  computerUsePrimaryCapabilities: 'Primary capabilities',
  computerUseImageSupported: 'Image supported',
  computerUseImageTextOnly: 'Text only',
  computerUseToolSupported: 'Tool calling supported',
  computerUseToolUnsupported: 'Tool calling unsupported',
  visionBridgeTitle: 'Vision bridge',
  visionBridgeDesc: 'Use separate vision model',
  visionBridgeMode: 'Bridge availability',
  visionBridgeModeDesc: 'Bridge mode description',
  visionBridgeProvider: 'Bridge provider',
  visionBridgeProviderDesc: 'Choose provider',
  visionBridgeProviderActive: 'Use primary provider',
  visionBridgeModel: 'Bridge model',
  visionBridgeModelDesc: 'Model id',
  visionBridgeNotReady: 'Not ready',
  visionBridgeOff: 'Off',
  computerUseRoute: 'Control route',
  computerUseRouteNative: 'Native vision model',
  computerUseRouteBridge: 'Vision bridge',
  computerUseRouteUnavailable: 'No available vision route',
  computerUseCapabilityProbe: 'Capability probe',
  computerUseCapabilityProbeDesc: 'Runs probe',
  computerUseCapabilityProbeUnavailable: 'Probe API unavailable',
  computerUseCapabilityProbeMissing: 'Probe config missing',
  computerUseRunProbe: 'Run probe',
  computerUseProbeResult: 'Status: {{status}}; image: {{image}}; tool: {{tool}}; screenshot result: {{screenshot}}',
  settingsFeatureUnavailable: 'Unavailable'
}

function t(key: string, options?: Record<string, unknown>): string {
  const template = labels[key] ?? key
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(options?.[name] ?? ''))
}

function baseCtx(): Record<string, unknown> {
  const provider = defaultModelProviderSettings()
  return {
    t,
    form: {
      provider
    },
    provider,
    analytix: defaultAnalytixRuntimeSettings(),
    updateAnalytix: () => undefined,
    openSettingsSection: () => undefined,
    selectControlClass: 'select',
    openPlugins: () => undefined,
    showTopNotice: () => undefined,
    refreshAnalytixDiagnostics: () => undefined,
    runtimeDiagnosticsBusy: false,
    runtimeInfo: {
      capabilities: {
        web: {
          status: 'available',
          fetch: { status: 'available' },
          search: { status: 'disabled' },
          provider: 'brave-search'
        }
      }
    },
    toolDiagnostics: {
      mcpServers: [{ name: 'browser' }]
    }
  }
}

afterEach(() => vi.unstubAllGlobals())

async function withSettings(element: ReactElement, run: (host: HTMLDivElement) => Promise<void>): Promise<void> {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  try {
    await act(async () => root.render(element))
    await run(host)
  } finally {
    await act(async () => root.unmount())
    host.remove()
  }
}

async function changeSelect(host: HTMLDivElement, label: string, value: string): Promise<void> {
  const select = host.querySelector<HTMLSelectElement>(`select[aria-label="${label}"]`)
  expect(select).not.toBeNull()
  await act(async () => {
    select!.value = value
    select!.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

async function clickButton(host: HTMLDivElement, label: string, toggle = false): Promise<void> {
  const button = toggle
    ? host.querySelector<HTMLButtonElement>(`button[role="switch"][aria-label="${label}"]`)
    : Array.from(host.querySelectorAll('button')).find((item) => item.textContent?.trim() === label)
  expect(button).toBeDefined()
  expect(button).not.toBeNull()
  expect(button!.disabled).toBe(false)
  await act(async () => button!.click())
}

describe('BrowserSettingsSection and ComputerUseSettingsSection', () => {
  it('renders the Codex-style browser settings and high permission defaults', () => {
    const html = renderToStaticMarkup(createElement(BrowserSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Local URL open target')
    expect(html).toContain('Always include')
    expect(html).toContain('Always allow')
    expect(html).toContain('Enable full CDP access')
    expect(html).toContain('No site-specific permissions yet')
    expect(html).not.toContain('Clear all browsing data')
    expect(html).not.toContain('Clear history and cache')
    expect(html).not.toMatch(/>Add<\/button>/)
    expect(html).not.toContain('Learn more')
    expect(html).toContain('Computer control settings')
  })

  it('preserves saved browser permissions after removing unavailable controls', () => {
    const ctx = baseCtx()
    const runtime = defaultAnalytixRuntimeSettings()
    runtime.browserUse.sitePermissions = [{ origin: 'https://allowed.invalid', approvalMode: 'neverAsk',
      downloadApprovalMode: 'alwaysAsk', uploadApprovalMode: 'alwaysAsk', fullCdpAccess: false }]
    ctx.analytix = runtime
    const before = JSON.stringify(runtime.browserUse)
    const html = renderToStaticMarkup(createElement(BrowserSettingsSection, { ctx }))

    expect(html).toContain('https://allowed.invalid')
    expect(html).toContain('Global allow')
    expect(html).toContain('Always allow')
    expect(html).not.toContain('No site-specific permissions yet')
    expect(JSON.stringify(runtime.browserUse)).toBe(before)
  })

  it('renders the Codex-style computer control page with analytix-computer-use', () => {
    const html = renderToStaticMarkup(createElement(ComputerUseSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Computer control')
    expect(html).toContain('Any app')
    expect(html).toContain('analytix-computer-use')
    expect(html).toContain('Google Chrome')
    expect(html).not.toContain('Lock screen operation')
    expect(html).not.toContain('Always-allowed apps')
    expect(html).not.toMatch(/>Add<\/button>/)
    expect(html).toContain('Vision model and routing')
    expect(html).toContain('Capability probe')
  })

  it('renders the Google Chrome management detail page without faking unavailable APIs', () => {
    const runtime = defaultAnalytixRuntimeSettings()
    const html = renderToStaticMarkup(
      createElement(ComputerUseChromeSettingsDetail, {
        ctx: baseCtx(),
        browserUse: runtime.browserUse,
        chromeStatus: null,
        onBack: () => undefined,
        onBrowserUseChange: () => undefined
      })
    )

    expect(html).toContain('Computer control')
    expect(html).toContain('Google Chrome')
    expect(html).toContain('Reinstall extension')
    expect(html).toContain('Open extension settings')
    expect(html).toContain('Connection diagnostics')
    expect(html).toContain('Chrome Browser Use diagnostics API missing')
    expect(html).toContain('Permissions')
    expect(html).not.toContain('History')
    expect(html).not.toContain('Downloads')
    expect(html).not.toContain('Uploads')
    expect(html).not.toContain('Learn more')
    expect(html).not.toMatch(/>Add<\/button>/)
    expect(html).toContain('Always ask')
    expect(html).toContain('Enable full CDP access')
    expect(html).toContain('No site-specific permissions yet')
    expect(html).not.toContain('disabled=""')
  })

  it('keeps Browser setting actions and navigation without replacing saved permissions', async () => {
    const ctx = baseCtx()
    const runtime = defaultAnalytixRuntimeSettings()
    const saved = { ...runtime.browserUse, approvalMode: 'alwaysAsk' as const, fullCdpAccess: false,
      chromeControlEnabled: false, sitePermissions: [{ origin: 'https://allowed.invalid', approvalMode: 'neverAsk' as const,
        downloadApprovalMode: 'alwaysAsk' as const, uploadApprovalMode: 'alwaysAsk' as const, fullCdpAccess: false }] }
    const before = JSON.stringify(saved)
    const update = vi.fn(), navigate = vi.fn(), notice = vi.fn()
    ctx.analytix = { ...runtime, browserUse: saved }
    ctx.updateAnalytix = update
    ctx.openSettingsSection = navigate
    ctx.showTopNotice = notice
    await withSettings(createElement(BrowserSettingsSection, { ctx }), async (host) => {
      await changeSelect(host, 'Local URL open target', 'system')
      await changeSelect(host, 'Annotation screenshots', 'off')
      await changeSelect(host, 'Approval', 'neverAsk')
      await clickButton(host, 'Enable full CDP access', true)
      await clickButton(host, 'Browser', true)
      await clickButton(host, 'Computer control settings')
      expect(update.mock.calls.map(([patch]) => patch)).toEqual([
        { browserUse: { ...saved, localUrlOpenTarget: 'system' } },
        { browserUse: { ...saved, annotationScreenshotsMode: 'off' } },
        { browserUse: { ...saved, approvalMode: 'neverAsk' } },
        { browserUse: { ...saved, fullCdpAccess: true } },
        { browserUse: { ...saved, enabled: false } }
      ])
      expect(navigate).toHaveBeenCalledExactlyOnceWith('computerUse')
      expect(notice).not.toHaveBeenCalled()
      expect(JSON.stringify(saved)).toBe(before)
      expect(host.textContent).toContain('https://allowed.invalid')
    })
  })

  it('keeps Chrome approval, CDP, extension and Back actions with saved configuration', async () => {
    const ctx = baseCtx()
    const runtime = defaultAnalytixRuntimeSettings()
    const saved = { ...runtime.browserUse, approvalMode: 'alwaysAsk' as const, fullCdpAccess: false,
      sitePermissions: [{ origin: 'https://allowed.invalid', approvalMode: 'neverAsk' as const,
        downloadApprovalMode: 'alwaysAsk' as const, uploadApprovalMode: 'alwaysAsk' as const, fullCdpAccess: false }] }
    const before = JSON.stringify(saved)
    const change = vi.fn(), back = vi.fn(), notice = vi.fn()
    const openExtension = vi.fn().mockResolvedValue({ ok: true })
    ctx.showTopNotice = notice
    vi.stubGlobal('analytix', { app: { openChromeBrowserUseExtensionPage: openExtension } })
    await withSettings(createElement(ComputerUseChromeSettingsDetail, {
      ctx, browserUse: saved, chromeStatus: null, onBack: back, onBrowserUseChange: change
    }), async (host) => {
      await changeSelect(host, 'Approval', 'neverAsk')
      await clickButton(host, 'Enable full CDP access', true)
      await clickButton(host, 'Reinstall extension')
      await clickButton(host, 'Open extension settings')
      await clickButton(host, 'Back')
      expect(change.mock.calls.map(([patch]) => patch)).toEqual([{ approvalMode: 'neverAsk' }, { fullCdpAccess: true }])
      expect(openExtension.mock.calls).toEqual([['webstore'], ['settings']])
      expect(back).toHaveBeenCalledOnce()
      expect(notice).not.toHaveBeenCalled()
      expect(JSON.stringify(saved)).toBe(before)
      expect(host.textContent).toContain('https://allowed.invalid')
    })
  })

  it('keeps Computer Use enable, Chrome management and the explicit capability probe', async () => {
    const ctx = baseCtx()
    const runtime = defaultAnalytixRuntimeSettings()
    const provider = defaultModelProviderSettings()
    const profile = { ...provider.providers[0], baseUrl: 'https://provider.invalid/v1' }
    provider.providers = [profile]
    provider.activeProviderId = profile.id
    const saved = { ...runtime, providerId: profile.id, model: 'fixture-model',
      computerUse: { ...runtime.computerUse, enabled: false, mode: 'off' as const, maxActionsPerTurn: 7 },
      browserUse: { ...runtime.browserUse, chromeControlEnabled: false },
      visionBridge: { ...runtime.visionBridge, enabled: false } }
    const before = JSON.stringify(saved)
    const update = vi.fn(), notice = vi.fn()
    ctx.analytix = saved
    ctx.form = { provider }
    ctx.updateAnalytix = update
    ctx.showTopNotice = notice
    let permissions: ComputerUsePermissions = { platform: 'darwin', supported: true, needsPermission: true,
      accessibility: 'denied', screenRecording: 'denied', accessibilityNeedsRestart: false }
    const request = vi.fn(async (kind: 'accessibility' | 'screenRecording') => {
      permissions = { ...permissions, [kind]: 'granted' }
      return { ...permissions }
    })
    const result: ModelCapabilityProbeResult = { ok: true, key: 'fixture', providerId: profile.id, model: saved.model,
      endpointFormat: profile.endpointFormat, sanitizedBaseUrl: profile.baseUrl,
      probedAt: '2026-10-10T00:00:00.000Z', staleAfter: '2026-10-11T00:00:00.000Z',
      status: 'supported', imageInput: 'supported', toolCalling: 'supported', toolResultImage: 'supported' }
    const probe = vi.fn().mockResolvedValue(result)
    vi.stubGlobal('analytix', { app: { getComputerUsePermissions: async () => ({ ...permissions }),
      requestComputerUsePermission: request }, runtime: { probeModelCapabilities: probe } })
    await withSettings(createElement(ComputerUseSettingsSection, { ctx }), async (host) => {
      expect(request).not.toHaveBeenCalled()
      expect(probe).not.toHaveBeenCalled()
      await clickButton(host, 'Any app', true)
      await clickButton(host, 'Google Chrome', true)
      expect(update.mock.calls.map(([patch]) => patch)).toEqual([
        { computerUse: { ...saved.computerUse, enabled: true, mode: 'always' } },
        { browserUse: { ...saved.browserUse, chromeControlEnabled: true } }
      ])
      expect(request.mock.calls).toEqual([['accessibility'], ['screenRecording']])
      await clickButton(host, 'Run probe')
      expect(probe).toHaveBeenCalledExactlyOnceWith({ providerId: profile.id, model: saved.model,
        baseUrl: profile.baseUrl, endpointFormat: profile.endpointFormat })
      await clickButton(host, 'Manage')
      expect(host.textContent).toContain('Connection diagnostics')
      await clickButton(host, 'Back')
      expect(host.textContent).toContain('Vision model and routing')
      expect(request).toHaveBeenCalledTimes(2)
      expect(notice).not.toHaveBeenCalled()
      expect(JSON.stringify(saved)).toBe(before)
    })
  })

  it('renders concrete Chrome Browser Use diagnostics without marking pending setup connected', () => {
    const runtime = defaultAnalytixRuntimeSettings()
    const html = renderToStaticMarkup(
      createElement(ComputerUseChromeSettingsDetail, {
        ctx: baseCtx(),
        browserUse: runtime.browserUse,
        chromeStatus: {
          platform: 'win32',
          checkedAt: '2026-07-05T00:00:00.000Z',
          extensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
          nativeHostName: 'top.analytix.codexextension',
          connected: false,
          state: 'configuredNotVerified',
          reason: 'Extension and Native Host are configured, but no handshake has passed.',
          extension: {
            status: 'enabled',
            extensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
            userDataDirectory: 'C:\\Users\\sun\\AppData\\Local\\Google\\Chrome\\User Data',
            selectedProfileDirectory: 'Default',
            selectedProfilePath: 'C:\\Users\\sun\\AppData\\Local\\Google\\Chrome\\User Data\\Default',
            profiles: [{
              profileDirectory: 'Default',
              profilePath: 'C:\\Users\\sun\\AppData\\Local\\Google\\Chrome\\User Data\\Default',
              preferencesPath: 'C:\\Users\\sun\\AppData\\Local\\Google\\Chrome\\User Data\\Default\\Preferences',
              extensionPath: 'C:\\Users\\sun\\AppData\\Local\\Google\\Chrome\\User Data\\Default\\Extensions\\bccibejdpcjcdlcnbpempcpjgladapgk',
              selected: true,
              installed: true,
              registered: true,
              enabled: true,
              disabled: false,
              state: 1,
              disableReasons: [],
              versions: ['1.0.0']
            }]
          },
          nativeHost: {
            status: 'configured',
            expectedHostName: 'top.analytix.codexextension',
            expectedExtensionId: 'bccibejdpcjcdlcnbpempcpjgladapgk',
            expectedOrigin: 'chrome-extension://bccibejdpcjcdlcnbpempcpjgladapgk/',
            manifestPath: 'C:\\Users\\sun\\AppData\\Local\\Analytix\\extension\\top.analytix.codexextension.json',
            registryKey: 'HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\top.analytix.codexextension',
            registryManifestPath: 'C:\\Users\\sun\\AppData\\Local\\Analytix\\extension\\top.analytix.codexextension.json',
            actualHostName: 'top.analytix.codexextension',
            actualType: 'stdio',
            actualHostPath: 'C:\\Users\\sun\\AppData\\Local\\Analytix\\extension\\extension-host.exe',
            hostPathExists: true,
            allowedOrigins: ['chrome-extension://bccibejdpcjcdlcnbpempcpjgladapgk/']
          },
          connectionProbe: {
            status: 'unavailable',
            problem: 'No main-process handshake adapter is wired.'
          }
        },
        onBack: () => undefined,
        onBrowserUseChange: () => undefined
      })
    )

    expect(html).toContain('Pending check')
    expect(html).not.toContain('Connected</span>')
    expect(html).toContain('Windows registry')
    expect(html).toContain('Host path')
    expect(html).toContain('Allowed origins')
    expect(html).toContain('chrome-extension://bccibejdpcjcdlcnbpempcpjgladapgk/')
    expect(html).toContain('No main-process handshake adapter is wired.')
  })

  it('preserves the original vision bridge model controls when enabled', () => {
    const ctx = baseCtx()
    const runtime = defaultAnalytixRuntimeSettings()
    ctx.analytix = {
      ...runtime,
      visionBridge: {
        ...runtime.visionBridge,
        enabled: true,
        providerId: '',
        model: 'mimo-v2.5'
      }
    }
    const html = renderToStaticMarkup(createElement(ComputerUseSettingsSection, { ctx }))

    expect(html).toContain('Bridge availability')
    expect(html).toContain('Bridge provider')
    expect(html).toContain('Bridge model')
    expect(html).toContain('mimo-v2.5')
  })

  it('requests macOS computer-control permissions from the unified enable path', async () => {
    const requests: string[] = []
    const denied = {
      platform: 'darwin' as const,
      supported: true,
      needsPermission: true,
      accessibility: 'denied' as const,
      screenRecording: 'denied' as const,
      accessibilityNeedsRestart: false
    }
    const afterAccessibility = {
      ...denied,
      accessibility: 'granted' as const
    }

    await requestComputerUsePermissionsIfNeeded({
      getComputerUsePermissions: async () => denied,
      requestComputerUsePermission: async (kind) => {
        requests.push(kind)
        return kind === 'accessibility'
          ? afterAccessibility
          : { ...afterAccessibility, screenRecording: 'granted' as const }
      }
    })

    expect(requests).toEqual(['accessibility', 'screenRecording'])
  })

  it('does not request OS permissions on platforms without a permission gate', async () => {
    const requests: string[] = []

    await requestComputerUsePermissionsIfNeeded({
      getComputerUsePermissions: async () => ({
        platform: 'win32' as const,
        supported: true,
        needsPermission: false,
        accessibility: 'granted',
        screenRecording: 'granted',
        accessibilityNeedsRestart: false
      }),
      requestComputerUsePermission: async (kind) => {
        requests.push(kind)
        return {
          platform: 'win32' as const,
          supported: true,
          needsPermission: false,
          accessibility: 'granted',
          screenRecording: 'granted',
          accessibilityNeedsRestart: false
        }
      }
    })

    expect(requests).toEqual([])
  })
})
