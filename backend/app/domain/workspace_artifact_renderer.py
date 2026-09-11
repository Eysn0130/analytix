from __future__ import annotations

import math
import re
from typing import Any, Iterable, Sequence


_QUERY_REF_RE = re.compile(r"^sqlquery_v1_[a-f0-9]{64}$")
_QUERY_CATEGORIES = frozenset(
    {"bounded_query", "diagnostic", "bounded_preview", "schema_profile", "other"}
)
_ISO_TIMESTAMP_RE = re.compile(
    r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$"
)

def _trim(value: Any) -> str:
    return str(value or "").strip()


def _trim_list(values: Iterable[Any]) -> list[str]:
    return [_trim(item) for item in values if _trim(item)]


def _optional_nonnegative_int(value: Any) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    if isinstance(value, str):
        value = value.strip()
        if not re.fullmatch(r"\d+", value):
            return None
    try:
        number = int(value)
    except (TypeError, ValueError, OverflowError):
        return None
    if number < 0 or isinstance(value, float) and not value.is_integer():
        return None
    return number


def _count_text(value: Any) -> str:
    number = _optional_nonnegative_int(value)
    return str(number) if number is not None else "未返回"


def _confidence_text(value: Any) -> str:
    if value is None or isinstance(value, bool):
        return "未核验"
    if isinstance(value, str) and not value.strip():
        return "未核验"
    try:
        number = float(value)
    except (TypeError, ValueError, OverflowError):
        return "未核验"
    if not math.isfinite(number) or number < 0 or number > 1:
        return "未核验"
    rounded = round(number, 2)
    return "0" if rounded == 0 else str(rounded)


def _optional_bool(value: Any) -> bool | None:
    if isinstance(value, bool):
        return value
    if isinstance(value, int) and value in {0, 1}:
        return bool(value)
    if isinstance(value, str):
        normalized = value.strip().lower()
        if normalized in {"true", "1"}:
            return True
        if normalized in {"false", "0"}:
            return False
    return None


def _revalidation_text(value: Any) -> str:
    required = _optional_bool(value)
    if required is True:
        return "需结合当前证据复核"
    if required is False:
        return "已明确标记为无需复核的工作指引"
    return "复核状态未核验；不得作为案件事实"


