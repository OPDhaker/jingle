# PRD: jingle

Play a producer tag when you `git commit` / `git push`. Install once, globally; works in every repo. Agent-first CLI now, GUI later. Constraints and architecture: see `CLAUDE.md`.

## Goals

- One command to install globally; one to uninstall that leaves git config exactly as it was.
- Never slows down, blocks, or breaks a git operation, and never disables a user's existing hooks.
- Fully drivable by an AI agent: no prompts, `--json` everywhere, stable exit codes, idempotent.

## Non-goals (for now)

- GUI packaging and release (Phase 5 built the app; shipping it comes later).
- Per-repo configuration.
- Sound on events other than commit, push, and (opt-in) merge.

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

## Phase 2: Correctness & polish ✅ done (husky deferred)

- ✅ Push sound only on a successful push (`pre-push` marker + `reference-transaction`, validated in Phase 0). "Play on attempt" fallback for pushes to a raw URL.
- ✅ Suppress sounds during rebase and cherry-pick (detect `rebase-merge/` and `CHERRY_PICK_HEAD`); cooldown so a burst of commits plays once.
- ✅ Merges don't fire `post-commit`; optional `merge` event via `post-merge`.
- ✅ `doctor`: detects repo-local `core.hooksPath` (husky etc.), missing player, old git, broken audio path; each problem gets a fix hint.
- Opt-in husky integration. **Deferred**; see "Husky findings" below.
- ✅ Multiple sounds per event, picked at random. Volume setting.

**Exit:** `doctor --json` covers every known failure mode; no sound plays on a failed push or during a rebase.

**Progress (pass 1):** no sound on a rejected, dry-run, or up-to-date push. `push.when = attempt` restores the old behavior. Raw-URL pushes and git < 2.28 fall back to it automatically. Rebase, `pull --rebase`, cherry-pick, and revert are silent, linked worktrees included. `cooldown` (default 3 s, per event) collapses bursts, and `play` ignores it. Covered in `integration/`. Verified locally on macOS only: GitHub Actions has not started any run on the repo yet (it shows no runs at all; likely a billing limit on the private repo), so Linux and Windows are still untested. **Progress (pass 2):** `jingle doctor [--repo <dir>]` reports every known failure mode with a severity (`error` stops sounds, `warning` degrades them) and a fix. It exits 1 on any error (`problems_found`, report still on stdout). It checks the repo in the current directory, or `--repo`, for an effective `core.hooksPath` that overrides ours (local, worktree, `includeIf`) and names husky. It also catches a deleted shim binary and a bad `JINGLE_PLAYER`. `status` shares the same checks (package `internal/doctor`), minus the repo check, and still exits 0. Also verified: hook overhead is unchanged from Phase 1.

**Progress (pass 3):** `<event>.sound` became `<event>.sounds`, a list. `config set` replaces the list, `config add`/`config remove` change one entry, and each play picks at random among the files that exist. A missing file is a `sound_missing` warning, or an error only when every file is gone. `play --sound <name>` plays one specific sound. Old configs with `sound = "..."` load and migrate on the next save; the CLI key is gone (nothing has been released). Two different files with the same name get separate copies (`<event>-<stem>-2<ext>`). `volume` (0–100, default 100) maps to afplay `-v` and pw-play `--volume` (0–1 factor), paplay `--volume` (0–65536), and ffplay `-volume`; aplay and `JINGLE_PLAYER` get no flag, and doctor warns with `volume_unsupported`. `afplay -v` was checked on macOS; the Linux flags are unverified. The `merge` event (off by default) plays from `post-merge` only when HEAD has a second parent, so fast-forwards, fast-forward pulls, and squash merges stay quiet. A merge stopped by a conflict plays the commit sound when the resolution is committed. Hook overhead is unchanged (about 37 ms per commit on macOS).

**Remaining:** husky (deferred; see below).

**Husky findings (for when husky work resumes):**
- Husky v9 sets a repo-local `core.hooksPath=.husky/_`, which overrides jingle's global one. Husky creates wrappers for 14 hooks only; it has no `reference-transaction` wrapper, so success-only push detection can't work in husky repos through husky.
- `~/.config/husky/init.sh` is **not** a usable hook point. Husky's `h` script exits before sourcing it unless the repo has its own `.husky/<hook>`. Most repos have no `.husky/post-commit`, so the commit sound would never play.
- git 2.54 added config-based hooks: `hook.<name>.event` (multi-valued) and `hook.<name>.command`, in any config scope. They run in addition to the hooks-dir hook (configured hooks first, hooks dir last), so they also run in husky repos. `hook.<name>.enabled=false` opts one repo out. This is the likely route, and on git ≥ 2.54 it could replace setting `core.hooksPath` entirely (no shims, no chaining). Spike first:
  - Does each configured hook get the full stdin for `pre-push` and `reference-transaction`?
  - How are exit codes combined?
  - Do `post-commit`, `pre-push`, and `reference-transaction` all go through the hook API?
  - How should uninstall restore the gitconfig byte for byte (a new `[hook "jingle"]` section)?
