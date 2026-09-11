use std::collections::BTreeSet;

use anyhow::{anyhow, bail, Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

use super::args::{
    QueryChartCounterpartiesArgs, QueryChartDashboardArgs, QueryChartDetailRowsArgs,
    QueryChartDistributionArgs, QueryChartFlowArgs, QueryChartHeatmapArgs, QueryChartSummaryArgs,
    QueryChartTrendArgs, QueryFlowFocusGraphArgs, QueryFlowFocusRowsArgs, QueryStatsDateRangeArgs,
    QueryStatsRowsArgs, QueryStatsTreeArgs, QueryStatsTxnRowsArgs,
};
use crate::txn_daily_store::{
    build_materialization_identity, materialization_result_signature, require_base_table,
    verify_txn_daily_snapshot_binding, MaterializeArgs,
};

const SESSION_DOMAIN: &[u8] = b"analytix.verified-stats-query-session/v1";
const RANGE_DOMAIN: &[u8] = b"analytix.verified-stats-query-range/v1";

const AGG_COLUMNS: &[&str] = &[
    "txn_day",
    "acct_key",
    "cp_key",
    "cp_display",
    "cp_raw",
    "cp_placeholder_kind",
    "cp_name",
    "cp_name_pick",
    "cp_name_pick_cnt",
    "stats_name_key",
    "dc_val",
    "txn_count",
    "amount_source_present_count",
    "amount_valid_count",
    "amount_missing_count",
    "amount_parse_failed_count",
    "amt_sum",
    "first_ts",
    "last_ts",
    "open_name",
    "counterparty_bank",
    "location",
];

const DETAIL_COLUMNS: &[&str] = &[
    "id",
    "txn_day",
    "acct_key",
    "cp_key",
    "cp_raw",
    "cp_placeholder_kind",
    "cp_name",
    "cp_name_pick",
    "cp_name_pick_cnt",
    "stats_name_key",
    "dc_val",
    "txn_ts",
    "txn_time",
    "amount",
    "amount_source_present",
    "amount_parse_failed",
    "balance",
    "card_no",
    "acct_no",
    "account_open_name",
    "opener_id_no",
    "counterparty_acct",
    "cash_flag",
    "counterparty_name",
    "counterparty_id_no",
    "counterparty_bank",
    "summary",
    "currency",
    "branch_name",
    "branch_code",
    "location",
    "is_success",
    "voucher_no",
    "terminal_no",
    "ip_addr",
    "mac_addr",
    "counterparty_balance",
    "txn_id",
    "file_id",
    "log_id",
    "voucher_type",
    "voucher_id",
    "teller_no",
    "merchant_name",
    "merchant_no",
    "remark",
    "txn_type",
    "query_feedback_reason",
];

const KEYWORD_COLUMNS: &[&str] = &[
    "txn_row_id",
    "stable_txn_id",
    "kind",
    "token",
    "token_order",
];

const ACCOUNT_DIM_COLUMNS: &[&str] = &[
    "account_key",
    "acct_display",
    "card_display",
    "open_name",
    "id_no",
    "bank_name",
    "branch_name",
    "acct_type",
];

#[derive(Debug)]
struct MaterializationMeta {
    agg_name: String,
    source_revision: i64,
    source_row_count: i64,
    source_max_txn_ts: String,
    source_max_id: i64,
    source_signature: String,
    result_signature: String,
    row_count: i64,
}

#[derive(Debug)]
struct VerifiedRange {
    digest: String,
    row_count: i64,
    empty_observation_allowed: bool,
}

#[derive(Debug, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct ResultTableCounts {
    aggregate: i64,
    detail: i64,
    keyword: i64,
    account_dim: i64,
}

pub(super) struct VerifiedStatsQuerySession<'a> {
    conn: &'a Connection,
    context_digest: String,
    snapshot_binding: VerifiedStatsSnapshotBinding,
    ranges: Vec<VerifiedRange>,
    active: bool,
}

#[derive(Debug)]
pub(super) struct VerifiedStatsSnapshotBinding {
    pub(super) duckdb_content_snapshot_digest: String,
    pub(super) duckdb_snapshot_manifest_sha256: String,
    pub(super) materialization_identity: String,
    pub(super) source_signature: String,
    pub(super) result_signature: String,
    pub(super) producer_content_id: String,
    pub(super) producer_manifest_sha256: String,
    pub(super) normalized_row_count: i64,
    pub(super) accepted_row_count: i64,
    pub(super) rejected_row_count: i64,
    pub(super) duplicate_row_count: i64,
}

