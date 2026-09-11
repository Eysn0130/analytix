use anyhow::{bail, Result};
use duckdb::{
    core::{DataChunkHandle, Inserter, LogicalTypeHandle, LogicalTypeId},
    ffi::duckdb_string_t,
    types::DuckString,
    vscalar::{ScalarFunctionSignature, VScalar},
    vtab::arrow::WritableVector,
    Connection,
};
use std::borrow::Cow;
use std::time::Instant;

use crate::{elapsed_ms, open_txn_connection, scalar_i64, sql_literal, Args};

#[derive(Debug)]
pub(crate) struct AmountBalanceSummary {
    pub(crate) amount_updated: i64,
    pub(crate) balance_updated: i64,
    pub(crate) amount_failed: i64,
    pub(crate) balance_failed: i64,
    pub(crate) amount_fixed_total: i64,
    pub(crate) amount_failed_total: i64,
    pub(crate) balance_fixed_total: i64,
    pub(crate) balance_failed_total: i64,
    pub(crate) rows_affected: i64,
    pub(crate) timings: AmountBalanceTimings,
}

#[derive(Clone, Copy, Debug, Default)]
pub(crate) struct AmountBalanceTimings {
    pub(crate) desired_ms: i64,
    pub(crate) counts_ms: i64,
    pub(crate) update_ms: i64,
    pub(crate) totals_ms: i64,
}

pub(crate) fn run_amount_balance(args: &Args) -> Result<AmountBalanceSummary> {
    let conn = open_txn_connection(args)?;
    run_amount_balance_on_conn(&conn, args)
}

