#!/bin/sh
# Install the jingle binary from GitHub Releases (macOS and Linux):
#
#   curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | sh
#
# This only puts the binary on disk. It does not touch your git config; run
# `jingle install --yes` for that. No sudo needed.
#
# Options (environment variables):
#   JINGLE_VERSION       tag to install, e.g. v0.1.0 (default: latest release)
#   JINGLE_INSTALL_DIR   where to put the binary (default: ~/.local/bin)
#   JINGLE_DOWNLOAD_URL  base URL holding the archives and checksums.txt,
#                        instead of the GitHub release (file:// works)
#
# To remove jingle later:
#   jingle uninstall --yes --purge && rm ~/.local/bin/jingle
set -eu

repo=OPDhaker/jingle

say() { printf '%s\n' "$*"; }
die() {
	printf 'jingle install: %s\n' "$*" >&2
	exit 1
}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "unsupported OS $(uname -s): jingle supports macOS and Linux for now" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported CPU $(uname -m): jingle supports amd64 and arm64" ;;
esac

version=${JINGLE_VERSION:-latest}
if [ -n "${JINGLE_DOWNLOAD_URL:-}" ]; then
	base=${JINGLE_DOWNLOAD_URL%/}
elif [ "$version" = latest ]; then
	base=https://github.com/$repo/releases/latest/download
else
	base=https://github.com/$repo/releases/download/$version
fi
dir=${JINGLE_INSTALL_DIR:-$HOME/.local/bin}
archive=jingle_${os}_${arch}.tar.gz

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "sha256sum or shasum is required to verify the download"
fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t jingle)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

say "Downloading $archive ($version)..."
curl -fsSL -o "$tmp/$archive" "$base/$archive" || die "download failed: $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "download failed: $base/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
got=$(sha256 "$tmp/$archive")
[ "$got" = "$want" ] || die "checksum mismatch for $archive (got $got, want $want); not installing"

tar -xzf "$tmp/$archive" -C "$tmp" jingle || die "cannot unpack $archive"
mkdir -p "$dir" || die "cannot create $dir"
# Copy to a temp name, then rename: hooks may run jingle at any moment, and
# must never see a half-written binary.
cp "$tmp/jingle" "$dir/.jingle.tmp.$$" || die "cannot write to $dir"
chmod 755 "$dir/.jingle.tmp.$$"
mv -f "$dir/.jingle.tmp.$$" "$dir/jingle"

installed=$("$dir/jingle" version 2>/dev/null) || die "installed $dir/jingle, but it does not run"
say "Installed $installed"
say "  -> $dir/jingle"

case ":$PATH:" in
*":$dir:"*)
	other=$(command -v jingle 2>/dev/null || true)
	if [ -n "$other" ] && [ "$other" != "$dir/jingle" ]; then
		say ""
		say "Note: another jingle at $other comes first on your PATH."
	fi
	;;
*)
	say ""
	say "Note: $dir is not on your PATH. Add it to your shell profile:"
	say "  export PATH=\"$dir:\$PATH\""
	;;
esac

say ""
say "jingle is not turned on yet. Next steps:"
say "  jingle install --yes                           # hooks for every repo"
say "  jingle config set commit.sounds ~/Music/tag.mp3"
say "  jingle play commit                             # check the audio"
say ""
say "To remove it: jingle uninstall --yes --purge && rm \"$dir/jingle\""
