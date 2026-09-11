use serde_json::Value;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};
use std::time::{SystemTime, UNIX_EPOCH};

pub(crate) fn accelerator_bin() -> &'static str {
    env!("CARGO_BIN_EXE_analytix-import-accelerator")
}

pub(crate) struct TestDir {
    path: PathBuf,
}

impl TestDir {
    pub(crate) fn new(name: &str) -> Self {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let path = std::env::temp_dir().join(format!(
            "analytix-import-accelerator-bin-{name}-{}-{unique}",
            std::process::id()
        ));
        fs::create_dir_all(&path).unwrap();
        Self { path }
    }

    pub(crate) fn join(&self, name: &str) -> PathBuf {
        self.path.join(name)
    }
}

impl Drop for TestDir {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.path);
    }
}

pub(crate) fn write_text(path: &Path, text: &str) {
    fs::write(path, text.as_bytes()).unwrap();
}

pub(crate) fn run_json(command: &mut Command) -> Value {
    let output = command.output().unwrap();
    assert_success(&output);
    serde_json::from_slice(&output.stdout).unwrap()
}

pub(crate) fn run_failure(command: &mut Command) -> Output {
    let output = command.output().unwrap();
    assert!(
        !output.status.success(),
        "status: {}\nstdout: {}\nstderr: {}",
        output.status,
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    output
}

fn assert_success(output: &Output) {
    assert!(
        output.status.success(),
        "status: {}\nstdout: {}\nstderr: {}",
        output.status,
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
}

pub(crate) fn assert_failure_contains(output: &Output, expected: &str) {
    assert!(
        output.stdout.is_empty(),
        "failure should not emit success JSON stdout: {}",
        String::from_utf8_lossy(&output.stdout)
    );
    assert!(
        String::from_utf8_lossy(&output.stderr).contains(expected),
        "stderr should contain {expected:?}, got: {}",
        String::from_utf8_lossy(&output.stderr)
    );
}
