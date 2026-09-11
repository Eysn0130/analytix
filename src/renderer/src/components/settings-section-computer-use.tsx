import { useEffect, useRef, useState, type ReactElement, type ReactNode } from 'react'
import type {
  AnalytixBrowserApprovalMode,
  AnalytixBrowserUseSettingsV1,
  AnalytixComputerUseSettingsV1,
  AnalytixVisionBridgeMode,
  AnalytixVisionBridgeSettingsV1,
  ModelProviderProfileV1
} from '@shared/app-settings'
import {
  DEFAULT_MODEL_PROVIDER_ID,
  defaultAnalytixBrowserUseSettings,
  defaultAnalytixComputerUseSettings,
  defaultAnalytixVisionBridgeSettings,
  defaultModelProviderSettings,
  modelCapabilityProbeKey
} from '@shared/app-settings'
import type {
  ChromeBrowserUseConnectionState,
  ChromeBrowserUseStatus,
  ComputerUseDoctorResult,
  ComputerUsePermissionKind,
  ComputerUsePermissions,
  ComputerUsePermissionState,
  ModelCapabilityProbeResult
} from '@shared/analytix-api'
import { ArrowLeft, ChevronRight, Loader2, Plus, RefreshCw, ShieldAlert } from 'lucide-react'
import computerUseIcon from '../../../../plugins/analytix-computer-use/assets/icon.png'
import chromeIcon from '../../../asset/img/chrome-production-large.png'
import {
  CodexEmptyState,
  CodexSelect,
  CodexSettingsPage,
  CodexSettingsPanel,
  CodexSettingsRow,
  CodexSettingsSection,
  CodexToggle,
  CodexToolbarButton
} from './settings-codex-list'

type ComputerUsePermissionApi = {
  getComputerUsePermissions?: () => Promise<ComputerUsePermissions>
  requestComputerUsePermission?: (kind: ComputerUsePermissionKind) => Promise<ComputerUsePermissions>
}

type ChromeConnectionUiState = ChromeBrowserUseConnectionState | 'disabled'

function normalizeModelId(model: string | undefined): string {
  const normalized = model?.trim().toLowerCase() ?? ''
  return normalized === 'auto' ? '' : normalized
}

export async function requestComputerUsePermissionsIfNeeded(
  appApi: ComputerUsePermissionApi | undefined
): Promise<void> {
  const getComputerUsePermissions = appApi?.getComputerUsePermissions
  const requestComputerUsePermission = appApi?.requestComputerUsePermission
  if (typeof getComputerUsePermissions !== 'function' || typeof requestComputerUsePermission !== 'function') return
  const permissions = await getComputerUsePermissions()
  if (!permissions.needsPermission) return
  let latest = permissions
  if (latest.accessibility !== 'granted' && !latest.accessibilityNeedsRestart) {
    latest = await requestComputerUsePermission('accessibility')
  }
  if (latest.screenRecording !== 'granted') {
    await requestComputerUsePermission('screenRecording')
  }
}

function permissionBadgeClass(state: ComputerUsePermissionState): string {
  if (state === 'granted') return 'bg-emerald-500 text-white'
  if (state === 'denied') return 'bg-red-500 text-white'
  return 'bg-ds-subtle text-ds-muted'
}

function PermissionBadge({
  label,
  state,
  t
}: {
  label: string
  state: ComputerUsePermissionState
  t: (key: string) => string
}): ReactElement {
  return (
    <span className={`rounded-full px-2.5 py-1 text-[12px] font-semibold ${permissionBadgeClass(state)}`}>
      {label} · {t(`computerUsePermission_${state}`)}
    </span>
  )
}

function ComputerUsePermissionPanel({ t }: { t: (key: string) => string }): ReactElement | null {
  const [permissions, setPermissions] = useState<ComputerUsePermissions | null>(null)

  const refresh = (): void => {
    void window.analytix?.app?.getComputerUsePermissions?.().then(setPermissions).catch(() => undefined)
  }

  useEffect(() => {
    refresh()
  }, [])

  if (!permissions?.needsPermission) return null

  const request = (kind: ComputerUsePermissionKind): void => {
    void window.analytix?.app
      ?.requestComputerUsePermission?.(kind)
      .then(setPermissions)
      .catch(() => undefined)
  }

  return (
    <div className="mt-3">
      <CodexSettingsPanel>
        <CodexSettingsRow
          title={t('computerUsePermissions')}
          description={permissions.accessibilityNeedsRestart ? t('computerUseRestartHint') : t('computerUsePermissionsDesc')}
          control={
            <div className="flex max-w-[360px] flex-wrap justify-end gap-2">
              <PermissionBadge
                label={t('computerUseAccessibility')}
                state={permissions.accessibilityNeedsRestart ? 'granted' : permissions.accessibility}
                t={t}
              />
              <PermissionBadge
                label={t('computerUseScreenRecording')}
                state={permissions.screenRecording}
                t={t}
              />
              <CodexToolbarButton onClick={() => request('accessibility')}>
                {t('computerUseGrantAccessibility')}
              </CodexToolbarButton>
              <CodexToolbarButton onClick={() => request('screenRecording')}>
                {t('computerUseGrantScreenRecording')}
              </CodexToolbarButton>
              <CodexToolbarButton onClick={refresh}>
                {t('computerUseRecheck')}
              </CodexToolbarButton>
            </div>
          }
        />
      </CodexSettingsPanel>
    </div>
  )
}

