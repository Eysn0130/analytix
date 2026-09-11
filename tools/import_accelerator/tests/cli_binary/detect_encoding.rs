use crate::support::{accelerator_bin, run_json, write_text, TestDir};
use serde_json::json;
use std::process::Command;

#[test]
fn binary_cli_detect_encoding_outputs_json_payload() {
    let dir = TestDir::new("detect-encoding");
    let path = dir.join("sample.csv");
    write_text(&path, "a,b\n1,2\n");

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("detect-encoding")
            .arg("--path")
            .arg(&path),
    );

    assert_eq!(payload, json!({ "ok": true, "encoding": "utf-8" }));
}
