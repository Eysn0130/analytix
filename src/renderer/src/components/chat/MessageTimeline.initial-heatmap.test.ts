import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import i18n from '../../i18n'
import { MessageTimelineEmptyHero } from './message-timeline-empty'

async function readBaseShellSource(): Promise<string> {
  const nodeFs = 'node:fs/promises'
  const { readFile } = await import(/* @vite-ignore */ nodeFs)
  return readFile(new URL('../../styles/base-shell.css', import.meta.url), 'utf8')
}

function renderHero(options: {
  route?: 'chat' | 'claw'
  ready?: boolean
  hasWorkspace?: boolean
  hasComposerWorkspacePicker?: boolean
  runtimeError?: string | null
  phase?: 'settling' | 'hero' | 'active'
} = {}): string {
  return renderToStaticMarkup(
    createElement(MessageTimelineEmptyHero, {
      phase: options.phase,
      route: options.route ?? 'chat',
      ready: options.ready ?? true,
      hasWorkspace: options.hasWorkspace ?? true,
      hasComposerWorkspacePicker: options.hasComposerWorkspacePicker,
      runtimeError: options.runtimeError ?? null,
      activeClawChannel: null,
      onPickWorkspace: () => undefined,
      onRetry: () => undefined,
      onOpenSettings: () => undefined,
      onSelectSuggestion: () => undefined
    })
  )
}

describe('MessageTimeline initial conversation routing', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('shows the requested formal brand with the preview pill beside its title', () => {
    const html = renderHero({ phase: 'hero' })
    expect(html).toContain('dsh-conversation-hero')
    expect(html).toContain('Analytix 灵鉴')
    expect(html).toMatch(/<h1><span>Analytix 灵鉴<\/span><span class="dsh-conversation-preview">Preview<\/span><\/h1>/)
    expect(html).toContain('ax-brand-mark-image-light')
    expect(html).toContain('ax-brand-mark-image-dark')
    expect(html).not.toContain('Start a new conversation')
    expect(html).not.toContain('Pick a task, or just type your question')
    expect(html).not.toContain('Expand calendar')
    expect(html).not.toContain('ds-initial-usage-heatmap')
  })

  it('keeps the same brand and Chinese preview label in the requested locale', async () => {
    await i18n.changeLanguage('zh-CN')
    const html = renderHero({ phase: 'hero' })
    expect(html).toMatch(/<h1><span>Analytix 灵鉴<\/span><span class="dsh-conversation-preview">预览版<\/span><\/h1>/)
  })

  it('shows the actual history-loading phase without flashing a new-conversation hero', () => {
    const html = renderHero({ phase: 'settling' })
    expect(html).toContain('role="status"')
    expect(html).toContain('Loading')
    expect(html).not.toContain('Analytix 灵鉴')
    expect(html).not.toContain('dsh-conversation-hero-mark')
  })

  it('does not insert a hero into an active conversation whose empty row briefly survives a producer update', () => {
    expect(renderHero({ phase: 'active' })).toBe('')
  })

  it('keeps offline, missing-workspace, and Claw entries ahead of the ordinary hero', () => {
    const offlineHtml = renderHero({ ready: false })
    expect(offlineHtml).toContain('Analytix is waking the local agent')
    expect(offlineHtml).toContain('ds-home-transition')
    expect(offlineHtml).toContain('ax-brand-mark')
    const workspaceHtml = renderHero({ hasWorkspace: false })
    expect(workspaceHtml).toContain('Choose working directory')
    expect(workspaceHtml).toContain('Analytix 灵鉴')
    expect(workspaceHtml).toContain('ax-brand-mark')
    expect(workspaceHtml).toContain('dsh-conversation-hero-mark')
    const clawHtml = renderHero({ route: 'claw' })
    expect(clawHtml).toContain('Waiting for WeChat')
    expect(clawHtml).toContain('claw-phone-hero ds-home-transition')
    expect(clawHtml).toContain('ds-home-transition-stage')
    expect(clawHtml).toContain('claw-phone-hero-device')
    expect(clawHtml).toContain('ax-brand-mark')
    expect(clawHtml).not.toContain('Analytix usage')
  })

  it('omits only the duplicate workspace copy and action when the composer provides its picker', () => {
    const html = renderHero({ phase: 'hero', hasWorkspace: false, hasComposerWorkspacePicker: true })
    expect(html).toContain('Analytix 灵鉴')
    expect(html).toContain('dsh-conversation-preview')
    expect(html).not.toContain(i18n.t('common:emptyHeroSubNoWorkspace'))
    expect(html).not.toContain('<button')
  })

  it.each([
    { options: { ready: false, hasWorkspace: false }, expected: 'Analytix is waking the local agent' },
    { options: { ready: false, hasWorkspace: false, runtimeError: 'ISOLATED_RUNTIME_ERROR' }, expected: 'Cannot connect to the local runtime' },
    { options: { route: 'claw', hasWorkspace: false }, expected: 'Choose working directory' },
    { options: { phase: 'settling' }, expected: 'Loading' }
  ] as const)('preserves the earlier routing boundary with a composer picker: %j', ({ options, expected }) => {
    const html = renderHero({ ...options, hasComposerWorkspacePicker: true })
    expect(html).toContain(expected)
    expect(html).not.toContain('dsh-conversation-hero-mark')
    if ('route' in options && options.route === 'claw') {
      expect(html).toMatch(/<button[^>]*>Choose working directory<\/button>/)
    }
  })

  it('keeps the brand-mark default lower-specificity than explicit empty-state sizing', async () => {
    const css = await readBaseShellSource()
    const baseRule = css.match(/\.ax-brand-mark\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(baseRule).not.toContain('width: 100%')
    expect(baseRule).not.toContain('height: 100%')
    expect(css).toMatch(/:where\(\.ax-brand-mark\)\s*\{[^}]*width:\s*1em;[^}]*height:\s*1em;/s)
  })

  it('shows the runtime error in the offline hero when one is available', () => {
    const html = renderHero({
      ready: false,
      runtimeError: i18n.t('common:runtimePortConflict')
    })

    expect(html).toContain('The runtime port is already in use.')
  })
})
