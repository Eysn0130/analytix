from __future__ import annotations


def account_key_expr() -> str:
    acct_key = "COALESCE(NULLIF(clean_acct_no,''), acct_no_norm, acct_no)"
    card_key = "COALESCE(NULLIF(clean_card_no,''), card_no_norm, card_no)"
    return f"COALESCE(NULLIF(TRIM({acct_key}), ''), NULLIF(TRIM({card_key}), ''))"


__all__ = ["account_key_expr"]
