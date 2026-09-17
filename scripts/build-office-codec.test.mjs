import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import JSZip from 'jszip'
import { assertOfficeCodecImports, buildOfficeCodec, OFFICE_CODEC_ENTRY, validateOfficeCodecDirectory } from './build-office-codec.mjs'

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-office-codec-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  return root
}

test('closure rejects package, relative and dynamic imports but accepts Node builtins', () => {
  assertOfficeCodecImports('const fs = require("node:fs"); const path = require("path"); import("node:util")')
  for (const code of ['require("docx")', 'require("./shared.js")', 'require(name)', 'import("jszip")', 'import(name)', 'require("electron")']) {
    assert.throws(() => assertOfficeCodecImports(code), /office-codec-/)
  }
})

test('directory admission rejects missing closure, wrong module type and symlinks', t => {
  const root = fixture(t)
  writeFileSync(join(root, OFFICE_CODEC_ENTRY), 'require("node:fs")')
  assert.throws(() => validateOfficeCodecDirectory(root), /files-invalid/)
  writeFileSync(join(root, 'package.json'), '{"type":"module"}\n')
  assert.throws(() => validateOfficeCodecDirectory(root), /module-type-invalid/)
  writeFileSync(join(root, 'package.json'), '{"type":"commonjs"}\n')
  assert.equal(validateOfficeCodecDirectory(root).files.length, 2)
  writeFileSync(join(root, 'shared.js'), 'module.exports = 1')
  assert.throws(() => validateOfficeCodecDirectory(root), /files-invalid/)
  rmSync(join(root, 'shared.js'))
  rmSync(join(root, OFFICE_CODEC_ENTRY))
  symlinkSync(join(root, 'package.json'), join(root, OFFICE_CODEC_ENTRY))
  assert.throws(() => validateOfficeCodecDirectory(root), /file-invalid/)
})

test('standalone bundle executes real DOCX and PPTX outside the source tree with only its two files', { timeout: 120000 }, async t => {
  const root = fixture(t), outputDirectory = join(root, 'codec')
  const result = await buildOfficeCodec({ outputDirectory })
  assert.equal(result.files.length, 2)
  assert.ok(result.watchFiles.some(file => file.endsWith('presentation-generation-codec.ts')))
  assert.deepEqual(readdirSync(outputDirectory).sort(), [OFFICE_CODEC_ENTRY, 'package.json'].sort())
  // A source-tree or ambient NODE_PATH fallback must not satisfy dependencies.
  const cwd = join(root, 'empty-cwd'); mkdirSync(cwd)
  const encode = input => {
    const child = spawnSync(process.execPath, [result.entryPath], {
      cwd, env: { ELECTRON_RUN_AS_NODE: '1', LANG: 'en_US.UTF-8' },
      input: JSON.stringify(input), timeout: 60000, maxBuffer: 16 * 1024 * 1024
    })
    assert.equal(child.error, undefined)
    assert.equal(child.status, 0, child.stderr.toString())
    return child.stdout
  }
  const png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='
  const images = [{ id: 'figure', type: 'png', dataBase64: png }]
  const docx = await JSZip.loadAsync(encode({ schemaVersion: 1, kind: 'docx', title: '合成中文文档',
    markdown: '# 中文标题\n\n| 项目 | 数值 |\n| --- | --- |\n| 合计 | 300 |\n\n![示意](figure)', images }))
  const document = await docx.file('word/document.xml').async('string')
  assert.match(document, /中文标题/); assert.match(document, /<w:tbl>/); assert.match(document, /300/)
  const docMedia = Object.keys(docx.files).find(name => /^word\/media\/.*\.png$/.test(name))
  assert.deepEqual(await docx.file(docMedia).async('nodebuffer'), Buffer.from(png, 'base64'))
  const pptx = await JSZip.loadAsync(encode({ schemaVersion: 1, kind: 'pptx', images,
    presentation: { slides: [{ id: 'summary', objects: [
      { id: 'title', kind: 'text', x: 1, y: 0.5, w: 10, h: 1, text: '合成中文演示' },
      { id: 'chart', kind: 'chart', x: 1, y: 2, w: 8, h: 4, chartType: 'bar', categories: ['甲', '乙'], series: [{ name: '金额', values: [100, 200] }] },
      { id: 'picture', kind: 'image', imageId: 'figure', x: 10, y: 2, w: 1, h: 1 }
    ] }] } }))
  const slide = await pptx.file('ppt/slides/slide1.xml').async('string')
  assert.match(slide, /name="summary"/); assert.match(slide, /name="title"/); assert.match(slide, /合成中文演示/)
  assert.match(await pptx.file('ppt/charts/chart1.xml').async('string'), /200/)
  const workbook = Object.keys(pptx.files).find(name => /^ppt\/embeddings\/.*\.xlsx$/.test(name))
  const embedded = await JSZip.loadAsync(await pptx.file(workbook).async('nodebuffer'))
  assert.match(await embedded.file('xl/worksheets/sheet1.xml').async('string'), /200/)
  for (const zip of [docx, pptx]) {
    assert.ok(zip.file('[Content_Types].xml')); assert.ok(zip.file('_rels/.rels'))
    for (const [name, part] of Object.entries(zip.files)) if (name.endsWith('.rels')) assert.doesNotMatch(await part.async('string'), /TargetMode="External"/)
  }
  const rejected = spawnSync(process.execPath, [result.entryPath], { cwd, env: { ELECTRON_RUN_AS_NODE: '1' }, input: '{"kind":"unknown"}', timeout: 60000 })
  assert.equal(rejected.status, 1); assert.equal(rejected.stdout.length, 0); assert.equal(rejected.stderr.length, 0)
  assertOfficeCodecImports(readFileSync(result.entryPath, 'utf8'))
})
