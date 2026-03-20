name: python-testing-patterns
type: knowledge
trigger: "test_*.py|pytest|argparse"
description: Testing patterns for Python CLI tools with argparse
---
## Testing Argparse CLIs with Pytest

**Never call an argparse `main()` directly in tests** without handling `sys.argv`. Argparse reads `sys.argv` and raises `SystemExit` on parse errors.

### Correct patterns:

**Option A -- subprocess (integration tests, recommended):**
```python
import subprocess

def test_add_task(tmp_path):
    result = subprocess.run(
        ["python", "-m", "package_name", "add", "--title", "Test"],
        capture_output=True, text=True, cwd=str(tmp_path)
    )
    assert result.returncode == 0
    assert "Added" in result.stdout
```

**Option B -- monkeypatch sys.argv (unit tests):**
```python
def test_add_task(monkeypatch, tmp_path):
    monkeypatch.setattr("sys.argv", ["prog", "add", "--title", "Test"])
    main()  # Now argparse sees the correct argv
```

**Option C -- parse_args directly (parser unit tests):**
```python
def test_parser():
    parser = create_parser()
    args = parser.parse_args(["add", "--title", "Test"])
    assert args.title == "Test"
```

### Common mistakes:
- Calling `main()` without setting `sys.argv` causes `SystemExit: 2`
- Not using `tmp_path` fixture for file I/O causes test pollution
- Forgetting `capture_output=True` in subprocess means no assertion data
- Not isolating JSON storage path causes tests to share state
