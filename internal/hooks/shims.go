// Package hooks installs and uninstalls jingle's global git hooks.
//
// It owns the POSIX sh shims in the hooks dir, setting git's global
// core.hooksPath, saving the previous value to state.json, and restoring it
// exactly on uninstall. Shims always chain to the repo's own hooks (or to the
// previous global hooks dir), passing stdin, args, and exit codes through.
// The push sound plays only once a push succeeds: pre-push writes a marker
// keyed by the git push process, and reference-transaction plays when that
// same process updates a remote-tracking ref (design: spikes/README.md).
package hooks

import (
	"bytes"
	"strings"
	"text/template"
)

// Marker is on the second line of every shim. Only files carrying it are
// ever overwritten or removed.
const Marker = "# jingle-managed"

// HookNames are the hooks git runs from core.hooksPath. Once jingle owns that
// path, git stops looking in each repo's .git/hooks, so every one of these
// gets a pass-through shim or the user's own hooks would silently stop.
//
// Left out on purpose: push-to-checkout and proc-receive (their mere presence
// changes git's behavior), fsmonitor-watchman and p4-* (not run from the
// hooks dir in normal use).
var HookNames = []string{
	"applypatch-msg", "pre-applypatch", "post-applypatch",
	"pre-commit", "pre-merge-commit", "prepare-commit-msg", "commit-msg", "post-commit",
	"pre-rebase", "post-checkout", "post-merge", "pre-push",
	"pre-receive", "update", "post-receive", "post-update",
	"reference-transaction", "pre-auto-gc", "post-rewrite",
	"sendemail-validate", "post-index-change",
}

// ShimConfig is baked into every shim at install time.
type ShimConfig struct {
	Jingle string // jingle binary to call; falls back to `command -v jingle`
	Chain  string // hooks dir to chain to; "" = the repo's own hooks dir
	Self   string // jingle's hooks dir (never chained to)
	Run    string // dir for push markers (see config.Paths.RunDir)
}

var shimTmpl = template.Must(template.New("shim").Funcs(template.FuncMap{"q": shQuote}).Parse(`#!/bin/sh
` + Marker + `: written by "jingle install", removed by "jingle uninstall". Do not edit.
# Runs the {{.Hook}} hook git would have run without jingle, passing args,
# stdin, and exit status through.
{{- with .Does}}
# Also {{.}}.
{{- end}}
chain={{q .Chain}}
self={{q .Self}}
{{- if .Event}}
run={{q .Run}}
{{- end}}
# Find the repo's hooks dir without forking git when possible: git runs hooks
# from the worktree root (or from $GIT_DIR in a bare repo), and about a dozen
# hooks fire per commit. Linked worktrees keep hooks in the common dir.
if [ -n "$chain" ]; then
	dir=$chain
elif [ -z "${GIT_DIR:-}" ] && [ -d .git ] && [ ! -f .git/commondir ]; then
	dir=.git/hooks
elif [ -n "${GIT_DIR:-}" ] && [ -d "$GIT_DIR" ] && [ ! -f "$GIT_DIR/commondir" ]; then
	dir=$GIT_DIR/hooks
else
	dir=$(git rev-parse --git-common-dir 2>/dev/null) && dir=$dir/hooks || dir=
fi
target=
if [ -n "$dir" ] && [ "$dir" != "$self" ] && [ -f "$dir/{{.Hook}}" ] && [ -x "$dir/{{.Hook}}" ]; then
	target=$dir/{{.Hook}}
fi
{{- if not .Event}}
[ -n "$target" ] && exec "$target" "$@"
exit 0
{{- else}}
{{- if eq .Hook "reference-transaction"}}

# Fires 3 times per ref update in every git command: stay a plain
# pass-through unless this is the committed phase of a push that pre-push
# marked (the marker is keyed by the git push process, our parent).
if [ "$1" != committed ] || [ ! -f "$run/push-$PPID" ]; then
	[ -n "$target" ] && exec "$target" "$@"
	exit 0
fi
{{- end}}

play() {
	j={{q .Jingle}}
	[ -x "$j" ] || j=$(command -v jingle 2>/dev/null) || return 0
	"$j" hook-event "$@" >/dev/null 2>&1 </dev/null &
}
{{- if eq .Hook "post-commit"}}

status=0
if [ -n "$target" ]; then
	"$target" "$@"
	status=$?
fi
# Rebase, cherry-pick, and revert fire post-commit once per replayed commit:
# stay quiet while one is in progress. Checked here, not in jingle, because
# git removes the state as soon as the last commit is replayed.
gd=${GIT_DIR:-}
if [ -z "$gd" ]; then
	if [ -d .git ]; then
		gd=.git
	else
		gd=$(git rev-parse --git-dir 2>/dev/null) || gd=
	fi
fi
if [ -n "$gd" ]; then
	for f in rebase-merge rebase-apply CHERRY_PICK_HEAD REVERT_HEAD sequencer; do
		[ -e "$gd/$f" ] && exit "$status"
	done
fi
play commit
exit "$status"
{{- else}}

input=$(cat)
status=0
if [ -n "$target" ]; then
	if [ -n "$input" ]; then
		printf '%s\n' "$input" | "$target" "$@"
	else
		"$target" "$@" </dev/null
	fi
	status=$?
fi
{{- if eq .Hook "pre-push"}}
# Empty stdin means nothing to send (up to date, or rejected): no sound.
# Otherwise mark this push now, before git sends anything, so the
# reference-transaction hook can tell it apart from a fetch.
if [ "$status" -eq 0 ] && [ -n "$input" ]; then
	mkdir -p "$run" 2>/dev/null && : >"$run/push-$PPID" 2>/dev/null
	play pre-push "$PPID" "$1" "$2"
fi
{{- else}}
# A remote-tracking ref moved in the marked push process: the push went through.
case $input in
*" refs/remotes/"*) play push-done "$PPID" ;;
esac
{{- end}}
exit "$status"
{{- end}}
{{- end}}
`))

// events maps the hooks that take part in playing a sound to their event.
var events = map[string]string{
	"post-commit":           "commit",
	"pre-push":              "push",
	"reference-transaction": "push",
}

// does describes, for the shim header, what a hook adds beyond chaining.
var does = map[string]string{
	"post-commit":           "plays the commit sound in the background, except mid-rebase, cherry-pick, or revert",
	"pre-push":              "marks the push for reference-transaction",
	"reference-transaction": "plays the push sound once a marked push succeeds",
}

// RenderShim returns the shim script for hook.
func RenderShim(hook string, c ShimConfig) []byte {
	var b bytes.Buffer
	data := struct {
		ShimConfig
		Hook, Event, Does string
	}{c, hook, events[hook], does[hook]}
	if err := shimTmpl.Execute(&b, data); err != nil {
		panic(err) // the template is static; any error is a bug
	}
	return b.Bytes()
}

// Shims returns every shim, keyed by hook name.
func Shims(c ShimConfig) map[string][]byte {
	m := make(map[string][]byte, len(HookNames))
	for _, h := range HookNames {
		m[h] = RenderShim(h, c)
	}
	return m
}

// shQuote single-quotes s for POSIX sh.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// IsManaged reports whether data is a jingle shim.
func IsManaged(data []byte) bool {
	_, rest, ok := bytes.Cut(data, []byte("\n"))
	return ok && bytes.HasPrefix(rest, []byte(Marker))
}
