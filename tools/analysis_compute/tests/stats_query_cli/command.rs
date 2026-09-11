use anyhow::{bail, Context, Result};
use serde_json::Value;
use std::path::PathBuf;
use std::process::Command;

pub(crate) struct CommandFailure {
    pub(crate) stdout: String,
    pub(crate) stderr: String,
}

pub(crate) fn run_analysis_compute_command(args: &[&str]) -> Result<Value> {
    let output = run_command(args)?;
    if !output.status.success() {
        bail!(
            "analytix-analysis-compute failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    let mut payload: Value =
        serde_json::from_str(stdout.trim()).context("parse command stdout JSON")?;
    if let Value::Object(map) = &mut payload {
        map.remove("diagnostics");
    }
    Ok(payload)
}

pub(crate) fn run_analysis_compute_failure(args: &[&str]) -> Result<CommandFailure> {
    let output = run_command(args)?;
    if output.status.success() {
        bail!(
            "analytix-analysis-compute unexpectedly succeeded: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    Ok(CommandFailure {
        stdout: String::from_utf8(output.stdout).context("decode command stdout")?,
        stderr: String::from_utf8(output.stderr).context("decode command stderr")?,
    })
}

fn run_command(args: &[&str]) -> Result<std::process::Output> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    Command::new(binary)
        .args(args)
        .output()
        .with_context(|| format!("run analytix-analysis-compute {}", args.join(" ")))
}
