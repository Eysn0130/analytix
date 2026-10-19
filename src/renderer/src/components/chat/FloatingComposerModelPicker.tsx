/* Presentation adapted from DeepSeek Harness 687f282e8625c6b8d11ff58211a90524be2e4317.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md.
 * Analytix retains its own catalog, settings/next-turn owners and bridge. */
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactElement,
  type KeyboardEvent as ReactKeyboardEvent
} from 'react'
import { createPortal } from 'react-dom'
import {
  Check,
  ChevronDown,
  ChevronRight,
  Loader2,
  X
} from '../../design/AnalytixUiIcons'
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
import { hasChatTokenScope } from './presentation/Tooltip'
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
  onReloadComposerModels?: () => Promise<void>
  open?: boolean
  onOpenChange?: (open: boolean) => void
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

type FloatingMenuAnchorRect = Pick<DOMRect, 'bottom' | 'right' | 'top'>

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
const FLOATING_MENU_WIDTH = 240
const FLOATING_MENU_MIN_HEIGHT = 112
const FLOATING_MENU_MAX_HEIGHT = 360
const EMPTY_MODEL_GROUPS: ModelProviderModelGroup[] = []
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

type ModelMenuPane = 'root' | 'model' | 'effort'

export function FloatingComposerModelPicker({
  compact,
  mode,
  composerModel,
  composerProviderId = '',
  composerPickList,
  composerModelGroups = EMPTY_MODEL_GROUPS,
  canChangeModel,
  stretch = false,
  composerReasoningEffort = 'max',
  onComposerModelChange,
  onComposerReasoningEffortChange,
  onConfigureProviders,
  onReloadComposerModels,
  open: controlledOpen,
  onOpenChange
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const id = useId()
  const pickerRef = useRef<HTMLDivElement | null>(null)
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const searchRef = useRef<HTMLInputElement | null>(null)
  const onOpenChangeRef = useRef(onOpenChange)
  const reloadRef = useRef(onReloadComposerModels)
  onOpenChangeRef.current = onOpenChange
  reloadRef.current = onReloadComposerModels
  const [internalOpen, setInternalOpen] = useState(false)
  const [pane, setPane] = useState<ModelMenuPane>('root')
  const [query, setQuery] = useState('')
  const [highlightedIndex, setHighlightedIndex] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [menuPlacement, setMenuPlacement] = useState<FloatingMenuPlacement | null>(null)
  const loadSequence = useRef(0)
  const wasOpenRef = useRef(false)
  const menuOpen = controlledOpen ?? internalOpen
  const setMenuOpen = useCallback((next: boolean): void => {
    setInternalOpen(next)
    onOpenChangeRef.current?.(next)
  }, [])
  const closeMenu = useCallback((restoreFocus = false): void => {
    setMenuOpen(false)
    if (restoreFocus && triggerRef.current?.isConnected && !triggerRef.current.disabled) {
      triggerRef.current.focus({ preventScroll: true })
    }
  }, [setMenuOpen])
  const reloadModels = useCallback((): void => {
    const reload = reloadRef.current
    if (!reload) return
    const sequence = ++loadSequence.current
    setLoading(true)
    setLoadFailed(false)
    void Promise.resolve().then(reload).catch(() => {
      if (loadSequence.current === sequence) setLoadFailed(true)
    }).finally(() => {
      if (loadSequence.current === sequence) setLoading(false)
    })
  }, [])
  useEffect(() => () => {
    loadSequence.current += 1
    wasOpenRef.current = false
  }, [])

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
  const providerMenuGroups = useMemo<ComposerModelMenuGroup[]>(() => buildComposerModelMenuGroups({
    composerModelGroups,
    modelOptions,
    ungroupedLabel: t('composerOtherModels')
  }), [composerModelGroups, modelOptions, t])
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
  const reasoningEnabled = !needsProviderSetup && !providerUnavailable && Boolean(onComposerReasoningEffortChange) && reasoningOptions.length > 0
  const currentReasoning = normalizeComposerReasoningMenuEffort(composerReasoningEffort, currentModelProfile)
  const currentReasoningLabel = t(reasoningLabelKey(currentReasoning))
  const canOpenModelControls = canChangeModel || (needsProviderSetup && Boolean(onConfigureProviders || onReloadComposerModels))
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
  const showSearch = !needsProviderSetup && providerMenuGroups.reduce((count, group) => count + group.modelIds.length, 0) > 4
  const filteredGroups = useMemo(() => needsProviderSetup ? [] : providerMenuGroups.map((group) => ({
    ...group,
    modelIds: filterComposerModelIdsByLabel(group, showSearch ? query : '')
  })).filter((group) => group.modelIds.length > 0), [needsProviderSetup, providerMenuGroups, query, showSearch])
  const visibleModels = filteredGroups.flatMap((group) => group.modelIds.map((modelId) => ({ group, modelId })))
  const selectedVisibleIndex = visibleModels.findIndex(({ group, modelId }) => composerModelMenuItemSelected({
    groupProviderId: group.providerId, selectedProviderId, currentModel, modelId,
    aliases: modelProfileForModel(group, modelId)?.aliases
  }))
  const activeModelIndex = Math.min(highlightedIndex ?? Math.max(0, selectedVisibleIndex), visibleModels.length - 1)

  useEffect(() => {
    if (showSearch) return
    setQuery('')
    setHighlightedIndex(null)
  }, [showSearch])

  useEffect(() => {
    if (!reasoningEnabled) return
    const rawReasoning = normalizeComposerReasoningEffortValue(composerReasoningEffort)
    if (rawReasoning !== currentReasoning) onComposerReasoningEffortChange?.(currentReasoning)
  }, [composerReasoningEffort, currentReasoning, onComposerReasoningEffortChange, reasoningEnabled])

  useEffect(() => {
    if (menuOpen && !canOpenModelControls) closeMenu()
  }, [menuOpen, canOpenModelControls, closeMenu])

  useEffect(() => {
    if (wasOpenRef.current === menuOpen) return
    wasOpenRef.current = menuOpen
    if (!menuOpen) {
      loadSequence.current += 1
      setLoading(false)
      return
    }
    setQuery('')
    setHighlightedIndex(null)
    setPane(needsProviderSetup || providerUnavailable ? 'model' : 'root')
    reloadModels()
  }, [menuOpen, needsProviderSetup, providerUnavailable, reloadModels])

  useEffect(() => {
    if (!menuOpen) return
    const closeOutside = (event: Event): void => {
      const target = event.target
      if (target instanceof Node && (pickerRef.current?.contains(target) || menuRef.current?.contains(target))) return
      closeMenu()
    }
    window.addEventListener('pointerdown', closeOutside)
    window.addEventListener('focusin', closeOutside)
    return () => {
      window.removeEventListener('pointerdown', closeOutside)
      window.removeEventListener('focusin', closeOutside)
    }
  }, [menuOpen, closeMenu])

  useLayoutEffect(() => {
    if (!menuOpen) return
    const place = (): void => {
      const rect = triggerRef.current?.getBoundingClientRect()
      if (!rect) return
      setMenuPlacement(calculateFloatingMenuPlacement({
        anchorRect: rect,
        menuHeight: menuRef.current?.offsetHeight ?? 0,
        viewportHeight: window.innerHeight,
        viewportWidth: window.innerWidth,
        coordinateScale: currentBodyZoom()
      }))
    }
    place()
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(place) : null
    if (menuRef.current) observer?.observe(menuRef.current)
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      observer?.disconnect()
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [menuOpen, pane, query, loading, filteredGroups])

  useLayoutEffect(() => {
    if (!menuOpen || pane === 'root') return
    if (pane === 'model' && showSearch) {
      searchRef.current?.focus({ preventScroll: true })
      return
    }
    const selected = menuRef.current?.querySelector<HTMLButtonElement>("[role='menuitemradio'][aria-checked='true']:not(:disabled)")
    const first = menuRef.current?.querySelector<HTMLButtonElement>("[role='menuitemradio']:not(:disabled)")
    ;(selected ?? first)?.focus({ preventScroll: true })
  }, [menuOpen, pane, showSearch, loading])

  useLayoutEffect(() => {
    if (pane !== 'model' || !menuOpen || activeModelIndex < 0) return
    menuRef.current?.querySelector<HTMLElement>(`[data-model-index='${activeModelIndex}']`)?.scrollIntoView?.({ block: 'nearest' })
  }, [pane, menuOpen, activeModelIndex])

  const drill = (next: ModelMenuPane): void => {
    setQuery('')
    setHighlightedIndex(null)
    setPane(next)
  }
  const back = (): void => {
    const from = pane
    setPane('root')
    queueMicrotask(() => menuRef.current?.querySelector<HTMLButtonElement>(`[data-model-pane='${from}']`)?.focus({ preventScroll: true }))
  }
  const chooseModel = (group: ComposerModelMenuGroup, modelId: string): void => {
    if (!canChangeModel || loading) return
    if (composerModelMenuItemSelected({ groupProviderId: group.providerId, selectedProviderId, currentModel, modelId, aliases: modelProfileForModel(group, modelId)?.aliases })) {
      closeMenu(true)
      return
    }
    const nextReasoning = normalizeComposerReasoningMenuEffort(composerReasoningEffort, modelProfileForModel(group, modelId))
    onComposerModelChange(modelId, group.providerId === UNGROUPED_MODEL_PROVIDER_ID ? undefined : group.providerId)
    if (nextReasoning !== currentReasoning) onComposerReasoningEffortChange?.(nextReasoning)
    closeMenu(true)
  }
  const handleKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>): void => {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return
    if (!menuOpen) {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault()
        setMenuOpen(true)
      }
      return
    }
    if (event.key === 'Escape' || (event.key === 'Tab' && event.shiftKey)) {
      event.preventDefault()
      event.stopPropagation()
      if (pane !== 'root' && !needsProviderSetup && !providerUnavailable) back()
      else closeMenu(true)
      return
    }
    if (pane === 'model' && showSearch && event.target === searchRef.current) {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault()
        if (visibleModels.length) setHighlightedIndex((activeModelIndex + (event.key === 'ArrowDown' ? 1 : -1) + visibleModels.length) % visibleModels.length)
        return
      }
      if (event.key === 'Enter' || (event.key === 'Tab' && visibleModels.length > 0)) {
        event.preventDefault()
        const choice = visibleModels[activeModelIndex]
        if (choice) chooseModel(choice.group, choice.modelId)
        return
      }
    }
    const rows = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>("[role^='menuitem']:not(:disabled)") ?? [])
    if (event.key === 'Tab') {
      const focused = document.activeElement
      if (focused instanceof HTMLButtonElement && rows.includes(focused)) {
        event.preventDefault()
        focused.click()
      } else if (focused === triggerRef.current) {
        event.preventDefault()
        rows[0]?.focus({ preventScroll: true })
      }
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || rows.length === 0) return
    event.preventDefault()
    const current = rows.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? rows.length - 1
      : current < 0 ? (event.key === 'ArrowUp' ? rows.length - 1 : 0)
      : (current + (event.key === 'ArrowUp' ? -1 : 1) + rows.length) % rows.length
    rows[next]?.focus({ preventScroll: true })
  }
  const menuStyle: CSSProperties = menuPlacement ? menuPlacement : { visibility: 'hidden', left: 0, top: 0, width: FLOATING_MENU_WIDTH }
  let modelIndex = 0
  const menu = menuOpen && canOpenModelControls ? (
    <div ref={menuRef} id={`${id}-menu`} role={pane === 'model' ? 'group' : 'menu'} aria-label={t('composerModelControls')}
      aria-busy={loading} data-analytix-chat-token-scope={pickerRef.current && hasChatTokenScope(pickerRef.current) || undefined} style={menuStyle} className="ds-model-menu ds-composer-picker-menu" data-model-picker-pane={pane}>
      {pane === 'root' ? <>
        <button type="button" role="menuitem" data-model-pane="model" className="ds-model-cell" onClick={() => drill('model')}>
          <span>{t('composerModel')}</span><span className="ds-model-cell-value">{modelLabel}</span><ChevronRight aria-hidden="true" />
        </button>
        {reasoningEnabled ? <button type="button" role="menuitem" data-model-pane="effort" className="ds-model-cell" onClick={() => drill('effort')}>
          <span>{t('composerReasoning')}</span><span className="ds-model-cell-value">{currentReasoningLabel}</span><ChevronRight aria-hidden="true" />
        </button> : null}
      </> : null}
      {pane === 'model' ? <>
        {showSearch ? <div className="ds-model-search-row">
          <input ref={searchRef} type="text" role="searchbox" className="ds-model-search" aria-label={t('composerModelSearchPlaceholder')}
            aria-controls={`${id}-models`} aria-activedescendant={activeModelIndex < 0 ? undefined : `${id}-model-${activeModelIndex}`}
            placeholder={t('composerModelSearchPlaceholder')} value={query} readOnly={loading}
            onChange={(event) => { setQuery(event.target.value); setHighlightedIndex(0) }} />
          {query ? <button type="button" className="ds-model-search-clear" aria-label={t('clear')} disabled={loading}
            onClick={() => { setQuery(''); setHighlightedIndex(null); searchRef.current?.focus() }}><X aria-hidden="true" /></button> : null}
        </div> : null}
        {loading ? <div className="ds-model-status" role="status">{t('loading')}</div> : null}
        {loadFailed ? <div className="ds-model-load-error" role="status"><span>{t('composerProviderUnavailable')}</span>
          <button type="button" onClick={reloadModels}>{t('retryConnection')}</button></div> : null}
        <div id={`${id}-models`} role="menu" aria-label={t('composerModel')} className="ds-model-groups">
          {!needsProviderSetup ? filteredGroups.map((group) => <div key={group.menuId} role="group" aria-label={group.label}>
            <div className="ds-model-group-label" title={[group.label, group.endpointKind ? t(`providerConnection_${group.endpointKind}`) : ''].filter(Boolean).join(' · ')}>{group.label}</div>
            {group.modelIds.map((modelId) => {
              const index = modelIndex++
              const selected = composerModelMenuItemSelected({ groupProviderId: group.providerId, selectedProviderId, currentModel, modelId,
                aliases: modelProfileForModel(group, modelId)?.aliases })
              const label = modelLabelFromCatalog(group.modelLabels, modelId)
              return <button key={`${group.menuId}:${modelId}`} id={`${id}-model-${index}`} type="button" role="menuitemradio" aria-checked={selected}
                tabIndex={showSearch ? -1 : 0} data-model-index={index} data-highlighted={showSearch && index === activeModelIndex ? '' : undefined}
                disabled={loading || !canChangeModel} className="ds-model-option"
                aria-label={label === modelId ? modelId : `${label} · ${modelId}`} title={`${label} · ${modelId} · ${t(composerModelCapabilityLabelKey(group, modelId))}`}
                onFocus={() => setHighlightedIndex(index)} onMouseMove={() => { if (showSearch && !loading) setHighlightedIndex(index) }}
                onMouseDown={(event) => event.preventDefault()} onClick={() => chooseModel(group, modelId)}>
                <span className="ds-model-option-label">{label}</span><span className="ds-model-check" aria-hidden="true">{selected ? <Check /> : null}</span>
              </button>
            })}
          </div>) : null}
          {!loading && (needsProviderSetup || filteredGroups.length === 0) ? <div className="ds-model-status" role="status">
            {t(needsProviderSetup ? 'composerNoProviders' : 'composerNoMatchingModels')}
          </div> : null}
        </div>
        {!loading && needsProviderSetup && onReloadComposerModels && !loadFailed ? <button type="button" className="ds-model-reload" onClick={reloadModels}>{t('retryConnection')}</button> : null}
        {needsProviderSetup && onConfigureProviders ? <button type="button" className="ds-model-reload" onClick={() => { closeMenu(); onConfigureProviders() }}>{t('composerConfigureProviders')}</button> : null}
      </> : null}
      {pane === 'effort' ? <div role="group" aria-label={t('composerReasoning')} className="ds-model-groups">
        {reasoningOptions.map((option) => <button key={option.id} type="button" role="menuitemradio" aria-checked={currentReasoning === option.id}
          disabled={loading || !canChangeModel} className="ds-model-option" onMouseDown={(event) => event.preventDefault()}
          onClick={() => { onComposerReasoningEffortChange?.(option.id); closeMenu(true) }}>
          <span className="ds-model-option-label">{t(option.labelKey)}</span><span className="ds-model-check" aria-hidden="true">{currentReasoning === option.id ? <Check /> : null}</span>
        </button>)}
      </div> : null}
    </div>
  ) : null

  return <div ref={pickerRef} onKeyDown={handleKeyDown}
    className={`ds-composer-model-picker ds-no-drag ${stretch ? 'ds-model-picker-stretch' : ''}`} data-model-picker-mode={mode} data-compact={compact || undefined}>
    <button ref={triggerRef} type="button" disabled={!canOpenModelControls} className="ds-composer-picker-trigger ds-model-trigger"
      title={controlsTitle} aria-label={`${t('composerModelControls')}: ${controlsTitle}`} aria-haspopup="menu" aria-expanded={menuOpen && canOpenModelControls}
      aria-controls={menuOpen ? `${id}-menu` : undefined} aria-busy={loading} onClick={() => setMenuOpen(!menuOpen)}>
      <ComposerModelIcon className="ds-model-trigger-icon" />
      <span className="ds-model-trigger-label">{modelLabel}</span>
      {reasoningEnabled ? <span className="ds-model-trigger-effort">{currentReasoningLabel}</span> : null}
      {loading ? <Loader2 className="ds-picker-spinner" aria-hidden="true" /> : <ChevronDown className={`ds-picker-chevron ${menuOpen ? 'ds-picker-chevron-open' : ''}`} aria-hidden="true" />}
    </button>
    {menu && typeof document !== 'undefined' ? createPortal(menu, document.body) : null}
  </div>
}

