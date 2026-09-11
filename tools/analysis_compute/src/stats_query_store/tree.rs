use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use sha1::{Digest, Sha1};
use std::collections::{HashMap, HashSet};

use super::args::QueryStatsTreeArgs;
use super::session::VerifiedStatsQuerySession;

#[derive(Clone)]
struct AccountTreeRow {
    account_key: String,
    acct_display: String,
    card_display: String,
    open_name: String,
    id_no: String,
    bank_name: String,
    branch_name: String,
    acct_type: String,
}

#[derive(Clone)]
struct TreeItem {
    id: String,
    title: String,
    sub: String,
}

struct TreeGroup {
    id: String,
    title: String,
    meta: String,
    extra: String,
    items: Vec<TreeItem>,
    seen: HashSet<String>,
    kind_order: i32,
}

pub(crate) fn query_stats_tree(args: &QueryStatsTreeArgs) -> Result<Vec<Value>> {
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_stats_tree_with_session(args, &mut session)?;
    session.commit()?;
    Ok(result)
}

pub(super) fn query_stats_tree_with_session(
    args: &QueryStatsTreeArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
) -> Result<Vec<Value>> {
    let sql = account_tree_rows_sql(session.conn(), &args.case_id)?;
    session.require_nonempty_query("stats_tree", &sql)?;
    let rows = query_account_tree_rows(session.conn(), &sql)?;
    Ok(build_tree_groups(&args.tab, rows))
}

fn query_account_tree_rows(conn: &Connection, sql: &str) -> Result<Vec<AccountTreeRow>> {
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], |row| {
        Ok(AccountTreeRow {
            account_key: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
            acct_display: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            card_display: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
            open_name: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
            id_no: row.get::<_, Option<String>>(4)?.unwrap_or_default(),
            bank_name: row.get::<_, Option<String>>(5)?.unwrap_or_default(),
            branch_name: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
            acct_type: row.get::<_, Option<String>>(7)?.unwrap_or_default(),
        })
    })?;
    let mut rows = Vec::new();
    for item in mapped {
        rows.push(item?);
    }
    Ok(rows)
}

fn account_tree_rows_sql(conn: &Connection, case_id: &str) -> Result<String> {
    if !crate::table_exists(conn, "analysis_manual_account_mapping")? {
        return Ok(format!(
            "
            SELECT
              COALESCE(NULLIF(TRIM(account_key), ''), '') AS account_key,
              COALESCE(acct_display, '') AS acct_display,
              COALESCE(card_display, '') AS card_display,
              COALESCE(open_name, '') AS open_name,
              COALESCE(id_no, '') AS id_no,
              COALESCE(bank_name, '') AS bank_name,
              COALESCE(branch_name, '') AS branch_name,
              COALESCE(acct_type, '') AS acct_type
            FROM {}
            ORDER BY account_key
            ",
            crate::ACCOUNT_DIM_TABLE
        ));
    }

    let manual_cols = crate::table_columns(conn, "analysis_manual_account_mapping")?;
    let manual_open_name = manual_text_expr(&manual_cols, "account_open_name");
    let manual_id_no = manual_text_expr(&manual_cols, "opener_id_no");
    let manual_bank_name = manual_text_expr(&manual_cols, "bank_name");
    Ok(format!(
        "
        SELECT
          COALESCE(NULLIF(TRIM(d.account_key), ''), '') AS account_key,
          COALESCE(d.acct_display, '') AS acct_display,
          COALESCE(d.card_display, '') AS card_display,
          CASE WHEN m.account_key IS NOT NULL THEN COALESCE({manual_open_name}, '') ELSE COALESCE(d.open_name, '') END AS open_name,
          CASE WHEN m.account_key IS NOT NULL THEN COALESCE({manual_id_no}, '') ELSE COALESCE(d.id_no, '') END AS id_no,
          CASE WHEN {manual_bank_name} IS NOT NULL THEN {manual_bank_name} ELSE COALESCE(d.bank_name, '') END AS bank_name,
          COALESCE(d.branch_name, '') AS branch_name,
          COALESCE(d.acct_type, '') AS acct_type
        FROM {} d
        LEFT JOIN analysis_manual_account_mapping m
          ON m.case_id = {}
         AND m.account_key = d.account_key
        ORDER BY d.account_key
        ",
        crate::ACCOUNT_DIM_TABLE,
        crate::sql_literal(case_id),
    ))
}

