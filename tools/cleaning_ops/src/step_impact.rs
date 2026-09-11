#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CleaningStepSummaryImpact {
    pub step: i64,
    pub affected_rows: i64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CleaningStepImpact {
    pub txn_step_affected: i64,
    pub account_step_affected: i64,
    pub step_hits: i64,
}

pub fn project_cleaning_step_impact(steps: &[CleaningStepSummaryImpact]) -> CleaningStepImpact {
    let mut impact = CleaningStepImpact {
        txn_step_affected: 0,
        account_step_affected: 0,
        step_hits: 0,
    };

    for step in steps {
        if step.step <= 7 {
            impact.txn_step_affected += step.affected_rows;
        }
        if step.step >= 8 {
            impact.account_step_affected += step.affected_rows;
        }
        if step.affected_rows > 0 {
            impact.step_hits += 1;
        }
    }

    impact
}
