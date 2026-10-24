import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { createSuperDocUI } from 'superdoc/ui'
import { Script } from 'node:vm'
import { parse } from 'acorn'
import { dependencyChecks } from './development-doctor.mjs'

const require = createRequire(import.meta.url)
const { applyDshPreviewPatches, dshPreviewPatchInputDigest, requiredDshPreviewPatchesCurrent } = require('./apply-dsh-preview-patches.cjs')
const { appInstallationDigest, appInstallCurrent, beginAppInstall, recordAppInstall } = require('./app-install-state.cjs')
const sourceRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const patchPaths = ["patches/@fortune-sheet+core+1.0.4.patch", "patches/@fortune-sheet+react+1.0.4.patch", "patches/exceljs+4.4.0.patch", "patches/superdoc+2.24.0.patch"]
const copiedTargets = ["node_modules/@fortune-sheet/core/dist/index.esm.js", "node_modules/@fortune-sheet/core/dist/index.js", "node_modules/@fortune-sheet/core/dist/settings.d.ts", "node_modules/@fortune-sheet/core/dist/context.d.ts", "node_modules/@fortune-sheet/core/dist/canvas.d.ts", "node_modules/@fortune-sheet/react/dist/index.esm.js", "node_modules/@fortune-sheet/react/dist/index.js", "node_modules/@fortune-sheet/react/dist/components/Workbook/index.d.ts", "node_modules/@fortune-sheet/react/dist/components/SheetTab/index.d.ts", "node_modules/exceljs/dist/exceljs.js", "node_modules/exceljs/lib/utils/parse-sax.js", "node_modules/exceljs/lib/xlsx/xform/book/workbook-xform.js", "node_modules/exceljs/lib/xlsx/xform/comment/comment-xform.js", "node_modules/exceljs/lib/xlsx/xform/sheet/worksheet-xform.js", "node_modules/exceljs/lib/xlsx/xlsx.js", "node_modules/exceljs/package.json", "node_modules/superdoc/dist/chunks/create-super-doc-ui-8UTu16Lw.es.js", "node_modules/superdoc/dist/chunks/create-super-doc-ui-bTFGKVRb.cjs"]
const metadataPaths = ["node_modules/@fortune-sheet/core/package.json", "node_modules/@fortune-sheet/react/package.json", "node_modules/exceljs/package.json", "node_modules/patch-package/package.json", "node_modules/superdoc/package.json"]
function fixture(t) {
  // Run after the one normal dependency-install qualification. Only ordinary
  // package/source bytes are copied; no package modules or scripts are loaded.
  assert.equal(requiredDshPreviewPatchesCurrent(sourceRoot), true, 'the test source must contain the qualified required patches')
  const root = mkdtempSync(join(tmpdir(), 'analytix-preview-patches-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  for (const path of [...new Set([...patchPaths, ...copiedTargets, ...metadataPaths,
    'scripts/postinstall.cjs', 'scripts/ensure-runtime-install.cjs', 'scripts/app-install-state.cjs', 'scripts/apply-dsh-preview-patches.cjs'])]) {
    const target = join(root, path); mkdirSync(dirname(target), { recursive: true }); copyFileSync(join(sourceRoot, path), target)
  }
  const manifest = { dependencies: { '@fortune-sheet/core': '1.0.4', '@fortune-sheet/react': '1.0.4', exceljs: '4.4.0', superdoc: '2.24.0' }, devDependencies: { 'patch-package': '8.0.1' } }
  writeFileSync(join(root, 'package.json'), JSON.stringify(manifest))
  const lock = { packages: Object.fromEntries(Object.entries({ ...manifest.dependencies, ...manifest.devDependencies }).map(([name, version]) => ['node_modules/' + name, { version }])) }
  writeFileSync(join(root, 'package-lock.json'), JSON.stringify(lock))
  for (const name of ['typescript', 'electron-vite', 'electron']) {
    const path = join(root, 'node_modules', name, 'package.json'); mkdirSync(dirname(path), { recursive: true }); writeFileSync(path, '{}')
  }
  return { root, stamp: join(root, 'node_modules/.analytix-install.json'), digest: () => appInstallationDigest(root) }
}
test('a fully qualified patch set is idempotent without invoking any package code', t => {
  const f = fixture(t)
  const result = applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') })
  assert.equal(result.status, 'already_applied')
  recordAppInstall(f.root, f.digest()); assert.equal(appInstallCurrent(f.root), true)
})
test('npm string and named bin metadata qualify only the fixed local patch entry', t => {
  const f = fixture(t), path = join(f.root, 'node_modules/patch-package/package.json')
  const tool = JSON.parse(readFileSync(path, 'utf8'))
  for (const bin of ['./index.js', 'index.js', { 'patch-package': './index.js' }, { 'patch-package': 'index.js' }]) {
    writeFileSync(path, JSON.stringify({ ...tool, bin }))
    assert.equal(requiredDshPreviewPatchesCurrent(f.root), true)
    assert.equal(applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }).status, 'already_applied')
  }
  for (const bin of ['../index.js', './other.js', { 'patch-package': './other.js' }, { 'patch-package': './index.js', other: './other.js' }]) {
    writeFileSync(path, JSON.stringify({ ...tool, bin }))
    assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
    assert.throws(() => applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }), /reviewed local patch tool/)
  }
})
test('same-version altered package code is never installation-current and doctor does not repair it', t => {
  const f = fixture(t); recordAppInstall(f.root, f.digest())
  const beforeStamp = readFileSync(f.stamp)
  const target = join(f.root, 'node_modules/@fortune-sheet/core/dist/index.esm.js')
  const changed = Buffer.concat([readFileSync(target), Buffer.from('\n// synthetic body alteration\n')]); writeFileSync(target, changed)
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
  assert.equal(appInstallCurrent(f.root), false)
  assert.equal(dependencyChecks(f.root).find(check => check.name.startsWith('App dependencies match')).passed, false)
  assert.deepEqual(readFileSync(target), changed); assert.deepEqual(readFileSync(f.stamp), beforeStamp)
  assert.throws(() => recordAppInstall(f.root, f.digest()), /incomplete/)
  assert.throws(() => applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }), /Partial or unexpected/)
})
test('a missing or changed reviewed patch cannot obtain or retain a successful stamp', t => {
  const f = fixture(t); recordAppInstall(f.root, f.digest())
  const path = join(f.root, patchPaths[0]); writeFileSync(path, Buffer.concat([readFileSync(path), Buffer.from('\n')]))
  assert.equal(appInstallCurrent(f.root), false)
  assert.throws(() => dshPreviewPatchInputDigest(f.root), /content differs/)
  rmSync(path); assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
})
test('wrong local tool version and incomplete declarations fail before any package invocation', t => {
  const f = fixture(t)
  const path = join(f.root, 'node_modules/patch-package/package.json')
  const tool = JSON.parse(readFileSync(path, 'utf8')); tool.version = '7.0.0'; writeFileSync(path, JSON.stringify(tool))
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
  assert.throws(() => applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }), /reviewed local patch tool/)
  const manifestPath = join(f.root, 'package.json'); const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')); delete manifest.dependencies.exceljs; writeFileSync(manifestPath, JSON.stringify(manifest))
  assert.throws(() => dshPreviewPatchInputDigest(f.root), /declared together/)
})
test('a same-input postinstall retry clears its previous success until the lifecycle records completion', t => {
  const f = fixture(t); recordAppInstall(f.root, f.digest())
  const digest = beginAppInstall(f.root); assert.equal(appInstallCurrent(f.root), false)
  recordAppInstall(f.root, digest); assert.equal(appInstallCurrent(f.root), true)
})