- Until then, `doctor` reports husky repos as `repo_hooks_path_override` with `repo.husky = true`. Older git (< 2.54) has no clean way in.

## Phase 3: Distribution (macOS + Linux) ✅ done; Windows deferred

- Release CI: cross-platform binaries, checksums, semver. ✅ darwin/linux × amd64/arm64.
- Package channels: Homebrew and a one-line install script. ✅
- Publish the JSON output schema and exit-code table; schema changes are versioned. ✅
- `uninstall --purge`, so one command leaves a clean machine. ✅
- **Deferred:** Windows playback and shims under Git for Windows; Scoop/winget.

**Exit (scoped to macOS + Linux):** clean-machine install → commit plays sound → uninstall, through each package channel.

**Result:** a pushed `v*` tag runs GoReleaser (`.goreleaser.yaml`, `.github/workflows/release.yml`). It publishes versionless archives (`jingle_<os>_<arch>.tar.gz`), `checksums.txt`, and `install.sh` to the GitHub release, and a cask to `OPDhaker/homebrew-tap`. The cask clears quarantine (the binary is unsigned) and has no uninstall or zap hook: casks run uninstall hooks on upgrade, and a zap that removed the hooks dir would leave `core.hooksPath` pointing at nothing, silently disabling every repo's own hooks. `install.sh` checks the sha256, installs to `~/.local/bin` without sudo, and does not change git config. `uninstall --purge` also deletes the config and imported sounds, and removes the config/data dirs only when nothing else is in them. `docs/json.md` documents every command's JSON, the error, problem, and exit codes, and the versioning rule; `version --json` reports `schema_version` (1); golden tests (`internal/cli/testdata/json/`) pin every command's output. `scripts/e2e-clean-machine.sh` runs install.sh → install → commit and push play → `uninstall --purge` → gitconfig byte-identical with no files left; it passes on macOS and Ubuntu 24.04, and CI runs it on both. `scripts/linux-player-smoke` runs pw-play, paplay, aplay, and ffplay against null outputs in Docker: each is detected when alone on PATH and exits 0 with jingle's flags at volume 100 and 40. So the Linux command lines are verified, though nothing has been heard on real Linux hardware. The full test suite also passes on Linux. Still open: the first tagged release and the manual brew round trip (`docs/releasing.md`).

## Phase 4: Agent integration ✅ done

- ✅ Ship an agent skill: "install and configure `jingle` for this user."
- ✅ `jingle schema`: machine-readable description of the commands.
- ✅ Bundled default tag(s), so setup works with zero files supplied.

**Exit:** a fresh agent session sets up the tool end to end from the skill alone, with no human steps.

**Result:**
- **Built-in sounds.** jingle has four built-in sounds (`builtin:chime`, `fanfare`, `harmony`, `boom`), synthesized in Go (`internal/builtin`), so there are no audio assets and nothing to license.
  - They are the defaults: commit plays chime, push plays fanfare, merge plays harmony. `jingle install --yes` alone makes sound, and a fresh `doctor` reports only `not_installed`.
  - A built-in is rendered on first play to `<data>/sounds/builtin/<name>.wav`, a 16-bit 44.1 kHz WAV that every supported player handles. It is rewritten whenever it doesn't match the binary.
  - Found along the way: TOML drops nil lists, so a cleared sound list would have loaded back as the default. `Save` now writes `[]`. A legacy `sound` key now replaces the default list instead of joining it.
- **`jingle schema`** prints JSON built from the live cobra tree and from code tables (`config.KeyInfos`, `output.ErrorCodes`, `doctor.Codes`). The root `--help` error list is generated from the same tables.
  - `TestCodesDocumented` checks that the codes used in the source, the tables, and `docs/json.md` match row for row. It found `git_failed` (from `doctor --repo`), which had never been documented.
- **The skill** lives in `plugin/skills/jingle/SKILL.md`. It ships two ways from that one file:
  - through the plugin marketplace in this repo (`/plugin install jingle --marketplace OPDhaker/jingle`; `claude plugin validate` passes);
  - embedded in the binary: `jingle skill` prints it, so it always matches the installed version.

  `TestSkillMatchesCLI` checks that every command, flag, config key, and built-in the skill shows actually exists.
