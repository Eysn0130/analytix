from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List, Sequence


@dataclass(frozen=True)
class LocalFieldMappingRequest:
    file_name: str
    kind: str
    headers: List[str]
    target_keys: List[str]
    target_labels: Dict[str, str]
    samples_by_header: Dict[str, List[str]]


@dataclass(frozen=True)
class LocalFieldMappingResponse:
    available: bool
    mappings: Dict[str, str] = field(default_factory=dict)
    message: str = ""


class LocalFieldMappingModel:
    """Constrained llama.cpp adapter for local field-mapping classification.

    The model is only allowed to choose from supplied target keys and source headers.
    Callers must still validate returned mappings against column profiles before applying.
    """

    def __init__(
        self,
        *,
        binary_path: Path | None = None,
        model_path: Path | None = None,
        timeout_seconds: int = 20,
    ) -> None:
        # Configuration cannot become a capability. Do not retain or inspect
        # caller/environment paths while this consumer has no host admission.
        del binary_path, model_path
        self._timeout_seconds = max(1, int(timeout_seconds or 20))

    def suggest(self, request: LocalFieldMappingRequest) -> LocalFieldMappingResponse:
        del request
        # This adapter used to launch an arbitrary configured executable and
        # send case file names, headers, account/card samples, and model paths
        # over stdin/argv. Do not even probe those configured paths here: a
        # sanitized environment or an existing file does not prove executable,
        # model, working-directory, filesystem, or descendant authority. Keep
        # the optional classifier mechanically unavailable until a package-
        # owned execution adapter binds all of those objects and admits the
        # request before receiving case data. Deterministic mapping continues
        # through the caller's non-model validation path.
        return LocalFieldMappingResponse(
            available=False,
            message="local field mapping execution authority is unavailable",
        )


def _build_prompt(request: LocalFieldMappingRequest) -> str:
    targets = ", ".join(
        f"{index}={key}/{_target_label_with_hint(key, request.target_labels.get(key, key))}"
        for index, key in enumerate(request.target_keys)
    )
    columns = "; ".join(
        f"{index}={header} samples: {', '.join(request.samples_by_header.get(header, [])[:8])}"
        for index, header in enumerate(request.headers)
    )
    return (
        "你是字段映射分类器，只返回 JSON。"
        "任务：用索引把每个 target 映射到最合适的 source；不确定就省略该项。"
        "规则：账号/卡号样本映射账号字段；日期时间样本映射时间字段；金额/正负数样本映射金额字段。"
        f"文件：{request.file_name}；类型：{request.kind}。"
        f"Target indexes: {targets}。"
        f"Source indexes: {columns}。"
        '只输出形如 {"mappings":[{"target":0,"source":1}]} 的 JSON，target/source 必须是上述整数索引。'
    )


def _target_label_with_hint(key: str, label: str) -> str:
    hints = {
        "acct_no": "账号/卡号/账户",
        "card_no": "卡号/银行卡号",
        "txn_time": "交易日期/日期列/记账时间/发生时间",
        "amount": "发生额/收入/支出/收支/借方/贷方/正负数",
        "dc_flag": "收付标志/借贷标识/进出方向",
        "balance": "余额/账户余额",
        "counterparty_acct": "对方账号/对手账号",
        "counterparty_name": "对方户名/对手名称",
        "summary": "摘要/交易用途/交易说明",
        "currency": "币种/币别/货币/CNY/USD/人民币",
        "cash_flag": "现金标识/现钞/现汇",
        "is_success": "交易结果/成功/失败",
        "branch_code": "交易网点代码/机构代码/网点号",
        "terminal_no": "终端号/设备编号/自助设备编号",
        "merchant_name": "商户名称/特约商户",
        "merchant_no": "商户号/商户编号/特约商户号",
        "txn_type": "交易种类/业务类型/业务种类",
    }
    normalized_key = str(key or "").strip()
    base_label = str(label or normalized_key).strip()
    hint = hints.get(normalized_key, "")
    return f"{base_label}/{hint}" if hint and hint not in base_label else base_label


def _build_json_schema() -> str:
    # This schema is intentionally request-independent because llama.cpp accepts
    # it through argv. Case headers and samples travel only through anonymous
    # stdin; host validation below resolves indexes back to the request values.
    return json.dumps(
        {
            "type": "object",
            "properties": {
                "mappings": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "target": {"type": "integer", "minimum": 0},
                            "source": {"type": "integer", "minimum": 0},
                        },
                        "required": ["target", "source"],
                        "additionalProperties": False,
                    },
                    "maxItems": 128,
                }
            },
            "required": ["mappings"],
            "additionalProperties": False,
        },
        ensure_ascii=True,
        separators=(",", ":"),
    )


def _extract_json_object(text: str) -> object:
    value = str(text or "").strip()
    parsed_objects: list[dict[str, object]] = []
    mapping_payload: dict[str, object] | None = None
    for candidate in _iter_json_object_strings(value):
        try:
            payload = json.loads(candidate)
        except json.JSONDecodeError:
            continue
        if not isinstance(payload, dict):
            continue
        parsed_objects.append(payload)
        if isinstance(payload.get("mappings"), list):
            mapping_payload = payload
    if mapping_payload is not None:
        return mapping_payload
    return parsed_objects[-1] if parsed_objects else None


def _iter_json_object_strings(text: str) -> Sequence[str]:
    objects: list[str] = []
    start: int | None = None
    depth = 0
    in_string = False
    escaped = False
    for index, char in enumerate(text):
        if in_string:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == '"':
                in_string = False
            continue
        if char == '"':
            in_string = True
            continue
        if char == "{":
            if depth == 0:
                start = index
            depth += 1
            continue
        if char == "}" and depth > 0:
            depth -= 1
            if depth == 0 and start is not None:
                objects.append(text[start : index + 1])
                start = None
    return objects
