/*
Copyright 2018 The CDI Authors.
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

package main

// importer.go copies an NBD source exported by the source importer into a target PVC.
// This process expects several environmental variables:
//    ImporterNBDEndpoint    NBD endpoint URL of the source to copy from.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/common"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/importer"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util"
	prometheusutil "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util/prometheus"
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
		return reportFailure(fmt.Errorf("create the certs directory: %w", err))
	}
	defer func() { _ = os.RemoveAll(certsDirectory) }()
	prometheusutil.StartPrometheusEndpoint(certsDirectory)

	nbdEndpoint, err := util.ParseEnvVar(common.ImporterNBDEndpoint, false)
	if err != nil {
		return reportFailure(err)
	}
	if nbdEndpoint == "" {
		return reportFailure(errors.New("IMPORTER_NBD_ENDPOINT is required"))
	}

	dest := importerDestPath()
	if err := importer.WaitForNBDEndpoint(nbdEndpoint, nbdConnectTimeout); err != nil {
		return reportFailure(fmt.Errorf("unable to connect to NBD source: %w", err))
	}

	// nbdcopy is a separate process copying between two ends, and its exit code says nothing
	// about which one gave up: a source that went away is transient, a target that refuses the
	// write is not. Guessing from the message text is what mislabeled storage failures as
	// failed downloads before, so the copy stays unclassified and the pod keeps retrying.
	if err := importer.CopyNBDToDevice(nbdEndpoint, dest); err != nil {
		return reportFailure(fmt.Errorf("unable to copy NBD image: %w", err))
	}

	// A failed fsync is unambiguous: the data never reached the storage, and the next attempt
	// writes to the same target. It is reported as permanent so the disk fails with the reason
	// instead of provisioning forever.
	if err := fsyncDataFile(dest); err != nil {
		return reportFailure(importer.NewWriteFailedError(err))
	}
	if err := writeTerminationMessage(completeMessage); err != nil {
		klog.Errorf("%+v", err)
		return 1
	}
	return 0
}

// reportFailure hands the reason to the controller through the termination message and returns
// the exit code. The pod runs with RestartPolicy: OnFailure and never reaches PodFailed, so a
// silent exit leaves the disk provisioning forever with the reason nowhere to be found.
func reportFailure(err error) int {
	klog.Errorf("%+v", err)
	if writeErr := importer.WriteFailureMessage(err); writeErr != nil {
		klog.Errorf("%+v", writeErr)
	}
	return 1
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
