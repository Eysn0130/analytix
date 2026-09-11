use anyhow::{anyhow, bail, Context, Result};
use duckdb::{
    params_from_iter,
    types::{Value as DuckValue, ValueRef},
    AccessMode, Config, Connection,
};
use serde::{
    de::{self, MapAccess, SeqAccess, Visitor},
    Deserialize, Deserializer, Serialize,
};
use serde_json::{json, Map, Number, Value};
use sha2::{Digest, Sha256};
use std::collections::{HashMap, HashSet};
use std::fmt;
use std::fs::{self, File, OpenOptions};
use std::io::{self, BufRead, Read, Write};
use std::path::{Path, PathBuf};
use std::sync::{
    atomic::{AtomicBool, AtomicU64, Ordering},
    mpsc, Arc,
};
use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

#[cfg(unix)]
use std::os::fd::FromRawFd;
#[cfg(unix)]
use std::os::unix::fs::MetadataExt;

#[cfg(target_os = "macos")]
mod owner_liveness_macos;

const MAX_REQUEST_FRAME_BYTES: usize = 4 * 1024 * 1024;
const MAX_RESPONSE_FRAME_BYTES: usize = 4 * 1024 * 1024;
const MAX_REQUEST_JSON_DEPTH: usize = 64;
const MAX_REQUEST_JSON_NODES: usize = 200_000;
const MAX_REQUEST_JSON_STRING_BYTES: usize = 1024 * 1024;
const NATIVE_PROTOCOL_VERSION: &str = "analytix-native-v1";
const NATIVE_COMPONENT_ID: &str = "data-engine";
const NATIVE_READINESS_SCHEMA_VERSION: u64 = 3;
const NATIVE_READY_FRAME_MAX_BYTES: usize = 4 * 1024;
const NATIVE_LAUNCH_NONCE_BYTES: usize = 64;
const MAX_DIAGNOSTIC_RUN_MS: u64 = 10_000;
const OWNER_RECORD_VERSION: u32 = 2;
const SHA256_HEX_BYTES: usize = 64;
const MAX_OWNER_RECORD_BYTES: usize = 4 * 1024;
#[cfg(unix)]
const ACCOUNT_FLOW_SNAPSHOT_FD: libc::c_int = 3;
#[cfg(unix)]
const CANONICAL_CSV_OUTPUT_FD: libc::c_int = 5;
#[cfg(unix)]
// The host stages the exclusive private inode with basename `3` before
// open+unlink. The pinned macOS DuckDB canonicalizer otherwise substitutes
// the descriptor's former basename while opening this fixed device path.
const ACCOUNT_FLOW_SNAPSHOT_FD_PATH: &str = "/dev/fd/3";

static OWNER_EPOCH_COUNTER: AtomicU64 = AtomicU64::new(0);

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct EngineRequest {
    request_id: Option<String>,
    command: String,
    case_id: Option<String>,
    #[serde(default)]
    db_path: RequestDbPath,
    payload: Option<StrictJsonValue>,
}

impl EngineRequest {
    fn payload_value(&self) -> Option<&Value> {
        self.payload.as_ref().map(|payload| &payload.0)
    }
}

#[derive(Debug, Default)]
struct RequestDbPath {
    value: Option<PathBuf>,
    supplied: bool,
}

impl RequestDbPath {
    fn as_ref(&self) -> Option<&PathBuf> {
        self.value.as_ref()
    }

    fn is_some(&self) -> bool {
        self.value.is_some()
    }

    fn was_supplied(&self) -> bool {
        self.supplied
    }
}

impl<'de> Deserialize<'de> for RequestDbPath {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        struct RequestDbPathVisitor;

        impl<'de> Visitor<'de> for RequestDbPathVisitor {
            type Value = RequestDbPath;

            fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
                formatter.write_str("a database path string or null")
            }

            fn visit_none<E>(self) -> std::result::Result<Self::Value, E> {
                Ok(RequestDbPath {
                    value: None,
                    supplied: true,
                })
            }

            fn visit_some<D>(self, deserializer: D) -> std::result::Result<Self::Value, D::Error>
            where
                D: Deserializer<'de>,
            {
                Ok(RequestDbPath {
                    value: Some(PathBuf::deserialize(deserializer)?),
                    supplied: true,
                })
            }
        }

        deserializer.deserialize_option(RequestDbPathVisitor)
    }
}

#[derive(Debug)]
struct StrictJsonValue(Value);

impl<'de> Deserialize<'de> for StrictJsonValue {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserializer.deserialize_any(StrictJsonVisitor)
    }
}

struct StrictJsonVisitor;

impl<'de> Visitor<'de> for StrictJsonVisitor {
    type Value = StrictJsonValue;

    fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("a strict JSON value without duplicate object keys")
    }

    fn visit_bool<E>(self, value: bool) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::Bool(value)))
    }

    fn visit_i64<E>(self, value: i64) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::Number(Number::from(value))))
    }

    fn visit_u64<E>(self, value: u64) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::Number(Number::from(value))))
    }

    fn visit_f64<E>(self, value: f64) -> std::result::Result<Self::Value, E>
    where
        E: de::Error,
    {
        Number::from_f64(value)
            .map(Value::Number)
            .map(StrictJsonValue)
            .ok_or_else(|| E::custom("non-finite JSON number"))
    }

    fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::String(value.to_owned())))
    }

    fn visit_string<E>(self, value: String) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::String(value)))
    }

    fn visit_none<E>(self) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::Null))
    }

    fn visit_unit<E>(self) -> std::result::Result<Self::Value, E> {
        Ok(StrictJsonValue(Value::Null))
    }

    fn visit_some<D>(self, deserializer: D) -> std::result::Result<Self::Value, D::Error>
    where
        D: Deserializer<'de>,
    {
        StrictJsonValue::deserialize(deserializer)
    }

    fn visit_seq<A>(self, mut sequence: A) -> std::result::Result<Self::Value, A::Error>
    where
        A: SeqAccess<'de>,
    {
        let mut values = Vec::new();
        while let Some(value) = sequence.next_element::<StrictJsonValue>()? {
            values.push(value.0);
        }
        Ok(StrictJsonValue(Value::Array(values)))
    }

    fn visit_map<A>(self, mut object: A) -> std::result::Result<Self::Value, A::Error>
    where
        A: MapAccess<'de>,
    {
        let mut keys = HashSet::new();
        let mut values = Map::new();
        while let Some((key, value)) = object.next_entry::<String, StrictJsonValue>()? {
            if !keys.insert(key.clone()) {
                return Err(de::Error::custom("duplicate JSON object key"));
            }
            values.insert(key, value.0);
        }
        Ok(StrictJsonValue(Value::Object(values)))
    }
}

fn main() -> Result<()> {
    let arguments: Vec<_> = std::env::args_os().skip(1).collect();
    if arguments.len() == 1 && arguments[0] == "--help" {
        println!("Usage: analytix-data-engine");
        return Ok(());
    }
    let build_probe = arguments.len() == 1 && arguments[0] == "--analytix-native-probe";
    if !arguments.is_empty() && !build_probe {
        return Err(anyhow!("data_engine_startup_contract_invalid"));
    }
    let stdin = io::stdin();
    let mut stdout = io::stdout().lock();
    bootstrap_native_process(&mut stdout, !build_probe)?;
    let mut input = stdin.lock();
    let mut state = EngineState::default();
    loop {
        let frame = match read_bounded_request_frame(&mut input, MAX_REQUEST_FRAME_BYTES) {
            Ok(Some(frame)) => frame,
            Ok(None) => break,
            Err(error) if error.kind() == io::ErrorKind::InvalidData => {
                write_bounded_response(
                    &mut stdout,
                    &fixed_protocol_error("data_engine_request_limit_exceeded"),
                    MAX_RESPONSE_FRAME_BYTES,
                )?;
                break;
            }
            Err(error) if error.kind() == io::ErrorKind::UnexpectedEof => {
                write_bounded_response(
                    &mut stdout,
                    &fixed_protocol_error("invalid_request_json"),
                    MAX_RESPONSE_FRAME_BYTES,
                )?;
                break;
            }
            Err(error) => return Err(error.into()),
        };
        if frame.iter().all(u8::is_ascii_whitespace) {
            continue;
        }
        let response = match validate_request_json_shape(&frame) {
            Ok(()) => match std::str::from_utf8(&frame) {
                Ok(line) if build_probe => handle_build_probe_line(&mut state, line),
                Ok(line) => handle_line_frame_with_state(&mut state, line),
                Err(_) => EngineResponseFrame::Value(fixed_protocol_error("invalid_request_json")),
            },
            Err(JsonShapeError::LimitExceeded) => EngineResponseFrame::Value(fixed_protocol_error(
                "data_engine_request_limit_exceeded",
            )),
            Err(JsonShapeError::Invalid) => {
                EngineResponseFrame::Value(fixed_protocol_error("invalid_request_json"))
            }
        };
        write_bounded_response(&mut stdout, &response, MAX_RESPONSE_FRAME_BYTES)?;
    }
    Ok(())
}

fn bootstrap_native_process<W: Write>(writer: &mut W, require_owner_liveness: bool) -> Result<()> {
    let nonce = native_launch_nonce()?;
    let protocol = std::env::var("ANALYTIX_NATIVE_PROTOCOL_VERSION")
        .map_err(|_| anyhow!("data_engine_bootstrap_invalid"))?;
    if protocol != NATIVE_PROTOCOL_VERSION {
        return Err(anyhow!("data_engine_bootstrap_invalid"));
    }
    std::env::remove_var("ANALYTIX_NATIVE_LAUNCH_NONCE");
    std::env::remove_var("ANALYTIX_NATIVE_PROTOCOL_VERSION");
    if require_owner_liveness {
        arm_owner_liveness()?;
    }
    let readiness = json!({
        "kind": "analytix_native_ready",
        "schema_version": NATIVE_READINESS_SCHEMA_VERSION,
        "component_id": NATIVE_COMPONENT_ID,
        "launch_nonce": nonce,
        "protocol_version": NATIVE_PROTOCOL_VERSION,
        "process_id": std::process::id(),
    });
    write_bounded_response(writer, &readiness, NATIVE_READY_FRAME_MAX_BYTES)
        .context("data_engine_bootstrap_write_failed")
}

#[cfg(target_os = "macos")]
fn arm_owner_liveness() -> Result<()> {
    owner_liveness_macos::arm()
}

#[cfg(not(target_os = "macos"))]
fn arm_owner_liveness() -> Result<()> {
    Ok(())
}

fn handle_build_probe_line(state: &mut EngineState, line: &str) -> EngineResponseFrame {
    let request = serde_json::from_str::<EngineRequest>(line);
    if !matches!(request.as_ref(), Ok(request) if request.command == "ping") {
        return EngineResponseFrame::Value(fixed_protocol_error(
            "data_engine_request_contract_invalid",
        ));
    }
    handle_line_frame_with_state(state, line)
}

fn native_launch_nonce() -> Result<String> {
    let nonce = std::env::var("ANALYTIX_NATIVE_LAUNCH_NONCE")
        .map_err(|_| anyhow!("data_engine_bootstrap_invalid"))?;
    if !valid_native_launch_nonce(&nonce) {
        return Err(anyhow!("data_engine_bootstrap_invalid"));
    }
    Ok(nonce)
}

fn valid_native_launch_nonce(nonce: &str) -> bool {
    nonce.len() == NATIVE_LAUNCH_NONCE_BYTES
        && nonce
            .bytes()
            .all(|value| value.is_ascii_digit() || (b'a'..=b'f').contains(&value))
}

fn fixed_protocol_error(code: &str) -> Value {
    json!({
        "request_id": Value::Null,
        "ok": false,
        "error": {
            "code": code,
            "message": public_error_message(code),
        },
        "diagnostics": diagnostics("", false, false, Instant::now()),
    })
}

fn read_bounded_request_frame<R: BufRead>(
    reader: &mut R,
    limit: usize,
) -> io::Result<Option<Vec<u8>>> {
    let mut frame = Vec::new();
    loop {
        let available = reader.fill_buf()?;
        if available.is_empty() {
            return if frame.is_empty() {
                Ok(None)
            } else {
                Err(io::Error::new(
                    io::ErrorKind::UnexpectedEof,
                    "data_engine_request_frame_incomplete",
                ))
            };
        }
        let newline = available.iter().position(|byte| *byte == b'\n');
        let content_bytes = newline.unwrap_or(available.len());
        if content_bytes > limit.saturating_sub(frame.len()) {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "data_engine_request_limit_exceeded",
            ));
        }
        reserve_bounded_bytes(
            &mut frame,
            content_bytes,
            limit,
            "request allocation failed",
        )?;
        frame.extend_from_slice(&available[..content_bytes]);
        let consumed = content_bytes + usize::from(newline.is_some());
        reader.consume(consumed);
        if newline.is_some() {
            if frame.last() == Some(&b'\r') {
                frame.pop();
            }
            return Ok(Some(frame));
        }
    }
}

fn reserve_bounded_bytes(
    buffer: &mut Vec<u8>,
    additional: usize,
    limit: usize,
    error_message: &'static str,
) -> io::Result<()> {
    let required = buffer
        .len()
        .checked_add(additional)
        .filter(|required| *required <= limit)
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidData, error_message))?;
    if required <= buffer.capacity() {
        return Ok(());
    }
    let mut target = buffer.capacity().max(8 * 1024).min(limit);
    while target < required {
        let next = target.saturating_mul(2).min(limit);
        if next <= target {
            target = required;
            break;
        }
        target = next;
    }
    buffer
        .try_reserve_exact(target.saturating_sub(buffer.capacity()))
        .map_err(|_| io::Error::new(io::ErrorKind::OutOfMemory, error_message))
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum JsonShapeError {
    Invalid,
    LimitExceeded,
}

fn validate_request_json_shape(frame: &[u8]) -> std::result::Result<(), JsonShapeError> {
    validate_request_json_shape_with_limits(
        frame,
        MAX_REQUEST_JSON_DEPTH,
        MAX_REQUEST_JSON_NODES,
        MAX_REQUEST_JSON_STRING_BYTES,
    )
}

fn validate_request_json_shape_with_limits(
    frame: &[u8],
    max_depth: usize,
    max_nodes: usize,
    max_string_bytes: usize,
) -> std::result::Result<(), JsonShapeError> {
    let mut depth = 0usize;
    let mut nodes = 0usize;
    let mut in_string = false;
    let mut escaped = false;
    let mut string_bytes = 0usize;

    for byte in frame {
        if in_string {
            if escaped {
                escaped = false;
                string_bytes = string_bytes.saturating_add(1);
            } else if *byte == b'\\' {
                escaped = true;
                string_bytes = string_bytes.saturating_add(1);
            } else if *byte == b'"' {
                in_string = false;
            } else {
                string_bytes = string_bytes.saturating_add(1);
            }
            if string_bytes > max_string_bytes {
                return Err(JsonShapeError::LimitExceeded);
            }
            continue;
        }

        match *byte {
            b'"' => {
                in_string = true;
                escaped = false;
                string_bytes = 0;
                nodes = nodes.saturating_add(1);
            }
            b'{' | b'[' => {
                depth = depth.saturating_add(1);
                nodes = nodes.saturating_add(1);
                if depth > max_depth {
                    return Err(JsonShapeError::LimitExceeded);
                }
            }
            b'}' | b']' => {
                if depth == 0 {
                    return Err(JsonShapeError::Invalid);
                }
                depth -= 1;
            }
            b',' | b':' => nodes = nodes.saturating_add(1),
            byte if byte.is_ascii_whitespace() => {}
            _ => nodes = nodes.saturating_add(1),
        }
        if nodes > max_nodes {
            return Err(JsonShapeError::LimitExceeded);
        }
    }

    if in_string || escaped || depth != 0 {
        return Err(JsonShapeError::Invalid);
    }
    Ok(())
}

struct BoundedResponseBuffer {
    bytes: Vec<u8>,
    limit: usize,
}

impl BoundedResponseBuffer {
    fn new(limit: usize) -> Self {
        Self {
            bytes: Vec::new(),
            limit,
        }
    }
}

impl Write for BoundedResponseBuffer {
    fn write(&mut self, value: &[u8]) -> io::Result<usize> {
        if value.len() > self.limit.saturating_sub(self.bytes.len()) {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "data_engine_response_limit_exceeded",
            ));
        }
        reserve_bounded_bytes(
            &mut self.bytes,
            value.len(),
            self.limit,
            "response allocation failed",
        )?;
        self.bytes.extend_from_slice(value);
        Ok(value.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

enum EngineResponseData {
    Value(Value),
    FundsTransactionSourceRowPage(analytix_analysis_compute::FundsTransactionSourceRowPageV1Result),
}

#[derive(Serialize)]
struct FundsTransactionSourceRowSuccessFrame {
    data: analytix_analysis_compute::FundsTransactionSourceRowPageV1Result,
    diagnostics: Value,
    ok: bool,
    request_id: Value,
}

#[derive(Serialize)]
#[serde(untagged)]
enum EngineResponseFrame {
    Value(Value),
    FundsTransactionSourceRowPage(FundsTransactionSourceRowSuccessFrame),
}

#[cfg(test)]
impl EngineResponseFrame {
    fn into_value(self) -> Value {
        match self {
            Self::Value(value) => value,
            Self::FundsTransactionSourceRowPage(response) => serde_json::to_value(response)
                .unwrap_or_else(|_| fixed_protocol_error("data_engine_response_limit_exceeded")),
        }
    }
}

fn encode_bounded_response<T>(response: &T, limit: usize) -> io::Result<Vec<u8>>
where
    T: Serialize + ?Sized,
{
    let mut output = BoundedResponseBuffer::new(limit);
    serde_json::to_writer(&mut output, response)
        .map_err(|error| io::Error::new(io::ErrorKind::InvalidData, error))?;
    output.write_all(b"\n")?;
    Ok(output.bytes)
}

fn write_bounded_response<W: Write, T: Serialize + ?Sized>(
    writer: &mut W,
    response: &T,
    limit: usize,
) -> io::Result<()> {
    let encoded = match encode_bounded_response(response, limit) {
        Ok(encoded) => encoded,
        Err(error) if error.kind() == io::ErrorKind::InvalidData => encode_bounded_response(
            &fixed_protocol_error("data_engine_response_limit_exceeded"),
            limit,
        )?,
        Err(error) => return Err(error),
    };
    writer.write_all(&encoded)?;
    writer.flush()
}

#[cfg(test)]
fn handle_line(line: &str) -> Value {
    let mut state = EngineState::default();
    handle_line_with_state(&mut state, line)
}

#[cfg(test)]
fn handle_line_with_state(state: &mut EngineState, line: &str) -> Value {
    handle_line_frame_with_state(state, line).into_value()
}

fn handle_line_frame_with_state(state: &mut EngineState, line: &str) -> EngineResponseFrame {
    let started = Instant::now();
    let request = match serde_json::from_str::<EngineRequest>(line) {
        Ok(value) => value,
        Err(error) => {
            let _ = error;
            return EngineResponseFrame::Value(json!({
                "request_id": Value::Null,
                "ok": false,
                "error": {
                    "code": "invalid_request_json",
                    "message": public_error_message("invalid_request_json"),
                },
                "diagnostics": diagnostics("", false, false, started),
            }));
        }
    };
    let request_id = request
        .request_id
        .clone()
        .map(Value::String)
        .unwrap_or(Value::Null);
    let command = request.command.as_str();
    let case_bound = request
        .case_id
        .as_deref()
        .map(str::trim)
        .map(|value| !value.is_empty())
        .unwrap_or(false);
    match handle_request(state, &request) {
        Ok(data) => {
            let db_bound = request.db_path.is_some()
                || matches!(
                    command,
                    "funds.resolve_account_ingress"
                        | "funds.analyze_account_flows"
                        | "funds.direct_source_preview"
                        | "funds.deterministic_cleaning_v1"
                        | "funds.build_canonical_csv_snapshot_v1"
                        | "funds.transaction_source_row_page_v1"
                );
            let diagnostics = diagnostics(command, case_bound, db_bound, started);
            match data {
                EngineResponseData::Value(data) => EngineResponseFrame::Value(json!({
                    "request_id": request_id,
                    "ok": true,
                    "data": data,
                    "diagnostics": diagnostics,
                })),
                EngineResponseData::FundsTransactionSourceRowPage(data) => {
                    EngineResponseFrame::FundsTransactionSourceRowPage(
                        FundsTransactionSourceRowSuccessFrame {
                            data,
                            diagnostics,
                            ok: true,
                            request_id,
                        },
                    )
                }
            }
        }
        Err(error) => {
            let code = classify_error(&error);
            let db_bound = request.db_path.is_some()
                && command != "funds.resolve_account_ingress"
                && command != "funds.analyze_account_flows"
                && command != "funds.direct_source_preview"
                && command != "funds.deterministic_cleaning_v1"
                && command != "funds.transaction_source_row_page_v1";
            EngineResponseFrame::Value(json!({
                "request_id": request_id,
                "ok": false,
                "error": {
                    "code": code,
                    "message": public_error_message(code),
                },
                "diagnostics": diagnostics(command, case_bound, db_bound, started),
            }))
        }
    }
}

fn handle_request(state: &mut EngineState, request: &EngineRequest) -> Result<EngineResponseData> {
    validate_request_contract(request)?;
    let response = match request.command.as_str() {
        "ping" => Ok(json!({
            "pong": true,
            "pid": std::process::id(),
        })),
        "duckdb.open_session" => handle_duckdb_open_session(state, request),
        "duckdb.execute" => handle_duckdb_execute(state, request),
        "duckdb.executemany" => handle_duckdb_executemany(state, request),
        "duckdb.query" => handle_duckdb_query(state, request),
        "duckdb.close_session" => handle_duckdb_close_session(state, request),
        "funds.materialize_txn_daily_v1" => handle_funds_materialize_txn_daily_v1(state, request),
        "funds.build_canonical_csv_snapshot_v1" => {
            handle_funds_build_canonical_csv_snapshot_v1(request)
        }
        "funds.transaction_source_row_page_v1" => {
            return handle_funds_transaction_source_row_page_v1(request)
                .map(EngineResponseData::FundsTransactionSourceRowPage);
        }
        "funds.resolve_account_ingress" => handle_funds_resolve_account_ingress(state, request),
        "funds.analyze_account_flows" => handle_funds_analyze_account_flows(state, request),
        "funds.direct_source_preview" => handle_funds_direct_source_preview(state, request),
        "funds.deterministic_cleaning_v1" => handle_funds_deterministic_cleaning(state, request),
        "analysis.compute" => handle_analysis_compute(state, request),
        "analysis.verify" => handle_analysis_verify(state, request),
        "analysis.stats_query" => handle_analysis_stats_query(state, request),
        "cleaning.clean_all" => handle_cleaning_clean_all(state, request),
        other => Err(anyhow!("unsupported data engine command: {other}")),
    };
    response.map(EngineResponseData::Value)
}

fn validate_request_contract(request: &EngineRequest) -> Result<()> {
    match request.command.as_str() {
        "ping" => {
            if request.case_id.is_some()
                || request.db_path.is_some()
                || request.payload_value().is_some()
            {
                return Err(anyhow!("data_engine_request_contract_invalid"));
            }
        }
        "duckdb.open_session" => {
            require_request_context(request)?;
            require_payload_keys(request, &[], &["read_only"])?;
        }
        "duckdb.execute" => {
            require_request_context(request)?;
            require_payload_keys(request, &["session_id", "sql"], &["params"])?;
        }
        "duckdb.executemany" => {
            require_request_context(request)?;
            require_payload_keys(request, &["session_id", "sql", "rows"], &[])?;
        }
        "duckdb.query" => {
            require_request_context(request)?;
            require_payload_keys(
                request,
                &["session_id", "sql"],
                &["params", "row_limit", "max_result_bytes", "timeout_ms"],
            )?;
        }
        "duckdb.close_session" => {
            require_request_context(request)?;
            require_payload_keys(request, &["session_id"], &[])?;
        }
        "analysis.compute" | "analysis.verify" | "analysis.stats_query" => {
            require_request_context(request)?;
            require_payload_keys(request, &["command", "args"], &[])?;
        }
        "funds.materialize_txn_daily_v1" => {
            require_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "sourceRevision",
                    "sourceRowCount",
                    "sourceMaxTxnTs",
                    "sourceMaxId",
                    "rawArtifactManifestSha256",
                ],
                &[],
            )?;
        }
        "funds.build_canonical_csv_snapshot_v1" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "profile",
                    "privateImportFileId",
                    "sourceRevision",
                    "rawArtifactManifestSha256",
                ],
                &[],
            )?;
        }
        "funds.transaction_source_row_page_v1" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "bindingKeyDigest",
                    "parsedGenerationIdentitySha256",
                    "relation",
                    "maxRows",
                    "cursor",
                ],
                &[],
            )?;
        }
        "funds.resolve_account_ingress" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "datasetSnapshotId",
                    "contextEpoch",
                    "contextDigest",
                    "caseBindingHash",
                    "expectedProducerContentId",
                    "expectedProducerManifestSha256",
                    "candidates",
                ],
                &[],
            )?;
        }
        "funds.analyze_account_flows" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "datasetSnapshotId",
                    "contextEpoch",
                    "contextDigest",
                    "caseBindingHash",
                    "expectedProducerContentId",
                    "expectedProducerManifestSha256",
                    "subjectRef",
                    "resolvedAccountKey",
                    "subjectResolutionDigest",
                    "startInclusive",
                    "endInclusive",
                    "evidenceRowLimit",
                    "datasetUtcOffsetMinutes",
                    "expectedCurrency",
                    "minorUnitScale",
                    "scanCap",
                ],
                &[],
            )?;
        }
        "funds.direct_source_preview" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "datasetSnapshotId",
                    "caseBindingHash",
                    "expectedProducerContentId",
                    "expectedProducerManifestSha256",
                    "fields",
                    "rowOffset",
                    "rowLimit",
                    "datasetUtcOffsetMinutes",
                    "expectedCurrency",
                    "minorUnitScale",
                ],
                &[],
            )?;
        }
        "funds.deterministic_cleaning_v1" => {
            require_inherited_snapshot_request_context(request)?;
            require_payload_keys(
                request,
                &[
                    "caseId",
                    "datasetSnapshotId",
                    "caseBindingHash",
                    "expectedProducerContentId",
                    "expectedProducerManifestSha256",
                    "ruleGeneration",
                    "ruleDigest",
                    "datasetUtcOffsetMinutes",
                    "expectedCurrency",
                    "minorUnitScale",
                ],
                &[],
            )?;
        }
        "cleaning.clean_all" => {
            require_request_context(request)?;
            require_payload_keys(request, &[], &["txn_file_ids", "acc_file_ids"])?;
        }
        _ => {}
    }
    Ok(())
}

