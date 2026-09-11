use anyhow::Result;
use duckdb::Connection;
use sha2::{Digest, Sha256};

use super::args::MaterializeArgs;
use super::identity::MaterializationIdentity;

use crate::{
    sql_literal, ACCOUNT_DIM_STAGING_TABLE, ACCOUNT_DIM_TABLE, AGG_STAGING_TABLE, AGG_TABLE,
    AGG_VERSION, DETAIL_STAGING_TABLE, DETAIL_TABLE, KEYWORD_STAGING_TABLE, KEYWORD_TABLE,
    MATERIALIZATION_IDENTITY_SCHEMA_VERSION, META_TABLE,
};

pub(super) fn swap_tables_and_index(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {AGG_TABLE};
        ALTER TABLE {AGG_STAGING_TABLE} RENAME TO {AGG_TABLE};
        DROP TABLE IF EXISTS {DETAIL_TABLE};
        ALTER TABLE {DETAIL_STAGING_TABLE} RENAME TO {DETAIL_TABLE};
        DROP TABLE IF EXISTS {KEYWORD_TABLE};
        ALTER TABLE {KEYWORD_STAGING_TABLE} RENAME TO {KEYWORD_TABLE};
        DROP TABLE IF EXISTS {ACCOUNT_DIM_TABLE};
        ALTER TABLE {ACCOUNT_DIM_STAGING_TABLE} RENAME TO {ACCOUNT_DIM_TABLE};
        "
    ))?;
    for sql in [
        format!("CREATE INDEX IF NOT EXISTS idx_detail_acct_ts ON {DETAIL_TABLE}(acct_key, txn_ts)"),
        format!("CREATE INDEX IF NOT EXISTS idx_detail_txn_id ON {DETAIL_TABLE}(txn_id)"),
        format!("CREATE INDEX IF NOT EXISTS idx_detail_file_ts ON {DETAIL_TABLE}(file_id, txn_ts)"),
        format!(
            "CREATE INDEX IF NOT EXISTS idx_{KEYWORD_TABLE}_kind_token_id ON {KEYWORD_TABLE}(kind, token, txn_row_id)"
        ),
        format!(
            "CREATE INDEX IF NOT EXISTS idx_{KEYWORD_TABLE}_id_kind ON {KEYWORD_TABLE}(txn_row_id, kind)"
        ),
        format!("CREATE INDEX IF NOT EXISTS idx_account_dim_key ON {ACCOUNT_DIM_TABLE}(account_key)"),
    ] {
        let _ = conn.execute_batch(&sql);
    }
    Ok(())
}

pub(super) fn upsert_meta(
    conn: &Connection,
    args: &MaterializeArgs,
    identity: &MaterializationIdentity,
    row_count: i64,
) -> Result<()> {
    let actual_row_count = crate::scalar_i64(conn, &format!("SELECT COUNT(1) FROM {AGG_TABLE}"))?;
    if actual_row_count != row_count {
        anyhow::bail!("materialization result row count is inconsistent");
    }
    let result_signature = materialization_result_signature(conn, &args.case_id)?;
    let max_ts_sql = if args.source_max_txn_ts.trim().is_empty() {
        "NULL".to_string()
    } else {
        format!(
            "CAST({} AS TIMESTAMP)",
            sql_literal(&args.source_max_txn_ts)
        )
    };
    conn.execute_batch(&format!(
        "
        DELETE FROM {META_TABLE}
         WHERE starts_with(agg_name, 'txn_daily_')
           AND agg_name <> {};
        INSERT INTO {META_TABLE}(
            agg_name, agg_version, case_id, identity_schema_version,
            source_revision, source_row_count, source_max_txn_ts, source_max_id,
            source_signature, result_signature, built_at, row_count
        )
        VALUES ({}, {AGG_VERSION}, {}, {MATERIALIZATION_IDENTITY_SCHEMA_VERSION},
                {}, {}, {max_ts_sql}, {}, {}, {}, now(), {row_count})
        ON CONFLICT(agg_name) DO NOTHING;
        ",
        sql_literal(&identity.value),
        sql_literal(&identity.value),
        sql_literal(&args.case_id),
        args.source_revision,
        args.source_row_count,
        args.source_max_id,
        sql_literal(&identity.source_signature),
        sql_literal(&result_signature),
    ))?;
    let row = conn.query_row(
        &format!(
            "SELECT COUNT(1), COALESCE(MAX(agg_version), 0), COALESCE(MAX(case_id), ''), \
                    COALESCE(MAX(identity_schema_version), 0), COALESCE(MAX(source_revision), 0), \
                    COALESCE(MAX(source_row_count), 0), \
                    COALESCE(CAST(MAX(source_max_txn_ts) AS VARCHAR), ''), \
                    COALESCE(MAX(source_max_id), 0), COALESCE(MAX(source_signature), ''), \
                    COALESCE(MAX(result_signature), ''), COALESCE(MAX(row_count), 0) \
               FROM {META_TABLE} WHERE starts_with(agg_name, 'txn_daily_')"
        ),
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, i64>(5)?,
                row.get::<_, String>(6)?,
                row.get::<_, i64>(7)?,
                row.get::<_, String>(8)?,
                row.get::<_, String>(9)?,
                row.get::<_, i64>(10)?,
            ))
        },
    )?;
    let expected = (
        1,
        AGG_VERSION,
        args.case_id.clone(),
        MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
        args.source_revision,
        args.source_row_count,
        args.source_max_txn_ts.clone(),
        args.source_max_id,
        identity.source_signature.clone(),
        result_signature,
        row_count,
    );
    if row != expected {
        anyhow::bail!("materialization identity metadata is inconsistent");
    }
    Ok(())
}

