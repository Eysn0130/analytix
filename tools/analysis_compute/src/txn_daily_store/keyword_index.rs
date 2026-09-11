use anyhow::Result;
use duckdb::{params, Connection};

use crate::stats_query_store::chart_keywords::{remark_keyword_tokens, summary_keyword_tokens};
use crate::{DETAIL_STAGING_TABLE, KEYWORD_STAGING_TABLE};

pub(super) fn create_keyword_index(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {KEYWORD_STAGING_TABLE};
        CREATE TABLE {KEYWORD_STAGING_TABLE}(
          txn_row_id BIGINT NOT NULL,
          stable_txn_id TEXT NOT NULL,
          kind TEXT NOT NULL,
          token TEXT NOT NULL,
          token_order BIGINT NOT NULL
        );
        "
    ))?;

    let sql = format!(
        "
        SELECT
          id,
          COALESCE(NULLIF(TRIM(COALESCE(txn_id, '')), ''), CAST(id AS VARCHAR)) AS stable_txn_id,
          COALESCE(NULLIF(TRIM(summary), ''), NULLIF(TRIM(remark), ''), '') AS summary_text,
          COALESCE(remark, '') AS remark_text
        FROM {DETAIL_STAGING_TABLE}
        ORDER BY id
        "
    );
    let mut appender = conn.appender(KEYWORD_STAGING_TABLE)?;
    let mut stmt = conn.prepare(&sql)?;
    let mut rows = stmt.query([])?;
    while let Some(row) = rows.next()? {
        let id = row.get::<_, i64>(0)?;
        let stable_txn_id = row.get::<_, String>(1)?;
        let summary_text = row.get::<_, String>(2)?;
        let remark_text = row.get::<_, String>(3)?;

        for (token_order, token) in summary_keyword_tokens(&summary_text)
            .into_iter()
            .enumerate()
        {
            appender.append_row(params![
                id,
                stable_txn_id.as_str(),
                "summary",
                token,
                token_order as i64
            ])?;
        }
        for (token_order, token) in remark_keyword_tokens(&remark_text).into_iter().enumerate() {
            appender.append_row(params![
                id,
                stable_txn_id.as_str(),
                "remark",
                token,
                token_order as i64
            ])?;
        }
    }
    appender.flush()?;
    Ok(())
}