fn require_request_context(request: &EngineRequest) -> Result<()> {
    require_case_id(request)?;
    let db_path = request
        .db_path
        .as_ref()
        .filter(|path| path.is_absolute())
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    if db_path.as_os_str().is_empty() {
        return Err(anyhow!("data_engine_request_contract_invalid"));
    }
    Ok(())
}

fn require_inherited_snapshot_request_context(request: &EngineRequest) -> Result<()> {
    require_case_id(request)?;
    if request.db_path.was_supplied() {
        return Err(anyhow!("data_engine_request_contract_invalid"));
    }
    Ok(())
}

fn require_case_id(request: &EngineRequest) -> Result<&str> {
    let case_id = request
        .case_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    if case_id.len() > 512 || case_id.chars().any(char::is_control) {
        return Err(anyhow!("data_engine_request_contract_invalid"));
    }
    Ok(case_id)
}

fn require_payload_keys(
    request: &EngineRequest,
    required: &[&str],
    optional: &[&str],
) -> Result<()> {
    let payload = request
        .payload_value()
        .and_then(Value::as_object)
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    for key in required {
        if !payload.contains_key(*key) {
            return Err(anyhow!("data_engine_request_contract_invalid"));
        }
    }
    if payload
        .keys()
        .any(|key| !required.contains(&key.as_str()) && !optional.contains(&key.as_str()))
    {
        return Err(anyhow!("data_engine_request_contract_invalid"));
    }
    Ok(())
}

fn diagnostics(command: &str, case_bound: bool, db_bound: bool, started: Instant) -> Value {
    let command = match command {
        "ping"
        | "duckdb.open_session"
        | "duckdb.execute"
        | "duckdb.executemany"
        | "duckdb.query"
        | "duckdb.close_session"
        | "funds.materialize_txn_daily_v1"
        | "funds.build_canonical_csv_snapshot_v1"
        | "funds.transaction_source_row_page_v1"
        | "funds.resolve_account_ingress"
        | "funds.analyze_account_flows"
        | "funds.direct_source_preview"
        | "funds.deterministic_cleaning_v1"
        | "analysis.compute"
        | "analysis.verify"
        | "analysis.stats_query"
        | "cleaning.clean_all" => command,
        _ => "unknown",
    };
    json!({
        "engine": "analytix-data-engine",
        "command": command,
        "case_bound": case_bound,
        "db_bound": db_bound,
        "pid": std::process::id(),
        "queue_wait_ms": 0,
        "run_ms": elapsed_ms(started),
        "owner_epoch": "",
    })
}

fn elapsed_ms(started: Instant) -> u64 {
    bounded_elapsed_ms(started.elapsed())
}

fn bounded_elapsed_ms(elapsed: Duration) -> u64 {
    elapsed.as_millis().min(u128::from(MAX_DIAGNOSTIC_RUN_MS)) as u64
}

#[derive(Default)]
struct EngineState {
    next_session_id: u64,
    duckdb_sessions: HashMap<String, DuckDbSession>,
}

struct DuckDbSession {
    case_id: String,
    db_path: PathBuf,
    read_only: bool,
    conn: Connection,
    _guard: OwnerGuard,
}

struct QueryDeadline {
    completed: Option<mpsc::Sender<()>>,
    expired: Arc<AtomicBool>,
}

impl QueryDeadline {
    fn start(conn: &Connection, timeout_ms: u64) -> Self {
        let interrupt = conn.interrupt_handle();
        let (completed, receiver) = mpsc::channel();
        let expired = Arc::new(AtomicBool::new(false));
        let worker_expired = Arc::clone(&expired);
        thread::spawn(move || {
            if receiver
                .recv_timeout(Duration::from_millis(timeout_ms))
                .is_err()
            {
                worker_expired.store(true, Ordering::SeqCst);
                interrupt.interrupt();
            }
        });
        Self {
            completed: Some(completed),
            expired,
        }
    }

    fn expired(&self) -> bool {
        self.expired.load(Ordering::SeqCst)
    }
}

impl Drop for QueryDeadline {
    fn drop(&mut self) {
        if let Some(completed) = self.completed.take() {
            let _ = completed.send(());
        }
    }
}

fn handle_duckdb_open_session(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let case_id = request.case_id.as_deref().unwrap_or("").to_string();
    let db_path = request
        .db_path
        .as_ref()
        .ok_or_else(|| anyhow!("db_path is required"))?;
    let payload = request.payload_value().unwrap_or(&Value::Null);
    let read_only = match payload.get("read_only") {
        None => false,
        Some(Value::Bool(value)) => *value,
        Some(_) => return Err(anyhow!("data_engine_request_contract_invalid")),
    };
    let canonical = canonical_db_path(db_path);
    if state
        .duckdb_sessions
        .values()
        .any(|session| session.db_path == canonical)
    {
        return Err(anyhow!(
            "data_engine_owner_conflict: db_path_hash={}",
            db_path_hash(&canonical)
        ));
    }
    let guard = OwnerGuard::acquire(db_path)?;
    let conn = if read_only {
        Connection::open_with_flags(
            db_path,
            Config::default().access_mode(AccessMode::ReadOnly)?,
        )
    } else {
        Connection::open(db_path)
    }
    .with_context(|| format!("open DuckDB {}", db_path.display()))?;
    state.next_session_id = state.next_session_id.saturating_add(1);
    let session_id = format!(
        "duckdb-session-{}-{}",
        std::process::id(),
        state.next_session_id
    );
    state.duckdb_sessions.insert(
        session_id.clone(),
        DuckDbSession {
            case_id,
            db_path: canonical,
            read_only,
            conn,
            _guard: guard,
        },
    );
    Ok(json!({
        "session_id": session_id,
        "pid": std::process::id(),
    }))
}

fn handle_duckdb_execute(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("duckdb.execute payload is required"))?;
    let sql = payload
        .get("sql")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("duckdb.execute payload.sql is required"))?;
    let params = json_array_to_duck_values(payload.get("params"))?;
    let session = duckdb_session_mut(state, request, payload)?;
    if session.read_only {
        return Err(anyhow!("duckdb_read_only_violation"));
    }
    let affected_rows = session
        .conn
        .execute(sql, params_from_iter(params.iter()))
        .context("execute DuckDB statement")?;
    Ok(json!({
        "affected_rows": affected_rows,
    }))
}

fn handle_duckdb_executemany(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("duckdb.executemany payload is required"))?;
    let sql = payload
        .get("sql")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("duckdb.executemany payload.sql is required"))?;
    let rows = payload
        .get("rows")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("duckdb.executemany payload.rows is required"))?;
    let session = duckdb_session_mut(state, request, payload)?;
    if session.read_only {
        return Err(anyhow!("duckdb_read_only_violation"));
    }
    let mut stmt = session
        .conn
        .prepare(sql)
        .context("prepare DuckDB statement")?;
    let mut affected_rows = 0usize;
    for row in rows {
        let values = json_array_to_duck_values(Some(row))?;
        affected_rows = affected_rows.saturating_add(
            stmt.execute(params_from_iter(values.iter()))
                .context("execute DuckDB statement")?,
        );
    }
    Ok(json!({
        "affected_rows": affected_rows,
        "row_count": rows.len(),
    }))
}

fn handle_duckdb_query(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("duckdb.query payload is required"))?;
    let sql = payload
        .get("sql")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("duckdb.query payload.sql is required"))?;
    let params = json_array_to_duck_values(payload.get("params"))?;
    let session = duckdb_session_mut(state, request, payload)?;
    if session.read_only && !sql_returns_rows(sql) {
        return Err(anyhow!("duckdb_read_only_violation"));
    }
    let row_limit = bounded_payload_u64(payload, "row_limit", 10_000, 1, 100_000)? as usize;
    let max_result_bytes = bounded_payload_u64(
        payload,
        "max_result_bytes",
        3 * 1024 * 1024,
        1_024,
        3 * 1024 * 1024,
    )? as usize;
    let timeout_ms = bounded_payload_u64(payload, "timeout_ms", 30_000, 1, 300_000)?;
    let deadline = QueryDeadline::start(&session.conn, timeout_ms);
    if !sql_returns_rows(sql) {
        let result = session.conn.execute(sql, params_from_iter(params.iter()));
        if deadline.expired() {
            return Err(anyhow!("duckdb_query_timeout"));
        }
        let affected_rows = result.context("execute DuckDB statement")?;
        return Ok(json!({
            "columns": [],
            "rows": [],
            "row_count": 0,
            "affected_rows": affected_rows,
        }));
    }
    let prepare_result = session.conn.prepare(sql);
    if deadline.expired() {
        return Err(anyhow!("duckdb_query_timeout"));
    }
    let mut stmt = prepare_result.context("prepare DuckDB query")?;
    let query_result = stmt.query(params_from_iter(params.iter()));
    if deadline.expired() {
        return Err(anyhow!("duckdb_query_timeout"));
    }
    let mut query_rows = query_result.context("query DuckDB statement")?;
    let columns = query_rows
        .as_ref()
        .map(|statement| statement.column_names())
        .unwrap_or_default();
    let column_count = query_rows
        .as_ref()
        .map(|statement| statement.column_count())
        .unwrap_or(0);
    let mut rows = Vec::new();
    let mut result_bytes = 0usize;
    let mut truncated = false;
    loop {
        let next_row = query_rows.next();
        if deadline.expired() {
            return Err(anyhow!("duckdb_query_timeout"));
        }
        let Some(row) = next_row.context("fetch DuckDB row")? else {
            break;
        };
        if rows.len() >= row_limit {
            truncated = true;
            break;
        }
        let mut values = Vec::with_capacity(column_count);
        for index in 0..column_count {
            values.push(duck_value_ref_to_json(row.get_ref(index)?));
        }
        let row = Value::Array(values);
        result_bytes = result_bytes.saturating_add(
            serde_json::to_vec(&row)
                .map_err(|_| anyhow!("duckdb_result_contract_invalid"))?
                .len(),
        );
        if result_bytes > max_result_bytes {
            return Err(anyhow!("duckdb_result_limit_exceeded"));
        }
        rows.push(row);
    }
    Ok(json!({
        "columns": columns,
        "rows": rows,
        "row_count": rows.len(),
        "truncated": truncated,
        "row_limit": row_limit,
    }))
}

fn handle_duckdb_close_session(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("duckdb.close_session payload is required"))?;
    let session_id = payload
        .get("session_id")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("duckdb.close_session payload.session_id is required"))?;
    let session = state
        .duckdb_sessions
        .get(session_id)
        .ok_or_else(|| anyhow!("duckdb session not found"))?;
    validate_session_binding(session, request)?;
    let closed = state.duckdb_sessions.remove(session_id).is_some();
    Ok(json!({
        "closed": closed,
        "session_id": session_id,
    }))
}

fn close_duckdb_sessions_for_db_path(state: &mut EngineState, db_path: &Path) -> usize {
    let canonical = canonical_db_path(db_path);
    let session_ids = state
        .duckdb_sessions
        .iter()
        .filter_map(|(session_id, session)| {
            if canonical_db_path(&session.db_path) == canonical {
                Some(session_id.clone())
            } else {
                None
            }
        })
        .collect::<Vec<_>>();
    let count = session_ids.len();
    for session_id in session_ids {
        let _ = state.duckdb_sessions.remove(&session_id);
    }
    count
}

fn duckdb_session_mut<'a>(
    state: &'a mut EngineState,
    request: &EngineRequest,
    payload: &Value,
) -> Result<&'a mut DuckDbSession> {
    let session_id = payload
        .get("session_id")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("payload.session_id is required"))?;
    let session = state
        .duckdb_sessions
        .get_mut(session_id)
        .ok_or_else(|| anyhow!("duckdb session not found"))?;
    validate_session_binding(session, request)?;
    Ok(session)
}

fn validate_session_binding(session: &DuckDbSession, request: &EngineRequest) -> Result<()> {
    let case_id = request.case_id.as_deref().map(str::trim).unwrap_or("");
    let db_path = request.db_path.as_ref().map(|path| canonical_db_path(path));
    if case_id != session.case_id || db_path.as_ref() != Some(&session.db_path) {
        return Err(anyhow!("data_engine_context_mismatch"));
    }
    Ok(())
}

fn json_array_to_duck_values(value: Option<&Value>) -> Result<Vec<DuckValue>> {
    let Some(value) = value else {
        return Ok(Vec::new());
    };
    let array = value
        .as_array()
        .ok_or_else(|| anyhow!("expected JSON array parameters"))?;
    array.iter().map(json_to_duck_value).collect()
}

fn json_to_duck_value(value: &Value) -> Result<DuckValue> {
    Ok(match value {
        Value::Null => DuckValue::Null,
        Value::Bool(value) => DuckValue::Boolean(*value),
        Value::Number(value) => {
            if let Some(integer) = value.as_i64() {
                DuckValue::BigInt(integer)
            } else if let Some(unsigned) = value.as_u64() {
                DuckValue::UBigInt(unsigned)
            } else if let Some(float) = value.as_f64() {
                DuckValue::Double(float)
            } else {
                DuckValue::Text(value.to_string())
            }
        }
        Value::String(value) => DuckValue::Text(value.clone()),
        Value::Array(items) => DuckValue::List(
            items
                .iter()
                .map(json_to_duck_value)
                .collect::<Result<Vec<_>>>()?,
        ),
        Value::Object(_) => DuckValue::Text(serde_json::to_string(value)?),
    })
}

fn duck_value_ref_to_json(value: ValueRef<'_>) -> Value {
    match value {
        ValueRef::Null => Value::Null,
        ValueRef::Boolean(value) => Value::Bool(value),
        ValueRef::TinyInt(value) => json!(value),
        ValueRef::SmallInt(value) => json!(value),
        ValueRef::Int(value) => json!(value),
        ValueRef::BigInt(value) => json!(value),
        ValueRef::HugeInt(value) => json!(value.to_string()),
        ValueRef::UTinyInt(value) => json!(value),
        ValueRef::USmallInt(value) => json!(value),
        ValueRef::UInt(value) => json!(value),
        ValueRef::UBigInt(value) => json!(value),
        ValueRef::Float(value) => json!(value),
        ValueRef::Double(value) => json!(value),
        ValueRef::Decimal(value) => json!(value.to_string()),
        ValueRef::Timestamp(_, value) => json!(value),
        ValueRef::Text(value) => json!(String::from_utf8_lossy(value).to_string()),
        ValueRef::Blob(value) => json!(value),
        ValueRef::Date32(value) => json!(value),
        ValueRef::Time64(_, value) => json!(value),
        ValueRef::Interval {
            months,
            days,
            nanos,
        } => json!({
            "months": months,
            "days": days,
            "nanos": nanos,
        }),
        other => json!(format!("{other:?}")),
    }
}

fn bounded_payload_u64(
    payload: &Value,
    key: &str,
    default_value: u64,
    minimum: u64,
    maximum: u64,
) -> Result<u64> {
    if default_value < minimum || default_value > maximum {
        return Err(anyhow!("data_engine_request_contract_invalid"));
    }
    let Some(raw) = payload.get(key) else {
        return Ok(default_value);
    };
    let value = raw
        .as_u64()
        .filter(|value| *value >= minimum && *value <= maximum)
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    Ok(value)
}

fn sql_returns_rows(sql: &str) -> bool {
    let trimmed = sql.trim_start();
    let upper = trimmed.to_ascii_uppercase();
    let first = trimmed
        .split_whitespace()
        .next()
        .unwrap_or("")
        .trim_matches(|ch: char| !ch.is_ascii_alphabetic())
        .to_ascii_uppercase();
    if first == "COPY" && upper.starts_with("COPY FROM DATABASE") {
        return false;
    }
    matches!(
        first.as_str(),
        "SELECT" | "WITH" | "PRAGMA" | "SHOW" | "DESCRIBE" | "DESC" | "EXPLAIN" | "COPY"
    )
}

