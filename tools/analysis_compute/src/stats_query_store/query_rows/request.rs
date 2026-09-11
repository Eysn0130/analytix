use super::super::args::QueryStatsRowsArgs;

pub(super) struct QueryStatsRowsRequest {
    pub(super) case_id: String,
    pub(super) selected: Vec<String>,
    pub(super) mode: String,
    pub(super) group_key: &'static str,
    pub(super) date_start: String,
    pub(super) date_end: String,
    pub(super) search_text: String,
    pub(super) row_sort_col: String,
    row_sort_dir: &'static str,
    pub(super) row_offset: i64,
    pub(super) row_limit: i64,
    pub(super) row_format: String,
    pub(super) fields: Vec<String>,
}

impl QueryStatsRowsRequest {
    pub(super) fn from_args(args: &QueryStatsRowsArgs) -> Self {
        let mode = args.mode.trim().to_string();
        let group_key = if mode == "inAccount" || mode == "outAccount" {
            "cp_key"
        } else {
            "cp_name"
        };
        let row_sort_dir = if args.row_sort_dir.trim().eq_ignore_ascii_case("asc") {
            "asc"
        } else {
            "desc"
        };
        Self {
            case_id: args.case_id.trim().to_string(),
            selected: args
                .selected_keys
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>(),
            mode,
            group_key,
            date_start: args.date_start.trim().to_string(),
            date_end: args.date_end.trim().to_string(),
            search_text: args.search_text.trim().to_lowercase(),
            row_sort_col: args.row_sort_col.trim().to_string(),
            row_sort_dir,
            row_offset: args.row_offset.max(0),
            row_limit: args.row_limit.max(0),
            row_format: args.row_format.trim().to_string(),
            fields: args
                .fields
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>(),
        }
    }

    pub(super) fn uses_pagination(&self) -> bool {
        self.row_limit > 0 || self.row_offset > 0
    }

    pub(super) fn limit_sql(&self) -> i64 {
        if self.row_limit > 0 {
            self.row_limit
        } else {
            i64::MAX
        }
    }

    pub(super) fn sort_order_sql(&self) -> &'static str {
        if self.row_sort_dir == "asc" {
            "ASC"
        } else {
            "DESC"
        }
    }

    pub(super) fn default_sort_expr(&self) -> &'static str {
        if self.mode.starts_with("out") {
            "(out_amount - in_amount)"
        } else {
            "(in_amount - out_amount)"
        }
    }
}
