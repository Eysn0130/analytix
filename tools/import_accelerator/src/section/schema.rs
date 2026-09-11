pub(crate) fn detect_fc_kind(headers: &[String]) -> Option<&'static str> {
    let got = headers.iter().map(|h| h.as_str()).collect::<Vec<_>>();
    let got_norm = got
        .iter()
        .map(|h| normalize_header_for_match(h))
        .collect::<Vec<_>>();
    let sub_account_hint = got_norm
        .iter()
        .any(|h| h.contains("子账户") || h.contains("子帐户"));
    let schemas = fc_schemas();

    let mut best_kind = "";
    let mut best_score = 0_i32;
    for schema in schemas {
        let mut score = 0_i32;
        for header in schema.headers {
            if got.iter().any(|h| h == header) {
                score += 1;
                continue;
            }
            let normalized = normalize_header_for_match(header);
            if got_norm.iter().any(|h| h == &normalized) {
                score += 1;
                continue;
            }
            if schema
                .aliases
                .iter()
                .filter(|(standard, _)| standard == header)
                .any(|(_, alias)| {
                    let alias_norm = normalize_header_for_match(alias);
                    got_norm.iter().any(|h| h == &alias_norm)
                })
            {
                score += 1;
            }
        }
        if schema.kind == "fc_sub_account" && sub_account_hint {
            score += 3;
        }
        if score > best_score {
            best_kind = schema.kind;
            best_score = score;
        }
    }
    if best_score >= 3 {
        Some(best_kind)
    } else {
        None
    }
}

fn normalize_header_for_match(name: &str) -> String {
    let mut out = String::new();
    let normalized = name.replace('\u{feff}', "");
    let mut paren_depth = 0_i32;
    for ch in normalized.trim().chars() {
        if ch == '(' || ch == '（' {
            paren_depth += 1;
            continue;
        }
        if ch == ')' || ch == '）' {
            if paren_depth > 0 {
                paren_depth -= 1;
            }
            continue;
        }
        if paren_depth > 0 || ch.is_whitespace() || ch == '\u{3000}' {
            continue;
        }
        out.push(ch);
    }
    out
}

struct Schema {
    kind: &'static str,
    headers: &'static [&'static str],
    aliases: &'static [(&'static str, &'static str)],
}

fn fc_schemas() -> Vec<Schema> {
    vec![
        Schema {
            kind: "fc_account",
            headers: &[
                "账户开户名称",
                "开户人证件号码",
                "交易卡号",
                "交易账号",
                "账号开户时间",
                "账户余额",
                "可用余额",
                "币种",
                "开户网点代码",
                "开户网点",
                "账户状态",
                "钞汇标志名称",
                "销户日期",
                "账户类型",
                "备注",
                "账号开户银行",
                "销户网点",
                "最后交易时间",
            ],
            aliases: &ACCOUNT_ALIASES,
        },
        Schema {
            kind: "fc_person",
            headers: &[
                "客户名称",
                "证照类型",
                "证照号码",
                "单位地址",
                "单位电话",
                "工作单位",
                "邮箱地址",
                "代办人姓名",
                "代办人证件类型",
                "代办人证件号码",
                "国税纳税号",
                "地税纳税号",
                "法人代表",
                "客户工商执照号码",
            ],
            aliases: &[],
        },
        Schema {
            kind: "fc_coercive_measure",
            headers: &[
                "银行名称",
                "账号",
                "冻结措施类型",
                "冻结金额",
                "冻结机关",
                "冻结开始日期",
                "冻结截止日期",
                "措施序号",
                "备注",
            ],
            aliases: &[],
        },
        Schema {
            kind: "fc_transaction",
            headers: &[
                "交易卡号",
                "交易账号",
                "账户开户名称",
                "开户人证件号码",
                "交易时间",
                "交易金额",
                "交易余额",
                "收付标志",
                "交易对手账卡号",
                "现金标志",
                "对手户名",
                "对手身份证号",
                "对手开户银行",
                "摘要说明",
                "交易币种",
                "交易网点名称",
                "交易发生地",
                "交易是否成功",
                "传票号",
                "IP地址",
                "MAC地址",
                "对手交易余额",
                "交易流水号",
                "日志号",
                "凭证种类",
                "凭证号",
                "交易柜员号",
                "备注",
                "交易类型",
                "查询反馈结果原因",
            ],
            aliases: &TXN_ALIASES,
        },
        Schema {
            kind: "fc_sub_account",
            headers: &[
                "银行名称",
                "开户账号",
                "子账户账号",
                "余额",
                "可用余额",
                "子账户类别",
                "子账户序号",
                "币种",
                "钞汇标识",
                "账户状态",
                "账户序号",
            ],
            aliases: &SUB_ACCOUNT_ALIASES,
        },
        Schema {
            kind: "fc_person_address",
            headers: &["开户名称", "证照类型", "证照号码", "住宅地址", "住宅电话"],
            aliases: &[],
        },
        Schema {
            kind: "fc_person_contact",
            headers: &["开户名称", "证照类型", "证照号码", "联系电话"],
            aliases: &[],
        },
        Schema {
            kind: "fc_task_success",
            headers: &TASK_HEADERS,
            aliases: &[],
        },
        Schema {
            kind: "fc_task_fail",
            headers: &TASK_HEADERS,
            aliases: &[],
        },
    ]
}

const TASK_HEADERS: [&str; 14] = [
    "任务流水号",
    "银行名称",
    "主体类别",
    "证账号码",
    "账卡号",
    "发送时间",
    "反馈时间",
    "反馈结果",
    "反馈非明细结果",
    "反馈明细结果",
    "入库时间",
    "入库状态",
    "请求单号",
    "查询结果",
];