fn manual_text_expr(cols: &[String], column: &str) -> String {
    if cols.iter().any(|item| item == column) {
        format!("NULLIF(TRIM(m.{column}), '')")
    } else {
        "NULL".to_string()
    }
}

fn build_tree_groups(tab: &str, rows: Vec<AccountTreeRow>) -> Vec<Value> {
    let tree_tab = normalize_tree_tab(tab);
    let mut groups: Vec<TreeGroup> = Vec::new();
    let mut group_indexes: HashMap<String, usize> = HashMap::new();
    let mut seen_accounts = HashSet::new();

    for raw in rows {
        let account_key = clean_text(&raw.account_key);
        if account_key.is_empty() || !seen_accounts.insert(account_key.clone()) {
            continue;
        }

        let open_name = clean_text(&raw.open_name);
        let id_no = clean_text(&raw.id_no);
        let bank_name = normalize_tree_bank_name(&raw.bank_name);
        let branch_name = clean_text(&raw.branch_name);
        let acct_type = normalize_tree_account_type(&raw.acct_type);
        let item_title = first_non_empty(&[&raw.card_display, &raw.acct_display, &account_key]);

        let (group_key, group, item_sub) = if tree_tab == "byCard" {
            let display_name = if open_name.is_empty() {
                "未命名户名".to_string()
            } else {
                open_name.clone()
            };
            let group_key = if bank_name.is_empty() {
                "未知开户行".to_string()
            } else {
                bank_name.clone()
            };
            let item_sub = join_non_empty(&[&display_name, &acct_type]);
            (
                group_key.clone(),
                TreeGroup {
                    id: stable_tree_group_id(&tree_tab, &group_key),
                    title: group_key,
                    meta: "卡号列表".to_string(),
                    extra: branch_name,
                    items: Vec::new(),
                    seen: HashSet::new(),
                    kind_order: 0,
                },
                item_sub,
            )
        } else if open_name.is_empty() {
            let group_key = format!("acct:{account_key}");
            let title = item_title.clone();
            let extra = if bank_name.is_empty() {
                branch_name.clone()
            } else {
                bank_name.clone()
            };
            let item_sub = join_non_empty(&[&id_no, &bank_name, &acct_type]);
            (
                group_key.clone(),
                TreeGroup {
                    id: stable_tree_group_id(&tree_tab, &group_key),
                    title,
                    meta: "未登记户名".to_string(),
                    extra,
                    items: Vec::new(),
                    seen: HashSet::new(),
                    kind_order: 1,
                },
                item_sub,
            )
        } else {
            let display_id = if id_no.is_empty() {
                "无证件号".to_string()
            } else {
                id_no.clone()
            };
            let group_key = format!("{open_name}|{display_id}");
            let extra = if bank_name.is_empty() {
                branch_name.clone()
            } else {
                bank_name.clone()
            };
            let item_sub = join_non_empty(&[&bank_name, &acct_type]);
            (
                group_key.clone(),
                TreeGroup {
                    id: stable_tree_group_id(&tree_tab, &group_key),
                    title: open_name,
                    meta: display_id,
                    extra,
                    items: Vec::new(),
                    seen: HashSet::new(),
                    kind_order: 0,
                },
                item_sub,
            )
        };

        let index = if let Some(index) = group_indexes.get(&group_key) {
            *index
        } else {
            groups.push(group);
            let index = groups.len() - 1;
            group_indexes.insert(group_key, index);
            index
        };
        let group = &mut groups[index];
        if group.seen.insert(account_key.clone()) {
            group.items.push(TreeItem {
                id: account_key,
                title: item_title,
                sub: item_sub,
            });
        }
    }

    for group in &mut groups {
        group.items.sort_by(|left, right| {
            left.title
                .cmp(&right.title)
                .then_with(|| left.sub.cmp(&right.sub))
        });
    }
    if tree_tab == "byName" {
        groups.sort_by(|left, right| {
            left.kind_order
                .cmp(&right.kind_order)
                .then_with(|| right.items.len().cmp(&left.items.len()))
                .then_with(|| left.title.cmp(&right.title))
        });
    } else {
        groups.sort_by(|left, right| left.title.cmp(&right.title));
    }

    groups
        .into_iter()
        .map(|group| {
            json!({
                "id": group.id,
                "title": group.title,
                "meta": group.meta,
                "extra": group.extra,
                "items": group.items.into_iter().map(|item| {
                    json!({
                        "id": item.id,
                        "title": item.title,
                        "sub": item.sub,
                    })
                }).collect::<Vec<_>>(),
            })
        })
        .collect()
}

