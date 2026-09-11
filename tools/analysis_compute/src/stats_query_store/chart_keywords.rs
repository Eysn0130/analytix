use std::collections::HashSet;

pub(crate) fn summary_keyword_tokens(text: &str) -> Vec<String> {
    let content = text.trim();
    if content.is_empty() {
        return Vec::new();
    }
    let tokens = segment_keyword_runs(content);
    if tokens.is_empty() {
        vec![content.chars().take(24).collect()]
    } else {
        tokens
    }
}

pub(crate) fn remark_keyword_tokens(text: &str) -> Vec<String> {
    let content = normalize_keyword_text(text);
    if content.is_empty() {
        return Vec::new();
    }
    let mut tokens = Vec::new();
    for segment in segment_keyword_runs(&content) {
        tokens.extend(remark_segment_tokens(&segment));
    }
    unique_text_tokens(tokens)
}

fn remark_segment_tokens(segment: &str) -> Vec<String> {
    let content = normalize_keyword_text(segment);
    if content.is_empty() || matches!(content.as_str(), "无附言" | "未保存" | "未知") {
        return Vec::new();
    }
    if is_number_like(&content) {
        return Vec::new();
    }
    if content.chars().all(|ch| ch.is_ascii_alphanumeric()) && content.chars().count() >= 2 {
        let token = content.to_ascii_uppercase();
        if token.chars().all(|ch| ch.is_ascii_digit()) || is_remark_stopword(&token) {
            return Vec::new();
        }
        if !matches!(token.as_str(), "ATM" | "ETC" | "POS") {
            return Vec::new();
        }
        return vec![token];
    }

    let mut tokens = remark_alias_tokens(&content);
    let org_like = is_org_like_keyword_segment(&content);
    let content_len = content.chars().count();
    if (2..=4).contains(&content_len) && !org_like && !is_remark_stopword(&content) {
        tokens.push(content.clone());
    }
    for phrase in remark_keyword_phrases() {
        if content.contains(phrase) {
            tokens.push((*phrase).to_string());
        }
    }
    if !org_like {
        for suffix in remark_keyword_suffixes() {
            for candidate in suffix_candidates(&content, suffix) {
                let candidate_len = candidate.chars().count();
                if (2..=6).contains(&candidate_len) {
                    let has_other_phrase = remark_keyword_phrases()
                        .iter()
                        .any(|phrase| *phrase != *suffix && candidate.contains(*phrase));
                    if candidate != *suffix && has_other_phrase {
                        continue;
                    }
                    tokens.push(candidate);
                }
            }
        }
        if tokens.is_empty() && content_len <= 6 {
            tokens.push(content.clone());
        }
    }

    unique_text_tokens(tokens.into_iter().filter_map(|token| {
        let canonical = canonicalize_remark_keyword(&token);
        if canonical.chars().count() >= 2
            && !is_remark_stopword(&canonical)
            && !is_number_like(&canonical)
        {
            Some(canonical)
        } else {
            None
        }
    }))
}

fn normalize_keyword_text(text: &str) -> String {
    let mut content = text.trim().to_string();
    if content.is_empty() {
        return content;
    }
    content = drop_between(&content, "交易余额", "等字段");
    content = drop_credit_card_tail(&content);
    let mut out = String::with_capacity(content.len());
    for ch in content.chars() {
        if ch.is_control() || is_keyword_separator(ch) {
            out.push(' ');
        } else {
            out.push(ch);
        }
    }
    out.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn segment_keyword_runs(text: &str) -> Vec<String> {
    let mut tokens = Vec::new();
    let mut current = String::new();
    let mut current_kind = 0_u8;
    for ch in text.chars() {
        let kind = if is_cjk(ch) {
            1
        } else if ch.is_ascii_alphanumeric() {
            2
        } else {
            0
        };
        if kind == 0 {
            push_segment(&mut tokens, &mut current, current_kind);
            current_kind = 0;
            continue;
        }
        if current_kind != 0 && current_kind != kind {
            push_segment(&mut tokens, &mut current, current_kind);
        }
        current_kind = kind;
        current.push(ch);
    }
    push_segment(&mut tokens, &mut current, current_kind);
    tokens
}

fn push_segment(tokens: &mut Vec<String>, current: &mut String, kind: u8) {
    if !current.is_empty() && current.chars().count() >= 2 && (kind == 1 || kind == 2) {
        tokens.push(current.clone());
    }
    current.clear();
}

fn unique_text_tokens(tokens: impl IntoIterator<Item = String>) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut out = Vec::new();
    for token in tokens {
        let normalized = token.trim().to_string();
        if !normalized.is_empty() && seen.insert(normalized.clone()) {
            out.push(normalized);
        }
    }
    out
}

