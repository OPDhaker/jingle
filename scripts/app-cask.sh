#!/bin/sh
# Prints the Homebrew cask for the menu-bar app (Casks/jingle-app.rb in
# OPDhaker/homebrew-tap). The release workflow runs it after uploading
# Jingle_macos_universal.dmg to the GitHub release.
#
#   scripts/app-cask.sh <version without v> <sha256 of the dmg>
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <version> <sha256>" >&2
	exit 2
fi
version=$1
sha=$2

cat <<CASK
cask "jingle-app" do
  version "$version"
  sha256 "$sha"

  url "https://github.com/OPDhaker/jingle/releases/download/v#{version}/Jingle_macos_universal.dmg"
  name "Jingle"
  desc "Menu-bar app for jingle, the git commit and push producer tag"
  homepage "https://github.com/OPDhaker/jingle"

  # The app runs the jingle CLI for everything it does.
  depends_on cask: "opdhaker/tap/jingle"
  depends_on macos: ">= :big_sur"

  app "Jingle.app"

  # The app isn't notarized, and Homebrew quarantines cask downloads, so
  # Gatekeeper would block it on first launch.
  postflight do
    system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{appdir}/Jingle.app"]
  end

  uninstall quit: "dev.opdhaker.jingle"

  zap trash: [
    "~/Library/Application Support/dev.opdhaker.jingle",
    "~/Library/Caches/dev.opdhaker.jingle",
    "~/Library/WebKit/dev.opdhaker.jingle",
  ]
end
CASK
