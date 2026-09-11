use anyhow::{anyhow, Context, Result};
use duckdb::Connection;
use serde::Deserialize;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::fs;
use std::io::{self, BufRead, Write};
use std::path::{Path, PathBuf};
use std::sync::mpsc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use super::args::{
    QueryStatsDateRangeArgs, QueryStatsRowsArgs, QueryStatsTreeArgs, QueryStatsTxnRowsArgs,
};
use super::date_range::query_stats_date_range_with_session;
use super::perf::StatsQueryDiagnostics;
use super::query_rows::{query_stats_rows_json_fields_with_session, StatsRowsQueryMetadata};
use super::session::VerifiedStatsQuerySession;
use super::tree::query_stats_tree_with_session;
use super::txn_rows::{query_stats_txn_rows_json_fields_with_session, StatsTxnRowsQueryMetadata};

const DEFAULT_CONNECTION_IDLE_TIMEOUT: Duration = Duration::from_millis(1_500);
const CONNECTION_IDLE_POLL_INTERVAL: Duration = Duration::from_millis(250);

#[derive(Debug, Deserialize)]
struct StatsQueryWorkerRequest {
    id: Option<Value>,
    command: String,
    args: Value,
}

pub(crate) fn run_stats_query_worker() -> Result<()> {
    let stdin = io::stdin();
    let (tx, rx) = mpsc::channel::<io::Result<String>>();
    std::thread::spawn(move || {
        for line in stdin.lock().lines() {
            if tx.send(line).is_err() {
                break;
            }
        }
    });

    let mut stdout = io::BufWriter::new(io::stdout());
    let mut state = StatsQueryWorkerState::new();
    loop {
        match rx.recv_timeout(CONNECTION_IDLE_POLL_INTERVAL) {
            Ok(line) => {
                let line = line?;
                if line.trim().is_empty() {
                    continue;
                }
                let response = handle_worker_line(&line, &mut state);
                stdout.write_all(&response)?;
                stdout.write_all(b"\n")?;
                stdout.flush()?;
                state.close_idle_connections();
            }
            Err(mpsc::RecvTimeoutError::Timeout) => {
                state.close_idle_connections();
            }
            Err(mpsc::RecvTimeoutError::Disconnected) => break,
        }
    }
    Ok(())
}

pub(crate) fn run_data_engine_stats_query(command: &str, args: Value) -> Result<Value> {
    let mut state = StatsQueryWorkerState::new();
    let request = StatsQueryWorkerRequest {
        id: Some(Value::Null),
        command: command.to_string(),
        args,
    };
    let mut out = Vec::new();
    write_worker_response(&request, &Value::Null, &mut state, &mut out)?;
    let mut payload: Value =
        serde_json::from_slice(&out).context("decode data engine stats response")?;
    if let Some(object) = payload.as_object_mut() {
        object.remove("id");
    }
    Ok(payload)
}

pub(crate) fn validate_data_engine_stats_query_args(command: &str, args: &Value) -> Result<()> {
    let result = match command {
        "query-stats-rows" => {
            serde_json::from_value::<QueryStatsRowsArgs>(args.clone()).map(|_| ())
        }
        "query-stats-txn-rows" => {
            serde_json::from_value::<QueryStatsTxnRowsArgs>(args.clone()).map(|_| ())
        }
        "query-stats-tree" => {
            serde_json::from_value::<QueryStatsTreeArgs>(args.clone()).map(|_| ())
        }
        "query-stats-date-range" => {
            serde_json::from_value::<QueryStatsDateRangeArgs>(args.clone()).map(|_| ())
        }
        other => return Err(anyhow!("unsupported data engine command: {other}")),
    };
    result.map_err(|_| anyhow!("data_engine_request_contract_invalid"))
}

