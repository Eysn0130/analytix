import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { resolveUiPluginFigure } from '@shared/ui-plugin'
import { useUiPluginStore } from '../../store/ui-plugin-store'
import {
  AnimatedWorkLogo,
  MASCOT_CAMEO_DURATIONS_MS,
  MASCOT_CAMEO_TYPES,
  MascotCameo,
  MASCOT_WORK_LOGO_VARIANTS,
  MASCOT_WORK_LOGO_VARIANT_LABEL_KEYS,
  CAMEO_CELEBRATION_DURATIONS_MS,
  CAMEO_CELEBRATION_VARIANTS,
  CameoCelebration,
  CameoStateFigure,
  SidebarMascot,
  UI_PLUGIN_CAMEO_SLOTS,
  UI_PLUGIN_CELEBRATION_SLOTS,
  UI_PLUGIN_STATE_SLOTS,
  WORK_LOGO_SWIM_MODES,
  WORK_LOGO_SWIM_MODE_LABEL_KEYS,
  pickMascotCameo,
  pickCameoCelebration
} from './AnimatedWorkLogo'
import { WorkMetaRow } from './message-timeline-cards'

describe('AnimatedWorkLogo', () => {
  it('ships the Xiezhi asset used as the default work mark', async () => {
    const nodeFs = 'node:fs/promises'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const xiezhiFigure = await readFile(new URL('../../../../asset/img/xiezhi_profile.png', import.meta.url))

    expect(pngDimensions(xiezhiFigure)).toEqual({ width: 512, height: 512 })
  })

  it('renders layered logo markup for swim animation', () => {
    const html = renderToStaticMarkup(
      createElement(AnimatedWorkLogo, { active: true, className: 'extra-class', size: 'md' })
    )

    expect(html).toContain('ds-work-logo')
    expect(html).toContain('ds-work-logo-md')
    expect(html).toContain('ds-work-logo-phase-lead')
    expect(html).toContain('is-active')
    expect(html).toContain('extra-class')
    expect(html).toContain('ds-work-logo-gust')
    expect(html).toContain('ds-work-logo-current')
    expect(html).toContain('ds-work-logo-swell')
    expect(html).toContain('ds-work-logo-wave-back')
    expect(html).toContain('ds-work-logo-ripple')
    expect(html).toContain('ds-work-logo-wave-front')
    expect(html).toContain('ds-work-logo-breaker')
    expect(html).toContain('ds-work-logo-wake')
    expect(html).toContain('ds-work-logo-foam')
    expect(html).toContain('ds-work-logo-crest')
    expect(html).toContain('ds-work-logo-splash')
    expect(html).toContain('ds-work-logo-spray')
    expect(html).toContain('ds-work-logo-bubbles')
    expect(html).toContain('ds-work-logo-echo')
    expect(html).toContain('ds-work-logo-track')
    expect(html).toContain('ds-work-logo-body')
    expect(html).toContain('ds-work-logo-image')
    expect(html).toContain('ds-mascot-logo')
    expect(html).toContain('ds-mascot-figure')
    expect(html).toMatch(/ds-mascot-logo-(dribble|run|boba)/)
    expect(html).toMatch(/ds-work-logo-mode-(propel|sprint|dive|surf)/)
  })

  it('renders the state figures with their kind classes', () => {
    for (const kind of ['greet', 'sleep', 'sit'] as const) {
      const html = renderToStaticMarkup(createElement(CameoStateFigure, { kind }))
      expect(html).toContain(`ds-cameo-state-${kind}`)
      expect(html).toContain('ds-cameo-state-figure')
      expect(html).toContain('ds-mascot-state-figure')
    }
  })

  describe('UI plugin slot fallback chains', () => {
    const figures = {
      swim: 'data:image/png;base64,SWIM',
      greet: 'data:image/png;base64,GREET'
    }

    it('every surface chain ends at swim so partial skins always resolve', () => {
      const chains = [
        ...Object.values(UI_PLUGIN_STATE_SLOTS),
        ...Object.values(UI_PLUGIN_CAMEO_SLOTS),
        ...Object.values(UI_PLUGIN_CELEBRATION_SLOTS)
      ]
      for (const chain of chains) {
        expect(chain[chain.length - 1]).toBe('swim')
      }
    })

    it('resolves missing slots through the chains', () => {
      expect(resolveUiPluginFigure(figures, UI_PLUGIN_STATE_SLOTS.greet)).toBe(figures.greet)
      // sleep 槽位缺失 → 回退链最终落到 swim
      expect(resolveUiPluginFigure(figures, UI_PLUGIN_STATE_SLOTS.sleep)).toBe(figures.swim)
      expect(resolveUiPluginFigure(figures, UI_PLUGIN_CAMEO_SLOTS.dash)).toBe(figures.swim)
      expect(resolveUiPluginFigure(figures, UI_PLUGIN_CELEBRATION_SLOTS.cheer)).toBe(figures.greet)
      expect(resolveUiPluginFigure(null, UI_PLUGIN_STATE_SLOTS.greet)).toBeNull()
    })

    it('keeps default art when no plugin is active', () => {
      expect(useUiPluginStore.getState().activeRuntime).toBeNull()
      const html = renderToStaticMarkup(createElement(CameoStateFigure, { kind: 'greet' }))
      expect(html).not.toContain('data:image')
      expect(html).toContain('ds-cameo-state-figure')
    })
  })

  it('pins the swim mode when one is provided', () => {
    const html = renderToStaticMarkup(
      createElement(AnimatedWorkLogo, { active: true, mode: 'dive' })
    )

    expect(html).toContain('ds-work-logo-mode-dive')
  })

  it('pins the mascot variant when one is provided', () => {
    const html = renderToStaticMarkup(
      createElement(AnimatedWorkLogo, { active: true, mascotVariant: 'boba' })
    )

    expect(html).toContain('ds-mascot-logo-boba')
  })

  it('renders the sidebar mascot with a state figure', () => {
    const html = renderToStaticMarkup(createElement(SidebarMascot))

    expect(html).toContain('ds-sidebar-mascot')
    expect(html).toMatch(/ds-cameo-state-(sit|greet|sleep)/)
  })

  it('renders every mascot cameo type with side classes', () => {
    for (const type of ['dash', 'peek', 'boba', 'nap'] as const) {
      const html = renderToStaticMarkup(createElement(MascotCameo, { cameo: { type, side: 'left' } }))
      expect(html).toContain(`ds-mascot-cameo-${type}`)
      expect(html).toContain('is-left')
      expect(html).toContain('ds-mascot-cameo-figure')
    }

    const chaseHtml = renderToStaticMarkup(
      createElement(MascotCameo, { cameo: { type: 'chase', side: 'right' } })
    )
    expect(chaseHtml.match(/ds-mascot-cameo-dash/g)?.length).toBe(2)
    expect(chaseHtml).toContain('is-second')
  })

  it('renders every celebration variant with dual figures and confetti', () => {
    for (const variant of CAMEO_CELEBRATION_VARIANTS) {
      const html = renderToStaticMarkup(createElement(CameoCelebration, { variant }))
      expect(html).toContain(`ds-cameo-celebration-${variant}`)
      expect(html).toContain('is-cameo')
      expect(html).toContain('is-mascot')
      expect(html).toContain('ds-cameo-confetti')
      expect(html.match(/<i><\/i>/g)?.length).toBe(10)
      expect(CAMEO_CELEBRATION_DURATIONS_MS[variant]).toBeGreaterThan(0)
    }
  })

  it('picks valid celebration variants with increasing ids', () => {
    const first = pickCameoCelebration()
    const second = pickCameoCelebration()

    expect(CAMEO_CELEBRATION_VARIANTS).toContain(first.variant)
    expect(second.id).toBeGreaterThan(first.id)
  })

  it('picks valid cameo specs with increasing ids and complete durations', () => {
    const first = pickMascotCameo()
    const second = pickMascotCameo()

    expect(MASCOT_CAMEO_TYPES).toContain(first.type)
    expect(['left', 'right']).toContain(first.side)
    expect(second.id).toBeGreaterThan(first.id)

    for (const type of MASCOT_CAMEO_TYPES) {
      expect(MASCOT_CAMEO_DURATIONS_MS[type]).toBeGreaterThan(0)
    }
  })

  it('maps every swim mode and mascot variant to a status label key in both locales', async () => {
    const nodeFs = 'node:fs/promises'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const zh = JSON.parse(await readFile(new URL('../../locales/zh/common.json', import.meta.url), 'utf8'))
    const en = JSON.parse(await readFile(new URL('../../locales/en/common.json', import.meta.url), 'utf8'))

    for (const swimMode of WORK_LOGO_SWIM_MODES) {
      const labelKey = WORK_LOGO_SWIM_MODE_LABEL_KEYS[swimMode]
      expect(labelKey).toBeTruthy()
      expect(zh[labelKey]).toBeTruthy()
      expect(en[labelKey]).toBeTruthy()
    }

    for (const variant of MASCOT_WORK_LOGO_VARIANTS) {
      const labelKey = MASCOT_WORK_LOGO_VARIANT_LABEL_KEYS[variant]
      expect(labelKey).toBeTruthy()
      expect(zh[labelKey]).toBeTruthy()
      expect(en[labelKey]).toBeTruthy()
    }
  })

  it('defaults to a static logo unless active', () => {
    const html = renderToStaticMarkup(createElement(AnimatedWorkLogo))

    expect(html).toContain('ds-work-logo')
    expect(html).toContain('ds-work-logo-phase-lead')
    expect(html).not.toContain('is-active')
  })

  it('keeps wave and splash layers mounted in static state to avoid layout churn', () => {
    const html = renderToStaticMarkup(createElement(AnimatedWorkLogo, { size: 'sm' }))

    expect(html).toContain('ds-work-logo-sm')
    expect(html).toContain('ds-work-logo-gust')
    expect(html).toContain('ds-work-logo-swell')
    expect(html).toContain('ds-work-logo-wave-back')
    expect(html).toContain('ds-work-logo-wave-front')
    expect(html).toContain('ds-work-logo-breaker')
    expect(html).toContain('ds-work-logo-foam')
    expect(html).toContain('ds-work-logo-crest')
    expect(html).toContain('ds-work-logo-splash')
    expect(html).toContain('ds-work-logo-spray')
    expect(html).not.toContain('is-active')
  })

  it('can render a desynchronized trailing phase', () => {
    const html = renderToStaticMarkup(createElement(AnimatedWorkLogo, { active: true, phase: 'trail' }))

    expect(html).toContain('is-active')
    expect(html).toContain('ds-work-logo-phase-trail')
  })

  it('keeps the processing work row as text-only status', () => {
    const html = renderToStaticMarkup(
      createElement(WorkMetaRow, {
        processing: true,
        stepCount: 3,
        expanded: true,
        onToggle: () => undefined
      })
    )

    expect(html).toContain('ds-shiny-text')
    expect(html).not.toContain('ds-work-logo-slot')
  })

  it('keeps the swim animation layers wired in CSS', async () => {
    const nodeFs = 'node:fs/promises'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const baseShellCss = await readFile(new URL('../../styles/base-shell.css', import.meta.url), 'utf8')

    for (const layer of [
      'gust',
      'swell',
      'wave-front',
      'breaker',
      'wake',
      'foam',
      'waterline',
      'crest',
      'splash',
      'spray',
      'bubbles'
    ]) {
      expect(baseShellCss).toContain(`ds-work-logo-${layer}`)
    }

    expect(baseShellCss).toContain('.ds-work-logo.is-active .ds-work-logo-body::after')
    expect(baseShellCss).toContain('@keyframes ds-work-logo-waterline')
    expect(baseShellCss).not.toContain('ds-work-logo-tail')
    expect(baseShellCss).not.toContain('transform: translateZ(0) scaleX(-1)')
    expect(baseShellCss).toContain('.ds-work-logo.ds-work-logo-mode-sprint')
    expect(baseShellCss).toContain('.ds-work-logo.ds-work-logo-mode-dive')
    expect(baseShellCss).toContain('.ds-work-logo.ds-work-logo-mode-surf')
    expect(baseShellCss).toContain('@keyframes ds-work-logo-sprint-path')
    expect(baseShellCss).toContain('@keyframes ds-work-logo-dive-path')
    expect(baseShellCss).toContain('@keyframes ds-work-logo-dive-figure')
    expect(baseShellCss).toContain('@keyframes ds-work-logo-surf-path')
    expect(baseShellCss).toContain('@keyframes ds-cameo-greet-wave')
    expect(baseShellCss).toContain('@keyframes ds-cameo-sleep-breathe')
    expect(baseShellCss).toContain('@keyframes ds-cameo-sit-sway')
    expect(baseShellCss).toContain('.ds-work-logo:hover')
    expect(baseShellCss).toContain('.ds-cameo-state:hover')
    expect(baseShellCss).toContain("[data-mascot-mode='on'] .ds-work-logo .ds-mascot-logo")
    expect(baseShellCss).toContain("[data-mascot-mode='on'] {")
    expect(baseShellCss).toContain("[data-theme='dark'][data-mascot-mode='on'],")
    expect(baseShellCss).toContain("[data-theme='dark'][data-mascot-mode='on'] .ds-workbench-shell")
    expect(baseShellCss).toContain('@keyframes ds-mascot-dribble')
    expect(baseShellCss).toContain('@keyframes ds-mascot-run')
    expect(baseShellCss).toContain('@keyframes ds-mascot-boba')
    expect(baseShellCss).toContain('.ds-mascot-cameo-layer')
    expect(baseShellCss).toContain('@keyframes ds-mascot-cameo-cross')
    expect(baseShellCss).toContain('@keyframes ds-mascot-cameo-peek')
    expect(baseShellCss).toContain('@keyframes ds-mascot-cameo-rise')
    expect(baseShellCss).toContain('@keyframes ds-mascot-cameo-doze')
    expect(baseShellCss).toContain('.ds-cameo-celebration-layer')
    expect(baseShellCss).toContain('@keyframes ds-cameo-celebrate-cheer')
    expect(baseShellCss).toContain('@keyframes ds-cameo-celebrate-lap')
    expect(baseShellCss).toContain('@keyframes ds-cameo-celebrate-toast')
    expect(baseShellCss).toContain('@keyframes ds-cameo-confetti-burst')
    expect(baseShellCss).toContain('@media (prefers-reduced-motion: reduce)')
    expect(baseShellCss).toContain("[data-focus-mode='on'] .ds-mascot-cameo-layer")
    expect(baseShellCss).toContain("[data-focus-mode='on'] .ds-cameo-celebration-layer")
    expect(baseShellCss).toContain("[data-focus-mode='on'] .ds-cameo-state")
    expect(baseShellCss).toContain("[data-focus-mode='on'] .ds-work-logo")
    expect(baseShellCss).toContain("[data-focus-mode='on'] .ds-work-logo-slot:has(.ds-work-logo)")
    expect(baseShellCss).toContain('display: none !important;')
    expect(baseShellCss).not.toContain("[data-focus-mode='on'] .ds-shiny-text")
    expect(baseShellCss).not.toContain("[data-focus-mode='on'] .ds-runtime-wake-shell::before")
  })

  it('keeps generated Analytix PNG icon dimensions stable for packaging', async () => {
    const nodeFs = 'node:fs/promises'
    const nodeZlib = 'node:zlib'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const { inflateSync } = await import(/* @vite-ignore */ nodeZlib)
    const appIcon = await readFile(new URL('../../../../asset/brand/analytix-app-icon-512.png', import.meta.url))
    const macIcon = await readFile(new URL('../../../../asset/brand/analytix-app-icon-1024.png', import.meta.url))
    const monoSymbol = await readFile(new URL('../../../../asset/brand/analytix-symbol-mono-black.png', import.meta.url))
    const splash = await readFile(new URL('../../../../asset/brand/analytix-splash.png', import.meta.url))

    expect(pngDimensions(appIcon)).toEqual({ width: 512, height: 512 })
    expect(pngDimensions(macIcon)).toEqual({ width: 1024, height: 1024 })
    expect(pngDimensions(monoSymbol)).toEqual({ width: 1104, height: 1052 })
    expect(pngDimensions(splash)).toEqual({ width: 1600, height: 900 })
    expect(pngColorType(appIcon)).toBe(6)
    expect(pngColorType(macIcon)).toBe(6)
    expect(pngCornerAlphas(appIcon, inflateSync)).toEqual([0, 0, 0, 0])
    expect(pngCornerAlphas(macIcon, inflateSync)).toEqual([0, 0, 0, 0])
    const appIconBounds = pngAlphaBounds(appIcon, inflateSync)
    const macIconBounds = pngAlphaBounds(macIcon, inflateSync)
    expect(appIconBounds.visibleWidthRatio).toBeCloseTo(0.826, 3)
    expect(appIconBounds.visibleHeightRatio).toBeCloseTo(0.859, 3)
    expect(macIconBounds.visibleWidthRatio).toBeCloseTo(0.826, 3)
    expect(macIconBounds.visibleHeightRatio).toBeCloseTo(0.859, 3)
  })

  it('records Analytix brand asset provenance', async () => {
    const nodeFs = 'node:fs/promises'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const raw = await readFile(new URL('../../../../asset/brand/provenance.json', import.meta.url), 'utf8')
    const manifest = JSON.parse(raw) as {
      brand: string
      primarySourceDirectory: string
      assets: Array<{ target: string }>
    }

    expect(manifest.brand).toBe('analytix')
    expect(manifest.primarySourceDirectory).toBe('/Users/sun/Downloads/analytix-logo')
    expect(manifest.assets.map((asset) => asset.target)).toEqual(expect.arrayContaining([
      'src/asset/brand/analytix-app-icon-512.png',
      'build/icon.icns',
      'build/icon.ico'
    ]))
  })

  it('keeps the generated Windows ICO aligned with the Analytix app icon', async () => {
    const nodeFs = 'node:fs/promises'
    const nodeZlib = 'node:zlib'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const { inflateSync } = await import(/* @vite-ignore */ nodeZlib)
    const windowsIcon = await readFile(new URL('../../../../../build/icon.ico', import.meta.url))
    const entries = icoEntries(windowsIcon)

    expect(entries.map((entry) => entry.size)).toEqual([16, 24, 32, 48, 64, 72, 96, 128, 256])
    for (const entry of entries) {
      expect(entry.bitCount).toBe(32)
      expect(pngColorType(entry.image)).toBe(6)
      expect(pngCornerAlphas(entry.image, inflateSync)).toEqual([0, 0, 0, 0])
      const bounds = pngAlphaBounds(entry.image, inflateSync)
      expect(bounds.visibleWidthRatio).toBeGreaterThanOrEqual(0.82)
      expect(bounds.visibleHeightRatio).toBeGreaterThanOrEqual(0.85)
    }
  })

  it('ships the cameo state figure assets', async () => {
    const nodeFs = 'node:fs/promises'
    const { readFile } = await import(/* @vite-ignore */ nodeFs)
    const expected: Record<string, { width: number; height: number }> = {
      cameo_greet: { width: 512, height: 460 },
      cameo_sleep: { width: 512, height: 390 },
      cameo_surf: { width: 512, height: 479 },
      cameo_sit: { width: 512, height: 493 }
    }

    for (const [name, dimensions] of Object.entries(expected)) {
      const figure = await readFile(new URL(`../../../../asset/img/${name}.png`, import.meta.url))
      expect(pngDimensions(figure)).toEqual(dimensions)
    }
  })

})

