from __future__ import annotations

from typing import Any, Iterable, List, Optional

CASE_TYPE_ALIASES: dict[str, str] = {
    "贷款黑灰产": "贷款黑灰产 / 帮信通道",
    "帮信通道": "贷款黑灰产 / 帮信通道",
    "虚拟币": "虚拟币 / 链上资金",
    "链上资金": "虚拟币 / 链上资金",
    "涉税骗税": "涉税犯罪",
    "涉税": "涉税犯罪",
    "商贸领域": "商贸欺诈",
}

CASE_ANALYSIS_SKELETON_BY_TYPE: dict[str, str] = {
    "合同诈骗": "fraud",
    "贷款黑灰产 / 帮信通道": "fraud",
    "保险诈骗": "fraud",
    "商贸欺诈": "fraud",
    "非法集资": "fundraising_pyramid",
    "传销": "fundraising_pyramid",
    "涉税犯罪": "tax",
    "洗钱": "anti_money_laundering",
    "地下钱庄": "anti_money_laundering",
    "虚拟币 / 链上资金": "anti_money_laundering",
    "非法经营": "anti_money_laundering",
    "职务侵占": "internal_embezzlement",
    "挪用资金": "internal_embezzlement",
    "证券期货": "securities_futures",
    "监察委案件": "supervision_duty_crime",
    "贪污贿赂（兼监察）": "supervision_duty_crime",
    "国企职务犯罪 / 工程招投标（兼监察）": "supervision_duty_crime",
}

CASE_DIRECTION_THEME_FOCUS: dict[str, dict[str, str]] = {
    "fraud": {
        "summary_accounts": "优先锁定疑似承接受害资金、快速分流或通道化最明显的账户",
        "rules": "重点确认受害资金首跳、收后即转和后续分流",
        "counterparty": "重点确认该对手是否承担受害资金首跳、中转或通道分流角色",
        "unknown": "重点借助摘要、备注和现金特征还原受害资金来源与去向",
        "same_name": "重点确认同名账户是否由同一控制人操作并参与分账或过渡",
        "trace": "重点追踪受害资金首跳、多跳分流和最终落点",
        "path": "重点核实可疑路径上的收后即转与通道分流节点",
        "drilldown": "重点核实可疑时间窗内的收后即转与分流细节",
        "shared_counterparty": "重点确认是否通过共享对手形成中转网络",
        "brief": "简报应突出受害资金首跳、通道分流和关键时间窗",
        "report": "报告应围绕受害资金首跳、多跳分流和关键通道节点组织",
        "review": "复核时应优先检查首跳去向、分流逻辑和关键时间窗是否自洽",
    },
    "fundraising_pyramid": {
        "summary_accounts": "优先锁定涉众归集、层级分发或兑付压力最明显的账户",
        "rules": "重点核实涉众归集、层级分发和兑付去向",
        "counterparty": "重点确认该对手是否承担归集、层级分发或返利兑付角色",
        "unknown": "重点还原涉众入金、返利兑付和现金归集场景",
        "same_name": "重点确认是否存在多卡归集或层级控制账户",
        "trace": "重点追踪归集账户、多跳分发和兑付去向",
        "path": "重点核实归集账户到下游分发节点的关键路径",
        "drilldown": "重点核实集中归集和分发高峰时间窗",
        "shared_counterparty": "重点核实是否存在共享归集端或层级下游",
        "brief": "简报应突出涉众归集、层级分发和兑付压力",
        "report": "报告应围绕归集账户、层级分发和兑付去向组织",
        "review": "复核时应优先检查归集、分发和兑付链条是否闭合",
    },
    "tax": {
        "summary_accounts": "优先锁定回款异常、公私混用或四流不一致最明显的账户",
        "rules": "重点核实异常交易对应的回款、开票、税负与四流一致性",
        "counterparty": "重点核实该对手是否对应销项、进项、回款或税负异常节点",
        "unknown": "重点借助摘要、备注和交易类型还原票流、货流、税流与回款关系",
        "same_name": "重点确认公私混用、资金回流和实际控制关系",
        "trace": "重点追踪回款路径、上下游流转与资金回流",
        "path": "重点核实重点路径上的票款对应和异常回流节点",
        "drilldown": "重点核实重点时间窗内的回款、票款对应和异常分拆",
        "shared_counterparty": "重点核实上下游企业、开票方和收款方是否高度重合",
        "brief": "简报应突出回款异常、公私混用和四流一致性缺口",
        "report": "报告应围绕票流、货流、税流、资金回流和税负核验组织",
        "review": "复核时应优先检查四流一致性、税负变化和异常回流是否自洽",
    },
    "anti_money_laundering": {
        "summary_accounts": "优先锁定归集分流、通道过账或落点遮挡最明显的账户",
        "rules": "重点确认归集分流、通道过账和真实落点",
        "counterparty": "重点确认该对手是否承担归集、分流或通道落点角色",
        "unknown": "重点通过摘要、备注和现金交易还原真实落点",
        "same_name": "重点确认同名账户是否参与归集、周转或回流",
        "trace": "重点追踪归集资金、多跳穿透和最终落点",
        "path": "重点核实关键路径上的归集、分流和回流节点",
        "drilldown": "重点核实关键时间窗内的快进快出和通道切换",
        "shared_counterparty": "重点核实是否存在共享归集端、通道端或落点端",
        "brief": "简报应突出归集分流、通道过账和真实落点缺口",
        "report": "报告应围绕归集、分流、通道切换和最终落点组织",
        "review": "复核时应优先检查归集分流逻辑、通道落点和回流节点是否自洽",
    },
    "internal_embezzlement": {
        "summary_accounts": "优先锁定承接单位资金、个人周转或回流最明显的账户",
        "rules": "重点核实单位资金外流后是否被个人账户承接、周转或回流",
        "counterparty": "重点核实该对手是否承接单位资金或形成个人回流",
        "unknown": "重点通过摘要、备注和现金交易确认是否存在隐匿承接或套现",
        "same_name": "重点确认同名或关联账户是否属于同一控制人并参与归集",
        "trace": "重点追踪单位资金离开账户后的承接路径和回流路径",
        "path": "重点核实单位资金外流后的承接、周转和回流节点",
        "drilldown": "重点核实敏感时间窗内的承接、周转和回流",
        "shared_counterparty": "重点核实是否存在共同承接账户、供应商或个人回流节点",
        "brief": "简报应突出单位资金外流、个人承接和回流风险",
        "report": "报告应围绕单位资金外流、个人承接、周转和回流组织",
        "review": "复核时应优先检查单位资金承接、周转和回流链路是否闭合",
    },
    "securities_futures": {
        "summary_accounts": "优先锁定保证金归集、代客理财或收益返还最明显的账户",
        "rules": "重点核实保证金归集、代客理财和收益返还",
        "counterparty": "重点确认该对手是否对应保证金账户、配资端或收益返还端",
        "unknown": "重点结合摘要、备注还原证券期货、配资和代客理财场景",
        "same_name": "重点确认是否存在自有账户腾挪或代客分账",
        "trace": "重点追踪保证金、配资资金和收益返还路径",
        "path": "重点核实保证金调拨、收益返还和异常回流节点",
        "drilldown": "重点核实异常收益、集中调拨和返还时间窗",
        "shared_counterparty": "重点核实是否存在共用券商、平台或返还节点",
        "brief": "简报应突出保证金归集、代客理财和收益返还",
        "report": "报告应围绕保证金调拨、代客理财和收益返还组织",
        "review": "复核时应优先检查保证金、收益返还和异常回流是否自洽",
    },
}

