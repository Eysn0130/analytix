from __future__ import annotations

import duckdb

from app.repositories.cleaning_execution_context import CleaningExecutionContext
from app.repositories.cleaning_front_steps import step_failed_reversal
from app.repositories.cleaning_scope_state import CleaningScopeState
from app.repositories.cleaning_step_context import CleaningStepContext


def _context(case_id: str = "case-1") -> CleaningStepContext:
    scope = CleaningScopeState(txn_where="case_id=?", txn_params=[case_id])
    return CleaningStepContext(
        case_id=case_id,
        scope_state=scope,
        execution_context=CleaningExecutionContext(),
    )


def test_failed_reversal_marks_negative_reversal_details_without_positive_keyword_false_positive() -> None:
    con = duckdb.connect(":memory:")
    con.execute(
        """CREATE TABLE fc_transaction_norm (
               id INTEGER,
               case_id VARCHAR,
               query_feedback_reason VARCHAR,
               summary VARCHAR,
               txn_type VARCHAR,
               remark VARCHAR,
               orig_amount VARCHAR,
               amount VARCHAR,
               clean_failed INTEGER DEFAULT 0,
               clean_reversal INTEGER DEFAULT 0
           )"""
    )
    con.executemany(
        """INSERT INTO fc_transaction_norm
           (id, case_id, query_feedback_reason, summary, txn_type, remark, orig_amount, amount)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
        [
            (1, "case-1", "查询失败", "", "", "", "10.00", "10.00"),
            (2, "case-1", "冲正", "", "", "", "10.00", "10.00"),
            (3, "case-1", "", "冲正", "转账", "", "-10.00", "-10.00"),
            (4, "case-1", "", "", "消费退货", "", "", "-20.00"),
            (5, "case-1", "", "", "", "抹账", "-30.00", "-30.00"),
            (6, "case-1", "", "冲正", "转账", "", "40.00", "40.00"),
            (7, "case-1", "", "账务调整", "", "", "-50.00", "-50.00"),
            (8, "other-case", "冲正", "冲正", "", "", "-60.00", "-60.00"),
        ],
    )

    assert step_failed_reversal(_context("case-1"), con) == (1, 4, 1, 4)

    rows = {
        row[0]: (row[1], row[2])
        for row in con.execute("SELECT id, clean_failed, clean_reversal FROM fc_transaction_norm ORDER BY id").fetchall()
    }
    assert rows[1] == (1, 0)
    assert rows[2] == (0, 1)
    assert rows[3] == (0, 1)
    assert rows[4] == (0, 1)
    assert rows[5] == (0, 1)
    assert rows[6] == (0, 0)
    assert rows[7] == (0, 0)
    assert rows[8] == (0, 0)
