use super::preview::CsvPreview;
use serde_json::json;
use std::path::PathBuf;

#[derive(Debug)]
pub(crate) struct PreparedCsvPayload {
    pub(crate) encoding: String,
    pub(crate) preview: CsvPreview,
    pub(crate) preclean_rows: Option<u64>,
    pub(crate) output: Option<PathBuf>,
}

impl PreparedCsvPayload {
    pub(crate) fn into_json(self) -> serde_json::Value {
        let columns_total = self.preview.header_preview.len();
        let CsvPreview {
            rows_total,
            header_preview,
            sample_rows,
        } = self.preview;

        json!({
            "ok": true,
            "encoding": self.encoding,
            "rows_total": rows_total,
            "columns_total": columns_total,
            "header_preview": header_preview,
            "sample_rows": sample_rows,
            "preclean_rows": self.preclean_rows,
            "output": self.output,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn prepared_csv_payload_json_shape_is_stable() {
        let payload = PreparedCsvPayload {
            encoding: "gb18030".to_string(),
            preview: CsvPreview {
                rows_total: 2,
                header_preview: vec!["交易时间".to_string(), "交易金额".to_string()],
                sample_rows: vec![vec!["2026-01-01".to_string(), "100".to_string()]],
            },
            preclean_rows: Some(2),
            output: Some(PathBuf::from("clean.csv")),
        }
        .into_json();

        assert_eq!(
            payload,
            json!({
                "ok": true,
                "encoding": "gb18030",
                "rows_total": 2,
                "columns_total": 2,
                "header_preview": ["交易时间", "交易金额"],
                "sample_rows": [["2026-01-01", "100"]],
                "preclean_rows": 2,
                "output": "clean.csv",
            })
        );
    }
}