CASE_DIRECTION_THEME_FOCUS_GENERIC: dict[str, str] = {
    "summary_accounts": "优先锁定异常最明显、最值得继续下钻的账户",
    "rules": "重点确认异常交易的资金来源和后续去向",
    "counterparty": "重点确认该对手在资金链中的角色和去向",
    "unknown": "重点通过摘要、备注和交易类型还原真实场景",
    "same_name": "重点确认是否属于同一控制人、多卡归集或回流",
    "trace": "重点追踪首跳、多跳和最终落点",
    "path": "重点核实关键路径上的异常节点和时间窗",
    "drilldown": "重点核实关键时间窗内的异常细节",
    "shared_counterparty": "重点核实共享对手是否形成异常网络",
    "brief": "简报应突出当前最关键的异常事实、路径和证据缺口",
    "report": "报告应围绕关键事实、路径、风险判断和补证方向组织",
    "review": "复核时应优先检查事实、路径、风险和动作建议是否前后一致",
}

CASE_DIRECTION_TAG_FOCUS: tuple[tuple[str, str], ...] = (
    ("资金穿透", "首跳和多跳路径"),
    ("第三方支付", "第三方支付通道后的真实对手"),
    ("通道", "通道后的真实落点"),
    ("归集账户", "归集分流结构"),
    ("实控人", "实际控制关系"),
    ("虚开发票", "票流、货流、税流与回款一致性"),
    ("发票", "票流、货流、税流与回款一致性"),
    ("税务稽查", "税负与回款核验"),
    ("四流", "票流、货流、税流与回款一致性"),
    ("虚拟币", "链上转移与出入金口"),
    ("链上资金", "链上转移与出入金口"),
)


