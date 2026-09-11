use super::arg_utils::{reject_extra, required_path_arg};
use super::Command;
use anyhow::Result;

pub(crate) fn parse_split_account_sections_args(
    iter: &mut impl Iterator<Item = String>,
) -> Result<Command> {
    let input = required_path_arg(iter, "--input")?;
    let output_dir = required_path_arg(iter, "--output-dir")?;
    reject_extra(iter)?;
    Ok(Command::SplitAccountSections { input, output_dir })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_split_account_sections_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_split_account_sections_args_parses_input_and_output_dir() {
        assert_eq!(
            parse(vec!["--input", "book.xlsx", "--output-dir", "sections"]).unwrap(),
            Command::SplitAccountSections {
                input: PathBuf::from("book.xlsx"),
                output_dir: PathBuf::from("sections"),
            }
        );
    }

    #[test]
    fn parse_split_account_sections_args_rejects_extra_arguments() {
        assert_eq!(
            error_message(vec![
                "--input",
                "book.xlsx",
                "--output-dir",
                "sections",
                "--extra"
            ]),
            "unexpected argument: --extra"
        );
    }

    #[test]
    fn parse_split_account_sections_args_preserves_fixed_flag_order_errors() {
        assert_eq!(
            error_message(vec!["--output-dir", "sections", "--input", "book.xlsx"]),
            "expected --input, got --output-dir"
        );
    }
}