function useComputerUseDoctor(): ComputerUseDoctorResult | null {
  const [doctor, setDoctor] = useState<ComputerUseDoctorResult | null>(null)

  useEffect(() => {
    void window.analytix?.app?.getComputerUseDoctor?.().then(setDoctor).catch(() => undefined)
  }, [])

  return doctor
}

function useChromeBrowserUseStatus(refreshKey: boolean): ChromeBrowserUseStatus | null {
  const [status, setStatus] = useState<ChromeBrowserUseStatus | null>(null)

  useEffect(() => {
    const getChromeBrowserUseStatus = window.analytix?.app?.getChromeBrowserUseStatus
    if (typeof getChromeBrowserUseStatus !== 'function') {
      setStatus(null)
      return
    }
    let canceled = false
    void getChromeBrowserUseStatus()
      .then((result) => {
        if (!canceled) setStatus(result)
      })
      .catch(() => {
        if (!canceled) setStatus(null)
      })
    return () => {
      canceled = true
    }
  }, [refreshKey])

  return status
}

function chromeUiState(
  chromeStatus: ChromeBrowserUseStatus | null,
  chromeControlEnabled: boolean
): ChromeConnectionUiState {
  if (!chromeControlEnabled) return 'disabled'
  return chromeStatus?.state ?? 'diagnosticsUnavailable'
}

function chromeStatusKey(state: ChromeConnectionUiState, compact = false): string {
  const suffix = compact ? 'Short' : ''
  if (state === 'connected') return compact ? 'computerUseChromeConnectedShort' : 'computerUseChromeConnected'
  if (state === 'disconnected') return compact ? 'computerUseChromeDisconnectedShort' : 'computerUseChromeDisconnected'
  if (state === 'configuredNotVerified') return `computerUseChromeConfigured${suffix}`
  if (state === 'extensionMissing') return `computerUseChromeExtensionMissing${suffix}`
  if (state === 'extensionDisabled') return `computerUseChromeExtensionDisabled${suffix}`
  if (state === 'nativeHostMissing') return `computerUseChromeNativeHostMissing${suffix}`
  if (state === 'nativeHostInvalid') return `computerUseChromeNativeHostInvalid${suffix}`
  if (state === 'unsupported') return `computerUseChromeUnsupported${suffix}`
  if (state === 'disabled') return `computerUseChromeControlOff${suffix}`
  return `computerUseChromeDiagnosticsUnavailable${suffix}`
}

function chromeToneClass(state: ChromeConnectionUiState, pill = false): string {
  if (state === 'connected') {
    return pill ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-emerald-500'
  }
  if (state === 'configuredNotVerified') {
    return pill ? 'bg-amber-500/10 text-amber-700 dark:text-amber-300' : 'bg-amber-500'
  }
  if (state === 'disconnected') {
    return pill ? 'bg-red-500/10 text-red-700 dark:text-red-300' : 'bg-red-500'
  }
  if (state === 'disabled' || state === 'diagnosticsUnavailable' || state === 'unsupported') {
    return pill ? 'bg-ds-subtle text-ds-muted' : 'bg-ds-muted'
  }
  return pill ? 'bg-red-500/10 text-red-700 dark:text-red-300' : 'bg-red-500'
}

function ChromeStatus({
  chromeStatus,
  chromeControlEnabled,
  t
}: {
  chromeStatus: ChromeBrowserUseStatus | null
  chromeControlEnabled: boolean
  t: (key: string) => string
}): ReactElement {
  const state = chromeUiState(chromeStatus, chromeControlEnabled)
  return (
    <span className="inline-flex max-w-full items-center gap-2">
      <span className={`h-2.5 w-2.5 rounded-full ${chromeToneClass(state)}`} />
      <span>{t(chromeStatusKey(state))}</span>
    </span>
  )
}

function ChromeConnectionPill({
  chromeStatus,
  chromeControlEnabled,
  t
}: {
  chromeStatus: ChromeBrowserUseStatus | null
  chromeControlEnabled: boolean
  t: (key: string) => string
}): ReactElement {
  const state = chromeUiState(chromeStatus, chromeControlEnabled)
  return (
    <span
      className={`inline-flex items-center gap-2 rounded-xl px-2.5 py-1 text-[13px] font-semibold ${chromeToneClass(state, true)}`}
    >
      <span className={`h-2.5 w-2.5 rounded-full ${chromeToneClass(state)}`} />
      {t(chromeStatusKey(state, true))}
    </span>
  )
}