pub(crate) fn run_amount_balance_on_conn(
    conn: &Connection,
    args: &Args,
) -> Result<AmountBalanceSummary> {
    let mut timings = AmountBalanceTimings::default();
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    if scope_rows <= 0 {
        bail!("no transaction rows in cleaning scope");
    }

    register_number_norm_function(conn)?;

    let desired_started = Instant::now();
    conn.execute_batch("DROP TABLE IF EXISTS amount_balance_desired")?;
    let desired_sql = r#"
        CREATE TEMP TABLE amount_balance_desired AS
        WITH source_raw AS (
            SELECT
                id,
                orig_amount,
                orig_balance,
                clean_amount,
                clean_balance,
                clean_amt_fixed,
                clean_amt_failed,
                clean_bal_fixed,
                clean_bal_failed,
                COALESCE(TRIM(CAST(amount AS TEXT)), '') AS amt_old,
                COALESCE(TRIM(CAST(balance AS TEXT)), '') AS bal_old
            FROM fc_transaction_norm
            WHERE id IN (SELECT id FROM txn_scope)
        ),
        normalized AS (
            SELECT
                id,
                orig_amount,
                orig_balance,
                clean_amount,
                clean_balance,
                clean_amt_fixed,
                clean_amt_failed,
                clean_bal_fixed,
                clean_bal_failed,
                amt_old,
                bal_old,
                analytix_amount_number_norm(amt_old) AS amt_norm,
                analytix_amount_number_norm(bal_old) AS bal_norm
            FROM source_raw
        ),
        classified AS (
            SELECT
                *,
                amt_norm IS NOT NULL AS amt_valid,
                bal_norm IS NOT NULL AS bal_valid
            FROM normalized
        ),
        change_flags AS (
            SELECT
                *,
                amt_valid AND amt_old<>'' AND amt_norm<>amt_old AS amount_needs_fixed,
                NOT amt_valid AND amt_old<>'' AS amount_needs_failed,
                amt_valid AND COALESCE(clean_amt_failed, 0)=1 AS amount_clears_failed,
                bal_valid AND bal_old<>'' AND bal_norm<>bal_old AS balance_needs_fixed,
                NOT bal_valid AND bal_old<>'' AS balance_needs_failed,
                bal_valid AND COALESCE(clean_bal_failed, 0)=1 AS balance_clears_failed
            FROM classified
        ),
        joined AS (
            SELECT
                *,
                (
                    amount_needs_fixed
                    OR amount_needs_failed
                    OR amount_clears_failed
                    OR balance_needs_fixed
                    OR balance_needs_failed
                    OR balance_clears_failed
                    OR orig_amount IS NULL OR orig_amount=''
                    OR orig_balance IS NULL OR orig_balance=''
                ) AS update_candidate
            FROM change_flags
        ),
        desired_values AS (
            SELECT
                id,
                orig_amount,
                orig_balance,
                clean_amount,
                clean_balance,
                clean_amt_fixed,
                clean_amt_failed,
                clean_bal_fixed,
                clean_bal_failed,
                update_candidate,
                CASE
                    WHEN update_candidate THEN COALESCE(NULLIF(orig_amount,''), NULLIF(amt_old,''))
                    ELSE orig_amount
                END AS orig_amount_new,
                CASE
                    WHEN update_candidate THEN COALESCE(NULLIF(orig_balance,''), NULLIF(bal_old,''))
                    ELSE orig_balance
                END AS orig_balance_new,
                CASE
                    WHEN NOT update_candidate THEN clean_amount
                    WHEN amount_needs_failed THEN NULL
                    WHEN amt_valid THEN amt_norm
                    ELSE clean_amount
                END AS clean_amount_new,
                CASE
                    WHEN NOT update_candidate THEN clean_balance
                    WHEN balance_needs_failed THEN NULL
                    WHEN bal_valid THEN bal_norm
                    ELSE clean_balance
                END AS clean_balance_new,
                CASE
                    WHEN update_candidate
                         AND amount_needs_fixed
                    THEN 1 ELSE clean_amt_fixed END AS clean_amt_fixed_new,
                CASE
                    WHEN NOT update_candidate THEN clean_amt_failed
                    WHEN amount_needs_failed THEN 1
                    WHEN amt_valid THEN 0
                    ELSE clean_amt_failed
                END AS clean_amt_failed_new,
                CASE
                    WHEN update_candidate
                         AND balance_needs_fixed
                    THEN 1 ELSE clean_bal_fixed END AS clean_bal_fixed_new,
                CASE
                    WHEN NOT update_candidate THEN clean_bal_failed
                    WHEN balance_needs_failed THEN 1
                    WHEN bal_valid THEN 0
                    ELSE clean_bal_failed
                END AS clean_bal_failed_new,
                CASE
                    WHEN update_candidate
                         AND amount_needs_fixed
                         AND (
                             clean_amount IS DISTINCT FROM amt_norm
                             OR clean_amt_fixed IS DISTINCT FROM 1
                             OR clean_amt_failed IS DISTINCT FROM 0
                    )
                    THEN 1 ELSE 0 END AS amount_updated_changed,
                CASE
                    WHEN update_candidate
                         AND amount_needs_failed
                         AND (
                             clean_amount IS NOT NULL
                             OR clean_amt_failed IS DISTINCT FROM 1
                    )
                    THEN 1 ELSE 0 END AS amount_failed_changed,
                CASE
                    WHEN update_candidate
                         AND balance_needs_fixed
                         AND (
                             clean_balance IS DISTINCT FROM bal_norm
                             OR clean_bal_fixed IS DISTINCT FROM 1
                             OR clean_bal_failed IS DISTINCT FROM 0
                    )
                    THEN 1 ELSE 0 END AS balance_updated_changed,
                CASE
                    WHEN update_candidate
                         AND balance_needs_failed
                         AND (
                             clean_balance IS NOT NULL
                             OR clean_bal_failed IS DISTINCT FROM 1
                         )
                    THEN 1 ELSE 0 END AS balance_failed_changed
            FROM joined
        )
        SELECT
            id,
            orig_amount_new,
            orig_balance_new,
            clean_amount_new,
            clean_balance_new,
            clean_amt_fixed_new,
            clean_amt_failed_new,
            clean_bal_fixed_new,
            clean_bal_failed_new,
            amount_failed_changed,
            amount_updated_changed,
            balance_failed_changed,
            balance_updated_changed,
            CASE
                WHEN update_candidate
                     AND (
                         orig_amount_new IS DISTINCT FROM orig_amount
                         OR orig_balance_new IS DISTINCT FROM orig_balance
                         OR clean_amount_new IS DISTINCT FROM clean_amount
                         OR clean_balance_new IS DISTINCT FROM clean_balance
                         OR clean_amt_fixed_new IS DISTINCT FROM clean_amt_fixed
                         OR clean_amt_failed_new IS DISTINCT FROM clean_amt_failed
                         OR clean_bal_fixed_new IS DISTINCT FROM clean_bal_fixed
                         OR clean_bal_failed_new IS DISTINCT FROM clean_bal_failed
                     )
                THEN 1 ELSE 0 END AS should_update
        FROM desired_values
        "#;
    conn.execute_batch(desired_sql)?;
    timings.desired_ms = elapsed_ms(desired_started);

    let counts_started = Instant::now();
    let counts_sql = r#"
        SELECT
            SUM(amount_failed_changed) AS amt_failed,
            SUM(amount_updated_changed) AS amt_fixed,
            SUM(balance_failed_changed) AS bal_failed,
            SUM(balance_updated_changed) AS bal_fixed,
            SUM(should_update) AS rows_affected
        FROM amount_balance_desired
        "#;
    let (amount_failed, amount_updated, balance_failed, balance_updated, rows_affected) =
        amount_balance_delta_count_row(conn, counts_sql)?;
    timings.counts_ms = elapsed_ms(counts_started);

    let update_started = Instant::now();
    let update_sql = r#"
        UPDATE fc_transaction_norm AS t
           SET orig_amount=d.orig_amount_new,
               orig_balance=d.orig_balance_new,
               clean_amount=d.clean_amount_new,
               clean_balance=d.clean_balance_new,
               clean_amt_fixed=d.clean_amt_fixed_new,
               clean_amt_failed=d.clean_amt_failed_new,
               clean_bal_fixed=d.clean_bal_fixed_new,
               clean_bal_failed=d.clean_bal_failed_new
          FROM amount_balance_desired d
         WHERE t.id=d.id
           AND d.should_update=1
        "#;
    conn.execute_batch(update_sql)?;
    timings.update_ms = elapsed_ms(update_started);

    let totals_started = Instant::now();
    let totals_sql = format!(
        r#"
        SELECT
            SUM(CASE WHEN clean_amt_fixed=1 THEN 1 ELSE 0 END),
            SUM(CASE WHEN clean_amt_failed=1 THEN 1 ELSE 0 END),
            SUM(CASE WHEN clean_bal_fixed=1 THEN 1 ELSE 0 END),
            SUM(CASE WHEN clean_bal_failed=1 THEN 1 ELSE 0 END)
        FROM fc_transaction_norm
        WHERE case_id={}
        "#,
        sql_literal(&args.case_id)
    );
    let (amount_fixed_total, amount_failed_total, balance_fixed_total, balance_failed_total) =
        amount_balance_count_row(conn, &totals_sql)?;
    timings.totals_ms = elapsed_ms(totals_started);

    Ok(AmountBalanceSummary {
        amount_updated,
        balance_updated,
        amount_failed,
        balance_failed,
        amount_fixed_total,
        amount_failed_total,
        balance_fixed_total,
        balance_failed_total,
        rows_affected,
        timings,
    })
}

