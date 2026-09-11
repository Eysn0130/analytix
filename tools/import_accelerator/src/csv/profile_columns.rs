use super::encoding::counted_decoded_file_reader;
use super::preview::read_csv_header;
use super::preview_normalize::preview_cell_text;
use anyhow::Result;
use csv::{ReaderBuilder, StringRecord};
use serde_json::{json, Value};
use std::collections::HashSet;
use std::io;
use std::path::PathBuf;

const SAMPLE_LIMIT: usize = 5;
const DISTINCT_LIMIT: usize = 4096;

pub(crate) fn profile_csv_columns(path: &PathBuf, encoding: &str) -> Result<Value> {
    let (decoded, line_counter) = counted_decoded_file_reader(path, encoding)?;
    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(decoded);
    let mut record = StringRecord::new();
    let header = read_csv_header(&mut reader, &mut record)?;
    let mut profiles = ColumnProfiles::from_header(&header);
    let mut row_no = 0_u64;
    loop {
        record.clear();
        if !reader.read_record(&mut record)? {
            break;
        }
        row_no += 1;
        profiles.observe_record(row_no, &record);
    }

    let mut decoded = reader.into_inner();
    io::copy(&mut decoded, &mut io::sink())?;
    let rows_total = line_counter.borrow().rows_total();

    Ok(profiles.into_json(rows_total, encoding))
}

pub(super) struct ColumnProfiles {
    profiles: Vec<ColumnProfile>,
}

impl ColumnProfiles {
    pub(super) fn from_header(header: &super::preview::CsvHeader) -> Self {
        let profiles = header
            .preview_columns
            .indexes()
            .iter()
            .zip(header.preview_columns.header_preview.iter())
            .enumerate()
            .map(|(profile_index, (source_index, title))| {
                ColumnProfile::new(profile_index, *source_index, title.clone())
            })
            .collect();
        Self { profiles }
    }

    pub(super) fn observe_record(&mut self, row_no: u64, record: &StringRecord) {
        for profile in &mut self.profiles {
            let value = preview_cell_text(record.get(profile.source_index).unwrap_or(""));
            profile.observe(row_no, &value);
        }
    }

    pub(super) fn into_json(self, rows_total: u64, encoding: &str) -> Value {
        let column_payloads = self
            .profiles
            .into_iter()
            .map(|profile| profile.into_json(rows_total))
            .collect::<Vec<_>>();

        json!({
            "ok": true,
            "encoding": encoding,
            "rows_total": rows_total,
            "columns_total": column_payloads.len(),
            "columns": column_payloads,
        })
    }
}

struct ColumnProfile {
    profile_index: usize,
    source_index: usize,
    header: String,
    non_empty: u64,
    amount_like: u64,
    signed_amount: u64,
    positive_amount: u64,
    negative_amount: u64,
    date_like: u64,
    datetime_like: u64,
    account_like: u64,
    id_no_like: u64,
    phone_like: u64,
    ip_like: u64,
    mac_like: u64,
    first_non_empty: Vec<String>,
    fixed_seed_samples: Vec<RankedSample>,
    longest_sample: Option<String>,
    min_amount_sample: Option<(f64, String)>,
    max_amount_sample: Option<(f64, String)>,
    date_sample: Option<String>,
    distinct_values: HashSet<String>,
    distinct_limit_exceeded: bool,
}

impl ColumnProfile {
    fn new(profile_index: usize, source_index: usize, header: String) -> Self {
        Self {
            profile_index,
            source_index,
            header,
            non_empty: 0,
            amount_like: 0,
            signed_amount: 0,
            positive_amount: 0,
            negative_amount: 0,
            date_like: 0,
            datetime_like: 0,
            account_like: 0,
            id_no_like: 0,
            phone_like: 0,
            ip_like: 0,
            mac_like: 0,
            first_non_empty: Vec::new(),
            fixed_seed_samples: Vec::new(),
            longest_sample: None,
            min_amount_sample: None,
            max_amount_sample: None,
            date_sample: None,
            distinct_values: HashSet::new(),
            distinct_limit_exceeded: false,
        }
    }