function ChromeDiagnosticBadge({
  label,
  tone
}: {
  label: string
  tone: 'success' | 'warning' | 'danger' | 'neutral'
}): ReactElement {
  const toneClass = tone === 'success'
    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    : tone === 'warning'
      ? 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
      : tone === 'danger'
        ? 'bg-red-500/10 text-red-700 dark:text-red-300'
        : 'bg-ds-subtle text-ds-muted'
  return (
    <span className={`rounded-xl px-2.5 py-1 text-[13px] font-semibold ${toneClass}`}>
      {label}
    </span>
  )
}

function ChromeDiagnosticDetails({
  summary,
  details
}: {
  summary: ReactNode
  details: Array<{ label: string; value: string | undefined | null }>
}): ReactElement {
  const visibleDetails = details.filter((item) => item.value && item.value.trim().length > 0)
  return (
    <div className="space-y-1">
      <div>{summary}</div>
      {visibleDetails.length > 0 ? (
        <div className="grid gap-1 pt-1 text-[12px] leading-5">
          {visibleDetails.map((item) => (
            <div key={item.label} className="min-w-0">
              <span className="font-semibold text-ds-ink">{item.label}: </span>
              <span className="break-all font-mono text-[11px] text-ds-muted">{item.value}</span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function ChromeDiagnosticsPanel({
  chromeStatus,
  t
}: {
  chromeStatus: ChromeBrowserUseStatus | null
  t: (key: string) => string
}): ReactElement {
  if (!chromeStatus) {
    return (
      <CodexSettingsSection title={t('computerUseChromeDiagnosticsTitle')}>
        <CodexSettingsPanel>
          <CodexSettingsRow
            title={t('computerUseChromeConnectionProbe')}
            description={t('computerUseChromeStatusApiMissing')}
            control={<ChromeDiagnosticBadge label={t('computerUseChromeDiagnosticsUnavailableShort')} tone="neutral" />}
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>
    )
  }

  const extensionTone = chromeStatus.extension.status === 'enabled'
    ? 'success'
    : chromeStatus.extension.status === 'disabled'
      ? 'warning'
      : 'danger'
  const nativeHostTone = chromeStatus.nativeHost.status === 'configured'
    ? 'success'
    : chromeStatus.nativeHost.status === 'missing' || chromeStatus.nativeHost.status === 'invalid'
      ? 'danger'
      : 'neutral'
  const probeTone = chromeStatus.connectionProbe.status === 'passed'
    ? 'success'
    : chromeStatus.connectionProbe.status === 'failed'
      ? 'danger'
      : 'neutral'
  const selectedProfile = chromeStatus.extension.profiles.find((profile) => profile.selected)
  const extensionSummary =
    chromeStatus.extension.problem ??
    chromeStatus.extension.selectedProfilePath ??
    chromeStatus.extension.userDataDirectory ??
    t('computerUseChromeDiagnosticsUnavailable')
  const nativeHostSummary =
    chromeStatus.nativeHost.problem ??
    chromeStatus.nativeHost.manifestPath ??
    t('computerUseChromeDiagnosticsUnavailable')
  const probeSummary =
    chromeStatus.connectionProbe.problem ?? chromeStatus.reason ?? t('computerUseChromeConfigured')

  return (
    <CodexSettingsSection title={t('computerUseChromeDiagnosticsTitle')}>
      <CodexSettingsPanel>
        <CodexSettingsRow
          title={t('computerUseChromeExtensionStatus')}
          description={
            <ChromeDiagnosticDetails
              summary={extensionSummary}
              details={[
                { label: t('computerUseChromeUserDataDirectory'), value: chromeStatus.extension.userDataDirectory },
                { label: t('computerUseChromeSelectedProfile'), value: chromeStatus.extension.selectedProfilePath },
                { label: t('computerUseChromePreferencesPath'), value: selectedProfile?.preferencesPath },
                { label: t('computerUseChromeExtensionPath'), value: selectedProfile?.extensionPath }
              ]}
            />
          }
          control={
            <ChromeDiagnosticBadge
              label={t(`computerUseChromeExtensionStatus_${chromeStatus.extension.status}`)}
              tone={extensionTone}
            />
          }
        />
        <CodexSettingsRow
          title={t('computerUseChromeNativeHostStatus')}
          description={
            <ChromeDiagnosticDetails
              summary={nativeHostSummary}
              details={[
                { label: t('computerUseChromeNativeHostManifest'), value: chromeStatus.nativeHost.manifestPath },
                { label: t('computerUseChromeWindowsRegistry'), value: chromeStatus.nativeHost.registryKey },
                { label: t('computerUseChromeRegistryManifest'), value: chromeStatus.nativeHost.registryManifestPath },
                { label: t('computerUseChromeNativeHostPath'), value: chromeStatus.nativeHost.actualHostPath },
                { label: t('computerUseChromeAllowedOrigins'), value: chromeStatus.nativeHost.allowedOrigins?.join(', ') }
              ]}
            />
          }
          control={
            <ChromeDiagnosticBadge
              label={t(`computerUseChromeNativeHostStatus_${chromeStatus.nativeHost.status}`)}
              tone={nativeHostTone}
            />
          }
        />
        <CodexSettingsRow
          title={t('computerUseChromeConnectionProbe')}
          description={
            <ChromeDiagnosticDetails
              summary={probeSummary}
              details={[
                { label: t('computerUseChromeBrowserClient'), value: chromeStatus.connectionProbe.browserClientPath },
                { label: t('computerUseChromeCheckedAt'), value: chromeStatus.checkedAt }
              ]}
            />
          }
          control={
            <ChromeDiagnosticBadge
              label={t(`computerUseChromeConnectionProbe_${chromeStatus.connectionProbe.status}`)}
              tone={probeTone}
            />
          }
        />
      </CodexSettingsPanel>
    </CodexSettingsSection>
  )
}

type ComputerUseChromeSettingsDetailProps = {
  ctx: Record<string, any>
  browserUse: AnalytixBrowserUseSettingsV1
  chromeStatus: ChromeBrowserUseStatus | null
  onBack: () => void
  onBrowserUseChange: (patch: Partial<AnalytixBrowserUseSettingsV1>) => void
}

export function ComputerUseChromeSettingsDetail({
  ctx,
  browserUse,
  chromeStatus,
  onBack,
  onBrowserUseChange
}: ComputerUseChromeSettingsDetailProps): ReactElement {
  const { t, showTopNotice } = ctx
  const unavailableTitle = t('settingsFeatureUnavailable')
  const approvalOptions = [
    { value: 'neverAsk', label: t('browserApprovalAlwaysAllow') },
    { value: 'alwaysAsk', label: t('browserApprovalAlwaysAsk') }
  ]
  const askOnlyOptions = [
    { value: 'alwaysAsk', label: t('browserApprovalAlwaysAsk') }
  ]
  const showUnavailable = (messageKey: string): void => {
    showTopNotice?.({ tone: 'info', message: t(messageKey) })
  }
  const openExtensionPage = (target: 'webstore' | 'settings'): void => {
    const openPage = window.analytix?.app?.openChromeBrowserUseExtensionPage
    if (typeof openPage !== 'function') {
      showUnavailable('computerUseChromeExtensionActionUnavailable')
      return
    }
    void openPage(target)
      .then((result) => {
        if (!result.ok) {
          showTopNotice?.({
            tone: 'warning',
            message: result.message ?? t('computerUseChromeExtensionOpenFailed')
          })
        }
      })
      .catch((error) => {
        showTopNotice?.({
          tone: 'warning',
          message: error instanceof Error ? error.message : t('computerUseChromeExtensionOpenFailed')
        })
      })
  }

  return (
    <CodexSettingsPage
      title={t('computerUseChrome')}
      subtitle={
        <ChromeConnectionPill
          chromeStatus={chromeStatus}
          chromeControlEnabled={browserUse.chromeControlEnabled}
          t={t}
        />
      }
      eyebrow={
        <div className="flex min-w-0 items-center gap-2 text-[14px] font-semibold text-ds-muted">
          <button
            type="button"
            className="inline-flex items-center gap-1.5 rounded-lg px-0 py-1 text-ds-muted transition hover:text-ds-ink"
            onClick={onBack}
          >
            <ArrowLeft className="h-4 w-4" strokeWidth={2} />
            {t('computerUseBack')}
          </button>
          <span className="truncate">{t('computerUseChromeBreadcrumbParent')}</span>
          <ChevronRight className="h-4 w-4 shrink-0" strokeWidth={2} />
          <span className="truncate text-ds-ink">{t('computerUseChrome')}</span>
        </div>
      }
      actions={
        <>
          <CodexToolbarButton
            onClick={() => openExtensionPage('webstore')}
          >
            {t('computerUseChromeReinstallExtension')}
          </CodexToolbarButton>
          <CodexToolbarButton
            onClick={() => openExtensionPage('settings')}
          >
            {t('computerUseChromeOpenExtensionSettings')}
          </CodexToolbarButton>
        </>
      }
    >
      <ChromeDiagnosticsPanel chromeStatus={chromeStatus} t={t} />

      <CodexSettingsSection title={t('computerUseChromePermissionsTitle')}>
        <CodexSettingsPanel>
          <CodexSettingsRow
            title={t('browserApproval')}
            description={
              <>
                {t('browserApprovalDesc')}{' '}
                <button
                  type="button"
                  className="font-semibold text-accent hover:underline"
                  onClick={() => showUnavailable('browserLearnMoreUnavailable')}
                >
                  {t('browserLearnMore')}
                </button>
              </>
            }
            control={
              <CodexSelect
                value={browserUse.approvalMode}
                options={approvalOptions}
                onChange={(value) => onBrowserUseChange({ approvalMode: value as AnalytixBrowserApprovalMode })}
              />
            }
          />
          <CodexSettingsRow
            title={t('computerUseChromeHistory')}
            description={t('computerUseChromeHistoryDesc')}
            control={
              <CodexSelect
                value="alwaysAsk"
                options={askOnlyOptions}
                onChange={() => showUnavailable('computerUseChromePermissionUnavailable')}
                disabled
                title={unavailableTitle}
              />
            }
          />
          <CodexSettingsRow
            title={t('computerUseChromeDownload')}
            description={t('computerUseChromeDownloadDesc')}
            control={
              <CodexSelect
                value="alwaysAsk"
                options={askOnlyOptions}
                onChange={() => showUnavailable('computerUseChromePermissionUnavailable')}
                disabled
                title={unavailableTitle}
              />
            }
          />
          <CodexSettingsRow
            title={t('computerUseChromeUpload')}
            description={t('computerUseChromeUploadDesc')}
            control={
              <CodexSelect
                value="alwaysAsk"
                options={askOnlyOptions}
                onChange={() => showUnavailable('computerUseChromePermissionUnavailable')}
                disabled
                title={unavailableTitle}
              />
            }
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>

      <CodexSettingsSection
        title={t('browserSitePermissionsTitle')}
        subtitle={t('browserSitePermissionsSubtitle')}
        action={
          <CodexToolbarButton
            onClick={() => showUnavailable('browserSitePermissionsAddUnavailable')}
            disabled
            title={unavailableTitle}
          >
            <Plus className="h-5 w-5" strokeWidth={1.9} />
            {t('browserSitePermissionsAdd')}
          </CodexToolbarButton>
        }
      >
        {browserUse.sitePermissions.length > 0 ? (
          <CodexSettingsPanel>
            {browserUse.sitePermissions.map((permission) => (
              <CodexSettingsRow
                key={permission.origin}
                title={permission.origin}
                description={t('browserSitePermissionAlwaysAllow')}
                control={<span className="text-[13px] font-semibold text-ds-muted">{t('browserApprovalAlwaysAllow')}</span>}
              />
            ))}
          </CodexSettingsPanel>
        ) : (
          <CodexEmptyState>{t('browserSitePermissionsEmpty')}</CodexEmptyState>
        )}
      </CodexSettingsSection>

      <CodexSettingsSection title={t('browserDeveloperMode')}>
        <CodexSettingsPanel>
          <CodexSettingsRow
            title={
              <span className="flex flex-col gap-1">
                <span className="inline-flex items-center gap-1.5 text-[13px] text-orange-600">
                  <ShieldAlert className="h-4 w-4" strokeWidth={2} />
                  {t('browserRiskElevated')}
                </span>
                <span>{t('browserFullCdp')}</span>
              </span>
            }
            description={t('browserFullCdpDesc')}
            control={
              <CodexToggle
                checked={browserUse.fullCdpAccess}
                onChange={(fullCdpAccess) => onBrowserUseChange({ fullCdpAccess })}
                label={t('browserFullCdp')}
              />
            }
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>
    </CodexSettingsPage>
  )
}

export function ComputerUseSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    form,
    analytix,
    updateAnalytix,
    showTopNotice
  } = ctx
  const [panel, setPanel] = useState<'overview' | 'chrome'>('overview')
  const doctor = useComputerUseDoctor()
  const provider = form?.provider ?? ctx.provider ?? defaultModelProviderSettings()
  const modelProviders = Array.isArray(provider.providers)
    ? provider.providers as ModelProviderProfileV1[]
    : []
  const activeProviderId = analytix.providerId?.trim() || provider.activeProviderId?.trim() || DEFAULT_MODEL_PROVIDER_ID
  const activeProvider = modelProviders.find((item) => item.id === activeProviderId) ?? modelProviders[0]
  const computerUse: AnalytixComputerUseSettingsV1 = {
    ...defaultAnalytixComputerUseSettings(),
    ...(analytix.computerUse ?? {})
  }
  const browserUse: AnalytixBrowserUseSettingsV1 = {
    ...defaultAnalytixBrowserUseSettings(),
    ...(analytix.browserUse ?? {}),
    sitePermissions: analytix.browserUse?.sitePermissions ?? []
  }
  const visionBridge: AnalytixVisionBridgeSettingsV1 = {
    ...defaultAnalytixVisionBridgeSettings(),
    ...(analytix.visionBridge ?? {})
  }
  const activeModelProfile = activeProvider?.modelProfiles?.[normalizeModelId(analytix.model)]
  const activeModelSupportsImage = activeModelProfile?.inputModalities?.includes('image') ?? false
  const activeModelSupportsToolCalling = activeModelProfile?.supportsToolCalling !== false
  const bridgeProviderId = visionBridge.providerId?.trim() || activeProviderId
  const bridgeProvider = modelProviders.find((item) => item.id === bridgeProviderId) ?? activeProvider
  const bridgeModel = visionBridge.model?.trim() || bridgeProvider?.models?.[0] || ''
  const bridgeBaseUrl = visionBridge.baseUrl?.trim() || bridgeProvider?.baseUrl?.trim() || ''
  const bridgeEndpointFormat =
    visionBridge.endpointFormat ?? bridgeProvider?.endpointFormat ?? activeProvider?.endpointFormat ?? 'chat_completions'
  const bridgeProbeKey = bridgeProviderId && bridgeModel && bridgeBaseUrl
    ? modelCapabilityProbeKey({
        providerId: bridgeProviderId,
        model: bridgeModel,
        baseUrl: bridgeBaseUrl,
        endpointFormat: bridgeEndpointFormat
      })
    : ''
  const bridgeProbe = bridgeProbeKey ? analytix.modelCapabilityProbes?.[bridgeProbeKey] : undefined
  const bridgeProbeFresh = bridgeProbe ? Date.parse(bridgeProbe.staleAfter) > Date.now() : false
  const visionBridgeReady =
    visionBridge.enabled &&
    visionBridge.mode !== 'off' &&
    Boolean(bridgeProviderId && bridgeModel && bridgeBaseUrl) &&
    Boolean(bridgeProbeFresh && bridgeProbe?.status === 'supported')
  const activeModelBridgeHint = !activeModelSupportsImage && visionBridgeReady
  const [capabilityProbeBusy, setCapabilityProbeBusy] = useState(false)
  const [capabilityProbe, setCapabilityProbe] = useState<ModelCapabilityProbeResult | null>(null)
  const autoVisionBridgeProbeKeyRef = useRef('')
  const chromeStatus = useChromeBrowserUseStatus(browserUse.chromeControlEnabled)
  const backendUnavailable = doctor?.backend.available === false

  const setComputerUse = (patch: Partial<AnalytixComputerUseSettingsV1>): void => {
    updateAnalytix({
      computerUse: {
        ...computerUse,
        ...patch
      }
    })
  }

  const setBrowserUse = (patch: Partial<AnalytixBrowserUseSettingsV1>): void => {
    updateAnalytix({
      browserUse: {
        ...browserUse,
        ...patch
      }
    })
  }

  const setVisionBridge = (patch: Partial<AnalytixVisionBridgeSettingsV1>): void => {
    updateAnalytix({
      visionBridge: {
        ...visionBridge,
        ...patch
      }
    })
  }

  const requestSystemPermissions = (): void => {
    const appApi = typeof window !== 'undefined' ? window.analytix?.app : undefined
    void requestComputerUsePermissionsIfNeeded(appApi).catch(() => undefined)
  }

  const toggleAnyApp = (enabled: boolean): void => {
    setComputerUse({ enabled, mode: enabled ? 'always' : 'off' })
    if (enabled) requestSystemPermissions()
  }

  const showUnavailable = (messageKey: string): void => {
    showTopNotice?.({ tone: 'info', message: t(messageKey) })
  }

  const runCapabilityProbe = async (): Promise<void> => {
    const probeModelCapabilities = typeof window !== 'undefined'
      ? window.analytix?.runtime?.probeModelCapabilities
      : undefined
    const probeProvider = visionBridge.enabled ? bridgeProvider : activeProvider
    const probeModel = visionBridge.enabled ? bridgeModel : analytix.model
    const probeBaseUrl = visionBridge.enabled ? bridgeBaseUrl : activeProvider?.baseUrl
    const probeEndpointFormat = visionBridge.enabled ? bridgeEndpointFormat : activeProvider?.endpointFormat
    if (typeof probeModelCapabilities !== 'function') {
      showUnavailable('computerUseCapabilityProbeUnavailable')
      return
    }
    if (!probeProvider || !probeModel?.trim() || !probeBaseUrl?.trim() || !probeEndpointFormat) {
      showUnavailable('computerUseCapabilityProbeMissing')
      return
    }
    setCapabilityProbeBusy(true)
    try {
      const result = await probeModelCapabilities({
        providerId: probeProvider.id,
        model: probeModel,
        baseUrl: probeBaseUrl,
        endpointFormat: probeEndpointFormat
      })
      setCapabilityProbe(result)
    } finally {
      setCapabilityProbeBusy(false)
    }
  }

  useEffect(() => {
    if (!visionBridge.enabled || visionBridge.mode === 'off') return
    if (!bridgeProbeKey || !bridgeProvider || !bridgeModel.trim() || !bridgeBaseUrl.trim() || !bridgeEndpointFormat) return
    if (bridgeProbeFresh && bridgeProbe?.status === 'supported') return
    const probeModelCapabilities = typeof window !== 'undefined'
      ? window.analytix?.runtime?.probeModelCapabilities
      : undefined
    if (typeof probeModelCapabilities !== 'function') return

    const autoProbeKey = [
      bridgeProbeKey,
      bridgeProbe?.status ?? 'missing'
    ].join('|')
    if (autoVisionBridgeProbeKeyRef.current === autoProbeKey) return
    autoVisionBridgeProbeKeyRef.current = autoProbeKey

    let disposed = false
    setCapabilityProbeBusy(true)
    void probeModelCapabilities({
      providerId: bridgeProvider.id,
      model: bridgeModel,
      baseUrl: bridgeBaseUrl,
      endpointFormat: bridgeEndpointFormat
    })
      .then((result) => {
        if (!disposed) setCapabilityProbe(result)
      })
      .catch(() => undefined)
      .finally(() => {
        if (!disposed) setCapabilityProbeBusy(false)
      })

    return () => {
      disposed = true
    }
  }, [
    bridgeBaseUrl,
    bridgeEndpointFormat,
    bridgeModel,
    bridgeProbe?.status,
    bridgeProbeFresh,
    bridgeProbeKey,
    bridgeProvider,
    visionBridge.enabled,
    visionBridge.mode
  ])

  const statusDescription: ReactNode = backendUnavailable
    ? t('computerUseBackendUnavailable')
    : t('computerUseAnyAppDesc')
  const unavailableTitle = t('settingsFeatureUnavailable')

  if (panel === 'chrome') {
    return (
      <ComputerUseChromeSettingsDetail
        ctx={ctx}
        browserUse={browserUse}
        chromeStatus={chromeStatus}
        onBack={() => setPanel('overview')}
        onBrowserUseChange={setBrowserUse}
      />
    )
  }

  return (
    <CodexSettingsPage
      title={t('computerUseTitle')}
      subtitle={t('computerUseSubtitle')}
    >
      <CodexSettingsSection title={t('computerUseControlSection')}>
        <CodexSettingsPanel>
          <CodexSettingsRow
            icon={
              <img
                src={computerUseIcon}
                alt=""
                className="h-10 w-10 rounded-[10px] object-cover"
                draggable={false}
              />
            }
            title={t('computerUseAnyApp')}
            description={statusDescription}
            control={
              <CodexToggle
                checked={computerUse.enabled && computerUse.mode !== 'off'}
                onChange={toggleAnyApp}
                label={t('computerUseAnyApp')}
              />
            }
          />
          <CodexSettingsRow
            icon={
              <img
                src={chromeIcon}
                alt=""
                className="h-10 w-10 object-contain"
                draggable={false}
              />
            }
            title={t('computerUseChrome')}
            description={
              <ChromeStatus
                chromeStatus={chromeStatus}
                chromeControlEnabled={browserUse.chromeControlEnabled}
                t={t}
              />
            }
            control={
              <div className="flex items-center gap-2">
                <CodexToolbarButton onClick={() => setPanel('chrome')}>
                  {t('computerUseManage')}
                </CodexToolbarButton>
                <CodexToggle
                  checked={browserUse.chromeControlEnabled}
                  onChange={(chromeControlEnabled) => setBrowserUse({ chromeControlEnabled })}
                  label={t('computerUseChrome')}
                />
              </div>
            }
          />
        </CodexSettingsPanel>

        <ComputerUsePermissionPanel t={t} />
      </CodexSettingsSection>

      <CodexSettingsSection
        title={t('computerUseAlwaysAllowedApps')}
        action={
          <CodexToolbarButton
            onClick={() => showUnavailable('computerUseAllowedAppsAddUnavailable')}
            disabled
            title={unavailableTitle}
          >
            <Plus className="h-5 w-5" strokeWidth={1.9} />
            {t('computerUseAllowedAppsAdd')}
          </CodexToolbarButton>
        }
      >
        <CodexEmptyState>{t('computerUseAllowedAppsEmpty')}</CodexEmptyState>
      </CodexSettingsSection>

      <CodexSettingsSection
        title={t('computerUseVisionTitle')}
        subtitle={t('computerUseVisionSubtitle')}
      >
        <CodexSettingsPanel>
          <CodexSettingsRow
            title={t('computerUseRouteStatus')}
            description={
              <div className="mt-3 grid gap-2 text-[13px] leading-5 sm:grid-cols-2">
                <div className="rounded-xl bg-ds-subtle px-3 py-2">
                  <div className="font-semibold text-ds-ink">{t('computerUsePrimaryModel')}</div>
                  <div className="mt-0.5 text-ds-muted">
                    {activeProvider?.name ?? activeProviderId} / {analytix.model}
                  </div>
                </div>
                <div className="rounded-xl bg-ds-subtle px-3 py-2">
                  <div className="font-semibold text-ds-ink">{t('computerUsePrimaryCapabilities')}</div>
                  <div className="mt-0.5 text-ds-muted">
                    {t(activeModelSupportsImage ? 'computerUseImageSupported' : 'computerUseImageTextOnly')}
                    {' · '}
                    {t(activeModelSupportsToolCalling ? 'computerUseToolSupported' : 'computerUseToolUnsupported')}
                  </div>
                </div>
                <div className="rounded-xl bg-ds-subtle px-3 py-2">
                  <div className="font-semibold text-ds-ink">{t('visionBridgeTitle')}</div>
                  <div className="mt-0.5 text-ds-muted">
                    {visionBridgeReady
                      ? `${bridgeProvider?.name ?? bridgeProviderId} / ${bridgeModel}`
                      : visionBridge.enabled && visionBridge.mode !== 'off'
                        ? t('visionBridgeNotReady')
                        : t('visionBridgeOff')}
                  </div>
                </div>
                <div className="rounded-xl bg-ds-subtle px-3 py-2">
                  <div className="font-semibold text-ds-ink">{t('computerUseRoute')}</div>
                  <div className="mt-0.5 text-ds-muted">
                    {activeModelSupportsImage
                      ? t('computerUseRouteNative')
                      : activeModelBridgeHint
                        ? t('computerUseRouteBridge')
                        : t('computerUseRouteUnavailable')}
                  </div>
                </div>
              </div>
            }
          />
          <CodexSettingsRow
            title={t('visionBridgeTitle')}
            description={t('visionBridgeDesc')}
            control={
              <CodexToggle
                checked={visionBridge.enabled}
                onChange={(enabled) => setVisionBridge({ enabled })}
                label={t('visionBridgeTitle')}
              />
            }
          />
          {visionBridge.enabled ? (
            <>
              <CodexSettingsRow
                title={t('visionBridgeMode')}
                description={t('visionBridgeModeDesc')}
                control={
                  <CodexSelect
                    value={visionBridge.mode}
                    options={[
                      { value: 'auto', label: t('computerUseModeAuto') },
                      { value: 'always', label: t('computerUseModeAlways') },
                      { value: 'off', label: t('computerUseModeOff') }
                    ]}
                    onChange={(mode) => setVisionBridge({ mode: mode as AnalytixVisionBridgeMode })}
                  />
                }
              />
              <CodexSettingsRow
                title={t('visionBridgeProvider')}
                description={t('visionBridgeProviderDesc')}
                control={
                  <CodexSelect
                    value={visionBridge.providerId}
                    options={[
                      { value: '', label: t('visionBridgeProviderActive') },
                      ...modelProviders.map((item) => ({ value: item.id, label: item.name }))
                    ]}
                    onChange={(providerId) => setVisionBridge({ providerId })}
                  />
                }
              />
              <CodexSettingsRow
                title={t('visionBridgeModel')}
                description={t('visionBridgeModelDesc')}
                control={
                  <input
                    className="h-10 w-[220px] rounded-xl border-0 bg-ds-subtle px-3.5 text-[14px] font-semibold text-ds-ink outline-none transition hover:bg-ds-hover focus:ring-2 focus:ring-accent/25"
                    value={visionBridge.model}
                    onChange={(event) => setVisionBridge({ model: event.target.value })}
                  />
                }
              />
            </>
          ) : null}
          <CodexSettingsRow
            title={t('computerUseCapabilityProbe')}
            description={t('computerUseCapabilityProbeDesc')}
            control={
              <div className="flex max-w-[300px] flex-col items-end gap-2">
                <CodexToolbarButton
                  onClick={() => void runCapabilityProbe()}
                  disabled={capabilityProbeBusy}
                >
                  {capabilityProbeBusy ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="h-4 w-4" />
                  )}
                  {t('computerUseRunProbe')}
                </CodexToolbarButton>
                {capabilityProbe ? (
                  <span className="text-right text-[12px] font-medium leading-5 text-ds-muted">
                    {t('computerUseProbeResult', {
                      status: capabilityProbe.status,
                      image: capabilityProbe.imageInput,
                      tool: capabilityProbe.toolCalling,
                      screenshot: capabilityProbe.toolResultImage
                    })}
                  </span>
                ) : null}
              </div>
            }
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>
    </CodexSettingsPage>
  )
}
