import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement
} from 'react'
import { createPortal } from 'react-dom'
import {
  Check,
  ChevronDown,
  ChevronRight
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  MODEL_REASONING_EFFORTS,
  isComposerChatModelId,
  modelProfileSupportsTextChat,
  modelSupportsImageInput,
  type ModelReasoningEffort,
  type ModelProviderModelProfileV1
} from '@shared/app-settings'
import type { ModelProviderModelGroup } from '@shared/analytix-api'
import { projectModelReasoningEffortV1 } from '@shared/model-reasoning-effort'
import { modelLabelFromCatalog, providerModelDisplayName, type ProviderEndpointKind } from '@shared/provider-display'
import './model-picker.css'

export type ComposerReasoningEffort = ModelReasoningEffort

type Props = {
  compact: boolean
  mode: 'select' | 'combobox'
  composerModel: string
  composerProviderId?: string
  composerPickList: string[]
  composerModelGroups?: ModelProviderModelGroup[]
  canChangeModel: boolean
  stretch?: boolean
  composerReasoningEffort?: string
  onComposerModelChange: (modelId: string, providerId?: string) => void
  onComposerReasoningEffortChange?: (effort: ComposerReasoningEffort) => void
  onConfigureProviders?: () => void
}

const REASONING_OPTIONS: Array<{ id: ComposerReasoningEffort; labelKey: string }> = [
  { id: 'auto', labelKey: 'composerReasoningAuto' },
  { id: 'off', labelKey: 'composerReasoningOff' },
  { id: 'low', labelKey: 'composerReasoningLow' },
  { id: 'medium', labelKey: 'composerReasoningMedium' },
  { id: 'high', labelKey: 'composerReasoningHigh' },
  { id: 'max', labelKey: 'composerReasoningMax' }
]
const LEGACY_REASONING_EFFORTS: ComposerReasoningEffort[] = ['off', 'low', 'medium', 'high', 'max']

type FloatingMenuPlacement = {
  left: number
  top: number
  width: number
  maxHeight: number
}

type FloatingSubmenuPlacement = {
  left: number
  top: number
  width: number
  maxHeight: number
}

type FloatingMenuAnchorRect = Pick<DOMRect, 'bottom' | 'right' | 'top'>
type FloatingSubmenuAnchorRect = Pick<DOMRect, 'bottom' | 'left' | 'right' | 'top'>

type ComposerModelMenuGroup = {
  menuId: string
  providerId: string
  label: string
  endpointKind?: ProviderEndpointKind
  subtitleKey?: string
  modelIds: string[]
  modelLabels?: Record<string, string>
  modelProfiles?: Record<string, ModelProviderModelProfileV1>
}

type ComposerModelFamily = {
  id: 'deepseek' | 'mimo' | 'qwen'
  label: string
  subtitleKey: string
}

const FLOATING_MENU_MARGIN = 12
const FLOATING_MENU_GAP = 7
const FLOATING_MENU_WIDTH = 208
const FLOATING_MENU_MIN_WIDTH = 176
const FLOATING_MENU_MIN_HEIGHT = 112
const FLOATING_MENU_MAX_HEIGHT = 424
const FLOATING_SUBMENU_GAP = 6
const FLOATING_SUBMENU_WIDTH = 232
const FLOATING_SUBMENU_MIN_HEIGHT = 80
const FLOATING_SUBMENU_MAX_HEIGHT = 320
const UNGROUPED_MODEL_PROVIDER_ID = '__composer_models__'
const DEEPSEEK_MODEL_FAMILY: ComposerModelFamily = {
  id: 'deepseek',
  label: 'DeepSeek',
  subtitleKey: 'composerModelFamilyDeepseek'
}
const MIMO_MODEL_FAMILY: ComposerModelFamily = {
  id: 'mimo',
  label: 'MiMo',
  subtitleKey: 'composerModelFamilyMimo'
}
const QWEN_MODEL_FAMILY: ComposerModelFamily = {
  id: 'qwen',
  label: 'Qwen',
  subtitleKey: 'composerModelFamilyQwen'
}
const COMPOSER_MODEL_FAMILIES: ComposerModelFamily[] = [
  DEEPSEEK_MODEL_FAMILY,
  MIMO_MODEL_FAMILY,
  QWEN_MODEL_FAMILY
]
const HIDDEN_COMPOSER_REASONING_EFFORTS = new Set<ComposerReasoningEffort>(['off'])

