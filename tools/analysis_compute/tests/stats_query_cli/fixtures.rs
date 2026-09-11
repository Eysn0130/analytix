use anyhow::{bail, Context, Result};
use chrono::{DateTime, NaiveDate, NaiveTime, Timelike, Utc};
use duckdb::types::{TimeUnit, ValueRef};
use duckdb::{params, Connection};
use serde::Serialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::path::Path;

pub(crate) const CASE_ID: &str = "case-stats-query-golden";
const SOURCE_REVISION: i64 = 7;
const RAW_SHA256: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const RAW_ARTIFACT_MANIFEST_SHA256: &str =
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

pub(crate) fn seed_stats_query_tables(path: &Path) -> Result<()> {
    let conn = Connection::open(path).context("open golden DuckDB for seed")?;
    conn.execute_batch(
        "
        CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT);
        INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7);

        CREATE TABLE fc_transaction_norm(
          id BIGINT,
          row_no BIGINT,
          case_id TEXT,
          txn_ts TIMESTAMP,
          clean_amount VARCHAR,
          file_id VARCHAR,
          clean_invalid INTEGER,
          clean_failed INTEGER,
          clean_reversal INTEGER
        );
        INSERT INTO fc_transaction_norm VALUES
          (1, 1, 'case-stats-query-golden', TIMESTAMP '2026-04-01 09:00:00',
           '10000.00', 'file-1', 0, 0, 0),
          (2, 2, 'case-stats-query-golden', TIMESTAMP '2026-04-01 10:00:00',
           '3000.00', 'file-1', 0, 0, 0),
          (3, 3, 'case-stats-query-golden', TIMESTAMP '2026-04-02 11:00:00',
           '500.00', 'file-1', 0, 0, 0);

        CREATE TABLE import_file_log(
          file_id TEXT,
          case_id TEXT,
          kind TEXT,
          sha256 TEXT,
          rows_imported_norm BIGINT,
          status TEXT,
          cleaned_status TEXT
        );
        INSERT INTO import_file_log VALUES
          ('file-1', 'case-stats-query-golden', 'fc_transaction',
           'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
           3, '已完成', 'done');

        CREATE TABLE analysis_txn_daily_agg(
          txn_day DATE,
          acct_key TEXT,
          cp_key TEXT,
          cp_display TEXT,
          cp_raw TEXT,
          cp_placeholder_kind TEXT,
          cp_name TEXT,
          cp_name_pick TEXT,
          cp_name_pick_cnt BIGINT,
          stats_name_key TEXT,
          dc_val TEXT,
          txn_count BIGINT,
          amount_source_present_count BIGINT,
          amount_valid_count BIGINT,
          amount_missing_count BIGINT,
          amount_parse_failed_count BIGINT,
          amt_sum DOUBLE,
          first_ts TIMESTAMP,
          last_ts TIMESTAMP,
          open_name TEXT,
          counterparty_bank TEXT,
          location TEXT
        );
        INSERT INTO analysis_txn_daily_agg VALUES
          (DATE '2026-04-01', 'CARD-001', 'CP-001', 'CP-001', 'CP-001', NULL,
           '对手甲', '对手甲', 3, '对手甲', '进', 1, 1, 1, 0, 0, 10000.0,
           TIMESTAMP '2026-04-01 09:00:00', TIMESTAMP '2026-04-01 09:00:00',
           '张三', '测试银行', '贵阳'),
          (DATE '2026-04-01', 'CARD-001', 'CP-002', 'CP-002', 'CP-002', NULL,
           '对手乙', '对手乙', 3, '对手乙', '出', 1, 1, 1, 0, 0, 3000.0,
           TIMESTAMP '2026-04-01 10:00:00', TIMESTAMP '2026-04-01 10:00:00',
           '张三', '测试银行', '贵阳'),
          (DATE '2026-04-02', 'CARD-002', NULL, NULL, '', 'empty', NULL, NULL, 0,
           '__cp_placeholder__::empty', '进', 1, 1, 1, 0, 0, 500.0,
           TIMESTAMP '2026-04-02 11:00:00', TIMESTAMP '2026-04-02 11:00:00',
           '李四', '', '');

        CREATE TABLE analysis_txn_detail_idx(
          id BIGINT,
          txn_day DATE,
          acct_key TEXT,
          cp_key TEXT,
          cp_raw TEXT,
          cp_placeholder_kind TEXT,
          cp_name TEXT,
          cp_name_pick TEXT,
          cp_name_pick_cnt BIGINT,
          stats_name_key TEXT,
          dc_val TEXT,
          txn_ts TIMESTAMP,
          txn_time TEXT,
          amount DOUBLE,
          amount_source_present BIGINT,
          amount_parse_failed BIGINT,
          balance DOUBLE,
          card_no TEXT,
          acct_no TEXT,
          account_open_name TEXT,
          opener_id_no TEXT,
          counterparty_acct TEXT,
          cash_flag TEXT,
          counterparty_name TEXT,
          counterparty_id_no TEXT,
          counterparty_bank TEXT,
          summary TEXT,
          currency TEXT,
          branch_name TEXT,
          branch_code TEXT,
          location TEXT,
          is_success TEXT,
          voucher_no TEXT,
          terminal_no TEXT,
          ip_addr TEXT,
          mac_addr TEXT,
          counterparty_balance DOUBLE,
          txn_id TEXT,
          file_id TEXT,
          log_id TEXT,
          voucher_type TEXT,
          voucher_id TEXT,
          teller_no TEXT,
          merchant_name TEXT,
          merchant_no TEXT,
          remark TEXT,
          txn_type TEXT,
          query_feedback_reason TEXT
        );
        INSERT INTO analysis_txn_detail_idx VALUES
          (1, DATE '2026-04-01', 'CARD-001', 'CP-001', 'CP-001', NULL, '对手甲',
           '对手甲', 3, '对手甲', '进', TIMESTAMP '2026-04-01 09:00:00',
           '2026-04-01 09:00:00', 10000.0, 1, 0, 10000.0, 'CARD-001', 'A-001',
           '张三', 'ID-A', 'CP-001', '否', '对手甲', '', '测试银行', '工资入账',
           'CNY', '一号支行', 'BR-001', '贵阳', '成功', '', 'TERM-01', '', '',
           NULL, 'txn-rust-in', 'file-1', '', '', '', 'T-01', '商户甲', 'M-001',
           '', '转账', ''),
          (2, DATE '2026-04-01', 'CARD-001', 'CP-002', 'CP-002', NULL, '对手乙',
           '对手乙', 3, '对手乙', '出', TIMESTAMP '2026-04-01 10:00:00',
           '2026-04-01 10:00:00', 3000.0, 1, 0, 7000.0, 'CARD-001', 'A-001',
           '张三', 'ID-A', 'CP-002', '否', '对手乙', '', '测试银行', '转出', 'CNY',
           '一号支行', 'BR-001', '贵阳', '成功', '', 'TERM-01', '', '', NULL,
           'txn-rust-out', 'file-1', '', '', '', 'T-01', '', '', '', '转账', ''),
          (3, DATE '2026-04-02', 'CARD-002', NULL, '', 'empty', NULL, NULL, 0,
           '__cp_placeholder__::empty', '进', TIMESTAMP '2026-04-02 11:00:00',
           '2026-04-02 11:00:00', 500.0, 1, 0, 500.0, 'CARD-002', 'A-002',
           '李四', 'ID-B', '', '否', '', '', '', '未知对手入账', 'CNY', '', '', '',
           '成功', '', '', '', '', NULL, 'txn-placeholder', 'file-1', '', '', '', '',
           '', '', '', '转账', '');

        CREATE TABLE analysis_txn_keyword_idx(
          txn_row_id BIGINT,
          stable_txn_id TEXT,
          kind TEXT,
          token TEXT,
          token_order BIGINT
        );
        INSERT INTO analysis_txn_keyword_idx VALUES
          (1, 'txn-rust-in', 'summary', '工资入账', 0),
          (2, 'txn-rust-out', 'summary', '转出', 0),
          (3, 'txn-placeholder', 'summary', '未知对手入账', 0);
        CREATE INDEX idx_analysis_txn_keyword_idx_kind_token_id
          ON analysis_txn_keyword_idx(kind, token, txn_row_id);

        CREATE TABLE analysis_account_dim(
          account_key TEXT,
          acct_display TEXT,
          card_display TEXT,
          open_name TEXT,
          id_no TEXT,
          bank_name TEXT,
          branch_name TEXT,
          acct_type TEXT
        );
        INSERT INTO analysis_account_dim VALUES
          ('CARD-001', 'A-001', 'CARD-001', '张三', 'ID-A', '测试银行', '一号支行', '个人储蓄账户'),
          ('CARD-002', 'A-002', 'CARD-002', '李四', 'ID-B', '测试银行', '', '个人储蓄账户');

        CREATE TABLE analysis_materialization_meta(
          agg_name TEXT PRIMARY KEY,
          agg_version INTEGER NOT NULL,
          case_id TEXT,
          identity_schema_version INTEGER,
          source_revision BIGINT NOT NULL,
          source_row_count BIGINT,
          source_max_txn_ts TIMESTAMP,
          source_max_id BIGINT,
          source_signature TEXT,
          source_parameter_signature TEXT,
          result_signature TEXT,
          built_at TIMESTAMP,
          row_count BIGINT DEFAULT 0
        );
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(path)
}

