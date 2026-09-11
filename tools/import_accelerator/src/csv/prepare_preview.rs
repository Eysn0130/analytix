use super::encoding::counted_decoded_file_reader;
use super::preview::{read_csv_header, CsvPreview, PreviewColumns};
use anyhow::Result;
use csv::{ReaderBuilder, StringRecord};
use std::io;
use std::path::PathBuf;

pub(crate) fn scan_csv_preview(path: &PathBuf, encoding: &str, limit: usize) -> Result<CsvPreview> {
    let (decoded, line_counter) = counted_decoded_file_reader(path, encoding)?;

    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(decoded);
    let mut record = StringRecord::new();
    let header = read_csv_header(&mut reader, &mut record)?;
    let sample_rows =
        collect_preview_sample(&mut reader, &mut record, &header.preview_columns, limit)?;

    let mut decoded = reader.into_inner();
    io::copy(&mut decoded, &mut io::sink())?;
    let rows_total = line_counter.borrow().rows_total();

    Ok(CsvPreview {
        rows_total,
        header_preview: header.preview_columns.header_preview,
        sample_rows,
    })
}

fn collect_preview_sample<R: io::Read>(
    reader: &mut csv::Reader<R>,
    record: &mut StringRecord,
    preview_columns: &PreviewColumns,
    limit: usize,
) -> Result<Vec<Vec<String>>> {
    let mut sample_rows: Vec<Vec<String>> = Vec::new();
    if preview_columns.is_empty() || limit == 0 {
        return Ok(sample_rows);
    }

    while sample_rows.len() < limit {
        record.clear();
        if !reader.read_record(record)? {
            break;
        }
        if let Some(row) = preview_columns.sample_row(record) {
            sample_rows.push(row);
        }
    }
    Ok(sample_rows)
}