fn normalize_tree_tab(tab: &str) -> String {
    let value = tab.trim();
    if value.is_empty() {
        "byName".to_string()
    } else {
        value.to_string()
    }
}

fn stable_tree_group_id(tab: &str, group_key: &str) -> String {
    let normalized_tab = normalize_tree_tab(tab);
    let normalized_key = if group_key.trim().is_empty() {
        "_"
    } else {
        group_key.trim()
    };
    let mut hasher = Sha1::new();
    hasher.update(format!("{normalized_tab}:{normalized_key}").as_bytes());
    format!("{normalized_tab}-{:x}", hasher.finalize())
}

fn clean_text(value: &str) -> String {
    value.trim().to_string()
}

fn first_non_empty(values: &[&str]) -> String {
    values
        .iter()
        .map(|value| value.trim())
        .find(|value| !value.is_empty())
        .unwrap_or("")
        .to_string()
}

fn join_non_empty(values: &[&str]) -> String {
    values
        .iter()
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>()
        .join(" · ")
}

fn normalize_tree_bank_name(value: &str) -> String {
    let raw = value.trim();
    if raw.is_empty() {
        return String::new();
    }
    let mut cleaned = remove_parenthetical(raw, '（', '）');
    cleaned = remove_parenthetical(&cleaned, '(', ')');
    cleaned = cleaned.chars().filter(|ch| !ch.is_whitespace()).collect();
    if is_tree_bank_placeholder(&cleaned) {
        return String::new();
    }
    if let Some(region) = cleaned.strip_suffix("农村信用社联合社") {
        if !region.is_empty() {
            let region = shorten_tree_bank_region(region);
            return if region.is_empty() {
                "农信".to_string()
            } else {
                format!("{region}农信")
            };
        }
    }
    if let Some(region) = cleaned.strip_suffix("农村信用合作联社") {
        if !region.is_empty() {
            let region = shorten_tree_bank_region(region);
            return if region.is_empty() {
                "农信联社".to_string()
            } else {
                format!("{region}农信联社")
            };
        }
    }
    for (needle, normalized) in TREE_BANK_DISPLAY_ALIASES {
        if cleaned.contains(needle) {
            return normalized.to_string();
        }
    }
    let mut out = cleaned;
    loop {
        let mut changed = false;
        for suffix in TREE_BANK_NAME_SUFFIXES {
            if out.ends_with(suffix) {
                out = out[..out.len() - suffix.len()].trim().to_string();
                changed = true;
                break;
            }
        }
        if !changed || out.is_empty() {
            break;
        }
    }
    if is_tree_bank_placeholder(&out) {
        String::new()
    } else {
        out
    }
}

fn remove_parenthetical(value: &str, open: char, close: char) -> String {
    let mut out = String::new();
    let mut depth = 0_i32;
    for ch in value.chars() {
        if ch == open {
            depth += 1;
            continue;
        }
        if ch == close && depth > 0 {
            depth -= 1;
            continue;
        }
        if depth == 0 {
            out.push(ch);
        }
    }
    out
}

fn shorten_tree_bank_region(value: &str) -> String {
    let mut out = value.trim().to_string();
    for suffix in [
        "自治区",
        "自治州",
        "地区",
        "省",
        "市",
        "县",
        "区",
        "州",
        "盟",
    ] {
        if out.ends_with(suffix) && out.len() > suffix.len() {
            out = out[..out.len() - suffix.len()].trim().to_string();
            break;
        }
    }
    out
}

fn is_tree_bank_placeholder(value: &str) -> bool {
    matches!(value, "" | "未录入" | "未录入归属信息" | "未录入归属行")
}

