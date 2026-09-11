use super::ranges::SectionRange;

pub(crate) fn collect_section_data_rows(
    rows: &[Vec<String>],
    section: &SectionRange,
) -> Vec<Vec<String>> {
    rows.iter()
        .take(section.end)
        .skip(section.start + 1)
        .filter(|row| is_section_data_row(row))
        .cloned()
        .collect()
}

fn is_section_data_row(row: &[String]) -> bool {
    row.iter().filter(|cell| !cell.is_empty()).count() >= 2
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    fn section(start: usize, end: usize) -> SectionRange {
        SectionRange {
            start,
            end,
            kind: "fc_account",
            header_cells: row(&["账户开户名称", "开户人证件号码"]),
        }
    }

    #[test]
    fn collect_section_data_rows_uses_rows_after_header_until_section_end() {
        let rows = vec![
            row(&["导出批次", "测试"]),
            row(&["账户开户名称", "开户人证件号码"]),
            row(&["张三", "110101"]),
            row(&["说明：以下为关联子账户"]),
            row(&["银行名称", "开户账号", "子账户账号"]),
            row(&["招商银行", "A-001", "SUB-001"]),
        ];

        assert_eq!(
            collect_section_data_rows(&rows, &section(1, 4)),
            vec![row(&["张三", "110101"])]
        );
    }

    #[test]
    fn collect_section_data_rows_skips_empty_and_single_cell_note_rows() {
        let rows = vec![
            row(&["账户开户名称", "开户人证件号码"]),
            row(&["说明：以下为关联子账户"]),
            row(&["", "", ""]),
            row(&["李四", "", "220202"]),
        ];

        assert_eq!(
            collect_section_data_rows(&rows, &section(0, rows.len())),
            vec![row(&["李四", "", "220202"])]
        );
    }

    #[test]
    fn collect_section_data_rows_returns_empty_when_range_has_no_data_rows() {
        let rows = vec![
            row(&["账户开户名称", "开户人证件号码"]),
            row(&["说明：无数据"]),
        ];

        assert!(collect_section_data_rows(&rows, &section(0, rows.len())).is_empty());
        assert!(collect_section_data_rows(&rows, &section(0, 1)).is_empty());
    }
}
