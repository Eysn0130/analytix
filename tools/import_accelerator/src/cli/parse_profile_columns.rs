use super::arg_utils::next_arg_value;
use super::Command;
use anyhow::{anyhow, bail, Result};
use std::path::PathBuf;

pub(crate) fn parse_profile_columns_args(
    iter: &mut impl Iterator<Item = String>,
) -> Result<Command> {
    let mut path = None;
    let mut encoding = None;
    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--path" => path = Some(next_arg_value(iter, "--path")?),
            "--encoding" => encoding = Some(next_arg_value(iter, "--encoding")?),
            other => bail!("unexpected argument: {other}"),
        }
    }
    Ok(Command::ProfileColumns {
        path: PathBuf::from(path.ok_or_else(|| anyhow!("missing --path"))?),
        encoding,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_profile_columns_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_profile_columns_args_parses_path_and_encoding() {
        let command = parse(vec!["--path", "input.csv", "--encoding", "gb18030"]).unwrap();

        assert_eq!(
            command,
            Command::ProfileColumns {
                path: PathBuf::from("input.csv"),
                encoding: Some("gb18030".to_string()),
            }
        );
    }

    #[test]
    fn parse_profile_columns_args_reports_missing_path() {
        assert_eq!(error_message(vec!["--encoding", "utf-8"]), "missing --path");
    }

    #[test]
    fn parse_profile_columns_args_rejects_unknown_argument() {
        assert_eq!(
            error_message(vec!["--path", "input.csv", "--bogus"]),
            "unexpected argument: --bogus"
        );
    }
}
