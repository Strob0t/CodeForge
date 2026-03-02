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

import structlog

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
