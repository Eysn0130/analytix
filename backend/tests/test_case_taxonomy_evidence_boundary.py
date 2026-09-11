from app.domain.case_taxonomy import (
    build_case_direction_context,
    build_case_direction_focus,
    infer_case_analysis_skeleton_from_text,
)


def test_untrusted_project_words_do_not_select_a_case_scenario() -> None:
    note = "工程招投标项目可能存在贿赂和利益输送"

    assert infer_case_analysis_skeleton_from_text(note, ["工程", "招投标"]) == "generic_ecrime"
    assert build_case_direction_context("", ["工程", "招投标"], note)["skeleton_key"] == "generic_ecrime"


def test_explicit_supervision_type_uses_generic_evidence_focus() -> None:
    prohibited = ("项目款", "工程款", "利益输送", "招投标", "施工方", "隐匿受益人")

    for theme in (
        "summary_accounts",
        "rules",
        "counterparty",
        "unknown",
        "same_name",
        "trace",
        "path",
        "drilldown",
        "shared_counterparty",
        "brief",
        "report",
        "review",
    ):
        focus = build_case_direction_focus(
            "国企职务犯罪 / 工程招投标（兼监察）",
            ["工程", "招投标"],
            theme,
            "用户提供的分类不是案件事实",
        )
        assert focus
        assert not any(marker in focus for marker in prohibited)
