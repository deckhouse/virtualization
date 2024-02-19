package main

import (
	"github.com/deckhouse/virtualization-controller/cmd/virtualization-api/app"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/logs"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	cmd := app.NewAPIServerCommand(genericapiserver.SetupSignalHandler())
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
