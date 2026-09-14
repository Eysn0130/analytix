import { describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import type { AppSettingsV1 } from '@shared/app-settings'
import {
  FloatingComposer,
  buildComposerThreadUsageDisplay,
  buildComposerTurnChipText,
  buildResearchPrompt,
  formatGoalElapsedSeconds,
  handleComposerImagePaste,
  imageFilesFromTransfer,
  imageTransferHasImages,
  parseCompactCommand,
  parseGoalCommand,
  parseNewCommand,
  parseResearchCommand,
  parseReviewCommand,
  shouldShowGoalFloater
} from './FloatingComposer'
import {
  FloatingComposerModelPicker,
  buildComposerModelMenuGroups,
  calculateFloatingMenuPlacement,
  calculateFloatingSubmenuPlacement,
  composerModelCapabilityKind,
  composerModelIdLooksMultimodal,
  composerModelMenuItemSelected,
  composerMenuSupportsModel,
  composerReasoningEffortRequestValue,
  composerReasoningMenuOptionsForModel,
  filterComposerModelIds,
  normalizeComposerReasoningEffort,
  normalizeComposerReasoningMenuEffort
} from './FloatingComposerModelPicker'
import { FloatingComposerExecutionPicker } from './FloatingComposerExecutionPicker'
import { speechToTextPreferenceEnabled } from './use-voice-dictation'
import floatingComposerSource from './FloatingComposer.tsx?raw'
import { getGoalPanelDraftObjective } from './floating-composer-commands'
import { useChatStore } from '../../store/chat-store'
import {
  buildComposerFileContextPrompt,
  composerFileReferenceFromPath,
  filterWorkspaceFileMentionSuggestions,
  formatComposerFileMentionToken,
  getFileMentionAtCursor,
  isFileWithinDirectory,
  removeComposerFileMentionToken,
  replaceFileMentionInInput,
  type ComposerFileReference
} from '../../lib/composer-file-references'
import { filesUnderDirectory } from '../../lib/workspace-file-index'
import {
  buildComposerPromptProjection,
  composerRawOffsetToPmPos,
  parseComposerPromptDoc,
  pmPosToComposerRawOffset,
  serializeComposerPromptDoc
} from '../../lib/composer-prompt-document'

const DEEPSEEK_PROVIDER_GROUP = {
  providerId: 'deepseek',
  label: 'DeepSeek',
  modelIds: ['deepseek-v4-pro', 'deepseek-v4-flash']
}

function composerEditorHostHtml(html: string): string {
  return html.match(/<div[^>]*data-composer-prompt-editor="true"[^>]*>/)?.[0] ?? ''
}

describe('FloatingComposer slash commands', () => {
  it('parses compact command aliases', () => {
    expect(parseCompactCommand('/compact')).toEqual({})
    expect(parseCompactCommand('/compress')).toEqual({})
    expect(parseCompactCommand('/summarize')).toEqual({})
    expect(parseCompactCommand('/压缩')).toEqual({})
    expect(parseCompactCommand('/压缩会话')).toEqual({})
    expect(parseCompactCommand('/总结')).toEqual({})
  })

  it('parses compact reasons and ignores adjacent command names', () => {
    expect(parseCompactCommand('/compact preparing for a long continuation')).toEqual({
      reason: 'preparing for a long continuation'
    })
    expect(parseCompactCommand('/压缩会话 继续实现前整理上下文')).toEqual({
      reason: '继续实现前整理上下文'
    })
    expect(parseCompactCommand('/compactness')).toBeNull()
    expect(parseCompactCommand('please /compact')).toBeNull()
  })

  it('parses goal command controls and objectives', () => {
    expect(parseGoalCommand('/goal')).toEqual({ action: 'menu' })
    expect(parseGoalCommand('/goal pause')).toEqual({ action: 'pause' })
    expect(parseGoalCommand('/goal resume')).toEqual({ action: 'resume' })
    expect(parseGoalCommand('/goal clear')).toEqual({ action: 'clear' })
    expect(parseGoalCommand('/goal ship the feature')).toEqual({
      action: 'set',
      objective: 'ship the feature'
    })
    expect(parseGoalCommand('/goal --research cache economics')).toEqual({
      action: 'research',
      objective: 'cache economics'
    })
    expect(parseGoalCommand('/goal --research')).toEqual({ action: 'menu' })
    expect(parseGoalCommand('/goalkeeper')).toBe(false)
  })

  it('parses new session command aliases', () => {
    expect(parseNewCommand('/new')).toBe(true)
    expect(parseNewCommand('/new-thread')).toBe(true)
    expect(parseNewCommand('/新建会话')).toBe(true)
    expect(parseNewCommand('/new current task')).toBe(false)
    expect(parseNewCommand('/new-topic')).toBe(false)
  })

  it('parses review command targets', () => {
    expect(parseReviewCommand('/review')).toEqual({ kind: 'uncommittedChanges' })
    expect(parseReviewCommand('/review base main')).toEqual({ kind: 'baseBranch', branch: 'main' })
    expect(parseReviewCommand('/review branch release/1.2')).toEqual({ kind: 'baseBranch', branch: 'release/1.2' })
    expect(parseReviewCommand('/review commit abc123')).toEqual({ kind: 'commit', sha: 'abc123' })
    expect(parseReviewCommand('/review focus on auth regressions')).toEqual({
      kind: 'custom',
      instructions: 'focus on auth regressions'
    })
    expect(parseReviewCommand('/reviewer')).toBe(false)
  })

  it('parses research topics and fills the research brief', () => {
    expect(parseResearchCommand('/research')).toBeNull()
    expect(parseResearchCommand('/deepresearch cache economics')).toBe('cache economics')
    expect(parseResearchCommand('/deep-research web + papers')).toBe('web + papers')
    expect(parseResearchCommand('/researcher')).toBe(false)
    expect(buildResearchPrompt('Topic: {{topic}}', 'provider cache')).toBe('Topic: provider cache')
    expect(buildResearchPrompt('Topic: {{topic}}', null)).toBe('Topic: {{topic}}')
  })

  it('uses ordinary composer text as a goal draft only when the goal panel is open', () => {
    expect(getGoalPanelDraftObjective('ship the goal UX', true)).toBe('ship the goal UX')
    expect(getGoalPanelDraftObjective('  ship the goal UX  ', true)).toBe('ship the goal UX')
    expect(getGoalPanelDraftObjective('ship the goal UX', false)).toBe('')
    expect(getGoalPanelDraftObjective('/goal pause', true)).toBe('')
    expect(getGoalPanelDraftObjective('/compact after this', true)).toBe('')
  })
})

describe('FloatingComposer goal helpers', () => {
  it('formats elapsed goal time compactly', () => {
    expect(formatGoalElapsedSeconds(3)).toBe('3s')
    expect(formatGoalElapsedSeconds(60)).toBe('1m')
    expect(formatGoalElapsedSeconds(125)).toBe('2m 5s')
    expect(formatGoalElapsedSeconds(3720)).toBe('1h 2m')
  })

  it('shows the goal banner only when no other composer overlay is active', () => {
    expect(shouldShowGoalFloater({
      compact: false,
      hasActiveGoal: true,
      slashQuery: null,
      goalPanelOpen: false,
      composerMenuOpen: false
    })).toBe(true)

    expect(shouldShowGoalFloater({
      compact: true,
      hasActiveGoal: true,
      slashQuery: null,
      goalPanelOpen: false,
      composerMenuOpen: false
    })).toBe(false)

    expect(shouldShowGoalFloater({
      compact: false,
      hasActiveGoal: true,
      slashQuery: 'goal',
      goalPanelOpen: false,
      composerMenuOpen: false
    })).toBe(false)

    expect(shouldShowGoalFloater({
      compact: false,
      hasActiveGoal: true,
      slashQuery: null,
      goalPanelOpen: true,
      composerMenuOpen: false
    })).toBe(false)

    expect(shouldShowGoalFloater({
      compact: false,
      hasActiveGoal: false,
      slashQuery: null,
      goalPanelOpen: false,
      composerMenuOpen: false
    })).toBe(false)
  })
})

describe('FloatingComposer usage footer helpers', () => {
  it('keeps nonzero unpriced Go runtime usage visible without presenting zero cost', () => {
    const display = buildComposerThreadUsageDisplay({
      inputTokens: 100,
      outputTokens: 20,
      reasoningTokens: 0,
      totalTokens: 120,
      cachedTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.4,
      lastTurnCacheHitRate: 0.986,
      costUsd: 0,
      costCny: 0,
      priceConfigured: false,
      turns: 2,
      tokenEconomySavingsTokens: 4096
    }, 'en')

    expect(display.tokens).toBe('120')
    expect(display.cost).toBe('Not configured')
    expect(display.cost).not.toBe('$0.0000')
    expect(display.saved).toBe('4.1k')
    expect(display.cache).toBe('40%')
    expect(display.primaryCache).toBe('99%')
    expect(display.latestCache).toBe('99%')
    expect(display.cached).toBe('80')
    expect(display.miss).toBe('20')
    expect(display.turns).toBe(2)
    expect(display.showContextSavings).toBe(true)
    expect(display.showCache).toBe(true)
    expect(display.showLatestCache).toBe(true)
    expect(buildComposerTurnChipText(display)).toBe('2')
  })

  it('keeps configured zero price distinct from missing price configuration', () => {
    const display = buildComposerThreadUsageDisplay({
      inputTokens: 8,
      outputTokens: 2,
      reasoningTokens: 0,
      totalTokens: 10,
      cachedTokens: 0,
      cacheMissTokens: 10,
      cacheHitRate: 0,
      lastTurnCacheHitRate: null,
      costUsd: 0,
      costCny: null,
      priceConfigured: true,
      turns: 1,
      tokenEconomySavingsTokens: 0
    }, 'en')

    expect(display.tokens).toBe('10')
    expect(display.cost).toBe('$0.0000')
    expect(display.showContextSavings).toBe(false)
    expect(display.showCache).toBe(false)
    expect(display.showLatestCache).toBe(false)
  })

  it('uses the same turn count for the compact circular turn chip', () => {
    const display = buildComposerThreadUsageDisplay({
      inputTokens: 100,
      outputTokens: 20,
      reasoningTokens: 0,
      totalTokens: 120,
      cachedTokens: 46,
      cacheMissTokens: 54,
      cacheHitRate: 0.46,
      lastTurnCacheHitRate: null,
      costUsd: 0,
      costCny: 0,
      priceConfigured: false,
      turns: 3,
      tokenEconomySavingsTokens: 0
    }, 'zh')

    expect(display.turns).toBe(3)
    expect(buildComposerTurnChipText(display)).toBe('3')
  })

  it('keeps the circular turn chip empty until usage has loaded', () => {
    expect(buildComposerTurnChipText(null)).toBe('-')
  })
})

describe('FloatingComposer file references', () => {
  it('parses @ file mention queries at the current cursor', () => {
    expect(getFileMentionAtCursor('please inspect @src/ren', 'please inspect @src/ren'.length)).toEqual({
      start: 15,
      end: 23,
      query: 'src/ren',
      quoted: false
    })
    expect(getFileMentionAtCursor('compare @"docs/product plan', 'compare @"docs/product plan'.length)).toEqual({
      start: 8,
      end: 27,
      query: 'docs/product plan',
      quoted: true
    })
    expect(getFileMentionAtCursor('email test@example.com', 'email test@example.com'.length)).toBeNull()
  })

  it('formats, inserts, removes, and ranks composer file references', () => {
    const files = [
      { path: '/repo/src/App.tsx', relativePath: 'src/App.tsx', name: 'App.tsx' },
      { path: '/repo/package.json', relativePath: 'package.json', name: 'package.json' },
      { path: '/repo/docs/product plan.md', relativePath: 'docs/product plan.md', name: 'product plan.md' }
    ]

    expect(formatComposerFileMentionToken('docs/product plan.md')).toBe('@"docs/product plan.md"')
    expect(filterWorkspaceFileMentionSuggestions(files, 'pack')).toEqual([files[1]])

    const mention = getFileMentionAtCursor('open @doc', 'open @doc'.length)
    expect(mention).not.toBeNull()
    const replaced = replaceFileMentionInInput('open @doc', mention!, files[2])
    expect(replaced.input).toBe('open @"docs/product plan.md" ')
    expect(removeComposerFileMentionToken(replaced.input, files[2].relativePath)).toBe('open')
  })

  it('builds file references from picked local paths', () => {
    expect(composerFileReferenceFromPath('/repo/src/App.tsx', '/repo')).toEqual({
      path: '/repo/src/App.tsx',
      relativePath: 'src/App.tsx',
      name: 'App.tsx',
      type: 'file'
    })
    expect(composerFileReferenceFromPath('/tmp/outside.txt', '/repo')).toEqual({
      path: '/tmp/outside.txt',
      relativePath: '/tmp/outside.txt',
      name: 'outside.txt',
      type: 'file',
      workspaceRoot: null
    })
  })

  it('formats, inserts, and removes directory mentions with a trailing slash', () => {
    expect(formatComposerFileMentionToken('src/components', true)).toBe('@src/components/')
    expect(formatComposerFileMentionToken('docs/product specs', true)).toBe('@"docs/product specs/"')

    const mention = getFileMentionAtCursor('check @src/comp', 'check @src/comp'.length)
    expect(mention).not.toBeNull()
    const replaced = replaceFileMentionInInput('check @src/comp', mention!, {
      relativePath: 'src/components',
      type: 'directory'
    })
    expect(replaced.input).toBe('check @src/components/ ')
    expect(removeComposerFileMentionToken(replaced.input, 'src/components', true)).toBe('check')
  })

  it('keeps a nested file mention intact when removing its parent directory mention', () => {
    const input = 'review @src/ and @src/App.tsx now'
    expect(removeComposerFileMentionToken(input, 'src', true)).toBe('review and @src/App.tsx now')
    // …even when the nested file mention appears before the standalone directory token.
    const reordered = 'review @src/App.tsx and @src/ now'
    expect(removeComposerFileMentionToken(reordered, 'src', true)).toBe('review @src/App.tsx and now')
  })

  it('ranks directories alongside files and favors them for trailing-slash queries', () => {
    const entries: ComposerFileReference[] = [
      { path: '/repo/src', relativePath: 'src', name: 'src', type: 'directory' },
      { path: '/repo/src/App.tsx', relativePath: 'src/App.tsx', name: 'App.tsx', type: 'file' },
      { path: '/repo/src/index.ts', relativePath: 'src/index.ts', name: 'index.ts', type: 'file' }
    ]
    const suggestions = filterWorkspaceFileMentionSuggestions(entries, 'src/')
    expect(suggestions[0]).toEqual(entries[0])
    expect(suggestions.map((entry) => entry.relativePath)).toContain('src/App.tsx')
  })

  it('lists every indexed file beneath a referenced directory', () => {
    const files: ComposerFileReference[] = [
      { path: '/repo/src/App.tsx', relativePath: 'src/App.tsx', name: 'App.tsx', type: 'file' },
      { path: '/repo/src/lib/util.ts', relativePath: 'src/lib/util.ts', name: 'util.ts', type: 'file' },
      { path: '/repo/docs/readme.md', relativePath: 'docs/readme.md', name: 'readme.md', type: 'file' }
    ]
    expect(filesUnderDirectory(files, 'src').map((file) => file.relativePath)).toEqual([
      'src/App.tsx',
      'src/lib/util.ts'
    ])
    expect(isFileWithinDirectory('src/App.tsx', 'src')).toBe(true)
    expect(isFileWithinDirectory('srcabc/App.tsx', 'src')).toBe(false)
    expect(isFileWithinDirectory('docs/readme.md', 'src')).toBe(false)
  })

  it('builds a compact prompt from referenced workspace files', () => {
    const prompt = buildComposerFileContextPrompt('summarize this', [{
      relativePath: 'src/App.tsx',
      content: 'export function App() {}',
      truncated: true
    }])

    expect(prompt).toContain('<workspace_file path="src/App.tsx" truncated="true">')
    expect(prompt).toContain('export function App() {}')
    expect(prompt).toContain('User request:\nsummarize this')
  })
})

describe('FloatingComposer model controls', () => {
  it('passes explicit reasoning choices through to the runtime', () => {
    expect(composerReasoningEffortRequestValue('off')).toBe('off')
    expect(composerReasoningEffortRequestValue('low')).toBe('low')
    expect(composerReasoningEffortRequestValue('max')).toBe('max')
    expect(composerReasoningEffortRequestValue(' high ')).toBeUndefined()
    expect(composerReasoningEffortRequestValue('HIGH')).toBeUndefined()
    expect(composerReasoningEffortRequestValue('none')).toBeUndefined()
  })

  it('falls back to the model default when the selected model does not support the current effort', () => {
    expect(normalizeComposerReasoningEffort('max', {
      reasoning: {
        supportedEfforts: ['off', 'low', 'medium', 'high'],
        defaultEffort: 'high',
        requestProtocol: 'mimo-chat-completions'
      }
    })).toBe('high')
  })

  it('hides the off reasoning option in the composer menu', () => {
    const profile = {
      reasoning: {
        supportedEfforts: ['off' as const, 'low' as const, 'medium' as const, 'high' as const],
        defaultEffort: 'high' as const,
        requestProtocol: 'mimo-chat-completions' as const
      }
    }

    expect(composerReasoningMenuOptionsForModel(profile).map((option) => option.id)).toEqual([
      'low',
      'medium',
      'high'
    ])
    expect(normalizeComposerReasoningMenuEffort('off', profile)).toBe('high')
  })

  it('anchors the model menu to the trigger using the rendered menu height', () => {
    const placement = calculateFloatingMenuPlacement({
      anchorRect: { top: 780, right: 920, bottom: 816 },
      menuHeight: 140,
      viewportHeight: 900,
      viewportWidth: 1000
    })

    expect(placement.left).toBe(712)
    expect(placement.top).toBe(633)
  })

  it('keeps the model menu anchored when the app UI is zoomed', () => {
    const placement = calculateFloatingMenuPlacement({
      anchorRect: { top: 624, right: 736, bottom: 652.8 },
      menuHeight: 140,
      viewportHeight: 720,
      viewportWidth: 800,
      coordinateScale: 0.8
    })

    expect(placement.left).toBe(712)
    expect(placement.top).toBe(633)
  })

  it('places the model submenu beside the active provider row', () => {
    const placement = calculateFloatingSubmenuPlacement({
      anchorRect: { top: 650, right: 700, bottom: 686, left: 492 },
      submenuHeight: 140,
      viewportHeight: 900,
      viewportWidth: 1000
    })

    expect(placement.left).toBe(706)
    expect(placement.top).toBe(642)
  })

  it('flips the model submenu left when there is not enough room on the right', () => {
    const placement = calculateFloatingSubmenuPlacement({
      anchorRect: { top: 650, right: 920, bottom: 686, left: 712 },
      submenuHeight: 140,
      viewportHeight: 900,
      viewportWidth: 1000
    })

    expect(placement.left).toBe(474)
    expect(placement.top).toBe(642)
  })

  it('keeps non-text models out of the composer model menu', () => {
    const group = {
      modelProfiles: {
        'glm-4v': {
          inputModalities: ['text', 'image'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text', 'image_url']
        },
        'banana-canvas': {
          inputModalities: ['text'],
          outputModalities: ['image'],
          supportsToolCalling: false,
          messageParts: ['text']
        }
      }
    } satisfies Parameters<typeof composerMenuSupportsModel>[0]

    expect(composerMenuSupportsModel(group, 'glm-4v')).toBe(true)
    expect(composerMenuSupportsModel(group, 'unknown-chat-model')).toBe(true)
    expect(composerMenuSupportsModel(group, 'banana-canvas')).toBe(false)
    expect(composerMenuSupportsModel(group, 'whisper-1')).toBe(false)
    expect(composerMenuSupportsModel(group, 'dall-e-3')).toBe(false)
    expect(composerMenuSupportsModel(group, 'seedream-4-0-250828')).toBe(false)
    expect(composerMenuSupportsModel(group, 'text-embedding-3-large')).toBe(false)
  })

  it('keeps provider model aliases out of the ungrouped fallback menu', () => {
    const groups = buildComposerModelMenuGroups({
      composerModelGroups: [{
        providerId: 'minimax-token-plan',
        label: 'MiniMax Token Plan',
        modelIds: ['minimax-m3'],
        modelProfiles: {
          'minimax-m3': {
            aliases: ['MiniMax-M3'],
            inputModalities: ['text', 'image'],
            outputModalities: ['text'],
            supportsToolCalling: true,
            messageParts: ['text', 'image_url']
          }
        }
      }],
      modelOptions: ['MiniMax-M3', 'loose-model'],
      ungroupedLabel: 'Other models'
    })

    expect(groups).toHaveLength(2)
    expect(groups[0]).toMatchObject({
      providerId: 'minimax-token-plan',
      modelIds: ['minimax-m3']
    })
    expect(groups[1]).toMatchObject({
      providerId: '__composer_models__',
      label: 'Other models',
      modelIds: ['loose-model']
    })
  })

  it('splits mixed-family and ungrouped model lists without provider-specific behavior', () => {
    const mixedProviderGroups = buildComposerModelMenuGroups({
      composerModelGroups: [{
        providerId: 'local-relay',
        label: 'Local relay',
        modelIds: ['qwen3.7-plus', 'deepseek-v4-pro', 'mimo-v2.5-pro']
      }],
      modelOptions: [],
      ungroupedLabel: 'Other models'
    })

    expect(mixedProviderGroups.map((group) => ({
      label: group.label,
      providerId: group.providerId,
      subtitleKey: group.subtitleKey,
      modelIds: group.modelIds
    }))).toEqual([
      {
        label: 'DeepSeek',
        providerId: 'local-relay',
        subtitleKey: 'composerModelFamilyDeepseek',
        modelIds: ['deepseek-v4-pro']
      },
      {
        label: 'MiMo',
        providerId: 'local-relay',
        subtitleKey: 'composerModelFamilyMimo',
        modelIds: ['mimo-v2.5-pro']
      },
      {
        label: 'Qwen',
        providerId: 'local-relay',
        subtitleKey: 'composerModelFamilyQwen',
        modelIds: ['qwen3.7-plus']
      }
    ])

    const ungrouped = buildComposerModelMenuGroups({
      composerModelGroups: [],
      modelOptions: ['deepseek-v4-flash', 'mimo-v2.5', 'qwen3.6-plus'],
      ungroupedLabel: 'Other models'
    })

    expect(ungrouped.map((group) => group.label)).toEqual(['DeepSeek', 'MiMo', 'Qwen'])
    expect(ungrouped.every((group) => group.providerId === '__composer_models__')).toBe(true)
  })

  it('labels known multimodal relay models separately from text-only models', () => {
    const group = {
      modelProfiles: {
        'custom-vision': {
          inputModalities: ['text' as const, 'image' as const],
          outputModalities: ['text' as const],
          supportsToolCalling: true,
          messageParts: ['text' as const, 'image_url' as const]
        }
      }
    }

    expect(composerModelCapabilityKind(group, 'deepseek-v4-pro')).toBe('text')
    expect(composerModelCapabilityKind(group, 'mimo-v2.5')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'mimo-v2.5-pro')).toBe('text')
    expect(composerModelCapabilityKind(group, 'qwen3-vl-plus')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'qwen3.7-plus')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'qwen3.7-max')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'qwen3.6-plus')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'qwen3.6-flash')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'qwen3.6-flash-2026-04-16')).toBe('multimodal')
    expect(composerModelCapabilityKind(group, 'custom-vision')).toBe('multimodal')
    expect(composerModelIdLooksMultimodal('qwen3.7-plus')).toBe(true)
    expect(composerModelIdLooksMultimodal('gpt-4o-mini')).toBe(true)
    expect(composerModelIdLooksMultimodal('custom-vision-model')).toBe(true)
    expect(composerModelIdLooksMultimodal('custom-visual-model')).toBe(true)
    expect(composerModelIdLooksMultimodal('custom-multimodal-model')).toBe(true)
    expect(composerModelIdLooksMultimodal('custom-omni-model')).toBe(true)
    expect(composerModelIdLooksMultimodal('gpt-4o-audio-preview')).toBe(false)
    expect(composerModelIdLooksMultimodal('deepseek-v4-pro')).toBe(false)
  })

  it('deduplicates models within a provider but keeps the same model id across providers', () => {
    const groups = buildComposerModelMenuGroups({
      composerModelGroups: [
        {
          providerId: 'deepseek',
          label: 'DeepSeek',
          modelIds: ['deepseek-v4-pro', 'deepseek-v4-pro'],
          modelProfiles: {}
        },
        {
          providerId: 'custom-provider-3',
          label: 'test',
          modelIds: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ],
      modelOptions: ['deepseek-v4-pro'],
      ungroupedLabel: 'Other models'
    })

    expect(groups).toEqual([
      expect.objectContaining({
        providerId: 'deepseek',
        modelIds: ['deepseek-v4-pro']
      }),
      expect.objectContaining({
        providerId: 'custom-provider-3',
        modelIds: ['deepseek-v4-pro']
      })
    ])
  })

  it('selects duplicate model ids by provider and model id together', () => {
    expect(composerModelMenuItemSelected({
      groupProviderId: 'deepseek',
      selectedProviderId: 'deepseek',
      currentModel: 'deepseek-v4-pro',
      modelId: 'deepseek-v4-pro'
    })).toBe(true)
    expect(composerModelMenuItemSelected({
      groupProviderId: 'custom-provider-3',
      selectedProviderId: 'deepseek',
      currentModel: 'deepseek-v4-pro',
      modelId: 'deepseek-v4-pro'
    })).toBe(false)
  })

  it('selects canonical provider model rows when the current model is a deprecated alias', () => {
    expect(composerModelMenuItemSelected({
      groupProviderId: 'xiaomi-token-plan',
      selectedProviderId: 'xiaomi-token-plan',
      currentModel: 'mimo-v2.5-pro-ultraspeed',
      modelId: 'mimo-v2.5-pro',
      aliases: ['mimo-v2.5-pro-ultraspeed']
    })).toBe(true)
    expect(composerModelMenuItemSelected({
      groupProviderId: 'deepseek',
      selectedProviderId: 'xiaomi-token-plan',
      currentModel: 'mimo-v2.5-pro-ultraspeed',
      modelId: 'deepseek-v4-pro'
    })).toBe(false)
  })

  it('filters provider model ids by substring without changing the empty query list', () => {
    const modelIds = [
      'deepseek-v4-pro',
      'MiniMax-M2',
      'moonshot-v1-128k'
    ]

    expect(filterComposerModelIds(modelIds, '')).toEqual(modelIds)
    expect(filterComposerModelIds(modelIds, 'max')).toEqual(['MiniMax-M2'])
    expect(filterComposerModelIds(modelIds, '128K')).toEqual(['moonshot-v1-128k'])
  })

  it('keeps the reasoning strength visible in the model control', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact: false,
        mode: 'select',
        composerModel: 'auto',
        composerPickList: ['auto', 'deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        composerReasoningEffort: 'high',
        canChangeModel: true,
        onComposerModelChange: () => undefined,
        onComposerReasoningEffortChange: () => undefined
      })
    )

    expect(html).toContain('Auto')
    expect(html).toContain('High')
  })

  it('keeps provider setup reachable when no chat providers are available', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact: false,
        mode: 'select',
        composerModel: 'auto',
        composerPickList: ['auto'],
        composerModelGroups: [],
        canChangeModel: false,
        onComposerModelChange: () => undefined,
        onConfigureProviders: () => undefined
      })
    )

    expect(html).toContain('Set up provider')
    expect(html).toContain('aria-haspopup="menu"')
    expect(html).not.toContain('disabled=""')
  })

  it.each([
    { surface: 'Code', compact: false, mode: 'select' as const },
    { surface: 'Write', compact: true, mode: 'combobox' as const }
  ])('offers committed default models in the $surface picker', ({ compact, mode }) => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact,
        mode,
        composerModel: 'deepseek-v4-flash',
        composerProviderId: 'deepseek',
        composerPickList: ['deepseek-v4-flash', 'deepseek-v4-pro'],
        composerModelGroups: [{
          providerId: 'deepseek',
          label: 'deepseek',
          modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro']
        }],
        canChangeModel: true,
        onComposerModelChange: () => undefined,
        onConfigureProviders: () => undefined
      })
    )

    expect(html).toContain('deepseek-v4-flash')
    expect(html).toContain('aria-haspopup="menu"')
    expect(html).not.toContain('Set up provider')
  })

  it('does not treat default fallback models as configured providers', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact: false,
        mode: 'select',
        composerModel: 'deepseek-v4-pro',
        composerPickList: ['deepseek-v4-pro', 'deepseek-v4-flash'],
        composerModelGroups: [],
        canChangeModel: true,
        onComposerModelChange: () => undefined,
        onConfigureProviders: () => undefined
      })
    )

    expect(html).toContain('Set up provider')
    expect(html).not.toContain('deepseek-v4-pro')
  })
})

