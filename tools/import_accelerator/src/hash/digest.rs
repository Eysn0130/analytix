use super::algorithms::HashSelection;
use super::hex::to_upper_hex;
use anyhow::Result;
use md5::Md5;
use sha2::{Digest, Sha256};
use std::io::Read;

#[derive(Debug, PartialEq, Eq)]
pub(crate) struct HashDigests {
    pub(crate) md5: Option<String>,
    pub(crate) sha256: Option<String>,
}

pub(crate) fn hash_reader<R: Read>(
    reader: &mut R,
    selection: &HashSelection,
) -> Result<HashDigests> {
    let mut md5_hasher = Md5::new();
    let mut sha256_hasher = Sha256::new();
    let mut buffer = [0_u8; 1024 * 1024];

    loop {
        let read = reader.read(&mut buffer)?;
        if read == 0 {
            break;
        }
        if selection.wants_md5() {
            md5_hasher.update(&buffer[..read]);
        }
        if selection.wants_sha256() {
            sha256_hasher.update(&buffer[..read]);
        }
    }

    Ok(HashDigests {
        md5: selection
            .wants_md5()
            .then(|| to_upper_hex(&md5_hasher.finalize())),
        sha256: selection
            .wants_sha256()
            .then(|| to_upper_hex(&sha256_hasher.finalize())),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    fn labels(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn hash_reader_computes_selected_md5_and_sha256_uppercase() {
        let selection = HashSelection::from_labels(&labels(&["md5", "sha256"])).unwrap();
        let mut reader = Cursor::new(b"abc");

        let digests = hash_reader(&mut reader, &selection).unwrap();

        assert_eq!(
            digests,
            HashDigests {
                md5: Some("900150983CD24FB0D6963F7D28E17F72".to_string()),
                sha256: Some(
                    "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD".to_string()
                ),
            }
        );
    }

    #[test]
    fn hash_reader_only_returns_selected_algorithms() {
        let selection = HashSelection::from_labels(&labels(&["sha256"])).unwrap();
        let mut reader = Cursor::new(b"abc");

        let digests = hash_reader(&mut reader, &selection).unwrap();

        assert_eq!(digests.md5, None);
        assert_eq!(
            digests.sha256,
            Some("BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD".to_string())
        );
    }

    #[test]
    fn hash_reader_consumes_reader_even_when_no_algorithm_is_selected() {
        let selection = HashSelection::from_labels(&[]).unwrap();
        let mut reader = Cursor::new(b"abc");

        let digests = hash_reader(&mut reader, &selection).unwrap();

        assert_eq!(
            digests,
            HashDigests {
                md5: None,
                sha256: None,
            }
        );
        assert_eq!(reader.position(), 3);
    }
}
