use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde::Serialize;
use serde_json::json;
use std::collections::BTreeSet;

use super::args::MaterializeArgs;

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub(crate) struct MaterializationIdentity {
    pub(crate) value: String,
    pub(crate) source_signature: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct RawSourceIdentity {
    cleaned_status: String,
    file_id: String,
    rows_imported_norm: i64,
    sha256: String,
    status: String,
}

pub(super) fn ensure_identity_columns(conn: &Connection) -> Result<()> {
    let columns = crate::table_columns(conn, crate::META_TABLE)?;
    for (name, ty) in [
        ("case_id", "TEXT"),
        ("identity_schema_version", "INTEGER"),
        ("source_signature", "TEXT"),
    ] {
        if !crate::has_col(&columns, name) {
            conn.execute_batch(&format!(
                "ALTER TABLE {} ADD COLUMN {name} {ty}",
                crate::META_TABLE
            ))?;
        }
    }
    Ok(())
}

pub(crate) fn build_materialization_identity(
    conn: &Connection,
    args: &MaterializeArgs,
) -> Result<MaterializationIdentity> {
    let case_id = args.case_id.trim();
    if case_id.is_empty() {
        bail!("materialization case identity is unavailable");
    }

    let revision = conn.query_row(
        "SELECT COUNT(1), COALESCE(MIN(revision), 0), COALESCE(MAX(revision), 0) \
         FROM analysis_revision_state WHERE revision_key='stats_flow_source'",
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        },
    )?;
    if revision.0 != 1 {
        bail!("materialization source revision is ambiguous");
    }
    if revision.1 <= 0 || revision.1 != args.source_revision || revision.1 != revision.2 {
        bail!("materialization source revision changed");
    }

    let source = conn.query_row(
        "SELECT COUNT(1), COUNT(TRY_CAST(id AS BIGINT)), \
                COUNT(DISTINCT TRY_CAST(id AS BIGINT)), \
                COALESCE(CAST(MAX(txn_ts) AS VARCHAR), ''), \
                COALESCE(MAX(id), 0), \
                SUM(CASE WHEN clean_invalid IS NULL OR clean_invalid NOT IN (0, 1) \
                              OR clean_failed IS NULL OR clean_failed NOT IN (0, 1) \
                              OR clean_reversal IS NULL OR clean_reversal NOT IN (0, 1) \
                         THEN 1 ELSE 0 END), \
                SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1 THEN 1 ELSE 0 END) \
           FROM fc_transaction_norm WHERE case_id=?",
        [case_id],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, Option<i64>>(5)?.unwrap_or(0),
                row.get::<_, Option<i64>>(6)?.unwrap_or(0),
            ))
        },
    )?;
    if source.0 != source.1 || source.0 != source.2 {
        bail!("materialization source row lineage is ambiguous");
    }
    if source.5 != 0 {
        bail!("materialization rejection state is invalid");
    }
    if source.0 != args.source_row_count
        || source.3 != args.source_max_txn_ts
        || source.4 != args.source_max_id
    {
        bail!("materialization source content changed");
    }

    let mut statement = conn.prepare(
        "SELECT file_id, sha256, rows_imported_norm, status, \
                COALESCE(cleaned_status, '') \
           FROM import_file_log \
          WHERE case_id=? AND kind='fc_transaction' \
          ORDER BY file_id",
    )?;
    let rows = statement.query_map([case_id], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?.unwrap_or_default(),
            row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            row.get::<_, Option<i64>>(2)?,
            row.get::<_, Option<String>>(3)?.unwrap_or_default(),
            row.get::<_, Option<String>>(4)?.unwrap_or_default(),
        ))
    })?;
    let mut raw_sources = Vec::new();
    let mut seen_file_ids = BTreeSet::new();
    for row in rows {
        let (file_id, sha256, rows_imported_norm, status, cleaned_status) = row?;
        let file_id = file_id.trim().to_string();
        let sha256 = sha256.trim().to_ascii_lowercase();
        let status = status.trim().to_string();
        let cleaned_status = cleaned_status.trim().to_ascii_lowercase();
        let Some(rows_imported_norm) = rows_imported_norm else {
            bail!("materialization source manifest is incomplete");
        };
        if file_id.is_empty()
            || !seen_file_ids.insert(file_id.clone())
            || !is_complete_sha256(&sha256)
            || rows_imported_norm < 0
            || status != "已完成"
            || cleaned_status != "done"
        {
            bail!("materialization source manifest is incomplete");
        }
        raw_sources.push(RawSourceIdentity {
            cleaned_status,
            file_id,
            rows_imported_norm,
            sha256,
            status,
        });
    }
    if raw_sources.is_empty() {
        bail!("materialization source manifest is unavailable");
    }

    let normalized_source_sha256 = crate::canonical_case_table_signature(
        conn,
        "fc_transaction_norm",
        case_id,
        "analytix.txn-daily-normalized-source/v12",
        "TRY_CAST(r.id AS BIGINT)",
    )?;

    let canonical = serde_json::to_string(&json!({
        "acceptedRowCount": source.0 - source.6,
        "caseId": case_id,
        "normalizedSourceSha256": normalized_source_sha256,
        "rawSources": raw_sources,
        "rejectedRowCount": source.6,
        "schemaVersion": crate::MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
        "sourceMaxId": source.4,
        "sourceMaxTxnTimestamp": source.3,
        "sourceRevision": revision.1,
        "sourceRowCount": source.0,
    }))?;
    let source_signature = conn
        .query_row("SELECT sha256(?)", [canonical.as_str()], |row| {
            row.get::<_, String>(0)
        })
        .context("hash transaction materialization identity")?;
    if !is_complete_sha256(&source_signature) {
        bail!("materialization identity hash is invalid");
    }
    Ok(MaterializationIdentity {
        value: format!(
            "{}{}",
            crate::MATERIALIZATION_IDENTITY_PREFIX,
            source_signature
        ),
        source_signature,
    })
}

