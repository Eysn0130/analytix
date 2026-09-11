import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import type { ClawImChannelV1 } from '@shared/app-settings'
import i18n from '../../i18n'
import {
  ConnectPhoneSidebarPanel,
  ConnectPhoneView,
  connectPhoneInstallRequestOptions,
  connectPhoneProviderForTarget,
  createConnectPhoneAgentProfile,
  createConnectPhoneChannelOptions,
  createConnectPhoneCredential,
  createConnectPhoneTelegramCredential,
  formatConnectPhoneUserCode,
  hasClawPhoneChannel,
  hasEnabledClawPhoneChannel
} from './ConnectPhoneView'

function channel(enabled: boolean, provider: ClawImChannelV1['provider'] = 'feishu'): ClawImChannelV1 {
  return {
    id: `${provider}-${enabled ? 'enabled' : 'disabled'}`,
    provider,
    label: enabled ? 'Enabled' : 'Disabled',
    enabled,
    model: 'auto',
    threadId: '',
    workspaceRoot: '',
    agentProfile: {
      name: 'analytix',
      description: '',
      identity: '',
      personality: '',
      userContext: '',
      replyRules: ''
    },
    conversations: [],
    createdAt: '2026-06-03T00:00:00.000Z',
    updatedAt: '2026-06-03T00:00:00.000Z'
  }
}

describe('ConnectPhoneView', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('renders the dedicated phone connection page before a channel is enabled', () => {
    const html = renderToStaticMarkup(
      createElement(ConnectPhoneView, {
        channels: [],
        onAddProvider: async () => undefined
      })
    )

    expect(html).toContain('Connect analytix with WeChat')
    expect(html).toContain('ds-home-transition')
    expect(html).toContain('ds-home-transition-stage')
    expect(html).toContain('Generate authorization QR')
    expect(html).not.toContain('Analytix usage')
  })

  it('maps scan targets to the matching install API provider', () => {
    expect(connectPhoneProviderForTarget('feishu')).toBe('feishu')
    expect(connectPhoneProviderForTarget('lark')).toBe('feishu')
    expect(connectPhoneProviderForTarget('weixin')).toBe('weixin')
    expect(connectPhoneProviderForTarget('telegram')).toBe('telegram')
    expect(connectPhoneInstallRequestOptions('feishu')).toEqual({
      provider: 'feishu',
      options: { isLark: false }
    })
    expect(connectPhoneInstallRequestOptions('lark')).toEqual({
      provider: 'feishu',
      options: { isLark: true }
    })
    expect(connectPhoneInstallRequestOptions('weixin')).toEqual({
      provider: 'weixin'
    })
  })

  it('formats the official user code instead of the opaque device code', () => {
    expect(formatConnectPhoneUserCode('YWAZ-ZZ8P', 'v1:opaque-device-code')).toBe('YWAZ-ZZ8P')
    expect(formatConnectPhoneUserCode('', 'abcd1234-rest-of-token')).toBe('ABCD-1234')
  })

  it('builds the default analytix channel payload after a successful scan', () => {
    expect(createConnectPhoneAgentProfile()).toEqual({
      name: 'analytix',
      description: '',
      identity: '',
      personality: '',
      userContext: '',
      replyRules: ''
    })
    expect(createConnectPhoneChannelOptions()).toEqual({
      model: 'auto',
      enabled: true,
      im: {
        enabled: true,
        provider: 'weixin'
      }
    })
    expect(createConnectPhoneChannelOptions('weixin')).toEqual({
      model: 'auto',
      enabled: true,
      im: {
        enabled: true,
        provider: 'weixin'
      }
    })
    expect(createConnectPhoneChannelOptions('telegram')).toEqual({
      model: 'auto',
      enabled: true,
      im: {
        enabled: true,
        provider: 'telegram'
      }
    })
    expect(
      createConnectPhoneCredential(
        {
          done: true,
          kind: 'feishu',
          appId: 'cli_a',
          domain: 'lark',
          channelId: 'feishu-channel',
          credentialConfigured: true,
          settingsCommitted: true
        },
        '2026-06-03T01:02:03.000Z'
      )
    ).toEqual({
      kind: 'feishu',
      accountId: 'cli_a',
      appId: 'cli_a',
      domain: 'lark',
      createdAt: '2026-06-03T01:02:03.000Z'
    })
    expect(
      createConnectPhoneCredential(
        {
          done: true,
          kind: 'weixin',
          accountId: 'wx_account',
          channelId: 'weixin-channel',
          credentialConfigured: true,
          settingsCommitted: true
        },
        '2026-06-03T01:02:03.000Z'
      )
    ).toEqual({
      kind: 'weixin',
      accountId: 'wx_account',
      createdAt: '2026-06-03T01:02:03.000Z'
    })
    expect(
      createConnectPhoneTelegramCredential(
        'telegram-channel',
        ' 1001, 1002 ',
        '@analytix_bot',
        '2026-06-03T01:02:03.000Z'
      )
    ).toEqual({
      kind: 'telegram',
      accountId: 'telegram-channel',
      allowedChatIds: '1001, 1002',
      botUsername: 'analytix_bot',
      createdAt: '2026-06-03T01:02:03.000Z'
    })
  })

  it('treats only enabled channels for the selected provider as connected phone channels', () => {
    expect(hasEnabledClawPhoneChannel([])).toBe(false)
    expect(hasEnabledClawPhoneChannel([channel(false)])).toBe(false)
    expect(hasEnabledClawPhoneChannel([channel(false), channel(true)])).toBe(true)
    expect(hasEnabledClawPhoneChannel([channel(true, 'weixin')], 'feishu')).toBe(false)
    expect(hasEnabledClawPhoneChannel([channel(true, 'weixin')], 'weixin')).toBe(true)
    expect(hasEnabledClawPhoneChannel([channel(true, 'telegram')], 'telegram')).toBe(true)
  })

  it('reserves only the selected provider slot once a channel exists', () => {
    expect(hasClawPhoneChannel([])).toBe(false)
    expect(hasClawPhoneChannel([channel(false)])).toBe(true)
    expect(hasClawPhoneChannel([channel(true)])).toBe(true)
    expect(hasClawPhoneChannel([channel(true, 'feishu')], 'weixin')).toBe(false)
    expect(hasClawPhoneChannel([channel(true, 'weixin')], 'weixin')).toBe(true)
    expect(hasClawPhoneChannel([channel(true, 'telegram')], 'telegram')).toBe(true)
  })

  it('shows settings and disconnect actions for an existing phone connection', () => {
    const html = renderToStaticMarkup(
      createElement(ConnectPhoneSidebarPanel, {
        channels: [channel(true, 'weixin')],
        onAddProvider: async () => undefined,
        onDisconnect: async () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('Phone connection settings')
    expect(html).toContain('Disconnect phone')
    expect(html).not.toContain('Add WeChat')
  })

  it('hides the redundant provider selector in the phone connection sidebar', () => {
    const html = renderToStaticMarkup(
      createElement(ConnectPhoneSidebarPanel, {
        channels: [channel(true, 'feishu'), channel(true, 'weixin')],
        onAddProvider: async () => undefined,
        onDisconnect: async () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('WeChat')
    expect(html).toContain('Connect phone')
    expect(html).not.toContain('aria-pressed')
    expect(html).not.toContain('Feishu / Lark')
    expect(html).not.toContain('Telegram')
  })
})
