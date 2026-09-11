import { useEffect, useMemo, useState, type RefObject, type UIEventHandler, type WheelEventHandler } from "react";
import type { TxnDetailDialogColumn, TxnDetailDialogRow } from "../../../components/txn-detail/TxnDetailDialog";
import { useTxnDetailViewport } from "../../../components/txn-detail/useTxnDetailViewport";
import {
  type CleaningStepDetailDTO,
  getCleaningStepDetail
} from "../../../services/cleaning/api";
import {
  CLEANING_STEP_TXN_HEADER_HEIGHT,
  CLEANING_STEP_TXN_OVERSCAN_ROWS,
  CLEANING_STEP_TXN_ROW_HEIGHT
} from "../model/constants";
import { buildCleaningStepDetailColumns } from "../model/step-detail-render-model";

interface UseCleaningStepDetailOptions {
  activeCaseId: string;
}

interface CleaningStepDetailState {
  detailStep: number | null;
  detailData: CleaningStepDetailDTO | null;
  detailLoading: boolean;
  detailError: string;
  detailColumns: TxnDetailDialogColumn[];
  detailRows: TxnDetailDialogRow[];
  detailTopSpacerHeight: number;
  detailBottomSpacerHeight: number;
  detailTableWrapRef: RefObject<HTMLDivElement | null>;
  openStepDetail: (step: number) => void;
  closeStepDetail: () => void;
  onDetailTableScroll: UIEventHandler<HTMLDivElement>;
  onDetailTableWheel: WheelEventHandler<HTMLDivElement>;
}

export function useCleaningStepDetail({ activeCaseId }: UseCleaningStepDetailOptions): CleaningStepDetailState {
  const [detailStep, setDetailStep] = useState<number | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailData, setDetailData] = useState<CleaningStepDetailDTO | null>(null);

  useEffect(() => {
    if (!activeCaseId || detailStep === null) {
      setDetailData(null);
      setDetailError("");
      setDetailLoading(false);
      return;
    }

    const frozenCaseId = activeCaseId;
    const frozenStep = detailStep;
    let cancelled = false;
    setDetailLoading(true);
    setDetailError("");
    setDetailData(null);

    void getCleaningStepDetail({
      caseId: frozenCaseId,
      step: frozenStep,
      page: 1,
      pageSize: 1,
      offset: 0
    })
      .then((boundary) => {
        if (cancelled || boundary.case_id !== frozenCaseId || boundary.step !== frozenStep) {
          return;
        }
        setDetailData({
          ...boundary,
          fact_answer_allowed: false,
          raw_details_exposed: false,
          items: [],
          page: null
        });
      })
      .catch(() => {
        if (!cancelled) {
          setDetailError("清洗明细证据边界暂不可用");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [activeCaseId, detailStep]);

  const detailColumns = useMemo<TxnDetailDialogColumn[]>(
    () => buildCleaningStepDetailColumns(detailData),
    [detailData]
  );
  const detailRows = useMemo<TxnDetailDialogRow[]>(() => [], []);
  const detailViewport = useTxnDetailViewport({
    open: detailStep !== null,
    rowCount: 0,
    rowHeight: CLEANING_STEP_TXN_ROW_HEIGHT,
    headerHeight: CLEANING_STEP_TXN_HEADER_HEIGHT,
    overscanRows: CLEANING_STEP_TXN_OVERSCAN_ROWS,
    initialViewportRows: 12,
    resetKey: `${activeCaseId}:${String(detailStep ?? "")}`
  });

  return {
    detailStep,
    detailData,
    detailLoading,
    detailError,
    detailColumns,
    detailRows,
    detailTopSpacerHeight: detailViewport.topSpacerHeight,
    detailBottomSpacerHeight: detailViewport.bottomSpacerHeight,
    detailTableWrapRef: detailViewport.tableWrapRef,
    openStepDetail: setDetailStep,
    closeStepDetail: () => setDetailStep(null),
    onDetailTableScroll: detailViewport.onTableScroll,
    onDetailTableWheel: detailViewport.onTableWheel
  };
}
