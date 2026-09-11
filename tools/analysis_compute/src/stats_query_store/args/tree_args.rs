use anyhow::{bail, Result};
use serde::Deserialize;
use std::path::PathBuf;

use super::common::require_case_id_and_db_path;

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct QueryStatsTreeArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) tab: String,
}

pub(crate) fn parse_query_stats_tree_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryStatsTreeArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut tab = String::from("byName");
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--tab" => tab = crate::required_value(&mut args, "--tab")?,
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    Ok(QueryStatsTreeArgs {
        case_id,
        db_path,
        tab,
    })
}
