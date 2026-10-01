package git

import "strings"

// keyClass tells how OpenRepo treats a repository config key.
type keyClass int

const (
	keyAllowed     keyClass = iota // data only, or overridden on the command line
	keyNetworkOnly                 // inert locally, refused for network operations
	keyRefused                     // could run code or leave the workspace: repository refused
)

// The allowlist is fail closed: a key is allowed only if it cannot make the
// git commands the Go Core runs (status, add, write-tree, commit-tree,
// read-tree, diff, merge-tree, reset, update-ref, log, checkout, branch,
// fetch/pull/push) run a program, read another repository or leave the
// workspace. Keys that only take data - including read-only file paths
// such as core.excludesFile or commit.template, which git reads as data -
// are allowed. Keys that name a program, a command, a tool, a template
// directory or another repository are refused (refusedKeys, and anything
// not listed). Linked worktrees and submodule working directories (a .git
// file) are refused by OpenRepo, not by config.

// allowedSections may hold any variable, with or without subsection, except
// refusedKeys: identity, display and formatting, aliases (git never runs an
// alias for a builtin), branch tracking data, LFS settings (its filters are
// neutralised), credential settings (helpers are dropped on the command
// line), gc/maintenance (auto runs are disabled on the command line) and
// settings of commands the Go Core does not run (rerere, format, apply,
// blame, notes, rebase, ...).
var allowedSections = map[string]bool{
	"user": true, "author": true, "committer": true,
	"color": true, "advice": true, "column": true, "i18n": true, "gui": true, "alias": true, "help": true,
	"branch": true, "lfs": true, "credential": true, "gc": true, "maintenance": true,
	"commit": true, "tag": true, "log": true, "pretty": true, "format": true, "versionsort": true,
	"rerere": true, "apply": true, "blame": true, "notes": true, "rebase": true, "status": true,
	"pull": true, "fetch": true, "push": true, "checkout": true, "clone": true, "add": true,
	"index": true, "pack": true, "repack": true, "sparse": true, "feature": true, "grep": true,
	"transfer": true, "receive": true, "safe": true, "worktree": true, "splitindex": true,
}

// refusedKeys name programs, tools or template directories, or are
// overridden only for some commands: refused even in an allowed section.
var refusedKeys = map[string]bool{
	"pull.twohead": true, "pull.octopus": true, // merge strategy programs (git-merge-<name>)
	"diff.external": true, "diff.tool": true, "diff.guitool": true,
	"merge.tool": true, "merge.guitool": true,
	"core.askpass": true, "core.worktree": true,
	"init.templatedir": true,
}

// refusedAnywhereKeys are refused with or without subsection, for every
// command (S3-F security review S1): they name a program git runs for a
// transport (ssh command, proxy command, remote-side programs, a remote
// helper), or they make a remote a promisor - then a local command that
// needs a missing object (diff, merge-tree, cat-file) fetches it lazily,
// with the repository's transport programs, inside the Go Core. Operators
// configure ssh with GIT_SSH_COMMAND in the Go Core's environment.
var refusedAnywhereKeys = map[string]bool{
	"core.sshcommand": true, "core.gitproxy": true,
	"remote.uploadpack": true, "remote.receivepack": true, "remote.vcs": true,
	"remote.promisor": true, "remote.partialclonefilter": true, "extensions.partialclone": true,
}

