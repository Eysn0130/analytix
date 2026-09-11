use crate::hash::md5_lower_10;
use std::fs::Metadata;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

pub(crate) fn section_csv_path(
    input: &Path,
    output_dir: &Path,
    start: usize,
    kind: &str,
) -> PathBuf {
    let tag = md5_lower_10(&format!("{}|{}|{}", input.display(), start, kind));
    let stem = input
        .file_stem()
        .and_then(|value| value.to_str())
        .unwrap_or("workbook");
    output_dir.join(format!("{stem}_{kind}_{tag}.csv"))
}

pub(crate) fn should_write_section_csv(csv_path: &Path, input: &Path) -> bool {
    match (csv_path.metadata(), input.metadata()) {
        (Ok(csv_meta), Ok(input_meta)) => should_write_for_metadata(&csv_meta, &input_meta),
        _ => true,
    }
}

fn should_write_for_metadata(csv_meta: &Metadata, input_meta: &Metadata) -> bool {
    should_write_for_modified(csv_meta.modified().ok(), input_meta.modified().ok())
}

fn should_write_for_modified(
    csv_modified: Option<SystemTime>,
    input_modified: Option<SystemTime>,
) -> bool {
    match (csv_modified, input_modified) {
        (Some(csv_modified), Some(input_modified)) => csv_modified < input_modified,
        _ => true,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    #[test]
    fn section_csv_path_keeps_unicode_stem_kind_and_stable_tag() {
        let input = PathBuf::from("账户子账户混合.xlsx");
        let output_dir = PathBuf::from("out");
        let path = section_csv_path(&input, &output_dir, 3, "fc_account");
        let tag = md5_lower_10("账户子账户混合.xlsx|3|fc_account");

        assert_eq!(
            path,
            output_dir.join(format!("账户子账户混合_fc_account_{tag}.csv"))
        );
    }

    #[test]
    fn section_csv_path_falls_back_to_workbook_when_stem_is_missing() {
        let input = PathBuf::new();
        let output_dir = PathBuf::from("out");
        let path = section_csv_path(&input, &output_dir, 0, "fc_sub_account");
        let tag = md5_lower_10("|0|fc_sub_account");

        assert_eq!(
            path,
            output_dir.join(format!("workbook_fc_sub_account_{tag}.csv"))
        );
    }

    #[test]
    fn should_write_for_modified_only_reuses_new_enough_csv() {
        let earlier = SystemTime::UNIX_EPOCH + Duration::from_secs(10);
        let later = SystemTime::UNIX_EPOCH + Duration::from_secs(20);

        assert!(should_write_for_modified(Some(earlier), Some(later)));
        assert!(!should_write_for_modified(Some(later), Some(earlier)));
        assert!(!should_write_for_modified(Some(later), Some(later)));
    }

    #[test]
    fn should_write_for_modified_rewrites_when_timestamp_is_unavailable() {
        let timestamp = SystemTime::UNIX_EPOCH + Duration::from_secs(10);

        assert!(should_write_for_modified(None, Some(timestamp)));
        assert!(should_write_for_modified(Some(timestamp), None));
        assert!(should_write_for_modified(None, None));
    }
}
