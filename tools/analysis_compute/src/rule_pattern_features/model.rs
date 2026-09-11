#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum CashClass {
    Cash,
    NonCash,
    Unknown,
    Conflict,
}

impl CashClass {
    pub(crate) fn is_known(self) -> bool {
        matches!(self, Self::Cash | Self::NonCash)
    }
}

#[derive(Clone)]
pub(crate) struct RulePatternTxnRow {
    pub(crate) account_key: String,
    pub(crate) txn_id: String,
    pub(crate) txn_time: String,
    pub(crate) txn_epoch: i64,
    pub(crate) txn_hour: i64,
    pub(crate) amount: f64,
    pub(crate) direction: String,
    pub(crate) cash_class: CashClass,
}

pub(crate) struct RulePatternFeature {
    pub(crate) feature_code: &'static str,
    pub(crate) account_key: String,
    pub(crate) direction: String,
    pub(crate) txn_count: i64,
    pub(crate) total_amount: f64,
    pub(crate) txn_ids_json: String,
    pub(crate) amounts_json: String,
    pub(crate) detail_json: String,
    pub(crate) first_time: String,
    pub(crate) last_time: String,
    pub(crate) round_unit: Option<f64>,
    pub(crate) min_amount: f64,
    pub(crate) min_count: i64,
    pub(crate) min_total_amount: f64,
    pub(crate) start_hour: Option<i64>,
    pub(crate) end_hour: Option<i64>,
}
