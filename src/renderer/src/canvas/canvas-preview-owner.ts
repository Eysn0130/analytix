import { createCanvasPreviewSession } from './canvas-preview-session'

// The renderer owns only two presentation handles. Core owns permission,
// retained review intent, receipts and shutdown. Shared with actual tab close.
export const canvasPreviewSessions = createCanvasPreviewSession(request => window.analytix.canvas.request(request))
