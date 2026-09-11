use crate::support::{
    accelerator_bin, assert_failure_contains, run_failure, run_json, write_text, TestDir,
};
use serde_json::json;
use std::process::Command;

#[test]
fn binary_cli_hashes_outputs_json_payload() {
    let dir = TestDir::new("hashes");
    let path = dir.join("sample.txt");
    write_text(&path, "abc");

    let payload = run_json(
        Command::new(accelerator_bin())
            .arg("hashes")
            .arg("--path")
            .arg(&path)
            .arg("--algos")
            .arg("md5,sha256"),
    );

    assert_eq!(
        payload,
        json!({
            "ok": true,
            "hashes": {
                "md5": "900150983CD24FB0D6963F7D28E17F72",
                "sha256": "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD",
            },
        })
    );
}

#[test]
fn binary_cli_hashes_rejects_unsupported_algorithm_without_json() {
    let dir = TestDir::new("hashes-unsupported");
    let path = dir.join("sample.txt");
    write_text(&path, "abc");

    let output = run_failure(
        Command::new(accelerator_bin())
            .arg("hashes")
            .arg("--path")
            .arg(&path)
            .arg("--algos")
            .arg("md5,sha1"),
    );

    assert_failure_contains(&output, "unsupported hash algorithm: sha1");
}
