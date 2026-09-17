use duckdb::Connection;
use serde_json::{json, Value};
use std::fs;
#[cfg(unix)]
use std::fs::File;
use std::io::{BufRead, BufReader, Read, Seek, SeekFrom, Write};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

#[cfg(unix)]
use std::os::fd::{AsRawFd, FromRawFd};
#[cfg(unix)]
use std::os::unix::process::CommandExt;

const PROTOCOL: &str = "analytix-native-v1";
const NONCE: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const RAW_ARTIFACT_MANIFEST_SHA256: &str =
    "1111111111111111111111111111111111111111111111111111111111111111";
const PRIVATE_CANONICAL_ACCOUNT: &str = "6222021234567890123";

fn data_engine() -> Command {
    let mut command = Command::new(env!("CARGO_BIN_EXE_analytix-data-engine"));
    command.env_clear();
    command
}

#[cfg(target_os = "macos")]
fn spawn_data_engine(mut command: Command) -> (Child, File) {
    let (reader, writer) = macos_pipe();
    let source_fd = reader.as_raw_fd();
    // SAFETY: only an async-signal-safe descriptor operation runs in the child.
    // The production binary, not its caller, establishes RLIMIT_NPROC before
    // readiness or request input. reader remains alive until spawn returns.
    unsafe {
        command.pre_exec(move || {
            map_test_descriptor(source_fd, 4)?;
            Ok(())
        });
    }
    let child = command.spawn().expect("spawn data engine");
    drop(reader);
    (child, writer)
}

#[cfg(not(target_os = "macos"))]
fn spawn_data_engine(mut command: Command) -> (Child, ()) {
    (command.spawn().expect("spawn data engine"), ())
}

#[cfg(target_os = "macos")]
fn macos_pipe() -> (File, File) {
    let mut descriptors = [-1; 2];
    assert_eq!(unsafe { libc::pipe(descriptors.as_mut_ptr()) }, 0);
    for descriptor in descriptors {
        assert_eq!(
            unsafe { libc::fcntl(descriptor, libc::F_SETFD, libc::FD_CLOEXEC) },
            0
        );
    }
    // SAFETY: pipe returned two owned descriptors exactly once.
    unsafe {
        (
            File::from_raw_fd(descriptors[0]),
            File::from_raw_fd(descriptors[1]),
        )
    }
}

#[cfg(unix)]
fn map_test_descriptor(source_fd: libc::c_int, target_fd: libc::c_int) -> std::io::Result<()> {
    if source_fd == target_fd {
        if unsafe { libc::fcntl(target_fd, libc::F_SETFD, 0) } != 0 {
            return Err(std::io::Error::last_os_error());
        }
    } else if unsafe { libc::dup2(source_fd, target_fd) } < 0 {
        return Err(std::io::Error::last_os_error());
    }
    Ok(())
}

#[cfg(unix)]
fn data_engine_with_snapshot(snapshot: &File) -> Command {
    use std::os::fd::AsRawFd;
    use std::os::unix::process::CommandExt;

    const SNAPSHOT_FD: libc::c_int = 3;
    let source_fd = snapshot.as_raw_fd();
    let mut command = data_engine();
    // SAFETY: only async-signal-safe descriptor operations run between fork
    // and exec. The source File remains alive until spawn returns.
    unsafe {
        command.pre_exec(move || {
            if source_fd == SNAPSHOT_FD {
                if libc::fcntl(SNAPSHOT_FD, libc::F_SETFD, 0) != 0 {
                    return Err(std::io::Error::last_os_error());
                }
            } else if libc::dup2(source_fd, SNAPSHOT_FD) < 0 {
                return Err(std::io::Error::last_os_error());
            }
            Ok(())
        });
    }
    command
}

#[cfg(unix)]
fn spawn_canonical_data_engine(
    mut command: Command,
    source: &File,
    output: &File,
) -> (Child, Option<File>) {
    let source_duplicate = duplicate_test_descriptor(source.as_raw_fd(), 16);
    let output_duplicate = duplicate_test_descriptor(output.as_raw_fd(), 17);
    let source_fd = source_duplicate.as_raw_fd();
    let output_fd = output_duplicate.as_raw_fd();

    #[cfg(target_os = "macos")]
    let (owner_reader, owner_writer) = macos_pipe();
    #[cfg(target_os = "macos")]
    let owner_duplicate = duplicate_test_descriptor(owner_reader.as_raw_fd(), 18);
    #[cfg(target_os = "macos")]
    let owner_fd = owner_duplicate.as_raw_fd();

    // SAFETY: only async-signal-safe descriptor operations run between fork
    // and exec. Every source descriptor remains live until spawn returns.
    unsafe {
        command.pre_exec(move || {
            map_test_descriptor(source_fd, 3)?;
            map_test_descriptor(output_fd, 5)?;
            #[cfg(target_os = "macos")]
            map_test_descriptor(owner_fd, 4)?;
            Ok(())
        });
    }
    let child = command.spawn().expect("spawn canonical data engine");
    drop(source_duplicate);
    drop(output_duplicate);
    #[cfg(target_os = "macos")]
    {
        drop(owner_duplicate);
        drop(owner_reader);
        (child, Some(owner_writer))
    }
    #[cfg(not(target_os = "macos"))]
    {
        (child, None)
    }
}

