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

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"
)

const (
	completeMessage = "Import Complete"

	sourceRegistry = "registry"

	contentTypeKubeVirt = "kubevirt"
	contentTypeArchive  = "archive"
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func touchDoneFile() {
	doneFile, _ := util.ParseEnvVar(common.ImporterDoneFile, false)
	if doneFile == "" {
		return
	}
	f, err := os.OpenFile(doneFile, os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		klog.Errorf("Failed creating file %s: %+v", doneFile, err)
	}
	f.Close()
}

func main() {
	defer klog.Flush()

	certsDirectory, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(certsDirectory)
	prometheusutil.StartPrometheusEndpoint(certsDirectory)
	klog.V(1).Infoln("Starting importer")

	source, _ := util.ParseEnvVar(common.ImporterSource, false)
	contentType, _ := util.ParseEnvVar(common.ImporterContentType, false)
	imageSize, _ := util.ParseEnvVar(common.ImporterImageSize, false)
	filesystemOverhead, _ := strconv.ParseFloat(os.Getenv(common.FilesystemOverheadVar), 64)
	preallocation := false

	volumeMode := v1.PersistentVolumeBlock
	if _, err := os.Stat(common.WriteBlockPath); os.IsNotExist(err) {
		volumeMode = v1.PersistentVolumeFilesystem
	}

	// With writeback cache mode it's possible that the process will exit before all writes have been committed to storage.
	// To guarantee that our write was committed to storage, we make a fsync syscall and ensure success.
	// Also might be a good idea to sync any chmod's we might have done.
	defer fsyncDataFile(contentType, volumeMode)

	//Registry import currently support kubevirt content type only
	if contentType != contentTypeKubeVirt && source == sourceRegistry {
		klog.Errorf("Unsupported content type %s when importing from %s", contentType, source)
		os.Exit(1)
	}

	if _, err := util.GetAvailableSpaceByVolumeMode(volumeMode); err != nil {
		klog.Errorf("%+v", err)
		os.Exit(1)
	}

	exitCode := handleImport(source, contentType, volumeMode, imageSize, filesystemOverhead, preallocation)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func handleImport(
	source string,
	contentType string,
	volumeMode v1.PersistentVolumeMode,
	imageSize string,
	filesystemOverhead float64,
	preallocation bool) int {
	klog.V(1).Infoln("begin import process")

	ds := newDataSource(source)
	defer ds.Close()

	processor := newDataProcessor(contentType, volumeMode, ds, imageSize, filesystemOverhead, preallocation)
	err := processor.ProcessData()

	scratchSpaceRequired := errors.Is(err, importer.ErrRequiresScratchSpace)
	if err != nil && !scratchSpaceRequired {
		klog.Errorf("%+v", err)
		if err := util.WriteTerminationMessage(fmt.Sprintf("Unable to process data: %v", err.Error())); err != nil {
			klog.Errorf("%+v", err)
		}
		return 1
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
		// Exiting instead of returning 0 as normally to avoid clashing
		// with cleanup functions (fsyncDataFile) that assume the imported
		// file will be there during regular exit.
		os.Exit(0)
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

func newDataProcessor(contentType string, volumeMode v1.PersistentVolumeMode, ds importer.DataSourceInterface, imageSize string, filesystemOverhead float64, preallocation bool) *importer.DataProcessor {
	dest := getImporterDestPath(contentType, volumeMode)
	processor := importer.NewDataProcessor(ds, dest, common.ImporterDataDir, common.ScratchDataDir, imageSize, filesystemOverhead, preallocation, os.Getenv(common.CacheMode))
	return processor
}

func getImporterDestPath(contentType string, volumeMode v1.PersistentVolumeMode) string {
	dest := common.ImporterWritePath

	if contentType == contentTypeArchive {
		dest = common.ImporterVolumePath
	}
	if volumeMode == v1.PersistentVolumeBlock {
		dest = common.WriteBlockPath
	}

	return dest
}

func newDataSource(source string) importer.DataSourceInterface {
	ep, _ := util.ParseEnvVar(common.ImporterEndpoint, false)
	acc, _ := util.ParseEnvVar(common.ImporterAccessKeyID, false)
	sec, _ := util.ParseEnvVar(common.ImporterSecretKey, false)
	certDir, _ := util.ParseEnvVar(common.ImporterCertDirVar, false)
	insecureTLS, _ := strconv.ParseBool(os.Getenv(common.InsecureTLSVar))

	switch source {
	case sourceRegistry:
		ds := importer.NewRegistryDataSource(ep, acc, sec, certDir, insecureTLS)
		return ds
	default:
		klog.Errorf("Unknown source type %s\n", source)
		err := util.WriteTerminationMessage(fmt.Sprintf("Unknown data source: %s", source))
		if err != nil {
			klog.Errorf("%+v", err)
		}
		os.Exit(1)
	}

	return nil
}

func fsyncDataFile(contentType string, volumeMode v1.PersistentVolumeMode) {
	dataFile := getImporterDestPath(contentType, volumeMode)
	file, err := os.Open(dataFile)
	if err != nil {
		klog.Errorf("could not get file descriptor for fsync call: %+v", err)
		os.Exit(1)
	}
	if err := file.Sync(); err != nil {
		klog.Errorf("could not fsync following qemu-img writing: %+v", err)
		os.Exit(1)
	}
	klog.V(3).Infof("Successfully completed fsync(%s) syscall, committed to disk\n", dataFile)
	file.Close()
}
