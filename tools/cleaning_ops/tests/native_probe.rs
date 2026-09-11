use serde_json::Value;
use std::io::{BufRead, BufReader, Read, Write};
use std::process::{Child, Command, Output, Stdio};
use std::thread;
use std::time::{Duration, Instant};

const ARGUMENT: &str = "--analytix-native-probe";
const PROTOCOL: &str = "analytix-native-v1";
const NONCE: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const REQUEST_ID: &str = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

fn binary() -> Command {
    let mut command = Command::new(env!("CARGO_BIN_EXE_analytix-cleaning-ops"));
    command.env_clear();
    command
}

fn spawn_probe() -> Child {
    binary()
        .arg(ARGUMENT)
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn native probe")
}

#[test]
fn native_probe_emits_strict_readiness_and_one_health_response() {
    let mut child = spawn_probe();
    let mut reader = BufReader::new(child.stdout.take().expect("probe stdout"));
    let ready = read_json_frame(&mut reader);
    assert_exact_keys(
        &ready,
        &[
            "kind",
            "schema_version",
            "component_id",
            "launch_nonce",
            "protocol_version",
            "process_id",
        ],
    );
    assert_eq!(ready["kind"], "analytix_native_ready");
    assert_eq!(ready["schema_version"], 3);
    assert_eq!(ready["component_id"], "cleaning-ops");
    assert_eq!(ready["launch_nonce"], NONCE);
    assert_eq!(ready["protocol_version"], PROTOCOL);
    assert_eq!(ready["process_id"], child.id());

    let mut stdin = child.stdin.take().expect("probe stdin");
    writeln!(
        stdin,
        "{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}"
    )
    .expect("write ping");
    stdin.flush().expect("flush ping");
    let response = read_json_frame(&mut reader);
    assert_health_response(&response, child.id());
    drop(stdin);
    thread::sleep(Duration::from_millis(20));
    assert!(child.try_wait().expect("poll probe").is_none());
    child.kill().expect("terminate probe");
    let output = finish_child(child);
    assert!(output.stderr.is_empty(), "stderr={:?}", output.stderr);
}

#[test]
fn malformed_probe_frames_fail_closed_without_stderr() {
    for frame in [
        format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"unknown\"}}\n"),
        format!(
            "{{\"request_id\":\"{REQUEST_ID}\",\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}\n"
        ),
        format!(
            "{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\",\"extra\":true}}\n"
        ),
        format!("{{\"REQUEST_ID\":\"{REQUEST_ID}\",\"command\":\"ping\"}}\n"),
        format!("{{\"request_id\":\"{REQUEST_ID}\",\"COMMAND\":\"ping\"}}\n"),
        format!("{{\"request_id\":\"{REQUEST_ID}\",\"command\":\"ping\"}}"),
        format!("{}\n", "x".repeat(512)),
    ] {
        let mut child = spawn_probe();
        let mut stdout = BufReader::new(child.stdout.take().expect("probe stdout"));
        let _ = read_json_frame(&mut stdout);
        child
            .stdin
            .take()
            .expect("probe stdin")
            .write_all(frame.as_bytes())
            .expect("write malformed frame");
        let status = wait_bounded(&mut child);
        assert!(!status.success());
        let mut trailing = Vec::new();
        stdout.read_to_end(&mut trailing).expect("read stdout");
        assert!(trailing.is_empty(), "unexpected stdout={trailing:?}");
        let output = finish_child(child);
        assert!(output.stderr.is_empty(), "stderr={:?}", output.stderr);
    }
}

#[test]
fn native_probe_source_cannot_create_processes() {
    let source = include_str!("../src/native_probe.rs");
    for forbidden in [
        "use std::process",
        "std::process::Command",
        "Command::new",
        "::spawn(",
        ".spawn(",
        "fork(",
        "vfork(",
        "posix_spawn",
        "execve(",
        "system(",
        "unsafe",
        "libc::",
        "nix::",
    ] {
        assert!(
            !source.contains(forbidden),
            "native probe process-creation primitive is forbidden: {forbidden}"
        );
    }
    assert!(source.contains("std::process::id()"));
    assert!(source.contains("thread::park()"));
}

