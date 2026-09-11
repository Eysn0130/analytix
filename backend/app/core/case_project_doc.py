from __future__ import annotations

import hashlib
from collections.abc import Iterable
from pathlib import Path
import re
from typing import Any

from app.core.paths import ensure_dir


CASE_PROJECT_DOC_FILENAME = "简要案情.md"
CASE_DIRECTION_DOC_FILENAME = "研判方向.md"
CASE_RUNTIME_PROJECT_DOC_FILENAME = ".analytix/project-doc.md"
CASE_PROJECT_DOC_FALLBACK_FILENAMES = (CASE_RUNTIME_PROJECT_DOC_FILENAME,)
CASE_PROJECT_DOC_DOCUMENT_ID = "__case_project_doc_brief__"
CASE_PROJECT_DOC_FILE_ID = "__case_project_doc_brief__"
CASE_DIRECTION_DOC_DOCUMENT_ID = "__case_project_doc_direction__"
CASE_DIRECTION_DOC_FILE_ID = "__case_project_doc_direction__"
CASE_PROJECT_DOC_KIND = "case_project_doc"
CASE_PROJECT_DOC_PARSER = "codex_project_doc"
CASE_PROJECT_DOC_METADATA_BEGIN = "<!-- ANALYTIX:CASE-METADATA:BEGIN -->"
CASE_PROJECT_DOC_METADATA_END = "<!-- ANALYTIX:CASE-METADATA:END -->"
_DEFAULT_DIRECTION_PLACEHOLDER = "请在这里维护本案的人工研判方向、关键假设、待核实问题和下一步动作。"
_CASE_PROJECT_DOC_EDIT_INTENT_RE = re.compile(
    r"(?:删除|删掉|移除|修改|编辑|更新|写入|追加|改写|重写|清空|清除|替换|改成|改为|调整|补充)",
    re.IGNORECASE,
)
_CASE_PROJECT_DOC_EDIT_ALIASES = {
    CASE_PROJECT_DOC_FILENAME: (CASE_PROJECT_DOC_FILENAME,),
    CASE_DIRECTION_DOC_FILENAME: (
        CASE_DIRECTION_DOC_FILENAME,
        "研判方向文件",
        "研判方向",
    ),
}


def required_case_project_doc_file_change_paths(latest_user_message: str) -> list[str]:
    normalized = re.sub(r"\s+", "", str(latest_user_message or "").strip())
    if not normalized or not _CASE_PROJECT_DOC_EDIT_INTENT_RE.search(normalized):
        return []
    required: list[str] = []
    for filename, aliases in _CASE_PROJECT_DOC_EDIT_ALIASES.items():
        if any(alias in normalized for alias in aliases):
            required.append(filename)
    return required


def looks_like_case_project_doc_file_edit_request(latest_user_message: str) -> bool:
    return bool(required_case_project_doc_file_change_paths(latest_user_message))


def case_project_doc_path(case_dir: Path) -> Path:
    return Path(case_dir) / CASE_PROJECT_DOC_FILENAME


def case_direction_doc_path(case_dir: Path) -> Path:
    return Path(case_dir) / CASE_DIRECTION_DOC_FILENAME


def case_runtime_project_doc_path(case_dir: Path) -> Path:
    return Path(case_dir) / CASE_RUNTIME_PROJECT_DOC_FILENAME


def read_case_project_doc(case_dir: Path, *, limit: int | None = None) -> str:
    path = case_project_doc_path(case_dir)
    if not path.exists():
        return ""
    text = path.read_text(encoding="utf-8", errors="ignore")
    if limit is None:
        return text
    return text[: max(0, int(limit))]


def read_case_direction_doc(case_dir: Path, *, limit: int | None = None) -> str:
    path = case_direction_doc_path(case_dir)
    if not path.exists():
        return ""
    text = path.read_text(encoding="utf-8", errors="ignore")
    if limit is None:
        return text
    return text[: max(0, int(limit))]


def case_project_doc_sha256(case_dir: Path) -> str:
    path = case_project_doc_path(case_dir)
    if not path.exists():
        return ""
    try:
        return hashlib.sha256(path.read_bytes()).hexdigest()
    except OSError:
        return ""


