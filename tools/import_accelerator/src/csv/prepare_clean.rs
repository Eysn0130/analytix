use super::encoding::counted_decoded_file_reader;
use super::preview::{read_csv_header, CsvPreview, PreviewColumns};
use crate::output_file::PendingOutput;
use anyhow::{Context, Result};
use csv::{ReaderBuilder, StringRecord, WriterBuilder};
use std::fs::File;
use std::io;
use std::path::Path;
use std::path::PathBuf;

pub(crate) struct PreparedCsvCleanResult {
    pub(crate) preview: CsvPreview,
    pub(crate) preclean_rows: u64,
    pub(crate) output: PathBuf,
}

pub(crate) struct PreparedCsvMaybeCleanResult {
    pub(crate) preview: CsvPreview,
    pub(crate) preclean_rows: u64,
    pub(crate) output: Option<PathBuf>,
}

pub(crate) fn clean_csv_to_output(
    path: &PathBuf,
    encoding: &str,
    limit: usize,
    output: &PathBuf,
) -> Result<PreparedCsvCleanResult> {
    let pending_output = PendingOutput::new(output)?;
    let result =
        clean_csv_to_temp_output(path, encoding, limit, pending_output.temp_path(), output);

    match result {
        Ok(cleaned) => pending_output.commit().map(|()| cleaned),
        Err(err) => {
            pending_output.discard();
            Err(err)
        }
    }
}

pub(crate) fn clean_csv_to_output_if_needed(
    path: &PathBuf,
    encoding: &str,
    limit: usize,
    output: &PathBuf,
) -> Result<PreparedCsvMaybeCleanResult> {
    if !can_reuse_original_when_clean(encoding) {
        let cleaned = clean_csv_to_output(path, encoding, limit, output)?;
        return Ok(PreparedCsvMaybeCleanResult {
            preview: cleaned.preview,
            preclean_rows: cleaned.preclean_rows,
            output: Some(cleaned.output),
        });
    }

    let scanned = scan_csv_cleanliness(path, encoding, limit)?;
    if !scanned.changed {
        return Ok(PreparedCsvMaybeCleanResult {
            preview: scanned.preview,
            preclean_rows: scanned.preclean_rows,
            output: None,
        });
    }

    let cleaned = clean_csv_to_output(path, encoding, limit, output)?;
    Ok(PreparedCsvMaybeCleanResult {
        preview: cleaned.preview,
        preclean_rows: cleaned.preclean_rows,
        output: Some(cleaned.output),
    })
}

fn can_reuse_original_when_clean(encoding: &str) -> bool {
    encoding.trim().eq_ignore_ascii_case("utf-8")
}

fn clean_csv_to_temp_output(
    path: &PathBuf,
    encoding: &str,
    limit: usize,
    temp_output: &Path,
    final_output: &PathBuf,
) -> Result<PreparedCsvCleanResult> {
    let (decoded, line_counter) = counted_decoded_file_reader(path, encoding)?;
    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(decoded);
    let file =
        File::create(temp_output).with_context(|| format!("create {}", final_output.display()))?;
    let mut writer = WriterBuilder::new().from_writer(file);
    let mut rows_written: u64 = 0;

    let mut record = StringRecord::new();
    let header = read_csv_header(&mut reader, &mut record)?;
    if header.has_header {
        let cleaned = record.iter().map(clean_csv_cell).collect::<Vec<_>>();
        writer.write_record(cleaned)?;
        rows_written += 1;
    }

    let (sample_rows, body_rows_written) = write_cleaned_body_and_collect_sample(
        &mut reader,
        &mut writer,
        &mut record,
        &header.preview_columns,
        limit,
    )?;
    rows_written += body_rows_written;
    writer.flush()?;

    let mut decoded = reader.into_inner();
    io::copy(&mut decoded, &mut io::sink())?;
    let rows_total = line_counter.borrow().rows_total();
    let preclean_rows = rows_written.saturating_sub(1);

    Ok(PreparedCsvCleanResult {
        preview: CsvPreview {
            rows_total,
            header_preview: header.preview_columns.header_preview,
            sample_rows,
        },
        preclean_rows,
        output: final_output.clone(),
    })
}

fn clean_csv_cell(value: &str) -> String {
    value
        .chars()
        .filter(|ch| {
            let code = *ch as u32;
            !(code <= 0x1f || code == 0x7f || *ch == '\u{00a0}')
        })
        .collect::<String>()
        .trim()
        .to_string()
}

struct CsvCleanScanResult {
    preview: CsvPreview,
    preclean_rows: u64,
    changed: bool,
}

