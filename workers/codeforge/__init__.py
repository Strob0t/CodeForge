"""CodeForge AI Workers — LLM integration and agent execution."""

from pathlib import Path


def _read_version() -> str:
    """Read version from the project-root VERSION file."""
    for candidate in [
        Path(__file__).resolve().parent.parent.parent / "VERSION",
        Path("VERSION"),
    ]:
        if candidate.is_file():
            return candidate.read_text().strip()
    return "dev"


__version__ = _read_version()
