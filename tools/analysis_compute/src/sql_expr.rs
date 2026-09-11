use crate::{NULL_DOUBLE, NULL_TEXT, NULL_TIMESTAMP};

pub(crate) fn has_col(cols: &[String], col: &str) -> bool {
    cols.iter().any(|item| item == col)
}

pub(crate) fn trim_expr(expr: &str) -> String {
    format!("NULLIF(TRIM(CAST({expr} AS VARCHAR)), '')")
}

fn clean_text_expr(expr: &str) -> String {
    let cleaned = trim_expr(expr);
    format!("NULLIF(NULLIF(NULLIF({cleaned}, '-'), '—'), '－')")
}

fn trim_keep_empty_expr(expr: &str) -> String {
    format!("CASE WHEN {expr} IS NULL THEN NULL ELSE TRIM(CAST({expr} AS VARCHAR)) END")
}

pub(crate) fn text_col_expr(
    prefix: &str,
    cols: &[String],
    col: &str,
    clean_invalid: bool,
) -> Option<String> {
    if !has_col(cols, col) {
        return None;
    }
    if clean_invalid {
        return Some(clean_text_expr(&format!("{prefix}.{col}")));
    }
    Some(trim_expr(&format!("{prefix}.{col}")))
}

pub(crate) fn first_text_expr(
    prefix: &str,
    cols: &[String],
    candidates: &[&str],
) -> Option<String> {
    let exprs = candidates
        .iter()
        .map(|col| text_col_expr(prefix, cols, col, false))
        .collect::<Vec<_>>();
    if exprs.iter().all(Option::is_none) {
        return None;
    }
    Some(coalesce_expr(&exprs, NULL_TEXT))
}

pub(crate) fn text_col_expr_minlen(
    prefix: &str,
    cols: &[String],
    col: &str,
    min_len: i64,
) -> Option<String> {
    let expr = text_col_expr(prefix, cols, col, true)?;
    Some(format!(
        "CASE WHEN {expr} IS NULL THEN NULL WHEN LENGTH({expr}) < {min_len} THEN NULL ELSE {expr} END"
    ))
}

pub(crate) fn num_col_expr(prefix: &str, cols: &[String], col: &str) -> Option<String> {
    if !has_col(cols, col) {
        return None;
    }
    if col.ends_with("_val") {
        return Some(format!("{prefix}.{col}"));
    }
    Some(format!(
        "TRY_CAST({} AS DOUBLE)",
        trim_expr(&format!("{prefix}.{col}"))
    ))
}

pub(crate) fn coalesce_expr(exprs: &[Option<String>], default: &str) -> String {
    let parts = exprs
        .iter()
        .filter_map(|item| item.clone())
        .collect::<Vec<_>>();
    match parts.len() {
        0 => default.to_string(),
        1 => parts[0].clone(),
        _ => format!("COALESCE({})", parts.join(",")),
    }
}

pub(crate) fn account_key_expr(prefix: &str, cols: &[String], candidates: &[&str]) -> String {
    coalesce_expr(
        &candidates
            .iter()
            .map(|col| text_col_expr(prefix, cols, col, true))
            .collect::<Vec<_>>(),
        NULL_TEXT,
    )
}

pub(crate) fn counterparty_key_expr(prefix: &str, cols: &[String]) -> String {
    coalesce_expr(
        &["counterparty_acct_norm", "counterparty_acct"]
            .iter()
            .map(|col| text_col_expr(prefix, cols, col, false))
            .collect::<Vec<_>>(),
        NULL_TEXT,
    )
}

pub(crate) fn counterparty_raw_expr(prefix: &str, cols: &[String]) -> String {
    coalesce_expr(
        &["counterparty_acct", "counterparty_acct_norm"]
            .iter()
            .filter(|col| has_col(cols, col))
            .map(|col| Some(trim_keep_empty_expr(&format!("{prefix}.{col}"))))
            .collect::<Vec<_>>(),
        NULL_TEXT,
    )
}

pub(crate) fn dc_value_expr(prefix: &str, cols: &[String]) -> String {
    coalesce_expr(
        &["dc_final", "clean_dc_flag", "dc_flag"]
            .iter()
            .map(|col| text_col_expr(prefix, cols, col, false))
            .collect::<Vec<_>>(),
        NULL_TEXT,
    )
}

pub(crate) fn amount_value_expr(prefix: &str, cols: &[String]) -> String {
    finite_num_value_expr(prefix, cols, &["clean_amount", "amount_val", "amount"])
}