fn remark_alias_tokens(content: &str) -> Vec<String> {
    let mut tokens = Vec::new();
    for (variants, aliases) in remark_keyword_aliases() {
        if variants.iter().any(|variant| content.contains(*variant)) {
            tokens.extend(aliases.iter().map(|item| (*item).to_string()));
        }
    }
    tokens
}

fn suffix_candidates(content: &str, suffix: &str) -> Vec<String> {
    let chars = content.chars().collect::<Vec<_>>();
    let suffix_chars = suffix.chars().collect::<Vec<_>>();
    let suffix_len = suffix_chars.len();
    if suffix_len == 0 || chars.len() < suffix_len {
        return Vec::new();
    }
    let mut out = Vec::new();
    for index in 0..=chars.len() - suffix_len {
        if chars[index..index + suffix_len] == suffix_chars[..] {
            let mut start = index;
            let mut remaining = 4;
            while start > 0 && remaining > 0 && is_cjk(chars[start - 1]) {
                start -= 1;
                remaining -= 1;
            }
            out.push(chars[start..index + suffix_len].iter().collect::<String>());
        }
    }
    out
}

fn drop_between(content: &str, start: &str, end: &str) -> String {
    let mut out = content.to_string();
    while let Some(start_idx) = out.find(start) {
        let tail = &out[start_idx..];
        if let Some(end_rel) = tail.find(end) {
            let end_idx = start_idx + end_rel + end.len();
            out.replace_range(start_idx..end_idx, " ");
        } else {
            break;
        }
    }
    out
}

fn drop_credit_card_tail(content: &str) -> String {
    let marker = "信用卡尾号";
    let mut out = String::new();
    let chars = content.chars().collect::<Vec<_>>();
    let marker_chars = marker.chars().collect::<Vec<_>>();
    let mut index = 0;
    while index < chars.len() {
        if index + marker_chars.len() <= chars.len()
            && chars[index..index + marker_chars.len()] == marker_chars[..]
        {
            index += marker_chars.len();
            while index < chars.len() && chars[index].is_ascii_digit() {
                index += 1;
            }
            out.push(' ');
        } else {
            out.push(chars[index]);
            index += 1;
        }
    }
    out
}

fn is_keyword_separator(ch: char) -> bool {
    matches!(
        ch,
        '，' | ','
            | '、'
            | '；'
            | ';'
            | '：'
            | ':'
            | '/'
            | '\\'
            | '|'
            | '（'
            | '）'
            | '('
            | ')'
            | '【'
            | '】'
            | '['
            | ']'
            | '<'
            | '>'
            | '《'
            | '》'
            | '“'
            | '”'
            | '"'
            | '\''
            | '‘'
            | '’'
            | '·'
            | '•'
            | '—'
            | '-'
            | '_'
            | '+'
            | '='
    )
}

fn is_cjk(ch: char) -> bool {
    ('\u{4e00}'..='\u{9fff}').contains(&ch)
}

fn is_number_like(value: &str) -> bool {
    let mut seen_digit = false;
    let mut seen_dot = false;
    for ch in value.chars() {
        if ch.is_ascii_digit() {
            seen_digit = true;
        } else if ch == '.' && !seen_dot {
            seen_dot = true;
        } else {
            return false;
        }
    }
    seen_digit
}

fn is_org_like_keyword_segment(content: &str) -> bool {
    remark_keyword_org_hints()
        .iter()
        .any(|hint| content.contains(*hint))
}