#[cfg(unix)]
fn duplicate_test_descriptor(fd: libc::c_int, minimum: libc::c_int) -> File {
    let duplicate = unsafe { libc::fcntl(fd, libc::F_DUPFD_CLOEXEC, minimum) };
    assert!(duplicate >= minimum);
    // SAFETY: fcntl returned a new owned descriptor.
    unsafe { File::from_raw_fd(duplicate) }
}

#[test]
fn bootstrap_precedes_request_bytes_and_emits_identity_readiness_v3() {
    let mut command = data_engine();
    command
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let (mut child, _owner_liveness) = spawn_data_engine(command);
    let stdout = child.stdout.take().expect("data engine stdout");
    let mut reader = BufReader::new(stdout);
    let mut ready_frame = Vec::new();
    reader
        .read_until(b'\n', &mut ready_frame)
        .expect("read readiness before any request write");
    if ready_frame.is_empty() {
        let mut stderr = String::new();
        child
            .stderr
            .take()
            .expect("data engine stderr")
            .read_to_string(&mut stderr)
            .expect("read bootstrap stderr");
        let status = child.wait().expect("wait failed bootstrap");
        panic!("data engine emitted no readiness: status={status:?} stderr={stderr:?}");
    }
    let ready: Value = serde_json::from_slice(trim_newline(&ready_frame)).expect("readiness JSON");
    assert_eq!(ready["kind"], "analytix_native_ready");
    assert_eq!(ready["schema_version"], 3);
    assert_eq!(ready["component_id"], "data-engine");
    assert_eq!(ready["launch_nonce"], NONCE);
    assert_eq!(ready["protocol_version"], PROTOCOL);
    assert_eq!(ready["process_id"], child.id());
    let ready = ready.as_object().expect("readiness object");
    assert_eq!(
        ready.len(),
        6,
        "readiness must contain identity fields only"
    );
    assert!(!ready.contains_key("process_containment"));
    let request = b"{\"request_id\":\"ping\",\"command\":\"ping\"}\n";
    let mut stdin = child.stdin.take().expect("data engine stdin");
    stdin
        .write_all(request)
        .expect("write ping after readiness");
    drop(stdin);
    let mut response_frame = Vec::new();
    reader
        .read_until(b'\n', &mut response_frame)
        .expect("read ping response");
    let response: Value = serde_json::from_slice(trim_newline(&response_frame)).expect("ping JSON");
    assert_eq!(response["request_id"], "ping");
    assert_eq!(response["ok"], true);
    let mut trailing = Vec::new();
    reader
        .read_to_end(&mut trailing)
        .expect("read trailing stdout");
    assert!(trailing.is_empty(), "unexpected stdout={trailing:?}");
    let status = child.wait().expect("wait data engine");
    assert!(status.success());
}

