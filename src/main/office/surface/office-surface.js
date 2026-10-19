/* global Module, FS */
// First-party surface and transport. No Node, host bridge, remote fetch or arbitrary UNO entry point.
const MAX_BYTES = 16 * 1024 * 1024;
// Main verifies the exact manifest before serving this classic-script runtime.
// Internal registration observers below apply only to this qualified JS/WASM pair.
const INPUT_ASSET_VERSION = 'efaf0670b4d055f838a2849becb10f08aa06a257:5143e5354f470b87f86ba272bcfef857bd13e6f07b59666e48a7ccb89643cd77:9ebd9a487e849a24b9c69f843ebdb451709c27b7722c010e36846433474a5bd4';
const token = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(value);
const identity = r => r && token(r.channel) && token(r.operationId) && token(r.documentId) && token(r.version);
const key = r => r.command + ':' + r.operationId;
const matches = (a, b) => a.channel === b.channel && a.operationId === b.operationId && a.documentId === b.documentId && a.version === b.version && a.command === b.command;
const kinds = ['docx', 'xlsx', 'pptx'];
const previewCommands = ['local-view','open', 'edit', 'manual-edit', 'format', 'freeze', 'replace', 'replaceCells', 'replacePresentation', 'export', 'ack', 'captureSelection', 'captureCells', 'validateSelection', 'close'];
const exact = (value, fields) => value && Object.getPrototypeOf(value) === Object.prototype && Object.keys(value).length === fields.length && fields.every(k => Object.hasOwn(value,k));
const integer = value => Number.isSafeInteger(value) && value >= 0;
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const nativeOperation = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/.test(value);
const nativeToken = value => typeof value === 'string' && /^[A-Za-z0-9_-]{8,128}$/.test(value);
const inputText = value => typeof value === 'string' && !/\p{Cc}/u.test(value.replace(/[\r\n\t]/g,'')) && !/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= 65536;
const validLease = value => exact(value,['generation','sequence','token']) && integer(value.generation) && integer(value.sequence) && nativeToken(value.token);
const validFormat = value => exact(value,['kind','value']) && (value.kind === 'font-family'
  ? typeof value.value === 'string' && value.value.length >= 1 && value.value.length <= 128 && value.value.trim() === value.value && !/\p{Cc}/u.test(value.value)
  : value.kind === 'font-size' ? typeof value.value === 'number' && Number.isFinite(value.value) && value.value >= 6 && value.value <= 96 && Number.isInteger(value.value * 2)
  : ['bold','italic','underline'].includes(value.kind) && typeof value.value === 'boolean');
const pointerControl = target => !!target?.closest?.('#pointerActions [data-format-control]');
const formatFields = ['fontFamily','fontSize','bold','italic','underline'];
const validFormatState = value => exact(value,formatFields) && formatFields.every(field => value[field] === null || validFormat({kind:field === 'fontFamily' ? 'font-family' : field === 'fontSize' ? 'font-size' : field,value:value[field]}));
const sameLease = (a,b) => validLease(a) && validLease(b) && ['generation','sequence','token'].every(k => a[k] === b[k]);
const validFormatting = value => exact(value,['changeId','rangeLease','formatState','labels']) && digest(value.changeId) && validLease(value.rangeLease) && validFormatState(value.formatState) &&
  exact(value.labels,['fontFamily','fontSize','bold','italic','underline','mixed']) && Object.values(value.labels).every(label => typeof label === 'string' && label.length >= 1 && label.length <= 120 && !/\p{Cc}/u.test(label));
const validAuthority = value => exact(value,['version','origin','changeId','baseRevision','saveOperationId','undoOperationId']) && value.version === 2 && value.origin === 'manual' && digest(value.changeId) && digest(value.baseRevision) && nativeOperation(value.saveOperationId) && nativeOperation(value.undoOperationId);
const validFreeze = value => exact(value,['changeId','operationId','generation','sequence']) && digest(value.changeId) && nativeOperation(value.operationId) && integer(value.generation) && integer(value.sequence);
const playActions = ['play-read','play-start','play-current','play-next-effect','play-previous-effect','play-next-slide','play-previous-slide','play-pause','play-resume','play-end'];
const presentationErrors = ['presentation-unsupported','presentation-failed','presentation-start-timeout'];
const playing = view => ['running','paused'].includes(view?.playback?.phase);
const validWorkbookBounds = value => exact(value,['sheet','startColumn','startRow','endColumn','endRow']) &&
  ['sheet','startColumn','startRow','endColumn','endRow'].every(k => integer(value[k])) && value.sheet < 20 && value.endColumn < 1024 && value.endRow < 100000 &&
  value.startColumn <= value.endColumn && value.startRow <= value.endRow && (value.endColumn-value.startColumn+1)*(value.endRow-value.startRow+1) <= 256;
const validWorkbookTarget = value => exact(value,['sheet','column','row']) && integer(value.sheet) && value.sheet < 20 && integer(value.column) && value.column < 1024 && integer(value.row) && value.row < 100000;
function validPlayback(value) {
  const fields = ['generation','sequence','phase','slideIndex','slideCount'];
  if (!value || Object.getPrototypeOf(value) !== Object.prototype || Object.keys(value).length !== fields.length || !fields.every(k => Object.hasOwn(value,k)) ||
      !Number.isSafeInteger(value.generation) || value.generation < 0 || !Number.isSafeInteger(value.sequence) || value.sequence < 0 ||
      !['idle','running','paused','unsupported','error'].includes(value.phase)) return false;
  return ['running','paused'].includes(value.phase)
    ? Number.isSafeInteger(value.slideCount) && value.slideCount >= 1 && value.slideCount <= 512 && Number.isSafeInteger(value.slideIndex) && value.slideIndex >= -1 && value.slideIndex < value.slideCount
    : value.slideIndex === null && value.slideCount === null;
}
function validPreviewView(value, kind, editing, nativeWritable = false) {
  const fields = ['zoom','zoomType','fit','page','pages','readOnly','modified'];
  if (!value || Object.getPrototypeOf(value) !== Object.prototype || !fields.every(k => Object.hasOwn(value,k)) ||
      Object.keys(value).some(k => !fields.includes(k) && k !== 'playback') || !Number.isFinite(value.zoom) || value.zoom <= 0 || value.zoom > 1000 ||
      !Number.isSafeInteger(value.zoomType) || value.zoomType < 0 || typeof value.fit !== 'boolean' || typeof value.readOnly !== 'boolean' || (!value.readOnly && (kind !== 'docx' || !nativeWritable)) ||
      typeof value.modified !== 'boolean' || (!editing && value.modified !== false)) return false;
  if (kind === 'pptx' ? !Number.isSafeInteger(value.page) || !Number.isSafeInteger(value.pages) || value.page < 1 || value.page > value.pages || value.pages > 512 : value.page !== null || value.pages !== null) return false;
  return !Object.hasOwn(value, 'playback') || kind === 'pptx' && validPlayback(value.playback);
}
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
  if (r.command === 'local-view') {
    if (playActions.includes(r.action)) {
      if (r.zoom !== undefined || (r.action === 'play-read' ? r.playbackGeneration !== undefined : !Number.isSafeInteger(r.playbackGeneration) || r.playbackGeneration < 0)) return false;
      extra = r.action === 'play-read' ? ['action'] : ['action','playbackGeneration'];
    } else {
      if (r.action === 'set-zoom' ? !Number.isSafeInteger(r.zoom) || r.zoom < 20 || r.zoom > 400 : !['read','reset','fit','zoom-in','zoom-out','previous-page','next-page'].includes(r.action) || r.zoom !== undefined) return false;
      extra = r.action === 'set-zoom' ? ['action','zoom'] : ['action'];
    }
  } else if (r.command === 'open') {
    if (!kinds.includes(r.kind) || !(r.bytes instanceof Uint8Array) || !(r.bytes.buffer instanceof ArrayBuffer) || !r.bytes.byteLength || r.bytes.byteLength > MAX_BYTES) return false;
    extra = ['kind', 'bytes'];
  } else if (r.command === 'close') {
    if (!Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0 || typeof r.discard !== 'boolean') return false;
    extra = ['expectedChangeSequence', 'discard'];
  } else if (r.command === 'manual-edit') {
    if (!validAuthority(r.manual) || r.manual.baseRevision !== r.version) return false;
    extra = ['manual'];
  } else if (r.command === 'format') {
    if (!digest(r.changeId) || !validLease(r.rangeLease) || !validFormat(r.format)) return false;
    extra = ['changeId','rangeLease','format'];
  } else if (r.command === 'insert-text') {
    // Private surface-to-worker command, never admitted from a Main/DOM port.
    if (!digest(r.changeId) || !nativeOperation(r.operationId) || !inputText(r.text)) return false;
    extra = ['changeId','text'];
  } else if (r.command === 'bind-input-runtime') {
    if (!Number.isSafeInteger(r.threadId) || r.threadId <= 0 || r.assetVersion !== INPUT_ASSET_VERSION || !nativeOperation(r.operationId)) return false;
    extra = ['threadId','assetVersion'];
  } else if (r.command === 'freeze') {
    if (!digest(r.changeId) || !nativeOperation(r.operationId)) return false;
    extra = ['changeId'];
  } else if (r.command === 'export') {
    if (r.changeId !== undefined || r.freezeAck !== undefined) {
      if (!digest(r.changeId) || !validFreeze(r.freezeAck) || r.freezeAck.changeId !== r.changeId) return false;
      extra = ['changeId','freezeAck'];
    }
  } else if (r.command === 'validateSelection') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0) return false;
    extra = ['selectionToken','expectedChangeSequence'];
  } else if (r.command === 'captureCells') {
    if (!token(r.selectionToken) || !integer(r.expectedChangeSequence) || !validWorkbookBounds(r.bounds)) return false;
    extra = ['selectionToken','expectedChangeSequence','bounds'];
  } else if (r.command === 'replacePresentation') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence<0 || !validPresentationReview(r.presentation)) return false;
    extra=['selectionToken','expectedChangeSequence','presentation'];
  } else if (r.command === 'replaceCells') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence<0 || !validWorkbookReview(r.workbook) || r.writeTarget !== undefined && !validWorkbookTarget(r.writeTarget)) return false;
    extra=['selectionToken','expectedChangeSequence','workbook',...(r.writeTarget === undefined ? [] : ['writeTarget'])];
  } else if (r.command === 'replace') {
    if (!token(r.selectionToken) || !Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0) return false;
    if (typeof r.text !== 'string' || r.text.length > 4096 || r.valueType !== 'text') return false;
    extra = ['selectionToken', 'expectedChangeSequence', 'text', 'valueType'];
  } else if (r.command === 'ack') {
    if (!token(r.exportOperationId) || !Number.isSafeInteger(r.exportedSequence) || r.exportedSequence < 0 || !['committed','conflict','failed'].includes(r.status) || (r.status === 'committed' && !token(r.persistedVersion))) return false;
    extra = ['exportOperationId','exportedSequence','status', ...(r.status === 'committed' ? ['persistedVersion'] : [])];
  } else if (!['captureSelection','edit'].includes(r.command)) return false;
  return Object.keys(r).every(k => ['channel', 'operationId', 'documentId', 'version', 'command', ...extra].includes(k));
}

