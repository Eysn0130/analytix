use encoding_rs::Encoding;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

pub(crate) struct TestDir {
    path: PathBuf,
}

impl TestDir {
    pub(crate) fn new(name: &str) -> Self {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let path = std::env::temp_dir().join(format!(
            "analytix-import-accelerator-{name}-{}-{unique}",
            std::process::id()
        ));
        fs::create_dir_all(&path).unwrap();
        Self { path }
    }

    pub(crate) fn join(&self, name: &str) -> PathBuf {
        self.path.join(name)
    }
}

impl Drop for TestDir {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.path);
    }
}

pub(crate) fn write_text(path: &Path, text: &str) {
    fs::write(path, text.as_bytes()).unwrap();
}

pub(crate) fn write_gb18030(path: &Path, text: &str) {
    let encoding = Encoding::for_label(b"gb18030").unwrap();
    let (encoded, _, had_errors) = encoding.encode(text);
    assert!(!had_errors);
    fs::write(path, encoded.as_ref()).unwrap();
}

pub(crate) fn string_array(value: &serde_json::Value, key: &str) -> Vec<String> {
    value[key]
        .as_array()
        .unwrap()
        .iter()
        .map(|item| item.as_str().unwrap().to_string())
        .collect()
}

pub(crate) fn string_rows(value: &serde_json::Value, key: &str) -> Vec<Vec<String>> {
    value[key]
        .as_array()
        .unwrap()
        .iter()
        .map(|row| {
            row.as_array()
                .unwrap()
                .iter()
                .map(|cell| cell.as_str().unwrap().to_string())
                .collect()
        })
        .collect()
}
