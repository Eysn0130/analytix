/* global Module, FS */
// First-party surface and transport. No Node, host bridge, remote fetch or arbitrary UNO entry point.
const MAX_BYTES = 16 * 1024 * 1024;
const token = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(value);
const identity = r => r && token(r.channel) && token(r.operationId) && token(r.documentId) && token(r.version);
const key = r => r.command + ':' + r.operationId;
const matches = (a, b) => a.channel === b.channel && a.operationId === b.operationId && a.documentId === b.documentId && a.version === b.version && a.command === b.command;
const kinds = ['docx', 'xlsx', 'pptx'];
const previewCommands = ['open', 'edit', 'replace', 'replaceCells', 'replacePresentation', 'export', 'ack', 'captureSelection', 'close'];
function validWorkbookReview(value) {
  const record=v=>v && Object.getPrototypeOf(v)===Object.prototype;
  const exact=(v,keys)=>record(v)&&Object.keys(v).length===keys.length&&keys.every(k=>Object.hasOwn(v,k));
  const integer=v=>Number.isSafeInteger(v)&&v>=0;
  const text=(v,max)=>typeof v==='string'&&v.length<=max;
  const fields=['sheet','sheetName','startColumn','startRow','endColumn','endRow','cells'];
  const snapshot=s=> {
    if(!exact(s,fields)||!['sheet','startColumn','startRow','endColumn','endRow'].every(k=>integer(s[k]))||s.sheet>=20||s.endColumn>=1024||s.endRow>=100000||!text(s.sheetName,31)||!s.sheetName||!Array.isArray(s.cells)||s.cells.length>256)return false;
    const width=s.endColumn-s.startColumn+1,height=s.endRow-s.startRow+1;
    return width>0&&height>0&&width*height===s.cells.length&&s.cells.every((c,i)=>exact(c,['sheet','column','row','text','formula','value','valueType','numberFormat','rowVisible','columnVisible','merged'])&&c.sheet===s.sheet&&c.column===s.startColumn+i%width&&c.row===s.startRow+Math.floor(i/width)&&text(c.text,4096)&&text(c.formula,4096)&&Number.isFinite(c.value)&&integer(c.numberFormat)&&c.rowVisible===true&&c.columnVisible===true&&c.merged===false&&['empty','text','number','formula'].includes(c.valueType));
  };
  return exact(value,['before','after','results'])&&snapshot(value.before)&&snapshot(value.after)&&fields.filter(k=>k!=='cells').every(k=>value.before[k]===value.after[k])&&Array.isArray(value.results)&&value.results.length===value.after.cells.length&&value.results.every(v=>text(v,65536));
}
function validPresentationSelection(s) {
  const exact=(v,keys)=>v&&typeof v==='object'&&!Array.isArray(v)&&Object.keys(v).length===keys.length&&keys.every(k=>Object.hasOwn(v,k));
  const n=(v,min,max)=>Number.isSafeInteger(v)&&v>=min&&v<=max;
  const text=v=>typeof v==='string'&&!v.includes('\0')&&!/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(v)&&new TextEncoder().encode(v).length<=4096;
  if(!exact(s,['pageIndex','targetShapeIndex','pageWidth100thMm','pageHeight100thMm','shapes'])||!n(s.pageIndex,0,511)||!n(s.pageWidth100thMm,1,1000000)||!n(s.pageHeight100thMm,1,1000000)||!Array.isArray(s.shapes)||!n(s.shapes.length,1,64)||!n(s.targetShapeIndex,0,s.shapes.length-1))return false;
  let bytes=0;
  return s.shapes.every((v,i)=>{
    if(!exact(v,['shapeIndex','kind','name','text','x100thMm','y100thMm','width100thMm','height100thMm','fillRGB'])||v.shapeIndex!==i||!['rectangle','ellipse','text'].includes(v.kind)||!text(v.name)||!text(v.text)||!/^#[0-9a-f]{6}$/.test(v.fillRGB)||!n(v.x100thMm,0,s.pageWidth100thMm)||!n(v.y100thMm,0,s.pageHeight100thMm)||!n(v.width100thMm,1,s.pageWidth100thMm-v.x100thMm)||!n(v.height100thMm,1,s.pageHeight100thMm-v.y100thMm))return false;
    bytes+=new TextEncoder().encode(v.name+v.text).length;return bytes<=65536;
  });
}
function validPresentationReview(r) {
  if(!r||Object.keys(r).length!==2||!validPresentationSelection(r.before)||!validPresentationSelection(r.after))return false;
  const a=r.before,b=r.after,expected=JSON.parse(JSON.stringify(a)),old=a.shapes[a.targetShapeIndex],next=b.shapes[b.targetShapeIndex];
  if(!next)return false;
  const target=expected.shapes[a.targetShapeIndex];
  if(old.fillRGB!==next.fillRGB)target.fillRGB=next.fillRGB;
  else for(const k of ['x100thMm','y100thMm','width100thMm','height100thMm'])target[k]=next[k];
  const equal=(x,y)=>x.pageIndex===y.pageIndex&&x.targetShapeIndex===y.targetShapeIndex&&x.pageWidth100thMm===y.pageWidth100thMm&&x.pageHeight100thMm===y.pageHeight100thMm&&x.shapes.length===y.shapes.length&&x.shapes.every((s,i)=>Object.keys(s).every(k=>s[k]===y.shapes[i][k]));
  return !equal(a,b)&&equal(expected,b);
}
function validRequest(r) {
  if (!identity(r) || Object.getPrototypeOf(r) !== Object.prototype) return false;
  let extra = [];
  if (r.command === 'open') {
    if (!kinds.includes(r.kind) || !(r.bytes instanceof Uint8Array) || !(r.bytes.buffer instanceof ArrayBuffer) || !r.bytes.byteLength || r.bytes.byteLength > MAX_BYTES) return false;
    extra = ['kind', 'bytes'];
  } else if (r.command === 'close') {
    if (!Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0 || typeof r.discard !== 'boolean') return false;
    extra = ['expectedChangeSequence', 'discard'];
  } else if (r.command === 'replacePresentation') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence<0 || !validPresentationReview(r.presentation)) return false;
    extra=['selectionToken','expectedChangeSequence','presentation'];
  } else if (r.command === 'replaceCells') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence<0 || !validWorkbookReview(r.workbook)) return false;
    extra=['selectionToken','expectedChangeSequence','workbook'];
  } else if (r.command === 'replace') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0) return false;
    if (typeof r.text !== 'string' || r.text.length > 4096 || r.valueType !== 'text') return false;
    extra = ['selectionToken', 'expectedChangeSequence', 'text', 'valueType'];
  } else if (r.command === 'ack') {
    if (!token(r.exportOperationId) || !Number.isSafeInteger(r.exportedSequence) || r.exportedSequence < 0 || !['committed','conflict','failed'].includes(r.status) || (r.status === 'committed' && !token(r.persistedVersion))) return false;
    extra = ['exportOperationId','exportedSequence','status', ...(r.status === 'committed' ? ['persistedVersion'] : [])];
  } else if (!['captureSelection','edit','export'].includes(r.command)) return false;
  return Object.keys(r).every(k => ['channel', 'operationId', 'documentId', 'version', 'command', ...extra].includes(k));
}

