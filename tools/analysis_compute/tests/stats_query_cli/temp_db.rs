use anyhow::{Context, Result};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

pub(crate) struct TempDb {
    path: PathBuf,
}

impl TempDb {
    pub(crate) fn new(label: &str) -> Result<Self> {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .context("system clock before UNIX_EPOCH")?
            .as_nanos();
        let path = std::env::temp_dir().join(format!(
            "analytix-{label}-{}-{unique}.duckdb",
            std::process::id()
        ));
        Ok(Self { path })
    }

    pub(crate) fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for TempDb {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
        let mut wal_path = self.path.as_os_str().to_os_string();
        wal_path.push(".wal");
        let _ = fs::remove_file(PathBuf::from(wal_path));
    }
}