#[cfg(unix)]
#[test]
fn canonical_csv_build_round_trips_fd3_and_fd5_through_the_real_binary() {
    use std::os::unix::fs::{MetadataExt, OpenOptionsExt, PermissionsExt};

    let source_dir = canonical_fixture_directory("canonical-source");
    let source_path = source_dir.join("3");
    fs::write(
        &source_path,
        canonical_csv_source(PRIVATE_CANONICAL_ACCOUNT, "完整敏感姓名"),
    )
    .expect("write canonical source");
    fs::set_permissions(&source_path, fs::Permissions::from_mode(0o400))
        .expect("freeze canonical source");
    let source = File::open(&source_path).expect("open canonical source");
    fs::remove_file(&source_path).expect("unlink canonical source");

    let output_dir = canonical_fixture_directory("canonical-output");
    let output_path = output_dir.join("5");
    let mut output = fs::OpenOptions::new()
        .create_new(true)
        .read(true)
        .write(true)
        .mode(0o600)
        .open(&output_path)
        .expect("create canonical output");
    fs::remove_file(&output_path).expect("unlink canonical output");
    assert_eq!(source.metadata().expect("source metadata").nlink(), 0);
    assert_eq!(output.metadata().expect("output metadata").nlink(), 0);

    let working_dir = canonical_fixture_directory("canonical-working");
    let mut command = data_engine();
    command
        .current_dir(&working_dir)
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let (mut child, _owner_liveness) = spawn_canonical_data_engine(command, &source, &output);
    let mut reader = BufReader::new(child.stdout.take().expect("canonical stdout"));
    let readiness = read_response(&mut reader);
    assert_eq!(readiness["kind"], "analytix_native_ready");

    let request = json!({
        "request_id": "canonical-build",
        "command": "funds.build_canonical_csv_snapshot_v1",
        "case_id": "case-a",
        "payload": {
            "caseId": "case-a",
            "profile": "canonical_direct_csv_v1",
            "privateImportFileId": "0123456789abcdefabcd",
            "sourceRevision": 7,
            "rawArtifactManifestSha256": RAW_ARTIFACT_MANIFEST_SHA256
        }
    });
    let mut stdin = child.stdin.take().expect("canonical stdin");
    writeln!(stdin, "{request}").expect("write canonical request");
    stdin.flush().expect("flush canonical request");
    let response = read_response(&mut reader);
    assert_eq!(response["ok"], true, "response={response}");
    assert_eq!(response["diagnostics"]["case_bound"], true);
    assert_eq!(response["diagnostics"]["db_bound"], true);
    assert_eq!(
        response["data"]["operation"],
        "funds.build_canonical_csv_snapshot_v1"
    );
    assert_eq!(response["data"]["sourceRowCount"], 1);
    assert_eq!(
        response["data"]["rawArtifactManifestSha256"],
        RAW_ARTIFACT_MANIFEST_SHA256
    );
    let encoded = serde_json::to_string(&response).expect("serialize canonical response");
    assert!(!encoded.contains(PRIVATE_CANONICAL_ACCOUNT));
    assert!(!encoded.contains("完整敏感姓名"));
    assert!(!encoded.contains("db_path"));
    assert!(!encoded.contains("/dev/fd/"));

    drop(stdin);
    let mut trailing = Vec::new();
    reader
        .read_to_end(&mut trailing)
        .expect("read canonical trailing stdout");
    assert!(
        trailing.is_empty(),
        "unexpected trailing output={trailing:?}"
    );
    let status = child.wait().expect("wait canonical data engine");
    assert!(status.success(), "status={status:?}");

    let metadata = output.metadata().expect("completed output metadata");
    assert!(metadata.len() > 0);
    assert_eq!(metadata.nlink(), 0);
    assert_eq!(metadata.mode() & 0o7777, 0o600);
    output
        .seek(SeekFrom::Start(0))
        .expect("seek completed output directly");
    let mut prefix = [0_u8; 16];
    output
        .read_exact(&mut prefix)
        .expect("read completed output directly");
    assert!(prefix.iter().any(|value| *value != 0));
    assert_eq!(fs::read_dir(&source_dir).expect("source dir").count(), 0);
    assert_eq!(fs::read_dir(&output_dir).expect("output dir").count(), 0);
    assert_eq!(fs::read_dir(&working_dir).expect("working dir").count(), 0);
    drop(source);
    drop(output);
    fs::remove_dir(source_dir).expect("remove source dir");
    fs::remove_dir(output_dir).expect("remove output dir");
    fs::remove_dir(working_dir).expect("remove working dir");
}

