# jingle JSON output

Every command takes `--json`. This page is the contract for that output: field names, types, exit codes, and error codes. Agents and scripts can rely on it.

Schema version: **1** (reported by `jingle version --json` as `schema_version`).

## Rules

- A successful result is one JSON object on **stdout**. Nothing else is printed to stdout.
- An error is one JSON object on **stderr** (the [error envelope](#errors)), and stdout is empty. One exception: `doctor` exits 1 with `problems_found` on stderr but still prints its full report on stdout.
- Paths are absolute. Lists are `[]` when empty, never `null`. A field documented as `string | null` is `null` when the thing is unset.
- **Versioning.** New fields can be added at any time without a version bump, so ignore fields you don't know. Renaming, removing, or changing the type or meaning of a field is a breaking change: `schema_version` goes up and the change is listed under [Schema history](#schema-history).
- The exact output of every command is pinned by golden tests in `internal/cli/testdata/json/`, which also serve as examples.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | The requested state was reached, including "unchanged" (already that way). |
| 1 | Error: the requested state was not reached. For `doctor`: at least one problem has severity `error`. |
| 2 | Usage: bad command, flag, or argument, or `--yes` missing on `install`/`uninstall`. |

New codes may be added; existing ones are never renumbered.

## Errors

```json
{"error": {"code": "invalid_key", "message": "unknown config key \"nope\" (valid keys: ...)"}}
```

`code` is stable and machine-readable. `message` is for humans and may change.

| Code | Exit | When |
|---|---|---|
| `usage` | 2 | Unknown command or flag, wrong number of arguments. |
| `confirmation_required` | 2 | `install`/`uninstall` without `--yes` or `--dry-run`. |
| `invalid_key` | 2 | `config` with an unknown key, or `add`/`remove` on a key that isn't a sound list. |
| `invalid_value` | 2 | `config set` with a value of the wrong type or out of range, or an audio file of an unsupported type; `config remove` of a sound that isn't in the list. |
| `unknown_event` | 2 | `play` with an event other than `commit`, `push`, `merge`. |
| `not_a_repo` | 1 | `doctor --repo <dir>` where the dir isn't in a git repository. |
| `git_not_found` | 1 | git is not installed or not on PATH. |
| `install_failed` | 1 | `install` could not finish (git config or file write failed). Safe to retry. |
| `uninstall_failed` | 1 | `uninstall` could not finish. Safe to retry. |
| `config_invalid` | 1 | The config file can't be parsed. |
| `sound_not_found` | 1 | `config set/add` with a file that doesn't exist. |
| `no_sound` | 1 | `play` for an event with no sounds set, or `--sound` naming one that isn't in the list. |
| `sound_missing` | 1 | `play`: every sound file for the event is gone. |
| `no_player` | 1 | No audio player found, or `JINGLE_PLAYER` isn't executable. |
| `player_failed` | 1 | The player could not be started (or, with `play --wait`, exited non-zero). |
| `paths_unavailable` | 1 | The config or data dir can't be determined (no home dir). |
| `io_error` | 1 | Some other filesystem error. |
| `problems_found` | 1 | `doctor` found at least one `error` problem. The report is still on stdout. |

## Commands

### `version`

```json
{"version": "v0.1.0", "commit": "8ad4021ed92c", "go": "go1.25.2", "os": "darwin", "arch": "arm64", "schema_version": 1}
```

| Field | Type | Meaning |
|---|---|---|
| `version` | string | Release version (`dev` for local builds). |
| `commit` | string | First 12 chars of the git commit it was built from, or `unknown`. |
| `go`, `os`, `arch` | string | Go version and build target. |
| `schema_version` | int | Version of this JSON contract. |

### `install`, `uninstall`

Both return the same shape. `--dry-run` returns what *would* change and changes nothing.

```json
{
  "status": "installed",
  "hooks_dir": "/Users/me/.local/share/jingle/hooks",
  "previous_hooks_path": null,
  "chain_dir": "",
  "changes": [
    {"action": "write_state", "target": "/Users/me/.local/share/jingle/state.json"},
    {"action": "write_hook", "target": "/Users/me/.local/share/jingle/hooks/post-commit"},
    {"action": "set_git_config", "target": "core.hooksPath", "to": "/Users/me/.local/share/jingle/hooks"}
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `status` | string | `installed`, `uninstalled`, `unchanged` (nothing to do), or `would_change` (dry run). |
| `hooks_dir` | string | jingle's hooks dir. |
| `previous_hooks_path` | string \| null | The global `core.hooksPath` from before install, which uninstall restores. `null` if there was none. |
| `chain_dir` | string | The hooks dir the shims chain to; `""` means each repo's own `.git/hooks`. |
| `changes` | array | Steps taken (or, for a dry run, that would be taken), in order. `[]` when unchanged. |
| `changes[].action` | string | `write_state`, `remove_state`, `write_hook`, `remove_hook`, `remove_dir`, `remove_file`, `set_git_config`, `unset_git_config`, `remove_git_section`. |
| `changes[].target` | string | A path, git config key, or section name. |
| `changes[].from`, `.to` | string | Old and new value, for git config changes. Omitted when empty. |
| `changes[].note` | string | A condition on the step, such as `only if empty`. Omitted when empty. |
| `notes` | array of string | Things the caller should know, such as a dir `--purge` kept because it holds other files. Omitted when empty. |

`uninstall --purge` adds `remove_file` (config file), `remove_dir` with note `imported sounds`, and `remove_dir` for the config and data dirs.

### `config get`

```json
{"key": "push.when", "value": "success"}
```

### `config set`, `config add`, `config remove`

```json
{"key": "commit.sounds", "value": ["/Users/me/.local/share/jingle/sounds/commit-tag.wav"], "changed": true}
```

| Field | Type | Meaning |
|---|---|---|
| `key` | string | The config key. |
| `value` | bool \| int \| string \| array of string | The value after the command, typed by key (see below). Sound lists hold the paths of jingle's own copies. |
| `changed` | bool | `false` when the value was already that way. Only present for `set`/`add`/`remove`. |

### `config list`

```json
{"path": "/Users/me/.config/jingle/config.toml", "values": {"enabled": true, "cooldown": 3, "volume": 100, "commit.enabled": true, "commit.sounds": [], "push.enabled": true, "push.sounds": [], "push.when": "success", "merge.enabled": false, "merge.sounds": []}}
```

`values` has every key, including defaults:

| Key | Type | Default |
|---|---|---|
| `enabled` | bool | `true` |
| `cooldown` | int (seconds, ≥ 0) | `3` |
| `volume` | int (0–100) | `100` |
| `commit.enabled`, `push.enabled` | bool | `true` |
| `merge.enabled` | bool | `false` |
| `commit.sounds`, `push.sounds`, `merge.sounds` | array of string | `[]` |
| `push.when` | string: `success` \| `attempt` | `success` |

### `play`

```json
{"event": "commit", "sound": "/Users/me/.local/share/jingle/sounds/commit-tag.wav", "sounds": 1, "player": {"name": "afplay", "path": "/usr/bin/afplay", "args": [], "volume": true}, "volume": 100, "enabled": true, "waited": false}
```

| Field | Type | Meaning |
|---|---|---|
| `event` | string | The event played. |
| `sound` | string | The file that played (picked at random from the event's sounds that exist). |
| `sounds` | int | How many sounds the event has. |
| `player` | object | See [player](#player). |
| `volume` | int | Volume used (0–100). |
| `enabled` | bool | Whether this event would play on a real commit/push/merge (`play` plays even when it wouldn't). |
| `waited` | bool | Whether `play` waited for the player to finish (`--wait`). |

### `status`, `doctor`

`status` and `doctor` print the same report. `doctor` also checks the repo in the current dir (or `--repo <dir>`), adds `repo`, and exits 1 if any problem has severity `error`. `status` always exits 0.

| Field | Type | Meaning |
|---|---|---|
| `installed` | bool | The global `core.hooksPath` points at jingle's hooks dir. |
| `hooks_dir` | string | jingle's hooks dir. |
| `global_hooks_path` | string \| null | Current global `core.hooksPath`. |
| `system_hooks_path` | string \| null | Current system-level `core.hooksPath`. |
| `previous_hooks_path` | string \| null | The value saved at install. |
| `chain_dir` | string | Where the shims chain to (`""` = each repo's own hooks). |
| `jingle_path` | string | The binary the installed shims call (`""` if not installed). |
| `state_installed` | bool | `state.json` records an install. |
| `shims_current` | bool | Every shim exists and matches what this jingle writes. |
| `jingle_executable` | string | Path of the jingle running this command. |
| `config_path` | string | The config file. |
| `config` | object | Effective config, same as `config list`'s `values`. |
| `player` | object \| null | The detected [player](#player); `null` if none. |
| `git` | object \| null | `{"path": string, "version": string}`; `null` if git isn't found. |
| `repo` | object | `doctor` only: `path` (repo root), `hooks_path` (string \| null, the `core.hooksPath` git uses there), `hooks_path_scope` (`global`, `local`, `worktree`, ... ; omitted if unset), `husky` (bool). |
| `problems` | array | Known problems, `[]` if none. |
| `problems[].code` | string | Stable problem code (below). |
| `problems[].severity` | string | `error` (sounds won't play) or `warning` (sounds play, but worse). |
| `problems[].message` | string | For humans. |
| `problems[].fix` | string | A command or step that fixes it. Omitted when there's none. |

Problem codes:

| Code | Severity | Meaning |
|---|---|---|
| `not_installed` | error | Hooks aren't installed. |
| `hooks_path_overridden` | error | Something changed the global `core.hooksPath` after install. |
| `repo_hooks_path_override` | error | This repo sets its own `core.hooksPath` (husky and similar), so jingle doesn't run here. |
| `shims_outdated` | warning | Installed hooks are from another jingle version. |
| `jingle_binary_moved` | warning | The binary the hooks call is gone, but `jingle` is on PATH. |
| `jingle_binary_missing` | error | The binary the hooks call is gone and `jingle` isn't on PATH. |
| `config_invalid` | error | The config file can't be parsed. |
| `no_sound` | error | An enabled event has no sounds. |
| `sound_missing` | error / warning | Every sound file for an event is gone (error), or only some of them (warning). |
| `no_player` | error | No audio player found, or `JINGLE_PLAYER` is bad. |
| `volume_unsupported` | warning | `volume` < 100 but the player can't change the volume. |
| `git_not_found` | error | git isn't installed. |
| `git_too_old` | warning | git < 2.28: push plays on attempt, not on success. |
| `git_failed` | error | A git command failed unexpectedly. |

### Player

```json
{"name": "afplay", "path": "/usr/bin/afplay", "args": [], "volume": true}
```

| Field | Type | Meaning |
|---|---|---|
| `name` | string | `afplay`, `pw-play`, `paplay`, `aplay`, `ffplay`, or `JINGLE_PLAYER` (override). |
| `path` | string | Full path of the executable. |
| `args` | array of string | Fixed arguments placed before the volume flags and the file. |
| `volume` | bool | Whether this player can play below full volume. |

## Schema history

- **1**: first published version (v0.1.0).
