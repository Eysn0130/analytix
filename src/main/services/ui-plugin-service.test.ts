import { mkdir, mkdtemp, readdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  installUiPluginFromDirectory,
  listUiPlugins,
  loadUiPluginFigures,
  removeUiPlugin,
  seedUiPlugin,
  uiPluginsRootDir
} from './ui-plugin-service'

/** 1x1 transparent PNG */
const PNG_BYTES = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64'
)

let userDataDir = ''
let sourceDir = ''

async function writeSourcePlugin(manifest: unknown, figures: string[] = ['img/swim.png']): Promise<void> {
  await mkdir(join(sourceDir, 'img'), { recursive: true })
  await writeFile(join(sourceDir, 'manifest.json'), JSON.stringify(manifest), 'utf8')
  for (const figure of figures) {
    await mkdir(join(sourceDir, figure, '..'), { recursive: true })
    await writeFile(join(sourceDir, ...figure.split('/')), PNG_BYTES)
  }
}

const manifest = {
  id: 'starlight',
  name: 'Analytix Cameo',
  version: '1.0.0',
  figures: { swim: 'img/swim.png' }
}

beforeEach(async () => {
  userDataDir = await mkdtemp(join(tmpdir(), 'analytix-ui-plugin-data-'))
  sourceDir = await mkdtemp(join(tmpdir(), 'analytix-ui-plugin-src-'))
})

afterEach(async () => {
  await rm(userDataDir, { recursive: true, force: true })
  await rm(sourceDir, { recursive: true, force: true })
})

describe('installUiPluginFromDirectory', () => {
  it('installs a valid plugin by allowlist copy and lists it', async () => {
    await writeSourcePlugin(manifest)
    // 源目录里混入不该被复制的文件
    await writeFile(join(sourceDir, 'evil.js'), 'process.exit(1)', 'utf8')
    await writeFile(join(sourceDir, 'img', 'unreferenced.png'), PNG_BYTES)

    const result = await installUiPluginFromDirectory(userDataDir, sourceDir)
    expect(result.ok).toBe(true)

    const installedFiles = await readdir(join(uiPluginsRootDir(userDataDir), 'starlight'), {
      recursive: true
    })
    const flat = installedFiles.map(String).sort()
    expect(flat).toContain('manifest.json')
    expect(flat).toContain(join('img', 'swim.png'))
    expect(flat).not.toContain('evil.js')
    expect(flat.some((f) => f.includes('unreferenced'))).toBe(false)

    const plugins = await listUiPlugins(userDataDir)
    expect(plugins).toHaveLength(1)
    expect(plugins[0]?.manifest.id).toBe('starlight')
    expect(plugins[0]?.previewDataUrl?.startsWith('data:image/png;base64,')).toBe(true)
  })

  it('rejects manifests with missing figures or invalid content', async () => {
    const missingFigureSentinel = 'img/customer-pii-13900000016.png'
    await writeSourcePlugin({ ...manifest, figures: { swim: missingFigureSentinel } }, [])
    const missing = await installUiPluginFromDirectory(userDataDir, sourceDir)
    expect(missing).toEqual({ ok: false, errors: ['槽位 swim 加载失败:文件不存在'] })
    expect(JSON.stringify(missing)).not.toContain(missingFigureSentinel)

    const sentinel = '/private/customer-pii-13900000013'
    await writeFile(join(sourceDir, 'manifest.json'), sentinel, 'utf8')
    const invalid = await installUiPluginFromDirectory(userDataDir, sourceDir)
    expect(invalid).toEqual({ ok: false, errors: ['manifest.json 不是合法 JSON'] })
    expect(JSON.stringify(invalid)).not.toContain(sentinel)

    const validationSentinel = '/private/customer-pii-13900000015'
    await writeFile(
      join(sourceDir, 'manifest.json'),
      JSON.stringify({ ...manifest, figures: { [validationSentinel]: 'img/swim.png' } }),
      'utf8'
    )
    const structurallyInvalid = await installUiPluginFromDirectory(userDataDir, sourceDir)
    expect(structurallyInvalid).toEqual({ ok: false, errors: ['manifest.json 内容无效'] })
    expect(JSON.stringify(structurallyInvalid)).not.toContain(validationSentinel)
    await expect(readdir(uiPluginsRootDir(userDataDir))).rejects.toThrow()
  })

  it('projects a hostile target-filesystem failure through the install result', async () => {
    const targetRootSentinel = 'customer-pii-13900000024'
    const blockedUserDataDir = join(userDataDir, targetRootSentinel)
    await writeSourcePlugin(manifest)
    await writeFile(blockedUserDataDir, 'not a directory', 'utf8')

    const result = await installUiPluginFromDirectory(blockedUserDataDir, sourceDir)

    expect(result).toEqual({ ok: false, errors: ['插件安装失败'] })
    expect(JSON.stringify(result)).not.toContain(targetRootSentinel)
    await expect(readdir(uiPluginsRootDir(blockedUserDataDir))).rejects.toThrow()
  })
})

