use super::preview_normalize::{preview_cell_text, sanitize_csv_header};
use anyhow::Result;
use csv::StringRecord;
use std::io::Read;

#[derive(Debug)]
pub(crate) struct CsvPreview {
    pub(crate) rows_total: u64,
    pub(crate) header_preview: Vec<String>,
    pub(crate) sample_rows: Vec<Vec<String>>,
}

#[derive(Debug)]
pub(crate) struct PreviewColumns {
    indexes: Vec<usize>,
    pub(crate) header_preview: Vec<String>,
}

impl PreviewColumns {
    fn from_headers(headers: &[String]) -> Self {
        let mut indexes = Vec::new();
        let mut header_preview = Vec::new();
        for (idx, header) in headers.iter().enumerate() {
            let normalized = preview_cell_text(header);
            if !normalized.is_empty() {
                indexes.push(idx);
                header_preview.push(normalized);
            }
        }
        Self {
            indexes,
            header_preview,
        }
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.indexes.is_empty()
    }

    pub(crate) fn indexes(&self) -> &[usize] {
        &self.indexes
    }

    pub(crate) fn sample_row(&self, record: &StringRecord) -> Option<Vec<String>> {
        let row = self
            .indexes
            .iter()
            .map(|idx| preview_cell_text(record.get(*idx).unwrap_or("")))
            .collect::<Vec<_>>();
        if row.iter().any(|cell| !cell.is_empty()) {
            Some(row)
        } else {
            None
        }
    }
}

#[derive(Debug)]
pub(crate) struct CsvHeader {
    pub(crate) has_header: bool,
    pub(crate) preview_columns: PreviewColumns,
}

pub(crate) fn read_csv_header<R: Read>(
    reader: &mut csv::Reader<R>,
    record: &mut StringRecord,
) -> Result<CsvHeader> {
    record.clear();
    let has_header = reader.read_record(record)?;
    let headers = if has_header {
        record.iter().map(sanitize_csv_header).collect::<Vec<_>>()
    } else {
        Vec::new()
    };
    Ok(CsvHeader {
        has_header,
        preview_columns: PreviewColumns::from_headers(&headers),
    })
}