function pngDimensions(buffer: Uint8Array): { width: number; height: number } {
  const signature = [...buffer.slice(0, 8)].map((byte) => byte.toString(16).padStart(2, '0')).join('')
  expect(signature).toBe('89504e470d0a1a0a')
  return {
    width: readUint32BE(buffer, 16),
    height: readUint32BE(buffer, 20)
  }
}

function pngColorType(buffer: Uint8Array): number {
  expect(readAscii(buffer, 12, 16)).toBe('IHDR')
  return buffer[25]
}

function pngCornerAlphas(
  buffer: Uint8Array,
  inflateSync: (input: Uint8Array) => Uint8Array
): number[] {
  const { width, height, pixels } = decodePngRgba(buffer, inflateSync)
  return [
    rgbaAlphaAt(pixels, width, 0, 0),
    rgbaAlphaAt(pixels, width, width - 1, 0),
    rgbaAlphaAt(pixels, width, 0, height - 1),
    rgbaAlphaAt(pixels, width, width - 1, height - 1)
  ]
}

function pngAlphaBounds(
  buffer: Uint8Array,
  inflateSync: (input: Uint8Array) => Uint8Array
): { visibleWidthRatio: number; visibleHeightRatio: number } {
  const { width, height, pixels } = decodePngRgba(buffer, inflateSync)
  let minX = width
  let minY = height
  let maxX = -1
  let maxY = -1
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      if (rgbaAlphaAt(pixels, width, x, y) === 0) continue
      minX = Math.min(minX, x)
      minY = Math.min(minY, y)
      maxX = Math.max(maxX, x)
      maxY = Math.max(maxY, y)
    }
  }
  expect(maxX).toBeGreaterThanOrEqual(0)
  return {
    visibleWidthRatio: (maxX - minX + 1) / width,
    visibleHeightRatio: (maxY - minY + 1) / height
  }
}

