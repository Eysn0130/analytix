use calamine::Data;
use std::collections::HashSet;

pub(crate) fn excel_cell_to_pandas_text(cell: &Data) -> String {
    match cell {
        Data::Empty => String::new(),
        Data::String(value) => value.to_string(),
        Data::Float(value) => {
            if value.is_finite() && value.fract() == 0.0 {
                format!("{value:.0}")
            } else {
                value.to_string()
            }
        }
        Data::Int(value) => value.to_string(),
        Data::Bool(value) => {
            if *value {
                "True".to_string()
            } else {
                "False".to_string()
            }
        }
        Data::DateTime(value) => value
            .as_datetime()
            .map(|dt| dt.to_string())
            .unwrap_or_else(|| value.to_string()),
        Data::DateTimeIso(value) => value.to_string(),
        Data::DurationIso(value) => value.to_string(),
        Data::Error(value) => value.to_string(),
    }
}

pub(crate) fn pandas_header_names(raw_headers: Vec<String>) -> Vec<String> {
    let mut seen = HashSet::new();
    raw_headers
        .into_iter()
        .enumerate()
        .map(|(idx, raw)| {
            let base = if raw.is_empty() {
                format!("Unnamed: {idx}")
            } else {
                raw
            };
            if seen.insert(base.clone()) {
                return base;
            }
            let mut suffix = 1;
            loop {
                let candidate = format!("{base}.{suffix}");
                if seen.insert(candidate.clone()) {
                    return candidate;
                }
                suffix += 1;
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use calamine::{ExcelDateTime, ExcelDateTimeType};

    #[test]
    fn excel_cell_to_pandas_text_formats_scalar_values() {
        assert_eq!(excel_cell_to_pandas_text(&Data::Empty), "");
        assert_eq!(
            excel_cell_to_pandas_text(&Data::String("工资入账".to_string())),
            "工资入账"
        );
        assert_eq!(excel_cell_to_pandas_text(&Data::Int(100)), "100");
        assert_eq!(excel_cell_to_pandas_text(&Data::Float(100.0)), "100");
        assert_eq!(excel_cell_to_pandas_text(&Data::Float(80.5)), "80.5");
        assert_eq!(excel_cell_to_pandas_text(&Data::Bool(true)), "True");
        assert_eq!(excel_cell_to_pandas_text(&Data::Bool(false)), "False");
    }

    #[test]
    fn excel_cell_to_pandas_text_formats_date_values() {
        let ten_am = 10.0 / 24.0;
        let datetime = Data::DateTime(ExcelDateTime::new(
            46023.0 + ten_am,
            ExcelDateTimeType::DateTime,
            false,
        ));

        assert_eq!(excel_cell_to_pandas_text(&datetime), "2026-01-01 10:00:00");
        assert_eq!(
            excel_cell_to_pandas_text(&Data::DateTimeIso("2026-01-01T10:00:00".to_string())),
            "2026-01-01T10:00:00"
        );
        assert_eq!(
            excel_cell_to_pandas_text(&Data::DurationIso("PT1H".to_string())),
            "PT1H"
        );
    }

    #[test]
    fn pandas_header_names_matches_empty_and_duplicate_header_semantics() {
        assert_eq!(
            pandas_header_names(vec![
                "交易时间".to_string(),
                String::new(),
                "交易金额".to_string(),
                "交易金额".to_string(),
                String::new(),
            ]),
            vec![
                "交易时间".to_string(),
                "Unnamed: 1".to_string(),
                "交易金额".to_string(),
                "交易金额.1".to_string(),
                "Unnamed: 4".to_string(),
            ]
        );
    }
}
