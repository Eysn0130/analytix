use super::prepare_clean::{clean_csv_to_output, clean_csv_to_output_if_needed};
use super::prepare_model::PreparedCsvPayload;
use super::prepare_preview::scan_csv_preview;
use anyhow::Result;
use std::path::PathBuf;

pub(crate) enum PrepareCsvMode<'a> {
    PreviewOnly,
    CleanToOutput { output: &'a PathBuf },
    CleanToOutputIfNeeded { output: &'a PathBuf },
}

pub(crate) fn prepare_csv_payload(
    path: &PathBuf,
    encoding: String,
    limit: usize,
    mode: PrepareCsvMode<'_>,
) -> Result<serde_json::Value> {
    match mode {
        PrepareCsvMode::PreviewOnly => preview_payload(path, encoding, limit),
        PrepareCsvMode::CleanToOutput { output } => clean_payload(path, encoding, limit, output),
        PrepareCsvMode::CleanToOutputIfNeeded { output } => {
            clean_if_needed_payload(path, encoding, limit, output)
        }
    }
}

fn preview_payload(path: &PathBuf, encoding: String, limit: usize) -> Result<serde_json::Value> {
    let preview = scan_csv_preview(path, &encoding, limit)?;
    Ok(PreparedCsvPayload {
        encoding,
        preview,
        preclean_rows: None,
        output: None,
    }
    .into_json())
}

fn clean_if_needed_payload(
    path: &PathBuf,
    encoding: String,
    limit: usize,
    output: &PathBuf,
) -> Result<serde_json::Value> {
    let cleaned = clean_csv_to_output_if_needed(path, &encoding, limit, output)?;

    Ok(PreparedCsvPayload {
        encoding,
        preview: cleaned.preview,
        preclean_rows: Some(cleaned.preclean_rows),
        output: cleaned.output,
    }
    .into_json())
}

fn clean_payload(
    path: &PathBuf,
    encoding: String,
    limit: usize,
    output: &PathBuf,
) -> Result<serde_json::Value> {
    let cleaned = clean_csv_to_output(path, &encoding, limit, output)?;

    Ok(PreparedCsvPayload {
        encoding,
        preview: cleaned.preview,
        preclean_rows: Some(cleaned.preclean_rows),
        output: Some(cleaned.output),
    }
    .into_json())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{string_array, string_rows, write_text, TestDir};
    use std::fs;

    #[test]
    fn prepare_csv_payload_preview_only_uses_scan_preview_result_shape() {
        let dir = TestDir::new("prepare-adapter-preview");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "交易时间,交易金额\n2026-01-01 10:00:00,100.00\n2026-01-02 11:00:00,80.50\n",
        );

        let payload = prepare_csv_payload(
            &csv_path,
            "utf-8-sig".to_string(),
            1,
            PrepareCsvMode::PreviewOnly,
        )
        .unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["encoding"], "utf-8-sig");
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(
            string_array(&payload, "header_preview"),
            vec!["交易时间", "交易金额"]
        );
        assert_eq!(
            string_rows(&payload, "sample_rows"),
            vec![vec!["2026-01-01 10:00:00", "100.00"]]
        );
        assert!(payload["preclean_rows"].is_null());
        assert!(payload["output"].is_null());
    }

    #[test]
    fn prepare_csv_payload_clean_to_output_preserves_output_metadata() {
        let dir = TestDir::new("prepare-adapter-clean");
        let csv_path = dir.join("sample.csv");
        let output_path = dir.join("nested/clean.csv");
        write_text(
            &csv_path,
            "交易时间,交易金额\n 2026-01-01 10:00:00\t, 100.00 \n",
        );

        let payload = prepare_csv_payload(
            &csv_path,
            "utf-8-sig".to_string(),
            5,
            PrepareCsvMode::CleanToOutput {
                output: &output_path,
            },
        )
        .unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["encoding"], "utf-8-sig");
        assert_eq!(payload["preclean_rows"], 1);
        assert_eq!(payload["output"], serde_json::json!(output_path));
        assert_eq!(
            fs::read_to_string(&output_path).unwrap(),
            "交易时间,交易金额\n2026-01-01 10:00:00,100.00\n"
        );
    }

    #[test]
    fn prepare_csv_payload_clean_if_needed_skips_output_for_clean_utf8() {
        let dir = TestDir::new("prepare-adapter-clean-if-needed");
        let csv_path = dir.join("sample.csv");
        let output_path = dir.join("clean.csv");
        write_text(&csv_path, "交易时间,交易金额\n2026-01-01,100\n");

        let payload = prepare_csv_payload(
            &csv_path,
            "utf-8".to_string(),
            5,
            PrepareCsvMode::CleanToOutputIfNeeded {
                output: &output_path,
            },
        )
        .unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["encoding"], "utf-8");
        assert_eq!(payload["rows_total"], 1);
        assert_eq!(payload["preclean_rows"], 1);
        assert!(payload["output"].is_null());
        assert!(!output_path.exists());
    }
}