describe('FloatingComposer image transfer helpers', () => {
  it('extracts image files from clipboard or drop payloads', () => {
    const screenshot = new File([new Uint8Array([1, 2, 3])], 'shot.png', { type: 'image/png' })
    const pastedWebp = new File([new Uint8Array([4])], '', { type: 'image/webp' })
    const notes = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    const source = {
      items: {
        length: 3,
        0: { kind: 'file', type: 'image/webp', getAsFile: () => pastedWebp },
        1: { kind: 'file', type: 'text/plain', getAsFile: () => notes },
        2: { kind: 'string', type: 'text/plain', getAsFile: () => null }
      },
      files: {
        length: 2,
        0: screenshot,
        1: notes
      }
    }

    expect(imageFilesFromTransfer(source)).toEqual([pastedWebp, screenshot])
    expect(imageTransferHasImages(source)).toBe(true)
  })

  it('deduplicates files exposed through both transfer item and file lists', () => {
    const screenshot = new File([new Uint8Array([1])], 'shot.png', { type: 'image/png' })
    const source = {
      items: {
        length: 1,
        0: { kind: 'file', type: 'image/png', getAsFile: () => screenshot }
      },
      files: {
        length: 1,
        0: screenshot
      }
    }

    expect(imageFilesFromTransfer(source)).toEqual([screenshot])
  })

  it('keeps clipboard item MIME hints when pasted image files omit their own type', () => {
    const screenshot = new File([new Uint8Array([1])], 'shot', { type: '' })
    const source = {
      items: {
        length: 1,
        0: { kind: 'file', type: 'image/png', getAsFile: () => screenshot }
      },
      files: {
        length: 0
      }
    }

    const [file] = imageFilesFromTransfer(source)

    expect(file).toBeInstanceOf(File)
    expect(file?.type).toBe('image/png')
    expect(file?.name).toBe('shot')
    expect(imageTransferHasImages(source)).toBe(true)
  })

  it('routes pasted image files through the clipboard bridge when available', () => {
    const screenshot = new File([new Uint8Array([1])], 'shot.png', { type: 'image/png' })
    const preventDefault = vi.fn()
    const onPickAttachments = vi.fn()
    const onPasteClipboardImage = vi.fn()
    const handled = handleComposerImagePaste({
      canPickAttachment: true,
      clipboardData: {
        getData: () => '',
        items: {
          length: 1,
          0: { kind: 'file', type: 'image/png', getAsFile: () => screenshot }
        }
      },
      preventDefault,
      onPickAttachments,
      onPasteClipboardImage
    })

    expect(handled).toBe(true)
    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(onPickAttachments).not.toHaveBeenCalled()
    expect(onPasteClipboardImage).toHaveBeenCalledWith({ silentNoImage: false })
  })

  it('still uses the attachment picker for pasted image files when the clipboard bridge is unavailable', () => {
    const screenshot = new File([new Uint8Array([1])], 'shot.png', { type: 'image/png' })
    const preventDefault = vi.fn()
    const onPickAttachments = vi.fn()
    const handled = handleComposerImagePaste({
      canPickAttachment: true,
      clipboardData: {
        getData: () => '',
        items: {
          length: 1,
          0: { kind: 'file', type: 'image/png', getAsFile: () => screenshot }
        }
      },
      preventDefault,
      onPickAttachments
    })

    expect(handled).toBe(true)
    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(onPickAttachments).toHaveBeenCalledWith([screenshot])
  })

  it('does not intercept ordinary text paste', () => {
    const preventDefault = vi.fn()
    const onPasteClipboardImage = vi.fn()
    const handled = handleComposerImagePaste({
      canPickAttachment: true,
      clipboardData: {
        getData: (format) => format === 'text/plain' ? 'hello' : ''
      },
      preventDefault,
      onPasteClipboardImage
    })

    expect(handled).toBe(false)
    expect(preventDefault).not.toHaveBeenCalled()
    expect(onPasteClipboardImage).toHaveBeenCalledWith({ silentNoImage: true })
  })

  it('falls back to the Electron clipboard image bridge when files are unavailable', () => {
    const preventDefault = vi.fn()
    const onPasteClipboardImage = vi.fn()
    const handled = handleComposerImagePaste({
      canPickAttachment: true,
      clipboardData: {
        getData: () => ''
      },
      preventDefault,
      onPasteClipboardImage
    })

    expect(handled).toBe(true)
    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(onPasteClipboardImage).toHaveBeenCalledWith({ silentNoImage: false })
  })

  it('round-trips plugin mention nodes back to raw prompt tokens', () => {
    const token = '@[Analytix Computer Use](plugin://analytix-computer-use)'
    const input = `Use ${token} for TextEdit smoke`
    const doc = parseComposerPromptDoc(input, {
      plugins: [{
        pluginId: 'analytix-computer-use',
        title: 'Analytix Computer Use',
        description: 'Desktop automation',
        brandColor: '#3366ff'
      }]
    })

    expect(serializeComposerPromptDoc(doc)).toBe(input)
  })

  it('maps plugin mentions as one ProseMirror inline atom while preserving raw offsets', () => {
    const token = '@[Analytix 涉案资金研判](plugin://analytix-fund-analysis)'
    const input = `使用 ${token} 插件`
    const doc = parseComposerPromptDoc(input)
    const projection = buildComposerPromptProjection(doc)
    const mention = projection.segments.find((segment) => segment.kind === 'mention')

    if (!mention) throw new Error('Expected plugin mention projection segment')
    expect(mention.rawEnd - mention.rawStart).toBe(token.length)
    expect(mention.pmEnd - mention.pmStart).toBe(1)
    expect(pmPosToComposerRawOffset(doc, mention.pmEnd)).toBe(input.indexOf(token) + token.length)
    expect(composerRawOffsetToPmPos(doc, input.indexOf(token) + token.length)).toBe(mention.pmEnd)
  })
})

