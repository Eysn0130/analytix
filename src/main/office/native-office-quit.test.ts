import { EventEmitter } from 'node:events'
import { createWriteShutdownCoordinator } from '../write-shutdown'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { expect, test, vi } from 'vitest'

// Execute the production callback without importing Electron startup, live
// runtime initialization, credentials or unrelated process lifecycle hooks.
const source=ts.createSourceFile('index.ts',readFileSync(new URL('../index.ts',import.meta.url),'utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.TS)
const callbacks:ts.Expression[]=[]
function visit(node:ts.Node){
  if(ts.isCallExpression(node)&&ts.isPropertyAccessExpression(node.expression)&&ts.isIdentifier(node.expression.expression)&&node.expression.expression.text==='app'&&node.expression.name.text==='on'&&ts.isStringLiteral(node.arguments[0])&&node.arguments[0].text==='before-quit')callbacks.push(node.arguments[1])
  ts.forEachChild(node,visit)
}
visit(source)
function setup(){
  expect(callbacks).toHaveLength(1)
  const trace:string[]=[]
  const context={isQuitting:false,managedRuntimesStoppedForQuit:false,nativeOfficeQuitPending:false,desktopStartupBarrierComplete:true,
    prepareNativeOfficeQuit:vi.fn(async()=>{trace.push('prepare');return true}),
    cancelNativeOfficeQuit:vi.fn(async()=>{trace.push('cancel')}),
    stopRuntimeWatchdog:vi.fn(()=>trace.push('watchdog-stop')),
    startRuntimeWatchdog:vi.fn(()=>trace.push('watchdog-start')),
    stopManagedRuntimesForQuit:vi.fn(async()=>{trace.push('core-stop');context.managedRuntimesStoppedForQuit=true}),
    publicConsoleWarn:vi.fn(),app:{quit:vi.fn(()=>trace.push('app-quit'))},
    beforeQuit:undefined as undefined|((event:{preventDefault:()=>void})=>void)}
  Object.assign(context, { writeShutdown: { prepareQuit: (...args: []) => context.prepareNativeOfficeQuit(...args), cancel: (...args: []) => context.cancelNativeOfficeQuit(...args) } })
  const javascript=ts.transpileModule(`globalThis.beforeQuit = ${callbacks[0].getText(source)}`,{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
  runInNewContext(javascript,context,{timeout:1000})
  const invoke=()=>{const event={preventDefault:vi.fn()};context.beforeQuit!(event);return event}
  return {context,trace,invoke}
}
test('the actual before-quit callback waits for Office preparation and Core stop, and repeated events do not duplicate either',async()=>{
  const h=setup();let prepared!:(value:boolean)=>void,stopped!:()=>void
  h.context.prepareNativeOfficeQuit.mockImplementation(()=>new Promise(resolve=>{h.trace.push('prepare');prepared=resolve}))
  h.context.stopManagedRuntimesForQuit.mockImplementation(()=>new Promise(resolve=>{h.trace.push('core-stop');stopped=()=>{h.context.managedRuntimesStoppedForQuit=true;resolve()}}))
  expect(h.invoke().preventDefault).toHaveBeenCalledOnce();h.invoke()
  expect(h.trace).toEqual(['prepare']);expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
  prepared(true);await vi.waitFor(()=>expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce())
  h.invoke();expect(h.context.prepareNativeOfficeQuit).toHaveBeenCalledOnce();expect(h.context.app.quit).not.toHaveBeenCalled()
  stopped();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(h.trace).toEqual(['prepare','watchdog-stop','core-stop','app-quit'])
  expect(h.invoke().preventDefault).not.toHaveBeenCalled()
  expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce()
})
test('cancelled or failed preparation leaves Core running and permits a later explicit quit attempt',async()=>{
  const h=setup();h.context.prepareNativeOfficeQuit.mockResolvedValue(false)
  h.invoke();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPending).toBe(false))
  expect(h.context.isQuitting).toBe(false);expect(h.context.stopRuntimeWatchdog).not.toHaveBeenCalled()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
  h.context.prepareNativeOfficeQuit.mockResolvedValue(true);h.invoke()
  await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(h.context.prepareNativeOfficeQuit).toHaveBeenCalledTimes(2)
})
test('unexpected preparation rejection thaws Office and restores startup watchdog without stopping Core',async()=>{
  const h=setup();h.context.prepareNativeOfficeQuit.mockRejectedValue(Error('freeze or flush failed'))
  h.invoke();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPending).toBe(false))
  expect(h.context.cancelNativeOfficeQuit).toHaveBeenCalledOnce();expect(h.context.startRuntimeWatchdog).toHaveBeenCalledOnce()
  expect(h.context.isQuitting).toBe(false);expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
})


test('actual createWindow and openThread entry points refuse new renderers while the prepared quit awaits Core stop',async()=>{
  const h=setup(),handlers=new Map<string,(...args:any[])=>any>()
  const main=Object.assign(new EventEmitter(),{isDestroyed:()=>false,close:vi.fn(),
    webContents:Object.assign(new EventEmitter(),{mainFrame:{},send:vi.fn(),isLoadingMainFrame:()=>false,setWindowOpenHandler:vi.fn()})})
  const coordinator=createWriteShutdownCoordinator({ipc:{handle:(name:string,handler:(...args:any[])=>any)=>{handlers.set(name,handler)}} as any,
    prepareNative:async()=>true,cancelNative:async()=>undefined,notifyBlocked:async()=>undefined})
  coordinator.trackWindow(main as any)
  main.webContents.send.mockImplementation((_channel,request)=>handlers.get('write:shutdown-ack')!({sender:main.webContents,senderFrame:main.webContents.mainFrame},{...request,outcome:{result:'ready'}}))
  const constructor=vi.fn(), context=Object.assign(h.context,{writeShutdown:coordinator,BrowserWindow:constructor,create:undefined as undefined|((options?:unknown)=>unknown),open:undefined as undefined|((threadId:string)=>void)})
  const declarations=['createWindow','openThreadInNewWindow'].map(name=>{
    const node=source.statements.find(statement=>ts.isFunctionDeclaration(statement)&&statement.name?.text===name)
    if(!node)throw Error('Missing production window entry point')
    return node.getText(source)
  }).join('\n')
  runInNewContext(ts.transpileModule(declarations+'\nglobalThis.create=createWindow;globalThis.open=openThreadInNewWindow',{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText,context,{timeout:1000})
  let stopped!:()=>void
  h.context.stopManagedRuntimesForQuit.mockImplementation(()=>new Promise(resolve=>{stopped=()=>{h.context.managedRuntimesStoppedForQuit=true;resolve()}}))
  h.invoke();await vi.waitFor(()=>expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce())
  expect(context.create!({primary:false})).toBeNull()
  expect(()=>context.open!('late-thread')).not.toThrow()
  expect(constructor).not.toHaveBeenCalled();expect(main.webContents.send).toHaveBeenCalledOnce()
  stopped();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
})