def case_project_docs_sha256(case_dir: Path) -> str:
    hash_value = hashlib.sha256()
    found = False
    for path in (
        case_project_doc_path(case_dir),
        case_direction_doc_path(case_dir),
        case_runtime_project_doc_path(case_dir),
    ):
        if not path.exists():
            continue
        try:
            content = path.read_bytes()
        except OSError:
            continue
        hash_value.update(str(path.relative_to(case_dir)).encode("utf-8", errors="ignore"))
        hash_value.update(b"\0")
        hash_value.update(content)
        hash_value.update(b"\0")
        found = True
    return hash_value.hexdigest() if found else ""


def build_case_project_doc_content(case: Any) -> str:
    return "\n".join(
        [
            "# 简要案情",
            "",
            build_case_project_doc_metadata_block(case),
            "",
            "## 协作说明",
            "",
            f"- 稳定案件背景维护在本文件；可调整的研判主线维护在 `{CASE_DIRECTION_DOC_FILENAME}`。",
            f"- Codex runtime 会通过生成的 `{CASE_RUNTIME_PROJECT_DOC_FILENAME}` 同时读取本文件和 `{CASE_DIRECTION_DOC_FILENAME}`。",
            "- 请不要把研判方向、候选对象或人工假设直接写成证据事实。",
            "",
            "## 补充说明",
            "",
            "可继续追加模型需要优先理解的稳定案件背景。",
            "",
        ]
    )


def build_case_direction_doc_content(case: Any, *, migrated_direction: str = "") -> str:
    direction_text = _normalize_migrated_direction(migrated_direction)
    current_mainline = direction_text or "- 待补充。"
    return "\n".join(
        [
            "# 研判方向",
            "",
            "> 本文件由人工和 agent 共同维护，记录当前可调整的分析方向、关键假设和下一步动作；它不是证据事实。",
            "",
            "## 当前主线",
            "",
            current_mainline,
            "",
            "## 关键假设",
            "",
            "- 待补充。",
            "",
            "## 重点对象与范围",
            "",
            "- 待补充。",
            "",
            "## 待核实问题",
            "",
            "- 待补充。",
            "",
            "## 下一步动作",
            "",
            "- 待补充。",
            "",
            "## 证据边界",
            "",
            "- 只有结构化工具、资金数据、证据包或报告引用支持的内容，才能进入事实区。",
            "- bootstrap / candidate 对象只能作为线索；确认前不得写成已确认重点对象。",
            "",
        ]
    )


def build_case_project_doc_metadata_block(case: Any) -> str:
    tags = _normalize_tags(_case_value(case, "tags"))
    note = _case_text(case, "summary", "note")
    lines = [
        CASE_PROJECT_DOC_METADATA_BEGIN,
        "## 案件页元数据（自动同步）",
        "",
        f"- 案件 ID：{_case_text(case, 'case_id') or '未填写'}",
        f"- 案件名称：{_case_text(case, 'name', 'case_name') or '未填写'}",
        f"- 案件编号：{_case_text(case, 'case_no', 'case_number') or '未填写'}",
        f"- 案件类别：{_case_text(case, 'case_type') or '未填写'}",
        f"- 类别标签：{('，'.join(tags) if tags else '未填写')}",
        f"- 案件状态：{_case_text(case, 'status') or '未填写'}",
        "",
        "### 备注",
        "",
        note or "未填写",
        "",
        "### 使用约束",
        "",
        "- 本区块来自案件页登记元数据、人工备注或保持本格式的 agent 更新。",
        "- 如需修改本区块，请保留本标记区、标题和字段列表格式；案件页会按本格式同步。",
        f"- 本文件名固定为 `{CASE_PROJECT_DOC_FILENAME}`；需要复查本案项目文档时读取本文件。",
        f"- 可调整的研判主线维护在 `{CASE_DIRECTION_DOC_FILENAME}`。",
        f"- Codex runtime 实际读取的项目文档入口由 `{CASE_RUNTIME_PROJECT_DOC_FILENAME}` 组合生成。",
        "- 本文件不替代证据、资金数据、工具结果或审计引用。",
        CASE_PROJECT_DOC_METADATA_END,
    ]
    return "\n".join(lines)