impl<'a> VerifiedStatsQuerySession<'a> {
    pub(super) fn begin<T: VerifiedStatsQueryRequest>(
        conn: &'a Connection,
        request: &T,
    ) -> Result<Self> {
        Self::begin_with_access_requirement(conn, request, true)
    }

    fn begin_with_access_requirement<T: VerifiedStatsQueryRequest>(
        conn: &'a Connection,
        request: &T,
        require_readonly: bool,
    ) -> Result<Self> {
        if request.case_id().trim().is_empty() {
            bail!("stats_query_case_binding_required");
        }
        if !request.has_required_scope() {
            bail!("stats_query_scope_required");
        }

        disable_external_access(conn)?;
        conn.execute_batch("BEGIN TRANSACTION")
            .context("begin verified stats query transaction")?;
        match Self::validate_started(conn, request, require_readonly) {
            Ok(session) => Ok(session),
            Err(error) => {
                if let Err(rollback_error) = conn.execute_batch("ROLLBACK") {
                    return Err(anyhow!(
                        "{error:#}; additionally failed to rollback verified stats query transaction: {rollback_error}"
                    ));
                }
                Err(error)
            }
        }
    }

    fn validate_started<T: VerifiedStatsQueryRequest>(
        conn: &'a Connection,
        request: &T,
        require_readonly: bool,
    ) -> Result<Self> {
        if require_readonly {
            let access_mode = conn.query_row(
                "SELECT COUNT(1), COALESCE(MIN(value), ''), COALESCE(MAX(value), '') \
                   FROM duckdb_settings() WHERE name='access_mode'",
                [],
                |row| {
                    Ok((
                        row.get::<_, i64>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, String>(2)?,
                    ))
                },
            )?;
            if access_mode.0 != 1 || access_mode.1 != "read_only" || access_mode.1 != access_mode.2
            {
                bail!("stats_query_connection_not_readonly");
            }
        }
        for table in [
            crate::META_TABLE,
            "analysis_revision_state",
            "fc_transaction_norm",
            "import_file_log",
            crate::AGG_TABLE,
            crate::DETAIL_TABLE,
            crate::KEYWORD_TABLE,
            crate::ACCOUNT_DIM_TABLE,
            "analysis_funds_content_manifest_v1",
            "analysis_dataset_snapshot_manifest_v2",
        ] {
            require_base_table(conn, table)
                .with_context(|| format!("stats_query_base_table_unavailable:{table}"))?;
        }
        let case_id = request.case_id().trim();
        let meta = load_materialization_meta(conn, case_id)?;
        verify_fixed_schemas(conn)?;
        let table_counts = verify_table_isolation_and_counts(conn, case_id, &meta)?;

        let identity = build_materialization_identity(
            conn,
            &MaterializeArgs {
                case_id: case_id.to_string(),
                db_path: Default::default(),
                source_revision: meta.source_revision,
                source_row_count: meta.source_row_count,
                source_max_txn_ts: meta.source_max_txn_ts.clone(),
                source_max_id: meta.source_max_id,
            },
        )
        .context("stats_query_source_identity_invalid")?;
        if identity.value != meta.agg_name || identity.source_signature != meta.source_signature {
            bail!("stats_query_source_identity_mismatch");
        }

        let actual_result_signature = materialization_result_signature(conn, case_id)
            .context("stats_query_result_signature_invalid")?;
        if actual_result_signature != meta.result_signature {
            bail!("stats_query_result_signature_mismatch");
        }

        let snapshot_binding = verify_txn_daily_snapshot_binding(conn, case_id)
            .context("stats_query_snapshot_content_invalid")?;

        let canonical_scope = canonical_json_bytes(&request.canonical_scope())?;
        let mut hasher = Sha256::new();
        hash_framed(&mut hasher, SESSION_DOMAIN);
        hash_framed(&mut hasher, case_id.as_bytes());
        hash_framed(&mut hasher, request.command().as_bytes());
        hash_framed(&mut hasher, meta.source_signature.as_bytes());
        hash_framed(&mut hasher, meta.result_signature.as_bytes());
        hash_framed(&mut hasher, &canonical_json_bytes(&json!(table_counts))?);
        hash_framed(&mut hasher, &canonical_scope);
        Ok(Self {
            conn,
            context_digest: format!("{:x}", hasher.finalize()),
            snapshot_binding: VerifiedStatsSnapshotBinding {
                duckdb_content_snapshot_digest: snapshot_binding.duckdb_content_snapshot_digest,
                duckdb_snapshot_manifest_sha256: snapshot_binding.duckdb_snapshot_manifest_sha256,
                materialization_identity: snapshot_binding.materialization_identity,
                source_signature: snapshot_binding.source_signature,
                result_signature: snapshot_binding.result_signature,
                producer_content_id: snapshot_binding.producer_content_id,
                producer_manifest_sha256: snapshot_binding.producer_manifest_sha256,
                normalized_row_count: snapshot_binding.source_row_count,
                accepted_row_count: snapshot_binding.accepted_row_count,
                rejected_row_count: snapshot_binding.rejected_row_count,
                duplicate_row_count: snapshot_binding.duplicate_row_count,
            },
            ranges: Vec::new(),
            active: true,
        })
    }

