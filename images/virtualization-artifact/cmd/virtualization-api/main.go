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
	"fmt"
	"os"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/logs"

	"github.com/deckhouse/virtualization-controller/cmd/virtualization-api/app"
)

func main() {
	logs.InitLogs()

	if err := app.NewAPIServerCommand().ExecuteContext(genericapiserver.SetupSignalContext()); err != nil {
		logs.FlushLogs()
		_, _ = fmt.Fprintf(os.Stderr, "ERROR:%v\n", err)
		os.Exit(1)
	}

	logs.FlushLogs()
}
