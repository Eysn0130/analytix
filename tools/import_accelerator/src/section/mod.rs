mod data_rows;
mod output_path;
mod ranges;
mod row_normalize;
mod schema;
mod workbook_reader;
mod writer;

use anyhow::Result;
use data_rows::collect_section_data_rows;
use ranges::find_section_ranges;
use std::path::PathBuf;
use workbook_reader::read_first_sheet_section_rows;
use writer::write_section_output;

pub(crate) fn split_account_sections(
    input: &PathBuf,
    output_dir: &PathBuf,
) -> Result<Vec<serde_json::Value>> {
    std::fs::create_dir_all(output_dir)?;
    let rows = read_first_sheet_section_rows(input)?;

    let mut outputs = Vec::new();
    for section in find_section_ranges(&rows) {
        let data_rows = collect_section_data_rows(&rows, &section);

        if let Some(output) = write_section_output(input, output_dir, &section, &data_rows)? {
            outputs.push(output);
        }
    }

    Ok(outputs)
}