pub(crate) fn insert_replacement_card_duplicate(path: &Path) -> Result<()> {
    let conn = Connection::open(path).context("open golden DuckDB for replacement-card seed")?;
    conn.execute_batch(
        "
        INSERT INTO analysis_account_dim VALUES
          ('CARD-003', 'A-003', 'CARD-003', '张三', 'ID-A',
           '测试银行账户信息.csv', '一号支行', '借记卡');
        INSERT INTO analysis_txn_detail_idx
        SELECT * REPLACE (
          101 AS id,
          'CARD-003' AS acct_key,
          'CARD-003' AS card_no,
          'A-003' AS acct_no
        )
        FROM analysis_txn_detail_idx
        WHERE id = 1;
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(path)
}

pub(crate) fn refresh_stats_query_materialization_meta(path: &Path) -> Result<()> {
    let conn = Connection::open(path).context("open golden DuckDB to refresh authority meta")?;
    sync_test_raw_lineage(&conn)?;
    let source = source_identity(&conn)?;
    let result_signature = result_signature(&conn)?;
    let row_count = scalar_i64(&conn, "SELECT COUNT(1) FROM analysis_txn_daily_agg")?;
    conn.execute(
        "DELETE FROM analysis_materialization_meta WHERE starts_with(agg_name, 'txn_daily_')",
        [],
    )?;
    conn.execute(
        "INSERT INTO analysis_materialization_meta(
           agg_name, agg_version, case_id, identity_schema_version,
           source_revision, source_row_count, source_max_txn_ts, source_max_id,
           source_signature, result_signature, built_at, row_count
         ) VALUES (?, 12, ?, 2, ?, ?, CAST(? AS TIMESTAMP), ?, ?, ?, now(), ?)",
        params![
            source.agg_name,
            CASE_ID,
            SOURCE_REVISION,
            source.row_count,
            source.max_txn_ts,
            source.max_id,
            source.source_signature,
            result_signature,
            row_count,
        ],
    )?;
    let stored_name = conn.query_row(
        "SELECT agg_name FROM analysis_materialization_meta",
        [],
        |row| row.get::<_, String>(0),
    )?;
    if !stored_name.starts_with("txn_daily_") {
        bail!("test authority meta name is invalid: {stored_name}");
    }
    let stored_prefix_count = scalar_i64(
        &conn,
        "SELECT COUNT(1) FROM analysis_materialization_meta WHERE starts_with(agg_name, 'txn_daily_')",
    )?;
    if stored_prefix_count != 1 {
        bail!("test authority meta prefix query failed: {stored_name}:{stored_prefix_count}");
    }
    refresh_test_snapshot_manifests(&conn, &source, &result_signature, row_count)
}

