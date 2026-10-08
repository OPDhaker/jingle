# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Phase 1 of `PRD.md` (MVP, macOS + Linux) is done: `install`, `uninstall`, `config list|get|set`, `play`, `status`, and the hidden `hook-event`, with integration tests against real git (`integration/`). Phase 2 is partly done: the push sound plays only on a successful push (`push.when` config key), replayed commits (rebase, cherry-pick, revert) are silent, a per-event `cooldown` is in place, and `doctor` reports every known failure mode. Still open: multiple sounds per event + volume, and an optional `merge` event. Husky integration is deferred; the research is in `PRD.md` under "Husky findings" (short version: husky's `init.sh` doesn't work for most repos; git 2.54 config-based hooks are the likely route). Phase 0 decisions are below; spikes are in `spikes/` (findings in `spikes/README.md`). **Scope right now: CLI only.** A GUI comes later and must be a thin client over the same core and config. Don't build GUI code yet.

## Commands

```sh
make build                                  # → bin/jingle (version injected via -ldflags)
make test                                   # go test ./...
go test ./internal/config -run TestResolvePaths   # single test
go test ./integration/                      # builds bin, runs real git in a sandbox (skipped on Windows)
make lint                                   # golangci-lint v2 (config: .golangci.yml)
make fmt                                    # gofmt + goimports via golangci-lint
```

If golangci-lint isn't installed: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run`. CI (`.github/workflows/ci.yml`) runs tests and builds on macOS, Ubuntu, and Windows, plus lint.

`spikes/playback` is its own Go module, so `./...` from the root skips it. Spikes are throwaway; don't import from them.

## Decisions (Phase 0)