    fn observe(&mut self, row_no: u64, raw_value: &str) {
        let value = raw_value.trim();
        if value.is_empty() {
            return;
        }
        self.non_empty += 1;
        if self.first_non_empty.len() < SAMPLE_LIMIT {
            self.first_non_empty.push(value.to_string());
        }
        self.observe_fixed_seed_sample(row_no, value);
        self.observe_distinct(value);
        self.observe_longest(value);

        let date_kind = detect_date_kind(value);
        let account_like = looks_like_account(value);
        let id_no_like = looks_like_id_no(value);
        let phone_like = looks_like_phone(value);
        let ip_like = looks_like_ip(value);
        let mac_like = looks_like_mac(value);

        if let Some(parsed) = parse_amount(value).filter(|_| {
            should_profile_as_amount(value, &date_kind, account_like, id_no_like, phone_like)
        }) {
            self.amount_like += 1;
            if parsed.signed {
                self.signed_amount += 1;
            }
            if parsed.value > 0.0 {
                self.positive_amount += 1;
            } else if parsed.value < 0.0 {
                self.negative_amount += 1;
            }
            self.observe_amount_extremes(parsed.value, value);
        }
        if date_kind.date_like {
            self.date_like += 1;
            if self.date_sample.is_none() {
                self.date_sample = Some(value.to_string());
            }
        }
        if date_kind.datetime_like {
            self.datetime_like += 1;
        }
        if account_like {
            self.account_like += 1;
        }
        if id_no_like {
            self.id_no_like += 1;
        }
        if phone_like {
            self.phone_like += 1;
        }
        if ip_like {
            self.ip_like += 1;
        }
        if mac_like {
            self.mac_like += 1;
        }
    }

    fn observe_fixed_seed_sample(&mut self, row_no: u64, value: &str) {
        let rank = stable_sample_rank(self.profile_index, row_no, value);
        let sample = RankedSample {
            rank,
            row_no,
            value: value.to_string(),
        };
        if self.fixed_seed_samples.len() < SAMPLE_LIMIT {
            self.fixed_seed_samples.push(sample);
            return;
        }
        let Some((replace_idx, worst)) = self
            .fixed_seed_samples
            .iter()
            .enumerate()
            .max_by_key(|(_, item)| item.rank)
        else {
            return;
        };
        if rank < worst.rank {
            self.fixed_seed_samples[replace_idx] = sample;
        }
    }

    fn observe_distinct(&mut self, value: &str) {
        if self.distinct_limit_exceeded {
            return;
        }
        self.distinct_values.insert(value.to_string());
        if self.distinct_values.len() > DISTINCT_LIMIT {
            self.distinct_values.clear();
            self.distinct_limit_exceeded = true;
        }
    }

    fn observe_longest(&mut self, value: &str) {
        let should_replace = self
            .longest_sample
            .as_ref()
            .map(|existing| value.chars().count() > existing.chars().count())
            .unwrap_or(true);
        if should_replace {
            self.longest_sample = Some(value.to_string());
        }
    }

    fn observe_amount_extremes(&mut self, value: f64, raw: &str) {
        if self
            .min_amount_sample
            .as_ref()
            .map(|(current, _)| value < *current)
            .unwrap_or(true)
        {
            self.min_amount_sample = Some((value, raw.to_string()));
        }
        if self
            .max_amount_sample
            .as_ref()
            .map(|(current, _)| value > *current)
            .unwrap_or(true)
        {
            self.max_amount_sample = Some((value, raw.to_string()));
        }
    }

