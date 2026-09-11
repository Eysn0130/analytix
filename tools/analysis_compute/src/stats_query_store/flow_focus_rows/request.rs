use anyhow::{bail, Result};

use super::super::args::QueryFlowFocusRowsArgs;
use super::super::values::dedupe_placeholder_kinds;

pub(super) struct FlowFocusRowsRequest {
    pub(super) seeds: Vec<String>,
    pub(super) focus_ids: Vec<String>,
    pub(super) placeholder_kinds: Vec<String>,
    pub(super) include_missing_counterparty: bool,
    pub(super) direction: String,
    pub(super) date_start: String,
    pub(super) date_end_excl: String,
}

impl FlowFocusRowsRequest {
    pub(super) fn from_args(args: &QueryFlowFocusRowsArgs) -> Result<Self> {
        if !args.min_amount.is_finite() {
            bail!("--min-amount must be finite");
        }
        if args.min_amount > 0.0 {
            bail!("query-flow-focus-rows does not support per-transaction min_amount filters");
        }
        Ok(Self {
            seeds: args
                .query_seed_ids
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>(),
            focus_ids: args
                .selected_focus_ids
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>(),
            placeholder_kinds: dedupe_placeholder_kinds(&args.selected_placeholder_kinds),
            include_missing_counterparty: args.include_missing_counterparty,
            direction: args.direction.trim().to_string(),
            date_start: args.date_start.trim().to_string(),
            date_end_excl: args.date_end_excl.trim().to_string(),
        })
    }

    pub(super) fn should_return_empty(&self) -> bool {
        self.seeds.is_empty() || (self.focus_ids.is_empty() && self.placeholder_kinds.is_empty())
    }
}
