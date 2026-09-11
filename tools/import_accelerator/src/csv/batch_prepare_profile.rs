use super::detect_encoding;
use super::prepare_profile::prepare_csv_with_profile;
use anyhow::{anyhow, Context, Result};
use serde_json::{json, Value};
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Mutex;
use std::time::Instant;

#[derive(Clone)]
struct ProfileBatchItem {
    path: PathBuf,
    limit: usize,
    encoding: Option<String>,
}

pub(crate) fn batch_prepare_csv_with_profile(manifest: &PathBuf) -> Result<Value> {
    let text = fs::read_to_string(manifest)
        .with_context(|| format!("read batch profile manifest {}", manifest.display()))?;
    let payload: Value = serde_json::from_str(&text)
        .with_context(|| format!("parse batch profile manifest {}", manifest.display()))?;
    let items = payload
        .get("items")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("batch profile manifest requires items array"))?;
    let batch_items = items
        .iter()
        .enumerate()
        .map(|(index, item)| parse_item(item, index))
        .collect::<Result<Vec<_>>>()?;
    if batch_items.is_empty() {
        return Ok(json!({ "ok": true, "items": [] }));
    }

    let worker_count = worker_count(batch_items.len());
    let next_index = AtomicUsize::new(0);
    let results: Vec<Mutex<Option<Result<Value, String>>>> =
        (0..batch_items.len()).map(|_| Mutex::new(None)).collect();

    std::thread::scope(|scope| {
        for _ in 0..worker_count {
            let batch_items = &batch_items;
            let results = &results;
            let next_index = &next_index;
            scope.spawn(move || loop {
                let index = next_index.fetch_add(1, Ordering::Relaxed);
                if index >= batch_items.len() {
                    break;
                }
                let result = prepare_one(&batch_items[index]).map_err(|err| format!("{err:#}"));
                let mut slot = results[index].lock().expect("batch result lock poisoned");
                *slot = Some(result);
            });
        }
    });

    let mut prepared_items = Vec::with_capacity(batch_items.len());
    for (index, result) in results.into_iter().enumerate() {
        let item = result
            .into_inner()
            .map_err(|_| anyhow!("batch profile result lock poisoned"))?
            .ok_or_else(|| anyhow!("batch profile item {index} did not run"))?;
        match item {
            Ok(payload) => prepared_items.push(payload),
            Err(message) => return Err(anyhow!("batch profile csv item {index}: {message}")),
        }
    }

    Ok(json!({
        "ok": true,
        "items": prepared_items,
    }))
}

fn parse_item(item: &Value, index: usize) -> Result<ProfileBatchItem> {
    let path = item_path(item, "path", index)?;
    let limit = match item.get("limit").and_then(Value::as_u64) {
        Some(value) => usize::try_from(value).context("batch profile item limit is too large")?,
        None => 0,
    };
    let encoding = item
        .get("encoding")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string);
    Ok(ProfileBatchItem {
        path,
        limit,
        encoding,
    })
}

fn prepare_one(item: &ProfileBatchItem) -> Result<Value> {
    let encoding = match item.encoding.as_deref() {
        Some(value) => value.to_string(),
        None => detect_encoding(&item.path)?.to_string(),
    };
    let started = Instant::now();
    let mut prepared = prepare_csv_with_profile(&item.path, encoding, item.limit)
        .with_context(|| item.path.display().to_string())?;
    let obj = prepared
        .as_object_mut()
        .ok_or_else(|| anyhow!("prepare-csv-with-profile returned non-object payload"))?;
    obj.insert("path".to_string(), json!(item.path));
    obj.insert(
        "elapsed_s".to_string(),
        json!(started.elapsed().as_secs_f64()),
    );
    Ok(prepared)
}

fn item_path(item: &Value, key: &str, index: usize) -> Result<PathBuf> {
    let value = item
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("batch profile item {index} missing {key}"))?;
    Ok(PathBuf::from(value))
}

fn worker_count(item_count: usize) -> usize {
    let parallelism = std::thread::available_parallelism()
        .map(|value| value.get())
        .unwrap_or(1);
    item_count.min(parallelism.max(1)).min(8)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::profile_columns::profile_csv_columns;
    use crate::csv::test_support::{write_text, TestDir};

    #[test]
    fn batch_prepare_csv_with_profile_processes_items_in_order() {
        let dir = TestDir::new("batch-prepare-profile");
        let first = dir.join("first.csv");
        let second = dir.join("second.csv");
        let manifest = dir.join("manifest.json");
        write_text(&first, "交易日期,交易金额\n20260102,-123.45\n");
        write_text(&second, "交易账号,交易金额\n6222000000000000001,88.00\n");
        write_text(
            &manifest,
            &json!({
                "items": [
                    {"path": first, "limit": 1, "encoding": "utf-8-sig"},
                    {"path": second, "limit": 1, "encoding": "utf-8-sig"}
                ]
            })
            .to_string(),
        );

        let payload = batch_prepare_csv_with_profile(&manifest).unwrap();
        let items = payload["items"].as_array().unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(items.len(), 2);
        assert_eq!(items[0]["path"], json!(first));
        assert_eq!(items[0]["rows_total"], 1);
        assert_eq!(items[0]["column_profiles"]["columns_total"], 2);
        assert_eq!(items[1]["path"], json!(second));
        assert_eq!(items[1]["rows_total"], 1);
        assert!(items[0]["elapsed_s"].as_f64().unwrap() >= 0.0);
    }

    #[test]
    fn batch_prepare_csv_with_profile_matches_single_profile_payload() {
        let dir = TestDir::new("batch-prepare-profile-match");
        let csv_path = dir.join("sample.csv");
        let manifest = dir.join("manifest.json");
        write_text(
            &csv_path,
            "交易日期,交易金额,交易账号\n20260102,-123.45,6222000000000000001\n2026-01-03,88.00,6222000000000000002\n",
        );
        write_text(
            &manifest,
            &json!({
                "items": [{"path": csv_path, "limit": 1, "encoding": "utf-8-sig"}]
            })
            .to_string(),
        );

        let payload = batch_prepare_csv_with_profile(&manifest).unwrap();
        let profile = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        assert_eq!(payload["items"][0]["column_profiles"], profile);
        assert_eq!(
            payload["items"][0]["sample_rows"].as_array().unwrap().len(),
            1
        );
    }
}