/** Engine lifecycle contains no parent-window trust decision. Bind a port separately. */
export class OfficeEngineSurface {
  constructor(canvas, onEvent, onView = () => {}) {
    this.canvas = canvas; this.onEvent = onEvent; this.onView = onView; this.view = null; this.pending = new Map(); this.state = null;
    this.openOperationId = null; this.editing = false; this.busy = false; this.started = false;
    // Native models remain read-only, including during controlled AI mutations.
    // This guard also blocks text input before Qt's hidden input handlers.
    const block = event => { event.preventDefault(); event.stopImmediatePropagation(); };
    const navigation = new Set(['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Home','End','PageUp','PageDown','Tab','Escape']);
    const modifiers = new Set(['Shift','Control','Meta','Alt','CapsLock']);
    const intercept = event => {
      const key = event.key.toLowerCase();
      if ((event.metaKey || event.ctrlKey) && key === 's') {
        block(event);
        if (event.type === 'keydown' && this.editing && this.state?.dirty) this.onEvent({type:'save-requested', channel:this.channel, operationId:this.openOperationId, documentId:this.state.documentId, version:this.state.version});
        return;
      }
      if (event.target?.closest?.('[data-view]') && ['Enter',' '].includes(event.key)) return;
      if (modifiers.has(event.key) || navigation.has(event.key)) return;
      if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && ['a','c'].includes(event.key.toLowerCase())) return;
      block(event);
    };
    for (const type of ['keydown','keypress','keyup']) window.addEventListener(type, intercept, true);
    for (const type of ['beforeinput','paste','drop','cut','compositionstart','compositionupdate','compositionend']) window.addEventListener(type, block, true);
  }
  async start(channel) {
    if (this.started) throw Error('already-bound');
    this.started = true; this.channel = channel;
    if (!crossOriginIsolated) throw Error('cross-origin-isolation-required');
    const assets = new URL('./assets/', location.href);
    const allowed = new Set(['soffice.js', 'soffice.wasm', 'soffice.data', 'soffice.data.js.metadata']);
    const ready = new Promise((resolve, reject) => { this.resolveReady = resolve; this.rejectReady = reject; });
    const fontResponse = await fetch(new URL('NotoSansCJKsc-Regular.otf', assets));
    if (!fontResponse.ok) throw Error('font-unavailable');
    const fontBytes = new Uint8Array(await fontResponse.arrayBuffer());
    const fontHash = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', fontBytes))).map(x => x.toString(16).padStart(2, '0')).join('');
    if (fontBytes.length !== 16437364 || fontHash !== '2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b') throw Error('font-integrity-failed');
    globalThis.Module = {
      preRun:[() => { FS.mkdirTree('/usr/share/fonts/analytix'); FS.writeFile('/usr/share/fonts/analytix/NotoSansCJKsc-Regular.otf', fontBytes); }],
      canvas:this.canvas,
      uno_scripts:[new URL('zeta.js', assets).href, new URL('./office-worker.js', location.href).href],
      locateFile:name => { if (!allowed.has(name)) throw Error('unexpected-engine-asset'); return new URL(name, assets).href; },
      print() {}, printErr() {},
    };
    const script = document.createElement('script'); script.src = new URL('soffice.js', assets).href;
    script.onerror = () => this.rejectReady(Error('engine-load-failed'));
    script.onload = () => Module.uno_main.then(port => {
      this.workerPort = port;
      port.onmessage = ({data:r}) => {
        if (r?.type === 'worker-ready') { port.postMessage({command:'bind', channel}); return; }
        if (r?.channel !== channel) return;
        if (r.type === 'ready') { this.resolveReady(); return; }
        if (r.type === 'result') {
          const pending = this.pending.get(key(r));
          if (!pending || !matches(pending.request, r)) return;
          this.pending.delete(key(r)); clearTimeout(pending.timer);
          if (r.ok) pending.resolve(r); else pending.reject(Error(r.error || 'engine-operation-failed'));
        } else if (['changed','selection'].includes(r.type) && this.state && r.documentId === this.state.documentId && r.version === this.state.version && r.operationId === this.openOperationId) {
          if (r.type === 'changed') this.state = r.state;
          this.onEvent(r);
        }
      };
    }).catch(() => this.rejectReady(Error('engine-load-failed')));
    document.body.appendChild(script);
    await ready;
    try { FS.mkdir('/tmp/analytix-office'); } catch { /* A missing temporary in-memory file needs no cleanup. */ }
  }
  worker(request) {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(key(request));
        this.failed = true;
        reject(Error('engine-timeout-state-unknown'));
      }, 60000);
      this.pending.set(key(request), {request, resolve, reject, timer});
      this.workerPort.postMessage(request);
    });
  }
  async localView(action) {
    if (!['read','reset','fit','zoom-in','zoom-out','previous-page','next-page'].includes(action)) throw Error('unsupported-command');
    if (!this.state || this.failed || this.busy) throw Error('operation-in-progress');
    this.busy = true;
    const documentId = this.state.documentId, version = this.state.version, opened = this.openOperationId;
    try {
      const result = await this.worker({command:'local-view', channel:this.channel, operationId:crypto.randomUUID(), documentId, version, action});
      if (!this.state || this.state.documentId !== documentId || this.state.version !== version || this.openOperationId !== opened) throw Error('stale-document-version');
      const view = result.view;
      if (!view || !Number.isFinite(view.zoom) || view.zoom <= 0 || view.zoom > 1000 || !Number.isSafeInteger(view.zoomType) || typeof view.fit !== 'boolean' || view.readOnly !== true || (!this.editing && view.modified !== false) || (this.state.kind === 'pptx' && (!Number.isSafeInteger(view.page) || !Number.isSafeInteger(view.pages) || view.page < 1 || view.page > view.pages || view.pages > 512))) throw Error('view-unavailable');
      this.view = view; this.onView(this.view);
      return result;
    } finally { this.busy = false; }
  }
  async request(r) {
    if (!identity(r) || r.channel !== this.channel) throw Error('invalid-request');
    if (!previewCommands.includes(r.command)) throw Error('unsupported-command');
    if (!this.editing && !['open','edit','captureSelection','close'].includes(r.command)) throw Error('unsupported-command');
    if (!validRequest(r)) throw Error('invalid-request');
    if (this.failed) throw Error('surface-state-unknown-recreate-required');
    if (this.busy) throw Error('operation-in-progress');
    if (r.command !== 'open' && (!this.state || r.documentId !== this.state.documentId || r.version !== this.state.version)) throw Error('stale-document-version');
    if (r.command === 'open' && this.state) throw Error('document-already-open');
    this.busy = true;
    try {
      const request = {...r};
      if (r.command === 'open') {
        FS.writeFile('/tmp/analytix-office/input.' + r.kind, r.bytes);
        delete request.bytes;
      }
      const result = await this.worker(request);
      if (r.command === 'export') {
        const path = '/tmp/analytix-office/output.' + this.state.kind;
        const size = FS.stat(path).size;
        if (!Number.isSafeInteger(size) || size < 1 || size > MAX_BYTES) throw Error('export-too-large');
        result.bytes = new Uint8Array(FS.readFile(path));
        FS.unlink(path);
      }
      if (result.state) this.state = result.state;
      if (r.command === 'edit') this.editing = true;
      if (r.command === 'open') { this.openOperationId = r.operationId; dispatchEvent(new Event('resize')); }
      if (r.command === 'close') {
        for (const kind of kinds) try { FS.unlink('/tmp/analytix-office/input.' + kind); } catch { /* A missing temporary in-memory file needs no cleanup. */ }
        this.state = this.openOperationId = this.view = null; this.editing = false; this.onView(null);
      }
      return result;
    } finally { this.busy = false; }
  }
}

