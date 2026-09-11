from __future__ import annotations

from typing import Sequence


TRANSACTION_MISSING_WARNING_IGNORED_HEADERS = {
    "账户开户名称",
    "开户人证件号码",
    "交易对手账卡号",
    "交易网点代码",
    "终端号",
    "商户名称",
    "商户号",
}


def transaction_missing_headers_for_warning(mapping_missing: Sequence[str]) -> list[str]:
    return [
        str(item or "").strip()
        for item in mapping_missing
        if str(item or "").strip() and str(item or "").strip() not in TRANSACTION_MISSING_WARNING_IGNORED_HEADERS
    ]
