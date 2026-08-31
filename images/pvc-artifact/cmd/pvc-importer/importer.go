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

// importer.go imports a registry image into a target PVC.
// This process expects several environmental variables:
//    ImporterEndpoint       Source registry image URL.
//    ImporterAccessKeyID  Optional. Access key is the user ID that uniquely identifies your
//			      account.
//    ImporterSecretKey     Optional. Secret key is the password to your account.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/common"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/importer"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util"
	prometheusutil "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util/prometheus"
)

const (
	completeMessage = "Import Complete"

	sourceRegistry = "registry"

	contentTypeKubeVirt = "kubevirt"
	contentTypeArchive  = "archive"
)

func touchDoneFile() {
	doneFile, _ := util.ParseEnvVar(common.ImporterDoneFile, false)
	if doneFile == "" {
		return
	}
	f, err := os.OpenFile(doneFile, os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		klog.Errorf("Failed creating file %s: %+v", doneFile, err)
	}
	f.Close()
}

func main() {
	os.Exit(run())
}

func run() int {
	// The flags are parsed here rather than in an init function so that the package stays
	// importable by a test binary, whose own flags are not registered yet at init time.
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	certsDirectory, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		return reportFailure(fmt.Errorf("create the certs directory: %w", err))
	}
	defer func() { _ = os.RemoveAll(certsDirectory) }()
	prometheusutil.StartPrometheusEndpoint(certsDirectory)
	klog.V(1).Infoln("Starting importer")

	source, _ := util.ParseEnvVar(common.ImporterSource, false)
	contentType, _ := util.ParseEnvVar(common.ImporterContentType, false)
	imageSize, _ := util.ParseEnvVar(common.ImporterImageSize, false)
	filesystemOverhead, _ := strconv.ParseFloat(os.Getenv(common.FilesystemOverheadVar), 64)
	preallocation := false

	volumeMode := corev1.PersistentVolumeBlock
	if _, err := os.Stat(common.WriteBlockPath); os.IsNotExist(err) {
		volumeMode = corev1.PersistentVolumeFilesystem
	}

	// With writeback cache mode it's possible that the process will exit before all writes have been committed to storage.
	// To guarantee that our write was committed to storage, we make a fsync syscall and ensure success.
	// Also might be a good idea to sync any chmod's we might have done.

	// Registry import currently support kubevirt content type only
	if contentType != contentTypeKubeVirt && source == sourceRegistry {
		return reportFailure(fmt.Errorf("unsupported content type %s when importing from %s", contentType, source))
	}

	if _, err := util.GetAvailableSpaceByVolumeMode(volumeMode); err != nil {
		return reportFailure(err)
	}

	exitCode := handleImport(source, contentType, volumeMode, imageSize, filesystemOverhead, preallocation)
	if exitCode == scratchExitCode {
		return 0
	}
	if exitCode != 0 {
		return exitCode
	}

	if err := fsyncDataFile(contentType, volumeMode); err != nil {
		// The data did not reach the storage, so the file that is there is not the image:
		// drop it instead of leaving something that looks like an imported disk on the volume.
		if cleanErr := importer.CleanAll(getImporterDestPath(contentType, volumeMode)); cleanErr != nil {
			klog.Errorf("Unable to remove the target after a failed fsync: %+v", cleanErr)
		}
		return reportFailure(err)
	}
	return 0
}

// reportFailure logs the error, hands it to the controller through the termination message and
// returns the exit code. Every failing exit path goes through it: a silent exit leaves the disk
// provisioning forever with the reason nowhere to be found.
func reportFailure(err error) int {
	klog.Errorf("%+v", err)
	if writeErr := importer.WriteFailureMessage(err); writeErr != nil {
		klog.Errorf("%+v", writeErr)
	}
	return 1
}

const scratchExitCode = 2