    fn into_json(mut self, rows_total: u64) -> Value {
        self.fixed_seed_samples
            .sort_by_key(|sample| (sample.rank, sample.row_no));
        let fixed_seed_samples = self
            .fixed_seed_samples
            .into_iter()
            .map(|sample| sample.value)
            .collect::<Vec<_>>();
        let feature_samples = json!({
            "longest": self.longest_sample,
            "min_amount": self.min_amount_sample.map(|(_, raw)| raw),
            "max_amount": self.max_amount_sample.map(|(_, raw)| raw),
            "date_like": self.date_sample,
        });
        let distinct_count = if self.distinct_limit_exceeded {
            Value::Null
        } else {
            json!(self.distinct_values.len())
        };

        json!({
            "index": self.profile_index,
            "source_index": self.source_index,
            "header": self.header,
            "non_empty": self.non_empty,
            "non_empty_ratio": ratio(self.non_empty, rows_total),
            "amount_like": self.amount_like,
            "amount_like_ratio": ratio(self.amount_like, self.non_empty),
            "signed_amount": self.signed_amount,
            "positive_amount": self.positive_amount,
            "negative_amount": self.negative_amount,
            "date_like": self.date_like,
            "date_like_ratio": ratio(self.date_like, self.non_empty),
            "datetime_like": self.datetime_like,
            "account_like": self.account_like,
            "id_no_like": self.id_no_like,
            "phone_like": self.phone_like,
            "ip_like": self.ip_like,
            "mac_like": self.mac_like,
            "distinct_count": distinct_count,
            "distinct_limit_exceeded": self.distinct_limit_exceeded,
            "first_non_empty": self.first_non_empty,
            "fixed_seed_samples": fixed_seed_samples,
            "feature_samples": feature_samples,
        })
    }
}

struct RankedSample {
    rank: u64,
    row_no: u64,
    value: String,
}

struct ParsedAmount {
    value: f64,
    signed: bool,
}

fn parse_amount(value: &str) -> Option<ParsedAmount> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return None;
    }
    let mut negative_by_parentheses = false;
    let mut inner = trimmed;
    if inner.starts_with('(') && inner.ends_with(')') && inner.len() > 2 {
        negative_by_parentheses = true;
        inner = &inner[1..inner.len() - 1];
    }
    let compact = inner
        .chars()
        .filter(|ch| {
            !ch.is_whitespace() && *ch != ',' && !matches!(*ch, '¥' | '￥' | '$' | '€' | '£')
        })
        .collect::<String>();
    if compact.is_empty() {
        return None;
    }
    let signed_by_prefix = compact.starts_with('-') || compact.starts_with('+');
    let parsed = compact.parse::<f64>().ok()?;
    let value = if negative_by_parentheses {
        -parsed.abs()
    } else {
        parsed
    };
    Some(ParsedAmount {
        value,
        signed: signed_by_prefix || negative_by_parentheses,
    })
}

fn should_profile_as_amount(
    value: &str,
    date_kind: &DateKind,
    account_like: bool,
    id_no_like: bool,
    phone_like: bool,
) -> bool {
    if date_kind.date_like || id_no_like || phone_like {
        return false;
    }
    let compact = value
        .trim()
        .chars()
        .filter(|ch| !ch.is_whitespace())
        .collect::<String>();
    let has_amount_marker = compact.contains('.')
        || compact.contains(',')
        || compact.starts_with('-')
        || compact.starts_with('+')
        || (compact.starts_with('(') && compact.ends_with(')'));
    !account_like || has_amount_marker
}

struct DateKind {
    date_like: bool,
    datetime_like: bool,
}

fn detect_date_kind(value: &str) -> DateKind {
    let compact = value.trim();
    let digits = compact.chars().filter(|ch| ch.is_ascii_digit()).count();
    let all_digits = compact.chars().all(|ch| ch.is_ascii_digit());
    let has_time_separator = compact.contains(':');
    let compact_datetime_like =
        all_digits && matches!(digits, 12 | 14) && compact_datetime_components_plausible(compact);
    let compact_date_like = all_digits && digits == 8 && compact_date_components_plausible(compact);
    let separated_date_like = separated_date_components_plausible(compact);
    let date_like = compact_date_like || separated_date_like || compact_datetime_like;
    let datetime_like = date_like && (has_time_separator || compact_datetime_like);
    DateKind {
        date_like,
        datetime_like,
    }
}

fn compact_date_components_plausible(value: &str) -> bool {
    if value.len() != 8 {
        return false;
    }
    let parse = |range: std::ops::Range<usize>| value[range].parse::<u32>().ok();
    let (Some(year), Some(month), Some(day)) = (parse(0..4), parse(4..6), parse(6..8)) else {
        return false;
    };
    date_components_plausible(year, month, day)
}

