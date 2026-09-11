use anyhow::{bail, Result};
use std::path::PathBuf;

pub(super) fn require_case_id_and_db_path(case_id: &str, db_path: &PathBuf) -> Result<()> {
    if case_id.trim().is_empty() {
        bail!("missing --case-id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing --db-path");
    }
    Ok(())
}

pub(super) fn parse_finite_f64_arg(raw: &str, name: &str) -> Result<f64> {
    let value = raw
        .trim()
        .parse::<f64>()
        .map_err(|_| anyhow::anyhow!("{name} must be numeric"))?;
    if !value.is_finite() {
        bail!("{name} must be finite");
    }
    Ok(value)
}