fn handle_analysis_stats_query(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let (case_id, db_path) = authoritative_analysis_context(request)?;
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("analysis.stats_query payload is required"))?;
    let command = payload
        .get("command")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("analysis.stats_query payload.command is required"))?;
    let args = payload
        .get("args")
        .ok_or_else(|| anyhow!("analysis.stats_query payload.args is required"))?;
    let args = analysis_stats_args_with_context(args, &case_id, &db_path)?;
    analytix_analysis_compute::validate_data_engine_stats_query_args(command, &args)?;
    close_duckdb_sessions_for_db_path(state, &db_path);
    let result = with_owner_file(Some(db_path.as_path()), || {
        analytix_analysis_compute::run_data_engine_stats_query(command, args)
    })?;
    validate_analysis_result_context(result, &case_id)
}

fn handle_analysis_compute(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let (case_id, db_path) = authoritative_analysis_context(request)?;
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("analysis.compute payload is required"))?;
    let command = payload
        .get("command")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("analysis.compute payload.command is required"))?;
    if command == "materialize-txn-daily" {
        return Err(anyhow!(
            "unsupported data engine analysis command: materialize-txn-daily"
        ));
    }
    let args = payload
        .get("args")
        .ok_or_else(|| anyhow!("analysis.compute payload.args is required"))?;
    let args = analysis_compute_args_with_context(args, &case_id, &db_path)?;
    close_duckdb_sessions_for_db_path(state, &db_path);
    let result = with_owner_file(Some(db_path.as_path()), || {
        analytix_analysis_compute::run_data_engine_analysis_command(command, args)
    })?;
    validate_analysis_result_context(result, &case_id)
}

fn handle_funds_materialize_txn_daily_v1(
    state: &mut EngineState,
    request: &EngineRequest,
) -> Result<Value> {
    let (case_id, db_path) = authoritative_analysis_context(request)?;
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let arguments = serde_json::from_value::<
        analytix_analysis_compute::FundsMaterializeTxnDailyV1Arguments,
    >(payload.clone())
    .map_err(|_| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_materialize_txn_daily_v1_arguments(
        &arguments, &case_id,
    )?;
    close_duckdb_sessions_for_db_path(state, &db_path);
    let result = with_owner_file(Some(db_path.as_path()), || {
        analytix_analysis_compute::run_funds_materialize_txn_daily_v1(arguments, &db_path)
    })?;
    if result.case_id != case_id {
        return Err(anyhow!("data_engine_context_mismatch"));
    }
    serde_json::to_value(result).map_err(Into::into)
}

fn handle_funds_build_canonical_csv_snapshot_v1(request: &EngineRequest) -> Result<Value> {
    let case_id = require_case_id(request)?.to_string();
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let arguments = serde_json::from_value::<
        analytix_analysis_compute::FundsBuildCanonicalCSVSnapshotV1Arguments,
    >(payload.clone())
    .map_err(|_| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_build_canonical_csv_snapshot_v1_arguments(
        &arguments, &case_id,
    )?;

    let mut output = CanonicalCSVSnapshotOutputGuard::acquire()?;
    let mut source = inherited_canonical_csv_source_v1()?;
    let result = analytix_analysis_compute::run_funds_build_canonical_csv_snapshot_v1(
        arguments,
        &source,
        &mut output.file,
    );
    source.fill(0);
    let result = result?;
    output.verify_completed()?;
    serde_json::to_value(result)
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_result_contract_invalid"))
}

fn handle_funds_transaction_source_row_page_v1(
    request: &EngineRequest,
) -> Result<analytix_analysis_compute::FundsTransactionSourceRowPageV1Result> {
    let case_id = require_case_id(request)?.to_string();
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("transaction_source_row_request_contract_invalid"))?;
    let arguments = serde_json::from_value::<
        analytix_analysis_compute::FundsTransactionSourceRowPageV1Arguments,
    >(payload.clone())
    .map_err(|_| anyhow!("transaction_source_row_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_transaction_source_row_page_v1_arguments(
        &arguments, &case_id,
    )?;
    let db_path = inherited_account_flow_snapshot_path()
        .map_err(|_| anyhow!("transaction_source_row_snapshot_connection_invalid"))?;
    analytix_analysis_compute::run_funds_transaction_source_row_page_v1(
        arguments, &case_id, &db_path,
    )
}

#[cfg(unix)]
struct CanonicalCSVSnapshotOutputGuard {
    file: File,
    device: u64,
    inode: u64,
}

#[cfg(unix)]
impl CanonicalCSVSnapshotOutputGuard {
    fn acquire() -> Result<Self> {
        // The native process authority transfers sole ownership of the
        // already-unlinked output inode as fixed FD 5. Validate that exact
        // capability and take ownership directly; never reopen a descriptor
        // path in this process or its parent.
        let status_flags = unsafe { libc::fcntl(CANONICAL_CSV_OUTPUT_FD, libc::F_GETFL) };
        let descriptor_flags = unsafe { libc::fcntl(CANONICAL_CSV_OUTPUT_FD, libc::F_GETFD) };
        let mut stat = std::mem::MaybeUninit::<libc::stat>::zeroed();
        if status_flags < 0
            || descriptor_flags < 0
            || status_flags & libc::O_ACCMODE != libc::O_RDWR
            || descriptor_flags & libc::FD_CLOEXEC != 0
            || unsafe { libc::fstat(CANONICAL_CSV_OUTPUT_FD, stat.as_mut_ptr()) } != 0
        {
            bail!("funds_canonical_csv_snapshot_output_invalid");
        }
        // SAFETY: fstat returned success and initialized the complete value.
        let stat = unsafe { stat.assume_init() };
        if stat.st_mode & libc::S_IFMT != libc::S_IFREG
            || stat.st_nlink != 0
            || stat.st_uid != unsafe { libc::geteuid() }
            || stat.st_gid != unsafe { libc::getegid() }
            || stat.st_mode & 0o7777 != 0o600
            || stat.st_size != 0
            || unsafe { libc::lseek(CANONICAL_CSV_OUTPUT_FD, 0, libc::SEEK_SET) } != 0
        {
            bail!("funds_canonical_csv_snapshot_output_invalid");
        }
        // SAFETY: the fixed descriptor passed every invariant above and has
        // one owner after exec. This File closes it exactly once.
        let file = unsafe { File::from_raw_fd(CANONICAL_CSV_OUTPUT_FD) };
        let metadata = file
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
        if !canonical_empty_output_metadata(&metadata) {
            bail!("funds_canonical_csv_snapshot_output_invalid");
        }
        Ok(Self {
            file,
            device: metadata.dev(),
            inode: metadata.ino(),
        })
    }

    fn verify_completed(&self) -> Result<()> {
        self.file
            .sync_all()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
        let metadata = self
            .file
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
        if !canonical_completed_output_metadata(&metadata)
            || metadata.dev() != self.device
            || metadata.ino() != self.inode
        {
            bail!("funds_canonical_csv_snapshot_output_invalid");
        }
        Ok(())
    }
}

#[cfg(unix)]
fn canonical_empty_output_metadata(metadata: &fs::Metadata) -> bool {
    canonical_output_metadata(metadata) && metadata.len() == 0
}

#[cfg(unix)]
fn canonical_completed_output_metadata(metadata: &fs::Metadata) -> bool {
    canonical_output_metadata(metadata) && metadata.len() > 0
}

#[cfg(unix)]
fn canonical_output_metadata(metadata: &fs::Metadata) -> bool {
    metadata.file_type().is_file()
        && metadata.uid() == unsafe { libc::geteuid() }
        && metadata.gid() == unsafe { libc::getegid() }
        && metadata.nlink() == 0
        && metadata.mode() & 0o7777 == 0o600
}

#[cfg(not(unix))]
struct CanonicalCSVSnapshotOutputGuard {
    file: File,
}

#[cfg(not(unix))]
impl CanonicalCSVSnapshotOutputGuard {
    fn acquire() -> Result<Self> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }

    fn verify_completed(&self) -> Result<()> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }
}

#[cfg(unix)]
fn inherited_canonical_csv_source_v1() -> Result<Vec<u8>> {
    let invalid = || anyhow!("funds_canonical_csv_snapshot_inherited_source_invalid");
    let status_flags = unsafe { libc::fcntl(ACCOUNT_FLOW_SNAPSHOT_FD, libc::F_GETFL) };
    let descriptor_flags = unsafe { libc::fcntl(ACCOUNT_FLOW_SNAPSHOT_FD, libc::F_GETFD) };
    let mut stat = std::mem::MaybeUninit::<libc::stat>::zeroed();
    if status_flags < 0
        || descriptor_flags < 0
        || status_flags & libc::O_ACCMODE != libc::O_RDONLY
        || descriptor_flags & libc::FD_CLOEXEC != 0
        || unsafe { libc::fstat(ACCOUNT_FLOW_SNAPSHOT_FD, stat.as_mut_ptr()) } != 0
    {
        return Err(invalid());
    }
    // SAFETY: fstat returned success and initialized the complete value.
    let stat = unsafe { stat.assume_init() };
    if stat.st_mode & libc::S_IFMT != libc::S_IFREG
        || stat.st_nlink != 0
        || stat.st_uid != unsafe { libc::geteuid() }
        || stat.st_gid != unsafe { libc::getegid() }
        || stat.st_mode & 0o7777 != 0o400
        || stat.st_size <= 0
        || stat.st_size as u64
            > analytix_analysis_compute::FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1 as u64
        || unsafe { libc::lseek(ACCOUNT_FLOW_SNAPSHOT_FD, 0, libc::SEEK_SET) } != 0
    {
        return Err(invalid());
    }

    let duplicate = unsafe { libc::fcntl(ACCOUNT_FLOW_SNAPSHOT_FD, libc::F_DUPFD_CLOEXEC, 6) };
    if duplicate < 6 {
        if duplicate >= 0 {
            let _ = unsafe { libc::close(duplicate) };
        }
        return Err(invalid());
    }
    // SAFETY: fcntl returned a new owned descriptor.
    let file = unsafe { File::from_raw_fd(duplicate) };
    let mut bounded =
        file.take(analytix_analysis_compute::FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1 as u64 + 1);
    let mut source = Vec::new();
    source
        .try_reserve_exact(stat.st_size as usize)
        .map_err(|_| invalid())?;
    bounded.read_to_end(&mut source).map_err(|_| invalid())?;
    let after = bounded.get_ref().metadata().map_err(|_| invalid())?;
    if source.len() != stat.st_size as usize
        || source.len() > analytix_analysis_compute::FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1
        || after.dev() != stat.st_dev as u64
        || after.ino() != stat.st_ino as u64
        || after.nlink() != 0
        || after.uid() != unsafe { libc::geteuid() }
        || after.gid() != unsafe { libc::getegid() }
        || after.mode() & 0o7777 != 0o400
        || after.len() != stat.st_size as u64
    {
        source.fill(0);
        return Err(invalid());
    }
    Ok(source)
}

#[cfg(not(unix))]
fn inherited_canonical_csv_source_v1() -> Result<Vec<u8>> {
    Err(anyhow!(
        "funds_canonical_csv_snapshot_inherited_source_unavailable"
    ))
}

fn handle_funds_analyze_account_flows(
    _state: &mut EngineState,
    request: &EngineRequest,
) -> Result<Value> {
    let case_id = require_case_id(request)?.to_string();
    let arguments = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_analyze_account_flows_arguments(arguments, &case_id)?;
    let db_path = inherited_account_flow_snapshot_path()?;
    analytix_analysis_compute::run_funds_analyze_account_flows(arguments, &case_id, &db_path)
}

fn handle_funds_resolve_account_ingress(
    _state: &mut EngineState,
    request: &EngineRequest,
) -> Result<Value> {
    let case_id = require_case_id(request)?.to_string();
    let arguments = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_resolve_account_ingress_arguments(
        arguments, &case_id,
    )?;
    let db_path = inherited_account_flow_snapshot_path()
        .map_err(|_| anyhow!("account_ingress_snapshot_connection_invalid"))?;
    analytix_analysis_compute::run_funds_resolve_account_ingress(arguments, &case_id, &db_path)
}

fn handle_funds_direct_source_preview(
    _state: &mut EngineState,
    request: &EngineRequest,
) -> Result<Value> {
    let case_id = require_case_id(request)?.to_string();
    let arguments = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_direct_source_preview_arguments(arguments, &case_id)?;
    let db_path = inherited_account_flow_snapshot_path()
        .map_err(|_| anyhow!("direct_source_preview_snapshot_connection_invalid"))?;
    analytix_analysis_compute::run_funds_direct_source_preview(arguments, &case_id, &db_path)
}

fn handle_funds_deterministic_cleaning(
    _state: &mut EngineState,
    request: &EngineRequest,
) -> Result<Value> {
    let case_id = require_case_id(request)?.to_string();
    let arguments = request
        .payload_value()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    analytix_analysis_compute::validate_funds_deterministic_cleaning_arguments(
        arguments, &case_id,
    )?;
    let db_path = inherited_account_flow_snapshot_path()
        .map_err(|_| anyhow!("deterministic_cleaning_snapshot_connection_invalid"))?;
    let mut output = CanonicalCSVSnapshotOutputGuard::acquire()?;
    let result = analytix_analysis_compute::run_funds_deterministic_cleaning(
        arguments,
        &case_id,
        &db_path,
        &mut output.file,
    )?;
    output.verify_completed()?;
    Ok(result)
}

#[cfg(unix)]
fn inherited_account_flow_snapshot_path() -> Result<PathBuf> {
    // SAFETY: fcntl, fstat, geteuid, and lseek only inspect the fixed inherited
    // descriptor. No File is constructed, so this process never assumes or
    // transfers ownership of the host's descriptor.
    let flags = unsafe { libc::fcntl(ACCOUNT_FLOW_SNAPSHOT_FD, libc::F_GETFL) };
    if flags < 0 || flags & libc::O_ACCMODE != libc::O_RDONLY {
        return Err(anyhow!("account_flow_inherited_snapshot_invalid"));
    }

    let mut metadata = std::mem::MaybeUninit::<libc::stat>::zeroed();
    if unsafe { libc::fstat(ACCOUNT_FLOW_SNAPSHOT_FD, metadata.as_mut_ptr()) } != 0 {
        return Err(anyhow!("account_flow_inherited_snapshot_invalid"));
    }
    // SAFETY: fstat returned success and initialized the complete stat value.
    let metadata = unsafe { metadata.assume_init() };
    if metadata.st_mode & libc::S_IFMT != libc::S_IFREG
        || metadata.st_nlink != 0
        || metadata.st_uid != unsafe { libc::geteuid() }
        || metadata.st_mode & 0o7777 != 0o400
        || metadata.st_size <= 0
    {
        return Err(anyhow!("account_flow_inherited_snapshot_invalid"));
    }

    if unsafe { libc::lseek(ACCOUNT_FLOW_SNAPSHOT_FD, 0, libc::SEEK_CUR) } < 0 {
        return Err(anyhow!("account_flow_inherited_snapshot_invalid"));
    }

    Ok(PathBuf::from(ACCOUNT_FLOW_SNAPSHOT_FD_PATH))
}

#[cfg(not(unix))]
fn inherited_account_flow_snapshot_path() -> Result<PathBuf> {
    Err(anyhow!("account_flow_inherited_snapshot_unavailable"))
}

fn handle_analysis_verify(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let (case_id, db_path) = authoritative_analysis_context(request)?;
    let payload = request
        .payload_value()
        .ok_or_else(|| anyhow!("analysis.verify payload is required"))?;
    let command = payload
        .get("command")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("analysis.verify payload.command is required"))?;
    let args = payload
        .get("args")
        .ok_or_else(|| anyhow!("analysis.verify payload.args is required"))?;
    let args = analysis_compute_args_with_context(args, &case_id, &db_path)?;
    if has_duckdb_session_for_db_path(state, &db_path) {
        return Err(anyhow!(
            "data_engine_owner_conflict: db_path_hash={}",
            db_path_hash(&db_path)
        ));
    }
    let result = analytix_analysis_compute::run_data_engine_analysis_verify_command(command, args)?;
    validate_analysis_result_context(result, &case_id)
}

fn authoritative_analysis_context(request: &EngineRequest) -> Result<(String, PathBuf)> {
    let case_id = request
        .case_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?
        .to_string();
    let db_path = request
        .db_path
        .as_ref()
        .filter(|path| path.is_absolute())
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    Ok((case_id, canonical_db_path(db_path)))
}

fn analysis_compute_args_with_context(
    args: &Value,
    case_id: &str,
    db_path: &Path,
) -> Result<Value> {
    let object = args
        .as_object()
        .filter(|object| object.len() == 1 && object.contains_key("argv"))
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let items = object
        .get("argv")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let db_path = db_path
        .to_str()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let mut argv = Vec::with_capacity(items.len().saturating_add(4));
    argv.push(Value::String("--case-id".to_string()));
    argv.push(Value::String(case_id.to_string()));
    argv.push(Value::String("--db-path".to_string()));
    argv.push(Value::String(db_path.to_string()));
    for item in items {
        let text = item
            .as_str()
            .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
        if is_inner_context_flag(text) {
            return Err(anyhow!("data_engine_context_mismatch"));
        }
        argv.push(Value::String(text.to_string()));
    }
    Ok(json!({"argv": argv}))
}

fn analysis_stats_args_with_context(args: &Value, case_id: &str, db_path: &Path) -> Result<Value> {
    let mut object = args
        .as_object()
        .cloned()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    if object.keys().any(|key| is_inner_context_key(key)) {
        return Err(anyhow!("data_engine_context_mismatch"));
    }
    let db_path = db_path
        .to_str()
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    object.insert("case_id".to_string(), Value::String(case_id.to_string()));
    object.insert("db_path".to_string(), Value::String(db_path.to_string()));
    Ok(Value::Object(object))
}

fn is_inner_context_flag(value: &str) -> bool {
    matches!(value, "--case-id" | "--db-path")
        || value.starts_with("--case-id=")
        || value.starts_with("--db-path=")
}

fn is_inner_context_key(value: &str) -> bool {
    let normalized = value
        .chars()
        .filter(|character| !matches!(character, '_' | '-'))
        .flat_map(char::to_lowercase)
        .collect::<String>();
    matches!(normalized.as_str(), "caseid" | "dbpath")
}

fn has_duckdb_session_for_db_path(state: &EngineState, db_path: &Path) -> bool {
    let canonical = canonical_db_path(db_path);
    state
        .duckdb_sessions
        .values()
        .any(|session| canonical_db_path(&session.db_path) == canonical)
}

fn validate_analysis_result_context(result: Value, case_id: &str) -> Result<Value> {
    if result.get("ok") != Some(&Value::Bool(true))
        || result.get("case_id").and_then(Value::as_str) != Some(case_id)
    {
        return Err(anyhow!("data_engine_context_mismatch"));
    }
    Ok(result)
}

fn handle_cleaning_clean_all(state: &mut EngineState, request: &EngineRequest) -> Result<Value> {
    let case_id = request
        .case_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("case_id is required"))?;
    let db_path = request
        .db_path
        .as_ref()
        .ok_or_else(|| anyhow!("db_path is required"))?;
    let payload = request.payload_value().unwrap_or(&Value::Null);
    let txn_file_ids = string_array(payload.get("txn_file_ids"));
    let acc_file_ids = string_array(payload.get("acc_file_ids"));
    close_duckdb_sessions_for_db_path(state, db_path);
    with_owner_file(Some(db_path.as_path()), || {
        analytix_cleaning_ops::run_data_engine_clean_all(
            case_id,
            db_path,
            &txn_file_ids,
            &acc_file_ids,
        )
    })
}

fn string_array(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
                .collect()
        })
        .unwrap_or_default()
}

fn with_owner_file<T, F>(db_path: Option<&Path>, run: F) -> Result<T>
where
    F: FnOnce() -> Result<T>,
{
    let _guard = db_path.map(OwnerGuard::acquire).transpose()?;
    let result = run();
    result
}

#[derive(Debug)]
struct OwnerGuard {
    owner_path: PathBuf,
    owner: OwnerRecord,
    _mutex: DbOwnerMutex,
}

impl OwnerGuard {
    fn acquire(db_path: &Path) -> Result<Self> {
        let canonical = canonical_db_path(db_path);
        let owner_path = owner_file_path(&canonical);
        let db_path_hash = db_path_hash(&canonical);
        let mutex = DbOwnerMutex::acquire(&canonical, &db_path_hash, &owner_path)?;
        let owner = create_owner_file(&owner_path, &db_path_hash)?;
        Ok(Self {
            owner_path,
            owner,
            _mutex: mutex,
        })
    }
}

impl Drop for OwnerGuard {
    fn drop(&mut self) {
        remove_owner_file_if_owned(&self.owner_path, &self.owner);
    }
}

fn canonical_db_path(db_path: &Path) -> PathBuf {
    if let Ok(path) = fs::canonicalize(db_path) {
        return path;
    }
    let Some(parent) = db_path.parent() else {
        return db_path.to_path_buf();
    };
    let Some(name) = db_path.file_name() else {
        return db_path.to_path_buf();
    };
    fs::canonicalize(parent)
        .map(|canonical_parent| canonical_parent.join(name))
        .unwrap_or_else(|_| db_path.to_path_buf())
}

fn owner_file_path(db_path: &Path) -> PathBuf {
    let file_name = db_path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("case.duckdb");
    db_path.with_file_name(format!("{file_name}.owner.json"))
}

fn owner_lock_file_path(db_path: &Path) -> PathBuf {
    let file_name = db_path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("case.duckdb");
    db_path.with_file_name(format!("{file_name}.owner.lock"))
}

fn db_path_hash(db_path: &Path) -> String {
    let mut hasher = Sha256::new();
    update_path_digest(&mut hasher, db_path);
    format!("{:x}", hasher.finalize())
}

