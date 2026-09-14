const test = require('node:test')
const assert = require('node:assert/strict')
const { mkdtempSync, mkdirSync, writeFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const { join } = require('node:path')
const { _internals: api } = require('./after-pack.cjs')
const native = {kind:'development_non_publishable'}
const env = {ANALYTIX_OFFICE_PRIVATE_LOCAL_BUILD:'1',ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT:'/assets',ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE:'isolated-local-v1'}
function fixture() {
 const root=mkdtempSync(join(tmpdir(),'office-private-packaging-'))
 const context={appOutDir:root,electronPlatformName:'darwin',arch:'arm64',packager:{appInfo:{productFilename:'Analytix'}}}
 mkdirSync(api.packedResourcesDir(context),{recursive:true})
 return context
}
test('only explicitly isolated private arm64 development packages can stage Office',()=>{
 const context=fixture()
 assert.equal(api.privateOfficeBuildRequested(context,native,{}),false)
 assert.equal(api.privateOfficeBuildRequested(context,native,env),true)
 for(const change of [{ANALYTIX_OFFICE_PRIVATE_LOCAL_BUILD:'0'},{ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT:'relative'},{ANALYTIX_RELEASE_BUILD:'1'},{MAC_SIGN:'1'},{CSC_LINK:'synthetic'},{ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE:undefined}])assert.throws(()=>api.privateOfficeBuildRequested(context,native,{...env,...change}),/scope_invalid/)
 assert.throws(()=>api.privateOfficeBuildRequested({...context,arch:'x64'},native,env),/scope_invalid/)
 assert.throws(()=>api.privateOfficeBuildRequested(context,{kind:'controlled_release'},env),/scope_invalid/)
})
test('ordinary packaging rejects preexisting private payload; private requires qualified payload',()=>{
 const context=fixture(),root=join(api.packedResourcesDir(context),'office-private')
 const snapshot={sourceCommit:'a'.repeat(40),snapshotDigest:'b'.repeat(64)}
 assert.equal(api.verifyPrivateOfficeForPack(context,native,snapshot,{}),undefined)
 assert.throws(()=>api.verifyPrivateOfficeForPack(context,native,snapshot,env),/office-private-local/)
 mkdirSync(root);writeFileSync(join(root,'qualification.json'),'{}')
 assert.throws(()=>api.verifyPrivateOfficeForPack(context,native,snapshot,{}),/not_authorized/)
 assert.throws(()=>api.verifyPrivateOfficeForPack(context,native,snapshot,env),/office-private-local/)
 assert.throws(()=>api.stagePrivateOfficeForPack(context,native,'/repo',snapshot,env),/already_present/)
})
