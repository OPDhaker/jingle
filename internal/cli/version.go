package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/output"
)

// version is set at build time: -ldflags "-X github.com/OPDhaker/jingle/internal/cli.version=v1.2.3".
var version = "dev"

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func newVersionCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the jingle version",
		Example: `  jingle version
  jingle version --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfo{
				Version: version,
				Commit:  vcsRevision(),
				Go:      runtime.Version(),
				OS:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			}
			return printer(cmd).Result(info, func(w io.Writer) {
				fmt.Fprintf(w, "jingle %s (%s, %s, %s/%s)\n", info.Version, info.Commit, info.Go, info.OS, info.Arch)
			})
		},
	}
}

func vcsRevision() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return s.Value[:12]
			}
		}
	}
	return "unknown"
}
