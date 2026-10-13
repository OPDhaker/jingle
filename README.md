# jingle

Play a producer tag when you `git commit` or `git push`.

Install it once and it works in every repo on your machine, with no per-repo setup. Your existing git hooks keep running exactly as before, and `jingle uninstall` puts your git config back byte for byte.

macOS and Linux. (Windows is planned, not supported yet.)

## Install

**Homebrew** (macOS and Linux):

```sh
brew install OPDhaker/tap/jingle
```

**Install script** (puts the binary in `~/.local/bin`, no sudo, verifies the sha256 checksum):

```sh
curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | sh
```

Set `JINGLE_VERSION=v0.1.0` to pin a version, or `JINGLE_INSTALL_DIR` to install somewhere else.

**From source** (Go 1.25+):

```sh
go install github.com/OPDhaker/jingle/cmd/jingle@latest
```

Installing the binary doesn't change anything yet. Turning it on is a separate step:

## Quick start

```sh
jingle install --yes      # turn on for every repo
jingle play commit        # check the audio
jingle doctor             # check for problems
```

That's it: built-in sounds play on commit and push with no audio files needed. To use your own producer tag:

```sh
jingle config set commit.sounds ~/Music/tag.mp3   # jingle keeps its own copy
jingle config set push.sounds ~/Music/push.mp3
```

Every command that changes something takes `--dry-run` to show exactly what it would do first.

## Configuration

```sh
jingle config list
jingle config set <key> <value>
jingle config add commit.sounds ~/Music/tag2.mp3   # several sounds: one is picked at random
jingle config remove commit.sounds tag2.mp3
```

| Key | Default | Meaning |
|---|---|---|
| `enabled` | `true` | Master switch. `false` mutes everything but leaves the hooks installed. |
| `commit.enabled`, `push.enabled` | `true` | Per-event switches. |
| `merge.enabled` | `false` | Play on real merge commits (not fast-forwards). |
| `commit.sounds`, `push.sounds`, `merge.sounds` | `builtin:chime`, `builtin:fanfare`, `builtin:harmony` | Audio files (mp3, wav, aiff, m4a, ogg, flac) or built-in sounds. |
| `push.when` | `success` | `success`: play only when the push went through. `attempt`: play when the push starts. |
| `cooldown` | `3` | Seconds; a burst of commits plays once. |
| `volume` | `100` | 0–100. |

### Built-in sounds

jingle ships four short sounds, synthesized in code (so there's nothing to license or download). Put them in any sounds list by name:

| Sound | What it is | Default for |
|---|---|---|
| `builtin:chime` | two-note rising bell | commit |
| `builtin:fanfare` | quick rising brass arpeggio | push |
| `builtin:harmony` | two voices resolving to a chord | merge |
| `builtin:boom` | snare hit into an 808 drop | |

```sh
jingle config set push.sounds builtin:boom
jingle play push --sound boom
```

The config lives in `~/.config/jingle/config.toml`, and jingle's copies of your sounds in `~/.local/share/jingle/` (both honor `XDG_CONFIG_HOME` / `XDG_DATA_HOME`).

## Uninstall

```sh
jingle uninstall --yes --purge
```

That restores your global git config exactly as it was before `jingle install`, removes jingle's hooks, and (with `--purge`) deletes its config and imported sounds. Without `--purge`, your config and sounds are kept for a later reinstall. Then remove the binary:

```sh
brew uninstall jingle       # Homebrew
rm ~/.local/bin/jingle      # install script
```

**If you delete the binary without running `jingle uninstall` first**, git keeps working: the hooks skip the sound and still run your own hooks. To clean up afterwards, `git config --global --unset core.hooksPath` turns jingle's hooks off (if you had your own global `core.hooksPath` before, set it back instead), and `rm -rf ~/.config/jingle ~/.local/share/jingle` removes the rest.

## How it works

`jingle install` points git's global `core.hooksPath` at jingle's hooks dir. Each hook there first runs the hook git would have run without jingle (the repo's `.git/hooks/<name>`, or your previous global hooks path), with the same arguments, stdin, and exit status, and then starts the sound in the background. A hook never waits for the audio, never fails because of jingle, and adds a few tens of milliseconds per commit.

- **Commit**: `post-commit`. Silent while a rebase, cherry-pick, or revert replays commits.
- **Push**: git has no post-push hook, so `pre-push` marks the push and the sound plays when the remote-tracking ref update proves it succeeded. A rejected or up-to-date push stays silent. Pushes to a raw URL (no remote name) play on attempt.
- **Merge** (opt-in): `post-merge`, only for real merge commits.

Audio players: `afplay` on macOS; `pw-play`, `paplay`, `aplay`, or `ffplay` on Linux (first one found).

### Limits

- Repos that set their own `core.hooksPath` (husky and similar) override the global one, so jingle doesn't play there. `jingle doctor` inside such a repo says so.
- Success-only push detection needs git 2.28 or newer; older git plays on attempt.

## For scripts and AI agents

jingle is built to be driven without a person at the keyboard: no prompts (`--yes` confirms), `--json` on every command, stable exit codes (0 ok, 1 error, 2 usage), and idempotent commands that report `unchanged` on retry.

- **Agent skill.** Step-by-step instructions an agent follows to install, configure, check, and remove jingle. In Claude Code:
  ```
  /plugin install jingle --marketplace OPDhaker/jingle
  ```
  (or `/plugin marketplace add OPDhaker/jingle`, then `/plugin install jingle@jingle`). For any agent, once the binary is installed, `jingle skill` prints the same guide as markdown. It matches the installed version: `mkdir -p ~/.claude/skills/jingle && jingle skill > ~/.claude/skills/jingle/SKILL.md`.
- **`jingle schema`** prints every command, flag, config key (type, default, allowed values), built-in sound, and exit, error, and problem code as JSON.
- The JSON output contract is documented in [docs/json.md](docs/json.md). Every command's `--help` has examples.

## License

MIT
