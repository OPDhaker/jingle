# PRD: jingle

Play a producer tag when you `git commit` / `git push`. Install once, globally; works in every repo. Agent-first CLI now, GUI later. Constraints and architecture: see `CLAUDE.md`.

## Goals

- One command to install globally; one to uninstall that leaves git config exactly as it was.
- Never slows down, blocks, or breaks a git operation, and never disables a user's existing hooks.
- Fully drivable by an AI agent: no prompts, `--json` everywhere, stable exit codes, idempotent.

## Non-goals (for now)

- GUI (Phase 5).
- Per-repo configuration.
- Sound on events other than commit and push.

---

## Phase 0: Decisions & spikes ✅ done

| Item | Output |
|---|---|
| Language + CLI name | Decision recorded in `CLAUDE.md` |
| Config format + location | Decision recorded (OS config dir, human-readable) |
| Spike: `reference-transaction` push detection | Works / doesn't, across git versions; fallback = `pre-push` |
| Spike: detached playback per OS | Working command for macOS, Linux, Windows; hook returns in < 50 ms |

**Exit:** all four decided; spikes written up in `CLAUDE.md`.

**Result:** Go + `jingle`, TOML in XDG/AppData dirs. Push detection via a marker keyed by the `git push` process plus `reference-transaction` works. Detached playback adds 10–30 ms on macOS. Linux/Windows playback still unverified (→ Phase 3). Details: `spikes/README.md`. The Go skeleton (`jingle version`, CI, lint) is also in place.

## Phase 1: MVP (macOS + Linux) ✅ done

- `install` / `uninstall` (`--yes`, `--dry-run`): set global `core.hooksPath`, save and restore the previous value.
- Hook shims for `post-commit` and `pre-push`, chaining to the repo's `.git/hooks/` and to any previous global hooks path.
- Hidden event subcommand: reads config, plays sound detached, always exits 0.
- `config get|set|list`: global on/off, per-event on/off, per-event audio file (copied into data dir).
- `play <event>`: test the sound without committing.
- `status`: install state, effective config, detected player.
- `--json`, documented exit codes, plain output when not a TTY.

**Exit:** integration tests (real git, temp `HOME`) prove: install → commit plays → user's repo hook still runs with the same exit code → uninstall restores config byte-for-byte. Install and uninstall are idempotent.

**Result:** all of the above is in place, and `integration/` covers the exit criteria, plus chaining to a previous global `core.hooksPath`, linked worktrees, and `pre-push` stdin/args passthrough. Push plays on attempt: from `pre-push`, only if there is something to send and the user's `pre-push` passes. All 21 hooks git runs from the hooks dir get pass-through shims, adding about 35 ms per commit on macOS. Verified locally on macOS only; Linux relies on the CI job.

## Phase 2: Correctness & polish

- Push sound only on a successful push (`pre-push` marker + `reference-transaction`, validated in Phase 0). Optional "play on attempt" fallback for pushes to a raw URL.
- Suppress sounds during rebase and cherry-pick (detect `rebase-merge/` and `CHERRY_PICK_HEAD`); cooldown so a burst of commits plays once. Merges don't fire `post-commit`; optionally add a `post-merge` event.
- `doctor`: detects repo-local `core.hooksPath` (husky etc.), missing player, old git, broken audio path; each problem gets a fix hint.
- Opt-in husky integration.
- Multiple sounds per event, picked at random. Volume setting.

**Exit:** `doctor --json` covers every known failure mode; no sound plays on a failed push or during a rebase.

## Phase 3: Windows + distribution

- Windows playback and shims verified under Git for Windows.
- Release CI: cross-platform binaries, checksums, semver.
- Package channels: Homebrew, Scoop/winget, plus a one-line install script.
- Publish the JSON output schema and exit-code table; schema changes are versioned.

**Exit:** clean-machine install → commit plays sound → uninstall, on all three OSes, through each package channel.

## Phase 4: Agent integration

- Ship an agent skill / `AGENTS.md` snippet: "install and configure `jingle` for this user."
- `jingle schema` (or `--help --json`): machine-readable description of the commands.
- Bundled default tag(s), so setup works with zero files supplied.

**Exit:** a fresh agent session sets up the tool end to end from the skill alone, with no human steps.

## Phase 5: GUI

- Tray / menu-bar app (Tauri suggested) over the core library or `jingle --json`; no duplicated logic.
- Toggle on/off and per-event switches, pick and preview audio, install/uninstall, show `doctor` results.

**Exit:** everything the GUI does is also possible from the CLI, and both read and write the same config.

---

## Open questions

- Should commits made by agents (e.g. detected by commit author or environment) have their own toggle or sound?
- How does the tool behave in CI and other headless environments (detect them and stay silent)?
- Licensing for bundled default sounds.
