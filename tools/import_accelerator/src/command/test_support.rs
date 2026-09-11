use crate::csv::test_support::write_text;
use anyhow::Error;
use serde_json::{json, Value};
use std::path::{Path, PathBuf};

pub(crate) use crate::csv::test_support::TestDir;

pub(crate) fn sample_utf8_csv(dir: &TestDir) -> PathBuf {
    let path = dir.join("sample.csv");
    write_text(&path, "a,b\n1,2\n");
    path
}

pub(crate) fn sample_prepare_csv(dir: &TestDir) -> PathBuf {
    let path = dir.join("sample.csv");
    write_text(&path, "交易时间,交易金额\n2026-01-01,100\n");
    path
}

pub(crate) fn sample_profile_csv(dir: &TestDir) -> PathBuf {
    let path = dir.join("profile.csv");
    write_text(
        &path,
        "交易日期,交易金额,交易账号\n20260102,-123.45,6222000000000000001\n2026-01-03,88.00,6222000000000000002\n",
    );
    path
}

pub(crate) fn sample_hash_text(dir: &TestDir) -> PathBuf {
    let path = dir.join("sample.txt");
    write_text(&path, "abc");
    path
}

pub(crate) fn missing_xlsx(dir: &TestDir) -> PathBuf {
    dir.join("missing.xlsx")
}

pub(crate) fn path_arg(path: &Path) -> String {
    path.to_string_lossy().into_owned()
}

pub(crate) fn hash_algos() -> Vec<String> {
    vec!["md5".to_string(), "sha256".to_string()]
}

pub(crate) fn expected_detect_encoding_payload() -> Value {
    json!({ "ok": true, "encoding": "utf-8" })
}

pub(crate) fn expected_prepare_csv_payload() -> Value {
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
}

pub(crate) fn expected_profile_columns_payload() -> Value {
    json!({
        "ok": true,
        "encoding": "utf-8-sig",
        "rows_total": 2,
        "columns_total": 3,
        "columns": [
            {
                "index": 0,
                "source_index": 0,
                "header": "交易日期",
                "non_empty": 2,
                "non_empty_ratio": 1.0,
                "amount_like": 0,
                "amount_like_ratio": 0.0,
                "signed_amount": 0,
                "positive_amount": 0,
                "negative_amount": 0,
                "date_like": 2,
                "date_like_ratio": 1.0,
                "datetime_like": 0,
                "account_like": 0,
                "id_no_like": 0,
                "phone_like": 0,
                "ip_like": 0,
                "mac_like": 0,
                "distinct_count": 2,
                "distinct_limit_exceeded": false,
                "first_non_empty": ["20260102", "2026-01-03"],
                "fixed_seed_samples": ["20260102", "2026-01-03"],
                "feature_samples": {
                    "longest": "2026-01-03",
                    "min_amount": null,
                    "max_amount": null,
                    "date_like": "20260102",
                },
            },
            {
                "index": 1,
                "source_index": 1,
                "header": "交易金额",
                "non_empty": 2,
                "non_empty_ratio": 1.0,
                "amount_like": 2,
                "amount_like_ratio": 1.0,
                "signed_amount": 1,
                "positive_amount": 1,
                "negative_amount": 1,
                "date_like": 0,
                "date_like_ratio": 0.0,
                "datetime_like": 0,
                "account_like": 0,
                "id_no_like": 0,
                "phone_like": 0,
                "ip_like": 0,
                "mac_like": 0,
                "distinct_count": 2,
                "distinct_limit_exceeded": false,
                "first_non_empty": ["-123.45", "88.00"],
                "fixed_seed_samples": ["88.00", "-123.45"],
                "feature_samples": {
                    "longest": "-123.45",
                    "min_amount": "-123.45",
                    "max_amount": "88.00",
                    "date_like": null,
                },
            },
            {
                "index": 2,
                "source_index": 2,
                "header": "交易账号",
                "non_empty": 2,
                "non_empty_ratio": 1.0,
                "amount_like": 0,
                "amount_like_ratio": 0.0,
                "signed_amount": 0,
                "positive_amount": 0,
                "negative_amount": 0,
                "date_like": 0,
                "date_like_ratio": 0.0,
                "datetime_like": 0,
                "account_like": 2,
                "id_no_like": 0,
                "phone_like": 0,
                "ip_like": 0,
                "mac_like": 0,
                "distinct_count": 2,
                "distinct_limit_exceeded": false,
                "first_non_empty": ["6222000000000000001", "6222000000000000002"],
                "fixed_seed_samples": ["6222000000000000002", "6222000000000000001"],
                "feature_samples": {
                    "longest": "6222000000000000001",
                    "min_amount": null,
                    "max_amount": null,
                    "date_like": null,
                },
            },
        ],
    })
}

pub(crate) fn expected_hashes_payload() -> Value {
    json!({
        "ok": true,
        "hashes": {
            "md5": "900150983CD24FB0D6963F7D28E17F72",
            "sha256": "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD",
        },
    })
}

pub(crate) fn assert_workbook_open_error(err: Error) {
    assert!(err.to_string().starts_with("open workbook "), "{err}");
}
