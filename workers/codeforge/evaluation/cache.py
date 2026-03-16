"""Dataset download and caching for external benchmark providers.

Downloads benchmark datasets from HuggingFace or other URLs once,
caches them in data/benchmarks/{provider_name}/, and returns the
local file path for subsequent loads.
"""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    import httpx

logger = structlog.get_logger(__name__)

_DEFAULT_CACHE_DIR = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(__file__)))),
    "data",
    "benchmarks",
)


def get_cache_dir(provider_name: str, base_dir: str = "") -> Path:
    """Return the cache directory for a provider, creating it if needed."""
    base = Path(base_dir) if base_dir else Path(_DEFAULT_CACHE_DIR)
    cache_dir = base / provider_name
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir


def get_cached_path(provider_name: str, filename: str, base_dir: str = "") -> Path | None:
    """Return the cached file path if it exists, otherwise None."""
    cache_dir = get_cache_dir(provider_name, base_dir)
    path = cache_dir / filename
    if path.exists() and path.stat().st_size > 0:
        return path
    return None


async def download_dataset(
    url: str,
    provider_name: str,
    filename: str,
    expected_sha256: str = "",
    base_dir: str = "",
) -> Path:
    """Download a dataset file if not already cached.

    Args:
        url: URL to download from.
        provider_name: Provider name (used as subdirectory).
        filename: Local filename to save as.
        expected_sha256: Optional SHA-256 hex digest to verify integrity.
        base_dir: Override base cache directory.

    Returns:
        Path to the cached file.

    Raises:
        RuntimeError: If download fails or checksum doesn't match.
    """
    cached = get_cached_path(provider_name, filename, base_dir)
    if cached is not None:
        if expected_sha256 and not _verify_checksum(cached, expected_sha256):
            logger.warning("cached file checksum mismatch, re-downloading", path=str(cached))
        else:
            logger.debug("using cached dataset", path=str(cached))
            return cached

    cache_dir = get_cache_dir(provider_name, base_dir)
    target = cache_dir / filename
    tmp_path = target.with_suffix(".tmp")

    log = logger.bind(url=url, target=str(target))
    log.info("downloading dataset")

    try:
        import httpx

        async with httpx.AsyncClient(timeout=120.0, follow_redirects=True) as client:
            resp = await client.get(url)
            resp.raise_for_status()
            tmp_path.write_bytes(resp.content)

        if expected_sha256 and not _verify_checksum(tmp_path, expected_sha256):
            tmp_path.unlink(missing_ok=True)
            msg = f"checksum mismatch for {filename}"
            raise RuntimeError(msg)

        tmp_path.rename(target)
        log.info("dataset downloaded", size_bytes=target.stat().st_size)
        return target

    except Exception as exc:
        tmp_path.unlink(missing_ok=True)
        msg = f"failed to download {url}: {exc}"
        raise RuntimeError(msg) from exc


def _verify_checksum(path: Path, expected_sha256: str) -> bool:
    """Verify SHA-256 checksum of a file."""
    sha256 = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256.update(chunk)
    actual = sha256.hexdigest()
    return actual == expected_sha256


def load_jsonl(path: Path) -> list[dict]:
    """Load a JSONL file into a list of dicts."""
    records: list[dict] = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def load_json(path: Path) -> list[dict] | dict:
    """Load a JSON file."""
    with open(path, encoding="utf-8") as f:
        return json.loads(f.read())


async def _fetch_with_retry(
    client: httpx.AsyncClient,
    url: str,
    params: dict[str, str],
    log: structlog.stdlib.BoundLogger,
) -> httpx.Response | None:
    """Fetch a URL with 3 retries on 5xx errors and timeouts.

    Returns the response on success, or None if all retries failed.
    """
    import asyncio

    import httpx

    for attempt in range(3):
        try:
            resp = await client.get(url, params=params)
        except httpx.TimeoutException:
            wait = 2**attempt
            log.warning("HF API timeout, retrying", attempt=attempt + 1, wait_s=wait)
            await asyncio.sleep(wait)
            continue
        if resp.status_code < 500:
            return resp
        wait = 2**attempt
        log.warning("HF API server error, retrying", status=resp.status_code, attempt=attempt + 1, wait_s=wait)
        await asyncio.sleep(wait)
    return None


