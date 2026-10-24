// Exact dependency patches; source preparation only until normal npm qualification.
const { createHash } = require('node:crypto')
const { readFileSync, lstatSync, readdirSync } = require('node:fs')
const { resolve, join } = require('node:path')
const { spawnSync } = require('node:child_process')

const patchPackageVersion = '8.0.1'
const requiredPatches = [
  {
    "package": "@fortune-sheet/core",
    "version": "1.0.4",
    "path": "patches/@fortune-sheet+core+1.0.4.patch",
    "sha256": "4580a4e2d5e116b6df0ed8654ab6495751a49104a459eea8f9027ec35537fc7d",
    "targets": [
      {
        "path": "dist/index.esm.js",
        "beforeSha256": "579fa2bcc2797c0f8ea1c588f610345fc94a72b824ba473bfbd9d4536512e8fe",
        "afterSha256": "050cebc5b6f51ea96f4528c1e1266cf1b83393bc08a676f991bc5d263d5185d1"
      },
      {
        "path": "dist/index.js",
        "beforeSha256": "39f929eae09794138c32385f2b9aedb3687e69d9abaa6c91fd577ebb55283cd8",
        "afterSha256": "4ee19ef511a10bb73feaf7324308282e1f12e1f0241fed416db621dc8812c106"
      },
      {
        "path": "dist/settings.d.ts",
        "beforeSha256": "4c7ed609238e1994f6c1b635c261704c9a4cbe2d666e5fe79404e8a3c7d95348",
        "afterSha256": "bf93adb38cc88b65fa7e1ad80ec65469b8723301555bd2e90f31eab0dc271aef"
      },
      {
        "path": "dist/context.d.ts",
        "beforeSha256": "e762241508fc4c87711c370cfe697497e3d0509f091341ddfe2a00bb69f2a5f0",
        "afterSha256": "810c7933e4796458f61e67d19581ec973386ec6cb2527c8ba221924e569d850d"
      },
      {
        "path": "dist/canvas.d.ts",
        "beforeSha256": "9b5a71a0d1bd3101d50709a74ce934476f6b45edfdf17e5356f69c140863e185",
        "afterSha256": "d6c10610cc2e6422df270421f8cb6ddff963e4dc399a317670b59822571bfd39"
      }
    ]
  },
  {
    "package": "@fortune-sheet/react",
    "version": "1.0.4",
    "path": "patches/@fortune-sheet+react+1.0.4.patch",
    "sha256": "c6d8f2bd7001c7a176b8824c563c414c8230cf4f2ef56fbebc53930ec7f8601a",
    "targets": [
      {
        "path": "dist/index.esm.js",
        "beforeSha256": "2f20fa295a69e2de4e4a1dd89415e71e94e6e9e7fae2ee60506b2bd4f558d940",
        "afterSha256": "d2ac41600b12f355e94ba475b2558acb3c1d6e183aeedc71c0531e0220bf1647"
      },
      {
        "path": "dist/index.js",
        "beforeSha256": "ee032539387860831092d8eb7ca7075d5096e4d0863c983b9540b5e40491240e",
        "afterSha256": "36c429297e9d87d4ad886ad6574a10c143a38fba84ebe4a80dbf841cdb1beadc"
      },
      {
        "path": "dist/components/Workbook/index.d.ts",
        "beforeSha256": "1f024b1b5bfc8bcad8d47017e5b119df6949ff75ef1b05922d2f66c1519ac451",
        "afterSha256": "809043e8feadc893cc9fd8accbe5429ee9b743c502a40a73352128ab4d2da7b0"
      },
      {
        "path": "dist/components/SheetTab/index.d.ts",
        "beforeSha256": "d16ab34c953fb15638b217d16e99264ad2c0cffbf3dddbe8c7c429ccb4bb7bce",
        "afterSha256": "7b93e74147ee4e3cd4d1d0112d76695e1ef9939438a785fe580bd64608d445dd"
      }
    ]
  },
  {
    "package": "exceljs",
    "version": "4.4.0",
    "path": "patches/exceljs+4.4.0.patch",
    "sha256": "64e04cc4e93b336d5de03c4fb86d9f4d2957530c92165627f23687d507b45b9e",
    "targets": [
      {
        "path": "dist/exceljs.js",
        "beforeSha256": "217ea0c8d055db909ca8b6d44b0c8dfef2a95f07010c634af88aa61202e4f08b",
        "afterSha256": "6cab0f80971d6131da4903e5323d7e064a29660a8dabf34e7c4d3b4e410fa73b"
      },
      {
        "path": "lib/utils/parse-sax.js",
        "beforeSha256": "14c05f86bc7eb0a698ad263b479ccaa53335821b9c082bb977fca980a93b1b27",
        "afterSha256": "69f96f1d12c4e3c7f321a9e730006f163e9e459c503907bd565f72e874ce5c4c"
      },
      {
        "path": "lib/xlsx/xform/book/workbook-xform.js",
        "beforeSha256": "8e18451fffd30ca63b4dd08090fad60c923bee945dd6fac44efb1bdb86fe57f0",
        "afterSha256": "a0e72c003f148e9bf75532d819468d1304170d5a19bc519871ce7945dc8790e3"
      },
      {
        "path": "lib/xlsx/xform/comment/comment-xform.js",
        "beforeSha256": "05b315bcdb768c8b6998a418bd25dfc51ea893220db70651f1d2614e786c4de2",
        "afterSha256": "75f52fed2e4f78ea1da6313764b93db96a87a77db82e1302c3f2b4a0517efeba"
      },
      {
        "path": "lib/xlsx/xform/sheet/worksheet-xform.js",
        "beforeSha256": "d95a66d9febb6f5d79eff3e6b7f9cb3115bb2615063dabc120de7ec6ec97b950",
        "afterSha256": "64e9d8b9d0cc3e3584adafd7717342711556d0be3c8c76bdf9e6c80fdfe8472b"
      },
      {
        "path": "lib/xlsx/xlsx.js",
        "beforeSha256": "0766abd994d57c2da21b9ebd3326bf5772ddc74f1d65e7dcf942a0b0a39516d5",
        "afterSha256": "74bd8f011291420ffacdf2072a9f674efc1d7910b4394481eda65d516d372f55"
      },
      {
        "path": "package.json",
        "beforeSha256": "3f81d65b7244dbd7407c9d6d706877c123c2740f4667f09e059148c898455486",
        "afterSha256": "7d1321861ab2da17de8435457d2e4375b44fbb8c825b02f460564f10823e21ca"
      }
    ]
  },
  {
    "package": "superdoc",
    "version": "2.24.0",
    "path": "patches/superdoc+2.24.0.patch",
    "sha256": "42c6f70cf70709ceccd1077eac4461dc2c6ffec77fe1ae1520a71fb644ac8b07",
    "targets": [
      {
        "path": "dist/chunks/create-super-doc-ui-8UTu16Lw.es.js",
        "beforeSha256": "35e4d9dc24d9e2424b8ecd40066ebb6919a88f8a430c0d4f8e74fcbe37aa83a4",
        "afterSha256": "29c9fbe6fe70ff8d82c6bad250428043d75d08ef8938d2a01c643f5cbed8400e"
      },
      {
        "path": "dist/chunks/create-super-doc-ui-bTFGKVRb.cjs",
        "beforeSha256": "93133039799bfdb19d1158355ec7066d847f51350b01ac274f5cc3de6437b3c1",
        "afterSha256": "646d6d0d887f7277f4c8ec73bf7958fb63b1b2e257ebf59a1f71f9a69b48c11b"
      }
    ]
  }
]
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')
const readJson = path => JSON.parse(readFileSync(path, 'utf8'))

