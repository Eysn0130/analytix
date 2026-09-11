pub(crate) fn sanitize_csv_header(value: &str) -> String {
    value
        .replace('\u{feff}', "")
        .replace('\t', "")
        .trim()
        .to_string()
}

pub(crate) fn preview_cell_text(value: &str) -> String {
    let text = value.replace('\u{feff}', "").trim().to_string();
    let lower = text.to_lowercase();
    if text.is_empty() || lower == "nan" || lower == "none" || lower == "null" {
        String::new()
    } else {
        text
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sanitize_csv_header_removes_bom_tabs_and_trims_space() {
        assert_eq!(sanitize_csv_header("\u{feff}交易时间\t "), "交易时间");
    }

    #[test]
    fn preview_cell_text_removes_bom_and_trims_space() {
        assert_eq!(preview_cell_text(" \u{feff} 工资入账 "), "工资入账");
    }

    #[test]
    fn preview_cell_text_treats_empty_nan_none_and_null_as_blank() {
        for value in ["", "   ", "nan", "NaN", "none", "None", "null", "NULL"] {
            assert_eq!(preview_cell_text(value), "", "{value}");
        }
    }

    #[test]
    fn preview_cell_text_preserves_inner_tabs_for_data_cells() {
        assert_eq!(preview_cell_text(" A\tB "), "A\tB");
    }
}