def _trim_text(value: Any) -> str:
    return str(value or "").strip()


def normalize_case_type(value: object) -> str:
    raw = str(value or "").strip()
    if not raw:
        return ""
    return CASE_TYPE_ALIASES.get(raw, raw)


def normalize_case_tags(value: object) -> List[str]:
    if isinstance(value, str):
        items: Iterable[object] = value.replace("，", ",").replace("；", ",").split(",")
    elif isinstance(value, list):
        items = value
    elif value is None:
        items = []
    else:
        items = [value]

    dedup: dict[str, None] = {}
    for item in items:
        normalized = str(item or "").strip()
        if normalized:
            dedup.setdefault(normalized, None)
        if len(dedup) >= 32:
            break
    return list(dedup.keys())


def resolve_case_analysis_skeleton(case_type: object) -> Optional[str]:
    normalized = normalize_case_type(case_type)
    if not normalized:
        return None
    return CASE_ANALYSIS_SKELETON_BY_TYPE.get(normalized)


def infer_case_analysis_skeleton_from_text(direction_source: object, tags: object) -> Optional[str]:
    normalized_source = _trim_text(direction_source)
    normalized_tags = normalize_case_tags(tags)
    text = " ".join([normalized_source, *normalized_tags]).strip()
    if not text:
        return None
    if any(token in text for token in ("涉税", "税务", "虚开", "骗税", "发票", "税票", "四流不一致", "票货分离")):
        return "tax"
    if any(token in text for token in ("非法集资", "传销", "涉众", "资金池")):
        return "fundraising_pyramid"
    if any(token in text for token in ("洗钱", "地下钱庄", "虚拟币", "链上资金", "USDT", "OTC", "币商", "资金转移", "过账", "分流")):
        return "anti_money_laundering"
    if any(token in text for token in ("职务侵占", "挪用资金")):
        return "internal_embezzlement"
    if any(token in text for token in ("证券", "期货")):
        return "securities_futures"
    if any(token in text for token in ("合同诈骗", "贷款黑灰产", "帮信", "保险诈骗", "商贸欺诈", "诈骗", "电诈", "被骗", "受害资金", "涉诈")):
        return "fraud"
    return "generic_ecrime"


def case_direction_label_for_skeleton(skeleton_key: object) -> str:
    normalized = _trim_text(skeleton_key)
    label_by_skeleton = {
        "fraud": "涉诈案件研判方向",
        "fundraising_pyramid": "非法集资 / 传销案件研判方向",
        "tax": "涉税案件研判方向",
        "anti_money_laundering": "反洗钱 / 资金转移研判方向",
        "internal_embezzlement": "企业内部侵财案件研判方向",
        "securities_futures": "证券期货案件研判方向",
        "supervision_duty_crime": "监察 / 职务犯罪案件研判方向",
    }
    return label_by_skeleton.get(normalized, "经侦通用研判方向")


def build_case_direction_context(case_type: object, tags: object, note: object = None) -> dict[str, Any]:
    normalized_case_type = normalize_case_type(case_type)
    normalized_tags = normalize_case_tags(tags)
    normalized_note = _trim_text(note)
    direction_source = " ".join([normalized_case_type, normalized_note]).strip()
    skeleton_key = str(
        resolve_case_analysis_skeleton(normalized_case_type)
        or infer_case_analysis_skeleton_from_text(direction_source, normalized_tags)
        or ""
    ).strip()
    label = (
        f"{normalized_case_type}案件研判方向"
        if normalized_case_type
        else case_direction_label_for_skeleton(skeleton_key)
    )
    return {
        "case_type": normalized_case_type,
        "tags": normalized_tags,
        "note": normalized_note,
        "skeleton_key": skeleton_key,
        "label": label,
    }


def build_case_direction_focus(case_type: object, tags: object, theme: str, note: object = None) -> str:
    ctx = build_case_direction_context(case_type, tags, note)
    skeleton_key = str(ctx.get("skeleton_key") or "").strip()
    base_focus = (
        CASE_DIRECTION_THEME_FOCUS.get(skeleton_key, {}).get(theme)
        or CASE_DIRECTION_THEME_FOCUS_GENERIC.get(theme, "")
    )
    extras: list[str] = []
    for keyword, phrase in CASE_DIRECTION_TAG_FOCUS:
        if any(keyword in tag for tag in list(ctx.get("tags") or [])) and phrase not in base_focus and phrase not in extras:
            extras.append(phrase)
        if len(extras) >= 2:
            break
    if extras:
        if base_focus:
            return f"{base_focus}，并补看{'、'.join(extras)}"
        return f"重点补看{'、'.join(extras)}"
    return base_focus