class WorkspaceArtifactRenderer:
    """Renders human-readable workspace artifacts.

    These markdown files are presentation artifacts only. They should be
    rendered from structured service/repository outputs and never feed back
    into factual decision paths.
    """

    def render_case(
        self,
        *,
        case_payload: dict[str, Any],
        tags: Sequence[str],
        scope_summary: dict[str, int],
        semantic_scope_targets: dict[str, Any],
        hypothesis_count: int,
        finding_count: int,
        stats_payload: dict[str, Any],
    ) -> str:
        counts = dict(semantic_scope_targets.get("counts") or {})
        confirmed_targets = list(semantic_scope_targets.get("confirmed_targets") or [])
        candidate_targets = list(semantic_scope_targets.get("candidate_targets") or [])
        bootstrap_targets = list(semantic_scope_targets.get("bootstrap_targets") or [])
        lines = [
            "# CASE",
            "",
            "## 案件概况",
            f"- 案件名称：{_trim(case_payload.get('case_name')) or '未命名案件'}",
            f"- 案号：{_trim(case_payload.get('case_number')) or '待补充'}",
            f"- 案件类型：{_trim(case_payload.get('case_type')) or '待补充'}",
            f"- 负责人：{_trim(case_payload.get('owner')) or '待补充'}",
            f"- 状态：{_trim(case_payload.get('status')) or '未核验'}",
            f"- 标签：{'、'.join(_trim_list(tags)) or '待补充'}",
            "",
            "## 当前备注",
            _trim(case_payload.get("note")) or "暂无案件备注。",
            "",
            "## 线索范围",
            f"- 已确认账户：{_count_text(counts.get('confirmed'))}",
            f"- 候选账户：{_count_text(counts.get('candidate'))}",
            f"- 自动引导候选：{_count_text(counts.get('bootstrap'))}",
            f"- 人员线索：{_count_text(scope_summary.get('person_clues'))}",
            f"- 设备线索：{_count_text(scope_summary.get('device_clues'))}",
            f"- 时间窗线索：{_count_text(scope_summary.get('time_range_clues'))}",
            "",
            "## 已确认对象",
        ]
        if confirmed_targets:
            lines.extend([f"- {_trim(item.get('display_name')) or '未命名对象'}" for item in confirmed_targets])
        else:
            lines.append("- 暂无已确认对象。")
        lines.extend(["", "## 候选对象"])
        if candidate_targets:
            for item in candidate_targets[:8]:
                source = _trim(item.get("source")) or "candidate"
                label = "自动引导" if source == "auto_txn_bootstrap" else "候选"
                lines.append(f"- {_trim(item.get('display_name')) or '未命名对象'}（{label}）")
        else:
            lines.append("- 暂无候选对象。")
        if bootstrap_targets:
            lines.extend(
                [
                    "",
                    "## 说明",
                    f"- 当前存在 {len(bootstrap_targets)} 个自动引导候选对象，仅用于排查起点，不代表已确认重点账户。",
                ]
            )
        lines.extend(
            [
                "",
                "## 当前进度",
                f"- 交易总量：{_count_text(stats_payload.get('transactions'))}",
                f"- 账户总量：{_count_text(stats_payload.get('accounts'))}",
                f"- 假设条数：{_count_text(hypothesis_count)}",
                f"- 结构化发现：{_count_text(finding_count)}",
                "",
                "## 初始侦查方向",
                _trim(case_payload.get("initial_direction")) or "先做全案扫雷，再确定后续穿透入口。",
            ]
        )
        return "\n".join(lines).strip() + "\n"

    def render_timeline(self, timeline_items: Sequence[dict[str, str]]) -> str:
        lines = ["# TIMELINE", ""]
        if not timeline_items:
            lines.append("- 暂无关键时间线。")
            return "\n".join(lines).strip() + "\n"
        for item in timeline_items:
            lines.append(f"- {_trim(item.get('time')) or '未知时间'} · {_trim(item.get('label')) or '未命名事件'}")
        return "\n".join(lines).strip() + "\n"

    def render_hypotheses(self, hypotheses: Sequence[dict[str, Any]]) -> str:
        lines = ["# HYPOTHESES", ""]
        if not hypotheses:
            lines.append("当前暂无结构化假设。")
            return "\n".join(lines).strip() + "\n"
        for item in hypotheses:
            lines.extend(
                [
                    f"## {_trim(item.get('title')) or '未命名假设'}",
                    f"- 状态：{_trim(item.get('status')) or '未核验'}",
                    f"- 置信度：{_confidence_text(item.get('confidence'))}",
                    f"- 更新时间：{_trim(item.get('updated_at') or item.get('created_at')) or '未知'}",
                    "",
                    _trim(item.get("content_md")) or "暂无描述。",
                    "",
                ]
            )
        return "\n".join(lines).strip() + "\n"

    def render_evidence_index(
        self,
        evidence_rows: Sequence[dict[str, Any]],
        query_rows: Sequence[dict[str, Any]],
    ) -> str:
        lines = ["# EVIDENCE_INDEX", ""]
        if evidence_rows:
            lines.append("## 证据索引")
            for row in evidence_rows[:20]:
                lines.append(
                    f"- `{_trim(row.get('evidence_id'))}` · {_trim(row.get('title')) or '未命名证据'} "
                    f"(`{_trim(row.get('ref_table'))}` / `{_trim(row.get('ref_pk'))}`)"
                )
            lines.append("")
        if query_rows:
            lines.append("## 最近查询")
            for row in query_rows[:10]:
                query_ref = _trim(row.get("query_id"))
                if _QUERY_REF_RE.fullmatch(query_ref) is None:
                    query_ref = "query-unavailable"
                tool_name = _trim(row.get("tool_name"))
                if tool_name not in _QUERY_CATEGORIES:
                    tool_name = "other"
                created_at = _trim(row.get("created_at"))
                if _ISO_TIMESTAMP_RE.fullmatch(created_at) is None:
                    created_at = "未知时间"
                lines.append(
                    f"- `{query_ref}` · {tool_name} · 未检查 · {created_at}"
                )
        elif not evidence_rows:
            lines.append("当前暂无证据与查询记录。")
        return "\n".join(lines).strip() + "\n"

    def render_workspace_memory(self, notes: Sequence[dict[str, Any]]) -> str:
        type_labels = {
            "workflow_feedback": "工作流反馈",
            "workspace_reference": "工作区参考",
            "project_signal": "项目态势",
            "operator_preference": "协作偏好",
        }
        freshness_labels = {
            "stable": "稳定",
            "time_sensitive": "时效",
            "volatile": "易变",
        }
        trust_labels = {
            "confirmed": "已确认",
            "working": "工作假设",
            "preference": "协作偏好",
        }
        lines = ["# WORKSPACE_MEMORY", "", "## 当前共享记忆"]
        if not notes:
            lines.append("- 当前暂无共享工作区记忆。")
            return "\n".join(lines).strip() + "\n"
        for item in notes:
            lines.extend(
                [
                    f"### {_trim(item.get('title')) or '未命名记忆'}",
                    f"- 类型：{type_labels.get(_trim(item.get('memory_type')), _trim(item.get('memory_type')) or '未分类')}",
                    f"- 更新时间：{_trim(item.get('updated_at')) or _trim(item.get('created_at')) or '未知'}",
                    f"- 新鲜度：{freshness_labels.get(_trim(item.get('freshness')), _trim(item.get('freshness')) or '未标注')}",
                    f"- 信任级别：{trust_labels.get(_trim(item.get('trust_level')), _trim(item.get('trust_level')) or '未标注')}",
                    f"- 使用要求：{_revalidation_text(item.get('required_revalidation'))}",
                    f"- 标签：{'、'.join(_trim_list(item.get('tags') or [])) or '无'}",
                ]
            )
            summary = _trim(item.get("summary"))
            detail = _trim(item.get("detail"))
            if summary:
                lines.extend(["", summary])
            if detail:
                lines.extend(["", detail])
            lines.append("")
        return "\n".join(lines).strip() + "\n"

    def render_memory(self, day_text: str, notes: Sequence[dict[str, Any]]) -> str:
        lines = [f"# memory/{day_text}.md", "", "## 当日分析记录"]
        if not notes:
            lines.append("- 今日尚未写入分析记录。")
            return "\n".join(lines).strip() + "\n"
        for item in notes:
            title = _trim(item.get("title")) or "未命名笔记"
            author = _trim(item.get("author")) or "system"
            created_at = _trim(item.get("created_at")) or "未知时间"
            query_ids = _trim_list(item.get("source_query_ids_json") if isinstance(item.get("source_query_ids_json"), Sequence) and not isinstance(item.get("source_query_ids_json"), (str, bytes)) else [])
            evidence_ids = _trim_list(item.get("source_evidence_ids_json") if isinstance(item.get("source_evidence_ids_json"), Sequence) and not isinstance(item.get("source_evidence_ids_json"), (str, bytes)) else [])
            lines.extend(
                [
                    f"### {title}",
                    f"- 时间：{created_at}",
                    f"- 作者：{author}",
                    f"- 关联查询：{', '.join(query_ids) or '无'}",
                    f"- 关联证据：{', '.join(evidence_ids) or '无'}",
                    "",
                    _trim(item.get("content_md")) or "（空）",
                    "",
                ]
            )
        return "\n".join(lines).strip() + "\n"

    def build_report_projection_file_name(self, *, scope: str, status: str, version: int) -> str:
        normalized_scope = _trim(scope) or "stage_report"
        normalized_status = _trim(status) or "draft"
        safe_scope = "".join(ch if ch.isalnum() or ch in {"-", "_"} else "-" for ch in normalized_scope).strip("-") or "stage_report"
        safe_status = "".join(ch if ch.isalnum() or ch in {"-", "_"} else "-" for ch in normalized_status).strip("-") or "draft"
        return f"reports/{safe_scope}-{safe_status}-v{max(1, int(version))}.md"
