import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { WriteWorkspaceStart } from './WriteWorkspaceStart'

describe('WriteWorkspaceStart', () => {
  it('renders the top-level write home instead of a current-space folder page', () => {
    const html = renderToStaticMarkup(
      createElement(WriteWorkspaceStart, {
        onAskAssistant: vi.fn(),
        onCreateDraft: vi.fn(),
        onPickWorkspace: vi.fn(),
        onRefreshWorkspace: vi.fn(),
        workspaceName: 'write_workspace',
        workspacePathLabel: '/Users/sun/.analytix/write_workspace'
      })
    )

    expect(html).toContain('write-empty-report-window')
    expect(html).toContain('ds-home-transition')
    expect(html).toContain('ds-home-transition-stage')
    expect(html).not.toContain('当前空间')
    expect(html).not.toContain('空间路径')
    expect(html).not.toContain('write_workspace</')
  })
})