fn amount_balance_count_row(conn: &Connection, sql: &str) -> Result<(i64, i64, i64, i64)> {
    let mut stmt = conn.prepare(sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let first: Option<i64> = row.get(0)?;
        let second: Option<i64> = row.get(1)?;
        let third: Option<i64> = row.get(2)?;
        let fourth: Option<i64> = row.get(3)?;
        Ok((
            first.unwrap_or(0),
            second.unwrap_or(0),
            third.unwrap_or(0),
            fourth.unwrap_or(0),
        ))
    } else {
        Ok((0, 0, 0, 0))
    }
}

fn amount_balance_delta_count_row(
    conn: &Connection,
    sql: &str,
) -> Result<(i64, i64, i64, i64, i64)> {
    let mut stmt = conn.prepare(sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let first: Option<i64> = row.get(0)?;
        let second: Option<i64> = row.get(1)?;
        let third: Option<i64> = row.get(2)?;
        let fourth: Option<i64> = row.get(3)?;
        let fifth: Option<i64> = row.get(4)?;
        Ok((
            first.unwrap_or(0),
            second.unwrap_or(0),
            third.unwrap_or(0),
            fourth.unwrap_or(0),
            fifth.unwrap_or(0),
        ))
    } else {
        Ok((0, 0, 0, 0, 0))
    }
}

