import { randomUUID } from 'node:crypto'
import { isAbsolute, join, resolve } from 'node:path'
import { officePrivateAdmissionPath, officePrivateAdmissionRequestSchema, officePrivateAdmissionResponseSchema } from '../../../packages/runtime/src/contracts/office-private-admission'
import { loadPrivateOfficeBuffers, officeAssetFailure, type PrivateOfficeBuffers } from './office-assets'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'

type Transport = (path:string,body:string)=>Promise<{ok:boolean;status:number;body:string}>
// Object identity, not a caller-provided flag or structural TypeScript cast,
// proves that a token came from this Main-owned Core transport.
declare const admissionBrand: unique symbol
export type OfficePrivateAdmission = Readonly<{[admissionBrand]:true}>
const witnesses = new WeakMap<OfficePrivateAdmission,{root:string;digest:string;query:()=>Promise<{root:string;qualificationDigest:string}>}>()
export function isOfficePrivateAdmission(value:unknown): value is OfficePrivateAdmission {
  return !!value && typeof value==='object' && witnesses.has(value as OfficePrivateAdmission)
}
export function createOfficePrivateAdmissionProvider(transport:Transport,resourcesPath:string=process.resourcesPath) {
  const root=typeof resourcesPath==='string'&&isAbsolute(resourcesPath)&&resolve(resourcesPath)===resourcesPath?join(resourcesPath,'office-private'):''
  const query=async()=>{
    try {
      if(!root)throw officeAssetFailure()
      const request=officePrivateAdmissionRequestSchema.parse({requestId:randomUUID()})
      const response=await transport(officePrivateAdmissionPath,JSON.stringify(request))
      if(!response.ok || response.status!==200 || Buffer.byteLength(response.body)>64*1024)throw officeAssetFailure()
      const value=officePrivateAdmissionResponseSchema.parse(parseStrictJsonObject(Buffer.from(response.body),{maxBytes:64*1024,maxDepth:4,maxTokens:32,maxStringBytes:32768,maxNumberBytes:16}))
      if(!value.ok || value.requestId!==request.requestId || value.root!==root)throw officeAssetFailure()
      return {root:value.root,qualificationDigest:value.qualificationDigest}
    }catch{throw officeAssetFailure()}
  }
  return async():Promise<OfficePrivateAdmission>=>{
    const current=await query(), token=Object.freeze({}) as OfficePrivateAdmission
    witnesses.set(token,{root:current.root,digest:current.qualificationDigest,query})
    return token
  }
}
/** Every surface rechecks the current Core before and after loading held bytes. */
export async function loadAdmittedOfficeBuffers(admission:OfficePrivateAdmission):Promise<PrivateOfficeBuffers>{
  const witness=witnesses.get(admission)
  if(!witness)throw officeAssetFailure()
  let loaded:PrivateOfficeBuffers|undefined
  try{
    const before=await witness.query()
    if(before.root!==witness.root || before.qualificationDigest!==witness.digest)throw officeAssetFailure()
    loaded=await loadPrivateOfficeBuffers(witness.root,witness.digest)
    const after=await witness.query()
    if(after.root!==witness.root || after.qualificationDigest!==witness.digest)throw officeAssetFailure()
    return loaded
  }catch{loaded?.buffers.clear();throw officeAssetFailure()}
}
