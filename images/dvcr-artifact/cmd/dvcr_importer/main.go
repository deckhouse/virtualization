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
	"context"
	"flag"
	"os"

	"github.com/google/go-containerregistry/pkg/logs"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/importer"
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	defer klog.Flush()

	logs.Progress.SetOutput(os.Stdout)
	logs.Warn.SetOutput(os.Stderr)

	klog.Infoln("Starting registry importer")

	imp := importer.New()
	if err := imp.Run(context.Background()); err != nil {
		klog.Fatalf("Error running registry importer: %+v", err)
	}

	klog.Infoln("Finished running registry importer")
}