fn handle_worker_line(line: &str, state: &mut StatsQueryWorkerState) -> Vec<u8> {
    let request = match serde_json::from_str::<StatsQueryWorkerRequest>(line) {
        Ok(value) => value,
        Err(exc) => {
            return value_to_bytes(&json!({
                "ok": false,
                "error": format!("invalid worker request: {exc}"),
            }))
        }
    };
    let id = request.id.clone().unwrap_or(Value::Null);
    let mut out = Vec::new();
    match write_worker_response(&request, &id, state, &mut out) {
        Ok(()) => out,
        Err(exc) => value_to_bytes(&json!({
            "id": id,
            "ok": false,
            "error": format!("{exc:#}"),
        })),
    }
}

fn write_worker_response<W: Write>(
    request: &StatsQueryWorkerRequest,
    id: &Value,
    state: &mut StatsQueryWorkerState,
    writer: &mut W,
) -> Result<()> {
    match request.command.as_str() {
        "query-stats-rows" => write_stats_rows_worker_response(request, id, state, writer),
        "query-stats-txn-rows" => write_stats_txn_rows_worker_response(request, id, state, writer),
        _ => {
            let mut payload = handle_worker_request(request, state)?;
            payload["id"] = id.clone();
            serde_json::to_writer(writer, &payload)?;
            Ok(())
        }
    }
}

fn write_stats_rows_worker_response<W: Write>(
    request: &StatsQueryWorkerRequest,
    id: &Value,
    state: &mut StatsQueryWorkerState,
    writer: &mut W,
) -> Result<()> {
    let args = normalize_rows_args(
        serde_json::from_value::<QueryStatsRowsArgs>(request.args.clone())
            .context("decode query-stats-rows args")?,
    )?;
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let cached = state.connection_entry(&args.db_path)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let CachedStatsConnection {
        conn,
        rows_metadata,
        ..
    } = cached;
    let mut session = VerifiedStatsQuerySession::begin(conn, &args)?;
    let mut fields = Vec::new();
    let result = query_stats_rows_json_fields_with_session(
        &args,
        &mut session,
        diagnostics,
        total_started,
        &mut fields,
        |conn| cached_rows_metadata(conn, rows_metadata),
    )?;
    session.commit()?;

    writer.write_all(b"{\"id\":")?;
    serde_json::to_writer(&mut *writer, id)?;
    writer.write_all(b",\"ok\":true,\"case_id\":")?;
    serde_json::to_writer(&mut *writer, &args.case_id)?;
    writer.write_all(b",")?;
    writer.write_all(&fields)?;
    writer.write_all(b",\"group_key\":")?;
    serde_json::to_writer(&mut *writer, &result.group_key)?;
    writer.write_all(b",\"diagnostics\":")?;
    serde_json::to_writer(&mut *writer, &result.diagnostics)?;
    writer.write_all(b"}")?;
    Ok(())
}

fn write_stats_txn_rows_worker_response<W: Write>(
    request: &StatsQueryWorkerRequest,
    id: &Value,
    state: &mut StatsQueryWorkerState,
    writer: &mut W,
) -> Result<()> {
    let args = normalize_txn_rows_args(
        serde_json::from_value::<QueryStatsTxnRowsArgs>(request.args.clone())
            .context("decode query-stats-txn-rows args")?,
    )?;
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let cached = state.connection_entry(&args.db_path)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let CachedStatsConnection {
        conn,
        txn_rows_metadata,
        ..
    } = cached;
    let mut session = VerifiedStatsQuerySession::begin(conn, &args)?;
    let mut fields = Vec::new();
    let result = query_stats_txn_rows_json_fields_with_session(
        &args,
        &mut session,
        diagnostics,
        total_started,
        &mut fields,
        |conn| cached_txn_rows_metadata(conn, txn_rows_metadata),
    )?;
    session.commit()?;

    writer.write_all(b"{\"id\":")?;
    serde_json::to_writer(&mut *writer, id)?;
    writer.write_all(b",\"ok\":true,\"case_id\":")?;
    serde_json::to_writer(&mut *writer, &args.case_id)?;
    writer.write_all(b",")?;
    writer.write_all(&fields)?;
    writer.write_all(b",\"diagnostics\":")?;
    serde_json::to_writer(&mut *writer, &result.diagnostics)?;
    writer.write_all(b"}")?;
    Ok(())
}

