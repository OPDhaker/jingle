#!/bin/sh
# Spike: how much does a post-commit hook that plays audio slow down `git commit`?
# Usage: bench.sh <path-to-built-playback-binary> [audio-file]
# Sandboxed: temp HOME + GIT_CONFIG_GLOBAL. Timing runs use volume 0 (silent).
set -eu
BIN=$1
SOUND=${2:-/System/Library/Sounds/Glass.aiff}
N=${N:-10}

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
export HOME="$WORK/home" GIT_CONFIG_GLOBAL="$WORK/gitconfig" GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=b GIT_AUTHOR_EMAIL=b@b GIT_COMMITTER_NAME=b GIT_COMMITTER_EMAIL=b@b
mkdir -p "$HOME" "$WORK/none" "$WORK/detached" "$WORK/sync"

# Shim under test: what the real hook will do (chain omitted), output fully detached.
cat >"$WORK/detached/post-commit" <<HOOK
#!/bin/sh
"$BIN" -v 0 "$SOUND" >/dev/null 2>&1 </dev/null &
exit 0
HOOK
# Anti-pattern for comparison: play synchronously in the hook.
cat >"$WORK/sync/post-commit" <<HOOK
#!/bin/sh
afplay -v 0 "$SOUND"
HOOK
chmod +x "$WORK"/*/post-commit

now_ms() { perl -MTime::HiRes=time -e 'printf "%d\n", time*1000'; }

git init -q "$WORK/repo"
cd "$WORK/repo"
for mode in ${MODES:-none detached sync}; do
	git config --global core.hooksPath "$WORK/$mode"
	start=$(now_ms)
	i=0
	while [ $i -lt $N ]; do
		git commit -q --allow-empty -m "$mode $i"
		i=$((i + 1))
	done
	end=$(now_ms)
	printf '%-9s %4d ms/commit\n' "$mode" $(((end - start) / N))
done