- **Exit check:** passed. A fresh `claude -p` session was given only the plugin, in a sandbox (`XDG_*`, `GIT_CONFIG_GLOBAL`, a logging `JINGLE_PLAYER`), with the prompt "Set up jingle so I hear a sound when I commit and push". It took 9 turns and no human steps. Afterwards doctor was clean, a commit played chime and a push played fanfare, and the real `~/.gitconfig` and jingle config were byte-identical. `scripts/e2e-clean-machine.sh` now asserts that the built-ins play with no `config set`.
- **Still open:** nobody has listened to the built-ins on Linux hardware. Tuning them by ear is welcome.

## Phase 5: GUI ✅ built for macOS; packaging deferred

- ✅ Tray / menu-bar app (Tauri suggested) over the core library or `jingle --json`; no duplicated logic.
- ✅ Toggle on/off and per-event switches, pick and preview audio, install/uninstall, show `doctor` results.

**Exit:** everything the GUI does is also possible from the CLI, and both read and write the same config.

**Result:**
- **Stack.** Tauri v2 in `gui/`: Rust in `gui/src-tauri/`, plain TypeScript and Vite in `gui/src/`, no UI framework. The `.app` is 6.3 MB.
- **Thin client.** The app runs the installed `jingle` CLI with `--json` for everything; `gui/src-tauri/src/jingle.rs` is the only code that talks to it. It doesn't ship its own copy of jingle. If it did, `install` would record a path inside the `.app` for the hooks to call, and moving or updating the app would break them. The window can call only a fixed set of commands (`commands.rs`), so it can't run arbitrary arguments such as `uninstall --purge`. Positional values go after `--`, so a file named `-x.mp3` can't be read as a flag.
- **Finding jingle.** It uses `$JINGLE_BIN` if that's set (and reports a bad one rather than falling back). Otherwise it asks the login shell (`$SHELL -ilc 'command -v jingle'`), then tries Homebrew, `~/.local/bin`, and `~/go/bin`. An app launched from Finder gets a bare PATH, so jingle runs with the login shell's PATH and can still find git and the player. Checked by launching with `env -i PATH=/usr/bin:/bin`.
- **One view of the state.** The window and tray both render one snapshot, which is `jingle status --json` plus a one-line summary. It refreshes after every change made from the app, whenever the window gains focus, and when the mtime of the config, `state.json`, the global gitconfig, or the jingle binary changes (checked every 1.5 s). A `jingle config set` or `jingle uninstall` typed in a terminal shows up in the app within about 2 s. If jingle is missing or failing, the app looks for it again every 6 s, so installing the CLI while the app is open just works.
- **Changes to the CLI** (both additive, `schema_version` stays 1):
  - `jingle preview <builtin:name|file> [--wait]` plays any sound at the configured volume without adding it anywhere, so the user can hear a sound before choosing it. `play --sound` only takes sounds already in the list.
  - `jingle schema` now lists `sound_types` (accepted file extensions), so the file picker's filter comes from jingle instead of a copy in the app. Key ranges, `push.when` values, events, and built-ins also come from `schema`.
- **Tray menu:** a summary line (On / Off / Not installed / On, N problems; click it to open settings), Sounds on, Commit / Push / Merge, Play ▸ event, Settings…, Quit. No Dock icon. Closing the window keeps the app running, and a second launch opens the existing window.
- **Verified:** Rust unit tests cover locating jingle (in order), parsing output (result, error envelope, the doctor report on exit 1, non-JSON output) and timeouts. The settings UI was driven in Chrome through a stand-in for the Tauri bridge that ran the real CLI in a sandbox. Every control changed `config list` as expected, and play and preview reached the stub player with the right file. With the native app running in a sandbox, a CLI change refreshed it within 2 s, and an uninstall left the sandbox gitconfig byte-identical to before the install. **Not yet verified by hand:** the tray menu itself (the terminal had no screen-recording or accessibility permission), and Linux at all; CI builds and tests it on Ubuntu.
- **Deferred:** packaging and release (dmg / cask / AppImage, signing, notarization) and launch at login. The app already accepts `--hidden`, which starts it in the menu bar only, for when launch at login is added.

---

## Open questions

- Should commits made by agents (e.g. detected by commit author or environment) have their own toggle or sound?
- How does the tool behave in CI and other headless environments (detect them and stay silent)?
- ~~Licensing for bundled default sounds.~~ Resolved: they are synthesized in code (MIT).
