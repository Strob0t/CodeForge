package policy

import (
	"reflect"
	"testing"
)

// KI-69 (d): the files a command redirects to (written) and from (read) are
// extracted so that path_deny lists apply to them; a target whose value is
// not known statically is flagged.
func TestParseShellCommand_RedirectionTargets(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		writes, reads []string
		unknownWrite  bool
		unknownRead   bool
	}{
		{"output", "echo x > .env", []string{".env"}, nil, false, false},
		{"append", "echo x >> log", []string{"log"}, nil, false, false},
		{"noclobber", "echo x >| out", []string{"out"}, nil, false, false},
		{"stdout and stderr", "go test &> log", []string{"log"}, nil, false, false},
		{"stdout and stderr append", "go test &>> log", []string{"log"}, nil, false, false},
		{"stderr", "echo x 2> err.txt", []string{"err.txt"}, nil, false, false},
		{"dup to file", "echo x >& both.txt", []string{"both.txt"}, nil, false, false},
		{"no space", "echo x>.env", []string{".env"}, nil, false, false},
		{"double-quoted", `echo x > ".env"`, []string{".env"}, nil, false, false},
		{"single-quoted with space", `echo x >'.e nv'`, []string{".e nv"}, nil, false, false},
		{"escaped", `echo x > \.env`, []string{".env"}, nil, false, false},
		{"input", "cat < .env", nil, []string{".env"}, false, false},
		{"read-write", "exec 3<> rw.txt", []string{"rw.txt"}, []string{"rw.txt"}, false, false},
		{"several segments", "a > x; b >> y | c < z", []string{"x", "y"}, []string{"z"}, false, false},
		{"fd duplication is no file", "ls 2>&1", nil, nil, false, false},
		{"stdout to stderr is no file", "ls >&2", nil, nil, false, false},
		{"closed fd is no file", "ls <&- 2>&-", nil, nil, false, false},
		{"here-string is no file", "cat <<< hi", nil, nil, false, false},
		{"here-document is no file", "cat <<EOF\nx\nEOF", nil, nil, false, false},
		{"variable target", `echo x > "$F"`, nil, nil, true, false},
		{"substituted target", "echo x > $(pwd)/a", nil, nil, true, false},
		{"glob target", "echo x > *.txt", nil, nil, true, false},
		{"variable input", `cat < $F`, nil, nil, false, true},
		{"network target", "echo x > /dev/tcp/h/80", nil, nil, true, false},
		{"tilde target", "echo x > ~/k", nil, nil, true, false},
		// A relative target is listed for every directory the shell may be in.
		{"after cd", "cd a && echo x > k", []string{"k", "a/k"}, nil, false, false},
		{"after absolute cd", "cd /abs && echo x > k", []string{"k", "/abs/k"}, nil, false, false},
		{"after two cds", "cd a; cd b; echo x > k", []string{"k", "a/k", "b/k", "a/b/k"}, nil, false, false},
		{"after pushd with options", "pushd -n a; echo x > k", []string{"k", "a/k"}, nil, false, false},
		{"after assignment prefix", "CI=1 cd a; echo x > k", []string{"k", "a/k"}, nil, false, false},
		{"after command cd", "command cd a; echo x > k", []string{"k", "a/k"}, nil, false, false},
		{"absolute after cd", "cd a && echo x > /abs/k", []string{"/abs/k"}, nil, false, false},
		{"cd after the target", "echo x > k; cd a", []string{"k"}, nil, false, false},
		{"popd returns to an earlier directory", "popd; cd -; pushd +1; echo x > k", []string{"k"}, nil, false, false},
		{"cd home", "cd && echo x > k", nil, nil, true, false},
		{"cd tilde", "cd ~/x && echo x > k", nil, nil, true, false},
		{"computed cd", "cd $D && cat < k", nil, nil, false, true},
		{"eval", "eval x; echo x > k", nil, nil, true, false},
		{"computed command", "$C a; echo x > k", nil, nil, true, false},
		{"too many cds", "cd a; cd b; cd c; cd d; cd e; echo x > k", nil, nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := parseShellCommand(tt.cmd)
			if !reflect.DeepEqual(c.writes, tt.writes) || !reflect.DeepEqual(c.reads, tt.reads) {
				t.Errorf("writes=%q reads=%q, want writes=%q reads=%q", c.writes, c.reads, tt.writes, tt.reads)
			}
			if c.unknownWrite != tt.unknownWrite || c.unknownRead != tt.unknownRead {
				t.Errorf("unknownWrite=%v unknownRead=%v, want %v %v", c.unknownWrite, c.unknownRead, tt.unknownWrite, tt.unknownRead)
			}
		})
	}
}

