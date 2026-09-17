// 必须是第一个 import:把旧品牌前缀的 localStorage 键拷贝到新前缀,
// 后面的 store 模块在 import 阶段就会读这些键。
import './lib/legacy-local-storage-migration'
import { createWriteShutdownHandler } from './write/write-shutdown'
import React from 'react'
import ReactDOM from 'react-dom/client'
import '@fontsource/noto-serif-sc/chinese-simplified-400.css'
import '@fontsource/noto-serif-sc/chinese-simplified-700.css'
import { DEFAULT_APP_MOTION_PREFERENCE } from '@shared/app-settings'
import './index.css'
import './styles/base-shell.css'
import './styles/surfaces-write.css'
import './styles/markdown-code.css'
import './styles/write-editor.css'
import './styles/write-rich-editor.css'
import App from './App'
import './i18n'
import { applyCursorSpotlight, applyMotionPreference } from './lib/apply-theme'
import { installCursorSpotlightTracking } from './lib/cursor-spotlight'
import { desktopQueryCache } from './lib/desktop-query-cache'

async function installBrowserPreviewBridgeIfNeeded(): Promise<void> {
  if (typeof window.analytix !== 'undefined') return
  const { installBrowserAnalytixBridge } = await import('./lib/browser-analytix-bridge')
  installBrowserAnalytixBridge()
}

async function startRenderer(): Promise<void> {
  await installBrowserPreviewBridgeIfNeeded()
  window.analytix.write.onShutdown(createWriteShutdownHandler())
  desktopQueryCache.installBridge()
  document.documentElement.dataset.platform = window.analytix?.app?.platform ?? 'unknown'
  applyCursorSpotlight(true)
  applyMotionPreference(DEFAULT_APP_MOTION_PREFERENCE)
  installCursorSpotlightTracking()

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}

void startRenderer()