#[cfg(unix)]
#[test]
fn fixed_account_flow_command_round_trips_through_the_real_binary() {
    let db_path = account_flow_db_path();
    seed_account_flow_source(&db_path);
    let mut materializer_command = data_engine();
    materializer_command
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let (mut materializer, _materializer_owner_liveness) = spawn_data_engine(materializer_command);
    let mut materializer_stdin = materializer.stdin.take().expect("data engine stdin");
    let mut materializer_reader =
        BufReader::new(materializer.stdout.take().expect("data engine stdout"));
    let mut ready_frame = Vec::new();
    materializer_reader
        .read_until(b'\n', &mut ready_frame)
        .expect("read readiness");
    let ready: Value = serde_json::from_slice(trim_newline(&ready_frame)).expect("readiness JSON");
    assert_eq!(ready["kind"], "analytix_native_ready");

    let materialize = json!({
        "request_id": "materialize",
        "command": "funds.materialize_txn_daily_v1",
        "case_id": "case-a",
        "db_path": db_path.display().to_string(),
        "payload": {
            "caseId": "case-a",
            "sourceRevision": 7,
            "sourceRowCount": 2,
            "sourceMaxTxnTs": "2026-01-02 01:02:03",
            "sourceMaxId": 2,
            "rawArtifactManifestSha256": RAW_ARTIFACT_MANIFEST_SHA256
        }
    });
    writeln!(materializer_stdin, "{materialize}").expect("write materialize request");
    materializer_stdin
        .flush()
        .expect("flush materialize request");
    let materialized = read_response(&mut materializer_reader);
    assert_eq!(materialized["ok"], true, "materialized={materialized}");
    assert_eq!(
        materialized["data"]["rawArtifactManifestSha256"],
        RAW_ARTIFACT_MANIFEST_SHA256
    );
    drop(materializer_stdin);
    let mut materializer_trailing = Vec::new();
    materializer_reader
        .read_to_end(&mut materializer_trailing)
        .expect("read materializer trailing stdout");
    assert!(materializer_trailing.is_empty());
    assert!(materializer.wait().expect("wait materializer").success());

    let snapshot_parent = db_path.parent().expect("snapshot parent");
    let owner_lock = snapshot_parent.join("case.duckdb.owner.lock");
    fs::remove_file(&owner_lock).expect("remove materialization-only owner lock");
    let inherited_backing = snapshot_parent.join("3");
    fs::rename(&db_path, &inherited_backing).expect("stage fixed descriptor basename");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&inherited_backing, fs::Permissions::from_mode(0o400))
            .expect("freeze inherited snapshot mode");
    }
    let snapshot = File::open(&inherited_backing).expect("open immutable snapshot read-only");
    fs::remove_file(&inherited_backing).expect("unlink immutable snapshot");
    assert_eq!(
        fs::read_dir(snapshot_parent)
            .expect("read empty snapshot parent")
            .count(),
        0
    );

    let mut flow_command = data_engine_with_snapshot(&snapshot);
    flow_command
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let (mut child, _flow_owner_liveness) = spawn_data_engine(flow_command);
    let mut stdin = child.stdin.take().expect("data engine stdin");
    let mut reader = BufReader::new(child.stdout.take().expect("data engine stdout"));
    let mut ready_frame = Vec::new();
    reader
        .read_until(b'\n', &mut ready_frame)
        .expect("read inherited-snapshot readiness");
    let ready: Value = serde_json::from_slice(trim_newline(&ready_frame)).expect("readiness JSON");
    assert_eq!(ready["kind"], "analytix_native_ready");

    let flow = json!({
        "request_id": "account-flow",
        "command": "funds.analyze_account_flows",
        "case_id": "case-a",
        "payload": {
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
            "resolvedAccountKey": "A-1",
            "subjectResolutionDigest": "6".repeat(64),
            "startInclusive": "2026-01-01T00:00:00Z",
            "endInclusive": "2026-01-03T00:00:00Z",
            "evidenceRowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
            "scanCap": 100
        }
    });
    writeln!(stdin, "{flow}").expect("write account-flow request");
    stdin.flush().expect("flush account-flow request");
    let result = read_response(&mut reader);
    assert_eq!(result["ok"], true, "result={result}");
    assert_eq!(result["data"]["inflowMinor"], "1250");
    assert_eq!(result["data"]["outflowMinor"], "0");
    assert_eq!(result["data"]["netMinor"], "1250");
    assert_eq!(result["data"]["transactionCount"], 2);
    assert_eq!(
        result["data"]["evidenceRows"].as_array().map(Vec::len),
        Some(2)
    );
    assert_eq!(
        result["diagnostics"]["db_bound"], true,
        "only a successful inherited snapshot flow is database-bound"
    );
    let encoded = serde_json::to_string(&result).expect("serialize flow result");
    assert!(!encoded.contains("db_path"));
    assert!(!encoded.contains("/dev/fd/3"));
    assert!(!encoded.contains(inherited_backing.to_string_lossy().as_ref()));
    assert_eq!(
        fs::read_dir(snapshot_parent)
            .expect("read snapshot parent after flow")
            .count(),
        0,
        "inherited read-only flow must create no owner, lock, WAL, or other sidecar"
    );

    drop(stdin);
    let mut trailing = Vec::new();
    reader
        .read_to_end(&mut trailing)
        .expect("read trailing stdout");
    assert!(trailing.is_empty(), "unexpected stdout={trailing:?}");
    assert!(child.wait().expect("wait data engine").success());
    cleanup_account_flow_db(&db_path);
}

