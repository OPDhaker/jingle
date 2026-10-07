package hooks

import "github.com/OPDhaker/jingle/internal/config"

// Inspection is the install state as seen from git config, state.json, and
// the hooks dir.
type Inspection struct {
	// Installed: the global core.hooksPath points at jingle's hooks dir.
	Installed         bool    `json:"installed"`
	HooksDir          string  `json:"hooks_dir"`
	GlobalHooksPath   *string `json:"global_hooks_path"`
	SystemHooksPath   *string `json:"system_hooks_path"`
	PreviousHooksPath *string `json:"previous_hooks_path"`
	ChainDir          string  `json:"chain_dir"`
	// JinglePath is the binary the installed shims call.
	JinglePath string `json:"jingle_path"`
	// StateInstalled: state.json records an install.
	StateInstalled bool `json:"state_installed"`
	// ShimsCurrent: every shim exists and matches what this jingle would write.
	ShimsCurrent bool `json:"shims_current"`
}

// Inspect reports the current install state without changing anything.
func Inspect(env Env) (Inspection, error) {
	in := Inspection{HooksDir: env.Paths.HooksDir()}
	cur, ok, err := env.Git.GetGlobal(HooksPathKey)
	if err != nil {
		return in, err
	}
	if ok {
		in.GlobalHooksPath = &cur
		in.Installed = samePath(expandHome(cur, env.Home), in.HooksDir)
	}
	if sys, ok, err := env.Git.GetSystem(HooksPathKey); err != nil {
		return in, err
	} else if ok {
		in.SystemHooksPath = &sys
	}
	st, err := config.LoadState(env.Paths)
	if err != nil {
		return in, err
	}
	in.StateInstalled = st.Installed
	in.PreviousHooksPath, in.ChainDir, in.JinglePath = st.PreviousHooksPath, st.ChainDir, st.JinglePath
	if st.Installed {
		shims := Shims(ShimConfig{Jingle: st.JinglePath, Chain: st.ChainDir, Self: in.HooksDir})
		writes, stale, err := diffShims(in.HooksDir, shims)
		if err != nil {
			return in, err
		}
		in.ShimsCurrent = len(writes) == 0 && len(stale) == 0
	}
	return in, nil
}
