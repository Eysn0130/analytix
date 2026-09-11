export type AttachmentUploadAvailabilityInput = {
  runtimeConnection: string
  route: string
  mode: 'plan' | 'agent'
  attachmentStoreAvailable?: boolean
  modelSupportsImageInput?: boolean
}

export function isChatAttachmentUploadEnabled(input: AttachmentUploadAvailabilityInput): boolean {
  return (
    input.runtimeConnection === 'ready' &&
    (input.route === 'chat' || input.route === 'write') &&
    (input.mode === 'agent' || input.mode === 'plan')
  )
}

export type ImageAttachmentUploadAvailabilityInput = {
  attachmentUploadEnabled: boolean
  modelSupportsImageInput?: boolean
  visionBridgeAvailable?: boolean
}

export function canUploadImageAttachment(input: ImageAttachmentUploadAvailabilityInput): boolean {
  return (
    input.attachmentUploadEnabled &&
    (input.modelSupportsImageInput === true || input.visionBridgeAvailable === true)
  )
}
