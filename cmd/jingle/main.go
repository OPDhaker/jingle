// Command jingle plays a producer tag when you git commit or push.
package main

import (
	"os"

	"github.com/OPDhaker/jingle/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
