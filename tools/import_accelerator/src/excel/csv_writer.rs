use super::pandas_compat::{excel_cell_to_pandas_text, pandas_header_names};
use crate::output_file::PendingOutput;
use anyhow::{Context, Result};
use calamine::{Data, Range};
use csv::WriterBuilder;
use std::fs::File;
use std::path::Path;

pub(crate) fn write_excel_range_csv(range: &Range<Data>, output: &Path) -> Result<u64> {
    let pending_output = PendingOutput::new(output)?;
    let result = write_excel_range_csv_to_temp(range, pending_output.temp_path(), output);

    match result {
        Ok(rows) => pending_output.commit().map(|()| rows),
        Err(err) => {
            pending_output.discard();
            Err(err)
        }
    }
}

fn write_excel_range_csv_to_temp(
    range: &Range<Data>,
    temp_output: &Path,
    final_output: &Path,
) -> Result<u64> {
    let file =
        File::create(temp_output).with_context(|| format!("create {}", final_output.display()))?;
    let mut writer = WriterBuilder::new().from_writer(file);
    let mut rows: u64 = 0;
    for row in range.rows() {
        let mut values = row
            .iter()
            .map(excel_cell_to_pandas_text)
            .collect::<Vec<_>>();
        if rows == 0 {
            values = pandas_header_names(values);
        }
        writer.write_record(values)?;
        rows += 1;
    }
    writer.flush()?;
    Ok(rows.saturating_sub(1))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::TestDir;
    use calamine::Cell;

    fn row(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    fn records(path: &Path) -> Vec<Vec<String>> {
        let mut reader = csv::ReaderBuilder::new()
            .has_headers(false)
            .from_path(path)
            .unwrap();
        reader
            .records()
            .map(|record| {
                record
                    .unwrap()
                    .iter()
                    .map(|cell| cell.to_string())
                    .collect()
            })
            .collect()
    }

    #[test]
    fn write_excel_range_csv_writes_pandas_headers_rows_and_count() {
        let range = Range::from_sparse(vec![
            Cell::new((0, 0), Data::String("交易金额".to_string())),
            Cell::new((0, 2), Data::String("交易金额".to_string())),
            Cell::new((1, 0), Data::Int(100)),
            Cell::new((1, 1), Data::Float(80.5)),
            Cell::new((1, 2), Data::Bool(true)),
            Cell::new((2, 0), Data::String("工资入账".to_string())),
        ]);
        let dir = TestDir::new("excel-csv-writer");
        let output = dir.join("nested/out.csv");

        let rows = write_excel_range_csv(&range, &output).unwrap();

        assert_eq!(rows, 2);
        assert_eq!(
            records(&output),
            vec![
                row(&["交易金额", "Unnamed: 1", "交易金额.1"]),
                row(&["100", "80.5", "True"]),
                row(&["工资入账", "", ""]),
            ]
        );
    }

    #[test]
    fn write_excel_range_csv_returns_zero_for_header_only_range() {
        let range = Range::from_sparse(vec![
            Cell::new((0, 0), Data::String("交易时间".to_string())),
            Cell::new((0, 1), Data::String("交易金额".to_string())),
        ]);
        let dir = TestDir::new("excel-csv-writer-header-only");
        let output = dir.join("out.csv");

        let rows = write_excel_range_csv(&range, &output).unwrap();

        assert_eq!(rows, 0);
        assert_eq!(records(&output), vec![row(&["交易时间", "交易金额"])]);
    }

    #[test]
    fn write_excel_range_csv_replaces_existing_output() {
        let range = Range::from_sparse(vec![
            Cell::new((0, 0), Data::String("交易时间".to_string())),
            Cell::new((1, 0), Data::String("2026-01-01".to_string())),
        ]);
        let dir = TestDir::new("excel-csv-writer-replace");
        let output = dir.join("out.csv");
        std::fs::write(&output, "old\n").unwrap();

        let rows = write_excel_range_csv(&range, &output).unwrap();

        assert_eq!(rows, 1);
        assert_eq!(
            records(&output),
            vec![row(&["交易时间"]), row(&["2026-01-01"])]
        );
    }

    #[test]
    fn write_excel_range_csv_returns_zero_for_empty_range() {
        let range = Range::<Data>::empty();
        let dir = TestDir::new("excel-csv-writer-empty");
        let output = dir.join("out.csv");

        let rows = write_excel_range_csv(&range, &output).unwrap();

        assert_eq!(rows, 0);
        assert_eq!(records(&output), Vec::<Vec<String>>::new());
    }
}
