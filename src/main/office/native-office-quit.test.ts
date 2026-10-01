import { EventEmitter } from 'node:events'
import { createWriteShutdownCoordinator } from '../write-shutdown'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { afterEach, expect, test, vi } from 'vitest'

// Execute the production callback without importing Electron startup, live
// runtime initialization, credentials or unrelated process lifecycle hooks.
const source=ts.createSourceFile('index.ts',readFileSync(new URL('../index.ts',import.meta.url),'utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.TS)
const callbacks:ts.Expression[]=[]
const willQuitCallbacks:ts.Expression[]=[]
const allClosedCallbacks:ts.Expression[]=[]
function initializer(name:string):ts.Expression{
  for(const statement of source.statements){
    if(!ts.isVariableStatement(statement))continue
    const declaration=statement.declarationList.declarations.find(item=>ts.isIdentifier(item.name)&&item.name.text===name)
    if(declaration?.initializer)return declaration.initializer
  }
  throw Error(`Missing production ${name}`)
}
function visit(node:ts.Node){
  if(ts.isCallExpression(node)&&ts.isPropertyAccessExpression(node.expression)&&ts.isIdentifier(node.expression.expression)&&node.expression.expression.text==='app'&&node.expression.name.text==='on'&&ts.isStringLiteral(node.arguments[0])&&node.arguments[0].text==='before-quit')callbacks.push(node.arguments[1])
  ts.forEachChild(node,visit)
}
visit(source)
for(const statement of source.statements){
  if(!ts.isExpressionStatement(statement)||!ts.isCallExpression(statement.expression))continue
  const call=statement.expression
  if(ts.isPropertyAccessExpression(call.expression)&&ts.isIdentifier(call.expression.expression)&&call.expression.expression.text==='app'&&call.expression.name.text==='on'&&ts.isStringLiteral(call.arguments[0])&&call.arguments[0].text==='will-quit')willQuitCallbacks.push(call.arguments[1])
  if(ts.isPropertyAccessExpression(call.expression)&&ts.isIdentifier(call.expression.expression)&&call.expression.expression.text==='app'&&call.expression.name.text==='on'&&ts.isStringLiteral(call.arguments[0])&&call.arguments[0].text==='window-all-closed')allClosedCallbacks.push(call.arguments[1])
}
function setup(){
  expect(callbacks).toHaveLength(1)
  expect(willQuitCallbacks).toHaveLength(1)
  expect(allClosedCallbacks).toHaveLength(1)
  const trace:string[]=[]
  const context={isQuitting:false,managedRuntimesStoppedForQuit:false,nativeOfficeQuitPhase:'idle',quitCloseObservationTimer:null as ReturnType<typeof setTimeout>|null,desktopStartupBarrierComplete:true,setTimeout,clearTimeout,queueMicrotask,
    process:{platform:'linux'},BrowserWindow:{getAllWindows:vi.fn(()=>[])},
    prepareNativeOfficeQuit:vi.fn(async()=>{trace.push('prepare');return true}),
    cancelNativeOfficeQuit:vi.fn(async()=>{trace.push('cancel')}),
    waitForNativeOfficeQuitTeardown:vi.fn(async()=>{trace.push('office-settled')}),
    clearQuitCloseObservation:vi.fn(()=>{if(context.quitCloseObservationTimer)clearTimeout(context.quitCloseObservationTimer);context.quitCloseObservationTimer=null}),
    stopRuntimeWatchdog:vi.fn(()=>trace.push('watchdog-stop')),
    startRuntimeWatchdog:vi.fn(()=>trace.push('watchdog-start')),
    stopManagedRuntimesForQuit:vi.fn(async()=>{trace.push('core-stop');context.managedRuntimesStoppedForQuit=true}),
    stopManagedRuntimes:vi.fn(async()=>{trace.push('core-stop')}),
    traceStartup:vi.fn(),
    runtimeErrorPublicDiagnosticV1:vi.fn(()=>({errorBytes:0,errorSha256:'0'.repeat(64)})),
    publicConsoleWarn:vi.fn(),app:{quit:vi.fn(()=>{trace.push('app-quit')})},
    beforeQuit:undefined as undefined|((event:{preventDefault:()=>void})=>void),
    willQuit:undefined as undefined|((event:{preventDefault:()=>void})=>void),
    windowAllClosed:undefined as undefined|(()=>void),
    onQuitCloseDecision:undefined as undefined|((decision:string)=>void)}
  Object.assign(context, { writeShutdown: { prepareQuit: (...args: []) => context.prepareNativeOfficeQuit(...args), cancel: (...args: []) => context.cancelNativeOfficeQuit(...args) } })
  const javascript=ts.transpileModule(`const quitCloseTraceStage = ${initializer('quitCloseTraceStage').getText(source)};globalThis.onQuitCloseDecision = ${initializer('onQuitCloseDecision').getText(source)};globalThis.beforeQuit = ${callbacks[0].getText(source)};globalThis.willQuit = ${willQuitCallbacks[0].getText(source)};globalThis.windowAllClosed = ${allClosedCallbacks[0].getText(source)}`,{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
  runInNewContext(javascript,context,{timeout:1000})
  const invoke=()=>{const event={preventDefault:vi.fn()};context.beforeQuit!(event);return event}
  const invokeWill=()=>{const event={preventDefault:vi.fn()};context.willQuit!(event);return event}
  return {context,trace,invoke,invokeWill}
}
afterEach(()=>vi.useRealTimers())
test('the actual before-quit callback waits for Office preparation and Core stop, and repeated events do not duplicate either',async()=>{
  const h=setup();let prepared!:(value:boolean)=>void,stopped!:()=>void
  h.context.prepareNativeOfficeQuit.mockImplementation(()=>new Promise(resolve=>{h.trace.push('prepare');prepared=resolve}))
  h.context.stopManagedRuntimesForQuit.mockImplementation(()=>new Promise(resolve=>{h.trace.push('core-stop');stopped=()=>{h.context.managedRuntimesStoppedForQuit=true;resolve()}}))
  expect(h.invoke().preventDefault).toHaveBeenCalledOnce();h.invoke()
  expect(h.trace).toEqual(['prepare']);expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
  prepared(true);await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  expect(h.invoke().preventDefault).not.toHaveBeenCalled()
  expect(h.invokeWill().preventDefault).toHaveBeenCalledOnce()
  await vi.waitFor(()=>expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce())
  expect(h.invokeWill().preventDefault).toHaveBeenCalledOnce()
  stopped();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledTimes(2))
  expect(h.trace).toEqual(['prepare','app-quit','office-settled','watchdog-stop','core-stop','app-quit'])
  expect(h.invoke().preventDefault).not.toHaveBeenCalled()
  expect(h.invokeWill().preventDefault).not.toHaveBeenCalled()
  expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce()
})
test('cancelled or failed preparation leaves Core running and permits a later explicit quit attempt',async()=>{
  const h=setup();h.context.prepareNativeOfficeQuit.mockResolvedValue(false)
  h.invoke();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPhase).toBe('idle'))
  expect(h.context.isQuitting).toBe(false);expect(h.context.stopRuntimeWatchdog).not.toHaveBeenCalled()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
  h.context.prepareNativeOfficeQuit.mockResolvedValue(true);h.invoke()
  await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  h.invokeWill();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledTimes(2))
  expect(h.context.prepareNativeOfficeQuit).toHaveBeenCalledTimes(2)
})
test('unexpected preparation rejection thaws Office and restores startup watchdog without stopping Core',async()=>{
  const h=setup();h.context.prepareNativeOfficeQuit.mockRejectedValue(Error('freeze or flush failed'))
  h.invoke();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPhase).toBe('idle'))
  expect(h.context.cancelNativeOfficeQuit).toHaveBeenCalledOnce();expect(h.context.startRuntimeWatchdog).not.toHaveBeenCalled()
  expect(h.context.isQuitting).toBe(false);expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled();expect(h.context.app.quit).not.toHaveBeenCalled()
})