fn sync_test_raw_lineage(conn: &Connection) -> Result<()> {
    conn.execute(
        "INSERT INTO fc_transaction_norm(
           id, row_no, case_id, txn_ts, clean_amount, file_id,
           clean_invalid, clean_failed, clean_reversal
         )
         SELECT TRY_CAST(d.id AS BIGINT), TRY_CAST(d.id AS BIGINT), ?, d.txn_ts,
                CAST(d.amount AS VARCHAR), d.file_id, 0, 0, 0
           FROM analysis_txn_detail_idx d
          WHERE TRY_CAST(d.id AS BIGINT) IS NOT NULL
            AND NOT EXISTS (
              SELECT 1 FROM fc_transaction_norm r
               WHERE r.case_id=? AND TRY_CAST(r.id AS BIGINT)=TRY_CAST(d.id AS BIGINT)
            )",
        params![CASE_ID, CASE_ID],
    )?;
    let row_count = scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id='{}'",
            CASE_ID
        ),
    )?;
    conn.execute(
        "UPDATE import_file_log SET rows_imported_norm=? WHERE case_id=? AND kind='fc_transaction'",
        params![row_count, CASE_ID],
    )?;
    Ok(())
}

struct SourceIdentity {
    agg_name: String,
    source_signature: String,
    row_count: i64,
    max_txn_ts: String,
    max_id: i64,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct RawSourceIdentity {
    cleaned_status: String,
    file_id: String,
    rows_imported_norm: i64,
    sha256: String,
    status: String,
}

fn source_identity(conn: &Connection) -> Result<SourceIdentity> {
    let (row_count, numeric_count, distinct_count, max_txn_ts, max_id, rejected_count) =
        conn.query_row(
            "SELECT COUNT(1), COUNT(TRY_CAST(id AS BIGINT)),
                    COUNT(DISTINCT TRY_CAST(id AS BIGINT)),
                    COALESCE(CAST(MAX(txn_ts) AS VARCHAR), ''), COALESCE(MAX(id), 0),
                    SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1 THEN 1 ELSE 0 END)
               FROM fc_transaction_norm WHERE case_id=?",
            [CASE_ID],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, i64>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, i64>(4)?,
                    row.get::<_, Option<i64>>(5)?.unwrap_or(0),
                ))
            },
        )?;
    if row_count != numeric_count || row_count != distinct_count {
        bail!("test raw source lineage is ambiguous");
    }
    let normalized_source_sha256 = canonical_case_table_signature(
        conn,
        "fc_transaction_norm",
        CASE_ID,
        "analytix.txn-daily-normalized-source/v12",
        "TRY_CAST(r.id AS BIGINT)",
    )?;
    let raw_sources = vec![RawSourceIdentity {
        cleaned_status: "done".to_string(),
        file_id: "file-1".to_string(),
        rows_imported_norm: row_count,
        sha256: RAW_SHA256.to_string(),
        status: "已完成".to_string(),
    }];
    let canonical = serde_json::to_vec(&json!({
        "acceptedRowCount": row_count - rejected_count,
        "caseId": CASE_ID,
        "normalizedSourceSha256": normalized_source_sha256,
        "rawSources": raw_sources,
        "rejectedRowCount": rejected_count,
        "schemaVersion": 2,
        "sourceMaxId": max_id,
        "sourceMaxTxnTimestamp": max_txn_ts,
        "sourceRevision": SOURCE_REVISION,
        "sourceRowCount": row_count,
    }))?;
    let source_signature = format!("{:x}", Sha256::digest(canonical));
    Ok(SourceIdentity {
        agg_name: format!("txn_daily_snapshot:v12:{source_signature}"),
        source_signature,
        row_count,
        max_txn_ts,
        max_id,
    })
}