    pub(super) fn conn(&self) -> &Connection {
        self.conn
    }

    pub(super) fn snapshot_binding(&self) -> &VerifiedStatsSnapshotBinding {
        &self.snapshot_binding
    }

    #[cfg(test)]
    fn begin_readwrite_snapshot_test<T: VerifiedStatsQueryRequest>(
        conn: &'a Connection,
        request: &T,
    ) -> Result<Self> {
        Self::begin_with_access_requirement(conn, request, false)
    }

    pub(super) fn require_nonempty_query(&mut self, label: &str, sql: &str) -> Result<i64> {
        let row_count = crate::scalar_i64(
            self.conn,
            &format!("SELECT COUNT(1) FROM ({sql}) verified_stats_query_range"),
        )?;
        self.record_nonempty_range(label, sql, row_count)?;
        Ok(row_count)
    }

    pub(super) fn record_nonempty_range(
        &mut self,
        label: &str,
        query_identity: &str,
        row_count: i64,
    ) -> Result<()> {
        if row_count <= 0 {
            bail!("stats_query_scope_empty");
        }
        let mut hasher = Sha256::new();
        hash_framed(&mut hasher, RANGE_DOMAIN);
        hash_framed(&mut hasher, self.context_digest.as_bytes());
        hash_framed(&mut hasher, label.trim().as_bytes());
        hash_framed(&mut hasher, query_identity.as_bytes());
        hash_framed(&mut hasher, row_count.to_string().as_bytes());
        self.ranges.push(VerifiedRange {
            digest: format!("{:x}", hasher.finalize()),
            row_count,
            empty_observation_allowed: false,
        });
        Ok(())
    }

    pub(super) fn record_observed_range(
        &mut self,
        label: &str,
        query_identity: &str,
        row_count: i64,
    ) -> Result<()> {
        if label.trim().is_empty() || !crate::is_sha256_hex(query_identity) || row_count < 0 {
            bail!("stats_query_range_unverified");
        }
        let mut hasher = Sha256::new();
        hash_framed(&mut hasher, RANGE_DOMAIN);
        hash_framed(&mut hasher, self.context_digest.as_bytes());
        hash_framed(&mut hasher, label.trim().as_bytes());
        hash_framed(&mut hasher, query_identity.as_bytes());
        hash_framed(&mut hasher, row_count.to_string().as_bytes());
        hash_framed(&mut hasher, b"observed");
        self.ranges.push(VerifiedRange {
            digest: format!("{:x}", hasher.finalize()),
            row_count,
            empty_observation_allowed: true,
        });
        Ok(())
    }

    pub(super) fn commit(mut self) -> Result<()> {
        if self.ranges.is_empty()
            || self.ranges.iter().any(|range| {
                range.row_count < 0
                    || (range.row_count == 0 && !range.empty_observation_allowed)
                    || !crate::is_sha256_hex(range.digest.as_str())
            })
        {
            bail!("stats_query_range_unverified");
        }
        match self.conn.execute_batch("COMMIT") {
            Ok(()) => {
                self.active = false;
                Ok(())
            }
            Err(commit_error) => {
                if let Err(rollback_error) = self.conn.execute_batch("ROLLBACK") {
                    self.active = false;
                    return Err(anyhow!(
                        "failed to commit verified stats query transaction: {commit_error}; additionally failed to rollback: {rollback_error}"
                    ));
                }
                self.active = false;
                Err(commit_error.into())
            }
        }
    }
}

impl Drop for VerifiedStatsQuerySession<'_> {
    fn drop(&mut self) {
        if self.active {
            let _ = self.conn.execute_batch("ROLLBACK");
            self.active = false;
        }
    }
}