test('the actual quit callback confirms BrowserWindow.closed while Core is still alive',async()=>{
  const h=setup(),handlers=new Map<string,(...args:any[])=>any>()
  let destroyed=false
  const main=Object.assign(new EventEmitter(),{isDestroyed:()=>destroyed,close:vi.fn(),
    webContents:Object.assign(new EventEmitter(),{mainFrame:{},send:vi.fn(),isLoadingMainFrame:()=>false,setWindowOpenHandler:vi.fn()})})
  main.close.mockImplementation(()=>{
    if(destroyed)return
    const event={defaultPrevented:false,preventDefault(){this.defaultPrevented=true}}
    main.emit('close',event)
    if(!event.defaultPrevented){destroyed=true;main.emit('closed');h.trace.push('closed')}
  })
  const coordinator=createWriteShutdownCoordinator({ipc:{handle:(name:string,handler:(...args:any[])=>any)=>{handlers.set(name,handler)}} as any,
    prepareNative:h.context.prepareNativeOfficeQuit,cancelNative:h.context.cancelNativeOfficeQuit,notifyBlocked:async()=>undefined,
    onQuitCloseDecision:decision=>h.context.onQuitCloseDecision!(decision)})
  coordinator.trackWindow(main as any)
  main.webContents.send.mockImplementation((_channel,request)=>handlers.get('write:shutdown-ack')!({sender:main.webContents,senderFrame:main.webContents.mainFrame},{...request,outcome:{result:'ready'}}))
  Object.assign(h.context,{writeShutdown:coordinator})
  h.context.app.quit.mockImplementation(()=>{
    h.trace.push('app-quit')
    if(h.invoke().preventDefault.mock.calls.length===0){main.close();if(destroyed)h.invokeWill()}
  })
  h.invoke()
  await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledTimes(2))
  expect(h.trace.indexOf('closed')).toBeGreaterThanOrEqual(0)
  expect(h.trace.indexOf('closed')).toBeLessThan(h.trace.indexOf('core-stop'))
})
test.each(['prior','later'] as const)('a %s close veto cancels the quit while Core is still alive',async kind=>{
  const h=setup(),handlers=new Map<string,(...args:any[])=>any>()
  const main=Object.assign(new EventEmitter(),{isDestroyed:()=>false,close:vi.fn(),
    webContents:Object.assign(new EventEmitter(),{mainFrame:{},send:vi.fn(),isLoadingMainFrame:()=>false,setWindowOpenHandler:vi.fn()})})
  const coordinator=createWriteShutdownCoordinator({ipc:{handle:(name:string,handler:(...args:any[])=>any)=>{handlers.set(name,handler)}} as any,
    prepareNative:h.context.prepareNativeOfficeQuit,cancelNative:h.context.cancelNativeOfficeQuit,notifyBlocked:async()=>undefined,
    onQuitCloseDecision:decision=>h.context.onQuitCloseDecision!(decision)})
  coordinator.trackWindow(main as any)
  main.webContents.send.mockImplementation((_channel,request)=>handlers.get('write:shutdown-ack')!({sender:main.webContents,senderFrame:main.webContents.mainFrame},{...request,outcome:{result:'ready'}}))
  if(kind==='prior')main.prependListener('close',event=>event.preventDefault())
  else main.on('close',event=>event.preventDefault())
  Object.assign(h.context,{writeShutdown:coordinator})
  h.context.app.quit.mockImplementation(()=>{
    h.trace.push('app-quit')
    if(h.invoke().preventDefault.mock.calls.length===0){
      const event={defaultPrevented:false,preventDefault(){this.defaultPrevented=true}}
      main.emit('close',event)
    }
  })
  h.invoke()
  await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPhase).toBe('idle'))
  expect(h.context.traceStartup).toHaveBeenCalledWith(`app before quit:close veto ${kind==='later'?'late':'prior'}`)
  expect(h.context.cancelNativeOfficeQuit).toHaveBeenCalledOnce()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  expect(h.context.app.quit).toHaveBeenCalledOnce()
})
test('an unknown close timeout keeps Core alive and a late will-quit can complete',async()=>{
  vi.useFakeTimers()
  const h=setup()
  h.invoke();await vi.advanceTimersByTimeAsync(0)
  expect(h.context.app.quit).toHaveBeenCalledOnce()
  await vi.advanceTimersByTimeAsync(15_000)
  expect(h.context.nativeOfficeQuitPhase).toBe('unknown')
  expect(h.context.cancelNativeOfficeQuit).not.toHaveBeenCalled()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  h.invokeWill();await vi.advanceTimersByTimeAsync(0)
  expect(h.context.app.quit).toHaveBeenCalledTimes(2)
  expect(h.context.nativeOfficeQuitPhase).toBe('committed')
})
test('Office teardown must settle before Core stop, and stop failure never commits quit',async()=>{
  const h=setup();let finish!:(value:void)=>void
  h.context.waitForNativeOfficeQuitTeardown.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  h.context.stopManagedRuntimesForQuit.mockRejectedValue(Error('stop failed'))
  h.invoke();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(h.invokeWill().preventDefault).toHaveBeenCalledOnce()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  finish();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPhase).toBe('failed'))
  expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce()
  expect(h.context.app.quit).toHaveBeenCalledOnce()
  expect(h.context.managedRuntimesStoppedForQuit).toBe(false)
  expect(h.context.startRuntimeWatchdog).not.toHaveBeenCalled()
  expect(h.context.traceStartup).toHaveBeenCalledWith('app before quit:stop failed')
})
test('failed Office teardown keeps Core alive and reports its own failure stage',async()=>{
  const h=setup()
  h.context.waitForNativeOfficeQuitTeardown.mockRejectedValue(Error('teardown failed'))
  h.invoke();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  h.invokeWill();await vi.waitFor(()=>expect(h.context.nativeOfficeQuitPhase).toBe('failed'))
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  expect(h.context.app.quit).toHaveBeenCalledOnce()
  expect(h.context.traceStartup).toHaveBeenCalledWith('app before quit:office teardown failed')
})
test('an updater-owned runtime stop keeps the existing direct quit path',()=>{
  const h=setup();h.context.managedRuntimesStoppedForQuit=true
  expect(h.invoke().preventDefault).not.toHaveBeenCalled()
  expect(h.invokeWill().preventDefault).not.toHaveBeenCalled()
  expect(h.context.prepareNativeOfficeQuit).not.toHaveBeenCalled()
})
test('ordinary last-window close waits for Office teardown before stopping Core',async()=>{
  const h=setup();let finish!:(value:void)=>void
  h.context.waitForNativeOfficeQuitTeardown.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  h.context.windowAllClosed!()
  expect(h.context.stopManagedRuntimesForQuit).not.toHaveBeenCalled()
  expect(h.context.app.quit).not.toHaveBeenCalled()
  finish();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(h.trace).toEqual(['core-stop','app-quit'])
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
  h.invoke();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledOnce())
  expect(context.create!({primary:false})).toBeNull()
  expect(()=>context.open!('late-thread')).not.toThrow()
  expect(constructor).not.toHaveBeenCalled();expect(main.webContents.send).toHaveBeenCalledOnce()
  h.invokeWill();await vi.waitFor(()=>expect(h.context.stopManagedRuntimesForQuit).toHaveBeenCalledOnce())
  stopped();await vi.waitFor(()=>expect(h.context.app.quit).toHaveBeenCalledTimes(2))
})
