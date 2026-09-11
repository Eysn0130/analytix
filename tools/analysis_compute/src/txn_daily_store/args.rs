use anyhow::{bail, Context, Result};
use std::collections::BTreeSet;
use std::path::PathBuf;

#[derive(Debug)]
pub(crate) struct MaterializeArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) source_revision: i64,
    pub(crate) source_row_count: i64,
    pub(crate) source_max_txn_ts: String,
    pub(crate) source_max_id: i64,
}

pub(crate) fn parse_args(iter: impl Iterator<Item = String>) -> Result<MaterializeArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut source_revision = 0_i64;
    let mut source_row_count = 0_i64;
    let mut source_max_txn_ts = String::new();
    let mut source_max_id = 0_i64;
    let mut seen = BTreeSet::new();
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        if arg.starts_with("--") && !seen.insert(arg.clone()) {
            bail!("duplicate argument: {arg}");
        }
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--source-revision" => {
                source_revision = crate::required_value(&mut args, "--source-revision")?
                    .parse()
                    .context("--source-revision must be an integer")?
            }
            "--source-row-count" => {
                source_row_count = crate::required_value(&mut args, "--source-row-count")?
                    .parse()
                    .context("--source-row-count must be an integer")?
            }
            "--source-max-txn-ts" => {
                source_max_txn_ts = crate::required_value(&mut args, "--source-max-txn-ts")?
            }
            "--source-max-id" => {
                source_max_id = crate::required_value(&mut args, "--source-max-id")?
                    .parse()
                    .context("--source-max-id must be an integer")?
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    if case_id.trim().is_empty() {
        bail!("missing --case-id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing --db-path");
    }
    Ok(MaterializeArgs {
        case_id,
        db_path,
        source_revision,
        source_row_count,
        source_max_txn_ts,
        source_max_id,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn materialize_args_reject_duplicate_flags() {
        let error = parse_args(
            [
                "--case-id",
                "case-a",
                "--db-path",
                "/tmp/case.duckdb",
                "--source-revision",
                "7",
                "--source-revision",
                "8",
                "--source-row-count",
                "2",
                "--source-max-txn-ts",
                "2026-01-02 01:02:03",
                "--source-max-id",
                "2",
            ]
            .into_iter()
            .map(str::to_string),
        )
        .expect_err("duplicate flags must fail before opening a database");

        assert!(format!("{error:#}").contains("duplicate argument: --source-revision"));
    }
}
