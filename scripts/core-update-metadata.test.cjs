const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const { join } = require('node:path')
const { tmpdir } = require('node:os')
const { createHash } = require('node:crypto')
const yaml = require('js-yaml')
const hook = require('./core-update-metadata.cjs')

test('Core builder metadata requires exact ZIP/DMG bytes and remains channel bound', async () => {
  const root = fs.mkdtempSync(join(tmpdir(), 'core-update-'))
  try {
    for (const channel of ['stable', 'beta']) {
      const name = channel === 'stable' ? 'latest-mac.yml' : 'beta-mac.yml'
      const bytes = Buffer.from('synthetic archive')
      const repackedBytes = Buffer.from('synthetic signature-preserving archive')
      const files = ['zip', 'dmg'].map(ext => ({url:`analytix-core-1.0.7-mac-arm64.${ext}`,size:bytes.length,sha512:createHash('sha512').update(bytes).digest('base64')}))
      for (const file of files) fs.writeFileSync(join(root, file.url), bytes)
      fs.writeFileSync(join(root,name),yaml.dump({version:'1.0.7', files, path:files[0].url, sha512:files[0].sha512}))
      await hook({outDir:root,configuration:{extraMetadata:{releaseProfile:'core',releaseChannel:channel}}}, (outDir, version) => {
        assert.equal(version, '1.0.7')
        fs.writeFileSync(join(outDir, files[0].url), repackedBytes)
        return {size:repackedBytes.length,sha512:createHash('sha512').update(repackedBytes).digest('base64')}
      })
      const metadata = yaml.load(fs.readFileSync(join(root,name),'utf8'))
      assert.equal(metadata.files[0].size, repackedBytes.length)
      assert.equal(metadata.sha512, metadata.files[0].sha512)
      hook.verifyCoreUpdateArtifacts(root,metadata,{version:'1.0.7',channel})
      assert.throws(()=>hook.verifyCoreUpdateArtifacts(root,metadata,{version:'1.0.7',channel:channel==='stable'?'beta':'stable'}))
      assert.throws(()=>hook.validateCoreUpdateMetadata({...metadata,files:files.slice(1)},{version:'1.0.7',channel}),/zip_required/)
      fs.writeFileSync(join(root,files[0].url),'damaged')
      assert.throws(()=>hook.verifyCoreUpdateArtifacts(root,metadata,{version:'1.0.7',channel}),/digest_mismatch/)
    }
  } finally { fs.rmSync(root,{recursive:true}) }
})
