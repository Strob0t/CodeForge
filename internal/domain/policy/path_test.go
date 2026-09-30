package policy

import "testing"

func TestNormalizePath(t *testing.T) {
	const ws = "/srv/workspaces/t1/p1"
	tests := []struct {
		name      string
		workspace string
		path      string
		want      string
		wantOK    bool
	}{
		{"empty stays empty", ws, "", "", true},
		{"plain relative", ws, "src/main.go", "src/main.go", true},
		{"dot prefix", ws, "./.env", ".env", true},
		{"inner dotdot", ws, "a/../.env", ".env", true},
		{"double slash", ws, "src//x.go", "src/x.go", true},
		{"trailing slash", ws, "secrets/", "secrets", true},
		{"workspace root dot", ws, ".", ".", true},
		{"absolute inside", ws, ws + "/secrets/a", "secrets/a", true},
		{"absolute workspace root", ws, ws, ".", true},
		{"absolute inside with dotdot", ws, ws + "/src/../.env", ".env", true},
		{"relative escape", ws, "../other/x", "", false},
		{"relative escape after clean", ws, "a/../../x", "", false},
		{"bare dotdot", ws, "..", "", false},
		{"absolute outside", ws, "/etc/passwd", "", false},
		{"absolute sibling prefix", ws, ws + "-evil/x", "", false},
		{"absolute escape via dotdot", ws, ws + "/../p2/.env", "", false},
		{"absolute without workspace", "", "/etc/passwd", "", false},
		{"relative without workspace", "", "src/x.go", "src/x.go", true},
		{"escape without workspace", "", "../x", "", false},
		{"relative workspace cannot check absolute", "data/ws", "/data/ws/x", "", false},
		{"dotdot-like file name", ws, "..foo", "..foo", true},
		{"tilde is literal", ws, "~/.ssh/id_rsa", "~/.ssh/id_rsa", true},
		{"whitespace is literal", ws, " .env", " .env", true},
		{"workspace with trailing slash", ws + "/", ws + "/x", "x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizePath(tt.workspace, tt.path)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("NormalizePath(%q, %q) = (%q, %v), want (%q, %v)", tt.workspace, tt.path, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