/** A trusted Main-owned isolated preload may supply a port directly. No origin bypass or URL trust parameter. */
export async function attachOfficePort(port, channel, engine, onResult = () => {}) {
  if (!(port instanceof MessagePort) || !token(channel)) throw Error('invalid-port');
  const seen = new Set();
  port.onmessage = async ({data:r}) => {
    if (!identity(r) || r.channel !== channel) return;
    const response = {type:'result', command:r.command, channel, operationId:r.operationId, documentId:r.documentId, version:r.version};
    try {
      if (!previewCommands.includes(r.command)) throw Error('unsupported-command');
      if (!validRequest(r)) throw Error('invalid-request');
      if (seen.has(key(r))) throw Error('operation-replayed');
      if (seen.size >= 4096) throw Error('session-operation-limit');
      seen.add(key(r));
      if (r.command === 'open') onResult({type:'loading'});
      const result = await engine.request(r);
      onResult(result);
      port.postMessage(result, result.bytes ? [result.bytes.buffer] : []);
    } catch (e) {
      port.postMessage({...response, ok:false, error:e.message});
      onResult({...response, ok:false, error:e.message});
    }
  };
  port.start();
  await engine.start(channel);
  port.postMessage({type:'ready', channel, protocolVersion:1});
}

/** Default demo transport accepts ONLY the same-origin parent; opaque/null origins are never trusted. */
function installSurface() {
  const canvas = document.getElementById('qtcanvas');
  if (!canvas) return;
  canvas.addEventListener('contextmenu', e => { e.preventDefault(); e.stopImmediatePropagation(); });
  let bound = false, hostPort, channel, engine;
  const status = document.getElementById('status'), selectionText = document.getElementById('selection');
  const preview = document.getElementById('preview'), footer = document.getElementById('footer');
  const viewControls = document.getElementById('viewControls');
  let viewTimer, viewReadTimer;
  function renderView(view) {
    viewControls.hidden = !view;
    if (!view) return;
    document.getElementById('zoomValue').textContent = `${view.zoom}%`;
    const slides = engine.state.kind === 'pptx';
    document.getElementById('pageControls').hidden = !slides;
    document.getElementById('pageValue').textContent = slides ? `${view.page} / ${view.pages}` : '';
    const fit = document.getElementById('fitView');
    fit.textContent = slides ? '适合页面' : engine.state.kind === 'docx' ? '适合宽度' : '100%';
    fit.setAttribute('aria-label', fit.textContent);
    fit.setAttribute('aria-pressed', String(view.fit));
    fit.title = fit.textContent;
    for (const button of viewControls.querySelectorAll('[data-view]')) {
      button.disabled = button.dataset.view === 'previous-page' ? view.page <= 1 : button.dataset.view === 'next-page' ? view.page >= view.pages : button.dataset.view === 'zoom-out' ? view.zoom <= 20 : button.dataset.view === 'zoom-in' ? view.zoom >= 400 : false;
    }
  }
  async function applyView(action) {
    if (!engine?.state || engine.failed) return;
    if (engine.busy) { scheduleView(action); return; }
    try { await engine.localView(action); if (action !== 'read') scheduleRead(); }
    catch { viewControls.hidden = true; status.textContent = '只读预览 · 视图调整不可用'; }
  }
  function sameViewGeneration(documentId, version, opened) {
    return !!engine?.state && engine.state.documentId === documentId && engine.state.version === version && engine.openOperationId === opened;
  }
  function scheduleView(action) {
    clearTimeout(viewTimer);
    const documentId = engine?.state?.documentId, version = engine?.state?.version, opened = engine?.openOperationId;
    viewTimer = setTimeout(() => { if (sameViewGeneration(documentId, version, opened)) void applyView(action); }, 120);
  }
  function scheduleRead() {
    clearTimeout(viewReadTimer);
    const documentId = engine?.state?.documentId, version = engine?.state?.version, opened = engine?.openOperationId;
    viewReadTimer = setTimeout(() => { if (sameViewGeneration(documentId, version, opened)) void applyView('read'); }, 180);
  }
  for (const type of ['keyup','mouseup','wheel']) canvas.addEventListener(type, scheduleRead, {passive:true});
  addEventListener('resize', () => { if (engine?.state && (!engine.view || engine.view.fit)) scheduleView(engine.view ? 'fit' : 'reset'); });
  for (const button of viewControls.querySelectorAll('[data-view]')) {
    // Keep focus on the activated control so Tab / Enter can continue through
    // the toolbar. Native canvas navigation resumes when the user focuses it.
    button.addEventListener('click', () => { void applyView(button.dataset.view); });
  }
  function loading(message) { status.textContent = message; preview.setAttribute('aria-busy','true'); footer.dataset.error = 'false'; }
  function render(result) {
    if (result?.command === 'open' && result.ok) scheduleView('reset');
    if (result?.type === 'loading') { loading('正在打开文档…'); return; }
    preview.setAttribute('aria-busy','false');
    footer.dataset.error = String(result?.ok === false);
    if (result?.ok === false) {
      status.textContent = result.error === 'unsupported-format-fidelity' ? '当前引擎无法确认此文档的中西文字号保真，原件未保存。可继续预览。' : result.error === 'unsupported-command' ? '只读预览不支持此操作' : '预览未能完成，请重新打开';
      return;
    }
    if (!engine?.state) { status.textContent = '等待打开文档'; selectionText.textContent = ''; return; }
    document.getElementById('kind').textContent = {docx:'文档',xlsx:'表格',pptx:'演示文稿'}[engine.state.kind] || '预览';
    status.textContent = engine.state.dirty ? 'AI 修改待保存' : '只读预览';
    if (result?.selection) {
      const selection = result.selection;
      if (selection.kind === 'text') selectionText.textContent = selection.text ? '已选择 ' + Array.from(selection.text).length + ' 个字符' : '';
      else if (selection.kind === 'cells') selectionText.textContent = selection.ranges.map(r => `行 ${r.startRow + 1}–${r.endRow + 1} · 列 ${r.startColumn + 1}–${r.endColumn + 1}`).join('；');
      else if (selection.kind === 'shapes') selectionText.textContent = selection.shapes.map(s => `第 ${s.pageIndex + 1} 页 · 对象 ${s.shapeIndex + 1}`).join('；');
      else selectionText.textContent = '';
    }
  }
  // The isolated preload retains the private Main port and exposes only typed messages.
  // No DOM port transfer or null-origin exception is used for the product surface.
  const bridge = globalThis.analytixOfficeSurface;
  if (bridge && typeof bridge.connect === 'function' && typeof bridge.send === 'function') {
    bridge.connect(async r => {
      if (r?.type === 'bind') {
        if (bound || !token(r.channel)) return;
        bound = true; channel = r.channel;
        engine = new OfficeEngineSurface(canvas, event => {
          render(event);
          bridge.send(event);
        }, renderView);
        loading('正在加载预览…');
        try { await engine.start(channel); bridge.send({type:'ready', channel, protocolVersion:1}); render(); }
        catch { render({ok:false, error:'engine-load-failed'}); bridge.send({type:'fatal', channel, error:'engine-load-failed'}); }
        return;
      }
      if (!bound || !identity(r) || r.channel !== channel) return;
      try { if (r.command === 'open') loading('正在打开文档…'); const result = await engine.request(r); render(result); bridge.send(result); }
      catch (error) {
        const known = ['typed-mutation-failed','stale-selection','unsupported-selection','unsupported-format-fidelity','invalid-control-value','invalid-request','already-bound','engine-operation-failed','engine-load-failed',
          'document-already-open','open-failed','stale-document-version','export-awaiting-ack','ack-mismatch',
          'unsaved-changes','command-unavailable','unsupported-command','native-controls-hide-failed',
          'operation-in-progress','export-too-large','engine-timeout-state-unknown','surface-state-unknown-recreate-required'];
        const result = {type:'result', command:r.command, channel, operationId:r.operationId,
          documentId:r.documentId, version:r.version, ok:false,
          error:known.includes(error?.message) ? error.message : 'engine-operation-failed'};
        render(result); bridge.send(result);
      }
    });
  }
  addEventListener('message', e => {
    if (bridge || bound || e.source !== parent || parent === window || e.origin === 'null' || e.origin !== location.origin) return;
    if (e.data?.type !== 'analytix-office-bind' || e.data.protocolVersion !== 1 || !token(e.data.channel) || e.ports.length !== 1) return;
    bound = true; hostPort = e.ports[0]; channel = e.data.channel;
    engine = new OfficeEngineSurface(canvas, event => { render(event); hostPort.postMessage(event); }, renderView);
    loading('正在加载预览…');
    attachOfficePort(hostPort, channel, engine, render).then(() => render()).catch(() => { render({ok:false, error:'engine-load-failed'}); hostPort.postMessage({type:'fatal', channel, error:'engine-load-failed'}); });
  });
}

