package main

import (
	"os"

	"github.com/deckhouse/virtualization/src/pkg/cli/pkg/command"
)

func main() {
	programName := "d8"
	virtCmd, _ := command.NewCommand(programName)
	if err := virtCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