test('extra patch files cannot be applied or qualify an installation', t => {
  const f = fixture(t)
  writeFileSync(join(f.root, 'patches', 'unreviewed.patch'), 'synthetic unreviewed source')
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
  assert.throws(() => applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }), /exactly the reviewed/)
})

// This finite harness qualifies only actual patched dist entry branches.
// It does not load the whole Fortune package, render a DOM/native UI, prove
// complete keyboard dispatch, or establish the converter's Node qualification.
const qualifiedCoreSlices = new Map()
const astMember = (node, object, property) => node?.type === 'MemberExpression' && !node.computed &&
  node.object?.type === 'Identifier' && node.object.name === object &&
  node.property?.type === 'Identifier' && node.property.name === property
const protectedCondition = node => node?.type === 'BinaryExpression' && node.operator === '!==' &&
  astMember(node.left, 'ctx', 'protectedCopy') && node.right?.type === 'Identifier' && node.right.name === 'undefined'
const callNamed = (node, name) => node?.type === 'CallExpression' && node.callee?.type === 'Identifier' && node.callee.name === name
const eventCall = (node, name) => node?.type === 'ExpressionStatement' && node.expression?.type === 'CallExpression' &&
  astMember(node.expression.callee, 'e', name) && node.expression.arguments.length === 0