function patchesRequired(root) {
  const manifest = readJson(join(root, 'package.json'))
  const declared = { ...manifest.dependencies, ...manifest.devDependencies }
  if (!requiredPatches.some(patch => declared[patch.package] !== undefined)) return false
  for (const patch of requiredPatches) {
    if (declared[patch.package] !== patch.version) throw new Error('[dsh-preview-patches] All reviewed dependency versions must be declared together.')
  }
  if (declared['patch-package'] !== patchPackageVersion) throw new Error('[dsh-preview-patches] Local patch-package 8.0.1 must be declared.')
  return true
}

function patchInputs(root) {
  const directory = join(root, 'patches')
  if (!lstatSync(directory).isDirectory()) throw new Error('[dsh-preview-patches] The patch directory must be an ordinary source directory.')
  const expectedNames = requiredPatches.map(patch => patch.path.slice('patches/'.length)).sort()
  if (JSON.stringify(readdirSync(directory).sort()) !== JSON.stringify(expectedNames)) {
    throw new Error('[dsh-preview-patches] The patch directory must contain exactly the reviewed patches.')
  }
  const entries = []
  for (const patch of requiredPatches) {
    const path = join(root, patch.path)
    if (!lstatSync(path).isFile()) throw new Error('[dsh-preview-patches] Required patches must be ordinary source files.')
    const bytes = readFileSync(path)
    if (sha256(bytes) !== patch.sha256) throw new Error('[dsh-preview-patches] Required patch content differs from the reviewed source.')
    entries.push({ path: patch.path, bytes })
  }
  return entries
}

