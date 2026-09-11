import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
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

describe('BrowserSettingsSection and ComputerUseSettingsSection', () => {
  it('renders the Codex-style browser settings and high permission defaults', () => {
    const html = renderToStaticMarkup(createElement(BrowserSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Local URL open target')
    expect(html).toContain('Always include')
    expect(html).toContain('Always allow')
    expect(html).toContain('Enable full CDP access')
    expect(html).toContain('No site-specific permissions yet')
  })

  it('renders the Codex-style computer control page with analytix-computer-use', () => {
    const html = renderToStaticMarkup(createElement(ComputerUseSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Computer control')
    expect(html).toContain('Any app')
    expect(html).toContain('analytix-computer-use')
    expect(html).toContain('Google Chrome')
    expect(html).not.toContain('Lock screen operation')
    expect(html).toContain('Always-allowed apps')
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
    expect(html).toContain('History')
    expect(html).toContain('Downloads')
    expect(html).toContain('Uploads')
    expect(html).toContain('Always ask')
    expect(html).toContain('Enable full CDP access')
    expect(html).toContain('No site-specific permissions yet')
    expect(html).toContain('disabled=""')
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