fn normalize_number_text(raw: &str) -> Option<Cow<'_, str>> {
    let src = normalize_source_text(raw);
    if src.is_empty() {
        return None;
    }

    let len = src.len();
    if src.as_bytes().first() == Some(&b'(') && src.as_bytes().last() == Some(&b')') {
        let valid = valid_unsigned_number_text(&src[1..len - 1]);
        if valid {
            return Some(normalized_text_range(src, 1, len - 1));
        }
    }

    let plain_start = match src.as_bytes().first() {
        Some(b'-') | Some(b'+') => 1,
        _ => 0,
    };
    valid_unsigned_number_text(&src[plain_start..])
        .then(|| normalized_text_range(src, plain_start, len))
}

fn normalize_source_text(raw: &str) -> Cow<'_, str> {
    let trimmed = raw.trim();
    if !trimmed.as_bytes().contains(&b' ') && !trimmed.contains('，') {
        return Cow::Borrowed(trimmed);
    }

    let mut normalized = String::with_capacity(trimmed.len());
    for ch in trimmed.chars() {
        match ch {
            ' ' => {}
            '，' => normalized.push(','),
            _ => normalized.push(ch),
        }
    }
    Cow::Owned(normalized)
}

fn normalized_text_range<'a>(src: Cow<'a, str>, start: usize, end: usize) -> Cow<'a, str> {
    normalize_leading_decimal(match src {
        Cow::Borrowed(value) => strip_ascii_commas(Cow::Borrowed(&value[start..end])),
        Cow::Owned(value) => {
            if start == 0 && end == value.len() {
                strip_ascii_commas(Cow::Owned(value))
            } else {
                strip_ascii_commas(Cow::Owned(value[start..end].to_string()))
            }
        }
    })
}

