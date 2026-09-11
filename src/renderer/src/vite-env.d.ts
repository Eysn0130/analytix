/// <reference types="vite/client" />

import type { DetailedHTMLProps, HTMLAttributes } from 'react'

export type AnalytixInstallerState = {
  source?: string
  platform?: string
  packageEdition?: string
  version?: string
  installedVersion?: string
  canUpdate?: boolean
  installed?: boolean
  installPath?: string
  canChooseInstallPath?: boolean
  canInstall?: boolean
  canUninstall?: boolean
  lifecycleState?: string
  progressPercent?: number
  detail?: string
  lastError?: string
  updatedAt?: string
}

export type AnalytixInstallerBridge = {
  version: string
  platform: 'win32'
  closeWindow: () => Promise<boolean>
  minimizeWindow: () => Promise<boolean>
  toggleFullscreenWindow: () => Promise<boolean>
  getPackageInstallerState: () => Promise<AnalytixInstallerState>
  installPackage: (params?: { installPath?: string }) => Promise<AnalytixInstallerState>
  uninstallPackage: () => Promise<AnalytixInstallerState>
  onPackageInstallerState: (handler: (state: AnalytixInstallerState) => void) => () => void
  pickDirectory: () => Promise<string>
}

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      webview: DetailedHTMLProps<HTMLAttributes<HTMLElement>, HTMLElement> & {
        allowpopups?: string
        partition?: string
        src?: string
        webpreferences?: string
      }
    }
  }
}

declare global {
  interface Window {
    analytixInstaller?: AnalytixInstallerBridge
    __analytixReleaseInstallerPrepaint?: () => boolean
  }
}
