from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace

import pytest

from app.core.db_engine import DuckDBEngine
from app.core.fc_group_importer import (
    FcGroupImportSource,
    _clean_stats_by_file,
    _count_by_file,
    _extra_json_hints_by_file,
    _key_missing_by_file,
    import_fc_csv_group_into_db,
)
from app.core.fc_import_insert_plan import iter_insert_chunks
from app.core.fc_import_row_writer import ImportRowWriteRequest, write_import_rows
from app.core.fc_import_schema import FC_SCHEMAS
from app.core.fc_import_staging_metrics import (
    ImportStagingMetricsRequest,
    collect_import_staging_metrics,
)
from app.core.import_count_semantics import MAX_PUBLIC_IMPORT_COUNT


class _QueryEngine:
    def __init__(self, *responses: object) -> None:
        self.responses = list(responses)
        self.executions: list[str] = []

    def query(self, _sql: str, _params=()):
        if not self.responses:
            raise AssertionError("unexpected query")
        response = self.responses.pop(0)
        if isinstance(response, BaseException):
            raise response
        return response

    def execute(self, sql: str, _params=()) -> None:
        self.executions.append(sql)


@pytest.mark.parametrize(
    "response",
    (
        [],
        [(None,)],
        [(True,)],
        [(-1,)],
        [(MAX_PUBLIC_IMPORT_COUNT + 1,)],
    ),
)
def test_staging_count_empty_null_or_invalid_is_not_zero(response: object) -> None:
    engine = _QueryEngine(response)

    with pytest.raises(ValueError):
        collect_import_staging_metrics(
            engine=engine,  # type: ignore[arg-type]
            request=ImportStagingMetricsRequest(
                schema=SimpleNamespace(table="fc_account"),  # type: ignore[arg-type]
                staging_table="staging_rows",
            ),
            timings={},
        )


def test_staging_count_query_failure_is_not_zero() -> None:
    engine = _QueryEngine(RuntimeError("count unavailable"))

    with pytest.raises(RuntimeError, match="count unavailable"):
        collect_import_staging_metrics(
            engine=engine,  # type: ignore[arg-type]
            request=ImportStagingMetricsRequest(
                schema=SimpleNamespace(table="fc_account"),  # type: ignore[arg-type]
                staging_table="staging_rows",
            ),
            timings={},
        )


def test_explicit_staging_zero_remains_a_verified_zero() -> None:
    result = collect_import_staging_metrics(
        engine=_QueryEngine([(0,)]),  # type: ignore[arg-type]
        request=ImportStagingMetricsRequest(
            schema=SimpleNamespace(table="fc_account"),  # type: ignore[arg-type]
            staging_table="staging_rows",
        ),
        timings={},
    )

    assert result.total_rows == 0


def test_transaction_staging_metrics_require_every_exact_count() -> None:
    request = ImportStagingMetricsRequest(
        schema=SimpleNamespace(table="fc_transaction"),  # type: ignore[arg-type]
        staging_table="staging_rows",
    )

    with pytest.raises(ValueError, match="transaction_staging_missing_amount_count_invalid"):
        collect_import_staging_metrics(
            engine=_QueryEngine([(1, 0, None, 0)]),  # type: ignore[arg-type]
            request=request,
            timings={},
        )

    result = collect_import_staging_metrics(
        engine=_QueryEngine([(0, 0, 0, 0)]),  # type: ignore[arg-type]
        request=request,
        timings={},
    )
    assert result.total_rows == 0
    assert result.key_missing == {}


def test_transaction_staging_metrics_use_exact_counts_in_duckdb(tmp_path: Path) -> None:
    engine = DuckDBEngine(tmp_path / "staging-metrics.duckdb")
    try:
        engine.execute(
            """CREATE TABLE staging_rows(
                   txn_time_raw VARCHAR,
                   amount_raw VARCHAR,
                   card_no_raw VARCHAR,
                   acct_no_raw VARCHAR
               )"""
        )
        engine.execute(
            """INSERT INTO staging_rows VALUES
                   (NULL, '', 'card-a', 'acct-a'),
                   ('2026-01-01', '10.00', '', '')"""
        )

        result = collect_import_staging_metrics(
            engine=engine,
            request=ImportStagingMetricsRequest(
                schema=SimpleNamespace(table="fc_transaction"),  # type: ignore[arg-type]
                staging_table="staging_rows",
            ),
            timings={},
        )

        assert result.total_rows == 2
        assert result.key_missing == {
            "交易时间": 0.5,
            "交易金额": 0.5,
            "交易账号/卡号": 0.5,
        }
    finally:
        engine.close()


