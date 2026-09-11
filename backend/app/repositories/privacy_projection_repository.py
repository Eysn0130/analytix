from __future__ import annotations

import json
import hashlib
import os
import re
import unicodedata
from datetime import datetime
from pathlib import Path
from typing import Any

from app.core.db_engine import DuckDBEngine, get_case_db_path


_VERSION = "privacy_projection_v1"
_ACCOUNT_KEY_MARKERS = (
    "account",
    "acct",
    "card",
    "账号",
    "账户",
    "卡号",
    "银行卡",
)
_PERSON_ID_KEY_MARKERS = (
    "id_no",
    "idno",
    "identity",
    "身份证",
    "证件",
)
_PERSON_NAME_KEYS = {
    "name",
    "holder_name",
    "account_name",
    "account_open_name",
    "opener_name",
    "counterparty_name",
    "display_name",
    "户名",
    "姓名",
    "对手方",
    "对手户名",
}
_DROP_KEY_MARKERS = (
    "address",
    "addr",
    "home_addr",
    "org_addr",
    "phone",
    "mobile",
    "tel",
    "email",
    "mail",
    "bank",
    "branch_name",
    "开户地址",
    "住址",
    "地址",
    "电话",
    "手机",
    "邮箱",
    "开户行",
    "支行",
)
_SUMMARY_KEY_MARKERS = ("summary", "remark", "memo", "note", "摘要", "备注")
_CHANNEL_MARKERS = ("微信", "支付宝", "财付通", "银联", "云闪付")
_ORG_MARKERS = ("公司", "银行", "支行", "中心", "商户", "单位", "企业", "有限", "集团")
_LONG_NUMBER_RE = re.compile(r"(?<!\d)\d{11,24}(?!\d)")
_GROUPED_LONG_NUMBER_RE = re.compile(
    r"(?<!\d)(?:\d[ \-\u2010-\u2015\u2212\ufe58\ufe63\uff0d]?){10,23}\d(?!\d)"
)
_ID_NO_RE = re.compile(r"(?<!\d)\d{17}[\dXx](?!\d)")
_EMAIL_RE = re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")
_PHONE_RE = re.compile(r"(?<!\d)1[3-9]\d{9}(?!\d)")
_MAC_RE = re.compile(r"(?i)(?<![0-9a-f])(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}(?![0-9a-f])")
_LABEL_VALUE_PATTERNS = (
    (re.compile(r"(户名|姓名|对手方|对手户名)([:：])\s*([^|\n，,；;\s]+)"), "[PERSON]"),
    (re.compile(r"(账号|账户|卡号|银行卡号|对手账号|对手卡号)([:：])\s*([^|\n，,；;\s]+)"), "[ACCOUNT]"),
    (re.compile(r"(身份证号|证件号)([:：])\s*([^|\n，,；;\s]+)"), "[ID_NO]"),
    (re.compile(r"(开户行|支行|开户地址|住址|地址)([:：])\s*([^|\n，,；;\s]+)"), "[REMOVED]"),
)
_COUNTERPARTY_PIPE_RE = re.compile(r"(对手(?:方)?\s+).*?(\s*\|\s*摘要)")
_PERSON_ROLE_VALUE_RE = re.compile(
    r"(经办人员|联系人|持卡人|开户人|实际控制人|法定代表人)([:：]?\s*)[^|,，;；。\n]+"
)
_PRIVACY_TOKEN_RE = re.compile(r"^(?:ACCOUNT:A|PERSON:P|ORG:O|CHANNEL:C|DEVICE:D|BRANCH:B|TELLER:T)[0-9A-F]{12}$")
_PRIVACY_ALIAS_RE = re.compile(r"^(?:A|P|O|C|D|B|T|E)[1-9][0-9]*$")
_LOCAL_PRIVACY_FILTER_PLACEHOLDERS = {
    "account_number": "[ACCOUNT]",
    "private_address": "[ADDRESS]",
    "private_email": "[EMAIL]",
    "private_person": "[PERSON]",
    "private_phone": "[PHONE]",
    "private_url": "[URL]",
    "secret": "[SECRET]",
}
_SAFE_PRIVACY_PLACEHOLDERS = frozenset(_LOCAL_PRIVACY_FILTER_PLACEHOLDERS.values()) | {
    "[COUNTERPARTY]",
    "[DEVICE]",
    "[ID_NO]",
    "[NUMBER]",
    "[REMOVED]",
}
_PRIVACY_PROJECTION_UNAVAILABLE = "privacy_projection_unavailable"
_PRIVACY_STATE_UNAVAILABLE = "privacy_projection_state_unavailable"
_PRIVACY_SALT_RE = re.compile(r"^[a-f0-9]{64}$")
_TYPED_NUMERIC_MEASURE_KEY_MARKERS = frozenset(
    {
        "amount",
        "balance",
        "count",
        "total",
        "sum",
        "rate",
        "ratio",
        "score",
        "duration",
        "size",
        "金额",
        "余额",
        "笔数",
        "数量",
        "合计",
        "比例",
    }
)


