use super::output_path::{section_csv_path, should_write_section_csv};
use super::ranges::SectionRange;
use super::row_normalize::{pad_row, trim_width};
use crate::output_file::PendingOutput;
use anyhow::{Context, Result};
use csv::WriterBuilder;
use serde_json::{json, Value};
use std::fs::File;
use std::path::Path;

pub(crate) fn write_section_output(
    input: &Path,
    output_dir: &Path,
    section: &SectionRange,
    data_rows: &[Vec<String>],
) -> Result<Option<Value>> {
    let width = trim_width(&section.header_cells, data_rows);
    if width == 0 {
        return Ok(None);
    }

    let csv_path = section_csv_path(input, output_dir, section.start, section.kind);
    if should_write_section_csv(&csv_path, input) {
        write_section_csv(&csv_path, &section.header_cells, data_rows, width)?;
    }

    Ok(Some(json!({
        "path": csv_path,
        "kind": section.kind,
    })))
}

fn write_section_csv(
    csv_path: &Path,
    header_cells: &[String],
    data_rows: &[Vec<String>],
    width: usize,
) -> Result<()> {
    let pending_output = PendingOutput::new(csv_path)?;
    let result = write_section_csv_to_temp(
        pending_output.temp_path(),
        csv_path,
        header_cells,
        data_rows,
        width,
    );

    match result {
        Ok(()) => pending_output.commit(),
        Err(err) => {
            pending_output.discard();
            Err(err)
        }
    }
}

fn write_section_csv_to_temp(
    temp_output: &Path,
    final_output: &Path,
    header_cells: &[String],
    data_rows: &[Vec<String>],
    width: usize,
) -> Result<()> {
    let file =
        File::create(temp_output).with_context(|| format!("create {}", final_output.display()))?;
    let mut writer = WriterBuilder::new().from_writer(file);
    writer.write_record(pad_row(header_cells, width))?;
    for row in data_rows {
        writer.write_record(pad_row(row, width))?;
    }
    writer.flush()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_text, TestDir};

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
    fn write_section_output_writes_padded_csv_and_payload_shape() {
        let dir = TestDir::new("section-writer");
        let input = dir.join("账户子账户混合.xlsx");
        let output_dir = dir.join("out");
        std::fs::create_dir_all(&output_dir).unwrap();
        write_text(&input, "source");
        let section = SectionRange {
            start: 2,
            end: 5,
            kind: "fc_account",
            header_cells: row(&["账户开户名称", "开户人证件号码", "", ""]),
        };
        let data_rows = vec![
            row(&["张三", "110101", "62220001", ""]),
            row(&["李四", "220202", "", ""]),
        ];

        let output = write_section_output(&input, &output_dir, &section, &data_rows)
            .unwrap()
            .unwrap();

        let csv_path = section_csv_path(&input, &output_dir, section.start, section.kind);
        assert_eq!(
            output,
            json!({
                "path": csv_path.clone(),
                "kind": "fc_account",
            })
        );
        assert_eq!(
            records(&csv_path),
            vec![
                row(&["账户开户名称", "开户人证件号码", ""]),
                row(&["张三", "110101", "62220001"]),
                row(&["李四", "220202", ""]),
            ]
        );
    }

    #[test]
    fn write_section_csv_replaces_existing_csv() {
        let dir = TestDir::new("section-writer-replace");
        let input = dir.join("账户子账户混合.xlsx");
        let output_dir = dir.join("out");
        std::fs::create_dir_all(&output_dir).unwrap();
        write_text(&input, "source");
        let section = SectionRange {
            start: 2,
            end: 4,
            kind: "fc_account",
            header_cells: row(&["账户开户名称", "开户人证件号码"]),
        };
        let data_rows = vec![row(&["张三", "110101"])];
        let csv_path = section_csv_path(&input, &output_dir, section.start, section.kind);
        write_text(&csv_path, "stale\n");

        write_section_csv(&csv_path, &section.header_cells, &data_rows, 2).unwrap();

        assert_eq!(
            records(&csv_path),
            vec![
                row(&["账户开户名称", "开户人证件号码"]),
                row(&["张三", "110101"])
            ]
        );
    }

    #[test]
    fn write_section_csv_discards_temp_file_when_commit_fails() {
        let dir = TestDir::new("section-writer-commit-failure");
        let csv_path = dir.join("blocked.csv");
        std::fs::create_dir_all(&csv_path).unwrap();

        let error = write_section_csv(
            &csv_path,
            &row(&["账户开户名称", "开户人证件号码"]),
            &[row(&["张三", "110101"])],
            2,
        )
        .unwrap_err();

        assert!(error.to_string().contains("replace"));
        assert!(csv_path.is_dir());
        assert_eq!(
            std::fs::read_dir(csv_path.parent().unwrap())
                .unwrap()
                .map(|entry| entry.unwrap().file_name())
                .collect::<Vec<_>>(),
            vec![std::ffi::OsString::from("blocked.csv")]
        );
    }

    #[test]
    fn write_section_output_skips_empty_width_without_payload_or_file() {
        let dir = TestDir::new("section-writer-empty");
        let input = dir.join("empty.xlsx");
        let output_dir = dir.join("out");
        std::fs::create_dir_all(&output_dir).unwrap();
        write_text(&input, "source");
        let section = SectionRange {
            start: 0,
            end: 1,
            kind: "fc_sub_account",
            header_cells: row(&["", ""]),
        };
        let csv_path = section_csv_path(&input, &output_dir, section.start, section.kind);

        let output =
            write_section_output(&input, &output_dir, &section, &[row(&["", ""])]).unwrap();

        assert!(output.is_none());
        assert!(!csv_path.exists());
    }
}
