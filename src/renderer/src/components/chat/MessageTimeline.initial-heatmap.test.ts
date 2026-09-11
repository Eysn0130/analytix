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
  runtimeError?: string | null
} = {}): string {
  return renderToStaticMarkup(
    createElement(MessageTimelineEmptyHero, {
      route: options.route ?? 'chat',
      ready: options.ready ?? true,
      hasWorkspace: options.hasWorkspace ?? true,
      runtimeError: options.runtimeError ?? null,
      activeClawChannel: null,
      onPickWorkspace: () => undefined,
      onRetry: () => undefined,
      onOpenSettings: () => undefined,
      onSelectSuggestion: () => undefined
    })
  )
}

describe('MessageTimeline initial heatmap empty hero routing', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('shows the collapsed Analytix calendar for eligible initial chat states', () => {
    const html = renderHero()

    expect(html).toContain('Expand calendar')
    expect(html).toContain('ds-home-transition')
    expect(html).toContain('ds-home-transition-stage')
    expect(html).not.toContain('Daily Analytix usage calendar')
    expect(html).not.toContain('Start a new conversation')
  })

  it('keeps offline, missing-workspace, and Claw empty states gated away from the heatmap', () => {
    const offlineHtml = renderHero({ ready: false })
    expect(offlineHtml).toContain('Analytix is waking the local agent')
    expect(offlineHtml).toContain('ds-home-transition')
    expect(offlineHtml).toContain('ax-brand-mark')
    const workspaceHtml = renderHero({ hasWorkspace: false })
    expect(workspaceHtml).toContain('Choose working directory')
    expect(workspaceHtml).toContain('ds-home-transition')
    expect(workspaceHtml).toContain('ax-brand-mark')
    expect(workspaceHtml).toContain('h-16 w-16')
    const clawHtml = renderHero({ route: 'claw' })
    expect(clawHtml).toContain('Waiting for WeChat')
    expect(clawHtml).toContain('claw-phone-hero ds-home-transition')
    expect(clawHtml).toContain('ds-home-transition-stage')
    expect(clawHtml).toContain('claw-phone-hero-device')
    expect(clawHtml).toContain('ax-brand-mark')
    expect(clawHtml).not.toContain('Analytix usage')
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