pub(crate) fn finite_num_value_expr(prefix: &str, cols: &[String], candidates: &[&str]) -> String {
    let branches = candidates
        .iter()
        .filter_map(|column| {
            let candidate = num_col_expr(prefix, cols, column)?;
            let present = if column.ends_with("_val") {
                format!("{prefix}.{column} IS NOT NULL")
            } else {
                format!("{} IS NOT NULL", trim_expr(&format!("{prefix}.{column}")))
            };
            Some(format!(
                "WHEN {present} THEN CASE WHEN ({candidate}) IS NOT NULL AND isfinite({candidate}) \
                 THEN {candidate} ELSE NULL END"
            ))
        })
        .collect::<Vec<_>>();
    if branches.is_empty() {
        return NULL_DOUBLE.to_string();
    }
    format!("CASE {} ELSE {NULL_DOUBLE} END", branches.join(" "))
}

pub(crate) fn clean_row_acceptance_expr(prefix: &str, cols: &[String]) -> Option<String> {
    let required = ["clean_invalid", "clean_failed", "clean_reversal"];
    if required.iter().any(|column| !has_col(cols, column)) {
        return None;
    }
    Some(
        required
            .iter()
            .map(|column| format!("{prefix}.{column}=0"))
            .collect::<Vec<_>>()
            .join(" AND "),
    )
}

pub(crate) fn txn_ts_expr(prefix: &str, cols: &[String]) -> String {
    let mut parts = Vec::new();
    if has_col(cols, "txn_ts") {
        parts.push(format!("{prefix}.txn_ts"));
    }
    if has_col(cols, "txn_time") {
        parts.push(ts_norm_expr(&format!("{prefix}.txn_time")));
    }
    match parts.len() {
        0 => NULL_TIMESTAMP.to_string(),
        1 => parts[0].clone(),
        _ => format!("COALESCE({})", parts.join(",")),
    }
}

fn ts_norm_expr(expr: &str) -> String {
    format!(
        "COALESCE(try_strptime({expr}, '%Y-%m-%d %H:%M:%S'),try_strptime({expr}, '%Y-%m-%d %H:%M'),try_strptime({expr}, '%Y-%m-%d'),try_strptime({expr}, '%Y%m%d%H%M%S'),try_strptime({expr}, '%Y%m%d'))"
    )
}

pub(crate) fn placeholder_kind_sql(expr: &str) -> String {
    let trimmed = format!("TRIM({expr})");
    let lowered = format!("LOWER({trimmed})");
    format!(
        "CASE WHEN {expr} IS NULL THEN 'db_null' \
         WHEN {trimmed} = '' THEN 'empty' \
         WHEN {trimmed} = (CHR(92) || 'N') THEN 'slash_n' \
         WHEN {trimmed} = '-' THEN 'dash' \
         WHEN {trimmed} = '—' THEN 'emdash' \
         WHEN {trimmed} = '－' THEN 'fw_dash' \
         WHEN {lowered} = 'null' THEN 'literal_null' \
         WHEN {lowered} = 'none' THEN 'literal_none' \
         WHEN {lowered} = 'nan' THEN 'literal_nan' \
         ELSE NULL END"
    )
}

pub(crate) fn placeholder_token_sql(kind_expr: &str) -> String {
    format!(
        "CASE WHEN {kind_expr} = 'db_null' THEN '__cp_placeholder__::db_null' \
         WHEN {kind_expr} = 'empty' THEN '__cp_placeholder__::empty' \
         WHEN {kind_expr} = 'slash_n' THEN '__cp_placeholder__::slash_n' \
         WHEN {kind_expr} = 'dash' THEN '__cp_placeholder__::dash' \
         WHEN {kind_expr} = 'emdash' THEN '__cp_placeholder__::emdash' \
         WHEN {kind_expr} = 'fw_dash' THEN '__cp_placeholder__::fw_dash' \
         WHEN {kind_expr} = 'literal_null' THEN '__cp_placeholder__::literal_null' \
         WHEN {kind_expr} = 'literal_none' THEN '__cp_placeholder__::literal_none' \
         WHEN {kind_expr} = 'literal_nan' THEN '__cp_placeholder__::literal_nan' \
         ELSE NULL END"
    )
}

pub(crate) fn placeholder_label_sql(kind_expr: &str) -> String {
    format!(
        "CASE WHEN {kind_expr} = 'db_null' THEN NULL \
         WHEN {kind_expr} = 'empty' THEN '空串' \
         WHEN {kind_expr} = 'slash_n' THEN (CHR(92) || 'N') \
         WHEN {kind_expr} = 'dash' THEN '-' \
         WHEN {kind_expr} = 'emdash' THEN '—' \
         WHEN {kind_expr} = 'fw_dash' THEN '－' \
         WHEN {kind_expr} = 'literal_null' THEN 'null' \
         WHEN {kind_expr} = 'literal_none' THEN 'none' \
         WHEN {kind_expr} = 'literal_nan' THEN 'nan' \
         ELSE NULL END"
    )
}

pub(crate) fn sql_literal(value: &str) -> String {
    format!("'{}'", value.replace('\'', "''"))
}
