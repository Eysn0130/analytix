import { WorkbenchModeCard } from "../../../components/workbench-ui";
import type { CleaningStepViewModel } from "../model/view-model";

interface CleaningStepInsightGridProps {
  metaLabel: string;
  selectedStep: number | null;
  steps: CleaningStepViewModel[];
  onOpenStepDetail: (step: number) => void;
}

export function CleaningStepInsightGrid({
  metaLabel,
  selectedStep,
  steps,
  onOpenStepDetail
}: CleaningStepInsightGridProps): JSX.Element {
  return (
    <section className="cleaning-steps">
      <div className="cleaning-steps__head">
        <div>
          <div className="cleaning-section__eyebrow">Step Insight</div>
          <h3>步骤洞察</h3>
        </div>
        <div className="cleaning-steps__meta">{metaLabel}</div>
      </div>

      <div className="cleaning-step-grid">
        {steps.map((step) => (
          <WorkbenchModeCard
            key={step.step}
            className={`cleaning-step-insight ${selectedStep === step.step ? "is-selected" : ""}`}
            active={step.hit}
            type="button"
            onClick={() => onOpenStepDetail(step.step)}
          >
            <div className="cleaning-step-insight__line" />
            <div className="cleaning-step-insight__head">
              <span className="cleaning-step-insight__index">{step.indexLabel}</span>
              <span className={`cleaning-kind-chip ${step.kindClassName}`}>{step.kind}</span>
            </div>
            <div className="cleaning-step-insight__title">{step.title}</div>
            <div className="cleaning-step-insight__desc">{step.description}</div>
            <div className="cleaning-step-insight__foot">
              <div>
                <strong>{step.affectedRowsLabel}</strong>
                <span>受影响记录</span>
              </div>
              <span className={`cleaning-step-insight__state ${step.stateClassName}`.trim()}>
                {step.stateLabel}
              </span>
            </div>
          </WorkbenchModeCard>
        ))}
      </div>
    </section>
  );
}
