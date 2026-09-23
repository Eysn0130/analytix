#!/usr/bin/env node

const { existsSync, lstatSync, realpathSync } = require('node:fs')
const { createRequire } = require('node:module')
const { delimiter, dirname, isAbsolute, join } = require('node:path')

async function main() {
  const [zip, blockmap] = process.argv.slice(2)
  if (!isAbsolute(zip) || !isAbsolute(blockmap)) throw Error('core_update_blockmap_path_invalid')
  const builderBin = process.env.PATH?.split(delimiter)
    .map(directory => join(directory, 'electron-builder'))
    .find(existsSync)
  if (!builderBin) throw Error('core_update_builder_unavailable')
  const builderEntry = realpathSync(builderBin)
  const builder = require(join(dirname(builderEntry), 'package.json'))
  if (builder.version !== '26.15.3') throw Error('core_update_builder_version_mismatch')
  const { buildBlockMap } = createRequire(builderEntry)(
    'app-builder-lib/out/targets/blockmap/blockmap.js'
  )
  const result = await buildBlockMap(zip, 'gzip', blockmap)
  const stat = lstatSync(blockmap)
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 || stat.size === 0 ||
    !Number.isSafeInteger(result.size) || !/^[A-Za-z0-9+/]{86}==$/.test(result.sha512)) {
    throw Error('core_update_blockmap_invalid')
  }
  process.stdout.write(`${JSON.stringify(result)}\n`)
}

main().catch(error => {
  console.error(error.message)
  process.exitCode = 1
})
