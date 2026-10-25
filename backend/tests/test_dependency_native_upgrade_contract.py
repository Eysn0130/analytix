"""Bounded synthetic checks of the actual upgraded Python/native consumers."""

from __future__ import annotations

from decimal import Decimal
from importlib.metadata import version
from io import BytesIO

import duckdb
import pypdfium2 as pdfium
import pytest
from cryptography.exceptions import InvalidTag
from cryptography.hazmat.backends.openssl.backend import backend
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import BaseModel, ConfigDict
from pypdf import PdfWriter


class SyntheticBody(BaseModel):
    model_config = ConfigDict(extra="forbid")
    amount: Decimal


def test_exact_target_and_preserved_security_versions() -> None:
    expected = {
        "fastapi": "0.153.0",
        "pydantic": "2.14.1",
        "pydantic-core": "2.50.1",
        "duckdb": "1.5.6",
        "cryptography": "50.0.2",
        "pypdfium2": "5.14.0",
        "opentelemetry-api": "1.45.1",
        "certifi": "2026.10.19",
        "urllib3": "2.8.0",
        "pypdf": "6.21.2",
        "anyio": "4.15.1",
    }
    assert {name: version(name) for name in expected} == expected


def test_duckdb_native_null_decimal_and_unicode() -> None:
    with duckdb.connect(":memory:") as connection:
        row = connection.execute(
            "SELECT CAST(NULL AS DECIMAL(18,2)), "
            "CAST('12.34' AS DECIMAL(18,2)), '合成'"
        ).fetchone()
    assert row == (None, Decimal("12.34"), "合成")


def test_crypto_native_roundtrip_and_wrong_tag_rejection() -> None:
    cipher = AESGCM(bytes(range(32)))
    nonce, plaintext, associated = bytes(range(12)), b"synthetic payload", b"context"
    encrypted = cipher.encrypt(nonce, plaintext, associated)
    assert cipher.decrypt(nonce, encrypted, associated) == plaintext
    changed = encrypted[:-1] + bytes([encrypted[-1] ^ 1])
    with pytest.raises(InvalidTag):
        cipher.decrypt(nonce, changed, associated)
    assert backend.openssl_version_text().startswith("OpenSSL ")


def test_pdfium_native_renders_a_bounded_generated_pdf() -> None:
    writer, stream = PdfWriter(), BytesIO()
    writer.add_blank_page(width=32, height=24)
    writer.write(stream)
    with pdfium.PdfDocument(stream.getvalue()) as document:
        assert len(document) == 1
        with document[0] as page:
            with page.render(scale=1) as bitmap:
                image = bitmap.to_pil()
                try:
                    assert image.size == (32, 24)
                    assert image.mode in {"RGB", "RGBA", "RGBX"}
                    assert 0 < len(image.tobytes()) <= 32 * 24 * 4
                finally:
                    image.close()


@pytest.mark.parametrize("content", [b"%PDF-1.7\ninvalid", b"%PDF-1.7\n1 0 obj\n"])
def test_pdfium_native_rejects_malformed_and_truncated_pdf(content: bytes) -> None:
    with pytest.raises(pdfium.PdfiumError):
        pdfium.PdfDocument(content)


def test_fastapi_pydantic_validation_and_response_contract() -> None:
    app = FastAPI()

    @app.post("/synthetic", response_model=SyntheticBody)
    def echo(body: SyntheticBody) -> SyntheticBody:
        return body

    with TestClient(app) as client:
        accepted = client.post("/synthetic", json={"amount": "12.34"})
        assert accepted.status_code == 200
        assert accepted.json() == {"amount": "12.34"}
        for body in [{"amount": "invalid"}, {"amount": "12.34", "extra": True}, {}]:
            assert client.post("/synthetic", json=body).status_code == 422
