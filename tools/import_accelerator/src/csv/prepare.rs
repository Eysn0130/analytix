use super::prepare_adapter::{prepare_csv_payload, PrepareCsvMode};
use super::prepare_encoding::resolve_prepare_csv_encoding;
use anyhow::Result;
use std::path::PathBuf;

pub(crate) fn prepare_csv(
    path: &PathBuf,
    limit: usize,
    output: Option<&PathBuf>,
    encoding_override: Option<&str>,
) -> Result<serde_json::Value> {
    prepare_csv_with_output_policy(path, limit, output, encoding_override, false)
}

pub(crate) fn prepare_csv_with_output_policy(
    path: &PathBuf,
    limit: usize,
    output: Option<&PathBuf>,
    encoding_override: Option<&str>,
    clean_if_needed: bool,
) -> Result<serde_json::Value> {
    let encoding = resolve_prepare_csv_encoding(path, encoding_override)?;
    let mode = match output {
        Some(output) if clean_if_needed => PrepareCsvMode::CleanToOutputIfNeeded { output },
        Some(output) => PrepareCsvMode::CleanToOutput { output },
        None => PrepareCsvMode::PreviewOnly,
    };
    prepare_csv_payload(path, encoding, limit, mode)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{string_array, string_rows, write_gb18030, write_text, TestDir};
    use std::fs;

    #[test]
    fn prepare_csv_preview_only_reports_encoding_counts_and_preview() {
        let dir = TestDir::new("preview-only");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "\u{feff}交易时间\t,,交易金额,nan,摘要说明\n 2026-01-01 10:00:00\t,ignored, 100.00 ,ignored, 工资入账 \n,ignored,,ignored,\n2026-01-02 11:00:00,ignored,80.50,ignored,\n",
        );

        let payload = prepare_csv(&csv_path, 5, None, None).unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["encoding"], "utf-8-sig");
        assert_eq!(payload["rows_total"], 3);
        assert_eq!(payload["columns_total"], 3);
        assert_eq!(
            string_array(&payload, "header_preview"),
            vec!["交易时间", "交易金额", "摘要说明"]
        );
        assert_eq!(
            string_rows(&payload, "sample_rows"),
            vec![
                vec!["2026-01-01 10:00:00", "100.00", "工资入账"],
                vec!["2026-01-02 11:00:00", "80.50", ""],
            ]
        );
        assert!(payload["preclean_rows"].is_null());
        assert!(payload["output"].is_null());
    }

    #[test]
    fn prepare_csv_with_output_cleans_cells_and_preserves_preview_semantics() {
        let dir = TestDir::new("with-output");
        let csv_path = dir.join("sample_gb18030.csv");
        let output_path = dir.join("clean.csv");
        write_gb18030(
            &csv_path,
            "交易时间,交易金额,摘要说明\n 2026-01-01 10:00:00\t, 100.00 , 工资入账 \n2026-01-02 11:00:00,80.50,转账\n",
        );

        let payload = prepare_csv(&csv_path, 5, Some(&output_path), Some("gb18030")).unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["encoding"], "gb18030");
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(payload["preclean_rows"], 2);
        assert_eq!(
            string_rows(&payload, "sample_rows"),
            vec![
                vec!["2026-01-01 10:00:00", "100.00", "工资入账"],
                vec!["2026-01-02 11:00:00", "80.50", "转账"],
            ]
        );
        assert_eq!(
            fs::read_to_string(&output_path).unwrap(),
            "交易时间,交易金额,摘要说明\n2026-01-01 10:00:00,100.00,工资入账\n2026-01-02 11:00:00,80.50,转账\n"
        );
    }

    #[test]
    fn prepare_csv_preview_edge_row_counts_match_physical_lines() {
        let dir = TestDir::new("edge-row-counts");
        let cases = [
            (
                "empty.csv",
                "",
                0,
                0,
                Vec::<&str>::new(),
                Vec::<Vec<&str>>::new(),
            ),
            (
                "header_only_no_newline.csv",
                "a,b",
                0,
                2,
                vec!["a", "b"],
                vec![],
            ),
            (
                "header_only_crlf.csv",
                "a,b\r\n",
                0,
                2,
                vec!["a", "b"],
                vec![],
            ),
            (
                "no_tail_newline.csv",
                "a,b\n1,2",
                1,
                2,
                vec!["a", "b"],
                vec![vec!["1", "2"]],
            ),
            (
                "crlf.csv",
                "a,b\r\n1,2\r\n3,4\r\n",
                2,
                2,
                vec!["a", "b"],
                vec![vec!["1", "2"], vec!["3", "4"]],
            ),
        ];

        for (name, content, rows_total, columns_total, headers, sample_rows) in cases {
            let csv_path = dir.join(name);
            write_text(&csv_path, content);
            let payload = prepare_csv(&csv_path, 5, None, Some("utf-8-sig")).unwrap();
            let expected_headers = headers
                .into_iter()
                .map(|value| value.to_string())
                .collect::<Vec<_>>();
            let expected_rows = sample_rows
                .into_iter()
                .map(|row| {
                    row.into_iter()
                        .map(|value| value.to_string())
                        .collect::<Vec<_>>()
                })
                .collect::<Vec<_>>();

            assert_eq!(payload["rows_total"], rows_total, "{name}");
            assert_eq!(payload["columns_total"], columns_total, "{name}");
            assert_eq!(
                string_array(&payload, "header_preview"),
                expected_headers,
                "{name}"
            );
            assert_eq!(
                string_rows(&payload, "sample_rows"),
                expected_rows,
                "{name}"
            );
        }
    }

    #[test]
    fn prepare_csv_preview_ignores_late_malformed_csv_after_limit() {
        let dir = TestDir::new("late-malformed");
        let csv_path = dir.join("late_malformed.csv");
        write_text(&csv_path, "a,b\n1,2\n\"unterminated\n");

        let payload = prepare_csv(&csv_path, 1, None, None).unwrap();

        assert_eq!(payload["rows_total"], 2);
        assert_eq!(payload["columns_total"], 2);
        assert_eq!(string_array(&payload, "header_preview"), vec!["a", "b"]);
        assert_eq!(string_rows(&payload, "sample_rows"), vec![vec!["1", "2"]]);
    }

    #[test]
    fn prepare_csv_with_output_rejects_late_malformed_csv_after_limit() {
        let dir = TestDir::new("late-malformed-output");
        let csv_path = dir.join("late_malformed.csv");
        let output_path = dir.join("clean.csv");
        write_text(&csv_path, "a,b\n1,2\n\"unterminated\n");

        let result = prepare_csv(&csv_path, 1, Some(&output_path), None);

        assert!(result.is_err());
    }

    #[test]
    fn prepare_csv_with_clean_if_needed_uses_original_clean_utf8() {
        let dir = TestDir::new("with-output-if-needed-clean");
        let csv_path = dir.join("sample.csv");
        let output_path = dir.join("clean.csv");
        write_text(&csv_path, "交易时间,交易金额\n2026-01-01,100\n");

        let payload =
            prepare_csv_with_output_policy(&csv_path, 5, Some(&output_path), Some("utf-8"), true)
                .unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["rows_total"], 1);
        assert_eq!(payload["preclean_rows"], 1);
        assert!(payload["output"].is_null());
        assert!(!output_path.exists());
    }
}
