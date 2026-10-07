// Spike: start an OS audio player fully detached and return immediately.
// Usage: playback [-v volume] <file>
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	vol := flag.String("v", "1", "afplay volume (0..1)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: playback [-v volume] <file>")
		os.Exit(0) // hooks must never fail
	}
	cmd := exec.Command("afplay", "-v", *vol, flag.Arg(0))
	// New session: survives the hook/shell exiting, gets no terminal signals.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// nil std streams => /dev/null, so git never waits on our pipes.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		os.Exit(0)
	}
	_ = cmd.Process.Release()
}