function filterComposerModelIdsByLabel(group: ComposerModelMenuGroup, query: string): string[] {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return group.modelIds
  const rank = (label: string): number | undefined => {
    const text = label.toLowerCase()
    if (text.startsWith(normalized)) return 0
    const contiguous = text.indexOf(normalized)
    if (contiguous >= 0) return 1 + contiguous
    let cursor = 0
    let gaps = 0
    for (const character of normalized) {
      const found = text.indexOf(character, cursor)
      if (found < 0) return undefined
      gaps += found - cursor
      cursor = found + 1
    }
    return text.length + gaps
  }
  return group.modelIds.map((id, index) => {
    const scores = [id, modelLabelFromCatalog(group.modelLabels, id)].map(rank).filter((score): score is number => score !== undefined)
    return { id, index, score: scores.length ? Math.min(...scores) : undefined }
  }).filter((match): match is { id: string; index: number; score: number } => match.score !== undefined)
    .sort((left, right) => left.score - right.score || left.index - right.index).map(({ id }) => id)
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
    0,
    normalizedViewportWidth - FLOATING_MENU_MARGIN * 2
  )
  const width = Math.min(FLOATING_MENU_WIDTH, viewportMaxWidth)
  const left = clamp(
    normalizedAnchorRect.right - width,
    FLOATING_MENU_MARGIN,
    Math.max(FLOATING_MENU_MARGIN, normalizedViewportWidth - FLOATING_MENU_MARGIN - width)
  )
  const contentHeight = Math.max(menuHeight, FLOATING_MENU_MIN_HEIGHT)
  const spaceAbove = Math.max(0, normalizedAnchorRect.top - FLOATING_MENU_MARGIN - FLOATING_MENU_GAP)
  const spaceBelow = Math.max(
    0,
    normalizedViewportHeight - normalizedAnchorRect.bottom - FLOATING_MENU_MARGIN - FLOATING_MENU_GAP
  )
  const targetHeight = Math.min(contentHeight, FLOATING_MENU_MAX_HEIGHT)
  const openAbove = spaceAbove >= targetHeight || spaceAbove >= spaceBelow
  const availableHeight = Math.max(0, openAbove ? spaceAbove : spaceBelow)
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