#[test]
fn partial_eof_request_is_rejected_without_execution() {
    let mut command = data_engine();
    command
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let (mut child, _owner_liveness) = spawn_data_engine(command);
    let mut reader = BufReader::new(child.stdout.take().expect("data engine stdout"));
    let mut ready = Vec::new();
    reader
        .read_until(b'\n', &mut ready)
        .expect("read readiness");
    let mut stdin = child.stdin.take().expect("data engine stdin");
    stdin
        .write_all(b"{\"request_id\":\"must-not-run\",\"command\":\"ping\"}")
        .expect("write partial request");
    drop(stdin);
    let mut response = Vec::new();
    reader.read_to_end(&mut response).expect("read rejection");
    let text = String::from_utf8(response).expect("UTF-8 response");
    assert!(text.contains("invalid_request_json"), "response={text}");
    assert!(!text.contains("must-not-run"), "response={text}");
    assert!(!text.contains("pong"), "response={text}");
    assert!(child.wait().expect("wait data engine").success());
}

#[test]
fn invalid_bootstrap_rejects_before_reading_case_bytes() {
    let marker = "6222020202020202020";
    for (nonce, protocol) in [("invalid", PROTOCOL), (NONCE, "invalid")] {
        // Queue input before spawning: a correct early exit must not race a
        // parent write. Keep a reader to prove rejection consumed no case bytes.
        let (mut unread_input, mut input_writer) = std::io::pipe().expect("create input pipe");
        input_writer.write_all(marker.as_bytes()).expect("queue marker");
        drop(input_writer);
        let mut command = data_engine();
        command
            .env("ANALYTIX_NATIVE_LAUNCH_NONCE", nonce)
            .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", protocol)
            .stdin(unread_input.try_clone().expect("clone input reader"))
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        let (child, _owner_liveness) = spawn_data_engine(command);
        let output = child.wait_with_output().expect("wait data engine");
        assert_eq!(output.status.code(), Some(1), "must reject, not crash");
        assert!(output.stdout.is_empty(), "invalid bootstrap emitted stdout");
        let combined = [output.stdout, output.stderr].concat();
        let text = String::from_utf8_lossy(&combined);
        assert!(text.contains("data_engine_bootstrap_invalid"));
        assert!(!text.contains(marker));
        assert!(!text.contains("analytix_native_ready"));
        let mut remaining = Vec::new();
        unread_input.read_to_end(&mut remaining).expect("read unconsumed input");
        assert_eq!(remaining, marker.as_bytes(), "bootstrap consumed case bytes");
    }
}

#[test]
fn build_probe_mode_needs_no_owner_fd_and_exposes_only_ping() {
    let mut child = data_engine()
        .arg("--analytix-native-probe")
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn data-engine build probe");
    let mut input = child.stdin.take().expect("build-probe stdin");
    let mut output = BufReader::new(child.stdout.take().expect("build-probe stdout"));
    let mut ready = Vec::new();
    output
        .read_until(b'\n', &mut ready)
        .expect("read build-probe readiness");
    assert_eq!(
        serde_json::from_slice::<Value>(trim_newline(&ready)).expect("readiness JSON")["kind"],
        "analytix_native_ready"
    );

    writeln!(
        input,
        "{}",
        json!({"request_id":"forbidden","command":"duckdb.open_session"})
    )
    .expect("write forbidden build-probe request");
    input.flush().expect("flush forbidden request");
    let forbidden = read_response(&mut output);
    assert_eq!(forbidden["ok"], false);
    assert_eq!(
        forbidden.pointer("/error/code").and_then(Value::as_str),
        Some("data_engine_request_contract_invalid")
    );

    writeln!(input, "{}", json!({"request_id":"probe","command":"ping"}))
        .expect("write build-probe ping");
    input.flush().expect("flush build-probe ping");
    let ping = read_response(&mut output);
    assert_eq!(ping["request_id"], "probe");
    assert_eq!(ping["ok"], true);
    drop(input);
    assert!(child.wait().expect("wait build probe").success());
}

#[cfg(target_os = "macos")]
#[test]
fn owner_liveness_eof_exits_without_waiting_for_stdin_or_destructors() {
    use std::os::unix::process::ExitStatusExt;

    let command = bootstrap_command();
    let (mut child, owner_liveness) = spawn_data_engine(command);
    let mut reader = BufReader::new(child.stdout.take().expect("data engine stdout"));
    let mut ready = Vec::new();
    reader
        .read_until(b'\n', &mut ready)
        .expect("read armed readiness");
    assert_eq!(
        serde_json::from_slice::<Value>(trim_newline(&ready)).expect("readiness JSON")["kind"],
        "analytix_native_ready"
    );
    assert!(
        child.stdin.is_some(),
        "stdin must remain live during owner loss"
    );
    drop(owner_liveness);
    let status = wait_for_child_exit(&mut child, Duration::from_secs(5));
    assert_eq!(status.code(), Some(190), "watchdog status={status:?}");
    assert_eq!(status.signal(), None);
    let mut trailing = Vec::new();
    reader
        .read_to_end(&mut trailing)
        .expect("read post-watchdog stdout");
    assert!(trailing.is_empty(), "unexpected stdout={trailing:?}");
}

