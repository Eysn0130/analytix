import { createHash } from 'node:crypto'
import privateLayout from '../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json'
import privateManifest from '../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json'
// Tiny source-owned layout substitutes exercise real filesystem reads without
// loading production engine binaries or claiming a real resource-seal check.
vi.mock('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json',async(importOriginal)=>{
  const {default:actual}=await importOriginal<any>()
  const {createHash}=await import('node:crypto')
  const next=structuredClone(actual)
  for(const asset of next.assets){const bytes=Buffer.from('private-fixture:assets/'+asset.path);asset.bytes=bytes.length;asset.sha256=createHash('sha256').update(bytes).digest('hex')}
  return {default:next}
})
vi.mock('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json',async(importOriginal)=>{
  const {default:actual}=await importOriginal<any>()
  const {default:manifest}=await import('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json')
  const {createHash}=await import('node:crypto')
  const next=structuredClone(actual),raw=Buffer.from(JSON.stringify(manifest,null,2)+'\n')
  next.manifestSha256=createHash('sha256').update(raw).digest('hex')
  for(const file of next.files){
    const asset=manifest.assets.find(asset=>'assets/'+asset.path===file.path)
    if(asset){file.byteLength=asset.bytes;file.sha256=asset.sha256}
    else if(file.owner==='asset'){const bytes=Buffer.from('private-fixture:'+file.path);file.byteLength=bytes.length;file.sha256=createHash('sha256').update(bytes).digest('hex')}
    if(file.path==='manifest.json'){file.byteLength=raw.length;file.sha256=next.manifestSha256}
  }
  return {default:next}
})
import { afterEach, expect, test, vi } from 'vitest'
import { writeFile, unlink, symlink, mkdir, link } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { loadOfficeBuffers, loadPrivateOfficeBuffers, startOfficeAssetServer } from './office-assets'
import { fixture } from './test-fixture'
const cleanups: (() => Promise<void>)[] = []
afterEach(async () => { for (const c of cleanups.splice(0)) await c() })
async function setup() { const f = await fixture(); cleanups.push(f.cleanup); return f }
test('fixed routes serve verified held buffers after backing file changes', async () => {
  const f = await setup(), buffers = await loadOfficeBuffers(f.sourceRoot, f.assetRoot)
  expect(buffers.size).toBe(9)
  await writeFile(join(f.assetRoot, 'soffice.js'), 'mutated after verification')
  const server = await startOfficeAssetServer(buffers, '11111111-1111-1111-1111-111111111111')
  try {
    const response = await fetch(server.entryURL.replace('office-surface.html', 'assets/soffice.js'))
    expect(await response.text()).toBe('synthetic-asset-soffice.js')
    for (const [name, value] of [['cross-origin-opener-policy','same-origin'], ['cross-origin-embedder-policy','require-corp'], ['cross-origin-resource-policy','same-origin']]) expect(response.headers.get(name)).toBe(value)
    expect(response.headers.get('content-security-policy')).toContain("frame-ancestors 'none'")
    for (const url of [server.entryURL + '?x=1', server.origin + '/etc/passwd', server.entryURL.replace('office-surface.html','assets/not-allowed.js')]) expect((await fetch(url)).status).toBe(404)
    expect((await fetch(server.entryURL, { method: 'POST', body: 'no writes' })).status).toBe(404)
    expect((await fetch(server.entryURL, { headers: { Origin: 'https://outside.invalid' } })).status).toBe(404)
    expect(server.permits('https://outside.invalid', 'GET')).toBe(false)
    expect(server.permits(server.entryURL + '#x', 'GET')).toBe(false)
  } finally { server.destroy() }
  expect(buffers.size).toBe(0)
})
test.each(['digest', 'size', 'path', 'extra', 'duplicate', 'mode', 'symlink'])('rejects %s before admitting resources', async mode => {
  const f = await setup()
  if (mode === 'digest') f.manifest.assets[0].sha256 = '0'.repeat(64)
  if (mode === 'size') f.manifest.assets[0].bytes--
  if (mode === 'path') f.manifest.assets[0].path = '../outside'
  if (mode === 'extra') f.manifest.assets.push(f.manifest.assets[0])
  if (mode === 'mode') f.manifest.publishable = true
  if (mode === 'symlink') { const path = join(f.assetRoot, 'soffice.js'); await unlink(path); await symlink(f.preloadPath, path) }
  const raw = JSON.stringify(f.manifest, null, 2) + '\n'
  await writeFile(f.manifestPath, mode === 'duplicate' ? raw.replace('"schemaVersion": 1,', '"schemaVersion": 1,\n  "schemaVersion": 1,') : raw)
  await expect(loadOfficeBuffers(f.sourceRoot, f.assetRoot)).rejects.toThrow('office-assets-unavailable')
})