function astNodes(node, predicate, results = []) {
  if (!node || typeof node !== 'object') return results
  if (typeof node.type === 'string' && predicate(node)) results.push(node)
  for (const value of Object.values(node)) {
    if (Array.isArray(value)) for (const child of value) astNodes(child, predicate, results)
    else if (value && typeof value === 'object' && typeof value.type === 'string') astNodes(value, predicate, results)
  }
  return results
}
function qualifiedCoreSlice(file) {
  if (qualifiedCoreSlices.has(file)) return qualifiedCoreSlices.get(file)
  assert.equal(requiredDshPreviewPatchesCurrent(sourceRoot), true, 'all reviewed actual installed postimages must qualify before dist extraction')
  const source = readFileSync(join(sourceRoot, 'node_modules/@fortune-sheet/core/dist', file), 'utf8')
  const ast = parse(source, { ecmaVersion: 'latest', sourceType: file.endsWith('.esm.js') ? 'module' : 'script' })
  const namedFunction = name => {
    const matches = ast.body.filter(node => node.type === 'FunctionDeclaration' && node.id?.name === name)
    assert.equal(matches.length, 1, `exactly one top-level ${name} declaration in ${file}`)
    return matches[0]
  }
  const copy = namedFunction('copy'), handleCopy = namedFunction('handleCopy'), overlap = namedFunction('selectIsOverlap')
  const bounds = ['isReadonlyViewport', 'trueSheetBounds', 'trueRangeInBounds', 'trueSelectionsInBounds', 'readonlyTruePixel']
    .map(name => { const node = namedFunction(name); return source.slice(node.start, node.end) }).join('\n')
  const geometry = ['readonlyViewAxis', 'readonlyViewAxisEnd', 'readonlyViewAxisIndex', 'createReadonlyViewGeometry']
    .map(name => { const node = namedFunction(name); return source.slice(node.start, node.end) }).join('\n')
  const wheel = namedFunction('handleGlobalWheel')
  const wheelGuards = wheel.body.body.filter(node => node.type === 'IfStatement' &&
    astNodes(node.test, value => callNamed(value, 'isReadonlyViewport')).length)
  assert.equal(wheelGuards.length, 1, 'one opt-in readonly viewport wheel branch')
  const wheelGuard = source.slice(wheelGuards[0].start, wheelGuards[0].end)
  const controlKeys = namedFunction('handleWithCtrlOrMetaKey')
  const menu = namedFunction('handleContextMenu')
  assert.equal(menu.body.body[0].type, 'IfStatement')
  assert.ok(astNodes(menu.body.body[0], node => callNamed(node, 'trueSelectionsInBounds')).length,
    'actual opt-in menu entry rejects a pre-existing virtual selection first')
  assert.equal(menu.body.body[1].type, 'VariableDeclaration')
  assert.equal(menu.body.body[1].declarations[0].id.name, 'readonlyProtectedCopyOnly')
  assert.equal(menu.body.body[2].type, 'IfStatement')
  assert.equal(menu.body.body[2].test.operator, '&&')
  assert.equal(menu.body.body[3].type, 'VariableDeclaration')
  assert.ok(astNodes(menu.body.body[3], node => callNamed(node, 'getFlowdata')).length, 'normal menu body follows the readonly gate')
  const guard = copy.body.body[0]
  assert.equal(guard.type, 'IfStatement'); assert.equal(protectedCondition(guard.test), true)
  assert.equal(guard.alternate, null); assert.equal(guard.consequent.type, 'BlockStatement')
  assert.equal(guard.consequent.body.at(-1)?.type, 'ReturnStatement', 'the actual protected branch must end before the native copy body')
  assert.equal(guard.consequent.body.at(-1)?.argument, null)
  assert.ok(astNodes(copy.body.body[1], node => callNamed(node, 'getFlowdata')).length, 'native flow access follows the protected branch')
  assert.ok(astNodes(handleCopy.body, node => callNamed(node, 'hasPartMC')).length, 'the actual partial-merge preflight remains')
  assert.ok(astNodes(handleCopy.body, node => callNamed(node, 'selectIsOverlap')).length, 'the actual multi-range preflight remains')
  const keys = astNodes(controlKeys.body, node => node.type === 'IfStatement' && node.test?.type === 'BinaryExpression' &&
    node.test.operator === '===' && astMember(node.test.left, 'e', 'code') && node.test.right?.type === 'Literal' && node.test.right.value === 'KeyC')
  assert.equal(keys.length, 1, 'extract only one AST-proven KeyC leaf branch')
  const keyC = keys[0].consequent
  assert.equal(keyC.type, 'BlockStatement')
  const statements = keyC.body
  assert.equal(statements.length, 4, 'unexpected KeyC branch shape needs a new review')
  assert.equal(statements[0].type, 'IfStatement'); assert.equal(protectedCondition(statements[0].test), true)
  assert.equal(eventCall(statements[0].consequent, 'preventDefault'), true)
  assert.equal(callNamed(statements[1].expression, 'handleCopy'), true)
  assert.equal(statements[1].expression.arguments[0]?.name, 'ctx')
  assert.equal(eventCall(statements[2], 'stopPropagation'), true)
  assert.equal(statements[3].type, 'ReturnStatement'); assert.equal(statements[3].argument, null)
  const slice = { bounds, geometry, wheelGuard, guard: source.slice(guard.start, guard.end), handleCopy: source.slice(handleCopy.start, handleCopy.end), overlap: source.slice(overlap.start, overlap.end),
    keyC: source.slice(keyC.start + 1, keyC.end - 1),
    menuGate: source.slice(menu.body.body[1].start, menu.body.body[3].start) }
  qualifiedCoreSlices.set(file, slice)
  return slice
}
function distCopyHarness(file, { partialMerge = false } = {}) {
  const slice = qualifiedCoreSlice(file)
  const effects = [], trace = []
  const forbidden = name => () => { effects.push(name); throw new Error(`Forbidden native effect: ${name}`) }
  const forbiddenObject = name => new Proxy({}, { get() { return forbidden(name)() } })
  const lodash = { isEmpty: value => value == null || value.length === 0, isNil: value => value == null, cloneDeep: value => structuredClone(value) }
  const sandbox = { _: lodash, ___default: { default: lodash }, getFlowdata: forbidden('getFlowdata'),
    clipboard: forbiddenObject('clipboard'), localStorage: forbiddenObject('localStorage'), sessionStorage: forbiddenObject('sessionStorage'),
    storage: forbiddenObject('storage'), navigator: forbiddenObject('navigator'), document: forbiddenObject('document'),
    cancelPaintModel: forbidden('cancelPaintModel'),
    getSheetIndex: (ctx, id) => ctx.luckysheetfile.findIndex(sheet => sheet.id === id),
    getComputeMap: forbidden('getComputeMap'), checkCF: forbidden('checkCF'),
    hasPartMC: () => { trace.push('partialMergePreflight'); return partialMerge } }
  const api = new Script(`(() => {
    ${slice.bounds}
    function copy(ctx) { ${slice.guard}\n getFlowdata(ctx); }
    ${slice.overlap}
    ${slice.handleCopy}
    function keyC(ctx, e) { ${slice.keyC} }
    return { copy, handleCopy, keyC };
  })()`).runInNewContext(sandbox, { timeout: 1000 })
  return { ...api, effects, trace }
}
function protectedContext(callback) {
  return { protectedCopy: callback, allowEdit: false, readonlyViewport: true, currentSheetId: 'current-sheet',
    luckysheetfile: [{ id: 'current-sheet', data: Array.from({ length: 128 }, () => Array(8).fill(null)) }],
    luckysheet_select_save: [{ row: [2, 4], column: [1, 3] }], config: {}, luckysheetPaintModelOn: false,
    luckysheet_selection_range: ['existing-selection'], luckysheet_copy_save: { existing: true }, iscopyself: false }
}
for (const file of ['index.esm.js', 'index.js']) {
  test(`${file}: readonly menus admit only protected cell Copy without enabling editing`, () => {
    const slice = qualifiedCoreSlice(file)
    const gate = new Script(`(function(ctx, settings, area) { ${slice.menuGate} return true; })`).runInNewContext({}, { timeout: 1000 })
    for (const protectedCopy of [null, () => false]) {
      assert.equal(gate({ allowEdit: false, protectedCopy }, { cellContextMenu: ['copy'] }, 'cell'), true)
    }
    for (const [ctx, settings, area] of [
      [{ allowEdit: false }, { cellContextMenu: ['copy'] }, 'cell'],
      [{ allowEdit: false, protectedCopy: null }, { cellContextMenu: ['copy'] }, 'row'],
      [{ allowEdit: false, protectedCopy: null }, { cellContextMenu: ['copy'] }, 'column'],
      [{ allowEdit: false, protectedCopy: null }, { cellContextMenu: ['copy', 'paste'] }, 'cell'],
      [{ allowEdit: false, protectedCopy: null }, { cellContextMenu: [] }, 'cell'],
      [{ allowEdit: false, protectedCopy: null }, { cellContextMenu: 'copy' }, 'cell'],
      [{ protectedCopy: null }, { cellContextMenu: ['copy'] }, 'cell'],
    ]) assert.equal(gate(ctx, settings, area), undefined)
    assert.equal(gate({ allowEdit: true }, { cellContextMenu: ['copy', 'paste'] }, 'row'), true)
  })

  test(`${file}: the protected copy branch has no native effects for null, false, true, throw or Promise`, () => {
    for (const behavior of ['null', 'false', 'true', 'throw', 'Promise']) {
      const harness = distCopyHarness(file)
      let calls = 0
      const callback = behavior === 'null' ? null : () => {
        calls++
        if (behavior === 'throw') throw new Error('Synthetic unavailable owner')
        if (behavior === 'Promise') return Promise.resolve(true)
        return behavior === 'true'
      }
      const ctx = protectedContext(callback), savedSelection = ctx.luckysheet_selection_range, savedCopy = ctx.luckysheet_copy_save
      harness.copy(ctx)
      assert.equal(calls, behavior === 'null' ? 0 : 1)
      assert.deepEqual(harness.effects, [], behavior)
      assert.equal(ctx.luckysheet_selection_range, savedSelection); assert.equal(ctx.luckysheet_copy_save, savedCopy)
      assert.equal(ctx.iscopyself, false)
    }
  })
  test(`${file}: one, two and 32 aligned ranges pass handleCopy with current sheet identity, order and cloned tuples`, () => {
    for (const count of [1, 2, 32]) {
      const harness = distCopyHarness(file)
      let captured
      const ctx = protectedContext(selection => { captured = selection; return true })
      ctx.luckysheet_select_save = Array.from({ length: count }, (_, index) => ({ row: [(count - index - 1) * 2, (count - index - 1) * 2], column: [1, 3] }))
      const original = structuredClone(ctx.luckysheet_select_save)
      harness.handleCopy(ctx)
      assert.equal(captured.sheetId, 'current-sheet'); assert.equal(captured.ranges.length, count)
      assert.deepEqual(Array.from(captured.ranges, range => ({ row: Array.from(range.row), column: Array.from(range.column) })), original)
      assert.notEqual(captured.ranges, ctx.luckysheet_select_save)
      for (let index = 0; index < count; index++) {
        assert.notEqual(captured.ranges[index].row, ctx.luckysheet_select_save[index].row)
        assert.notEqual(captured.ranges[index].column, ctx.luckysheet_select_save[index].column)
      }
      ctx.currentSheetId = 'successor-sheet'; captured.ranges[0].row[0] = 99; captured.ranges[0].column[0] = 99
      assert.equal(captured.sheetId, 'current-sheet'); assert.deepEqual(ctx.luckysheet_select_save, original)
      assert.deepEqual(harness.effects, [])
    }
  })
  test(`${file}: invalid, empty, over-32 and missing-current-sheet selections never reach the protected callback`, () => {
    const mutations = [ctx => { ctx.luckysheet_select_save = [] },
      ctx => { ctx.luckysheet_select_save = Array.from({ length: 33 }, (_, index) => ({ row: [index * 2, index * 2], column: [1, 3] })) },
      ctx => { ctx.luckysheet_select_save[0].row = [-1, 4] },
      ctx => { ctx.luckysheet_select_save[0].column = [3, 1] },
      ctx => { ctx.luckysheet_select_save[0].row = [2, Infinity] },
      ctx => { ctx.luckysheetfile = [{ id: 'other-sheet' }] },
      ctx => { ctx.currentSheetId = null },
      ctx => { ctx.luckysheet_select_save[0].row = [0, 128] },
      ctx => { ctx.luckysheet_select_save[0].column = [0, 8] },
      ctx => { delete ctx.luckysheetfile[0].data }]
    for (const mutate of mutations) {
      const harness = distCopyHarness(file); let calls = 0
      const ctx = protectedContext(() => { calls++; return true }); mutate(ctx)
      harness.copy(ctx)
      assert.equal(calls, 0); assert.deepEqual(harness.effects, [])
    }
  })
  test(`${file}: the original handleCopy prechecks reject nonaligned and overlapping ranges`, () => {
    for (const ranges of [
      [{ row: [0, 1], column: [0, 1] }, { row: [3, 4], column: [4, 5] }],
      [{ row: [0, 2], column: [1, 3] }, { row: [2, 4], column: [1, 3] }],
    ]) {
      const harness = distCopyHarness(file); let calls = 0
      const ctx = protectedContext(() => { calls++; return true }); ctx.luckysheet_select_save = ranges
      harness.handleCopy(ctx)
      assert.equal(calls, 0); assert.deepEqual(harness.effects, [])
    }
  })
  test(`${file}: the existing partial-merge preflight still rejects before protected copy`, () => {
    const harness = distCopyHarness(file, { partialMerge: true }); let calls = 0
    const ctx = protectedContext(() => { calls++; return true }); ctx.config.merge = { existing: true }
    harness.handleCopy(ctx)
    assert.equal(calls, 0); assert.deepEqual(harness.trace, ['partialMergePreflight']); assert.deepEqual(harness.effects, [])
  })
  test(`${file}: the AST-proven KeyC leaf prevents default synchronously before protected copy`, () => {
    const harness = distCopyHarness(file)
    const ctx = protectedContext(() => { harness.trace.push('protectedCopy'); return true })
    harness.keyC(ctx, { preventDefault: () => harness.trace.push('preventDefault'), stopPropagation: () => harness.trace.push('stopPropagation') })
    assert.deepEqual(harness.trace, ['preventDefault', 'protectedCopy', 'stopPropagation']); assert.deepEqual(harness.effects, [])
    const unavailable = distCopyHarness(file)
    unavailable.keyC(protectedContext(null), { preventDefault: () => unavailable.trace.push('preventDefault'), stopPropagation: () => unavailable.trace.push('stopPropagation') })
    assert.deepEqual(unavailable.trace, ['preventDefault', 'stopPropagation']); assert.deepEqual(unavailable.effects, [])
  })
}


