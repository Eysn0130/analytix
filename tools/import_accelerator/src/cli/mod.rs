mod arg_utils;
mod command;
mod parse_batch_prepare_csv;
mod parse_batch_prepare_csv_with_profile;
mod parse_detect_encoding;
mod parse_excel_to_csv;
mod parse_hashes;
mod parse_prepare_csv;
mod parse_prepare_csv_with_profile;
mod parse_profile_columns;
mod parse_split_account_sections;

pub(crate) use command::Command;

use anyhow::{anyhow, bail, Result};
use parse_batch_prepare_csv::parse_batch_prepare_csv_args;
use parse_batch_prepare_csv_with_profile::parse_batch_prepare_csv_with_profile_args;
use parse_detect_encoding::parse_detect_encoding_args;
use parse_excel_to_csv::parse_excel_to_csv_args;
use parse_hashes::parse_hashes_args;
use parse_prepare_csv::parse_prepare_csv_args;
use parse_prepare_csv_with_profile::parse_prepare_csv_with_profile_args;
use parse_profile_columns::parse_profile_columns_args;
use parse_split_account_sections::parse_split_account_sections_args;
use std::env;

pub(crate) fn parse_args() -> Result<Command> {
    parse_args_from(env::args().skip(1))
}

pub(crate) fn parse_args_from<I, S>(args: I) -> Result<Command>
where
    I: IntoIterator<Item = S>,
    S: Into<String>,
{
    let mut iter = args.into_iter().map(Into::into);
    let command = iter.next().ok_or_else(|| anyhow!("missing command"))?;
    match command.as_str() {
        "detect-encoding" => parse_detect_encoding_args(&mut iter),
        "prepare-csv" => parse_prepare_csv_args(&mut iter),
        "prepare-csv-with-profile" => parse_prepare_csv_with_profile_args(&mut iter),
        "batch-prepare-csv-with-profile" => parse_batch_prepare_csv_with_profile_args(&mut iter),
        "batch-prepare-csv" => parse_batch_prepare_csv_args(&mut iter),
        "profile-columns" => parse_profile_columns_args(&mut iter),
        "hashes" => parse_hashes_args(&mut iter),
        "excel-to-csv" => parse_excel_to_csv_args(&mut iter),
        "split-account-sections" => parse_split_account_sections_args(&mut iter),
        "--help" | "-h" => {
            println!(
                "Usage: analytix-import-accelerator <detect-encoding|prepare-csv|prepare-csv-with-profile|batch-prepare-csv-with-profile|batch-prepare-csv|profile-columns|hashes|excel-to-csv|split-account-sections> ..."
            );
            std::process::exit(0);
        }
        other => bail!("unsupported command: {other}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn error_message(args: Vec<&'static str>) -> String {
        parse_args_from(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_args_from_reports_missing_command() {
        assert_eq!(error_message(vec![]), "missing command");
    }

    #[test]
    fn parse_args_from_reports_unknown_command() {
        assert_eq!(
            error_message(vec!["unknown-command"]),
            "unsupported command: unknown-command"
        );
    }
}