fn value_to_bytes(value: &Value) -> Vec<u8> {
    serde_json::to_vec(value)
        .unwrap_or_else(|_| b"{\"ok\":false,\"error\":\"serialize worker response\"}".to_vec())
}

fn handle_worker_request(
    request: &StatsQueryWorkerRequest,
    state: &mut StatsQueryWorkerState,
) -> Result<Value> {
    match request.command.as_str() {
        "query-stats-tree" => {
            let args = normalize_tree_args(
                serde_json::from_value::<QueryStatsTreeArgs>(request.args.clone())
                    .context("decode query-stats-tree args")?,
            )?;
            let conn = state.connection(&args.db_path)?;
            let mut session = VerifiedStatsQuerySession::begin(conn, &args)?;
            let groups = query_stats_tree_with_session(&args, &mut session)?;
            session.commit()?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "groups": groups,
            }))
        }
        "query-stats-date-range" => {
            let args = normalize_date_range_args(
                serde_json::from_value::<QueryStatsDateRangeArgs>(request.args.clone())
                    .context("decode query-stats-date-range args")?,
            )?;
            let conn = state.connection(&args.db_path)?;
            let mut session = VerifiedStatsQuerySession::begin(conn, &args)?;
            let date_range = query_stats_date_range_with_session(&mut session)?;
            session.commit()?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "date_range": date_range,
            }))
        }
        other => Err(anyhow!("unsupported worker command: {other}")),
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct DbFileState {
    len: u64,
    modified_ns: u128,
}

struct CachedStatsConnection {
    conn: Connection,
    file_state: Option<DbFileState>,
    rows_metadata: Option<StatsRowsQueryMetadata>,
    txn_rows_metadata: Option<StatsTxnRowsQueryMetadata>,
    last_used: Instant,
}

struct StatsQueryWorkerState {
    connections: HashMap<PathBuf, CachedStatsConnection>,
    idle_timeout: Duration,
}

impl StatsQueryWorkerState {
    fn new() -> Self {
        Self {
            connections: HashMap::new(),
            idle_timeout: worker_connection_idle_timeout(),
        }
    }

    fn connection_entry(&mut self, db_path: &Path) -> Result<&mut CachedStatsConnection> {
        self.close_idle_connections();
        let key = normalize_db_path(db_path);
        let file_state = db_file_state(db_path);
        let should_reuse = self
            .connections
            .get(&key)
            .map(|cached| cached.file_state == file_state)
            .unwrap_or(false);
        if !should_reuse {
            self.connections.remove(&key);
            let conn = crate::open_readonly_connection(db_path)
                .with_context(|| format!("open DuckDB {}", db_path.display()))?;
            crate::configure_connection(&conn)?;
            self.connections.insert(
                key.clone(),
                CachedStatsConnection {
                    conn,
                    file_state,
                    rows_metadata: None,
                    txn_rows_metadata: None,
                    last_used: Instant::now(),
                },
            );
        }
        let cached = self
            .connections
            .get_mut(&key)
            .ok_or_else(|| anyhow!("cached DuckDB connection missing"))?;
        cached.last_used = Instant::now();
        Ok(cached)
    }

    fn connection(&mut self, db_path: &Path) -> Result<&Connection> {
        Ok(&self.connection_entry(db_path)?.conn)
    }

    fn close_idle_connections(&mut self) {
        let idle_timeout = self.idle_timeout;
        self.connections
            .retain(|_, cached| cached.last_used.elapsed() < idle_timeout);
    }
}

fn cached_rows_metadata(
    conn: &Connection,
    metadata: &mut Option<StatsRowsQueryMetadata>,
) -> Result<StatsRowsQueryMetadata> {
    if let Some(metadata) = *metadata {
        return Ok(metadata);
    }
    let loaded = StatsRowsQueryMetadata::load(conn)?;
    *metadata = Some(loaded);
    Ok(loaded)
}