// The view descriptor is independent of the initialized matrix. This harness
// executes only the actual admitted helper/wheel leaves; it is not UI evidence.
for (const file of ['index.esm.js', 'index.js']) {
  function viewportHarness() {
    const slice = qualifiedCoreSlice(file)
    const lodash = { sortedIndex: (values, value) => {
      let low = 0, high = values.length
      while (low < high) { const mid = (low + high) >>> 1; if (values[mid] < value) low = mid + 1; else high = mid }
      return low
    } }
    return new Script(`(() => {
      ${slice.bounds}
      ${slice.geometry}
      function wheel(ctx, e, scrollbarX, scrollbarY) { ${slice.wheelGuard} }
      return { createReadonlyViewGeometry, readonlyViewAxisEnd, readonlyViewAxisIndex,
        readonlyTruePixel, trueSelectionsInBounds, wheel };
    })()`).runInNewContext({ _: lodash, ___default: { default: lodash } }, { timeout: 1000 })
  }
  function viewportContext() {
    return { allowEdit: false, readonlyViewport: true, visibledatarow: [20, 40, 60, 80],
      visibledatacolumn: [74, 148, 222], defaultrowlen: 19, defaultcollen: 73,
      cellmainWidth: 512, cellmainHeight: 320, zoomRatio: 1, rh_height: 160, ch_width: 342,
      currentSheetId: 'real', luckysheetfile: [{ id: 'real', row: 4, column: 3,
        data: Array.from({ length: 4 }, () => [null, null, null]) }],
      luckysheet_select_save: [{ row: [0, 3], column: [0, 2] }] }
  }
  test(`${file}: readonly view continuation leaves true data and axes untouched at row 39`, () => {
    const api = viewportHarness(), ctx = viewportContext(), before = structuredClone(ctx)
    const view = api.createReadonlyViewGeometry(ctx)
    assert.equal(view.rows.real, ctx.visibledatarow); assert.equal(view.columns.real, ctx.visibledatacolumn)
    assert.equal(view.rows.count, 1000); assert.equal(view.columns.count, 256)
    assert.equal(view.height, 20000); assert.equal(view.width, 18944)
    assert.equal(api.readonlyViewAxisIndex(view.rows, 761), 38)
    assert.equal(api.readonlyViewAxisEnd(view.rows, 38), 780)
    assert.equal(api.readonlyTruePixel(ctx, 10, 79), true)
    assert.equal(api.readonlyTruePixel(ctx, 10, 80), false)
    assert.equal(api.readonlyTruePixel(ctx, 222, 10), false)
    assert.equal(api.readonlyTruePixel(ctx, -1, 10), false)
    assert.equal(api.readonlyTruePixel(ctx, 10, Infinity), false)
    assert.equal(api.trueSelectionsInBounds(ctx, ctx.luckysheet_select_save), true)
    assert.equal(api.trueSelectionsInBounds(ctx, [{ row: [38, 38], column: [0, 0] }]), false)
    assert.deepEqual(ctx, before)
  })
  test(`${file}: viewport sizing accounts for zoom, resize and large real sheets without copying axes`, () => {
    const api = viewportHarness()
    for (const zoomRatio of [0.25, 0.5, 1, 2]) {
      const ctx = viewportContext(); ctx.zoomRatio = zoomRatio
      ctx.visibledatarow = ctx.visibledatarow.map(value => value * zoomRatio)
      ctx.visibledatacolumn = ctx.visibledatacolumn.map(value => value * zoomRatio)
      const view = api.createReadonlyViewGeometry(ctx)
      assert.equal(view.rows.step, Math.max(1, Math.round(20 * zoomRatio)))
      assert.equal(view.columns.step, Math.max(1, Math.round(74 * zoomRatio)))
      assert.ok(view.height >= 320); assert.ok(view.width >= 512)
      assert.equal(view.rows.real, ctx.visibledatarow)
    }
    const tall = viewportContext(); tall.cellmainHeight = 50000
    const resized = api.createReadonlyViewGeometry(tall)
    assert.ok(resized.height >= tall.visibledatarow.at(-1) + 50000)
    const large = viewportContext(); large.visibledatarow = Array.from({ length: 250000 }, (_, index) => (index + 1) * 20)
    large.visibledatacolumn = [74]
    large.luckysheetfile[0].row = 250000; large.luckysheetfile[0].column = 1
    large.luckysheetfile[0].data = Array.from({ length: 250000 }, () => [null])
    const realRows = large.visibledatarow, view = api.createReadonlyViewGeometry(large)
    assert.equal(view.rows.real, realRows); assert.equal(realRows.length, 250000)
    assert.ok(view.rows.count > realRows.length)
    const ordinary = viewportContext(); ordinary.readonlyViewport = false
    assert.equal(api.createReadonlyViewGeometry(ordinary), null)
    ordinary.readonlyViewport = true; ordinary.allowEdit = true
    assert.equal(api.createReadonlyViewGeometry(ordinary), null)
    ordinary.allowEdit = false; ordinary.zoomRatio = NaN
    assert.equal(api.createReadonlyViewGeometry(ordinary), null)
  })
  test(`${file}: finite pixel, line and page wheel deltas use browser-clamped offsets`, () => {
    const api = viewportHarness()
    for (const [deltaMode, expectedX, expectedY] of [[0, 2, 3], [1, 148, 60], [2, 1024, 960]]) {
      const ctx = viewportContext(), x = { scrollLeft: 0 }, y = { scrollTop: 0 }; let prevented = 0
      api.wheel(ctx, { deltaMode, deltaX: 2, deltaY: 3, preventDefault() { prevented++ } }, x, y)
      assert.equal(x.scrollLeft, expectedX); assert.equal(y.scrollTop, expectedY)
      assert.equal(ctx.scrollLeft, expectedX); assert.equal(ctx.scrollTop, expectedY); assert.equal(prevented, 1)
    }
    const ctx = viewportContext()
    let left = 0, top = 0
    const x = { get scrollLeft() { return left }, set scrollLeft(value) { left = Math.max(0, Math.min(100, value)) } }
    const y = { get scrollTop() { return top }, set scrollTop(value) { top = Math.max(0, Math.min(800, value)) } }
    api.wheel(ctx, { deltaMode: 0, deltaX: 200, deltaY: 2000, preventDefault() {} }, x, y)
    assert.equal(ctx.scrollLeft, 100); assert.equal(ctx.scrollTop, 800)
    for (const e of [{ deltaMode: 3, deltaX: 1, deltaY: 1 }, { deltaMode: 0, deltaX: Infinity, deltaY: 1 }]) {
      api.wheel(ctx, { ...e, preventDefault() {} }, x, y)
      assert.equal(ctx.scrollLeft, 100); assert.equal(ctx.scrollTop, 800)
    }
  })
}


