package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/image"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"
)

const (
	completeMessage = "Import Complete"

	nbdConnectTimeout = 10 * time.Minute
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	os.Exit(run())
}

func run() int {
	defer klog.Flush()

	certsDirectory, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(certsDirectory) }()
	prometheusutil.StartPrometheusEndpoint(certsDirectory)

	nbdEndpoint, err := util.ParseEnvVar(common.ImporterNBDEndpoint, true)
	if err != nil {
		klog.Error(err)
		return 1
	}

	dest := importerDestPath()
	if err := importer.WaitForNBDEndpoint(nbdEndpoint, nbdConnectTimeout); err != nil {
		klog.Errorf("%+v", err)
		if writeErr := util.WriteTerminationMessage(fmt.Sprintf("Unable to connect to NBD source: %v", err.Error())); writeErr != nil {
			klog.Errorf("%+v", writeErr)
		}
		return 1
	}

	if err := image.ConvertNBDToRaw(nbdEndpoint, dest); err != nil {
		klog.Errorf("%+v", err)
		if writeErr := util.WriteTerminationMessage(fmt.Sprintf("Unable to convert NBD image: %v", err.Error())); writeErr != nil {
			klog.Errorf("%+v", writeErr)
		}
		return 1
	}

	if err := fsyncDataFile(dest); err != nil {
		klog.Errorf("%+v", err)
		return 1
	}
	if err := writeTerminationMessage(completeMessage); err != nil {
		klog.Errorf("%+v", err)
		return 1
	}
	return 0
}

func importerDestPath() string {
	if _, err := os.Stat(common.WriteBlockPath); err == nil {
		return common.WriteBlockPath
	}
	return common.ImporterWritePath
}

func writeTerminationMessage(message string) error {
	termMsg := &common.TerminationMessage{Message: &message}
	msg, err := termMsg.String()
	if err != nil {
		return err
	}
	if err := util.WriteTerminationMessage(msg); err != nil {
		return err
	}
	klog.V(1).Infoln(msg)
	return nil
}

func fsyncDataFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file for fsync: %w", err)
	}
	defer file.Close()

	if err := file.Sync(); err != nil {
		return fmt.Errorf("fsync after qemu-img write: %w", err)
	}
	klog.V(3).Infof("Successfully completed fsync(%s) syscall, committed to disk\n", path)
	return nil
}