fn is_complete_sha256(value: &str) -> bool {
    value.len() == 64
        && value
            .as_bytes()
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture(case_id: &str, sha256: &str, revision: i64) -> (Connection, MaterializeArgs) {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, txn_ts TIMESTAMP, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             CREATE TABLE import_file_log( \
               file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, \
               rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT \
             );",
        )
        .expect("create fixture schema");
        conn.execute(
            "INSERT INTO analysis_revision_state VALUES ('stats_flow_source', ?)",
            [revision],
        )
        .expect("insert revision");
        conn.execute(
            "INSERT INTO fc_transaction_norm VALUES \
               (1, ?, NULL, 0, 0, 0), (2, ?, NULL, 1, 0, 0)",
            [case_id, case_id],
        )
        .expect("insert transactions");
        conn.execute(
            "INSERT INTO import_file_log VALUES ('file-1', ?, 'fc_transaction', ?, 2, '已完成', 'done')",
            [case_id, sha256],
        )
        .expect("insert manifest");
        let args = MaterializeArgs {
            case_id: case_id.to_string(),
            db_path: Default::default(),
            source_revision: revision,
            source_row_count: 2,
            source_max_txn_ts: String::new(),
            source_max_id: 2,
        };
        (conn, args)
    }

    #[test]
    fn identity_is_case_revision_and_content_bound() {
        let (conn_a, args_a) = fixture("case-a", &"a".repeat(64), 7);
        let identity_a = build_materialization_identity(&conn_a, &args_a).expect("identity a");
        let identity_a_again =
            build_materialization_identity(&conn_a, &args_a).expect("identity a again");
        let (conn_b, args_b) = fixture("case-b", &"a".repeat(64), 7);
        let (conn_revision, args_revision) = fixture("case-a", &"a".repeat(64), 8);
        let (conn_content, args_content) = fixture("case-a", &"b".repeat(64), 7);

        assert_eq!(identity_a, identity_a_again);
        assert_eq!(
            identity_a.source_signature,
            "a5c945c07a0f0d47f008deb3ddbd139eb2bc4757ed5247ea6e9d268595533284"
        );
        assert!(identity_a
            .value
            .starts_with(crate::MATERIALIZATION_IDENTITY_PREFIX));
        conn_a
            .execute(
                "UPDATE fc_transaction_norm SET clean_invalid=CASE id WHEN 1 THEN 1 ELSE 0 END",
                [],
            )
            .expect("swap cleaning decisions without changing counts");
        assert_ne!(
            identity_a,
            build_materialization_identity(&conn_a, &args_a)
                .expect("clean-row binding changes identity")
        );
        assert_ne!(
            identity_a,
            build_materialization_identity(&conn_b, &args_b).expect("case-bound identity")
        );
        assert_ne!(
            identity_a,
            build_materialization_identity(&conn_revision, &args_revision)
                .expect("revision-bound identity")
        );
        assert_ne!(
            identity_a,
            build_materialization_identity(&conn_content, &args_content)
                .expect("content-bound identity")
        );
    }

    #[test]
    fn stale_or_invalid_source_state_fails_closed() {
        let (conn, mut args) = fixture("case-a", &"a".repeat(64), 7);
        args.source_row_count = 3;
        assert!(build_materialization_identity(&conn, &args).is_err());
        conn.execute(
            "UPDATE import_file_log SET sha256='not-a-complete-sha256'",
            [],
        )
        .expect("corrupt manifest");
        args.source_row_count = 2;
        assert!(build_materialization_identity(&conn, &args).is_err());

        let (conn, args) = fixture("case-a", &"a".repeat(64), 7);
        conn.execute(
            "UPDATE fc_transaction_norm SET clean_failed=NULL WHERE id=1",
            [],
        )
        .expect("corrupt cleaning state");
        assert!(build_materialization_identity(&conn, &args).is_err());

        let (conn, args) = fixture("case-a", &"a".repeat(64), 7);
        conn.execute("UPDATE fc_transaction_norm SET id=1 WHERE id=2", [])
            .expect("duplicate source lineage");
        assert!(build_materialization_identity(&conn, &args).is_err());
    }

    #[test]
    fn materialization_identity_rejects_null_rows_imported_norm() {
        let (conn, args) = fixture("case-a", &"a".repeat(64), 7);
        conn.execute("UPDATE import_file_log SET rows_imported_norm=NULL", [])
            .expect("clear manifest row count");

        let error = build_materialization_identity(&conn, &args)
            .expect_err("missing manifest row count must not become zero");
        assert!(error
            .to_string()
            .contains("materialization source manifest is incomplete"));
    }
}
