import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { WriteWorkspaceStart } from './WriteWorkspaceStart'

describe('WriteWorkspaceStart', () => {
  it('offers real document actions in the current workspace', () => {
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

    expect(html.match(/type="button"/g)).toHaveLength(4)
    expect(html).toContain('write_workspace')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('/Users/sun/.analytix/write_workspace')
  })
})
