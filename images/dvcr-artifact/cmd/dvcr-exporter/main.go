package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/cmd/dvcr-exporter/app"
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	defer klog.Flush()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.NewDVCRExporterCommand().ExecuteContext(ctx); err != nil {
		klog.Error(err)
		os.Exit(1)
	}
}