const digest=(bytes:Buffer|string)=>createHash('sha256').update(bytes).digest('hex')
async function privateFixture(){
  const f=await setup(),root=join(f.root,'Resources','office-private'),files=[]
  for(const entry of privateLayout.files){
    const bytes=entry.path==='manifest.json'?Buffer.from(JSON.stringify(privateManifest,null,2)+'\n'):Buffer.from('private-fixture:'+entry.path)
    await mkdir(dirname(join(root,entry.path)),{recursive:true});await writeFile(join(root,entry.path),bytes)
    files.push({path:entry.path,byteLength:bytes.length,sha256:digest(bytes)})
  }
  const body={schemaVersion:1,contract:'analytix.office-private-local/v1',usage:'private-local',publishable:false,releaseEligible:false,targetKey:'darwin-arm64',sourceCommit:'a'.repeat(40),worktreeSnapshotDigest:'b'.repeat(64),engineBuildId:privateLayout.buildId,engineManifestSha256:privateLayout.manifestSha256,files}
  const qualify=async(value:Record<string,unknown>=body)=>{
    const qualificationDigest=digest(Buffer.concat([Buffer.from('AnalytixOfficePrivateLocalV1\0'),Buffer.from(JSON.stringify(value))]))
    const q={...value,qualificationDigest},raw=JSON.stringify(q)+'\n'
    await writeFile(join(root,'qualification.json'),raw)
    return {qualificationDigest,raw,q}
  }
  return {root,body,qualify,...await qualify()}
}
test('private qualification loads exactly the fixed runtime buffers and verifies the external preload bytes',async()=>{
  const f=await privateFixture(),loaded=await loadPrivateOfficeBuffers(f.root,f.qualificationDigest)
  expect(loaded.buffers.size).toBe(9);expect(loaded.preloadPath).toBe(join(f.root,'preload/office.cjs'))
  expect(loaded.buffers.get('/assets/soffice.js')?.bytes.toString()).toBe('private-fixture:assets/soffice.js')
  expect(loaded.buffers.get('/office-surface.html')?.bytes.toString()).toBe('private-fixture:surface/office-surface.html')
  expect([...loaded.buffers.keys()].some(path=>path.includes('preload')||path.includes('qualification')||path.includes('plugins'))).toBe(false)
  await writeFile(join(f.root,'preload/office.cjs'),'corrupt preload')
  await expect(loadPrivateOfficeBuffers(f.root,f.qualificationDigest)).rejects.toThrow('office-assets-unavailable')
})
test.each(['unknown','reordered','duplicate-key','missing-file','duplicate-file','external-path','wrong-build','wrong-target','publishable','wrong-pinned-hash','core-digest','symlink','hardlink','surface-corruption','oversize'])('private resource admission rejects %s',async(mode)=>{
  const f=await privateFixture();let expected=f.qualificationDigest
  const body=structuredClone(f.body)
  if(mode==='unknown')Object.assign(body,{unknown:true})
  if(mode==='reordered'){const {schemaVersion,...rest}=body;Object.assign(body,rest);const result=await f.qualify({...rest,schemaVersion});expected=result.qualificationDigest}
  if(mode==='missing-file')body.files.pop()
  if(mode==='duplicate-file')body.files[1]={...body.files[0]}
  if(mode==='external-path')body.files[0].path='../outside'
  if(mode==='wrong-build')body.engineBuildId='0'.repeat(40)
  if(mode==='wrong-target')body.targetKey='darwin-amd64'
  if(mode==='publishable')body.publishable=true
  if(mode==='wrong-pinned-hash')body.files[0].sha256='0'.repeat(64)
  if(['unknown','missing-file','duplicate-file','external-path','wrong-build','wrong-target','publishable','wrong-pinned-hash'].includes(mode))expected=(await f.qualify(body)).qualificationDigest
  if(mode==='duplicate-key')await writeFile(join(f.root,'qualification.json'),f.raw.replace('"schemaVersion":1','"schemaVersion":1,"schemaVersion":1'))
  if(mode==='core-digest')expected='0'.repeat(64)
  if(mode==='oversize')await writeFile(join(f.root,'qualification.json'),' '.repeat(65537))
  const surface=join(f.root,'surface/office-surface.js')
  if(mode==='surface-corruption')await writeFile(surface,'corrupted')
  if(mode==='symlink'){await unlink(surface);await symlink(join(f.root,'preload/office.cjs'),surface)}
  if(mode==='hardlink')await link(surface,join(f.root,'alias'))
  await expect(loadPrivateOfficeBuffers(f.root,expected)).rejects.toThrow('office-assets-unavailable')
})

test('production stager qualification golden is accepted by the Main parser with identical field casing and domain digest',async()=>{
  const {createRequire}=await import('node:module'), require=createRequire(import.meta.url)
  const {validateQualification}=require('../../../scripts/office-private-local-contract.cjs')
  const actualLayout=require('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json')
  const files=actualLayout.files.map((file:any)=>({path:file.path,byteLength:file.byteLength??1,sha256:file.sha256??'c'.repeat(64)}))
  const body={schemaVersion:1,contract:'analytix.office-private-local/v1',usage:'private-local',publishable:false,releaseEligible:false,targetKey:'darwin-arm64',sourceCommit:'a'.repeat(40),worktreeSnapshotDigest:'b'.repeat(64),engineBuildId:actualLayout.buildId,engineManifestSha256:actualLayout.manifestSha256,files}
  const qualificationDigest=digest(Buffer.concat([Buffer.from('AnalytixOfficePrivateLocalV1\0'),Buffer.from(JSON.stringify(body))]))
  const raw=Buffer.from(JSON.stringify({...body,qualificationDigest})+'\n')
  expect(validateQualification(raw).qualificationDigest).toBe(qualificationDigest)
  vi.doUnmock('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/private-local-layout.json')
  vi.doUnmock('../../../packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json')
  vi.resetModules()
  const {parsePrivateOfficeQualification}=await import('./office-assets')
  expect([...parsePrivateOfficeQualification(raw,qualificationDigest).values()]).toEqual(files)
  const wrongCase=Buffer.from(raw.toString().replace('engineManifestSha256','engineManifestSHA256'))
  expect(()=>validateQualification(wrongCase)).toThrow()
  expect(()=>parsePrivateOfficeQualification(wrongCase,qualificationDigest)).toThrow('office-assets-unavailable')
})
