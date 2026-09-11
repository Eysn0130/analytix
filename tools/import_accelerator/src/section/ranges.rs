use super::schema::detect_fc_kind;

#[derive(Debug, PartialEq, Eq)]
pub(crate) struct SectionRange {
    pub(crate) start: usize,
    pub(crate) end: usize,
    pub(crate) kind: &'static str,
    pub(crate) header_cells: Vec<String>,
}

pub(crate) fn find_section_ranges(rows: &[Vec<String>]) -> Vec<SectionRange> {
    let headers = collect_section_headers(rows);
    headers
        .iter()
        .enumerate()
        .map(|(idx, header)| SectionRange {
            start: header.start,
            end: headers
                .get(idx + 1)
                .map(|next| next.start)
                .unwrap_or(rows.len()),
            kind: header.kind,
            header_cells: header.header_cells.clone(),
        })
        .collect()
}

#[derive(Debug)]
struct SectionHeader {
    start: usize,
    kind: &'static str,
    header_cells: Vec<String>,
}

fn collect_section_headers(rows: &[Vec<String>]) -> Vec<SectionHeader> {
    let mut headers = Vec::new();
    for (idx, row) in rows.iter().enumerate() {
        let non_empty = row
            .iter()
            .filter(|cell| !cell.is_empty())
            .cloned()
            .collect::<Vec<_>>();
        if non_empty.len() < 2 {
            continue;
        }
        if let Some(kind) = detect_fc_kind(&non_empty) {
            if kind == "fc_account" || kind == "fc_sub_account" {
                headers.push(SectionHeader {
                    start: idx,
                    kind,
                    header_cells: row.clone(),
                });
            }
        }
    }
    headers
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn find_section_ranges_detects_multiple_sections() {
        let rows = vec![
            row(&["导出批次", "测试"]),
            row(&["账户开户名称", "开户人证件号码", "交易卡号", "交易账号"]),
            row(&["张三", "110101", "62220001", "A-001"]),
            row(&["银行名称", "开户账号", "子账户账号", "余额"]),
            row(&["招商银行", "A-001", "SUB-001", "10"]),
        ];

        let ranges = find_section_ranges(&rows);

        assert_eq!(ranges.len(), 2);
        assert_eq!(ranges[0].start, 1);
        assert_eq!(ranges[0].end, 3);
        assert_eq!(ranges[0].kind, "fc_account");
        assert_eq!(ranges[1].start, 3);
        assert_eq!(ranges[1].end, rows.len());
        assert_eq!(ranges[1].kind, "fc_sub_account");
    }

    #[test]
    fn find_section_ranges_supports_single_section_to_end() {
        let rows = vec![
            row(&["开户银行", "账卡号", "子账户号", "余额"]),
            row(&["招商银行", "A-001", "SUB-001", "10"]),
        ];

        let ranges = find_section_ranges(&rows);

        assert_eq!(
            ranges,
            vec![SectionRange {
                start: 0,
                end: 2,
                kind: "fc_sub_account",
                header_cells: row(&["开户银行", "账卡号", "子账户号", "余额"]),
            }]
        );
    }

    #[test]
    fn find_section_ranges_returns_empty_without_section_headers() {
        let rows = vec![
            row(&["交易时间", "交易金额", "交易账号"]),
            row(&["2026-01-01", "100", "A-001"]),
        ];

        assert!(find_section_ranges(&rows).is_empty());
    }

    #[test]
    fn find_section_ranges_ignores_supported_schema_kinds_that_are_not_account_sections() {
        let rows = vec![row(&["交易卡号", "交易账号", "交易时间", "交易金额"])];

        assert!(find_section_ranges(&rows).is_empty());
    }
}