// Redirection targets are checked against the path_deny lists of the file
// tools that would do the same: writes against Write and Edit rules, reads
// against Read rules. Arguments are not paths (a Bash command can open any
// file); only redirections, which bash itself opens, are checked.
func TestEvaluate_BashRedirectionsAgainstPathDeny(t *testing.T) {
	trusted := PresetTrustedMountAutonomous() // Bash allowed, Write/Edit deny secret paths
	readGuard := PolicyProfile{
		Name: "read-guard",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolRead}, Decision: DecisionAllow, PathDeny: []string{".env"}},
			{Specifier: ToolSpecifier{Tool: ToolBash}, Decision: DecisionAllow},
		},
	}
	tests := []struct {
		name    string
		profile *PolicyProfile
		tool    string
		command string
		want    Decision
	}{
		{"write to .env", &trusted, "bash", "echo x > .env", DecisionDeny},
		{"append to secrets", &trusted, "Bash", "echo x >> secrets/key", DecisionDeny},
		{"climbing path", &trusted, "bash", "echo x > ./a/../.env", DecisionDeny},
		{"absolute path inside the workspace", &trusted, "bash", "echo x > " + testWorkspace + "/.env", DecisionDeny},
		{"quoted target", &trusted, "bash", `echo x > ".env"`, DecisionDeny},
		{"stdout and stderr", &trusted, "bash", "go test &> config/credentials.json", DecisionDeny},
		{"dup to file", &trusted, "bash", "go test >& .env", DecisionDeny},
		{"second segment", &trusted, "bash", "go test ./... ; echo x > .env", DecisionDeny},
		{"Claude Code Monitor", &trusted, "Monitor", "tail -f log > .env", DecisionDeny},
		{"unknown target fails closed", &trusted, "bash", `echo x > "$F"`, DecisionDeny},
		{"ordinary file", &trusted, "bash", "echo x > out.txt", DecisionAllow},
		{"fd duplication", &trusted, "bash", "go test 2>&1 | tee test.log", DecisionAllow},
		{"outside the workspace", &trusted, "bash", "go test > /tmp/out", DecisionAllow},
		{"dev null", &trusted, "bash", "go test 2>/dev/null", DecisionAllow},
		{"read without read path_deny", &trusted, "bash", "cat < .env", DecisionAllow},
		{"read against read path_deny", &readGuard, "bash", "cat < .env", DecisionDeny},
		{"unknown read target fails closed", &readGuard, "bash", "cat < $F", DecisionDeny},
		{"write without write path_deny", &readGuard, "bash", "echo x > .env", DecisionAllow},
		{"argument is not a redirection", &readGuard, "bash", "cat .env", DecisionAllow},
		{"relative to a changed directory", &trusted, "bash", "cd secrets && echo x > key", DecisionDeny},
		{"absolute cd into the workspace", &trusted, "bash", "cd " + testWorkspace + "/secrets && echo x > key", DecisionDeny},
		{"ordinary file after cd", &trusted, "bash", "cd frontend && npm test > out.log 2>&1", DecisionAllow},
		{"unknown directory", &trusted, "bash", "cd $D && echo x > out.log", DecisionDeny},
		{"edit rule alone", &PolicyProfile{Name: "edit-guard", Mode: ModeAcceptEdits, Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "edit_file"}, Decision: DecisionAllow, PathDeny: []string{".env"}},
		}}, "bash", "echo x >> .env", DecisionDeny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.profile.Evaluate(ToolCall{Tool: tt.tool, Command: tt.command}, WithWorkspace(testWorkspace))
			if res.Decision != tt.want {
				t.Errorf("%q -> %s (%s), want %s", tt.command, res.Decision, res.Reason, tt.want)
			}
		})
	}
}

