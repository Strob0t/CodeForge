name: python-package-patterns
type: knowledge
trigger: "__init__.py|setup.py|pyproject.toml"
description: Python package structure and entry point patterns
---
## Python Package Structure

When creating a Python package (directory with `__init__.py`):

1. **Always create `__main__.py`** so `python -m package_name` works:
   ```python
   from .cli import main

   if __name__ == "__main__":
       main()
   ```

2. **Standard package layout:**
   ```
   package_name/
     __init__.py        # Can be empty or contain __version__
     __main__.py        # Entry point for python -m
     cli.py             # CLI logic (argparse)
     models.py          # Data structures
   tests/
     __init__.py
     conftest.py        # Shared fixtures
     test_cli.py
     test_models.py
   ```

3. **Entry points in setup.py / pyproject.toml:**
   ```python
   entry_points={"console_scripts": ["cmd-name=package_name.cli:main"]}
   ```

4. **Common mistake:** Forgetting `__main__.py` means `python -m package_name` fails with `No module named package_name.__main__`.
