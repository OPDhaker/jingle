// Package hooks installs and uninstalls jingle's global git hooks.
//
// It owns the POSIX sh shims in the hooks dir, setting git's global
// core.hooksPath, saving the previous value to state.json, and restoring it
// exactly on uninstall. Shims always chain to the repo's own .git/hooks and to
// the previous global hooks path, passing stdin, args, and exit codes through.
// See spikes/README.md for the push-detection design these shims implement.
package hooks
