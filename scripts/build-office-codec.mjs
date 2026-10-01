import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { isBuiltin } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { parse } from 'acorn'
import { build } from 'vite'

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
export const OFFICE_CODEC_ENTRY = 'office-generation-codec-entry.js'
export const OFFICE_CODEC_DIRECTORY = 'out/office-codec'
const maximumBytes = 16 * 1024 * 1024

// Check the emitted program as well as Rollup's dependency graph. A remaining
// dynamic require must not silently find a dependency outside the shipped tree.
export function assertOfficeCodecImports(code) {
  const tree = parse(code, { ecmaVersion: 'latest', sourceType: 'script' })
  const check = value => {
    if (typeof value !== 'string' || !isBuiltin(value)) throw new Error(`office-codec-external-dependency: ${String(value).slice(0, 160)}`)
  }
  const visit = node => {
    if (!node || typeof node !== 'object') return
    if (node.type === 'CallExpression' && node.callee?.type === 'Identifier' && node.callee.name === 'require') {
      if (node.arguments.length !== 1 || node.arguments[0].type !== 'Literal') throw new Error('office-codec-dynamic-require')
      check(node.arguments[0].value)
    }
    if (node.type === 'ImportExpression') check(node.source?.type === 'Literal' ? node.source.value : undefined)
    for (const value of Object.values(node)) {
      if (Array.isArray(value)) value.forEach(visit)
      else if (value && typeof value === 'object') visit(value)
    }
  }
  visit(tree)
}

/** Reusable afterPack check; these two files are covered by the existing payload authority. */
export function validateOfficeCodecDirectory(directory) {
  const root = lstatSync(directory)
  if (!root.isDirectory() || root.isSymbolicLink()) throw new Error('office-codec-directory-invalid')
  const expected = [OFFICE_CODEC_ENTRY, 'package.json'].sort()
  if (JSON.stringify(readdirSync(directory).sort()) !== JSON.stringify(expected)) throw new Error('office-codec-files-invalid')
  const files = expected.map(name => {
    const path = join(directory, name), stat = lstatSync(path)
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size === 0 || stat.size > maximumBytes) throw new Error('office-codec-file-invalid')
    const bytes = readFileSync(path)
    if (name === 'package.json') {
      if (bytes.toString('utf8') !== '{"type":"commonjs"}\n') throw new Error('office-codec-module-type-invalid')
    } else assertOfficeCodecImports(bytes.toString('utf8'))
    return { name, byteLength: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex') }
  })
  return { entryPath: join(directory, OFFICE_CODEC_ENTRY), files }
}

export async function buildOfficeCodec({ outputDirectory = join(repoRoot, OFFICE_CODEC_DIRECTORY) } = {}) {
  const result = await build({
    configFile: false,
    envDir: false,
    root: repoRoot,
    publicDir: false,
    logLevel: 'error',
    resolve: { conditions: ['node'], mainFields: ['module', 'main'] },
    build: {
      target: 'node22',
      write: false,
      minify: false,
      sourcemap: false,
      commonjsOptions: {
        transformMixedEsModules: true,
        // debug probes this uninstalled optional terminal-color package inside
        // try/catch. The codec has no terminal; preserve its no-color fallback.
        ignoreTryCatch: id => id === 'supports-color' ? 'remove' : false
      },
      lib: { entry: join(repoRoot, 'src/main/office/office-generation-codec-entry.ts'), formats: ['cjs'] },
      rollupOptions: {
        external: id => isBuiltin(id),
        onwarn(warning, warn) {
          if (warning.code === 'UNRESOLVED_IMPORT') throw new Error(`office-codec-unresolved-import: ${warning.exporter ?? warning.source ?? warning.message}`)
          warn(warning)
        },
        output: { format: 'cjs', entryFileNames: OFFICE_CODEC_ENTRY, inlineDynamicImports: true }
      }
    }
  })
  const outputs = Array.isArray(result) ? result : [result]
  if (outputs.length !== 1 || !('output' in outputs[0]) || outputs[0].output.length !== 1) throw new Error('office-codec-output-invalid')
  const chunk = outputs[0].output[0]
  if (chunk.type !== 'chunk' || chunk.fileName !== OFFICE_CODEC_ENTRY ||
      [...chunk.imports, ...chunk.dynamicImports].some(id => !isBuiltin(id))) throw new Error('office-codec-output-invalid')
  assertOfficeCodecImports(chunk.code)
  if (Buffer.byteLength(chunk.code) > maximumBytes) throw new Error('office-codec-output-too-large')
  mkdirSync(outputDirectory, { recursive: true })
  writeFileSync(join(outputDirectory, OFFICE_CODEC_ENTRY), chunk.code)
  writeFileSync(join(outputDirectory, 'package.json'), '{"type":"commonjs"}\n')
  return { ...validateOfficeCodecDirectory(outputDirectory), watchFiles: Object.keys(chunk.modules).filter(id => !id.startsWith('\0')) }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  if (process.argv.length !== 2) throw new Error('office-codec-build-unexpected-arguments')
  const output = await buildOfficeCodec()
  console.log(`Office codec built: ${output.entryPath}`)
}
