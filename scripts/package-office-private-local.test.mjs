import test from 'node:test'
import assert from 'node:assert/strict'
import {privateOfficePackagePlan} from './package-office-private-local.mjs'
const options={repoRoot:'/repo',assetRoot:'/assets',output:'/task/candidate',platform:'darwin',arch:'arm64',env:{}}
test('private local packaging has explicit isolated scope and never publishes',()=>{
 const result=privateOfficePackagePlan(options)
 assert.equal(result.env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE,'isolated-local-v1')
 assert.equal(result.env.CSC_IDENTITY_AUTO_DISCOVERY,'false')
 assert.equal(result.env.ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT,'/assets')
 const args=result.commands[2][1];assert.equal(args[args.indexOf('--publish')+1],'never')
 assert.ok(args.includes('--arm64'));assert.ok(args.includes('--config.directories.output=/task/candidate'))
 assert.deepEqual(result.commands[0],['npm',['run','build:data-native:development','--','--platform','darwin','--arch','arm64']])
})
test('rejects other platforms, overlapping output and conflicting signing or publication',()=>{
 for(const change of [{platform:'linux'},{arch:'x64'},{output:'/repo/out'},{output:'/assets/out'},{output:'/repo'},{output:'relative'},{output:'/task/../candidate'},
  ...['MAC_SIGN','ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE','ANALYTIX_RELEASE_BUILD','CSC_LINK','APPLE_API_KEY_ID'].map(key=>({env:{[key]:'1'}}))])assert.throws(()=>privateOfficePackagePlan({...options,...change}))
})
