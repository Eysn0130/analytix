use super::arg_utils::{reject_extra, required_path_arg};
use super::Command;
use anyhow::Result;

pub(crate) fn parse_excel_to_csv_args(iter: &mut impl Iterator<Item = String>) -> Result<Command> {
    let input = required_path_arg(iter, "--input")?;
    let output = required_path_arg(iter, "--output")?;
    reject_extra(iter)?;
    Ok(Command::ExcelToCsv { input, output })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_excel_to_csv_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_excel_to_csv_args_parses_input_and_output() {
        assert_eq!(
            parse(vec!["--input", "book.xlsx", "--output", "out.csv"]).unwrap(),
            Command::ExcelToCsv {
                input: PathBuf::from("book.xlsx"),
                output: PathBuf::from("out.csv"),
            }
        );
    }

    #[test]
    fn parse_excel_to_csv_args_rejects_extra_arguments() {
        assert_eq!(
            error_message(vec![
                "--input",
                "book.xlsx",
                "--output",
                "out.csv",
                "--extra"
            ]),
            "unexpected argument: --extra"
        );
    }

    #[test]
    fn parse_excel_to_csv_args_preserves_fixed_flag_order_errors() {
        assert_eq!(
            error_message(vec!["--output", "out.csv", "--input", "book.xlsx"]),
            "expected --input, got --output"
        );
    }
}
