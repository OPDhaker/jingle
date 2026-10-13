#!/bin/sh
# Runs inside the Dockerfile's image: for each Linux player jingle knows,
# make it the only one on PATH, then check that jingle detects it and that
# `jingle play --wait` exits 0 at full and reduced volume (so the volume
# flags parse). Outputs are null sinks, so this proves the command lines,
# not that anything is heard.
#
#   docker build -t jingle-smoke scripts/linux-player-smoke
#   docker run --rm -v "$PWD/scripts/linux-player-smoke:/smoke:ro" \
#     -v "$PWD/dist/jingle_linux_arm64_v8.0/jingle:/jingle:ro" jingle-smoke
set -eu

# PipeWire's session manager needs a D-Bus session.
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
	exec dbus-run-session -- sh "$0" "$@"
fi

jingle=${JINGLE_BIN:-/jingle}
export XDG_RUNTIME_DIR=/tmp/xdg-$$
mkdir -m 700 "$XDG_RUNTIME_DIR"
export JINGLE_HOME="$HOME/jh"
sox -n -r 44100 -c 2 "$HOME/tag.wav" synth 0.3 sine 440
"$jingle" config set commit.sounds "$HOME/tag.wav" >/dev/null

fails=0
check() { # check <player>: run with only that player on PATH
	bin="$HOME/only-$1"
	mkdir -p "$bin"
	ln -sf "$(command -v "$1")" "$bin/$1"
	name=$(PATH="$bin" "$jingle" status --json | sed -n 's/^    "name": "\(.*\)",$/\1/p')
	if [ "$name" != "$1" ]; then
		echo "FAIL $1: status detected '$name'"
		fails=$((fails + 1))
		return
	fi
	for vol in 100 40; do
		"$jingle" config set volume "$vol" >/dev/null
		if out=$(timeout 20 env PATH="$bin" "$jingle" play commit --wait --json 2>&1); then
			echo "ok   $1 volume=$vol"
		else
			echo "FAIL $1 volume=$vol: $out"
			fails=$((fails + 1))
		fi
	done
}

pulseaudio -D --exit-idle-time=-1 2>/dev/null
sleep 1
check paplay
check ffplay
pulseaudio -k
sleep 1

printf 'pcm.!default { type null }\n' >"$HOME/.asoundrc"
check aplay

pipewire >/dev/null 2>&1 &
sleep 1
wireplumber >/dev/null 2>&1 &
sleep 2
pw-cli create-node adapter "{ factory.name=support.null-audio-sink node.name=jingle-null media.class=Audio/Sink object.linger=true audio.position=[FL FR] }" >/dev/null
sleep 1
check pw-play
kill %1 %2 2>/dev/null || true

if [ "$fails" -gt 0 ]; then
	echo "$fails check(s) failed"
	exit 1
fi
echo PASS