pub(crate) fn materialization_result_signature(conn: &Connection, case_id: &str) -> Result<String> {
    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, b"analytix.txn-daily-result/v12");
    hash_framed(&mut hasher, case_id.as_bytes());
    for (table, order_by) in [
        (AGG_TABLE, "r.acct_key, r.txn_day, r.cp_key, r.dc_val"),
        (DETAIL_TABLE, "TRY_CAST(r.id AS BIGINT), r.id"),
        (
            KEYWORD_TABLE,
            "TRY_CAST(r.txn_row_id AS BIGINT), r.txn_row_id, r.kind, r.token, r.token_order",
        ),
        (ACCOUNT_DIM_TABLE, "r.account_key"),
    ] {
        crate::ensure_required_table(conn, table)?;
        let columns = crate::table_columns(conn, table)?;
        if columns.is_empty() {
            anyhow::bail!("{table} result schema is unavailable");
        }
        hash_framed(&mut hasher, table.as_bytes());
        for column in columns {
            hash_framed(&mut hasher, column.as_bytes());
        }
        let mut statement = conn.prepare(&format!(
            "SELECT to_json(r) FROM {table} r ORDER BY {order_by}, to_json(r)"
        ))?;
        let rows = statement.query_map([], |row| row.get::<_, String>(0))?;
        for row in rows {
            hash_framed(&mut hasher, row?.as_bytes());
        }
    }
    Ok(format!("{:x}", hasher.finalize()))
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn result_signature_has_cross_language_stable_typed_rows() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE analysis_txn_daily_agg( \
               acct_key TEXT, txn_day DATE, cp_key TEXT, dc_val TEXT \
             ); \
             INSERT INTO analysis_txn_daily_agg VALUES \
               ('账户一', DATE '2026-01-02', '对手甲', '进'), \
               ('账户一', DATE '2026-01-01', NULL, '出'); \
             CREATE TABLE analysis_txn_detail_idx( \
               id BIGINT, amount DOUBLE, note TEXT, exact_amount DECIMAL(20,4), \
               huge_value HUGEINT, observed_at TIMESTAMPTZ \
             ); \
             INSERT INTO analysis_txn_detail_idx VALUES \
               (2, NULL, '中文', 12.3400, 170141183460469231731687303715884105, \
                TIMESTAMPTZ '2026-01-01 10:00:00+08:00'), \
               (1, 0, 'zero', 0.0000, 0, NULL); \
             CREATE TABLE analysis_txn_keyword_idx( \
               txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT \
             ); \
             INSERT INTO analysis_txn_keyword_idx VALUES \
               (2, 'summary', '关键词', 0), (1, 'remark', 'zero', 0); \
             CREATE TABLE analysis_account_dim(account_key TEXT, label TEXT); \
             INSERT INTO analysis_account_dim VALUES ('账户一', '测试账户');",
        )
        .expect("seed typed materialization rows");

        let signature =
            materialization_result_signature(&conn, "case-跨语言").expect("hash result tables");
        assert_eq!(
            signature,
            "affbe8e3b71ba16deea7ed1254a5b1505956f70ddfb7881c64556da6758c07cf"
        );
        crate::ensure_meta_table(&conn).expect("create materialization meta");
        let args = MaterializeArgs {
            case_id: "case-跨语言".to_string(),
            db_path: Default::default(),
            source_revision: 1,
            source_row_count: 2,
            source_max_txn_ts: String::new(),
            source_max_id: 2,
        };
        let identity = MaterializationIdentity {
            value: format!(
                "{}{}",
                crate::MATERIALIZATION_IDENTITY_PREFIX,
                "a".repeat(64)
            ),
            source_signature: "a".repeat(64),
        };
        upsert_meta(&conn, &args, &identity, 2).expect("persist result signature");
        let stored = conn
            .query_row(
                &format!(
                    "SELECT result_signature FROM {} WHERE agg_name=?",
                    crate::META_TABLE
                ),
                [identity.value.as_str()],
                |row| row.get::<_, String>(0),
            )
            .expect("read stored result signature");
        assert_eq!(stored, signature);
    }
}
