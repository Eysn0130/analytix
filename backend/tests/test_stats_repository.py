from __future__ import annotations

from app.repositories.stats_repository import _trim_chart_dashboard_for_response


def test_trim_chart_dashboard_for_response_bounds_dashboard_only_lists() -> None:
    source = {
        "flow": {
            "inbound_items": [{"label": f"in-{index}"} for index in range(25)],
            "outbound_items": [{"label": f"out-{index}"} for index in range(55)],
            "totals": {"in_count": 25, "out_count": 55},
        },
        "counterparties": {
            "views": {
                "name": {
                    "top_items": [{"label": "top"}],
                    "all_items": [{"label": f"name-{index}"} for index in range(60)],
                },
                "account": {
                    "top_items": [],
                    "all_items": [{"label": f"account-{index}"} for index in range(12)],
                },
            }
        },
    }

    trimmed = _trim_chart_dashboard_for_response(source)

    assert len(trimmed["flow"]["inbound_items"]) == 20
    assert trimmed["flow"]["inbound_item_count"] == 25
    assert trimmed["flow"]["inbound_items_truncated"] is True
    assert len(trimmed["flow"]["outbound_items"]) == 20
    assert trimmed["flow"]["outbound_item_count"] == 55
    assert trimmed["flow"]["outbound_items_truncated"] is True

    name_view = trimmed["counterparties"]["views"]["name"]
    assert name_view["top_items"] == [{"label": "top"}]
    assert len(name_view["all_items"]) == 50
    assert name_view["all_item_count"] == 60
    assert name_view["all_items_truncated"] is True

    account_view = trimmed["counterparties"]["views"]["account"]
    assert len(account_view["all_items"]) == 12
    assert account_view["all_item_count"] == 12
    assert account_view["all_items_truncated"] is False

    assert len(source["flow"]["inbound_items"]) == 25
    assert "inbound_item_count" not in source["flow"]
