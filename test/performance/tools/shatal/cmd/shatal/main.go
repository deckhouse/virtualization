/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
