use analytix_cleaning_ops::export_realtime::{
    ExportRealtimeProjection, ExportRealtimeTerminalAction, ExportState, ExportStateStatus,
};
use analytix_cleaning_ops::step_impact::CleaningStepSummaryImpact;
use serde_json::Value;
use std::fs;
use std::path::PathBuf;

pub fn read_contract_fixture() -> Value {
    let path = fixture_path();
    let source = fs::read_to_string(&path)
        .unwrap_or_else(|error| panic!("read fixture {}: {error}", path.display()));
    serde_json::from_str(&source)
        .unwrap_or_else(|error| panic!("parse fixture {}: {error}", path.display()))
}

pub fn step_impact_summaries(section: &Value) -> Vec<CleaningStepSummaryImpact> {
    array_at(section, &["summaries"])
        .iter()
        .map(|item| CleaningStepSummaryImpact {
            step: int_at(item, &["step"]),
            affected_rows: int_at(item, &["affected_rows"]),
        })
        .collect()
}

pub fn export_realtime_projection_from_event(event: &Value) -> ExportRealtimeProjection {
    let payload = at(event, &["payload"]);
    ExportRealtimeProjection {
        event_name: text_at(event, &["event"]).to_string(),
        job_id: text_at(event, &["job_id"]).trim().to_string(),
        payload_message: optional_text_at(payload, &["message"]),
        payload_code: optional_text_at(payload, &["code"]),
        payload_output_path: first_non_empty(
            &optional_text_at(payload, &["output_path"]),
            &optional_text_at(payload, &["path"]),
        ),
        progress: optional_int_at(payload, &["progress"]),
    }
}

pub fn export_realtime_projection_at(value: &Value, path: &[&str]) -> ExportRealtimeProjection {
    let projection = at(value, path);
    ExportRealtimeProjection {
        event_name: text_at(projection, &["eventName"]).to_string(),
        job_id: text_at(projection, &["jobId"]).to_string(),
        payload_message: text_at(projection, &["payloadMessage"]).to_string(),
        payload_code: text_at(projection, &["payloadCode"]).to_string(),
        payload_output_path: text_at(projection, &["payloadOutputPath"]).to_string(),
        progress: optional_int_at(projection, &["progress"]),
    }
}

pub fn export_state_at(value: &Value, path: &[&str]) -> ExportState {
    let state = at(value, path);
    ExportState {
        job_id: text_at(state, &["jobId"]).to_string(),
        kind: text_at(state, &["kind"]).to_string(),
        status: export_status_at(state, &["status"]),
        progress: int_at(state, &["progress"]),
        output_path: text_at(state, &["outputPath"]).to_string(),
        error: text_at(state, &["error"]).to_string(),
        message: text_at(state, &["message"]).to_string(),
    }
}

pub fn export_terminal_action_at(value: &Value, path: &[&str]) -> ExportRealtimeTerminalAction {
    let action = at(value, path);
    match text_at(action, &["type"]) {
        "notify" => ExportRealtimeTerminalAction::Notify {
            status: export_status_at(action, &["status"]),
            output_path: text_at(action, &["outputPath"]).to_string(),
            error_text: text_at(action, &["errorText"]).to_string(),
        },
        "sync" => ExportRealtimeTerminalAction::Sync,
        other => panic!("unknown export terminal action type: {other}"),
    }
}

fn fixture_path() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/cleaning/model-contract.json")
}

pub fn at<'a>(value: &'a Value, path: &[&str]) -> &'a Value {
    path.iter().fold(value, |current, key| {
        current
            .get(*key)
            .unwrap_or_else(|| panic!("missing fixture field: {}", path.join(".")))
    })
}

pub fn array_at<'a>(value: &'a Value, path: &[&str]) -> &'a [Value] {
    at(value, path)
        .as_array()
        .unwrap_or_else(|| panic!("fixture field is not an array: {}", path.join(".")))
}

pub fn bool_at(value: &Value, path: &[&str]) -> bool {
    at(value, path)
        .as_bool()
        .unwrap_or_else(|| panic!("fixture field is not a boolean: {}", path.join(".")))
}

pub fn int_at(value: &Value, path: &[&str]) -> i64 {
    at(value, path)
        .as_i64()
        .unwrap_or_else(|| panic!("fixture field is not an integer: {}", path.join(".")))
}

pub fn text_at<'a>(value: &'a Value, path: &[&str]) -> &'a str {
    at(value, path)
        .as_str()
        .unwrap_or_else(|| panic!("fixture field is not a string: {}", path.join(".")))
}

fn export_status_at(value: &Value, path: &[&str]) -> ExportStateStatus {
    match text_at(value, path) {
        "idle" => ExportStateStatus::Idle,
        "running" => ExportStateStatus::Running,
        "done" => ExportStateStatus::Done,
        "failed" => ExportStateStatus::Failed,
        "canceled" => ExportStateStatus::Canceled,
        other => panic!("unknown export state status: {other}"),
    }
}

fn optional_text_at(value: &Value, path: &[&str]) -> String {
    value
        .pointer(&json_pointer(path))
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string()
}

fn optional_int_at(value: &Value, path: &[&str]) -> Option<i64> {
    value.pointer(&json_pointer(path)).and_then(Value::as_i64)
}

fn first_non_empty(primary: &str, secondary: &str) -> String {
    if !primary.is_empty() {
        primary.to_string()
    } else {
        secondary.to_string()
    }
}

fn json_pointer(path: &[&str]) -> String {
    let mut pointer = String::new();
    for segment in path {
        pointer.push('/');
        pointer.push_str(&segment.replace('~', "~0").replace('/', "~1"));
    }
    pointer
}
