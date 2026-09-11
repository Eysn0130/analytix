use crate::support::{
    accelerator_bin, assert_failure_contains, run_failure, run_json, write_text, TestDir,
};
use serde_json::json;
use std::fs;
use std::process::Command;

#[test]
fn binary_cli_prepare_csv_outputs_json_payload() {
    let dir = TestDir::new("prepare-csv");
    let path = dir.join("sample.csv");
    write_text(&path, "交易时间,交易金额\n2026-01-01,100\n");

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("prepare-csv")
            .arg("--path")
            .arg(&path)
            .arg("--limit")
            .arg("5")
            .arg("--encoding")
            .arg("utf-8-sig"),
    );

    assert_eq!(
        payload,
        json!({
            "ok": true,
            "encoding": "utf-8-sig",
            "rows_total": 1,
            "columns_total": 2,
            "header_preview": ["交易时间", "交易金额"],
            "sample_rows": [["2026-01-01", "100"]],
            "preclean_rows": null,
            "output": null,
        })
    );
}

#[test]
fn binary_cli_batch_prepare_csv_with_profile_outputs_json_payload() {
    let dir = TestDir::new("batch-prepare-csv-with-profile");
    let path = dir.join("sample.csv");
    let manifest = dir.join("manifest.json");
    write_text(
        &path,
        "交易日期,交易金额,交易账号\n20260102,-123.45,6222000000000000001\n",
    );
    write_text(
        &manifest,
        &json!({
            "items": [{"path": path, "limit": 1, "encoding": "utf-8-sig"}]
        })
        .to_string(),
    );

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("batch-prepare-csv-with-profile")
            .arg("--manifest")
            .arg(&manifest),
    );

    assert_eq!(payload["ok"], true);
    assert_eq!(payload["items"].as_array().unwrap().len(), 1);
    assert_eq!(payload["items"][0]["rows_total"], 1);
    assert_eq!(payload["items"][0]["column_profiles"]["columns_total"], 3);
    assert!(payload["items"][0]["elapsed_s"].as_f64().unwrap() >= 0.0);
}

#[test]
fn binary_cli_prepare_csv_output_writes_cleaned_file_and_metadata() {
    let dir = TestDir::new("prepare-csv-output");
    let path = dir.join("sample.csv");
    let output_path = dir.join("nested/clean.csv");
    write_text(
        &path,
        "交易时间,交易金额,摘要说明\n 2026-01-01 10:00:00\t, 100.00 , 工资入账 \n2026-01-02 11:00:00,80.50,转账\n",
    );
    fs::create_dir_all(output_path.parent().unwrap()).unwrap();
    write_text(&output_path, "stale output\n");

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("prepare-csv")
            .arg("--path")
            .arg(&path)
            .arg("--limit")
            .arg("1")
            .arg("--encoding")
            .arg("utf-8-sig")
            .arg("--output")
            .arg(&output_path),
    );

    assert_eq!(
        payload,
        json!({
            "ok": true,
            "encoding": "utf-8-sig",
            "rows_total": 2,
            "columns_total": 3,
            "header_preview": ["交易时间", "交易金额", "摘要说明"],
            "sample_rows": [["2026-01-01 10:00:00", "100.00", "工资入账"]],
            "preclean_rows": 2,
            "output": &output_path,
        })
    );
    assert_eq!(
        fs::read_to_string(&output_path).unwrap(),
        "交易时间,交易金额,摘要说明\n2026-01-01 10:00:00,100.00,工资入账\n2026-01-02 11:00:00,80.50,转账\n"
    );
}

#[test]
fn binary_cli_prepare_csv_output_rejects_malformed_csv_without_partial_output() {
    let dir = TestDir::new("prepare-csv-malformed-output");
    let path = dir.join("sample.csv");
    let output_path = dir.join("nested/clean.csv");
    write_text(&path, "交易时间,交易金额\n2026-01-01,100\n\"unterminated\n");
    fs::create_dir_all(output_path.parent().unwrap()).unwrap();
    write_text(&output_path, "previous clean output\n");

    let output = run_failure(
        Command::new(accelerator_bin())
            .arg("prepare-csv")
            .arg("--path")
            .arg(&path)
            .arg("--limit")
            .arg("1")
            .arg("--encoding")
            .arg("utf-8-sig")
            .arg("--output")
            .arg(&output_path),
    );

    assert_failure_contains(&output, "CSV error");
    assert_eq!(
        fs::read_to_string(&output_path).unwrap(),
        "previous clean output\n"
    );
}
