from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, NoReturn, Optional

from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)

_IMPORT_ACCELERATOR_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"


class ImportAcceleratorUnavailableError(RuntimeError):
    pass


def _raise_import_accelerator_capability_unavailable() -> NoReturn:
    """Reject before case arguments or executable configuration are inspected.

    The shared managed-process authority does not yet define an
    import-accelerator-specific, versioned capability admission. A global
    execution-authority status therefore cannot authorize this consumer.
    """

    _import_accelerator_capability_health()
    raise ImportAcceleratorUnavailableError(_IMPORT_ACCELERATOR_CAPABILITY_UNAVAILABLE)


def _import_accelerator_capability_health() -> ExecutionAuthorityHealth:
    return unadmitted_execution_capability_health(execution_authority_health())


@dataclass(frozen=True)
class PreparedCsv:
    encoding: str
    rows_total: int
    columns_total: int
    header_preview: list[str]
    sample_rows: list[list[str]]
    preclean_rows: Optional[int] = None
    output: Optional[Path] = None


@dataclass(frozen=True)
class PreparedCsvWithProfile:
    prepared: PreparedCsv
    profiles: Optional["CsvColumnProfiles"]
    elapsed_s: Optional[float] = None


@dataclass(frozen=True)
class PrepareCsvBatchItem:
    path: Path
    out_path: Optional[Path] = None
    limit: int = 0
    encoding: Optional[str] = None
    clean_if_needed: bool = False


@dataclass(frozen=True)
class PrepareCsvWithProfileBatchItem:
    path: Path
    limit: int = 0
    encoding: Optional[str] = None


@dataclass(frozen=True)
class ColumnFeatureSamples:
    longest: Optional[str] = None
    min_amount: Optional[str] = None
    max_amount: Optional[str] = None
    date_like: Optional[str] = None


@dataclass(frozen=True)
class CsvColumnProfile:
    index: int
    source_index: int
    header: str
    non_empty: int
    non_empty_ratio: float
    amount_like: int
    amount_like_ratio: float
    signed_amount: int
    positive_amount: int
    negative_amount: int
    date_like: int
    date_like_ratio: float
    datetime_like: int
    account_like: int
    id_no_like: int
    phone_like: int
    ip_like: int
    mac_like: int
    distinct_count: Optional[int]
    distinct_limit_exceeded: bool
    first_non_empty: list[str]
    fixed_seed_samples: list[str]
    feature_samples: ColumnFeatureSamples


@dataclass(frozen=True)
class CsvColumnProfiles:
    encoding: str
    rows_total: int
    columns_total: int
    columns: list[CsvColumnProfile]


def detect_encoding(path: Path) -> str:
    """Raw-file encoding probe kept for non-CSV callers; CSV paths should use prepare_csv().encoding."""
    _raise_import_accelerator_capability_unavailable()


def prepare_csv(
    path: Path,
    *,
    limit: int,
    out_path: Optional[Path] = None,
    encoding: Optional[str] = None,
) -> PreparedCsv:
    _raise_import_accelerator_capability_unavailable()


def prepare_csv_with_profile(
    path: Path,
    *,
    limit: int,
    encoding: Optional[str] = None,
) -> PreparedCsvWithProfile:
    _raise_import_accelerator_capability_unavailable()


def prepare_csv_with_profile_batch(
    items: Iterable[PrepareCsvWithProfileBatchItem],
) -> list[PreparedCsvWithProfile]:
    _raise_import_accelerator_capability_unavailable()


def prepare_csv_batch(items: Iterable[PrepareCsvBatchItem]) -> list[PreparedCsv]:
    _raise_import_accelerator_capability_unavailable()


def _prepared_csv_from_payload(
    payload: dict,
    *,
    out_path: Optional[Path],
    require_output: bool = False,
) -> PreparedCsv:
    output = _prepared_csv_output_path(payload.get("output"))
    if output is not None and not output.exists():
        raise ImportAcceleratorUnavailableError("Rust import accelerator prepare-csv returned missing output")
    if require_output and output is None:
        output = out_path
    if require_output and (output is None or not output.exists()):
        raise ImportAcceleratorUnavailableError("Rust import accelerator prepare-csv did not create output")
    encoding = str(payload.get("encoding") or "").strip().lower()
    if encoding not in {"utf-8-sig", "utf-8", "gb18030", "gbk"}:
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV encoding")
    headers_payload = payload.get("header_preview")
    sample_rows_payload = payload.get("sample_rows")
    if not isinstance(headers_payload, list) or not isinstance(sample_rows_payload, list):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid prepared CSV preview")
    sample_rows: list[list[str]] = []
    for row in sample_rows_payload:
        if not isinstance(row, list):
            raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid prepared CSV row")
        sample_rows.append([str(cell or "") for cell in row])
    try:
        preclean_rows = payload.get("preclean_rows")
        return PreparedCsv(
            encoding=encoding,
            rows_total=max(0, int(payload.get("rows_total") or 0)),
            columns_total=max(0, int(payload.get("columns_total") or len(headers_payload))),
            header_preview=[str(item or "") for item in headers_payload],
            sample_rows=sample_rows,
            preclean_rows=None if preclean_rows is None else max(0, int(preclean_rows or 0)),
            output=output,
        )
    except Exception as exc:
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid prepared CSV counts") from exc