#[test]
fn extra_probe_arguments_are_rejected_and_help_is_unchanged() {
    let rejected = binary()
        .args([ARGUMENT, "extra"])
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .output()
        .expect("run invalid probe invocation");
    assert!(!rejected.status.success());
    assert!(rejected.stdout.is_empty());
    assert!(rejected.stderr.is_empty());

    let help = binary()
        .arg("--help")
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .output()
        .expect("run help");
    assert!(help.status.success());
    assert_eq!(
        String::from_utf8(help.stdout).expect("help UTF-8"),
        "Usage: analytix-cleaning-ops <clean-all|amount-balance|quality-flags|dc-flag|account-keys|account-info> --case-id <id> --db-path <case.duckdb> [--txn-file-id <file_id> ...] [--acc-file-id <file_id> ...]\n"
    );
    assert!(help.stderr.is_empty());
}

#[test]
fn invalid_probe_authority_is_rejected_before_readiness() {
    let uppercase_nonce = "A".repeat(64);
    for (nonce, protocol) in [
        (uppercase_nonce.as_str(), PROTOCOL),
        (NONCE, "analytix-native-v2"),
    ] {
        let output = binary()
            .arg(ARGUMENT)
            .env("ANALYTIX_NATIVE_LAUNCH_NONCE", nonce)
            .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", protocol)
            .output()
            .expect("run unauthorized probe");
        assert!(!output.status.success());
        assert!(output.stdout.is_empty());
        assert!(output.stderr.is_empty());
    }
}

fn read_json_frame(reader: &mut impl BufRead) -> Value {
    let mut frame = Vec::new();
    reader.read_until(b'\n', &mut frame).expect("read frame");
    assert_eq!(frame.pop(), Some(b'\n'));
    serde_json::from_slice(&frame).expect("strict JSON fixture")
}

fn assert_health_response(response: &Value, process_id: u32) {
    assert_exact_keys(response, &["request_id", "ok", "data", "diagnostics"]);
    assert_eq!(response["request_id"], REQUEST_ID);
    assert_eq!(response["ok"], true);
    assert_exact_keys(&response["data"], &["pong", "pid"]);
    assert_eq!(response["data"]["pong"], true);
    assert_eq!(response["data"]["pid"], process_id);
    assert_exact_keys(
        &response["diagnostics"],
        &[
            "engine",
            "command",
            "case_bound",
            "db_bound",
            "pid",
            "queue_wait_ms",
            "run_ms",
            "owner_epoch",
        ],
    );
    assert_eq!(response["diagnostics"]["engine"], "analytix-cleaning-ops");
    assert_eq!(response["diagnostics"]["command"], "ping");
    assert_eq!(response["diagnostics"]["case_bound"], false);
    assert_eq!(response["diagnostics"]["db_bound"], false);
    assert_eq!(response["diagnostics"]["pid"], process_id);
    assert_eq!(response["diagnostics"]["queue_wait_ms"], 0);
    assert_eq!(response["diagnostics"]["run_ms"], 0);
    assert_eq!(response["diagnostics"]["owner_epoch"], "");
}

fn assert_exact_keys(value: &Value, keys: &[&str]) {
    let object = value.as_object().expect("JSON object");
    assert_eq!(object.len(), keys.len());
    assert!(keys.iter().all(|key| object.contains_key(*key)));
}

fn wait_bounded(child: &mut Child) -> std::process::ExitStatus {
    let deadline = Instant::now() + Duration::from_secs(3);
    loop {
        if let Some(status) = child.try_wait().expect("poll child") {
            return status;
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            panic!("probe did not reject malformed input");
        }
        thread::sleep(Duration::from_millis(10));
    }
}

fn finish_child(mut child: Child) -> Output {
    let status = child.wait().expect("wait child");
    let mut stderr = Vec::new();
    child
        .stderr
        .take()
        .expect("probe stderr")
        .read_to_end(&mut stderr)
        .expect("read stderr");
    Output {
        status,
        stdout: Vec::new(),
        stderr,
    }
}