fn canonical_case_table_signature(
    conn: &Connection,
    table: &str,
    case_id: &str,
    domain: &str,
    order_by: &str,
) -> Result<String> {
    let columns = table_columns(conn, table)?;
    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, domain.as_bytes());
    hash_framed(&mut hasher, case_id.as_bytes());
    for column in columns {
        hash_framed(&mut hasher, column.as_bytes());
    }
    let mut statement = conn.prepare(&format!(
        "SELECT to_json(r) FROM {table} r WHERE r.case_id=? ORDER BY {order_by}, to_json(r)"
    ))?;
    let rows = statement.query_map([case_id], |row| row.get::<_, String>(0))?;
    for row in rows {
        hash_framed(&mut hasher, row?.as_bytes());
    }
    Ok(format!("{:x}", hasher.finalize()))
}

fn result_signature(conn: &Connection) -> Result<String> {
    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, b"analytix.txn-daily-result/v12");
    hash_framed(&mut hasher, CASE_ID.as_bytes());
    for (table, order_by) in [
        (
            "analysis_txn_daily_agg",
            "r.acct_key, r.txn_day, r.cp_key, r.dc_val",
        ),
        ("analysis_txn_detail_idx", "TRY_CAST(r.id AS BIGINT), r.id"),
        (
            "analysis_txn_keyword_idx",
            "TRY_CAST(r.txn_row_id AS BIGINT), r.txn_row_id, r.kind, r.token, r.token_order",
        ),
        ("analysis_account_dim", "r.account_key"),
    ] {
        hash_framed(&mut hasher, table.as_bytes());
        for column in table_columns(conn, table)? {
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

const PRODUCER_OPERATION_SCHEMA: &str = concat!(
    "{\"additionalProperties\":false,\"properties\":{",
    "\"caseId\":{\"type\":\"string\"},",
    "\"sourceMaxId\":{\"minimum\":0,\"type\":\"integer\"},",
    "\"sourceMaxTxnTs\":{\"type\":\"string\"},",
    "\"sourceRevision\":{\"minimum\":1,\"type\":\"integer\"},",
    "\"sourceRowCount\":{\"minimum\":0,\"type\":\"integer\"}},",
    "\"required\":[\"caseId\",\"sourceMaxId\",\"sourceMaxTxnTs\",",
    "\"sourceRevision\",\"sourceRowCount\"],\"type\":\"object\"}"
);

const PRODUCER_CONTENT_DIGEST_DOMAIN: &[u8] = b"AnalytixFundsProducerContentManifestV1\0";
const SNAPSHOT_DIGEST_DOMAIN: &[u8] = b"AnalytixDuckDBDatasetSnapshotManifestV2\0";
const SNAPSHOT_DUPLICATE_DOMAIN: &[u8] = b"AnalytixDuckDBDatasetSnapshotNoDuplicateRowsV2\0";
const SNAPSHOT_CLASSIFICATION_POLICY: &str = "clean-flags-v1-source-id-unique-no-dedup";

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct TestProducerManifestPayload {
    accepted_row_count: i64,
    account_content_sha256: String,
    account_row_count: i64,
    aggregate_content_sha256: String,
    aggregate_row_count: i64,
    canonical_encoder: String,
    case_id: String,
    contract: String,
    detail_content_sha256: String,
    detail_row_count: i64,
    duckdb_version: String,
    duplicate_row_count: i64,
    keyword_content_sha256: String,
    keyword_row_count: i64,
    normalized_content_sha256: String,
    normalized_row_count: i64,
    producer_component_id: String,
    producer_component_version: String,
    producer_operation: String,
    producer_operation_schema_hash: String,
    raw_manifest_sha256: String,
    rejected_row_count: i64,
    schema_version: i64,
    source_revision: i64,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct TestSnapshotManifestPayload {
    accepted_content_sha256: String,
    accepted_row_count: i64,
    aggregate_content_sha256: String,
    aggregate_row_count: i64,
    case_id: String,
    classification_policy: String,
    contract: String,
    duplicate_content_sha256: String,
    duplicate_row_count: i64,
    identity_schema_version: i64,
    materialization_name: String,
    materialization_version: i64,
    normalized_content_sha256: String,
    normalized_row_count: i64,
    producer_content_id: String,
    producer_manifest_sha256: String,
    raw_manifest_sha256: String,
    rejected_content_sha256: String,
    rejected_row_count: i64,
    result_signature: String,
    schema_version: i64,
    source_revision: i64,
    source_signature: String,
}

struct TestTableContentManifest {
    content_sha256: String,
    row_count: i64,
}

#[derive(Serialize)]
struct TestColumnManifest {
    name: String,
    #[serde(rename = "type")]
    data_type: String,
}

#[derive(Serialize)]
struct TestTableManifestHeader<'a> {
    columns: &'a [TestColumnManifest],
    #[serde(rename = "schemaVersion")]
    schema_version: i64,
    table: &'a str,
}

#[derive(Serialize)]
struct TestTableManifestFooter {
    #[serde(rename = "rowCount")]
    row_count: i64,
}

fn refresh_test_snapshot_manifests(
    conn: &Connection,
    source: &SourceIdentity,
    result_signature: &str,
    aggregate_row_count: i64,
) -> Result<()> {
    // These query fixtures intentionally construct derived rows directly. Keep
    // their persisted producer/snapshot manifests in sync; the production
    // session still recomputes every digest and rejects any mismatch.
    let normalized = test_table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!("\"case_id\"='{CASE_ID}'"),
    )?;
    let detail = test_table_content_manifest(conn, "analysis_txn_detail_idx", "")?;
    let aggregate = test_table_content_manifest(conn, "analysis_txn_daily_agg", "")?;
    let keyword = test_table_content_manifest(conn, "analysis_txn_keyword_idx", "")?;
    let account = test_table_content_manifest(conn, "analysis_account_dim", "")?;
    let rejected_row_count = conn.query_row(
        "SELECT SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1 \
                         THEN 1 ELSE 0 END) \
           FROM fc_transaction_norm WHERE case_id=?",
        [CASE_ID],
        |row| Ok(row.get::<_, Option<i64>>(0)?.unwrap_or(0)),
    )?;
    let accepted_row_count = source.row_count - rejected_row_count;
    if normalized.row_count != source.row_count
        || detail.row_count != accepted_row_count
        || aggregate.row_count != aggregate_row_count
    {
        bail!("test snapshot fixture row coverage is inconsistent");
    }

    let duckdb_version = conn.query_row("SELECT version()", [], |row| row.get::<_, String>(0))?;
    let producer = TestProducerManifestPayload {
        accepted_row_count,
        account_content_sha256: account.content_sha256,
        account_row_count: account.row_count,
        aggregate_content_sha256: aggregate.content_sha256,
        aggregate_row_count: aggregate.row_count,
        canonical_encoder: "analytix.duckdb-content-manifest/v1".to_string(),
        case_id: CASE_ID.to_string(),
        contract: "analytix.funds-producer-content-manifest/v1".to_string(),
        detail_content_sha256: detail.content_sha256,
        detail_row_count: detail.row_count,
        duckdb_version,
        duplicate_row_count: 0,
        keyword_content_sha256: keyword.content_sha256,
        keyword_row_count: keyword.row_count,
        normalized_content_sha256: normalized.content_sha256,
        normalized_row_count: normalized.row_count,
        producer_component_id: "analysis-compute".to_string(),
        producer_component_version: env!("CARGO_PKG_VERSION").to_string(),
        producer_operation: "materialize-txn-daily".to_string(),
        producer_operation_schema_hash: sha256_bytes(PRODUCER_OPERATION_SCHEMA.as_bytes()),
        raw_manifest_sha256: RAW_ARTIFACT_MANIFEST_SHA256.to_string(),
        rejected_row_count,
        schema_version: 1,
        source_revision: SOURCE_REVISION,
    };
    let producer_manifest_bytes = serde_json::to_vec(&producer)?;
    let producer_manifest_sha256 = sha256_bytes(&producer_manifest_bytes);
    let producer_content_digest =
        sha256_domain(PRODUCER_CONTENT_DIGEST_DOMAIN, &producer_manifest_bytes);
    let producer_content_id = format!("fpc1_{producer_content_digest}");
    conn.execute_batch(
        "DROP TABLE IF EXISTS analysis_funds_content_manifest_v1; \
         CREATE TABLE analysis_funds_content_manifest_v1( \
           schema_version INTEGER NOT NULL, \
           contract TEXT NOT NULL, \
           case_id TEXT NOT NULL, \
           source_revision BIGINT NOT NULL, \
           producer_component_id TEXT NOT NULL, \
           producer_component_version TEXT NOT NULL, \
           producer_operation TEXT NOT NULL, \
           producer_operation_schema_hash TEXT NOT NULL, \
           duckdb_version TEXT NOT NULL, \
           canonical_encoder TEXT NOT NULL, \
           raw_manifest_sha256 TEXT NOT NULL, \
           normalized_content_sha256 TEXT NOT NULL, \
           detail_content_sha256 TEXT NOT NULL, \
           aggregate_content_sha256 TEXT NOT NULL, \
           keyword_content_sha256 TEXT NOT NULL, \
           account_content_sha256 TEXT NOT NULL, \
           normalized_row_count BIGINT NOT NULL, \
           accepted_row_count BIGINT NOT NULL, \
           rejected_row_count BIGINT NOT NULL, \
           duplicate_row_count BIGINT NOT NULL, \
           detail_row_count BIGINT NOT NULL, \
           aggregate_row_count BIGINT NOT NULL, \
           keyword_row_count BIGINT NOT NULL, \
           account_row_count BIGINT NOT NULL, \
           manifest_sha256 TEXT NOT NULL, \
           producer_content_digest TEXT NOT NULL, \
           producer_content_id TEXT NOT NULL \
         );",
    )?;
    conn.execute(
        "INSERT INTO analysis_funds_content_manifest_v1 VALUES ( \
           ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,? \
         )",
        params![
            producer.schema_version,
            &producer.contract,
            &producer.case_id,
            producer.source_revision,
            &producer.producer_component_id,
            &producer.producer_component_version,
            &producer.producer_operation,
            &producer.producer_operation_schema_hash,
            &producer.duckdb_version,
            &producer.canonical_encoder,
            &producer.raw_manifest_sha256,
            &producer.normalized_content_sha256,
            &producer.detail_content_sha256,
            &producer.aggregate_content_sha256,
            &producer.keyword_content_sha256,
            &producer.account_content_sha256,
            producer.normalized_row_count,
            producer.accepted_row_count,
            producer.rejected_row_count,
            producer.duplicate_row_count,
            producer.detail_row_count,
            producer.aggregate_row_count,
            producer.keyword_row_count,
            producer.account_row_count,
            &producer_manifest_sha256,
            &producer_content_digest,
            &producer_content_id,
        ],
    )?;

    let accepted = test_table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!(
            "\"case_id\"='{CASE_ID}' AND clean_invalid=0 AND clean_failed=0 AND clean_reversal=0"
        ),
    )?;
    let rejected = test_table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!(
            "\"case_id\"='{CASE_ID}' AND (clean_invalid=1 OR clean_failed=1 OR clean_reversal=1)"
        ),
    )?;
    if accepted.row_count != accepted_row_count || rejected.row_count != rejected_row_count {
        bail!("test snapshot fixture classification coverage is inconsistent");
    }
    let duplicate_content_sha256 = sha256_domain(
        SNAPSHOT_DUPLICATE_DOMAIN,
        format!(
            "{}\0{}\0{}",
            source.source_signature,
            producer.normalized_content_sha256,
            SNAPSHOT_CLASSIFICATION_POLICY
        )
        .as_bytes(),
    );
    let snapshot = TestSnapshotManifestPayload {
        accepted_content_sha256: accepted.content_sha256,
        accepted_row_count,
        aggregate_content_sha256: producer.aggregate_content_sha256.clone(),
        aggregate_row_count,
        case_id: CASE_ID.to_string(),
        classification_policy: SNAPSHOT_CLASSIFICATION_POLICY.to_string(),
        contract: "analytix.duckdb-dataset-snapshot-manifest/v2".to_string(),
        duplicate_content_sha256,
        duplicate_row_count: 0,
        identity_schema_version: 2,
        materialization_name: source.agg_name.clone(),
        materialization_version: 12,
        normalized_content_sha256: producer.normalized_content_sha256.clone(),
        normalized_row_count: producer.normalized_row_count,
        producer_content_id,
        producer_manifest_sha256,
        raw_manifest_sha256: producer.raw_manifest_sha256.clone(),
        rejected_content_sha256: rejected.content_sha256,
        rejected_row_count,
        result_signature: result_signature.to_string(),
        schema_version: 2,
        source_revision: SOURCE_REVISION,
        source_signature: source.source_signature.clone(),
    };
    let snapshot_manifest_bytes = serde_json::to_vec(&snapshot)?;
    let snapshot_manifest_sha256 = sha256_bytes(&snapshot_manifest_bytes);
    let snapshot_digest = sha256_domain(SNAPSHOT_DIGEST_DOMAIN, &snapshot_manifest_bytes);
    conn.execute_batch(
        "DROP TABLE IF EXISTS analysis_dataset_snapshot_manifest_v2; \
         CREATE TABLE analysis_dataset_snapshot_manifest_v2( \
           manifest_schema_version INTEGER NOT NULL, \
           contract TEXT NOT NULL, \
           case_id TEXT NOT NULL, \
           source_revision BIGINT NOT NULL, \
           manifest_bytes BLOB NOT NULL, \
           manifest_sha256 TEXT NOT NULL, \
           snapshot_digest TEXT NOT NULL \
         );",
    )?;
    conn.execute(
        "INSERT INTO analysis_dataset_snapshot_manifest_v2 VALUES (?,?,?,?,?,?,?)",
        params![
            snapshot.schema_version,
            &snapshot.contract,
            &snapshot.case_id,
            snapshot.source_revision,
            &snapshot_manifest_bytes,
            &snapshot_manifest_sha256,
            &snapshot_digest,
        ],
    )?;
    Ok(())
}