fn canonicalize_remark_keyword(token: &str) -> String {
    remark_keyword_canonical_map()
        .iter()
        .find_map(|(source, target)| {
            if token == *source {
                Some((*target).to_string())
            } else {
                None
            }
        })
        .unwrap_or_else(|| token.trim().to_string())
}

fn is_remark_stopword(token: &str) -> bool {
    remark_keyword_stopwords().contains(&token)
}

fn remark_keyword_stopwords() -> &'static [&'static str] {
    &[
        "",
        "交易",
        "备注",
        "摘要",
        "说明",
        "用途",
        "业务",
        "账务",
        "系统",
        "当前",
        "默认",
        "银行",
        "支行",
        "账户",
        "账号",
        "卡号",
        "对方",
        "正常",
        "公司",
        "企业",
        "机构",
        "集团",
        "有限公司",
        "有限责任公司",
        "股份有限公司",
        "分公司",
        "有限公",
        "网络技术",
        "电子商务",
        "电子支付",
        "支付服务",
        "管理中心",
        "管理中",
        "本级核算",
        "本级",
        "核算",
        "综合",
        "名称",
        "代码",
        "字段",
        "等字段",
        "交易余额",
        "网点名称",
        "网点代码",
        "交易网点名称",
        "交易网点代码",
        "交易网点",
        "交易对方",
        "对方账卡号",
        "方账卡号",
        "账号开户行编码",
        "户行编码",
        "客户备付金",
        "尾号",
        "开户行",
        "户名",
        "分行",
        "营业部",
        "总行",
        "中国",
        "北京",
        "深圳市",
        "贵阳市",
        "APP",
        "其他",
        "其他款项",
        "个人",
        "特约",
        "特约商户",
        "辅助无密",
        "批处理",
        "借记卡",
        "支付平台",
        "网络支付",
        "网银平台",
        "公共交通",
        "扫二维码付款",
        "用卡约定还款",
        "用卡预约还款",
    ]
}

fn remark_keyword_canonical_map() -> &'static [(&'static str, &'static str)] {
    &[
        ("微信支付", "微信"),
        ("微信转账", "微信"),
        ("微信红包", "微信"),
        ("美团支付", "美团"),
        ("信用卡还款", "信用卡"),
        ("京东支付", "京东金融"),
        ("京东白条", "京东金融"),
        ("抖音月付", "抖音支付"),
        ("抖音红包", "抖音支付"),
        ("抖音直播收入", "抖音支付"),
        ("抖币", "抖音支付"),
        ("赎回零钱", "赎回"),
        ("收回贷款", "贷款"),
        ("买爽得宝", "爽得宝"),
        ("个人活期结息", "结息"),
        ("批量结息", "结息"),
        ("批量代扣费", "扣费"),
        ("短信扣费", "扣费"),
        ("短信扣款", "扣费"),
        ("用卡一键还款", "还款"),
        ("手机银行转账", "转账"),
        ("转账存入", "转账"),
        ("银校卡转帐", "银联转账"),
        ("银联入账", "银联转账"),
        ("网联", "平安付"),
    ]
}

fn remark_keyword_phrases() -> &'static [&'static str] {
    &[
        "手机银行转账",
        "信用卡还款",
        "工资发放",
        "工资收入",
        "工资奖金",
        "奖金发放",
        "面对面收款",
        "银联转账",
        "跨行转出",
        "电子汇入",
        "京东金融",
        "同程艺龙",
        "抖音支付",
        "项目款",
        "工程款",
        "材料款",
        "劳务费",
        "服务费",
        "手续费",
        "管理费",
        "咨询费",
        "备用金",
        "保证金",
        "过路费",
        "对付通",
        "财付通",
        "云闪付",
        "支付宝",
        "微信支付",
        "微信转账",
        "微信红包",
        "扫码付款",
        "银行卡",
        "公积金",
        "网银在线",
        "百付宝",
        "盛付通",
        "拼多多",
        "余额宝",
        "薪金煲",
        "工资",
        "奖金",
        "津贴",
        "分红",
        "税款",
        "保费",
        "押金",
        "微信",
        "货款",
        "社保",
        "房租",
        "租金",
        "物业费",
        "电费",
        "水费",
        "油费",
        "餐费",
        "路费",
        "运费",
        "差旅",
        "报销",
        "退款",
        "还款",
        "借款",
        "贷款",
        "房贷",
        "车贷",
        "结息",
        "利息",
        "本金",
        "本息",
        "消费",
        "取现",
        "提现",
        "转账",
        "汇款",
        "收款",
        "付款",
        "充值",
        "缴费",
        "定期",
        "自定义",
        "信用卡",
        "公交",
        "美团",
        "爽得宝",
        "红包",
        "申购",
        "赎回",
        "ATM",
        "POS",
        "ETC",
    ]
}

