// Command shale is the agent PR evidence CLI.
// See https://github.com/provasign/shale and docs/ for the full design.
package main

import (
	"os"

	"github.com/provasign/shale/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