fn disable_external_access(conn: &Connection) -> Result<()> {
    conn.execute_batch("SET enable_external_access=false")
        .map_err(|_| anyhow!("stats_query_external_access_not_disabled"))?;
    let setting = conn
        .query_row(
            "SELECT COUNT(1), COALESCE(MIN(value), ''), COALESCE(MAX(value), '') \
               FROM duckdb_settings() WHERE name='enable_external_access'",
            [],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                ))
            },
        )
        .map_err(|_| anyhow!("stats_query_external_access_not_disabled"))?;
    if setting.0 != 1 || setting.1.to_ascii_lowercase() != "false" || setting.1 != setting.2 {
        bail!("stats_query_external_access_not_disabled");
    }
    Ok(())
}

fn load_materialization_meta(conn: &Connection, case_id: &str) -> Result<MaterializationMeta> {
    require_base_table(conn, crate::META_TABLE)
        .context("stats_query_materialization_meta_unavailable")?;
    let meta_columns = crate::table_columns(conn, crate::META_TABLE)?;
    for required in [
        "agg_name",
        "agg_version",
        "case_id",
        "identity_schema_version",
        "source_revision",
        "source_row_count",
        "source_max_txn_ts",
        "source_max_id",
        "source_signature",
        "result_signature",
        "row_count",
    ] {
        if !meta_columns.iter().any(|column| column == required) {
            bail!("stats_query_materialization_meta_schema_invalid");
        }
    }
    let count = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {} WHERE starts_with(agg_name, 'txn_daily_')",
            crate::META_TABLE
        ),
    )?;
    if count != 1 {
        let total =
            crate::scalar_i64(conn, &format!("SELECT COUNT(1) FROM {}", crate::META_TABLE))?;
        bail!("stats_query_materialization_meta_ambiguous:{count}:{total}");
    }

    let row = conn.query_row(
        &format!(
            "SELECT agg_name, agg_version, case_id, identity_schema_version, \
                    source_revision, source_row_count, \
                    COALESCE(CAST(source_max_txn_ts AS VARCHAR), ''), source_max_id, \
                    source_signature, result_signature, row_count \
               FROM {} WHERE starts_with(agg_name, 'txn_daily_')",
            crate::META_TABLE
        ),
        [],
        |row| {
            Ok((
                row.get::<_, String>(0)?,
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
    let (
        agg_name,
        agg_version,
        meta_case_id,
        identity_schema_version,
        source_revision,
        source_row_count,
        source_max_txn_ts,
        source_max_id,
        source_signature,
        result_signature,
        row_count,
    ) = row;
    if agg_version != crate::AGG_VERSION
        || identity_schema_version != crate::MATERIALIZATION_IDENTITY_SCHEMA_VERSION
    {
        bail!("stats_query_materialization_version_unsupported");
    }
    if meta_case_id.trim().is_empty() || meta_case_id != case_id {
        bail!("stats_query_case_binding_mismatch");
    }
    if !crate::is_sha256_hex(&source_signature)
        || !crate::is_sha256_hex(&result_signature)
        || agg_name
            != format!(
                "{}{}",
                crate::MATERIALIZATION_IDENTITY_PREFIX,
                source_signature
            )
    {
        bail!("stats_query_materialization_signature_invalid");
    }
    if source_revision <= 0 || source_row_count < 0 || source_max_id < 0 || row_count < 0 {
        bail!("stats_query_materialization_meta_invalid");
    }
    Ok(MaterializationMeta {
        agg_name,
        source_revision,
        source_row_count,
        source_max_txn_ts,
        source_max_id,
        source_signature,
        result_signature,
        row_count,
    })
}

fn verify_fixed_schemas(conn: &Connection) -> Result<()> {
    for (table, expected) in [
        (crate::AGG_TABLE, AGG_COLUMNS),
        (crate::DETAIL_TABLE, DETAIL_COLUMNS),
        (crate::KEYWORD_TABLE, KEYWORD_COLUMNS),
        (crate::ACCOUNT_DIM_TABLE, ACCOUNT_DIM_COLUMNS),
    ] {
        require_base_table(conn, table)
            .with_context(|| format!("stats_query_result_table_unavailable:{table}"))?;
        let actual = crate::table_columns(conn, table)?;
        if actual != expected {
            bail!("stats_query_result_schema_mismatch:{table}");
        }
    }
    Ok(())
}

fn verify_table_isolation_and_counts(
    conn: &Connection,
    case_id: &str,
    meta: &MaterializationMeta,
) -> Result<ResultTableCounts> {
    require_base_table(conn, "fc_transaction_norm")
        .context("stats_query_raw_source_unavailable")?;
    let agg_count = crate::scalar_i64(conn, &format!("SELECT COUNT(1) FROM {}", crate::AGG_TABLE))?;
    let detail_count = crate::scalar_i64(
        conn,
        &format!("SELECT COUNT(1) FROM {}", crate::DETAIL_TABLE),
    )?;
    let keyword_count = crate::scalar_i64(
        conn,
        &format!("SELECT COUNT(1) FROM {}", crate::KEYWORD_TABLE),
    )?;
    let account_count = crate::scalar_i64(
        conn,
        &format!("SELECT COUNT(1) FROM {}", crate::ACCOUNT_DIM_TABLE),
    )?;
    if agg_count != meta.row_count
        || detail_count < 0
        || keyword_count < 0
        || account_count < 0
        || (detail_count == 0) != (agg_count == 0)
        || (detail_count > 0 && account_count == 0)
    {
        bail!("stats_query_result_row_count_mismatch");
    }
    let case_literal = crate::sql_literal(case_id);
    let invalid_detail = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {detail} d \
              WHERE TRY_CAST(d.id AS BIGINT) IS NULL \
                 OR NOT EXISTS ( \
                      SELECT 1 FROM fc_transaction_norm r \
                       WHERE r.case_id={case_literal} \
                         AND TRY_CAST(r.id AS BIGINT)=TRY_CAST(d.id AS BIGINT) \
                         AND r.clean_invalid=0 AND r.clean_failed=0 AND r.clean_reversal=0 \
                 )",
            detail = crate::DETAIL_TABLE,
        ),
    )?;
    let duplicate_detail = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM (SELECT TRY_CAST(id AS BIGINT) AS id FROM {} GROUP BY 1 HAVING COUNT(1) <> 1) duplicates",
            crate::DETAIL_TABLE
        ),
    )?;
    let invalid_keywords = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {keyword} k \
              WHERE TRY_CAST(k.txn_row_id AS BIGINT) IS NULL \
                 OR NOT EXISTS ( \
                      SELECT 1 FROM {detail} d \
                       WHERE TRY_CAST(d.id AS BIGINT)=TRY_CAST(k.txn_row_id AS BIGINT) \
                         AND COALESCE(NULLIF(TRIM(d.txn_id), ''), CAST(d.id AS VARCHAR))=k.stable_txn_id \
                 )",
            keyword = crate::KEYWORD_TABLE,
            detail = crate::DETAIL_TABLE,
        ),
    )?;
    let invalid_accounts = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {accounts} a \
              WHERE a.account_key IS NULL OR TRIM(a.account_key)='' \
                 OR NOT EXISTS (SELECT 1 FROM {detail} d WHERE d.acct_key=a.account_key OR d.cp_key=a.account_key)",
            accounts = crate::ACCOUNT_DIM_TABLE,
            detail = crate::DETAIL_TABLE,
        ),
    )?;
    let duplicate_accounts = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM (SELECT account_key FROM {} GROUP BY account_key HAVING COUNT(1) <> 1) duplicates",
            crate::ACCOUNT_DIM_TABLE
        ),
    )?;
    let invalid_aggregates = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {agg} a \
              WHERE a.txn_count <= 0 \
                 OR NOT EXISTS ( \
                      SELECT 1 FROM {detail} d \
                       WHERE d.txn_day IS NOT DISTINCT FROM a.txn_day \
                         AND d.acct_key IS NOT DISTINCT FROM a.acct_key \
                         AND d.cp_key IS NOT DISTINCT FROM a.cp_key \
                         AND d.cp_raw IS NOT DISTINCT FROM a.cp_raw \
                         AND d.cp_placeholder_kind IS NOT DISTINCT FROM a.cp_placeholder_kind \
                         AND d.cp_name IS NOT DISTINCT FROM a.cp_name \
                         AND d.dc_val IS NOT DISTINCT FROM a.dc_val \
                 )",
            agg = crate::AGG_TABLE,
            detail = crate::DETAIL_TABLE,
        ),
    )?;
    if invalid_detail != 0
        || duplicate_detail != 0
        || invalid_keywords != 0
        || invalid_accounts != 0
        || duplicate_accounts != 0
        || invalid_aggregates != 0
    {
        bail!("stats_query_case_isolation_invalid");
    }
    Ok(ResultTableCounts {
        aggregate: agg_count,
        detail: detail_count,
        keyword: keyword_count,
        account_dim: account_count,
    })
}