def _prepared_csv_with_profile_from_payload(payload: dict) -> PreparedCsvWithProfile:
    prepared = _prepared_csv_from_payload(payload, out_path=None, require_output=False)
    profile_payload = payload.get("column_profiles")
    profiles = _column_profiles_from_payload(profile_payload) if isinstance(profile_payload, dict) else None
    elapsed_value = payload.get("elapsed_s")
    try:
        elapsed_s = None if elapsed_value is None else max(0.0, float(elapsed_value or 0.0))
    except Exception as exc:
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid profile elapsed time") from exc
    return PreparedCsvWithProfile(prepared=prepared, profiles=profiles, elapsed_s=elapsed_s)


def _prepared_csv_output_path(value: Any) -> Optional[Path]:
    if value is None:
        return None
    text = str(value).strip()
    return Path(text) if text else None


def profile_csv_columns(path: Path, *, encoding: Optional[str] = None) -> CsvColumnProfiles:
    _raise_import_accelerator_capability_unavailable()


def _column_profiles_from_payload(payload: Any) -> CsvColumnProfiles:
    if not isinstance(payload, dict):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV column profiles")
    encoding_value = str(payload.get("encoding") or "").strip().lower()
    if encoding_value not in {"utf-8-sig", "utf-8", "gb18030", "gbk"}:
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV profile encoding")
    columns_payload = payload.get("columns")
    if not isinstance(columns_payload, list):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV column profiles")
    try:
        rows_total = max(0, int(payload.get("rows_total") or 0))
        columns_total = max(0, int(payload.get("columns_total") or len(columns_payload)))
        columns = [_parse_column_profile(item) for item in columns_payload]
    except Exception as exc:
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV profile counts") from exc
    if columns_total != len(columns):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned mismatched CSV profile columns")
    return CsvColumnProfiles(
        encoding=encoding_value,
        rows_total=rows_total,
        columns_total=columns_total,
        columns=columns,
    )


def _parse_column_profile(payload: Any) -> CsvColumnProfile:
    if not isinstance(payload, dict):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV column profile")
    feature_samples_payload = payload.get("feature_samples")
    if not isinstance(feature_samples_payload, dict):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV feature samples")
    return CsvColumnProfile(
        index=max(0, int(payload.get("index") or 0)),
        source_index=max(0, int(payload.get("source_index") or 0)),
        header=str(payload.get("header") or ""),
        non_empty=max(0, int(payload.get("non_empty") or 0)),
        non_empty_ratio=float(payload.get("non_empty_ratio") or 0.0),
        amount_like=max(0, int(payload.get("amount_like") or 0)),
        amount_like_ratio=float(payload.get("amount_like_ratio") or 0.0),
        signed_amount=max(0, int(payload.get("signed_amount") or 0)),
        positive_amount=max(0, int(payload.get("positive_amount") or 0)),
        negative_amount=max(0, int(payload.get("negative_amount") or 0)),
        date_like=max(0, int(payload.get("date_like") or 0)),
        date_like_ratio=float(payload.get("date_like_ratio") or 0.0),
        datetime_like=max(0, int(payload.get("datetime_like") or 0)),
        account_like=max(0, int(payload.get("account_like") or 0)),
        id_no_like=max(0, int(payload.get("id_no_like") or 0)),
        phone_like=max(0, int(payload.get("phone_like") or 0)),
        ip_like=max(0, int(payload.get("ip_like") or 0)),
        mac_like=max(0, int(payload.get("mac_like") or 0)),
        distinct_count=(
            None
            if payload.get("distinct_count") is None
            else max(0, int(payload.get("distinct_count") or 0))
        ),
        distinct_limit_exceeded=bool(payload.get("distinct_limit_exceeded")),
        first_non_empty=_string_list(payload.get("first_non_empty")),
        fixed_seed_samples=_string_list(payload.get("fixed_seed_samples")),
        feature_samples=ColumnFeatureSamples(
            longest=_optional_string(feature_samples_payload.get("longest")),
            min_amount=_optional_string(feature_samples_payload.get("min_amount")),
            max_amount=_optional_string(feature_samples_payload.get("max_amount")),
            date_like=_optional_string(feature_samples_payload.get("date_like")),
        ),
    )


def _optional_string(value: Any) -> Optional[str]:
    if value is None:
        return None
    return str(value)


def _string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        raise ImportAcceleratorUnavailableError("Rust import accelerator returned invalid CSV profile samples")
    return [str(item or "") for item in value]


def accelerated_file_hashes(path: Path, algos: Iterable[str]) -> Optional[dict[str, str]]:
    return None


def excel_to_csv(path: Path, *, out_path: Path) -> Path:
    _raise_import_accelerator_capability_unavailable()


def accelerated_excel_to_csv(path: Path, *, out_path: Path) -> bool:
    return False


def split_excel_account_sections(path: Path, *, output_dir: Path) -> list[tuple[Path, str]]:
    _raise_import_accelerator_capability_unavailable()


def _run_accelerator(args: list[str]) -> Optional[dict]:
    return None