def parse_case_project_doc_metadata(text: str) -> dict[str, Any]:
    block = _metadata_block_text(text)
    if not block:
        return {}

    fields: dict[str, Any] = {}
    for line in block.splitlines():
        stripped = line.strip()
        if not stripped.startswith("- "):
            continue
        label, separator, value = stripped[2:].partition("：")
        if not separator:
            label, separator, value = stripped[2:].partition(":")
        if not separator:
            continue
        normalized_value = _blank_if_unfilled(value)
        if label == "案件 ID":
            fields["case_id"] = normalized_value
        elif label == "案件名称":
            fields["name"] = normalized_value
        elif label == "案件编号":
            fields["case_no"] = normalized_value
        elif label == "案件类别":
            fields["case_type"] = normalized_value
        elif label == "类别标签":
            fields["tags"] = _normalize_tags(normalized_value)
        elif label == "案件状态":
            fields["status"] = normalized_value or "进行中"

    note = _metadata_section_text(block, "### 备注")
    if note is not None:
        fields["summary"] = note
    return fields


def sync_case_project_doc(case_dir: Path, case: Any) -> Path:
    path = case_project_doc_path(case_dir)
    ensure_dir(path.parent)
    current = ""
    if path.exists():
        try:
            current = path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            current = ""
    direction_path = case_direction_doc_path(case_dir)
    migrated_direction = _extract_markdown_section(current, "## 研判方向")
    direction_current = ""
    if direction_path.exists():
        try:
            direction_current = direction_path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            direction_current = ""
    if not direction_path.exists() or (
        _normalize_migrated_direction(migrated_direction) and _direction_doc_is_placeholder(direction_current)
    ):
        ensure_dir(direction_path.parent)
        direction_path.write_text(
            build_case_direction_doc_content(case, migrated_direction=migrated_direction),
            encoding="utf-8",
        )
    content = merge_case_project_doc_content(current, case) if current.strip() else build_case_project_doc_content(case)
    content = _remove_markdown_section(content, "## 研判方向")
    if current != content:
        path.write_text(content, encoding="utf-8")
    sync_case_runtime_project_doc(case_dir)
    return path


def sync_case_runtime_project_doc(case_dir: Path) -> Path:
    runtime_path = case_runtime_project_doc_path(case_dir)
    ensure_dir(runtime_path.parent)
    brief_text = read_case_project_doc(case_dir)
    direction_text = read_case_direction_doc(case_dir)
    content = build_case_runtime_project_doc_content(
        brief_text=brief_text,
        direction_text=direction_text,
    )
    current = ""
    if runtime_path.exists():
        try:
            current = runtime_path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            current = ""
    if current != content:
        runtime_path.write_text(content, encoding="utf-8")
    return runtime_path


def build_case_runtime_project_doc_content(*, brief_text: str, direction_text: str) -> str:
    return "\n".join(
        [
            "# Analytix 案件项目文档",
            "",
            "本文件由 Analytix 根据案件目录中的可编辑文档生成，供 Codex 原生 project-doc 机制读取。",
            f"- 稳定背景来源：`{CASE_PROJECT_DOC_FILENAME}`",
            f"- 当前研判方向来源：`{CASE_DIRECTION_DOC_FILENAME}`",
            "- 研判方向、候选对象和人工假设均不是证据事实；事实必须来自结构化工具、资金数据或审计引用。",
            "",
            f"## {CASE_PROJECT_DOC_FILENAME}",
            "",
            str(brief_text or "").strip() or "未填写。",
            "",
            f"## {CASE_DIRECTION_DOC_FILENAME}",
            "",
            str(direction_text or "").strip() or "未填写。",
            "",
        ]
    )