describe('FloatingComposer capability controls', () => {
  it('uses only the explicit speech UI preference, not copied Provider fields, to expose the Go-resolved attempt', () => {
    const settingsWith = (enabled: boolean, baseUrl: string, model: string): Pick<AppSettingsV1, 'runtime'> => ({
      runtime: {
        speechToText: { enabled, baseUrl, model }
      }
    } as unknown as Pick<AppSettingsV1, 'runtime'>)

    expect(speechToTextPreferenceEnabled(settingsWith(true, '', ''))).toBe(true)
    expect(speechToTextPreferenceEnabled(settingsWith(true, 'https://stale.invalid/v1', 'stale-model'))).toBe(true)
    expect(speechToTextPreferenceEnabled(settingsWith(false, 'https://stale.invalid/v1', 'stale-model'))).toBe(false)
    expect(floatingComposerSource).toContain('const showVoiceDictation = speechToTextEnabled')
    expect(floatingComposerSource).not.toMatch(/speechToTextSettings\.(?:baseUrl|model)/u)
    expect(floatingComposerSource).not.toContain('speechToText: speechToTextSettings')
  })

  it('enables quote-only sending while preserving the runtime readiness gate', () => {
    useChatStore.setState({ activeThreadId: 'thr_quote', activeThreadGoal: null, route: 'chat', workspaceRoot: '/workspace/analytix' })
    const render = (count: number, ready = true): string => renderToStaticMarkup(createElement(FloatingComposer, {
      input: '', setInput: () => undefined, mode: 'agent', setMode: () => undefined,
      busy: false, runtimeReady: ready, hasActiveThread: true,
      composerModel: '', composerPickList: [], onComposerModelChange: () => undefined,
      queuedMessages: [], onRemoveQueuedMessage: () => undefined, onSend: () => undefined,
      onInterrupt: () => undefined, documentReferenceCount: count
    })).match(/<button[^>]*ds-composer-primary-action-button[^>]*>/)?.[0] ?? ''
    expect(render(1)).toContain('ds-composer-primary-action-button')
    expect(render(1)).not.toContain('disabled=""')
    expect(render(0)).toContain('disabled=""')
    expect(render(1, false)).toContain('disabled=""')
  })

  it('renders the ProseMirror composer host instead of the legacy overlay textarea', () => {
    useChatStore.setState({
      activeThreadId: 'thr_mention',
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix'
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '使用 @[Computer Use](plugin://computer-use) 插件',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        workspaceRootOverride: '/workspace/analytix',
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(composerEditorHostHtml(html)).toContain('data-composer-prompt-editor="true"')
    expect(html).not.toContain('data-composer-mention-visual')
    expect(html).not.toContain('<textarea')
  })

  it('renders queued messages in the above-composer stack', () => {
    useChatStore.setState({
      activeThreadId: 'thr_1',
      activeThreadGoal: {
        threadId: 'thr_1',
        objective: 'Ship the top tray',
        status: 'active',
        tokensUsed: 12,
        timeUsedSeconds: 61,
        createdAt: '2026-06-29T00:00:00.000Z',
        updatedAt: '2026-06-29T00:01:00.000Z'
      },
      activeThreadHandoffOperationId: null,
      queuedMessagesPausedReason: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix',
      threadHandoffOperations: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        workspaceRootOverride: '/workspace/analytix',
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [{
          id: 'queued-1',
          text: 'queued follow-up',
          mode: 'agent',
          model: 'deepseek-v4-pro',
          providerId: 'deepseek',
          reasoningEffort: 'high'
        }],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(html).toContain('data-above-composer-portal="true"')
    expect(html.indexOf('queued follow-up')).toBeGreaterThanOrEqual(0)
  })

  it('enables goal setup before a thread exists when a workspace is available', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: ''
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '/goal',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: false,
        workspaceRootOverride: '/workspace/analytix',
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    const goalButton = html.match(/<button[^>]*>[\s\S]*?\/goal[\s\S]*?<\/button>/)?.[0] ?? ''
    expect(goalButton).toContain('/goal')
    expect(goalButton).not.toContain('disabled=""')
  })

  it('enables new session before a thread exists when a workspace is available', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: ''
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '/new',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: false,
        workspaceRootOverride: '/workspace/analytix',
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        onNewCommand: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    const newButton = html.match(/<button[^>]*>[\s\S]*?\/new[\s\S]*?<\/button>/)?.[0] ?? ''
    expect(newButton).toContain('/new')
    expect(newButton).not.toContain('disabled=""')
  })

  it('enables plan mode before a thread exists when a workspace is available', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: ''
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '/plan',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: false,
        workspaceRootOverride: '/workspace/analytix',
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        onPlanCommand: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    const planButton = html.match(/<button[^>]*>[\s\S]*?\/plan[\s\S]*?<\/button>/)?.[0] ?? ''
    expect(planButton).toContain('/plan')
    expect(planButton).not.toContain('disabled=""')
  })

  it('shows discovered project Skills in the slash command menu', () => {
    useChatStore.setState({
      activeThreadId: 'thr_1',
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix',
      threads: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '/openspec',
        setInput: () => undefined,
        workspaceRootOverride: '/workspace/analytix',
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false,
        skillCommands: [{
          id: 'openspec-apply-change',
          name: 'Openspec Apply Change',
          description: 'Implement tasks from an OpenSpec change',
          root: '/workspace/analytix/.codex/skills/openspec-apply-change'
        }]
      })
    )

    expect(html).toContain('Openspec Apply Change')
    expect(html).toContain('Implement tasks from an OpenSpec change')
    expect(html).toContain('Project')
    expect(html).toContain('/skill:openspec-apply-change')
  })

  it('hides disabled Skills from the slash command menu', () => {
    useChatStore.setState({
      activeThreadId: 'thr_1',
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix',
      threads: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '/skill',
        setInput: () => undefined,
        workspaceRootOverride: '/workspace/analytix',
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false,
        disabledSkillIds: ['/skill:test-skill-08'],
        skillCommands: [
          {
            id: 'test-skill-08',
            name: 'Test Skill 08',
            description: 'Disabled test skill',
            root: '/workspace/analytix/.agents/skills/test-skill-08'
          },
          {
            id: 'test-skill-09',
            name: 'Test Skill 09',
            description: 'Enabled test skill',
            root: '/workspace/analytix/.agents/skills/test-skill-09'
          }
        ]
      })
    )

    expect(html).not.toContain('Test Skill 08')
    expect(html).not.toContain('/skill:test-skill-08')
    expect(html).toContain('Test Skill 09')
    expect(html).toContain('/skill:test-skill-09')
  })

  it('enables local Claw input when a WeChat channel is already mapped to a local thread', () => {
    useChatStore.setState({
      activeThreadId: 'thr_weixin',
      activeThreadGoal: null,
      route: 'claw',
      workspaceRoot: '',
      activeClawChannelId: 'channel_weixin',
      clawChannels: [{
        id: 'channel_weixin',
        provider: 'weixin',
        label: 'weixin agent',
        enabled: true,
        model: 'auto',
        threadId: 'thr_weixin',
        workspaceRoot: '',
        agentProfile: {
          name: '',
          description: '',
          identity: '',
          personality: '',
          userContext: '',
          replyRules: ''
        },
        platformAccount: {
          kind: 'weixin',
          accountId: 'wx_account',
          createdAt: '2026-06-02T00:00:00.000Z'
        },
        conversations: [],
        createdAt: '2026-06-02T00:00:00.000Z',
        updatedAt: '2026-06-02T00:00:00.000Z'
      }]
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: 'auto',
        composerPickList: ['auto'],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    const editor = composerEditorHostHtml(html)
    expect(editor).not.toContain('aria-disabled="true"')
    expect(editor).not.toContain('先去飞书')
  })

  it('hides image upload when attachment upload is unavailable', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'hello',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )
    expect(html).not.toContain('Attach image')
    expect(html).not.toContain('Image input is unavailable')
  })

  it('renders the plus trigger alongside uploaded attachments', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'describe this',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachments: [{ id: 'att_1', name: 'shot.png', mimeType: 'image/png' }],
        attachmentUploadEnabled: true,
        webAccessAvailable: true,
        onRemoveAttachment: () => undefined
      })
    )
    expect(html).toContain('More actions')
    expect(html).not.toContain('Attach image')
    expect(html).toContain('shot.png')
    expect(html).toContain('image/heic')
  })

  it('uses the primary action to send a follow-up while busy when text is present', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'hello',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: true,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: 'deepseek-v4-pro',
        composerPickList: ['deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(html).toContain('deepseek-v4-pro')
    expect(html).toContain('Send follow-up without stopping')
    expect(composerEditorHostHtml(html)).not.toContain('aria-disabled="true"')
    expect(html).not.toContain('Queue message')
    expect(html.match(/ds-composer-primary-action-button/g)?.length).toBe(1)
    expect(html).not.toContain('Stop and discard')
    expect(html).not.toContain('lucide-trash-2')
    expect(html).not.toContain('lucide-zap')
    expect(html).not.toContain('Default (thread)')
  })

  it('keeps the busy primary action as stop when the draft is empty', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: true,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: 'deepseek-v4-pro',
        composerPickList: ['deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(html).toContain('Stop')
    expect(html).not.toContain('Send follow-up without stopping')
    expect(composerEditorHostHtml(html)).not.toContain('aria-disabled="true"')
  })

  it('keeps the busy primary action as stop when only attachments are present', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: true,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: 'deepseek-v4-pro',
        composerPickList: ['deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachments: [{ id: 'att_busy_empty', name: 'shot.png', mimeType: 'image/png' }],
        attachmentUploadEnabled: true,
        webAccessAvailable: true,
        onRemoveAttachment: () => undefined
      })
    )

    expect(html).toContain('Stop')
    expect(html).toContain('shot.png')
    expect(html).not.toContain('Send follow-up without stopping')
  })

  it('renders the model control chip without an empty default option', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact: false,
        mode: 'select',
        composerModel: 'deepseek-v4-pro',
        composerPickList: ['auto', 'deepseek-v4-flash', 'deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        canChangeModel: true,
        onComposerReasoningEffortChange: () => undefined,
        onComposerModelChange: () => undefined
      })
    )

    expect(html).toContain('deepseek-v4-pro')
    expect(html).toContain('Ultra')
    expect(html).toContain('Model and reasoning settings')
    expect(html).not.toContain('>Auto<')
    expect(html).not.toContain('<option value=""></option>')
    expect(html).not.toContain('Default (thread)')
  })

  it('renders compact combobox controls as a picker button with model and reasoning labels', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerModelPicker, {
        compact: true,
        mode: 'combobox',
        composerModel: 'deepseek-v4-flash',
        composerPickList: ['auto', 'deepseek-v4-flash', 'deepseek-v4-pro'],
        composerModelGroups: [DEEPSEEK_PROVIDER_GROUP],
        canChangeModel: true,
        composerReasoningEffort: 'high',
        onComposerReasoningEffortChange: () => undefined,
        onComposerModelChange: () => undefined
      })
    )

    expect(html).toContain('deepseek-v4-flash')
    expect(html).toContain('High')
    expect(html).toContain('Model and reasoning settings')
    expect(html).toContain('aria-haspopup="menu"')
    expect(html).not.toContain('<input')
  })

  it('shows a plan badge in the input toolbar when plan mode is enabled', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'plan this',
        setInput: () => undefined,
        mode: 'plan',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        onPlanCommand: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )
    expect(html).toContain('title="Plan"')
    expect(html).toContain('>Plan</span>')
  })

  it('renders image attachment thumbnails when a local preview is available', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachments: [{
          id: 'att_1',
          name: 'shot.png',
          mimeType: 'image/png',
          previewUrl: 'blob:shot-preview'
        }],
        attachmentUploadEnabled: true,
        webAccessAvailable: true,
        onRemoveAttachment: () => undefined
      })
    )

    expect(html).toContain('src="blob:shot-preview"')
    expect(html).toContain('alt="shot.png"')
  })

  it('renders @ file reference chips as sendable context', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        fileReferenceEnabled: true,
        fileReferences: [{
          path: '/workspace/analytix/src/App.tsx',
          relativePath: 'src/App.tsx',
          name: 'App.tsx'
        }],
        onRemoveFileReference: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(html).toContain('src/App.tsx')
    expect(html).toContain('Remove reference')
    expect(html).toContain('aria-label="Send"')
    expect(html).not.toContain('aria-label="Send" disabled=""')
  })

  it('renders the current permission mode next to the composer menu button', () => {
    useChatStore.setState({
      activeThreadId: 'thr_1',
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix'
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'hello',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false,
        executionSettings: {
          approvalPolicy: 'auto',
          sandboxMode: 'danger-full-access'
        },
        onExecutionSettingsChange: () => undefined
      })
    )

    expect(html).toContain('aria-label="Permissions"')
    expect(html).toContain('Permissions: Full access')
    expect(html).toContain('>Full access<')
    expect(html).toContain('lucide-settings')
  })

  it('renders the standalone permission picker with the current mode label', () => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerExecutionPicker, {
        value: {
          approvalPolicy: 'auto',
          sandboxMode: 'danger-full-access'
        },
        onChange: () => undefined
      })
    )

    expect(html).toContain('aria-label="Permissions"')
    expect(html).toContain('>Full access<')
    expect(html).toContain('Permissions: Full access')
    expect(html).not.toContain('>Execution<')
  })

  it.each([
    ['request approval', { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' }, 'Request approval'],
    ['full access, ask first', { approvalPolicy: 'untrusted', sandboxMode: 'danger-full-access' }, 'Full access, ask first'],
    ['full access', { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }, 'Full access'],
    ['custom', { approvalPolicy: 'on-request', sandboxMode: 'read-only' }, 'Custom']
  ] as const)('renders %s as the visible permission label', (_name, value, label) => {
    const html = renderToStaticMarkup(
      createElement(FloatingComposerExecutionPicker, {
        value,
        onChange: () => undefined
      })
    )

    expect(html).toContain(`>${label}<`)
  })

  it('renders a changed-file review card above the input', () => {
    useChatStore.setState({
      activeThreadId: 'thr_1',
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix'
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'review this',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: true,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false,
        changedFiles: [
          { path: 'src/a.ts', added: 3, removed: 1 },
          { path: 'src/b.ts', added: 2, removed: 4 }
        ],
        changedFileStats: { added: 5, removed: 5 },
        onOpenChanges: () => undefined,
        onReviewChanges: () => undefined
      })
    )

    expect(html).toContain('2 files changed')
    expect(html).toContain('src/a.ts')
    expect(html).toContain('+5')
    expect(html).toContain('-5')
    expect(html).toContain('Preview')
    expect(html).toContain('Review')
  })

  it('keeps the empty-session composer interactive in the Electron drag shell', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix',
      threads: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: '',
        setInput: () => undefined,
        workspaceRootOverride: '/workspace/analytix',
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: false,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(html).toContain('ds-floating-composer ds-no-drag')
    expect(html).toContain('ds-composer-shell ds-chat-composer ds-frosted ds-no-drag')
    const editor = composerEditorHostHtml(html)
    expect(editor).toContain('w-full')
    expect(editor).not.toContain('aria-disabled="true"')
  })

  it('allows typing while a new chat has no selected runtime thread yet', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '',
      threads: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'draft while creating',
        setInput: () => undefined,
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: true,
        hasActiveThread: false,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(composerEditorHostHtml(html)).not.toContain('aria-disabled="true"')
    expect(html).toContain('Choose a working directory before starting or continuing a thread.')
    const sendButton = html.match(/<button[^>]*aria-label="Send"[^>]*>/)?.[0] ?? ''
    expect(sendButton).toContain('disabled=""')
  })

  it('keeps the draft editable while the runtime is loading and shows send loading', () => {
    useChatStore.setState({
      activeThreadId: null,
      activeThreadGoal: null,
      route: 'chat',
      workspaceRoot: '/workspace/analytix',
      threads: []
    })

    const html = renderToStaticMarkup(
      createElement(FloatingComposer, {
        input: 'draft during startup',
        setInput: () => undefined,
        workspaceRootOverride: '/workspace/analytix',
        mode: 'agent',
        setMode: () => undefined,
        busy: false,
        runtimeReady: false,
        hasActiveThread: false,
        composerModel: '',
        composerPickList: [],
        onComposerModelChange: () => undefined,
        queuedMessages: [],
        onRemoveQueuedMessage: () => undefined,
        onSend: () => undefined,
        onInterrupt: () => undefined,
        attachmentUploadEnabled: false,
        webAccessAvailable: false
      })
    )

    expect(composerEditorHostHtml(html)).not.toContain('aria-disabled="true"')
    const sendButton = html.match(/<button[^>]*aria-label="Send"[^>]*>/)?.[0] ?? ''
    expect(sendButton).toContain('disabled=""')
    expect(html).toContain('lucide-loader-circle')
  })
})