fn remark_keyword_suffixes() -> &'static [&'static str] {
    &[
        "工资发放",
        "工资收入",
        "工资奖金",
        "项目款",
        "工程款",
        "材料款",
        "货款",
        "劳务费",
        "服务费",
        "手续费",
        "管理费",
        "咨询费",
        "备用金",
        "保证金",
        "过路费",
        "路费",
        "运费",
        "差旅",
        "餐费",
        "油费",
        "房租",
        "租金",
        "物业费",
        "电费",
        "水费",
        "社保",
        "公积金",
        "税款",
        "保费",
        "押金",
        "报销",
        "退款",
        "还款",
        "借款",
        "贷款",
        "房贷",
        "车贷",
        "结息",
        "利息",
        "本金",
        "本息",
        "消费",
        "取现",
        "提现",
        "转账",
        "汇款",
        "收款",
        "付款",
        "充值",
        "缴费",
        "定期",
        "自定义",
    ]
}

fn remark_keyword_aliases() -> &'static [(&'static [&'static str], &'static [&'static str])] {
    &[
        (&["自助设备", "电子设备管理"], &["ATM"]),
        (&["贵阳市公共交通", "公共交通", "公交"], &["公交"]),
        (&["微信面对面收款"], &["微信", "面对面收款"]),
        (&["微信支付"], &["微信"]),
        (&["微信转账"], &["微信"]),
        (&["微信红包"], &["微信", "红包"]),
        (&["扫码付款", "扫二维码付款"], &["扫码付款"]),
        (&["银联转账"], &["银联转账"]),
        (&["银校卡转帐", "银联入账"], &["银联转账"]),
        (&["跨行转出"], &["跨行转出"]),
        (&["电子汇入"], &["电子汇入"]),
        (&["手机银行转账", "转账存入"], &["转账"]),
        (
            &["约定还款", "预约还款", "信用卡还款"],
            &["信用卡", "信用卡还款", "还款"],
        ),
        (&["支付宝"], &["支付宝"]),
        (&["财付通"], &["财付通"]),
        (&["云闪付"], &["云闪付"]),
        (&["网银在线"], &["网银在线"]),
        (&["百付宝"], &["百付宝"]),
        (&["盛付通"], &["盛付通"]),
        (&["拼多多"], &["拼多多"]),
        (&["美团"], &["美团"]),
        (&["京东金融", "京东支付", "京东白条"], &["京东金融"]),
        (&["同程艺龙"], &["同程艺龙"]),
        (&["余额宝"], &["余额宝"]),
        (&["薪金煲"], &["薪金煲"]),
        (
            &["抖音支付", "抖音月付", "抖音红包", "抖音直播收入", "抖币"],
            &["抖音支付"],
        ),
        (&["爽得宝", "买爽得宝"], &["爽得宝"]),
        (&["赎回零钱"], &["赎回"]),
        (&["收回贷款"], &["贷款"]),
        (&["个人活期结息", "批量结息"], &["结息"]),
        (&["批量代扣费", "短信扣费", "短信扣款"], &["扣费"]),
        (&["平安付", "网联"], &["平安付"]),
        (&["用卡一键还款"], &["还款"]),
    ]
}

fn remark_keyword_org_hints() -> &'static [&'static str] {
    &[
        "银行",
        "分行",
        "支行",
        "营业部",
        "总行",
        "有限公司",
        "有限责任公司",
        "股份有限公司",
        "分公司",
        "有限公",
        "网络技术",
        "电子商务",
        "电子支付",
        "支付服务",
        "管理中心",
        "本级核算",
        "本级",
        "核算",
        "客户备付金",
    ]
}
