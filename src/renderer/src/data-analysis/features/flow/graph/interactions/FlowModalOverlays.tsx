import {
  Fragment,
  KeyboardEvent,
  useEffect,
  useMemo,
  useRef
} from "react";
import { createPortal } from "react-dom";
import { TxnDetailDialog, type TxnDetailDialogColumn, type TxnDetailDialogRow } from "../../../../components/txn-detail/TxnDetailDialog";
import { buildTxnDetailDialogRows } from "../../../../components/txn-detail/txn-detail-dialog-row-model";
import { useTxnDetailViewport } from "../../../../components/txn-detail/useTxnDetailViewport";
import "../../../../components/txn-detail/txn-detail-dialog.css";
import "../../../../components/workbench-ui/workbench-ui-kit.css";
import type { FlowShellBridge, FlowShellMounts, FlowShellState } from "../runtime/flow-runtime";
import { EdgeTxnReadonlyPanel, stopEventPropagation } from "./FlowOverlayPrimitives";

interface FlowModalOverlaysProps {
  bridge: FlowShellBridge;
  mounts: FlowShellMounts;
  state: FlowShellState;
}

const TXN_DETAIL_ROW_HEIGHT = 32;
const TXN_DETAIL_HEADER_HEIGHT = 32;
const TXN_DETAIL_OVERSCAN_ROWS = 6;

