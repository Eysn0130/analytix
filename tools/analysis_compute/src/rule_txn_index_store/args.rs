use anyhow::{bail, Result};
use std::collections::HashSet;
use std::path::PathBuf;

#[derive(Debug)]
pub(crate) struct RuleTxnIndexArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) force: bool,
}

pub(crate) fn parse_args(iter: impl Iterator<Item = String>) -> Result<RuleTxnIndexArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut force = false;
    let mut args = iter.peekable();
    let mut seen = HashSet::new();
    while let Some(arg) = args.next() {
        if !seen.insert(arg.clone()) {
            bail!("duplicate argument: {arg}");
        }
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--force" => force = parse_bool_strict(&crate::required_value(&mut args, "--force")?)?,
            other => bail!("unknown argument: {other}"),
        }
    }
    if case_id.trim().is_empty() {
        bail!("missing --case-id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing --db-path");
    }
    Ok(RuleTxnIndexArgs {
        case_id,
        db_path,
        force,
    })
}

fn parse_bool_strict(value: &str) -> Result<bool> {
    match value.trim().to_ascii_lowercase().as_str() {
        "1" | "true" | "yes" | "on" => Ok(true),
        "0" | "false" | "no" | "off" => Ok(false),
        _ => bail!("--force must be a boolean"),
    }
}

#[cfg(test)]
mod tests {
    use super::parse_args;

    fn base_args() -> Vec<String> {
        [
            "--case-id",
            "case-a",
            "--db-path",
            "/tmp/case-a.duckdb",
            "--force",
            "false",
        ]
        .into_iter()
        .map(str::to_string)
        .collect()
    }

    #[test]
    fn rejects_duplicate_flags_instead_of_last_value_winning() {
        let mut args = base_args();
        args.extend(["--case-id".to_string(), "case-b".to_string()]);

        let error = parse_args(args.into_iter()).expect_err("duplicate case flag must fail");

        assert!(format!("{error:#}").contains("duplicate argument: --case-id"));
    }

    #[test]
    fn rejects_unknown_force_value_instead_of_coercing_false() {
        let mut args = base_args();
        let force_value = args
            .iter()
            .position(|value| value == "--force")
            .expect("force flag")
            + 1;
        args[force_value] = "sometimes".to_string();

        let error = parse_args(args.into_iter()).expect_err("invalid force must fail");

        assert!(format!("{error:#}").contains("--force must be a boolean"));
    }
}
