pub(in crate::stats_query_store) fn stats_rows_sort_expr(sort_col: &str) -> Option<&'static str> {
    match sort_col {
        "counterparty_account" => Some("counterparty_account"),
        "counterparty_name" => Some("counterparty_name"),
        "location" => Some("location"),
        "bank" => Some("bank"),
        "total_amount" => Some("(in_amount + out_amount)"),
        "total_count" => Some("(in_count + out_count)"),
        "net_in" => Some("(in_amount - out_amount)"),
        "net_out" => Some("(out_amount - in_amount)"),
        "in_amount" => Some("in_amount"),
        "in_count" => Some("in_count"),
        "out_amount" => Some("out_amount"),
        "out_count" => Some("out_count"),
        "first_time" => Some("first_time"),
        "last_time" => Some("last_time"),
        _ => None,
    }
}
