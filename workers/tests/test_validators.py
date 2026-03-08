from codeforge._validators import coerce_none_to_list


def test_coerce_none_returns_empty_list():
    assert coerce_none_to_list(None) == []


def test_coerce_list_returns_same():
    assert coerce_none_to_list(["a", "b"]) == ["a", "b"]


def test_coerce_empty_list_returns_empty():
    assert coerce_none_to_list([]) == []