@pytest.mark.parametrize(
    "invalid",
    (None, True, -1, 1.5, "0", MAX_PUBLIC_IMPORT_COUNT + 1),
)
def test_insert_chunks_reject_unknown_or_invalid_total(invalid: object) -> None:
    with pytest.raises(ValueError, match="insert_total_rows_count_invalid"):
        list(iter_insert_chunks(invalid, 10))  # type: ignore[arg-type]


def test_insert_chunks_preserve_explicit_zero_and_reject_invalid_chunk_size() -> None:
    assert list(iter_insert_chunks(0, 10)) == []
    with pytest.raises(ValueError, match="insert_chunk_size_count_invalid"):
        list(iter_insert_chunks(1, 0))


def _row_write_request(*, total_rows: object = 1) -> ImportRowWriteRequest:
    return ImportRowWriteRequest(
        schema=FC_SCHEMAS["fc_account"],
        raw_table="fc_account_raw",
        norm_table="fc_account_norm",
        staging_table="staging_rows",
        dedup_table="dedup_rows",
        case_id="case-a",
        file_id="file-a",
        total_rows=total_rows,  # type: ignore[arg-type]
        mapping_missing=(),
        chunk_rows=100,
        precleaned_csv=True,
    )


def test_row_writer_missing_chunk_count_fails_before_raw_insert() -> None:
    engine = _QueryEngine([])

    with pytest.raises(RuntimeError, match="dedup_chunk_rows_row_unavailable"):
        write_import_rows(
            engine=engine,  # type: ignore[arg-type]
            request=_row_write_request(),
            timings={},
        )

    assert not any(sql.lstrip().startswith("INSERT INTO fc_account_raw") for sql in engine.executions)


def test_row_writer_missing_distinct_count_is_not_duplicate_zero() -> None:
    engine = _QueryEngine([(0,)], [(None,)])

    with pytest.raises(RuntimeError, match="staging_distinct_hashes_count_invalid"):
        write_import_rows(
            engine=engine,  # type: ignore[arg-type]
            request=_row_write_request(),
            timings={},
        )


@pytest.mark.parametrize(
    ("rows", "error"),
    (
        ([], "group_file_rows_file_id_missing"),
        ([("file-a", 1), ("file-a", 0), ("file-b", 0)], "group_file_rows_file_id_duplicate"),
        ([("file-a", 1), ("file-b", 0), ("file-c", 0)], "group_file_rows_file_id_unexpected"),
        ([("file-a", None), ("file-b", 0)], "group_file_rows_1_count_invalid"),
        ([(None, 1), ("file-b", 0)], "group_file_rows_file_id_invalid"),
    ),
)
def test_group_count_map_rejects_missing_duplicate_unexpected_or_null_file_fact(
    rows: object,
    error: str,
) -> None:
    with pytest.raises(ValueError, match=rf"^{error}$"):
        _count_by_file(
            _QueryEngine(rows),  # type: ignore[arg-type]
            "staging_rows",
            expected_file_ids=["file-a", "file-b"],
        )


def test_group_count_query_returns_explicit_zero_for_verified_empty_file(tmp_path: Path) -> None:
    engine = DuckDBEngine(tmp_path / "group-count.duckdb")
    try:
        engine.execute("CREATE TABLE staging_rows(file_id VARCHAR)")
        engine.execute("INSERT INTO staging_rows VALUES ('file-a'), ('file-a')")

        assert _count_by_file(
            engine,
            "staging_rows",
            expected_file_ids=["file-a", "file-b"],
        ) == {"file-a": 2, "file-b": 0}

        engine.execute("INSERT INTO staging_rows VALUES ('file-c')")
        with pytest.raises(ValueError, match="group_file_rows_file_id_unexpected"):
            _count_by_file(
                engine,
                "staging_rows",
                expected_file_ids=["file-a", "file-b"],
            )
    finally:
        engine.close()