fn normalize_tree_account_type(value: &str) -> String {
    let cleaned: String = value
        .trim()
        .chars()
        .filter(|ch| !ch.is_whitespace())
        .collect();
    if cleaned.is_empty() || is_tree_account_type_placeholder(&cleaned) {
        return String::new();
    }
    if let Some(alias) = tree_account_type_exact_alias(&cleaned) {
        return alias.to_string();
    }

    let level = normalize_tree_account_level(&cleaned);
    if !level.is_empty() && is_exact_level_account(&cleaned) {
        return level;
    }
    if cleaned.len() <= 6 && cleaned.chars().all(|ch| ch.is_ascii_alphanumeric()) {
        return String::new();
    }

    let base = if cleaned.contains("贷记卡") || cleaned.contains("信用卡") {
        "信用卡".to_string()
    } else if cleaned.contains("结算账户") {
        "活期结算".to_string()
    } else if cleaned.contains("储蓄账户") || cleaned.contains("储蓄存款") {
        "储蓄".to_string()
    } else if cleaned.contains("活期") || cleaned.contains("无折户") || cleaned.contains("一本通")
    {
        "活期".to_string()
    } else if cleaned.contains("定期") || cleaned.contains("存单") {
        "定期".to_string()
    } else if cleaned.contains("借记卡") {
        "借记卡".to_string()
    } else if cleaned.ends_with('卡') {
        "卡账户".to_string()
    } else if cleaned.contains("实体账户") {
        "实体账户".to_string()
    } else if cleaned.contains("薪金煲") {
        "薪金煲".to_string()
    } else {
        cleaned
    };

    if !level.is_empty() {
        if !base.is_empty() && base != level {
            return format!("{base} · {level}");
        }
        return level;
    }
    base
}

fn is_tree_account_type_placeholder(value: &str) -> bool {
    matches!(
        value,
        "" | "-" | "—" | "－" | "未知" | "未录入" | "未录入账户类型"
    )
}

fn tree_account_type_exact_alias(value: &str) -> Option<&'static str> {
    match value {
        "借记卡" => Some("借记卡"),
        "个人储蓄账户" => Some("储蓄"),
        "零售活期结算账户" => Some("活期结算"),
        "个人人民币活期普通结算账户" => Some("活期结算"),
        "活期" | "活期存款" | "活期无折户" | "活期多币种" | "活期储蓄存款" | "活期一本通" => {
            Some("活期")
        }
        "定期" | "定期一本通" | "存单" => Some("定期"),
        "信用卡" | "信用卡主卡" | "贷记卡" => Some("信用卡"),
        "实体账户" => Some("实体账户"),
        "卡" => Some("卡账户"),
        "薪金煲" => Some("薪金煲"),
        _ => None,
    }
}

fn normalize_tree_account_level(value: &str) -> String {
    let upper = value.to_ascii_uppercase();
    if upper.contains("III类") || value.contains("Ⅲ类") || value.contains("三类") {
        return "III类".to_string();
    }
    if upper.contains("II类") || value.contains("Ⅱ类") || value.contains("二类") {
        return "II类".to_string();
    }
    if has_i_level(&upper) || value.contains("Ⅰ类") || value.contains("一类") {
        return "I类".to_string();
    }
    String::new()
}

fn has_i_level(value: &str) -> bool {
    let mut start = 0;
    while let Some(relative_index) = value[start..].find("I类") {
        let index = start + relative_index;
        let previous = value[..index].chars().next_back();
        if index == 0 || !previous.map(|ch| ch.is_ascii_alphabetic()).unwrap_or(false) {
            return true;
        }
        start = index + 1;
    }
    false
}

fn is_exact_level_account(value: &str) -> bool {
    matches!(
        value,
        "I类账户"
            | "II类账户"
            | "III类账户"
            | "Ⅰ类账户"
            | "Ⅱ类账户"
            | "Ⅲ类账户"
            | "一类账户"
            | "二类账户"
            | "三类账户"
    )
}

const TREE_BANK_DISPLAY_ALIASES: &[(&str, &str)] = &[
    ("中国邮政储蓄银行", "邮政储蓄银行"),
    ("中国工商银行", "工商银行"),
    ("中国建设银行", "建设银行"),
    ("中国农业银行", "农业银行"),
    ("中国民生银行", "民生银行"),
    ("中国光大银行", "光大银行"),
    ("中国银行", "中国银行"),
    ("上海浦东发展银行", "上海浦东发展银行"),
    ("招商银行", "招商银行"),
    ("交通银行", "交通银行"),
    ("兴业银行", "兴业银行"),
    ("华夏银行", "华夏银行"),
    ("广发银行", "广发银行"),
    ("中信银行", "中信银行"),
    ("平安银行", "平安银行"),
    ("重庆银行", "重庆银行"),
    ("贵州银行", "贵州银行"),
    ("贵阳银行", "贵阳银行"),
];

const TREE_BANK_NAME_SUFFIXES: &[&str] = &[
    "股份有限公司",
    "有限责任公司",
    "股份公司",
    "股份",
    "有限公司",
];
