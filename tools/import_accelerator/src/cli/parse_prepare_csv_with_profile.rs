use super::arg_utils::next_arg_value;
use super::Command;
use anyhow::{anyhow, bail, Context, Result};
use std::path::PathBuf;

pub(crate) fn parse_prepare_csv_with_profile_args(
    iter: &mut impl Iterator<Item = String>,
) -> Result<Command> {
    let mut path = None;
    let mut limit = None;
    let mut encoding = None;
    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--path" => path = Some(next_arg_value(iter, "--path")?),
            "--limit" => {
                limit = Some(
                    next_arg_value(iter, "--limit")?
                        .parse::<usize>()
                        .context("--limit must be a non-negative integer")?,
                )
            }
            "--encoding" => encoding = Some(next_arg_value(iter, "--encoding")?),
            other => bail!("unexpected argument: {other}"),
        }
    }
    Ok(Command::PrepareCsvWithProfile {
        path: PathBuf::from(path.ok_or_else(|| anyhow!("missing --path"))?),
        limit: limit.ok_or_else(|| anyhow!("missing --limit"))?,
        encoding,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_prepare_csv_with_profile_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_prepare_csv_with_profile_args_parses_path_limit_and_encoding() {
        let command = parse(vec![
            "--path",
            "input.csv",
            "--limit",
            "5",
            "--encoding",
            "gb18030",
        ])
        .unwrap();

        assert_eq!(
            command,
            Command::PrepareCsvWithProfile {
                path: PathBuf::from("input.csv"),
                limit: 5,
                encoding: Some("gb18030".to_string()),
            }
        );
    }

    #[test]
    fn parse_prepare_csv_with_profile_args_reports_missing_path() {
        assert_eq!(error_message(vec!["--limit", "5"]), "missing --path");
    }

    #[test]
    fn parse_prepare_csv_with_profile_args_rejects_output_argument() {
        assert_eq!(
            error_message(vec![
                "--path",
                "input.csv",
                "--limit",
                "5",
                "--output",
                "clean.csv"
            ]),
            "unexpected argument: --output"
        );
    }
}
