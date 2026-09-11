use super::super::values::{cursor_bool, cursor_f64, cursor_i64, cursor_string};
use super::request::QueryStatsTxnRowsRequest;

pub(super) fn push_cursor_filter(
    final_where: &mut Vec<String>,
    request: &QueryStatsTxnRowsRequest,
) {
    if request.limit <= 0 {
        return;
    }
    let Some(cursor) = request.cursor.as_ref() else {
        return;
    };
    let Some(last_id) = cursor_i64(cursor, "id") else {
        return;
    };

    let op = if request.dir_sql() == "ASC" { ">" } else { "<" };
    if request.sort_col == "amount" {
        if cursor_bool(cursor, "absNull") {
            final_where.push(format!(
                "((amount IS NULL OR NOT isfinite(amount)) AND id {op} {last_id})"
            ));
        } else if let Some(last_abs) = cursor_f64(cursor, "abs") {
            final_where.push(format!(
                "((amount IS NOT NULL AND isfinite(amount) AND \
                    (ABS(amount) {op} {last_abs} OR \
                     (ABS(amount) = {last_abs} AND id {op} {last_id}))) \
                  OR amount IS NULL OR NOT isfinite(amount))"
            ));
        }
        return;
    }

    let last_ts = cursor_string(cursor, "ts");
    let last_ts_null = cursor_bool(cursor, "tsNull") || last_ts.trim().is_empty();
    if last_ts_null {
        final_where.push(format!("(txn_ts IS NULL AND id {op} {last_id})"));
    } else {
        let ts_literal = crate::sql_literal(last_ts.trim());
        final_where.push(format!(
            "((txn_ts IS NOT NULL AND (txn_ts {op} CAST({ts_literal} AS TIMESTAMP) \
             OR (txn_ts = CAST({ts_literal} AS TIMESTAMP) AND id {op} {last_id}))) \
             OR txn_ts IS NULL)"
        ));
    }
}
