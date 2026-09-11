use super::algorithms::HashSelection;
use super::digest::hash_reader;
use super::hex::to_lower_hex;
use anyhow::{Context, Result};
use md5::Md5;
use serde_json::json;
use sha2::Digest;
use std::fs::File;
use std::io::BufReader;
use std::path::PathBuf;

pub(crate) fn file_hashes(
    path: &PathBuf,
    algos: &[String],
) -> Result<serde_json::Map<String, serde_json::Value>> {
    let selection = HashSelection::from_labels(algos)?;
    let file = File::open(path).with_context(|| format!("open {}", path.display()))?;
    let mut reader = BufReader::with_capacity(1024 * 1024, file);
    let digests = hash_reader(&mut reader, &selection)?;

    let mut out = serde_json::Map::new();
    if let Some(md5) = digests.md5 {
        out.insert("md5".to_string(), json!(md5));
    }
    if let Some(sha256) = digests.sha256 {
        out.insert("sha256".to_string(), json!(sha256));
    }
    Ok(out)
}

pub(crate) fn md5_lower_10(value: &str) -> String {
    let mut hasher = Md5::new();
    hasher.update(value.as_bytes());
    to_lower_hex(&hasher.finalize())[..10].to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_text, TestDir};

    fn labels(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn file_hashes_validates_algorithm_before_opening_file() {
        let missing = PathBuf::from("missing.csv");
        let err = file_hashes(&missing, &labels(&["sha1"])).unwrap_err();

        assert_eq!(err.to_string(), "unsupported hash algorithm: sha1");
    }

    #[test]
    fn file_hashes_preserves_empty_algorithm_call_semantics() {
        let dir = TestDir::new("file-hashes-empty-algos");
        let path = dir.join("sample.txt");
        write_text(&path, "abc");

        assert!(file_hashes(&path, &[]).unwrap().is_empty());
    }

    #[test]
    fn md5_lower_10_preserves_lowercase_short_tag() {
        assert_eq!(md5_lower_10("abc"), "900150983c");
    }
}
