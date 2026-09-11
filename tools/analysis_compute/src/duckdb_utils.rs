use anyhow::{bail, Result};
use duckdb::{AccessMode, Config, Connection};
use sha2::{Digest, Sha256};
use std::path::Path;

use crate::{has_col, sql_literal, META_TABLE};

const ACCOUNT_FLOW_MAX_MEMORY: &str = "512MB";
const ACCOUNT_FLOW_MAX_THREADS: i64 = 2;

pub(crate) fn configure_connection(conn: &Connection) -> Result<()> {
    let threads = std::thread::available_parallelism()
        .map(|count| count.get().clamp(1, 8))
        .unwrap_or(4);
    conn.execute_batch(&format!(
        "
        SET autoinstall_known_extensions=false;
        SET autoload_known_extensions=false;
        PRAGMA threads={threads};
        SET preserve_insertion_order=false;
        "
    ))?;
    // TimeZone is ICU-owned in the bundled DuckDB build. The packaged native
    // process has a closed environment and must never autoload or install an
    // extension from a user home directory. Exact funds operations therefore
    // accept host-canonical UTC instants, use plain TIMESTAMP storage, and
    // perform deterministic fixed-offset conversion in the typed query.
    Ok(())
}

pub(crate) fn open_readonly_connection(path: &Path) -> Result<Connection> {
    Ok(Connection::open_with_flags(
        path,
        Config::default()
            .access_mode(AccessMode::ReadOnly)?
            .enable_external_access(false)?,
    )?)
}

// The fixed account-flow query is intentionally stricter than legacy
// read-only analysis sessions. It cannot spill to a path, autoload an
// extension, exceed its private memory budget, or fan out across an
// unbounded number of worker threads.
pub(crate) fn open_account_flow_readonly_connection(path: &Path) -> Result<Connection> {
    Ok(Connection::open_with_flags(
        path,
        Config::default()
            .with("temp_directory", "")?
            .with("max_temp_directory_size", "0B")?
            .max_memory(ACCOUNT_FLOW_MAX_MEMORY)?
            .threads(ACCOUNT_FLOW_MAX_THREADS)?
            .enable_autoload_extension(false)?
            .with("preserve_insertion_order", "false")?
            .access_mode(AccessMode::ReadOnly)?
            .enable_external_access(false)?,
    )?)
}

pub(crate) fn ensure_required_table(conn: &Connection, table: &str) -> Result<()> {
    if !table_exists(conn, table)? {
        bail!("{table} not found");
    }
    Ok(())
}

pub(crate) fn ensure_meta_table(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        CREATE TABLE IF NOT EXISTS {META_TABLE}(
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
            row_count BIGINT
        )
        "
    ))?;
    for (name, ty) in [
        ("case_id", "TEXT"),
        ("identity_schema_version", "INTEGER"),
        ("source_row_count", "BIGINT"),
        ("source_max_txn_ts", "TIMESTAMP"),
        ("source_max_id", "BIGINT"),
        ("source_signature", "TEXT"),
        ("source_parameter_signature", "TEXT"),
        ("result_signature", "TEXT"),
    ] {
        if !has_col(&table_columns(conn, META_TABLE)?, name) {
            conn.execute_batch(&format!("ALTER TABLE {META_TABLE} ADD COLUMN {name} {ty}"))?;
        }
    }
    conn.execute_batch(&format!(
        "ALTER TABLE {META_TABLE} ALTER COLUMN row_count DROP DEFAULT"
    ))?;
    Ok(())
}

pub(crate) fn canonical_case_table_signature(
    conn: &Connection,
    table: &str,
    case_id: &str,
    domain: &str,
    order_by: &str,
) -> Result<String> {
    ensure_required_table(conn, table)?;
    let columns = table_columns(conn, table)?;
    if columns.is_empty() || !columns.iter().any(|column| column == "case_id") {
        bail!("{table} does not expose case-bound result lineage");
    }

    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, domain.as_bytes());
    hash_framed(&mut hasher, case_id.as_bytes());
    for column in &columns {
        hash_framed(&mut hasher, column.as_bytes());
    }

    let mut statement = conn.prepare(&format!(
        "SELECT to_json(r) FROM {table} r WHERE r.case_id={} ORDER BY {order_by}, to_json(r)",
        sql_literal(case_id)
    ))?;
    let rows = statement.query_map([], |row| row.get::<_, String>(0))?;
    for row in rows {
        hash_framed(&mut hasher, row?.as_bytes());
    }
    Ok(format!("{:x}", hasher.finalize()))
}