fn separated_date_components_plausible(value: &str) -> bool {
    let groups = value
        .split(|ch: char| !ch.is_ascii_digit())
        .filter(|part| !part.is_empty())
        .take(3)
        .filter_map(|part| part.parse::<u32>().ok())
        .collect::<Vec<_>>();
    if groups.len() < 3 {
        return false;
    }
    date_components_plausible(groups[0], groups[1], groups[2])
}

fn compact_datetime_components_plausible(value: &str) -> bool {
    if value.len() != 12 && value.len() != 14 {
        return false;
    }
    let parse = |range: std::ops::Range<usize>| value[range].parse::<u32>().ok();
    let year = parse(0..4);
    let month = parse(4..6);
    let day = parse(6..8);
    let hour = parse(8..10);
    let minute = parse(10..12);
    let second = if value.len() == 14 {
        parse(12..14).unwrap_or(60)
    } else {
        0
    };
    let (Some(year), Some(month), Some(day), Some(hour), Some(minute)) =
        (year, month, day, hour, minute)
    else {
        return false;
    };
    if !date_components_plausible(year, month, day) || hour > 23 || minute > 59 || second > 59 {
        return false;
    }
    true
}

fn date_components_plausible(year: u32, month: u32, day: u32) -> bool {
    if !(1900..=2199).contains(&year) || !(1..=12).contains(&month) {
        return false;
    }
    let max_day = match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if is_leap_year(year) => 29,
        2 => 28,
        _ => 0,
    };
    (1..=max_day).contains(&day)
}

fn is_leap_year(year: u32) -> bool {
    (year % 4 == 0 && year % 100 != 0) || year % 400 == 0
}

fn looks_like_account(value: &str) -> bool {
    let digits = digits_only(value);
    (10..=22).contains(&digits.len())
}

fn looks_like_id_no(value: &str) -> bool {
    let compact = value
        .chars()
        .filter(|ch| !ch.is_whitespace())
        .collect::<String>();
    let chars = compact.chars().collect::<Vec<_>>();
    if chars.len() == 18 {
        let (head, tail) = chars.split_at(17);
        return head.iter().all(|ch| ch.is_ascii_digit())
            && tail
                .iter()
                .all(|ch| ch.is_ascii_digit() || *ch == 'X' || *ch == 'x');
    }
    chars.len() == 15 && chars.iter().all(|ch| ch.is_ascii_digit())
}

fn looks_like_phone(value: &str) -> bool {
    let digits = digits_only(value);
    digits.len() == 11 && digits.starts_with('1')
}

fn looks_like_ip(value: &str) -> bool {
    let parts = value.trim().split('.').collect::<Vec<_>>();
    parts.len() == 4
        && parts.iter().all(|part| {
            !part.is_empty()
                && part.len() <= 3
                && part.chars().all(|ch| ch.is_ascii_digit())
                && part.parse::<u8>().is_ok()
        })
}

fn looks_like_mac(value: &str) -> bool {
    let text = value.trim();
    let separator = if text.contains(':') {
        ':'
    } else if text.contains('-') {
        '-'
    } else {
        return false;
    };
    let parts = text.split(separator).collect::<Vec<_>>();
    parts.len() == 6
        && parts
            .iter()
            .all(|part| part.len() == 2 && part.chars().all(|ch| ch.is_ascii_hexdigit()))
}

fn digits_only(value: &str) -> String {
    value.chars().filter(|ch| ch.is_ascii_digit()).collect()
}

fn ratio(count: u64, total: u64) -> f64 {
    if total == 0 {
        0.0
    } else {
        ((count as f64 / total as f64) * 10000.0).round() / 10000.0
    }
}