#[cfg(unix)]
fn update_path_digest(hasher: &mut Sha256, db_path: &Path) {
    use std::os::unix::ffi::OsStrExt;

    hasher.update(db_path.as_os_str().as_bytes());
}

#[cfg(windows)]
fn update_path_digest(hasher: &mut Sha256, db_path: &Path) {
    use std::os::windows::ffi::OsStrExt;

    for unit in db_path.as_os_str().encode_wide() {
        hasher.update(unit.to_le_bytes());
    }
}

#[cfg_attr(not(any(windows, test)), allow(dead_code))]
fn owner_mutex_name(db_path_hash: &str) -> String {
    format!("Local\\AnalytixDuckDB_{db_path_hash}")
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
struct OwnerRecord {
    version: u32,
    pid: u32,
    db_path_hash: String,
    started_at_ms: u64,
    owner_epoch: String,
}

fn create_owner_file(path: &Path, db_path_hash: &str) -> Result<OwnerRecord> {
    if !valid_sha256_hex(db_path_hash) {
        return Err(anyhow!("data_engine_owner_contract_invalid"));
    }
    reclaim_known_stale_owner(path, db_path_hash)?;
    let owner = new_owner_record(db_path_hash);
    let encoded = serde_json::to_vec(&owner).context("serialize owner file")?;
    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;

        options
            .mode(0o600)
            .custom_flags(libc::O_CLOEXEC | libc::O_NOFOLLOW);
    }
    let mut file = options
        .open(path)
        .map_err(|error| owner_create_error(error, db_path_hash))?;
    validate_open_owner_file(path, &file)
        .map_err(|_| anyhow!("data_engine_owner_sidecar_write_failed"))?;
    if let Err(error) = file.write_all(&encoded).and_then(|()| file.sync_all()) {
        let identity = validate_open_owner_file(path, &file).ok();
        drop(file);
        if let Some(identity) = identity {
            let _ = remove_owner_path_if_identity(path, identity);
        }
        return Err(anyhow!("data_engine_owner_sidecar_write_failed: {error}"));
    }
    let identity = validate_open_owner_file(path, &file)
        .map_err(|_| anyhow!("data_engine_owner_sidecar_write_failed"))?;
    drop(file);
    if sync_owner_parent(path).is_err() {
        let _ = remove_owner_path_if_identity(path, identity);
        return Err(anyhow!("data_engine_owner_sidecar_write_failed"));
    }
    let (persisted, _) = read_owner_record_snapshot(path)
        .map_err(|_| anyhow!("data_engine_owner_sidecar_write_failed"))?;
    if persisted != owner {
        let _ = remove_owner_path_if_identity(path, identity);
        return Err(anyhow!("data_engine_owner_sidecar_write_failed"));
    }
    Ok(owner)
}

fn new_owner_record(db_path_hash: &str) -> OwnerRecord {
    let started_at = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    let started_at_ms = u64::try_from(started_at.as_millis()).unwrap_or(u64::MAX);
    let counter = OWNER_EPOCH_COUNTER.fetch_add(1, Ordering::Relaxed);
    let mut epoch = Sha256::new();
    epoch.update(db_path_hash.as_bytes());
    epoch.update(std::process::id().to_le_bytes());
    epoch.update(started_at.as_nanos().to_le_bytes());
    epoch.update(counter.to_le_bytes());
    OwnerRecord {
        version: OWNER_RECORD_VERSION,
        pid: std::process::id(),
        db_path_hash: db_path_hash.to_string(),
        started_at_ms,
        owner_epoch: format!("{:x}", epoch.finalize()),
    }
}

fn reclaim_known_stale_owner(path: &Path, db_path_hash: &str) -> Result<()> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(error) => {
            return Err(anyhow!(
                "data_engine_owner_sidecar_inspection_failed: {error}"
            ))
        }
    };
    if !metadata.file_type().is_file() || metadata.file_type().is_symlink() {
        return Err(owner_conflict(db_path_hash, "owner_file_invalid_type"));
    }
    let (owner, identity) = read_owner_record_snapshot(path)
        .map_err(|_| owner_conflict(db_path_hash, "owner_file_invalid"))?;
    validate_owner_record(&owner, db_path_hash)
        .map_err(|_| owner_conflict(db_path_hash, "owner_file_invalid"))?;
    let process_live = process_exists(owner.pid)
        .map_err(|_| owner_conflict(db_path_hash, "owner_process_state_unknown"))?;
    if process_live {
        return Err(owner_conflict(db_path_hash, "owner_process_live"));
    }
    let (current, current_identity) = read_owner_record_snapshot(path)
        .map_err(|_| owner_conflict(db_path_hash, "owner_file_changed"))?;
    if current != owner || current_identity != identity {
        return Err(owner_conflict(db_path_hash, "owner_file_changed"));
    }
    fs::remove_file(path).map_err(|error| {
        anyhow!("data_engine_owner_stale_cleanup_failed: db_path_hash={db_path_hash} error={error}")
    })?;
    sync_owner_parent(path).map_err(|_| {
        anyhow!("data_engine_owner_stale_cleanup_failed: db_path_hash={db_path_hash}")
    })?;
    Ok(())
}

#[cfg_attr(not(any(windows, test)), allow(dead_code))]
fn read_owner_record(path: &Path) -> Result<OwnerRecord> {
    read_owner_record_snapshot(path).map(|(owner, _)| owner)
}

#[cfg(unix)]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct OwnerFileIdentity {
    device: u64,
    inode: u64,
    mode: u32,
    uid: u32,
    links: u64,
    size: u64,
    modified_seconds: i64,
    modified_nanoseconds: i64,
}

#[cfg(not(unix))]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct OwnerFileIdentity {
    size: u64,
}

#[cfg(unix)]
fn owner_file_identity(metadata: &fs::Metadata) -> Result<OwnerFileIdentity> {
    use std::os::unix::fs::MetadataExt;

    let identity = OwnerFileIdentity {
        device: metadata.dev(),
        inode: metadata.ino(),
        mode: metadata.mode(),
        uid: metadata.uid(),
        links: metadata.nlink(),
        size: metadata.size(),
        modified_seconds: metadata.mtime(),
        modified_nanoseconds: metadata.mtime_nsec(),
    };
    if !metadata.file_type().is_file()
        || metadata.file_type().is_symlink()
        || identity.device == 0
        || identity.inode == 0
        || identity.uid != unsafe { libc::geteuid() }
        || identity.mode & 0o7777 != 0o600
        || identity.links != 1
        || identity.size > MAX_OWNER_RECORD_BYTES as u64
    {
        return Err(anyhow!("data_engine_owner_file_identity_invalid"));
    }
    Ok(identity)
}

#[cfg(not(unix))]
fn owner_file_identity(metadata: &fs::Metadata) -> Result<OwnerFileIdentity> {
    if !metadata.file_type().is_file() || metadata.len() > MAX_OWNER_RECORD_BYTES as u64 {
        return Err(anyhow!("data_engine_owner_file_identity_invalid"));
    }
    Ok(OwnerFileIdentity {
        size: metadata.len(),
    })
}

fn validate_open_owner_file(path: &Path, file: &File) -> Result<OwnerFileIdentity> {
    let opened = owner_file_identity(&file.metadata().context("inspect owner file")?)?;
    let linked = owner_file_identity(&fs::symlink_metadata(path).context("inspect owner path")?)?;
    if opened != linked {
        return Err(anyhow!("data_engine_owner_file_identity_changed"));
    }
    Ok(opened)
}

fn read_owner_record_snapshot(path: &Path) -> Result<(OwnerRecord, OwnerFileIdentity)> {
    let mut options = OpenOptions::new();
    options.read(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;

        options.custom_flags(libc::O_CLOEXEC | libc::O_NOFOLLOW);
    }
    let mut file = options.open(path).context("read owner file")?;
    let identity = validate_open_owner_file(path, &file)?;
    let mut payload = Vec::with_capacity((identity.size as usize).min(MAX_OWNER_RECORD_BYTES));
    Read::by_ref(&mut file)
        .take((MAX_OWNER_RECORD_BYTES + 1) as u64)
        .read_to_end(&mut payload)
        .context("read owner file")?;
    if payload.len() > MAX_OWNER_RECORD_BYTES || payload.len() as u64 != identity.size {
        return Err(anyhow!("data_engine_owner_file_limit_exceeded"));
    }
    let after = validate_open_owner_file(path, &file)?;
    if after != identity {
        return Err(anyhow!("data_engine_owner_file_identity_changed"));
    }
    let owner: OwnerRecord = serde_json::from_slice(&payload).context("parse owner file")?;
    if serde_json::to_vec(&owner).context("serialize owner file")? != payload {
        return Err(anyhow!("data_engine_owner_file_encoding_invalid"));
    }
    Ok((owner, identity))
}

#[cfg(unix)]
fn sync_owner_parent(path: &Path) -> Result<()> {
    use std::os::fd::AsRawFd;
    use std::os::unix::fs::OpenOptionsExt;

    let parent = path
        .parent()
        .ok_or_else(|| anyhow!("data_engine_owner_parent_invalid"))?;
    let directory = OpenOptions::new()
        .read(true)
        .custom_flags(libc::O_CLOEXEC | libc::O_DIRECTORY | libc::O_NOFOLLOW)
        .open(parent)
        .context("open owner parent")?;
    if unsafe { libc::fsync(directory.as_raw_fd()) } != 0 {
        return Err(anyhow!("data_engine_owner_parent_sync_failed"));
    }
    Ok(())
}

#[cfg(not(unix))]
fn sync_owner_parent(_path: &Path) -> Result<()> {
    Ok(())
}

fn validate_owner_record(owner: &OwnerRecord, db_path_hash: &str) -> Result<()> {
    if owner.version != OWNER_RECORD_VERSION
        || owner.pid == 0
        || owner.started_at_ms == 0
        || owner.db_path_hash != db_path_hash
        || !valid_sha256_hex(&owner.db_path_hash)
        || !valid_sha256_hex(&owner.owner_epoch)
    {
        return Err(anyhow!("data_engine_owner_contract_invalid"));
    }
    #[cfg(unix)]
    if owner.pid > i32::MAX as u32 {
        return Err(anyhow!("data_engine_owner_contract_invalid"));
    }
    Ok(())
}

fn valid_sha256_hex(value: &str) -> bool {
    value.len() == SHA256_HEX_BYTES
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn owner_create_error(error: io::Error, db_path_hash: &str) -> anyhow::Error {
    if error.kind() == io::ErrorKind::AlreadyExists {
        owner_conflict(db_path_hash, "owner_file_exists")
    } else {
        anyhow!("data_engine_owner_sidecar_create_failed: {error}")
    }
}

fn owner_conflict(db_path_hash: &str, reason: &str) -> anyhow::Error {
    anyhow!("data_engine_owner_conflict: db_path_hash={db_path_hash} owner_state={reason}")
}

fn remove_owner_file_if_owned(path: &Path, expected: &OwnerRecord) {
    let Ok((owner, identity)) = read_owner_record_snapshot(path) else {
        return;
    };
    if owner == *expected {
        let _ = remove_owner_path_if_identity(path, identity);
    }
}

fn validate_open_owner_path_identity(path: &Path, expected: OwnerFileIdentity) -> Result<()> {
    let current = owner_file_identity(&fs::symlink_metadata(path).context("inspect owner path")?)?;
    if current != expected {
        return Err(anyhow!("data_engine_owner_file_identity_changed"));
    }
    Ok(())
}

fn remove_owner_path_if_identity(path: &Path, expected: OwnerFileIdentity) -> Result<()> {
    validate_open_owner_path_identity(path, expected)?;
    fs::remove_file(path).context("remove owner file")?;
    sync_owner_parent(path)
}

#[cfg(windows)]
fn owner_file_summary(path: &Path) -> String {
    let Ok(owner) = read_owner_record(path) else {
        return "owner_file=invalid_or_missing".to_string();
    };
    json!({
        "version": owner.version,
        "pid": owner.pid,
        "db_path_hash": owner.db_path_hash,
        "owner_epoch": owner.owner_epoch,
    })
    .to_string()
}

#[cfg(unix)]
fn process_exists(pid: u32) -> Result<bool> {
    if pid == 0 || pid > i32::MAX as u32 {
        return Err(anyhow!("data_engine_owner_pid_invalid"));
    }
    // SAFETY: signal 0 performs a liveness/permission probe without delivering
    // a signal. The PID is range-checked before conversion to pid_t.
    if unsafe { libc::kill(pid as libc::pid_t, 0) } == 0 {
        return Ok(true);
    }
    match io::Error::last_os_error().raw_os_error() {
        Some(libc::ESRCH) => Ok(false),
        Some(libc::EPERM) => Ok(true),
        Some(error) => Err(anyhow!("data_engine_owner_pid_probe_failed: errno={error}")),
        None => Err(anyhow!("data_engine_owner_pid_probe_failed")),
    }
}

#[cfg(windows)]
fn process_exists(pid: u32) -> Result<bool> {
    use windows_sys::Win32::Foundation::{
        CloseHandle, GetLastError, ERROR_ACCESS_DENIED, ERROR_INVALID_PARAMETER,
    };
    use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION};

    if pid == 0 {
        return Err(anyhow!("data_engine_owner_pid_invalid"));
    }
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if !handle.is_null() {
            CloseHandle(handle);
            return Ok(true);
        }
        match GetLastError() {
            ERROR_INVALID_PARAMETER => Ok(false),
            ERROR_ACCESS_DENIED => Ok(true),
            error => Err(anyhow!("data_engine_owner_pid_probe_failed: error={error}")),
        }
    }
}

#[cfg(unix)]
#[derive(Debug)]
struct DbOwnerMutex {
    file: File,
}

#[cfg(unix)]
impl DbOwnerMutex {
    fn acquire(db_path: &Path, db_path_hash: &str, _owner_path: &Path) -> Result<Self> {
        use std::os::fd::AsRawFd;
        use std::os::unix::fs::OpenOptionsExt;

        let lock_path = owner_lock_file_path(db_path);
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .mode(0o600)
            .custom_flags(libc::O_CLOEXEC | libc::O_NOFOLLOW)
            .open(&lock_path)
            .map_err(|error| {
                anyhow!(
                    "data_engine_owner_lock_open_failed: db_path_hash={db_path_hash} error={error}"
                )
            })?;
        if !file
            .metadata()
            .map_err(|error| anyhow!("data_engine_owner_lock_inspection_failed: {error}"))?
            .file_type()
            .is_file()
        {
            return Err(owner_conflict(db_path_hash, "lock_file_invalid_type"));
        }
        // SAFETY: file owns a live descriptor for a regular lock file. LOCK_NB
        // makes ownership admission deterministic and never waits on another
        // process.
        if unsafe { libc::flock(file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) } != 0 {
            let error = io::Error::last_os_error();
            let errno = error.raw_os_error();
            if errno == Some(libc::EWOULDBLOCK) || errno == Some(libc::EAGAIN) {
                return Err(owner_conflict(db_path_hash, "advisory_lock_held"));
            }
            return Err(anyhow!(
                "data_engine_owner_lock_failed: db_path_hash={db_path_hash} error={error}"
            ));
        }
        validate_open_owner_file(&lock_path, &file)
            .map_err(|_| owner_conflict(db_path_hash, "lock_file_invalid"))?;
        Ok(Self { file })
    }
}

#[cfg(unix)]
impl Drop for DbOwnerMutex {
    fn drop(&mut self) {
        use std::os::fd::AsRawFd;

        // SAFETY: this descriptor remains owned by self until after drop.
        let _ = unsafe { libc::flock(self.file.as_raw_fd(), libc::LOCK_UN) };
    }
}

#[cfg(windows)]
#[derive(Debug)]
struct DbOwnerMutex {
    handle: windows_sys::Win32::Foundation::HANDLE,
    acquired: bool,
}

#[cfg(windows)]
impl DbOwnerMutex {
    fn acquire(_db_path: &Path, db_path_hash: &str, owner_path: &Path) -> Result<Self> {
        use std::os::windows::ffi::OsStrExt;
        use windows_sys::Win32::Foundation::{
            CloseHandle, GetLastError, WAIT_ABANDONED, WAIT_OBJECT_0, WAIT_TIMEOUT,
        };
        use windows_sys::Win32::System::Threading::{CreateMutexW, WaitForSingleObject};

        let mutex_name = owner_mutex_name(db_path_hash);
        let wide_name = std::ffi::OsStr::new(&mutex_name)
            .encode_wide()
            .chain(std::iter::once(0))
            .collect::<Vec<u16>>();
        unsafe {
            let handle = CreateMutexW(std::ptr::null(), 0, wide_name.as_ptr());
            if handle.is_null() {
                return Err(anyhow!(
                    "create data engine owner mutex failed: name={mutex_name} error={}",
                    GetLastError()
                ));
            }
            let wait = WaitForSingleObject(handle, 0);
            if wait == WAIT_OBJECT_0 || wait == WAIT_ABANDONED {
                return Ok(Self {
                    handle,
                    acquired: true,
                });
            }
            if wait == WAIT_TIMEOUT {
                CloseHandle(handle);
                return Err(anyhow!(
                    "data_engine_owner_conflict: db_path_hash={} owner={}",
                    db_path_hash,
                    owner_file_summary(owner_path)
                ));
            }
            let error = GetLastError();
            CloseHandle(handle);
            Err(anyhow!(
                "data engine owner mutex wait failed: db_path_hash={} name={} wait={} error={}",
                db_path_hash,
                mutex_name,
                wait,
                error
            ))
        }
    }
}

#[cfg(windows)]
impl Drop for DbOwnerMutex {
    fn drop(&mut self) {
        use windows_sys::Win32::Foundation::CloseHandle;
        use windows_sys::Win32::System::Threading::ReleaseMutex;

        unsafe {
            if self.acquired {
                let _ = ReleaseMutex(self.handle);
            }
            CloseHandle(self.handle);
        }
    }
}

fn classify_error(error: &anyhow::Error) -> &'static str {
    let message = format!("{error:#}").to_lowercase();
    if message.contains("duckdb_read_only_violation") {
        "duckdb_read_only_violation"
    } else if message.contains("data_engine_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("data_engine_context_mismatch") {
        "data_engine_context_mismatch"
    } else if message.contains("direct_source_preview_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("direct_source_preview_") {
        "funds_direct_source_preview_failed"
    } else if message.contains("deterministic_cleaning_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("deterministic_cleaning_") {
        "funds_deterministic_cleaning_failed"
    } else if message.contains("transaction_source_row_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("transaction_source_row_") {
        "funds_transaction_source_row_failed"
    } else if message.contains("account_ingress_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("account_ingress_") {
        "funds_account_flow_failed"
    } else if message.contains("account_flow_request_contract_invalid") {
        "data_engine_request_contract_invalid"
    } else if message.contains("account_flow_") {
        "funds_account_flow_failed"
    } else if message.contains("duckdb_query_timeout") {
        "duckdb_query_timeout"
    } else if message.contains("duckdb_result_limit_exceeded") {
        "duckdb_result_limit_exceeded"
    } else if message.contains("duckdb_result_contract_invalid") {
        "duckdb_result_contract_invalid"
    } else if message.contains("data_engine_owner_conflict") {
        "data_engine_owner_conflict"
    } else if message.contains("cannot open file")
        || message.contains("another process")
        || message.contains("another program is using this file")
        || message.contains("being used by another process")
        || message.contains("could not set lock on file")
        || message.contains("conflicting lock is held")
        || message.contains("另一个程序正在使用此文件")
        || message.contains("进程无法访问")
    {
        "external_process_holds_duckdb"
    } else if message.contains("unsupported data engine command")
        || message.contains("unsupported data engine analysis command")
        || message.contains("unsupported data engine analysis verify command")
    {
        "data_engine_unsupported_command"
    } else {
        "data_engine_error"
    }
}

