import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'fs'
import { readFile } from 'fs/promises'
import { homedir } from 'os'
import { extname, join, posix, relative, resolve } from 'path'
import { pathToFileURL } from 'node:url'
import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import react from '@vitejs/plugin-react'
import type { Plugin } from 'vite'

const browserRuntimePort =
  process.env.ANALYTIX_BROWSER_RUNTIME_PORT ??
  process.env.VITE_ANALYTIX_RUNTIME_PORT ??
  '8899'
process.env.VITE_ANALYTIX_RUNTIME_PORT ??= browserRuntimePort
process.env.VITE_ANALYTIX_BROWSER_WORKSPACE_ROOT ??=
  process.env.ANALYTIX_BROWSER_WORKSPACE_ROOT ?? process.cwd()
const SETTINGS_FILE_NAME = 'analytix-settings.json'
// Ordinary isolated development must not auto-load an Owner's repository .env
// into a browser bundle. Legacy explicit test-account runs keep their routing.
const developmentEnvDir = process.env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE === 'isolated-local-v1'
  ? process.env.ANALYTIX_USER_DATA_DIR
  : undefined
const FLOW_RUNTIME_ASSET_ROOT = resolve('src/renderer/src/data-analysis/features/flow/runtime/embedded')
const FLOW_RUNTIME_ROUTE_PREFIX = '/flow-runtime/'

function toPosixPath(value: string): string {
  return value.replace(/\\/g, '/')
}

function officeCodecBuildPlugin(): Plugin {
  return {
    name: 'analytix-office-codec-build',
    async buildStart() {
      // Keep the data-only subprocess separate from Main's external dependencies
      // and shared chunks. This hook also covers direct electron-vite dev runs.
      const script = pathToFileURL(resolve('scripts/build-office-codec.mjs')).href
      const { buildOfficeCodec } = await import(/* @vite-ignore */ script)
      const output: { watchFiles: string[] } = await buildOfficeCodec()
      this.addWatchFile(resolve('scripts/build-office-codec.mjs'))
      for (const file of output.watchFiles) this.addWatchFile(file)
    }
  }
}

function buildGraphModuleId(moduleId: string): string | null {
  if (!moduleId || moduleId.startsWith('\0')) return null
  const clean = moduleId.split('?')[0]
  const relativePath = toPosixPath(relative(process.cwd(), clean))
  if (!relativePath || relativePath.startsWith('../')) return null
  return relativePath
}

function hubColdBuildGraphPlugin(target: 'main' | 'renderer'): Plugin {
  return {
    name: `analytix-hub-cold-build-graph-${target}`,
    generateBundle(_options, bundle) {
      const graphDir = process.env.ANALYTIX_HUB_COLD_GRAPH_DIR?.trim()
      if (!graphDir) return

      const internalModuleIds = new Set<string>()
      for (const output of Object.values(bundle)) {
        if (output.type !== 'chunk') continue
        for (const moduleId of Object.keys(output.modules)) {
          if (buildGraphModuleId(moduleId)) internalModuleIds.add(moduleId)
        }
      }

      const modules: Record<string, { staticImports: string[]; dynamicImports: string[] }> = {}
      for (const moduleId of internalModuleIds) {
        const normalized = buildGraphModuleId(moduleId)
        if (!normalized) continue
        const info = this.getModuleInfo(moduleId)
        modules[normalized] = {
          staticImports: (info?.importedIds ?? [])
            .map(buildGraphModuleId)
            .filter((value): value is string => Boolean(value))
            .sort(),
          dynamicImports: (info?.dynamicallyImportedIds ?? [])
            .map(buildGraphModuleId)
            .filter((value): value is string => Boolean(value))
            .sort()
        }
      }

      const chunks = Object.fromEntries(Object.values(bundle)
        .filter((output) => output.type === 'chunk')
        .map((output) => [output.fileName, {
          modules: Object.keys(output.modules)
            .map(buildGraphModuleId)
            .filter((value): value is string => Boolean(value))
            .sort(),
          imports: [...output.imports].sort(),
          dynamicImports: [...output.dynamicImports].sort()
        }]))

      mkdirSync(graphDir, { recursive: true })
      writeFileSync(join(graphDir, `${target}.json`), JSON.stringify({
        schemaVersion: 1,
        target,
        modules,
        chunks
      }))
    }
  }
}

function listFlowRuntimeFiles(root: string): Array<{ absolutePath: string; routePath: string; fileName: string }> {
  const result: Array<{ absolutePath: string; routePath: string; fileName: string }> = []

  const visit = (dir: string): void => {
    readdirSync(dir, { withFileTypes: true }).forEach((entry) => {
      const absolutePath = join(dir, entry.name)
      if (entry.isDirectory()) {
        visit(absolutePath)
        return
      }
      const relativePath = toPosixPath(relative(root, absolutePath))
      result.push({
        absolutePath,
        routePath: `${FLOW_RUNTIME_ROUTE_PREFIX}${relativePath}`,
        fileName: posix.join('flow-runtime', relativePath)
      })
    })
  }

  visit(root)
  return result.sort((left, right) => left.routePath.localeCompare(right.routePath))
}

