"""Tests for download_hf_dataset_parquet() (Issue E)."""

from __future__ import annotations

import json
import sys
from typing import TYPE_CHECKING
from unittest.mock import MagicMock, patch

if TYPE_CHECKING:
    from pathlib import Path

import pytest


@pytest.fixture
def tmp_cache(tmp_path: Path) -> Path:
    """Create a temporary cache directory."""
    cache_dir = tmp_path / "test_provider"
    cache_dir.mkdir()
    return tmp_path


@pytest.mark.asyncio
async def test_parquet_download_creates_jsonl(tmp_cache: Path) -> None:
    """download_hf_dataset_parquet should create JSONL from dataset rows."""
    from codeforge.evaluation.cache import download_hf_dataset_parquet

    fake_rows = [
        {"id": "1", "question": "hello", "answer": "world"},
        {"id": "2", "question": "foo", "answer": "bar"},
    ]
    mock_dataset = MagicMock()
    mock_dataset.__iter__ = MagicMock(return_value=iter(fake_rows))
    mock_dataset.__len__ = MagicMock(return_value=len(fake_rows))

    mock_ds_module = MagicMock()
    mock_ds_module.load_dataset = MagicMock(return_value=mock_dataset)

    with patch.dict("sys.modules", {"datasets": mock_ds_module}):
        result = await download_hf_dataset_parquet(
            dataset="test/dataset",
            split="test",
            provider_name="test_provider",
            filename="test.jsonl",
            base_dir=str(tmp_cache),
        )

    assert result.exists()
    assert result.name == "test.jsonl"
    lines = result.read_text().strip().split("\n")
    assert len(lines) == 2
    assert json.loads(lines[0])["id"] == "1"
    assert json.loads(lines[1])["id"] == "2"


@pytest.mark.asyncio
async def test_parquet_download_uses_cache(tmp_cache: Path) -> None:
    """If cached file exists, download_hf_dataset_parquet should return it."""
    from codeforge.evaluation.cache import download_hf_dataset_parquet

    # Pre-create cached file
    cache_dir = tmp_cache / "test_provider"
    cache_dir.mkdir(exist_ok=True)
    cached_file = cache_dir / "test.jsonl"
    cached_file.write_text('{"id": "cached"}\n')

    result = await download_hf_dataset_parquet(
        dataset="test/dataset",
        split="test",
        provider_name="test_provider",
        filename="test.jsonl",
        base_dir=str(tmp_cache),
    )

    assert result == cached_file
    # Verify the cached content was returned (no re-download)
    assert json.loads(result.read_text().strip())["id"] == "cached"


@pytest.mark.asyncio
async def test_parquet_download_missing_library(tmp_cache: Path) -> None:
    """If datasets library is not installed, should raise RuntimeError."""
    # Temporarily hide the datasets module
    hidden = sys.modules.pop("datasets", None)
    try:
        with patch.dict("sys.modules", {"datasets": None}):
            from codeforge.evaluation.cache import download_hf_dataset_parquet

            with pytest.raises(RuntimeError, match=r"datasets.*library.*required"):
                await download_hf_dataset_parquet(
                    dataset="test/dataset",
                    split="test",
                    provider_name="test_provider",
                    filename="missing.jsonl",
                    base_dir=str(tmp_cache),
                )
    finally:
        if hidden is not None:
            sys.modules["datasets"] = hidden


@pytest.mark.asyncio
async def test_parquet_download_propagates_hf_token(tmp_cache: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """HF_TOKEN env var should be passed to datasets.load_dataset()."""
    monkeypatch.setenv("HF_TOKEN", "hf_test_token_123")

    fake_rows = [{"id": "1"}]
    mock_dataset = MagicMock()
    mock_dataset.__iter__ = MagicMock(return_value=iter(fake_rows))
    mock_dataset.__len__ = MagicMock(return_value=1)

    mock_ds_module = MagicMock()
    mock_ds_module.load_dataset = MagicMock(return_value=mock_dataset)

    with patch.dict("sys.modules", {"datasets": mock_ds_module}):
        from codeforge.evaluation.cache import download_hf_dataset_parquet

        await download_hf_dataset_parquet(
            dataset="test/dataset",
            split="test",
            provider_name="test_provider",
            filename="token_test.jsonl",
            base_dir=str(tmp_cache),
        )

    # Verify load_dataset was called with the token
    mock_ds_module.load_dataset.assert_called_once()
    call_kwargs = mock_ds_module.load_dataset.call_args
    assert call_kwargs.kwargs.get("token") == "hf_test_token_123"


@pytest.mark.asyncio
async def test_parquet_download_cleans_up_on_error(tmp_cache: Path) -> None:
    """On download failure, tmp file should be cleaned up."""
    mock_ds_module = MagicMock()
    mock_ds_module.load_dataset = MagicMock(side_effect=ValueError("download failed"))

    with patch.dict("sys.modules", {"datasets": mock_ds_module}):
        from codeforge.evaluation.cache import download_hf_dataset_parquet

        with pytest.raises(RuntimeError, match="failed to download"):
            await download_hf_dataset_parquet(
                dataset="test/dataset",
                split="test",
                provider_name="test_provider",
                filename="error_test.jsonl",
                base_dir=str(tmp_cache),
            )

    # Verify no .tmp file remains
    tmp_files = list((tmp_cache / "test_provider").glob("*.tmp"))
    assert len(tmp_files) == 0


@pytest.mark.asyncio
async def test_livecodebench_fallback_to_http() -> None:
    """LiveCodeBench provider should fall back to HTTP API if datasets library is missing."""
    from codeforge.evaluation.providers.livecodebench import LiveCodeBenchProvider

    provider = LiveCodeBenchProvider(
        tasks=[
            {"question_id": "1", "question_title": "Test", "question_content": "test", "difficulty": "easy"},
        ]
    )
    tasks = await provider.load_tasks()
    assert len(tasks) == 1
    assert tasks[0].id == "lcb_1"
