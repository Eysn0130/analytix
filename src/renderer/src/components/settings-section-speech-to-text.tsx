import { useState, type ReactElement } from 'react'
import {
  CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID,
  DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
  SPEECH_TO_TEXT_PROTOCOLS
} from '@shared/app-settings'
import { Loader2, PlugZap } from 'lucide-react'
import {
  AdvancedSettingsDisclosure,
  InlineNoticeView,
  ModelSelect,
  SettingsCard,
  SettingRow,
  Toggle,
  type InlineNotice
} from './settings-controls'
import { useChatStore } from '../store/chat-store'

const SPEECH_LANGUAGE_OPTIONS: readonly string[] = ['', 'zh', 'en', 'ja', 'ko']

/**
 * 0.5s 440Hz mono 16kHz sine tone — enough for the ASR endpoint to accept the
 * request and prove auth + base URL + model are wired correctly.
 */
function buildTestToneWavBase64(): string {
  const sampleRate = 16_000
  const sampleCount = sampleRate / 2
  const dataBytes = sampleCount * 2
  const buffer = new ArrayBuffer(44 + dataBytes)
  const view = new DataView(buffer)
  const writeAscii = (offset: number, text: string): void => {
    for (let i = 0; i < text.length; i++) view.setUint8(offset + i, text.charCodeAt(i))
  }
  writeAscii(0, 'RIFF')
  view.setUint32(4, 36 + dataBytes, true)
  writeAscii(8, 'WAVEfmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, sampleRate, true)
  view.setUint32(28, sampleRate * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  writeAscii(36, 'data')
  view.setUint32(40, dataBytes, true)
  for (let i = 0; i < sampleCount; i++) {
    view.setInt16(44 + i * 2, Math.round(8000 * Math.sin((2 * Math.PI * 440 * i) / sampleRate)), true)
  }
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.length; i += 8192) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 8192))
  }
  return btoa(binary)
}

const DEFAULT_SPEECH_TO_TEXT = {
  enabled: false,
  providerId: '',
  protocol: DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
  baseUrl: '',
  model: '',
  language: '',
  timeoutMs: 60000
}

