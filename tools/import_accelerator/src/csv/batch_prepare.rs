use super::prepare::prepare_csv_with_output_policy;
use anyhow::{anyhow, Context, Result};
use serde_json::{json, Value};
use std::fs;
use std::path::PathBuf;

pub(crate) fn batch_prepare_csv(manifest: &PathBuf) -> Result<Value> {
    let text = fs::read_to_string(manifest)
        .with_context(|| format!("read batch prepare manifest {}", manifest.display()))?;
    let payload: Value = serde_json::from_str(&text)
        .with_context(|| format!("parse batch prepare manifest {}", manifest.display()))?;
    let items = payload
        .get("items")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("batch prepare manifest requires items array"))?;

    let mut prepared_items = Vec::with_capacity(items.len());
    for (index, item) in items.iter().enumerate() {
        let path = item_path(item, "path", index)?;
        let output = match item.get("output").and_then(Value::as_str) {
            Some(value) if !value.trim().is_empty() => Some(PathBuf::from(value.trim())),
            _ => None,
        };
        let limit = match item.get("limit").and_then(Value::as_u64) {
            Some(value) => {
                usize::try_from(value).context("batch prepare item limit is too large")?
            }
            None => 0,
        };
        let encoding = item
            .get("encoding")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let clean_if_needed = item
            .get("clean_if_needed")
            .and_then(Value::as_bool)
            .unwrap_or(false);

        let prepared = prepare_csv_with_output_policy(
            &path,
            limit,
            output.as_ref(),
            encoding,
            clean_if_needed,
        )
        .with_context(|| format!("batch prepare csv item {index}: {}", path.display()))?;
        prepared_items.push(attach_batch_paths(prepared, &path)?);
    }

    Ok(json!({
        "ok": true,
        "items": prepared_items,
    }))
}

fn item_path(item: &Value, key: &str, index: usize) -> Result<PathBuf> {
    let value = item
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("batch prepare item {index} missing {key}"))?;
    Ok(PathBuf::from(value))
}

fn attach_batch_paths(mut prepared: Value, path: &PathBuf) -> Result<Value> {
    let obj = prepared
        .as_object_mut()
        .ok_or_else(|| anyhow!("prepare-csv returned non-object payload"))?;
    obj.insert("path".to_string(), json!(path));
    obj.entry("output".to_string()).or_insert(Value::Null);
    Ok(prepared)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{string_array, write_text, TestDir};

    #[test]
    fn batch_prepare_csv_processes_manifest_items_in_order() {
        let dir = TestDir::new("batch-prepare");
        let first = dir.join("first.csv");
        let second = dir.join("second.csv");
        let second_clean = dir.join("second.clean.csv");
        let manifest = dir.join("manifest.json");
        write_text(&first, "a,b\n1,2\n");
        write_text(&second, "交易时间,交易金额\n2026-01-01, 100 \n");
        write_text(
            &manifest,
            &json!({
                "items": [
                    {"path": first, "limit": 0},
                    {"path": second, "output": second_clean, "limit": 0}
                ]
            })
            .to_string(),
        );

        let payload = batch_prepare_csv(&manifest).unwrap();
        let items = payload["items"].as_array().unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(items.len(), 2);
        assert_eq!(items[0]["rows_total"], 1);
        assert_eq!(items[1]["rows_total"], 1);
        assert_eq!(
            string_array(&items[1], "header_preview"),
            vec!["交易时间", "交易金额"]
        );
        assert!(second_clean.exists());
    }

    #[test]
    fn batch_prepare_csv_can_skip_output_for_clean_utf8_items() {
        let dir = TestDir::new("batch-prepare-clean-if-needed");
        let csv_path = dir.join("clean.csv");
        let output_path = dir.join("clean.duckdb.csv");
        let manifest = dir.join("manifest.json");
        write_text(&csv_path, "交易时间,交易金额\n2026-01-01,100\n");
        write_text(
            &manifest,
            &json!({
                "items": [
                    {
                        "path": csv_path,
                        "output": output_path,
                        "limit": 0,
                        "encoding": "utf-8",
                        "clean_if_needed": true
                    }
                ]
            })
            .to_string(),
        );

        let payload = batch_prepare_csv(&manifest).unwrap();
        let items = payload["items"].as_array().unwrap();

        assert_eq!(items[0]["rows_total"], 1);
        assert!(items[0]["output"].is_null());
        assert!(!output_path.exists());
    }
}