fn canonical_json_bytes(value: &Value) -> Result<Vec<u8>> {
    serde_json::to_vec(value).context("serialize canonical stats query scope")
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

fn normalized_set(values: &[String]) -> Vec<String> {
    values
        .iter()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect()
}

fn normalized_list(values: &[String]) -> Vec<String> {
    values
        .iter()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .collect()
}

fn canonical_chart_filters(filters: &[super::args::QueryChartFilterArg]) -> Vec<Value> {
    let mut values = filters
        .iter()
        .map(|filter| {
            json!({
                "dimension": filter.dimension.trim(),
                "payload": filter.payload,
                "value": filter.value.trim(),
            })
        })
        .collect::<Vec<_>>();
    values.sort_by_key(|value| serde_json::to_string(value).unwrap_or_default());
    values
}

pub(super) trait VerifiedStatsQueryRequest {
    fn case_id(&self) -> &str;
    fn command(&self) -> &'static str;
    fn canonical_scope(&self) -> Value;
    fn has_required_scope(&self) -> bool {
        true
    }
}

impl VerifiedStatsQueryRequest for QueryStatsDateRangeArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-stats-date-range"
    }
    fn canonical_scope(&self) -> Value {
        json!({"range": "materialized"})
    }
}

impl VerifiedStatsQueryRequest for QueryStatsTreeArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-stats-tree"
    }
    fn canonical_scope(&self) -> Value {
        json!({"tab": self.tab.trim()})
    }
}

