# Worktree I: Python Error Handling — Atomic Plan

> **Branch:** `fix/python-error-handling`
> **Effort:** ~1h | **Findings:** 3 | **Risk:** Low (logging-only changes)

---

## Task I1: Fix Swallowed Exception in model_resolver (Q-019)

**File:** `workers/codeforge/model_resolver.py:69-70`

- [ ] Change from:
```python
    except Exception:
        return set()
```
To:
```python
    except Exception:
        logger.warning("failed to fetch healthy models from LiteLLM", exc_info=True)
        return set()
```
- [ ] Verify: `cd workers && python -m pytest tests/test_model_resolver*.py -v` (if test exists) or `python -c "from codeforge.model_resolver import _fetch_healthy_models"`

**Commit:** `fix: log swallowed exception in model_resolver health check (Q-019)`

---

## Task I2: Add Exception Variable to context_reranker (Q-004)

**File:** `workers/codeforge/context_reranker.py:80-81`

- [ ] Change from:
```python
    except Exception:
        logger.warning("context rerank LLM call failed, using original order")
```
To:
```python
    except Exception:
        logger.warning("context rerank LLM call failed, using original order", exc_info=True)
```
- [ ] Verify: `cd workers && python -c "from codeforge.context_reranker import ContextReranker"`

**Commit:** `fix: add exc_info to context_reranker exception handler (Q-004)`

---

## Task I3: Add Exception Details to history.py (Q-004)

**File:** `workers/codeforge/history.py`

- [ ] Line 208: Change from:
```python
        except Exception:
            logger.warning(
                "skipping image with invalid base64 data",
                extra={"image_id": getattr(img, "id", "unknown")},
            )
```
To:
```python
        except Exception:
            logger.warning(
                "skipping image with invalid base64 data",
                extra={"image_id": getattr(img, "id", "unknown")},
                exc_info=True,
            )
```

- [ ] Line 364: Change from:
```python
    except Exception:
        logger.warning("conversation summarization failed, keeping original history")
```
To:
```python
    except Exception:
        logger.warning("conversation summarization failed, keeping original history", exc_info=True)
```
- [ ] Verify: `cd workers && python -m pytest tests/test_history*.py -v`

**Commit:** `fix: add exc_info to history.py exception handlers (Q-004)`

---

## Verification

- [ ] `cd workers && python -m pytest tests/ -v --timeout=30`
- [ ] `cd workers && ruff check workers/codeforge/model_resolver.py workers/codeforge/context_reranker.py workers/codeforge/history.py`
