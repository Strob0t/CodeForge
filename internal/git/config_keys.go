package git

import "strings"

// keyClass tells how OpenRepo treats a repository config key.
type keyClass int

const (
	keyAllowed     keyClass = iota // data only, or overridden on the command line
	keyNetworkOnly                 // inert locally, refused for network operations
	keyRefused                     // could run code or leave the workspace: repository refused
)

// allowedSections may hold any variable, with or without subsection: user
// identity, display settings, aliases (git never runs an alias for a builtin),
// branch tracking data, LFS settings (its filters are neutralised), credential
// settings (helpers are dropped on the command line) and gc/maintenance (auto
// runs are disabled on the command line).
var allowedSections = map[string]bool{
	"user": true, "author": true, "committer": true,
	"color": true, "advice": true, "column": true, "i18n": true, "gui": true, "alias": true,
	"branch": true, "lfs": true, "credential": true, "gc": true, "maintenance": true,
}

// allowedKeys are the section.variable keys (no subsection) a workspace
// repository may set. The fsmonitor, hooks path, untracked cache, split index
// and signing keys are overridden on the command line.
var allowedKeys = map[string]bool{
	"core.repositoryformatversion": true, "core.filemode": true, "core.bare": true, "core.logallrefupdates": true,
	"core.ignorecase": true, "core.precomposeunicode": true, "core.symlinks": true, "core.autocrlf": true,
	"core.eol": true, "core.safecrlf": true, "core.sparsecheckout": true, "core.sparsecheckoutcone": true,
	"core.compression": true, "core.loosecompression": true, "core.bigfilethreshold": true, "core.quotepath": true,
	"core.checkstat": true, "core.trustctime": true, "core.abbrev": true, "core.commentchar": true,
	"core.whitespace": true, "core.fsmonitor": true, "core.hookspath": true, "core.untrackedcache": true,
	"core.splitindex": true, "core.editor": true, "core.pager": true,
	"init.defaultbranch": true,
	"pull.rebase":        true, "pull.ff": true,
	"push.default": true, "push.autosetupremote": true, "push.followtags": true, "push.gpgsign": true,
	"fetch.prune": true, "fetch.prunetags": true, "fetch.recursesubmodules": true,
	"merge.ff": true, "merge.conflictstyle": true,
	"rebase.autostash": true,
	"commit.gpgsign":   true, "tag.gpgsign": true,
	"status.showuntrackedfiles": true, "status.relativepaths": true, "status.short": true, "status.branch": true,
	"diff.renames": true, "diff.algorithm": true, "diff.mnemonicprefix": true,
	"extensions.objectformat": true,
	"index.version":           true,
}

// allowedSubsectionKeys are the section.<name>.variable keys allowed for any
// subsection name. Filter drivers are neutralised on the command line.
var allowedSubsectionKeys = map[string]bool{
	"remote.url": true, "remote.pushurl": true, "remote.fetch": true, "remote.tagopt": true,
	"remote.prune": true, "remote.prunetags": true, "remote.skipdefaultupdate": true, "remote.skipfetchall": true,
	"filter.clean": true, "filter.smudge": true, "filter.process": true, "filter.required": true,
	"diff.xfuncname": true, "diff.funcname": true, "diff.binary": true, "diff.wordregex": true,
	"submodule.url": true, "submodule.active": true, "submodule.branch": true,
}

// networkOnlyKeys configure transports: inert for local operations, refused
// for fetch, pull and push. Section-level entries cover every key of the
// section (with or without subsection).
var networkOnlySections = map[string]bool{"http": true, "protocol": true, "url": true}

var networkOnlyKeys = map[string]bool{
	"core.sshcommand": true, "core.gitproxy": true,
	"remote.receivepack": true, "remote.uploadpack": true, "remote.vcs": true,
	"remote.proxy": true, "remote.proxyauthmethod": true,
}

// splitKey splits a config key as git lists it (section and variable lower
// case) into section, subsection ("" if none) and variable.
func splitKey(key string) (section, subsection, variable string) {
	first := strings.IndexByte(key, '.')
	last := strings.LastIndexByte(key, '.')
	if first < 0 {
		return key, "", ""
	}
	if first == last {
		return key[:first], "", key[last+1:]
	}
	return key[:first], key[first+1 : last], key[last+1:]
}

// classifyKey classifies one repository config entry.
func classifyKey(key, value string) keyClass {
	section, subsection, variable := splitKey(key)
	switch {
	case section == "core" && variable == "bare" && subsection == "" && isTrue(value):
		return keyRefused // no worktree to work in
	case allowedSections[section]:
		return keyAllowed
	case networkOnlySections[section]:
		return keyNetworkOnly
	case subsection == "" && allowedKeys[section+"."+variable]:
		return keyAllowed
	case subsection == "" && networkOnlyKeys[section+"."+variable]:
		return keyNetworkOnly
	case subsection != "" && allowedSubsectionKeys[section+"."+variable]:
		return keyAllowed
	case subsection != "" && networkOnlyKeys[section+"."+variable]:
		return keyNetworkOnly
	default:
		return keyRefused // include, includeIf, core.worktree, drivers, programs, anything unknown
	}
}

// isTrue reports whether a config value is a git boolean true ("" is true:
// a key without "=").
func isTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", "yes", "on", "1":
		return true
	}
	return false
}
