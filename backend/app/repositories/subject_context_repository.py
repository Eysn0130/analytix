from __future__ import annotations

import hashlib
import math
import re
from dataclasses import dataclass
from typing import Dict, Iterable, List, Optional

from app.core.db_engine import DuckDBEngine
from app.core.storage import CaseStorage
from app.utils.time import utc_now


_SOURCE_ORDER = {
    "fc_person": 0,
    "fc_person_address": 1,
    "fc_person_contact": 2,
}

_SOURCE_LABELS = {
    "fc_person": "人员信息",
    "fc_person_address": "人员住址信息",
    "fc_person_contact": "人员联系方式信息",
}

_SELECTION_TABLE = "llm_subject_selection"
_WHITESPACE_RE = re.compile(r"\s+")


@dataclass(frozen=True)
class SubjectSourceEntry:
    source_kind: str
    source_label: str
    source_row_key: str
    file_id: str
    row_no: int
    row_hash: str
    imported_at: str
    name: str
    normalized_name: str
    id_no: str
    normalized_id_no: str
    id_type: str
    employer: str = ""
    email: str = ""
    org_phone: str = ""
    org_addr: str = ""
    home_addr: str = ""
    home_phone: str = ""
    contact_phone: str = ""


class SubjectContextRepository:
    def __init__(self) -> None:
        self._storage = CaseStorage()

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def open_case_engine(self, case_id: str, *, read_only: bool = False) -> DuckDBEngine:
        return self._storage.open_case_engine(case_id, read_only=read_only)

    def ensure_tables(self, engine: DuckDBEngine) -> None:
        engine.execute(
            f"""CREATE TABLE IF NOT EXISTS {_SELECTION_TABLE}(
                case_id TEXT,
                subject_id TEXT,
                selected_for_llm BOOLEAN DEFAULT TRUE,
                updated_at TEXT,
                PRIMARY KEY(case_id, subject_id)
            )"""
        )
        engine.execute(
            f"CREATE INDEX IF NOT EXISTS idx_{_SELECTION_TABLE}_case_updated "
            f"ON {_SELECTION_TABLE}(case_id, updated_at)"
        )

    def list_subjects(self, case_id: str, *, selected_for_llm: Optional[bool] = None) -> List[dict]:
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            selected_ids = self._load_selected_ids(engine, case_id)
            profiles = self._build_subject_profiles(engine, case_id, selected_ids=selected_ids)
            profiles = [item for item in profiles if str(item.get("identity_bucket") or "") == "has_id"]
            if selected_for_llm is not None:
                profiles = [item for item in profiles if bool(item.get("selected_for_llm")) is bool(selected_for_llm)]
            return [self._profile_to_public_dict(item) for item in profiles]
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def get_subject_detail(self, case_id: str, subject_id: str) -> Optional[dict]:
        normalized_subject_id = str(subject_id or "").strip()
        if not normalized_subject_id:
            return None

        engine = self.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            selected_ids = self._load_selected_ids(engine, case_id)
            profiles = self._build_subject_profiles(engine, case_id, selected_ids=selected_ids)
            lookup = self._build_subject_lookup(profiles, include_aliases=True)
            profile = lookup.get(normalized_subject_id)
            if profile is None:
                return None
            file_lookup = self._load_source_file_lookup(engine, case_id)
            return self._profile_to_detail_dict(profile, file_lookup=file_lookup)
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def resolve_subjects(
        self,
        case_id: str,
        *,
        subject_ids: Iterable[str],
        use_selected_subjects: bool,
    ) -> List[dict]:
        requested_ids = [str(item or "").strip() for item in subject_ids if str(item or "").strip()]
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            selected_ids = self._load_selected_ids(engine, case_id)
            profiles = self._build_subject_profiles(engine, case_id, selected_ids=selected_ids)
            if requested_ids:
                lookup = self._build_subject_lookup(profiles, include_aliases=True)
                resolved: List[dict] = []
                seen_subject_ids: set[str] = set()
                for item_id in requested_ids:
                    profile = lookup.get(item_id)
                    canonical_id = str(profile.get("subject_id") or "") if isinstance(profile, dict) else ""
                    if profile is None or not canonical_id or canonical_id in seen_subject_ids:
                        continue
                    seen_subject_ids.add(canonical_id)
                    resolved.append(profile)
                return resolved
            if use_selected_subjects:
                return [item for item in profiles if bool(item.get("selected_for_llm"))]
            return []
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def set_selected_for_llm(
        self,
        case_id: str,
        subject_ids: Iterable[str],
        *,
        selected: bool,
    ) -> int:
        normalized_ids = [str(item or "").strip() for item in subject_ids if str(item or "").strip()]
        if not normalized_ids:
            return 0

        engine = self.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            profiles = self._build_subject_profiles(
                engine,
                case_id,
                selected_ids=self._load_selected_ids(engine, case_id),
            )
            lookup = {
                str(item.get("subject_id") or ""): item
                for item in profiles
                if str(item.get("subject_id") or "").strip()
            }
            target_profiles = []
            seen_subject_ids: set[str] = set()
            for item_id in normalized_ids:
                profile = lookup.get(item_id)
                canonical_id = str(profile.get("subject_id") or "") if isinstance(profile, dict) else ""
                if profile is None or not canonical_id or canonical_id in seen_subject_ids:
                    continue
                seen_subject_ids.add(canonical_id)
                target_profiles.append(profile)
            if not target_profiles:
                return 0

            now = utc_now().isoformat()
            for profile in target_profiles:
                selection_ids = self._selection_ids_for_profile(profile)
                for target_id in selection_ids:
                    engine.execute(
                        f"DELETE FROM {_SELECTION_TABLE} WHERE case_id=? AND subject_id=?",
                        (case_id, target_id),
                    )
                if selected:
                    subject_id = str(profile.get("subject_id") or "")
                    engine.execute(
                        f"""INSERT INTO {_SELECTION_TABLE}(case_id, subject_id, selected_for_llm, updated_at)
                            VALUES (?, ?, TRUE, ?)
                            ON CONFLICT(case_id, subject_id)
                            DO UPDATE SET selected_for_llm=excluded.selected_for_llm, updated_at=excluded.updated_at""",
                        (case_id, subject_id, now),
                    )
            return len(target_profiles)
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def _load_selected_ids(self, engine: DuckDBEngine, case_id: str) -> set[str]:
        rows = engine.query(
            f"SELECT subject_id FROM {_SELECTION_TABLE} WHERE case_id=? AND selected_for_llm=TRUE",
            (case_id,),
        )
        return {str(row[0] or "") for row in rows if str(row[0] or "").strip()}

    def _build_subject_profiles(self, engine: DuckDBEngine, case_id: str, *, selected_ids: set[str]) -> List[dict]:
        entries = self._load_source_entries(engine, case_id)
        profiles_by_key: Dict[str, dict] = {}

        for entry in entries:
            profile_key, merge_status, identity_bucket = self._build_profile_key(entry)

            profile = profiles_by_key.get(profile_key)
            if profile is None:
                subject_id = self._subject_id_from_key(profile_key)
                profile = {
                    "subject_id": subject_id,
                    "case_id": case_id,
                    "display_name": entry.name or "未命名对象",
                    "canonical_name": entry.name or "",
                    "id_no": entry.id_no or "",
                    "id_type": entry.id_type or "",
                    "identity_bucket": identity_bucket,
                    "merge_status": merge_status,
                    "selected_for_llm": subject_id in selected_ids,
                    "source_kinds": set(),
                    "source_labels": set(),
                    "source_file_ids": set(),
                    "source_entries": [],
                    "selection_alias_ids": set(),
                    "names": set(),
                    "contacts": set(),
                    "addresses": set(),
                    "emails": set(),
                    "employers": set(),
                    "org_phones": set(),
                    "home_phones": set(),
                    "row_refs": [],
                    "updated_at": entry.imported_at or "",
                }
                profiles_by_key[profile_key] = profile

            profile["source_file_ids"].add(entry.file_id)
            profile["source_kinds"].add(entry.source_kind)
            profile["source_labels"].add(entry.source_label)
            profile["source_entries"].append(entry)
            if entry.name:
                profile["names"].add(entry.name)
            alias_ids = self._selection_alias_ids_for_entry(entry)
            profile["selection_alias_ids"].update(alias_ids)
            profile["selected_for_llm"] = profile["selected_for_llm"] or any(
                selection_id in selected_ids
                for selection_id in {str(profile.get("subject_id") or ""), *alias_ids}
            )
            profile["row_refs"].append(entry.source_row_key)
            if entry.imported_at and entry.imported_at > str(profile.get("updated_at") or ""):
                profile["updated_at"] = entry.imported_at
            if not profile.get("canonical_name") and entry.name:
                profile["canonical_name"] = entry.name
                profile["display_name"] = entry.name
            if not profile.get("id_no") and entry.id_no:
                profile["id_no"] = entry.id_no
            if not profile.get("id_type") and entry.id_type:
                profile["id_type"] = entry.id_type

            for value in (entry.contact_phone, entry.org_phone, entry.home_phone):
                cleaned = self._clean_text(value)
                if cleaned:
                    profile["contacts"].add(cleaned)
            if entry.org_phone:
                profile["org_phones"].add(self._clean_text(entry.org_phone))
            if entry.home_phone:
                profile["home_phones"].add(self._clean_text(entry.home_phone))
            for value in (entry.org_addr, entry.home_addr):
                cleaned = self._clean_text(value)
                if cleaned:
                    profile["addresses"].add(cleaned)
            for value in (entry.email,):
                cleaned = self._clean_text(value)
                if cleaned:
                    profile["emails"].add(cleaned)
            for value in (entry.employer,):
                cleaned = self._clean_text(value)
                if cleaned:
                    profile["employers"].add(cleaned)

        profiles = [self._finalize_profile(item) for item in profiles_by_key.values()]
        profiles.sort(
            key=lambda item: (
                0 if item.get("identity_bucket") == "has_id" else 1,
                0 if item.get("merge_status") == "identified" else 1,
                str(item.get("display_name") or ""),
                str(item.get("id_no") or ""),
                str(item.get("subject_id") or ""),
            )
        )
        return profiles

    def _build_subject_lookup(self, profiles: List[dict], *, include_aliases: bool) -> Dict[str, dict]:
        lookup: Dict[str, dict] = {}
        for profile in profiles:
            subject_id = str(profile.get("subject_id") or "").strip()
            if subject_id:
                lookup[subject_id] = profile
            if not include_aliases:
                continue
            for alias_id in profile.get("selection_alias_ids") or []:
                normalized_alias = str(alias_id or "").strip()
                if normalized_alias and normalized_alias not in lookup:
                    lookup[normalized_alias] = profile
        return lookup

    def _load_source_file_lookup(self, engine: DuckDBEngine, case_id: str) -> Dict[str, dict]:
        if not self._table_exists(engine, "import_file_log"):
            return {}
        rows = engine.query(
            """SELECT file_id, kind, filename, display_path, status, created_at
               FROM import_file_log
               WHERE case_id=?""",
            (case_id,),
        )
        return {
            str(row[0] or ""): {
                "file_id": str(row[0] or ""),
                "kind": str(row[1] or ""),
                "filename": str(row[2] or ""),
                "display_path": str(row[3] or ""),
                "status": str(row[4] or ""),
                "created_at": str(row[5] or ""),
            }
            for row in rows
            if str(row[0] or "").strip()
        }

    def _load_source_entries(self, engine: DuckDBEngine, case_id: str) -> List[SubjectSourceEntry]:
        entries: List[SubjectSourceEntry] = []
        if self._table_exists(engine, "fc_person_norm"):
            rows = engine.query(
                """SELECT file_id, row_no, row_hash, imported_at, customer_name, id_type, id_no,
                          employer, email, org_phone, org_addr
                   FROM fc_person_norm
                   WHERE case_id=?
                   ORDER BY imported_at ASC, row_no ASC""",
                (case_id,),
            )
            for row in rows:
                name = self._clean_text(row[4])
                id_no = self._clean_text(row[6])
                entries.append(
                    SubjectSourceEntry(
                        source_kind="fc_person",
                        source_label=_SOURCE_LABELS["fc_person"],
                        source_row_key=f"fc_person:{row[0] or ''}:{row[2] or row[1] or ''}",
                        file_id=str(row[0] or ""),
                        row_no=int(row[1] or 0),
                        row_hash=str(row[2] or ""),
                        imported_at=str(row[3] or ""),
                        name=name,
                        normalized_name=self._normalize_name(name),
                        id_no=id_no,
                        normalized_id_no=self._normalize_id_no(id_no),
                        id_type=self._clean_text(row[5]),
                        employer=self._clean_text(row[7]),
                        email=self._clean_text(row[8]),
                        org_phone=self._clean_text(row[9]),
                        org_addr=self._clean_text(row[10]),
                    )
                )
        if self._table_exists(engine, "fc_person_address_norm"):
            rows = engine.query(
                """SELECT file_id, row_no, row_hash, imported_at, open_name, id_type, id_no, home_addr, home_phone
                   FROM fc_person_address_norm
                   WHERE case_id=?
                   ORDER BY imported_at ASC, row_no ASC""",
                (case_id,),
            )
            for row in rows:
                name = self._clean_text(row[4])
                id_no = self._clean_text(row[6])
                entries.append(
                    SubjectSourceEntry(
                        source_kind="fc_person_address",
                        source_label=_SOURCE_LABELS["fc_person_address"],
                        source_row_key=f"fc_person_address:{row[0] or ''}:{row[2] or row[1] or ''}",
                        file_id=str(row[0] or ""),
                        row_no=int(row[1] or 0),
                        row_hash=str(row[2] or ""),
                        imported_at=str(row[3] or ""),
                        name=name,
                        normalized_name=self._normalize_name(name),
                        id_no=id_no,
                        normalized_id_no=self._normalize_id_no(id_no),
                        id_type=self._clean_text(row[5]),
                        home_addr=self._clean_text(row[7]),
                        home_phone=self._clean_text(row[8]),
                    )
                )
        if self._table_exists(engine, "fc_person_contact_norm"):
            rows = engine.query(
                """SELECT file_id, row_no, row_hash, imported_at, open_name, id_type, id_no, contact_phone
                   FROM fc_person_contact_norm
                   WHERE case_id=?
                   ORDER BY imported_at ASC, row_no ASC""",
                (case_id,),
            )
            for row in rows:
                name = self._clean_text(row[4])
                id_no = self._clean_text(row[6])
                entries.append(
                    SubjectSourceEntry(
                        source_kind="fc_person_contact",
                        source_label=_SOURCE_LABELS["fc_person_contact"],
                        source_row_key=f"fc_person_contact:{row[0] or ''}:{row[2] or row[1] or ''}",
                        file_id=str(row[0] or ""),
                        row_no=int(row[1] or 0),
                        row_hash=str(row[2] or ""),
                        imported_at=str(row[3] or ""),
                        name=name,
                        normalized_name=self._normalize_name(name),
                        id_no=id_no,
                        normalized_id_no=self._normalize_id_no(id_no),
                        id_type=self._clean_text(row[5]),
                        contact_phone=self._clean_text(row[7]),
                    )
                )
        return entries

    def _finalize_profile(self, profile: dict) -> dict:
        source_kinds = sorted(
            {str(item or "") for item in profile.get("source_kinds") or [] if str(item or "").strip()},
            key=lambda item: (_SOURCE_ORDER.get(item, 99), item),
        )
        source_labels = [_SOURCE_LABELS.get(item, item) for item in source_kinds]
        source_file_ids = sorted(
            {str(item or "") for item in profile.get("source_file_ids") or [] if str(item or "").strip()}
        )
        source_entries = list(profile.get("source_entries") or [])
        names = sorted({str(item or "") for item in profile.get("names") or [] if str(item or "").strip()})
        contacts = sorted({str(item or "") for item in profile.get("contacts") or [] if str(item or "").strip()})
        addresses = sorted({str(item or "") for item in profile.get("addresses") or [] if str(item or "").strip()})
        emails = sorted({str(item or "") for item in profile.get("emails") or [] if str(item or "").strip()})
        employers = sorted({str(item or "") for item in profile.get("employers") or [] if str(item or "").strip()})
        has_id = bool(profile.get("id_no"))
        merge_status = str(profile.get("merge_status") or "candidate")
        display_name = str(profile.get("display_name") or names[0] if names else "未命名对象")
        id_no = str(profile.get("id_no") or "")
        id_type = str(profile.get("id_type") or "")
        alias_names = [item for item in names if item != display_name]

        identity_label = "有证件号" if has_id else "无证件号"
        merge_label = "已识别主体" if merge_status == "identified" else "待确认主体"
        detail_hint = self._build_profile_hint(
            has_id=has_id,
            id_no=id_no,
            contacts=contacts,
            addresses=addresses,
            source_labels=source_labels,
        )
        summary_text = " · ".join([part for part in [identity_label, detail_hint, "/".join(source_labels)] if part])
        rendered_context = self._build_subject_context_text(
            display_name=display_name,
            merge_label=merge_label,
            identity_label=identity_label,
            id_no=id_no,
            id_type=id_type,
            alias_names=alias_names,
            source_labels=source_labels,
            contacts=contacts,
            addresses=addresses,
            emails=emails,
            employers=employers,
        )
        light_context = self._build_subject_light_context_text(
            display_name=display_name,
            id_no=id_no,
        )

        return {
            "subject_id": str(profile.get("subject_id") or ""),
            "case_id": str(profile.get("case_id") or ""),
            "display_name": display_name,
            "canonical_name": str(profile.get("canonical_name") or display_name),
            "id_no": id_no,
            "id_type": id_type,
            "identity_bucket": str(profile.get("identity_bucket") or ("has_id" if has_id else "missing_id")),
            "identity_label": identity_label,
            "merge_status": merge_status,
            "merge_label": merge_label,
            "selected_for_llm": bool(profile.get("selected_for_llm")),
            "llm_ready": bool(rendered_context.strip()),
            "source_kinds": source_kinds,
            "source_labels": source_labels,
            "source_count": len(source_kinds),
            "source_file_count": len(source_file_ids),
            "source_row_count": len(source_entries),
            "contact_count": len(contacts),
            "address_count": len(addresses),
            "email_count": len(emails),
            "base_count": len(employers) + len(emails),
            "detail_hint": detail_hint,
            "summary_text": summary_text,
            "contacts": contacts,
            "addresses": addresses,
            "emails": emails,
            "employers": employers,
            "token_estimate": self._estimate_tokens(rendered_context),
            "context_text": rendered_context,
            "light_context_text": light_context,
            "alias_names": alias_names,
            "selection_alias_ids": sorted(
                {
                    str(item or "")
                    for item in profile.get("selection_alias_ids") or []
                    if str(item or "").strip()
                }
            ),
            "source_file_ids": source_file_ids,
            "source_entries": source_entries,
            "updated_at": str(profile.get("updated_at") or ""),
        }

    def _profile_to_public_dict(self, profile: dict) -> dict:
        return {
            "subject_id": str(profile.get("subject_id") or ""),
            "case_id": str(profile.get("case_id") or ""),
            "display_name": str(profile.get("display_name") or ""),
            "canonical_name": str(profile.get("canonical_name") or ""),
            "id_no": str(profile.get("id_no") or ""),
            "id_type": str(profile.get("id_type") or ""),
            "identity_bucket": str(profile.get("identity_bucket") or ""),
            "identity_label": str(profile.get("identity_label") or ""),
            "merge_status": str(profile.get("merge_status") or ""),
            "merge_label": str(profile.get("merge_label") or ""),
            "selected_for_llm": bool(profile.get("selected_for_llm")),
            "llm_ready": bool(profile.get("llm_ready")),
            "source_kinds": list(profile.get("source_kinds") or []),
            "source_labels": list(profile.get("source_labels") or []),
            "source_file_ids": list(profile.get("source_file_ids") or []),
            "source_count": int(profile.get("source_count") or 0),
            "source_file_count": int(profile.get("source_file_count") or 0),
            "source_row_count": int(profile.get("source_row_count") or 0),
            "contact_count": int(profile.get("contact_count") or 0),
            "address_count": int(profile.get("address_count") or 0),
            "email_count": int(profile.get("email_count") or 0),
            "base_count": int(profile.get("base_count") or 0),
            "detail_hint": str(profile.get("detail_hint") or ""),
            "summary_text": str(profile.get("summary_text") or ""),
            "token_estimate": int(profile.get("token_estimate") or 0),
            "updated_at": str(profile.get("updated_at") or ""),
        }

    def _profile_to_detail_dict(self, profile: dict, *, file_lookup: Dict[str, dict]) -> dict:
        public_profile = self._profile_to_public_dict(profile)
        source_file_ids = [
            str(item or "")
            for item in profile.get("source_file_ids") or []
            if str(item or "").strip()
        ]
        source_files = []
        for file_id in source_file_ids:
            file_meta = file_lookup.get(file_id) or {}
            source_files.append(
                {
                    "file_id": file_id,
                    "kind": str(file_meta.get("kind") or ""),
                    "filename": str(file_meta.get("filename") or file_id),
                    "display_path": str(file_meta.get("display_path") or ""),
                    "status": str(file_meta.get("status") or ""),
                    "created_at": str(file_meta.get("created_at") or ""),
                }
            )

        source_rows = []
        for entry in profile.get("source_entries") or []:
            if not isinstance(entry, SubjectSourceEntry):
                continue
            file_meta = file_lookup.get(entry.file_id) or {}
            source_rows.append(
                {
                    "source_row_key": entry.source_row_key,
                    "source_kind": entry.source_kind,
                    "source_label": entry.source_label,
                    "file_id": entry.file_id,
                    "file_name": str(file_meta.get("filename") or entry.file_id),
                    "row_no": entry.row_no,
                    "content_text": self._render_source_entry_text(entry),
                }
            )

        return {
            "subject": public_profile,
            "resolved_subject_id": str(profile.get("subject_id") or ""),
            "text_preview": str(profile.get("context_text") or ""),
            "source_files": source_files,
            "source_rows": source_rows,
        }

    def _build_profile_hint(
        self,
        *,
        has_id: bool,
        id_no: str,
        contacts: List[str],
        addresses: List[str],
        source_labels: List[str],
    ) -> str:
        if has_id and id_no:
            return self._mask_id_no(id_no)
        if contacts:
            return f"联系方式 {self._mask_phone(contacts[0])}"
        if addresses:
            return self._clip_text(addresses[0], 16)
        if source_labels:
            return source_labels[0]
        return "待补充信息"

    def _build_subject_context_text(
        self,
        *,
        display_name: str,
        merge_label: str,
        identity_label: str,
        id_no: str,
        id_type: str,
        alias_names: List[str],
        source_labels: List[str],
        contacts: List[str],
        addresses: List[str],
        emails: List[str],
        employers: List[str],
    ) -> str:
        lines = [
            f"主体姓名: {display_name}",
            f"主体状态: {merge_label}",
            f"证件归类: {identity_label}",
        ]
        if alias_names:
            lines.append(f"其他姓名: {' / '.join(alias_names)}")
        if id_no:
            lines.append(f"证件号码: {id_no}")
        else:
            lines.append("证件号码: 未提供")
        if id_type:
            lines.append(f"证件类型: {id_type}")
        if source_labels:
            lines.append(f"来源覆盖: {' / '.join(source_labels)}")
        if employers:
            lines.append("基础信息:")
            lines.extend([f"- 工作单位: {item}" for item in employers])
        if emails:
            if "基础信息:" not in lines:
                lines.append("基础信息:")
            lines.extend([f"- 邮箱: {item}" for item in emails])
        if contacts:
            lines.append("联系方式:")
            lines.extend([f"- {item}" for item in contacts])
        if addresses:
            lines.append("住址信息:")
            lines.extend([f"- {item}" for item in addresses])
        return "\n".join(lines)

    def _build_subject_light_context_text(
        self,
        *,
        display_name: str,
        id_no: str,
    ) -> str:
        return "\n".join(
            [
                f"主体姓名: {display_name}",
                f"证件号码: {id_no or '未提供'}",
            ]
        )

    def _build_profile_key(self, entry: SubjectSourceEntry) -> tuple[str, str, str]:
        if entry.normalized_id_no:
            return f"identified|{entry.normalized_id_no}", "identified", "has_id"
        return (
            f"candidate|{entry.source_kind}|{entry.file_id}|{entry.row_hash or entry.row_no}",
            "candidate",
            "missing_id",
        )

    def _selection_alias_ids_for_entry(self, entry: SubjectSourceEntry) -> set[str]:
        subject_key = (
            f"identified|{entry.normalized_name}|{entry.normalized_id_no}"
            if entry.normalized_name and entry.normalized_id_no
            else f"candidate|{entry.source_kind}|{entry.file_id}|{entry.row_hash or entry.row_no}"
        )
        return {self._subject_id_from_key(subject_key)}

    def _selection_ids_for_profile(self, profile: dict) -> List[str]:
        return sorted(
            {
                str(profile.get("subject_id") or ""),
                *[
                    str(item or "")
                    for item in profile.get("selection_alias_ids") or []
                    if str(item or "").strip()
                ],
            }
            - {""}
        )

    def _render_source_entry_text(self, entry: SubjectSourceEntry) -> str:
        lines = []
        if entry.name:
            lines.append(f"姓名: {entry.name}")
        else:
            lines.append("姓名: 未提供")
        if entry.id_no:
            lines.append(f"证件号码: {entry.id_no}")
        if entry.id_type:
            lines.append(f"证件类型: {entry.id_type}")
        for label, value in (
            ("工作单位", entry.employer),
            ("邮箱", entry.email),
            ("单位电话", entry.org_phone),
            ("单位地址", entry.org_addr),
            ("住宅电话", entry.home_phone),
            ("住宅地址", entry.home_addr),
            ("联系电话", entry.contact_phone),
        ):
            cleaned = self._clean_text(value)
            if cleaned:
                lines.append(f"{label}: {cleaned}")
        return "\n".join(lines)

    @staticmethod
    def _subject_id_from_key(profile_key: str) -> str:
        return hashlib.md5(profile_key.encode("utf-8")).hexdigest()[:24]

    @staticmethod
    def _table_exists(engine: DuckDBEngine, table_name: str) -> bool:
        rows = engine.query(
            "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name=? LIMIT 1",
            (table_name,),
        )
        return bool(rows)

    @staticmethod
    def _clean_text(value: object) -> str:
        text = str(value or "").strip()
        if text in {"", "-", "—", "－", "null", "NULL", "None"}:
            return ""
        return text

    @staticmethod
    def _normalize_name(value: str) -> str:
        return _WHITESPACE_RE.sub("", str(value or "").strip())

    @staticmethod
    def _normalize_id_no(value: str) -> str:
        return _WHITESPACE_RE.sub("", str(value or "").strip()).upper()

    @staticmethod
    def _mask_id_no(value: str) -> str:
        text = str(value or "").strip()
        if len(text) <= 8:
            return text
        return f"{text[:4]}********{text[-4:]}"

    @staticmethod
    def _mask_phone(value: str) -> str:
        digits = re.sub(r"\D+", "", str(value or ""))
        if len(digits) >= 7:
            return f"{digits[:3]}****{digits[-4:]}"
        return str(value or "")

    @staticmethod
    def _clip_text(value: str, limit: int) -> str:
        text = str(value or "").strip()
        if len(text) <= limit:
            return text
        return f"{text[: max(1, limit - 1)]}…"

    @staticmethod
    def _estimate_tokens(text: str) -> int:
        normalized = _WHITESPACE_RE.sub(" ", str(text or "").strip())
        if not normalized:
            return 0
        return max(1, int(math.ceil(len(normalized) / 2.2)))
