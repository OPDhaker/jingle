# Releasing

A pushed `v*` tag runs `.github/workflows/release.yml`: tests, then GoReleaser (`.goreleaser.yaml`), which:

- builds darwin/linux × amd64/arm64 and attaches `jingle_<os>_<arch>.tar.gz`, `checksums.txt`, and `install.sh` to a GitHub release;
- pushes `Casks/jingle.rb` to [`OPDhaker/homebrew-tap`](https://github.com/OPDhaker/homebrew-tap).

Then the `app` job builds the menu-bar app on macOS as one universal `.dmg` (Apple Silicon + Intel), attaches it to the same release as `Jingle_macos_universal.dmg`, and pushes `Casks/jingle-app.rb` (written by `scripts/app-cask.sh`) to the tap. The tag sets the app's version.

Versions are semver tags: `v0.1.0`, `v0.2.0`, and so on. A tag with a suffix (`v0.2.0-rc.1`) is marked as a pre-release and doesn't update the tap (for either cask).

## One-time setup

1. Create the tap repo (it must be public and named `homebrew-tap`):

   ```sh
   gh repo create OPDhaker/homebrew-tap --public --description "Homebrew tap for jingle"
   ```

2. Create a fine-grained personal access token at <https://github.com/settings/personal-access-tokens/new>:
   - Repository access: **Only select repositories** → `OPDhaker/homebrew-tap`.
   - Permissions: **Contents: Read and write**. Nothing else.

3. Store it as a secret on the jingle repo:

   ```sh
   gh secret set HOMEBREW_TAP_TOKEN --repo OPDhaker/jingle
   ```

## Before tagging

```sh
make test && make lint
goreleaser release --snapshot --clean      # builds dist/, publishes nothing
scripts/e2e-clean-machine.sh dist          # install → play → uninstall --purge
```

CI runs the same checks (`release-e2e` job), plus the Linux player smoke test.

The app's release build, as the `app` job runs it (needs `rustup target add x86_64-apple-darwin` once):

```sh
cd gui && pnpm tauri build --target universal-apple-darwin --bundles dmg --config '{"version":"0.0.0-test"}'
# → src-tauri/target/universal-apple-darwin/release/bundle/dmg/Jingle_0.0.0-test_universal.dmg
```

If the JSON output changed, check that `docs/json.md` matches, and bump `schema_version` (`internal/cli/version.go`) for a breaking change.

## Release

```sh
git tag v0.1.0
git push origin v0.1.0
```

Then watch the `release` workflow. Afterwards, check:

```sh
brew update && brew trust opdhaker/tap && brew install OPDhaker/tap/jingle && jingle version
brew install --cask OPDhaker/tap/jingle-app && open -a Jingle
curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | JINGLE_INSTALL_DIR=/tmp/jingle-check sh
```

## If a release goes wrong

Delete the GitHub release and the tag (`git push --delete origin v0.1.0`), fix the problem, and tag again. If the cask was already pushed, the next release overwrites it.

## App signing

The app is ad-hoc signed (`signingIdentity: "-"` in `gui/src-tauri/tauri.conf.json`), not notarized. Homebrew users see no warning, because the cask removes the quarantine flag. People who download the `.dmg` have to click **Open Anyway** in System Settings → Privacy & Security once.

To sign and notarize, with a paid Apple Developer account:

1. Export the "Developer ID Application" certificate as a `.p12` and add these secrets to the jingle repo: `APPLE_CERTIFICATE` (the `.p12`, base64), `APPLE_CERTIFICATE_PASSWORD`, `APPLE_SIGNING_IDENTITY` (e.g. `Developer ID Application: Name (TEAMID)`), `APPLE_ID`, `APPLE_PASSWORD` (an app-specific password), and `APPLE_TEAM_ID`.
2. Pass them as `env` to the `Build` step of the `app` job. Tauri reads them, signs, and notarizes.
3. Remove `"signingIdentity": "-"` from `tauri.conf.json` and the `postflight` block from `scripts/app-cask.sh`.
