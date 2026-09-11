use super::arg_utils::{reject_extra, required_path_arg};
use super::Command;
use anyhow::Result;

pub(crate) fn parse_detect_encoding_args(
    iter: &mut impl Iterator<Item = String>,
) -> Result<Command> {
    let path = required_path_arg(iter, "--path")?;
    reject_extra(iter)?;
    Ok(Command::DetectEncoding { path })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_detect_encoding_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_detect_encoding_args_parses_path() {
        assert_eq!(
            parse(vec!["--path", "input.csv"]).unwrap(),
            Command::DetectEncoding {
                path: PathBuf::from("input.csv"),
            }
        );
    }

    #[test]
    fn parse_detect_encoding_args_rejects_extra_arguments() {
        assert_eq!(
            error_message(vec!["--path", "input.csv", "--extra"]),
            "unexpected argument: --extra"
        );
    }

    #[test]
    fn parse_detect_encoding_args_preserves_fixed_flag_order_errors() {
        assert_eq!(
            error_message(vec!["--input", "input.csv"]),
            "expected --path, got --input"
        );
    }
}
