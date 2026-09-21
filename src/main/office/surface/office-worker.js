/* global Module */
// First-party adapter using zetajs 1.2.0 public UNO APIs; see README provenance.
'use strict';
Module.zetajs.then(zeta => {
  const css = zeta.uno.com.sun.star;
  const context = zeta.getUnoComponentContext();
  const desktop = css.frame.Desktop.create(context);
  const filters = {docx:'Office Open XML Text', xlsx:'Calc MS Excel 2007 XML', pptx:'Impress MS PowerPoint 2007 XML'};
  // com.sun.star.document.MacroExecMode IDL: NEVER_EXECUTE = 0.
  // Keep this explicit even in builds without a macro runtime.
  const NEVER_EXECUTE = 0;
  const prop = (Name, Value) => new css.beans.PropertyValue({Name, Value});
  const emit = data => zeta.mainPort.postMessage(data);
  let channel, model, controller, active, modifyListener, selectionListener;
  let handleSequence = 0, suppressChanges = false;
  const handles = new Map();
  const state = () => ({documentId:active.documentId, version:active.version, kind:active.kind,
    changeSequence:active.sequence, acknowledgedSequence:active.acknowledged, dirty:active.sequence !== active.acknowledged});
  const event = (type, payload) => emit({type, channel, documentId:active.documentId, version:active.version,
    operationId:active.openOperationId, ...payload});
  const boundedText = value => {
    const text = String(value ?? '');
    let end = Math.min(text.length, 4096);
    if (end < text.length && /[\uD800-\uDBFF]/.test(text[end - 1])) end--;
    return text.slice(0, end);
  };
  function captured(value, complete = true) {
    const text = String(value ?? '');
    return {capturedCharacters:boundedText(text).length, totalCharacters:text.length, truncated:text.length > 4096 || !complete, unit:'utf-16', complete:complete && text.length <= 4096};
  }
  function remember(target, text, kind) {
    // Session-local opaque references: never pointers or guessed text offsets.
    const token = 'selection_' + active.openOperationId.replace(/[^A-Za-z0-9_-]/g, '_') + '_' + (++handleSequence);
    handles.set(token, {target, text, kind, version:active.version, sequence:active.sequence});
    while (handles.size > 32) handles.delete(handles.keys().next().value);
    return token;
  }
  function rememberWorkbook(target, text, kind, ranges, cells) {
    const token=remember(target,text,kind);
    if (ranges.length===1) handles.get(token).workbook=JSON.parse(JSON.stringify({...ranges[0],cells}));
    return token;
  }
  const formulaKey = value => {
    let out='',quoted=false;const source=String(value).replace(/^=/,'');
    for(let i=0;i<source.length;i++){let c=source[i];if(c==='"'){if(quoted&&source[i+1]==='"'){out+='""';i++;continue;}quoted=!quoted;}if(!quoted&&c===';')c=',';out+=c;}return out;
  };
  const nativeFormula = value => {
    let out='=',quoted=false;const source=formulaKey(value);
    for(let i=0;i<source.length;i++){let c=source[i];if(c==='"'){if(quoted&&source[i+1]==='"'){out+='""';i++;continue;}quoted=!quoted;}if(!quoted&&c===',')c=';';out+=c;}return out;
  };
  function workbookCell(sheet, before) {
    const cell=sheet.getCellByPosition(before.column,before.row), nativeType=cell.getType();
    const type=typeof nativeType==='number'?nativeType:nativeType.value;
    return {sheet:before.sheet,column:before.column,row:before.row,text:cell.getString(),formula:cell.getFormula(),value:cell.getValue(),valueType:['empty','number','text','formula'][type],numberFormat:Number(cell.getPropertyValue('NumberFormat')),rowVisible:sheet.getRows().getByIndex(before.row).getPropertyValue('IsVisible'),columnVisible:sheet.getColumns().getByIndex(before.column).getPropertyValue('IsVisible'),merged:cell.getIsMerged()};
  }
  function workbookFormulaReferences(expression, scope, budget) {
    let input=formulaKey(expression), refs=[];
    const functions=['SUM','AVERAGE','MIN','MAX','COUNT','COUNTA','COUNTIF','SUMIF','IF','ROUND','ABS'];
    const point=value=>{let col=0;const match=/^\$?([A-Z]+)\$?([0-9]+)$/i.exec(value);for(const c of match[1].toUpperCase())col=col*26+c.charCodeAt(0)-64;const row=Number(match[2])-1;col--;if(col<scope.startColumn||col>scope.endColumn||row<scope.startRow||row>scope.endRow)throw Error('invalid-control-value');return {col,row};};
    while(input.length){input=input.trimStart();if(!input)break;
      const literal=/^"(?:[^"]|"")*"/.exec(input);if(literal){input=input.slice(literal[0].length);continue;}
      const sheet=/^'((?:[^']|'')+)'!/.exec(input);if(sheet){if(sheet[1].replace(/''/g,"'")!==scope.sheetName)throw Error('invalid-control-value');input=input.slice(sheet[0].length);continue;}
      const numeric=/^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?/.exec(input);if(numeric){if(!Number.isFinite(Number(numeric[0])))throw Error('invalid-control-value');input=input.slice(numeric[0].length);continue;}
      const ref=/^\$?[A-Z]{1,3}\$?[1-9][0-9]*/i.exec(input);if(ref){const a=point(ref[0]);input=input.slice(ref[0].length);let b=a;const end=/^\s*:\s*(\$?[A-Z]{1,3}\$?[1-9][0-9]*)/i.exec(input);if(end){b=point(end[1]);input=input.slice(end[0].length);}if(b.col<a.col||b.row<a.row)throw Error('invalid-control-value');for(let row=a.row;row<=b.row;row++)for(let col=a.col;col<=b.col;col++){if(++budget.work>1000000)throw Error('invalid-control-value');refs.push((row-scope.startRow)*(scope.endColumn-scope.startColumn+1)+col-scope.startColumn);}continue;}
      const name=/^[A-Za-z_][A-Za-z0-9_]*/.exec(input);if(name){input=input.slice(name[0].length);if(input.startsWith('!')){if(name[0]!==scope.sheetName)throw Error('invalid-control-value');input=input.slice(1);continue;}if(!['TRUE','FALSE'].includes(name[0].toUpperCase())&&(!functions.includes(name[0].toUpperCase())||!input.trimStart().startsWith('(')))throw Error('invalid-control-value');continue;}
      if(/^[+*/^&%=(),:<>-]/.test(input)){input=input.slice(1);continue;}
      throw Error('invalid-control-value');
    }return refs;
  }
  function validateWorkbookFormulas(scope){
    const budget={work:0},graph=scope.cells.map(c=>c.valueType==='formula'?workbookFormulaReferences(c.formula,scope,budget):[]),state=[],depths=[];
    const visit=(i,depth)=>{if(depth>64||state[i]===1)throw Error('invalid-control-value');if(state[i]===2)return depths[i];state[i]=1;let height=1;for(const j of graph[i])if(scope.cells[j].valueType==='formula')height=Math.max(height,visit(j,depth+1)+1);if(height>64)throw Error('invalid-control-value');state[i]=2;depths[i]=height;return height;};
    for(let i=0;i<graph.length;i++)visit(i,1);
  }
  function replaceWorkbook(r) {
    const handle=targetFor(r), review=r.workbook, saved=handle.workbook;
    if(active.kind!=='xlsx'||!saved||!review||!review.before||!review.after)throw Error('unsupported-selection');
    const before=review.before,after=review.after,keys=['sheet','sheetName','startColumn','startRow','endColumn','endRow'];
    if(!keys.every(k=>before[k]===saved[k]&&after[k]===saved[k])||before.cells.length!==saved.cells.length||after.cells.length!==saved.cells.length||saved.cells.length===0||saved.cells.length>256)throw Error('stale-selection');
    const width=before.endColumn-before.startColumn+1,height=before.endRow-before.startRow+1;
    if(width<1||height<1||width*height!==saved.cells.length)throw Error('unsupported-selection');
    const sheet=model.getSheets().getByIndex(before.sheet), targets=[];
    if(sheet.getName()!==before.sheetName)throw Error('stale-selection');
    for(let i=0;i<before.cells.length;i++){
      const b=before.cells[i],a=after.cells[i],old=saved.cells[i],current=workbookCell(sheet,b);
      if(!Object.keys(current).every(k=>current[k]===b[k]&&b[k]===old[k]))throw Error('stale-selection');
      if(b.column!==before.startColumn+i%width||b.row!==before.startRow+Math.floor(i/width)||!b.rowVisible||!b.columnVisible||b.merged||a.sheet!==b.sheet||a.row!==b.row||a.column!==b.column||a.numberFormat!==b.numberFormat||!a.rowVisible||!a.columnVisible||a.merged)throw Error('unsupported-selection');
      if(!['empty','text','number','formula'].includes(a.valueType)||!Number.isFinite(a.value)||typeof a.text!=='string'||a.text.length>4096||typeof a.formula!=='string'||a.formula.length>4096)throw Error('invalid-control-value');
      if(a.valueType==='text'&&!['empty','text'].includes(b.valueType)||a.valueType==='empty'&&b.valueType!=='empty')throw Error('invalid-control-value');
      if(a.valueType==='formula'&&(!a.formula.startsWith('=')||a.formula.length>1025))throw Error('invalid-control-value');
      targets.push(sheet.getCellByPosition(b.column,b.row));
    }
    validateWorkbookFormulas(after);
    try {
      mutate(()=>{
        for(let i=0;i<targets.length;i++){
          const c=after.cells[i],old=before.cells[i];
          if(c.valueType===old.valueType&&(c.valueType==='formula'?formulaKey(c.formula)===formulaKey(old.formula):c.valueType==='number'?c.value===old.value:c.text===old.text))continue;
          if(c.valueType==='number')targets[i].setValue(c.value);
          else if(c.valueType==='formula')targets[i].setFormula(nativeFormula(c.formula));
          else targets[i].setString(c.text);
        }
        model.calculateAll();
        for(let i=0;i<targets.length;i++){
          const c=after.cells[i],actual=workbookCell(sheet,c);
          if(actual.valueType!==c.valueType||actual.numberFormat!==c.numberFormat||actual.merged||!actual.rowVisible||!actual.columnVisible||c.valueType==='number'&&actual.value!==c.value||c.valueType==='text'&&actual.text!==c.text||c.valueType==='formula'&&(formulaKey(actual.formula)!==formulaKey(c.formula)||targets[i].getError()!==0))throw Error('typed-mutation-failed');
        }
      });
    }catch {active.mutationFailed=true;handles.clear();throw Error('typed-mutation-failed');}
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
  const enumNumber = value => typeof value==='number'?value:value?.value;
  function presentationSnapshot(pageIndex, targetShapeIndex) {
    if(active.kind!=='pptx')throw Error('unsupported-selection');
    const pages=model.getDrawPages();
    if(pages.getCount()>512||pageIndex<0||pageIndex>=pages.getCount())throw Error('unsupported-selection');
    const page=pages.getByIndex(pageIndex),count=page.getCount();
    if(count<1||count>64)throw Error('unsupported-selection');
    const shapes=[];
    for(let i=0;i<count;i++){
      const shape=page.getByIndex(i),kind=({'com.sun.star.drawing.RectangleShape':'rectangle','com.sun.star.drawing.EllipseShape':'ellipse','com.sun.star.drawing.TextShape':'text'})[shape.getShapeType()];
      if(!kind||enumNumber(shape.getPropertyValue('FillStyle'))!==1||Number(shape.getPropertyValue('RotateAngle'))!==0||Number(shape.getPropertyValue('ShearAngle'))!==0)throw Error('unsupported-selection');
      // These properties must belong to this shape, not a template/master style.
      for(const key of ['FillStyle','FillColor'])if(enumNumber(shape.getPropertyState(key))!==0)throw Error('unsupported-selection');
      const info=shape.getPropertySetInfo();
      for(const key of ['IsMirrored','MirroredX','MirroredY'])if(info.hasPropertyByName(key)&&shape.getPropertyValue(key)!==false)throw Error('unsupported-selection');
      const position=shape.getPosition(),size=shape.getSize(),fill=Number(shape.getPropertyValue('FillColor'));
      if(!Number.isInteger(fill)||fill<0||fill>0xffffff)throw Error('unsupported-selection');
      shapes.push({shapeIndex:i,kind,name:shape.getName(),text:shape.getString(),x100thMm:position.X,y100thMm:position.Y,width100thMm:size.Width,height100thMm:size.Height,fillRGB:'#'+fill.toString(16).padStart(6,'0')});
    }
    const result={pageIndex,targetShapeIndex,pageWidth100thMm:Number(page.getPropertyValue('Width')),pageHeight100thMm:Number(page.getPropertyValue('Height')),shapes};
    if(!validPresentationSelection(result))throw Error('unsupported-selection');
    return result;
  }
  function replacePresentation(r) {
    const handle=targetFor(r),review=r.presentation;
    if(!handle.presentation||!validPresentationReview(review)||JSON.stringify(handle.presentation)!==JSON.stringify(review.before))throw Error('unsupported-selection');
    const before=review.before,current=presentationSnapshot(before.pageIndex,before.targetShapeIndex);
    const page=model.getDrawPages().getByIndex(before.pageIndex),target=page.getByIndex(before.targetShapeIndex);
    if(!zeta.sameUnoObject(target,handle.target)||!zeta.sameUnoObject(controller.getCurrentPage(),page)||JSON.stringify(current)!==JSON.stringify(before))throw Error('stale-selection');
    const a=before.shapes[before.targetShapeIndex],b=review.after.shapes[before.targetShapeIndex];
    mutate(()=>{
      if(a.fillRGB!==b.fillRGB)setViewProperty(target,'FillColor',parseInt(b.fillRGB.slice(1),16));
      else {
        target.setPosition(new css.awt.Point({X:b.x100thMm,Y:b.y100thMm}));
        target.setSize(new css.awt.Size({Width:b.width100thMm,Height:b.height100thMm}));
      }
      if(JSON.stringify(presentationSnapshot(before.pageIndex,before.targetShapeIndex))!==JSON.stringify(review.after))throw Error('typed-mutation-failed');
    });
  }
  function requirePlainTextRange(target) {
    // setString replaces native structure, not just its displayed characters.
    // Fields, links and unknown portions need a typed structural edit contract.
    // Enumerate the captured range itself so unrelated paragraph content is kept.
    try {
      const paragraphs = target.createEnumeration();
      let budget = 8192, textPortions = 0;
      while (paragraphs.hasMoreElements()) {
        if (--budget < 0) throw Error('unsupported-selection');
        const portions = paragraphs.nextElement().createEnumeration();
        while (portions.hasMoreElements()) {
          if (--budget < 0) throw Error('unsupported-selection');
          const portion = portions.nextElement();
          if (portion.getPropertyValue('TextPortionType') !== 'Text' ||
              portion.getPropertyValue('HyperLinkURL') !== '') throw Error('unsupported-selection');
          textPortions++;
        }
      }
      if (!textPortions) throw Error('unsupported-selection');
    } catch { throw Error('unsupported-selection'); }
  }
  function selection() {
    const base = {documentId:active.documentId, version:active.version, changeSequence:active.sequence};
    try {
      const selected = controller.getSelection();
      if (active.kind === 'docx') {
        const text = selected.getCount ? Array.from({length:Math.min(selected.getCount(), 16)}, (_, i) => selected.getByIndex(i).getString()).join('\n') : selected.getString();
        const target = selected.getCount ? (selected.getCount() === 1 ? selected.getByIndex(0) : null) : selected;
        return {...base, kind:'text', scope:'session-text-range-at-version-and-change-sequence', text:boundedText(text), capture:captured(text, !selected.getCount || selected.getCount() <= 16), ...(target ? {token:remember(target, text, 'text')} : {})};
      }
      if (active.kind === 'xlsx') {
        let addresses;
        if (selected.getRangeAddresses) addresses = selected.getRangeAddresses();
        else if (selected.getRangeAddress) addresses = [selected.getRangeAddress()];
        else { const a = selected.getCellAddress(); addresses = [{Sheet:a.Sheet, StartColumn:a.Column, EndColumn:a.Column, StartRow:a.Row, EndRow:a.Row}]; }
        const ranges = addresses.slice(0, 64).map(a => ({sheet:a.Sheet, sheetName:model.getSheets().getByIndex(a.Sheet).getName(), startColumn:a.StartColumn, startRow:a.StartRow, endColumn:a.EndColumn, endRow:a.EndRow}));
        const cells = [], rawTexts = []; let complete = addresses.length <= 64, totalCells = 0;
        for (const a of addresses) totalCells += (a.EndColumn-a.StartColumn+1)*(a.EndRow-a.StartRow+1);
        for (const a of addresses.slice(0, 64)) {
          const sheet = model.getSheets().getByIndex(a.Sheet);
          for (let row = a.StartRow; row <= a.EndRow && cells.length < 256; row++) {
            for (let column = a.StartColumn; column <= a.EndColumn && cells.length < 256; column++) {
              const cell = sheet.getCellByPosition(column, row);
              const nativeType = cell.getType();
              const type = typeof nativeType === 'number' ? nativeType : nativeType.value;
              if (![0,1,2,3].includes(type)) throw Error('unsupported-selection');
              rawTexts.push(cell.getString());
              if (cell.getFormula().length > 4096) complete = false;
              cells.push({sheet:a.Sheet, column, row, text:boundedText(cell.getString()), formula:boundedText(cell.getFormula()), value:cell.getValue(), valueType:['empty','number','text','formula'][type], numberFormat:Number(cell.getPropertyValue('NumberFormat')), rowVisible:sheet.getRows().getByIndex(row).getPropertyValue('IsVisible'), columnVisible:sheet.getColumns().getByIndex(column).getPropertyValue('IsVisible'), merged:cell.getIsMerged()});
            }
          }
        }
        complete = complete && cells.length === totalCells;
        const text = rawTexts.join('\t');
        const one = addresses.length === 1 && totalCells === 1;
        const a = addresses[0];
        const target = one ? model.getSheets().getByIndex(a.Sheet).getCellByPosition(a.StartColumn, a.StartRow) : selected;
        return {...base, kind:'cells', scope:'sheet-range-address-at-version-and-change-sequence', ranges, cells, text:boundedText(text), capture:captured(text, complete), token:rememberWorkbook(target, one ? target.getString() : null, one ? 'cell' : 'range', ranges, cells)};
      }
      const page = controller.getCurrentPage(), pages = model.getDrawPages();
      let pageIndex = -1;
      for (let i = 0; i < Math.min(pages.getCount(), 512); ++i) if (zeta.sameUnoObject(pages.getByIndex(i), page)) { pageIndex = i; break; }
      const shapes = [], rawTexts = [];
      const count = selected.getCount ? Math.min(selected.getCount(), 64) : 0;
      for (let i = 0; i < count; ++i) {
        const shape = selected.getByIndex(i); let shapeIndex = -1, text = '', name = '';
        for (let j = 0; j < Math.min(page.getCount(), 2048); ++j) if (zeta.sameUnoObject(page.getByIndex(j), shape)) { shapeIndex = j; break; }
        try { const raw = shape.getString(); rawTexts.push(raw); text = boundedText(raw); } catch { /* This selection type does not expose the optional text/name interface. */ }
        try { name = boundedText(shape.getName()); } catch { /* This selection type does not expose the optional text/name interface. */ }
        if (pageIndex >= 0 && shapeIndex >= 0) shapes.push({pageIndex, shapeIndex, name, type:shape.getShapeType(), text});
      }
      const text = rawTexts.join('\n');
      const target = count === 1 && shapes.length === 1 ? selected.getByIndex(0) : null;
      let presentation;
      if(target)try {presentation=presentationSnapshot(pageIndex,shapes[0].shapeIndex);}catch { /* Unsupported shapes retain the existing read-only/text selection. */ }
      const token=target&&(presentation||text.length>0)?remember(target,text,'shape'):null;
      if(token&&presentation)handles.get(token).presentation=presentation;
      return {...base, kind:'shapes', scope:'page-and-shape-index-at-version-and-change-sequence', shapes, capture:captured(text, !selected.getCount || selected.getCount() <= 64), ...(token?{token}:{}),...(presentation?{presentation}:{})};
    } catch {
      return {...base, kind:'unavailable', scope:'engine selection interface unavailable in this view'};
    }
  }
  // Local view-only allowlist. These properties belong to the controller/view,
  // never the model, document text, cells or draw-page collection.
  const viewActions = ['read','reset','fit','zoom-in','zoom-out','previous-page','next-page'];
  const viewProperties = () => active.kind === 'docx' ? controller.getViewSettings() : controller;
  function setViewProperty(target, name, value) {
    const type = target.getPropertySetInfo().getPropertyByName(name).Type;
    target.setPropertyValue(name, new zeta.Any(type, value));
  }
  function viewState() {
    const target = viewProperties();
    const zoom = Number(target.getPropertyValue('ZoomValue'));
    const zoomType = Number(target.getPropertyValue('ZoomType'));
    if (!Number.isFinite(zoom) || zoom <= 0 || zoom > 1000) throw Error('view-unavailable');
    const result = {zoom, zoomType, fit:active.viewFit, page:null, pages:null, readOnly:model.isReadonly(), modified:model.isModified()};
    if (active.kind === 'pptx') {
      const pages = model.getDrawPages(), current = controller.getCurrentPage();
      const count = pages.getCount();
      if (count < 1 || count > 512) throw Error('view-unavailable');
      for (let i = 0; i < count; i++) if (zeta.sameUnoObject(pages.getByIndex(i), current)) { result.page = i + 1; break; }
      if (result.page === null) throw Error('view-unavailable');
      result.pages = count;
    }
    return result;
  }
  function localView(action) {
    if (!viewActions.includes(action)) throw Error('unsupported-command');
    if (!model.isReadonly()) throw Error('view-unavailable');
    const target = viewProperties();
    if (action === 'reset' || action === 'fit') {
      active.viewFit = active.kind !== 'xlsx';
      setViewProperty(target, 'ZoomType', active.kind === 'docx' ? 1 : active.kind === 'pptx' ? 2 : 3);
      if (active.kind === 'xlsx') setViewProperty(target, 'ZoomValue', 100);
    } else if (action === 'zoom-in' || action === 'zoom-out') {
      const current = viewState();
      const value = Math.min(400, Math.max(20, Math.round(current.zoom / 10) * 10 + (action === 'zoom-in' ? 10 : -10)));
      setViewProperty(target, 'ZoomType', 3);
      setViewProperty(target, 'ZoomValue', value);
      active.viewFit = false;
    } else if (action === 'previous-page' || action === 'next-page') {
      if (active.kind !== 'pptx') throw Error('unsupported-command');
      const current = viewState();
      const index = Math.min(current.pages - 1, Math.max(0, current.page - 1 + (action === 'next-page' ? 1 : -1)));
      controller.setCurrentPage(model.getDrawPages().getByIndex(index));
      if (active.viewFit) setViewProperty(target, 'ZoomType', 2);
    }
    return viewState();
  }
  function configurePreviewStage() {
    const provider = context.getServiceManager().createInstanceWithContext('com.sun.star.configuration.ConfigurationProvider', context);
    const settings = provider.createInstanceWithArguments('com.sun.star.configuration.ConfigurationUpdateAccess', [prop('nodepath', '/org.openoffice.Office.UI/ColorScheme')]);
    const schemeName = settings.getPropertyValue('CurrentColorScheme');
    const scheme = settings.getByName('ColorSchemes').getByName(schemeName);
    const background = scheme.getByName('AppBackground');
    setViewProperty(background, 'Color', 0xf7f7f7);
    settings.commitChanges();
    if (background.getPropertyValue('Color') !== 0xf7f7f7) throw Error('view-unavailable');
  }
  function close() {
    if (model) {
      if (modifyListener) model.removeModifyListener(modifyListener);
      if (selectionListener) controller.removeSelectionChangeListener(selectionListener);
      model.close(true);
    }
    model = controller = active = modifyListener = selectionListener = null;
    handles.clear();
  }
  function requireEditing() { if(active.mutationFailed)throw Error('typed-mutation-failed'); if (!active.editing || model.isReadonly() !== true) throw Error('unsupported-command'); }
  function targetFor(r) {
    requireEditing();
    const handle = handles.get(r.selectionToken);
    if (!handle || handle.version !== active.version || handle.sequence !== active.sequence || r.expectedChangeSequence !== active.sequence) throw Error('stale-selection');
    if (handle.text !== null && handle.target.getString() !== handle.text) throw Error('stale-selection');
    return handle;
  }
  function mutate(operation) {
    requireEditing();
    const before = active.sequence;
    try { operation(); } catch {
      // UNO may change part of a range before throwing. Its state is unknown;
      // no later command may export or continue editing this native model.
      active.mutationFailed = true;
      throw Error('typed-mutation-failed');
    } finally {
      if (active.sequence === before) { active.sequence++; event('changed', {state:state()}); }
      handles.clear();
    }
  }
  function openModel(r) {
        if (active || !filters[r.kind]) throw Error('document-already-open');
        // Configuration lives in this isolated browser engine's virtual FS only.
        try { configurePreviewStage(); } catch { /* Optional stage styling must not block read-only preview. */ }
        model = desktop.loadComponentFromURL('file:///tmp/analytix-office/input.' + r.kind, '_default', 0,
          [prop('MacroExecutionMode', NEVER_EXECUTE), prop('UpdateDocMode', 0), prop('ReadOnly', true), prop('LockEditDoc', true), prop('LockSave', true), prop('LockExport', true), prop('PickListEntry', false)]);
        if (!model) throw Error('open-failed');
        if (model.isReadonly() !== true) { close(); throw Error('open-failed'); }
        controller = model.getCurrentController();
        if (r.kind === 'docx') {
          try {
            const view = controller.getViewSettings();
            setViewProperty(view, 'ShowHoriRuler', false); setViewProperty(view, 'ShowVertRuler', false);
            if (view.getPropertyValue('ShowHoriRuler') !== false || view.getPropertyValue('ShowVertRuler') !== false) throw Error('view-unavailable');
          } catch { /* This optional view preference has no document mutation fallback. */ }
        }
        controller.getFrame().getContainerWindow().FullScreen = true;
        // XLayoutManager.setVisible(false) hides every managed native UI element.
        // The product exposes a read-only preview, with no mutation dispatch API.
        const layout = controller.getFrame().LayoutManager;
        layout.setVisible(false);
        if (layout.isVisible()) throw Error('native-controls-hide-failed');
        active = {documentId:r.documentId, version:r.version, kind:r.kind, openOperationId:r.operationId, sequence:0, acknowledged:0, editing:false, viewFit:r.kind !== 'xlsx'};
        modifyListener = zeta.unoObject([css.util.XModifyListener], {disposing() {}, modified() {
          if (!active || suppressChanges) return;
          // Writer view/layout notifications also use XModifyListener. Only a
          // confirmed read-only, unmodified model is a non-edit notification.
          // Failed reads and every other state still reach Main's fail-closed gate.
          try { if (model.isReadonly() === true && model.isModified() === false) return; }
          catch { /* Unknown native state must not be classified as an unmodified view. */ }
          active.sequence++; handles.clear();
          event('changed', {state:state()});
        }});
        selectionListener = zeta.unoObject([css.view.XSelectionChangeListener], {disposing() {}, selectionChanged() {
          if (active) event('selection', {selection:selection()});
        }});
        model.addModifyListener(modifyListener);
        controller.addSelectionChangeListener(selectionListener);
  }
  zeta.mainPort.onmessage = ({data:r}) => {
    if (!r || typeof r !== 'object') return;
    if (r.command === 'bind' && !channel && typeof r.channel === 'string') { channel = r.channel; emit({type:'ready', channel}); return; }
    if (r.channel !== channel || !channel) return;
    const reply = payload => emit({type:'result', command:r.command, channel, operationId:r.operationId, documentId:r.documentId, version:r.version, ...payload});
    try {
      if (r.command === 'open') {
        openModel(r);
        reply({ok:true, state:state(), selection:selection()});
        return;
      }
      if (!active || r.documentId !== active.documentId || r.version !== active.version) throw Error('stale-document-version');
      if (r.command === 'local-view') {
        if (Object.keys(r).sort().join(',') !== 'action,channel,command,documentId,operationId,version') throw Error('invalid-request');
        reply({ok:true, view:localView(r.action)});
      } else if (r.command === 'edit') {
        if (active.sequence !== active.acknowledged) throw Error('unsaved-changes');
        if (model.isReadonly() !== true) throw Error('unsupported-command');
        // Internal AI mutation authority; the native view stays read-only.
        active.editing = true;
        reply({ok:true, state:state(), selection:selection()});
      } else if (r.command === 'replace') {
        const handle = targetFor(r);
        if (!['text','cell','shape'].includes(handle.kind) || typeof r.text !== 'string' || r.text.length > 4096) throw Error('unsupported-selection');
        if (r.valueType !== 'text') throw Error('invalid-control-value');
        if (handle.kind === 'cell') {
          // The engine, not a renderer-supplied type, owns the current cell type.
          // Numeric/formula changes require verified typed operations; never coerce them.
          const nativeType = handle.target.getType();
          const type = typeof nativeType === 'number' ? nativeType : nativeType.value;
          if (![0,2].includes(type) || handle.target.getIsMerged()) throw Error('unsupported-selection');
        }
        if (handle.target.getString() === r.text) throw Error('invalid-control-value');
        if (handle.kind === 'text') requirePlainTextRange(handle.target);
        mutate(() => handle.target.setString(r.text));
        reply({ok:true, state:state(), selection:selection()});
      } else if (r.command === 'replacePresentation') {
        replacePresentation(r);reply({ok:true,state:state(),selection:selection()});
      } else if (r.command === 'replaceCells') {
        replaceWorkbook(r);reply({ok:true,state:state(),selection:selection()});
      } else if (r.command === 'export') {
        requireEditing();
        if (active.pendingExport) throw Error('export-awaiting-ack');
        const sequence = active.sequence;
        // Native UI is readonly and worker commands are serialized. Synchronous
        // filter/layout notifications during export are not another AI edit.
        suppressChanges = true;
        try { model.storeToURL('file:///tmp/analytix-office/output.' + active.kind, [prop('FilterName', filters[active.kind]), prop('Overwrite', true)]); }
        finally { suppressChanges = false; }
        active.pendingExport = {operationId:r.operationId, sequence};
        reply({ok:true, state:state(), exportedSequence:sequence});
      } else if (r.command === 'ack') {
        requireEditing();
        const pending = active.pendingExport;
        if (!pending || pending.operationId !== r.exportOperationId || pending.sequence !== r.exportedSequence || !['committed','conflict','failed'].includes(r.status)) throw Error('ack-mismatch');
        if (r.status === 'committed') {
          if (typeof r.persistedVersion !== 'string' || !/^[a-f0-9]{64}$/.test(r.persistedVersion)) throw Error('ack-mismatch');
          active.version = r.persistedVersion; active.acknowledged = pending.sequence; handles.clear();
          if (active.sequence === active.acknowledged) { suppressChanges = true; try { model.setModified(false); } finally { suppressChanges = false; } }
        }
        delete active.pendingExport;
        reply({ok:true, state:state(), selection:selection()});
      } else if (r.command === 'captureSelection') {
        reply({ok:true, state:state(), selection:selection()});
      } else if (r.command === 'close') {
        if (!Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0 || typeof r.discard !== 'boolean') throw Error('invalid-request');
        // The condition and close run in the same worker turn; no await permits intervening input.
        if (!r.discard && (active.sequence !== r.expectedChangeSequence || active.sequence !== active.acknowledged)) throw Error('unsaved-changes');
        close(); reply({ok:true});
      } else throw Error('unsupported-command');
    } catch (error) {
      const known = ['typed-mutation-failed','stale-selection','view-unavailable','unsupported-local-control','invalid-control-value','unsupported-selection','single-cell-required','control-limit','invalid-request','unsaved-changes','native-controls-hide-failed','document-already-open','open-failed','stale-document-version','export-awaiting-ack','ack-mismatch','command-unavailable','unsupported-command'];
      const code = known.includes(error.message) ? error.message : 'engine-operation-failed';
      reply({ok:false, error:code});
    }
  };
  emit({type:'worker-ready'});
});