export function SpeechToTextSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    form,
    provider,
    analytix,
    selectControlClass,
    updateAnalytix
  } = ctx
  const speechToText = {
    ...DEFAULT_SPEECH_TO_TEXT,
    ...(analytix.speechToText ?? {})
  }
  const speechProviders = (provider?.providers ?? []).filter((item: {
    speech?: unknown
  }) => Boolean(item.speech))
  const selectedProviderId = speechToText.providerId || CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID
  const selectedSpeechProvider = speechProviders.find((item: { id: string }) => item.id === selectedProviderId)
  const usingCustomProvider = selectedProviderId === CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID || !selectedSpeechProvider
  const selectedProviderSpeech = selectedSpeechProvider?.speech
  const speechModelOptions = usingCustomProvider
    ? []
    : selectedProviderSpeech?.models ?? []
  const [testState, setTestState] = useState<'idle' | 'busy' | InlineNotice>('idle')
  const showTopNotice = useChatStore((s) => s.showTopNotice)
  const finishSpeechTest = (notice: InlineNotice): void => {
    setTestState(notice)
    showTopNotice({ tone: notice.tone, message: notice.message })
  }
  const updateSpeechToText = (patch: Record<string, unknown>): void => {
    updateAnalytix({
      speechToText: {
        ...speechToText,
        ...patch
      }
    })
  }

  const runSpeechTest = async (): Promise<void> => {
    if (typeof window.analytix?.speech?.transcribe !== 'function') return
    setTestState('busy')
    try {
      const result = await window.analytix.speech.transcribe({
        audioBase64: buildTestToneWavBase64(),
        mimeType: 'audio/wav',
        durationMs: 500
      })
      if (result.ok) {
        finishSpeechTest({ tone: 'success', message: t('speechToTextTestSuccess', { text: result.text }) })
      } else if (result.message === 'transcription result is empty') {
        // 测试音是一段正弦音,模型可能返回空转写——鉴权和链路本身是通的。
        finishSpeechTest({ tone: 'success', message: t('speechToTextTestEmptyOk') })
      } else {
        finishSpeechTest({ tone: 'error', message: t('speechToTextTestFailed', { message: result.message }) })
      }
    } catch (error) {
      finishSpeechTest({
        tone: 'error',
        message: t('speechToTextTestFailed', {
          message: error instanceof Error ? error.message : String(error)
        })
      })
    }
  }

  return (
    <SettingsCard title={t('speechToText')}>
      <SettingRow
        title={t('speechToTextEnabled')}
        description={t('speechToTextEnabledDesc')}
        control={
          <Toggle
            checked={speechToText.enabled}
            onChange={(enabled) => {
              // 首次开启时直接选中第一个带语音能力的供应商,
              // 避免落进字段全空的「自定义」模式。providerId 为空但已填过
              // baseUrl/model 说明用户在用隐式自定义配置,不能覆盖。
              const firstSpeechProvider = speechProviders[0]
              const customUntouched =
                !speechToText.baseUrl.trim() && !speechToText.model.trim()
              if (enabled && !speechToText.providerId.trim() && customUntouched && firstSpeechProvider) {
                updateSpeechToText({
                  enabled,
                  providerId: firstSpeechProvider.id,
                  baseUrl: '',
                  protocol: firstSpeechProvider.speech?.protocol ?? DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
                  model: firstSpeechProvider.speech?.models?.[0] ?? ''
                })
                return
              }
              updateSpeechToText({ enabled })
            }}
          />
        }
      />
      {speechToText.enabled ? (
        <>
          <SettingRow
            title={t('speechToTextProvider')}
            description={t('speechToTextProviderDesc')}
            control={
              <div className="w-full min-w-0 md:max-w-md">
                <select
                  className={selectControlClass}
                  value={usingCustomProvider ? CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID : selectedProviderId}
                  onChange={(e) => {
                    const providerId = e.target.value
                    const nextProvider = speechProviders.find((item: { id: string }) => item.id === providerId)
                    updateSpeechToText({
                      providerId,
                      baseUrl: providerId === CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID ? speechToText.baseUrl : '',
                      protocol: providerId === CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID
                        ? speechToText.protocol
                        : nextProvider?.speech?.protocol ?? DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
                      model: providerId === CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID
                        ? speechToText.model
                        : nextProvider?.speech?.models?.[0] ?? ''
                    })
                  }}
                >
                  {speechProviders.map((item: { id: string; name: string }) => (
                    <option key={item.id} value={item.id}>{item.name}</option>
                  ))}
                  <option value={CUSTOM_SPEECH_TO_TEXT_PROVIDER_ID}>{t('speechToTextProviderCustom')}</option>
                </select>
              </div>
            }
          />
          {usingCustomProvider ? (
            <>
              <SettingRow
                title={t('speechToTextProtocol')}
                description={t('speechToTextProtocolDesc')}
                control={
                  <select
                    className={selectControlClass}
                    value={speechToText.protocol}
                    onChange={(e) => updateSpeechToText({ protocol: e.target.value })}
                  >
                    {SPEECH_TO_TEXT_PROTOCOLS.map((protocol) => (
                      <option key={protocol} value={protocol}>
                        {t(protocol === 'mimo-asr' ? 'speechProtocolMimoAsr' : 'speechProtocolOpenAi')}
                      </option>
                    ))}
                  </select>
                }
              />
              <SettingRow
                title={t('speechToTextBaseUrl')}
                description={t('speechToTextBaseUrlDesc')}
                control={
                  <input
                    className="w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30 md:max-w-md"
                    value={speechToText.baseUrl}
                    placeholder={t('speechToTextBaseUrlPlaceholder')}
                    onChange={(e) => updateSpeechToText({ baseUrl: e.target.value })}
                  />
                }
              />
            </>
          ) : null}
          <SettingRow
            title={t('speechToTextModel')}
            description={t('speechToTextModelDesc')}
            control={
              <div className="w-full min-w-0 md:max-w-md">
                {usingCustomProvider ? (
                  <input
                    className="w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                    value={speechToText.model}
                    placeholder={t('speechToTextModelPlaceholder')}
                    onChange={(e) => updateSpeechToText({ model: e.target.value })}
                  />
                ) : (
                  <ModelSelect
                    value={speechModelOptions.includes(speechToText.model) ? speechToText.model : ''}
                    options={speechModelOptions}
                    defaultLabel={t('modelSelectDefaultOption', {
                      model: speechModelOptions[0] ?? ''
                    })}
                    selectClassName={selectControlClass}
                    onChange={(model) => updateSpeechToText({ model })}
                  />
                )}
              </div>
            }
          />
          <SettingRow
            title={t('speechToTextLanguage')}
            description={t('speechToTextLanguageDesc')}
            control={
              <select
                className={selectControlClass}
                value={speechToText.language}
                onChange={(e) => updateSpeechToText({ language: e.target.value })}
              >
                {SPEECH_LANGUAGE_OPTIONS.map((language) => (
                  <option key={language || 'auto'} value={language}>
                    {t(`speechLanguage_${language || 'auto'}`)}
                  </option>
                ))}
                {!SPEECH_LANGUAGE_OPTIONS.includes(speechToText.language) ? (
                  <option value={speechToText.language}>{speechToText.language}</option>
                ) : null}
              </select>
            }
          />
          <div className="px-3 py-4">
            <AdvancedSettingsDisclosure
              title={t('speechToTextAdvanced')}
              description={t('speechToTextAdvancedDesc')}
            >
              <div className="divide-y divide-ds-border-muted">
                <SettingRow
                  title={t('speechToTextTimeout')}
                  description={t('speechToTextTimeoutDesc')}
                  control={
                    <input
                      type="number"
                      min={5000}
                      max={600000}
                      step={5000}
                      className="w-32 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                      value={speechToText.timeoutMs}
                      onChange={(e) => updateSpeechToText({ timeoutMs: Number(e.target.value) })}
                    />
                  }
                />
              </div>
            </AdvancedSettingsDisclosure>
          </div>
          <SettingRow
            title={t('speechToTextTest')}
            description={t('speechToTextTestDesc')}
            control={
              <div className="flex w-full min-w-0 flex-col gap-2 md:max-w-md">
                <button
                  type="button"
                  disabled={testState === 'busy'}
                  onClick={() => void runSpeechTest()}
                  className="inline-flex h-9 w-fit items-center gap-1.5 rounded-full border border-ds-border bg-ds-card px-3 text-[12.5px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {testState === 'busy'
                    ? <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
                    : <PlugZap className="h-3.5 w-3.5" strokeWidth={1.9} />}
                  {testState === 'busy' ? t('speechToTextTesting') : t('speechToTextTestAction')}
                </button>
                {typeof testState === 'object' ? <InlineNoticeView notice={testState} /> : null}
              </div>
            }
          />
        </>
      ) : null}
    </SettingsCard>
  )
}
