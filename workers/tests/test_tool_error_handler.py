import pytest

from codeforge.tools._base import ToolResult
from codeforge.tools._error_handler import catch_os_error


@pytest.mark.asyncio
async def test_catch_os_error_success():
    @catch_os_error
    async def my_tool(self, arguments, workspace_path):
        return ToolResult(output="ok", error="", success=True)

    result = await my_tool(None, {}, "/tmp")
    assert result.success is True
    assert result.output == "ok"


@pytest.mark.asyncio
async def test_catch_os_error_handles_oserror():
    @catch_os_error
    async def my_tool(self, arguments, workspace_path):
        raise OSError("Permission denied")

    result = await my_tool(None, {}, "/tmp")
    assert result.success is False
    assert "Permission denied" in result.error
