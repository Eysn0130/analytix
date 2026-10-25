import { describe, expect, it } from 'vitest'
import { isWorkspaceTextPreviewPath } from './workspace-text-preview'

describe('workspace text preview path helper', () => {
  it('allows common source and config files', () => {
    expect(isWorkspaceTextPreviewPath('/tmp/project/src/App.tsx')).toBe(true)
    expect(isWorkspaceTextPreviewPath('/tmp/project/package-lock.json')).toBe(true)
    expect(isWorkspaceTextPreviewPath('/tmp/project/Dockerfile')).toBe(true)
  })

  it('rejects binary image paths', () => {
    expect(isWorkspaceTextPreviewPath('/tmp/project/logo.png')).toBe(false)
  })
})

it('keeps .htm and .html available as ordinary source previews', () => {
  expect(isWorkspaceTextPreviewPath('/workspace/page.htm')).toBe(true)
  expect(isWorkspaceTextPreviewPath('/workspace/page.HTML')).toBe(true)
})