class PrivacyProjectionLeakError(RuntimeError):
    """Raised when provider-visible content still contains raw sensitive identifiers."""


class PrivacyProjectionRepository:
    def _open_case_engine(self, case_id: str) -> DuckDBEngine:
        db_path = get_case_db_path(case_id)
        if not db_path.exists():
            raise KeyError(case_id)
        # DuckDB rejects mixing read_only=true and read/write connections for the
        # same file in one process. Privacy read paths use the same local profile
        # as CaseStorage and rely on repository policy, not DuckDB mode, for safety.
        return DuckDBEngine(db_path)

    def get_status(self, case_id: str) -> dict[str, Any]:
        normalized_case_id = self._normalize_case_id(case_id)
        engine = self._open_case_engine(normalized_case_id)
        try:
            if not self._table_exists(engine, "privacy_runtime_state"):
                return self._default_status(normalized_case_id)
            rows = engine.query(
                """
                SELECT version, status, enabled, progress, salt, updated_at
                  FROM privacy_runtime_state
                 WHERE case_id=?
                 LIMIT 1
                """,
                (normalized_case_id,),
            )
            if not rows:
                return self._default_status(normalized_case_id)
            version, status, enabled, progress, salt, updated_at = rows[0]
            state_status = str(status or "disabled").strip() or "disabled"
            state_enabled = bool(enabled)
            state_progress = int(progress or 0)
            if (
                version != _VERSION
                or state_status not in {"disabled", "running", "completed", "failed", "stale"}
                or state_progress < 0
                or state_progress > 100
                or (state_enabled and _PRIVACY_SALT_RE.fullmatch(str(salt or "")) is None)
                or (state_enabled and not self._valid_state_timestamp(updated_at))
            ):
                return self._unavailable_status(normalized_case_id)
            source_rows = 0
            source_hash = ""
            projected_rows = 0
            projected_hash = ""
            if state_enabled and state_status == "completed" and state_progress >= 100:
                try:
                    source_rows, source_hash = self._source_fingerprint(engine, normalized_case_id)
                    projected_rows, projected_hash = self._projection_fingerprint(
                        engine,
                        normalized_case_id,
                    )
                except Exception:
                    return self._unavailable_status(normalized_case_id)
            requires_refresh = (
                state_enabled
                and state_status == "completed"
                and state_progress >= 100
                and (source_rows != projected_rows or source_hash != projected_hash)
            )
            effective_status = "stale" if requires_refresh else state_status
            return {
                "case_id": normalized_case_id,
                "version": _VERSION,
                "status": effective_status,
                "enabled": state_enabled,
                "progress": 100
                if state_enabled and not requires_refresh and state_status == "completed"
                else state_progress,
                "label": self._status_label(
                    status=effective_status,
                    enabled=state_enabled,
                    progress=state_progress,
                    requires_refresh=requires_refresh,
                ),
                "summary": "",
                "error": "privacy_projection_failed" if state_status == "failed" else "",
                "updated_at": str(updated_at or ""),
                "requires_refresh": requires_refresh,
                "source_rows": int(source_rows or 0),
                "projected_rows": int(projected_rows or 0),
            }
        finally:
            engine.close()

    def get_db_path(self, case_id: str) -> Path:
        normalized_case_id = self._normalize_case_id(case_id)
        db_path = get_case_db_path(normalized_case_id)
        if not db_path.exists():
            raise KeyError(normalized_case_id)
        return db_path

    def is_enabled(self, case_id: str) -> bool:
        try:
            status = self.get_status(case_id)
            progress = int(status.get("progress") or 0)
        except Exception:
            return False
        return (
            bool(status.get("enabled"))
            and str(status.get("status") or "").strip() == "completed"
            and progress >= 100
            and not bool(status.get("requires_refresh"))
        )

    def mark_failed(self, case_id: str, error: str) -> None:
        normalized_case_id = self._normalize_case_id(case_id)
        _ = error
        db_path = get_case_db_path(normalized_case_id)
        if not db_path.exists():
            return
        engine = DuckDBEngine(db_path)
        try:
            engine.execute(
                """
                CREATE TABLE IF NOT EXISTS privacy_runtime_state (
                  case_id TEXT PRIMARY KEY,
                  version TEXT,
                  status TEXT,
                  enabled BOOLEAN,
                  progress INTEGER,
                  summary TEXT,
                  error TEXT,
                  salt TEXT,
                  updated_at TEXT
                )
                """
            )
            salt = self._read_salt(engine, normalized_case_id)
            engine.execute("DELETE FROM privacy_runtime_state WHERE case_id=?", (normalized_case_id,))
            engine.execute(
                """
                INSERT INTO privacy_runtime_state(
                  case_id, version, status, enabled, progress, summary, error, salt, updated_at
                ) VALUES (?, ?, 'failed', false, 0, '', ?, ?, CAST(current_timestamp AS VARCHAR))
                """,
                (normalized_case_id, _VERSION, "privacy_projection_failed", salt),
            )
        finally:
            engine.close()

    def project_model_payload(self, case_id: str, payload: Any) -> Any:
        normalized_case_id = self._normalize_case_id(case_id)
        context = self._ordinary_projection_context(normalized_case_id)
        if context is None:
            return self._projection_unavailable_payload()
        salt, token_aliases, replacements = context
        projected = self._project_value(
            payload,
            salt=salt,
            parent_key="",
            replacements=replacements,
            token_aliases=token_aliases,
        )
        try:
            self.assert_provider_visible_payload_safe(normalized_case_id, projected)
        except PrivacyProjectionLeakError:
            return self._projection_unavailable_payload()
        return projected

    def project_model_text(self, case_id: str, text: str) -> str:
        normalized_case_id = self._normalize_case_id(case_id)
        if not text:
            return ""
        context = self._ordinary_projection_context(normalized_case_id)
        if context is None:
            return _PRIVACY_PROJECTION_UNAVAILABLE
        _, token_aliases, replacements = context
        projected = self._redact_text(
            str(text),
            replacements=replacements,
            token_aliases=token_aliases,
        )
        try:
            self.assert_provider_visible_payload_safe(normalized_case_id, projected)
        except PrivacyProjectionLeakError:
            return _PRIVACY_PROJECTION_UNAVAILABLE
        return projected

    def project_ordinary_artifact_text(self, case_id: str, text: str) -> str:
        """Project ordinary persisted artifacts without requiring privacy state.

        Workspace bootstrap can run before the optional case token map exists.
        The deterministic baseline redactor therefore remains mandatory and a
        residual sensitive identifier fails the write instead of restoring the
        caller's original text.
        """

        normalized_case_id = self._normalize_case_id(case_id)
        projected = self._redact_text(str(text or ""))
        self.assert_provider_visible_payload_safe(normalized_case_id, projected)
        return projected

    def assert_provider_visible_payload_safe(self, case_id: str, payload: Any) -> None:
        self._normalize_case_id(case_id)
        leaks = self._provider_visible_leak_samples(payload)
        if leaks:
            raise PrivacyProjectionLeakError(_PRIVACY_PROJECTION_UNAVAILABLE)

    def _ordinary_projection_context(
        self,
        case_id: str,
    ) -> tuple[str, dict[str, str], list[tuple[str, str]]] | None:
        try:
            if not self.is_enabled(case_id):
                return None
            salt = self._salt_for_projection(case_id)
            if _PRIVACY_SALT_RE.fullmatch(salt) is None:
                return None
            token_aliases = self._entity_aliases_for_projection(case_id)
            replacements = self._entity_replacements_for_projection(
                case_id,
                token_aliases=token_aliases,
            )
        except Exception:
            return None
        return salt, token_aliases, replacements

    def get_display_map(self, case_id: str, *, limit: int = 20000) -> dict[str, str]:
        normalized_case_id = self._normalize_case_id(case_id)
        if not self.is_enabled(normalized_case_id):
            return {}
        engine = self._open_case_engine(normalized_case_id)
        try:
            if not self._table_exists(engine, "privacy_entity_map"):
                return {}
            has_display_value = self._column_exists(engine, "privacy_entity_map", "display_value")
            if not has_display_value:
                return {}
            alias_join = ""
            alias_select = "'' AS alias"
            if self._table_exists(engine, "privacy_entity_alias"):
                alias_join = """
                  LEFT JOIN privacy_entity_alias a
                    ON a.case_id=m.case_id
                   AND a.entity_id=m.entity_id
                """
                alias_select = "COALESCE(a.alias, '') AS alias"
            rows = engine.query(
                f"""
                SELECT m.entity_id, m.display_value, {alias_select}
                  FROM privacy_entity_map m
                  {alias_join}
                 WHERE m.case_id=?
                   AND COALESCE(m.entity_id, '')<>''
                   AND COALESCE(m.display_value, '')<>''
                   AND m.entity_type IN ('account', 'person', 'org', 'channel')
                 ORDER BY row_count DESC
                 LIMIT ?
                """,
                (normalized_case_id, max(1, min(100000, int(limit or 20000)))),
            )
        finally:
            engine.close()
        display_map: dict[str, str] = {}
        display_counts: dict[str, int] = {}
        for _token, display_value, _alias in rows:
            normalized_display = str(display_value or "").strip()
            if normalized_display:
                display_counts[normalized_display] = display_counts.get(normalized_display, 0) + 1
        for token, display_value, alias in rows:
            normalized_token = str(token or "").strip()
            normalized_display = str(display_value or "").strip()
            normalized_alias = str(alias or "").strip()
            alias_display = (
                f"{normalized_display}（{normalized_alias}）"
                if normalized_alias and display_counts.get(normalized_display, 0) > 1
                else normalized_display
            )
            if normalized_token and normalized_display:
                display_map[normalized_token] = alias_display
            if normalized_alias and normalized_display:
                display_map[normalized_alias] = alias_display
        return display_map

    def changed_file_ids_for_projection(self, case_id: str, *, limit: int = 256) -> list[str]:
        normalized_case_id = self._normalize_case_id(case_id)
        db_path = get_case_db_path(normalized_case_id)
        if not db_path.exists():
            return []
        engine = DuckDBEngine(db_path)
        try:
            if not self._table_exists(engine, "privacy_source_file_checkpoint"):
                return []
            if not self._table_exists(engine, "import_file_log"):
                return []
            active_rows = engine.query(
                """
                SELECT DISTINCT file_id
                  FROM import_file_log
                 WHERE case_id=?
                   AND COALESCE(file_id, '')<>''
                 LIMIT ?
                """,
                (normalized_case_id, max(1, min(10000, int(limit or 256) * 4))),
            )
            checkpoint_rows = engine.query(
                """
                SELECT DISTINCT file_id
                  FROM privacy_source_file_checkpoint
                 WHERE case_id=?
                   AND COALESCE(file_id, '')<>''
                 LIMIT ?
                """,
                (normalized_case_id, max(1, min(10000, int(limit or 256) * 4))),
            )
        finally:
            engine.close()
        active_ids = {str(row[0] or "").strip() for row in active_rows if str(row[0] or "").strip()}
        checkpoint_ids = {str(row[0] or "").strip() for row in checkpoint_rows if str(row[0] or "").strip()}
        changed = sorted(active_ids.symmetric_difference(checkpoint_ids))
        if len(changed) > max(1, int(limit or 256)):
            return []
        return changed

    def _salt_for_projection(self, case_id: str) -> str:
        db_path = get_case_db_path(case_id)
        if not db_path.exists():
            return ""
        engine = DuckDBEngine(db_path)
        try:
            if self._table_exists(engine, "privacy_runtime_state"):
                salt = self._read_salt(engine, case_id)
                if salt:
                    return salt
        finally:
            engine.close()
        return ""

    def _read_salt(self, engine: DuckDBEngine, case_id: str) -> str:
        try:
            rows = engine.query(
                "SELECT salt FROM privacy_runtime_state WHERE case_id=? AND COALESCE(salt,'')<>'' LIMIT 1",
                (case_id,),
            )
        except Exception:
            return ""
        salt = str(rows[0][0] or "").strip() if rows else ""
        return salt if _PRIVACY_SALT_RE.fullmatch(salt) else ""

    def _entity_aliases_for_projection(self, case_id: str, *, limit: int = 200000) -> dict[str, str]:
        db_path = get_case_db_path(case_id)
        if not db_path.exists():
            return {}
        engine = DuckDBEngine(db_path)
        try:
            if not self._table_exists(engine, "privacy_entity_alias"):
                return {}
            rows = engine.query(
                """
                SELECT entity_id, alias
                  FROM privacy_entity_alias
                 WHERE case_id=?
                   AND COALESCE(entity_id, '')<>''
                   AND COALESCE(alias, '')<>''
                 LIMIT ?
                """,
                (case_id, max(1, min(500000, int(limit or 200000)))),
            )
        finally:
            engine.close()
        aliases: dict[str, str] = {}
        for token, alias in rows:
            normalized_token = str(token or "").strip()
            normalized_alias = str(alias or "").strip()
            if (
                _PRIVACY_TOKEN_RE.fullmatch(normalized_token)
                and _PRIVACY_ALIAS_RE.fullmatch(normalized_alias)
            ):
                aliases[normalized_token] = normalized_alias
        return aliases

    def _entity_replacements_for_projection(
        self,
        case_id: str,
        *,
        token_aliases: dict[str, str] | None = None,
        limit: int = 50000,
    ) -> list[tuple[str, str]]:
        db_path = get_case_db_path(case_id)
        if not db_path.exists():
            return []
        engine = DuckDBEngine(db_path)
        try:
            if not self._table_exists(engine, "privacy_entity_map"):
                return []
            if not self._column_exists(engine, "privacy_entity_map", "display_value"):
                return []
            rows = engine.query(
                """
                WITH filtered AS (
                  SELECT display_value, entity_id, entity_type, row_count
                    FROM privacy_entity_map
                   WHERE case_id=?
                     AND COALESCE(display_value, '')<>''
                     AND COALESCE(entity_id, '')<>''
                     AND entity_type IN ('account', 'person', 'org', 'device', 'teller')
                ),
                display_counts AS (
                  SELECT display_value, entity_type, COUNT(DISTINCT entity_id) AS entity_count
                    FROM filtered
                   GROUP BY display_value, entity_type
                )
                SELECT f.display_value, f.entity_id, f.entity_type, c.entity_count
                  FROM filtered f
                  JOIN display_counts c
                    ON c.display_value=f.display_value
                   AND c.entity_type=f.entity_type
                 ORDER BY LENGTH(f.display_value) DESC, f.row_count DESC
                 LIMIT ?
                """,
                (case_id, max(1, min(200000, int(limit or 50000)))),
            )
        finally:
            engine.close()

        replacements: list[tuple[str, str]] = []
        seen: set[str] = set()
        for display_value, token, entity_type, entity_count in rows:
            raw = str(display_value or "").strip()
            replacement = str(token or "").strip()
            if not raw or not replacement or raw in seen:
                continue
            normalized_entity_type = str(entity_type or "")
            if not self._should_replace_entity_text(raw, entity_type=normalized_entity_type):
                continue
            seen.add(raw)
            if normalized_entity_type == "person" and int(entity_count or 0) > 1:
                replacements.append((raw, "[PERSON]"))
            else:
                replacements.append((raw, self._alias_token(replacement, token_aliases=token_aliases or {})))
        return replacements

    def _should_replace_entity_text(self, value: str, *, entity_type: str) -> bool:
        text = str(value or "").strip()
        if not text:
            return False
        if entity_type == "account":
            return len(re.sub(r"\D", "", text)) >= 6
        if entity_type == "person":
            return len(text) >= 2
        if entity_type == "org":
            return len(text) >= 4
        if entity_type in {"device", "teller"}:
            return len(text) >= 3
        return False

    def _name_key_contextual_raw_value(self, key: str, raw_child: Any, siblings: dict[Any, Any]) -> str:
        if not isinstance(raw_child, str):
            return ""
        name = str(raw_child or "").strip()
        if not name:
            return ""
        normalized_key = self._normalize_key(key)
        if not self._is_person_name_key(normalized_key):
            return ""
        identity = (
            self._sibling_value(siblings, self._counterparty_identity_key_markers())
            if self._is_counterparty_name_key(normalized_key)
            else self._sibling_value(siblings, _PERSON_ID_KEY_MARKERS, exclude_markers=("counterparty", "opponent", "对手"))
        )
        if identity:
            return identity
        account = (
            self._sibling_value(siblings, self._counterparty_account_key_markers())
            if self._is_counterparty_name_key(normalized_key)
            else self._sibling_value(siblings, _ACCOUNT_KEY_MARKERS, exclude_markers=("counterparty", "opponent", "对手"))
        )
        if account:
            return f"{name}:acct:{account}"
        return ""

    def _is_person_name_key(self, normalized_key: str) -> bool:
        return (
            normalized_key in _PERSON_NAME_KEYS
            or "name" in normalized_key
            or "姓名" in normalized_key
            or "户名" in normalized_key
            or "对手方" in normalized_key
        )

    def _is_counterparty_name_key(self, normalized_key: str) -> bool:
        return any(marker in normalized_key for marker in ("counterparty", "opponent", "对手"))

    def _counterparty_account_key_markers(self) -> tuple[str, ...]:
        return (
            "counterparty_acct",
            "counterparty_account",
            "counterparty_card",
            "opponent_account",
            "opponent_card",
            "对手账号",
            "对手账户",
            "对手卡号",
            "对手银行卡",
        )

    def _counterparty_identity_key_markers(self) -> tuple[str, ...]:
        return (
            "counterparty_id_no",
            "counterparty_id",
            "counterparty_identity",
            "opponent_id",
            "对手身份证",
            "对手证件",
        )

    def _sibling_value(
        self,
        siblings: dict[Any, Any],
        markers: tuple[str, ...],
        *,
        exclude_markers: tuple[str, ...] = (),
    ) -> str:
        for raw_key, value in siblings.items():
            normalized_key = self._normalize_key(str(raw_key or ""))
            if any(marker in normalized_key for marker in exclude_markers):
                continue
            if any(marker in markers for marker in _ACCOUNT_KEY_MARKERS) and self._is_person_name_key(normalized_key):
                continue
            if not any(marker in normalized_key for marker in markers):
                continue
            if isinstance(value, (str, int, float)):
                normalized_value = str(value or "").strip()
                if normalized_value:
                    return normalized_value
        return ""

    def _project_value(
        self,
        value: Any,
        *,
        salt: str,
        parent_key: str,
        replacements: list[tuple[str, str]],
        token_aliases: dict[str, str],
    ) -> Any:
        if isinstance(value, dict):
            projected: dict[str, Any] = {}
            for raw_key, raw_child in value.items():
                key = str(raw_key or "")
                if self._should_drop_key(key):
                    continue
                projected_key = self._project_key(key, salt=salt)
                contextual_name_key = self._name_key_contextual_raw_value(key, raw_child, value)
                if contextual_name_key:
                    projected[projected_key] = self._alias_token(
                        self._token("person", contextual_name_key, salt=salt),
                        token_aliases=token_aliases,
                    )
                else:
                    projected[projected_key] = self._project_value(
                        raw_child,
                        salt=salt,
                        parent_key=key,
                        replacements=replacements,
                        token_aliases=token_aliases,
                    )
            return projected
        if isinstance(value, list):
            return [
                self._project_value(
                    item,
                    salt=salt,
                    parent_key=parent_key,
                    replacements=replacements,
                    token_aliases=token_aliases,
                )
                for item in value
            ]
        if isinstance(value, tuple):
            return tuple(
                self._project_value(item, salt=salt, parent_key=parent_key, replacements=replacements, token_aliases=token_aliases)
                for item in value
            )
        if isinstance(value, str):
            return self._project_string(parent_key, value, salt=salt, replacements=replacements, token_aliases=token_aliases)
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            normalized_key = self._normalize_key(parent_key)
            if self._is_account_key(normalized_key):
                return self._alias_token(
                    self._token("account", str(value), salt=salt),
                    token_aliases=token_aliases,
                )
            if any(marker in normalized_key for marker in _PERSON_ID_KEY_MARKERS):
                return self._alias_token(
                    self._token("person", str(value), salt=salt),
                    token_aliases=token_aliases,
                )
        return value

    def _project_key(self, key: str, *, salt: str) -> str:
        if self._looks_like_account(key):
            return self._token("account", key, salt=salt)
        return str(key or "")

    def _project_string(
        self,
        key: str,
        value: str,
        *,
        salt: str,
        replacements: list[tuple[str, str]],
        token_aliases: dict[str, str],
    ) -> str:
        text = str(value or "")
        if not text:
            return text
        normalized_key = self._normalize_key(key)
        if self._is_account_key(normalized_key) or self._looks_like_account(text):
            return self._alias_token(self._token("account", text, salt=salt), token_aliases=token_aliases)
        if any(marker in normalized_key for marker in _PERSON_ID_KEY_MARKERS):
            return self._alias_token(self._token("person", text, salt=salt), token_aliases=token_aliases)
        if normalized_key in _PERSON_NAME_KEYS or any(item in normalized_key for item in ("name", "姓名", "户名", "对手方")):
            return self._alias_token(self._classify_name_token(text, salt=salt), token_aliases=token_aliases)
        if "ip" in normalized_key or _MAC_RE.search(text):
            return self._alias_token(self._token("device", text, salt=salt), token_aliases=token_aliases)
        if "mac" in normalized_key:
            return self._alias_token(self._token("device", text, salt=salt), token_aliases=token_aliases)
        if "teller" in normalized_key or "柜员" in normalized_key:
            return self._alias_token(self._token("teller", text, salt=salt), token_aliases=token_aliases)
        if any(marker in normalized_key for marker in _SUMMARY_KEY_MARKERS):
            return self._redact_text(text, replacements=replacements, token_aliases=token_aliases)
        return self._redact_text(text, replacements=replacements, token_aliases=token_aliases)

    def _classify_name_token(self, value: str, *, salt: str) -> str:
        if any(marker in value for marker in _CHANNEL_MARKERS):
            return self._token("channel", value, salt=salt)
        if any(marker in value for marker in _ORG_MARKERS):
            return self._token("org", value, salt=salt)
        return self._token("person", value, salt=salt)

    def _token(self, entity_type: str, value: str, *, salt: str) -> str:
        normalized = str(value or "").strip()
        namespace = {
            "account": ("ACCOUNT:A", "account"),
            "person": ("PERSON:P", "person"),
            "org": ("ORG:O", "org"),
            "channel": ("CHANNEL:C", "channel"),
            "device": ("DEVICE:D", "device"),
            "branch": ("BRANCH:B", "branch"),
            "teller": ("TELLER:T", "teller"),
        }.get(entity_type, ("ENTITY:E", "entity"))
        digest = hashlib.sha256(f"{salt}:{namespace[1]}:{normalized}".encode("utf-8")).hexdigest()
        return f"{namespace[0]}{digest[:12].upper()}"

    def _alias_token(self, token: str, *, token_aliases: dict[str, str]) -> str:
        normalized = str(token or "").strip()
        if _PRIVACY_TOKEN_RE.fullmatch(normalized) is None:
            return "[REMOVED]"
        alias = str(token_aliases.get(normalized) or "").strip()
        return alias if _PRIVACY_ALIAS_RE.fullmatch(alias) else normalized

    def _apply_token_aliases(self, text: str, *, token_aliases: dict[str, str] | None = None) -> str:
        projected = str(text or "")
        for token, alias in sorted((token_aliases or {}).items(), key=lambda item: len(item[0]), reverse=True):
            if _PRIVACY_TOKEN_RE.fullmatch(token) and _PRIVACY_ALIAS_RE.fullmatch(alias):
                projected = projected.replace(token, alias)
        return projected

    def _redact_text(
        self,
        value: str,
        *,
        replacements: list[tuple[str, str]] | None = None,
        token_aliases: dict[str, str] | None = None,
    ) -> str:
        # Redaction must preserve non-sensitive user-facing typography. NFKC is
        # still used by keys and the final leak detector, but applying it to the
        # display string would silently rewrite punctuation (for example, `：`
        # to `:`). Strip format controls here and match the original glyphs.
        text = self._sanitize_display_text(value)
        for raw, token in list(replacements or []):
            if raw and token:
                text = text.replace(raw, token)
        text = self._apply_token_aliases(text, token_aliases=token_aliases)
        text = self._apply_local_privacy_filter(text)
        for pattern, replacement in _LABEL_VALUE_PATTERNS:
            text = pattern.sub(
                lambda match, placeholder=replacement: self._label_replacement(match, placeholder),
                text,
            )
        text = _COUNTERPARTY_PIPE_RE.sub(r"\1[COUNTERPARTY]\2", text)
        text = _PERSON_ROLE_VALUE_RE.sub(r"\1\2[PERSON]", text)
        text = _EMAIL_RE.sub("[EMAIL]", text)
        text = _ID_NO_RE.sub("[ID_NO]", text)
        text = _PHONE_RE.sub("[PHONE]", text)
        text = _MAC_RE.sub("[DEVICE]", text)
        text = _GROUPED_LONG_NUMBER_RE.sub("[NUMBER]", text)
        text = _LONG_NUMBER_RE.sub("[NUMBER]", text)
        return text

    def _normalize_untrusted_text(self, value: Any) -> str:
        normalized = unicodedata.normalize("NFKC", str(value or ""))
        return "".join(
            character
            for character in normalized
            if unicodedata.category(character) != "Cf"
        )

    def _sanitize_display_text(self, value: Any) -> str:
        return "".join(
            character
            for character in str(value or "")
            if unicodedata.category(character) != "Cf"
        )

    def _apply_local_privacy_filter(self, text: str) -> str:
        value = str(text or "")
        # Never send ordinary report/chat text to an arbitrary environment-
        # configured executable. The deterministic host redactors below remain
        # authoritative. A future classifier may return only typed spans after
        # package identity, execution containment, and case-data admission are
        # all enforced by the shared execution authority.
        return value

    def _redact_local_privacy_filter_spans(self, text: str, stdout: str) -> str:
        payload: dict[str, Any] | None = None
        for line in reversed([item.strip() for item in str(stdout or "").splitlines() if item.strip()]):
            try:
                decoded = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(decoded, dict):
                payload = decoded
                break
        if not payload:
            return text
        spans = payload.get("detected_spans")
        if not isinstance(spans, list):
            return text
        projected = str(text or "")

        def span_start(item: dict[str, Any]) -> int:
            try:
                return int(item.get("start") or 0)
            except (TypeError, ValueError):
                return 0

        for span in sorted((item for item in spans if isinstance(item, dict)), key=span_start, reverse=True):
            label = str(span.get("label") or "").strip()
            placeholder = _LOCAL_PRIVACY_FILTER_PLACEHOLDERS.get(label)
            if not placeholder:
                continue
            try:
                start = max(0, min(len(projected), int(span.get("start") or 0)))
                end = max(start, min(len(projected), int(span.get("end") or start)))
            except (TypeError, ValueError):
                continue
            if start >= end:
                continue
            projected = f"{projected[:start]}{placeholder}{projected[end:]}"
        return projected

    def _label_replacement(self, match: re.Match[str], placeholder: str) -> str:
        label = match.group(1)
        separator = match.group(2)
        value = str(match.group(3) or "").strip()
        if _PRIVACY_TOKEN_RE.match(value) or _PRIVACY_ALIAS_RE.match(value):
            return f"{label}{separator}{value}"
        return f"{label}{separator}{placeholder}"

    def _provider_visible_leak_samples(self, payload: Any, *, limit: int = 5) -> list[dict[str, str]]:
        leaks: list[dict[str, str]] = []

        def visit(value: Any, path: str, parent_key: str = "") -> None:
            if len(leaks) >= limit:
                return
            if isinstance(value, dict):
                for index, (key, child) in enumerate(value.items()):
                    child_path = f"{path}.field[{index}]" if path else f"field[{index}]"
                    key_text = str(key or "")
                    if self._raw_sensitive_text_kind(key_text, parent_key=""):
                        leaks.append({"path": child_path, "kind": "sensitive_key"})
                    else:
                        visit(child, child_path, key_text)
                    if len(leaks) >= limit:
                        return
                return
            if isinstance(value, (list, tuple)):
                for index, child in enumerate(value):
                    visit(child, f"{path}[{index}]", parent_key)
                    if len(leaks) >= limit:
                        return
                return
            if not isinstance(value, (str, int, float)) or isinstance(value, bool):
                return
            if isinstance(value, (int, float)) and self._is_typed_numeric_measure(parent_key):
                return
            text = self._normalize_untrusted_text(value)
            kind = self._raw_sensitive_text_kind(text, parent_key=parent_key)
            if kind:
                leaks.append({"path": path or "$", "kind": kind})

        visit(payload, "$")
        return leaks

    def _raw_sensitive_text_kind(self, value: str, *, parent_key: str) -> str:
        text = self._normalize_untrusted_text(value).strip()
        if not text:
            return ""
        if _PRIVACY_TOKEN_RE.fullmatch(text) or _PRIVACY_ALIAS_RE.fullmatch(text):
            return ""
        if text in _SAFE_PRIVACY_PLACEHOLDERS:
            return ""
        normalized_key = self._normalize_key(parent_key)
        if self._is_account_key(normalized_key):
            return "account"
        if any(marker in normalized_key for marker in _PERSON_ID_KEY_MARKERS):
            return "id_no"
        if self._is_person_name_key(normalized_key):
            return "person_name"
        if self._should_drop_key(normalized_key):
            return "restricted_field"
        for kind, pattern in (
            ("id_no", _ID_NO_RE),
            ("phone", _PHONE_RE),
            ("email", _EMAIL_RE),
            ("device", _MAC_RE),
            ("grouped_long_number", _GROUPED_LONG_NUMBER_RE),
            ("long_number", _LONG_NUMBER_RE),
        ):
            if pattern.search(text):
                return kind
        return ""

    def _source_fingerprint(self, engine: DuckDBEngine, case_id: str) -> tuple[int, str]:
        if not self._table_exists(engine, "fc_transaction_norm"):
            raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE)
        try:
            rows = engine.query(
                """
                SELECT COUNT(*), COALESCE(SUM(hash(COALESCE(row_hash, ''))), 0)
                  FROM fc_transaction_norm
                 WHERE case_id=?
                """,
                (case_id,),
            )
            if len(rows) != 1 or len(rows[0]) != 2:
                raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE)
            return int(rows[0][0] or 0), str(rows[0][1] or "0")
        except Exception:
            raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE) from None

    def _projection_fingerprint(self, engine: DuckDBEngine, case_id: str) -> tuple[int, str]:
        if not self._table_exists(engine, "privacy_txn_projection"):
            raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE)
        try:
            rows = engine.query(
                """
                SELECT COUNT(*), COALESCE(SUM(hash(COALESCE(source_row_hash, ''))), 0)
                  FROM privacy_txn_projection
                 WHERE case_id=?
                """,
                (case_id,),
            )
            if len(rows) != 1 or len(rows[0]) != 2:
                raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE)
            return int(rows[0][0] or 0), str(rows[0][1] or "0")
        except Exception:
            raise PrivacyProjectionLeakError(_PRIVACY_STATE_UNAVAILABLE) from None

    def _table_exists(self, engine: DuckDBEngine, table: str) -> bool:
        rows = engine.query(
            "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name=? LIMIT 1",
            (table,),
        )
        return bool(rows)

    def _column_exists(self, engine: DuckDBEngine, table: str, column: str) -> bool:
        rows = engine.query(
            """
            SELECT 1
              FROM information_schema.columns
             WHERE table_schema='main' AND table_name=? AND column_name=?
             LIMIT 1
            """,
            (table, column),
        )
        return bool(rows)

    def _default_status(self, case_id: str) -> dict[str, Any]:
        return {
            "case_id": case_id,
            "version": _VERSION,
            "status": "disabled",
            "enabled": False,
            "progress": 0,
            "label": "脱敏",
            "summary": "",
            "error": "",
            "updated_at": "",
            "requires_refresh": False,
            "source_rows": 0,
            "projected_rows": 0,
        }

    def _unavailable_status(self, case_id: str) -> dict[str, Any]:
        return {
            **self._default_status(case_id),
            "status": "stale",
            "enabled": False,
            "error": _PRIVACY_STATE_UNAVAILABLE,
            "requires_refresh": True,
        }

    def _projection_unavailable_payload(self) -> dict[str, Any]:
        return {
            "privacy_projection": _PRIVACY_PROJECTION_UNAVAILABLE,
            "fact_answer_allowed": False,
            "raw_details_exposed": False,
        }

    def _valid_state_timestamp(self, value: Any) -> bool:
        if type(value) is not str or not value.strip():
            return False
        normalized = value.strip().replace("Z", "+00:00")
        try:
            return datetime.fromisoformat(normalized) is not None
        except ValueError:
            return False

    def _status_label(self, *, status: str, enabled: bool, progress: int, requires_refresh: bool) -> str:
        if requires_refresh:
            return "脱敏"
        if enabled and status == "completed" and progress >= 100:
            return "已脱敏"
        if status == "running":
            return f"{max(0, min(100, int(progress or 0)))}%"
        return "脱敏"

    def _normalize_case_id(self, case_id: str) -> str:
        normalized = str(case_id or "").strip()
        if not normalized:
            raise KeyError("case_id")
        return normalized

    def _normalize_key(self, key: str) -> str:
        return self._normalize_untrusted_text(key).strip().lower()

    def _should_drop_key(self, key: str) -> bool:
        normalized = self._normalize_key(key)
        if self._is_account_key(normalized):
            return False
        return any(marker in normalized for marker in _DROP_KEY_MARKERS)

    def _is_typed_numeric_measure(self, key: str) -> bool:
        normalized = self._normalize_key(key)
        return bool(
            normalized
            and not self._is_account_key(normalized)
            and not any(marker in normalized for marker in _PERSON_ID_KEY_MARKERS)
            and any(marker in normalized for marker in _TYPED_NUMERIC_MEASURE_KEY_MARKERS)
        )

    def _is_account_key(self, normalized_key: str) -> bool:
        if any(marker in normalized_key for marker in _PERSON_ID_KEY_MARKERS):
            return False
        if "name" in normalized_key or "姓名" in normalized_key or "户名" in normalized_key:
            return False
        return any(marker in normalized_key for marker in _ACCOUNT_KEY_MARKERS)

    def _looks_like_account(self, value: str) -> bool:
        compact = re.sub(r"\D", "", self._normalize_untrusted_text(value))
        return len(compact) >= 11
