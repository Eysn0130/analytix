use crate::excel::excel_cell_to_pandas_text;
use calamine::Data;

pub(crate) fn excel_cell_to_section_text(cell: &Data) -> String {
    let text = excel_cell_to_pandas_text(cell)
        .replace('\u{feff}', "")
        .replace('\t', "")
        .trim()
        .to_string();
    let lower = text.to_lowercase();
    if text.is_empty() || lower == "nan" || lower == "none" || lower == "null" {
        String::new()
    } else {
        text
    }
}

pub(crate) fn trim_width(header: &[String], rows: &[Vec<String>]) -> usize {
    let mut width = last_non_empty_index(header);
    for row in rows {
        width = width.max(last_non_empty_index(row));
    }
    width
}

pub(crate) fn pad_row(row: &[String], width: usize) -> Vec<String> {
    let mut out = row.iter().take(width).cloned().collect::<Vec<_>>();
    out.resize(width, String::new());
    out
}

fn last_non_empty_index(row: &[String]) -> usize {
    row.iter()
        .rposition(|cell| !cell.is_empty())
        .map(|idx| idx + 1)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn excel_cell_to_section_text_removes_bom_tabs_and_trims_blank_values() {
        assert_eq!(
            excel_cell_to_section_text(&Data::String("\u{feff} 张\t三 ".to_string())),
            "张三"
        );
        for value in ["", "  ", "nan", "None", "NULL"] {
            assert_eq!(
                excel_cell_to_section_text(&Data::String(value.to_string())),
                "",
                "{value}"
            );
        }
    }

    #[test]
    fn trim_width_uses_last_non_empty_cell_across_header_and_rows() {
        assert_eq!(
            trim_width(
                &row(&["账户开户名称", "开户人证件号码", "", ""]),
                &[row(&["张三", "110101", "62220001", ""])]
            ),
            3
        );
        assert_eq!(trim_width(&row(&["", ""]), &[row(&["", ""])]), 0);
    }

    #[test]
    fn pad_row_truncates_or_extends_to_width() {
        assert_eq!(pad_row(&row(&["a", "b", "c"]), 2), row(&["a", "b"]));
        assert_eq!(pad_row(&row(&["a"]), 3), row(&["a", "", ""]));
    }
}
