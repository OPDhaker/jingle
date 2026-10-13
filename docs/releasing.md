# Releasing

A pushed `v*` tag runs `.github/workflows/release.yml`: tests, then GoReleaser (`.goreleaser.yaml`), which:

- builds darwin/linux × amd64/arm64 and attaches `jingle_<os>_<arch>.tar.gz`, `checksums.txt`, and `install.sh` to a GitHub release;
- pushes `Casks/jingle.rb` to [`OPDhaker/homebrew-tap`](https://github.com/OPDhaker/homebrew-tap).

Versions are semver tags: `v0.1.0`, `v0.2.0`, and so on. A tag with a suffix (`v0.2.0-rc.1`) is marked as a pre-release and doesn't update the tap.

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

If the JSON output changed, check that `docs/json.md` matches, and bump `schema_version` (`internal/cli/version.go`) for a breaking change.

## Release

```sh
git tag v0.1.0
git push origin v0.1.0
```

Then watch the `release` workflow. Afterwards, check:

```sh
brew update && brew install OPDhaker/tap/jingle && jingle version
curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | JINGLE_INSTALL_DIR=/tmp/jingle-check sh
```

## If a release goes wrong

Delete the GitHub release and the tag (`git push --delete origin v0.1.0`), fix the problem, and tag again. If the cask was already pushed, the next release overwrites it.