def merge_case_project_doc_content(current: str, case: Any) -> str:
    metadata_block = build_case_project_doc_metadata_block(case)
    text = str(current or "")
    begin_index = text.find(CASE_PROJECT_DOC_METADATA_BEGIN)
    end_index = text.find(CASE_PROJECT_DOC_METADATA_END)
    if begin_index >= 0 and end_index >= begin_index:
        end_index += len(CASE_PROJECT_DOC_METADATA_END)
        return f"{text[:begin_index]}{metadata_block}{text[end_index:]}"

    stripped = text.strip()
    if not stripped:
        return build_case_project_doc_content(case)
    lines = stripped.splitlines()
    if lines and lines[0].lstrip().startswith("# "):
        return "\n".join([lines[0], "", metadata_block, "", *lines[1:]]).rstrip() + "\n"
    return "\n".join(["# 简要案情", "", metadata_block, "", stripped]).rstrip() + "\n"


def _extract_markdown_section(text: str, heading: str) -> str:
    lines = str(text or "").splitlines()
    collected: list[str] = []
    in_section = False
    for line in lines:
        stripped = line.strip()
        if stripped == heading:
            in_section = True
            continue
        if in_section and stripped.startswith("## "):
            break
        if in_section:
            collected.append(line.rstrip())
    while collected and not collected[0].strip():
        collected.pop(0)
    while collected and not collected[-1].strip():
        collected.pop()
    return "\n".join(collected).strip()


def _remove_markdown_section(text: str, heading: str) -> str:
    lines = str(text or "").splitlines()
    output: list[str] = []
    in_section = False
    removed = False
    for line in lines:
        stripped = line.strip()
        if stripped == heading:
            in_section = True
            removed = True
            continue
        if in_section and stripped.startswith("## "):
            in_section = False
        if not in_section:
            output.append(line.rstrip())
    result = "\n".join(output).rstrip() + "\n"
    return result if removed else str(text or "")


def _normalize_migrated_direction(value: str) -> str:
    text = str(value or "").strip()
    if not text or text == _DEFAULT_DIRECTION_PLACEHOLDER:
        return ""
    return text


def _direction_doc_is_placeholder(value: str) -> bool:
    text = str(value or "").strip()
    if not text:
        return True
    meaningful_lines = [
        line.strip()
        for line in text.splitlines()
        if line.strip()
        and not line.strip().startswith("#")
        and not line.strip().startswith(">")
        and line.strip() not in {"- 待补充。"}
    ]
    return not meaningful_lines


def _metadata_block_text(text: str) -> str:
    source = str(text or "")
    begin_index = source.find(CASE_PROJECT_DOC_METADATA_BEGIN)
    end_index = source.find(CASE_PROJECT_DOC_METADATA_END)
    if begin_index < 0 or end_index < begin_index:
        return ""
    begin_index += len(CASE_PROJECT_DOC_METADATA_BEGIN)
    return source[begin_index:end_index]


def _metadata_section_text(block: str, heading: str) -> str | None:
    lines = str(block or "").splitlines()
    collected: list[str] = []
    in_section = False
    for line in lines:
        stripped = line.strip()
        if stripped == heading:
            in_section = True
            continue
        if in_section and stripped.startswith("### "):
            break
        if in_section:
            collected.append(line.rstrip())

    if not in_section:
        return None

    while collected and not collected[0].strip():
        collected.pop(0)
    while collected and not collected[-1].strip():
        collected.pop()
    return _blank_if_unfilled("\n".join(collected))


def _blank_if_unfilled(value: Any) -> str:
    text = str(value or "").strip()
    return "" if text == "未填写" else text


def _case_value(case: Any, *names: str) -> Any:
    for name in names:
        if isinstance(case, dict) and name in case:
            return case.get(name)
        if hasattr(case, name):
            return getattr(case, name)
    return None


def _case_text(case: Any, *names: str) -> str:
    for name in names:
        value = _case_value(case, name)
        if value is not None:
            return str(value or "").strip()
    return ""


def _normalize_tags(raw: Any) -> list[str]:
    if raw is None:
        return []
    if isinstance(raw, str):
        values: Iterable[Any] = raw.replace("，", ",").split(",")
    elif isinstance(raw, Iterable):
        values = raw
    else:
        values = (raw,)
    result: list[str] = []
    for item in values:
        text = str(item or "").strip()
        if text and text not in result:
            result.append(text)
    return result