/** Engine lifecycle contains no parent-window trust decision. Bind a port separately. */
export class OfficeEngineSurface {
  constructor(canvas, onEvent, onView = () => {}) {
    this.canvas = canvas; this.onEvent = onEvent; this.onView = onView; this.view = null; this.pending = new Map(); this.state = null;
    this.openOperationId = null; this.editing = false; this.busy = false; this.viewRead = null; this.started = false;
    this.nativeWritable = false; this.manualInput = null; this.inputGate = true; this.inputPending = 0;
    this.inputTail = Promise.resolve(); this.composition = null; this.compositionSerial = 0; this.compositionKeys = new Set();
    this.finalComposition = null; this.exportArtifact = null; this.freezeProof = null;
    this.manualLoading = null; this.lastManualGeneration = 0; this.fontFamilies = [];
    this.inputRuntime = {generation:0,invalid:false,registrations:new Map(),wrappers:{},bound:null,qualified:null,binding:false};
    // AI/preview body input stays closed. Only a correlated manual-edit result may
    // open native keyboard admission; absence of an AI proposal is no authority.
    const block = event => { event.preventDefault(); event.stopImmediatePropagation(); };
    // Consume only finite presentation keys on this canvas. The general readonly
    // filter below stays unchanged, and Qt never sees a second copy of the key.
    const playbackKey = event => {
      if (event.target !== this.canvas || !this.state || this.state.kind !== 'pptx' || this.failed || event.isComposing || event.ctrlKey || event.metaKey || event.altKey) return;
      const snapshot = this.view?.playback;
      if (!snapshot) return;
      let action;
      if (event.key === 'F5') action = event.shiftKey ? 'play-current' : 'play-start';
      else if (playing(this.view) && !event.shiftKey) action = {
        ' ':'play-next-effect', Enter:'play-next-effect', ArrowRight:'play-next-effect', ArrowDown:'play-next-effect',
        Backspace:'play-previous-effect', ArrowLeft:'play-previous-effect', ArrowUp:'play-previous-effect',
        PageDown:'play-next-slide', PageUp:'play-previous-slide', Escape:'play-end'
      }[event.key];
      if (!action) return;
      block(event);
      if (event.type !== 'keydown' || event.repeat || this.busy) return;
      if (event.key === 'F5' && playing(this.view)) return;
      void this.localView(action, undefined, snapshot.generation).catch(() => { /* The owned failure view is already published. */ });
    };
    for (const type of ['keydown','keypress','keyup']) window.addEventListener(type, playbackKey, true);
    const navigation = new Set(['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Home','End','PageUp','PageDown','Tab','Escape']);
    const modifiers = new Set(['Shift','Control','Meta','Alt','CapsLock']);
    const intercept = event => {
      const key = event.key.toLowerCase(), body = event.target === this.canvas;
      if (body && (this.failed || this.manualLoading)) { block(event); return; }
      if ((event.metaKey || event.ctrlKey) && key === 's') {
        block(event);
        if (event.type === 'keydown' && this.editing && (this.state?.dirty || this.manualInput)) this.onEvent({type:'save-requested', channel:this.channel, operationId:this.openOperationId, documentId:this.state.documentId, version:this.state.version});
        return;
      }
      if (body && this.state?.kind === 'xlsx' && event.key === 'F2') {
        block(event);
        if (event.type === 'keydown' && !event.repeat && !event.isComposing && !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey) void this.requestCellEdit('f2');
        return;
      }
      if (pointerControl(event.target)) return;
      if (event.target?.closest?.('[data-view], [data-pointer-action]') && ['Enter',' '].includes(event.key)) return;
      if (body && this.manualInput) {
        const composing = this.composition || event.isComposing || event.keyCode === 229 || ['Process','Dead'].includes(event.key);
        const physical = event.code || event.key;
        if (composing || event.type === 'keyup' && this.compositionKeys.has(physical)) {
          // Let the browser IME run, but never also send its keys through Qt.
          if (event.type === 'keydown') this.compositionKeys.add(physical);
          if (event.type === 'keyup') this.compositionKeys.delete(physical);
          event.stopImmediatePropagation();
          if (!this.manualAvailable() || this.inputGate) event.preventDefault();
          return;
        }
        if (!this.manualAvailable() || this.inputGate || this.inputPending || this.busy) { block(event); return; }
        this.finalComposition = null;
        if (event.shiftKey && event.key === 'Delete') { block(event); return; }
        if (modifiers.has(event.key) || navigation.has(event.key)) return;
        if ((event.metaKey || event.ctrlKey) && !event.altKey) {
          // Copy/select/paste are the only browser shortcuts. Formatting uses
          // its closed range lease; native undo/save/menu shortcuts stay closed.
          if (!event.shiftKey && key === 'v') {
            // Qt also reads navigator.clipboard from the V keydown. Stop that
            // path, retaining the browser default which emits our plain paste.
            event.stopImmediatePropagation(); return;
          }
          if (!event.shiftKey && ['a','c'].includes(key) || ['ArrowLeft','ArrowRight','Home','End','Backspace','Delete'].includes(event.key)) return;
          block(event); return;
        }
        if ((!event.altKey || event.getModifierState?.('AltGraph')) && (Array.from(event.key).length === 1 || ['Backspace','Delete','Enter'].includes(event.key))) return;
        block(event); return;
      }
      if (modifiers.has(event.key) || navigation.has(event.key)) return;
      if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && ['a','c'].includes(event.key.toLowerCase())) return;
      block(event);
    };
    for (const type of ['keydown','keypress','keyup']) window.addEventListener(type, intercept, true);
    window.addEventListener('dblclick', event => {
      if (event.target !== this.canvas || this.state?.kind !== 'xlsx') return;
      block(event);
      if (event.button === 0 && !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey) void this.requestCellEdit('double-click');
    }, true);
    for (const type of ['compositionstart','compositionupdate','compositionend']) window.addEventListener(type, event => {
      if (pointerControl(event.target)) return;
      const settling = this.composition?.owner === this.manualInput && event.type !== 'compositionstart';
      if (event.target !== this.canvas || !this.manualAvailable() || this.inputGate || (!settling && (this.busy || this.inputPending))) { block(event); return; }
      event.stopImmediatePropagation();
      if (event.type === 'compositionstart') {
        if (!this.composition) this.composition = {serial:++this.compositionSerial, owner:this.manualInput};
        this.finalComposition = null;
      } else if (event.type === 'compositionend') {
        const active = this.composition; this.composition = null;
        if (!active || active.owner !== this.manualInput) return;
        // A later insertText beforeinput is the browser's copy of this same
        // commit, not a second native insertion. Empty data is cancellation.
        this.finalComposition = {serial:active.serial, owner:active.owner};
        if (event.data !== '') this.queueText(event.data);
      }
    }, true);
    window.addEventListener('beforeinput', event => {
      if (pointerControl(event.target)) return;
      // No browser DOM document or mirror: this also consumes the final
      // insertText emitted after compositionend and insertFromPaste duplicates.
      block(event);
      if (event.target === this.canvas && this.finalComposition?.owner === this.manualInput && !event.isComposing) this.finalComposition = null;
    }, true);
    window.addEventListener('input', event => {
      if (event.target === this.canvas) { event.stopImmediatePropagation(); this.canvas.textContent = ''; }
    }, true);
    window.addEventListener('paste', event => {
      if (pointerControl(event.target)) return;
      block(event);
      if (event.target !== this.canvas || !this.manualAvailable() || this.inputGate || this.busy || this.composition) return;
      // Read no HTML, files or clipboard handles; retain the native caret style.
      const text = event.clipboardData?.getData('text/plain');
      if (text !== undefined && text !== '') this.queueText(text);
    }, true);
    window.addEventListener('drop', block, true);
    window.addEventListener('cut', event => { if (!pointerControl(event.target)) block(event); }, true);
    for (const type of ['pointerdown','pointerup','pointermove','mousedown','mouseup','mousemove','contextmenu']) window.addEventListener(type, event => {
      if (event.target !== this.canvas) return;
      // Qt owns native menus as well as its canvas mouse listeners. Hide-layout
      // settings and Electron's Chromium context-menu hook cannot close them.
      if (this.failed || this.manualLoading || type === 'contextmenu' || event.button !== 0 || event.ctrlKey || this.manualInput && (this.inputGate || this.inputPending || this.busy || this.composition)) block(event);
    }, true);
  }
  async requestCellEdit(intent) {
    if (this.failed || this.busy || !this.state || this.state.kind !== 'xlsx' || this.state.dirty || this.manualInput || this.manualLoading) return;
    const {documentId,version,changeSequence}=this.state, opened=this.openOperationId;
    try {
      const result=await this.request({command:'captureSelection',channel:this.channel,operationId:crypto.randomUUID(),documentId,version});
      const selection=result.selection;
      if(this.failed||!this.state||this.state.documentId!==documentId||this.state.version!==version||this.state.changeSequence!==changeSequence||this.openOperationId!==opened||
          selection?.kind!=='cells'||!token(selection.token)||selection.ranges?.length!==1||selection.cells?.length!==1||selection.capture?.complete!==true)return;
      const range=selection.ranges[0];
      if(range.startColumn!==range.endColumn||range.startRow!==range.endRow)return;
      this.onEvent({type:'cell-edit-requested',intent,channel:this.channel,operationId:opened,documentId,version,selection});
    }catch { /* A busy or changed selection never opens an editor for an older cell. */ }
  }
  observeInputRuntime() {
    const runtime = this.inputRuntime, events = globalThis.JSEvents, resolveTarget = globalThis.findEventTarget;
    const names = {registerKeyEventCallback:{keydown:2,keyup:3},registerMouseEventCallback:{mousedown:5,mouseup:6,mousemove:8}};
    if (runtime.invalid || Object.keys(runtime.wrappers).length || !events || typeof events.getTargetThreadForEventCallback !== 'function' || typeof resolveTarget !== 'function' ||
      Object.keys(names).some(name => typeof globalThis[name] !== 'function')) { this.invalidateInputRuntime(); return; }
    runtime.events = events; runtime.resolveThread = events.getTargetThreadForEventCallback; runtime.resolveTarget = resolveTarget;
    for (const [name,types] of Object.entries(names)) {
      const original = globalThis[name], engine = this;
      const observer = function(...args) {
        let sample;
        try {
          if (Object.hasOwn(types,args[5]) && resolveTarget(args[0]) === engine.canvas) {
            // Resolve while currentProxiedOperationCallerThread still belongs
            // to this native registration; never inspect hardcoded heap offsets.
            sample = {type:args[5],typeId:args[4],threadId:events.getTargetThreadForEventCallback(args[6]),callback:args[3],userData:args[1],useCapture:args[2]};
          }
        } catch { engine.invalidateInputRuntime(); }
        let result;
        try { result = Reflect.apply(original,this,args); }
        catch (error) { engine.invalidateInputRuntime(); throw error; }
        if (sample) {
          if (result !== 0 || sample.typeId !== types[sample.type] || !Number.isSafeInteger(sample.threadId) || sample.threadId <= 0 ||
            !Number.isSafeInteger(sample.callback) || sample.callback < 0 || !Number.isSafeInteger(runtime.generation + 1)) engine.invalidateInputRuntime();
          else if (engine.manualLoading) {
            const loading = engine.manualLoading;
            if (!engine.manualLoadingCurrent(loading) || sample.threadId !== loading.threadId ||
              (sample.callback === 0 ? !runtime.registrations.has(sample.type) || loading.removed.has(sample.type)
                : !loading.removed.has(sample.type) || runtime.registrations.has(sample.type))) engine.invalidateInputRuntime();
            else {
              if (sample.callback === 0) { runtime.registrations.delete(sample.type); loading.removed.add(sample.type); }
              else { runtime.registrations.set(sample.type,sample); loading.removed.delete(sample.type); }
              runtime.generation++;
            }
          } else if (sample.callback === 0 || runtime.bound || runtime.binding || runtime.registrations.has(sample.type) ||
            [...runtime.registrations.values()].some(value => value.threadId !== sample.threadId)) engine.invalidateInputRuntime();
          else { runtime.registrations.set(sample.type,sample); runtime.generation++; }
        }
        return result;
      };
      runtime.wrappers[name] = observer; globalThis[name] = observer;
    }
  }
  invalidateInputRuntime() {
    const runtime = this.inputRuntime, active = runtime.bound || runtime.binding || this.manualInput || this.manualLoading;
    runtime.invalid = true; runtime.bound = runtime.qualified = null; this.freezeProof = null; this.inputGate = true;
    if (active && !this.failed) this.retireInput();
  }
  inputRuntimeIdentity() {
    const runtime = this.inputRuntime;
    if (runtime.invalid || !Object.keys(runtime.wrappers).length) return null;
    if (runtime.events !== globalThis.JSEvents || runtime.resolveTarget !== globalThis.findEventTarget ||
      runtime.resolveThread !== runtime.events?.getTargetThreadForEventCallback ||
      Object.keys(runtime.wrappers).length !== 2 || Object.entries(runtime.wrappers).some(([name,wrapper]) => globalThis[name] !== wrapper)) { this.invalidateInputRuntime(); return null; }
    const down = runtime.registrations.get('keydown');
    if (!down || !['keyup','mousedown','mouseup','mousemove'].every(type => runtime.registrations.get(type)?.threadId === down.threadId)) return null;
    return {threadId:down.threadId,generation:runtime.generation};
  }
  async bindInputRuntime() {
    const proof = this.inputRuntimeIdentity(), runtime = this.inputRuntime;
    if (!proof) throw Error('native-input-unavailable');
    const documentId = this.state.documentId, version = this.state.version, opened = this.openOperationId;
    if (runtime.qualified?.threadId === proof.threadId && runtime.qualified.generation === proof.generation) {
      runtime.bound = {...proof,documentId,version,opened}; return runtime.bound;
    }
    const request = {command:'bind-input-runtime',channel:this.channel,operationId:crypto.randomUUID(),documentId,version,threadId:proof.threadId,assetVersion:INPUT_ASSET_VERSION};
    if (!validRequest(request)) throw Error('native-input-unavailable');
    runtime.binding = true;
    try {
      const result = await this.worker(request), current = this.inputRuntimeIdentity();
      if (this.failed || !current || current.threadId !== proof.threadId || current.generation !== proof.generation ||
        this.state?.documentId !== documentId || this.state.version !== version || this.openOperationId !== opened ||
        !exact(result.inputRuntime,['assetVersion','threadMatches']) || result.inputRuntime.assetVersion !== INPUT_ASSET_VERSION || result.inputRuntime.threadMatches !== true ||
        result.state?.documentId !== documentId || result.state.version !== version) {
        this.invalidateInputRuntime(); throw Error('native-input-unavailable');
      }
      runtime.qualified = {...proof}; runtime.bound = {...proof,documentId,version,opened}; return runtime.bound;
    } finally { runtime.binding = false; }
  }
  manualLoadingCurrent(loading) {
    return !!loading && loading === this.manualLoading && !this.failed && !this.inputRuntime.invalid &&
      this.busy && this.inputGate && !this.manualInput && !this.inputRuntime.binding &&
      loading.documentId === this.state?.documentId && loading.version === this.state.version && loading.opened === this.openOperationId &&
      Number.isSafeInteger(loading.expectedGeneration) && loading.expectedGeneration === this.lastManualGeneration + 1 && loading.threadId === this.inputRuntime.bound?.threadId;
  }
  armManualLoading(request, proof) {
    const current = this.inputRuntimeIdentity();
    if (this.manualLoading || !current || this.inputRuntime.bound !== proof || current.threadId !== proof.threadId || current.generation !== proof.generation ||
      !integer(this.lastManualGeneration) || !Number.isSafeInteger(this.lastManualGeneration + 1)) throw Error('native-input-unavailable');
    this.inputGate = true; this.canvas.contentEditable = 'false';
    const loading = {operationId:request.operationId,documentId:request.documentId,version:request.version,opened:this.openOperationId,
      changeId:request.manual.changeId,threadId:proof.threadId,expectedGeneration:this.lastManualGeneration + 1,removed:new Set()};
    this.manualLoading = loading; return loading;
  }
  manualCurrent(owner = this.manualInput) {
    const manual = this.state?.manual, proof = this.inputRuntimeIdentity(), bound = this.inputRuntime.bound;
    return !!owner && owner === this.manualInput && !this.failed && this.nativeWritable && this.editing && this.state?.kind === 'docx' &&
      owner.documentId === this.state.documentId && owner.version === this.state.version && owner.opened === this.openOperationId &&
      !!proof && !!bound && proof.threadId === bound.threadId && proof.generation === owner.inputGeneration && bound.generation === owner.inputGeneration &&
      bound.documentId === owner.documentId && bound.version === owner.version && bound.opened === owner.opened &&
      manual?.version === 2 && manual.origin === 'manual' && manual.changeId === owner.changeId && manual.generation === owner.generation;
  }
  manualAvailable(owner = this.manualInput) { return this.manualCurrent(owner) && this.state.manual.frozen === false; }
  closeInput() {
    if (this.manualLoading) { this.failed = true; this.inputRuntime.bound = this.inputRuntime.qualified = null; }
    this.manualLoading = null;
    this.inputGate = true; this.manualInput = null; this.composition = this.finalComposition = null; this.compositionKeys.clear();
    this.freezeProof = null; this.canvas.contentEditable = 'false'; this.canvas.textContent = '';
    this.canvas.setAttribute?.('aria-label','Office 只读预览；可浏览和选择内容');
  }
  retireInput() {
    this.failed = true; this.closeInput();
    this.onEvent({type:'fatal',channel:this.channel,error:'engine-operation-failed'});
  }
  queueText(text) {
    if (!inputText(text)) { this.onInputError?.('invalid-control-value'); return; }
    if (!text || !this.manualAvailable() || this.inputGate) return;
    const owner = this.manualInput;
    const request = {command:'insert-text',channel:this.channel,operationId:crypto.randomUUID(),documentId:owner.documentId,version:owner.version,changeId:owner.changeId,text};
    if (!validRequest(request)) { this.onInputError?.('invalid-request'); return; }
    this.inputPending++;
    const operation = this.inputTail.then(async () => {
      if (!this.manualAvailable(owner)) throw Error('stale-document-version');
      const result = await this.worker(request);
      if (!this.manualAvailable(owner) || result.state?.manual?.changeId !== owner.changeId || result.state.manual.generation !== owner.generation ||
        result.state.documentId !== owner.documentId || result.state.version !== owner.version) throw Error('surface-state-unknown-recreate-required');
      this.state = result.state;
      this.onEvent({type:'changed',channel:this.channel,operationId:owner.opened,documentId:owner.documentId,version:owner.version,state:this.state});
      if (result.selection) this.onEvent({type:'selection',channel:this.channel,operationId:owner.opened,documentId:owner.documentId,version:owner.version,selection:result.selection});
    });
    this.inputTail = operation.catch(() => { this.retireInput(); }).finally(() => { this.inputPending--; });
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
      preRun:[() => { this.observeInputRuntime(); FS.mkdirTree('/usr/share/fonts/analytix'); FS.writeFile('/usr/share/fonts/analytix/NotoSansCJKsc-Regular.otf', fontBytes); }],
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
        if (r.type === 'fatal') {
          this.retireInput();
          return;
        }
        if (r.type === 'ready') { this.resolveReady(); return; }
        if (r.type === 'result') {
          const pending = this.pending.get(key(r));
          if (!pending || !matches(pending.request, r)) return;
          this.pending.delete(key(r)); clearTimeout(pending.timer);
          if (r.ok) pending.resolve(r); else pending.reject(Error(r.error || 'engine-operation-failed'));
        } else if (r.type === 'selection-confirmed') {
          if (this.state?.kind === 'docx' && r.documentId === this.state.documentId && r.version === this.state.version && r.operationId === this.openOperationId &&
            typeof r.eligible === 'boolean' && typeof r.changed === 'boolean' && (r.selectionToken === null || token(r.selectionToken))) this.onSelectionIdentity?.(r);
        } else if (r.type === 'view') {
          try { this.receiveView(r); }
          catch { this.retireInput(); }
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
        this.retireInput();
        reject(Error('engine-timeout-state-unknown'));
      }, 60000);
      this.pending.set(key(request), {request, resolve, reject, timer});
      this.workerPort.postMessage(request);
    });
  }
  acceptView(view) {
    if (this.failed) return false;
    if (!validPreviewView(view, this.state.kind, this.editing, this.nativeWritable)) throw Error('view-unavailable');
    const previous = this.view?.playback, next = view.playback;
    if (previous && (!next || next.generation < previous.generation || next.generation === previous.generation && next.sequence <= previous.sequence)) return false;
    this.view = {...view, ...(next ? {playback:{...next}} : {})}; this.onView(this.view);
    return true;
  }
  receiveView(event) {
    if (this.failed || !this.state || event.channel !== this.channel || event.documentId !== this.state.documentId || event.version !== this.state.version || event.operationId !== this.openOperationId) return;
    if (!this.acceptView(event.view)) return;
    this.onEvent({...event, view:this.view});
  }
  async localView(action, zoom, playbackGeneration, hostOwned = false) {
    if (action === 'set-zoom' ? !Number.isSafeInteger(zoom) || zoom < 20 || zoom > 400 : zoom !== undefined) throw Error('invalid-control-value');
    const play = playActions.includes(action);
    if (!play && !['read','reset','fit','zoom-in','zoom-out','previous-page','next-page','set-zoom'].includes(action)) throw Error('unsupported-command');
    if (play && action !== 'play-read' ? !Number.isSafeInteger(playbackGeneration) || playbackGeneration < 0 : playbackGeneration !== undefined) throw Error('invalid-control-value');
    if (!this.state || this.failed || this.busy || this.composition || this.inputPending) throw Error('operation-in-progress');
    if (playing(this.view) && !play && action !== 'read') throw Error('operation-in-progress');
    this.busy = true;
    const documentId = this.state.documentId, version = this.state.version, opened = this.openOperationId;
    let finishRead;
    const read = action === 'read' && this.state.kind === 'docx' ? {documentId,version,opened,owner:this.manualInput,
      manual:this.state.manual ? {...this.state.manual} : null,sequence:this.state.changeSequence,ok:false,error:null,done:new Promise(resolve => { finishRead = resolve; })} : null;
    if (read) this.viewRead = read;
    try {
      const result = await this.worker({command:'local-view', channel:this.channel, operationId:crypto.randomUUID(), documentId, version, action,
        ...(action === 'set-zoom' ? {zoom} : {}), ...(play && action !== 'play-read' ? {playbackGeneration} : {})});
      if (this.failed) throw Error('surface-state-unknown-recreate-required');
      if (!this.state || this.state.documentId !== documentId || this.state.version !== version || this.openOperationId !== opened) throw Error('stale-document-version');
      if (play && !result.view?.playback) throw Error('view-unavailable');
      if (this.acceptView(result.view)) this.onEvent({type:'view', channel:this.channel, operationId:opened, documentId, version, view:this.view});
      if (read) read.ok = true;
      return result;
    } catch (error) {
      if (read) read.error = error;
      const owns = this.state?.documentId === documentId && this.state?.version === version && this.openOperationId === opened;
      const reportedFailure = play && presentationErrors.includes(error.message) && ['unsupported','error'].includes(this.view?.playback?.phase);
      // Canvas shortcuts do not have a Main request waiting to catch a startup
      // timeout. Retire this surface through the existing fatal channel instead;
      // an error view alone cannot cancel a pending native configuration callback.
      if (error.message === 'presentation-start-timeout' && !hostOwned) {
        this.failed = true;
        this.onEvent({type:'fatal',channel:this.channel,error:'engine-operation-failed'});
      }
      if (owns && !this.failed && !reportedFailure && !['stale-selection','operation-in-progress'].includes(error.message)) {
        this.view = null; this.onView(null);
        this.onEvent({type:'view', channel:this.channel, operationId:opened, documentId, version, view:null});
      }
      throw error;
    } finally {
      this.busy = false;
      if (read) { if (this.viewRead === read) this.viewRead = null; finishRead(); }
    }
  }
  async request(r, selectionCurrent) {
    if (!identity(r) || r.channel !== this.channel) throw Error('invalid-request');
    if (!previewCommands.includes(r.command)) throw Error('unsupported-command');
    if (!this.editing && !['local-view','open','edit','manual-edit','captureSelection','captureCells','validateSelection','close'].includes(r.command)) throw Error('unsupported-command');
    if (!validRequest(r)) throw Error('invalid-request');
    if (this.failed) throw Error('surface-state-unknown-recreate-required');
    // A menu can race the guest's autonomous read after mouseup. Wait only for
    // that known read, never for a write, native input, or an unknown busy owner.
    if (r.command === 'validateSelection' && this.busy && this.viewRead && typeof selectionCurrent === 'function' && !this.composition && !this.inputPending) {
      const read = this.viewRead;
      r = {...r};
      const current = () => this.state?.kind === 'docx' && read.documentId === r.documentId && read.version === r.version &&
        this.state.documentId === read.documentId && this.state.version === read.version && this.openOperationId === read.opened &&
        this.state.changeSequence === read.sequence && r.expectedChangeSequence === read.sequence && this.manualInput === read.owner &&
        (read.manual ? !!this.state.manual && ['version','origin','changeId','baseRevision','generation','frozen'].every(name => this.state.manual[name] === read.manual[name]) : !this.state.manual) && selectionCurrent();
      if (!current()) throw Error('stale-selection');
      await read.done;
      if (!read.ok) throw read.error;
      if (this.failed) throw Error('surface-state-unknown-recreate-required');
      if (!current()) throw Error('stale-selection');
    }
    if (this.busy) throw Error('operation-in-progress');
    if (r.command !== 'open' && (!this.state || r.documentId !== this.state.documentId || r.version !== this.state.version)) throw Error('stale-document-version');
    if (r.command === 'open' && this.state) throw Error('document-already-open');
    if (r.command === 'ack' && (!this.exportArtifact || this.exportArtifact.operationId !== r.exportOperationId || this.exportArtifact.sequence !== r.exportedSequence ||
      this.exportArtifact.documentId !== r.documentId || this.exportArtifact.version !== r.version)) throw Error('ack-mismatch');
    if (r.command === 'format' && (!this.manualAvailable() || r.changeId !== this.manualInput.changeId)) throw Error('native-input-unavailable');
    if (r.command === 'freeze' && (!this.manualCurrent() || r.changeId !== this.manualInput.changeId)) throw Error('native-input-unavailable');
    if (r.command === 'manual-edit' && (this.state.kind !== 'docx' || this.manualInput || this.composition || this.inputPending)) throw Error('operation-in-progress');
    if (r.command === 'freeze') {
      const owner = this.manualInput, gate = this.inputGate;
      if (this.freezeProof) {
        if (r.operationId !== this.freezeProof.operationId) throw Error('operation-in-progress');
        if (!this.state.manual.frozen || this.state.changeSequence !== this.freezeProof.sequence) { this.retireInput(); throw Error('surface-state-unknown-recreate-required'); }
        return {type:'result',command:r.command,channel:r.channel,operationId:r.operationId,documentId:r.documentId,version:r.version,
          ok:true,state:{...this.state,manual:{...this.state.manual}},freezeAck:{...this.freezeProof}};
      }
      this.inputGate = true; this.busy = true;
      let dispatched = false;
      try {
        if (this.composition) throw Error('native-composition-pending');
        await this.inputTail;
        if (!this.manualAvailable(owner)) throw Error('surface-state-unknown-recreate-required');
        // The bound worker owns native mailbox draining and its correlated
        // callback. Browser/port completion alone is never a freeze proof.
        dispatched = true;
        const result = await this.worker({...r}), state = result.state, ack = result.freezeAck, manual = state?.manual;
        if (!this.manualCurrent(owner) || !validFreeze(ack) || ack.changeId !== owner.changeId || ack.operationId !== r.operationId || ack.generation !== owner.generation ||
          state?.documentId !== owner.documentId || state.version !== owner.version || !integer(state.changeSequence) || state.changeSequence < this.state.changeSequence ||
          !integer(state.acknowledgedSequence) || state.acknowledgedSequence > state.changeSequence || state.dirty !== (state.changeSequence !== state.acknowledgedSequence) ||
          !exact(manual,['version','origin','changeId','baseRevision','generation','frozen']) || manual.version !== 2 || manual.origin !== 'manual' || manual.changeId !== owner.changeId ||
          manual.baseRevision !== owner.version || manual.generation !== owner.generation || manual.frozen !== true || ack.sequence !== state.changeSequence) {
          this.retireInput(); throw Error('surface-state-unknown-recreate-required');
        }
        this.state = state; this.freezeProof = {...ack};
        return {...result,freezeAck:{...ack}};
      } catch (error) {
        if (dispatched && !this.failed) this.retireInput();
        throw error;
      } finally { this.busy = false; if (!this.freezeProof && this.manualAvailable(owner)) this.inputGate = gate; }
    }
    if (this.manualInput && r.command === 'export' && (!this.manualCurrent() || !this.state.manual.frozen || !this.freezeProof || r.changeId !== this.manualInput.changeId ||
      !validFreeze(r.freezeAck) || !['changeId','operationId','generation','sequence'].every(name => r.freezeAck[name] === this.freezeProof[name]) ||
      this.state.changeSequence !== this.freezeProof.sequence)) throw Error('native-input-unavailable');
    if (this.manualInput && !['close','ack'].includes(r.command) && (this.composition || this.inputPending)) throw Error(this.composition ? 'native-composition-pending' : 'operation-in-progress');
    if (r.command === 'close' && this.manualInput) {
      if (!r.discard && this.composition) throw Error('native-composition-pending');
      // A discard retires the browser generation first; it does not undo an
      // already committed Core revision. Main separately rereads that revision.
      const previousGate = this.inputGate;
      this.inputGate = true; this.busy = true;
      if (r.discard) {
        this.composition = this.finalComposition = null; this.compositionKeys.clear();
        this.canvas.contentEditable = 'false';
      }
      await this.inputTail;
      if (this.failed) throw Error('surface-state-unknown-recreate-required');
      if (r.discard) this.closeInput();
      else this.inputGate = previousGate;
    }
    if (r.command === 'local-view') {
      await this.localView(r.action, r.zoom, r.playbackGeneration, true);
      return {type:'result', command:r.command, channel:r.channel, operationId:r.operationId,
        documentId:r.documentId, version:r.version, ok:true, state:{...this.state}, view:{...this.view}};
    }
    this.busy = true;
    const owner = this.manualInput, gate = this.inputGate;
    let loading = null;
    if (owner) this.inputGate = true;
    try {
      const request = {...r};
      if (r.command === 'open') {
        FS.writeFile('/tmp/analytix-office/input.' + r.kind, r.bytes);
        delete request.bytes;
      }
      const inputProof = r.command === 'manual-edit' ? await this.bindInputRuntime() : null;
      if (r.command === 'manual-edit') loading = this.armManualLoading(r,inputProof);
      const result = await this.worker(request);
      if (this.failed) throw Error('surface-state-unknown-recreate-required');
      if (r.command === 'manual-edit') {
        const state = result.state, manual = state?.manual;
        const runtime = this.inputRuntimeIdentity(), evidence = result.inputRuntime;
        if (!inputProof || !this.manualLoadingCurrent(loading) || loading.operationId !== r.operationId || loading.changeId !== r.manual.changeId || loading.removed.size !== 0 ||
          this.inputRuntime.bound !== inputProof || !runtime || runtime.threadId !== loading.threadId ||
          state?.kind !== 'docx' || state.documentId !== r.documentId || state.version !== r.version ||
          !integer(state.changeSequence) || !integer(state.acknowledgedSequence) || state.acknowledgedSequence > state.changeSequence || state.dirty !== (state.changeSequence !== state.acknowledgedSequence) ||
          !exact(manual,['version','origin','changeId','baseRevision','generation','frozen']) || manual.version !== 2 || manual.origin !== 'manual' ||
          manual.changeId !== r.manual.changeId || manual.baseRevision !== r.manual.baseRevision || manual.generation !== loading.expectedGeneration || manual.frozen !== false ||
          !exact(evidence,['assetVersion','threadMatches','generation','changeId']) || evidence.assetVersion !== INPUT_ASSET_VERSION || evidence.threadMatches !== true ||
          evidence.generation !== manual.generation || evidence.changeId !== manual.changeId) {
          this.retireInput(); throw Error('surface-state-unknown-recreate-required');
        }
        this.inputRuntime.qualified = {...runtime}; this.inputRuntime.bound = {...runtime,documentId:r.documentId,version:r.version,opened:loading.opened};
        this.lastManualGeneration = manual.generation; this.manualLoading = null;
        const fonts = result.availableFonts ?? [];
        if (!Array.isArray(fonts) || fonts.length > 256 || new Set(fonts).size !== fonts.length || fonts.some(value => !validFormat({kind:'font-family',value}))) {
          this.retireInput(); throw Error('surface-state-unknown-recreate-required');
        }
        this.fontFamilies = [...fonts]; delete result.availableFonts;
        delete result.inputRuntime;
        this.nativeWritable = true; this.editing = true;
        this.manualInput = {changeId:manual.changeId,generation:manual.generation,documentId:r.documentId,version:r.version,opened:this.openOperationId,inputGeneration:runtime.generation};
        this.inputGate = false; this.canvas.contentEditable = 'true';
        this.canvas.setAttribute?.('aria-label','Word 文档；可编辑、浏览和选择内容');
      }
      if (r.command === 'export') {
        try {
          const path = '/tmp/analytix-office/output.' + this.state.kind;
          const size = FS.stat(path).size;
          if (!Number.isSafeInteger(size) || size < 1 || size > MAX_BYTES) throw Error('export-too-large');
          result.bytes = new Uint8Array(FS.readFile(path));
          if (result.bytes.byteLength !== size || !integer(result.exportedSequence) || result.state?.documentId !== r.documentId || result.state.version !== r.version || result.exportedSequence !== result.state.changeSequence) throw Error('surface-state-unknown-recreate-required');
          // The result buffer can be transferred to Main. Retain an independent
          // exact artifact, never reread a possibly replaced output path on ACK.
          this.exportArtifact = {operationId:r.operationId,sequence:result.exportedSequence,documentId:r.documentId,version:r.version,kind:this.state.kind,bytes:new Uint8Array(result.bytes)};
          FS.unlink(path);
        } catch (error) {
          this.retireInput();
          throw Error(error.message === 'export-too-large' ? 'export-too-large' : 'surface-state-unknown-recreate-required');
        }
      }
      if (r.command === 'ack') {
        const artifact = this.exportArtifact;
        if (!artifact || artifact.operationId !== r.exportOperationId || artifact.sequence !== r.exportedSequence || artifact.documentId !== r.documentId || artifact.version !== r.version) {
          this.retireInput(); throw Error('surface-state-unknown-recreate-required');
        }
        if (r.status === 'committed') {
          try {
            if (result.state?.version !== r.persistedVersion) throw Error('ack-mismatch');
            FS.writeFile('/tmp/analytix-office/input.' + artifact.kind, artifact.bytes);
          } catch {
            this.retireInput(); throw Error('surface-state-unknown-recreate-required');
          }
          if (owner) { this.closeInput(); this.editing = false; }
        }
        this.exportArtifact = null;
      }
      if (result.state) this.state = result.state;
      if (r.command === 'edit') this.editing = true;
      if (r.command === 'open') { this.lastManualGeneration = 0; this.openOperationId = r.operationId; dispatchEvent(new Event('resize')); }
      if (r.command === 'close') {
        for (const kind of kinds) try { FS.unlink('/tmp/analytix-office/input.' + kind); } catch { /* A missing temporary in-memory file needs no cleanup. */ }
        this.closeInput(); this.lastManualGeneration = 0; this.nativeWritable = false; this.exportArtifact = null;
        this.state = this.openOperationId = this.view = null; this.editing = false; this.onView(null);
      }
      return result;
    } catch (error) {
      if (loading) { if (!this.failed) this.retireInput(); else this.closeInput(); }
      throw error;
    } finally {
      this.busy = false;
      if (owner && this.manualAvailable(owner)) this.inputGate = gate;
    }
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

/** DOCX-only drawing owner. Native endpoints confirm content; pointer position is only an anchor. */
export class OfficePointerToolbar {
  constructor(canvas, root, send, current) {
    this.canvas = canvas; this.root = root; this.send = send; this.current = current;
    this.bar = document.createElement('div'); this.bar.id = 'pointerActions'; this.bar.hidden = true;
    this.bar.setAttribute('role','toolbar'); this.bar.setAttribute('aria-label','所选文本操作'); root.appendChild(this.bar);
    canvas.addEventListener('pointerdown', event => this.pointerDown(event));
    canvas.addEventListener('pointerup', event => this.pointerUp(event));
    canvas.addEventListener('pointercancel', () => this.cancel());
    // Keep the native range when clicking this owned UI. Qt canvas input is never intercepted.
    this.bar.addEventListener('pointerdown', event => {
      if (event.isTrusted && event.button === 0 && event.target?.closest?.('button')) {
        event.preventDefault();
        if (pointerControl(event.target)) event.target.closest('button').focus({preventScroll:true});
      }
    });
    window.addEventListener('pointerdown', event => this.outsidePointer(event), true);
    for (const type of ['wheel','scroll']) window.addEventListener(type, event => { if (!this.inside(event.target)) this.cancel(); }, true);
    for (const type of ['resize','pagehide']) window.addEventListener(type, () => this.cancel(), true);
    window.addEventListener('blur', () => this.cancel());
    window.addEventListener('visibilitychange', () => { if (document.hidden) this.cancel(); }, true);
    window.addEventListener('keydown', event => this.keyDown(event), true);
    this.bar.addEventListener('focusin', event => this.reveal(event.target));
    this.bar.addEventListener('focusout', event => { if (!this.rendering && !this.inside(event.relatedTarget) && !(this.continuation && event.relatedTarget === null)) this.cancel(); });
  }
  inside(target) { return !!target?.nodeType && this.bar.contains(target); }
  outsidePointer(event) {
    // Retire only our UI; preserve Qt's event delivery and the user's new focus.
    if (event.isTrusted && event.target !== this.canvas && !this.inside(event.target)) this.cancel();
  }
  reveal(button) {
    if (!this.inside(button) || !['BUTTON','INPUT','SELECT','OUTPUT'].includes(button.tagName)) return;
    const item = button.getBoundingClientRect(), box = this.bar.getBoundingClientRect();
    if (item.top < box.top) this.bar.scrollTop += item.top - box.top;
    else if (item.bottom > box.bottom) this.bar.scrollTop += item.bottom - box.bottom;
    if (item.left < box.left) this.bar.scrollLeft += item.left - box.left;
    else if (item.right > box.right) this.bar.scrollLeft += item.right - box.right;
  }
  keyDown(event) {
    const inside = this.inside(event.target);
    if (event.key === 'Escape') {
      const origin = this.returnFocus, lease = this.lease, engine = this.current();
      const restore = event.isTrusted && inside && this.inside(document.activeElement) && document.hasFocus() && origin === this.canvas && origin.isConnected &&
        this.available() && lease?.documentId === engine.state.documentId && lease.version === engine.state.version && lease.expectedChangeSequence === engine.state.changeSequence;
      if (inside) { event.preventDefault(); event.stopImmediatePropagation(); }
      this.cancel();
      // Only an explicit Escape from our focused toolbar may return focus.
      if (restore && document.activeElement === document.body) origin.focus({preventScroll:true});
      return;
    }
    // Let browser Tab enter normally; showing the toolbar never calls focus().
    if (event.key === 'Tab') {
      if (event.isTrusted && !event.ctrlKey && !event.metaKey && !event.altKey && !this.bar.hidden && this.formatting && event.target === this.canvas && this.formatCurrent()) {
        const controls = [...this.bar.querySelectorAll('select:not(:disabled),input:not(:disabled),button:not(:disabled),output[tabindex]')];
        const control = event.shiftKey ? controls.at(-1) : controls[0];
        if (control) { event.preventDefault(); event.stopImmediatePropagation(); control.focus({preventScroll:true}); this.reveal(control); }
        return;
      }
      if (!this.bar.hidden && (inside || event.target === this.canvas || event.target === this.returnFocus)) return;
      this.cancel(); return;
    }
    if (!inside) { this.cancel(); return; }
    if (!event.isTrusted || event.isComposing || event.ctrlKey || event.metaKey || event.altKey || this.bar.hidden || !this.inside(document.activeElement)) return;
    if (event.target?.closest?.('input,select')) return;
    const direction = {ArrowRight:1,ArrowDown:1,ArrowLeft:-1,ArrowUp:-1}[event.key];
    if (!direction && !['Home','End'].includes(event.key)) return;
    const buttons = [...this.bar.querySelectorAll('button:not(:disabled)')];
    if (!buttons.length) return;
    const current = buttons.indexOf(document.activeElement);
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (current + direction + buttons.length) % buttons.length;
    event.preventDefault(); event.stopImmediatePropagation();
    buttons[index].focus({preventScroll:true}); this.reveal(buttons[index]);
  }
  viewport(view) {
    const next = JSON.stringify(view && [view.zoom,view.zoomType,view.fit,view.page,view.pages,view.playback]);
    if (this.viewIdentity !== undefined && next !== this.viewIdentity || !view) this.cancel();
    this.viewIdentity = next;
  }
  available() {
    const engine = this.current();
    return engine?.state?.kind === 'docx' && !engine.failed && !playing(engine.view) && !!engine.openOperationId;
  }
  message(phase, extra = {}) { return {...this.lease,type:'pointer-actions',phase,...extra}; }
  retire() {
    clearTimeout(this.continuation?.timer); this.continuation = null; this.formatting = null; this.resumeFocus = null;
    const lease = this.lease; this.lease = null; this.bar.hidden = true; this.bar.replaceChildren();
    if (lease) this.send({...lease,type:'pointer-actions',phase:'cancel'});
  }
  cancel() { this.retire(); this.gesture = this.confirmed = this.selected = this.returnFocus = this.gestureManual = null; }
  pointerDown(event) {
    this.cancel();
    if (!event.isTrusted || event.button !== 0 || !event.isPrimary || event.target !== this.canvas || !this.available()) return;
    const engine = this.current(); this.returnFocus = this.canvas;
    this.gestureManual = engine.state.manual?.origin === 'manual' ? {changeId:engine.state.manual.changeId,generation:engine.state.manual.generation} : null;
    this.gesture = {interactionId:crypto.randomUUID(),pointerId:event.pointerId,documentId:engine.state.documentId,version:engine.state.version,opened:engine.openOperationId};
  }
  pointerUp(event) {
    if (!event.isTrusted || event.button !== 0 || !event.isPrimary || event.target !== this.canvas || !this.gesture || event.pointerId !== this.gesture.pointerId) { this.cancel(); return; }
    this.gesture.anchor = {x:event.clientX,y:event.clientY}; this.offer();
  }
  selection(event) {
    if (this.continuation?.request) { this.selected = event.selection; return; }
    const next = event.selection, previous = this.selected, engine = this.current();
    const lease = this.lease ?? this.continuation?.lease, formatting = this.formatting ?? this.continuation?.formatting;
    const sameRange = next?.rangeLease || previous?.rangeLease
      ? sameLease(next?.rangeLease,previous?.rangeLease) && engine?.manualAvailable?.() === true && !!this.gestureManual &&
        this.gestureManual.changeId === engine.state.manual?.changeId && this.gestureManual.generation === engine.state.manual?.generation &&
        this.gestureManual.generation === next.rangeLease.generation
      : true;
    const sameFormat = next?.formatState || previous?.formatState
      ? validFormatState(next?.formatState) && validFormatState(previous?.formatState) && formatFields.every(key => next.formatState[key] === previous.formatState[key])
      : true;
    const ownerCurrent = lease
      ? lease.channel === engine?.channel && lease.operationId === engine?.openOperationId && lease.documentId === engine?.state?.documentId &&
        lease.version === engine.state.version && lease.expectedChangeSequence === engine.state.changeSequence
      : !!this.gesture && !!this.confirmed && this.gesture.documentId === engine?.state?.documentId &&
        this.gesture.version === engine.state.version && this.gesture.opened === engine.openOperationId;
    // The worker's continuous native range lease survives duplicate samples.
    // Validation may temporarily close input; action admission stays in formatCurrent.
    if (this.available() && ownerCurrent && next?.kind === 'text' && previous?.kind === 'text' &&
      next.token === (lease?.selectionToken ?? this.confirmed) && next.text?.trim() &&
      ['documentId','version','changeSequence','token','text','scope'].every(key => next[key] === previous[key]) &&
      next.documentId === engine.state.documentId && next.version === engine.state.version && next.changeSequence === engine.state.changeSequence && sameRange && sameFormat &&
      (!formatting || formatting.changeId === engine.state.manual?.changeId && sameLease(formatting.rangeLease,next.rangeLease) &&
        formatFields.every(key => formatting.formatState[key] === next.formatState?.[key]))) {
      this.selected = next; return;
    }
    this.retire(); this.confirmed = null;
    this.selected = next;
    // A drag may first collapse the native range; wait for its later confirmation.
  }
  confirm(event) {
    if (!event.eligible || !event.changed || !this.gesture || event.selectionToken !== this.selected?.token) return;
    this.confirmed = event.selectionToken; this.offer();
  }
  offer() {
    const engine = this.current(), gesture = this.gesture, selected = this.selected;
    if (!this.available() || !gesture?.anchor || !this.confirmed || selected?.token !== this.confirmed ||
      selected.documentId !== engine.state.documentId || selected.version !== engine.state.version || selected.changeSequence !== engine.state.changeSequence ||
      gesture.documentId !== engine.state.documentId || gesture.version !== engine.state.version || gesture.opened !== engine.openOperationId) return;
    this.anchor = gesture.anchor;
    this.lease = {channel:engine.channel,operationId:engine.openOperationId,documentId:engine.state.documentId,version:engine.state.version,
      expectedChangeSequence:engine.state.changeSequence,interactionId:gesture.interactionId,selectionToken:this.confirmed};
    this.gesture = this.confirmed = null;
    this.send(this.message('request'));
  }
  changed() { if (!this.continuation?.request) this.cancel(); }
  formatCurrent(formatting = this.formatting, lease = this.lease) {
    const engine = this.current(), manual = engine?.state?.manual, selected = this.selected;
    return this.available() && engine.manualAvailable?.() === true && validFormatting(formatting) && !!lease && !engine.busy && !engine.inputGate && !engine.inputPending && !engine.composition &&
      !!engine.manualInput && engine.nativeWritable && manual?.origin === 'manual' && manual.changeId === formatting.changeId && manual.frozen === false &&
      manual.generation === formatting.rangeLease.generation && engine.manualInput.changeId === manual.changeId && engine.manualInput.generation === manual.generation &&
      lease.documentId === engine.state.documentId && lease.version === engine.state.version && lease.operationId === engine.openOperationId && lease.expectedChangeSequence === engine.state.changeSequence &&
      formatting.rangeLease.sequence === lease.expectedChangeSequence && formatting.rangeLease.token === lease.selectionToken &&
      selected?.kind === 'text' && selected.token === lease.selectionToken && selected.documentId === lease.documentId && selected.version === lease.version && selected.changeSequence === lease.expectedChangeSequence &&
      sameLease(selected.rangeLease,formatting.rangeLease) && validFormatState(selected.formatState) && formatFields.every(k => selected.formatState[k] === formatting.formatState[k]);
  }
  chooseFormat(event, format) {
    if (!event.isTrusted || this.bar.hidden || this.continuation || !validFormat(format) || !this.formatCurrent()) return;
    const field = {'font-family':'fontFamily','font-size':'fontSize',bold:'bold',italic:'italic',underline:'underline'}[format.kind];
    if (this.formatting.formatState[field] === format.value) return;
    const lease = this.lease, formatting = this.formatting;
    this.continuation = {lease:{...lease},formatting,format:{...format},anchor:{...this.anchor},focus:this.inside(document.activeElement) ? document.activeElement.dataset.formatControl : null,request:null};
    this.continuation.timer = setTimeout(() => this.cancel(),65000);
    this.lease = null; this.formatting = null;
    this.bar.setAttribute('aria-busy','true');
    for (const control of this.bar.querySelectorAll('button,input,select')) control.disabled = true;
    this.send({...lease,type:'pointer-actions',phase:'format',changeId:formatting.changeId,rangeLease:{...formatting.rangeLease},format:{...format}});
  }
  beginFormat(request) {
    const next = this.continuation;
    if (!next || next.request || request.command !== 'format' || !this.formatCurrent(next.formatting,next.lease) ||
      request.channel !== next.lease.channel || request.documentId !== next.lease.documentId || request.version !== next.lease.version || request.changeId !== next.formatting.changeId ||
      !sameLease(request.rangeLease,next.formatting.rangeLease) || !validFormat(request.format) || request.format.kind !== next.format.kind || request.format.value !== next.format.value) return false;
    next.request = {...request,rangeLease:{...request.rangeLease},format:{...request.format}}; return true;
  }
  completeFormat(request, result) {
    const next = this.continuation, state = result?.state, selected = result?.selection, manual = state?.manual, engine = this.current(), current = engine?.state;
    if (!next?.request || !result || !matches(next.request,request) || !matches(request,result) || result.ok !== true || !this.available() || engine.manualAvailable?.() !== true || engine.busy || engine.inputGate || engine.inputPending || engine.composition ||
      engine.openOperationId !== next.lease.operationId || current?.documentId !== state?.documentId || current?.version !== state?.version || current?.changeSequence !== state?.changeSequence ||
      current?.manual?.changeId !== manual?.changeId || current?.manual?.generation !== manual?.generation ||
      state?.documentId !== next.lease.documentId || state.version !== next.lease.version || manual?.origin !== 'manual' || manual.changeId !== next.formatting.changeId || manual.frozen !== false ||
      manual.generation !== next.formatting.rangeLease.generation || !integer(state.changeSequence) || state.changeSequence <= next.lease.expectedChangeSequence ||
      selected?.kind !== 'text' || selected.documentId !== state.documentId || selected.version !== state.version || selected.changeSequence !== state.changeSequence ||
      !validLease(selected.rangeLease) || selected.rangeLease.generation !== manual.generation || selected.rangeLease.sequence !== state.changeSequence || selected.rangeLease.token !== selected.token || selected.token === next.lease.selectionToken ||
      !sameLease(result.rangeLease,selected.rangeLease) || !validFormatState(selected.formatState) || !validFormatState(result.formatState) || formatFields.some(k => selected.formatState[k] !== result.formatState[k])) { this.cancel(); return; }
    clearTimeout(next.timer); this.continuation = null; this.resumeFocus = next.focus;
    this.anchor = next.anchor; this.selected = selected;
    this.lease = {...next.lease,interactionId:crypto.randomUUID(),selectionToken:selected.token,expectedChangeSequence:state.changeSequence};
    this.send(this.message('request'));
  }
  renderFormatting(formatting) {
    this.formatting = formatting;
    const group = document.createElement('div'); group.className = 'pointer-format';
    const labels = formatting.labels, state = formatting.formatState, fonts = this.current().fontFamilies;
    if (Array.isArray(fonts) && fonts.length >= 2) {
      const family = document.createElement('select'); family.dataset.formatControl = 'font-family'; family.setAttribute('aria-label',labels.fontFamily); family.title = labels.fontFamily;
      if (state.fontFamily === null || !fonts.includes(state.fontFamily)) { const current = document.createElement('option'); current.value = ''; current.textContent = state.fontFamily ?? labels.mixed; current.disabled = true; current.selected = true; family.appendChild(current); }
      for (const name of fonts) { const option = document.createElement('option'); option.value = option.textContent = name; option.selected = state.fontFamily === name; family.appendChild(option); }
      family.addEventListener('change',event => { if (fonts.includes(family.value)) this.chooseFormat(event,{kind:'font-family',value:family.value}); }); group.appendChild(family);
    } else {
      const family = document.createElement('output'); family.dataset.formatControl = 'font-family'; family.tabIndex = 0; family.setAttribute('aria-label',labels.fontFamily);
      family.textContent = state.fontFamily ?? labels.mixed; family.title = labels.fontFamily + ': ' + family.textContent; group.appendChild(family);
    }
    const size = document.createElement('input'); size.type = 'number'; size.min = '6'; size.max = '96'; size.step = '0.5'; size.dataset.formatControl = 'font-size';
    size.setAttribute('aria-label',labels.fontSize); size.title = labels.fontSize; size.placeholder = labels.mixed; size.value = state.fontSize === null ? '' : String(state.fontSize);
    const applySize = event => { if (size.value !== '' && size.validity.valid) this.chooseFormat(event,{kind:'font-size',value:Number(size.value)}); };
    size.addEventListener('change',applySize); size.addEventListener('keydown',event => { if (event.key === 'Enter' && !event.isComposing) { event.preventDefault(); applySize(event); } }); group.appendChild(size);
    for (const [kind,label] of [['bold','B'],['italic','I'],['underline','U']]) {
      const button = document.createElement('button'); button.type = 'button'; button.dataset.formatControl = kind; button.textContent = label;
      button.title = labels[kind]; button.setAttribute('aria-label',labels[kind]); button.setAttribute('aria-pressed',state[kind] === null ? 'mixed' : String(state[kind]));
      button.addEventListener('click',event => this.chooseFormat(event,{kind,value:state[kind] !== true})); group.appendChild(button);
    }
    this.bar.appendChild(group);
  }
  receive(message) {
    if (!this.lease || !['channel','operationId','documentId','version','expectedChangeSequence','interactionId','selectionToken'].every(k => message[k] === this.lease[k])) return;
    if (message.phase === 'dismiss') { this.cancel(); return; }
    if (message.phase !== 'present' || !this.available() || !Array.isArray(message.actions) || message.actions.length < 1 || message.actions.length > 32) return;
    if (message.formatting !== undefined && (!validFormatting(message.formatting) || !this.formatCurrent(message.formatting))) { this.cancel(); return; }
    this.rendering = true; this.bar.replaceChildren(); this.bar.removeAttribute('aria-busy');
    if (message.formatting) this.renderFormatting(message.formatting);
    for (const action of message.actions) {
      const button = document.createElement('button'); button.type = 'button'; button.dataset.pointerAction = action.id;
      button.textContent = action.label; button.disabled = !action.enabled;
      button.addEventListener('click', event => {
        if (!event.isTrusted || !this.lease || !action.enabled || this.bar.hidden) return;
        this.bar.hidden = true; this.send(this.message('choice',{actionId:action.id}));
      });
      this.bar.appendChild(button);
    }
    this.bar.hidden = false; this.rendering = false;
    const focus = this.resumeFocus; this.resumeFocus = null;
    if (focus && document.activeElement === document.body && document.hasFocus()) this.bar.querySelector(`[data-format-control="${focus}"]`)?.focus({preventScroll:true});
    const box = this.root.getBoundingClientRect(), width = this.bar.offsetWidth, height = this.bar.offsetHeight, gap = 8;
    this.bar.style.left = Math.max(gap,Math.min(this.anchor.x - box.left,box.width - width - gap)) + 'px';
    const above = this.anchor.y - box.top - height - gap;
    this.bar.style.top = Math.max(gap,Math.min(above >= gap ? above : this.anchor.y - box.top + gap,box.height - height - gap)) + 'px';
  }
}

/** Default demo transport accepts ONLY the same-origin parent; opaque/null origins are never trusted. */
function installSurface() {
  const canvas = document.getElementById('qtcanvas');
  if (!canvas) return;
  canvas.addEventListener('contextmenu', e => { e.preventDefault(); e.stopImmediatePropagation(); });
  let bound = false, hostPort, channel, engine, pointerToolbar;
  const status = document.getElementById('status'), selectionText = document.getElementById('selection');
  const preview = document.getElementById('preview'), footer = document.getElementById('footer');
  let viewTimer, viewReadTimer;
  function clearViewTimers() { clearTimeout(viewTimer); clearTimeout(viewReadTimer); }
  async function applyView(action) {
    if (!engine?.state || engine.failed || playing(engine.view)) return;
    if (engine.busy) { scheduleView(action); return; }
    try { await engine.localView(action); if (action !== 'read') scheduleRead(); }
    catch { status.textContent = '只读预览 · 视图调整不可用'; }
  }
  function sameViewGeneration(documentId, version, opened, generation) {
    return !!engine?.state && engine.state.documentId === documentId && engine.state.version === version && engine.openOperationId === opened && engine.view?.playback?.generation === generation;
  }
  function scheduleView(action) {
    clearTimeout(viewTimer);
    const documentId = engine?.state?.documentId, version = engine?.state?.version, opened = engine?.openOperationId;
    const generation = engine?.view?.playback?.generation;
    viewTimer = setTimeout(() => { if (sameViewGeneration(documentId, version, opened, generation)) void applyView(action); }, 120);
  }
  function scheduleRead() {
    clearTimeout(viewReadTimer);
    if (playing(engine?.view)) return;
    const documentId = engine?.state?.documentId, version = engine?.state?.version, opened = engine?.openOperationId;
    const generation = engine?.view?.playback?.generation;
    viewReadTimer = setTimeout(() => { if (sameViewGeneration(documentId, version, opened, generation)) void applyView('read'); }, 180);
  }
  for (const type of ['keyup','mouseup','wheel']) canvas.addEventListener(type, scheduleRead, {passive:true});
  addEventListener('resize', () => { if (engine?.state && !playing(engine.view) && (!engine.view || engine.view.fit)) scheduleView(engine.view ? 'fit' : 'reset'); });
  function loading(message) { status.textContent = message; preview.setAttribute('aria-busy','true'); footer.dataset.error = 'false'; }
  function render(result) {
    if (!engine?.state || playing(engine.view) || result?.type === 'loading') clearViewTimers();
    if (result?.command === 'open' && result.ok) scheduleView('reset');
    if (result?.type === 'loading') { loading('正在打开文档…'); return; }
    preview.setAttribute('aria-busy','false');
    footer.dataset.error = String(result?.ok === false);
    if (result?.ok === false) {
      status.textContent = result.error === 'native-composition-pending' ? '请先完成当前输入法组合，再保存。' : result.error === 'native-input-unavailable' ? '当前输入尚不能确认完成，文档未保存。' : result.error === 'invalid-control-value' ? '输入内容不受支持，请使用纯文本。' : presentationErrors.includes(result.error) ? '当前原生引擎未能完成放映；原件未修改。' : result.error === 'unsupported-format-fidelity' ? '当前引擎无法确认此文档的中西文字号保真，原件未保存。可继续预览。' : result.error === 'unsupported-command' ? '只读预览不支持此操作' : '预览未能完成，请重新打开';
      return;
    }
    if (!engine?.state) { status.textContent = '等待打开文档'; selectionText.textContent = ''; return; }
    document.getElementById('kind').textContent = {docx:'文档',xlsx:'表格',pptx:'演示文稿'}[engine.state.kind] || '预览';
    const show = engine.view?.playback;
    status.textContent = playing(engine.view) ? `${show.phase === 'paused' ? '已暂停' : '放映中'} · ${show.slideIndex < 0 ? '—' : show.slideIndex + 1} / ${show.slideCount}` : engine.manualInput ? engine.state.dirty ? '修改待保存' : '可编辑文档' : engine.state.dirty ? 'AI 修改待保存' : '只读预览';
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
          if (event.type === 'selection') pointerToolbar?.selection(event);
          else if (event.type === 'view') pointerToolbar?.viewport(event.view);
          else if (event.type === 'changed') pointerToolbar?.changed();
          else if (['fatal','save-requested'].includes(event.type)) pointerToolbar?.cancel();
          render(event);
          bridge.send(event);
        });
        engine.onInputError = error => render({ok:false,error});
        pointerToolbar = new OfficePointerToolbar(canvas,preview,message => bridge.send(message),() => engine);
        engine.onSelectionIdentity = event => pointerToolbar.confirm(event);
        loading('正在加载预览…');
        try { await engine.start(channel); bridge.send({type:'ready', channel, protocolVersion:1}); render(); }
        catch { render({ok:false, error:'engine-load-failed'}); bridge.send({type:'fatal', channel, error:'engine-load-failed'}); }
        return;
      }
      if (bound && r?.type === 'pointer-actions' && r.channel === channel) { pointerToolbar?.receive(r); return; }
      if (!bound || !identity(r) || r.channel !== channel) return;
      const continuingFormat = r.command === 'format' && pointerToolbar?.beginFormat(r);
      if (!continuingFormat && !['captureSelection','validateSelection'].includes(r.command) && !(r.command === 'local-view' && ['read','play-read'].includes(r.action))) pointerToolbar?.cancel();
      const selectionLease = r.command === 'validateSelection' ? pointerToolbar?.lease ?? (!pointerToolbar?.continuation?.request ? pointerToolbar?.continuation?.lease : null) : null;
      const selectionCurrent = selectionLease && ['channel','documentId','version','expectedChangeSequence','selectionToken'].every(name => selectionLease[name] === r[name])
        ? () => (pointerToolbar.lease === selectionLease || !pointerToolbar.continuation?.request && pointerToolbar.continuation?.lease === selectionLease) &&
          pointerToolbar.selected?.token === selectionLease.selectionToken && selectionLease.operationId === engine.openOperationId : undefined;
      try { if (r.command === 'open') loading('正在打开文档…'); const result = await engine.request(r,selectionCurrent); render(result); bridge.send(result); if (continuingFormat) pointerToolbar.completeFormat(r,result); }
      catch (error) {
        const known = ['typed-mutation-failed','stale-selection','unsupported-selection','unsupported-format-fidelity','invalid-control-value','invalid-request','already-bound','engine-operation-failed','engine-load-failed',
          'document-already-open','open-failed','stale-document-version','export-awaiting-ack','ack-mismatch',
          'unsaved-changes','command-unavailable','unsupported-command','native-controls-hide-failed',
          'native-composition-pending','native-input-unavailable',
          'view-unavailable','unsupported-local-control','operation-in-progress','export-too-large','engine-timeout-state-unknown','surface-state-unknown-recreate-required', ...presentationErrors];
        const result = {type:'result', command:r.command, channel, operationId:r.operationId,
          documentId:r.documentId, version:r.version, ok:false,
          error:known.includes(error?.message) ? error.message : 'engine-operation-failed'};
        pointerToolbar?.cancel(); render(result); bridge.send(result);
      }
    });
  }
  addEventListener('message', e => {
    if (bridge || bound || e.source !== parent || parent === window || e.origin === 'null' || e.origin !== location.origin) return;
    if (e.data?.type !== 'analytix-office-bind' || e.data.protocolVersion !== 1 || !token(e.data.channel) || e.ports.length !== 1) return;
    bound = true; hostPort = e.ports[0]; channel = e.data.channel;
    engine = new OfficeEngineSurface(canvas, event => { render(event); hostPort.postMessage(event); });
    engine.onInputError = error => render({ok:false,error});
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
      if (['changed','selection','view'].includes(r.type)) onEvent(r);
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
