import { spawn } from 'node:child_process'
import { lstatSync, realpathSync } from 'node:fs'
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = realpathSync(resolve(dirname(fileURLToPath(import.meta.url)), '..'))
const overlaps = (parent, child) => { const path=relative(parent,child); return path==='' || !path.startsWith(`..${sep}`) && path!=='..' && !isAbsolute(path) }
export function privateOfficePackagePlan({assetRoot,output,repoRoot=repositoryRoot,platform=process.platform,arch=process.arch,env=process.env}) {
  if (platform!=='darwin' || arch!=='arm64') throw Error('private-office-requires-darwin-arm64')
  for(const path of [assetRoot,output]) if(typeof path!=='string' || !isAbsolute(path) || resolve(path)!==path) throw Error('private-office-canonical-path-required')
  if(overlaps(repoRoot,output) || overlaps(output,repoRoot) || overlaps(assetRoot,output) || overlaps(output,assetRoot)) throw Error('private-office-output-overlap')
  if(env.ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE==='1' || env.ANALYTIX_RELEASE_BUILD==='1' || env.MAC_SIGN==='1' ||
    ['CSC_LINK','CSC_NAME','CSC_KEY_PASSWORD','WIN_CSC_LINK','ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1','APPLE_API_KEY','APPLE_API_KEY_BASE64','APPLE_API_KEY_ID','APPLE_API_ISSUER'].some(key=>Boolean(env[key]))) throw Error('private-office-conflicting-release-intent')
  return {env:{...env,ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE:'isolated-local-v1',ANALYTIX_OFFICE_PRIVATE_LOCAL_BUILD:'1',ANALYTIX_OFFICE_PRIVATE_LOCAL_ASSET_ROOT:assetRoot,CSC_IDENTITY_AUTO_DISCOVERY:'false'},commands:[
    ['npm',['run','build:data-native:development','--','--platform','darwin','--arch','arm64']],
    ['npm',['run','build']],
    ['npx',['--no-install','electron-builder','--config','electron-builder.config.cjs','--publish','never','--mac','dmg','--arm64',`--config.directories.output=${output}`]]
  ]}
}
async function execute(args) {
  if(args.length!==4 || args[0]!=='--asset-root' || args[2]!=='--output') throw Error('usage: --asset-root <absolute-directory> --output <new-absolute-directory>')
  const assetRoot=args[1],output=args[3]
  const plan=privateOfficePackagePlan({assetRoot,output})
  if(realpathSync(assetRoot)!==assetRoot || !lstatSync(assetRoot).isDirectory()) throw Error('private-office-asset-root-invalid')
  if(realpathSync(dirname(output))!==dirname(output)) throw Error('private-office-output-parent-invalid')
  try {lstatSync(output);throw Error('private-office-output-already-exists')} catch(error) {if(error.code!=='ENOENT')throw error}
  for(const [command,argv] of plan.commands) await new Promise((resolveStep,reject)=>{
    const child=spawn(command,argv,{cwd:repositoryRoot,env:plan.env,stdio:'inherit'})
    child.once('error',()=>reject(Error('private-office-build-start-failed')))
    child.once('exit',code=>code===0?resolveStep():reject(Error('private-office-build-step-failed')))
  })
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  execute(process.argv.slice(2)).catch(error=>{console.error(error.message);process.exitCode=1})
}
