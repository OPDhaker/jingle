# Phase 0 spikes

Throwaway experiments. Not shipped, not part of the root Go module. Both scripts are sandboxed (temp `HOME` + `GIT_CONFIG_GLOBAL`) and never touch the real `~/.gitconfig`.

Run on: macOS 27.0 (arm64), git 2.54.0, Go 1.25.2.

## 1. Push detection: `push-detect/run.sh`

Global `core.hooksPath` pointing at logging hooks (`pre-push`, `post-commit`, `post-rewrite`, `reference-transaction` in its `committed` phase). Each hook logs its args, stdin, `$PPID`, `GIT_REFLOG_ACTION`, and any in-progress state files in `$GIT_DIR`.

| Scenario | `pre-push` | `pre-push` stdin | `reference-transaction` committed on `refs/remotes/*` | Same PPID? |
|---|---|---|---|---|
| Successful push | yes | ref lines | yes | **yes** |
| `push --force` (success) | yes | ref lines | yes | yes |
| Rejected (non-ff) | yes | *(empty)* | no | — |
| Everything up-to-date | yes | **empty** | yes (same sha) | yes |
| `push --dry-run` | yes | ref lines | no | — |
| Push to raw URL / path | yes | ref lines | no (no tracking ref) | — |
| Unreachable remote | **no** | — | no | — |
| `fetch` / `pull` | no | — | yes | — (no marker) |

Note: with a local path remote, the remote side's `receive-pack` also runs our global hooks (it updates `refs/heads/*`, from a different process). Filtering on `refs/remotes/*` + PPID ignores it.

**Conclusion: the marker approach works.**
- `pre-push`: if stdin has at least one ref line, write a marker keyed by `$PPID` (the `git push` process).
- `reference-transaction` with arg `committed`: if stdin updates any `refs/remotes/*` **and** a marker for `$PPID` exists → push succeeded → play the push sound, delete the marker.
- A marker left behind by a dry-run or failed push cannot match a later `fetch`, because the fetch is a different process. Prune markers older than a TTL anyway.
- Known gap: pushing to a raw URL, or to a remote with no fetch refspec, updates no tracking ref, so no sound. Acceptable. A Phase 2 setting could fall back to "play on attempt".
- Cost: `reference-transaction` fires 3× (preparing/prepared/committed) for **every** ref update, including commits, checkouts, and rebases. The shim must exit in plain `sh` unless the arg is `committed` and a marker for `$PPID` exists. Only then does it start `jingle`.

**`post-commit` findings (Phase 2 suppression):**
| Operation | `post-commit` fires | Detectable by |
|---|---|---|
| Normal commit | once | — |
| `commit --amend` | once (+ `post-rewrite amend`) | — |
| `rebase` / `pull --rebase` (N commits) | **N times** | `$GIT_DIR/rebase-merge/` exists |
| `cherry-pick` | once | `$GIT_DIR/CHERRY_PICK_HEAD` exists |
| `merge` (non-ff) | **never** (runs `post-merge`) | — |

`GIT_REFLOG_ACTION` is unreliable (empty for a plain `rebase`), so check the state files instead.

## 2. Detached playback: `playback/`

`main.go` starts `afplay` with `Setsid: true`, nil stdio (→ `/dev/null`), and `Process.Release()`. The hook runs it as `"$BIN" ... >/dev/null 2>&1 </dev/null &`.

`bench.sh` (`N=30 MODES="none detached" ./bench.sh <bin>`), time per `git commit`:

| Hook | ms/commit |
|---|---|
| none | 13–21 |
| detached shim + Go launcher | 28–41 |
| synchronous `afplay` in hook | ~2391 |

**Conclusion:** the detached launch adds roughly 10–30 ms, which is within the 50 ms budget, and most of that is the cost of having any hook at all (an empty `exit 0` hook measured the same). Git does not wait on the player. The Go launcher alone takes about 4 ms. Played audibly once on macOS.

**Not verified here** (Docker/OrbStack daemon was not running):
- Linux: player detection order `pw-play` → `paplay` → `aplay` → `ffplay -nodisp -autoexit`. Verify detached launch in the CI ubuntu job.
- Windows: candidate `powershell -NoProfile -WindowStyle Hidden -Command "(New-Object System.Media.SoundPlayer '<wav>').PlaySync()"` handles WAV only. MP3 needs `System.Windows.Media.MediaPlayer` (PresentationCore) with a sleep loop. Detach with `CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS`. To be verified in Phase 3.
