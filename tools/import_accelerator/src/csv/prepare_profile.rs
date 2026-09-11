use super::encoding::counted_decoded_file_reader;
use super::prepare_model::PreparedCsvPayload;
use super::preview::{read_csv_header, CsvPreview};
use super::profile_columns::ColumnProfiles;
use anyhow::Result;
use csv::{ReaderBuilder, StringRecord};
use serde_json::Value;
use std::io;
use std::path::PathBuf;

pub(crate) fn prepare_csv_with_profile(
    path: &PathBuf,
    encoding: String,
    limit: usize,
) -> Result<Value> {
    let (decoded, line_counter) = counted_decoded_file_reader(path, &encoding)?;
    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(decoded);
    let mut record = StringRecord::new();
    let header = read_csv_header(&mut reader, &mut record)?;
    let mut profiles = ColumnProfiles::from_header(&header);
    let mut sample_rows: Vec<Vec<String>> = Vec::new();
    let mut row_no = 0_u64;
    let mut profile_ok = true;

    loop {
        record.clear();
        match reader.read_record(&mut record) {
            Ok(false) => break,
            Ok(true) => {
                row_no += 1;
                if sample_rows.len() < limit {
                    if let Some(row) = header.preview_columns.sample_row(&record) {
                        sample_rows.push(row);
                    }
                }
                profiles.observe_record(row_no, &record);
            }
            Err(err)
                if limit == 0
                    || sample_rows.len() >= limit
                    || header.preview_columns.is_empty() =>
            {
                let _ = err;
                profile_ok = false;
                break;
            }
            Err(err) => return Err(err.into()),
        }
    }

    let mut decoded = reader.into_inner();
    io::copy(&mut decoded, &mut io::sink())?;
    let rows_total = line_counter.borrow().rows_total();

    let preview = CsvPreview {
        rows_total,
        header_preview: header.preview_columns.header_preview,
        sample_rows,
    };
    let mut payload = PreparedCsvPayload {
        encoding: encoding.clone(),
        preview,
        preclean_rows: None,
        output: None,
    }
    .into_json();
    if let Value::Object(ref mut object) = payload {
        let profile_payload = if profile_ok {
            profiles.into_json(rows_total, &encoding)
        } else {
            Value::Null
        };
        object.insert("column_profiles".to_string(), profile_payload);
    }
    Ok(payload)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::profile_columns::profile_csv_columns;
    use crate::csv::test_support::{string_rows, write_text, TestDir};

    #[test]
    fn prepare_csv_with_profile_matches_profile_columns_in_one_scan() {
        let dir = TestDir::new("prepare-with-profile");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "交易日期,交易金额,交易账号\n\
20260102,-123.45,6222000000000000001\n\
2026-01-03,88.00,6222000000000000002\n",
        );

        let payload = prepare_csv_with_profile(&csv_path, "utf-8-sig".to_string(), 1).unwrap();
        let profile = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(
            string_rows(&payload, "sample_rows"),
            vec![vec!["20260102", "-123.45", "6222000000000000001"]]
        );
        assert_eq!(payload["column_profiles"], profile);
    }

    #[test]
    fn prepare_csv_with_profile_profiles_late_quoted_rows_after_preview_limit() {
        let dir = TestDir::new("prepare-with-profile-late-quoted-row");
        let csv_path = dir.join("sample.csv");
        write_text(&csv_path, "a,b\n1,2\n\"late,malformed\n");

        let payload = prepare_csv_with_profile(&csv_path, "utf-8-sig".to_string(), 1).unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(string_rows(&payload, "sample_rows"), vec![vec!["1", "2"]]);
        assert_eq!(payload["column_profiles"]["rows_total"], 2);
    }
}
