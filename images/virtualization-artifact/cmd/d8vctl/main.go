package main

import (
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/d8vctl"
)

func main() {
	d8vctl.Execute()
	os.Exit(0)
}
