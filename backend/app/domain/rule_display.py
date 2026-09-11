from __future__ import annotations


RULE_CODE_DISPLAY_TITLES: dict[str, str] = {
    "FAST_IN_FAST_OUT": "短时快进快出",
    "HIGH_FREQ_SMALL_OUT": "高频小额转出",
    "THRESHOLD_SPLIT": "临界额拆分",
    "SHARED_IP_MULTI_ACCOUNT": "同 IP 多账户",
    "SHARED_MAC_MULTI_ACCOUNT": "同 MAC 多账户",
    "COUNTERPARTY_CONCENTRATION": "对手集中度异常",
    "FAILED_RETRY_PROBE": "失败后重试探测",
}


def rule_code_display_text(value: object) -> str:
    normalized = str(value or "").strip().upper()
    if normalized in RULE_CODE_DISPLAY_TITLES:
        return RULE_CODE_DISPLAY_TITLES[normalized]
    return str(value or "").strip() or "未知规则"