function decodePngRgba(
  buffer: Uint8Array,
  inflateSync: (input: Uint8Array) => Uint8Array
): { width: number; height: number; pixels: Uint8Array } {
  const width = readUint32BE(buffer, 16)
  const height = readUint32BE(buffer, 20)
  const bitDepth = buffer[24]
  const colorType = pngColorType(buffer)
  expect(bitDepth).toBe(8)
  expect(colorType).toBe(6)

  const idatChunks: Uint8Array[] = []
  let offset = 8
  while (offset < buffer.length) {
    const length = readUint32BE(buffer, offset)
    const type = readAscii(buffer, offset + 4, offset + 8)
    const dataStart = offset + 8
    const dataEnd = dataStart + length
    if (type === 'IDAT') idatChunks.push(buffer.slice(dataStart, dataEnd))
    if (type === 'IEND') break
    offset = dataEnd + 4
  }

  const compressedLength = idatChunks.reduce((sum, chunk) => sum + chunk.length, 0)
  const compressed = new Uint8Array(compressedLength)
  let writeOffset = 0
  for (const chunk of idatChunks) {
    compressed.set(chunk, writeOffset)
    writeOffset += chunk.length
  }

  const raw = inflateSync(compressed)
  const bytesPerPixel = 4
  const stride = width * bytesPerPixel
  const pixels = new Uint8Array(width * height * bytesPerPixel)
  let rawOffset = 0
  for (let y = 0; y < height; y += 1) {
    const filter = raw[rawOffset]
    rawOffset += 1
    const row = raw.slice(rawOffset, rawOffset + stride)
    rawOffset += stride
    const previousRow = y === 0 ? undefined : pixels.slice((y - 1) * stride, y * stride)
    const decoded = unfilterPngRow(row, previousRow, filter, bytesPerPixel)
    pixels.set(decoded, y * stride)
  }
  return { width, height, pixels }
}

