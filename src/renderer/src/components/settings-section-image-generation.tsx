import { useState, useEffect, type ReactElement } from 'react'
import {
  CUSTOM_IMAGE_GENERATION_PROVIDER_ID,
  DEFAULT_IMAGE_GENERATION_PROTOCOL,
  IMAGE_GENERATION_PROTOCOLS,
  resolveAnalytixImageGenerationSettings
} from '@shared/app-settings'
import { InlineNoticeView, ModelSelect, SettingsCard, SettingRow, Toggle } from './settings-controls'

const DEFAULT_IMAGE_GENERATION = {
  enabled: false,
  providerId: '',
  protocol: DEFAULT_IMAGE_GENERATION_PROTOCOL,
  baseUrl: '',
  model: '',
  defaultSize: '',
  timeoutMs: 180000
}

export function ImageGenerationSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    form,
    provider,
    analytix,
    selectControlClass,
    updateAnalytix
  } = ctx
  const imageGeneration = {
    ...DEFAULT_IMAGE_GENERATION,
    ...(analytix.imageGeneration ?? {})
  }
  const effectiveImageGeneration = form
    ? resolveAnalytixImageGenerationSettings(form)
    : imageGeneration
  const imageProviders = (provider?.providers ?? []).filter((item: {
    image?: unknown
  }) => Boolean(item.image))
  const selectedProviderId = imageGeneration.providerId || CUSTOM_IMAGE_GENERATION_PROVIDER_ID
  const selectedImageProvider = imageProviders.find((item: { id: string }) => item.id === selectedProviderId)
  const usingCustomProvider = selectedProviderId === CUSTOM_IMAGE_GENERATION_PROVIDER_ID || !selectedImageProvider
  const selectedProviderImage = selectedImageProvider?.image
  const imageModelOptions = usingCustomProvider
    ? []
    : selectedProviderImage?.models ?? []
  const [defaultSizeInput, setDefaultSizeInput] = useState(imageGeneration.defaultSize)
  useEffect(() => {
    setDefaultSizeInput(imageGeneration.defaultSize)
  }, [imageGeneration.defaultSize])
  const updateImageGeneration = (patch: Record<string, unknown>): void => {
    updateAnalytix({
      imageGeneration: {
        ...imageGeneration,
        ...patch
      }
    })
  }

  return (
    <SettingsCard title={t('imageGen')}>
      <SettingRow
        title={t('imageGenEnabled')}
        description={t('imageGenEnabledDesc')}
        control={
          <Toggle
            checked={imageGeneration.enabled}
            onChange={(enabled) => updateImageGeneration({ enabled })}
          />
        }
      />
      {imageGeneration.enabled ? (
        <>
          <SettingRow
            title={t('imageGenProvider')}
            description={t('imageGenProviderDesc')}
            control={
              <div className="w-full min-w-0 md:max-w-md">
                <select
                  className={selectControlClass}
                  value={usingCustomProvider ? CUSTOM_IMAGE_GENERATION_PROVIDER_ID : selectedProviderId}
                  onChange={(e) => {
                    const providerId = e.target.value
                    const nextProvider = imageProviders.find((item: { id: string }) => item.id === providerId)
                    updateImageGeneration({
                      providerId,
                      protocol: providerId === CUSTOM_IMAGE_GENERATION_PROVIDER_ID
                        ? imageGeneration.protocol
                        : nextProvider?.image?.protocol ?? DEFAULT_IMAGE_GENERATION_PROTOCOL,
                      model: providerId === CUSTOM_IMAGE_GENERATION_PROVIDER_ID
                        ? imageGeneration.model
                        : nextProvider?.image?.models?.[0] ?? ''
                    })
                  }}
                >
                  {imageProviders.map((item: { id: string; name: string }) => (
                    <option key={item.id} value={item.id}>{item.name}</option>
                  ))}
                  <option value={CUSTOM_IMAGE_GENERATION_PROVIDER_ID}>{t('imageGenProviderCustom')}</option>
                </select>
              </div>
            }
          />
          {usingCustomProvider ? (
            <>
              <SettingRow
                title={t('imageGenProtocol')}
                description={t('imageGenProtocolDesc')}
                control={
                  <select
                    className={selectControlClass}
                    value={imageGeneration.protocol}
                    onChange={(e) => updateImageGeneration({ protocol: e.target.value })}
                  >
                    {IMAGE_GENERATION_PROTOCOLS.map((protocol) => (
                      <option key={protocol} value={protocol}>
                        {t(protocol === 'minimax-image' ? 'imageGenProtocolMiniMax' : 'imageGenProtocolOpenAi')}
                      </option>
                    ))}
                  </select>
                }
              />
              <SettingRow
                title={t('imageGenBaseUrl')}
                description={t('imageGenBaseUrlDesc')}
                control={
                  <input
                    className="w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30 md:max-w-md"
                    value={imageGeneration.baseUrl}
                    placeholder={t('imageGenBaseUrlPlaceholder')}
                    onChange={(e) => updateImageGeneration({ baseUrl: e.target.value })}
                  />
                }
              />
            </>
          ) : null}
          <SettingRow
            title={t('imageGenModel')}
            description={t('imageGenModelDesc')}
            control={
              <div className="w-full min-w-0 md:max-w-md">
                {usingCustomProvider ? (
                  <input
                    className="w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                    value={imageGeneration.model}
                    placeholder={t('imageGenModelPlaceholder')}
                    onChange={(e) => updateImageGeneration({ model: e.target.value })}
                  />
                ) : (
                  <ModelSelect
                    value={imageModelOptions.includes(imageGeneration.model) ? imageGeneration.model : ''}
                    options={imageModelOptions}
                    defaultLabel={t('modelSelectDefaultOption', {
                      model: imageModelOptions[0] ?? ''
                    })}
                    selectClassName={selectControlClass}
                    onChange={(model) => updateImageGeneration({ model })}
                  />
                )}
              </div>
            }
          />
          <div className="px-3 py-3">
            <InlineNoticeView notice={{ tone: 'info', message: t('imageGenModelQualityHint') }} />
          </div>
          <SettingRow
            title={t('imageGenDefaultSize')}
            description={t('imageGenDefaultSizeDesc')}
            control={
              <input
                className="w-40 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                value={defaultSizeInput}
                placeholder="1024x1024"
                onChange={(e) => setDefaultSizeInput(e.target.value)}
                onBlur={() => updateImageGeneration({ defaultSize: defaultSizeInput })}
              />
            }
          />
          <SettingRow
            title={t('imageGenTimeout')}
            description={t('imageGenTimeoutDesc')}
            control={
              <input
                type="number"
                min={10000}
                max={600000}
                step={10000}
                className="w-32 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30"
                value={imageGeneration.timeoutMs}
                onChange={(e) => updateImageGeneration({ timeoutMs: Number(e.target.value) })}
              />
            }
          />
        </>
      ) : null}
    </SettingsCard>
  )
}
