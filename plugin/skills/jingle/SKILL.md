---
name: jingle
description: Install, configure, check, or remove jingle, the CLI that plays a short sound (a "producer tag") on every git commit and git push, in every repo on the machine. Use when the user wants a sound when they commit, push, or merge, says "producer tag", or asks to set up, change sounds for, mute, troubleshoot, or uninstall jingle.
---

# jingle

jingle plays a short audio clip when the user runs `git commit` or `git push` (and, opt-in, on merge commits). It is installed once, globally, by pointing git's global `core.hooksPath` at jingle's hooks dir. The user's existing hooks keep running, and uninstall restores the git config byte for byte. It has built-in sounds, so you don't need any audio files to set it up. macOS and Linux only.

Every command is non-interactive, takes `--json`, and is safe to retry (a repeat reports `unchanged`). Exit codes: `0` the requested state was reached, `1` error, `2` usage (bad command, flag, or value, or `--yes` missing). With `--json`, errors are `{"error": {"code": "...", "message": "..."}}` on stderr.

## Set up

1. **Get the binary.** If `command -v jingle` finds it, skip to step 2. Otherwise use the first of these that works:
   - Homebrew available (`command -v brew`): `brew trust opdhaker/tap` (Homebrew 7 and later refuse untrusted third-party taps; on older Homebrew the command doesn't exist and isn't needed), then `brew install OPDhaker/tap/jingle`
   - Otherwise: `curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | sh`. This installs to `~/.local/bin` without sudo. If that dir isn't on `PATH`, call `~/.local/bin/jingle` directly and tell the user to add it to their shell profile.
   - Go 1.25+ available: `go install github.com/OPDhaker/jingle/cmd/jingle@latest`

   Check with `jingle version --json`.

2. **Install the hooks.** This changes the user's global git config, so do it only when the user asked for jingle to be set up. Preview first, then apply:
   ```sh
   jingle install --dry-run --json
   jingle install --yes --json
   ```
   Never set or unset `core.hooksPath` yourself. jingle saves any previous value, chains to it, and restores it on uninstall.

3. **Sounds.** Commit plays `builtin:chime` and push plays `builtin:fanfare` out of the box. Change them only if the user wants to:
   ```sh
   jingle config set commit.sounds ~/Music/tag.mp3      # the user's own file (jingle keeps a copy)
   jingle config set push.sounds builtin:boom           # another built-in
   jingle config add commit.sounds ~/Music/tag2.wav     # several sounds: one plays at random
   jingle config set merge.enabled true                 # opt-in sound for real merge commits
   ```
   Built-ins: `builtin:chime`, `builtin:fanfare`, `builtin:harmony`, `builtin:boom`. Supported file types are mp3, wav, aiff, aif, m4a, ogg, and flac. Always change settings through `jingle config`, never by editing `config.toml`; the CLI validates every change.

4. **Verify.**
   ```sh
   jingle play commit --wait --json    # plays the commit sound now; exit 0 = the player ran
   jingle doctor --json                # exit 0 = no errors
   ```
   If `doctor` exits 1, read `problems[]` in its stdout. Each problem has a stable `code`, a `severity` (`error` means no sound; `warning` means a degraded sound), and usually a `fix` command. Run each fix, then run `jingle doctor --json` again. Run `doctor` inside a repo, or pass `--repo <dir>`, to check that repo too.

5. **Tell the user** what you did: it is on for every repo, which sound plays for which event, and that `jingle config set enabled false` mutes it.

## Common changes

| Want | Command |
|---|---|
| Mute everything (hooks stay installed) | `jingle config set enabled false` |
| Only commits, no push sound | `jingle config set push.enabled false` |
| Play the push sound when the push starts, not when it succeeds | `jingle config set push.when attempt` |
| Quieter | `jingle config set volume 50` |
| Allow back-to-back sounds | `jingle config set cooldown 0` |
| Hear a sound before choosing it | `jingle preview builtin:boom --wait` |
| See every setting | `jingle config list --json` |
| Full status (installed? player? git version? problems?) | `jingle status --json` |

## Uninstall

Run this **before** removing the binary:

```sh
jingle uninstall --yes --purge --json   # restores git config; --purge also deletes jingle's config and sounds
brew uninstall jingle                   # if installed with Homebrew
rm ~/.local/bin/jingle                  # if installed with install.sh
```

If the binary is already gone, git still works (the hooks skip the sound and still run the user's own hooks). To clean up by hand: `git config --global --unset core.hooksPath` (or restore the user's earlier value), then `rm -rf ~/.config/jingle ~/.local/share/jingle`.

## Known limits

- **`repo_hooks_path_override`**: the repo sets its own `core.hooksPath` (husky and similar), so jingle stays silent there. If `repo.husky` is true, there is no fix yet; tell the user. Otherwise the fix unsets the repo's value, which stops that repo's own hooks dir from running. Ask the user before you run it.
- **`git_too_old`** (git < 2.28): push plays when the push starts instead of when it succeeds.
- **`no_player`**: a headless machine or container may have no audio player (macOS: `afplay`; Linux: `pw-play`, `paplay`, `aplay`, or `ffplay`). Nothing can play there, and that's expected.
- Rebases, cherry-picks, and reverts are silent on purpose. A burst of commits plays once (`cooldown`, default 3 s).

## Reference

`jingle schema` prints a JSON description of every command, flag, config key (type, default, allowed values), built-in sound, exit code, error code, and problem code. Every command's `--help` has examples. Run `jingle skill` to print this guide as it matches the installed version.
