import { describe, expect, it } from 'vitest'
import mainSource from '../main.tsx?raw'
import startupAuthGateSource from '../account/StartupAuthGate.tsx?raw'
import source from './SettingsView.tsx?raw'
import sectionsSource from './settings-sections.tsx?raw'
import acceptedSlotDisplaySource from './chat/AcceptedSlotDisplay.tsx?raw'
import floatingComposerSource from './chat/FloatingComposer.tsx?raw'
import floatingComposerModelPickerSource from './chat/FloatingComposerModelPicker.tsx?raw'
import sidebarSource from './chat/Sidebar.tsx?raw'
import sidebarAccountMenuSource from './chat/SidebarAccountMenu.tsx?raw'

describe('Settings Hub compatibility boundary', () => {
  it('keeps ordinary Settings and local Provider settings free of static Hub account imports', () => {
    expect(source).not.toContain("from '@shared/hub-account'")
    expect(source).not.toContain('AccountProfileSettingsSection,')
    expect(source).not.toContain('HubManagedProvidersSection')
    expect(sectionsSource).not.toContain("export { AccountProfileSettingsSection } from './settings-section-account'")
    expect(source).toContain('<ProvidersSettingsSection ctx={settingsSectionContext} />')
  })

  it('loads preserved Hub account behavior only inside the explicit deprecated category', () => {
    expect(source).toContain("lazy(async () =>")
    expect(source).toContain("await import('./settings-section-account')")
    expect(source).toContain("category === 'account'")
    expect(source).toContain('<DeprecatedHubCompatibilitySection ctx={settingsSectionContext} />')
  })

  it('does not invoke the deprecated Hub account service before an explicit account-menu action', () => {
    expect(sidebarAccountMenuSource).not.toMatch(
      /useEffect\(\(\) => \{\s*if \(snapshot\) return\s*void refresh\(\)/u
    )
    expect(sidebarAccountMenuSource).toContain('const handleToggleAccountMenu = () => {')
    expect(sidebarAccountMenuSource).toContain('if (opening && !snapshot)')
  })

  it('does not invoke the deprecated Hub marketplace service before an explicit mention action', () => {
    expect(floatingComposerSource).toContain("if (route === 'claw')")
    expect(floatingComposerSource).toContain('if (!showAtMentionMenu) {')
    expect(floatingComposerSource).not.toMatch(
      /useEffect\(\(\) => \{\s*if \(route === 'claw'\)[\s\S]*?void sync\(\{ mode: 'cache' \}\)[\s\S]*?\}, \[route\]\)/u
    )
  })

  it('keeps the ordinary renderer entry and workspace graph free of static Hub authority', () => {
    const ordinarySources = [
      mainSource,
      startupAuthGateSource,
      source,
      sidebarSource,
      acceptedSlotDisplaySource,
      floatingComposerModelPickerSource
    ]
    for (const ordinarySource of ordinarySources) {
      for (const forbidden of [
        "from '@shared/hub-account'",
        'from "@shared/hub-account"',
        'hub-account-store',
        'AnimatedHubLoginPage',
        'SidebarAccountMenu',
        'HUB_MODEL_PROVIDER_ID',
        'analytix-hub'
      ]) {
        expect(ordinarySource).not.toContain(forbidden)
      }
    }
  })

  it('loads the browser preview compatibility bridge only after detecting a missing Electron bridge', () => {
    expect(mainSource).not.toContain(
      "import { installBrowserAnalytixBridge } from './lib/browser-analytix-bridge'"
    )
    const guard = mainSource.indexOf("if (typeof window.analytix !== 'undefined') return")
    const dynamicImport = mainSource.indexOf("await import('./lib/browser-analytix-bridge')")
    const guardedInstall = mainSource.indexOf('await installBrowserPreviewBridgeIfNeeded()')
    const bridgeConsumer = mainSource.indexOf('desktopQueryCache.installBridge()')
    const render = mainSource.indexOf('ReactDOM.createRoot')

    expect(guard).toBeGreaterThanOrEqual(0)
    expect(dynamicImport).toBeGreaterThan(guard)
    expect(mainSource.match(/import\('\.\/lib\/browser-analytix-bridge'\)/g)).toHaveLength(1)
    expect(guardedInstall).toBeGreaterThan(dynamicImport)
    expect(bridgeConsumer).toBeGreaterThan(guardedInstall)
    expect(render).toBeGreaterThan(bridgeConsumer)
  })
})
