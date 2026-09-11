use anyhow::{anyhow, bail, Result};
use std::path::PathBuf;

pub(crate) fn required_string_arg(
    iter: &mut impl Iterator<Item = String>,
    name: &str,
) -> Result<String> {
    let flag = iter.next().ok_or_else(|| anyhow!("missing {name}"))?;
    if flag != name {
        bail!("expected {name}, got {flag}");
    }
    next_arg_value(iter, name)
}

pub(crate) fn next_arg_value(
    iter: &mut impl Iterator<Item = String>,
    name: &str,
) -> Result<String> {
    iter.next()
        .ok_or_else(|| anyhow!("{name} requires a value"))
        .map(|value| value.trim().to_string())
}

pub(crate) fn required_path_arg(
    iter: &mut impl Iterator<Item = String>,
    name: &str,
) -> Result<PathBuf> {
    Ok(PathBuf::from(required_string_arg(iter, name)?))
}

pub(crate) fn reject_extra(mut iter: impl Iterator<Item = String>) -> Result<()> {
    if let Some(arg) = iter.next() {
        bail!("unexpected argument: {arg}");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn iter(args: Vec<&'static str>) -> impl Iterator<Item = String> {
        args.into_iter().map(str::to_string)
    }

    #[test]
    fn next_arg_value_trims_values() {
        let mut args = iter(vec!["  input.csv  "]);

        assert_eq!(next_arg_value(&mut args, "--path").unwrap(), "input.csv");
    }

    #[test]
    fn next_arg_value_reports_missing_value() {
        let mut args = iter(vec![]);

        assert_eq!(
            next_arg_value(&mut args, "--path").unwrap_err().to_string(),
            "--path requires a value"
        );
    }

    #[test]
    fn required_string_arg_preserves_fixed_flag_order_errors() {
        let mut args = iter(vec!["--algos", "md5"]);

        assert_eq!(
            required_string_arg(&mut args, "--path")
                .unwrap_err()
                .to_string(),
            "expected --path, got --algos"
        );
    }

    #[test]
    fn required_path_arg_trims_path_values() {
        let mut args = iter(vec!["--path", " input.csv "]);

        assert_eq!(
            required_path_arg(&mut args, "--path").unwrap(),
            PathBuf::from("input.csv")
        );
    }

    #[test]
    fn reject_extra_reports_first_extra_argument() {
        assert_eq!(
            reject_extra(iter(vec!["--extra", "value"]))
                .unwrap_err()
                .to_string(),
            "unexpected argument: --extra"
        );
    }
}
