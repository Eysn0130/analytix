const ASPECT_RATIOS = new Set(['1:1', '4:3', '3:4', '16:9', '9:16', '3:2', '2:3', '21:9'])
const SIZE_TIERS: Record<string, number> = { '1K': 1024, '2K': 2048 }
const SIZE_STEP = 64
const MIN_EDGE = 256

export function detectImage(buffer: Buffer): { mimeType: string; width?: number; height?: number } | null {
  if (buffer.length >= 24 && buffer[0] === 0x89 && buffer[1] === 0x50 && buffer[2] === 0x4e && buffer[3] === 0x47) {
    return { mimeType: 'image/png', width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) }
  }
  if (buffer.length >= 3 && buffer[0] === 0xff && buffer[1] === 0xd8 && buffer[2] === 0xff) {
    return { mimeType: 'image/jpeg' }
  }
  if (buffer.length >= 12 && buffer.subarray(0, 4).toString('ascii') === 'RIFF' && buffer.subarray(8, 12).toString('ascii') === 'WEBP') {
    return { mimeType: 'image/webp' }
  }
  return null
}

export function mapImageSize(
  aspectRatio: string | undefined,
  imageSize: string | undefined,
  defaultSize: string | undefined
): string | undefined {
  if (!aspectRatio && !imageSize) return defaultSize
  const tier = SIZE_TIERS[imageSize ?? ''] ?? SIZE_TIERS['1K']
  const parsed = parseRatio(aspectRatio)
  if (!parsed) return `${tier}x${tier}`
  const { w, h } = parsed
  if (w === h) return `${tier}x${tier}`
  const short = Math.max(MIN_EDGE, Math.round((tier * Math.min(w, h)) / Math.max(w, h) / SIZE_STEP) * SIZE_STEP)
  return w > h ? `${tier}x${short}` : `${short}x${tier}`
}

function parseRatio(aspectRatio: string | undefined): { w: number; h: number } | null {
  if (!aspectRatio || !ASPECT_RATIOS.has(aspectRatio)) return null
  const [w, h] = aspectRatio.split(':').map(Number)
  if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) return null
  return { w, h }
}
