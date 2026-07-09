/*
Copyright 2026 Flant JSC

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

// This file was initially copied from the Containerized Data Importer (CDI)
// project (https://github.com/kubevirt/containerized-data-importer) and adapted
// for the virtualization module.

package main

// importer.go copies an NBD source exported by the source importer into a target PVC.
// This process expects several environmental variables:
//    ImporterNBDEndpoint    NBD endpoint URL of the source to copy from.

import (
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
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

	nbdEndpoint, err := util.ParseEnvVar(common.ImporterNBDEndpoint, false)
	if err != nil {
		klog.Error(err)
		return 1
	}
	if nbdEndpoint == "" {
		klog.Error("IMPORTER_NBD_ENDPOINT is required")
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

	if err := importer.CopyNBDToDevice(nbdEndpoint, dest); err != nil {
		klog.Errorf("%+v", err)
		if writeErr := util.WriteTerminationMessage(fmt.Sprintf("Unable to copy NBD image: %v", err.Error())); writeErr != nil {
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
		return fmt.Errorf("fsync after nbdcopy write: %w", err)
	}
	klog.V(3).Infof("Successfully completed fsync(%s) syscall, committed to disk\n", path)
	return nil
}