| Item | Decision |
|---|---|
| Name / binary | `jingle` (not "tagit", which reads as `git tag`) |
| Module | `github.com/OPDhaker/jingle`, Go 1.25 |
| CLI framework | `spf13/cobra` |
| Config | TOML (`BurntSushi/toml`, add it in Phase 1), with a top-level `version = 1` |
| Paths | Config: `$XDG_CONFIG_HOME/jingle/config.toml` (default `~/.config/jingle/`, macOS too); Windows `%AppData%\jingle\`. Data: `$XDG_DATA_HOME/jingle/` (default `~/.local/share/jingle/`); Windows `%LocalAppData%\jingle\`. Holds `hooks/`, `sounds/`, `run/`, `state.json`. |
| Config vs. state | User preferences go in `config.toml`. Tool-owned machine state (the saved previous `core.hooksPath`, install info) goes in `state.json`. Never mix them. |
| Test isolation | `JINGLE_HOME` puts config and data in one dir. `JINGLE_PLAYER` replaces the detected player (called as `<exe> <file>`). Integration tests must also set `HOME` and `GIT_CONFIG_GLOBAL` to temp paths, and must never touch the real `~/.gitconfig`. |
| Playback | Shell out to the OS player. No audio library, no cgo. |

## Code layout and conventions

- `cmd/jingle`: `main` only. `internal/cli`: cobra commands. `internal/output`: JSON/plain rendering, error envelope, exit codes. `internal/config`: paths, config, state, sound import. `internal/hooks`: install/uninstall/inspect and shim templates. `internal/gitcfg`: the `git config` / `git --version` calls. `internal/event`: decides whether and what to play (shared by `play` and `hook-event`), including push markers and the cooldown. `internal/doctor`: problem detection shared by `status`, `doctor`, and the future GUI. A new failure mode gets a check (code, severity, fix) there. `internal/player`: player detection and detached launch. `internal/fsutil`: atomic writes. `integration/`: end-to-end tests with the built binary.
- `run/` in the data dir is scratch space shared by hooks: push markers (`push-<pid>`) and cooldown stamps and locks (`last-<event>`, `lock-<event>`). Deleting it at any time is safe, and uninstall removes it. Removing a push marker is the claim to play: only the process whose `os.Remove` succeeds plays, so a push never plays twice.
- Shims pass through **every** hook git runs from the hooks dir (`hooks.HookNames`), not only the two that play, because setting `core.hooksPath` hides all repo hooks. If a previous global `core.hooksPath` existed, shims chain to it (not to the repo hooks, which git was already ignoring), so behavior is unchanged.
- Uninstall restores the gitconfig byte for byte using facts recorded in `state.json` at install (file existed? `[core]` had keys? final newline?). Change that logic only together with `TestUninstallRestoresGitconfigByteForByte`.
- Every cobra `RunE` returns `nil` or an `*output.Error` (stable `Code` + exit code). Any other error is assumed to come from cobra's argument parsing and is reported as `usage` / exit 2. Results are printed only through `output.Printer.Result`, which keeps `--json` output consistent.
- Exit codes (`internal/output`): 0 ok, 1 error, 2 usage. Add new codes; never renumber existing ones.
- Each command's cobra `Example:` field holds real invocations. Agents learn the tool from `--help`.
- `cli.Run(args, stdout, stderr)` is the test entry point. CLI tests call it directly instead of building a binary.

## What this is

A tool that plays a short audio clip (a music "producer tag") when the developer runs `git commit` and/or `git push`. It is meant to be installed **once, globally**, and work in every repo on the machine with no per-repo setup. Users turn it on/off, choose which events trigger a sound (commit, push, or both), and choose the audio file per event, first through a CLI and later through a GUI. The goal is a polished product people can publish and install.

Prior art: [leomosley/tagthat](https://github.com/leomosley/tagthat) (Bun CLI) does the same thing, but it installs a `pre-push` hook **per repo** and keeps the audio in a `.tagthat/<name>/` folder inside each repo. This project's main difference is that it is global and set up once.

## Core design constraints (git behavior that drives the architecture)

- **Global hooks via `core.hooksPath`.** `git config --global core.hooksPath <dir>` points every repo at one shared hooks directory. Things that follow from this:
  - Once it is set, git **ignores each repo's `.git/hooks/`**. Every global hook script must find the repo's own hook (`$(git rev-parse --git-dir)/hooks/<name>`), run it, pass along its stdin and arguments, and **return its exit code**. If it doesn't, user hooks (lint, tests) stop running without any warning.
  - A repo-level `core.hooksPath` (husky and similar tools set one) overrides the global value, so our hooks won't run in those repos. `jingle doctor` detects this (`repo_hooks_path_override`, and it names husky). The candidate fix is git 2.54's config-based hooks (`hook.<name>.event`/`.command`), which run regardless of `core.hooksPath`; not built yet.
  - When installing, save any global `core.hooksPath` the user already has, and chain to it too. Uninstall must put the previous value back exactly.
  - `init.templateDir` is not a substitute. It only affects repos cloned or initialized after it is set.
- **Commit hook = `post-commit`.** It runs after the commit succeeds and cannot affect the commit. Note that it also fires on `--amend`.
- **There is no `post-push` hook.** `pre-push` runs *before* any data is sent, and a non-zero exit aborts the push. Verified design (`spikes/README.md`), for git ≥ 2.28:
  - `pre-push` writes a marker keyed by `$PPID` (the `git push` process), but only if its stdin has ref lines. Stdin is empty for "everything up-to-date" and for rejected pushes.
  - `reference-transaction` with argument `committed`, whose stdin updates `refs/remotes/*` from the same `$PPID`, means the push succeeded. Play the sound and delete the marker.
  - Fetches also update `refs/remotes/*`, but they run in a different process, so the PPID key ignores them. Stale markers are pruned after 1 h.
  - Falls back to playing on attempt (from `pre-push`) when `push.when = attempt`, when pushing to a raw URL (`$1 == $2`), and on git < 2.28.
  - Pushing to a raw URL updates no tracking ref, so success can't be detected; that case uses the attempt fallback above.
  - `reference-transaction` fires 3× for every ref update in every git command, so the shim must bail out in plain `sh` before starting `jingle`.
- **`post-commit` fires once per replayed commit during `rebase` / `pull --rebase`** (when `$GIT_DIR/rebase-merge/` exists) and during `cherry-pick` (when `$GIT_DIR/CHERRY_PICK_HEAD` exists). It never fires for `merge` (git runs `post-merge` instead). The post-commit shim stays silent while `rebase-merge/`, `rebase-apply/`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`, or `sequencer/` exists in the git dir. That check runs in `sh`, synchronously, because git deletes the state right after the last replayed commit, so a background `jingle` could miss it. Don't use `GIT_REFLOG_ACTION`; it's unreliable.
- **Hooks must never block or break git.** Start playback detached and exit right away. The shim runs `jingle ... >/dev/null 2>&1 </dev/null &`; Go starts the player with `Setsid`, nil stdio, and `Process.Release()`. Measured with all 21 pass-through shims installed: 35–60 ms extra per commit on macOS, depending on the run (about 12 hooks fire per commit; most of the cost is `sh` startup). Phase 2's shim changes measured the same as Phase 1 side by side (about 60 ms that day). Shims locate the repo's hooks dir without forking `git` when they can, because a `git rev-parse` per hook roughly tripled that. On macOS the first run of each newly written shim is slow, once (OS scan of new executables). A synchronous player adds about 2.4 s. A hook's own logic always exits 0. The only non-zero exit allowed is one passed through from a chained user hook. A missing config, missing audio file, or missing player means skip silently.
- **Hook scripts must be POSIX `sh`.** They run under Git for Windows' bundled sh as well as macOS and Linux shells. Keep them minimal: chain the user's hook, then start `jingle` in the background to handle the event. Only two things run synchronously in `sh`, both builtins only: `pre-push` writing the push marker, and `post-commit` checking for rebase/cherry-pick/revert state. If `jingle` is not on `PATH` (e.g. the user removed it without uninstalling), skip playback but still chain.
- **Audio players by OS:** macOS `afplay` (verified). Linux: try `pw-play`, `paplay`, `aplay`, then `ffplay -nodisp -autoexit` (unverified). Windows: PowerShell `System.Windows.Media.MediaPlayer` for MP3, `System.Media.SoundPlayer` for WAV only (unverified until Phase 3).

## Agent-first CLI

The main users of the CLI are AI coding agents (Claude Code and similar) setting it up and configuring it for someone. Humans come second. Every command must be fully usable without a person at the keyboard:

- **No interactive prompts, ever.** Every input is a flag or an argument. Destructive or global changes (install, uninstall) take a `--yes` flag rather than asking for confirmation.
- **`--json` on every command.** Stable, documented output schema. Errors go to stderr as JSON (`{"error": {"code": "...", "message": "..."}}`) when `--json` is set. Treat schema changes as breaking changes.
- **Meaningful exit codes**, documented and stable. `0` means the requested state was reached.
- **Idempotent.** Running `install` twice, or setting a value it already has, succeeds and reports "unchanged". Agents retry; retries must be safe.
- **Inspectable state.** `status` and `doctor` report everything an agent needs to decide what to do next: whether it's installed, the current and saved `core.hooksPath`, the effective config, which audio player was detected, git version (for `reference-transaction`), and known problems (e.g. a repo-local `core.hooksPath` overriding ours). Each problem has a stable `code`, a `severity` (`error` stops sounds, `warning` degrades them), and a `fix`. `status` always exits 0; `doctor` also checks the current repo and exits 1 when any problem is an error.
- **`--dry-run`** on anything that changes git config or files, listing exactly what would change.
- **Config changes go through the CLI** (`config get/set/list`-style subcommands) and are validated there. The config file stays human-readable, but agents should never need to hand-edit it.
- **A way to check audio without committing** (a `play`/`test <event>` command), so an agent can confirm setup end-to-end.
- **Plain output when not a TTY.** No color, spinners, or progress bars when stdout isn't a terminal or `NO_COLOR` is set.
- **`--help` with concrete examples** for every subcommand. Agents learn the tool from this text.

## Intended architecture

There are four loosely coupled parts. The config file is the only state they share.

1. **Core library**: config read/write/validation, hook install/uninstall (the `core.hooksPath` save/restore logic above), player detection, and event handling. All behavior lives here.
2. **CLI**: a thin layer over the core that adds argument parsing and the agent-first output rules above. It also has a hidden internal subcommand that the hook shims call per git event; that subcommand must exit 0 quickly no matter what happens.
3. **Hook shims**: the global hooks directory described above. They hold no logic beyond chaining, the push marker, and the replay check (see above); everything else is forwarded to the CLI.
4. **Config**: one user-level file (e.g. under `~/.config/<app>/`, or the OS-appropriate config dir) holding the global on/off switch, per-event on/off switches, per-event audio paths, `cooldown`, and `push.when`. User audio files are copied into the app's data dir so that moving the originals doesn't break anything.

No background daemon is needed, because git runs the hooks. The future GUI will call the core library (or `jingle --json`), not reimplement its logic.