/** Same-origin preview demo helper; product transport remains Main-owned. */
export function connectOfficeSurface(iframe, {onEvent = () => {}} = {}) {
  const url = new URL(iframe.src, location.href);
  if (url.origin === 'null' || url.origin !== location.origin) throw Error('same-origin-demo-only');
  const channel = crypto.randomUUID(), ports = new MessageChannel(), pending = new Map();
  let current, openOperationId, closed = false, resolveReady, rejectReady;
  const ready = new Promise((resolve, reject) => { resolveReady = resolve; rejectReady = reject; });
  const readyTimer = setTimeout(() => rejectReady(Error('engine-ready-timeout')), 60000);
  ports.port1.onmessage = ({data:r}) => {
    if (r?.channel !== channel) return;
    if (r.type === 'ready') { clearTimeout(readyTimer); resolveReady(); return; }
    if (r.type === 'fatal') { clearTimeout(readyTimer); rejectReady(Error(r.error)); return; }
    if (r.type === 'result') {
      const call = pending.get(key(r));
      if (!call || !matches(call.request, r)) return;
      pending.delete(key(r)); clearTimeout(call.timer);
      if (r.ok) {
        if (r.state) current = r.state;
        if (r.command === 'open') openOperationId = r.operationId;
        call.resolve(r);
      } else call.reject(Error(r.error || 'surface-operation-failed'));
    } else if (current && r.documentId === current.documentId && r.version === current.version &&
      r.operationId === openOperationId) {
      if (r.type === 'changed') current = r.state;
      if (['changed','selection'].includes(r.type)) onEvent(r);
    }
  };
  ports.port1.start();
  iframe.contentWindow.postMessage({type:'analytix-office-bind', protocolVersion:1, channel}, url.origin, [ports.port2]);
  async function rpc(command, input = {}) {
    await ready;
    if (closed) throw Error('surface-closed');
    const request = {channel, command, operationId:crypto.randomUUID(), documentId:current?.documentId, version:current?.version, ...input};
    if (!validRequest(request) || pending.has(key(request))) throw Error('invalid-or-duplicate-request');
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { pending.delete(key(request)); reject(Error('surface-timeout-state-unknown')); }, 65000);
      pending.set(key(request), {request, resolve, reject, timer});
      ports.port1.postMessage(request);
    });
  }
  return Object.freeze({
    ready, open:input => rpc('open', input), captureSelection:() => rpc('captureSelection'),
    close:async ({discard = false} = {}) => {
      const result = await rpc('close', {expectedChangeSequence:current?.changeSequence, discard});
      {
        closed = true; clearTimeout(readyTimer); ports.port1.close(); iframe.remove();
        for (const call of pending.values()) { clearTimeout(call.timer); call.reject(Error('surface-closed')); }
        pending.clear();
      }
      return result;
    },
  });
}

installSurface();
