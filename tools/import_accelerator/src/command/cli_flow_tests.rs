use super::handlers::handle_command;
use super::test_support::{
    assert_workbook_open_error, expected_detect_encoding_payload, expected_hashes_payload,
    expected_prepare_csv_payload, expected_profile_columns_payload, missing_xlsx, path_arg,
    sample_hash_text, sample_prepare_csv, sample_profile_csv, sample_utf8_csv, TestDir,
};
use crate::cli::parse_args_from;
use anyhow::Result;
use serde_json::Value;

fn run_cli_args(args: Vec<String>) -> Result<Value> {
    handle_command(parse_args_from(args)?)
}

#[test]
fn cli_flow_detect_encoding_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-detect-encoding");
    let path = sample_utf8_csv(&dir);

    let payload = run_cli_args(vec![
        "detect-encoding".to_string(),
        "--path".to_string(),
        path_arg(&path),
    ])
    .unwrap();

    assert_eq!(payload, expected_detect_encoding_payload());
}

#[test]
fn cli_flow_prepare_csv_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-prepare-csv");
    let path = sample_prepare_csv(&dir);

    let payload = run_cli_args(vec![
        "prepare-csv".to_string(),
        "--path".to_string(),
        path_arg(&path),
        "--limit".to_string(),
        "5".to_string(),
        "--encoding".to_string(),
        "utf-8-sig".to_string(),
    ])
    .unwrap();

    assert_eq!(payload, expected_prepare_csv_payload());
}

#[test]
fn cli_flow_prepare_csv_with_profile_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-prepare-csv-with-profile");
    let path = sample_profile_csv(&dir);

    let payload = run_cli_args(vec![
        "prepare-csv-with-profile".to_string(),
        "--path".to_string(),
        path_arg(&path),
        "--limit".to_string(),
        "1".to_string(),
        "--encoding".to_string(),
        "utf-8-sig".to_string(),
    ])
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
fn cli_flow_batch_prepare_csv_with_profile_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-batch-prepare-csv-with-profile");
    let path = sample_profile_csv(&dir);
    let manifest = dir.join("manifest.json");
    crate::csv::test_support::write_text(
        &manifest,
        &serde_json::json!({
            "items": [{"path": path, "limit": 1, "encoding": "utf-8-sig"}]
        })
        .to_string(),
    );

    let payload = run_cli_args(vec![
        "batch-prepare-csv-with-profile".to_string(),
        "--manifest".to_string(),
        path_arg(&manifest),
    ])
    .unwrap();

    assert_eq!(payload["ok"], true);
    assert_eq!(payload["items"].as_array().unwrap().len(), 1);
    assert_eq!(payload["items"][0]["rows_total"], 2);
    assert_eq!(
        payload["items"][0]["column_profiles"],
        expected_profile_columns_payload()
    );
}

#[test]
fn cli_flow_batch_prepare_csv_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-batch-prepare-csv");
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

    let payload = run_cli_args(vec![
        "batch-prepare-csv".to_string(),
        "--manifest".to_string(),
        path_arg(&manifest),
    ])
    .unwrap();

    assert_eq!(payload["ok"], true);
    assert_eq!(payload["items"].as_array().unwrap().len(), 1);
    assert_eq!(payload["items"][0]["rows_total"], 1);
}

#[test]
fn cli_flow_profile_columns_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-profile-columns");
    let path = sample_profile_csv(&dir);

    let payload = run_cli_args(vec![
        "profile-columns".to_string(),
        "--path".to_string(),
        path_arg(&path),
        "--encoding".to_string(),
        "utf-8-sig".to_string(),
    ])
    .unwrap();

    assert_eq!(payload, expected_profile_columns_payload());
}

#[test]
fn cli_flow_hashes_maps_args_to_payload() {
    let dir = TestDir::new("cli-flow-hashes");
    let path = sample_hash_text(&dir);

    let payload = run_cli_args(vec![
        "hashes".to_string(),
        "--path".to_string(),
        path_arg(&path),
        "--algos".to_string(),
        "md5,sha256".to_string(),
    ])
    .unwrap();

    assert_eq!(payload, expected_hashes_payload());
}

#[test]
fn cli_flow_excel_to_csv_maps_args_to_workbook_reader() {
    let dir = TestDir::new("cli-flow-excel-to-csv");
    let input = missing_xlsx(&dir);
    let output = dir.join("out.csv");

    let err = run_cli_args(vec![
        "excel-to-csv".to_string(),
        "--input".to_string(),
        path_arg(&input),
        "--output".to_string(),
        path_arg(&output),
    ])
    .unwrap_err();

    assert_workbook_open_error(err);
}

#[test]
fn cli_flow_split_account_sections_maps_args_to_workbook_reader() {
    let dir = TestDir::new("cli-flow-split-account-sections");
    let input = missing_xlsx(&dir);
    let output_dir = dir.join("sections");

    let err = run_cli_args(vec![
        "split-account-sections".to_string(),
        "--input".to_string(),
        path_arg(&input),
        "--output-dir".to_string(),
        path_arg(&output_dir),
    ])
    .unwrap_err();

    assert_workbook_open_error(err);
}
