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
	"flag"
	"os"

	"github.com/google/go-containerregistry/pkg/logs"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/importer"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/runnablegroup"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/server"
)

const (
	flagProbeAddr = "health-probe-bind-address"
	flagPprofAddr = "pprof-bind-address"

	HealthProbeBindAddressEnv = "HEALTH_PROBE_BIND_ADDRESS"
	PprofBindAddressEnv       = "PPROF_BIND_ADDRESS"
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

type Options struct {
	ProbeAddr string
	PprofAddr string
}

func NewOptions() Options {
	return Options{}
}

func (o *Options) Flags(fs *flag.FlagSet) {
	fs.StringVar(&o.ProbeAddr, flagProbeAddr, os.Getenv(HealthProbeBindAddressEnv), "The address the probe endpoint binds to.")
	fs.StringVar(&o.PprofAddr, flagPprofAddr, os.Getenv(PprofBindAddressEnv), "The address the pprof endpoint binds to.")
}

func main() {
	defer klog.Flush()

	logs.Progress.SetOutput(os.Stdout)
	logs.Warn.SetOutput(os.Stderr)

	opts := NewOptions()
	opts.Flags(flag.CommandLine)
	flag.Parse()

	klog.Infoln("Starting registry importer")

	rgroup := runnablegroup.NewRunnableGroup()

	imp := importer.New()
	rgroup.Add(imp)

	serverOptions := server.Options{
		HealthProbeBindAddress: opts.ProbeAddr,
		PprofBindAddress:       opts.PprofAddr,
	}

	srv, err := server.NewServer(serverOptions)
	if err != nil {
		klog.Fatalf("Error starting registry importer: %+v", err)
	}
	rgroup.Add(srv)

	ctx := signals.SetupSignalHandler()

	if err = rgroup.Run(ctx); err != nil {
		klog.Fatalf("Error running registry importer: %+v", err)
	}

	klog.Infoln("Finished running registry importer")
}