fn test_table_content_manifest(
    conn: &Connection,
    table: &str,
    where_sql: &str,
) -> Result<TestTableContentManifest> {
    let mut schema = conn.prepare(
        "SELECT column_name, data_type, collation_name \
           FROM information_schema.columns \
          WHERE table_schema='main' AND table_name=? \
          ORDER BY ordinal_position",
    )?;
    let raw_columns = schema
        .query_map([table], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, Option<String>>(2)?,
            ))
        })?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    let mut columns = Vec::with_capacity(raw_columns.len());
    for (name, data_type, collation_name) in raw_columns {
        if collation_name
            .as_deref()
            .is_some_and(|value| !value.is_empty())
        {
            bail!("test snapshot fixture has unsupported collation");
        }
        columns.push(TestColumnManifest { name, data_type });
    }
    if columns.is_empty() {
        bail!("test snapshot fixture table is unavailable: {table}");
    }
    let quoted_columns = columns
        .iter()
        .map(|column| quote_identifier(&column.name))
        .collect::<Vec<_>>();
    let order_sql = quoted_columns
        .iter()
        .map(|column| format!("{column} NULLS FIRST"))
        .collect::<Vec<_>>()
        .join(", ");
    let mut sql = format!(
        "SELECT {} FROM {} AS r",
        quoted_columns.join(", "),
        quote_identifier(table)
    );
    if !where_sql.is_empty() {
        sql.push_str(" WHERE ");
        sql.push_str(where_sql);
    }
    sql.push_str(" ORDER BY ");
    sql.push_str(&order_sql);
    sql.push_str(", to_json(r)");

    let mut digest = Sha256::new();
    hash_json_line(
        &mut digest,
        &TestTableManifestHeader {
            columns: &columns,
            schema_version: 1,
            table,
        },
    )?;
    let mut statement = conn.prepare(&sql)?;
    let mut rows = statement.query([])?;
    let mut row_count = 0_i64;
    while let Some(row) = rows.next()? {
        let mut values = Vec::with_capacity(columns.len());
        for (index, column) in columns.iter().enumerate() {
            values.push(test_canonical_hash_value(
                row.get_ref(index)?,
                &column.data_type,
            )?);
        }
        hash_json_line(&mut digest, &values)?;
        row_count = row_count
            .checked_add(1)
            .context("test snapshot fixture row count overflow")?;
    }
    hash_json_line(&mut digest, &TestTableManifestFooter { row_count })?;
    Ok(TestTableContentManifest {
        content_sha256: format!("{:x}", digest.finalize()),
        row_count,
    })
}