function unfilterPngRow(
  row: Uint8Array,
  previousRow: Uint8Array | undefined,
  filter: number,
  bytesPerPixel: number
): Uint8Array {
  const decoded = new Uint8Array(row.length)
  for (let index = 0; index < row.length; index += 1) {
    const left = index >= bytesPerPixel ? decoded[index - bytesPerPixel] : 0
    const up = previousRow?.[index] ?? 0
    const upLeft = index >= bytesPerPixel ? previousRow?.[index - bytesPerPixel] ?? 0 : 0
    let predictor = 0
    if (filter === 1) predictor = left
    if (filter === 2) predictor = up
    if (filter === 3) predictor = Math.floor((left + up) / 2)
    if (filter === 4) predictor = paeth(left, up, upLeft)
    decoded[index] = (row[index] + predictor) & 0xff
  }
  return decoded
}

function paeth(left: number, up: number, upLeft: number): number {
  const estimate = left + up - upLeft
  const leftDistance = Math.abs(estimate - left)
  const upDistance = Math.abs(estimate - up)
  const upLeftDistance = Math.abs(estimate - upLeft)
  if (leftDistance <= upDistance && leftDistance <= upLeftDistance) return left
  if (upDistance <= upLeftDistance) return up
  return upLeft
}

function rgbaAlphaAt(pixels: Uint8Array, width: number, x: number, y: number): number {
  return pixels[(y * width + x) * 4 + 3]
}

