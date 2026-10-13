package doctor

// CodeInfo documents one problem code.
type CodeInfo struct {
	Code string `json:"code"`
	// Severities are the severities the problem can have.
	Severities []Severity `json:"severities"`
	Meaning    string     `json:"meaning"`
}

var (
	errorOnly   = []Severity{Error}
	warningOnly = []Severity{Warning}
)

// Codes lists every problem Check can report. A test keeps it in step with
// the checks and with docs/json.md, so a new problem goes here first.
var Codes = []CodeInfo{
	{"not_installed", errorOnly, "Hooks aren't installed."},
	{"hooks_path_overridden", errorOnly, "Something changed the global `core.hooksPath` after install."},
	{"repo_hooks_path_override", errorOnly, "This repo sets its own `core.hooksPath` (husky and similar), so jingle doesn't run here."},
	{"shims_outdated", warningOnly, "Installed hooks are from another jingle version."},
	{"jingle_binary_moved", warningOnly, "The binary the hooks call is gone, but `jingle` is on PATH."},
	{"jingle_binary_missing", errorOnly, "The binary the hooks call is gone and `jingle` isn't on PATH."},
	{"config_invalid", errorOnly, "The config file can't be parsed."},
	{"no_sound", errorOnly, "An enabled event has no sounds."},
	{"sound_missing", []Severity{Error, Warning}, "Every sound file for an event is gone, or names an unknown built-in (error), or only some of them are (warning)."},
	{"no_player", errorOnly, "No audio player found, or `JINGLE_PLAYER` is bad."},
	{"volume_unsupported", warningOnly, "`volume` < 100 but the player can't change the volume."},
	{"git_not_found", errorOnly, "git isn't installed."},
	{"git_too_old", warningOnly, "git < 2.28: push plays on attempt, not on success."},
	{"git_failed", errorOnly, "A git command failed unexpectedly."},
}