function dshPreviewPatchInputDigest(root) {
  const hash = createHash('sha256')
  if (!patchesRequired(root)) return hash.update('dsh-preview-patches:not-required').digest('hex')
  for (const entry of patchInputs(root)) hash.update(entry.path).update('\0').update(entry.bytes)
  return hash.digest('hex')
}

function installedVersions(root) {
  for (const patch of requiredPatches) {
    if (readJson(join(root, 'node_modules', patch.package, 'package.json')).version !== patch.version) {
      throw new Error('[dsh-preview-patches] Installed dependency version differs from the reviewed version.')
    }
  }
  const tool = readJson(join(root, 'node_modules', 'patch-package', 'package.json'))
  // npm accepts a single bin string and may normalize a named bin path.
  // All supported metadata forms still resolve to the fixed local index.js.
  const bin = typeof tool.bin === 'string' ? tool.bin
    : tool.bin && Object.keys(tool.bin).length === 1 ? tool.bin['patch-package'] : undefined
  if (tool.version !== patchPackageVersion || !['index.js', './index.js'].includes(bin)) {
    throw new Error('[dsh-preview-patches] The reviewed local patch tool is unavailable.')
  }
}

function targetState(root) {
  return requiredPatches.flatMap(patch => patch.targets.map(target => {
    const bytes = readFileSync(join(root, 'node_modules', patch.package, target.path))
    const hash = sha256(bytes)
    return { package: patch.package, before: hash === target.beforeSha256, after: hash === target.afterSha256 }
  }))
}

// Read-only qualification. Importing this module never invokes the patch tool.
function requiredDshPreviewPatchesCurrent(root) {
  try {
    if (!patchesRequired(root)) return true
    patchInputs(root)
    installedVersions(root)
    return targetState(root).every(target => target.after)
  } catch { return false }
}

function applyDshPreviewPatches(root = resolve(__dirname, '..'), run = spawnSync) {
  if (!patchesRequired(root)) return { status: 'not_required' }
  patchInputs(root)
  installedVersions(root)
  const state = targetState(root)
  if (state.every(target => target.after)) return { status: 'already_applied' }
  if (!requiredPatches.every(patch => {
    const targets = state.filter(target => target.package === patch.package)
    return targets.every(target => target.before) || targets.every(target => target.after)
  })) {
    throw new Error('[dsh-preview-patches] Partial or unexpected package contents require a fresh normal installation.')
  }
  const result = run(process.execPath, [join(root, 'node_modules', 'patch-package', 'index.js'),
    '--patch-dir', 'patches', '--error-on-fail'], { cwd: root, stdio: 'inherit' })
  if (result.error || result.signal || result.status !== 0) throw new Error('[dsh-preview-patches] Required package patch application failed.')
  if (!requiredDshPreviewPatchesCurrent(root)) throw new Error('[dsh-preview-patches] Required patch postimages were not produced.')
  return { status: 'applied' }
}

module.exports = { dshPreviewPatchInputDigest, requiredDshPreviewPatchesCurrent, applyDshPreviewPatches }
if (require.main === module) {
  try { console.log('[dsh-preview-patches] ' + applyDshPreviewPatches().status) }
  catch (error) { console.error(error.message); process.exitCode = 1 }
}
