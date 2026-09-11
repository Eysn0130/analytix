use super::arg_utils::{reject_extra, required_path_arg};
use super::Command;
use anyhow::Result;

pub(crate) fn parse_batch_prepare_csv_with_profile_args(
    iter: &mut impl Iterator<Item = String>,
) -> Result<Command> {
    let manifest = required_path_arg(iter, "--manifest")?;
    reject_extra(iter)?;
    Ok(Command::BatchPrepareCsvWithProfile { manifest })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_batch_prepare_csv_with_profile_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_batch_prepare_csv_with_profile_args_parses_manifest() {
        assert_eq!(
            parse(vec!["--manifest", "manifest.json"]).unwrap(),
            Command::BatchPrepareCsvWithProfile {
                manifest: PathBuf::from("manifest.json")
            }
        );
    }

    #[test]
    fn parse_batch_prepare_csv_with_profile_args_reports_missing_manifest() {
        assert_eq!(error_message(vec![]), "missing --manifest");
    }

    #[test]
    fn parse_batch_prepare_csv_with_profile_args_rejects_extra_arguments() {
        assert_eq!(
            error_message(vec!["--manifest", "manifest.json", "--extra"]),
            "unexpected argument: --extra"
        );
    }
}
