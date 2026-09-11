use super::payloads::{
    detect_encoding_payload, excel_to_csv_payload, hashes_payload, split_account_sections_payload,
};
use crate::cli::Command;
use crate::csv::{
    batch_prepare_csv, batch_prepare_csv_with_profile, detect_encoding, prepare_csv,
    prepare_csv_with_profile, profile_csv_columns,
};
use crate::excel::excel_to_csv;
use crate::hash::file_hashes;
use crate::section::split_account_sections;
use anyhow::Result;
use serde_json::Value;

pub(crate) fn handle_command(command: Command) -> Result<Value> {
    match command {
        Command::DetectEncoding { path } => {
            let encoding = detect_encoding(&path)?;
            Ok(detect_encoding_payload(&encoding))
        }
        Command::PrepareCsv {
            path,
            limit,
            output,
            encoding,
        } => prepare_csv(&path, limit, output.as_ref(), encoding.as_deref()),
        Command::PrepareCsvWithProfile {
            path,
            limit,
            encoding,
        } => {
            let encoding = match encoding {
                Some(value) => value,
                None => detect_encoding(&path)?.to_string(),
            };
            prepare_csv_with_profile(&path, encoding, limit)
        }
        Command::BatchPrepareCsvWithProfile { manifest } => {
            batch_prepare_csv_with_profile(&manifest)
        }
        Command::BatchPrepareCsv { manifest } => batch_prepare_csv(&manifest),
        Command::ProfileColumns { path, encoding } => {
            let encoding = match encoding {
                Some(value) => value,
                None => detect_encoding(&path)?.to_string(),
            };
            profile_csv_columns(&path, &encoding)
        }
        Command::Hashes { path, algos } => {
            let hashes = file_hashes(&path, &algos)?;
            Ok(hashes_payload(hashes))
        }
        Command::ExcelToCsv { input, output } => {
            let rows = excel_to_csv(&input, &output)?;
            Ok(excel_to_csv_payload(rows, &output))
        }
        Command::SplitAccountSections { input, output_dir } => {
            let items = split_account_sections(&input, &output_dir)?;
            Ok(split_account_sections_payload(items))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::test_support::{
        assert_workbook_open_error, expected_detect_encoding_payload, expected_hashes_payload,
        expected_prepare_csv_payload, expected_profile_columns_payload, hash_algos, missing_xlsx,
        sample_hash_text, sample_prepare_csv, sample_profile_csv, sample_utf8_csv, TestDir,
    };
    use super::*;

    #[test]
    fn handle_command_maps_detect_encoding_to_payload() {
        let dir = TestDir::new("handler-detect-encoding");
        let path = sample_utf8_csv(&dir);

        let payload = handle_command(Command::DetectEncoding { path }).unwrap();

        assert_eq!(payload, expected_detect_encoding_payload());
    }

    #[test]
    fn handle_command_maps_prepare_csv_to_payload() {
        let dir = TestDir::new("handler-prepare-csv");
        let path = sample_prepare_csv(&dir);

        let payload = handle_command(Command::PrepareCsv {
            path,
            limit: 5,
            output: None,
            encoding: Some("utf-8-sig".to_string()),
        })
        .unwrap();

        assert_eq!(payload, expected_prepare_csv_payload());
    }

    #[test]
    fn handle_command_maps_prepare_csv_with_profile_to_payload() {
        let dir = TestDir::new("handler-prepare-csv-with-profile");
        let path = sample_profile_csv(&dir);

        let payload = handle_command(Command::PrepareCsvWithProfile {
            path,
            limit: 1,
            encoding: Some("utf-8-sig".to_string()),
        })
        .unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(payload["sample_rows"].as_array().unwrap().len(), 1);
        assert_eq!(
            payload["column_profiles"],
            expected_profile_columns_payload()
        );
    }

    #[test]
    fn handle_command_maps_batch_prepare_csv_to_payload() {
        let dir = TestDir::new("handler-batch-prepare-csv");
        let path = sample_prepare_csv(&dir);
        let output = dir.join("clean.csv");
        let manifest = dir.join("manifest.json");
        crate::csv::test_support::write_text(
            &manifest,
            &serde_json::json!({
                "items": [{"path": path, "output": output, "limit": 5, "encoding": "utf-8-sig"}]
            })
            .to_string(),
        );

        let payload = handle_command(Command::BatchPrepareCsv { manifest }).unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["items"].as_array().unwrap().len(), 1);
        assert_eq!(payload["items"][0]["rows_total"], 1);
    }

    #[test]
    fn handle_command_maps_batch_prepare_csv_with_profile_to_payload() {
        let dir = TestDir::new("handler-batch-prepare-csv-with-profile");
        let path = sample_profile_csv(&dir);
        let manifest = dir.join("manifest.json");
        crate::csv::test_support::write_text(
            &manifest,
            &serde_json::json!({
                "items": [{"path": path, "limit": 1, "encoding": "utf-8-sig"}]
            })
            .to_string(),
        );

        let payload = handle_command(Command::BatchPrepareCsvWithProfile { manifest }).unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["items"].as_array().unwrap().len(), 1);
        assert_eq!(payload["items"][0]["rows_total"], 2);
        assert_eq!(
            payload["items"][0]["column_profiles"],
            expected_profile_columns_payload()
        );
    }

    #[test]
    fn handle_command_maps_profile_columns_to_payload() {
        let dir = TestDir::new("handler-profile-columns");
        let path = sample_profile_csv(&dir);

        let payload = handle_command(Command::ProfileColumns {
            path,
            encoding: Some("utf-8-sig".to_string()),
        })
        .unwrap();

        assert_eq!(payload, expected_profile_columns_payload());
    }

    #[test]
    fn handle_command_maps_hashes_to_payload() {
        let dir = TestDir::new("handler-hashes");
        let path = sample_hash_text(&dir);

        let payload = handle_command(Command::Hashes {
            path,
            algos: hash_algos(),
        })
        .unwrap();

        assert_eq!(payload, expected_hashes_payload());
    }

    #[test]
    fn handle_command_maps_excel_to_csv_to_workbook_reader() {
        let dir = TestDir::new("handler-excel-to-csv");
        let input = missing_xlsx(&dir);
        let output = dir.join("out.csv");

        let err = handle_command(Command::ExcelToCsv { input, output }).unwrap_err();

        assert_workbook_open_error(err);
    }

    #[test]
    fn handle_command_maps_split_account_sections_to_workbook_reader() {
        let dir = TestDir::new("handler-split-account-sections");
        let input = missing_xlsx(&dir);
        let output_dir = dir.join("sections");

        let err = handle_command(Command::SplitAccountSections { input, output_dir }).unwrap_err();

        assert_workbook_open_error(err);
    }
}