#[cfg(target_os = "macos")]
#[test]
fn owner_liveness_payload_is_a_terminal_protocol_violation() {
    let command = bootstrap_command();
    let (mut child, mut owner_liveness) = spawn_data_engine(command);
    let mut reader = BufReader::new(child.stdout.take().expect("data engine stdout"));
    let mut ready = Vec::new();
    reader
        .read_until(b'\n', &mut ready)
        .expect("read armed readiness");
    assert_eq!(
        serde_json::from_slice::<Value>(trim_newline(&ready)).expect("readiness JSON")["kind"],
        "analytix_native_ready"
    );
    owner_liveness
        .write_all(b"x")
        .expect("write forbidden liveness payload");
    let status = wait_for_child_exit(&mut child, Duration::from_secs(5));
    assert_eq!(status.code(), Some(191), "watchdog status={status:?}");
}

#[cfg(target_os = "macos")]
#[test]
fn owner_crash_closes_the_only_writer_and_reaps_the_real_data_engine() {
    let output = Command::new(std::env::current_exe().expect("bootstrap test executable"))
        .arg("--exact")
        .arg("owner_crash_subprocess_helper")
        .arg("--nocapture")
        .env("ANALYTIX_TEST_OWNER_CRASH_HELPER", "1")
        .output()
        .expect("run owner-crash helper");
    assert_eq!(
        output.status.code(),
        Some(88),
        "helper status={:?}",
        output.status
    );
    let text = String::from_utf8(output.stdout).expect("helper pid UTF-8");
    let pid = text
        .lines()
        .find_map(|line| line.strip_prefix("owner_target_pid="))
        .and_then(|value| value.parse::<libc::pid_t>().ok())
        .expect("owner helper target pid");
    let started = Instant::now();
    loop {
        let result = unsafe { libc::kill(pid, 0) };
        if result == -1 && std::io::Error::last_os_error().raw_os_error() == Some(libc::ESRCH) {
            break;
        }
        assert!(
            started.elapsed() < Duration::from_secs(5),
            "data engine survived owner crash: pid={pid}"
        );
        std::thread::sleep(Duration::from_millis(10));
    }
}

#[cfg(target_os = "macos")]
#[test]
fn owner_crash_subprocess_helper() {
    if std::env::var("ANALYTIX_TEST_OWNER_CRASH_HELPER").as_deref() != Ok("1") {
        return;
    }
    let command = bootstrap_command();
    let (mut child, _owner_liveness) = spawn_data_engine(command);
    let mut reader = BufReader::new(child.stdout.take().expect("data engine stdout"));
    let mut ready = Vec::new();
    reader
        .read_until(b'\n', &mut ready)
        .expect("read armed readiness");
    assert_eq!(
        serde_json::from_slice::<Value>(trim_newline(&ready)).expect("readiness JSON")["kind"],
        "analytix_native_ready"
    );
    let message = format!("owner_target_pid={}\n", child.id());
    let written =
        unsafe { libc::write(libc::STDOUT_FILENO, message.as_ptr().cast(), message.len()) };
    assert_eq!(written, message.len() as isize);
    unsafe { libc::_exit(88) }
}

#[cfg(target_os = "macos")]
#[test]
fn readiness_is_never_emitted_before_the_watchdog_is_armed() {
    let (reader, writer) = macos_pipe();
    drop(writer);
    let child = spawn_with_macos_authority(bootstrap_command(), Some(&reader), None);
    let output = child
        .wait_with_output()
        .expect("wait pre-closed owner channel");
    assert_eq!(
        output.status.code(),
        Some(190),
        "status={:?}",
        output.status
    );
    assert!(!String::from_utf8_lossy(&output.stdout).contains("analytix_native_ready"));
}