function contentTypeFor(fileName: string): string {
  switch (extname(fileName).toLowerCase()) {
    case '.css':
      return 'text/css; charset=utf-8'
    case '.html':
      return 'text/html; charset=utf-8'
    case '.js':
      return 'application/javascript; charset=utf-8'
    case '.json':
      return 'application/json; charset=utf-8'
    default:
      return 'text/plain; charset=utf-8'
  }
}

function flowRuntimeAssetsPlugin(): Plugin {
  const resolveFlowRuntimeFile = (routePath: string): string | null => {
    const routeMap = new Map(
      listFlowRuntimeFiles(FLOW_RUNTIME_ASSET_ROOT).map((entry) => [entry.routePath, entry.absolutePath])
    )
    return routeMap.get(routePath) || null
  }

  return {
    name: 'analytix-flow-runtime-assets',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const rawUrl = String(req.url || '')
        const pathname = (() => {
          try {
            return decodeURIComponent(rawUrl.split('?')[0] || '')
          } catch {
            return rawUrl.split('?')[0] || ''
          }
        })()
        const filePath = resolveFlowRuntimeFile(pathname)
        if (!filePath) {
          next()
          return
        }
        res.setHeader('Content-Type', contentTypeFor(filePath))
        res.end(readFileSync(filePath))
      })
    },
    buildStart() {
      const rootStats = statSync(FLOW_RUNTIME_ASSET_ROOT)
      if (!rootStats.isDirectory()) {
        throw new Error(`flow runtime asset root missing: ${FLOW_RUNTIME_ASSET_ROOT}`)
      }
    },
    generateBundle() {
      listFlowRuntimeFiles(FLOW_RUNTIME_ASSET_ROOT).forEach((entry) => {
        this.emitFile({
          type: 'asset',
          fileName: entry.fileName,
          source: readFileSync(entry.absolutePath)
        })
      })
    }
  }
}

function desktopSettingsCandidates(): string[] {
  const explicit = process.env.ANALYTIX_BROWSER_SETTINGS_PATH?.trim()
  if (explicit) return [explicit]
  const home = homedir()
  switch (process.platform) {
    case 'darwin':
      return [
        join(home, 'Library', 'Application Support', 'Analytix', SETTINGS_FILE_NAME),
        join(home, 'Library', 'Application Support', 'analytix', SETTINGS_FILE_NAME)
      ]
    case 'win32': {
      const appData = process.env.APPDATA || join(home, 'AppData', 'Roaming')
      return [
        join(appData, 'analytix', SETTINGS_FILE_NAME),
        join(appData, 'Analytix', SETTINGS_FILE_NAME)
      ]
    }
    default: {
      const configHome = process.env.XDG_CONFIG_HOME || join(home, '.config')
      return [
        join(configHome, 'analytix', SETTINGS_FILE_NAME),
        join(configHome, 'Analytix', SETTINGS_FILE_NAME)
      ]
    }
  }
}

function redactDesktopSettings(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redactDesktopSettings)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    const normalized = key.toLowerCase()
    if (
      normalized === 'apikey' ||
      normalized === 'appsecret' ||
      normalized === 'secret' ||
      normalized === 'sessionkey' ||
      normalized === 'bottoken' ||
      normalized === 'runtimetoken'
    ) {
      out[key] = typeof child === 'string' && child.trim() ? '__configured_in_desktop__' : ''
    } else {
      out[key] = redactDesktopSettings(child)
    }
  }
  return out
}

async function readDesktopSettingsObject(): Promise<Record<string, unknown> | null> {
  for (const candidate of desktopSettingsCandidates()) {
    if (!existsSync(candidate)) continue
    const raw = await readFile(candidate, 'utf8')
    return JSON.parse(raw) as Record<string, unknown>
  }
  return null
}

async function readDesktopSettingsJson(): Promise<string | null> {
  const parsed = await readDesktopSettingsObject()
  return parsed ? JSON.stringify(redactDesktopSettings(parsed)) : null
}

function runtimePortFromDesktopSettings(settings: Record<string, unknown> | null): string {
  const runtime = settings?.runtime
  const port = runtime && typeof runtime === 'object'
    ? Number((runtime as Record<string, unknown>).port)
    : Number.NaN
  return Number.isFinite(port) && port > 0 ? String(Math.round(port)) : browserRuntimePort
}