fn test_canonical_hash_value(value: ValueRef<'_>, declared_type: &str) -> Result<Value> {
    if matches!(value, ValueRef::Null) {
        return Ok(json!({"type": "null"}));
    }
    let tagged = match value {
        ValueRef::TinyInt(value) => test_integer_value(value),
        ValueRef::SmallInt(value) => test_integer_value(value),
        ValueRef::Int(value) => test_integer_value(value),
        ValueRef::BigInt(value) => test_integer_value(value),
        ValueRef::HugeInt(value) => test_integer_value(value),
        ValueRef::UTinyInt(value) => test_integer_value(value),
        ValueRef::USmallInt(value) => test_integer_value(value),
        ValueRef::UInt(value) => test_integer_value(value),
        ValueRef::UBigInt(value) => test_integer_value(value),
        ValueRef::Float(value) => test_float_value(value as f64)?,
        ValueRef::Double(value) => test_float_value(value)?,
        ValueRef::Timestamp(unit, value) => json!({
            "type": "temporal",
            "value": test_timestamp_iso(unit, value, declared_type == "TIMESTAMP WITH TIME ZONE")?,
        }),
        ValueRef::Date32(value) => json!({
            "type": "temporal",
            "value": test_date_iso(value)?,
        }),
        ValueRef::Text(value) => json!({
            "type": "text",
            "value": std::str::from_utf8(value)
                .context("test snapshot fixture contains invalid UTF-8")?,
        }),
        _ => bail!("test snapshot fixture contains an unsupported value type"),
    };
    Ok(tagged)
}

