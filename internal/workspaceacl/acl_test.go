package workspaceacl

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type vectorEntry struct {
	Tag  string  `json:"tag"`
	Perm uint16  `json:"perm"`
	ID   *uint32 `json:"id"`
}

type vector struct {
	Name         string        `json:"name"`
	ToolUID      int           `json:"tool_uid"`
	WorkspaceGID int           `json:"workspace_gid"`
	Entries      []vectorEntry `json:"entries"`
	Hex          string        `json:"hex"`
}

var tagNames = map[string]uint16{
	"user_obj": TagUserObj, "user": TagUser, "group_obj": TagGroupObj, "group": TagGroup, "mask": TagMask, "other": TagOther,
}

func loadVectors(t *testing.T) map[string]vector {
	t.Helper()
	data, err := os.ReadFile("testdata/acl_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []vector `json:"vectors"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	out := map[string]vector{}
	for _, v := range file.Vectors {
		out[v.Name] = v
	}
	return out
}

func (v vector) entries(t *testing.T) []Entry {
	t.Helper()
	out := make([]Entry, 0, len(v.Entries))
	for _, e := range v.Entries {
		tag, ok := tagNames[e.Tag]
		if !ok {
			t.Fatalf("%s: unknown tag %q", v.Name, e.Tag)
		}
		id := UndefinedID
		if e.ID != nil {
			id = *e.ID
		}
		out = append(out, Entry{Tag: tag, Perm: e.Perm, ID: id})
	}
	return out
}

// TestCodecMatchesTheSharedVectors: the Go Core and the worker
// (posix_acl.py) encode the same ACLs to the same bytes.
func TestCodecMatchesTheSharedVectors(t *testing.T) {
	vectors := loadVectors(t)
	if len(vectors) < 4 {
		t.Fatalf("only %d vectors", len(vectors))
	}
	for name, v := range vectors {
		entries := v.entries(t)
		if got := hex.EncodeToString(Encode(entries)); got != v.Hex {
			t.Errorf("%s: Encode = %s, want %s", name, got, v.Hex)
		}
		raw, err := hex.DecodeString(v.Hex)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := Decode(raw)
		if err != nil || !Equal(decoded, entries) {
			t.Errorf("%s: Decode = %v, %v; want %v", name, decoded, err, entries)
		}
	}
	if !Equal(TenantAccess(20000), vectors["tenant_access"].entries(t)) {
		t.Error("TenantAccess differs from the tenant_access vector")
	}
	if !Equal(TenantDefault(20000, 10010), vectors["tenant_default"].entries(t)) {
		t.Error("TenantDefault differs from the tenant_default vector")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, data := range [][]byte{nil, {1, 2, 3}, {2, 0, 0, 0, 1}, {3, 0, 0, 0}} {
		if _, err := Decode(data); err == nil {
			t.Errorf("Decode(%v) accepted garbage", data)
		}
	}
	if entries, err := Decode([]byte{2, 0, 0, 0}); err != nil || len(entries) != 0 {
		t.Errorf("empty ACL: %v, %v", entries, err)
	}
}
