use anyhow::{bail, Result};
use duckdb::Connection;
use std::time::Instant;

use crate::sql_update::update_count;
use crate::{
    create_acc_scope, elapsed_ms, open_txn_connection, scalar_i64, sql_literal, table_exists,
    valid_expr, Args,
};

#[derive(Debug)]
pub(crate) struct AccountInfoSummary {
    pub(crate) account_fill_count: i64,
    pub(crate) account_fill_total: i64,
    pub(crate) timings: AccountInfoTimings,
}

#[derive(Clone, Copy, Debug, Default)]
pub(crate) struct AccountInfoTimings {
    pub(crate) pending_ms: i64,
    pub(crate) key_maps_ms: i64,
    pub(crate) primary_match_ms: i64,
    pub(crate) projection_ms: i64,
    pub(crate) txn_unique_map_ms: i64,
    pub(crate) txn_unique_match_ms: i64,
    pub(crate) counterparty_map_ms: i64,
    pub(crate) counterparty_match_ms: i64,
    pub(crate) alpha_card_match_ms: i64,
    pub(crate) acct_to_card_map_ms: i64,
    pub(crate) acct_to_card_match_ms: i64,
    pub(crate) fill_total_ms: i64,
}

pub(crate) fn run_account_info(args: &Args) -> Result<AccountInfoSummary> {
    let conn = open_txn_connection(args)?;
    create_acc_scope(&conn, &args.case_id, &args.acc_file_ids)?;
    run_account_info_on_conn(&conn, args)
}

pub(crate) fn run_account_info_on_conn(
    conn: &Connection,
    args: &Args,
) -> Result<AccountInfoSummary> {
    let mut timings = AccountInfoTimings::default();
    let scope_rows = scalar_i64(conn, "SELECT COUNT(1) FROM txn_scope")?;
    if scope_rows <= 0 {
        bail!("no transaction rows in cleaning scope");
    }
    if !table_exists(conn, "fc_account_norm")? {
        return Ok(AccountInfoSummary {
            account_fill_count: 0,
            account_fill_total: 0,
            timings,
        });
    }

    let p_card = "p.card_key";
    let p_acct = "p.acct_key";

    let started = Instant::now();
    create_account_info_pending_ids(conn)?;
    timings.pending_ms = elapsed_ms(started);
    if !has_account_info_pending_ids(conn)? {
        let (account_fill_total, fill_total_ms) = timed_account_info_fill_total(conn, args)?;
        timings.fill_total_ms += fill_total_ms;
        return Ok(AccountInfoSummary {
            account_fill_count: 0,
            account_fill_total,
            timings,
        });
    }

    let started = Instant::now();
    create_account_info_key_maps(conn)?;
    timings.key_maps_ms = elapsed_ms(started);
    let mut updated = 0;
    let started = Instant::now();
    updated += run_account_info_primary_matches(conn, p_card, p_acct)?;
    timings.primary_match_ms = elapsed_ms(started);

    if !has_account_info_pending_ids(conn)? {
        let (account_fill_total, fill_total_ms) = timed_account_info_fill_total(conn, args)?;
        timings.fill_total_ms += fill_total_ms;
        return Ok(AccountInfoSummary {
            account_fill_count: updated,
            account_fill_total,
            timings,
        });
    }

    let valid_card = valid_expr(p_card);
    let started = Instant::now();
    create_account_info_txn_projection(conn)?;
    timings.projection_ms = elapsed_ms(started);

    let started = Instant::now();
    create_account_info_txn_card_map(conn)?;
    timings.txn_unique_map_ms = elapsed_ms(started);
    let started = Instant::now();
    updated += run_account_info_key_match(conn, "account_info_txn_card_map", p_card, &valid_card)?;
    timings.txn_unique_match_ms = elapsed_ms(started);

    if !has_account_info_pending_ids(conn)? {
        let (account_fill_total, fill_total_ms) = timed_account_info_fill_total(conn, args)?;
        timings.fill_total_ms += fill_total_ms;
        return Ok(AccountInfoSummary {
            account_fill_count: updated,
            account_fill_total,
            timings,
        });
    }

    let started = Instant::now();
    create_account_info_counterparty_name_map(conn)?;
    timings.counterparty_map_ms = elapsed_ms(started);
    let started = Instant::now();
    updated += run_account_info_key_match(
        conn,
        "account_info_counterparty_name_map",
        p_card,
        &format!("{valid_card} AND COALESCE(TRIM(t.account_open_name),'')=''"),
    )?;
    timings.counterparty_match_ms = elapsed_ms(started);

    if !has_account_info_pending_ids(conn)? {
        let (account_fill_total, fill_total_ms) = timed_account_info_fill_total(conn, args)?;
        timings.fill_total_ms += fill_total_ms;
        return Ok(AccountInfoSummary {
            account_fill_count: updated,
            account_fill_total,
            timings,
        });
    }

    let t_card_digits = format!("NULLIF(regexp_extract({p_card}, '^[0-9]+', 0), '')");
    let has_alpha = format!("regexp_matches(COALESCE({p_card}, ''), '.*[A-Za-z].*')");
    let started = Instant::now();
    updated += run_account_info_key_match(
        conn,
        "account_info_card_map",
        &t_card_digits,
        &format!("{has_alpha} AND {t_card_digits} IS NOT NULL"),
    )?;
    timings.alpha_card_match_ms = elapsed_ms(started);

    let started = Instant::now();
    create_account_info_acct_to_card_map(conn)?;
    timings.acct_to_card_map_ms = elapsed_ms(started);
    let started = Instant::now();
    updated += run_account_info_key_match(
        conn,
        "account_info_acct_to_card_map",
        &format!("TRIM({p_card})"),
        &valid_card,
    )?;
    timings.acct_to_card_match_ms = elapsed_ms(started);

    let (account_fill_total, fill_total_ms) = timed_account_info_fill_total(conn, args)?;
    timings.fill_total_ms += fill_total_ms;

    Ok(AccountInfoSummary {
        account_fill_count: updated,
        account_fill_total,
        timings,
    })
}