function appendRequestHeaders(target: Headers, rawHeaders: Record<string, string | string[] | undefined>): void {
  for (const [key, raw] of Object.entries(rawHeaders)) {
    if (raw === undefined) continue
    const normalized = key.toLowerCase()
    if (normalized === 'host' || normalized === 'connection' || normalized === 'content-length') continue
    if (Array.isArray(raw)) {
      for (const value of raw) target.append(key, value)
    } else {
      target.set(key, raw)
    }
  }
}

async function readRequestBody(req: NodeJS.ReadableStream): Promise<Buffer | undefined> {
  const chunks: Buffer[] = []
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk)))
  }
  return chunks.length > 0 ? Buffer.concat(chunks) : undefined
}

async function writeFetchResponse(res: NodeJS.WritableStream & {
  statusCode?: number
  setHeader?: (name: string, value: string) => void
  end?: (chunk?: string | Buffer) => void
}, upstream: Response): Promise<void> {
  res.statusCode = upstream.status
  upstream.headers.forEach((value, key) => {
    res.setHeader?.(key, value)
  })
  if (!upstream.body) {
    res.end?.()
    return
  }
  const reader = upstream.body.getReader()
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (value) res.write(Buffer.from(value))
    }
  } finally {
    res.end?.()
    reader.releaseLock()
  }
}

export default defineConfig({
  main: {
    envDir: developmentEnvDir,
    plugins: [externalizeDepsPlugin(), hubColdBuildGraphPlugin('main'), officeCodecBuildPlugin()],
    build: {
      rollupOptions: {
        input: {
          index: resolve('src/main/index.ts'),
          'claw-schedule-mcp-node-entry': resolve('src/main/claw-schedule-mcp-node-entry.ts')
        }
      }
    }
  },
  preload: {
    envDir: developmentEnvDir,
    // Sandboxed preloads can only require Electron and a small Node subset.
    // Bundle the runtime validators instead of leaving an unavailable
    // `require("zod")` in the generated preload.
    plugins: [externalizeDepsPlugin({ exclude: ['zod'] })],
    build: {
      rollupOptions: {
        input: { index: resolve('src/preload/index.ts'), office: resolve('src/main/office/office-preload.ts') },
        output: {
          format: 'cjs',
          entryFileNames: '[name].cjs'
        }
      }
    }
  },
  renderer: {
    envDir: developmentEnvDir,
    optimizeDeps: {
      force: true
    },
    resolve: {
      alias: {
        '@renderer': resolve('src/renderer/src'),
        '@shared': resolve('src/shared')
      }
    },
    plugins: [
      react(),
      flowRuntimeAssetsPlugin(),
      hubColdBuildGraphPlugin('renderer'),
      {
        name: 'analytix-browser-preview-settings',
        configureServer(server) {
          server.middlewares.use('/__analytix-runtime', (req, res) => {
            void (async () => {
              try {
                const settings = await readDesktopSettingsObject()
                const explicit = process.env.ANALYTIX_BROWSER_RUNTIME_URL?.trim()
                const target = explicit || `http://127.0.0.1:${runtimePortFromDesktopSettings(settings)}`
                const path = req.url?.startsWith('/') ? req.url : `/${req.url ?? ''}`
                const url = `${target}${path}`
                const method = req.method ?? 'GET'
                const headers = new Headers()
                appendRequestHeaders(headers, req.headers)
                const rawBody = method === 'GET' || method === 'HEAD' ? undefined : await readRequestBody(req)
                const body = rawBody ? new Uint8Array(rawBody) : undefined
                const upstream = await fetch(url, { method, headers, body })
                await writeFetchResponse(res, upstream)
              } catch (error) {
                res.statusCode = 502
                res.setHeader('content-type', 'application/json; charset=utf-8')
                res.end(JSON.stringify({
                  code: 'browser_preview_proxy_failed',
                  message: error instanceof Error ? error.message : String(error)
                }))
              }
            })()
          })
          server.middlewares.use('/__analytix-desktop/settings', (req, res) => {
            if (req.method !== 'GET') {
              res.statusCode = 405
              res.setHeader('content-type', 'application/json; charset=utf-8')
              res.end(JSON.stringify({ ok: false, message: 'Method not allowed' }))
              return
            }
            void readDesktopSettingsJson()
              .then((body) => {
                if (!body) {
                  res.statusCode = 404
                  res.setHeader('content-type', 'application/json; charset=utf-8')
                  res.end(JSON.stringify({ ok: false, message: 'Desktop settings not found' }))
                  return
                }
                res.statusCode = 200
                res.setHeader('content-type', 'application/json; charset=utf-8')
                res.end(body)
              })
              .catch((error) => {
                res.statusCode = 500
                res.setHeader('content-type', 'application/json; charset=utf-8')
                res.end(JSON.stringify({
                  ok: false,
                  message: error instanceof Error ? error.message : String(error)
                }))
              })
          })
        }
      }
    ]
  }
})
