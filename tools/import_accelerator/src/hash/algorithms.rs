use anyhow::{bail, Result};

#[derive(Debug, Default, PartialEq, Eq)]
pub(crate) struct HashSelection {
    md5: bool,
    sha256: bool,
}

impl HashSelection {
    pub(crate) fn from_labels(labels: &[String]) -> Result<Self> {
        let mut selection = Self::default();
        for label in labels {
            match label.as_str() {
                "md5" => selection.md5 = true,
                "sha256" => selection.sha256 = true,
                other => bail!("unsupported hash algorithm: {other}"),
            }
        }
        Ok(selection)
    }

    pub(crate) fn wants_md5(&self) -> bool {
        self.md5
    }

    pub(crate) fn wants_sha256(&self) -> bool {
        self.sha256
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn labels(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn hash_selection_accepts_supported_algorithms_and_deduplicates() {
        let selection = HashSelection::from_labels(&labels(&["md5", "sha256", "md5"])).unwrap();

        assert!(selection.wants_md5());
        assert!(selection.wants_sha256());
    }

    #[test]
    fn hash_selection_allows_empty_labels_for_existing_call_semantics() {
        let selection = HashSelection::from_labels(&[]).unwrap();

        assert!(!selection.wants_md5());
        assert!(!selection.wants_sha256());
    }

    #[test]
    fn hash_selection_rejects_unsupported_algorithm() {
        let err = HashSelection::from_labels(&labels(&["md5", "sha1"])).unwrap_err();

        assert_eq!(err.to_string(), "unsupported hash algorithm: sha1");
    }
}