fn cached_txn_rows_metadata(
    conn: &Connection,
    metadata: &mut Option<StatsTxnRowsQueryMetadata>,
) -> Result<StatsTxnRowsQueryMetadata> {
    if let Some(metadata) = metadata.clone() {
        return Ok(metadata);
    }
    let loaded = StatsTxnRowsQueryMetadata::load(conn)?;
    *metadata = Some(loaded.clone());
    Ok(loaded)
}

fn worker_connection_idle_timeout() -> Duration {
    std::env::var("ANALYTIX_STATS_WORKER_CONN_IDLE_MS")
        .ok()
        .and_then(|raw| raw.trim().parse::<u64>().ok())
        .map(|value| value.clamp(100, 60_000))
        .map(Duration::from_millis)
        .unwrap_or(DEFAULT_CONNECTION_IDLE_TIMEOUT)
}

fn normalize_db_path(path: &Path) -> PathBuf {
    fs::canonicalize(path).unwrap_or_else(|_| path.to_path_buf())
}

fn db_file_state(path: &Path) -> Option<DbFileState> {
    let metadata = fs::metadata(path).ok()?;
    let modified = metadata.modified().ok()?;
    Some(DbFileState {
        len: metadata.len(),
        modified_ns: system_time_ns(modified),
    })
}

fn system_time_ns(value: SystemTime) -> u128 {
    value
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_nanos())
        .unwrap_or(0)
}

fn normalize_rows_args(mut args: QueryStatsRowsArgs) -> Result<QueryStatsRowsArgs> {
    if args.case_id.trim().is_empty() {
        return Err(anyhow!("--case-id is required"));
    }
    if args.db_path.as_os_str().is_empty() {
        return Err(anyhow!("--db-path is required"));
    }
    args.row_offset = args.row_offset.max(0);
    args.row_limit = args.row_limit.max(0);
    args.output_json = None;
    Ok(args)
}

fn normalize_date_range_args(args: QueryStatsDateRangeArgs) -> Result<QueryStatsDateRangeArgs> {
    if args.case_id.trim().is_empty() {
        return Err(anyhow!("--case-id is required"));
    }
    if args.db_path.as_os_str().is_empty() {
        return Err(anyhow!("--db-path is required"));
    }
    Ok(args)
}

fn normalize_tree_args(mut args: QueryStatsTreeArgs) -> Result<QueryStatsTreeArgs> {
    if args.case_id.trim().is_empty() {
        return Err(anyhow!("--case-id is required"));
    }
    if args.db_path.as_os_str().is_empty() {
        return Err(anyhow!("--db-path is required"));
    }
    if args.tab.trim().is_empty() {
        args.tab = "byName".to_string();
    }
    Ok(args)
}

fn normalize_txn_rows_args(mut args: QueryStatsTxnRowsArgs) -> Result<QueryStatsTxnRowsArgs> {
    if args.case_id.trim().is_empty() {
        return Err(anyhow!("--case-id is required"));
    }
    if args.db_path.as_os_str().is_empty() {
        return Err(anyhow!("--db-path is required"));
    }
    if args.cursor.is_some() && args.limit <= 0 {
        args.limit = 200;
    }
    args.limit = args.limit.max(0);
    args.output_json = None;
    Ok(args)
}

#[cfg(test)]
mod tests {
    use super::validate_data_engine_stats_query_args;
    use serde_json::json;

    #[test]
    fn data_engine_stats_args_reject_unknown_properties() {
        let exact = json!({
            "case_id": "case-a",
            "db_path": "/tmp/case-a.duckdb"
        });
        validate_data_engine_stats_query_args("query-stats-date-range", &exact)
            .expect("exact date range args");

        let extra = json!({
            "case_id": "case-a",
            "db_path": "/tmp/case-a.duckdb",
            "unexpected": true
        });
        let error = validate_data_engine_stats_query_args("query-stats-date-range", &extra)
            .expect_err("unknown field must fail");
        assert!(format!("{error:#}").contains("data_engine_request_contract_invalid"));
    }
}