// allowedKeys are the section.variable keys (no subsection) a workspace
// repository may set outside the allowed sections. The fsmonitor, hooks
// path, untracked cache, split index, attributes file and signing keys are
// overridden on the command line.
var allowedKeys = map[string]bool{
	"core.repositoryformatversion": true, "core.filemode": true, "core.bare": true, "core.logallrefupdates": true,
	"core.ignorecase": true, "core.precomposeunicode": true, "core.symlinks": true, "core.autocrlf": true,
	"core.eol": true, "core.safecrlf": true, "core.sparsecheckout": true, "core.sparsecheckoutcone": true,
	"core.compression": true, "core.loosecompression": true, "core.bigfilethreshold": true, "core.quotepath": true,
	"core.checkstat": true, "core.trustctime": true, "core.abbrev": true, "core.commentchar": true, "core.commentstring": true,
	"core.whitespace": true, "core.fsmonitor": true, "core.fsmonitorhookversion": true, "core.hookspath": true,
	"core.untrackedcache": true, "core.splitindex": true, "core.editor": true, "core.pager": true,
	"core.excludesfile": true, "core.attributesfile": true, "core.preloadindex": true, "core.fscache": true,
	"core.longpaths": true, "core.sharedrepository": true, "core.commitgraph": true, "core.multipackindex": true,
	"core.packedgitlimit": true, "core.packedgitwindowsize": true, "core.deltabasecachesize": true,
	"core.packedrefstimeout": true, "core.filesreftimeout": true, "core.warnambiguousrefs": true,
	"core.hidedotfiles": true, "core.protecthfs": true, "core.protectntfs": true, "core.checkroundtripencoding": true,
	"core.notesref": true, "core.createobject": true, "core.restrictinheritedhandles": true,
	"init.defaultbranch": true,
	"merge.ff":           true, "merge.conflictstyle": true, "merge.renames": true, "merge.renamelimit": true,
	"merge.directoryrenames": true, "merge.log": true, "merge.branchdesc": true, "merge.stat": true,
	"merge.autostash": true, "merge.defaulttoupstream": true, "merge.verifysignatures": true,
	"diff.renames": true, "diff.algorithm": true, "diff.mnemonicprefix": true, "diff.colormoved": true,
	"diff.colormovedws": true, "diff.context": true, "diff.interhunkcontext": true, "diff.indentheuristic": true,
	"diff.noprefix": true, "diff.srcprefix": true, "diff.dstprefix": true, "diff.relative": true,
	"diff.suppressblankempty": true, "diff.statgraphwidth": true, "diff.statnamewidth": true, "diff.dirstat": true,
	"diff.orderfile": true, "diff.ignoresubmodules": true, "diff.submodule": true, "diff.wserrorhighlight": true,
	"diff.autorefreshindex": true, "diff.renamelimit": true, "diff.trustexitcode": true,
	"gpg.format":              true,
	"extensions.objectformat": true,
}

// allowedSubsectionKeys are the section.<name>.variable keys allowed for any
// subsection name. Filter drivers are neutralised on the command line.
var allowedSubsectionKeys = map[string]bool{
	"remote.url": true, "remote.pushurl": true, "remote.fetch": true, "remote.push": true, "remote.tagopt": true,
	"remote.prune": true, "remote.prunetags": true, "remote.skipdefaultupdate": true, "remote.skipfetchall": true,
	"remote.mirror": true,
	"filter.clean":  true, "filter.smudge": true, "filter.process": true, "filter.required": true,
	"diff.xfuncname": true, "diff.funcname": true, "diff.binary": true, "diff.wordregex": true, "diff.cachetextconv": true,
	"merge.name": true, "merge.recursive": true,
	"submodule.url": true, "submodule.active": true, "submodule.branch": true, "submodule.ignore": true,
	"submodule.shallow": true, "submodule.fetchrecursesubmodules": true,
}

// networkOnly keys configure transports with data (URLs, rewrites, options),
// never a program: inert for local operations, refused for fetch, pull and
// push. Section-level entries cover every key of the section (with or
// without subsection).
var networkOnlySections = map[string]bool{"http": true, "protocol": true, "url": true}

var networkOnlyKeys = map[string]bool{
	"fetch.bundleuri": true,
	"remote.proxy":    true, "remote.proxyauthmethod": true, "remote.serveroption": true,
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
	name := section + "." + variable
	switch {
	case section == "core" && variable == "bare" && subsection == "" && isTrue(value):
		return keyRefused // no worktree to work in
	case subsection == "" && refusedKeys[name], refusedAnywhereKeys[name]:
		return keyRefused
	case networkOnlyKeys[name]:
		return keyNetworkOnly
	case section == "gpg" && subsection == "" && variable == "format":
		return keyAllowed
	case allowedSections[section]:
		return keyAllowed
	case networkOnlySections[section]:
		return keyNetworkOnly
	case subsection == "" && allowedKeys[name]:
		return keyAllowed
	case subsection != "" && allowedSubsectionKeys[name]:
		return keyAllowed
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