impl VerifiedStatsQueryRequest for QueryStatsRowsArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-stats-rows"
    }
    fn canonical_scope(&self) -> Value {
        json!({
            "dateEnd": self.date_end.trim(),
            "dateStart": self.date_start.trim(),
            "fields": normalized_list(&self.fields),
            "mode": self.mode.trim(),
            "rowFormat": self.row_format.trim(),
            "rowLimit": self.row_limit,
            "rowOffset": self.row_offset,
            "rowSortCol": self.row_sort_col.trim(),
            "rowSortDir": self.row_sort_dir.trim(),
            "searchText": self.search_text.trim(),
            "selectedKeys": normalized_set(&self.selected_keys),
        })
    }
    fn has_required_scope(&self) -> bool {
        !normalized_set(&self.selected_keys).is_empty()
    }
}

impl VerifiedStatsQueryRequest for QueryStatsTxnRowsArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-stats-txn-rows"
    }
    fn canonical_scope(&self) -> Value {
        json!({
            "cursor": self.cursor,
            "dateEnd": self.date_end.trim(),
            "dateStart": self.date_start.trim(),
            "direction": self.direction.trim(),
            "endTime": self.end_time.trim(),
            "fields": normalized_list(&self.fields),
            "keyType": self.key_type.trim(),
            "keyValue": self.key_value.trim(),
            "keyValues": normalized_set(&self.key_values),
            "limit": self.limit,
            "rowFormat": self.row_format.trim(),
            "selectedKeys": normalized_set(&self.selected_keys),
            "sortCol": self.sort_col.trim(),
            "sortDir": self.sort_dir.trim(),
            "startTime": self.start_time.trim(),
        })
    }
    fn has_required_scope(&self) -> bool {
        !self.key_value.trim().is_empty()
            || !normalized_set(&self.key_values).is_empty()
            || !normalized_set(&self.selected_keys).is_empty()
    }
}

macro_rules! impl_chart_scope {
    ($type:ty, $command:literal, {$($field:ident),* $(,)?}) => {
        impl VerifiedStatsQueryRequest for $type {
            fn case_id(&self) -> &str { &self.case_id }
            fn command(&self) -> &'static str { $command }
            fn canonical_scope(&self) -> Value {
                json!({
                    "chartFilters": canonical_chart_filters(&self.chart_filters),
                    "dateEnd": self.date_end.trim(),
                    "dateStart": self.date_start.trim(),
                    "selectedKeys": normalized_set(&self.selected_keys),
                    $(stringify!($field): self.$field,)*
                })
            }
            fn has_required_scope(&self) -> bool {
                !normalized_set(&self.selected_keys).is_empty()
            }
        }
    };
}