fn strip_ascii_commas(value: Cow<'_, str>) -> Cow<'_, str> {
    if !value.as_bytes().contains(&b',') {
        return value;
    }
    Cow::Owned(value.chars().filter(|ch| *ch != ',').collect())
}

fn normalize_leading_decimal(value: Cow<'_, str>) -> Cow<'_, str> {
    if !value.as_ref().starts_with('.') {
        return value;
    }
    Cow::Owned(format!("0{}", value.as_ref()))
}

fn valid_unsigned_number_text(value: &str) -> bool {
    let bytes = value.as_bytes();
    if bytes.is_empty() {
        return false;
    }

    let mut index = 0;
    let mut has_integer_digits = false;
    while index < bytes.len() && bytes[index].is_ascii_digit() {
        has_integer_digits = true;
        index += 1;
    }

    while index < bytes.len() && bytes[index] == b',' {
        if !has_integer_digits {
            return false;
        }
        index += 1;
        for _ in 0..3 {
            if index >= bytes.len() || !bytes[index].is_ascii_digit() {
                return false;
            }
            index += 1;
        }
    }

    if index < bytes.len() && bytes[index] == b'.' {
        index += 1;
        let decimal_start = index;
        while index < bytes.len() && bytes[index].is_ascii_digit() {
            index += 1;
        }
        if index == decimal_start {
            return false;
        }
    } else if !has_integer_digits {
        return false;
    }

    if index < bytes.len() && matches!(bytes[index], b'e' | b'E') {
        index += 1;
        if index < bytes.len() && matches!(bytes[index], b'+' | b'-') {
            index += 1;
        }
        let exponent_start = index;
        while index < bytes.len() && bytes[index].is_ascii_digit() {
            index += 1;
        }
        if index == exponent_start {
            return false;
        }
    }

    index == bytes.len()
}

fn register_number_norm_function(conn: &Connection) -> Result<()> {
    conn.register_scalar_function::<AmountBalanceNumberNorm>("analytix_amount_number_norm")?;
    Ok(())
}

struct AmountBalanceNumberNorm;

impl VScalar for AmountBalanceNumberNorm {
    type State = ();

    fn invoke(
        _: &Self::State,
        input: &mut DataChunkHandle,
        output: &mut dyn WritableVector,
    ) -> std::result::Result<(), Box<dyn std::error::Error>> {
        let len = input.len();
        let values = input.flat_vector(0);
        let values_slice = unsafe { values.as_slice_with_len::<duckdb_string_t>(len) };
        let mut output = output.flat_vector();

        for (index, value) in values_slice.iter().enumerate().take(len) {
            if values.row_is_null(index as u64) {
                output.set_null(index);
                continue;
            }

            let mut value = *value;
            let raw = DuckString::new(&mut value).as_str();
            if let Some(normalized) = normalize_number_text(raw.as_ref()) {
                output.insert(index, normalized.as_ref());
            } else {
                output.set_null(index);
            }
        }

        Ok(())
    }

    fn signatures() -> Vec<ScalarFunctionSignature> {
        vec![ScalarFunctionSignature::exact(
            vec![LogicalTypeHandle::from(LogicalTypeId::Varchar)],
            LogicalTypeHandle::from(LogicalTypeId::Varchar),
        )]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rust_number_normalization_matches_sql_expression_cases() -> Result<()> {
        let cases = [
            ("", None),
            ("   ", None),
            ("1,000.50", Some("1000.50")),
            ("2，000.75", Some("2000.75")),
            ("1 800.75", Some("1800.75")),
            ("(200.00)", Some("200.00")),
            ("(1,000.50)", Some("1000.50")),
            ("-30", Some("30")),
            ("+30", Some("30")),
            (" -30 ", Some("30")),
            ("1.2e3", Some("1.2e3")),
            ("1.2E+3", Some("1.2E+3")),
            ("1e9999", Some("1e9999")),
            ("1,234,567", Some("1234567")),
            ("1234,567", Some("1234567")),
            (".42", Some("0.42")),
            ("-.42", Some("0.42")),
            ("+.42", Some("0.42")),
            ("(.42)", Some("0.42")),
            (".42e3", Some("0.42e3")),
            (
                "99999999999999999999999999999999999999999999999999",
                Some("99999999999999999999999999999999999999999999999999"),
            ),
            ("abc", None),
            ("bad-balance", None),
            ("1,23", None),
            ("1,2345", None),
            ("(abc)", None),
            ("(1,23)", None),
            ("(+30)", None),
            (".", None),
            ("e3", None),
            ("--30", None),
        ];

        let conn = Connection::open_in_memory()?;
        register_number_norm_function(&conn)?;
        let null_actual: Option<String> =
            conn.query_row("SELECT analytix_amount_number_norm(NULL)", [], |row| {
                row.get(0)
            })?;
        assert_eq!(null_actual, None);

        for (raw, expected) in cases {
            let sql = format!("SELECT analytix_amount_number_norm({})", sql_literal(raw));
            let actual: Option<String> = conn.query_row(&sql, [], |row| row.get(0))?;

            assert_eq!(
                actual.as_deref(),
                expected,
                "SQL normalization mismatch for {raw:?}"
            );
            assert_eq!(
                normalize_number_text(raw).as_deref(),
                expected,
                "Rust normalization mismatch for {raw:?}"
            );

            if let Some(normalized) = expected {
                let cast_sql = format!(
                    "SELECT TRY_CAST({} AS DOUBLE) IS NOT NULL",
                    sql_literal(normalized)
                );
                let parseable: bool = conn.query_row(&cast_sql, [], |row| row.get(0))?;
                assert!(
                    parseable,
                    "normalized value should preserve prior TRY_CAST parseability for {raw:?}"
                );
            }
        }

        Ok(())
    }
}
