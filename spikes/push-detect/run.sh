#!/bin/sh
# Spike: which hooks fire, with what args/stdin, for each git scenario.
# Fully sandboxed: temp HOME + GIT_CONFIG_GLOBAL, never touches the real ~/.gitconfig.
set -u

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

export HOME="$WORK/home"
export GIT_CONFIG_GLOBAL="$WORK/gitconfig"
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=spike GIT_AUTHOR_EMAIL=spike@example.com
export GIT_COMMITTER_NAME=spike GIT_COMMITTER_EMAIL=spike@example.com
mkdir -p "$HOME"

HOOKS="$WORK/hooks"
LOG="$WORK/hooks.log"
mkdir -p "$HOOKS"
: >"$LOG"

for h in pre-push post-commit reference-transaction post-rewrite; do
	cat >"$HOOKS/$h" <<EOF
#!/bin/sh
# reference-transaction: only log the "committed" phase.
[ "$h" = reference-transaction ] && [ "\$1" != committed ] && { cat >/dev/null; exit 0; }
gitdir=\$(git rev-parse --git-dir)
inprog=""
[ -d "\$gitdir/rebase-merge" ] && inprog="\$inprog rebase-merge"
[ -d "\$gitdir/rebase-apply" ] && inprog="\$inprog rebase-apply"
[ -f "\$gitdir/CHERRY_PICK_HEAD" ] && inprog="\$inprog CHERRY_PICK_HEAD"
[ -f "\$gitdir/MERGE_HEAD" ] && inprog="\$inprog MERGE_HEAD"
{
	printf '  %s args=[%s] ppid=%s reflog_action=[%s] in_progress=[%s]\n' "$h" "\$*" "\$PPID" "\${GIT_REFLOG_ACTION:-}" "\$inprog"
	while IFS= read -r line; do printf '    stdin: %s\n' "\$line"; done
} >>"$LOG"
exit 0
EOF
	chmod +x "$HOOKS/$h"
done

git config --global core.hooksPath "$HOOKS"
git config --global init.defaultBranch main
git config --global advice.detachedHead false

git init -q --bare "$WORK/remote.git"
git clone -q "$WORK/remote.git" "$WORK/clone" 2>/dev/null
cd "$WORK/clone" || exit 1

c() { echo "$1" >>file.txt && git add file.txt && git commit -q -m "$1"; }
# Setup commits run with hooks off so only the scenario under test is logged.
cq() { echo "$1" >>file.txt && git add file.txt && git -c core.hooksPath=/dev/null commit -q -m "$1"; }

scenario() {
	printf '\n=== %s ===\n' "$1" >>"$LOG"
	shift
	"$@" >/dev/null 2>&1
	printf '  (exit=%s)\n' "$?" >>"$LOG"
}

scenario "commit" c one
scenario "push -u origin main (success, first push)" git push -u origin main
scenario "commit --amend" git commit -q --amend -m "one amended"
scenario "push (rejected non-ff, amended history)" git push
scenario "push --force (success)" git push --force
scenario "push again (everything up-to-date)" git push
cq two
scenario "push --dry-run" git push --dry-run
scenario "push to raw URL (no remote-tracking ref)" git push "$WORK/remote.git" main
cq three
scenario "push to unreachable remote" git push /nonexistent/repo.git main

# Make remote ahead so fetch/pull have something to do.
# (hooks disabled for this setup step so it doesn't pollute the log)
git -c core.hooksPath=/dev/null clone -q "$WORK/remote.git" "$WORK/other" 2>/dev/null
(cd "$WORK/other" && echo x >other.txt && git add other.txt && git -c core.hooksPath=/dev/null commit -q -m other && git -c core.hooksPath=/dev/null push -q) >/dev/null 2>&1
scenario "fetch" git fetch
scenario "pull --rebase (local commit 'three' replayed)" git pull --rebase

cq four; cq five; cq six
scenario "rebase -i style: rebase last 3 onto HEAD~3 with --force-rebase" git rebase --force-rebase HEAD~3
git switch -q -c side HEAD~1
echo s >side.txt && git add side.txt && git -c core.hooksPath=/dev/null commit -q -m side-commit
echo s2 >side2.txt && git add side2.txt && git -c core.hooksPath=/dev/null commit -q -m side-commit-2
git switch -q main
scenario "cherry-pick" git cherry-pick side~1
scenario "merge (non-ff)" git merge --no-ff -m merge side

cat "$LOG"