// Selected IconDataOutlineRegular artwork only; no upstream icon registry or package is imported.
function ComposerModelIcon({ className }: { className?: string }): ReactElement {
  return (
  <svg width={16} height={16} className={className} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
    <path d="M7.8667 0.349609C8.96906 0.349634 10.0601 0.481272 11.0317 0.735352C11.9973 0.987845 12.8453 1.362 13.4644 1.84766C14.0744 2.32629 14.507 2.95539 14.5161 3.69336H14.5171V8.53516C14.0843 8.32076 13.6108 8.17679 13.1108 8.11816C13.1831 7.96848 13.2162 7.82856 13.2163 7.70312V5.76758C12.6269 6.16618 11.8739 6.47995 11.0317 6.7002C10.0602 6.95423 8.96896 7.08494 7.8667 7.08496C6.76461 7.08493 5.67411 6.95415 4.70264 6.7002C3.85994 6.48006 3.10694 6.1662 2.51709 5.76758V7.70312L2.521 7.78418C2.56374 8.19554 2.93361 8.74414 3.91357 9.23145C4.9281 9.73585 6.35004 10.0371 7.8667 10.0371C8.26373 10.0371 8.6543 10.0141 9.03271 9.97461C8.75596 10.3799 8.54664 10.8349 8.42041 11.3232C8.23666 11.3313 8.0518 11.3369 7.8667 11.3369C6.20108 11.3369 4.57025 11.01 3.33447 10.3955C3.04163 10.2499 2.76658 10.0836 2.51709 9.90039V11.6738C2.51728 12.1379 2.88589 12.7556 3.92236 13.292C4.93457 13.8157 6.35342 14.1289 7.8667 14.1289C8.12318 14.1289 8.37694 14.1161 8.62646 14.0986C8.82021 14.5535 9.08999 14.9682 9.41943 15.3271C8.91285 15.3934 8.39149 15.4287 7.8667 15.4287C6.19761 15.4287 4.56379 15.0869 3.32568 14.4463C2.11244 13.8185 1.21649 12.8562 1.21631 11.6738V3.76367C1.21595 3.74853 1.21438 3.733 1.21436 3.71777C1.21436 2.96917 1.65103 2.33053 2.26807 1.84668C2.88747 1.36112 3.73675 0.987685 4.70264 0.735352C5.67413 0.481376 6.76457 0.349636 7.8667 0.349609ZM7.8667 1.65039C6.86269 1.65042 5.88326 1.77028 5.03076 1.99316C4.17183 2.2176 3.50421 2.52956 3.06982 2.87012C2.65043 3.19909 2.52622 3.48898 2.51709 3.69336V3.74414C2.52719 3.94845 2.65185 4.23772 3.06982 4.56543C3.50425 4.90601 4.17172 5.21795 5.03076 5.44238C5.88326 5.66527 6.8627 5.78513 7.8667 5.78516C8.8707 5.78513 9.85015 5.66525 10.7026 5.44238C11.5611 5.21787 12.2286 4.9049 12.6626 4.56445C13.0982 4.22252 13.2163 3.9231 13.2163 3.71777L13.2104 3.63574C13.1818 3.43623 13.044 3.16941 12.6626 2.87012C12.2286 2.52957 11.5614 2.21773 10.7026 1.99316C9.85009 1.77025 8.8708 1.65041 7.8667 1.65039Z" fill="currentColor" />
    <path d="M12.8936 10.0361L13.2061 10.5566C13.2296 10.5959 13.2651 10.6562 13.3027 10.707C13.3469 10.7666 13.4148 10.8431 13.5195 10.9023C13.6244 10.9617 13.725 10.9801 13.7988 10.9873C13.8619 10.9934 13.9318 10.9932 13.9775 10.9932H14.6162L14.8896 11.4502L14.5947 11.9443C14.5698 11.9859 14.5312 12.0483 14.5029 12.1084C14.4781 12.1611 14.4514 12.2312 14.4395 12.3164L14.4326 12.4072L14.4395 12.4971C14.4514 12.5825 14.4781 12.6532 14.5029 12.7061C14.5312 12.7661 14.5689 12.8287 14.5938 12.8701L14.8896 13.3633L14.6162 13.8213H13.9775C13.9318 13.8213 13.8619 13.821 13.7988 13.8271C13.7433 13.8326 13.6728 13.8442 13.5967 13.875L13.5195 13.9121C13.4148 13.9714 13.3469 14.0478 13.3027 14.1074C13.265 14.1583 13.2296 14.2186 13.2061 14.2578L12.8936 14.7783H12.3115L11.999 14.2578C11.9755 14.2186 11.9401 14.1583 11.9023 14.1074C11.8693 14.0628 11.823 14.0083 11.7578 13.959L11.6855 13.9121L11.6074 13.875C11.5316 13.8445 11.4615 13.8325 11.4062 13.8271C11.3432 13.821 11.2733 13.8213 11.2275 13.8213H10.5889L10.3135 13.3633L10.6104 12.8701C10.6352 12.8287 10.6739 12.7661 10.7021 12.7061C10.7352 12.6357 10.7724 12.534 10.7725 12.4072C10.7724 12.2804 10.7352 12.1788 10.7021 12.1084C10.6739 12.0483 10.6353 11.9859 10.6104 11.9443L10.3135 11.4502L10.5889 10.9932H11.2275C11.2733 10.9932 11.3432 10.9934 11.4062 10.9873C11.4801 10.9801 11.5808 10.9616 11.6855 10.9023C11.7903 10.843 11.8582 10.7666 11.9023 10.707C11.94 10.6562 11.9755 10.5959 11.999 10.5566L12.3115 10.0361H12.8936Z" stroke="currentColor" strokeMiterlimit="10" />
  </svg>
  )
}
