use anyhow::{Context, Result};
use std::ffi::{OsStr, OsString};
use std::io;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

pub(crate) struct PendingOutput {
    final_path: PathBuf,
    temp_path: PathBuf,
}

impl PendingOutput {
    pub(crate) fn new(final_path: &Path) -> Result<Self> {
        ensure_output_parent(final_path)?;
        Ok(Self {
            final_path: final_path.to_path_buf(),
            temp_path: temporary_output_path(final_path),
        })
    }

    pub(crate) fn temp_path(&self) -> &Path {
        &self.temp_path
    }

    pub(crate) fn commit(&self) -> Result<()> {
        match replace_output_file(&self.temp_path, &self.final_path) {
            Ok(()) => Ok(()),
            Err(err) => {
                self.discard();
                Err(err)
            }
        }
    }

    pub(crate) fn discard(&self) {
        let _ = std::fs::remove_file(&self.temp_path);
    }
}

fn ensure_output_parent(output: &Path) -> Result<()> {
    if let Some(parent) = output.parent() {
        if !parent.as_os_str().is_empty() {
            std::fs::create_dir_all(parent)?;
        }
    }
    Ok(())
}

fn temporary_output_path(output: &Path) -> PathBuf {
    let file_name = output
        .file_name()
        .unwrap_or_else(|| OsStr::new("output.csv"));
    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_nanos())
        .unwrap_or(0);
    let mut temp_name = OsString::from(".");
    temp_name.push(file_name);
    temp_name.push(format!(".{}.{}.tmp", std::process::id(), unique));
    output.with_file_name(temp_name)
}

fn replace_output_file(temp_output: &Path, output: &Path) -> Result<()> {
    match std::fs::rename(temp_output, output) {
        Ok(()) => return Ok(()),
        Err(_err) if output.exists() => {}
        Err(err) => {
            return Err(err).with_context(|| format!("replace {}", output.display()));
        }
    }

    match std::fs::remove_file(output) {
        Ok(()) => {}
        Err(err) if err.kind() == io::ErrorKind::NotFound => {}
        Err(err) => {
            return Err(err).with_context(|| format!("replace {}", output.display()));
        }
    }
    std::fs::rename(temp_output, output).with_context(|| format!("replace {}", output.display()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_text, TestDir};
    use std::fs;

    #[test]
    fn pending_output_commit_replaces_existing_output_file() {
        let dir = TestDir::new("pending-output-replace");
        let output_path = dir.join("nested/clean.csv");
        fs::create_dir_all(output_path.parent().unwrap()).unwrap();
        write_text(&output_path, "old\n");

        let pending = PendingOutput::new(&output_path).unwrap();
        write_text(pending.temp_path(), "new\n");
        pending.commit().unwrap();

        assert_eq!(fs::read_to_string(&output_path).unwrap(), "new\n");
        assert!(!pending.temp_path().exists());
    }

    #[test]
    fn pending_output_discard_removes_temporary_file_without_touching_output() {
        let dir = TestDir::new("pending-output-discard");
        let output_path = dir.join("clean.csv");
        write_text(&output_path, "old\n");

        let pending = PendingOutput::new(&output_path).unwrap();
        write_text(pending.temp_path(), "partial\n");
        pending.discard();

        assert_eq!(fs::read_to_string(&output_path).unwrap(), "old\n");
        assert!(!pending.temp_path().exists());
    }
}
