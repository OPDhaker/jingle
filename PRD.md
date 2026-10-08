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

**Result:** all of the above is in place, and `integration/` covers the exit criteria, plus chaining to a previous global `core.hooksPath`, linked worktrees, and `pre-push` stdin/args passthrough. Push played on attempt: from `pre-push`, only if there was something to send and the user's `pre-push` passed (Phase 2 changed this to success only). All 21 hooks git runs from the hooks dir get pass-through shims, adding 35–60 ms per commit on macOS depending on the run. Verified locally on macOS only.

## Phase 2: Correctness & polish (in progress)

- ✅ Push sound only on a successful push (`pre-push` marker + `reference-transaction`, validated in Phase 0). "Play on attempt" fallback for pushes to a raw URL.
- ✅ Suppress sounds during rebase and cherry-pick (detect `rebase-merge/` and `CHERRY_PICK_HEAD`); cooldown so a burst of commits plays once.
- Merges don't fire `post-commit`; optionally add a `post-merge` event.
- ✅ `doctor`: detects repo-local `core.hooksPath` (husky etc.), missing player, old git, broken audio path; each problem gets a fix hint.
- Opt-in husky integration. **Deferred**; see "Husky findings" below.
- Multiple sounds per event, picked at random. Volume setting.

**Exit:** `doctor --json` covers every known failure mode; no sound plays on a failed push or during a rebase.

**Progress (pass 1):** no sound on a rejected, dry-run, or up-to-date push. `push.when = attempt` restores the old behavior. Raw-URL pushes and git < 2.28 fall back to it automatically. Rebase, `pull --rebase`, cherry-pick, and revert are silent, linked worktrees included. `cooldown` (default 3 s, per event) collapses bursts, and `play` ignores it. Covered in `integration/`. Verified locally on macOS only: GitHub Actions has not started any run on the repo yet (it shows no runs at all; likely a billing limit on the private repo), so Linux and Windows are still untested. **Progress (pass 2):** `jingle doctor [--repo <dir>]` reports every known failure mode with a severity (`error` stops sounds, `warning` degrades them) and a fix. It exits 1 on any error (`problems_found`, report still on stdout). It checks the repo in the current directory, or `--repo`, for an effective `core.hooksPath` that overrides ours (local, worktree, `includeIf`) and names husky. It also catches a deleted shim binary and a bad `JINGLE_PLAYER`. `status` shares the same checks (package `internal/doctor`), minus the repo check, and still exits 0. Also verified: hook overhead is unchanged from Phase 1.

**Remaining:** multiple sounds per event + volume; optional `merge` event (`post-merge`, default off); husky (deferred). Earlier sketch for multiple sounds (not yet decided): config key `<event>.sounds` as a list, still accepting the old single `sound` string; `config add`/`config remove`; `play --sound` to pick one; volume as a global 0–100 mapped to each player's flag (afplay `-v`, pw-play/paplay `--volume`, ffplay `-volume`; aplay has none).

**Husky findings (for when husky work resumes):**
- Husky v9 sets a repo-local `core.hooksPath=.husky/_`, which overrides jingle's global one. Husky creates wrappers for 14 hooks only; it has no `reference-transaction` wrapper, so success-only push detection can't work in husky repos through husky.
- `~/.config/husky/init.sh` is **not** a usable hook point. Husky's `h` script exits before sourcing it unless the repo has its own `.husky/<hook>`. Most repos have no `.husky/post-commit`, so the commit sound would never play.
- git 2.54 added config-based hooks: `hook.<name>.event` (multi-valued) and `hook.<name>.command`, in any config scope. They run in addition to the hooks-dir hook (configured hooks first, hooks dir last), so they also run in husky repos. `hook.<name>.enabled=false` opts one repo out. This is the likely route, and on git ≥ 2.54 it could replace setting `core.hooksPath` entirely (no shims, no chaining). Spike first:
  - Does each configured hook get the full stdin for `pre-push` and `reference-transaction`?
  - How are exit codes combined?
  - Do `post-commit`, `pre-push`, and `reference-transaction` all go through the hook API?
  - How should uninstall restore the gitconfig byte for byte (a new `[hook "jingle"]` section)?
- Until then, `doctor` reports husky repos as `repo_hooks_path_override` with `repo.husky = true`. Older git (< 2.54) has no clean way in.

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
