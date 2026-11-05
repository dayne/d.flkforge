// flkforge forges a new flk app from snippets. See README.md and SPEC.md.
package main

import (
	"os"

	"github.com/dayne/d.flkforge/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.Options{}))
}