async def download_hf_dataset(
    dataset: str,
    split: str,
    provider_name: str,
    filename: str,
    base_dir: str = "",
    config: str = "default",
) -> Path:
    """Download a HuggingFace dataset via the rows API and cache as JSONL.

    The HuggingFace parquet API only returns parquet file URLs, not actual
    data.  This function uses the datasets-server rows API which returns
    JSON rows directly, handles pagination (100 rows per request), and
    caches the result locally as JSONL.

    Args:
        dataset: HuggingFace dataset identifier (e.g. "princeton-nlp/SWE-bench_Lite").
        split: Dataset split (e.g. "test").
        provider_name: Provider name (used as cache subdirectory).
        filename: Local filename to save as.
        base_dir: Override base cache directory.
        config: Dataset config name (default "default").

    Returns:
        Path to the cached JSONL file.

    Raises:
        RuntimeError: If download fails.
    """
    cached = get_cached_path(provider_name, filename, base_dir)
    if cached is not None:
        logger.debug("using cached dataset", path=str(cached))
        return cached

    cache_dir = get_cache_dir(provider_name, base_dir)
    target = cache_dir / filename
    tmp_path = target.with_suffix(".tmp")

    log = logger.bind(dataset=dataset, split=split, target=str(target))
    log.info("downloading HuggingFace dataset via rows API")

    base_url = "https://datasets-server.huggingface.co/rows"
    page_sizes = [100, 10, 1]
    offset = 0
    all_rows: list[dict] = []

    try:
        import httpx

        headers: dict[str, str] = {}
        hf_token = os.getenv("HF_TOKEN", "")
        if hf_token:
            headers["Authorization"] = f"Bearer {hf_token}"

        async with httpx.AsyncClient(timeout=180.0, follow_redirects=True, headers=headers) as client:
            # Start with largest page size, fall back to smaller on persistent 5xx/timeout.
            page_size_idx = 0
            page_size = page_sizes[page_size_idx]

            while True:
                params = {
                    "dataset": dataset,
                    "config": config,
                    "split": split,
                    "offset": str(offset),
                    "length": str(page_size),
                }
                resp = await _fetch_with_retry(client, base_url, params, log)

                if resp is None and page_size_idx + 1 < len(page_sizes):
                    page_size_idx += 1
                    page_size = page_sizes[page_size_idx]
                    log.warning("reducing page size after persistent errors", new_page_size=page_size)
                    continue

                if resp is None and page_size == 1:
                    log.warning("skipping broken row after persistent errors", offset=offset)
                    offset += 1
                    continue

                if resp is None:
                    msg = f"all retries exhausted for {dataset}/{split} at page_size={page_size}"
                    raise RuntimeError(msg)
                resp.raise_for_status()
                data = resp.json()
                rows = data.get("rows", [])
                if not rows:
                    break
                all_rows.extend(rw.get("row", rw) for rw in rows)
                if len(rows) < page_size:
                    break
                offset += page_size

        with open(tmp_path, "w", encoding="utf-8") as f:
            f.writelines(json.dumps(record) + "\n" for record in all_rows)

        tmp_path.rename(target)
        log.info("dataset downloaded", rows=len(all_rows), size_bytes=target.stat().st_size)
        return target

    except Exception as exc:
        tmp_path.unlink(missing_ok=True)
        msg = f"failed to download dataset {dataset}/{split}: {exc}"
        raise RuntimeError(msg) from exc


async def download_hf_dataset_parquet(
    dataset: str,
    split: str,
    provider_name: str,
    filename: str,
    base_dir: str = "",
    config: str = "default",
) -> Path:
    """Download a HuggingFace dataset via the ``datasets`` library and cache as JSONL.

    This is an alternative to ``download_hf_dataset()`` that uses the ``datasets``
    Python library for direct Parquet download instead of the HTTP rows API.
    More reliable for large datasets (e.g., LiveCodeBench) where the HTTP API
    returns 502/504 errors.

    Requires: ``pip install datasets`` (or ``poetry install -E hf``)

    Args:
        dataset: HuggingFace dataset identifier (e.g. "livecodebench/code_generation").
        split: Dataset split (e.g. "test").
        provider_name: Provider name (used as cache subdirectory).
        filename: Local filename to save as.
        base_dir: Override base cache directory.
        config: Dataset config name (default "default").

    Returns:
        Path to the cached JSONL file.

    Raises:
        RuntimeError: If download fails or ``datasets`` library is not installed.
    """
    cached = get_cached_path(provider_name, filename, base_dir)
    if cached is not None:
        logger.debug("using cached dataset", path=str(cached))
        return cached

    cache_dir = get_cache_dir(provider_name, base_dir)
    target = cache_dir / filename
    tmp_path = target.with_suffix(".tmp")

    log = logger.bind(dataset=dataset, split=split, target=str(target))
    log.info("downloading HuggingFace dataset via datasets library")

    try:
        from datasets import load_dataset as hf_load_dataset
    except ImportError:
        msg = "The 'datasets' library is required for Parquet download. Install it with: poetry install -E hf"
        raise RuntimeError(msg) from None

    try:
        hf_token = os.getenv("HF_TOKEN", "") or None
        # Use trust_remote_code=False for security
        ds = hf_load_dataset(
            dataset,
            config if config != "default" else None,
            split=split,
            token=hf_token,
            trust_remote_code=False,
        )

        with open(tmp_path, "w", encoding="utf-8") as f:
            f.writelines(json.dumps(row, default=str) + "\n" for row in ds)

        tmp_path.rename(target)
        log.info("dataset downloaded via datasets library", rows=len(ds), size_bytes=target.stat().st_size)
        return target

    except Exception as exc:
        tmp_path.unlink(missing_ok=True)
        msg = f"failed to download dataset {dataset}/{split} via datasets library: {exc}"
        raise RuntimeError(msg) from exc
