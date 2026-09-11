use crate::support::{accelerator_bin, run_json, write_text, TestDir};
use std::process::Command;

#[test]
fn binary_cli_profile_columns_outputs_column_shape_payload() {
    let dir = TestDir::new("profile-columns");
    let path = dir.join("sample.csv");
    write_text(
        &path,
        "交易日期,交易金额,交易账号\n20260102,-123.45,6222000000000000001\n2026-01-03,88.00,6222000000000000002\n",
    );

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("profile-columns")
            .arg("--path")
            .arg(&path)
            .arg("--encoding")
            .arg("utf-8-sig"),
    );

    assert_eq!(payload["ok"], true);
    assert_eq!(payload["encoding"], "utf-8-sig");
    assert_eq!(payload["rows_total"], 2);
    assert_eq!(payload["columns_total"], 3);
    let columns = payload["columns"].as_array().unwrap();
    assert_eq!(columns[0]["header"], "交易日期");
    assert_eq!(columns[0]["date_like"], 2);
    assert_eq!(columns[1]["header"], "交易金额");
    assert_eq!(columns[1]["amount_like"], 2);
    assert_eq!(columns[1]["signed_amount"], 1);
    assert_eq!(columns[1]["negative_amount"], 1);
    assert_eq!(columns[2]["header"], "交易账号");
    assert_eq!(columns[2]["account_like"], 2);
}