fn scan_csv_cleanliness(
    path: &PathBuf,
    encoding: &str,
    limit: usize,
) -> Result<CsvCleanScanResult> {
    let (decoded, line_counter) = counted_decoded_file_reader(path, encoding)?;
    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(decoded);
    let mut record = StringRecord::new();
    let header = read_csv_header(&mut reader, &mut record)?;
    let mut changed = header.has_header && record_needs_cleaning(&record);

    let (sample_rows, body_rows_seen, body_changed) =
        scan_body_and_collect_sample(&mut reader, &mut record, &header.preview_columns, limit)?;
    changed |= body_changed;

    let mut decoded = reader.into_inner();
    io::copy(&mut decoded, &mut io::sink())?;
    let rows_total = line_counter.borrow().rows_total();

    Ok(CsvCleanScanResult {
        preview: CsvPreview {
            rows_total,
            header_preview: header.preview_columns.header_preview,
            sample_rows,
        },
        preclean_rows: body_rows_seen,
        changed,
    })
}

fn record_needs_cleaning(record: &StringRecord) -> bool {
    record.iter().any(|value| clean_csv_cell(value) != value)
}

fn scan_body_and_collect_sample<R: io::Read>(
    reader: &mut csv::Reader<R>,
    record: &mut StringRecord,
    preview_columns: &PreviewColumns,
    limit: usize,
) -> Result<(Vec<Vec<String>>, u64, bool)> {
    let mut sample_rows: Vec<Vec<String>> = Vec::new();
    let mut rows_seen = 0_u64;
    let mut changed = false;
    loop {
        record.clear();
        if !reader.read_record(record)? {
            break;
        }
        if !preview_columns.is_empty() && sample_rows.len() < limit {
            if let Some(row) = preview_columns.sample_row(record) {
                sample_rows.push(row);
            }
        }
        changed |= record_needs_cleaning(record);
        rows_seen += 1;
    }
    Ok((sample_rows, rows_seen, changed))
}

fn write_cleaned_body_and_collect_sample<R: io::Read, W: io::Write>(
    reader: &mut csv::Reader<R>,
    writer: &mut csv::Writer<W>,
    record: &mut StringRecord,
    preview_columns: &PreviewColumns,
    limit: usize,
) -> Result<(Vec<Vec<String>>, u64)> {
    let mut sample_rows: Vec<Vec<String>> = Vec::new();
    let mut rows_written = 0_u64;
    loop {
        record.clear();
        if !reader.read_record(record)? {
            break;
        }
        if !preview_columns.is_empty() && sample_rows.len() < limit {
            if let Some(row) = preview_columns.sample_row(record) {
                sample_rows.push(row);
            }
        }
        let cleaned = record.iter().map(clean_csv_cell).collect::<Vec<_>>();
        writer.write_record(cleaned)?;
        rows_written += 1;
    }
    Ok((sample_rows, rows_written))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_gb18030, write_text, TestDir};

    #[test]
    fn clean_if_needed_reuses_clean_utf8_input_without_output_copy() {
        let dir = TestDir::new("clean-if-needed-clean-utf8");
        let csv_path = dir.join("clean.csv");
        let output_path = dir.join("cleaned.csv");
        write_text(&csv_path, "交易时间,交易金额\n2026-01-01,100\n");

        let result = clean_csv_to_output_if_needed(&csv_path, "utf-8", 5, &output_path).unwrap();

        assert_eq!(result.preview.rows_total, 1);
        assert_eq!(result.preclean_rows, 1);
        assert!(result.output.is_none());
        assert!(!output_path.exists());
    }

    #[test]
    fn clean_if_needed_writes_output_for_dirty_utf8_input() {
        let dir = TestDir::new("clean-if-needed-dirty-utf8");
        let csv_path = dir.join("dirty.csv");
        let output_path = dir.join("cleaned.csv");
        write_text(&csv_path, "交易时间,交易金额\n 2026-01-01\t, 100 \n");

        let result = clean_csv_to_output_if_needed(&csv_path, "utf-8", 5, &output_path).unwrap();

        assert_eq!(result.preview.rows_total, 1);
        assert_eq!(result.preclean_rows, 1);
        assert_eq!(result.output, Some(output_path.clone()));
        assert_eq!(
            std::fs::read_to_string(&output_path).unwrap(),
            "交易时间,交易金额\n2026-01-01,100\n"
        );
    }

    #[test]
    fn clean_if_needed_still_transcodes_non_utf8_input() {
        let dir = TestDir::new("clean-if-needed-gb18030");
        let csv_path = dir.join("gb18030.csv");
        let output_path = dir.join("cleaned.csv");
        write_gb18030(&csv_path, "交易时间,交易金额\n2026-01-01,100\n");

        let result = clean_csv_to_output_if_needed(&csv_path, "gb18030", 5, &output_path).unwrap();

        assert_eq!(result.output, Some(output_path.clone()));
        assert_eq!(
            std::fs::read_to_string(&output_path).unwrap(),
            "交易时间,交易金额\n2026-01-01,100\n"
        );
    }
}
