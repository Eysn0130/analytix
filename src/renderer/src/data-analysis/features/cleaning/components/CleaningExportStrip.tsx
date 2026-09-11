import { WorkbenchToolbarButton } from "../../../components/workbench-ui";
import { getStatusLabel, getJobTone } from "../model/status";
import type { ExportState } from "../model/types";
import { shouldShowCleaningExportStrip } from "../model/view-model";

interface CleaningExportStripProps {
  exportState: ExportState;
  onCancelExport: () => void;
  onOpenExportOutput: () => void;
}

export function CleaningExportStrip({
  exportState,
  onCancelExport,
  onOpenExportOutput
}: CleaningExportStripProps): JSX.Element | null {
  if (!shouldShowCleaningExportStrip(exportState)) {
    return null;
  }

  return (
    <section className="cleaning-export-strip">
      <div className="cleaning-export-strip__copy">
        <span className={`cleaning-status-badge tone-${getJobTone(exportState.status === "done" ? "succeeded" : exportState.status)}`}>
          {getStatusLabel(exportState.status === "done" ? "succeeded" : exportState.status)}
        </span>
        <strong>{exportState.message}</strong>
        {exportState.outputPath ? <span className="cleaning-code-line">{exportState.outputPath}</span> : null}
      </div>
      <div className="cleaning-export-strip__actions">
        {exportState.status === "running" ? (
          <WorkbenchToolbarButton className="cleaning-export-strip__button" tone="ghost" type="button" onClick={onCancelExport}>
            取消导出
          </WorkbenchToolbarButton>
        ) : null}
        {exportState.outputPath ? (
          <WorkbenchToolbarButton className="cleaning-export-strip__button" tone="ghost" type="button" onClick={onOpenExportOutput}>
            打开目录
          </WorkbenchToolbarButton>
        ) : null}
      </div>
    </section>
  );
}