export function FloatingComposerModelPicker({
  compact,
  mode,
  composerModel,
  composerProviderId = '',
  composerPickList,
  composerModelGroups = [],
  canChangeModel,
  stretch = false,
  composerReasoningEffort = 'max',
  onComposerModelChange,
  onComposerReasoningEffortChange,
  onConfigureProviders
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const pickerRef = useRef<HTMLElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const submenuRef = useRef<HTMLDivElement | null>(null)
  const providerRowRefs = useRef<Map<string, HTMLButtonElement>>(new Map())
  const [menuOpen, setMenuOpen] = useState(false)
  const [activeProviderId, setActiveProviderId] = useState<string | null>(null)
  const [menuPlacement, setMenuPlacement] = useState<FloatingMenuPlacement | null>(null)
  const [submenuPlacement, setSubmenuPlacement] = useState<FloatingSubmenuPlacement | null>(null)
  const modelOptions = useMemo(() => {
    const ordered = new Set<string>()
    for (const id of composerPickList) {
      const normalized = id.trim()
      if (normalized) ordered.add(normalized)
    }
    const current = composerModel.trim()
    if (current) ordered.add(current)
    return [...ordered]
  }, [composerModel, composerPickList])
  const providerMenuGroups = useMemo<ComposerModelMenuGroup[]>(() => {
    return buildComposerModelMenuGroups({
      composerModelGroups,
      modelOptions,
      ungroupedLabel: t('composerOtherModels')
    })
  }, [composerModelGroups, modelOptions, t])
  const currentModel = composerModel.trim()
  const explicitProviderId = composerProviderId.trim()
  const selectedProviderGroup = providerMenuGroups.find((group) =>
    (!explicitProviderId || group.providerId === explicitProviderId) &&
    group.modelIds.some((id) => modelIdsMatch(group, id, currentModel))
  ) ?? null
  const selectedProviderId = selectedProviderGroup?.providerId ?? null
  const providerUnavailable = Boolean(explicitProviderId && !selectedProviderGroup)
  const currentModelProfile = providerUnavailable ? undefined : modelProfileForSelection(providerMenuGroups, currentModel, selectedProviderId)
  const needsProviderSetup = shouldShowProviderSetupPrompt(providerMenuGroups)
  const reasoningOptions = composerReasoningMenuOptionsForModel(currentModelProfile)
  const reasoningEnabled =
    !needsProviderSetup && !providerUnavailable && Boolean(onComposerReasoningEffortChange) && reasoningOptions.length > 0
  const currentReasoning = normalizeComposerReasoningMenuEffort(
    composerReasoningEffort,
    currentModelProfile
  )
  const currentReasoningLabel = t(reasoningLabelKey(currentReasoning))
  const canOpenModelControls = canChangeModel || (needsProviderSetup && Boolean(onConfigureProviders))
  const modelLabel = needsProviderSetup && !providerUnavailable
    ? t('composerNoProvidersShort')
    : composerModelDisplayLabel(providerMenuGroups, currentModel, selectedProviderId, t('autoLabel'))
  const modelIdentity = needsProviderSetup && !providerUnavailable ? modelLabel : [
    selectedProviderGroup?.label ?? explicitProviderId,
    modelLabel,
    currentModel !== modelLabel ? currentModel : undefined
  ].filter(Boolean).join(' · ')
  const sourceLabel = providerUnavailable ? t('composerProviderUnavailable')
    : selectedProviderGroup?.endpointKind && selectedProviderGroup.endpointKind !== 'official-deepseek'
      ? t(`providerConnection_${selectedProviderGroup.endpointKind}`) : ''
  const controlsTitle = reasoningEnabled
    ? `${modelIdentity} / ${currentReasoningLabel}${sourceLabel ? ` · ${sourceLabel}` : ''}`
    : [modelIdentity, sourceLabel].filter(Boolean).join(' · ')
  const activeProviderGroup =
    providerMenuGroups.find((group) => group.menuId === activeProviderId) ?? null
  const activeProviderModelIds = activeProviderGroup?.modelIds ?? []
  const comboboxWidthClass = stretch
    ? 'min-w-0 flex-1 max-w-[284px]'
    : compact
      ? 'w-[184px] max-w-[184px] shrink-0'
      : 'w-[248px] max-w-[260px] shrink-0'

  useEffect(() => {
    if (!reasoningEnabled) return
    const rawReasoning = normalizeComposerReasoningEffortValue(composerReasoningEffort)
    if (rawReasoning !== currentReasoning) {
      onComposerReasoningEffortChange?.(currentReasoning)
    }
  }, [composerReasoningEffort, currentReasoning, onComposerReasoningEffortChange, reasoningEnabled])

  useEffect(() => {
    if (!menuOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (!(target instanceof Node)) return
      if (pickerRef.current?.contains(target)) return
      if (menuRef.current?.contains(target)) return
      if (submenuRef.current?.contains(target)) return
      setMenuOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    return () => window.removeEventListener('pointerdown', onPointerDown)
  }, [menuOpen])

  useEffect(() => {
    if (!menuOpen) return
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape' || event.isComposing) return
      event.preventDefault()
      event.stopPropagation()
      setMenuOpen(false)
      pickerRef.current?.querySelector('button')?.focus()
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [menuOpen])

  useEffect(() => {
    if (!menuOpen) {
      setMenuPlacement(null)
      setSubmenuPlacement(null)
      return
    }

    const updatePlacement = (): void => {
      const picker = pickerRef.current
      if (!picker) return

      setMenuPlacement(
        calculateFloatingMenuPlacement({
          anchorRect: picker.getBoundingClientRect(),
          menuHeight: menuRef.current?.offsetHeight ?? 0,
          viewportHeight: window.innerHeight,
          viewportWidth: window.innerWidth,
          coordinateScale: currentBodyZoom()
        })
      )
    }

    updatePlacement()
    window.addEventListener('resize', updatePlacement)
    window.addEventListener('scroll', updatePlacement, true)
    return () => {
      window.removeEventListener('resize', updatePlacement)
      window.removeEventListener('scroll', updatePlacement, true)
    }
  }, [menuOpen])

  useEffect(() => {
    if (!menuOpen) {
      setActiveProviderId(null)
      return
    }
    if (providerMenuGroups.length === 0) {
      setActiveProviderId(null)
      return
    }
    setActiveProviderId((current) => {
      if (current && providerMenuGroups.some((group) => group.menuId === current)) return current
      return null
    })
  }, [menuOpen, providerMenuGroups])

  useEffect(() => {
    if (!menuOpen || !activeProviderGroup) {
      setSubmenuPlacement(null)
      return
    }

    const updatePlacement = (): void => {
      const row = providerRowRefs.current.get(activeProviderGroup.menuId)
      if (!row) return

      setSubmenuPlacement(
        calculateFloatingSubmenuPlacement({
          anchorRect: row.getBoundingClientRect(),
          submenuHeight:
            submenuRef.current?.offsetHeight
            || estimatedModelSubmenuHeight(activeProviderModelIds.length),
          viewportHeight: window.innerHeight,
          viewportWidth: window.innerWidth,
          coordinateScale: currentBodyZoom()
        })
      )
    }

    updatePlacement()
    const menu = menuRef.current
    menu?.addEventListener('scroll', updatePlacement, true)
    window.addEventListener('resize', updatePlacement)
    window.addEventListener('scroll', updatePlacement, true)
    return () => {
      menu?.removeEventListener('scroll', updatePlacement, true)
      window.removeEventListener('resize', updatePlacement)
      window.removeEventListener('scroll', updatePlacement, true)
    }
  }, [activeProviderGroup, activeProviderModelIds.length, menuOpen])

  const menuStyle: CSSProperties = menuPlacement
    ? {
        left: `${menuPlacement.left}px`,
        top: `${menuPlacement.top}px`,
        width: `${menuPlacement.width}px`,
        maxHeight: `${menuPlacement.maxHeight}px`
      }
    : {
        left: 0,
        top: 0,
        width: `${FLOATING_MENU_WIDTH}px`,
        maxHeight: `${FLOATING_MENU_MAX_HEIGHT}px`,
        visibility: 'hidden'
      }

  const submenuStyle: CSSProperties = submenuPlacement
    ? {
        left: `${submenuPlacement.left}px`,
        top: `${submenuPlacement.top}px`,
        width: `${submenuPlacement.width}px`,
        maxHeight: `${submenuPlacement.maxHeight}px`
      }
    : {
        left: 0,
        top: 0,
        width: `${FLOATING_SUBMENU_WIDTH}px`,
        maxHeight: `${FLOATING_SUBMENU_MAX_HEIGHT}px`,
        visibility: 'hidden'
      }

  const renderMenu = (className: string): ReactElement | null => {
    if (!menuOpen || !canOpenModelControls) return null
    const menu = (
      <>
        <div
          ref={menuRef}
          role="menu"
          style={menuStyle}
          className={`ds-model-menu ${className}`}
        >
          {reasoningEnabled && !needsProviderSetup ? (
            <>
              <MenuSectionTitle>{t('composerReasoning')}</MenuSectionTitle>
              <div className="flex flex-col gap-1">
                {reasoningOptions.map((option) => (
                  <PickerRow
                    key={option.id}
                    compact
                    selected={currentReasoning === option.id}
                    title={t(option.labelKey)}
                    onClick={() => {
                      onComposerReasoningEffortChange?.(option.id)
                      setMenuOpen(false)
                    }}
                  />
                ))}
              </div>
              <MenuSeparator />
            </>
          ) : null}

          <div className="pr-0.5">
            {needsProviderSetup ? (
              <div className="px-2.5 py-2">
                <p className="text-[12.5px] leading-5 text-ds-muted">
                  {t('composerNoProviders')}
                </p>
                {onConfigureProviders ? (
                  <button
                    type="button"
                    onClick={() => {
                      setMenuOpen(false)
                      onConfigureProviders()
                    }}
                    className="mt-2 flex w-full items-center justify-center rounded-lg border border-ds-border bg-ds-surface-subtle px-3 py-2 text-[12.5px] font-semibold text-ds-ink transition hover:bg-ds-hover"
                  >
                    {t('composerConfigureProviders')}
                  </button>
                ) : null}
              </div>
            ) : (
              providerMenuGroups.map((group) => {
                const groupHasCurrentModel = group.modelIds.some((id) =>
                  modelIdsMatch(group, id, currentModel)
                )
                const selectedModel = groupHasCurrentModel
                  ? currentModel
                  : ''
                const subtitle = [
                  group.subtitleKey ? t(group.subtitleKey) : modelLabelFromCatalog(group.modelLabels, selectedModel),
                  group.endpointKind ? t(`providerConnection_${group.endpointKind}`) : ''
                ].filter(Boolean).join(' · ')
                return (
                  <ProviderRow
                    key={group.menuId}
                    refNode={(node) => {
                      if (node) providerRowRefs.current.set(group.menuId, node)
                      else providerRowRefs.current.delete(group.menuId)
                    }}
                    active={activeProviderId === group.menuId}
                    selected={selectedProviderId === group.providerId && groupHasCurrentModel}
                    title={group.label}
                    subtitle={subtitle}
                    onClick={() => setActiveProviderId(group.menuId)}
                    onMouseEnter={() => setActiveProviderId(group.menuId)}
                  />
                )
              })
            )}
          </div>
        </div>
        {activeProviderGroup ? (
          <div
            ref={submenuRef}
            role="menu"
            aria-label={activeProviderGroup.label}
            style={submenuStyle}
            className="ds-model-menu fixed z-[1001] overflow-y-auto rounded-xl border border-ds-border bg-ds-card p-1.5 text-[13px] text-ds-muted shadow-lg"
          >
            {activeProviderModelIds.length > 0 ? (
              activeProviderModelIds.map((id) => (
                <PickerRow
                  key={`${activeProviderGroup.menuId}:${id}`}
                  selectedIndicator="rail"
                  selected={composerModelMenuItemSelected({
                    groupProviderId: activeProviderGroup.providerId,
                    selectedProviderId,
                    currentModel,
                    modelId: id,
                    aliases: modelProfileForModel(activeProviderGroup, id)?.aliases
                  })}
                  title={modelLabelFromCatalog(activeProviderGroup.modelLabels, id)}
                  detail={id}
                  rightSlot={
                    <ModelCapabilityBadge
                      kind={composerModelCapabilityKind(activeProviderGroup, id)}
                      label={t(composerModelCapabilityLabelKey(activeProviderGroup, id))}
                    />
                  }
                  onClick={() => {
                    const nextReasoning = normalizeComposerReasoningMenuEffort(
                      composerReasoningEffort,
                      modelProfileForModel(activeProviderGroup, id)
                    )
                    onComposerModelChange(
                      id,
                      activeProviderGroup.providerId === UNGROUPED_MODEL_PROVIDER_ID
                        ? undefined
                        : activeProviderGroup.providerId
                    )
                    if (nextReasoning !== currentReasoning) {
                      onComposerReasoningEffortChange?.(nextReasoning)
                    }
                    setMenuOpen(false)
                  }}
                />
              ))
            ) : (
              <div className="px-2.5 py-2 text-[12.5px] font-medium text-ds-faint">
                {t('composerNoMatchingModels')}
              </div>
            )}
          </div>
        ) : null}
      </>
    )

    if (typeof document === 'undefined') return menu
    return createPortal(menu, document.body)
  }

  if (mode === 'combobox') {
    return (
      <div
        ref={(node) => {
          pickerRef.current = node
        }}
        className={`ds-composer-model-picker ds-no-drag relative flex h-9 items-center rounded-full transition ${comboboxWidthClass} ${
          canOpenModelControls ? 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink' : 'text-ds-faint'
        }`}
        title={controlsTitle}
      >
        <span className="sr-only">{t('composerModel')}</span>
        <button
          type="button"
          disabled={!canOpenModelControls}
          onClick={() => setMenuOpen((open) => !open)}
          title={controlsTitle}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          aria-label={`${t('composerModelControls')}: ${controlsTitle}`}
          className={`flex h-9 min-w-0 flex-1 items-center justify-end gap-1 rounded-full py-2 pl-3 pr-1 text-[13px] font-medium outline-none transition ${
            canOpenModelControls
              ? 'text-current focus-visible:ring-2 focus-visible:ring-accent/25'
              : 'cursor-not-allowed text-ds-faint'
          }`}
        >
          <span className="min-w-0 truncate text-right">
            {modelLabel}
          </span>
          {reasoningEnabled ? (
            <span className="shrink-0 text-[12px] font-semibold text-ds-faint">
              {currentReasoningLabel}
            </span>
          ) : null}
          <span className="mr-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-ds-faint">
            <ChevronDown className="h-3.5 w-3.5" strokeWidth={1.8} />
          </span>
        </button>
        {renderMenu('fixed z-[1000] overflow-x-hidden overflow-y-auto rounded-xl border border-ds-border bg-white p-1.5 text-[12.5px] shadow-[0_18px_50px_rgba(20,47,95,0.16)] dark:bg-ds-card')}
      </div>
    )
  }

  return (
    <div
      className={`ds-composer-model-picker ds-no-drag relative h-9 shrink-0 items-center rounded-full transition ${
        canOpenModelControls ? 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink' : 'text-ds-faint'
      } ${
        compact ? 'max-w-[220px]' : 'max-w-[260px]'
      }`}
      ref={(node) => {
        pickerRef.current = node
      }}
    >
      <button
        type="button"
        disabled={!canOpenModelControls}
        onClick={() => setMenuOpen((open) => !open)}
        className={`flex h-9 max-w-full items-center gap-1.5 rounded-full px-2.5 text-[13.5px] font-semibold transition disabled:cursor-not-allowed ${
          canOpenModelControls ? 'hover:bg-ds-hover' : ''
        }`}
        aria-expanded={menuOpen}
        aria-haspopup="menu"
        aria-label={`${t('composerModelControls')}: ${controlsTitle}`}
        title={controlsTitle}
      >
        <span className="min-w-0 truncate">{modelLabel}</span>
        {reasoningEnabled ? (
          <span className="shrink-0 border-l border-ds-border pl-1.5 text-[12px] font-normal text-ds-faint">
            {t(reasoningLabelKey(currentReasoning))}
          </span>
        ) : null}
        <ChevronDown className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={1.8} />
      </button>

      {menuOpen && canOpenModelControls ? (
        renderMenu('fixed z-[1000] overflow-x-hidden overflow-y-auto rounded-xl border border-ds-border bg-white p-1.5 text-[13px] text-ds-muted shadow-[0_22px_64px_rgba(20,47,95,0.18)] dark:bg-ds-card')
      ) : null}
    </div>
  )
}

export function buildComposerModelMenuGroups({
  composerModelGroups,
  modelOptions,
  ungroupedLabel
}: {
  composerModelGroups: readonly ModelProviderModelGroup[]
  modelOptions: readonly string[]
  ungroupedLabel: string
}): ComposerModelMenuGroup[] {
  const configuredModelKeys = new Set<string>()
  const groups = composerModelGroups
    .map((group) => {
      const seenInProvider = new Set<string>()
      const ids = group.modelIds
        .map((id) => id.trim())
        .filter((id) => {
          const key = normalizeModelCapabilityKey(id)
          if (!key || seenInProvider.has(key)) return false
          if (!composerMenuSupportsModel(group, id)) return false
          markModelSeen(seenInProvider, group, id)
          markModelSeen(configuredModelKeys, group, id)
          return true
        })
      return {
        ...group,
        menuId: providerMenuId(group.providerId),
        providerId: group.providerId.trim(),
        label: group.label.trim() || group.providerId,
        modelIds: ids,
        modelProfiles: group.modelProfiles
      }
    })
    .filter((group) => group.modelIds.length > 0)

  const ungrouped: string[] = []
  const seenUngrouped = new Set<string>()
  for (const rawId of modelOptions) {
    const id = rawId.trim()
    const key = normalizeModelCapabilityKey(id)
    if (!key || configuredModelKeys.has(key) || seenUngrouped.has(key) || !isComposerChatModelId(id)) continue
    seenUngrouped.add(key)
    ungrouped.push(id)
  }

  if (ungrouped.length > 0) {
    groups.push({
      menuId: UNGROUPED_MODEL_PROVIDER_ID,
      providerId: UNGROUPED_MODEL_PROVIDER_ID,
      label: ungroupedLabel,
      modelIds: ungrouped,
      modelProfiles: {}
    })
  }
  return splitComposerModelFamilyGroups(groups)
}

export function filterComposerModelIds(
  modelIds: readonly string[],
  query: string
): string[] {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return [...modelIds]
  return modelIds.filter((id) => id.toLowerCase().includes(normalizedQuery))
}

function shouldShowProviderSetupPrompt(groups: readonly ComposerModelMenuGroup[]): boolean {
  return !groups.some((group) => group.providerId !== UNGROUPED_MODEL_PROVIDER_ID)
}

export function normalizeComposerReasoningEffort(
  value: string | undefined,
  profile?: Pick<ModelProviderModelProfileV1, 'reasoning'>
): ComposerReasoningEffort {
  const normalized = normalizeComposerReasoningEffortValue(value)
  if (!profile?.reasoning) return normalized ?? 'max'
  const supported = profile.reasoning.supportedEfforts
  if (normalized && supported.includes(normalized)) return normalized
  if (normalized === 'low' && supported.includes('off') && !supported.includes('low')) {
    return 'off'
  }
  return profile.reasoning.defaultEffort
}

function normalizeComposerReasoningEffortValue(
  value: string | undefined
): ComposerReasoningEffort | undefined {
  const normalized = value?.trim().toLowerCase()
  return MODEL_REASONING_EFFORTS.includes(normalized as ComposerReasoningEffort)
    ? normalized as ComposerReasoningEffort
    : undefined
}

export function composerReasoningEffortRequestValue(
  value: unknown
): ComposerReasoningEffort | undefined {
  return projectModelReasoningEffortV1(value)
}

export function calculateFloatingMenuPlacement({
  anchorRect,
  menuHeight,
  viewportHeight,
  viewportWidth,
  coordinateScale = 1
}: {
  anchorRect: FloatingMenuAnchorRect
  menuHeight: number
  viewportHeight: number
  viewportWidth: number
  coordinateScale?: number
}): FloatingMenuPlacement {
  const scale = Number.isFinite(coordinateScale) && coordinateScale > 0 ? coordinateScale : 1
  const normalizedAnchorRect = {
    bottom: anchorRect.bottom / scale,
    right: anchorRect.right / scale,
    top: anchorRect.top / scale
  }
  const normalizedViewportHeight = viewportHeight / scale
  const normalizedViewportWidth = viewportWidth / scale
  const viewportMaxWidth = Math.max(
    FLOATING_MENU_MIN_WIDTH,
    normalizedViewportWidth - FLOATING_MENU_MARGIN * 2
  )
  const width = Math.min(FLOATING_MENU_WIDTH, viewportMaxWidth)
  const left = clamp(
    normalizedAnchorRect.right - width,
    FLOATING_MENU_MARGIN,
    normalizedViewportWidth - FLOATING_MENU_MARGIN - width
  )
  const contentHeight = Math.max(menuHeight, FLOATING_MENU_MIN_HEIGHT)
  const spaceAbove = Math.max(0, normalizedAnchorRect.top - FLOATING_MENU_MARGIN - FLOATING_MENU_GAP)
  const spaceBelow = Math.max(
    0,
    normalizedViewportHeight - normalizedAnchorRect.bottom - FLOATING_MENU_MARGIN - FLOATING_MENU_GAP
  )
  const targetHeight = Math.min(contentHeight, FLOATING_MENU_MAX_HEIGHT)
  const openAbove = spaceAbove >= targetHeight || spaceAbove >= spaceBelow
  const availableHeight = Math.max(openAbove ? spaceAbove : spaceBelow, FLOATING_MENU_MIN_HEIGHT)
  const maxHeight = Math.min(FLOATING_MENU_MAX_HEIGHT, availableHeight)
  const visibleHeight = Math.min(contentHeight, maxHeight)
  const preferredTop = openAbove
    ? normalizedAnchorRect.top - FLOATING_MENU_GAP - visibleHeight
    : normalizedAnchorRect.bottom + FLOATING_MENU_GAP
  const top = clamp(
    preferredTop,
    FLOATING_MENU_MARGIN,
    Math.max(FLOATING_MENU_MARGIN, normalizedViewportHeight - FLOATING_MENU_MARGIN - visibleHeight)
  )

  return { left, top, width, maxHeight }
}

export function calculateFloatingSubmenuPlacement({
  anchorRect,
  submenuHeight,
  viewportHeight,
  viewportWidth,
  coordinateScale = 1
}: {
  anchorRect: FloatingSubmenuAnchorRect
  submenuHeight: number
  viewportHeight: number
  viewportWidth: number
  coordinateScale?: number
}): FloatingSubmenuPlacement {
  const scale = Number.isFinite(coordinateScale) && coordinateScale > 0 ? coordinateScale : 1
  const normalizedAnchorRect = {
    bottom: anchorRect.bottom / scale,
    left: anchorRect.left / scale,
    right: anchorRect.right / scale,
    top: anchorRect.top / scale
  }
  const normalizedViewportHeight = viewportHeight / scale
  const normalizedViewportWidth = viewportWidth / scale
  const viewportMaxWidth = Math.max(
    FLOATING_MENU_MIN_WIDTH,
    normalizedViewportWidth - FLOATING_MENU_MARGIN * 2
  )
  const width = Math.min(FLOATING_SUBMENU_WIDTH, viewportMaxWidth)
  const spaceRight = normalizedViewportWidth - normalizedAnchorRect.right - FLOATING_MENU_MARGIN
  const spaceLeft = normalizedAnchorRect.left - FLOATING_MENU_MARGIN
  const openRight = spaceRight >= width + FLOATING_SUBMENU_GAP || spaceRight >= spaceLeft
  const preferredLeft = openRight
    ? normalizedAnchorRect.right + FLOATING_SUBMENU_GAP
    : normalizedAnchorRect.left - width - FLOATING_SUBMENU_GAP
  const left = clamp(
    preferredLeft,
    FLOATING_MENU_MARGIN,
    normalizedViewportWidth - FLOATING_MENU_MARGIN - width
  )
  const contentHeight = Math.max(submenuHeight, FLOATING_SUBMENU_MIN_HEIGHT)
  const maxHeight = Math.min(
    FLOATING_SUBMENU_MAX_HEIGHT,
    Math.max(FLOATING_SUBMENU_MIN_HEIGHT, normalizedViewportHeight - FLOATING_MENU_MARGIN * 2)
  )
  const visibleHeight = Math.min(contentHeight, maxHeight)
  const preferredTop = normalizedAnchorRect.top - 8
  const top = clamp(
    preferredTop,
    FLOATING_MENU_MARGIN,
    Math.max(FLOATING_MENU_MARGIN, normalizedViewportHeight - FLOATING_MENU_MARGIN - visibleHeight)
  )

  return { left, top, width, maxHeight }
}

function currentBodyZoom(): number {
  if (typeof window === 'undefined') return 1
  const zoom = window.getComputedStyle(document.body).zoom
  const parsed = Number.parseFloat(zoom)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1
}

function reasoningLabelKey(value: ComposerReasoningEffort): string {
  return REASONING_OPTIONS.find((option) => option.id === value)?.labelKey ?? 'composerReasoningMax'
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}

export function composerModelDisplayLabel(
  groups: readonly Pick<ComposerModelMenuGroup, 'providerId' | 'modelLabels'>[],
  model: string,
  providerId: string | null,
  autoLabel: string
): string {
  const trimmed = model.trim()
  if (!trimmed || trimmed.toLowerCase() === 'auto') return autoLabel
  const label = modelLabelFromCatalog(groups.find((group) => group.providerId === providerId)?.modelLabels, trimmed)
  return label === trimmed ? providerModelDisplayName(trimmed) : label
}

function estimatedModelSubmenuHeight(modelCount: number): number {
  return Math.max(1, modelCount) * 36 + 12
}

function normalizeModelCapabilityKey(modelId: string): string {
  return modelId.trim().toLowerCase()
}

function providerMenuId(providerId: string, familyId?: ComposerModelFamily['id']): string {
  const base = providerId.trim() || UNGROUPED_MODEL_PROVIDER_ID
  return familyId ? `${base}:${familyId}` : base
}

function splitComposerModelFamilyGroups(
  groups: readonly ComposerModelMenuGroup[]
): ComposerModelMenuGroup[] {
  const splitGroups: ComposerModelMenuGroup[] = []
  for (const group of groups) {
    const familyBuckets = new Map<ComposerModelFamily['id'], string[]>()
    const otherModelIds: string[] = []

    for (const modelId of group.modelIds) {
      const family = modelFamilyForComposerModel(modelId)
      if (!family) {
        otherModelIds.push(modelId)
        continue
      }
      familyBuckets.set(family.id, [...(familyBuckets.get(family.id) ?? []), modelId])
    }

    const shouldSplitFamilies =
      group.providerId === UNGROUPED_MODEL_PROVIDER_ID ||
      familyBuckets.size > 1
    if (!shouldSplitFamilies || familyBuckets.size === 0) {
      splitGroups.push(group)
      continue
    }

    for (const family of COMPOSER_MODEL_FAMILIES) {
      const modelIds = familyBuckets.get(family.id)
      if (!modelIds?.length) continue
      splitGroups.push({
        ...group,
        menuId: providerMenuId(group.providerId, family.id),
        label: family.label,
        subtitleKey: family.subtitleKey,
        modelIds
      })
    }

    if (otherModelIds.length > 0) {
      splitGroups.push({
        ...group,
        menuId: providerMenuId(group.providerId),
        modelIds: otherModelIds
      })
    }
  }
  return splitGroups
}

function modelFamilyForComposerModel(modelId: string): ComposerModelFamily | null {
  const normalized = normalizeModelCapabilityKey(modelId)
  if (normalized.startsWith('deepseek')) return DEEPSEEK_MODEL_FAMILY
  if (normalized.startsWith('mimo')) return MIMO_MODEL_FAMILY
  if (normalized.startsWith('qwen')) return QWEN_MODEL_FAMILY
  return null
}

function modelIdsMatch(
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'>,
  modelId: string,
  selectedModelId: string
): boolean {
  const left = normalizeModelCapabilityKey(modelId)
  const right = normalizeModelCapabilityKey(selectedModelId)
  if (!left || !right) return false
  if (left === right) return true
  return Boolean(modelProfileForModel(group, modelId)?.aliases?.some((alias) =>
    normalizeModelCapabilityKey(alias) === right
  ))
}

export function composerModelMenuItemSelected(input: {
  groupProviderId: string
  selectedProviderId: string | null
  currentModel: string
  modelId: string
  aliases?: readonly string[]
}): boolean {
  const current = normalizeModelCapabilityKey(input.currentModel)
  const selectedModel = normalizeModelCapabilityKey(input.modelId)
  const modelMatches = Boolean(current) && (
    current === selectedModel ||
    Boolean(input.aliases?.some((alias) => normalizeModelCapabilityKey(alias) === current))
  )
  return (
    Boolean(input.selectedProviderId) &&
    input.groupProviderId === input.selectedProviderId &&
    modelMatches
  )
}

function markModelSeen(
  seen: Set<string>,
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'>,
  modelId: string
): void {
  for (const id of [modelId, ...(modelProfileForModel(group, modelId)?.aliases ?? [])]) {
    const key = normalizeModelCapabilityKey(id)
    if (key) seen.add(key)
  }
}

function modelProfileForModel(
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'> | null | undefined,
  modelId: string
): ModelProviderModelProfileV1 | undefined {
  if (!group) return undefined
  const key = normalizeModelCapabilityKey(modelId)
  if (!key) return undefined
  const profiles = group.modelProfiles ?? {}
  const direct = Object.prototype.hasOwnProperty.call(profiles, key) ? profiles[key]
    : Object.prototype.hasOwnProperty.call(profiles, modelId.trim()) ? profiles[modelId.trim()] : undefined
  if (direct) return direct
  return Object.values(profiles).find((profile) =>
    profile.aliases?.some((alias) => normalizeModelCapabilityKey(alias) === key)
  )
}

function modelProfileForSelection(
  groups: readonly ComposerModelMenuGroup[],
  modelId: string,
  providerId?: string | null
): ModelProviderModelProfileV1 | undefined {
  const selectedGroup = providerId
    ? groups.find((group) => group.providerId === providerId)
    : null
  if (selectedGroup && selectedGroup.modelIds.some((id) => modelIdsMatch(selectedGroup, id, modelId))) {
    const profile = modelProfileForModel(selectedGroup, modelId)
    if (profile) return profile
  }
  for (const group of groups) {
    if (!group.modelIds.some((id) => modelIdsMatch(group, id, modelId))) continue
    const profile = modelProfileForModel(group, modelId)
    if (profile) return profile
  }
  for (const group of groups) {
    const profile = modelProfileForModel(group, modelId)
    if (profile) return profile
  }
  return undefined
}

function reasoningOptionsForModel(
  profile: Pick<ModelProviderModelProfileV1, 'reasoning'> | undefined
): Array<{ id: ComposerReasoningEffort; labelKey: string }> {
  const supported = profile?.reasoning?.supportedEfforts ?? LEGACY_REASONING_EFFORTS
  return supported
    .map((effort) => REASONING_OPTIONS.find((option) => option.id === effort))
    .filter((option): option is { id: ComposerReasoningEffort; labelKey: string } => Boolean(option))
}

export function composerReasoningMenuOptionsForModel(
  profile: Pick<ModelProviderModelProfileV1, 'reasoning'> | undefined
): Array<{ id: ComposerReasoningEffort; labelKey: string }> {
  return reasoningOptionsForModel(profile).filter((option) =>
    !HIDDEN_COMPOSER_REASONING_EFFORTS.has(option.id)
  )
}

export function normalizeComposerReasoningMenuEffort(
  value: string | undefined,
  profile?: Pick<ModelProviderModelProfileV1, 'reasoning'>
): ComposerReasoningEffort {
  const normalized = normalizeComposerReasoningEffort(value, profile)
  const visibleOptions = composerReasoningMenuOptionsForModel(profile)
  if (visibleOptions.some((option) => option.id === normalized)) return normalized
  const defaultEffort = profile?.reasoning?.defaultEffort
  if (defaultEffort && visibleOptions.some((option) => option.id === defaultEffort)) {
    return defaultEffort
  }
  return visibleOptions[visibleOptions.length - 1]?.id ?? normalized
}

export function composerMenuSupportsModel(
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'>,
  modelId: string
): boolean {
  if (!isComposerChatModelId(modelId)) return false
  return modelProfileSupportsTextChat(modelProfileForModel(group, modelId))
}

export function composerModelCapabilityKind(
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'>,
  modelId: string
): 'multimodal' | 'text' {
  if (modelSupportsImageInput(modelProfileForModel(group, modelId))) return 'multimodal'
  return composerModelIdLooksMultimodal(modelId) ? 'multimodal' : 'text'
}

function composerModelCapabilityLabelKey(
  group: Pick<ComposerModelMenuGroup, 'modelProfiles'>,
  modelId: string
): string {
  return composerModelCapabilityKind(group, modelId) === 'multimodal'
    ? 'composerModelMultimodal'
    : 'composerModelTextOnly'
}

export function composerModelIdLooksMultimodal(modelId: string): boolean {
  const normalized = normalizeModelCapabilityKey(modelId)
  if (normalized === 'mimo-v2.5' || normalized === 'mimo-v2-omni') return true
  if (/^qwen3\.7-(?:plus|max)(?:-|$)/.test(normalized)) return true
  if (/^qwen3\.6-(?:plus|flash)(?:-|$)/.test(normalized)) return true
  const tokens = normalized.split(/[-_./:]+/).filter(Boolean)
  if (tokens.includes('audio')) return false
  if (normalized.startsWith('gpt-4o')) return true
  return /^qwen(?:3)?-?vl/.test(normalized) || tokens.some((token) =>
    token === 'vl' ||
    token === 'vision' ||
    token === 'visual' ||
    token === 'multimodal' ||
    token === 'omni'
  )
}

function MenuSectionTitle({
  children
}: {
  children: string
}): ReactElement {
  return (
    <div className="flex h-7 items-center px-2 text-[12.5px] font-bold text-ds-faint">
      <span>{children}</span>
    </div>
  )
}

function MenuSeparator(): ReactElement {
  return <div className="my-1.5 h-px bg-ds-border-muted" />
}

function PickerRow({
  compact = false,
  selectedIndicator = 'check',
  selected,
  title,
  detail,
  rightSlot,
  onClick
}: {
  compact?: boolean
  selectedIndicator?: 'check' | 'rail'
  selected: boolean
  title: string
  detail?: string
  rightSlot?: ReactElement | null
  onClick: () => void
}): ReactElement {
  const rowDensityClass = compact ? 'min-h-8 px-2.5 py-1' : 'min-h-9 px-2.5 py-1.5'
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={selected}
      title={detail && detail !== title ? `${title} · ${detail}` : title}
      aria-label={detail && detail !== title ? `${title} · ${detail}` : title}
      onMouseDown={(event) => event.preventDefault()}
      onClick={onClick}
      className={`relative flex w-full items-center gap-2 rounded-lg text-left transition ${rowDensityClass} ${
        selected
          ? 'bg-ds-hover text-ds-ink'
          : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
      }`}
    >
      {selected && selectedIndicator === 'rail' ? (
        <span
          aria-hidden="true"
          className="absolute left-1 top-2 bottom-2 w-0.5 rounded-full bg-accent"
        />
      ) : null}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-semibold">{title}</span>
      </span>
      {rightSlot}
      {selected && selectedIndicator === 'check' ? (
        <Check className="h-4 w-4 shrink-0 text-accent" strokeWidth={2} />
      ) : null}
    </button>
  )
}

function ModelCapabilityBadge({
  kind,
  label
}: {
  kind: 'multimodal' | 'text'
  label: string
}): ReactElement {
  const tone = kind === 'multimodal'
    ? 'border-accent/25 bg-accent/5 text-accent'
    : 'border-ds-border bg-ds-surface-subtle text-ds-muted'
  return (
    <span
      className={`inline-flex h-5 shrink-0 items-center rounded-full border px-1.5 text-[10.5px] font-semibold leading-none ${tone}`}
      title={label}
    >
      <span>{label}</span>
    </span>
  )
}

function ProviderRow({
  active,
  selected,
  title,
  subtitle,
  refNode,
  onClick,
  onMouseEnter
}: {
  active: boolean
  selected: boolean
  title: string
  subtitle?: string
  refNode: (node: HTMLButtonElement | null) => void
  onClick: () => void
  onMouseEnter: () => void
}): ReactElement {
  return (
    <button
      ref={refNode}
      type="button"
      role="menuitem"
      aria-haspopup="menu"
      aria-expanded={active}
      title={subtitle ? `${title} / ${subtitle}` : title}
      onMouseDown={(event) => event.preventDefault()}
      onMouseEnter={onMouseEnter}
      onFocus={onMouseEnter}
      onClick={onClick}
      className={`flex min-h-10 w-full items-center gap-1.5 rounded-lg px-2.5 py-1 text-left transition ${
        active
          ? 'bg-ds-hover text-ds-ink'
          : selected
            ? 'text-ds-ink hover:bg-ds-hover'
            : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
      }`}
    >
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-semibold">{title}</span>
        {subtitle ? (
          <span className="block truncate text-[11.5px] font-medium text-ds-faint">{subtitle}</span>
        ) : null}
      </span>
      <ChevronRight className="h-4 w-4 shrink-0 text-ds-faint" strokeWidth={1.8} />
    </button>
  )
}
