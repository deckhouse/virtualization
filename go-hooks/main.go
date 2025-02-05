package main

import (
	_ "tlscertificate/subfolder"

	"github.com/deckhouse/module-sdk/pkg/app"
)

func main() {
	app.Run()
}