impl_chart_scope!(QueryChartSummaryArgs, "query-chart-summary", {
    success_filter,
    cash_filter,
    selection_mode,
    large_txn_threshold,
});
impl_chart_scope!(QueryChartTrendArgs, "query-chart-trend", {
    metric_mode,
    direction_mode,
    granularity,
    success_filter,
    cash_filter,
    selection_mode,
});
impl_chart_scope!(QueryChartCounterpartiesArgs, "query-chart-counterparties", {
    metric_mode,
    direction_mode,
    success_filter,
    cash_filter,
});
impl_chart_scope!(QueryChartHeatmapArgs, "query-chart-heatmap", {
    metric_mode,
    direction_mode,
    success_filter,
    cash_filter,
});
impl_chart_scope!(QueryChartDistributionArgs, "query-chart-distribution", {
    metric_mode,
    direction_mode,
    success_filter,
    cash_filter,
    selection_mode,
});
impl_chart_scope!(QueryChartFlowArgs, "query-chart-flow", {
    direction,
    success_filter,
    cash_filter,
    selection_mode,
});
impl_chart_scope!(QueryChartDashboardArgs, "query-chart-dashboard", {
    metric_mode,
    direction_mode,
    granularity,
    success_filter,
    cash_filter,
    selection_mode,
    large_txn_threshold,
});

impl VerifiedStatsQueryRequest for QueryChartDetailRowsArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-chart-detail-rows"
    }
    fn canonical_scope(&self) -> Value {
        json!({
            "cashFilter": self.cash_filter.trim(),
            "chartFilters": canonical_chart_filters(&self.chart_filters),
            "dateEnd": self.date_end.trim(),
            "dateStart": self.date_start.trim(),
            "directionMode": self.direction_mode.trim(),
            "limit": self.limit,
            "page": self.page,
            "selectedKeys": normalized_set(&self.selected_keys),
            "sortCol": self.sort_col.trim(),
            "sortDir": self.sort_dir.trim(),
            "successFilter": self.success_filter.trim(),
        })
    }
    fn has_required_scope(&self) -> bool {
        !normalized_set(&self.selected_keys).is_empty()
    }
}

impl VerifiedStatsQueryRequest for QueryFlowFocusRowsArgs {
    fn case_id(&self) -> &str {
        &self.case_id
    }
    fn command(&self) -> &'static str {
        "query-flow-focus-rows"
    }
    fn canonical_scope(&self) -> Value {
        json!({
            "dateEndExcl": self.date_end_excl.trim(),
            "dateStart": self.date_start.trim(),
            "direction": self.direction.trim(),
            "includeMissingCounterparty": self.include_missing_counterparty,
            "minAmount": self.min_amount,
            "querySeedIds": normalized_set(&self.query_seed_ids),
            "selectedFocusIds": normalized_set(&self.selected_focus_ids),
            "selectedPlaceholderKinds": normalized_set(&self.selected_placeholder_kinds),
        })
    }
    fn has_required_scope(&self) -> bool {
        !normalized_set(&self.query_seed_ids).is_empty()
            && (!normalized_set(&self.selected_focus_ids).is_empty()
                || !normalized_set(&self.selected_placeholder_kinds).is_empty())
    }
}

impl VerifiedStatsQueryRequest for QueryFlowFocusGraphArgs {
    fn case_id(&self) -> &str {
        &self.rows.case_id
    }
    fn command(&self) -> &'static str {
        "query-flow-focus-graph"
    }
    fn canonical_scope(&self) -> Value {
        json!({
            "depth": self.depth,
            "expectedRowCount": self.expected_row_count,
            "expectedTotalAmount": self.expected_total_amount,
            "focusIdRaw": self.focus_id_raw.trim(),
            "focusKeyType": self.focus_key_type.trim(),
            "focusLabel": self.focus_label.trim(),
            "requestId": self.request_id.trim(),
            "rows": QueryFlowFocusRowsArgs::canonical_scope(&self.rows),
            "seedIds": normalized_set(&self.seed_ids),
            "source": self.source.trim(),
            "viewMode": self.view_mode.trim(),
        })
    }
    fn has_required_scope(&self) -> bool {
        QueryFlowFocusRowsArgs::has_required_scope(&self.rows)
    }
}

#[cfg(test)]
mod snapshot_tests {
    use super::*;
    use std::fs;
    use std::path::PathBuf;
    use std::time::{SystemTime, UNIX_EPOCH};

    struct TempDb(PathBuf);