fn test_integer_value(value: impl ToString) -> Value {
    json!({"type": "integer", "value": value.to_string()})
}

fn test_float_value(value: f64) -> Result<Value> {
    if !value.is_finite() {
        bail!("test snapshot fixture contains a non-finite numeric value");
    }
    let normalized = if value == 0.0 { 0.0 } else { value };
    let bits = normalized.to_bits();
    let sign = if bits >> 63 == 1 { "-" } else { "" };
    let exponent_bits = ((bits >> 52) & 0x7ff) as i32;
    let fraction = bits & ((1_u64 << 52) - 1);
    let rendered = if exponent_bits == 0 && fraction == 0 {
        format!("{sign}0x0.0p+0")
    } else {
        let (leading, exponent) = if exponent_bits == 0 {
            (0, -1022)
        } else {
            (1, exponent_bits - 1023)
        };
        format!("{sign}0x{leading:x}.{fraction:013x}p{exponent:+}")
    };
    Ok(json!({"type": "float", "value": rendered}))
}

fn test_timestamp_iso(unit: TimeUnit, value: i64, with_timezone: bool) -> Result<String> {
    let micros = test_time_unit_micros(unit, value)?;
    let datetime = DateTime::<Utc>::from_timestamp_micros(micros)
        .context("test snapshot fixture timestamp is out of range")?;
    let mut rendered = format!(
        "{} {}",
        datetime.date_naive().format("%Y-%m-%d"),
        test_format_naive_time(datetime.time())
    );
    if with_timezone {
        rendered.push_str("+00:00");
    }
    Ok(rendered)
}

