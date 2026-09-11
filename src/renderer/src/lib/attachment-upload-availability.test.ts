import { describe, expect, it } from 'vitest'
import { canUploadImageAttachment, isChatAttachmentUploadEnabled } from './attachment-upload-availability'

describe('isChatAttachmentUploadEnabled', () => {
  it('enables composer attachments in chat when the runtime is ready', () => {
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'ready',
      route: 'chat',
      mode: 'agent',
      attachmentStoreAvailable: true,
      modelSupportsImageInput: true
    })).toBe(true)
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'ready',
      route: 'chat',
      mode: 'plan',
      attachmentStoreAvailable: true,
      modelSupportsImageInput: true
    })).toBe(true)
  })

  it('enables composer attachments in Write mode assistants', () => {
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'ready',
      route: 'write',
      mode: 'agent',
      attachmentStoreAvailable: true,
      modelSupportsImageInput: true
    })).toBe(true)
  })

  it('disables composer attachments outside ready supported modes', () => {
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'connecting',
      route: 'chat',
      mode: 'agent',
      attachmentStoreAvailable: true,
      modelSupportsImageInput: true
    })).toBe(false)
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'ready',
      route: 'settings',
      mode: 'agent',
      attachmentStoreAvailable: true,
      modelSupportsImageInput: true
    })).toBe(false)
  })

  it('keeps the attachment picker reachable for non-image documents', () => {
    expect(isChatAttachmentUploadEnabled({
      runtimeConnection: 'ready',
      route: 'chat',
      mode: 'agent',
      attachmentStoreAvailable: false,
      modelSupportsImageInput: false
    })).toBe(true)
  })

  it('allows image uploads for native vision models or available Vision Bridge', () => {
    expect(canUploadImageAttachment({
      attachmentUploadEnabled: true,
      modelSupportsImageInput: true,
      visionBridgeAvailable: false
    })).toBe(true)
    expect(canUploadImageAttachment({
      attachmentUploadEnabled: true,
      modelSupportsImageInput: false,
      visionBridgeAvailable: true
    })).toBe(true)
  })

  it('blocks image uploads for text-only models without Vision Bridge', () => {
    expect(canUploadImageAttachment({
      attachmentUploadEnabled: true,
      modelSupportsImageInput: false,
      visionBridgeAvailable: false
    })).toBe(false)
    expect(canUploadImageAttachment({
      attachmentUploadEnabled: false,
      modelSupportsImageInput: true,
      visionBridgeAvailable: true
    })).toBe(false)
  })
})
