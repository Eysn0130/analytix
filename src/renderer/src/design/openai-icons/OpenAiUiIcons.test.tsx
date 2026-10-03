// @vitest-environment jsdom
import license from './LICENSE?raw'
import { act, createElement, createRef } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { OpenAiUiIcons } from './OpenAiUiIcons'
import provenance from './provenance.json'

describe('admitted official semantic icons', () => {
  it('retains exactly the reviewed selection, source objects and MIT notice', async () => {
    const { createHash } = await vi.importActual<{
      createHash: (algorithm: string) => { update: (value: string) => { digest: (encoding: 'hex') => string } }
    }>('node:crypto')
    const raw = import.meta.glob('./svg/*.tsx', { eager: true, query: '?raw', import: 'default' }) as Record<string, string>
    const sources = Object.fromEntries(Object.entries(raw).map(([path, source]) => [path.split('/').pop()!.replace('.tsx', ''), source]))
    const admittedNames = ["Archive", "ArrowCurvedLeft", "ArrowDown", "ArrowLeft", "ArrowRight", "ArrowRotateCcw", "ArrowRotateCw", "ArrowUp", "AtSign", "AvatarProfile", "BarChart", "Bell", "BookOpen", "Brain", "Branch", "BranchAlt", "Bug", "Cabinet", "Calendar", "Chat", "ChatCompose", "ChatTripleDots", "Check", "CheckCircle", "ChevronDown", "ChevronLeft", "ChevronRight", "ChevronUp", "CircleDashed", "ClappingBoardClosed", "Clock", "Code", "CollapseLarge", "ColorTheme", "Compare", "CompareArrows", "ComposeEditSquare", "Connect", "Copy", "CreditCard", "Cube", "Cursor", "Desktop", "Document", "DotsHorizontal", "Download", "EditPencil", "EmptyCircle", "Error", "ExclamationMarkCircle", "ExitLogout", "Expand", "ExpandLarge", "ExternalLink", "Eye", "EyeOff", "FileBlank", "FileCode", "FileDocument", "FilePresentation", "FileSpreadsheet", "Filter", "Flask", "Folder", "FolderDocumentsFinder", "FolderOpen", "FolderPlus", "GenerateSuggestedEdits", "Globe", "Grid", "HandRaised", "Identity", "ImageSquare", "InfoCircle", "Invoice", "Key", "Keyboard", "Lightbulb", "Lock", "LockKeyHole", "MicLgDictate", "Minus", "Mobile", "Moon", "Music", "Nodes", "NotebookPencil", "On", "PageBlank", "Paperclip", "PauseOutline", "PauseSm", "Pin", "PinFilled", "PlayOutline", "PlaySm", "Plugin", "PlusCircle", "PlusComposer", "PopOutWindow", "PullRequestMerged", "Quote", "QuoteReplyFilledQuoteXs", "Reload", "RobotHead", "Search", "SettingsCog", "SettingsSlider", "SettingsWrench", "Share", "ShieldCheck", "Sidebar", "SidebarCollapseRight", "SidebarOpenRightAlt", "SimpleSmile", "Sparkles", "Spelling", "Stack", "Stop", "Storage", "Sun", "Tasks", "Terminal", "Text", "Timer", "Tools", "Trash", "Unarchive", "Undo", "Upgrade", "Upscale", "User", "Users", "Video", "Voice5BarsSoundwave", "Warning", "Widget", "X", "XCircle"]
    expect(Object.keys(sources).sort()).toEqual(admittedNames)
    expect(provenance.icons.map((icon) => icon.name).sort()).toEqual(admittedNames)
    for (const icon of provenance.icons) {
      const source = sources[icon.name]
      const byteLength = new TextEncoder().encode(source).length
      expect(createHash('sha1').update('blob ' + byteLength + '\0' + source).digest('hex')).toBe(icon.sourceBlobSha)
      expect(createHash('sha256').update(source).digest('hex')).toBe(icon.sourceSha256)
      expect(icon.exactWebPathOrPixelMatch).toBe(false)
      expect(icon.consumers.length).toBeGreaterThan(0)
      expect(source).not.toMatch(/<(?:image|use|script|foreignObject)|href=/)
    }
    expect(createHash('sha256').update(license).digest('hex')).toBe('f26a46776c890b0d7b45d042cb8a78fdca9c4742a5c7fd97f0f53767411b1cc6')
    expect(provenance.exportDecisions).toHaveLength(203)
    expect(new Set(provenance.exportDecisions.map((item) => item.export)).size).toBe(203)
    expect(provenance.inlineDecisions).toHaveLength(37)
    expect(provenance.registryDecisions).toHaveLength(22)
    expect(new Set(provenance.registryDecisions.map((item) => item.key)).size).toBe(22)
    for (const item of [...provenance.exportDecisions, ...provenance.inlineDecisions, ...provenance.registryDecisions]) {
      expect(['converted', 'semantic-split', 'retained-with-reason']).toContain(item.status)
      expect(item.reason.length).toBeGreaterThan(10)
    }
  })

  it('keeps native vector painting while accepting existing size and accessibility props', () => {
    for (const Icon of Object.values(OpenAiUiIcons)) {
      const html = renderToStaticMarkup(createElement(Icon, {
        size: 18, strokeWidth: 9, fill: 'none', absoluteStrokeWidth: true,
        'aria-label': 'Existing operation', className: 'existing-icon'
      }))
      const source = provenance.icons.find((item) => Icon.displayName === `AnalytixOfficial${item.name}`)!
      expect(html).toContain(`viewBox="${source.viewBox}"`)
      expect(html).toContain('width="18"')
      expect(html).toContain('height="18"')
      expect(html).toContain('fill="currentColor"')
      expect(html).toContain('stroke="none"')
      expect(html).not.toContain('stroke-width="9"')
      expect(html).toContain('aria-label="Existing operation"')
      expect(html).toContain('class="existing-icon"')
      expect(html).toContain('style="fill:currentColor;stroke:none"')
    }
  })

  it('preserves the SVG ref, event handler and explicit dimensions', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const ref = createRef<SVGSVGElement>()
    const onClick = vi.fn()
    try {
      await act(async () => root.render(createElement(OpenAiUiIcons.search, {
        ref, onClick, size: 18, width: 14, height: 16
      })))
      expect(ref.current).toBe(container.querySelector('svg'))
      expect(ref.current?.getAttribute('width')).toBe('14')
      expect(ref.current?.getAttribute('height')).toBe('16')
      await act(async () => ref.current?.dispatchEvent(new MouseEvent('click', { bubbles: true })))
      expect(onClick).toHaveBeenCalledTimes(1)
    } finally {
      await act(async () => root.unmount())
      container.remove()
      vi.unstubAllGlobals()
    }
  })
})