fn public_error_message(code: &str) -> &'static str {
    match code {
        "invalid_request_json" => "The data engine request was not valid JSON.",
        "data_engine_request_contract_invalid" => {
            "The data engine request did not satisfy the command contract."
        }
        "data_engine_context_mismatch" => {
            "The data engine request did not match the admitted case context."
        }
        "data_engine_request_limit_exceeded" => {
            "The data engine request exceeded the host resource limit."
        }
        "data_engine_response_limit_exceeded" => {
            "The data engine response exceeded the host resource limit."
        }
        "duckdb_read_only_violation" => {
            "The DuckDB operation exceeded the session's read-only authority."
        }
        "duckdb_query_timeout" => "The DuckDB query exceeded the host timeout.",
        "duckdb_result_limit_exceeded" => "The DuckDB result exceeded the host resource limit.",
        "duckdb_result_contract_invalid" => "The DuckDB result did not satisfy the host contract.",
        "data_engine_owner_conflict" | "external_process_holds_duckdb" => {
            "The DuckDB database is currently owned by another authorized process."
        }
        "funds_account_flow_failed" => {
            "The authorized account-flow analysis could not be completed."
        }
        "funds_direct_source_preview_failed" => {
            "The authorized direct source preview could not be completed."
        }
        "funds_deterministic_cleaning_failed" => {
            "The authorized deterministic cleaning could not be completed."
        }
        "funds_transaction_source_row_failed" => {
            "The authorized transaction source-row page could not be completed."
        }
        "data_engine_unsupported_command" => "The requested data engine command is not supported.",
        _ => "The data engine operation failed.",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn diagnostic_run_milliseconds_have_a_cross_language_upper_bound() {
        assert_eq!(bounded_elapsed_ms(Duration::from_millis(9_999)), 9_999);
        assert_eq!(bounded_elapsed_ms(Duration::from_millis(10_000)), 10_000);
        assert_eq!(bounded_elapsed_ms(Duration::from_millis(10_001)), 10_000);
        assert_eq!(bounded_elapsed_ms(Duration::MAX), 10_000);
    }

    #[test]
    fn bounded_payload_u64_rejects_out_of_range_defaults() {
        for default_value in [0, 4] {
            let error = bounded_payload_u64(&json!({}), "limit", default_value, 1, 3)
                .expect_err("out-of-range internal default must fail closed");

            assert!(
                format!("{error:#}").contains("data_engine_request_contract_invalid"),
                "unexpected error: {error:#}"
            );
        }
    }

    #[test]
    fn ping_returns_stable_envelope() {
        let payload = handle_line(r#"{"request_id":"r1","command":"ping"}"#);

        assert_eq!(
            payload.get("request_id").and_then(Value::as_str),
            Some("r1")
        );
        assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(true));
        assert_eq!(
            payload.pointer("/data/pong").and_then(Value::as_bool),
            Some(true)
        );
        assert_eq!(
            payload
                .pointer("/diagnostics/engine")
                .and_then(Value::as_str),
            Some("analytix-data-engine")
        );
        assert_eq!(
            payload
                .pointer("/diagnostics/command")
                .and_then(Value::as_str),
            Some("ping")
        );
    }

    #[test]
    fn invalid_json_returns_error_envelope() {
        let payload = handle_line("{not json");

        assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            payload.pointer("/error/code").and_then(Value::as_str),
            Some("invalid_request_json")
        );
        assert!(payload.get("diagnostics").is_some());
    }

    #[test]
    fn request_envelope_rejects_unknown_fields_and_non_string_ids() {
        for request in [
            r#"{"request_id":"r1","command":"ping","unknown":true}"#,
            r#"{"request_id":1,"command":"ping"}"#,
            r#"{"request_id":"r1","command":"ping","payload":{"value":1,"value":2}}"#,
            r#"{"request_id":"r1","command":"ping","payload":{"items":[{"value":1,"value":2}]}}"#,
        ] {
            let payload = handle_line(request);
            assert_eq!(payload.get("request_id"), Some(&Value::Null));
            assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(false));
            assert_eq!(
                payload.pointer("/error/code").and_then(Value::as_str),
                Some("invalid_request_json")
            );
        }
    }

    #[test]
    fn command_contract_rejects_unknown_payload_fields_and_invalid_limits() {
        let db_path = temp_case_db_path("strict-command-contract");
        let unknown = json!({
            "request_id": "unknown",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false, "unknown": true}
        });
        let unknown_response = handle_line(&unknown.to_string());
        assert_eq!(
            unknown_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert!(!db_path.exists());

        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-1",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        let session_id = opened
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("session id");
        let invalid_limit = json!({
            "request_id": "invalid-limit",
            "command": "duckdb.query",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "session_id": session_id,
                "sql": "SELECT 1",
                "row_limit": 0
            }
        });
        let invalid_limit_response = handle_line_with_state(&mut state, &invalid_limit.to_string());
        assert_eq!(
            invalid_limit_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn duckdb_session_rejects_cross_case_and_cross_database_reuse() {
        let db_path = temp_case_db_path("session-context-a");
        let other_db_path = temp_case_db_path("session-context-b");
        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        let session_id = opened
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("session id");

        for (case_id, request_db_path) in [
            ("case-b", db_path.as_path()),
            ("case-a", other_db_path.as_path()),
        ] {
            let response = handle_line_with_state(
                &mut state,
                &json!({
                    "request_id": "mismatch",
                    "command": "duckdb.query",
                    "case_id": case_id,
                    "db_path": request_db_path.display().to_string(),
                    "payload": {"session_id": session_id, "sql": "SELECT 1"}
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_context_mismatch")
            );
        }

        assert!(state.duckdb_sessions.contains_key(session_id));
        let closed = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "close",
                "command": "duckdb.close_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"session_id": session_id}
            })
            .to_string(),
        );
        assert_eq!(closed.get("ok").and_then(Value::as_bool), Some(true));
        cleanup_temp_case_db(&db_path);
        cleanup_temp_case_db(&other_db_path);
    }

    #[test]
    fn request_frames_are_bounded_before_newline_allocation() {
        let input = b"{\"command\":\"ping\"}\r\nnext\n";
        let mut reader = std::io::BufReader::with_capacity(4, std::io::Cursor::new(input));

        assert_eq!(
            read_bounded_request_frame(&mut reader, 64).expect("first frame"),
            Some(b"{\"command\":\"ping\"}".to_vec())
        );
        assert_eq!(
            read_bounded_request_frame(&mut reader, 64).expect("second frame"),
            Some(b"next".to_vec())
        );
        assert_eq!(
            read_bounded_request_frame(&mut reader, 64).expect("eof"),
            None
        );

        let mut oversized =
            std::io::BufReader::with_capacity(3, std::io::Cursor::new(b"123456789\n"));
        let error = read_bounded_request_frame(&mut oversized, 8)
            .expect_err("oversized frame must fail before an unbounded read");
        assert_eq!(error.kind(), io::ErrorKind::InvalidData);

        let mut partial = std::io::BufReader::new(std::io::Cursor::new(b"{\"command\":\"ping\"}"));
        let error = read_bounded_request_frame(&mut partial, 64)
            .expect_err("EOF before newline must never execute a partial request");
        assert_eq!(error.kind(), io::ErrorKind::UnexpectedEof);
    }

    #[test]
    fn request_shape_limits_run_before_deserialization() {
        assert_eq!(
            validate_request_json_shape_with_limits(b"[[[0]]]", 2, 100, 100),
            Err(JsonShapeError::LimitExceeded)
        );
        assert_eq!(
            validate_request_json_shape_with_limits(b"[0,0]", 10, 3, 100),
            Err(JsonShapeError::LimitExceeded)
        );
        assert_eq!(
            validate_request_json_shape_with_limits(b"{\"key\":\"abcd\"}", 10, 100, 3),
            Err(JsonShapeError::LimitExceeded)
        );
        assert_eq!(
            validate_request_json_shape_with_limits(b"{\"key\":[1,2]}", 10, 100, 100),
            Ok(())
        );
        assert_eq!(
            validate_request_json_shape_with_limits(b"{\"key\":[1,2]", 10, 100, 100),
            Err(JsonShapeError::Invalid)
        );
    }

    #[test]
    fn response_encoding_replaces_oversized_payload_with_fixed_boundary() {
        let response = json!({
            "request_id": "private-request-id",
            "ok": true,
            "data": {"private": "6".repeat(4096)},
        });
        let mut output = Vec::new();

        write_bounded_response(&mut output, &response, 1024).expect("fixed response");

        assert!(output.len() <= 1024);
        assert!(output.ends_with(b"\n"));
        let decoded: Value = serde_json::from_slice(&output).expect("response JSON");
        assert_eq!(decoded.get("request_id"), Some(&Value::Null));
        assert_eq!(decoded.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            decoded.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_response_limit_exceeded")
        );
        assert!(!String::from_utf8_lossy(&output).contains("private-request-id"));
    }

    #[test]
    fn unsupported_command_returns_specific_code() {
        let payload = handle_line(r#"{"request_id":"r2","command":"missing"}"#);

        assert_eq!(
            payload.get("request_id").and_then(Value::as_str),
            Some("r2")
        );
        assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            payload.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_unsupported_command")
        );
        assert!(payload.get("diagnostics").is_some());
    }

    #[test]
    fn analysis_compute_unsupported_inner_command_returns_specific_code() {
        let db_path = temp_case_db("unsupported-inner");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let request = json!({
            "request_id": "r3",
            "command": "analysis.compute",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "command": "missing-inner",
                "args": {"argv": []}
            }
        });

        let payload = handle_line(&request.to_string());

        assert_eq!(
            payload.get("request_id").and_then(Value::as_str),
            Some("r3")
        );
        assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            payload.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_unsupported_command")
        );
        assert!(payload.get("diagnostics").is_some());
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn funds_materialize_txn_daily_v1_has_exact_typed_response() {
        let db_path = temp_case_db_path("funds-materialize-typed");
        seed_txn_daily_source(&db_path);
        let mut state = EngineState::default();

        let response = materialize_txn_daily_fixture(&mut state, &db_path);

        assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
        assert_eq!(
            response
                .pointer("/diagnostics/command")
                .and_then(Value::as_str),
            Some("funds.materialize_txn_daily_v1")
        );
        let data = response
            .get("data")
            .and_then(Value::as_object)
            .expect("typed funds response");
        let expected = [
            "aggName",
            "aggVersion",
            "caseId",
            "duckdbContentSnapshotDigest",
            "duckdbSnapshotManifestSha256",
            "materializationIdentity",
            "materializationIdentitySchemaVersion",
            "producerContentId",
            "producerContentManifestBase64",
            "producerContentManifestByteLength",
            "producerContentManifestSha256",
            "rawArtifactManifestSha256",
            "rawSourceManifestBase64",
            "rawSourceManifestByteLength",
            "rawSourceManifestSha256",
            "rowCount",
            "schemaDigest",
        ]
        .into_iter()
        .collect::<HashSet<_>>();
        assert_eq!(
            data.keys().map(String::as_str).collect::<HashSet<_>>(),
            expected
        );
        assert_eq!(data.get("caseId").and_then(Value::as_str), Some("case-a"));
        assert_eq!(data.get("rowCount").and_then(Value::as_i64), Some(2));
        assert!(data
            .get("producerContentId")
            .and_then(Value::as_str)
            .is_some_and(|value| value.starts_with("fpc1_")));
        assert_eq!(
            data.get("materializationIdentitySchemaVersion")
                .and_then(Value::as_i64),
            Some(2)
        );
        for field in [
            "producerContentManifestBase64",
            "producerContentManifestSha256",
            "rawSourceManifestBase64",
            "rawSourceManifestSha256",
        ] {
            assert!(data
                .get(field)
                .and_then(Value::as_str)
                .is_some_and(|value| !value.is_empty()));
        }
        for field in [
            "producerContentManifestByteLength",
            "rawSourceManifestByteLength",
        ] {
            assert!(data
                .get(field)
                .and_then(Value::as_u64)
                .is_some_and(|value| value > 0));
        }
        assert!(!data.contains_key("phases_s"));
        assert!(!data.contains_key("phaseGroupsS"));
        assert!(state.duckdb_sessions.is_empty());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn canonical_csv_build_rejection_is_recognized_but_not_db_bound() {
        let response = handle_line(
            &json!({
                "request_id": "canonical-rejected",
                "command": "funds.build_canonical_csv_snapshot_v1",
                "case_id": "case-a",
                "payload": {
                    "caseId": "case-a",
                    "profile": "canonical_direct_csv_v1",
                    "privateImportFileId": "0123456789abcdefabcd",
                    "sourceRevision": 7
                }
            })
            .to_string(),
        );

        assert_eq!(response["request_id"], "canonical-rejected");
        assert_eq!(response["ok"], false);
        assert_eq!(
            response["error"]["code"],
            "data_engine_request_contract_invalid"
        );
        assert_eq!(
            response["diagnostics"]["command"],
            "funds.build_canonical_csv_snapshot_v1"
        );
        assert_eq!(response["diagnostics"]["case_bound"], true);
        assert_eq!(response["diagnostics"]["db_bound"], false);
    }

    #[test]
    fn funds_transaction_source_row_command_contract_is_closed_and_value_free() {
        let exact = transaction_source_row_payload();
        let private_path = "/private/AB_R2_SOURCE_ROW_PATH_SENTINEL";
        let private_value = "AB_R2_SOURCE_ROW_VALUE_SENTINEL";
        let invalid_requests = [
            {
                let mut payload = exact.clone();
                payload
                    .as_object_mut()
                    .expect("source-row payload")
                    .remove("cursor");
                json!({
                    "request_id": "missing-key",
                    "command": "funds.transaction_source_row_page_v1",
                    "case_id": "case-a",
                    "payload": payload,
                })
            },
            {
                let mut payload = exact.clone();
                payload.as_object_mut().expect("source-row payload").insert(
                    "sourcePath".to_string(),
                    Value::String(private_path.to_string()),
                );
                json!({
                    "request_id": "unknown-key",
                    "command": "funds.transaction_source_row_page_v1",
                    "case_id": "case-a",
                    "payload": payload,
                })
            },
            json!({
                "request_id": "caller-path",
                "command": "funds.transaction_source_row_page_v1",
                "case_id": "case-a",
                "db_path": private_path,
                "payload": exact,
            }),
            {
                let mut payload = transaction_source_row_payload();
                payload.as_object_mut().expect("source-row payload").insert(
                    "relation".to_string(),
                    Value::String(private_value.to_string()),
                );
                json!({
                    "request_id": "invalid-relation",
                    "command": "funds.transaction_source_row_page_v1",
                    "case_id": "case-a",
                    "payload": payload,
                })
            },
        ];
        for request in invalid_requests {
            let response = handle_line(&request.to_string());
            assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_request_contract_invalid")
            );
            assert_eq!(
                response
                    .pointer("/diagnostics/command")
                    .and_then(Value::as_str),
                Some("funds.transaction_source_row_page_v1")
            );
            assert_eq!(
                response
                    .pointer("/diagnostics/db_bound")
                    .and_then(Value::as_bool),
                Some(false)
            );
            let encoded = serde_json::to_string(&response).expect("serialize closed error");
            assert!(!encoded.contains(private_path));
            assert!(!encoded.contains(private_value));
            assert!(!encoded.contains("db_path"));
        }

        let response = handle_line(
            &json!({
                "request_id": "recognized-without-fd3",
                "command": "funds.transaction_source_row_page_v1",
                "case_id": "case-a",
                "payload": transaction_source_row_payload(),
            })
            .to_string(),
        );
        assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            response.pointer("/error/code").and_then(Value::as_str),
            Some("funds_transaction_source_row_failed")
        );
        assert_eq!(
            response.pointer("/error/message").and_then(Value::as_str),
            Some("The authorized transaction source-row page could not be completed.")
        );
        assert_eq!(
            response
                .pointer("/diagnostics/command")
                .and_then(Value::as_str),
            Some("funds.transaction_source_row_page_v1")
        );
    }

    #[cfg(unix)]
    #[test]
    fn funds_transaction_source_row_reads_unlinked_inherited_fd3_with_exact_diagnostics() {
        use std::io::{Seek, SeekFrom};
        use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};

        let fixture_path = temp_case_db_path("funds-source-row-inherited");
        let fixture_root = fixture_path.parent().expect("source-row fixture root");
        fs::set_permissions(fixture_root, fs::Permissions::from_mode(0o700))
            .expect("secure source-row fixture root");

        let source_path = fixture_root.join("source.csv");
        let mut source_options = OpenOptions::new();
        source_options.create_new(true).write(true).mode(0o600);
        let mut source_writer = source_options
            .open(&source_path)
            .expect("create canonical CSV source fixture");
        let mut source_bytes = canonical_csv_source_fixture();
        source_writer
            .write_all(&source_bytes)
            .expect("write canonical CSV source fixture");
        source_writer
            .sync_all()
            .expect("sync canonical CSV source fixture");
        source_bytes.fill(0);
        drop(source_writer);
        fs::set_permissions(&source_path, fs::Permissions::from_mode(0o400))
            .expect("freeze canonical CSV source fixture");
        let source = File::open(&source_path).expect("open canonical CSV source fixture");
        fs::remove_file(&source_path).expect("unlink canonical CSV source fixture");

        let output_path = fixture_root.join(CANONICAL_CSV_OUTPUT_FD.to_string());
        let mut output_options = OpenOptions::new();
        output_options
            .create_new(true)
            .read(true)
            .write(true)
            .mode(0o600);
        let mut output = output_options
            .open(&output_path)
            .expect("create canonical snapshot output fixture");
        fs::remove_file(&output_path).expect("unlink canonical snapshot output fixture");

        let built = run_inherited_snapshot_helper(
            "funds.build_canonical_csv_snapshot_v1",
            "canonical_build_success",
            Some(&source),
            Some(&output),
            &canonical_csv_build_payload(),
            Some(&source_path),
            Some(fixture_root),
        );
        assert!(built.status.success(), "canonical build helper failed");
        drop(source);

        output
            .seek(SeekFrom::Start(0))
            .expect("rewind canonical snapshot fixture");
        let installed_path = fixture_root.join(ACCOUNT_FLOW_SNAPSHOT_FD.to_string());
        let mut installed_options = OpenOptions::new();
        installed_options
            .create_new(true)
            .read(true)
            .write(true)
            .mode(0o600);
        let mut installed_writer = installed_options
            .open(&installed_path)
            .expect("create installed snapshot fixture");
        io::copy(&mut output, &mut installed_writer).expect("install canonical snapshot fixture");
        installed_writer
            .sync_all()
            .expect("sync installed snapshot fixture");
        drop(installed_writer);
        fs::set_permissions(&installed_path, fs::Permissions::from_mode(0o400))
            .expect("freeze installed snapshot fixture");
        let installed = File::open(&installed_path).expect("open installed snapshot fixture");
        fs::remove_file(&installed_path).expect("unlink installed snapshot fixture");

        let paged = run_inherited_snapshot_helper(
            "funds.transaction_source_row_page_v1",
            "transaction_source_row_success",
            Some(&installed),
            None,
            &transaction_source_row_payload(),
            Some(&installed_path),
            None,
        );
        assert!(paged.status.success(), "source-row helper failed");

        drop(installed);
        drop(output);
        fs::remove_dir(fixture_root).expect("remove empty source-row fixture root");
    }

    #[test]
    fn funds_resolve_account_ingress_rejects_caller_path_and_contract_drift_without_reflection() {
        let db_path = temp_case_db_path("funds-account-ingress-caller-path");
        let materialized = json!({
            "data": {
                "producerContentId": format!("fpc1_{}", "4".repeat(64)),
                "producerContentManifestSha256": "5".repeat(64)
            }
        });
        let response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "account-ingress-caller-path",
                "command": "funds.resolve_account_ingress",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": account_ingress_payload(&materialized)
            })
            .to_string(),
        );
        assert_eq!(
            response.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(
            response
                .pointer("/diagnostics/db_bound")
                .and_then(Value::as_bool),
            Some(false)
        );
        let encoded = serde_json::to_string(&response).expect("serialize response");
        assert!(!encoded.contains(db_path.to_string_lossy().as_ref()));
        assert!(!encoded.contains("db_path"));
        assert!(!db_path.exists());

        let mut drifted = account_ingress_payload(&materialized);
        drifted
            .as_object_mut()
            .expect("account-ingress payload")
            .insert("sql".to_string(), json!("SELECT * FROM private_table"));
        let drifted_response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "account-ingress-contract-drift",
                "command": "funds.resolve_account_ingress",
                "case_id": "case-a",
                "payload": drifted
            })
            .to_string(),
        );
        assert_eq!(
            drifted_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert!(!serde_json::to_string(&drifted_response)
            .expect("serialize drift response")
            .contains("private_table"));
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn funds_analyze_account_flows_rejects_caller_database_path_without_reflection() {
        let db_path = temp_case_db_path("funds-account-flow-caller-path");
        let materialized = json!({
            "data": {
                "producerContentId": format!("fpc1_{}", "4".repeat(64)),
                "producerContentManifestSha256": "5".repeat(64)
            }
        });
        let response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "caller-path",
                "command": "funds.analyze_account_flows",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": account_flow_payload(&materialized)
            })
            .to_string(),
        );

        assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            response.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(
            response
                .pointer("/diagnostics/db_bound")
                .and_then(Value::as_bool),
            Some(false)
        );
        let encoded = serde_json::to_string(&response).expect("serialize response");
        assert!(!encoded.contains(db_path.to_string_lossy().as_ref()));
        assert!(!encoded.contains("db_path"));
        assert!(!db_path.exists());

        let null_path_response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "caller-null-path",
                "command": "funds.analyze_account_flows",
                "case_id": "case-a",
                "db_path": Value::Null,
                "payload": account_flow_payload(&materialized)
            })
            .to_string(),
        );
        assert_eq!(
            null_path_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(
            null_path_response
                .pointer("/diagnostics/db_bound")
                .and_then(Value::as_bool),
            Some(false)
        );
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn funds_direct_source_preview_rejects_caller_database_path_without_reflection() {
        let db_path = temp_case_db_path("funds-direct-source-preview-caller-path");
        let response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "direct-caller-path",
                "command": "funds.direct_source_preview",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": direct_source_preview_payload()
            })
            .to_string(),
        );

        assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
        assert_eq!(
            response.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(
            response
                .pointer("/diagnostics/db_bound")
                .and_then(Value::as_bool),
            Some(false)
        );
        let encoded = serde_json::to_string(&response).expect("serialize response");
        assert!(!encoded.contains(db_path.to_string_lossy().as_ref()));
        assert!(!encoded.contains("db_path"));
        assert!(!db_path.exists());

        let null_path_response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "direct-caller-null-path",
                "command": "funds.direct_source_preview",
                "case_id": "case-a",
                "db_path": Value::Null,
                "payload": direct_source_preview_payload()
            })
            .to_string(),
        );
        assert_eq!(
            null_path_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(
            null_path_response
                .pointer("/diagnostics/db_bound")
                .and_then(Value::as_bool),
            Some(false)
        );
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn deterministic_cleaning_rejects_caller_database_path_and_extra_authority_without_reflection()
    {
        let db_path = temp_case_db_path("deterministic-cleaning-caller-path");
        for payload in [deterministic_cleaning_payload(), {
            let mut payload = deterministic_cleaning_payload();
            payload
                .as_object_mut()
                .expect("cleaning payload object")
                .insert(
                    "rawLog".to_string(),
                    Value::String("RAW_CLEANING_CANARY".to_string()),
                );
            payload
        }] {
            let response = handle_line_with_state(
                &mut EngineState::default(),
                &json!({
                    "request_id": "cleaning-caller-path",
                    "command": "funds.deterministic_cleaning_v1",
                    "case_id": "case-a",
                    "db_path": db_path.display().to_string(),
                    "payload": payload
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_request_contract_invalid")
            );
            let encoded = serde_json::to_string(&response).expect("serialize response");
            assert!(!encoded.contains(db_path.to_string_lossy().as_ref()));
            assert!(!encoded.contains("db_path"));
            assert!(!encoded.contains("RAW_CLEANING_CANARY"));
        }
        assert!(!db_path.exists());
    }

    #[cfg(unix)]
    #[test]
    fn funds_resolve_account_ingress_reads_unlinked_inherited_fd3_without_sidecars() {
        use std::os::unix::fs::MetadataExt;

        let db_path = temp_case_db_path("funds-account-ingress-inherited");
        seed_txn_daily_source(&db_path);
        let mut state = EngineState::default();
        let materialized = materialize_txn_daily_fixture(&mut state, &db_path);
        assert_eq!(materialized.get("ok").and_then(Value::as_bool), Some(true));

        let parent = db_path.parent().expect("snapshot parent");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let lock_path = owner_lock_file_path(&canonical_db_path(&db_path));
        assert!(!owner_path.exists());
        fs::remove_file(&lock_path).expect("remove materialization-only owner lock");
        assert_eq!(
            fs::read_dir(parent).expect("read snapshot parent").count(),
            1,
            "only the DuckDB snapshot may remain before unlink"
        );
        let inherited_backing = parent.join("3");
        fs::rename(&db_path, &inherited_backing).expect("stage fixed descriptor basename");
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&inherited_backing, fs::Permissions::from_mode(0o400))
            .expect("freeze inherited snapshot mode");
        let snapshot = File::open(&inherited_backing).expect("open immutable snapshot read-only");
        fs::remove_file(&inherited_backing).expect("unlink immutable snapshot");
        assert_eq!(
            snapshot
                .metadata()
                .expect("inherited snapshot metadata")
                .nlink(),
            0
        );

        let output = run_inherited_snapshot_helper(
            "funds.resolve_account_ingress",
            "account_ingress_success",
            Some(&snapshot),
            None,
            &account_ingress_payload(&materialized),
            Some(&inherited_backing),
            None,
        );
        assert!(
            output.status.success(),
            "account-ingress helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
        assert_eq!(
            fs::read_dir(parent)
                .expect("read snapshot parent after account ingress")
                .count(),
            0,
            "read-only inherited ingress must create no owner, lock, WAL, or other sidecar"
        );
        fs::remove_dir(parent).expect("remove empty snapshot parent");
    }

    #[cfg(unix)]
    #[test]
    fn funds_analyze_account_flows_reads_unlinked_inherited_fd3_without_sidecars() {
        use std::os::unix::fs::MetadataExt;

        let db_path = temp_case_db_path("funds-account-flow-inherited");
        seed_txn_daily_source(&db_path);
        let mut state = EngineState::default();
        let materialized = materialize_txn_daily_fixture(&mut state, &db_path);
        assert_eq!(materialized.get("ok").and_then(Value::as_bool), Some(true));

        let parent = db_path.parent().expect("snapshot parent");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let lock_path = owner_lock_file_path(&canonical_db_path(&db_path));
        assert!(!owner_path.exists());
        fs::remove_file(&lock_path).expect("remove materialization-only owner lock");
        assert_eq!(
            fs::read_dir(parent).expect("read snapshot parent").count(),
            1,
            "only the DuckDB snapshot may remain before unlink"
        );
        let inherited_backing = parent.join("3");
        fs::rename(&db_path, &inherited_backing).expect("stage fixed descriptor basename");
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&inherited_backing, fs::Permissions::from_mode(0o400))
            .expect("freeze inherited snapshot mode");
        let snapshot = File::open(&inherited_backing).expect("open immutable snapshot read-only");
        fs::remove_file(&inherited_backing).expect("unlink immutable snapshot");
        assert_eq!(
            snapshot
                .metadata()
                .expect("inherited snapshot metadata")
                .nlink(),
            0
        );

        let output = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "success",
            Some(&snapshot),
            None,
            &account_flow_payload(&materialized),
            Some(&inherited_backing),
            None,
        );
        assert!(
            output.status.success(),
            "inherited snapshot helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
        assert_eq!(
            fs::read_dir(parent)
                .expect("read snapshot parent after account flow")
                .count(),
            0,
            "read-only inherited flow must create no owner, lock, WAL, or other sidecar"
        );
        fs::remove_dir(parent).expect("remove empty snapshot parent");
    }

    #[cfg(unix)]
    #[test]
    fn funds_analyze_account_flows_rejects_missing_linked_wrong_mode_writeonly_and_nonregular_fd3()
    {
        use std::os::fd::FromRawFd;

        let payload = account_flow_payload(&json!({
            "data": {
                "producerContentId": format!("fpc1_{}", "4".repeat(64)),
                "producerContentManifestSha256": "5".repeat(64)
            }
        }));

        let missing = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "invalid",
            None,
            None,
            &payload,
            None,
            None,
        );
        assert!(
            missing.status.success(),
            "missing fd helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&missing.stdout),
            String::from_utf8_lossy(&missing.stderr)
        );

        let linked_path = temp_case_db("funds-account-flow-linked-fd");
        let linked = File::open(&linked_path).expect("open linked fd fixture");
        let invalid = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "invalid",
            Some(&linked),
            None,
            &payload,
            Some(&linked_path),
            None,
        );
        assert!(
            invalid.status.success(),
            "linked fd helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&invalid.stdout),
            String::from_utf8_lossy(&invalid.stderr)
        );
        cleanup_temp_case_db(&linked_path);

        let write_only_path = temp_case_db("funds-account-flow-writeonly-fd");
        let write_only = OpenOptions::new()
            .write(true)
            .open(&write_only_path)
            .expect("open write-only fd fixture");
        fs::remove_file(&write_only_path).expect("unlink write-only fd fixture");
        let invalid = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "invalid",
            Some(&write_only),
            None,
            &payload,
            Some(&write_only_path),
            None,
        );
        assert!(
            invalid.status.success(),
            "write-only fd helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&invalid.stdout),
            String::from_utf8_lossy(&invalid.stderr)
        );

        let wrong_mode_path = temp_case_db("funds-account-flow-wrong-mode-fd");
        let wrong_mode = File::open(&wrong_mode_path).expect("open wrong-mode fd fixture");
        fs::remove_file(&wrong_mode_path).expect("unlink wrong-mode fd fixture");
        let invalid = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "invalid",
            Some(&wrong_mode),
            None,
            &payload,
            Some(&wrong_mode_path),
            None,
        );
        assert!(
            invalid.status.success(),
            "wrong-mode fd helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&invalid.stdout),
            String::from_utf8_lossy(&invalid.stderr)
        );
        drop(wrong_mode);
        cleanup_temp_case_db(&wrong_mode_path);
        cleanup_temp_case_db(&write_only_path);

        let mut pipe_fds = [-1; 2];
        assert_eq!(unsafe { libc::pipe(pipe_fds.as_mut_ptr()) }, 0);
        // SAFETY: pipe returned two new descriptors and ownership transfers to
        // these File values exactly once.
        let pipe_reader = unsafe { File::from_raw_fd(pipe_fds[0]) };
        let _pipe_writer = unsafe { File::from_raw_fd(pipe_fds[1]) };
        let invalid = run_inherited_snapshot_helper(
            "funds.analyze_account_flows",
            "invalid",
            Some(&pipe_reader),
            None,
            &payload,
            None,
            None,
        );
        assert!(
            invalid.status.success(),
            "non-regular fd helper failed: stdout={} stderr={}",
            String::from_utf8_lossy(&invalid.stdout),
            String::from_utf8_lossy(&invalid.stderr)
        );
    }

    #[test]
    fn funds_analyze_account_flows_rejects_contract_drift_before_filesystem_side_effect() {
        let db_path = temp_case_db_path("funds-account-flow-invalid");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let lock_path = owner_lock_file_path(&canonical_db_path(&db_path));
        let materialized = json!({
            "data": {
                "producerContentId": format!("fpc1_{}", "4".repeat(64)),
                "producerContentManifestSha256": "5".repeat(64)
            }
        });
        let exact = account_flow_payload(&materialized);
        let required = exact
            .as_object()
            .expect("account-flow payload")
            .keys()
            .cloned()
            .collect::<Vec<_>>();

        for missing in required {
            let mut invalid = exact.clone();
            invalid
                .as_object_mut()
                .expect("account-flow payload")
                .remove(&missing);
            let response = handle_line_with_state(
                &mut EngineState::default(),
                &json!({
                    "request_id": "missing",
                    "command": "funds.analyze_account_flows",
                    "case_id": "case-a",
                    "payload": invalid
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_request_contract_invalid"),
                "missing field {missing}"
            );
            assert!(!owner_path.exists());
            assert!(!lock_path.exists());
        }

        let mut unknown = exact.clone();
        unknown
            .as_object_mut()
            .expect("account-flow payload")
            .insert("sql".to_string(), json!("SELECT * FROM private_table"));
        let unknown_response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "unknown",
                "command": "funds.analyze_account_flows",
                "case_id": "case-a",
                "payload": unknown
            })
            .to_string(),
        );
        assert_eq!(
            unknown_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert!(!owner_path.exists());
        assert!(!lock_path.exists());

        let mut cross_case = exact;
        cross_case
            .as_object_mut()
            .expect("account-flow payload")
            .insert("caseId".to_string(), json!("case-b"));
        let cross_case_response = handle_line_with_state(
            &mut EngineState::default(),
            &json!({
                "request_id": "cross-case",
                "command": "funds.analyze_account_flows",
                "case_id": "case-a",
                "payload": cross_case
            })
            .to_string(),
        );
        assert_eq!(
            cross_case_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_context_mismatch")
        );
        assert!(!owner_path.exists());
        assert!(!lock_path.exists());

        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn funds_materialize_and_legacy_argv_reject_before_owner_side_effect() {
        let db_path = temp_case_db_path("funds-materialize-rejection");
        seed_txn_daily_source(&db_path);
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        assert_eq!(opened.get("ok").and_then(Value::as_bool), Some(true));
        assert!(owner_path.exists());

        let exact = json!({
            "caseId": "case-a",
            "sourceRevision": 7,
            "sourceRowCount": 2,
            "sourceMaxTxnTs": "2026-01-02 01:02:03",
            "sourceMaxId": 2,
            "rawArtifactManifestSha256": "1".repeat(64)
        });
        let invalid_payloads = [
            {
                let mut value = exact.clone();
                value.as_object_mut().expect("object").remove("sourceMaxId");
                value
            },
            {
                let mut value = exact.clone();
                value
                    .as_object_mut()
                    .expect("object")
                    .remove("rawArtifactManifestSha256");
                value
            },
            {
                let mut value = exact.clone();
                value
                    .as_object_mut()
                    .expect("object")
                    .insert("unexpected".to_string(), Value::Bool(true));
                value
            },
            {
                let mut value = exact.clone();
                value
                    .as_object_mut()
                    .expect("object")
                    .insert("sourceRevision".to_string(), Value::String("7".to_string()));
                value
            },
            {
                let mut value = exact.clone();
                value
                    .as_object_mut()
                    .expect("object")
                    .insert("caseId".to_string(), Value::String("case-b".to_string()));
                value
            },
        ];
        for payload in invalid_payloads {
            let response = handle_line_with_state(
                &mut state,
                &json!({
                    "request_id": "invalid",
                    "command": "funds.materialize_txn_daily_v1",
                    "case_id": "case-a",
                    "db_path": db_path.display().to_string(),
                    "payload": payload
                })
                .to_string(),
            );
            assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
            assert!(matches!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_request_contract_invalid" | "data_engine_context_mismatch")
            ));
            assert_eq!(state.duckdb_sessions.len(), 1);
            assert!(owner_path.exists());
        }

        let legacy = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "legacy",
                "command": "analysis.compute",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {
                    "command": "materialize-txn-daily",
                    "args": {"argv": []}
                }
            })
            .to_string(),
        );
        assert_eq!(
            legacy.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_unsupported_command")
        );
        assert_eq!(state.duckdb_sessions.len(), 1);
        assert!(owner_path.exists());

        drop(state);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn analysis_verify_routes_to_read_only_bounded_producer_verifier() {
        let db_path = temp_case_db_path("analysis-verify-readonly");
        seed_txn_daily_source(&db_path);
        let mut state = EngineState::default();
        let materialized = materialize_txn_daily_fixture(&mut state, &db_path);
        assert_eq!(materialized.get("ok").and_then(Value::as_bool), Some(true));
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        assert!(!owner_path.exists());
        let before = file_sha256(&db_path);

        let response = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "verify",
                "command": "analysis.verify",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {
                    "command": "verify-txn-daily",
                    "args": {"argv": []}
                }
            })
            .to_string(),
        );

        assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
        assert_eq!(
            response
                .pointer("/diagnostics/command")
                .and_then(Value::as_str),
            Some("analysis.verify")
        );
        let data = response
            .get("data")
            .and_then(Value::as_object)
            .expect("bounded verifier metadata");
        assert_eq!(data.len(), 14);
        for key in [
            "ok",
            "case_id",
            "source_revision",
            "source_row_count",
            "accepted_row_count",
            "rejected_row_count",
            "duplicate_row_count",
            "aggregate_row_count",
            "materialization_identity",
            "result_signature",
            "producer_content_contract",
            "producer_content_id",
            "producer_manifest_sha256",
            "duckdb_version",
        ] {
            assert!(data.contains_key(key), "missing bounded field {key}");
        }
        assert_eq!(data.get("case_id").and_then(Value::as_str), Some("case-a"));
        assert_eq!(
            data.get("producer_content_contract")
                .and_then(Value::as_str),
            Some("analytix.funds-producer-content-manifest/v1")
        );
        let producer_content_id = data
            .get("producer_content_id")
            .and_then(Value::as_str)
            .expect("producer content id");
        assert!(producer_content_id.starts_with("fpc1_"));
        assert!(!producer_content_id.starts_with("dsv2_"));
        assert_eq!(file_sha256(&db_path), before);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn analysis_verify_rejects_live_session_without_closing_or_writing() {
        let db_path = temp_case_db_path("analysis-verify-session-conflict");
        seed_txn_daily_source(&db_path);
        let mut state = EngineState::default();
        assert_eq!(
            materialize_txn_daily_fixture(&mut state, &db_path)
                .get("ok")
                .and_then(Value::as_bool),
            Some(true)
        );
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": true}
            })
            .to_string(),
        );
        assert_eq!(opened.get("ok").and_then(Value::as_bool), Some(true));
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        assert!(owner_path.exists());
        let before = file_sha256(&db_path);

        let response = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "verify-conflict",
                "command": "analysis.verify",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {
                    "command": "verify-txn-daily",
                    "args": {"argv": []}
                }
            })
            .to_string(),
        );

        assert_eq!(
            response.pointer("/error/code").and_then(Value::as_str),
            Some("data_engine_owner_conflict")
        );
        assert_eq!(state.duckdb_sessions.len(), 1);
        assert!(owner_path.exists());
        assert_eq!(file_sha256(&db_path), before);
        drop(state);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn analysis_compute_missing_fields_keep_error_envelope_stable() {
        let cases = [
            json!({
                "request_id": "missing-db",
                "command": "analysis.compute",
                "case_id": "case-1",
                "payload": {"command": "query-chart-dashboard", "args": {"argv": []}},
            }),
            json!({
                "request_id": "missing-payload",
                "command": "analysis.compute",
                "case_id": "case-1",
                "db_path": "case.duckdb",
            }),
            json!({
                "request_id": "missing-inner-command",
                "command": "analysis.compute",
                "case_id": "case-1",
                "db_path": "case.duckdb",
                "payload": {"args": {"argv": []}},
            }),
            json!({
                "request_id": "missing-inner-args",
                "command": "analysis.compute",
                "case_id": "case-1",
                "db_path": "case.duckdb",
                "payload": {"command": "query-chart-dashboard"},
            }),
        ];

        for request in cases {
            let payload = handle_line(&request.to_string());
            assert_eq!(payload.get("ok").and_then(Value::as_bool), Some(false));
            assert!(payload
                .pointer("/error/code")
                .and_then(Value::as_str)
                .is_some());
            assert!(payload
                .pointer("/error/message")
                .and_then(Value::as_str)
                .is_some());
            assert_eq!(
                payload
                    .pointer("/diagnostics/engine")
                    .and_then(Value::as_str),
                Some("analytix-data-engine")
            );
        }
    }

    #[test]
    fn analysis_context_is_injected_once_from_the_outer_request() {
        let db_path = temp_case_db_path("analysis-context-injection");
        let canonical = canonical_db_path(&db_path);

        let compute = analysis_compute_args_with_context(
            &json!({"argv": ["--force", "false"]}),
            "case-a",
            &canonical,
        )
        .expect("inject compute context");
        let argv = compute
            .get("argv")
            .and_then(Value::as_array)
            .expect("compute argv");
        assert_eq!(
            argv.iter()
                .filter(|value| value.as_str() == Some("--case-id"))
                .count(),
            1
        );
        assert_eq!(
            argv.iter()
                .filter(|value| value.as_str() == Some("--db-path"))
                .count(),
            1
        );
        assert_eq!(argv.get(1).and_then(Value::as_str), Some("case-a"));
        assert_eq!(argv.get(3).and_then(Value::as_str), canonical.to_str());

        let stats =
            analysis_stats_args_with_context(&json!({"tab": "byName"}), "case-a", &canonical)
                .expect("inject stats context");
        assert_eq!(stats.get("case_id").and_then(Value::as_str), Some("case-a"));
        assert_eq!(
            stats.get("db_path").and_then(Value::as_str),
            canonical.to_str()
        );
        assert!(validate_analysis_result_context(
            json!({"ok": true, "case_id": "case-a"}),
            "case-a"
        )
        .is_ok());
        for invalid in [
            json!({"ok": true, "case_id": "case-b"}),
            json!({"ok": true}),
            json!({"ok": false, "case_id": "case-a"}),
        ] {
            let error = validate_analysis_result_context(invalid, "case-a")
                .expect_err("result context mismatch must fail");
            assert!(format!("{error:#}").contains("data_engine_context_mismatch"));
        }
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn analysis_compute_rejects_inner_context_before_any_side_effect() {
        let db_path = temp_case_db_path("analysis-context-outer");
        let other_db_path = temp_case_db_path("analysis-context-inner");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        assert_eq!(opened.get("ok").and_then(Value::as_bool), Some(true));
        assert!(owner_path.exists());

        for argv in [
            json!(["--case-id", "case-b"]),
            json!(["--case-id=case-b"]),
            json!(["--db-path", other_db_path.display().to_string()]),
            json!([format!("--db-path={}", other_db_path.display())]),
        ] {
            let response = handle_line_with_state(
                &mut state,
                &json!({
                    "request_id": "mismatch",
                    "command": "analysis.compute",
                    "case_id": "case-a",
                    "db_path": db_path.display().to_string(),
                    "payload": {
                        "command": "materialize-rule-txn-index",
                        "args": {"argv": argv}
                    }
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_context_mismatch")
            );
            assert_eq!(state.duckdb_sessions.len(), 1);
            assert!(owner_path.exists());
            assert!(!other_db_path.exists());
        }

        drop(state);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
        cleanup_temp_case_db(&other_db_path);
    }

    #[test]
    fn analysis_stats_query_rejects_inner_context_before_any_side_effect() {
        let db_path = temp_case_db_path("stats-context-outer");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        assert_eq!(opened.get("ok").and_then(Value::as_bool), Some(true));

        for args in [
            json!({"case_id": "case-b"}),
            json!({"caseId": "case-b"}),
            json!({"db_path": "/tmp/other.duckdb"}),
            json!({"dbPath": "/tmp/other.duckdb"}),
        ] {
            let response = handle_line_with_state(
                &mut state,
                &json!({
                    "request_id": "mismatch",
                    "command": "analysis.stats_query",
                    "case_id": "case-a",
                    "db_path": db_path.display().to_string(),
                    "payload": {"command": "query-stats-date-range", "args": args}
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_context_mismatch")
            );
            assert_eq!(state.duckdb_sessions.len(), 1);
            assert!(owner_path.exists());
        }

        let invalid_schema = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "invalid-schema",
                "command": "analysis.stats_query",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {
                    "command": "query-stats-date-range",
                    "args": {"unexpected": true}
                }
            })
            .to_string(),
        );
        assert_eq!(
            invalid_schema
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_request_contract_invalid")
        );
        assert_eq!(state.duckdb_sessions.len(), 1);
        assert!(owner_path.exists());

        drop(state);
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn analysis_compute_requires_exact_inner_argv_envelope_before_side_effect() {
        let db_path = temp_case_db_path("analysis-argv-envelope");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let opened = handle_line_with_state(
            &mut state,
            &json!({
                "request_id": "open",
                "command": "duckdb.open_session",
                "case_id": "case-a",
                "db_path": db_path.display().to_string(),
                "payload": {"read_only": false}
            })
            .to_string(),
        );
        assert_eq!(opened.get("ok").and_then(Value::as_bool), Some(true));

        for args in [
            json!([]),
            json!({"args": []}),
            json!({"argv": [], "unexpected": true}),
            json!({"argv": "--force"}),
            json!({"argv": [1]}),
        ] {
            let response = handle_line_with_state(
                &mut state,
                &json!({
                    "request_id": "invalid",
                    "command": "analysis.compute",
                    "case_id": "case-a",
                    "db_path": db_path.display().to_string(),
                    "payload": {"command": "materialize-rule-txn-index", "args": args}
                })
                .to_string(),
            );
            assert_eq!(
                response.pointer("/error/code").and_then(Value::as_str),
                Some("data_engine_request_contract_invalid")
            );
            assert_eq!(state.duckdb_sessions.len(), 1);
            assert!(owner_path.exists());
        }

        drop(state);
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn owner_mutex_name_uses_db_path_hash_contract() {
        let db_path = temp_case_db("mutex-name");
        let canonical = canonical_db_path(&db_path);
        let hash = db_path_hash(&canonical);
        let name = owner_mutex_name(&hash);

        assert_eq!(hash.len(), SHA256_HEX_BYTES);
        assert!(hash.chars().all(|ch| ch.is_ascii_hexdigit()));
        assert_eq!(name, format!("Local\\AnalytixDuckDB_{hash}"));
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn owner_sidecar_contains_only_redacted_sha256_identity() {
        let db_path = temp_case_db_path("owner-redacted-payload");
        let canonical = canonical_db_path(&db_path);
        let owner_path = owner_file_path(&canonical);
        let guard = OwnerGuard::acquire(&db_path).expect("acquire owner");

        let encoded = fs::read_to_string(&owner_path).expect("read owner sidecar");
        let payload: Value = serde_json::from_str(&encoded).expect("owner JSON");
        let object = payload.as_object().expect("owner object");
        assert_eq!(
            object.keys().cloned().collect::<HashSet<_>>(),
            [
                "version".to_string(),
                "pid".to_string(),
                "db_path_hash".to_string(),
                "started_at_ms".to_string(),
                "owner_epoch".to_string(),
            ]
            .into_iter()
            .collect()
        );
        assert!(valid_sha256_hex(
            payload["db_path_hash"].as_str().expect("path hash")
        ));
        assert!(valid_sha256_hex(
            payload["owner_epoch"].as_str().expect("owner epoch")
        ));
        assert!(!encoded.contains(&canonical.display().to_string()));
        assert!(!encoded.contains("\"db_path\""));
        assert!(!encoded.contains("\"exe\""));

        drop(guard);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn malformed_owner_sidecar_fails_closed_without_overwrite() {
        let db_path = temp_case_db_path("owner-invalid-sidecar");
        let canonical = canonical_db_path(&db_path);
        let owner_path = owner_file_path(&canonical);
        let marker = b"private-account-6222020202020202020";
        write_private_owner_fixture(&owner_path, marker);

        let error = OwnerGuard::acquire(&db_path).expect_err("invalid owner must conflict");

        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
        assert_eq!(fs::read(&owner_path).expect("owner remains"), marker);
        assert!(!format!("{error:#}").contains("6222020202020202020"));
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn live_owner_sidecar_fails_closed_even_when_advisory_lock_is_free() {
        let db_path = temp_case_db_path("owner-live-sidecar");
        let canonical = canonical_db_path(&db_path);
        let owner_path = owner_file_path(&canonical);
        let owner = new_owner_record(&db_path_hash(&canonical));
        let encoded = serde_json::to_vec(&owner).expect("serialize live owner");
        write_private_owner_fixture(&owner_path, &encoded);

        let error = OwnerGuard::acquire(&db_path).expect_err("live owner must conflict");

        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
        assert!(format!("{error:#}").contains("owner_process_live"));
        assert_eq!(fs::read(&owner_path).expect("owner remains"), encoded);
        cleanup_temp_case_db(&db_path);
    }

    #[cfg(unix)]
    #[test]
    fn known_dead_owner_is_reclaimed_only_after_liveness_check() {
        let db_path = temp_case_db_path("owner-known-dead");
        let canonical = canonical_db_path(&db_path);
        let owner_path = owner_file_path(&canonical);
        let hash = db_path_hash(&canonical);
        let dead_pid = ((i32::MAX - 1024)..=i32::MAX)
            .rev()
            .map(|pid| pid as u32)
            .find(|pid| matches!(process_exists(*pid), Ok(false)))
            .expect("known absent POSIX pid");
        let stale = OwnerRecord {
            version: OWNER_RECORD_VERSION,
            pid: dead_pid,
            db_path_hash: hash.clone(),
            started_at_ms: 1,
            owner_epoch: "a".repeat(SHA256_HEX_BYTES),
        };
        write_private_owner_fixture(
            &owner_path,
            &serde_json::to_vec(&stale).expect("serialize stale owner"),
        );

        let guard = OwnerGuard::acquire(&db_path).expect("reclaim known-dead owner");
        let current = read_owner_record(&owner_path).expect("read current owner");

        assert_eq!(current.pid, std::process::id());
        assert_ne!(current.owner_epoch, stale.owner_epoch);
        drop(guard);
        cleanup_temp_case_db(&db_path);
    }

    #[cfg(unix)]
    #[test]
    fn oversized_owner_sidecar_fails_closed_without_unbounded_read_or_reflection() {
        let db_path = temp_case_db_path("owner-oversized-sidecar");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let marker = b"private-account-6222020202020202020";
        let mut payload = vec![b'x'; MAX_OWNER_RECORD_BYTES + 1];
        payload[..marker.len()].copy_from_slice(marker);
        write_private_owner_fixture(&owner_path, &payload);

        let error = OwnerGuard::acquire(&db_path).expect_err("oversized owner must conflict");

        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
        assert_eq!(
            fs::metadata(&owner_path).expect("owner remains").len(),
            payload.len() as u64
        );
        assert!(!format!("{error:#}").contains("6222020202020202020"));
        cleanup_temp_case_db(&db_path);
    }

    #[cfg(unix)]
    #[test]
    fn owner_and_lock_require_private_single_link_files() {
        use std::os::unix::fs::PermissionsExt;

        for surface in ["owner", "lock"] {
            let db_path = temp_case_db_path(&format!("{surface}-file-authority"));
            let canonical = canonical_db_path(&db_path);
            let target = if surface == "owner" {
                owner_file_path(&canonical)
            } else {
                owner_lock_file_path(&canonical)
            };
            let alias = target.with_extension("attacker-link");
            if surface == "owner" {
                let owner = new_owner_record(&db_path_hash(&canonical));
                write_private_owner_fixture(
                    &target,
                    &serde_json::to_vec(&owner).expect("serialize owner"),
                );
            } else {
                write_private_owner_fixture(&target, b"");
            }
            fs::hard_link(&target, &alias).expect("create hostile hardlink");

            let error =
                OwnerGuard::acquire(&db_path).expect_err("hardlinked authority file must conflict");

            assert_eq!(classify_error(&error), "data_engine_owner_conflict");
            assert!(target.exists());
            assert!(alias.exists());
            fs::remove_file(&alias).expect("remove hostile hardlink");
            fs::set_permissions(&target, fs::Permissions::from_mode(0o644))
                .expect("make authority file public");

            let error =
                OwnerGuard::acquire(&db_path).expect_err("public authority file must conflict");

            assert_eq!(classify_error(&error), "data_engine_owner_conflict");
            cleanup_temp_case_db(&db_path);
        }
    }

    #[cfg(unix)]
    #[test]
    fn owner_file_validation_rejects_path_to_fd_replacement() {
        let db_path = temp_case_db_path("owner-path-replacement");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let moved_path = owner_path.with_extension("moved-owner");
        write_private_owner_fixture(&owner_path, b"original");
        let opened = OpenOptions::new()
            .read(true)
            .open(&owner_path)
            .expect("open original owner");
        fs::rename(&owner_path, &moved_path).expect("move original owner");
        write_private_owner_fixture(&owner_path, b"replacement");

        let error = validate_open_owner_file(&owner_path, &opened)
            .expect_err("replacement path must not validate against original fd");

        assert!(format!("{error:#}").contains("identity_changed"));
        assert!(!format!("{error:#}").contains(&owner_path.display().to_string()));
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn duckdb_session_owner_file_lives_until_close() {
        let db_path = temp_case_db_path("duckdb-session-owner");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let open_request = json!({
            "request_id": "open",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });

        let open_response = handle_line_with_state(&mut state, &open_request.to_string());

        assert_eq!(open_response.get("ok").and_then(Value::as_bool), Some(true));
        assert!(owner_path.exists());
        let session_id = open_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("session id");
        let close_request = json!({
            "request_id": "close",
            "command": "duckdb.close_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": session_id}
        });
        let close_response = handle_line_with_state(&mut state, &close_request.to_string());

        assert_eq!(
            close_response.get("ok").and_then(Value::as_bool),
            Some(true)
        );
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn engine_state_rejects_second_session_for_same_database() {
        let db_path = temp_case_db_path("single-engine-state-owner");
        let mut state = EngineState::default();
        let first = json!({
            "request_id": "first",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let first_response = handle_line_with_state(&mut state, &first.to_string());
        let session_id = first_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("first session")
            .to_string();
        let second = json!({
            "request_id": "second",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": true}
        });

        let second_response = handle_line_with_state(&mut state, &second.to_string());

        assert_eq!(
            second_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_owner_conflict")
        );
        assert_eq!(state.duckdb_sessions.len(), 1);
        let close = json!({
            "request_id": "close",
            "command": "duckdb.close_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": session_id}
        });
        assert_eq!(
            handle_line_with_state(&mut state, &close.to_string())
                .get("ok")
                .and_then(Value::as_bool),
            Some(true)
        );
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn duckdb_session_query_executes_write_and_read_sql() {
        let db_path = temp_case_db_path("duckdb-session-query");
        let mut state = EngineState::default();
        let open_request = json!({
            "request_id": "open",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let open_response = handle_line_with_state(&mut state, &open_request.to_string());
        assert_eq!(open_response.get("ok").and_then(Value::as_bool), Some(true));
        let session_id = open_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("session id")
            .to_string();
        for sql in [
            "CREATE TABLE items(value INTEGER)",
            "INSERT INTO items VALUES (7)",
        ] {
            let request = json!({
                "request_id": "query",
                "command": "duckdb.query",
                "case_id": "case-1",
                "db_path": db_path.display().to_string(),
                "payload": {"session_id": session_id, "sql": sql, "params": []}
            });
            let response = handle_line_with_state(&mut state, &request.to_string());
            assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
        }
        let select_request = json!({
            "request_id": "select",
            "command": "duckdb.query",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": session_id, "sql": "SELECT value FROM items", "params": []}
        });
        let select_response = handle_line_with_state(&mut state, &select_request.to_string());
        assert_eq!(
            select_response.get("ok").and_then(Value::as_bool),
            Some(true)
        );
        assert_eq!(
            select_response
                .pointer("/data/rows/0/0")
                .and_then(Value::as_i64),
            Some(7)
        );
        let close_request = json!({
            "request_id": "close",
            "command": "duckdb.close_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": session_id}
        });
        let _ = handle_line_with_state(&mut state, &close_request.to_string());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn read_only_session_rejects_writes_and_redacts_private_input() {
        let db_path = temp_case_db_path("duckdb-read-only");
        let mut state = EngineState::default();
        let write_open = json!({
            "request_id": "open-write",
            "command": "duckdb.open_session",
            "case_id": "case-private-6222021234567890123",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let write_response = handle_line_with_state(&mut state, &write_open.to_string());
        let write_session = write_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("write session")
            .to_string();
        let close_write = json!({
            "request_id": "close-write",
            "command": "duckdb.close_session",
            "case_id": "case-private-6222021234567890123",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": write_session}
        });
        let _ = handle_line_with_state(&mut state, &close_write.to_string());

        let read_open = json!({
            "request_id": "open-read",
            "command": "duckdb.open_session",
            "case_id": "case-private-6222021234567890123",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": true}
        });
        let read_response = handle_line_with_state(&mut state, &read_open.to_string());
        let read_session = read_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("read session")
            .to_string();
        let forbidden = json!({
            "request_id": "write-private",
            "command": "duckdb.query",
            "case_id": "case-private-6222021234567890123",
            "db_path": db_path.display().to_string(),
            "payload": {
                "session_id": read_session,
                "sql": "CREATE TABLE private_6222021234567890123(value TEXT)",
                "params": []
            }
        });
        let forbidden_response = handle_line_with_state(&mut state, &forbidden.to_string());

        assert_eq!(
            forbidden_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("duckdb_read_only_violation")
        );
        assert_eq!(
            forbidden_response
                .pointer("/error/message")
                .and_then(Value::as_str),
            Some("The DuckDB operation exceeded the session's read-only authority.")
        );
        let serialized = forbidden_response.to_string();
        assert!(!serialized.contains("6222021234567890123"));
        assert!(!serialized.contains(db_path.to_string_lossy().as_ref()));
        assert!(forbidden_response.pointer("/diagnostics/case_id").is_none());
        assert!(forbidden_response.pointer("/diagnostics/db_path").is_none());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn read_only_query_enforces_row_byte_and_timeout_limits() {
        let db_path = temp_case_db_path("duckdb-query-limits");
        let mut state = EngineState::default();
        let initialize = json!({
            "request_id": "initialize",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let initialized = handle_line_with_state(&mut state, &initialize.to_string());
        let initial_session = initialized
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("initial session")
            .to_string();
        let close = json!({
            "request_id": "close",
            "command": "duckdb.close_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": initial_session}
        });
        let _ = handle_line_with_state(&mut state, &close.to_string());
        let read_open = json!({
            "request_id": "read",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": true}
        });
        let read_response = handle_line_with_state(&mut state, &read_open.to_string());
        let read_session = read_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("read session")
            .to_string();
        let bounded = json!({
            "request_id": "bounded",
            "command": "duckdb.query",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "session_id": read_session,
                "sql": "SELECT i FROM range(5) values_table(i)",
                "params": [],
                "row_limit": 2,
                "max_result_bytes": 4096,
                "timeout_ms": 1000
            }
        });
        let bounded_response = handle_line_with_state(&mut state, &bounded.to_string());
        assert_eq!(
            bounded_response
                .pointer("/data/row_count")
                .and_then(Value::as_u64),
            Some(2)
        );
        assert_eq!(
            bounded_response
                .pointer("/data/truncated")
                .and_then(Value::as_bool),
            Some(true)
        );

        let timed = json!({
            "request_id": "timed",
            "command": "duckdb.query",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "session_id": read_session,
                "sql": "SELECT sum(sin(i)) FROM range(1000000000) values_table(i)",
                "params": [],
                "row_limit": 1,
                "timeout_ms": 1
            }
        });
        let timed_response = handle_line_with_state(&mut state, &timed.to_string());
        assert_eq!(
            timed_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("duckdb_query_timeout")
        );
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn duckdb_query_default_max_result_bytes_fails_closed_above_three_mib() {
        let db_path = temp_case_db_path("duckdb-default-byte-limit");
        let mut state = EngineState::default();
        let initialize = json!({
            "request_id": "initialize",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let initialized = handle_line_with_state(&mut state, &initialize.to_string());
        let initial_session = initialized
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("initial session")
            .to_string();
        let close = json!({
            "request_id": "close",
            "command": "duckdb.close_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"session_id": initial_session}
        });
        let _ = handle_line_with_state(&mut state, &close.to_string());
        let read_open = json!({
            "request_id": "read",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": true}
        });
        let read_response = handle_line_with_state(&mut state, &read_open.to_string());
        let read_session = read_response
            .pointer("/data/session_id")
            .and_then(Value::as_str)
            .expect("read session")
            .to_string();
        let oversized = json!({
            "request_id": "oversized",
            "command": "duckdb.query",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "session_id": read_session,
                "sql": "SELECT repeat('x', 3145729) AS payload",
                "params": [],
                "row_limit": 1,
                "timeout_ms": 1000
            }
        });

        let oversized_response = handle_line_with_state(&mut state, &oversized.to_string());

        assert_eq!(
            oversized_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("duckdb_result_limit_exceeded")
        );
        drop(state);
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn owner_commands_close_lingering_duckdb_sessions_for_same_db() {
        let db_path = temp_case_db_path("close-lingering-session");
        let owner_path = owner_file_path(&canonical_db_path(&db_path));
        let mut state = EngineState::default();
        let open_request = json!({
            "request_id": "open",
            "command": "duckdb.open_session",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {"read_only": false}
        });
        let open_response = handle_line_with_state(&mut state, &open_request.to_string());
        assert_eq!(open_response.get("ok").and_then(Value::as_bool), Some(true));
        assert_eq!(state.duckdb_sessions.len(), 1);
        assert!(owner_path.exists());

        let analysis_request = json!({
            "request_id": "analysis",
            "command": "analysis.compute",
            "case_id": "case-1",
            "db_path": db_path.display().to_string(),
            "payload": {
                "command": "missing-inner",
                "args": {"argv": []}
            }
        });
        let analysis_response = handle_line_with_state(&mut state, &analysis_request.to_string());

        assert_eq!(
            analysis_response.get("ok").and_then(Value::as_bool),
            Some(false)
        );
        assert_eq!(
            analysis_response
                .pointer("/error/code")
                .and_then(Value::as_str),
            Some("data_engine_unsupported_command")
        );
        assert!(state.duckdb_sessions.is_empty());
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn owner_conflict_errors_are_classified() {
        let error = anyhow!("data_engine_owner_conflict: db_path_hash=abc owner={{}}");
        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
    }

    #[cfg(unix)]
    #[test]
    fn unix_advisory_lock_rejects_independent_process_owner() {
        use std::process::{Command, Stdio};

        let db_path = temp_case_db_path("independent-process-owner");
        let parent = db_path.parent().expect("database parent");
        let ready_path = parent.join("owner-ready");
        let release_path = parent.join("owner-release");
        let mut child = Command::new(std::env::current_exe().expect("current test executable"))
            .arg("--exact")
            .arg("tests::owner_lock_subprocess_holder")
            .arg("--nocapture")
            .env("ANALYTIX_TEST_OWNER_DB", &db_path)
            .env("ANALYTIX_TEST_OWNER_READY", &ready_path)
            .env("ANALYTIX_TEST_OWNER_RELEASE", &release_path)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn independent owner");

        let ready = wait_for_test_path(&ready_path, Duration::from_secs(5));
        let acquisition = if ready {
            Some(OwnerGuard::acquire(&db_path))
        } else {
            None
        };
        let _ = fs::write(&release_path, b"release");
        let status = child.wait().expect("wait independent owner");

        assert!(ready, "independent owner did not acquire lock");
        assert!(status.success(), "independent owner process failed");
        let error = acquisition
            .expect("acquisition attempted")
            .expect_err("second process must not acquire owner lock");
        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
        assert!(format!("{error:#}").contains("advisory_lock_held"));
        let _ = fs::remove_file(&ready_path);
        let _ = fs::remove_file(&release_path);
        cleanup_temp_case_db(&db_path);
    }

    #[cfg(unix)]
    #[test]
    fn owner_lock_subprocess_holder() {
        let Ok(db_path) = std::env::var("ANALYTIX_TEST_OWNER_DB") else {
            return;
        };
        let ready_path = PathBuf::from(
            std::env::var("ANALYTIX_TEST_OWNER_READY").expect("ready path authority"),
        );
        let release_path = PathBuf::from(
            std::env::var("ANALYTIX_TEST_OWNER_RELEASE").expect("release path authority"),
        );
        let _guard = OwnerGuard::acquire(Path::new(&db_path)).expect("subprocess owner");
        fs::write(&ready_path, b"ready").expect("publish owner readiness");
        assert!(
            wait_for_test_path(&release_path, Duration::from_secs(15)),
            "owner release timed out"
        );
    }

    #[cfg(windows)]
    #[test]
    fn windows_named_mutex_conflict_returns_owner_conflict() {
        use std::os::windows::ffi::OsStrExt;
        use std::sync::mpsc;
        use std::time::Duration;
        use windows_sys::Win32::Foundation::{CloseHandle, GetLastError};
        use windows_sys::Win32::System::Threading::{CreateMutexW, ReleaseMutex};

        let db_path = temp_case_db("windows-mutex-conflict");
        let canonical = canonical_db_path(&db_path);
        let hash = db_path_hash(&canonical);
        let owner_path = owner_file_path(&canonical);
        create_owner_file(&owner_path, &hash).expect("write test owner file");
        let mutex_name = owner_mutex_name(&hash);
        let (ready_tx, ready_rx) = mpsc::channel::<Result<()>>();
        let (release_tx, release_rx) = mpsc::channel::<()>();
        let holder = std::thread::spawn(move || {
            let wide_name = std::ffi::OsStr::new(&mutex_name)
                .encode_wide()
                .chain(std::iter::once(0))
                .collect::<Vec<u16>>();
            unsafe {
                let handle = CreateMutexW(std::ptr::null(), 1, wide_name.as_ptr());
                if handle.is_null() {
                    let _ = ready_tx.send(Err(anyhow!("CreateMutexW failed: {}", GetLastError())));
                    return;
                }
                let _ = ready_tx.send(Ok(()));
                let _ = release_rx.recv_timeout(Duration::from_secs(5));
                let _ = ReleaseMutex(handle);
                CloseHandle(handle);
            }
        });
        ready_rx
            .recv_timeout(Duration::from_secs(5))
            .expect("mutex holder ready")
            .expect("mutex holder created mutex");

        let error = DbOwnerMutex::acquire(&canonical, &hash, &owner_path)
            .expect_err("second owner must conflict");

        assert_eq!(classify_error(&error), "data_engine_owner_conflict");
        let message = format!("{error:#}");
        assert!(message.contains("data_engine_owner_conflict"));
        assert!(message.contains(&hash));
        assert!(!message.contains(&canonical.display().to_string()));
        let _ = release_tx.send(());
        holder.join().expect("mutex holder joined");
        let _ = fs::remove_file(&owner_path);
        assert!(!owner_path.exists());
        cleanup_temp_case_db(&db_path);
    }

    #[test]
    fn duckdb_lock_errors_are_classified() {
        let cases = [
            "Cannot open file case.duckdb: another program is using this file",
            "Could not set lock on file",
            "Conflicting lock is held",
            "另一个程序正在使用此文件",
            "进程无法访问",
        ];
        for message in cases {
            let error = anyhow!(message);
            assert_eq!(classify_error(&error), "external_process_holds_duckdb");
        }
    }

    fn temp_case_db(label: &str) -> PathBuf {
        let path = temp_case_db_path(label);
        fs::write(&path, b"not-a-real-duckdb").expect("write temp case db marker");
        path
    }

    fn seed_txn_daily_source(path: &Path) {
        Connection::open(path)
            .expect("open transaction verifier fixture")
            .execute_batch(
                "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
                 INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7); \
                 CREATE TABLE fc_transaction_norm( \
                   id BIGINT, case_id TEXT, txn_ts TIMESTAMP, acct_no TEXT, amount DOUBLE, \
                   clean_amount TEXT, counterparty_acct TEXT, counterparty_name TEXT, \
                   counterparty_bank TEXT, currency TEXT, dc_flag TEXT, summary TEXT, \
                   remark TEXT, file_id TEXT, row_no BIGINT, clean_invalid INTEGER, \
                   clean_failed INTEGER, clean_reversal INTEGER \
                 ); \
                 INSERT INTO fc_transaction_norm VALUES \
                   (1, 'case-a', TIMESTAMP '2026-01-01 01:02:03', '6222021234567890', 12.5, \
                    '12.50', 'CP-1', '对手一', '银行甲', 'CNY', '进', '工资入账', '', \
                    'file-1', 1001, 0, 0, 0), \
                   (2, 'case-a', TIMESTAMP '2026-01-02 01:02:03', '6222021234567890', 0.0, \
                    '0.00', 'CP-2', '对手二', '银行乙', 'CNY', '出', '转出', 'POS', \
                    'file-1', 1002, 0, 0, 0); \
                 CREATE TABLE import_file_log( \
                   file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, \
                   rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT \
                 ); \
                 INSERT INTO import_file_log VALUES ( \
                   'file-1', 'case-a', 'fc_transaction', \
                   'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', \
                   2, '已完成', 'done' \
                 );",
            )
            .expect("seed transaction verifier fixture");
    }

    fn transaction_source_row_payload() -> Value {
        json!({
            "caseId": "case-a",
            "bindingKeyDigest": "1".repeat(64),
            "parsedGenerationIdentitySha256": "2".repeat(64),
            "relation": "fc_transaction_raw",
            "maxRows": 100,
            "cursor": null,
        })
    }

    #[cfg(unix)]
    fn assert_bounded_wire_object_key_order(
        frame: &[u8],
        start_marker: &[u8],
        end_marker: &[u8],
        keys: &[&str],
    ) {
        let start = frame
            .windows(start_marker.len())
            .position(|window| window == start_marker)
            .map(|position| position + start_marker.len())
            .expect("source-row wire object start missing");
        let tail = &frame[start..];
        let end = tail
            .windows(end_marker.len())
            .position(|window| window == end_marker)
            .expect("source-row wire object end missing");
        let object = &tail[..end];
        let mut offset = 0usize;
        for key in keys {
            let marker = format!("\"{key}\":");
            let relative = object[offset..]
                .windows(marker.len())
                .position(|window| window == marker.as_bytes())
                .expect("source-row wire key missing or out of order");
            offset += relative + marker.len();
        }
    }

    #[cfg(unix)]
    fn assert_transaction_source_row_bounded_wire_order(frame: &[u8]) {
        assert!(frame.len() <= MAX_RESPONSE_FRAME_BYTES);
        assert_eq!(frame.last(), Some(&b'\n'));
        assert!(!frame[..frame.len() - 1].contains(&b'\n'));
        assert_bounded_wire_object_key_order(
            frame,
            b"\"data\":{",
            b"},\"diagnostics\":",
            &[
                "schemaVersion",
                "operation",
                "caseId",
                "bindingKeyDigest",
                "parsedGenerationIdentitySha256",
                "relation",
                "materializationIdentity",
                "producerContentContract",
                "producerContentId",
                "producerContentManifestSha256",
                "rawArtifactManifestSha256",
                "duckdbContentSnapshotDigest",
                "duckdbSnapshotManifestSha256",
                "analyticalSchemaDigest",
                "parserId",
                "parserVersion",
                "locatorOrdering",
                "sourceProofStatus",
                "requiredHostRawReplayProfile",
                "hostRawReplayRequired",
                "sourceSnapshot",
                "inventory",
                "rows",
                "nextCursor",
                "complete",
                "pageDigest",
            ],
        );
        assert_bounded_wire_object_key_order(
            frame,
            b"\"sourceSnapshot\":{",
            b"},\"inventory\":[",
            &[
                "sourceRevision",
                "sourceRowCount",
                "sourceMaxTxnTs",
                "sourceMaxId",
                "acceptedRowCount",
                "rejectedRowCount",
                "duplicateRowCount",
                "inventoryDigest",
                "sourceSnapshotDigest",
            ],
        );
        assert_bounded_wire_object_key_order(
            frame,
            b"\"inventory\":[{",
            b"}],\"rows\":[",
            &[
                "sourceFileId",
                "sourceFileIdDigest",
                "privateImportFileId",
                "sourceArtifactSha256",
                "fileType",
                "rowCount",
                "acceptedRowCount",
                "rejectedRowCount",
            ],
        );
        assert_bounded_wire_object_key_order(
            frame,
            b"\"rows\":[{",
            b"}],\"nextCursor\":",
            &[
                "sourceFileId",
                "sourceFileIdDigest",
                "sourceRowNumber",
                "sourceArtifactSha256",
                "canonicalRowSha256",
                "canonicalTypedRow",
                "disposition",
            ],
        );
        assert_bounded_wire_object_key_order(
            frame,
            b"\"canonicalTypedRow\":[{",
            b"}],\"disposition\":",
            &["name", "scalar"],
        );
        assert_bounded_wire_object_key_order(frame, b"\"scalar\":{", b"}}", &["kind", "value"]);

        let cursor = analytix_analysis_compute::FundsTransactionSourceRowCursorV1 {
            source_file_id_digest: "1".repeat(64),
            source_row_number: 7,
        };
        let cursor_frame =
            encode_bounded_response(&cursor, 256).expect("encode bounded source-row cursor");
        let expected = format!(
            "{{\"sourceFileIdDigest\":\"{}\",\"sourceRowNumber\":7}}\n",
            "1".repeat(64)
        );
        assert_eq!(cursor_frame, expected.as_bytes());
    }

    #[cfg(unix)]
    fn canonical_csv_build_payload() -> Value {
        json!({
            "caseId": "case-a",
            "profile": "canonical_direct_csv_v1",
            "privateImportFileId": "0123456789abcdefabcd",
            "sourceRevision": 7,
            "rawArtifactManifestSha256": "3".repeat(64),
        })
    }

    #[cfg(unix)]
    fn canonical_csv_source_fixture() -> Vec<u8> {
        concat!(
            "交易卡号,交易账号,账户开户名称,开户人证件号码,交易时间,交易金额,交易余额,收付标志,交易对手账卡号,现金标志,对手户名,对手身份证号,对手开户银行,摘要说明,交易币种,交易网点名称,交易网点代码,交易发生地,交易是否成功,传票号,终端号,IP地址,MAC地址,对手交易余额,交易流水号,日志号,凭证种类,凭证号,交易柜员号,商户名称,商户号,备注,交易类型,查询反馈结果原因\r\n",
            "6222021234567890001,1000001,合成账户甲,SYNTHID0001,2026-08-27 10:00:00,12.5,1000,进,CP001,否,合成对手甲,SYNTHCPID001,合成银行,合成交易,CNY,合成网点,001,合成地,是,V001,T001,127.0.0.1,000000000001,88.5,TXN001,LOG001,SYNTH,VID001,TEL001,合成商户,M001,AB_R2_DATA_ENGINE_PRIVATE_SENTINEL,synthetic,synthetic\r\n"
        )
        .as_bytes()
        .to_vec()
    }

    fn account_flow_payload(materialized: &Value) -> Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "contextEpoch": 7,
            "contextDigest": "2".repeat(64),
            "caseBindingHash": "3".repeat(64),
            "expectedProducerContentId": materialized
                .pointer("/data/producerContentId")
                .and_then(Value::as_str)
                .expect("producer content id"),
            "expectedProducerManifestSha256": materialized
                .pointer("/data/producerContentManifestSha256")
                .and_then(Value::as_str)
                .expect("producer manifest sha256"),
            "subjectRef": format!("cer1_{}", "a".repeat(64)),
            "resolvedAccountKey": "6222021234567890",
            "subjectResolutionDigest": "6".repeat(64),
            "startInclusive": "2026-01-01T00:00:00Z",
            "endInclusive": "2026-01-03T00:00:00Z",
            "evidenceRowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
            "scanCap": 100,
        })
    }

    fn account_ingress_payload(materialized: &Value) -> Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "contextEpoch": 7,
            "contextDigest": "2".repeat(64),
            "caseBindingHash": "3".repeat(64),
            "expectedProducerContentId": materialized
                .pointer("/data/producerContentId")
                .and_then(Value::as_str)
                .expect("producer content id"),
            "expectedProducerManifestSha256": materialized
                .pointer("/data/producerContentManifestSha256")
                .and_then(Value::as_str)
                .expect("producer manifest sha256"),
            "candidates": [{
                "ordinal": 0,
                "normalizedCandidate": "6222021234567890"
            }],
        })
    }

    fn direct_source_preview_payload() -> Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "caseBindingHash": "2".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "3".repeat(64)),
            "expectedProducerManifestSha256": "4".repeat(64),
            "fields": ["transactionTime", "account", "amountText"],
            "rowOffset": 0,
            "rowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
        })
    }

    fn deterministic_cleaning_payload() -> Value {
        let rule_digest = format!(
            "{:x}",
            Sha256::digest(
                analytix_analysis_compute::FUNDS_DETERMINISTIC_CLEANING_RULE_CONTRACT.as_bytes()
            )
        );
        let mut generation = Sha256::new();
        generation.update(b"AnalytixFundsDeterministicCleaningGenerationV1\0");
        generation.update(rule_digest.as_bytes());
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "caseBindingHash": "2".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "3".repeat(64)),
            "expectedProducerManifestSha256": "4".repeat(64),
            "ruleGeneration": format!("tlgen1_{:x}", generation.finalize()),
            "ruleDigest": rule_digest,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
        })
    }

    #[cfg(unix)]
    #[test]
    fn funds_inherited_snapshot_subprocess_helper() {
        let Ok(expectation) = std::env::var("ANALYTIX_TEST_INHERITED_SNAPSHOT_EXPECTATION") else {
            return;
        };
        let command = std::env::var("ANALYTIX_TEST_INHERITED_SNAPSHOT_COMMAND")
            .expect("inherited snapshot helper command");
        let payload = std::env::var("ANALYTIX_TEST_INHERITED_SNAPSHOT_PAYLOAD")
            .expect("inherited snapshot helper payload");
        let payload: Value = serde_json::from_str(&payload).expect("parse helper payload");
        let request_line = json!({
            "request_id": "inherited-snapshot-helper",
            "command": command,
            "case_id": "case-a",
            "payload": payload
        })
        .to_string();
        let response_frame =
            handle_line_frame_with_state(&mut EngineState::default(), &request_line);
        let encoded_frame = encode_bounded_response(&response_frame, MAX_RESPONSE_FRAME_BYTES)
            .expect("encode bounded helper response");
        let response: Value =
            serde_json::from_slice(&encoded_frame).expect("parse bounded helper response");
        assert!(!encoded_frame
            .windows(b"db_path".len())
            .any(|window| window == b"db_path"));
        assert!(!encoded_frame
            .windows(ACCOUNT_FLOW_SNAPSHOT_FD_PATH.len())
            .any(|window| window == ACCOUNT_FLOW_SNAPSHOT_FD_PATH.as_bytes()));
        if let Ok(sentinel) = std::env::var("ANALYTIX_TEST_INHERITED_SNAPSHOT_PATH_SENTINEL") {
            assert!(!sentinel.is_empty());
            assert!(!encoded_frame
                .windows(sentinel.len())
                .any(|window| window == sentinel.as_bytes()));
        }

        match expectation.as_str() {
            "success" => {
                assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
                assert_eq!(
                    response
                        .pointer("/diagnostics/db_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response
                        .pointer("/data/inflowMinor")
                        .and_then(Value::as_str),
                    Some("1250")
                );
                assert_eq!(
                    response
                        .pointer("/data/outflowMinor")
                        .and_then(Value::as_str),
                    Some("0")
                );
                assert_eq!(
                    response.pointer("/data/netMinor").and_then(Value::as_str),
                    Some("1250")
                );
                assert_eq!(
                    response
                        .pointer("/data/transactionCount")
                        .and_then(Value::as_u64),
                    Some(2)
                );
                assert_eq!(
                    response
                        .pointer("/data/evidenceRows/0/sourceFileId")
                        .and_then(Value::as_str),
                    Some("file-1")
                );
            }
            "account_ingress_success" => {
                assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
                assert_eq!(
                    response
                        .pointer("/diagnostics/command")
                        .and_then(Value::as_str),
                    Some("funds.resolve_account_ingress")
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/db_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response
                        .pointer("/data/resolutions/0/ordinal")
                        .and_then(Value::as_u64),
                    Some(0)
                );
                assert_eq!(
                    response
                        .pointer("/data/resolutions/0/disposition")
                        .and_then(Value::as_str),
                    Some("resolved")
                );
                assert_eq!(
                    response
                        .pointer("/data/resolutions/0/entityType")
                        .and_then(Value::as_str),
                    Some("bank_account_number")
                );
                assert!(!encoded_frame
                    .windows(b"6222021234567890".len())
                    .any(|window| window == b"6222021234567890"));
            }
            "canonical_build_success" => {
                assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
                assert_eq!(
                    response
                        .pointer("/diagnostics/command")
                        .and_then(Value::as_str),
                    Some("funds.build_canonical_csv_snapshot_v1")
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/case_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/db_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response
                        .pointer("/data/sourceRowCount")
                        .and_then(Value::as_u64),
                    Some(1)
                );
            }
            "transaction_source_row_success" => {
                assert_transaction_source_row_bounded_wire_order(&encoded_frame);
                assert_eq!(response.get("ok").and_then(Value::as_bool), Some(true));
                assert_eq!(
                    response
                        .pointer("/diagnostics/command")
                        .and_then(Value::as_str),
                    Some("funds.transaction_source_row_page_v1")
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/case_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/db_bound")
                        .and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(
                    response.pointer("/data/operation").and_then(Value::as_str),
                    Some("funds.transaction_source_row_page_v1")
                );
                assert_eq!(
                    response
                        .pointer("/data/rows")
                        .and_then(Value::as_array)
                        .map(Vec::len),
                    Some(1)
                );
                assert_eq!(
                    response.pointer("/data/complete").and_then(Value::as_bool),
                    Some(true)
                );
                assert_eq!(response.pointer("/data/nextCursor"), Some(&Value::Null));
            }
            "invalid" => {
                assert_eq!(response.get("ok").and_then(Value::as_bool), Some(false));
                assert_eq!(
                    response.pointer("/error/code").and_then(Value::as_str),
                    Some("funds_account_flow_failed")
                );
                assert_eq!(
                    response
                        .pointer("/diagnostics/db_bound")
                        .and_then(Value::as_bool),
                    Some(false)
                );
            }
            other => panic!("unexpected helper expectation: {other}"),
        }
    }

    #[cfg(unix)]
    fn run_inherited_snapshot_helper(
        command_name: &str,
        expectation: &str,
        snapshot: Option<&File>,
        output: Option<&File>,
        payload: &Value,
        path_sentinel: Option<&Path>,
        working_directory: Option<&Path>,
    ) -> std::process::Output {
        use std::os::fd::AsRawFd;
        use std::os::unix::process::CommandExt;
        use std::process::Command;

        let duplicate = |file: &File| {
            let fd = unsafe { libc::fcntl(file.as_raw_fd(), libc::F_DUPFD_CLOEXEC, 6) };
            assert!(fd >= 6, "duplicate inherited test descriptor");
            // SAFETY: fcntl returned a new descriptor owned by this File.
            unsafe { File::from_raw_fd(fd) }
        };
        let source_duplicate = snapshot.map(duplicate);
        let output_duplicate = output.map(duplicate);
        let source_fd = source_duplicate.as_ref().map(AsRawFd::as_raw_fd);
        let output_fd = output_duplicate.as_ref().map(AsRawFd::as_raw_fd);
        let mut command = Command::new(std::env::current_exe().expect("current test executable"));
        command
            .arg("--exact")
            .arg("tests::funds_inherited_snapshot_subprocess_helper")
            .arg("--nocapture")
            .env("ANALYTIX_TEST_INHERITED_SNAPSHOT_COMMAND", command_name)
            .env("ANALYTIX_TEST_INHERITED_SNAPSHOT_EXPECTATION", expectation)
            .env(
                "ANALYTIX_TEST_INHERITED_SNAPSHOT_PAYLOAD",
                serde_json::to_string(payload).expect("serialize helper payload"),
            );
        if let Some(directory) = working_directory {
            command.current_dir(directory);
        }
        if let Some(path) = path_sentinel {
            command.env(
                "ANALYTIX_TEST_INHERITED_SNAPSHOT_PATH_SENTINEL",
                path.as_os_str(),
            );
        } else {
            command.env_remove("ANALYTIX_TEST_INHERITED_SNAPSHOT_PATH_SENTINEL");
        }

        // SAFETY: only async-signal-safe descriptor operations run between
        // fork and exec. The duplicated Files remain alive until spawn returns.
        unsafe {
            command.pre_exec(move || {
                for (fd, target) in [
                    (source_fd, ACCOUNT_FLOW_SNAPSHOT_FD),
                    (output_fd, CANONICAL_CSV_OUTPUT_FD),
                ] {
                    match fd {
                        Some(fd) => {
                            if libc::dup2(fd, target) < 0 {
                                return Err(io::Error::last_os_error());
                            }
                        }
                        None => {
                            if libc::close(target) != 0 {
                                let error = io::Error::last_os_error();
                                if error.raw_os_error() != Some(libc::EBADF) {
                                    return Err(error);
                                }
                            }
                        }
                    }
                    if libc::fcntl(target, libc::F_SETFD, 0) != 0 {
                        if fd.is_some() {
                            return Err(io::Error::last_os_error());
                        }
                    }
                }
                Ok(())
            });
        }
        command.output().expect("run inherited snapshot helper")
    }

    fn materialize_txn_daily_fixture(state: &mut EngineState, path: &Path) -> Value {
        handle_line_with_state(
            state,
            &json!({
                "request_id": "materialize",
                "command": "funds.materialize_txn_daily_v1",
                "case_id": "case-a",
                "db_path": path.display().to_string(),
                "payload": {
                    "caseId": "case-a",
                    "sourceRevision": 7,
                    "sourceRowCount": 2,
                    "sourceMaxTxnTs": "2026-01-02 01:02:03",
                    "sourceMaxId": 2,
                    "rawArtifactManifestSha256": "1".repeat(64)
                }
            })
            .to_string(),
        )
    }

    fn file_sha256(path: &Path) -> String {
        format!(
            "{:x}",
            Sha256::digest(fs::read(path).expect("read transaction verifier fixture"))
        )
    }

    fn temp_case_db_path(label: &str) -> PathBuf {
        let started_at_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|duration| duration.as_millis())
            .unwrap_or(0);
        let dir = std::env::temp_dir().join(format!(
            "analytix-data-engine-{label}-{}-{started_at_ms}",
            std::process::id()
        ));
        fs::create_dir_all(&dir).expect("create temp data engine dir");
        dir.join("case.duckdb")
    }

    fn write_private_owner_fixture(path: &Path, payload: &[u8]) {
        let mut options = OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;

            options.mode(0o600);
        }
        let mut file = options.open(path).expect("create private owner fixture");
        file.write_all(payload)
            .expect("write private owner fixture");
        file.sync_all().expect("sync private owner fixture");
    }

    #[cfg(unix)]
    fn wait_for_test_path(path: &Path, timeout: Duration) -> bool {
        let started = Instant::now();
        while started.elapsed() < timeout {
            if path.exists() {
                return true;
            }
            thread::sleep(Duration::from_millis(10));
        }
        false
    }

    fn cleanup_temp_case_db(path: &Path) {
        let _ = fs::remove_file(path);
        if let Some(parent) = path.parent() {
            let _ = fs::remove_file(owner_file_path(path));
            let _ = fs::remove_file(owner_lock_file_path(path));
            let _ = fs::remove_dir(parent);
        }
    }
}