    impl TempDb {
        fn new() -> Self {
            let unique = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock after UNIX epoch")
                .as_nanos();
            Self(std::env::temp_dir().join(format!(
                "analytix-stats-session-concurrent-{}-{unique}.duckdb",
                std::process::id()
            )))
        }
    }

    impl Drop for TempDb {
        fn drop(&mut self) {
            let _ = fs::remove_file(&self.0);
            let mut wal = self.0.as_os_str().to_os_string();
            wal.push(".wal");
            let _ = fs::remove_file(PathBuf::from(wal));
        }
    }

    fn seed_materialized_db(path: &std::path::Path) {
        let conn = Connection::open(path).expect("open concurrent session fixture");
        conn.execute_batch(
            "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
             INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, txn_ts TIMESTAMP, txn_time TEXT, \
               acct_no TEXT, card_no TEXT, account_open_name TEXT, opener_id_no TEXT, \
               counterparty_acct TEXT, counterparty_name TEXT, counterparty_bank TEXT, \
               dc_flag TEXT, amount DOUBLE, clean_amount TEXT, balance DOUBLE, summary TEXT, currency TEXT, \
               file_id TEXT, row_no BIGINT, clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES ( \
               1, 'case-concurrent', TIMESTAMP '2026-04-01 09:00:00', \
               '2026-04-01 09:00:00', 'A-001', 'CARD-001', '张三', 'ID-A', \
               'CP-001', '对手甲', '测试银行', '进', 100.0, '100.00', 100.0, '入账', 'CNY', \
               'file-1', 1, 0, 0, 0 \
             ); \
             CREATE TABLE import_file_log( \
               file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, \
               rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT \
             ); \
             INSERT INTO import_file_log VALUES ( \
               'file-1', 'case-concurrent', 'fc_transaction', \
               'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', \
               1, '已完成', 'done' \
             );",
        )
        .expect("seed concurrent source tables");
        drop(conn);
        let materialize_args = MaterializeArgs {
            case_id: "case-concurrent".to_string(),
            db_path: path.to_path_buf(),
            source_revision: 1,
            source_row_count: 1,
            source_max_txn_ts: "2026-04-01 09:00:00".to_string(),
            source_max_id: 1,
        };
        materialize_args
            .materialize_with_raw_artifact_manifest_sha256(&"1".repeat(64))
            .expect("materialize concurrent fixture");
    }

    #[test]
    fn concurrent_raw_mutation_cannot_enter_active_verified_snapshot() {
        let db = TempDb::new();
        seed_materialized_db(&db.0);
        let conn = Connection::open(&db.0).expect("open verified session connection");
        let mutator = conn
            .try_clone()
            .expect("clone concurrent writer connection");
        let args = QueryStatsDateRangeArgs {
            case_id: "case-concurrent".to_string(),
            db_path: db.0.clone(),
        };

        let access_error = match VerifiedStatsQuerySession::begin(&conn, &args) {
            Ok(_) => panic!("production session must reject a read-write connection"),
            Err(error) => error,
        };
        assert!(
            format!("{access_error:#}").contains("stats_query_connection_not_readonly"),
            "error={access_error:#}"
        );

        let mut session = VerifiedStatsQuerySession::begin_readwrite_snapshot_test(&conn, &args)
            .expect("begin verified session");
        mutator
            .execute(
                "UPDATE fc_transaction_norm \
                    SET txn_ts=TIMESTAMP '2026-04-01 09:00:01' WHERE id=1",
                [],
            )
            .expect("commit concurrent raw mutation");
        let range = super::super::date_range::query_stats_date_range_with_session(&mut session)
            .expect("active snapshot remains internally consistent");
        assert_eq!(range["min"], "2026-04-01 09:00:00");
        session.commit().expect("commit verified snapshot");

        let error = match VerifiedStatsQuerySession::begin_readwrite_snapshot_test(&conn, &args) {
            Ok(_) => panic!("next session must reject stale materialization"),
            Err(error) => error,
        };
        let error_text = format!("{error:#}");
        assert!(
            error_text.contains("stats_query_source_identity_mismatch")
                || error_text.contains("stats_query_source_identity_invalid"),
            "error={error:#}"
        );
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn lower_complete_sha_is_required() {
        assert!(crate::is_sha256_hex(&"a".repeat(64)));
        assert!(!crate::is_sha256_hex(&"A".repeat(64)));
        assert!(!crate::is_sha256_hex(&"a".repeat(63)));
    }
}
