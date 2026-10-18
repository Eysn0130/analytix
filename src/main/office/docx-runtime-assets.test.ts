import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, expect, it, vi } from 'vitest'
import { docxRuntimeAssetsPlugin } from '../../../electron.vite.config'

vi.mock('electron-vite',()=>({defineConfig:(config:unknown)=>config,externalizeDepsPlugin:()=>({name:'fixture-externalize'})}))
vi.mock('@vitejs/plugin-react',()=>({default:()=>({name:'fixture-react'})}))

type Middleware=(req:{url?:string},res:{setHeader:(name:string,value:string)=>void;end:(bytes:Buffer)=>void},next:()=>void)=>void
type Asset={type:'asset';fileName:string;source:Buffer}
type Hooks={
  config:()=>{define:{__ANALYTIX_DOCX_WORKER_PATHS__:string}}
  configureServer:(server:{middlewares:{use:(middleware:Middleware)=>void}})=>void
  generateBundle:(this:{emitFile:(asset:Asset)=>void})=>void
}
const entries={document:'browser-worker-entry-new123.js',collaboration:'collaboration-worker-entry-new456.js',reviewIndex:'review-index-worker-entry-new789.js'}
const roots:string[]=[]
function fixture(){const root=mkdtempSync(join(tmpdir(),'analytix-docx-assets-'));roots.push(root);return root}
function put(root:string,path:string,value=path){mkdirSync(dirname(join(root,path)),{recursive:true});writeFileSync(join(root,path),value)}
function hooks(root:string){return docxRuntimeAssetsPlugin(root) as unknown as Hooks}
function workers(root:string){for(const entry of Object.values(entries))put(root,'assets/'+entry)}
afterEach(()=>{for(const root of roots.splice(0))rmSync(root,{recursive:true,force:true})})

it('defines the three unique entry paths from the copied catalog, independent of package hashes',()=>{
  const root=fixture();workers(root)
  expect(JSON.parse(hooks(root).config().define.__ANALYTIX_DOCX_WORKER_PATHS__)).toEqual(Object.fromEntries(
    Object.entries(entries).map(([role,entry])=>[role,'superdoc-engine/assets/'+entry])))
})

it.each(Object.keys(entries) as Array<keyof typeof entries>)('fails clearly when the %s worker is missing',role=>{
  const root=fixture()
  for(const [key,entry] of Object.entries(entries))if(key!==role)put(root,'assets/'+entry)
  put(root,'assets/'+entries[role]+'.map');put(root,'assets/not-'+entries[role])
  put(root,'assets/'+entries[role].replace('.js','')+'/nested.js')
  expect(()=>hooks(root).config()).toThrow(`DOCX ${role} worker requires exactly one ${entries[role].split('new')[0]}*.js entry; found 0.`)
})
it.each(Object.keys(entries) as Array<keyof typeof entries>)('fails clearly when the %s worker is duplicated',role=>{
  const root=fixture();workers(root);put(root,'assets/'+entries[role].replace('new','another'))
  expect(()=>hooks(root).config()).toThrow(`DOCX ${role} worker requires exactly one ${entries[role].split('new')[0]}*.js entry; found 2.`)
})

it('serves and emits the same frozen graph including shared dependencies, while excluding maps and declarations',()=>{
  const root=fixture();workers(root)
  const extra={'assets/shared.js':'export const fixture=1','assets/font.woff2':'fixture-font','style.css':'body{}'}
  for(const [path,body] of Object.entries(extra))put(root,path,body)
  for(const path of ['assets/worker.js.map','index.d.ts','index.d.cts'])put(root,path)
  const plugin=hooks(root),paths=JSON.parse(plugin.config().define.__ANALYTIX_DOCX_WORKER_PATHS__) as Record<string,string>
  // All hooks share the catalog established by config(), rather than rescanning independently.
  put(root,'assets/'+entries.document.replace('new','late'))
  let middleware!:Middleware
  plugin.configureServer({middlewares:{use:value=>{middleware=value}}})
  const emitted:Asset[]=[]
  plugin.generateBundle.call({emitFile:asset=>{emitted.push(asset)}})
  const expected:Record<string,string>={}
  for(const entry of Object.values(entries))expected['superdoc-engine/assets/'+entry]='assets/'+entry
  for(const [path,body] of Object.entries(extra))expected['superdoc-engine/'+path]=body
  expect(Object.fromEntries(emitted.map(asset=>[asset.fileName,asset.source.toString()]))).toEqual(expected)
  for(const [path,body] of Object.entries(expected)){
    const setHeader=vi.fn(),end=vi.fn(),next=vi.fn()
    middleware({url:'/'+path+'?fixture=1'},{setHeader,end},next)
    expect(end).toHaveBeenCalledWith(Buffer.from(body));expect(next).not.toHaveBeenCalled()
    if(Object.values(paths).includes(path))expect(setHeader).toHaveBeenCalledWith('Content-Type','application/javascript; charset=utf-8')
  }
  expect(Object.values(paths).every(path=>emitted.some(asset=>asset.fileName===path))).toBe(true)
  const next=vi.fn(),end=vi.fn()
  middleware({url:'/superdoc-engine/missing.js'},{setHeader:vi.fn(),end},next)
  expect(next).toHaveBeenCalledOnce();expect(end).not.toHaveBeenCalled()
})