pub(crate) fn is_sha256_hex(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

pub(crate) fn table_exists(conn: &Connection, table: &str) -> Result<bool> {
    Ok(scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM information_schema.tables WHERE table_schema='main' AND table_name={}",
            sql_literal(table)
        ),
    )? > 0)
}

pub(crate) fn table_columns(conn: &Connection, table: &str) -> Result<Vec<String>> {
    let mut stmt = conn.prepare(&format!(
        "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name={} ORDER BY ordinal_position",
        sql_literal(table)
    ))?;
    let rows = stmt.query_map([], |row| row.get::<_, String>(0))?;
    let mut out = Vec::new();
    for row in rows {
        out.push(row?);
    }
    Ok(out)
}

pub(crate) fn scalar_i64(conn: &Connection, sql: &str) -> Result<i64> {
    let mut stmt = conn.prepare(sql)?;
    Ok(stmt.query_row([], |row| row.get::<_, i64>(0))?)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn scalar_i64_preserves_zero_but_rejects_null() {
        let conn = Connection::open_in_memory().expect("open DuckDB fixture");
        assert_eq!(scalar_i64(&conn, "SELECT 0::BIGINT").expect("read zero"), 0);
        assert!(scalar_i64(&conn, "SELECT CAST(NULL AS BIGINT)").is_err());
    }

    #[test]
    fn readonly_connection_disables_external_access_before_open() {
        let path = std::env::temp_dir().join(format!(
            "analytix-readonly-external-access-{}.duckdb",
            std::process::id()
        ));
        let _ = std::fs::remove_file(&path);
        {
            let conn = Connection::open(&path).expect("create DuckDB fixture");
            conn.execute_batch("CREATE TABLE fixture(id BIGINT)")
                .expect("create fixture table");
        }

        let conn = open_readonly_connection(&path).expect("open restricted read-only fixture");
        configure_connection(&conn).expect("configure without extension autoload");
        let enabled = conn
            .query_row(
                "SELECT value FROM duckdb_settings() WHERE name='enable_external_access'",
                [],
                |row| row.get::<_, String>(0),
            )
            .expect("read external access setting");
        assert_eq!(enabled.to_ascii_lowercase(), "false");
        assert!(conn.execute_batch("ATTACH ':memory:' AS injected").is_err());

        drop(conn);
        std::fs::remove_file(path).expect("remove DuckDB fixture");
    }

    #[test]
    fn account_flow_connection_is_readonly_memory_bounded_and_never_spills() {
        let path = std::env::temp_dir().join(format!(
            "analytix-account-flow-resource-policy-{}.duckdb",
            std::process::id()
        ));
        let _ = std::fs::remove_file(&path);
        {
            let conn = Connection::open(&path).expect("create account-flow DuckDB fixture");
            conn.execute_batch("CREATE TABLE fixture(id BIGINT)")
                .expect("create account-flow fixture table");
        }

        let conn = open_account_flow_readonly_connection(&path)
            .expect("open resource-limited account-flow fixture");
        let setting = |name: &str| {
            conn.query_row(
                "SELECT value FROM duckdb_settings() WHERE name=?",
                [name],
                |row| row.get::<_, String>(0),
            )
            .expect("read account-flow DuckDB setting")
        };
        assert_eq!(
            setting("enable_external_access").to_ascii_lowercase(),
            "false"
        );
        assert_eq!(
            setting("autoinstall_known_extensions").to_ascii_lowercase(),
            "false"
        );
        assert_eq!(
            setting("autoload_known_extensions").to_ascii_lowercase(),
            "false"
        );
        assert_eq!(setting("threads"), ACCOUNT_FLOW_MAX_THREADS.to_string());
        assert_eq!(setting("temp_directory"), "");
        assert!(setting("max_temp_directory_size").starts_with('0'));
        assert_ne!(setting("memory_limit"), "unlimited");
        assert!(conn.execute_batch("ATTACH ':memory:' AS injected").is_err());

        drop(conn);
        std::fs::remove_file(path).expect("remove account-flow DuckDB fixture");
    }
}
