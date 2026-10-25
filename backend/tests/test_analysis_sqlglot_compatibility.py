"""Real-parser compatibility corpus; no case storage or SQL engine execution."""

from __future__ import annotations

import re

import pytest

from app.repositories import analysis_repository as owner


@pytest.fixture
def repository():
    instance = object.__new__(owner.AnalysisRepository)
    looked_up: list[str] = []

    def table_exists(_engine, table: str) -> bool:
        looked_up.append(table)
        return table == "analysis_txn_detail_idx"

    instance._table_exists = table_exists
    return instance, looked_up


@pytest.mark.parametrize(
    ("sql", "preview", "ctes"),
    [
        ("SELECT amount FROM analysis_txn_detail_idx", False, []),
        (
            "WITH scoped AS (SELECT amount FROM analysis_txn_detail_idx) "
            "SELECT amount FROM scoped",
            False,
            ["scoped"],
        ),
        ("SELECT amount FROM main.analysis_txn_detail_idx", False, []),
        (
            "SELECT EXTRACT(HOUR FROM txn_ts) AS h FROM analysis_txn_detail_idx",
            False,
            [],
        ),
        ("SELECT 'drop;read_csv' AS note FROM analysis_txn_detail_idx -- ATTACH", False, []),
        ("SELECT * FROM analysis_txn_detail_idx LIMIT 5", True, []),
        ("SELECT COUNT(*) FROM analysis_txn_detail_idx LIMIT 5", True, []),
    ],
)
def test_real_parser_preserves_allowed_sql(repository, sql, preview, ctes) -> None:
    instance, looked_up = repository
    statement, tables, actual_ctes = instance._validate_workbench_sql(
        object(), sql, allow_select_star_with_limit=preview
    )
    assert statement
    assert tables == ["analysis_txn_detail_idx"]
    assert actual_ctes == ctes
    assert looked_up == ["analysis_txn_detail_idx"]


@pytest.mark.parametrize(
    ("sql", "preview", "message", "lookups"),
    [
        ("", False, "run_case_sql requires a non-empty SQL statement", []),
        (
            "SELECT amount FROM analysis_txn_detail_idx; SELECT 1",
            False,
            "Controlled Case Workbench accepts exactly one SELECT/WITH statement",
            [],
        ),
        ("DELETE FROM analysis_txn_detail_idx", False, "Controlled Case Workbench SQL must start with SELECT or WITH", []),
        ("SELECT (", False, "Controlled Case Workbench SQL parse failed:", []),
        (
            "SELECT amount FROM analysis_txn_detail_idx UNION "
            "SELECT amount FROM analysis_txn_detail_idx",
            False,
            "Controlled Case Workbench SQL must be a SELECT/WITH query",
            [],
        ),
        (
            "SELECT amount FROM read_csv_auto('/synthetic/not-opened.csv')",
            False,
            "external file/network/secret function is not allowed:",
            [],
        ),
        ("SELECT * FROM analysis_txn_detail_idx LIMIT 5", False, "unbounded SELECT * is not allowed;", []),
        ("SELECT * FROM analysis_txn_detail_idx", True, "unbounded SELECT * is not allowed;", []),
        ("SELECT COUNT(*) FROM analysis_txn_detail_idx", False, "unbounded SELECT * is not allowed;", []),
        ("SELECT * FROM fc_txn_raw", False, "unbounded SELECT * is not allowed;", []),
        ("SELECT * FROM fc_txn_raw LIMIT 5", True, "raw/source tables are not allowed in Controlled Case Workbench", []),
        (
            "SELECT amount FROM other_case.analysis_txn_detail_idx",
            False,
            "cross-database or attached-catalog table references are not allowed",
            [],
        ),
        (
            "SELECT * FROM other_case.fc_txn_raw",
            False,
            "unbounded SELECT * is not allowed;",
            [],
        ),
        (
            "SELECT amount FROM unapproved_table",
            False,
            "Controlled Case Workbench can only read cleaned fc_*_norm and approved analysis tables:",
            [],
        ),
        (
            "SELECT amount FROM analysis_txn_missing",
            False,
            "requested workbench table is unavailable in the current case:",
            ["analysis_txn_missing"],
        ),
        (
            "SELECT 1",
            False,
            "Controlled Case Workbench SQL must reference at least one cleaned fc_*_norm or approved analysis table",
            [],
        ),
    ],
)
def test_real_parser_preserves_rejection_priority(repository, sql, preview, message, lookups) -> None:
    instance, looked_up = repository
    with pytest.raises(ValueError, match="^" + re.escape(message)):
        instance._validate_workbench_sql(object(), sql, allow_select_star_with_limit=preview)
    assert looked_up == lookups


def test_missing_parser_fails_closed_before_table_lookup(repository, monkeypatch) -> None:
    instance, looked_up = repository
    monkeypatch.setattr(owner, "_sqlglot_parse", None)
    with pytest.raises(ValueError, match="^Structured SQL parser is unavailable;"):
        instance._validate_workbench_sql(object(), "SELECT amount FROM analysis_txn_detail_idx")
    assert looked_up == []
