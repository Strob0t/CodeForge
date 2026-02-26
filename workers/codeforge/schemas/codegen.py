"""Schema for code generation step."""

from __future__ import annotations

from pydantic import BaseModel, Field


class CodeGenInput(BaseModel):
    """Input for the code generation step."""

    spec: str
    existing_code: str = ""
    constraints: list[str] = Field(default_factory=list)
    language: str = ""
    file_path: str = ""


class CodeGenOutput(BaseModel):
    """Output from the code generation step."""

    code: str
    tests: str = ""
    explanation: str
    files_modified: list[str] = Field(default_factory=list)
