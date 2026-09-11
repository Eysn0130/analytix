use super::row_normalize::excel_cell_to_section_text;
use crate::excel::read_first_sheet_range;
use anyhow::Result;
use calamine::{Data, Range};
use std::path::Path;

pub(crate) fn read_first_sheet_section_rows(input: &Path) -> Result<Vec<Vec<String>>> {
    let range = read_first_sheet_range(input)?;
    Ok(section_rows_from_range(&range))
}

fn section_rows_from_range(range: &Range<Data>) -> Vec<Vec<String>> {
    range
        .rows()
        .map(|row| {
            row.iter()
                .map(excel_cell_to_section_text)
                .collect::<Vec<String>>()
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use calamine::Cell;

    fn row(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn section_rows_from_range_preserves_sparse_width_and_normalizes_cells() {
        let range = Range::from_sparse(vec![
            Cell::new((0, 0), Data::String("\u{feff}账户开户名称\t".to_string())),
            Cell::new((0, 2), Data::String("开户人证件号码".to_string())),
            Cell::new((1, 0), Data::String("张三".to_string())),
            Cell::new((1, 1), Data::String("None".to_string())),
            Cell::new((1, 2), Data::Int(110101)),
        ]);

        assert_eq!(
            section_rows_from_range(&range),
            vec![
                row(&["账户开户名称", "", "开户人证件号码"]),
                row(&["张三", "", "110101"]),
            ]
        );
    }

    #[test]
    fn section_rows_from_range_returns_empty_rows_for_empty_range() {
        let range = Range::<Data>::empty();

        assert!(section_rows_from_range(&range).is_empty());
    }
}
