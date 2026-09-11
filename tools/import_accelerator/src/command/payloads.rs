use serde_json::json;
use serde_json::{Map, Value};
use std::path::PathBuf;

pub(crate) fn detect_encoding_payload(encoding: &str) -> Value {
    json!({ "ok": true, "encoding": encoding })
}

pub(crate) fn hashes_payload(hashes: Map<String, Value>) -> Value {
    json!({ "ok": true, "hashes": hashes })
}

pub(crate) fn excel_to_csv_payload(rows: u64, output: &PathBuf) -> Value {
    json!({ "ok": true, "rows": rows, "output": output })
}

pub(crate) fn split_account_sections_payload(items: Vec<Value>) -> Value {
    json!({ "ok": true, "items": items })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detect_encoding_payload_shape_is_stable() {
        assert_eq!(
            detect_encoding_payload("gb18030"),
            json!({ "ok": true, "encoding": "gb18030" })
        );
    }

    #[test]
    fn hashes_payload_shape_is_stable() {
        let mut hashes = Map::new();
        hashes.insert("md5".to_string(), json!("ABC123"));
        hashes.insert("sha256".to_string(), json!("DEF456"));

        assert_eq!(
            hashes_payload(hashes),
            json!({
                "ok": true,
                "hashes": {
                    "md5": "ABC123",
                    "sha256": "DEF456",
                },
            })
        );
    }

    #[test]
    fn excel_to_csv_payload_shape_is_stable() {
        assert_eq!(
            excel_to_csv_payload(3, &PathBuf::from("out.csv")),
            json!({ "ok": true, "rows": 3, "output": "out.csv" })
        );
    }

    #[test]
    fn split_account_sections_payload_shape_is_stable() {
        let items = vec![
            json!({ "path": "account.csv", "kind": "fc_account" }),
            json!({ "path": "sub_account.csv", "kind": "fc_sub_account" }),
        ];

        assert_eq!(
            split_account_sections_payload(items),
            json!({
                "ok": true,
                "items": [
                    { "path": "account.csv", "kind": "fc_account" },
                    { "path": "sub_account.csv", "kind": "fc_sub_account" },
                ],
            })
        );
    }
}
