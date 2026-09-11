import type { RefObject, UIEventHandler, WheelEventHandler } from "react";
import {
  TxnDetailDialog,
  type TxnDetailDialogColumn,
  type TxnDetailDialogRow
} from "../../../components/txn-detail/TxnDetailDialog";
import type { CleaningStepDetailDialogViewModel } from "../model/step-detail-render-model";

interface CleaningStepDetailDialogProps {
  columns: TxnDetailDialogColumn[];
  dialog: CleaningStepDetailDialogViewModel;
  loading: boolean;
  onClose: () => void;
  onTableScroll: UIEventHandler<HTMLDivElement>;
  onTableWheel: WheelEventHandler<HTMLDivElement>;
  rows: TxnDetailDialogRow[];
  tableWrapRef: RefObject<HTMLDivElement | null>;
  topSpacerHeight: number;
  bottomSpacerHeight: number;
}

export function CleaningStepDetailDialog({
  columns,
  dialog,
  loading,
  onClose,
  onTableScroll,
  onTableWheel,
  rows,
  tableWrapRef,
  topSpacerHeight,
  bottomSpacerHeight
}: CleaningStepDetailDialogProps): JSX.Element {
  return (
    <TxnDetailDialog
      open={dialog.open}
      ariaLabel={dialog.ariaLabel}
      title={dialog.title}
      meta={dialog.meta}
      columns={columns}
      rows={rows}
      loading={loading}
      emptyText={dialog.emptyText}
      onClose={onClose}
      tableWrapRef={tableWrapRef}
      onTableScroll={onTableScroll}
      onTableWheel={onTableWheel}
      wrapClassName="txnModalWrap txnModalThemeStats cleaning-step-txn-modal"
      topSpacerHeight={topSpacerHeight}
      bottomSpacerHeight={bottomSpacerHeight}
      footerContent={
        dialog.footerLabel ? (
          <div className="txnModalFooterSummary">
            <span>{dialog.footerLabel}</span>
          </div>
        ) : null
      }
    />
  );
}