for (const file of ['index.esm.js', 'index.js']) {
  test(`${file}: original compact Zoom actions retain exact quarter scale and checked preset without data history`, () => {
    assert.equal(requiredDshPreviewPatchesCurrent(sourceRoot), true)
    const source = readFileSync(join(sourceRoot, 'node_modules/@fortune-sheet/react/dist', file), 'utf8')
    const ast = parse(source, { ecmaVersion: 'latest', sourceType: file.endsWith('.esm.js') ? 'module' : 'script' })
    const declarations = astNodes(ast, node => node.type === 'VariableDeclarator' && node.id?.name === 'ZoomControl')
    assert.equal(declarations.length, 1); assert.equal(declarations[0].init.type, 'FunctionExpression')
    const actual = source.slice(declarations[0].init.start, declarations[0].init.end)
    let menuOpen = false
    const data = [[{ v: 0 }]], ctx = { currentSheetId: 'sheet', zoomRatio: 1, allowEdit: false,
      luckysheetfile: [{ id: 'sheet', zoomRatio: 1, data }] }
    const before = structuredClone(data), history = []
    const setContext = (update, options) => { update(ctx); history.push(structuredClone(options)) }
    const createElement = (type, props, ...children) => ({ type, props: props ?? {}, children })
    const hooks = { createElement, useContext: () => ({ context: ctx, setContext }),
      useRef: () => ({ current: null }), useEffect: () => {}, useCallback: callback => callback,
      useState: () => [menuOpen, value => { menuOpen = typeof value === 'function' ? value(menuOpen) : value }] }
    const core = { locale: () => ({ info: {} }), MIN_ZOOM_RATIO: 0.1, MAX_ZOOM_RATIO: 4,
      getSheetIndex: (context, id) => context.luckysheetfile.findIndex(sheet => sheet.id === id) }
    const render = new Script(`(${actual})`).runInNewContext({
      React: hooks, React__default: { default: hooks }, ...hooks, core,
      locale: core.locale, MIN_ZOOM_RATIO: core.MIN_ZOOM_RATIO, MAX_ZOOM_RATIO: core.MAX_ZOOM_RATIO,
      getSheetIndex: core.getSheetIndex, WorkbookContext: {}, useOutsideClick: () => {},
      _slicedToArray: value => value }, { timeout: 1000 })
    function elements(node, predicate, result = []) {
      if (Array.isArray(node)) { for (const child of node) elements(child, predicate, result) }
      else if (node && typeof node === 'object') {
        if (predicate(node)) result.push(node)
        for (const child of node.children ?? []) elements(child, predicate, result)
      }
      return result
    }
    let stops = 0, prevents = 0
    const event = { stopPropagation() { stops++ }, preventDefault() { prevents++ } }
    let tree = render({ compact: true })
    tree.props.onKeyDown({ ...event, key: 'Escape' })
    assert.equal(stops, 0); assert.equal(prevents, 0, 'closed menu leaves desktop Escape to its owner')
    for (const [label, scale] of [['25%', 0.25], ['50%', 0.5], ['100%', 1], ['200%', 2]]) {
      tree = render({ compact: true })
      elements(tree, node => node.props['aria-haspopup'] === 'menu')[0].props.onClick(event)
      tree = render({ compact: true })
      const choices = elements(tree, node => node.props.role === 'menuitemradio')
      assert.deepEqual(choices.map(node => node.children[0].children[0]), ['25%', '50%', '100%', '200%'])
      assert.equal(choices.filter(node => node.props['aria-checked']).length, 1)
      choices.find(node => node.children[0].children[0] === label).props.onClick(event)
      assert.equal(ctx.zoomRatio, scale); assert.equal(ctx.luckysheetfile[0].zoomRatio, scale)
      assert.equal(menuOpen, false); assert.equal(ctx.allowEdit, false)
      assert.equal(ctx.luckysheetfile[0].data, data); assert.deepEqual(data, before)
      assert.deepEqual(history.at(-1), { noHistory: true })
    }
    assert.equal(history.length, 4)
    tree = render({ compact: true })
    elements(tree, node => node.props['aria-haspopup'] === 'menu')[0].props.onClick(event)
    tree = render({ compact: true }); stops = 0; prevents = 0
    tree.props.onKeyDown({ ...event, key: 'Escape' })
    assert.equal(stops, 1); assert.equal(prevents, 1); assert.equal(menuOpen, false)
  })
}