function DetailDrawerPortal({ bridge, mounts, state }: FlowModalOverlaysProps): JSX.Element | null {
  const drawer = state.overlay.drawer;
  if (!mounts.overlay || !drawer.open) {
    return null;
  }

  return createPortal(
    <div className="drawerWrap active analytix-react-detail-drawer">
      <aside
        className="drawer open"
        aria-hidden="false"
        onMouseDown={stopEventPropagation}
        onClick={stopEventPropagation}
        onWheel={stopEventPropagation}
      >
        <div className="dHead">
          <div className="dTitle">{drawer.title || "详情"}</div>
          <button className="dClose" type="button" onClick={() => bridge.commands.overlay.closeDrawer()}>
            关闭
          </button>
        </div>
        <div className="dBody">
          <div className="kv">
            {drawer.rows.map((row, index) => (
              <Fragment key={`${row.label}-${index}`}>
                <div className="k">{row.label}</div>
                <div
                  className="v"
                  style={{
                    ...(row.mono ? { fontFamily: "var(--mono)" } : {}),
                    ...(row.tone === "in" ? { color: "#15803d" } : {}),
                    ...(row.tone === "out" ? { color: "#dc2626" } : {})
                  }}
                >
                  {row.value}
                </div>
              </Fragment>
            ))}
          </div>
          {drawer.actions.length ? (
            <div style={{ marginTop: "12px", display: "flex", gap: "10px" }}>
              {drawer.actions.map((action) => (
                <button
                  key={action.id}
                  className={action.variant === "ghost" ? "btn ghost" : "btn"}
                  type="button"
                  onClick={() => bridge.commands.overlay.runDrawerAction(action.id)}
                >
                  {action.label}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </aside>
    </div>,
    mounts.overlay
  );
}

function EdgeLabelModalPortal({ bridge, mounts, state }: FlowModalOverlaysProps): JSX.Element | null {
  const edgeLabel = state.overlay.edgeLabel;
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!edgeLabel.open || !edgeLabel.editable) {
      return;
    }
    window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 0);
  }, [edgeLabel.open, edgeLabel.editable]);

  if (!mounts.overlay || !edgeLabel.open) {
    return null;
  }

  const readonlyOpen = !edgeLabel.editable || edgeLabel.readonly.open;

  return createPortal(
    <div
      className="modalMask show analytix-react-overlay-panel analytix-react-edge-label-modal"
      aria-hidden="false"
      onClick={() => bridge.commands.overlay.closeEdgeLabel()}
    >
      <div
        className={`modalCard${readonlyOpen ? " edgeTxnMode" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="edgeLabelTitle"
        onMouseDown={stopEventPropagation}
        onClick={stopEventPropagation}
        onWheel={stopEventPropagation}
      >
        <div className="modalTitle" id="edgeLabelTitle">
          {edgeLabel.title || "编辑连线文本"}
        </div>
        <div className="modalBody">
          {edgeLabel.editable ? (
            <input
              ref={inputRef}
              className="input"
              value={edgeLabel.value}
              placeholder={edgeLabel.placeholder || "输入文本内容…"}
              autoComplete="off"
              onChange={(event) => bridge.commands.overlay.setEdgeLabelValue(event.currentTarget.value)}
              onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  bridge.commands.overlay.saveEdgeLabel();
                  return;
                }
                if (event.key === "Escape") {
                  event.preventDefault();
                  bridge.commands.overlay.closeEdgeLabel();
                }
              }}
            />
          ) : null}
          {edgeLabel.editable && edgeLabel.hint ? <div className="modalHint">{edgeLabel.hint}</div> : null}
          {readonlyOpen ? <EdgeTxnReadonlyPanel data={edgeLabel.readonly} onOpenDetail={null} /> : null}
        </div>
        <div className="modalActions">
          <button className="btn ghost" type="button" onClick={() => bridge.commands.overlay.closeEdgeLabel()}>
            {edgeLabel.cancelLabel || (edgeLabel.editable ? "取消" : "关闭")}
          </button>
          {edgeLabel.editable ? (
            <button className="btn primary" type="button" onClick={() => bridge.commands.overlay.saveEdgeLabel()}>
              {edgeLabel.confirmLabel || "确定"}
            </button>
          ) : null}
        </div>
      </div>
    </div>,
    mounts.overlay
  );
}

function TxnModalPortal({ bridge, mounts, state }: FlowModalOverlaysProps): JSX.Element | null {
  const txnModal = state.overlay.txnModal;
  const txnViewport = useTxnDetailViewport({
    open: txnModal.open,
    rowCount: txnModal.rows.length,
    rowHeight: TXN_DETAIL_ROW_HEIGHT,
    headerHeight: TXN_DETAIL_HEADER_HEIGHT,
    overscanRows: TXN_DETAIL_OVERSCAN_ROWS,
    initialViewportRows: 20,
    resetKey: txnModal.requestKey
  });

  const columns: TxnDetailDialogColumn[] = useMemo(
    () =>
      txnModal.columns.map((column) => ({
        key: column.key,
        title: column.title,
        width: column.width,
        sortable: column.sortable,
        sorted: column.sorted,
        sortDir: (column.sortDir === "desc" ? "desc" : "asc") as TxnDetailDialogColumn["sortDir"],
        resizable: true
      })),
    [txnModal.columns]
  );
  const windowStart = txnViewport.windowStart;
  const windowEnd = txnViewport.windowEnd;
  const rows: TxnDetailDialogRow[] = useMemo(
    () => buildTxnDetailDialogRows({ rows: txnModal.rows, columns: txnModal.columns, windowStart, windowEnd }),
    [txnModal.columns, txnModal.rows, windowEnd, windowStart]
  );
  const footerContent = txnModal.hint ? (
    <div className="txnModalFooterSummary">
      <span className="txnModalFooterSummaryLabel">明细状态</span>
      <span>{txnModal.hint}</span>
    </div>
  ) : null;

  if (!mounts.overlay || !txnModal.open) {
    return null;
  }

  return createPortal(
    <TxnDetailDialog
      open
      ariaLabel={txnModal.title || "交易明细"}
      title={txnModal.title || "交易明细"}
      columns={columns}
      rows={rows}
      loading={txnModal.loading}
      emptyText={txnModal.emptyText}
      onClose={() => bridge.commands.overlay.closeTxnModal()}
      onToggleSort={(columnKey) => bridge.commands.overlay.toggleTxnSort(columnKey)}
      onResizeColumn={(columnKey, width) => bridge.commands.overlay.setTxnColumnWidth(columnKey, width)}
      tableWrapRef={txnViewport.tableWrapRef}
      onTableScroll={txnViewport.onTableScroll}
      onTableWheel={(event) => stopEventPropagation(event)}
      wrapClassName="txnModalWrap analytix-react-txn-modal txnModalThemeStats"
      topSpacerHeight={txnViewport.topSpacerHeight}
      bottomSpacerHeight={txnViewport.bottomSpacerHeight}
      footerContent={footerContent}
    />,
    mounts.overlay
  );
}

export function FlowModalOverlays(props: FlowModalOverlaysProps): JSX.Element {
  return (
    <>
      <DetailDrawerPortal {...props} />
      <EdgeLabelModalPortal {...props} />
      <TxnModalPortal {...props} />
    </>
  );
}
