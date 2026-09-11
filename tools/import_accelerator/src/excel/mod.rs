mod csv_writer;
mod pandas_compat;
mod workbook_reader;

pub(crate) use pandas_compat::excel_cell_to_pandas_text;
pub(crate) use workbook_reader::read_first_sheet_range;

use anyhow::Result;
use csv_writer::write_excel_range_csv;
use std::path::PathBuf;

pub(crate) fn excel_to_csv(input: &PathBuf, output: &PathBuf) -> Result<u64> {
    let range = read_first_sheet_range(input)?;
    write_excel_range_csv(&range, output)
}
