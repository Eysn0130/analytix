import type { ReactElement } from 'react'
import type { ApprovalPolicy, SandboxMode } from '@shared/app-settings'
import { InlineNoticeView, SettingsCard, SettingRow } from './settings-controls'

export function PermissionsSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const { t, analytix, updateAnalytix, selectControlClass } = ctx
  const elevatedAccess = analytix.sandboxMode === 'danger-full-access' && analytix.approvalPolicy === 'auto'

  return (
    <SettingsCard title={t('permissions')}>
      <div className="px-3 py-4">
        <InlineNoticeView notice={{ tone: 'info', message: t('permissionsBehaviorHint') }} />
        {elevatedAccess ? (
          <div className="mt-3">
            <InlineNoticeView notice={{ tone: 'info', message: t('permissionsElevatedHint') }} />
          </div>
        ) : null}
      </div>
      <SettingRow
        title={t('approvalPolicy')}
        description={t('approvalPolicyDesc')}
        control={
          <select
            className={selectControlClass}
            value={analytix.approvalPolicy}
            onChange={(e) => updateAnalytix({ approvalPolicy: e.target.value as ApprovalPolicy })}
          >
            <option value="on-request">{t('approvalOnRequest')}</option>
            <option value="always">{t('approvalAlways')}</option>
            <option value="auto">{t('approvalAuto')}</option>
            <option value="untrusted">{t('approvalUntrusted')}</option>
            <option value="suggest">{t('approvalSuggest')}</option>
            <option value="never">{t('approvalNever')}</option>
          </select>
        }
      />
      <SettingRow
        title={t('sandboxMode')}
        description={t('sandboxModeDesc')}
        control={
          <select
            className={selectControlClass}
            value={analytix.sandboxMode}
            onChange={(e) => updateAnalytix({ sandboxMode: e.target.value as SandboxMode })}
          >
            <option value="workspace-write">{t('sandboxWorkspaceWrite')}</option>
            <option value="read-only">{t('sandboxReadOnly')}</option>
            <option value="danger-full-access">{t('sandboxFullAccess')}</option>
            <option value="external-sandbox">{t('sandboxExternal')}</option>
          </select>
        }
      />
    </SettingsCard>
  )
}