// Bash runs in the workspace, so a relative redirection target is resolved
// against it before it is compared: a target that climbs out and back in
// (../p1/.env with the workspace /srv/ws/p1) is a workspace file. Only a
// target that really resolves outside the workspace is skipped; without an
// absolute workspace no target can be placed and each one is unknown.
func TestEvaluate_RedirectionTargetsResolvedAgainstTheWorkspace(t *testing.T) {
	trusted := PresetTrustedMountAutonomous()
	tests := []struct {
		name      string
		workspace string
		command   string
		want      Decision
	}{
		{"climbing back into the workspace", testWorkspace, "go test > ../p1/.env", DecisionDeny},
		{"climbing back into a protected directory", testWorkspace, "echo x > ../p1/secrets/key", DecisionDeny},
		{"from the parent directory", testWorkspace, "cd .. && echo x > p1/.env", DecisionDeny},
		{"parent and back", testWorkspace, "cd .. && cd p1 && echo x > .env", DecisionDeny},
		{"absolute target", testWorkspace, "echo x > " + testWorkspace + "/.env", DecisionDeny},
		{"really outside", testWorkspace, "echo x > ../p2/.env", DecisionAllow},
		{"parent directory file", testWorkspace, "cd .. && echo x > out.log", DecisionAllow},
		{"no workspace", "", "echo x > out.log", DecisionDeny},
		{"no workspace, absolute target", "", "echo x > /srv/ws/p1/.env", DecisionDeny},
		{"relative workspace", "ws/p1", "echo x > out.log", DecisionDeny},
		{"no workspace, no redirection", "", "go test ./...", DecisionAllow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := trusted.Evaluate(ToolCall{Tool: "bash", Command: tt.command}, WithWorkspace(tt.workspace))
			if res.Decision != tt.want {
				t.Errorf("%q in %q -> %s (%s), want %s", tt.command, tt.workspace, res.Decision, res.Reason, tt.want)
			}
		})
	}
}

// A workspace path can go through a symlink (/srv/ws/p1 -> /data/real). The
// kernel resolves a redirection target from the physical directory, so
// ../real/.env from the workspace is its .env, and an absolute target can
// name the real path. Targets are compared against both forms of the
// workspace (WithWorkspaceRealPath), so neither form slips past path_deny.
func TestEvaluate_RedirectionTargetsInASymlinkedWorkspace(t *testing.T) {
	trusted := PresetTrustedMountAutonomous()
	const realWS = "/data/real"
	tests := []struct {
		name     string
		realPath string
		command  string
		want     Decision
	}{
		{"real path, absolute target", realWS, "echo x > /data/real/.env", DecisionDeny},
		{"real path, physical parent", realWS, "echo x > ../real/.env", DecisionDeny},
		{"real path, protected directory", realWS, "echo x > /data/real/secrets/key", DecisionDeny},
		{"real path, logical target still checked", realWS, "echo x > " + testWorkspace + "/.env", DecisionDeny},
		{"real path, outside both forms", realWS, "echo x > /data/other/.env", DecisionAllow},
		{"real path, workspace file", realWS, "echo x > out.log", DecisionAllow},
		{"real path same as workspace", testWorkspace, "echo x > ../p2/.env", DecisionAllow},
		{"relative real path is ignored", "data/real", "echo x > ../p2/.env", DecisionAllow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := trusted.Evaluate(ToolCall{Tool: "bash", Command: tt.command},
				WithWorkspace(testWorkspace), WithWorkspaceRealPath(tt.realPath))
			if res.Decision != tt.want {
				t.Errorf("%q in %q (real %q) -> %s (%s), want %s", tt.command, testWorkspace, tt.realPath, res.Decision, res.Reason, tt.want)
			}
		})
	}
}
