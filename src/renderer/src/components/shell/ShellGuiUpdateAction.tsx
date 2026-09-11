import type { ReactElement } from 'react'
import { useEffect, useMemo, useState } from 'react'
import type { GuiUpdateState } from '@shared/gui-update'
import {
  ArrowUpCircle,
  ExternalLink,
  Loader2,
  RefreshCw
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { shellToolbarIconButtonClass, ToolbarTooltip } from './ShellToolbar'

type AvailableGuiUpdateInfo = Extract<GuiUpdateState, { status: 'available' }>['info']
export type ShellGuiUpdateActionMode = 'icon' | 'text'

type ShellGuiUpdateActionProps = {
  mode?: ShellGuiUpdateActionMode
}

export function shellGuiUpdateActionMode(leftSidebarCollapsed: boolean): ShellGuiUpdateActionMode {
  return leftSidebarCollapsed ? 'text' : 'icon'
}

export function shellGuiUpdateActionInfo(state: GuiUpdateState): AvailableGuiUpdateInfo | null {
  if (state.status === 'available' || state.status === 'downloaded') {
    return state.info.hasUpdate ? state.info : null
  }
  if (state.status === 'downloading' || state.status === 'installing') {
    return state.info?.hasUpdate ? state.info : null
  }
  if (state.status === 'error' && state.info?.ok && state.info.hasUpdate) {
    return state.info
  }
  return null
}

export function shellGuiUpdateBusy(state: GuiUpdateState, applying: boolean): boolean {
  return applying || state.status === 'downloading' || state.status === 'installing'
}

export function ShellGuiUpdateAction({ mode = 'icon' }: ShellGuiUpdateActionProps): ReactElement | null {
  const { t } = useTranslation(['common', 'settings'])
  const [guiUpdateState, setGuiUpdateState] = useState<GuiUpdateState>({ status: 'idle' })
  const [applyingGuiUpdate, setApplyingGuiUpdate] = useState(false)

  useEffect(() => {
    if (typeof window.analytix?.updates?.onState !== 'function') return
    const applyState = (state: GuiUpdateState): void => {
      setGuiUpdateState(state)
    }
    const unsubscribe = window.analytix.updates.onState(applyState)
    if (typeof window.analytix?.updates?.getState === 'function') {
      void window.analytix.updates.getState().then(applyState).catch(() => undefined)
    }
    return unsubscribe
  }, [])

  const guiUpdateAction = useMemo(() => shellGuiUpdateActionInfo(guiUpdateState), [guiUpdateState])

  const guiUpdateBusy = shellGuiUpdateBusy(guiUpdateState, applyingGuiUpdate)

  const guiUpdateLabel = useMemo(() => {
    if (!guiUpdateAction) return ''
    if (guiUpdateState.status === 'downloading') {
      return t('guiUpdateTopbarDownloading', {
        percent: Math.max(0, Math.round(guiUpdateState.progress.percent))
      })
    }
    if (guiUpdateState.status === 'installing') {
      return t('guiUpdateTopbarInstalling')
    }
    if (guiUpdateAction.downloaded || guiUpdateState.status === 'downloaded') {
      return t('settings:guiUpdateInstall')
    }
    if (guiUpdateAction.manualOnly) {
      return t('guiUpdateTopbarManual', { version: guiUpdateAction.latestVersion })
    }
    return t('guiUpdateTopbarAvailable', { version: guiUpdateAction.latestVersion })
  }, [guiUpdateAction, guiUpdateState, t])

  const guiUpdateTitle = useMemo(() => {
    if (!guiUpdateAction) return ''
    return guiUpdateAction.manualOnly
      ? t('settings:guiUpdateAvailableManual', {
          current: guiUpdateAction.currentVersion,
          latest: guiUpdateAction.latestVersion
        })
      : t('settings:guiUpdateAvailable', {
          current: guiUpdateAction.currentVersion,
          latest: guiUpdateAction.latestVersion
        })
  }, [guiUpdateAction, t])

  const guiUpdateShortLabel = useMemo(() => {
    if (!guiUpdateAction) return ''
    if (guiUpdateState.status === 'downloading' || guiUpdateState.status === 'installing' || applyingGuiUpdate) {
      return guiUpdateLabel
    }
    if (guiUpdateAction.downloaded || guiUpdateState.status === 'downloaded') {
      return t('settings:guiUpdateInstall')
    }
    if (guiUpdateAction.manualOnly) {
      return t('guiUpdateTopbarDownload')
    }
    return t('guiUpdateTopbarText')
  }, [applyingGuiUpdate, guiUpdateAction, guiUpdateLabel, guiUpdateState.status, t])

  const runGuiUpdateAction = async (): Promise<void> => {
    if (!guiUpdateAction || guiUpdateBusy) return
    if (guiUpdateAction.manualOnly) {
      if (typeof window.analytix?.app?.openExternal === 'function') {
        await window.analytix.app.openExternal(guiUpdateAction.releaseUrl)
      }
      return
    }
    if (
      typeof window.analytix?.updates?.download !== 'function' ||
      typeof window.analytix?.updates?.install !== 'function'
    ) {
      return
    }

    setApplyingGuiUpdate(true)
    try {
      if (!guiUpdateAction.downloaded && guiUpdateState.status !== 'downloaded') {
        const downloadResult = await window.analytix.updates.download(guiUpdateAction.channel)
        if (!downloadResult.ok) return
      }
      const installResult = await window.analytix.updates.install()
      if (!installResult.ok && typeof window.analytix?.logs?.error === 'function') {
        await window.analytix.logs.error('gui-update', 'Failed to install GUI update from shell action', {
          version: guiUpdateAction.latestVersion,
          message: installResult.message
        })
      }
    } catch (error) {
      if (typeof window.analytix?.logs?.error === 'function') {
        await window.analytix.logs.error('gui-update', 'Failed to apply GUI update from shell action', {
          version: guiUpdateAction.latestVersion,
          message: error instanceof Error ? error.message : String(error)
        })
      }
    } finally {
      setApplyingGuiUpdate(false)
    }
  }

  const renderIcon = (): ReactElement => {
    if (guiUpdateState.status === 'downloading' || guiUpdateState.status === 'installing' || applyingGuiUpdate) {
      return <Loader2 className="ds-toolbar-icon-svg animate-spin" strokeWidth={2} />
    }
    if (guiUpdateAction?.downloaded || guiUpdateState.status === 'downloaded') {
      return <RefreshCw className="ds-toolbar-icon-svg" strokeWidth={1.85} />
    }
    if (guiUpdateAction?.manualOnly) {
      return <ExternalLink className="ds-toolbar-icon-svg" strokeWidth={1.85} />
    }
    return <ArrowUpCircle className="ds-toolbar-icon-svg" strokeWidth={1.85} />
  }

  if (!guiUpdateAction) return null
  const showText = mode === 'text'

  return (
    <ToolbarTooltip label={guiUpdateTitle || guiUpdateLabel}>
      <button
        type="button"
        onClick={() => void runGuiUpdateAction()}
        disabled={guiUpdateBusy}
        className={shellToolbarIconButtonClass(
          false,
          showText ? 'ds-shell-update-button ds-shell-update-button-text' : 'ds-shell-update-button'
        )}
        aria-label={guiUpdateTitle || guiUpdateLabel}
      >
        {renderIcon()}
        {showText ? (
          <span className="ds-shell-update-button-label">{guiUpdateShortLabel}</span>
        ) : (
          <span className="sr-only">{guiUpdateLabel}</span>
        )}
      </button>
    </ToolbarTooltip>
  )
}
