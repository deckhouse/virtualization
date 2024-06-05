package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/deckhouse/virtualization/shatal/internal/api"
	"github.com/deckhouse/virtualization/shatal/internal/config"
	"github.com/deckhouse/virtualization/shatal/internal/logger"
	"github.com/deckhouse/virtualization/shatal/internal/shatal"
)

func main() {
	conf, err := config.New()
	if err != nil {
		panic(err)
	}

	var log *slog.Logger
	if conf.Debug {
		log = logger.New(logger.NewDebugOption())
	} else {
		log = logger.New()
	}

	client, err := api.NewClient(conf.Kubeconfig, conf.Namespace, conf.ResourcesPrefix, log)
	if err != nil {
		panic(err)
	}

	service, err := shatal.New(client, conf, log)
	if err != nil {
		panic(err)
	}

	service.Run()

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	<-exit

	service.Stop()
}