fn stable_sample_rank(column_index: usize, row_no: u64, value: &str) -> u64 {
    let mut hash = 0xcbf29ce484222325_u64;
    for byte in column_index
        .to_le_bytes()
        .into_iter()
        .chain(row_no.to_le_bytes())
        .chain(value.as_bytes().iter().copied())
    {
        hash ^= u64::from(byte);
        hash = hash.wrapping_mul(0x100000001b3);
    }
    hash
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::csv::test_support::{write_text, TestDir};

    #[test]
    fn profile_csv_columns_detects_samples_and_value_shapes() {
        let dir = TestDir::new("profile-columns");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "交易日期,交易时刻,交易金额,交易账号,证件号码,IP地址,MAC地址\n\
20260102,093010,-123.45,6222000000000000001,11010119900307123X,192.168.1.1,AA:BB:CC:DD:EE:FF\n\
2026-01-03,10:15,88.00,6222000000000000002,110101199003071230,10.0.0.1,aa-bb-cc-dd-ee-00\n\
2026/01/04 11:00:00,,(66.50),6222000000000000003,110101900307123,not-ip,not-mac\n",
        );

        let payload = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        assert_eq!(payload["ok"], true);
        assert_eq!(payload["rows_total"], 3);
        assert_eq!(payload["columns_total"], 7);
        let columns = payload["columns"].as_array().unwrap();
        assert_eq!(columns[0]["header"], "交易日期");
        assert_eq!(columns[0]["date_like"], 3);
        assert_eq!(columns[0]["datetime_like"], 1);
        assert_eq!(columns[2]["amount_like"], 3);
        assert_eq!(columns[2]["signed_amount"], 2);
        assert_eq!(columns[2]["negative_amount"], 2);
        assert_eq!(columns[2]["positive_amount"], 1);
        assert_eq!(columns[3]["account_like"], 3);
        assert_eq!(columns[4]["id_no_like"], 3);
        assert_eq!(columns[5]["ip_like"], 2);
        assert_eq!(columns[6]["mac_like"], 2);
        assert_eq!(
            columns[2]["first_non_empty"],
            json!(["-123.45", "88.00", "(66.50)"])
        );
        assert_eq!(columns[2]["feature_samples"]["min_amount"], "-123.45");
        assert_eq!(columns[2]["feature_samples"]["max_amount"], "88.00");
    }

    #[test]
    fn profile_csv_columns_detects_twelve_digit_compact_datetime() {
        let dir = TestDir::new("profile-columns-compact-datetime");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "交易时间,交易金额\n\
202601081040,80.00\n\
202602281159,-20.00\n",
        );

        let payload = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        let columns = payload["columns"].as_array().unwrap();
        assert_eq!(payload["rows_total"], 2);
        assert_eq!(columns[0]["date_like"], 2);
        assert_eq!(columns[0]["datetime_like"], 2);
        assert_eq!(columns[1]["amount_like"], 2);
        assert_eq!(columns[1]["negative_amount"], 1);
    }

    #[test]
    fn profile_csv_columns_keeps_currency_amounts_and_decimal_balances_out_of_dates() {
        let dir = TestDir::new("profile-columns-currency-amounts");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "交易金额,账户余额,发生日期\n\
\"¥1,234.50\",\"10,000.50\",2026.01.12\n\
\"(2,345.67)\",\"7,654.83\",20260113\n\
-88.00,\"7,566.83\",2026-01-14\n",
        );

        let payload = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        let columns = payload["columns"].as_array().unwrap();
        assert_eq!(payload["rows_total"], 3);
        assert_eq!(columns[0]["amount_like"], 3);
        assert_eq!(columns[0]["negative_amount"], 2);
        assert_eq!(columns[1]["amount_like"], 3);
        assert_eq!(columns[1]["date_like"], 0);
        assert_eq!(columns[2]["date_like"], 3);
    }

    #[test]
    fn profile_csv_columns_handles_multibyte_text_without_id_byte_split() {
        let dir = TestDir::new("profile-columns-multibyte-text");
        let csv_path = dir.join("sample.csv");
        write_text(
            &csv_path,
            "摘要\n\
规则拆列支出\n",
        );

        let payload = profile_csv_columns(&csv_path, "utf-8-sig").unwrap();

        let columns = payload["columns"].as_array().unwrap();
        assert_eq!(payload["ok"], true);
        assert_eq!(columns[0]["header"], "摘要");
        assert_eq!(columns[0]["id_no_like"], 0);
        assert_eq!(columns[0]["first_non_empty"], json!(["规则拆列支出"]));
    }
}
