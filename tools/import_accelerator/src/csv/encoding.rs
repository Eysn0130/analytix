use anyhow::{anyhow, Context, Result};
use encoding_rs::Encoding;
use encoding_rs_io::DecodeReaderBytesBuilder;
use std::cell::RefCell;
use std::fs::File;
use std::io::{self, BufReader, Read};
use std::path::PathBuf;
use std::rc::Rc;

pub(crate) fn detect_encoding(path: &PathBuf) -> Result<&'static str> {
    let mut file = File::open(path).with_context(|| format!("open {}", path.display()))?;
    let mut sample = vec![0_u8; 64 * 1024];
    let read = file.read(&mut sample)?;
    sample.truncate(read);
    Ok(detect_encoding_from_sample(&sample))
}

fn detect_encoding_from_sample(sample: &[u8]) -> &'static str {
    if sample.starts_with(&[0xef, 0xbb, 0xbf]) {
        return "utf-8-sig";
    }
    if std::str::from_utf8(sample).is_ok() {
        return "utf-8";
    }
    for label in ["gb18030", "gbk"] {
        if let Some(encoding) = Encoding::for_label(label.as_bytes()) {
            let (_, _, had_errors) = encoding.decode(sample);
            if !had_errors {
                return label;
            }
        }
    }
    "gb18030"
}

pub(crate) fn normalize_encoding_label(label: &str) -> Result<String> {
    let normalized = label.trim().to_ascii_lowercase();
    encoding_for_label(&normalized)?;
    Ok(normalized)
}

fn encoding_for_label(label: &str) -> Result<&'static Encoding> {
    let normalized = match label.trim().to_ascii_lowercase().as_str() {
        "utf-8-sig" => "utf-8",
        _ => label.trim(),
    };
    Encoding::for_label(normalized.as_bytes())
        .ok_or_else(|| anyhow!("unsupported encoding: {label}"))
}

pub(crate) fn counted_decoded_file_reader(
    path: &PathBuf,
    encoding: &str,
) -> Result<(impl Read, Rc<RefCell<PhysicalLineCounter>>)> {
    let file = File::open(path).with_context(|| format!("open {}", path.display()))?;
    let reader = BufReader::with_capacity(1024 * 1024, file);
    let decoder = encoding_for_label(encoding)?;
    let counter = Rc::new(RefCell::new(PhysicalLineCounter::default()));
    let counted = CountingReader {
        inner: reader,
        counter: Rc::clone(&counter),
    };
    let decoded = DecodeReaderBytesBuilder::new()
        .encoding(Some(decoder))
        .build(counted);
    Ok((decoded, counter))
}

#[derive(Default)]
pub(crate) struct PhysicalLineCounter {
    newline_count: u64,
    bytes_read_total: u64,
    last_byte: Option<u8>,
}

impl PhysicalLineCounter {
    fn observe(&mut self, bytes: &[u8]) {
        if bytes.is_empty() {
            return;
        }
        self.bytes_read_total += bytes.len() as u64;
        self.newline_count += bytes.iter().filter(|byte| **byte == b'\n').count() as u64;
        self.last_byte = bytes.last().copied();
    }

    pub(crate) fn rows_total(&self) -> u64 {
        if self.bytes_read_total == 0 {
            return 0;
        }
        let physical_lines = self.newline_count + u64::from(self.last_byte != Some(b'\n'));
        physical_lines.saturating_sub(1)
    }
}

struct CountingReader<R> {
    inner: R,
    counter: Rc<RefCell<PhysicalLineCounter>>,
}

impl<R: Read> Read for CountingReader<R> {
    fn read(&mut self, buffer: &mut [u8]) -> io::Result<usize> {
        let read = self.inner.read(buffer)?;
        if read > 0 {
            self.counter.borrow_mut().observe(&buffer[..read]);
        }
        Ok(read)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn physical_data_rows_matches_csv_preview_edges() {
        let cases = [
            (b"".as_slice(), 0),
            (b"a,b".as_slice(), 0),
            (b"a,b\n".as_slice(), 0),
            (b"a,b\n1,2".as_slice(), 1),
            (b"a,b\r\n1,2\r\n3,4\r\n".as_slice(), 2),
        ];
        for (raw, expected) in cases {
            let mut counter = PhysicalLineCounter::default();
            counter.observe(raw);
            assert_eq!(counter.rows_total(), expected);
        }
    }

    #[test]
    fn detect_encoding_prefers_utf8_before_gb_compatible_decoders() {
        assert_eq!(
            detect_encoding_from_sample("交易时间,交易金额\n".as_bytes()),
            "utf-8"
        );
        assert_eq!(
            detect_encoding_from_sample(b"\xef\xbb\xbftransaction_time,amount\n"),
            "utf-8-sig"
        );

        let gb18030 = Encoding::for_label(b"gb18030").unwrap();
        let (encoded, _, _) = gb18030.encode("交易时间,交易金额\n");
        assert_eq!(detect_encoding_from_sample(encoded.as_ref()), "gb18030");
    }
}