function icoEntries(buffer: Uint8Array): Array<{
  size: number
  bitCount: number
  image: Uint8Array
}> {
  expect(readUint16LE(buffer, 0)).toBe(0)
  expect(readUint16LE(buffer, 2)).toBe(1)
  const count = readUint16LE(buffer, 4)
  const entries: Array<{ size: number; bitCount: number; image: Uint8Array }> = []
  for (let index = 0; index < count; index += 1) {
    const offset = 6 + index * 16
    const width = buffer[offset] === 0 ? 256 : buffer[offset]
    const height = buffer[offset + 1] === 0 ? 256 : buffer[offset + 1]
    const bitCount = readUint16LE(buffer, offset + 6)
    const byteLength = readUint32LE(buffer, offset + 8)
    const imageOffset = readUint32LE(buffer, offset + 12)
    expect(width).toBe(height)
    entries.push({
      size: width,
      bitCount,
      image: buffer.slice(imageOffset, imageOffset + byteLength)
    })
  }
  return entries.sort((left, right) => left.size - right.size)
}

function readAscii(buffer: Uint8Array, start: number, end: number): string {
  return String.fromCharCode(...buffer.slice(start, end))
}

function readUint16LE(buffer: Uint8Array, offset: number): number {
  return buffer[offset] + buffer[offset + 1] * 256
}

function readUint32LE(buffer: Uint8Array, offset: number): number {
  return (
    buffer[offset] +
    buffer[offset + 1] * 256 +
    buffer[offset + 2] * 65_536 +
    buffer[offset + 3] * 16_777_216
  )
}

function readUint32BE(buffer: Uint8Array, offset: number): number {
  return (
    buffer[offset] * 16_777_216 +
    buffer[offset + 1] * 65_536 +
    buffer[offset + 2] * 256 +
    buffer[offset + 3]
  )
}
