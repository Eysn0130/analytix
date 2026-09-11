import { describe, expect, it } from 'vitest'
import startupAuthGateSource from './StartupAuthGate.tsx?raw'
describe('StartupAuthGate local Provider routing', () => {
  it('prewarms only the local shell and boots local Provider readiness', () => {
    expect(startupAuthGateSource).toContain('prewarmStartupSurfaces()')
    expect(startupAuthGateSource).toContain('useChatStore.getState().boot()')
    expect(startupAuthGateSource).not.toContain('initialize()')
    expect(startupAuthGateSource).not.toContain('refresh()')
  })

  it('keeps the ordinary startup module graph Hub-cold', () => {
    for (const forbidden of [
      "from '@shared/hub-account'",
      "from './AnimatedHubLoginPage'",
      "from './hub-account-store'",
      'useHubAccountStore',
      'HubAccountSnapshot',
      'HUB LOGIN',
      'HUB GATEWAY'
    ]) {
      expect(startupAuthGateSource).not.toContain(forbidden)
    }
  })
})
