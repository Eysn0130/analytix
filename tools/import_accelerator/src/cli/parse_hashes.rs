use super::arg_utils::{reject_extra, required_path_arg, required_string_arg};
use super::Command;
use anyhow::{bail, Result};

pub(crate) fn parse_hashes_args(iter: &mut impl Iterator<Item = String>) -> Result<Command> {
    let path = required_path_arg(iter, "--path")?;
    let algos = required_string_arg(iter, "--algos")?
        .split(',')
        .map(|item| item.trim().to_ascii_lowercase())
        .filter(|item| !item.is_empty())
        .collect::<Vec<_>>();
    if algos.is_empty() {
        bail!("--algos must include at least one algorithm");
    }
    reject_extra(iter)?;
    Ok(Command::Hashes { path, algos })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn parse(args: Vec<&'static str>) -> Result<Command> {
        let mut iter = args.into_iter().map(str::to_string);
        parse_hashes_args(&mut iter)
    }

    fn error_message(args: Vec<&'static str>) -> String {
        parse(args).unwrap_err().to_string()
    }

    #[test]
    fn parse_hashes_args_trims_lowercases_and_filters_algos() {
        let command = parse(vec![
            "--path",
            "input.csv",
            "--algos",
            " MD5, SHA256, ,md5 ",
        ])
        .unwrap();

        assert_eq!(
            command,
            Command::Hashes {
                path: PathBuf::from("input.csv"),
                algos: vec!["md5".to_string(), "sha256".to_string(), "md5".to_string()],
            }
        );
    }

    #[test]
    fn parse_hashes_args_rejects_empty_algos() {
        assert_eq!(
            error_message(vec!["--path", "input.csv", "--algos", " , , "]),
            "--algos must include at least one algorithm"
        );
    }

    #[test]
    fn parse_hashes_args_rejects_extra_arguments_after_algos() {
        assert_eq!(
            error_message(vec!["--path", "input.csv", "--algos", "md5", "--extra"]),
            "unexpected argument: --extra"
        );
    }

    #[test]
    fn parse_hashes_args_preserves_fixed_flag_order_errors() {
        assert_eq!(
            error_message(vec!["--algos", "md5", "--path", "input.csv"]),
            "expected --path, got --algos"
        );
    }
}