const superdocTargets = [
  'node_modules/superdoc/dist/chunks/create-super-doc-ui-8UTu16Lw.es.js',
  'node_modules/superdoc/dist/chunks/create-super-doc-ui-bTFGKVRb.cjs'
]
function beforeSuperdocPatch(source) {
  const line = '\t\t\t\tif (result.ok) selectionEpoch += 1;\n'
  assert.equal(source.split(line).length, 2)
  return source.replace(line, '')
}
test('a newly reviewed package can be patched beside complete existing postimages', t => {
  const f = fixture(t), postimages = superdocTargets.map(path => readFileSync(join(f.root, path), 'utf8'))
  superdocTargets.forEach((path, index) => writeFileSync(join(f.root, path), beforeSuperdocPatch(postimages[index])))
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
  let applications = 0
  const result = applyDshPreviewPatches(f.root, (command, args, options) => {
    applications++
    assert.equal(command, process.execPath)
    assert.deepEqual(args, [join(f.root, 'node_modules/patch-package/index.js'), '--patch-dir', 'patches', '--error-on-fail'])
    assert.equal(options.cwd, f.root)
    superdocTargets.forEach((path, index) => writeFileSync(join(f.root, path), postimages[index]))
    return { status: 0 }
  })
  assert.equal(applications, 1); assert.equal(result.status, 'applied')
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), true)
})
test('a partial package remains refused even beside complete existing postimages', t => {
  const f = fixture(t), path = join(f.root, superdocTargets[0])
  writeFileSync(path, beforeSuperdocPatch(readFileSync(path, 'utf8')))
  assert.equal(requiredDshPreviewPatchesCurrent(f.root), false)
  assert.throws(() => applyDshPreviewPatches(f.root, () => { throw new Error('Unexpected package execution') }), /Partial or unexpected/)
})