def test_group_metric_queries_preserve_verified_zero_for_empty_file(tmp_path: Path) -> None:
    engine = DuckDBEngine(tmp_path / "group-metrics.duckdb")
    try:
        engine.execute(
            """CREATE TABLE staging_rows(
                   file_id VARCHAR,
                   txn_time_raw VARCHAR,
                   amount_raw VARCHAR,
                   card_no_raw VARCHAR,
                   acct_no_raw VARCHAR
               )"""
        )
        engine.execute(
            "INSERT INTO staging_rows VALUES ('file-a', NULL, '', 'card-a', 'acct-a')"
        )
        key_missing = _key_missing_by_file(
            engine,
            schema=FC_SCHEMAS["fc_transaction"],
            staging_table="staging_rows",
            expected_counts={"file-a": 1, "file-b": 0},
        )

        assert key_missing["file-a"] == {
            "交易时间": 1.0,
            "交易金额": 1.0,
            "交易账号/卡号": 0.0,
        }
        assert key_missing["file-b"] == {
            "交易时间": 0.0,
            "交易金额": 0.0,
            "交易账号/卡号": 0.0,
        }

        engine.execute(
            """CREATE TABLE fc_transaction_norm(
                   case_id VARCHAR,
                   file_id VARCHAR,
                   clean_amt_fixed INTEGER,
                   clean_amt_failed INTEGER,
                   clean_bal_failed INTEGER,
                   clean_dc_normalized INTEGER,
                   clean_dc_inferred INTEGER,
                   clean_suffix_fixed INTEGER
               )"""
        )
        engine.execute(
            """INSERT INTO fc_transaction_norm
               VALUES ('case-a', 'file-a', 1, 0, 0, 1, 0, 0)"""
        )
        clean_stats = _clean_stats_by_file(
            engine,
            schema=FC_SCHEMAS["fc_transaction"],
            norm_table="fc_transaction_norm",
            case_id="case-a",
            expected_counts={"file-a": 1, "file-b": 0},
        )

        assert clean_stats["file-a"]["amt_fixed"] == 1
        assert clean_stats["file-a"]["dc_norm"] == 1
        assert clean_stats["file-b"] == {
            "amt_fixed": 0,
            "amt_failed": 0,
            "bal_failed": 0,
            "dc_norm": 0,
            "dc_infer": 0,
            "suffix_fixed": 0,
        }
    finally:
        engine.close()


def test_group_metric_maps_require_complete_exact_file_ids() -> None:
    schema = FC_SCHEMAS["fc_transaction"]

    with pytest.raises(ValueError, match="group_key_missing_file_id_missing"):
        _key_missing_by_file(
            _QueryEngine([("file-a", 1, 0, 0, 0)]),  # type: ignore[arg-type]
            schema=schema,
            staging_table="staging_rows",
            expected_counts={"file-a": 1, "file-b": 0},
        )

    with pytest.raises(ValueError, match="group_clean_stats_2_count_invalid"):
        _clean_stats_by_file(
            _QueryEngine(
                [
                    ("file-a", 1, None, 0, 0, 0, 0, 0),
                    ("file-b", 0, 0, 0, 0, 0, 0, 0),
                ]
            ),  # type: ignore[arg-type]
            schema=schema,
            norm_table="fc_transaction_norm",
            case_id="case-a",
            expected_counts={"file-a": 1, "file-b": 0},
        )


def test_group_quality_query_failure_is_not_clean_success() -> None:
    with pytest.raises(RuntimeError, match="quality unavailable"):
        _extra_json_hints_by_file(
            _QueryEngine(RuntimeError("quality unavailable")),  # type: ignore[arg-type]
            schema=FC_SCHEMAS["fc_account"],
            norm_table="fc_account_norm",
            case_id="case-a",
            sources=[
                FcGroupImportSource(
                    file_id="file-a",
                    display_name="a.csv",
                    path=Path("a.csv"),
                    rows_total=1,
                )
            ],
            mapping_missing=["交易账号"],
        )


@pytest.mark.parametrize("invalid_rows_total", (None, True, -1, "0"))
def test_group_sources_reject_invalid_counts_before_engine_access(invalid_rows_total: object) -> None:
    sources = [
        FcGroupImportSource("file-a", "a.csv", Path("a.csv"), invalid_rows_total),  # type: ignore[arg-type]
        FcGroupImportSource("file-b", "b.csv", Path("b.csv"), 0),
    ]

    with pytest.raises(ValueError, match="group_source_rows_total_count_invalid"):
        import_fc_csv_group_into_db(
            engine=object(),  # type: ignore[arg-type]
            case_id="case-a",
            kind="fc_transaction",
            sources=sources,
            headers=[],
        )


def test_group_sources_reject_duplicate_file_ids_before_engine_access() -> None:
    sources = [
        FcGroupImportSource("file-a", "a.csv", Path("a.csv"), 0),
        FcGroupImportSource("file-a", "b.csv", Path("b.csv"), 0),
    ]

    with pytest.raises(ValueError, match="group_source_file_id_duplicate"):
        import_fc_csv_group_into_db(
            engine=object(),  # type: ignore[arg-type]
            case_id="case-a",
            kind="fc_transaction",
            sources=sources,
            headers=[],
        )
