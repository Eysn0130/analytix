import type {
  TxnDetailDialogColumn,
} from "../../../components/txn-detail/TxnDetailDialog";
import type { TxnDetailDialogColumnSpec } from "../../../components/txn-detail/txn-detail-column-model";
import type { CleaningStepDetailDTO } from "../../../services/cleaning/api";

export interface CleaningStepDetailDialogViewModel {
  ariaLabel: string;
  emptyText: string;
  footerLabel: string;
  meta: string;
  open: boolean;
  title: string;
}

export function buildCleaningStepDetailDialogViewModel({
  detailData,
  detailError,
  footerLabel,
  step
}: {
  detailData: CleaningStepDetailDTO | null;
  detailError: string;
  footerLabel: string;
  step: number | null;
}): CleaningStepDetailDialogViewModel {
  const title = detailData?.title || (step ? `STEP ${step}` : "步骤详情");
  const normalizedError = detailError.trim();
  return {
    ariaLabel: title,
    emptyText: normalizedError || "原始明细需同案证据回执和受控披露授权",
    footerLabel: footerLabel.trim(),
    meta: detailData?.description || "普通界面仅显示能力边界，不发布案件明细。",
    open: step !== null,
    title
  };
}

export function buildCleaningStepDetailColumns(
  _detailData: CleaningStepDetailDTO | null
): TxnDetailDialogColumn[] {
  return [];
}

export function buildCleaningStepDetailColumnSpecs(
  _detailData: CleaningStepDetailDTO | null
): TxnDetailDialogColumnSpec[] {
  return [];
}
