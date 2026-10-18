import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import JSZip from 'jszip'
import ExcelJS from 'exceljs'
import { assertOfficeCodecImports, buildOfficeCodec, OFFICE_CODEC_ENTRY, EXCEL_PREVIEW_COPY_CODEC_ENTRY, validateOfficeCodecDirectory } from './build-office-codec.mjs'

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
  writeFileSync(join(root, EXCEL_PREVIEW_COPY_CODEC_ENTRY), 'require("node:buffer")')
  writeFileSync(join(root, 'package.json'), '{"type":"module"}\n')
  assert.throws(() => validateOfficeCodecDirectory(root), /module-type-invalid/)
  writeFileSync(join(root, 'package.json'), '{"type":"commonjs"}\n')
  assert.equal(validateOfficeCodecDirectory(root).files.length, 3)
  writeFileSync(join(root, 'shared.js'), 'module.exports = 1')
  assert.throws(() => validateOfficeCodecDirectory(root), /files-invalid/)
  rmSync(join(root, 'shared.js'))
  rmSync(join(root, OFFICE_CODEC_ENTRY))
  symlinkSync(join(root, 'package.json'), join(root, OFFICE_CODEC_ENTRY))
  assert.throws(() => validateOfficeCodecDirectory(root), /file-invalid/)
})

test('standalone bundle executes real DOCX and PPTX outside the source tree with only its three files', { timeout: 120000 }, async t => {
  const root = fixture(t), outputDirectory = join(root, 'codec')
  const result = await buildOfficeCodec({ outputDirectory })
  assert.equal(result.files.length, 3)
  assert.ok(result.watchFiles.some(file => file.endsWith('presentation-generation-codec.ts')))
  assert.deepEqual(readdirSync(outputDirectory).sort(), [OFFICE_CODEC_ENTRY, EXCEL_PREVIEW_COPY_CODEC_ENTRY, 'package.json'].sort())
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
  // Qualify the same single built payload's data-only Excel entry with no ambient modules.
  const run = value => spawnSync(process.execPath, [join(outputDirectory, EXCEL_PREVIEW_COPY_CODEC_ENTRY)], {
    cwd, env: { ELECTRON_RUN_AS_NODE: '1', LANG: 'C', TZ: 'UTC' }, input: JSON.stringify(value), timeout: 20000, maxBuffer: 16 << 20
  })
  const selection = { sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 1], column: [0, 1] }] }
  const request = { schemaVersion: 1, op: 'excel-preview-copy-cells-v1', format: 'csv', selection,
    dataBase64: Buffer.from('literal,=2+2\nzero,false').toString('base64') }
  const child = run(request)
  assert.equal(child.error, undefined); assert.equal(child.status, 0); assert.equal(child.stderr.length, 0)
  const frame = JSON.parse(child.stdout.toString())
  assert.equal(frame.sheetId, '1'); assert.equal(frame.cells[1][2].v, '=2+2')
  assert.match(frame.html, /<table data-type="fortune-copy-action-table">/); assert.match(frame.html, /literal<\/td>/)
  const saved = new ExcelJS.Workbook(), savedSheet = saved.addWorksheet('Saved')
  savedSheet.getCell('A1').value = 1234.5; savedSheet.getCell('A1').numFmt = '#,##0.00'
  savedSheet.getCell('B1').value = { formula: '1-1', result: 0 }
  savedSheet.getCell('C1').value = { formula: '1=2', result: false }
  savedSheet.getCell('D1').value = { formula: '1+1' }
  const cached = run({ ...request, format: 'xlsx', selection: { sheetIndex: 0, sheetId: '1', ranges: [{ row: [0, 0], column: [0, 3] }] },
    dataBase64: Buffer.from(await saved.xlsx.writeBuffer()).toString('base64') })
  assert.equal(cached.error, undefined); assert.equal(cached.status, 0); assert.equal(cached.stderr.length, 0)
  const cachedFrame = JSON.parse(cached.stdout.toString())
  assert.equal(cachedFrame.cells.find(cell => cell[1] === 0)[2].m, '1,234.50')
  assert.equal(cachedFrame.cells.find(cell => cell[1] === 1)[2].v, 0)
  assert.equal(cachedFrame.cells.find(cell => cell[1] === 2)[2].v, false)
  assert.equal(cachedFrame.cells.find(cell => cell[1] === 3)[2].m, '')
  assert.equal(Object.hasOwn(cachedFrame.cells.find(cell => cell[1] === 3)[2], 'v'), false)
  assert.match(cachedFrame.html, /1,234.50<\/td>/); assert.match(cachedFrame.html, />0<\/td>/); assert.match(cachedFrame.html, />false<\/td>/)
  for (const invalid of [{ ...request, selection: { ...selection, sheetId: 'wrong' } }, { ...request, path: '/untrusted' }]) {
    const rejected = run(invalid)
    assert.equal(rejected.status, 1); assert.equal(rejected.stdout.length, 0); assert.equal(rejected.stderr.length, 0)
  }
})


test('Excel Copy entry closes network builtin escape paths while generation closure keeps its contract', () => {
  for (const name of ['http', 'node:https', 'node:http2', 'net', 'tls', 'dns/promises', 'dgram', 'node:child_process', 'worker_threads', 'vm']) {
    assert.throws(() => assertOfficeCodecImports(`require(${JSON.stringify(name)})`, { allowNetwork: false }), /office-codec-/)
  }
  assertOfficeCodecImports('require("node:buffer"); require("node:stream")', { allowNetwork: false })
})
