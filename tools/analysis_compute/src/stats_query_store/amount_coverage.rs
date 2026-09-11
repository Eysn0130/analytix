use anyhow::{bail, Result};
use duckdb::Connection;

pub(super) const AMOUNT_COVERAGE_INCOMPLETE: &str = "transaction_amount_coverage_incomplete";

pub(super) fn require_complete_detail_amount_coverage(conn: &Connection) -> Result<()> {
    let counts = conn.query_row(
        &format!(
            "SELECT \
               COUNT(1), \
               COALESCE(SUM(CASE WHEN amount_source_present=1 THEN 1 ELSE 0 END), 0), \
               COALESCE(SUM(CASE WHEN amount IS NOT NULL AND isfinite(amount) \
                                      AND amount_source_present=1 AND amount_parse_failed=0 \
                                 THEN 1 ELSE 0 END), 0), \
               COALESCE(SUM(CASE WHEN amount IS NULL AND amount_source_present=0 \
                                      AND amount_parse_failed=0 THEN 1 ELSE 0 END), 0), \
               COALESCE(SUM(CASE WHEN amount IS NULL AND amount_source_present=1 \
                                      AND amount_parse_failed=1 THEN 1 ELSE 0 END), 0), \
               COALESCE(SUM(CASE WHEN dc_val IN ('进','出') THEN 1 ELSE 0 END), 0) \
             FROM {}",
            crate::DETAIL_TABLE
        ),
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, i64>(5)?,
            ))
        },
    )?;
    let (total, source_present, valid, missing, parse_failed, direction_covered) = counts;
    if total <= 0
        || source_present != total
        || valid != total
        || missing != 0
        || parse_failed != 0
        || direction_covered != total
        || valid + missing + parse_failed != total
        || source_present != valid + parse_failed
    {
        bail!(AMOUNT_COVERAGE_INCOMPLETE);
    }
    Ok(())
}

pub(super) fn require_complete_aggregate_amount_coverage(conn: &Connection) -> Result<()> {
    let counts = conn.query_row(
        &format!(
            "SELECT \
               COALESCE(SUM(txn_count), 0), \
               COALESCE(SUM(amount_source_present_count), 0), \
               COALESCE(SUM(amount_valid_count), 0), \
               COALESCE(SUM(amount_missing_count), 0), \
               COALESCE(SUM(amount_parse_failed_count), 0), \
               COALESCE(SUM(CASE WHEN dc_val IN ('进','出') THEN txn_count ELSE 0 END), 0), \
               COALESCE(SUM(CASE WHEN amt_sum IS NOT NULL AND isfinite(amt_sum) \
                                      THEN txn_count ELSE 0 END), 0) \
             FROM {}",
            crate::AGG_TABLE
        ),
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, i64>(5)?,
                row.get::<_, i64>(6)?,
            ))
        },
    )?;
    let (total, source_present, valid, missing, parse_failed, direction_covered, summed) = counts;
    if total <= 0
        || source_present != total
        || valid != total
        || missing != 0
        || parse_failed != 0
        || direction_covered != total
        || summed != total
        || valid + missing + parse_failed != total
        || source_present != valid + parse_failed
    {
        bail!(AMOUNT_COVERAGE_INCOMPLETE);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn real_zero_is_complete_but_missing_parse_failure_and_unknown_direction_block() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(&format!(
            "CREATE TABLE {}(amount DOUBLE, amount_source_present BIGINT, \
                                  amount_parse_failed BIGINT, dc_val TEXT); \
             INSERT INTO {} VALUES (0,1,0,'进'), (12.34,1,0,'出');",
            crate::DETAIL_TABLE,
            crate::DETAIL_TABLE
        ))
        .expect("seed complete detail");
        require_complete_detail_amount_coverage(&conn).expect("real zero is valid evidence");

        conn.execute(
            &format!("INSERT INTO {} VALUES (NULL,0,0,'进')", crate::DETAIL_TABLE),
            [],
        )
        .expect("seed missing amount");
        let error = require_complete_detail_amount_coverage(&conn).expect_err("missing must block");
        assert_eq!(error.to_string(), AMOUNT_COVERAGE_INCOMPLETE);
    }

    #[test]
    fn aggregate_partial_sum_is_rejected_before_query_consumers() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(&format!(
            "CREATE TABLE {}(txn_count BIGINT, amount_source_present_count BIGINT, \
                               amount_valid_count BIGINT, amount_missing_count BIGINT, \
                               amount_parse_failed_count BIGINT, dc_val TEXT, amt_sum DOUBLE); \
             INSERT INTO {} VALUES (1,1,1,0,0,'进',100), (1,0,0,1,0,'出',NULL);",
            crate::AGG_TABLE,
            crate::AGG_TABLE
        ))
        .expect("seed partial aggregate");

        let error = require_complete_aggregate_amount_coverage(&conn)
            .expect_err("partial aggregate must not become a subset sum");
        assert_eq!(error.to_string(), AMOUNT_COVERAGE_INCOMPLETE);
    }
}
