import { test } from 'node:test'
import assert from 'node:assert/strict'
import { inspectExactArtifactLegalInventory } from './artifact-legal-obligations-audit.mjs'

test('exact package license discovery recognizes shipped upstream filenames without exempting missing text', () => {
  for (const name of ['LICENSE.markdown','LICENSE.BSD','LICENSE.MIT','LICENSE.APACHE2']) {
    const entries = {'node_modules/synthetic/package.json':JSON.stringify({name:'synthetic',version:'1.0.0',license:'MIT'})}
    entries[`node_modules/synthetic/${name}`]='Synthetic test license text'
    const result = inspectExactArtifactLegalInventory({artifact:{entries}})
    const dependency = result.dependencyInstances.find(value=>value.name==='synthetic')
    assert.equal(dependency.licenseFile,`/node_modules/synthetic/${name}`)
    delete entries[`node_modules/synthetic/${name}`]
    const missing = inspectExactArtifactLegalInventory({artifact:{entries}}).dependencyInstances.find(value=>value.name==='synthetic')
    assert.equal(missing.status,'blocked')
  }
})
