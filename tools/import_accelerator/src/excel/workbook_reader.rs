use anyhow::{anyhow, Context, Result};
use calamine::{open_workbook_auto, Data, Range, Reader};
use std::path::Path;

pub(crate) fn read_first_sheet_range(input: &Path) -> Result<Range<Data>> {
    let mut workbook =
        open_workbook_auto(input).with_context(|| format!("open workbook {}", input.display()))?;
    let sheet_names = workbook.sheet_names();
    let sheet_name = first_sheet_name(input, &sheet_names)?;
    let range = workbook.worksheet_range(&sheet_name)?;
    Ok(range)
}

fn first_sheet_name(input: &Path, sheet_names: &[String]) -> Result<String> {
    sheet_names
        .first()
        .cloned()
        .ok_or_else(|| anyhow!("workbook has no sheets: {}", input.display()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn first_sheet_name_selects_first_sheet() {
        let names = vec!["交易SheetA".to_string(), "交易SheetB".to_string()];

        assert_eq!(
            first_sheet_name(Path::new("多sheet.xlsx"), &names).unwrap(),
            "交易SheetA"
        );
    }

    #[test]
    fn first_sheet_name_preserves_empty_workbook_error_message() {
        let err = first_sheet_name(Path::new("空工作簿.xlsx"), &[]).unwrap_err();

        assert_eq!(err.to_string(), "workbook has no sheets: 空工作簿.xlsx");
    }
}
