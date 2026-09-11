import { WorkbenchToolbarButton } from "../../../components/workbench-ui";
import type { CleaningHeroActionsViewModel } from "../model/hero-actions";
import type { CleaningBoardStatus } from "../model/types";

interface CleaningHeroProps {
  actions: CleaningHeroActionsViewModel;
  boardStatus: CleaningBoardStatus;
  boardStatusLabel: string;
  cleanedExportHint: string;
  cleanedExportReady: boolean;
  onExportCleaned: () => Promise<void>;
  onExportRaw: () => Promise<void>;
  onRunCleaning: (forceRebuild?: boolean) => Promise<void>;
}

export function CleaningHero({
  actions,
  boardStatus,
  boardStatusLabel,
  cleanedExportHint,
  cleanedExportReady,
  onExportCleaned,
  onExportRaw,
  onRunCleaning
}: CleaningHeroProps): JSX.Element {
  return (
    <header className="cleaning-hero">
      <div className="cleaning-hero__copy">
        <div className="cleaning-hero__title">
          <span className="cleaning-hero__icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" focusable="false">
              <rect x="4.25" y="5.25" width="15.5" height="13.5" rx="2.2" stroke="currentColor" strokeWidth="1.6" />
              <path d="M8 9.25H16" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
              <path d="M8 13H13" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </span>
          <div className="cleaning-hero__title-copy">
            <div className="cleaning-hero__title-main">
              <h1>数据清洗</h1>
              <span className={`cleaning-hero__status is-${boardStatus}`}>{boardStatusLabel}</span>
            </div>
            <p>DATA CLEANING WORKBENCH</p>
          </div>
        </div>
      </div>
      <div className="cleaning-hero__action-panel">
        <div className="cleaning-hero__actions" role="group" aria-label="清洗主操作">
          <WorkbenchToolbarButton
            className="cleaning-hero__action cleaning-hero__action--primary"
            tone="accent"
            type="button"
            loading={actions.start.loading}
            title={actions.start.title}
            disabled={actions.start.disabled}
            onClick={() => void onRunCleaning(false)}
          >
            开始清洗
          </WorkbenchToolbarButton>
          <WorkbenchToolbarButton
            className="cleaning-hero__action"
            type="button"
            loading={actions.reclean.loading}
            title={actions.reclean.title}
            disabled={actions.reclean.disabled}
            onClick={() => void onRunCleaning(true)}
          >
            重新清洗
          </WorkbenchToolbarButton>
          <WorkbenchToolbarButton
            className="cleaning-hero__action"
            tone="ghost"
            type="button"
            loading={actions.exportRaw.loading}
            disabled={actions.exportRaw.disabled}
            onClick={() => void onExportRaw()}
          >
            导出未清洗
          </WorkbenchToolbarButton>
          <span className="cleaning-hero__action-shell" title={actions.exportCleaned.title}>
            <WorkbenchToolbarButton
              className="cleaning-hero__action"
              tone="ghost"
              type="button"
              aria-describedby="cleaned-export-hint"
              loading={actions.exportCleaned.loading}
              disabled={actions.exportCleaned.disabled}
              onClick={() => void onExportCleaned()}
            >
              导出已清洗
            </WorkbenchToolbarButton>
          </span>
        </div>
        <p id="cleaned-export-hint" className={`cleaning-hero__action-hint${cleanedExportReady ? " is-ready" : ""}`}>
          {cleanedExportHint}
        </p>
      </div>
    </header>
  );
}
