package output

// ExitCode documents one process exit status.
type ExitCode struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

// ExitCodes lists every exit status, as documented in docs/json.md.
var ExitCodes = []ExitCode{
	{ExitOK, `The requested state was reached, including "unchanged" (already that way).`},
	{ExitError, "Error: the requested state was not reached. For `doctor`: at least one problem has severity `error`."},
	{ExitUsage, "Usage: bad command, flag, or argument, or `--yes` missing on `install`/`uninstall`."},
}

// Code documents one error code and the exit status it comes with.
type Code struct {
	Code    string `json:"code"`
	Exit    int    `json:"exit"`
	Meaning string `json:"meaning"`
}

// ErrorCodes lists every error code a command can return. A test keeps it in
// step with the codes used in the source and with docs/json.md, so a new code
// goes here first.
var ErrorCodes = []Code{
	{"usage", ExitUsage, "Unknown command or flag, wrong number of arguments."},
	{"confirmation_required", ExitUsage, "`install`/`uninstall` without `--yes` or `--dry-run`."},
	{"invalid_key", ExitUsage, "`config` with an unknown key, or `add`/`remove` on a key that isn't a sound list."},
	{"invalid_value", ExitUsage, "`config set` with a value of the wrong type or out of range, an audio file of an unsupported type, or an unknown `builtin:` sound; `config remove` of a sound that isn't in the list."},
	{"unknown_event", ExitUsage, "`play` with an event other than `commit`, `push`, `merge`."},
	{"not_a_repo", ExitError, "`doctor --repo <dir>` where the dir isn't in a git repository."},
	{"git_not_found", ExitError, "git is not installed or not on PATH."},
	{"git_failed", ExitError, "`doctor`: a git command failed unexpectedly while checking the repo."},
	{"install_failed", ExitError, "`install` could not finish (git config or file write failed). Safe to retry."},
	{"uninstall_failed", ExitError, "`uninstall` could not finish. Safe to retry."},
	{"config_invalid", ExitError, "The config file can't be parsed."},
	{"sound_not_found", ExitError, "`config set/add` with a file that doesn't exist, or `play --sound` naming a sound that isn't in the event's list."},
	{"no_sound", ExitError, "`play` for an event with no sounds set."},
	{"sound_missing", ExitError, "`play`: every sound file for the event is gone."},
	{"no_player", ExitError, "No audio player found, or `JINGLE_PLAYER` isn't executable."},
	{"player_failed", ExitError, "The player could not be started (or, with `play --wait`, exited non-zero)."},
	{"paths_unavailable", ExitError, "The config or data dir can't be determined (no home dir)."},
	{"io_error", ExitError, "Some other filesystem error."},
	{"problems_found", ExitError, "`doctor` found at least one `error` problem. The report is still on stdout."},
}
