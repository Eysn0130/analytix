use super::encoding::{detect_encoding, normalize_encoding_label};
use anyhow::Result;
use std::path::PathBuf;

pub(crate) fn resolve_prepare_csv_encoding(
    path: &PathBuf,
    encoding_override: Option<&str>,
) -> Result<String> {
    match encoding_override {
        Some(label) => normalize_encoding_label(label),
        None => detect_encoding(path).map(str::to_string),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_gb18030, write_text, TestDir};

    #[test]
    fn resolve_prepare_csv_encoding_prefers_override_without_reading_file() {
        let missing = PathBuf::from("missing.csv");

        assert_eq!(
            resolve_prepare_csv_encoding(&missing, Some(" UTF-8-SIG ")).unwrap(),
            "utf-8-sig"
        );
    }

    #[test]
    fn resolve_prepare_csv_encoding_rejects_unsupported_override() {
        let missing = PathBuf::from("missing.csv");
        let err = resolve_prepare_csv_encoding(&missing, Some("not-a-codec")).unwrap_err();

        assert_eq!(err.to_string(), "unsupported encoding: not-a-codec");
    }

    #[test]
    fn resolve_prepare_csv_encoding_detects_when_override_is_absent() {
        let dir = TestDir::new("prepare-encoding");
        let utf8_sig = dir.join("utf8_sig.csv");
        let gb18030 = dir.join("gb18030.csv");
        write_text(&utf8_sig, "\u{feff}交易时间,交易金额\n");
        write_gb18030(&gb18030, "交易时间,交易金额\n");

        assert_eq!(
            resolve_prepare_csv_encoding(&utf8_sig, None).unwrap(),
            "utf-8-sig"
        );
        assert_eq!(
            resolve_prepare_csv_encoding(&gb18030, None).unwrap(),
            "gb18030"
        );
    }
}
