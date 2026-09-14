/* global Module */
// First-party adapter using zetajs 1.2.0 public UNO APIs; see README provenance.
'use strict';
Module.zetajs.then(zeta => {
  const css = zeta.uno.com.sun.star;
  const context = zeta.getUnoComponentContext();
  const desktop = css.frame.Desktop.create(context);
  const filters = {docx:'Office Open XML Text', xlsx:'Calc MS Excel 2007 XML', pptx:'Impress MS PowerPoint 2007 XML'};
  const prop = (Name, Value) => new css.beans.PropertyValue({Name, Value});
  const emit = data => zeta.mainPort.postMessage(data);
  let channel, model, controller, active, modifyListener, selectionListener;
  const state = () => ({documentId:active.documentId, version:active.version, kind:active.kind,
    changeSequence:active.sequence, acknowledgedSequence:active.acknowledged, dirty:active.sequence !== active.acknowledged});
  const event = (type, payload) => emit({type, channel, documentId:active.documentId, version:active.version,
    operationId:active.openOperationId, ...payload});
  const boundedText = value => String(value ?? '').slice(0, 4096);
  function selection() {
    const base = {documentId:active.documentId, version:active.version, changeSequence:active.sequence};
    try {
      const selected = controller.getSelection();
      if (active.kind === 'docx') {
        const text = selected.getCount ? Array.from({length:Math.min(selected.getCount(), 16)}, (_, i) => selected.getByIndex(i).getString()).join('\n') : selected.getString();
        return {...base, kind:'text', scope:'current-view-text-only; no verified structural offset or durable anchor', text:boundedText(text)};
      }
      if (active.kind === 'xlsx') {
        let addresses;
        if (selected.getRangeAddresses) addresses = selected.getRangeAddresses();
        else if (selected.getRangeAddress) addresses = [selected.getRangeAddress()];
        else { const a = selected.getCellAddress(); addresses = [{Sheet:a.Sheet, StartColumn:a.Column, EndColumn:a.Column, StartRow:a.Row, EndRow:a.Row}]; }
        const ranges = addresses.slice(0, 64).map(a => ({sheet:a.Sheet, sheetName:model.getSheets().getByIndex(a.Sheet).getName(), startColumn:a.StartColumn, startRow:a.StartRow, endColumn:a.EndColumn, endRow:a.EndRow}));
        let text = ''; try { text = boundedText(selected.getString()); } catch { /* This selection type does not expose the optional text/name interface. */ }
        return {...base, kind:'cells', scope:'sheet-range-address-at-version-and-change-sequence', ranges, text};
      }
      const page = controller.getCurrentPage(), pages = model.getDrawPages();
      let pageIndex = -1;
      for (let i = 0; i < Math.min(pages.getCount(), 512); ++i) if (zeta.sameUnoObject(pages.getByIndex(i), page)) { pageIndex = i; break; }
      const shapes = [];
      const count = selected.getCount ? Math.min(selected.getCount(), 64) : 0;
      for (let i = 0; i < count; ++i) {
        const shape = selected.getByIndex(i); let shapeIndex = -1, text = '', name = '';
        for (let j = 0; j < Math.min(page.getCount(), 2048); ++j) if (zeta.sameUnoObject(page.getByIndex(j), shape)) { shapeIndex = j; break; }
        try { text = boundedText(shape.getString()); } catch { /* This selection type does not expose the optional text/name interface. */ }
        try { name = boundedText(shape.getName()); } catch { /* This selection type does not expose the optional text/name interface. */ }
        if (pageIndex >= 0 && shapeIndex >= 0) shapes.push({pageIndex, shapeIndex, name, type:shape.getShapeType(), text});
      }
      return {...base, kind:'shapes', scope:'page-and-shape-index-at-version-and-change-sequence', shapes};
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
  }
  zeta.mainPort.onmessage = ({data:r}) => {
    if (!r || typeof r !== 'object') return;
    if (r.command === 'bind' && !channel && typeof r.channel === 'string') { channel = r.channel; emit({type:'ready', channel}); return; }
    if (r.channel !== channel || !channel) return;
    const reply = payload => emit({type:'result', command:r.command, channel, operationId:r.operationId, documentId:r.documentId, version:r.version, ...payload});
    try {
      if (r.command === 'open') {
        if (active || !filters[r.kind]) throw Error('document-already-open');
        // Configuration lives in this isolated browser engine's virtual FS only.
        try { configurePreviewStage(); } catch { /* Optional stage styling must not block read-only preview. */ }
        model = desktop.loadComponentFromURL('file:///tmp/analytix-office/input.' + r.kind, '_default', 0,
          [prop('MacroExecutionMode', 4), prop('UpdateDocMode', 0), prop('ReadOnly', true), prop('LockEditDoc', true), prop('LockSave', true), prop('LockExport', true), prop('PickListEntry', false)]);
        if (!model) throw Error('open-failed');
        if (!model.isReadonly()) { close(); throw Error('open-failed'); }
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
        active = {documentId:r.documentId, version:r.version, kind:r.kind, openOperationId:r.operationId, sequence:0, acknowledged:0, viewFit:r.kind !== 'xlsx'};
        modifyListener = zeta.unoObject([css.util.XModifyListener], {disposing() {}, modified() {
          if (!active) return;
          // Writer view/layout notifications also use XModifyListener. Only a
          // confirmed read-only, unmodified model is a non-edit notification.
          // Failed reads and every other state still reach Main's fail-closed gate.
          try { if (model.isReadonly() === true && model.isModified() === false) return; }
          catch { /* Unknown native state must not be classified as an unmodified view. */ }
          active.sequence++;
          event('changed', {state:state()});
        }});
        selectionListener = zeta.unoObject([css.view.XSelectionChangeListener], {disposing() {}, selectionChanged() {
          if (active) event('selection', {selection:selection()});
        }});
        model.addModifyListener(modifyListener);
        controller.addSelectionChangeListener(selectionListener);
        reply({ok:true, state:state(), selection:selection()});
        return;
      }
      if (!active || r.documentId !== active.documentId || r.version !== active.version) throw Error('stale-document-version');
      if (r.command === 'local-view') {
        if (Object.keys(r).sort().join(',') !== 'action,channel,command,documentId,operationId,version') throw Error('invalid-request');
        reply({ok:true, view:localView(r.action)});
      } else if (r.command === 'captureSelection') {
        reply({ok:true, state:state(), selection:selection()});
      } else if (r.command === 'close') {
        if (!Number.isSafeInteger(r.expectedChangeSequence) || r.expectedChangeSequence < 0 || typeof r.discard !== 'boolean') throw Error('invalid-request');
        // The condition and close run in the same worker turn; no await permits intervening input.
        if (!r.discard && (active.sequence !== r.expectedChangeSequence || active.sequence !== active.acknowledged)) throw Error('unsaved-changes');
        close(); reply({ok:true});
      } else throw Error('unsupported-command');
    } catch (error) {
      const known = ['view-unavailable','unsupported-local-control','invalid-control-value','unsupported-selection','single-cell-required','control-limit','invalid-request','unsaved-changes','native-controls-hide-failed','document-already-open','open-failed','stale-document-version','export-awaiting-ack','ack-mismatch','command-unavailable','unsupported-command'];
      const code = known.includes(error.message) ? error.message : 'engine-operation-failed';
      reply({ok:false, error:code});
    }
  };
  emit({type:'worker-ready'});
});