fn has_account_info_pending_ids(conn: &Connection) -> Result<bool> {
    Ok(scalar_i64(conn, "SELECT 1 FROM account_info_pending_ids LIMIT 1")? > 0)
}

fn account_info_fill_total(conn: &Connection, args: &Args) -> Result<i64> {
    scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id={} AND clean_account_filled=1",
            sql_literal(&args.case_id)
        ),
    )
}

fn timed_account_info_fill_total(conn: &Connection, args: &Args) -> Result<(i64, i64)> {
    let started = Instant::now();
    let total = account_info_fill_total(conn, args)?;
    Ok((total, elapsed_ms(started)))
}

fn create_account_info_key_maps(conn: &Connection) -> Result<()> {
    conn.execute_batch(
        "DROP TABLE IF EXISTS account_info_account_projection;
         DROP TABLE IF EXISTS account_info_card_map;
         DROP TABLE IF EXISTS account_info_acct_map;",
    )?;
    create_account_info_account_projection(conn)?;
    create_account_info_key_map(conn, "account_info_card_map", "card_key")?;
    create_account_info_key_map(conn, "account_info_acct_map", "acct_key")
}

fn create_account_info_key_map(conn: &Connection, table_name: &str, key_expr: &str) -> Result<()> {
    let valid_key = valid_expr(key_expr);
    let sql = format!(
        r#"
        CREATE TEMP TABLE {table_name} AS
        SELECT {key_expr} AS match_key,
               MIN(name_val) AS name_val,
               MIN(id_val) AS id_val,
               COUNT(DISTINCT name_val) AS name_cnt,
               COUNT(DISTINCT id_val) AS id_cnt
          FROM account_info_account_projection
         WHERE {valid_key}
           AND (name_val IS NOT NULL OR id_val IS NOT NULL)
         GROUP BY {key_expr}
        HAVING COUNT(DISTINCT name_val)=1 OR COUNT(DISTINCT id_val)=1
        "#,
        key_expr = key_expr,
        table_name = table_name,
        valid_key = valid_key,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn create_account_info_account_projection(conn: &Connection) -> Result<()> {
    conn.execute_batch(
        r#"
        CREATE TEMP TABLE account_info_account_projection AS
        SELECT COALESCE(NULLIF(clean_card_no,''), card_no) AS card_key,
               COALESCE(NULLIF(clean_acct_no,''), acct_no) AS acct_key,
               NULLIF(TRIM(account_open_name),'') AS name_val,
               NULLIF(TRIM(opener_id_no),'') AS id_val
          FROM fc_account_norm
         WHERE id IN (SELECT id FROM acc_scope)
        "#,
    )?;
    Ok(())
}

fn create_account_info_pending_ids(conn: &Connection) -> Result<()> {
    conn.execute_batch(
        r#"
        DROP TABLE IF EXISTS account_info_pending_ids;
        CREATE TEMP TABLE account_info_pending_ids AS
        SELECT id,
               COALESCE(NULLIF(clean_card_no,''), card_no) AS card_key,
               COALESCE(NULLIF(clean_acct_no,''), acct_no) AS acct_key
          FROM fc_transaction_norm
         WHERE id IN (SELECT id FROM txn_scope)
           AND (
             COALESCE(TRIM(account_open_name),'')=''
             OR COALESCE(TRIM(opener_id_no),'')=''
           )
        "#,
    )?;
    Ok(())
}

fn prune_account_info_pending_ids(conn: &Connection) -> Result<()> {
    conn.execute_batch(
        r#"
        DELETE FROM account_info_pending_ids AS p
        USING fc_transaction_norm AS t
        WHERE t.id=p.id
          AND COALESCE(TRIM(t.account_open_name),'')<>''
          AND COALESCE(TRIM(t.opener_id_no),'')<>''
        "#,
    )?;
    Ok(())
}

fn create_account_info_txn_projection(conn: &Connection) -> Result<()> {
    let valid_card = valid_expr("card_key");
    let txn_card_key = "COALESCE(NULLIF(clean_card_no,''), card_no)";
    let txn_acct_key = "TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no))";
    let txn_counterparty_acct_key =
        "TRIM(COALESCE(NULLIF(counterparty_acct_norm,''), counterparty_acct))";
    let sql = format!(
        r#"
        DROP TABLE IF EXISTS account_info_txn_projection;
        CREATE TEMP TABLE account_info_txn_projection AS
        WITH pending_keys AS MATERIALIZED (
            SELECT DISTINCT card_key AS raw_key,
                   TRIM(card_key) AS trimmed_key
              FROM account_info_pending_ids
             WHERE {valid_card}
        )
        SELECT id,
               {txn_card_key} AS card_key,
               {txn_acct_key} AS acct_key,
               {txn_counterparty_acct_key} AS counterparty_acct_key,
               NULLIF(TRIM(counterparty_name),'') AS counterparty_name_val,
               NULLIF(TRIM(account_open_name),'') AS name_val,
               NULLIF(TRIM(opener_id_no),'') AS id_val
          FROM fc_transaction_norm
         WHERE id IN (SELECT id FROM txn_scope)
           AND (
             {txn_card_key} IN (SELECT raw_key FROM pending_keys)
             OR {txn_counterparty_acct_key} IN (SELECT raw_key FROM pending_keys)
             OR {txn_acct_key} IN (SELECT trimmed_key FROM pending_keys)
           )
        "#,
        valid_card = valid_card,
        txn_acct_key = txn_acct_key,
        txn_card_key = txn_card_key,
        txn_counterparty_acct_key = txn_counterparty_acct_key,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn create_account_info_txn_card_map(conn: &Connection) -> Result<()> {
    let valid_card = valid_expr("card_key");
    let sql = format!(
        r#"
        DROP TABLE IF EXISTS account_info_txn_card_map;
        CREATE TEMP TABLE account_info_txn_card_map AS
        WITH pending_keys AS (
            SELECT DISTINCT card_key AS match_key
              FROM account_info_pending_ids
             WHERE {valid_card}
        )
        SELECT card_key AS match_key,
               MIN(name_val) AS name_val,
               MIN(id_val) AS id_val,
               COUNT(DISTINCT name_val) AS name_cnt,
               COUNT(DISTINCT id_val) AS id_cnt
          FROM account_info_txn_projection
         WHERE {valid_card}
           AND card_key IN (SELECT match_key FROM pending_keys)
           AND (name_val IS NOT NULL OR id_val IS NOT NULL)
         GROUP BY card_key
        HAVING COUNT(DISTINCT name_val)=1 OR COUNT(DISTINCT id_val)=1
        "#,
        valid_card = valid_card,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn create_account_info_counterparty_name_map(conn: &Connection) -> Result<()> {
    let valid_card = valid_expr("card_key");
    let sql = format!(
        r#"
        DROP TABLE IF EXISTS account_info_counterparty_name_map;
        CREATE TEMP TABLE account_info_counterparty_name_map AS
        WITH pending_keys AS (
            SELECT DISTINCT card_key AS match_key
              FROM account_info_pending_ids
             WHERE {valid_card}
        ),
        name_stats AS (
            SELECT counterparty_acct_key AS match_key,
                   counterparty_name_val AS name_val,
                   COUNT(1) AS cnt
              FROM account_info_txn_projection
             WHERE COALESCE(counterparty_acct_key,'') NOT IN ('','-','—','_')
               AND counterparty_acct_key IN (SELECT match_key FROM pending_keys)
               AND counterparty_name_val IS NOT NULL
               AND counterparty_name_val NOT IN ('-','—','_')
             GROUP BY counterparty_acct_key, counterparty_name_val
        ),
        max_counts AS (
            SELECT match_key,
                   MAX(cnt) AS max_cnt
              FROM name_stats
             GROUP BY match_key
        )
        SELECT match_key,
               MIN(name_val) AS name_val,
               CAST(NULL AS VARCHAR) AS id_val,
               1 AS name_cnt,
               0 AS id_cnt
          FROM name_stats AS s
          JOIN max_counts AS m USING (match_key)
         WHERE s.cnt=m.max_cnt
         GROUP BY match_key
        "#,
        valid_card = valid_card,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn create_account_info_acct_to_card_map(conn: &Connection) -> Result<()> {
    let valid_card = valid_expr("card_key");
    let sql = format!(
        r#"
        DROP TABLE IF EXISTS account_info_acct_to_card_map;
        CREATE TEMP TABLE account_info_acct_to_card_map AS
        WITH pending_keys AS MATERIALIZED (
            SELECT DISTINCT TRIM(card_key) AS match_key
              FROM account_info_pending_ids
             WHERE {valid_card}
        ),
        name_stats AS (
            SELECT acct_key AS match_key,
                   name_val,
                   COUNT(1) AS cnt
              FROM account_info_txn_projection
             WHERE acct_key NOT IN ('','-','—','_')
               AND acct_key IN (SELECT match_key FROM pending_keys)
               AND name_val IS NOT NULL
               AND name_val NOT IN ('-','—','_')
             GROUP BY acct_key, name_val
        ),
        name_max_counts AS (
            SELECT match_key,
                   MAX(cnt) AS max_cnt
              FROM name_stats
             GROUP BY match_key
        ),
        picked_names AS (
            SELECT s.match_key,
                   MIN(s.name_val) AS name_val
              FROM name_stats AS s
              JOIN name_max_counts AS m USING (match_key)
             WHERE s.cnt=m.max_cnt
             GROUP BY s.match_key
        ),
        id_stats AS (
            SELECT acct_key AS match_key,
                   id_val,
                   COUNT(1) AS cnt
              FROM account_info_txn_projection
             WHERE acct_key NOT IN ('','-','—','_')
               AND acct_key IN (SELECT match_key FROM pending_keys)
               AND id_val IS NOT NULL
               AND id_val NOT IN ('-','—','_')
             GROUP BY acct_key, id_val
        ),
        id_max_counts AS (
            SELECT match_key,
                   MAX(cnt) AS max_cnt
              FROM id_stats
             GROUP BY match_key
        ),
        picked_ids AS (
            SELECT s.match_key,
                   MIN(s.id_val) AS id_val
              FROM id_stats AS s
              JOIN id_max_counts AS m USING (match_key)
             WHERE s.cnt=m.max_cnt
             GROUP BY s.match_key
        ),
        keys AS (
            SELECT match_key FROM picked_names
            UNION
            SELECT match_key FROM picked_ids
        )
        SELECT k.match_key,
               n.name_val,
               i.id_val,
               CASE WHEN n.name_val IS NULL THEN 0 ELSE 1 END AS name_cnt,
               CASE WHEN i.id_val IS NULL THEN 0 ELSE 1 END AS id_cnt
          FROM keys AS k
          LEFT JOIN picked_names AS n ON n.match_key=k.match_key
          LEFT JOIN picked_ids AS i ON i.match_key=k.match_key
        "#,
        valid_card = valid_card,
    );
    conn.execute_batch(&sql)?;
    Ok(())
}

fn run_account_info_key_match(
    conn: &Connection,
    map_table: &str,
    txn_key_expr: &str,
    extra_cond: &str,
) -> Result<i64> {
    if !has_account_info_pending_ids(conn)? {
        return Ok(0);
    }
    let needs_fill =
        "COALESCE(TRIM(t.account_open_name),'')='' OR COALESCE(TRIM(t.opener_id_no),'')=''";
    let sql = format!(
        r#"
        UPDATE fc_transaction_norm AS t SET
            account_open_name=CASE
              WHEN COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL
              THEN m.name_val ELSE t.account_open_name END,
            opener_id_no=CASE
              WHEN COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL
              THEN m.id_val ELSE t.opener_id_no END,
            clean_account_filled=CASE
              WHEN (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL)
                OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL)
              THEN 1 ELSE t.clean_account_filled END
        FROM account_info_pending_ids AS p, {map_table} AS m
        WHERE t.id=p.id
          AND ({needs_fill})
          AND ({extra_cond})
          AND {txn_key_expr}=m.match_key
          AND (
            (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL
              AND t.account_open_name IS DISTINCT FROM m.name_val)
            OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL
              AND t.opener_id_no IS DISTINCT FROM m.id_val)
            OR (t.clean_account_filled IS DISTINCT FROM 1 AND (
              (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL)
              OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL)
            ))
          )
        "#,
        extra_cond = extra_cond,
        map_table = map_table,
        needs_fill = needs_fill,
        txn_key_expr = txn_key_expr,
    );
    let updated = update_count(conn, &sql)?;
    if updated > 0 {
        prune_account_info_pending_ids(conn)?;
    }
    Ok(updated)
}

fn run_account_info_primary_matches(conn: &Connection, p_card: &str, p_acct: &str) -> Result<i64> {
    if !has_account_info_pending_ids(conn)? {
        return Ok(0);
    }
    let valid_card = valid_expr(p_card);
    let valid_acct = valid_expr(p_acct);
    let needs_fill =
        "COALESCE(TRIM(t.account_open_name),'')='' OR COALESCE(TRIM(t.opener_id_no),'')=''";
    let sql = format!(
        r#"
        UPDATE fc_transaction_norm AS t SET
            account_open_name=CASE
              WHEN COALESCE(TRIM(t.account_open_name),'')='' AND c.name_val IS NOT NULL
              THEN c.name_val ELSE t.account_open_name END,
            opener_id_no=CASE
              WHEN COALESCE(TRIM(t.opener_id_no),'')='' AND c.id_val IS NOT NULL
              THEN c.id_val ELSE t.opener_id_no END,
            clean_account_filled=CASE
              WHEN (COALESCE(TRIM(t.account_open_name),'')='' AND c.name_val IS NOT NULL)
                OR (COALESCE(TRIM(t.opener_id_no),'')='' AND c.id_val IS NOT NULL)
              THEN 1 ELSE t.clean_account_filled END
        FROM (
            WITH candidates AS (
                SELECT p.id, m.name_val, m.id_val, m.name_cnt, m.id_cnt, 1 AS priority
                  FROM account_info_pending_ids AS p
                  JOIN account_info_card_map AS m ON {p_card}=m.match_key
                 WHERE {valid_card}
                UNION ALL
                SELECT p.id, m.name_val, m.id_val, m.name_cnt, m.id_cnt, 2 AS priority
                  FROM account_info_pending_ids AS p
                  JOIN account_info_acct_map AS m ON {p_acct}=m.match_key
                 WHERE {valid_acct}
                UNION ALL
                SELECT p.id, m.name_val, m.id_val, m.name_cnt, m.id_cnt, 3 AS priority
                  FROM account_info_pending_ids AS p
                  JOIN account_info_acct_map AS m ON {p_card}=m.match_key
                 WHERE {valid_card}
            ),
            picked AS (
                SELECT id,
                       COALESCE(
                           MIN(CASE WHEN priority=1 AND name_cnt=1 AND name_val IS NOT NULL THEN name_val END),
                           MIN(CASE WHEN priority=2 AND name_cnt=1 AND name_val IS NOT NULL THEN name_val END),
                           MIN(CASE WHEN priority=3 AND name_cnt=1 AND name_val IS NOT NULL THEN name_val END)
                       ) AS name_val,
                       COALESCE(
                           MIN(CASE WHEN priority=1 AND id_cnt=1 AND id_val IS NOT NULL THEN id_val END),
                           MIN(CASE WHEN priority=2 AND id_cnt=1 AND id_val IS NOT NULL THEN id_val END),
                           MIN(CASE WHEN priority=3 AND id_cnt=1 AND id_val IS NOT NULL THEN id_val END)
                       ) AS id_val
                  FROM candidates
                 GROUP BY id
            )
            SELECT id,
                   name_val,
                   id_val
              FROM picked
             WHERE name_val IS NOT NULL OR id_val IS NOT NULL
        ) AS c
        WHERE t.id=c.id
          AND ({needs_fill})
          AND (
            (COALESCE(TRIM(t.account_open_name),'')='' AND c.name_val IS NOT NULL
              AND t.account_open_name IS DISTINCT FROM c.name_val)
            OR (COALESCE(TRIM(t.opener_id_no),'')='' AND c.id_val IS NOT NULL
              AND t.opener_id_no IS DISTINCT FROM c.id_val)
            OR (t.clean_account_filled IS DISTINCT FROM 1 AND (
              (COALESCE(TRIM(t.account_open_name),'')='' AND c.name_val IS NOT NULL)
              OR (COALESCE(TRIM(t.opener_id_no),'')='' AND c.id_val IS NOT NULL)
            ))
          )
        "#,
        needs_fill = needs_fill,
        p_acct = p_acct,
        p_card = p_card,
        valid_acct = valid_acct,
        valid_card = valid_card,
    );
    let updated = update_count(conn, &sql)?;
    if updated > 0 {
        prune_account_info_pending_ids(conn)?;
    }
    Ok(updated)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn projection_maps_pick_most_frequent_values_with_stable_ties() -> Result<()> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TEMP TABLE account_info_pending_ids(
                id BIGINT,
                card_key TEXT,
                acct_key TEXT
            );
            INSERT INTO account_info_pending_ids VALUES
                (1, 'card-a', ''),
                (2, 'acct-a', '');

            CREATE TEMP TABLE account_info_txn_projection(
                id BIGINT,
                card_key TEXT,
                acct_key TEXT,
                counterparty_acct_key TEXT,
                counterparty_name_val TEXT,
                name_val TEXT,
                id_val TEXT
            );
            INSERT INTO account_info_txn_projection VALUES
                (1, '', '', 'card-a', 'Bob', NULL, NULL),
                (2, '', '', 'card-a', 'Bob', NULL, NULL),
                (3, '', '', 'card-a', 'Alice', NULL, NULL),
                (4, '', '', 'card-a', 'Alice', NULL, NULL),
                (5, '', 'acct-a', '', NULL, 'Zed', 'ID2'),
                (6, '', 'acct-a', '', NULL, 'Amy', 'ID2'),
                (7, '', 'acct-a', '', NULL, NULL, 'ID1');
            "#,
        )?;

        create_account_info_counterparty_name_map(&conn)?;
        create_account_info_acct_to_card_map(&conn)?;

        let counterparty_name: String = conn.query_row(
            "SELECT name_val FROM account_info_counterparty_name_map WHERE match_key='card-a'",
            [],
            |row| row.get(0),
        )?;
        let acct_to_card = conn.query_row(
            "SELECT name_val, id_val FROM account_info_acct_to_card_map WHERE match_key='acct-a'",
            [],
            |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
        )?;

        assert_eq!(counterparty_name, "Alice");
        assert_eq!(acct_to_card, ("Amy".to_string(), "ID2".to_string()));
        Ok(())
    }

    #[test]
    fn primary_match_picks_name_and_id_by_independent_priority() -> Result<()> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(
            r#"
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                account_open_name TEXT,
                opener_id_no TEXT,
                clean_account_filled INTEGER
            );
            INSERT INTO fc_transaction_norm VALUES
                (1, '', '', 0),
                (2, '', '', 0);

            CREATE TEMP TABLE account_info_pending_ids(
                id BIGINT,
                card_key TEXT,
                acct_key TEXT
            );
            INSERT INTO account_info_pending_ids VALUES
                (1, 'card-1', 'acct-1'),
                (2, 'card-2', 'acct-2');

            CREATE TEMP TABLE account_info_card_map(
                match_key TEXT,
                name_val TEXT,
                id_val TEXT,
                name_cnt BIGINT,
                id_cnt BIGINT
            );
            INSERT INTO account_info_card_map VALUES
                ('card-1', 'card-name-1', NULL, 1, 0),
                ('card-2', NULL, 'card-id-2', 0, 1);

            CREATE TEMP TABLE account_info_acct_map(
                match_key TEXT,
                name_val TEXT,
                id_val TEXT,
                name_cnt BIGINT,
                id_cnt BIGINT
            );
            INSERT INTO account_info_acct_map VALUES
                ('acct-1', 'acct-name-1', 'acct-id-1', 1, 1),
                ('acct-2', 'acct-name-2', 'acct-id-2', 1, 1),
                ('card-1', 'card-as-acct-name-1', 'card-as-acct-id-1', 1, 1);
            "#,
        )?;

        let updated = run_account_info_primary_matches(&conn, "p.card_key", "p.acct_key")?;

        assert_eq!(updated, 2);
        let rows = conn
            .prepare(
                r#"
                SELECT id, account_open_name, opener_id_no, clean_account_filled
                  FROM fc_transaction_norm
                 ORDER BY id
                "#,
            )?
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, i64>(3)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()?;

        assert_eq!(
            rows,
            vec![
                (1, "card-name-1".to_string(), "acct-id-1".to_string(), 1),
                (2, "acct-name-2".to_string(), "card-id-2".to_string(), 1),
            ]
        );
        Ok(())
    }
}
