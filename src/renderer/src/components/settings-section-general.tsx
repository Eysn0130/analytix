import { useId, type ReactElement } from 'react'
import type { AppMotionPreference, AppSettingsV1, WindowCloseAction } from '@shared/app-settings'
import { APP_ACCENT_COLORS, APP_MOTION_PREFERENCES, DEFAULT_APP_MOTION_PREFERENCE, normalizeAppAccentColor } from '@shared/app-settings'
import { Check, FolderOpen } from '../design/AnalytixUiIcons'
import { SettingsCard, SettingRow, Toggle } from './settings-controls'
import { LegacySessionImportCard } from './settings-section-general-legacy-import'
import { UiThemeSettingsSection } from './settings-section-ui-theme'
import { SettingsPerformanceUsage } from './settings-performance-usage'

export function GeneralSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    tCommon,
    form,
    update,
    selectControlClass,
    openProviderSettings,
    pickWorkspace,
    resetWorkspaceToDefault,
    workspacePickerError,
    logPath,
    logsSectionRef,
    logDirOpenError,
    setLogDirOpenError
  } = ctx
  const themeId = useId()
  const themeOptions: AppSettingsV1['theme'][] = ['system', 'light', 'dark']
  const platform = typeof window !== 'undefined' ? window.analytix?.app?.platform ?? '' : ''
  const openAtLoginSupported = platform === 'win32' || platform === 'darwin'
  const startMinimizedSupported = platform === 'win32'
  const desktopBehavior = form.appBehavior
  const closeAction = desktopBehavior.closeAction ?? (desktopBehavior.closeToTray ? 'tray' : 'ask')
  const closeActionOptions: WindowCloseAction[] = ['ask', 'tray', 'quit']
  const fontScaleOptions: AppSettingsV1['uiFontScale'][] = ['small', 'medium', 'large']
  const motionOptions = APP_MOTION_PREFERENCES
  const selectedFontScaleIndex = fontScaleOptions.indexOf(form.uiFontScale)
  const fontScaleIndex = selectedFontScaleIndex >= 0 ? selectedFontScaleIndex : 0
  const currentFontScale = fontScaleOptions[fontScaleIndex]
  const fontScaleLabel = (scale: AppSettingsV1['uiFontScale']): string => {
    if (scale === 'large') return t('fontScaleLarge')
    if (scale === 'medium') return t('fontScaleMedium')
    return t('fontScaleSmall')
  }
  const motionPreferenceLabel = (preference: AppMotionPreference): string => {
    if (preference === 'on') return t('motionPreferenceOn')
    if (preference === 'off') return t('motionPreferenceOff')
    return t('motionPreferenceSystem')
  }

  return (
    <>
      <SettingsCard title={t('sectionGeneral')} className="ax-settings-basics">
        <SettingRow
          title={t('language')}
          description={t('languageDesc')}
          control={
            <select
              className={selectControlClass}
              value={form.locale}
              onChange={(e) => update({ locale: e.target.value as 'en' | 'zh' })}
            >
              <option value="en">English</option>
              <option value="zh">简体中文</option>
            </select>
          }
        />
        <fieldset className="ax-settings-row ax-settings-theme-row" aria-describedby={`${themeId}-description`}>
          <legend>{t('theme')}</legend>
          <p id={`${themeId}-description`}>{t('themeDesc')}</p>
          <div className="ax-settings-theme-options">
            {themeOptions.map((theme) => (
              <label className="ax-settings-theme-option" data-selected={form.theme === theme} key={theme}>
                <input
                  className="ax-settings-theme-radio"
                  type="radio"
                  name={`${themeId}-theme`}
                  value={theme}
                  checked={form.theme === theme}
                  onChange={() => update({ theme })}
                />
                <span className="ax-settings-theme-preview" data-theme={theme} aria-hidden="true">
                  <span className="ax-settings-preview-window">
                    <span className="ax-settings-preview-bar"><span /><span /><span /></span>
                    <span className="ax-settings-preview-sidebar"><i /><i /><i /><i /><i /></span>
                    <span className="ax-settings-preview-content"><strong>Analytix</strong><span /><span className="ax-settings-preview-panels"><i /><i /></span></span>
                  </span>
                </span>
                {form.theme === theme ? <span className="ax-settings-theme-check" aria-hidden="true"><Check size={12} /></span> : null}
                <span className="ax-settings-theme-label">{t(theme === 'system' ? 'themeSystem' : theme === 'light' ? 'themeLight' : 'themeDark')}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset className="ax-settings-row ax-settings-accent-row">
          <legend>{t('accentColor')}</legend>
          <p>{t('accentColorDescription')}</p>
          <div className="ax-settings-accent-options">
            {APP_ACCENT_COLORS.map(accent => (
              <label className="ax-settings-accent-option" data-accent={accent} key={accent}>
                <input type="radio" name={`${themeId}-accent`} value={accent}
                  checked={normalizeAppAccentColor(form.accentColor) === accent}
                  aria-label={t(`accentColor_${accent}`)}
                  onChange={() => update({ accentColor: accent })} />
                <span aria-hidden="true">{normalizeAppAccentColor(form.accentColor) === accent ? <Check size={13} /> : null}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <SettingRow
          title={t('fontScale')}
          description={t('fontScaleDesc')}
          control={
            <div className="ax-settings-font-size w-full min-w-0 md:max-w-md">
              <div className="flex items-center justify-between text-[12px] font-medium text-ds-faint">
                {fontScaleOptions.map((scale) => (
                  <span key={scale}>{fontScaleLabel(scale)}</span>
                ))}
              </div>
              <input
                type="range"
                min={0}
                max={fontScaleOptions.length - 1}
                step={1}
                value={fontScaleIndex}
                aria-label={t('fontScale')}
                aria-valuetext={fontScaleLabel(currentFontScale)}
                className="mt-2 w-full accent-accent"
                onChange={(e) => {
                  const nextScale = fontScaleOptions[Number(e.target.value)] ?? 'medium'
                  update({ uiFontScale: nextScale })
                }}
              />
              <div className="mt-1.5 text-[13px] font-medium text-ds-muted">
                {t('fontScaleCurrent', { value: fontScaleLabel(currentFontScale) })}
              </div>
            </div>
          }
        />
        <SettingRow
          title={t('motionPreference')}
          description={t('motionPreferenceDesc')}
          control={
            <select
              className={selectControlClass}
              value={form.motionPreference ?? DEFAULT_APP_MOTION_PREFERENCE}
              onChange={(e) => update({ motionPreference: e.target.value as AppMotionPreference })}
            >
              {motionOptions.map((preference) => (
                <option key={preference} value={preference}>
                  {motionPreferenceLabel(preference)}
                </option>
              ))}
            </select>
          }
        />
        <SettingRow
          title={t('cursorSpotlight')}
          description={t('cursorSpotlightDesc')}
          control={
            <Toggle
              checked={form.cursorSpotlight !== false}
              onChange={(enabled) => update({ cursorSpotlight: enabled })}
            />
          }
        />
        <SettingsPerformanceUsage update={update} t={t} value={form.performanceUsageMode} selectClassName={selectControlClass} />
      </SettingsCard>

      <details className="ax-settings-disclosure">
        <summary>{t('settingsAdvancedGeneral')}</summary>
        <SettingsCard title={t('workspaceRoot')}>
          <SettingRow
            title={t('workspaceRoot')}
            description={t('workspaceRootDesc')}
            control={
              <div className="w-full min-w-0 md:max-w-xl">
                <div className="ax-settings-path-control flex flex-wrap items-center gap-2">
                  <input
                    className="w-full rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                    value={form.workspaceRoot}
                    onChange={(e) => update({ workspaceRoot: e.target.value })}
                    placeholder={t('workspaceRootPlaceholder')}
                  />
                  <button
                    type="button"
                    onClick={resetWorkspaceToDefault}
                    className="shrink-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover"
                  >
                    {t('restoreWorkspaceDefault')}
                  </button>
                  <button
                    type="button"
                    onClick={() => void pickWorkspace()}
                    className="shrink-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover"
                  >
                    {t('browse')}
                  </button>
                </div>
                {workspacePickerError ? (
                  <p className="mt-2 text-[13px] leading-5 text-amber-700 dark:text-amber-300">
                    {workspacePickerError}
                  </p>
                ) : null}
              </div>
            }
          />
        </SettingsCard>

        <UiThemeSettingsSection ctx={ctx} />

        <SettingsCard title={t('desktopBehavior')} className="mt-6">
          <SettingRow
            title={t('desktopOpenAtLogin')}
            description={
              openAtLoginSupported
                ? t('desktopOpenAtLoginDesc')
                : t('desktopOpenAtLoginUnsupportedDesc')
            }
            control={
              <Toggle
                checked={desktopBehavior.openAtLogin}
                disabled={!openAtLoginSupported}
                onChange={(v) =>
                  update({
                    appBehavior: {
                      openAtLogin: v,
                      startMinimized: v ? desktopBehavior.startMinimized : false
                    }
                  })
                }
              />
            }
          />
          <SettingRow
            title={t('desktopStartMinimized')}
            description={
              desktopBehavior.openAtLogin && startMinimizedSupported
                ? t('desktopStartMinimizedDesc')
                : t('desktopStartMinimizedDisabledDesc')
            }
            control={
              <Toggle
                checked={desktopBehavior.startMinimized}
                disabled={!desktopBehavior.openAtLogin || !startMinimizedSupported}
                onChange={(v) => update({ appBehavior: { startMinimized: v } })}
              />
            }
          />
          <SettingRow
            title={t('desktopCloseAction')}
            description={t('desktopCloseActionDesc')}
            control={
              <select
                className={selectControlClass}
                value={closeAction}
                onChange={(e) => update({ appBehavior: { closeAction: e.target.value as WindowCloseAction } })}
              >
                {closeActionOptions.map((option) => (
                  <option key={option} value={option}>
                    {t(`desktopCloseAction_${option}`)}
                  </option>
                ))}
              </select>
            }
          />
          <SettingRow
            title={t('turnCompleteNotification')}
            description={t('turnCompleteNotificationDesc')}
            control={
              <Toggle
                checked={form.notifications.turnComplete}
                onChange={(v) => update({ notifications: { turnComplete: v } })}
              />
            }
          />
        </SettingsCard>

        <SettingsCard title={t('providers')} className="mt-6">
          <SettingRow
            title={t('analytixProvider')}
            description={t('providersDesc')}
            control={
              <button
                type="button"
                onClick={openProviderSettings}
                className="inline-flex w-fit items-center rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover"
              >
                {t('providers')}
              </button>
            }
          />
        </SettingsCard>

        <LegacySessionImportCard t={t} tCommon={tCommon} />

        <div ref={logsSectionRef} className="mt-6">
        <SettingsCard title={t('logTitle')}>
          <SettingRow
            title={t('logEnabled')}
            description={t('logEnabledDesc')}
            control={
              <Toggle
                checked={form.log.enabled}
                onChange={(v) => update({ log: { enabled: v } })}
              />
            }
          />
          <SettingRow
            title={t('logRetention')}
            description={t('logRetentionDesc')}
            control={
              <select
                className={selectControlClass}
                value={form.log.retentionDays}
                onChange={(e) =>
                  update({ log: { retentionDays: Number(e.target.value) } })
                }
              >
                <option value={1}>{t('logRetentionOne')}</option>
                <option value={2}>{t('logRetentionTwo')}</option>
                <option value={3}>{t('logRetentionThree')}</option>
                <option value={5}>{t('logRetentionFive')}</option>
                <option value={7}>{t('logRetentionSeven')}</option>
              </select>
            }
          />
          <SettingRow
            title={t('logDir')}
            description={t('logDirDesc')}
            wideControl
            control={
              <div className="flex w-full min-w-0 flex-col items-start gap-2">
                {logPath ? (
                  <code className="block w-full max-w-full break-all rounded-xl bg-ds-main/70 px-3 py-2 font-mono text-[12px] text-ds-muted shadow-sm">
                    {logPath}
                  </code>
                ) : (
                  <span className="text-[13px] text-ds-faint">…</span>
                )}
                <button
                  type="button"
                  className="inline-flex items-center gap-1.5 rounded-xl border border-ds-border bg-ds-card px-3 py-1.5 text-[13px] font-medium text-ds-ink shadow-sm transition hover:bg-ds-hover disabled:opacity-50"
                  disabled={typeof window.analytix?.logs?.openDir !== 'function'}
                  onClick={async () => {
                    if (typeof window.analytix?.logs?.openDir !== 'function') return
                    setLogDirOpenError(null)
                    try {
                      const result = await window.analytix.logs.openDir()
                      if (!result.ok) setLogDirOpenError(result.message ?? 'Unknown error')
                    } catch (e) {
                      setLogDirOpenError(e instanceof Error ? e.message : String(e))
                    }
                  }}
                >
                  <FolderOpen className="h-4 w-4" />
                  {t('logDirOpen')}
                </button>
                {logDirOpenError ? (
                  <p className="text-[12px] text-red-700 dark:text-red-300">
                    {logDirOpenError}
                  </p>
                ) : null}
              </div>
            }
          />
        </SettingsCard>
        </div>
      </details>
    </>
  )
}