// Run the installed public controller; only the host and asynchronous Document API are fixtures.
const superdocFactories = [['ESM', createSuperDocUI], ['CJS', require('superdoc/ui').createSuperDocUI]]
const restoredTarget = { kind: 'selection', coordinateSpace: 'visible',
  start: { kind: 'text', blockId: 'fixture-block', offset: 0 },
  end: { kind: 'text', blockId: 'fixture-block', offset: 16 } }
const restoredInfo = { empty: false, text: '1234567890abcdef', target: null, selectionTarget: restoredTarget,
  activeMarks: [], activeCommentIds: [], activeChangeIds: [] }
const emptyInfo = { ...restoredInfo, empty: true, text: '', selectionTarget: null }
function superdocFixture(t, createUI) {
  const dom = new EventTarget(), handlers = new Map(), reads = [], applies = [], observations = []
  let read = () => restoredInfo, applyResult = { ok: true }, release
  const pending = new Promise(resolve => { release = resolve })
  const current = options => { reads.push(options); return read() }
  const editor = { editorVersion: 2, isDirty: false, options: { documentMode: 'viewing' },
    mount: { container: { ownerDocument: dom } }, doc: { selection: { current } },
    host: { getHandles: () => ({ selection: { apply: target => {
      applies.push(target)
      if (applyResult.ok) read = () => pending
      return applyResult
    } } }) } }
  const superdoc = { activeEditor: editor, config: { documentMode: 'viewing' },
    on(event, listener) { const set = handlers.get(event) ?? new Set(); set.add(listener); handlers.set(event, set) },
    off(event, listener) { handlers.get(event)?.delete(listener) } }
  const ui = createUI({ superdoc })
  assert.equal(ui.selection.getSnapshot().status, 'ready')
  const capture = ui.selection.capture()
  assert.equal(capture.quotedText, restoredInfo.text)
  read = () => emptyInfo
  dom.dispatchEvent(new Event('selectionchange'))
  assert.equal(ui.selection.getSnapshot().status, 'ready'); assert.equal(ui.selection.getSnapshot().empty, true)
  const detach = ui.selection.observe(snapshot => observations.push(snapshot))
  t.after(() => { detach(); ui.destroy() })
  return { ui, capture, superdoc, reads, applies, observations, release,
    refuse() { applyResult = { ok: false, reason: 'target-unresolved' } },
    changeOwner(present) {
      superdoc.activeEditor = present ? { ...editor, doc: { selection: { current: () => emptyInfo } } } : null
      for (const listener of handlers.get('active-editor-change') ?? []) listener()
    } }
}
const drainSuperdocRead = () => new Promise(resolve => setImmediate(resolve))
for (const [format, createUI] of superdocFactories) {
  test(`SuperDoc ${format} successful viewing restore publishes fresh asynchronous target and text`, async t => {
    const f = superdocFixture(t, createUI), before = f.reads.length
    assert.deepEqual(f.ui.selection.restore(f.capture), { ok: true, success: true })
    assert.deepEqual(f.applies, [restoredTarget])
    assert.equal(f.reads.length, before + 1, 'successful restore must invalidate the real controller selection cache')
    assert.equal(f.ui.selection.getSnapshot().empty, true)
    assert.equal(f.ui.selection.getSnapshot().status, 'stale')
    assert.equal(f.observations.some(snapshot => snapshot.status === 'ready' && !snapshot.empty), false)
    f.release(restoredInfo); await drainSuperdocRead()
    const actual = f.ui.selection.getSnapshot()
    assert.equal(actual.status, 'ready'); assert.equal(actual.empty, false)
    assert.equal(actual.quotedText, f.capture.quotedText); assert.deepEqual(actual.selectionTarget, f.capture.selectionTarget)
    assert.deepEqual(f.reads.at(-1), { includeText: true })
    assert.equal(f.observations.some(snapshot => snapshot.status === 'ready' && snapshot.quotedText === restoredInfo.text && !snapshot.empty), true)
  })
  test(`SuperDoc ${format} refused restore does not fabricate a selection or refresh success`, async t => {
    const f = superdocFixture(t, createUI), before = f.reads.length; f.refuse()
    assert.deepEqual(f.ui.selection.restore(f.capture), { ok: false, reason: 'target-unresolved', success: false })
    await drainSuperdocRead()
    assert.equal(f.reads.length, before); assert.equal(f.ui.selection.getSnapshot().status, 'ready')
    assert.equal(f.ui.selection.getSnapshot().empty, true)
    assert.equal(f.observations.some(snapshot => !snapshot.empty), false)
  })
  for (const present of [false, true]) test(`SuperDoc ${format} ignores restored text after active owner ${present ? 'changes' : 'closes'}`, async t => {
    const f = superdocFixture(t, createUI), before = f.reads.length
    assert.equal(f.ui.selection.restore(f.capture).ok, true)
    assert.equal(f.reads.length, before + 1)
    f.observations.length = 0; f.changeOwner(present)
    f.release(restoredInfo); await drainSuperdocRead()
    assert.equal(f.ui.selection.getSnapshot().empty, true)
    assert.equal(f.observations.some(snapshot => !snapshot.empty || snapshot.quotedText === restoredInfo.text), false)
  })
}
