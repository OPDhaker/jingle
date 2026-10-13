#!/bin/sh
# End-to-end check of the release, as a new user would see it: install.sh
# from a goreleaser dist/ → jingle install → commit and push play → uninstall
# --purge → gitconfig byte-identical and no jingle files left. Uses jingle's
# default paths under a temp HOME, never the real one.
#
#   goreleaser release --snapshot --clean && scripts/e2e-clean-machine.sh dist
set -eu

dist=$(cd "${1:-dist}" && pwd)
src=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d 2>/dev/null || mktemp -d -t jingle-e2e)
work=$(cd "$work" && pwd -P)
trap 'rm -rf "$work"' EXIT

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}
step() { printf '== %s\n' "$*"; }

# A clean machine: fresh HOME, no system config, no jingle overrides.
unset XDG_CONFIG_HOME XDG_DATA_HOME JINGLE_HOME JINGLE_INSTALL_DIR JINGLE_VERSION
export HOME="$work/home"
export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_NOSYSTEM=1
mkdir -p "$HOME"
printf '[user]\n\tname = e2e\n\temail = e2e@example.com\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"
cp "$GIT_CONFIG_GLOBAL" "$work/gitconfig.before"

# Audio is faked: the player logs what it was asked to play.
log="$work/played.log"
: >"$log"
# $1 below belongs to the generated script, not this one.
# shellcheck disable=SC2016
printf '#!/bin/sh\nprintf "%%s\\n" "$1" >> "%s"\n' "$log" >"$work/player"
chmod +x "$work/player"
export JINGLE_PLAYER="$work/player"
printf 'RIFF fake' >"$work/tag.wav"

# played <n>: wait up to 5 s for the log to have n lines.
played() {
	i=0
	while [ "$(wc -l <"$log" | tr -d ' ')" -lt "$1" ]; do
		i=$((i + 1))
		[ "$i" -le 50 ] || fail "expected $1 sound(s), log has: $(cat "$log")"
		sleep 0.1
	done
}

step "install.sh"
JINGLE_DOWNLOAD_URL="file://$dist" sh "$src/install.sh" >/dev/null
export PATH="$HOME/.local/bin:$PATH"
[ "$(command -v jingle)" = "$HOME/.local/bin/jingle" ] || fail "jingle not on PATH after install.sh"
jingle version

step "jingle install"
jingle install --yes >/dev/null
jingle config set commit.sounds "$work/tag.wav" >/dev/null
jingle config set push.sounds "$work/tag.wav" >/dev/null
[ -f "$HOME/.config/jingle/config.toml" ] || fail "config not at the default path"
[ -d "$HOME/.local/share/jingle/hooks" ] || fail "hooks not at the default path"

step "commit plays"
git init -q "$work/repo"
git -C "$work/repo" commit -q --allow-empty -m one
played 1

step "push plays"
git init -q --bare "$work/remote.git"
git -C "$work/repo" remote add origin "$work/remote.git"
git -C "$work/repo" push -q -u origin main
played 2

step "doctor"
jingle doctor --repo "$work/repo" >/dev/null || fail "doctor reported problems: $(jingle doctor --repo "$work/repo")"

step "uninstall --purge"
jingle uninstall --yes --purge >/dev/null
cmp "$GIT_CONFIG_GLOBAL" "$work/gitconfig.before" || fail "gitconfig differs after uninstall"
[ ! -e "$HOME/.config/jingle" ] || fail "config dir left behind"
[ ! -e "$HOME/.local/share/jingle" ] || fail "data dir left behind"

step "silent after uninstall"
git -C "$work/repo" commit -q --allow-empty -m two
sleep 1
[ "$(wc -l <"$log" | tr -d ' ')" -eq 2 ] || fail "a sound played after uninstall"

rm "$HOME/.local/bin/jingle"
echo "PASS"