#[cfg(target_os = "macos")]
#[test]
fn bootstrap_rejects_missing_or_forged_fd4_and_self_applies_nproc() {
    let missing = spawn_with_macos_authority(bootstrap_command(), None, None)
        .wait_with_output()
        .expect("wait missing fd4");
    assert_bootstrap_rejected(missing);

    let regular_path = account_flow_db_path();
    fs::write(&regular_path, b"forged liveness").expect("write forged regular fd4");
    let regular = File::open(&regular_path).expect("open forged regular fd4");
    let forged_regular = spawn_with_macos_authority(bootstrap_command(), Some(&regular), None)
        .wait_with_output()
        .expect("wait forged regular fd4");
    assert_bootstrap_rejected(forged_regular);
    drop(regular);
    cleanup_account_flow_db(&regular_path);

    let (pipe_reader, pipe_writer) = macos_pipe();
    let forged_writer = spawn_with_macos_authority(bootstrap_command(), Some(&pipe_writer), None)
        .wait_with_output()
        .expect("wait write-only fd4");
    assert_bootstrap_rejected(forged_writer);
    drop(pipe_reader);
    drop(pipe_writer);

    let fifo_path = account_flow_db_path();
    // Use CString so the named FIFO is an exact forged path-bearing pipe.
    let fifo_c =
        std::ffi::CString::new(fifo_path.as_os_str().as_encoded_bytes()).expect("fifo path");
    assert_eq!(unsafe { libc::mkfifo(fifo_c.as_ptr(), 0o600) }, 0);
    let fifo_fd = unsafe { libc::open(fifo_c.as_ptr(), libc::O_RDONLY | libc::O_NONBLOCK) };
    assert!(fifo_fd >= 0);
    let fifo = unsafe { File::from_raw_fd(fifo_fd) };
    let current = unsafe { libc::fcntl(fifo.as_raw_fd(), libc::F_GETFL) };
    assert!(current >= 0);
    assert_eq!(
        unsafe { libc::fcntl(fifo.as_raw_fd(), libc::F_SETFL, current & !libc::O_NONBLOCK) },
        0
    );
    let forged_fifo = spawn_with_macos_authority(bootstrap_command(), Some(&fifo), None)
        .wait_with_output()
        .expect("wait named fifo fd4");
    assert_bootstrap_rejected(forged_fifo);
    drop(fifo);
    fs::remove_file(&fifo_path).expect("remove named fifo");
    fs::remove_dir(fifo_path.parent().expect("fifo parent")).expect("remove fifo parent");

    let (valid_reader, _valid_writer) = macos_pipe();
    let self_hardened = spawn_with_macos_authority(bootstrap_command(), Some(&valid_reader), None)
        .wait_with_output()
        .expect("wait self-hardened nproc");
    assert!(
        self_hardened.status.success(),
        "status={:?} stderr={}",
        self_hardened.status,
        String::from_utf8_lossy(&self_hardened.stderr)
    );
    assert!(String::from_utf8_lossy(&self_hardened.stdout).contains("analytix_native_ready"));
}

#[cfg(target_os = "macos")]
#[test]
fn bootstrap_rejects_every_inherited_descriptor_outside_the_fixed_inventory() {
    let (reader, _writer) = macos_pipe();
    let extra_path = account_flow_db_path();
    fs::write(&extra_path, b"unexpected inherited fd").expect("write extra fd fixture");
    let extra = File::open(&extra_path).expect("open extra fd fixture");
    let output = spawn_with_macos_authority(bootstrap_command(), Some(&reader), Some(&extra))
        .wait_with_output()
        .expect("wait extra fd rejection");
    assert_bootstrap_rejected(output);
    cleanup_account_flow_db(&extra_path);
}

#[cfg(target_os = "macos")]
fn bootstrap_command() -> Command {
    let mut command = data_engine();
    command
        .env("ANALYTIX_NATIVE_LAUNCH_NONCE", NONCE)
        .env("ANALYTIX_NATIVE_PROTOCOL_VERSION", PROTOCOL)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    command
}

#[cfg(target_os = "macos")]
fn spawn_with_macos_authority(
    mut command: Command,
    owner_reader: Option<&File>,
    extra: Option<&File>,
) -> Child {
    let owner_fd = owner_reader.map(AsRawFd::as_raw_fd);
    let extra_fd = extra.map(AsRawFd::as_raw_fd);
    unsafe {
        command.pre_exec(move || {
            if let Some(source) = owner_fd {
                map_test_descriptor(source, 4)?;
            }
            if let Some(source) = extra_fd {
                map_test_descriptor(source, 5)?;
            }
            Ok(())
        });
    }
    command
        .spawn()
        .expect("spawn data engine authority fixture")
}

#[cfg(target_os = "macos")]
fn assert_bootstrap_rejected(output: std::process::Output) {
    assert!(!output.status.success(), "status={:?}", output.status);
    assert!(!String::from_utf8_lossy(&output.stdout).contains("analytix_native_ready"));
}