const ACCOUNT_ALIASES: [(&str, &str); 36] = [
    ("账户开户名称", "姓名"),
    ("账户开户名称", "姓名(查询条件)"),
    ("账户开户名称", "开户名"),
    ("账户开户名称", "户名"),
    ("账户开户名称", "客户名称"),
    ("账户开户名称", "账户名称"),
    ("开户人证件号码", "证件号码"),
    ("开户人证件号码", "证件号码(查询条件)"),
    ("开户人证件号码", "证件号"),
    ("开户人证件号码", "身份证号"),
    ("交易卡号", "卡号"),
    ("交易卡号", "本方卡号"),
    ("交易卡号", "账卡号"),
    ("交易卡号", "查询卡号"),
    ("交易账号", "账号"),
    ("交易账号", "账户账号"),
    ("交易账号", "账户号"),
    ("交易账号", "本方账号"),
    ("交易账号", "本方账户"),
    ("交易账号", "查询账号"),
    ("交易账号", "查询帐号"),
    ("账号开户时间", "开户日期"),
    ("账号开户时间", "开户时间"),
    ("账号开户时间", "账号开户日期"),
    ("账号开户时间", "开户日期时间"),
    ("账户余额", "余额"),
    ("账户余额", "账户余额"),
    ("账户余额", "账面余额"),
    ("可用余额", "可用余额"),
    ("可用余额", "可用金额"),
    ("可用余额", "可用资金"),
    ("币种", "币种"),
    ("币种", "币别"),
    ("开户网点代码", "交易网点代码"),
    ("开户网点代码", "网点代码"),
    ("开户网点", "网点名称"),
];

const SUB_ACCOUNT_ALIASES: [(&str, &str); 22] = [
    ("银行名称", "开户银行"),
    ("银行名称", "开户行"),
    ("银行名称", "银行"),
    ("开户账号", "账卡号"),
    ("开户账号", "账户账号"),
    ("开户账号", "账号"),
    ("开户账号", "主账户账号"),
    ("开户账号", "本方账号"),
    ("开户账号", "卡号"),
    ("子账户账号", "子账号"),
    ("子账户账号", "子账户号"),
    ("余额", "账户余额"),
    ("余额", "子账户余额"),
    ("可用余额", "可用金额"),
    ("子账户类别", "子账户类型"),
    ("子账户序号", "子账户编号"),
    ("子账户序号", "子账户顺序号"),
    ("币种", "币别"),
    ("钞汇标志", "钞汇标志名称"),
    ("钞汇标志", "钞汇"),
    ("账户状态", "账号状态"),
    ("账户序号", "总账户序号"),
];

const TXN_ALIASES: [(&str, &str); 45] = [
    ("交易卡号", "本方卡号"),
    ("交易卡号", "查询卡号"),
    ("交易账号", "本方账号"),
    ("账户开户名称", "姓名"),
    ("账户开户名称", "姓名(查询条件)"),
    ("账户开户名称", "开户名"),
    ("账户开户名称", "户名"),
    ("开户人证件号码", "证件号码"),
    ("开户人证件号码", "证件号码(查询条件)"),
    ("开户人证件号码", "证件号"),
    ("开户人证件号码", "身份证号"),
    ("收付标志", "借贷标志"),
    ("收付标志", "借贷方向"),
    ("收付标志", "借方贷方"),
    ("收付标志", "进出标志"),
    ("收付标志", "借贷标记"),
    ("交易对手账卡号", "交易对方账号"),
    ("交易对手账卡号", "交易对方卡号"),
    ("交易对手账卡号", "对方账号"),
    ("交易对手账卡号", "对方卡号"),
    ("交易对手账卡号", "对手账号"),
    ("交易对手账卡号", "对手卡号"),
    ("对手户名", "交易对方名称"),
    ("对手户名", "对方名称"),
    ("对手户名", "对手名称"),
    ("对手身份证号", "交易对方证件号码"),
    ("对手身份证号", "交易对方证件号"),
    ("对手身份证号", "对方证件号码"),
    ("对手身份证号", "对方证件号"),
    ("对手身份证号", "对手证件号"),
    ("对手开户银行", "交易对方账号开户行"),
    ("对手开户银行", "交易对方开户行"),
    ("对手开户银行", "对方开户行"),
    ("对手开户银行", "对手开户行"),
    ("对手开户银行", "对方开户银行"),
    ("摘要说明", "交易摘要"),
    ("摘要说明", "摘要"),
    ("交易币种", "币种"),
    ("交易币种", "币别"),
    ("对手交易余额", "交易对手余额"),
    ("对手交易余额", "对方余额"),
    ("对手交易余额", "对手余额"),
    ("交易流水号", "流水号"),
    ("凭证种类", "凭证类型"),
    ("凭证号", "凭证编号"),
];

#[cfg(test)]
mod tests {
    use super::*;

    fn headers(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn normalizes_bom_spaces_and_parenthesized_query_suffixes() {
        assert_eq!(
            normalize_header_for_match("\u{feff} 证件号码（查询条件） "),
            "证件号码"
        );
        assert_eq!(normalize_header_for_match("账　卡 号"), "账卡号");
    }

    #[test]
    fn detects_account_from_alias_headers() {
        let got = headers(&["姓名(查询条件)", "证件号码", "卡号", "账号"]);
        assert_eq!(detect_fc_kind(&got), Some("fc_account"));
    }

    #[test]
    fn detects_sub_account_with_sub_account_hint() {
        let got = headers(&["开户银行", "账卡号", "子账户号"]);
        assert_eq!(detect_fc_kind(&got), Some("fc_sub_account"));
    }

    #[test]
    fn requires_minimum_schema_confidence() {
        let got = headers(&["姓名", "证件号码"]);
        assert_eq!(detect_fc_kind(&got), None);
    }
}