func handleImport(
	source string,
	contentType string,
	volumeMode corev1.PersistentVolumeMode,
	imageSize string,
	filesystemOverhead float64,
	preallocation bool,
) int {
	klog.V(1).Infoln("begin import process")

	ds, err := newDataSource(source)
	if err != nil {
		return reportFailure(err)
	}
	defer func() {
		if ds != nil {
			_ = ds.Close()
		}
	}()

	processor, err := newDataProcessor(contentType, volumeMode, ds, imageSize, filesystemOverhead, preallocation)
	if err != nil {
		return reportFailure(fmt.Errorf("unable to start import: %w", err))
	}

	err = processor.ProcessData()

	scratchSpaceRequired := errors.Is(err, importer.ErrRequiresScratchSpace)
	if err != nil && !scratchSpaceRequired {
		// A failure past the copy (a size validation, a resize) leaves the target file
		// behind: drop it so the volume does not keep the leftover of an attempt that
		// produced no image. The retry truncates whatever is left anyway, this only keeps
		// the volume from holding it in between. A block device is left alone by CleanAll.
		if cleanErr := importer.CleanAll(getImporterDestPath(contentType, volumeMode)); cleanErr != nil {
			klog.Errorf("Unable to remove the incomplete target: %+v", cleanErr)
		}
		return reportFailure(fmt.Errorf("unable to process data: %w", err))
	}

	termMsg := ds.GetTerminationMessage()
	if termMsg == nil {
		termMsg = &common.TerminationMessage{}
	}
	termMsg.ScratchSpaceRequired = &scratchSpaceRequired
	termMsg.PreallocationApplied = ptr.To(processor.PreallocationApplied())
	termMsg.Message = ptr.To(completeMessage)

	touchDoneFile()
	if err := writeTerminationMessage(termMsg); err != nil {
		klog.Errorf("%+v", err)
		return 1
	}

	if scratchSpaceRequired {
		return scratchExitCode
	}

	return 0
}

func writeTerminationMessage(termMsg *common.TerminationMessage) error {
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

func newDataProcessor(contentType string, volumeMode corev1.PersistentVolumeMode, ds importer.DataSourceInterface, imageSize string, filesystemOverhead float64, preallocation bool) (*importer.DataProcessor, error) {
	dest := getImporterDestPath(contentType, volumeMode)
	qemuConvertThreads, _ := strconv.Atoi(os.Getenv(common.ImporterQemuConvertThreads))
	return importer.NewDataProcessor(ds, dest, common.ImporterDataDir, common.ScratchDataDir, imageSize, filesystemOverhead, preallocation, os.Getenv(common.CacheMode), qemuConvertThreads)
}

func getImporterDestPath(contentType string, volumeMode corev1.PersistentVolumeMode) string {
	dest := common.ImporterWritePath

	if contentType == contentTypeArchive {
		dest = common.ImporterVolumePath
	}
	if volumeMode == corev1.PersistentVolumeBlock {
		dest = common.WriteBlockPath
	}

	return dest
}

func newDataSource(source string) (importer.DataSourceInterface, error) {
	ep, _ := util.ParseEnvVar(common.ImporterEndpoint, false)
	acc, _ := util.ParseEnvVar(common.ImporterAccessKeyID, false)
	sec, _ := util.ParseEnvVar(common.ImporterSecretKey, false)
	certDir, _ := util.ParseEnvVar(common.ImporterCertDirVar, false)
	insecureTLS, _ := strconv.ParseBool(os.Getenv(common.InsecureTLSVar))
	directTransfer, _ := strconv.ParseBool(os.Getenv(common.ImporterDirectTransfer))

	if source != sourceRegistry {
		return nil, fmt.Errorf("unknown data source: %s", source)
	}
	return importer.NewRegistryDataSource(ep, acc, sec, certDir, insecureTLS, directTransfer), nil
}

// fsyncDataFile commits the written image to storage. Its failure means the data never got
// there, so it must reach the user like any other import failure instead of exiting silently.
func fsyncDataFile(contentType string, volumeMode corev1.PersistentVolumeMode) error {
	dataFile := getImporterDestPath(contentType, volumeMode)
	file, err := os.Open(dataFile)
	if err != nil {
		return fmt.Errorf("could not get file descriptor for fsync call: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("could not fsync following qemu-img writing: %w", err)
	}
	klog.V(3).Infof("Successfully completed fsync(%s) syscall, committed to disk\n", dataFile)
	return nil
}