#[cfg(target_os = "macos")]
fn wait_for_child_exit(child: &mut Child, timeout: Duration) -> std::process::ExitStatus {
    let started = Instant::now();
    loop {
        if let Some(status) = child.try_wait().expect("poll data engine") {
            return status;
        }
        if started.elapsed() >= timeout {
            let _ = child.kill();
            let _ = child.wait();
            panic!("data engine did not exit within {timeout:?}");
        }
        std::thread::sleep(Duration::from_millis(10));
    }
}

#[test]
fn help_probe_requires_no_bootstrap_authority() {
    let output = data_engine().arg("--help").output().expect("run help");
    assert!(output.status.success());
    assert_eq!(
        String::from_utf8(output.stdout).expect("help UTF-8"),
        "Usage: analytix-data-engine\n"
    );
}

fn trim_newline(frame: &[u8]) -> &[u8] {
    frame.strip_suffix(b"\n").unwrap_or(frame)
}

fn read_response(reader: &mut BufReader<impl Read>) -> Value {
    let mut frame = Vec::new();
    reader
        .read_until(b'\n', &mut frame)
        .expect("read data engine response");
    serde_json::from_slice(trim_newline(&frame)).expect("data engine response JSON")
}

fn account_flow_db_path() -> PathBuf {
    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock after UNIX epoch")
        .as_nanos();
    let dir = std::env::temp_dir().join(format!(
        "analytix-data-engine-bootstrap-{}-{unique}",
        std::process::id()
    ));
    fs::create_dir_all(&dir).expect("create account-flow fixture directory");
    dir.join("case.duckdb")
}

fn seed_account_flow_source(path: &PathBuf) {
    Connection::open(path)
        .expect("open account-flow source")
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
               (1, 'case-a', TIMESTAMP '2026-01-01 01:02:03', 'A-1', 12.5, \
                '12.50', 'CP-1', '对手一', '银行甲', 'CNY', '进', '工资入账', '', \
                'file-1', 1001, 0, 0, 0), \
               (2, 'case-a', TIMESTAMP '2026-01-02 01:02:03', 'A-1', 0.0, \
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
        .expect("seed account-flow source");
}

#[cfg(unix)]
fn canonical_fixture_directory(label: &str) -> PathBuf {
    use std::os::unix::fs::PermissionsExt;

    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock after UNIX epoch")
        .as_nanos();
    let path = std::env::temp_dir().join(format!(
        "analytix-data-engine-{label}-{}-{unique}",
        std::process::id()
    ));
    fs::create_dir(&path).expect("create canonical fixture directory");
    fs::set_permissions(&path, fs::Permissions::from_mode(0o700))
        .expect("protect canonical fixture directory");
    path
}

#[cfg(unix)]
fn canonical_csv_source(private_account: &str, private_name: &str) -> Vec<u8> {
    let headers = [
        "交易卡号",
        "交易账号",
        "账户开户名称",
        "开户人证件号码",
        "交易时间",
        "交易金额",
        "交易余额",
        "收付标志",
        "交易对手账卡号",
        "现金标志",
        "对手户名",
        "对手身份证号",
        "对手开户银行",
        "摘要说明",
        "交易币种",
        "交易网点名称",
        "交易网点代码",
        "交易发生地",
        "交易是否成功",
        "传票号",
        "终端号",
        "IP地址",
        "MAC地址",
        "对手交易余额",
        "交易流水号",
        "日志号",
        "凭证种类",
        "凭证号",
        "交易柜员号",
        "商户名称",
        "商户号",
        "备注",
        "交易类型",
        "查询反馈结果原因",
    ];
    let mut row = vec![String::new(); headers.len()];
    row[1] = private_account.to_string();
    row[2] = private_name.to_string();
    row[4] = "2026-01-02 03:04:05".to_string();
    row[5] = "12.5".to_string();
    row[6] = "100.01".to_string();
    row[7] = "进".to_string();
    row[14] = "CNY".to_string();
    format!("{}\r\n{}\r\n", headers.join(","), row.join(",")).into_bytes()
}

fn cleanup_account_flow_db(path: &PathBuf) {
    let _ = fs::remove_file(path);
    let mut wal = path.as_os_str().to_os_string();
    wal.push(".wal");
    let _ = fs::remove_file(PathBuf::from(wal));
    if let Some(parent) = path.parent() {
        let file_name = path
            .file_name()
            .and_then(|value| value.to_str())
            .unwrap_or("case.duckdb");
        let _ = fs::remove_file(parent.join(format!("{file_name}.owner.json")));
        let _ = fs::remove_file(parent.join(format!("{file_name}.owner.lock")));
        let _ = fs::remove_dir(parent);
    }
}