describe('loadUiPluginFigures', () => {
  it('returns data URLs for installed figures', async () => {
    await writeSourcePlugin(manifest)
    await installUiPluginFromDirectory(userDataDir, sourceDir)

    const result = await loadUiPluginFigures(userDataDir, 'starlight')
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.figures.swim?.startsWith('data:image/png;base64,')).toBe(true)
  })

  it('refuses ids that escape the plugins root', async () => {
    const sentinel = '../private/customer-pii-13900000014'
    const result = await loadUiPluginFigures(userDataDir, sentinel)
    expect(result).toEqual({ ok: false, error: '插件标识无效' })
    expect(JSON.stringify(result)).not.toContain(sentinel)
  })
})

describe('removeUiPlugin', () => {
  it('removes an installed plugin and refuses traversal ids', async () => {
    await writeSourcePlugin(manifest)
    await installUiPluginFromDirectory(userDataDir, sourceDir)

    expect(await removeUiPlugin(userDataDir, '../escape')).toBe(false)
    expect(await removeUiPlugin(userDataDir, 'starlight')).toBe(true)
    expect(await listUiPlugins(userDataDir)).toHaveLength(0)
  })
})

describe('seedUiPlugin (bundled compatibility plugins)', () => {
  it('seeds a plugin from in-memory bytes and it lists/loads like any other', async () => {
    const result = await seedUiPlugin(
      userDataDir,
      {
        id: 'mascot',
        name: '旧版 mascot',
        version: '1.0.0',
        figures: { swim: 'img/dribble.png', greet: 'img/wave.png' },
        features: { cameos: true }
      },
      { swim: PNG_BYTES, greet: PNG_BYTES }
    )
    expect(result.ok, JSON.stringify(result)).toBe(true)

    const plugins = await listUiPlugins(userDataDir)
    expect(plugins.map((p) => p.manifest.id)).toContain('mascot')

    const loaded = await loadUiPluginFigures(userDataDir, 'mascot')
    expect(loaded.ok).toBe(true)
    if (!loaded.ok) return
    expect(loaded.figures.swim?.startsWith('data:image/png;base64,')).toBe(true)
    expect(loaded.manifest.features?.cameos).toBe(true)
  })

  it('rejects seeding when figure bytes are missing', async () => {
    const result = await seedUiPlugin(
      userDataDir,
      { id: 'mascot', name: 'x', version: '1.0.0', figures: { swim: 'img/a.png' } },
      {}
    )
    expect(result.ok).toBe(false)
  })
})

describe('bundled starlight example', () => {
  it('installs and loads end to end', async () => {
    const exampleDir = join(process.cwd(), 'examples', 'ui-plugins', 'starlight')
    const installed = await installUiPluginFromDirectory(userDataDir, exampleDir)
    expect(installed.ok, JSON.stringify(installed)).toBe(true)

    const loaded = await loadUiPluginFigures(userDataDir, 'starlight')
    expect(loaded.ok).toBe(true)
    if (!loaded.ok) return
    expect(loaded.manifest.name).toBe('Analytix Cameo')
    expect(loaded.figures.swim?.startsWith('data:image/png;base64,')).toBe(true)
    expect(loaded.manifest.features?.cameos).toBe(true)
    expect(loaded.manifest.tokens?.light?.['--ds-accent']).toBe('#7a5fd0')
  })
})

describe('listUiPlugins', () => {
  it('skips directories whose name does not match manifest id', async () => {
    await writeSourcePlugin(manifest)
    await installUiPluginFromDirectory(userDataDir, sourceDir)
    // 手工伪造一个目录名与 id 不一致的插件
    const fakeDir = join(uiPluginsRootDir(userDataDir), 'impostor')
    await mkdir(join(fakeDir, 'img'), { recursive: true })
    await writeFile(join(fakeDir, 'manifest.json'), JSON.stringify(manifest), 'utf8')
    await writeFile(join(fakeDir, 'img', 'swim.png'), PNG_BYTES)

    const plugins = await listUiPlugins(userDataDir)
    expect(plugins.map((p) => p.manifest.id)).toEqual(['starlight'])
  })
})
