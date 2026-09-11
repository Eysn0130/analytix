import type { ReactElement } from 'react'
import type {
  AnalytixBrowserAnnotationScreenshotMode,
  AnalytixBrowserApprovalMode,
  AnalytixBrowserLocalUrlTarget,
  AnalytixBrowserUseSettingsV1
} from '@shared/app-settings'
import { Plus, ShieldAlert, SquareMousePointer } from 'lucide-react'
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

function defaultBrowserUse(): AnalytixBrowserUseSettingsV1 {
  return {
    enabled: true,
    localUrlOpenTarget: 'analytix',
    annotationScreenshotsMode: 'always',
    approvalMode: 'neverAsk',
    fullCdpAccess: true,
    chromeControlEnabled: true,
    sitePermissions: []
  }
}

export function BrowserSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    analytix,
    updateAnalytix,
    openSettingsSection,
    showTopNotice
  } = ctx
  const browserUse: AnalytixBrowserUseSettingsV1 = {
    ...defaultBrowserUse(),
    ...(analytix.browserUse ?? {}),
    sitePermissions: analytix.browserUse?.sitePermissions ?? []
  }
  const setBrowserUse = (patch: Partial<AnalytixBrowserUseSettingsV1>): void => {
    updateAnalytix({
      browserUse: {
        ...browserUse,
        ...patch
      }
    })
  }
  const showUnavailable = (messageKey: string): void => {
    showTopNotice?.({ tone: 'info', message: t(messageKey) })
  }
  const unavailableTitle = t('settingsFeatureUnavailable')

  return (
    <CodexSettingsPage
      title={t('browser')}
      subtitle={
        <>
          {t('browserPageSubtitle')}{' '}
          <button
            type="button"
            className="font-semibold text-accent hover:underline"
            onClick={() => openSettingsSection?.('computerUse')}
          >
            {t('browserPageSubtitleComputerUse')}
          </button>
        </>
      }
    >
      <CodexSettingsPanel>
        <CodexSettingsRow
          icon={<SquareMousePointer className="h-10 w-10 text-ds-ink" strokeWidth={1.85} />}
          title={t('browserBuiltInControl')}
          description={t('browserBuiltInControlDesc')}
          control={
            <CodexToggle
              checked={browserUse.enabled}
              onChange={(enabled) => setBrowserUse({ enabled })}
              label={t('browserBuiltInControl')}
            />
          }
        />
      </CodexSettingsPanel>

      <CodexSettingsSection title={t('browserGeneralTitle')}>
        <CodexSettingsPanel>
          <CodexSettingsRow
            title={t('browserLocalUrlTarget')}
            description={t('browserLocalUrlTargetDesc')}
            control={
              <CodexSelect
                value={browserUse.localUrlOpenTarget}
                disabled={!browserUse.enabled}
                options={[
                  { value: 'analytix', label: t('browserLocalUrlTargetAnalytix') },
                  { value: 'system', label: t('browserLocalUrlTargetSystem') }
                ]}
                onChange={(value) => setBrowserUse({ localUrlOpenTarget: value as AnalytixBrowserLocalUrlTarget })}
              />
            }
          />
          <CodexSettingsRow
            title={t('browserClearData')}
            description={t('browserClearDataDescCompact')}
            control={
              <CodexToolbarButton
                onClick={() => showUnavailable('browserClearDataUnavailable')}
                disabled
                title={unavailableTitle}
              >
                {t('browserClearAllData')}
              </CodexToolbarButton>
            }
          />
          <CodexSettingsRow
            title={t('browserAnnotationScreenshots')}
            description={t('browserAnnotationScreenshotsDesc')}
            control={
              <CodexSelect
                value={browserUse.annotationScreenshotsMode}
                disabled={!browserUse.enabled}
                options={[
                  { value: 'always', label: t('browserAnnotationScreenshotsAlways') },
                  { value: 'necessary', label: t('browserAnnotationScreenshotsNecessary') },
                  { value: 'off', label: t('browserAnnotationScreenshotsOff') }
                ]}
                onChange={(value) => setBrowserUse({ annotationScreenshotsMode: value as AnalytixBrowserAnnotationScreenshotMode })}
              />
            }
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>

      <CodexSettingsSection title={t('browserPermissionsSection')}>
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
                disabled={!browserUse.enabled}
                options={[
                  { value: 'neverAsk', label: t('browserApprovalAlwaysAllow') },
                  { value: 'alwaysAsk', label: t('browserApprovalAlwaysAsk') }
                ]}
                onChange={(value) => setBrowserUse({ approvalMode: value as AnalytixBrowserApprovalMode })}
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
                disabled={!browserUse.enabled}
                onChange={(fullCdpAccess) => setBrowserUse({ fullCdpAccess })}
                label={t('browserFullCdp')}
              />
            }
          />
        </CodexSettingsPanel>
      </CodexSettingsSection>
    </CodexSettingsPage>
  )
}
