use super::arg_utils::next_arg_value;
use super::Command;
use anyhow::{anyhow, bail, Context, Result};
use std::path::PathBuf;

pub(crate) fn parse_prepare_csv_args(iter: &mut impl Iterator<Item = String>) -> Result<Command> {
    let mut path = None;
    let mut limit = None;
    let mut output = None;
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
            "--output" => output = Some(PathBuf::from(next_arg_value(iter, "--output")?)),
            "--encoding" => encoding = Some(next_arg_value(iter, "--encoding")?),
            other => bail!("unexpected argument: {other}"),
        }
    }
    Ok(Command::PrepareCsv {
        path: PathBuf::from(path.ok_or_else(|| anyhow!("missing --path"))?),
        limit: limit.ok_or_else(|| anyhow!("missing --limit"))?,
        output,
        encoding,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_prepare_csv_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_prepare_csv_args_parses_optional_output_and_encoding() {
        let command = parse(vec![
            "--path",
            "input.csv",
            "--limit",
            "5",
            "--output",
            "clean.csv",
            "--encoding",
            "gb18030",
        ])
        .unwrap();

        assert_eq!(
            command,
            Command::PrepareCsv {
                path: PathBuf::from("input.csv"),
                limit: 5,
                output: Some(PathBuf::from("clean.csv")),
                encoding: Some("gb18030".to_string()),
            }
        );
    }

    #[test]
    fn parse_prepare_csv_args_reports_missing_path() {
        assert_eq!(error_message(vec!["--limit", "5"]), "missing --path");
    }

    #[test]
    fn parse_prepare_csv_args_reports_missing_limit() {
        assert_eq!(
            error_message(vec!["--path", "input.csv"]),
            "missing --limit"
        );
    }

    #[test]
    fn parse_prepare_csv_args_rejects_unknown_argument() {
        assert_eq!(
            error_message(vec!["--path", "input.csv", "--bogus"]),
            "unexpected argument: --bogus"
        );
    }
}
