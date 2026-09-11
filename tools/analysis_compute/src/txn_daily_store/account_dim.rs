use anyhow::Result;
use duckdb::Connection;

use crate::{
    account_key_expr, coalesce_expr, sql_literal, table_columns, table_exists, text_col_expr,
    text_col_expr_minlen, ACCOUNT_DIM_STAGING_TABLE, ACCOUNT_KEY_CANDIDATES, DETAIL_STAGING_TABLE,
    NULL_TEXT,
};

pub(super) fn create_account_dim(conn: &Connection, case_id: &str) -> Result<()> {
    if table_exists(conn, "fc_account_norm")? {
        let acct_cols = table_columns(conn, "fc_account_norm")?;
        let acct_key_expr_a = account_key_expr("a", &acct_cols, ACCOUNT_KEY_CANDIDATES);
        let acct_open_name_expr = text_col_expr("a", &acct_cols, "account_open_name", true)
            .unwrap_or_else(|| NULL_TEXT.to_string());
        let acct_id_no_expr = text_col_expr("a", &acct_cols, "opener_id_no", true)
            .unwrap_or_else(|| NULL_TEXT.to_string());
        let acct_bank_expr = coalesce_expr(
            &["open_bank", "branch_name"]
                .iter()
                .map(|col| text_col_expr_minlen("a", &acct_cols, col, 2))
                .collect::<Vec<_>>(),
            NULL_TEXT,
        );
        let acct_branch_expr = text_col_expr_minlen("a", &acct_cols, "branch_name", 2)
            .unwrap_or_else(|| NULL_TEXT.to_string());
        let acct_type_expr = text_col_expr_minlen("a", &acct_cols, "acct_type", 2)
            .unwrap_or_else(|| NULL_TEXT.to_string());
        let mut sub_parent_ctes = String::new();
        let mut sub_parent_join = String::new();
        let mut sub_open_name_expr = "NULL".to_string();
        let mut sub_id_no_expr = "NULL".to_string();
        let mut sub_bank_expr_out = "NULL".to_string();
        let mut sub_branch_expr_out = "NULL".to_string();
        let mut sub_type_expr_out = "NULL".to_string();
        if table_exists(conn, "fc_sub_account_norm")? {
            let sub_cols = table_columns(conn, "fc_sub_account_norm")?;
            let sub_acct_expr = text_col_expr("s", &sub_cols, "sub_acct", true);
            let parent_acct_expr = text_col_expr("s", &sub_cols, "parent_acct", true);
            let has_case_id = sub_cols.iter().any(|col| col == "case_id");
            if has_case_id {
                if let (Some(sub_acct_expr), Some(parent_acct_expr)) =
                    (sub_acct_expr, parent_acct_expr)
                {
                    let sub_bank_expr = text_col_expr_minlen("s", &sub_cols, "bank_name", 2)
                        .unwrap_or_else(|| NULL_TEXT.to_string());
                    let sub_type_expr = text_col_expr_minlen("s", &sub_cols, "sub_type", 2)
                        .unwrap_or_else(|| NULL_TEXT.to_string());
                    sub_parent_ctes = format!(
                        ",
            sub_parent_base AS (
              SELECT {sub_acct_expr} AS account_key,
                     {parent_acct_expr} AS parent_account_key,
                     {sub_bank_expr} AS sub_bank_name,
                     {sub_type_expr} AS sub_acct_type
              FROM fc_sub_account_norm s
              WHERE s.case_id={}
            ), sub_parent_joined AS (
              SELECT s.account_key,
                     a.acct_open_name,
                     a.acct_id_no,
                     COALESCE(NULLIF(s.sub_bank_name, ''), NULLIF(a.bank_name, '')) AS bank_name,
                     a.branch_name,
                     COALESCE(NULLIF(s.sub_acct_type, ''), NULLIF(a.acct_type, '')) AS acct_type
              FROM sub_parent_base s
              JOIN acct_dim a ON a.account_key = s.parent_account_key
              WHERE s.account_key IS NOT NULL
                AND TRIM(s.account_key) <> ''
                AND s.parent_account_key IS NOT NULL
                AND TRIM(s.parent_account_key) <> ''
            ), sub_parent_dim AS (
              SELECT account_key,
                     CASE WHEN COUNT(DISTINCT NULLIF(acct_open_name, '')) = 1
                       THEN MAX(NULLIF(acct_open_name, '')) ELSE NULL END AS parent_open_name,
                     CASE WHEN COUNT(DISTINCT NULLIF(acct_id_no, '')) = 1
                       THEN MAX(NULLIF(acct_id_no, '')) ELSE NULL END AS parent_id_no,
                     CASE WHEN COUNT(DISTINCT NULLIF(bank_name, '')) = 1
                       THEN MAX(NULLIF(bank_name, '')) ELSE NULL END AS parent_bank_name,
                     CASE WHEN COUNT(DISTINCT NULLIF(branch_name, '')) = 1
                       THEN MAX(NULLIF(branch_name, '')) ELSE NULL END AS parent_branch_name,
                     CASE WHEN COUNT(DISTINCT NULLIF(acct_type, '')) = 1
                       THEN MAX(NULLIF(acct_type, '')) ELSE NULL END AS parent_acct_type
              FROM sub_parent_joined
              GROUP BY account_key
            )",
                        sql_literal(case_id)
                    );
                    sub_parent_join =
                        "LEFT JOIN sub_parent_dim sp ON sp.account_key = b.account_key".to_string();
                    sub_open_name_expr = "NULLIF(MAX(sp.parent_open_name), '')".to_string();
                    sub_id_no_expr = "NULLIF(MAX(sp.parent_id_no), '')".to_string();
                    sub_bank_expr_out = "NULLIF(MAX(sp.parent_bank_name), '')".to_string();
                    sub_branch_expr_out = "NULLIF(MAX(sp.parent_branch_name), '')".to_string();
                    sub_type_expr_out = "NULLIF(MAX(sp.parent_acct_type), '')".to_string();
                }
            }
        }
        conn.execute_batch(&format!(
            "
            CREATE TABLE {ACCOUNT_DIM_STAGING_TABLE} AS
            WITH txn_base AS (
              SELECT acct_key AS account_key,
                     COALESCE(NULLIF(acct_no, ''), acct_key) AS acct_display,
                     COALESCE(NULLIF(card_no, ''), NULLIF(acct_no, ''), acct_key) AS card_display,
                     account_open_name AS open_name,
                     opener_id_no AS id_no,
                     txn_ts AS txn_ts
              FROM {DETAIL_STAGING_TABLE}
            ), txn_card_ranked AS (
              SELECT account_key, card_display, txn_ts,
                     ROW_NUMBER() OVER (
                       PARTITION BY account_key
                       ORDER BY (txn_ts IS NOT NULL) DESC, txn_ts DESC, card_display DESC
                     ) AS rn
              FROM txn_base
              WHERE account_key IS NOT NULL AND TRIM(account_key) <> ''
                AND COALESCE(card_display, '') <> ''
            ), txn_card_pick AS (
              SELECT account_key, card_display
              FROM txn_card_ranked
              WHERE rn = 1
            ), txn_name_stats AS (
              SELECT account_key, open_name, id_no, COUNT(1) AS hit_count, MAX(txn_ts) AS last_ts
              FROM txn_base
              WHERE account_key IS NOT NULL AND TRIM(account_key) <> ''
                AND (COALESCE(open_name, '') <> '' OR COALESCE(id_no, '') <> '')
              GROUP BY account_key, open_name, id_no
            ), txn_name_pick AS (
              SELECT account_key, open_name, id_no
              FROM (
                SELECT account_key, open_name, id_no, hit_count, last_ts,
                       ROW_NUMBER() OVER (
                         PARTITION BY account_key
                         ORDER BY CASE WHEN COALESCE(open_name, '') <> '' THEN 1 ELSE 0 END DESC,
                                  hit_count DESC, (last_ts IS NOT NULL) DESC, last_ts DESC, open_name DESC, id_no DESC
                       ) AS rn
                FROM txn_name_stats
              ) ranked
              WHERE rn = 1
            ), acct_base AS (
              SELECT {acct_key_expr_a} AS account_key,
                     {acct_open_name_expr} AS acct_open_name,
                     {acct_id_no_expr} AS acct_id_no,
                     {acct_bank_expr} AS bank_name,
                     {acct_branch_expr} AS branch_name,
                     {acct_type_expr} AS acct_type
              FROM fc_account_norm a
              WHERE a.case_id={}
            ), acct_dim AS (
              SELECT account_key, MAX(acct_open_name) AS acct_open_name, MAX(acct_id_no) AS acct_id_no,
                     MAX(bank_name) AS bank_name, MAX(branch_name) AS branch_name, MAX(acct_type) AS acct_type
              FROM acct_base
              WHERE account_key IS NOT NULL AND TRIM(account_key) <> ''
              GROUP BY account_key
            ){sub_parent_ctes}
            SELECT
              b.account_key AS account_key,
              COALESCE(NULLIF(MAX(b.acct_display), ''), b.account_key) AS acct_display,
              COALESCE(NULLIF(MAX(c.card_display), ''), NULLIF(MAX(b.card_display), ''), NULLIF(MAX(b.acct_display), ''), b.account_key) AS card_display,
              COALESCE(NULLIF(MAX(a.acct_open_name), ''), NULLIF(MAX(p.open_name), ''), {sub_open_name_expr}, '') AS open_name,
              COALESCE(NULLIF(MAX(a.acct_id_no), ''), NULLIF(MAX(p.id_no), ''), {sub_id_no_expr}, '') AS id_no,
              COALESCE(NULLIF(MAX(a.bank_name), ''), {sub_bank_expr_out}, '') AS bank_name,
              COALESCE(NULLIF(MAX(a.branch_name), ''), {sub_branch_expr_out}, '') AS branch_name,
              COALESCE(NULLIF(MAX(a.acct_type), ''), {sub_type_expr_out}, '') AS acct_type
            FROM txn_base b
            LEFT JOIN txn_card_pick c ON c.account_key = b.account_key
            LEFT JOIN txn_name_pick p ON p.account_key = b.account_key
            LEFT JOIN acct_dim a ON a.account_key = b.account_key
            {sub_parent_join}
            WHERE b.account_key IS NOT NULL AND TRIM(b.account_key) <> ''
            GROUP BY b.account_key
            ORDER BY b.account_key
            ",
            sql_literal(case_id)
        ))?;
        return Ok(());
    }
    conn.execute_batch(&format!(
        "
        CREATE TABLE {ACCOUNT_DIM_STAGING_TABLE} AS
        WITH txn_base AS (
          SELECT acct_key AS account_key,
                 COALESCE(NULLIF(acct_no, ''), acct_key) AS acct_display,
                 COALESCE(NULLIF(card_no, ''), NULLIF(acct_no, ''), acct_key) AS card_display,
                 account_open_name AS open_name,
                 opener_id_no AS id_no,
                 txn_ts AS txn_ts
          FROM {DETAIL_STAGING_TABLE}
        ), txn_card_ranked AS (
          SELECT account_key, card_display, txn_ts,
                 ROW_NUMBER() OVER (
                   PARTITION BY account_key
                   ORDER BY (txn_ts IS NOT NULL) DESC, txn_ts DESC, card_display DESC
                 ) AS rn
          FROM txn_base
          WHERE account_key IS NOT NULL AND TRIM(account_key) <> ''
            AND COALESCE(card_display, '') <> ''
        ), txn_card_pick AS (
          SELECT account_key, card_display
          FROM txn_card_ranked
          WHERE rn = 1
        ), txn_name_stats AS (
          SELECT account_key, open_name, id_no, COUNT(1) AS hit_count, MAX(txn_ts) AS last_ts
          FROM txn_base
          WHERE account_key IS NOT NULL AND TRIM(account_key) <> ''
            AND (COALESCE(open_name, '') <> '' OR COALESCE(id_no, '') <> '')
          GROUP BY account_key, open_name, id_no
        ), txn_name_pick AS (
          SELECT account_key, open_name, id_no
          FROM (
            SELECT account_key, open_name, id_no, hit_count, last_ts,
                   ROW_NUMBER() OVER (
                     PARTITION BY account_key
                     ORDER BY CASE WHEN COALESCE(open_name, '') <> '' THEN 1 ELSE 0 END DESC,
                              hit_count DESC, (last_ts IS NOT NULL) DESC, last_ts DESC, open_name DESC, id_no DESC
                   ) AS rn
            FROM txn_name_stats
          ) ranked
          WHERE rn = 1
        )
        SELECT
          b.account_key AS account_key,
          COALESCE(NULLIF(MAX(b.acct_display), ''), b.account_key) AS acct_display,
          COALESCE(NULLIF(MAX(c.card_display), ''), NULLIF(MAX(b.card_display), ''), NULLIF(MAX(b.acct_display), ''), b.account_key) AS card_display,
          COALESCE(NULLIF(MAX(p.open_name), ''), '') AS open_name,
          COALESCE(NULLIF(MAX(p.id_no), ''), '') AS id_no,
          '' AS bank_name,
          '' AS branch_name,
          '' AS acct_type
        FROM txn_base b
        LEFT JOIN txn_card_pick c ON c.account_key = b.account_key
        LEFT JOIN txn_name_pick p ON p.account_key = b.account_key
        WHERE b.account_key IS NOT NULL AND TRIM(b.account_key) <> ''
        GROUP BY b.account_key
        ORDER BY b.account_key
        "
    ))?;
    Ok(())
}