fn test_date_iso(value: i32) -> Result<String> {
    let ce_days = 719_163_i32
        .checked_add(value)
        .context("test snapshot fixture date is out of range")?;
    let date = NaiveDate::from_num_days_from_ce_opt(ce_days)
        .context("test snapshot fixture date is out of range")?;
    Ok(date.format("%Y-%m-%d").to_string())
}

fn test_time_unit_micros(unit: TimeUnit, value: i64) -> Result<i64> {
    match unit {
        TimeUnit::Second => value
            .checked_mul(1_000_000)
            .context("test snapshot fixture temporal value is out of range"),
        TimeUnit::Millisecond => value
            .checked_mul(1_000)
            .context("test snapshot fixture temporal value is out of range"),
        TimeUnit::Microsecond => Ok(value),
        TimeUnit::Nanosecond => Ok(value / 1_000),
    }
}

fn test_format_naive_time(value: NaiveTime) -> String {
    if value.nanosecond() == 0 {
        value.format("%H:%M:%S").to_string()
    } else {
        value.format("%H:%M:%S%.6f").to_string()
    }
}

fn sha256_bytes(value: &[u8]) -> String {
    format!("{:x}", Sha256::digest(value))
}

fn sha256_domain(domain: &[u8], body: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(domain);
    hasher.update(body);
    format!("{:x}", hasher.finalize())
}

fn hash_json_line(hasher: &mut Sha256, value: &impl Serialize) -> Result<()> {
    serde_json::to_writer(&mut *hasher, value)?;
    hasher.update(b"\n");
    Ok(())
}

fn quote_identifier(value: &str) -> String {
    format!("\"{}\"", value.replace('"', "\"\""))
}

fn table_columns(conn: &Connection, table: &str) -> Result<Vec<String>> {
    let mut statement = conn.prepare(
        "SELECT column_name FROM information_schema.columns
          WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
    )?;
    let rows = statement.query_map([table], |row| row.get::<_, String>(0))?;
    rows.collect::<std::result::Result<Vec<_>, _>>()
        .map_err(Into::into)
}

fn scalar_i64(conn: &Connection, sql: &str) -> Result<i64> {
    Ok(conn.query_row(sql, [], |row| row.get::<_, i64>(0))?)
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}
